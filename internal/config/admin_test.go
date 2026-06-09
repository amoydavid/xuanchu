package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveServerAdminConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xuanchu.toml")
	err := os.WriteFile(path, []byte(`
[server.admin]
enabled = true

[[server.admin.tokens]]
name = "ops-primary"
hash = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
enabled = true
description = "primary"

[[server.admin.tokens]]
name = "ops-rotation"
hash_env = "XUANCHU_ADMIN_TOKEN_HASH"
enabled = true
`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := Resolve(Options{
		ConfigPath: path,
		HomeDir:    dir,
		Env: map[string]string{
			"XUANCHU_ADMIN_TOKEN_HASH": "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if !cfg.ServerAdmin.Enabled {
		t.Fatal("ServerAdmin.Enabled = false")
	}
	if len(cfg.ServerAdmin.Tokens) != 2 {
		t.Fatalf("tokens = %#v, want 2", cfg.ServerAdmin.Tokens)
	}
	if cfg.ServerAdmin.Tokens[1].Hash != "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("hash_env not resolved: %#v", cfg.ServerAdmin.Tokens[1])
	}
}

func TestResolveServerAdminRejectsDuplicateTokenName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xuanchu.toml")
	err := os.WriteFile(path, []byte(`
[server.admin]
enabled = true

[[server.admin.tokens]]
name = "ops"
hash = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

[[server.admin.tokens]]
name = "ops"
hash = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Resolve(Options{ConfigPath: path, HomeDir: dir, Env: map[string]string{}})
	if err == nil || !strings.Contains(err.Error(), "duplicate admin token name") {
		t.Fatalf("Resolve() err = %v, want duplicate admin token name", err)
	}
}

func TestResolveServerAdminRejectsMissingHashEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xuanchu.toml")
	err := os.WriteFile(path, []byte(`
[server.admin]
enabled = true

[[server.admin.tokens]]
name = "ops"
hash_env = "MISSING_HASH"
`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Resolve(Options{ConfigPath: path, HomeDir: dir, Env: map[string]string{}})
	if err == nil || !strings.Contains(err.Error(), "MISSING_HASH") {
		t.Fatalf("Resolve() err = %v, want missing hash env", err)
	}
}

func TestResolveServerAdminRejectsMalformedHash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xuanchu.toml")
	err := os.WriteFile(path, []byte(`
[server.admin]
enabled = true

[[server.admin.tokens]]
name = "ops"
hash = "sha256:not-hex"
`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Resolve(Options{ConfigPath: path, HomeDir: dir, Env: map[string]string{}})
	if err == nil || !strings.Contains(err.Error(), "invalid hash") {
		t.Fatalf("Resolve() err = %v, want invalid hash", err)
	}
}

func TestResolveServerAdminHashEnvUsesProcessEnvironment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xuanchu.toml")
	err := os.WriteFile(path, []byte(`
[server.admin]
enabled = true

[[server.admin.tokens]]
name = "ops"
hash_env = "XUANCHU_ADMIN_TOKEN_HASH"
`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("XUANCHU_ADMIN_TOKEN_HASH", "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc")
	cfg, err := Resolve(Options{ConfigPath: path, HomeDir: dir})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got := cfg.ServerAdmin.Tokens[0].Hash; got != "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc" {
		t.Fatalf("hash = %q", got)
	}
}
