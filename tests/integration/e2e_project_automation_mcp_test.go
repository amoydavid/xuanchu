package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestE2EProjectAutomationMCPTools 通过 HTTP MCP 跑通项目自动化管理全链路：
// MCP 建规则 -> 预览脱敏 -> 立即测试 -> dispatcher 投递成功 -> 投递查询/重放 -> 修改/停用/删除，
// 并验证 tools/list 暴露面与受限 token 的双 scope 拒绝。
func TestE2EProjectAutomationMCPTools(t *testing.T) {
	bin := buildXuanchu(t)
	dir := t.TempDir()
	serverDB := filepath.Join(dir, "server.db")
	configPath, _ := writeE2ELogConfig(t, dir)

	// httptest server 模拟 OpenAI 兼容 Agent Provider，记录收到的请求。
	var receivedAuth string
	var receivedModel string
	providerTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		if m, ok := body["model"].(string); ok {
			receivedModel = m
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_mcp_e2e","usage":{"prompt_tokens":12,"completion_tokens":4}}`))
	}))
	defer providerTarget.Close()

	// 预置项目和 provider config（agent.provider.* 是内置 schema，直接 set value）。
	run(t, bin, "--db", serverDB, "--workspace", "local", "project", "add", "adsops", "name:广告投放优化")
	run(t, bin, "--db", serverDB, "--workspace", "local", "project", "config", "set", "adsops", "agent.provider.base_url", providerTarget.URL)
	run(t, bin, "--db", serverDB, "--workspace", "local", "project", "config", "set", "adsops", "agent.provider.api_key", "sk-e2e-secret")
	run(t, bin, "--db", serverDB, "--workspace", "local", "project", "config", "set", "adsops", "agent.provider.model", "project-operator")
	providerHost := hostFromURL(t, providerTarget.URL)
	run(t, bin, "--db", serverDB, "--workspace", "local", "project", "config", "set", "adsops", "agent.provider.allowed_hosts", `["`+providerHost+`"]`)

	// 全权限 token + 只有 project:read 的受限 token（缺 hook scope，应被双 scope 校验拒绝）。
	adminToken := parseRawToken(t, createTokenJSON(t, bin, "--db", serverDB, "automation-mcp-e2e", "*"))
	limitedToken := parseRawToken(t, createTokenJSON(t, bin, "--db", serverDB, "automation-mcp-limited", "project:read"))

	cmd, baseURL := startXuanchuServer(t, bin,
		"--config", configPath,
		"--db", serverDB,
		"--automation-dispatcher-interval", "1s",
		"--automation-scheduler-interval", "60s",
	)
	defer stopXuanchuServer(t, cmd)

	session, cancel := connectHTTPMCP(t, baseURL, adminToken)
	defer cancel()

	// 0. tools/list 暴露 project_automation_* 全部 14 个工具。
	toolsResult, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	gotNames := map[string]bool{}
	for _, tool := range toolsResult.Tools {
		gotNames[tool.Name] = true
	}
	for _, name := range []string{
		"project_automation_list", "project_automation_get",
		"project_automation_add", "project_automation_modify", "project_automation_remove",
		"project_automation_enable", "project_automation_disable",
		"project_automation_test", "project_automation_preview", "project_automation_preview_saved",
		"project_automation_delivery_list", "project_automation_delivery_get", "project_automation_delivery_replay",
		"project_automation_list_template_vars",
	} {
		if !gotNames[name] {
			t.Fatalf("tools/list missing %q", name)
		}
	}

	ruleArgs := func(extra map[string]any) map[string]any {
		args := map[string]any{
			"workspace": "local",
			"project":   "adsops",
			"name":      "每日项目巡检",
			"enabled":   true,
			"trigger_type": "schedule",
			"trigger_config": map[string]any{
				"schedule_type":  "daily_at",
				"schedule_value": "09:30",
				"timezone":       "Asia/Shanghai",
			},
			"action": map[string]any{
				"protocol":           "chat_completions",
				"base_url_config_key": "agent.provider.base_url",
				"api_key_config_key":  "agent.provider.api_key",
				"model_config_key":    "agent.provider.model",
				"temperature":         0.2,
			},
			"instruction_template": "生成 {{project.name}} 巡检报告",
		}
		for key, value := range extra {
			args[key] = value
		}
		return args
	}

	// 1. MCP 创建 schedule 规则。
	added := callMCPTool(t, session, "project_automation_add", ruleArgs(nil))
	if added.IsError {
		t.Fatalf("project_automation_add error: %#v", added.StructuredContent)
	}
	rule := mcpDataMap(t, added)["automation"].(map[string]any)
	ruleID, _ := rule["id"].(string)
	if ruleID == "" {
		t.Fatalf("created rule missing id: %#v", rule)
	}

	// 2. preview_saved 脱敏：不含 secret 明文，Authorization 显示 Bearer ****。
	preview := callMCPTool(t, session, "project_automation_preview_saved", map[string]any{
		"workspace": "local", "project": "adsops", "rule_id": ruleID,
	})
	if preview.IsError {
		t.Fatalf("project_automation_preview_saved error: %#v", preview.StructuredContent)
	}
	previewJSON, _ := json.Marshal(mcpDataMap(t, preview)["preview"])
	if strings.Contains(string(previewJSON), "sk-e2e-secret") {
		t.Fatalf("preview leaked secret: %s", previewJSON)
	}
	if !strings.Contains(string(previewJSON), `"Authorization":"Bearer ****"`) {
		t.Fatalf("preview missing masked authorization: %s", previewJSON)
	}

	// 3. 立即测试 -> manual_test delivery 入队。
	tested := callMCPTool(t, session, "project_automation_test", map[string]any{
		"workspace": "local", "project": "adsops", "rule_id": ruleID,
	})
	if tested.IsError {
		t.Fatalf("project_automation_test error: %#v", tested.StructuredContent)
	}
	delivery := mcpDataMap(t, tested)["delivery"].(map[string]any)
	deliveryID, _ := delivery["id"].(string)
	if deliveryID == "" || delivery["trigger_type"] != "manual_test" {
		t.Fatalf("test delivery = %#v", delivery)
	}

	// 4. 轮询 delivery_get 直到 succeeded。
	deadline := time.Now().Add(15 * time.Second)
	var status string
	for time.Now().Before(deadline) {
		got := callMCPTool(t, session, "project_automation_delivery_get", map[string]any{
			"workspace": "local", "project": "adsops", "delivery_id": deliveryID,
		})
		if got.IsError {
			t.Fatalf("project_automation_delivery_get error: %#v", got.StructuredContent)
		}
		status, _ = mcpDataMap(t, got)["delivery"].(map[string]any)["status"].(string)
		if status == "succeeded" || status == "dead_lettered" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if status != "succeeded" {
		t.Fatalf("delivery status = %q, want succeeded", status)
	}

	// 5. Agent Provider 收到正确 Authorization 和 model；delivery 记录含 usage 与 provider request id。
	if receivedAuth != "Bearer sk-e2e-secret" {
		t.Fatalf("provider received Authorization = %q", receivedAuth)
	}
	if receivedModel != "project-operator" {
		t.Fatalf("provider received model = %q", receivedModel)
	}
	succeeded := callMCPTool(t, session, "project_automation_delivery_get", map[string]any{
		"workspace": "local", "project": "adsops", "delivery_id": deliveryID,
	})
	deliveryDetail := mcpDataMap(t, succeeded)["delivery"].(map[string]any)
	if detail, _ := deliveryDetail["provider_request_id"].(string); detail != "chatcmpl_mcp_e2e" {
		t.Fatalf("provider_request_id = %#v", deliveryDetail["provider_request_id"])
	}
	usage, _ := deliveryDetail["usage"].(map[string]any)
	if usage["prompt_tokens"] != float64(12) {
		t.Fatalf("usage = %#v", usage)
	}

	// 6. delivery_list 按 rule_id 过滤 + replay 生成新 queued 记录。
	listed := callMCPTool(t, session, "project_automation_delivery_list", map[string]any{
		"workspace": "local", "project": "adsops", "rule_id": ruleID,
	})
	if count, _ := mcpDataMap(t, listed)["count"].(float64); count != 1 {
		t.Fatalf("delivery count = %v, want 1", mcpDataMap(t, listed)["count"])
	}
	replayed := callMCPTool(t, session, "project_automation_delivery_replay", map[string]any{
		"workspace": "local", "project": "adsops", "delivery_id": deliveryID,
	})
	if replayed.IsError {
		t.Fatalf("project_automation_delivery_replay error: %#v", replayed.StructuredContent)
	}
	replayDelivery := mcpDataMap(t, replayed)["delivery"].(map[string]any)
	if replayDelivery["status"] != "queued" {
		t.Fatalf("replay status = %v", replayDelivery["status"])
	}

	// 7. modify 改名 -> disable -> list 包含停用规则 -> remove。
	modified := callMCPTool(t, session, "project_automation_modify", map[string]any{
		"workspace": "local", "project": "adsops", "rule_id": ruleID, "name": "每日项目巡检 v2",
	})
	if mcpDataMap(t, modified)["automation"].(map[string]any)["name"] != "每日项目巡检 v2" {
		t.Fatalf("modified name mismatch: %#v", mcpDataMap(t, modified)["automation"])
	}
	disabled := callMCPTool(t, session, "project_automation_disable", map[string]any{
		"workspace": "local", "project": "adsops", "rule_id": ruleID,
	})
	if mcpDataMap(t, disabled)["automation"].(map[string]any)["enabled"] != false {
		t.Fatalf("disable 后 enabled 应为 false")
	}
	listedAll := callMCPTool(t, session, "project_automation_list", map[string]any{
		"workspace": "local", "project": "adsops", "include_disabled": true,
	})
	if count, _ := mcpDataMap(t, listedAll)["count"].(float64); count != 1 {
		t.Fatalf("include_disabled 后 count 应为 1，got %v", mcpDataMap(t, listedAll)["count"])
	}
	removed := callMCPTool(t, session, "project_automation_remove", map[string]any{
		"workspace": "local", "project": "adsops", "rule_id": ruleID,
	})
	if removed.IsError {
		t.Fatalf("project_automation_remove error: %#v", removed.StructuredContent)
	}

	// 8. template vars 两组触发器。
	vars := callMCPTool(t, session, "project_automation_list_template_vars", map[string]any{
		"workspace": "local", "project": "adsops",
	})
	triggers, _ := mcpDataMap(t, vars)["triggers"].([]any)
	if len(triggers) != 2 {
		t.Fatalf("template var triggers = %d, want 2", len(triggers))
	}

	// 9. 受限 token 只有 project:read、缺 hook scope，双 scope 校验应拒绝。
	limitedSession, limitedCancel := connectHTTPMCP(t, baseURL, limitedToken)
	defer limitedCancel()
	denied := callMCPTool(t, limitedSession, "project_automation_list", map[string]any{
		"workspace": "local", "project": "adsops",
	})
	if !denied.IsError {
		t.Fatalf("limited token should be denied on project_automation_list: %#v", denied.StructuredContent)
	}
	if code := mcpToolErrorCode(t, denied); code != "token_scope_denied" {
		t.Fatalf("denied code = %q, want token_scope_denied", code)
	}
}

// mcpToolErrorCode 从 IsError result 的 structuredContent 中提取错误码。
func mcpToolErrorCode(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if result.StructuredContent == nil {
		return ""
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structuredContent: %v", err)
	}
	var envelope struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("unmarshal structuredContent: %v\n%s", err, raw)
	}
	return envelope.Code
}
