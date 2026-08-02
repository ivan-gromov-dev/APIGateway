package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	t.Setenv("GATEWAY_SERVER_ADDRESS", ":8888")
	path := filepath.Join(t.TempDir(), "gateway.yaml")
	data := []byte("admin:\n  address: ':9090'\nroutes:\n  - path_prefix: /api/\n    upstreams: [http://localhost:8081]\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Address != ":8888" {
		t.Fatalf("server address = %q, want :8888", cfg.Server.Address)
	}
	if cfg.Server.ReadTimeout == 0 {
		t.Fatal("expected default read timeout")
	}
}

func TestExampleConfigurations(t *testing.T) {
	for _, name := range []string{"gateway.yaml", "gateway.docker.yaml", "gateway.keycloak.yaml"} {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(filepath.Join("..", "..", "configs", name)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLoadRetryConfiguration(t *testing.T) {
	cfg, err := loadYAML(t, `
retry:
  max_attempts: 4
  per_attempt_timeout: 750ms
  backoff: 25ms
  statuses: [500, 502, 504]
routes:
  - path_prefix: /api/
    upstreams: [http://localhost:8081]
`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Retry.MaxAttempts != 4 ||
		cfg.Retry.PerAttemptTimeout != 750*time.Millisecond ||
		cfg.Retry.Backoff != 25*time.Millisecond {
		t.Fatalf("retry configuration = %+v", cfg.Retry)
	}
	if len(cfg.Retry.Statuses) != 3 || cfg.Retry.Statuses[0] != 500 {
		t.Fatalf("retry statuses = %v", cfg.Retry.Statuses)
	}
}

func TestLoadGRPCConfiguration(t *testing.T) {
	cfg, err := loadYAML(t, `
grpc:
  enabled: true
  routes:
    - path_prefix: /echo.Echo/
      upstream: http://localhost:50051
      timeout: 30s
routes:
  - path_prefix: /api/
    upstreams: [http://localhost:8081]
`)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.GRPC.Enabled || len(cfg.GRPC.Routes) != 1 || cfg.GRPC.Routes[0].Timeout != 30*time.Second {
		t.Fatalf("grpc configuration=%+v", cfg.GRPC)
	}
}

func TestValidateRejectsUnsafeGRPCConfiguration(t *testing.T) {
	base := func() Config {
		cfg := defaults()
		cfg.Routes = []Route{{PathPrefix: "/api/", Upstreams: []string{"http://localhost:8081"}}}
		cfg.GRPC = GRPC{Enabled: true, Routes: []GRPCRoute{{PathPrefix: "/echo.Echo/", Upstream: "http://localhost:50051"}}}
		return cfg
	}
	tests := map[string]func(*Config){
		"disabled routes":  func(c *Config) { c.GRPC.Enabled = false },
		"bad prefix":       func(c *Config) { c.GRPC.Routes[0].PathPrefix = "echo.Echo" },
		"bad upstream":     func(c *Config) { c.GRPC.Routes[0].Upstream = "ftp://localhost" },
		"upstream path":    func(c *Config) { c.GRPC.Routes[0].Upstream = "http://localhost/base" },
		"negative timeout": func(c *Config) { c.GRPC.Routes[0].Timeout = -time.Second },
		"duplicate":        func(c *Config) { c.GRPC.Routes = append(c.GRPC.Routes, c.GRPC.Routes[0]) },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := base()
			mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestLoadCircuitBreakerConfiguration(t *testing.T) {
	cfg, err := loadYAML(t, `
circuit_breaker:
  failure_threshold: 7
  open_timeout: 45s
  failure_statuses: [500, 502]
routes:
  - path_prefix: /api/
    upstreams: [http://localhost:8081]
`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CircuitBreaker.FailureThreshold != 7 {
		t.Fatalf("failure threshold = %d, want 7", cfg.CircuitBreaker.FailureThreshold)
	}
	if cfg.CircuitBreaker.OpenTimeout != 45*time.Second {
		t.Fatalf("open timeout = %s, want 45s", cfg.CircuitBreaker.OpenTimeout)
	}
	if len(cfg.CircuitBreaker.FailureStatuses) != 2 || cfg.CircuitBreaker.FailureStatuses[0] != 500 {
		t.Fatalf("failure statuses = %v", cfg.CircuitBreaker.FailureStatuses)
	}
}

func TestLoadTelemetryConfiguration(t *testing.T) {
	cfg, err := loadYAML(t, `
telemetry:
  tracing:
    enabled: true
    service_name: test-gateway
    endpoint: collector:4318
    insecure: false
    sample_ratio: 0.25
    shutdown_timeout: 3s
routes:
  - path_prefix: /api/
    upstreams: [http://localhost:8081]
`)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Telemetry.Tracing.Enabled || cfg.Telemetry.Tracing.ServiceName != "test-gateway" ||
		cfg.Telemetry.Tracing.SampleRatio != 0.25 || cfg.Telemetry.Tracing.ShutdownTimeout != 3*time.Second {
		t.Fatalf("tracing configuration = %+v", cfg.Telemetry.Tracing)
	}
}

func TestValidateRejectsInvalidTracing(t *testing.T) {
	for name, mutate := range map[string]func(*Tracing){
		"sample ratio":     func(c *Tracing) { c.SampleRatio = 1.1 },
		"shutdown timeout": func(c *Tracing) { c.ShutdownTimeout = 0 },
		"enabled fields":   func(c *Tracing) { c.Enabled = true; c.Endpoint = "" },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := defaults()
			cfg.Routes = []Route{{PathPrefix: "/", Upstreams: []string{"http://localhost:8081"}}}
			mutate(&cfg.Telemetry.Tracing)
			if err := cfg.Validate(); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestTelemetryEnvironmentOverrides(t *testing.T) {
	t.Setenv("GATEWAY_TELEMETRY_TRACING_ENABLED", "true")
	t.Setenv("GATEWAY_TELEMETRY_TRACING_ENDPOINT", "collector:4318")
	t.Setenv("GATEWAY_TELEMETRY_TRACING_SAMPLE_RATIO", "0.5")
	cfg, err := loadYAML(t, "routes:\n  - path_prefix: /\n    upstreams: [http://localhost:8081]\n")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Telemetry.Tracing.Enabled || cfg.Telemetry.Tracing.SampleRatio != 0.5 {
		t.Fatalf("tracing = %+v", cfg.Telemetry.Tracing)
	}
}

func TestLoadRateLimitConfiguration(t *testing.T) {
	cfg, err := loadYAML(t, `
rate_limit:
  enabled: true
  default_backend: redis
  on_backend_error: deny
  operation_timeout: 50ms
  redis:
    address: redis:6379
    database: 1
    key_prefix: test
  rules:
    - name: global
      key: global
      filter: all
      token_bucket:
        requests_per_second: 10
        burst: 20
        ttl: 1m
routes:
  - path_prefix: /api/
    upstreams: [http://localhost:8081]
    rate_limits:
      - name: route
        key: client_ip
        filter: writes_only
        token_bucket:
          requests_per_second: 2
          burst: 4
          ttl: 30s
`)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.RateLimit.Enabled || cfg.RateLimit.OperationTimeout != 50*time.Millisecond ||
		len(cfg.RateLimit.Rules) != 1 || len(cfg.Routes[0].RateLimits) != 1 {
		t.Fatalf("rate limit = %+v routes=%+v", cfg.RateLimit, cfg.Routes)
	}
}

func TestLoadAuthAndCacheConfiguration(t *testing.T) {
	cfg, err := loadYAML(t, `
auth:
  providers:
    main:
      jwks_url: https://identity.example/jwks
      issuer: https://identity.example
      audience: gateway
      algorithms: [RS256]
      clock_skew: 30s
      http_timeout: 2s
cache:
  enabled: true
  on_backend_error: deny
  operation_timeout: 50ms
  max_body_bytes: 2048
  redis:
    address: redis:6379
    key_prefix: gateway:cache
routes:
  - path_prefix: /api/
    upstreams: [http://localhost:8081]
    auth:
      required: true
      provider: main
      required_scopes: [users.read]
    cache:
      enabled: true
      ttl: 1m
      vary_headers: [Accept]
`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Auth.Providers["main"].ClockSkew != 30*time.Second ||
		cfg.Routes[0].Cache.TTL != time.Minute || cfg.Cache.MaxBodyBytes != 2048 {
		t.Fatalf("configuration = %+v", cfg)
	}
}

func TestValidateRejectsInvalidAuthAndCache(t *testing.T) {
	base := func() Config {
		cfg := defaults()
		cfg.Routes = []Route{{PathPrefix: "/api/", Upstreams: []string{"http://localhost:8081"}}}
		return cfg
	}
	tests := map[string]func(*Config){
		"provider fields": func(c *Config) { c.Auth.Providers = map[string]JWTProvider{"x": {}} },
		"jwks url": func(c *Config) {
			c.Auth.Providers = map[string]JWTProvider{"x": {JWKSURL: "file:///x", Issuer: "i", Audience: "a", Algorithms: []string{"RS256"}, HTTPTimeout: time.Second}}
		},
		"algorithm": func(c *Config) {
			c.Auth.Providers = map[string]JWTProvider{"x": {JWKSURL: "https://x/jwks", Issuer: "i", Audience: "a", Algorithms: []string{"none"}, HTTPTimeout: time.Second}}
		},
		"unknown provider": func(c *Config) { c.Routes[0].Auth = RouteAuth{Required: true, Provider: "missing"} },
		"cache policy":     func(c *Config) { c.Cache.OnBackendError = "sometimes" },
		"cache bounds":     func(c *Config) { c.Cache.MaxBodyBytes = 0 },
		"cache redis":      func(c *Config) { c.Cache.Enabled = true; c.Cache.Redis.Address = "" },
		"route cache":      func(c *Config) { c.Routes[0].Cache.Enabled = true },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := base()
			mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestLoadPartialCircuitBreakerConfigurationPreservesDefaults(t *testing.T) {
	cfg, err := loadYAML(t, `
circuit_breaker:
  failure_threshold: 7
routes:
  - path_prefix: /api/
    upstreams: [http://localhost:8081]
`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CircuitBreaker.OpenTimeout != 30*time.Second {
		t.Fatalf("open timeout = %s, want default 30s", cfg.CircuitBreaker.OpenTimeout)
	}
}

func TestLoadMiddlewareConfiguration(t *testing.T) {
	cfg, err := loadYAML(t, `
middleware:
  request_timeout: 3s
  cors:
    allowed_origins: [https://example.com]
    allowed_methods: [GET]
    allowed_headers: [X-Request-ID]
routes:
  - path_prefix: /api/
    upstreams: [http://localhost:8081]
`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Middleware.RequestTimeout != 3*time.Second {
		t.Fatalf("request timeout = %s", cfg.Middleware.RequestTimeout)
	}
	if len(cfg.Middleware.CORS.AllowedOrigins) != 1 ||
		cfg.Middleware.CORS.AllowedOrigins[0] != "https://example.com" {
		t.Fatalf("allowed origins = %v", cfg.Middleware.CORS.AllowedOrigins)
	}
	if len(cfg.Middleware.CORS.AllowedMethods) != 1 ||
		cfg.Middleware.CORS.AllowedMethods[0] != "GET" {
		t.Fatalf("allowed methods = %v", cfg.Middleware.CORS.AllowedMethods)
	}
	if len(cfg.Middleware.CORS.AllowedHeaders) != 1 ||
		cfg.Middleware.CORS.AllowedHeaders[0] != "X-Request-ID" {
		t.Fatalf("allowed headers = %v", cfg.Middleware.CORS.AllowedHeaders)
	}
}

func TestLoadRejectsInvalidConfiguredDurations(t *testing.T) {
	tests := map[string]string{
		"retry attempt timeout":  "retry:\n  per_attempt_timeout: eventually\n",
		"retry backoff":          "retry:\n  backoff: later\n",
		"middleware timeout":     "middleware:\n  request_timeout: soon\n",
		"server timeout":         "server:\n  read_timeout: tomorrow\n",
		"circuit open timeout":   "circuit_breaker:\n  open_timeout: someday\n",
		"rate operation timeout": "rate_limit:\n  operation_timeout: someday\n",
		"token bucket ttl":       "rate_limit:\n  rules:\n    - name: x\n      key: global\n      filter: all\n      token_bucket:\n        requests_per_second: 1\n        burst: 1\n        ttl: someday\n",
	}
	for name, fragment := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := loadYAML(t, fragment+validRoutesUnlessPresent(fragment))
			if err == nil {
				t.Fatal("expected invalid duration error")
			}
		})
	}
}

func TestValidateRejectsInvalidUpstream(t *testing.T) {
	cfg := defaults()
	cfg.Routes = []Route{{PathPrefix: "/api/", Upstreams: []string{"ftp://example.com"}}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	tests := map[string]string{
		"root":            "unknown: true\n",
		"server":          "server:\n  unknown: true\n",
		"middleware":      "middleware:\n  unknown: true\n",
		"cors":            "middleware:\n  cors:\n    unknown: true\n",
		"retry":           "retry:\n  unknown: true\n",
		"circuit breaker": "circuit_breaker:\n  unknown: true\n",
		"route":           "routes:\n  - path_prefix: /api/\n    upstreams: [http://localhost:8081]\n    unknown: true\n",
		"auth":            "auth:\n  unknown: true\n",
		"provider":        "auth:\n  providers:\n    x:\n      unknown: true\n",
		"cache":           "cache:\n  unknown: true\n",
	}
	for name, fragment := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := loadYAML(t, fragment+validRoutesUnlessPresent(fragment))
			if err == nil || !strings.Contains(err.Error(), "field unknown not found") {
				t.Fatalf("error = %v, want unknown field error", err)
			}
		})
	}
}

func TestLoadRejectsMultipleDocuments(t *testing.T) {
	_, err := loadYAML(t, "routes:\n  - path_prefix: /api/\n    upstreams: [http://localhost:8081]\n---\nroutes: []\n")
	if err == nil || !strings.Contains(err.Error(), "exactly one YAML document") {
		t.Fatalf("error = %v, want multiple document error", err)
	}
}

func TestLoadRejectsInvalidEnvironmentDuration(t *testing.T) {
	t.Setenv("GATEWAY_SERVER_READ_TIMEOUT", "eventually")
	_, err := loadYAML(t, "routes:\n  - path_prefix: /api/\n    upstreams: [http://localhost:8081]\n")
	if err == nil || !strings.Contains(err.Error(), "GATEWAY_SERVER_READ_TIMEOUT") {
		t.Fatalf("error = %v, want environment variable error", err)
	}
}

func TestValidateRuntimeSettings(t *testing.T) {
	tests := map[string]func(*Config){
		"shutdown timeout": func(cfg *Config) { cfg.Server.ShutdownTimeout = 0 },
		"request timeout":  func(cfg *Config) { cfg.Middleware.RequestTimeout = -1 },
		"log level":        func(cfg *Config) { cfg.Log.Level = "verbose" },
		"log format":       func(cfg *Config) { cfg.Log.Format = "xml" },
		"upstream host":    func(cfg *Config) { cfg.Routes[0].Upstreams = []string{"http://"} },
		"empty upstreams":  func(cfg *Config) { cfg.Routes[0].Upstreams = nil },
		"duplicate": func(cfg *Config) {
			cfg.Routes[0].Upstreams = []string{"http://localhost:8081", "http://localhost:8081"}
		},
		"retry attempts": func(cfg *Config) { cfg.Retry.MaxAttempts = 0 },
		"retry timeout":  func(cfg *Config) { cfg.Retry.PerAttemptTimeout = 0 },
		"retry backoff":  func(cfg *Config) { cfg.Retry.Backoff = -1 },
		"retry statuses": func(cfg *Config) { cfg.Retry.Statuses = []int{429} },
		"duplicate retry status": func(cfg *Config) {
			cfg.Retry.Statuses = []int{503, 503}
		},
		"circuit breaker threshold": func(cfg *Config) { cfg.CircuitBreaker.FailureThreshold = 0 },
		"circuit breaker timeout":   func(cfg *Config) { cfg.CircuitBreaker.OpenTimeout = 0 },
		"circuit breaker statuses": func(cfg *Config) {
			cfg.CircuitBreaker.FailureStatuses = []int{429}
		},
		"duplicate circuit breaker status": func(cfg *Config) {
			cfg.CircuitBreaker.FailureStatuses = []int{503, 503}
		},
		"rate error policy":    func(cfg *Config) { cfg.RateLimit.OnBackendError = "sometimes" },
		"rate timeout":         func(cfg *Config) { cfg.RateLimit.OperationTimeout = 0 },
		"rate enabled backend": func(cfg *Config) { cfg.RateLimit.Enabled = true; cfg.RateLimit.DefaultBackend = "" },
		"rate invalid rule": func(cfg *Config) {
			cfg.RateLimit.Rules = []RateLimitRule{{Name: "x", Key: "global", Filter: "all"}}
		},
		"rate duplicate rule": func(cfg *Config) {
			rule := RateLimitRule{Name: "x", Key: "global", Filter: "all", TokenBucket: TokenBucket{RequestsPerSecond: 1, Burst: 1, TTL: time.Minute}}
			cfg.RateLimit.Rules = []RateLimitRule{rule, rule}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := defaults()
			cfg.Routes = []Route{{PathPrefix: "/api/", Upstreams: []string{"http://localhost:8081"}}}
			mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func loadYAML(t *testing.T, contents string) (Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gateway.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return Load(path)
}

func validRoutesUnlessPresent(contents string) string {
	if strings.Contains(contents, "routes:") {
		return ""
	}
	return "routes:\n  - path_prefix: /api/\n    upstreams: [http://localhost:8081]\n"
}
