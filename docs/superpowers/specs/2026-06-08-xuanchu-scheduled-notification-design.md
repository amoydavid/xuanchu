# Xuanchu 定时通知设计规格

> **给 agentic workers 的要求：** 编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

**目标：** 为 Xuanchu 增加规则驱动的定时通知能力，让任务在到期前、到期后、或后续扩展的时间窗口内，能够稳定通知任务负责人、显式配置的协作者或外部 Agent，并且保持 workspace 隔离、幂等投递、审计可追溯。

**范围策略：** 本规格只设计 Xuanchu 侧的 reminder rule、scheduler、notification outbox、动态 endpoint resolver 和外部通知 sink 契约。Xuanchu 不内置 OpenClaw、飞书、Slack、邮件等具体 adapter，也不让 Agent 自己承担定时扫描职责。OpenClaw 这类 Agent 平台作为首个典型 sink：负责向用户发送消息、理解用户回复，并通过 Xuanchu HTTP/MCP 代表用户继续操作任务。

**现状补充：** 本规格的 `trigger_type/offset/after` 是 M16 第一阶段兼容模型。实际日常规则已由扩展规格升级为 `schedule + task filter`，详见 `docs/superpowers/specs/2026-06-08-xuanchu-scheduled-notification-rule-filter-extension-design.md`。新规则应优先使用 `schedule_type/schedule_value/filter_source`，旧 trigger 字段保留用于兼容简单到期/逾期提醒。

**需求来源：**

- 当前任务模型已经支持 `due`、`status`、`project`、多 assignee、`task_slug`。
- M8 已有 server-side Hook 和 durable delivery queue，但 Hook 事件是“业务写事件后触发”，不能覆盖“到期前 n 小时”这类纯时间触发。
- M9 已明确 assignee 是 task 基础模型，但不包含通知编排。
- M10 已建立 Agent token 与 request-scoped impersonation 边界，适合 OpenClaw 在用户回复后代表用户操作任务。
- M15 已要求 MCP tool 使用下划线命名，并尽量作为 `internal/app` 的薄壳。

---

## 1. 当前基础与问题

Xuanchu 当前已经具备：

- workspace / project / task 的租户和项目隔离。
- 多 assignee，并能在 JSON、HTTP、MCP、Hook payload 中输出。
- `due:` 与 `overdue` 报表。
- HTTP API、Remote CLI、MCP Server 与 Agent token。
- Hook definition、hook delivery、异步投递、重试、dead-letter、manual replay。
- 用户外部 ID 绑定，可表达同一 Xuanchu 用户在外部系统里的身份。

当前缺口：

- 到期前提醒不是业务写事件，现有 Hook 不会自然触发。
- 到期后如果任务没人再修改，`task.modified` 也不会触发，无法靠 Hook 发现。
- 如果让外部 Agent 自己轮询任务，会绕开 Xuanchu 的 workspace scope、幂等记录、审计和重试机制。
- 如果提醒是隐式默认行为，用户很难理解为什么某些 workspace 会发、某些 workspace 不发。

因此，定时通知必须成为 Xuanchu 服务端的显式规则能力，而不是普通 Hook 的副作用。

## 2. 设计目标

首版完成后，应支持：

```bash
# 到期前 4 小时提醒任务负责人
xuanchu reminder rule add due-before-4h \
  --workspace dajee \
  --project agentapi \
  --trigger due_before \
  --offset 4h \
  --audience assignees \
  --sink openclaw-project

# 到期后仍未完成，每 24 小时提醒负责人和显式配置的 PM
xuanchu reminder rule add overdue-daily \
  --workspace dajee \
  --project agentapi \
  --trigger overdue \
  --after 0h \
  --repeat every:24h \
  --audience assignees_and_explicit_users \
  --recipient pm@example.com \
  --sink openclaw-project

# 查看通知投递
xuanchu notification delivery list --workspace dajee --status dead_lettered
xuanchu notification delivery replay <delivery-id>
```

核心目标：

- 到期前 `n` 小时提醒。
- 到期后仍未完成提醒。
- 支持 project 级和 workspace 级规则。
- recipient 从 task assignees、显式用户集合或二者组合解析；project owner / maintainer 类 audience 等项目角色模型明确后再启用。
- 规则显式配置；没有规则就不发送提醒。
- 所有通知先落 durable outbox，再由 dispatcher 投递。
- notification sink 支持动态解析 endpoint，允许按 workspace、project、rule、recipient 或共享配置生成不同 URL。
- notification sink 支持 `webhook` 和 `http_template`：前者投递标准 envelope，后者用数据库里的 method/header/body 模板直接调用第三方 Web API。
- 每个规则、任务、recipient、触发窗口必须幂等，避免重复发送。
- 外部系统失败不回滚任务事务。
- 失败投递可查询、可重试、可进入 dead-letter。
- OpenClaw 可以接收通知并向用户发消息；用户回复后 OpenClaw 通过 HTTP/MCP 调用 Xuanchu。

## 3. 非目标

本规格不做：

- 不内置 OpenClaw、飞书、Slack、邮件、短信 adapter。
- 不把 OpenClaw 变成唯一通知通道。
- 不执行任意 shell、JS 或外部脚本；首版只做受控 HTTP request template。
- 不让 Agent 自己承担到期扫描和幂等判断。
- 不把 reminder rule 混进现有 Hook definition。
- 不允许任务描述、用户自然语言回复、未校验 external ID 直接生成任意 webhook URL。
- 不做 UI 自动化页面。
- 不做复杂工作流引擎、条件分支、审批流。
- 不引入跨 workspace “我的所有逾期任务”聚合视图。
- 不改动 urgency 公式。
- 不因被提醒而自动改变任务状态。
- 不默认创建逾期提醒规则。

## 4. 核心产品决策

### 4.1 定时通知是规则驱动，不是隐式行为

Xuanchu 不因为任务设置了 `due` 就自动发提醒。只有 active reminder rule 命中时才产生 notification。

原因：

- 企业 workspace 对通知噪音非常敏感，默认隐式提醒容易造成误发。
- 不同项目的提醒策略差异很大，必须显式配置。
- 运维排查时可以从 rule -> evaluation -> delivery 追踪完整链路。

### 4.2 Scheduler 属于 Xuanchu 服务端，不属于 Agent

Scheduler 负责周期性扫描数据库，并基于当前时间和规则生成 notification delivery。

OpenClaw 这类 Agent 不应该自己扫描所有任务。Agent 只接收 Xuanchu 发出的标准通知，再负责消息触达和对话处理。

这样做的边界：

- Xuanchu 保持任务事实源和权限边界。
- Xuanchu 负责幂等、重试、dead-letter、审计。
- Agent 负责用户消息体验和自然语言回复处理。

### 4.3 复用 Hook delivery 的运维思想，但不复用 Hook 事件语义

Reminder notification 与 Hook 都是“向外部 URL 投递 envelope”，但触发源不同：

- Hook：任务或项目发生写事件后触发。
- Reminder：时间窗口命中后触发。

因此首版可以复用 Hook delivery 的实现经验，例如 claim、retry、dead-letter、signature、outbound network guard，但不要把 reminder event 塞进 Hook event 列表。

### 4.4 OpenClaw 是 notification sink 和 conversation executor

OpenClaw 与 Xuanchu 的推荐配合：

1. 管理员在 Xuanchu 创建 OpenClaw notification sink。
2. 管理员创建 reminder rule，sink 指向 OpenClaw。
3. Xuanchu scheduler 命中规则后生成 notification delivery。
4. Dispatcher 向 OpenClaw webhook 投递通知。
5. OpenClaw 根据 recipient 的 external ID 给用户发消息。
6. 用户回复“完成”“延期到明天”“需要帮助”等。
7. OpenClaw 使用 Xuanchu HTTP API 或 MCP tool 执行动作。
8. 如果代表某个用户执行，OpenClaw 使用 Agent token + `X-Xuanchu-As`，由 Xuanchu 记录 subject 和 delegator。

### 4.5 Endpoint 必须动态但受控

Notification sink 不应该只支持一个写死 URL。现实集成里常见需求包括：

- 每个 workspace 有自己的 OpenClaw tenant endpoint。
- 每个 project 接入不同 Agent 实例。
- 同一个 rule 根据 recipient 投递到不同用户会话 endpoint。
- 测试环境、预发环境、生产环境 endpoint 不同。

因此 sink 应表达“如何解析 endpoint”，而不是只表达“固定 URL”。但 endpoint 解析必须受控：

- 动态值只能来自 workspace/project shared config、sink/rule 显式字段、recipient `external_ids`、系统生成的 delivery 上下文。
- 不允许从 task description、annotation、用户回复文本中直接拼接 endpoint。
- 解析出的 URL 仍必须通过出站网络防护和 allowlist 校验。
- delivery 落库时必须固化最终 `resolved_url`，后续 retry 使用同一个 URL，避免配置变更导致同一 delivery 重试到不同目标。

## 5. 领域模型

### 5.1 Reminder Rule

逻辑字段：

- `id`
- `workspace_id`
- `project_id` nullable
- `name`
- `enabled`
- `trigger_type`
- `offset_seconds`
- `after_seconds`
- `repeat_policy`
- `audience_type`
- `recipient_user_ids_json`
- `sink_id`
- `template_key`
- `created_by`
- `created_at`
- `modified_at`

`trigger_type` 首版支持：

- `due_before`
- `overdue`

`audience_type` 首版支持：

- `assignees`
- `explicit_users`
- `assignees_and_explicit_users`

`repeat_policy` 首版支持：

- `once`
- `every:<duration>`

规则：

- `project_id` 为空表示 workspace 级规则。
- `project_id` 非空时，该 project 必须属于同一 workspace。
- `due_before` 必须设置 `offset_seconds > 0`。
- `overdue` 可设置 `after_seconds >= 0`。
- `repeat_policy=once` 表示同一任务、同一 recipient、同一 trigger 只通知一次。
- `repeat_policy=every:24h` 表示按固定窗口重复通知，直到任务不再满足条件。

### 5.2 Notification Sink

Notification sink 表达外部投递目标和请求模板，不表达业务触发条件。

逻辑字段：

- `id`
- `workspace_id`
- `name`
- `type`
- `endpoint_mode`
- `url`
- `url_template`
- `config_key`
- `allowed_hosts_json`
- `http_method`
- `header_templates_json`
- `body_template`
- `body_content_type`
- `secret_refs_json`
- `secret`
- `enabled`
- `created_by`
- `created_at`
- `modified_at`

`type` 首版支持：

- `webhook`
- `http_template`

两种类型的区别：

- `webhook`：向外部系统投递 Xuanchu 标准 notification envelope。OpenClaw 作为通知 sink 时通常使用这个模式。
- `http_template`：Xuanchu 根据数据库中保存的 HTTP method、header 模板、body 模板，直接调用第三方固定 Web API。适用于飞书机器人、企业微信机器人、Slack webhook、短信 API、自建通知网关等固定接口。

`endpoint_mode` 首版支持：

- `static_url`
- `template`
- `config_value`

字段语义：

- `static_url`：使用 sink 上的固定 `url`。
- `template`：使用受限模板 `url_template` 解析 endpoint。
- `config_value`：从 workspace/project shared config 的 `config_key` 读取 endpoint；该 key 必须由 sink 显式声明，不能由 rule 或 task 动态传入。
- `allowed_hosts_json`：限制解析后的 host，支持精确 host 或后续实现计划定义的安全通配规则。
- `http_method`：`http_template` 的请求方法，首版只允许 `POST`。`webhook` 固定为 `POST`。
- `header_templates_json`：HTTP header 模板，存数据库；渲染后写入 delivery 的 `rendered_headers_json`。
- `body_template`：HTTP body 模板，存数据库；渲染后写入 delivery 的 `rendered_body`。
- `body_content_type`：body 的 content type，例如 `application/json`。
- `secret_refs_json`：模板可引用的 secret 映射，保存“模板别名 -> shared config key”，不保存渲染后的 secret 到 view 或 audit。目标 shared config schema 必须存在且 `secret=true`。
- `secret`：仅用于 `webhook` envelope 签名的共享密钥；`http_template` 不把任意密钥值直接塞进 sink view，而是通过 `secret_refs_json` 声明引用，再由渲染器从受控 secret/config 存储读取。

OpenClaw 使用 `type=webhook`：

```bash
xuanchu notification sink add openclaw-webhook \
  --workspace dajee \
  --type webhook \
  --url https://openclaw.example.com/xuanchu/notifications \
  --secret-stdin
```

首版不需要把 `openclaw` 做成独立 sink type。OpenClaw 是 webhook consumer，不是 Xuanchu 内置 adapter。

第三方 Web API 使用 `http_template`：

```bash
xuanchu notification sink add feishu-bot \
  --workspace dajee \
  --type http_template \
  --endpoint-mode config_value \
  --config-key integrations.feishu.webhook_url \
  --allowed-host open.feishu.cn \
  --method POST \
  --header-template 'Content-Type=application/json' \
  --header-template 'Authorization=Bearer {{secret.feishu_bot_token}}' \
  --body-content-type application/json \
  --body-template '{"msg_type":"text","content":{"text":"任务 {{task.task_slug}} 即将到期：{{task.description}}"}}' \
  --secret-ref feishu_bot_token=integrations.feishu.bot_token
```

模板存储规则：

- header/body 模板必须保存在 `notification_sinks` 表中，而不是只存在 CLI 参数或本地文件。
- 模板只能使用受控变量和 secret 引用。
- header 模板以结构化 JSON 保存，例如 `[{"name":"Content-Type","value":"application/json"},{"name":"Authorization","value":"Bearer {{secret.feishu_bot_token}}"}]`，不能用换行字符串拼接。
- body 模板以原始字符串保存；如果 `body_content_type=application/json`，保存时必须能通过 JSON 语法校验，但模板变量替换后的最终 body 仍以渲染快照为准。
- `secret_refs_json` 以结构化 JSON 保存，例如 `{"feishu_bot_token":{"config_key":"integrations.feishu.bot_token"}}`。
- secret 值由 Xuanchu 在渲染时从 shared config 读取并注入。读取规则复用 project config 的继承链：有 project 时先读 project 显式值，再回退 workspace 显式值和 schema default；目标 schema 必须 `secret=true`。
- view、audit、delivery view 不得泄露 secret 明文，只能显示 alias、config key、是否已配置、fingerprint 等安全元数据。
- delivery 创建时必须固化渲染后的请求快照，retry/replay 使用快照，不因 sink 模板变化改变请求内容。

动态 endpoint 示例：

```bash
# 从 project config 读取 endpoint。不同 project 可以投到不同 OpenClaw app。
xuanchu notification sink add openclaw-project \
  --workspace dajee \
  --type webhook \
  --endpoint-mode config_value \
  --config-key integrations.openclaw.notification_url \
  --allowed-host openclaw.example.com \
  --secret-stdin

# 使用受限模板。变量只允许来自受控上下文。
xuanchu notification sink add openclaw-tenant \
  --workspace dajee \
  --type webhook \
  --endpoint-mode template \
  --url-template 'https://openclaw.example.com/tenants/{{workspace.slug}}/xuanchu/notifications' \
  --allowed-host openclaw.example.com \
  --secret-stdin
```

### 5.2.1 Endpoint Resolver

Endpoint resolver 在 scheduler 生成 delivery 时执行，输入是：

- sink definition
- reminder rule
- workspace
- project nullable
- task snapshot
- recipient `task.UserInfo`
- shared config reader
- sink 允许的 config key

输出是：

- `resolved_url`
- `resolved_endpoint_source`
- `resolved_endpoint_fingerprint`
- `rendered_method`
- `rendered_headers_json`
- `rendered_body`
- `rendered_content_type`
- `payload_json`

解析顺序：

1. 根据 `endpoint_mode` 选择解析器。
2. 从受控上下文读取动态值。
3. 生成候选 URL。
4. 校验 URL scheme 只允许 `https`，本地开发可以通过显式配置允许 `http`。
5. 校验 host 命中 `allowed_hosts_json`。
6. 执行出站网络防护解析，禁止内网、loopback、link-local 等地址。
7. 生成标准 Xuanchu notification envelope，并写入 delivery 的 `payload_json`。
8. 如果 sink 是 `webhook`，使用 envelope body、webhook 签名 header 和固定 `POST`。
9. 如果 sink 是 `http_template`，渲染数据库中保存的 method/header/body 模板，生成 `rendered_method`、`rendered_headers_json`、`rendered_body`、`rendered_content_type`。
10. 把最终 URL 和请求快照写入 delivery；后续 retry/replay 不再读取当前 sink 模板重新渲染。

受控模板变量首版只允许：

- `workspace.id`
- `workspace.slug`
- `project.id`
- `project.slug`
- `rule.id`
- `rule.name`
- `recipient.id`
- `recipient.external_ids.<provider>`
- `task.uuid`
- `task.task_slug`
- `task.description`
- `task.due`
- `task.status`
- `secret.<name>`，仅限 header/body 模板，且 `<name>` 必须在 `secret_refs_json` 中声明。

变量使用边界：

- endpoint URL 模板只允许 workspace、project、rule、recipient external id 等路由上下文变量，不允许 `task.description` 或 secret。
- header/body 模板可以使用 task、recipient、workspace、project、rule 变量，以及 `secret_refs_json` 显式声明的 secret 引用。
- secret 只能用于 header/body 模板，不能用于 URL。

如果变量缺失，resolver 必须返回 `template_unresolved` 或 `endpoint_unresolved`，该 recipient 的 delivery 不进入投递队列，或进入 `dead_lettered` 并记录原因。实现计划需要在两者中选择一种一致策略。

禁止：

- 任意 Go template / JS 表达式。
- 从 task description / annotation / UDA 读取 URL。
- 从 HTTP query、MCP 参数或 OpenClaw 用户回复覆盖 delivery endpoint。
- redirect 后重新解析到未授权 host。
- 在 view、audit、日志、delivery view 中输出渲染后的 secret。

### 5.3 Notification Delivery

Delivery 是 durable outbox 记录。

逻辑字段：

- `id`
- `workspace_id`
- `project_id`
- `rule_id`
- `sink_id`
- `task_uuid`
- `recipient_user_id`
- `event_type`
- `dedupe_key`
- `resolved_url`
- `resolved_endpoint_source`
- `resolved_endpoint_fingerprint`
- `rendered_method`
- `rendered_headers_json`
- `rendered_body`
- `rendered_content_type`
- `payload_json`
- `headers_json`
- `status`
- `attempt_count`
- `next_attempt_at`
- `last_attempt_at`
- `claim_expires_at`
- `last_status_code`
- `last_error`
- `created_at`
- `modified_at`

`event_type` 首版支持：

- `task.due_soon`
- `task.overdue`

`status` 首版复用 Hook delivery 状态语义：

- `queued`
- `delivering`
- `retry_wait`
- `succeeded`
- `dead_lettered`
- `disabled_skipped`

`dedupe_key` 必须唯一，建议格式：

```text
<workspace_id>:<rule_id>:<task_uuid>:<recipient_user_id>:<event_type>:<window_start>
```

其中 `window_start` 由 trigger 和 repeat policy 决定：

- `due_before + once`：可使用任务 due 时间减 offset 后的窗口。
- `overdue + once`：可使用任务 due 时间加 after 后的窗口。
- `every:24h`：使用按 repeat duration floor 后的窗口。

## 6. Scheduler 语义

### 6.1 扫描范围

Scheduler 周期性执行：

1. 读取 enabled reminder rules。
2. 对每条 rule 解析 workspace / project scope。
3. 查询命中条件的 pending tasks。
4. 解析 recipients。
5. 为每个 recipient 解析 sink endpoint。
6. 为每个 recipient 构造 delivery。
7. 通过 `dedupe_key` 幂等插入。

任务必须满足：

- `status = pending`
- `due IS NOT NULL`
- `workspace_id` 属于 rule workspace
- 如果 rule 指定 project，则 task 必须属于该 project

不通知：

- completed task
- deleted task
- 无 due 的 task
- 无 recipient 的 task
- sink disabled 的 rule
- endpoint 无法解析或解析后不满足安全策略的 recipient

### 6.2 due_before

`due_before` 命中条件：

```text
now >= task.due - offset
now < task.due
task.status = pending
```

如果 scheduler 延迟运行，只要任务仍在 due 前且 dedupe key 未发过，就可以补发。

如果 scheduler 首次看到时任务已经 overdue，`due_before` 不补发，交给 `overdue` 规则处理。

### 6.3 overdue

`overdue` 命中条件：

```text
now >= task.due + after
task.status = pending
```

`repeat_policy=once` 只发送一次。

`repeat_policy=every:24h` 表示每个 24 小时窗口最多发送一次，直到任务 completed / deleted / due 被改到未来。

### 6.4 due 变更后的行为

任务 due 被修改后：

- 新 due 会产生新的时间窗口。
- 旧 due 对应的已发送 delivery 不删除，作为历史记录保留。
- 未投递成功但已经不再满足条件的 delivery，dispatcher 不需要自动删除；但 scheduler 不再生成新的 delivery。

如果任务被完成：

- scheduler 不再生成 reminder delivery。
- 已 queued 的 delivery 在投递前应重新检查 task status；如果任务已完成，标记为 `disabled_skipped` 或新增 `condition_skipped`。首版优先复用 `disabled_skipped`，但 payload / last_error 要能说明原因。

## 7. Recipient 解析

首版 recipient 只从 Xuanchu 用户模型解析，不直接从 OpenClaw 用户 ID 解析。

规则：

- `assignees`：取 task assignees。
- `explicit_users`：使用 rule 中配置的 user IDs。
- `assignees_and_explicit_users`：task assignees + rule 中配置的 user IDs 去重。

首版不支持 `project_owner` 或 `project_maintainer` audience。当前 project 模型只有创建者等审计字段，没有稳定的项目负责人角色模型；实现时不能把 `created_by` 静默当成项目负责人。后续如果引入 project role，再新增 `project_role:<role>` 或类似 audience。

输出 payload 中的用户引用必须使用统一 `task.UserInfo` 结构：

```json
{
  "id": "u_alice",
  "name": "Alice",
  "email": "alice@example.com",
  "external_ids": [
    {
      "provider": "openclaw",
      "external_id": "openclaw_user_123"
    }
  ]
}
```

OpenClaw 根据 `external_ids` 定位用户。如果用户没有 OpenClaw external ID，OpenClaw 可以返回失败，Xuanchu delivery 进入 retry 或 dead-letter。

## 8. Notification Payload

投递给 OpenClaw 或其它 `webhook` sink 的 body 使用稳定 envelope：

```json
{
  "event_id": "ntf_001",
  "event_type": "task.due_soon",
  "event_version": 1,
  "workspace": {
    "id": "ws_dajee",
    "slug": "dajee",
    "name": "Dajee"
  },
  "project": {
    "id": "proj_agentapi",
    "slug": "agentapi",
    "name": "AI Agent Platform"
  },
  "rule": {
    "id": "rule_due_before_4h",
    "name": "due-before-4h",
    "trigger_type": "due_before"
  },
  "task": {
    "uuid": "task_uuid",
    "task_slug": "agentapi-27",
    "description": "完成 OAuth 回调错误处理",
    "status": "pending",
    "due": 1781008800,
    "assignees": []
  },
  "recipient": {
    "id": "u_alice",
    "name": "Alice",
    "email": "alice@example.com",
    "external_ids": []
  },
  "action_context": {
    "preferred_workspace": "dajee",
    "preferred_task_ref": "agentapi-27"
  },
  "occurred_at": 1780994400
}
```

要求：

- `task` 尽量复用 `task.ToJSON()` 输出。
- `recipient` 必须是完整 `task.UserInfo`，不能是裸 UUID。
- `event_version` 首版为 `1`。
- `action_context` 帮助 Agent 后续调用 Xuanchu，不作为权限来源。

`http_template` sink 的 body 不使用上述 envelope 作为最终请求 body，而是使用 `body_template` 渲染后的 `rendered_body`。但 delivery 的 `payload_json` 仍应保存 Xuanchu 标准 envelope，便于审计、排查和后续 replay 解释。

## 9. OpenClaw 集成示例

### 9.1 到期前提醒

任务：

```text
agentapi-27
描述：完成 OAuth 回调错误处理
负责人：Alice
due：2026-06-09 18:00
状态：pending
```

项目配置：

```bash
xuanchu --workspace dajee project config set agentapi \
  integrations.openclaw.notification_url \
  https://openclaw.example.com/apps/agentapi/xuanchu/notifications
```

Sink：

```bash
xuanchu notification sink add openclaw-project \
  --workspace dajee \
  --type webhook \
  --endpoint-mode config_value \
  --config-key integrations.openclaw.notification_url \
  --allowed-host openclaw.example.com \
  --secret-stdin
```

规则：

```bash
xuanchu reminder rule add due-before-4h \
  --workspace dajee \
  --project agentapi \
  --trigger due_before \
  --offset 4h \
  --audience assignees \
  --sink openclaw-project
```

到 `2026-06-09 14:00`，Xuanchu 从 project config 解析出 `resolved_url=https://openclaw.example.com/apps/agentapi/xuanchu/notifications`，校验 host 后落库 delivery，并向 OpenClaw 投递 `task.due_soon`。

OpenClaw 给 Alice 发送：

```text
Alice，任务 agentapi-27 将在 4 小时后到期：

完成 OAuth 回调错误处理

你可以回复：
- 完成
- 延期到明天
- 添加备注：...
```

### 9.2 用户回复“延期到明天”

OpenClaw 收到 Alice 回复后，使用 Agent token 调用 Xuanchu：

```http
Authorization: Bearer xuanchu_agent_xxx
X-Xuanchu-As: alice@example.com
```

MCP tool call：

```json
{
  "tool": "task_modify",
  "arguments": {
    "workspace": "dajee",
    "task": "agentapi-27",
    "due": 1781095199,
    "annotations": [
      "Alice 通过 OpenClaw 将到期时间延期到明天"
    ]
  }
}
```

Xuanchu 必须校验：

- Agent token 是否允许访问 workspace。
- Agent token 是否有 `impersonate` scope。
- Alice 是否是 workspace 成员。
- Alice 是否具备 task write 权限。
- Audit 同时记录 subject Alice 和 delegator OpenClaw Agent。

### 9.3 逾期升级提醒

规则：

```bash
xuanchu reminder rule add overdue-daily \
  --workspace dajee \
  --project agentapi \
  --trigger overdue \
  --after 0h \
  --repeat every:24h \
  --audience assignees_and_explicit_users \
  --recipient pm@example.com \
  --sink openclaw-project
```

任务逾期后，Xuanchu 向 Alice 和显式配置的 PM 分别投递 `task.overdue`。

OpenClaw 可以给 Alice 发送：

```text
任务 agentapi-27 已逾期，还没有完成。

完成 OAuth 回调错误处理

回复“完成”可直接标记完成，回复“需要帮助：...”可通知 PM。
```

给 PM 发送：

```text
任务 agentapi-27 已逾期，负责人 Alice 尚未完成。

你可以回复：
- 催办 Alice
- 指派 Bob 协助
- 查看详情
```

如果 PM 回复“指派 Bob 协助”，OpenClaw 再以 PM 为 subject 调用 `task_modify` 增加 assignee。

## 10. 权限与审计

### 10.1 管理权限

首版建议：

- `reminder rule list/info`：workspace member 可读。
- `reminder rule add/modify/delete/enable/disable`：workspace admin/owner。
- `notification sink list/info`：workspace admin/owner。
- `notification sink add/modify/delete/enable/disable`：workspace admin/owner。
- `notification delivery list/info`：workspace admin/owner；后续可放宽给 project maintainer。
- `notification delivery replay`：workspace admin/owner。

如果现有权限常量不足，实现计划中应补齐，而不是在 HTTP/MCP 层写特殊判断。

### 10.2 Scheduler actor

Scheduler 不是人类用户。它产生 delivery 时，应记录：

- `system_actor = "scheduler"`
- `rule_id`
- `created_by` 原规则创建者

Scheduler 生成 notification delivery 不等同于修改 task，不需要写 task audit action。是否写 notification audit action 由实现计划决定，但至少 delivery row 必须能追踪来源。

OpenClaw 在用户回复后修改任务，必须通过普通 task write 路径，并按 M10 记录 subject / delegator。

## 11. HTTP / MCP / CLI 契约

### 11.1 CLI 命令草案

```bash
xuanchu notification sink add <name> --type webhook --url <url> --secret-stdin
xuanchu notification sink add <name> --type http_template --endpoint-mode config_value --config-key <key> --header-template <k=v> --body-template <json>
xuanchu notification sink list
xuanchu notification sink info <sink>
xuanchu notification sink modify <sink> ...
xuanchu notification sink enable <sink>
xuanchu notification sink disable <sink>
xuanchu notification sink delete <sink>

xuanchu reminder rule add <name> --trigger due_before --offset 4h --audience assignees --sink <sink>
xuanchu reminder rule list
xuanchu reminder rule info <rule>
xuanchu reminder rule modify <rule> ...
xuanchu reminder rule enable <rule>
xuanchu reminder rule disable <rule>
xuanchu reminder rule delete <rule>

xuanchu notification delivery list --status dead_lettered
xuanchu notification delivery info <delivery>
xuanchu notification delivery replay <delivery>
```

命名可在 implementation plan 中微调，但必须保持用户可理解的层级：sink、rule、delivery 不混淆。

### 11.2 MCP tool 草案

MCP tool name 必须使用下划线：

- `notification_sink_add`
- `notification_sink_list`
- `notification_sink_info`
- `notification_sink_modify`
- `notification_sink_enable`
- `notification_sink_disable`
- `notification_sink_remove`
- `reminder_rule_add`
- `reminder_rule_list`
- `reminder_rule_info`
- `reminder_rule_modify`
- `reminder_rule_enable`
- `reminder_rule_disable`
- `reminder_rule_remove`
- `notification_delivery_list`
- `notification_delivery_info`
- `notification_delivery_replay`

MCP tool 仍然只是 `internal/app` 的薄壳，不在 MCP 层解析 recipient、due 窗口或权限。

### 11.3 HTTP API 草案

HTTP API 可以采用：

- `/api/v1/notification-sinks`
- `/api/v1/reminder-rules`
- `/api/v1/notification-deliveries`

所有接口必须支持 workspace scope 解析，不能允许跨 workspace 读取 sink/rule/delivery。

## 12. Outbound 安全

Notification webhook / HTTP template 请求必须沿用 Hook 的出站网络防护：

- 禁止 loopback。
- 禁止 link-local。
- 禁止 RFC1918 私网。
- 禁止 RFC6598 carrier-grade NAT。
- 禁止 multicast。
- 禁止 unspecified 地址。
- 不自动跟随 HTTP redirect。

动态 endpoint 额外要求：

- 解析后的 `resolved_url` 必须在 delivery 创建时固化。
- retry 和 replay 默认使用 delivery 上固化的 `resolved_url`，不重新套用当前配置。
- 如果管理员希望按新配置重建投递，应通过新的 delivery 生成流程，而不是 replay 旧 delivery。
- `allowed_hosts_json` 是动态 endpoint 的强制字段；`static_url` 也建议保存 host fingerprint，便于审计。
- DNS 解析必须在投递前再次检查 IP 是否落入禁止网段，不能只校验字符串 host。
- `http_template` 的 rendered header/body 中如果包含 secret，不能写入 audit、日志或对外 response。delivery 内部可以保存用于 retry 的渲染快照，但 delivery view 必须脱敏。

签名机制建议沿用 Hook：

- `X-Xuanchu-Event`
- `X-Xuanchu-Event-Id`
- `X-Xuanchu-Event-Version`
- `X-Xuanchu-Signature-256`
- `X-Xuanchu-Timestamp`
- `X-Xuanchu-Delivery`
- `User-Agent`

签名输入：

```text
<delivery_id>.<timestamp_unix_seconds>.<body>
```

## 13. 数据库与 CGO 约束

实现必须继续支持：

- SQLite：`github.com/glebarez/sqlite`
- PostgreSQL：`gorm.io/driver/postgres`
- `CGO_ENABLED=0`

表结构通过 GORM model 表达，避免使用 SQLite 或 PostgreSQL 专属 DDL。时间戳继续使用当前项目的一致整数时间语义。

新增表的唯一约束必须同时在 SQLite 和 PostgreSQL 下工作，特别是 `dedupe_key` 唯一约束。

## 14. 与现有 Hook 的关系

Reminder notification 不替代 Hook。

Hook 继续用于：

- task.created
- task.modified
- task.completed
- task.deleted
- project.archived

Reminder notification 用于：

- task.due_soon
- task.overdue

两者可以共享 dispatcher 内部组件，但用户配置面应分开：

- Hook definition：订阅业务写事件。
- Reminder rule：订阅时间窗口。
- Notification sink：定义出站投递目标。

## 15. 验收标准

规格完成后的实现应满足：

- 可以创建、列出、修改、启停、删除 notification sink。
- 可以创建、列出、修改、启停、删除 reminder rule。
- `due_before` 能在到期前窗口产生 delivery。
- `overdue` 能在逾期后产生 delivery。
- completed / deleted / 无 due 任务不会产生提醒。
- 同一 rule + task + recipient + trigger window 不重复产生 delivery。
- sink disabled 时不会投递，且状态可追踪。
- delivery 支持 retry、dead-letter、manual replay。
- OpenClaw webhook 收到的 payload 包含 workspace、project、rule、task、recipient 和 action_context。
- `http_template` sink 的 method、header template、body template 保存在数据库。
- `http_template` delivery 创建时固化 rendered method、headers、body、content type；retry/replay 使用固化快照。
- notification sink 支持 `static_url`、`template`、`config_value` 三种 endpoint 解析模式。
- 动态 endpoint 只能使用受控变量，并且必须通过 allowed host 和出站网络防护。
- delivery 落库时固化 `resolved_url`，retry/replay 不因配置变化改投其它 endpoint。
- recipient 使用完整 `task.UserInfo` 结构，不能是裸 UUID。
- OpenClaw 后续通过 HTTP/MCP 代表用户操作任务时，复用 M10 impersonation 审计边界。
- MCP tool 命名全部使用下划线。
- HTTP / MCP / CLI 均复用 `internal/app`，不复制业务逻辑。
- `go test ./...`、`CGO_ENABLED=0 go test ./...`、`CGO_ENABLED=0 go build ./cmd/xuanchu` 通过。

## 16. 后续扩展

本规格刻意收窄首版。后续可以独立设计：

- UI 自动化页面。
- 更多 trigger：`scheduled_before`、`wait_released`、`stale_task`。
- 更复杂 audience：watchers、team、role。
- project maintainer 权限模型。
- 通知模板和多语言。
- 静默时间与节假日。
- 汇总提醒，例如每日 9 点发送“今日到期任务”。
- Agent 主动生成行动建议，但仍不接管 scheduler。
- 沙箱脚本 request builder，例如 Starlark 生成 HTTP request 描述，但脚本仍不能直接发网络请求。
