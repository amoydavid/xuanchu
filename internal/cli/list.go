package cli

import (
	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/render"
	"github.com/spf13/cobra"
)

func newListCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "list",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			tasks, err := svc.List(app.ListInput{})
			if err != nil {
				return err
			}
			if opts.JSON {
				return render.JSON(cmd.OutOrStdout(), tasks)
			}
			render.TaskList(cmd.OutOrStdout(), tasks)
			return nil
		},
	}
}
