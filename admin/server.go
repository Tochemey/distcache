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

	"github.com/tochemey/distcache/log"
)

const defaultAdminBasePath = "/_distcache/admin"

// SnapshotProvider supplies data snapshots for admin endpoints.
type SnapshotProvider interface {
	// SnapshotPeers returns the current peer view or an error.
	SnapshotPeers() (any, error)
	// SnapshotKeySpaces returns the current keyspace snapshots.
	SnapshotKeySpaces() any
	// Ready reports whether the engine has finished bootstrapping and is
	// serving traffic. Used by the /readyz endpoint.
	Ready() bool
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
//
// Start returns once the listener is bound; requests are served on a
// background goroutine until Shutdown is called. A nil error means the socket
// is accepting connections.
func (s *Server) Start(ctx context.Context) error {
	if s.cfg.ListenAddr == "" {
		return nil
	}

	if s.provider == nil {
		return errors.New("admin snapshot provider is required")
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	mux := http.NewServeMux()
	handler := newHandler(s.cfg.BasePath, s.provider, s.logger)
	handler.register(mux)

	var root http.Handler = mux
	if s.cfg.Middleware != nil {
		root = s.cfg.Middleware(mux)
	}

	s.server = &http.Server{
		Handler:      root,
		ReadTimeout:  s.cfg.ReadTimeout,
		WriteTimeout: s.cfg.WriteTimeout,
		IdleTimeout:  s.cfg.IdleTimeout,
	}
	if s.logger != nil {
		// route net/http's internal diagnostics (TLS handshake failures, bad
		// requests) through the configured logger instead of the stdlib one
		s.server.ErrorLog = s.logger.StdLogger()
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

	return nil
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
	logger   log.Logger
}

func newHandler(basePath string, provider SnapshotProvider, logger log.Logger) *handler {
	prefix := "/" + strings.Trim(basePath, "/")
	if prefix == "/" {
		// a bare "/" base path would otherwise register "//peers"
		prefix = ""
	}
	return &handler{
		basePath: prefix,
		provider: provider,
		logger:   logger,
	}
}

// register installs the endpoints on the mux. The method-qualified patterns
// make the mux answer 405 for other methods; "GET" patterns also match HEAD,
// which load balancers commonly use for health probes.
func (h *handler) register(mux *http.ServeMux) {
	mux.HandleFunc("GET "+h.basePath+"/peers", h.handlePeers)
	mux.HandleFunc("GET "+h.basePath+"/keyspaces", h.handleKeySpaces)
	mux.HandleFunc("GET "+h.basePath+"/healthz", h.handleHealthz)
	mux.HandleFunc("GET "+h.basePath+"/readyz", h.handleReadyz)
}

func (h *handler) handlePeers(w http.ResponseWriter, _ *http.Request) {
	peers, err := h.provider.SnapshotPeers()
	if err != nil {
		if h.logger != nil {
			h.logger.Errorf("admin peers snapshot failed: %v", err)
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, peers)
}

func (h *handler) handleKeySpaces(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, h.provider.SnapshotKeySpaces())
}

func (h *handler) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (h *handler) handleReadyz(w http.ResponseWriter, _ *http.Request) {
	if !h.provider.Ready() {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready"))
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(value)
}
