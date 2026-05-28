package config

import (
	"path/filepath"
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
