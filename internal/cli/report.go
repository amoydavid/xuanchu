package cli

import (
	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/query"
	"github.com/dajee/taskg/internal/render"
	"github.com/dajee/taskg/internal/task"
	"github.com/spf13/cobra"
)

func newReportCommand(opts Options, name string) *cobra.Command {
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

			var expr query.Expr
			if len(args) > 0 {
				expr, err = query.ParseFilterExpr(args)
				if err != nil {
					return err
				}
			}

			result, err := svc.RunReport(app.ReportInput{Name: name, Query: expr})
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				dtos := make([]task.JSONTask, len(result.Tasks))
				for i, tsk := range result.Tasks {
					dtos[i] = task.ToJSON(tsk)
				}
				return render.JSON(cmd.OutOrStdout(), dtos)
			}
			render.TaskList(cmd.OutOrStdout(), result.Tasks)
			return nil
		},
	}
}

func newAllCommand(opts Options) *cobra.Command      { return newReportCommand(opts, "all") }
func newCompletedCommand(opts Options) *cobra.Command { return newReportCommand(opts, "completed") }
func newDeletedCommand(opts Options) *cobra.Command   { return newReportCommand(opts, "deleted") }
func newOverdueCommand(opts Options) *cobra.Command   { return newReportCommand(opts, "overdue") }
func newActiveCommand(opts Options) *cobra.Command    { return newReportCommand(opts, "active") }
func newWaitingCommand(opts Options) *cobra.Command   { return newReportCommand(opts, "waiting") }
func newReadyCommand(opts Options) *cobra.Command     { return newReportCommand(opts, "ready") }
func newBlockedCommand(opts Options) *cobra.Command   { return newReportCommand(opts, "blocked") }
func newBlockingCommand(opts Options) *cobra.Command  { return newReportCommand(opts, "blocking") }
