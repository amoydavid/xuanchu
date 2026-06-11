# v0.3.0 Priority 1 语义事件补齐

**日期：** 2026-06-10
**状态：** 已实现
**版本：** v0.3.0
**对应 ROADMAP：** v0.3.0 事件通知、Hook sink 化与语义事件补齐

## 1. 背景

Xuanchu v0.3.0 已完成 Hook sink 化、dispatcher 并发与背压、事件通知规则，以及首批 8 个事件类型：

- `task.created`、`task.modified`、`task.completed`、`task.deleted`
- `project.archived`、`project.annotated`、`project.denotated`
- `task.unblocked`

ROADMAP 明确列出 9 个 Priority 1 事件需要在 v0.3.0 中补齐。补齐后 v0.3.0 可以正式关闭。

## 2. 目标

为 Xuanchu 补齐 9 个 Priority 1 语义事件，让 Hook 和 notification rule 可以按字段级粒度精确订阅任务变更。

## 3. 新增事件类型

| 事件类型 | 触发时机 | payload 差异字段 |
|---|---|---|
| `task.assigned` | assignee 列表新增成员 | `added_assignees` / `current_assignees` |
| `task.unassigned` | assignee 列表移除成员 | `removed_assignees` / `current_assignees` |
| `task.started` | `start` 操作 | 无额外差异 |
| `task.stopped` | `stop` 操作 | 无额外差异 |
| `task.blocked` | 任务从未 blocked 变为 blocked（modify 新增依赖 / add 带依赖） | `blocking_dependencies` |
| `task.due_changed` | due 字段变更 | `previous_due` / `current_due` |
| `task.priority_changed` | priority 字段变更 | `previous_priority` / `current_priority` |
| `task.project_changed` | project 字段变更 | `previous_project` / `current_project` |
| `task.tags_changed` | tags 变更 | `added_tags` / `removed_tags` / `current_tags` |

## 4. 事件分类规则

### 4.1 start / stop

- `Start` 发射 `task.started`，**不再**发射 `task.modified`。
- `Stop` 发射 `task.stopped`，**不再**发射 `task.modified`。

### 4.2 modify

`Modify` 发射 `task.modified` **加上**按实际变更字段叠加的细粒度事件。

示例：如果 modify 同时变更了 priority 和 tags，事件列表为：

```text
task.modified, task.priority_changed, task.tags_changed
```

如果 modify 只变更了 description，事件列表为：

```text
task.modified
```

### 4.3 其他写操作

- `add` → `task.created`（不变）
- `done` → `task.completed` + `task.unblocked`（不变）
- `delete` → `task.deleted`（不变）
- `annotate` / `denotate` / `append` / `prepend` / `edit` → `task.modified`（不变，不加细粒度）

### 4.4 assignee 变更

assignee 变更通过 `modify` 操作触发（`+@ref` / `-@ref` / `@ref`）。

- 新增 assignee → `task.assigned`
- 移除 assignee → `task.unassigned`
- 同时新增和移除 → `task.assigned` + `task.unassigned`

assignee 变更**不会**额外发射 `task.modified` 以外的细粒度 assignee 事件——它本身就是细粒度事件的一部分。即 modify 如果同时变了 assignee 和 priority，事件列表为：

```text
task.modified, task.assigned, task.priority_changed
```

### 4.5 blocked 状态变化

`task.blocked` 在以下时机触发：

- **modify 新增依赖**：如果新增的依赖目标未完成，且被修改任务从 non-blocked 变为 blocked
- **add 新建任务带依赖**：如果新建任务已有未完成依赖

注意：`done` 完成任务只会让依赖它的任务 unblocked，不会让任何任务变成 blocked，因此 done 不需要检测 `task.blocked`。

检测策略与 `task.unblocked` 对称：在操作前后各跑一次 `buildDependencyState`，找出从 non-blocked 变为 blocked 的任务。

## 5. 统一 Task Change Diff 构建器

### 5.1 核心数据结构

新增文件 `internal/app/task_change_events.go`：

```go
type TaskChangeDiff struct {
    AssigneesChanged  bool
    AddedAssignees    []task.UserInfo
    RemovedAssignees  []task.UserInfo

    DueChanged      bool
    PreviousDue     *int64
    CurrentDue      *int64

    PriorityChanged  bool
    PreviousPriority *string
    CurrentPriority  *string

    ProjectChanged  bool
    PreviousProject *string
    CurrentProject  *string

    TagsChanged bool
    AddedTags   []string
    RemovedTags []string
}
```

### 5.2 核心函数

```go
// diffTaskChanges 对比 before/after 任务，返回字段差异。
func diffTaskChanges(before, after task.Task) TaskChangeDiff

// buildFineGrainedEvents 根据 diff 构建细粒度事件列表（不含 task.modified 本身）。
func buildFineGrainedEvents(diff TaskChangeDiff, after task.Task, runtime RuntimeContext, now int64) []HookEvent
```

### 5.3 assignee diff 特殊处理

assignee diff 需要 hydrate `UserInfo`。在 `modifyLocked` 中，before.assignees 和 after.assignees 都是 `[]string`（user_id 列表）。构建 diff 时：

1. 计算 `added` 和 `removed` user_id 集合
2. 批量调用 `resolveUserInfos` 解析完整 `UserInfo`
3. 在 `buildFineGrainedEvents` 中构建带完整 `UserInfo` 的 `added_assignees` / `removed_assignees` / `current_assignees`

### 5.4 blocked 检测

blocked 检测不在 `diffTaskChanges` 中，因为 blocked 状态不是任务字段，而是运行时依赖计算结果。blocked 事件在以下位置构建：

- `modifyLocked`：新增依赖后，复用 `buildDependencyState` 检测
- `addLocked`：新建任务有依赖时
- `doneLocked`：完成后，与 `task.unblocked` 对称检测

## 6. 各写路径改造

### 6.1 Modify

```
Modify(target, input)
  before = resolveTargetForWrite(target)   // 保存修改前快照
  modified = modifyLocked(target, input)   // 执行修改
  diff = diffTaskChanges(before, modified)
  fineGrained = buildFineGrainedEvents(diff, modified, ...)
  blocked = detectBlockedEvents(...)
  events = [task.modified] + fineGrained + blocked
```

### 6.2 Start

```
Start(target)
  before = resolveTargetForWrite(target)
  started = startLocked(target)
  events = [buildTaskHookEvent("task.started", started, ...)]
```

### 6.3 Stop

```
Stop(target)
  before = resolveTargetForWrite(target)
  stopped = stopLocked(target)
  events = [buildTaskHookEvent("task.stopped", stopped, ...)]
```

### 6.4 Add

```
Add(input)
  created = addLocked(input)
  events = [buildTaskHookEvent("task.created", created, ...)]
  blocked = detectBlockedEventsAfterAdd(created, ...)
  events += blocked
```

### 6.5 Done

```
Done(target)
  doneTask = doneLocked(target)
  events = [buildTaskHookEvent("task.completed", doneTask, ...)]
  unblocked = taskUnblockedEventsAfterDone(...)
  events += unblocked
```

Done 只会 unblock 依赖它的任务，不会产生 `task.blocked` 事件。

### 6.6 不变的写路径

`annotate`、`denotate`、`append`、`prepend`、`edit`、`delete` 保持现有事件发射逻辑不变。

## 7. 事件 Payload 结构

所有细粒度事件 payload 遵循统一规范：

- 必须携带标准 task 快照（`data.task`），复用 `task.ToJSON(tsk)`
- 差异信息放在 `data` 根层，不在 `task` 内部
- 用户字段使用 `task.UserInfo`
- null 值用 JSON `null` 表示（如 `previous_priority: null` 表示之前没有 priority）

### 7.1 task.priority_changed

```json
{
  "event_type": "task.priority_changed",
  "data": {
    "task": { "uuid": "...", "description": "...", "priority": "H", ... },
    "previous_priority": "M",
    "current_priority": "H"
  }
}
```

### 7.2 task.tags_changed

```json
{
  "event_type": "task.tags_changed",
  "data": {
    "task": { ... },
    "added_tags": ["urgent"],
    "removed_tags": ["review"],
    "current_tags": ["urgent", "docs"]
  }
}
```

### 7.3 task.assigned

```json
{
  "event_type": "task.assigned",
  "data": {
    "task": { ... },
    "added_assignees": [
      {"id": "user-uuid", "name": "alice", "email": "alice@example.com", "external_ids": []}
    ],
    "current_assignees": [
      {"id": "user-uuid", "name": "alice", "email": "alice@example.com", "external_ids": []}
    ]
  }
}
```

### 7.4 task.unassigned

```json
{
  "event_type": "task.unassigned",
  "data": {
    "task": { ... },
    "removed_assignees": [
      {"id": "user-uuid", "name": "bob", "email": null, "external_ids": []}
    ],
    "current_assignees": []
  }
}
```

### 7.5 task.due_changed

```json
{
  "event_type": "task.due_changed",
  "data": {
    "task": { ... },
    "previous_due": 1749427199,
    "current_due": 1750291199
  }
}
```

### 7.6 task.project_changed

```json
{
  "event_type": "task.project_changed",
  "data": {
    "task": { ... },
    "previous_project": "oldproject",
    "current_project": "newproject"
  }
}
```

### 7.7 task.blocked

```json
{
  "event_type": "task.blocked",
  "data": {
    "task": { ... },
    "blocking_dependencies": ["uuid-of-blocking-task-1", "uuid-of-blocking-task-2"]
  }
}
```

### 7.8 task.started / task.stopped

```json
{
  "event_type": "task.started",
  "data": {
    "task": { ... }
  }
}
```

无额外差异字段。task 快照中已包含更新后的 `start` 时间戳。

## 8. 白名单扩展

`allowedHookEventTypes`（`internal/app/hook.go`）从 8 个扩展到 17 个：

```go
var allowedHookEventTypes = map[string]bool{
    "task.created":        true,
    "task.modified":       true,
    "task.completed":      true,
    "task.deleted":        true,
    "task.started":        true,
    "task.stopped":        true,
    "task.assigned":       true,
    "task.unassigned":     true,
    "task.blocked":        true,
    "task.due_changed":    true,
    "task.priority_changed":  true,
    "task.project_changed":   true,
    "task.tags_changed":      true,
    "task.unblocked":         true,
    "project.archived":       true,
    "project.annotated":      true,
    "project.denotated":      true,
}
```

notification rule 的 `normalizeEventNotificationRuleFields` 复用同一白名单，无需额外改造。

## 9. 外部接口影响

### 9.1 MCP tool

`tools_hook.go` 的 `Events` 字段 jsonschema description 需要更新，包含全部 17 个事件类型。

`tools_notification.go` 的 `Event` 字段 jsonschema description 同理。

### 9.2 HTTP API

`POST /api/v1/hooks` 和 `PATCH /api/v1/hooks/{hookID}` 的 event_types 校验自动跟随 `allowedHookEventTypes` 扩展，无需额外改造。

`POST /api/v1/notification-rules` 和 `PATCH /api/v1/notification-rules/{ruleID}` 的 event_type 校验同理。

### 9.3 CLI

`hook add --event` 的 event 类型校验自动跟随白名单扩展。

### 9.4 OpenAPI

`docs/openapi/xuanchu-v1.yaml` 中的 hook event type enum 需要更新。

### 9.5 MCP Skill 文档

`docs/skills/*/SKILL.md` 中的 event type 列表需要同步更新。

### 9.6 Breaking Change

`start` 和 `stop` 不再发射 `task.modified`。订阅了 `task.modified` 的 hook/notification rule 将不再收到 start/stop 事件。这是一个**有意的行为变更**，属于 v0.3.0 的版本内变更。

迁移建议：如果用户需要同时捕获 start/stop 和 modify，需要额外订阅 `task.started` / `task.stopped`。

## 10. 不进入本 spec

- Priority 2 事件：`task.annotated`、`task.denotated`、`task.link_added`、`task.link_removed`、`project.created`、`project.updated`、`workspace.member_added`、`workspace.member_removed`、`workspace.member_role_changed`
- `task.due_soon`、`task.overdue` 等时间条件事件（属 reminder rule 范畴）
- 事件订阅 UI / 实时推送（WebSocket / SSE）
- 多实例部署的全局并发控制
- 事件持久化 / 事件溯源

## 11. 验收标准

- `allowedHookEventTypes` 包含全部 17 个事件类型
- Hook 和 notification rule 可订阅全部新事件类型
- `Modify` 按实际变更字段发射 `task.modified` + 细粒度事件
- `Start` 只发射 `task.started`，不再发射 `task.modified`
- `Stop` 只发射 `task.stopped`，不再发射 `task.modified`
- 细粒度事件 payload 包含标准 task 快照和差异字段
- `task.blocked` 在 blocked 状态变化时正确触发
- `task.unblocked` 继续正常工作
- `annotate`、`denotate`、`append`、`prepend`、`edit` 继续只发射 `task.modified`
- 现有 `task.created`、`task.completed`、`task.deleted` 行为不变
- `go test ./...`、`CGO_ENABLED=0 go test ./...`、`CGO_ENABLED=0 go build ./cmd/xuanchu` 通过

## 12. 规格与实施计划

- Spec：`docs/superpowers/specs/2026-06-10-xuanchu-p1-semantic-events-design.md`（本文档）
- Plan：`docs/superpowers/plans/2026-06-10-xuanchu-p1-semantic-events-implementation.md`
