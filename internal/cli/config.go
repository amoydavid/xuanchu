package cli

import (
	"fmt"
	"sort"

	"github.com/dajee/taskg/internal/config"
	"github.com/dajee/taskg/internal/storage/sqlite"
	"github.com/spf13/cobra"
)

func newShowCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "show",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			rt, err := runtimeFromOptions(currentOpts)
			if err != nil {
				return err
			}
			for _, key := range []string{"database.path", "color", "json", "date.format", "context.active"} {
				value, _ := rt.Get(key)
				fmt.Fprintf(cmd.OutOrStdout(), "%s=%s\n", key, value)
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
	cmd.AddCommand(newConfigUnsetCommand(opts))
	cmd.AddCommand(newConfigListCommand(opts))
	return cmd
}

func newConfigGetCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "get <key>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			key := args[0]
			rt, err := runtimeFromOptions(currentOpts)
			if err != nil {
				return err
			}
			if value, ok := rt.Get(key); ok {
				fmt.Fprintln(cmd.OutOrStdout(), value)
				return nil
			}
			return fmt.Errorf("unknown config key %q", key)
		},
	}
}

func newConfigSetCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "set <key> <value>",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			key, value := args[0], args[1]
			switch key {
			case "color", "date.format":
				// allowed
			case "database.path":
				return fmt.Errorf("database.path is read-only; use --db or TASKG_DB")
			default:
				return fmt.Errorf("unknown config key %q", key)
			}
			store, err := openStore(currentOpts)
			if err != nil {
				return err
			}
			defer store.Close()
			return store.SetMeta(key, value)
		},
	}
}

func newConfigUnsetCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "unset <key>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			key := args[0]
			switch key {
			case "date.format", "color", "json":
				// allowed
			case "database.path":
				return fmt.Errorf("database.path is read-only; use --db or TASKG_DB")
			case "context.active":
				return fmt.Errorf("context.active is managed by context commands")
			default:
				return fmt.Errorf("unknown config key %q", key)
			}
			store, err := openStore(currentOpts)
			if err != nil {
				return err
			}
			defer store.Close()
			return store.DeleteMeta(key)
		},
	}
}

func newConfigListCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "list",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			rt, err := runtimeFromOptions(currentOpts)
			if err != nil {
				return err
			}
			keys := rt.Keys()
			sort.Strings(keys)
			for _, key := range keys {
				value, _ := rt.Get(key)
				fmt.Fprintf(cmd.OutOrStdout(), "%s=%s\n", key, value)
			}
			return nil
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

func runtimeFromOptions(opts Options) (config.Runtime, error) {
	cfg, err := config.Resolve(config.Options{
		DataDir: opts.DataDir,
		DBPath:  opts.DBPath,
		JSON:    opts.JSON,
		NoColor: opts.NoColor,
	})
	if err != nil {
		return config.Runtime{}, err
	}
	store, err := sqlite.Open(cfg.DatabasePath)
	if err != nil {
		return config.Runtime{}, err
	}
	defer store.Close()
	meta, err := store.ListMeta()
	if err != nil {
		return config.Runtime{}, err
	}
	return config.LoadRuntime(config.RuntimeOptions{
		Meta:        meta,
		RCOverrides: opts.RCOverrides,
		Defaults: map[string]string{
			"database.path": cfg.DatabasePath,
			"json":          fmt.Sprintf("%v", opts.JSON),
			"color":         fmt.Sprintf("%v", !opts.NoColor),
		},
	})
}
