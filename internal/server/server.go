package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/pprof"

	"github.com/Djunichi/APIGateway/internal/config"
	"github.com/Djunichi/APIGateway/internal/metrics"
	"github.com/Djunichi/APIGateway/internal/middleware"
	"github.com/Djunichi/APIGateway/internal/proxy"
)

type Server struct {
	cfg    config.Config
	logger *slog.Logger
}

func New(cfg config.Config, logger *slog.Logger) *Server {
	return &Server{cfg: cfg, logger: logger}
}

func (s *Server) Run(ctx context.Context) error {
	collector := &metrics.Collector{}
	proxyHandler, err := proxy.Handler(s.cfg.Routes, s.logger)
	if err != nil {
		return err
	}
	handler := middleware.Chain(proxyHandler,
		middleware.Recovery(s.logger),
		middleware.RequestID,
		middleware.Logging(s.logger, collector),
		middleware.Timeout(s.cfg.Middleware.RequestTimeout),
		middleware.CORS(s.cfg.Middleware.CORS),
	)

	app := httpServer(s.cfg.Server, handler)
	admin := httpServer(s.cfg.Admin, adminHandler(collector))
	errs := make(chan error, 2)
	go serve(app, errs)
	go serve(admin, errs)

	select {
	case <-ctx.Done():
	case err := <-errs:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.cfg.Server.ShutdownTimeout)
	defer cancel()
	if err := app.Shutdown(shutdownCtx); err != nil {
		return err
	}
	return admin.Shutdown(shutdownCtx)
}

func httpServer(cfg config.HTTPServer, handler http.Handler) *http.Server {
	return &http.Server{
		Addr: cfg.Address, Handler: handler,
		ReadTimeout: cfg.ReadTimeout, WriteTimeout: cfg.WriteTimeout, IdleTimeout: cfg.IdleTimeout,
	}
}

func serve(server *http.Server, errs chan<- error) {
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		errs <- err
	}
}

func adminHandler(collector *metrics.Collector) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", ok)
	mux.HandleFunc("GET /readyz", ok)
	mux.Handle("GET /metrics", collector)
	mux.HandleFunc("GET /debug/pprof/", pprof.Index)
	mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)
	return mux
}

func ok(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
