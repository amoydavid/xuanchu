# Hook 工具

Hook 是事件驱动的出站集成。当 xuanchu 中发生特定事件时，系统会通过 workspace 级 notification sink 投递 HTTP 请求。Hook 不直接保存 URL 或 secret。

## 基本概念

- Hook 可挂载在 workspace 级别或 project 级别（传 `project`/`project_id`）。
- `sink` 可以是当前 workspace 内的 sink 名称或 ID；不能引用其他 workspace 的 sink。
- Hook 只引用 notification sink，不直接接收 URL 或 secret。
- sink 的 `max_concurrency` 控制同一 sink 的单进程出站并发；`0` 表示继承 dispatcher 默认 sink 并发。
- `xuanchu server` 内 hook dispatcher 与 notification dispatcher 共享同一个 sink limiter，同一 sink 的并发不会因为 Hook 和通知同时投递而翻倍。

事件订阅建议：
- `start` 只触发 `task.started`，`stop` 只触发 `task.stopped`；不要只靠 `task.modified` 捕获开始或停止。
- assignee、due、priority、project、tags、blocked 状态变化应优先订阅对应细粒度事件。
- `task.blocked` 表示从非 blocked 进入 blocked，`task.unblocked` 表示解除 blocked。

事件完整清单见 event-types.md。

## hook_add — 创建 Hook

`name`、`sink`、`events` 必填。

```json
// 输入：workspace 级别
{
  "workspace": "dajee",
  "name": "CI 触发器",
  "sink": "ci-webhook",
  "events": ["task.completed", "task.deleted"]
}

// 输入：项目级别
{
  "workspace": "dajee",
  "project_id": "proj-uuid-xxx",
  "name": "PR 检查",
  "sink": "ci-webhook",
  "events": ["task.created"]
}

// 返回
{
  "data": {
    "hook": {
      "id": "hook-uuid-xxx",
      "name": "CI 触发器",
      "scope_type": "workspace",
      "workspace_id": "ws-uuid-xxx",
      "event_types": ["task.completed", "task.deleted"],
      "sink_id": "sink-uuid-xxx",
      "sink_name": "ci-webhook",
      "sink_type": "webhook",
      "enabled": true
    }
  },
  "rendered": "created hook CI 触发器"
}
```

## hook_list — 列出 Hook（只读）

```json
{"workspace": "dajee"}
```

## hook_info — 查看 Hook 详情（只读）

```json
{"workspace": "dajee", "hook": "hook-uuid-xxx"}
```

## hook_modify — 修改 Hook

```json
{"workspace": "dajee", "hook": "hook-uuid-xxx", "name": "CI/CD 触发器"}
{"workspace": "dajee", "hook": "hook-uuid-xxx", "events": ["task.created", "task.completed"]}
{"workspace": "dajee", "hook": "hook-uuid-xxx", "sink": "ci-webhook-prod"}
{"workspace": "dajee", "hook": "hook-uuid-xxx", "active": false}
```

## hook_remove — 删除 Hook

```json
{"workspace": "dajee", "hook": "hook-uuid-xxx"}
```

## 投递记录

### hook_delivery_list — 列出投递记录（只读）

默认 20 条。

```json
{"workspace": "dajee", "hook": "hook-uuid-xxx", "limit": 10}
```

### hook_delivery_info — 查看投递详情（只读）

```json
{"workspace": "dajee", "hook": "hook-uuid-xxx", "delivery_id": "delivery-uuid-xxx"}
```

### hook_delivery_redeliver — 重试投递

```json
{"workspace": "dajee", "hook": "hook-uuid-xxx", "delivery_id": "delivery-uuid-xxx"}
```

## 典型工作流：设置 CI webhook

```json
// Step 1: 确认或创建 notification sink
notification_sink_add({
  "workspace": "dajee",
  "name": "ci-webhook",
  "type": "webhook",
  "endpoint_mode": "static_url",
  "url": "https://ci.example.com/webhook",
  "secret": "webhook-secret",
  "max_concurrency": 0
})

// Step 2: 创建 Hook
hook_add({
  "workspace": "dajee",
  "name": "CI 触发器",
  "sink": "ci-webhook",
  "events": ["task.completed", "task.deleted"]
})

// Step 3: 检查投递状态（完成任务后）
hook_delivery_list({"workspace": "dajee", "hook": "hook-uuid-xxx", "limit": 5})

// Step 4: 失败时重试
hook_delivery_redeliver({"workspace": "dajee", "hook": "hook-uuid-xxx", "delivery_id": "..."})
```

## 事件名口径

事件名遵循 `task.*` / `project.*` 白名单。常见事件：

- 任务基础：`task.created`、`task.modified`、`task.completed`、`task.deleted`
- 任务状态：`task.started`、`task.stopped`、`task.blocked`、`task.unblocked`
- 字段变化：`task.assigned`、`task.unassigned`、`task.due_changed`、`task.priority_changed`、`task.project_changed`、`task.tags_changed`
- 项目：`project.archived`、`project.transitioned`、`project.annotated`、`project.denotated`

> 旧的 `task.done` 不存在；请使用 `task.completed`。Web Console 的事件 checkbox 与本口径一致。

## Web Console 与 sink 测试

集成配置主入口是 Web Console `/hooks`（出站集成控制台）。sink/hook 的 CRUD、事件 checkbox、project 选择、投递排障与 replay 都在浏览器内完成。每个 sink 行有「发送测试」按钮，调用 `POST /api/v1/notification-sinks/{sinkID}/test` 验证可达性，不会写入正式 delivery 表。
