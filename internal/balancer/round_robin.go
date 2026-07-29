package balancer

import (
	"errors"
	"net/url"
	"sync/atomic"
)

// RoundRobin selects upstreams sequentially and is safe for concurrent use.
type RoundRobin struct {
	upstreams []*url.URL
	next      atomic.Uint64
}

// NewRoundRobin creates a round-robin balancer for a non-empty upstream list.
func NewRoundRobin(upstreams []*url.URL) (*RoundRobin, error) {
	if len(upstreams) == 0 {
		return nil, errors.New("at least one upstream is required")
	}
	return &RoundRobin{upstreams: append([]*url.URL(nil), upstreams...)}, nil
}

// Next returns the next upstream in rotation.
func (r *RoundRobin) Next() *url.URL {
	index := r.next.Add(1) - 1
	return r.upstreams[index%uint64(len(r.upstreams))]
}

var _ Balancer = (*RoundRobin)(nil)
