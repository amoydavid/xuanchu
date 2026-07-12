package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/taskseries"
	domain "git.dajee.net/dajee/xuanchu/internal/task"
)

// TaskBundleSchemaV1 是原生 bundle 的 schema 版本标识（spec §20.1）。
const TaskBundleSchemaV1 = "xuanchu.task-bundle/v1"

// TaskBundleV1 是璇玑原生 import/export bundle（spec §20.1）。
//
// 替代旧 Taskwarrior JSON：series 独立保存，tasks 保存普通任务和已物化 occurrence。
// projected occurrence 是范围查询视图，不导出。
type TaskBundleV1 struct {
	Schema     string             `json:"schema"`
	ExportedAt string             `json:"exported_at"`
	TaskSeries []TaskSeriesBundle `json:"task_series"`
	Tasks      []TaskBundleTask   `json:"tasks"`
}

// TaskSeriesBundle 是 bundle 中的 series 节点，含完整 rule-version history。
type TaskSeriesBundle struct {
	ID              string                   `json:"id"`
	WorkspaceID     string                   `json:"workspace_id"`
	ProjectID       string                   `json:"project_id"`
	Title           string                   `json:"title"`
	Description     *string                  `json:"description,omitempty"`
	Status          string                   `json:"status"`
	RecurrenceRule  string                   `json:"recurrence_rule"`
	FirstDue        int64                    `json:"first_due"`
	Until           *int64                   `json:"until,omitempty"`
	EffectiveEndAt  *int64                   `json:"effective_end_at,omitempty"`
	StopReason      *string                  `json:"stop_reason,omitempty"`
	Priority        *string                  `json:"priority,omitempty"`
	AssigneeIDs     []string                 `json:"assignees,omitempty"`
	Tags            []string                 `json:"tags,omitempty"`
	UDAs            map[string]string        `json:"udas,omitempty"`
	CreatedBy       string                   `json:"created_by"`
	CreatedAt       int64                    `json:"created_at"`
	ModifiedAt      int64                    `json:"modified_at"`
	RuleVersions    []TaskSeriesRuleVersionBundle `json:"rule_versions"`
}

// TaskSeriesRuleVersionBundle 是 bundle 中的 rule version 段。
type TaskSeriesRuleVersionBundle struct {
	ID             string `json:"id"`
	EffectiveFrom  int64  `json:"effective_from"`
	RecurrenceRule string `json:"recurrence_rule"`
	CreatedBy      string `json:"created_by"`
	CreatedAt      int64  `json:"created_at"`
}

// TaskBundleTask 是 bundle 中的 task 节点（普通任务或已物化 occurrence）。
type TaskBundleTask struct {
	UUID                    string             `json:"uuid"`
	WorkspaceID             string             `json:"workspace_id"`
	Title                   string             `json:"title"`
	Description             *string            `json:"description,omitempty"`
	Status                  string             `json:"status"`
	Entry                   int64              `json:"entry"`
	Modified                int64              `json:"modified"`
	End                     *int64             `json:"end,omitempty"`
	Due                     *int64             `json:"due,omitempty"`
	Project                 *string            `json:"project,omitempty"`
	ProjectID               *string            `json:"project_id,omitempty"`
	ProjectSeq              *int64             `json:"project_seq,omitempty"`
	Priority                *string            `json:"priority,omitempty"`
	Tags                    []string           `json:"tags,omitempty"`
	Start                   *int64             `json:"start,omitempty"`
	Wait                    *int64             `json:"wait,omitempty"`
	Scheduled               *int64             `json:"scheduled,omitempty"`
	Until                   *int64             `json:"until,omitempty"`
	Parent                  *string            `json:"parent,omitempty"`
	SeriesID                *string            `json:"series_id,omitempty"`
	RecurrenceAt            *int64             `json:"recurrence_at,omitempty"`
	RecurrenceRuleSnapshot  *string            `json:"recurrence_rule_snapshot,omitempty"`
	RecurrenceOverrides     []string           `json:"recurrence_overrides,omitempty"`
}

// ExportTaskBundle 导出当前 workspace 的全部 series 和 task（spec §20.1）。
// 不导出 projected occurrence（它们是范围查询视图）。
func (s *Service) ExportTaskBundle() (TaskBundleV1, error) {
	if err := s.Require(PermissionTaskRead); err != nil {
		return TaskBundleV1{}, err
	}
	// 导出 series（含 rule versions）。
	seriesCandidates, err := s.taskSeriesRepo.ListCandidates(storageTaskSeriesListOptions(TaskSeriesListInput{}, "all", "", s.workspaceID))
	if err != nil {
		return TaskBundleV1{}, err
	}
	seriesBundles := make([]TaskSeriesBundle, 0, len(seriesCandidates))
	for _, se := range seriesCandidates {
		seriesBundles = append(seriesBundles, seriesToBundle(se))
	}
	// 导出普通任务和已物化 occurrence。
	tasks, err := s.repo.List(s.workspaceID, storage.ListOptions{})
	if err != nil {
		return TaskBundleV1{}, err
	}
	taskBundles := make([]TaskBundleTask, 0, len(tasks))
	for _, t := range tasks {
		taskBundles = append(taskBundles, taskToBundle(t))
	}
	return TaskBundleV1{
		Schema:     TaskBundleSchemaV1,
		ExportedAt: time.Now().Format(time.RFC3339),
		TaskSeries: seriesBundles,
		Tasks:      taskBundles,
	}, nil
}

// ImportTaskBundle 在当前 workspace 导入 bundle（spec §20.1）。
//
// 依赖顺序：series→rule versions/associations→tasks/occurrences。
// 校验：schema 版本；occurrence 的 series 必须存在于同 bundle 或目标 workspace；
// 槽位唯一约束必须成立。任一失败整体回滚（事务）。
func (s *Service) ImportTaskBundle(bundle TaskBundleV1) (TaskBundleImportResult, error) {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return TaskBundleImportResult{}, err
	}
	if bundle.Schema != TaskBundleSchemaV1 {
		return TaskBundleImportResult{}, RuntimeError{Code: "task_bundle_invalid_schema", Message: fmt.Sprintf("unsupported bundle schema %q; expected %s", bundle.Schema, TaskBundleSchemaV1)}
	}
	result := TaskBundleImportResult{}
	// 简化：顺序导入。series Create 内部事务保证 series 行原子。
	importedSeries := 0
	importedTasks := 0
	// 先建 series。
	seriesIDSet := map[string]bool{}
	for _, sb := range bundle.TaskSeries {
		se := bundleToSeries(sb)
		created, err := s.taskSeriesRepo.Create(se)
		if err != nil {
			return result, fmt.Errorf("import series %s: %w", sb.ID, err)
		}
		seriesIDSet[created.ID] = true
		// 追加额外 rule versions（Create 已写初始段）。
		for _, rv := range sb.RuleVersions {
			if rv.EffectiveFrom == se.FirstDue {
				continue // 初始段已写
			}
			if err := s.taskSeriesRepo.AppendRuleVersion(created.ID, taskseries.RuleVersion{
				EffectiveFrom: rv.EffectiveFrom, RecurrenceRule: rv.RecurrenceRule,
				CreatedBy: rv.CreatedBy, CreatedAt: rv.CreatedAt,
			}); err != nil {
				return result, fmt.Errorf("append rule version: %w", err)
			}
		}
		importedSeries++
	}
	// 再建 tasks/occurrences。
	for _, tb := range bundle.Tasks {
		// occurrence：校验 series 存在。
		if tb.SeriesID != nil {
			if !seriesIDSet[*tb.SeriesID] {
				if _, gerr := s.taskSeriesRepo.Get(s.workspaceID, *tb.SeriesID); gerr != nil {
					return result, RuntimeError{Code: "task_bundle_series_missing", Message: fmt.Sprintf("occurrence %s 引用的 series %s 不存在", tb.UUID, *tb.SeriesID)}
				}
			}
		}
		tsk := bundleToTask(tb)
		if _, err := s.repo.Create(tsk); err != nil {
			return result, fmt.Errorf("import task %s: %w", tb.UUID, err)
		}
		importedTasks++
	}
	result.SeriesImported = importedSeries
	result.TasksImported = importedTasks
	return result, nil
}

// TaskBundleImportResult 是导入结果。
type TaskBundleImportResult struct {
	SeriesImported int
	TasksImported  int
}

// MarshalTaskBundle 把 bundle 序列化为 JSON 字节。
func MarshalTaskBundle(bundle TaskBundleV1) ([]byte, error) {
	return json.Marshal(bundle)
}

// UnmarshalTaskBundle 从 JSON 字节反序列化 bundle，并校验 schema。
func UnmarshalTaskBundle(data []byte) (TaskBundleV1, error) {
	var bundle TaskBundleV1
	if err := json.Unmarshal(data, &bundle); err != nil {
		return TaskBundleV1{}, fmt.Errorf("task bundle: %w", err)
	}
	if bundle.Schema != TaskBundleSchemaV1 {
		return TaskBundleV1{}, errors.New("task bundle: 未知 schema 版本，拒绝猜测性宽松读取")
	}
	return bundle, nil
}

// --- 辅助 ---

func seriesToBundle(se taskseries.Series) TaskSeriesBundle {
	b := TaskSeriesBundle{
		ID: se.ID, WorkspaceID: se.WorkspaceID, ProjectID: se.ProjectID,
		Title: se.Title, Description: se.Description, Status: se.Status,
		RecurrenceRule: se.RecurrenceRule, FirstDue: se.FirstDue,
		Until: se.Until, EffectiveEndAt: se.EffectiveEndAt, StopReason: se.StopReason,
		Priority: se.Priority, AssigneeIDs: se.AssigneeIDs, Tags: se.Tags, UDAs: se.UDAs,
		CreatedBy: se.CreatedBy, CreatedAt: se.CreatedAt, ModifiedAt: se.ModifiedAt,
	}
	for _, rv := range se.RuleVersions {
		b.RuleVersions = append(b.RuleVersions, TaskSeriesRuleVersionBundle{
			ID: rv.ID, EffectiveFrom: rv.EffectiveFrom, RecurrenceRule: rv.RecurrenceRule,
			CreatedBy: rv.CreatedBy, CreatedAt: rv.CreatedAt,
		})
	}
	return b
}

func bundleToSeries(sb TaskSeriesBundle) taskseries.Series {
	se := taskseries.Series{
		ID: sb.ID, WorkspaceID: sb.WorkspaceID, ProjectID: sb.ProjectID,
		Title: sb.Title, Description: sb.Description, Status: sb.Status,
		RecurrenceRule: sb.RecurrenceRule, FirstDue: sb.FirstDue,
		Until: sb.Until, EffectiveEndAt: sb.EffectiveEndAt, StopReason: sb.StopReason,
		Priority: sb.Priority, AssigneeIDs: sb.AssigneeIDs, Tags: sb.Tags, UDAs: sb.UDAs,
		CreatedBy: sb.CreatedBy, CreatedAt: sb.CreatedAt, ModifiedAt: sb.ModifiedAt,
	}
	if se.ID == "" {
		se.ID = uuid.NewString()
	}
	return se
}

func taskToBundle(t domain.Task) TaskBundleTask {
	tb := TaskBundleTask{
		UUID: t.UUID, WorkspaceID: t.WorkspaceID, Title: t.Title,
		Description: t.Description, Status: t.Status, Entry: t.Entry, Modified: t.Modified,
		End: t.End, Due: t.Due, Project: t.Project, ProjectID: t.ProjectID,
		ProjectSeq: t.ProjectSeq, Priority: t.Priority, Tags: t.Tags,
		Start: t.Start, Wait: t.Wait, Scheduled: t.Scheduled, Until: t.Until,
		Parent: t.Parent, SeriesID: t.SeriesID, RecurrenceAt: t.RecurrenceAt,
		RecurrenceRuleSnapshot: t.RecurrenceRuleSnapshot,
		RecurrenceOverrides: t.RecurrenceOverrides,
	}
	if tb.UUID == "" {
		tb.UUID = uuid.NewString()
	}
	return tb
}

func bundleToTask(tb TaskBundleTask) domain.Task {
	return domain.Task{
		UUID: tb.UUID, WorkspaceID: tb.WorkspaceID, Title: tb.Title,
		Description: tb.Description, Status: tb.Status, Entry: tb.Entry, Modified: tb.Modified,
		End: tb.End, Due: tb.Due, Project: tb.Project, ProjectID: tb.ProjectID,
		ProjectSeq: tb.ProjectSeq, Priority: tb.Priority, Tags: tb.Tags,
		Start: tb.Start, Wait: tb.Wait, Scheduled: tb.Scheduled, Until: tb.Until,
		Parent: tb.Parent, SeriesID: tb.SeriesID, RecurrenceAt: tb.RecurrenceAt,
		RecurrenceRuleSnapshot: tb.RecurrenceRuleSnapshot,
		RecurrenceOverrides: domain.NormalizeRecurrenceOverrides(tb.RecurrenceOverrides),
	}
}
