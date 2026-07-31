// Package proxy builds reverse proxies that forward configured routes to upstream services.
package proxy

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/Djunichi/APIGateway/internal/auth"
	"github.com/Djunichi/APIGateway/internal/balancer"
	"github.com/Djunichi/APIGateway/internal/cache"
	"github.com/Djunichi/APIGateway/internal/circuitbreaker"
	"github.com/Djunichi/APIGateway/internal/config"
	"github.com/Djunichi/APIGateway/internal/metrics"
	"github.com/Djunichi/APIGateway/internal/middleware"
	"github.com/Djunichi/APIGateway/internal/ratelimit"
	"github.com/Djunichi/APIGateway/internal/upstream"
	"go.opentelemetry.io/otel/trace"
)

var errNoAvailableUpstream = errors.New("no available upstream")

func Handler(routes []config.Route, retry config.Retry, circuit config.CircuitBreaker, logger *slog.Logger) (http.Handler, error) {
	return handlerWithBalancer(routes, retry, circuit, config.RateLimit{}, nil, logger, func(upstreams []*upstream.Target) (balancer.Balancer, error) {
		return balancer.NewRoundRobin(upstreams)
	})
}

func HandlerWithRateLimit(routes []config.Route, retry config.Retry, circuit config.CircuitBreaker, rate config.RateLimit, registry *ratelimit.Registry, logger *slog.Logger) (http.Handler, error) {
	return HandlerWithFeatures(routes, retry, circuit, rate, registry, nil, config.Cache{}, nil, logger)
}

func HandlerWithFeatures(routes []config.Route, retry config.Retry, circuit config.CircuitBreaker,
	rate config.RateLimit, registry *ratelimit.Registry, verifiers map[string]*auth.Verifier,
	cacheConfig config.Cache, cacheStore cache.Store, logger *slog.Logger) (http.Handler, error) {
	return handlerWithFeatures(routes, retry, circuit, rate, registry, verifiers, cacheConfig, cacheStore, logger, func(upstreams []*upstream.Target) (balancer.Balancer, error) {
		return balancer.NewRoundRobin(upstreams)
	})
}

func HandlerWithTelemetry(routes []config.Route, retry config.Retry, circuit config.CircuitBreaker,
	rate config.RateLimit, registry *ratelimit.Registry, verifiers map[string]*auth.Verifier,
	cacheConfig config.Cache, cacheStore cache.Store, logger *slog.Logger,
	base http.RoundTripper, tracer trace.Tracer) (http.Handler, error) {
	return handlerWithFeaturesAndTelemetry(routes, retry, circuit, rate, registry, verifiers,
		cacheConfig, cacheStore, logger, func(upstreams []*upstream.Target) (balancer.Balancer, error) {
			return balancer.NewRoundRobin(upstreams)
		}, base, tracer)
}

func HandlerWithObservability(routes []config.Route, retry config.Retry, circuit config.CircuitBreaker,
	rate config.RateLimit, registry *ratelimit.Registry, verifiers map[string]*auth.Verifier,
	cacheConfig config.Cache, cacheStore cache.Store, logger *slog.Logger,
	base http.RoundTripper, tracer trace.Tracer, collector *metrics.Collector) (http.Handler, error) {
	return handlerWithFeaturesAndObservability(routes, retry, circuit, rate, registry, verifiers,
		cacheConfig, cacheStore, logger, func(upstreams []*upstream.Target) (balancer.Balancer, error) {
			return balancer.NewRoundRobin(upstreams)
		}, base, tracer, collector)
}

func normalizedCircuitConfig(circuit config.CircuitBreaker) config.CircuitBreaker {
	if circuit.FailureThreshold <= 0 {
		circuit.FailureThreshold = 5
	}
	if circuit.OpenTimeout <= 0 {
		circuit.OpenTimeout = 30 * time.Second
	}
	if len(circuit.FailureStatuses) == 0 {
		circuit.FailureStatuses = []int{502, 503, 504}
	}
	return circuit
}

// HandlerWithBalancer builds a proxy handler using a separate balancer for
// every configured route.
func HandlerWithBalancer(
	routes []config.Route,
	retry config.Retry,
	circuit config.CircuitBreaker,
	logger *slog.Logger,
	newBalancer balancer.Factory,
) (http.Handler, error) {
	return handlerWithFeatures(routes, retry, circuit, config.RateLimit{}, nil, nil, config.Cache{}, nil, logger, newBalancer)
}

func handlerWithBalancer(
	routes []config.Route, retry config.Retry, circuit config.CircuitBreaker,
	rate config.RateLimit, registry *ratelimit.Registry, logger *slog.Logger,
	newBalancer balancer.Factory,
) (http.Handler, error) {
	return handlerWithFeatures(routes, retry, circuit, rate, registry, nil, config.Cache{}, nil, logger, newBalancer)
}

func handlerWithFeatures(
	routes []config.Route, retry config.Retry, circuit config.CircuitBreaker,
	rate config.RateLimit, registry *ratelimit.Registry, verifiers map[string]*auth.Verifier,
	cacheConfig config.Cache, cacheStore cache.Store, logger *slog.Logger,
	newBalancer balancer.Factory,
) (http.Handler, error) {
	return handlerWithFeaturesAndTelemetry(routes, retry, circuit, rate, registry, verifiers,
		cacheConfig, cacheStore, logger, newBalancer, http.DefaultTransport, nil)
}

func handlerWithFeaturesAndTelemetry(
	routes []config.Route, retry config.Retry, circuit config.CircuitBreaker,
	rate config.RateLimit, registry *ratelimit.Registry, verifiers map[string]*auth.Verifier,
	cacheConfig config.Cache, cacheStore cache.Store, logger *slog.Logger,
	newBalancer balancer.Factory, base http.RoundTripper, tracer trace.Tracer,
) (http.Handler, error) {
	return handlerWithFeaturesAndObservability(routes, retry, circuit, rate, registry, verifiers,
		cacheConfig, cacheStore, logger, newBalancer, base, tracer, nil)
}

func handlerWithFeaturesAndObservability(
	routes []config.Route, retry config.Retry, circuit config.CircuitBreaker,
	rate config.RateLimit, registry *ratelimit.Registry, verifiers map[string]*auth.Verifier,
	cacheConfig config.Cache, cacheStore cache.Store, logger *slog.Logger,
	newBalancer balancer.Factory, base http.RoundTripper, tracer trace.Tracer, collector *metrics.Collector,
) (http.Handler, error) {
	if newBalancer == nil {
		return nil, fmt.Errorf("balancer factory is required")
	}
	circuit = normalizedCircuitConfig(circuit)
	mux := http.NewServeMux()
	for _, route := range routes {
		targets := make([]*upstream.Target, 0, len(route.Upstreams))
		for _, rawTarget := range route.Upstreams {
			targetURL, err := url.Parse(rawTarget)
			if err != nil {
				return nil, fmt.Errorf("parse upstream %q: %w", rawTarget, err)
			}
			breaker, err := circuitbreaker.New(circuitbreaker.Config{
				FailureThreshold: circuit.FailureThreshold,
				OpenTimeout:      circuit.OpenTimeout,
			})
			if err != nil {
				return nil, fmt.Errorf("create circuit breaker for upstream %q: %w", rawTarget, err)
			}
			target, err := upstream.NewTarget(targetURL, breaker)
			if err != nil {
				return nil, fmt.Errorf("create upstream %q: %w", rawTarget, err)
			}
			targets = append(targets, target)
		}
		routeBalancer, err := newBalancer(targets)
		if err != nil {
			return nil, fmt.Errorf("create balancer for route %q: %w", route.PathPrefix, err)
		}
		if routeBalancer == nil {
			return nil, fmt.Errorf("create balancer for route %q: factory returned nil", route.PathPrefix)
		}
		proxy := &httputil.ReverseProxy{
			Rewrite: func(request *httputil.ProxyRequest) {
				request.SetXForwarded()
			},
			Transport: newRetryTransportWithObservability(base, routeBalancer, retry, circuit.FailureStatuses, tracer, collector, route.PathPrefix),
		}
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			status := http.StatusBadGateway
			if errors.Is(err, errNoAvailableUpstream) {
				status = http.StatusServiceUnavailable
			}
			logger.Error("upstream request failed", "error", err, "request_id", r.Header.Get("X-Request-ID"))
			http.Error(w, http.StatusText(status), status)
		}
		var handler http.Handler = proxy
		if route.StripPrefix {
			handler = http.StripPrefix(strings.TrimSuffix(route.PathPrefix, "/"), handler)
		}
		if route.Cache.Enabled {
			if cacheStore == nil {
				return nil, fmt.Errorf("cache store is required for route %q", route.PathPrefix)
			}
			handler = middleware.ResponseCacheWithMetrics(cacheStore, cacheConfig, route.Cache, route.PathPrefix, collector)(handler)
		}
		if rate.Enabled && len(route.RateLimits) > 0 {
			rateMiddleware, err := middleware.BuildRateLimitWithMetrics(route.RateLimits, rate, registry, collector, route.PathPrefix)
			if err != nil {
				return nil, fmt.Errorf("build rate limit for route %q: %w", route.PathPrefix, err)
			}
			handler = rateMiddleware(handler)
		}
		if route.Auth.Required {
			verifier := verifiers[route.Auth.Provider]
			if verifier == nil {
				return nil, fmt.Errorf("auth provider %q is unavailable", route.Auth.Provider)
			}
			handler = middleware.AuthenticationWithMetrics(verifier, route.Auth.RequiredScopes, collector, route.PathPrefix)(handler)
		}
		handler = middleware.RouteMetrics(collector, route.PathPrefix)(handler)
		mux.Handle(route.PathPrefix, handler)
	}
	return mux, nil
}
