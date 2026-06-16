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

这些键由本系统的 CIO agent 在接群通知时约定使用。首次使用前需 `config_schema_set` 定义。

| 键 | scope | 说明 |
|---|---|---|
| `integrations.feishu.webhook_url` | project | 飞书群机器人地址（sink 用 `config_value` 读） |
| `integrations.feishu.bot_token` | workspace/project（secret） | 飞书机器人 token，模板用 `secret_refs` 引用 |
| `im.group_id` | project | IM 群 ID（CIO agent 自身逻辑用；不在 config_summary 白名单内，用 `config_get` scope=project 读） |

群绑定的完整端到端流程见 wire-up-automation skill 的 references/feishu-bot-setup.md。

## 业务键（workspace 级）

| 键 | scope | 说明 |
|---|---|---|
| `urgency.priority.coeff` | workspace | priority 对 urgency 的贡献系数 |
| `date.*` | workspace | 日期相关配置 |

## 规则

- **workspace 级只支持业务键**（`urgency.*` / `date.*`），不支持 `agent.*`。
- **agent 指令必须 project 级**。
- 写自定义键前**必须 `config_schema_set` 定义** schema（key / value_type / allowed_scopes）。
- `secret: true` 的键值不回显，通过 `secret_refs` 在模板里用 `{{secret.<alias>}}` 引用。
- `project_get` 的 `config_summary` 只暴露一个**固定白名单**（`agent.background`/`agent.constraints`/`agent.default_context`/`agent.handoff` 这 4 个），不是所有 `agent.` 前缀键，更不含其他键。读其他 project 配置（如 `im.group_id`、`integrations.*`）用 `config_get` + `scope:"project"` 或 `project_config_list`。
