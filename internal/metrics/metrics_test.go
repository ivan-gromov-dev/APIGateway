package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCollectorExposesGatewayMetrics(t *testing.T) {
	collector := &Collector{}
	collector.BeginRequest()
	collector.Observe(http.MethodGet, http.StatusBadGateway, 512, 100*time.Millisecond)
	collector.ObserveRoute("/api/", http.StatusBadGateway)
	collector.ObserveProxyAttempt("/api/", "backend-1:8081", "failure", 50*time.Millisecond)
	collector.ObserveRetry("/api/", "status")
	collector.ObserveFeature("cache", "/api/", "", "hit")

	response := httptest.NewRecorder()
	collector.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := response.Body.String()
	for _, metric := range []string{
		`gateway_http_requests_total{method="GET",status_code="502"} 1`,
		`gateway_route_requests_total{route="/api/",status_code="502"} 1`,
		`gateway_proxy_attempts_total{outcome="failure",route="/api/",upstream="backend-1:8081"} 1`,
		`gateway_proxy_retries_total{reason="status",route="/api/"} 1`,
		`gateway_feature_decisions_total{feature="cache",name="",result="hit",scope="/api/"} 1`,
		`gateway_http_requests_in_flight 0`,
		`go_goroutines`, `process_cpu_seconds_total`,
	} {
		if !strings.Contains(body, metric) {
			t.Errorf("output does not contain %q", metric)
		}
	}
}

func TestCollectorIsSafeForConcurrentUse(t *testing.T) {
	collector := &Collector{}
	const observations = 100
	var group sync.WaitGroup
	group.Add(observations)
	for range observations {
		go func() {
			defer group.Done()
			collector.BeginRequest()
			collector.Observe("GET", 204, 0, time.Millisecond)
		}()
	}
	group.Wait()
	response := httptest.NewRecorder()
	collector.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !strings.Contains(response.Body.String(), `gateway_http_requests_total{method="GET",status_code="204"} 100`) {
		t.Fatal("concurrent observations were lost")
	}
}
