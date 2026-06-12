package storage

import (
	"errors"

	"gorm.io/gorm"
)

type ServerAdminTokenEntry struct {
	ID          string
	Name        string
	TokenPrefix string
	TokenHash   string
	Enabled     bool
	CreatedAt   int64
	RevokedAt   *int64
	LastUsedAt  *int64
	Description string
}

type ServerAdminTokenRepository struct {
	db *gorm.DB
}

func NewServerAdminTokenRepository(db *gorm.DB) *ServerAdminTokenRepository {
	return &ServerAdminTokenRepository{db: db}
}

func (r *ServerAdminTokenRepository) Create(entry ServerAdminTokenEntry) error {
	row := serverAdminTokenModel(entry)
	return r.db.Select("*").Create(&row).Error
}

func (r *ServerAdminTokenRepository) ListValid() ([]ServerAdminTokenEntry, error) {
	var rows []ServerAdminToken
	if err := r.db.
		Where("enabled = ? AND revoked_at IS NULL", true).
		Order("created_at DESC").
		Order("id DESC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ServerAdminTokenEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, serverAdminTokenEntry(row))
	}
	return out, nil
}

func (r *ServerAdminTokenRepository) GetByPrefix(prefix string) (ServerAdminTokenEntry, error) {
	var row ServerAdminToken
	err := r.db.Where("token_prefix = ?", prefix).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ServerAdminTokenEntry{}, ErrNotFound
	}
	if err != nil {
		return ServerAdminTokenEntry{}, err
	}
	return serverAdminTokenEntry(row), nil
}

func (r *ServerAdminTokenRepository) TouchLastUsed(id string, ts int64) error {
	result := r.db.Model(&ServerAdminToken{}).Where("id = ?", id).Update("last_used_at", ts)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func serverAdminTokenModel(entry ServerAdminTokenEntry) ServerAdminToken {
	return ServerAdminToken{
		ID:          entry.ID,
		Name:        entry.Name,
		TokenPrefix: entry.TokenPrefix,
		TokenHash:   entry.TokenHash,
		Enabled:     entry.Enabled,
		CreatedAt:   entry.CreatedAt,
		RevokedAt:   entry.RevokedAt,
		LastUsedAt:  entry.LastUsedAt,
		Description: entry.Description,
	}
}

func serverAdminTokenEntry(row ServerAdminToken) ServerAdminTokenEntry {
	return ServerAdminTokenEntry{
		ID:          row.ID,
		Name:        row.Name,
		TokenPrefix: row.TokenPrefix,
		TokenHash:   row.TokenHash,
		Enabled:     row.Enabled,
		CreatedAt:   row.CreatedAt,
		RevokedAt:   row.RevokedAt,
		LastUsedAt:  row.LastUsedAt,
		Description: row.Description,
	}
}
