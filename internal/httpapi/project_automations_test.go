package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/app"
)

// setupHTTPProjectAutomationConfig 通过 service 层创建项目并定义 provider config，供 HTTP 测试复用。
func setupHTTPProjectAutomationConfig(t *testing.T, fixture httpTokenFixture, projectSlug string) {
	t.Helper()
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store, ActorRef: "local", WorkspaceRef: "local"})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if _, err := svc.AddProject(app.AddProjectInput{Slug: projectSlug, Name: projectSlug}); err != nil {
		t.Fatalf("AddProject(%s): %v", projectSlug, err)
	}
	defs := []struct {
		key    string
		secret bool
		value  string
	}{
		{"agent.provider.base_url", false, "https://agent.example.com"},
		{"agent.provider.api_key", true, "sk-real-secret"},
		{"agent.provider.model", false, "project-operator"},
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
		if err := svc.ProjectConfigSet(projectSlug, def.key, def.value); err != nil {
			t.Fatalf("ProjectConfigSet(%s): %v", def.key, err)
		}
	}
}

func TestHTTPProjectAutomationPreviewMasksSecret(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "project:read", "project:write", "hook:read", "hook:write", "config:read", "config:write", "task:read")
	setupHTTPProjectAutomationConfig(t, fixture, "adsops")
	body := `{
		"name":"每日项目巡检",
		"enabled":true,
		"trigger_type":"schedule",
		"trigger_config":{"schedule_type":"daily_at","schedule_value":"09:30","timezone":"Asia/Shanghai"},
		"action":{"protocol":"chat_completions","base_url_config_key":"agent.provider.base_url","api_key_config_key":"agent.provider.api_key","model_config_key":"agent.provider.model","temperature":0.2},
		"context":{"include":["workspace","project","project_config"]},
		"instruction_template":"生成巡检"
	}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/projects/adsops/automations/preview", body, restfulFilterHeader(fixture.token))
	if rr.Code != http.StatusOK {
		t.Fatalf("preview status = %d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "sk-real-secret") {
		t.Fatalf("preview leaked secret: %s", rr.Body.String())
	}
	for _, want := range []string{`"method":"POST"`, `"url":"https://agent.example.com/v1/chat/completions"`, `"Authorization":"Bearer ****"`, `"model":"project-operator"`} {
		if !strings.Contains(rr.Body.String(), want) {
			t.Fatalf("response missing %s: %s", want, rr.Body.String())
		}
	}
}

func TestHTTPProjectAutomationLifecycle(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "project:read", "project:write", "hook:read", "hook:write")
	setupHTTPProjectAutomationConfig(t, fixture, "adsops")
	body := `{
		"name":"分配任务后拉群",
		"enabled":true,
		"trigger_type":"event",
		"trigger_config":{"event_type":"task.assigned"},
		"condition":{"only_added_assignees":true},
		"action":{"protocol":"chat_completions","base_url_config_key":"agent.provider.base_url","api_key_config_key":"agent.provider.api_key","model_config_key":"agent.provider.model","temperature":0.2},
		"context":{"include":["event","task","added_assignees","project","project_config"]},
		"instruction_template":"处理新增负责人"
	}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/projects/adsops/automations", body, restfulFilterHeader(fixture.token))
	if rr.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", rr.Code, rr.Body.String())
	}
	created := httpResponseDataMap(t, rr)
	if created["trigger_type"] != "event" || created["action_type"] != "openai_compatible" {
		t.Fatalf("created = %#v", created)
	}
	ruleID := created["id"].(string)
	rr = requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/projects/adsops/automations", restfulFilterHeader(fixture.token))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), ruleID) {
		t.Fatalf("list status=%d body=%s", rr.Code, rr.Body.String())
	}
	rr = requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/projects/adsops/automations/"+ruleID+"/disable", `{}`, restfulFilterHeader(fixture.token))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"enabled":false`) {
		t.Fatalf("disable status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHTTPProjectAutomationRequiresProjectAndHookScopes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		scopes []string
	}{
		{name: "project-only", scopes: []string{"project:read", "project:write"}},
		{name: "hook-only", scopes: []string{"hook:read", "hook:write"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newHTTPServerWithTokenFixture(t, tc.scopes...)
			setupHTTPProjectAutomationConfig(t, fixture, "adsops")
			rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/projects/adsops/automations", restfulFilterHeader(fixture.token))
			if rr.Code != http.StatusForbidden {
				t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
			}
		})
	}
}
