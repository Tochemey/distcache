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
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	goset "github.com/deckarep/golang-set/v3"
	"github.com/flowchartsman/retry"
	"github.com/groupcache/groupcache-go/v3"
	"github.com/groupcache/groupcache-go/v3/transport"
	"github.com/groupcache/groupcache-go/v3/transport/peer"
	"github.com/hashicorp/memberlist"

	"github.com/tochemey/distcache/admin"
	"github.com/tochemey/distcache/internal/errorschain"
	"github.com/tochemey/distcache/internal/members"
	"github.com/tochemey/distcache/internal/syncmap"
	"github.com/tochemey/distcache/internal/tcp"
	"github.com/tochemey/distcache/warmup"
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

	// GetMany retrieves multiple key/value pairs from the cache.
	//
	// Parameters:
	//   - ctx: The context for cancellation and deadlines.
	//   - keyspace: The keyspace from which to retrieve the key/value pairs.
	//   - keys: A slice of keys identifying the desired key/value pairs.
	//
	// Returns a slice of key/value pairs in the same order as keys, or an error
	// if any key retrieval fails.
	GetMany(ctx context.Context, keyspace string, keys []string) ([]*KV, error)

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

	// UpdateKeySpace replaces a keyspace definition at runtime.
	//
	// This operation recreates the underlying cache group using the new
	// keyspace settings (max bytes, TTL policy, and data source).
	//
	// Parameters:
	//   - ctx: The context for cancellation and deadlines.
	//   - keyspace: The updated keyspace definition.
	//
	// Returns an error if the keyspace does not exist or the update fails.
	UpdateKeySpace(ctx context.Context, keyspace KeySpace) error

	// KeySpaces returns the list of available KeySpaces from the cache.
	//
	// Returns an empty list if there are no keyspaces
	KeySpaces() []string

	// Events returns a channel of cluster-membership events (peer joined,
	// left, updated). Each call returns a fresh subscriber channel; multiple
	// subscribers each receive every event.
	//
	// The channel is buffered. If a subscriber falls behind, individual events
	// are dropped rather than blocking the engine. Subscribers should drain
	// the channel promptly.
	//
	// The channel is closed when the engine is stopped. There is no
	// unsubscribe; obtain the channel once at startup and reuse it for the
	// lifetime of the engine.
	Events() <-chan Event
}

type engine struct {
	config       *Config
	hostNode     *Peer
	mconfig      *memberlist.Config
	mtransport   memberlist.Transport
	mlist        memberlistAPI
	started      atomic.Bool
	eventsCancel context.CancelFunc
	eventsCh     chan memberlist.NodeEvent
	// peersLock serializes every daemon.SetPeers call so that the membership
	// view applied to the daemon is monotonic between Start and the events loop.
	// It also guards the daemon pointer until Start has published it.
	peersLock sync.Mutex
	// lock guards the engine lifecycle (Start/Stop) and keyspace mutations.
	// It is deliberately not taken on the data path (Get/Put/Delete).
	lock sync.Mutex
	// startMu guards startCancel, which lets Stop interrupt a Start that is
	// still retrying discovery or join.
	startMu      sync.Mutex
	startCancel  context.CancelFunc
	daemon       daemonAPI
	adminStarted bool
	// providerInitialized / providerRegistered record how far discovery got so
	// that shutdown only undoes what was actually done.
	providerInitialized  bool
	providerRegistered   bool
	groups               *syncmap.Map[string, transport.Group]
	keySpaces            *syncmap.Map[string, *keySpaceWrapper]
	instrumentation      *instrumentation
	admin                *admin.Server
	hotKeys              *warmup.Tracker
	warmupConfig         *warmup.Config
	events               *eventBus
	newTransportFunc     func(members.TransportConfig) (memberlist.Transport, error)
	createMemberlistFunc func(*memberlist.Config) (memberlistAPI, error)
	listenAndServeFunc   func(context.Context, string, groupcache.Options) (daemonAPI, error)
}

var _ Engine = (*engine)(nil)

const (
	defaultHTTPDialTimeout     = 15 * time.Second
	defaultHTTPClientTimeout   = 30 * time.Second
	defaultIdleConnTimeout     = 2 * time.Minute
	defaultTLSHandshakeTimeout = 15 * time.Second
	defaultMaxIdleConns        = 200
	defaultMaxIdleConnsPerHost = 50
)

type memberlistAPI interface {
	Members() []*memberlist.Node
	Join(existing []string) (int, error)
	Leave(timeout time.Duration) error
	Shutdown() error
}

type daemonAPI interface {
	SetPeers(context.Context, []peer.Info) error
	NewGroup(string, int64, groupcache.Getter) (transport.Group, error)
	RemoveGroup(name string)
	Shutdown(ctx context.Context) error
	GetInstance() *groupcache.Instance
}

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
	if config == nil {
		return nil, errors.New("config is nil")
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

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

	keySpaces := syncmap.New[string, *keySpaceWrapper]()
	for _, keySpace := range config.KeySpaces() {
		spec, err := newKeySpaceWrapper(config, keySpace)
		if err != nil {
			return nil, err
		}
		if _, exists := keySpaces.Get(keySpace.Name()); exists {
			return nil, fmt.Errorf("duplicate keyspace: %s", keySpace.Name())
		}
		keySpaces.Set(keySpace.Name(), spec)
	}

	eng := &engine{
		config:          config,
		hostNode:        hostNode,
		groups:          syncmap.New[string, transport.Group](),
		keySpaces:       keySpaces,
		instrumentation: newInstrumentation(config),
		events:          newEventBus(),
		newTransportFunc: func(cfg members.TransportConfig) (memberlist.Transport, error) {
			return members.NewTransport(cfg)
		},
		createMemberlistFunc: func(cfg *memberlist.Config) (memberlistAPI, error) {
			return memberlist.Create(cfg)
		},
		listenAndServeFunc: func(ctx context.Context, address string, opts groupcache.Options) (daemonAPI, error) {
			return groupcache.ListenAndServe(ctx, address, opts)
		},
	}

	if cfg := config.AdminConfig(); cfg != nil {
		eng.admin = admin.NewServer(*cfg, newAdminProvider(eng), config.Logger())
	}

	if cfg := config.WarmupConfig(); cfg != nil {
		normalized := cfg.Normalize()
		eng.warmupConfig = &normalized
		eng.hotKeys = warmup.NewTracker(normalized.MaxHotKeys)
	}

	return eng, nil
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
	defer x.lock.Unlock()

	if x.started.Load() {
		return ErrEngineAlreadyStarted
	}

	x.config.Logger().Infof("Cache Engine starting on [%s/%s, host=%s]...", runtime.GOOS, runtime.GOARCH, x.hostNode.Address())

	// Stop can cancel this context to interrupt a Start that is still retrying
	startCtx, cancelStart := context.WithCancel(ctx)
	defer cancelStart()
	x.setStartCancel(cancelStart)
	defer x.setStartCancel(nil)

	if err := x.setupMemberlistConfig(); err != nil {
		return err
	}

	// the events loop is detached from the caller's context so that it outlives
	// the Start call; it is stopped by Stop (or by a failed Start).
	eventsCtx, cancelEvents := context.WithCancel(context.Background())
	x.eventsCancel = cancelEvents

	if err := x.bootstrapCluster(startCtx, eventsCtx); err != nil {
		return x.abortStart(err)
	}

	bootCtx, cancel := context.WithTimeout(startCtx, x.config.BootstrapTimeout())
	defer cancel()

	if err := x.startDaemon(bootCtx); err != nil {
		return x.abortStart(err)
	}

	if err := x.applyPeers(bootCtx); err != nil {
		return x.abortStart(err)
	}

	if err := x.createGroups(); err != nil {
		return x.abortStart(err)
	}

	if x.admin != nil {
		if err := x.admin.Start(bootCtx); err != nil {
			return x.abortStart(err)
		}
		x.adminStarted = true
	}

	if x.warmupConfig != nil && x.warmupConfig.RefreshInterval > 0 {
		go x.refreshLoop(eventsCtx, x.warmupConfig.RefreshInterval)
	}

	x.started.Store(true)
	x.config.Logger().Infof("Cache engine started: host=%s.", x.hostNode.Address())
	return nil
}

// setStartCancel records (or clears) the cancel function of an in-progress Start.
func (x *engine) setStartCancel(cancel context.CancelFunc) {
	x.startMu.Lock()
	x.startCancel = cancel
	x.startMu.Unlock()
}

// cancelPendingStart interrupts a Start that is still retrying discovery or
// join so that Stop does not have to wait for the retries to run out.
func (x *engine) cancelPendingStart() {
	x.startMu.Lock()
	cancel := x.startCancel
	x.startMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// abortStart tears down whatever Start managed to bring up before failing and
// returns the original cause. Cleanup errors are logged, not returned. Cleanup
// runs under its own deadline because the caller's context may be the very
// reason Start failed.
func (x *engine) abortStart(cause error) error {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), x.config.ShutdownTimeout())
	defer cancel()
	if err := x.shutdown(cleanupCtx, false); err != nil {
		x.config.Logger().Warnf("Cache engine on host=%s cleanup after failed start: %v", x.hostNode.Address(), err)
	}
	return cause
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
	x.cancelPendingStart()

	x.lock.Lock()
	defer x.lock.Unlock()

	if !x.started.Load() {
		return nil
	}

	x.config.Logger().Infof("Cache engine on host=%s stopping...", x.hostNode.Address())

	err := x.shutdown(ctx, true)
	x.started.Store(false)
	if err != nil {
		return err
	}

	x.config.Logger().Infof("Cache engine on host=%s stopped.", x.hostNode.Address())
	return nil
}

// shutdown releases every resource the engine holds. It is best-effort: every
// step runs even when a previous one failed, and all errors are returned
// together. closeEvents controls whether the public event bus is closed, which
// only a real Stop should do.
func (x *engine) shutdown(ctx context.Context, closeEvents bool) error {
	if x.eventsCancel != nil {
		x.eventsCancel()
		x.eventsCancel = nil
	}
	if closeEvents && x.events != nil {
		x.events.close()
	}

	provider := x.config.DiscoveryProvider()
	chain := errorschain.New(errorschain.WithRunAll(), errorschain.WithContext(ctx))

	// mlist is read by the events loop and the admin handler under peersLock,
	// so it is snapshotted and cleared under that lock as well.
	x.peersLock.Lock()
	mlist := x.mlist
	mtransport := x.mtransport
	x.mlist = nil
	x.mtransport = nil
	x.peersLock.Unlock()

	if mlist != nil {
		chain = chain.AddRunner(func() error { return mlist.Leave(x.config.ShutdownTimeout()) })
	}
	if x.providerRegistered {
		chain = chain.AddRunner(provider.Deregister)
		x.providerRegistered = false
	}
	if x.providerInitialized {
		chain = chain.AddRunner(provider.Close)
		x.providerInitialized = false
	}
	switch {
	case mlist != nil:
		// memberlist shuts its transport down as part of its own Shutdown
		chain = chain.AddRunner(mlist.Shutdown)
	case mtransport != nil:
		// Start failed before memberlist was created: release the gossip listener
		chain = chain.AddRunner(mtransport.Shutdown)
	}

	x.peersLock.Lock()
	daemon := x.daemon
	x.daemon = nil
	x.peersLock.Unlock()
	if daemon != nil {
		chain = chain.AddContextRunner(daemon.Shutdown)
	}

	if x.admin != nil && x.adminStarted {
		chain = chain.AddContextRunner(x.admin.Shutdown)
		x.adminStarted = false
	}

	return chain.Run()
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
func (x *engine) Put(ctx context.Context, keyspace string, entry *Entry) (err error) {
	ctx, end := x.instrumentation.start(ctx, "put", keyspace)
	defer func() { end(err) }()

	if entry == nil {
		return ErrNilEntry
	}

	group, spec, err := x.lookup(keyspace)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, x.writeTimeout(spec))
	defer cancel()

	wrapped, err := spec.wrap(entry.Value)
	if err != nil {
		return err
	}
	return group.Set(ctx, entry.Key, wrapped, spec.expiryFor(entry.Expiry), true)
}

// PutMany stores multiple key/value pairs in the cache.
//
// Parameters:
//   - ctx: The context for cancellation and deadlines.
//   - keyspace: The keyspace in which to store the key/value pairs.
//   - kvs: A slice of key/value pairs to store.
//
// Returns an error if the operation fails.
func (x *engine) PutMany(ctx context.Context, keyspace string, entries []*Entry) (err error) {
	ctx, end := x.instrumentation.start(ctx, "put_many", keyspace)
	defer func() { end(err) }()

	group, spec, err := x.lookup(keyspace)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry == nil {
			return ErrNilEntry
		}

		wrapped, err := spec.wrap(entry.Value)
		if err != nil {
			return err
		}
		opCtx, cancel := context.WithTimeout(ctx, x.writeTimeout(spec))
		if err := group.Set(opCtx, entry.Key, wrapped, spec.expiryFor(entry.Expiry), true); err != nil {
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
func (x *engine) Get(ctx context.Context, keyspace string, key string) (kv *KV, err error) {
	ctx, end := x.instrumentation.start(ctx, "get", keyspace)
	defer func() { end(err) }()

	group, spec, err := x.lookup(keyspace)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, x.readTimeout(spec))
	defer cancel()

	var raw []byte
	if err := group.Get(ctx, key, transport.AllocatingByteSliceSink(&raw)); err != nil {
		return nil, err
	}

	value, err := spec.unwrap(raw)
	if err != nil {
		return nil, err
	}

	if x.hotKeys != nil {
		x.hotKeys.Record(keyspace, key)
	}

	return &KV{
		Key:   key,
		Value: value,
	}, nil
}

// GetMany retrieves multiple key/value pairs from the cache.
//
// Parameters:
//   - ctx: The context for cancellation and deadlines.
//   - keyspace: The keyspace from which to retrieve the key/value pairs.
//   - keys: A slice of keys identifying the desired key/value pairs.
//
// Returns a slice of key/value pairs in the same order as keys, or an error
// if any key retrieval fails.
func (x *engine) GetMany(ctx context.Context, keyspace string, keys []string) (kvs []*KV, err error) {
	ctx, end := x.instrumentation.start(ctx, "get_many", keyspace)
	defer func() { end(err) }()

	group, spec, err := x.lookup(keyspace)
	if err != nil {
		return nil, err
	}

	kvs = make([]*KV, 0, len(keys))
	for _, key := range keys {
		opCtx, cancel := context.WithTimeout(ctx, x.readTimeout(spec))
		var raw []byte
		if err := group.Get(opCtx, key, transport.AllocatingByteSliceSink(&raw)); err != nil {
			cancel()
			return nil, err
		}
		cancel()

		value, err := spec.unwrap(raw)
		if err != nil {
			return nil, err
		}

		if x.hotKeys != nil {
			x.hotKeys.Record(keyspace, key)
		}

		kvs = append(kvs, &KV{Key: key, Value: value})
	}

	return kvs, nil
}

// Delete removes a specific key/value pair from the cache.
//
// Parameters:
//   - ctx: The context for cancellation and deadlines.
//   - keyspace: The keyspace from which to delete the key/value pair.
//   - key: The key identifying the key/value pair to be deleted.
//
// Returns an error if the operation fails.
func (x *engine) Delete(ctx context.Context, keyspace string, key string) (err error) {
	ctx, end := x.instrumentation.start(ctx, "delete", keyspace)
	defer func() { end(err) }()

	group, spec, err := x.lookup(keyspace)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, x.writeTimeout(spec))
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
func (x *engine) DeleteMany(ctx context.Context, keyspace string, keys []string) (err error) {
	ctx, end := x.instrumentation.start(ctx, "delete_many", keyspace)
	defer func() { end(err) }()

	group, spec, err := x.lookup(keyspace)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, x.writeTimeout(spec))
	defer cancel()

	return group.RemoveKeys(ctx, keys...)
}

// KeySpaces returns the list of available KeySpaces from the cache.
//
// Returns an empty list if there are no keyspaces
func (x *engine) KeySpaces() []string {
	return x.keySpaces.Keys()
}

// lookup resolves the cache group and keyspace definition for the given
// keyspace name. It is lock-free: both maps are concurrency-safe.
func (x *engine) lookup(keyspace string) (transport.Group, *keySpaceWrapper, error) {
	group, ok := x.groups.Get(keyspace)
	if !ok {
		return nil, nil, ErrKeySpaceNotFound
	}
	spec, ok := x.keySpaces.Get(keyspace)
	if !ok {
		return nil, nil, ErrKeySpaceNotFound
	}
	return group, spec, nil
}

func (x *engine) Events() <-chan Event {
	return x.events.subscribe()
}

func (x *engine) publishEvent(t EventType, node *Peer) {
	if x.events == nil {
		return
	}
	x.events.publish(Event{Type: t, Peer: node, At: time.Now()})
}

// DeleteKeySpace delete a given keySpace from the cache.
//
// Parameters:
//   - ctx: The context for cancellation and deadlines.
//   - keyspace: The keyspace from which to delete the key/value pairs.
//
// Returns an error if the operation fails.
func (x *engine) DeleteKeySpace(ctx context.Context, keyspace string) (err error) {
	_, end := x.instrumentation.start(ctx, "delete_keyspace", keyspace)
	defer func() { end(err) }()

	x.lock.Lock()
	defer x.lock.Unlock()

	if x.daemon == nil {
		return ErrEngineNotStarted
	}

	group, ok := x.groups.Get(keyspace)
	if !ok {
		return ErrKeySpaceNotFound
	}

	x.daemon.RemoveGroup(group.Name())
	x.groups.Delete(keyspace)
	x.keySpaces.Delete(keyspace)
	x.removeKeySpaceFromConfig(keyspace)
	return nil
}

// DeleteKeyspaces removes multiple keyspaces from the cache.
//
// Parameters:
//   - ctx: The context for cancellation and deadlines.
//   - keyspaces: A slice of keyspaces to be deleted.
//
// Returns an error if the operation fails.
func (x *engine) DeleteKeyspaces(ctx context.Context, keyspaces []string) (err error) {
	_, end := x.instrumentation.start(ctx, "delete_keyspaces", "")
	defer func() { end(err) }()

	x.lock.Lock()
	defer x.lock.Unlock()

	if x.daemon == nil {
		return ErrEngineNotStarted
	}

	for _, keyspace := range keyspaces {
		if group, ok := x.groups.Get(keyspace); ok {
			x.daemon.RemoveGroup(group.Name())
			x.groups.Delete(keyspace)
			x.keySpaces.Delete(keyspace)
			x.removeKeySpaceFromConfig(keyspace)
		}
	}
	return nil
}

// UpdateKeySpace replaces a keyspace definition at runtime.
//
// This operation recreates the underlying cache group using the new keyspace
// settings and triggers a warm-up when configured.
func (x *engine) UpdateKeySpace(ctx context.Context, keyspace KeySpace) (err error) {
	if keyspace == nil {
		return fmt.Errorf("keyspace is nil")
	}

	_, end := x.instrumentation.start(ctx, "update_keyspace", keyspace.Name())
	defer func() { end(err) }()

	x.lock.Lock()
	defer x.lock.Unlock()

	if x.daemon == nil {
		return ErrEngineNotStarted
	}

	if _, ok := x.groups.Get(keyspace.Name()); !ok {
		return ErrKeySpaceNotFound
	}

	spec, err := newKeySpaceWrapper(x.config, keyspace)
	if err != nil {
		return err
	}

	oldSpec, _ := x.keySpaces.Get(keyspace.Name())
	x.daemon.RemoveGroup(keyspace.Name())

	group, err := x.createGroup(spec)
	if err != nil {
		if oldSpec != nil {
			if rollbackGroup, rollbackErr := x.createGroup(oldSpec); rollbackErr == nil {
				x.groups.Set(rollbackGroup.Name(), rollbackGroup)
			}
		}
		return err
	}

	x.groups.Set(group.Name(), group)
	x.keySpaces.Set(keyspace.Name(), spec)
	x.replaceKeySpaceInConfig(keyspace)

	x.triggerWarmup(keyspace.Name(), spec)

	return nil
}

func (x *engine) startDaemon(ctx context.Context) error {
	httpOpts := x.httpTransportOptions()
	daemonOpts := groupcache.Options{
		HashFn:    x.config.Hasher().HashCode,
		Replicas:  x.config.ReplicaCount(),
		Logger:    newCacheLog(x.config.Logger()),
		Transport: transport.NewHttpTransport(httpOpts),
	}

	if x.config.MetricConfig() != nil {
		daemonOpts.MetricProvider = groupcache.NewMeterProvider(
			groupcache.WithMeterProvider(x.config.MetricConfig().Provider()),
		)
	}

	daemon, err := x.listenAndServeFunc(ctx, x.hostNode.Address(), daemonOpts)
	if err != nil {
		return fmt.Errorf("failed to start engine daemon: %w", err)
	}

	x.peersLock.Lock()
	x.daemon = daemon
	x.peersLock.Unlock()
	return nil
}

func (x *engine) httpTransportOptions() transport.HttpTransportOptions {
	tlsInfo := x.config.TLSInfo()
	scheme := "http"
	if tlsInfo != nil {
		scheme = "https"
	}

	httpTransport := x.newHTTPTransport(tlsInfo)
	opts := transport.HttpTransportOptions{
		Client: &http.Client{
			Timeout:   defaultHTTPClientTimeout,
			Transport: httpTransport,
		},
		Scheme: scheme,
		Logger: newCacheLog(x.config.Logger()),
	}

	if tlsInfo != nil {
		opts.TLSConfig = tlsInfo.ServerTLS
	}

	if x.config.traceConfig != nil {
		opts.Tracer = transport.NewTracer(
			transport.WithTraceProvider(x.config.TraceConfig().TracerProvider()),
			transport.WithTracerAttributes(x.config.TraceConfig().Attributes()...),
		)
	}

	return opts
}

func (x *engine) newHTTPTransport(tlsInfo *TLSInfo) *http.Transport {
	httpTransport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   defaultHTTPDialTimeout,
			KeepAlive: x.config.KeepAlivePeriod(),
		}).DialContext,
		MaxIdleConns:        defaultMaxIdleConns,
		MaxIdleConnsPerHost: defaultMaxIdleConnsPerHost,
		IdleConnTimeout:     defaultIdleConnTimeout,
	}

	if tlsInfo != nil {
		httpTransport.TLSClientConfig = tlsInfo.ClientTLS
		httpTransport.TLSHandshakeTimeout = defaultTLSHandshakeTimeout
	}

	return httpTransport
}

// applyPeers pushes the current cluster membership to the daemon. It takes
// peersLock so that it cannot interleave with the events loop and apply a
// stale view after a fresher one.
func (x *engine) applyPeers(ctx context.Context) error {
	x.peersLock.Lock()
	defer x.peersLock.Unlock()

	if err := x.daemon.SetPeers(ctx, x.currentPeerInfos().ToSlice()); err != nil {
		return fmt.Errorf("failed to set peers: %w", err)
	}
	return nil
}

// currentPeerInfos returns the live membership (including self) as groupcache
// peer descriptors. Members whose metadata cannot be decoded are skipped.
func (x *engine) currentPeerInfos() goset.Set[peer.Info] {
	peerInfos := goset.NewSet[peer.Info]()
	peerInfos.Add(peer.Info{
		Address: x.hostNode.Address(),
		IsSelf:  true,
	})

	for _, xpeer := range x.peers() {
		peerInfos.Add(peer.Info{
			Address: xpeer.Address(),
			IsSelf:  xpeer.IsSelf,
		})
	}
	return peerInfos
}

func (x *engine) createGroups() error {
	if err := x.ensureKeySpaces(); err != nil {
		return err
	}

	var createErr error
	x.keySpaces.Range(func(_ string, spec *keySpaceWrapper) {
		if createErr != nil {
			return
		}
		group, err := x.createGroup(spec)
		if err != nil {
			createErr = err
			return
		}
		x.groups.Set(group.Name(), group)
	})

	return createErr
}

func (x *engine) configureEventDelegate() chan memberlist.NodeEvent {
	eventsCh := make(chan memberlist.NodeEvent, 1024)
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

	mtransport, err := x.newTransportFunc(mtConfig)
	if err != nil {
		x.config.Logger().Errorf("failed to create memberlist TCP transport: %v", err)
		return err
	}

	meta, _ := json.Marshal(x.hostNode)
	x.mtransport = mtransport
	x.mconfig = x.newMemberlistConfig(mtransport, meta)
	// the delegate must be in place before memberlist.Create runs: memberlist
	// reads Config.Events from its own goroutines, so installing it later is a
	// data race and misses nodes that join during bootstrap.
	x.eventsCh = x.configureEventDelegate()
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
	// Kubernetes-specific filtering is necessary because dynamic IP assignment can cause pods in different namespaces to share the same IP address over time.
	// This can lead to unintended cross-namespace communication within the memberlist ring.
	// To prevent this, all nodes are assigned the same label corresponding to the actor system name, enabling proper filtering.
	// As a result, even if a pod receives a gossip message from a reused IP now belonging to a different namespace,
	// the message will be rejected if it lacks the expected label identifying it as part of the correct ring.
	// The "prefix-" literal is part of the gossip wire format: changing it would
	// prevent rolling upgrades against nodes running an older release.
	config.Label = fmt.Sprintf("prefix-%s", x.config.Label())
	if secret := x.config.GossipSecret(); len(secret) > 0 {
		config.SecretKey = secret
	}
	return config
}

// bootstrapCluster initializes discovery, creates the memberlist instance,
// starts the cluster events loop and then joins the cluster. The events loop
// must be running before Join so memberlist never blocks on a full event
// channel while handing us the existing members.
func (x *engine) bootstrapCluster(ctx context.Context, eventsCtx context.Context) error {
	provider := x.config.DiscoveryProvider()
	return errorschain.
		New(errorschain.WithFailFast(), errorschain.WithContext(ctx)).
		AddRunner(func() error {
			if err := provider.Initialize(); err != nil {
				return err
			}
			x.providerInitialized = true
			return nil
		}).
		AddRunner(func() error {
			if err := provider.Register(); err != nil {
				return err
			}
			x.providerRegistered = true
			return nil
		}).
		AddRunner(x.createMemberlist).
		AddRunner(func() error {
			go x.eventsListener(eventsCtx, x.eventsCh)
			return nil
		}).
		AddContextRunner(x.joinCluster).
		Run()
}

func (x *engine) createMemberlist() error {
	mlist, err := x.createMemberlistFunc(x.mconfig)
	if err != nil {
		return err
	}
	x.peersLock.Lock()
	x.mlist = mlist
	x.peersLock.Unlock()
	return nil
}

// peers returns the current cluster members excluding the host node. Members
// whose metadata cannot be decoded (for example a foreign process gossiping on
// the same port) are logged and skipped rather than failing the whole list.
//
// The caller must hold peersLock: mlist is cleared under that lock by shutdown.
func (x *engine) peers() []*Peer {
	if x.mlist == nil {
		return nil
	}
	members := x.mlist.Members()
	peers := make([]*Peer, 0, len(members))
	for _, member := range members {
		peer, err := fromBytes(member.Meta)
		if err != nil {
			x.config.Logger().Warnf("Cache on host=%s ignoring member=%s with invalid metadata: %v", x.hostNode.Address(), member.Name, err)
			continue
		}

		// exclude the host node from the list of found peers
		if peer.Address() != x.hostNode.Address() {
			peers = append(peers, peer)
		}
	}
	return peers
}

// eventsListener listens to cluster events
func (x *engine) eventsListener(ctx context.Context, eventsChan chan memberlist.NodeEvent) {
	for {
		select {
		case <-ctx.Done():
			// finish listening to cluster events
			return
		case event := <-eventsChan:
			node, err := fromBytes(event.Node.Meta)
			if err != nil {
				x.config.Logger().Errorf("Cache on host=%s failed to decode event: %v", x.hostNode.Address(), err)
				continue
			}

			// skip self on cluster event
			if node.Address() == x.hostNode.Address() {
				x.config.Logger().Debugf("Cache on host=%s ignoring membership event about itself", x.hostNode.Address())
				continue
			}

			switch event.Event {
			case memberlist.NodeJoin:
				x.config.Logger().Infof("Cache on host=%s has noticed node=%s has joined the cluster", x.hostNode.Address(), node.Address())
				x.updateDaemonPeers(ctx, node, true)
				x.warmupOnClusterEvent(memberlist.NodeJoin)
				x.publishEvent(EventPeerJoined, node)

			case memberlist.NodeLeave:
				x.config.Logger().Infof("Cache on host=%s has noticed node=%s has left the cluster", x.hostNode.Address(), node.Address())
				x.updateDaemonPeers(ctx, node, false)
				x.warmupOnClusterEvent(memberlist.NodeLeave)
				x.publishEvent(EventPeerLeft, node)

			case memberlist.NodeUpdate:
				x.updateDaemonPeers(ctx, node, true)
				x.publishEvent(EventPeerUpdated, node)
			}
		}
	}
}

// updateDaemonPeers recomputes the membership from memberlist, adds or removes
// the node the event is about, and pushes the result to the daemon. Events that
// arrive before the daemon exists are skipped: Start applies a fresh snapshot
// once the daemon is up, under the same lock, so nothing is lost.
func (x *engine) updateDaemonPeers(ctx context.Context, node *Peer, present bool) {
	x.peersLock.Lock()
	defer x.peersLock.Unlock()

	if x.daemon == nil {
		return
	}

	peersSet := x.currentPeerInfos()
	info := peer.Info{Address: node.Address(), IsSelf: node.IsSelf}
	if present {
		peersSet.Add(info)
	} else {
		peersSet.Remove(info)
	}

	if err := x.daemon.SetPeers(ctx, peersSet.ToSlice()); err != nil {
		x.config.Logger().Errorf("Cache on host=%s failed to update peers after event about node=%s: %v", x.hostNode.Address(), node.Address(), err)
	}
}

// joinCluster attempts to join an existing cluster if peers are provided
func (x *engine) joinCluster(ctx context.Context) error {
	var err error
	joinTimeout := computeTimeout(x.config.MaxJoinAttempts(), x.config.JoinRetryInterval())
	discoverCtx, cancel := context.WithTimeout(ctx, joinTimeout)

	var peers []string
	retrier := retry.NewRetrier(x.config.MaxJoinAttempts(), x.config.JoinRetryInterval(), x.config.JoinRetryInterval())
	if err := retrier.RunContext(discoverCtx, func(_ context.Context) error {
		peers, err = x.config.DiscoveryProvider().DiscoverPeers()
		if err != nil {
			return err
		}
		return nil
	}); err != nil {
		cancel()
		return err
	}
	cancel()

	if len(peers) > 0 {
		// check whether the cluster quorum is met to operate
		if x.config.MinimumPeersQuorum() > len(peers) {
			return ErrClusterQuorum
		}

		// attempt to join
		joinCtx, joinCancel := context.WithTimeout(ctx, joinTimeout)
		joinRetrier := retry.NewRetrier(x.config.MaxJoinAttempts(), x.config.JoinRetryInterval(), x.config.JoinRetryInterval())
		if err := joinRetrier.RunContext(joinCtx, func(_ context.Context) error {
			if _, err := x.mlist.Join(peers); err != nil {
				return err
			}
			return nil
		}); err != nil {
			joinCancel()
			return err
		}
		joinCancel()

		x.config.Logger().Infof("Cache on host=%s joined cluster of [%s]", x.hostNode.Address(), strings.Join(peers, ","))
	}

	return nil
}

// computeTimeout calculates the approximate timeout given maxAttempts and retryInterval.
func computeTimeout(maxAttempts int, retryInterval time.Duration) time.Duration {
	return time.Duration(maxAttempts-1) * retryInterval
}

func (x *engine) warmupOnClusterEvent(event memberlist.NodeEventType) {
	if x.warmupConfig == nil {
		return
	}

	cfg := *x.warmupConfig
	switch event {
	case memberlist.NodeJoin:
		if !cfg.WarmOnJoin {
			return
		}
	case memberlist.NodeLeave:
		if !cfg.WarmOnLeave {
			return
		}
	}

	go func() {
		x.keySpaces.Range(func(name string, spec *keySpaceWrapper) {
			x.triggerWarmup(name, spec)
		})
	}()
}

func (x *engine) triggerWarmup(keyspace string, spec *keySpaceWrapper) {
	if x.warmupConfig == nil || spec == nil {
		return
	}

	cfg := *x.warmupConfig
	keys := x.collectWarmupKeys(keyspace, spec, cfg)
	if len(keys) == 0 {
		return
	}

	go x.prefetchKeys(keyspace, spec, keys, cfg)
}

func (x *engine) collectWarmupKeys(keyspace string, spec *keySpaceWrapper, cfg warmup.Config) []string {
	keys := make(map[string]struct{})
	for _, key := range spec.config.WarmKeys {
		if key == "" {
			continue
		}
		keys[key] = struct{}{}
	}

	if x.hotKeys != nil {
		for _, key := range x.hotKeys.TopKeys(keyspace, cfg.MaxHotKeys, cfg.MinHits) {
			keys[key] = struct{}{}
		}
	}

	if len(keys) == 0 {
		return nil
	}

	result := make([]string, 0, len(keys))
	for key := range keys {
		result = append(result, key)
	}
	return result
}

func (x *engine) prefetchKeys(keyspace string, spec *keySpaceWrapper, keys []string, cfg warmup.Config) {
	group, ok := x.groups.Get(keyspace)
	if !ok {
		return
	}

	timeout := cfg.Timeout
	if specTimeout := x.readTimeout(spec); specTimeout > 0 && specTimeout < timeout {
		timeout = specTimeout
	}

	sem := make(chan struct{}, cfg.Concurrency)
	var wg sync.WaitGroup

	for _, key := range keys {
		wg.Add(1)
		sem <- struct{}{}

		go func(k string) {
			defer wg.Done()
			defer func() { <-sem }()

			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			var value []byte
			_ = group.Get(ctx, k, transport.AllocatingByteSliceSink(&value))
		}(key)
	}

	wg.Wait()
}

// refreshLoop periodically forces a refresh of each keyspace's hot keys by
// invoking the DataSource directly and re-populating the cache. This keeps
// hot keys fresh ahead of TTL expiry so reads do not stampede the source.
//
// Only keys owned by this node are refreshed: every Set is fanned out to all
// peers by groupcache, so letting every node refresh every hot key would cost
// N fetches and N*N RPCs per key per interval.
func (x *engine) refreshLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			x.refreshHotKeys(ctx)
		}
	}
}

func (x *engine) refreshHotKeys(ctx context.Context) {
	if x.warmupConfig == nil {
		return
	}
	cfg := *x.warmupConfig

	x.keySpaces.Range(func(name string, spec *keySpaceWrapper) {
		if spec == nil {
			return
		}
		group, ok := x.groups.Get(name)
		if !ok {
			return
		}
		keys := x.collectWarmupKeys(name, spec, cfg)
		if len(keys) == 0 {
			return
		}

		timeout := cfg.Timeout
		if specTimeout := x.readTimeout(spec); specTimeout > 0 && specTimeout < timeout {
			timeout = specTimeout
		}

		sem := make(chan struct{}, cfg.Concurrency)
		var wg sync.WaitGroup
		for _, key := range keys {
			if !x.ownsKey(key) {
				continue
			}
			wg.Add(1)
			sem <- struct{}{}
			go func(k string) {
				defer wg.Done()
				defer func() { <-sem }()
				x.refreshKey(ctx, group, spec, k, timeout)
			}(key)
		}
		wg.Wait()
	})
}

// ownsKey reports whether this node is the consistent-hash owner of key.
// Without a daemon instance (single node, tests) every key is local.
func (x *engine) ownsKey(key string) bool {
	x.peersLock.Lock()
	daemon := x.daemon
	x.peersLock.Unlock()
	if daemon == nil {
		return true
	}
	instance := daemon.GetInstance()
	if instance == nil {
		return true
	}
	_, remote := instance.PickPeer(key)
	return !remote
}

func (x *engine) refreshKey(ctx context.Context, group transport.Group, spec *keySpaceWrapper, key string, timeout time.Duration) {
	fetchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	value, err := spec.dataSource.Fetch(fetchCtx, key)
	x.instrumentation.recordFetch(fetchCtx, spec.keyspace.Name(), start)
	if err != nil {
		if errors.Is(err, ErrNotFound) && spec.negativeCachingEnabled() {
			_ = group.Set(fetchCtx, key, []byte{tagTombstone}, time.Now().Add(spec.config.NegativeTTL), true)
		}
		return
	}

	wrapped, err := spec.wrap(value)
	if err != nil {
		return
	}
	_ = group.Set(fetchCtx, key, wrapped, spec.expiryFor(spec.keyspace.ExpiresAt(fetchCtx, key)), true)
}
