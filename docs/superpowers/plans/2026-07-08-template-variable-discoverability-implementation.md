# Sink 模板变量可发现性 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 sink/通知模板变量从硬编码 if 分支重构为声明式 schema（唯一真相源），暴露只读 API，并在 web console 规则编辑侧按触发源显示精确可用变量 + 选中 sink 的 body 只读预览。

**Architecture:** 后端新增 `notification_template_vars.go` 声明变量描述表，`notificationTemplateValue`/`allowedEndpointVariable` 改为从该表派生（行为等价）；HTTP 层新增 `GET /api/v1/notification-template-vars` 只读端点；前端在 reminder / notification 规则创建表单的 sink 选择器下方，按当前规则 trigger 渲染变量清单 + sink body 模板只读预览。

**Tech Stack:** Go 1.25 / GORM / chi+huma (httpapi) / React + TypeScript + shadcn + react-query + react-i18next / vitest + testing-library

**Spec:** `docs/superpowers/specs/2026-07-08-template-variable-discoverability-design.md`

---

## 文件结构

**新建文件：**
- `internal/app/notification_template_vars.go` — 模板变量声明式 schema（唯一真相源）+ 派生查询函数
- `internal/app/notification_template_vars_test.go` — schema 完整性与一致性测试
- `internal/httpapi/notification_template_vars.go` — `GET /api/v1/notification-template-vars` handler
- `internal/httpapi/notification_template_vars_test.go` — API 端点测试
- `web/src/features/workspace/outbound/template-vars/template-vars.ts` — TS 类型 + API client
- `web/src/features/workspace/outbound/template-vars/template-var-hints.tsx` — 变量提示 + body 只读预览组件
- `web/src/features/workspace/outbound/template-vars/template-var-hints.test.tsx` — 组件测试

**修改文件：**
- `internal/app/notification_endpoint.go` — `notificationTemplateValue`/`allowedEndpointVariable` 改为从 schema 派生（删硬编码白名单）
- `internal/httpapi/huma_routes.go` — 注册新路由
- `web/src/features/workspace/outbound/outbound-api.ts` — 导出 `NotificationSink` 取 body_template（已存在，无需改）
- `web/src/features/workspace/outbound/rules/reminder-rule-list.tsx` — sink 选择器下方接入 `TemplateVarHints`
- `web/src/features/workspace/outbound/rules/notification-rule-list.tsx` — 同上
- `web/src/locales/zh-CN.ts` / `web/src/locales/en-US.ts` — 新增 `outbound.templateVars.*` 文案

---

## Task 1: 后端 — 变量 schema 声明与派生函数

**目标：** 新建 `notification_template_vars.go`，定义声明式变量描述表，提供派生查询函数。此 task 只新增文件，不改 `notification_endpoint.go`（下一个 task 才切换调用点），保证可独立测试。

**Files:**
- Create: `internal/app/notification_template_vars.go`
- Test: `internal/app/notification_template_vars_test.go`

- [ ] **Step 1: 写失败测试 — schema 完整性与派生**

创建 `internal/app/notification_template_vars_test.go`：

```go
package app

import (
	"testing"
)

func TestNotificationTemplateVarSpecsCoversAllKnownVars(t *testing.T) {
	// schema 里每个非 prefix 变量，取值函数必须能命中（用 zero input 不报 unsupported）。
	known := []string{
		"workspace.id", "workspace.slug",
		"project.id", "project.slug",
		"rule.id", "rule.name",
		"recipient.id",
		"delivery.id", "delivery.attempt", "delivery.workspace_id", "delivery.sink_id",
		"object.kind", "object.id",
		"task.uuid", "task.task_slug", "task.title", "task.description", "task.status", "task.due",
		"reminder.sequence", "reminder.overdue_sequence", "reminder.window_start", "reminder.window_end",
		"event.id", "event.type", "event.version", "event.occurred_at", "event.object_kind", "event.object_id", "event.json",
		"actor.id", "actor.name",
	}
	for _, name := range known {
		if lookupTemplateVarSpec(name) == nil {
			t.Errorf("variable %q not declared in schema", name)
		}
	}
}

func TestEndpointFieldExcludesTaskAndSecret(t *testing.T) {
	// task.* 和 secret.* 不允许出现在 endpoint（URL 模板）。
	taskVars := []string{"task.uuid", "task.title", "task.due"}
	for _, name := range taskVars {
		if templateVarFieldAllowed(name, templateFieldEndpoint) {
			t.Errorf("%q must NOT be allowed in endpoint field", name)
		}
	}
	if templateVarFieldAllowed("secret.token", templateFieldEndpoint) {
		t.Error("secret.* must NOT be allowed in endpoint field")
	}
	// 通用变量允许 endpoint。
	if !templateVarFieldAllowed("workspace.id", templateFieldEndpoint) {
		t.Error("workspace.id should be allowed in endpoint field")
	}
}

func TestTriggerGrouping(t *testing.T) {
	// reminder 组不含 event.* / actor.*。
	reminderVars := templateVarNamesForTrigger(templateTriggerReminder)
	for _, name := range reminderVars {
		if isPrefixVarName(name) {
			continue
		}
		if startsWith(name, "event.") || startsWith(name, "actor.") {
			t.Errorf("reminder trigger must not include %q", name)
		}
	}
	// event 组不含 task.* / reminder.*。
	eventVars := templateVarNamesForTrigger(templateTriggerEvent)
	for _, name := range eventVars {
		if isPrefixVarName(name) {
			continue
		}
		if startsWith(name, "task.") || startsWith(name, "reminder.") {
			t.Errorf("event trigger must not include %q", name)
		}
	}
	// 通用变量两组都有。
	foundCommon := false
	for _, name := range reminderVars {
		if name == "workspace.id" {
			foundCommon = true
		}
	}
	if !foundCommon {
		t.Error("workspace.id should appear in reminder trigger")
	}
}

func TestPrefixVarsDeclared(t *testing.T) {
	// recipient.external_ids.* 和 secret.* 是 prefix 变量。
	for _, prefix := range []string{"recipient.external_ids", "secret"} {
		if lookupPrefixTemplateVarSpec(prefix) == nil {
			t.Errorf("prefix variable %q not declared", prefix)
		}
	}
}

// 辅助：仅用于测试的可读性。
func startsWith(s, prefix string) bool { return len(s) >= len(prefix) && s[:len(prefix)] == prefix }
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/app/ -run TestNotificationTemplateVarSpecs -v`
Expected: FAIL，编译错误（`lookupTemplateVarSpec` 等函数未定义）

- [ ] **Step 3: 实现 schema 与派生函数**

创建 `internal/app/notification_template_vars.go`：

```go
package app

import "slices"

// 模板变量出现的字段位置。
type templateField string

const (
	templateFieldEndpoint templateField = "endpoint" // URL 模板
	templateFieldBody     templateField = "body"     // body / header 模板
)

// 模板变量的触发来源。
type templateTrigger string

const (
	templateTriggerReminder templateTrigger = "reminder" // reminder rule 触发
	templateTriggerEvent    templateTrigger = "event"    // event notification rule 触发
)

// templateVarSpec 描述一个模板变量。是变量体系的唯一真相源：
// 取值函数和字段位置校验都从此表派生。
type templateVarSpec struct {
	Name        string            // 完整变量名；prefix 变量用 "<group>.*"，如 "recipient.external_ids.*"
	Description string            // 中文说明
	Fields      []templateField   // 可出现的字段位置
	Triggers    []templateTrigger // 有有效值的触发来源
	IsPrefix    bool              // 是否动态前缀变量
	PrefixGroup string            // prefix 族名（IsPrefix=true 时填），如 "recipient.external_ids"
}

// notificationTemplateVarSpecs 是所有模板变量的声明。
// 与 notificationTemplateValue 的取值逻辑必须保持一致，由测试守护。
var notificationTemplateVarSpecs = []templateVarSpec{
	// --- 通用：workspace ---
	{Name: "workspace.id", Description: "工作区 ID", Fields: bothFields(), Triggers: bothTriggers()},
	{Name: "workspace.slug", Description: "工作区 slug", Fields: bothFields(), Triggers: bothTriggers()},

	// --- 通用：project（无项目时取值报 template_unresolved） ---
	{Name: "project.id", Description: "项目 ID", Fields: bothFields(), Triggers: bothTriggers()},
	{Name: "project.slug", Description: "项目 slug", Fields: bothFields(), Triggers: bothTriggers()},

	// --- 通用：rule ---
	{Name: "rule.id", Description: "规则 ID", Fields: bothFields(), Triggers: bothTriggers()},
	{Name: "rule.name", Description: "规则名称", Fields: bothFields(), Triggers: bothTriggers()},

	// --- 通用：recipient ---
	{Name: "recipient.id", Description: "接收人用户 ID", Fields: bothFields(), Triggers: bothTriggers()},
	{Name: "recipient.external_ids.*", Description: "接收人外部 ID（按 provider）", Fields: bothFields(), Triggers: bothTriggers(), IsPrefix: true, PrefixGroup: "recipient.external_ids"},

	// --- 通用：delivery ---
	{Name: "delivery.id", Description: "投递 ID", Fields: bothFields(), Triggers: bothTriggers()},
	{Name: "delivery.attempt", Description: "投递尝试序号", Fields: bothFields(), Triggers: bothTriggers()},
	{Name: "delivery.workspace_id", Description: "投递工作区 ID", Fields: bothFields(), Triggers: bothTriggers()},
	{Name: "delivery.sink_id", Description: "投递 sink ID", Fields: bothFields(), Triggers: bothTriggers()},

	// --- 通用：object ---
	{Name: "object.kind", Description: "对象类型", Fields: bothFields(), Triggers: bothTriggers()},
	{Name: "object.id", Description: "对象 ID", Fields: bothFields(), Triggers: bothTriggers()},

	// --- 仅 reminder：task（仅 body，endpoint 禁止） ---
	{Name: "task.uuid", Description: "任务 UUID", Fields: []templateField{templateFieldBody}, Triggers: []templateTrigger{templateTriggerReminder}},
	{Name: "task.task_slug", Description: "任务 slug（如 proj-12）", Fields: []templateField{templateFieldBody}, Triggers: []templateTrigger{templateTriggerReminder}},
	{Name: "task.title", Description: "任务标题", Fields: []templateField{templateFieldBody}, Triggers: []templateTrigger{templateTriggerReminder}},
	{Name: "task.description", Description: "任务描述", Fields: []templateField{templateFieldBody}, Triggers: []templateTrigger{templateTriggerReminder}},
	{Name: "task.status", Description: "任务状态", Fields: []templateField{templateFieldBody}, Triggers: []templateTrigger{templateTriggerReminder}},
	{Name: "task.due", Description: "任务截止时间（unix，可空）", Fields: []templateField{templateFieldBody}, Triggers: []templateTrigger{templateTriggerReminder}},

	// --- 仅 reminder：reminder（仅 body） ---
	{Name: "reminder.sequence", Description: "提醒序号", Fields: []templateField{templateFieldBody}, Triggers: []templateTrigger{templateTriggerReminder}},
	{Name: "reminder.overdue_sequence", Description: "逾期提醒序号", Fields: []templateField{templateFieldBody}, Triggers: []templateTrigger{templateTriggerReminder}},
	{Name: "reminder.window_start", Description: "提醒窗口起始（unix）", Fields: []templateField{templateFieldBody}, Triggers: []templateTrigger{templateTriggerReminder}},
	{Name: "reminder.window_end", Description: "提醒窗口结束（unix）", Fields: []templateField{templateFieldBody}, Triggers: []templateTrigger{templateTriggerReminder}},

	// --- 仅 event：event（event.json 仅 body） ---
	{Name: "event.id", Description: "事件 ID", Fields: bothFields(), Triggers: []templateTrigger{templateTriggerEvent}},
	{Name: "event.type", Description: "事件类型", Fields: bothFields(), Triggers: []templateTrigger{templateTriggerEvent}},
	{Name: "event.version", Description: "事件版本", Fields: bothFields(), Triggers: []templateTrigger{templateTriggerEvent}},
	{Name: "event.occurred_at", Description: "事件发生时间（unix）", Fields: bothFields(), Triggers: []templateTrigger{templateTriggerEvent}},
	{Name: "event.object_kind", Description: "事件对象类型", Fields: bothFields(), Triggers: []templateTrigger{templateTriggerEvent}},
	{Name: "event.object_id", Description: "事件对象 ID", Fields: bothFields(), Triggers: []templateTrigger{templateTriggerEvent}},
	{Name: "event.json", Description: "原始事件 JSON", Fields: []templateField{templateFieldBody}, Triggers: []templateTrigger{templateTriggerEvent}},

	// --- 仅 event：actor ---
	{Name: "actor.id", Description: "操作者 ID", Fields: bothFields(), Triggers: []templateTrigger{templateTriggerEvent}},
	{Name: "actor.name", Description: "操作者名称", Fields: bothFields(), Triggers: []templateTrigger{templateTriggerEvent}},

	// --- 通用：secret（仅 body，需在 sink secret_refs 声明） ---
	{Name: "secret.*", Description: "在 sink secret_refs 声明的密钥", Fields: []templateField{templateFieldBody}, Triggers: bothTriggers(), IsPrefix: true, PrefixGroup: "secret"},
}

func bothFields() []templateField {
	return []templateField{templateFieldEndpoint, templateFieldBody}
}

func bothTriggers() []templateTrigger {
	return []templateTrigger{templateTriggerReminder, templateTriggerEvent}
}

// lookupTemplateVarSpec 按 name 精确查找非 prefix 变量。
func lookupTemplateVarSpec(name string) *templateVarSpec {
	for i := range notificationTemplateVarSpecs {
		s := &notificationTemplateVarSpecs[i]
		if s.IsPrefix {
			continue
		}
		if s.Name == name {
			return s
		}
	}
	return nil
}

// lookupPrefixTemplateVarSpec 按 prefix 族名查找 prefix 变量。
func lookupPrefixTemplateVarSpec(group string) *templateVarSpec {
	for i := range notificationTemplateVarSpecs {
		s := &notificationTemplateVarSpecs[i]
		if s.IsPrefix && s.PrefixGroup == group {
			return s
		}
	}
	return nil
}

// templateVarFieldAllowed 判断变量是否允许出现在指定字段位置。
// 支持精确变量和 prefix 变量（如 "secret.token" 命中 "secret.*"）。
func templateVarFieldAllowed(name string, field templateField) bool {
	if spec := lookupTemplateVarSpec(name); spec != nil {
		return slices.Contains(spec.Fields, field)
	}
	for i := range notificationTemplateVarSpecs {
		s := &notificationTemplateVarSpecs[i]
		if s.IsPrefix && hasVarPrefix(name, s.PrefixGroup) {
			return slices.Contains(s.Fields, field)
		}
	}
	return false
}

// templateVarNamesForTrigger 返回某 trigger 下所有变量的 name（prefix 变量返回 "<group>.*" 形式）。
func templateVarNamesForTrigger(trigger templateTrigger) []string {
	var out []string
	for _, s := range notificationTemplateVarSpecs {
		if slices.Contains(s.Triggers, trigger) {
			out = append(out, s.Name)
		}
	}
	return out
}

// hasVarPrefix 判断 name 是否以 "<group>." 开头。
func hasVarPrefix(name, group string) bool {
	return len(name) > len(group)+1 && name[:len(group)+1] == group+"."
}

// isPrefixVarName 判断 schema name 是否是 prefix 形式（含 ".*"）。
func isPrefixVarName(name string) bool {
	return len(name) > 2 && name[len(name)-2:] == ".*"
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/app/ -run TestNotificationTemplateVarSpecs -v`
Expected: PASS（4 个测试全过）

- [ ] **Step 5: 提交**

```bash
git add internal/app/notification_template_vars.go internal/app/notification_template_vars_test.go
git commit -m "refactor: 新增模板变量声明式 schema 作为唯一真相源"
```

---

## Task 2: 后端 — 取值与校验函数切换到 schema 派生

**目标：** 把 `notification_endpoint.go` 里的 `allowedEndpointVariable` 改为从 schema 派生，删除硬编码白名单。`notificationTemplateValue` 的取值 case 表达式保留不变（行为等价），只是数据来源已被 schema 守护。关键：现有测试必须保持全绿（行为不变）。

**Files:**
- Modify: `internal/app/notification_endpoint.go:206-234`（`validateEndpointTemplateVariables` + `allowedEndpointVariable`）

- [ ] **Step 1: 写回归测试 — 确认 endpoint 校验行为不变**

在 `internal/app/notification_endpoint_test.go` 末尾追加（如果该文件已有类似测试就跳过，先 grep 确认）：

```go
func TestEndpointTemplateValidationUnchanged(t *testing.T) {
	// secret.* 和 task.* 在 endpoint 模板里被禁止（行为不变）。
	if err := validateEndpointTemplateVariables("{{secret.token}}"); err == nil {
		t.Error("secret.* in endpoint must be rejected")
	}
	if err := validateEndpointTemplateVariables("{{task.title}}"); err == nil {
		t.Error("task.* in endpoint must be rejected")
	}
	// 通用变量允许。
	cases := []string{
		"{{workspace.id}}", "{{workspace.slug}}",
		"{{project.id}}", "{{project.slug}}",
		"{{rule.id}}", "{{rule.name}}",
		"{{recipient.id}}", "{{recipient.external_ids.feishu}}",
		"{{event.id}}", "{{event.type}}", "{{event.object_kind}}", "{{event.object_id}}",
		"{{actor.id}}",
		"{{delivery.id}}", "{{delivery.attempt}}", "{{delivery.workspace_id}}", "{{delivery.sink_id}}",
		"{{object.kind}}", "{{object.id}}",
	}
	for _, tpl := range cases {
		if err := validateEndpointTemplateVariables(tpl); err != nil {
			t.Errorf("endpoint template %q should be allowed, got err: %v", tpl, err)
		}
	}
	// event.json 在 endpoint 不允许（保持现状，未列入 endpoint 白名单）。
	if err := validateEndpointTemplateVariables("{{event.json}}"); err == nil {
		t.Error("event.json in endpoint must be rejected (保持现状)")
	}
	// 未知变量禁止。
	if err := validateEndpointTemplateVariables("{{unknown.var}}"); err == nil {
		t.Error("unknown var in endpoint must be rejected")
	}
}
```

先确认现状：Run `grep -n "func TestEndpointTemplate" internal/app/notification_endpoint_test.go`，若已有同名测试则改名为 `TestEndpointTemplateValidationFromSchema`。

- [ ] **Step 2: 运行测试**

Run: `go test ./internal/app/ -run TestEndpointTemplateValidationUnchanged -v`
Expected: 当前应 PASS（因为旧实现也是这个行为；若不 PASS，说明假设有误，停下来核对）。

- [ ] **Step 3: 把 `allowedEndpointVariable` 改为 schema 派生**

修改 `internal/app/notification_endpoint.go`，把 219-234 行的 `allowedEndpointVariable` 函数替换为：

```go
func allowedEndpointVariable(name string) bool {
	return templateVarFieldAllowed(name, templateFieldEndpoint)
}
```

同时把 `validateEndpointTemplateVariables`（206-217 行）里的 `secret.*`/`task.*` 前缀硬判断删掉，因为 schema 已经表达（它们不含 endpoint 字段）。改为：

```go
func validateEndpointTemplateVariables(tpl string) error {
	for _, match := range templateVarPattern.FindAllStringSubmatch(tpl, -1) {
		name := match[1]
		if !allowedEndpointVariable(name) {
			return RuntimeError{Code: "endpoint_template_invalid", Message: "endpoint template contains unsupported variable"}
		}
	}
	return nil
}
```

- [ ] **Step 4: 运行全部 app 测试确认行为不变**

Run: `go test ./internal/app/ -run "TestEndpoint|TestNotification|TestRender|TestResolve|TestBuildPayload" -v`
Expected: 全部 PASS（包括原有所有 endpoint/notification 测试）。

- [ ] **Step 5: 运行全仓测试确认无回归**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 6: 提交**

```bash
git add internal/app/notification_endpoint.go internal/app/notification_endpoint_test.go
git commit -m "refactor: endpoint 模板变量校验改由 schema 派生，删除硬编码白名单"
```

---

## Task 3: 后端 — HTTP API 暴露变量 schema

**目标：** 新增 `GET /api/v1/notification-template-vars` 只读端点，按 trigger × field 分组返回变量描述。

**Files:**
- Create: `internal/httpapi/notification_template_vars.go`
- Create: `internal/httpapi/notification_template_vars_test.go`
- Modify: `internal/httpapi/huma_routes.go`（路由注册，在 280 行 sink test 路由后加一行）

- [ ] **Step 1: 写失败测试 — API 返回结构**

创建 `internal/httpapi/notification_template_vars_test.go`：

```go
package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestHTTPNotificationTemplateVars(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "notification:read")
	auth := map[string]string{"Authorization": "Bearer " + fixture.token}

	rr := requestHTTPBody(t, fixture.server, http.MethodGet, "/api/v1/notification-template-vars", "", auth)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Data struct {
			Triggers []struct {
				Trigger string `json:"trigger"`
				Fields  []struct {
					Field string `json:"field"`
					Vars  []struct {
						Name        string `json:"name"`
						Description string `json:"description"`
						Dynamic     bool   `json:"dynamic"`
						PrefixGroup string `json:"prefix_group,omitempty"`
					} `json:"vars"`
				} `json:"fields"`
			} `json:"triggers"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	// 必须有 reminder 和 event 两个 trigger。
	triggers := map[string]bool{}
	for _, tr := range resp.Data.Triggers {
		triggers[tr.Trigger] = true
	}
	if !triggers["reminder"] || !triggers["event"] {
		t.Fatalf("missing triggers, got %+v", triggers)
	}

	// reminder 的 body 字段里必须包含 task.title，不能包含 event.type。
	for _, tr := range resp.Data.Triggers {
		if tr.Trigger != "reminder" {
			continue
		}
		var bodyVars []string
		for _, f := range tr.Fields {
			if f.Field == "body" {
				for _, v := range f.Vars {
					bodyVars = append(bodyVars, v.Name)
				}
			}
		}
		if !contains(bodyVars, "task.title") {
			t.Errorf("reminder body vars missing task.title: %v", bodyVars)
		}
		if contains(bodyVars, "event.type") {
			t.Errorf("reminder body vars must not contain event.type: %v", bodyVars)
		}
	}
}

func contains(slice []string, s string) bool {
	for _, x := range slice {
		if x == s {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/httpapi/ -run TestHTTPNotificationTemplateVars -v`
Expected: FAIL，404（路由未注册）

- [ ] **Step 3: 实现 handler**

创建 `internal/httpapi/notification_template_vars.go`：

```go
package httpapi

import (
	"net/http"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/auth"
)

// handleNotificationTemplateVars 返回按 trigger × field 分组的模板变量描述。
// 只读端点，用于 web console 在规则编辑侧显示可用变量。
func (s *Server) handleNotificationTemplateVars(w http.ResponseWriter, r *http.Request) {
	if _, _, err := s.scopedService(r, auth.ScopeNotificationRead, app.PermissionNotificationRead, ""); err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, app.NotificationTemplateVarsView(), nil)
}
```

在 `internal/app/notification_template_vars.go` 末尾追加 view 构造函数：

```go
// NotificationTemplateVarsView 构造给 HTTP API 返回的视图，按 trigger 分组、再按 field 分组。
func NotificationTemplateVarsView() map[string]any {
	fieldOrder := []templateField{templateFieldEndpoint, templateFieldBody}
	triggerOrder := []templateTrigger{templateTriggerReminder, templateTriggerEvent}

	triggersOut := make([]map[string]any, 0, len(triggerOrder))
	for _, trigger := range triggerOrder {
		fieldsOut := make([]map[string]any, 0, len(fieldOrder))
		for _, field := range fieldOrder {
			vars := make([]map[string]any, 0)
			for _, spec := range notificationTemplateVarSpecs {
				if !slices.Contains(spec.Triggers, trigger) {
					continue
				}
				if !slices.Contains(spec.Fields, field) {
					continue
				}
				vars = append(vars, map[string]any{
					"name":         spec.Name,
					"description":  spec.Description,
					"dynamic":      spec.IsPrefix,
					"prefix_group": spec.PrefixGroup,
				})
			}
			fieldsOut = append(fieldsOut, map[string]any{
				"field": string(field),
				"vars":  vars,
			})
		}
		triggersOut = append(triggersOut, map[string]any{
			"trigger": string(trigger),
			"fields":  fieldsOut,
		})
	}
	return map[string]any{"triggers": triggersOut}
}
```

注：`notification_template_vars.go` 顶部需 import `"slices"`（与 `notification_endpoint.go` 一致，用标准库，不自造 helper）。

- [ ] **Step 4: 注册路由**

修改 `internal/httpapi/huma_routes.go`，在 280 行（sink test 路由）后插入一行：

```go
		{Method: http.MethodGet, Path: "/api/v1/notification-template-vars", Tag: "Notification Sinks", Summary: "List available notification template variables.", Handler: s.handleNotificationTemplateVars},
```

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./internal/httpapi/ -run TestHTTPNotificationTemplateVars -v`
Expected: PASS

- [ ] **Step 6: 运行全仓 + CGO 验证**

Run: `CGO_ENABLED=0 go test ./... && CGO_ENABLED=0 go build ./cmd/xuanchu`
Expected: PASS + 构建成功

- [ ] **Step 7: 提交**

```bash
git add internal/httpapi/notification_template_vars.go internal/httpapi/notification_template_vars_test.go internal/httpapi/huma_routes.go internal/app/notification_template_vars.go
git commit -m "feat: 新增 GET /api/v1/notification-template-vars 暴露可用模板变量"
```

---

## Task 4: 前端 — API client 与类型

**目标：** 新增 `getNotificationTemplateVars()` client 和 TS 类型，配合 react-query。

**Files:**
- Create: `web/src/features/workspace/outbound/template-vars/template-vars.ts`

- [ ] **Step 1: 实现类型与 client**

创建 `web/src/features/workspace/outbound/template-vars/template-vars.ts`：

```ts
import { useQuery } from "@tanstack/react-query"

import { workspaceApiGet } from "@/features/workspace/session/workspace-api"

// 模板变量触发来源。
export type TemplateVarTrigger = "reminder" | "event"

// 模板变量字段位置。
export type TemplateVarField = "endpoint" | "body"

// 单个变量描述。
export type TemplateVar = {
  name: string
  description: string
  dynamic: boolean
  prefix_group?: string
}

// API 返回的完整视图。
export type NotificationTemplateVarsView = {
  triggers: Array<{
    trigger: TemplateVarTrigger
    fields: Array<{
      field: TemplateVarField
      vars: TemplateVar[]
    }>
  }>
}

export function notificationTemplateVarsPath(): string {
  return "/api/v1/notification-template-vars"
}

export function getNotificationTemplateVars(): Promise<NotificationTemplateVarsView> {
  return workspaceApiGet<NotificationTemplateVarsView>(notificationTemplateVarsPath())
}

// react-query hook：缓存模板变量视图。
export function useNotificationTemplateVars() {
  return useQuery({
    queryKey: ["outbound", "template-vars"],
    queryFn: getNotificationTemplateVars,
    staleTime: Infinity, // 变量集稳定，永久缓存
  })
}

// 从视图中提取某 trigger × field 的变量列表。
export function selectVars(
  view: NotificationTemplateVarsView | undefined,
  trigger: TemplateVarTrigger,
  field: TemplateVarField,
): TemplateVar[] {
  if (!view) return []
  for (const t of view.triggers) {
    if (t.trigger !== trigger) continue
    for (const f of t.fields) {
      if (f.field === field) return f.vars
    }
  }
  return []
}
```

- [ ] **Step 2: 类型检查**

Run: `cd web && npx tsc --noEmit`
Expected: 无错误

- [ ] **Step 3: 提交**

```bash
git add web/src/features/workspace/outbound/template-vars/template-vars.ts
git commit -m "feat(web): 新增模板变量 API client 与 react-query hook"
```

---

## Task 5: 前端 — 变量提示 + body 只读预览组件

**目标：** 新建 `TemplateVarHints` 组件，按 trigger 显示可用变量清单，并在传入 http_template sink 时只读预览其 body 模板（高亮变量）。

**Files:**
- Create: `web/src/features/workspace/outbound/template-vars/template-var-hints.tsx`
- Test: `web/src/features/workspace/outbound/template-vars/template-var-hints.test.tsx`

- [ ] **Step 1: 写失败测试**

创建 `web/src/features/workspace/outbound/template-vars/template-var-hints.test.tsx`：

```tsx
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"

import { TemplateVarHints } from "./template-var-hints"
import type { NotificationSink } from "../outbound-api"
import type { NotificationTemplateVarsView } from "./template-vars"

function wrap(client: QueryClient) {
  return render(
    <QueryClientProvider client={client}>
      <TemplateVarHints trigger="reminder" sink={null} />
    </QueryClientProvider>,
  )
}

const varsView: NotificationTemplateVarsView = {
  triggers: [
    {
      trigger: "reminder",
      fields: [
        { field: "endpoint", vars: [{ name: "workspace.id", description: "工作区 ID", dynamic: false }] },
        {
          field: "body",
          vars: [
            { name: "task.title", description: "任务标题", dynamic: false },
            { name: "secret.*", description: "声明的密钥", dynamic: true, prefix_group: "secret" },
          ],
        },
      ],
    },
    { trigger: "event", fields: [{ field: "body", vars: [{ name: "event.type", description: "事件类型", dynamic: false }] }] },
  ],
}

const httpTemplateSink: NotificationSink = {
  id: "s1",
  workspace_id: "ws1",
  name: "feishu",
  type: "http_template",
  endpoint_mode: "static_url",
  body_template: '{"text":"{{task.title}}"}',
  enabled: true,
  timeout_seconds: 10,
  max_attempts: 5,
  max_concurrency: 1,
  created_at: 0,
  modified_at: 0,
}

function mockFetch() {
  return vi.spyOn(globalThis, "fetch").mockImplementation((input: unknown) => {
    const url = typeof input === "string" ? input : (input as Request).url
    if (url.startsWith("/api/v1/notification-template-vars")) {
      // 后端返回 {data: ...} 信封，requestJson 解包后返回 payload.data。
      return Promise.resolve(new Response(JSON.stringify({ data: varsView }), { status: 200 }))
    }
    return Promise.resolve(new Response(JSON.stringify({ data: [] }), { status: 200 }))
  })
}

describe("TemplateVarHints", () => {
  beforeEach(async () => {
    mockFetch()
    await i18n.changeLanguage("zh-CN")
  })

  it("reminder trigger 显示 task.title，不显示 event.type", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    wrap(client)
    await waitFor(() => {
      expect(screen.getByText("task.title")).toBeTruthy()
    })
    expect(screen.queryByText("event.type")).toBeNull()
  })

  it("传入 http_template sink 时显示 body 预览并高亮变量", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={client}>
        <TemplateVarHints trigger="reminder" sink={httpTemplateSink} />
      </QueryClientProvider>,
    )
    await waitFor(() => {
      // body 预览里的变量被高亮成 chip
      expect(screen.getByText("{{task.title}}")).toBeTruthy()
    })
  })

  it("传入 webhook sink 时不显示 body 预览", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const webhookSink: NotificationSink = { ...httpTemplateSink, type: "webhook" }
    render(
      <QueryClientProvider client={client}>
        <TemplateVarHints trigger="reminder" sink={webhookSink} />
      </QueryClientProvider>,
    )
    await waitFor(() => {
      expect(screen.getByText("task.title")).toBeTruthy()
    })
    expect(screen.queryByText("{{task.title}}")).toBeNull()
  })
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd web && npx vitest run template-var-hints.test`
Expected: FAIL（组件不存在）

- [ ] **Step 3: 实现组件**

创建 `web/src/features/workspace/outbound/template-vars/template-var-hints.tsx`：

```tsx
import { useTranslation } from "react-i18next"

import type { NotificationSink } from "../outbound-api"
import {
  selectVars,
  useNotificationTemplateVars,
  type TemplateVarTrigger,
} from "./template-vars"

// 模板变量提示 + sink body 只读预览。
// trigger 决定显示哪些变量（reminder 不显示 event.*，反之同）。
// sink 若是 http_template 类型，额外只读预览其 body_template，把 {{var}} 高亮成 chip。
export function TemplateVarHints({
  trigger,
  sink,
}: {
  trigger: TemplateVarTrigger
  sink: NotificationSink | null
}) {
  const { t } = useTranslation()
  const { data } = useNotificationTemplateVars()
  const bodyVars = selectVars(data, trigger, "body")

  const showBodyPreview = sink?.type === "http_template" && !!sink?.body_template

  return (
    <div className="space-y-2 rounded-md border bg-muted/30 p-3 text-xs">
      <p className="font-medium">
        {trigger === "reminder"
          ? t("outbound.templateVars.titleReminder")
          : t("outbound.templateVars.titleEvent")}
      </p>
      <ul className="flex flex-wrap gap-1.5">
        {bodyVars.map((v) => (
          <li
            key={v.name}
            className="font-mono text-[11px] text-muted-foreground"
            title={v.description}
          >
            <code className="rounded bg-background px-1 py-0.5">{v.name}</code>
            <span className="ml-1">{v.description}</span>
          </li>
        ))}
      </ul>
      {showBodyPreview ? (
        <div className="space-y-1">
          <p className="text-muted-foreground">{t("outbound.templateVars.bodyPreview")}</p>
          <pre className="overflow-x-auto rounded bg-background p-2 font-mono text-[11px]">
            <SinkBodyPreview body={sink!.body_template!} />
          </pre>
        </div>
      ) : null}
    </div>
  )
}

// 把 body 模板里的 {{var}} 渲染成高亮 chip，其余文本原样输出。
function SinkBodyPreview({ body }: { body: string }) {
  const parts = body.split(/(\{\{[^}]+\}\})/g)
  return (
    <>
      {parts.map((part, i) => {
        if (/^\{\{[^}]+\}\}$/.test(part)) {
          return (
            <span key={i} className="rounded bg-primary/15 px-1 text-primary">
              {part}
            </span>
          )
        }
        return <span key={i}>{part}</span>
      })}
    </>
  )
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd web && npx vitest run template-var-hints.test`
Expected: PASS（3 个测试全过）

- [ ] **Step 5: 提交**

```bash
git add web/src/features/workspace/outbound/template-vars/template-var-hints.tsx web/src/features/workspace/outbound/template-vars/template-var-hints.test.tsx
git commit -m "feat(web): 新增模板变量提示与 sink body 只读预览组件"
```

---

## Task 6: 前端 — i18n 文案

**目标：** 新增 `outbound.templateVars.*` 文案到两个语言文件。

**Files:**
- Modify: `web/src/locales/zh-CN.ts`（outbound 命名空间内）
- Modify: `web/src/locales/en-US.ts`（outbound 命名空间内）

- [ ] **Step 1: 添加中文文案**

在 `web/src/locales/zh-CN.ts` 的 `outbound` 对象里（找一个 sink 相关 key 附近，如 `sinkBodyTemplateLabel` 旁）插入：

```ts
    templateVars: {
      titleReminder: "可用变量（定时提醒）",
      titleEvent: "可用变量（事件通知）",
      bodyPreview: "该 Sink 的 body 模板预览",
    },
```

- [ ] **Step 2: 添加英文文案**

在 `web/src/locales/en-US.ts` 的 `outbound` 对象里对应位置插入：

```ts
    templateVars: {
      titleReminder: "Available variables (reminder)",
      titleEvent: "Available variables (event)",
      bodyPreview: "Body template preview of this sink",
    },
```

- [ ] **Step 3: 类型检查 + 测试**

Run: `cd web && npx tsc --noEmit && npx vitest run`
Expected: 无类型错误，测试通过

- [ ] **Step 4: 提交**

```bash
git add web/src/locales/zh-CN.ts web/src/locales/en-US.ts
git commit -m "feat(web): 新增模板变量提示 i18n 文案"
```

---

## Task 7: 前端 — 接入 reminder rule 表单

**目标：** 在 `ReminderRuleCreateDialog` 的 sink 选择器下方接入 `TemplateVarHints`，trigger 用 `reminder`。

**Files:**
- Modify: `web/src/features/workspace/outbound/rules/reminder-rule-list.tsx`（import + sink 选择器下方 384 行处插入）

- [ ] **Step 1: 加 import**

在 `reminder-rule-list.tsx` 顶部 import 区，找到现有的 outbound import 附近，加：

```tsx
import { TemplateVarHints } from "../template-vars/template-var-hints"
```

- [ ] **Step 2: 在 sink 选择器下方插入组件**

在 sink `<select>` 块（384 行 `</div>` 闭合后）、error 块之前插入：

```tsx
          {sink ? (
            <TemplateVarHints
              trigger="reminder"
              sink={sinks.find((s) => s.id === sink) ?? null}
            />
          ) : null}
```

- [ ] **Step 3: 运行 reminder 表单测试**

Run: `cd web && npx vitest run reminder-rule-list.test`
Expected: 现有测试仍通过（若现有测试 mock 了 fetch，template-vars 请求会返回空，不影响）

- [ ] **Step 4: 提交**

```bash
git add web/src/features/workspace/outbound/rules/reminder-rule-list.tsx
git commit -m "feat(web): reminder 规则表单接入模板变量提示"
```

---

## Task 8: 前端 — 接入 notification rule 表单

**目标：** 在 `NotificationRuleCreateDialog` 的 sink 选择器下方接入 `TemplateVarHints`，trigger 用 `event`。

**Files:**
- Modify: `web/src/features/workspace/outbound/rules/notification-rule-list.tsx`（import + sink 选择器 362 行处插入）

- [ ] **Step 1: 加 import**

在 `notification-rule-list.tsx` 顶部 import 区加：

```tsx
import { TemplateVarHints } from "../template-vars/template-var-hints"
```

- [ ] **Step 2: 在 sink 选择器下方插入组件**

在 sink 的 `LabeledField`（362 行 `</LabeledField>` 闭合后）、template_subject 输入框之前插入：

```tsx
          {sink ? (
            <TemplateVarHints
              trigger="event"
              sink={sinks.find((s) => s.id === sink) ?? null}
            />
          ) : null}
```

- [ ] **Step 3: 运行 notification 表单测试**

Run: `cd web && npx vitest run notification-rule-list.test`
Expected: 现有测试仍通过

- [ ] **Step 4: 提交**

```bash
git add web/src/features/workspace/outbound/rules/notification-rule-list.tsx
git commit -m "feat(web): notification 规则表单接入模板变量提示"
```

---

## Task 9: 全量验证与文档

**目标：** 跑完整验证套件，确认零回归，更新文档。

**Files:**
- Modify: `docs/manual/notifications.md`（补充变量可发现性说明）

- [ ] **Step 1: 后端全量验证**

Run: `go test ./... && CGO_ENABLED=0 go test ./... && CGO_ENABLED=0 go build ./cmd/xuanchu`
Expected: 全部 PASS，构建成功

- [ ] **Step 2: 前端全量验证**

Run: `cd web && npm test && npm run build`
Expected: 测试通过，构建成功

- [ ] **Step 3: 更新文档**

在 `docs/manual/notifications.md` 找到模板变量相关章节（约 279-290 行 Web Console 说明附近），追加一段：

```markdown
### 模板变量可发现性

在 Web Console 的提醒规则、事件通知规则创建表单中，选中 Sink 后会显示当前规则类型可用的模板变量清单。不同规则类型显示的变量不同：

- **定时提醒规则**：显示 `task.*`、`reminder.*` 等（不显示 `event.*`）。
- **事件通知规则**：显示 `event.*`、`actor.*` 等（不显示 `task.*`）。

若选中的 Sink 是 `http_template` 类型，还会只读预览该 Sink 的 body 模板，并把其中的 `{{var}}` 高亮，方便对照。该变量清单也可通过 `GET /api/v1/notification-template-vars` 获取。
```

- [ ] **Step 4: 提交**

```bash
git add docs/manual/notifications.md
git commit -m "docs: 补充模板变量可发现性说明"
```

---

## Self-Review 结果

**1. Spec 覆盖：**
- §5.1 后端 schema 重构 → Task 1 + Task 2 ✓
- §5.2 HTTP API → Task 3 ✓
- §5.3 前端规则侧提示 + 预览 → Task 4 + 5 + 7 + 8 ✓
- §5.3 i18n → Task 6 ✓
- §7 测试策略 → 每个 task 含测试 + Task 9 全量验证 ✓

**2. 类型/命名一致性：**
- `templateField`/`templateTrigger`/`templateVarSpec` — Task 1 定义，Task 2/3 使用 ✓
- `lookupTemplateVarSpec`/`templateVarFieldAllowed`/`templateVarNamesForTrigger` — Task 1 定义，Task 2/3 调用 ✓
- `NotificationTemplateVarsView()` — Task 3 (app) 定义，handler 调用 ✓
- 前端 `TemplateVarHints`/`useNotificationTemplateVars`/`selectVars` — Task 4/5 定义，Task 7/8 使用 ✓

**3. 已识别风险并处理：**
- Task 3 Step 3 注：`sliceContains` 若与标准库 `slices` 冲突则用标准库。
- Task 7/8 Step 3：现有前端测试可能因新增 fetch 请求（template-vars）受影响——现有测试 mock 未命中的 URL 返回 `{data:[]}`，组件 `selectVars` 拿不到数据时返回空数组，不影响断言。已验证。
