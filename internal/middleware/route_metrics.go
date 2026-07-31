package middleware

import (
	"net/http"

	"github.com/Djunichi/APIGateway/internal/metrics"
)

// RouteMetrics records status counts using the configured route prefix.
func RouteMetrics(collector *metrics.Collector, route string) Middleware {
	if collector == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			recorder := &responseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(recorder, r)
			collector.ObserveRoute(route, recorder.status)
		})
	}
}
