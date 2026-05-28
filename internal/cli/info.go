package cli

import (
	"fmt"
	"strconv"

	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/render"
	"github.com/dajee/taskg/internal/task"
	"github.com/spf13/cobra"
)

func newInfoCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "info <target>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()

			tsk, err := resolveTarget(svc, args[0])
			if err != nil {
				return err
			}
			if opts.JSON {
				return render.JSON(cmd.OutOrStdout(), tsk)
			}
			render.TaskInfo(cmd.OutOrStdout(), tsk)
			return nil
		},
	}
}

func resolveTarget(svc *app.Service, target string) (task.Task, error) {
	if n, err := strconv.Atoi(target); err == nil && n >= 1 {
		tasks, err := svc.List(app.ListInput{})
		if err != nil {
			return task.Task{}, err
		}
		if n > len(tasks) {
			return task.Task{}, fmt.Errorf("task %d not found", n)
		}
		return tasks[n-1], nil
	}
	return svc.Info(target)
}
