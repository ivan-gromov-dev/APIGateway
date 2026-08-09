package integration

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Djunichi/APIGateway/internal/config"
	"github.com/Djunichi/APIGateway/internal/discovery"
	"github.com/Djunichi/APIGateway/internal/server"
)

type refreshProvider struct {
	mu       sync.Mutex
	answers  [][]string
	calls    int
	canceled chan struct{}
	blocked  chan struct{}
}

func (p *refreshProvider) Resolve(ctx context.Context, _ config.Discovery) ([]string, error) {
	p.mu.Lock()
	p.calls++
	call := p.calls
	answer := p.answers[min(call-1, len(p.answers)-1)]
	p.mu.Unlock()
	if call >= 4 {
		select {
		case <-p.blocked:
		default:
			close(p.blocked)
		}
		<-ctx.Done()
		select {
		case <-p.canceled:
		default:
			close(p.canceled)
		}
		return nil, ctx.Err()
	}
	return append([]string(nil), answer...), nil
}

func TestContinuousDNSMembershipPublishesDuringConcurrentTraffic(t *testing.T) {
	first := discoveryBackend(t, "first")
	second := discoveryBackend(t, "second")
	provider := &refreshProvider{answers: [][]string{{first.URL}, {second.URL}}, canceled: make(chan struct{}), blocked: make(chan struct{})}
	publicAddress, adminAddress := integrationFreeAddress(t), integrationFreeAddress(t)
	path := filepath.Join(t.TempDir(), "gateway.yaml")
	yaml := fmt.Sprintf("server:\n  address: %s\nadmin:\n  address: %s\nroutes:\n  - path_prefix: /api/\n    discovery:\n      provider: dns\n      name: users\n      scheme: http\n      port: 8080\n      interval: 15ms\n      grace: 100ms\n", publicAddress, adminAddress)
	if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	gateway := server.New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), server.WithDiscoveryRegistry(discovery.Registry{"dns": provider}))
	go func() { done <- gateway.Run(ctx) }()
	waitIntegrationStatus(t, "http://"+adminAddress+"/readyz", http.StatusOK)

	var group sync.WaitGroup
	for range 20 {
		group.Add(1)
		go func() {
			defer group.Done()
			response, requestErr := http.Get("http://" + publicAddress + "/api/")
			if requestErr == nil {
				_ = response.Body.Close()
			}
		}()
	}
	group.Wait()
	waitIntegrationBody(t, "http://"+publicAddress+"/api/", "second")
	select {
	case <-provider.blocked:
	case <-time.After(time.Second):
		t.Fatal("discovery resolver did not begin refresh")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	select {
	case <-provider.canceled:
	case <-time.After(time.Second):
		t.Fatal("discovery resolver did not observe runtime cancellation")
	}
}

func discoveryBackend(t *testing.T, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
	t.Cleanup(server.Close)
	return server
}

func integrationFreeAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	return address
}

func waitIntegrationStatus(t *testing.T, url string, want int) {
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

func waitIntegrationBody(t *testing.T, url, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(url)
		if err == nil {
			body, _ := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if string(body) == want {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%s did not return %q", url, want)
}
