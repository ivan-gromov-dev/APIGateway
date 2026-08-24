package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ivan-gromov-dev/APIGateway/internal/cache"
	"github.com/ivan-gromov-dev/APIGateway/internal/config"
)

type memoryCache struct {
	entries map[string]cache.Entry
	getErr  error
	sets    int
}

func (m *memoryCache) Get(_ context.Context, key string) (cache.Entry, bool, error) {
	e, ok := m.entries[key]
	return e, ok, m.getErr
}
func (m *memoryCache) Set(_ context.Context, key string, e cache.Entry, _ time.Duration) error {
	if m.entries == nil {
		m.entries = map[string]cache.Entry{}
	}
	m.entries[key] = e
	m.sets++
	return nil
}

func TestResponseCacheMissThenHit(t *testing.T) {
	store := &memoryCache{}
	calls := 0
	handler := ResponseCache(store, config.Cache{OperationTimeout: time.Second, MaxBodyBytes: 100, OnBackendError: "allow"}, config.RouteCache{TTL: time.Minute, VaryHeaders: []string{"Accept"}}, "/api/")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("body"))
	}))
	for i, expected := range []string{"MISS", "HIT"} {
		request := httptest.NewRequest(http.MethodGet, "/api/items?b=2&a=1", nil)
		request.Header.Set("Accept", "text/plain")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != 200 || response.Body.String() != "body" || response.Header().Get("X-Cache") != expected {
			t.Fatalf("request %d = %d %q %q", i, response.Code, response.Body.String(), response.Header().Get("X-Cache"))
		}
	}
	if calls != 1 {
		t.Fatalf("calls = %d", calls)
	}
}

func TestResponseCacheBypassesUnsafeAndPrivateResponses(t *testing.T) {
	for _, mutate := range []func(*http.Request){func(r *http.Request) { r.Method = http.MethodPost }, func(r *http.Request) { r.Header.Set("Authorization", "Bearer x") }} {
		store := &memoryCache{}
		handler := ResponseCache(store, config.Cache{OperationTimeout: time.Second, MaxBodyBytes: 100, OnBackendError: "allow"}, config.RouteCache{TTL: time.Minute}, "/")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Cache-Control", "private")
			_, _ = w.Write([]byte("x"))
		}))
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		mutate(request)
		handler.ServeHTTP(httptest.NewRecorder(), request)
		if store.sets != 0 {
			t.Fatal("response cached")
		}
	}
}

func TestResponseCacheBackendPolicy(t *testing.T) {
	for _, test := range []struct {
		policy string
		status int
	}{{"allow", 204}, {"deny", 503}} {
		store := &memoryCache{getErr: errors.New("down")}
		handler := ResponseCache(store, config.Cache{OperationTimeout: time.Second, MaxBodyBytes: 100, OnBackendError: test.policy}, config.RouteCache{TTL: time.Minute}, "/")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
		if response.Code != test.status {
			t.Fatalf("%s status = %d", test.policy, response.Code)
		}
	}
}
