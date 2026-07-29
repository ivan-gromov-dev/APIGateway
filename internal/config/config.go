// Package config loads, validates, and exposes the gateway's runtime configuration.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server     HTTPServer `yaml:"server"`
	Admin      HTTPServer `yaml:"admin"`
	Log        Log        `yaml:"log"`
	Middleware Middleware `yaml:"middleware"`
	Routes     []Route    `yaml:"routes"`
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

type Route struct {
	PathPrefix  string `yaml:"path_prefix"`
	Upstream    string `yaml:"upstream"`
	StripPrefix bool   `yaml:"strip_prefix"`
}

func Load(path string) (Config, error) {
	cfg := defaults()
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	applyEnvironment(&cfg)
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
		Middleware: Middleware{
			RequestTimeout: 15 * time.Second,
			CORS: CORS{
				AllowedOrigins: []string{"*"},
				AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
				AllowedHeaders: []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
			},
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
		if !strings.HasPrefix(route.Upstream, "http://") && !strings.HasPrefix(route.Upstream, "https://") {
			return fmt.Errorf("route %d: upstream must use http or https", i)
		}
	}
	return nil
}

func applyEnvironment(c *Config) {
	setString("GATEWAY_SERVER_ADDRESS", &c.Server.Address)
	setString("GATEWAY_ADMIN_ADDRESS", &c.Admin.Address)
	setString("GATEWAY_LOG_LEVEL", &c.Log.Level)
	setString("GATEWAY_LOG_FORMAT", &c.Log.Format)
	setDuration("GATEWAY_SERVER_READ_TIMEOUT", &c.Server.ReadTimeout)
	setDuration("GATEWAY_SERVER_WRITE_TIMEOUT", &c.Server.WriteTimeout)
	setDuration("GATEWAY_SERVER_IDLE_TIMEOUT", &c.Server.IdleTimeout)
	setDuration("GATEWAY_SERVER_SHUTDOWN_TIMEOUT", &c.Server.ShutdownTimeout)
	setDuration("GATEWAY_MIDDLEWARE_REQUEST_TIMEOUT", &c.Middleware.RequestTimeout)
}

func setString(key string, target *string) {
	if value, ok := os.LookupEnv(key); ok {
		*target = value
	}
}

func setDuration(key string, target *time.Duration) {
	if value, ok := os.LookupEnv(key); ok {
		if parsed, err := time.ParseDuration(value); err == nil {
			*target = parsed
		}
	}
}

func (d *HTTPServer) UnmarshalYAML(value *yaml.Node) error {
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
