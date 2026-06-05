package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func newStartCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "start <target>",
		Short: "开始任务",
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
				target, err := resolveRemoteTaskTarget(context.Background(), client, currentOpts, args[0])
				if err != nil {
					return err
				}
				if _, err := client.StartTask(context.Background(), currentOpts.Workspace, target); err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), "Started task", args[0])
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			if err := svc.Start(args[0]); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Started task", args[0])
			return nil
		},
	}
}

func newStopCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "stop <target>",
		Short: "停止任务",
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
				target, err := resolveRemoteTaskTarget(context.Background(), client, currentOpts, args[0])
				if err != nil {
					return err
				}
				if _, err := client.StopTask(context.Background(), currentOpts.Workspace, target); err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), "Stopped task", args[0])
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			if err := svc.Stop(args[0]); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Stopped task", args[0])
			return nil
		},
	}
}
