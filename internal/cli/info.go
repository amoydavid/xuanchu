package cli

import (
	"github.com/dajee/taskg/internal/render"
	"github.com/dajee/taskg/internal/task"
	"github.com/spf13/cobra"
)

func newInfoCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "info <target>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()

			tsk, err := svc.ResolveTarget(args[0])
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), task.ToJSON(tsk))
			}
			render.TaskInfo(cmd.OutOrStdout(), tsk)
			return nil
		},
	}
}
