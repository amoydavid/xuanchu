# Hook 管理

通过 xuanchu MCP 管理 Webhook Hook——自动化事件通知机制。

## 基本概念

Hook 是事件驱动的 Webhook 端点。当 xuanchu 中发生特定事件时，系统会向 Hook 的 URL 发送 HTTP POST 请求。

支持的事件类型：
- `task.created` — 任务创建
- `task.modified` — 任务修改
- `task.completed` — 任务完成
- `task.deleted` — 任务删除
- `project.archived` — 项目归档

Hook 可挂载在 workspace 级别或 project 级别（传 `project`/`project_id`）。

## Hook 生命周期

### hook_add — 创建 Hook

`name`、`url`、`events` 必填。

```json
// 输入：workspace 级别
{
  "workspace": "dajee",
  "name": "CI 触发器",
  "url": "https://ci.example.com/webhook",
  "events": ["task.completed", "task.deleted"],
  "secret": "whsec_xxx"
}

// 输入：项目级别
{
  "workspace": "dajee",
  "project_id": "proj-uuid-xxx",
  "name": "PR 检查",
  "url": "https://ci.example.com/pr-check",
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
      "endpoint_url": "https://ci.example.com/webhook",
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

### hook_test — 测试 Hook 配置（只读）

```json
{"workspace": "dajee", "hook": "hook-uuid-xxx"}
```

### hook_ping — Ping Hook（写审计日志）

```json
{"workspace": "dajee", "hook": "hook-uuid-xxx"}
```

## 典型 Agent 工作流

**场景：用户说"帮我设置一个 CI webhook"**

```json
// Step 1: 创建 Hook
hook_add({
  "workspace": "dajee",
  "name": "CI 触发器",
  "url": "https://ci.example.com/webhook",
  "events": ["task.completed", "task.deleted"]
})

// Step 2: 确认配置
hook_test({"workspace": "dajee", "hook": "返回的 hook ID"})

// Step 3: 检查投递状态（完成任务后）
hook_delivery_list({"workspace": "dajee", "hook": "hook-uuid-xxx", "limit": 5})

// Step 4: 失败时重试
hook_delivery_redeliver({"workspace": "dajee", "hook": "hook-uuid-xxx", "delivery_id": "..."})
```
