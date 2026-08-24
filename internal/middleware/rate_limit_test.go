package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ivan-gromov-dev/APIGateway/internal/config"
	"github.com/ivan-gromov-dev/APIGateway/internal/ratelimit"
)

func TestRateLimitAllowsAndRejectsRequests(t *testing.T) {
	store := &decisionStore{results: []ratelimit.TakeResult{
		{Allowed: true}, {Allowed: false, RetryAfter: 100 * time.Millisecond},
	}}
	registry := testRegistry(t, store)
	mw, err := BuildRateLimit([]config.RateLimitRule{testRule()}, testRateConfig(), registry)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	handler := mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/", nil))
	second := httptest.NewRecorder()
	handler.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/", nil))
	if calls != 1 || second.Code != http.StatusTooManyRequests || second.Header().Get("Retry-After") != "1" {
		t.Fatalf("calls=%d response=%d headers=%v", calls, second.Code, second.Header())
	}
}

func TestRateLimitBackendErrorPolicies(t *testing.T) {
	for _, policy := range []string{"allow", "deny"} {
		t.Run(policy, func(t *testing.T) {
			registry := testRegistry(t, &decisionStore{err: errors.New("down")})
			cfg := testRateConfig()
			cfg.OnBackendError = policy
			mw, err := BuildRateLimit([]config.RateLimitRule{testRule()}, cfg, registry)
			if err != nil {
				t.Fatal(err)
			}
			called := false
			response := httptest.NewRecorder()
			mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })).
				ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
			if policy == "allow" && !called {
				t.Fatal("fail-open request rejected")
			}
			if policy == "deny" && response.Code != http.StatusServiceUnavailable {
				t.Fatalf("status=%d", response.Code)
			}
		})
	}
}

func TestRateLimitResolversAndBuiltins(t *testing.T) {
	registry := ratelimit.NewRegistry()
	cfg := testRateConfig()
	if _, err := BuildRateLimit([]config.RateLimitRule{testRule()}, cfg, registry); err == nil {
		t.Fatal("missing store accepted")
	}
	_ = registry.RegisterStore("redis", &decisionStore{})
	if _, err := BuildRateLimit([]config.RateLimitRule{testRule()}, cfg, registry); err == nil {
		t.Fatal("missing key accepted")
	}
	_ = registry.RegisterKey("client_ip", ClientIPKey)
	if _, err := BuildRateLimit([]config.RateLimitRule{testRule()}, cfg, registry); err == nil {
		t.Fatal("missing filter accepted")
	}
	if !WritesOnly(httptest.NewRequest(http.MethodPost, "/", nil)) || WritesOnly(httptest.NewRequest(http.MethodGet, "/", nil)) {
		t.Fatal("writes filter mismatch")
	}
	if key, _ := GlobalKey(nil); key != "global" {
		t.Fatalf("global key=%q", key)
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "192.0.2.1:1234"
	if key, _ := ClientIPKey(request); key != "192.0.2.1" {
		t.Fatalf("client key=%q", key)
	}
}

func testRegistry(t *testing.T, store ratelimit.Store) *ratelimit.Registry {
	t.Helper()
	registry := ratelimit.NewRegistry()
	if err := registry.RegisterStore("redis", store); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterKey("client_ip", ClientIPKey); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterFilter("all", AllRequests); err != nil {
		t.Fatal(err)
	}
	return registry
}

func testRule() config.RateLimitRule {
	return config.RateLimitRule{Name: "test", Key: "client_ip", Filter: "all",
		TokenBucket: config.TokenBucket{RequestsPerSecond: 1, Burst: 1, TTL: time.Minute}}
}
func testRateConfig() config.RateLimit {
	return config.RateLimit{DefaultBackend: "redis", OnBackendError: "deny", OperationTimeout: time.Second}
}

type decisionStore struct {
	results []ratelimit.TakeResult
	err     error
}

func (s *decisionStore) Take(context.Context, ratelimit.TakeRequest) (ratelimit.TakeResult, error) {
	if s.err != nil {
		return ratelimit.TakeResult{}, s.err
	}
	result := s.results[0]
	s.results = s.results[1:]
	return result, nil
}
