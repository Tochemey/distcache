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
	"net"
	"os"
	"strconv"
	"time"

	"github.com/tochemey/distcache/admin"
	"github.com/tochemey/distcache/discovery"
	"github.com/tochemey/distcache/hash"
	"github.com/tochemey/distcache/internal/validation"
	"github.com/tochemey/distcache/log"
	"github.com/tochemey/distcache/otel"
	"github.com/tochemey/distcache/warmup"
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
	DefaultBootstrapTimeout = 30 * time.Second

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
	DefaultShutdownTimeout = 30 * time.Second

	// DefaultReadTimeout is the default timeout for reading data from the distributed cache
	DefaultReadTimeout = 5 * time.Second

	// DefaultWriteTimeout is default write timeout to write data into the distributed cache
	DefaultWriteTimeout = 5 * time.Second

	// MinimumMemberCountQuorum denotes minimum required count of members to form
	// a cluster.
	MinimumMemberCountQuorum = 1
)

// KeySpaceConfig defines optional, keyspace-specific behaviors.
//
// Zero values mean "inherit the engine defaults" unless otherwise stated.
type KeySpaceConfig struct {
	// MaxBytes overrides KeySpace.MaxBytes when greater than zero.
	MaxBytes int64
	// DefaultTTL applies when an entry has no explicit expiration.
	DefaultTTL time.Duration
	// ReadTimeout overrides the engine ReadTimeout when greater than zero.
	ReadTimeout time.Duration
	// WriteTimeout overrides the engine WriteTimeout when greater than zero.
	WriteTimeout time.Duration
	// WarmKeys lists keys to prefetch when the cluster topology changes.
	WarmKeys []string
	// RateLimit configures per-keyspace rate limiting.
	// When nil, the engine-level rate limiter is used.
	RateLimit *RateLimitConfig
	// CircuitBreaker configures per-keyspace circuit breaking.
	// When nil, the engine-level circuit breaker is used.
	CircuitBreaker *CircuitBreakerConfig
}

// RateLimitConfig defines a token bucket rate limiter configuration.
//
// The limiter enforces RequestsPerSecond with a configurable Burst capacity.
// When WaitTimeout is zero, requests that exceed the rate limit fail fast.
// When WaitTimeout is greater than zero, Fetch waits up to that duration for
// a token before returning ErrDataSourceRateLimited.
type RateLimitConfig struct {
	// RequestsPerSecond defines the steady-state rate limit.
	RequestsPerSecond float64
	// Burst is the maximum burst size allowed by the limiter.
	Burst int
	// WaitTimeout bounds how long Fetch waits for a token; zero means fail fast.
	WaitTimeout time.Duration
}

// CircuitBreakerConfig defines a consecutive-failure circuit breaker.
//
// Algorithm:
//   - Closed: all requests pass; failures are counted consecutively.
//   - Open: requests are rejected until ResetTimeout elapses.
//   - Half-open: exactly one request is allowed to probe recovery.
//     Success closes the breaker; failure re-opens it.
type CircuitBreakerConfig struct {
	// FailureThreshold is the number of consecutive failures before opening.
	FailureThreshold int
	// ResetTimeout is how long the breaker stays open before probing.
	ResetTimeout time.Duration
}

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

	// keySpaceConfigs holds optional per-keyspace overrides.
	keySpaceConfigs map[string]KeySpaceConfig

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

	// Label is an optional label to identify the distcache node.
	// This label should be the same for all nodes in the cluster.
	label string

	traceConfig  *otel.TracerConfig
	metricConfig *otel.MetricConfig
	adminConfig  *admin.Config
	warmupConfig *warmup.Config

	// dataSourcePolicy applies to all keyspaces unless overridden.
	dataSourcePolicy *dataSourceConfig
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
		label:              "distcache",
		traceConfig:        nil,
		metricConfig:       nil,
		adminConfig:        nil,
		warmupConfig:       nil,
		dataSourcePolicy:   nil,
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

// Label returns the distcache node label
// This label is used to identify the distcache node.
// This label should be the same for all nodes in the cluster.
func (c Config) Label() string {
	return c.label
}

// TraceConfig returns the trace configuration
func (c Config) TraceConfig() *otel.TracerConfig {
	return c.traceConfig
}

// MetricConfig returns the metric configuration
func (c Config) MetricConfig() *otel.MetricConfig {
	return c.metricConfig
}

// AdminConfig returns the admin server configuration.
func (c Config) AdminConfig() *admin.Config {
	return c.adminConfig
}

// WarmupConfig returns the warmup configuration.
func (c Config) WarmupConfig() *warmup.Config {
	return c.warmupConfig
}

// Validate validates the distcache configuration
func (c Config) Validate() error {
	chain := validation.
		New(validation.FailFast()).
		AddValidator(validation.NewEmptyStringValidator("label", c.label)).
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

	seenKeySpaces := make(map[string]struct{})
	for _, keySpace := range c.keySpaces {
		if _, exists := seenKeySpaces[keySpace.Name()]; exists {
			chain = chain.AddAssertion(false, "keySpace.Name must be unique")
		} else {
			seenKeySpaces[keySpace.Name()] = struct{}{}
		}
		chain = chain.
			AddValidator(validation.NewEmptyStringValidator(keySpace.Name(), "keySpace.Name")).
			AddAssertion(keySpace.DataSource() != nil, "keySpace.DataSource is required").
			AddAssertion(keySpace.MaxBytes() != 0, "keySpace.MaxBytes is required")
	}
	for name, cfg := range c.keySpaceConfigs {
		if _, exists := seenKeySpaces[name]; !exists {
			chain = chain.AddAssertion(false, "keySpace."+name+" config has no matching keyspace")
			continue
		}
		chain = chain.AddValidator(newKeySpaceConfigValidator(name, cfg))
	}

	if c.adminConfig != nil {
		cfg := c.adminConfig.Normalize()
		chain = chain.
			AddAssertion(cfg.ListenAddr != "", "adminConfig.listenAddr is required").
			AddAssertion(cfg.ReadTimeout > 0, "adminConfig.readTimeout is invalid").
			AddAssertion(cfg.WriteTimeout > 0, "adminConfig.writeTimeout is invalid").
			AddAssertion(cfg.IdleTimeout > 0, "adminConfig.idleTimeout is invalid")
	}

	if c.warmupConfig != nil {
		cfg := c.warmupConfig.Normalize()
		chain = chain.
			AddAssertion(cfg.MaxHotKeys > 0, "warmupConfig.maxHotKeys is invalid").
			AddAssertion(cfg.MinHits > 0, "warmupConfig.minHits is invalid").
			AddAssertion(cfg.Concurrency > 0, "warmupConfig.concurrency is invalid").
			AddAssertion(cfg.Timeout > 0, "warmupConfig.timeout is invalid")
	}

	if c.dataSourcePolicy != nil {
		chain = chain.AddValidator(c.dataSourcePolicy)
	}

	return chain.Validate()
}

type keySpaceConfigValidator struct {
	name string
	cfg  KeySpaceConfig
}

func newKeySpaceConfigValidator(name string, cfg KeySpaceConfig) keySpaceConfigValidator {
	return keySpaceConfigValidator{name: name, cfg: cfg}
}

func (v keySpaceConfigValidator) Validate() error {
	chain := validation.New(validation.AllErrors())
	if v.cfg.MaxBytes < 0 {
		chain = chain.AddAssertion(false, "keySpace."+v.name+".maxBytes is invalid")
	}
	if v.cfg.DefaultTTL < 0 {
		chain = chain.AddAssertion(false, "keySpace."+v.name+".defaultTTL is invalid")
	}
	if v.cfg.ReadTimeout < 0 {
		chain = chain.AddAssertion(false, "keySpace."+v.name+".readTimeout is invalid")
	}
	if v.cfg.WriteTimeout < 0 {
		chain = chain.AddAssertion(false, "keySpace."+v.name+".writeTimeout is invalid")
	}
	if policy := newDataSourceConfig(v.cfg.RateLimit, v.cfg.CircuitBreaker); policy != nil {
		if err := policy.Validate(); err != nil {
			chain = chain.AddValidator(validation.NewBooleanValidator(false, err.Error()))
		}
	}
	return chain.Validate()
}
