package circuitbreaker

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var baseTime = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

func TestNewValidatesConfig(t *testing.T) {
	tests := []Config{
		{FailureThreshold: 0, OpenTimeout: time.Second},
		{FailureThreshold: -1, OpenTimeout: time.Second},
		{FailureThreshold: 1, OpenTimeout: 0},
		{FailureThreshold: 1, OpenTimeout: -time.Second},
	}
	for _, cfg := range tests {
		if _, err := New(cfg); err == nil {
			t.Fatalf("New(%+v) succeeded", cfg)
		}
	}
}

func TestStateString(t *testing.T) {
	tests := map[State]string{
		StateClosed:   "closed",
		StateOpen:     "open",
		StateHalfOpen: "half-open",
		State(99):     "unknown",
	}
	for state, want := range tests {
		if got := state.String(); got != want {
			t.Fatalf("State(%d).String() = %q, want %q", state, got, want)
		}
	}
}

func TestClosedCircuitCountsFailuresAndSuccessResetsThem(t *testing.T) {
	breaker := newTestBreaker(t, 3, time.Minute)

	completeAttempt(t, breaker, baseTime, OutcomeFailure, baseTime)
	if got := breaker.Snapshot().ConsecutiveFailures; got != 1 {
		t.Fatalf("failures = %d, want 1", got)
	}
	completeAttempt(t, breaker, baseTime, OutcomeSuccess, baseTime)
	if got := breaker.Snapshot().ConsecutiveFailures; got != 0 {
		t.Fatalf("failures = %d, want 0", got)
	}
	completeAttempt(t, breaker, baseTime, OutcomeFailure, baseTime)
	completeAttempt(t, breaker, baseTime, OutcomeFailure, baseTime)
	completeAttempt(t, breaker, baseTime, OutcomeFailure, baseTime)

	snapshot := breaker.Snapshot()
	if snapshot.State != StateOpen || !snapshot.OpenedAt.Equal(baseTime) {
		t.Fatalf("snapshot = %+v, want open at %s", snapshot, baseTime)
	}
}

func TestOpenCircuitAllowsOneProbeAtTimeout(t *testing.T) {
	timeout := time.Minute
	breaker := openTestBreaker(t, timeout)

	if done, allowed := breaker.Acquire(baseTime.Add(timeout - time.Nanosecond)); allowed || done != nil {
		t.Fatal("request before timeout was allowed")
	}
	probe, allowed := breaker.Acquire(baseTime.Add(timeout))
	if !allowed || probe == nil {
		t.Fatal("probe at timeout was rejected")
	}
	if snapshot := breaker.Snapshot(); snapshot.State != StateHalfOpen {
		t.Fatalf("state = %s, want half-open", snapshot.State)
	}
	if done, allowed := breaker.Acquire(baseTime.Add(timeout)); allowed || done != nil {
		t.Fatal("second half-open probe was allowed")
	}
}

func TestSuccessfulProbeClosesCircuit(t *testing.T) {
	timeout := time.Minute
	breaker := openTestBreaker(t, timeout)
	probe, allowed := breaker.Acquire(baseTime.Add(timeout))
	if !allowed {
		t.Fatal("probe was rejected")
	}
	probe(OutcomeSuccess, baseTime.Add(timeout))

	snapshot := breaker.Snapshot()
	if snapshot.State != StateClosed || !snapshot.OpenedAt.IsZero() {
		t.Fatalf("snapshot = %+v, want closed", snapshot)
	}
	if _, allowed := breaker.Acquire(baseTime.Add(timeout)); !allowed {
		t.Fatal("closed circuit rejected request")
	}
}

func TestFailedProbeReopensCircuitFromCompletionTime(t *testing.T) {
	timeout := time.Minute
	breaker := openTestBreaker(t, timeout)
	probeAt := baseTime.Add(timeout)
	probe, allowed := breaker.Acquire(probeAt)
	if !allowed {
		t.Fatal("probe was rejected")
	}
	completedAt := probeAt.Add(10 * time.Second)
	probe(OutcomeFailure, completedAt)

	snapshot := breaker.Snapshot()
	if snapshot.State != StateOpen || !snapshot.OpenedAt.Equal(completedAt) {
		t.Fatalf("snapshot = %+v, want reopened at %s", snapshot, completedAt)
	}
	if _, allowed := breaker.Acquire(completedAt.Add(timeout - time.Nanosecond)); allowed {
		t.Fatal("request was allowed before restarted timeout")
	}
}

func TestDoneCallbackIsIdempotent(t *testing.T) {
	breaker := newTestBreaker(t, 2, time.Minute)
	done, allowed := breaker.Acquire(baseTime)
	if !allowed {
		t.Fatal("request was rejected")
	}
	done(OutcomeFailure, baseTime)
	done(OutcomeFailure, baseTime)
	if got := breaker.Snapshot().ConsecutiveFailures; got != 1 {
		t.Fatalf("failures = %d, want 1", got)
	}
}

func TestNeutralOutcomeDoesNotChangeClosedCircuitHistory(t *testing.T) {
	breaker := newTestBreaker(t, 3, time.Minute)
	completeAttempt(t, breaker, baseTime, OutcomeFailure, baseTime)
	completeAttempt(t, breaker, baseTime, OutcomeNeutral, baseTime.Add(time.Second))

	snapshot := breaker.Snapshot()
	if snapshot.State != StateClosed || snapshot.ConsecutiveFailures != 1 {
		t.Fatalf("snapshot = %+v, want closed with one failure", snapshot)
	}
}

func TestNeutralProbeReopensCircuitAndRestartsTimeout(t *testing.T) {
	timeout := time.Minute
	breaker := openTestBreaker(t, timeout)
	probeAt := baseTime.Add(timeout)
	probe, allowed := breaker.Acquire(probeAt)
	if !allowed {
		t.Fatal("probe was rejected")
	}
	completedAt := probeAt.Add(10 * time.Second)
	probe(OutcomeNeutral, completedAt)

	snapshot := breaker.Snapshot()
	if snapshot.State != StateOpen || !snapshot.OpenedAt.Equal(completedAt) {
		t.Fatalf("snapshot = %+v, want reopened at %s", snapshot, completedAt)
	}
	if _, allowed := breaker.Acquire(completedAt.Add(timeout - time.Nanosecond)); allowed {
		t.Fatal("request was allowed before restarted timeout")
	}
	if _, allowed := breaker.Acquire(completedAt.Add(timeout)); !allowed {
		t.Fatal("probe was rejected after restarted timeout")
	}
}

func TestStaleResultCannotChangeOpenedCircuit(t *testing.T) {
	breaker := newTestBreaker(t, 1, time.Minute)
	stale, _ := breaker.Acquire(baseTime)
	opening, _ := breaker.Acquire(baseTime)
	opening(OutcomeFailure, baseTime)
	stale(OutcomeSuccess, baseTime.Add(time.Second))

	if snapshot := breaker.Snapshot(); snapshot.State != StateOpen {
		t.Fatalf("state = %s, want open", snapshot.State)
	}
}

func TestOnlyOneConcurrentHalfOpenProbeIsAllowed(t *testing.T) {
	breaker := openTestBreaker(t, time.Minute)
	const workers = 100
	var allowed atomic.Int32
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			if _, ok := breaker.Acquire(baseTime.Add(time.Minute)); ok {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := allowed.Load(); got != 1 {
		t.Fatalf("allowed probes = %d, want 1", got)
	}
}

func TestConcurrentDoneCallsApplyOneOutcome(t *testing.T) {
	breaker := newTestBreaker(t, 2, time.Minute)
	done, _ := breaker.Acquire(baseTime)
	const workers = 100
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			done(OutcomeFailure, baseTime)
		}()
	}
	wg.Wait()
	if got := breaker.Snapshot().ConsecutiveFailures; got != 1 {
		t.Fatalf("failures = %d, want 1", got)
	}
}

func newTestBreaker(t *testing.T, threshold int, timeout time.Duration) *Breaker {
	t.Helper()
	breaker, err := New(Config{FailureThreshold: threshold, OpenTimeout: timeout})
	if err != nil {
		t.Fatal(err)
	}
	return breaker
}

func openTestBreaker(t *testing.T, timeout time.Duration) *Breaker {
	t.Helper()
	breaker := newTestBreaker(t, 1, timeout)
	completeAttempt(t, breaker, baseTime, OutcomeFailure, baseTime)
	return breaker
}

func completeAttempt(t *testing.T, breaker *Breaker, acquiredAt time.Time, outcome Outcome, completedAt time.Time) {
	t.Helper()
	done, allowed := breaker.Acquire(acquiredAt)
	if !allowed {
		t.Fatal("attempt was rejected")
	}
	done(outcome, completedAt)
}
