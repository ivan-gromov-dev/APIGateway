package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/Djunichi/APIGateway/internal/metrics"
)

// Logging records request details and updates the metrics collector.
func Logging(logger *slog.Logger, collector *metrics.Collector) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			recorder := &responseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(recorder, r)
			duration := time.Since(start)
			collector.Observe(recorder.status, duration)
			logger.Info(
				"request completed",
				"method", r.Method,
				"path", r.URL.Path,
				"status", recorder.status,
				"bytes", recorder.bytes,
				"duration_ms", duration.Milliseconds(),
				"request_id", w.Header().Get(requestIDHeader),
			)
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
