# Web Console 项目自动化与 OpenAI 兼容投递设计

> **给 agentic workers 的要求：** 编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

**日期：** 2026-07-08
**状态：** 设计中
**范围：** Web Console 项目详情页新增「自动化」tab；项目级定时触发、事件触发、OpenAI 兼容 Agent Provider 投递、投递 JSON 预览和投递记录。

## 0. 产品结论

项目自动化不是「调用瑶光」功能，也不是两个硬编码开关。璇础在这里承担的是项目事件和项目上下文的编排层：

```text
项目内定时触发 / 项目事件触发
-> 构造稳定的项目上下文和事件上下文
-> 按 OpenAI 兼容接口调用外部 Agent Provider
-> 记录投递请求、响应摘要、失败原因和重试状态
```

外部 Agent Provider 可以是瑶光，也可以是其它实现了 OpenAI-compatible API 的系统。璇础不直接调用飞书，不判断 Agent 是否真的发群消息、拉人或完成外部系统操作；这些业务动作由被调用的 Agent 通过自身 skill、MCP、CLI 或工具完成。

首版在项目详情页新增第 4 个 tab：`概览 / 任务 / 活动 / 自动化`。自动化 tab 管理当前项目的规则，支持两类触发器：

- `schedule`：每天固定时间触发。
- `event`：监听项目内事件，例如 `task.assigned`。

首版动作只支持一种：`openai_compatible`。每日巡检、任务分配后拉群都只是规则模板，不是特殊代码路径。

## 1. 背景

当前项目详情页已经拆成 `概览 / 任务 / 活动` 三个子页面。项目页能展示项目态势、任务执行和事实流，但还缺少“当项目发生某些事情时让 Agent 做事”的项目级编排入口。

已有能力提供了基础：

- 项目、任务、成员、配置值和 external IDs 已经在同一个 workspace 边界内。
- 任务修改已能产生 `task.assigned` 等细粒度事件。
- 项目 config value 可以承载项目绑定信息，例如飞书群、Agent Provider base URL、模型名、凭据引用。
- 出站集成已经有 sink、delivery、HTTP template、重试、审计等工程经验。
- 瑶光等 Agent 平台可以通过 OpenAI 兼容接口暴露 Agent。

但现有出站通知规则主要面向“给人发通知”或“原始 hook 投递”，不适合直接表达项目级“一天一次巡检”或“某事件发生后把项目上下文交给 Agent 执行”。因此本规格引入项目自动化规则，复用已有事件、配置、权限、投递和重试思想，但产品语义独立。

## 2. 目标

1. 在项目详情页新增「自动化」tab，展示、创建、编辑、启停、删除当前项目的自动化规则。
2. 支持定时触发：每天某个固定时间调用 OpenAI 兼容 Agent Provider。
3. 支持事件触发：项目内指定事件发生时调用 OpenAI 兼容 Agent Provider。
4. 规则动作统一为 OpenAI-compatible HTTP 调用，默认使用 `POST /v1/chat/completions`。
5. 支持从项目 config value 读取 provider base URL、API key、model 和其它业务上下文。
6. 支持在规则编辑页点击按钮弹窗预览最终投递 JSON。
7. 预览可以按当前项目真实数据生成，也可以显示模板变量。
8. 支持立即测试，使用与正式投递一致的渲染逻辑。
9. 记录每次投递的状态、目标、请求摘要、响应摘要、错误、HTTP 状态和 provider request id。
10. 保持 workspace/project scope、token scope、role、closed project 规则和 secret redaction 边界。

## 3. 非目标

- 不内置飞书发消息、拉群、群成员管理或其它飞书 API 调用。
- 不把瑶光写成唯一目标系统。
- 不要求外部 Agent Provider 支持非 OpenAI 兼容协议。
- 不做通用工作流引擎、分支、循环、审批或人工确认流。
- 不执行任意脚本、JS、shell 或用户上传代码。
- 不让规则读取 secret config 明文并塞进上下文。首版只允许把 secret 用作 HTTP Authorization 或 header，不作为 Agent 上下文内容展示。
- 不承诺 exactly-once。外部 Agent Provider 应使用上下文里的 `delivery_id` 或请求摘要自行幂等。
- 不在项目 Overview 中展示自动化运行结果；自动化有独立 tab。

## 4. 核心决策

| 主题 | 决策 |
|---|---|
| 项目入口 | 项目详情页新增「自动化」tab：`/workspaces/:workspaceSlug/projects/:projectSlug/automations` |
| 规则作用域 | 首版所有规则都是 project-scoped，不提供 workspace-global 自动化编辑入口 |
| 触发类型 | 首版支持 `schedule.daily_at` 和 `event` |
| 动作类型 | 首版只支持 `openai_compatible` |
| 默认接口 | `POST {base_url}/v1/chat/completions` |
| Responses API | 作为高级选项预留，不作为首版默认 |
| Provider 配置 | base URL、API key、默认 model 优先从 project effective config 读取 |
| 指令 | 每条规则保存一段 instruction template |
| 上下文 | 由规则选择：project、task_summary、matched_tasks、event、project_config |
| JSON 预览 | 编辑页提供「预览投递 JSON」按钮，弹窗生成，不做实时左右联动 |
| 业务模板 | 每日项目巡检、分配任务后拉群只是快速创建模板 |
| 投递记录 | 记录到自动化 delivery/run 表；不复用面向 recipient 的 notification delivery 语义 |
| secret | Authorization 和 secret headers 只显示遮掩值；复制 JSON 不包含 secret 明文 |

## 5. 信息架构

### 5.1 路由结构

```text
/workspaces/:workspaceSlug/projects/:projectSlug
  项目概览。

/workspaces/:workspaceSlug/projects/:projectSlug/tasks
  项目任务。

/workspaces/:workspaceSlug/projects/:projectSlug/activity
  项目活动。

/workspaces/:workspaceSlug/projects/:projectSlug/automations
  项目自动化规则、投递预览和运行记录。
```

### 5.2 项目页 tab

```text
项目：广告投放优化                         [复制链接] [项目状态 v] [设置]
────────────────────────────────────────────────────────────────────
概览   任务   活动   自动化                                      [收起侧栏]
```

`自动化` tab 仍复用项目 Header、项目状态 banner 和右侧项目信息栏。closed project 中允许查看规则和投递记录，但默认禁止新增、编辑、启用、立即测试。

## 6. 自动化 tab 原型

### 6.1 规则列表

```text
自动化
────────────────────────────────────────────────────────────────────
[+ 新建规则] [从模板创建 v]                         [只看启用 ✓]

状态   名称                 触发器                         动作              最近运行
────────────────────────────────────────────────────────────────────
●启用  每日项目巡检         schedule / 每天 09:30           Agent Provider     成功 08:31
●启用  分配任务后拉群       event / task.assigned           Agent Provider     成功 10:12
○停用  高优任务完成后总结   event / task.completed + filter Agent Provider     -

选中一条规则后，在下方或抽屉中编辑详情。
```

列表字段：

| 字段 | 说明 |
|---|---|
| 状态 | 启用 / 停用 |
| 名称 | 用户自定义规则名 |
| 触发器 | `schedule / 每天 HH:mm` 或 `event / event_type` |
| 动作 | 首版固定为 OpenAI 兼容 Agent Provider |
| 最近运行 | 最近一条 delivery 的状态和时间 |

### 6.2 定时规则编辑

```text
自动化规则：每日项目巡检                                      [启用 ✓]
────────────────────────────────────────────────────────────────
名称        [每日项目巡检________________]
说明        [每天生成项目巡检报告________]

触发器
(●) 定时触发
    类型      [每天固定时间 v]
    时间      [09:30]
    时区      [Asia/Shanghai v]
( ) 事件触发

条件
任务筛选    [status:pending or status:waiting]
最大任务数  [50]

动作
类型        调用 OpenAI 兼容接口
接口        [Chat Completions v]
Base URL    [config:agent.provider.base_url]
API Key     [config:agent.provider.api_key]
Model       [config:agent.provider.model]       覆盖 [________________]

指令模板
┌──────────────────────────────────────────────┐
│ 请读取这个项目的任务执行情况，生成项目巡检报告。 │
│ 如果项目配置中包含飞书群信息，请自行处理发送。   │
└──────────────────────────────────────────────┘

上下文
[✓] workspace  [✓] project  [✓] task_summary  [✓] matched_tasks  [✓] project_config

[预览投递 JSON]                         [取消] [保存] [立即测试]
```

### 6.3 事件规则编辑

```text
自动化规则：分配任务后拉群                                  [启用 ✓]
────────────────────────────────────────────────────────────────
名称        [分配任务后拉群________________]

触发器
( ) 定时触发
(●) 事件触发
    事件      [task.assigned v]
    项目范围  当前项目

事件过滤
任务筛选    [priority:H or priority:M________]
只处理新增负责人 [✓]

动作
类型        调用 OpenAI 兼容接口
接口        [Chat Completions v]
Base URL    [config:agent.provider.base_url]
API Key     [config:agent.provider.api_key]
Model       [config:agent.provider.model]

指令模板
┌──────────────────────────────────────────────┐
│ 有任务分配给了新负责人。请根据 added_assignees │
│ 和项目配置，完成后续协作动作。若需要飞书拉群， │
│ 请使用你所在系统可用的 skill、MCP 或工具完成。 │
└──────────────────────────────────────────────┘

上下文
[✓] event  [✓] task  [✓] added_assignees  [✓] project  [✓] project_config

[预览投递 JSON]                         [取消] [保存] [立即测试]
```

### 6.4 模板菜单

```text
从模板创建
────────────────────────────────────
[每日项目巡检]
每天固定时间调用 OpenAI 兼容 Agent，把项目摘要、任务执行情况和项目配置作为上下文传过去。

[分配任务后拉群]
监听 task.assigned，把 task、added_assignees 和项目配置发给 OpenAI 兼容 Agent。

[空白规则]
手动选择触发器、上下文和指令模板。
```

模板只负责预填表单，不产生特殊运行时代码。

## 7. 投递 JSON 预览

### 7.1 入口

规则编辑区只提供一个按钮：

```text
[预览投递 JSON]
```

点击后打开弹窗。弹窗按当前表单状态生成预览，不要求用户先保存规则。

### 7.2 弹窗原型

```text
预览投递 JSON
────────────────────────────────────────────────────────────
预览来源
(●) 按当前项目生成
( ) 模板变量

POST https://agent.example.com/v1/chat/completions
Authorization: Bearer ****
Content-Type: application/json

┌──────────────────────────────────────────────────────────┐
│ {                                                        │
│   "model": "project-operator",                          │
│   "messages": [                                         │
│     {                                                   │
│       "role": "system",                                 │
│       "content": "你是项目自动化执行 Agent..."            │
│     },                                                  │
│     {                                                   │
│       "role": "user",                                   │
│       "content": "请读取这个项目...\n\n<context>{...}"    │
│     }                                                   │
│   ],                                                    │
│   "temperature": 0.2                                    │
│ }                                                       │
└──────────────────────────────────────────────────────────┘

[复制 JSON] [复制 curl]                         [关闭]
```

事件规则的预览弹窗多一个事件来源：

```text
预览来源
(●) 使用模拟事件
( ) 选择最近事件  [task.assigned / adsops-42 v]
```

没有最近事件时只能使用模拟事件。模拟事件必须明显标记为 preview，不写入投递记录。

### 7.3 复制行为

| 按钮 | 行为 |
|---|---|
| 复制 JSON | 只复制 body JSON，不包含 Authorization 明文 |
| 复制 curl | 复制可执行结构，但 token 使用 `${AGENT_PROVIDER_API_KEY}` 或 `****` 占位 |
| 立即测试 | 使用同一套渲染逻辑真实投递，并写入测试 delivery |

## 8. OpenAI 兼容请求格式

### 8.1 Chat Completions 默认格式

首版默认生成严格兼容的最小请求体，避免部分 provider 拒绝未知字段。追踪信息放入 `<context>` 的 `_xuanchu` 节点，不默认写入顶层 `metadata`。

> **更新（2026-07-09）：** system prompt 和 instruction template 现在由用户通过表单编辑，支持 `{{变量}}` 占位符引用项目上下文（如 `{{project.slug}}`、`{{project_config}}`、`{{tasks}}`）。不再硬编码追加 `<context>` JSON。详见 [模板变量化设计](./2026-07-09-project-automation-template-variables-design.md)。

```json
{
  "model": "project-operator",
  "messages": [
    {
      "role": "system",
      "content": "你是项目自动化执行 Agent。你会收到来自璇础的项目上下文，请按用户指令执行。需要调用外部系统时，使用你所在 Agent 平台已配置的工具、skill、MCP 或 CLI。"
    },
    {
      "role": "user",
      "content": "请读取这个项目的任务执行情况，生成项目巡检报告。如果项目配置中包含飞书群信息，请自行处理发送。\n\n<context>{...}</context>"
    }
  ],
  "temperature": 0.2
}
```

### 8.2 context JSON 示例

```json
{
  "_xuanchu": {
    "source": "xuanchu",
    "workspace_id": "ws_123",
    "workspace_slug": "local",
    "project_id": "proj_456",
    "project_slug": "adsops",
    "automation_rule_id": "rule_789",
    "trigger_type": "schedule",
    "delivery_id": "delivery_001"
  },
  "workspace": {
    "id": "ws_123",
    "slug": "local",
    "name": "本地工作区"
  },
  "project": {
    "id": "proj_456",
    "slug": "adsops",
    "name": "广告投放优化",
    "status": "active"
  },
  "task_summary": {
    "pending": 18,
    "waiting": 3,
    "overdue": 4,
    "high_priority": 2,
    "unassigned": 1
  },
  "matched_tasks": [
    {
      "uuid": "task_1",
      "task_slug": "adsops-42",
      "title": "调整预算策略",
      "status": "pending",
      "priority": "H",
      "due": "2026-07-08T23:59:59+08:00",
      "assignees": [
        {
          "id": "user_1",
          "name": "alice",
          "email": "alice@example.com",
          "external_ids": [
            {
              "provider": "feishu_user_id",
              "external_id": "ou_alice"
            }
          ]
        }
      ]
    }
  ],
  "project_config": {
    "feishu.chat_id": "oc_xxx",
    "agent.provider.model": "project-operator",
    "agent.provider.base_url": "https://agent.example.com"
  }
}
```

### 8.3 事件上下文示例

```json
{
  "_xuanchu": {
    "source": "xuanchu",
    "workspace_id": "ws_123",
    "project_id": "proj_456",
    "automation_rule_id": "rule_789",
    "trigger_type": "event",
    "event_type": "task.assigned",
    "delivery_id": "delivery_002"
  },
  "event": {
    "id": "evt_123",
    "type": "task.assigned",
    "occurred_at": "2026-07-08T10:12:00+08:00",
    "actor": {
      "id": "user_owner",
      "name": "owner",
      "email": null,
      "external_ids": []
    }
  },
  "task": {
    "uuid": "task_1",
    "task_slug": "adsops-42",
    "title": "调整预算策略",
    "status": "pending",
    "priority": "H"
  },
  "added_assignees": [
    {
      "id": "user_1",
      "name": "alice",
      "email": "alice@example.com",
      "external_ids": [
        {
          "provider": "feishu_user_id",
          "external_id": "ou_alice"
        }
      ]
    }
  ],
  "project": {
    "id": "proj_456",
    "slug": "adsops",
    "name": "广告投放优化"
  },
  "project_config": {
    "feishu.chat_id": "oc_xxx",
    "agent.provider.model": "project-operator"
  }
}
```

### 8.4 可选 metadata 字段

部分 provider 支持顶层 `metadata`。首版可以在规则高级设置中提供开关：

```text
[ ] 在请求体中附加 metadata 字段
```

默认关闭。开启后才生成：

```json
{
  "metadata": {
    "source": "xuanchu",
    "workspace_id": "ws_123",
    "project_id": "proj_456",
    "automation_rule_id": "rule_789",
    "delivery_id": "delivery_001"
  }
}
```

## 9. 项目配置约定

通过 config definition 管理以下 key。首版按本表命名，避免不同规则各自发明配置名。

| key | 类型 | secret | scope | 说明 |
|---|---|---:|---|---|
| `agent.provider.base_url` | string | no | workspace/project | OpenAI 兼容接口 base URL，例如 `https://agent.example.com` |
| `agent.provider.api_key` | string | yes | workspace/project | HTTP Authorization Bearer token |
| `agent.provider.model` | string | no | workspace/project | 默认 model / agent id |
| `agent.provider.protocol` | enum | no | workspace/project | `chat_completions`，未来可加 `responses` |
| `agent.provider.allowed_hosts` | json | no | workspace/project | 允许投递的 hostname 列表，例如 `["agent.example.com"]` |
| `feishu.chat_id` | string | no | project | 业务上下文，由 Agent 自行解释 |

规则保存时不复制 secret 明文，只保存 config key 引用。预览和 delivery 详情中，secret 值必须显示为 `****` 或“已设置”。

## 10. 数据模型

### 10.1 `project_automation_rules`

推荐新增独立规则表，避免把项目级 Agent 编排塞进面向 recipient 的 notification rule。

| 字段 | 说明 |
|---|---|
| `id` | UUID |
| `workspace_id` | workspace 边界 |
| `project_id` | project 边界 |
| `name` | 规则名 |
| `description` | 可选说明 |
| `enabled` | 是否启用 |
| `trigger_type` | `schedule` / `event` |
| `trigger_config_json` | 定时或事件配置 |
| `condition_json` | 查询条件、最大任务数、事件过滤 |
| `action_type` | 首版固定 `openai_compatible` |
| `action_config_json` | provider config refs、model override、protocol、temperature |
| `context_config_json` | 上下文包含项和限制 |
| `instruction_template` | 用户指令模板 |
| `created_by` / actor columns | 创建人 |
| `created_at` / `modified_at` | 时间 |

### 10.2 `project_automation_deliveries`

| 字段 | 说明 |
|---|---|
| `id` | UUID |
| `workspace_id` | workspace 边界 |
| `project_id` | project 边界 |
| `rule_id` | 自动化规则 |
| `trigger_type` | `schedule` / `event` / `manual_test` |
| `event_id` | 事件触发时记录 |
| `event_type` | 事件触发时记录 |
| `dedupe_key` | 防重复投递 |
| `status` | `queued` / `delivering` / `retry_wait` / `succeeded` / `dead_lettered` |
| `resolved_url` | 目标 URL。首版 URL 本身不含 secret；若未来支持 URL secret template，必须先脱敏再展示 |
| `request_body_json` | 冻结后的完整请求体；不包含 API key 或 secret config 明文，仅供 dispatcher/replay 使用 |
| `request_body_preview` | 截断后的请求体预览，secret 已遮掩 |
| `request_body_hash` | 请求体 hash |
| `response_status_code` | HTTP status |
| `response_body_preview` | 截断后的响应体预览 |
| `provider_request_id` | 从响应中提取的 run id / request id，提取不到为空 |
| `usage_json` | prompt/completion token 等可选 usage |
| `attempt_count` | 尝试次数 |
| `next_attempt_at` | 下次重试时间 |
| `last_error` | 错误摘要 |
| `created_at` / `modified_at` | 时间 |

投递记录不保存 API key 明文，不保存未遮掩 secret header。首版保存完整 `request_body_json`，因为正式投递和 replay 必须使用与入队时一致的事件/项目上下文；该 body 不允许包含 secret config 明文。dispatcher 使用冻结的 `resolved_url` 和 `request_body_json`，只在发送前重新读取当前 secret config 生成 Authorization。

## 11. 后端 API

首版采用以下 HTTP API 路径。

```text
GET    /api/v1/projects/{projectRef}/automations
POST   /api/v1/projects/{projectRef}/automations
GET    /api/v1/projects/{projectRef}/automations/{ruleID}
PATCH  /api/v1/projects/{projectRef}/automations/{ruleID}
DELETE /api/v1/projects/{projectRef}/automations/{ruleID}
POST   /api/v1/projects/{projectRef}/automations/{ruleID}/enable
POST   /api/v1/projects/{projectRef}/automations/{ruleID}/disable

POST   /api/v1/projects/{projectRef}/automations/preview
POST   /api/v1/projects/{projectRef}/automations/{ruleID}/preview
POST   /api/v1/projects/{projectRef}/automations/{ruleID}/test

GET    /api/v1/projects/{projectRef}/automation-deliveries
GET    /api/v1/projects/{projectRef}/automation-deliveries/{deliveryID}
POST   /api/v1/projects/{projectRef}/automation-deliveries/{deliveryID}/replay
```

`/preview` 必须支持未保存表单输入。它返回：

```json
{
  "method": "POST",
  "url": "https://agent.example.com/v1/chat/completions",
  "headers": {
    "Authorization": "Bearer ****",
    "Content-Type": "application/json"
  },
  "body": {
    "model": "project-operator",
    "messages": []
  },
  "warnings": []
}
```

## 12. 触发与投递语义

### 12.1 定时触发

`schedule.daily_at` 每天按规则时区计算一次。调度器需要保证同一规则同一天同一计划时间只生成一条 delivery。

```text
dedupe_key = workspace_id + project_id + rule_id + local_date + schedule_value
```

定时规则的 `matched_tasks` 来自规则里的任务筛选和最大任务数。任务日期边界沿用璇础已有规则：date-only `due/until` 按本地日末，`wait/scheduled` 按本地日初。

### 12.2 事件触发

事件规则监听项目内事件。首版允许选择已有 hook event 白名单中的任务和项目事件，重点包括：

- `task.assigned`
- `task.completed`
- `task.modified`
- `task.unblocked`
- `project.annotated`
- `project.transitioned`

`task.assigned` 的上下文必须包含 `added_assignees`，不能只给当前 assignees。若规则开启“只处理新增负责人”，则没有新增负责人时不投递。

```text
dedupe_key = workspace_id + project_id + rule_id + event_id
```

### 12.3 立即测试

立即测试使用当前保存的规则，触发类型记为 `manual_test`。测试需要写 delivery，便于排障。测试不改变规则的下一次定时运行时间。

## 13. 权限与安全

| 操作 | 权限 |
|---|---|
| 查看规则 | `project:read` + `hook:read` |
| 编辑规则 | `project:write` + `hook:write` |
| 查看投递详情 | `project:read` + `hook:read` |
| 预览 JSON | 与编辑规则相同；只读身份不应预览包含潜在敏感上下文的完整请求 |
| 立即测试 / replay | 与编辑规则相同 |

安全规则：

- API key 只从 secret config value 读取，不回显。
- `project_config` 默认不包含 secret config value。
- `include_secret_config` 首版不提供。
- URL 必须通过 SSRF 防护，与现有 sink allowed host / resolver 策略保持一致。
- `agent.provider.allowed_hosts` 是可选的 SSRF 白名单。配置后 Agent Provider 的 hostname 必须命中该 allowlist（preview、立即测试、正式投递和 replay 走同一校验）；未配置时不限制目标 host，方便本地和内部部署快速启用。
- preview 不能绕过已配置的 project allowlist。
- 429/5xx 可以重试，但必须有最大尝试次数；超过后进入 `dead_lettered`，只允许手动 replay。
- closed project 禁止新增、编辑、启用、测试和 replay；允许查看历史。
- 所有对外 JSON 中的用户身份继续使用 `task.UserInfo` 统一结构。

## 14. 运行记录原型

```text
运行记录：每日项目巡检
────────────────────────────────────────────────────────────
时间                  触发来源             状态       结果
2026-07-08 09:30      schedule             成功       HTTP 200 / run_id=yg_run_123
2026-07-07 09:30      schedule             失败       HTTP 502 / upstream_unavailable

选中记录
────────────────────────────────────────────────────────────
投递目标
  POST https://agent.example.com/v1/chat/completions
  model: project-operator

请求摘要
  payload_hash: sha256:...
  context: project + task_summary + matched_tasks + project_config
  secret config values: 未包含

响应摘要
  status: succeeded
  http_status: 200
  provider_request_id: yg_run_123
  usage: prompt_tokens=3200 completion_tokens=860

[查看投递 JSON] [复制响应摘要] [重新投递]
```

`查看投递 JSON` 展示的是 delivery 当时冻结的脱敏 preview，不重新按当前项目数据生成。

## 15. 错误处理

| 错误 | UI 展示 |
|---|---|
| provider base URL 缺失 | `缺少 config:agent.provider.base_url` |
| API key 缺失 | `缺少 config:agent.provider.api_key` |
| model 缺失 | `缺少 model` |
| URL 被 SSRF 策略拒绝 | `目标地址不允许访问` |
| HTTP 401/403 | `凭据被拒绝`，保留 status code |
| HTTP 429/5xx | 未超过最大尝试次数时标记 `retry_wait` 并按规则重试；超过后标记 `dead_lettered` |
| body 超限 | `投递 JSON 超过大小限制，请减少任务数量或上下文字段` |
| provider 响应非 JSON | 仍记录 HTTP 状态和响应预览，不强制失败解析 |

## 16. 测试要求

后端：

- project automation rule CRUD、enable/disable/delete。
- project scope 隔离和 token project allowlist。
- closed project 写操作拒绝。
- preview 对未保存规则生效。
- preview 不回显 secret。
- schedule daily_at dedupe。
- event `task.assigned` 只包含新增负责人。
- OpenAI-compatible request body 渲染。
- delivery 成功、`retry_wait`、`dead_lettered`、replay。
- SQLite/PostgreSQL schema 和 repository 契约。
- `CGO_ENABLED=0 go test ./...` 和 `CGO_ENABLED=0 go build ./cmd/xuanchu`。

前端：

- 项目 tab 出现「自动化」并路由正确。
- 规则列表展示状态、触发器、动作、最近运行。
- 新建/编辑定时规则。
- 新建/编辑事件规则。
- 「预览投递 JSON」按钮打开弹窗。
- 弹窗展示遮掩 Authorization、body JSON、复制 JSON、复制 curl。
- closed project 禁用写按钮。
- 权限不足时隐藏或禁用写入口。
- `pnpm --dir web test`、`typecheck`、`lint`、`build`。

## 17. 文档同步

实现完成后需要同步：

- `README.md`：项目页新增自动化 tab、OpenAI 兼容 Agent Provider 投递、配置 key 示例。
- `ROADMAP.md`：记录 Web Console 项目自动化能力进入对应 milestone。
- 如新增 config definition 示例，补充到相关手册或 release note。
