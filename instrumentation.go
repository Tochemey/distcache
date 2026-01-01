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
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const instrumentationName = "github.com/tochemey/distcache/engine"

type instrumentation struct {
	tracer     trace.Tracer
	traceAttrs []attribute.KeyValue

	requests metric.Int64Counter
	errors   metric.Int64Counter
	duration metric.Float64Histogram
}

func newInstrumentation(cfg *Config) *instrumentation {
	inst := &instrumentation{}

	if cfg.TraceConfig() != nil && cfg.TraceConfig().TracerProvider() != nil {
		inst.tracer = cfg.TraceConfig().TracerProvider().Tracer(instrumentationName)
		inst.traceAttrs = append(inst.traceAttrs, cfg.TraceConfig().Attributes()...)
	}

	if cfg.MetricConfig() == nil {
		return inst
	}

	meter := cfg.MetricConfig().Provider().Meter(instrumentationName)
	inst.requests, _ = meter.Int64Counter(
		"distcache.engine.requests",
		metric.WithDescription("Total number of engine operations"),
	)
	inst.errors, _ = meter.Int64Counter(
		"distcache.engine.errors",
		metric.WithDescription("Total number of failed engine operations"),
	)
	inst.duration, _ = meter.Float64Histogram(
		"distcache.engine.duration.ms",
		metric.WithDescription("Engine operation latency in milliseconds"),
	)

	return inst
}

func (i *instrumentation) start(ctx context.Context, op string, keyspace string) (context.Context, func(error)) {
	if i == nil {
		return ctx, func(error) {}
	}

	start := time.Now()
	ctx, span := i.startSpan(ctx, op, keyspace)
	return ctx, func(err error) {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
		i.recordMetrics(ctx, op, keyspace, start, err)
	}
}

func (i *instrumentation) startSpan(ctx context.Context, op string, keyspace string) (context.Context, trace.Span) {
	if i.tracer == nil {
		return ctx, trace.SpanFromContext(ctx)
	}

	attrs := []attribute.KeyValue{
		attribute.String("distcache.operation", op),
	}
	if keyspace != "" {
		attrs = append(attrs, attribute.String("distcache.keyspace", keyspace))
	}
	attrs = append(attrs, i.traceAttrs...)

	return i.tracer.Start(ctx, "distcache."+op, trace.WithAttributes(attrs...))
}

func (i *instrumentation) recordMetrics(ctx context.Context, op string, keyspace string, start time.Time, err error) {
	if i == nil {
		return
	}

	attrs := []attribute.KeyValue{
		attribute.String("distcache.operation", op),
	}
	if keyspace != "" {
		attrs = append(attrs, attribute.String("distcache.keyspace", keyspace))
	}

	if i.requests != nil {
		i.requests.Add(ctx, 1, metric.WithAttributes(attrs...))
	}
	if err != nil && i.errors != nil {
		i.errors.Add(ctx, 1, metric.WithAttributes(attrs...))
	}
	if i.duration != nil {
		i.duration.Record(ctx, float64(time.Since(start).Milliseconds()), metric.WithAttributes(attrs...))
	}
}
