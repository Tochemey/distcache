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

package members

import (
	"crypto/md5" // nolint
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/memberlist"
	"github.com/kapetan-io/tackle/autotls"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/travisjeffery/go-dynaport"

	util "github.com/tochemey/distcache/internal/pause"
	dlog "github.com/tochemey/distcache/log"
)

// nolint
func TestTCPTransport(t *testing.T) {
	t.Run("NewTCPTransport with empty config", func(t *testing.T) {
		transport, err := NewTransport(TransportConfig{})
		require.NoError(t, err)
		require.NotNil(t, transport)
		assert.NoError(t, transport.Shutdown())
	})
	t.Run("NewTCPTransport failed with invalid address", func(t *testing.T) {
		transport, err := NewTransport(TransportConfig{
			BindAddrs: []string{"127.0.0.0.0.1"},
		})
		require.Error(t, err)
		require.Nil(t, transport)
	})
	t.Run("NewTCPTransport already listening on port", func(t *testing.T) {
		host := "127.0.0.1"
		ports := dynaport.Get(1)
		transport, err := NewTransport(TransportConfig{
			BindAddrs: []string{host},
			BindPort:  ports[0],
		})
		require.NoError(t, err)
		require.NotNil(t, transport)

		transport2, err := NewTransport(TransportConfig{
			BindAddrs: []string{host},
			BindPort:  ports[0],
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to start TCP listener")
		require.Nil(t, transport2)
		assert.NoError(t, transport.Shutdown())
	})
	t.Run("NewTCPTransport with TLS", func(t *testing.T) {
		host := "127.0.0.1"
		ports := dynaport.Get(1)
		// AutoGenerate TLS certs
		conf := autotls.Config{AutoTLS: true}
		require.NoError(t, autotls.Setup(&conf))
		transport, err := NewTransport(TransportConfig{
			BindAddrs:  []string{host},
			BindPort:   ports[0],
			TLSEnabled: true,
			TLS:        conf.ServerTLS,
		})
		require.NoError(t, err)
		require.NotNil(t, transport)
		assert.NoError(t, transport.Shutdown())
	})
	t.Run("With GetAutoBindPort", func(t *testing.T) {
		host := "127.0.0.1"
		transport, err := NewTransport(TransportConfig{
			BindAddrs: []string{host},
			BindPort:  0,
		})
		require.NoError(t, err)
		require.NotNil(t, transport)

		port := transport.GetAutoBindPort()
		assert.NotZero(t, port)
		assert.NoError(t, transport.Shutdown())
	})
	t.Run("With FinalAdvertiseAddr", func(t *testing.T) {
		host := "127.0.0.1"
		transport, err := NewTransport(TransportConfig{
			BindAddrs: []string{host},
			BindPort:  0,
		})
		require.NoError(t, err)
		require.NotNil(t, transport)
		ports := dynaport.Get(1)

		ip, port, err := transport.FinalAdvertiseAddr(host, ports[0])
		require.NoError(t, err)
		require.NotNil(t, ip)
		assert.Equal(t, ports[0], port)
		assert.NoError(t, transport.Shutdown())
	})
	t.Run("With FinalAdvertiseAddr with empty address", func(t *testing.T) {
		host := "127.0.0.1"
		transport, err := NewTransport(TransportConfig{
			BindAddrs: []string{host},
			BindPort:  0,
		})
		require.NoError(t, err)
		require.NotNil(t, transport)
		ports := dynaport.Get(1)

		ip, port, err := transport.FinalAdvertiseAddr("", ports[0])
		require.NoError(t, err)
		require.NotNil(t, ip)
		assert.NotEqual(t, ports[0], port)
		assert.NoError(t, transport.Shutdown())
	})
	t.Run("With FinalAdvertiseAddr with zeroZeroZeroZero", func(t *testing.T) {
		host := zeroZeroZeroZero
		transport, err := NewTransport(TransportConfig{
			BindAddrs: []string{host},
			BindPort:  0,
		})
		require.NoError(t, err)
		require.NotNil(t, transport)
		ports := dynaport.Get(1)

		ip, port, err := transport.FinalAdvertiseAddr("", ports[0])
		require.NoError(t, err)
		require.NotNil(t, ip)
		assert.NotEqual(t, ports[0], port)
		assert.NoError(t, transport.Shutdown())
	})
	t.Run("With FinalAdvertiseAddr with invalid address", func(t *testing.T) {
		host := "127.0.0.1"
		transport, err := NewTransport(TransportConfig{
			BindAddrs: []string{host},
			BindPort:  0,
		})
		require.NoError(t, err)
		require.NotNil(t, transport)
		ports := dynaport.Get(1)

		ip, port, err := transport.FinalAdvertiseAddr("127.0.0.0.0.1-invalid", ports[0])
		require.Error(t, err)
		require.Nil(t, ip)
		assert.Zero(t, port)
		assert.NoError(t, transport.Shutdown())
	})
	t.Run("SendBestEffort", func(t *testing.T) {
		cluster, cleanup := makeCluster(t)
		defer cleanup()

		msg := "message"
		err := cluster.members[0].SendBestEffort(cluster.members[1].Members()[1], []byte(msg))
		require.NoError(t, err)

		util.Pause(time.Second)
		require.Len(t, cluster.delegates[1].Messages(), 1)
	})
	t.Run("SendReliable", func(t *testing.T) {
		cluster, cleanup := makeCluster(t)
		defer cleanup()

		msg := "message"
		err := cluster.members[0].SendReliable(cluster.members[1].Members()[1], []byte(msg))
		require.NoError(t, err)

		util.Pause(time.Second)
		require.Len(t, cluster.delegates[1].Messages(), 1)
	})
}

// nolint
func makeCluster(t *testing.T) (*mockCluster, func()) {
	cluster := &mockCluster{
		members:   [2]*memberlist.Memberlist{},
		delegates: [2]*delegate{},
	}

	for i := range 2 {
		cluster.delegates[i] = &delegate{}
		cluster.members[i] = newNode(t, strconv.Itoa(i), cluster.delegates[i])
	}

	for i := range 2 {
		_, err := cluster.members[i].Join([]string{cluster.members[(i+1)%2].LocalNode().Address()})
		require.NoError(t, err)
	}

	util.Pause(time.Second)

	for i := range 2 {
		require.Len(t, cluster.members[i].Members(), 2)
	}

	cleanupFunc := func() {
		for i := range 2 {
			err := cluster.members[i].Shutdown()
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	return cluster, cleanupFunc
}

// nolint
func newNode(t *testing.T, name string, delegate memberlist.Delegate) *memberlist.Memberlist {
	// TODO: fix TLS verification
	conf := autotls.Config{
		AutoTLS:            true,
		ClientAuth:         tls.NoClientCert,
		InsecureSkipVerify: false,
	}

	require.NoError(t, autotls.Setup(&conf))

	mconf := memberlist.DefaultLocalConfig()
	mconf.BindPort = 0
	if delegate != nil {
		mconf.Delegate = delegate
	}
	mconf.BindAddr = "127.0.0.1"
	mconf.Logger = log.New(os.Stderr, name+": ", log.LstdFlags)

	tConfig := TransportConfig{
		BindAddrs:  []string{mconf.BindAddr},
		BindPort:   mconf.BindPort,
		TLS:        conf.ServerTLS,
		TLSEnabled: false, // TODO: fix TLS verification
	}

	// implement some poor mechanism here
	retry := func(limit int) (*Transport, error) {
		var err error
		for try := 0; try < limit; try++ {
			var transport *Transport
			if transport, err = NewTransport(tConfig); err == nil {
				return transport, nil
			}
			if strings.Contains(err.Error(), "address already in use") {
				mconf.Logger.Printf("[DEBUG] memberlist: Got bind error: %v", err)
				continue
			}
		}
		return nil, fmt.Errorf("failed to obtain an address: %v", err)
	}

	maxAttempts := 1
	if mconf.BindPort == 0 {
		maxAttempts = 10
	}

	transport, err := retry(maxAttempts)
	require.NoError(t, err)
	if mconf.BindPort == 0 {
		mconf.BindPort = transport.GetAutoBindPort()
		mconf.AdvertisePort = mconf.BindPort
		mconf.Logger.Printf("[DEBUG] memberlist: Using dynamic bind port %d", mconf.BindPort)
	}
	mconf.Transport = transport
	mconf.Name = fmt.Sprintf("node-%d", mconf.BindPort)
	mlist, err := memberlist.Create(mconf)
	require.NoError(t, err)
	return mlist
}

type mockCluster struct {
	members   [2]*memberlist.Memberlist
	delegates [2]*delegate
}

type delegate struct {
	sync.Mutex
	messages [][]byte
}

// nolint
func (d *delegate) NodeMeta(limit int) []byte {
	return []byte{}
}

func (d *delegate) Messages() [][]byte {
	d.Lock()
	defer d.Unlock()
	return d.messages
}

// nolint
func (d *delegate) NotifyMsg(m []byte) {
	d.Lock()
	defer d.Unlock()
	d.messages = append(d.messages, m)
}

// nolint
func (d *delegate) GetBroadcasts(overhead, limit int) [][]byte {
	return [][]byte{}
}

// nolint
func (d *delegate) LocalState(join bool) []byte {
	return []byte{}
}

// nolint
func (d *delegate) MergeRemoteState(buf []byte, join bool) {}

// newRawTransport starts a transport on the loopback interface for raw wire tests
func newRawTransport(t *testing.T, config TransportConfig) *Transport {
	t.Helper()
	config.BindAddrs = []string{"127.0.0.1"}
	config.BindPort = 0
	transport, err := NewTransport(config)
	require.NoError(t, err)
	require.NotNil(t, transport)
	return transport
}

// dialRaw opens a raw TCP connection to the given transport
func dialRaw(t *testing.T, transport *Transport) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", transport.tcpListeners[0].Addr().String(), time.Second)
	require.NoError(t, err)
	return conn
}

// sendRawPacket writes a complete packet frame with the given payload and digest, then closes the connection
func sendRawPacket(t *testing.T, transport *Transport, payload, digest []byte) {
	t.Helper()
	conn := dialRaw(t, transport)
	from := "127.0.0.1:1"
	header := append([]byte{byte(packet), byte(len(from))}, []byte(from)...)
	_, err := conn.Write(header)
	require.NoError(t, err)
	_, err = conn.Write(payload)
	require.NoError(t, err)
	_, err = conn.Write(digest)
	require.NoError(t, err)
	require.NoError(t, conn.Close())
}

func receivePacket(t *testing.T, transport *Transport, timeout time.Duration) *memberlist.Packet {
	t.Helper()
	select {
	case pkt := <-transport.PacketCh():
		return pkt
	case <-time.After(timeout):
		return nil
	}
}

func TestTransportConfiguredLoggerIsUsed(t *testing.T) {
	logger := dlog.New(dlog.InfoLevel, io.Discard)
	transport := newRawTransport(t, TransportConfig{Logger: logger, DebugEnabled: true})
	defer func() { assert.NoError(t, transport.Shutdown()) }()

	assert.Same(t, logger, transport.logger)
	// debug logging goes through the configured logger too
	assert.Same(t, logger, transport.debugLogger())

	// without debug the discard logger is returned
	transport.config.DebugEnabled = false
	assert.Same(t, dlog.DiscardLogger, transport.debugLogger())
}

func TestTransportDefaultLoggerWhenNoneConfigured(t *testing.T) {
	transport := newRawTransport(t, TransportConfig{})
	defer func() { assert.NoError(t, transport.Shutdown()) }()
	require.NotNil(t, transport.logger)
}

func TestTransportTLSEnabledWithoutConfig(t *testing.T) {
	transport, err := NewTransport(TransportConfig{
		BindAddrs:  []string{"127.0.0.1"},
		TLSEnabled: true,
	})
	require.Error(t, err)
	require.Nil(t, transport)
	assert.Contains(t, err.Error(), "TLS is enabled but no TLS configuration is provided")
}

func TestTransportDigestMismatchIsDropped(t *testing.T) {
	transport := newRawTransport(t, TransportConfig{Logger: dlog.DiscardLogger})
	defer func() { assert.NoError(t, transport.Shutdown()) }()

	payload := []byte("hello memberlist")
	bogus := make([]byte, md5.Size)

	// a corrupted packet must never be delivered
	sendRawPacket(t, transport, payload, bogus)
	require.Nil(t, receivePacket(t, transport, 500*time.Millisecond))

	// a valid packet is delivered
	digest := md5.Sum(payload) // nolint
	sendRawPacket(t, transport, payload, digest[:])
	pkt := receivePacket(t, transport, 2*time.Second)
	require.NotNil(t, pkt)
	assert.Equal(t, payload, pkt.Buf)
	assert.Equal(t, "127.0.0.1:1", pkt.From.String())
}

func TestTransportOversizedPacketIsRejected(t *testing.T) {
	transport := newRawTransport(t, TransportConfig{Logger: dlog.DiscardLogger})
	defer func() { assert.NoError(t, transport.Shutdown()) }()

	// payload + digest exceeds the maximum packet size by one byte
	payload := make([]byte, maxPacketSize-md5.Size+1)
	digest := md5.Sum(payload) // nolint
	sendRawPacket(t, transport, payload, digest[:])
	require.Nil(t, receivePacket(t, transport, 500*time.Millisecond))

	// payload + digest exactly at the maximum packet size is accepted
	payload = make([]byte, maxPacketSize-md5.Size)
	digest = md5.Sum(payload) // nolint
	sendRawPacket(t, transport, payload, digest[:])
	pkt := receivePacket(t, transport, 2*time.Second)
	require.NotNil(t, pkt)
	assert.Len(t, pkt.Buf, maxPacketSize-md5.Size)
}

func TestTransportReadDeadlineFiresOnStalledConnection(t *testing.T) {
	transport := newRawTransport(t, TransportConfig{
		Logger:            dlog.DiscardLogger,
		PacketReadTimeout: 200 * time.Millisecond,
	})
	defer func() { assert.NoError(t, transport.Shutdown()) }()

	conn := dialRaw(t, transport)
	defer conn.Close()

	// announce a packet and then stall forever
	_, err := conn.Write([]byte{byte(packet)})
	require.NoError(t, err)

	// the transport must give up and close the connection
	start := time.Now()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, err = conn.Read(make([]byte, 1))
	require.Error(t, err)
	assert.ErrorIs(t, err, io.EOF)
	assert.Less(t, time.Since(start), 3*time.Second)
	require.Nil(t, receivePacket(t, transport, 100*time.Millisecond))
}

func TestTransportShutdownWithInFlightConnection(t *testing.T) {
	transport := newRawTransport(t, TransportConfig{Logger: dlog.DiscardLogger})

	// open a stream connection that nobody will ever consume from StreamCh
	conn := dialRaw(t, transport)
	defer conn.Close()
	_, err := conn.Write([]byte{byte(stream)})
	require.NoError(t, err)

	// a packet handler blocked on PacketCh as well
	payload := []byte("in-flight")
	digest := md5.Sum(payload) // nolint
	sendRawPacket(t, transport, payload, digest[:])

	// let the handlers reach their blocking sends
	util.Pause(300 * time.Millisecond)

	done := make(chan error, 1)
	go func() { done <- transport.Shutdown() }()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown did not return with in-flight connections")
	}

	// the stream connection has been closed by the transport
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, err = conn.Read(make([]byte, 1))
	require.Error(t, err)

	// Shutdown is idempotent
	assert.NoError(t, transport.Shutdown())
}

func TestTransportFinalAdvertiseAddrWithIPv6Wildcard(t *testing.T) {
	transport, err := NewTransport(TransportConfig{
		BindAddrs: []string{"::"},
		BindPort:  0,
	})
	if err != nil {
		t.Skipf("IPv6 wildcard bind not available: %v", err)
	}
	defer func() { assert.NoError(t, transport.Shutdown()) }()

	ip, port, err := transport.FinalAdvertiseAddr("", 0)
	require.NoError(t, err)
	require.NotNil(t, ip)
	// the unspecified address must never be advertised
	assert.False(t, ip.IsUnspecified())
	assert.Equal(t, transport.GetAutoBindPort(), port)
}

func TestTransportShutdownClosesStalledConnections(t *testing.T) {
	transport := newRawTransport(t, TransportConfig{
		Logger:            dlog.DiscardLogger,
		PacketReadTimeout: 30 * time.Second,
	})

	// a peer that connects and never sends anything
	conn := dialRaw(t, transport)
	defer conn.Close()

	// let the handler reach its blocking read
	util.Pause(200 * time.Millisecond)

	begin := time.Now()
	done := make(chan error, 1)
	go func() { done <- transport.Shutdown() }()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown waited for the stalled connection's read deadline")
	}
	require.Less(t, time.Since(begin), 5*time.Second)

	// the stalled connection has been closed by the transport
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, err := conn.Read(make([]byte, 1))
	require.Error(t, err)
}
