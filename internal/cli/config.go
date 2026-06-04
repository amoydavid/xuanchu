package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/config"
	"github.com/dajee/taskg/internal/render"
	"github.com/dajee/taskg/internal/storage"
	taskrcparser "github.com/dajee/taskg/internal/taskrc"
	"github.com/spf13/cobra"
)

func newShowCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "show",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				values, err := client.ListConfig(context.Background(), currentOpts.Workspace)
				if err != nil {
					return err
				}
				for _, key := range []string{"color", "json", "date.format", "active.user", "active.workspace", "active.context"} {
					value, ok := values[key]
					if !ok {
						value = ""
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%s=%s\n", key, value)
				}
				return nil
			}
			rt, err := runtimeFromOptions(currentOpts)
			if err != nil {
				return err
			}
			for _, key := range []string{"database.path", "color", "json", "date.format", "active.user", "active.workspace", "active.context"} {
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
	cmd.AddCommand(newConfigImportTaskRCCommand(opts))
	return cmd
}

func newConfigGetCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "get <key>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			key := args[0]
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				value, err := client.GetConfig(context.Background(), currentOpts.Workspace, key)
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), value)
				return nil
			}
			rt, err := runtimeFromOptions(currentOpts)
			if err != nil {
				return err
			}
			if key == "context.active" {
				return fmt.Errorf("unsupported legacy key %q", key)
			}
			if value, ok := rt.Get(key); ok {
				if value != "" {
					fmt.Fprintln(cmd.OutOrStdout(), value)
					return nil
				}
				if _, known := map[string]bool{"color": true, "json": true, "date.format": true, "active.user": true, "active.workspace": true, "active.context": true, "database.path": true}[key]; known {
					fmt.Fprintln(cmd.OutOrStdout(), value)
					return nil
				}
			}
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
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				return client.SetConfig(context.Background(), currentOpts.Workspace, key, value)
			}
			if key == "context.active" {
				return fmt.Errorf("context.active is managed by context commands")
			}
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
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				return client.UnsetConfig(context.Background(), currentOpts.Workspace, key)
			}
			if key == "context.active" {
				return fmt.Errorf("context.active is managed by context commands")
			}
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
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				values, err := client.ListConfig(context.Background(), currentOpts.Workspace)
				if err != nil {
					return err
				}
				keys := make([]string, 0, len(values))
				for key := range values {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				for _, key := range keys {
					fmt.Fprintf(cmd.OutOrStdout(), "%s=%s\n", key, values[key])
				}
				return nil
			}
			rt, err := runtimeFromOptions(currentOpts)
			if err != nil {
				return err
			}
			keys := rt.Keys()
			sort.Strings(keys)
			for _, key := range keys {
				if !isPublicConfigKey(key) {
					continue
				}
				value, _ := rt.Get(key)
				fmt.Fprintf(cmd.OutOrStdout(), "%s=%s\n", key, value)
			}
			return nil
		},
	}
}

func newConfigImportTaskRCCommand(opts Options) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:  "import-taskrc <path>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				return app.RuntimeError{Code: "remote_unsupported_command", Message: `command "config import-taskrc" is not supported in remote mode`}
			}
			svc, closeFn, err := buildServiceFromOpts(currentOpts)
			if err != nil {
				return err
			}
			defer closeFn()
			report, err := svc.ImportTaskRC(args[0], dryRun)
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), report)
			}
			renderTaskRCReport(cmd.OutOrStdout(), report)
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "parse without writing")
	return cmd
}

func renderTaskRCReport(w io.Writer, report taskrcparser.Report) {
	fmt.Fprintln(w, "imported:")
	for _, entry := range report.Imported {
		fmt.Fprintf(w, "  %s\n", entry.Target)
	}
	fmt.Fprintln(w, "skipped:")
	for _, entry := range report.Skipped {
		fmt.Fprintf(w, "  %s (%s)\n", entry.Key, entry.Reason)
	}
	fmt.Fprintln(w, "unknown:")
	for _, entry := range report.Unknown {
		fmt.Fprintf(w, "  %s (%s)\n", entry.Key, entry.Reason)
	}
}

func runtimeFromOptions(opts Options) (config.Runtime, error) {
	env := RuntimeEnv()
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
	store, err := storage.Open(cfg.DatabasePath)
	if err != nil {
		return config.Runtime{}, err
	}
	defer store.Close()
	return runtimeFromResolvedConfig(opts, cfg, store, env)
}

func runtimeFromResolvedConfig(opts Options, cfg config.Config, store *storage.Store, env map[string]string) (config.Runtime, error) {
	rawMeta, err := store.ListMeta()
	if err != nil {
		return config.Runtime{}, err
	}
	meta := make(map[string]string, len(rawMeta))
	for key, value := range rawMeta {
		if isPublicConfigKey(key) {
			meta[key] = value
		}
	}
	svc, err := app.NewService(app.ServiceOptions{Store: store, NoContext: opts.NoContext, WorkspaceRef: opts.Workspace})
	if err != nil {
		return config.Runtime{}, err
	}
	svcValues, err := svc.ConfigValues()
	if err != nil {
		return config.Runtime{}, err
	}
	for key, value := range svcValues {
		if isPublicConfigKey(key) {
			meta[key] = value
		}
	}
	if activeName, ok, err := svc.ActiveContextName(); err != nil {
		return config.Runtime{}, err
	} else if ok {
		meta["active.context"] = activeName
	} else {
		meta["active.context"] = ""
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
		Flags: map[string]string{
			"database.path": cfg.DatabasePath,
		},
		Defaults: map[string]string{
			"json":             fmt.Sprintf("%v", opts.JSON),
			"color":            fmt.Sprintf("%v", !opts.NoColor),
			"active.user":      svc.Runtime().ActorName,
			"active.workspace": svc.Runtime().WorkspaceSlug,
		},
	})
}

func isPublicConfigKey(key string) bool {
	if strings.HasPrefix(key, "uda.") || strings.HasPrefix(key, "urgency.") {
		return true
	}
	switch key {
	case "color", "json", "date.format", "database.path", "active.user", "active.workspace", "active.context":
		return true
	default:
		return false
	}
}
