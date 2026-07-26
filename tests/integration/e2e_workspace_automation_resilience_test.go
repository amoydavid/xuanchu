package integration

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestE2EWorkspaceAutomationDeliverySurvivesRuleDeletion 验证 dispatcher 不依赖当前 Rule：
// 规则删除后，已 queued 的 Delivery 仍能被发送并进入 succeeded。
// spec §11.3 / §18.3: 「规则修改/删除后 dispatcher 不读取当前 Rule，仍使用 Delivery 冻结的
// auth key、安全策略 key 和 max attempts」。
func TestE2EWorkspaceAutomationDeliverySurvivesRuleDeletion(t *testing.T) {
	bin := buildXuanchu(t)
	dir := t.TempDir()
	serverDB := filepath.Join(dir, "server.db")
	configPath, _ := writeE2ELogConfig(t, dir)

	// fake provider 记录收到的请求。
	var requestCount int
	providerTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_del","usage":{}}`))
	}))
	defer providerTarget.Close()

	adminToken := parseRawToken(t, createTokenJSON(t, bin, "--db", serverDB, "del-admin", "*"))

	// 先暂停 dispatcher 让 Delivery 保持 queued：用长间隔启动 server，后面单独触发 dispatcher。
	// 这里用正常 1s 间隔，但在 delete 前快速注入一条 queued Delivery。
	cmd, baseURL := startXuanchuServer(t, bin,
		"--config", configPath,
		"--db", serverDB,
		"--automation-dispatcher-interval", "60s", // 拉长到 60s，给 delete 留窗口
		"--automation-scheduler-interval", "60s",
	)
	defer stopXuanchuServer(t, cmd)
	headers := authHeaders(adminToken)
	headers["Content-Type"] = "application/json"
	providerHost := hostFromURL(t, providerTarget.URL)

	// 配置 Provider。
	providerBody := `{"base_url":"` + providerTarget.URL + `","model":"workspace-operator","allowed_hosts":["` + providerHost + `"],"api_key":"sk-ws"}`
	httpJSONRaw(t, http.MethodPut, baseURL+"/api/v1/automations/provider-config", providerBody, headers)

	// 创建规则。
	ruleBody := `{"name":"del-rule","enabled":true,"trigger_type":"event","trigger_config":{"event_type":"project.created"},"action":{"protocol":"chat_completions","base_url_config_key":"agent.provider.base_url","api_key_config_key":"agent.provider.api_key","model_config_key":"agent.provider.model","temperature":0.2},"context":{"include":["workspace","project"]},"instruction_template":"x"}`
	ruleCreated := httpJSONRaw(t, http.MethodPost, baseURL+"/api/v1/automations", ruleBody, headers)
	ruleID, _ := ruleCreated["data"].(map[string]any)["id"].(string)
	if ruleID == "" {
		t.Fatalf("rule missing id: %#v", ruleCreated)
	}

	// 创建项目触发 project.created -> 一条 queued Delivery。
	projectCreated := httpJSONRaw(t, http.MethodPost, baseURL+"/api/v1/projects", `{"slug":"del1","name":"del proj"}`, headers)
	projectID, _ := projectCreated["data"].(map[string]any)["id"].(string)
	if projectID == "" {
		t.Fatalf("project missing id: %#v", projectCreated)
	}

	// 等待 Delivery 出现（dispatcher 60s 间隔，应保持 queued）。
	deadline := time.Now().Add(10 * time.Second)
	var deliveryID string
	for time.Now().Before(deadline) {
		list := httpJSON(t, http.MethodGet, baseURL+"/api/v1/automation-deliveries?rule_id="+ruleID, nil, headers)
		items, _ := list["data"].([]any)
		if len(items) > 0 {
			first, _ := items[0].(map[string]any)
			deliveryID, _ = first["id"].(string)
			if deliveryID != "" {
				break
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	if deliveryID == "" {
		t.Fatalf("delivery not created within window")
	}

	// 删除规则。
	delResp := httpJSON(t, http.MethodDelete, baseURL+"/api/v1/automations/"+ruleID, nil, headers)
	if delResp["data"] == nil {
		t.Fatalf("delete rule failed: %#v", delResp)
	}

	// 触发 dispatcher：通过 short-poll，因为 server 的 interval 是 60s。
	// 这里直接重启 server 用 1s 间隔，让 dispatcher 领取已 queued 的 Delivery。
	// 先停掉旧 server（defer 已经注册了 stop）。
	stopXuanchuServer(t, cmd)
	cmd2, baseURL2 := startXuanchuServer(t, bin,
		"--config", configPath,
		"--db", serverDB,
		"--automation-dispatcher-interval", "1s",
		"--automation-scheduler-interval", "60s",
	)
	defer stopXuanchuServer(t, cmd2)

	// 轮询直到 Delivery succeeded（规则已删除，但 Delivery 冻结了所有必要字段）。
	deadline = time.Now().Add(20 * time.Second)
	var finalStatus string
	for time.Now().Before(deadline) {
		detail := httpJSON(t, http.MethodGet, baseURL2+"/api/v1/automation-deliveries/"+deliveryID, nil, headers)
		data, _ := detail["data"].(map[string]any)
		finalStatus, _ = data["status"].(string)
		if finalStatus == "succeeded" || finalStatus == "dead_lettered" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if finalStatus != "succeeded" {
		t.Fatalf("delivery status = %q after rule deletion, want succeeded", finalStatus)
	}
	if requestCount == 0 {
		t.Fatalf("provider received 0 requests; dispatcher did not fire after rule deletion")
	}
}

// TestE2EWorkspaceAutomationProjectProviderFacadeHidesSecret 验证 Project Provider 安全
// facade GET 永远不回显 api_key 明文，浏览器/客户端通过 facade 而不是 project config list。
// spec §14.2: 「GET 只返回 9.3 的安全 DTO；API key 不得进入响应」。
func TestE2EWorkspaceAutomationProjectProviderFacadeHidesSecret(t *testing.T) {
	bin := buildXuanchu(t)
	dir := t.TempDir()
	serverDB := filepath.Join(dir, "server.db")
	configPath, _ := writeE2ELogConfig(t, dir)

	token := parseRawToken(t, createTokenJSON(t, bin, "--db", serverDB, "prov-admin", "*"))
	cmd, baseURL := startXuanchuServer(t, bin, "--config", configPath, "--db", serverDB)
	defer stopXuanchuServer(t, cmd)
	headers := authHeaders(token)
	headers["Content-Type"] = "application/json"

	// 创建项目 + 配置 provider（通过 CLI 写入原始 project config，模拟历史数据）。
	run(t, bin, "--db", serverDB, "--workspace", "local", "project", "add", "facade", "name:facade")
	run(t, bin, "--db", serverDB, "--workspace", "local", "project", "config", "set", "facade", "agent.provider.base_url", "https://agent.example.com")
	run(t, bin, "--db", serverDB, "--workspace", "local", "project", "config", "set", "facade", "agent.provider.api_key", "sk-facade-secret-must-not-leak")
	run(t, bin, "--db", serverDB, "--workspace", "local", "project", "config", "set", "facade", "agent.provider.model", "project-operator")
	run(t, bin, "--db", serverDB, "--workspace", "local", "project", "config", "set", "facade", "agent.provider.allowed_hosts", `["agent.example.com"]`)

	// GET facade：只暴露 api_key_set=true，不回显明文。
	resp := httpJSONRaw(t, http.MethodGet, baseURL+"/api/v1/projects/facade/automations/provider-config", ``, headers)
	body := toJSONString(t, resp)
	if strings.Contains(body, "sk-facade-secret-must-not-leak") {
		t.Fatalf("project provider facade leaked api_key: %s", body)
	}
	if !strings.Contains(body, `"api_key_set":true`) {
		t.Fatalf("facade missing api_key_set=true: %s", body)
	}

	// 确保通用 config list 仍然存在（但 facade 是独立路径，浏览器不应再走 config list 取 secret）。
	// 这里只断言 facade 不回显 secret。
}
