// Package limiter implements gateway rate limit policies.
package limiter

import (
	"context"
	"time"

	"github.com/Djunichi/APIGateway/internal/ratelimit"
)

type TokenBucket struct {
	store ratelimit.Store
	rate  float64
	burst int
	ttl   time.Duration
}

func NewTokenBucket(store ratelimit.Store, rate float64, burst int, ttl time.Duration) *TokenBucket {
	return &TokenBucket{store: store, rate: rate, burst: burst, ttl: ttl}
}

func (l *TokenBucket) Allow(ctx context.Context, key string) (ratelimit.TakeResult, error) {
	return l.store.Take(ctx, ratelimit.TakeRequest{
		Key: key, Rate: l.rate, Burst: l.burst, Cost: 1, TTL: l.ttl,
	})
}
