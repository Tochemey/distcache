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
	"encoding/binary"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tochemey/distcache/discovery"
)

// testDomain is the fully qualified name served by the test DNS server
const testDomain = "peers.distcache.test."

const (
	dnsTypeA    uint16 = 1
	dnsTypeAAAA uint16 = 28
)

// testDNSServer is a minimal UDP DNS responder answering A and AAAA queries
// for any name with the configured addresses. It keeps the tests hermetic.
type testDNSServer struct {
	conn *net.UDPConn
	v4   []net.IP
	v6   []net.IP
	// when silent is true, queries are swallowed to simulate a stalled server
	silent bool
}

func startDNSServer(t *testing.T, v4, v6 []string, silent bool) *testDNSServer {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	require.NoError(t, err)

	srv := &testDNSServer{conn: conn, silent: silent}
	for _, ip := range v4 {
		srv.v4 = append(srv.v4, net.ParseIP(ip).To4())
	}
	for _, ip := range v6 {
		srv.v6 = append(srv.v6, net.ParseIP(ip).To16())
	}

	go srv.serve()
	t.Cleanup(func() { _ = conn.Close() })
	return srv
}

func (s *testDNSServer) serve() {
	buf := make([]byte, 4096)
	for {
		n, remote, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		if s.silent {
			continue
		}
		if response := s.buildResponse(buf[:n]); response != nil {
			_, _ = s.conn.WriteToUDP(response, remote)
		}
	}
}

// buildResponse crafts a NOERROR answer echoing the question section
func (s *testDNSServer) buildResponse(query []byte) []byte {
	if len(query) < 12 {
		return nil
	}

	// walk the question name
	off := 12
	for off < len(query) {
		labelLen := int(query[off])
		off++
		if labelLen == 0 {
			break
		}
		off += labelLen
	}
	if off+4 > len(query) {
		return nil
	}
	qtype := binary.BigEndian.Uint16(query[off:])
	off += 4
	question := query[12:off]

	var answers [][]byte
	appendRR := func(rtype uint16, rdata []byte) {
		rr := make([]byte, 0, 12+len(rdata))
		rr = append(rr, 0xC0, 0x0C) // pointer to the question name
		rr = binary.BigEndian.AppendUint16(rr, rtype)
		rr = binary.BigEndian.AppendUint16(rr, 1) // IN class
		rr = binary.BigEndian.AppendUint32(rr, 60)
		rr = binary.BigEndian.AppendUint16(rr, uint16(len(rdata)))
		rr = append(rr, rdata...)
		answers = append(answers, rr)
	}

	switch qtype {
	case dnsTypeA:
		for _, ip := range s.v4 {
			appendRR(dnsTypeA, ip)
		}
	case dnsTypeAAAA:
		for _, ip := range s.v6 {
			appendRR(dnsTypeAAAA, ip)
		}
	}

	response := make([]byte, 0, 12+len(question)+len(answers)*28)
	response = append(response, query[0], query[1])            // transaction id
	response = binary.BigEndian.AppendUint16(response, 0x8180) // QR, RD, RA, NOERROR
	response = binary.BigEndian.AppendUint16(response, 1)
	response = binary.BigEndian.AppendUint16(response, uint16(len(answers)))
	response = binary.BigEndian.AppendUint16(response, 0)
	response = binary.BigEndian.AppendUint16(response, 0)
	response = append(response, question...)
	for _, rr := range answers {
		response = append(response, rr...)
	}
	return response
}

// resolver returns a Go resolver that always talks to the test server
func (s *testDNSServer) resolver() *net.Resolver {
	addr := s.conn.LocalAddr().String()
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "udp", addr)
		},
	}
}

func newTestProvider(t *testing.T, config *Config, srv *testDNSServer) *Discovery {
	t.Helper()
	provider := NewDiscovery(config)
	provider.resolver = srv.resolver()
	return provider
}

func TestDiscovery(t *testing.T) {
	t.Run("With a new instance", func(t *testing.T) {
		// create the instance of provider
		provider := NewDiscovery(nil)
		require.NotNil(t, provider)
		// assert that provider implements the Discovery interface
		// this is a cheap test
		// assert the type of svc
		assert.IsType(t, &Discovery{}, provider)
		var p interface{} = provider
		_, ok := p.(discovery.Provider)
		assert.True(t, ok)
	})
	t.Run("With ID assertion", func(t *testing.T) {
		// cheap test
		// create the instance of provider
		provider := NewDiscovery(nil)
		require.NotNil(t, provider)
		assert.Equal(t, "dns-sd", provider.ID())
	})
	t.Run("With Initialize", func(t *testing.T) {
		// create the config
		config := &Config{
			DomainName: testDomain,
		}
		// create the instance of provider
		provider := NewDiscovery(config)
		assert.NoError(t, provider.Initialize())
	})
	t.Run("With Initialize: already initialized", func(t *testing.T) {
		// create the config
		config := &Config{
			DomainName: testDomain,
		}
		// create the instance of provider
		provider := NewDiscovery(config)
		provider.initialized.Store(true)
		assert.Error(t, provider.Initialize())
	})
	t.Run("With Register", func(t *testing.T) {
		// create the config
		config := &Config{
			DomainName: testDomain,
		}
		// create the instance of provider
		provider := NewDiscovery(config)
		require.NoError(t, provider.Initialize())
		require.NoError(t, provider.Register())
		require.True(t, provider.initialized.Load())
		require.NoError(t, provider.Deregister())
		assert.False(t, provider.initialized.Load())
	})
	t.Run("With Register when already registered", func(t *testing.T) {
		// create the config
		config := &Config{
			DomainName: testDomain,
		}
		// create the instance of provider
		provider := NewDiscovery(config)
		require.NoError(t, provider.Initialize())
		require.NoError(t, provider.Register())
		require.True(t, provider.initialized.Load())
		err := provider.Register()
		require.Error(t, err)
		require.EqualError(t, err, discovery.ErrAlreadyRegistered.Error())
		require.NoError(t, provider.Deregister())
		assert.False(t, provider.initialized.Load())
	})
	t.Run("With Deregister", func(t *testing.T) {
		// create the config
		config := &Config{
			DomainName: testDomain,
		}
		// create the instance of provider
		provider := NewDiscovery(config)
		// for the sake of the test
		provider.initialized.Store(true)
		assert.NoError(t, provider.Deregister())
	})
	t.Run("With Deregister when not initialized", func(t *testing.T) {
		// create the config
		config := &Config{
			DomainName: testDomain,
		}
		// create the instance of provider
		provider := NewDiscovery(config)
		// for the sake of the test
		provider.initialized.Store(false)
		err := provider.Deregister()
		assert.Error(t, err)
		assert.EqualError(t, err, discovery.ErrNotInitialized.Error())
	})
	t.Run("With DiscoverPeers: not initialized", func(t *testing.T) {
		// create the config
		config := &Config{
			DomainName: testDomain,
		}
		// create the instance of provider
		provider := NewDiscovery(config)
		peers, err := provider.DiscoverPeers()
		assert.Error(t, err)
		assert.Empty(t, peers)
		assert.EqualError(t, err, discovery.ErrNotInitialized.Error())
	})
	t.Run("With DiscoverPeers", func(t *testing.T) {
		srv := startDNSServer(t, []string{"10.0.0.1", "10.0.0.2"}, []string{"fd00::1"}, false)
		config := &Config{
			DomainName: testDomain,
		}
		provider := newTestProvider(t, config, srv)

		require.NoError(t, provider.Initialize())
		require.NoError(t, provider.Register())
		require.True(t, provider.initialized.Load())

		// discover peers: both A and AAAA records are returned as bare IPs
		peers, err := provider.DiscoverPeers()
		require.NoError(t, err)
		require.NotEmpty(t, peers)
		require.NoError(t, validateAddrs(peers))
		assert.ElementsMatch(t, []string{"10.0.0.1", "10.0.0.2", "fd00::1"}, peers)
		assert.NoError(t, provider.Deregister())
		assert.NoError(t, provider.Close())
	})
	t.Run("With DiscoverPeers with IPV6", func(t *testing.T) {
		srv := startDNSServer(t, []string{"10.0.0.1"}, []string{"fd00::1", "fd00::2"}, false)
		ipv6 := true
		config := &Config{
			DomainName: testDomain,
			IPv6:       &ipv6,
		}
		provider := newTestProvider(t, config, srv)

		require.NoError(t, provider.Initialize())
		require.NoError(t, provider.Register())
		require.True(t, provider.initialized.Load())

		// discover peers: only AAAA records are returned
		peers, err := provider.DiscoverPeers()
		require.NoError(t, err)
		require.NotEmpty(t, peers)
		require.NoError(t, validateAddrs(peers))
		assert.ElementsMatch(t, []string{"fd00::1", "fd00::2"}, peers)
		assert.NoError(t, provider.Deregister())
		assert.NoError(t, provider.Close())
	})
	t.Run("With DiscoverPeers with Port", func(t *testing.T) {
		srv := startDNSServer(t, []string{"10.0.0.1", "10.0.0.2"}, []string{"fd00::1"}, false)
		config := &Config{
			DomainName: testDomain,
			Port:       3320,
		}
		provider := newTestProvider(t, config, srv)

		require.NoError(t, provider.Initialize())
		require.NoError(t, provider.Register())

		// discover peers: addresses are returned as host:port
		peers, err := provider.DiscoverPeers()
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"10.0.0.1:3320", "10.0.0.2:3320", "[fd00::1]:3320"}, peers)
		for _, peer := range peers {
			_, port, err := net.SplitHostPort(peer)
			require.NoError(t, err)
			assert.Equal(t, "3320", port)
		}
		assert.NoError(t, provider.Deregister())
		assert.NoError(t, provider.Close())
	})
	t.Run("With DiscoverPeers timeout", func(t *testing.T) {
		// the server never answers: the lookup must give up after the configured timeout
		srv := startDNSServer(t, []string{"10.0.0.1"}, nil, true)
		config := &Config{
			DomainName: testDomain,
			Timeout:    200 * time.Millisecond,
		}
		provider := newTestProvider(t, config, srv)

		require.NoError(t, provider.Initialize())
		require.NoError(t, provider.Register())

		start := time.Now()
		peers, err := provider.DiscoverPeers()
		require.Error(t, err)
		assert.Empty(t, peers)
		assert.Less(t, time.Since(start), 5*time.Second)
		assert.NoError(t, provider.Deregister())
	})
}

func validateAddrs(addrs []string) error {
	for _, addr := range addrs {
		ipaddr := net.ParseIP(addr)
		if ipaddr == nil {
			return fmt.Errorf("invalid IP address format: %s", addr)
		}
	}
	return nil
}
