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
	"net"
	"os"
	"strconv"
	"time"

	"github.com/tochemey/distcache/discovery"
	"github.com/tochemey/distcache/hash"
	"github.com/tochemey/distcache/internal/validation"
	"github.com/tochemey/distcache/log"
)

const (
	// DefaultPort is for distcache
	DefaultPort = 3320

	// DefaultDiscoveryPort is for memberlist
	DefaultDiscoveryPort = 3322

	// MinimumReplicaCount denotes default and minimum replica count in a distcache
	// cluster.
	MinimumReplicaCount = 1

	// DefaultBootstrapTimeout denotes default timeout value to check bootstrapping
	// status.
	DefaultBootstrapTimeout = 10 * time.Second

	// DefaultJoinRetryInterval denotes a time gap between sequential join attempts.
	DefaultJoinRetryInterval = time.Second

	// DefaultMaxJoinAttempts denotes a maximum number of failed join attempts
	// before forming a standalone cluster.
	DefaultMaxJoinAttempts = 10

	// DefaultKeepAlivePeriod is the default value of TCP keepalive. It's 300 seconds.
	// This option is useful in order to detect dead peers (clients that cannot
	// be reached even if they look connected). Moreover, if there is network
	// equipment between clients and servers that need to see some traffic in
	// order to take the connection open, the option will prevent unexpected
	// connection closed events.
	DefaultKeepAlivePeriod = 300 * time.Second

	// DefaultShutdownTimeout is the default value of maximum amount of time before
	// shutting down
	DefaultShutdownTimeout = 5 * time.Second

	// DefaultReadTimeout is the default timeout for reading data from the distributed cache
	DefaultReadTimeout = 3 * time.Second

	// DefaultWriteTimeout is default write timeout to write data into the distributed cache
	DefaultWriteTimeout = 3 * time.Second

	// MinimumMemberCountQuorum denotes minimum required count of members to form
	// a cluster.
	MinimumMemberCountQuorum = 1
)

// Config defines distcache configuration
type Config struct {
	// Interface denotes a binding interface. It can be used instead of BindAddr
	// if the interface is known but not the address. If both are provided, then
	// distcache verifies that the interface has the bind address that is provided.
	ifname string

	// BindAddr denotes the address that distcache will bind to for communication
	// with other distcache nodes.
	bindAddr string

	// BindPort denotes the address that distcache will bind to for communication
	// with other distcache nodes.
	bindPort int

	// DiscoveryPort denotes the port distcache will use to discover other distcache nodes
	// in the cluster
	discoveryPort int

	// KeepAlivePeriod denotes whether the operating system should send
	// keep-alive messages on the connection.
	keepAlivePeriod time.Duration

	// Timeout for bootstrap control
	//
	// An distcache node checks operation status before taking any action for the
	// cluster events, responding incoming requests and running API functions.
	// Bootstrapping status is one of the most important checkpoints for an
	// "operable" distcache node. BootstrapTimeout sets a deadline to check
	// bootstrapping status without blocking indefinitely.
	bootstrapTimeout time.Duration

	// MinimumPeersQuorum denotes the minimum number of peers
	// required to form a cluster
	minimumPeersQuorum int

	// ReplicaCount is 1, by default.
	replicaCount int

	// DiscoveryProvider denotes the discovery provider to use to locate other distcache nodes
	// in the cluster
	discoveryProvider discovery.Provider

	// Logger is a custom logger which you provide. If Logger is set, it will use
	// this for the internal logger.
	logger log.Logger

	// JoinRetryInterval is the time gap between attempts to join an existing
	// cluster.
	joinRetryInterval time.Duration

	// MaxJoinAttempts denotes the maximum number of attempts to join an existing
	// cluster before forming a new one.
	maxJoinAttempts int

	// Default hasher is github.com/zeebo/xxh3
	hasher hash.Hasher

	// KeySpaces defines the various keySpaces used by distcache
	keySpaces []KeySpace

	// distcache will broadcast a leave message but will not shut down the background
	// listeners, meaning the node will continue participating in gossip and state
	// updates.
	//
	// Sending a leave message will block until the leave message is successfully
	// broadcast to a member of the cluster, if any exist or until a specified timeout
	// is reached.
	shutdownTimeout time.Duration

	// ReadTimeout defines the read timeout. This timeout is used to read data from the distributed cache
	readTimeout time.Duration

	// WriteTimeout defines the write timeout used to set a key/value pair in the distributed cache engine
	writeTimeout time.Duration

	// TLS support.
	tlsInfo *TLSInfo
}

// enforce compilation error
var _ validation.Validator = (*Config)(nil)

// NewConfig creates a new configuration instance for the distributed cache engine.
//
// It initializes the cache engine with the given discovery provider, keyspaces, and optional settings.
//
// Parameters:
//   - provider: The discovery.Provider responsible for node discovery and cluster formation.
//   - keySpaces: A slice of KeySpace instances defining different storage namespaces within the cache.
//   - opts: Optional configuration settings that can be applied to customize the engine behavior.
//
// Returns:
//   - *Config: A pointer to the newly created Config instance.
func NewConfig(provider discovery.Provider, keySpaces []KeySpace, opts ...Option) *Config {
	config := &Config{
		joinRetryInterval:  DefaultJoinRetryInterval,
		maxJoinAttempts:    DefaultMaxJoinAttempts,
		shutdownTimeout:    DefaultShutdownTimeout,
		readTimeout:        DefaultReadTimeout,
		writeTimeout:       DefaultWriteTimeout,
		hasher:             hash.DefaultHasher(),
		keySpaces:          keySpaces,
		discoveryProvider:  provider,
		discoveryPort:      DefaultDiscoveryPort,
		replicaCount:       MinimumReplicaCount,
		logger:             log.New(log.InfoLevel, os.Stdout),
		bindAddr:           "0.0.0.0",
		bindPort:           DefaultPort,
		keepAlivePeriod:    DefaultKeepAlivePeriod,
		bootstrapTimeout:   DefaultBootstrapTimeout,
		minimumPeersQuorum: MinimumMemberCountQuorum,
	}

	for _, opt := range opts {
		opt.Apply(config)
	}
	return config
}

// Interface denotes a binding interface. It can be used instead of BindAddr
// if the interface is known but not the address. If both are provided, then
// distcache verifies that the interface has the bind address that is provided.
func (c Config) Interface() string {
	return c.ifname
}

// BindAddr denotes the address that distcache will bind to for communication
// with other distcache nodes.
func (c Config) BindAddr() string {
	return c.bindAddr
}

// BindPort denotes the address that distcache will bind to for communication
// with other distcache nodes.
func (c Config) BindPort() int {
	return c.bindPort
}

// DiscoveryPort denotes the port distcache will use to discover other distcache nodes
// in the cluster
func (c Config) DiscoveryPort() int {
	return c.discoveryPort
}

// KeepAlivePeriod denotes whether the operating system should send
// keep-alive messages on the connection.
func (c Config) KeepAlivePeriod() time.Duration {
	return c.keepAlivePeriod
}

// MinimumPeersQuorum denotes the minimum number of peers
// required to form a cluster
func (c Config) MinimumPeersQuorum() int {
	return c.minimumPeersQuorum
}

// BootstrapTimeout for bootstrap control
//
// An distcache node checks operation status before taking any action for the
// cluster events, responding incoming requests and running API functions.
// Bootstrapping status is one of the most important checkpoints for an
// "operable" distcache node. BootstrapTimeout sets a deadline to check
// bootstrapping status without blocking indefinitely.
func (c Config) BootstrapTimeout() time.Duration {
	return c.bootstrapTimeout
}

// ReplicaCount is 1, by default.
func (c Config) ReplicaCount() int {
	return c.replicaCount
}

// DiscoveryProvider denotes the discovery provider to use to locate other distcache nodes
// in the cluster
func (c Config) DiscoveryProvider() discovery.Provider {
	return c.discoveryProvider
}

// Logger is a custom logger which you provide. If Logger is set, it will use
// this for the internal logger.
func (c Config) Logger() log.Logger {
	return c.logger
}

// JoinRetryInterval is the time gap between attempts to join an existing
// cluster.
func (c Config) JoinRetryInterval() time.Duration {
	return c.joinRetryInterval
}

// MaxJoinAttempts denotes the maximum number of attempts to join an existing
// cluster before forming a new one.
func (c Config) MaxJoinAttempts() int {
	return c.maxJoinAttempts
}

// Hasher returns the hasher
func (c Config) Hasher() hash.Hasher {
	return c.hasher
}

// KeySpaces defines the various keySpaces used by distcache
func (c Config) KeySpaces() []KeySpace {
	return c.keySpaces
}

// ShutdownTimeout returns the shutdown timeout
//
// distcache will broadcast a leave message but will not shut down the background
// listeners, meaning the node will continue participating in gossip and state
// updates.
//
// Sending a leave message will block until the leave message is successfully
// broadcast to a member of the cluster, if any exist or until a specified timeout
// is reached.
func (c Config) ShutdownTimeout() time.Duration {
	return c.shutdownTimeout
}

// ReadTimeout defines the read timeout. This timeout is used to read data from the distributed cache
func (c Config) ReadTimeout() time.Duration {
	return c.readTimeout
}

// WriteTimeout defines the write timeout used to set a key/value pair in the distributed cache engine
func (c Config) WriteTimeout() time.Duration {
	return c.writeTimeout
}

// TLSInfo returns the TLS Info.
// This option allows secure communication by setting a custom TLS configuration
// for encrypting data in transit.
func (c Config) TLSInfo() *TLSInfo {
	return c.tlsInfo
}

// Validate validates the distcache configuration
func (c Config) Validate() error {
	chain := validation.
		New(validation.FailFast()).
		AddAssertion(c.shutdownTimeout > 0, "shutdownTimeout is invalid").
		AddAssertion(c.joinRetryInterval > 0, "joinRetryInterval is invalid").
		AddAssertion(c.maxJoinAttempts > 1, "maxJoinAttempts is invalid").
		AddAssertion(c.bootstrapTimeout > 0, "bootstrapTimeout is invalid").
		AddAssertion(c.readTimeout > 0, "readTimeout is invalid").
		AddAssertion(c.writeTimeout > 0, "writeTimeout is invalid").
		AddAssertion(c.replicaCount > 0, "replicaCount is invalid").
		AddAssertion(c.discoveryProvider != nil, "discoveryProvider is invalid").
		AddAssertion(len(c.keySpaces) > 0, "keySpaces are required").
		AddAssertion(c.hasher != nil, "hasher is required").
		AddAssertion(c.minimumPeersQuorum > 0, "minimumPeersQuorum is required").
		AddValidator(validation.NewConditionalValidator(c.ifname == "", validation.NewEmptyStringValidator(c.bindAddr, "bindAddr"))).
		AddValidator(validation.NewConditionalValidator(c.ifname == "", validation.NewTCPAddressValidator(net.JoinHostPort(c.bindAddr, strconv.Itoa(c.bindPort))))).
		AddValidator(validation.NewConditionalValidator(c.ifname == "", validation.NewTCPAddressValidator(net.JoinHostPort(c.bindAddr, strconv.Itoa(c.discoveryPort)))))

	for _, keySpace := range c.keySpaces {
		chain = chain.
			AddValidator(validation.NewEmptyStringValidator(keySpace.Name(), "keySpace.Name")).
			AddAssertion(keySpace.DataSource() != nil, "keySpace.DataSource is required").
			AddAssertion(keySpace.MaxBytes() != 0, "keySpace.MaxBytes is required")
	}

	return chain.Validate()
}
