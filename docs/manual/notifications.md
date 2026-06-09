---
title: "定时通知与第三方通知"
weight: 86
---

# 定时通知与第三方通知

Xuanchu 的通知系统用于“按任务状态和时间生成提醒”，例如任务到期前 4 小时通知负责人，或任务逾期后每天提醒一次。

通知系统由三类对象组成：

- `notification sink`：通知投递目标。可以是 Xuanchu 标准 webhook，也可以是数据库保存的 HTTP request template。
- `reminder rule`：定时提醒规则。首版支持 `due_before` 和 `overdue`。
- `notification delivery`：一次实际投递记录。失败后可重试，dead-letter 后可人工 replay。

Xuanchu 只负责规则评估、幂等生成 delivery、冻结请求快照和投递。OpenClaw、飞书、Slack、邮件等外部系统不内置在 Xuanchu 里，而是通过 sink 接入。

## 与 Hook 的区别

Webhook Hook 是事件驱动：任务创建、修改、完成、删除后投递。

定时通知是时间驱动：scheduler 扫描 pending task 的 `due`，命中 reminder rule 后生成 delivery。

两者都使用 outbox、重试、dead-letter 和人工 replay，但触发源不同。不要把定时提醒塞进 hook event。

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
  --body-template '{"msg_type":"text","content":{"text":"任务 {{task.task_slug}} 即将到期：{{task.description}}"}}'
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

到期前 4 小时提醒任务 assignee：

```bash
xuanchu reminder rule add due-before-4h \
  --trigger due_before \
  --offset 4h \
  --audience assignees \
  --sink openclaw
```

逾期后每天提醒：

```bash
xuanchu reminder rule add overdue-daily \
  --trigger overdue \
  --repeat every:24h \
  --audience assignees \
  --sink openclaw
```

首版 audience 支持：

- `assignees`
- `explicit_users`
- `assignees_and_explicit_users`

不支持 `project_owner` 或 `project_maintainer`。Xuanchu 当前没有稳定的项目负责人模型，不会把 `project.created_by` 当负责人。

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
xuanchu reminder rule modify <rule-id> --repeat every:12h
xuanchu reminder rule disable <rule-id>
xuanchu reminder rule enable <rule-id>
xuanchu reminder rule delete <rule-id>
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
- notification dispatcher：领取 queued/retry_wait delivery 并投递。

常用参数：

```bash
xuanchu server \
  --listen :8080 \
  --reminder-scheduler-interval 1m \
  --notification-dispatcher-interval 5s
```

投递失败不会修改任务状态，也不会回滚任务事务。delivery 自身记录 attempt、last error、next attempt 和 dead-letter 状态。
