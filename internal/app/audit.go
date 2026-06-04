package app

import (
	"encoding/json"
	"fmt"

	"github.com/dajee/taskg/internal/storage"
	"github.com/dajee/taskg/internal/task"
)

type AuditListInput struct {
	WorkspaceRef string
	ProjectRef   string
	Limit        int
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
	ID               int64
	Actor            *task.UserInfo
	WorkspaceID      *string
	ProjectID        *string
	Action           string
	TargetType       string
	TargetID         string
	PayloadJSON      string
	DelegatorTokenID *string
	DelegatorUser    *task.UserInfo
	CreatedAt        int64
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
	return s.store.Transaction(func(txStore *storage.Store) error {
		txSvc, err := s.withStore(txStore)
		if err != nil {
			return err
		}
		entries, events, err := fn(txSvc)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			workspaceID := &txSvc.runtime.WorkspaceID
			if entry.WorkspaceID != nil {
				workspaceID = entry.WorkspaceID
			}
			row := storage.AuditLogEntry{
				ActorUserID:      &txSvc.runtime.ActorUserID,
				WorkspaceID:      workspaceID,
				ProjectID:        entry.ProjectID,
				Action:           entry.Action,
				TargetType:       entry.TargetType,
				TargetID:         entry.TargetID,
				PayloadJSON:      "",
				DelegatorTokenID: stringPtr(txSvc.runtime.DelegatorTokenID),
				DelegatorUserID:  stringPtr(txSvc.runtime.DelegatorUserID),
				CreatedAt:        txSvc.clock.Unix(),
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
		return txSvc.enqueueHookEvents(events)
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
		Limit:       input.Limit,
	})
	if err != nil {
		return nil, err
	}
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
			ID:               row.ID,
			WorkspaceID:      row.WorkspaceID,
			ProjectID:        row.ProjectID,
			Action:           row.Action,
			TargetType:       row.TargetType,
			TargetID:         row.TargetID,
			PayloadJSON:      row.PayloadJSON,
			DelegatorTokenID: row.DelegatorTokenID,
			CreatedAt:        row.CreatedAt,
		}
		if row.ActorUserID != nil {
			ui := userInfos[*row.ActorUserID]
			view.Actor = &ui
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
