package cli

import (
	"context"
	"fmt"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/remote"
	"git.dajee.net/dajee/xuanchu/internal/render"
	"github.com/spf13/cobra"
)

func newUserCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "管理用户",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newUserListCommand(opts))
	cmd.AddCommand(newUserAddCommand(opts))
	cmd.AddCommand(newUserUseCommand(opts))
	cmd.AddCommand(newUserInfoCommand(opts))
	cmd.AddCommand(newUserBindCommand(opts))
	cmd.AddCommand(newUserUnbindCommand(opts))
	return cmd
}

func newUserListCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "列出所有用户",
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
				users, err := client.ListUsers(context.Background())
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					views := make([]app.UserView, 0, len(users))
					for _, u := range users {
						views = append(views, u)
					}
					return render.JSON(cmd.OutOrStdout(), userViewsForJSON(views))
				}
				for _, user := range users {
					active := " "
					if user.Active {
						active = "*"
					}
					email := ""
					if user.Email != nil {
						email = *user.Email
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s\n", active, user.Name, email)
				}
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			users, err := svc.ListUsers()
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), userViewsForJSON(users))
			}
			for _, user := range users {
				active := " "
				if user.Active {
					active = "*"
				}
				email := ""
				if user.Email != nil {
					email = *user.Email
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s\n", active, user.Name, email)
			}
			return nil
		},
	}
}

func newUserAddCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "add <name> [email:<email>]",
		Short: "创建用户",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if err := cobra.MinimumNArgs(1)(cmd, args); err != nil {
				return err
			}
			input, err := parseUserAddArgs(args)
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
				remoteInput := remote.AddUserInput{Name: input.Name}
				if input.Email != "" {
					remoteInput.Email = &input.Email
				}
				user, err := client.AddUser(context.Background(), remoteInput)
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), userViewForJSON(user))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Created user %s\n", user.Name)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			user, err := svc.AddUser(input)
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), userViewForJSON(user))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created user %s\n", user.Name)
			return nil
		},
	}
}

func newUserUseCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "use <name|email|uuid>",
		Short: "切换当前用户",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if err := remoteUnsupported(currentOpts, "user use"); err != nil {
				return err
			}
			if err := cobra.ExactArgs(1)(cmd, args); err != nil {
				return err
			}
			currentOpts.Workspace = ""
			svc, closeFn, err := buildServiceFromOpts(currentOpts)
			if err != nil {
				return err
			}
			defer closeFn()
			if err := svc.UseUser(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Switched active user to %s\n", args[0])
			return nil
		},
	}
}

func newUserInfoCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "info [name|email|uuid]",
		Short: "显示用户详情",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				// 远程模式下默认获取当前用户（使用 local 用户）
				ref := "local"
				if len(args) == 1 {
					ref = args[0]
				}
				user, err := client.UserInfo(context.Background(), ref)
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), userViewForJSON(user))
				}
				email := ""
				if user.Email != nil {
					email = *user.Email
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Name: %s\nEmail: %s\n", user.Name, email)
				if len(user.ExternalIDs) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "External IDs:\n")
					for _, eid := range user.ExternalIDs {
						fmt.Fprintf(cmd.OutOrStdout(), "  %s:%s\n", eid.Provider, eid.ExternalID)
					}
				}
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			ref := svc.Runtime().ActorName
			if len(args) == 1 {
				ref = args[0]
			}
			user, err := svc.UserInfo(ref)
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), userViewForJSON(user))
			}
			email := ""
			if user.Email != nil {
				email = *user.Email
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Name: %s\nEmail: %s\n", user.Name, email)
			if len(user.ExternalIDs) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "External IDs:\n")
				for _, eid := range user.ExternalIDs {
					fmt.Fprintf(cmd.OutOrStdout(), "  %s:%s\n", eid.Provider, eid.ExternalID)
				}
			}
			return nil
		},
	}
}

func parseUserAddArgs(args []string) (app.AddUserInput, error) {
	input := app.AddUserInput{Name: args[0]}
	values, err := parseKeyValueArgs(args[1:], map[string]bool{"email": true})
	if err != nil {
		return app.AddUserInput{}, err
	}
	input.Email = values["email"]
	return input, nil
}

func userViewsForJSON(users []app.UserView) []map[string]any {
	out := make([]map[string]any, 0, len(users))
	for _, user := range users {
		out = append(out, userViewForJSON(user))
	}
	return out
}

func userViewForJSON(user app.UserView) map[string]any {
	extIDs := make([]map[string]string, 0, len(user.ExternalIDs))
	for _, eid := range user.ExternalIDs {
		extIDs = append(extIDs, map[string]string{"provider": eid.Provider, "external_id": eid.ExternalID})
	}
	return map[string]any{
		"id":                   user.ID,
		"name":                 user.Name,
		"display_name":         user.DisplayName,
		"email":                user.Email,
		"default_workspace_id": user.DefaultWorkspaceID,
		"external_ids":         extIDs,
		"active":               user.Active,
		"created_at":           user.CreatedAt,
		"modified_at":          user.ModifiedAt,
	}
}

func newUserBindCommand(opts Options) *cobra.Command {
	var userRef string
	cmd := &cobra.Command{
		Use:   "bind <provider:external_id>",
		Short: "绑定外部身份",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			provider, externalID, ok := parseProviderExternalID(args[0])
			if !ok {
				return fmt.Errorf("invalid external ID format %q; expected provider:external_id", args[0])
			}
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				ref := userRef
				if ref == "" {
					ref = "local"
				}
				return client.BindExternalID(context.Background(), ref, provider, externalID)
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			targetUserID := svc.Runtime().ActorUserID
			if userRef != "" {
				user, err := svc.UserInfo(userRef)
				if err != nil {
					return err
				}
				targetUserID = user.ID
			}
			if err := svc.BindExternalID(targetUserID, provider, externalID); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Bound %s:%s\n", provider, externalID)
			return nil
		},
	}
	cmd.Flags().StringVar(&userRef, "user", "", "target user (name, email, or UUID); defaults to current user")
	return cmd
}

func newUserUnbindCommand(opts Options) *cobra.Command {
	var userRef string
	cmd := &cobra.Command{
		Use:   "unbind <provider:external_id>",
		Short: "解绑外部身份",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			provider, externalID, ok := parseProviderExternalID(args[0])
			if !ok {
				return fmt.Errorf("invalid external ID format %q; expected provider:external_id", args[0])
			}
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				ref := userRef
				if ref == "" {
					ref = "local"
				}
				return client.UnbindExternalID(context.Background(), ref, provider, externalID)
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			targetUserID := svc.Runtime().ActorUserID
			if userRef != "" {
				user, err := svc.UserInfo(userRef)
				if err != nil {
					return err
				}
				targetUserID = user.ID
			}
			if err := svc.UnbindExternalID(targetUserID, provider, externalID); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Unbound %s:%s\n", provider, externalID)
			return nil
		},
	}
	cmd.Flags().StringVar(&userRef, "user", "", "target user (name, email, or UUID); defaults to current user")
	return cmd
}

func parseProviderExternalID(s string) (string, string, bool) {
	idx := strings.Index(s, ":")
	if idx <= 0 || idx == len(s)-1 {
		return "", "", false
	}
	return s[:idx], s[idx+1:], true
}

func parseKeyValueArgs(args []string, allowed map[string]bool) (map[string]string, error) {
	values := map[string]string{}
	for _, arg := range args {
		key, value, ok := strings.Cut(arg, ":")
		if !ok || key == "" {
			return nil, fmt.Errorf("invalid argument %q; expected key:value", arg)
		}
		if !allowed[key] {
			return nil, fmt.Errorf("unknown argument %q", arg)
		}
		values[key] = value
	}
	return values, nil
}
