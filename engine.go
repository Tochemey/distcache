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
}

type engine struct {
	config                *Config
	hostNode              *Peer
	mconfig               *memberlist.Config
	mlist                 *memberlist.Memberlist
	started               *atomic.Bool
	stopEventsListenerSig chan struct{}
	eventsLock            *sync.Mutex
	lock                  *sync.Mutex
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
		started:               new(atomic.Bool),
		stopEventsListenerSig: make(chan struct{}, 1),
		eventsLock:            new(sync.Mutex),
		lock:                  new(sync.Mutex),
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
func (k *engine) Start(ctx context.Context) (err error) {
	k.lock.Lock()
	k.config.Logger().Infof("DistCache Engine starting on [%s/%s, host=%s]...", runtime.GOOS, runtime.GOARCH, k.hostNode.Address())
	// create the memberlist configuration
	k.mconfig = memberlist.DefaultLANConfig()
	k.mconfig.BindAddr = k.hostNode.BindAddr
	k.mconfig.BindPort = k.hostNode.DiscoveryPort
	k.mconfig.AdvertisePort = k.hostNode.DiscoveryPort
	k.mconfig.LogOutput = newLogWriter(k.config.Logger())
	k.mconfig.Name = net.JoinHostPort(k.hostNode.BindAddr, strconv.Itoa(k.hostNode.DiscoveryPort))

	meta, err := json.Marshal(k.hostNode)
	if err != nil {
		return fmt.Errorf("failed to marshal meta data: %w", err)
	}

	k.mconfig.Delegate = newDelegate(meta)
	provider := k.config.DiscoveryProvider()
	k.lock.Unlock()

	// start process
	if err := errorschain.
		New(errorschain.ReturnFirst()).
		AddError(provider.Initialize()).
		AddError(provider.Register()).
		AddError(k.joinCluster(ctx)).
		Error(); err != nil {
		return err
	}

	// get the list of peers
	peers, err := k.peers()
	if err != nil {
		return fmt.Errorf("failed to get peers: %w", err)
	}

	k.lock.Lock()
	// start the daemon
	hashFn := func(data []byte) uint64 {
		return k.config.Hasher().HashCode(data)
	}

	ctx, cancel := context.WithTimeout(ctx, k.config.BootstrapTimeout())
	defer cancel()

	scheme := "http"
	client := http.DefaultClient
	if k.config.TLSConfig() != nil {
		scheme = "https"
		client = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: k.config.TLSConfig(),
			},
		}
	}

	// Explicitly instantiate and use the HTTP transport
	httpTransport := transport.NewHttpTransport(
		transport.HttpTransportOptions{
			Client: client,
			Scheme: scheme,
			Logger: newGLogger(k.config.Logger()),
		},
	)

	daemon, err := groupcache.ListenAndServe(ctx, k.hostNode.Address(), groupcache.Options{
		HashFn:    hashFn,
		Replicas:  k.config.ReplicaCount(),
		Logger:    newGLogger(k.config.Logger()),
		Transport: httpTransport,
	})

	if err != nil {
		k.lock.Unlock()
		return fmt.Errorf("failed to start engine daemon: %w", err)
	}
	k.daemon = daemon

	// set the peers
	peerInfos := goset.NewSet[peer.Info]()
	peerInfos.Add(peer.Info{
		Address: k.hostNode.Address(),
		IsSelf:  k.hostNode.IsSelf,
	})

	for _, xpeer := range peers {
		bindAddr := net.JoinHostPort(xpeer.BindAddr, strconv.Itoa(xpeer.BindPort))
		peerInfos.Add(peer.Info{
			Address: bindAddr,
			IsSelf:  xpeer.IsSelf,
		})
	}

	if err := k.daemon.SetPeers(ctx, peerInfos.ToSlice()); err != nil {
		k.lock.Unlock()
		return fmt.Errorf("failed to set peers: %w", err)
	}

	// set the groups given the keySpaces
	keySpaces := k.config.KeySpaces()
	for _, keySpace := range keySpaces {
		group, err := k.daemon.NewGroup(keySpace.Name(), keySpace.MaxBytes(), groupcache.GetterFunc(
			func(ctx context.Context, id string, dest transport.Sink) error {
				bytea, err := keySpace.DataSource().Fetch(ctx, id)
				if err != nil {
					return err
				}
				expiredAt := keySpace.ExpiresAt(ctx, id)
				return dest.SetBytes(bytea, expiredAt)
			}))

		if err != nil {
			k.lock.Unlock()
			return fmt.Errorf("failed to create group: %w", err)
		}

		// add the group to the groups list
		k.groups.Set(group.Name(), group)
	}

	// create enough buffer to house the cluster events
	// TODO: revisit this number
	eventsCh := make(chan memberlist.NodeEvent, 256)
	k.mconfig.Events = &memberlist.ChannelEventDelegate{
		Ch: eventsCh,
	}

	k.started.Store(true)
	k.lock.Unlock()

	// start listening to events
	go k.eventsListener(eventsCh)

	k.config.Logger().Infof("DistCache engine started: host=%s.", k.hostNode.Address())
	return nil
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
func (k *engine) Stop(ctx context.Context) error {
	k.lock.Lock()
	defer k.lock.Unlock()

	k.config.Logger().Infof("DistCache engine on host=%s stopping...", k.hostNode.Address())

	if !k.started.Load() {
		return nil
	}

	// stop the events loop
	close(k.stopEventsListenerSig)

	provider := k.config.DiscoveryProvider()
	if err := errorschain.
		New(errorschain.ReturnFirst()).
		AddError(k.mlist.Leave(k.config.ShutdownTimeout())).
		AddError(provider.Deregister()).
		AddError(provider.Close()).
		AddError(k.mlist.Shutdown()).
		AddError(k.daemon.Shutdown(ctx)).
		Error(); err != nil {
		return err
	}

	k.config.Logger().Infof("DistCache engine on host=%s stopped.", k.hostNode.Address())
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
func (k *engine) Put(ctx context.Context, keyspace string, entry *Entry) error {
	k.lock.Lock()
	defer k.lock.Unlock()

	group, ok := k.groups.Get(keyspace)
	if !ok {
		return ErrKeySpaceNotFound
	}

	ctx, cancel := context.WithTimeout(ctx, k.config.WriteTimeout())
	defer cancel()

	return group.Set(ctx, entry.KV.Key, entry.KV.Value, entry.Expiry, true)
}

// PutMany stores multiple key/value pairs in the cache.
//
// Parameters:
//   - ctx: The context for cancellation and deadlines.
//   - keyspace: The keyspace in which to store the key/value pairs.
//   - kvs: A slice of key/value pairs to store.
//
// Returns an error if the operation fails.
func (k *engine) PutMany(ctx context.Context, keyspace string, entries []*Entry) error {
	k.lock.Lock()
	defer k.lock.Unlock()

	group, ok := k.groups.Get(keyspace)
	if !ok {
		return ErrKeySpaceNotFound
	}

	for _, entry := range entries {
		ctx, cancel := context.WithTimeout(ctx, k.config.WriteTimeout())
		if err := group.Set(ctx, entry.KV.Key, entry.KV.Value, entry.Expiry, true); err != nil {
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
func (k *engine) Get(ctx context.Context, keyspace string, key string) (*KV, error) {
	k.lock.Lock()
	defer k.lock.Unlock()

	group, ok := k.groups.Get(keyspace)
	if !ok {
		return nil, ErrKeySpaceNotFound
	}

	ctx, cancel := context.WithTimeout(ctx, k.config.ReadTimeout())
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
func (k *engine) Delete(ctx context.Context, keyspace string, key string) error {
	k.lock.Lock()
	defer k.lock.Unlock()

	group, ok := k.groups.Get(keyspace)
	if !ok {
		return ErrKeySpaceNotFound
	}

	ctx, cancel := context.WithTimeout(ctx, k.config.WriteTimeout())
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
func (k *engine) DeleteMany(ctx context.Context, keyspace string, keys []string) error {
	k.lock.Lock()
	defer k.lock.Unlock()

	group, ok := k.groups.Get(keyspace)
	if !ok {
		return ErrKeySpaceNotFound
	}

	for _, key := range keys {
		ctx, cancel := context.WithTimeout(ctx, k.config.WriteTimeout())
		if err := group.Remove(ctx, key); err != nil {
			cancel()
			return err
		}
		cancel()
	}
	return nil
}

// peers returns a channel containing the list of peers at a given time
func (k *engine) peers() ([]*Peer, error) {
	k.lock.Lock()
	members := k.mlist.Members()
	k.lock.Unlock()
	peers := make([]*Peer, 0, len(members))
	for _, member := range members {
		peer, err := fromBytes(member.Meta)
		if err != nil {
			return nil, err
		}

		if peer != nil && !peer.IsSelf {
			peers = append(peers, peer)
		}
	}
	return peers, nil
}

// eventsListener listens to cluster events
func (k *engine) eventsListener(eventsChan chan memberlist.NodeEvent) {
	for {
		select {
		case <-k.stopEventsListenerSig:
			// finish listening to cluster events
			return
		case event := <-eventsChan:
			node, err := fromBytes(event.Node.Meta)
			if err != nil {
				k.config.Logger().Errorf("DistCache on host=%s failed to decode event: %v", err)
				continue
			}

			// skip self on cluster event
			if node.Address() == k.hostNode.Address() {
				k.config.Logger().Warnf("DistCache on host=%s is already in use", node.Address())
				continue
			}

			ctx := context.Background()
			// we need to add the new peers
			currentPeers, _ := k.peers()
			peersSet := goset.NewSet[peer.Info]()
			peersSet.Add(peer.Info{
				Address: k.hostNode.Address(),
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
				k.config.Logger().Infof("DistCache on host=%s has noticed node=%s has joined the cluster", k.hostNode.Address(), node.Address())

				// add the joined node to the peers list
				// and set it to the daemon
				k.eventsLock.Lock()
				peersSet.Add(peer.Info{
					Address: node.Address(),
					IsSelf:  node.IsSelf,
				})

				_ = k.daemon.SetPeers(ctx, peersSet.ToSlice())
				k.eventsLock.Unlock()

			case memberlist.NodeLeave:
				k.config.Logger().Infof("DistCache on host=%s has noticed node=%s has left the cluster", k.hostNode.Address(), node.Address())

				// remove the left node from the peers list
				// and set it to the daemon
				k.eventsLock.Lock()
				peersSet.Remove(peer.Info{
					Address: node.Address(),
					IsSelf:  node.IsSelf,
				})
				_ = k.daemon.SetPeers(ctx, peersSet.ToSlice())
				k.eventsLock.Unlock()

			case memberlist.NodeUpdate:
				// TODO: need to handle that later
				continue
			}
		}
	}
}

// joinCluster attempts to join an existing cluster if peers are provided
func (k *engine) joinCluster(ctx context.Context) error {
	var err error
	k.mlist, err = memberlist.Create(k.mconfig)
	if err != nil {
		return err
	}

	joinTimeout := computeTimeout(k.config.MaxJoinAttempts(), k.config.JoinRetryInterval())
	ctx, cancel := context.WithTimeout(ctx, joinTimeout)

	var peers []string
	retrier := retry.NewRetrier(k.config.MaxJoinAttempts(), k.config.JoinRetryInterval(), k.config.JoinRetryInterval())
	if err := retrier.RunContext(ctx, func(ctx context.Context) error {
		peers, err = k.config.DiscoveryProvider().DiscoverPeers()
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
		if k.config.MinimumPeersQuorum() > len(peers) {
			cancel()
			return ErrClusterQuorum
		}
		cancel()

		// attempt to join
		ctx, cancel = context.WithTimeout(ctx, joinTimeout)
		joinRetrier := retry.NewRetrier(k.config.MaxJoinAttempts(), k.config.JoinRetryInterval(), k.config.JoinRetryInterval())
		if err := joinRetrier.RunContext(ctx, func(ctx context.Context) error {
			if _, err := k.mlist.Join(peers); err != nil {
				return err
			}
			return nil
		}); err != nil {
			cancel()
			return err
		}

		k.config.Logger().Infof("DistCache on host=%s joined cluster of [%s]", k.hostNode.Address(), strings.Join(peers, ","))
	}

	cancel()
	return nil
}

// computeTimeout calculates the approximate timeout given maxAttempts and retryInterval.
func computeTimeout(maxAttempts int, retryInterval time.Duration) time.Duration {
	if maxAttempts <= 1 {
		return 0 // No retries, so no waiting time.
	}
	return time.Duration(maxAttempts-1) * retryInterval
}
