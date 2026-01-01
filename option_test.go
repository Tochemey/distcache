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
	"crypto/tls"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/tochemey/distcache/admin"
	"github.com/tochemey/distcache/hash"
	"github.com/tochemey/distcache/log"
	"github.com/tochemey/distcache/otel"
	"github.com/tochemey/distcache/warmup"
)

func TestOptions(t *testing.T) {
	tlsConfig := &tls.Config{InsecureSkipVerify: true} // nolint
	metricConfig := otel.NewMetricConfig()
	traceConfig := otel.NewTracerConfig()
	tlsInfo := &TLSInfo{
		ClientTLS: tlsConfig,
		ServerTLS: tlsConfig,
	}
	rateLimit := RateLimitConfig{RequestsPerSecond: 5, Burst: 2, WaitTimeout: time.Second}
	circuitBreaker := CircuitBreakerConfig{FailureThreshold: 2, ResetTimeout: 5 * time.Second}
	keySpaceName := "users"
	keySpaceWarmKeys := []string{"a", "b"}

	hashFn := hash.DefaultHasher()
	testCases := []struct {
		name     string
		option   Option
		expected Config
	}{
		{
			name:     "WithLogger",
			option:   WithLogger(log.DiscardLogger),
			expected: Config{logger: log.DiscardLogger},
		},
		{
			name:     "WithBindAddr",
			option:   WithBindAddr("0.0.0.0"),
			expected: Config{bindAddr: "0.0.0.0"},
		},
		{
			name:     "WithInterface",
			option:   WithInterface("eth0"),
			expected: Config{ifname: "eth0"},
		},
		{
			name:     "WithBindPort",
			option:   WithBindPort(8080),
			expected: Config{bindPort: 8080},
		},
		{
			name:     "WithDiscoveryPort",
			option:   WithDiscoveryPort(8080),
			expected: Config{discoveryPort: 8080},
		},
		{
			name:     "WithKeepAlivePeriod",
			option:   WithKeepAlivePeriod(30 * time.Second),
			expected: Config{keepAlivePeriod: 30 * time.Second},
		},
		{
			name:     "WithBootstrapTimeout",
			option:   WithBootstrapTimeout(30 * time.Second),
			expected: Config{bootstrapTimeout: 30 * time.Second},
		},
		{
			name:     "WithReplicaCount",
			option:   WithReplicaCount(2),
			expected: Config{replicaCount: 2},
		},
		{
			name:     "WithShutdownTimeout",
			option:   WithShutdownTimeout(30 * time.Second),
			expected: Config{shutdownTimeout: 30 * time.Second},
		},
		{
			name:     "WithJoinRetryInterval",
			option:   WithJoinRetryInterval(30 * time.Second),
			expected: Config{joinRetryInterval: 30 * time.Second},
		},
		{
			name:     "WithMaxJoinAttempts",
			option:   WithMaxJoinAttempts(2),
			expected: Config{maxJoinAttempts: 2},
		},
		{
			name:     "WithMinimumPeersQuorum",
			option:   WithMinimumPeersQuorum(2),
			expected: Config{minimumPeersQuorum: 2},
		},
		{
			name:     "WithTLS",
			option:   WithTLS(tlsInfo),
			expected: Config{tlsInfo: tlsInfo},
		},
		{
			name:     "WithHasher",
			option:   WithHasher(hashFn),
			expected: Config{hasher: hashFn},
		},
		{
			name:     "WithLabel",
			option:   WithLabel("my-distcache"),
			expected: Config{label: "my-distcache"},
		},
		{
			name:     "WithTracing",
			option:   WithTracing(traceConfig),
			expected: Config{traceConfig: traceConfig},
		},
		{
			name:     "WithMetrics",
			option:   WithMetrics(metricConfig),
			expected: Config{metricConfig: metricConfig},
		},
		{
			name:     "WithAdminServer",
			option:   WithAdminServer("127.0.0.1:9000"),
			expected: Config{adminConfig: &admin.Config{ListenAddr: "127.0.0.1:9000"}},
		},
		{
			name:     "WithAdminConfig",
			option:   WithAdminConfig(admin.Config{ListenAddr: "127.0.0.1:9100", BasePath: "/admin"}),
			expected: Config{adminConfig: &admin.Config{ListenAddr: "127.0.0.1:9100", BasePath: "/admin"}},
		},
		{
			name:     "WithWarmup",
			option:   WithWarmup(warmup.Config{MaxHotKeys: 10}),
			expected: Config{warmupConfig: &warmup.Config{MaxHotKeys: 10}},
		},
		{
			name:   "WithRateLimiter",
			option: WithRateLimiter(rateLimit),
			expected: Config{dataSourcePolicy: &dataSourceConfig{
				RateLimit: &rateLimit,
			}},
		},
		{
			name:   "WithCircuitBreaker",
			option: WithCircuitBreaker(circuitBreaker),
			expected: Config{dataSourcePolicy: &dataSourceConfig{
				CircuitBreaker: &circuitBreaker,
			}},
		},
		{
			name:   "WithKeySpaceMaxBytes",
			option: WithKeySpaceMaxBytes(keySpaceName, 128),
			expected: Config{keySpaceConfigs: map[string]KeySpaceConfig{
				keySpaceName: {MaxBytes: 128},
			}},
		},
		{
			name:   "WithKeySpaceDefaultTTL",
			option: WithKeySpaceDefaultTTL(keySpaceName, time.Second),
			expected: Config{keySpaceConfigs: map[string]KeySpaceConfig{
				keySpaceName: {DefaultTTL: time.Second},
			}},
		},
		{
			name:   "WithKeySpaceReadTimeout",
			option: WithKeySpaceReadTimeout(keySpaceName, 3*time.Second),
			expected: Config{keySpaceConfigs: map[string]KeySpaceConfig{
				keySpaceName: {ReadTimeout: 3 * time.Second},
			}},
		},
		{
			name:   "WithKeySpaceWriteTimeout",
			option: WithKeySpaceWriteTimeout(keySpaceName, 4*time.Second),
			expected: Config{keySpaceConfigs: map[string]KeySpaceConfig{
				keySpaceName: {WriteTimeout: 4 * time.Second},
			}},
		},
		{
			name:   "WithKeySpaceWarmKeys",
			option: WithKeySpaceWarmKeys(keySpaceName, keySpaceWarmKeys),
			expected: Config{keySpaceConfigs: map[string]KeySpaceConfig{
				keySpaceName: {WarmKeys: keySpaceWarmKeys},
			}},
		},
		{
			name:   "WithKeySpaceRateLimiter",
			option: WithKeySpaceRateLimiter(keySpaceName, rateLimit),
			expected: Config{keySpaceConfigs: map[string]KeySpaceConfig{
				keySpaceName: {RateLimit: &rateLimit},
			}},
		},
		{
			name:   "WithKeySpaceCircuitBreaker",
			option: WithKeySpaceCircuitBreaker(keySpaceName, circuitBreaker),
			expected: Config{keySpaceConfigs: map[string]KeySpaceConfig{
				keySpaceName: {CircuitBreaker: &circuitBreaker},
			}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var config Config
			tc.option.Apply(&config)
			assert.Equal(t, tc.expected, config)
		})
	}
}
