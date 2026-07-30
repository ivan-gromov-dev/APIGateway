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

	"github.com/Djunichi/APIGateway/internal/balancer"
	"github.com/Djunichi/APIGateway/internal/circuitbreaker"
	"github.com/Djunichi/APIGateway/internal/config"
	"github.com/Djunichi/APIGateway/internal/upstream"
)

var errNoAvailableUpstream = errors.New("no available upstream")

func Handler(routes []config.Route, retry config.Retry, circuit config.CircuitBreaker, logger *slog.Logger) (http.Handler, error) {
	return HandlerWithBalancer(routes, retry, circuit, logger, func(upstreams []*upstream.Target) (balancer.Balancer, error) {
		return balancer.NewRoundRobin(upstreams)
	})
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
			Transport: newRetryTransport(http.DefaultTransport, routeBalancer, retry, circuit.FailureStatuses),
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
		mux.Handle(route.PathPrefix, handler)
	}
	return mux, nil
}
