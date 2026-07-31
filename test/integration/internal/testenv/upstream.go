package testenv

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// RecordedRequest is a request observed by a test upstream.
type RecordedRequest struct {
	Method      string
	Path        string
	RequestID   string
	Traceparent string
}

type upstream struct {
	service  string
	server   *httptest.Server
	mu       sync.Mutex
	requests []RecordedRequest
	statuses []int
}

func newUpstream(t testing.TB, service string) *upstream {
	t.Helper()
	instance := &upstream{service: service}
	instance.server = httptest.NewServer(http.HandlerFunc(instance.serveHTTP))
	t.Cleanup(instance.server.Close)
	return instance
}

func newUpstreamWithStatuses(t testing.TB, service string, statuses ...int) *upstream {
	t.Helper()
	instance := newUpstream(t, service)
	instance.statuses = append([]int(nil), statuses...)
	return instance
}

func (u *upstream) URL() string {
	return u.server.URL
}

func (u *upstream) serveHTTP(w http.ResponseWriter, r *http.Request) {
	request := RecordedRequest{
		Method:      r.Method,
		Path:        r.URL.Path,
		RequestID:   r.Header.Get("X-Request-ID"),
		Traceparent: r.Header.Get("traceparent"),
	}
	u.mu.Lock()
	u.requests = append(u.requests, request)
	status := http.StatusOK
	if len(u.statuses) > 0 {
		status = u.statuses[0]
		u.statuses = u.statuses[1:]
	}
	u.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(upstreamPayload{
		Service:     u.service,
		Path:        request.Path,
		RequestID:   request.RequestID,
		Traceparent: request.Traceparent,
	})
}
