package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newContextCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "管理任务上下文过滤器",
		Args:  cobra.NoArgs,
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
		Use:   "define <name> <filter...>",
		Short: "定义上下文过滤器",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				if err := client.DefineContext(context.Background(), currentOpts.Workspace, args[0], strings.Join(args[1:], " ")); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Defined context %s\n", args[0])
				return nil
			}
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
		Use:   "use <name>",
		Short: "激活上下文",
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
				return client.UseContext(context.Background(), currentOpts.Workspace, args[0])
			}
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
		Use:   "none",
		Short: "清除当前活跃上下文",
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
				return client.ClearContext(context.Background(), currentOpts.Workspace)
			}
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
		Use:   "show",
		Short: "显示当前活跃上下文",
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
				contexts, active, err := client.ListContexts(context.Background(), currentOpts.Workspace)
				if err != nil {
					return err
				}
				for _, ctx := range contexts {
					if ctx.Name == active {
						fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", ctx.Name, ctx.FilterSource)
						return nil
					}
				}
				return nil
			}
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
		Use:   "list",
		Short: "列出所有上下文",
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
				contexts, _, err := client.ListContexts(context.Background(), currentOpts.Workspace)
				if err != nil {
					return err
				}
				for _, ctx := range contexts {
					fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", ctx.Name, ctx.FilterSource)
				}
				return nil
			}
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
		Use:   "delete <name>",
		Short: "删除上下文",
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
				return client.DeleteContext(context.Background(), currentOpts.Workspace, args[0])
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			return svc.ContextDelete(args[0])
		},
	}
}
