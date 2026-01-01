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

package distcache

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/groupcache/groupcache-go/v3/transport"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/tochemey/distcache/internal/size"
	mockDiscovery "github.com/tochemey/distcache/mocks/discovery"
)

type MockKeySpace struct {
	*sync.RWMutex
	name       string
	maxBytes   int64
	dataSource DataSource
}

var _ KeySpace = (*MockKeySpace)(nil)

func NewMockKeySpace(name string, maxBytes int64, source DataSource) *MockKeySpace {
	return &MockKeySpace{
		RWMutex:    &sync.RWMutex{},
		name:       name,
		maxBytes:   maxBytes,
		dataSource: source,
	}
}

func (x *MockKeySpace) Name() string {
	x.RLock()
	defer x.RUnlock()
	return x.name
}

func (x *MockKeySpace) MaxBytes() int64 {
	x.RLock()
	defer x.RUnlock()
	return x.maxBytes
}

func (x *MockKeySpace) DataSource() DataSource {
	x.RLock()
	defer x.RUnlock()
	return x.dataSource
}

func (x *MockKeySpace) ExpiresAt(ctx context.Context, key string) time.Time {
	x.Lock()
	defer x.Unlock()

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if _, err := x.dataSource.Fetch(ctx, key); err != nil {
		return time.Now().Add(time.Second)
	}

	return time.Time{}
}

type staticExpiryKeySpace struct {
	name       string
	maxBytes   int64
	dataSource DataSource
	expiry     time.Time
}

func (s *staticExpiryKeySpace) Name() string                                { return s.name }
func (s *staticExpiryKeySpace) MaxBytes() int64                             { return s.maxBytes }
func (s *staticExpiryKeySpace) DataSource() DataSource                      { return s.dataSource }
func (s *staticExpiryKeySpace) ExpiresAt(context.Context, string) time.Time { return s.expiry }

type staticDataSource struct {
	value []byte
}

func (s *staticDataSource) Fetch(context.Context, string) ([]byte, error) {
	return s.value, nil
}

type expirySink struct {
	expiry time.Time
	value  []byte
}

func (s *expirySink) SetString(v string, e time.Time) error {
	return s.SetBytes([]byte(v), e)
}

func (s *expirySink) SetBytes(v []byte, e time.Time) error {
	s.value = append([]byte(nil), v...)
	s.expiry = e
	return nil
}

func (s *expirySink) SetProto(proto.Message, time.Time) error { return nil }
func (s *expirySink) View() (transport.ByteView, error)       { return transport.ByteView{}, nil }

func TestKeySpaceConfigValidation(t *testing.T) {
	keyspace := NewMockKeySpace("users", size.MB, NewMockDataSource())
	config := NewConfig(
		mockDiscovery.NewProvider(t),
		[]KeySpace{keyspace},
		WithKeySpaceDefaultTTL("users", -1*time.Second),
	)
	err := config.Validate()
	require.Error(t, err)
}

func TestKeySpaceDefaultTTL(t *testing.T) {
	keyspace := NewMockKeySpace("ks", size.MB, NewMockDataSource())
	engine, _, _ := newMockEngine(t, mockDiscovery.NewProvider(t), keyspace)
	updateKeySpaceConfig(engine.config, "ks", func(cfg *KeySpaceConfig) {
		cfg.DefaultTTL = 2 * time.Second
	})
	spec, err := newKeySpaceWrapper(engine.config, keyspace)
	require.NoError(t, err)
	engine.keySpaces.Set("ks", spec)
	g := &MockGroup{name: "ks"}
	engine.groups.Set("ks", g)

	entry := &Entry{
		KV: KV{
			Key:   "a",
			Value: []byte("v"),
		},
	}

	err = engine.Put(context.Background(), "ks", entry)
	require.NoError(t, err)
	setCalls := g.SetCalls()
	require.Len(t, setCalls, 1)
	require.WithinDuration(t, time.Now().Add(2*time.Second), setCalls[0].expiry, 500*time.Millisecond)
}

func TestBuildKeySpaceSpecErrors(t *testing.T) {
	cfg := NewConfig(MockProvider{}, nil)

	testCases := []struct {
		name     string
		keyspace KeySpace
	}{
		{name: "nil keyspace", keyspace: nil},
		{name: "empty name", keyspace: NewMockKeySpace("", size.MB, NewMockDataSource())},
		{name: "nil datasource", keyspace: NewMockKeySpace("ks", size.MB, nil)},
		{name: "invalid max bytes", keyspace: NewMockKeySpace("ks", 0, NewMockDataSource())},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := newKeySpaceWrapper(cfg, tc.keyspace)
			require.Error(t, err)
		})
	}
}

func TestBuildKeySpaceSpecConfigAndProtector(t *testing.T) {
	source := &countingDataSource{}
	keyspace := NewMockKeySpace("ks", size.MB, source)
	warmKeys := []string{"alpha", "beta"}
	rateLimit := RateLimitConfig{
		RequestsPerSecond: 1,
		Burst:             1,
		WaitTimeout:       0,
	}

	cfg := NewConfig(
		MockProvider{},
		[]KeySpace{keyspace},
		WithRateLimiter(rateLimit),
		WithKeySpaceWarmKeys("ks", warmKeys),
	)
	spec, err := newKeySpaceWrapper(cfg, keyspace)
	require.NoError(t, err)
	require.Equal(t, int64(size.MB), spec.config.MaxBytes)

	warmKeys[0] = "mutated"
	require.Equal(t, "alpha", spec.config.WarmKeys[0])

	_, err = spec.dataSource.Fetch(context.Background(), "a")
	require.NoError(t, err)
	_, err = spec.dataSource.Fetch(context.Background(), "b")
	require.ErrorIs(t, err, ErrDataSourceRateLimited)
	require.Equal(t, 1, source.callCount())
}

func TestBuildKeySpaceSpecProtectorOverrideAndValidation(t *testing.T) {
	source := &countingDataSource{}
	keyspace := NewMockKeySpace("ks", size.MB, source)
	configRateLimit := RateLimitConfig{
		RequestsPerSecond: 1,
		Burst:             1,
		WaitTimeout:       0,
	}

	cfg := NewConfig(
		MockProvider{},
		[]KeySpace{keyspace},
		WithRateLimiter(configRateLimit),
		WithKeySpaceRateLimiter("ks", RateLimitConfig{
			RequestsPerSecond: 100,
			Burst:             2,
			WaitTimeout:       0,
		}),
	)
	spec, err := newKeySpaceWrapper(cfg, keyspace)
	require.NoError(t, err)
	require.NotEqual(t, source, spec.dataSource)

	_, err = spec.dataSource.Fetch(context.Background(), "a")
	require.NoError(t, err)
	_, err = spec.dataSource.Fetch(context.Background(), "b")
	require.NoError(t, err)
	require.Equal(t, 2, source.callCount())

	invalidCfg := NewConfig(
		MockProvider{},
		[]KeySpace{keyspace},
		WithKeySpaceRateLimiter("ks", RateLimitConfig{RequestsPerSecond: 0}),
	)
	_, err = newKeySpaceWrapper(invalidCfg, keyspace)
	require.Error(t, err)
}

func TestEngineTimeoutOverrides(t *testing.T) {
	engine, _, _ := newMockEngine(t, MockProvider{}, NewMockKeySpace("ks", size.MB, NewMockDataSource()))
	engine.config.readTimeout = 12 * time.Second
	engine.config.writeTimeout = 15 * time.Second

	spec := &keySpaceWrapper{
		config: KeySpaceConfig{
			ReadTimeout: 2 * time.Second,
		},
	}

	require.Equal(t, 2*time.Second, engine.readTimeout(spec))
	require.Equal(t, 15*time.Second, engine.writeTimeout(spec))
	require.Equal(t, 12*time.Second, engine.readTimeout(nil))
}

func TestCreateGroupDefaultTTL(t *testing.T) {
	engine, _, _ := newMockEngine(t, MockProvider{}, NewMockKeySpace("ks", size.MB, NewMockDataSource()))

	dataSource := &staticDataSource{value: []byte("value")}
	keyspace := &staticExpiryKeySpace{
		name:       "ks",
		maxBytes:   size.MB,
		dataSource: dataSource,
		expiry:     time.Time{},
	}
	spec := &keySpaceWrapper{
		keyspace: keyspace,
		config: KeySpaceConfig{
			MaxBytes:   size.MB,
			DefaultTTL: 2 * time.Second,
		},
		dataSource: dataSource,
	}

	group, err := engine.createGroup(spec)
	require.NoError(t, err)

	sink := &expirySink{}
	require.NoError(t, group.Get(context.Background(), "k", sink))
	require.WithinDuration(t, time.Now().Add(2*time.Second), sink.expiry, 500*time.Millisecond)
}

func TestCreateGroupUsesKeySpaceExpiry(t *testing.T) {
	engine, _, _ := newMockEngine(t, MockProvider{}, NewMockKeySpace("ks", size.MB, NewMockDataSource()))

	dataSource := &staticDataSource{value: []byte("value")}
	expiry := time.Now().Add(5 * time.Minute)
	keyspace := &staticExpiryKeySpace{
		name:       "ks",
		maxBytes:   size.MB,
		dataSource: dataSource,
		expiry:     expiry,
	}
	spec := &keySpaceWrapper{
		keyspace: keyspace,
		config: KeySpaceConfig{
			MaxBytes:   size.MB,
			DefaultTTL: 2 * time.Second,
		},
		dataSource: dataSource,
	}

	group, err := engine.createGroup(spec)
	require.NoError(t, err)

	sink := &expirySink{}
	require.NoError(t, group.Get(context.Background(), "k", sink))
	require.WithinDuration(t, expiry, sink.expiry, 500*time.Millisecond)
}

func TestEnsureKeySpacesDetectsDuplicate(t *testing.T) {
	engine, _, _ := newMockEngine(t, MockProvider{}, NewMockKeySpace("ks", size.MB, NewMockDataSource()))
	engine.keySpaces = nil
	engine.config.keySpaces = []KeySpace{
		NewMockKeySpace("dup", size.MB, NewMockDataSource()),
		NewMockKeySpace("dup", size.MB, NewMockDataSource()),
	}

	err := engine.ensureKeySpaces()
	require.Error(t, err)
}

func TestReplaceAndRemoveKeySpaceInConfig(t *testing.T) {
	engine, _, _ := newMockEngine(t, MockProvider{}, NewMockKeySpace("one", size.MB, NewMockDataSource()), NewMockKeySpace("two", size.MB, NewMockDataSource()))

	replacement := NewMockKeySpace("one", 2*size.MB, NewMockDataSource())
	engine.replaceKeySpaceInConfig(replacement)
	require.Equal(t, replacement, engine.config.keySpaces[0])

	engine.removeKeySpaceFromConfig("two")
	require.Len(t, engine.config.keySpaces, 1)
	require.Equal(t, "one", engine.config.keySpaces[0].Name())
}
