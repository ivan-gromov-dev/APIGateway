package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/http"
	"strconv"

	"github.com/Djunichi/APIGateway/internal/config"
	"github.com/Djunichi/APIGateway/internal/limiter"
	"github.com/Djunichi/APIGateway/internal/metrics"
	"github.com/Djunichi/APIGateway/internal/ratelimit"
)

type rateRule struct {
	name    string
	limiter ratelimit.Limiter
	key     ratelimit.KeyFunc
	filter  ratelimit.FilterFunc
}

func BuildRateLimit(rules []config.RateLimitRule, cfg config.RateLimit, registry *ratelimit.Registry) (Middleware, error) {
	return BuildRateLimitWithMetrics(rules, cfg, registry, nil, "global")
}

func BuildRateLimitWithMetrics(rules []config.RateLimitRule, cfg config.RateLimit, registry *ratelimit.Registry, collector *metrics.Collector, scope string) (Middleware, error) {
	if len(rules) == 0 {
		return func(next http.Handler) http.Handler { return next }, nil
	}
	built := make([]rateRule, 0, len(rules))
	for _, rule := range rules {
		backend := rule.Backend
		if backend == "" {
			backend = cfg.DefaultBackend
		}
		store, ok := registry.Store(backend)
		if !ok {
			return nil, fmt.Errorf("rate limit backend %q is not registered", backend)
		}
		key, ok := registry.Key(rule.Key)
		if !ok {
			return nil, fmt.Errorf("rate limit key %q is not registered", rule.Key)
		}
		filter, ok := registry.Filter(rule.Filter)
		if !ok {
			return nil, fmt.Errorf("rate limit filter %q is not registered", rule.Filter)
		}
		built = append(built, rateRule{
			name: rule.Name, key: key, filter: filter,
			limiter: limiter.NewTokenBucket(store, rule.TokenBucket.RequestsPerSecond, rule.TokenBucket.Burst, rule.TokenBucket.TTL),
		})
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, rule := range built {
				if !rule.filter(r) {
					continue
				}
				key, err := rule.key(r)
				if err != nil {
					observeFeature(collector, "rate_limit", scope, rule.name, "key_error")
					writeRateError(w, http.StatusServiceUnavailable, "rate limiter unavailable")
					return
				}
				ctx, cancel := context.WithTimeout(r.Context(), cfg.OperationTimeout)
				result, err := rule.limiter.Allow(ctx, rule.name+":"+key)
				cancel()
				if err != nil {
					if cfg.OnBackendError == "allow" {
						observeFeature(collector, "rate_limit", scope, rule.name, "backend_error_allowed")
						continue
					}
					observeFeature(collector, "rate_limit", scope, rule.name, "backend_error_denied")
					writeRateError(w, http.StatusServiceUnavailable, "rate limiter unavailable")
					return
				}
				if !result.Allowed {
					observeFeature(collector, "rate_limit", scope, rule.name, "limited")
					seconds := int(math.Ceil(result.RetryAfter.Seconds()))
					if seconds < 1 {
						seconds = 1
					}
					w.Header().Set("Retry-After", strconv.Itoa(seconds))
					writeRateError(w, http.StatusTooManyRequests, "rate limit exceeded")
					return
				}
				observeFeature(collector, "rate_limit", scope, rule.name, "allowed")
			}
			next.ServeHTTP(w, r)
		})
	}, nil
}

func observeFeature(collector *metrics.Collector, feature, scope, name, result string) {
	if collector != nil {
		collector.ObserveFeature(feature, scope, name, result)
	}
}

func writeRateError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func GlobalKey(*http.Request) (string, error) { return "global", nil }

func ClientIPKey(r *http.Request) (string, error) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		if net.ParseIP(r.RemoteAddr) != nil {
			return r.RemoteAddr, nil
		}
		return "", fmt.Errorf("parse remote address: %w", err)
	}
	return host, nil
}

func AllRequests(*http.Request) bool { return true }

func WritesOnly(r *http.Request) bool {
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}
