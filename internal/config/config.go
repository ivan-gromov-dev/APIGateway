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
	PathPrefix  string   `yaml:"path_prefix"`
	Upstreams   []string `yaml:"upstreams"`
	StripPrefix bool     `yaml:"strip_prefix"`
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
