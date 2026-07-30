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

func TestLoadCircuitBreakerConfiguration(t *testing.T) {
	cfg, err := loadYAML(t, `
circuit_breaker:
  failure_threshold: 7
  open_timeout: 45s
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
		"retry attempt timeout": "retry:\n  per_attempt_timeout: eventually\n",
		"retry backoff":         "retry:\n  backoff: later\n",
		"middleware timeout":    "middleware:\n  request_timeout: soon\n",
		"server timeout":        "server:\n  read_timeout: tomorrow\n",
		"circuit open timeout":  "circuit_breaker:\n  open_timeout: someday\n",
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
