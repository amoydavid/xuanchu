package cli

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/remote"
	"git.dajee.net/dajee/xuanchu/internal/task"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func newInfoCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "info <target>",
		Short: "显示任务详情",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				target, err := resolveRemoteTaskTarget(context.Background(), client, currentOpts, args[0])
				if err != nil {
					return err
				}
				tsk, err := client.GetTaskView(context.Background(), currentOpts.Workspace, target)
				if err != nil {
					return err
				}
				renderTaskOccurrenceInfo(cmd.OutOrStdout(), currentOpts.JSON, remoteOccurrenceDTOToView(tsk))
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()

			view, err := svc.GetTaskView(args[0])
			if err != nil {
				return err
			}
			renderTaskOccurrenceInfo(cmd.OutOrStdout(), currentOpts.JSON, view)
			return nil
		},
	}
}

func renderTaskOccurrenceInfo(w io.Writer, asJSON bool, view app.TaskOccurrenceView) {
	if asJSON {
		renderOccurrenceViewJSON(w, view)
		return
	}
	value := func(v *string) string {
		if v == nil {
			return "-"
		}
		return *v
	}
	timestamp := func(v *int64) string {
		if v == nil {
			return "-"
		}
		return time.Unix(*v, 0).UTC().Format(time.RFC3339)
	}
	fmt.Fprintf(w, "Reference: %s\n", occurrenceHumanRef(view))
	fmt.Fprintf(w, "URL: %s\n", view.URL)
	fmt.Fprintf(w, "UUID: %s\n", value(view.UUID))
	fmt.Fprintf(w, "Task slug: %s\n", value(view.TaskSlug))
	fmt.Fprintf(w, "Status: %s\n", view.Status)
	fmt.Fprintf(w, "Title: %s\n", view.Title)
	fmt.Fprintf(w, "Description: %s\n", value(view.Description))
	fmt.Fprintf(w, "Entry: %s\n", timestamp(view.Entry))
	fmt.Fprintf(w, "Modified: %s\n", timestamp(view.Modified))
	fmt.Fprintf(w, "End: %s\n", timestamp(view.End))
	fmt.Fprintf(w, "Due: %s\n", timestamp(view.Due))
	fmt.Fprintf(w, "Project: %s\n", value(view.Project))
	fmt.Fprintf(w, "Priority: %s\n", value(view.Priority))
	fmt.Fprintf(w, "Assignees: %s\n", formatOccurrenceAssignees(view.Assignees))
	fmt.Fprintf(w, "Tags: %s\n", strings.Join(view.Tags, ","))
	if view.RecurrenceInfo != nil {
		label := "循环实例"
		if view.RecurrenceInfo.Materialization == "projected" {
			label = "计划实例"
		}
		fmt.Fprintf(w, "Type: %s\n", label)
		fmt.Fprintf(w, "Occurrence ID: %s\n", view.ID)
		fmt.Fprintf(w, "Series: %s (%s)\n", view.RecurrenceInfo.SeriesTitle, view.RecurrenceInfo.SeriesID)
		fmt.Fprintf(w, "Recurrence: %s\n", view.RecurrenceInfo.Rule)
		fmt.Fprintf(w, "Occurrence at: %s\n", time.Unix(view.RecurrenceInfo.RecurrenceAt, 0).UTC().Format(time.RFC3339))
		fmt.Fprintf(w, "Materialization: %s\n", view.RecurrenceInfo.Materialization)
		fmt.Fprintf(w, "Overrides: %s\n", strings.Join(view.RecurrenceInfo.Overrides, ","))
	}
	if len(view.Links) > 0 {
		fmt.Fprintln(w, "Links:")
		for _, link := range view.Links {
			fmt.Fprintf(w, "  [%s] %s %s\n", link.Type, link.Title, link.URL)
		}
	}
}

func formatOccurrenceAssignees(assignees []task.UserInfo) string {
	labels := make([]string, 0, len(assignees))
	for _, assignee := range assignees {
		label := assignee.Name
		if label == "" {
			label = assignee.DisplayName
		}
		if label == "" && assignee.Email != nil {
			label = *assignee.Email
		}
		if label == "" {
			label = assignee.ID
		}
		if label != "" {
			labels = append(labels, "@"+label)
		}
	}
	return strings.Join(labels, ", ")
}

func resolveRemoteTaskTarget(ctx context.Context, client *remote.Client, opts Options, target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", fmt.Errorf("task target is required")
	}
	if _, err := uuid.Parse(target); err == nil {
		if opts.Project != "" || opts.ProjectID != "" {
			page, err := client.QueryTasks(ctx, remote.TaskQueryInput{
				Workspace: opts.Workspace,
				Project:   opts.Project,
				ProjectID: opts.ProjectID,
				NoContext: true,
			})
			if err != nil {
				return "", err
			}
			for _, tsk := range remotePageToTasks(page) {
				if tsk.UUID == target {
					return target, nil
				}
			}
			return "", fmt.Errorf("task %q not found", target)
		}
		return target, nil
	}
	if app.IsOccurrenceRef(target) {
		return target, nil
	}
	if isTaskSlugRef(target) {
		return target, nil
	}
	tasks, err := remoteDefaultWorkingSet(ctx, client, opts)
	if err != nil {
		return "", err
	}
	if n, err := strconv.Atoi(target); err == nil {
		if n < 1 || n > len(tasks) {
			return "", fmt.Errorf("task %q not found", target)
		}
		return tasks[n-1].UUID, nil
	}
	var matched string
	for _, tsk := range tasks {
		if strings.HasPrefix(tsk.UUID, target) {
			if matched != "" {
				return "", fmt.Errorf("task target %q is ambiguous", target)
			}
			matched = tsk.UUID
		}
	}
	if matched == "" {
		return "", fmt.Errorf("task %q not found", target)
	}
	return matched, nil
}

func isTaskSlugRef(target string) bool {
	dash := strings.LastIndex(target, "-")
	if dash <= 0 || dash == len(target)-1 {
		return false
	}
	if seq, err := strconv.ParseInt(target[dash+1:], 10, 64); err != nil || seq < 1 {
		return false
	}
	project := target[:dash]
	if len(project) < 3 || len(project) > 10 {
		return false
	}
	first := project[0]
	if !((first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z')) {
		return false
	}
	for i := 0; i < len(project); i++ {
		ch := project[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
			continue
		}
		return false
	}
	return true
}

func remoteDefaultWorkingSet(ctx context.Context, client *remote.Client, opts Options) ([]task.Task, error) {
	page, err := client.QueryTasks(ctx, remote.TaskQueryInput{
		Workspace: opts.Workspace,
		Project:   opts.Project,
		ProjectID: opts.ProjectID,
		Filters:   []string{"(status:pending or status:waiting)"},
		NoContext: true,
	})
	if err != nil {
		return nil, err
	}
	return remotePageToTasks(page), nil
}

// remotePageToTasks 把 TaskViewPageDTO 的 items 转为 []task.Task（spec §13.5）。
// 列表端点返回的窄 DTO 只有部分字段；这里构造最小 task.Task 供渲染层使用。
func remotePageToTasks(page remote.TaskViewPageDTO) []task.Task {
	out := make([]task.Task, 0, len(page.Items))
	for _, dto := range page.Items {
		out = append(out, remoteDTOToTask(dto))
	}
	return out
}

func remotePageToTaskViewPage(page remote.TaskViewPageDTO) app.TaskViewPage {
	view := app.TaskViewPage{
		Items: remoteOccurrenceDTOsToViews(page.Items), Total: page.Total,
		Limit: page.Limit, Offset: page.Offset,
		OccurrenceMode: app.OccurrenceMode(page.OccurrenceMode),
	}
	if page.Range != nil {
		view.Range = &app.TaskViewRange{Start: page.Range.Start, End: page.Range.End}
	}
	return view
}

// remoteDTOToTask 把统一 TaskOccurrenceDTO 转为 task.Task，供既有 human renderer 使用。
// projected occurrence 无 UUID 时用 DTO.ID（occurrence_ref）作为 UUID 占位。
func remoteDTOToTask(dto remote.TaskOccurrenceDTO) task.Task {
	return enrichTaskFromDTO(taskFromOccurrenceView(remoteOccurrenceDTOToView(dto)), dto)
}

func taskFromOccurrenceView(view app.TaskOccurrenceView) task.Task {
	tsk := task.Task{
		WorkspaceID: view.WorkspaceID, Title: view.Title, Description: view.Description,
		Status: view.Status, End: view.End, Due: view.Due, Project: view.Project,
		ProjectID: view.ProjectID, ProjectSeq: view.ProjectSeq, Priority: view.Priority,
		Tags: view.Tags, Start: view.Start, Wait: view.Wait, Scheduled: view.Scheduled,
		Until: view.Until, Annotations: view.Annotations, Depends: view.Depends,
		Parent: view.Parent, Links: view.Links, UDAs: view.UDAs,
	}
	if view.UUID != nil {
		tsk.UUID = *view.UUID
	}
	if view.Entry != nil {
		tsk.Entry = *view.Entry
	}
	if view.Modified != nil {
		tsk.Modified = *view.Modified
	}
	for _, assignee := range view.Assignees {
		tsk.Assignees = append(tsk.Assignees, task.AssigneeInfo{
			UserID: assignee.ID, Name: assignee.Name, DisplayName: assignee.DisplayName,
			Email: assignee.Email, ExternalIDs: assignee.ExternalIDs,
		})
	}
	if view.RecurrenceInfo != nil {
		tsk.SeriesID = &view.RecurrenceInfo.SeriesID
		tsk.RecurrenceAt = &view.RecurrenceInfo.RecurrenceAt
		rule := view.RecurrenceInfo.Rule
		tsk.RecurrenceRuleSnapshot = &rule
		tsk.RecurrenceOverrides = append([]string(nil), view.RecurrenceInfo.Overrides...)
	}
	return tsk
}

// enrichTaskFromDTO 用 DTO 的 occurrence 字段补充 task.Task。
func enrichTaskFromDTO(tsk task.Task, dto remote.TaskOccurrenceDTO) task.Task {
	if dto.RecurrenceInfo != nil {
		ri := dto.RecurrenceInfo
		tsk.SeriesID = &ri.SeriesID
		tsk.RecurrenceAt = &ri.RecurrenceAt
		if ri.Rule != "" {
			snap := ri.Rule
			tsk.RecurrenceRuleSnapshot = &snap
		}
	}
	return tsk
}
