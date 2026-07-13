package cli

import (
	"context"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/query"
	"git.dajee.net/dajee/xuanchu/internal/remote"
	"git.dajee.net/dajee/xuanchu/internal/render"
	"git.dajee.net/dajee/xuanchu/internal/task"
	"github.com/spf13/cobra"
)

var listCommandShorts = map[string]string{
	"list": "列出待办任务",
	"next": "列出按 urgency 排序的待办任务",
}

func newListCommand(opts Options) *cobra.Command {
	return newTaskListCommand(opts, "list", "")
}

func newNextCommand(opts Options) *cobra.Command {
	return newTaskListCommand(opts, "next", "next")
}

func newTaskListCommand(opts Options, name, sort string) *cobra.Command {
	return &cobra.Command{
		Use:   name + " [filters...]",
		Short: listCommandShorts[name],
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				input := remote.TaskQueryInput{
					Workspace: currentOpts.Workspace,
					Project:   currentOpts.Project,
					ProjectID: currentOpts.ProjectID,
					NoContext: currentOpts.NoContext,
				}
				if sort != "" {
					input.Sort = sort
				}
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				if len(args) > 0 {
					if isPlainTargetArg(args) {
						target, err := resolveRemoteTaskTarget(context.Background(), client, currentOpts, args[0])
						if err != nil {
							return err
						}
						input.Target = target
					} else {
						input.Filters = append([]string(nil), args...)
					}
				}
				page, err := client.QueryTasks(context.Background(), input)
				if err != nil {
					return err
				}
				tasks := remotePageToTasks(page)
				if currentOpts.JSON {
					dtos := make([]task.JSONTask, len(tasks))
					for i, tsk := range tasks {
						dtos[i] = task.ToJSON(tsk)
					}
					return render.JSON(cmd.OutOrStdout(), dtos)
				}
				ids, err := remoteWorkingSetIDs(context.Background(), client, currentOpts, tasks)
				if err != nil {
					return err
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

func remoteWorkingSetIDs(ctx context.Context, client *remote.Client, opts Options, tasks []task.Task) ([]int, error) {
	if len(tasks) == 0 {
		return nil, nil
	}
	workingSet, err := remoteDefaultWorkingSet(ctx, client, opts)
	if err != nil {
		return nil, err
	}
	index := make(map[string]int, len(workingSet))
	for i, tsk := range workingSet {
		index[tsk.UUID] = i + 1
	}
	ids := make([]int, len(tasks))
	for i, tsk := range tasks {
		ids[i] = index[tsk.UUID]
	}
	return ids, nil
}

func isPlainTargetArg(args []string) bool {
	if len(args) != 1 {
		return false
	}
	s := args[0]
	if isDecimalDigitsArg(s) {
		return true
	}
	if isTaskSlugRef(s) {
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

func isDecimalDigitsArg(value string) bool {
	if value == "" {
		return false
	}
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}
