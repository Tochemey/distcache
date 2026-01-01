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
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/groupcache/groupcache-go/v3"
	"github.com/groupcache/groupcache-go/v3/transport"
	"github.com/hashicorp/memberlist"
	"github.com/kapetan-io/tackle/autotls"
	"github.com/stretchr/testify/require"
	"github.com/travisjeffery/go-dynaport"
	"go.opentelemetry.io/otel/attribute"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"github.com/tochemey/distcache/admin"
	"github.com/tochemey/distcache/internal/members"
	"github.com/tochemey/distcache/internal/pause"
	"github.com/tochemey/distcache/internal/size"
	"github.com/tochemey/distcache/internal/syncmap"
	"github.com/tochemey/distcache/log"
	mockDiscovery "github.com/tochemey/distcache/mocks/discovery"
	mocks "github.com/tochemey/distcache/mocks/discovery"
	"github.com/tochemey/distcache/otel"
	"github.com/tochemey/distcache/warmup"
)

func TestEngineErrorsPath(t *testing.T) {
	t.Run("Start fails when setupMemberlistConfig fails", func(t *testing.T) {
		engine, _, _ := newMockEngine(t, mockDiscovery.NewProvider(t), NewMockKeySpace("ks", size.MB, NewMockDataSource()))
		engine.newTransportFunc = func(members.TransportConfig) (memberlist.Transport, error) { return nil, errors.New("transport") }

		err := engine.Start(context.Background())
		require.Error(t, err)
	})

	t.Run("Start fails when startDaemon fails", func(t *testing.T) {
		provider := mockDiscovery.NewProvider(t)
		provider.EXPECT().Initialize().Return(nil)
		provider.EXPECT().Register().Return(nil)
		provider.EXPECT().DiscoverPeers().Return([]string{}, nil)

		engine, _, _ := newMockEngine(t, provider, NewMockKeySpace("ks", size.MB, NewMockDataSource()))
		engine.listenAndServeFunc = func(context.Context, string, groupcache.Options) (daemonAPI, error) {
			return nil, errors.New("daemon")
		}

		err := engine.Start(context.Background())
		require.Error(t, err)
	})

	t.Run("Start fails when applyPeers fails", func(t *testing.T) {
		provider := mockDiscovery.NewProvider(t)
		provider.EXPECT().Initialize().Return(nil)
		provider.EXPECT().Register().Return(nil)
		provider.EXPECT().DiscoverPeers().Return([]string{}, nil)

		engine, daemon, _ := newMockEngine(t, provider, NewMockKeySpace("ks", size.MB, NewMockDataSource()))
		daemon.setPeersErr = errors.New("peers")
		engine.listenAndServeFunc = func(context.Context, string, groupcache.Options) (daemonAPI, error) {
			return daemon, nil
		}

		err := engine.Start(context.Background())
		require.Error(t, err)
	})

	t.Run("Start fails when createGroups fails", func(t *testing.T) {
		provider := mockDiscovery.NewProvider(t)
		provider.EXPECT().Initialize().Return(nil)
		provider.EXPECT().Register().Return(nil)
		provider.EXPECT().DiscoverPeers().Return([]string{}, nil)

		engine, daemon, _ := newMockEngine(t, provider, NewMockKeySpace("ks", size.MB, NewMockDataSource()))
		daemon.newGroupErr = errors.New("group")
		engine.listenAndServeFunc = func(context.Context, string, groupcache.Options) (daemonAPI, error) {
			return daemon, nil
		}

		err := engine.Start(context.Background())
		require.Error(t, err)
	})

	t.Run("Getter returns datasource error", func(t *testing.T) {
		dsErr := errors.New("fetch")
		errorDS := &MockErrDataSource{err: dsErr}
		ks := NewMockKeySpace("ks", size.MB, errorDS)
		provider := mockDiscovery.NewProvider(t)
		engine, daemon, _ := newMockEngine(t, provider, ks)
		engine.daemon = daemon

		require.NoError(t, engine.createGroups())
		group, ok := engine.groups.Get("ks")
		require.True(t, ok)
		err := group.Get(context.Background(), "key", transport.AllocatingByteSliceSink(&[]byte{}))
		require.ErrorIs(t, err, dsErr)
	})

	t.Run("Stop fails when leave returns error", func(t *testing.T) {
		engine, daemon, mlist := newMockEngine(t, mockDiscovery.NewProvider(t), NewMockKeySpace("ks", size.MB, NewMockDataSource()))
		engine.started.Store(true)
		mlist.leaveErr = errors.New("leave")
		engine.mlist = mlist
		engine.daemon = daemon

		err := engine.Stop(context.Background())
		require.Error(t, err)
	})

	t.Run("PutMany returns error when group set fails", func(t *testing.T) {
		engine, _, _ := newMockEngine(t, mockDiscovery.NewProvider(t), NewMockKeySpace("ks", size.MB, NewMockDataSource()))
		g := &MockGroup{name: "ks", setErr: errors.New("set")}
		engine.groups.Set("ks", g)

		err := engine.PutMany(context.Background(), "ks", []*Entry{{KV: KV{Key: "a", Value: []byte("v")}}})
		require.Error(t, err)
	})

	t.Run("Get returns error when group get fails", func(t *testing.T) {
		engine, _, _ := newMockEngine(t, mockDiscovery.NewProvider(t), NewMockKeySpace("ks", size.MB, NewMockDataSource()))
		g := &MockGroup{name: "ks", getErr: errors.New("get")}
		engine.groups.Set("ks", g)

		_, err := engine.Get(context.Background(), "ks", "a")
		require.Error(t, err)
	})

	t.Run("DeleteMany returns error when RemoveKeys fails", func(t *testing.T) {
		engine, _, _ := newMockEngine(t, mockDiscovery.NewProvider(t), NewMockKeySpace("ks", size.MB, NewMockDataSource()))
		g := &MockGroup{name: "ks", removeKeysErr: errors.New("remove")}
		engine.groups.Set("ks", g)

		err := engine.DeleteMany(context.Background(), "ks", []string{"a"})
		require.Error(t, err)
		removeKeysCalls := g.RemoveKeysCalls()
		require.Len(t, removeKeysCalls, 1)
		require.Equal(t, []string{"a"}, removeKeysCalls[0])
		require.Empty(t, g.RemoveCalls())
	})

	t.Run("DeleteKeySpace returns not found", func(t *testing.T) {
		engine, _, _ := newMockEngine(t, mockDiscovery.NewProvider(t), NewMockKeySpace("ks", size.MB, NewMockDataSource()))
		err := engine.DeleteKeySpace(context.Background(), "missing")
		require.ErrorIs(t, err, ErrKeySpaceNotFound)
	})

	t.Run("Peers returns error when decoding fails", func(t *testing.T) {
		engine, _, mlist := newMockEngine(t, mockDiscovery.NewProvider(t), NewMockKeySpace("ks", size.MB, NewMockDataSource()))
		mlist.nodes = []*memberlist.Node{{Meta: []byte("bad json")}}
		engine.mlist = mlist

		_, err := engine.peers()
		require.Error(t, err)
	})

	t.Run("EventsListener handles decode error and skips self", func(t *testing.T) {
		engine, daemon, _ := newMockEngine(t, mockDiscovery.NewProvider(t), NewMockKeySpace("ks", size.MB, NewMockDataSource()))
		engine.daemon = daemon
		engine.stopEventsListenerSig = make(chan struct{}, 1)
		eventsCh := make(chan memberlist.NodeEvent, 2)

		go engine.eventsListener(eventsCh)

		eventsCh <- memberlist.NodeEvent{Node: &memberlist.Node{Meta: []byte("bad")}}

		selfMeta, _ := json.Marshal(engine.hostNode)
		eventsCh <- memberlist.NodeEvent{
			Node:  &memberlist.Node{Meta: selfMeta},
			Event: memberlist.NodeJoin,
		}

		engine.stopEventsListenerSig <- struct{}{}
	})

	t.Run("EventsListener logs decode errors", func(t *testing.T) {
		engine, daemon, _ := newMockEngine(t, mockDiscovery.NewProvider(t), NewMockKeySpace("ks", size.MB, NewMockDataSource()))
		engine.daemon = daemon
		engine.stopEventsListenerSig = make(chan struct{}, 1)
		eventsCh := make(chan memberlist.NodeEvent, 1)

		go engine.eventsListener(eventsCh)
		eventsCh <- memberlist.NodeEvent{Node: &memberlist.Node{Meta: []byte("bad meta")}}
		pause.Pause(10 * time.Millisecond)
		engine.stopEventsListenerSig <- struct{}{}
	})

	t.Run("EventsListener handles node update events", func(t *testing.T) {
		engine, daemon, _ := newMockEngine(t, mockDiscovery.NewProvider(t), NewMockKeySpace("ks", size.MB, NewMockDataSource()))
		engine.daemon = daemon
		engine.stopEventsListenerSig = make(chan struct{}, 1)
		eventsCh := make(chan memberlist.NodeEvent, 1)

		meta, err := json.Marshal(&Peer{BindAddr: "10.0.0.2", BindPort: 9000, DiscoveryPort: 9001})
		require.NoError(t, err)

		go engine.eventsListener(eventsCh)
		eventsCh <- memberlist.NodeEvent{
			Node:  &memberlist.Node{Meta: meta},
			Event: memberlist.NodeUpdate,
		}

		pause.Pause(20 * time.Millisecond)
		engine.stopEventsListenerSig <- struct{}{}
		require.NotZero(t, daemon.setPeersCallsLen())
	})

	t.Run("JoinCluster fails when memberlist creation fails", func(t *testing.T) {
		engine, _, _ := newMockEngine(t, mockDiscovery.NewProvider(t), NewMockKeySpace("ks", size.MB, NewMockDataSource()))
		engine.createMemberlistFunc = func(*memberlist.Config) (memberlistAPI, error) { return nil, errors.New("mlist") }

		err := engine.joinCluster(context.Background())
		require.Error(t, err)
	})

	t.Run("JoinCluster fails when discover peers fails", func(t *testing.T) {
		provider := mockDiscovery.NewProvider(t)
		provider.EXPECT().DiscoverPeers().Return(nil, errors.New("discover"))
		engine, _, mlist := newMockEngine(t, provider, NewMockKeySpace("ks", size.MB, NewMockDataSource()))
		engine.createMemberlistFunc = func(*memberlist.Config) (memberlistAPI, error) { return mlist, nil }

		err := engine.joinCluster(context.Background())
		require.Error(t, err)
	})

	t.Run("JoinCluster fails when quorum not met", func(t *testing.T) {
		provider := mockDiscovery.NewProvider(t)
		provider.EXPECT().DiscoverPeers().Return([]string{"p1"}, nil)
		engine, _, mlist := newMockEngine(t, provider, NewMockKeySpace("ks", size.MB, NewMockDataSource()))
		engine.createMemberlistFunc = func(*memberlist.Config) (memberlistAPI, error) { return mlist, nil }
		engine.config.minimumPeersQuorum = 2

		err := engine.joinCluster(context.Background())
		require.ErrorIs(t, err, ErrClusterQuorum)
	})

	t.Run("JoinCluster fails when join returns error", func(t *testing.T) {
		provider := mockDiscovery.NewProvider(t)
		provider.EXPECT().DiscoverPeers().Return([]string{"p1"}, nil)
		engine, _, mlist := newMockEngine(t, provider, NewMockKeySpace("ks", size.MB, NewMockDataSource()))
		mlist.joinErr = errors.New("join")
		engine.createMemberlistFunc = func(*memberlist.Config) (memberlistAPI, error) { return mlist, nil }

		err := engine.joinCluster(context.Background())
		require.Error(t, err)
	})
}

func TestEngine(t *testing.T) {
	t.Run("With NewEngine failure due invalid IP", func(t *testing.T) {
		engine, err := NewEngine(&Config{
			bindAddr: "127.0.0.0.12",
		})
		require.Error(t, err)
		require.Nil(t, engine)
	})
	t.Run("When discovery provider registration failed engine cannot start", func(t *testing.T) {
		ctx := context.Background()
		// generate the ports for the single startNode
		ports := dynaport.Get(2)
		discoveryPort := ports[0]
		bindPort := ports[1]
		host := "127.0.0.1"

		// mock the discovery provider
		provider := new(mocks.Provider)
		dataSource := NewMockDataSource()
		keySpace := "users"

		keySpaces := []KeySpace{
			NewMockKeySpace(keySpace, size.MB, dataSource),
		}

		traceConfig := otel.NewTracerConfig()
		metricConfig := otel.NewMetricConfig()

		config := NewConfig(provider, keySpaces,
			WithBindAddr(host),
			WithLogger(log.DiscardLogger),
			WithDiscoveryPort(discoveryPort),
			WithTracing(traceConfig),
			WithMetrics(metricConfig),
			WithBindPort(bindPort))

		engine, err := NewEngine(config)
		require.NoError(t, err)
		require.NotNil(t, engine)

		err = errors.New("some error")
		provider.EXPECT().Initialize().Return(nil)
		provider.EXPECT().Register().Return(err)

		err = engine.Start(ctx)
		require.Error(t, err)
		require.NoError(t, engine.Stop(ctx))
		provider.AssertExpectations(t)
	})
	t.Run("With KeySpace Not Found error", func(t *testing.T) {
		ctx := context.Background()

		srv := startNatsServer(t)
		serverAddress := srv.Addr().String()

		dataSource := NewMockDataSource()
		keySpace := "users"

		engine, provider := startEngine(t, serverAddress, []KeySpace{
			NewMockKeySpace(keySpace, size.MB, dataSource),
		})
		require.NotNil(t, engine)
		require.NotNil(t, provider)

		user := &User{
			ID:   "user1",
			Name: "user",
			Age:  10,
		}

		bytea, err := json.Marshal(user)
		require.NoError(t, err)

		entry := &Entry{
			KV: KV{
				Key:   "users",
				Value: bytea,
			},
		}

		// put when keyspace does not exist
		err = engine.Put(ctx, "invalid Space", entry)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrKeySpaceNotFound)

		// delete when keyspace does not exist
		err = engine.Delete(ctx, "invalid Space", "users")
		require.Error(t, err)
		require.ErrorIs(t, err, ErrKeySpaceNotFound)

		err = engine.DeleteMany(ctx, "ivalid Space", []string{"keys"})
		require.Error(t, err)
		require.ErrorIs(t, err, ErrKeySpaceNotFound)

		_, err = engine.Get(ctx, "invalid Space", "key")
		require.Error(t, err)
		require.ErrorIs(t, err, ErrKeySpaceNotFound)

		require.NoError(t, engine.Stop(ctx))
		require.NoError(t, provider.Close())
		srv.Shutdown()
	})
	t.Run("With PutMany And DeleteMany", func(t *testing.T) {
		ctx := context.Background()

		srv := startNatsServer(t)
		serverAddress := srv.Addr().String()

		dataSource := NewMockDataSource()
		keySpace := "users"

		engine, provider := startEngine(t, serverAddress, []KeySpace{
			NewMockKeySpace(keySpace, size.MB, dataSource),
		})
		require.NotNil(t, engine)
		require.NotNil(t, provider)

		user := &User{
			ID:   "user1",
			Name: "user",
			Age:  10,
		}

		bytea, err := json.Marshal(user)
		require.NoError(t, err)

		entries := []*Entry{
			{
				KV: KV{
					Key:   "users",
					Value: bytea,
				},
			},
		}

		// When keySpace exist
		err = engine.PutMany(ctx, keySpace, entries)
		require.NoError(t, err)

		err = engine.PutMany(ctx, "invalid Space", entries)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrKeySpaceNotFound)

		err = engine.DeleteMany(ctx, keySpace, []string{"users"})
		require.NoError(t, err)

		require.NoError(t, engine.Stop(ctx))
		require.NoError(t, provider.Close())
		srv.Shutdown()
	})
	t.Run("With caching operations in cluster", func(t *testing.T) {
		ctx := context.Background()

		srv := startNatsServer(t)
		serverAddress := srv.Addr().String()

		// three names space with three data sources
		source1 := NewMockDataSource()
		source2 := NewMockDataSource()
		source3 := NewMockDataSource()

		keySpace1 := "keyspace1"
		keySpace2 := "keyspace2"
		keySpace3 := "keyspace3"

		keysSpaces := []KeySpace{
			NewMockKeySpace(keySpace1, size.MB, source1),
			NewMockKeySpace(keySpace2, size.MB, source2),
			NewMockKeySpace(keySpace3, size.MB, source3),
		}

		engine1, provider1 := startEngine(t, serverAddress, keysSpaces)
		require.NotNil(t, engine1)
		require.NotNil(t, provider1)

		engine2, provider2 := startEngine(t, serverAddress, keysSpaces)
		require.NotNil(t, engine2)
		require.NotNil(t, provider2)

		engine3, provider3 := startEngine(t, serverAddress, keysSpaces)
		require.NotNil(t, engine3)
		require.NotNil(t, provider3)

		user := &User{
			ID:   "user1",
			Name: "user",
			Age:  10,
		}

		bytea, err := json.Marshal(user)
		require.NoError(t, err)

		// Let us insert some record into the data source and try get it from any other engine
		require.NoError(t, source1.Insert(ctx, []*User{user}))

		// cache the user record
		require.NoError(t, engine1.Put(ctx, keySpace1, &Entry{
			KV: KV{
				Key:   user.ID,
				Value: bytea,
			},
			Expiry: time.Time{},
		}))

		kv, err := engine2.Get(ctx, keySpace1, user.ID)
		require.NoError(t, err)
		require.NotNil(t, kv)
		require.Equal(t, user.ID, kv.Key)
		require.True(t, bytes.Equal(bytea, kv.Value))

		// let us insert a record with data source 2 and try to get it from any other engine
		user2 := &User{ID: "user2", Name: "user2", Age: 10}
		require.NoError(t, source2.Insert(ctx, []*User{user2}))

		bytea, err = json.Marshal(user2)
		require.NoError(t, err)

		kv, err = engine3.Get(ctx, keySpace2, user2.ID)
		require.NoError(t, err)
		require.NotNil(t, kv)
		require.Equal(t, user2.ID, kv.Key)
		require.True(t, bytes.Equal(bytea, kv.Value))

		kv, err = engine1.Get(ctx, keySpace2, user2.ID)
		require.NoError(t, err)
		require.NotNil(t, kv)
		require.Equal(t, user2.ID, kv.Key)
		require.True(t, bytes.Equal(bytea, kv.Value))

		err = engine3.Delete(ctx, keySpace2, user2.ID)
		require.NoError(t, err)

		// fetching it will go to the data source and fetch it
		kv, err = engine1.Get(ctx, keySpace2, user2.ID)
		require.NoError(t, err)
		require.NotNil(t, kv)
		require.Equal(t, user2.ID, kv.Key)
		require.True(t, bytes.Equal(bytea, kv.Value))

		require.NoError(t, engine1.Stop(ctx))
		require.NoError(t, engine2.Stop(ctx))
		require.NoError(t, engine3.Stop(ctx))

		require.NoError(t, provider1.Close())
		require.NoError(t, provider2.Close())
		require.NoError(t, provider3.Close())

		srv.Shutdown()
	})
	t.Run("With cluster topology changes", func(t *testing.T) {
		ctx := context.Background()

		srv := startNatsServer(t)
		serverAddress := srv.Addr().String()

		// three names space with three data sources
		source1 := NewMockDataSource()
		source2 := NewMockDataSource()
		source3 := NewMockDataSource()

		keySpace1 := "keyspace1"
		keySpace2 := "keyspace2"
		keySpace3 := "keyspace3"

		keysSpaces := []KeySpace{
			NewMockKeySpace(keySpace1, size.MB, source1),
			NewMockKeySpace(keySpace2, size.MB, source2),
			NewMockKeySpace(keySpace3, size.MB, source3),
		}

		engine1, provider1 := startEngine(t, serverAddress, keysSpaces)
		require.NotNil(t, engine1)
		require.NotNil(t, provider1)

		engine2, provider2 := startEngine(t, serverAddress, keysSpaces)
		require.NotNil(t, engine2)
		require.NotNil(t, provider2)

		engine3, provider3 := startEngine(t, serverAddress, keysSpaces)
		require.NotNil(t, engine3)
		require.NotNil(t, provider3)

		user := &User{
			ID:   "user1",
			Name: "user",
			Age:  10,
		}

		bytea, err := json.Marshal(user)
		require.NoError(t, err)

		// Let us insert some record into the data source and try get it from any other engine
		require.NoError(t, source1.Insert(ctx, []*User{user}))

		// cache the user record
		require.NoError(t, engine1.Put(ctx, keySpace1, &Entry{
			KV: KV{
				Key:   user.ID,
				Value: bytea,
			},
			Expiry: time.Time{},
		}))

		kv, err := engine2.Get(ctx, keySpace1, user.ID)
		require.NoError(t, err)
		require.NotNil(t, kv)
		require.Equal(t, user.ID, kv.Key)
		require.True(t, bytes.Equal(bytea, kv.Value))

		// stop the engine 3
		require.NoError(t, engine3.Stop(ctx))
		pause.Pause(time.Minute)

		// let us insert a record with data source 2 and try to get it from any other engine
		user2 := &User{ID: "user2", Name: "user2", Age: 10}
		require.NoError(t, source2.Insert(ctx, []*User{user2}))

		bytea, err = json.Marshal(user2)
		require.NoError(t, err)

		kv, err = engine1.Get(ctx, keySpace2, user2.ID)
		require.NoError(t, err)
		require.NotNil(t, kv)
		require.Equal(t, user2.ID, kv.Key)
		require.True(t, bytes.Equal(bytea, kv.Value))

		require.NoError(t, engine1.Stop(ctx))
		require.NoError(t, engine2.Stop(ctx))

		require.NoError(t, provider1.Close())
		require.NoError(t, provider2.Close())
		require.NoError(t, provider3.Close())

		srv.Shutdown()
	})
	t.Run("With secured caching operations in cluster", func(t *testing.T) {
		ctx := context.Background()
		// AutoGenerate TLS certs
		conf := autotls.Config{AutoTLS: true}
		require.NoError(t, autotls.Setup(&conf))

		tlsInfo := &TLSInfo{
			ClientTLS: conf.ClientTLS,
			ServerTLS: conf.ServerTLS,
		}

		srv := startNatsServer(t)
		serverAddress := srv.Addr().String()

		// three names space with three data sources
		source1 := NewMockDataSource()
		source2 := NewMockDataSource()
		source3 := NewMockDataSource()

		keySpace1 := "keyspace1"
		keySpace2 := "keyspace2"
		keySpace3 := "keyspace3"

		keysSpaces := []KeySpace{
			NewMockKeySpace(keySpace1, size.MB, source1),
			NewMockKeySpace(keySpace2, size.MB, source2),
			NewMockKeySpace(keySpace3, size.MB, source3),
		}

		engine1, provider1 := startSecuredEngine(t, serverAddress, keysSpaces, tlsInfo)
		require.NotNil(t, engine1)
		require.NotNil(t, provider1)

		engine2, provider2 := startSecuredEngine(t, serverAddress, keysSpaces, tlsInfo)
		require.NotNil(t, engine2)
		require.NotNil(t, provider2)

		engine3, provider3 := startSecuredEngine(t, serverAddress, keysSpaces, tlsInfo)
		require.NotNil(t, engine3)
		require.NotNil(t, provider3)

		user := &User{
			ID:   "user1",
			Name: "user",
			Age:  10,
		}

		bytea, err := json.Marshal(user)
		require.NoError(t, err)

		// Let us insert some record into the data source and try get it from any other engine
		require.NoError(t, source1.Insert(ctx, []*User{user}))

		// cache the user record
		require.NoError(t, engine1.Put(ctx, keySpace1, &Entry{
			KV: KV{
				Key:   user.ID,
				Value: bytea,
			},
			Expiry: time.Time{},
		}))

		kv, err := engine2.Get(ctx, keySpace1, user.ID)
		require.NoError(t, err)
		require.NotNil(t, kv)
		require.Equal(t, user.ID, kv.Key)
		require.True(t, bytes.Equal(bytea, kv.Value))

		// let us insert a record with data source 2 and try to get it from any other engine
		user2 := &User{ID: "user2", Name: "user2", Age: 10}
		require.NoError(t, source2.Insert(ctx, []*User{user2}))

		bytea, err = json.Marshal(user2)
		require.NoError(t, err)

		kv, err = engine3.Get(ctx, keySpace2, user2.ID)
		require.NoError(t, err)
		require.NotNil(t, kv)
		require.Equal(t, user2.ID, kv.Key)
		require.True(t, bytes.Equal(bytea, kv.Value))

		kv, err = engine1.Get(ctx, keySpace2, user2.ID)
		require.NoError(t, err)
		require.NotNil(t, kv)
		require.Equal(t, user2.ID, kv.Key)
		require.True(t, bytes.Equal(bytea, kv.Value))

		err = engine3.Delete(ctx, keySpace2, user2.ID)
		require.NoError(t, err)

		// fetching it will go to the data source and fetch it
		kv, err = engine1.Get(ctx, keySpace2, user2.ID)
		require.NoError(t, err)
		require.NotNil(t, kv)
		require.Equal(t, user2.ID, kv.Key)
		require.True(t, bytes.Equal(bytea, kv.Value))

		require.NoError(t, engine1.Stop(ctx))
		require.NoError(t, engine2.Stop(ctx))
		require.NoError(t, engine3.Stop(ctx))

		require.NoError(t, provider1.Close())
		require.NoError(t, provider2.Close())
		require.NoError(t, provider3.Close())

		srv.Shutdown()
	})
	t.Run("With Delete KeySpace", func(t *testing.T) {
		ctx := context.Background()

		srv := startNatsServer(t)
		serverAddress := srv.Addr().String()

		dataSource := NewMockDataSource()
		keySpace := "users"

		engine, provider := startEngine(t, serverAddress, []KeySpace{
			NewMockKeySpace(keySpace, size.MB, dataSource),
		})
		require.NotNil(t, engine)
		require.NotNil(t, provider)

		keySpaces := engine.KeySpaces()
		require.Len(t, keySpaces, 1)

		err := engine.DeleteKeySpace(ctx, keySpace)
		require.NoError(t, err)

		keySpaces = engine.KeySpaces()
		require.Zero(t, len(keySpaces))

		require.NoError(t, engine.Stop(ctx))
		require.NoError(t, provider.Close())
		srv.Shutdown()
	})
	t.Run("With Delete KeySpaces", func(t *testing.T) {
		ctx := context.Background()

		srv := startNatsServer(t)
		serverAddress := srv.Addr().String()

		dataSource := NewMockDataSource()
		keySpace := "users"

		engine, provider := startEngine(t, serverAddress, []KeySpace{
			NewMockKeySpace(keySpace, size.MB, dataSource),
		})
		require.NotNil(t, engine)
		require.NotNil(t, provider)

		keySpaces := engine.KeySpaces()
		require.Len(t, keySpaces, 1)

		err := engine.DeleteKeyspaces(ctx, []string{keySpace})
		require.NoError(t, err)

		keySpaces = engine.KeySpaces()
		require.Zero(t, len(keySpaces))

		require.NoError(t, engine.Stop(ctx))
		require.NoError(t, provider.Close())
		srv.Shutdown()
	})
}

func TestStartStop(t *testing.T) {
	remoteMeta, err := json.Marshal(&Peer{BindAddr: "127.0.0.2", BindPort: 9100, DiscoveryPort: 9200})
	require.NoError(t, err)

	mlist := &MockMemberlist{nodes: []*memberlist.Node{{Meta: remoteMeta}}}
	daemon := &MockDaemon{}

	ds := NewMockDataSource()
	keySpaces := []KeySpace{NewMockKeySpace("users", size.MB, ds)}
	provider := mockDiscovery.NewProvider(t)
	provider.EXPECT().Initialize().Return(nil)
	provider.EXPECT().Register().Return(nil)
	provider.EXPECT().DiscoverPeers().Return([]string{"127.0.0.2:9200"}, nil)
	provider.EXPECT().Deregister().Return(nil)
	provider.EXPECT().Close().Return(nil)

	cfg := NewConfig(provider, keySpaces,
		WithLogger(log.DiscardLogger),
		WithBindAddr("127.0.0.1"),
		WithBindPort(9000),
		WithDiscoveryPort(9200),
	)

	eng, err := NewEngine(cfg)
	require.NoError(t, err)
	e := eng.(*engine)
	e.newTransportFunc = func(cfg members.TransportConfig) (memberlist.Transport, error) { return &MockTransport{}, nil }
	e.createMemberlistFunc = func(cfg *memberlist.Config) (memberlistAPI, error) { return mlist, nil }
	e.listenAndServeFunc = func(ctx context.Context, address string, opts groupcache.Options) (daemonAPI, error) {
		daemon.address = address
		return daemon, nil
	}

	require.NoError(t, eng.Start(context.Background()))
	require.True(t, eng.(*engine).started.Load())
	require.Len(t, daemon.groups, 1)
	require.Len(t, daemon.setPeersCalls, 1)

	require.NoError(t, eng.Stop(context.Background()))
	require.True(t, daemon.shutdownCalled)
	require.True(t, mlist.leaveCalled)
}

func TestSetupMemberlistConfigError(t *testing.T) {
	provider := new(mockDiscovery.Provider)
	cfg := NewConfig(provider, []KeySpace{NewMockKeySpace("ks", size.MB, NewMockDataSource())},
		WithLogger(log.DiscardLogger),
	)

	eng, err := NewEngine(cfg)
	require.NoError(t, err)
	e := eng.(*engine)
	e.newTransportFunc = func(cfg members.TransportConfig) (memberlist.Transport, error) { return nil, errors.New("boom") }

	require.Error(t, e.setupMemberlistConfig())
}

func TestStartDaemonWithTLS(t *testing.T) {
	var capturedScheme string
	var capturedTLS *tls.Config

	provider := new(mockDiscovery.Provider)
	cfg := NewConfig(provider, []KeySpace{NewMockKeySpace("ks", size.MB, NewMockDataSource())},
		WithLogger(log.DiscardLogger),
	)

	eng, err := NewEngine(cfg)
	require.NoError(t, err)
	e := eng.(*engine)

	e.listenAndServeFunc = func(ctx context.Context, address string, opts groupcache.Options) (daemonAPI, error) {
		httpTransport, ok := opts.Transport.(*transport.HttpTransport)
		require.True(t, ok)

		optsVal := extractHTTPOpts(httpTransport)
		capturedScheme = optsVal.Scheme
		capturedTLS = optsVal.TLSConfig
		return &MockDaemon{}, nil
	}

	tlsInfo := &TLSInfo{ClientTLS: &tls.Config{}, ServerTLS: &tls.Config{}}
	e.config.tlsInfo = tlsInfo

	require.NoError(t, e.startDaemon(context.Background()))
	require.Equal(t, "https", capturedScheme)
	require.Equal(t, tlsInfo.ServerTLS, capturedTLS)
}

func TestApplyPeersError(t *testing.T) {
	daemon := &MockDaemon{setPeersErr: errors.New("no peers")}
	e := &engine{
		config:   &Config{},
		hostNode: &Peer{BindAddr: "127.0.0.1", BindPort: 1111},
		daemon:   daemon,
	}

	err := e.applyPeers(context.Background(), []*Peer{{BindAddr: "10.0.0.1", BindPort: 2222}})
	require.Error(t, err)
}

func TestCreateGroupsError(t *testing.T) {
	daemon := &MockDaemon{newGroupErr: errors.New("no group")}
	e := &engine{
		config: &Config{
			keySpaces: []KeySpace{
				NewMockKeySpace("ks", size.MB, NewMockDataSource()),
			},
		},
		daemon: daemon,
		groups: syncmap.New[string, transport.Group](),
	}

	require.Error(t, e.createGroups())
}

func TestConfigureEventDelegate(t *testing.T) {
	e := &engine{mconfig: memberlist.DefaultLANConfig()}
	ch := e.configureEventDelegate()
	require.NotNil(t, ch)
	require.NotNil(t, e.mconfig.Events)
}

func TestComputeTimeout(t *testing.T) {
	require.Equal(t, 9*time.Second, computeTimeout(10, time.Second))
}

func TestFromBytes(t *testing.T) {
	meta, err := json.Marshal(&Peer{BindAddr: "127.0.0.1", BindPort: 80})
	require.NoError(t, err)

	peer, err := fromBytes(meta)
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1", peer.BindAddr)
	require.Equal(t, 80, peer.BindPort)

	_, err = fromBytes([]byte("bad"))
	require.Error(t, err)
}

func TestEngineGetMany(t *testing.T) {
	t.Run("GetMany returns values in order", func(t *testing.T) {
		engine, _, _ := newMockEngine(t, mockDiscovery.NewProvider(t), NewMockKeySpace("ks", size.MB, NewMockDataSource()))
		g := &MockGroup{
			name: "ks",
			data: map[string][]byte{
				"a": []byte("first"),
				"b": []byte("second"),
			},
		}
		engine.groups.Set("ks", g)

		values, err := engine.GetMany(context.Background(), "ks", []string{"a", "b"})
		require.NoError(t, err)
		require.Len(t, values, 2)
		require.Equal(t, "a", values[0].Key)
		require.Equal(t, "first", string(values[0].Value))
		require.Equal(t, "b", values[1].Key)
		require.Equal(t, "second", string(values[1].Value))
		require.Equal(t, []string{"a", "b"}, g.GetCalls())
	})

	t.Run("GetMany returns error on failure", func(t *testing.T) {
		engine, _, _ := newMockEngine(t, mockDiscovery.NewProvider(t), NewMockKeySpace("ks", size.MB, NewMockDataSource()))
		g := &MockGroup{name: "ks", getErr: errors.New("get")}
		engine.groups.Set("ks", g)

		_, err := engine.GetMany(context.Background(), "ks", []string{"a"})
		require.Error(t, err)
	})

	t.Run("GetMany returns keyspace not found", func(t *testing.T) {
		engine, _, _ := newMockEngine(t, mockDiscovery.NewProvider(t), NewMockKeySpace("ks", size.MB, NewMockDataSource()))
		_, err := engine.GetMany(context.Background(), "missing", []string{"a"})
		require.ErrorIs(t, err, ErrKeySpaceNotFound)
	})
}

func TestUpdateKeySpace(t *testing.T) {
	t.Run("UpdateKeySpace recreates group with new settings", func(t *testing.T) {
		engine, daemon, _ := newMockEngine(t, mockDiscovery.NewProvider(t), NewMockKeySpace("ks", size.MB, NewMockDataSource()))
		require.NoError(t, engine.createGroups())

		newKeySpace := NewMockKeySpace("ks", 2*size.MB, NewMockDataSource())
		err := engine.UpdateKeySpace(context.Background(), newKeySpace)
		require.NoError(t, err)

		spec, ok := engine.keySpaces.Get("ks")
		require.True(t, ok)
		require.Equal(t, int64(2*size.MB), spec.config.MaxBytes)
		require.NotEmpty(t, daemon.newGroupCalls)
		require.Equal(t, int64(2*size.MB), daemon.newGroupCalls[len(daemon.newGroupCalls)-1].maxBytes)
	})

	t.Run("UpdateKeySpace returns not found", func(t *testing.T) {
		engine, _, _ := newMockEngine(t, mockDiscovery.NewProvider(t), NewMockKeySpace("ks", size.MB, NewMockDataSource()))
		err := engine.UpdateKeySpace(context.Background(), NewMockKeySpace("missing", size.MB, NewMockDataSource()))
		require.ErrorIs(t, err, ErrKeySpaceNotFound)
	})

	t.Run("UpdateKeySpace rejects nil keyspace", func(t *testing.T) {
		engine, _, _ := newMockEngine(t, mockDiscovery.NewProvider(t), NewMockKeySpace("ks", size.MB, NewMockDataSource()))
		err := engine.UpdateKeySpace(context.Background(), nil)
		require.Error(t, err)
	})

	t.Run("UpdateKeySpace rolls back on group creation failure", func(t *testing.T) {
		engine, _, _ := newMockEngine(t, mockDiscovery.NewProvider(t), NewMockKeySpace("ks", size.MB, NewMockDataSource()))
		require.NoError(t, engine.createGroups())

		oldSpec, _ := engine.keySpaces.Get("ks")
		daemon := &MockFailOnceDaemon{}
		engine.daemon = daemon

		err := engine.UpdateKeySpace(context.Background(), NewMockKeySpace("ks", 2*size.MB, NewMockDataSource()))
		require.Error(t, err)
		require.Equal(t, 2, daemon.calls)

		spec, ok := engine.keySpaces.Get("ks")
		require.True(t, ok)
		require.Equal(t, oldSpec.config.MaxBytes, spec.config.MaxBytes)

		group, ok := engine.groups.Get("ks")
		require.True(t, ok)
		require.NotNil(t, group)
	})
}

func TestSnapshotKeySpaces(t *testing.T) {
	engine, _, _ := newMockEngine(t, mockDiscovery.NewProvider(t), NewMockKeySpace("ks", size.MB, NewMockDataSource()))
	engine.config.readTimeout = 5 * time.Second
	engine.config.writeTimeout = 6 * time.Second

	spec := &keySpaceWrapper{
		keyspace: NewMockKeySpace("ks", size.MB, NewMockDataSource()),
		config: KeySpaceConfig{
			MaxBytes:     size.MB,
			DefaultTTL:   time.Second,
			ReadTimeout:  2 * time.Second,
			WriteTimeout: 3 * time.Second,
			WarmKeys:     []string{"alpha", "beta"},
		},
	}

	engine.keySpaces.Set("ks", spec)
	engine.keySpaces.Set("empty", nil)
	group := &MockGroup{name: "ks"}
	group.stats.Gets.Add(2)
	engine.groups.Set("ks", group)

	snapshots := engine.snapshotKeySpaces()
	require.Len(t, snapshots, 1)

	var snapshot admin.KeySpaceSnapshot
	for _, ks := range snapshots {
		if ks.Name == "ks" {
			snapshot = ks
		}
	}

	require.Equal(t, int64(size.MB), snapshot.MaxBytes)
	require.Equal(t, time.Second, snapshot.DefaultTTL)
	require.Equal(t, 2*time.Second, snapshot.ReadTimeout)
	require.Equal(t, 3*time.Second, snapshot.WriteTimeout)
	require.NotNil(t, snapshot.Stats)
	require.Equal(t, int64(2), snapshot.Stats.Gets)

	snapshot.WarmKeys[0] = "mutated"
	require.Equal(t, "alpha", spec.config.WarmKeys[0])
}

func TestSnapshotKeySpacesWithoutStats(t *testing.T) {
	engine, _, _ := newMockEngine(t, mockDiscovery.NewProvider(t), NewMockKeySpace("custom", size.MB, NewMockDataSource()))
	engine.keySpaces.Set("custom", &keySpaceWrapper{
		keyspace: NewMockKeySpace("custom", size.MB, NewMockDataSource()),
		config: KeySpaceConfig{
			MaxBytes: size.MB,
		},
	})
	engine.groups.Set("custom", &minimalGroup{name: "custom", main: 12, hot: 3})

	snapshots := engine.snapshotKeySpaces()
	require.Len(t, snapshots, 1)

	require.Equal(t, "custom", snapshots[0].Name)
	require.Equal(t, int64(12), snapshots[0].MainCacheBytes)
	require.Equal(t, int64(3), snapshots[0].HotCacheBytes)
	require.Nil(t, snapshots[0].Stats)
}

func TestSnapshotPeersIncludesHostAndRemote(t *testing.T) {
	engine, _, mlist := newMockEngine(t, mockDiscovery.NewProvider(t), NewMockKeySpace("ks", size.MB, NewMockDataSource()))

	hostMeta, err := json.Marshal(&Peer{
		BindAddr:      engine.hostNode.BindAddr,
		BindPort:      engine.hostNode.BindPort,
		DiscoveryPort: engine.hostNode.DiscoveryPort,
	})
	require.NoError(t, err)

	remoteMeta, err := json.Marshal(&Peer{BindAddr: "127.0.0.2", BindPort: 9000, DiscoveryPort: 9001})
	require.NoError(t, err)

	mlist.nodes = []*memberlist.Node{
		{Meta: hostMeta},
		{Meta: remoteMeta},
	}

	peers, err := engine.snapshotPeers()
	require.NoError(t, err)
	require.Len(t, peers, 2)
	require.True(t, peers[0].IsSelf)
	require.Equal(t, "127.0.0.2:9000", peers[1].Address())
	require.False(t, peers[1].IsSelf)
}

func TestSnapshotPeersInvalidMeta(t *testing.T) {
	engine, _, mlist := newMockEngine(t, mockDiscovery.NewProvider(t), NewMockKeySpace("ks", size.MB, NewMockDataSource()))
	mlist.nodes = []*memberlist.Node{{Meta: []byte("bad")}}

	_, err := engine.snapshotPeers()
	require.Error(t, err)
}

func TestEngineInstrumentationNoop(t *testing.T) {
	cfg := NewConfig(MockProvider{}, []KeySpace{NewMockKeySpace("ks", size.MB, NewMockDataSource())})
	inst := newInstrumentation(cfg)
	require.NotNil(t, inst)

	ctx, end := inst.start(context.Background(), "get", "ks")
	require.NotNil(t, ctx)
	end(nil)
	end(errors.New("fail"))
}

func TestEngineInstrumentationWithProviders(t *testing.T) {
	traceCfg := otel.NewTracerConfig(
		otel.WithTracerProvider(tracenoop.NewTracerProvider()),
		otel.WithAttributes(attribute.String("env", "test")),
	)
	metricCfg := otel.NewMetricConfig(
		otel.WithMeterProvider(metricnoop.NewMeterProvider()),
	)
	cfg := NewConfig(
		MockProvider{},
		[]KeySpace{NewMockKeySpace("ks", size.MB, NewMockDataSource())},
		WithTracing(traceCfg),
		WithMetrics(metricCfg),
	)

	inst := newInstrumentation(cfg)
	require.NotNil(t, inst)

	ctx, end := inst.start(context.Background(), "put", "ks")
	require.NotNil(t, ctx)
	end(nil)
	end(errors.New("fail"))
}

func TestCollectWarmupKeys(t *testing.T) {
	engine, _, _ := newMockEngine(t, mockDiscovery.NewProvider(t), NewMockKeySpace("ks", size.MB, NewMockDataSource()))
	cfg := warmup.Config{MaxHotKeys: 2, MinHits: 1, Concurrency: 1, Timeout: time.Second, WarmOnJoin: true, WarmOnLeave: true}
	normalized := cfg.Normalize()

	engine.warmupConfig = &normalized
	engine.hotKeys = warmup.NewTracker(normalized.MaxHotKeys)
	engine.hotKeys.Record("ks", "hot1")
	engine.hotKeys.Record("ks", "hot1")
	engine.hotKeys.Record("ks", "hot2")

	spec := &keySpaceWrapper{
		keyspace: NewMockKeySpace("ks", size.MB, NewMockDataSource()),
		config: KeySpaceConfig{
			WarmKeys: []string{"static"},
		},
	}

	keys := engine.collectWarmupKeys("ks", spec, normalized)
	require.ElementsMatch(t, []string{"static", "hot1", "hot2"}, keys)
}

func TestPrefetchKeysHonorsReadTimeout(t *testing.T) {
	engine, _, _ := newMockEngine(t, mockDiscovery.NewProvider(t), NewMockKeySpace("ks", size.MB, NewMockDataSource()))
	recorder := &deadlineRecorder{}
	group := &MockGroup{
		name: "ks",
		getter: groupcache.GetterFunc(func(ctx context.Context, key string, dest transport.Sink) error {
			if deadline, ok := ctx.Deadline(); ok {
				recorder.record(time.Until(deadline))
			}
			return dest.SetBytes([]byte(key), time.Time{})
		}),
	}
	engine.groups.Set("ks", group)

	specTimeout := 100 * time.Millisecond
	spec := &keySpaceWrapper{
		keyspace: NewMockKeySpace("ks", size.MB, NewMockDataSource()),
		config: KeySpaceConfig{
			ReadTimeout: specTimeout,
		},
	}

	engine.prefetchKeys("ks", spec, []string{"a", "b"}, warmup.Config{
		Concurrency: 2,
		Timeout:     500 * time.Millisecond,
	})

	require.ElementsMatch(t, []string{"a", "b"}, group.GetCalls())
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	require.Len(t, recorder.deadlines, 2)
	for _, deadline := range recorder.deadlines {
		require.Greater(t, deadline, time.Duration(0))
		require.LessOrEqual(t, deadline, specTimeout)
	}
}

func TestWarmupOnClusterEventRespectsConfig(t *testing.T) {
	engine, _, _ := newMockEngine(t, mockDiscovery.NewProvider(t), NewMockKeySpace("ks", size.MB, NewMockDataSource()))
	spec := &keySpaceWrapper{
		keyspace: NewMockKeySpace("ks", size.MB, NewMockDataSource()),
		config: KeySpaceConfig{
			WarmKeys: []string{"hot"},
		},
	}
	engine.keySpaces.Set("ks", spec)
	group := &MockGroup{name: "ks"}
	engine.groups.Set("ks", group)

	cfg := warmup.Config{
		MaxHotKeys:  10,
		MinHits:     1,
		Concurrency: 1,
		Timeout:     50 * time.Millisecond,
		WarmOnLeave: true,
	}
	normalized := cfg.Normalize()
	engine.warmupConfig = &normalized

	engine.warmupOnClusterEvent(memberlist.NodeJoin)
	time.Sleep(50 * time.Millisecond)
	require.Empty(t, group.GetCalls())

	engine.warmupOnClusterEvent(memberlist.NodeLeave)
	require.Eventually(t, func() bool {
		return group.GetCallsLen() == 1
	}, 500*time.Millisecond, 20*time.Millisecond)
}

func extractHTTPOpts(tp *transport.HttpTransport) transport.HttpTransportOptions {
	val := reflect.ValueOf(tp).Elem().FieldByName("opts")
	return reflect.NewAt(val.Type(), unsafe.Pointer(val.UnsafeAddr())).Elem().Interface().(transport.HttpTransportOptions)
}
