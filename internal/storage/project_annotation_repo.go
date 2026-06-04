package storage

import (
	"errors"

	"gorm.io/gorm"
)

type ProjectAnnotationRepository struct {
	db *gorm.DB
}

type TimelineRow struct {
	SourceType  string
	SourceID    string
	SourceLabel string
	Entry       int64
	Content     string
	CreatedBy   string
}

func NewProjectAnnotationRepository(db *gorm.DB) *ProjectAnnotationRepository {
	return &ProjectAnnotationRepository{db: db}
}

func (r *ProjectAnnotationRepository) Create(annotation ProjectAnnotation) (ProjectAnnotation, error) {
	if err := r.db.Create(&annotation).Error; err != nil {
		return ProjectAnnotation{}, err
	}
	return annotation, nil
}

func (r *ProjectAnnotationRepository) Delete(id string) error {
	result := r.db.Where("id = ?", id).Delete(&ProjectAnnotation{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *ProjectAnnotationRepository) ListByProject(projectID string) ([]ProjectAnnotation, error) {
	var annotations []ProjectAnnotation
	if err := r.db.Where("project_id = ?", projectID).Order("entry ASC").Find(&annotations).Error; err != nil {
		return nil, err
	}
	return annotations, nil
}

func (r *ProjectAnnotationRepository) GetByID(id string) (ProjectAnnotation, error) {
	var annotation ProjectAnnotation
	err := r.db.Where("id = ?", id).First(&annotation).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ProjectAnnotation{}, ErrNotFound
	}
	if err != nil {
		return ProjectAnnotation{}, err
	}
	return annotation, nil
}

func (r *ProjectAnnotationRepository) RecentByProject(projectID string, limit int) ([]ProjectAnnotation, error) {
	var annotations []ProjectAnnotation
	if err := r.db.Where("project_id = ?", projectID).Order("entry DESC").Limit(limit).Find(&annotations).Error; err != nil {
		return nil, err
	}
	for i, j := 0, len(annotations)-1; i < j; i, j = i+1, j-1 {
		annotations[i], annotations[j] = annotations[j], annotations[i]
	}
	return annotations, nil
}

func (r *ProjectAnnotationRepository) TimelineByProjectID(projectID string, limit, offset int) ([]TimelineRow, error) {
	var project Project
	if err := r.db.Where("id = ?", projectID).First(&project).Error; err != nil {
		return nil, err
	}

	var rows []TimelineRow
	err := r.db.Raw(`
SELECT source_type, source_id, source_label, entry, content, created_by FROM (
    SELECT 'project' AS source_type, ? AS source_id, ? AS source_label,
           entry, content, created_by
    FROM project_annotations WHERE project_id = ?
    UNION ALL
    SELECT 'task' AS source_type, t.uuid AS source_id, t.description AS source_label,
           ta.entry, ta.description AS content, '' AS created_by
    FROM task_annotations ta
    JOIN tasks t ON t.uuid = ta.task_uuid
    WHERE t.project_id = ?
)
ORDER BY entry ASC
LIMIT ? OFFSET ?`,
		projectID, project.Slug,
		projectID,
		projectID,
		limit, offset,
	).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}
