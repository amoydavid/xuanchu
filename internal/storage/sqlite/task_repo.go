package sqlite

import (
	"errors"
	"sort"

	"github.com/dajee/taskg/internal/query"
	domain "github.com/dajee/taskg/internal/task"
	"gorm.io/gorm"
)

type TaskRepository struct {
	db *gorm.DB
}

type ListOptions struct {
	Status  string
	Sort    string
	Query   query.Expr
	NowUnix int64
}

func NewTaskRepository(db *gorm.DB) *TaskRepository {
	return &TaskRepository{db: db}
}

func (r *TaskRepository) Create(tsk domain.Task) (domain.Task, error) {
	if err := tsk.Validate(); err != nil {
		return domain.Task{}, err
	}
	model := toModel(tsk)
	if err := r.db.Create(&model).Error; err != nil {
		return domain.Task{}, err
	}
	return fromModel(model), nil
}

func (r *TaskRepository) List(workspaceID string, opts ListOptions) ([]domain.Task, error) {
	var models []Task
	q := r.db.Preload("Tags")
	if opts.Status != "" {
		q = q.Where("status = ?", opts.Status)
	}
	if opts.Query != nil {
		q = ApplyQuery(q, opts.Query, QueryCompileOptions{WorkspaceID: workspaceID, NowUnix: opts.NowUnix})
	} else {
		q = q.Where("workspace_id = ?", workspaceID)
	}
	switch opts.Sort {
	case "next":
		q = q.Order("due IS NULL ASC").Order("due ASC").Order("CASE priority WHEN 'H' THEN 3 WHEN 'M' THEN 2 WHEN 'L' THEN 1 ELSE 0 END DESC").Order("entry ASC")
	case "completed":
		q = q.Order("end_ts DESC").Order("modified DESC")
	case "due":
		q = q.Order("due IS NULL ASC").Order("due ASC")
	default:
		q = q.Order("entry ASC")
	}
	if err := q.Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Task, 0, len(models))
	for _, model := range models {
		out = append(out, fromModel(model))
	}
	return out, nil
}

func (r *TaskRepository) GetByUUID(workspaceID, uuid string) (domain.Task, error) {
	var model Task
	err := r.db.Preload("Tags").Where("workspace_id = ? AND uuid = ?", workspaceID, uuid).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Task{}, ErrNotFound
	}
	if err != nil {
		return domain.Task{}, err
	}
	return fromModel(model), nil
}

func (r *TaskRepository) Update(tsk domain.Task) error {
	if err := tsk.Validate(); err != nil {
		return err
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		model := toModel(tsk)
		if err := tx.Model(&Task{}).Where("uuid = ? AND workspace_id = ?", tsk.UUID, tsk.WorkspaceID).Updates(map[string]any{
			"description": model.Description,
			"status":      model.Status,
			"modified":    model.Modified,
			"end_ts":      model.EndTS,
			"due":         model.Due,
			"project":     model.Project,
			"priority":    model.Priority,
		}).Error; err != nil {
			return err
		}
		if err := tx.Where("task_uuid = ?", tsk.UUID).Delete(&TaskTag{}).Error; err != nil {
			return err
		}
		for _, tag := range sortedUnique(tsk.Tags) {
			if err := tx.Create(&TaskTag{TaskUUID: tsk.UUID, Tag: tag}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

var ErrNotFound = errors.New("task not found")

func (r *TaskRepository) Projects(workspaceID string) ([]string, error) {
	var projects []string
	err := r.db.Model(&Task{}).
		Where("workspace_id = ? AND project IS NOT NULL AND project != ''", workspaceID).
		Distinct("project").
		Order("project ASC").
		Pluck("project", &projects).Error
	return projects, err
}

func (r *TaskRepository) Tags(workspaceID string) ([]string, error) {
	var tags []string
	err := r.db.Model(&TaskTag{}).
		Joins("JOIN tasks ON tasks.uuid = task_tags.task_uuid").
		Where("tasks.workspace_id = ?", workspaceID).
		Distinct("tag").
		Order("tag ASC").
		Pluck("tag", &tags).Error
	return tags, err
}

func toModel(tsk domain.Task) Task {
	tags := make([]TaskTag, 0, len(tsk.Tags))
	for _, tag := range sortedUnique(tsk.Tags) {
		tags = append(tags, TaskTag{TaskUUID: tsk.UUID, Tag: tag})
	}
	return Task{
		UUID: tsk.UUID, WorkspaceID: tsk.WorkspaceID, Description: tsk.Description,
		Status: tsk.Status, Entry: tsk.Entry, Modified: tsk.Modified,
		EndTS: tsk.End, Due: tsk.Due, Project: tsk.Project, Priority: tsk.Priority,
		Tags: tags,
	}
}

func fromModel(model Task) domain.Task {
	tags := make([]string, 0, len(model.Tags))
	for _, tag := range model.Tags {
		tags = append(tags, tag.Tag)
	}
	sort.Strings(tags)
	return domain.Task{
		UUID: model.UUID, WorkspaceID: model.WorkspaceID, Description: model.Description,
		Status: model.Status, Entry: model.Entry, Modified: model.Modified,
		End: model.EndTS, Due: model.Due, Project: model.Project, Priority: model.Priority,
		Tags: tags,
	}
}

func sortedUnique(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
