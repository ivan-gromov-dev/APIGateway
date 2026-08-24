package balancer

import (
	"context"
	"hash/fnv"
	"net/http"
	"time"

	"github.com/ivan-gromov-dev/APIGateway/internal/config"
	"github.com/ivan-gromov-dev/APIGateway/internal/upstream"
)

// Rollout selects the final upstream as a canary according to a bounded
// percentage, header presence, or stable hash. The remaining upstreams use
// the supplied weighted policy. It is safe for concurrent requests.
type Rollout struct {
	primary Balancer
	canary  *upstream.Target
	policy  config.Rollout
}

func NewRollout(primary Balancer, canary *upstream.Target, policy config.Rollout) *Rollout {
	return &Rollout{primary: primary, canary: canary, policy: policy}
}
func (r *Rollout) Next(ctx context.Context, now time.Time) (Selection, bool) {
	if r.canary != nil && eligible(ctx, r.policy) {
		if done, ok := r.canary.Acquire(now); ok {
			return Selection{Target: r.canary, Done: done}, true
		}
	}
	return r.primary.Next(ctx, now)
}
func eligible(ctx context.Context, p config.Rollout) bool {
	if p.Strategy == "header" {
		return headerValue(ctx, p.Header) != ""
	}
	value := ""
	if p.Strategy == "stable_hash" {
		value = headerValue(ctx, p.StableHash)
	} else {
		value = headerValue(ctx, "X-Request-ID")
	}
	if value == "" {
		return false
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(value))
	return int(h.Sum32()%100) < p.Percentage
}
func headerValue(ctx context.Context, name string) string {
	if request, ok := ctx.Value(requestKey{}).(*http.Request); ok && request != nil {
		return request.Header.Get(name)
	}
	return ""
}

type requestKey struct{}

func WithRequest(ctx context.Context, request *http.Request) context.Context {
	return context.WithValue(ctx, requestKey{}, request)
}

var _ Balancer = (*Rollout)(nil)
