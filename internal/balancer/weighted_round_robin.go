package balancer

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/Djunichi/APIGateway/internal/upstream"
)

// WeightedRoundRobin selects targets in a smooth weighted cycle. Weights are
// copied at construction, must be positive, and are bounded by the route's
// configured upstream list. Next is lock-free and skips unavailable targets.
type WeightedRoundRobin struct {
	upstreams []*upstream.Target
	weights   []int
	total     uint64
	next      atomic.Uint64
}

func NewWeightedRoundRobin(upstreams []*upstream.Target, weights []int) (*WeightedRoundRobin, error) {
	if len(upstreams) == 0 || len(upstreams) != len(weights) {
		return nil, errors.New("upstreams and weights must be non-empty and equal")
	}
	var total int
	for _, w := range weights {
		if w <= 0 {
			return nil, errors.New("weights must be positive")
		}
		total += w
	}
	return &WeightedRoundRobin{upstreams: append([]*upstream.Target(nil), upstreams...), weights: append([]int(nil), weights...), total: uint64(total)}, nil
}

func (w *WeightedRoundRobin) Next(_ context.Context, now time.Time) (Selection, bool) {
	point := w.next.Add(1) - 1
	for offset := uint64(0); offset < w.total; offset++ {
		pos := (point + offset) % w.total
		for i, weight := range w.weights {
			if pos < uint64(weight) {
				if done, ok := w.upstreams[i].Acquire(now); ok {
					return Selection{Target: w.upstreams[i], Done: done}, true
				}
				break
			}
			pos -= uint64(weight)
		}
	}
	return Selection{}, false
}

var _ Balancer = (*WeightedRoundRobin)(nil)
