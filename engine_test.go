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
	"fmt"
	"net"
	"reflect"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/groupcache/groupcache-go/v3"
	"github.com/groupcache/groupcache-go/v3/transport"
	"github.com/groupcache/groupcache-go/v3/transport/peer"
	"github.com/hashicorp/memberlist"
	"github.com/kapetan-io/tackle/autotls"
	"github.com/stretchr/testify/require"
	"github.com/travisjeffery/go-dynaport"

	"github.com/tochemey/distcache/discovery"
	"github.com/tochemey/distcache/discovery/nats"
	"github.com/tochemey/distcache/internal/members"
	"github.com/tochemey/distcache/internal/pause"
	"github.com/tochemey/distcache/internal/size"
	"github.com/tochemey/distcache/internal/syncmap"
	"github.com/tochemey/distcache/log"
	mockDiscovery "github.com/tochemey/distcache/mocks/discovery"
	mocks "github.com/tochemey/distcache/mocks/discovery"
	"github.com/tochemey/distcache/otel"
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
		require.Len(t, g.removeKeysCalls, 1)
		require.Equal(t, []string{"a"}, g.removeKeysCalls[0])
		require.Empty(t, g.removeCalls)
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

func extractHTTPOpts(tp *transport.HttpTransport) transport.HttpTransportOptions {
	val := reflect.ValueOf(tp).Elem().FieldByName("opts")
	return reflect.NewAt(val.Type(), unsafe.Pointer(val.UnsafeAddr())).Elem().Interface().(transport.HttpTransportOptions)
}

type MockErrDataSource struct {
	err error
}

func (s *MockErrDataSource) Fetch(context.Context, string) ([]byte, error) { return nil, s.err }

type unitMocks struct {
	daemon     *MockDaemon
	memberlist *MockMemberlist
}

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

type MockDaemon struct {
	address        string
	setPeersCalls  [][]peer.Info
	groups         map[string]transport.Group
	setPeersErr    error
	newGroupErr    error
	shutdownCalled bool
	mu             sync.Mutex
}

func (x *MockDaemon) SetPeers(_ context.Context, infos []peer.Info) error {
	x.mu.Lock()
	x.setPeersCalls = append(x.setPeersCalls, infos)
	x.mu.Unlock()
	return x.setPeersErr
}

func (x *MockDaemon) NewGroup(name string, _ int64, getter groupcache.Getter) (transport.Group, error) {
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

type MockGroup struct {
	name            string
	data            map[string][]byte
	setErr          error
	getErr          error
	removeErr       error
	removeKeysErr   error
	removeCalls     []string
	removeKeysCalls [][]string
	getter          groupcache.Getter
}

func (g *MockGroup) Set(_ context.Context, key string, value []byte, _ time.Time, _ bool) error {
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
	if g.getErr != nil {
		return g.getErr
	}
	if g.getter != nil {
		return g.getter.Get(ctx, key, dest)
	}
	value, ok := g.data[key]
	if !ok {
		return fmt.Errorf("not found")
	}
	return dest.SetBytes(value, time.Time{})
}

func (g *MockGroup) Remove(_ context.Context, key string) error {
	g.removeCalls = append(g.removeCalls, key)
	if g.removeErr != nil {
		return g.removeErr
	}
	delete(g.data, key)
	return nil
}

func (g *MockGroup) UsedBytes() (int64, int64) { return 0, 0 }
func (g *MockGroup) Name() string              { return g.name }
func (g *MockGroup) RemoveKeys(_ context.Context, keys ...string) error {
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
