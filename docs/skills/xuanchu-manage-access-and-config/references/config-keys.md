# 配置键语义表

Xuanchu 的配置键按用途分组。写自定义键前必须用 `config_schema_set` 定义 schema。

## 内置键（无需自定义，已预置 schema）

| 键 | scope | 说明 |
|---|---|---|
| `agent.background` | project | 项目级 agent 背景指令 |
| `agent.constraints` | project | agent 约束 |
| `agent.default_context` | project | agent 默认 context |
| `agent.handoff` | project | 交接说明 |
| `context.default` | project | 默认过滤 context |

## 约定键（群绑定/集成用，需 config_schema_set 定义）

这些键用于项目群通知接线。首次使用前需 `config_schema_set` 定义。

| 键 | scope | 说明 |
|---|---|---|
| `integrations.feishu.webhook_url` | project | 飞书群机器人地址（sink 用 `config_value` 读） |
| `integrations.feishu.bot_token` | workspace/project（secret） | 飞书机器人 token，模板用 `secret_refs` 引用 |
| `im.group_id` | project | IM 群 ID（可经 `project_get` 的 config_summary 读） |

群绑定的完整端到端流程见 `docs/manual/notifications.md`（sink / reminder rule / notification rule 配置）与 `docs/manual/hooks.md`（事件订阅）；MCP 侧对应 `notification_sink_*`、`notification_rule_*`、`reminder_rule_*` 工具，详见 `docs/manual/mcp.md`「通知与提醒」一节。

## 业务键（workspace 级）

| 键 | scope | 说明 |
|---|---|---|
| `urgency.priority.coeff` | workspace | priority 对 urgency 的贡献系数 |
| `date.*` | workspace | 日期相关配置 |

## 规则

- **workspace 级只支持业务键**（`urgency.*` / `date.*`），不支持 `agent.*`。
- **agent 指令必须 project 级**。
- 写自定义键前**必须 `config_schema_set` 定义** schema（key / value_type / allowed_scopes）。
- `secret: true` 的键值不回显到 `config_summary`（用 `config_get` scope=project 单独读，或通过 `secret_refs` 在模板里引用）。
- `project_get` 的 `config_summary` 暴露该 project 全部**非 secret** 配置（agent 指令、群绑定等集成键都可见）。secret 键（schema 标 `secret:true`，如 `integrations.feishu.bot_token`）不回显。
