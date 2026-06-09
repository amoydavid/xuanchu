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
- `project.archived`
- `project.annotated`
- `project.denotated`
- `task.unblocked`

说明：当前代码白名单只包含以上事件。`task.started`、`task.stopped`、`task.annotated`、`task.denotated`、`task.dependency_added`、`task.dependency_removed`、`project.created`、`project.modified` 等事件属于后续版本候选，不能在当前 hook 中注册。

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
