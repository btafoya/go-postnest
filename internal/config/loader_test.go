package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoader_Load_FromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "postnest.conf")

	content := `
config_version = 1

[server]
http_addr = ":9090"

[database]
dsn = "postgres://file@localhost/file"

[security]
session_key = "file-secret"
`
	if err := os.WriteFile(path, []byte(content), 0640); err != nil {
		t.Fatalf("write config: %v", err)
	}

	loader := NewLoader(path)
	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if cfg.HTTPAddr != ":9090" {
		t.Errorf("HTTPAddr = %q, want :9090", cfg.HTTPAddr)
	}
	if cfg.PostgresDSN != "postgres://file@localhost/file" {
		t.Errorf("PostgresDSN = %q, want file DSN", cfg.PostgresDSN)
	}
	if cfg.SessionKey != "file-secret" {
		t.Errorf("SessionKey = %q, want file-secret", cfg.SessionKey)
	}
}

func TestLoader_Load_EnvOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "postnest.conf")

	content := `
config_version = 1

[database]
dsn = "postgres://file@localhost/file"

[security]
session_key = "file-secret"
`
	if err := os.WriteFile(path, []byte(content), 0640); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("POSTNEST_DATABASE_DSN", "postgres://env@localhost/env")
	t.Setenv("POSTNEST_SECURITY_SESSION_KEY", "env-secret")

	loader := NewLoader(path)
	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if cfg.PostgresDSN != "postgres://env@localhost/env" {
		t.Errorf("PostgresDSN = %q, want env DSN", cfg.PostgresDSN)
	}
	if cfg.SessionKey != "env-secret" {
		t.Errorf("SessionKey = %q, want env-secret", cfg.SessionKey)
	}
}

func TestLoader_Load_LegacyEnv(t *testing.T) {
	t.Setenv("POSTGRES_DSN", "postgres://legacy@localhost/legacy")
	t.Setenv("SESSION_KEY", "legacy-secret")

	loader := NewLoader("/nonexistent/path.conf")
	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if cfg.PostgresDSN != "postgres://legacy@localhost/legacy" {
		t.Errorf("PostgresDSN = %q, want legacy DSN", cfg.PostgresDSN)
	}
	if cfg.SessionKey != "legacy-secret" {
		t.Errorf("SessionKey = %q, want legacy-secret", cfg.SessionKey)
	}
}

func TestLoader_Load_MissingRequired(t *testing.T) {
	loader := NewLoader("/nonexistent/path.conf")
	_, err := loader.Load()
	if err == nil {
		t.Fatal("expected error for missing required fields")
	}
}
