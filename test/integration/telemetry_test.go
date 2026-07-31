package integration_test

import (
	"net/http"
	"testing"

	"github.com/Djunichi/APIGateway/internal/telemetry"
	"github.com/Djunichi/APIGateway/test/integration/internal/testenv"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestTracingContinuesIncomingTraceAndExportsGatewaySpans(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	runtime := telemetry.NewRuntime(provider, provider.Shutdown)
	t.Cleanup(func() { _ = provider.Shutdown(t.Context()) })
	env := testenv.New(t, testenv.WithUsers(1), testenv.WithTelemetry(runtime))

	const traceID = "0af7651916cd43dd8448eb211c80319c"
	headers := http.Header{"Traceparent": {"00-" + traceID + "-b7ad6b7169203331-01"}}
	env.Do(http.MethodGet, "/api/users/42", headers).
		RequireStatus(http.StatusOK).
		RequireTraceID(traceID)

	if got := len(exporter.GetSpans()); got < 3 {
		t.Fatalf("exported spans = %d, want at least server, attempt, and client spans", got)
	}
}
