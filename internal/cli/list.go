package cli

import (
	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/query"
	"github.com/dajee/taskg/internal/render"
	"github.com/dajee/taskg/internal/task"
	"github.com/spf13/cobra"
)

func newListCommand(opts Options) *cobra.Command {
	return newTaskListCommand(opts, "list", "")
}

func newNextCommand(opts Options) *cobra.Command {
	return newTaskListCommand(opts, "next", "next")
}

func newTaskListCommand(opts Options, name, sort string) *cobra.Command {
	return &cobra.Command{
		Use:  name + " [filters...]",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()

			input := app.ListInput{}
			if len(args) > 0 {
				filter, err := query.ParseFilters(args)
				if err != nil {
					return err
				}
				input.Target = filter.Target
				input.Status = derefStr(filter.Status)
				input.Project = filter.Project
				input.Priority = filter.Priority
				input.Tags = filter.Tags
				input.Text = filter.Text
			}
			input.Sort = sort

			tasks, err := svc.List(input)
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				dtos := make([]task.JSONTask, len(tasks))
				for i, tsk := range tasks {
					dtos[i] = task.ToJSON(tsk)
				}
				return render.JSON(cmd.OutOrStdout(), dtos)
			}
			render.TaskList(cmd.OutOrStdout(), tasks)
			return nil
		},
	}
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
