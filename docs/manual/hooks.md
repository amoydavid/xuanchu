---
title: "Webhook Hook 使用指南"
weight: 90
---

# Webhook Hook 使用指南

Hook 是 Xuanchu 的服务端自动化扩展边界。当内部事件发生后，xuanchu 会异步向外部 webhook URL 投递事件。

Hook 不是业务域 adapter 市场。Xuanchu 不内置飞书、Jira、Slack adapter，也不做 memory、replica 或 sync。

## 支持的事件

当前稳定事件：

- `task.created`
- `task.modified`
- `task.completed`
- `task.deleted`
- `project.archived`

## 创建 workspace 级 Hook

```bash
xuanchu hook add audit-sink \
  --scope workspace \
  --event task.created \
  --event task.completed \
  --url https://example.com/xuanchu/webhook \
  --secret-stdin
```

## 创建 project 级 Hook

```bash
xuanchu --workspace dajee hook add mcp-project-hook \
  --scope project \
  --project agentapi \
  --event task.created \
  --event task.modified \
  --url https://example.com/xuanchu/project-hook \
  --secret-file /run/secrets/xuanchu-hook
```

project-scoped hook 只接收该 project 内的事件。

## 管理 Hook

```bash
xuanchu hook list
xuanchu hook list --project agentapi
xuanchu hook info <hook-id>
xuanchu hook modify <hook-id> --name renamed-hook --url https://example.com/new
xuanchu hook disable <hook-id>
xuanchu hook enable <hook-id>
xuanchu hook delete <hook-id>
```

Hook secret 不会出现在 CLI/HTTP response、audit log 或 server log 中。

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

xuanchu dispatcher 发送 `POST` 请求，body 是稳定 JSON envelope。

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
