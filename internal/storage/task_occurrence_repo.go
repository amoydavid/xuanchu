package storage

import (
	"errors"

	domain "git.dajee.net/dajee/xuanchu/internal/task"
	"gorm.io/gorm"
)

// TaskOccurrenceRepository 负责已物化 occurrence 的幂等写入与范围查询（spec §7.2、§9）。
type TaskOccurrenceRepository struct {
	db           *gorm.DB
	taskRepo     *TaskRepository
	taskLinkRepo *TaskLinkRepository
}

// NewTaskOccurrenceRepository 构造 TaskOccurrenceRepository。
func NewTaskOccurrenceRepository(db *gorm.DB) *TaskOccurrenceRepository {
	return &TaskOccurrenceRepository{
		db:           db,
		taskRepo:     NewTaskRepository(db),
		taskLinkRepo: NewTaskLinkRepository(db),
	}
}

// CreateOccurrence 按 (workspace_id, series_id, recurrence_at) 幂等写入 occurrence（spec §7.3、§22）。
// 若该槽位已存在任意状态的 occurrence，返回 (existing, true, nil)，保证永久幂等（completed/deleted 也算已占用）。
// 否则插入新行，返回 (created, false, nil)。
func (r *TaskOccurrenceRepository) CreateOccurrence(tsk domain.Task) (domain.Task, bool, error) {
	if tsk.SeriesID == nil || tsk.RecurrenceAt == nil {
		return domain.Task{}, false, errors.New("occurrence requires series_id and recurrence_at")
	}
	if tsk.WorkspaceID == "" {
		return domain.Task{}, false, errors.New("occurrence requires workspace_id")
	}
	created, err := r.taskRepo.Create(tsk)
	if err == nil {
		return created, false, nil
	}
	if !isUniqueConstraintError(err) {
		return domain.Task{}, false, err
	}
	// 唯一约束冲突：读取已存在的 occurrence（任意状态）。
	existing, findErr := r.GetOccurrence(tsk.WorkspaceID, *tsk.SeriesID, *tsk.RecurrenceAt)
	if findErr != nil {
		return domain.Task{}, false, err
	}
	return existing, true, nil
}

// GetOccurrence 按 (workspace_id, series_id, recurrence_at) 读取已物化 occurrence。
func (r *TaskOccurrenceRepository) GetOccurrence(workspaceID, seriesID string, recurrenceAt int64) (domain.Task, error) {
	var model Task
	err := r.taskRepo.preloadAssociations().
		Where("workspace_id = ? AND series_id = ? AND recurrence_at = ?", workspaceID, seriesID, recurrenceAt).
		First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Task{}, ErrOccurrenceNotFound
		}
		return domain.Task{}, err
	}
	usersByID, err := r.taskRepo.loadAssigneeUsers([]Task{model})
	if err != nil {
		return domain.Task{}, err
	}
	linksByTask, err := r.taskRepo.loadLinksByTask([]Task{model})
	if err != nil {
		return domain.Task{}, err
	}
	return fromModel(model, usersByID, linksByTask), nil
}

// OccurrenceRangeOptions 是范围查询 materialized occurrence 的参数。
type OccurrenceRangeOptions struct {
	WorkspaceID string
	SeriesID    string
	Start       int64 // inclusive
	End         int64 // exclusive
	Status      string
	Limit       int
	Offset      int
	Descending  bool
}

// ListOccurrenceExceptions 查询 recurrence_at 或实际 due 命中范围的 materialized occurrence（spec §7.9）。
// rescheduled occurrence 改期后，原槽位和新 due 都应被覆盖。
func (r *TaskOccurrenceRepository) ListOccurrenceExceptions(opts OccurrenceRangeOptions) ([]domain.Task, error) {
	q := applyOccurrenceRange(r.taskRepo.preloadAssociations().Model(&Task{}), opts)
	order := "COALESCE(tasks.due, 0) ASC, tasks.uuid ASC"
	if opts.Descending {
		order = "COALESCE(tasks.due, 0) DESC, tasks.uuid DESC"
	}
	q = q.Order(order)
	if opts.Offset > 0 {
		q = q.Offset(opts.Offset)
	}
	if opts.Limit > 0 {
		q = q.Limit(opts.Limit)
	}
	var models []Task
	if err := q.Find(&models).Error; err != nil {
		return nil, err
	}
	usersByID, err := r.taskRepo.loadAssigneeUsers(models)
	if err != nil {
		return nil, err
	}
	linksByTask, err := r.taskRepo.loadLinksByTask(models)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Task, 0, len(models))
	for _, m := range models {
		out = append(out, fromModel(m, usersByID, linksByTask))
	}
	return out, nil
}

// CountOccurrenceExceptions 返回与 ListOccurrenceExceptions 相同过滤条件下的完整数量，
// 不受 limit/offset 影响，供协议分页返回准确 total。
func (r *TaskOccurrenceRepository) CountOccurrenceExceptions(opts OccurrenceRangeOptions) (int, error) {
	q := applyOccurrenceRange(r.db.Model(&Task{}), opts)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return 0, err
	}
	return int(total), nil
}

func applyOccurrenceRange(q *gorm.DB, opts OccurrenceRangeOptions) *gorm.DB {
	if opts.WorkspaceID != "" {
		q = q.Where("tasks.workspace_id = ?", opts.WorkspaceID)
	}
	if opts.SeriesID != "" {
		q = q.Where("tasks.series_id = ?", opts.SeriesID)
	}
	q = q.Where("(tasks.recurrence_at >= ? AND tasks.recurrence_at < ?) OR (tasks.due >= ? AND tasks.due < ?)",
		opts.Start, opts.End, opts.Start, opts.End)
	if opts.Status != "" {
		q = q.Where("tasks.status = ?", opts.Status)
	}
	return q
}

// OccurrenceCounts 是 series 的 occurrence 计数（spec §7.7）。
type OccurrenceCounts struct {
	Open      int // pending + waiting
	Pending   int
	Waiting   int
	Completed int
	Deleted   int
	Overdue   int // open 且 due < now
}

// SeriesOccurrenceSummary 是 Series list 所需的批量派生统计。
type SeriesOccurrenceSummary struct {
	Counts          OccurrenceCounts
	MaxRecurrenceAt int64
}

// SummarizeSeriesOccurrences 用一次分组查询返回多个 series 的计数与最大槽位，
// 避免 Series list 按行读取 counts/max 形成 N+1。
func (r *TaskOccurrenceRepository) SummarizeSeriesOccurrences(workspaceID string, seriesIDs []string, now int64) (map[string]SeriesOccurrenceSummary, error) {
	out := make(map[string]SeriesOccurrenceSummary, len(seriesIDs))
	for _, id := range seriesIDs {
		out[id] = SeriesOccurrenceSummary{}
	}
	if len(seriesIDs) == 0 {
		return out, nil
	}
	type row struct {
		SeriesID        string
		Pending         int64
		Waiting         int64
		Completed       int64
		Deleted         int64
		Overdue         int64
		MaxRecurrenceAt *int64
	}
	var rows []row
	err := r.db.Model(&Task{}).
		Select(`series_id,
			SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) AS pending,
			SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) AS waiting,
			SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) AS completed,
			SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) AS deleted,
			SUM(CASE WHEN status IN (?, ?) AND due IS NOT NULL AND due < ? THEN 1 ELSE 0 END) AS overdue,
			MAX(recurrence_at) AS max_recurrence_at`,
			domain.StatusPending, domain.StatusWaiting, domain.StatusCompleted, domain.StatusDeleted,
			domain.StatusPending, domain.StatusWaiting, now).
		Where("workspace_id = ? AND series_id IN ?", workspaceID, seriesIDs).
		Group("series_id").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, item := range rows {
		counts := OccurrenceCounts{
			Pending: int(item.Pending), Waiting: int(item.Waiting),
			Completed: int(item.Completed), Deleted: int(item.Deleted), Overdue: int(item.Overdue),
		}
		counts.Open = counts.Pending + counts.Waiting
		summary := SeriesOccurrenceSummary{Counts: counts}
		if item.MaxRecurrenceAt != nil {
			summary.MaxRecurrenceAt = *item.MaxRecurrenceAt
		}
		out[item.SeriesID] = summary
	}
	return out, nil
}

// CountSeriesOccurrences 统计 series 的 materialized occurrence 数量。
func (r *TaskOccurrenceRepository) CountSeriesOccurrences(workspaceID, seriesID string, now int64) (OccurrenceCounts, error) {
	var counts OccurrenceCounts
	type row struct {
		Status string
		N      int64
	}
	var rows []row
	err := r.db.Model(&Task{}).
		Select("status, count(*) as n").
		Where("workspace_id = ? AND series_id = ?", workspaceID, seriesID).
		Group("status").
		Scan(&rows).Error
	if err != nil {
		return counts, err
	}
	for _, r := range rows {
		switch r.Status {
		case domain.StatusPending:
			counts.Pending = int(r.N)
		case domain.StatusWaiting:
			counts.Waiting = int(r.N)
		case domain.StatusCompleted:
			counts.Completed = int(r.N)
		case domain.StatusDeleted:
			counts.Deleted = int(r.N)
		}
	}
	counts.Open = counts.Pending + counts.Waiting
	// overdue: open 且 due < now
	var overdue int64
	err = r.db.Model(&Task{}).
		Where("workspace_id = ? AND series_id = ? AND status IN ? AND due IS NOT NULL AND due < ?",
			workspaceID, seriesID, []string{domain.StatusPending, domain.StatusWaiting}, now).
		Count(&overdue).Error
	if err != nil {
		return counts, err
	}
	counts.Overdue = int(overdue)
	return counts, nil
}

// ErrOccurrenceNotFound 表示 occurrence 槽位未物化。
var ErrOccurrenceNotFound = errors.New("task occurrence not materialized")
