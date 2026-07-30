package limiter

import (
	"context"
	"testing"
	"time"

	"github.com/Djunichi/APIGateway/internal/ratelimit"
)

func TestTokenBucketPassesPolicyToStore(t *testing.T) {
	store := &recordingStore{result: ratelimit.TakeResult{Allowed: true}}
	bucket := NewTokenBucket(store, 12.5, 20, time.Minute)
	result, err := bucket.Allow(context.Background(), "client")
	if err != nil || !result.Allowed {
		t.Fatalf("result=%+v error=%v", result, err)
	}
	if store.request.Key != "client" || store.request.Rate != 12.5 ||
		store.request.Burst != 20 || store.request.Cost != 1 || store.request.TTL != time.Minute {
		t.Fatalf("request = %+v", store.request)
	}
}

type recordingStore struct {
	request ratelimit.TakeRequest
	result  ratelimit.TakeResult
}

func (s *recordingStore) Take(_ context.Context, request ratelimit.TakeRequest) (ratelimit.TakeResult, error) {
	s.request = request
	return s.result, nil
}
