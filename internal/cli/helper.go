package cli

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/dom"
	"github.com/dajee/taskg/internal/query"
	"github.com/dajee/taskg/internal/remote"
	"github.com/spf13/cobra"
)

func RuntimeEnv() map[string]string {
	values := map[string]string{}
	for _, key := range []string{
		"HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME",
		"TASKG_DB", "TASKG_DB_URL", "TASKG_SERVER", "TASKG_TOKEN",
	} {
		if value, ok := os.LookupEnv(key); ok {
			values[key] = value
		}
	}
	return values
}

func remoteUnsupported(opts Options, name string) error {
	remoteMode, _, err := isRemoteMode(opts)
	if err != nil {
		return err
	}
	if remoteMode {
		return app.RuntimeError{
			Code:    "remote_unsupported_command",
			Message: fmt.Sprintf("command %q is not supported in remote mode", name),
		}
	}
	return nil
}

func newGetCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "_get [expr...]",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				for _, expr := range args {
					dot := strings.Index(expr, ".")
					if dot < 0 {
						return fmt.Errorf("invalid DOM expression %q", expr)
					}
					target := expr[:dot]
					field := expr[dot+1:]
					resolved, err := resolveRemoteTaskTarget(context.Background(), client, currentOpts, target)
					if err != nil {
						return err
					}
					tsk, err := client.GetTask(context.Background(), currentOpts.Workspace, resolved)
					if err != nil {
						return err
					}
					var urg float64
					if field == "urgency" {
						explain, err := client.ExplainUrgency(context.Background(), currentOpts.Workspace, resolved)
						if err != nil {
							return err
						}
						urg = explain.Total
					}
					value, err := dom.Resolve(tsk, field, urg)
					if err != nil {
						return err
					}
					fmt.Fprintln(cmd.OutOrStdout(), value)
				}
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()

			for _, expr := range args {
				dot := strings.Index(expr, ".")
				if dot < 0 {
					return fmt.Errorf("invalid DOM expression %q", expr)
				}
				target := expr[:dot]
				field := expr[dot+1:]

				tsk, err := svc.ResolveTarget(target)
				if err != nil {
					return err
				}

				var urg float64
				if field == "urgency" {
					explain, err := svc.ExplainUrgency(target)
					if err != nil {
						return err
					}
					urg = explain.Total
				}

				value, err := dom.Resolve(tsk, field, urg)
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), value)
			}
			return nil
		},
	}
}

func newIDsCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "_ids [filters...]",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				matches, err := client.ListTasks(context.Background(), remote.ListTasksInput{
					Workspace: currentOpts.Workspace,
					Project:   currentOpts.Project,
					ProjectID: currentOpts.ProjectID,
					Filters:   append([]string(nil), args...),
					NoContext: currentOpts.NoContext,
				})
				if err != nil {
					return err
				}
				matched := make(map[string]struct{}, len(matches))
				for _, tsk := range matches {
					matched[tsk.UUID] = struct{}{}
				}
				workingSet, err := remoteDefaultWorkingSet(context.Background(), client, currentOpts)
				if err != nil {
					return err
				}
				for i, tsk := range workingSet {
					if _, ok := matched[tsk.UUID]; ok {
						fmt.Fprintln(cmd.OutOrStdout(), i+1)
					}
				}
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()

			var expr query.Expr
			if len(args) > 0 {
				expr, err = query.ParseFilterExpr(args)
				if err != nil {
					return err
				}
			}

			ids, err := svc.IDs(app.ListInput{Query: expr})
			if err != nil {
				return err
			}
			for _, id := range ids {
				fmt.Fprintln(cmd.OutOrStdout(), id)
			}
			return nil
		},
	}
}

func newUUIDsCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "_uuids [filters...]",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				tasks, err := client.ListTasks(context.Background(), remote.ListTasksInput{
					Workspace: currentOpts.Workspace,
					Project:   currentOpts.Project,
					ProjectID: currentOpts.ProjectID,
					Filters:   append([]string(nil), args...),
					NoContext: currentOpts.NoContext,
				})
				if err != nil {
					return err
				}
				for _, tsk := range tasks {
					fmt.Fprintln(cmd.OutOrStdout(), tsk.UUID)
				}
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()

			var expr query.Expr
			if len(args) > 0 {
				expr, err = query.ParseFilterExpr(args)
				if err != nil {
					return err
				}
			}

			uuids, err := svc.UUIDs(app.ListInput{Query: expr})
			if err != nil {
				return err
			}
			for _, uuid := range uuids {
				fmt.Fprintln(cmd.OutOrStdout(), uuid)
			}
			return nil
		},
	}
}

func newProjectsCommand(opts Options) *cobra.Command {
	var includeArchived bool
	cmd := &cobra.Command{
		Use:  "_projects",
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
				projects, err := client.ListProjects(context.Background(), currentOpts.Workspace, includeArchived)
				if err != nil {
					return err
				}
				for _, p := range projects {
					fmt.Fprintln(cmd.OutOrStdout(), p.Slug)
				}
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()

			projects, err := svc.ListProjects(includeArchived)
			if err != nil {
				return err
			}
			for _, p := range projects {
				fmt.Fprintln(cmd.OutOrStdout(), p.Slug)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&includeArchived, "all", false, "include archived projects")
	return cmd
}

func newTagsCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "_tags",
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
				tasks, err := client.ListTasks(context.Background(), remote.ListTasksInput{
					Workspace: currentOpts.Workspace,
					Project:   currentOpts.Project,
					ProjectID: currentOpts.ProjectID,
					NoContext: currentOpts.NoContext,
				})
				if err != nil {
					return err
				}
				tags := map[string]bool{}
				for _, tsk := range tasks {
					for _, tag := range tsk.Tags {
						if tag != "" {
							tags[tag] = true
						}
					}
				}
				keys := sortedKeys(tags)
				for _, tag := range keys {
					fmt.Fprintln(cmd.OutOrStdout(), tag)
				}
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()

			tags, err := svc.Tags()
			if err != nil {
				return err
			}
			for _, tag := range tags {
				fmt.Fprintln(cmd.OutOrStdout(), tag)
			}
			return nil
		},
	}
}

func newUDAsCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "_udas",
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
				names := map[string]bool{}
				for key := range values {
					if strings.HasPrefix(key, "uda.") && strings.HasSuffix(key, ".type") {
						name := strings.TrimSuffix(strings.TrimPrefix(key, "uda."), ".type")
						if name != "" {
							names[name] = true
						}
					}
				}
				for _, name := range sortedKeys(names) {
					fmt.Fprintln(cmd.OutOrStdout(), name)
				}
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			defs, err := svc.ListUDAs()
			if err != nil {
				return err
			}
			for _, def := range defs {
				fmt.Fprintln(cmd.OutOrStdout(), def.Name)
			}
			return nil
		},
	}
}

func newUniqueCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "_unique <attr> [filters...]",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				tasks, err := client.ListTasks(context.Background(), remote.ListTasksInput{
					Workspace: currentOpts.Workspace,
					Project:   currentOpts.Project,
					ProjectID: currentOpts.ProjectID,
					Filters:   append([]string(nil), args[1:]...),
					NoContext: currentOpts.NoContext,
				})
				if err != nil {
					return err
				}
				values := map[string]bool{}
				field := strings.TrimPrefix(args[0], "uda.")
				for _, tsk := range tasks {
					switch args[0] {
					case "project":
						if tsk.Project != nil && *tsk.Project != "" {
							values[*tsk.Project] = true
						}
					case "priority":
						if tsk.Priority != nil && *tsk.Priority != "" {
							values[*tsk.Priority] = true
						}
					case "tags":
						for _, tag := range tsk.Tags {
							if tag != "" {
								values[tag] = true
							}
						}
					default:
						if value, ok := tsk.UDAs[field]; ok && value.Raw != "" {
							values[value.Raw] = true
						}
					}
				}
				for _, value := range sortedKeys(values) {
					fmt.Fprintln(cmd.OutOrStdout(), value)
				}
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			var expr query.Expr
			if len(args) > 1 {
				expr, err = query.ParseFilterExpr(args[1:])
				if err != nil {
					return err
				}
			}
			values, err := svc.UniqueValues(args[0], app.ListInput{Query: expr})
			if err != nil {
				return err
			}
			for _, value := range values {
				fmt.Fprintln(cmd.OutOrStdout(), value)
			}
			return nil
		},
	}
}

func newShowHelperCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "_show [key...]",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				for _, key := range args {
					if key == "database.path" || key == "remote.server" || key == "remote.token" {
						return app.RuntimeError{Code: "remote_unsupported_command", Message: `remote _show does not expose local or server runtime paths`}
					}
				}
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				values, err := client.ListConfig(context.Background(), currentOpts.Workspace)
				if err != nil {
					return err
				}
				if len(args) == 0 {
					for _, key := range sortedStringMapKeys(values) {
						fmt.Fprintf(cmd.OutOrStdout(), "%s=%s\n", key, values[key])
					}
					return nil
				}
				for _, key := range args {
					value, ok := values[key]
					if !ok {
						return fmt.Errorf("unsupported remote _show key %q", key)
					}
					fmt.Fprintln(cmd.OutOrStdout(), value)
				}
				return nil
			}
			rt, err := runtimeFromOptions(currentOpts)
			if err != nil {
				return err
			}
			if len(args) == 0 {
				for _, key := range rt.Keys() {
					if !isPublicConfigKey(key) {
						continue
					}
					value, _ := rt.Get(key)
					fmt.Fprintf(cmd.OutOrStdout(), "%s=%s\n", key, value)
				}
				return nil
			}
			for _, key := range args {
				if key == "context.active" {
					return fmt.Errorf("unsupported legacy key %q", key)
				}
				if !isPublicConfigKey(key) {
					return fmt.Errorf("unsupported internal key %q", key)
				}
				value, _ := rt.Get(key)
				fmt.Fprintln(cmd.OutOrStdout(), value)
			}
			return nil
		},
	}
}

func sortedKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedStringMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func newVersionHelperCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "_version",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			version := opts.Version
			if version == "" {
				version = "dev"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "taskg %s\n", version)
			return nil
		},
	}
}
