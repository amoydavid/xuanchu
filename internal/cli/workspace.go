package cli

import (
	"fmt"
	"strings"

	"github.com/dajee/taskg/internal/app"
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
				return render.JSON(cmd.OutOrStdout(), workspaces)
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
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			input := parseWorkspaceAddArgs(args)
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
				return render.JSON(cmd.OutOrStdout(), workspace)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created workspace %s\n", workspace.Slug)
			return nil
		},
	}
}

func newWorkspaceUseCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "use <slug|uuid>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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
				return render.JSON(cmd.OutOrStdout(), workspace)
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
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			input := parseWorkspaceModifyArgs(args[1:])
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
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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

func parseWorkspaceAddArgs(args []string) app.AddWorkspaceInput {
	input := app.AddWorkspaceInput{Slug: args[0]}
	for _, arg := range args[1:] {
		if value, ok := trimKV(arg, "name"); ok {
			input.Name = value
			continue
		}
		if value, ok := trimKV(arg, "description"); ok {
			input.Description = value
			continue
		}
		if value, ok := trimKV(arg, "visibility"); ok {
			input.Visibility = value
		}
	}
	return input
}

func parseWorkspaceModifyArgs(args []string) app.ModifyWorkspaceInput {
	input := app.ModifyWorkspaceInput{}
	for _, arg := range args {
		if value, ok := trimKV(arg, "name"); ok {
			v := value
			input.Name = &v
			continue
		}
		if value, ok := trimKV(arg, "description"); ok {
			v := value
			input.Description = &v
			continue
		}
		if value, ok := trimKV(arg, "visibility"); ok {
			v := value
			input.Visibility = &v
		}
	}
	return input
}

func trimKV(arg, key string) (string, bool) {
	prefix := key + ":"
	if !strings.HasPrefix(arg, prefix) {
		return "", false
	}
	return strings.TrimPrefix(arg, prefix), true
}
