// Package metrics collects and exposes Prometheus gateway metrics.
package metrics

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Collector owns a private Prometheus registry and bounded-cardinality metrics.
type Collector struct {
	once             sync.Once
	registry         *prometheus.Registry
	handler          http.Handler
	httpRequests     *prometheus.CounterVec
	httpDuration     *prometheus.HistogramVec
	httpSize         *prometheus.HistogramVec
	inFlight         prometheus.Gauge
	routeRequests    *prometheus.CounterVec
	proxyAttempts    *prometheus.CounterVec
	proxyDuration    *prometheus.HistogramVec
	retries          *prometheus.CounterVec
	featureDecisions *prometheus.CounterVec
}

func (c *Collector) init() {
	c.once.Do(func() {
		c.registry = prometheus.NewRegistry()
		c.httpRequests = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "gateway_http_requests_total", Help: "Completed public HTTP requests."}, []string{"method", "status_code"})
		c.httpDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "gateway_http_request_duration_seconds", Help: "Public HTTP request duration.", Buckets: prometheus.DefBuckets}, []string{"method"})
		c.httpSize = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "gateway_http_response_size_bytes", Help: "Public HTTP response size.", Buckets: prometheus.ExponentialBuckets(128, 4, 8)}, []string{"method"})
		c.inFlight = prometheus.NewGauge(prometheus.GaugeOpts{Name: "gateway_http_requests_in_flight", Help: "Currently executing public HTTP requests."})
		c.routeRequests = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "gateway_route_requests_total", Help: "Completed requests by configured route."}, []string{"route", "status_code"})
		c.proxyAttempts = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "gateway_proxy_attempts_total", Help: "Proxy attempts by configured route, upstream, and outcome."}, []string{"route", "upstream", "outcome"})
		c.proxyDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "gateway_proxy_attempt_duration_seconds", Help: "Proxy attempt duration.", Buckets: prometheus.DefBuckets}, []string{"route", "upstream"})
		c.retries = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "gateway_proxy_retries_total", Help: "Additional proxy attempts by configured route and reason."}, []string{"route", "reason"})
		c.featureDecisions = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "gateway_feature_decisions_total", Help: "Authentication, cache, and rate-limit decisions."}, []string{"feature", "scope", "name", "result"})
		c.registry.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
			c.httpRequests, c.httpDuration, c.httpSize, c.inFlight, c.routeRequests,
			c.proxyAttempts, c.proxyDuration, c.retries, c.featureDecisions)
		c.handler = promhttp.HandlerFor(c.registry, promhttp.HandlerOpts{})
	})
}

// BeginRequest increments the current public-request gauge.
func (c *Collector) BeginRequest() { c.init(); c.inFlight.Inc() }

// Observe records a completed public request.
func (c *Collector) Observe(method string, status, bytes int, duration time.Duration) {
	c.init()
	statusCode := strconv.Itoa(status)
	c.httpRequests.WithLabelValues(method, statusCode).Inc()
	c.httpDuration.WithLabelValues(method).Observe(duration.Seconds())
	c.httpSize.WithLabelValues(method).Observe(float64(bytes))
	c.inFlight.Dec()
}

// ObserveRoute records a completed request against a configured route prefix.
func (c *Collector) ObserveRoute(route string, status int) {
	c.init()
	c.routeRequests.WithLabelValues(route, strconv.Itoa(status)).Inc()
}

// ObserveProxyAttempt records an upstream attempt with a bounded outcome.
func (c *Collector) ObserveProxyAttempt(route, upstream, outcome string, duration time.Duration) {
	c.init()
	c.proxyAttempts.WithLabelValues(route, upstream, outcome).Inc()
	c.proxyDuration.WithLabelValues(route, upstream).Observe(duration.Seconds())
}

// ObserveRetry records an additional proxy attempt and its trigger.
func (c *Collector) ObserveRetry(route, reason string) {
	c.init()
	c.retries.WithLabelValues(route, reason).Inc()
}

// ObserveFeature records a bounded decision from an optional gateway feature.
func (c *Collector) ObserveFeature(feature, scope, name, result string) {
	c.init()
	c.featureDecisions.WithLabelValues(feature, scope, name, result).Inc()
}

// ServeHTTP exposes the private registry in Prometheus format.
func (c *Collector) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.init()
	c.handler.ServeHTTP(w, r)
}
