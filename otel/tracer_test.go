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
	"testing"

	"github.com/stretchr/testify/require"
	sdkotel "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestNewTracerConfigDefaults(t *testing.T) {
	original := sdkotel.GetTracerProvider()
	custom := noop.NewTracerProvider()
	sdkotel.SetTracerProvider(custom)
	t.Cleanup(func() {
		sdkotel.SetTracerProvider(original)
	})

	cfg := NewTracerConfig()

	require.Equal(t, custom, cfg.TracerProvider())
	require.Empty(t, cfg.Attributes())
}

func TestNewTracerConfigWithOptions(t *testing.T) {
	attrs := []attribute.KeyValue{
		attribute.String("service", "distcache"),
		attribute.Bool("debug", true),
	}
	provider := noop.NewTracerProvider()

	cfg := NewTracerConfig(
		WithTracerProvider(provider),
		WithAttributes(attrs[0]),
		WithAttributes(attrs[1]),
	)

	require.Equal(t, provider, cfg.TracerProvider())
	require.Equal(t, attrs, cfg.Attributes())
}

func TestNewTracerConfigWithNilProvider(t *testing.T) {
	cfg := NewTracerConfig(WithTracerProvider(nil))

	require.Nil(t, cfg.TracerProvider())
	require.Empty(t, cfg.Attributes())
}
