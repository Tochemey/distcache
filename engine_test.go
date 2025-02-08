/*
 * MIT License
 *
 * Copyright (c) 2025 Arsene Tochemey Gandote
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

package distcache

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/travisjeffery/go-dynaport"

	"github.com/tochemey/distcache/discovery"
	"github.com/tochemey/distcache/discovery/nats"
	"github.com/tochemey/distcache/internal/size"
	"github.com/tochemey/distcache/internal/util"
)

func TestEngine(t *testing.T) {
	t.Run("With caching operations in cluster", func(t *testing.T) {
		ctx := context.Background()

		srv := StartNatsServer(t)
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

		srv := StartNatsServer(t)
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
		util.Pause(time.Minute)

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
}

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
		WithDiscoveryPort(discoveryPort),
		WithBindPort(bindPort))

	engine, err := NewEngine(config)
	require.NoError(t, err)
	require.NotNil(t, engine)

	// start the node
	require.NoError(t, engine.Start(ctx))

	util.Pause(500 * time.Millisecond)

	// return the cluster startNode
	return engine, provider
}
