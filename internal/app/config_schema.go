package app

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

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
	ConfigValueTypeString   ConfigValueType = "string"
	ConfigValueTypeNumber   ConfigValueType = "number"
	ConfigValueTypeBoolean  ConfigValueType = "boolean"
	ConfigValueTypeJSON     ConfigValueType = "json"
	ConfigValueTypeDate     ConfigValueType = "date"
	ConfigValueTypeDateTime ConfigValueType = "datetime"
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
	Key               string   `json:"key"`
	ValueType         string   `json:"value_type"`
	AllowedScopes     []string `json:"allowed_scopes"`
	Label             string   `json:"label"`
	Description       string   `json:"description"`
	EnumValues        []string `json:"enum_values"`
	DefaultValue      *string  `json:"default_value"`
	Required          bool     `json:"required"`
	Secret            bool     `json:"secret"`
	ShowOnConsoleHome bool     `json:"show_on_console_home"`
	CreatedAt         int64    `json:"created_at"`
	ModifiedAt        int64    `json:"modified_at"`
}

func normalizeConfigDefinitionInput(input ConfigSchemaInput) (storage.ConfigDefinition, error) {
	key := strings.TrimSpace(input.Key)
	if key == "" {
		return storage.ConfigDefinition{}, RuntimeError{Code: "config_key_invalid", Message: "config key is required"}
	}
	valueType := ConfigValueType(strings.TrimSpace(input.ValueType))
	switch valueType {
	case ConfigValueTypeString, ConfigValueTypeNumber, ConfigValueTypeBoolean, ConfigValueTypeJSON, ConfigValueTypeDate, ConfigValueTypeDateTime:
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

	// json / date / datetime 类型不支持 enum：枚举预定义值对这些类型语义不清，
	// 且 date/datetime 的 enum 校验会让归一化逻辑变复杂无收益。
	hasNonEmptyEnum := false
	for _, raw := range input.EnumValues {
		if strings.TrimSpace(raw) != "" {
			hasNonEmptyEnum = true
			break
		}
	}
	if hasNonEmptyEnum {
		switch valueType {
		case ConfigValueTypeJSON:
			return storage.ConfigDefinition{}, RuntimeError{Code: "config_value_invalid", Message: "json config does not support enum values"}
		case ConfigValueTypeDate, ConfigValueTypeDateTime:
			return storage.ConfigDefinition{}, RuntimeError{Code: "config_value_invalid", Message: fmt.Sprintf("%s config does not support enum values", valueType)}
		}
	}

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
	case ConfigValueTypeDate:
		// date 统一存 YYYY-MM-DD，按 UTC 解析避免本地时区把日期偏移一天。
		parsed, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(value), time.UTC)
		if err != nil {
			return "", RuntimeError{Code: "config_value_invalid", Message: fmt.Sprintf("invalid date value %q", value)}
		}
		return parsed.UTC().Format("2006-01-02"), nil
	case ConfigValueTypeDateTime:
		// datetime 必须是带时区 offset 的 RFC3339，统一归一化为 UTC 存储。
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
		if err != nil {
			return "", RuntimeError{Code: "config_value_invalid", Message: fmt.Sprintf("invalid datetime value %q (requires RFC3339 with timezone)", value)}
		}
		return parsed.UTC().Format(time.RFC3339), nil
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
