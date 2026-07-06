# Web Console 出站集成控制台设计

> **给 agentic workers 的要求：** 编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

**日期：** 2026-07-05
**状态：** 设计中
**范围：** Web Console 的 `/hooks` 与出站集成相关页面；必要的 HTTP API 小扩展；不改变现有 delivery 表、sink 表和 hook 事件语义。
**相关背景：** v0.3.0 已完成 Hook sink 化、事件通知、dispatcher 并发背压；v0.5.2 已把 `/hooks` 从只读表升级为基础 CRUD 控制台。

## 0. 产品结论

当前 `/hooks` 页面已经能做 Hook 的基础运维，但它仍然不是一个人类可以独立完成配置的集成控制台。真正的配置链路不是“创建一个 Hook”，而是：

```text
确认目标系统 -> 创建或选择 outbound sink -> 选择触发事件 -> 限定 workspace/project 范围
-> 保存 Hook 或通知规则 -> 发送测试投递 -> 观察 delivery -> 修复错误 -> replay
```

现在的 Web Console 把这条链路切断了：

1. `/hooks` 只管理 Hook，不能创建或编辑 sink。
2. sink 承载 URL、secret、HTTP template、allowed hosts、重试、超时、并发等关键配置，但 `/notifications` 目前只是只读 sink 表。
3. 新建 Hook 需要手填 sink 名称和逗号事件，project scope 缺 project_ref 输入。
4. delivery 排障信息只展示很少字段，API 已有 payload、headers、attempt、next_attempt、错误等细节没有被 UI 使用。
5. 没有“发送测试”能力，用户只能靠真的改任务触发事件来验证配置。

因此，本规格把 `/hooks` 重新定义为 **出站集成控制台**。页面仍可以保留 `/hooks` 路由和“集成 / Webhook”入口，但产品心智应从单一 Hook 表升级为“配置和运维外部投递链路”。

## 1. 当前代码与能力边界

### 1.1 已有后端能力

Hook：

- `GET /api/v1/hooks`
- `POST /api/v1/hooks`
- `GET /api/v1/hooks/{hookID}`
- `PATCH /api/v1/hooks/{hookID}`
- `DELETE /api/v1/hooks/{hookID}`
- `POST /api/v1/hooks/{hookID}/enable`
- `POST /api/v1/hooks/{hookID}/disable`
- `GET /api/v1/hooks/{hookID}/deliveries`
- `GET /api/v1/hook-deliveries/{deliveryID}`
- `POST /api/v1/hook-deliveries/{deliveryID}/replay`

Notification sink：

- `GET /api/v1/notification-sinks`
- `POST /api/v1/notification-sinks`
- `GET /api/v1/notification-sinks/{sinkID}`
- `PATCH /api/v1/notification-sinks/{sinkID}`
- `DELETE /api/v1/notification-sinks/{sinkID}`
- `POST /api/v1/notification-sinks/{sinkID}/enable`
- `POST /api/v1/notification-sinks/{sinkID}/disable`

Reminder / event notification rule：

- `GET/POST/PATCH/DELETE /api/v1/reminder-rules`
- `POST /api/v1/reminder-rules/{ruleID}/enable|disable`
- `GET/POST/PATCH/DELETE /api/v1/notification-rules`
- `POST /api/v1/notification-rules/{ruleID}/enable|disable`

Notification delivery：

- `GET /api/v1/notification-deliveries`
- `GET /api/v1/notification-deliveries/{deliveryID}`
- `POST /api/v1/notification-deliveries/{deliveryID}/replay`

这些能力都已经走现有 `internal/app`、authz、workspace 隔离、CSRF 和 audit 边界。本规格优先复用，不重写一套前端专用接口。

### 1.2 已有前端能力

当前 `/hooks` 具备：

- Hook 列表。
- 新建 Hook。
- enable / disable。
- delete。
- 行内展开最近 delivery。
- replay 失败 delivery。

当前 `/notifications` 只展示 notification sink 的只读表，不支持 sink 创建、编辑、启停、删除、投递历史或测试。

### 1.3 已知缺口

- `HookCreateDialog` 的 event placeholder 使用了不存在的 `task.done`，应改为 `task.completed`。
- Hook project scope 缺少 project selector / project_ref 输入。
- Hook sink 选择应使用当前 workspace 的 sink 列表，不应要求用户记名称。
- Hook 的 `timeout_seconds`、`max_attempts` 已有 API 字段，但 UI 没有配置。
- Hook modify API 已有，UI 没有编辑入口。
- 权限上，资源分发当前直接把 HookConsole 当作可写页面。UI 应按当前 token scopes 和 effective role 决定写按钮是否可用，而不是让用户点击后才得到 403。
- Delivery 详情缺失 payload / headers / actor / project / retry 时间等排障信息。
- 缺少 sink 测试投递 API。MCP 的 `hook_ping` 只是读取 Hook 信息和写 audit，不是真实 HTTP 投递测试，不能解决集成验证问题。

## 2. 目标

首版完成后，workspace owner/admin 或具备相关 scope 的系统身份应能在浏览器内完成：

1. 创建、编辑、启停、删除 outbound sink。
2. 创建标准 webhook sink：URL、secret、timeout、max attempts、max concurrency。
3. 创建 HTTP template sink：endpoint mode、allowed hosts、header templates、body template、secret refs。
4. 创建、编辑、启停、删除 Hook。
5. 创建 project-scoped Hook，并正确选择 project。
6. 从当前 workspace sink 列表中选择 Hook 目标 sink。
7. 用分组 checkbox 选择允许的事件类型，避免拼写错误。
8. 查看 Hook delivery 和 notification delivery 的关键状态。
9. 查看单条 delivery 详情：payload、headers、attempt、last_error、next_attempt_at、actor、project、冻结请求摘要。
10. 对 dead-letter / failed / retryable delivery 手动 replay。
11. 对 sink 发送测试投递，并看到 HTTP 状态、错误和请求摘要。
12. 明确知道当前身份为什么不能写：缺 role、缺 scope、project allowlist 不匹配或 closed project 限制。

第二阶段及以后还应支持：

1. 在同一个控制台配置 reminder rule 和 event notification rule。
2. 把“面向机器的 Hook”和“面向人的通知规则”放在同一张出站集成图谱里展示。
3. 支持预设模板，例如标准 webhook、飞书机器人、企业微信机器人、Slack webhook、自建 OpenClaw 网关。
4. 支持基于样例事件预览 HTTP template 渲染结果。
5. 支持 delivery 搜索、过滤、分页、导出和失败聚合。

## 3. 非目标

- 不内置飞书、Slack、Jira、OpenClaw 等业务 adapter。预设模板只帮用户填写 HTTP sink 字段，运行时仍是通用 sink。
- 不执行任意脚本、JS 或 shell。
- 不改变现有 hook delivery / notification delivery 的 at-least-once 语义。
- 不承诺 exactly-once。外部系统仍应使用 delivery_id 幂等。
- 不把 Hook 和 Notification Rule 混成同一个数据库模型。它们共享 sink 和投递运维心智，但业务语义不同。
- 不改变现有 event 白名单。新增事件类型要走单独事件规格。
- 不让 Web Console 绕过现有 authz、token scope、workspace allowlist、project allowlist 或 CSRF。
- 不把 secret 明文回显给浏览器。secret 只在写入时提交；读取时展示“已配置/未配置”和指纹，不展示原文。

## 4. 产品概念

### 4.1 Outbound Sink

Outbound sink 是外部投递出口。现有 API/DB 名称仍是 `notification sink`，但产品文案应逐步解释为“出站目标 / Sink”，避免用户误解它只能用于个人通知。

Sink 表达：

- 投递到哪里：`endpoint_mode`、`url`、`url_template`、`config_key`。
- 如何限制动态 URL：`allowed_hosts`。
- 如何发 HTTP 请求：method 固定 POST，headers、body、content type。
- 如何携带密钥：标准 webhook secret 或 `secret_refs`。
- 如何重试与并发：`timeout_seconds`、`max_attempts`、`max_concurrency`。
- 是否启用：`enabled`。

### 4.2 Hook

Hook 表示“把原始事件 envelope 推给外部系统”。它引用 sink，不直接保存 URL 或 secret。

Hook 表达：

- 监听哪些事件。
- 监听 workspace 还是某个 project。
- 用哪个 sink 投递。
- 是否启用。
- 单 Hook 的 timeout / max attempts 覆盖。

### 4.3 Notification Rule

Notification rule 表示“基于事件给人发通知”。它也引用 sink，但它不是原始事件同步，而是面向 recipient 的通知生成。

本规格要求页面信息架构给 notification rule 留位置，但第一阶段不必须实现完整 rule 编辑器。

### 4.4 Reminder Rule

Reminder rule 表示“基于时间扫描任务并给人发通知”。它同样引用 sink。它与 Hook 不共享触发语义，但共享 delivery 排障、replay、sink 测试和权限体验。

### 4.5 Delivery

Delivery 是实际投递记录，是排障和审计的核心。用户排查集成时通常不是先看定义，而是先问：

- 刚才有没有生成 delivery？
- 投递到哪个 sink？
- 请求冻结后长什么样？
- 收到了什么 HTTP status？
- 为什么失败？
- 下次什么时候重试？
- 能否手动 replay？

因此 delivery 必须成为出站集成控制台的一等对象。

## 5. 信息架构

推荐保留 `/hooks` 路由，但页面标题改为“集成 / 出站投递”或“集成 / Webhook”。内部使用 tab：

```text
集成 / 出站投递
═══════════════════════════════════════════════════════════
[概览] [Sinks] [Hooks] [通知规则] [定时规则] [投递记录] [模板]
```

### 5.1 概览

概览用于回答“当前出站集成健康吗”：

```text
出站集成概览
───────────────────────────────────────────────────────────
Sinks            4 个启用 / 1 个暂停
Hooks            8 个启用 / 2 个暂停
通知规则          3 个启用
定时规则          5 个启用
最近失败投递       7 条
Dead letter       2 条

最近失败
时间              类型        sink          status   错误
14:02             hook        feishu-bot    500      bad token
13:58             notification openclaw     timeout  context deadline exceeded
```

第一阶段可以先用并行查询拼装，不新增 summary endpoint。若多请求造成明显抖动，再新增后端 summary endpoint。

### 5.2 Sinks

Sinks tab 是完整配置入口，不再只读：

```text
Sinks                                             [+ 新建 Sink]
───────────────────────────────────────────────────────────
名称              类型            endpoint              状态    最近失败
feishu-bot        http_template   config_value           启用    1
audit-stream      webhook         https://audit...        启用    0
openclaw-prod     webhook         config_value            暂停    -
```

行操作：

- 查看详情。
- 编辑。
- 启用 / 暂停。
- 发送测试。
- 删除。

详情区域展示：

- endpoint mode。
- URL 或 config key。
- allowed hosts。
- header templates。
- body template 摘要。
- secret refs。
- timeout / max attempts / max concurrency。
- 最近 notification deliveries。
- 最近 hook deliveries 中引用该 sink 的失败聚合。若后端暂时没有按 sink 查询 hook deliveries，可以先只展示 notification deliveries，并在 Hook tab 下展示 hook deliveries。

### 5.3 Hooks

Hooks tab 聚焦机器到机器事件同步：

```text
Hooks                                             [+ 新建 Hook]
───────────────────────────────────────────────────────────
名称              范围       事件              sink          状态  最近投递
task-audit        workspace  8 events          audit-stream  启用  成功 2m
project-ci        project    project.archived  ci-webhook    启用  失败 1h
```

行展开展示：

- Hook 定义详情。
- 编辑 / 启用 / 暂停 / 删除。
- 最近投递。
- replay。
- 跳转到关联 sink。

### 5.4 通知规则

Notification Rules tab 是事件通知规则。第一阶段可以先只读或隐藏在后续阶段，完整能力应包括：

- 创建 / 编辑 / 启停 / 删除。
- event type 选择。
- audience type 选择：`assignees`、`explicit_users`、`assignees_and_explicit_users`，以及后端已支持或未来支持的 audience。
- recipients 选择用户。
- filter_source 输入和校验。
- sink 选择。
- template_subject / template_body。
- 最近 notification deliveries。

### 5.5 定时规则

Reminder Rules tab 是时间扫描提醒规则。完整能力应包括：

- 创建 / 编辑 / 启停 / 删除。
- project scope 选择。
- 新模型优先：`schedule_type` + `schedule_value` + `filter_source`。
- 兼容旧模型：`trigger_type`、offset、after、repeat_policy。
- audience / recipients / sink。
- 最近 notification deliveries。

### 5.6 投递记录

Deliveries tab 是跨 Hook 和 Notification 的排障中心：

```text
投递记录
───────────────────────────────────────────────────────────
[类型: 全部▾] [sink▾] [status▾] [event▾] [limit▾] [刷新]

时间              类型           event              sink          status   操作
14:02             hook           task.completed     audit-stream  200      查看
13:58             notification   task.unblocked     openclaw      timeout  查看/重放
```

第一阶段后端没有统一 delivery endpoint 时，前端可以：

- Hook delivery：在 Hook 行内或 Hook tab 内按 hook 查询。
- Notification delivery：使用 `/api/v1/notification-deliveries?sink=&status=&limit=`。

完整阶段可新增聚合 endpoint：

```text
GET /api/v1/outbound-deliveries?kind=hook|notification&sink=&status=&event_type=&limit=&offset=
```

该 endpoint 只读聚合，不改变底层表。

## 6. 表单设计

### 6.1 新建 / 编辑 Sink

表单分三层：基础、Endpoint、高级。

基础：

- name。
- type：`webhook` / `http_template`。
- enabled 创建后默认 true，不在创建表单里做开关。

Endpoint：

- endpoint_mode：`static_url` / `template` / `config_value`。
- `static_url`：显示 URL 输入。
- `template`：显示 URL template + allowed hosts。
- `config_value`：显示 config key + allowed hosts。

Webhook 类型：

- secret 输入。编辑时不回显，只展示“已配置 secret”；用户填写新值才覆盖。
- timeout_seconds。
- max_attempts。
- max_concurrency。

HTTP template 类型：

- header_templates：可增删的 name/value 列表。
- body_content_type。
- body_template，大文本编辑区。
- secret_refs：alias + config_key 列表。
- allowed_hosts 对动态 endpoint 必填。
- timeout_seconds。
- max_attempts。
- max_concurrency。

校验：

- name 必填。
- static_url 必须是 URL。
- dynamic endpoint 必须至少一个 allowed host。
- max_concurrency 非负。
- timeout 1..120。
- max_attempts 1..20。
- body_content_type 是 `application/json` 时，前端可做基本 JSON 模板括号校验；后端仍是最终裁决。

### 6.2 新建 / 编辑 Hook

字段：

- name。
- scope_type：workspace / project。
- project_ref：scope_type 为 project 时必填，通过项目选择器选择 slug。
- sink：下拉选择当前 workspace sink，显示 name、type、enabled、endpoint 摘要。
- event_types：分组 checkbox。
- timeout_seconds。
- max_attempts。

事件分组：

```text
任务基础
- task.created
- task.modified
- task.completed
- task.deleted

任务状态
- task.started
- task.stopped
- task.blocked
- task.unblocked

字段变化
- task.assigned
- task.unassigned
- task.due_changed
- task.priority_changed
- task.project_changed
- task.tags_changed

项目
- project.archived
- project.transitioned
- project.annotated
- project.denotated
```

要求：

- 事件清单必须来自前端常量，和后端当前白名单对齐。
- placeholder 和文案不得使用 `task.done`，统一使用 `task.completed`。
- 支持“全选任务事件”“全选项目事件”，但不要默认全选，避免误发。
- 如果选择了 disabled sink，允许保存但显示风险提示；也可以要求先启用，实施计划二选一明确。

### 6.3 通知规则 / 定时规则表单

完整版本应实现，但可以落到后续阶段。

事件通知规则字段：

- name。
- project_ref 可选。
- event_type。
- filter_source。
- audience_type。
- recipients。
- sink。
- template_subject。
- template_body。

定时规则字段：

- name。
- project_ref 可选。
- schedule_type。
- schedule_value。
- filter_source。
- trigger_type / offset / after / repeat_policy 兼容字段。
- audience_type。
- recipients。
- sink_ref。

## 7. 测试投递设计

### 7.1 为什么需要新后端 API

真实用户配置 sink/hook 时，最核心的问题是“这个目标能不能收到 Xuanchu 发出的请求”。现有能力无法优雅验证：

- `hook_ping` 只是读 Hook 信息，不执行 HTTP 投递。
- 创建 Hook 后必须制造真实任务事件，才能触发 delivery。
- 失败时用户很难区分是 URL、secret、模板、allowed host、网络防护还是目标系统问题。

因此需要新增测试投递 API。

### 7.2 Sink 测试 API

建议新增：

```text
POST /api/v1/notification-sinks/{sinkID}/test
```

请求：

```json
{
  "kind": "hook",
  "event_type": "task.completed",
  "sample": "default",
  "project_ref": "agentapi"
}
```

字段语义：

- `kind`：`hook` 或 `notification`。决定使用哪类 envelope 样例。
- `event_type`：用于样例 payload 和 headers。
- `sample`：首版只支持 `default`。
- `project_ref`：可选，用于测试 `config_value` endpoint 在 project config 下的解析。

响应：

```json
{
  "status": "succeeded",
  "status_code": 200,
  "duration_ms": 132,
  "resolved_endpoint_source": "static_url",
  "resolved_endpoint_fingerprint": "sha256:...",
  "rendered_method": "POST",
  "rendered_headers": {
    "Content-Type": ["application/json"],
    "X-Xuanchu-Event": ["task.completed"]
  },
  "rendered_body_preview": "{\"event_type\":\"task.completed\",...}",
  "error": ""
}
```

失败响应仍使用成功 HTTP envelope 或 4xx/5xx 需在 implementation plan 中确定。推荐：

- 配置无效、权限不足、sink 不存在：正常 API 错误。
- 已发出请求但目标返回 4xx/5xx：HTTP 200 + `status: "failed"`，因为 API 调用本身成功，测试结果失败。

安全与审计：

- 需要 `notification:write` 和 `PermissionNotificationWrite`。
- 不绕过 SSRF/network guard。
- 不落入正式 delivery 表，避免污染业务投递队列。
- 必须写 audit：`notification.sink.test`，payload 不包含 secret 或完整 body，只包含 sink_id、kind、event_type、status、status_code、endpoint fingerprint。
- 不回显 secret。
- 测试请求 header 必须带明显标记，例如 `X-Xuanchu-Test: true`。
- 测试请求必须有独立 delivery/test id，便于接收方幂等和排查。

### 7.3 Hook 测试入口

UI 上可以提供“测试 Hook”，但实现上调用 sink test：

1. 读取 Hook。
2. 取 Hook 的 sink_id 和第一个或用户选择的 event_type。
3. 调用 `POST /notification-sinks/{sinkID}/test`。

后续可新增：

```text
POST /api/v1/hooks/{hookID}/test
```

它可以更完整地模拟 Hook envelope，但不是第一阶段必需。

### 7.4 模板预览 API

后续阶段建议新增只渲染不发送：

```text
POST /api/v1/notification-sinks/{sinkID}/preview
```

用途：

- 预览 http_template 的 rendered URL、headers、body。
- 检查 secret refs 是否存在，但不回显 secret 值。
- 检查 JSON body template 是否有效。
- 帮用户在发送前修模板。

## 8. Delivery 详情与排障

### 8.1 Hook delivery 详情

详情弹窗展示：

- 基础：id、hook_id、event_id、event_type、workspace_id、project_id。
- actor：使用 `task.UserInfo` / `task.ActorInfo` JSON 形态展示，不退化成裸 UUID。
- status：queued / delivering / retry_wait / succeeded / dead_lettered / disabled_skipped。
- attempt_count、next_attempt_at、claim_expires_at、last_attempt_at。
- last_status_code、last_error。
- headers。
- payload JSON。

### 8.2 Notification delivery 详情

详情弹窗展示：

- id、rule_id、sink_id。
- recipient。
- event_id、event_type。
- object_kind、object_id、task_uuid。
- resolved endpoint source / fingerprint。
- rendered method / headers / body / content type。
- payload。
- status、attempt、retry、错误。

### 8.3 replay 规则

UI 只在可 replay 的状态显示 replay：

- `dead_lettered`。
- 明确后端允许 replay 的失败状态。

对于 `succeeded`，默认不显示 replay；如果后续要支持强制 replay，必须二次确认并写 audit。

## 9. 权限与可见性

### 9.1 前端能力判断

前端应从 `/api/v1/credentials/current` 的 `effective_role`、`token.scopes` 和 `capabilities` 计算可写性：

- Hook 读：`hook:read` 或 `*`，且角色允许。
- Hook 写：`hook:write` 或 `*`，且 owner/admin 或后端 PermissionHookWrite 允许的系统身份。
- Sink / notification 写：`notification:write` 或 `*`。
- Reminder rule 写：`reminder:write` 或 `*`。

UI 判断只用于减少误点；服务端仍是最终裁决。不要因为前端判断就省略错误处理。

### 9.2 只读模式

只读用户仍应能：

- 查看自己有权限读取的 hooks、sinks、rules、deliveries。
- 查看失败原因。
- 复制 delivery id / payload。

只读用户不能：

- 创建 / 编辑 / 删除。
- enable / disable。
- replay。
- test sink。

### 9.3 Project allowlist

如果 token 有 project allowlist：

- 不能创建 workspace-scoped hook。
- project-scoped hook 只能选择 allowlist 内项目。
- sink 仍是 workspace 级资源，但是否可引用由后端 `resolveNotificationSink` 和 request scope 控制。
- 前端项目选择器应只显示可见项目；服务端仍兜底。

## 10. 前端组件边界

建议新增或重组：

```text
web/src/features/workspace/outbound/
  outbound-console.tsx
  outbound-permissions.ts
  outbound-api.ts
  sinks/
    sink-list.tsx
    sink-form-dialog.tsx
    sink-detail-dialog.tsx
    sink-test-dialog.tsx
  hooks/
    hook-list.tsx
    hook-form-dialog.tsx
    hook-delivery-table.tsx
    hook-delivery-detail-dialog.tsx
  deliveries/
    delivery-status-badge.tsx
    delivery-json-viewer.tsx
  rules/
    notification-rule-list.tsx
    reminder-rule-list.tsx
```

迁移策略：

- 第一阶段可以保留现有 `web/src/features/workspace/hooks/*`，但新组件命名应反映 outbound/integration 概念。
- 若文件增长过大，应拆成 `sinks/`、`hooks/`、`deliveries/` 子目录，避免一个 `hook-console.tsx` 承载全部逻辑。
- `NotificationConsole` 目前是只读 sink 表，后续应并入 outbound console，或让 `/notifications` 重定向到 `/hooks?tab=rules`。不要维护两个割裂的 sink UI。

## 11. API 扩展

### 11.1 必做

```text
POST /api/v1/notification-sinks/{sinkID}/test
```

原因：没有真实测试投递，用户无法完成配置闭环。

### 11.2 可选

```text
POST /api/v1/notification-sinks/{sinkID}/preview
POST /api/v1/hooks/{hookID}/test
GET  /api/v1/outbound-deliveries
GET  /api/v1/outbound-summary
GET  /api/v1/hook-event-types
```

说明：

- `hook-event-types` 可避免前端硬编码事件白名单，但第一阶段可以硬编码并用测试守住。
- `outbound-deliveries` 和 `outbound-summary` 是性能和体验优化，不是配置闭环前置条件。

## 12. 分阶段落地

### Phase 0：文档与当前 UI 修正

- 修正 `task.done` 文案为 `task.completed`。
- 在现有 `/hooks` 页面增加说明：Hook 引用 sink，URL/secret 在 Sink 中配置。
- 新建 spec 和 implementation plan。

验收：

- 文案不再引导用户输入不存在事件。
- spec 明确完整目标和阶段。

### Phase 1：不改后端，补完整 sink + hook 配置

- `/hooks` 页面改成出站集成控制台，至少包含 Sinks 和 Hooks 两个 tab。
- Sinks 支持 list/create/edit/enable/disable/delete。
- Hooks 支持 list/create/edit/enable/disable/delete。
- Hook 表单使用 sink 下拉、event checkbox、project selector。
- Hook delivery 详情弹窗使用现有 API。
- 权限按 scopes/role 控制按钮显隐和 disabled 状态。

验收：

- 用户能从空 workspace 创建 sink，再创建 Hook 引用该 sink。
- project scope Hook 能成功提交 project_ref。
- 不需要 CLI 即可完成基础 Hook 配置。
- 不展示 secret 明文。

### Phase 2：新增测试投递能力

- 后端新增 sink test API。
- 前端增加“发送测试”入口。
- 测试结果展示 status code、错误、endpoint fingerprint、headers/body preview。
- 写 audit。

验收：

- 用户不需要制造真实任务事件也能验证 sink 可达。
- SSRF 防护、allowed hosts、secret refs、template 错误能以可读形式反馈。

### Phase 3：通知规则和定时规则进入同一控制台

- Notification Rules tab 支持 CRUD/enable/disable。
- Reminder Rules tab 支持 CRUD/enable/disable。
- Rules 与 sink/delivery 建立跳转。

验收：

- 用户能配置“事件发生时通知人”和“定时扫描提醒人”两类规则。
- Hook 与 Notification Rule 的区别在 UI 文案中清晰。

### Phase 4：统一投递排障中心

- 增加聚合 delivery 视图或后端 `outbound-deliveries`。
- 支持 kind/sink/status/event/limit 过滤。
- 支持失败聚合、导出、复制 curl 或复制请求摘要。

验收：

- owner/admin 能在一个页面查清楚所有出站失败。
- replay 操作有明确二次确认和审计。

### Phase 5：模板与预设

- 增加 sink preset：标准 webhook、飞书机器人、企业微信机器人、Slack webhook、自建 HTTP API。
- 增加 preview API 或前端本地预览。
- 增加模板变量说明面板。

验收：

- 非工程用户也能基于模板完成常见机器人 webhook 配置。
- 高级用户仍能切换到 raw HTTP template。

## 13. 测试策略

### 13.1 前端单测

- permissions：scope 和 role 组合。
- sink form：不同 endpoint_mode 的字段显隐与 payload。
- hook form：event checkbox、project scope、sink select payload。
- delivery table：状态 badge、replay 按钮可见性。
- i18n：事件名和文案不出现 `task.done`。

### 13.2 前端集成测试

- 从空列表创建 sink。
- 创建 Hook 引用 sink。
- 编辑 Hook 的事件和 sink。
- 展开 Hook delivery，查看详情，失败项 replay。
- 只读用户看不到写按钮。

### 13.3 后端测试

若实现 sink test API：

- 权限：notification:write 必需。
- workspace 隔离：不能测试其他 workspace sink。
- SSRF 防护：loopback/private/link-local 被拒。
- static_url 正常投递。
- config_value + allowed_hosts 正常解析。
- http_template body/header 渲染。
- secret 不出现在 response、audit、日志。
- 目标返回 500 时 API 返回测试结果而不是污染正式 delivery。

### 13.4 验证命令

涉及前端：

```bash
pnpm --dir web test
pnpm --dir web typecheck
pnpm --dir web build
```

涉及后端：

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```

提交前：

```bash
git diff --check
```

## 14. 文档同步

实现后必须同步：

- `README.md` 的 Web Console 能力说明。
- `ROADMAP.md` 对 Web Console 出站集成控制台的状态。
- `docs/manual/hooks.md`：补 Web Console 配置路径和测试投递。
- `docs/manual/notifications.md`：解释 sink 不只是通知，也服务 Hook。
- `docs/skills/wire-up-automation/references/hook-tools.md`：事件名与 UI 口径同步。
- `docs/skills/wire-up-automation/references/notification-tools.md`：sink test / preview 若新增需同步。

## 15. 风险与取舍

### 15.1 `/hooks` 命名过窄

长期更准确的路由是 `/integrations` 或 `/outbound`，但当前导航已有 `/hooks`，用户也已经知道 Hook 入口。决策：

- 第一版保留侧栏入口 `/hooks`，避免导航大改。
- Phase 1 同时新增 `/integrations` alias，渲染同一个出站集成控制台。
- 页面标题使用“集成 / 出站投递”，不再把页面解释成单一 Hook 表。

```text
/integrations -> /hooks
```

### 15.2 `notification_sinks` 名称过窄

DB/API 暂不重命名。产品文案用“Sink / 出站目标”，解释现有 `notification-sinks` API 是底层资源。不要为了命名整洁大迁移表结构。

### 15.3 测试投递是否落正式 delivery

建议不落正式 delivery，避免污染业务队列和重试逻辑。测试投递写 audit，并在 UI 展示最近测试结果。若后续需要长期保存测试记录，可新增 `outbound_test_runs`，不要塞进 `hook_deliveries`。

### 15.4 高级模板 UI 复杂

HTTP template 功能强但复杂。第一版表单应默认显示标准 webhook，HTTP template 放到高级模式。模板预设在 Phase 5 做，不阻塞核心闭环。

### 15.5 事件白名单前端硬编码

第一阶段可以硬编码并测试，因为后端白名单稳定且已文档化。若事件扩展频繁，再新增 `GET /api/v1/hook-event-types`。

## 16. 最小可发布切片

如果只做一个最小但真正有价值的发布切片，应包含：

1. Sinks tab：list/create/edit/enable/disable/delete 标准 webhook sink。
2. Hooks tab：list/create/edit/enable/disable/delete，sink 下拉，event checkbox，project selector。
3. Hook delivery detail + replay。
4. 权限显隐。
5. 文案修正 `task.completed`。

这已经能让用户不离开浏览器完成“创建 sink -> 创建 Hook -> 看投递 -> replay”的基础闭环。

下一小切片再做：

1. sink test API。
2. sink test UI。
3. HTTP template 高级表单。

## 17. 已裁定问题

1. **`/notifications` 保留路由，但不保留独立 sink 管理页面。** `/notifications` 渲染同一个出站集成控制台，默认打开“通知规则”或“Sinks”相关 tab；`/hooks` 默认打开 Hooks tab。这样保留已有入口，又避免两个割裂的 sink UI。
2. **sink test 首版只写 audit，不新增历史表。** 不新增 `outbound_test_runs`，不污染 `hook_deliveries` / `notification_deliveries`。如果后续需要长期保存测试结果，再单独设计表。
3. **sink test 必须支持可选 `project_ref`。** 对 `config_value` endpoint，用户可以选择 project 来验证 project config 解析；未传 project 时只按 workspace/default 解析，解析失败返回清晰错误。
4. **统一 delivery 聚合 endpoint 不进入 Phase 1。** Phase 1 使用现有 Hook delivery 和 Notification delivery endpoints；Phase 4 再引入 `GET /api/v1/outbound-deliveries`。
5. **Phase 1 新增 `/integrations` alias。** 侧栏仍指向 `/hooks`，但 `/integrations` 可直接访问同一页面，方便后续改名和外部链接过渡。
