---
name: manage-access-and-config
description: 管理 API token（创建/轮换/撤销）、查可用权限、调整 urgency 排序权重、给项目配 agent 指令、定义自定义配置项与 schema、查看过滤上下文。用户提到 token、权限、调权重、配 agent 指令、定义配置键、看 context、轮换密钥时使用。
---

# 权限与配置运维

处理项目协作所需的后台运维：给别的系统/agent 发 token、调排序权重、给项目配 agent 指令、定义配置键。

## 何时使用

当你要发 token、查权限、调 urgency 权重、配 agent 指令、定义配置 schema、查看 context 偏好时，用本 skill。

## 核心原则

- **先遵循 `xuanchu-mcp-base` 的工具名解析规则。** 本文中的 `token_create` 等是 canonical tool name，真实运行时可能带 MCP server 前缀。
- **当前高权限接入方通常由后台预先发放 `["*"]` agent_token**，本 skill 不教你给自己建 token。这里讲的 token 操作是给别的系统/agent 发凭证。
- **每次调用都显式传 `workspace`**，不依赖隐式状态。
- 给别的系统发 token：通用 `["*"]` 或最小化专用 scope；**raw_token 只出现一次，必须保存**。
- config 三级 scope：`workspace`（业务键 `urgency.*` / `date.*`，**不支持 `agent.*`**）/ `project` / `local`（仅 stdio 只读）。
- agent 指令走 project 级 `agent.*`；写自定义键前先 `config_schema_set` 定义。
- context 是只读偏好，默认不改 active context（`context_set`/`context_none` 仅用户明确要求时用）。

## 标准工作流

### 场景 A：给新系统发 token

```
1. token_create({"workspace":"dajee","name":"ci-token","scope":["*"],"expires_in_seconds":86400})
2. // raw_token 只返回一次，保存后告诉使用方
```

### 场景 B：给项目配 agent 指令 + 调 urgency 权重

```
1. project_config_set({"workspace":"dajee","project":"apiplat","key":"agent.background","value":"你负责 API 平台的项目协作..."})
2. config_set({"workspace":"dajee","scope":"workspace","key":"urgency.priority.coeff","value":"6.0"})
```

### 场景 C：定义自定义配置键（用于集成）

```
1. config_schema_set({"workspace":"dajee","key":"integrations.feishu.webhook_url","value_type":"string","allowed_scopes":["project"]})
2. // 之后用 project_config_set 写值（详见 wire-up-automation skill 的 references/feishu-bot-setup.md）
```

## 易错点

- **workspace 级 config 只支持业务键**（`urgency.*` / `date.*`），不支持 `agent.*`；agent 指令必须 project 级。
- `config_set` 必须显式传 `scope`；`config_get` 不传 scope 默认读 workspace 级。
- `config_schema_set` 的 `value_type` 支持 `string` / `number` / `boolean` / `json`；枚举约束用 `enum_values`，**不要把 `value_type` 写成 `enum`**。
- 内置 schema 键无需自己定义：`agent.background`、`agent.constraints`、`agent.default_context`、`agent.handoff`、`context.default`（均 project scope）。
- token scope 通配符：`*`（含 impersonate）、`task:*`、`*:read`。
- `config_schema_delete` 默认只删定义；已有配置值会失败，确认连值删传 `purge:true`。

## 参考文档

| 文件 | 何时读 |
|---|---|
| references/token-tools.md | token 操作完整 JSON |
| references/scopes.md | scope 完整清单与通配符 |
| references/config-tools.md | config get/set/unset/list 完整 JSON |
| references/config-schema-tools.md | config schema 操作完整 JSON |
| references/config-keys.md | 配置键语义表 |
