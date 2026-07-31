// Package redis implements response cache storage using Redis.
package redis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/Djunichi/APIGateway/internal/cache"
)

type Config struct {
	Address, Username, Password, KeyPrefix string
	Database                               int
}

type Store struct {
	client client
	prefix string
}

type client interface {
	Get(context.Context, string) *goredis.StringCmd
	Set(context.Context, string, any, time.Duration) *goredis.StatusCmd
	Ping(context.Context) *goredis.StatusCmd
	Close() error
}

func New(cfg Config) *Store {
	return &Store{client: goredis.NewClient(&goredis.Options{
		Addr: cfg.Address, Username: cfg.Username, Password: cfg.Password, DB: cfg.Database,
	}), prefix: cfg.KeyPrefix}
}

func (s *Store) Ping(ctx context.Context) error { return s.client.Ping(ctx).Err() }
func (s *Store) Close() error                   { return s.client.Close() }

func (s *Store) Get(ctx context.Context, key string) (cache.Entry, bool, error) {
	data, err := s.client.Get(ctx, s.key(key)).Bytes()
	if err == goredis.Nil {
		return cache.Entry{}, false, nil
	}
	if err != nil {
		return cache.Entry{}, false, fmt.Errorf("get cached response: %w", err)
	}
	var entry cache.Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		return cache.Entry{}, false, fmt.Errorf("decode cached response: %w", err)
	}
	return entry, true, nil
}

func (s *Store) Set(ctx context.Context, key string, entry cache.Entry, ttl time.Duration) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encode cached response: %w", err)
	}
	if err := s.client.Set(ctx, s.key(key), data, ttl).Err(); err != nil {
		return fmt.Errorf("set cached response: %w", err)
	}
	return nil
}

func (s *Store) key(value string) string {
	sum := sha256.Sum256([]byte(value))
	return s.prefix + ":" + hex.EncodeToString(sum[:])
}
