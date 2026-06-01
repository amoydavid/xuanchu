package cli

import (
	"context"
	"fmt"
	"sort"

	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/remote"
	"github.com/dajee/taskg/internal/render"
	"github.com/spf13/cobra"
)

func newProjectCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:  "project",
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(newProjectListCommand(opts))
	cmd.AddCommand(newProjectAddCommand(opts))
	cmd.AddCommand(newProjectInfoCommand(opts))
	cmd.AddCommand(newProjectModifyCommand(opts))
	cmd.AddCommand(newProjectArchiveCommand(opts))
	cmd.AddCommand(newProjectConfigCommand(opts))
	return cmd
}

func newProjectListCommand(opts Options) *cobra.Command {
	var includeArchived bool
	cmd := &cobra.Command{
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
				projects, err := client.ListProjects(context.Background(), currentOpts.Workspace, includeArchived)
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), projectViewsForJSON(projects))
				}
				for _, project := range projects {
					archived := ""
					if project.ArchivedAt != nil {
						archived = " archived"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%s %s%s\n", project.Slug, project.Name, archived)
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
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), projectViewsForJSON(projects))
			}
			for _, project := range projects {
				archived := ""
				if project.ArchivedAt != nil {
					archived = " archived"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s %s%s\n", project.Slug, project.Name, archived)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&includeArchived, "all", false, "include archived projects")
	return cmd
}

func newProjectAddCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "add <slug> [name:<name>] [description:<text>]",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			input, err := parseProjectAddArgs(args)
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
				project, err := client.AddProject(context.Background(), currentOpts.Workspace, remote.AddProjectInput{
					Slug:        input.Slug,
					Name:        input.Name,
					Description: input.Description,
				})
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), projectViewForJSON(project))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Created project %s\n", project.Slug)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			project, err := svc.AddProject(input)
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), projectViewForJSON(project))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created project %s\n", project.Slug)
			return nil
		},
	}
}

func newProjectInfoCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "info <slug|uuid>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				project, err := client.GetProject(context.Background(), currentOpts.Workspace, args[0])
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), projectViewForJSON(project))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Slug: %s\nName: %s\n", project.Slug, project.Name)
				if project.Description != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "Description: %s\n", project.Description)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Status: %s\nTask Count: %d\n", project.Status, project.TaskCount)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			project, err := svc.ProjectInfo(args[0])
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), projectViewForJSON(project))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Slug: %s\nName: %s\n", project.Slug, project.Name)
			if project.Description != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Description: %s\n", project.Description)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Status: %s\nTask Count: %d\n", project.Status, project.TaskCount)
			return nil
		},
	}
}

func newProjectModifyCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "modify <slug|uuid> [name:<name>] [description:<text>]",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			input, err := parseProjectModifyArgs(args[1:])
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
				project, err := client.ModifyProject(context.Background(), currentOpts.Workspace, args[0], remote.ModifyProjectInput{
					Slug:        input.Slug,
					Name:        input.Name,
					Description: input.Description,
				})
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), projectViewForJSON(project))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Modified project %s\n", args[0])
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			if err := svc.ModifyProject(args[0], input); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Modified project %s\n", args[0])
			return nil
		},
	}
}

func newProjectArchiveCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "archive <slug|uuid>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				project, err := client.ArchiveProject(context.Background(), currentOpts.Workspace, args[0])
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), projectViewForJSON(project))
				}
				if project.TaskCount > 0 {
					fmt.Fprintf(cmd.ErrOrStderr(), "taskg: warning: archived project %s still has %d non-deleted task(s)\n", project.Slug, project.TaskCount)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Archived project %s\n", args[0])
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			project, err := svc.ArchiveProject(args[0])
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), projectViewForJSON(project))
			}
			if project.TaskCount > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "taskg: warning: archived project %s still has %d non-deleted task(s)\n", project.Slug, project.TaskCount)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Archived project %s\n", args[0])
			return nil
		},
	}
}

func newProjectConfigCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:  "config",
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(newProjectConfigGetCommand(opts))
	cmd.AddCommand(newProjectConfigSetCommand(opts))
	cmd.AddCommand(newProjectConfigUnsetCommand(opts))
	cmd.AddCommand(newProjectConfigListCommand(opts))
	return cmd
}

func newProjectConfigGetCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "get <project> <key>",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				value, err := client.ProjectConfigGet(context.Background(), currentOpts.Workspace, args[0], args[1])
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), value)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			value, ok, err := svc.ProjectConfigGet(args[0], args[1])
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("unknown project config key %q", args[1])
			}
			fmt.Fprintln(cmd.OutOrStdout(), value)
			return nil
		},
	}
}

func newProjectConfigSetCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "set <project> <key> <value>",
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				return client.ProjectConfigSet(context.Background(), currentOpts.Workspace, args[0], args[1], args[2])
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			return svc.ProjectConfigSet(args[0], args[1], args[2])
		},
	}
}

func newProjectConfigUnsetCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "unset <project> <key>",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				return client.ProjectConfigUnset(context.Background(), currentOpts.Workspace, args[0], args[1])
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			return svc.ProjectConfigUnset(args[0], args[1])
		},
	}
}

func newProjectConfigListCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "list <project>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			var values map[string]string
			var err error
			if remoteMode, _, modeErr := isRemoteMode(currentOpts); modeErr != nil {
				return modeErr
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				values, err = client.ProjectConfigList(context.Background(), currentOpts.Workspace, args[0])
			} else {
				svc, closeFn, err := buildServiceFromCmd(cmd, opts)
				if err != nil {
					return err
				}
				defer closeFn()
				values, err = svc.ProjectConfigList(args[0])
			}
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
		},
	}
}

func parseProjectAddArgs(args []string) (app.AddProjectInput, error) {
	input := app.AddProjectInput{Slug: args[0]}
	values, err := parseKeyValueArgs(args[1:], map[string]bool{"name": true, "description": true})
	if err != nil {
		return app.AddProjectInput{}, err
	}
	input.Name = values["name"]
	input.Description = values["description"]
	return input, nil
}

func parseProjectModifyArgs(args []string) (app.ModifyProjectInput, error) {
	input := app.ModifyProjectInput{}
	values, err := parseKeyValueArgs(args, map[string]bool{"name": true, "description": true})
	if err != nil {
		return app.ModifyProjectInput{}, err
	}
	if value, ok := values["name"]; ok {
		v := value
		input.Name = &v
	}
	if value, ok := values["description"]; ok {
		v := value
		input.Description = &v
	}
	return input, nil
}

func projectViewsForJSON(projects []app.ProjectView) []map[string]any {
	out := make([]map[string]any, 0, len(projects))
	for _, project := range projects {
		out = append(out, projectViewForJSON(project))
	}
	return out
}

func projectViewForJSON(project app.ProjectView) map[string]any {
	return map[string]any{
		"id":           project.ID,
		"workspace_id": project.WorkspaceID,
		"slug":         project.Slug,
		"name":         project.Name,
		"description":  project.Description,
		"status":       project.Status,
		"task_count":   project.TaskCount,
		"created_at":   project.CreatedAt,
		"modified_at":  project.ModifiedAt,
		"archived_at":  project.ArchivedAt,
	}
}
