package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveDatabasePathPrefersExplicitDB(t *testing.T) {
	env := map[string]string{
		"TASKG_DB": "/env/taskg.db",
	}
	cfg, err := Resolve(Options{
		DBPath:  "/explicit/taskg.db",
		Env:     env,
		HomeDir: "/home/alice",
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if cfg.DatabasePath != "/explicit/taskg.db" {
		t.Fatalf("DatabasePath = %q", cfg.DatabasePath)
	}
}

func TestResolveDatabasePathUsesTaskgDB(t *testing.T) {
	cfg, err := Resolve(Options{
		Env:     map[string]string{"TASKG_DB": "/env/taskg.db"},
		HomeDir: "/home/alice",
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if cfg.DatabasePath != "/env/taskg.db" {
		t.Fatalf("DatabasePath = %q", cfg.DatabasePath)
	}
}

func TestResolveDatabasePathUsesXDGDataHome(t *testing.T) {
	cfg, err := Resolve(Options{
		Env:     map[string]string{"XDG_DATA_HOME": "/xdg"},
		HomeDir: "/home/alice",
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	want := filepath.Join("/xdg", "taskg", "taskg.db")
	if cfg.DatabasePath != want {
		t.Fatalf("DatabasePath = %q, want %q", cfg.DatabasePath, want)
	}
}

func TestResolveDatabasePathUsesHomeFallback(t *testing.T) {
	cfg, err := Resolve(Options{
		Env:     map[string]string{},
		HomeDir: "/home/alice",
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	want := filepath.Join("/home/alice", ".local", "share", "taskg", "taskg.db")
	if cfg.DatabasePath != want {
		t.Fatalf("DatabasePath = %q, want %q", cfg.DatabasePath, want)
	}
}

func TestResolveDatabasePathUsesToml(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	if err := os.MkdirAll(filepath.Join(configDir, "taskg"), 0o755); err != nil {
		t.Fatal(err)
	}
	tomlDB := filepath.Join(dir, "toml.db")
	if err := os.WriteFile(filepath.Join(configDir, "taskg", "taskg.toml"), []byte("database.path = \""+tomlDB+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Resolve(Options{
		Env:     map[string]string{"XDG_CONFIG_HOME": configDir},
		HomeDir: filepath.Join(dir, "home"),
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if cfg.DatabasePath != tomlDB {
		t.Fatalf("DatabasePath = %q, want %q", cfg.DatabasePath, tomlDB)
	}
}

func TestRuntimeMergesSourcesAndRcOverrides(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	if err := os.MkdirAll(filepath.Join(configDir, "taskg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "taskg", "taskg.toml"), []byte(strings.Join([]string{
		"color = false",
		"date.format = \"epoch\"",
		"[context]",
		"active = \"work\"",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	rt, err := LoadRuntime(RuntimeOptions{
		ConfigDir: configDir,
		Meta: map[string]string{
			"date.format": "rfc3339",
		},
		RCOverrides: map[string]*string{
			"color":          stringPtr("true"),
			"context.active": nil,
		},
	})
	if err != nil {
		t.Fatalf("LoadRuntime() error = %v", err)
	}
	if got, _ := rt.Get("date.format"); got != "rfc3339" {
		t.Fatalf("date.format = %q", got)
	}
	if got, _ := rt.Get("color"); got != "true" {
		t.Fatalf("color = %q", got)
	}
	if got, _ := rt.Get("context.active"); got != "" {
		t.Fatalf("context.active = %q, want empty", got)
	}
}

func stringPtr(v string) *string {
	return &v
}
