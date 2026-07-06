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
		if err := s.validateConfigDefinitionUpdate(existing, def); err != nil {
			return err
		}
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

// validateConfigDefinitionUpdate 在已有定义被更新时收紧破坏性变更。
// existing 是 storage 原始行，updated 是 normalize 后的新行。
// 规则：
//   - 已有 value 时不能改 value_type。
//   - 不能移除仍有 value 的 scope。
//   - 新 enum 非空时必须覆盖所有现有 workspace/project value。
func (s *Service) validateConfigDefinitionUpdate(existing, updated storage.ConfigDefinition) error {
	workspaceCount, projectCount, err := s.configRepo.CountByKey(s.workspaceID, existing.Key)
	if err != nil {
		return err
	}
	totalValues := workspaceCount + projectCount

	// type 锁定
	if updated.ValueType != existing.ValueType && totalValues > 0 {
		return RuntimeError{
			Code:    "config_definition_type_locked",
			Message: fmt.Sprintf("config definition %q has existing values; value_type cannot be changed", existing.Key),
		}
	}

	existingScopes, err := decodeStringListJSON(existing.AllowedScopesJSON)
	if err != nil {
		return err
	}
	updatedScopes, err := decodeStringListJSON(updated.AllowedScopesJSON)
	if err != nil {
		return err
	}

	// scope 收窄锁定
	existingAllowsWorkspace := slices.Contains(existingScopes, string(ConfigAllowedScopeWorkspace))
	updatedAllowsWorkspace := slices.Contains(updatedScopes, string(ConfigAllowedScopeWorkspace))
	if existingAllowsWorkspace && !updatedAllowsWorkspace && workspaceCount > 0 {
		return RuntimeError{
			Code:    "config_definition_scope_locked",
			Message: fmt.Sprintf("config definition %q has %d workspace values; cannot remove workspace scope", existing.Key, workspaceCount),
		}
	}
	existingAllowsProject := slices.Contains(existingScopes, string(ConfigAllowedScopeProject))
	updatedAllowsProject := slices.Contains(updatedScopes, string(ConfigAllowedScopeProject))
	if existingAllowsProject && !updatedAllowsProject && projectCount > 0 {
		return RuntimeError{
			Code:    "config_definition_scope_locked",
			Message: fmt.Sprintf("config definition %q has %d project values; cannot remove project scope", existing.Key, projectCount),
		}
	}

	// enum 锁定：新 enum 非空时必须覆盖所有现有 value
	updatedEnum, err := decodeStringListJSON(updated.EnumValuesJSON)
	if err != nil {
		return err
	}
	if len(updatedEnum) > 0 && totalValues > 0 {
		existingEnum, err := decodeStringListJSON(existing.EnumValuesJSON)
		if err != nil {
			return err
		}
		// 只有原定义本身是 enum，或现有值依赖 enum 合法性时才校验。
		// 如果旧定义无 enum（自由值），但新定义加了 enum，仍需现有值合法。
		_ = existingEnum
		values, err := s.configRepo.ValuesByKey(s.workspaceID, existing.Key)
		if err != nil {
			return err
		}
		enumSet := make(map[string]bool, len(updatedEnum))
		for _, v := range updatedEnum {
			enumSet[v] = true
		}
		for _, v := range values {
			if !enumSet[v] {
				return RuntimeError{
					Code:    "config_definition_enum_locked",
					Message: fmt.Sprintf("config definition %q has existing value %q not in new enum", existing.Key, v),
				}
			}
		}
	}

	return nil
}
