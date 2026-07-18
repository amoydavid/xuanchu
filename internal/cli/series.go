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
	"git.dajee.net/dajee/xuanchu/internal/task"
	"git.dajee.net/dajee/xuanchu/internal/taskseries"
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
	var recur, firstDue, until, projectRef, description, priority string
	var assignees, tags []string
	var udas map[string]string
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
				Description: stringPtrOrNil(description), Priority: stringPtrOrNil(priority),
				Assignees: assignees, Tags: tags, UDAs: udas,
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
	cmd.Flags().StringVar(&description, "description", "", "描述")
	cmd.Flags().StringVar(&priority, "priority", "", "优先级（H|M|L）")
	cmd.Flags().StringSliceVar(&assignees, "assignee", nil, "负责人（可重复）")
	cmd.Flags().StringSliceVar(&tags, "tag", nil, "标签（可重复）")
	cmd.Flags().StringToStringVar(&udas, "uda", nil, "自定义字段（name=value）")
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
			if err := validateSeriesPaginationFlags(cmd, limit, offset); err != nil {
				return err
			}
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
	var recur, until, effectiveFrom, title, description, priority string
	var assignees, tags, clear []string
	var udas map[string]string
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
			if description != "" {
				input.Description = &description
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
			if len(udas) > 0 {
				input.UDAs = udas
			}
			if err := app.ApplyTaskSeriesClearFields(&input, clear); err != nil {
				return err
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
	cmd.Flags().StringVar(&description, "description", "", "描述")
	cmd.Flags().StringVar(&priority, "priority", "", "优先级")
	cmd.Flags().StringSliceVar(&assignees, "assignee", nil, "负责人")
	cmd.Flags().StringSliceVar(&tags, "tag", nil, "标签")
	cmd.Flags().StringToStringVar(&udas, "uda", nil, "自定义字段（name=value）")
	cmd.Flags().StringSliceVar(&clear, "clear", nil, "清空字段（description|priority|assignees|tags|until|uda.<name>）")
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
			if err := validateSeriesPaginationFlags(cmd, limit, offset); err != nil {
				return err
			}
			currentOpts := optionsFromCmd(cmd, opts)
			seriesRef := args[0]
			if statusFilter == "" {
				statusFilter = "all"
			}
			input := app.TaskSeriesOccurrenceListInput{
				Status: statusFilter, Limit: limit, Offset: offset,
			}
			if dueAfter != "" {
				ts, err := parseSeriesOccurrenceBoundary(dueAfter, false)
				if err != nil {
					return fmt.Errorf("--due-after: %w", err)
				}
				input.DueAfter = &ts
			}
			if dueBefore != "" {
				ts, err := parseSeriesOccurrenceBoundary(dueBefore, true)
				if err != nil {
					return fmt.Errorf("--due-before: %w", err)
				}
				input.DueBefore = &ts
			}
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				return runSeriesOccurrencesRemote(cmd, currentOpts, seriesRef, input, dueAfter, dueBefore)
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

func validateSeriesPaginationFlags(cmd *cobra.Command, limit, offset int) error {
	if cmd.Flags().Changed("limit") && (limit < 1 || limit > 1000) {
		return fmt.Errorf("--limit 必须在 1 到 1000 之间")
	}
	if offset < 0 {
		return fmt.Errorf("--offset 不能为负数")
	}
	return nil
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
					fmt.Fprintf(cmd.OutOrStdout(), "已停止循环任务 %s\nURL: %s\n", view.Title, view.URL)
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
				fmt.Fprintf(cmd.OutOrStdout(), "已停止循环任务 %s\nURL: %s\n", view.Title, view.URL)
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
					fmt.Fprintf(cmd.OutOrStdout(), "已跳过实例 %s\nURL: %s\n", view.ID, view.URL)
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
				fmt.Fprintf(cmd.OutOrStdout(), "已跳过实例 %s\nURL: %s\n", view.ID, view.URL)
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

func parseSeriesOccurrenceBoundary(raw string, endExclusive bool) (int64, error) {
	value, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(raw), time.Local)
	if err != nil {
		return 0, err
	}
	if endExclusive {
		value = value.AddDate(0, 0, 1)
	}
	return value.Unix(), nil
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
		Description: input.Description, Priority: input.Priority,
		Assignees: input.Assignees, Tags: input.Tags, UDAs: input.UDAs,
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
		fmt.Fprintf(cmd.OutOrStdout(), "URL: %s\n", result.Series.URL)
		if result.FirstOccurrence != nil {
			mat := "计划实例"
			if result.FirstOccurrence.RecurrenceInfo != nil {
				mat = result.FirstOccurrence.RecurrenceInfo.Materialization
			}
			fmt.Fprintf(cmd.OutOrStdout(), "首次实例：%s（%s） %s\n", result.FirstOccurrence.ID, mat, result.FirstOccurrence.URL)
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
	clear := make([]string, 0, 5+len(input.ClearUDAs))
	if input.ClearDescription {
		clear = append(clear, "description")
	}
	if input.ClearPriority {
		clear = append(clear, "priority")
	}
	if input.ClearAssignees {
		clear = append(clear, "assignees")
	}
	if input.ClearTags {
		clear = append(clear, "tags")
	}
	if input.ClearUntil {
		clear = append(clear, "until")
	}
	for _, name := range input.ClearUDAs {
		clear = append(clear, "uda."+name)
	}
	dto, err := client.ModifyTaskSeries(context.Background(), currentOpts.Workspace, seriesRef, remote.ModifyTaskSeriesInput{
		Title: input.Title, Description: input.Description, Priority: input.Priority,
		Assignees: input.Assignees, Tags: input.Tags, UDAs: input.UDAs,
		RecurrenceRule: input.RecurrenceRule, EffectiveFrom: input.EffectiveFrom,
		Until: input.Until, Clear: clear,
	})
	if err != nil {
		return err
	}
	if currentOpts.JSON {
		_ = renderJSON(cmd.OutOrStdout(), dto)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "已修改循环任务 %s\nURL: %s\n", dto.Title, dto.URL)
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
	renderSeriesDetail(cmd.OutOrStdout(), currentOpts.JSON, remoteSeriesDetailDTOToView(dto))
	return nil
}

func runSeriesOccurrencesRemote(cmd *cobra.Command, currentOpts Options, seriesRef string, input app.TaskSeriesOccurrenceListInput, dueAfter, dueBefore string) error {
	client, err := buildRemoteClient(currentOpts)
	if err != nil {
		return err
	}
	page, err := client.ListTaskSeriesOccurrences(context.Background(), remote.TaskSeriesOccurrenceListInput{
		Workspace: currentOpts.Workspace, SeriesRef: seriesRef, Status: input.Status,
		DueAfter: dueAfter, DueBefore: dueBefore, Limit: input.Limit, Offset: input.Offset,
	})
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
		payload := map[string]any{"series": seriesViewJSON(result.Series)}
		if result.FirstOccurrence != nil {
			payload["first_occurrence"] = occurrenceViewJSON(*result.FirstOccurrence)
		}
		_ = renderJSON(w, payload)
		return
	}
	fmt.Fprintf(w, "已创建循环任务 %s（%s）\n", result.Series.Title, result.Series.RecurrenceRule)
	fmt.Fprintf(w, "URL: %s\n", result.Series.URL)
	if result.FirstOccurrence != nil {
		mat := "计划实例"
		if result.FirstOccurrence.RecurrenceInfo != nil {
			mat = result.FirstOccurrence.RecurrenceInfo.Materialization
		}
		fmt.Fprintf(w, "首次实例：%s（%s） %s\n", result.FirstOccurrence.ID, mat, result.FirstOccurrence.URL)
	}
}

func renderSeriesList(w io.Writer, asJSON bool, page app.TaskSeriesPage) {
	if asJSON {
		items := make([]map[string]any, 0, len(page.Items))
		for _, item := range page.Items {
			items = append(items, seriesViewJSON(item))
		}
		_ = renderJSON(w, map[string]any{"items": items, "total": page.Total, "limit": page.Limit, "offset": page.Offset})
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
		fmt.Fprintf(w, "%s  %s  %s  未完成 %d  下次 %s  %s\n", s.ID[:8], s.Title, s.Status, s.OpenOccurrenceCount, next, s.URL)
	}
	if page.Total > len(page.Items) {
		fmt.Fprintf(w, "... 共 %d 条\n", page.Total)
	}
}

func renderSeriesDetail(w io.Writer, asJSON bool, detail app.TaskSeriesDetailView) {
	if asJSON {
		payload := seriesViewJSON(detail.Series)
		payload["open_occurrences"] = occurrenceViewsJSON(detail.OpenOccurrences)
		payload["recent_completed"] = occurrenceViewsJSON(detail.RecentCompleted)
		payload["recent_skipped"] = occurrenceViewsJSON(detail.RecentSkipped)
		_ = renderJSON(w, payload)
		return
	}
	s := detail.Series
	fmt.Fprintf(w, "循环任务：%s\n", s.Title)
	fmt.Fprintf(w, "ID：%s\n", s.ID)
	fmt.Fprintf(w, "URL: %s\n", s.URL)
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
	renderSeriesOccurrenceGroup(w, "未完成实例", detail.OpenOccurrences)
	renderSeriesOccurrenceGroup(w, "最近完成", detail.RecentCompleted)
	renderSeriesOccurrenceGroup(w, "最近跳过", detail.RecentSkipped)
}

func renderSeriesOccurrenceGroup(w io.Writer, title string, rows []app.TaskOccurrenceView) {
	if len(rows) == 0 {
		return
	}
	fmt.Fprintf(w, "%s：\n", title)
	for _, row := range rows {
		due := "—"
		if row.Due != nil {
			due = formatUnixShort(*row.Due)
		}
		fmt.Fprintf(w, "  %s  %s  %s  %s  %s\n", occurrenceHumanRef(row), row.Status, due, row.Title, row.URL)
	}
}

func renderOccurrencePage(w io.Writer, asJSON bool, page app.TaskViewPage) {
	if asJSON {
		payload := map[string]any{
			"items": occurrenceViewsJSON(page.Items), "total": page.Total,
			"limit": page.Limit, "offset": page.Offset, "occurrence_mode": string(page.OccurrenceMode),
		}
		if page.Range != nil {
			payload["range"] = map[string]int64{"start": page.Range.Start, "end": page.Range.End}
		}
		_ = renderJSON(w, payload)
		return
	}
	if len(page.Items) == 0 {
		fmt.Fprintln(w, "没有实例")
		return
	}
	for _, it := range page.Items {
		due := "—"
		if it.Due != nil {
			due = formatUnixShort(*it.Due)
		}
		mat := ""
		if it.RecurrenceInfo != nil {
			mat = "（" + it.RecurrenceInfo.Materialization + "）"
		}
		fmt.Fprintf(w, "%s  %s  %s  %s%s  %s\n", occurrenceHumanRef(it), it.Status, due, it.Title, mat, it.URL)
	}
	if page.Total > len(page.Items) {
		fmt.Fprintf(w, "... 共 %d 条\n", page.Total)
	}
}

func occurrenceHumanRef(v app.TaskOccurrenceView) string {
	if v.TaskSlug != nil && strings.TrimSpace(*v.TaskSlug) != "" {
		return *v.TaskSlug
	}
	if v.RecurrenceInfo != nil {
		date := time.Unix(v.RecurrenceInfo.RecurrenceAt, 0).In(time.Local).Format("01-02")
		return "↻" + date + " (" + v.ID + ")"
	}
	if v.UUID != nil {
		return *v.UUID
	}
	return v.ID
}

func renderSeriesViewJSON(w io.Writer, v app.TaskSeriesView) {
	_ = renderJSON(w, seriesViewJSON(v))
}

func renderOccurrenceViewJSON(w io.Writer, v app.TaskOccurrenceView) {
	_ = renderJSON(w, occurrenceViewJSON(v))
}

func seriesViewJSON(v app.TaskSeriesView) map[string]any {
	out := map[string]any{
		"url": v.URL,
		"id":  v.ID, "workspace_id": v.WorkspaceID, "project_id": v.ProjectID,
		"title": v.Title, "status": v.Status, "recurrence_rule": v.RecurrenceRule,
		"first_due": v.FirstDue, "open_occurrence_count": v.OpenOccurrenceCount,
		"completed_count": v.CompletedCount, "skipped_count": v.SkippedCount,
		"overdue_count": v.OverdueCount, "created_at": v.CreatedAt, "modified_at": v.ModifiedAt,
		"created_by": task.UserInfoToJSON(v.CreatedBy),
	}
	if v.ProjectSlug != "" {
		out["project_slug"] = v.ProjectSlug
	}
	if slug := app.SeriesSlugOf(v.Series); slug != "" {
		out["series_slug"] = slug
	}
	if v.Description != nil {
		out["description"] = *v.Description
	}
	if v.Until != nil {
		out["until"] = *v.Until
	}
	if v.Priority != nil {
		out["priority"] = *v.Priority
	}
	if len(v.Tags) > 0 {
		out["tags"] = v.Tags
	}
	if len(v.UDAs) > 0 {
		out["udas"] = v.UDAs
	}
	if len(v.Assignees) > 0 {
		rows := make([]task.JSONUserInfo, 0, len(v.Assignees))
		for _, user := range v.Assignees {
			rows = append(rows, task.UserInfoToJSON(user))
		}
		out["assignees"] = rows
	}
	if v.NextRecurrenceAt != nil {
		out["next_recurrence_at"] = *v.NextRecurrenceAt
	}
	if v.SuggestedRuleEffectiveFrom != nil {
		out["suggested_rule_effective_from"] = *v.SuggestedRuleEffectiveFrom
	}
	return out
}

func occurrenceViewsJSON(rows []app.TaskOccurrenceView) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, occurrenceViewJSON(row))
	}
	return out
}

func occurrenceViewJSON(v app.TaskOccurrenceView) map[string]any {
	udas := make(map[string]string, len(v.UDAs))
	for name, value := range v.UDAs {
		udas[name] = value.Raw
	}
	assignees := make([]task.JSONUserInfo, 0, len(v.Assignees))
	for _, user := range v.Assignees {
		assignees = append(assignees, task.UserInfoToJSON(user))
	}
	links := make([]task.JSONTaskLink, 0, len(v.Links))
	for _, link := range v.Links {
		links = append(links, task.JSONTaskLink{
			ID: link.ID, Type: link.Type, URL: link.URL, Title: link.Title,
			CreatedAt: time.Unix(link.CreatedAt, 0).Format(time.RFC3339), CreatedBy: task.ActorInfoToJSON(link.CreatedBy),
		})
	}
	out := map[string]any{
		"url": v.URL,
		"id":  v.ID, "workspace_id": v.WorkspaceID, "title": v.Title, "status": v.Status,
		"tags": v.Tags, "assignees": assignees, "depends": v.Depends,
		"annotations": task.AnnotationsToJSON(v.Annotations), "links": links, "udas": udas,
		"uuid": nil, "task_slug": nil, "project_seq": nil,
		"entry": nil, "modified": nil, "start": nil, "end": nil,
	}
	if v.UUID != nil {
		out["uuid"] = *v.UUID
	}
	if v.TaskSlug != nil {
		out["task_slug"] = *v.TaskSlug
	}
	if v.ProjectSeq != nil {
		out["project_seq"] = *v.ProjectSeq
	}
	if v.ProjectID != nil {
		out["project_id"] = *v.ProjectID
	}
	if v.Project != nil {
		out["project"] = *v.Project
	}
	if v.Description != nil {
		out["description"] = *v.Description
	}
	if v.Priority != nil {
		out["priority"] = *v.Priority
	}
	if v.Parent != nil {
		out["parent"] = *v.Parent
	}
	for name, value := range map[string]*int64{
		"entry": v.Entry, "modified": v.Modified, "due": v.Due, "start": v.Start,
		"end": v.End, "wait": v.Wait, "scheduled": v.Scheduled, "until": v.Until,
	} {
		if value != nil {
			out[name] = *value
		}
	}
	if v.RecurrenceInfo != nil {
		recurrence := map[string]any{
			"role": v.RecurrenceInfo.Role, "series_id": v.RecurrenceInfo.SeriesID,
			"series_title":  v.RecurrenceInfo.SeriesTitle,
			"series_status": v.RecurrenceInfo.SeriesStatus, "rule": v.RecurrenceInfo.Rule,
			"recurrence_at":   v.RecurrenceInfo.RecurrenceAt,
			"materialization": v.RecurrenceInfo.Materialization, "overrides": v.RecurrenceInfo.Overrides,
		}
		if v.RecurrenceInfo.Until != nil {
			recurrence["until"] = *v.RecurrenceInfo.Until
		}
		out["recurrence_info"] = recurrence
	}
	return out
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
	assigneeIDs := make([]string, 0, len(dto.Assignees))
	assignees := make([]task.UserInfo, 0, len(dto.Assignees))
	for _, assignee := range dto.Assignees {
		assigneeIDs = append(assigneeIDs, assignee.ID)
		assignees = append(assignees, task.UserInfoFromJSON(assignee))
	}
	return app.TaskSeriesView{
		Series: taskseries.Series{
			ID: dto.ID, WorkspaceID: dto.WorkspaceID, ProjectID: dto.ProjectID,
			Title: dto.Title, Description: dto.Description, Status: dto.Status,
			RecurrenceRule: dto.RecurrenceRule, FirstDue: dto.FirstDue, Until: dto.Until,
			Priority: dto.Priority, AssigneeIDs: assigneeIDs, Tags: dto.Tags, UDAs: dto.UDAs,
			CreatedBy: dto.CreatedBy.ID, CreatedAt: dto.CreatedAt, ModifiedAt: dto.ModifiedAt,
		},
		URL:                        dto.URL,
		OpenOccurrenceCount:        dto.OpenOccurrenceCount,
		CompletedCount:             dto.CompletedCount,
		SkippedCount:               dto.SkippedCount,
		OverdueCount:               dto.OverdueCount,
		NextRecurrenceAt:           dto.NextRecurrenceAt,
		SuggestedRuleEffectiveFrom: dto.SuggestedRuleEffectiveFrom,
		CreatedBy:                  task.UserInfoFromJSON(dto.CreatedBy),
		Assignees:                  assignees,
	}
}

func remoteSeriesDetailDTOToView(dto remote.TaskSeriesDTO) app.TaskSeriesDetailView {
	return app.TaskSeriesDetailView{
		Series:          remoteSeriesDTOToView(dto),
		OpenOccurrences: remoteOccurrenceDTOsToViews(dto.OpenOccurrences),
		RecentCompleted: remoteOccurrenceDTOsToViews(dto.RecentCompleted),
		RecentSkipped:   remoteOccurrenceDTOsToViews(dto.RecentSkipped),
	}
}

func remoteOccurrenceDTOsToViews(rows []remote.TaskOccurrenceDTO) []app.TaskOccurrenceView {
	out := make([]app.TaskOccurrenceView, 0, len(rows))
	for _, row := range rows {
		out = append(out, remoteOccurrenceDTOToView(row))
	}
	return out
}

func remoteOccurrenceDTOToView(dto remote.TaskOccurrenceDTO) app.TaskOccurrenceView {
	v := app.TaskOccurrenceView{
		URL: dto.URL, ID: dto.ID, UUID: dto.UUID, TaskSlug: dto.TaskSlug, ProjectSeq: dto.ProjectSeq,
		WorkspaceID: dto.WorkspaceID, ProjectID: dto.ProjectID, Project: dto.Project,
		Title: dto.Title, Description: dto.Description, Status: dto.Status,
		Entry: dto.Entry, Modified: dto.Modified, Start: dto.Start, End: dto.End,
		Due: dto.Due, Wait: dto.Wait, Scheduled: dto.Scheduled, Until: dto.Until,
		Parent: dto.Parent, Priority: dto.Priority, Tags: dto.Tags, Depends: dto.Depends,
	}
	for _, assignee := range dto.Assignees {
		v.Assignees = append(v.Assignees, task.UserInfoFromJSON(assignee))
	}
	for name, raw := range dto.UDAs {
		if v.UDAs == nil {
			v.UDAs = map[string]task.UDAValue{}
		}
		v.UDAs[name] = task.UDAValue{Raw: raw}
	}
	for _, annotation := range dto.Annotations {
		entry, _ := time.Parse(time.RFC3339, annotation.Entry)
		v.Annotations = append(v.Annotations, task.Annotation{
			ID: annotation.ID, Entry: entry.Unix(), Description: annotation.Description,
		})
	}
	for _, link := range dto.Links {
		createdAt, _ := time.Parse(time.RFC3339, link.CreatedAt)
		v.Links = append(v.Links, task.TaskLinkInfo{
			ID: link.ID, Type: link.Type, URL: link.URL, Title: link.Title,
			CreatedAt: createdAt.Unix(), CreatedBy: task.ActorInfoFromJSON(link.CreatedBy),
		})
	}
	if dto.RecurrenceInfo != nil {
		v.RecurrenceInfo = &app.RecurrenceInfo{
			Role: dto.RecurrenceInfo.Role, SeriesID: dto.RecurrenceInfo.SeriesID,
			SeriesTitle:  dto.RecurrenceInfo.SeriesTitle,
			SeriesStatus: dto.RecurrenceInfo.SeriesStatus, Rule: dto.RecurrenceInfo.Rule,
			RecurrenceAt:    dto.RecurrenceInfo.RecurrenceAt,
			Materialization: dto.RecurrenceInfo.Materialization,
			Overrides:       dto.RecurrenceInfo.Overrides, Until: dto.RecurrenceInfo.Until,
		}
	}
	return v
}
