package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCollectorObserveAndServeHTTP(t *testing.T) {
	collector := &Collector{}
	collector.Observe(http.StatusOK, 50*time.Millisecond)
	collector.Observe(http.StatusBadGateway, 100*time.Millisecond)

	response := httptest.NewRecorder()
	collector.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if contentType := response.Header().Get("Content-Type"); contentType != "text/plain; version=0.0.4" {
		t.Fatalf("Content-Type = %q", contentType)
	}
	body := response.Body.String()
	for _, metric := range []string{
		"gateway_http_requests_total 2",
		"gateway_http_errors_total 1",
		"gateway_http_request_duration_seconds_sum 0.150000",
	} {
		if !strings.Contains(body, metric) {
			t.Errorf("metrics output does not contain %q:\n%s", metric, body)
		}
	}
}

func TestCollectorIsSafeForConcurrentUse(t *testing.T) {
	collector := &Collector{}
	const observations = 100
	var waitGroup sync.WaitGroup
	waitGroup.Add(observations)

	for range observations {
		go func() {
			defer waitGroup.Done()
			collector.Observe(http.StatusNoContent, time.Millisecond)
		}()
	}
	waitGroup.Wait()

	response := httptest.NewRecorder()
	collector.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !strings.Contains(response.Body.String(), "gateway_http_requests_total 100") {
		t.Fatalf("unexpected metrics output:\n%s", response.Body.String())
	}
}
