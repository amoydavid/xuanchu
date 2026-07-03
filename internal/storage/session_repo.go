package storage

import (
	"errors"

	"gorm.io/gorm"
)

type SessionRepository struct {
	db *gorm.DB
}

func NewSessionRepository(db *gorm.DB) *SessionRepository {
	return &SessionRepository{db: db}
}

func (r *SessionRepository) CreateSession(idHash, userID, workspaceID, csrfHash string, createdAt, expiresAt int64) error {
	return r.db.Create(&BrowserSession{
		ID:          idHash,
		UserID:      userID,
		WorkspaceID: workspaceID,
		CSRFHash:    csrfHash,
		ExpiresAt:   expiresAt,
		CreatedAt:   createdAt,
		LastSeenAt:  createdAt,
	}).Error
}

func (r *SessionRepository) GetSession(idHash string) (BrowserSession, error) {
	var s BrowserSession
	err := r.db.First(&s, "id = ?", idHash).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return BrowserSession{}, ErrNotFound
	}
	return s, err
}

func (r *SessionRepository) TouchSession(idHash string, now int64) error {
	return r.db.Model(&BrowserSession{}).Where("id = ?", idHash).
		Update("last_seen_at", now).Error
}

func (r *SessionRepository) DeleteSession(idHash string) error {
	return r.db.Delete(&BrowserSession{}, "id = ?", idHash).Error
}

func (r *SessionRepository) PurgeExpiredSessions(now int64) (int64, error) {
	res := r.db.Where("expires_at < ?", now).Delete(&BrowserSession{})
	return res.RowsAffected, res.Error
}

func (r *SessionRepository) CreateAuthFlow(state, workspaceID, pkceVerifier string, createdAt, expiresAt int64) error {
	return r.db.Create(&BrowserAuthFlow{
		State:        state,
		WorkspaceID:  workspaceID,
		PKCEVerifier: pkceVerifier,
		CreatedAt:    createdAt,
		ExpiresAt:    expiresAt,
	}).Error
}

func (r *SessionRepository) GetAuthFlow(state string) (BrowserAuthFlow, error) {
	var f BrowserAuthFlow
	err := r.db.First(&f, "state = ?", state).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return BrowserAuthFlow{}, ErrNotFound
	}
	return f, err
}

func (r *SessionRepository) DeleteAuthFlow(state string) error {
	return r.db.Delete(&BrowserAuthFlow{}, "state = ?", state).Error
}

func (r *SessionRepository) PurgeExpiredAuthFlows(now int64) (int64, error) {
	res := r.db.Where("expires_at < ?", now).Delete(&BrowserAuthFlow{})
	return res.RowsAffected, res.Error
}
