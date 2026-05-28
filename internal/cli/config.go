package cli

import (
	"fmt"

	"github.com/dajee/taskg/internal/config"
	"github.com/dajee/taskg/internal/storage/sqlite"
	"github.com/spf13/cobra"
)

func newShowCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "show",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Resolve(config.Options{
				DataDir: opts.DataDir,
				DBPath:  opts.DBPath,
				JSON:    opts.JSON,
				NoColor: opts.NoColor,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "database.path=%s\n", cfg.DatabasePath)
			fmt.Fprintf(cmd.OutOrStdout(), "color=%v\n", cfg.Color)

			store, err := openStore(opts)
			if err != nil {
				return err
			}
			defer store.Close()
			if value, ok, _ := store.GetMeta("date.format"); ok {
				fmt.Fprintf(cmd.OutOrStdout(), "date.format=%s\n", value)
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "date.format=rfc3339")
			}
			return nil
		},
	}
}

func newConfigCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:  "config",
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(newConfigGetCommand(opts))
	cmd.AddCommand(newConfigSetCommand(opts))
	return cmd
}

func newConfigGetCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "get <key>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			defer store.Close()

			if value, ok, _ := store.GetMeta(key); ok {
				fmt.Fprintln(cmd.OutOrStdout(), value)
				return nil
			}
			switch key {
			case "database.path":
				cfg, _ := config.Resolve(config.Options{
					DataDir: opts.DataDir, DBPath: opts.DBPath,
				})
				fmt.Fprintln(cmd.OutOrStdout(), cfg.DatabasePath)
			case "color":
				fmt.Fprintln(cmd.OutOrStdout(), "true")
			case "date.format":
				fmt.Fprintln(cmd.OutOrStdout(), "rfc3339")
			default:
				return fmt.Errorf("unknown config key %q", key)
			}
			return nil
		},
	}
}

func newConfigSetCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "set <key> <value>",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, value := args[0], args[1]
			switch key {
			case "color", "date.format":
				// allowed
			case "database.path":
				return fmt.Errorf("database.path is read-only; use --db or TASKG_DB")
			default:
				return fmt.Errorf("unknown config key %q", key)
			}
			store, err := openStore(opts)
			if err != nil {
				return err
			}
			defer store.Close()
			return store.SetMeta(key, value)
		},
	}
}

func openStore(opts Options) (*sqlite.Store, error) {
	cfg, err := config.Resolve(config.Options{
		DataDir: opts.DataDir, DBPath: opts.DBPath,
	})
	if err != nil {
		return nil, err
	}
	return sqlite.Open(cfg.DatabasePath)
}
