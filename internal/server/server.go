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

	"github.com/Djunichi/APIGateway/internal/config"
	"github.com/Djunichi/APIGateway/internal/metrics"
	"github.com/Djunichi/APIGateway/internal/middleware"
	"github.com/Djunichi/APIGateway/internal/proxy"
)

type Server struct {
	cfg    config.Config
	logger *slog.Logger
	ready  atomic.Bool
}

func New(cfg config.Config, logger *slog.Logger) *Server {
	return &Server{cfg: cfg, logger: logger}
}

func (s *Server) Run(ctx context.Context) error {
	collector := &metrics.Collector{}
	proxyHandler, err := proxy.Handler(s.cfg.Routes, s.cfg.Retry, s.logger)
	if err != nil {
		return err
	}
	handler := middleware.Chain(
		proxyHandler,
		middleware.Recovery(s.logger),
		middleware.RequestID,
		middleware.Logging(s.logger, collector),
		middleware.Timeout(s.cfg.Middleware.RequestTimeout),
		middleware.CORS(s.cfg.Middleware.CORS),
	)

	app := httpServer(s.cfg.Server, handler)
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
