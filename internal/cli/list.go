package cli

import (
	"context"
	"strings"
	"time"

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
	var viewFlags taskViewFlags
	cmd := &cobra.Command{
		Use:   name + " [filters...]",
		Short: listCommandShorts[name],
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				if len(args) > 0 && isPlainTargetArg(args) {
					target, err := resolveRemoteTaskTarget(context.Background(), client, currentOpts, args[0])
					if err != nil {
						return err
					}
					view, err := client.GetTaskView(context.Background(), currentOpts.Workspace, target)
					if err != nil {
						return err
					}
					page := app.TaskViewPage{Items: []app.TaskOccurrenceView{remoteOccurrenceDTOToView(view)}, Total: 1, Limit: 1}
					ids, err := remoteWorkingSetIDsForViews(context.Background(), client, currentOpts, page.Items)
					if err != nil {
						return err
					}
					renderTaskViewPage(cmd, currentOpts.JSON, page, ids)
					return nil
				}
				input := remote.TaskQueryInput{
					Workspace:      currentOpts.Workspace,
					Project:        currentOpts.Project,
					ProjectID:      currentOpts.ProjectID,
					NoContext:      currentOpts.NoContext,
					Report:         name,
					DueAfter:       viewFlags.DueAfter,
					DueBefore:      viewFlags.DueBefore,
					OccurrenceMode: viewFlags.OccurrenceMode,
					Sort:           viewFlags.Sort, Limit: viewFlags.Limit, Offset: viewFlags.Offset,
					Filters: append([]string(nil), args...),
				}
				page, err := client.QueryTasks(context.Background(), input)
				if err != nil {
					return err
				}
				viewPage := remotePageToTaskViewPage(page)
				ids, err := remoteWorkingSetIDsForViews(context.Background(), client, currentOpts, viewPage.Items)
				if err != nil {
					return err
				}
				renderTaskViewPage(cmd, currentOpts.JSON, viewPage, ids)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()

			if len(args) > 0 && isPlainTargetArg(args) {
				view, err := svc.GetTaskView(args[0])
				if err != nil {
					return err
				}
				page := app.TaskViewPage{Items: []app.TaskOccurrenceView{view}, Total: 1, Limit: 1}
				ids, err := svc.WorkingSetIDsForViews(page.Items)
				if err != nil {
					return err
				}
				renderTaskViewPage(cmd, currentOpts.JSON, page, ids)
				return nil
			}
			var expr query.Expr
			if len(args) > 0 {
				expr, err = query.ParseFilterExpr(args)
				if err != nil {
					return err
				}
			}
			expr, dateRange, err := viewFlags.localQuery(expr, time.Local)
			if err != nil {
				return err
			}
			page, err := svc.RunTaskViewReport(app.ReportViewInput{
				Name: name, Query: expr, Range: dateRange,
				OccurrenceMode: app.OccurrenceMode(viewFlags.OccurrenceMode),
				NoContext:      currentOpts.NoContext, Sort: viewFlags.Sort,
				Limit: viewFlags.Limit, Offset: viewFlags.Offset,
			})
			if err != nil {
				return err
			}
			ids, err := svc.WorkingSetIDsForViews(page.Items)
			if err != nil {
				return err
			}
			renderTaskViewPage(cmd, currentOpts.JSON, page, ids)
			return nil
		},
	}
	_ = sort
	viewFlags.bind(cmd)
	return cmd
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

func remoteWorkingSetIDsForViews(ctx context.Context, client *remote.Client, opts Options, views []app.TaskOccurrenceView) ([]int, error) {
	if len(views) == 0 {
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
	ids := make([]int, len(views))
	for i, view := range views {
		if view.UUID != nil {
			ids[i] = index[*view.UUID]
		}
	}
	return ids, nil
}

func renderTaskViewPage(cmd *cobra.Command, asJSON bool, page app.TaskViewPage, ids []int) {
	if asJSON {
		renderOccurrencePage(cmd.OutOrStdout(), true, page)
		return
	}
	tasks := make([]task.Task, 0, len(page.Items))
	for _, view := range page.Items {
		tsk := taskFromOccurrenceView(view)
		if view.RecurrenceInfo != nil {
			if view.RecurrenceInfo.Materialization == "projected" {
				tsk.Title += " [计划实例 · " + view.ID + "]"
			} else {
				tsk.Title += " [循环实例 · " + view.RecurrenceInfo.Rule + "]"
			}
		}
		tasks = append(tasks, tsk)
	}
	render.TaskListWithIDs(cmd.OutOrStdout(), tasks, ids)
}

func isPlainTargetArg(args []string) bool {
	if len(args) != 1 {
		return false
	}
	s := args[0]
	if app.IsOccurrenceRef(s) {
		return true
	}
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
