# xuanchu M8 设计规格

> **给 agentic workers 的要求：** 编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

**目标：** 为 `xuanchu server` 增加可审计、可控、可恢复的服务端 Hook / automation 能力，让内部事件发生后可以稳定触发外部 webhook，同时补齐与该能力直接相关的部署、发布与恢复文档。`xuanchu` 继续保持“企业任务运行时”定位，不演化成业务域 adapter 市场、memory 系统或多副本同步平台。

**范围策略：** M8 主线是 server-side post-commit webhook hook。M8 不做飞书 / GitHub / Jira / Slack adapter，不做 memory，不做 replica/sync，不做本地 CLI shell hook，也不做没有明确业务动机支撑的 scheduler 类能力。与 Hook 无直接关系的“大而全平台化”需求必须在本规格里显式排除。

**需求来源：** 本规格从 [README.md](/Users/mac/code/projects/dajee/task/README.md)、[ROADMAP.md](/Users/mac/code/projects/dajee/task/ROADMAP.md)、[M6 设计规格](/Users/mac/code/projects/dajee/task/docs/superpowers/specs/2026-05-31-xuanchu-m6-design.md)、[M7 设计规格](/Users/mac/code/projects/dajee/task/docs/superpowers/specs/2026-06-01-xuanchu-m7-design.md)、当前实现，以及本轮关于“Hook 应作为内部事件扩展边界，而非业务域 adapter 或 memory/sync 入口”的讨论收束。

---

## 1. 当前基础

M6 与 M7 已完成。当前项目已有：

- 单一 `xuanchu` 二进制。
- 本地 CLI、远程 CLI、HTTP/JSON API 服务端、MCP Server。
- `actor + workspace + project` 权限边界。
- PAT / Agent token、workspace scope、project scope。
- 统一 `internal/app` service。
- 审计、context、project config、MCP tool/resource。
- `xuanchu server` 作为唯一服务端入口。

M8 不重做这些能力。M8 的工作是：

- 把“内部状态变化”整理成稳定事件。
- 把“事件到外部 webhook 的投递”做成可靠、可恢复的服务端能力。
- 把“Hook 配置、投递状态、失败恢复”纳入 CLI / HTTP API / audit / 文档。

M8 不创建第二套业务执行入口，不创建第二套权限模型，也不让 Hook 直接绕过现有 `internal/app` 事务与审计边界。

## 2. 范围与非目标

### 2.1 M8 进入范围

M8 进入范围的能力只有四类：

- 服务端 Hook definition 与 webhook 投递。
- 稳定事件模型与 payload 契约。
- durable delivery queue、retry、dead-letter、manual replay。
- 与上述能力直接相关的部署、发布、backup / restore 文档与验收。

### 2.2 M8 明确不做

以下能力明确不进入 M8，也不以“后续可选范围”模糊保留：

- 飞书 / GitHub / Jira / Slack 等业务域 adapter。
- agent memory / workspace memory / personal memory 系统。
- replica / sync / operation log 同步。
- 本地 CLI shell hook。
- 通用工作流编排平台。
- 事务内外部回调强一致。
- 外部失败回滚本地 task/project 事务。

### 2.3 本轮评估后不进入 M8 首版的 trigger

本轮对 `schedule / heartbeat / one-shot` 做过评估，但当前没有足够明确、稳定的业务事件驱动场景，因此不进入 M8 首版：

- `heartbeat`：不做。
- `schedule`：不做。
- `one-shot`：不做。

后续如要引入，必须在新 milestone 或新的补充 spec 中先给出明确业务用例，再讨论调度模型、持久化语义和权限边界。M8 首版不为了“功能完整性”引入这些能力。

## 3. 核心产品决策

### 3.1 Hook 是扩展边界，不是 adapter 市场

M8 的核心不是“接多少第三方系统”，而是“内部事件能否稳定、安全地扩展出去”。

因此：

- `xuanchu` 负责产生标准事件。
- `xuanchu` 负责按权限边界投递 webhook。
- `xuanchu` 负责记录审计、失败状态和 replay。
- `xuanchu` 不负责承载第三方业务域语义。
- `xuanchu` 不内置飞书 / GitHub / Jira / Slack 业务逻辑。

第三方系统如果未来要接入，应该消费 M8 暴露的 Hook 或基于 HTTP API / MCP 自己实现，而不是把其业务域适配逻辑写进 `xuanchu` 核心。

### 3.2 M8 只做 server-side hook，不做本地 CLI hook

M8 首版只提供 `xuanchu server` 侧的 Hook runtime。

不做本地 CLI shell hook 的原因：

- 本地 CLI hook 更像开发者工具或兼容层，不是企业服务端自动化能力。
- 本地脚本执行环境、路径、shell、权限、stderr/stdout 语义与服务端 webhook 完全不同。
- 把两类能力硬塞进同一 milestone 会让配置模型和失败语义变形。

因此 M8 的 Hook 统一指：

- 存在于服务端数据库中的 definition。
- 由服务端后台 dispatcher 投递。
- 面向外部 HTTP webhook。

### 3.3 M8 只做 post-commit 异步投递

M8 首版只支持：

- 业务数据变更与 outbox delivery record 在同一个 SQLite transaction 中原子落盘。
- 如果 outbox 写入失败，整个本地业务 transaction 必须失败并回滚，避免“业务成功但事件丢失”。
- SQLite transaction 提交成功后，由异步 dispatcher 发起外部 HTTP 请求。

M8 明确不支持：

- pre-commit hook。
- 事务中同步调用外部 webhook。
- 外部 webhook 失败导致已经提交的 task/project 事务回滚。

本文档中 “post-commit” 只指外部 HTTP webhook 投递发生在本地 SQLite commit 之后，不指 outbox enqueue 与业务写入分离。M8 应该追求“本地事件不丢、外部投递可重试、可恢复”，而不是“跨网络分布式事务外观”。

### 3.4 M8 使用 durable outbox / delivery queue，但不演化为 sync log

为了避免“本地事务提交成功，但 webhook 还没发就丢失事件”，M8 必须使用持久化 delivery queue / outbox。

但这里的 outbox 不是：

- 全局 operation log。
- 多副本同步协议。
- 通用事件存储系统。

M8 的持久化记录只服务 Hook 投递本身：

- 事件 snapshot。
- 匹配到的 hook definition。
- 投递状态、次数、错误和 replay。

这套记录不承担 replica/sync 语义。

### 3.5 Hook 管理面走 CLI + HTTP API，不走 MCP 写操作

M8 首版 Hook 管理面必须支持：

- 本地 CLI。
- 远程 CLI。
- HTTP API。

M8 首版不要求通过 MCP 暴露 Hook 写操作，原因：

- Hook 配置与 replay 属于管理 / 运维面。
- Agent 的核心是消费任务能力，而不是自我改写系统自动化边界。
- 将 Hook 写能力直接暴露给 MCP 会显著提高误配置和权限外溢风险。

如未来需要 MCP 读 Hook 状态，可在后续 milestone 再评估。M8 首版不做。

### 3.6 M8 只开放少量稳定事件

M8 首版不做“全量实体事件总线”，只开放少量真正稳定、业务价值明确的事件：

- `task.created`
- `task.modified`
- `task.completed`
- `task.deleted`
- `project.archived`

说明：

- `task.modified` 负责覆盖普通修改、tag 变化、annotation、depends、start/stop、日期调整等修改类行为。
- `task.completed` 与 `task.deleted` 使用专用事件，不复用 `task.modified`。
- `task.deleted` 指 task 进入 deleted 状态的逻辑删除语义，不是物理删行。
- M8 首版不开放 `workspace.*`、`member.*`、`token.*`、`config.*` 等管理域事件，避免 payload 合约在第一版失控。

### 3.7 M8 同时收口与 Hook 直接相关的运维交付

M8 除了 Hook runtime，还应补齐与该能力直接相关的运维交付：

- 服务端部署说明。
- reverse proxy / TLS termination 建议。
- backup / restore 演练文档。
- CGO-free 发布产物与安装说明。

这些内容是 Hook 真正可交付所需的基础设施文档，不属于“偏题扩 scope”。

## 4. 事件模型

### 4.1 事件 envelope

每个事件都必须有稳定 envelope，至少包含：

- `event_id`
- `event_type`
- `event_version`
- `occurred_at`
- `actor_user_id`
- `workspace_id`
- `workspace_slug`
- 可选 `project_id`
- 可选 `project_slug`
- `object_kind`
- `object_id`
- `data`

规则：

- `event_id` 是幂等主键，由服务端生成，消费方必须据此去重。
- `event_version` 从 `1` 开始，为未来 payload 扩展预留。
- `data` 保存事件对应的稳定对象 snapshot，不直接泄露内部表结构。

### 4.2 M8 v1 payload 只发送 after snapshot

M8 v1 的 `data` 只发送提交后的当前 snapshot，不发送完整 `before/after diff`。

原因：

- after snapshot 已能覆盖大多数外部消费场景。
- diff 语义很容易放大 payload 和兼容性成本。
- delete 在 `xuanchu` 中是逻辑删除，after snapshot 仍可表达 deleted 状态。

如果未来需要 diff，应通过新的 `event_version` 显式扩展，而不是在 v1 中模糊混入。

### 4.3 task 事件 payload

task 事件 payload 至少包含：

- `task`：沿用当前稳定 JSON task 视图。
- `completed`
- `deleted`

规则：

- Hook payload 是对外契约。即使底层 CLI `task.ToJSON` 未来扩展字段，Hook payload 中字段的删除、重命名、类型变化都必须 bump `event_version`。
- M8 v1 可以复用当前稳定 JSON task 视图构造 payload，但必须用测试冻结字段集合，避免无意破坏外部 webhook 消费方。

不得包含：

- token 原文。
- secret 原文。
- 不属于当前 scope 的隐藏对象。

### 4.4 project 事件 payload

`project.archived` payload 至少包含：

- `project`：项目当前稳定视图。
- `archived`: `true`

M8 首版不开放 project config 全量内容，尤其不把敏感 config 值自动下发到 webhook。

## 5. Hook 与投递数据模型

### 5.1 Hook definition

M8 需要持久化 Hook definition。逻辑字段至少包含：

- `id`
- `name`
- `scope_type`：`workspace` 或 `project`
- `workspace_id`
- 可选 `project_id`
- `actor_user_id`
- `event_types`
- `endpoint_url`
- `secret`
- `enabled`
- `timeout_seconds`
- `max_attempts`
- `created_at`
- `modified_at`

规则：

- Hook 必须显式绑定 scope，不能存在“全局 hook”。
- project 级 Hook 必须属于其 workspace。
- `timeout_seconds` 必须有上限，M8 建议范围为 `1..120` 秒，避免单个 Hook 长时间占用 dispatcher worker。
- `max_attempts` 必须有上限，M8 建议范围为 `1..20`。
- `endpoint_url` 必须在创建和修改时做 URL 与网络安全校验；详见 §7.4。
- `secret` 是 write-only 敏感字段：
  - CLI / HTTP API 创建或更新时可以传入。
  - list / info 不回显原文。
  - audit 不记录原文。
  - server log 不输出原文。
- M8 首版为支持 HMAC 签名，需要在 SQLite 中保存可用于签名的 secret 值；“write-only” 仅表示 API / CLI / audit / log 不回显，不表示数据库 at-rest 不可读。
- Hook create / secret rotation 的 audit payload 不记录 secret 原文，但应记录不可逆短指纹，例如 `secret_fingerprint=sha256(secret)[:8]`，便于确认是否发生过轮换。
- 部署文档必须要求保护 SQLite 数据库文件和备份文件权限；如果未来需要 secret at-rest 加密，应进入独立安全增强 milestone。

### 5.2 Delivery record

每个命中事件与 Hook definition 的组合都应生成一条 delivery record。逻辑字段至少包含：

- `id`
- `hook_id`
- `event_id`
- `event_type`
- `workspace_id`
- 可选 `project_id`
- `actor_user_id`
- `payload_json`
- `headers_json`
- `status`
- `attempt_count`
- `next_attempt_at`
- `claim_expires_at`
- `last_attempt_at`
- `last_status_code`
- `last_error`
- `created_at`
- `modified_at`

状态至少包含：

- `queued`
- `delivering`
- `succeeded`
- `retry_wait`
- `dead_lettered`
- `disabled_skipped`

规则：

- `workspace_id`、`project_id`、`actor_user_id` 是 delivery 创建时的不可变 scope snapshot，用于 list / info / replay 的授权判断；不能只依赖当前 Hook definition 的状态。
- `claim_expires_at` 用于 dispatcher 崩溃恢复。超过该时间仍停留在 `delivering` 的 delivery 可以被重新置为 `queued` 或重新 claim。
- Hook 在 delivery 创建后被 disabled 时，尚未成功投递的 delivery 在 dispatcher 下次处理时必须被 lazy 标记为 `disabled_skipped`，不再自动投递。重新 enable Hook 不会自动恢复这些 skipped delivery；管理员如需补发，必须手动 replay。

### 5.3 Dead-letter 语义

M8 可以用独立表或独立状态承载 dead-letter，但语义必须明确：

- 进入 dead-letter 的 delivery 不再自动重试。
- 管理员可以查看 dead-letter 原因。
- 管理员可以手动 replay。
- replay 产生新的投递尝试记录，但保留原事件和失败历史。

## 6. 执行流水线

### 6.1 业务事务与事件落盘

当 `internal/app` 中的业务写操作成功时：

1. 在同一数据库事务中提交业务数据。
2. 根据事件类型和 scope 匹配可用 Hook definition。
3. 为每个命中的 Hook 写入 delivery record，并保存稳定 payload snapshot。
4. 提交事务。

M8 不允许“业务成功但 delivery record 没写入”的非原子落盘路径。

Hook 匹配规则固定为：

- workspace 级 Hook：`hook.workspace_id == event.workspace_id` 时命中，包括没有 project 归属的 task 事件。
- project 级 Hook：必须同时满足 `hook.workspace_id == event.workspace_id`、`event.project_id != nil`、`hook.project_id == event.project_id`。
- 不同 workspace 永不匹配。
- project 级 Hook 不接收没有 project 归属的事件。

### 6.2 Dispatcher

后台 dispatcher 负责：

- 轮询 `queued` / `retry_wait` 且已到期的 delivery。
- 抢占一条 delivery 的投递权。
- 构造 HTTP 请求并发送。
- 按结果更新状态。

规则：

- 同一 delivery 同一时刻只能被一个 worker 发送。
- 不要求全局严格顺序。
- 单 Hook 下可以按 delivery 创建顺序优先出队，但重试场景不保证外部观察到严格有序事件流。
- M8 是 at-least-once 投递语义；崩溃恢复、timeout 边界和 manual replay 都可能造成重复投递。
- 外部消费方必须使用 `event_id` 或 delivery header 自己处理重复和乱序。
- `claim_expires_at` 必须晚于当前 Hook 的 HTTP timeout，建议 `ClaimTTL = max(5m, timeout_seconds + 60s)`。
- dispatcher 启动或 claim 前必须处理过期 `delivering` 记录，避免进程崩溃后 delivery 永久卡死。

### 6.3 Replay

manual replay 的语义必须是：

- 基于原始保存的 `payload_json` 和 headers 再发一次。
- 不重新从当前数据库状态重建 payload。
- 签名在 replay 发送时使用 Hook 当前 secret 重新计算；M8 不保存历史 secret snapshot。

这样可以保证：

- replay 的事件 payload 与基础 headers 和最初事件一致。
- 不会因为对象后续变化而扭曲历史事件。
- replay 的签名、timestamp、attempt header 属于本次 HTTP 发送上下文，可以不同于首次发送。

## 7. Webhook 协议

### 7.1 请求格式

M8 首版 webhook 使用：

- `POST`
- `Content-Type: application/json; charset=utf-8`

稳定 header：

- `X-Xuanchu-Event`
- `X-Xuanchu-Event-Id`
- `X-Xuanchu-Event-Version`
- `X-Xuanchu-Delivery`
- `X-Xuanchu-Hook-Id`
- `X-Xuanchu-Attempt`
- `X-Xuanchu-Timestamp`
- `X-Xuanchu-Signature-256`
- `User-Agent: xuanchu-webhook/<version>`

`X-Xuanchu-Signature-256` 使用 HMAC-SHA256。签名输入为：

```text
<delivery_id>.<timestamp_unix_seconds>.<body>
```

签名格式固定为：

```text
sha256=<hex>
```

### 7.2 签名与 secret

规则：

- 有 `secret` 时必须签名。
- 无 `secret` 时不发送签名 header。
- 有 `secret` 时必须发送 `X-Xuanchu-Timestamp`，消费方应拒绝超过 5 分钟窗口的请求。
- 签名算法和 header 名称必须在文档中固定。

M8 首版不做：

- mTLS。
- OAuth webhook 回调。
- 第三方平台专用签名协议。

### 7.3 超时与重试

M8 必须定义稳定的失败分类：

- 网络错误：重试。
- timeout：重试。
- HTTP `3xx`：不自动跟随 redirect，默认进入 dead-letter。
- HTTP `5xx`：重试。
- HTTP `429`：重试。
- HTTP 其他 `4xx`：默认不重试，直接进入 dead-letter。

重试策略至少要有：

- 最大尝试次数。
- 带 jitter 的退避策略。
- 对 `429` / `503` 的 `Retry-After` header 处理。
- 下一次尝试时间。

具体退避参数可留给 implementation plan，但语义必须在 M8 中固定。

### 7.4 Endpoint SSRF 防护

M8 首版必须默认启用 outbound webhook SSRF 防护：

- `endpoint_url` 只允许 `http://` 和 `https://`。
- 禁止 loopback、link-local、private RFC1918、carrier-grade NAT RFC6598、multicast、unspecified 地址。
- 创建 / 修改 Hook 时必须解析 host 并拒绝上述地址。
- dispatcher 真正发送前必须再次解析 host，防止 DNS rebinding。
- HTTP client 必须禁用自动 redirect；`3xx` 不跟随，按 §7.3 分类处理。
- 自托管用户如确需投递到内网，应通过后续明确配置项放开；M8 默认不提供“任意内网可投递”的宽松模式。

## 8. 权限、作用域与审计

### 8.1 Hook 绑定身份

Hook definition 必须绑定：

- `actor_user_id`
- `workspace_id`
- 可选 `project_id`

含义：

- 该 Hook 代表哪个 actor 在哪个 scope 上订阅事件。
- 该 Hook 只能接收其 scope 内事件。
- 该 Hook 不是系统级超级旁路。

### 8.2 事件可见性

命中规则必须遵守现有边界：

- workspace 级 Hook 只能看该 workspace 内事件。
- project 级 Hook 只能看该 project 内事件。
- 不得因为 Hook 位于服务端内部，就自动看到跨 workspace 或跨 project 数据。

### 8.3 管理权限

M8 Hook 管理至少要求：

- workspace 级 Hook：需要具备该 workspace 的管理写权限。
- project 级 Hook：需要具备该 project 所属 workspace 的管理写权限。

具体 permission 名称可在 implementation plan 中收敛，但 M8 必须明确 Hook 不属于普通 task 写权限可随意修改的对象。

### 8.4 Audit 范围

以下行为必须写 audit：

- create hook
- modify hook
- enable hook
- disable hook
- delete hook
- manual replay

以下行为不写业务 audit，而进入专用 delivery 运行记录：

- 每一次自动投递尝试
- 每一次自动重试
- 每一次远端 HTTP 返回码

否则 audit 会迅速膨胀成低价值噪音。

## 9. CLI 与 HTTP API 面

### 9.1 CLI

M8 增加 `hook` 命令组，至少覆盖：

- `hook list`
- `hook add`
- `hook info`
- `hook modify`
- `hook enable`
- `hook disable`
- `hook delete`
- `hook deliveries`
- `hook replay`

规则：

- 本地 CLI 与远程 CLI 复用同一语义。
- 远程 CLI 必须通过 HTTP API，不得直接触碰客户端本地数据库。
- JSON 输出结构必须稳定。

### 9.2 HTTP API

M8 增加 Hook 管理与状态查询 endpoint，至少覆盖：

- Hook definition 的增删改查。
- delivery list / info。
- replay。

规则：

- endpoint 命名走现有 `/api/v1/*` 风格。
- OpenAPI 必须同步更新。
- 远程 CLI 与 HTTP API 不应出现语义分叉。
- delivery list 必须支持按 `status` 过滤，至少可筛选 `dead_lettered`。

### 9.3 MCP

M8 首版不要求新增 MCP Hook 管理 tool。

原因：

- Hook 是管理面对象。
- 需要更严格的误操作控制。
- 当前产品主线是让 Agent 使用任务运行时，而不是让 Agent 重写系统自动化边界。

## 10. 部署、发布与恢复

### 10.1 部署文档

M8 文档必须覆盖：

- `xuanchu server` 基础启动方式。
- 反向代理 / TLS termination 建议。
- Hook outbound 网络需求。
- secret 配置与最小权限建议。
- SQLite 数据库文件和备份文件权限建议，尤其是 Hook secret 会随数据库和备份保存。

### 10.2 Backup / restore

M8 文档必须覆盖：

- SQLite `VACUUM INTO` 备份建议。
- JSON export 的适用范围与局限。
- 恢复演练步骤。
- Hook definition 与 delivery record 是否进入备份的明确说明。

M8 不要求做复杂在线快照编排器，但必须把“如何恢复含 Hook 的服务端”写清楚。

### 10.3 发布

M8 继续要求：

- linux/amd64
- linux/arm64
- darwin/amd64
- darwin/arm64
- windows/amd64

全部保持 CGO-free。

## 11. 设计边界与不做项

M8 必须明确拒绝以下方向，避免实现中被“顺手加一点”拖偏：

- 不做 adapter SDK。
- 不做 memory store。
- 不做 sync / replica / global op-log。
- 不做 scheduler 平台。
- 不做本地 shell script hook。
- 不做 hook 里直接执行任意本机命令。
- 不做 pre-commit / rollback hook。
- 不做“hook 失败则业务失败”的分布式事务幻觉。
- 不做双 secret 轮换窗口、delivery stats endpoint、delivery retention GC、per-hook 公平调度等运维增强；这些可以作为后续安全 / 运维增强单独规划。
- 不扩展 project lifecycle 全量事件。M8 只保留 `project.archived`，因为归档会影响 project-scoped task 自动化边界。

## 12. 验收标准

M8 的验收标准如下：

- 管理员可以配置 workspace 级 webhook hook。
- 管理员可以配置 project 级 webhook hook。
- 命中 `task.created`、`task.modified`、`task.completed`、`task.deleted`、`project.archived` 时可以收到稳定 JSON payload。
- project-scoped hook 不会收到 scope 外事件。
- Hook 失败不会回滚已提交的 task/project 事务。
- timeout、retry、dead-letter、disable、manual replay 都有端到端测试。
- Hook 配置变更和人工 replay 都有 audit。
- README / 部署文档足以让管理员完成：
  - 服务端部署
  - token 配置
  - hook 配置
  - 基础 backup / restore 演练
- 发布产物继续通过：

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```

## 13. 对 implementation plan 的约束

M8 implementation plan 至少要显式拆出以下阶段：

1. 数据模型与 migration。
2. 事件生成与 outbox 落盘。
3. Dispatcher 与 retry / dead-letter。
4. CLI + HTTP API 管理面。
5. audit 与 delivery 观测。
6. 文档与发布收口。

每个阶段都必须包含：

- 红测到绿测的 TDD 步骤。
- 与该阶段直接相关的局部验证命令。

整个 milestone 的最终验收必须包含端到端测试、`CGO_ENABLED=0 go test ./...` 与 `CGO_ENABLED=0 go build ./cmd/xuanchu`。不要求每个小阶段都重复跑完整 CGO-free 全量验证，但阶段合入前不能跳过局部测试。

M8 不允许在没有 durable queue、没有 replay、没有权限测试的情况下宣称 Hook 能力完成。
