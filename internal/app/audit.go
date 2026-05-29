package app

import (
	"encoding/json"

	"github.com/dajee/taskg/internal/storage/sqlite"
)

type AuditListInput struct {
	WorkspaceRef string
	Limit        int
}

type AuditEntry struct {
	Action     string
	TargetType string
	TargetID   string
	Payload    map[string]any
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
			if err := txSvc.auditRepo.Append(sqlite.AuditLogEntry{
				ActorUserID: &txSvc.runtime.ActorUserID,
				WorkspaceID: &txSvc.runtime.WorkspaceID,
				Action:      entry.Action,
				TargetType:  entry.TargetType,
				TargetID:    entry.TargetID,
				PayloadJSON: marshalAuditPayload(entry.Payload),
				CreatedAt:   txSvc.clock.Unix(),
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func marshalAuditPayload(payload map[string]any) string {
	if len(payload) == 0 {
		return ""
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(data)
}

func (s *Service) ListAudit(input AuditListInput) ([]sqlite.AuditLogEntry, error) {
	if err := s.Require(PermissionAuditRead); err != nil {
		return nil, err
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}
	return s.auditRepo.List(sqlite.AuditListOptions{
		WorkspaceID: &s.runtime.WorkspaceID,
		Limit:       limit,
	})
}
