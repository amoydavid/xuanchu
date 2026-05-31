package app

import (
	"fmt"
	"strings"

	"github.com/dajee/taskg/internal/storage/sqlite"
)

var projectConfigKeys = map[string]bool{
	"agent.background":  true,
	"agent.constraints": true,
	"context.default":   true,
}

func (s *Service) ProjectConfigGet(projectRef, key string) (string, bool, error) {
	if err := s.Require(PermissionProjectConfigRead); err != nil {
		return "", false, err
	}
	key, err := normalizeProjectConfigKey(key)
	if err != nil {
		return "", false, err
	}
	project, err := s.ResolveProject(projectRef)
	if err != nil {
		return "", false, err
	}
	return s.configRepo.Get(sqlite.ConfigKey{
		WorkspaceID: s.workspaceID,
		Scope:       sqlite.ConfigScopeProject,
		ScopeID:     project.ID,
		Key:         key,
	})
}

func (s *Service) ProjectConfigSet(projectRef, key, value string) error {
	if err := s.Require(PermissionProjectConfigWrite); err != nil {
		return err
	}
	key, err := normalizeProjectConfigKey(key)
	if err != nil {
		return err
	}
	return s.withAudit("project.config.set", func(tx *Service) (AuditEntry, error) {
		project, err := tx.ResolveProject(projectRef)
		if err != nil {
			return AuditEntry{}, err
		}
		if err := ensureProjectConfigWritable(project); err != nil {
			return AuditEntry{}, err
		}
		if err := tx.configRepo.Set(sqlite.ConfigKey{
			WorkspaceID: tx.workspaceID,
			Scope:       sqlite.ConfigScopeProject,
			ScopeID:     project.ID,
			Key:         key,
		}, value); err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{
			WorkspaceID: &project.WorkspaceID,
			ProjectID:   &project.ID,
			TargetType:  "project",
			TargetID:    project.ID,
			Payload: map[string]any{
				"key":   key,
				"value": value,
			},
		}, nil
	})
}

func (s *Service) ProjectConfigUnset(projectRef, key string) error {
	if err := s.Require(PermissionProjectConfigWrite); err != nil {
		return err
	}
	key, err := normalizeProjectConfigKey(key)
	if err != nil {
		return err
	}
	return s.withAudit("project.config.unset", func(tx *Service) (AuditEntry, error) {
		project, err := tx.ResolveProject(projectRef)
		if err != nil {
			return AuditEntry{}, err
		}
		if err := ensureProjectConfigWritable(project); err != nil {
			return AuditEntry{}, err
		}
		if err := tx.configRepo.Unset(sqlite.ConfigKey{
			WorkspaceID: tx.workspaceID,
			Scope:       sqlite.ConfigScopeProject,
			ScopeID:     project.ID,
			Key:         key,
		}); err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{
			WorkspaceID: &project.WorkspaceID,
			ProjectID:   &project.ID,
			TargetType:  "project",
			TargetID:    project.ID,
			Payload: map[string]any{
				"key": key,
			},
		}, nil
	})
}

func (s *Service) ProjectConfigList(projectRef string) (map[string]string, error) {
	if err := s.Require(PermissionProjectConfigRead); err != nil {
		return nil, err
	}
	project, err := s.ResolveProject(projectRef)
	if err != nil {
		return nil, err
	}
	return s.configRepo.ListScope(s.workspaceID, sqlite.ConfigScopeProject, project.ID)
}

func isProjectConfigKey(key string) bool {
	return projectConfigKeys[strings.TrimSpace(key)]
}

func normalizeProjectConfigKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if !projectConfigKeys[key] {
		return "", RuntimeError{
			Code:    "project_config_key_invalid",
			Message: fmt.Sprintf("unknown project config key %q", key),
		}
	}
	return key, nil
}

func ensureProjectConfigWritable(project sqlite.Project) error {
	if project.Status == string(sqlite.ProjectStatusArchived) || project.ArchivedAt != nil {
		return RuntimeError{Code: "project_archived", Message: fmt.Sprintf("project %q is archived", project.Slug)}
	}
	return nil
}

func projectConfigScopeRequiredError(verb string) RuntimeError {
	message := "project config requires project scope; use project config set <project> <key> <value>"
	switch verb {
	case "get":
		message = "project config requires project scope; use project config get <project> <key>"
	case "unset":
		message = "project config requires project scope; use project config unset <project> <key>"
	}
	return RuntimeError{
		Code:    "project_config_scope_required",
		Message: message,
	}
}
