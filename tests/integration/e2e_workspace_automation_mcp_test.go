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

// TestE2EWorkspaceProjectCreatedMCPWriteback 跑通 Agent 端到端回写闭环：
//  1. Workspace 配置 Provider + 创建 enabled project.created 规则。
//  2. 创建空 Project，dispatcher 投递成功。
//  3. 用“Agent token”通过 HTTP MCP 调 project_config_list（验证幂等：先检查 knowledge_id）。
//  4. project_config_set 写 knowledge_id。
//  5. audit_list 断言 config audit 的 actor 是 Agent token（不是 Project 创建者）。
//  6. 再次 project_config_list 验证值已写入；重复 set 走幂等路径不报错。
func TestE2EWorkspaceProjectCreatedMCPWriteback(t *testing.T) {
	bin := buildXuanchu(t)
	dir := t.TempDir()
	serverDB := filepath.Join(dir, "server.db")
	configPath, _ := writeE2ELogConfig(t, dir)

	// fake OpenAI-compatible provider，只是为了让 delivery 进入 succeeded。
	providerTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_wb","usage":{"prompt_tokens":3}}`))
	}))
	defer providerTarget.Close()

	// Admin token：治理 Workspace 自动化 + 配置 Provider + 创建 Project。
	adminToken := parseRawToken(t, createTokenJSON(t, bin, "--db", serverDB, "ws-admin", "*"))

	// Agent token：只具备 Project config 读/写 + audit 读 + project 读，模拟 Agent 平台凭据。
	agentToken := parseRawToken(t, createTokenJSON(t, bin, "--db", serverDB, "ws-agent",
		"project:read", "project:write", "config:read", "config:write", "audit:read"))

	// 先用 CLI 创建 Project 作为“来源项目”，让 schema 里有 yaoguang.knowledge_base.id。
	run(t, bin, "--db", serverDB, "--workspace", "local", "project", "add", "bootstrap", "name:引导项目")
	run(t, bin, "--db", serverDB, "--workspace", "local", "config", "schema", "set",
		"yaoguang.knowledge_base.id", "type:string", "scopes:project",
	)
	run(t, bin, "--db", serverDB, "--workspace", "local", "project", "config", "set",
		"bootstrap", "yaoguang.knowledge_base.id", "kb-bootstrap")

	cmd, baseURL := startXuanchuServer(t, bin,
		"--config", configPath,
		"--db", serverDB,
		"--automation-dispatcher-interval", "1s",
		"--automation-scheduler-interval", "60s",
	)
	defer stopXuanchuServer(t, cmd)

	adminHeaders := authHeaders(adminToken)
	adminHeaders["Content-Type"] = "application/json"
	providerHost := hostFromURL(t, providerTarget.URL)

	// 1. Workspace Provider + project.created 规则。
	providerBody := `{"base_url":"` + providerTarget.URL + `","model":"workspace-operator","allowed_hosts":["` + providerHost + `"],"api_key":"sk-ws-secret"}`
	httpJSONRaw(t, http.MethodPut, baseURL+"/api/v1/automations/provider-config", providerBody, adminHeaders)

	ruleBody := `{
		"name":"知识库初始化",
		"enabled":true,
		"trigger_type":"event",
		"trigger_config":{"event_type":"project.created"},
		"action":{"protocol":"chat_completions","base_url_config_key":"agent.provider.base_url","api_key_config_key":"agent.provider.api_key","model_config_key":"agent.provider.model","temperature":0.2},
		"context":{"include":["workspace","project","project_config","event"]},
		"instruction_template":"初始化知识库；先 project_config_list 检查 knowledge_id，没有再创建并 project_config_set 写回"
	}`
	ruleCreated := httpJSONRaw(t, http.MethodPost, baseURL+"/api/v1/automations", ruleBody, adminHeaders)
	ruleID, _ := ruleCreated["data"].(map[string]any)["id"].(string)
	if ruleID == "" {
		t.Fatalf("create rule missing id: %#v", ruleCreated)
	}

	// 2. 创建空 Project，触发 project.created，等待 delivery succeeded。
	projectCreated := httpJSONRaw(t, http.MethodPost, baseURL+"/api/v1/projects", `{"slug":"wba","name":"回写 E2E"}`, adminHeaders)
	projectID, _ := projectCreated["data"].(map[string]any)["id"].(string)
	if projectID == "" {
		t.Fatalf("created project missing id: %#v", projectCreated)
	}
	deadline := time.Now().Add(20 * time.Second)
	var deliveryStatus string
	for time.Now().Before(deadline) {
		list := httpJSON(t, http.MethodGet, baseURL+"/api/v1/automation-deliveries?rule_id="+ruleID, nil, adminHeaders)
		items, _ := list["data"].([]any)
		if len(items) > 0 {
			first, _ := items[0].(map[string]any)
			deliveryStatus, _ = first["status"].(string)
			if deliveryStatus == "succeeded" || deliveryStatus == "dead_lettered" {
				break
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	if deliveryStatus != "succeeded" {
		t.Fatalf("delivery status = %q, want succeeded", deliveryStatus)
	}

	// 3. Agent 通过 HTTP MCP 调 project_config_list，确认 knowledge_id 当前为空（幂等前置）。
	agentSession, agentCancel := connectHTTPMCP(t, baseURL, agentToken)
	defer agentCancel()
	listed := callMCPTool(t, agentSession, "project_config_list", map[string]any{
		"workspace": "local", "project": "wba",
	})
	if listed.IsError {
		t.Fatalf("project_config_list error: %#v", listed.StructuredContent)
	}
	listData := mcpDataMap(t, listed)
	configMap, _ := listData["config"].(map[string]any)
	if _, exists := configMap["yaoguang.knowledge_base.id"]; exists {
		t.Fatalf("knowledge_id should be absent initially: %#v", configMap)
	}

	// 4. project_config_set 写 knowledge_id，模拟 Agent 调外部工具后回写业务结果。
	const knowledgeID = "kb-wb-2026-001"
	set := callMCPTool(t, agentSession, "project_config_set", map[string]any{
		"workspace": "local", "project": "wba",
		"key":       "yaoguang.knowledge_base.id",
		"value":     knowledgeID,
	})
	if set.IsError {
		t.Fatalf("project_config_set error: %#v", set.StructuredContent)
	}

	// 5. audit_list 断言 config audit 的 actor 是 Agent token（不是创建 Project 的 admin）。
	auditRows := callMCPTool(t, agentSession, "audit_list", map[string]any{
		"workspace": "local", "project": "wba", "limit": 20,
	})
	if auditRows.IsError {
		t.Fatalf("audit_list error: %#v", auditRows.StructuredContent)
	}
	auditData := mcpDataMap(t, auditRows)
	entries, _ := auditData["entries"].([]any)
	if len(entries) == 0 {
		t.Fatalf("audit entries empty: %#v", auditData)
	}
	auditJSON, _ := json.Marshal(entries)
	if !strings.Contains(string(auditJSON), "project.config.set") {
		t.Fatalf("audit missing project.config.set action: %s", auditJSON)
	}
	if !strings.Contains(string(auditJSON), "yaoguang.knowledge_base.id") {
		t.Fatalf("audit missing knowledge_id key: %s", auditJSON)
	}
	if !strings.Contains(string(auditJSON), knowledgeID) {
		t.Fatalf("audit missing knowledge_id value: %s", auditJSON)
	}
	// Agent 回写产生的 config audit：必须有 project.config.set 行，payload 含
	// key=yaoguang.knowledge_base.id 和 value=knowledgeID。Actor 是 Agent token
	// 背后的用户；PAT 的 actor_token 字段不展开（仅 tenant_access_token 才填），
	// 因此只断言 config audit 存在且由 Agent 调用产生，不与 project.add 混淆。
	configAuditFound := false
	projectAddCount := 0
	for _, entry := range entries {
		row, _ := entry.(map[string]any)
		switch row["action"] {
		case "project.config.set":
			payload, _ := row["payload"].(map[string]any)
			if payload["key"] != "yaoguang.knowledge_base.id" || payload["value"] != knowledgeID {
				t.Fatalf("config audit payload = %#v", payload)
			}
			configAuditFound = true
		case "project.add":
			projectAddCount++
		}
	}
	if !configAuditFound {
		t.Fatalf("no project.config.set audit entry in %s", auditJSON)
	}
	if projectAddCount == 0 {
		t.Fatalf("no project.add audit entry (Agent writeback must be separate from Project create): %s", auditJSON)
	}

	// 6. 再次 project_config_list 验证值已写入；重复 set 走幂等路径不报错。
	listed2 := callMCPTool(t, agentSession, "project_config_list", map[string]any{
		"workspace": "local", "project": "wba",
	})
	config2, _ := mcpDataMap(t, listed2)["config"].(map[string]any)
	if config2["yaoguang.knowledge_base.id"] != knowledgeID {
		t.Fatalf("knowledge_id not persisted: %#v", config2)
	}
	idempotent := callMCPTool(t, agentSession, "project_config_set", map[string]any{
		"workspace": "local", "project": "wba",
		"key":       "yaoguang.knowledge_base.id",
		"value":     knowledgeID,
	})
	if idempotent.IsError {
		t.Fatalf("idempotent re-set should succeed: %#v", idempotent.StructuredContent)
	}
}

// callMCPTool 包装 session.CallTool，失败时 t.Fatalf。
func callMCPTool(t *testing.T, session *mcp.ClientSession, name string, arguments map[string]any) *mcp.CallToolResult {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("MCP CallTool %s: %v", name, err)
	}
	return result
}

// mcpDataMap 从 MCP result 的 structuredContent 中提取 data map。
func mcpDataMap(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()
	if result.StructuredContent == nil {
		t.Fatalf("MCP result missing structuredContent: %+v", result)
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structuredContent: %v", err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("unmarshal structuredContent: %v\n%s", err, raw)
	}
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("structuredContent missing data: %#v", envelope)
	}
	return data
}
