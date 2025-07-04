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

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/travisjeffery/go-dynaport"

	"github.com/tochemey/distcache"
	"github.com/tochemey/distcache/discovery/nats"
	"github.com/tochemey/distcache/internal/size"
	"github.com/tochemey/distcache/internal/syncmap"
	"github.com/tochemey/distcache/log"
)

func main() {
	ctx := context.Background()

	ports := dynaport.Get(2)
	discoveryPort := ports[0]
	bindPort := ports[1]

	logger := log.DefaultLogger

	// start the nats server
	server, err := startNatsServer()
	if err != nil {
		logger.Fatal(err)
		os.Exit(1)
	}

	// create an instance of the NATs discovery provider
	// Note: we can also use kubernetes, static, dns or any custom discovery provider
	host := "127.0.0.1"
	provider := nats.NewDiscovery(&nats.Config{
		Server:        fmt.Sprintf("nats://%s", server.Addr().String()),
		Subject:       "example",
		Host:          host,
		DiscoveryPort: discoveryPort,
	})

	// create an instance of the DataSource
	dataSource := NewUsersDataSource()

	// create an instance of the KeySpace
	keySpace := NewUsersKeySpace("users", 20*size.MB, dataSource)
	// You can create different KeySpaces based upon your application needs

	keySpaces := []distcache.KeySpace{keySpace}

	// create an instance of the DistCache configuration
	config := distcache.NewConfig(provider, keySpaces,
		distcache.WithBindAddr(host),
		distcache.WithLogger(logger),
		distcache.WithDiscoveryPort(discoveryPort),
		distcache.WithBindPort(bindPort))

	// create an instance of the Engine
	engine, err := distcache.NewEngine(config)
	if err != nil {
		logger.Fatal(err)
		os.Exit(1)
	}

	// Start the Engine
	if err := engine.Start(ctx); err != nil {
		logger.Fatal(err)
		os.Exit(1)
	}

	// With the Engine started one can:
	//   - Put: Stores a single key/value pair in the specified .
	//   - PutMany: Stores multiple key/value pairs in one operation.
	//   - Get: Retrieves a specific key/value pair from the cache using its key.
	//   - Delete: Removes a specific key/value pair from the cache using its key.
	//   - DeleteMany: Removes multiple key/value pairs from the cache given their keys.
	//   - DeleteKeySpace: Remove a given keySpace from the cache
	//   - DeleteKeyspaces: Remove a set of keySpaces from the cache

	// For instance let us cache a User record
	user := &User{
		ID:   "user1",
		Name: "user",
		Age:  10,
	}

	bytea, _ := json.Marshal(user)

	logger.Infof("caching user=%s", user.Name)
	if err := engine.Put(ctx, keySpace.Name(), &distcache.Entry{
		KV: distcache.KV{
			Key:   user.ID,
			Value: bytea,
		},
		Expiry: time.Time{}, // this means the entry will never expire
	}); err != nil {
		logger.Error(fmt.Errorf("failed to cache user %s: %w", user.Name, err))
	}

	logger.Infof("user=(%s) successfully cached", user.Name)

	// wait for interruption/termination
	notifier := make(chan os.Signal, 1)
	done := make(chan struct{}, 1)
	signal.Notify(notifier, syscall.SIGINT, syscall.SIGTERM)
	// wait for a shutdown signal, and then shutdown
	go func() {
		sig := <-notifier
		logger.Infof("received an interrupt signal (%s) to shutdown", sig.String())
		signal.Stop(notifier)
		done <- struct{}{}
	}()
	<-done

	// stop the distcache Engine
	// Please handle error in a production application to avoid resource leakage
	_ = engine.Stop(ctx)

	pid := os.Getpid()
	// make sure if it is unix init process to exit
	if pid == 1 {
		os.Exit(0)
	}

	process, _ := os.FindProcess(pid)
	switch runtime.GOOS {
	case "windows":
		_ = process.Kill()
	default:
		_ = process.Signal(syscall.SIGTERM)
	}
}

// startNatsServer starts  NATs
func startNatsServer() (*natsserver.Server, error) {
	serv, err := natsserver.NewServer(&natsserver.Options{
		Host: "127.0.0.1",
		Port: -1,
	})

	if err != nil {
		return nil, err
	}

	ready := make(chan bool)
	go func() {
		ready <- true
		serv.Start()
	}()
	<-ready

	if !serv.ReadyForConnections(2 * time.Second) {
		return nil, fmt.Errorf("nats server not ready")
	}

	return serv, nil
}

type User struct {
	ID   string
	Name string
	Age  int
}

type UsersKeySpace struct {
	*sync.RWMutex
	name       string
	maxBytes   int64
	dataSource *UsersDataSource
}

var _ distcache.KeySpace = (*UsersKeySpace)(nil)

func NewUsersKeySpace(name string, maxBytes int64, source *UsersDataSource) *UsersKeySpace {
	return &UsersKeySpace{
		RWMutex:    &sync.RWMutex{},
		name:       name,
		maxBytes:   maxBytes,
		dataSource: source,
	}
}

func (x *UsersKeySpace) Name() string {
	x.RLock()
	defer x.RUnlock()
	return x.name
}

func (x *UsersKeySpace) MaxBytes() int64 {
	x.RLock()
	defer x.RUnlock()
	return x.maxBytes
}

func (x *UsersKeySpace) DataSource() distcache.DataSource {
	x.RLock()
	defer x.RUnlock()
	return x.dataSource
}

func (x *UsersKeySpace) ExpiresAt(ctx context.Context, key string) time.Time {
	x.Lock()
	defer x.Unlock()

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if _, err := x.dataSource.Fetch(ctx, key); err != nil {
		return time.Now().Add(time.Second)
	}

	return time.Time{}
}

type UsersDataSource struct {
	store *syncmap.SyncMap[string, User]
}

var _ distcache.DataSource = (*UsersDataSource)(nil)

func NewUsersDataSource() *UsersDataSource {
	return &UsersDataSource{
		store: syncmap.New[string, User](),
	}
}

func (x *UsersDataSource) Insert(ctx context.Context, users []*User) error {
	_, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	for _, user := range users {
		x.store.Set(user.ID, *user)
	}
	return nil
}

func (x *UsersDataSource) Fetch(ctx context.Context, key string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	user, ok := x.store.Get(key)
	if !ok {
		return nil, errors.New("not found")
	}

	bytea, err := json.Marshal(user)
	if err != nil {
		return nil, err
	}
	return bytea, nil
}
