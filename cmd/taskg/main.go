package main

import (
	"fmt"
	"os"

	"github.com/dajee/taskg/internal/cli"
)

var version = "dev"

func main() {
	cmd := cli.NewRootCommand(cli.Options{
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Version: version,
	})
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "taskg:", err)
		os.Exit(1)
	}
}
