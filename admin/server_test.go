// MIT License
//
// Copyright (c) 2025-2026 Arsene Tochemey Gandote
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package admin

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/groupcache/groupcache-go/v3"
	"github.com/groupcache/groupcache-go/v3/transport"

	"github.com/tochemey/distcache/log"
)

func TestAdminEndpoints(t *testing.T) {
	provider := stubProvider{
		peers: []peerSnapshot{{IsSelf: true}},
		keyspaces: []KeySpaceSnapshot{
			{
				Name: "ks",
				Stats: &KeySpaceStatsSnapshot{
					Gets: 3,
				},
			},
		},
	}

	handler := newHandler(defaultAdminBasePath, provider, nil)
	mux := http.NewServeMux()
	handler.register(mux)

	t.Run("peers endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, defaultAdminBasePath+"/peers", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		var peers []peerSnapshot
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &peers))
		require.Len(t, peers, 1)
		require.True(t, peers[0].IsSelf)
	})

	t.Run("keyspaces endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, defaultAdminBasePath+"/keyspaces", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		var keyspaces []KeySpaceSnapshot
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &keyspaces))
		require.Len(t, keyspaces, 1)
		require.Equal(t, "ks", keyspaces[0].Name)
		require.NotNil(t, keyspaces[0].Stats)
		require.Equal(t, int64(3), keyspaces[0].Stats.Gets)
	})
}

func TestHandlerMethodNotAllowed(t *testing.T) {
	provider := stubProvider{}
	handler := newHandler(defaultAdminBasePath, provider, nil)
	mux := http.NewServeMux()
	handler.register(mux)

	tests := []struct {
		name string
		path string
	}{
		{name: "peers", path: defaultAdminBasePath + "/peers"},
		{name: "keyspaces", path: defaultAdminBasePath + "/keyspaces"},
		{name: "healthz", path: defaultAdminBasePath + "/healthz"},
		{name: "readyz", path: defaultAdminBasePath + "/readyz"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, test.path, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
		})
	}
}

func TestHealthzAlwaysOK(t *testing.T) {
	handler := newHandler(defaultAdminBasePath, stubProvider{}, nil)
	mux := http.NewServeMux()
	handler.register(mux)

	req := httptest.NewRequest(http.MethodGet, defaultAdminBasePath+"/healthz", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestReadyzReflectsProvider(t *testing.T) {
	t.Run("not ready returns 503", func(t *testing.T) {
		handler := newHandler(defaultAdminBasePath, stubProvider{ready: false}, nil)
		mux := http.NewServeMux()
		handler.register(mux)

		req := httptest.NewRequest(http.MethodGet, defaultAdminBasePath+"/readyz", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	})

	t.Run("ready returns 200", func(t *testing.T) {
		handler := newHandler(defaultAdminBasePath, stubProvider{ready: true}, nil)
		mux := http.NewServeMux()
		handler.register(mux)

		req := httptest.NewRequest(http.MethodGet, defaultAdminBasePath+"/readyz", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestHandlerPeersError(t *testing.T) {
	expectedErr := "peer snapshot error"
	provider := stubProvider{
		peersErr: errStub(expectedErr),
	}
	logger := &countingLogger{Logger: log.DiscardLogger}
	handler := newHandler(defaultAdminBasePath, provider, logger)
	mux := http.NewServeMux()
	handler.register(mux)

	req := httptest.NewRequest(http.MethodGet, defaultAdminBasePath+"/peers", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	// the detail is logged, never returned to the client
	require.Equal(t, "internal error\n", rec.Body.String())
	require.NotContains(t, rec.Body.String(), expectedErr)
	require.Equal(t, int64(1), logger.errorfCalls())
}

func TestHandlerHeadProbes(t *testing.T) {
	handler := newHandler(defaultAdminBasePath, stubProvider{ready: true}, nil)
	mux := http.NewServeMux()
	handler.register(mux)

	for _, path := range []string{"/healthz", "/readyz"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodHead, defaultAdminBasePath+path, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			require.Equal(t, http.StatusOK, rec.Code)
		})
	}
}

func TestSnapshotKeySpaceStats(t *testing.T) {
	statsGroup := &groupWithStats{}
	statsGroup.stats.Gets.Add(2)
	statsGroup.stats.CacheHits.Add(1)

	snapshot := SnapshotKeySpaceStats(statsGroup)
	require.NotNil(t, snapshot)
	require.Equal(t, int64(2), snapshot.Gets)
	require.Equal(t, int64(1), snapshot.CacheHits)

	require.Nil(t, SnapshotKeySpaceStats(transportOnlyGroup{}))
}

func TestServerStartProviderRequired(t *testing.T) {
	server := NewServer(Config{ListenAddr: "127.0.0.1:0"}, nil, nil)
	err := server.Start(context.Background())
	require.Error(t, err)
}

func TestServerStartListenError(t *testing.T) {
	server := NewServer(Config{ListenAddr: "127.0.0.1"}, stubProvider{}, nil)
	err := server.Start(context.Background())
	require.Error(t, err)
}

func TestServerStartAndShutdown(t *testing.T) {
	server := NewServer(Config{ListenAddr: "127.0.0.1:0"}, stubProvider{}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	require.NoError(t, server.Start(ctx))
	require.NoError(t, server.Shutdown(context.Background()))
}

func TestServerStartTLSLogsServeError(t *testing.T) {
	logger := &countingLogger{Logger: log.DiscardLogger}
	server := NewServer(Config{
		ListenAddr: "127.0.0.1:0",
		TLSConfig:  &tls.Config{},
	}, stubProvider{}, logger)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	t.Cleanup(func() {
		_ = server.Shutdown(context.Background())
	})

	require.NoError(t, server.Start(ctx))
	require.NotNil(t, server.listener)
	require.NoError(t, server.listener.Close())

	require.Eventually(t, func() bool {
		return logger.errorfCalls() > 0
	}, time.Second, 10*time.Millisecond)
}

func TestServerStartCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	server := NewServer(Config{ListenAddr: "127.0.0.1:0"}, stubProvider{}, nil)
	require.ErrorIs(t, server.Start(ctx), context.Canceled)
	require.Nil(t, server.listener)
}

func TestServerMiddlewareInvoked(t *testing.T) {
	var calls atomic.Int64
	middleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			if r.Header.Get("Authorization") != "Bearer secret" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	server := NewServer(Config{
		ListenAddr: "127.0.0.1:0",
		Middleware: middleware,
	}, stubProvider{ready: true}, log.DiscardLogger)
	require.NoError(t, server.Start(context.Background()))
	t.Cleanup(func() {
		_ = server.Shutdown(context.Background())
	})

	url := "http://" + server.listener.Addr().String() + defaultAdminBasePath + "/readyz"

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	req.Header.Set("Authorization", "Bearer secret")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	require.Equal(t, int64(2), calls.Load())
}

type peerSnapshot struct {
	IsSelf bool `json:"is_self"`
}

type stubProvider struct {
	peers     any
	keyspaces any
	peersErr  error
	ready     bool
}

func (p stubProvider) SnapshotPeers() (any, error) {
	return p.peers, p.peersErr
}

func (p stubProvider) SnapshotKeySpaces() any {
	return p.keyspaces
}

func (p stubProvider) Ready() bool {
	return p.ready
}

type errStub string

func (e errStub) Error() string {
	return string(e)
}

type groupWithStats struct {
	stats groupcache.GroupStats
}

func (g *groupWithStats) Set(context.Context, string, []byte, time.Time, bool) error { return nil }
func (g *groupWithStats) Get(context.Context, string, transport.Sink) error          { return nil }
func (g *groupWithStats) Remove(context.Context, string) error                       { return nil }
func (g *groupWithStats) UsedBytes() (int64, int64)                                  { return 0, 0 }
func (g *groupWithStats) Name() string                                               { return "ks" }
func (g *groupWithStats) RemoveKeys(context.Context, ...string) error                { return nil }
func (g *groupWithStats) GroupStats() groupcache.GroupStats                          { return g.stats }

type transportOnlyGroup struct{}

func (transportOnlyGroup) Set(context.Context, string, []byte, time.Time, bool) error { return nil }
func (transportOnlyGroup) Get(context.Context, string, transport.Sink) error          { return nil }
func (transportOnlyGroup) Remove(context.Context, string) error                       { return nil }
func (transportOnlyGroup) UsedBytes() (int64, int64)                                  { return 0, 0 }
func (transportOnlyGroup) Name() string                                               { return "ks" }
func (transportOnlyGroup) RemoveKeys(context.Context, ...string) error                { return nil }

type countingLogger struct {
	log.Logger
	count atomic.Int64
}

func (l *countingLogger) Errorf(format string, args ...any) {
	l.count.Add(1)
	if l.Logger != nil {
		l.Logger.Errorf(format, args...)
	}
}

func (l *countingLogger) errorfCalls() int64 {
	return l.count.Load()
}

func TestHandlerRootBasePath(t *testing.T) {
	mux := http.NewServeMux()
	newHandler("/", stubProvider{}, nil).register(mux)

	for _, path := range []string{"/healthz", "/peers", "/keyspaces"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, rec.Code, path)
	}
}
