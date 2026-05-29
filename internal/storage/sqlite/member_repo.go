package sqlite

import (
	"errors"

	"gorm.io/gorm"
)

type MemberWithUser struct {
	Membership Membership
	User       User
}

type MemberRepository struct {
	db *gorm.DB
}

func NewMemberRepository(db *gorm.DB) *MemberRepository {
	return &MemberRepository{db: db}
}

func (r *MemberRepository) Get(userID, workspaceID string) (Membership, error) {
	var membership Membership
	err := r.db.Where("user_id = ? AND workspace_id = ?", userID, workspaceID).First(&membership).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Membership{}, ErrNotFound
	}
	if err != nil {
		return Membership{}, err
	}
	return membership, nil
}

func (r *MemberRepository) Upsert(member Membership) error {
	return r.db.Save(&member).Error
}

func (r *MemberRepository) UpdateRole(userID, workspaceID, role string, modifiedAt int64) error {
	member, err := r.Get(userID, workspaceID)
	if err != nil {
		return err
	}
	member.Role = role
	member.ModifiedAt = modifiedAt
	return r.db.Save(&member).Error
}

func (r *MemberRepository) List(workspaceID string) ([]MemberWithUser, error) {
	type row struct {
		UserID                 string  `gorm:"column:user_id"`
		WorkspaceID            string  `gorm:"column:workspace_id"`
		Role                   string  `gorm:"column:role"`
		JoinedAt               int64   `gorm:"column:joined_at"`
		ModifiedAt             int64   `gorm:"column:modified_at"`
		UserName               string  `gorm:"column:user_name"`
		UserEmail              *string `gorm:"column:user_email"`
		UserDefaultWorkspaceID *string `gorm:"column:user_default_workspace_id"`
		UserCreatedAt          int64   `gorm:"column:user_created_at"`
		UserModifiedAt         int64   `gorm:"column:user_modified_at"`
	}
	var rows []row
	if err := r.db.Table("memberships").
		Select("memberships.*, users.id AS user_id, users.name AS user_name, users.email AS user_email, users.default_workspace_id AS user_default_workspace_id, users.created_at AS user_created_at, users.modified_at AS user_modified_at").
		Joins("JOIN users ON users.id = memberships.user_id").
		Where("memberships.workspace_id = ?", workspaceID).
		Order("users.name ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]MemberWithUser, 0, len(rows))
	for _, item := range rows {
		out = append(out, MemberWithUser{
			Membership: Membership{
				UserID:      item.UserID,
				WorkspaceID: item.WorkspaceID,
				Role:        item.Role,
				JoinedAt:    item.JoinedAt,
				ModifiedAt:  item.ModifiedAt,
			},
			User: User{
				ID:                 item.UserID,
				Name:               item.UserName,
				Email:              item.UserEmail,
				DefaultWorkspaceID: item.UserDefaultWorkspaceID,
				CreatedAt:          item.UserCreatedAt,
				ModifiedAt:         item.UserModifiedAt,
			},
		})
	}
	return out, nil
}

func (r *MemberRepository) CountOwners(workspaceID string) (int64, error) {
	var count int64
	if err := r.db.Model(&Membership{}).Where("workspace_id = ? AND role = ?", workspaceID, "owner").Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *MemberRepository) OtherUnarchivedWorkspaces(userID, excludeWorkspaceID string) ([]Workspace, error) {
	var workspaces []Workspace
	if err := r.db.Table("workspaces").
		Select("workspaces.*").
		Joins("JOIN memberships ON memberships.workspace_id = workspaces.id").
		Where("memberships.user_id = ? AND workspaces.id <> ? AND workspaces.archived_at IS NULL", userID, excludeWorkspaceID).
		Order("workspaces.slug ASC").
		Find(&workspaces).Error; err != nil {
		return nil, err
	}
	return workspaces, nil
}
