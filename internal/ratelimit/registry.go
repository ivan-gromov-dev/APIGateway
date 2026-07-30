package ratelimit

import (
	"errors"
	"fmt"
	"sync"
)

// Registry contains startup-time rate-limit extension registrations.
type Registry struct {
	mu      sync.RWMutex
	frozen  bool
	stores  map[string]Store
	keys    map[string]KeyFunc
	filters map[string]FilterFunc
}

// NewRegistry constructs an empty mutable registry.
func NewRegistry() *Registry {
	return &Registry{
		stores: make(map[string]Store), keys: make(map[string]KeyFunc),
		filters: make(map[string]FilterFunc),
	}
}

// RegisterStore adds a named backend.
func (r *Registry) RegisterStore(name string, store Store) error {
	if store == nil {
		return errors.New("rate limit store is required")
	}
	return r.register(name, func() bool { _, ok := r.stores[name]; return ok }, func() { r.stores[name] = store })
}

// RegisterKey adds a named identity extractor.
func (r *Registry) RegisterKey(name string, key KeyFunc) error {
	if key == nil {
		return errors.New("rate limit key function is required")
	}
	return r.register(name, func() bool { _, ok := r.keys[name]; return ok }, func() { r.keys[name] = key })
}

// RegisterFilter adds a named request filter.
func (r *Registry) RegisterFilter(name string, filter FilterFunc) error {
	if filter == nil {
		return errors.New("rate limit filter is required")
	}
	return r.register(name, func() bool { _, ok := r.filters[name]; return ok }, func() { r.filters[name] = filter })
}

func (r *Registry) register(name string, exists func() bool, add func()) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.frozen {
		return errors.New("rate limit registry is frozen")
	}
	if name == "" {
		return errors.New("rate limit registration name is required")
	}
	if exists() {
		return fmt.Errorf("rate limit registration %q already exists", name)
	}
	add()
	return nil
}

// Freeze prevents further registration.
func (r *Registry) Freeze() {
	r.mu.Lock()
	r.frozen = true
	r.mu.Unlock()
}

// Store resolves a named backend.
func (r *Registry) Store(name string) (Store, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	value, ok := r.stores[name]
	return value, ok
}

// Key resolves a named identity extractor.
func (r *Registry) Key(name string) (KeyFunc, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	value, ok := r.keys[name]
	return value, ok
}

// Filter resolves a named request filter.
func (r *Registry) Filter(name string) (FilterFunc, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	value, ok := r.filters[name]
	return value, ok
}
