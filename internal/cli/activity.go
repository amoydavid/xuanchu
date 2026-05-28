package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newStartCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "start <target>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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
		Use:  "stop <target>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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
