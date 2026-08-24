package redis

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/ivan-gromov-dev/APIGateway/internal/cache"
)

type fakeClient struct {
	value          string
	getErr, setErr error
	key            string
}

func (f *fakeClient) Get(_ context.Context, key string) *goredis.StringCmd {
	f.key = key
	return goredis.NewStringResult(f.value, f.getErr)
}
func (f *fakeClient) Set(_ context.Context, key string, value any, _ time.Duration) *goredis.StatusCmd {
	f.key = key
	if text, ok := value.([]byte); ok {
		f.value = string(text)
	}
	return goredis.NewStatusResult("OK", f.setErr)
}
func (*fakeClient) Ping(context.Context) *goredis.StatusCmd {
	return goredis.NewStatusResult("PONG", nil)
}
func (*fakeClient) Close() error { return nil }

func TestStoreRoundTrip(t *testing.T) {
	client := &fakeClient{}
	store := &Store{client: client, prefix: "cache"}
	entry := cache.Entry{Status: 200, Header: http.Header{"Content-Type": {"text/plain"}}, Body: []byte("body")}
	if err := store.Set(context.Background(), "raw-secret-key", entry, time.Minute); err != nil {
		t.Fatal(err)
	}
	if client.key == "raw-secret-key" {
		t.Fatal("key was not hashed")
	}
	got, found, err := store.Get(context.Background(), "raw-secret-key")
	if err != nil || !found || got.Status != 200 || string(got.Body) != "body" {
		t.Fatalf("got=%+v found=%v err=%v", got, found, err)
	}
}

func TestStoreMissAndErrors(t *testing.T) {
	store := &Store{client: &fakeClient{getErr: goredis.Nil}, prefix: "x"}
	if _, found, err := store.Get(context.Background(), "x"); err != nil || found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	store.client = &fakeClient{getErr: errors.New("down")}
	if _, _, err := store.Get(context.Background(), "x"); err == nil {
		t.Fatal("expected get error")
	}
	store.client = &fakeClient{setErr: errors.New("down")}
	if err := store.Set(context.Background(), "x", cache.Entry{}, time.Minute); err == nil {
		t.Fatal("expected set error")
	}
	store.client = &fakeClient{value: "{"}
	if _, _, err := store.Get(context.Background(), "x"); err == nil {
		t.Fatal("expected decode error")
	}
	store.client = &fakeClient{}
	if err := store.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}
