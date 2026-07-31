package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestRuntimeCreatesServerAndClientSpansAndPropagatesContext(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	runtime := NewRuntime(provider, provider.Shutdown)

	var traceparent string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceparent = r.Header.Get("traceparent")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	handler := runtime.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, upstream.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := runtime.HTTPClient(time.Second).Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		w.WriteHeader(http.StatusNoContent)
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "http://gateway.test/api", nil))

	if traceparent == "" {
		t.Fatal("outgoing request has no traceparent")
	}
	if got := len(exporter.GetSpans()); got != 2 {
		t.Fatalf("span count = %d, want 2", got)
	}
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestDisabledRuntimeIsUsable(t *testing.T) {
	runtime, err := New(context.Background(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	runtime.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusAccepted) })).
		ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d", recorder.Code)
	}
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestNewBuildsEnabledRuntime(t *testing.T) {
	runtime, err := New(context.Background(), Config{Enabled: true, ServiceName: "gateway", Endpoint: "localhost:1", Insecure: true, SampleRatio: 1})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = runtime.Shutdown(ctx)
}
