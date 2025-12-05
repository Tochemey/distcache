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

package otel

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// MetricConfig holds configuration used to create and access OpenTelemetry meters.
// It encapsulates the MeterProvider to use and the instrumentation name associated
// with the meter.
type MetricConfig struct {
	provider metric.MeterProvider
}

// MetricOption configures a MetricConfig.
// Options are applied in order by NewMetricConfig.
type MetricOption func(*MetricConfig)

// WithMeterProvider sets the MeterProvider to be used when obtaining a meter.
//
// If not provided, NewMetricConfig defaults to otel.GetMeterProvider().
func WithMeterProvider(provider metric.MeterProvider) MetricOption {
	return func(cfg *MetricConfig) {
		cfg.provider = provider
	}
}

// NewMetricConfig builds a MetricConfig using the supplied options.
//
// Defaults:
//   - provider: otel.GetMeterProvider()
//
// Example:
//
//	cfg := NewMetricConfig(WithMeterProvider(customProvider))
func NewMetricConfig(opts ...MetricOption) *MetricConfig {
	cfg := &MetricConfig{
		provider: otel.GetMeterProvider(),
	}

	for _, opt := range opts {
		opt(cfg)
	}

	return cfg
}

// Provider returns the configured MeterProvider.
// Use this provider to obtain meters for creating instruments.
func (c MetricConfig) Provider() metric.MeterProvider {
	return c.provider
}
