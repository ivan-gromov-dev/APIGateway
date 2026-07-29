package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	t.Setenv("GATEWAY_SERVER_ADDRESS", ":8888")
	path := filepath.Join(t.TempDir(), "gateway.yaml")
	data := []byte("admin:\n  address: ':9090'\nroutes:\n  - path_prefix: /api/\n    upstream: http://localhost:8081\n")
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
	cfg.Routes = []Route{{PathPrefix: "/api/", Upstream: "ftp://example.com"}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}
