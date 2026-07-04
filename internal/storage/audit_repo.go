package storage

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

var ErrInvalidAuditScope = errors.New("invalid audit scope")

type AuditLogEntry struct {
	ID                      int64
	ActorType               string
	ActorUserID             *string
	ActorTokenID            *string
	ActorTokenName          *string
	ActorTokenPrefix        *string
	WorkspaceID             *string
	ProjectID               *string
	Action                  string
	TargetType              string
	TargetID                string
	PayloadJSON             string
	DelegatorTokenID        *string
	DelegatorUserID         *string
	AdminActingSessionID    *string
	DelegatorAdminTokenID   *string
	DelegatorAdminTokenName string
	CreatedAt               int64
}

type AuditListOptions struct {
	WorkspaceID *string
	ProjectID   *string
	TargetType  *string
	TargetID    *string
	Action      *string
	Limit       int
	Offset      int
}

type AuditRepository struct {
	db *gorm.DB
}

func NewAuditRepository(db *gorm.DB) *AuditRepository {
	return &AuditRepository{db: db}
}

func (r *AuditRepository) Append(entry AuditLogEntry) error {
	return r.db.Create(&AuditLog{
		ID:                      entry.ID,
		ActorType:               entry.ActorType,
		ActorUserID:             entry.ActorUserID,
		ActorTokenID:            entry.ActorTokenID,
		ActorTokenName:          entry.ActorTokenName,
		ActorTokenPrefix:        entry.ActorTokenPrefix,
		WorkspaceID:             entry.WorkspaceID,
		ProjectID:               entry.ProjectID,
		Action:                  entry.Action,
		TargetType:              entry.TargetType,
		TargetID:                entry.TargetID,
		PayloadJSON:             entry.PayloadJSON,
		DelegatorTokenID:        entry.DelegatorTokenID,
		DelegatorUserID:         entry.DelegatorUserID,
		AdminActingSessionID:    entry.AdminActingSessionID,
		DelegatorAdminTokenID:   entry.DelegatorAdminTokenID,
		DelegatorAdminTokenName: entry.DelegatorAdminTokenName,
		CreatedAt:               entry.CreatedAt,
	}).Error
}

func (r *AuditRepository) List(opts AuditListOptions) ([]AuditLogEntry, error) {
	if opts.ProjectID != nil && opts.WorkspaceID == nil {
		return nil, fmt.Errorf("%w: workspace_id is required when filtering audit logs by project_id", ErrInvalidAuditScope)
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	query := r.db.Model(&AuditLog{})
	if opts.WorkspaceID != nil {
		query = query.Where("workspace_id = ?", *opts.WorkspaceID)
	}
	if opts.ProjectID != nil {
		query = query.Where("project_id = ?", *opts.ProjectID)
	}
	if opts.TargetType != nil {
		query = query.Where("target_type = ?", *opts.TargetType)
	}
	if opts.TargetID != nil {
		query = query.Where("target_id = ?", *opts.TargetID)
	}
	if opts.Action != nil {
		query = query.Where("action = ?", *opts.Action)
	}
	var rows []AuditLog
	if err := query.Order("created_at DESC").Order("id DESC").Offset(opts.Offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]AuditLogEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, AuditLogEntry{
			ID:                      row.ID,
			ActorType:               row.ActorType,
			ActorUserID:             row.ActorUserID,
			ActorTokenID:            row.ActorTokenID,
			ActorTokenName:          row.ActorTokenName,
			ActorTokenPrefix:        row.ActorTokenPrefix,
			WorkspaceID:             row.WorkspaceID,
			ProjectID:               row.ProjectID,
			Action:                  row.Action,
			TargetType:              row.TargetType,
			TargetID:                row.TargetID,
			PayloadJSON:             row.PayloadJSON,
			DelegatorTokenID:        row.DelegatorTokenID,
			DelegatorUserID:         row.DelegatorUserID,
			AdminActingSessionID:    row.AdminActingSessionID,
			DelegatorAdminTokenID:   row.DelegatorAdminTokenID,
			DelegatorAdminTokenName: row.DelegatorAdminTokenName,
			CreatedAt:               row.CreatedAt,
		})
	}
	return out, nil
}
