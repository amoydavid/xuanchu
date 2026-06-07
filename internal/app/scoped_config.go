package app

import (
	"fmt"
	"slices"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

func (s *Service) ConfigSchemaSet(input ConfigSchemaInput) error {
	if err := s.Require(PermissionConfigSchemaWrite); err != nil {
		return err
	}
	def, err := normalizeConfigDefinitionInput(input)
	if err != nil {
		return err
	}
	def.WorkspaceID = s.workspaceID
	now := s.clock.Unix()
	existing, ok, err := s.configDefRepo.Get(s.workspaceID, def.Key)
	if err != nil {
		return err
	}
	if ok {
		def.CreatedAt = existing.CreatedAt
	} else {
		def.CreatedAt = now
	}
	def.ModifiedAt = now
	return s.withAudit("config.schema.set", func(tx *Service) (AuditEntry, error) {
		if err := tx.configDefRepo.Set(def); err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{
			WorkspaceID: &tx.workspaceID,
			TargetType:  "config_schema",
			TargetID:    def.Key,
			Payload: map[string]any{
				"key":            def.Key,
				"value_type":     def.ValueType,
				"allowed_scopes": input.AllowedScopes,
			},
		}, nil
	})
}

func (s *Service) ConfigSchemaGet(key string) (ConfigDefinitionView, bool, error) {
	if err := s.Require(PermissionConfigSchemaRead); err != nil {
		return ConfigDefinitionView{}, false, err
	}
	row, ok, err := s.configDefRepo.Get(s.workspaceID, strings.TrimSpace(key))
	if err != nil || !ok {
		return ConfigDefinitionView{}, ok, err
	}
	view, err := configDefinitionViewFromRow(row)
	if err != nil {
		return ConfigDefinitionView{}, false, err
	}
	return view, true, nil
}

func (s *Service) ConfigSchemaList() ([]ConfigDefinitionView, error) {
	if err := s.Require(PermissionConfigSchemaRead); err != nil {
		return nil, err
	}
	rows, err := s.configDefRepo.List(s.workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]ConfigDefinitionView, 0, len(rows))
	for _, row := range rows {
		view, err := configDefinitionViewFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, view)
	}
	return out, nil
}

func (s *Service) ConfigSchemaDelete(key string, purge bool) error {
	if err := s.Require(PermissionConfigSchemaWrite); err != nil {
		return err
	}
	key, err := normalizeScopedConfigKey(key)
	if err != nil {
		return err
	}
	if _, err := s.scopedConfigDefinition(key); err != nil {
		return err
	}
	return s.withAudit("config.schema.delete", func(tx *Service) (AuditEntry, error) {
		workspaceCount, projectCount, err := tx.configRepo.CountByKey(tx.workspaceID, key)
		if err != nil {
			return AuditEntry{}, err
		}
		if !purge && (workspaceCount > 0 || projectCount > 0) {
			return AuditEntry{}, RuntimeError{
				Code:    "config_definition_in_use",
				Message: fmt.Sprintf("config definition %q still has values", key),
			}
		}
		if purge {
			if err := tx.configRepo.DeleteByKey(tx.workspaceID, key); err != nil {
				return AuditEntry{}, err
			}
		}
		if err := tx.configDefRepo.Delete(tx.workspaceID, key); err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{
			WorkspaceID: &tx.workspaceID,
			TargetType:  "config_schema",
			TargetID:    key,
			Payload: map[string]any{
				"key":   key,
				"purge": purge,
			},
		}, nil
	})
}

func (s *Service) scopedConfigDefinition(key string) (ConfigDefinitionView, error) {
	row, ok, err := s.configDefRepo.Get(s.workspaceID, strings.TrimSpace(key))
	if err != nil {
		return ConfigDefinitionView{}, err
	}
	if !ok {
		return ConfigDefinitionView{}, RuntimeError{
			Code:    "config_definition_not_found",
			Message: fmt.Sprintf("config definition %q not found", key),
		}
	}
	return configDefinitionViewFromRow(row)
}

func (s *Service) validateScopedConfigValue(def ConfigDefinitionView, scope storage.ConfigScope, value string) (string, error) {
	if !configDefinitionAllowsScope(def, scope) {
		return "", RuntimeError{
			Code:    "config_scope_not_allowed",
			Message: fmt.Sprintf("config key %q does not allow %s scope", def.Key, scope),
		}
	}
	normalized, err := normalizeScopedConfigValue(def.ValueType, value)
	if err != nil {
		return "", err
	}
	if len(def.EnumValues) > 0 && !slices.Contains(def.EnumValues, normalized) {
		return "", RuntimeError{
			Code:    "config_value_invalid",
			Message: fmt.Sprintf("config value for %q is not in enum", def.Key),
		}
	}
	return normalized, nil
}

func configDefinitionAllowsScope(def ConfigDefinitionView, scope storage.ConfigScope) bool {
	target := ""
	switch scope {
	case storage.ConfigScopeWorkspace:
		target = string(ConfigAllowedScopeWorkspace)
	case storage.ConfigScopeProject:
		target = string(ConfigAllowedScopeProject)
	default:
		return false
	}
	return slices.Contains(def.AllowedScopes, target)
}
