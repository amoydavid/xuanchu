package storage

import (
	"errors"

	"gorm.io/gorm"
)

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(user User) (User, error) {
	if err := r.db.Create(&user).Error; err != nil {
		return User{}, err
	}
	return user, nil
}

func (r *UserRepository) GetByID(id string) (User, error) {
	return r.find("id = ?", id)
}

func (r *UserRepository) GetByName(name string) (User, error) {
	return r.find("name = ?", name)
}

func (r *UserRepository) GetByEmail(email string) (User, error) {
	return r.find("email = ?", email)
}

func (r *UserRepository) List() ([]User, error) {
	var users []User
	if err := r.db.Order("name ASC").Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

// ListByIDs 按 ID 批量查询用户（参数绑定，防注入）。
// ids 为空时直接返回空切片，不查 DB。供 admin 批量解析 token 的 user info，避免 N+1。
func (r *UserRepository) ListByIDs(ids []string) ([]User, error) {
	if len(ids) == 0 {
		return []User{}, nil
	}
	var users []User
	if err := r.db.Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

func (r *UserRepository) UpdateDefaultWorkspace(userID, workspaceID string, modifiedAt int64) error {
	result := r.db.Model(&User{}).Where("id = ?", userID).Updates(map[string]any{
		"default_workspace_id": workspaceID,
		"modified_at":          modifiedAt,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *UserRepository) UpdateDisplayName(userID, displayName string, modifiedAt int64) error {
	result := r.db.Model(&User{}).Where("id = ?", userID).Updates(map[string]any{
		"display_name": displayName,
		"modified_at":  modifiedAt,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *UserRepository) GetByExternalID(provider, externalID string) (User, error) {
	var extID UserExternalID
	if err := r.db.Where("provider = ? AND external_id = ?", provider, externalID).First(&extID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return User{}, ErrNotFound
		}
		return User{}, err
	}
	return r.GetByID(extID.UserID)
}

func (r *UserRepository) find(query string, args ...any) (User, error) {
	var user User
	err := r.db.Where(query, args...).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	return user, nil
}
