# Hook 管理

通过 Xuanchu MCP 管理 Webhook Hook——面向外部系统的事件投递机制。

## 基本概念

Hook 是事件驱动的出站集成。当 xuanchu 中发生特定事件时，系统会通过 workspace 级 notification sink 投递 HTTP 请求。Hook 不直接保存 URL 或 secret。

支持的事件类型：
- `task.created` — 任务创建
- `task.modified` — 任务修改
- `task.completed` — 任务完成
- `task.deleted` — 任务删除
- `task.started` — 任务开始
- `task.stopped` — 任务停止
- `task.assigned` — 任务新增负责人
- `task.unassigned` — 任务移除负责人
- `task.blocked` — 任务进入阻塞状态
- `task.due_changed` — 任务 due 字段变更
- `task.priority_changed` — 任务 priority 字段变更
- `task.project_changed` — 任务 project 字段变更
- `task.tags_changed` — 任务 tags 变更
- `project.archived` — 项目归档
- `project.annotated` — 项目新增注释
- `project.denotated` — 项目删除注释
- `task.unblocked` — 任务依赖解除阻塞

Hook 可挂载在 workspace 级别或 project 级别（传 `project`/`project_id`）。
`sink` 可以是当前 workspace 内的 sink 名称或 ID；不能引用其他 workspace 的 sink。
Hook 只引用 notification sink，不直接接收 URL 或 secret。sink 的 `max_concurrency` 控制同一 sink 的单进程出站并发；`0` 表示继承 dispatcher 默认 sink 并发。`xuanchu server` 内 hook dispatcher 与 notification dispatcher 共享同一个 sink limiter，同一 sink 的并发不会因为 Hook 和通知同时投递而翻倍。

## Hook 生命周期

### hook_add — 创建 Hook

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

### hook_list — 列出 Hook

只读。

```json
{"workspace": "dajee"}
```

### hook_info — 查看 Hook 详情

只读。

```json
{"workspace": "dajee", "hook": "hook-uuid-xxx"}
```

### hook_modify — 修改 Hook

```json
{"workspace": "dajee", "hook": "hook-uuid-xxx", "name": "CI/CD 触发器"}
{"workspace": "dajee", "hook": "hook-uuid-xxx", "events": ["task.created", "task.completed"]}
{"workspace": "dajee", "hook": "hook-uuid-xxx", "sink": "ci-webhook-prod"}
{"workspace": "dajee", "hook": "hook-uuid-xxx", "active": false}
```

### hook_remove — 删除 Hook

```json
{"workspace": "dajee", "hook": "hook-uuid-xxx"}
```

## 投递记录

### hook_delivery_list — 列出投递记录

只读。默认 20 条。

```json
{"workspace": "dajee", "hook": "hook-uuid-xxx", "limit": 10}
```

### hook_delivery_info — 查看投递详情

只读。

```json
{"workspace": "dajee", "hook": "hook-uuid-xxx", "delivery_id": "delivery-uuid-xxx"}
```

### hook_delivery_redeliver — 重试投递

```json
{"workspace": "dajee", "hook": "hook-uuid-xxx", "delivery_id": "delivery-uuid-xxx"}
```

## 测试与诊断

## 典型 Agent 工作流

**场景：用户说"帮我设置一个 CI webhook"**

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
