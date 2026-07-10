# 项目自动化提示词模板变量化设计

> **给 agentic workers 的要求：** 编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

**日期：** 2026-07-09
**状态：** 设计中
**范围：** 项目自动化规则的 system prompt 和 instruction template 改为用户完全可控的模板，支持 `{{变量}}` 占位符引用项目上下文；前端用 Markdown 编辑器编辑提示词，并在 `{{` 后弹出变量选择器。

## 0. 产品结论

当前自动化投递的请求体有一半是系统拼装的黑盒：system prompt 写死、user message 被强制追加一大段 context JSON。用户在表单里写的「指令模板」只是最终 user message 的一个前缀，无法控制上下文怎么用、放在哪里。

本次改造把 system prompt 和 user message 的构造权完全交给用户。上下文数据通过 `{{变量}}` 占位符暴露，用户在 Markdown 编辑器里自由编排消息结构。编辑器在输入 `{{` 后自动弹出变量选择器，点一下即可插入，不需要手记变量名。

## 1. 背景

### 1.1 当前问题

`renderProjectAutomationRequest` 硬编码了三样东西：

1. **system prompt 写死**（`project_automation_preview.go:114-115`）：
   ```
   "你是项目自动化执行 Agent。你会收到来自璇础的项目上下文..."
   ```
   用户完全不能修改。

2. **context 被拼在 user message 尾部**（`project_automation_preview.go:119`）：
   ```go
   input.InstructionTemplate + "\n\n<context>" + string(ctxJSON) + "</context>"
   ```
   用户写的指令模板后面被强制追加一大段 JSON，无法控制。

3. **没有变量引用**：instruction template 是纯文本，无法引用 `{{project.slug}}` 等数据。

### 1.2 已有基础设施

- **Tiptap Markdown 编辑器**（`web/src/components/markdown/markdown-editor.tsx`）：成熟的 WYSIWYG 编辑器，序列化为 Markdown 字符串。目前用于任务描述和注解，未用于自动化。未安装 `@tiptap/extension-mention`。
- **模板变量声明模式**（`internal/app/notification_template_vars.go`）：notification/hook 已有声明式变量定义 schema，通过 `templateVarSpec` 统一注册，自动派生 API view 和字段校验。
- **Popover + Command 组合**（`web/src/components/ui/popover.tsx` + `command.tsx`）：shadcn 组件，`UserPicker` 是已有 combobox 参考。
- **SinkBodyPreview**（`template-var-hints.tsx`）：已有 `{{var}}` token 高亮渲染（只读展示，无插入能力）。

## 2. 目标

1. system prompt 可由用户在表单中编辑，有合理默认值，可为空。
2. instruction template 支持插入 `{{变量}}` 占位符，渲染时替换为真实数据。
3. 编辑器用现有 Tiptap Markdown WYSIWYG 组件。
4. 编辑器在用户输入 `{{` 后自动弹出变量选择器，可搜索、可点击插入。
5. 变量列表区分触发器类型（schedule / event），只展示当前触发器下有值的变量。
6. 预览弹窗展示渲染后的最终 system / user message 明文，让用户确认变量替换结果。
7. 向后兼容：旧规则没有 system_prompt 字段时使用默认值；旧 instruction_template 仍按纯文本渲染（不含变量时行为不变）。

## 3. 非目标

- 不引入 `@tiptap/extension-mention` 或 Tiptap 原生 suggestion 插件（复杂度高、与 Markdown 序列化冲突）。用轻量的「源码模式 textarea + `{{` 触发 Popover」方案。
- 不做变量嵌套（`{{project.config.feishu.chat_id}}`）。项目配置统一用 `{{project_config}}` 输出 JSON 对象。
- 不做条件逻辑、循环、管道等模板引擎特性（不是 Jinja2 / Go template 的完整实现）。
- 不改 notification/hook 的模板变量体系（它们是独立域）。
- 不改 OpenAI 兼容请求的 `model`/`temperature`/`metadata` 字段。

## 4. 模板变量设计

### 4.1 变量列表

变量按触发器分组，只有当前触发器下有值的变量才展示：

| 变量 | 说明 | schedule | event |
|---|---|:---:|:---:|
| `{{project.id}}` | 项目 UUID | ✓ | ✓ |
| `{{project.slug}}` | 项目 slug | ✓ | ✓ |
| `{{project.name}}` | 项目名称 | ✓ | ✓ |
| `{{project.status}}` | 项目状态 | ✓ | ✓ |
| `{{workspace.id}}` | workspace UUID | ✓ | ✓ |
| `{{workspace.slug}}` | workspace slug | ✓ | ✓ |
| `{{workspace.name}}` | workspace 名称 | ✓ | ✓ |
| `{{project_config}}` | 项目非 secret 配置 JSON 对象 | ✓ | ✓ |
| `{{tasks}}` | 匹配任务列表 JSON 数组 | ✓ | — |
| `{{task_summary}}` | 任务统计摘要 JSON | ✓ | — |
| `{{delivery_id}}` | 本次投递 ID | ✓ | ✓ |
| `{{trigger_type}}` | 触发类型 schedule/event/manual_test | ✓ | ✓ |
| `{{event.type}}` | 事件类型（如 task.assigned） | — | ✓ |
| `{{event.id}}` | 事件 ID | — | ✓ |
| `{{task}}` | 触发事件的任务 JSON | — | ✓ |
| `{{added_assignees}}` | 新增负责人 JSON 数组（task.assigned） | — | ✓ |

### 4.2 变量值格式

- 标量变量（`project.id`、`project.slug` 等）：直接替换为字符串值。
- JSON 变量（`project_config`、`tasks`、`task`、`added_assignees`、`task_summary`）：替换为格式化 JSON 字符串（`json.MarshalIndent`，2 空格缩进）。如果值为空（无匹配任务、无新增负责人），替换为空字符串。
- 未定义变量（如 schedule 规则里写 `{{event.type}}`）：替换为空字符串，不报错。

### 4.3 渲染规则

- 渲染引擎为简单正则替换：匹配 `\{\{([^}]+)\}\}`，按变量名查表替换。
- 变量名做 `strings.TrimSpace` 后精确匹配，不支持表达式。
- 渲染在投递入队时执行（渲染结果冻结到 `request_body_json`），不在 dispatcher 发送时重新渲染。这与现有「delivery 入队时冻结 body」一致。
- 预览也走同一渲染逻辑。

## 5. 后端改动

### 5.1 新增字段

`ProjectAutomationRuleAddInput` 和 `ProjectAutomationRuleModifyInput` 新增：

```go
SystemPrompt string  // 可为空，空时使用默认值
```

`ProjectAutomationRuleView` 新增：

```go
SystemPrompt string `json:"system_prompt"`
```

storage model `ProjectAutomationRule` 新增列：

```go
SystemPrompt string `gorm:"not null;default:''"`
```

### 5.2 默认 system prompt

```text
你是项目自动化执行 Agent。你会收到来自璇础的项目上下文，请按用户指令执行。需要调用外部系统时，使用你所在 Agent 平台已配置的工具、skill、MCP 或 CLI。
```

保存时如果 `SystemPrompt` 为空，存空字符串。渲染时如果 `SystemPrompt` 为空，使用上述默认值。

### 5.3 渲染改造

`renderProjectAutomationRequest` 改为：

1. 构建变量 map（`map[string]string`），键为变量名（不含 `{{}}`），值为渲染后的字符串。
2. 用变量 map 渲染 `SystemPrompt`（空则用默认值）。
3. 用变量 map 渲染 `InstructionTemplate`。
4. 组装 messages：
   ```json
   {
     "messages": [
       {"role": "system", "content": "<渲染后的 system prompt>"},
       {"role": "user", "content": "<渲染后的 instruction template>"}
     ]
   }
   ```
5. 不再追加 `<context>` JSON。

### 5.4 变量定义

新增 `internal/app/project_automation_template_vars.go`，声明式定义可用变量：

```go
type automationTemplateVarSpec struct {
    Name        string
    Description string
    Triggers    []string // "schedule", "event"
}

var automationTemplateVarSpecs = []automationTemplateVarSpec{
    {Name: "project.id", Description: "项目 UUID", Triggers: []string{"schedule", "event"}},
    // ...
}
```

并提供 `AutomationTemplateVarsView()` 返回按触发器分组的变量列表，供前端变量选择器使用。

### 5.5 新增 API

```text
GET /api/v1/projects/{projectRef}/automation-template-vars
```

返回按触发器分组的变量列表。权限要求 `project:read` + `hook:read`（与自动化读一致）。

```json
{
  "triggers": [
    {
      "trigger": "schedule",
      "vars": [
        {"name": "project.id", "description": "项目 UUID"},
        {"name": "project.slug", "description": "项目 slug"}
      ]
    },
    {
      "trigger": "event",
      "vars": [...]
    }
  ]
}
```

## 6. 前端改动

### 6.1 编辑器替换

规则编辑 Dialog（`automation-rule-dialog.tsx`）中：

- **System Prompt**：新增字段，用 `MarkdownEditor` 编辑（`minHeight: 120`），有 label「系统提示词」。
- **Instruction Template**：从 `Textarea` 改为 `MarkdownEditor`（`minHeight: 200`）。

### 6.2 变量选择器

不安装 `@tiptap/extension-mention`。采用**源码模式 + `{{` 触发 Popover**方案：

1. `MarkdownEditor` 已有 WYSIWYG / 源码切换。变量插入在**源码模式**（纯 textarea）下工作。
2. 新增 `TemplateVariablePicker` 组件：监听 textarea 的 `input` 事件，当光标前最近两个字符是 `{{` 时，弹出 Popover（用 shadcn `Popover` + `Command`）展示当前触发器下可用变量列表。
3. 用户点击变量或按 Enter，在光标位置插入变量名 + `}}`，Popover 关闭。
4. 变量列表按触发器过滤，支持搜索（Command 的 built-in filter）。
5. 在编辑器下方常驻一个折叠的「可用变量」提示区（用 `TemplateVarHints` 风格），展示所有变量名和说明，点击可复制变量名到剪贴板。这是 fallback：如果用户在 WYSIWYG 模式下编辑，无法触发 Popover，但仍可手动粘贴。

### 6.3 数据流

- Dialog 打开时从 `GET /automation-template-vars` 获取变量列表（react-query，`staleTime: Infinity`）。
- 变量列表传给 `TemplateVariablePicker`，按当前 `trigger_type` 过滤。
- 切换触发器类型时变量列表自动更新。

### 6.4 预览弹窗

`AutomationPreviewDialog` 改为展示渲染后的**明文 messages**（而非 raw body JSON）：

```text
[System]
你是项目自动化执行 Agent...

[User]
请检查项目 广告投放优化（adsops）的任务执行情况。

项目配置：
{"feishu.chat_id":"oc_xxx"}

待检查任务：
[{"uuid":"...","title":"调整预算策略"}]
```

同时保留「复制 JSON」按钮复制完整 request body（给需要 raw payload 的用户）。

## 7. 数据迁移

- `project_automation_rules` 新增 `system_prompt` 列（`ALTER TABLE ... ADD COLUMN IF NOT EXISTS`），默认空字符串。
- SQLite 和 PostgreSQL 的 AutoMigrate 自动处理（GORM tag `default:''`）。
- 旧规则没有 system_prompt → 渲染时使用默认值，行为等价于改造前。
- 旧 instruction_template 不含 `{{}}` → 渲染时原样输出，行为不变。

## 8. 测试要求

后端：
- 模板渲染：标量变量替换、JSON 变量替换、未定义变量替换为空、空 system prompt 使用默认值。
- 向后兼容：旧规则（无 system_prompt）渲染结果等价。
- 变量定义完整性：`AutomationTemplateVarsView()` 返回的变量覆盖所有可注入的变量。
- API：`GET /automation-template-vars` 返回正确结构。

前端：
- Markdown 编辑器渲染 system prompt 和 instruction template。
- 变量选择器在输入 `{{` 后弹出，点击插入变量。
- 切换触发器类型时变量列表更新。
- 预览弹窗展示渲染后的明文 messages。
- `pnpm --dir web test`、`typecheck`、`lint`、`build`。

## 9. 文档同步

- 更新 spec `2026-07-08-web-console-project-automation-openai-compatible-design.md` §8（OpenAI 兼容请求格式），标注 system prompt 和 instruction template 现在由用户模板控制。
- 更新 README 项目自动化配置说明，提及模板变量。
