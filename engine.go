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
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	goset "github.com/deckarep/golang-set/v2"
	"github.com/flowchartsman/retry"
	"github.com/groupcache/groupcache-go/v3"
	"github.com/groupcache/groupcache-go/v3/transport"
	"github.com/groupcache/groupcache-go/v3/transport/peer"
	"github.com/hashicorp/memberlist"

	"github.com/tochemey/distcache/internal/errorschain"
	"github.com/tochemey/distcache/internal/members"
	"github.com/tochemey/distcache/internal/syncmap"
	"github.com/tochemey/distcache/internal/tcp"
)

// Engine defines a set of operations for managing key/value pairs in a cache or store,
// organized by keyspace. It supports inserting, retrieving, listing, and deleting
// individual or multiple key/value entries. Each method accepts a context.Context
// to allow for cancellation, timeouts, and passing request-scoped values.
//
// The methods include:
//
//   - Put: Stores a single key/value pair in the specified .
//   - PutMany: Stores multiple key/value pairs in one operation.
//   - Get: Retrieves a specific key/value pair from the cache using its key.
//   - Delete: Removes a specific key/value pair from the cache using its key.
//   - DeleteMany: Removes multiple key/value pairs from the cache given their keys.
//   - DeleteKeySpace: Remove a given keySpace from the cache
//   - DeleteKeyspaces: Remove a set of keySpaces from the cache
type Engine interface {
	// Put stores a single key/value pair in the cache.
	//
	// Parameters:
	//   - ctx: The context for cancellation and deadlines.
	//   - keyspace: The keyspace in which to store the key/value pair.
	//   - entry: The key/value pair to store with an optional expiration time.
	//     If Expiry is set to the zero value (time.Time{}), the entry does not expire.
	//
	// Returns an error if the operation fails.
	Put(ctx context.Context, keyspace string, entry *Entry) error

	// PutMany stores multiple key/value pairs in the cache.
	//
	// Parameters:
	//   - ctx: The context for cancellation and deadlines.
	//   - keyspace: The keyspace in which to store the key/value pairs.
	//   - kvs: A slice of key/value pairs to store.
	//
	// Returns an error if the operation fails.
	PutMany(ctx context.Context, keyspace string, entries []*Entry) error

	// Get retrieves a specific key/value pair from the cache.
	//
	// Parameters:
	//   - ctx: The context for cancellation and deadlines.
	//   - keyspace: The keyspace from which to retrieve the key/value pair.
	//   - key: The key identifying the desired key/value pair.
	//
	// Returns the key/value pair if found, or an error if the operation fails or the key does not exist.
	Get(ctx context.Context, keyspace string, key string) (*KV, error)

	// Delete removes a specific key/value pair from the cache.
	//
	// Parameters:
	//   - ctx: The context for cancellation and deadlines.
	//   - keyspace: The keyspace from which to delete the key/value pair.
	//   - key: The key identifying the key/value pair to be deleted.
	//
	// Returns an error if the operation fails.
	Delete(ctx context.Context, keyspace string, key string) error

	// DeleteMany removes multiple key/value pairs from the cache.
	//
	// Parameters:
	//   - ctx: The context for cancellation and deadlines.
	//   - keyspace: The keyspace from which to delete the key/value pairs.
	//   - keys: A slice of keys identifying the key/value pairs to be deleted.
	//
	// Returns an error if the operation fails.
	DeleteMany(ctx context.Context, keyspace string, keys []string) error

	// Start initializes and runs the distributed cache engine.
	// It performs the following operations:
	//   - Discovers existing nodes in the system.
	//   - Joins an existing cluster or forms a new one if none exists.
	//   - Starts the cache engine to handle key-value storage and retrieval.
	//   - Builds the configured keyspaces, preparing them for use.
	//
	// Parameters:
	//   - ctx: A context used to manage initialization timeouts or cancellations.
	//
	// Returns:
	//   - err: An error if the startup process fails, otherwise nil.
	Start(ctx context.Context) (err error)

	// Stop gracefully shuts down the distributed cache engine.
	// It ensures that any ongoing operations complete and that the cluster
	// state is properly maintained before termination.
	//
	// Parameters:
	//   - ctx: A context used to manage shutdown timeouts or cancellations.
	//
	// Returns:
	//   - error: An error if the shutdown process encounters issues, otherwise nil.
	Stop(ctx context.Context) error

	// DeleteKeySpace delete a given keySpace from the cache.
	//
	// Parameters:
	//   - ctx: The context for cancellation and deadlines.
	//   - keyspace: The keyspace from which to delete the key/value pairs.
	//
	// Returns an error if the operation fails.
	DeleteKeySpace(ctx context.Context, keyspace string) error

	// DeleteKeyspaces removes multiple keyspaces from the cache.
	//
	// Parameters:
	//   - ctx: The context for cancellation and deadlines.
	//   - keyspaces: A slice of keyspaces to be deleted.
	//
	// Returns an error if the operation fails.
	DeleteKeyspaces(ctx context.Context, keyspaces []string) error

	// KeySpaces returns the list of available KeySpaces from the cache.
	//
	// Returns an empty list if there are no keyspaces
	KeySpaces() []string
}

type engine struct {
	config                *Config
	hostNode              *Peer
	mconfig               *memberlist.Config
	mlist                 *memberlist.Memberlist
	started               atomic.Bool
	stopEventsListenerSig chan struct{}
	eventsLock            sync.Mutex
	lock                  sync.Mutex
	daemon                *groupcache.Daemon
	groups                *syncmap.SyncMap[string, groupcache.Group]
}

var _ Engine = (*engine)(nil)

// NewEngine creates and initializes a new distributed cache engine based on the provided configuration.
//
// It sets up the necessary components required for caching, including:
//   - Cluster discovery and membership management.
//   - Keyspace initialization.
//   - Cache storage backend configuration.
//   - Any additional settings defined in the provided configuration.
//
// Parameters:
//   - config: A pointer to a Config struct containing the necessary settings for initializing the cache engine.
//
// Returns:
//   - Engine: An instance of the initialized cache engine.
//   - error: An error if the engine fails to initialize due to misconfiguration or other issues.
func NewEngine(config *Config) (Engine, error) {
	// get a bindIP
	addr := net.JoinHostPort(config.BindAddr(), strconv.Itoa(config.BindPort()))
	bindIP, err := tcp.GetBindIP(config.Interface(), addr)
	if err != nil {
		return nil, fmt.Errorf("failed to get bindIP: %w", err)
	}

	hostNode := &Peer{
		BindAddr:      bindIP,
		BindPort:      config.BindPort(),
		DiscoveryPort: config.DiscoveryPort(),
		IsSelf:        true,
	}

	return &engine{
		config:                config,
		hostNode:              hostNode,
		stopEventsListenerSig: make(chan struct{}, 1),
		groups:                syncmap.New[string, groupcache.Group](),
	}, nil
}

// Start initializes and runs the distributed cache engine.
// It performs the following operations:
//   - Discovers existing nodes in the system.
//   - Joins an existing cluster or forms a new one if none exists.
//   - Starts the cache engine to handle key-value storage and retrieval.
//   - Builds the configured keyspaces, preparing them for use.
//
// Parameters:
//   - ctx: A context used to manage initialization timeouts or cancellations.
//
// Returns:
//   - err: An error if the startup process fails, otherwise nil.
func (x *engine) Start(ctx context.Context) (err error) {
	x.lock.Lock()
	x.config.Logger().Infof("Cache Engine starting on [%s/%s, host=%s]...", runtime.GOOS, runtime.GOARCH, x.hostNode.Address())

	if err := x.setupMemberlistConfig(); err != nil {
		x.lock.Unlock()
		return err
	}
	x.lock.Unlock()

	if err := x.bootstrapCluster(); err != nil {
		return err
	}

	peers, _ := x.peers()

	x.lock.Lock()
	ctx, cancel := context.WithTimeout(ctx, x.config.BootstrapTimeout())
	defer cancel()

	if err := x.startDaemon(ctx); err != nil {
		x.lock.Unlock()
		return err
	}

	if err := x.applyPeers(ctx, peers); err != nil {
		x.lock.Unlock()
		return err
	}

	if err := x.createGroups(); err != nil {
		x.lock.Unlock()
		return err
	}

	eventsCh := x.configureEventDelegate()
	x.started.Store(true)
	x.lock.Unlock()

	go x.eventsListener(eventsCh)

	x.config.Logger().Infof("Cache engine started: host=%s.", x.hostNode.Address())
	return nil
}

func (x *engine) startDaemon(ctx context.Context) error {
	hashFn := func(data []byte) uint64 {
		return x.config.Hasher().HashCode(data)
	}

	client := http.DefaultClient
	scheme := "http"
	if x.config.TLSInfo() != nil {
		scheme = "https"
		client = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: x.config.TLSInfo().ClientTLS,
			},
		}
	}

	transportOpts := transport.HttpTransportOptions{
		Client: client,
		Scheme: scheme,
		Logger: newCacheLog(x.config.Logger()),
	}

	if x.config.TLSInfo() != nil {
		transportOpts.TLSConfig = x.config.TLSInfo().ServerTLS
	}

	daemon, err := groupcache.ListenAndServe(ctx, x.hostNode.Address(), groupcache.Options{
		HashFn:    hashFn,
		Replicas:  x.config.ReplicaCount(),
		Logger:    newCacheLog(x.config.Logger()),
		Transport: transport.NewHttpTransport(transportOpts),
	})
	if err != nil {
		return fmt.Errorf("failed to start engine daemon: %w", err)
	}

	x.daemon = daemon
	return nil
}

func (x *engine) applyPeers(ctx context.Context, peers []*Peer) error {
	peerInfos := goset.NewSet[peer.Info]()
	peerInfos.Add(peer.Info{
		Address: x.hostNode.Address(),
		IsSelf:  x.hostNode.IsSelf,
	})

	for _, xpeer := range peers {
		bindAddr := net.JoinHostPort(xpeer.BindAddr, strconv.Itoa(xpeer.BindPort))
		peerInfos.Add(peer.Info{
			Address: bindAddr,
			IsSelf:  xpeer.IsSelf,
		})
	}

	if err := x.daemon.SetPeers(ctx, peerInfos.ToSlice()); err != nil {
		return fmt.Errorf("failed to set peers: %w", err)
	}

	return nil
}

func (x *engine) createGroups() error {
	keySpaces := x.config.KeySpaces()
	for _, keySpace := range keySpaces {
		group, err := x.daemon.NewGroup(keySpace.Name(), keySpace.MaxBytes(), groupcache.GetterFunc(
			func(ctx context.Context, id string, dest transport.Sink) error {
				bytea, err := keySpace.DataSource().Fetch(ctx, id)
				if err != nil {
					return err
				}
				expiredAt := keySpace.ExpiresAt(ctx, id)
				return dest.SetBytes(bytea, expiredAt)
			}))

		if err != nil {
			return fmt.Errorf("failed to create group: %w", err)
		}

		x.groups.Set(group.Name(), group)
	}
	return nil
}

func (x *engine) configureEventDelegate() chan memberlist.NodeEvent {
	eventsCh := make(chan memberlist.NodeEvent, 256)
	x.mconfig.Events = &memberlist.ChannelEventDelegate{
		Ch: eventsCh,
	}
	return eventsCh
}

func (x *engine) setupMemberlistConfig() error {
	mtConfig := members.TransportConfig{
		BindAddrs:          []string{x.hostNode.BindAddr},
		BindPort:           x.hostNode.DiscoveryPort,
		PacketDialTimeout:  5 * time.Second,
		PacketWriteTimeout: 5 * time.Second,
		Logger:             x.config.Logger(),
		DebugEnabled:       false,
	}

	if x.config.TLSInfo() != nil {
		mtConfig.TLSEnabled = true
		mtConfig.TLS = x.config.TLSInfo().ServerTLS
	}

	mtransport, err := members.NewTransport(mtConfig)
	if err != nil {
		x.config.Logger().Errorf("failed to create memberlist TCP transport: %v", err)
		return err
	}

	meta, _ := json.Marshal(x.hostNode)
	x.mconfig = x.newMemberlistConfig(mtransport, meta)
	return nil
}

func (x *engine) newMemberlistConfig(transport memberlist.Transport, meta []byte) *memberlist.Config {
	config := memberlist.DefaultLANConfig()
	config.BindAddr = x.hostNode.BindAddr
	config.BindPort = x.hostNode.DiscoveryPort
	config.AdvertisePort = x.hostNode.DiscoveryPort
	config.LogOutput = newLogWriter(x.config.Logger())
	config.Name = net.JoinHostPort(x.hostNode.BindAddr, strconv.Itoa(x.hostNode.DiscoveryPort))
	config.UDPBufferSize = 10 * 1024 * 1024
	config.ProbeInterval = 5 * time.Second
	config.ProbeTimeout = 2 * time.Second
	config.DisableTcpPings = true
	config.Transport = transport
	config.Delegate = newDelegate(meta)
	return config
}

func (x *engine) bootstrapCluster() error {
	provider := x.config.DiscoveryProvider()
	return errorschain.
		New(errorschain.WithFailFast()).
		AddRunner(provider.Initialize).
		AddRunner(provider.Register).
		AddContextRunner(x.joinCluster).
		Run()
}

// Stop gracefully shuts down the distributed cache engine.
// It ensures that any ongoing operations complete and that the cluster
// state is properly maintained before termination.
//
// Parameters:
//   - ctx: A context used to manage shutdown timeouts or cancellations.
//
// Returns:
//   - error: An error if the shutdown process encounters issues, otherwise nil.
func (x *engine) Stop(ctx context.Context) error {
	x.lock.Lock()
	defer x.lock.Unlock()

	x.config.Logger().Infof("Cache engine on host=%s stopping...", x.hostNode.Address())

	if !x.started.Load() {
		return nil
	}

	// stop the events loop
	close(x.stopEventsListenerSig)

	provider := x.config.DiscoveryProvider()
	if err := errorschain.
		New(errorschain.WithFailFast()).
		AddRunner(func() error { return x.mlist.Leave(x.config.ShutdownTimeout()) }).
		AddRunner(provider.Deregister).
		AddRunner(provider.Close).
		AddRunner(x.mlist.Shutdown).
		AddContextRunner(x.daemon.Shutdown).
		Run(); err != nil {
		return err
	}

	x.config.Logger().Infof("Cache engine on host=%s stopped.", x.hostNode.Address())
	return nil
}

// Put stores a single key/value pair in the cache.
//
// Parameters:
//   - ctx: The context for cancellation and deadlines.
//   - keyspace: The keyspace in which to store the key/value pair.
//   - entry: The key/value pair to store with an optional expiration time.
//     If Expiry is set to the zero value (time.Time{}), the entry does not expire.
//
// Returns an error if the operation fails.
func (x *engine) Put(ctx context.Context, keyspace string, entry *Entry) error {
	x.lock.Lock()
	defer x.lock.Unlock()

	group, ok := x.groups.Get(keyspace)
	if !ok {
		return ErrKeySpaceNotFound
	}

	ctx, cancel := context.WithTimeout(ctx, x.config.WriteTimeout())
	defer cancel()

	return group.Set(ctx, entry.Key, entry.Value, entry.Expiry, true)
}

// PutMany stores multiple key/value pairs in the cache.
//
// Parameters:
//   - ctx: The context for cancellation and deadlines.
//   - keyspace: The keyspace in which to store the key/value pairs.
//   - kvs: A slice of key/value pairs to store.
//
// Returns an error if the operation fails.
func (x *engine) PutMany(ctx context.Context, keyspace string, entries []*Entry) error {
	x.lock.Lock()
	defer x.lock.Unlock()

	group, ok := x.groups.Get(keyspace)
	if !ok {
		return ErrKeySpaceNotFound
	}

	for _, entry := range entries {
		ctx, cancel := context.WithTimeout(ctx, x.config.WriteTimeout())
		if err := group.Set(ctx, entry.Key, entry.Value, entry.Expiry, true); err != nil {
			cancel()
			return err
		}
		cancel()
	}

	return nil
}

// Get retrieves a specific key/value pair from the cache.
//
// Parameters:
//   - ctx: The context for cancellation and deadlines.
//   - keyspace: The keyspace from which to retrieve the key/value pair.
//   - key: The key identifying the desired key/value pair.
//
// Returns the key/value pair if found, or an error if the operation fails or the key does not exist.
func (x *engine) Get(ctx context.Context, keyspace string, key string) (*KV, error) {
	x.lock.Lock()
	defer x.lock.Unlock()

	group, ok := x.groups.Get(keyspace)
	if !ok {
		return nil, ErrKeySpaceNotFound
	}

	ctx, cancel := context.WithTimeout(ctx, x.config.ReadTimeout())
	defer cancel()

	var value []byte
	if err := group.Get(ctx, key, transport.AllocatingByteSliceSink(&value)); err != nil {
		return nil, err
	}

	return &KV{
		Key:   key,
		Value: value,
	}, nil
}

// Delete removes a specific key/value pair from the cache.
//
// Parameters:
//   - ctx: The context for cancellation and deadlines.
//   - keyspace: The keyspace from which to delete the key/value pair.
//   - key: The key identifying the key/value pair to be deleted.
//
// Returns an error if the operation fails.
func (x *engine) Delete(ctx context.Context, keyspace string, key string) error {
	x.lock.Lock()
	defer x.lock.Unlock()

	group, ok := x.groups.Get(keyspace)
	if !ok {
		return ErrKeySpaceNotFound
	}

	ctx, cancel := context.WithTimeout(ctx, x.config.WriteTimeout())
	defer cancel()

	return group.Remove(ctx, key)
}

// DeleteMany removes multiple key/value pairs from the cache.
//
// Parameters:
//   - ctx: The context for cancellation and deadlines.
//   - keyspace: The keyspace from which to delete the key/value pairs.
//   - keys: A slice of keys identifying the key/value pairs to be deleted.
//
// Returns an error if the operation fails.
func (x *engine) DeleteMany(ctx context.Context, keyspace string, keys []string) error {
	x.lock.Lock()
	defer x.lock.Unlock()

	group, ok := x.groups.Get(keyspace)
	if !ok {
		return ErrKeySpaceNotFound
	}

	for _, key := range keys {
		ctx, cancel := context.WithTimeout(ctx, x.config.WriteTimeout())
		if err := group.Remove(ctx, key); err != nil {
			cancel()
			return err
		}
		cancel()
	}
	return nil
}

// KeySpaces returns the list of available KeySpaces from the cache.
//
// Returns an empty list if there are no keyspaces
func (x *engine) KeySpaces() []string {
	x.lock.Lock()
	defer x.lock.Unlock()
	return x.groups.Keys()
}

// DeleteKeySpace delete a given keySpace from the cache.
//
// Parameters:
//   - ctx: The context for cancellation and deadlines.
//   - keyspace: The keyspace from which to delete the key/value pairs.
//
// Returns an error if the operation fails.
func (x *engine) DeleteKeySpace(ctx context.Context, keyspace string) error {
	x.lock.Lock()
	defer x.lock.Unlock()

	group, ok := x.groups.Get(keyspace)
	if !ok {
		return ErrKeySpaceNotFound
	}

	_, cancel := context.WithTimeout(ctx, x.config.WriteTimeout())
	defer cancel()

	x.daemon.RemoveGroup(group.Name())
	x.groups.Delete(keyspace)
	return nil
}

// DeleteKeyspaces removes multiple keyspaces from the cache.
//
// Parameters:
//   - ctx: The context for cancellation and deadlines.
//   - keyspaces: A slice of keyspaces to be deleted.
//
// Returns an error if the operation fails.
func (x *engine) DeleteKeyspaces(ctx context.Context, keyspaces []string) error {
	x.lock.Lock()
	defer x.lock.Unlock()

	_, cancel := context.WithTimeout(ctx, x.config.WriteTimeout())
	defer cancel()

	for _, keyspace := range keyspaces {
		if group, ok := x.groups.Get(keyspace); ok {
			x.daemon.RemoveGroup(group.Name())
			x.groups.Delete(keyspace)
		}
	}
	return nil
}

// peers returns a channel containing the list of peers at a given time
func (x *engine) peers() ([]*Peer, error) {
	x.lock.Lock()
	members := x.mlist.Members()
	x.lock.Unlock()
	peers := make([]*Peer, 0, len(members))
	for _, member := range members {
		peer, err := fromBytes(member.Meta)
		if err != nil {
			return nil, err
		}

		// exclude the host node from the list of found peers
		if peer != nil && peer.Address() != x.hostNode.Address() {
			peers = append(peers, peer)
		}
	}
	return peers, nil
}

// eventsListener listens to cluster events
func (x *engine) eventsListener(eventsChan chan memberlist.NodeEvent) {
	for {
		select {
		case <-x.stopEventsListenerSig:
			// finish listening to cluster events
			return
		case event := <-eventsChan:
			node, err := fromBytes(event.Node.Meta)
			if err != nil {
				x.config.Logger().Errorf("Cache on host=%s failed to decode event: %v", err)
				continue
			}

			// skip self on cluster event
			if node.Address() == x.hostNode.Address() {
				x.config.Logger().Warnf("Cache on host=%s is already in use", node.Address())
				continue
			}

			ctx := context.Background()
			// we need to add the new peers
			currentPeers, _ := x.peers()
			peersSet := goset.NewSet[peer.Info]()
			peersSet.Add(peer.Info{
				Address: x.hostNode.Address(),
				IsSelf:  true,
			})

			for _, xpeer := range currentPeers {
				peersSet.Add(peer.Info{
					Address: xpeer.Address(),
					IsSelf:  xpeer.IsSelf,
				})
			}

			switch event.Event {
			case memberlist.NodeJoin:
				x.config.Logger().Infof("Cache on host=%s has noticed node=%s has joined the cluster", x.hostNode.Address(), node.Address())

				// add the joined node to the peers list
				// and set it to the daemon
				x.eventsLock.Lock()
				peersSet.Add(peer.Info{
					Address: node.Address(),
					IsSelf:  node.IsSelf,
				})

				_ = x.daemon.SetPeers(ctx, peersSet.ToSlice())
				x.eventsLock.Unlock()

			case memberlist.NodeLeave:
				x.config.Logger().Infof("Cache on host=%s has noticed node=%s has left the cluster", x.hostNode.Address(), node.Address())

				// remove the left node from the peers list
				// and set it to the daemon
				x.eventsLock.Lock()
				peersSet.Remove(peer.Info{
					Address: node.Address(),
					IsSelf:  node.IsSelf,
				})
				_ = x.daemon.SetPeers(ctx, peersSet.ToSlice())
				x.eventsLock.Unlock()

			case memberlist.NodeUpdate:
				// TODO: need to handle that later
				continue
			}
		}
	}
}

// joinCluster attempts to join an existing cluster if peers are provided
func (x *engine) joinCluster(ctx context.Context) error {
	var err error
	x.mlist, err = memberlist.Create(x.mconfig)
	if err != nil {
		return err
	}

	joinTimeout := computeTimeout(x.config.MaxJoinAttempts(), x.config.JoinRetryInterval())
	ctx, cancel := context.WithTimeout(ctx, joinTimeout)

	var peers []string
	retrier := retry.NewRetrier(x.config.MaxJoinAttempts(), x.config.JoinRetryInterval(), x.config.JoinRetryInterval())
	if err := retrier.RunContext(ctx, func(_ context.Context) error {
		peers, err = x.config.DiscoveryProvider().DiscoverPeers()
		if err != nil {
			return err
		}
		return nil
	}); err != nil {
		cancel()
		return err
	}

	if len(peers) > 0 {
		// check whether the cluster quorum is met to operate
		if x.config.MinimumPeersQuorum() > len(peers) {
			cancel()
			return ErrClusterQuorum
		}
		cancel()

		// attempt to join
		ctx, cancel = context.WithTimeout(ctx, joinTimeout)
		joinRetrier := retry.NewRetrier(x.config.MaxJoinAttempts(), x.config.JoinRetryInterval(), x.config.JoinRetryInterval())
		if err := joinRetrier.RunContext(ctx, func(_ context.Context) error {
			if _, err := x.mlist.Join(peers); err != nil {
				return err
			}
			return nil
		}); err != nil {
			cancel()
			return err
		}

		x.config.Logger().Infof("Cache on host=%s joined cluster of [%s]", x.hostNode.Address(), strings.Join(peers, ","))
	}

	cancel()
	return nil
}

// computeTimeout calculates the approximate timeout given maxAttempts and retryInterval.
func computeTimeout(maxAttempts int, retryInterval time.Duration) time.Duration {
	return time.Duration(maxAttempts-1) * retryInterval
}
