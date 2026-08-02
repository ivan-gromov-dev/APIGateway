// Package config loads, validates, and exposes the gateway's runtime configuration.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server            HTTPServer        `yaml:"server"`
	Admin             HTTPServer        `yaml:"admin"`
	Log               Log               `yaml:"log"`
	Middleware        Middleware        `yaml:"middleware"`
	Retry             Retry             `yaml:"retry"`
	GRPC              GRPC              `yaml:"grpc"`
	Routes            []Route           `yaml:"routes"`
	CircuitBreaker    CircuitBreaker    `yaml:"circuit_breaker"`
	ActiveHealthCheck ActiveHealthCheck `yaml:"active_health_check"`
	RateLimit         RateLimit         `yaml:"rate_limit"`
	Auth              Auth              `yaml:"auth"`
	Cache             Cache             `yaml:"cache"`
	Telemetry         Telemetry         `yaml:"telemetry"`
	Reload            Reload            `yaml:"reload"`
}

type Reload struct {
	Interval time.Duration `yaml:"interval"`
}

func (r *Reload) UnmarshalYAML(value *yaml.Node) error {
	if err := rejectUnknownFields(value, "interval"); err != nil {
		return err
	}
	var raw struct {
		Interval string `yaml:"interval"`
	}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	if raw.Interval != "" {
		d, err := time.ParseDuration(raw.Interval)
		if err != nil {
			return fmt.Errorf("interval: %w", err)
		}
		r.Interval = d
	}
	return nil
}

type ActiveHealthCheck struct {
	Enabled            bool          `yaml:"enabled"`
	Path               string        `yaml:"path"`
	Interval           time.Duration `yaml:"interval"`
	Timeout            time.Duration `yaml:"timeout"`
	HealthyThreshold   int           `yaml:"healthy_threshold"`
	UnhealthyThreshold int           `yaml:"unhealthy_threshold"`
}

type Telemetry struct {
	Tracing Tracing `yaml:"tracing"`
}

type Tracing struct {
	Enabled         bool          `yaml:"enabled"`
	ServiceName     string        `yaml:"service_name"`
	Endpoint        string        `yaml:"endpoint"`
	Insecure        bool          `yaml:"insecure"`
	SampleRatio     float64       `yaml:"sample_ratio"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
}

func (t *Telemetry) UnmarshalYAML(value *yaml.Node) error {
	if err := rejectUnknownFields(value, "tracing"); err != nil {
		return err
	}
	type plain Telemetry
	return value.Decode((*plain)(t))
}

func (t *Tracing) UnmarshalYAML(value *yaml.Node) error {
	if err := rejectUnknownFields(value, "enabled", "service_name", "endpoint", "insecure", "sample_ratio", "shutdown_timeout"); err != nil {
		return err
	}
	var v struct {
		Enabled         *bool    `yaml:"enabled"`
		ServiceName     string   `yaml:"service_name"`
		Endpoint        string   `yaml:"endpoint"`
		Insecure        *bool    `yaml:"insecure"`
		SampleRatio     *float64 `yaml:"sample_ratio"`
		ShutdownTimeout string   `yaml:"shutdown_timeout"`
	}
	if err := value.Decode(&v); err != nil {
		return err
	}
	if v.Enabled != nil {
		t.Enabled = *v.Enabled
	}
	if v.ServiceName != "" {
		t.ServiceName = v.ServiceName
	}
	if v.Endpoint != "" {
		t.Endpoint = v.Endpoint
	}
	if v.Insecure != nil {
		t.Insecure = *v.Insecure
	}
	if v.SampleRatio != nil {
		t.SampleRatio = *v.SampleRatio
	}
	if v.ShutdownTimeout != "" {
		duration, err := time.ParseDuration(v.ShutdownTimeout)
		if err != nil {
			return fmt.Errorf("shutdown_timeout: %w", err)
		}
		t.ShutdownTimeout = duration
	}
	return nil
}

type HTTPServer struct {
	Address         string        `yaml:"address"`
	ReadTimeout     time.Duration `yaml:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout"`
	IdleTimeout     time.Duration `yaml:"idle_timeout"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
}

type Log struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

type Middleware struct {
	RequestTimeout time.Duration `yaml:"request_timeout"`
	CORS           CORS          `yaml:"cors"`
}

type CORS struct {
	AllowedOrigins []string `yaml:"allowed_origins"`
	AllowedMethods []string `yaml:"allowed_methods"`
	AllowedHeaders []string `yaml:"allowed_headers"`
}

type Retry struct {
	MaxAttempts       int           `yaml:"max_attempts"`
	PerAttemptTimeout time.Duration `yaml:"per_attempt_timeout"`
	Backoff           time.Duration `yaml:"backoff"`
	Statuses          []int         `yaml:"statuses"`
}

// GRPC configures the independent transparent gRPC proxy vertical. RPCs are
// never passed through the HTTP retry policy; every call has one attempt.
type GRPC struct {
	Enabled bool        `yaml:"enabled"`
	Routes  []GRPCRoute `yaml:"routes"`
}

type GRPCRoute struct {
	PathPrefix string        `yaml:"path_prefix"`
	Upstream   string        `yaml:"upstream"`
	Timeout    time.Duration `yaml:"timeout"`
}

func (g *GRPC) UnmarshalYAML(value *yaml.Node) error {
	if err := rejectUnknownFields(value, "enabled", "routes"); err != nil {
		return err
	}
	type plain GRPC
	return value.Decode((*plain)(g))
}

func (r *GRPCRoute) UnmarshalYAML(value *yaml.Node) error {
	if err := rejectUnknownFields(value, "path_prefix", "upstream", "timeout"); err != nil {
		return err
	}
	var raw struct {
		PathPrefix string `yaml:"path_prefix"`
		Upstream   string `yaml:"upstream"`
		Timeout    string `yaml:"timeout"`
	}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	r.PathPrefix, r.Upstream = raw.PathPrefix, raw.Upstream
	if raw.Timeout != "" {
		timeout, err := time.ParseDuration(raw.Timeout)
		if err != nil {
			return fmt.Errorf("timeout: %w", err)
		}
		r.Timeout = timeout
	}
	return nil
}

func (c *CORS) UnmarshalYAML(value *yaml.Node) error {
	if err := rejectUnknownFields(value, "allowed_origins", "allowed_methods", "allowed_headers"); err != nil {
		return err
	}
	type plain CORS
	return value.Decode((*plain)(c))
}

type Route struct {
	PathPrefix  string          `yaml:"path_prefix"`
	Upstreams   []string        `yaml:"upstreams"`
	Discovery   *Discovery      `yaml:"discovery"`
	Balancer    string          `yaml:"balancer"`
	Weights     []int           `yaml:"weights"`
	Rollout     Rollout         `yaml:"rollout"`
	StripPrefix bool            `yaml:"strip_prefix"`
	RateLimits  []RateLimitRule `yaml:"rate_limits"`
	Auth        RouteAuth       `yaml:"auth"`
	Cache       RouteCache      `yaml:"cache"`
}

// Discovery configures runtime resolution of a route's upstream instances.
// The first supported provider is DNS; resolved addresses retain the route's
// configured scheme and port.
type Discovery struct {
	Provider string        `yaml:"provider"`
	Name     string        `yaml:"name"`
	Scheme   string        `yaml:"scheme"`
	Port     int           `yaml:"port"`
	Interval time.Duration `yaml:"interval"`
	Grace    time.Duration `yaml:"grace"`
}

// Rollout controls gradual exposure of the final upstream in a route. The
// selector is deliberately bounded to request metadata, never raw paths.
type Rollout struct {
	Strategy   string `yaml:"strategy"`
	Percentage int    `yaml:"percentage"`
	Header     string `yaml:"header"`
	StableHash string `yaml:"stable_hash"`
}

type Auth struct {
	Providers map[string]JWTProvider `yaml:"providers"`
}

type JWTProvider struct {
	JWKSURL     string        `yaml:"jwks_url"`
	Issuer      string        `yaml:"issuer"`
	Audience    string        `yaml:"audience"`
	Algorithms  []string      `yaml:"algorithms"`
	ClockSkew   time.Duration `yaml:"clock_skew"`
	HTTPTimeout time.Duration `yaml:"http_timeout"`
}

type RouteAuth struct {
	Required       bool     `yaml:"required"`
	Provider       string   `yaml:"provider"`
	RequiredScopes []string `yaml:"required_scopes"`
}

type Cache struct {
	Enabled          bool          `yaml:"enabled"`
	OnBackendError   string        `yaml:"on_backend_error"`
	OperationTimeout time.Duration `yaml:"operation_timeout"`
	MaxBodyBytes     int64         `yaml:"max_body_bytes"`
	Redis            Redis         `yaml:"redis"`
}

type RouteCache struct {
	Enabled     bool          `yaml:"enabled"`
	TTL         time.Duration `yaml:"ttl"`
	VaryHeaders []string      `yaml:"vary_headers"`
}

type RateLimit struct {
	Enabled          bool            `yaml:"enabled"`
	DefaultBackend   string          `yaml:"default_backend"`
	OnBackendError   string          `yaml:"on_backend_error"`
	OperationTimeout time.Duration   `yaml:"operation_timeout"`
	Redis            Redis           `yaml:"redis"`
	Rules            []RateLimitRule `yaml:"rules"`
}

type Redis struct {
	Address   string `yaml:"address"`
	Username  string `yaml:"username"`
	Password  string `yaml:"password"`
	Database  int    `yaml:"database"`
	KeyPrefix string `yaml:"key_prefix"`
}

type RateLimitRule struct {
	Name        string      `yaml:"name"`
	Backend     string      `yaml:"backend"`
	Key         string      `yaml:"key"`
	Filter      string      `yaml:"filter"`
	TokenBucket TokenBucket `yaml:"token_bucket"`
}

type TokenBucket struct {
	RequestsPerSecond float64       `yaml:"requests_per_second"`
	Burst             int           `yaml:"burst"`
	TTL               time.Duration `yaml:"ttl"`
}

type CircuitBreaker struct {
	FailureThreshold int           `yaml:"failure_threshold"`
	OpenTimeout      time.Duration `yaml:"open_timeout"`
	FailureStatuses  []int         `yaml:"failure_statuses"`
}

func (h *ActiveHealthCheck) UnmarshalYAML(value *yaml.Node) error {
	if err := rejectUnknownFields(value, "enabled", "path", "interval", "timeout", "healthy_threshold", "unhealthy_threshold"); err != nil {
		return err
	}
	var v struct {
		Enabled            *bool  `yaml:"enabled"`
		Path               string `yaml:"path"`
		Interval           string `yaml:"interval"`
		Timeout            string `yaml:"timeout"`
		HealthyThreshold   int    `yaml:"healthy_threshold"`
		UnhealthyThreshold int    `yaml:"unhealthy_threshold"`
	}
	if err := value.Decode(&v); err != nil {
		return err
	}
	if v.Enabled != nil {
		h.Enabled = *v.Enabled
	}
	if v.Path != "" {
		h.Path = v.Path
	}
	h.HealthyThreshold = v.HealthyThreshold
	h.UnhealthyThreshold = v.UnhealthyThreshold
	if v.Interval != "" {
		d, err := time.ParseDuration(v.Interval)
		if err != nil {
			return fmt.Errorf("interval: %w", err)
		}
		h.Interval = d
	}
	if v.Timeout != "" {
		d, err := time.ParseDuration(v.Timeout)
		if err != nil {
			return fmt.Errorf("timeout: %w", err)
		}
		h.Timeout = d
	}
	return nil
}

func Load(path string) (Config, error) {
	cfg := defaults()
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return Config{}, errors.New("configuration must contain exactly one YAML document")
		}
		return Config{}, err
	}
	if err := applyEnvironment(&cfg); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func defaults() Config {
	return Config{
		Server: HTTPServer{
			Address: ":8080", ReadTimeout: 10 * time.Second,
			WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second,
			ShutdownTimeout: 10 * time.Second,
		},
		Admin: HTTPServer{Address: ":9090", ReadTimeout: 5 * time.Second, WriteTimeout: 10 * time.Second},
		Log:   Log{Level: "info", Format: "json"},
		Retry: Retry{
			MaxAttempts: 1, PerAttemptTimeout: 2 * time.Second,
			Statuses: []int{502, 503, 504},
		},
		Middleware: Middleware{
			RequestTimeout: 15 * time.Second,
			CORS: CORS{
				AllowedOrigins: []string{"*"},
				AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
				AllowedHeaders: []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
			},
		},
		CircuitBreaker: CircuitBreaker{
			FailureThreshold: 5,
			OpenTimeout:      30 * time.Second,
			FailureStatuses:  []int{502, 503, 504},
		},
		ActiveHealthCheck: ActiveHealthCheck{Path: "/healthz", Interval: 10 * time.Second, Timeout: 2 * time.Second, HealthyThreshold: 2, UnhealthyThreshold: 3},
		RateLimit: RateLimit{
			DefaultBackend: "redis", OnBackendError: "allow",
			OperationTimeout: 100 * time.Millisecond,
			Redis:            Redis{Address: "localhost:6379", KeyPrefix: "gateway:ratelimit"},
		},
		Cache: Cache{OnBackendError: "allow", OperationTimeout: 100 * time.Millisecond,
			MaxBodyBytes: 1 << 20, Redis: Redis{Address: "localhost:6379", KeyPrefix: "gateway:cache"}},
		Telemetry: Telemetry{Tracing: Tracing{ServiceName: "api-gateway", Endpoint: "localhost:4318",
			Insecure: true, SampleRatio: 1, ShutdownTimeout: 5 * time.Second}},
		Reload: Reload{Interval: 30 * time.Second},
	}
}

func (c Config) Validate() error {
	if c.Reload.Interval < 0 {
		return errors.New("reload interval must not be negative")
	}
	if err := c.validateTracing(); err != nil {
		return err
	}
	if c.Server.Address == "" || c.Admin.Address == "" {
		return errors.New("server and admin addresses are required")
	}
	if c.Server.Address == c.Admin.Address {
		return errors.New("server and admin addresses must differ")
	}
	if c.Server.ShutdownTimeout <= 0 {
		return errors.New("server shutdown_timeout must be positive")
	}
	if c.Server.ReadTimeout < 0 || c.Server.WriteTimeout < 0 || c.Server.IdleTimeout < 0 {
		return errors.New("server timeouts must not be negative")
	}
	if c.Admin.ReadTimeout < 0 || c.Admin.WriteTimeout < 0 || c.Admin.IdleTimeout < 0 {
		return errors.New("admin timeouts must not be negative")
	}
	if c.Middleware.RequestTimeout < 0 {
		return errors.New("middleware request_timeout must not be negative")
	}
	if c.CircuitBreaker.FailureThreshold <= 0 {
		return errors.New("circuit breaker failure threshold must be positive")
	}
	if c.CircuitBreaker.OpenTimeout <= 0 {
		return errors.New("circuit breaker open_timeout must be positive")
	}
	if c.ActiveHealthCheck.Interval <= 0 || c.ActiveHealthCheck.Timeout <= 0 || c.ActiveHealthCheck.Timeout > c.ActiveHealthCheck.Interval {
		return errors.New("active health check interval and timeout must be positive, with timeout no greater than interval")
	}
	if c.ActiveHealthCheck.Path == "" || !strings.HasPrefix(c.ActiveHealthCheck.Path, "/") {
		return errors.New("active health check path must start with /")
	}
	if c.ActiveHealthCheck.HealthyThreshold <= 0 || c.ActiveHealthCheck.UnhealthyThreshold <= 0 {
		return errors.New("active health check thresholds must be positive")
	}
	if err := validateStatuses("circuit breaker failure", c.CircuitBreaker.FailureStatuses); err != nil {
		return err
	}
	if err := c.validateRateLimits(); err != nil {
		return err
	}
	if err := c.validateAuthAndCache(); err != nil {
		return err
	}
	if c.Retry.MaxAttempts < 1 || c.Retry.MaxAttempts > 10 {
		return errors.New("retry max_attempts must be between 1 and 10")
	}
	if c.Retry.PerAttemptTimeout <= 0 {
		return errors.New("retry per_attempt_timeout must be positive")
	}
	if c.Retry.Backoff < 0 {
		return errors.New("retry backoff must not be negative")
	}
	if len(c.Retry.Statuses) == 0 {
		return errors.New("retry statuses must not be empty")
	}
	if err := validateStatuses("retry", c.Retry.Statuses); err != nil {
		return err
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(c.Log.Level)); err != nil {
		return fmt.Errorf("log level %q: %w", c.Log.Level, err)
	}
	if c.Log.Format != "json" && c.Log.Format != "text" {
		return fmt.Errorf("log format must be json or text, got %q", c.Log.Format)
	}
	if len(c.Routes) == 0 && (!c.GRPC.Enabled || len(c.GRPC.Routes) == 0) {
		return errors.New("at least one HTTP or enabled gRPC route is required")
	}
	if !c.GRPC.Enabled && len(c.GRPC.Routes) != 0 {
		return errors.New("grpc routes require grpc.enabled")
	}
	seenGRPC := make(map[string]struct{}, len(c.GRPC.Routes))
	for i, route := range c.GRPC.Routes {
		if !strings.HasPrefix(route.PathPrefix, "/") || !strings.HasSuffix(route.PathPrefix, "/") {
			return fmt.Errorf("grpc route %d: path_prefix must start and end with /", i)
		}
		if _, exists := seenGRPC[route.PathPrefix]; exists {
			return fmt.Errorf("grpc route %d: duplicate path_prefix %q", i, route.PathPrefix)
		}
		seenGRPC[route.PathPrefix] = struct{}{}
		upstreamURL, err := url.Parse(route.Upstream)
		if err != nil || (upstreamURL.Scheme != "http" && upstreamURL.Scheme != "https") || upstreamURL.Host == "" || upstreamURL.Path != "" {
			return fmt.Errorf("grpc route %d: upstream must be an http(s) origin without a path", i)
		}
		if route.Timeout < 0 {
			return fmt.Errorf("grpc route %d: timeout must not be negative", i)
		}
	}
	seen := make(map[string]struct{}, len(c.Routes))
	for i, route := range c.Routes {
		if !strings.HasPrefix(route.PathPrefix, "/") {
			return fmt.Errorf("route %d: path_prefix must start with /", i)
		}
		if _, ok := seen[route.PathPrefix]; ok {
			return fmt.Errorf("route %d: duplicate path_prefix %q", i, route.PathPrefix)
		}
		seen[route.PathPrefix] = struct{}{}
		if len(route.Upstreams) == 0 {
			if route.Discovery == nil {
				return fmt.Errorf("route %d: at least one upstream or discovery is required", i)
			}
		}
		if route.Discovery != nil {
			if route.Discovery.Provider != "dns" || route.Discovery.Name == "" || (route.Discovery.Scheme != "http" && route.Discovery.Scheme != "https") || route.Discovery.Port < 1 || route.Discovery.Port > 65535 {
				return fmt.Errorf("route %d: discovery requires provider dns, name, http(s) scheme, and valid port", i)
			}
			if route.Discovery.Interval <= 0 || route.Discovery.Grace < 0 {
				return fmt.Errorf("route %d: discovery interval must be positive and grace non-negative", i)
			}
		}
		if route.Balancer == "" {
			route.Balancer = "round_robin"
		}
		if route.Balancer != "round_robin" && route.Balancer != "weighted_round_robin" {
			return fmt.Errorf("route %d: balancer must be round_robin or weighted_round_robin", i)
		}
		if route.Balancer == "weighted_round_robin" && len(route.Weights) == 0 {
			return fmt.Errorf("route %d: weighted_round_robin requires weights", i)
		}
		if route.Balancer == "round_robin" && len(route.Weights) > 0 {
			return fmt.Errorf("route %d: weights require weighted_round_robin", i)
		}
		if len(route.Weights) != 0 && route.Discovery == nil && len(route.Weights) != len(route.Upstreams) {
			return fmt.Errorf("route %d: weights must match upstreams", i)
		}
		for _, weight := range route.Weights {
			if weight <= 0 {
				return fmt.Errorf("route %d: weights must be positive", i)
			}
		}
		if route.Rollout.Strategy != "" && route.Rollout.Strategy != "percentage" && route.Rollout.Strategy != "header" && route.Rollout.Strategy != "stable_hash" {
			return fmt.Errorf("route %d: rollout strategy must be percentage, header, or stable_hash", i)
		}
		if route.Rollout.Percentage < 0 || route.Rollout.Percentage > 100 {
			return fmt.Errorf("route %d: rollout percentage must be between 0 and 100", i)
		}
		if route.Rollout.Strategy == "header" && route.Rollout.Header == "" || route.Rollout.Strategy == "stable_hash" && route.Rollout.StableHash == "" {
			return fmt.Errorf("route %d: rollout selector is required", i)
		}
		seenUpstreams := make(map[string]struct{}, len(route.Upstreams))
		for j, rawUpstream := range route.Upstreams {
			upstream, err := url.Parse(rawUpstream)
			if err != nil || (upstream.Scheme != "http" && upstream.Scheme != "https") || upstream.Host == "" {
				return fmt.Errorf("route %d upstream %d: must use http or https", i, j)
			}
			if _, ok := seenUpstreams[upstream.String()]; ok {
				return fmt.Errorf("route %d: duplicate upstream %q", i, rawUpstream)
			}
			seenUpstreams[upstream.String()] = struct{}{}
		}
	}
	return nil
}

func (c Config) validateTracing() error {
	tracing := c.Telemetry.Tracing
	if tracing.SampleRatio < 0 || tracing.SampleRatio > 1 {
		return errors.New("telemetry tracing sample_ratio must be between 0 and 1")
	}
	if tracing.ShutdownTimeout <= 0 {
		return errors.New("telemetry tracing shutdown_timeout must be positive")
	}
	if tracing.Enabled && (tracing.ServiceName == "" || tracing.Endpoint == "") {
		return errors.New("enabled telemetry tracing requires service_name and endpoint")
	}
	return nil
}

func (c Config) validateAuthAndCache() error {
	for name, provider := range c.Auth.Providers {
		if name == "" || provider.JWKSURL == "" || provider.Issuer == "" || provider.Audience == "" {
			return fmt.Errorf("auth provider %q requires jwks_url, issuer, and audience", name)
		}
		u, err := url.Parse(provider.JWKSURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("auth provider %q jwks_url must use http or https", name)
		}
		if len(provider.Algorithms) == 0 || provider.ClockSkew < 0 || provider.HTTPTimeout <= 0 {
			return fmt.Errorf("auth provider %q requires algorithms, non-negative clock_skew, and positive http_timeout", name)
		}
		for _, algorithm := range provider.Algorithms {
			if algorithm != "RS256" {
				return fmt.Errorf("auth provider %q: unsupported algorithm %q", name, algorithm)
			}
		}
	}
	if c.Cache.OnBackendError != "allow" && c.Cache.OnBackendError != "deny" {
		return errors.New("cache on_backend_error must be allow or deny")
	}
	if c.Cache.OperationTimeout <= 0 || c.Cache.MaxBodyBytes <= 0 {
		return errors.New("cache operation_timeout and max_body_bytes must be positive")
	}
	if c.Cache.Enabled && (c.Cache.Redis.Address == "" || c.Cache.Redis.KeyPrefix == "") {
		return errors.New("enabled cache requires Redis address and key prefix")
	}
	for i, route := range c.Routes {
		if route.Auth.Required {
			if _, ok := c.Auth.Providers[route.Auth.Provider]; !ok {
				return fmt.Errorf("route %d references unknown auth provider %q", i, route.Auth.Provider)
			}
		}
		if route.Cache.Enabled && (!c.Cache.Enabled || route.Cache.TTL <= 0) {
			return fmt.Errorf("route %d cache requires global cache and positive ttl", i)
		}
	}
	return nil
}

func (c Config) validateRateLimits() error {
	if c.RateLimit.OnBackendError != "allow" && c.RateLimit.OnBackendError != "deny" {
		return errors.New("rate_limit on_backend_error must be allow or deny")
	}
	if c.RateLimit.OperationTimeout <= 0 {
		return errors.New("rate_limit operation_timeout must be positive")
	}
	if c.RateLimit.Enabled {
		if c.RateLimit.DefaultBackend == "" {
			return errors.New("enabled rate_limit requires a default backend")
		}
		if c.RateLimit.DefaultBackend == "redis" &&
			(c.RateLimit.Redis.Address == "" || c.RateLimit.Redis.KeyPrefix == "") {
			return errors.New("Redis rate limit backend requires address and key prefix")
		}
	}
	seen := make(map[string]struct{})
	validate := func(rule RateLimitRule) error {
		if rule.Name == "" || rule.Key == "" || rule.Filter == "" {
			return errors.New("rate limit rule name, key, and filter are required")
		}
		if _, ok := seen[rule.Name]; ok {
			return fmt.Errorf("duplicate rate limit rule %q", rule.Name)
		}
		seen[rule.Name] = struct{}{}
		if rule.TokenBucket.RequestsPerSecond <= 0 || rule.TokenBucket.Burst <= 0 || rule.TokenBucket.TTL <= 0 {
			return fmt.Errorf("rate limit rule %q token bucket values must be positive", rule.Name)
		}
		return nil
	}
	for _, rule := range c.RateLimit.Rules {
		if err := validate(rule); err != nil {
			return err
		}
	}
	for _, route := range c.Routes {
		for _, rule := range route.RateLimits {
			if err := validate(rule); err != nil {
				return err
			}
		}
	}
	return nil
}

func applyEnvironment(c *Config) error {
	setString("GATEWAY_SERVER_ADDRESS", &c.Server.Address)
	setString("GATEWAY_ADMIN_ADDRESS", &c.Admin.Address)
	setString("GATEWAY_LOG_LEVEL", &c.Log.Level)
	setString("GATEWAY_LOG_FORMAT", &c.Log.Format)
	setString("GATEWAY_TELEMETRY_TRACING_SERVICE_NAME", &c.Telemetry.Tracing.ServiceName)
	setString("GATEWAY_TELEMETRY_TRACING_ENDPOINT", &c.Telemetry.Tracing.Endpoint)
	if err := setBool("GATEWAY_TELEMETRY_TRACING_ENABLED", &c.Telemetry.Tracing.Enabled); err != nil {
		return err
	}
	if err := setBool("GATEWAY_TELEMETRY_TRACING_INSECURE", &c.Telemetry.Tracing.Insecure); err != nil {
		return err
	}
	if err := setFloat("GATEWAY_TELEMETRY_TRACING_SAMPLE_RATIO", &c.Telemetry.Tracing.SampleRatio); err != nil {
		return err
	}
	durations := map[string]*time.Duration{
		"GATEWAY_SERVER_READ_TIMEOUT":                &c.Server.ReadTimeout,
		"GATEWAY_SERVER_WRITE_TIMEOUT":               &c.Server.WriteTimeout,
		"GATEWAY_SERVER_IDLE_TIMEOUT":                &c.Server.IdleTimeout,
		"GATEWAY_SERVER_SHUTDOWN_TIMEOUT":            &c.Server.ShutdownTimeout,
		"GATEWAY_MIDDLEWARE_REQUEST_TIMEOUT":         &c.Middleware.RequestTimeout,
		"GATEWAY_TELEMETRY_TRACING_SHUTDOWN_TIMEOUT": &c.Telemetry.Tracing.ShutdownTimeout,
	}
	for key, target := range durations {
		if err := setDuration(key, target); err != nil {
			return err
		}
	}
	return nil
}

func setBool(key string, target *bool) error {
	if value, ok := os.LookupEnv(key); ok {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		*target = parsed
	}
	return nil
}

func setFloat(key string, target *float64) error {
	if value, ok := os.LookupEnv(key); ok {
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		*target = parsed
	}
	return nil
}

func setString(key string, target *string) {
	if value, ok := os.LookupEnv(key); ok {
		*target = value
	}
}

func setDuration(key string, target *time.Duration) error {
	if value, ok := os.LookupEnv(key); ok {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		*target = parsed
	}
	return nil
}

func (d *HTTPServer) UnmarshalYAML(value *yaml.Node) error {
	if err := rejectUnknownFields(value, "address", "read_timeout", "write_timeout", "idle_timeout", "shutdown_timeout"); err != nil {
		return err
	}
	type plain struct {
		Address         string `yaml:"address"`
		ReadTimeout     string `yaml:"read_timeout"`
		WriteTimeout    string `yaml:"write_timeout"`
		IdleTimeout     string `yaml:"idle_timeout"`
		ShutdownTimeout string `yaml:"shutdown_timeout"`
	}
	var p plain
	if err := value.Decode(&p); err != nil {
		return err
	}
	if p.Address != "" {
		d.Address = p.Address
	}
	return parseDurations(map[string]struct {
		raw string
		dst *time.Duration
	}{
		"read_timeout": {p.ReadTimeout, &d.ReadTimeout}, "write_timeout": {p.WriteTimeout, &d.WriteTimeout},
		"idle_timeout": {p.IdleTimeout, &d.IdleTimeout}, "shutdown_timeout": {p.ShutdownTimeout, &d.ShutdownTimeout},
	})
}

func (m *Middleware) UnmarshalYAML(value *yaml.Node) error {
	if err := rejectUnknownFields(value, "request_timeout", "cors"); err != nil {
		return err
	}
	type plain struct {
		RequestTimeout string `yaml:"request_timeout"`
		CORS           CORS   `yaml:"cors"`
	}
	var p plain
	if err := value.Decode(&p); err != nil {
		return err
	}
	m.CORS = p.CORS
	if p.RequestTimeout == "" {
		return nil
	}
	duration, err := time.ParseDuration(p.RequestTimeout)
	if err != nil {
		return fmt.Errorf("request_timeout: %w", err)
	}
	m.RequestTimeout = duration
	return nil
}

func (r *Retry) UnmarshalYAML(value *yaml.Node) error {
	if err := rejectUnknownFields(value, "max_attempts", "per_attempt_timeout", "backoff", "statuses"); err != nil {
		return err
	}
	type plain struct {
		MaxAttempts       *int   `yaml:"max_attempts"`
		PerAttemptTimeout string `yaml:"per_attempt_timeout"`
		Backoff           string `yaml:"backoff"`
		Statuses          []int  `yaml:"statuses"`
	}
	var p plain
	if err := value.Decode(&p); err != nil {
		return err
	}
	if p.MaxAttempts != nil {
		r.MaxAttempts = *p.MaxAttempts
	}
	if p.Statuses != nil {
		r.Statuses = p.Statuses
	}
	return parseDurations(map[string]struct {
		raw string
		dst *time.Duration
	}{
		"per_attempt_timeout": {p.PerAttemptTimeout, &r.PerAttemptTimeout},
		"backoff":             {p.Backoff, &r.Backoff},
	})
}

func (cb *CircuitBreaker) UnmarshalYAML(value *yaml.Node) error {
	if err := rejectUnknownFields(value, "failure_threshold", "open_timeout", "failure_statuses"); err != nil {
		return err
	}
	type plain struct {
		FailureThreshold *int   `yaml:"failure_threshold"`
		OpenTimeout      string `yaml:"open_timeout"`
		FailureStatuses  []int  `yaml:"failure_statuses"`
	}
	var p plain
	if err := value.Decode(&p); err != nil {
		return err
	}
	if p.FailureThreshold != nil {
		cb.FailureThreshold = *p.FailureThreshold
	}
	if p.FailureStatuses != nil {
		cb.FailureStatuses = p.FailureStatuses
	}
	if p.OpenTimeout != "" {
		duration, err := time.ParseDuration(p.OpenTimeout)
		if err != nil {
			return fmt.Errorf("open_timeout: %w", err)
		}
		cb.OpenTimeout = duration
	}
	return nil
}

func (r *RateLimit) UnmarshalYAML(value *yaml.Node) error {
	if err := rejectUnknownFields(value, "enabled", "default_backend", "on_backend_error", "operation_timeout", "redis", "rules"); err != nil {
		return err
	}
	type plain struct {
		Enabled          bool            `yaml:"enabled"`
		DefaultBackend   string          `yaml:"default_backend"`
		OnBackendError   string          `yaml:"on_backend_error"`
		OperationTimeout string          `yaml:"operation_timeout"`
		Redis            Redis           `yaml:"redis"`
		Rules            []RateLimitRule `yaml:"rules"`
	}
	var p plain
	if err := value.Decode(&p); err != nil {
		return err
	}
	r.Enabled = p.Enabled
	if p.DefaultBackend != "" {
		r.DefaultBackend = p.DefaultBackend
	}
	if p.OnBackendError != "" {
		r.OnBackendError = p.OnBackendError
	}
	if p.Redis.Address != "" {
		r.Redis = p.Redis
	}
	r.Rules = p.Rules
	if p.OperationTimeout != "" {
		duration, err := time.ParseDuration(p.OperationTimeout)
		if err != nil {
			return fmt.Errorf("operation_timeout: %w", err)
		}
		r.OperationTimeout = duration
	}
	return nil
}

func (r *RateLimitRule) UnmarshalYAML(value *yaml.Node) error {
	if err := rejectUnknownFields(value, "name", "backend", "key", "filter", "token_bucket"); err != nil {
		return err
	}
	type plain RateLimitRule
	return value.Decode((*plain)(r))
}

func (a *Auth) UnmarshalYAML(value *yaml.Node) error {
	if err := rejectUnknownFields(value, "providers"); err != nil {
		return err
	}
	type plain Auth
	return value.Decode((*plain)(a))
}

func (p *JWTProvider) UnmarshalYAML(value *yaml.Node) error {
	if err := rejectUnknownFields(value, "jwks_url", "issuer", "audience", "algorithms", "clock_skew", "http_timeout"); err != nil {
		return err
	}
	var v struct {
		JWKSURL     string   `yaml:"jwks_url"`
		Issuer      string   `yaml:"issuer"`
		Audience    string   `yaml:"audience"`
		Algorithms  []string `yaml:"algorithms"`
		ClockSkew   string   `yaml:"clock_skew"`
		HTTPTimeout string   `yaml:"http_timeout"`
	}
	if err := value.Decode(&v); err != nil {
		return err
	}
	p.JWKSURL, p.Issuer, p.Audience, p.Algorithms = v.JWKSURL, v.Issuer, v.Audience, v.Algorithms
	return parseDurations(map[string]struct {
		raw string
		dst *time.Duration
	}{
		"clock_skew": {v.ClockSkew, &p.ClockSkew}, "http_timeout": {v.HTTPTimeout, &p.HTTPTimeout},
	})
}

func (a *RouteAuth) UnmarshalYAML(value *yaml.Node) error {
	if err := rejectUnknownFields(value, "required", "provider", "required_scopes"); err != nil {
		return err
	}
	type plain RouteAuth
	return value.Decode((*plain)(a))
}

func (c *Cache) UnmarshalYAML(value *yaml.Node) error {
	if err := rejectUnknownFields(value, "enabled", "on_backend_error", "operation_timeout", "max_body_bytes", "redis"); err != nil {
		return err
	}
	type raw struct {
		Enabled          bool   `yaml:"enabled"`
		OnBackendError   string `yaml:"on_backend_error"`
		OperationTimeout string `yaml:"operation_timeout"`
		MaxBodyBytes     int64  `yaml:"max_body_bytes"`
		Redis            Redis  `yaml:"redis"`
	}
	var v raw
	if err := value.Decode(&v); err != nil {
		return err
	}
	c.Enabled = v.Enabled
	if v.OnBackendError != "" {
		c.OnBackendError = v.OnBackendError
	}
	if v.MaxBodyBytes != 0 {
		c.MaxBodyBytes = v.MaxBodyBytes
	}
	if v.Redis.Address != "" {
		c.Redis = v.Redis
	}
	if v.OperationTimeout != "" {
		d, err := time.ParseDuration(v.OperationTimeout)
		if err != nil {
			return fmt.Errorf("operation_timeout: %w", err)
		}
		c.OperationTimeout = d
	}
	return nil
}

func (c *RouteCache) UnmarshalYAML(value *yaml.Node) error {
	if err := rejectUnknownFields(value, "enabled", "ttl", "vary_headers"); err != nil {
		return err
	}
	type raw struct {
		Enabled     bool     `yaml:"enabled"`
		TTL         string   `yaml:"ttl"`
		VaryHeaders []string `yaml:"vary_headers"`
	}
	var v raw
	if err := value.Decode(&v); err != nil {
		return err
	}
	c.Enabled, c.VaryHeaders = v.Enabled, v.VaryHeaders
	if v.TTL != "" {
		d, err := time.ParseDuration(v.TTL)
		if err != nil {
			return fmt.Errorf("ttl: %w", err)
		}
		c.TTL = d
	}
	return nil
}

func (t *TokenBucket) UnmarshalYAML(value *yaml.Node) error {
	if err := rejectUnknownFields(value, "requests_per_second", "burst", "ttl"); err != nil {
		return err
	}
	type plain struct {
		RequestsPerSecond float64 `yaml:"requests_per_second"`
		Burst             int     `yaml:"burst"`
		TTL               string  `yaml:"ttl"`
	}
	var p plain
	if err := value.Decode(&p); err != nil {
		return err
	}
	t.RequestsPerSecond, t.Burst = p.RequestsPerSecond, p.Burst
	if p.TTL != "" {
		duration, err := time.ParseDuration(p.TTL)
		if err != nil {
			return fmt.Errorf("ttl: %w", err)
		}
		t.TTL = duration
	}
	return nil
}

func validateStatuses(name string, statuses []int) error {
	if len(statuses) == 0 {
		return fmt.Errorf("%s statuses must not be empty", name)
	}
	seen := make(map[int]struct{}, len(statuses))
	for _, status := range statuses {
		if status < 500 || status > 599 {
			return fmt.Errorf("%s status %d must be a 5xx status", name, status)
		}
		if _, exists := seen[status]; exists {
			return fmt.Errorf("duplicate %s status %d", name, status)
		}
		seen[status] = struct{}{}
	}
	return nil
}

func rejectUnknownFields(value *yaml.Node, allowed ...string) error {
	if value.Kind != yaml.MappingNode {
		return errors.New("expected a YAML mapping")
	}
	known := make(map[string]struct{}, len(allowed))
	for _, field := range allowed {
		known[field] = struct{}{}
	}
	for i := 0; i < len(value.Content); i += 2 {
		field := value.Content[i]
		if _, ok := known[field.Value]; !ok {
			return fmt.Errorf("line %d: field %s not found", field.Line, field.Value)
		}
	}
	return nil
}

func parseDurations(values map[string]struct {
	raw string
	dst *time.Duration
},
) error {
	for name, item := range values {
		if item.raw == "" {
			continue
		}
		duration, err := time.ParseDuration(item.raw)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		*item.dst = duration
	}
	return nil
}
