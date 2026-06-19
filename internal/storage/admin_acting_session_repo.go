package storage

import (
	"errors"

	"gorm.io/gorm"
)

// AdminActingSessionEntry 是 admin_acting_sessions 的领域投影。
// acting session 不与普通 api_tokens 共表，仓储独立维护。
type AdminActingSessionEntry struct {
	ID             string
	TokenPrefix    string
	TokenHash      string
	AdminTokenID   *string
	AdminTokenName string
	WorkspaceID    string
	ActorUserID    string
	Role           string
	CreatedAt      int64
	ExpiresAt      int64
	RevokedAt      *int64
	LastUsedAt     *int64
}

type AdminActingSessionRepository struct {
	db *gorm.DB
}

func NewAdminActingSessionRepository(db *gorm.DB) *AdminActingSessionRepository {
	return &AdminActingSessionRepository{db: db}
}

func (r *AdminActingSessionRepository) Create(entry AdminActingSessionEntry) error {
	row := adminActingSessionModel(entry)
	return r.db.Select("*").Create(&row).Error
}

// GetByPrefix 按 token 显示前缀查找。前缀由 raw acting token 截断得到。
func (r *AdminActingSessionRepository) GetByPrefix(prefix string) (AdminActingSessionEntry, error) {
	var row AdminActingSession
	err := r.db.Where("token_prefix = ?", prefix).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return AdminActingSessionEntry{}, ErrNotFound
	}
	if err != nil {
		return AdminActingSessionEntry{}, err
	}
	return adminActingSessionEntry(row), nil
}

func (r *AdminActingSessionRepository) GetByID(id string) (AdminActingSessionEntry, error) {
	var row AdminActingSession
	err := r.db.Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return AdminActingSessionEntry{}, ErrNotFound
	}
	if err != nil {
		return AdminActingSessionEntry{}, err
	}
	return adminActingSessionEntry(row), nil
}

// ListValid 返回未吊销、未过期的 acting session。
// 主要用于潜在的运营审计；鉴权路径只走 GetByPrefix + 时间校验。
func (r *AdminActingSessionRepository) ListValid(now int64) ([]AdminActingSessionEntry, error) {
	var rows []AdminActingSession
	if err := r.db.
		Where("revoked_at IS NULL AND expires_at > ?", now).
		Order("created_at DESC").
		Order("id DESC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]AdminActingSessionEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, adminActingSessionEntry(row))
	}
	return out, nil
}

func (r *AdminActingSessionRepository) TouchLastUsed(id string, ts int64) error {
	result := r.db.Model(&AdminActingSession{}).Where("id = ?", id).Update("last_used_at", ts)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *AdminActingSessionRepository) Revoke(id string, ts int64) error {
	result := r.db.Model(&AdminActingSession{}).Where("id = ?", id).Update("revoked_at", ts)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func adminActingSessionModel(entry AdminActingSessionEntry) AdminActingSession {
	return AdminActingSession{
		ID:             entry.ID,
		TokenPrefix:    entry.TokenPrefix,
		TokenHash:      entry.TokenHash,
		AdminTokenID:   entry.AdminTokenID,
		AdminTokenName: entry.AdminTokenName,
		WorkspaceID:    entry.WorkspaceID,
		ActorUserID:    entry.ActorUserID,
		Role:           entry.Role,
		CreatedAt:      entry.CreatedAt,
		ExpiresAt:      entry.ExpiresAt,
		RevokedAt:      entry.RevokedAt,
		LastUsedAt:     entry.LastUsedAt,
	}
}

func adminActingSessionEntry(row AdminActingSession) AdminActingSessionEntry {
	return AdminActingSessionEntry{
		ID:             row.ID,
		TokenPrefix:    row.TokenPrefix,
		TokenHash:      row.TokenHash,
		AdminTokenID:   row.AdminTokenID,
		AdminTokenName: row.AdminTokenName,
		WorkspaceID:    row.WorkspaceID,
		ActorUserID:    row.ActorUserID,
		Role:           row.Role,
		CreatedAt:      row.CreatedAt,
		ExpiresAt:      row.ExpiresAt,
		RevokedAt:      row.RevokedAt,
		LastUsedAt:     row.LastUsedAt,
	}
}
