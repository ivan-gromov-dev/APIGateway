// Package testenv provides a reusable real-network gateway integration harness.
package testenv

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/Djunichi/APIGateway/internal/config"
	"github.com/Djunichi/APIGateway/internal/server"
)

// Option configures an integration environment.
type Option func(*settings)

type settings struct {
	users        int
	billing      bool
	retry        config.Retry
	circuit      config.CircuitBreaker
	userStatuses map[int][]int
}

// WithCircuitBreaker configures passive circuit breaker tracking.
func WithCircuitBreaker(circuit config.CircuitBreaker) Option {
	return func(settings *settings) {
		settings.circuit = circuit
	}
}

// WithRetry configures the gateway retry policy.
func WithRetry(retry config.Retry) Option {
	return func(settings *settings) {
		settings.retry = retry
	}
}

// WithUserStatuses configures successive statuses for a users-service instance.
func WithUserStatuses(instance int, statuses ...int) Option {
	return func(settings *settings) {
		if settings.userStatuses == nil {
			settings.userStatuses = make(map[int][]int)
		}
		settings.userStatuses[instance] = append([]int(nil), statuses...)
	}
}

// WithUsers adds the requested number of users-service upstream instances.
func WithUsers(count int) Option {
	return func(settings *settings) {
		settings.users = count
	}
}

// WithBilling adds a billing-service upstream.
func WithBilling() Option {
	return func(settings *settings) {
		settings.billing = true
	}
}

// Environment owns a running gateway, its upstreams, and an HTTP client.
type Environment struct {
	t             testing.TB
	publicBaseURL string
	adminBaseURL  string
	client        *http.Client
	cancel        context.CancelFunc
	runResult     chan error
	stopOnce      sync.Once
	stopErr       error
}

// New starts a gateway with real public and administrative network listeners.
func New(t testing.TB, options ...Option) *Environment {
	t.Helper()
	cfg := settings{}
	for _, option := range options {
		option(&cfg)
	}
	if cfg.users < 0 {
		t.Fatal("users-service instance count must not be negative")
	}
	if cfg.users == 0 && !cfg.billing {
		t.Fatal("at least one upstream service is required")
	}

	routes := make([]config.Route, 0, 2)
	if cfg.billing {
		billing := newUpstream(t, "billing")
		routes = append(routes, config.Route{
			PathPrefix:  "/api/billing/",
			Upstreams:   []string{billing.URL()},
			StripPrefix: true,
		})
	}
	if cfg.users > 0 {
		upstreams := make([]string, 0, cfg.users)
		for i := 1; i <= cfg.users; i++ {
			upstream := newUpstreamWithStatuses(t, fmt.Sprintf("users-%d", i), cfg.userStatuses[i]...)
			upstreams = append(upstreams, upstream.URL())
		}
		routes = append(routes, config.Route{
			PathPrefix:  "/api/",
			Upstreams:   upstreams,
			StripPrefix: true,
		})
	}

	publicAddress := freeAddress(t)
	adminAddress := freeAddress(t)
	gatewayConfig := config.Config{
		Server: config.HTTPServer{
			Address:         publicAddress,
			ReadTimeout:     time.Second,
			WriteTimeout:    time.Second,
			IdleTimeout:     time.Second,
			ShutdownTimeout: 2 * time.Second,
		},
		Admin: config.HTTPServer{
			Address:      adminAddress,
			ReadTimeout:  time.Second,
			WriteTimeout: time.Second,
			IdleTimeout:  time.Second,
		},
		Log: config.Log{Level: "info", Format: "json"},
		Middleware: config.Middleware{
			RequestTimeout: time.Second,
		},
		Retry:          cfg.retry,
		CircuitBreaker: cfg.circuit,
		Routes:         routes,
	}

	ctx, cancel := context.WithCancel(context.Background())
	environment := &Environment{
		t:             t,
		publicBaseURL: "http://" + publicAddress,
		adminBaseURL:  "http://" + adminAddress,
		client:        &http.Client{Timeout: 2 * time.Second},
		cancel:        cancel,
		runResult:     make(chan error, 1),
	}
	gateway := server.New(gatewayConfig, slog.New(slog.NewTextHandler(io.Discard, nil)))
	go func() {
		environment.runResult <- gateway.Run(ctx)
	}()
	t.Cleanup(func() {
		if err := environment.Stop(); err != nil {
			t.Errorf("stop gateway: %v", err)
		}
	})
	environment.waitForReady()
	return environment
}

// Stop gracefully stops the gateway. It is safe to call more than once.
func (e *Environment) Stop() error {
	e.stopOnce.Do(func() {
		e.cancel()
		select {
		case e.stopErr = <-e.runResult:
		case <-time.After(3 * time.Second):
			e.stopErr = fmt.Errorf("gateway did not stop within timeout")
		}
	})
	return e.stopErr
}

func (e *Environment) waitForReady() {
	e.t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		response, err := e.client.Get(e.adminBaseURL + "/readyz")
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	e.t.Fatalf("gateway did not become ready at %s", e.adminBaseURL)
}

func freeAddress(t testing.TB) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release test address: %v", err)
	}
	return address
}
