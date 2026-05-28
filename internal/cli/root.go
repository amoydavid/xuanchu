package cli

import (
	"fmt"
	"io"

	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/config"
	"github.com/dajee/taskg/internal/storage/sqlite"
	"github.com/spf13/cobra"
)

type Options struct {
	Stdout  io.Writer
	Stderr  io.Writer
	Version string

	DataDir string
	DBPath  string
	JSON    bool
	NoColor bool
}

func NewRootCommand(opts Options) *cobra.Command {
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	if opts.Version == "" {
		opts.Version = "dev"
	}

	cmd := &cobra.Command{
		Use:           "taskg",
		Short:         "Taskwarrior-style task manager",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       opts.Version,
	}
	cmd.SetOut(opts.Stdout)
	cmd.SetErr(opts.Stderr)
	cmd.SetVersionTemplate(fmt.Sprintf("taskg %s\n", opts.Version))

	cmd.PersistentFlags().StringVar(&opts.DataDir, "data-dir", opts.DataDir, "data directory")
	cmd.PersistentFlags().StringVar(&opts.DBPath, "db", opts.DBPath, "SQLite database path")
	cmd.PersistentFlags().BoolVar(&opts.JSON, "json", opts.JSON, "render JSON output")
	cmd.PersistentFlags().BoolVar(&opts.NoColor, "no-color", opts.NoColor, "disable colored output")

	cmd.AddCommand(newAddCommand(opts))
	cmd.AddCommand(newListCommand(opts))
	cmd.AddCommand(newInfoCommand(opts))

	return cmd
}

func buildService(opts Options) (*app.Service, func() error, error) {
	cfg, err := config.Resolve(config.Options{
		DataDir: opts.DataDir,
		DBPath:  opts.DBPath,
		JSON:    opts.JSON,
		NoColor: opts.NoColor,
	})
	if err != nil {
		return nil, nil, err
	}
	store, err := sqlite.Open(cfg.DatabasePath)
	if err != nil {
		return nil, nil, err
	}
	svc, err := app.NewService(app.ServiceOptions{Store: store})
	if err != nil {
		_ = store.Close()
		return nil, nil, err
	}
	return svc, store.Close, nil
}
