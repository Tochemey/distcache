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
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/tochemey/distcache/internal/validation"
)

type breakerState int

const (
	breakerClosed breakerState = iota
	breakerOpen
	breakerHalfOpen
)

type dataSourceConfig struct {
	// RateLimit configures request rate limiting. Nil disables rate limiting.
	RateLimit *RateLimitConfig
	// CircuitBreaker configures circuit breaking. Nil disables circuit breaking.
	CircuitBreaker *CircuitBreakerConfig
}

func newDataSourceConfig(rateLimit *RateLimitConfig, circuitBreaker *CircuitBreakerConfig) *dataSourceConfig {
	if rateLimit == nil && circuitBreaker == nil {
		return nil
	}
	return &dataSourceConfig{
		RateLimit:      rateLimit,
		CircuitBreaker: circuitBreaker,
	}
}

func mergeDataSourceConfig(base *dataSourceConfig, rateLimit *RateLimitConfig, circuitBreaker *CircuitBreakerConfig) *dataSourceConfig {
	if base == nil && rateLimit == nil && circuitBreaker == nil {
		return nil
	}
	merged := &dataSourceConfig{}
	if base != nil {
		merged.RateLimit = base.RateLimit
		merged.CircuitBreaker = base.CircuitBreaker
	}
	if rateLimit != nil {
		merged.RateLimit = rateLimit
	}
	if circuitBreaker != nil {
		merged.CircuitBreaker = circuitBreaker
	}
	if merged.RateLimit == nil && merged.CircuitBreaker == nil {
		return nil
	}
	return merged
}

// Apply wraps the provided DataSource with the configured protections.
func (x *dataSourceConfig) Apply(source DataSource) DataSource {
	if x == nil {
		return source
	}
	if source == nil {
		return nil
	}

	limiter := newRateLimiter(x.RateLimit)
	breaker := newCircuitBreaker(x.CircuitBreaker)
	if limiter == nil && breaker == nil {
		return source
	}

	return &dataSourceWrapper{
		source:  source,
		limiter: limiter,
		breaker: breaker,
	}
}

// Validate validates the protection configuration.
func (x *dataSourceConfig) Validate() error {
	if x == nil {
		return nil
	}
	chain := validation.New(validation.AllErrors())
	if x.RateLimit != nil {
		chain = chain.
			AddAssertion(x.RateLimit.RequestsPerSecond > 0, "rateLimit.requestsPerSecond is invalid").
			AddAssertion(x.RateLimit.Burst >= 0, "rateLimit.burst is invalid").
			AddAssertion(x.RateLimit.WaitTimeout >= 0, "rateLimit.waitTimeout is invalid")
	}
	if x.CircuitBreaker != nil {
		chain = chain.
			AddAssertion(x.CircuitBreaker.FailureThreshold > 0, "circuitBreaker.failureThreshold is invalid").
			AddAssertion(x.CircuitBreaker.ResetTimeout > 0, "circuitBreaker.resetTimeout is invalid")
	}
	return chain.Validate()
}

type rateLimiter struct {
	limiter     *rate.Limiter
	waitTimeout time.Duration
}

func newRateLimiter(cfg *RateLimitConfig) *rateLimiter {
	if cfg == nil {
		return nil
	}
	if cfg.RequestsPerSecond <= 0 {
		return nil
	}
	burst := cfg.Burst
	if burst < 0 {
		burst = 0
	}
	return &rateLimiter{
		limiter:     rate.NewLimiter(rate.Limit(cfg.RequestsPerSecond), burst),
		waitTimeout: cfg.WaitTimeout,
	}
}

func (r *rateLimiter) Wait(ctx context.Context) error {
	if r.waitTimeout == 0 {
		if !r.limiter.Allow() {
			return ErrDataSourceRateLimited
		}
		return nil
	}

	waitCtx, cancel := context.WithTimeout(ctx, r.waitTimeout)
	defer cancel()
	if err := r.limiter.Wait(waitCtx); err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return err
		}
		if strings.Contains(err.Error(), "would exceed context deadline") {
			return fmt.Errorf("%w: %v", context.DeadlineExceeded, err)
		}
		return err
	}
	return nil
}

// circuitBreaker implements a consecutive-failure circuit breaker.
//
// Algorithm:
//   - Closed: requests are allowed. After FailureThreshold consecutive failures,
//     the breaker opens and rejects all requests.
//   - Open: requests are rejected until ResetTimeout elapses.
//   - Half-open: exactly one request is allowed. Success closes the breaker,
//     failure re-opens it.
//
// The implementation is concurrency-safe and guarantees at most one in-flight
// request while half-open.
type circuitBreaker struct {
	mu               sync.Mutex
	state            breakerState
	failures         int
	threshold        int
	resetTimeout     time.Duration
	openedAt         time.Time
	halfOpenInflight bool
}

func newCircuitBreaker(cfg *CircuitBreakerConfig) *circuitBreaker {
	if cfg == nil {
		return nil
	}
	if cfg.FailureThreshold <= 0 || cfg.ResetTimeout <= 0 {
		return nil
	}
	return &circuitBreaker{
		state:        breakerClosed,
		threshold:    cfg.FailureThreshold,
		resetTimeout: cfg.ResetTimeout,
	}
}

func (c *circuitBreaker) Allow() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch c.state {
	case breakerClosed:
		return true
	case breakerOpen:
		if time.Since(c.openedAt) >= c.resetTimeout {
			c.state = breakerHalfOpen
			if c.halfOpenInflight {
				return false
			}
			c.halfOpenInflight = true
			return true
		}
		return false
	case breakerHalfOpen:
		if c.halfOpenInflight {
			return false
		}
		c.halfOpenInflight = true
		return true
	default:
		return false
	}
}

func (c *circuitBreaker) OnSuccess() {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch c.state {
	case breakerHalfOpen:
		c.state = breakerClosed
		c.failures = 0
		c.halfOpenInflight = false
	case breakerClosed:
		c.failures = 0
	}
}

func (c *circuitBreaker) OnFailure() {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch c.state {
	case breakerHalfOpen:
		c.state = breakerOpen
		c.openedAt = time.Now()
		c.halfOpenInflight = false
	case breakerClosed:
		c.failures++
		if c.failures >= c.threshold {
			c.state = breakerOpen
			c.openedAt = time.Now()
		}
	}
}

func (c *circuitBreaker) Abort() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state == breakerHalfOpen {
		c.halfOpenInflight = false
	}
}

func (r RateLimitConfig) String() string {
	return fmt.Sprintf("rateLimit[rps=%.2f, burst=%d]", r.RequestsPerSecond, r.Burst)
}

type dataSourceWrapper struct {
	source  DataSource
	limiter *rateLimiter
	breaker *circuitBreaker
}

func (x *dataSourceWrapper) Fetch(ctx context.Context, key string) ([]byte, error) {
	if x.breaker != nil && !x.breaker.Allow() {
		return nil, ErrDataSourceCircuitOpen
	}

	if x.limiter != nil {
		if err := x.limiter.Wait(ctx); err != nil {
			if x.breaker != nil {
				x.breaker.Abort()
			}
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				return nil, err
			}
			return nil, ErrDataSourceRateLimited
		}
	}

	value, err := x.source.Fetch(ctx, key)
	if x.breaker != nil {
		if err != nil {
			x.breaker.OnFailure()
		} else {
			x.breaker.OnSuccess()
		}
	}

	return value, err
}
