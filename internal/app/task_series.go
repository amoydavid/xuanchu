package app

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/storage"
	domain "git.dajee.net/dajee/xuanchu/internal/task"
	"git.dajee.net/dajee/xuanchu/internal/taskseries"
	"github.com/google/uuid"
)

const (
	taskSeriesDefaultLimit = 200
	taskSeriesMaxLimit     = 1000
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
	URL                        string
	OpenOccurrenceCount        int
	CompletedCount             int
	SkippedCount               int
	OverdueCount               int
	NextRecurrenceAt           *int64
	SuggestedRuleEffectiveFrom *int64
	CreatedBy                  domain.UserInfo
	Assignees                  []domain.UserInfo
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
	Title            *string
	Description      *string
	ClearDescription bool
	Priority         *string
	ClearPriority    bool
	Assignees        []string
	ClearAssignees   bool
	Tags             []string
	ClearTags        bool
	UDAs             map[string]string
	ClearUDAs        []string
	RecurrenceRule   *string
	EffectiveFrom    *int64 // 修改 recurrence_rule 时必填
	Until            *int64
	ClearUntil       bool
}

// ApplyTaskSeriesClearFields 把各协议共用的 clear 字段名映射为 App 输入，
// 避免 HTTP、MCP、Remote CLI 对“清空”产生不同语义。
func ApplyTaskSeriesClearFields(input *ModifyTaskSeriesInput, fields []string) error {
	for _, field := range fields {
		switch field {
		case "description":
			input.ClearDescription = true
		case "priority":
			input.ClearPriority = true
		case "assignees":
			input.ClearAssignees = true
		case "tags":
			input.ClearTags = true
		case "until":
			input.ClearUntil = true
		default:
			if strings.HasPrefix(field, "uda.") && strings.TrimPrefix(field, "uda.") != "" {
				input.ClearUDAs = append(input.ClearUDAs, strings.TrimPrefix(field, "uda."))
				continue
			}
			return RuntimeError{Code: "task_series_invalid_clear", Message: fmt.Sprintf("unsupported clear field %q", field)}
		}
	}
	return nil
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
	Series          TaskSeriesView
	OpenOccurrences []TaskOccurrenceView
	RecentCompleted []TaskOccurrenceView
	RecentSkipped   []TaskOccurrenceView
}

// AddTaskSeries 创建循环系列（spec §11.2）。
//
// ensureSeriesProjectScope 校验当前 token 的 project allowlist 是否允许访问 series 所属项目。
// write=true 时用 project_scope_denied，否则用 task_not_found（不泄露存在性）。
func (s *Service) ensureSeriesProjectScope(series taskseries.Series, write bool) error {
	pid := series.ProjectID
	if write {
		return s.ensureProjectScope(&pid)
	}
	if !s.allowsProjectID(&pid) {
		return RuntimeError{Code: "task_series_not_found", Message: "series not found"}
	}
	return nil
}

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
	if _, err := s.validateDescriptionReferences(nil, input.Description, false); err != nil {
		return TaskSeriesCreateResult{}, err
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
	// 校验 project 属于当前 workspace（防止跨 workspace 创建）。
	if project.WorkspaceID != s.workspaceID {
		return TaskSeriesCreateResult{}, RuntimeError{Code: "project_not_found", Message: "project not found"}
	}
	// 校验项目 scope（token allowlist）。
	if err := s.ensureProjectScope(&projectID); err != nil {
		return TaskSeriesCreateResult{}, err
	}
	if isProjectClosed(project) {
		return TaskSeriesCreateResult{}, RuntimeError{Code: "task_series_project_closed", Message: "项目已关闭"}
	}

	// 解析 assignees 为 user IDs。
	assigneeIDs, err := s.resolveAssigneeRefs(input.Assignees)
	if err != nil {
		return TaskSeriesCreateResult{}, err
	}
	if _, err := s.normalizeUDAModifications(nil, input.UDAs, nil, false); err != nil {
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

	var result TaskSeriesCreateResult
	err = s.store.Transaction(func(txStore *storage.Store) error {
		txSvc, terr := s.withStore(txStore)
		if terr != nil {
			return terr
		}
		// 分配 series 在所属 project 内的自增序号（series_slug 派生用）。
		seq, serr := txSvc.projectRepo.AllocateProjectSeriesSeqLocked(s.workspaceID, projectID)
		if serr != nil {
			return serr
		}
		series.ProjectSeq = &seq
		series.ProjectSlug = project.Slug
		created, cerr := txSvc.taskSeriesRepo.Create(series)
		if cerr != nil {
			return cerr
		}
		// 写 audit。
		if aerr := txSvc.appendAuditEntry(AuditEntry{
			Action: "task.series.created", WorkspaceID: &txSvc.workspaceID, ProjectID: &projectID,
			TargetType: "task_series", TargetID: created.ID,
			Payload: map[string]any{
				"title": created.Title, "recurrence_rule": created.RecurrenceRule,
				"first_due": created.FirstDue,
			},
		}); aerr != nil {
			return aerr
		}
		// 判断 first_due 是否进入执行期。
		var firstOcc *TaskOccurrenceView
		availableAt := startOfDayUnix(input.FirstDue, txSvc.clock.Location())
		if availableAt <= now {
			view, merr := txSvc.materializeFirstOccurrence(created)
			if merr != nil {
				return merr
			}
			firstOcc = &view
		} else {
			slot := taskseries.Slot{RecurrenceAt: input.FirstDue, Rule: created.RecurrenceRule}
			userInfos, uerr := txSvc.resolveUserInfos(created.AssigneeIDs)
			if uerr != nil {
				return uerr
			}
			view := projectedOccurrenceView(txSvc.resourceBaseURL, txSvc.runtime.WorkspaceSlug, created, slot, seriesUserInfoList(created, userInfos))
			firstOcc = &view
		}
		seriesView, verr := txSvc.buildSeriesView(created)
		if verr != nil {
			return verr
		}
		result = TaskSeriesCreateResult{Series: seriesView, FirstOccurrence: firstOcc}
		return nil
	})
	if err != nil {
		return TaskSeriesCreateResult{}, err
	}
	return result, nil
}

// materializeFirstOccurrence 物化 series 的 first 槽位。
func (s *Service) materializeFirstOccurrence(series taskseries.Series) (TaskOccurrenceView, error) {
	now := s.clock.Unix()
	udas, err := s.normalizeUDAModifications(nil, series.UDAs, nil, false)
	if err != nil {
		return TaskOccurrenceView{}, err
	}
	occ := domain.Task{
		UUID: newUUID(), WorkspaceID: series.WorkspaceID, Title: series.Title,
		Description: series.Description, Status: domain.StatusPending,
		Entry: now, Modified: now, Priority: series.Priority, Tags: series.Tags,
		UDAs:     udas,
		SeriesID: &series.ID, RecurrenceAt: &series.FirstDue,
		RecurrenceRuleSnapshot: &series.RecurrenceRule, RecurrenceOverrides: []string{},
	}
	if err := s.bindOccurrenceProject(&occ, series.WorkspaceID, series.ProjectID); err != nil {
		return TaskOccurrenceView{}, err
	}
	due := series.FirstDue
	occ.Due = &due
	// 负责人必须在创建前写入，确保首个已物化实例与后续实例使用相同的
	// task_assignees 持久化路径。
	for _, id := range series.AssigneeIDs {
		occ.Assignees = append(occ.Assignees, domain.AssigneeInfo{UserID: id})
	}
	created, _, err := s.taskOccurrenceRepo.CreateOccurrence(occ)
	if err != nil {
		return TaskOccurrenceView{}, err
	}
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
	view := taskToView(s.resourceBaseURL, s.runtime.WorkspaceSlug, created, userInfoList(created.Assignees, userInfos))
	return view, nil
}

// ListTaskSeries 列出 series（spec §11.3）。
func (s *Service) ListTaskSeries(input TaskSeriesListInput) (TaskSeriesPage, error) {
	if err := s.Require(PermissionTaskRead); err != nil {
		return TaskSeriesPage{}, err
	}
	limit, offset, err := normalizeTaskSeriesPagination(input.Limit, input.Offset)
	if err != nil {
		return TaskSeriesPage{}, err
	}
	input.Limit, input.Offset = limit, offset
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
	// 过滤 project scope（token allowlist）。
	scoped := candidates
	if s.hasProjectScope() {
		scoped = scoped[:0]
		for _, c := range candidates {
			if s.allowsProjectID(&c.ProjectID) {
				scoped = append(scoped, c)
			}
		}
	}
	seriesIDs := make([]string, 0, len(scoped))
	userIDs := make([]string, 0, len(scoped)*2)
	for _, candidate := range scoped {
		seriesIDs = append(seriesIDs, candidate.ID)
		userIDs = append(userIDs, candidate.CreatedBy)
		userIDs = append(userIDs, candidate.AssigneeIDs...)
	}
	summaries, err := s.taskOccurrenceRepo.SummarizeSeriesOccurrences(s.workspaceID, seriesIDs, s.clock.Unix())
	if err != nil {
		return TaskSeriesPage{}, err
	}
	userInfos, err := s.resolveUserInfos(userIDs)
	if err != nil {
		return TaskSeriesPage{}, err
	}
	views := make([]TaskSeriesView, 0, len(scoped))
	for _, c := range scoped {
		view, err := s.buildSeriesViewFromSummary(c, summaries[c.ID], userInfos)
		if err != nil {
			return TaskSeriesPage{}, err
		}
		views = append(views, view)
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
	series, err := s.resolveSeriesRef(seriesID)
	if err != nil {
		return TaskSeriesDetailView{}, mapSeriesError(err)
	}
	if err := s.ensureSeriesProjectScope(series, false); err != nil {
		return TaskSeriesDetailView{}, err
	}
	view, err := s.buildSeriesView(series)
	if err != nil {
		return TaskSeriesDetailView{}, err
	}
	pending, err := s.listTaskSeriesOccurrences(seriesID, TaskSeriesOccurrenceListInput{Status: "pending", Limit: 200}, false)
	if err != nil {
		return TaskSeriesDetailView{}, err
	}
	waiting, err := s.listTaskSeriesOccurrences(seriesID, TaskSeriesOccurrenceListInput{Status: "waiting", Limit: 200}, false)
	if err != nil {
		return TaskSeriesDetailView{}, err
	}
	completed, err := s.listTaskSeriesOccurrences(seriesID, TaskSeriesOccurrenceListInput{Status: "completed", Limit: 10}, true)
	if err != nil {
		return TaskSeriesDetailView{}, err
	}
	skipped, err := s.listTaskSeriesOccurrences(seriesID, TaskSeriesOccurrenceListInput{Status: "deleted", Limit: 10}, true)
	if err != nil {
		return TaskSeriesDetailView{}, err
	}
	open := append(pending.Items, waiting.Items...)
	sortTaskViews(open, "due")
	if len(open) > 200 {
		open = open[:200]
	}
	return TaskSeriesDetailView{
		Series: view, OpenOccurrences: open,
		RecentCompleted: completed.Items, RecentSkipped: skipped.Items,
	}, nil
}

// ModifyTaskSeries 修改 series（spec §11.4）。
//
// 执行顺序：
// 1. 先 reconcile 已进入执行期的 backlog；若仍有 backlog 返回 task_recurrence_backlog。
// 2. 共享字段更新 + 同步未 override 的 open materialized occurrence（不含负责人）。
// 3. 规则修改：追加 RuleVersion，更新 series 当前规则。
// 4. until 修改：只影响未来槽位。
func (s *Service) ModifyTaskSeries(seriesID string, input ModifyTaskSeriesInput) (TaskSeriesView, error) {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return TaskSeriesView{}, err
	}
	now := s.clock.Unix()
	series, err := s.resolveSeriesRef(seriesID)
	if err != nil {
		return TaskSeriesView{}, mapSeriesError(err)
	}
	if err := s.ensureSeriesProjectScope(series, true); err != nil {
		return TaskSeriesView{}, err
	}
	if series.Status != taskseries.StatusActive {
		return TaskSeriesView{}, RuntimeError{Code: "task_series_inactive", Message: "只能修改 active series"}
	}
	beforeDescription := series.Description
	if input.Assignees != nil {
		resolved, err := s.resolveAssigneeRefs(input.Assignees)
		if err != nil {
			return TaskSeriesView{}, err
		}
		input.Assignees = extractAssigneeIDs(resolved)
	}
	if input.UDAs != nil || len(input.ClearUDAs) > 0 {
		normalized, err := s.normalizeUDAModifications(nil, input.UDAs, input.ClearUDAs, false)
		if err != nil {
			return TaskSeriesView{}, err
		}
		if input.UDAs != nil {
			input.UDAs = make(map[string]string, len(normalized))
			for name, value := range normalized {
				input.UDAs[name] = value.Raw
			}
		}
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
		if latestRuleStart := latestRuleVersionEffectiveFrom(series); eff <= latestRuleStart {
			return TaskSeriesView{}, RuntimeError{Code: "task_series_invalid_effective_from", Message: "effective_from 必须晚于当前规则段起点"}
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
		return TaskSeriesView{}, RuntimeError{Code: "task_series_invalid", Message: err.Error()}
	}
	if _, err := s.validateDescriptionReferences(beforeDescription, series.Description, false); err != nil {
		return TaskSeriesView{}, err
	}
	var result TaskSeriesView
	err = s.store.Transaction(func(txStore *storage.Store) error {
		txSvc, err := s.withStore(txStore)
		if err != nil {
			return err
		}
		if err := txSvc.taskSeriesRepo.Update(series); err != nil {
			return err
		}
		if input.RecurrenceRule != nil && input.EffectiveFrom != nil {
			if err := txSvc.taskSeriesRepo.AppendRuleVersion(seriesID, taskseries.RuleVersion{
				EffectiveFrom: *input.EffectiveFrom, RecurrenceRule: *input.RecurrenceRule,
				CreatedBy: txSvc.runtime.ActorUserID, CreatedAt: now,
			}); err != nil {
				return err
			}
		}
		if err := txSvc.syncSharedFieldsToOpenOccurrences(series, input); err != nil {
			return err
		}
		if err := txSvc.appendAuditEntry(AuditEntry{
			Action: "task.series.modified", WorkspaceID: &series.WorkspaceID,
			ProjectID: &series.ProjectID, TargetType: "task_series", TargetID: series.ID,
			Payload: map[string]any{
				"rule_changed": input.RecurrenceRule != nil, "effective_from": input.EffectiveFrom,
				"shared_fields": input.Title != nil || input.Description != nil || input.ClearDescription ||
					input.Priority != nil || input.ClearPriority || input.Assignees != nil || input.ClearAssignees ||
					input.Tags != nil || input.ClearTags || input.UDAs != nil || len(input.ClearUDAs) > 0 ||
					input.Until != nil || input.ClearUntil,
			},
		}); err != nil {
			return err
		}
		updated, err := txSvc.taskSeriesRepo.Get(txSvc.workspaceID, seriesID)
		if err != nil {
			return err
		}
		result, err = txSvc.buildSeriesView(updated)
		return err
	})
	if err != nil {
		return TaskSeriesView{}, err
	}
	return result, nil
}

// maxMaterializedRecurrenceAt 返回 series 已物化的最大 recurrence_at。
func (s *Service) maxMaterializedRecurrenceAt(series taskseries.Series) (int64, error) {
	summaries, err := s.taskOccurrenceRepo.SummarizeSeriesOccurrences(
		series.WorkspaceID, []string{series.ID}, s.clock.Unix(),
	)
	if err != nil {
		return 0, err
	}
	return summaries[series.ID].MaxRecurrenceAt, nil
}

// syncSharedFieldsToOpenOccurrences 把可同步共享字段同步到未 override 的 open materialized occurrence（spec §11.4）。
// 只更新 pending/waiting 且对应字段未在 RecurrenceOverrides 中的 occurrence；负责人只作为
// projected/后续物化实例的默认值，绝不回写已物化实例。
func (s *Service) syncSharedFieldsToOpenOccurrences(series taskseries.Series, input ModifyTaskSeriesInput) error {
	exceptions, err := s.taskOccurrenceRepo.ListOccurrenceExceptions(storage.OccurrenceRangeOptions{
		WorkspaceID: series.WorkspaceID, SeriesID: series.ID,
		Start: series.FirstDue, End: math.MaxInt64,
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
		if (input.UDAs != nil || len(input.ClearUDAs) > 0) && !overridden["udas"] {
			udas, err := s.normalizeUDAModifications(occ.UDAs, input.UDAs, input.ClearUDAs, false)
			if err != nil {
				return err
			}
			occ.UDAs = udas
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

func (s *Service) buildSeriesView(series taskseries.Series) (TaskSeriesView, error) {
	summaries, err := s.taskOccurrenceRepo.SummarizeSeriesOccurrences(series.WorkspaceID, []string{series.ID}, s.clock.Unix())
	if err != nil {
		return TaskSeriesView{}, err
	}
	userIDs := append([]string{series.CreatedBy}, series.AssigneeIDs...)
	info, err := s.resolveUserInfos(userIDs)
	if err != nil {
		return TaskSeriesView{}, err
	}
	return s.buildSeriesViewFromSummary(series, summaries[series.ID], info)
}

func (s *Service) buildSeriesViewFromSummary(series taskseries.Series, summary storage.SeriesOccurrenceSummary, userInfos map[string]domain.UserInfo) (TaskSeriesView, error) {
	createdBy := domain.UserInfo{ID: series.CreatedBy, Name: series.CreatedBy}
	if user, ok := userInfos[series.CreatedBy]; ok {
		createdBy = user
	}
	// 解析 assignees 为完整 UserInfo（含 display_name/email/external_ids）。
	assigneeInfos := make([]domain.UserInfo, 0, len(series.AssigneeIDs))
	for _, id := range series.AssigneeIDs {
		if user, ok := userInfos[id]; ok {
			assigneeInfos = append(assigneeInfos, user)
		} else {
			assigneeInfos = append(assigneeInfos, domain.UserInfo{ID: id, Name: id})
		}
	}
	next := computeNextRecurrenceAt(series, s.clock)
	suggested, err := s.computeSuggestedRuleEffectiveFromWithMax(series, summary.MaxRecurrenceAt)
	if err != nil {
		return TaskSeriesView{}, err
	}
	seriesSlug := SeriesSlugOf(series)
	if strings.TrimSpace(series.ProjectSlug) == "" || strings.TrimSpace(seriesSlug) == "" {
		return TaskSeriesView{}, RuntimeError{Code: "task_series_invalid", Message: "series project slug is unavailable"}
	}
	return TaskSeriesView{
		Series:                     series,
		URL:                        TaskSeriesURL(s.resourceBaseURL, s.runtime.WorkspaceSlug, series.ProjectSlug, seriesSlug),
		OpenOccurrenceCount:        summary.Counts.Open,
		CompletedCount:             summary.Counts.Completed,
		SkippedCount:               summary.Counts.Deleted,
		OverdueCount:               summary.Counts.Overdue,
		NextRecurrenceAt:           next,
		SuggestedRuleEffectiveFrom: suggested,
		CreatedBy:                  createdBy,
		Assignees:                  assigneeInfos,
	}, nil
}

// SeriesSlugOf 由 series 的 project slug + project_seq 派生短引用（如 ops-s-1）。
// project slug 或 seq 缺失时返回空字符串。
func SeriesSlugOf(series taskseries.Series) string {
	if series.ProjectSlug == "" || series.ProjectSeq == nil {
		return ""
	}
	return fmt.Sprintf("%s-s-%d", series.ProjectSlug, *series.ProjectSeq)
}

// computeSuggestedRuleEffectiveFrom 返回满足规则修改约束的默认切换槽位。
// 它严格晚于当前时间和所有已物化槽位，并且只从现有规则版本产生的合法槽位中选择。
func (s *Service) computeSuggestedRuleEffectiveFrom(series taskseries.Series) (*int64, error) {
	if series.Status != taskseries.StatusActive {
		return nil, nil
	}
	maxMaterialized, err := s.maxMaterializedRecurrenceAt(series)
	if err != nil {
		return nil, err
	}
	return s.computeSuggestedRuleEffectiveFromWithMax(series, maxMaterialized)
}

func (s *Service) computeSuggestedRuleEffectiveFromWithMax(series taskseries.Series, maxMaterialized int64) (*int64, error) {
	if series.Status != taskseries.StatusActive {
		return nil, nil
	}
	threshold := s.clock.Unix()
	if maxMaterialized > threshold {
		threshold = maxMaterialized
	}
	if latestRuleStart := latestRuleVersionEffectiveFrom(series); latestRuleStart > threshold {
		threshold = latestRuleStart
	}
	slot, err := taskseries.FirstSlotAfter(ruleVersionsForExpand(series), series.Until, threshold, s.clock.Location())
	if err != nil {
		return nil, err
	}
	if slot == nil || !slotInRangeForSeriesStatus(series, slot.RecurrenceAt) {
		return nil, nil
	}
	value := slot.RecurrenceAt
	return &value, nil
}

func latestRuleVersionEffectiveFrom(series taskseries.Series) int64 {
	latest := series.FirstDue
	for _, version := range series.RuleVersions {
		if version.EffectiveFrom > latest {
			latest = version.EffectiveFrom
		}
	}
	return latest
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

// resolveSeriesRef 统一解析 series 引用：先按 UUID 查，找不到再按 series_slug（{projectSlug}-s-{seq}）解析。
func (s *Service) resolveSeriesRef(target string) (taskseries.Series, error) {
	series, err := s.taskSeriesRepo.Get(s.workspaceID, target)
	if err == nil {
		return series, nil
	}
	if !errors.Is(err, storage.ErrSeriesNotFound) {
		return taskseries.Series{}, err
	}
	// series_slug 解析：{projectSlug}-s-{seq}。project slug 只含 [a-z0-9]，不含 -，所以 -s- 是唯一分隔。
	idx := strings.Index(target, "-s-")
	if idx <= 0 {
		return taskseries.Series{}, storage.ErrSeriesNotFound
	}
	projectSlug := target[:idx]
	seqStr := target[idx+3:]
	seq, perr := strconv.ParseInt(seqStr, 10, 64)
	if perr != nil || seq < 1 {
		return taskseries.Series{}, storage.ErrSeriesNotFound
	}
	project, perr := s.projectRepo.GetBySlug(s.workspaceID, projectSlug)
	if perr != nil {
		return taskseries.Series{}, storage.ErrSeriesNotFound
	}
	return s.taskSeriesRepo.GetByProjectSeq(s.workspaceID, project.ID, seq)
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
	series, err := s.resolveSeriesRef(seriesID)
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
	if endScan <= series.FirstDue {
		return TaskSeriesReconcileResult{}, nil
	}
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

// ReconcileWorkspaceTaskSeries 补齐当前 workspace 所有 active series 的已到期槽位（spec §9.3）。
// 本地 CLI 在任务/项目命令前调用，确保单机模式下 occurrence 及时生成。
func (s *Service) ReconcileWorkspaceTaskSeries(now int64) (TaskSeriesReconcileResult, error) {
	result := TaskSeriesReconcileResult{}
	offset := 0
	for {
		seriesList, err := s.taskSeriesRepo.ListActive(s.workspaceID, 100, offset)
		if err != nil {
			return result, err
		}
		if len(seriesList) == 0 {
			break
		}
		for _, series := range seriesList {
			res, err := s.ReconcileTaskSeries(series.ID, now, 100)
			if err != nil {
				return result, err
			}
			result.Created += res.Created
			result.BacklogRemaining += res.BacklogRemaining
			if res.Ended {
				result.Ended = true
			}
		}
		offset += len(seriesList)
		if len(seriesList) < 100 {
			break
		}
	}
	return result, nil
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
	series, err := s.resolveSeriesRef(seriesID)
	if err != nil {
		return TaskSeriesView{}, mapSeriesError(err)
	}
	if err := s.ensureSeriesProjectScope(series, true); err != nil {
		return TaskSeriesView{}, err
	}
	series.Status = taskseries.StatusStopped
	series.EffectiveEndAt = &now
	series.StopReason = &reason
	series.ModifiedAt = now
	deleteOpen := input.DeleteOpenOccurrences != nil && *input.DeleteOpenOccurrences
	var resultView TaskSeriesView
	err = s.store.Transaction(func(txStore *storage.Store) error {
		txSvc, terr := s.withStore(txStore)
		if terr != nil {
			return terr
		}
		if uerr := txSvc.taskSeriesRepo.Update(series); uerr != nil {
			return uerr
		}
		if aerr := txSvc.appendAuditEntry(AuditEntry{
			Action: "task.series.stopped", WorkspaceID: &series.WorkspaceID,
			ProjectID: &series.ProjectID, TargetType: "task_series", TargetID: series.ID,
			Payload: map[string]any{"reason": reason, "delete_open": deleteOpen},
		}); aerr != nil {
			return aerr
		}
		if deleteOpen {
			if derr := txSvc.deleteOpenOccurrencesForStop(series, now); derr != nil {
				return derr
			}
		}
		resultView, terr = txSvc.buildSeriesView(series)
		return terr
	})
	if err != nil {
		return TaskSeriesView{}, err
	}
	return resultView, nil
}

// deleteOpenOccurrencesForStop 在停止时把 open materialized occurrence 标 deleted，
// 并为已进入执行期但未物化的 projected 槽位创建 tombstone（spec §11.6）。
func (s *Service) deleteOpenOccurrencesForStop(series taskseries.Series, now int64) error {
	const maxDeleteOpenOccurrences = 1000
	affected := 0
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
			affected++
			if affected > maxDeleteOpenOccurrences {
				return RuntimeError{Code: "task_series_delete_open_limit", Message: "待删除实例超过 1000 条，请缩短范围后重试"}
			}
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
		affected++
		if affected > maxDeleteOpenOccurrences {
			return RuntimeError{Code: "task_series_delete_open_limit", Message: "待删除实例超过 1000 条，请缩短范围后重试"}
		}
		// 创建 tombstone。
		rule := slot.Rule
		tombstone := domain.Task{
			UUID: uuid.NewString(), WorkspaceID: series.WorkspaceID, Title: series.Title,
			Status: domain.StatusDeleted, Entry: now, Modified: now,
			End:      &now,
			SeriesID: &series.ID, RecurrenceAt: &slot.RecurrenceAt,
			RecurrenceRuleSnapshot: &rule, RecurrenceOverrides: []string{},
		}
		if err := s.bindOccurrenceProject(&tombstone, series.WorkspaceID, series.ProjectID); err != nil {
			return err
		}
		if _, _, err := s.taskOccurrenceRepo.CreateOccurrence(tombstone); err != nil {
			return err
		}
	}
	return nil
}

// TaskSeriesOccurrenceListInput 是 series occurrence 分页查询参数（spec §11.3）。
type TaskSeriesOccurrenceListInput struct {
	Status    string // pending|waiting|completed|deleted|all
	DueAfter  *int64 // inclusive range start
	DueBefore *int64 // exclusive range end
	Limit     int
	Offset    int
}

// ListTaskSeriesOccurrences 分页列出 series 的 materialized occurrence（spec §11.3）。
func (s *Service) ListTaskSeriesOccurrences(seriesID string, input TaskSeriesOccurrenceListInput) (TaskViewPage, error) {
	return s.listTaskSeriesOccurrences(seriesID, input, false)
}

func (s *Service) listTaskSeriesOccurrences(seriesID string, input TaskSeriesOccurrenceListInput, newestFirst bool) (TaskViewPage, error) {
	if err := s.Require(PermissionTaskRead); err != nil {
		return TaskViewPage{}, err
	}
	limit, offset, err := normalizeTaskSeriesPagination(input.Limit, input.Offset)
	if err != nil {
		return TaskViewPage{}, err
	}
	input.Limit, input.Offset = limit, offset
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
	series, err := s.resolveSeriesRef(seriesID)
	if err != nil {
		return TaskViewPage{}, mapSeriesError(err)
	}
	if err := s.ensureSeriesProjectScope(series, false); err != nil {
		return TaskViewPage{}, err
	}
	// 无范围时返回全部已物化历史；不用“当前时间 + N 年”的隐式截断。
	start := int64(0)
	end := int64(1<<63 - 1)
	if input.DueAfter != nil {
		start = *input.DueAfter
	}
	if input.DueBefore != nil {
		end = *input.DueBefore
	}
	storageOpts := storage.OccurrenceRangeOptions{
		WorkspaceID: series.WorkspaceID, SeriesID: series.ID, Start: start, End: end,
		Status: statusPredicate(status),
		Limit:  input.Limit, Offset: input.Offset, Descending: newestFirst,
	}
	total, err := s.taskOccurrenceRepo.CountOccurrenceExceptions(storageOpts)
	if err != nil {
		return TaskViewPage{}, err
	}
	exceptions, err := s.taskOccurrenceRepo.ListOccurrenceExceptions(storageOpts)
	if err != nil {
		return TaskViewPage{}, err
	}
	userInfos, err := s.resolveUserInfos(collectAssigneeUserIDs(exceptions))
	if err != nil {
		return TaskViewPage{}, err
	}
	views := make([]TaskOccurrenceView, 0, len(exceptions))
	for _, occ := range exceptions {
		v := taskToView(s.resourceBaseURL, s.runtime.WorkspaceSlug, occ, userInfoList(occ.Assignees, userInfos))
		if v.RecurrenceInfo != nil {
			v.RecurrenceInfo.SeriesTitle = series.Title
			v.RecurrenceInfo.SeriesStatus = series.Status
			v.RecurrenceInfo.Until = series.Until
		}
		views = append(views, v)
	}
	sortTaskViews(views, "due")
	if newestFirst {
		for left, right := 0, len(views)-1; left < right; left, right = left+1, right-1 {
			views[left], views[right] = views[right], views[left]
		}
	}
	return TaskViewPage{Items: views, Total: total, Limit: input.Limit, Offset: input.Offset, OccurrenceMode: OccurrenceModeMaterialized}, nil
}

func normalizeTaskSeriesPagination(limit, offset int) (int, int, error) {
	if limit == 0 {
		limit = taskSeriesDefaultLimit
	}
	if limit < 0 || limit > taskSeriesMaxLimit {
		return 0, 0, RuntimeError{Code: "api_bad_limit", Message: "limit must be between 1 and 1000"}
	}
	if offset < 0 {
		return 0, 0, RuntimeError{Code: "api_bad_offset", Message: "offset must be >= 0"}
	}
	return limit, offset, nil
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
	series, err := s.resolveSeriesRef(seriesID)
	if err != nil {
		return TaskOccurrenceView{}, mapSeriesError(err)
	}
	if err := s.ensureSeriesProjectScope(series, true); err != nil {
		return TaskOccurrenceView{}, err
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
		resultView = taskToView(txSvc.resourceBaseURL, txSvc.runtime.WorkspaceSlug, finalTask, userInfoList(finalTask.Assignees, userInfos))
		return nil
	})
	if err != nil {
		return TaskOccurrenceView{}, err
	}
	return resultView, nil
}
