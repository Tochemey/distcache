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

package dnssd

import (
	"context"
	"net"
	"strconv"
	"sync"

	goset "github.com/deckarep/golang-set/v3"
	"go.uber.org/atomic"

	"github.com/tochemey/distcache/discovery"
)

const (
	// DomainName is kept for backward compatibility.
	//
	// Deprecated: it is not used by the provider and will be removed in a future release.
	DomainName = "domain-name"
	// IPv6 is kept for backward compatibility.
	//
	// Deprecated: it is not used by the provider and will be removed in a future release.
	IPv6 = "ipv6"
)

// Discovery represents the DNS service discovery
// IP addresses are looked up by querying the default
// DNS resolver for A and AAAA records associated with the DNS name.
type Discovery struct {
	mu     sync.Mutex
	config *Config

	// states whether the actor system has started or not.
	// The pointer is set once at construction time and must never be replaced
	// afterwards: use Load/Store so that readers stay race-free.
	initialized *atomic.Bool

	// resolver is the DNS resolver used to look up the domain name.
	// When nil, a Go-based resolver is used. It is overridable for testing purposes.
	resolver *net.Resolver
}

// enforce compilation error
var _ discovery.Provider = &Discovery{}

// NewDiscovery returns an instance of the DNS discovery provider
func NewDiscovery(config *Config) *Discovery {
	return &Discovery{
		mu:          sync.Mutex{},
		config:      config,
		initialized: atomic.NewBool(false),
	}
}

// ID returns the discovery provider id
func (d *Discovery) ID() string {
	return "dns-sd"
}

// Initialize initializes the plugin: registers some internal data structures, clients etc.
func (d *Discovery) Initialize() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.initialized.Load() {
		return discovery.ErrAlreadyInitialized
	}

	return d.config.Validate()
}

// Register registers this node to a service discovery directory.
func (d *Discovery) Register() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.initialized.Load() {
		return discovery.ErrAlreadyRegistered
	}

	d.initialized.Store(true)
	return nil
}

// Deregister removes this node from a service discovery directory.
func (d *Discovery) Deregister() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.initialized.Load() {
		return discovery.ErrNotInitialized
	}
	d.initialized.Store(false)
	return nil
}

// DiscoverPeers returns a list of known nodes.
func (d *Discovery) DiscoverPeers() ([]string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.initialized.Load() {
		return nil, discovery.ErrNotInitialized
	}

	ctx, cancel := context.WithTimeout(context.Background(), d.config.timeout())
	defer cancel()

	// set ipv6 filter
	network := "ip"
	if d.config.IPv6 != nil && *d.config.IPv6 {
		network = "ip6"
	}

	resolver := d.resolver
	if resolver == nil {
		resolver = &net.Resolver{
			PreferGo: true, // Prefer Go's DNS resolver
		}
	}

	// lookup the addresses based upon the dns name
	ips, err := resolver.LookupIP(ctx, network, d.config.DomainName)
	if err != nil {
		return nil, err
	}

	addresses := goset.NewSet[string]()
	for _, ip := range ips {
		if d.config.Port > 0 {
			addresses.Add(net.JoinHostPort(ip.String(), strconv.Itoa(d.config.Port)))
			continue
		}
		addresses.Add(ip.String())
	}

	return addresses.ToSlice(), nil
}

// Close closes the provider
func (d *Discovery) Close() error {
	return nil
}
