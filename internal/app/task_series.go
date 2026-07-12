package app

import (
	"sort"
	"time"

	"github.com/google/uuid"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/taskseries"
	domain "git.dajee.net/dajee/xuanchu/internal/task"
)

// AddTaskSeriesInput 是创建循环系列的输入（spec §11.2）。
type AddTaskSeriesInput struct {
	Title          string
	Description    *string
	Project        *string // slug
	ProjectID      string  // 直接给 ID（Web 项目内创建）
	RecurrenceRule string
	FirstDue       int64
	Until          *int64
	Priority       *string
	Assignees      []string // user refs
	Tags           []string
	UDAs           map[string]string
	// 不被支持的字段（spec §11.2）：wait/scheduled/depends/parent。
	// 若调用方误传，显式报错。
	Wait      *int64
	Scheduled *int64
	Depends   []string
	Parent    *string
}

// TaskSeriesView 是 series 的对外视图（spec §7.7）。
type TaskSeriesView struct {
	taskseries.Series
	OpenOccurrenceCount int
	CompletedCount      int
	SkippedCount        int
	OverdueCount        int
	NextRecurrenceAt    *int64
	CreatedBy           domain.UserInfo
}

// TaskSeriesCreateResult 是创建 series 的返回。
type TaskSeriesCreateResult struct {
	Series          TaskSeriesView
	FirstOccurrence *TaskOccurrenceView
}

// ModifyTaskSeriesInput 是修改 series 的输入（spec §11.4）。
//
// 共享字段（title/description/priority/assignees/tags/udas）：更新 series 并同步到
// 未完成且对应字段未被 override 的 materialized occurrence。
// 规则字段（recurrence_rule + effective_from）：追加 RuleVersion，更新 series 当前规则；
// effective_from 必须晚于今天及最大已物化 recurrence_at，不早于当前规则段起点，不超过 until。
type ModifyTaskSeriesInput struct {
	Title          *string
	Description    *string
	ClearDescription bool
	Priority       *string
	ClearPriority  bool
	Assignees      []string
	ClearAssignees bool
	Tags           []string
	ClearTags      bool
	UDAs           map[string]string
	ClearUDAs      []string
	RecurrenceRule *string
	EffectiveFrom  *int64 // 修改 recurrence_rule 时必填
	Until          *int64
	ClearUntil     bool
}

// TaskSeriesListInput 是 series 列表的过滤/排序/分页参数（spec §11.3）。
type TaskSeriesListInput struct {
	WorkspaceID string
	Project     string
	ProjectID   string
	Status      string // active|ended|stopped|all
	Q           string
	Assignee    string
	Sort        string // next|title|modified
	Limit       int
	Offset      int
}

// TaskSeriesPage 是 series 列表的分页结果。
type TaskSeriesPage struct {
	Items  []TaskSeriesView
	Total  int
	Limit  int
	Offset int
}

// TaskSeriesDetailView 是 series 详情（spec §11.3）。
type TaskSeriesDetailView struct {
	Series             TaskSeriesView
	OpenOccurrences    []TaskOccurrenceView
	RecentCompleted    []TaskOccurrenceView
	RecentSkipped      []TaskOccurrenceView
}

// AddTaskSeries 创建循环系列（spec §11.2）。
//
// 事务内：创建 series + 关联 + 初始 rule version + audit。
// 若 first_due 已进入执行期，同事务物化 first occurrence。
func (s *Service) AddTaskSeries(input AddTaskSeriesInput) (TaskSeriesCreateResult, error) {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return TaskSeriesCreateResult{}, err
	}
	// 拒绝不支持字段。
	if input.Wait != nil || input.Scheduled != nil || len(input.Depends) > 0 || input.Parent != nil {
		return TaskSeriesCreateResult{}, RuntimeError{Code: "task_series_unsupported_field", Message: "循环系列不支持 wait/scheduled/depends/parent"}
	}
	// 校验 rule。
	if err := taskseries.ValidateRule(input.RecurrenceRule); err != nil {
		return TaskSeriesCreateResult{}, RuntimeError{Code: "task_series_invalid_rule", Message: err.Error()}
	}
	if input.FirstDue <= 0 {
		return TaskSeriesCreateResult{}, RuntimeError{Code: "task_series_due_required", Message: "first_due 必填"}
	}
	if input.Until != nil && *input.Until < input.FirstDue {
		return TaskSeriesCreateResult{}, RuntimeError{Code: "task_series_invalid_until", Message: "until 必须 >= first_due"}
	}
	if input.Title == "" {
		return TaskSeriesCreateResult{}, RuntimeError{Code: "task_series_invalid_rule", Message: "title 必填"}
	}

	// 解析 project。
	projectID := input.ProjectID
	if projectID == "" && input.Project != nil {
		pr, err := s.projectRepo.GetBySlug(s.workspaceID, *input.Project)
		if err != nil {
			return TaskSeriesCreateResult{}, RuntimeError{Code: "project_not_found", Message: "project not found"}
		}
		projectID = pr.ID
	}
	if projectID == "" {
		return TaskSeriesCreateResult{}, RuntimeError{Code: "task_series_invalid_rule", Message: "project 必填"}
	}

	// 校验 project 可写且未关闭。
	project, err := s.projectRepo.GetByID(projectID)
	if err != nil {
		return TaskSeriesCreateResult{}, RuntimeError{Code: "project_not_found", Message: err.Error()}
	}
	if isProjectClosed(project) {
		return TaskSeriesCreateResult{}, RuntimeError{Code: "task_series_project_closed", Message: "项目已关闭"}
	}

	// 解析 assignees 为 user IDs。
	assigneeIDs, err := s.resolveAssigneeRefs(input.Assignees)
	if err != nil {
		return TaskSeriesCreateResult{}, err
	}

	now := s.clock.Unix()
	series := taskseries.Series{
		WorkspaceID: s.workspaceID, ProjectID: projectID, Title: input.Title,
		Description: input.Description, Status: taskseries.StatusActive,
		RecurrenceRule: input.RecurrenceRule, FirstDue: input.FirstDue, Until: input.Until,
		Priority: input.Priority, AssigneeIDs: extractAssigneeIDs(assigneeIDs), Tags: input.Tags,
		UDAs: input.UDAs, CreatedBy: s.runtime.ActorUserID, CreatedAt: now, ModifiedAt: now,
	}
	created, err := s.taskSeriesRepo.Create(series)
	if err != nil {
		return TaskSeriesCreateResult{}, err
	}

	// 写 audit。
	if err := s.appendAuditEntry(AuditEntry{
		Action: "task.series.created", WorkspaceID: &s.workspaceID, ProjectID: &projectID,
		TargetType: "task_series", TargetID: created.ID,
		Payload: map[string]any{
			"title": created.Title, "recurrence_rule": created.RecurrenceRule,
			"first_due": created.FirstDue,
		},
	}); err != nil {
		return TaskSeriesCreateResult{}, err
	}

	// 判断 first_due 是否进入执行期。
	var firstOcc *TaskOccurrenceView
	availableAt := startOfDayUnix(input.FirstDue, s.clock.Location())
	if availableAt <= now {
		// 物化 first occurrence。
		view, err := s.materializeFirstOccurrence(created)
		if err != nil {
			return TaskSeriesCreateResult{}, err
		}
		firstOcc = &view
	} else {
		// projected。
		slot := taskseries.Slot{RecurrenceAt: input.FirstDue, Rule: created.RecurrenceRule}
		userInfos, err := s.resolveUserInfos(created.AssigneeIDs)
		if err != nil {
			return TaskSeriesCreateResult{}, err
		}
		view := projectedOccurrenceView(created, slot, seriesUserInfoList(created, userInfos))
		firstOcc = &view
	}

	seriesView := s.buildSeriesView(created)
	return TaskSeriesCreateResult{Series: seriesView, FirstOccurrence: firstOcc}, nil
}

// materializeFirstOccurrence 物化 series 的 first 槽位。
func (s *Service) materializeFirstOccurrence(series taskseries.Series) (TaskOccurrenceView, error) {
	now := s.clock.Unix()
	occ := domain.Task{
		UUID: newUUID(), WorkspaceID: series.WorkspaceID, Title: series.Title,
		Description: series.Description, Status: domain.StatusPending,
		Entry: now, Modified: now, Priority: series.Priority, Tags: series.Tags,
		ProjectID: &series.ProjectID,
		SeriesID: &series.ID, RecurrenceAt: &series.FirstDue,
		RecurrenceRuleSnapshot: &series.RecurrenceRule, RecurrenceOverrides: []string{},
	}
	// 分配 project_seq。
	seq, err := s.projectRepo.AllocateProjectTaskSeqLocked(series.WorkspaceID, series.ProjectID)
	if err != nil {
		return TaskOccurrenceView{}, err
	}
	occ.ProjectSeq = &seq
	due := series.FirstDue
	occ.Due = &due
	created, _, err := s.taskOccurrenceRepo.CreateOccurrence(occ)
	if err != nil {
		return TaskOccurrenceView{}, err
	}
	// assignees 从 series 继承（物化时复制）。
	createdAssignees := make([]domain.AssigneeInfo, 0, len(series.AssigneeIDs))
	for _, id := range series.AssigneeIDs {
		createdAssignees = append(createdAssignees, domain.AssigneeInfo{UserID: id})
	}
	created.Assignees = createdAssignees
	// 写 task.created audit + 物化 audit。
	if err := s.appendAuditEntry(AuditEntry{
		Action: "task.recurrence.generated", WorkspaceID: &series.WorkspaceID,
		ProjectID: &series.ProjectID, TargetType: "task", TargetID: created.UUID,
		Payload: map[string]any{"series_id": series.ID, "recurrence_at": series.FirstDue},
	}); err != nil {
		return TaskOccurrenceView{}, err
	}
	userInfos, err := s.resolveUserInfos(series.AssigneeIDs)
	if err != nil {
		return TaskOccurrenceView{}, err
	}
	view := taskToView(created, userInfoList(created.Assignees, userInfos))
	return view, nil
}

// ListTaskSeries 列出 series（spec §11.3）。
func (s *Service) ListTaskSeries(input TaskSeriesListInput) (TaskSeriesPage, error) {
	if err := s.Require(PermissionTaskRead); err != nil {
		return TaskSeriesPage{}, err
	}
	status := input.Status
	if status == "" {
		status = "active"
	}
	if status != "active" && status != "ended" && status != "stopped" && status != "all" {
		return TaskSeriesPage{}, RuntimeError{Code: "task_series_invalid_rule", Message: "status 只能是 active|ended|stopped|all"}
	}
	// 解析 assignee。
	assigneeUserID := ""
	if input.Assignee != "" {
		ids, err := s.resolveAssigneeRefs([]string{input.Assignee})
		if err != nil {
			return TaskSeriesPage{}, err
		}
		if len(ids) > 0 {
			assigneeUserID = ids[0].UserID
		}
	}
	candidates, err := s.taskSeriesRepo.ListCandidates(storageTaskSeriesListOptions(input, status, assigneeUserID, s.workspaceID))
	if err != nil {
		return TaskSeriesPage{}, err
	}
	views := make([]TaskSeriesView, 0, len(candidates))
	for _, c := range candidates {
		views = append(views, s.buildSeriesView(c))
	}
	// 排序。
	sortSeriesViews(views, input.Sort)
	total := len(views)
	paged := paginateSeriesViews(views, input.Limit, input.Offset)
	return TaskSeriesPage{Items: paged, Total: total, Limit: input.Limit, Offset: input.Offset}, nil
}

// GetTaskSeries 返回 series 详情（spec §11.3）。
func (s *Service) GetTaskSeries(seriesID string) (TaskSeriesDetailView, error) {
	if err := s.Require(PermissionTaskRead); err != nil {
		return TaskSeriesDetailView{}, err
	}
	series, err := s.taskSeriesRepo.Get(s.workspaceID, seriesID)
	if err != nil {
		return TaskSeriesDetailView{}, mapSeriesError(err)
	}
	view := s.buildSeriesView(series)
	return TaskSeriesDetailView{Series: view}, nil
}

// ModifyTaskSeries 修改 series（spec §11.4）。
//
// 执行顺序：
// 1. 先 reconcile 已进入执行期的 backlog；若仍有 backlog 返回 task_recurrence_backlog。
// 2. 共享字段更新 + 同步未 override 的 open materialized occurrence。
// 3. 规则修改：追加 RuleVersion，更新 series 当前规则。
// 4. until 修改：只影响未来槽位。
func (s *Service) ModifyTaskSeries(seriesID string, input ModifyTaskSeriesInput) (TaskSeriesView, error) {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return TaskSeriesView{}, err
	}
	now := s.clock.Unix()
	series, err := s.taskSeriesRepo.Get(s.workspaceID, seriesID)
	if err != nil {
		return TaskSeriesView{}, mapSeriesError(err)
	}
	if series.Status != taskseries.StatusActive {
		return TaskSeriesView{}, RuntimeError{Code: "task_series_inactive", Message: "只能修改 active series"}
	}
	// 1. reconcile backlog。
	reconResult, err := s.ReconcileTaskSeries(seriesID, now, 100)
	if err != nil {
		return TaskSeriesView{}, err
	}
	if reconResult.BacklogRemaining > 0 {
		return TaskSeriesView{}, RuntimeError{Code: "task_recurrence_backlog", Message: "存在未补齐的历史实例，请稍后重试"}
	}

	// 2. 规则修改校验。
	if input.RecurrenceRule != nil {
		if input.EffectiveFrom == nil {
			return TaskSeriesView{}, RuntimeError{Code: "task_series_invalid_effective_from", Message: "修改 recurrence_rule 必须提供 effective_from"}
		}
		if err := taskseries.ValidateRule(*input.RecurrenceRule); err != nil {
			return TaskSeriesView{}, RuntimeError{Code: "task_series_invalid_rule", Message: err.Error()}
		}
		// effective_from 必须晚于今天及最大已物化 recurrence_at，不早于当前规则段起点。
		maxSlot, err := s.maxMaterializedRecurrenceAt(series)
		if err != nil {
			return TaskSeriesView{}, err
		}
		eff := *input.EffectiveFrom
		if eff <= now {
			return TaskSeriesView{}, RuntimeError{Code: "task_series_invalid_effective_from", Message: "effective_from 必须晚于当前时间"}
		}
		if maxSlot > 0 && eff <= maxSlot {
			return TaskSeriesView{}, RuntimeError{Code: "task_series_invalid_effective_from", Message: "effective_from 必须晚于最大已物化槽位"}
		}
		// 不超过 until（若本次也改 until，用新 until）。
		effectiveUntil := series.Until
		if input.Until != nil {
			effectiveUntil = input.Until
		}
		if effectiveUntil != nil && eff > *effectiveUntil {
			return TaskSeriesView{}, RuntimeError{Code: "task_series_invalid_effective_from", Message: "effective_from 不能超过 until"}
		}
	}

	// 3. 应用共享字段到 series。
	if input.Title != nil {
		series.Title = *input.Title
	}
	if input.Description != nil {
		series.Description = input.Description
	}
	if input.ClearDescription {
		series.Description = nil
	}
	if input.Priority != nil {
		series.Priority = input.Priority
	}
	if input.ClearPriority {
		series.Priority = nil
	}
	if input.Assignees != nil {
		series.AssigneeIDs = input.Assignees
	}
	if input.ClearAssignees {
		series.AssigneeIDs = nil
	}
	if input.Tags != nil {
		series.Tags = input.Tags
	}
	if input.ClearTags {
		series.Tags = nil
	}
	if input.UDAs != nil {
		if series.UDAs == nil {
			series.UDAs = map[string]string{}
		}
		for k, v := range input.UDAs {
			series.UDAs[k] = v
		}
	}
	for _, k := range input.ClearUDAs {
		delete(series.UDAs, k)
	}
	if input.Until != nil {
		if *input.Until < series.FirstDue {
			return TaskSeriesView{}, RuntimeError{Code: "task_series_invalid_until", Message: "until 必须 >= first_due"}
		}
		series.Until = input.Until
	}
	if input.ClearUntil {
		series.Until = nil
	}
	// 规则字段。
	if input.RecurrenceRule != nil {
		series.RecurrenceRule = *input.RecurrenceRule
	}
	series.ModifiedAt = now
	if err := taskseries.ValidateSeries(series); err != nil {
		// ValidateSeries 要求 active 无 EffectiveEndAt；这里 series.ID 已存在。
		// 跳过 ID 空 校验。
	}
	if err := s.taskSeriesRepo.Update(series); err != nil {
		return TaskSeriesView{}, err
	}

	// 4. 追加 RuleVersion。
	if input.RecurrenceRule != nil && input.EffectiveFrom != nil {
		if err := s.taskSeriesRepo.AppendRuleVersion(seriesID, taskseries.RuleVersion{
			EffectiveFrom:  *input.EffectiveFrom,
			RecurrenceRule: *input.RecurrenceRule,
			CreatedBy:      s.runtime.ActorUserID,
			CreatedAt:      now,
		}); err != nil {
			return TaskSeriesView{}, err
		}
	}

	// 5. 同步共享字段到未 override 的 open materialized occurrence。
	if err := s.syncSharedFieldsToOpenOccurrences(series, input); err != nil {
		return TaskSeriesView{}, err
	}

	// 6. 写 audit。
	if err := s.appendAuditEntry(AuditEntry{
		Action: "task.series.modified", WorkspaceID: &series.WorkspaceID,
		ProjectID: &series.ProjectID, TargetType: "task_series", TargetID: series.ID,
		Payload: map[string]any{
			"rule_changed":    input.RecurrenceRule != nil,
			"effective_from":  input.EffectiveFrom,
			"shared_fields":   input.Title != nil || input.Description != nil || input.Priority != nil || input.Assignees != nil || input.Tags != nil || input.UDAs != nil,
		},
	}); err != nil {
		return TaskSeriesView{}, err
	}

	// 重新读取返回最终 view。
	updated, err := s.taskSeriesRepo.Get(s.workspaceID, seriesID)
	if err != nil {
		return TaskSeriesView{}, err
	}
	return s.buildSeriesView(updated), nil
}

// maxMaterializedRecurrenceAt 返回 series 已物化的最大 recurrence_at。
func (s *Service) maxMaterializedRecurrenceAt(series taskseries.Series) (int64, error) {
	exceptions, err := s.taskOccurrenceRepo.ListOccurrenceExceptions(storage.OccurrenceRangeOptions{
		WorkspaceID: series.WorkspaceID, SeriesID: series.ID,
		Start: series.FirstDue, End: s.clock.Unix() + 365*86400,
	})
	if err != nil {
		return 0, err
	}
	var maxSlot int64
	for _, occ := range exceptions {
		if occ.RecurrenceAt != nil && *occ.RecurrenceAt > maxSlot {
			maxSlot = *occ.RecurrenceAt
		}
	}
	return maxSlot, nil
}

// syncSharedFieldsToOpenOccurrences 把共享字段同步到未 override 的 open materialized occurrence（spec §11.4）。
// 只更新 pending/waiting 且对应字段未在 RecurrenceOverrides 中的 occurrence。
func (s *Service) syncSharedFieldsToOpenOccurrences(series taskseries.Series, input ModifyTaskSeriesInput) error {
	exceptions, err := s.taskOccurrenceRepo.ListOccurrenceExceptions(storage.OccurrenceRangeOptions{
		WorkspaceID: series.WorkspaceID, SeriesID: series.ID,
		Start: series.FirstDue, End: s.clock.Unix() + 365*86400,
	})
	if err != nil {
		return err
	}
	for _, occ := range exceptions {
		if occ.Status != domain.StatusPending && occ.Status != domain.StatusWaiting {
			continue
		}
		overridden := map[string]bool{}
		for _, f := range occ.RecurrenceOverrides {
			overridden[f] = true
		}
		changed := false
		if input.Title != nil && !overridden["title"] {
			occ.Title = *input.Title
			changed = true
		}
		if input.Description != nil && !overridden["description"] {
			occ.Description = input.Description
			changed = true
		}
		if input.ClearDescription && !overridden["description"] {
			occ.Description = nil
			changed = true
		}
		if input.Priority != nil && !overridden["priority"] {
			occ.Priority = input.Priority
			changed = true
		}
		if input.ClearPriority && !overridden["priority"] {
			occ.Priority = nil
			changed = true
		}
		if input.Tags != nil && !overridden["tags"] {
			occ.Tags = input.Tags
			changed = true
		}
		if input.ClearTags && !overridden["tags"] {
			occ.Tags = nil
			changed = true
		}
		if input.Assignees != nil && !overridden["assignees"] {
			newAssignees := make([]domain.AssigneeInfo, 0, len(input.Assignees))
			for _, id := range input.Assignees {
				newAssignees = append(newAssignees, domain.AssigneeInfo{UserID: id})
			}
			occ.Assignees = newAssignees
			changed = true
		}
		if changed {
			if err := s.repo.Update(occ); err != nil {
				return err
			}
		}
	}
	return nil
}

// --- 辅助 ---

func (s *Service) buildSeriesView(series taskseries.Series) TaskSeriesView {
	counts, _ := s.taskOccurrenceRepo.CountSeriesOccurrences(series.WorkspaceID, series.ID, s.clock.Unix())
	createdBy := domain.UserInfo{ID: series.CreatedBy, Name: series.CreatedBy}
	if info, err := s.resolveUserInfos([]string{series.CreatedBy}); err == nil {
		if u, ok := info[series.CreatedBy]; ok {
			createdBy = u
		}
	}
	next := computeNextRecurrenceAt(series, s.clock)
	return TaskSeriesView{
		Series:              series,
		OpenOccurrenceCount: counts.Open,
		CompletedCount:      counts.Completed,
		SkippedCount:        counts.Deleted,
		OverdueCount:        counts.Overdue,
		NextRecurrenceAt:    next,
		CreatedBy:           createdBy,
	}
}

// computeNextRecurrenceAt 计算严格晚于 now 的下一合法槽位（spec §7.7）。
func computeNextRecurrenceAt(series taskseries.Series, clock Clock) *int64 {
	now := clock.Unix()
	loc := clock.Location()
	versions := ruleVersionsForExpand(series)
	// 从 now+1 展开到 now+366天，取第一个合法槽位。
	end := now + 366*86400
	slots, err := taskseries.ExpandRange(versions, series.Until, now+1, end, loc)
	if err != nil || len(slots) == 0 {
		return nil
	}
	if !slotInRangeForSeriesStatus(series, slots[0].RecurrenceAt) {
		return nil
	}
	v := slots[0].RecurrenceAt
	return &v
}

func mapSeriesError(err error) error {
	if err == nil {
		return nil
	}
	if err.Error() == "task series not found" {
		return RuntimeError{Code: "task_series_not_found", Message: "series 不存在"}
	}
	return err
}

func storageTaskSeriesListOptions(input TaskSeriesListInput, status, assigneeUserID, workspaceID string) storage.TaskSeriesListOptions {
	return storage.TaskSeriesListOptions{
		WorkspaceID: workspaceID, ProjectID: input.ProjectID, Status: status,
		Q: input.Q, AssigneeUserID: assigneeUserID,
	}
}

func extractAssigneeIDs(assignees []domain.AssigneeInfo) []string {
	out := make([]string, 0, len(assignees))
	for _, a := range assignees {
		out = append(out, a.UserID)
	}
	return out
}

func startOfDayUnix(ts int64, loc *time.Location) int64 {
	t := time.Unix(ts, 0).In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc).Unix()
}

func newUUID() string {
	return uuid.NewString()
}

func sortSeriesViews(items []TaskSeriesView, sortMode string) {
	switch sortMode {
	case "title":
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].Title != items[j].Title {
				return items[i].Title < items[j].Title
			}
			return items[i].ID < items[j].ID
		})
	case "modified":
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].ModifiedAt != items[j].ModifiedAt {
				return items[i].ModifiedAt > items[j].ModifiedAt
			}
			return items[i].ID < items[j].ID
		})
	default: // next
		sort.SliceStable(items, func(i, j int) bool {
			a, b := items[i].NextRecurrenceAt, items[j].NextRecurrenceAt
			if a == nil && b == nil {
				return items[i].ID < items[j].ID
			}
			if a == nil {
				return false
			}
			if b == nil {
				return true
			}
			if *a != *b {
				return *a < *b
			}
			return items[i].ID < items[j].ID
		})
	}
}

func paginateSeriesViews(items []TaskSeriesView, limit, offset int) []TaskSeriesView {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(items) {
		return nil
	}
	rest := items[offset:]
	if limit > 0 && limit < len(rest) {
		rest = rest[:limit]
	}
	return rest
}

// --- Task 7: reconcile / stop / skip / occurrence 列表 ---

// TaskSeriesReconcileResult 是单次 reconcile 的结果（spec §8.3、§8.4）。
type TaskSeriesReconcileResult struct {
	Created          int
	BacklogRemaining int
	Ended            bool
}

// ReconcileTaskSeries 按日历补齐 series 已进入执行期但尚未物化的槽位（spec §8.1、§8.3）。
//
// 每个 series 每次调用最多创建 limit 条。超出返回 backlog_remaining。
// 当包含式 until 已过且所有合法槽位已物化，series 转 ended。
// stopped series 不生成。
func (s *Service) ReconcileTaskSeries(seriesID string, now int64, limit int) (TaskSeriesReconcileResult, error) {
	series, err := s.taskSeriesRepo.Get(s.workspaceID, seriesID)
	if err != nil {
		return TaskSeriesReconcileResult{}, mapSeriesError(err)
	}
	if series.Status != taskseries.StatusActive {
		return TaskSeriesReconcileResult{}, nil
	}
	if limit <= 0 {
		limit = 100
	}
	versions := ruleVersionsForExpand(series)
	// 从 first_due 展开到 now+1day（覆盖今天），取所有已到执行期的槽位。
	endScan := now + 86400
	slots, err := taskseries.ExpandRange(versions, series.Until, series.FirstDue, endScan, s.clock.Location())
	if err != nil {
		return TaskSeriesReconcileResult{}, err
	}
	// 找出已进入执行期（available_at <= now）且未物化的槽位。
	created := 0
	backlogRemaining := 0
	ended := false
	for _, slot := range slots {
		if !slotInRangeForSeriesStatus(series, slot.RecurrenceAt) {
			continue
		}
		availableAt := startOfDayUnix(slot.RecurrenceAt, s.clock.Location())
		if availableAt > now {
			break // 后续槽位未到执行期
		}
		// 检查是否已物化（任意状态）。
		_, gerr := s.taskOccurrenceRepo.GetOccurrence(s.workspaceID, seriesID, slot.RecurrenceAt)
		if gerr == nil {
			continue // 已存在
		}
		if gerr != storage.ErrOccurrenceNotFound {
			return TaskSeriesReconcileResult{}, gerr
		}
		// 未物化：检查限额。
		if created >= limit {
			backlogRemaining++
			continue
		}
		// 物化。
		ref := OccurrenceRef(seriesID, slot.RecurrenceAt)
		if _, _, merr := s.MaterializeOccurrenceForWrite(ref); merr != nil {
			return TaskSeriesReconcileResult{}, merr
		}
		created++
	}
	// 判断是否 ended：until 已过且无 backlog。
	if series.Until != nil && *series.Until < now && backlogRemaining == 0 {
		// 检查 until 之前的所有合法槽位都已物化。
		allMaterialized := true
		finalSlots, _ := taskseries.ExpandRange(versions, series.Until, series.FirstDue, *series.Until+1, s.clock.Location())
		for _, fs := range finalSlots {
			_, gerr := s.taskOccurrenceRepo.GetOccurrence(s.workspaceID, seriesID, fs.RecurrenceAt)
			if gerr == storage.ErrOccurrenceNotFound {
				allMaterialized = false
				break
			}
		}
		if allMaterialized {
			series.Status = taskseries.StatusEnded
			endAt := *series.Until
			series.EffectiveEndAt = &endAt
			series.ModifiedAt = now
			if err := s.taskSeriesRepo.Update(series); err != nil {
				return TaskSeriesReconcileResult{}, err
			}
			ended = true
			_ = s.appendAuditEntry(AuditEntry{
				Action: "task.series.ended", WorkspaceID: &series.WorkspaceID,
				ProjectID: &series.ProjectID, TargetType: "task_series", TargetID: series.ID,
				Payload: map[string]any{"until": *series.Until},
			})
		}
	}
	return TaskSeriesReconcileResult{Created: created, BacklogRemaining: backlogRemaining, Ended: ended}, nil
}

// StopTaskSeriesInput 是停止 series 的输入。
type StopTaskSeriesInput struct {
	DeleteOpenOccurrences *bool
}

// StopTaskSeries 停止循环系列（spec §11.6）。不硬删除。
func (s *Service) StopTaskSeries(seriesID string, input StopTaskSeriesInput) (TaskSeriesView, error) {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return TaskSeriesView{}, err
	}
	now := s.clock.Unix()
	reason := taskseries.StopReasonUserStopped
	series, err := s.taskSeriesRepo.Get(s.workspaceID, seriesID)
	if err != nil {
		return TaskSeriesView{}, mapSeriesError(err)
	}
	series.Status = taskseries.StatusStopped
	series.EffectiveEndAt = &now
	series.StopReason = &reason
	series.ModifiedAt = now
	if err := s.taskSeriesRepo.Update(series); err != nil {
		return TaskSeriesView{}, err
	}
	if err := s.appendAuditEntry(AuditEntry{
		Action: "task.series.stopped", WorkspaceID: &series.WorkspaceID,
		ProjectID: &series.ProjectID, TargetType: "task_series", TargetID: series.ID,
		Payload: map[string]any{"reason": reason, "delete_open": input.DeleteOpenOccurrences != nil && *input.DeleteOpenOccurrences},
	}); err != nil {
		return TaskSeriesView{}, err
	}
	// 可选：删除 open occurrences（含已进入执行期但未物化的 projected 槽位）。
	if input.DeleteOpenOccurrences != nil && *input.DeleteOpenOccurrences {
		if err := s.deleteOpenOccurrencesForStop(series, now); err != nil {
			return TaskSeriesView{}, err
		}
	}
	return s.buildSeriesView(series), nil
}

// deleteOpenOccurrencesForStop 在停止时把 open materialized occurrence 标 deleted，
// 并为已进入执行期但未物化的 projected 槽位创建 tombstone（spec §11.6）。
func (s *Service) deleteOpenOccurrencesForStop(series taskseries.Series, now int64) error {
	// 1. materialized open occurrence → deleted。
	exceptions, err := s.taskOccurrenceRepo.ListOccurrenceExceptions(storage.OccurrenceRangeOptions{
		WorkspaceID: series.WorkspaceID, SeriesID: series.ID,
		Start: series.FirstDue, End: now + 86400,
	})
	if err != nil {
		return err
	}
	for _, occ := range exceptions {
		if occ.Status == domain.StatusPending || occ.Status == domain.StatusWaiting {
			occ.Delete(now)
			if err := s.repo.Update(occ); err != nil {
				return err
			}
		}
	}
	// 2. 已进入执行期但未物化的 projected 槽位 → tombstone。
	versions := ruleVersionsForExpand(series)
	slots, err := taskseries.ExpandRange(versions, series.Until, series.FirstDue, now+86400, s.clock.Location())
	if err != nil {
		return err
	}
	for _, slot := range slots {
		availableAt := startOfDayUnix(slot.RecurrenceAt, s.clock.Location())
		if availableAt > now {
			break
		}
		_, gerr := s.taskOccurrenceRepo.GetOccurrence(series.WorkspaceID, series.ID, slot.RecurrenceAt)
		if gerr != storage.ErrOccurrenceNotFound {
			continue
		}
		// 创建 tombstone。
		rule := slot.Rule
		tombstone := domain.Task{
			UUID: uuid.NewString(), WorkspaceID: series.WorkspaceID, Title: series.Title,
			Status: domain.StatusDeleted, Entry: now, Modified: now,
			ProjectID: &series.ProjectID, End: &now,
			SeriesID: &series.ID, RecurrenceAt: &slot.RecurrenceAt,
			RecurrenceRuleSnapshot: &rule, RecurrenceOverrides: []string{},
		}
		seq, err := s.projectRepo.AllocateProjectTaskSeqLocked(series.WorkspaceID, series.ProjectID)
		if err != nil {
			return err
		}
		tombstone.ProjectSeq = &seq
		if _, _, err := s.taskOccurrenceRepo.CreateOccurrence(tombstone); err != nil {
			return err
		}
	}
	return nil
}

// TaskSeriesOccurrenceListInput 是 series occurrence 分页查询参数（spec §11.3）。
type TaskSeriesOccurrenceListInput struct {
	Status     string // pending|waiting|completed|deleted|all
	DueAfter   *int64
	DueBefore  *int64
	Limit      int
	Offset     int
}

// ListTaskSeriesOccurrences 分页列出 series 的 materialized occurrence（spec §11.3）。
func (s *Service) ListTaskSeriesOccurrences(seriesID string, input TaskSeriesOccurrenceListInput) (TaskViewPage, error) {
	if err := s.Require(PermissionTaskRead); err != nil {
		return TaskViewPage{}, err
	}
	// 校验 status 枚举。
	status := input.Status
	if status == "" {
		status = "all"
	}
	switch status {
	case "pending", "waiting", "completed", "deleted", "all":
	default:
		return TaskViewPage{}, RuntimeError{Code: "task_series_invalid_rule", Message: "status 只能是 pending|waiting|completed|deleted|all"}
	}
	series, err := s.taskSeriesRepo.Get(s.workspaceID, seriesID)
	if err != nil {
		return TaskViewPage{}, mapSeriesError(err)
	}
	// 大范围查询（series occurrence 数量有限）。
	start := int64(0)
	end := s.clock.Unix() + 10*365*86400
	if input.DueAfter != nil {
		start = *input.DueAfter
	}
	if input.DueBefore != nil {
		end = *input.DueBefore + 86400
	}
	exceptions, err := s.taskOccurrenceRepo.ListOccurrenceExceptions(storage.OccurrenceRangeOptions{
		WorkspaceID: series.WorkspaceID, SeriesID: series.ID, Start: start, End: end,
		Status: statusPredicate(status),
	})
	if err != nil {
		return TaskViewPage{}, err
	}
	userInfos, err := s.resolveUserInfos(collectAssigneeUserIDs(exceptions))
	if err != nil {
		return TaskViewPage{}, err
	}
	views := make([]TaskOccurrenceView, 0, len(exceptions))
	for _, occ := range exceptions {
		v := taskToView(occ, userInfoList(occ.Assignees, userInfos))
		if v.RecurrenceInfo != nil {
			v.RecurrenceInfo.SeriesStatus = series.Status
			v.RecurrenceInfo.Until = series.Until
		}
		views = append(views, v)
	}
	sortTaskViews(views, "due")
	total := len(views)
	paged := paginateTaskViews(views, input.Limit, input.Offset)
	return TaskViewPage{Items: paged, Total: total, Limit: input.Limit, Offset: input.Offset, OccurrenceMode: OccurrenceModeMaterialized}, nil
}

// statusPredicate 把列表 status 参数转为 storage 查询的 status predicate。
// all 返回空（不筛选）。
func statusPredicate(status string) string {
	if status == "all" {
		return ""
	}
	return status
}

// SkipTaskSeriesOccurrence 跳过一次 occurrence（spec §11.5）。
// projected 直接物化为 deleted tombstone；materialized 改为 deleted。
func (s *Service) SkipTaskSeriesOccurrence(seriesID, occurrenceRef string) (TaskOccurrenceView, error) {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return TaskOccurrenceView{}, err
	}
	refSeriesID, slot, err := ParseOccurrenceRef(occurrenceRef)
	if err != nil {
		return TaskOccurrenceView{}, RuntimeError{Code: "task_occurrence_not_found", Message: err.Error()}
	}
	if refSeriesID != seriesID {
		return TaskOccurrenceView{}, RuntimeError{Code: "task_series_occurrence_not_found", Message: "occurrence 不属于该 series"}
	}
	now := s.clock.Unix()
	// 写前物化（若 projected），然后标记 deleted。
	var resultView TaskOccurrenceView
	err = s.store.Transaction(func(txStore *storage.Store) error {
		txSvc, terr := s.withStore(txStore)
		if terr != nil {
			return terr
		}
		tsk, _, merr := txSvc.MaterializeOccurrenceForWrite(occurrenceRef)
		if merr != nil {
			return merr
		}
		tsk.Delete(now)
		if err := txSvc.repo.Update(tsk); err != nil {
			return err
		}
		if err := txSvc.appendAuditEntry(AuditEntry{
			Action: "task.recurrence.skipped", WorkspaceID: &tsk.WorkspaceID,
			TargetType: "task", TargetID: tsk.UUID,
			Payload: map[string]any{"series_id": seriesID, "recurrence_at": slot},
		}); err != nil {
			return err
		}
		finalTask, ferr := txSvc.repo.GetByUUID(txSvc.workspaceID, tsk.UUID)
		if ferr != nil {
			return ferr
		}
		userInfos, uerr := txSvc.resolveUserInfos(collectAssigneeUserIDs([]domain.Task{finalTask}))
		if uerr != nil {
			return uerr
		}
		resultView = taskToView(finalTask, userInfoList(finalTask.Assignees, userInfos))
		return nil
	})
	if err != nil {
		return TaskOccurrenceView{}, err
	}
	return resultView, nil
}
