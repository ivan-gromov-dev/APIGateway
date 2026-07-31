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
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server         HTTPServer     `yaml:"server"`
	Admin          HTTPServer     `yaml:"admin"`
	Log            Log            `yaml:"log"`
	Middleware     Middleware     `yaml:"middleware"`
	Retry          Retry          `yaml:"retry"`
	Routes         []Route        `yaml:"routes"`
	CircuitBreaker CircuitBreaker `yaml:"circuit_breaker"`
	RateLimit      RateLimit      `yaml:"rate_limit"`
	Auth           Auth           `yaml:"auth"`
	Cache          Cache          `yaml:"cache"`
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
	StripPrefix bool            `yaml:"strip_prefix"`
	RateLimits  []RateLimitRule `yaml:"rate_limits"`
	Auth        RouteAuth       `yaml:"auth"`
	Cache       RouteCache      `yaml:"cache"`
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
		RateLimit: RateLimit{
			DefaultBackend: "redis", OnBackendError: "allow",
			OperationTimeout: 100 * time.Millisecond,
			Redis:            Redis{Address: "localhost:6379", KeyPrefix: "gateway:ratelimit"},
		},
		Cache: Cache{OnBackendError: "allow", OperationTimeout: 100 * time.Millisecond,
			MaxBodyBytes: 1 << 20, Redis: Redis{Address: "localhost:6379", KeyPrefix: "gateway:cache"}},
	}
}

func (c Config) Validate() error {
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
	if len(c.Routes) == 0 {
		return errors.New("at least one route is required")
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
			return fmt.Errorf("route %d: at least one upstream is required", i)
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
	durations := map[string]*time.Duration{
		"GATEWAY_SERVER_READ_TIMEOUT":        &c.Server.ReadTimeout,
		"GATEWAY_SERVER_WRITE_TIMEOUT":       &c.Server.WriteTimeout,
		"GATEWAY_SERVER_IDLE_TIMEOUT":        &c.Server.IdleTimeout,
		"GATEWAY_SERVER_SHUTDOWN_TIMEOUT":    &c.Server.ShutdownTimeout,
		"GATEWAY_MIDDLEWARE_REQUEST_TIMEOUT": &c.Middleware.RequestTimeout,
	}
	for key, target := range durations {
		if err := setDuration(key, target); err != nil {
			return err
		}
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
