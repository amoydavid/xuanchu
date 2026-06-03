package sqlite

import (
	"errors"

	"gorm.io/gorm"
)

type ExternalIDRepository struct {
	db *gorm.DB
}

func NewExternalIDRepository(db *gorm.DB) *ExternalIDRepository {
	return &ExternalIDRepository{db: db}
}

func (r *ExternalIDRepository) Create(extID UserExternalID) (UserExternalID, error) {
	if err := r.db.Create(&extID).Error; err != nil {
		return UserExternalID{}, err
	}
	return extID, nil
}

func (r *ExternalIDRepository) GetByProviderAndExternalID(provider, externalID string) (UserExternalID, error) {
	var extID UserExternalID
	err := r.db.Where("provider = ? AND external_id = ?", provider, externalID).First(&extID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return UserExternalID{}, ErrNotFound
	}
	if err != nil {
		return UserExternalID{}, err
	}
	return extID, nil
}

func (r *ExternalIDRepository) ListByUser(userID string) ([]UserExternalID, error) {
	var ids []UserExternalID
	if err := r.db.Where("user_id = ?", userID).Order("provider ASC, external_id ASC").Find(&ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *ExternalIDRepository) ListByUsers(userIDs []string) ([]UserExternalID, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	var ids []UserExternalID
	if err := r.db.Where("user_id IN ?", userIDs).Order("user_id ASC, provider ASC, external_id ASC").Find(&ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *ExternalIDRepository) Delete(userID, provider, externalID string) error {
	result := r.db.Where("user_id = ? AND provider = ? AND external_id = ?", userID, provider, externalID).Delete(&UserExternalID{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
