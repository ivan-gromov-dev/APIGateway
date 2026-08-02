// Package server constructs and coordinates the gateway's public and administrative HTTP servers.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Djunichi/APIGateway/internal/auth"
	"github.com/Djunichi/APIGateway/internal/cache"
	cacheredis "github.com/Djunichi/APIGateway/internal/cachestore/redis"
	"github.com/Djunichi/APIGateway/internal/config"
	"github.com/Djunichi/APIGateway/internal/healthcheck"
	"github.com/Djunichi/APIGateway/internal/metrics"
	"github.com/Djunichi/APIGateway/internal/middleware"
	"github.com/Djunichi/APIGateway/internal/proxy"
	"github.com/Djunichi/APIGateway/internal/ratelimit"
	redisstore "github.com/Djunichi/APIGateway/internal/ratestore/redis"
	"github.com/Djunichi/APIGateway/internal/telemetry"
	"github.com/Djunichi/APIGateway/internal/upstream"
	"go.opentelemetry.io/otel/trace"
)

type Server struct {
	cfg          config.Config
	logger       *slog.Logger
	ready        atomic.Bool
	rateRegistry *ratelimit.Registry
	cacheStore   cache.Store
	telemetry    *telemetry.Runtime
	reloadMu     sync.Mutex
	runtime      *runtimeState
	dynamic      *dynamicHandler
}

type runtimeState struct {
	telemetry  *telemetry.Runtime
	collector  *metrics.Collector
	registry   *ratelimit.Registry
	cacheStore cache.Store
	transport  func(time.Duration) *http.Client
	tracer     trace.Tracer
}

type dynamicHandler struct{ value atomic.Value }

func (h *dynamicHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.value.Load().(http.Handler).ServeHTTP(w, r)
}
func (h *dynamicHandler) Store(next http.Handler) { h.value.Store(next) }

type Option func(*Server)

func WithRateLimitRegistry(registry *ratelimit.Registry) Option {
	return func(server *Server) { server.rateRegistry = registry }
}

func WithCacheStore(store cache.Store) Option {
	return func(server *Server) { server.cacheStore = store }
}

func WithTelemetry(runtime *telemetry.Runtime) Option {
	return func(server *Server) { server.telemetry = runtime }
}

func New(cfg config.Config, logger *slog.Logger, options ...Option) *Server {
	server := &Server{cfg: cfg, logger: logger}
	for _, option := range options {
		option(server)
	}
	return server
}

func (s *Server) Run(ctx context.Context) (resultErr error) {
	runtime, owned, err := s.prepareTelemetry(ctx)
	if err != nil {
		return err
	}
	if owned {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), s.cfg.Telemetry.Tracing.ShutdownTimeout)
			defer cancel()
			resultErr = errors.Join(resultErr, runtime.Shutdown(shutdownCtx))
		}()
	}
	collector := &metrics.Collector{}
	registry, closeRateLimit, err := s.prepareRateLimit(ctx)
	if err != nil {
		return err
	}
	defer closeRateLimit()
	verifiers, err := s.prepareAuthWithClient(ctx, runtime.HTTPClient)
	if err != nil {
		return err
	}
	cacheStore, closeCache, err := s.prepareCache(ctx)
	if err != nil {
		return err
	}
	defer closeCache()
	s.runtime = &runtimeState{telemetry: runtime, collector: collector, registry: registry, cacheStore: cacheStore, transport: runtime.HTTPClient, tracer: runtime.Tracer()}
	var targets []*upstream.Target
	handler, err := s.buildHandler(s.cfg, runtime, verifiers, &targets)
	if err != nil {
		return err
	}
	dynamic := &dynamicHandler{}
	dynamic.Store(handler)
	s.dynamic = dynamic
	checkCtx, stopChecks := context.WithCancel(ctx)
	defer stopChecks()
	go healthcheck.New(targets, s.cfg.ActiveHealthCheck, runtime.HTTPClient(s.cfg.ActiveHealthCheck.Timeout)).Run(checkCtx)

	app := httpServer(s.cfg.Server, dynamic)
	admin := httpServer(s.cfg.Admin, adminHandler(collector, &s.ready))

	appListener, err := net.Listen("tcp", app.Addr)
	if err != nil {
		return fmt.Errorf("listen on public address %s: %w", app.Addr, err)
	}
	adminListener, err := net.Listen("tcp", admin.Addr)
	if err != nil {
		_ = appListener.Close()
		return fmt.Errorf("listen on admin address %s: %w", admin.Addr, err)
	}

	results := make(chan error, 2)
	s.ready.Store(true)
	go serve("public", app, appListener, results)
	go serve("admin", admin, adminListener, results)

	var runErr error
	completed := 0
	select {
	case <-ctx.Done():
	case runErr = <-results:
		completed = 1
	}

	s.ready.Store(false)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.cfg.Server.ShutdownTimeout)
	defer cancel()
	shutdownErr := shutdownAll(shutdownCtx, app, admin)

	for completed < 2 {
		if err := <-results; err != nil && runErr == nil {
			runErr = err
		}
		completed++
	}

	return errors.Join(runErr, shutdownErr)
}

// Reload validates and prepares a new configuration before atomically making it
// visible to new requests. Listener addresses cannot change during a reload.
func (s *Server) Reload(ctx context.Context, path string) error {
	s.reloadMu.Lock()
	defer s.reloadMu.Unlock()
	if s.dynamic == nil || s.runtime == nil {
		return errors.New("gateway is not running")
	}
	cfg, err := config.Load(path)
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	if cfg.Server.Address != s.cfg.Server.Address || cfg.Admin.Address != s.cfg.Admin.Address {
		return errors.New("reload cannot change listener addresses")
	}
	runtime := s.runtime
	verifiers, err := s.prepareAuthWithClient(ctx, runtime.transport)
	if err != nil {
		return err
	}
	var targets []*upstream.Target
	handler, err := s.buildHandler(cfg, runtime.telemetry, verifiers, &targets)
	if err != nil {
		return err
	}
	if s.dynamic == nil {
		return errors.New("gateway is not running")
	}
	s.dynamic.Store(handler)
	s.cfg = cfg
	return nil
}

func (s *Server) buildHandler(cfg config.Config, runtime *telemetry.Runtime, verifiers map[string]*auth.Verifier, targets *[]*upstream.Target) (http.Handler, error) {
	proxyHandler, err := proxy.HandlerWithObservabilityAndTargets(cfg.Routes, cfg.Retry, cfg.CircuitBreaker, cfg.RateLimit, s.runtime.registry, verifiers, cfg.Cache, s.runtime.cacheStore, s.logger, runtime.Transport(http.DefaultTransport), runtime.Tracer(), s.runtime.collector, targets)
	if err != nil {
		return nil, err
	}
	global, err := middleware.BuildRateLimitWithMetrics(cfg.RateLimit.Rules, cfg.RateLimit, s.runtime.registry, s.runtime.collector, "global")
	if err != nil {
		return nil, fmt.Errorf("build global rate limit: %w", err)
	}
	return runtime.Handler(middleware.Chain(proxyHandler, middleware.RequestID, middleware.Logging(s.logger, s.runtime.collector), middleware.Recovery(s.logger), middleware.Timeout(cfg.Middleware.RequestTimeout), middleware.CORS(cfg.Middleware.CORS), global)), nil
}

func (s *Server) prepareTelemetry(ctx context.Context) (*telemetry.Runtime, bool, error) {
	if s.telemetry != nil {
		return s.telemetry, false, nil
	}
	cfg := s.cfg.Telemetry.Tracing
	runtime, err := telemetry.New(ctx, telemetry.Config{Enabled: cfg.Enabled, ServiceName: cfg.ServiceName,
		Endpoint: cfg.Endpoint, Insecure: cfg.Insecure, SampleRatio: cfg.SampleRatio})
	if err != nil {
		return nil, false, fmt.Errorf("prepare telemetry: %w", err)
	}
	return runtime, true, nil
}

func (s *Server) prepareAuth(ctx context.Context) (map[string]*auth.Verifier, error) {
	return s.prepareAuthWithClient(ctx, func(timeout time.Duration) *http.Client { return &http.Client{Timeout: timeout} })
}

func (s *Server) prepareAuthWithClient(ctx context.Context, client func(time.Duration) *http.Client) (map[string]*auth.Verifier, error) {
	result := make(map[string]*auth.Verifier, len(s.cfg.Auth.Providers))
	for name, provider := range s.cfg.Auth.Providers {
		verifier, err := auth.New(ctx, auth.Config{
			JWKSURL: provider.JWKSURL, Issuer: provider.Issuer, Audience: provider.Audience,
			Algorithms: provider.Algorithms, ClockSkew: provider.ClockSkew, HTTPTimeout: provider.HTTPTimeout,
			HTTPClient: client(provider.HTTPTimeout),
		})
		if err != nil {
			return nil, fmt.Errorf("prepare auth provider %q: %w", name, err)
		}
		result[name] = verifier
	}
	return result, nil
}

func (s *Server) prepareCache(ctx context.Context) (cache.Store, func(), error) {
	if !s.cfg.Cache.Enabled {
		return nil, func() {}, nil
	}
	if s.cacheStore != nil {
		return s.cacheStore, func() {}, nil
	}
	store := cacheredis.New(cacheredis.Config{
		Address: s.cfg.Cache.Redis.Address, Username: s.cfg.Cache.Redis.Username,
		Password: s.cfg.Cache.Redis.Password, Database: s.cfg.Cache.Redis.Database,
		KeyPrefix: s.cfg.Cache.Redis.KeyPrefix,
	})
	pingCtx, cancel := context.WithTimeout(ctx, s.cfg.Cache.OperationTimeout)
	err := store.Ping(pingCtx)
	cancel()
	if err != nil {
		_ = store.Close()
		return nil, func() {}, fmt.Errorf("connect cache Redis: %w", err)
	}
	return store, func() { _ = store.Close() }, nil
}

func (s *Server) prepareRateLimit(ctx context.Context) (*ratelimit.Registry, func(), error) {
	if !s.cfg.RateLimit.Enabled {
		return ratelimit.NewRegistry(), func() {}, nil
	}
	registry := s.rateRegistry
	closeStore := func() {}
	if registry == nil {
		if s.cfg.RateLimit.DefaultBackend != "redis" {
			return nil, func() {}, fmt.Errorf(
				"rate limit backend %q requires external registration",
				s.cfg.RateLimit.DefaultBackend,
			)
		}
		registry = ratelimit.NewRegistry()
		store := redisstore.New(redisstore.Config{
			Address: s.cfg.RateLimit.Redis.Address, Username: s.cfg.RateLimit.Redis.Username,
			Password: s.cfg.RateLimit.Redis.Password, Database: s.cfg.RateLimit.Redis.Database,
			KeyPrefix: s.cfg.RateLimit.Redis.KeyPrefix,
		})
		pingCtx, cancel := context.WithTimeout(ctx, s.cfg.RateLimit.OperationTimeout)
		err := store.Ping(pingCtx)
		cancel()
		if err != nil {
			_ = store.Close()
			return nil, func() {}, fmt.Errorf("connect rate limit Redis: %w", err)
		}
		if err := registry.RegisterStore(s.cfg.RateLimit.DefaultBackend, store); err != nil {
			_ = store.Close()
			return nil, func() {}, err
		}
		closeStore = func() { _ = store.Close() }
	}
	if err := registerBuiltins(registry); err != nil {
		closeStore()
		return nil, func() {}, err
	}
	registry.Freeze()
	return registry, closeStore, nil
}

func registerBuiltins(registry *ratelimit.Registry) error {
	if _, ok := registry.Key("global"); !ok {
		if err := registry.RegisterKey("global", middleware.GlobalKey); err != nil {
			return err
		}
	}
	if _, ok := registry.Key("client_ip"); !ok {
		if err := registry.RegisterKey("client_ip", middleware.ClientIPKey); err != nil {
			return err
		}
	}
	if _, ok := registry.Filter("all"); !ok {
		if err := registry.RegisterFilter("all", middleware.AllRequests); err != nil {
			return err
		}
	}
	if _, ok := registry.Filter("writes_only"); !ok {
		if err := registry.RegisterFilter("writes_only", middleware.WritesOnly); err != nil {
			return err
		}
	}
	return nil
}

func httpServer(cfg config.HTTPServer, handler http.Handler) *http.Server {
	return &http.Server{
		Addr: cfg.Address, Handler: handler,
		ReadTimeout: cfg.ReadTimeout, WriteTimeout: cfg.WriteTimeout, IdleTimeout: cfg.IdleTimeout,
	}
}

func serve(name string, server *http.Server, listener net.Listener, results chan<- error) {
	err := server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	if err != nil {
		err = fmt.Errorf("%s server: %w", name, err)
	}
	results <- err
}

func shutdownAll(ctx context.Context, servers ...*http.Server) error {
	errs := make([]error, len(servers))
	var wg sync.WaitGroup
	wg.Add(len(servers))
	for i, server := range servers {
		go func() {
			defer wg.Done()
			errs[i] = server.Shutdown(ctx)
		}()
	}
	wg.Wait()
	return errors.Join(errs...)
}

func adminHandler(collector *metrics.Collector, ready *atomic.Bool) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", ok)
	mux.HandleFunc("GET /readyz", readinessHandler(ready))
	mux.Handle("GET /metrics", collector)
	mux.HandleFunc("GET /debug/pprof/", pprof.Index)
	mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)
	return mux
}

func readinessHandler(ready *atomic.Bool) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if !ready.Load() {
			writeStatus(w, http.StatusServiceUnavailable, "not_ready")
			return
		}
		writeStatus(w, http.StatusOK, "ready")
	}
}

func ok(w http.ResponseWriter, _ *http.Request) {
	writeStatus(w, http.StatusOK, "ok")
}

func writeStatus(w http.ResponseWriter, status int, value string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"status":%q}`, value)
}
