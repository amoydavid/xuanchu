package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/auth/directory"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"github.com/google/uuid"
)

// DirectoryClient 是 DirectorySyncService 依赖的通讯录客户端接口（便于测试 mock）。
type DirectoryClient interface {
	ListMembers(baseURL, orgID, token string) ([]directory.Member, error)
}

type SyncStats struct {
	Added   int
	Removed int
	Updated int
}

type DirectorySyncService struct {
	store  *storage.Store
	client DirectoryClient
}

func NewDirectorySyncService(store *storage.Store, client DirectoryClient) *DirectorySyncService {
	return &DirectorySyncService{store: store, client: client}
}

// SyncOnce 拉取远端通讯录并 upsert 本地 user/external id/membership，移除 disabled 成员的 membership。
func (s *DirectorySyncService) SyncOnce(ctx context.Context, workspaceID, baseURL, orgID, token string) (SyncStats, error) {
	var stats SyncStats
	members, err := s.client.ListMembers(baseURL, orgID, token)
	if err != nil {
		return stats, fmt.Errorf("list directory members: %w", err)
	}

	now := time.Now().Unix()
	db := s.store.DB()
	userRepo := storage.NewUserRepository(db)
	extRepo := storage.NewExternalIDRepository(db)
	memberRepo := storage.NewMemberRepository(db)

	// 远端 active 成员的 sub 集合，用于检测需移除的本地成员
	activeSubs := make(map[string]bool, len(members))

	for _, m := range members {
		if m.Status == "disabled" {
			continue
		}
		activeSubs[m.Sub] = true

		// 查/建 user
		var userID string
		ext, err := extRepo.GetByProviderAndExternalID("yaoguang", m.Sub)
		switch {
		case err == nil:
			userID = ext.UserID
		case errors.Is(err, storage.ErrNotFound):
			name := uniqueUserName(userRepo, m.DisplayName, m.ID)
			u, err := userRepo.Create(storage.User{ID: uuid.NewString(), Name: name, DisplayName: m.DisplayName, CreatedAt: now, ModifiedAt: now})
			if err != nil {
				return stats, fmt.Errorf("create user for sub %s: %w", m.Sub, err)
			}
			userID = u.ID
			stats.Added++
		default:
			return stats, err
		}

		// 同步主映射 (yaoguang, sub)
		syncExtID(extRepo, userID, "yaoguang", m.Sub, now)
		// 同步 external_identities
		for _, e := range m.ExternalIdentities {
			syncExtID(extRepo, userID, e.Provider, e.Value, now)
		}

		// upsert membership
		if err := memberRepo.Upsert(storage.Membership{
			UserID: userID, WorkspaceID: workspaceID, Role: m.Role, JoinedAt: now, ModifiedAt: now,
		}); err != nil {
			return stats, fmt.Errorf("upsert membership: %w", err)
		}
		if ext.UserID != "" {
			// 已存在用户视为 update
			stats.Updated++
		}
	}

	// 移除 disabled / 远端已不存在的成员的 membership
	var localExt []storage.UserExternalID
	if err := db.Where("provider = ?", "yaoguang").Find(&localExt).Error; err != nil {
		return stats, err
	}
	for _, eid := range localExt {
		if activeSubs[eid.ExternalID] {
			continue
		}
		// 该 sub 在远端不存在或 disabled → 移除其在本 workspace 的 membership（不删 user）
		if err := db.Where("user_id = ? AND workspace_id = ?", eid.UserID, workspaceID).
			Delete(&storage.Membership{}).Error; err != nil {
			return stats, err
		}
		stats.Removed++
	}

	return stats, nil
}

func syncExtID(repo *storage.ExternalIDRepository, userID, provider, externalID string, now int64) {
	_, err := repo.GetByProviderAndExternalID(provider, externalID)
	if err == nil {
		return // 已存在
	}
	if !errors.Is(err, storage.ErrNotFound) {
		return
	}
	_, _ = repo.Create(storage.UserExternalID{
		ID: uuid.NewString(), UserID: userID, Provider: provider, ExternalID: externalID, CreatedAt: now,
	})
}

// uniqueUserName 处理 User.Name 唯一冲突：追加 " (yaoguang:{id})"。
func uniqueUserName(userRepo *storage.UserRepository, name, memberID string) string {
	_, err := userRepo.GetByName(name)
	if errors.Is(err, storage.ErrNotFound) {
		return name
	}
	if err != nil {
		// 出错时退回原名
		return name
	}
	return fmt.Sprintf("%s (yaoguang:%s)", name, memberID)
}
