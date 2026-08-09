package server

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/Djunichi/APIGateway/internal/auth"
	"github.com/Djunichi/APIGateway/internal/config"
	"github.com/Djunichi/APIGateway/internal/healthcheck"
	"github.com/Djunichi/APIGateway/internal/upstream"
)

func (s *Server) startDiscovery(_ context.Context, verifiers map[string]*auth.Verifier) {
	if s.discoveryCancel != nil {
		s.discoveryCancel()
	}
	ctx := s.runCtx
	if ctx == nil {
		return
	}
	ctx, s.discoveryCancel = context.WithCancel(ctx)
	s.discoveryExpired = make(map[int]bool)
	s.discoveryOK.Store(true)
	for i := range s.cfg.Routes {
		d := s.cfg.Routes[i].Discovery
		if d == nil {
			continue
		}
		interval, grace := d.Interval, d.Grace
		if interval <= 0 {
			interval = 30 * time.Second
		}
		if grace <= 0 {
			grace = 2 * time.Minute
		}
		go s.refreshDiscoveryRoute(ctx, i, interval, grace, verifiers)
	}
}

func (s *Server) refreshDiscoveryRoute(ctx context.Context, routeIndex int, interval, grace time.Duration, verifiers map[string]*auth.Verifier) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	lastSuccess := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.reloadMu.Lock()
			if ctx.Err() != nil {
				s.reloadMu.Unlock()
				return
			}
			if routeIndex >= len(s.cfg.Routes) || s.cfg.Routes[routeIndex].Discovery == nil {
				s.reloadMu.Unlock()
				return
			}
			route := s.cfg.Routes[routeIndex]
			upstreams, err := s.providers.Resolve(ctx, *route.Discovery)
			if err == nil && len(route.Weights) != 0 && len(route.Weights) != len(upstreams) {
				err = fmt.Errorf("weights must match discovered upstreams")
			}
			if err != nil {
				stale := now.Sub(lastSuccess)
				outcome := "stale"
				if stale >= grace {
					outcome = "expired"
					s.discoveryExpired[routeIndex] = true
					s.updateDiscoveryReadiness()
				}
				s.runtime.collector.ObserveDiscovery(route.PathPrefix, outcome, len(route.Upstreams), stale)
				s.logger.Warn("discovery refresh failed", "route", route.PathPrefix, "outcome", outcome, "error", err)
				s.reloadMu.Unlock()
				continue
			}
			if slices.Equal(route.Upstreams, upstreams) {
				lastSuccess = now
				delete(s.discoveryExpired, routeIndex)
				s.updateDiscoveryReadiness()
				s.runtime.collector.ObserveDiscovery(route.PathPrefix, "success", len(upstreams), 0)
				s.reloadMu.Unlock()
				continue
			}
			candidate := s.cfg
			candidate.Routes = append([]config.Route(nil), s.cfg.Routes...)
			candidate.Routes[routeIndex].Upstreams = append([]string(nil), upstreams...)
			handler, pool, buildErr := s.buildHandlerWithPool(candidate, s.runtime.telemetry, verifiers, s.targets)
			if buildErr != nil {
				s.runtime.collector.ObserveDiscovery(route.PathPrefix, "error", len(route.Upstreams), now.Sub(lastSuccess))
				s.logger.Error("build discovered routing snapshot", "route", route.PathPrefix, "error", buildErr)
				s.reloadMu.Unlock()
				continue
			}
			s.dynamic.Store(handler)
			s.cfg, s.targets = candidate, pool
			lastSuccess = now
			delete(s.discoveryExpired, routeIndex)
			s.updateDiscoveryReadiness()
			s.runtime.collector.ObserveDiscovery(route.PathPrefix, "success", len(upstreams), 0)
			s.startHealthChecks(ctx)
			s.reloadMu.Unlock()
		}
	}
}

func (s *Server) updateDiscoveryReadiness() {
	s.discoveryOK.Store(len(s.discoveryExpired) == 0)
	expired := make(map[string]bool, len(s.discoveryExpired))
	for index := range s.discoveryExpired {
		if index < len(s.cfg.Routes) {
			expired[s.cfg.Routes[index].PathPrefix] = true
		}
	}
	if s.dynamic != nil {
		s.dynamic.StoreExpired(expired)
	}
}

func (s *Server) startHealthChecks(_ context.Context) {
	if s.healthCancel != nil {
		s.healthCancel()
	}
	ctx := s.runCtx
	if ctx == nil {
		return
	}
	ctx, s.healthCancel = context.WithCancel(ctx)
	targets := make([]*upstream.Target, 0, len(s.targets))
	for _, target := range s.targets {
		targets = append(targets, target)
	}
	go healthcheck.New(targets, s.cfg.ActiveHealthCheck, s.runtime.transport(s.cfg.ActiveHealthCheck.Timeout)).Run(ctx)
}

func (s *Server) stopRuntimeWorkers() {
	if s.discoveryCancel != nil {
		s.discoveryCancel()
	}
	if s.healthCancel != nil {
		s.healthCancel()
	}
}
