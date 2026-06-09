# 上下文与配置

通过 Xuanchu MCP 管理 context、workspace/project 级配置和配置 schema。

## 重要原则

- **MCP 调用不依赖"上下文"。** 每个调用都应显式传 `workspace`、`project` 参数。
- Context（过滤器上下文）是 CLI 交互功能，Agent 不应使用 `context_set`/`context_none`/`workspace_use` 来管理隐式状态。
- Agent 需要过滤任务时，直接在 `task_query` 的 `query` 参数中指定过滤条件。

## Context（过滤器上下文）

Context 是预定义的查询过滤器，供 CLI 交互使用。Agent 可以**读取** context 了解用户的偏好过滤，但**不应修改** active context。

### context_get — 查看上下文

只读。了解当前激活的 context 是什么。

```json
// 输入
{"workspace": "dajee"}

// 输入：查看指定 context
{"workspace": "dajee", "name": "sprint"}
```

### context_list — 列出所有上下文

只读。

```json
{"workspace": "dajee"}
```

### context_delete — 删除上下文

仅在用户明确要求删除 context 时使用。

```json
{"workspace": "dajee", "name": "release"}
```

### context_set / context_none — 修改 active context

这两个 tool 只影响 stdio MCP 的隐式状态。Agent 默认不要使用；只有用户明确要求“设置当前 context”或“清除当前 context”时才调用。

```json
context_set({"workspace": "dajee", "name": "sprint"})
context_none({"workspace": "dajee"})
```

## Config（配置管理）

配置分三个作用域：`workspace`（默认）、`project`、`local`（仅 stdio）。

### config_get — 读取配置

`config_get` 不传 `scope` 时默认读取 workspace 级别；写入配置的 `config_set` 仍必须显式传 `scope`。

```json
// 输入：workspace 级别
{"workspace": "dajee", "key": "urgency.priority.coeff"}

// 输入：project 级别
{"workspace": "dajee", "scope": "project", "project": "apiplat", "key": "agent.background"}

// 输入：local 级别（仅 stdio）
{"scope": "local", "key": "remote.server"}

// 返回
{
  "data": {"key": "agent.background", "value": "你是一个 API 开发助手", "scope": "project"},
  "rendered": "agent.background=你是一个 API 开发助手"
}
```

### config_set — 写入配置

`scope` 是必填字段，取值通常是 `workspace` 或 `project`。workspace 级别只支持业务配置键（如 `urgency.*`、`date.*`），不支持 `agent.*`。`local` scope 不可写。

```json
// workspace 级别
{
  "workspace": "dajee",
  "scope": "workspace",
  "key": "urgency.priority.coeff",
  "value": "6.0"
}

// project 级别
{
  "workspace": "dajee",
  "scope": "project",
  "project": "apiplat",
  "key": "agent.handoff",
  "value": "交接说明：所有 API 变更需要经过 review"
}
```

### config_list — 列出配置

只读。

```json
{"workspace": "dajee"}
```

### config_unset — 删除配置

仅删除 workspace 级配置。删除 project 级配置请用 `project_config_unset`。

```json
{"workspace": "dajee", "key": "urgency.priority.coeff"}
```

## Config Schema（配置定义）

在写入自定义 workspace/project 配置前，应先用 schema 定义 key、类型、允许作用域和是否 secret。通知 sink 的 `config_value` endpoint 和 secret ref 也依赖这里的定义。

### config_schema_list / config_schema_get

只读。

```json
config_schema_list({"workspace": "dajee"})
config_schema_get({"workspace": "dajee", "key": "integrations.openclaw.notification_url"})
```

### config_schema_set

`key`、`value_type`、`allowed_scopes` 必填。`value_type` 支持 `string`、`number`、`boolean`、`json`；`allowed_scopes` 可包含 `workspace`、`project`。如果需要枚举约束，使用 `enum_values`，不要把 `enum` 写成 `value_type`。

```json
config_schema_set({
  "workspace": "dajee",
  "key": "integrations.openclaw.notification_url",
  "value_type": "string",
  "allowed_scopes": ["project"],
  "label": "OpenClaw 通知地址"
})

config_schema_set({
  "workspace": "dajee",
  "key": "integrations.feishu.bot_token",
  "value_type": "string",
  "allowed_scopes": ["workspace", "project"],
  "secret": true
})

config_schema_set({
  "workspace": "dajee",
  "key": "integrations.openclaw.channel",
  "value_type": "string",
  "allowed_scopes": ["workspace"],
  "enum_values": ["dev", "prod"]
})
```

### config_schema_delete

默认只删除定义；如果已有配置值会失败。确认要连同已有值一起删除时传 `purge: true`。

```json
config_schema_delete({"workspace": "dajee", "key": "integrations.openclaw.notification_url"})
config_schema_delete({"workspace": "dajee", "key": "integrations.openclaw.notification_url", "purge": true})
```

## 项目配置快捷方式

项目配置也可通过专门的 tool 操作（效果等同 `config_get/set` + `scope="project"`）：

```json
// 设置
project_config_set({"workspace": "dajee", "project": "apiplat", "key": "agent.background", "value": "..."})

// 列出
project_config_list({"workspace": "dajee", "project": "apiplat"})

// 删除
project_config_unset({"workspace": "dajee", "project": "apiplat", "key": "agent.handoff"})
```

## 典型 Agent 工作流

**场景：配置项目级 Agent 指令**

```json
// Step 1: 设置配置
project_config_set({
  "workspace": "dajee",
  "project": "apiplat",
  "key": "agent.background",
  "value": "你是一个 API 开发助手，负责 review 所有 API 变更"
})

// Step 2: 确认配置生效
project_get({"workspace": "dajee", "project": "apiplat"})
// config_summary 中应包含 agent.background
```
