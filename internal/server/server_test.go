package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Djunichi/APIGateway/internal/config"
	"github.com/Djunichi/APIGateway/internal/metrics"
	"github.com/Djunichi/APIGateway/internal/ratelimit"
)

func TestReadinessHandlerReflectsState(t *testing.T) {
	var ready atomic.Bool
	handler := adminHandler(&metrics.Collector{}, &ready)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "not_ready") {
		t.Fatalf("not-ready response = %d %q", response.Code, response.Body.String())
	}

	ready.Store(true)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"ready"`) {
		t.Fatalf("ready response = %d %q", response.Code, response.Body.String())
	}
}

func TestRunReportsPublicListenerError(t *testing.T) {
	occupied := listen(t)
	defer occupied.Close()
	cfg := testConfig(occupied.Addr().String(), freeAddress(t), "http://localhost:8081")

	err := New(cfg, discardLogger()).Run(context.Background())

	if err == nil || !strings.Contains(err.Error(), "listen on public address") {
		t.Fatalf("error = %v, want public listener error", err)
	}
}

func TestRunReportsAdminListenerError(t *testing.T) {
	occupied := listen(t)
	defer occupied.Close()
	cfg := testConfig(freeAddress(t), occupied.Addr().String(), "http://localhost:8081")

	err := New(cfg, discardLogger()).Run(context.Background())

	if err == nil || !strings.Contains(err.Error(), "listen on admin address") {
		t.Fatalf("error = %v, want admin listener error", err)
	}
}

func TestRunBuildsRateLimitsFromExternalRegistry(t *testing.T) {
	occupied := listen(t)
	defer occupied.Close()
	cfg := testConfig(occupied.Addr().String(), freeAddress(t), "http://localhost:8081")
	cfg.RateLimit = config.RateLimit{
		Enabled: true, DefaultBackend: "custom", OnBackendError: "allow",
		OperationTimeout: time.Second,
		Rules: []config.RateLimitRule{{
			Name: "global", Key: "global", Filter: "all",
			TokenBucket: config.TokenBucket{RequestsPerSecond: 1, Burst: 1, TTL: time.Minute},
		}},
	}
	registry := ratelimit.NewRegistry()
	if err := registry.RegisterStore("custom", serverStore{}); err != nil {
		t.Fatal(err)
	}

	err := New(cfg, discardLogger(), WithRateLimitRegistry(registry)).Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "listen on public address") {
		t.Fatalf("error = %v", err)
	}
}

type serverStore struct{}

func (serverStore) Take(context.Context, ratelimit.TakeRequest) (ratelimit.TakeResult, error) {
	return ratelimit.TakeResult{Allowed: true}, nil
}

func TestRunGracefullyWaitsForActiveRequest(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		_, _ = w.Write([]byte("done"))
	}))
	defer upstream.Close()

	publicAddress := freeAddress(t)
	adminAddress := freeAddress(t)
	cfg := testConfig(publicAddress, adminAddress, upstream.URL)
	ctx, cancel := context.WithCancel(context.Background())
	runResult := make(chan error, 1)
	gateway := New(cfg, discardLogger())
	go func() {
		runResult <- gateway.Run(ctx)
	}()
	waitUntilReady(t, "http://"+adminAddress+"/readyz")

	requestResult := make(chan error, 1)
	go func() {
		response, err := http.Get("http://" + publicAddress + "/api/")
		if err != nil {
			requestResult <- err
			return
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err == nil && string(body) != "done" {
			err = errors.New("unexpected response body")
		}
		requestResult <- err
	}()
	<-started
	cancel()

	select {
	case err := <-runResult:
		t.Fatalf("server stopped before active request completed: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	if err := <-requestResult; err != nil {
		t.Fatal(err)
	}
	if err := <-runResult; err != nil {
		t.Fatalf("Run returned an error: %v", err)
	}
}

func testConfig(publicAddress, adminAddress, upstream string) config.Config {
	return config.Config{
		Server: config.HTTPServer{
			Address: publicAddress, ReadTimeout: time.Second, WriteTimeout: time.Second,
			IdleTimeout: time.Second, ShutdownTimeout: time.Second,
		},
		Admin: config.HTTPServer{
			Address: adminAddress, ReadTimeout: time.Second, WriteTimeout: time.Second,
			IdleTimeout: time.Second,
		},
		Log: config.Log{Level: "info", Format: "json"},
		Middleware: config.Middleware{
			RequestTimeout: time.Second,
		},
		Routes: []config.Route{{PathPrefix: "/api/", Upstreams: []string{upstream}}},
	}
}

func listen(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return listener
}

func freeAddress(t *testing.T) string {
	t.Helper()
	listener := listen(t)
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return address
}

func waitUntilReady(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(url)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("gateway did not become ready at %s", url)
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
