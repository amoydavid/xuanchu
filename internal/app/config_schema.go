package app

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

var builtinScopedConfigSchemas = []ConfigSchemaInput{
	{Key: "agent.background", ValueType: string(ConfigValueTypeString), AllowedScopes: []string{string(ConfigAllowedScopeProject)}},
	{Key: "agent.constraints", ValueType: string(ConfigValueTypeString), AllowedScopes: []string{string(ConfigAllowedScopeProject)}},
	{Key: "agent.default_context", ValueType: string(ConfigValueTypeString), AllowedScopes: []string{string(ConfigAllowedScopeProject)}},
	{Key: "agent.handoff", ValueType: string(ConfigValueTypeString), AllowedScopes: []string{string(ConfigAllowedScopeProject)}},
	{Key: "context.default", ValueType: string(ConfigValueTypeString), AllowedScopes: []string{string(ConfigAllowedScopeProject)}},
}

type ConfigValueType string

const (
	ConfigValueTypeString  ConfigValueType = "string"
	ConfigValueTypeNumber  ConfigValueType = "number"
	ConfigValueTypeBoolean ConfigValueType = "boolean"
	ConfigValueTypeJSON    ConfigValueType = "json"
)

type ConfigAllowedScope string

const (
	ConfigAllowedScopeWorkspace ConfigAllowedScope = "workspace"
	ConfigAllowedScopeProject   ConfigAllowedScope = "project"
)

type ConfigSchemaInput struct {
	Key               string
	ValueType         string
	AllowedScopes     []string
	Label             string
	Description       string
	EnumValues        []string
	DefaultValue      *string
	Required          bool
	Secret            bool
	ShowOnConsoleHome bool
}

type ConfigDefinitionView struct {
	Key               string
	ValueType         string
	AllowedScopes     []string
	Label             string
	Description       string
	EnumValues        []string
	DefaultValue      *string
	Required          bool
	Secret            bool
	ShowOnConsoleHome bool
	CreatedAt         int64
	ModifiedAt        int64
}

func normalizeConfigDefinitionInput(input ConfigSchemaInput) (storage.ConfigDefinition, error) {
	key := strings.TrimSpace(input.Key)
	if key == "" {
		return storage.ConfigDefinition{}, RuntimeError{Code: "config_key_invalid", Message: "config key is required"}
	}
	valueType := ConfigValueType(strings.TrimSpace(input.ValueType))
	switch valueType {
	case ConfigValueTypeString, ConfigValueTypeNumber, ConfigValueTypeBoolean, ConfigValueTypeJSON:
	default:
		return storage.ConfigDefinition{}, RuntimeError{Code: "config_value_type_invalid", Message: fmt.Sprintf("unsupported config type %q", input.ValueType)}
	}

	scopeSet := map[string]bool{}
	scopes := make([]string, 0, len(input.AllowedScopes))
	for _, raw := range input.AllowedScopes {
		scope := string(ConfigAllowedScope(strings.TrimSpace(raw)))
		switch scope {
		case string(ConfigAllowedScopeWorkspace), string(ConfigAllowedScopeProject):
			if !scopeSet[scope] {
				scopeSet[scope] = true
				scopes = append(scopes, scope)
			}
		case "":
		default:
			return storage.ConfigDefinition{}, RuntimeError{Code: "config_scope_invalid", Message: fmt.Sprintf("unsupported config scope %q", raw)}
		}
	}
	if len(scopes) == 0 {
		return storage.ConfigDefinition{}, RuntimeError{Code: "config_scope_invalid", Message: "config schema must allow at least one scope"}
	}
	sort.Strings(scopes)

	enumValues := make([]string, 0, len(input.EnumValues))
	enumSet := map[string]bool{}
	for _, raw := range input.EnumValues {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		normalized, err := normalizeScopedConfigValue(string(valueType), trimmed)
		if err != nil {
			return storage.ConfigDefinition{}, err
		}
		if valueType == ConfigValueTypeJSON {
			return storage.ConfigDefinition{}, RuntimeError{Code: "config_value_invalid", Message: "json config does not support enum values"}
		}
		if !enumSet[normalized] {
			enumSet[normalized] = true
			enumValues = append(enumValues, normalized)
		}
	}
	sort.Strings(enumValues)

	var defaultValue string
	hasDefault := input.DefaultValue != nil
	if input.DefaultValue != nil {
		normalized, err := normalizeScopedConfigValue(string(valueType), *input.DefaultValue)
		if err != nil {
			return storage.ConfigDefinition{}, err
		}
		if len(enumValues) > 0 && !slices.Contains(enumValues, normalized) {
			return storage.ConfigDefinition{}, RuntimeError{Code: "config_value_invalid", Message: fmt.Sprintf("default value for %q is not in enum", key)}
		}
		defaultValue = normalized
	}

	scopesJSON, err := json.Marshal(scopes)
	if err != nil {
		return storage.ConfigDefinition{}, err
	}
	enumJSON, err := json.Marshal(enumValues)
	if err != nil {
		return storage.ConfigDefinition{}, err
	}

	return storage.ConfigDefinition{
		Key:               key,
		ValueType:         string(valueType),
		AllowedScopesJSON: string(scopesJSON),
		Label:             strings.TrimSpace(input.Label),
		Description:       strings.TrimSpace(input.Description),
		EnumValuesJSON:    string(enumJSON),
		DefaultValue:      defaultValue,
		HasDefault:        hasDefault,
		Required:          input.Required,
		Secret:            input.Secret,
		ShowOnConsoleHome: input.ShowOnConsoleHome,
	}, nil
}

func configDefinitionViewFromRow(row storage.ConfigDefinition) (ConfigDefinitionView, error) {
	scopes, err := decodeStringListJSON(row.AllowedScopesJSON)
	if err != nil {
		return ConfigDefinitionView{}, err
	}
	enumValues, err := decodeStringListJSON(row.EnumValuesJSON)
	if err != nil {
		return ConfigDefinitionView{}, err
	}
	var defaultValue *string
	if row.HasDefault {
		value := row.DefaultValue
		defaultValue = &value
	}
	return ConfigDefinitionView{
		Key:               row.Key,
		ValueType:         row.ValueType,
		AllowedScopes:     scopes,
		Label:             row.Label,
		Description:       row.Description,
		EnumValues:        enumValues,
		DefaultValue:      defaultValue,
		Required:          row.Required,
		Secret:            row.Secret,
		ShowOnConsoleHome: row.ShowOnConsoleHome,
		CreatedAt:         row.CreatedAt,
		ModifiedAt:        row.ModifiedAt,
	}, nil
}

func decodeStringListJSON(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	return values, nil
}

func normalizeScopedConfigValue(valueType, value string) (string, error) {
	switch ConfigValueType(valueType) {
	case ConfigValueTypeString:
		return value, nil
	case ConfigValueTypeNumber:
		number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return "", RuntimeError{Code: "config_value_invalid", Message: fmt.Sprintf("invalid number value %q", value)}
		}
		return strconv.FormatFloat(number, 'f', -1, 64), nil
	case ConfigValueTypeBoolean:
		switch strings.TrimSpace(strings.ToLower(value)) {
		case "true":
			return "true", nil
		case "false":
			return "false", nil
		default:
			return "", RuntimeError{Code: "config_value_invalid", Message: fmt.Sprintf("invalid boolean value %q", value)}
		}
	case ConfigValueTypeJSON:
		var payload any
		if err := json.Unmarshal([]byte(value), &payload); err != nil {
			return "", RuntimeError{Code: "config_value_invalid", Message: fmt.Sprintf("invalid json value for config: %v", err)}
		}
		normalized, err := json.Marshal(payload)
		if err != nil {
			return "", err
		}
		return string(normalized), nil
	default:
		return "", RuntimeError{Code: "config_value_type_invalid", Message: fmt.Sprintf("unsupported config type %q", valueType)}
	}
}

func (s *Service) ensureBuiltinConfigDefinitions(workspaceID string) error {
	now := s.clock.Unix()
	for _, input := range builtinScopedConfigSchemas {
		def, err := normalizeConfigDefinitionInput(input)
		if err != nil {
			return err
		}
		def.WorkspaceID = workspaceID
		_, ok, err := s.configDefRepo.Get(workspaceID, def.Key)
		if err != nil {
			return err
		}
		if ok {
			continue
		}
		def.CreatedAt = now
		def.ModifiedAt = now
		if err := s.configDefRepo.Set(def); err != nil {
			return err
		}
	}
	return nil
}
