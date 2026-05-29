package cli

import (
	"fmt"

	"github.com/dajee/taskg/internal/app"
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
				return render.JSON(cmd.OutOrStdout(), users)
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
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			input := parseUserAddArgs(args)
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
				return render.JSON(cmd.OutOrStdout(), user)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created user %s\n", user.Name)
			return nil
		},
	}
}

func newUserUseCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "use <name|email|uuid>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
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
				return render.JSON(cmd.OutOrStdout(), user)
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

func parseUserAddArgs(args []string) app.AddUserInput {
	input := app.AddUserInput{Name: args[0]}
	for _, arg := range args[1:] {
		if value, ok := trimKV(arg, "email"); ok {
			input.Email = value
		}
	}
	return input
}
