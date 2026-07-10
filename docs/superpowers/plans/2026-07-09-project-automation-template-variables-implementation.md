# 项目自动化提示词模板变量化 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把自动化规则的 system prompt 和 instruction template 从硬编码改为用户可控的模板，支持 `{{变量}}` 占位符引用项目上下文，前端用 Textarea 编辑并通过「插入变量」按钮选择变量。

**Architecture:** 后端新增 `system_prompt` 字段和模板变量定义，渲染时用正则替换 `{{变量}}`。前端在规则编辑 Dialog 新增 system prompt Textarea，两个 Textarea 下方各放「插入变量」按钮（Popover + Command），点击追加 `{{变量名}}` 到文本末尾。

**Tech Stack:** Go 1.25 / GORM / SQLite `github.com/glebarez/sqlite` / chi HTTP / React + TypeScript + TanStack Query + shadcn/ui + vitest + testing-library

**Spec:** `docs/superpowers/specs/2026-07-09-project-automation-template-variables-design.md`

## Global Constraints

- 主要语言：文档、注释和提交信息使用中文。
- 每次实现后必须满足 `go test ./...`、`CGO_ENABLED=0 go test ./...`、`CGO_ENABLED=0 go build ./cmd/xuanchu`。
- Web 改动必须满足 `pnpm --dir web test`、`pnpm --dir web typecheck`、`pnpm --dir web lint`、`pnpm --dir web build`。
- 不引入富文本编辑器或 Tiptap，system prompt 和 instruction template 都用 Textarea。
- 变量选择器不监听输入事件，只通过「插入变量」按钮触发。
- 不改 notification/hook 的模板变量体系。
- 向后兼容：旧规则（无 system_prompt）使用默认值；旧 instruction_template 不含 `{{}}` 时行为不变。

---

## 文件结构

**新建后端文件：**
- `internal/app/project_automation_template_vars.go` — 模板变量声明式定义 + `AutomationTemplateVarsView()` + `renderAutomationTemplate()` 渲染函数。
- `internal/app/project_automation_template_vars_test.go` — 渲染和变量定义测试。
- `internal/httpapi/project_automation_template_vars.go` — `GET /automation-template-vars` handler。
- `internal/httpapi/project_automation_template_vars_test.go` — HTTP 测试。

**修改后端文件：**
- `internal/storage/models.go` — `ProjectAutomationRule` 新增 `SystemPrompt` 列。
- `internal/app/project_automation.go` — input/view 类型新增 `SystemPrompt`，CRUD 传递，normalize 不强制。
- `internal/app/project_automation_preview.go` — `renderProjectAutomationRequest` 改为模板渲染 system + user message，不再追加 `<context>` JSON。新增 `buildAutomationTemplateVars`。
- `internal/httpapi/project_automations.go` — request DTO 新增 `SystemPrompt`，传递到 app input。
- `internal/httpapi/huma_routes.go` — 注册 `GET /automation-template-vars` 路由。

**修改前端文件：**
- `web/src/features/workspace/project-workbench/automations/project-automations-api.ts` — 类型新增 `system_prompt`，新增 `getAutomationTemplateVars`。
- `web/src/features/workspace/project-workbench/automations/automation-rule-dialog.tsx` — 新增 system prompt Textarea，两个 Textarea 下方各加「插入变量」按钮。
- `web/src/features/workspace/project-workbench/automations/automation-rule-form.tsx` — 默认模板新增 `system_prompt` 默认值。
- `web/src/features/workspace/project-workbench/automations/automation-preview-dialog.tsx` — 改为展示渲染后的明文 messages。

**新建前端文件：**
- `web/src/features/workspace/project-workbench/automations/template-variable-picker.tsx` — 「插入变量」Popover + Command 组件。
- `web/src/features/workspace/project-workbench/automations/template-variable-picker.test.tsx` — 组件测试。

---

## Task 1: 后端模板变量定义和渲染引擎

**目标：** 新增模板变量声明式定义、`renderAutomationTemplate()` 渲染函数、`AutomationTemplateVarsView()` API view。

**Files:**
- Create: `internal/app/project_automation_template_vars.go`
- Create: `internal/app/project_automation_template_vars_test.go`

- [ ] **Step 1: 写失败测试 — 模板渲染**

Create `internal/app/project_automation_template_vars_test.go`:

```go
package app

import (
	"testing"
)

func TestRenderAutomationTemplate(t *testing.T) {
	vars := map[string]string{
		"project.slug":  "adsops",
		"project.name":  "广告投放优化",
		"project_config": `{"feishu.chat_id":"oc_xxx"}`,
	}
	got := renderAutomationTemplate("请检查项目 {{project.name}}（{{project.slug}}）\n配置：{{project_config}}", vars)
	want := "请检查项目 广告投放优化（adsops）\n配置：{\"feishu.chat_id\":\"oc_xxx\"}"
	if got != want {
		t.Fatalf("renderAutomationTemplate = %q, want %q", got, want)
	}
}

func TestRenderAutomationTemplateUndefinedVar(t *testing.T) {
	got := renderAutomationTemplate("hello {{undefined.var}} world", map[string]string{})
	if got != "hello  world" {
		t.Fatalf("undefined var should be empty string, got %q", got)
	}
}

func TestRenderAutomationTemplateNoVars(t *testing.T) {
	got := renderAutomationTemplate("plain text no vars", map[string]string{"project.slug": "x"})
	if got != "plain text no vars" {
		t.Fatalf("no-vars template should be unchanged, got %q", got)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/app -run 'TestRenderAutomationTemplate' -v`

Expected: FAIL，`renderAutomationTemplate` 未定义。

- [ ] **Step 3: 实现渲染函数和变量定义**

Create `internal/app/project_automation_template_vars.go`:

```go
package app

import (
	"regexp"
	"strings"
)

// automationTemplateVarSpec 声明一个模板变量的元信息。
type automationTemplateVarSpec struct {
	Name        string
	Description string
	Triggers    []string // "schedule", "event"
}

// automationTemplateVarSpecs 是全部可用变量的声明式定义，单一真相源。
var automationTemplateVarSpecs = []automationTemplateVarSpec{
	{Name: "project.id", Description: "项目 UUID", Triggers: []string{"schedule", "event"}},
	{Name: "project.slug", Description: "项目 slug", Triggers: []string{"schedule", "event"}},
	{Name: "project.name", Description: "项目名称", Triggers: []string{"schedule", "event"}},
	{Name: "project.status", Description: "项目状态", Triggers: []string{"schedule", "event"}},
	{Name: "workspace.id", Description: "workspace UUID", Triggers: []string{"schedule", "event"}},
	{Name: "workspace.slug", Description: "workspace slug", Triggers: []string{"schedule", "event"}},
	{Name: "workspace.name", Description: "workspace 名称", Triggers: []string{"schedule", "event"}},
	{Name: "project_config", Description: "项目非 secret 配置 JSON 对象", Triggers: []string{"schedule", "event"}},
	{Name: "tasks", Description: "匹配任务列表 JSON 数组", Triggers: []string{"schedule"}},
	{Name: "task_summary", Description: "任务统计摘要 JSON", Triggers: []string{"schedule"}},
	{Name: "delivery_id", Description: "本次投递 ID", Triggers: []string{"schedule", "event"}},
	{Name: "trigger_type", Description: "触发类型 schedule/event/manual_test", Triggers: []string{"schedule", "event"}},
	{Name: "event.type", Description: "事件类型（如 task.assigned）", Triggers: []string{"event"}},
	{Name: "event.id", Description: "事件 ID", Triggers: []string{"event"}},
	{Name: "task", Description: "触发事件的任务 JSON", Triggers: []string{"event"}},
	{Name: "added_assignees", Description: "新增负责人 JSON 数组（task.assigned）", Triggers: []string{"event"}},
}

// AutomationTemplateVarView 是对外暴露的单个变量信息。
type AutomationTemplateVarView struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// AutomationTemplateTriggerView 是按触发器分组的变量列表。
type AutomationTemplateTriggerView struct {
	Trigger string                    `json:"trigger"`
	Vars    []AutomationTemplateVarView `json:"vars"`
}

// AutomationTemplateVarsView 是模板变量 API 的顶层返回结构。
type AutomationTemplateVarsView struct {
	Triggers []AutomationTemplateTriggerView `json:"triggers"`
}

// AutomationTemplateVars 构建按触发器分组的变量列表。
func AutomationTemplateVars() AutomationTemplateVarsView {
	triggers := []string{"schedule", "event"}
	out := AutomationTemplateVarsView{}
	for _, trigger := range triggers {
		entry := AutomationTemplateTriggerView{Trigger: trigger}
		for _, spec := range automationTemplateVarSpecs {
			for _, t := range spec.Triggers {
				if t == trigger {
					entry.Vars = append(entry.Vars, AutomationTemplateVarView{
						Name:        spec.Name,
						Description: spec.Description,
					})
					break
				}
			}
		}
		out.Triggers = append(out.Triggers, entry)
	}
	return out
}

// templateVarPattern 匹配 {{变量名}} 占位符。
var templateVarPattern = regexp.MustCompile(`\{\{([^}]+)\}\}`)

// renderAutomationTemplate 用 vars map 替换模板中的 {{变量}} 占位符。
// 未定义变量替换为空字符串，不报错。
func renderAutomationTemplate(tpl string, vars map[string]string) string {
	return templateVarPattern.ReplaceAllStringFunc(tpl, func(match string) string {
		// 去掉 {{ }} 取变量名
		name := strings.TrimSpace(match[2 : len(match)-2])
		if v, ok := vars[name]; ok {
			return v
		}
		return ""
	})
}
```

- [ ] **Step 4: 写失败测试 — AutomationTemplateVars 完整性**

Append to `internal/app/project_automation_template_vars_test.go`:

```go
func TestAutomationTemplateVars(t *testing.T) {
	view := AutomationTemplateVars()
	if len(view.Triggers) != 2 {
		t.Fatalf("expected 2 triggers, got %d", len(view.Triggers))
	}
	// schedule 组必须包含 tasks 和 project.slug
	var scheduleVars, eventVars []string
	for _, trig := range view.Triggers {
		switch trig.Trigger {
		case "schedule":
			for _, v := range trig.Vars {
				scheduleVars = append(scheduleVars, v.Name)
			}
		case "event":
			for _, v := range trig.Vars {
				eventVars = append(eventVars, v.Name)
			}
		}
	}
	if !containsStr(scheduleVars, "tasks") || !containsStr(scheduleVars, "project.slug") {
		t.Fatalf("schedule vars missing key, got %v", scheduleVars)
	}
	if !containsStr(eventVars, "event.type") || !containsStr(eventVars, "added_assignees") {
		t.Fatalf("event vars missing key, got %v", eventVars)
	}
	// schedule 不应包含 event.type
	if containsStr(scheduleVars, "event.type") {
		t.Fatalf("schedule should not contain event.type")
	}
}

func containsStr(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}
```

- [ ] **Step 5: 运行测试**

Run: `go test ./internal/app -run 'TestRenderAutomationTemplate|TestAutomationTemplateVars' -v`

Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add internal/app/project_automation_template_vars.go internal/app/project_automation_template_vars_test.go
git commit -m "feat: 增加项目自动化模板变量定义和渲染引擎"
```

---

## Task 2: 后端 system_prompt 字段和模板渲染改造

**目标：** storage model 新增 `SystemPrompt` 列；app input/view/CRUD 新增 `SystemPrompt`；`renderProjectAutomationRequest` 改为模板渲染，不再追加 context JSON。

**Files:**
- Modify: `internal/storage/models.go`
- Modify: `internal/app/project_automation.go`
- Modify: `internal/app/project_automation_preview.go`
- Modify: `internal/app/project_automation_test.go`

- [ ] **Step 1: storage model 新增 SystemPrompt 列**

Modify `internal/storage/models.go`，在 `ProjectAutomationRule` 的 `InstructionTemplate` 之后添加：

```go
	InstructionTemplate string  `gorm:"not null;default:''"`
	SystemPrompt        string  `gorm:"not null;default:''"`
```

- [ ] **Step 2: app input/view 类型新增 SystemPrompt**

Modify `internal/app/project_automation.go`:

在 `ProjectAutomationRuleAddInput` 的 `InstructionTemplate` 字段后加：
```go
	InstructionTemplate string
	SystemPrompt        string
```

在 `ProjectAutomationRuleModifyInput` 的 `InstructionTemplate` 字段后加：
```go
	InstructionTemplate *string
	SystemPrompt        *string
```

在 `ProjectAutomationRuleView` 的 `InstructionTemplate` 字段后加：
```go
	InstructionTemplate string                         `json:"instruction_template"`
	SystemPrompt        string                         `json:"system_prompt"`
```

- [ ] **Step 3: CRUD 传递 SystemPrompt**

Modify `internal/app/project_automation.go`:

在 `AddProjectAutomationRule` 构建 `row` 时加：
```go
		InstructionTemplate: normalized.InstructionTemplate,
		SystemPrompt:        normalized.SystemPrompt,
```

在 `ModifyProjectAutomationRule` 的字段覆盖逻辑中加（在 `if input.InstructionTemplate != nil` 之后）：
```go
	if input.SystemPrompt != nil {
		next.SystemPrompt = *input.SystemPrompt
	}
```

在写回 `row` 时加：
```go
	row.SystemPrompt = normalized.SystemPrompt
```

在 `normalizeProjectAutomationAddInput` 中加（在 `input.InstructionTemplate = strings.TrimSpace(input.InstructionTemplate)` 之后）：
```go
	input.SystemPrompt = strings.TrimSpace(input.SystemPrompt)
```

在 `projectAutomationRuleAddInputFromRow` 中加：
```go
		InstructionTemplate: row.InstructionTemplate,
		SystemPrompt:        row.SystemPrompt,
```

在 `projectAutomationRuleViewFromRow` 的 return 中加：
```go
		InstructionTemplate: row.InstructionTemplate,
		SystemPrompt:        row.SystemPrompt,
```

- [ ] **Step 4: 写失败测试 — 渲染使用模板变量**

Append to `internal/app/project_automation_test.go`:

```go
func TestProjectAutomationPreviewRendersTemplateVars(t *testing.T) {
	f := newProjectAutomationServiceFixture(t)
	project, err := f.svc.AddProject(AddProjectInput{Slug: "adsops", Name: "广告投放优化"})
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	defineConfigForTest(t, f.svc, "agent.provider.base_url", false, "https://agent.example.com")
	defineConfigForTest(t, f.svc, "agent.provider.api_key", true, "sk-real-secret")
	defineConfigForTest(t, f.svc, "agent.provider.model", false, "project-operator")
	defineConfigForTest(t, f.svc, "feishu.chat_id", false, "oc_xxx")

	view, err := f.svc.PreviewProjectAutomation(project.Slug, ProjectAutomationPreviewInput{
		Name:          "每日项目巡检",
		TriggerType:   "schedule",
		TriggerConfig: ProjectAutomationTriggerConfig{ScheduleType: "daily_at", ScheduleValue: "09:30", Timezone: "Asia/Shanghai"},
		Action:        defaultAutomationActionForTest(),
		Context:       ProjectAutomationContextConfig{Include: []string{"project", "project_config"}},
		InstructionTemplate: "请检查项目 {{project.name}}（{{project.slug}}）\n配置：{{project_config}}",
		SystemPrompt:        "你是 {{project.slug}} 项目的巡检 Agent。",
	})
	if err != nil {
		t.Fatalf("PreviewProjectAutomation: %v", err)
	}
	body := string(mustJSONBytes(t, view.Body))
	// user message 应包含变量替换后的值，不含 {{}} 占位符。
	if !contains(body, "广告投放优化") || !contains(body, "adsops") {
		t.Fatalf("body missing rendered vars: %s", body)
	}
	if contains(body, "{{project.name}}") || contains(body, "{{project.slug}}") {
		t.Fatalf("body contains unreplaced vars: %s", body)
	}
	if contains(body, "feishu.chat_id") {
		t.Fatalf("body should contain project_config JSON: %s", body)
	}
	// system prompt 应包含替换后的项目 slug。
	if !contains(body, "你是 adsops 项目的巡检 Agent.") {
		t.Fatalf("body missing rendered system prompt: %s", body)
	}
	// 不再追加 <context> JSON。
	if contains(body, "<context>") {
		t.Fatalf("body should not contain <context> tag: %s", body)
	}
}

func TestProjectAutomationPreviewUsesDefaultSystemPromptWhenEmpty(t *testing.T) {
	f := newProjectAutomationServiceFixture(t)
	project, _ := f.svc.AddProject(AddProjectInput{Slug: "adsops", Name: "广告投放优化"})
	defineProviderConfigForTest(t, f.svc)
	view, err := f.svc.PreviewProjectAutomation(project.Slug, ProjectAutomationPreviewInput{
		Name:                "测试",
		TriggerType:         "schedule",
		TriggerConfig:       ProjectAutomationTriggerConfig{ScheduleType: "daily_at", ScheduleValue: "09:30", Timezone: "Asia/Shanghai"},
		Action:              defaultAutomationActionForTest(),
		Context:             ProjectAutomationContextConfig{Include: []string{"project"}},
		InstructionTemplate: "hello",
		SystemPrompt:        "", // 空 → 用默认值
	})
	if err != nil {
		t.Fatalf("PreviewProjectAutomation: %v", err)
	}
	body := string(mustJSONBytes(t, view.Body))
	if !contains(body, "你是项目自动化执行 Agent") {
		t.Fatalf("body missing default system prompt: %s", body)
	}
}
```

- [ ] **Step 5: 运行测试确认失败**

Run: `go test ./internal/app -run 'TestProjectAutomationPreviewRendersTemplateVars|TestProjectAutomationPreviewUsesDefaultSystemPrompt' -v`

Expected: FAIL，`SystemPrompt` 字段不存在或渲染未改造。

- [ ] **Step 6: 改造 renderProjectAutomationRequest**

Modify `internal/app/project_automation_preview.go`，在文件顶部新增常量：

```go
const defaultAutomationSystemPrompt = "你是项目自动化执行 Agent。你会收到来自璇础的项目上下文，请按用户指令执行。需要调用外部系统时，使用你所在 Agent 平台已配置的工具、skill、MCP 或 CLI。"
```

把 `renderProjectAutomationRequest` 的 body 构造部分（第 102-123 行，即 `ctx, err := s.buildProjectAutomationContext(...)` 到 `body := map[string]any{...}` 整段）替换为：

```go
	vars, err := s.buildAutomationTemplateVars(project, ruleID, input, triggerType, deliveryID, event)
	if err != nil {
		return ProjectAutomationRenderedRequest{}, err
	}
	systemPrompt := input.SystemPrompt
	if strings.TrimSpace(systemPrompt) == "" {
		systemPrompt = defaultAutomationSystemPrompt
	}
	renderedSystem := renderAutomationTemplate(systemPrompt, vars)
	renderedUser := renderAutomationTemplate(input.InstructionTemplate, vars)
	body := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": renderedSystem,
			},
			{
				"role":    "user",
				"content": renderedUser,
			},
		},
		"temperature": input.Action.Temperature,
	}
```

保留 `if input.Action.AttachMetadata { ... }` 块不变。

- [ ] **Step 7: 实现 buildAutomationTemplateVars**

在 `internal/app/project_automation_preview.go` 中新增（放在 `buildProjectAutomationContext` 之前）：

```go
// buildAutomationTemplateVars 构建模板变量 map，键为变量名（不含 {{}}），值为渲染后的字符串。
func (s *Service) buildAutomationTemplateVars(project ProjectView, ruleID string, input ProjectAutomationRuleAddInput, triggerType string, deliveryID string, event *HookEvent) (map[string]string, error) {
	ctx, err := s.buildProjectAutomationContext(project, ruleID, input, triggerType, deliveryID, event)
	if err != nil {
		return nil, err
	}
	vars := map[string]string{
		"delivery_id":  deliveryID,
		"trigger_type": triggerType,
	}
	// workspace
	if wsNode, ok := ctx["workspace"].(map[string]any); ok {
		vars["workspace.id"] = toString(wsNode["id"])
		vars["workspace.slug"] = toString(wsNode["slug"])
		vars["workspace.name"] = toString(wsNode["name"])
	} else {
		if ws, err := s.workspaceRepo.GetByID(s.workspaceID); err == nil {
			vars["workspace.id"] = ws.ID
			vars["workspace.slug"] = ws.Slug
			vars["workspace.name"] = ws.Name
		}
	}
	// project
	vars["project.id"] = project.ID
	vars["project.slug"] = project.Slug
	vars["project.name"] = project.Name
	vars["project.status"] = project.Status
	// project_config
	if cfg, ok := ctx["project_config"]; ok {
		vars["project_config"] = toJSONStringIndented(cfg)
	} else {
		// 即使 include 没选 project_config，也提供空 JSON
		vars["project_config"] = "{}"
	}
	// tasks / task_summary（schedule）
	if tasks, ok := ctx["matched_tasks"]; ok {
		vars["tasks"] = toJSONStringIndented(tasks)
	}
	if summary, ok := ctx["task_summary"]; ok {
		vars["task_summary"] = toJSONStringIndented(summary)
	}
	// event 相关
	if event != nil {
		vars["event.type"] = event.EventType
		vars["event.id"] = event.EventID
	}
	if taskNode, ok := ctx["task"]; ok {
		vars["task"] = toJSONStringIndented(taskNode)
	}
	if added, ok := ctx["added_assignees"]; ok {
		vars["added_assignees"] = toJSONStringIndented(added)
	}
	return vars, nil
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func toJSONStringIndented(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return ""
	}
	return string(b)
}
```

- [ ] **Step 8: 运行测试**

Run: `go test ./internal/app -run 'TestProjectAutomation' -v`

Expected: PASS。包括新的模板渲染测试和已有的 CRUD/preview/dispatcher 测试。

- [ ] **Step 9: Commit**

```bash
git add internal/storage/models.go internal/app/project_automation.go internal/app/project_automation_preview.go internal/app/project_automation_test.go
git commit -m "feat: 项目自动化 system prompt 和模板变量渲染"
```

---

## Task 3: HTTP 层 — request DTO 和 template-vars API

**目标：** HTTP request DTO 新增 `SystemPrompt`，新增 `GET /automation-template-vars` 路由和 handler。

**Files:**
- Modify: `internal/httpapi/project_automations.go`
- Create: `internal/httpapi/project_automation_template_vars.go`
- Create: `internal/httpapi/project_automation_template_vars_test.go`
- Modify: `internal/httpapi/huma_routes.go`

- [ ] **Step 1: request DTO 新增 SystemPrompt**

Modify `internal/httpapi/project_automations.go`，在 `projectAutomationRuleRequest` 的 `InstructionTemplate` 字段后加：

```go
	InstructionTemplate string                            `json:"instruction_template"`
	SystemPrompt        string                            `json:"system_prompt"`
```

在 `projectAutomationAddInput` 中加：

```go
		InstructionTemplate: req.InstructionTemplate,
		SystemPrompt:        req.SystemPrompt,
```

在 `handleProjectAutomationModify` 中构建 `ProjectAutomationRuleModifyInput` 时加：

```go
		SystemPrompt:        &req.SystemPrompt,
```

- [ ] **Step 2: 写失败测试 — template-vars API**

Create `internal/httpapi/project_automation_template_vars_test.go`:

```go
package httpapi

import (
	"net/http"
	"testing"
)

func TestHTTPAutomationTemplateVars(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "project:read", "hook:read")
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/projects/adsops/automation-template-vars", restfulFilterHeader(fixture.token))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	data := httpResponseDataMap(t, rr)
	triggers, ok := data["triggers"].([]any)
	if !ok || len(triggers) != 2 {
		t.Fatalf("triggers = %#v", data["triggers"])
	}
	// 验证 schedule 组包含 project.slug
	schedule := triggers[0].(map[string]any)
	if schedule["trigger"] != "schedule" {
		t.Fatalf("first trigger = %v", schedule["trigger"])
	}
	vars := schedule["vars"].([]any)
	found := false
	for _, v := range vars {
		if v.(map[string]any)["name"] == "project.slug" {
			found = true
		}
	}
	if !found {
		t.Fatalf("schedule vars missing project.slug: %#v", vars)
	}
}
```

- [ ] **Step 3: 运行测试确认失败**

Run: `go test ./internal/httpapi -run 'TestHTTPAutomationTemplateVars' -v`

Expected: FAIL，route not found。

- [ ] **Step 4: 实现 handler**

Create `internal/httpapi/project_automation_template_vars.go`:

```go
package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"git.dajee.net/dajee/xuanchu/internal/app"
)

func (s *Server) handleProjectAutomationTemplateVars(w http.ResponseWriter, r *http.Request) {
	projectRef := chi.URLParam(r, "projectRef")
	scoped, err := s.scopedProjectAutomationService(r, projectRef, false)
	if err != nil {
		writeAppError(w, err)
		return
	}
	_ = scoped
	view := app.AutomationTemplateVars()
	writeSuccess(w, http.StatusOK, view, nil)
}
```

- [ ] **Step 5: 注册路由**

Modify `internal/httpapi/huma_routes.go`，在 automation-deliveries 路由之前加：

```go
		{Method: http.MethodGet, Path: "/api/v1/projects/{projectRef}/automation-template-vars", Tag: "Project Automations", Summary: "List available automation template variables.", Handler: s.handleProjectAutomationTemplateVars},
```

- [ ] **Step 6: 运行 HTTP 测试**

Run: `go test ./internal/httpapi -run 'TestHTTPAutomation' -v`

Expected: PASS。

- [ ] **Step 7: Commit**

```bash
git add internal/httpapi/project_automations.go internal/httpapi/project_automation_template_vars.go internal/httpapi/project_automation_template_vars_test.go internal/httpapi/huma_routes.go
git commit -m "feat: 暴露自动化模板变量 API 和 system_prompt 字段"
```

---

## Task 4: 前端 API 层和类型

**目标：** 前端类型新增 `system_prompt`，新增 `getAutomationTemplateVars` API client。

**Files:**
- Modify: `web/src/features/workspace/project-workbench/automations/project-automations-api.ts`
- Modify: `web/src/features/workspace/project-workbench/automations/automation-rule-form.tsx`

- [ ] **Step 1: 类型新增 system_prompt**

Modify `project-automations-api.ts`，在 `ProjectAutomationRule` 的 `instruction_template` 字段后加：

```ts
  instruction_template: string
  system_prompt: string
```

在文件末尾新增：

```ts
export type AutomationTemplateVar = {
  name: string
  description: string
}

export type AutomationTemplateVarsView = {
  triggers: Array<{
    trigger: string
    vars: AutomationTemplateVar[]
  }>
}

export async function getAutomationTemplateVars(projectSlug: string) {
  return workspaceApiGet<AutomationTemplateVarsView>(`/api/v1/projects/${projectSlug}/automation-template-vars`)
}
```

- [ ] **Step 2: 默认模板新增 system_prompt**

Modify `automation-rule-form.tsx`，在 `defaultScheduleAutomationInput` 和 `assigneeFeishuTemplateInput` 中新增 `system_prompt` 字段：

```ts
export const defaultAutomationSystemPrompt = "你是项目自动化执行 Agent。你会收到来自璇础的项目上下文，请按用户指令执行。需要调用外部系统时，使用你所在 Agent 平台已配置的工具、skill、MCP 或 CLI。"

export const defaultScheduleAutomationInput: ProjectAutomationRuleInput = {
  name: "每日项目巡检",
  description: "",
  enabled: true,
  trigger_type: "schedule",
  trigger_config: { schedule_type: "daily_at", schedule_value: "09:30", timezone: "Asia/Shanghai" },
  condition: { task_filter: "status:pending or status:waiting", max_tasks: 50 },
  action: {
    protocol: "chat_completions",
    base_url_config_key: "agent.provider.base_url",
    api_key_config_key: "agent.provider.api_key",
    model_config_key: "agent.provider.model",
    temperature: 0.2,
  },
  context: { include: ["workspace", "project", "task_summary", "matched_tasks", "project_config"] },
  instruction_template: "请读取这个项目的任务执行情况，生成项目巡检报告。如果项目配置中包含飞书群信息，请自行处理发送。",
  system_prompt: defaultAutomationSystemPrompt,
}
```

同样在 `assigneeFeishuTemplateInput` 中加 `system_prompt: defaultAutomationSystemPrompt`。

- [ ] **Step 3: 运行 typecheck**

Run: `pnpm --dir web typecheck`

Expected: PASS。

- [ ] **Step 4: Commit**

```bash
git add web/src/features/workspace/project-workbench/automations/project-automations-api.ts web/src/features/workspace/project-workbench/automations/automation-rule-form.tsx
git commit -m "feat(web): 新增 system_prompt 类型和模板变量 API"
```

---

## Task 5: 前端变量选择器组件

**目标：** 新增 `TemplateVariablePicker` 组件：一个「插入变量」按钮，点击打开 Popover（Command），展示当前触发器下可用变量列表，点击追加 `{{变量名}}` 到 Textarea。

**Files:**
- Create: `web/src/features/workspace/project-workbench/automations/template-variable-picker.tsx`
- Create: `web/src/features/workspace/project-workbench/automations/template-variable-picker.test.tsx`

- [ ] **Step 1: 写失败测试**

Create `template-variable-picker.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import { TemplateVariablePicker } from "./template-variable-picker"

describe("TemplateVariablePicker", () => {
  beforeEach(async () => {
    await i18n.changeLanguage("zh-CN")
  })

  it("opens popover and inserts variable on click", async () => {
    const onInsert = vi.fn()
    render(
      <TemplateVariablePicker
        trigger="schedule"
        onInsert={onInsert}
        disabled={false}
      />
    )
    await userEvent.click(screen.getByRole("button", { name: "插入变量" }))
    // Popover 打开后展示 project.slug
    const item = await screen.findByText("project.slug")
    await userEvent.click(item)
    expect(onInsert).toHaveBeenCalledWith("{{project.slug}}")
  })

  it("filters variables by trigger type", async () => {
    const onInsert = vi.fn()
    render(
      <TemplateVariablePicker
        trigger="event"
        onInsert={onInsert}
        disabled={false}
      />
    )
    await userEvent.click(screen.getByRole("button", { name: "插入变量" }))
    // event 触发器应展示 event.type
    expect(await screen.findByText("event.type")).toBeTruthy()
    // event 触发器不应展示 tasks
    expect(screen.queryByText("tasks")).toBeNull()
  })
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm --dir web test -- template-variable-picker.test.tsx --run`

Expected: FAIL，组件未定义。

- [ ] **Step 3: 实现组件**

Create `template-variable-picker.tsx`:

```tsx
import { useState } from "react"
import { VariableIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "@/components/ui/command"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"

import { useAutomationTemplateVars } from "./project-automations-api"

type Props = {
  trigger: "schedule" | "event"
  onInsert: (token: string) => void
  disabled?: boolean
}

// TemplateVariablePicker 是「插入变量」按钮 + Popover 变量选择器。
// 点击按钮打开 Popover，展示当前触发器下可用变量，点击追加 {{变量名}} 到 Textarea。
export function TemplateVariablePicker({ trigger, onInsert, disabled }: Props) {
  const [open, setOpen] = useState(false)
  const { data } = useAutomationTemplateVars()

  // 找到当前触发器对应的变量列表。
  const triggerGroup = data?.triggers?.find((t) => t.trigger === trigger)
  const vars = triggerGroup?.vars ?? []

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button type="button" variant="outline" size="sm" disabled={disabled}>
          <VariableIcon className="mr-1 h-3 w-3" />
          插入变量
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-72 p-0" align="start">
        <Command>
          <CommandInput placeholder="搜索变量..." />
          <CommandList>
            <CommandEmpty>无匹配变量</CommandEmpty>
            <CommandGroup>
              {vars.map((v) => (
                <CommandItem
                  key={v.name}
                  value={v.name}
                  onSelect={() => {
                    onInsert(`{{${v.name}}}`)
                    setOpen(false)
                  }}
                >
                  <div className="flex flex-col">
                    <span className="font-mono text-xs">{`{{${v.name}}}`}</span>
                    <span className="text-xs text-muted-foreground">{v.description}</span>
                  </div>
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
```

- [ ] **Step 4: API 层新增 useAutomationTemplateVars hook**

Modify `project-automations-api.ts`，在 `getAutomationTemplateVars` 后新增：

```ts
import { useQuery } from "@tanstack/react-query"
```

（注意：`useQuery` 的 import 需要加到文件顶部已有的 import 块。如果文件顶部没有 tanstack/react-query import，新增一行。）

```ts
export function useAutomationTemplateVars() {
  return useQuery({
    queryKey: ["automation-template-vars"],
    queryFn: () => getAutomationTemplateVars("__vars__"),
    staleTime: Infinity,
  })
}
```

注意：`getAutomationTemplateVars` 需要 `projectSlug` 参数，但变量列表是全局的（不依赖具体项目）。为了让 hook 在 Dialog 内部可用，改为不传 projectSlug，而是请求一个通用路径。修改 `getAutomationTemplateVars` 的路径为不依赖 projectRef 的全局端点。

实际上 route 是 `/api/v1/projects/{projectRef}/automation-template-vars`，需要 projectRef。改为让 hook 接受 projectSlug 参数：

```ts
export function useAutomationTemplateVars(projectSlug: string) {
  return useQuery({
    queryKey: ["automation-template-vars", projectSlug],
    queryFn: () => getAutomationTemplateVars(projectSlug),
    staleTime: Infinity,
  })
}
```

同步更新 `TemplateVariablePicker` 的 Props 和实现，新增 `projectSlug` prop：

```tsx
type Props = {
  projectSlug: string
  trigger: "schedule" | "event"
  onInsert: (token: string) => void
  disabled?: boolean
}

export function TemplateVariablePicker({ projectSlug, trigger, onInsert, disabled }: Props) {
  const { data } = useAutomationTemplateVars(projectSlug)
  // ... 其余不变
}
```

测试中也需要传 `projectSlug="adsops"` 并 mock `useAutomationTemplateVars`。

- [ ] **Step 5: 更新测试 mock**

Modify `template-variable-picker.test.tsx` 的 mock 和 render：

```tsx
vi.mock("./project-automations-api", () => ({
  useAutomationTemplateVars: () => ({
    data: {
      triggers: [
        {
          trigger: "schedule",
          vars: [
            { name: "project.slug", description: "项目 slug" },
            { name: "tasks", description: "匹配任务列表" },
          ],
        },
        {
          trigger: "event",
          vars: [
            { name: "event.type", description: "事件类型" },
            { name: "added_assignees", description: "新增负责人" },
          ],
        },
      ],
    },
  }),
}))

// render 调用改为：
render(
  <TemplateVariablePicker
    projectSlug="adsops"
    trigger="schedule"
    onInsert={onInsert}
    disabled={false}
  />
)
```

- [ ] **Step 6: 运行测试**

Run: `pnpm --dir web test -- template-variable-picker.test.tsx --run`

Expected: PASS。

- [ ] **Step 7: Commit**

```bash
git add web/src/features/workspace/project-workbench/automations/template-variable-picker.tsx web/src/features/workspace/project-workbench/automations/template-variable-picker.test.tsx web/src/features/workspace/project-workbench/automations/project-automations-api.ts
git commit -m "feat(web): 新增模板变量选择器组件"
```

---

## Task 6: 前端规则编辑 Dialog 集成

**目标：** 在 `AutomationRuleDialog` 新增 system prompt Textarea，两个 Textarea 下方各加 `TemplateVariablePicker`。

**Files:**
- Modify: `web/src/features/workspace/project-workbench/automations/automation-rule-dialog.tsx`
- Modify: `web/src/features/workspace/project-workbench/automations/automation-rule-form.tsx`（导出 `defaultAutomationSystemPrompt`）

- [ ] **Step 1: 在 Dialog 中新增 system prompt Textarea 和变量选择器**

Modify `automation-rule-dialog.tsx`:

新增 import:
```tsx
import { TemplateVariablePicker } from "./template-variable-picker"
import { defaultAutomationSystemPrompt } from "./automation-rule-form"
```

在 `ruleToInput` 中新增 `system_prompt: rule.system_prompt`。

在表单中（触发类型/事件/时间字段之后、指令模板之前）新增 system prompt Textarea：

```tsx
            <div className="grid gap-2">
              <label className="text-sm font-medium" htmlFor="automation-system-prompt">系统提示词</label>
              <Textarea
                id="automation-system-prompt"
                value={form.system_prompt}
                onChange={(event) => setForm({ ...form, system_prompt: event.target.value })}
                disabled={disabled}
                rows={3}
                placeholder={defaultAutomationSystemPrompt}
              />
              <TemplateVariablePicker
                projectSlug={projectSlug}
                trigger={form.trigger_type}
                disabled={disabled}
                onInsert={(token) => setForm({ ...form, system_prompt: form.system_prompt + token })}
              />
            </div>
```

在指令模板 Textarea 下方也加 `TemplateVariablePicker`：

```tsx
            <div className="grid gap-2">
              <label className="text-sm font-medium" htmlFor="automation-instruction">指令模板</label>
              <Textarea
                id="automation-instruction"
                value={form.instruction_template}
                onChange={(event) => setForm({ ...form, instruction_template: event.target.value })}
                disabled={disabled}
                rows={6}
              />
              <TemplateVariablePicker
                projectSlug={projectSlug}
                trigger={form.trigger_type}
                disabled={disabled}
                onInsert={(token) => setForm({ ...form, instruction_template: form.instruction_template + token })}
              />
            </div>
```

- [ ] **Step 2: 运行 typecheck**

Run: `pnpm --dir web typecheck`

Expected: PASS。

- [ ] **Step 3: Commit**

```bash
git add web/src/features/workspace/project-workbench/automations/automation-rule-dialog.tsx web/src/features/workspace/project-workbench/automations/automation-rule-form.tsx
git commit -m "feat(web): 规则编辑弹窗新增系统提示词和变量选择器"
```

---

## Task 7: 前端预览弹窗改为展示明文 messages

**目标：** `AutomationPreviewDialog` 除了展示 raw JSON，还展示渲染后的明文 system/user messages，方便用户确认变量替换结果。

**Files:**
- Modify: `web/src/features/workspace/project-workbench/automations/automation-preview-dialog.tsx`

- [ ] **Step 1: 改造预览弹窗**

Modify `automation-preview-dialog.tsx`，在 body JSON 展示区域之前新增明文 messages 展示：

```tsx
// 从 body 中提取 messages 明文展示。
function extractMessages(body: unknown): Array<{ role: string; content: string }> {
  if (body && typeof body === "object") {
    const b = body as Record<string, unknown>
    if (Array.isArray(b.messages)) {
      return b.messages as Array<{ role: string; content: string }>
    }
  }
  return []
}
```

在 `{preview ? (` 块内，在 `<pre>{bodyJSON}</pre>` 之前新增：

```tsx
            <div className="space-y-2">
              {extractMessages(preview.body).map((msg, i) => (
                <div key={i} className="space-y-1">
                  <span className="text-xs font-medium uppercase text-muted-foreground">[{msg.role}]</span>
                  <pre className="max-h-[200px] overflow-auto rounded-md border bg-muted p-3 text-xs whitespace-pre-wrap">
                    {msg.content}
                  </pre>
                </div>
              ))}
            </div>
```

- [ ] **Step 2: 运行 typecheck 和 lint**

Run: `pnpm --dir web typecheck && pnpm --dir web lint`

Expected: PASS。

- [ ] **Step 3: Commit**

```bash
git add web/src/features/workspace/project-workbench/automations/automation-preview-dialog.tsx
git commit -m "feat(web): 预览弹窗展示渲染后的明文 messages"
```

---

## Task 8: 全量验证和文档同步

**目标：** 跑完整后端和前端验证，更新 spec 和 README。

**Files:**
- Modify: `docs/superpowers/specs/2026-07-08-web-console-project-automation-openai-compatible-design.md` §8
- Modify: `README.md`

- [ ] **Step 1: 后端完整验证**

Run:

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
go vet ./...
```

Expected: 全部 PASS。

- [ ] **Step 2: 前端完整验证**

Run:

```bash
pnpm --dir web test
pnpm --dir web typecheck
pnpm --dir web lint
pnpm --dir web build
```

Expected: 全部 PASS。

- [ ] **Step 3: 更新 spec §8**

Modify `docs/superpowers/specs/2026-07-08-web-console-project-automation-openai-compatible-design.md`，在 §8.1 的 system message 说明处加注：

```md
> system prompt 和 instruction template 现在由用户通过表单编辑，支持 `{{变量}}` 占位符引用项目上下文。详见 [模板变量化设计](./2026-07-09-project-automation-template-variables-design.md)。
```

- [ ] **Step 4: Commit**

```bash
git add docs/superpowers/specs/2026-07-08-web-console-project-automation-openai-compatible-design.md
git commit -m "docs: 标注自动化请求体改为用户模板控制"
```

---

## Self-Review Checklist

- [ ] Spec §2.1：system prompt 可编辑 — Task 2 新增字段，Task 6 前端 Textarea。
- [ ] Spec §2.2：instruction template 支持 `{{变量}}` — Task 2 渲染改造。
- [ ] Spec §2.3：Textarea 编辑 — Task 6 前端。
- [ ] Spec §2.4：「插入变量」按钮 — Task 5 组件，Task 6 集成。
- [ ] Spec §2.5：变量按触发器分组 — Task 1 变量定义，Task 5 前端过滤。
- [ ] Spec §2.6：预览展示明文 messages — Task 7。
- [ ] Spec §2.7：向后兼容 — Task 2 空 system_prompt 用默认值，旧 instruction_template 不含变量时行为不变。
- [ ] Spec §4.1：16 个变量全部定义 — Task 1 `automationTemplateVarSpecs`。
- [ ] Spec §5.5：`GET /automation-template-vars` API — Task 3。
- [ ] Spec §6.2：变量选择器不监听输入事件 — Task 5 用按钮触发。
- [ ] Spec §8：前端测试覆盖 — Task 5 组件测试。
