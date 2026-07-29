// Package metrics collects and exposes Prometheus-compatible gateway metrics.
package metrics

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

type Collector struct {
	requests   atomic.Uint64
	errors     atomic.Uint64
	durationNS atomic.Uint64
}

func (c *Collector) Observe(status int, duration time.Duration) {
	c.requests.Add(1)
	c.durationNS.Add(uint64(duration))
	if status >= http.StatusInternalServerError {
		c.errors.Add(1)
	}
}

func (c *Collector) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "# HELP gateway_http_requests_total Total HTTP requests.\n")
	fmt.Fprintf(w, "# TYPE gateway_http_requests_total counter\n")
	fmt.Fprintf(w, "gateway_http_requests_total %d\n", c.requests.Load())
	fmt.Fprintf(w, "# HELP gateway_http_errors_total Total HTTP 5xx responses.\n")
	fmt.Fprintf(w, "# TYPE gateway_http_errors_total counter\n")
	fmt.Fprintf(w, "gateway_http_errors_total %d\n", c.errors.Load())
	fmt.Fprintf(w, "# HELP gateway_http_request_duration_seconds_sum Cumulative request duration.\n")
	fmt.Fprintf(w, "# TYPE gateway_http_request_duration_seconds_sum counter\n")
	fmt.Fprintf(w, "gateway_http_request_duration_seconds_sum %f\n", time.Duration(c.durationNS.Load()).Seconds())
}
