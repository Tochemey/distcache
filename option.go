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
	"time"

	"github.com/tochemey/distcache/hash"
	"github.com/tochemey/distcache/log"
	"github.com/tochemey/distcache/otel"
)

// Option defines a configuration option that can be applied to a Config.
//
// Implementations of this interface modify the configuration when applied.
type Option interface {
	// Apply applies the configuration option to the given Config instance.
	Apply(config *Config)
}

// enforce compilation error if OptionFunc does not implement Option
var _ Option = OptionFunc(nil)

// OptionFunc is a function type that implements the Option interface.
//
// It allows functions to be used as configuration options for Config.
type OptionFunc func(config *Config)

// Apply applies the OptionFunc to the given Config.
//
// This enables the use of functions as dynamic configuration options.
func (f OptionFunc) Apply(config *Config) {
	f(config)
}

// WithLogger configures the config to use a custom logger.
//
// Parameters:
//   - logger: An instance of log.Logger used for logging.
//
// Returns:
//   - An Option that applies the custom logger to the Config.
//
// Usage:
//
//	config := NewConfig(WithLogger(myLogger))
func WithLogger(logger log.Logger) Option {
	return OptionFunc(
		func(config *Config) {
			config.logger = logger
		},
	)
}

// WithBindAddr configures the config to use a custom bind address.
//
// BindAddr denotes the address that distcache will bind to for communication
// with other distcache nodes.
//
// Parameters:
//   - addr: The bind address to use.
//
// Returns:
//   - An Option that applies the custom bind addr to the Config.
//
// Usage:
//
//	config := NewConfig(WithBindAddr(addr))
func WithBindAddr(addr string) Option {
	return OptionFunc(
		func(config *Config) {
			config.bindAddr = addr
		},
	)
}

// WithInterface configures the config to use a custom bind interface.
//
// Interface denotes a binding interface. It can be used instead of BindAddr
// if the interface is known but not the address. If both are provided, then
// distcache verifies that the interface has the bind address that is provided.
//
// Parameters:
//   - ifname: The bind interface to use.
//
// Returns:
//   - An Option that applies the custom bind addr to the Config.
//
// Usage:
//
//	config := NewConfig(WithInterface(ifname))
func WithInterface(ifname string) Option {
	return OptionFunc(
		func(config *Config) {
			config.ifname = ifname
		},
	)
}

// WithBindPort configures the config to use a custom bind port.
//
// BindPort denotes the address that distcache will bind to for communication
// with other distcache nodes.
//
// Parameters:
//   - port: The bind port to use.
//
// Returns:
//   - An Option that applies the custom bind port to the Config.
//
// Usage:
//
//	config := NewConfig(WithBindPort(port))
func WithBindPort(port int) Option {
	return OptionFunc(func(config *Config) {
		config.bindPort = port
	})
}

// WithDiscoveryPort configures the config to use a custom discovery port.
//
// Parameters:
//   - port: The discovery port to use.
//
// Returns:
//   - An Option that applies the custom discovery port to the Config.
//
// Usage:
//
//	config := NewConfig(WithDiscoveryPort(port))
func WithDiscoveryPort(port int) Option {
	return OptionFunc(func(config *Config) {
		config.discoveryPort = port
	})
}

// WithKeepAlivePeriod configures the config to use a custom keepAlive period.
//
// KeepAlivePeriod denotes whether the operating system should send
// keep-alive messages on the connection.
//
// Parameters:
//   - period: The keep alive period.
//
// Returns:
//   - An Option that applies the custom keep alive period to the Config.
//
// Usage:
//
//	config := NewConfig(WithKeepAlivePeriod(period))
func WithKeepAlivePeriod(period time.Duration) Option {
	return OptionFunc(func(config *Config) {
		config.keepAlivePeriod = period
	})
}

// WithBootstrapTimeout configures the config to use a custom bootstrap timeout.
//
// A distcache node checks operation status before taking any action for the
// cluster events, responding incoming requests and running API functions.
// Bootstrapping status is one of the most important checkpoints for an
// "operable" distcache node. BootstrapTimeout sets a deadline to check
// bootstrapping status without blocking indefinitely.
//
// Parameters:
//   - timeout: The custom bootstrap timeout.
//
// Returns:
//   - An Option that applies the custom bootstrap timeout to the Config.
//
// Usage:
//
//	config := NewConfig(WithBootstrapTimeout(timeout))
func WithBootstrapTimeout(timeout time.Duration) Option {
	return OptionFunc(func(config *Config) {
		config.bootstrapTimeout = timeout
	})
}

// WithReplicaCount configures the config to use a custom replica count.
//
// Parameters:
//   - count: The custom replica count.
//
// Returns:
//   - An Option that applies the custom replica count to the Config.
//
// Usage:
//
//	config := NewConfig(WithReplicaCount(count))
func WithReplicaCount(count int) Option {
	return OptionFunc(func(config *Config) {
		config.replicaCount = count
	})
}

// WithShutdownTimeout configures the config to use a custom shutdown timeout.
//
// distcache will broadcast a leave message but will not shut down the background
// listeners, meaning the node will continue participating in gossip and state
// updates.
//
// Sending a leave message will block until the leave message is successfully
// broadcast to a member of the cluster, if any exist or until a specified timeout
// is reached.
//
// Parameters:
//   - timeout: The custom shutdown timeout.
//
// Returns:
//   - An Option that applies the custom shutdown timeout to the Config.
//
// Usage:
//
//	config := NewConfig(WithShutdownTimeout(timeout))
func WithShutdownTimeout(timeout time.Duration) Option {
	return OptionFunc(
		func(config *Config) {
			config.shutdownTimeout = timeout
		},
	)
}

// WithJoinRetryInterval configures the config to use a custom join retry interval.
//
// JoinRetryInterval is the time gap between attempts to join an existing
// cluster.
//
// Parameters:
//   - interval: The custom join retry interval.
//
// Returns:
//   - An Option that applies the custom retry interval to the Config.
//
// Usage:
//
//	config := NewConfig(WithJoinRetryInterval(interval))
func WithJoinRetryInterval(interval time.Duration) Option {
	return OptionFunc(func(config *Config) {
		config.joinRetryInterval = interval
	})
}

// WithMaxJoinAttempts configures the config to use a custom max join attempts.
//
// MaxJoinAttempts denotes the maximum number of attempts to join an existing
// cluster before forming a new one.
//
// Parameters:
//   - maxAttempts: The custom maximum join attempts.
//
// Returns:
//   - An Option that applies the custom maximum join attempts to the Config.
//
// Usage:
//
//	config := NewConfig(WithMaxJoinAttempts(maxAttempts))
func WithMaxJoinAttempts(maxAttempts int) Option {
	return OptionFunc(func(config *Config) {
		config.maxJoinAttempts = maxAttempts
	})
}

// WithMinimumPeersQuorum configures the config to use a custom minimum peers quorum.
//
// MinimumPeersQuorum denotes the minimum number of peers
// required to form a cluster.
//
// Parameters:
//   - minQuorum: The custom minimum peers quorum.
//
// Returns:
//   - An Option that applies the custom minimum peers quorum to the Config.
//
// Usage:
//
//	config := NewConfig(WithMinimumPeersQuorum(minQuorum)
func WithMinimumPeersQuorum(minQuorum int) Option {
	return OptionFunc(func(config *Config) {
		config.minimumPeersQuorum = minQuorum
	})
}

// WithTLS configures the cache engine to use the specified TLS settings for both the Server and Client.
//
// Ensure that both the Server and Client are configured with the same
// root Certificate Authority (CA) to enable successful handshake and
// mutual authentication.
//
// This option allows secure communication by setting a custom TLS configuration
// for encrypting data in transit.
//
// Parameters:
//   - info: A pointer to TLSInfo struct that defines TLS settings,
//     such as certificates, cipher suites, and authentication options.
//
// Returns:
//   - Option: A functional option that applies the TLS configuration to the cache engine.
func WithTLS(info *TLSInfo) Option {
	return OptionFunc(func(config *Config) {
		config.tlsInfo = info
	})
}

// WithHasher configures the cache engine to use a custom hashing function.
//
// This option allows you to specify a custom hash function for key hashing,
// which can be useful for controlling hash collisions, performance, or cryptographic security.
//
// Parameters:
//   - hashFn: A hash.Hasher that defines the hashing function to be used for key hashing.
//
// Returns:
//   - Option: A functional option that applies the specified hashing function to the cache engine.
func WithHasher(hashFn hash.Hasher) Option {
	return OptionFunc(func(config *Config) {
		config.hasher = hashFn
	})
}

// WithLabel configures the distcache node with a specific label.
//
// This label is used to identify the distcache node.
// It is required that all nodes in the cluster use the same label
// to ensure proper identification and grouping.
//
// Parameters:
//   - label: A string representing the label for the distcache node.
//
// Returns:
//   - Option: A functional option that applies the specified label to the distcache node.
func WithLabel(label string) Option {
	return OptionFunc(func(config *Config) {
		config.label = label
	})
}

// WithMetrics configures distcache to use the provided OpenTelemetry metric settings.
//
// Use this option to supply a pre-built otel.MetricConfig (e.g., a custom MeterProvider
// or instrumentation name) for creating meters and instruments.
//
// Parameters:
//   - metricsConfig: A pointer to otel.MetricConfig that defines the MeterProvider and
//     instrumentation name to be used.
//
// Returns:
//   - Option: A functional option that applies the metric configuration to distcache.
//
// Usage:
//
//	cfg := NewConfig(WithMetrics(otel.NewMetricConfig()))
func WithMetrics(metricsConfig *otel.MetricConfig) Option {
	return OptionFunc(func(config *Config) {
		config.metricConfig = metricsConfig
	})
}

// WithTracing configures distcache to use the provided OpenTelemetry tracing settings.
//
// Use this option to supply a pre-built otel.TracerConfig (e.g., a custom TracerProvider
// or instrumentation name) for creating tracers and spans.
//
// Parameters:
//   - traceConfig: A pointer to otel.TracerConfig that defines the TracerProvider and
//     instrumentation name to be used.
//
// Returns:
//   - Option: A functional option that applies the tracing configuration to distcache.
//
// Usage:
//
//	cfg := NewConfig(WithTracing(otel.NewTracerConfig()))
func WithTracing(traceConfig *otel.TracerConfig) Option {
	return OptionFunc(func(config *Config) {
		config.traceConfig = traceConfig
	})
}
