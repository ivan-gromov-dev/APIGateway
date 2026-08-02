package balancer

import (
	"context"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Djunichi/APIGateway/internal/circuitbreaker"
	"github.com/Djunichi/APIGateway/internal/upstream"
)

func TestNewRoundRobinRequiresUpstream(t *testing.T) {
	if _, err := NewRoundRobin(nil); err == nil {
		t.Fatal("expected an error for an empty upstream list")
	}
}

func TestRoundRobinCyclesInOrder(t *testing.T) {
	targets := []*upstream.Target{
		newTarget(t, "first", 3),
		newTarget(t, "second", 3),
		newTarget(t, "third", 3),
	}
	balancer, err := NewRoundRobin(targets)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"first", "second", "third", "first", "second", "third"}
	for i, expected := range want {
		selection, ok := balancer.Next(context.Background(), time.Time{})
		if !ok {
			t.Fatalf("selection %d unavailable", i)
		}
		if selected := selection.Target.URL().Host; selected != expected {
			t.Fatalf("selection %d = %s, want %s", i, selected, expected)
		}
		selection.Done(circuitbreaker.OutcomeSuccess, time.Time{})
	}
}

func TestRoundRobinSkipsOpenTargets(t *testing.T) {
	first := newTarget(t, "first", 1)
	second := newTarget(t, "second", 1)
	openTarget(t, first)
	balancer, err := NewRoundRobin([]*upstream.Target{first, second})
	if err != nil {
		t.Fatal(err)
	}

	selection, ok := balancer.Next(context.Background(), time.Time{})
	if !ok || selection.Target.URL().Host != "second" {
		t.Fatalf("selection = %+v available=%v, want second", selection, ok)
	}
	selection.Done(circuitbreaker.OutcomeSuccess, time.Time{})
}

func TestRoundRobinReportsNoAvailableTarget(t *testing.T) {
	first := newTarget(t, "first", 1)
	second := newTarget(t, "second", 1)
	openTarget(t, first)
	openTarget(t, second)
	balancer, err := NewRoundRobin([]*upstream.Target{first, second})
	if err != nil {
		t.Fatal(err)
	}

	if selection, ok := balancer.Next(context.Background(), time.Time{}); ok || selection.Target != nil || selection.Done != nil {
		t.Fatalf("selection = %+v available=%v", selection, ok)
	}
}

func TestRoundRobinCopiesInput(t *testing.T) {
	first := newTarget(t, "first", 1)
	targets := []*upstream.Target{first}
	balancer, err := NewRoundRobin(targets)
	if err != nil {
		t.Fatal(err)
	}
	targets[0] = newTarget(t, "mutated", 1)

	selection, ok := balancer.Next(context.Background(), time.Time{})
	if !ok || selection.Target != first {
		t.Fatal("caller mutation changed balancer targets")
	}
}

func TestRoundRobinIsSafeForConcurrentUse(t *testing.T) {
	targets := []*upstream.Target{newTarget(t, "first", 1000), newTarget(t, "second", 1000)}
	balancer, err := NewRoundRobin(targets)
	if err != nil {
		t.Fatal(err)
	}

	var counts [2]atomic.Int64
	var waitGroup sync.WaitGroup
	const requests = 100
	waitGroup.Add(requests)
	for range requests {
		go func() {
			defer waitGroup.Done()
			selection, ok := balancer.Next(context.Background(), time.Time{})
			if !ok {
				t.Error("target unavailable")
				return
			}
			if selection.Target == targets[0] {
				counts[0].Add(1)
			} else {
				counts[1].Add(1)
			}
			selection.Done(circuitbreaker.OutcomeSuccess, time.Time{})
		}()
	}
	waitGroup.Wait()

	if counts[0].Load() != requests/2 || counts[1].Load() != requests/2 {
		t.Fatalf("distribution = [%d, %d]", counts[0].Load(), counts[1].Load())
	}
}

func newTarget(t *testing.T, host string, threshold int) *upstream.Target {
	t.Helper()
	breaker, err := circuitbreaker.New(circuitbreaker.Config{
		FailureThreshold: threshold,
		OpenTimeout:      time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	target, err := upstream.NewTarget(&url.URL{Scheme: "http", Host: host}, breaker)
	if err != nil {
		t.Fatal(err)
	}
	return target
}

func openTarget(t *testing.T, target *upstream.Target) {
	t.Helper()
	done, allowed := target.Acquire(time.Time{})
	if !allowed {
		t.Fatal("target unavailable before opening")
	}
	done(circuitbreaker.OutcomeFailure, time.Time{})
}
