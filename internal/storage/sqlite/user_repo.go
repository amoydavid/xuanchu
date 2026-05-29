package sqlite

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

func (r *UserRepository) UpdateDefaultWorkspace(userID, workspaceID string, modifiedAt int64) error {
	return r.db.Model(&User{}).Where("id = ?", userID).Updates(map[string]any{
		"default_workspace_id": workspaceID,
		"modified_at":          modifiedAt,
	}).Error
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
