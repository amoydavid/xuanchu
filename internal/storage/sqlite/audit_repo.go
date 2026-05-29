package sqlite

import "gorm.io/gorm"

type AuditLogEntry struct {
	ID          int64
	ActorUserID *string
	WorkspaceID *string
	Action      string
	TargetType  string
	TargetID    string
	PayloadJSON string
	CreatedAt   int64
}

type AuditListOptions struct {
	WorkspaceID *string
	Limit       int
}

type AuditRepository struct {
	db *gorm.DB
}

func NewAuditRepository(db *gorm.DB) *AuditRepository {
	return &AuditRepository{db: db}
}

func (r *AuditRepository) Append(entry AuditLogEntry) error {
	return r.db.Create(&AuditLog{
		ID:          entry.ID,
		ActorUserID: entry.ActorUserID,
		WorkspaceID: entry.WorkspaceID,
		Action:      entry.Action,
		TargetType:  entry.TargetType,
		TargetID:    entry.TargetID,
		PayloadJSON: entry.PayloadJSON,
		CreatedAt:   entry.CreatedAt,
	}).Error
}

func (r *AuditRepository) List(opts AuditListOptions) ([]AuditLogEntry, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	query := r.db.Model(&AuditLog{})
	if opts.WorkspaceID != nil {
		query = query.Where("workspace_id = ?", *opts.WorkspaceID)
	}
	var rows []AuditLog
	if err := query.Order("created_at DESC").Order("id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]AuditLogEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, AuditLogEntry{
			ID:          row.ID,
			ActorUserID: row.ActorUserID,
			WorkspaceID: row.WorkspaceID,
			Action:      row.Action,
			TargetType:  row.TargetType,
			TargetID:    row.TargetID,
			PayloadJSON: row.PayloadJSON,
			CreatedAt:   row.CreatedAt,
		})
	}
	return out, nil
}
