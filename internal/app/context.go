package app

import (
	"fmt"
	"strings"

	"github.com/dajee/taskg/internal/query"
	"github.com/dajee/taskg/internal/taskcontext"
)

const activeContextMetaKey = "context.active"

func (s *Service) DefineContext(name, filterSource string) error {
	name = strings.TrimSpace(name)
	filterSource = strings.TrimSpace(filterSource)
	if !taskcontext.ValidateName(name) {
		return fmt.Errorf("invalid context name %q", name)
	}
	if _, err := query.ParseQuery(filterSource); err != nil {
		return fmt.Errorf("invalid context filter: %w", err)
	}
	now := s.clock.Unix()
	existing, err := s.contextRepo.Get(s.workspaceID, name)
	if err == nil {
		now = existing.CreatedAt
	}
	return s.contextRepo.Upsert(taskcontext.Context{
		WorkspaceID:  s.workspaceID,
		Name:         name,
		FilterSource: filterSource,
		CreatedAt:    now,
		ModifiedAt:   s.clock.Unix(),
	})
}

func (s *Service) UseContext(name string) error {
	name = strings.TrimSpace(name)
	if _, err := s.contextRepo.Get(s.workspaceID, name); err != nil {
		return err
	}
	return s.store.SetMeta(activeContextMetaKey, name)
}

func (s *Service) ContextNone() error {
	s.activeContextOverride = nil
	return s.store.DeleteMeta(activeContextMetaKey)
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

func (s *Service) ContextList() ([]taskcontext.Context, error) {
	return s.contextRepo.List(s.workspaceID)
}

func (s *Service) ContextDelete(name string) error {
	name = strings.TrimSpace(name)
	if err := s.contextRepo.Delete(s.workspaceID, name); err != nil {
		return err
	}
	activeName, ok, err := s.store.GetMeta(activeContextMetaKey)
	if err != nil {
		return err
	}
	if ok && activeName == name {
		return s.ContextNone()
	}
	return nil
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

func (s *Service) activeContextName() (string, bool, error) {
	if s.activeContextOverride != nil {
		return *s.activeContextOverride, *s.activeContextOverride != "", nil
	}
	name, ok, err := s.store.GetMeta(activeContextMetaKey)
	if err != nil {
		return "", false, err
	}
	return name, ok, nil
}
