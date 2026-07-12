package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/remote"
)

// parseLocalDateEndOfDay 把 YYYY-MM-DD 解析为本地时区当天 23:59:59。
func parseLocalDateEndOfDay(raw string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02 15:04:05", raw+" 23:59:59", time.Local)
}

// newSeriesCommand 构造 `xuanchu series` 命令树（spec §13.6）。
func newSeriesCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "series",
		Short: "管理循环任务系列",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newSeriesAddCommand(opts))
	cmd.AddCommand(newSeriesListCommand(opts))
	cmd.AddCommand(newSeriesInfoCommand(opts))
	cmd.AddCommand(newSeriesModifyCommand(opts))
	cmd.AddCommand(newSeriesOccurrencesCommand(opts))
	cmd.AddCommand(newSeriesStopCommand(opts))
	cmd.AddCommand(newSeriesSkipCommand(opts))
	return cmd
}

// --- series add ---

func newSeriesAddCommand(opts Options) *cobra.Command {
	var recur, firstDue, until, projectRef, priority string
	var assignees, tags []string
	cmd := &cobra.Command{
		Use:   "add <title>",
		Short: "创建循环任务系列",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			title := strings.Join(args, " ")
			// 解析 project：--project flag 优先，否则用 currentOpts.Project。
			projRef := projectRef
			if projRef == "" {
				projRef = currentOpts.Project
			}
			// 解析 first_due。
			firstDueTs, err := parseSeriesDateFlag(firstDue)
			if err != nil {
				return fmt.Errorf("--first-due: %w", err)
			}
			input := app.AddTaskSeriesInput{
				Title: title, RecurrenceRule: recur, FirstDue: firstDueTs,
				Priority: stringPtrOrNil(priority), Assignees: assignees, Tags: tags,
			}
			if projRef != "" {
				// 用 project_id 还是 slug：若像 UUID 则用 ProjectID，否则用 Project。
				if isLikelyUUID(projRef) {
					input.ProjectID = projRef
				} else {
					p := projRef
					input.Project = &p
				}
			}
			if until != "" {
				untilTs, err := parseSeriesDateFlag(until)
				if err != nil {
					return fmt.Errorf("--until: %w", err)
				}
				input.Until = &untilTs
			}
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				return runSeriesAddRemote(cmd, currentOpts, input)
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			result, err := svc.AddTaskSeries(input)
			if err != nil {
				return err
			}
			renderSeriesCreateResult(cmd.OutOrStdout(), currentOpts.JSON, result)
			return nil
		},
	}
	cmd.Flags().StringVar(&recur, "recur", "", "循环规则（daily|weekly|monthly|<N>days|<N>weeks|<N>months）")
	cmd.Flags().StringVar(&firstDue, "first-due", "", "首次截止日期（YYYY-MM-DD）")
	cmd.Flags().StringVar(&until, "until", "", "循环结束日期（YYYY-MM-DD）")
	cmd.Flags().StringVar(&projectRef, "project", "", "项目 slug 或 ID")
	cmd.Flags().StringVar(&priority, "priority", "", "优先级（H|M|L）")
	cmd.Flags().StringSliceVar(&assignees, "assignee", nil, "负责人（可重复）")
	cmd.Flags().StringSliceVar(&tags, "tag", nil, "标签（可重复）")
	_ = cmd.MarkFlagRequired("recur")
	_ = cmd.MarkFlagRequired("first-due")
	return cmd
}

// --- series list ---

func newSeriesListCommand(opts Options) *cobra.Command {
	var statusFilter, query, assignee, sortBy string
	var limit, offset int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "列出循环任务系列",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if statusFilter == "" {
				statusFilter = "active"
			}
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				return runSeriesListRemote(cmd, currentOpts, statusFilter, query, assignee, sortBy, limit, offset)
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			input := app.TaskSeriesListInput{
				Project: currentOpts.Project, ProjectID: currentOpts.ProjectID,
				Status: statusFilter, Q: query, Assignee: assignee, Sort: sortBy,
				Limit: limit, Offset: offset,
			}
			page, err := svc.ListTaskSeries(input)
			if err != nil {
				return err
			}
			renderSeriesList(cmd.OutOrStdout(), currentOpts.JSON, page)
			return nil
		},
	}
	cmd.Flags().StringVar(&statusFilter, "status", "active", "状态过滤：active|ended|stopped|all")
	cmd.Flags().StringVar(&query, "query", "", "标题/描述搜索")
	cmd.Flags().StringVar(&assignee, "assignee", "", "负责人过滤")
	cmd.Flags().StringVar(&sortBy, "sort", "next", "排序：next|title|modified")
	cmd.Flags().IntVar(&limit, "limit", 0, "分页上限")
	cmd.Flags().IntVar(&offset, "offset", 0, "分页偏移")
	return cmd
}

// --- series info ---

func newSeriesInfoCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "info <series-ref>",
		Short: "查看循环任务系列详情",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			seriesRef := args[0]
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				return runSeriesInfoRemote(cmd, currentOpts, seriesRef)
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			detail, err := svc.GetTaskSeries(seriesRef)
			if err != nil {
				return err
			}
			renderSeriesDetail(cmd.OutOrStdout(), currentOpts.JSON, detail)
			return nil
		},
	}
}

// --- series modify ---

func newSeriesModifyCommand(opts Options) *cobra.Command {
	var recur, until, effectiveFrom, title, priority string
	var assignees, tags []string
	cmd := &cobra.Command{
		Use:   "modify <series-ref>",
		Short: "修改循环任务系列",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			seriesRef := args[0]
			// 构造 ModifyTaskSeriesInput。
			input := app.ModifyTaskSeriesInput{}
			if title != "" {
				t := title
				input.Title = &t
			}
			if priority != "" {
				input.Priority = &priority
			}
			if len(assignees) > 0 {
				input.Assignees = assignees
			}
			if len(tags) > 0 {
				input.Tags = tags
			}
			if until != "" {
				ts, err := parseSeriesDateFlag(until)
				if err != nil {
					return fmt.Errorf("--until: %w", err)
				}
				input.Until = &ts
			}
			if recur != "" {
				input.RecurrenceRule = &recur
				if effectiveFrom == "" {
					return fmt.Errorf("修改 --recur 必须同时提供 --effective-from")
				}
				ts, err := parseSeriesDateFlag(effectiveFrom)
				if err != nil {
					return fmt.Errorf("--effective-from: %w", err)
				}
				input.EffectiveFrom = &ts
			}
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				return runSeriesModifyRemote(cmd, currentOpts, seriesRef, input)
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			view, err := svc.ModifyTaskSeries(seriesRef, input)
			if err != nil {
				return err
			}
			renderSeriesDetail(cmd.OutOrStdout(), currentOpts.JSON, app.TaskSeriesDetailView{Series: view})
			return nil
		},
	}
	cmd.Flags().StringVar(&recur, "recur", "", "新循环规则")
	cmd.Flags().StringVar(&effectiveFrom, "effective-from", "", "新规则生效日期（YYYY-MM-DD）")
	cmd.Flags().StringVar(&until, "until", "", "循环结束日期")
	cmd.Flags().StringVar(&title, "title", "", "标题")
	cmd.Flags().StringVar(&priority, "priority", "", "优先级")
	cmd.Flags().StringSliceVar(&assignees, "assignee", nil, "负责人")
	cmd.Flags().StringSliceVar(&tags, "tag", nil, "标签")
	return cmd
}

// --- series occurrences ---

func newSeriesOccurrencesCommand(opts Options) *cobra.Command {
	var statusFilter string
	var dueAfter, dueBefore string
	var limit, offset int
	cmd := &cobra.Command{
		Use:   "occurrences <series-ref>",
		Short: "列出循环任务系列的实例",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			seriesRef := args[0]
			if statusFilter == "" {
				statusFilter = "all"
			}
			input := app.TaskSeriesOccurrenceListInput{
				Status: statusFilter, Limit: limit, Offset: offset,
			}
			if dueAfter != "" {
				ts, err := parseSeriesDateFlag(dueAfter)
				if err != nil {
					return fmt.Errorf("--due-after: %w", err)
				}
				input.DueAfter = &ts
			}
			if dueBefore != "" {
				ts, err := parseSeriesDateFlag(dueBefore)
				if err != nil {
					return fmt.Errorf("--due-before: %w", err)
				}
				input.DueBefore = &ts
			}
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				return runSeriesOccurrencesRemote(cmd, currentOpts, seriesRef, statusFilter, limit, offset)
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			page, err := svc.ListTaskSeriesOccurrences(seriesRef, input)
			if err != nil {
				return err
			}
			renderOccurrencePage(cmd.OutOrStdout(), currentOpts.JSON, page)
			return nil
		},
	}
	cmd.Flags().StringVar(&statusFilter, "status", "all", "状态：pending|waiting|completed|deleted|all")
	cmd.Flags().StringVar(&dueAfter, "due-after", "", "截止起始日期")
	cmd.Flags().StringVar(&dueBefore, "due-before", "", "截止结束日期")
	cmd.Flags().IntVar(&limit, "limit", 0, "分页上限")
	cmd.Flags().IntVar(&offset, "offset", 0, "分页偏移")
	return cmd
}

// --- series stop ---

func newSeriesStopCommand(opts Options) *cobra.Command {
	var deleteOpen bool
	cmd := &cobra.Command{
		Use:   "stop <series-ref>",
		Short: "停止循环任务系列",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			seriesRef := args[0]
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				view, err := client.StopTaskSeries(context.Background(), currentOpts.Workspace, seriesRef, deleteOpen)
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					_ = renderJSON(cmd.OutOrStdout(), view)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "已停止循环任务 %s\n", view.Title)
				}
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			view, err := svc.StopTaskSeries(seriesRef, app.StopTaskSeriesInput{DeleteOpenOccurrences: &deleteOpen})
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				renderSeriesViewJSON(cmd.OutOrStdout(), view)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "已停止循环任务 %s\n", view.Title)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&deleteOpen, "delete-open", false, "同时跳过当前未完成实例")
	return cmd
}

// --- series skip ---

func newSeriesSkipCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "skip <series-ref> <occurrence-ref>",
		Short: "跳过循环任务的一次实例",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			seriesRef := args[0]
			occurrenceRef := args[1]
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				view, err := client.SkipTaskSeriesOccurrence(context.Background(), currentOpts.Workspace, seriesRef, occurrenceRef)
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					_ = renderJSON(cmd.OutOrStdout(), view)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "已跳过实例 %s\n", view.ID)
				}
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			view, err := svc.SkipTaskSeriesOccurrence(seriesRef, occurrenceRef)
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				renderOccurrenceViewJSON(cmd.OutOrStdout(), view)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "已跳过实例 %s\n", view.ID)
			}
			return nil
		},
	}
}

// --- 辅助 ---

func parseSeriesDateFlag(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("日期不能为空")
	}
	// 尝试 unix 时间戳。
	if ts, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return ts, nil
	}
	// 尝试 YYYY-MM-DD（按本地 23:59:59）。
	t, err := parseLocalDateEndOfDay(raw)
	if err != nil {
		return 0, err
	}
	return t.Unix(), nil
}

func isLikelyUUID(s string) bool {
	// 简单判断：36 字符且含 4 个连字符。
	return len(s) == 36 && strings.Count(s, "-") == 4
}

func stringPtrOrNil(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

// --- remote 路径 ---

func runSeriesAddRemote(cmd *cobra.Command, currentOpts Options, input app.AddTaskSeriesInput) error {
	client, err := buildRemoteClient(currentOpts)
	if err != nil {
		return err
	}
	remoteInput := remote.AddTaskSeriesInput{
		Title: input.Title, RecurrenceRule: input.RecurrenceRule,
		Priority: input.Priority, Assignees: input.Assignees, Tags: input.Tags,
	}
	if input.Project != nil {
		remoteInput.Project = input.Project
	}
	if input.ProjectID != "" {
		remoteInput.ProjectID = input.ProjectID
	}
	if input.FirstDue > 0 {
		fd := input.FirstDue
		remoteInput.FirstDue = &fd
	}
	if input.Until != nil {
		remoteInput.Until = input.Until
	}
	result, err := client.AddTaskSeries(context.Background(), currentOpts.Workspace, remoteInput)
	if err != nil {
		return err
	}
	if currentOpts.JSON {
		_ = renderJSON(cmd.OutOrStdout(), result)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "已创建循环任务 %s（%s）\n", result.Series.Title, result.Series.RecurrenceRule)
		if result.FirstOccurrence != nil {
			mat := "计划实例"
			if result.FirstOccurrence.RecurrenceInfo != nil {
				mat = result.FirstOccurrence.RecurrenceInfo.Materialization
			}
			fmt.Fprintf(cmd.OutOrStdout(), "首次实例：%s（%s）\n", result.FirstOccurrence.ID, mat)
		}
	}
	return nil
}

func runSeriesListRemote(cmd *cobra.Command, currentOpts Options, status, q, assignee, sort string, limit, offset int) error {
	client, err := buildRemoteClient(currentOpts)
	if err != nil {
		return err
	}
	page, err := client.ListTaskSeries(context.Background(), remote.TaskSeriesListInput{
		Workspace: currentOpts.Workspace, Project: currentOpts.Project, ProjectID: currentOpts.ProjectID,
		Status: status, Q: q, Assignee: assignee, Sort: sort, Limit: limit, Offset: offset,
	})
	if err != nil {
		return err
	}
	// 转换 DTO 到本地 view 以复用渲染。
	views := make([]app.TaskSeriesView, 0, len(page.Items))
	for _, it := range page.Items {
		views = append(views, remoteSeriesDTOToView(it))
	}
	renderSeriesList(cmd.OutOrStdout(), currentOpts.JSON, app.TaskSeriesPage{Items: views, Total: page.Total, Limit: page.Limit, Offset: page.Offset})
	return nil
}

func runSeriesModifyRemote(cmd *cobra.Command, currentOpts Options, seriesRef string, input app.ModifyTaskSeriesInput) error {
	client, err := buildRemoteClient(currentOpts)
	if err != nil {
		return err
	}
	dto, err := client.ModifyTaskSeries(context.Background(), currentOpts.Workspace, seriesRef, input)
	if err != nil {
		return err
	}
	if currentOpts.JSON {
		_ = renderJSON(cmd.OutOrStdout(), dto)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "已修改循环任务 %s\n", dto.Title)
	}
	return nil
}

func runSeriesInfoRemote(cmd *cobra.Command, currentOpts Options, seriesRef string) error {
	client, err := buildRemoteClient(currentOpts)
	if err != nil {
		return err
	}
	dto, err := client.GetTaskSeries(context.Background(), currentOpts.Workspace, seriesRef)
	if err != nil {
		return err
	}
	renderSeriesDetail(cmd.OutOrStdout(), currentOpts.JSON, app.TaskSeriesDetailView{Series: remoteSeriesDTOToView(dto)})
	return nil
}

func runSeriesOccurrencesRemote(cmd *cobra.Command, currentOpts Options, seriesRef, status string, limit, offset int) error {
	client, err := buildRemoteClient(currentOpts)
	if err != nil {
		return err
	}
	page, err := client.ListTaskSeriesOccurrences(context.Background(), currentOpts.Workspace, seriesRef, status, limit, offset)
	if err != nil {
		return err
	}
	items := make([]app.TaskOccurrenceView, 0, len(page.Items))
	for _, it := range page.Items {
		items = append(items, remoteOccurrenceDTOToView(it))
	}
	renderOccurrencePage(cmd.OutOrStdout(), currentOpts.JSON, app.TaskViewPage{
		Items: items, Total: page.Total, Limit: page.Limit, Offset: page.Offset,
		OccurrenceMode: app.OccurrenceMode(page.OccurrenceMode),
	})
	return nil
}

// --- 渲染 ---

func renderSeriesCreateResult(w io.Writer, asJSON bool, result app.TaskSeriesCreateResult) {
	if asJSON {
		_ = renderJSON(w, result)
		return
	}
	fmt.Fprintf(w, "已创建循环任务 %s（%s）\n", result.Series.Title, result.Series.RecurrenceRule)
	if result.FirstOccurrence != nil {
		mat := "计划实例"
		if result.FirstOccurrence.RecurrenceInfo != nil {
			mat = result.FirstOccurrence.RecurrenceInfo.Materialization
		}
		fmt.Fprintf(w, "首次实例：%s（%s）\n", result.FirstOccurrence.ID, mat)
	}
}

func renderSeriesList(w io.Writer, asJSON bool, page app.TaskSeriesPage) {
	if asJSON {
		_ = renderJSON(w, page)
		return
	}
	if len(page.Items) == 0 {
		fmt.Fprintln(w, "没有循环任务")
		return
	}
	for _, s := range page.Items {
		next := "—"
		if s.NextRecurrenceAt != nil {
			next = formatUnixShort(*s.NextRecurrenceAt)
		}
		fmt.Fprintf(w, "%s  %s  %s  未完成 %d  下次 %s\n", s.ID[:8], s.Title, s.Status, s.OpenOccurrenceCount, next)
	}
	if page.Total > len(page.Items) {
		fmt.Fprintf(w, "... 共 %d 条\n", page.Total)
	}
}

func renderSeriesDetail(w io.Writer, asJSON bool, detail app.TaskSeriesDetailView) {
	if asJSON {
		_ = renderJSON(w, detail)
		return
	}
	s := detail.Series
	fmt.Fprintf(w, "循环任务：%s\n", s.Title)
	fmt.Fprintf(w, "ID：%s\n", s.ID)
	fmt.Fprintf(w, "状态：%s\n", s.Status)
	fmt.Fprintf(w, "规则：%s\n", s.RecurrenceRule)
	fmt.Fprintf(w, "首次截止：%s\n", formatUnixShort(s.FirstDue))
	if s.Until != nil {
		fmt.Fprintf(w, "有效至：%s\n", formatUnixShort(*s.Until))
	}
	fmt.Fprintf(w, "未完成：%d  已完成：%d  已跳过：%d  逾期：%d\n", s.OpenOccurrenceCount, s.CompletedCount, s.SkippedCount, s.OverdueCount)
	if s.NextRecurrenceAt != nil {
		fmt.Fprintf(w, "下一槽位：%s\n", formatUnixShort(*s.NextRecurrenceAt))
	}
}

func renderOccurrencePage(w io.Writer, asJSON bool, page app.TaskViewPage) {
	if asJSON {
		_ = renderJSON(w, page)
		return
	}
	if len(page.Items) == 0 {
		fmt.Fprintln(w, "没有实例")
		return
	}
	for _, it := range page.Items {
		id := it.ID
		if it.UUID != nil {
			id = *it.UUID
		}
		due := "—"
		if it.Due != nil {
			due = formatUnixShort(*it.Due)
		}
		mat := ""
		if it.RecurrenceInfo != nil {
			mat = "（" + it.RecurrenceInfo.Materialization + "）"
		}
		fmt.Fprintf(w, "%s  %s  %s  %s%s\n", id, it.Status, due, it.Title, mat)
	}
	if page.Total > len(page.Items) {
		fmt.Fprintf(w, "... 共 %d 条\n", page.Total)
	}
}

func renderSeriesViewJSON(w io.Writer, v app.TaskSeriesView) {
	_ = renderJSON(w, v)
}

func renderOccurrenceViewJSON(w io.Writer, v app.TaskOccurrenceView) {
	_ = renderJSON(w, v)
}

func renderJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func formatUnixShort(ts int64) string {
	// 简短日期渲染（本地时区）。
	return strconv.FormatInt(ts, 10)
}

// remote DTO → app view（用于复用本地渲染）。

func remoteSeriesDTOToView(dto remote.TaskSeriesDTO) app.TaskSeriesView {
	return app.TaskSeriesView{
		OpenOccurrenceCount: dto.OpenOccurrenceCount,
		CompletedCount:      dto.CompletedCount,
		SkippedCount:        dto.SkippedCount,
		OverdueCount:        dto.OverdueCount,
		NextRecurrenceAt:    dto.NextRecurrenceAt,
	}
}

func remoteOccurrenceDTOToView(dto remote.TaskOccurrenceDTO) app.TaskOccurrenceView {
	v := app.TaskOccurrenceView{
		ID: dto.ID, UUID: dto.UUID, TaskSlug: dto.TaskSlug, ProjectSeq: dto.ProjectSeq,
		Title: dto.Title, Status: dto.Status, Entry: dto.Entry, Modified: dto.Modified,
		Due: dto.Due, Priority: dto.Priority, Tags: dto.Tags,
	}
	if dto.RecurrenceInfo != nil {
		v.RecurrenceInfo = &app.RecurrenceInfo{
			Role: dto.RecurrenceInfo.Role, SeriesID: dto.RecurrenceInfo.SeriesID,
			SeriesStatus: dto.RecurrenceInfo.SeriesStatus, Rule: dto.RecurrenceInfo.Rule,
			RecurrenceAt: dto.RecurrenceInfo.RecurrenceAt,
			Materialization: dto.RecurrenceInfo.Materialization,
			Overrides: dto.RecurrenceInfo.Overrides, Until: dto.RecurrenceInfo.Until,
		}
	}
	return v
}
