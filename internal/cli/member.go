package cli

import (
	"fmt"
	"time"

	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/render"
	"github.com/spf13/cobra"
)

func newMemberCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:  "member",
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(newMemberListCommand(opts))
	cmd.AddCommand(newMemberAddCommand(opts))
	cmd.AddCommand(newMemberRoleCommand(opts))
	return cmd
}

func newMemberListCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "list",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			members, err := svc.ListMembers(currentOpts.Workspace)
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), memberViewsForJSON(members))
			}
			for _, member := range members {
				email := ""
				if member.Email != nil {
					email = *member.Email
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s %s\n", member.Name, email, member.Role, time.Unix(member.JoinedAt, 0).UTC().Format(time.RFC3339))
			}
			return nil
		},
	}
}

func memberViewsForJSON(members []app.MemberView) []map[string]any {
	out := make([]map[string]any, 0, len(members))
	for _, member := range members {
		out = append(out, map[string]any{
			"user_id":     member.UserID,
			"name":        member.Name,
			"email":       member.Email,
			"role":        member.Role,
			"joined_at":   member.JoinedAt,
			"modified_at": member.ModifiedAt,
		})
	}
	return out
}

func newMemberAddCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "add <user> [role:<role>]",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			input := app.AddMemberInput{
				WorkspaceRef: optionsFromCmd(cmd, opts).Workspace,
				UserRef:      args[0],
				Role:         app.Role("member"),
			}
			values, err := parseKeyValueArgs(args[1:], map[string]bool{"role": true})
			if err != nil {
				return err
			}
			if role, ok := values["role"]; ok {
				input.Role = app.Role(role)
			}
			if err := svc.AddMember(input); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Added member %s\n", input.UserRef)
			return nil
		},
	}
}

func newMemberRoleCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "role <user> <owner|admin|member|viewer>",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			if err := svc.ChangeMemberRole(app.ChangeMemberRoleInput{
				WorkspaceRef: optionsFromCmd(cmd, opts).Workspace,
				UserRef:      args[0],
				Role:         app.Role(args[1]),
			}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Updated member %s\n", args[0])
			return nil
		},
	}
}
