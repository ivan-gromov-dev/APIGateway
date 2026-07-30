// Package ratelimit defines internal contracts for gateway rate limiting.
package ratelimit

import (
	"context"
	"net/http"
	"time"
)

// TakeRequest describes one atomic Token Bucket admission attempt.
type TakeRequest struct {
	Key   string
	Rate  float64
	Burst int
	Cost  int
	TTL   time.Duration
}

// TakeResult is the backend's admission decision.
type TakeResult struct {
	Allowed    bool
	Remaining  float64
	RetryAfter time.Duration
}

// Store atomically updates distributed Token Bucket state.
type Store interface {
	Take(context.Context, TakeRequest) (TakeResult, error)
}

// Limiter evaluates one configured rate-limit rule.
type Limiter interface {
	Allow(context.Context, string) (TakeResult, error)
}

// KeyFunc extracts the identity constrained by a rule.
type KeyFunc func(*http.Request) (string, error)

// FilterFunc reports whether a rule applies to a request.
type FilterFunc func(*http.Request) bool
