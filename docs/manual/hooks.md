---
title: "Webhook Hook 使用指南"
weight: 90
---

# Webhook Hook 使用指南

Hook 是 Xuanchu 的服务端自动化扩展边界。当内部事件发生后，xuanchu 会异步通过 workspace 级 outbound sink 投递事件。

Hook 不是业务域 adapter 市场。Xuanchu 不内置飞书、Jira、Slack adapter，也不做 memory、replica 或 sync。

## 支持的事件

当前稳定事件：

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
- `project.transitioned`
- `project.annotated`
- `project.denotated`
- `task.unblocked`

说明：当前代码白名单只包含以上事件。`task.annotated`、`task.denotated`、`task.link_added`、`task.link_removed`、`project.created`、`project.updated`、`workspace.member_added`、`workspace.member_removed`、`workspace.member_role_changed` 等 Priority 2 事件属于后续版本候选，不能在当前 hook 中注册。

## 事件语义与迁移

从 v0.3.0 开始，Hook 事件语义按具体动作拆分，不再把所有任务动作都折叠进 `task.modified`：

- `xuanchu start <task>` 只触发 `task.started`。
- `xuanchu stop <task>` 只触发 `task.stopped`。
- 普通 `modify` 触发 `task.modified`。
- assignee、due、priority、project、tags、blocked 状态变化会额外触发对应的细粒度事件。
- 任务从非 blocked 变为 blocked 时触发 `task.blocked`；从 blocked 变为非 blocked 时触发 `task.unblocked`。

如果旧集成曾经只监听 `task.modified` 来捕获所有任务变化，需要按真实用途补充订阅：

```bash
xuanchu hook add task-change-watch \
  --event task.modified \
  --event task.started \
  --event task.stopped \
  --event task.due_changed \
  --event task.priority_changed \
  --event task.tags_changed \
  --event task.blocked \
  --event task.unblocked \
  --sink audit-stream
```

字段级自动化建议直接订阅细粒度事件。这样接收方不需要从宽泛的 `task.modified` payload 中再次推断变化类型。

## 创建 sink

Hook 不直接保存 URL 或 secret。目标 endpoint、header、body、secret、动态 endpoint allowlist 都由 notification sink 描述：

```bash
xuanchu notification sink add audit-stream \
  --type webhook \
  --url https://example.com/xuanchu/webhook \
  --secret "$WEBHOOK_SECRET"
```

`--sink <sink-ref>` 可以传 sink name 或 sink UUID。sink-ref 是 workspace 级隔离资源引用：按 name 解析时只查当前 workspace；按 UUID 解析时也必须属于当前 workspace。project-scoped hook 可以限制事件来源 project，但不能引用其他 workspace 的 sink。

## 创建 workspace 级 Hook

```bash
xuanchu hook add audit-sink \
  --scope workspace \
  --event task.created \
  --event task.completed \
  --sink audit-stream
```

## 创建 project 级 Hook

```bash
xuanchu --workspace dajee hook add mcp-project-hook \
  --scope project \
  --project agentapi \
  --event task.created \
  --event task.modified \
  --sink audit-stream
```

project-scoped hook 只接收该 project 内的事件。

## 管理 Hook

```bash
xuanchu hook list
xuanchu hook list --project agentapi
xuanchu hook info <hook-id>
xuanchu hook modify <hook-id> --name renamed-hook --sink audit-stream
xuanchu hook disable <hook-id>
xuanchu hook enable <hook-id>
xuanchu hook delete <hook-id>
```

Hook response 中会返回 `sink_id`、`sink_name`、`sink_type`，不会返回 sink secret。secret 不会出现在 CLI/HTTP response、audit log 或 server log 中。

## 查看投递

```bash
xuanchu hook deliveries <hook-id>
xuanchu hook deliveries <hook-id> --status dead_lettered
```

可见 delivery 状态包括：

- `queued`
- `delivering`
- `retry_wait`
- `succeeded`
- `dead_lettered`
- `disabled_skipped`

手动 replay：

```bash
xuanchu hook replay <delivery-id>
```

manual replay 会写 audit。

## Webhook 请求

xuanchu dispatcher 使用 delivery 里冻结的请求快照发送 HTTP 请求。标准 webhook sink 的 body 是稳定 JSON envelope；`http_template` sink 可以用模板改写 header/body。

hook delivery 表是 hook dispatcher 的可靠队列。dispatcher 默认 `max_concurrency=1`，每轮按可用执行容量领取到期 delivery；超过进程级或 sink 级并发的 delivery 留在数据库队列中等待。sink `max_concurrency=0` 表示继承默认 sink 并发；`xuanchu server` 内 hook dispatcher 与 notification dispatcher 共享同一个 sink limiter。

请求 body 中会包含稳定 `delivery_id`、`hook_id`、`rule_id`、`workspace_id`、`sink_id`、`object_kind`、`object_id`、`created_at`，供接收方幂等去重和审计。body 里的 `attempt` 是入队时冻结的初始上下文；本次 HTTP 请求真实尝试次数看 header `X-Xuanchu-Attempt`。

常见 header：

- `X-Xuanchu-Event`
- `X-Xuanchu-Event-Id`
- `X-Xuanchu-Event-Version`
- `X-Xuanchu-Signature-256`
- `X-Xuanchu-Timestamp`
- `X-Xuanchu-Delivery`
- `X-Xuanchu-Hook-Id`
- `X-Xuanchu-Attempt`
- `User-Agent`

签名输入：

```text
<delivery_id>.<timestamp_unix_seconds>.<body>
```

签名格式：

```text
sha256=<hex>
```

Python 验证示例：

```python
import hmac, hashlib

def verify_signature(secret, delivery_id, timestamp, body, signature):
    signing_input = f"{delivery_id}.{timestamp}.{body}"
    expected = "sha256=" + hmac.new(
        secret.encode(), signing_input.encode(), hashlib.sha256
    ).hexdigest()
    return hmac.compare_digest(expected, signature)
```

建议消费方拒绝超过 5 分钟窗口的请求，降低重放风险。

## 重试与 Dead Letter

Hook 投递是 post-commit 异步语义：

- 本地 task/project 事务成功后，事件进入 durable delivery queue。
- 外部 webhook 失败不会回滚本地事务。
- delivery 生成时冻结 sink 渲染后的 URL、method、headers、body、content type。
- retry/replay 使用冻结快照，不会按当前 sink 配置重新渲染。
- 失败后按退避策略重试。
- 超过最大尝试次数后进入 dead-letter。
- dead-letter 可手动 replay。

## 出站网络防护

Xuanchu 默认禁止投递到：

- loopback
- link-local
- RFC1918 私网
- RFC6598 carrier-grade NAT
- multicast
- unspecified 地址

HTTP 3xx redirect 不会被自动跟随。

## Web Console 配置

`/hooks`（或等价的 `/integrations`、`/notifications`）现在是**出站集成控制台**，可在浏览器内完成 sink → hook → 投递 → replay 的完整闭环：

- **Sinks tab**：创建/编辑/启停/删除 webhook 与 http_template sink。
  - `webhook` 适合标准 webhook（URL + secret + 重试）。
  - `http_template` 适合自定义 HTTP 请求（header/body 模板、secret refs、allowed hosts）。
  - 编辑 sink 时 secret 不回显，提交新值才会覆盖。
- **Hooks tab**：创建/编辑 hook，从 workspace 的 sink 列表中下拉选择目标，事件类型用分组 checkbox 勾选，project scope 用项目选择器。
- **通知规则 / 定时规则**：事件通知规则与定时提醒规则共享同一个控制台。
- **概览**：并行汇总 sinks / hooks / 规则计数与最近 dead-letter 投递。

> 事件名遵循 `task.completed`、`project.archived` 等白名单，旧的 `task.done` 不再使用。

### Sink 测试投递

每个 sink 行有「发送测试」按钮，会调用 `POST /api/v1/notification-sinks/{sinkID}/test`：

- 请求 body：`{"kind":"hook|notification","event_type":"task.completed","project_ref":"<可选>"}`。
  - `kind=hook` 时 `event_type` 必须在 [支持的事件](#支持的事件) 白名单内；`kind=notification` 时 `event_type` 仅作为样例 payload 标签，不强制白名单。
  - `project_ref` 仅对 `config_value` endpoint 有意义，用于在 project config 下解析目标 URL；其它 mode 下被忽略。
- 后端使用与正式 dispatch 相同的 sink 渲染、SSRF 防护（loopback / 私网 / link-local 拒绝）与 HTTP 投递。
- 测试投递**不会**写入 `hook_deliveries` / `notification_deliveries`，只写一条 `notification.sink.test` audit（payload 只含 sink_id / kind / event_type / status / status_code / endpoint fingerprint，不含 secret / 完整 body）。
- 发往目标的请求带：`X-Xuanchu-Test: true`、独立的 `X-Xuanchu-Delivery: test-<uuid>`、`X-Xuanchu-Event: sink.test`；webhook 类型且配置了 secret 时还会带 `X-Xuanchu-Timestamp` 与 `X-Xuanchu-Signature-256`（HMAC-SHA256 算法与正式投递一致，接收方可用同一套验签逻辑）。
- 测试结果在 UI 展示 status code、duration、endpoint fingerprint、rendered method / headers / body 摘要。
- 目标返回 4xx/5xx 时 API 仍返回 HTTP 200，body 中 `status` 为 `failed`——因为 API 调用本身成功，是测试结果失败。配置无效 / 权限不足 / sink 不存在 / SSRF 拒绝仍返回正常 API 错误（4xx）。
- 安全提示：`http_template` sink 的 rendered headers / body 是模板渲染后的真实请求内容，若模板里引用了 `{{ secret.* }}`，渲染结果会出现在响应的 `rendered_headers` / `rendered_body_preview` 中（与正式 delivery 详情展示语义一致，便于排障）。webhook sink 的 HMAC `secret` 不会出现在任何响应字段，只用于签名。
