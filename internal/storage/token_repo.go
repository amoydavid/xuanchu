package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

type ApiTokenEntry struct {
	ID                     string
	UserID                 *string
	Name                   string
	Type                   string
	TokenPrefix            string
	TokenHash              string
	ScopesJSON             string
	WorkspaceIDsJSON       string
	ProjectIDsJSON         string
	IssuedVia              string
	IssuedByAdminTokenID   *string
	IssuedByAdminTokenName *string
	Purpose                string
	CreatedAt              int64
	ExpiresAt              *int64
	RevokedAt              *int64
	LastUsedAt             *int64
}

var ErrAmbiguousTokenRef = errors.New("ambiguous token reference")

func (u TokenUpdates) ChangedFields() map[string]any {
	attrs := map[string]any{}
	if u.Name != nil {
		attrs["name"] = *u.Name
	}
	if u.ScopesJSON != nil {
		attrs["scopes"] = *u.ScopesJSON
	}
	if u.WorkspaceIDsJSON != nil {
		attrs["workspace_ids"] = *u.WorkspaceIDsJSON
	}
	if u.ProjectIDsJSON != nil {
		attrs["project_ids"] = *u.ProjectIDsJSON
	}
	if u.ClearExpiresAt {
		attrs["expires_at"] = nil
	} else if u.ExpiresAt != nil {
		attrs["expires_at"] = *u.ExpiresAt
	}
	return attrs
}

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
	query := r.db.Where("user_id = ? AND type IN ?", userID, []string{"pat", "agent"})
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

// ListAll 返回全部 token（跨 workspace，供 admin 管控用）。
// includeRevoked=false 时过滤已吊销。
func (r *TokenRepository) ListAll(includeRevoked bool) ([]ApiTokenEntry, error) {
	var rows []ApiToken
	query := r.db.Where("type IN ?", []string{"pat", "agent"})
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

func (r *TokenRepository) ListAllByType(tokenType string, includeRevoked bool) ([]ApiTokenEntry, error) {
	return r.ListAllByTypeWithPurpose(tokenType, includeRevoked, false)
}

func (r *TokenRepository) ListAllByTypeWithPurpose(tokenType string, includeRevoked bool, includeAdminSwitch bool) ([]ApiTokenEntry, error) {
	var rows []ApiToken
	query := r.db.Where("type = ?", tokenType)
	if !includeAdminSwitch {
		query = query.Where("(purpose = ? OR purpose = '')", "api")
	}
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

func (r *TokenRepository) ListTenantByWorkspace(workspaceID string, includeRevoked bool) ([]ApiTokenEntry, error) {
	return r.ListTenantByWorkspaceWithPurpose(workspaceID, includeRevoked, false)
}

func (r *TokenRepository) ListTenantByWorkspaceWithPurpose(workspaceID string, includeRevoked bool, includeAdminSwitch bool) ([]ApiTokenEntry, error) {
	rows, err := r.ListAllByTypeWithPurpose("tenant_access_token", includeRevoked, includeAdminSwitch)
	if err != nil {
		return nil, err
	}
	out := make([]ApiTokenEntry, 0, len(rows))
	for _, row := range rows {
		if jsonStringArrayContains(row.WorkspaceIDsJSON, workspaceID) {
			out = append(out, row)
		}
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

type TokenUpdates struct {
	Name             *string
	ScopesJSON       *string
	WorkspaceIDsJSON *string
	ProjectIDsJSON   *string
	ExpiresAt        *int64
	ClearExpiresAt   bool
}

func (r *TokenRepository) Update(id string, updates TokenUpdates) error {
	attrs := map[string]any{}
	if updates.Name != nil {
		attrs["name"] = *updates.Name
	}
	if updates.ScopesJSON != nil {
		attrs["scopes_json"] = *updates.ScopesJSON
	}
	if updates.WorkspaceIDsJSON != nil {
		attrs["workspace_ids_json"] = *updates.WorkspaceIDsJSON
	}
	if updates.ProjectIDsJSON != nil {
		attrs["project_ids_json"] = *updates.ProjectIDsJSON
	}
	if updates.ClearExpiresAt {
		attrs["expires_at"] = nil
	} else if updates.ExpiresAt != nil {
		attrs["expires_at"] = *updates.ExpiresAt
	}
	if len(attrs) == 0 {
		return nil
	}
	result := r.db.Model(&ApiToken{}).Where("id = ?", id).Updates(attrs)
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
		ID:                     entry.ID,
		UserID:                 entry.UserID,
		Name:                   entry.Name,
		Type:                   entry.Type,
		TokenPrefix:            entry.TokenPrefix,
		TokenHash:              entry.TokenHash,
		ScopesJSON:             entry.ScopesJSON,
		WorkspaceIDsJSON:       entry.WorkspaceIDsJSON,
		ProjectIDsJSON:         entry.ProjectIDsJSON,
		IssuedVia:              defaultString(entry.IssuedVia, "user"),
		IssuedByAdminTokenID:   entry.IssuedByAdminTokenID,
		IssuedByAdminTokenName: entry.IssuedByAdminTokenName,
		Purpose:                defaultString(entry.Purpose, "api"),
		CreatedAt:              entry.CreatedAt,
		ExpiresAt:              entry.ExpiresAt,
		RevokedAt:              entry.RevokedAt,
		LastUsedAt:             entry.LastUsedAt,
	}
}

func apiTokenEntry(row ApiToken) ApiTokenEntry {
	return ApiTokenEntry{
		ID:                     row.ID,
		UserID:                 row.UserID,
		Name:                   row.Name,
		Type:                   row.Type,
		TokenPrefix:            row.TokenPrefix,
		TokenHash:              row.TokenHash,
		ScopesJSON:             row.ScopesJSON,
		WorkspaceIDsJSON:       row.WorkspaceIDsJSON,
		ProjectIDsJSON:         row.ProjectIDsJSON,
		IssuedVia:              row.IssuedVia,
		IssuedByAdminTokenID:   row.IssuedByAdminTokenID,
		IssuedByAdminTokenName: row.IssuedByAdminTokenName,
		Purpose:                row.Purpose,
		CreatedAt:              row.CreatedAt,
		ExpiresAt:              row.ExpiresAt,
		RevokedAt:              row.RevokedAt,
		LastUsedAt:             row.LastUsedAt,
	}
}

func escapeLike(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_")
	return replacer.Replace(value)
}

func jsonStringArrayContains(raw, value string) bool {
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return false
	}
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
