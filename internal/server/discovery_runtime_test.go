package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Djunichi/APIGateway/internal/config"
	"github.com/Djunichi/APIGateway/internal/discovery"
)

type changingProvider struct {
	mu      sync.Mutex
	answers [][]string
	err     error
	calls   int
}

func (p *changingProvider) Resolve(context.Context, config.Discovery) ([]string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if p.err != nil {
		return nil, p.err
	}
	answer := p.answers[0]
	if len(p.answers) > 1 {
		p.answers = p.answers[1:]
	}
	return append([]string(nil), answer...), nil
}

func TestContinuousDiscoveryRefreshesAndExpires(t *testing.T) {
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("first")) }))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("second")) }))
	defer second.Close()
	provider := &changingProvider{answers: [][]string{{first.URL}, {second.URL}}}
	cfg := testConfig(freeAddress(t), freeAddress(t), "")
	cfg.Routes[0].Upstreams = nil
	cfg.Routes[0].Discovery = &config.Discovery{Provider: "test", Name: "service", Scheme: "http", Port: 80, Interval: 10 * time.Millisecond, Grace: 35 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	gateway := New(cfg, discardLogger(), WithDiscoveryRegistry(discovery.Registry{"test": provider}))
	done := make(chan error, 1)
	go func() { done <- gateway.Run(ctx) }()
	waitUntilReady(t, "http://"+cfg.Admin.Address+"/readyz")
	waitBody(t, "http://"+cfg.Server.Address+"/api/", "second")

	provider.mu.Lock()
	provider.err = context.DeadlineExceeded
	provider.mu.Unlock()
	waitProviderCalls(t, provider, 3)
	waitBody(t, "http://"+cfg.Server.Address+"/api/", "second")
	waitStatus(t, "http://"+cfg.Admin.Address+"/readyz", http.StatusServiceUnavailable)
	waitStatus(t, "http://"+cfg.Server.Address+"/api/", http.StatusServiceUnavailable)
	provider.mu.Lock()
	provider.err = nil
	provider.mu.Unlock()
	waitUntilReady(t, "http://"+cfg.Admin.Address+"/readyz")
	waitBody(t, "http://"+cfg.Server.Address+"/api/", "second")
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func waitProviderCalls(t *testing.T, provider *changingProvider, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		provider.mu.Lock()
		calls := provider.calls
		provider.mu.Unlock()
		if calls >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("provider calls did not reach %d", want)
}

func waitBody(t *testing.T, url, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(url)
		if err == nil {
			body := make([]byte, len(want))
			_, _ = response.Body.Read(body)
			_ = response.Body.Close()
			if string(body) == want {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%s did not return %q", url, want)
}

func waitStatus(t *testing.T, url string, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(url)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == want {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%s did not return %d", url, want)
}
