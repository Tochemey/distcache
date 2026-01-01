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

// Package admin exposes diagnostic HTTP endpoints for distcache.
package admin

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/tochemey/distcache/log"
)

const defaultAdminBasePath = "/_distcache/admin"

// SnapshotProvider supplies data snapshots for admin endpoints.
type SnapshotProvider interface {
	// SnapshotPeers returns the current peer view or an error.
	SnapshotPeers() (any, error)
	// SnapshotKeySpaces returns the current keyspace snapshots.
	SnapshotKeySpaces() any
}

// Server hosts the admin HTTP endpoints.
type Server struct {
	cfg      Config
	provider SnapshotProvider
	logger   log.Logger
	server   *http.Server
	listener net.Listener
}

// NewServer constructs an admin server with the given configuration.
func NewServer(cfg Config, provider SnapshotProvider, logger log.Logger) *Server {
	return &Server{
		cfg:      cfg.Normalize(),
		provider: provider,
		logger:   logger,
	}
}

// Start begins serving the admin endpoints.
func (s *Server) Start(ctx context.Context) error {
	if s.cfg.ListenAddr == "" {
		return nil
	}

	if s.provider == nil {
		return errors.New("admin snapshot provider is required")
	}

	mux := http.NewServeMux()
	handler := newHandler(s.cfg.BasePath, s.provider)
	handler.register(mux)

	s.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  s.cfg.ReadTimeout,
		WriteTimeout: s.cfg.WriteTimeout,
		IdleTimeout:  s.cfg.IdleTimeout,
	}

	listener, err := net.Listen("tcp", s.cfg.ListenAddr)
	if err != nil {
		return err
	}

	if s.cfg.TLSConfig != nil {
		listener = tls.NewListener(listener, s.cfg.TLSConfig)
	}

	s.listener = listener
	go func() {
		if err := s.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			if s.logger != nil {
				s.logger.Errorf("admin server error: %v", err)
			}
		}
	}()

	return waitForAdminConnect(ctx, listener.Addr().String())
}

// Shutdown stops the admin HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}

type handler struct {
	basePath string
	provider SnapshotProvider
}

func newHandler(basePath string, provider SnapshotProvider) *handler {
	return &handler{
		basePath: "/" + strings.Trim(basePath, "/"),
		provider: provider,
	}
}

func (h *handler) register(mux *http.ServeMux) {
	mux.HandleFunc(h.basePath+"/peers", h.handlePeers)
	mux.HandleFunc(h.basePath+"/keyspaces", h.handleKeySpaces)
}

func (h *handler) handlePeers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	peers, err := h.provider.SnapshotPeers()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, peers)
}

func (h *handler) handleKeySpaces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, h.provider.SnapshotKeySpaces())
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(value)
}

func waitForAdminConnect(ctx context.Context, address string) error {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(5 * time.Second)
	}

	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}

	return context.DeadlineExceeded
}
