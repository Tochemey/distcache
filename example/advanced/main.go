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

// Standalone example demonstrating optional features:
//   - Negative caching with ErrNotFound
//   - Periodic hot-key refresh
//   - Cluster event subscription
//   - Memberlist gossip authentication
//   - Admin server with /healthz and /readyz
//
// Run with: go run ./example/advanced
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/travisjeffery/go-dynaport"

	"github.com/tochemey/distcache"
	"github.com/tochemey/distcache/admin"
	"github.com/tochemey/distcache/internal/size"
	"github.com/tochemey/distcache/log"
	"github.com/tochemey/distcache/warmup"
)

func main() {
	ctx := context.Background()
	logger := log.DefaultLogger

	ports := dynaport.Get(3)
	bindPort, discoveryPort, adminPort := ports[0], ports[1], ports[2]

	source := newProductsDataSource()
	source.upsert("p1", "Widget")

	keyspace := newProductsKeySpace("products", 10*size.MB, source)

	// 32-byte AES-256 key. In production load this from a secret manager.
	gossipSecret := []byte("0123456789abcdef0123456789abcdef")

	cfg := distcache.NewStandaloneConfig(
		[]distcache.KeySpace{keyspace},
		distcache.WithLogger(logger),
		distcache.WithBindAddr("127.0.0.1"),
		distcache.WithBindPort(bindPort),
		distcache.WithDiscoveryPort(discoveryPort),

		// Cache "not found" results for 30 seconds to protect the backend
		// from repeated lookups of nonexistent keys. Requires the DataSource
		// to return distcache.ErrNotFound for missing keys.
		distcache.WithKeySpaceNegativeTTL("products", 30*time.Second),

		// Periodically re-fetch tracked hot keys from the source so they
		// stay fresh ahead of TTL expiry.
		distcache.WithWarmup(warmup.Config{
			MaxHotKeys:      100,
			MinHits:         3,
			Concurrency:     4,
			Timeout:         2 * time.Second,
			RefreshInterval: 30 * time.Second,
		}),

		// Authenticate cluster gossip with a shared secret. Every peer must
		// configure the same key.
		distcache.WithGossipSecret(gossipSecret),

		// Expose /healthz, /readyz, /peers, /keyspaces.
		distcache.WithAdminConfig(admin.Config{
			ListenAddr: fmt.Sprintf("127.0.0.1:%d", adminPort),
		}),
	)

	engine, err := distcache.NewEngine(cfg)
	if err != nil {
		logger.Fatal(err)
	}

	// Subscribe to cluster events BEFORE starting the engine so no event is
	// missed. Subscribers must drain the channel; events are dropped if the
	// buffer fills.
	events := engine.Events()
	go func() {
		for ev := range events {
			logger.Infof("cluster event: type=%s peer=%s at=%s",
				ev.Type, ev.Peer.Address(), ev.At.Format(time.RFC3339))
		}
	}()

	if err := engine.Start(ctx); err != nil {
		logger.Fatal(err)
	}
	logger.Infof("admin endpoints listening on http://127.0.0.1:%d/_distcache/admin/", adminPort)

	// Demonstrate negative caching: the source has no "missing" key, so the
	// first Get triggers Fetch -> ErrNotFound -> tombstone is cached.
	if _, err := engine.Get(ctx, "products", "missing"); errors.Is(err, distcache.ErrNotFound) {
		logger.Infof("first lookup of 'missing' returned ErrNotFound (tombstone cached)")
	}
	// Second lookup returns ErrNotFound from the tombstone without calling Fetch.
	if _, err := engine.Get(ctx, "products", "missing"); errors.Is(err, distcache.ErrNotFound) {
		logger.Infof("second lookup of 'missing' served from negative cache")
	}

	if kv, err := engine.Get(ctx, "products", "p1"); err == nil {
		logger.Infof("lookup of 'p1' returned %q", string(kv.Value))
	}

	notifier := make(chan os.Signal, 1)
	signal.Notify(notifier, syscall.SIGINT, syscall.SIGTERM)
	logger.Infof("ready (Ctrl-C to exit)")
	<-notifier
	signal.Stop(notifier)

	if err := engine.Stop(ctx); err != nil {
		logger.Errorf("shutdown: %v", err)
	}
}

type productsKeySpace struct {
	mu         sync.RWMutex
	name       string
	maxBytes   int64
	dataSource distcache.DataSource
}

func newProductsKeySpace(name string, maxBytes int64, source distcache.DataSource) *productsKeySpace {
	return &productsKeySpace{name: name, maxBytes: maxBytes, dataSource: source}
}

func (k *productsKeySpace) Name() string                                { return k.name }
func (k *productsKeySpace) MaxBytes() int64                             { return k.maxBytes }
func (k *productsKeySpace) DataSource() distcache.DataSource            { return k.dataSource }
func (k *productsKeySpace) ExpiresAt(context.Context, string) time.Time { return time.Time{} }

type productsDataSource struct {
	mu    sync.RWMutex
	items map[string]string
}

func newProductsDataSource() *productsDataSource {
	return &productsDataSource{items: make(map[string]string)}
}

func (d *productsDataSource) upsert(key, value string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.items[key] = value
}

// Fetch returns distcache.ErrNotFound for missing keys so the engine can
// cache the absence (negative caching). Any other error is treated as a
// transient failure and is not cached.
func (d *productsDataSource) Fetch(_ context.Context, key string) ([]byte, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	v, ok := d.items[key]
	if !ok {
		return nil, distcache.ErrNotFound
	}
	return []byte(v), nil
}
