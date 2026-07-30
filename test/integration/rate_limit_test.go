package integration_test

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/Djunichi/APIGateway/internal/config"
	"github.com/Djunichi/APIGateway/internal/ratelimit"
	"github.com/Djunichi/APIGateway/test/integration/internal/testenv"
)

func TestGlobalRateLimitReturnsTooManyRequests(t *testing.T) {
	cfg := config.RateLimit{
		Enabled: true, DefaultBackend: "test", OnBackendError: "deny",
		OperationTimeout: time.Second,
		Rules: []config.RateLimitRule{{
			Name: "global", Key: "global", Filter: "all",
			TokenBucket: config.TokenBucket{RequestsPerSecond: 1, Burst: 2, TTL: time.Minute},
		}},
	}
	env := testenv.New(t, testenv.WithUsers(1), testenv.WithRateLimit(cfg, newCountingStore()))

	env.GET("/api/users").RequireStatus(http.StatusOK)
	env.GET("/api/users").RequireStatus(http.StatusOK)
	response := env.GET("/api/users").RequireStatus(http.StatusTooManyRequests)
	if response.Header.Get("Retry-After") == "" || response.Header.Get("X-Request-ID") == "" {
		t.Fatalf("headers = %v", response.Header)
	}
}

type countingStore struct {
	mu     sync.Mutex
	counts map[string]int
}

func newCountingStore() *countingStore { return &countingStore{counts: make(map[string]int)} }

func (s *countingStore) Take(_ context.Context, request ratelimit.TakeRequest) (ratelimit.TakeResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counts[request.Key]++
	return ratelimit.TakeResult{
		Allowed: s.counts[request.Key] <= request.Burst, RetryAfter: time.Second,
	}, nil
}
