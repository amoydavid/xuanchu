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
	err := r.db.
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
}

// ListOccurrenceExceptions 查询 recurrence_at 或实际 due 命中范围的 materialized occurrence（spec §7.9）。
// rescheduled occurrence 改期后，原槽位和新 due 都应被覆盖。
func (r *TaskOccurrenceRepository) ListOccurrenceExceptions(opts OccurrenceRangeOptions) ([]domain.Task, error) {
	q := r.taskRepo.preloadAssociations().Model(&Task{})
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
	var models []Task
	if err := q.Order("tasks.recurrence_at ASC").Find(&models).Error; err != nil {
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

// OccurrenceCounts 是 series 的 occurrence 计数（spec §7.7）。
type OccurrenceCounts struct {
	Open      int // pending + waiting
	Pending   int
	Waiting   int
	Completed int
	Deleted   int
	Overdue   int // open 且 due < now
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
