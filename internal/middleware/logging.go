package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/ivan-gromov-dev/APIGateway/internal/metrics"
	"go.opentelemetry.io/otel/trace"
)

// Logging records request details and updates the metrics collector.
func Logging(logger *slog.Logger, collector *metrics.Collector) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			collector.BeginRequest()
			start := time.Now()
			recorder := &responseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(recorder, r)
			duration := time.Since(start)
			collector.Observe(r.Method, recorder.status, recorder.bytes, duration)
			attributes := []any{
				"request completed",
				"method", r.Method,
				"path", r.URL.Path,
				"status", recorder.status,
				"bytes", recorder.bytes,
				"duration_ms", duration.Milliseconds(),
				"request_id", w.Header().Get(requestIDHeader),
			}
			spanContext := trace.SpanContextFromContext(r.Context())
			if spanContext.IsValid() {
				attributes = append(attributes, "trace_id", spanContext.TraceID().String(), "span_id", spanContext.SpanID().String())
			}
			logger.Info(attributes[0].(string), attributes[1:]...)
		})
	}
}

type responseWriter struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func (w *responseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Write(body []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(body)
	w.bytes += n
	return n, err
}

func (w *responseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
