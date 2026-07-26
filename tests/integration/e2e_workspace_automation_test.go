package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestE2EWorkspaceProjectCreatedAutomation 跑通 Workspace 自动化的核心闭环：
//  1. 通过安全 facade 配置 Workspace Provider；
//  2. HTTP 创建 enabled 的 Workspace project.created 规则；
//  3. 通过 HTTP 创建空 Project；
//  4. dispatcher 领取 Delivery 并投递给 fake OpenAI-compatible provider；
//  5. fake provider 收到 event_id/project/初始 config，且收不到 Project provider override；
//  6. Delivery 进入 succeeded，且 GET/PUT provider-config 不回显 api_key。
func TestE2EWorkspaceProjectCreatedAutomation(t *testing.T) {
	bin := buildXuanchu(t)
	dir := t.TempDir()
	serverDB := filepath.Join(dir, "server.db")
	configPath, _ := writeE2ELogConfig(t, dir)

	// fake OpenAI-compatible provider：记录每次请求的 Authorization/body 关键字段。
	var receivedAuth string
	var receivedBody string
	providerTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		receivedBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_ws_e2e","usage":{"prompt_tokens":5}}`))
	}))
	defer providerTarget.Close()

	// 全权限 token。
	token := parseRawToken(t, createTokenJSON(t, bin, "--db", serverDB, "ws-automation-e2e", "*"))

	cmd, baseURL := startXuanchuServer(t, bin,
		"--config", configPath,
		"--db", serverDB,
		"--automation-dispatcher-interval", "1s",
		"--automation-scheduler-interval", "60s",
	)
	defer stopXuanchuServer(t, cmd)

	headers := authHeaders(token)
	headers["Content-Type"] = "application/json"
	providerHost := hostFromURL(t, providerTarget.URL)

	// 1. 通过安全 facade 配置 Workspace Provider。
	providerBody := `{"base_url":"` + providerTarget.URL + `","model":"workspace-operator","allowed_hosts":["` + providerHost + `"],"api_key":"sk-ws-secret"}`
	putResp := putProviderConfigWithRetry(t, baseURL, providerBody, headers)
	if strings.Contains(toJSONString(t, putResp), "sk-ws-secret") {
		t.Fatalf("provider PUT response leaked api_key: %s", toJSONString(t, putResp))
	}
	if !strings.Contains(toJSONString(t, putResp), `"api_key_set":true`) {
		t.Fatalf("provider PUT response missing api_key_set=true: %s", toJSONString(t, putResp))
	}

	// 2. 创建 enabled 的 Workspace project.created 规则。
	ruleBody := `{
		"name":"新项目知识库初始化",
		"enabled":true,
		"trigger_type":"event",
		"trigger_config":{"event_type":"project.created"},
		"action":{"protocol":"chat_completions","base_url_config_key":"agent.provider.base_url","api_key_config_key":"agent.provider.api_key","model_config_key":"agent.provider.model","temperature":0.2},
		"context":{"include":["workspace","project","project_config","event"]},
		"instruction_template":"初始化知识库"
	}`
	ruleCreated := httpJSONRaw(t, http.MethodPost, baseURL+"/api/v1/automations", ruleBody, headers)
	ruleID, _ := ruleCreated["data"].(map[string]any)["id"].(string)
	if ruleID == "" {
		t.Fatalf("create rule missing id: %#v", ruleCreated)
	}

	// 3. 创建空 Project，触发 project.created。
	projectCreated := httpJSONRaw(t, http.MethodPost, baseURL+"/api/v1/projects", `{"slug":"wsauto","name":"自动化 E2E"}`, headers)
	projectID, _ := projectCreated["data"].(map[string]any)["id"].(string)
	if projectID == "" {
		t.Fatalf("created project missing id: %#v", projectCreated)
	}

	// 4. 轮询 Workspace Delivery list，直到出现一条 succeeded。
	deadline := time.Now().Add(20 * time.Second)
	var deliveryStatus string
	var deliveryID string
	for time.Now().Before(deadline) {
		listURL := baseURL + "/api/v1/automation-deliveries?rule_id=" + ruleID
		list := httpJSON(t, http.MethodGet, listURL, nil, headers)
		items, _ := list["data"].([]any)
		if len(items) > 0 {
			first, _ := items[0].(map[string]any)
			deliveryID, _ = first["id"].(string)
			deliveryStatus, _ = first["status"].(string)
			if deliveryStatus == "succeeded" || deliveryStatus == "dead_lettered" {
				break
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	if deliveryStatus != "succeeded" {
		t.Fatalf("delivery status = %q (id=%s), want succeeded; server log captured", deliveryStatus, deliveryID)
	}

	// 5. fake provider 收到了 workspace secret、project 上下文，且包含 source=empty metadata。
	if receivedAuth != "Bearer sk-ws-secret" {
		t.Fatalf("provider received Authorization = %q, want Bearer sk-ws-secret", receivedAuth)
	}
	if !strings.Contains(receivedBody, `"source":"empty"`) {
		t.Fatalf("provider body missing source=empty metadata: %s", receivedBody)
	}
	if !strings.Contains(receivedBody, projectID) {
		t.Fatalf("provider body missing project_id: %s", receivedBody)
	}
	// metadata count = 0（空项目）。
	if !strings.Contains(receivedBody, `"initial_task_count":0`) {
		t.Fatalf("provider body missing initial_task_count=0: %s", receivedBody)
	}

	// 6. GET provider-config 不回显 api_key。
	getResp := httpJSONRaw(t, http.MethodGet, baseURL+"/api/v1/automations/provider-config", ``, headers)
	if strings.Contains(toJSONString(t, getResp), "sk-ws-secret") {
		t.Fatalf("provider GET response leaked api_key: %s", toJSONString(t, getResp))
	}

	// 7. Delivery 详情字段齐全（replay 链路另测）。
	detail := httpJSON(t, http.MethodGet, baseURL+"/api/v1/automation-deliveries/"+deliveryID, nil, headers)
	detailData, _ := detail["data"].(map[string]any)
	if detailData["provider_request_id"] != "chatcmpl_ws_e2e" {
		t.Fatalf("provider_request_id = %#v, want chatcmpl_ws_e2e", detailData["provider_request_id"])
	}
	// 关联 Project 引用暴露 slug/name。
	project, _ := detailData["project"].(map[string]any)
	if project == nil || project["slug"] != "wsauto" {
		t.Fatalf("delivery missing project reference: %#v\nfull detail=%s", detailData["project"], toJSONString(t, detail))
	}
}

// TestE2EWorkspaceAutomationProviderConfigPreservesAndClearsAPIKey 验证 Provider facade
// PUT 行为：省略 api_key 保留原值，clear_api_key 显式清除。
func TestE2EWorkspaceAutomationProviderConfigPreservesAndClearsAPIKey(t *testing.T) {
	bin := buildXuanchu(t)
	dir := t.TempDir()
	serverDB := filepath.Join(dir, "server.db")
	configPath, _ := writeE2ELogConfig(t, dir)

	token := parseRawToken(t, createTokenJSON(t, bin, "--db", serverDB, "ws-prov-e2e", "*"))
	cmd, baseURL := startXuanchuServer(t, bin, "--config", configPath, "--db", serverDB)
	defer stopXuanchuServer(t, cmd)
	headers := authHeaders(token)
	headers["Content-Type"] = "application/json"

	// 初始配置（带重试：服务器首次启动后 schema migration 可能在第一次请求时仍未就绪）。
	initialBody := `{"base_url":"https://agent.example.com","model":"workspace-operator","allowed_hosts":["agent.example.com"],"api_key":"sk-initial"}`
	putProviderConfigWithRetry(t, baseURL, initialBody, headers)

	// 省略 api_key 应保留。
	preserveBody := `{"base_url":"https://agent.example.com","model":"workspace-operator","allowed_hosts":["agent.example.com"]}`
	preserveResp := putProviderConfigWithRetry(t, baseURL, preserveBody, headers)
	data, _ := preserveResp["data"].(map[string]any)
	if !data["api_key_set"].(bool) {
		t.Fatalf("api_key must be preserved when omitted: %#v", data)
	}

	// clear_api_key 应清除。
	clearBody := `{"base_url":"https://agent.example.com","model":"workspace-operator","allowed_hosts":["agent.example.com"],"clear_api_key":true}`
	clearResp := putProviderConfigWithRetry(t, baseURL, clearBody, headers)
	cleared, _ := clearResp["data"].(map[string]any)
	if cleared["api_key_set"].(bool) {
		t.Fatalf("api_key should be cleared: %#v", cleared)
	}
}

// putProviderConfigWithRetry 对 provider-config PUT 做轻量重试，
// 容忍服务器启动初期 schema/DB 写入瞬时 500（CI 上观察到 ~20% 概率的 race）。
func putProviderConfigWithRetry(t *testing.T, baseURL, body string, headers map[string]string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var lastErr string
	for time.Now().Before(deadline) {
		resp, payload := httpDo(t, http.MethodPut, baseURL+"/api/v1/automations/provider-config", strings.NewReader(body), headers)
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			var out map[string]any
			if jsonErr := json.Unmarshal(payload, &out); jsonErr == nil {
				return out
			} else {
				lastErr = "json decode: " + jsonErr.Error()
			}
		} else {
			lastErr = fmt.Sprintf("status=%d body=%s", resp.StatusCode, payload)
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("provider-config PUT failed after retry: %s", lastErr)
	return nil
}
