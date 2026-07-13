package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/remote"
	"git.dajee.net/dajee/xuanchu/internal/render"
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
				taskObj := remoteDTOToTask(tsk)
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), task.ToJSON(taskObj))
				}
				render.TaskInfo(cmd.OutOrStdout(), taskObj)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()

			tsk, err := svc.ResolveTarget(args[0])
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), task.ToJSON(tsk))
			}
			render.TaskInfo(cmd.OutOrStdout(), tsk)
			return nil
		},
	}
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

// remoteDTOToTask 把单个 DTO 转为 task.Task。
// 优先使用 RawJSON（单任务端点的完整 JSONTask）；否则从窄字段构造。
// projected occurrence 无 UUID 时用 DTO.ID（occurrence_ref）作为 UUID 占位，
// 供渲染层显示（spec §13.5：不降级 occurrence，CLI 用稳定公开 id）。
func remoteDTOToTask(dto remote.TaskOccurrenceDTO) task.Task {
	if dto.JSONTask != nil {
		if tsk, err := task.FromJSONStrict(*dto.JSONTask); err == nil {
			return enrichTaskFromDTO(tsk, dto)
		}
	}
	// 统一 task view 使用 Unix 时间和 recurrence_info；先复用完整 DTO→View
	// 映射，再转为 CLI 既有的 domain Task 渲染输入，避免遗漏 wait 等字段。
	view := remoteOccurrenceDTOToView(dto)
	id := view.ID
	if view.UUID != nil {
		id = *view.UUID
	}
	tsk := task.Task{
		UUID: id, WorkspaceID: view.WorkspaceID, Title: view.Title, Description: view.Description,
		Status: view.Status, End: view.End, Due: view.Due, Project: view.Project,
		ProjectID: view.ProjectID, ProjectSeq: view.ProjectSeq, Priority: view.Priority,
		Tags: view.Tags, Start: view.Start, Wait: view.Wait, Scheduled: view.Scheduled,
		Until: view.Until, Annotations: view.Annotations, Depends: view.Depends,
		Parent: view.Parent, Links: view.Links, UDAs: view.UDAs,
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
	return enrichTaskFromDTO(tsk, dto)
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
