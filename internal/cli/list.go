package cli

import (
	"context"
	"strconv"
	"strings"

	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/query"
	"github.com/dajee/taskg/internal/remote"
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
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				input := remote.ListTasksInput{Workspace: currentOpts.Workspace}
				if sort != "" {
					input.Report = name
				}
				if len(args) > 0 {
					if isPlainTargetArg(args) {
						input.Target = args[0]
					} else {
						input.Filters = append([]string(nil), args...)
					}
				}
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				tasks, err := client.ListTasks(context.Background(), input)
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
				ids := make([]int, len(tasks))
				for i := range tasks {
					ids[i] = i + 1
				}
				render.TaskListWithIDs(cmd.OutOrStdout(), tasks, ids)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()

			input := app.ListInput{}
			if len(args) > 0 {
				if isPlainTargetArg(args) {
					v := args[0]
					input.Target = &v
				} else {
					expr, err := query.ParseFilterExpr(args)
					if err != nil {
						return err
					}
					input.Query = expr
				}
			}
			input.Sort = sort

			tasks, err := svc.ListReport(name, input)
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
			ids, err := svc.WorkingSetIDs(tasks)
			if err != nil {
				return err
			}
			render.TaskListWithIDs(cmd.OutOrStdout(), tasks, ids)
			return nil
		},
	}
}

func isPlainTargetArg(args []string) bool {
	if len(args) != 1 {
		return false
	}
	s := args[0]
	if _, err := strconv.Atoi(s); err == nil {
		return true
	}
	// Full 36-char UUID with hyphens
	if len(s) == 36 && strings.Count(s, "-") == 4 {
		return true
	}
	// Full 32-char UUID without hyphens
	if len(s) == 32 {
		hex := true
		for _, c := range s {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				hex = false
				break
			}
		}
		if hex {
			return true
		}
	}
	return false
}
