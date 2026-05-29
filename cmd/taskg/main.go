package main

import (
	"fmt"
	"os"

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
		switch e := err.(type) {
		case app.RuntimeError:
			fmt.Fprintln(os.Stderr, "taskg:", e.Code+":", e.Message)
			os.Exit(1)
		case app.PermissionError:
			fmt.Fprintln(os.Stderr, "taskg:", e.Code+":", e.Message)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "taskg:", err)
		os.Exit(1)
	}
}
