package circuitbreaker

import "time"

// State is the current circuit breaker state.
type State uint8

const (
	// StateClosed admits normal traffic and records outcomes.
	StateClosed State = iota
	// StateOpen rejects traffic until the open timeout expires.
	StateOpen
	// StateHalfOpen admits only the probe that entered this state.
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Outcome describes the result of an admitted attempt.
type Outcome uint8

const (
	// OutcomeSuccess records a successful attempt.
	OutcomeSuccess Outcome = iota
	// OutcomeFailure records a failed attempt.
	OutcomeFailure
)

// Snapshot is an immutable view of a Breaker.
type Snapshot struct {
	State               State
	ConsecutiveFailures int
	OpenedAt            time.Time
}
