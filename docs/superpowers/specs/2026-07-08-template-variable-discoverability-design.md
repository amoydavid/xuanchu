# Sink 模板变量可发现性设计

- 日期：2026-07-08
- 状态：已完成
- 范围：通知投递系统（sink / reminder rule / event notification rule）的模板变量可发现性

## 1. 背景与动机

通知投递系统的 sink 支持 URL 模板、header 模板、body 模板，模板里可用 `{{var}}` 占位符在投递时被替换为实际值。但当前存在两个问题：

1. **变量硬编码、不可发现**：可用变量散落在 `internal/app/notification_endpoint.go` 的两个函数里——`notificationTemplateValue`（全量取值，约 256-356 行）负责所有变量的取值，`allowedEndpointVariable`（约 219-234 行）负责 URL 模板的子集白名单。用户在 web console 配置 sink 模板时，没有任何界面元素告诉他们能用哪些变量，只能去翻文档（`docs/skills/.../http-template-vars.md`），体验割裂。

2. **变量是上下文相关的，固定清单会误导**：可用变量是「触发来源 × 字段位置」的函数，不是一份固定清单：
   - reminder rule 触发时，payload 有 `task.*`、`reminder.*`，但没有 `event.*`、`actor.*`（事件路径才填这些）。
   - event notification rule 触发时，有 `event.*`、`actor.*`，但没有 `task.*`、`reminder.*`。
   - URL 模板里禁止 `secret.*` 和 `task.*`（安全限制，`validateEndpointTemplateVariables`），body/header 模板则全部允许。

   如果只给用户一份全集清单，用户在 reminder 的 URL 模板里写 `{{task.title}}` 会直接报错，或在 reminder 的 body 里写 `{{event.type}}` 虽不报错但永远是空值——两种都会误导。

## 2. 目标与非目标

### 目标

- 把模板变量从散落的硬编码 if 分支，重构为**单一声明式 schema**（唯一真相源）。
- 取值函数（`notificationTemplateValue`）和字段位置校验（`allowedEndpointVariable`）都从 schema 派生，消除双轨。
- 暴露一个只读 HTTP API，按「触发源 × 字段位置」组织返回可用变量描述。
- 在 web console 的**规则编辑侧**（reminder rule / event notification rule 创建表单），根据当前规则类型显示**精确可用变量**，并在选中 sink 后显示该 sink body 模板的**只读预览**。

### 非目标（明确排除）

- **项目每日聚合投递**：不改动 reminder 的 per-task 调度模型，不新增 `tasks`（复数）变量或时间变量（`{{date}}`/`{{now}}`）。这是后续 milestone 的事。
- **body 模板编辑入口下沉**：sink 的 body/header 模板仍在 sink 表单编辑，本次只在规则侧做只读预览。
- **reminder rule 表单新增 body 模板字段**：reminder rule 当前没有 body 模板输入，本次不新增。
- **sink 表单本身的变量提示**：本次不动 sink 表单，避免出现 sink 侧（全集）和规则侧（精确）两个不一致的提示入口。

## 3. 关键定义

**变量 trigger 归属的定义**：某个变量归属于某 trigger，当且仅当**该触发源投递时，该变量会被填充有效值**。

注意这不是"调用 `notificationTemplateValue` 是否报错"。例如 `{{event.type}}` 在 reminder 触发时调用取值函数不报错（返回空串），但 reminder 调度路径根本不填 Event 字段，所以它是空值、无意义——因此 `event.*` 只归属 event trigger。这个定义保证提示给用户的变量都是有意义的。

**字段位置（field）**：变量出现的模板位置，取值 `endpoint`（URL 模板）或 `body`（body / header 模板）。URL 模板出于安全考虑禁用 `secret.*` 和 `task.*`。

## 4. 变量映射表（schema 的数据来源）

基于 `notificationTemplateValue` 和 `allowedEndpointVariable` 的现状，完整映射如下。`✓` 表示可用，`✗` 表示不可用。

| 变量 | endpoint | body | reminder | event | 说明 |
|---|---|---|---|---|---|
| `workspace.id` | ✓ | ✓ | ✓ | ✓ | 工作区 ID |
| `workspace.slug` | ✓ | ✓ | ✓ | ✓ | 工作区 slug |
| `project.id` | ✓ | ✓ | ✓ | ✓ | 项目 ID（无项目时取值报 `template_unresolved`） |
| `project.slug` | ✓ | ✓ | ✓ | ✓ | 项目 slug |
| `rule.id` | ✓ | ✓ | ✓ | ✓ | 规则 ID |
| `rule.name` | ✓ | ✓ | ✓ | ✓ | 规则名称 |
| `recipient.id` | ✓ | ✓ | ✓ | ✓ | 接收人用户 ID |
| `recipient.external_ids.*` | ✓ | ✓ | ✓ | ✓ | 接收人外部 ID（按 provider 动态，prefix） |
| `delivery.id` | ✓ | ✓ | ✓ | ✓ | 投递 ID |
| `delivery.attempt` | ✓ | ✓ | ✓ | ✓ | 投递尝试序号 |
| `delivery.workspace_id` | ✓ | ✓ | ✓ | ✓ | 投递工作区 ID |
| `delivery.sink_id` | ✓ | ✓ | ✓ | ✓ | 投递 sink ID |
| `object.kind` | ✓ | ✓ | ✓ | ✓ | 对象类型 |
| `object.id` | ✓ | ✓ | ✓ | ✓ | 对象 ID |
| `task.uuid` | ✗ | ✓ | ✓ | ✗ | 任务 UUID |
| `task.task_slug` | ✗ | ✓ | ✓ | ✗ | 任务 slug（如 `proj-12`） |
| `task.title` | ✗ | ✓ | ✓ | ✗ | 任务标题 |
| `task.description` | ✗ | ✓ | ✓ | ✗ | 任务描述 |
| `task.status` | ✗ | ✓ | ✓ | ✗ | 任务状态 |
| `task.due` | ✗ | ✓ | ✓ | ✗ | 任务截止时间（unix，可空） |
| `reminder.sequence` | ✗ | ✓ | ✓ | ✗ | 提醒序号 |
| `reminder.overdue_sequence` | ✗ | ✓ | ✓ | ✗ | 逾期提醒序号 |
| `reminder.window_start` | ✗ | ✓ | ✓ | ✗ | 提醒窗口起始（unix） |
| `reminder.window_end` | ✗ | ✓ | ✓ | ✗ | 提醒窗口结束（unix） |
| `event.id` | ✓ | ✓ | ✗ | ✓ | 事件 ID |
| `event.type` | ✓ | ✓ | ✗ | ✓ | 事件类型 |
| `event.version` | ✗ | ✓ | ✗ | ✓ | 事件版本（仅 body） |
| `event.occurred_at` | ✗ | ✓ | ✗ | ✓ | 事件发生时间（unix，仅 body） |
| `event.object_kind` | ✓ | ✓ | ✗ | ✓ | 事件对象类型 |
| `event.object_id` | ✓ | ✓ | ✗ | ✓ | 事件对象 ID |
| `event.json` | ✗ | ✓ | ✗ | ✓ | 原始事件 JSON（仅 body） |
| `actor.id` | ✓ | ✓ | ✗ | ✓ | 操作者 ID |
| `actor.name` | ✗ | ✓ | ✗ | ✓ | 操作者名称（仅 body） |
| `secret.*` | ✗ | ✓ | ✓ | ✓ | 在 sink secret_refs 声明的密钥（prefix，需声明） |

注：现有 `allowedEndpointVariable` 只允许 `event.id/type/object_kind/object_id` 和 `actor.id` 出现在 URL 模板，`event.version`/`event.occurred_at`/`event.json`/`actor.name` 均不在 endpoint 白名单。本次重构保持这一现状，不扩大 endpoint 可用范围。

## 5. 设计

### 5.1 后端：变量 schema 重构

新建 `internal/app/notification_template_vars.go`，定义声明式变量描述表作为唯一真相源：

```go
type templateField string

const (
    templateFieldEndpoint templateField = "endpoint"
    templateFieldBody     templateField = "body"
)

type templateTrigger string

const (
    templateTriggerReminder templateTrigger = "reminder"
    templateTriggerEvent    templateTrigger = "event"
)

// templateVarSpec 描述一个模板变量。
type templateVarSpec struct {
    Name        string            // 完整变量名，如 "task.title"；prefix 变量用 "recipient.external_ids.*"
    Description string            // 中文说明
    Fields      []templateField   // 出现的字段位置
    Triggers    []templateTrigger // 有有效值的触发源
    IsPrefix    bool              // 是否动态前缀变量
    PrefixGroup string            // prefix 族名，前端分组用（IsPrefix=true 时填），如 "recipient.external_ids"
}
```

把第 4 节映射表的每一行录入为一个 `templateVarSpec` 条目，组成 `notificationTemplateVarSpecs []templateVarSpec`。

**派生现有函数**（行为不变，数据来源改 schema）：

- `notificationTemplateValue(name, input, allowSecrets)`：先在 schema 里按 name（精确或 prefix）找到 spec，再走原有取值逻辑。原有 case-by-case 取值表达式不变，只是改为从 schema 查到 spec 后分支取值，保证行为等价。
- `allowedEndpointVariable(name)`：改为查 schema 判断 `slices.Contains(spec.Fields, templateFieldEndpoint)`。
- `validateEndpointTemplateVariables(tpl)`：`secret.*`/`task.*` 的禁用现在由 schema 的 Fields 自然表达（不含 endpoint），不再硬编码前缀判断；逻辑等价。

**一致性保证**：新增 `notification_template_vars_test.go`，断言：
- schema 里声明的每个非 prefix 变量，`notificationTemplateValue` 都能解析（用构造的 input）。
- schema 声明 endpoint 可用的变量，`allowedEndpointVariable` 返回 true；声明不可用的，返回 false。
- trigger 分组：reminder 组不含 `event.*`/`actor.*`；event 组不含 `task.*`/`reminder.*`。

### 5.2 HTTP API：暴露变量 schema

新增只读端点：

```
GET /api/v1/notification-template-vars
```

无需请求参数，沿用现有 workspace 上下文鉴权（workspace 内只读）。响应：

```json
{
  "triggers": [
    {
      "trigger": "reminder",
      "fields": [
        {
          "field": "endpoint",
          "vars": [
            {"name": "workspace.id", "description": "工作区 ID", "dynamic": false},
            {"name": "recipient.external_ids.*", "description": "接收人外部 ID（按 provider）", "dynamic": true, "prefix_group": "recipient.external_ids"}
          ]
        },
        {
          "field": "body",
          "vars": [
            {"name": "task.title", "description": "任务标题", "dynamic": false},
            {"name": "secret.*", "description": "在 sink secret_refs 声明的密钥", "dynamic": true, "prefix_group": "secret"}
          ]
        }
      ]
    },
    {
      "trigger": "event",
      "fields": [
        {
          "field": "endpoint",
          "vars": [
            {"name": "workspace.id", "description": "工作区 ID", "dynamic": false},
            {"name": "event.type", "description": "事件类型", "dynamic": false},
            {"name": "actor.id", "description": "操作者 ID", "dynamic": false}
          ]
        },
        {
          "field": "body",
          "vars": [
            {"name": "event.json", "description": "原始事件 JSON", "dynamic": false},
            {"name": "actor.name", "description": "操作者名称", "dynamic": false},
            {"name": "secret.*", "description": "在 sink secret_refs 声明的密钥", "dynamic": true, "prefix_group": "secret"}
          ]
        }
      ]
    }
  ]
}
```

实现：从 `notificationTemplateVarSpecs` 按 trigger 分组、按 field 分组后序列化。路由注册在 `internal/httpapi/huma_routes.go`，handler 在 `internal/httpapi/notifications.go`（或新文件）。

### 5.3 前端：规则侧精确提示 + sink body 只读预览

**落点**：两个规则创建表单（`web/src/features/workspace/outbound/rules/reminder-rule-list.tsx` 和 `notification-rule-list.tsx` 的内联 CreateDialog），在 sink 选择器下方新增「可用变量」区域。

**数据获取**：
- 新增 `getNotificationTemplateVars()` client（`outbound-api.ts`），配合 react-query 缓存（queryKey 如 `["outbound","template-vars"]`）。
- sink 的 `body_template` 已在两个表单打开时加载的本地 `sinks` state 里，选中 sink 后 `sinks.find(s => s.id === selectedSinkId)` 直接取，无需额外请求。

**UI 细节**：

- reminder rule 表单 sink 选择器下方：
  - 面板标题「可用变量（定时提醒）」。
  - 按分组列出该 trigger 下 `body` 字段的变量：`task.*`、`reminder.*`、`recipient.*`、`project.*`、`workspace.*`、`delivery.*`、`object.*`、`secret.*`。
  - 每个变量：等宽小字变量名 + 中文说明。
  - 若选中 sink 且其 `type === "http_template"`：显示该 sink `body_template` 的只读预览（等宽框，把模板里出现的 `{{var}}` 高亮成 chip 标签）。`webhook` 类型不显示预览（body 是默认 payload，无自定义模板）。

- notification rule 表单同理，变量列表换成 event trigger 的（`event.*`、`actor.*` 等，**不显示 `task.*`/`reminder.*`**）。

**不做什么**：
- 不做变量编辑（预览只读）。
- 不引入新的编辑器组件（沿用现有 Textarea 风格 + 简单 chip 高亮）。
- 不改 sink 表单本身。
- 不做点击复制（YAGNI，后置）。

**i18n**：新增 `outbound.templateVars.*` 文案到 `web/src/locales/zh-CN.ts` 和 `en-US.ts`。

## 6. 数据流

```
后端 notificationTemplateVarSpecs（唯一真相源）
  ├─ 派生 notificationTemplateValue / allowedEndpointVariable（投递时取值+校验）
  └─ GET /api/v1/notification-template-vars
        └─ 前端 getNotificationTemplateVars()（react-query 缓存）
              └─ 规则表单按当前 trigger + field 渲染精确变量列表
                  └─ 选中 sink → 从本地 sinks state 取 body_template → 只读预览（chip 高亮变量）
```

## 7. 测试策略

**后端**：
- `internal/app/notification_template_vars_test.go`：schema 完整性（每个声明变量可取值）、endpoint 字段一致性（`task.*`/`secret.*` 不含 endpoint）、trigger 分组正确性。
- `internal/httpapi` 测试：`GET /api/v1/notification-template-vars` 返回结构正确。
- 保留 `internal/app/notification_endpoint_test.go` 现有测试全绿（重构不改行为）。

**前端**：
- 变量提示 + 预览组件测试：给定 trigger=reminder + 某 http_template sink，渲染正确变量列表和 body 预览。
- 两个 rule 表单测试：sink 选中后面板显示。

**验证命令**（项目标准，见 AGENTS.md §6）：
```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
cd web && npm test && npm run build
```

## 8. 兼容性与风险

- **行为等价**：重构取值/校验函数时，原有 case 表达式原样保留，只改数据来源（从散落 if 改为 schema 查询），保证现有投递和校验行为零变化。现有 `notification_endpoint_test.go` 是回归保护网。
- **API 新增不破坏**：`GET /notification-template-vars` 是全新只读端点，不影响现有。
- **前端增量**：仅在两个规则表单新增只读区域，不改动现有表单字段和提交流程。
- **主要风险**：schema 表与取值逻辑漂移。通过一致性测试（§5.1）+ 让取值函数从 schema 派生来缓解。

## 9. 后续（不在本次范围）

- 项目每日聚合投递（reminder 改 per-project 聚合，新增 `tasks` 变量）——这是用户原始场景「每天 9:00 按项目汇总推给 yaoguang wake」的真正解法，需独立 spec。
- 时间变量（`{{date}}`/`{{now}}`/`{{today}}`）。
- sink 表单侧的变量提示（若规则侧提示覆盖不足，再考虑）。
- body 模板编辑入口是否下沉到规则侧的重新评估。
