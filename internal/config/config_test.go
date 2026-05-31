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

func TestResolveReadsRemoteSettingsFromToml(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	if err := os.MkdirAll(filepath.Join(configDir, "taskg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "taskg", "taskg.toml"), []byte(strings.Join([]string{
		"[remote]",
		`server = "http://127.0.0.1:8080"`,
		`token = "taskg_pat_toml"`,
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Resolve(Options{
		Env:     map[string]string{"XDG_CONFIG_HOME": configDir},
		HomeDir: filepath.Join(dir, "home"),
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if cfg.RemoteServer != "http://127.0.0.1:8080" {
		t.Fatalf("RemoteServer = %q", cfg.RemoteServer)
	}
	if cfg.RemoteToken != "taskg_pat_toml" {
		t.Fatalf("RemoteToken = %q", cfg.RemoteToken)
	}
}

func TestResolveRemoteEnvOverridesToml(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	if err := os.MkdirAll(filepath.Join(configDir, "taskg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "taskg", "taskg.toml"), []byte(strings.Join([]string{
		"[remote]",
		`server = "http://127.0.0.1:8080"`,
		`token = "taskg_pat_toml"`,
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Resolve(Options{
		Env: map[string]string{
			"XDG_CONFIG_HOME": configDir,
			"TASKG_SERVER":    "http://env.example",
			"TASKG_TOKEN":     "taskg_pat_env",
		},
		HomeDir: filepath.Join(dir, "home"),
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if cfg.RemoteServer != "http://env.example" {
		t.Fatalf("RemoteServer = %q", cfg.RemoteServer)
	}
	if cfg.RemoteToken != "taskg_pat_env" {
		t.Fatalf("RemoteToken = %q", cfg.RemoteToken)
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

func TestLoadRuntimeReadsRemoteSettingsFromToml(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "taskg.toml"), []byte(strings.Join([]string{
		"[remote]",
		`server = "http://127.0.0.1:8080"`,
		`token = "taskg_pat_toml"`,
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	rt, err := LoadRuntime(RuntimeOptions{ConfigDir: configDir})
	if err != nil {
		t.Fatalf("LoadRuntime() error = %v", err)
	}
	if got, _ := rt.Get("remote.server"); got != "http://127.0.0.1:8080" {
		t.Fatalf("remote.server = %q", got)
	}
	if got, _ := rt.Get("remote.token"); got != "taskg_pat_toml" {
		t.Fatalf("remote.token = %q", got)
	}
}

func TestLoadRuntimeEnvOverridesRemoteToml(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "taskg.toml"), []byte(strings.Join([]string{
		"[remote]",
		`server = "http://127.0.0.1:8080"`,
		`token = "taskg_pat_toml"`,
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	rt, err := LoadRuntime(RuntimeOptions{
		ConfigDir: configDir,
		Env: map[string]string{
			"TASKG_SERVER": "http://env.example",
			"TASKG_TOKEN":  "taskg_pat_env",
		},
	})
	if err != nil {
		t.Fatalf("LoadRuntime() error = %v", err)
	}
	if got, _ := rt.Get("remote.server"); got != "http://env.example" {
		t.Fatalf("remote.server = %q", got)
	}
	if got, _ := rt.Get("remote.token"); got != "taskg_pat_env" {
		t.Fatalf("remote.token = %q", got)
	}
}

func TestLoadRuntimeMapsDisplayTomlKeys(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "taskg.toml"), []byte(strings.Join([]string{
		"[display]",
		"color = false",
		"json = true",
		"[date]",
		"format = \"epoch\"",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	rt, err := LoadRuntime(RuntimeOptions{ConfigDir: configDir})
	if err != nil {
		t.Fatalf("LoadRuntime() error = %v", err)
	}
	for key, want := range map[string]string{
		"color":       "false",
		"json":        "true",
		"date.format": "epoch",
	} {
		got, _ := rt.Get(key)
		if got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestLoadRuntimePreservesHashInsideQuotedTomlValue(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "taskg.toml"), []byte(strings.Join([]string{
		`[context]`,
		`active = "work#alpha" # trailing comment`,
		`[uda.ticket]`,
		`label = "fix #1234"`,
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	rt, err := LoadRuntime(RuntimeOptions{ConfigDir: configDir})
	if err != nil {
		t.Fatalf("LoadRuntime() error = %v", err)
	}
	if got, _ := rt.Get("context.active"); got != "work#alpha" {
		t.Fatalf("context.active = %q, want work#alpha", got)
	}
	if got, _ := rt.Get("uda.ticket.label"); got != "fix #1234" {
		t.Fatalf("uda.ticket.label = %q, want quoted hash preserved", got)
	}
}

func TestLoadRuntimeHandlesSectionCommentsAndArrayValues(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "taskg.toml"), []byte(strings.Join([]string{
		`[context] # active context section`,
		`active = "work"`,
		`[uda.estimate] # estimate schema`,
		`values = ["1", "2", "3"]`,
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	rt, err := LoadRuntime(RuntimeOptions{ConfigDir: configDir})
	if err != nil {
		t.Fatalf("LoadRuntime() error = %v", err)
	}
	if got, _ := rt.Get("context.active"); got != "work" {
		t.Fatalf("context.active = %q, want work", got)
	}
	if got, _ := rt.Get("uda.estimate.values"); got != "1,2,3" {
		t.Fatalf("uda.estimate.values = %q, want 1,2,3", got)
	}
}

func TestLoadRuntimeSupportsTomlMultilineStrings(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "taskg.toml"), []byte(strings.Join([]string{
		`display.color = false`,
		`[uda.ticket]`,
		`label = """fix #1234`,
		`second line"""`,
		`values = ["1", "2", "3"]`,
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	rt, err := LoadRuntime(RuntimeOptions{ConfigDir: configDir})
	if err != nil {
		t.Fatalf("LoadRuntime() error = %v", err)
	}
	if got, _ := rt.Get("color"); got != "false" {
		t.Fatalf("color = %q, want false", got)
	}
	if got, _ := rt.Get("uda.ticket.label"); got != "fix #1234\nsecond line" {
		t.Fatalf("uda.ticket.label = %q, want multiline TOML string", got)
	}
	if got, _ := rt.Get("uda.ticket.values"); got != "1,2,3" {
		t.Fatalf("uda.ticket.values = %q, want 1,2,3", got)
	}
}

func TestRemoteTokenPermissionWarningWarnsForWidePermissions(t *testing.T) {
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "taskg.toml")
	if err := os.WriteFile(configPath, []byte(strings.Join([]string{
		`[remote]`,
		`token = "taskg_pat_toml"`,
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	warning, err := RemoteTokenPermissionWarning(configDir)
	if err != nil {
		t.Fatalf("RemoteTokenPermissionWarning() error = %v", err)
	}
	if warning == "" {
		t.Fatal("RemoteTokenPermissionWarning() = empty, want warning")
	}
	for _, want := range []string{"remote.token", "0600", configPath} {
		if !strings.Contains(warning, want) {
			t.Fatalf("warning = %q, want substring %q", warning, want)
		}
	}
}

func TestRemoteTokenPermissionWarningAllows0600(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "taskg.toml"), []byte(strings.Join([]string{
		`[remote]`,
		`token = "taskg_pat_toml"`,
		"",
	}, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}

	warning, err := RemoteTokenPermissionWarning(configDir)
	if err != nil {
		t.Fatalf("RemoteTokenPermissionWarning() error = %v", err)
	}
	if warning != "" {
		t.Fatalf("warning = %q, want empty", warning)
	}
}

func TestRemoteTokenPermissionWarningIgnoresFilesWithoutToken(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "taskg.toml"), []byte(strings.Join([]string{
		`[remote]`,
		`server = "http://127.0.0.1:8080"`,
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	warning, err := RemoteTokenPermissionWarning(configDir)
	if err != nil {
		t.Fatalf("RemoteTokenPermissionWarning() error = %v", err)
	}
	if warning != "" {
		t.Fatalf("warning = %q, want empty", warning)
	}
}

func stringPtr(v string) *string {
	return &v
}
