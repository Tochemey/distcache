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

// Package standalone provides a no-op discovery provider that runs the cache
// engine as a single, self-contained node without joining any cluster.
//
// Use this provider during local development, testing, or any deployment where
// peer discovery is not needed. Because [Discovery.DiscoverPeers] always
// returns an empty list, the engine skips the cluster-join step and operates
// with only itself as a peer.
//
// Example:
//
//	provider := standalone.NewDiscovery()
//	config   := distcache.NewConfig(provider, keySpaces)
//	engine, _ := distcache.NewEngine(config)
//	engine.Start(ctx)
package standalone

import "github.com/tochemey/distcache/discovery"

// Discovery implements [discovery.Provider] as a no-op.
//
// Every lifecycle method succeeds immediately and [DiscoverPeers] returns an
// empty peer list, which causes the engine to run as a standalone node.
type Discovery struct{}

// enforce compilation error
var _ discovery.Provider = (*Discovery)(nil)

// NewDiscovery creates a standalone discovery provider.
func NewDiscovery() *Discovery {
	return &Discovery{}
}

// ID returns the discovery provider identifier.
func (d *Discovery) ID() string {
	return "standalone"
}

// Initialize is a no-op for the standalone provider.
func (d *Discovery) Initialize() error {
	return nil
}

// Register is a no-op for the standalone provider.
func (d *Discovery) Register() error {
	return nil
}

// Deregister is a no-op for the standalone provider.
func (d *Discovery) Deregister() error {
	return nil
}

// DiscoverPeers always returns an empty list and no error, signalling to the
// engine that there are no peers to join.
func (d *Discovery) DiscoverPeers() ([]string, error) {
	return nil, nil
}

// Close is a no-op for the standalone provider.
func (d *Discovery) Close() error {
	return nil
}
