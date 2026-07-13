package app

import (
	"errors"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/storage"
	domain "git.dajee.net/dajee/xuanchu/internal/task"
)

// TaskResourceKind 描述 task reference 最终解析到的领域资源类型。
// 协议层应依赖此结果，而不是根据输入字符串形式推断普通任务或循环实例。
type TaskResourceKind string

const (
	TaskResourceNormal     TaskResourceKind = "normal"
	TaskResourceOccurrence TaskResourceKind = "occurrence"
)

// TaskRefResolution 是 UUID、task_slug、occurrence_ref 的统一解析结果。
// StableID 对普通任务为 UUID，对循环实例始终为 occurrence_ref。
type TaskRefResolution struct {
	Kind            TaskResourceKind
	StableID        string
	UUID            *string
	TaskSlug        *string
	OccurrenceRef   *string
	Materialization string
	Task            *domain.Task
	View            TaskOccurrenceView
}

// ResolveTaskReferenceForRead 解析任意可读 task reference，且不会物化 projected occurrence。
func (s *Service) ResolveTaskReferenceForRead(ref string) (TaskRefResolution, error) {
	if err := s.Require(PermissionTaskRead); err != nil {
		return TaskRefResolution{}, err
	}
	ref = strings.TrimSpace(ref)
	if IsOccurrenceRef(ref) {
		view, err := s.getOccurrenceView(ref)
		if err != nil {
			return TaskRefResolution{}, err
		}
		resolution := taskRefResolutionFromView(view, nil)
		if view.UUID == nil {
			return resolution, nil
		}
		tsk, err := s.repo.GetByUUID(s.workspaceID, *view.UUID)
		if err != nil {
			return TaskRefResolution{}, err
		}
		if err := s.ensureReadableTaskScope(tsk); err != nil {
			return TaskRefResolution{}, RuntimeError{Code: "task_occurrence_not_found", Message: "occurrence not found"}
		}
		resolution.Task = &tsk
		return resolution, nil
	}

	tsk, err := s.resolveTargetForRead(ref)
	if err != nil {
		return TaskRefResolution{}, err
	}
	return s.taskRefResolutionFromTask(tsk)
}

// ResolveExistingTaskReference 解析已经存在的 task row。
// projected occurrence 不会在此处物化；需要“写前物化”的动作必须使用 WithTaskForWrite。
// write 仅控制权限与 project scope 错误语义。
func (s *Service) ResolveExistingTaskReference(ref string, write bool) (TaskRefResolution, error) {
	if write {
		if err := s.Require(PermissionTaskWrite); err != nil {
			return TaskRefResolution{}, err
		}
	} else if err := s.Require(PermissionTaskRead); err != nil {
		return TaskRefResolution{}, err
	}
	ref = strings.TrimSpace(ref)
	var (
		tsk domain.Task
		err error
	)
	if IsOccurrenceRef(ref) {
		seriesID, slot, parseErr := ParseOccurrenceRef(ref)
		if parseErr != nil {
			return TaskRefResolution{}, RuntimeError{Code: "task_occurrence_not_found", Message: parseErr.Error()}
		}
		tsk, err = s.taskOccurrenceRepo.GetOccurrence(s.workspaceID, seriesID, slot)
		if errors.Is(err, storage.ErrOccurrenceNotFound) {
			return TaskRefResolution{}, RuntimeError{Code: "task_occurrence_not_found", Message: "projected occurrence has no task row"}
		}
	} else if write {
		tsk, err = s.resolveTargetForWrite(ref)
	} else {
		tsk, err = s.resolveTargetForRead(ref)
	}
	if err != nil {
		return TaskRefResolution{}, err
	}
	if write {
		err = s.ensureWritableTaskScope(tsk)
	} else {
		err = s.ensureReadableTaskScope(tsk)
	}
	if err != nil {
		return TaskRefResolution{}, err
	}
	return s.taskRefResolutionFromTask(tsk)
}

func (s *Service) taskRefResolutionFromTask(tsk domain.Task) (TaskRefResolution, error) {
	userInfos, err := s.resolveUserInfos(collectAssigneeUserIDs([]domain.Task{tsk}))
	if err != nil {
		return TaskRefResolution{}, err
	}
	view := taskToView(tsk, userInfoList(tsk.Assignees, userInfos))
	if view.RecurrenceInfo != nil {
		if series, seriesErr := s.taskSeriesRepo.Get(s.workspaceID, view.RecurrenceInfo.SeriesID); seriesErr == nil {
			view.RecurrenceInfo.SeriesTitle = series.Title
			view.RecurrenceInfo.SeriesStatus = series.Status
			view.RecurrenceInfo.Until = series.Until
		} else if !errors.Is(seriesErr, storage.ErrSeriesNotFound) {
			return TaskRefResolution{}, seriesErr
		}
	}
	return taskRefResolutionFromView(view, &tsk), nil
}

func taskRefResolutionFromView(view TaskOccurrenceView, tsk *domain.Task) TaskRefResolution {
	resolution := TaskRefResolution{
		Kind:            TaskResourceNormal,
		StableID:        view.ID,
		UUID:            cloneStringPtr(view.UUID),
		TaskSlug:        cloneStringPtr(view.TaskSlug),
		Materialization: "materialized",
		Task:            tsk,
		View:            view,
	}
	if view.RecurrenceInfo == nil {
		return resolution
	}
	occurrenceRef := view.ID
	resolution.Kind = TaskResourceOccurrence
	resolution.StableID = occurrenceRef
	resolution.OccurrenceRef = &occurrenceRef
	resolution.Materialization = view.RecurrenceInfo.Materialization
	return resolution
}

// changedOccurrenceFields 把真实字段 diff 映射为 recurrence override canonical key。
// no-op 不产生 override；同一字段重复修改由 NormalizeRecurrenceOverrides 去重。
func changedOccurrenceFields(before, after domain.Task) []string {
	diff := diffTaskChanges(before, after)
	fields := make([]string, 0, 11)
	if diff.TitleChanged {
		fields = append(fields, "title")
	}
	if diff.DescriptionChanged {
		fields = append(fields, "description")
	}
	if diff.PriorityChanged {
		fields = append(fields, "priority")
	}
	if diff.DueChanged {
		fields = append(fields, "due")
	}
	if diff.AssigneesChanged {
		fields = append(fields, "assignees")
	}
	if diff.TagsChanged {
		fields = append(fields, "tags")
	}
	if diff.UDAsChanged {
		fields = append(fields, "udas")
	}
	if diff.WaitChanged {
		fields = append(fields, "wait")
	}
	if diff.ScheduledChanged {
		fields = append(fields, "scheduled")
	}
	if diff.DependsChanged {
		fields = append(fields, "depends")
	}
	return fields
}
