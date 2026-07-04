package app

import (
	"encoding/json"
	"fmt"

	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

type AuditListInput struct {
	WorkspaceRef string
	ProjectRef   string
	TargetType   *string
	TargetID     *string
	Action       *string
	Limit        int
	Offset       int
}

// TaskAuditInput 是任务详情历史专用查询入参。
// 它只接受 limit/offset；target 由内部按 taskRef 解析，
// action 固定为 task.modify。
type TaskAuditInput struct {
	Limit  int
	Offset int
}

type AuditEntry struct {
	Action      string
	WorkspaceID *string
	ProjectID   *string
	TargetType  string
	TargetID    string
	Payload     map[string]any
}

type AuditLogView struct {
	ID                      int64
	ActorType               string
	Actor                   *task.UserInfo
	ActorToken              *TokenActorInfo
	ActorTokenID            *string
	ActorTokenName          *string
	ActorTokenPrefix        *string
	WorkspaceID             *string
	ProjectID               *string
	Action                  string
	TargetType              string
	TargetID                string
	PayloadJSON             string
	Changes                 []TaskFieldChange
	DelegatorTokenID        *string
	DelegatorUser           *task.UserInfo
	AdminActingSessionID    *string
	DelegatorAdminTokenID   *string
	DelegatorAdminTokenName string
	CreatedAt               int64
}

// TaskChangeDisplayValue 是变更历史里某个值的可渲染表示。
// Raw 保留原始机器值（可为 nil），Text 是兜底显示文案。
// 前端应优先用 field + Raw + 当前 i18n locale 格式化，只在
// 未知类型时回退到 Text。
type TaskChangeDisplayValue struct {
	Raw  any
	Text string
}

// TaskFieldChange 是字段级变更的可渲染视图。
// Kind 区分 scalar / set / uda，前端不靠字段 presence 猜渲染模板。
//   - scalar: Previous / Current 必须非 nil（即使 Raw 为 nil），
//     以便 JSON 输出保留显式 null。
//   - set: Added / Removed 总是数组（可为空）。
//   - uda: Entries 承载每个 UDA 的 name + before/after。
type TaskFieldChange struct {
	Field    string
	Kind     string
	LabelKey string
	Previous *TaskChangeDisplayValue
	Current  *TaskChangeDisplayValue
	Added    []TaskChangeDisplayValue
	Removed  []TaskChangeDisplayValue
	Entries  []UDAEntryChange
}

// UDAEntryChange 描述单个 UDA 的 name + before/after。
// Before / After 用指针，nil 表示该侧不存在（新增/删除的 UDA）。
type UDAEntryChange struct {
	Name    string
	Before  *TaskChangeDisplayValue
	After   *TaskChangeDisplayValue
}

type TokenActorInfo struct {
	ID     string
	Name   string
	Prefix string
}

func (s *Service) withAudit(action string, fn func(*Service) (AuditEntry, error)) error {
	return s.withAuditAndEvents(func(tx *Service) (*AuditEntry, []HookEvent, error) {
		entry, err := fn(tx)
		if err != nil {
			return nil, nil, err
		}
		if entry.Action == "" {
			entry.Action = action
		}
		return &entry, nil, nil
	})
}

func (s *Service) withAuditEntries(fn func(*Service) ([]AuditEntry, error)) error {
	return s.withAuditEntriesAndEvents(func(tx *Service) ([]AuditEntry, []HookEvent, error) {
		entries, err := fn(tx)
		if err != nil {
			return nil, nil, err
		}
		return entries, nil, nil
	})
}

func (s *Service) withAuditAndEvents(fn func(*Service) (*AuditEntry, []HookEvent, error)) error {
	return s.withAuditEntriesAndEvents(func(tx *Service) ([]AuditEntry, []HookEvent, error) {
		entry, events, err := fn(tx)
		if err != nil {
			return nil, nil, err
		}
		return []AuditEntry{*entry}, events, nil
	})
}

func (s *Service) withAuditEntriesAndEvents(fn func(*Service) ([]AuditEntry, []HookEvent, error)) error {
	var events []HookEvent
	if err := s.store.Transaction(func(txStore *storage.Store) error {
		txSvc, err := s.withStore(txStore)
		if err != nil {
			return err
		}
		entries, txEvents, err := fn(txSvc)
		if err != nil {
			return err
		}
		events = append([]HookEvent(nil), txEvents...)
		for _, entry := range entries {
			workspaceID := &txSvc.runtime.WorkspaceID
			if entry.WorkspaceID != nil {
				workspaceID = entry.WorkspaceID
			}
			actorType := txSvc.runtime.ActorType
			if actorType == "" {
				actorType = "user"
			}
			var actorUserID *string
			var actorTokenID *string
			var actorTokenName *string
			var actorTokenPrefix *string
			if actorType == "tenant_access_token" {
				actorTokenID = stringPtr(txSvc.runtime.ActorTokenID)
				actorTokenName = stringPtr(txSvc.runtime.ActorTokenName)
				actorTokenPrefix = stringPtr(txSvc.runtime.ActorTokenPrefix)
			} else {
				actorUserID = stringPtr(txSvc.runtime.ActorUserID)
			}
			row := storage.AuditLogEntry{
				ActorType:               actorType,
				ActorUserID:             actorUserID,
				ActorTokenID:            actorTokenID,
				ActorTokenName:          actorTokenName,
				ActorTokenPrefix:        actorTokenPrefix,
				WorkspaceID:             workspaceID,
				ProjectID:               entry.ProjectID,
				Action:                  entry.Action,
				TargetType:              entry.TargetType,
				TargetID:                entry.TargetID,
				PayloadJSON:             "",
				DelegatorTokenID:        stringPtr(txSvc.runtime.DelegatorTokenID),
				DelegatorUserID:         stringPtr(txSvc.runtime.DelegatorUserID),
				AdminActingSessionID:    stringPtr(txSvc.runtime.AdminActingSessionID),
				DelegatorAdminTokenID:   stringPtr(txSvc.runtime.DelegatorAdminTokenID),
				DelegatorAdminTokenName: txSvc.runtime.DelegatorAdminTokenName,
				CreatedAt:               txSvc.clock.Unix(),
			}
			payload, err := marshalAuditPayload(entry.Payload)
			if err != nil {
				return err
			}
			row.PayloadJSON = payload
			if err := txSvc.auditRepo.Append(row); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}
	_ = s.enqueueHookEvents(events)
	_ = s.enqueueEventNotificationDeliveries(events)
	return nil
}

func (s *Service) appendAdminAuditInTx(tx *Service, entry AuditEntry, adminTokenName string) error {
	payload := map[string]any{}
	for key, value := range entry.Payload {
		payload[key] = value
	}
	payload["admin"] = true
	payload["admin_token_name"] = adminTokenName
	raw, err := marshalAuditPayload(payload)
	if err != nil {
		return err
	}
	return tx.auditRepo.Append(storage.AuditLogEntry{
		ActorUserID: nil,
		WorkspaceID: entry.WorkspaceID,
		ProjectID:   entry.ProjectID,
		Action:      entry.Action,
		TargetType:  entry.TargetType,
		TargetID:    entry.TargetID,
		PayloadJSON: raw,
		CreatedAt:   tx.clock.Unix(),
	})
}

func marshalAuditPayload(payload map[string]any) (string, error) {
	if len(payload) == 0 {
		return "", nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal audit payload: %w", err)
	}
	return string(data), nil
}

func (s *Service) ListAudit(input AuditListInput) ([]AuditLogView, error) {
	if err := s.Require(PermissionAuditRead); err != nil {
		return nil, err
	}
	var projectID *string
	if input.ProjectRef != "" {
		project, err := s.ResolveProject(input.ProjectRef)
		if err != nil {
			return nil, err
		}
		projectID = &project.ID
	}
	rows, err := s.auditRepo.List(storage.AuditListOptions{
		WorkspaceID: &s.runtime.WorkspaceID,
		ProjectID:   projectID,
		TargetType:  input.TargetType,
		TargetID:    input.TargetID,
		Action:      input.Action,
		Limit:       input.Limit,
		Offset:      input.Offset,
	})
	if err != nil {
		return nil, err
	}
	return s.auditLogViewsFromRows(rows)
}

// ListTaskAudit 是任务详情历史专用入口，只要求 task:read。
// 它只返回该 task 的 task.modify 变更，避免普通 member/viewer
// 能读任务却看不到详情页历史。
func (s *Service) ListTaskAudit(taskRef string, input TaskAuditInput) ([]AuditLogView, error) {
	if err := s.Require(PermissionTaskRead); err != nil {
		return nil, err
	}
	resolved, err := s.ResolveProtocolTarget(taskRef)
	if err != nil {
		return nil, err
	}
	targetType := "task"
	targetID := resolved.UUID
	action := "task.modify"
	rows, err := s.auditRepo.List(storage.AuditListOptions{
		WorkspaceID: &s.runtime.WorkspaceID,
		TargetType:  &targetType,
		TargetID:    &targetID,
		Action:      &action,
		Limit:       input.Limit,
		Offset:      input.Offset,
	})
	if err != nil {
		return nil, err
	}
	return s.auditLogViewsFromRows(rows)
}

// auditLogViewsFromRows 把 storage 行转换为视图，统一解析 actor / delegator
// 和 payload 里的字段级 changes，避免 ListAudit / ListTaskAudit 复制逻辑。
func (s *Service) auditLogViewsFromRows(rows []storage.AuditLogEntry) ([]AuditLogView, error) {
	out := make([]AuditLogView, 0, len(rows))
	userIDs := make([]string, 0)
	for _, row := range rows {
		if row.ActorUserID != nil {
			userIDs = append(userIDs, *row.ActorUserID)
		}
		if row.DelegatorUserID != nil {
			userIDs = append(userIDs, *row.DelegatorUserID)
		}
	}
	userInfos, err := s.resolveUserInfos(userIDs)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		view := AuditLogView{
			ID:                      row.ID,
			ActorType:               auditActorType(row),
			ActorTokenID:            row.ActorTokenID,
			ActorTokenName:          row.ActorTokenName,
			ActorTokenPrefix:        row.ActorTokenPrefix,
			WorkspaceID:             row.WorkspaceID,
			ProjectID:               row.ProjectID,
			Action:                  row.Action,
			TargetType:              row.TargetType,
			TargetID:                row.TargetID,
			PayloadJSON:             row.PayloadJSON,
			Changes:                 parseTaskFieldChanges(row.PayloadJSON),
			DelegatorTokenID:        row.DelegatorTokenID,
			AdminActingSessionID:    row.AdminActingSessionID,
			DelegatorAdminTokenID:   row.DelegatorAdminTokenID,
			DelegatorAdminTokenName: row.DelegatorAdminTokenName,
			CreatedAt:               row.CreatedAt,
		}
		if row.ActorUserID != nil {
			ui := userInfos[*row.ActorUserID]
			view.Actor = &ui
		}
		if view.ActorType == "tenant_access_token" && row.ActorTokenID != nil {
			view.ActorToken = &TokenActorInfo{
				ID:     *row.ActorTokenID,
				Name:   derefString(row.ActorTokenName),
				Prefix: derefString(row.ActorTokenPrefix),
			}
		}
		if row.DelegatorUserID != nil {
			ui := userInfos[*row.DelegatorUserID]
			view.DelegatorUser = &ui
		}
		out = append(out, view)
	}
	return filterAuditByScope(s.requestScope, out), nil
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func auditActorType(row storage.AuditLogEntry) string {
	if row.ActorType != "" {
		return row.ActorType
	}
	if row.ActorUserID != nil {
		return "user"
	}
	return ""
}
