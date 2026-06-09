# 系统与审计

通过 Xuanchu MCP 查看审计日志、当前身份和可用权限。

## 查看当前身份

### me_get — 获取当前用户信息

用于 Agent 确认"我是谁"。stdio 模式返回 active user，HTTP 模式返回 token 绑定的用户。

```json
// 输入
{}

// 返回
{
  "data": {
    "user": {
      "id": "user-uuid-xxx",
      "name": "alice",
      "email": "alice@example.com",
      "external_ids": [
        {"provider": "feishu", "external_id": "ou_12345"}
      ]
    }
  },
  "rendered": "user alice"
}
```

## 审计日志

### audit_list — 列出审计条目

审计日志记录所有写操作。支持 `limit`、`offset`、`actor` 过滤。

```json
// 输入：最近 10 条
{"workspace": "dajee", "limit": 10}

// 输入：按操作者过滤
{"workspace": "dajee", "actor": "alice", "limit": 5}

// 输入：指定项目
{"workspace": "dajee", "project_id": "proj-uuid-xxx", "limit": 20}

// 返回
{
  "data": {
    "entries": [
      {
        "id": 42,
        "actor": {"id": "user-uuid-xxx", "name": "alice"},
        "workspace_id": "ws-uuid-xxx",
        "project_id": "proj-uuid-xxx",
        "action": "task.add",
        "target_type": "task",
        "target_id": "task-uuid-xxx",
        "payload": {"description": "修复白屏问题"},
        "created_at": 1748707200
      }
    ],
    "count": 1
  }
}
```

常见 `action` 值：
- 任务：`task.add`、`task.done`、`task.delete`、`task.modify`、`task.annotate`、`task.denotate`、`task.import`
- 项目：`project.add`、`project.modify`、`project.archive`、`project.annotate`、`project.denotate`
- Workspace：`workspace.add`、`workspace.modify`、`workspace.archive`、`workspace.use`
- 用户与成员：`user.add`、`user.use`、`user.bind_external_id`、`user.unbind_external_id`、`member.add`、`member.role`
- Context / Config：`context.define`、`context.use`、`context.none`、`context.delete`、`config.set`、`config.unset`、`config.schema.set`、`config.schema.delete`
- Hook：`hook.create`、`hook.modify`、`hook.delete`、`hook.replay`
- 通知与提醒：`notification.sink.create`、`notification.sink.modify`、`notification.sink.enable`、`notification.sink.disable`、`notification.sink.delete`、`notification.delivery.replay`、`reminder.rule.create`、`reminder.rule.modify`、`reminder.rule.enable`、`reminder.rule.disable`、`reminder.rule.delete`
- Token：`token.create`、`token.modified`、`token.revoke`

## 可用权限

### scope_list — 列出所有可用 scope

无需鉴权。

```json
// 输入
{}

// 返回
{
  "data": {
    "scopes": [
      "task:read", "task:write",
      "project:read", "project:write",
      "context:read", "context:write",
      "workspace:read", "workspace:write",
      "config:read", "config:write",
      "hook:read", "hook:write",
      "notification:read", "notification:write",
      "reminder:read", "reminder:write",
      "token:read", "token:write",
      "audit:read",
      "impersonate"
    ]
  }
}
```

## 典型 Agent 工作流

**场景：排查"谁删除了我的任务"**

```json
// Step 1: 查看最近的删除操作
audit_list({"workspace": "dajee", "limit": 20, "actor": "bob"})

// Step 2: 过滤 action 为 task.delete 的条目，查看 payload 和 target_id
```

**场景：Agent 启动自检**

```json
// Step 1: 确认身份
me_get({})

// Step 2: 查看可用权限
scope_list({})
```
