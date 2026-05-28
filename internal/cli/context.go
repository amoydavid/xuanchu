package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newContextCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:  "context",
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(newContextDefineCommand(opts))
	cmd.AddCommand(newContextUseCommand(opts))
	cmd.AddCommand(newContextNoneCommand(opts))
	cmd.AddCommand(newContextShowCommand(opts))
	cmd.AddCommand(newContextListCommand(opts))
	cmd.AddCommand(newContextDeleteCommand(opts))
	return cmd
}

func newContextDefineCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "define <name> <filter...>",
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			if err := svc.DefineContext(args[0], strings.Join(args[1:], " ")); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Defined context %s\n", args[0])
			return nil
		},
	}
}

func newContextUseCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "use <name>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			return svc.UseContext(args[0])
		},
	}
}

func newContextNoneCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "none",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			return svc.ContextNone()
		},
	}
}

func newContextShowCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "show",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			show, err := svc.ContextShow()
			if err != nil {
				return err
			}
			if show != "" {
				fmt.Fprintln(cmd.OutOrStdout(), show)
			}
			return nil
		},
	}
}

func newContextListCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "list",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			contexts, err := svc.ContextList()
			if err != nil {
				return err
			}
			for _, ctx := range contexts {
				fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", ctx.Name, ctx.FilterSource)
			}
			return nil
		},
	}
}

func newContextDeleteCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "delete <name>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			return svc.ContextDelete(args[0])
		},
	}
}
