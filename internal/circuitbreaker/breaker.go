// Package circuitbreaker provides a concurrency-safe circuit breaker state machine.
package circuitbreaker

import (
	"errors"
	"sync"
	"time"
)

// Config defines the thresholds that control a Breaker.
type Config struct {
	FailureThreshold int
	OpenTimeout      time.Duration
}

// DoneFunc reports the outcome of one attempt admitted by Acquire.
// Only the first call has an effect.
type DoneFunc func(outcome Outcome, completedAt time.Time)

// Breaker is a concurrency-safe circuit breaker.
type Breaker struct {
	mu sync.Mutex

	failureThreshold int
	openTimeout      time.Duration

	state               State
	consecutiveFailures int
	openedAt            time.Time
	generation          uint64
}

// New constructs a closed circuit breaker.
func New(cfg Config) (*Breaker, error) {
	if cfg.FailureThreshold <= 0 {
		return nil, errors.New("failure threshold must be positive")
	}
	if cfg.OpenTimeout <= 0 {
		return nil, errors.New("open timeout must be positive")
	}
	return &Breaker{
		failureThreshold: cfg.FailureThreshold,
		openTimeout:      cfg.OpenTimeout,
		state:            StateClosed,
	}, nil
}

// Acquire admits an attempt when the circuit is closed or when it can become
// the single half-open probe. The returned callback must be completed once.
func (b *Breaker) Acquire(now time.Time) (DoneFunc, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case StateClosed:
		return b.doneFunc(b.generation), true
	case StateOpen:
		if now.Before(b.openedAt.Add(b.openTimeout)) {
			return nil, false
		}
		b.state = StateHalfOpen
		b.generation++
		return b.doneFunc(b.generation), true
	case StateHalfOpen:
		return nil, false
	default:
		return nil, false
	}
}

// Snapshot returns an immutable view of the current state.
func (b *Breaker) Snapshot() Snapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	return Snapshot{
		State:               b.state,
		ConsecutiveFailures: b.consecutiveFailures,
		OpenedAt:            b.openedAt,
	}
}

func (b *Breaker) doneFunc(generation uint64) DoneFunc {
	var once sync.Once
	return func(outcome Outcome, completedAt time.Time) {
		once.Do(func() {
			b.complete(generation, outcome, completedAt)
		})
	}
}

func (b *Breaker) complete(generation uint64, outcome Outcome, completedAt time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if generation != b.generation {
		return
	}
	switch b.state {
	case StateClosed:
		b.completeClosed(outcome, completedAt)
	case StateHalfOpen:
		b.completeHalfOpen(outcome, completedAt)
	}
}

func (b *Breaker) completeClosed(outcome Outcome, completedAt time.Time) {
	switch outcome {
	case OutcomeSuccess:
		b.consecutiveFailures = 0
	case OutcomeFailure:
		b.consecutiveFailures++
		if b.consecutiveFailures >= b.failureThreshold {
			b.open(completedAt)
		}
	case OutcomeNeutral:
		// The attempt says nothing about upstream health.
	}
}

func (b *Breaker) completeHalfOpen(outcome Outcome, completedAt time.Time) {
	switch outcome {
	case OutcomeSuccess:
		b.state = StateClosed
		b.consecutiveFailures = 0
		b.openedAt = time.Time{}
		b.generation++
	case OutcomeFailure:
		b.open(completedAt)
	case OutcomeNeutral:
		b.open(completedAt)
	}
}

func (b *Breaker) open(at time.Time) {
	b.state = StateOpen
	b.consecutiveFailures = 0
	b.openedAt = at
	b.generation++
}
