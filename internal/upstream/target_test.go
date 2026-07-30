package upstream

import (
	"net/url"
	"testing"
	"time"

	"github.com/Djunichi/APIGateway/internal/circuitbreaker"
)

func TestTargetCopiesURLAndDelegatesCircuitState(t *testing.T) {
	raw := &url.URL{Scheme: "http", Host: "upstream.example"}
	breaker, err := circuitbreaker.New(circuitbreaker.Config{
		FailureThreshold: 1,
		OpenTimeout:      time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	target, err := NewTarget(raw, breaker)
	if err != nil {
		t.Fatal(err)
	}
	raw.Host = "mutated.example"

	if got := target.URL().Host; got != "upstream.example" {
		t.Fatalf("host = %q", got)
	}
	done, allowed := target.Acquire(time.Time{})
	if !allowed {
		t.Fatal("healthy target was rejected")
	}
	done(circuitbreaker.OutcomeFailure, time.Time{})
	if snapshot := target.Snapshot(); snapshot.State != circuitbreaker.StateOpen {
		t.Fatalf("state = %s, want open", snapshot.State)
	}
}

func TestNewTargetValidatesDependencies(t *testing.T) {
	raw := &url.URL{Scheme: "http", Host: "upstream.example"}
	breaker, err := circuitbreaker.New(circuitbreaker.Config{
		FailureThreshold: 1, OpenTimeout: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewTarget(nil, breaker); err == nil {
		t.Fatal("nil URL was accepted")
	}
	if _, err := NewTarget(raw, nil); err == nil {
		t.Fatal("nil circuit breaker was accepted")
	}
}
