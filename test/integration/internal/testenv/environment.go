// Package testenv provides a reusable real-network gateway integration harness.
package testenv

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Djunichi/APIGateway/internal/cache"
	"github.com/Djunichi/APIGateway/internal/config"
	"github.com/Djunichi/APIGateway/internal/ratelimit"
	"github.com/Djunichi/APIGateway/internal/server"
	"github.com/Djunichi/APIGateway/internal/telemetry"
)

// Option configures an integration environment.
type Option func(*settings)

type settings struct {
	users        int
	billing      bool
	retry        config.Retry
	circuit      config.CircuitBreaker
	userStatuses map[int][]int
	activeHealth config.ActiveHealthCheck
	rateLimit    config.RateLimit
	rateStore    ratelimit.Store
	auth         config.Auth
	routeAuth    config.RouteAuth
	cache        config.Cache
	routeCache   config.RouteCache
	cacheStore   cache.Store
	balancer     string
	weights      []int
	rollout      config.Rollout
	discovery    bool
	telemetry    *telemetry.Runtime
	grpc         config.GRPC
}

// WithGRPC enables transparent gRPC routes on the public listener.
func WithGRPC(cfg config.GRPC) Option { return func(settings *settings) { settings.grpc = cfg } }

// WithTelemetry installs a scenario-owned tracing runtime.
func WithTelemetry(runtime *telemetry.Runtime) Option {
	return func(settings *settings) { settings.telemetry = runtime }
}

// WithRateLimit configures rate limiting with a scenario-provided store.
func WithRateLimit(cfg config.RateLimit, store ratelimit.Store) Option {
	return func(settings *settings) {
		settings.rateLimit, settings.rateStore = cfg, store
	}
}

// WithAuth protects the users route with a JWT provider.
func WithAuth(cfg config.Auth, route config.RouteAuth) Option {
	return func(settings *settings) { settings.auth, settings.routeAuth = cfg, route }
}

// WithCache enables response caching for the users route.
func WithCache(cfg config.Cache, route config.RouteCache, store cache.Store) Option {
	return func(settings *settings) { settings.cache, settings.routeCache, settings.cacheStore = cfg, route, store }
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

// WithRouteBalancing configures the users route's explicit balancing policy.
func WithRouteBalancing(algorithm string, weights []int, rollout config.Rollout) Option {
	return func(settings *settings) {
		settings.balancer, settings.weights, settings.rollout = algorithm, append([]int(nil), weights...), rollout
	}
}

// WithDNSDiscovery resolves the users route through localhost DNS.
func WithDNSDiscovery() Option { return func(settings *settings) { settings.discovery = true } }

// WithActiveHealthCheck enables active upstream probing for the scenario.
func WithActiveHealthCheck(check config.ActiveHealthCheck) Option {
	return func(settings *settings) { settings.activeHealth = check }
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
	gateway       *server.Server
	config        config.Config
	stopOnce      sync.Once
	stopErr       error
	users         []*upstream
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
	if cfg.users == 0 && !cfg.billing && (!cfg.grpc.Enabled || len(cfg.grpc.Routes) == 0) {
		t.Fatal("at least one upstream service is required")
	}

	routes := make([]config.Route, 0, 2)
	users := make([]*upstream, 0, cfg.users)
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
			users = append(users, upstream)
			upstreams = append(upstreams, upstream.URL())
		}
		routes = append(routes, config.Route{
			PathPrefix:  "/api/",
			Upstreams:   upstreams,
			StripPrefix: true,
			Auth:        cfg.routeAuth,
			Cache:       cfg.routeCache,
			Balancer:    cfg.balancer,
			Weights:     cfg.weights,
			Rollout:     cfg.rollout,
		})
		if cfg.discovery {
			parsed, err := url.Parse(upstreams[0])
			if err != nil {
				t.Fatal(err)
			}
			port, err := strconv.Atoi(parsed.Port())
			if err != nil {
				t.Fatal(err)
			}
			routes[len(routes)-1].Upstreams = nil
			routes[len(routes)-1].Discovery = &config.Discovery{Provider: "dns", Name: "127.0.0.1", Scheme: parsed.Scheme, Port: port}
		}
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
		Retry:             cfg.retry,
		CircuitBreaker:    cfg.circuit,
		RateLimit:         cfg.rateLimit,
		Auth:              cfg.auth,
		Cache:             cfg.cache,
		Routes:            routes,
		ActiveHealthCheck: cfg.activeHealth,
		GRPC:              cfg.grpc,
	}

	ctx, cancel := context.WithCancel(context.Background())
	environment := &Environment{
		t:             t,
		publicBaseURL: "http://" + publicAddress,
		adminBaseURL:  "http://" + adminAddress,
		client:        &http.Client{Timeout: 2 * time.Second},
		cancel:        cancel,
		runResult:     make(chan error, 1),
		config:        gatewayConfig,
		users:         users,
	}
	var serverOptions []server.Option
	if cfg.rateStore != nil {
		registry := ratelimit.NewRegistry()
		if err := registry.RegisterStore(cfg.rateLimit.DefaultBackend, cfg.rateStore); err != nil {
			t.Fatal(err)
		}
		serverOptions = append(serverOptions, server.WithRateLimitRegistry(registry))
	}
	if cfg.cacheStore != nil {
		serverOptions = append(serverOptions, server.WithCacheStore(cfg.cacheStore))
	}
	if cfg.telemetry != nil {
		serverOptions = append(serverOptions, server.WithTelemetry(cfg.telemetry))
	}
	gateway := server.New(gatewayConfig, slog.New(slog.NewTextHandler(io.Discard, nil)), serverOptions...)
	environment.gateway = gateway
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

// PublicAddress returns the gateway public listener address for protocol clients.
func (e *Environment) PublicAddress() string { return strings.TrimPrefix(e.publicBaseURL, "http://") }

// SetUserHealthStatus changes the status returned by a users upstream's /healthz endpoint.
func (e *Environment) SetUserHealthStatus(instance, status int) {
	if instance < 1 || instance > len(e.users) {
		e.t.Fatalf("users instance %d out of range", instance)
	}
	e.users[instance-1].SetHealthStatus(status)
}

// ReloadRoutes writes a temporary valid gateway configuration and applies it
// through the same transactional path used by production reloads.
func (e *Environment) ReloadRoutes(upstreams ...string) error {
	cfg := e.config
	cfg.Routes = []config.Route{{PathPrefix: "/api/", Upstreams: upstreams, StripPrefix: true}}
	data := []byte(fmt.Sprintf("server:\n  address: %s\nadmin:\n  address: %s\nroutes:\n  - path_prefix: /api/\n    upstreams: [%s]\n    strip_prefix: true\n", cfg.Server.Address, cfg.Admin.Address, strings.Join(upstreams, ", ")))
	path := filepath.Join(e.t.TempDir(), "gateway.yaml")
	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	return e.gateway.Reload(context.Background(), path)
}

// ReloadBroken attempts to apply malformed YAML and returns the reload error.
func (e *Environment) ReloadBroken() error {
	path := filepath.Join(e.t.TempDir(), "gateway.yaml")
	if err := os.WriteFile(path, []byte("routes: ["), 0600); err != nil {
		return err
	}
	return e.gateway.Reload(context.Background(), path)
}

// ReloadDiscoveryBroken attempts to publish a snapshot whose provider cannot resolve.
func (e *Environment) ReloadDiscoveryBroken() error {
	path := filepath.Join(e.t.TempDir(), "gateway.yaml")
	contents := fmt.Sprintf("server:\n  address: %s\nadmin:\n  address: %s\nroutes:\n  - path_prefix: /api/\n    discovery:\n      provider: dns\n      name: does-not-exist.invalid\n      scheme: http\n      port: 8081\n      interval: 1s\n      grace: 1s\n    strip_prefix: true\n", e.config.Server.Address, e.config.Admin.Address)
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		return err
	}
	return e.gateway.Reload(context.Background(), path)
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
