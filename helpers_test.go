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
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/groupcache/groupcache-go/v3"
	"github.com/groupcache/groupcache-go/v3/transport"
	"github.com/groupcache/groupcache-go/v3/transport/peer"
	"github.com/hashicorp/memberlist"
	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/stretchr/testify/require"
	"github.com/travisjeffery/go-dynaport"

	"github.com/tochemey/distcache/discovery"
	"github.com/tochemey/distcache/discovery/nats"
	"github.com/tochemey/distcache/internal/members"
	"github.com/tochemey/distcache/internal/pause"
	"github.com/tochemey/distcache/log"
)

// MockProvider satisfies discovery.Provider for unit tests that do not
// require discovery behavior.
type MockProvider struct{}

func (MockProvider) ID() string                       { return "stub" }
func (MockProvider) Initialize() error                { return nil }
func (MockProvider) Register() error                  { return nil }
func (MockProvider) Deregister() error                { return nil }
func (MockProvider) DiscoverPeers() ([]string, error) { return nil, nil }
func (MockProvider) Close() error                     { return nil }

type deadlineRecorder struct {
	mu        sync.Mutex
	deadlines []time.Duration
}

func (d *deadlineRecorder) record(deadline time.Duration) {
	d.mu.Lock()
	d.deadlines = append(d.deadlines, deadline)
	d.mu.Unlock()
}

type MockErrDataSource struct {
	err error
}

func (s *MockErrDataSource) Fetch(context.Context, string) ([]byte, error) { return nil, s.err }

func newMockEngine(t *testing.T, provider discovery.Provider, keySpaces ...KeySpace) (*engine, *MockDaemon, *MockMemberlist) {
	t.Helper()

	cfg := NewConfig(provider, keySpaces,
		WithLogger(log.DiscardLogger),
		WithBindAddr("127.0.0.1"),
		WithBindPort(0),
		WithDiscoveryPort(0),
	)

	engAny, err := NewEngine(cfg)
	require.NoError(t, err)
	eng := engAny.(*engine)

	daemon := &MockDaemon{}
	memberlistMock := &MockMemberlist{}

	eng.newTransportFunc = func(members.TransportConfig) (memberlist.Transport, error) { return &MockTransport{}, nil }
	eng.createMemberlistFunc = func(*memberlist.Config) (memberlistAPI, error) { return memberlistMock, nil }
	eng.listenAndServeFunc = func(context.Context, string, groupcache.Options) (daemonAPI, error) { return daemon, nil }
	eng.mlist = memberlistMock
	eng.daemon = daemon

	return eng, daemon, memberlistMock
}

type MockTransport struct{}

func (x *MockTransport) FinalAdvertiseAddr(ip string, port int) (net.IP, int, error) {
	return net.ParseIP(ip), port, nil
}

func (x *MockTransport) WriteTo(b []byte, addr string) (time.Time, error) {
	return time.Now(), nil
}

func (x *MockTransport) PacketCh() <-chan *memberlist.Packet {
	return make(chan *memberlist.Packet)
}

func (x *MockTransport) DialTimeout(addr string, timeout time.Duration) (net.Conn, error) {
	c1, c2 := net.Pipe()
	_ = c1.Close()
	return c2, nil
}

func (x *MockTransport) StreamCh() <-chan net.Conn {
	return make(chan net.Conn)
}

func (x *MockTransport) Shutdown() error { return nil }

type MockMemberlist struct {
	nodes          []*memberlist.Node
	joinCalledWith [][]string
	leaveCalled    bool
	joinErr        error
	leaveErr       error
	shutdownErr    error
}

func (x *MockMemberlist) Members() []*memberlist.Node {
	return x.nodes
}

func (x *MockMemberlist) Join(existing []string) (int, error) {
	x.joinCalledWith = append(x.joinCalledWith, existing)
	if x.joinErr != nil {
		return 0, x.joinErr
	}
	return len(existing), nil
}

func (x *MockMemberlist) Leave(_ time.Duration) error {
	x.leaveCalled = true
	return x.leaveErr
}

func (x *MockMemberlist) Shutdown() error { return x.shutdownErr }

func startEngine(t *testing.T, serverAddr string, keySpaces []KeySpace) (Engine, discovery.Provider) {
	// create a context
	ctx := context.TODO()

	// generate the ports for the single startNode
	ports := dynaport.Get(2)
	discoveryPort := ports[0]
	bindPort := ports[1]

	host := "127.0.0.1"
	provider := nats.NewDiscovery(&nats.Config{
		Server:        fmt.Sprintf("nats://%s", serverAddr),
		Subject:       "example",
		Host:          host,
		DiscoveryPort: discoveryPort,
	})

	config := NewConfig(provider, keySpaces,
		WithBindAddr(host),
		WithLogger(log.DiscardLogger),
		WithDiscoveryPort(discoveryPort),
		WithBindPort(bindPort))

	engine, err := NewEngine(config)
	require.NoError(t, err)
	require.NotNil(t, engine)

	// start the node
	require.NoError(t, engine.Start(ctx))

	pause.Pause(500 * time.Millisecond)

	// return the cluster startNode
	return engine, provider
}

func startSecuredEngine(t *testing.T, serverAddr string, keySpaces []KeySpace, info *TLSInfo) (Engine, discovery.Provider) {
	// create a context
	ctx := context.TODO()

	// generate the ports for the single startNode
	ports := dynaport.Get(2)
	discoveryPort := ports[0]
	bindPort := ports[1]

	host := "127.0.0.1"
	provider := nats.NewDiscovery(&nats.Config{
		Server:        fmt.Sprintf("nats://%s", serverAddr),
		Subject:       "example",
		Host:          host,
		DiscoveryPort: discoveryPort,
	})

	config := NewConfig(provider, keySpaces,
		WithBindAddr(host),
		WithLogger(log.DiscardLogger),
		WithDiscoveryPort(discoveryPort),
		WithShutdownTimeout(3*time.Second),
		WithTLS(info),
		WithBindPort(bindPort))

	engine, err := NewEngine(config)
	require.NoError(t, err)
	require.NotNil(t, engine)

	// start the node
	require.NoError(t, engine.Start(ctx))

	pause.Pause(500 * time.Millisecond)

	// return the cluster startNode
	return engine, provider
}

func startNatsServer(t *testing.T) *natsserver.Server {
	t.Helper()
	serv, err := natsserver.NewServer(&natsserver.Options{
		Host: "127.0.0.1",
		Port: -1,
	})

	require.NoError(t, err)

	ready := make(chan bool)
	go func() {
		ready <- true
		serv.Start()
	}()
	<-ready

	if !serv.ReadyForConnections(2 * time.Second) {
		t.Fatalf("nats-io server failed to start")
	}

	return serv
}

type MockDaemon struct {
	address        string
	setPeersCalls  [][]peer.Info
	groups         map[string]transport.Group
	newGroupCalls  []mockNewGroupCall
	setPeersErr    error
	newGroupErr    error
	shutdownCalled bool
	mu             sync.Mutex
}

type mockNewGroupCall struct {
	name     string
	maxBytes int64
}

func (x *MockDaemon) SetPeers(_ context.Context, infos []peer.Info) error {
	x.mu.Lock()
	x.setPeersCalls = append(x.setPeersCalls, infos)
	x.mu.Unlock()
	return x.setPeersErr
}

func (x *MockDaemon) NewGroup(name string, maxBytes int64, getter groupcache.Getter) (transport.Group, error) {
	x.newGroupCalls = append(x.newGroupCalls, mockNewGroupCall{name: name, maxBytes: maxBytes})
	if x.newGroupErr != nil {
		return nil, x.newGroupErr
	}
	if x.groups == nil {
		x.groups = make(map[string]transport.Group)
	}
	g := &MockGroup{name: name, getter: getter}
	x.groups[name] = g
	return g, nil
}

func (x *MockDaemon) RemoveGroup(name string) {
	delete(x.groups, name)
}

func (x *MockDaemon) Shutdown(_ context.Context) error {
	x.shutdownCalled = true
	return nil
}

func (x *MockDaemon) setPeersCallsLen() int {
	x.mu.Lock()
	defer x.mu.Unlock()
	return len(x.setPeersCalls)
}

type MockFailOnceDaemon struct {
	calls int
}

func (x *MockFailOnceDaemon) SetPeers(context.Context, []peer.Info) error { return nil }

func (x *MockFailOnceDaemon) NewGroup(name string, maxBytes int64, getter groupcache.Getter) (transport.Group, error) {
	x.calls++
	if x.calls == 1 {
		return nil, errors.New("new group failed")
	}
	return &MockGroup{name: name, getter: getter}, nil
}

func (x *MockFailOnceDaemon) RemoveGroup(string) {}

func (x *MockFailOnceDaemon) Shutdown(context.Context) error { return nil }

type MockGroup struct {
	mu              sync.Mutex
	name            string
	data            map[string][]byte
	setErr          error
	getErr          error
	removeErr       error
	removeKeysErr   error
	removeCalls     []string
	removeKeysCalls [][]string
	getCalls        []string
	setCalls        []mockSetCall
	stats           groupcache.GroupStats
	getter          groupcache.Getter
}

type mockSetCall struct {
	key    string
	expiry time.Time
}

func (g *MockGroup) Set(_ context.Context, key string, value []byte, expiry time.Time, _ bool) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.setCalls = append(g.setCalls, mockSetCall{key: key, expiry: expiry})
	if g.setErr != nil {
		return g.setErr
	}
	if g.data == nil {
		g.data = make(map[string][]byte)
	}
	g.data[key] = value
	return nil
}

func (g *MockGroup) Get(ctx context.Context, key string, dest transport.Sink) error {
	g.mu.Lock()
	g.getCalls = append(g.getCalls, key)
	if g.getErr != nil {
		err := g.getErr
		g.mu.Unlock()
		return err
	}
	getter := g.getter
	if getter != nil {
		g.mu.Unlock()
		return getter.Get(ctx, key, dest)
	}
	value, ok := g.data[key]
	g.mu.Unlock()
	if !ok {
		return fmt.Errorf("not found")
	}
	return dest.SetBytes(value, time.Time{})
}

func (g *MockGroup) Remove(_ context.Context, key string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.removeCalls = append(g.removeCalls, key)
	if g.removeErr != nil {
		return g.removeErr
	}
	delete(g.data, key)
	return nil
}

func (g *MockGroup) UsedBytes() (int64, int64) { return 0, 0 }
func (g *MockGroup) Name() string              { return g.name }
func (g *MockGroup) GroupStats() groupcache.GroupStats {
	return g.stats
}
func (g *MockGroup) RemoveKeys(_ context.Context, keys ...string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.removeKeysCalls = append(g.removeKeysCalls, keys)
	if g.removeKeysErr != nil {
		return g.removeKeysErr
	}
	if g.removeErr != nil {
		return g.removeErr
	}
	for _, key := range keys {
		delete(g.data, key)
	}
	return nil
}

func (g *MockGroup) GetCalls() []string {
	g.mu.Lock()
	defer g.mu.Unlock()

	return append([]string(nil), g.getCalls...)
}

func (g *MockGroup) GetCallsLen() int {
	g.mu.Lock()
	defer g.mu.Unlock()

	return len(g.getCalls)
}

func (g *MockGroup) SetCalls() []mockSetCall {
	g.mu.Lock()
	defer g.mu.Unlock()

	return append([]mockSetCall(nil), g.setCalls...)
}

func (g *MockGroup) RemoveCalls() []string {
	g.mu.Lock()
	defer g.mu.Unlock()

	return append([]string(nil), g.removeCalls...)
}

func (g *MockGroup) RemoveKeysCalls() [][]string {
	g.mu.Lock()
	defer g.mu.Unlock()

	result := make([][]string, len(g.removeKeysCalls))
	for i, call := range g.removeKeysCalls {
		result[i] = append([]string(nil), call...)
	}
	return result
}

type minimalGroup struct {
	name string
	main int64
	hot  int64
}

func (m *minimalGroup) Set(context.Context, string, []byte, time.Time, bool) error { return nil }
func (m *minimalGroup) Get(context.Context, string, transport.Sink) error          { return nil }
func (m *minimalGroup) Remove(context.Context, string) error                       { return nil }
func (m *minimalGroup) UsedBytes() (int64, int64)                                  { return m.main, m.hot }
func (m *minimalGroup) Name() string                                               { return m.name }
func (m *minimalGroup) RemoveKeys(context.Context, ...string) error                { return nil }
