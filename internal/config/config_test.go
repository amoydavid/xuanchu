package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolveDatabasePathPrefersExplicitDB(t *testing.T) {
	env := map[string]string{
		"XUANCHU_DB": "/env/xuanchu.db",
	}
	cfg, err := Resolve(Options{
		DBPath:  "/explicit/xuanchu.db",
		Env:     env,
		HomeDir: "/home/alice",
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if cfg.DatabasePath != "/explicit/xuanchu.db" {
		t.Fatalf("DatabasePath = %q", cfg.DatabasePath)
	}
}

func TestResolveDatabasePathUsesXuanchuDB(t *testing.T) {
	cfg, err := Resolve(Options{
		Env:     map[string]string{"XUANCHU_DB": "/env/xuanchu.db"},
		HomeDir: "/home/alice",
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if cfg.DatabasePath != "/env/xuanchu.db" {
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
	want := filepath.Join("/xdg", "xuanchu", "xuanchu.db")
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
	want := filepath.Join("/home/alice", ".local", "share", "xuanchu", "xuanchu.db")
	if cfg.DatabasePath != want {
		t.Fatalf("DatabasePath = %q, want %q", cfg.DatabasePath, want)
	}
}

func TestResolveDatabasePathUsesToml(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	if err := os.MkdirAll(filepath.Join(configDir, "xuanchu"), 0o755); err != nil {
		t.Fatal(err)
	}
	tomlDB := filepath.Join(dir, "toml.db")
	if err := os.WriteFile(filepath.Join(configDir, "xuanchu", "xuanchu.toml"), []byte("database.path = \""+tomlDB+"\"\n"), 0o644); err != nil {
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
	if err := os.MkdirAll(filepath.Join(configDir, "xuanchu"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "xuanchu", "xuanchu.toml"), []byte(strings.Join([]string{
		"[remote]",
		`server = "http://127.0.0.1:8080"`,
		`token = "xuanchu_pat_toml"`,
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
	if cfg.RemoteToken != "xuanchu_pat_toml" {
		t.Fatalf("RemoteToken = %q", cfg.RemoteToken)
	}
}

func TestResolveReadsDispatcherConfigFromToml(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xuanchu.toml")
	if err := os.WriteFile(path, []byte(strings.Join([]string{
		"[notifications.dispatcher]",
		"max_concurrency = 4",
		"batch_size = 20",
		"prefetch_factor = 2",
		`poll_interval = "3s"`,
		`claim_ttl = "7m"`,
		"",
		"[hooks.dispatcher]",
		"max_concurrency = 3",
		"batch_size = 15",
		"prefetch_factor = 1",
		`poll_interval = "4s"`,
		`claim_ttl = "8m"`,
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Resolve(Options{ConfigPath: path, HomeDir: dir, Env: map[string]string{}})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if cfg.NotificationDispatcher.MaxConcurrency != 4 {
		t.Fatalf("NotificationDispatcher.MaxConcurrency = %d, want 4", cfg.NotificationDispatcher.MaxConcurrency)
	}
	if cfg.NotificationDispatcher.BatchSize != 20 {
		t.Fatalf("NotificationDispatcher.BatchSize = %d, want 20", cfg.NotificationDispatcher.BatchSize)
	}
	if cfg.NotificationDispatcher.PrefetchFactor != 2 {
		t.Fatalf("NotificationDispatcher.PrefetchFactor = %d, want 2", cfg.NotificationDispatcher.PrefetchFactor)
	}
	if cfg.NotificationDispatcher.PollInterval != 3*time.Second {
		t.Fatalf("NotificationDispatcher.PollInterval = %v, want 3s", cfg.NotificationDispatcher.PollInterval)
	}
	if cfg.NotificationDispatcher.ClaimTTL != 7*time.Minute {
		t.Fatalf("NotificationDispatcher.ClaimTTL = %v, want 7m", cfg.NotificationDispatcher.ClaimTTL)
	}
	if cfg.HookDispatcher.MaxConcurrency != 3 {
		t.Fatalf("HookDispatcher.MaxConcurrency = %d, want 3", cfg.HookDispatcher.MaxConcurrency)
	}
	if cfg.HookDispatcher.BatchSize != 15 {
		t.Fatalf("HookDispatcher.BatchSize = %d, want 15", cfg.HookDispatcher.BatchSize)
	}
	if cfg.HookDispatcher.PollInterval != 4*time.Second {
		t.Fatalf("HookDispatcher.PollInterval = %v, want 4s", cfg.HookDispatcher.PollInterval)
	}
	if cfg.HookDispatcher.ClaimTTL != 8*time.Minute {
		t.Fatalf("HookDispatcher.ClaimTTL = %v, want 8m", cfg.HookDispatcher.ClaimTTL)
	}
}

func TestResolveDispatcherConfigDefaults(t *testing.T) {
	cfg, err := Resolve(Options{HomeDir: "/home/alice", Env: map[string]string{}})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	assertDefaultDispatcherConfig(t, "NotificationDispatcher", cfg.NotificationDispatcher)
	assertDefaultDispatcherConfig(t, "HookDispatcher", cfg.HookDispatcher)
}

func assertDefaultDispatcherConfig(t *testing.T, name string, cfg DispatcherConfig) {
	t.Helper()
	if cfg.MaxConcurrency != 1 {
		t.Fatalf("%s.MaxConcurrency = %d, want 1", name, cfg.MaxConcurrency)
	}
	if cfg.BatchSize != 50 {
		t.Fatalf("%s.BatchSize = %d, want 50", name, cfg.BatchSize)
	}
	if cfg.PrefetchFactor != 1 {
		t.Fatalf("%s.PrefetchFactor = %d, want 1", name, cfg.PrefetchFactor)
	}
	if cfg.PollInterval != 5*time.Second {
		t.Fatalf("%s.PollInterval = %v, want 5s", name, cfg.PollInterval)
	}
	if cfg.ClaimTTL != 5*time.Minute {
		t.Fatalf("%s.ClaimTTL = %v, want 5m", name, cfg.ClaimTTL)
	}
}

func TestResolveRejectsInvalidDispatcherConfig(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "negative int",
			body: strings.Join([]string{
				"[notifications.dispatcher]",
				"max_concurrency = -1",
				"",
			}, "\n"),
			want: "notifications.dispatcher.max_concurrency",
		},
		{
			name: "zero int",
			body: strings.Join([]string{
				"[hooks.dispatcher]",
				"batch_size = 0",
				"",
			}, "\n"),
			want: "hooks.dispatcher.batch_size",
		},
		{
			name: "invalid duration",
			body: strings.Join([]string{
				"[notifications.dispatcher]",
				`claim_ttl = "soon"`,
				"",
			}, "\n"),
			want: "notifications.dispatcher.claim_ttl",
		},
		{
			name: "zero duration",
			body: strings.Join([]string{
				"[hooks.dispatcher]",
				`poll_interval = "0s"`,
				"",
			}, "\n"),
			want: "hooks.dispatcher.poll_interval",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "xuanchu.toml")
			if err := os.WriteFile(path, []byte(tt.body), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := Resolve(Options{ConfigPath: path, HomeDir: dir, Env: map[string]string{}})
			if err == nil {
				t.Fatal("Resolve() error = nil, want invalid dispatcher config error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Resolve() error = %q, want contains %q", err.Error(), tt.want)
			}
		})
	}
}

func TestResolveRemoteEnvOverridesToml(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	if err := os.MkdirAll(filepath.Join(configDir, "xuanchu"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "xuanchu", "xuanchu.toml"), []byte(strings.Join([]string{
		"[remote]",
		`server = "http://127.0.0.1:8080"`,
		`token = "xuanchu_pat_toml"`,
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Resolve(Options{
		Env: map[string]string{
			"XDG_CONFIG_HOME": configDir,
			"XUANCHU_SERVER":  "http://env.example",
			"XUANCHU_TOKEN":   "xuanchu_pat_env",
		},
		HomeDir: filepath.Join(dir, "home"),
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if cfg.RemoteServer != "http://env.example" {
		t.Fatalf("RemoteServer = %q", cfg.RemoteServer)
	}
	if cfg.RemoteToken != "xuanchu_pat_env" {
		t.Fatalf("RemoteToken = %q", cfg.RemoteToken)
	}
}

func TestRuntimeMergesSourcesAndRcOverrides(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	if err := os.MkdirAll(filepath.Join(configDir, "xuanchu"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "xuanchu", "xuanchu.toml"), []byte(strings.Join([]string{
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
	if err := os.WriteFile(filepath.Join(configDir, "xuanchu.toml"), []byte(strings.Join([]string{
		"[remote]",
		`server = "http://127.0.0.1:8080"`,
		`token = "xuanchu_pat_toml"`,
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
	if got, _ := rt.Get("remote.token"); got != "xuanchu_pat_toml" {
		t.Fatalf("remote.token = %q", got)
	}
}

func TestLoadRuntimeEnvOverridesRemoteToml(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "xuanchu.toml"), []byte(strings.Join([]string{
		"[remote]",
		`server = "http://127.0.0.1:8080"`,
		`token = "xuanchu_pat_toml"`,
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	rt, err := LoadRuntime(RuntimeOptions{
		ConfigDir: configDir,
		Env: map[string]string{
			"XUANCHU_SERVER": "http://env.example",
			"XUANCHU_TOKEN":  "xuanchu_pat_env",
		},
	})
	if err != nil {
		t.Fatalf("LoadRuntime() error = %v", err)
	}
	if got, _ := rt.Get("remote.server"); got != "http://env.example" {
		t.Fatalf("remote.server = %q", got)
	}
	if got, _ := rt.Get("remote.token"); got != "xuanchu_pat_env" {
		t.Fatalf("remote.token = %q", got)
	}
}

func TestLoadRuntimeMapsDisplayTomlKeys(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "xuanchu.toml"), []byte(strings.Join([]string{
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
	if err := os.WriteFile(filepath.Join(configDir, "xuanchu.toml"), []byte(strings.Join([]string{
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
	if err := os.WriteFile(filepath.Join(configDir, "xuanchu.toml"), []byte(strings.Join([]string{
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
	if err := os.WriteFile(filepath.Join(configDir, "xuanchu.toml"), []byte(strings.Join([]string{
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
	configPath := filepath.Join(configDir, "xuanchu.toml")
	if err := os.WriteFile(configPath, []byte(strings.Join([]string{
		`[remote]`,
		`token = "xuanchu_pat_toml"`,
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
	if err := os.WriteFile(filepath.Join(configDir, "xuanchu.toml"), []byte(strings.Join([]string{
		`[remote]`,
		`token = "xuanchu_pat_toml"`,
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
	if err := os.WriteFile(filepath.Join(configDir, "xuanchu.toml"), []byte(strings.Join([]string{
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

func TestResolve_DBURLAndDBPathMutualExclusion(t *testing.T) {
	_, err := Resolve(Options{DBURL: "postgres://host/db", DBPath: "/path/to.db", HomeDir: "/home"})
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected mutual exclusion error, got: %v", err)
	}
}

func TestResolve_DBURLOnly(t *testing.T) {
	cfg, err := Resolve(Options{DBURL: "postgres://host/db", HomeDir: "/home"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != "postgres://host/db" {
		t.Errorf("got %q", cfg.DatabaseURL)
	}
	if cfg.DatabasePath != "" {
		t.Errorf("got %q", cfg.DatabasePath)
	}
}

func TestResolve_DBURLEnvVar(t *testing.T) {
	cfg, err := Resolve(Options{HomeDir: "/home", Env: map[string]string{"XUANCHU_DB_URL": "postgres://env/db"}})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != "postgres://env/db" {
		t.Errorf("got %q", cfg.DatabaseURL)
	}
}

func TestResolve_DBPathRejectsURL(t *testing.T) {
	_, err := Resolve(Options{DBPath: "postgres://host/db", HomeDir: "/home"})
	if err == nil {
		t.Error("expected error")
	}
}

func TestResolve_DBPathRejectsAnyScheme(t *testing.T) {
	_, err := Resolve(Options{DBPath: "mysql://host/db", HomeDir: "/home"})
	if err == nil {
		t.Error("expected error")
	}
}

func TestResolve_DBURLEnvOverridesDBEnv(t *testing.T) {
	cfg, err := Resolve(Options{HomeDir: "/home", Env: map[string]string{"XUANCHU_DB_URL": "postgres://env/db", "XUANCHU_DB": "/path/to.db"}})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != "postgres://env/db" {
		t.Errorf("got %q", cfg.DatabaseURL)
	}
	if cfg.DatabasePath != "" {
		t.Errorf("got %q", cfg.DatabasePath)
	}
}

func stringPtr(v string) *string {
	return &v
}

func TestResolveConfigPathLoadsToml(t *testing.T) {
	dir := t.TempDir()
	tomlPath := dir + "/custom.toml"
	tomlDB := dir + "/toml.db"
	if err := os.WriteFile(tomlPath, []byte("database.path = \""+tomlDB+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Resolve(Options{
		ConfigPath: tomlPath,
		HomeDir:    "/home/alice",
		Env:        map[string]string{},
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if cfg.DatabasePath != tomlDB {
		t.Fatalf("DatabasePath = %q, want %q", cfg.DatabasePath, tomlDB)
	}
}

func TestResolveConfigPathNonexistentReturnsError(t *testing.T) {
	_, err := Resolve(Options{
		ConfigPath: "/nonexistent/path.toml",
		HomeDir:    "/home/alice",
		Env:        map[string]string{},
	})
	if err == nil {
		t.Fatal("expected error for nonexistent config path")
	}
	if !strings.Contains(err.Error(), "--config") {
		t.Fatalf("error = %q, want --config prefix", err.Error())
	}
}

func TestResolveXuanchuConfigEnvFallback(t *testing.T) {
	dir := t.TempDir()
	tomlPath := dir + "/env.toml"
	tomlDB := dir + "/env.db"
	if err := os.WriteFile(tomlPath, []byte("database.path = \""+tomlDB+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Resolve(Options{
		HomeDir: "/home/alice",
		Env:     map[string]string{"XUANCHU_CONFIG": tomlPath},
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if cfg.DatabasePath != tomlDB {
		t.Fatalf("DatabasePath = %q, want %q", cfg.DatabasePath, tomlDB)
	}
}

func TestResolveLogConfigFromToml(t *testing.T) {
	dir := t.TempDir()
	tomlPath := dir + "/xuanchu.toml"
	if err := os.WriteFile(tomlPath, []byte(strings.Join([]string{
		"[log]",
		`level = "debug"`,
		`format = "json"`,
		"[log.file]",
		`path = "/tmp/xuanchu.log"`,
		`rotate = "daily"`,
		`max_size_mb = 50`,
		`max_age_days = 7`,
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Resolve(Options{
		ConfigPath: tomlPath,
		HomeDir:    "/home/alice",
		Env:        map[string]string{},
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if cfg.Log.Level != "debug" {
		t.Fatalf("Log.Level = %q, want %q", cfg.Log.Level, "debug")
	}
	if cfg.Log.Format != "json" {
		t.Fatalf("Log.Format = %q, want %q", cfg.Log.Format, "json")
	}
	if cfg.Log.File == nil {
		t.Fatal("Log.File is nil")
	}
	if cfg.Log.File.Path != "/tmp/xuanchu.log" {
		t.Fatalf("Log.File.Path = %q", cfg.Log.File.Path)
	}
	if cfg.Log.File.Rotate != "daily" {
		t.Fatalf("Log.File.Rotate = %q", cfg.Log.File.Rotate)
	}
	if cfg.Log.File.MaxSizeMB != 50 {
		t.Fatalf("Log.File.MaxSizeMB = %d, want 50", cfg.Log.File.MaxSizeMB)
	}
	if cfg.Log.File.MaxAgeDays != 7 {
		t.Fatalf("Log.File.MaxAgeDays = %d, want 7", cfg.Log.File.MaxAgeDays)
	}
}

func TestResolveLogEnvOverridesToml(t *testing.T) {
	dir := t.TempDir()
	tomlPath := dir + "/xuanchu.toml"
	if err := os.WriteFile(tomlPath, []byte(strings.Join([]string{
		"[log]",
		`level = "debug"`,
		"[log.file]",
		`path = "/tmp/xuanchu.log"`,
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Resolve(Options{
		ConfigPath: tomlPath,
		HomeDir:    "/home/alice",
		Env: map[string]string{
			"XUANCHU_LOG_LEVEL": "warn",
			"XUANCHU_LOG_FILE":  "/var/log/xuanchu.log",
		},
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if cfg.Log.Level != "warn" {
		t.Fatalf("Log.Level = %q, want %q", cfg.Log.Level, "warn")
	}
	if cfg.Log.File == nil {
		t.Fatal("Log.File is nil")
	}
	if cfg.Log.File.Path != "/var/log/xuanchu.log" {
		t.Fatalf("Log.File.Path = %q, want /var/log/xuanchu.log", cfg.Log.File.Path)
	}
}

func TestResolveLogEnvWithoutToml(t *testing.T) {
	cfg, err := Resolve(Options{
		HomeDir: "/home/alice",
		Env: map[string]string{
			"XUANCHU_LOG_LEVEL": "error",
			"XUANCHU_LOG_FILE":  "/tmp/from-env.log",
		},
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if cfg.Log.Level != "error" {
		t.Fatalf("Log.Level = %q, want %q", cfg.Log.Level, "error")
	}
	if cfg.Log.File == nil {
		t.Fatal("Log.File is nil")
	}
	if cfg.Log.File.Path != "/tmp/from-env.log" {
		t.Fatalf("Log.File.Path = %q, want /tmp/from-env.log", cfg.Log.File.Path)
	}
}
