package app

import (
	"encoding/json"
	"fmt"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

// 默认 Provider config key，按 spec §9.3。规则可保存自定义 key，但默认指向这些。
const (
	defaultProviderBaseURLKey      = "agent.provider.base_url"
	defaultProviderAPIKeyKey       = "agent.provider.api_key"
	defaultProviderModelKey        = "agent.provider.model"
	defaultProviderProtocolKey     = "agent.provider.protocol"
	defaultProviderAllowedHostsKey = "agent.provider.allowed_hosts"
)

// AutomationProviderConfigView 是安全 Provider 配置读取 DTO。
// 永远不返回 API key、密钥掩码或可逆密文，只暴露 api_key_set 布尔。
type AutomationProviderConfigView struct {
	BaseURL       string   `json:"base_url"`
	Model         string   `json:"model"`
	AllowedHosts  []string `json:"allowed_hosts"`
	APIKeySet     bool     `json:"api_key_set"`
	Complete      bool     `json:"complete"`
	MissingFields []string `json:"missing_fields"`
}

// AutomationProviderConfigInput 是安全 Provider 配置写入 DTO。
// APIKey 省略/为空表示保留当前值；ClearAPIKey=true 显式清除；两者同时为 true 返回 400。
type AutomationProviderConfigInput struct {
	BaseURL      string
	Model        string
	AllowedHosts []string
	APIKey       string
	ClearAPIKey  bool
}

// AutomationProviderConfig 是 Automation 安全 facade 的入口，按 scope 区分读取/写入。
// 底层复用 Config storage/service/audit；不新增 Provider 表，不扩大通用 config allowlist。
type AutomationProviderConfig struct {
	svc   *Service
	scope AutomationScope
}

// NewAutomationProviderConfig 构造指定 scope 的 facade。
func NewAutomationProviderConfig(svc *Service, scope AutomationScope) AutomationProviderConfig {
	return AutomationProviderConfig{svc: svc, scope: scope}
}

// View 返回当前 scope 下的安全 Provider 配置摘要。
// Workspace scope：只读 workspace/default；Project scope：保留 project > workspace > default。
func (f AutomationProviderConfig) View() (AutomationProviderConfigView, error) {
	if err := f.svc.requireProviderConfigRead(f.scope); err != nil {
		return AutomationProviderConfigView{}, err
	}
	scope, err := f.svc.normalizeAutomationScope(f.scope)
	if err != nil {
		return AutomationProviderConfigView{}, err
	}

	baseURL, err := f.resolveProviderValue(scope, defaultProviderBaseURLKey)
	if err != nil {
		return AutomationProviderConfigView{}, err
	}
	apiKey, err := f.resolveProviderValue(scope, defaultProviderAPIKeyKey)
	if err != nil {
		return AutomationProviderConfigView{}, err
	}
	model, err := f.resolveProviderValue(scope, defaultProviderModelKey)
	if err != nil {
		return AutomationProviderConfigView{}, err
	}
	allowedRaw, err := f.resolveProviderValue(scope, defaultProviderAllowedHostsKey)
	if err != nil {
		return AutomationProviderConfigView{}, err
	}
	allowedHosts, err := decodeProviderAllowedHosts(allowedRaw)
	if err != nil {
		return AutomationProviderConfigView{}, err
	}

	missing := make([]string, 0, 3)
	if strings.TrimSpace(baseURL) == "" {
		missing = append(missing, "base_url")
	}
	if strings.TrimSpace(apiKey) == "" {
		missing = append(missing, "api_key")
	}
	if strings.TrimSpace(model) == "" {
		missing = append(missing, "model")
	}
	return AutomationProviderConfigView{
		BaseURL:       strings.TrimSpace(baseURL),
		Model:         strings.TrimSpace(model),
		AllowedHosts:  allowedHosts,
		APIKeySet:     strings.TrimSpace(apiKey) != "",
		Complete:      len(missing) == 0,
		MissingFields: missing,
	}, nil
}

// Update 写入 Provider 配置；api_key 省略/为空保留原值，clear_api_key 显式清除。
func (f AutomationProviderConfig) Update(input AutomationProviderConfigInput) (AutomationProviderConfigView, error) {
	if err := f.svc.requireProviderConfigWrite(f.scope); err != nil {
		return AutomationProviderConfigView{}, err
	}
	scope, err := f.svc.normalizeAutomationScope(f.scope)
	if err != nil {
		return AutomationProviderConfigView{}, err
	}
	if scope.Type == AutomationScopeProject {
		project, err := f.svc.ResolveProject(scope.ID)
		if err != nil {
			return AutomationProviderConfigView{}, err
		}
		if err := ensureProjectConfigWritable(project); err != nil {
			return AutomationProviderConfigView{}, err
		}
	}
	trimmedKey := strings.TrimSpace(input.APIKey)
	if input.ClearAPIKey && trimmedKey != "" {
		return AutomationProviderConfigView{}, RuntimeError{Code: "automation_provider_config_invalid", Message: "cannot set api_key and clear_api_key together"}
	}
	if err := f.validateProviderConfigInput(input); err != nil {
		return AutomationProviderConfigView{}, err
	}

	err = f.svc.withAudit("automation.provider_config.set", func(tx *Service) (AuditEntry, error) {
		if err := tx.setProviderConfigValue(scope, defaultProviderBaseURLKey, strings.TrimSpace(input.BaseURL), true); err != nil {
			return AuditEntry{}, err
		}
		if err := tx.setProviderConfigValue(scope, defaultProviderModelKey, strings.TrimSpace(input.Model), true); err != nil {
			return AuditEntry{}, err
		}
		allowedJSON := "[]"
		if len(input.AllowedHosts) > 0 {
			raw, err := json.Marshal(input.AllowedHosts)
			if err != nil {
				return AuditEntry{}, RuntimeError{Code: "automation_provider_config_invalid", Message: "allowed_hosts marshal failed"}
			}
			allowedJSON = string(raw)
		}
		if err := tx.setProviderConfigValue(scope, defaultProviderAllowedHostsKey, allowedJSON, true); err != nil {
			return AuditEntry{}, err
		}
		// api_key 处理：clear 显式 unset；非空 set；省略保留。
		if input.ClearAPIKey {
			if err := tx.unsetProviderConfigValue(scope, defaultProviderAPIKeyKey); err != nil {
				return AuditEntry{}, err
			}
		} else if trimmedKey != "" {
			if err := tx.setProviderConfigValue(scope, defaultProviderAPIKeyKey, trimmedKey, true); err != nil {
				return AuditEntry{}, err
			}
		}
		entry := AuditEntry{Action: "automation.provider_config.set", TargetType: "automation_provider_config"}
		ws := tx.workspaceID
		entry.WorkspaceID = &ws
		if scope.Type == AutomationScopeProject {
			projectID := scope.ID
			entry.ProjectID = &projectID
		}
		payload := map[string]any{
			"scope_type":      string(scope.Type),
			"scope_id":        scope.ID,
			"changed":         []string{"base_url", "model", "allowed_hosts"},
			"api_key_changed": input.ClearAPIKey || trimmedKey != "",
		}
		entry.Payload = payload
		return entry, nil
	})
	if err != nil {
		return AutomationProviderConfigView{}, err
	}
	return f.View()
}

// validateProviderConfigInput 只校验非空字段，空值表示“保留原值”。
// base_url/model 留空时跳过校验，最终在 View().Complete 里反映为 missing。
func (f AutomationProviderConfig) validateProviderConfigInput(input AutomationProviderConfigInput) error {
	if trimmed := strings.TrimSpace(input.BaseURL); trimmed != "" {
		if !looksLikeHTTPURL(trimmed) {
			return RuntimeError{Code: "automation_provider_config_invalid", Message: "base_url must be a http(s) URL"}
		}
	}
	for _, host := range input.AllowedHosts {
		if strings.TrimSpace(host) == "" {
			return RuntimeError{Code: "automation_provider_config_invalid", Message: "allowed_hosts must not contain empty value"}
		}
	}
	if trimmed := strings.TrimSpace(input.APIKey); trimmed != "" && len(trimmed) > agentConfigValueMaxBytes {
		return RuntimeError{Code: "automation_provider_config_invalid", Message: "api_key too large"}
	}
	return nil
}

// resolveProviderValue 按 scope 解析单个 Provider config 值。
// Workspace：只读 workspace/default；Project：保留 project > workspace > default。
func (f AutomationProviderConfig) resolveProviderValue(scope AutomationScope, key string) (string, error) {
	normalizedKey, err := normalizeScopedConfigKey(key)
	if err != nil {
		return "", err
	}
	if scope.Type == AutomationScopeProject {
		return f.svc.projectAutomationEffectiveConfigValue(scope.ID, normalizedKey)
	}
	return f.svc.workspaceAutomationConfigValue(normalizedKey)
}

// setProviderConfigValue 写入指定 scope 的 config 值，复用 schema validation/secret encryption。
// audit 由调用方在外层 withAudit 事务里统一记录，避免每个 key 重复产生 config.set audit。
func (s *Service) setProviderConfigValue(scope AutomationScope, key, value string, skipAudit bool) error {
	normalizedKey, err := normalizeScopedConfigKey(key)
	if err != nil {
		return err
	}
	def, err := s.scopedConfigDefinition(normalizedKey)
	if err != nil {
		return err
	}
	storageScope := storage.ConfigScopeWorkspace
	scopeID := s.workspaceID
	if scope.Type == AutomationScopeProject {
		if !configDefinitionAllowsScope(def, storage.ConfigScopeProject) {
			return RuntimeError{Code: "config_scope_not_allowed", Message: fmt.Sprintf("config key %q does not allow project scope", normalizedKey)}
		}
		storageScope = storage.ConfigScopeProject
		scopeID = scope.ID
	} else if !configDefinitionAllowsScope(def, storage.ConfigScopeWorkspace) {
		return RuntimeError{Code: "config_scope_not_allowed", Message: fmt.Sprintf("config key %q does not allow workspace scope", normalizedKey)}
	}
	normalizedValue, err := s.validateScopedConfigValue(def, storageScope, value)
	if err != nil {
		return err
	}
	return s.configRepo.Set(storage.ConfigKey{
		WorkspaceID: s.workspaceID,
		Scope:       storageScope,
		ScopeID:     scopeID,
		Key:         normalizedKey,
	}, normalizedValue)
}

// unsetProviderConfigValue 删除指定 scope 的 config 值。
func (s *Service) unsetProviderConfigValue(scope AutomationScope, key string) error {
	normalizedKey, err := normalizeScopedConfigKey(key)
	if err != nil {
		return err
	}
	storageScope := storage.ConfigScopeWorkspace
	scopeID := s.workspaceID
	if scope.Type == AutomationScopeProject {
		storageScope = storage.ConfigScopeProject
		scopeID = scope.ID
	}
	return s.configRepo.Unset(storage.ConfigKey{
		WorkspaceID: s.workspaceID,
		Scope:       storageScope,
		ScopeID:     scopeID,
		Key:         normalizedKey,
	})
}

// requireProviderConfigRead 检查 Provider config 读权限。
// 与 Automation rule 权限独立：复用 config:read + workspace.read/modify。
func (s *Service) requireProviderConfigRead(scope AutomationScope) error {
	switch scope.Type {
	case AutomationScopeWorkspace:
		if err := s.Require(PermissionWorkspaceRead); err != nil {
			return err
		}
		return s.Require(PermissionConfigSchemaRead)
	default:
		if err := s.Require(PermissionProjectConfigRead); err != nil {
			return err
		}
		return s.Require(PermissionProjectRead)
	}
}

// requireProviderConfigWrite 检查 Provider config 写权限。
func (s *Service) requireProviderConfigWrite(scope AutomationScope) error {
	switch scope.Type {
	case AutomationScopeWorkspace:
		if err := s.Require(PermissionWorkspaceModify); err != nil {
			return err
		}
		return s.Require(PermissionConfigSchemaWrite)
	default:
		if err := s.Require(PermissionProjectConfigWrite); err != nil {
			return err
		}
		return s.Require(PermissionProjectManage)
	}
}

// decodeProviderAllowedHosts 解析 allowed_hosts JSON；空/null 视为未配置。
func decodeProviderAllowedHosts(raw string) ([]string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" || trimmed == "[]" {
		return []string{}, nil
	}
	var out []string
	if err := json.Unmarshal([]byte(trimmed), &out); err != nil {
		return nil, RuntimeError{Code: "automation_provider_allowed_hosts_invalid", Message: "agent.provider.allowed_hosts must be a JSON string array"}
	}
	return out, nil
}

// looksLikeHTTPURL 是轻量 URL 校验，避免引入额外依赖；真实 SSRF 校验由
// validateResolvedNotificationURL 在投递/preview 时完成。
func looksLikeHTTPURL(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return false
	}
	rest := lower[strings.Index(lower, "://")+3:]
	return strings.TrimSpace(rest) != ""
}
