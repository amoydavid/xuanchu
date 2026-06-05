package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

func newAnnotateCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "annotate <target> <description...>",
		Short: "为任务添加注解",
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
				target, err := resolveRemoteTaskTarget(context.Background(), client, currentOpts, args[0])
				if err != nil {
					return err
				}
				if _, err := client.AnnotateTask(context.Background(), currentOpts.Workspace, target, strings.Join(args[1:], " ")); err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), "Annotated task", args[0])
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			if err := svc.Annotate(args[0], strings.Join(args[1:], " ")); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Annotated task", args[0])
			return nil
		},
	}
}

func newDenotateCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "denotate <target> <index>",
		Short: "删除任务的注解",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				target, err := resolveRemoteTaskTarget(context.Background(), client, currentOpts, args[0])
				if err != nil {
					return err
				}
				index, err := strconv.Atoi(args[1])
				if err != nil {
					return err
				}
				if _, err := client.DenotateTask(context.Background(), currentOpts.Workspace, target, index); err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), "Removed annotation from task", args[0])
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			index, err := strconv.Atoi(args[1])
			if err != nil {
				return err
			}
			if err := svc.Denotate(args[0], index); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Removed annotation from task", args[0])
			return nil
		},
	}
}
