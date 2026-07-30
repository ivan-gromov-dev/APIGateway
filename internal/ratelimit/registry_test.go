package ratelimit

import (
	"context"
	"net/http"
	"testing"
)

type stubStore struct{}

func (stubStore) Take(context.Context, TakeRequest) (TakeResult, error) { return TakeResult{}, nil }

func TestRegistryRegistrationAndFreeze(t *testing.T) {
	registry := NewRegistry()
	if err := registry.RegisterStore("store", stubStore{}); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterKey("key", func(*http.Request) (string, error) { return "key", nil }); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterFilter("filter", func(*http.Request) bool { return true }); err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Store("store"); !ok {
		t.Fatal("store missing")
	}
	if _, ok := registry.Key("key"); !ok {
		t.Fatal("key missing")
	}
	if _, ok := registry.Filter("filter"); !ok {
		t.Fatal("filter missing")
	}
	if err := registry.RegisterStore("store", stubStore{}); err == nil {
		t.Fatal("duplicate accepted")
	}
	registry.Freeze()
	if err := registry.RegisterStore("other", stubStore{}); err == nil {
		t.Fatal("frozen registry changed")
	}
}
