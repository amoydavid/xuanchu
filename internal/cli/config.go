package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/dajee/taskg/internal/app"
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
			if strings.HasPrefix(key, "uda.") {
				svc, closeFn, err := buildServiceFromOpts(currentOpts)
				if err != nil {
					return err
				}
				defer closeFn()
				if value, ok, err := svc.GetConfig(key); err != nil {
					return err
				} else if ok {
					fmt.Fprintln(cmd.OutOrStdout(), value)
					return nil
				}
			}
			if value, ok := rt.Get(key); ok {
				if value != "" {
					fmt.Fprintln(cmd.OutOrStdout(), value)
					return nil
				}
				if _, known := map[string]bool{"color": true, "json": true, "date.format": true, "context.active": true, "database.path": true}[key]; known {
					fmt.Fprintln(cmd.OutOrStdout(), value)
					return nil
				}
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
			svc, closeFn, err := buildServiceFromOpts(currentOpts)
			if err != nil {
				return err
			}
			defer closeFn()
			return svc.SetConfig(key, value)
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
			svc, closeFn, err := buildServiceFromOpts(currentOpts)
			if err != nil {
				return err
			}
			defer closeFn()
			return svc.UnsetConfig(key)
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
	env := runtimeEnv()
	cfg, err := config.Resolve(config.Options{
		DataDir: opts.DataDir,
		DBPath:  opts.DBPath,
		JSON:    opts.JSON,
		NoColor: opts.NoColor,
		Env:     env,
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
	svc, err := app.NewService(app.ServiceOptions{Store: store, NoContext: opts.NoContext})
	if err != nil {
		return config.Runtime{}, err
	}
	svcValues, err := svc.ConfigValues()
	if err != nil {
		return config.Runtime{}, err
	}
	for key, value := range svcValues {
		meta[key] = value
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return config.Runtime{}, err
	}
	return config.LoadRuntime(config.RuntimeOptions{
		ConfigDir:   config.ConfigDir(home, env),
		Meta:        meta,
		Env:         env,
		RCOverrides: opts.RCOverrides,
		Defaults: map[string]string{
			"database.path": cfg.DatabasePath,
			"json":          fmt.Sprintf("%v", opts.JSON),
			"color":         fmt.Sprintf("%v", !opts.NoColor),
		},
	})
}
