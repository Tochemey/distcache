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
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/travisjeffery/go-dynaport"

	"github.com/tochemey/distcache/admin"
	"github.com/tochemey/distcache/discovery/nats"
	"github.com/tochemey/distcache/hash"
	"github.com/tochemey/distcache/internal/size"
	"github.com/tochemey/distcache/log"
	"github.com/tochemey/distcache/otel"
	"github.com/tochemey/distcache/warmup"
)

func TestConfig(t *testing.T) {
	t.Run("WithValid config", func(t *testing.T) {
		// start the NATS server
		srv := startNatsServer(t)
		serverAddress := srv.Addr().String()
		// generate the ports for the single startNode
		ports := dynaport.Get(1)
		discoveryPort := ports[0]
		host := "127.0.0.1"

		// create the nats discovery provider
		provider := nats.NewDiscovery(&nats.Config{
			Server:        fmt.Sprintf("nats://%s", serverAddress),
			Subject:       "example",
			Host:          host,
			DiscoveryPort: discoveryPort,
		})

		dataSource := NewMockDataSource()
		keySpaces := []KeySpace{
			NewMockKeySpace("users", size.MB, dataSource),
		}

		config := NewConfig(provider, keySpaces)
		err := config.Validate()
		require.NoError(t, err)

		err = provider.Close()
		require.NoError(t, err)
		srv.Shutdown()
	})
}

func TestConfigAccessors(t *testing.T) {
	keyspace := NewMockKeySpace("users", size.MB, NewMockDataSource())
	hashFn := hash.DefaultHasher()
	tlsInfo := &TLSInfo{
		ClientTLS: &tls.Config{InsecureSkipVerify: true}, // nolint:gosec
		ServerTLS: &tls.Config{InsecureSkipVerify: true}, // nolint:gosec
	}
	traceCfg := otel.NewTracerConfig()
	metricCfg := otel.NewMetricConfig()
	adminCfg := admin.Config{ListenAddr: "127.0.0.1:9090", BasePath: "/admin"}
	warmCfg := warmup.Config{MaxHotKeys: 10}
	rateLimit := RateLimitConfig{RequestsPerSecond: 5, Burst: 2, WaitTimeout: time.Second}
	circuitBreaker := CircuitBreakerConfig{FailureThreshold: 2, ResetTimeout: 5 * time.Second}

	cfg := NewConfig(
		MockProvider{},
		[]KeySpace{keyspace},
		WithLogger(log.DiscardLogger),
		WithBindAddr("127.0.0.1"),
		WithInterface("lo0"),
		WithBindPort(1234),
		WithDiscoveryPort(5678),
		WithKeepAlivePeriod(12*time.Second),
		WithBootstrapTimeout(8*time.Second),
		WithJoinRetryInterval(3*time.Second),
		WithMaxJoinAttempts(4),
		WithMinimumPeersQuorum(2),
		WithReplicaCount(2),
		WithShutdownTimeout(9*time.Second),
		WithLabel("distcache-test"),
		WithHasher(hashFn),
		WithTLS(tlsInfo),
		WithTracing(traceCfg),
		WithMetrics(metricCfg),
		WithAdminConfig(adminCfg),
		WithWarmup(warmCfg),
		WithRateLimiter(rateLimit),
		WithCircuitBreaker(circuitBreaker),
	)
	cfg.readTimeout = 11 * time.Second
	cfg.writeTimeout = 13 * time.Second

	require.Equal(t, "lo0", cfg.Interface())
	require.Equal(t, "127.0.0.1", cfg.BindAddr())
	require.Equal(t, 1234, cfg.BindPort())
	require.Equal(t, 5678, cfg.DiscoveryPort())
	require.Equal(t, 12*time.Second, cfg.KeepAlivePeriod())
	require.Equal(t, 2, cfg.MinimumPeersQuorum())
	require.Equal(t, 8*time.Second, cfg.BootstrapTimeout())
	require.Equal(t, 2, cfg.ReplicaCount())
	require.Equal(t, MockProvider{}, cfg.DiscoveryProvider())
	require.Equal(t, log.DiscardLogger, cfg.Logger())
	require.Equal(t, 3*time.Second, cfg.JoinRetryInterval())
	require.Equal(t, 4, cfg.MaxJoinAttempts())
	require.Equal(t, hashFn, cfg.Hasher())
	require.Equal(t, []KeySpace{keyspace}, cfg.KeySpaces())
	require.Equal(t, 9*time.Second, cfg.ShutdownTimeout())
	require.Equal(t, 11*time.Second, cfg.ReadTimeout())
	require.Equal(t, 13*time.Second, cfg.WriteTimeout())
	require.Equal(t, tlsInfo, cfg.TLSInfo())
	require.Equal(t, "distcache-test", cfg.Label())
	require.Equal(t, traceCfg, cfg.TraceConfig())
	require.Equal(t, metricCfg, cfg.MetricConfig())
	require.Equal(t, &adminCfg, cfg.AdminConfig())
	require.Equal(t, &warmCfg, cfg.WarmupConfig())
	require.NotNil(t, cfg.dataSourcePolicy)
	require.Equal(t, rateLimit, *cfg.dataSourcePolicy.RateLimit)
	require.Equal(t, circuitBreaker, *cfg.dataSourcePolicy.CircuitBreaker)
}

func TestConfigValidateDuplicateKeySpaces(t *testing.T) {
	ks1 := NewMockKeySpace("dup", size.MB, NewMockDataSource())
	ks2 := NewMockKeySpace("dup", size.MB, NewMockDataSource())
	cfg := NewConfig(MockProvider{}, []KeySpace{ks1, ks2})

	err := cfg.Validate()
	require.Error(t, err)
}

func TestConfigValidateAdminWarmupAndProtector(t *testing.T) {
	keyspace := NewMockKeySpace("users", size.MB, NewMockDataSource())

	t.Run("admin config without listen addr fails", func(t *testing.T) {
		cfg := NewConfig(MockProvider{}, []KeySpace{keyspace}, WithAdminConfig(admin.Config{}))
		err := cfg.Validate()
		require.Error(t, err)
	})

	t.Run("warmup config normalizes and validates", func(t *testing.T) {
		cfg := NewConfig(MockProvider{}, []KeySpace{keyspace}, WithWarmup(warmup.Config{}))
		err := cfg.Validate()
		require.NoError(t, err)
	})

	t.Run("data source protector validation failure", func(t *testing.T) {
		cfg := NewConfig(MockProvider{}, []KeySpace{keyspace}, WithRateLimiter(RateLimitConfig{RequestsPerSecond: 0}))
		err := cfg.Validate()
		require.Error(t, err)
	})
}

func TestConfigValidateKeySpaceProtector(t *testing.T) {
	keyspace := NewMockKeySpace("users", size.MB, NewMockDataSource())
	cfg := NewConfig(
		MockProvider{},
		[]KeySpace{keyspace},
		WithKeySpaceRateLimiter("users", RateLimitConfig{RequestsPerSecond: 0}),
	)

	err := cfg.Validate()
	require.Error(t, err)
}
