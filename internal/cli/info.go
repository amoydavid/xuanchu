package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

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
	// 窄字段构造（列表场景）。
	id := dto.ID
	if dto.UUID != nil {
		id = *dto.UUID
	}
	tsk := task.Task{
		UUID:     id,
		Title:    dto.Title,
		Status:   dto.Status,
		Tags:     dto.Tags,
		Priority: dto.Priority,
		Project:  dto.Project,
	}
	if dto.TaskSlug != nil {
		project, seq, err := parseRemoteTaskSlug(*dto.TaskSlug)
		if err == nil {
			tsk.Project = &project
			tsk.ProjectSeq = &seq
		}
	}
	if dto.Due != nil {
		d := *dto.Due
		tsk.Due = &d
	}
	if dto.Entry != nil {
		tsk.Entry = *dto.Entry
	}
	if dto.Modified != nil {
		tsk.Modified = *dto.Modified
	}
	if len(dto.UDAs) > 0 {
		tsk.UDAs = make(map[string]task.UDAValue, len(dto.UDAs))
		for name, raw := range dto.UDAs {
			tsk.UDAs[name] = task.UDAValue{Name: name, Raw: raw}
		}
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

func parseRemoteTaskSlug(slug string) (string, int64, error) {
	dash := strings.LastIndex(slug, "-")
	if dash <= 0 {
		return "", 0, fmt.Errorf("invalid task_slug %q", slug)
	}
	seq, err := strconv.ParseInt(slug[dash+1:], 10, 64)
	if err != nil || seq < 1 {
		return "", 0, fmt.Errorf("invalid task_slug %q", slug)
	}
	return slug[:dash], seq, nil
}
