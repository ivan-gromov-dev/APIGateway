package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Djunichi/APIGateway/internal/config"
	"github.com/Djunichi/APIGateway/internal/metrics"
)

func TestChainAppliesMiddlewareInDeclarationOrder(t *testing.T) {
	var order []string
	named := func(name string) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name+":before")
				next.ServeHTTP(w, r)
				order = append(order, name+":after")
			})
		}
	}
	handler := Chain(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			order = append(order, "handler")
		}),
		named("first"),
		named("second"),
	)

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	want := "first:before,second:before,handler,second:after,first:after"
	if got := strings.Join(order, ","); got != want {
		t.Fatalf("order = %q, want %q", got, want)
	}
}

func TestRequestIDPropagatesExistingID(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(requestIDHeader, "existing-id")
	response := httptest.NewRecorder()
	var received string
	handler := RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		received = r.Header.Get(requestIDHeader)
	}))

	handler.ServeHTTP(response, request)

	if received != "existing-id" || response.Header().Get(requestIDHeader) != "existing-id" {
		t.Fatalf("request ID was not propagated: request=%q response=%q", received, response.Header().Get(requestIDHeader))
	}
}

func TestRequestIDGeneratesID(t *testing.T) {
	response := httptest.NewRecorder()
	handler := RequestID(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))

	if id := response.Header().Get(requestIDHeader); !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(id) {
		t.Fatalf("generated request ID = %q", id)
	}
}

func TestRecoveryConvertsPanicToInternalServerError(t *testing.T) {
	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, nil))
	handler := Recovery(log)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(logs.String(), "panic recovered") || !strings.Contains(logs.String(), "boom") {
		t.Fatalf("panic was not logged: %q", logs.String())
	}
}

func TestLoggingRecordsResponseAndMetrics(t *testing.T) {
	var logs bytes.Buffer
	collector := &metrics.Collector{}
	handler := Logging(slog.New(slog.NewJSONHandler(&logs, nil)), collector)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
			w.WriteHeader(http.StatusNoContent)
			_, _ = w.Write([]byte("created"))
		}),
	)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/items", nil))

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusCreated)
	}
	logOutput := logs.String()
	if !strings.Contains(logOutput, `"status":201`) || !strings.Contains(logOutput, `"bytes":7`) {
		t.Fatalf("unexpected request log: %q", logOutput)
	}
	metricsResponse := httptest.NewRecorder()
	collector.ServeHTTP(metricsResponse, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !strings.Contains(metricsResponse.Body.String(), "gateway_http_requests_total 1") {
		t.Fatalf("request metric was not recorded: %q", metricsResponse.Body.String())
	}
}

func TestTimeoutReturnsServiceUnavailable(t *testing.T) {
	handler := Timeout(time.Millisecond)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	if !strings.Contains(response.Body.String(), "gateway timeout") {
		t.Fatalf("unexpected timeout body: %q", response.Body.String())
	}
}

func TestTimeoutDisabled(t *testing.T) {
	handler := Timeout(0)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusAccepted)
	}
}

func TestCORSPreflight(t *testing.T) {
	cfg := config.CORS{
		AllowedOrigins: []string{"https://example.com"},
		AllowedMethods: []string{"GET", "POST"},
		AllowedHeaders: []string{"Authorization", "Content-Type"},
	}
	called := false
	handler := CORS(cfg)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	request := httptest.NewRequest(http.MethodOptions, "/", nil)
	request.Header.Set("Origin", "https://example.com")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if called {
		t.Fatal("preflight request reached the next handler")
	}
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
		t.Fatalf("allowed origin = %q", got)
	}
	if got := response.Header().Get("Access-Control-Allow-Methods"); got != "GET, POST" {
		t.Fatalf("allowed methods = %q", got)
	}
}

func TestCORSDoesNotAllowUnknownOrigin(t *testing.T) {
	cfg := config.CORS{AllowedOrigins: []string{"https://example.com"}}
	handler := CORS(cfg)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Origin", "https://untrusted.example")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if origin := response.Header().Get("Access-Control-Allow-Origin"); origin != "" {
		t.Fatalf("unexpected allowed origin %q", origin)
	}
}

func TestResponseWriterUnwrap(t *testing.T) {
	underlying := httptest.NewRecorder()
	writer := &responseWriter{ResponseWriter: underlying}
	if writer.Unwrap() != underlying {
		t.Fatal("Unwrap did not return the underlying response writer")
	}
}
