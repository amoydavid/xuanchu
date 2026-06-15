package app

import (
	"fmt"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

const agentConfigValueMaxBytes = 16 * 1024

func (s *Service) ProjectConfigGet(projectRef, key string) (string, bool, error) {
	if err := s.Require(PermissionProjectConfigRead); err != nil {
		return "", false, err
	}
	key, err := normalizeScopedConfigKey(key)
	if err != nil {
		return "", false, err
	}
	def, err := s.scopedConfigDefinition(key)
	if err != nil {
		return "", false, err
	}
	if !configDefinitionAllowsScope(def, storage.ConfigScopeProject) {
		return "", false, RuntimeError{
			Code:    "config_scope_not_allowed",
			Message: fmt.Sprintf("config key %q does not allow project scope", key),
		}
	}
	project, err := s.ResolveProject(projectRef)
	if err != nil {
		return "", false, err
	}
	value, ok, err := s.configRepo.Get(storage.ConfigKey{
		WorkspaceID: s.workspaceID,
		Scope:       storage.ConfigScopeProject,
		ScopeID:     project.ID,
		Key:         key,
	})
	if err != nil {
		return "", false, err
	}
	if ok {
		return value, true, nil
	}
	value, ok, err = s.configRepo.Get(storage.ConfigKey{
		WorkspaceID: s.workspaceID,
		Scope:       storage.ConfigScopeWorkspace,
		ScopeID:     s.workspaceID,
		Key:         key,
	})
	if err != nil {
		return "", false, err
	}
	if ok {
		return value, true, nil
	}
	if def.DefaultValue != nil {
		return *def.DefaultValue, true, nil
	}
	return "", false, nil
}

func (s *Service) ProjectConfigSet(projectRef, key, value string) error {
	if err := s.Require(PermissionProjectConfigWrite); err != nil {
		return err
	}
	key, err := normalizeScopedConfigKey(key)
	if err != nil {
		return err
	}
	if strings.HasPrefix(key, "agent.") && len(value) > agentConfigValueMaxBytes {
		return RuntimeError{Code: "config_value_too_large", Message: "config value too large"}
	}
	def, err := s.scopedConfigDefinition(key)
	if err != nil {
		return err
	}
	normalizedValue, err := s.validateScopedConfigValue(def, storage.ConfigScopeProject, value)
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
		if err := tx.configRepo.Set(storage.ConfigKey{
			WorkspaceID: tx.workspaceID,
			Scope:       storage.ConfigScopeProject,
			ScopeID:     project.ID,
			Key:         key,
		}, normalizedValue); err != nil {
			return AuditEntry{}, err
		}
		payload := map[string]any{
			"key": key,
		}
		if def.Secret {
			payload["secret"] = true
			payload["changed"] = true
		} else {
			payload["value"] = normalizedValue
		}
		return AuditEntry{
			WorkspaceID: &project.WorkspaceID,
			ProjectID:   &project.ID,
			TargetType:  "project",
			TargetID:    project.ID,
			Payload:     payload,
		}, nil
	})
}

func (s *Service) ProjectConfigUnset(projectRef, key string) error {
	if err := s.Require(PermissionProjectConfigWrite); err != nil {
		return err
	}
	key, err := normalizeScopedConfigKey(key)
	if err != nil {
		return err
	}
	def, err := s.scopedConfigDefinition(key)
	if err != nil {
		return err
	}
	if !configDefinitionAllowsScope(def, storage.ConfigScopeProject) {
		return RuntimeError{
			Code:    "config_scope_not_allowed",
			Message: fmt.Sprintf("config key %q does not allow project scope", key),
		}
	}
	return s.withAudit("project.config.unset", func(tx *Service) (AuditEntry, error) {
		project, err := tx.ResolveProject(projectRef)
		if err != nil {
			return AuditEntry{}, err
		}
		if err := ensureProjectConfigWritable(project); err != nil {
			return AuditEntry{}, err
		}
		if err := tx.configRepo.Unset(storage.ConfigKey{
			WorkspaceID: tx.workspaceID,
			Scope:       storage.ConfigScopeProject,
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
	return s.configRepo.ListScope(s.workspaceID, storage.ConfigScopeProject, project.ID)
}

func normalizeScopedConfigKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", RuntimeError{
			Code:    "config_key_invalid",
			Message: "config key is required",
		}
	}
	return key, nil
}

func ensureProjectConfigWritable(project storage.Project) error {
	if isProjectClosed(project) {
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
