package balancer

import (
	"context"
	"github.com/Djunichi/APIGateway/internal/circuitbreaker"
	"github.com/Djunichi/APIGateway/internal/upstream"
	"testing"
	"time"
)

func TestWeightedRoundRobinDistribution(t *testing.T) {
	a, b := newTarget(t, "a", 10), newTarget(t, "b", 10)
	w, err := NewWeightedRoundRobin([]*upstream.Target{a, b}, []int{1, 3})
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for i := 0; i < 40; i++ {
		s, ok := w.Next(context.Background(), time.Time{})
		if !ok {
			t.Fatal("selection unavailable")
		}
		counts[s.Target.URL().Host]++
		s.Done(circuitbreaker.OutcomeSuccess, time.Time{})
	}
	if counts["b"] != 30 || counts["a"] != 10 {
		t.Fatalf("counts = %#v", counts)
	}
}

func TestWeightedRoundRobinValidatesAndCopiesWeights(t *testing.T) {
	if _, err := NewWeightedRoundRobin(nil, nil); err == nil {
		t.Fatal("expected empty error")
	}
	a := newTarget(t, "a", 2)
	weights := []int{1}
	w, err := NewWeightedRoundRobin([]*upstream.Target{a}, weights)
	if err != nil {
		t.Fatal(err)
	}
	weights[0] = 9
	if w.weights[0] != 1 {
		t.Fatal("weights were not copied")
	}
	if _, err := NewWeightedRoundRobin([]*upstream.Target{a}, []int{0}); err == nil {
		t.Fatal("expected positive weight error")
	}
}
