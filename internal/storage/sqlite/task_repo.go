package sqlite

import (
	"errors"
	"sort"

	domain "github.com/dajee/taskg/internal/task"
	"gorm.io/gorm"
)

type TaskRepository struct {
	db *gorm.DB
}

type ListOptions struct {
	Status   string
	Project  *string
	Priority *string
	Tags     []string
	Text     *string
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
	q := r.db.Preload("Tags").Where("workspace_id = ?", workspaceID)
	if opts.Status != "" {
		q = q.Where("status = ?", opts.Status)
	}
	if opts.Project != nil {
		q = q.Where("project = ?", *opts.Project)
	}
	if opts.Priority != nil {
		q = q.Where("priority = ?", *opts.Priority)
	}
	if opts.Text != nil {
		q = q.Where("description LIKE ?", "%"+*opts.Text+"%")
	}
	for _, tag := range opts.Tags {
		q = q.Where("uuid IN (SELECT task_uuid FROM task_tags WHERE tag = ?)", tag)
	}
	if err := q.Order("entry ASC").Find(&models).Error; err != nil {
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
