// Package proxy builds reverse proxies that forward configured routes to upstream services.
package proxy

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/Djunichi/APIGateway/internal/balancer"
	"github.com/Djunichi/APIGateway/internal/config"
)

func Handler(routes []config.Route, logger *slog.Logger) (http.Handler, error) {
	return HandlerWithBalancer(routes, logger, func(upstreams []*url.URL) (balancer.Balancer, error) {
		return balancer.NewRoundRobin(upstreams)
	})
}

// HandlerWithBalancer builds a proxy handler using a separate balancer for
// every configured route.
func HandlerWithBalancer(
	routes []config.Route,
	logger *slog.Logger,
	newBalancer balancer.Factory,
) (http.Handler, error) {
	if newBalancer == nil {
		return nil, fmt.Errorf("balancer factory is required")
	}
	mux := http.NewServeMux()
	for _, route := range routes {
		targets := make([]*url.URL, 0, len(route.Upstreams))
		for _, rawTarget := range route.Upstreams {
			target, err := url.Parse(rawTarget)
			if err != nil {
				return nil, fmt.Errorf("parse upstream %q: %w", rawTarget, err)
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
				request.SetURL(routeBalancer.Next())
				request.SetXForwarded()
			},
		}
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			logger.Error("upstream request failed", "error", err, "request_id", r.Header.Get("X-Request-ID"))
			http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		}
		var handler http.Handler = proxy
		if route.StripPrefix {
			handler = http.StripPrefix(strings.TrimSuffix(route.PathPrefix, "/"), handler)
		}
		mux.Handle(route.PathPrefix, handler)
	}
	return mux, nil
}
