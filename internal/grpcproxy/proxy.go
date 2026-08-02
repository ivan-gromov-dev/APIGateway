// Package grpcproxy provides transparent, streaming-safe gRPC reverse proxying.
package grpcproxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/Djunichi/APIGateway/internal/config"
	"github.com/Djunichi/APIGateway/internal/metrics"
	"golang.org/x/net/http2"
)

type route struct {
	prefix  string
	target  *url.URL
	timeout time.Duration
	proxy   *httputil.ReverseProxy
}

// New builds an immutable longest-prefix gRPC router. The HTTP/2 transport
// streams request and response bodies and exposes upstream trailers unchanged.
func New(cfg config.GRPC, collector *metrics.Collector) (http.Handler, error) {
	routes := make([]route, 0, len(cfg.Routes))
	for _, configured := range cfg.Routes {
		target, err := url.Parse(configured.Upstream)
		if err != nil {
			return nil, fmt.Errorf("parse upstream for %s: %w", configured.PathPrefix, err)
		}
		transport := &http2.Transport{}
		if target.Scheme == "http" {
			transport.AllowHTTP = true
			transport.DialTLSContext = func(ctx context.Context, network, address string, _ *tls.Config) (net.Conn, error) {
				var dialer net.Dialer
				return dialer.DialContext(ctx, network, address)
			}
		}
		proxy := &httputil.ReverseProxy{
			Rewrite: func(request *httputil.ProxyRequest) {
				request.SetURL(target)
				request.Out.Host = target.Host
			},
			Transport:     transport,
			FlushInterval: -1,
			ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
				w.Header().Set("Content-Type", "application/grpc")
				w.Header().Set("Trailer", "Grpc-Status, Grpc-Message")
				w.WriteHeader(http.StatusOK)
				w.Header().Set("Grpc-Status", "14")
				w.Header().Set("Grpc-Message", "upstream unavailable")
			},
		}
		routes = append(routes, route{prefix: configured.PathPrefix, target: target, timeout: configured.Timeout, proxy: proxy})
	}
	sort.Slice(routes, func(i, j int) bool { return len(routes[i].prefix) > len(routes[j].prefix) })
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.ProtoMajor != 2 || !strings.HasPrefix(strings.ToLower(request.Header.Get("Content-Type")), "application/grpc") {
			http.Error(w, "gRPC requires HTTP/2 and application/grpc", http.StatusUnsupportedMediaType)
			return
		}
		for _, candidate := range routes {
			if !strings.HasPrefix(request.URL.Path, candidate.prefix) {
				continue
			}
			started := time.Now()
			ctx := request.Context()
			cancel := func() {}
			if candidate.timeout > 0 {
				ctx, cancel = context.WithTimeout(ctx, candidate.timeout)
			}
			candidate.proxy.ServeHTTP(w, request.WithContext(ctx))
			cancel()
			if collector != nil {
				collector.ObserveGRPC(candidate.prefix, w.Header().Get("Grpc-Status"), time.Since(started))
			}
			return
		}
		http.Error(w, "gRPC route not found", http.StatusNotFound)
	}), nil
}
