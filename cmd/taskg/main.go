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
)

var version = "dev"

func main() {
	opts := cli.Options{
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Version: version,
	}
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

func jsonTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "0", "false", "off", "no":
		return false
	default:
		return true
	}
}
