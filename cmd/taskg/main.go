package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/cli"
	"github.com/dajee/taskg/internal/config"
	"github.com/dajee/taskg/internal/storage/sqlite"
)

var version = "dev"

func main() {
	opts := cli.Options{
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Version: version,
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
		fmt.Fprintln(os.Stderr, "taskg:", err)
		os.Exit(1)
	}
}

func maybeWarnM5Migration(w io.Writer, args []string, opts cli.Options) {
	if skipsMigrationWarning(args) {
		return
	}
	cfg, err := config.Resolve(config.Options{
		DataDir: opts.DataDir,
		DBPath:  opts.DBPath,
		Server:  opts.Server,
		Token:   opts.Token,
		JSON:    opts.JSON,
		NoColor: opts.NoColor,
		Env:     cli.RuntimeEnv(),
	})
	if err != nil {
		return
	}
	if cfg.RemoteServer != "" {
		return
	}
	store, err := sqlite.Open(cfg.DatabasePath)
	if err != nil {
		return
	}
	defer store.Close()
	report, err := store.M5ProjectMigrationReport()
	if err != nil || len(report) == 0 {
		return
	}
	fmt.Fprintln(w, "taskg: warning: M5 project migration skipped some legacy project strings; inspect migration.m5.projects.skipped for details")
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
	fmt.Fprintln(w, "taskg:", code+":", message)
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
