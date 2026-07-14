package cli

import (
	"fmt"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/query"
	"github.com/spf13/cobra"
)

type taskViewFlags struct {
	DueAfter       string
	DueBefore      string
	OccurrenceMode string
	Sort           string
	Limit          int
	Offset         int
}

func (flags *taskViewFlags) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&flags.DueAfter, "due-after", "", "截止起始日期（含，YYYY-MM-DD）")
	cmd.Flags().StringVar(&flags.DueBefore, "due-before", "", "截止结束日期（含，YYYY-MM-DD）")
	cmd.Flags().StringVar(&flags.OccurrenceMode, "occurrence-mode", "auto", "循环实例：auto|materialized|expand")
	cmd.Flags().StringVar(&flags.Sort, "sort", "", "排序：entry|due|modified|urgency")
	cmd.Flags().IntVar(&flags.Limit, "limit", 0, "分页上限")
	cmd.Flags().IntVar(&flags.Offset, "offset", 0, "分页偏移")
}

func (flags taskViewFlags) localQuery(base query.Expr, loc *time.Location) (query.Expr, *app.TaskViewRange, error) {
	if loc == nil {
		loc = time.Local
	}
	mode := app.OccurrenceMode(flags.OccurrenceMode)
	switch mode {
	case "", app.OccurrenceModeAuto, app.OccurrenceModeMaterialized, app.OccurrenceModeExpand:
	default:
		return nil, nil, fmt.Errorf("--occurrence-mode must be auto|materialized|expand")
	}
	var afterDate, beforeDate *time.Time
	if flags.DueAfter != "" {
		parsed, err := time.ParseInLocation("2006-01-02", flags.DueAfter, loc)
		if err != nil {
			return nil, nil, fmt.Errorf("--due-after: expected YYYY-MM-DD: %w", err)
		}
		afterDate = &parsed
		value := query.DateValue(flags.DueAfter)
		base = query.And(base, query.Or(
			query.Predicate{Attribute: query.AttrDue, Operator: query.OpEqual, Value: value},
			query.Predicate{Attribute: query.AttrDue, Operator: query.OpAfter, Value: value},
		))
	}
	if flags.DueBefore != "" {
		parsed, err := time.ParseInLocation("2006-01-02", flags.DueBefore, loc)
		if err != nil {
			return nil, nil, fmt.Errorf("--due-before: expected YYYY-MM-DD: %w", err)
		}
		beforeDate = &parsed
		nextDay := parsed.AddDate(0, 0, 1).Format("2006-01-02")
		base = query.And(base, query.Predicate{
			Attribute: query.AttrDue, Operator: query.OpBefore, Value: query.DateValue(nextDay),
		})
	}
	if afterDate == nil || beforeDate == nil {
		return base, nil, nil
	}
	return base, &app.TaskViewRange{
		Start: afterDate.Unix(),
		End:   beforeDate.AddDate(0, 0, 1).Unix(),
	}, nil
}
