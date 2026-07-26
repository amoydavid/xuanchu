package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/app"
)

// setupHTTPWorkspaceAutomationProviderConfig 通过 service 层在 workspace scope 配置 Provider。
func setupHTTPWorkspaceAutomationProviderConfig(t *testing.T, fixture httpTokenFixture) {
	t.Helper()
	ws, err := fixture.server.store.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace: %v", err)
	}
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store, ActorRef: "local", WorkspaceRef: "local"})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	defs := []struct {
		key    string
		secret bool
		value  string
	}{
		{"agent.provider.base_url", false, "https://agent.example.com"},
		{"agent.provider.api_key", true, "sk-workspace-secret"},
		{"agent.provider.model", false, "workspace-operator"},
		{"agent.provider.allowed_hosts", false, `["agent.example.com"]`},
	}
	for _, def := range defs {
		valueType := "string"
		if def.key == "agent.provider.allowed_hosts" {
			valueType = "json"
		}
		if err := svc.ConfigSchemaSet(app.ConfigSchemaInput{
			Key: def.key, ValueType: valueType, AllowedScopes: []string{"workspace", "project"}, Secret: def.secret,
		}); err != nil {
			t.Fatalf("ConfigSchemaSet(%s): %v", def.key, err)
		}
		// 直接写 workspace scope（不通过 ProjectConfigSet，避免触发 project 校验）。
		if setErr := fixture.server.store.DB().Exec(
			"INSERT INTO configs(workspace_id, scope, scope_id, key, value) VALUES(?, 'workspace', ?, ?, ?) ON CONFLICT(workspace_id, scope, scope_id, key) DO UPDATE SET value = excluded.value",
			ws.ID, ws.ID, def.key, def.value,
		).Error; setErr != nil {
			t.Fatalf("set workspace config %s: %v", def.key, setErr)
		}
	}
}

func TestHTTPWorkspaceAutomationLifecycle(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:read", "workspace:write", "hook:read", "hook:write")
	setupHTTPWorkspaceAutomationProviderConfig(t, fixture)
	body := `{
		"name":"新项目知识库初始化",
		"enabled":true,
		"trigger_type":"event",
		"trigger_config":{"event_type":"project.created"},
		"action":{"protocol":"chat_completions","base_url_config_key":"agent.provider.base_url","api_key_config_key":"agent.provider.api_key","model_config_key":"agent.provider.model","temperature":0.2},
		"context":{"include":["workspace","project","project_config","event"]},
		"instruction_template":"初始化知识库"
	}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/automations", body, restfulFilterHeader(fixture.token))
	if rr.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", rr.Code, rr.Body.String())
	}
	created := httpResponseDataMap(t, rr)
	if created["scope_type"] != "workspace" {
		t.Fatalf("created scope_type = %#v", created["scope_type"])
	}
	if _, ok := created["project_id"]; ok {
		t.Fatalf("workspace rule should not return project_id: %#v", created)
	}
	ruleID := created["id"].(string)

	rr = requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/automations", restfulFilterHeader(fixture.token))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), ruleID) {
		t.Fatalf("list status=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/automations/"+ruleID, restfulFilterHeader(fixture.token))
	if rr.Code != http.StatusOK {
		t.Fatalf("info status=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/automations/"+ruleID+"/disable", `{}`, restfulFilterHeader(fixture.token))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"enabled":false`) {
		t.Fatalf("disable status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHTTPWorkspaceAutomationRejectsProjectOnlyToken(t *testing.T) {
	// 只有 project scope 的 token 不能访问 workspace automation。
	fixture := newHTTPServerWithTokenFixture(t, "project:read", "project:write", "hook:read", "hook:write")
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/automations", restfulFilterHeader(fixture.token))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (missing workspace scope); body=%s", rr.Code, rr.Body.String())
	}
}

func TestHTTPWorkspaceAutomationRequiresHookScope(t *testing.T) {
	// 有 workspace:read/write 但没有 hook 的 token 应该被拒绝。
	fixture := newHTTPServerWithTokenFixture(t, "workspace:read", "workspace:write")
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/automations", restfulFilterHeader(fixture.token))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (missing hook scope); body=%s", rr.Code, rr.Body.String())
	}
}

func TestHTTPWorkspaceAutomationTemplateVars(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:read", "workspace:write", "hook:read", "hook:write")
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/automations/template-vars", restfulFilterHeader(fixture.token))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "project_config") {
		t.Fatalf("template vars body should describe project_config: %s", rr.Body.String())
	}
	// 不应回显具体 secret value 或 api_key 字段值。
	if strings.Contains(rr.Body.String(), "sk-") || strings.Contains(rr.Body.String(), "agent.provider.api_key") {
		t.Fatalf("template vars should not reference api_key: %s", rr.Body.String())
	}
}

func TestHTTPWorkspaceAutomationProviderConfigFacade(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:read", "workspace:write", "config:read", "config:write")
	setupHTTPWorkspaceAutomationProviderConfig(t, fixture)
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/automations/provider-config", restfulFilterHeader(fixture.token))
	if rr.Code != http.StatusOK {
		t.Fatalf("get status = %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if strings.Contains(body, "sk-workspace-secret") {
		t.Fatalf("GET response leaked api_key: %s", body)
	}
	if !strings.Contains(body, `"api_key_set":true`) || !strings.Contains(body, `"complete":true`) {
		t.Fatalf("GET response missing safe fields: %s", body)
	}

	// PUT：省略 api_key 必须保留 secret。
	putBody := `{"base_url":"https://agent.example.com","model":"workspace-operator","allowed_hosts":["agent.example.com"]}`
	rr = requestHTTPBody(t, fixture.server, http.MethodPut, "/api/v1/automations/provider-config", putBody, restfulFilterHeader(fixture.token))
	if rr.Code != http.StatusOK {
		t.Fatalf("put status = %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"api_key_set":true`) {
		t.Fatalf("PUT must preserve api_key_set=true: %s", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "sk-workspace-secret") {
		t.Fatalf("PUT response leaked api_key: %s", rr.Body.String())
	}
}

func TestHTTPProjectAutomationProviderConfigFacade(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "project:read", "project:write", "hook:read", "hook:write", "config:read", "config:write")
	setupHTTPProjectAutomationConfig(t, fixture, "adsops")
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/projects/adsops/automations/provider-config", restfulFilterHeader(fixture.token))
	if rr.Code != http.StatusOK {
		t.Fatalf("get status = %d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "sk-real-secret") {
		t.Fatalf("Project Provider facade leaked api_key: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"api_key_set":true`) {
		t.Fatalf("Project Provider facade missing api_key_set: %s", rr.Body.String())
	}
}

func TestHTTPWorkspaceAutomationDeliveriesListAndReplay(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:read", "workspace:write", "hook:read", "hook:write")
	ws, err := fixture.server.store.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace: %v", err)
	}
	// 直接注入一条 dead_lettered Delivery 模拟历史记录。
	if err := fixture.server.store.DB().Exec(`INSERT INTO automation_deliveries(
		id, workspace_id, rule_scope_type, rule_scope_id, rule_id, trigger_type, dedupe_key, status,
		resolved_url, rendered_method, rendered_headers_json, request_body_json, request_body_preview,
		request_body_hash, response_body_preview, provider_request_id, usage_json, attempt_count,
		max_attempts, last_error, created_at, modified_at)
		VALUES('dlv-1', ?, 'workspace', ?, 'rule-x', 'schedule', 'k1', 'dead_lettered',
		'https://x', 'POST', '{}', '', '', '', '', '', '{}', 1, 3, 'err', 100, 100)`, ws.ID, ws.ID).Error; err != nil {
		t.Fatalf("seed delivery: %v", err)
	}
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/automation-deliveries", restfulFilterHeader(fixture.token))
	if rr.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "dlv-1") {
		t.Fatalf("list body missing delivery: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"total":1`) {
		t.Fatalf("list body missing total meta: %s", rr.Body.String())
	}

	// Info。
	rr = requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/automation-deliveries/dlv-1", restfulFilterHeader(fixture.token))
	if rr.Code != http.StatusOK {
		t.Fatalf("info status=%d body=%s", rr.Code, rr.Body.String())
	}

	// Replay。
	rr = requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/automation-deliveries/dlv-1/replay", `{}`, restfulFilterHeader(fixture.token))
	if rr.Code != http.StatusOK {
		t.Fatalf("replay status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"replay_of_delivery_id":"dlv-1"`) {
		t.Fatalf("replay response missing replay_of: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"status":"queued"`) {
		t.Fatalf("replay response should be queued: %s", rr.Body.String())
	}
}
