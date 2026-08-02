package balancer

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Djunichi/APIGateway/internal/circuitbreaker"
	"github.com/Djunichi/APIGateway/internal/config"
	"github.com/Djunichi/APIGateway/internal/upstream"
)

func TestRolloutHeaderSelectsCanary(t *testing.T) {
	primary, canary := newTarget(t, "primary", 3), newTarget(t, "canary", 3)
	r := NewRollout(mustWeighted(t, primary), canary, config.Rollout{Strategy: "header", Header: "X-Canary"})
	request, _ := http.NewRequest(http.MethodGet, "http://gateway", nil)
	request.Header.Set("X-Canary", "true")
	selection, ok := r.Next(WithRequest(context.Background(), request), time.Time{})
	if !ok || selection.Target.URL().Host != "canary" {
		t.Fatalf("selection = %v, ok=%v", selection.Target, ok)
	}
	selection.Done(circuitbreaker.OutcomeSuccess, time.Time{})
}

func TestRolloutPercentageAndStableHashBoundaries(t *testing.T) {
	primary, canary := newTarget(t, "primary", 4), newTarget(t, "canary", 4)
	request, _ := http.NewRequest(http.MethodGet, "http://gateway", nil)
	request.Header.Set("X-Request-ID", "cohort")
	for _, policy := range []config.Rollout{{Strategy: "percentage", Percentage: 100}, {Strategy: "stable_hash", StableHash: "X-Request-ID", Percentage: 100}} {
		r := NewRollout(mustWeighted(t, primary), canary, policy)
		selection, ok := r.Next(WithRequest(context.Background(), request), time.Time{})
		if !ok || selection.Target.URL().Host != "canary" {
			t.Fatalf("policy %#v selected %v, ok=%v", policy, selection.Target, ok)
		}
		selection.Done(circuitbreaker.OutcomeSuccess, time.Time{})
	}
	zero := NewRollout(mustWeighted(t, primary), canary, config.Rollout{Strategy: "percentage", Percentage: 0})
	selection, ok := zero.Next(WithRequest(context.Background(), request), time.Time{})
	if !ok || selection.Target.URL().Host != "primary" {
		t.Fatalf("zero rollout selected %v, ok=%v", selection.Target, ok)
	}
	selection.Done(circuitbreaker.OutcomeSuccess, time.Time{})
}

func TestRolloutFallsBackToPrimaryAndValidatesContext(t *testing.T) {
	primary, canary := newTarget(t, "primary", 3), newTarget(t, "canary", 1)
	r := NewRollout(mustWeighted(t, primary), canary, config.Rollout{Strategy: "header", Header: "X-Canary"})
	selection, ok := r.Next(context.Background(), time.Time{})
	if !ok || selection.Target.URL().Host != "primary" {
		t.Fatalf("missing header selected %v, ok=%v", selection.Target, ok)
	}
	selection.Done(circuitbreaker.OutcomeSuccess, time.Time{})

	request, _ := http.NewRequest(http.MethodGet, "http://gateway", nil)
	request.Header.Set("X-Canary", "true")
	canary.SetHealth(false, time.Now())
	selection, ok = r.Next(WithRequest(context.Background(), request), time.Time{})
	if !ok || selection.Target.URL().Host != "primary" {
		t.Fatalf("unhealthy canary selected %v, ok=%v", selection.Target, ok)
	}
	selection.Done(circuitbreaker.OutcomeSuccess, time.Time{})
}

func mustWeighted(t *testing.T, target *upstream.Target) Balancer {
	t.Helper()
	result, err := NewWeightedRoundRobin([]*upstream.Target{target}, []int{1})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
