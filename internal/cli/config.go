package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/config"
	"git.dajee.net/dajee/xuanchu/internal/remote"
	"git.dajee.net/dajee/xuanchu/internal/render"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	taskrcparser "git.dajee.net/dajee/xuanchu/internal/taskrc"
	"github.com/spf13/cobra"
)

func newShowCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "显示当前配置",
		Args:  cobra.NoArgs,
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
		Use:   "config",
		Short: "查看和修改配置",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newConfigGetCommand(opts))
	cmd.AddCommand(newConfigSetCommand(opts))
	cmd.AddCommand(newConfigUnsetCommand(opts))
	cmd.AddCommand(newConfigListCommand(opts))
	cmd.AddCommand(newConfigSchemaCommand(opts))
	cmd.AddCommand(newConfigImportTaskRCCommand(opts))
	return cmd
}

func newConfigSchemaCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schema",
		Short: "管理共享配置 schema",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newConfigSchemaListCommand(opts))
	cmd.AddCommand(newConfigSchemaGetCommand(opts))
	cmd.AddCommand(newConfigSchemaSetCommand(opts))
	cmd.AddCommand(newConfigSchemaDeleteCommand(opts))
	return cmd
}

func newConfigSchemaListCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "列出当前 workspace 的配置 schema",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			var rows []remote.ConfigSchemaDefinition
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				rows, err = client.ListConfigSchemas(context.Background(), currentOpts.Workspace)
				if err != nil {
					return err
				}
			} else {
				svc, closeFn, err := buildServiceFromCmd(cmd, opts)
				if err != nil {
					return err
				}
				defer closeFn()
				defs, err := svc.ConfigSchemaList()
				if err != nil {
					return err
				}
				rows = configSchemaDefsFromApp(defs)
			}
			for _, row := range rows {
				fmt.Fprintf(cmd.OutOrStdout(), "%s type:%s scopes:%s\n", row.Key, row.ValueType, strings.Join(row.AllowedScopes, ","))
			}
			return nil
		},
	}
}

func newConfigSchemaGetCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "查看单个配置 schema",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			var row remote.ConfigSchemaDefinition
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				row, err = client.GetConfigSchema(context.Background(), currentOpts.Workspace, args[0])
				if err != nil {
					return err
				}
			} else {
				svc, closeFn, err := buildServiceFromCmd(cmd, opts)
				if err != nil {
					return err
				}
				defer closeFn()
				def, ok, err := svc.ConfigSchemaGet(args[0])
				if err != nil {
					return err
				}
				if !ok {
					return fmt.Errorf("unknown config schema key %q", args[0])
				}
				row = configSchemaDefFromApp(def)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s type:%s scopes:%s\n", row.Key, row.ValueType, strings.Join(row.AllowedScopes, ","))
			return nil
		},
	}
}

func newConfigSchemaSetCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> type:<type> scopes:<workspace|project|workspace,project> [label:<text>] [description:<text>] [values:<csv>] [default:<value>] [required:true|false] [secret:true|false]",
		Short: "创建或更新共享配置 schema",
		Args:  cobra.MinimumNArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			input, err := parseConfigSchemaArgs(args)
			if err != nil {
				return err
			}
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				return client.SetConfigSchema(context.Background(), currentOpts.Workspace, input.Key, remote.ConfigSchemaSetInput{
					ValueType:         input.ValueType,
					AllowedScopes:     input.AllowedScopes,
					Label:             input.Label,
					Description:       input.Description,
					EnumValues:        input.EnumValues,
					DefaultValue:      input.DefaultValue,
					Required:          input.Required,
					Secret:            input.Secret,
					ShowOnConsoleHome: input.ShowOnConsoleHome,
				})
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			return svc.ConfigSchemaSet(input)
		},
	}
}

func newConfigSchemaDeleteCommand(opts Options) *cobra.Command {
	var purge bool
	cmd := &cobra.Command{
		Use:   "delete <key>",
		Short: "删除共享配置 schema",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				return client.DeleteConfigSchema(context.Background(), currentOpts.Workspace, args[0], purge)
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			return svc.ConfigSchemaDelete(args[0], purge)
		},
	}
	cmd.Flags().BoolVar(&purge, "purge", false, "also purge all workspace/project values for this key")
	return cmd
}

func parseConfigSchemaArgs(args []string) (app.ConfigSchemaInput, error) {
	input := app.ConfigSchemaInput{Key: strings.TrimSpace(args[0])}
	values, err := parseKeyValueArgs(args[1:], map[string]bool{
		"type":        true,
		"scopes":      true,
		"label":       true,
		"description": true,
		"values":      true,
		"default":     true,
		"required":    true,
		"secret":      true,
	})
	if err != nil {
		return app.ConfigSchemaInput{}, err
	}
	input.ValueType = values["type"]
	input.AllowedScopes = splitCSV(values["scopes"])
	input.Label = values["label"]
	input.Description = values["description"]
	input.EnumValues = splitCSV(values["values"])
	if value, ok := values["default"]; ok {
		v := value
		input.DefaultValue = &v
	}
	if value, ok := values["required"]; ok {
		input.Required = strings.EqualFold(value, "true")
	}
	if value, ok := values["secret"]; ok {
		input.Secret = strings.EqualFold(value, "true")
	}
	return input, nil
}

func splitCSV(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func configSchemaDefsFromApp(rows []app.ConfigDefinitionView) []remote.ConfigSchemaDefinition {
	out := make([]remote.ConfigSchemaDefinition, 0, len(rows))
	for _, row := range rows {
		out = append(out, configSchemaDefFromApp(row))
	}
	return out
}

func configSchemaDefFromApp(row app.ConfigDefinitionView) remote.ConfigSchemaDefinition {
	return remote.ConfigSchemaDefinition{
		Key:               row.Key,
		ValueType:         row.ValueType,
		AllowedScopes:     row.AllowedScopes,
		Label:             row.Label,
		Description:       row.Description,
		EnumValues:        row.EnumValues,
		DefaultValue:      row.DefaultValue,
		Required:          row.Required,
		Secret:            row.Secret,
		ShowOnConsoleHome: row.ShowOnConsoleHome,
		CreatedAt:         row.CreatedAt,
		ModifiedAt:        row.ModifiedAt,
	}
}

func newConfigGetCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "获取配置项的值",
		Args:  cobra.ExactArgs(1),
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
		Use:   "set <key> <value>",
		Short: "设置配置项",
		Args:  cobra.ExactArgs(2),
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
		Use:   "unset <key>",
		Short: "删除配置项",
		Args:  cobra.ExactArgs(1),
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
		Use:   "list",
		Short: "列出所有配置项",
		Args:  cobra.NoArgs,
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
		Use:   "import-taskrc <path>",
		Short: "从 taskrc 文件导入配置",
		Args:  cobra.ExactArgs(1),
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
		DBURL:   opts.DBURL,
		JSON:    opts.JSON,
		NoColor: opts.NoColor,
		Env:     env,
	})
	if err != nil {
		return config.Runtime{}, err
	}
	store, err := storage.Open(cfg.DatabaseTarget())
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
	case "color", "json", "date.format", "database.path", "active.user", "active.workspace", "active.context", "migration.m5.projects.skipped":
		return true
	default:
		return false
	}
}
