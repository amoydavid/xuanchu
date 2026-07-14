package storage

import (
	"errors"
	"fmt"

	domain "git.dajee.net/dajee/xuanchu/internal/task"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ProjectStatus string

const (
	ProjectStatusPlanning  ProjectStatus = "planning"
	ProjectStatusActive    ProjectStatus = "active"
	ProjectStatusArchived  ProjectStatus = "archived"
	ProjectStatusCancelled ProjectStatus = "cancelled"
)

func IsValidProjectStatus(status string) bool {
	switch ProjectStatus(status) {
	case ProjectStatusPlanning, ProjectStatusActive, ProjectStatusArchived, ProjectStatusCancelled:
		return true
	}
	return false
}

// IsProjectClosedStatus 判断状态是否为关闭态（archived/cancelled）。
// 约定：cancelled 不设 ArchivedAt，仅靠 status 判别；archived 必设 ArchivedAt。
// 所有写路径（UpdateStatus/Archive）必须保持 status 与 ArchivedAt 的一致性。
func IsProjectClosedStatus(status string) bool {
	return status == string(ProjectStatusArchived) || status == string(ProjectStatusCancelled)
}

var ErrAlreadyArchived = errors.New("project already archived")

type ProjectRepository struct {
	db *gorm.DB
}

func NewProjectRepository(db *gorm.DB) *ProjectRepository {
	return &ProjectRepository{db: db}
}

func (r *ProjectRepository) Create(project Project) (Project, error) {
	if project.NextTaskSeq <= 0 {
		project.NextTaskSeq = 1
	}
	if err := r.db.Create(&project).Error; err != nil {
		return Project{}, err
	}
	return project, nil
}

func (r *ProjectRepository) AllocateProjectTaskSeqLocked(workspaceID, projectID string) (int64, error) {
	var project Project
	query := r.db.Where("workspace_id = ? AND id = ?", workspaceID, projectID)
	if r.db.Dialector.Name() == "postgres" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := query.First(&project).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	seq := project.NextTaskSeq
	if seq <= 0 {
		seq = 1
	}
	result := r.db.Model(&Project{}).
		Where("workspace_id = ? AND id = ?", workspaceID, projectID).
		Update("next_task_seq", seq+1)
	if result.Error != nil {
		return 0, result.Error
	}
	if result.RowsAffected == 0 {
		return 0, ErrNotFound
	}
	return seq, nil
}

// AllocateProjectSeriesSeqLocked 分配 series 在所属 project 内的自增序号（用于 series_slug）。
// 必须在写事务内调用；postgres 用 SELECT ... FOR UPDATE 串行化（同 AllocateProjectTaskSeqLocked）。
func (r *ProjectRepository) AllocateProjectSeriesSeqLocked(workspaceID, projectID string) (int64, error) {
	var project Project
	query := r.db.Where("workspace_id = ? AND id = ?", workspaceID, projectID)
	if r.db.Dialector.Name() == "postgres" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := query.First(&project).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	seq := project.NextSeriesSeq
	if seq <= 0 {
		seq = 1
	}
	result := r.db.Model(&Project{}).
		Where("workspace_id = ? AND id = ?", workspaceID, projectID).
		Update("next_series_seq", seq+1)
	if result.Error != nil {
		return 0, result.Error
	}
	if result.RowsAffected == 0 {
		return 0, ErrNotFound
	}
	return seq, nil
}

func (r *ProjectRepository) List(workspaceID string, includeArchived bool) ([]Project, error) {
	var projects []Project
	query := r.db.Where("workspace_id = ?", workspaceID)
	if !includeArchived {
		query = query.Where("status = ?", string(ProjectStatusActive))
	}
	if err := query.Order("slug ASC").Find(&projects).Error; err != nil {
		return nil, err
	}
	return projects, nil
}

func (r *ProjectRepository) GetByID(id string) (Project, error) {
	return r.find("id = ?", id)
}

func (r *ProjectRepository) GetBySlug(workspaceID, slug string) (Project, error) {
	return r.find("workspace_id = ? AND slug = ?", workspaceID, slug)
}

func (r *ProjectRepository) GetByRef(workspaceID, ref string) (Project, error) {
	return r.ResolveInWorkspace(workspaceID, ref)
}

func (r *ProjectRepository) ResolveInWorkspace(workspaceID, ref string) (Project, error) {
	return r.find("workspace_id = ? AND (id = ? OR slug = ?)", workspaceID, ref, ref)
}

func (r *ProjectRepository) Update(project Project) error {
	nextTaskSeq := project.NextTaskSeq
	if nextTaskSeq <= 0 {
		nextTaskSeq = 1
	}
	nextSeriesSeq := project.NextSeriesSeq
	if nextSeriesSeq <= 0 {
		nextSeriesSeq = 1
	}
	result := r.db.Model(&Project{}).
		Where("id = ?", project.ID).
		Updates(map[string]any{
			"workspace_id":   project.WorkspaceID,
			"slug":           project.Slug,
			"name":           project.Name,
			"description":    project.Description,
			"status":         project.Status,
			"settings_json":  project.SettingsJSON,
			"next_task_seq":  nextTaskSeq,
			"next_series_seq": nextSeriesSeq,
			"created_at":     project.CreatedAt,
			"modified_at":    project.ModifiedAt,
			"archived_at":    project.ArchivedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *ProjectRepository) Archive(workspaceID, id string, now int64) error {
	project, err := r.ResolveInWorkspace(workspaceID, id)
	if err != nil {
		return err
	}
	if IsProjectClosedStatus(project.Status) {
		return ErrAlreadyArchived
	}
	result := r.db.Model(&Project{}).
		Where("workspace_id = ? AND id = ?", workspaceID, id).
		Updates(map[string]any{
			"status":      string(ProjectStatusArchived),
			"archived_at": now,
			"modified_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *ProjectRepository) ListByStatus(workspaceID, statusFilter string) ([]Project, error) {
	var projects []Project
	query := r.db.Where("workspace_id = ?", workspaceID)
	switch statusFilter {
	case "all", "":
	case "open":
		query = query.Where("status IN ?", []string{string(ProjectStatusPlanning), string(ProjectStatusActive)})
	case "planning", "active", "archived", "cancelled":
		query = query.Where("status = ?", statusFilter)
	default:
		return nil, fmt.Errorf("invalid project status filter: %s", statusFilter)
	}
	if err := query.Order("slug ASC").Find(&projects).Error; err != nil {
		return nil, err
	}
	return projects, nil
}

func (r *ProjectRepository) UpdateStatus(workspaceID, projectID, status string, now int64) error {
	if !IsValidProjectStatus(status) {
		return fmt.Errorf("invalid project status: %s", status)
	}
	updates := map[string]any{
		"status":      status,
		"modified_at": now,
	}
	if status == string(ProjectStatusArchived) {
		updates["archived_at"] = now
	} else {
		updates["archived_at"] = nil
	}
	result := r.db.Model(&Project{}).
		Where("workspace_id = ? AND id = ?", workspaceID, projectID).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *ProjectRepository) TaskCounts(workspaceID string, projectIDs []string) (map[string]int, error) {
	counts := make(map[string]int, len(projectIDs))
	if len(projectIDs) == 0 {
		return counts, nil
	}
	for _, id := range projectIDs {
		counts[id] = 0
	}

	type row struct {
		ProjectID string
		Count     int
	}
	var rows []row
	if err := r.db.Model(&Task{}).
		Select("project_id, COUNT(*) AS count").
		Where("workspace_id = ? AND project_id IN ? AND status <> ?", workspaceID, projectIDs, domain.StatusDeleted).
		Group("project_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		counts[row.ProjectID] = row.Count
	}
	return counts, nil
}

// ProjectTaskCounts 是单个项目的任务状态分项计数（不含 deleted）。
type ProjectTaskCounts struct {
	Total     int
	Pending   int
	Completed int
}

// TaskStatusCounts 按 project 分组返回任务状态分项计数（一条 SQL，GROUP BY project_id, status）。
// deleted 状态不计入。projectIDs 中的项目若无任务，对应值为零值。
func (r *ProjectRepository) TaskStatusCounts(workspaceID string, projectIDs []string) (map[string]ProjectTaskCounts, error) {
	out := make(map[string]ProjectTaskCounts, len(projectIDs))
	if len(projectIDs) == 0 {
		return out, nil
	}
	type row struct {
		ProjectID string
		Status    string
		Count     int
	}
	var rows []row
	if err := r.db.Model(&Task{}).
		Select("project_id, status, COUNT(*) AS count").
		Where("workspace_id = ? AND project_id IN ? AND status <> ?", workspaceID, projectIDs, domain.StatusDeleted).
		Group("project_id, status").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		c := out[r.ProjectID]
		c.Total += r.Count
		switch r.Status {
		case domain.StatusPending:
			c.Pending += r.Count
		case domain.StatusCompleted:
			c.Completed += r.Count
		}
		out[r.ProjectID] = c
	}
	return out, nil
}

func (r *ProjectRepository) find(query string, args ...any) (Project, error) {
	var project Project
	err := r.db.Where(query, args...).First(&project).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Project{}, ErrNotFound
	}
	if err != nil {
		return Project{}, err
	}
	return project, nil
}

// ProjectTaskRefSummary 是项目任务摘要中的任务短引用。
// TaskSlug 由 Project + ProjectSeq 在 Go 端拼出（数据库无 task_slug 列）。
type ProjectTaskRefSummary struct {
	UUID     string
	TaskSlug string
	Title    string
}

// ProjectAssigneeWorkloadRow 是按 assignee 聚合的项目负责人负载行。
// UserID == "" 表示未分配任务。显示名相关字段保留原始列，label 由 app 层生成。
type ProjectAssigneeWorkloadRow struct {
	UserID            string
	Name              string
	DisplayName       string
	Email             *string
	OpenCount         int
	OverdueCount      int
	HighPriorityCount int
}

// ProjectTaskSummary 是项目全量任务摘要。所有计数都基于项目全量任务，
// 排除 completed/deleted 状态。Refs 查询只取最多 3 条短引用。
//
// 一次性进度计数（Overdue/HighPriority/Wait/Unassigned/Workload）只统计
// 普通任务（series_id IS NULL），不被每日 occurrence 扭曲（spec §17.4）。
// Series 运行情况单独通过 SeriesMetrics 返回。
type ProjectTaskSummary struct {
	OverdueCount          int
	OverdueRefs           []ProjectTaskRefSummary
	HighPriorityOpenCount int
	HighPriorityOpenRefs  []ProjectTaskRefSummary
	WaitReadyCount        int
	WaitReadyRefs         []ProjectTaskRefSummary
	UnassignedOpenCount   int
	UnassignedOpenRefs    []ProjectTaskRefSummary
	Workload              []ProjectAssigneeWorkloadRow
	SeriesMetrics         ProjectSeriesMetrics
}

// ProjectSeriesMetrics 是项目下循环系列运行情况（spec §17.4）。
// open/overdue occurrence 计数只统计 materialized 行。
type ProjectSeriesMetrics struct {
	RecurringSeriesCount        int // 项目下全部 series 数量
	ActiveRecurringSeriesCount  int // status = active 的 series 数量
	OpenRecurringOccurrenceCount    int // materialized occurrence 中 pending+waiting
	OverdueRecurringOccurrenceCount int // open 且 due < now
}

// TaskSummary 按项目全量任务聚合项目任务摘要。
// now 使用服务端注入时钟，date-only due/until/wait/scheduled 已在写入时按本地日边界落库，
// 这里只用落库后的 Unix 时间比较，不再重新解释浏览器时区。
func (r *ProjectRepository) TaskSummary(workspaceID, projectID string, now int64) (ProjectTaskSummary, error) {
	summary := ProjectTaskSummary{
		OverdueRefs:           []ProjectTaskRefSummary{},
		HighPriorityOpenRefs:  []ProjectTaskRefSummary{},
		WaitReadyRefs:         []ProjectTaskRefSummary{},
		UnassignedOpenRefs:    []ProjectTaskRefSummary{},
		Workload:              []ProjectAssigneeWorkloadRow{},
	}

	// 四类计数 + refs。count 查询返回项目全量普通任务（series_id IS NULL，
	// 排除 occurrence），避免每日实例扭曲一次性进度（spec §17.4）。
	// refs 查询只取最多 3 条。
	openStatusFilter := []string{domain.StatusCompleted, domain.StatusDeleted}
	ordinaryOnly := "series_id IS NULL"

	var overdueCount, highPriorityOpenCount, waitReadyCount, unassignedOpenCount int64

	// overdue count
	if err := r.db.Model(&Task{}).
		Where("workspace_id = ? AND project_id = ?", workspaceID, projectID).
		Where(ordinaryOnly).
		Where("status NOT IN ?", openStatusFilter).
		Where("due IS NOT NULL AND due < ?", now).
		Count(&overdueCount).Error; err != nil {
		return ProjectTaskSummary{}, err
	}
	// high priority open count
	if err := r.db.Model(&Task{}).
		Where("workspace_id = ? AND project_id = ?", workspaceID, projectID).
		Where(ordinaryOnly).
		Where("status NOT IN ?", openStatusFilter).
		Where("priority = ?", "H").
		Count(&highPriorityOpenCount).Error; err != nil {
		return ProjectTaskSummary{}, err
	}
	// wait ready count
	if err := r.db.Model(&Task{}).
		Where("workspace_id = ? AND project_id = ?", workspaceID, projectID).
		Where(ordinaryOnly).
		Where("status NOT IN ?", openStatusFilter).
		Where("wait IS NOT NULL AND wait <= ?", now).
		Count(&waitReadyCount).Error; err != nil {
		return ProjectTaskSummary{}, err
	}
	// unassigned open count
	if err := r.db.Model(&Task{}).
		Where("workspace_id = ? AND project_id = ?", workspaceID, projectID).
		Where(ordinaryOnly).
		Where("status NOT IN ?", openStatusFilter).
		Where("NOT EXISTS (SELECT 1 FROM task_assignees WHERE task_assignees.task_uuid = tasks.uuid)").
		Count(&unassignedOpenCount).Error; err != nil {
		return ProjectTaskSummary{}, err
	}
	summary.OverdueCount = int(overdueCount)
	summary.HighPriorityOpenCount = int(highPriorityOpenCount)
	summary.WaitReadyCount = int(waitReadyCount)
	summary.UnassignedOpenCount = int(unassignedOpenCount)

	// refs（最多 3 条）。顺序以 due/entry 为主，便于前端稳定展示。
	var err error
	summary.OverdueRefs, err = r.taskSummaryRefs(workspaceID, projectID, now, "overdue")
	if err != nil {
		return ProjectTaskSummary{}, err
	}
	summary.HighPriorityOpenRefs, err = r.taskSummaryRefs(workspaceID, projectID, now, "high_priority_open")
	if err != nil {
		return ProjectTaskSummary{}, err
	}
	summary.WaitReadyRefs, err = r.taskSummaryRefs(workspaceID, projectID, now, "wait_ready")
	if err != nil {
		return ProjectTaskSummary{}, err
	}
	summary.UnassignedOpenRefs, err = r.taskSummaryRefs(workspaceID, projectID, now, "unassigned_open")
	if err != nil {
		return ProjectTaskSummary{}, err
	}

	// 负责人负载：assignee 行 + 未分配行
	summary.Workload, err = r.taskSummaryWorkload(workspaceID, projectID, now)
	if err != nil {
		return ProjectTaskSummary{}, err
	}

	// 循环系列运行情况（spec §17.4）
	summary.SeriesMetrics, err = r.projectSeriesMetrics(workspaceID, projectID, now)
	if err != nil {
		return ProjectTaskSummary{}, err
	}

	return summary, nil
}

// projectSeriesMetrics 统计项目下循环系列运行情况（spec §17.4）。
// occurrence 计数只统计 materialized 行（task 表中 series_id 非空的行）。
func (r *ProjectRepository) projectSeriesMetrics(workspaceID, projectID string, now int64) (ProjectSeriesMetrics, error) {
	var metrics ProjectSeriesMetrics

	// 全部 series 数量
	var totalCount int64
	if err := r.db.Model(&TaskSeries{}).
		Where("workspace_id = ? AND project_id = ?", workspaceID, projectID).
		Count(&totalCount).Error; err != nil {
		return metrics, err
	}
	metrics.RecurringSeriesCount = int(totalCount)

	// active series 数量
	var activeCount int64
	if err := r.db.Model(&TaskSeries{}).
		Where("workspace_id = ? AND project_id = ? AND status = ?", workspaceID, projectID, "active").
		Count(&activeCount).Error; err != nil {
		return metrics, err
	}
	metrics.ActiveRecurringSeriesCount = int(activeCount)

	// materialized occurrence：open（pending+waiting）和 overdue
	openStatuses := []string{domain.StatusPending, domain.StatusWaiting}
	var openCount int64
	if err := r.db.Model(&Task{}).
		Where("workspace_id = ? AND project_id = ? AND series_id IS NOT NULL", workspaceID, projectID).
		Where("status IN ?", openStatuses).
		Count(&openCount).Error; err != nil {
		return metrics, err
	}
	metrics.OpenRecurringOccurrenceCount = int(openCount)

	var overdueCount int64
	if err := r.db.Model(&Task{}).
		Where("workspace_id = ? AND project_id = ? AND series_id IS NOT NULL", workspaceID, projectID).
		Where("status IN ?", openStatuses).
		Where("due IS NOT NULL AND due < ?", now).
		Count(&overdueCount).Error; err != nil {
		return metrics, err
	}
	metrics.OverdueRecurringOccurrenceCount = int(overdueCount)

	return metrics, nil
}

// taskSummaryRefs 按类别返回最多 3 条普通任务短引用（series_id IS NULL，spec §17.4）。
func (r *ProjectRepository) taskSummaryRefs(workspaceID, projectID string, now int64, kind string) ([]ProjectTaskRefSummary, error) {
	base := r.db.Model(&Task{}).
		Select("uuid, project, project_seq, title").
		Where("workspace_id = ? AND project_id = ?", workspaceID, projectID).
		Where("series_id IS NULL").
		Where("status NOT IN ?", []string{domain.StatusCompleted, domain.StatusDeleted})
	switch kind {
	case "overdue":
		base = base.Where("due IS NOT NULL AND due < ?", now)
	case "high_priority_open":
		base = base.Where("priority = ?", "H")
	case "wait_ready":
		base = base.Where("wait IS NOT NULL AND wait <= ?", now)
	case "unassigned_open":
		base = base.Where("NOT EXISTS (SELECT 1 FROM task_assignees WHERE task_assignees.task_uuid = tasks.uuid)")
	default:
		return nil, fmt.Errorf("unsupported task summary kind: %s", kind)
	}
	var rows []Task
	if err := base.Order("entry ASC").Limit(3).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ProjectTaskRefSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, ProjectTaskRefSummary{
			UUID:     row.UUID,
			TaskSlug: taskSlugFromModel(row.Project, row.ProjectSeq),
			Title:    row.Title,
		})
	}
	return out, nil
}

// taskSummaryWorkload 按负责人聚合未关闭普通任务（series_id IS NULL）的
// open/overdue/high 计数，并追加一行未分配任务汇总（spec §17.4）。
func (r *ProjectRepository) taskSummaryWorkload(workspaceID, projectID string, now int64) ([]ProjectAssigneeWorkloadRow, error) {
	// assignee 负载：tasks join task_assignees left join users
	type workloadRow struct {
		UserID      string
		Name        string
		DisplayName string
		Email       *string
		OpenCount   int
		Overdue     int
		HighPriority int
	}
	var rows []workloadRow
	openFilter := []string{domain.StatusCompleted, domain.StatusDeleted}
	if err := r.db.Table("tasks").
		Select("users.id AS user_id, users.name AS name, users.display_name AS display_name, users.email AS email, "+
			"COUNT(*) AS open_count, "+
			"SUM(CASE WHEN tasks.due IS NOT NULL AND tasks.due < ? THEN 1 ELSE 0 END) AS overdue, "+
			"SUM(CASE WHEN tasks.priority = ? THEN 1 ELSE 0 END) AS high_priority", now, "H").
		Joins("JOIN task_assignees ON task_assignees.task_uuid = tasks.uuid").
		Joins("LEFT JOIN users ON users.id = task_assignees.user_id").
		Where("tasks.workspace_id = ? AND tasks.project_id = ?", workspaceID, projectID).
		Where("tasks.series_id IS NULL").
		Where("tasks.status NOT IN ?", openFilter).
		Group("users.id, users.name, users.display_name, users.email").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]ProjectAssigneeWorkloadRow, 0, len(rows)+1)
	for _, row := range rows {
		out = append(out, ProjectAssigneeWorkloadRow{
			UserID:            row.UserID,
			Name:              row.Name,
			DisplayName:       row.DisplayName,
			Email:             row.Email,
			OpenCount:         row.OpenCount,
			OverdueCount:      row.Overdue,
			HighPriorityCount: row.HighPriority,
		})
	}

	// 未分配任务：NOT EXISTS assignee（只统计普通任务，spec §17.4）
	var unassigned struct {
		OpenCount    int
		Overdue      int
		HighPriority int
	}
	if err := r.db.Table("tasks").
		Select("COUNT(*) AS open_count, "+
			"SUM(CASE WHEN tasks.due IS NOT NULL AND tasks.due < ? THEN 1 ELSE 0 END) AS overdue, "+
			"SUM(CASE WHEN tasks.priority = ? THEN 1 ELSE 0 END) AS high_priority", now, "H").
		Where("tasks.workspace_id = ? AND tasks.project_id = ?", workspaceID, projectID).
		Where("tasks.series_id IS NULL").
		Where("tasks.status NOT IN ?", openFilter).
		Where("NOT EXISTS (SELECT 1 FROM task_assignees WHERE task_assignees.task_uuid = tasks.uuid)").
		Scan(&unassigned).Error; err != nil {
		return nil, err
	}
	out = append(out, ProjectAssigneeWorkloadRow{
		UserID:            "",
		OpenCount:         unassigned.OpenCount,
		OverdueCount:      unassigned.Overdue,
		HighPriorityCount: unassigned.HighPriority,
	})
	return out, nil
}

// taskSlugFromModel 在 storage 层复刻 task.ToJSON 的 slug 拼接规则：project-projectSeq。
func taskSlugFromModel(project *string, seq *int64) string {
	if project == nil || *project == "" || seq == nil {
		return ""
	}
	return fmt.Sprintf("%s-%d", *project, *seq)
}
