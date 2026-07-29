package proxy

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/Djunichi/APIGateway/internal/config"
)

func Handler(routes []config.Route, logger *slog.Logger) (http.Handler, error) {
	mux := http.NewServeMux()
	for _, route := range routes {
		target, err := url.Parse(route.Upstream)
		if err != nil {
			return nil, fmt.Errorf("parse upstream %q: %w", route.Upstream, err)
		}
		proxy := httputil.NewSingleHostReverseProxy(target)
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			logger.Error("upstream request failed", "upstream", target.String(), "error", err, "request_id", r.Header.Get("X-Request-ID"))
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
