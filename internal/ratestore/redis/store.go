// Package redis implements distributed rate limit storage using Redis.
package redis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/Djunichi/APIGateway/internal/ratelimit"
)

type Config struct {
	Address   string
	Username  string
	Password  string
	Database  int
	KeyPrefix string
}

type Store struct {
	client *goredis.Client
	prefix string
	runner scriptRunner
}

type scriptRunner interface {
	Run(context.Context, []string, ...any) ([]any, error)
}

type redisScriptRunner struct {
	client *goredis.Client
	script *goredis.Script
}

func (r redisScriptRunner) Run(ctx context.Context, keys []string, args ...any) ([]any, error) {
	return r.script.Run(ctx, r.client, keys, args...).Slice()
}

func New(cfg Config) *Store {
	client := goredis.NewClient(&goredis.Options{
		Addr: cfg.Address, Username: cfg.Username, Password: cfg.Password, DB: cfg.Database,
	})
	return &Store{client: client, prefix: cfg.KeyPrefix,
		runner: redisScriptRunner{client: client, script: goredis.NewScript(tokenBucketScript)}}
}

func (s *Store) Ping(ctx context.Context) error {
	return s.client.Ping(ctx).Err()
}

func (s *Store) Close() error {
	return s.client.Close()
}

func (s *Store) Take(ctx context.Context, request ratelimit.TakeRequest) (ratelimit.TakeResult, error) {
	hash := sha256.Sum256([]byte(request.Key))
	key := s.prefix + ":" + hex.EncodeToString(hash[:])
	result, err := s.runner.Run(ctx, []string{key},
		request.Rate, request.Burst, request.Cost, request.TTL.Milliseconds())
	if err != nil {
		return ratelimit.TakeResult{}, fmt.Errorf("execute token bucket: %w", err)
	}
	if len(result) != 3 {
		return ratelimit.TakeResult{}, fmt.Errorf("token bucket returned %d values", len(result))
	}
	allowed, err := asInt64(result[0])
	if err != nil {
		return ratelimit.TakeResult{}, err
	}
	remaining, err := asInt64(result[1])
	if err != nil {
		return ratelimit.TakeResult{}, err
	}
	retryMS, err := asInt64(result[2])
	if err != nil {
		return ratelimit.TakeResult{}, err
	}
	return ratelimit.TakeResult{
		Allowed: allowed == 1, Remaining: float64(remaining) / 1000,
		RetryAfter: time.Duration(retryMS) * time.Millisecond,
	}, nil
}

func asInt64(value any) (int64, error) {
	number, ok := value.(int64)
	if !ok {
		return 0, fmt.Errorf("unexpected token bucket value %T", value)
	}
	return number, nil
}

const tokenBucketScript = `
local rate = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])
local cost = tonumber(ARGV[3])
local ttl = tonumber(ARGV[4])
local now_parts = redis.call('TIME')
local now = tonumber(now_parts[1]) * 1000 + math.floor(tonumber(now_parts[2]) / 1000)
local state = redis.call('HMGET', KEYS[1], 'tokens', 'updated')
local tokens = tonumber(state[1])
local updated = tonumber(state[2])
if tokens == nil then tokens = burst end
if updated == nil then updated = now end
local elapsed = math.max(0, now - updated)
tokens = math.min(burst, tokens + elapsed * rate / 1000)
local allowed = 0
local retry_after = 0
if tokens >= cost then
  tokens = tokens - cost
  allowed = 1
else
  retry_after = math.ceil((cost - tokens) / rate * 1000)
end
redis.call('HSET', KEYS[1], 'tokens', tokens, 'updated', now)
redis.call('PEXPIRE', KEYS[1], ttl)
return {allowed, math.floor(tokens * 1000), retry_after}
`
