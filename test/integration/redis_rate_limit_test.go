package integration_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/ivan-gromov-dev/APIGateway/internal/ratelimit"
	redisstore "github.com/ivan-gromov-dev/APIGateway/internal/ratestore/redis"
)

func TestRedisTokenBucketIsAtomic(t *testing.T) {
	address := os.Getenv("REDIS_ADDRESS")
	if address == "" {
		t.Skip("REDIS_ADDRESS is not configured")
	}
	store := redisstore.New(redisstore.Config{
		Address: address, KeyPrefix: "gateway:test:" + time.Now().Format("150405.000000"),
	})
	defer store.Close()
	ctx := context.Background()
	request := ratelimit.TakeRequest{Key: "client", Rate: 1, Burst: 2, Cost: 1, TTL: time.Minute}
	for i, allowed := range []bool{true, true, false} {
		result, err := store.Take(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		if result.Allowed != allowed {
			t.Fatalf("decision %d allowed=%v, want %v", i, result.Allowed, allowed)
		}
	}
}
