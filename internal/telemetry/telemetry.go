// Package telemetry configures distributed tracing and HTTP trace propagation.
package telemetry

import (
	"context"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
	"go.opentelemetry.io/otel/trace"
)

const instrumentationName = "github.com/Djunichi/APIGateway"

// Config describes optional OTLP trace export.
type Config struct {
	Enabled     bool
	ServiceName string
	Endpoint    string
	Insecure    bool
	SampleRatio float64
}

// Runtime owns explicit tracing dependencies shared by gateway HTTP clients and handlers.
type Runtime struct {
	provider   trace.TracerProvider
	propagator propagation.TextMapPropagator
	shutdown   func(context.Context) error
}

// New creates a disabled runtime or an OTLP/HTTP-backed trace provider.
func New(ctx context.Context, cfg Config) (*Runtime, error) {
	if !cfg.Enabled {
		return Disabled(), nil
	}
	options := []otlptracehttp.Option{otlptracehttp.WithEndpoint(cfg.Endpoint)}
	if cfg.Insecure {
		options = append(options, otlptracehttp.WithInsecure())
	}
	exporter, err := otlptracehttp.New(ctx, options...)
	if err != nil {
		return nil, err
	}
	serviceResource, err := resource.Merge(resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceName(cfg.ServiceName)))
	if err != nil {
		return nil, err
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRatio))),
		sdktrace.WithResource(serviceResource),
	)
	return NewRuntime(provider, provider.Shutdown), nil
}

// Disabled returns a no-op runtime that preserves the same wiring contract.
func Disabled() *Runtime {
	return NewRuntime(trace.NewNoopTracerProvider(), func(context.Context) error { return nil })
}

// NewRuntime constructs a runtime around an externally owned provider, primarily for tests.
func NewRuntime(provider trace.TracerProvider, shutdown func(context.Context) error) *Runtime {
	if provider == nil {
		provider = trace.NewNoopTracerProvider()
	}
	if shutdown == nil {
		shutdown = func(context.Context) error { return nil }
	}
	return &Runtime{provider: provider, propagator: propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	), shutdown: shutdown}
}

// Tracer returns the gateway tracer used for internal spans.
func (r *Runtime) Tracer() trace.Tracer { return r.provider.Tracer(instrumentationName) }

// Handler instruments inbound requests and extracts W3C trace context.
func (r *Runtime) Handler(next http.Handler) http.Handler {
	return otelhttp.NewHandler(next, "gateway.request", otelhttp.WithTracerProvider(r.provider),
		otelhttp.WithPropagators(r.propagator))
}

// Transport instruments outbound requests and injects W3C trace context.
func (r *Runtime) Transport(base http.RoundTripper) http.RoundTripper {
	return otelhttp.NewTransport(base, otelhttp.WithTracerProvider(r.provider),
		otelhttp.WithPropagators(r.propagator))
}

// HTTPClient returns an instrumented client with a bounded request timeout.
func (r *Runtime) HTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Transport: r.Transport(http.DefaultTransport), Timeout: timeout}
}

// Shutdown flushes and closes owned exporter resources.
func (r *Runtime) Shutdown(ctx context.Context) error { return r.shutdown(ctx) }
