package sqlite

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

type ApiTokenEntry struct {
	ID               string
	UserID           string
	Name             string
	Type             string
	TokenPrefix      string
	TokenHash        string
	ScopesJSON       string
	WorkspaceIDsJSON string
	ProjectIDsJSON   string
	CreatedAt        int64
	ExpiresAt        *int64
	RevokedAt        *int64
	LastUsedAt       *int64
}

var ErrAmbiguousTokenRef = errors.New("ambiguous token reference")

type TokenRepository struct {
	db *gorm.DB
}

func NewTokenRepository(db *gorm.DB) *TokenRepository {
	return &TokenRepository{db: db}
}

func (r *TokenRepository) Create(entry ApiTokenEntry) error {
	return r.db.Create(apiTokenModel(entry)).Error
}

func (r *TokenRepository) ListByUser(userID string, includeRevoked bool) ([]ApiTokenEntry, error) {
	var rows []ApiToken
	query := r.db.Where("user_id = ?", userID)
	if !includeRevoked {
		query = query.Where("revoked_at IS NULL")
	}
	if err := query.Order("created_at DESC").Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ApiTokenEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, apiTokenEntry(row))
	}
	return out, nil
}

func (r *TokenRepository) GetByPrefix(prefix string) (ApiTokenEntry, error) {
	var row ApiToken
	err := r.db.Where("token_prefix = ?", prefix).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ApiTokenEntry{}, ErrNotFound
	}
	if err != nil {
		return ApiTokenEntry{}, err
	}
	return apiTokenEntry(row), nil
}

func (r *TokenRepository) GetByID(id string) (ApiTokenEntry, error) {
	var row ApiToken
	err := r.db.Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ApiTokenEntry{}, ErrNotFound
	}
	if err != nil {
		return ApiTokenEntry{}, err
	}
	return apiTokenEntry(row), nil
}

func (r *TokenRepository) GetByIDOrPrefix(ref string) (ApiTokenEntry, error) {
	if row, err := r.GetByID(ref); err == nil {
		return row, nil
	} else if err != ErrNotFound {
		return ApiTokenEntry{}, err
	}
	var rows []ApiToken
	if err := r.db.Where("token_prefix LIKE ? ESCAPE '\\'", escapeLike(ref)+"%").Order("created_at DESC").Find(&rows).Error; err != nil {
		return ApiTokenEntry{}, err
	}
	if len(rows) == 0 {
		return ApiTokenEntry{}, ErrNotFound
	}
	if len(rows) > 1 {
		return ApiTokenEntry{}, fmt.Errorf("%w: %q", ErrAmbiguousTokenRef, ref)
	}
	return apiTokenEntry(rows[0]), nil
}

func (r *TokenRepository) Revoke(ref string, ts int64) error {
	row, err := r.GetByIDOrPrefix(ref)
	if err != nil {
		return err
	}
	result := r.db.Model(&ApiToken{}).Where("id = ?", row.ID).Update("revoked_at", ts)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *TokenRepository) TouchLastUsed(id string, ts int64) error {
	result := r.db.Model(&ApiToken{}).Where("id = ?", id).Update("last_used_at", ts)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func apiTokenModel(entry ApiTokenEntry) ApiToken {
	return ApiToken{
		ID:               entry.ID,
		UserID:           entry.UserID,
		Name:             entry.Name,
		Type:             entry.Type,
		TokenPrefix:      entry.TokenPrefix,
		TokenHash:        entry.TokenHash,
		ScopesJSON:       entry.ScopesJSON,
		WorkspaceIDsJSON: entry.WorkspaceIDsJSON,
		ProjectIDsJSON:   entry.ProjectIDsJSON,
		CreatedAt:        entry.CreatedAt,
		ExpiresAt:        entry.ExpiresAt,
		RevokedAt:        entry.RevokedAt,
		LastUsedAt:       entry.LastUsedAt,
	}
}

func apiTokenEntry(row ApiToken) ApiTokenEntry {
	return ApiTokenEntry{
		ID:               row.ID,
		UserID:           row.UserID,
		Name:             row.Name,
		Type:             row.Type,
		TokenPrefix:      row.TokenPrefix,
		TokenHash:        row.TokenHash,
		ScopesJSON:       row.ScopesJSON,
		WorkspaceIDsJSON: row.WorkspaceIDsJSON,
		ProjectIDsJSON:   row.ProjectIDsJSON,
		CreatedAt:        row.CreatedAt,
		ExpiresAt:        row.ExpiresAt,
		RevokedAt:        row.RevokedAt,
		LastUsedAt:       row.LastUsedAt,
	}
}

func escapeLike(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_")
	return replacer.Replace(value)
}
