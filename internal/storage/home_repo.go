package storage

import "gorm.io/gorm"

// HomeRepository 聚合 Web Console 用户首页需要的 workspace 级数据。
// 它只返回存储事实；权限、排序和用户信息补全由 app.Service 负责。
type HomeRepository struct {
	db *gorm.DB
}

func NewHomeRepository(db *gorm.DB) *HomeRepository {
	return &HomeRepository{db: db}
}

// HomeProjectMetrics 是首页项目关注区使用的集合聚合结果。
// OverdueCount 已包含已物化循环实例；recurrence 字段只是其中的子集。
type HomeProjectMetrics struct {
	OverdueCount                    int
	HighPriorityOpenCount           int
	WaitReadyCount                  int
	UnassignedOpenCount             int
	RecurringSeriesCount            int
	ActiveRecurringSeriesCount      int
	OpenRecurringOccurrenceCount    int
	OverdueRecurringOccurrenceCount int
}

// ProjectMetrics 在一次 GROUP BY 查询中返回目标项目的风险计数，避免逐项目查询。
func (r *HomeRepository) ProjectMetrics(workspaceID string, projectIDs []string, now int64) (map[string]HomeProjectMetrics, error) {
	out := make(map[string]HomeProjectMetrics, len(projectIDs))
	if len(projectIDs) == 0 {
		return out, nil
	}
	type metricRow struct {
		ProjectID                       string
		OverdueCount                    int
		HighPriorityOpenCount           int
		WaitReadyCount                  int
		UnassignedOpenCount             int
		OpenRecurringOccurrenceCount    int
		OverdueRecurringOccurrenceCount int
	}
	var rows []metricRow
	err := r.db.Table("tasks").
		Select(`tasks.project_id AS project_id,
			SUM(CASE WHEN tasks.due IS NOT NULL AND tasks.due < ? THEN 1 ELSE 0 END) AS overdue_count,
			SUM(CASE WHEN tasks.priority = 'H' THEN 1 ELSE 0 END) AS high_priority_open_count,
			SUM(CASE WHEN tasks.wait IS NOT NULL AND tasks.wait <= ? THEN 1 ELSE 0 END) AS wait_ready_count,
			SUM(CASE WHEN NOT EXISTS (SELECT 1 FROM task_assignees WHERE task_assignees.task_uuid = tasks.uuid) THEN 1 ELSE 0 END) AS unassigned_open_count,
			SUM(CASE WHEN tasks.series_id IS NOT NULL THEN 1 ELSE 0 END) AS open_recurring_occurrence_count,
			SUM(CASE WHEN tasks.series_id IS NOT NULL AND tasks.due IS NOT NULL AND tasks.due < ? THEN 1 ELSE 0 END) AS overdue_recurring_occurrence_count`, now, now, now).
		Where("tasks.workspace_id = ? AND tasks.project_id IN ?", workspaceID, projectIDs).
		Where("tasks.status IN ?", []string{"pending", "waiting"}).
		Group("tasks.project_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ProjectID] = HomeProjectMetrics{
			OverdueCount:                    row.OverdueCount,
			HighPriorityOpenCount:           row.HighPriorityOpenCount,
			WaitReadyCount:                  row.WaitReadyCount,
			UnassignedOpenCount:             row.UnassignedOpenCount,
			OpenRecurringOccurrenceCount:    row.OpenRecurringOccurrenceCount,
			OverdueRecurringOccurrenceCount: row.OverdueRecurringOccurrenceCount,
		}
	}
	type seriesRow struct {
		ProjectID                  string
		RecurringSeriesCount       int
		ActiveRecurringSeriesCount int
	}
	var seriesRows []seriesRow
	if err := r.db.Table("task_series").
		Select(`project_id,
			COUNT(*) AS recurring_series_count,
			SUM(CASE WHEN status = 'active' THEN 1 ELSE 0 END) AS active_recurring_series_count`).
		Where("workspace_id = ? AND project_id IN ?", workspaceID, projectIDs).
		Group("project_id").
		Scan(&seriesRows).Error; err != nil {
		return nil, err
	}
	for _, row := range seriesRows {
		metrics := out[row.ProjectID]
		metrics.RecurringSeriesCount = row.RecurringSeriesCount
		metrics.ActiveRecurringSeriesCount = row.ActiveRecurringSeriesCount
		out[row.ProjectID] = metrics
	}
	return out, nil
}

// LatestProjectAnnotations 一次读取多个项目的 annotation，并保留每个项目 entry 最大的一条。
func (r *HomeRepository) LatestProjectAnnotations(projectIDs []string) (map[string]ProjectAnnotation, error) {
	out := make(map[string]ProjectAnnotation, len(projectIDs))
	if len(projectIDs) == 0 {
		return out, nil
	}
	var rows []ProjectAnnotation
	if err := r.db.Where("project_id IN ?", projectIDs).
		Order("project_id ASC").
		Order("entry DESC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		if _, exists := out[row.ProjectID]; !exists {
			out[row.ProjectID] = row
		}
	}
	return out, nil
}
