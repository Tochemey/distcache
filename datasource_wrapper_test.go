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
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type countingDataSource struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (c *countingDataSource) Fetch(_ context.Context, _ string) ([]byte, error) {
	c.mu.Lock()
	c.calls++
	err := c.err
	c.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return []byte("ok"), nil
}

func (c *countingDataSource) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func (c *countingDataSource) setError(err error) {
	c.mu.Lock()
	c.err = err
	c.mu.Unlock()
}

func TestDataSourceProtectionRateLimit(t *testing.T) {
	source := &countingDataSource{}
	policy := &dataSourceConfig{
		RateLimit: &RateLimitConfig{
			RequestsPerSecond: 1,
			Burst:             1,
			WaitTimeout:       0,
		},
	}

	protected := policy.Apply(source)
	_, err := protected.Fetch(context.Background(), "a")
	require.NoError(t, err)

	_, err = protected.Fetch(context.Background(), "b")
	require.ErrorIs(t, err, ErrDataSourceRateLimited)
	require.Equal(t, 1, source.callCount())
}

func TestDataSourceProtectionCircuitBreaker(t *testing.T) {
	source := &countingDataSource{err: errors.New("fail")}
	policy := &dataSourceConfig{
		CircuitBreaker: &CircuitBreakerConfig{
			FailureThreshold: 1,
			ResetTimeout:     50 * time.Millisecond,
		},
	}

	protected := policy.Apply(source)
	_, err := protected.Fetch(context.Background(), "a")
	require.Error(t, err)
	require.Equal(t, 1, source.callCount())

	_, err = protected.Fetch(context.Background(), "b")
	require.ErrorIs(t, err, ErrDataSourceCircuitOpen)
	require.Equal(t, 1, source.callCount())

	time.Sleep(60 * time.Millisecond)
	source.setError(nil)
	_, err = protected.Fetch(context.Background(), "c")
	require.NoError(t, err)
	require.Equal(t, 2, source.callCount())
}

func TestDataSourceProtectionValidate(t *testing.T) {
	policy := &dataSourceConfig{
		RateLimit: &RateLimitConfig{
			RequestsPerSecond: 0,
			Burst:             -1,
			WaitTimeout:       -1 * time.Second,
		},
		CircuitBreaker: &CircuitBreakerConfig{
			FailureThreshold: 0,
			ResetTimeout:     0,
		},
	}

	err := policy.Validate()
	require.Error(t, err)
}

func TestDataSourceProtectionWaitTimeoutReturnsContextError(t *testing.T) {
	source := &countingDataSource{}
	policy := &dataSourceConfig{
		RateLimit: &RateLimitConfig{
			RequestsPerSecond: 1,
			Burst:             1,
			WaitTimeout:       5 * time.Millisecond,
		},
	}

	protected := policy.Apply(source)
	_, err := protected.Fetch(context.Background(), "a")
	require.NoError(t, err)

	_, err = protected.Fetch(context.Background(), "b")
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestCircuitBreakerAbortAllowsRetry(t *testing.T) {
	breaker := newCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 1,
		ResetTimeout:     10 * time.Millisecond,
	})
	require.NotNil(t, breaker)

	breaker.OnFailure()
	time.Sleep(12 * time.Millisecond)

	require.True(t, breaker.Allow())
	breaker.Abort()
	require.True(t, breaker.Allow())
}

func TestRateLimitConfigString(t *testing.T) {
	cfg := RateLimitConfig{RequestsPerSecond: 2.5, Burst: 3}
	require.Equal(t, "rateLimit[rps=2.50, burst=3]", cfg.String())
}

func TestDataSourceProtectionNoop(t *testing.T) {
	source := &countingDataSource{}
	policy := &dataSourceConfig{}

	protected := policy.Apply(source)
	require.Equal(t, source, protected)
}

func TestDataSourceProtectionNilSource(t *testing.T) {
	policy := &dataSourceConfig{
		RateLimit: &RateLimitConfig{RequestsPerSecond: 1, Burst: 1},
	}
	require.Nil(t, policy.Apply(nil))
}
