package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/cli"
	"git.dajee.net/dajee/xuanchu/internal/config"
	"git.dajee.net/dajee/xuanchu/internal/logging"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

var version = ""

var globalLogger *logging.Logger

func main() {
	defer handlePanic()
	opts := cli.Options{
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
		Version:   resolveVersion(),
		SetLogger: func(l *logging.Logger) { globalLogger = l },
	}
	maybeWarnM5Migration(os.Stderr, os.Args[1:], warningOptionsFromArgs(os.Args[1:], opts))
	cmd := cli.NewRootCommand(opts)
	if err := cli.Execute(cmd, opts, os.Args[1:]); err != nil {
		var runtimeErr app.RuntimeError
		if errors.As(err, &runtimeErr) {
			writeError(os.Stderr, os.Args[1:], runtimeErr.Code, runtimeErr.Message)
			os.Exit(1)
		}
		var permissionErr app.PermissionError
		if errors.As(err, &permissionErr) {
			writeError(os.Stderr, os.Args[1:], permissionErr.Code, permissionErr.Message)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "xuanchu:", err)
		os.Exit(1)
	}
}

func maybeWarnM5Migration(w io.Writer, args []string, opts cli.Options) {
	if skipsMigrationWarning(args) {
		return
	}
	cfg, err := config.Resolve(config.Options{
		DataDir:    opts.DataDir,
		DBPath:     opts.DBPath,
		DBURL:      opts.DBURL,
		Server:     opts.Server,
		Token:      opts.Token,
		JSON:       opts.JSON,
		NoColor:    opts.NoColor,
		Env:        cli.RuntimeEnv(),
		ConfigPath: opts.Config,
	})
	if err != nil {
		return
	}
	if cfg.RemoteServer != "" {
		return
	}
	if cfg.DatabaseURL != "" {
		return
	}
	store, err := storage.Open(cfg.DatabasePath)
	if err != nil {
		return
	}
	defer store.Close()
	report, err := store.M5ProjectMigrationReport()
	if err != nil || len(report) == 0 {
		return
	}
	fmt.Fprintln(w, "xuanchu: warning: M5 project migration skipped some legacy project strings; inspect migration.m5.projects.skipped for details")
}

func skipsMigrationWarning(args []string) bool {
	for _, arg := range args {
		switch arg {
		case "completion", "help", "--help", "-h", "--version", "version", "mcp":
			return true
		}
	}
	return false
}

func writeError(w io.Writer, args []string, code, message string) {
	if wantsJSON(args) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"code":    code,
			"message": message,
		})
		return
	}
	fmt.Fprintln(w, "xuanchu:", code+":", message)
}

func wantsJSON(args []string) bool {
	for _, arg := range args {
		if arg == "--json" {
			return true
		}
		if strings.HasPrefix(arg, "--json=") {
			return jsonTruthy(strings.TrimPrefix(arg, "--json="))
		}
		if strings.HasPrefix(arg, "rc.json=") {
			return jsonTruthy(strings.TrimPrefix(arg, "rc.json="))
		}
		if strings.HasPrefix(arg, "rc.json:") {
			return jsonTruthy(strings.TrimPrefix(arg, "rc.json:"))
		}
	}
	return false
}

func warningOptionsFromArgs(args []string, base cli.Options) cli.Options {
	opts := base
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--db" && i+1 < len(args):
			opts.DBPath = args[i+1]
			i++
		case strings.HasPrefix(arg, "--db="):
			opts.DBPath = strings.TrimPrefix(arg, "--db=")
		case arg == "--db-url" && i+1 < len(args):
			opts.DBURL = args[i+1]
			i++
		case strings.HasPrefix(arg, "--db-url="):
			opts.DBURL = strings.TrimPrefix(arg, "--db-url=")
		case arg == "--data-dir" && i+1 < len(args):
			opts.DataDir = args[i+1]
			i++
		case strings.HasPrefix(arg, "--data-dir="):
			opts.DataDir = strings.TrimPrefix(arg, "--data-dir=")
		case arg == "--server" && i+1 < len(args):
			opts.Server = args[i+1]
			i++
		case strings.HasPrefix(arg, "--server="):
			opts.Server = strings.TrimPrefix(arg, "--server=")
		case arg == "--token" && i+1 < len(args):
			opts.Token = args[i+1]
			i++
		case strings.HasPrefix(arg, "--token="):
			opts.Token = strings.TrimPrefix(arg, "--token=")
		case arg == "--json":
			opts.JSON = true
		case strings.HasPrefix(arg, "--json="):
			opts.JSON = jsonTruthy(strings.TrimPrefix(arg, "--json="))
		case arg == "--no-color":
			opts.NoColor = true
		case arg == "--config" && i+1 < len(args):
			opts.Config = args[i+1]
			i++
		case strings.HasPrefix(arg, "--config="):
			opts.Config = strings.TrimPrefix(arg, "--config=")
		}
	}
	return opts
}

func jsonTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "0", "false", "off", "no":
		return false
	default:
		return true
	}
}

func handlePanic() {
	r := recover()
	if r == nil {
		return
	}
	fmt.Fprintln(os.Stderr, "xuanchu: internal error")
	logPanic(r)
	os.Exit(1)
}

func logPanic(r any) {
	defer func() { recover() }()
	stack := debug.Stack()
	msg := fmt.Sprintf("[%s] panic: %v\n%s\n", time.Now().Format(time.RFC3339), r, stack)
	if globalLogger != nil {
		globalLogger.Error("panic recovered", "panic", r, "stack", string(stack))
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	path := filepath.Join(home, ".local", "share", "xuanchu", "logs", "panic.log")
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", msg)
		return
	}
	defer f.Close()
	f.WriteString(msg)
}

func resolveVersion() string {
	if version != "" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	var rev, t string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.time":
			t = s.Value
		}
	}
	if rev == "" {
		return "dev"
	}
	short := rev
	if len(short) > 12 {
		short = short[:12]
	}
	v := "dev+" + short
	if t != "" {
		if pt, err := time.Parse(time.RFC3339, t); err == nil {
			v += " " + pt.Format("2006-01-02")
		}
	}
	return v
}
