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
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// TracerConfig holds tracing-related settings used when creating or acquiring
// a tracer. It is typically constructed via NewTracerConfig and one or more
// TracerOption values.
//
// Defaults:
//   - provider: otel.GetTracerProvider()
//   - attributes: none
//
// TracerConfig is intended to be treated as immutable after construction.
type TracerConfig struct {
	// provider is the TracerProvider to use when obtaining a tracer. If not set
	// via options, NewTracerConfig defaults to otel.GetTracerProvider().
	provider trace.TracerProvider

	// attributes are static attributes that callers may attach to spans or
	// resources when using this configuration. How these are applied is up to
	// the consumer of TracerConfig.
	attributes []attribute.KeyValue
}

// TracerOption is a functional option that mutates a TracerConfig.
//
// Example:
//
//	cfg := NewTracerConfig(
//	    WithTracerProvider(sdkTracerProvider),
//	    WithAttributes(attribute.String("env", "prod")),
//	)
type TracerOption func(*TracerConfig)

// WithTracerProvider returns a TracerOption that sets the tracer provider in
// the configuration. If you do not provide this option, NewTracerConfig uses
// the global provider returned by otel.GetTracerProvider().
//
// Note: Passing a nil provider will overwrite the default with nil.
func WithTracerProvider(provider trace.TracerProvider) TracerOption {
	return func(cfg *TracerConfig) {
		cfg.provider = provider
	}
}

// WithAttributes returns a TracerOption that appends the given attributes to
// the configuration. These attributes can be attached by consumers to spans
// or resources created with the configured tracer.
func WithAttributes(attrs ...attribute.KeyValue) TracerOption {
	return func(cfg *TracerConfig) {
		cfg.attributes = append(cfg.attributes, attrs...)
	}
}

// NewTracerConfig constructs a TracerConfig by applying the provided options
// on top of sane defaults.
//
// Defaults:
//   - provider: otel.GetTracerProvider()
//
// Options are applied in the order they are provided; later options may
// overwrite earlier ones.
func NewTracerConfig(opts ...TracerOption) *TracerConfig {
	cfg := &TracerConfig{
		provider: otel.GetTracerProvider(),
	}

	for _, opt := range opts {
		opt(cfg)
	}

	return cfg
}

// TracerProvider returns the configured trace.TracerProvider.
//
// If WithTracerProvider was not supplied to NewTracerConfig, this will be the
// global provider obtained from otel.GetTracerProvider() at construction time.
func (c *TracerConfig) TracerProvider() trace.TracerProvider {
	return c.provider
}

// Attributes returns the static attributes configured for this tracer config.
func (c *TracerConfig) Attributes() []attribute.KeyValue {
	return c.attributes
}
