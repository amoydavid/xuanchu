# Xuanchu Dispatcher 并发、背压与可靠投递设计

> **给 agentic workers 的要求：** 编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

**目标：** 为 notification dispatcher 和 hook dispatcher 增加明确的并发、背压和重启恢复契约，确保 Xuanchu 调用外部 HTTP API 时不会无限并发，也不会因为进程重启丢失已经进入队列的消息。

**范围策略：** 本规格只设计出站投递运行时。它建立在现有 `notification_deliveries`、`hook_deliveries`、outbound sink、claim、retry、dead-letter、manual replay 和 stale recovery 之上，不重新设计 reminder rule、notification rule、hook definition 或 sink 模板语法。

---

## 1. 背景与现状

Xuanchu 当前已经具备两类出站投递：

- notification dispatcher：投递 reminder rule / event notification rule 生成的 `notification_deliveries`。
- hook dispatcher：投递 hook definition 生成的 `hook_deliveries`。

两类 delivery 表都已经具备：

- `queued`
- `delivering`
- `retry_wait`
- `succeeded`
- `dead_lettered`
- `disabled_skipped`
- `attempt_count`
- `next_attempt_at`
- `claim_expires_at`
- `last_attempt_at`
- `last_status_code`
- `last_error`

当前 dispatcher 的实际行为是：

- 每轮先执行 `RecoverStaleDelivering(now)`。
- 再执行 `ClaimDue(now, claimExpiresAt, BatchSize)`。
- 然后在同一个 goroutine 内顺序投递。

因此当前单进程内等价于 `max_concurrency = 1`。这不会打爆外部系统，但吞吐偏低；同时 `BatchSize` 容易被误解为并发数。未来一旦改成并行投递，如果没有明确的并发和背压设计，就可能出现以下问题：

- 一次 claim 大量 delivery，进程重启后大量消息长时间处于 `delivering`。
- 外部系统慢时，Xuanchu 进程内堆积大量待发请求。
- 某个 sink 或 workspace 的大量投递挤占其它投递。
- 多 server 副本部署时，整体并发随副本数线性放大。
- 对方接口收到重复请求时无法去重。

---

## 2. 核心原则

### 2.1 数据库是唯一可靠队列

可靠队列只能是数据库中的 delivery 表。进程内 worker、channel、semaphore 只能做短暂执行协调，不能承载业务状态。

规则：

- 已生成的 delivery 必须先落库，再由 dispatcher 投递。
- 未投递的消息保持 `queued` 或 `retry_wait`。
- 正在投递的消息使用 `delivering + claim_expires_at` 表达。
- 进程重启后不恢复内存队列，只恢复 DB 中过期的 `delivering`。
- 成功、重试、死信、禁用跳过都必须落库。

### 2.2 不用进程内队列承载可靠性

禁止把大量 delivery claim 到本进程内再慢慢排队投递：

```text
错误模型：
claim 100 条 -> 放进内存 channel -> worker 慢慢发
```

这个模型在进程重启时不会丢失数据库记录，但会让大量实际还没开始投递的 delivery 进入 `delivering`，只能等 `claim_expires_at` 过期后恢复，造成延迟和重复投递风险。

推荐模型：

```text
正确模型：
计算可用执行容量 -> claim 少量 delivery -> 立即交给 worker 投递
```

### 2.3 至少一次投递

Xuanchu 对外部 HTTP API 的语义是：

```text
at-least-once delivery + durable retry + delivery_id 去重线索
```

不承诺 exactly-once。原因是网络请求可能出现“请求已到达外部系统，但 Xuanchu 没收到响应”的状态。此时安全策略是重试，并通过稳定 `delivery_id` 让接收方具备幂等处理条件。

### 2.4 并发和限速是两件事

- 并发控制：同时最多有多少个请求正在执行。
- 限速控制：单位时间最多发多少个请求。

首版必须实现并发控制。限速可以在模型中预留，但不作为首版必做能力。

---

## 3. 设计目标

首版完成后应支持：

- notification dispatcher 和 hook dispatcher 都有进程级 `max_concurrency`。
- outbound sink 有 workspace 内 sink 级的 `max_concurrency`。
- dispatcher 的 claim 数量受可用执行容量约束。
- 超过并发容量的 delivery 留在 DB 队列中等待，不进入可靠性依赖的进程内队列。
- 系统重启不会丢失 queued / retry_wait / delivering delivery。
- 启动或每轮调度前恢复过期 `delivering`。
- delivery payload 中携带稳定 `delivery_id`，供外部系统幂等去重。
- 文档明确 `BatchSize`、`max_concurrency`、`prefetch_factor`、`timeout`、`claim_ttl` 的关系。

---

## 4. 非目标

本规格不做：

- 不引入 Redis、Kafka、NATS 等外部队列。
- 不做集群级严格全局并发。
- 不做复杂动态自适应限流。
- 不做任意脚本执行。
- 不内置 OpenClaw、飞书、Slack、邮件等具体 adapter。
- 不改变 delivery 冻结请求快照的规则。
- 不改变 reminder rule、notification rule、hook definition 的业务语义。
- 不承诺 exactly-once。

---

## 5. 概念模型

### 5.1 BatchSize

`BatchSize` 表示单轮调度最多考虑 claim 的数量上限，不表示并发。

它的作用是防止一轮查询过大。实际 claim 数量必须继续受 worker 空闲容量约束。

### 5.2 MaxConcurrency

`max_concurrency` 表示同一运行时范围内最多同时执行多少个外部 HTTP 请求。

首版需要两个层级：

- dispatcher 级：限制当前进程内 notification / hook dispatcher 的总并发。
- sink 级：限制同一 workspace 下某个 outbound sink 的并发。

### 5.3 PrefetchFactor

`prefetch_factor` 表示允许 claim 的 delivery 数量相对于可用 worker slot 的倍数。

首版默认：

```text
prefetch_factor = 1
```

即有多少空闲执行容量，就最多 claim 多少条 delivery。

如果未来需要提高吞吐，可以允许：

```text
claim_limit = min(batch_size, available_slots * prefetch_factor)
```

但 `prefetch_factor` 不应默认大于 1。

### 5.4 ClaimTTL

`claim_ttl` 是 delivery 进入 `delivering` 后，最多允许保持不可见的时间。

`claim_expires_at` 必须覆盖外部请求 timeout 和少量缓冲。现有代码已经按 sink / hook timeout 与基础 TTL 取较大值的思路计算，后续实现不能退化为固定短 TTL。

### 5.5 Timeout

`timeout` 是单次外部 HTTP 请求的最大执行时间。

规则：

- sink timeout 用于 notification delivery。
- hook timeout 或 sink timeout 用于 hook delivery，具体以现有 hook sink 化实现计划为准。
- `claim_expires_at` 必须晚于请求 timeout。

---

## 6. 配置与数据模型

### 6.1 进程级 dispatcher 配置

进程级配置属于 server runtime config，适合放在 `config.toml` 和命令行 flag。

建议 TOML：

```toml
[notifications.dispatcher]
max_concurrency = 4
batch_size = 50
prefetch_factor = 1
poll_interval = "5s"
claim_ttl = "5m"

[hooks.dispatcher]
max_concurrency = 4
batch_size = 50
prefetch_factor = 1
poll_interval = "5s"
claim_ttl = "5m"
```

默认值建议：

| 配置 | 默认值 | 说明 |
|---|---:|---|
| `max_concurrency` | `1` | 保持与当前顺序投递一致，管理员需要吞吐时显式调大 |
| `batch_size` | `50` | 查询上限，不是并发 |
| `prefetch_factor` | `1` | 保证 claim 后能很快执行 |
| `poll_interval` | `5s` | 延续现有默认 |
| `claim_ttl` | `5m` | 延续现有默认 |

首版默认必须选择保守值：

```text
max_concurrency = 1
```

这样升级后默认行为与当前顺序投递一致。管理员需要吞吐时再显式调大。

### 6.2 Sink 级并发配置

sink 级配置属于 workspace 资源属性，必须保存在数据库中，而不是只写在本地 `config.toml`。

建议新增字段：

- `notification_sinks.max_concurrency`

语义：

- `NULL` 或 `0`：继承 dispatcher 级限制，不单独限制。
- `> 0`：该 sink 在当前进程内最多同时执行的投递数。

后续可预留但首版不要求实现：

- `notification_sinks.rate_limit_count`
- `notification_sinks.rate_limit_window_seconds`

### 6.3 Workspace 级并发

workspace 级并发有价值，但首版可以不暴露用户配置。

实现上应避免把架构做死。后续可以增加：

- `workspace_dispatcher_limits.max_notification_concurrency`
- `workspace_dispatcher_limits.max_hook_concurrency`

或把它放入 workspace settings。

---

## 7. 调度流程

### 7.1 单轮流程

dispatcher 每轮应遵循：

```text
1. now = clock.Unix()
2. RecoverStaleDelivering(now)
3. 计算 dispatcher 可用全局 slot
4. 如果没有 slot，结束本轮
5. claim_limit = min(batch_size, available_global_slots * prefetch_factor)
6. 从 DB claim due delivery，数量不超过 claim_limit
7. 对每条 delivery：
   7.1 读取 sink / hook / rule 必要元数据
   7.2 尝试获得 sink 级并发 token
   7.3 获得 token 后立即交给 worker 执行
   7.4 如果无法获得 token，不应长期占用 delivering；首选是不提前 claim 该 sink 的 delivery
8. worker 完成后更新 delivery 状态并释放 token
```

实现时可以分阶段：

- 第一阶段只做 dispatcher 级并发，claim 数量受全局 slot 限制。
- 第二阶段增加 sink 级并发，并尽量在 claim 查询时按 sink 排除已满 sink。

如果第一阶段已 claim 到某个 sink 的 delivery，但发现 sink token 已满，必须尽快把该 delivery requeue 或设置短 `next_attempt_at`，不能让它长期停留在 `delivering`。

### 7.2 Claim 查询要求

claim 查询必须保持：

- 只 claim `queued` 和到期的 `retry_wait`。
- 按 `created_at ASC` 优先处理旧消息。
- claim 和状态修改必须在单条 SQL 或事务中完成，避免多进程重复 claim。
- 支持 SQLite 和 PostgreSQL。
- 不使用 PostgreSQL 不兼容的未类型化 `MAX(unknown, bigint)`。

当实现 sink 级并发后，claim 查询可以增加 sink 过滤条件：

```text
WHERE sink_id IN (<当前仍有容量的 sink>)
```

首版不要求 DB 查询本身感知所有 sink 负载，但不能在进程内形成大量可靠性依赖队列。

### 7.3 Worker Pool

worker pool 应是 bounded 的。

规则：

- worker 数量不超过 `max_concurrency`。
- 每个 worker 处理一个 delivery 后退出或回到池中。
- context canceled 时，不再 claim 新 delivery。
- 正在执行的 HTTP 请求由 request context timeout 控制。
- server shutdown 不等待无限长时间；未完成的 delivery 依靠 `claim_expires_at` 恢复。

---

## 8. 重启与崩溃恢复

系统重启后：

- `queued` 仍然可被后续 claim。
- `retry_wait` 在 `next_attempt_at` 到期后可被 claim。
- `delivering` 在 `claim_expires_at < now` 后由 `RecoverStaleDelivering` 恢复为 `queued`。
- `succeeded` 不再自动投递。
- `dead_lettered` 只允许人工 replay。
- `disabled_skipped` 只允许人工 replay。

启动时必须执行一次 stale recovery；持续运行时每轮也应执行。

恢复逻辑不应删除 delivery，不应修改冻结请求快照，不应重置 `attempt_count`。

---

## 9. 幂等与外部去重

所有出站 payload 必须包含稳定投递标识。

Hook envelope 至少包含：

- `delivery_id`
- `event_id`
- `event_type`
- `workspace_id`
- `hook_id`
- `sink_id`
- `attempt`
- `created_at`

Notification envelope 至少包含：

- `delivery_id`
- `workspace_id`
- `rule_id` 或 `hook_event_rule_id`
- `sink_id`
- `object_kind`
- `object_id`
- `recipient`
- `attempt`
- `created_at`

对于 `http_template` sink，模板上下文必须提供：

```text
{{delivery.id}}
{{delivery.attempt}}
{{delivery.workspace_id}}
{{delivery.sink_id}}
{{event.id}}
{{event.type}}
{{object.kind}}
{{object.id}}
```

外部系统如果支持幂等，应使用 `delivery_id` 去重。

---

## 10. 多实例部署边界

首版只保证单进程内严格并发。

如果部署多个 `xuanchu server` 副本：

```text
整体最大并发 ~= 单实例 max_concurrency * 副本数
```

sink 级并发同理：

```text
整体 sink 最大并发 ~= 单实例 sink max_concurrency * 副本数
```

这是可接受的首版边界。若未来需要集群级严格并发，需要新增分布式 semaphore。可选方案：

- 基于数据库的 lease 表。
- 基于 Redis 的 token bucket / semaphore。

本规格不要求实现。

---

## 11. 错误处理

### 11.1 可重试错误

以下错误应进入 retry：

- 网络连接失败。
- 请求超时。
- HTTP `429`。
- HTTP `5xx`。

下一次重试时间继续使用现有指数退避和 jitter 策略。

### 11.2 不可重试错误

以下错误应进入 dead-letter 或 disabled-skip：

- endpoint URL 非法或 SSRF 校验失败。
- sink 不存在。
- sink workspace 不匹配。
- hook definition 不存在。
- 请求构造失败。
- 明确的 `4xx`，但 `429` 除外。
- sink / hook 已禁用。

### 11.3 请求取消

server shutdown 或 context canceled 时：

- 不再 claim 新 delivery。
- 已经开始的请求可以被 request context 取消。
- 被取消的 `delivering` delivery 不应直接标记失败；由 `claim_expires_at` 到期后恢复，除非实现能明确区分该请求尚未发出。

---

## 12. 可观测性

首版至少需要在日志中区分：

- 本轮 claim 数量。
- 本轮 stale recovery 数量。
- delivery 成功、重试、死信数量。
- 因没有全局 slot 跳过的轮次。
- 因 sink slot 已满跳过或 requeue 的数量。

后续可以暴露 metrics：

- `xuanchu_dispatcher_inflight`
- `xuanchu_dispatcher_claimed_total`
- `xuanchu_dispatcher_succeeded_total`
- `xuanchu_dispatcher_retry_total`
- `xuanchu_dispatcher_dead_lettered_total`
- `xuanchu_dispatcher_sink_inflight`
- `xuanchu_dispatcher_stale_recovered_total`

---

## 13. 安全边界

并发控制不能削弱现有出站安全：

- endpoint 仍必须做 SSRF 防护。
- 不跟随重定向。
- secret 明文不出现在 view、audit、delivery list 中。
- retry/replay 使用冻结请求快照。
- sink-ref 仍按 workspace 隔离。
- delivery payload 中的用户身份继续使用 `task.UserInfo`，不退化成裸 UUID。

---

## 14. 验收标准

实现完成后应能验证：

- `max_concurrency=1` 时行为与当前顺序投递一致。
- `max_concurrency=3` 时，同时执行的外部请求不超过 3。
- `batch_size=50` 且 `max_concurrency=3` 时，不会一次 claim 50 条进入长期 `delivering`。
- sink `max_concurrency=1` 时，同一 sink 同时最多一个请求。
- 两个不同 sink 可以在全局并发允许范围内并行。
- 进程在 delivery 处于 `queued` 时重启，消息不丢。
- 进程在 delivery 处于 `retry_wait` 时重启，消息不丢。
- 进程在 delivery 处于 `delivering` 时重启，`claim_expires_at` 到期后恢复并重试。
- timeout 小于 claim TTL 时，不会因为请求仍在执行而被提前 stale recovery。
- notification dispatcher 和 hook dispatcher 都满足上述行为。
- SQLite 和 PostgreSQL 都通过测试。
- `CGO_ENABLED=0 go test ./...` 通过。

---

## 15. 与现有规格的关系

本规格补充以下已有规格：

- `docs/superpowers/specs/2026-06-08-xuanchu-scheduled-notification-design.md`
- `docs/superpowers/specs/2026-06-08-xuanchu-scheduled-notification-rule-filter-extension-design.md`
- `docs/superpowers/specs/2026-06-09-xuanchu-event-notification-and-hook-events-design.md`

这些规格负责“什么时候生成 delivery”和“delivery 内容是什么”。本文负责“delivery 生成后如何可靠、可控地调用外部系统”。

---

## 16. 实施建议

建议按以下顺序落地：

1. 为 dispatcher options 增加 `MaxConcurrency` 和 `PrefetchFactor`，默认保持当前行为。
2. 增加 notification dispatcher 的 bounded worker pool。
3. 增加 hook dispatcher 的 bounded worker pool。
4. 调整 claim limit，确保受 available slot 限制。
5. 为 `notification_sinks` 增加 `max_concurrency`。
6. 在 dispatcher 中实现 sink 级并发 token。
7. 补充 config.toml、README、manual、MCP skill 文档。
8. 补齐 SQLite / PostgreSQL / CGO disabled 测试。

每一步都必须保持 delivery 不丢失、不破坏 retry/replay、不改变冻结请求快照。
