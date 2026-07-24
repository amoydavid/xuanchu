package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestE2ETaskModifyAuditedAcrossEntryPoints 校验通过 Web Console 使用的 HTTP
// 接口、MCP 工具和 CLI 对任务做任意字段修改时，都会被审计记录下来，并且能在
// /api/v1/tasks/{taskRef}/activity 里读到语义化字段 changes。
//
// 三个入口分别修改不同字段，避免互相干扰，便于在共享的 audit 流里精确定位。
func TestE2ETaskModifyAuditedAcrossEntryPoints(t *testing.T) {
	bin := buildXuanchu(t)
	dir := t.TempDir()
	db := filepath.Join(dir, "xuanchu.db")
	// 全量 scope token，三个入口共用，聚焦「字段修改是否被审计」这一行为本身。
	token := parseRawToken(t, createTokenJSON(t, bin, "--db", db, "task-audit-e2e", "*"))

	cmd, baseURL := startXuanchuServer(t, bin, "--db", db)
	defer stopXuanchuServer(t, cmd)

	// 1. Web Console / HTTP：创建任务 + 多次字段修改。
	httpTask := createTaskViaHTTP(t, baseURL, token, "http-entry task")
	patchTaskViaHTTP(t, baseURL, token, httpTask["uuid"].(string), map[string]any{
		"priority": "H",
	})
	patchTaskViaHTTP(t, baseURL, token, httpTask["uuid"].(string), map[string]any{
		"description": "由 web console 编辑的描述",
	})
	// 清空 due 的场景：current 应保留显式 null 语义。
	patchTaskViaHTTP(t, baseURL, token, httpTask["uuid"].(string), map[string]any{
		"due": 1_783_036_800,
	})
	patchTaskViaHTTP(t, baseURL, token, httpTask["uuid"].(string), map[string]any{
		"clear_due": true,
	})

	// 2. MCP：改 title。
	mcpTask := createTaskViaHTTP(t, baseURL, token, "mcp-entry task")
	mcpModifyTaskViaMCP(t, baseURL, token, mcpTask["uuid"].(string), map[string]any{
		"title": "mcp-entry task (renamed)",
	})

	// 3. CLI（remote 模式，走 HTTP）：改 tag。
	cliTask := createTaskViaHTTP(t, baseURL, token, "cli-entry task")
	run(t, bin,
		"--server", baseURL, "--token", token, "--workspace", "local",
		cliTask["uuid"].(string), "modify", "+audited",
	)

	// 4. 低频字段：HTTP 改 wait/scheduled/until/udas/depends，MCP 改 scheduled。
	// 覆盖 spec 第二期纳入的 wait/scheduled/until/depends/udas。
	// 注意：UDA 必须先 config 定义，否则 modify 会被拒；depends 必须引用存在的 task。
	run(t, bin, "--server", baseURL, "--token", token, "--workspace", "local",
		"config", "set", "uda.estimate.type", "string")
	lowFreqTask := createTaskViaHTTP(t, baseURL, token, "low-freq task")
	depTask := createTaskViaHTTP(t, baseURL, token, "low-freq dependency")
	patchTaskViaHTTP(t, baseURL, token, lowFreqTask["uuid"].(string), map[string]any{
		"wait":    1_783_036_800,
		"until":   1_783_209_600,
		"udas":    map[string]string{"estimate": "2h"},
		"depends": []string{depTask["uuid"].(string)},
	})
	mcpModifyTaskViaMCP(t, baseURL, token, lowFreqTask["uuid"].(string), map[string]any{
		"scheduled": 1_783_123_200,
	})

	// 统一通过 HTTP task activity 端点验证三个入口的修改都被记录。
	httpUUID := httpTask["uuid"].(string)
	assertTaskActivityChange(t, baseURL, token, httpUUID, "priority")
	assertTaskActivityChange(t, baseURL, token, httpUUID, "description")
	assertTaskActivityChange(t, baseURL, token, httpUUID, "due")
	// 清空 due 后 latest change 的 current 必须保留显式 null。
	assertTaskActivityChangeCurrentNull(t, baseURL, token, httpUUID, "due")
	assertTaskActivityChange(t, baseURL, token, mcpTask["uuid"].(string), "title")
	assertTaskActivityChange(t, baseURL, token, cliTask["uuid"].(string), "tags")

	// 低频字段也都被审计记录。
	lowUUID := lowFreqTask["uuid"].(string)
	assertTaskActivityChange(t, baseURL, token, lowUUID, "wait")
	assertTaskActivityChange(t, baseURL, token, lowUUID, "scheduled")
	assertTaskActivityChange(t, baseURL, token, lowUUID, "until")
	assertTaskActivityChange(t, baseURL, token, lowUUID, "depends")
	assertTaskActivityChange(t, baseURL, token, lowUUID, "udas")

	// 同时确认通用 audit 流也记录了 task.modify（HTTP audit:list，需 audit:read）。
	generalAudit := httpJSON(t, http.MethodGet, baseURL+"/api/v1/audit?limit=50", nil, authHeaders(token))
	if !auditListContainsAction(generalAudit, "task.modify") {
		t.Fatalf("general audit list missing task.modify: %#v", generalAudit)
	}
}

func createTaskViaHTTP(t *testing.T, baseURL, token, title string) map[string]any {
	t.Helper()
	resp := httpJSON(t, http.MethodPost, baseURL+"/api/v1/tasks?workspace=local",
		map[string]any{"title": title}, authHeaders(token))
	task, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("create task response missing data: %#v", resp)
	}
	if task["uuid"] == nil {
		t.Fatalf("created task missing uuid: %#v", task)
	}
	return task
}

func patchTaskViaHTTP(t *testing.T, baseURL, token, taskUUID string, body map[string]any) {
	t.Helper()
	httpJSON(t, http.MethodPatch, baseURL+"/api/v1/tasks/"+taskUUID+"?workspace=local",
		body, authHeaders(token))
}

func mcpModifyTaskViaMCP(t *testing.T, baseURL, token, taskUUID string, args map[string]any) {
	t.Helper()
	session, cancel := connectHTTPMCP(t, baseURL, token)
	defer cancel()
	input := map[string]any{"id": taskUUID, "workspace": "local"}
	for k, v := range args {
		input[k] = v
	}
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "task_modify",
		Arguments: input,
	})
	if err != nil {
		t.Fatalf("MCP task_modify error = %v", err)
	}
	_ = mcpStructuredMap(t, res)
}

// assertTaskActivityChange 拉 /tasks/{ref}/activity，确认存在一条
// fields_changed，且 changes 里包含期望字段。
func assertTaskActivityChange(t *testing.T, baseURL, token, taskUUID, wantField string) {
	t.Helper()
	resp := httpJSON(t, http.MethodGet,
		baseURL+"/api/v1/tasks/"+taskUUID+"/activity?workspace=local&limit=20",
		nil, authHeaders(token))
	data, _ := resp["data"].(map[string]any)
	rows, _ := data["entries"].([]any)
	if len(rows) == 0 {
		t.Fatalf("task %s activity returned no rows", taskUUID)
	}
	for _, row := range rows {
		entry, _ := row.(map[string]any)
		if entry["action"] != "fields_changed" {
			continue
		}
		if changesContainField(entry, wantField) {
			return
		}
	}
	// 输出原始 Activity 帮助定位失败原因。
	raw, _ := json.Marshal(resp)
	t.Fatalf("task %s activity missing %q field change; activity=%s", taskUUID, wantField, raw)
}

// assertTaskActivityChangeCurrentNull 验证某个字段最新一次 change 的 current
// 保留了显式 null（清空 due/description 等场景），而不是被 omitempty 丢掉。
func assertTaskActivityChangeCurrentNull(t *testing.T, baseURL, token, taskUUID, wantField string) {
	t.Helper()
	resp := httpJSON(t, http.MethodGet,
		baseURL+"/api/v1/tasks/"+taskUUID+"/activity?workspace=local&limit=20",
		nil, authHeaders(token))
	data, _ := resp["data"].(map[string]any)
	rows, _ := data["entries"].([]any)
	// Activity 默认按 occurred_at desc 返回，找第一条匹配字段的 change。
	for _, row := range rows {
		entry, _ := row.(map[string]any)
		if entry["action"] != "fields_changed" {
			continue
		}
		changes, _ := entry["changes"].([]any)
		for _, c := range changes {
			change, _ := c.(map[string]any)
			if change["field"] != wantField {
				continue
			}
			current, ok := change["current"].(map[string]any)
			if !ok {
				t.Fatalf("task %s %q change current missing; change=%#v", taskUUID, wantField, change)
			}
			// raw 为 null 时 JSON 解析成 nil。
			if current["raw"] != nil {
				t.Fatalf("task %s %q change current.raw = %#v, want nil (explicit null)", taskUUID, wantField, current["raw"])
			}
			return
		}
	}
	raw, _ := json.Marshal(resp)
	t.Fatalf("task %s activity missing %q field change; activity=%s", taskUUID, wantField, raw)
}

func changesContainField(entry map[string]any, wantField string) bool {
	changes, _ := entry["changes"].([]any)
	for _, c := range changes {
		change, _ := c.(map[string]any)
		if change["field"] == wantField {
			return true
		}
	}
	return false
}

func auditListContainsAction(resp map[string]any, action string) bool {
	rows, _ := resp["data"].([]any)
	for _, row := range rows {
		entry, _ := row.(map[string]any)
		if entry["action"] == action {
			return true
		}
	}
	return false
}
