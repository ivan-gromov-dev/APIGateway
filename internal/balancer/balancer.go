// Package balancer distributes requests across upstream service instances.
package balancer

import (
	"context"
	"time"

	"github.com/Djunichi/APIGateway/internal/circuitbreaker"
	"github.com/Djunichi/APIGateway/internal/upstream"
)

// Selection binds an available target to its circuit breaker completion callback.
type Selection struct {
	Target *upstream.Target
	Done   circuitbreaker.DoneFunc
}

// Balancer selects an available upstream for the next attempt.
type Balancer interface {
	Next(ctx context.Context, now time.Time) (Selection, bool)
}

// Factory creates an independent balancer for a route.
type Factory func(upstreams []*upstream.Target) (Balancer, error)
