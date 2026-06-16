package app

import (
	"errors"
	"strings"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

// AdminListTokens 列出全部 token（跨 workspace），供 server admin 管控。
// 与普通 ListTokens 不同：不按 user 过滤、批量解析完整 user info。
func (s *Service) AdminListTokens(includeRevoked bool) ([]TokenView, error) {
	rows, err := s.tokenRepo.ListAll(includeRevoked)
	if err != nil {
		return nil, RuntimeError{Code: "token_list_failed", Message: "failed to list tokens"}
	}
	return s.fillTokenViews(rows), nil
}

// AdminRevokeToken 以 admin 身份吊销任意 token，绕过 owner 校验。
func (s *Service) AdminRevokeToken(tokenRef, adminTokenName string) error {
	ref := strings.TrimSpace(tokenRef)
	return s.store.Transaction(func(txStore *storage.Store) error {
		txSvc, err := s.withStore(txStore)
		if err != nil {
			return err
		}
		entry, err := txSvc.tokenRepo.GetByIDOrPrefix(ref)
		if err != nil {
			return classifyAdminTokenLookupError(err)
		}
		if entry.RevokedAt != nil {
			return RuntimeError{Code: "token_revoked", Message: "token is already revoked"}
		}
		if err := txSvc.tokenRepo.Revoke(entry.ID, txSvc.clock.Unix()); err != nil {
			return err
		}
		return txSvc.appendAdminAuditInTx(txSvc, AuditEntry{
			Action:     "admin.token.revoke",
			TargetType: "token",
			TargetID:   entry.ID,
			Payload: map[string]any{
				"token_name": entry.Name,
				"user_id":    entry.UserID,
				"type":       entry.Type,
			},
		}, adminTokenName)
	})
}

// AdminModifyTokenInput admin 修改 token 入参。
// 不含 WorkspaceRefs/ProjectRefs：admin 不改 workspace/project 绑定（语义复杂，由 token owner 在普通 console 自服务）。
type AdminModifyTokenInput struct {
	TokenRef       string // ID 或 prefix，与 AdminRevokeToken 一致
	Name           *string
	Scopes         *[]string
	ExpiresIn      *time.Duration
	AdminTokenName string
}

// AdminModifyToken 以 admin 身份修改 token 的 name/scope/过期，绕过 owner 校验。
// TokenRef 支持 ID 或 prefix，与 AdminRevokeToken 的解析行为一致。
// 解析、校验、更新在同一事务内，避免 TOCTOU（查改间隙 token 被吊销）。
func (s *Service) AdminModifyToken(input AdminModifyTokenInput) (*TokenView, error) {
	ref := strings.TrimSpace(input.TokenRef)
	var updated storage.ApiTokenEntry
	tokenFound := false

	if err := s.store.Transaction(func(txStore *storage.Store) error {
		txSvc, err := s.withStore(txStore)
		if err != nil {
			return err
		}
		existing, err := txSvc.tokenRepo.GetByIDOrPrefix(ref)
		if err != nil {
			return classifyAdminTokenLookupError(err)
		}
		// admin 不校验 owner（admin 即最高权限），但仍拦截已吊销/已过期
		if existing.RevokedAt != nil {
			return RuntimeError{Code: "token_revoked", Message: "token is already revoked"}
		}
		if existing.ExpiresAt != nil && *existing.ExpiresAt < txSvc.clock.Unix() {
			return RuntimeError{Code: "token_expired", Message: "token is expired"}
		}

		updates := storage.TokenUpdates{}
		dirty := false

		if input.Name != nil {
			updates.Name = input.Name
			dirty = true
		}

		if input.Scopes != nil {
			// admin 不改 workspace，用 existing workspace 校验 scope 一致性
			scopes, err := auth.ValidateTokenCreate(auth.CreateTokenOptions{
				Type:         existing.Type,
				Scopes:       *input.Scopes,
				WorkspaceIDs: parseIDsFromJSON(existing.WorkspaceIDsJSON),
			})
			if err != nil {
				return RuntimeError{Code: "token_scope_invalid", Message: err.Error()}
			}
			sj, _ := marshalStringSlice(scopes.Values())
			updates.ScopesJSON = &sj
			dirty = true
		}

		if input.ExpiresIn != nil {
			if *input.ExpiresIn == 0 {
				updates.ClearExpiresAt = true
			} else if *input.ExpiresIn < 0 {
				return RuntimeError{Code: "token_scope_invalid", Message: "expires-in must be a positive duration"}
			} else {
				ts := txSvc.clock.Unix() + int64(input.ExpiresIn.Seconds())
				updates.ExpiresAt = &ts
			}
			dirty = true
		}

		if dirty {
			if err := txSvc.tokenRepo.Update(existing.ID, updates); err != nil {
				return err
			}
			if err := txSvc.appendAdminAuditInTx(txSvc, AuditEntry{
				Action:     "admin.token.modify",
				TargetType: "token",
				TargetID:   existing.ID,
				Payload: map[string]any{
					"user_id": existing.UserID,
					"name":    existing.Name,
					"changes": updates.ChangedFields(),
				},
			}, input.AdminTokenName); err != nil {
				return err
			}
		}

		updated = existing // 重新查询拿最新值
		// 事务内重新加载，拿到 update 后的最终状态
		reloaded, err := txSvc.tokenRepo.GetByID(existing.ID)
		if err != nil {
			return err
		}
		updated = reloaded
		tokenFound = true
		return nil
	}); err != nil {
		return nil, err
	}

	if !tokenFound {
		return nil, RuntimeError{Code: "token_not_found", Message: "token not found"}
	}
	views := s.fillTokenViews([]storage.ApiTokenEntry{updated})
	return &views[0], nil
}

// fillTokenViews 批量解析 user info 并转为 TokenView。
// 收集 distinct UserID → 单次 ListByIDs 查询 → 填充 UserInfo，避免 N+1。
func (s *Service) fillTokenViews(rows []storage.ApiTokenEntry) []TokenView {
	if len(rows) == 0 {
		return []TokenView{}
	}
	// 收集 distinct user ID
	seen := make(map[string]struct{}, len(rows))
	userIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		if _, ok := seen[row.UserID]; !ok {
			seen[row.UserID] = struct{}{}
			userIDs = append(userIDs, row.UserID)
		}
	}
	// 单次批量查询。失败时降级为 fallback user（token 仍可列出），
	// 不阻断列表——DB 异常会在 ListAll 阶段就已暴露。
	users, userErr := s.userRepo.ListByIDs(userIDs)
	userMap := make(map[string]storage.User, len(users))
	if userErr == nil {
		for _, u := range users {
			userMap[u.ID] = u
		}
	}

	out := make([]TokenView, 0, len(rows))
	for _, row := range rows {
		view := tokenEntryToView(row)
		if u, ok := userMap[row.UserID]; ok {
			view.User = task.UserInfo{ID: u.ID, Name: u.Name, Email: u.Email}
		} else {
			// 未找到的用户 fallback（与 AGENTS.md §用户信息规范一致）
			view.User = task.UserInfo{ID: row.UserID, Name: row.UserID}
		}
		out = append(out, view)
	}
	return out
}

// classifyAdminTokenLookupError 把 token repo 的查找错误映射为语义化的 RuntimeError。
// 区分「不存在」与「prefix 歧义」，便于 admin 判断是 ref 写错还是太短。
func classifyAdminTokenLookupError(err error) error {
	if errors.Is(err, storage.ErrAmbiguousTokenRef) {
		return RuntimeError{Code: "token_ambiguous_ref", Message: "token reference matches multiple tokens, use a longer prefix or full ID"}
	}
	return RuntimeError{Code: "token_not_found", Message: "token not found"}
}
