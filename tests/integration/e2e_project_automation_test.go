package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestE2EProjectAutomation 跑通项目自动化全链路：
// 建项目 -> 配置 provider config -> HTTP 创建规则 -> 预览脱敏 -> 立即测试入队 -> 后台 dispatcher 投递 -> delivery 成功。
func TestE2EProjectAutomation(t *testing.T) {
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
		_, _ = w.Write([]byte(`{"id":"chatcmpl_e2e","usage":{"prompt_tokens":12,"completion_tokens":4}}`))
	}))
	defer providerTarget.Close()

	// 预置项目和 provider config（agent.provider.* 是内置 schema，直接 set value）。
	run(t, bin, "--db", serverDB, "--workspace", "local", "project", "add", "adsops", "name:广告投放优化")
	run(t, bin, "--db", serverDB, "--workspace", "local", "project", "config", "set", "adsops", "agent.provider.base_url", providerTarget.URL)
	run(t, bin, "--db", serverDB, "--workspace", "local", "project", "config", "set", "adsops", "agent.provider.api_key", "sk-e2e-secret")
	run(t, bin, "--db", serverDB, "--workspace", "local", "project", "config", "set", "adsops", "agent.provider.model", "project-operator")
	providerHost := hostFromURL(t, providerTarget.URL)
	run(t, bin, "--db", serverDB, "--workspace", "local", "project", "config", "set", "adsops", "agent.provider.allowed_hosts", `["`+providerHost+`"]`)

	// 全权限 token。
	token := parseRawToken(t, createTokenJSON(t, bin, "--db", serverDB, "automation-e2e", "*"))

	// 启动 server，dispatcher 间隔 1s 保证测试投递尽快被处理。
	cmd, baseURL := startXuanchuServer(t, bin,
		"--config", configPath,
		"--db", serverDB,
		"--automation-dispatcher-interval", "1s",
		"--automation-scheduler-interval", "60s",
	)
	defer stopXuanchuServer(t, cmd)

	headers := authHeaders(token)
	headers["Content-Type"] = "application/json"

	// 1. 创建 schedule 规则。
	createBody := `{
		"name":"每日项目巡检",
		"enabled":true,
		"trigger_type":"schedule",
		"trigger_config":{"schedule_type":"daily_at","schedule_value":"09:30","timezone":"Asia/Shanghai"},
		"action":{"protocol":"chat_completions","base_url_config_key":"agent.provider.base_url","api_key_config_key":"agent.provider.api_key","model_config_key":"agent.provider.model","temperature":0.2},
		"context":{"include":["workspace","project","project_config"]},
		"instruction_template":"生成巡检报告"
	}`
	created := httpJSONRaw(t, http.MethodPost, baseURL+"/api/v1/projects/adsops/automations", createBody, headers)
	ruleID, _ := created["data"].(map[string]any)["id"].(string)
	if ruleID == "" {
		t.Fatalf("create rule missing id: %#v", created)
	}

	// 2. 预览脱敏：Authorization 显示 Bearer ****，不含 sk-e2e-secret。
	preview := httpJSONRaw(t, http.MethodPost, baseURL+"/api/v1/projects/adsops/automations/preview", createBody, headers)
	previewStr := toJSONString(t, preview)
	if strings.Contains(previewStr, "sk-e2e-secret") {
		t.Fatalf("preview leaked secret: %s", previewStr)
	}
	if !strings.Contains(previewStr, `"Authorization":"Bearer ****"`) {
		t.Fatalf("preview missing masked authorization: %s", previewStr)
	}

	// 3. 立即测试 -> 入队一条 manual_test delivery。
	tested := httpJSONRaw(t, http.MethodPost, baseURL+"/api/v1/projects/adsops/automations/"+ruleID+"/test", `{}`, headers)
	deliveryID, _ := tested["data"].(map[string]any)["id"].(string)
	if deliveryID == "" {
		t.Fatalf("test delivery missing id: %#v", tested)
	}

	// 4. 轮询 delivery 直到 succeeded（后台 dispatcher 会处理）。
	deadline := time.Now().Add(15 * time.Second)
	var status string
	for time.Now().Before(deadline) {
		got := httpJSON(t, http.MethodGet, baseURL+"/api/v1/projects/adsops/automation-deliveries/"+deliveryID, nil, headers)
		data, _ := got["data"].(map[string]any)
		status, _ = data["status"].(string)
		if status == "succeeded" || status == "dead_lettered" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if status != "succeeded" {
		t.Fatalf("delivery status = %q, want succeeded", status)
	}

	// 5. 断言 Agent Provider 收到了正确 Authorization 和 model。
	if receivedAuth != "Bearer sk-e2e-secret" {
		t.Fatalf("provider received Authorization = %q, want Bearer sk-e2e-secret", receivedAuth)
	}
	if receivedModel != "project-operator" {
		t.Fatalf("provider received model = %q, want project-operator", receivedModel)
	}

	// 6. delivery 详情包含 provider request id 和 usage。
	got := httpJSON(t, http.MethodGet, baseURL+"/api/v1/projects/adsops/automation-deliveries/"+deliveryID, nil, headers)
	data, _ := got["data"].(map[string]any)
	if data["provider_request_id"] != "chatcmpl_e2e" {
		t.Fatalf("provider_request_id = %#v, want chatcmpl_e2e", data["provider_request_id"])
	}

	// 7. 创建 cron 规则并验证字段正确回显（schedule_type=cron、schedule_value、timezone）。
	cronBody := `{
		"name":"每周项目回顾",
		"enabled":true,
		"trigger_type":"schedule",
		"trigger_config":{"schedule_type":"cron","schedule_value":"0 9 * * 1","timezone":"Asia/Shanghai"},
		"action":{"protocol":"chat_completions","base_url_config_key":"agent.provider.base_url","api_key_config_key":"agent.provider.api_key","model_config_key":"agent.provider.model","temperature":0.2},
		"context":{"include":["workspace","project","project_config"]},
		"instruction_template":"生成周报"
	}`
	cronCreated := httpJSONRaw(t, http.MethodPost, baseURL+"/api/v1/projects/adsops/automations", cronBody, headers)
	cronData, _ := cronCreated["data"].(map[string]any)
	cronCfg, _ := cronData["trigger_config"].(map[string]any)
	if cronCfg["schedule_type"] != "cron" {
		t.Fatalf("cron rule schedule_type = %#v, want cron", cronCfg["schedule_type"])
	}
	if cronCfg["schedule_value"] != "0 9 * * 1" {
		t.Fatalf("cron rule schedule_value = %#v", cronCfg["schedule_value"])
	}
	if cronCfg["timezone"] != "Asia/Shanghai" {
		t.Fatalf("cron rule timezone = %#v, want Asia/Shanghai", cronCfg["timezone"])
	}

	// 8. 非法 cron 表达式被拒绝。
	badCronBody := `{
		"name":"bad-cron",
		"trigger_type":"schedule",
		"trigger_config":{"schedule_type":"cron","schedule_value":"0 9 *","timezone":"Asia/Shanghai"},
		"action":{"protocol":"chat_completions","base_url_config_key":"agent.provider.base_url","api_key_config_key":"agent.provider.api_key","model_config_key":"agent.provider.model"},
		"instruction_template":"x"
	}`
	badResp, _ := httpDo(t, http.MethodPost, baseURL+"/api/v1/projects/adsops/automations", strings.NewReader(badCronBody), headers)
	defer badResp.Body.Close()
	if badResp.StatusCode < 400 {
		t.Fatalf("非法 cron 表达式应被拒绝，status = %d", badResp.StatusCode)
	}
}

func hostFromURL(t *testing.T, raw string) string {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse url %q: %v", raw, err)
	}
	return parsed.Hostname()
}

// httpJSONRaw 发送原始 JSON 字符串 body 并解析响应 envelope.data 为 map。
func httpJSONRaw(t *testing.T, method, urlStr, body string, headers map[string]string) map[string]any {
	t.Helper()
	resp, payload := httpDo(t, method, urlStr, strings.NewReader(body), headers)
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("%s %s status = %d body=%s", method, urlStr, resp.StatusCode, payload)
	}
	var out map[string]any
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("%s %s response is not JSON: %v\n%s", method, urlStr, err, payload)
	}
	return out
}

// TestE2EProjectAutomationTemplateVars 验证模板变量 API 和预览中的变量替换。
func TestE2EProjectAutomationTemplateVars(t *testing.T) {
	bin := buildXuanchu(t)
	dir := t.TempDir()
	serverDB := filepath.Join(dir, "server.db")
	configPath, _ := writeE2ELogConfig(t, dir)

	// 预置项目和 provider config。
	run(t, bin, "--db", serverDB, "--workspace", "local", "project", "add", "tmplvars", "name:模板变量测试")
	run(t, bin, "--db", serverDB, "--workspace", "local", "project", "config", "set", "tmplvars", "agent.provider.base_url", "https://agent.example.com")
	run(t, bin, "--db", serverDB, "--workspace", "local", "project", "config", "set", "tmplvars", "agent.provider.api_key", "sk-tmpl")
	run(t, bin, "--db", serverDB, "--workspace", "local", "project", "config", "set", "tmplvars", "agent.provider.model", "op")
	run(t, bin, "--db", serverDB, "--workspace", "local", "project", "config", "set", "tmplvars", "feishu.chat_id", "oc_tmpl")

	token := parseRawToken(t, createTokenJSON(t, bin, "--db", serverDB, "tmplvars-e2e", "*"))
	cmd, baseURL := startXuanchuServer(t, bin, "--config", configPath, "--db", serverDB)
	defer stopXuanchuServer(t, cmd)

	headers := authHeaders(token)
	headers["Content-Type"] = "application/json"

	// 1. template-vars API 返回按触发器分组的变量列表。
	varsResp := httpJSON(t, http.MethodGet, baseURL+"/api/v1/projects/tmplvars/automation-template-vars", nil, headers)
	varsData := varsResp["data"].(map[string]any)
	triggers := varsData["triggers"].([]any)
	if len(triggers) != 2 {
		t.Fatalf("expected 2 triggers, got %d", len(triggers))
	}

	// 2. 预览中使用模板变量，验证变量被正确替换。
	previewBody := `{
		"name":"模板变量测试",
		"enabled":true,
		"trigger_type":"schedule",
		"trigger_config":{"schedule_type":"daily_at","schedule_value":"09:30","timezone":"Asia/Shanghai"},
		"action":{"protocol":"chat_completions","base_url_config_key":"agent.provider.base_url","api_key_config_key":"agent.provider.api_key","model_config_key":"agent.provider.model","temperature":0.2},
		"context":{"include":["project","project_config"]},
		"instruction_template":"检查项目 {{project.name}}（{{project.slug}}），配置：{{project_config}}",
		"system_prompt":"你是 {{project.slug}} 的 Agent"
	}`
	preview := httpJSONRaw(t, http.MethodPost, baseURL+"/api/v1/projects/tmplvars/automations/preview", previewBody, headers)
	previewStr := toJSONString(t, preview)
	// 变量应被替换为实际值。
	if !strings.Contains(previewStr, "模板变量测试") {
		t.Fatalf("preview missing rendered project.name: %s", previewStr)
	}
	if !strings.Contains(previewStr, "tmplvars") {
		t.Fatalf("preview missing rendered project.slug: %s", previewStr)
	}
	if !strings.Contains(previewStr, "oc_tmpl") {
		t.Fatalf("preview missing rendered project_config: %s", previewStr)
	}
	// 不应包含未替换的占位符。
	if strings.Contains(previewStr, "{{project.name}}") {
		t.Fatalf("preview contains unreplaced var: %s", previewStr)
	}
	// system prompt 应包含替换后的 slug。
	if !strings.Contains(previewStr, "你是 tmplvars 的 Agent") {
		t.Fatalf("preview missing rendered system prompt: %s", previewStr)
	}
}
