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
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Djunichi/APIGateway/internal/auth"
	"github.com/Djunichi/APIGateway/internal/cache"
	cacheredis "github.com/Djunichi/APIGateway/internal/cachestore/redis"
	"github.com/Djunichi/APIGateway/internal/config"
	"github.com/Djunichi/APIGateway/internal/discovery"
	"github.com/Djunichi/APIGateway/internal/grpcproxy"
	"github.com/Djunichi/APIGateway/internal/metrics"
	"github.com/Djunichi/APIGateway/internal/middleware"
	"github.com/Djunichi/APIGateway/internal/proxy"
	"github.com/Djunichi/APIGateway/internal/ratelimit"
	redisstore "github.com/Djunichi/APIGateway/internal/ratestore/redis"
	"github.com/Djunichi/APIGateway/internal/telemetry"
	"github.com/Djunichi/APIGateway/internal/upstream"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

type Server struct {
	cfg              config.Config
	logger           *slog.Logger
	ready            atomic.Bool
	discoveryOK      atomic.Bool
	rateRegistry     *ratelimit.Registry
	cacheStore       cache.Store
	telemetry        *telemetry.Runtime
	reloadMu         sync.Mutex
	runtime          *runtimeState
	dynamic          *dynamicHandler
	providers        discovery.Registry
	targets          map[string]*upstream.Target
	discoveryCancel  context.CancelFunc
	healthCancel     context.CancelFunc
	runCtx           context.Context
	discoveryExpired map[int]bool
}

type runtimeState struct {
	telemetry  *telemetry.Runtime
	collector  *metrics.Collector
	registry   *ratelimit.Registry
	cacheStore cache.Store
	transport  func(time.Duration) *http.Client
	tracer     trace.Tracer
}

type dynamicHandler struct{ value, expired atomic.Value }

func (h *dynamicHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if prefixes, ok := h.expired.Load().(map[string]bool); ok {
		for prefix := range prefixes {
			if strings.HasPrefix(r.URL.Path, prefix) {
				http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
				return
			}
		}
	}
	h.value.Load().(http.Handler).ServeHTTP(w, r)
}
func (h *dynamicHandler) Store(next http.Handler)               { h.value.Store(next) }
func (h *dynamicHandler) StoreExpired(prefixes map[string]bool) { h.expired.Store(prefixes) }

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

// WithDiscoveryRegistry replaces built-in providers for deterministic tests or embedding.
func WithDiscoveryRegistry(registry discovery.Registry) Option {
	return func(server *Server) { server.providers = registry }
}

func New(cfg config.Config, logger *slog.Logger, options ...Option) *Server {
	server := &Server{cfg: cfg, logger: logger, providers: discovery.DefaultRegistry()}
	server.discoveryOK.Store(true)
	for _, option := range options {
		option(server)
	}
	return server
}

func (s *Server) Run(ctx context.Context) (resultErr error) {
	s.runCtx = ctx
	shutdownTimeout := s.cfg.Server.ShutdownTimeout
	telemetryShutdownTimeout := s.cfg.Telemetry.Tracing.ShutdownTimeout
	runtime, owned, err := s.prepareTelemetry(ctx)
	if err != nil {
		return err
	}
	if owned {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), telemetryShutdownTimeout)
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
	resolved, err := s.resolveDiscovery(ctx, s.cfg)
	if err != nil {
		return err
	}
	observeResolvedDiscovery(collector, resolved)
	handler, pool, err := s.buildHandlerWithPool(resolved, runtime, verifiers, nil)
	if err != nil {
		return err
	}
	dynamic := &dynamicHandler{}
	dynamic.Store(handler)
	dynamic.StoreExpired(map[string]bool{})
	s.dynamic = dynamic
	s.cfg = resolved
	s.targets = pool
	s.startHealthChecks(ctx)
	defer s.stopRuntimeWorkers()

	app := publicHTTPServer(resolved, dynamic)
	admin := httpServer(resolved.Admin, adminHandler(collector, &s.ready, &s.discoveryOK))

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
	s.startDiscovery(ctx, verifiers)
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
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
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
	if cfg.GRPC.Enabled != s.cfg.GRPC.Enabled {
		return errors.New("reload cannot change grpc.enabled")
	}
	runtime := s.runtime
	cfg, err = s.resolveDiscovery(ctx, cfg)
	if err != nil {
		return fmt.Errorf("resolve discovery: %w", err)
	}
	observeResolvedDiscovery(runtime.collector, cfg)
	verifiers, err := s.prepareAuthWithClient(ctx, runtime.transport)
	if err != nil {
		return err
	}
	handler, pool, err := s.buildHandlerWithPool(cfg, runtime.telemetry, verifiers, s.targets)
	if err != nil {
		return err
	}
	if s.dynamic == nil {
		return errors.New("gateway is not running")
	}
	s.dynamic.Store(handler)
	s.cfg = cfg
	s.targets = pool
	s.startHealthChecks(ctx)
	s.startDiscovery(ctx, verifiers)
	s.discoveryOK.Store(true)
	return nil
}

func observeResolvedDiscovery(collector *metrics.Collector, cfg config.Config) {
	for _, route := range cfg.Routes {
		if route.Discovery != nil {
			collector.ObserveDiscovery(route.PathPrefix, "success", len(route.Upstreams), 0)
		}
	}
}

func (s *Server) resolveDiscovery(ctx context.Context, cfg config.Config) (config.Config, error) {
	for i := range cfg.Routes {
		route := &cfg.Routes[i]
		if route.Discovery == nil {
			continue
		}
		upstreams, err := s.providers.Resolve(ctx, *route.Discovery)
		if err != nil {
			return config.Config{}, fmt.Errorf("route %d: %w", i, err)
		}
		route.Upstreams = upstreams
		if len(route.Weights) != 0 && len(route.Weights) != len(route.Upstreams) {
			return config.Config{}, fmt.Errorf("route %d: weights must match discovered upstreams", i)
		}
	}
	return cfg, nil
}

// resolveDiscovery retains the package-level helper used by focused tests.
func resolveDiscovery(ctx context.Context, cfg config.Config) (config.Config, error) {
	return (&Server{providers: discovery.DefaultRegistry()}).resolveDiscovery(ctx, cfg)
}

func (s *Server) buildHandlerWithPool(cfg config.Config, runtime *telemetry.Runtime, verifiers map[string]*auth.Verifier, previous map[string]*upstream.Target) (http.Handler, map[string]*upstream.Target, error) {
	proxyHandler, pool, err := proxy.HandlerWithObservabilityAndTargetPool(cfg.Routes, cfg.Retry, cfg.CircuitBreaker, cfg.RateLimit, s.runtime.registry, verifiers, cfg.Cache, s.runtime.cacheStore, s.logger, runtime.Transport(http.DefaultTransport), runtime.Tracer(), s.runtime.collector, previous)
	if err != nil {
		return nil, nil, err
	}
	handler, err := s.composeHandler(cfg, runtime, verifiers, proxyHandler)
	return handler, pool, err
}

func (s *Server) buildHandler(cfg config.Config, runtime *telemetry.Runtime, verifiers map[string]*auth.Verifier, targets *[]*upstream.Target) (http.Handler, error) {
	proxyHandler, err := proxy.HandlerWithObservabilityAndTargets(cfg.Routes, cfg.Retry, cfg.CircuitBreaker, cfg.RateLimit, s.runtime.registry, verifiers, cfg.Cache, s.runtime.cacheStore, s.logger, runtime.Transport(http.DefaultTransport), runtime.Tracer(), s.runtime.collector, targets)
	if err != nil {
		return nil, err
	}
	return s.composeHandler(cfg, runtime, verifiers, proxyHandler)
}

func (s *Server) composeHandler(cfg config.Config, runtime *telemetry.Runtime, verifiers map[string]*auth.Verifier, proxyHandler http.Handler) (http.Handler, error) {
	global, err := middleware.BuildRateLimitWithMetrics(cfg.RateLimit.Rules, cfg.RateLimit, s.runtime.registry, s.runtime.collector, "global")
	if err != nil {
		return nil, fmt.Errorf("build global rate limit: %w", err)
	}
	httpHandler := runtime.Handler(middleware.Chain(proxyHandler, middleware.RequestID, middleware.Logging(s.logger, s.runtime.collector), middleware.Recovery(s.logger), middleware.Timeout(cfg.Middleware.RequestTimeout), middleware.CORS(cfg.Middleware.CORS), global))
	if !cfg.GRPC.Enabled {
		return httpHandler, nil
	}
	grpcHandler, err := grpcproxy.New(cfg.GRPC, s.runtime.collector)
	if err != nil {
		return nil, fmt.Errorf("build gRPC proxy: %w", err)
	}
	grpcHandler = runtime.Handler(middleware.Chain(grpcHandler, middleware.RequestID, middleware.Logging(s.logger, s.runtime.collector), middleware.Recovery(s.logger)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 && strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/grpc") {
			grpcHandler.ServeHTTP(w, r)
			return
		}
		httpHandler.ServeHTTP(w, r)
	}), nil
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

func publicHTTPServer(cfg config.Config, handler http.Handler) *http.Server {
	publicHandler := handler
	publicConfig := cfg.Server
	if cfg.GRPC.Enabled {
		publicHandler = h2c.NewHandler(publicHandler, &http2.Server{})
		// Whole-request read/write deadlines are incompatible with long-lived
		// client/server streams. Per-route/client deadlines still bound RPCs.
		publicConfig.ReadTimeout, publicConfig.WriteTimeout = 0, 0
	}
	return httpServer(publicConfig, publicHandler)
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

func adminHandler(collector *metrics.Collector, ready *atomic.Bool, discoveryState ...*atomic.Bool) http.Handler {
	discoveryOK := &atomic.Bool{}
	discoveryOK.Store(true)
	if len(discoveryState) > 0 && discoveryState[0] != nil {
		discoveryOK = discoveryState[0]
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", ok)
	mux.HandleFunc("GET /readyz", readinessHandler(ready, discoveryOK))
	mux.Handle("GET /metrics", collector)
	mux.HandleFunc("GET /debug/pprof/", pprof.Index)
	mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)
	return mux
}

func readinessHandler(ready, discoveryOK *atomic.Bool) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if !ready.Load() || !discoveryOK.Load() {
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
