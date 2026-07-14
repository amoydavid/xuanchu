package cli

import (
	"context"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/query"
	"git.dajee.net/dajee/xuanchu/internal/remote"
	"github.com/spf13/cobra"
)

var reportShorts = map[string]string{
	"all":       "列出所有任务（含已完成和已删除）",
	"completed": "列出已完成的任务",
	"deleted":   "列出已删除的任务",
	"overdue":   "列出已过期的任务",
	"active":    "列出已开始的任务",
	"waiting":   "列出等待中的任务",
	"ready":     "列出就绪待办任务",
	"blocked":   "列出被阻塞的任务",
	"blocking":  "列出阻塞其他任务的任务",
}

func newReportCommand(opts Options, name string) *cobra.Command {
	var viewFlags taskViewFlags
	cmd := &cobra.Command{
		Use:   name + " [filters...]",
		Short: reportShorts[name],
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
				page, err := client.QueryTasks(context.Background(), remote.TaskQueryInput{
					Workspace: currentOpts.Workspace,
					Project:   currentOpts.Project,
					ProjectID: currentOpts.ProjectID,
					Report:    name,
					Filters:   append([]string(nil), args...),
					NoContext: currentOpts.NoContext,
					DueAfter:  viewFlags.DueAfter, DueBefore: viewFlags.DueBefore,
					OccurrenceMode: viewFlags.OccurrenceMode, Sort: viewFlags.Sort,
					Limit: viewFlags.Limit, Offset: viewFlags.Offset,
				})
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
	viewFlags.bind(cmd)
	return cmd
}

func newAllCommand(opts Options) *cobra.Command       { return newReportCommand(opts, "all") }
func newCompletedCommand(opts Options) *cobra.Command { return newReportCommand(opts, "completed") }
func newDeletedCommand(opts Options) *cobra.Command   { return newReportCommand(opts, "deleted") }
func newOverdueCommand(opts Options) *cobra.Command   { return newReportCommand(opts, "overdue") }
func newActiveCommand(opts Options) *cobra.Command    { return newReportCommand(opts, "active") }
func newWaitingCommand(opts Options) *cobra.Command   { return newReportCommand(opts, "waiting") }
func newReadyCommand(opts Options) *cobra.Command     { return newReportCommand(opts, "ready") }
func newBlockedCommand(opts Options) *cobra.Command   { return newReportCommand(opts, "blocked") }
func newBlockingCommand(opts Options) *cobra.Command  { return newReportCommand(opts, "blocking") }
