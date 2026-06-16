# Config Schema 工具

在写入自定义 workspace/project 配置前，应先用 schema 定义 key、类型、允许作用域和是否 secret。通知 sink 的 `config_value` endpoint 和 secret ref 也依赖这里的定义。

## 内置 schema 键（无需自定义）

以下键已预置 schema，无需 `config_schema_set`：

| 键 | scope | 说明 |
|---|---|---|
| `agent.background` | project | 项目级 agent 背景指令 |
| `agent.constraints` | project | agent 约束 |
| `agent.default_context` | project | agent 默认 context |
| `agent.handoff` | project | 交接说明 |
| `context.default` | project | 默认过滤 context |

## config_schema_list — 列出 schema（只读）

```json
{"workspace": "dajee"}
```

## config_schema_get — 查看 schema（只读）

```json
config_schema_get({"workspace": "dajee", "key": "integrations.openclaw.notification_url"})
```

## config_schema_set — 定义 schema

`key`、`value_type`、`allowed_scopes` 必填。

`value_type` 支持 `string`、`number`、`boolean`、`json`。
`allowed_scopes` 可包含 `workspace`、`project`。
需要枚举约束时用 `enum_values`，**不要把 `value_type` 写成 `enum`**。

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

## config_schema_delete — 删除 schema

默认只删除定义；如果已有配置值会失败。确认要连同已有值一起删除时传 `purge: true`。

```json
config_schema_delete({"workspace": "dajee", "key": "integrations.openclaw.notification_url"})
config_schema_delete({"workspace": "dajee", "key": "integrations.openclaw.notification_url", "purge": true})
```
