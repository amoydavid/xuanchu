# 通知与提醒

通过 Xuanchu MCP 管理 notification sink、reminder rule、notification rule 和通知投递记录。

## 重要原则

- 每次调用都显式传 `workspace`。
- `notification_sink_*` 管理投递目标；`reminder_rule_*` 管理时间驱动提醒规则；`notification_rule_*` 管理事件通知规则；`notification_delivery_*` 管理 delivery。
- 新 reminder rule 优先使用 `schedule_type` + `schedule_value` + `filter_source`，旧 `trigger_type/offset_seconds/after_seconds` 只作为兼容路径。
- 动态 endpoint 必须配置 `allowed_hosts`；delivery 生成后会冻结 URL、header、body，replay 不重新渲染当前模板。
- secret 不要直接写入 URL/body。HTTP template 中通过 `secret_refs` 引用 secret config。
- `sink` 是 workspace 级资源引用，可以用名称或 ID；不能跨 workspace 引用。

## notification_sink_add — 创建 sink

`name` 必填。`type` 可为 `webhook` 或 `http_template`；`endpoint_mode` 可为 `static_url`、`template`、`config_value`。

```json
// OpenClaw 标准 webhook
notification_sink_add({
  "workspace": "dajee",
  "name": "openclaw",
  "type": "webhook",
  "endpoint_mode": "static_url",
  "url": "https://openclaw.example.com/xuanchu/notifications",
  "secret": "webhook-secret"
})

// 按 project 配置读取 endpoint
notification_sink_add({
  "workspace": "dajee",
  "name": "openclaw-project",
  "type": "webhook",
  "endpoint_mode": "config_value",
  "config_key": "integrations.openclaw.notification_url",
  "allowed_hosts": ["openclaw.example.com"]
})
```

## HTTP request template sink

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
  "body_template": "{\"msg_type\":\"text\",\"content\":{\"text\":\"任务 {{task.task_slug}} 即将到期：{{task.description}}\"}}"
})
```

模板变量常用：

- `workspace.id`、`workspace.slug`
- `project.id`、`project.slug`
- `task.uuid`、`task.task_slug`、`task.description`、`task.due`
- `recipient.id`、`recipient.name`、`recipient.email`
- `reminder.sequence`、`reminder.overdue_sequence`、`reminder.window_start`、`reminder.window_end`
- `secret.<alias>`

## sink 管理

```json
notification_sink_list({"workspace": "dajee"})
notification_sink_list({"workspace": "dajee", "include_disabled": true})
notification_sink_info({"workspace": "dajee", "sink": "sink-id-or-name"})
notification_sink_modify({"workspace": "dajee", "sink": "sink-id", "name": "openclaw-prod"})
notification_sink_disable({"workspace": "dajee", "sink": "sink-id"})
notification_sink_enable({"workspace": "dajee", "sink": "sink-id"})
notification_sink_remove({"workspace": "dajee", "sink": "sink-id"})
```

## reminder_rule_add — 创建提醒规则

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

`filter_source` 使用 task query 语法。`status:pending and end.isnull` 覆盖未完成任务；未开始可额外加 `start.isnull`，进行中可额外加 `start.notnull`。`now+24h` / `now-2h` 中的 duration 使用 Go `time.ParseDuration`，支持 `24h`、`90m`、`2h30m`，不支持 `1d`。

兼容型规则：

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

## rule 管理

```json
reminder_rule_list({"workspace": "dajee"})
reminder_rule_list({"workspace": "dajee", "include_disabled": true})
reminder_rule_info({"workspace": "dajee", "rule": "rule-id"})
reminder_rule_modify({"workspace": "dajee", "rule": "rule-id", "schedule_value": "09:30"})
reminder_rule_disable({"workspace": "dajee", "rule": "rule-id"})
reminder_rule_enable({"workspace": "dajee", "rule": "rule-id"})
reminder_rule_remove({"workspace": "dajee", "rule": "rule-id"})
```

## notification_rule_add — 创建事件通知规则

notification rule 与 reminder rule 不同：它监听事件，不做定时扫描。`name`、`event`、`audience`、`sink` 必填。

当前允许的事件：

- `task.created`
- `task.modified`
- `task.completed`
- `task.deleted`
- `project.archived`
- `project.annotated`
- `project.denotated`
- `task.unblocked`

当前不允许注册的事件包括：`task.started`、`task.stopped`、`task.annotated`、`task.denotated`、`task.dependency_added`、`task.dependency_removed`、`project.created`、`project.modified`。

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

task 事件可以使用 `filter` 进一步过滤任务：

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

audience 支持：

- `actor`
- `explicit_users`
- `assignees`
- `assignees_and_explicit_users`

`assignees` 和 `assignees_and_explicit_users` 只支持 `task.*` 事件；project 事件没有 assignee 语义，应使用 `actor` 或 `explicit_users`。

## notification rule 管理

```json
notification_rule_list({"workspace": "dajee"})
notification_rule_list({"workspace": "dajee", "include_disabled": true})
notification_rule_info({"workspace": "dajee", "rule": "rule-id"})
notification_rule_modify({"workspace": "dajee", "rule": "rule-id", "event": "task.unblocked"})
notification_rule_disable({"workspace": "dajee", "rule": "rule-id"})
notification_rule_enable({"workspace": "dajee", "rule": "rule-id"})
notification_rule_remove({"workspace": "dajee", "rule": "rule-id"})
```

## delivery 查看与 replay

```json
notification_delivery_list({"workspace": "dajee", "status": "dead_lettered", "limit": 20})
notification_delivery_info({"workspace": "dajee", "delivery_id": "delivery-id"})
notification_delivery_replay({"workspace": "dajee", "delivery_id": "delivery-id"})
```

`notification_delivery_replay` 只适用于 dead-lettered 或 skipped delivery，不会重新渲染 URL、header、body。

事件通知 delivery 会包含 `object_kind` / `object_id`，用于标识事件对象；reminder delivery 继续以 task 为主。
