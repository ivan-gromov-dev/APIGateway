package configwatcher

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestWatchReloadsOnChangeAndStops(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gateway.yaml")
	if err := os.WriteFile(path, []byte("one"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	called := make(chan struct{}, 1)
	var once sync.Once
	go func() {
		_ = Watch(ctx, path, time.Millisecond, func(context.Context, string) error { once.Do(func() { called <- struct{}{} }); return nil })
	}()
	time.Sleep(3 * time.Millisecond)
	if err := os.WriteFile(path, []byte("two"), 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("reload was not called")
	}
	cancel()
}

func TestWatchValidatesArguments(t *testing.T) {
	ctx := context.Background()
	if err := Watch(ctx, "missing", time.Second, func(context.Context, string) error { return nil }); err == nil {
		t.Fatal("expected stat error")
	}
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Watch(ctx, path, 0, func(context.Context, string) error { return nil }); err == nil {
		t.Fatal("expected interval error")
	}
}
