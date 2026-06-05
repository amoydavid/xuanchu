package cli

import (
	"context"
	"fmt"

	"git.dajee.net/dajee/xuanchu/internal/render"
	"github.com/spf13/cobra"
)

func newUrgencyCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "urgency <target>",
		Short: "计算任务的 urgency 值",
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
				explain, err := client.ExplainUrgency(context.Background(), currentOpts.Workspace, target)
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), explain)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Urgency for %s\n", explain.UUID)
				fmt.Fprintf(cmd.OutOrStdout(), "  Total: %.3f\n", explain.Total)
				for _, item := range explain.Items {
					fmt.Fprintf(cmd.OutOrStdout(), "  %s\tcoef=%.1f\tcontrib=%.3f\t%s\n", item.Name, item.Coefficient, item.Contribution, item.Reason)
				}
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()

			explain, err := svc.ExplainUrgency(args[0])
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), explain)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Urgency for %s\n", explain.UUID)
			fmt.Fprintf(cmd.OutOrStdout(), "  Total: %.3f\n", explain.Total)
			for _, item := range explain.Items {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s\tcoef=%.1f\tcontrib=%.3f\t%s\n", item.Name, item.Coefficient, item.Contribution, item.Reason)
			}
			return nil
		},
	}
}

func newUrgencyHelperCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "_urgency <target>",
		Short: "输出匹配任务的 urgency 值",
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
				explain, err := client.ExplainUrgency(context.Background(), currentOpts.Workspace, target)
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%.3f\n", explain.Total)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()

			explain, err := svc.ExplainUrgency(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%.3f\n", explain.Total)
			return nil
		},
	}
}
