package app

import (
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

// defineWorkspaceProviderConfigForTest 在 Workspace scope 写入完整 Provider config。
func defineWorkspaceProviderConfigForTest(t *testing.T, svc *Service, baseURL string) {
	t.Helper()
	defineWorkspaceConfigForTest(t, svc, "agent.provider.base_url", false, baseURL)
	defineWorkspaceConfigForTest(t, svc, "agent.provider.api_key", true, "sk-workspace-secret")
	defineWorkspaceConfigForTest(t, svc, "agent.provider.model", false, "workspace-operator")
	defineWorkspaceConfigForTest(t, svc, "agent.provider.allowed_hosts", false, `["agent.example.com"]`)
}

// defineWorkspaceConfigForTest 在 Workspace scope 写入单个 config 值。
func defineWorkspaceConfigForTest(t *testing.T, svc *Service, key string, secret bool, value string) {
	t.Helper()
	valueType := string(ConfigValueTypeString)
	if key == "agent.provider.allowed_hosts" {
		valueType = string(ConfigValueTypeJSON)
	}
	if err := svc.ConfigSchemaSet(ConfigSchemaInput{Key: key, ValueType: valueType, AllowedScopes: []string{string(ConfigAllowedScopeWorkspace), string(ConfigAllowedScopeProject)}, Secret: secret}); err != nil {
		t.Fatalf("ConfigSchemaSet(%s): %v", key, err)
	}
	if err := svc.configRepo.Set(workspaceScopedConfigKey(svc, key), value); err != nil {
		t.Fatalf("workspace config set(%s): %v", key, err)
	}
}

func workspaceScopedConfigKey(svc *Service, key string) (k storage.ConfigKey) {
	return storage.ConfigKey{WorkspaceID: svc.workspaceID, Scope: storage.ConfigScopeWorkspace, ScopeID: svc.workspaceID, Key: key}
}

func TestWorkspaceProviderConfigViewNeverExposesAPIKey(t *testing.T) {
	f := newWorkspaceAutomationFixture(t)
	defineWorkspaceProviderConfigForTest(t, f.svc, "https://agent.example.com")
	view, err := f.svc.WorkspaceAutomationProviderConfig().View()
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	if !view.APIKeySet || !view.Complete || len(view.MissingFields) != 0 {
		t.Fatalf("view = %#v, want complete with api_key_set=true", view)
	}
	if view.BaseURL != "https://agent.example.com" || view.Model != "workspace-operator" {
		t.Fatalf("view fields = %#v", view)
	}
	if !containsString(view.AllowedHosts, "agent.example.com") {
		t.Fatalf("allowed_hosts = %#v", view.AllowedHosts)
	}
}

func TestWorkspaceProviderConfigViewMissingFields(t *testing.T) {
	f := newWorkspaceAutomationFixture(t)
	// 只定义 base_url，缺 api_key/model。
	defineWorkspaceConfigForTest(t, f.svc, "agent.provider.base_url", false, "https://agent.example.com")
	view, err := f.svc.WorkspaceAutomationProviderConfig().View()
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	if view.Complete {
		t.Fatalf("view should not be complete: %#v", view)
	}
	if !containsString(view.MissingFields, "api_key") || !containsString(view.MissingFields, "model") {
		t.Fatalf("missing fields = %#v", view.MissingFields)
	}
}

func TestWorkspaceProviderConfigUpdatePreservesAPIKeyWhenOmitted(t *testing.T) {
	f := newWorkspaceAutomationFixture(t)
	defineWorkspaceProviderConfigForTest(t, f.svc, "https://agent.example.com")
	// Update base_url，省略 api_key；api_key 必须保留。
	updated, err := f.svc.WorkspaceAutomationProviderConfig().Update(AutomationProviderConfigInput{
		BaseURL:      "https://agent.v2.example.com",
		Model:        "workspace-operator",
		AllowedHosts: []string{"agent.v2.example.com"},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !updated.APIKeySet {
		t.Fatalf("api_key must be preserved when omitted: %#v", updated)
	}
	if updated.BaseURL != "https://agent.v2.example.com" {
		t.Fatalf("base_url not updated: %#v", updated)
	}
	// 直接读取底层 config 验证 secret 仍然存在。
	value, err := f.svc.workspaceAutomationConfigValue("agent.provider.api_key")
	if err != nil || value != "sk-workspace-secret" {
		t.Fatalf("api_key value lost: value=%q err=%v", value, err)
	}
}

func TestWorkspaceProviderConfigUpdateClearAPIKey(t *testing.T) {
	f := newWorkspaceAutomationFixture(t)
	defineWorkspaceProviderConfigForTest(t, f.svc, "https://agent.example.com")
	updated, err := f.svc.WorkspaceAutomationProviderConfig().Update(AutomationProviderConfigInput{
		BaseURL:      "https://agent.example.com",
		Model:        "workspace-operator",
		AllowedHosts: []string{"agent.example.com"},
		ClearAPIKey:  true,
	})
	if err != nil {
		t.Fatalf("Update clear: %v", err)
	}
	if updated.APIKeySet {
		t.Fatalf("api_key should be cleared: %#v", updated)
	}
}

func TestWorkspaceProviderConfigUpdateRejectsAPIKeyAndClearTogether(t *testing.T) {
	f := newWorkspaceAutomationFixture(t)
	defineWorkspaceProviderConfigForTest(t, f.svc, "https://agent.example.com")
	_, err := f.svc.WorkspaceAutomationProviderConfig().Update(AutomationProviderConfigInput{
		BaseURL:      "https://agent.example.com",
		Model:        "workspace-operator",
		AllowedHosts: []string{"agent.example.com"},
		APIKey:       "sk-new",
		ClearAPIKey:  true,
	})
	if code := runtimeErrorCode(err); code != "automation_provider_config_invalid" {
		t.Fatalf("err = %v, want automation_provider_config_invalid", err)
	}
	// 错误消息不得回显输入值。
	if err != nil && strings.Contains(err.Error(), "sk-new") {
		t.Fatalf("error leaked api key value: %v", err)
	}
}

func TestWorkspaceProviderConfigUpdateAcceptsNewAPIKey(t *testing.T) {
	f := newWorkspaceAutomationFixture(t)
	defineWorkspaceProviderConfigForTest(t, f.svc, "https://agent.example.com")
	if _, err := f.svc.WorkspaceAutomationProviderConfig().Update(AutomationProviderConfigInput{
		BaseURL:      "https://agent.example.com",
		Model:        "workspace-operator",
		AllowedHosts: []string{"agent.example.com"},
		APIKey:       "sk-rotated",
	}); err != nil {
		t.Fatalf("Update new key: %v", err)
	}
	value, err := f.svc.workspaceAutomationConfigValue("agent.provider.api_key")
	if err != nil || value != "sk-rotated" {
		t.Fatalf("api_key not rotated: value=%q err=%v", value, err)
	}
}

func TestProjectProviderConfigFacadeEffectiveResolution(t *testing.T) {
	f := newWorkspaceAutomationFixture(t)
	project, err := f.svc.AddProject(AddProjectInput{Slug: "adsops", Name: "广告投放"})
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	// Workspace scope 配置 Provider。
	defineWorkspaceProviderConfigForTest(t, f.svc, "https://agent.example.com")
	// Project 不覆盖，effective 应回落到 Workspace。
	view, err := f.svc.ProjectAutomationProviderConfig(project.ID).View()
	if err != nil {
		t.Fatalf("Project View: %v", err)
	}
	if view.BaseURL != "https://agent.example.com" || !view.APIKeySet {
		t.Fatalf("effective provider config = %#v", view)
	}
}
