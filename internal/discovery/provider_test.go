package discovery

import (
	"context"
	"testing"

	"github.com/Djunichi/APIGateway/internal/config"
)

func TestRegistrySelectsProvider(t *testing.T) {
	expected := []string{"http://127.0.0.1:8080"}
	registry := Registry{"test": providerFunc(func(context.Context, config.Discovery) ([]string, error) { return expected, nil })}
	got, err := registry.Resolve(context.Background(), config.Discovery{Provider: "test"})
	if err != nil || len(got) != 1 || got[0] != expected[0] {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

func TestRegistryRejectsUnknownProvider(t *testing.T) {
	if _, err := (Registry{}).Resolve(context.Background(), config.Discovery{Provider: "missing"}); err == nil {
		t.Fatal("expected unknown provider error")
	}
}

type providerFunc func(context.Context, config.Discovery) ([]string, error)

func (f providerFunc) Resolve(ctx context.Context, cfg config.Discovery) ([]string, error) {
	return f(ctx, cfg)
}
