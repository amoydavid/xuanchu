package sqlite

import (
	"errors"

	"gorm.io/gorm"
)

type TaskLinkRepository struct {
	db *gorm.DB
}

func NewTaskLinkRepository(db *gorm.DB) *TaskLinkRepository {
	return &TaskLinkRepository{db: db}
}

func (r *TaskLinkRepository) Create(link TaskLink) (TaskLink, error) {
	if err := r.db.Create(&link).Error; err != nil {
		return TaskLink{}, err
	}
	return link, nil
}

func (r *TaskLinkRepository) GetByID(id string) (TaskLink, error) {
	var link TaskLink
	err := r.db.Where("id = ?", id).First(&link).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return TaskLink{}, ErrNotFound
	}
	if err != nil {
		return TaskLink{}, err
	}
	return link, nil
}

func (r *TaskLinkRepository) ListByTaskUUID(taskUUID string) ([]TaskLink, error) {
	var links []TaskLink
	if err := r.db.Where("task_uuid = ?", taskUUID).Order("created_at ASC").Find(&links).Error; err != nil {
		return nil, err
	}
	return links, nil
}

func (r *TaskLinkRepository) Delete(id string) error {
	result := r.db.Where("id = ?", id).Delete(&TaskLink{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *TaskLinkRepository) LoadByTaskUUIDs(taskUUIDs []string) (map[string][]TaskLink, error) {
	if len(taskUUIDs) == 0 {
		return nil, nil
	}
	var links []TaskLink
	if err := r.db.Where("task_uuid IN ?", taskUUIDs).Order("created_at ASC").Find(&links).Error; err != nil {
		return nil, err
	}
	result := make(map[string][]TaskLink, len(taskUUIDs))
	for _, link := range links {
		result[link.TaskUUID] = append(result[link.TaskUUID], link)
	}
	return result, nil
}
