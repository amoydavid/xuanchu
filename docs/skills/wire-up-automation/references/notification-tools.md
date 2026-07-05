# Notification 工具

管理 notification sink（投递目标）、reminder rule（定时提醒）、notification rule（事件通知）和 delivery（投递记录）。

## Notification Sink

### notification_sink_add — 创建 sink

`name` 必填。`type` 可为 `webhook` 或 `http_template`；`endpoint_mode` 可为 `static_url`、`template`、`config_value`。`max_concurrency` 可选，`0` 表示继承 dispatcher 默认 sink 并发。

```json
// OpenClaw 标准 webhook（静态 URL）
notification_sink_add({
  "workspace": "dajee",
  "name": "openclaw",
  "type": "webhook",
  "endpoint_mode": "static_url",
  "url": "https://openclaw.example.com/xuanchu/notifications",
  "secret": "webhook-secret",
  "max_concurrency": 0
})

// 按 project 配置读取 endpoint（多 project 共享 sink，各 project 配自己的 URL）
notification_sink_add({
  "workspace": "dajee",
  "name": "openclaw-project",
  "type": "webhook",
  "endpoint_mode": "config_value",
  "config_key": "integrations.openclaw.notification_url",
  "allowed_hosts": ["openclaw.example.com"],
  "max_concurrency": 1
})
```

### HTTP request template sink

第三方固定 Web API 使用 `http_template`。header/body 模板保存在数据库里。

```json
notification_sink_add({
  "workspace": "dajee",
  "name": "feishu-bot",
  "type": "http_template",
  "endpoint_mode": "config_value",
  "config_key": "integrations.feishu.webhook_url",
  "allowed_hosts": ["open.feishu.cn"],
  "header_templates": [
    {"name": "Content-Type", "value": "application/json"},
    {"name": "Authorization", "value": "Bearer {{secret.feishu_bot_token}}"}
  ],
  "secret_refs": [
    {"alias": "feishu_bot_token", "config_key": "integrations.feishu.bot_token"}
  ],
  "body_content_type": "application/json",
  "body_template": "{\"msg_type\":\"text\",\"content\":{\"text\":\"任务 {{task.task_slug}} 即将到期：{{task.title}}\"}}",
  "max_concurrency": 0
})
```

### sink 管理

```json
notification_sink_list({"workspace": "dajee"})
notification_sink_list({"workspace": "dajee", "include_disabled": true})
notification_sink_info({"workspace": "dajee", "sink": "sink-id-or-name"})
notification_sink_modify({"workspace": "dajee", "sink": "sink-id", "name": "openclaw-prod"})
notification_sink_modify({"workspace": "dajee", "sink": "sink-id", "max_concurrency": 2})
notification_sink_disable({"workspace": "dajee", "sink": "sink-id"})
notification_sink_enable({"workspace": "dajee", "sink": "sink-id"})
notification_sink_remove({"workspace": "dajee", "sink": "sink-id"})
```

## Reminder Rule

### reminder_rule_add — 创建提醒规则

`name`、`audience_type`、`sink` 必填。`audience_type` 支持 `assignees`、`explicit_users`、`assignees_and_explicit_users`。

每天 8:50 给未开始、未结束、未来 24 小时内到期的任务发预警：

```json
reminder_rule_add({
  "workspace": "dajee",
  "name": "due-soon-24h",
  "schedule_type": "daily_at",
  "schedule_value": "08:50",
  "filter_source": "end.isnull and start.isnull and due.after:now and due.before:now+24h",
  "audience_type": "assignees",
  "sink": "openclaw"
})
```

每天 9:00 给未开始或进行中、未结束、已到期的任务发逾期通知：

```json
reminder_rule_add({
  "workspace": "dajee",
  "name": "overdue-daily",
  "schedule_type": "daily_at",
  "schedule_value": "09:00",
  "filter_source": "status:pending and end.isnull and due.before:now",
  "repeat_policy": "every:24h",
  "audience_type": "assignees",
  "sink": "openclaw"
})
```

`filter_source` 使用 task query 语法。`status:pending and end.isnull` 覆盖未完成任务；未开始可额外加 `start.isnull`，进行中可额外加 `start.notnull`。`now+24h` / `now-2h` 中的 duration 使用 Go `time.ParseDuration`，支持 `24h`、`90m`、`2h30m`，**不支持 `1d`**。

兼容型规则（旧路径）：

```json
reminder_rule_add({
  "workspace": "dajee",
  "name": "due-before-4h",
  "trigger_type": "due_before",
  "offset_seconds": 14400,
  "audience_type": "assignees",
  "sink": "openclaw"
})
```

### rule 管理

```json
reminder_rule_list({"workspace": "dajee"})
reminder_rule_list({"workspace": "dajee", "include_disabled": true})
reminder_rule_info({"workspace": "dajee", "rule": "rule-id"})
reminder_rule_modify({"workspace": "dajee", "rule": "rule-id", "schedule_value": "09:30"})
reminder_rule_disable({"workspace": "dajee", "rule": "rule-id"})
reminder_rule_enable({"workspace": "dajee", "rule": "rule-id"})
reminder_rule_remove({"workspace": "dajee", "rule": "rule-id"})
```

## Notification Rule

### notification_rule_add — 创建事件通知规则

notification rule 与 reminder rule 不同：它监听事件，不做定时扫描。`name`、`event`、`audience`、`sink` 必填。

通知任务解除阻塞后的 assignee：

```json
notification_rule_add({
  "workspace": "dajee",
  "name": "task-unblocked",
  "event": "task.unblocked",
  "audience": "assignees",
  "sink": "openclaw"
})
```

监听某项目的项目注释，并通知显式用户：

```json
notification_rule_add({
  "workspace": "dajee",
  "project": "agentapi",
  "name": "project-annotation-watch",
  "event": "project.annotated",
  "audience": "explicit_users",
  "recipients": ["alice"],
  "sink": "openclaw"
})
```

task 事件可使用 `filter` 进一步过滤：

```json
notification_rule_add({
  "workspace": "dajee",
  "name": "urgent-task-changes",
  "event": "task.modified",
  "filter": "priority:H or +urgent",
  "audience": "assignees_and_explicit_users",
  "recipients": ["pm@example.com"],
  "sink": "openclaw",
  "template_subject": "任务事件 {{event.type}}",
  "template_body": "{{event.json}}"
})
```

audience 支持：`actor`、`explicit_users`、`assignees`、`assignees_and_explicit_users`。

`assignees` 和 `assignees_and_explicit_users` 只支持 `task.*` 事件；project 事件没有 assignee 语义，应使用 `actor` 或 `explicit_users`。

### notification rule 管理

```json
notification_rule_list({"workspace": "dajee"})
notification_rule_list({"workspace": "dajee", "include_disabled": true})
notification_rule_info({"workspace": "dajee", "rule": "rule-id"})
notification_rule_modify({"workspace": "dajee", "rule": "rule-id", "event": "task.unblocked"})
notification_rule_disable({"workspace": "dajee", "rule": "rule-id"})
notification_rule_enable({"workspace": "dajee", "rule": "rule-id"})
notification_rule_remove({"workspace": "dajee", "rule": "rule-id"})
```

## Delivery 查看与 replay

```json
notification_delivery_list({"workspace": "dajee", "status": "dead_lettered", "limit": 20})
notification_delivery_info({"workspace": "dajee", "delivery_id": "delivery-id"})
notification_delivery_replay({"workspace": "dajee", "delivery_id": "delivery-id"})
```

`notification_delivery_replay` 只适用于 dead-lettered 或 skipped delivery，**不会重新渲染** URL、header、body。

事件通知 delivery 会包含 `object_kind` / `object_id`，用于标识事件对象；reminder delivery 继续以 task 为主。

## Sink 测试投递

`POST /api/v1/notification-sinks/{sinkID}/test` 可以在不触发真实任务事件的前提下，对目标 sink 发一次样例投递：

- 请求 body：`{"kind":"hook|notification","event_type":"task.completed","project_ref":"<可选>"}`
  - `kind=hook` 时 `event_type` 必须在 hook 事件白名单内；`kind=notification` 时不强制白名单。
  - `project_ref` 仅对 `config_value` endpoint 有效（在 project config 下解析 URL）。
- 需 `notification:write` 权限。
- 发往目标的请求带 `X-Xuanchu-Test: true` + 独立 `X-Xuanchu-Delivery: test-<uuid>` + `X-Xuanchu-Event: sink.test`；webhook sink 配了 secret 时还带 `X-Xuanchu-Signature-256`（与正式投递同算法，可复用验签逻辑）。
- 响应包含 status / status_code / duration_ms / endpoint fingerprint / rendered method·headers·body preview。
- 失败的目标返回（4xx/5xx）不会让 API 报错——API 返回 HTTP 200，`status` 为 `failed`；配置无效 / 权限不足 / sink 不存在 / SSRF 拒绝返回正常 4xx。
- 写一条 `notification.sink.test` audit（不含 secret / 完整 body），**不**写入 `notification_deliveries`。
- 安全提示：`http_template` sink 的 `rendered_headers` / `rendered_body_preview` 是模板渲染后的真实请求内容，若模板里引用了 `{{ secret.* }}`，渲染值会出现在响应里（便于排障）；webhook sink 的 HMAC `secret` 不会出现在响应中。

Web Console 的 Sinks tab 提供「发送测试」UI 入口。
