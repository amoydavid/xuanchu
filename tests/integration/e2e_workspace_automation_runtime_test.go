package integration

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

// TestE2EWorkspaceAutomationServerGenericRuntime 验证 server 只启动一个 generic
// scheduler/dispatcher，且该 runtime 同时处理 workspace scope 与 project scope 规则。
//
// server wiring 只创建单个 automationScheduler + 单个 automationDispatcher（见
// internal/cli/server.go），不存在 workspace/project 两套 worker。本测试通过真实
// server 端到端验证：workspace event 规则和 project schedule 规则在同一 server
// 进程内都能被正确路由和投递。
func TestE2EWorkspaceAutomationServerGenericRuntime(t *testing.T) {
	bin := buildXuanchu(t)
	dir := t.TempDir()
	serverDB := filepath.Join(dir, "server.db")
	configPath, _ := writeE2ELogConfig(t, dir)

	// fake OpenAI-compatible provider。
	providerTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_runtime","usage":{"prompt_tokens":1}}`))
	}))
	defer providerTarget.Close()
	providerHost := hostFromURL(t, providerTarget.URL)

	// admin token 治理自动化 + 配置 + 创建项目。
	adminToken := parseRawToken(t, createTokenJSON(t, bin, "--db", serverDB, "runtime-admin", "*"))

	cmd, baseURL := startXuanchuServer(t, bin,
		"--config", configPath,
		"--db", serverDB,
		"--automation-dispatcher-interval", "1s",
		"--automation-scheduler-interval", "60s",
	)
	defer stopXuanchuServer(t, cmd)
	headers := authHeaders(adminToken)
	headers["Content-Type"] = "application/json"

	// 1. Workspace Provider。
	providerBody := `{"base_url":"` + providerTarget.URL + `","model":"workspace-operator","allowed_hosts":["` + providerHost + `"],"api_key":"sk-ws"}`
	httpJSONRaw(t, http.MethodPut, baseURL+"/api/v1/automations/provider-config", providerBody, headers)

	// 2. Workspace project.created 规则。
	wsRuleBody := `{
		"name":"ws event rule",
		"enabled":true,
		"trigger_type":"event",
		"trigger_config":{"event_type":"project.created"},
		"action":{"protocol":"chat_completions","base_url_config_key":"agent.provider.base_url","api_key_config_key":"agent.provider.api_key","model_config_key":"agent.provider.model","temperature":0.2},
		"context":{"include":["workspace","project"]},
		"instruction_template":"ws"
	}`
	wsRule := httpJSONRaw(t, http.MethodPost, baseURL+"/api/v1/automations", wsRuleBody, headers)
	wsRuleID, _ := wsRule["data"].(map[string]any)["id"].(string)
	if wsRuleID == "" {
		t.Fatalf("ws rule missing id: %#v", wsRule)
	}

	// 3. 创建项目：触发 workspace project.created（generic runtime 的 workspace path）。
	projectCreated := httpJSONRaw(t, http.MethodPost, baseURL+"/api/v1/projects", `{"slug":"rt1","name":"runtime 1"}`, headers)
	projectID, _ := projectCreated["data"].(map[string]any)["id"].(string)
	if projectID == "" {
		t.Fatalf("project missing id: %#v", projectCreated)
	}

	// 4. 在该项目下创建 project scope 规则并 test（generic runtime 的 project path）。
	// 先配 project provider。
	run(t, bin, "--db", serverDB, "--workspace", "local", "project", "config", "set", "rt1", "agent.provider.base_url", providerTarget.URL)
	run(t, bin, "--db", serverDB, "--workspace", "local", "project", "config", "set", "rt1", "agent.provider.api_key", "sk-proj")
	run(t, bin, "--db", serverDB, "--workspace", "local", "project", "config", "set", "rt1", "agent.provider.model", "project-operator")
	run(t, bin, "--db", serverDB, "--workspace", "local", "project", "config", "set", "rt1", "agent.provider.allowed_hosts", `["`+providerHost+`"]`)

	projRuleBody := `{
		"name":"proj event rule",
		"enabled":true,
		"trigger_type":"event",
		"trigger_config":{"event_type":"task.created"},
		"action":{"protocol":"chat_completions","base_url_config_key":"agent.provider.base_url","api_key_config_key":"agent.provider.api_key","model_config_key":"agent.provider.model","temperature":0.2},
		"context":{"include":["project"]},
		"instruction_template":"proj"
	}`
	projRule := httpJSONRaw(t, http.MethodPost, baseURL+"/api/v1/projects/rt1/automations", projRuleBody, headers)
	projRuleID, _ := projRule["data"].(map[string]any)["id"].(string)
	if projRuleID == "" {
		t.Fatalf("project rule missing id: %#v", projRule)
	}

	// test project rule：写入 manual_test delivery。
	testResp := httpJSONRaw(t, http.MethodPost, baseURL+"/api/v1/projects/rt1/automations/"+projRuleID+"/test", `{}`, headers)
	projDeliveryID, _ := testResp["data"].(map[string]any)["id"].(string)
	if projDeliveryID == "" {
		t.Fatalf("project test delivery missing id: %#v", testResp)
	}

	// 5. 轮询：workspace delivery（project.created 路径）+ project delivery（manual_test 路径）
	// 都由同一个 generic dispatcher 完成。
	deadline := time.Now().Add(20 * time.Second)
	var wsStatus, projStatus string
	for time.Now().Before(deadline) {
		if wsStatus != "succeeded" {
			wsList := httpJSON(t, http.MethodGet, baseURL+"/api/v1/automation-deliveries?rule_id="+wsRuleID, nil, headers)
			if items, _ := wsList["data"].([]any); len(items) > 0 {
				wsStatus, _ = items[0].(map[string]any)["status"].(string)
			}
		}
		if projStatus != "succeeded" {
			projDetail := httpJSON(t, http.MethodGet, baseURL+"/api/v1/projects/rt1/automation-deliveries/"+projDeliveryID, nil, headers)
			projStatus, _ = projDetail["data"].(map[string]any)["status"].(string)
		}
		if wsStatus == "succeeded" && projStatus == "succeeded" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if wsStatus != "succeeded" {
		t.Fatalf("workspace delivery status = %q, want succeeded (generic runtime workspace path)", wsStatus)
	}
	if projStatus != "succeeded" {
		t.Fatalf("project delivery status = %q, want succeeded (generic runtime project path)", projStatus)
	}
}
