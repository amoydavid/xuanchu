package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/remote"
	"github.com/dajee/taskg/internal/render"
	"github.com/spf13/cobra"
)

func newUserCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:  "user",
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(newUserListCommand(opts))
	cmd.AddCommand(newUserAddCommand(opts))
	cmd.AddCommand(newUserUseCommand(opts))
	cmd.AddCommand(newUserInfoCommand(opts))
	return cmd
}

func newUserListCommand(opts Options) *cobra.Command {
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
		Use:  "add <name> [email:<email>]",
		Args: cobra.ArbitraryArgs,
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
		Use:  "use <name|email|uuid>",
		Args: cobra.ArbitraryArgs,
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
		Use:  "info [name|email|uuid]",
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
	return map[string]any{
		"id":                   user.ID,
		"name":                 user.Name,
		"email":                user.Email,
		"default_workspace_id": user.DefaultWorkspaceID,
		"active":               user.Active,
		"created_at":           user.CreatedAt,
		"modified_at":          user.ModifiedAt,
	}
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
