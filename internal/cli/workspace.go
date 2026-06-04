package cli

import (
	"context"
	"fmt"

	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/remote"
	"github.com/dajee/taskg/internal/render"
	"github.com/spf13/cobra"
)

func newWorkspaceCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:  "workspace",
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(newWorkspaceListCommand(opts))
	cmd.AddCommand(newWorkspaceAddCommand(opts))
	cmd.AddCommand(newWorkspaceUseCommand(opts))
	cmd.AddCommand(newWorkspaceInfoCommand(opts))
	cmd.AddCommand(newWorkspaceModifyCommand(opts))
	cmd.AddCommand(newWorkspaceArchiveCommand(opts))
	return cmd
}

func newWorkspaceListCommand(opts Options) *cobra.Command {
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
				workspaces, err := client.ListWorkspaces(context.Background(), includeArchived)
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), workspaceViewsForJSON(workspaces))
				}
				for _, ws := range workspaces {
					active := " "
					if ws.Active {
						active = "*"
					}
					archived := ""
					if ws.ArchivedAt != nil {
						archived = " archived"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s %s%s\n", active, ws.Slug, ws.Name, ws.Role, archived)
				}
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			workspaces, err := svc.ListWorkspaces(includeArchived)
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), workspaceViewsForJSON(workspaces))
			}
			for _, ws := range workspaces {
				active := " "
				if ws.Active {
					active = "*"
				}
				archived := ""
				if ws.ArchivedAt != nil {
					archived = " archived"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s %s%s\n", active, ws.Slug, ws.Name, ws.Role, archived)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&includeArchived, "all", false, "include archived workspaces")
	return cmd
}

func newWorkspaceAddCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "add <slug> [name:<name>] [description:<text>] [visibility:private|team|public]",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if err := cobra.MinimumNArgs(1)(cmd, args); err != nil {
				return err
			}
			input, err := parseWorkspaceAddArgs(args)
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
				_, err = client.AddWorkspace(context.Background(), remote.AddWorkspaceInput{
					Slug: input.Slug, Name: input.Name, Description: input.Description, Visibility: input.Visibility,
				})
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Created workspace %s\n", input.Slug)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			workspace, err := svc.AddWorkspace(input)
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), workspaceViewForJSON(workspace))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created workspace %s\n", workspace.Slug)
			return nil
		},
	}
}

func newWorkspaceUseCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "use <slug|uuid>",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if err := cobra.ExactArgs(1)(cmd, args); err != nil {
				return err
			}
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				if _, err := client.UseWorkspace(context.Background(), args[0]); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Switched active workspace to %s\n", args[0])
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			if err := svc.UseWorkspace(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Switched active workspace to %s\n", args[0])
			return nil
		},
	}
}

func newWorkspaceInfoCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "info [slug|uuid]",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				ref := ""
				if len(args) == 1 {
					ref = args[0]
				} else {
					// 使用当前 workspace
					workspaces, err := client.ListWorkspaces(context.Background(), false)
					if err != nil {
						return err
					}
					for _, ws := range workspaces {
						if ws.Active {
							ref = ws.Slug
							break
						}
					}
					if ref == "" && len(workspaces) > 0 {
						ref = workspaces[0].Slug
					}
				}
				if ref == "" {
					return fmt.Errorf("no workspace available")
				}
				workspace, err := client.WorkspaceInfo(context.Background(), ref)
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), workspaceViewForJSON(workspace))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Slug: %s\nName: %s\nVisibility: %s\n", workspace.Slug, workspace.Name, workspace.Visibility)
				if workspace.Description != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "Description: %s\n", workspace.Description)
				}
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			ref := svc.Runtime().WorkspaceSlug
			if len(args) == 1 {
				ref = args[0]
			}
			workspace, err := svc.WorkspaceInfo(ref)
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), workspaceViewForJSON(workspace))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Slug: %s\nName: %s\nVisibility: %s\n", workspace.Slug, workspace.Name, workspace.Visibility)
			if workspace.Description != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Description: %s\n", workspace.Description)
			}
			return nil
		},
	}
}

func newWorkspaceModifyCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "modify <slug|uuid> [name:<name>] [description:<text>] [visibility:private|team|public]",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if err := cobra.MinimumNArgs(1)(cmd, args); err != nil {
				return err
			}
			input, err := parseWorkspaceModifyArgs(args[1:])
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
				if err := client.ModifyWorkspace(context.Background(), args[0], remote.ModifyWorkspaceInput{
					Name: input.Name, Description: input.Description, Visibility: input.Visibility,
				}); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Modified workspace %s\n", args[0])
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			if err := svc.ModifyWorkspace(args[0], input); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Modified workspace %s\n", args[0])
			return nil
		},
	}
}

func newWorkspaceArchiveCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "archive <slug|uuid>",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if err := cobra.ExactArgs(1)(cmd, args); err != nil {
				return err
			}
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				if err := client.ArchiveWorkspace(context.Background(), args[0]); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Archived workspace %s\n", args[0])
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			if err := svc.ArchiveWorkspace(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Archived workspace %s\n", args[0])
			return nil
		},
	}
}

func parseWorkspaceAddArgs(args []string) (app.AddWorkspaceInput, error) {
	input := app.AddWorkspaceInput{Slug: args[0]}
	values, err := parseKeyValueArgs(args[1:], map[string]bool{"name": true, "description": true, "visibility": true})
	if err != nil {
		return app.AddWorkspaceInput{}, err
	}
	input.Name = values["name"]
	input.Description = values["description"]
	input.Visibility = values["visibility"]
	return input, nil
}

func parseWorkspaceModifyArgs(args []string) (app.ModifyWorkspaceInput, error) {
	input := app.ModifyWorkspaceInput{}
	values, err := parseKeyValueArgs(args, map[string]bool{"name": true, "description": true, "visibility": true})
	if err != nil {
		return app.ModifyWorkspaceInput{}, err
	}
	if value, ok := values["name"]; ok {
		v := value
		input.Name = &v
	}
	if value, ok := values["description"]; ok {
		v := value
		input.Description = &v
	}
	if value, ok := values["visibility"]; ok {
		v := value
		input.Visibility = &v
	}
	return input, nil
}

func workspaceViewsForJSON(workspaces []app.WorkspaceView) []map[string]any {
	out := make([]map[string]any, 0, len(workspaces))
	for _, workspace := range workspaces {
		out = append(out, workspaceViewForJSON(workspace))
	}
	return out
}

func workspaceViewForJSON(workspace app.WorkspaceView) map[string]any {
	m := map[string]any{
		"id":           workspace.ID,
		"slug":         workspace.Slug,
		"name":         workspace.Name,
		"description":  workspace.Description,
		"visibility":   workspace.Visibility,
		"archived_at":  workspace.ArchivedAt,
		"role":         workspace.Role,
		"active":       workspace.Active,
		"created_at":   workspace.CreatedAt,
		"modified_at":  workspace.ModifiedAt,
	}
	if workspace.CreatedBy != nil {
		m["created_by"] = userInfoToJSONMap(workspace.CreatedBy)
	}
	return m
}
