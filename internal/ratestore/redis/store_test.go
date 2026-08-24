package redis

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ivan-gromov-dev/APIGateway/internal/ratelimit"
)

func TestTakeMapsScriptResult(t *testing.T) {
	runner := &fakeRunner{result: []any{int64(0), int64(1250), int64(250)}}
	store := &Store{prefix: "gateway", runner: runner}
	result, err := store.Take(context.Background(), ratelimit.TakeRequest{
		Key: "client", Rate: 10, Burst: 20, Cost: 1, TTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Allowed || result.Remaining != 1.25 || result.RetryAfter != 250*time.Millisecond {
		t.Fatalf("result = %+v", result)
	}
	if len(runner.keys) != 1 || !strings.HasPrefix(runner.keys[0], "gateway:") {
		t.Fatalf("keys = %v", runner.keys)
	}
}

func TestTakeRejectsInvalidScriptResults(t *testing.T) {
	tests := []struct {
		name   string
		result []any
		err    error
	}{
		{name: "runner error", err: errors.New("redis unavailable")},
		{name: "length", result: []any{int64(1)}},
		{name: "allowed type", result: []any{"1", int64(0), int64(0)}},
		{name: "remaining type", result: []any{int64(1), "0", int64(0)}},
		{name: "retry type", result: []any{int64(1), int64(0), "0"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &Store{runner: &fakeRunner{result: test.result, err: test.err}}
			if _, err := store.Take(context.Background(), ratelimit.TakeRequest{}); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

type fakeRunner struct {
	keys   []string
	result []any
	err    error
}

func (r *fakeRunner) Run(_ context.Context, keys []string, _ ...any) ([]any, error) {
	r.keys = keys
	return r.result, r.err
}
