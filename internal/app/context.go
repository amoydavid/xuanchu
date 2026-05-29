package app

import (
	"fmt"
	"strings"

	"github.com/dajee/taskg/internal/query"
	"github.com/dajee/taskg/internal/taskcontext"
)

func (s *Service) DefineContext(name, filterSource string) error {
	if err := s.Require(PermissionContextManage); err != nil {
		return err
	}
	return s.withAudit("context.define", func(tx *Service) (AuditEntry, error) {
		name, filterSource, err := tx.defineContextLocked(name, filterSource)
		if err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{
			TargetType: "context",
			TargetID:   name,
			Payload: map[string]any{
				"filter": filterSource,
			},
		}, nil
	})
}

func (s *Service) UseContext(name string) error {
	if err := s.Require(PermissionContextUse); err != nil {
		return err
	}
	return s.withAudit("context.use", func(tx *Service) (AuditEntry, error) {
		name, err := tx.useContextLocked(name)
		if err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{
			TargetType: "context",
			TargetID:   name,
		}, nil
	})
}

func (s *Service) ContextNone() error {
	if err := s.Require(PermissionContextUse); err != nil {
		return err
	}
	return s.withAudit("context.none", func(tx *Service) (AuditEntry, error) {
		if err := tx.contextNoneLocked(); err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{
			TargetType: "context",
		}, nil
	})
}

func (s *Service) ContextShow() (string, error) {
	active, err := s.activeContext()
	if err != nil {
		return "", err
	}
	if active == nil {
		return "", nil
	}
	return fmt.Sprintf("%s %s", active.Name, active.FilterSource), nil
}

func (s *Service) ActiveContextName() (string, bool, error) {
	return s.activeContextName()
}

func (s *Service) ContextList() ([]taskcontext.Context, error) {
	return s.contextRepo.List(s.workspaceID)
}

func (s *Service) ContextDelete(name string) error {
	if err := s.Require(PermissionContextManage); err != nil {
		return err
	}
	return s.withAudit("context.delete", func(tx *Service) (AuditEntry, error) {
		name, err := tx.contextDeleteLocked(name)
		if err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{
			TargetType: "context",
			TargetID:   name,
		}, nil
	})
}

func (s *Service) defineContextLocked(name, filterSource string) (string, string, error) {
	name = strings.TrimSpace(name)
	filterSource = strings.TrimSpace(filterSource)
	if !taskcontext.ValidateName(name) {
		return "", "", fmt.Errorf("invalid context name %q", name)
	}
	if _, err := query.ParseQuery(filterSource); err != nil {
		return "", "", fmt.Errorf("invalid context filter: %w", err)
	}
	now := s.clock.Unix()
	existing, err := s.contextRepo.Get(s.workspaceID, name)
	if err == nil {
		now = existing.CreatedAt
	}
	if err := s.contextRepo.Upsert(taskcontext.Context{
		WorkspaceID:  s.workspaceID,
		Name:         name,
		FilterSource: filterSource,
		CreatedAt:    now,
		ModifiedAt:   s.clock.Unix(),
	}); err != nil {
		return "", "", err
	}
	return name, filterSource, nil
}

func (s *Service) useContextLocked(name string) (string, error) {
	name = strings.TrimSpace(name)
	if _, err := s.contextRepo.Get(s.workspaceID, name); err != nil {
		return "", err
	}
	if err := s.store.SetMeta(s.activeContextMetaKey(), name); err != nil {
		return "", err
	}
	return name, nil
}

func (s *Service) contextNoneLocked() error {
	s.activeContextOverride = nil
	return s.store.SetMeta(s.activeContextMetaKey(), "")
}

func (s *Service) contextDeleteLocked(name string) (string, error) {
	name = strings.TrimSpace(name)
	if err := s.contextRepo.Delete(s.workspaceID, name); err != nil {
		return "", err
	}
	activeName, ok, err := s.store.GetMeta(s.activeContextMetaKey())
	if err != nil {
		return "", err
	}
	if ok && activeName == name {
		if err := s.contextNoneLocked(); err != nil {
			return "", err
		}
	}
	return name, nil
}

func (s *Service) activeContextFilter(skip bool) (query.Expr, error) {
	if skip || s.disableContext {
		return nil, nil
	}
	active, err := s.activeContext()
	if err != nil {
		return nil, err
	}
	if active == nil {
		return nil, nil
	}
	return query.ParseQuery(active.FilterSource)
}

func (s *Service) activeContext() (*taskcontext.Context, error) {
	name, ok, err := s.activeContextName()
	if err != nil {
		return nil, err
	}
	if !ok || name == "" {
		return nil, nil
	}
	ctx, err := s.contextRepo.Get(s.workspaceID, name)
	if err != nil {
		return nil, err
	}
	return &ctx, nil
}

func (s *Service) OverrideActiveContext(name string) {
	s.activeContextOverride = &name
}

func (s *Service) activeContextMetaKey() string {
	return activeContextMetaKey(s.runtime.ActorUserID, s.runtime.WorkspaceID)
}

func (s *Service) activeContextName() (string, bool, error) {
	if s.activeContextOverride != nil {
		return *s.activeContextOverride, *s.activeContextOverride != "", nil
	}
	if name, ok := s.runtimeOverrides["context.active"]; ok {
		return name, name != "", nil
	}
	name, ok, err := s.store.GetMeta(s.activeContextMetaKey())
	if err != nil {
		return "", false, err
	}
	if ok {
		return name, name != "", nil
	}
	return "", false, nil
}
