---
title: "定时通知与第三方通知"
weight: 86
---

# 定时通知、事件通知与第三方通知

Xuanchu 的通知系统用于“按任务状态、时间或事件生成面向人的通知”，例如任务到期前 4 小时通知负责人，任务逾期后每天提醒一次，或任务解除阻塞后通知 assignee。

通知系统由四类对象组成：

- `notification sink`：外部投递目标。可以是 Xuanchu 标准 webhook，也可以是数据库保存的 HTTP request template。Hook、reminder rule 和 notification rule 都通过 workspace 级 sink 出站。
- `reminder rule`：定时提醒规则。新规则优先使用 `schedule + task filter`，兼容旧的 `due_before` 和 `overdue`。
- `notification rule`：事件通知规则。监听某类事件，解析 audience 后生成面向人的 notification delivery。
- `notification delivery`：一次实际投递记录。失败后可重试，dead-letter 后可人工 replay。

Xuanchu 只负责规则评估、幂等生成 delivery、冻结请求快照和投递。OpenClaw、飞书、Slack、邮件等外部系统不内置在 Xuanchu 里，而是通过 sink 接入。

## 与 Hook 的区别

Webhook Hook 是事件驱动的机器到机器集成：任务或项目事件发生后投递原始事件 envelope。

reminder rule 是时间驱动：scheduler 按 schedule 执行 task filter，命中后生成 delivery。

notification rule 是事件驱动的用户通知：事件发生后按 audience 解析 recipient，生成面向人的 delivery。

三者都使用 outbox、重试、dead-letter 和人工 replay，但触发源、payload 和受众不同。不要把定时提醒塞进 hook event；也不要用 Hook 代替需要 recipient 的用户通知。

## 创建 OpenClaw sink

OpenClaw 通常有向用户发送消息的能力，因此适合作为 Xuanchu 的标准 webhook sink：

```bash
xuanchu notification sink add openclaw \
  --type webhook \
  --url https://openclaw.example.com/xuanchu/notifications \
  --secret "$WEBHOOK_SECRET"
```

动态 endpoint 可以使用模板：

```bash
xuanchu notification sink add openclaw-tenant \
  --type webhook \
  --endpoint-mode template \
  --url-template 'https://openclaw.example.com/tenants/{{workspace.slug}}/xuanchu/notifications' \
  --allowed-host openclaw.example.com
```

也可以从共享配置读取 endpoint：

```bash
xuanchu config schema set integrations.openclaw.notification_url \
  --type string \
  --scope project

xuanchu project config set agentapi integrations.openclaw.notification_url \
  https://openclaw.example.com/apps/agentapi/xuanchu/notifications

xuanchu notification sink add openclaw-project \
  --type webhook \
  --endpoint-mode config_value \
  --config-key integrations.openclaw.notification_url \
  --allowed-host openclaw.example.com
```

动态 endpoint 必须配置 `--allowed-host`。Xuanchu 会在生成 delivery 时解析 URL、校验 host 和 SSRF 风险，并把 `resolved_url` 冻结到 delivery。后续 retry/replay 使用冻结值，不会重新渲染当前 sink 模板。

## 创建第三方 HTTP request template sink

如果第三方通知接口已经固定，可以把 method、header、body 模板保存到数据库：

```bash
xuanchu config schema set integrations.feishu.webhook_url \
  --type string \
  --scope project

xuanchu config schema set integrations.feishu.bot_token \
  --type string \
  --scope project \
  --secret

xuanchu notification sink add feishu-bot \
  --type http_template \
  --endpoint-mode config_value \
  --config-key integrations.feishu.webhook_url \
  --allowed-host open.feishu.cn \
  --header 'Content-Type=application/json' \
  --header 'Authorization=Bearer {{secret.feishu_bot_token}}' \
  --secret-ref feishu_bot_token=integrations.feishu.bot_token \
  --body-content-type application/json \
  --body-template '{"msg_type":"text","content":{"text":"任务 {{task.task_slug}} 即将到期：{{task.title}}"}}'
```

模板里的 header/body 会保存在 `notification_sinks` 表中。secret 不直接写进模板，使用 `--secret-ref alias=config.key` 引用 secret config。

生成 delivery 时，Xuanchu 会把最终请求快照冻结到 delivery：

- `resolved_url`
- `rendered_method`
- `rendered_headers_json`
- `rendered_body`
- `rendered_content_type`

dispatcher 只读取 delivery 快照投递，不重新读取 sink 模板。这保证第三方接口模板被改动后，旧 delivery 的 retry/replay 仍然可解释、可审计。

## 创建 reminder rule

每天 8:50 对“未开始、未结束、未到期，并且 24 小时内即将到期”的任务提醒 assignee：

```bash
xuanchu reminder rule add due-soon-24h \
  --schedule daily@08:50 \
  --filter 'end.isnull and start.isnull and due.after:now and due.before:now+24h' \
  --audience assignees \
  --sink openclaw
```

每天 9:00 对“未开始或进行中、未结束、已到期”的任务发送逾期通知；模板里可以引用 `{{reminder.overdue_sequence}}` 显示这是第几次逾期通知：

```bash
xuanchu reminder rule add overdue-daily \
  --schedule daily@09:00 \
  --filter 'status:pending and end.isnull and due.before:now' \
  --repeat every:24h \
  --audience assignees \
  --sink openclaw
```

`--schedule daily@HH:MM` 会保存为 `schedule_type=daily_at` 和 `schedule_value=HH:MM`。`--filter` 使用 Xuanchu 查询表达式；`status:pending and end.isnull` 覆盖未完成任务，未开始可额外加 `start.isnull`，进行中可额外加 `start.notnull`。`now+24h`、`now-2h` 这类相对时间里的 duration 直接使用 Go `time.ParseDuration` 语法，例如 `24h`、`90m`、`2h30m`，不支持 `1d`。

旧的 `--trigger due_before --offset 4h` 和 `--trigger overdue` 仍作为兼容路径保留。需要表达更丰富条件时，优先使用 `--schedule` 和 `--filter`。

首版 audience 支持：

- `assignees`
- `explicit_users`
- `assignees_and_explicit_users`

不支持 `project_owner` 或 `project_maintainer`。Xuanchu 当前没有稳定的项目负责人模型，不会把 `project.created_by` 当负责人。

## 创建 notification rule

notification rule 用于事件通知。它和 reminder rule 都使用 `notification delivery`，但不做定时扫描；事件进入 app 层后，规则按 `event_type`、可选 project scope 和可选 task filter 匹配。

```bash
xuanchu notification rule add task-unblocked \
  --event task.unblocked \
  --audience assignees \
  --sink openclaw
```

按项目收窄，并只通知显式用户：

```bash
xuanchu notification rule add project-annotation-watch \
  --project agentapi \
  --event project.annotated \
  --audience explicit_users \
  --recipient alice \
  --sink openclaw
```

task 事件可以额外加任务过滤表达式：

```bash
xuanchu notification rule add urgent-task-changes \
  --event task.modified \
  --filter 'priority:H or +urgent' \
  --audience assignees_and_explicit_users \
  --recipient pm@example.com \
  --sink openclaw
```

当前代码允许的 notification rule 事件类型：

- `task.created`
- `task.modified`
- `task.completed`
- `task.deleted`
- `task.started`
- `task.stopped`
- `task.assigned`
- `task.unassigned`
- `task.blocked`
- `task.due_changed`
- `task.priority_changed`
- `task.project_changed`
- `task.tags_changed`
- `project.archived`
- `project.annotated`
- `project.denotated`
- `task.unblocked`

当前不允许注册的候选事件包括：`task.annotated`、`task.denotated`、`task.link_added`、`task.link_removed`、`project.created`、`project.updated`、`workspace.member_added`、`workspace.member_removed`、`workspace.member_role_changed`。

事件语义和 Hook 保持一致：

- `start` 只触发 `task.started`。
- `stop` 只触发 `task.stopped`。
- 普通编辑触发 `task.modified`。
- assignee、due、priority、project、tags、blocked 状态变化会额外触发对应的细粒度事件。
- `task.blocked` 只表示从非 blocked 进入 blocked；`task.unblocked` 只表示解除 blocked。

如果用户通知规则原本依赖 `task.modified` 覆盖所有任务变化，应按通知意图拆成多个规则。例如开始任务通知订阅 `task.started`，到期时间变更通知订阅 `task.due_changed`。这样可以避免在接收端再次解析 payload 判断变化类型。

notification rule 的 audience 支持：

- `actor`
- `explicit_users`
- `assignees`
- `assignees_and_explicit_users`

`assignees` 和 `assignees_and_explicit_users` 只支持 `task.*` 事件；project 事件没有 task assignee 语义，应使用 `actor` 或 `explicit_users`。`--sink` 可以传当前 workspace 内的 sink name 或 sink UUID；跨 workspace sink 会被当作不存在处理。

## 查看和管理

```bash
xuanchu notification sink list
xuanchu notification sink info <sink-id>
xuanchu notification sink modify <sink-id> --name openclaw-prod
xuanchu notification sink disable <sink-id>
xuanchu notification sink enable <sink-id>
xuanchu notification sink delete <sink-id>

xuanchu reminder rule list
xuanchu reminder rule info <rule-id>
xuanchu reminder rule modify <rule-id> --schedule daily@09:30 --filter 'end.isnull and due.before:now'
xuanchu reminder rule disable <rule-id>
xuanchu reminder rule enable <rule-id>
xuanchu reminder rule delete <rule-id>

xuanchu notification rule list
xuanchu notification rule list --project agentapi
xuanchu notification rule info <rule-id>
xuanchu notification rule modify <rule-id> --event task.unblocked --sink openclaw
xuanchu notification rule disable <rule-id>
xuanchu notification rule enable <rule-id>
xuanchu notification rule delete <rule-id>
```

查看投递记录：

```bash
xuanchu notification delivery list --status dead_lettered
xuanchu notification delivery info <delivery-id>
xuanchu notification delivery replay <delivery-id>
```

`replay` 只会把 `dead_lettered` 或 `disabled_skipped` 的 delivery 重新放回队列。它不会重新渲染 URL、header 或 body。

## 服务端运行

`xuanchu server` 会启动两个后台循环：

- reminder scheduler：扫描 due task 和 reminder rule，生成 notification delivery。
- event notification consumer：消费 app 层事件和 notification rule，生成 notification delivery。
- notification dispatcher：领取 queued/retry_wait delivery 并投递。

常用参数：

```bash
xuanchu server \
  --listen :8080 \
  --reminder-scheduler-interval 1m \
  --notification-dispatcher-interval 5s \
  --notification-dispatcher-max-concurrency 1
```

投递失败不会修改任务状态，也不会回滚任务事务。delivery 自身记录 attempt、last error、next attempt 和 dead-letter 状态。

事件通知 delivery 还会记录 `object_kind` / `object_id`，用于区分 task、project 等事件对象。旧 reminder delivery 仍保留 `task_uuid` 语义。

delivery 表是唯一可靠队列。dispatcher 每轮先按可用执行容量领取少量 `queued` / 到期 `retry_wait` delivery，再立即投递；不会把大量 delivery 领取到进程内队列里慢慢等待。`batch_size` 是每轮查询上限，不是并发数。`max_concurrency` 默认 `1`，保持顺序投递；sink 的 `max_concurrency=0` 表示继承默认 sink 并发，显式大于 `0` 时限制该 workspace 内同一 sink 的并发。`xuanchu server` 内 notification dispatcher 和 hook dispatcher 共享同一个 sink limiter，同一 sink 的单进程并发不会因为两个 runtime 同时工作而翻倍。

投递 payload / HTTP template context 会带稳定幂等字段：顶层 `delivery_id`、`attempt`、`workspace_id`、`sink_id`、`rule_id`、`object_kind`、`object_id`、`created_at`，以及嵌套 `delivery.id`、`delivery.attempt`、`object.kind`、`object.id`。body 中的 `attempt` 是入队时冻结的初始上下文；本次 HTTP 请求真实尝试次数看 header `X-Xuanchu-Attempt`。
