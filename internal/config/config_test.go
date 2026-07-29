package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestValidateRejectsInvalidUpstream(t *testing.T) {
	cfg := defaults()
	cfg.Routes = []Route{{PathPrefix: "/api/", Upstreams: []string{"ftp://example.com"}}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	tests := map[string]string{
		"root":       "unknown: true\n",
		"server":     "server:\n  unknown: true\n",
		"middleware": "middleware:\n  unknown: true\n",
		"cors":       "middleware:\n  cors:\n    unknown: true\n",
		"route":      "routes:\n  - path_prefix: /api/\n    upstreams: [http://localhost:8081]\n    unknown: true\n",
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
