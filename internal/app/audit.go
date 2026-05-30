package app

import (
	"encoding/json"
	"fmt"

	"github.com/dajee/taskg/internal/storage/sqlite"
)

type AuditListInput struct {
	WorkspaceRef string
	Limit        int
}

type AuditEntry struct {
	Action      string
	WorkspaceID *string
	TargetType  string
	TargetID    string
	Payload     map[string]any
}

type AuditLogView struct {
	ID          int64
	ActorUserID *string
	ActorName   string
	WorkspaceID *string
	Action      string
	TargetType  string
	TargetID    string
	PayloadJSON string
	CreatedAt   int64
}

func (s *Service) withAudit(action string, fn func(*Service) (AuditEntry, error)) error {
	return s.withAuditEntries(func(tx *Service) ([]AuditEntry, error) {
		entry, err := fn(tx)
		if err != nil {
			return nil, err
		}
		if entry.Action == "" {
			entry.Action = action
		}
		return []AuditEntry{entry}, nil
	})
}

func (s *Service) withAuditEntries(fn func(*Service) ([]AuditEntry, error)) error {
	return s.store.Transaction(func(txStore *sqlite.Store) error {
		txSvc, err := s.withStore(txStore)
		if err != nil {
			return err
		}
		entries, err := fn(txSvc)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			workspaceID := &txSvc.runtime.WorkspaceID
			if entry.WorkspaceID != nil {
				workspaceID = entry.WorkspaceID
			}
			row := sqlite.AuditLogEntry{
				ActorUserID: &txSvc.runtime.ActorUserID,
				WorkspaceID: workspaceID,
				Action:      entry.Action,
				TargetType:  entry.TargetType,
				TargetID:    entry.TargetID,
				PayloadJSON: "",
				CreatedAt:   txSvc.clock.Unix(),
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
	rows, err := s.auditRepo.List(sqlite.AuditListOptions{
		WorkspaceID: &s.runtime.WorkspaceID,
		Limit:       input.Limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]AuditLogView, 0, len(rows))
	names := map[string]string{}
	for _, row := range rows {
		view := AuditLogView{
			ID:          row.ID,
			ActorUserID: row.ActorUserID,
			WorkspaceID: row.WorkspaceID,
			Action:      row.Action,
			TargetType:  row.TargetType,
			TargetID:    row.TargetID,
			PayloadJSON: row.PayloadJSON,
			CreatedAt:   row.CreatedAt,
		}
		if row.ActorUserID != nil {
			actorID := *row.ActorUserID
			if name, ok := names[actorID]; ok {
				view.ActorName = name
			} else if user, err := s.userRepo.GetByID(actorID); err == nil {
				view.ActorName = user.Name
				names[actorID] = user.Name
			} else if err == sqlite.ErrNotFound {
				view.ActorName = actorID
				names[actorID] = view.ActorName
			} else {
				return nil, err
			}
		}
		out = append(out, view)
	}
	return out, nil
}
