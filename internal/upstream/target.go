// Package upstream models configured upstream instances and their health state.
package upstream

import (
	"errors"
	"net/url"
	"time"

	"github.com/Djunichi/APIGateway/internal/circuitbreaker"
)

// Target is one configured upstream instance with an independent circuit breaker.
// It is safe for concurrent use.
type Target struct {
	url     url.URL
	breaker *circuitbreaker.Breaker
}

// NewTarget creates a target and copies its URL.
func NewTarget(target *url.URL, breaker *circuitbreaker.Breaker) (*Target, error) {
	if target == nil {
		return nil, errors.New("target URL is required")
	}
	if breaker == nil {
		return nil, errors.New("circuit breaker is required")
	}
	return &Target{url: *target, breaker: breaker}, nil
}

// URL returns a copy of the target URL.
func (t *Target) URL() url.URL {
	return t.url
}

// Acquire attempts to reserve this target for one upstream attempt.
func (t *Target) Acquire(now time.Time) (circuitbreaker.DoneFunc, bool) {
	return t.breaker.Acquire(now)
}

// Snapshot returns the target's current circuit breaker state.
func (t *Target) Snapshot() circuitbreaker.Snapshot {
	return t.breaker.Snapshot()
}
