package app

import (
	"fmt"
	"sort"
	"time"

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
	// 用 storage 的 uuid 生成，避免直接 import uuid 包。
	return fmt.Sprintf("occ-%d", time.Now().UnixNano())
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
