package balancer

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/Djunichi/APIGateway/internal/upstream"
)

// RoundRobin selects upstreams sequentially and is safe for concurrent use.
type RoundRobin struct {
	upstreams []*upstream.Target
	next      atomic.Uint64
}

// NewRoundRobin creates a round-robin balancer for a non-empty upstream list.
func NewRoundRobin(upstreams []*upstream.Target) (*RoundRobin, error) {
	if len(upstreams) == 0 {
		return nil, errors.New("at least one upstream is required")
	}
	return &RoundRobin{upstreams: append([]*upstream.Target(nil), upstreams...)}, nil
}

// Next returns the next available upstream in rotation.
func (r *RoundRobin) Next(_ context.Context, now time.Time) (Selection, bool) {
	index := r.next.Add(1) - 1
	for offset := range uint64(len(r.upstreams)) {
		target := r.upstreams[(index+offset)%uint64(len(r.upstreams))]
		if done, allowed := target.Acquire(now); allowed {
			return Selection{Target: target, Done: done}, true
		}
	}
	return Selection{}, false
}

var _ Balancer = (*RoundRobin)(nil)
