# Task 变更历史（Audit Payload + 详情页 Timeline）设计

- 日期：2026-07-04
- 状态：已评审，待实现
- 里程碑：v0.5.2 候选（任务详情可追溯性）
- 关联文档：[ROADMAP.md](../../../ROADMAP.md)、[README.md](../../../README.md)、`docs/superpowers/specs/2026-06-17-web-console-project-task-browsing-design.md`、`docs/superpowers/specs/2026-06-27-xuanchu-web-console-editing-design.md`

## 1. 背景与目标

任务详情页 `/workspaces/{workspace}/projects/{project}/tasks/{task}` 已经能编辑 title、description、负责人、截止日期、优先级、标签、注解和链接，但无法回答「这个任务发生过哪些字段变化」。后端现状是：

1. 每次 task 修改都会写一条 `audit_logs` 记录，action 为 `task.modify`，带 actor、时间、target type/id 和 project 信息。
2. `diffTaskChanges` 已经为 HookEvent 计算 assignees / due / priority / project / tags 的 before/after 差异，但这些字段级差异只进入事件流，不持久化到 audit payload。
3. title / description 目前不产生字段级 HookEvent，也不进入 audit payload。
4. `Service.ListAudit` 和 `storage.AuditRepository.List` 只支持 workspace/project 过滤，不支持按 `target_type` / `target_id` 查询单个 task 的审计流。
5. Web Console 任务详情页没有历史区块。

本期目标：在任务详情页展示「谁、什么时候、把哪个字段从什么改成了什么」的字段级变更历史，并让这个历史来自已有 `audit_logs`，而不是另建一套事件存储。

## 2. 设计原则

- **复用 `audit_logs`，不新建表。** `audit_logs` 已经承载 actor、workspace、project、target、时间和权限上下文；字段级 diff 放进既有 `payload_json`。
- **复用现有 diff 逻辑。** assignees / due / priority / project / tags 继续以 `diffTaskChanges` + `hydrateAssigneeDiff` 为来源，不在 HTTP 或前端重新计算。
- **任务详情历史按 task read 授权。** 通用 `/api/v1/audit` 仍要求 `audit:read` / `PermissionAuditRead`；新增 `/api/v1/tasks/{taskRef}/audit` 是任务详情的一部分，先按 `task:read` 解析 task，再只返回该 task 的 `task.modify` 历史，避免普通 member/viewer 能读任务却看不到详情页历史。
- **写入和读取解耦。** 写入仍保持一条 `task.modify` audit；读取时 app/view 层把 payload 中的 `changes` 解释成结构化、可渲染的变化视图，HTTP 再序列化给前端。
- **持久化机器语义，展示层生成人话。** `audit_logs.payload_json` 只保存字段、旧值、新值、added/removed 等稳定机器语义，不把中文或英文整句写入数据库；HTTP/Web 层负责把它渲染成「Alice 将标题从 A 改为 B」这类人类文字。
- **不改变 HookEvent 契约。** 本期只增强 audit payload，不新增 `task.title_changed` / `task.description_changed` 事件，不改变已有 webhook/notification payload。

## 3. 范围与非范围

### 3.1 进入本期

| 位置 | 现状 | 本期改动 |
|---|---|---|
| `internal/app/task_change_events.go` | diff 覆盖 assignees / due / priority / project / tags | 扩展 diff 覆盖 title / description，仅供 audit payload 使用 |
| `internal/app/project_query.go` / 新 helper | `taskAuditEntry` 只写 project before/after payload | 保持通用 helper 不变；新增 `taskModifyAuditEntry` 只增强 `task.modify` payload |
| `internal/app/audit.go` | `AuditListInput` 只支持 workspace/project 过滤 | 新增 target 过滤；新增 task 专用读取入口，要求 `PermissionTaskRead` |
| `internal/storage/audit_repo.go` | 只按 workspace/project 查询 | 支持 `target_type` / `target_id` 过滤 |
| `internal/storage/models.go` | `audit_logs.target_type/target_id` 无复合索引 | 新增 `(workspace_id, target_type, target_id, created_at DESC)` 访问路径索引 |
| HTTP API | 仅 `GET /api/v1/audit` | 新增 `GET /api/v1/tasks/{taskRef}/audit` |
| HTTP audit response | 只输出原始 `payload` | 增加顶层可渲染 `changes`，保留 `payload` |
| Web Console | 任务详情页无历史区块 | 新增任务变更历史区块和 API client |

### 3.2 不进入本期

- 不做历史 audit payload 回填；旧 `task.modify` 行没有 `changes` 时返回空数组。
- 不引入软删、撤销、版本快照或完整字段快照表。
- 不接入 CLI、MCP、Remote Client 的 task history 展示；通用 audit CLI 继续保持现状。
- 不记录 annotation / link / dependency / UDA / wait / scheduled / until / recur 的字段级 diff。这些要么已有独立 action，要么需要另一个设计处理。
- 不做 description 并排 diff 视图；首版只展示截断文本和展开查看。
- 不做字段级权限过滤；能读该 task history 的用户能看到本期记录的全部字段旧值和新值。

## 4. Payload 契约

### 4.1 `audit_logs.payload_json`

现有 `task.modify` payload 保留 project 冗余字段：

```json
{
  "before_project_id": "...",
  "before_project_slug": "ops",
  "after_project_id": "...",
  "after_project_slug": "ops"
}
```

扩展后追加 `changes`：

```json
{
  "before_project_id": "...",
  "before_project_slug": "ops",
  "after_project_id": "...",
  "after_project_slug": "ops",
  "changes": [
    {
      "field": "assignees",
      "added": [{"id":"u2","name":"lisi","display_name":"李四","email":null,"external_ids":[]}],
      "removed": [{"id":"u1","name":"zhangsan","display_name":"张三","email":null,"external_ids":[]}]
    },
    {
      "field": "due",
      "previous": 1783036800,
      "current": null
    },
    {
      "field": "priority",
      "previous": "M",
      "current": "H"
    },
    {
      "field": "project",
      "previous": "ops",
      "current": "agentapi"
    },
    {
      "field": "tags",
      "added": ["dashboard"],
      "removed": ["ads"]
    },
    {
      "field": "title",
      "previous": "旧标题",
      "current": "新标题"
    },
    {
      "field": "description",
      "previous": "旧描述...",
      "current": "新描述..."
    }
  ]
}
```

### 4.2 字段编码规则

| field | 键 | 类型 | 语义 |
|---|---|---|---|
| `assignees` | `added` / `removed` | `UserInfo[]` | 集合 diff |
| `tags` | `added` / `removed` | `string[]` | 集合 diff |
| `due` | `previous` / `current` | unix 秒或 `null` | 标量替换 |
| `priority` | `previous` / `current` | 字符串或 `null` | 标量替换 |
| `project` | `previous` / `current` | project slug 或 `null` | 标量替换 |
| `title` | `previous` / `current` | 字符串 | 标量替换 |
| `description` | `previous` / `current` | 字符串或 `null` | 标量替换 |

约束：

- 未变更的字段不出现在 `changes` 中。
- 集合字段只有 added/removed 至少一边非空时才写入。
- 顺序固定为 `assignees` -> `due` -> `priority` -> `project` -> `tags` -> `title` -> `description`，方便测试和前端稳定渲染。
- `assignees` 使用完整 `task.UserInfo` JSON 形状：`id` / `name` / `display_name` / `email` / `external_ids`。`name` 是稳定查找名，`display_name` 是展示名；前端展示人名时按 `display_name` -> `name` -> `id` 回退。实现可以改造 app 层现有 `userInfoToEventPayload` 或复用 `task.UserInfoToJSON` 的语义，但不能输出裸 UUID，也不能丢掉 `display_name`。
- `project` 的 slug diff 与顶层 `before_project_*` / `after_project_*` 必须来自同一个 `projectChange`，避免两个来源漂移。
- payload 中的 `previous` / `current` key 必须保留显式 `null`。例如清空 due 时必须是 `"current": null`，不能因为 Go `omitempty` 或前端可选字段把 key 省略。

### 4.3 HTTP 可渲染 change view

HTTP 顶层 `changes` 不是把 payload 原样搬出，而是面向前端渲染的人类可读视图。它必须保留 raw 值，同时给前端足够信息生成自然语言：

```json
{
  "field": "due",
  "kind": "scalar",
  "label_key": "projectWorkbench.taskHistory.field.due",
  "previous": {"raw": 1783036800, "text": "2026-07-04"},
  "current": {"raw": null, "text": "未设置"}
}
```

集合类：

```json
{
  "field": "assignees",
  "kind": "set",
  "label_key": "projectWorkbench.taskHistory.field.assignees",
  "added": [
    {
      "raw": {"id":"u2","name":"lisi","display_name":"李四","email":null,"external_ids":[]},
      "text": "李四"
    }
  ],
  "removed": []
}
```

规则：

- `field` 是稳定机器字段名；前端可据此选择 icon、样式和字段专用格式化。
- `kind` 只取 `scalar` / `set`，前端不需要靠是否存在 `added` 来猜渲染模板。
- `label_key` 是 i18n key，不是最终文案；前端用当前语言翻译字段名。
- `text` 是兜底显示值，不是最终文案。前端应优先用 `field` + `raw` + 当前 i18n locale 格式化值：空值用 `projectWorkbench.taskHistory.unset`，due 用本地日期格式，UserInfo 优先 `display_name` / `name` / `id`，tags/project/priority/title 直接文本化，description 取纯文本摘要；只有遇到未知类型时才回退到 `text`。
- `raw` 保留原始值，前端需要更好的 locale 格式化时可覆盖 `text`。例如 due 可由前端按用户 locale 格式化为日期。
- 标量 change 的 `previous` 和 `current` 必须同时出现，即使 raw 为 `null`。
- 集合 change 的 `added` 和 `removed` 必须同时出现；可以为空数组。
- HTTP response 不提供整句 `summary` 作为契约字段，避免服务端固定语言和语序。整句由前端 i18n 模板生成。

## 5. 后端设计

### 5.1 字段 diff 扩展

`internal/app/task_change_events.go`：

- `TaskChangeDiff` 新增 title 和 description 字段：

```go
TitleChanged       bool
PreviousTitle      string
CurrentTitle       string
DescriptionChanged bool
PreviousDescription *string
CurrentDescription  *string
```

- `diffTaskChanges(before, after task.Task)` 增加：
  - `before.Title != after.Title`
  - `ptrStringDiff(before.Description, after.Description)`
- `buildFineGrainedEvents` 不为 title/description 生成 HookEvent。

### 5.2 audit payload 构造

建议新增 `internal/app/task_audit_payload.go`，避免继续膨胀 `project_query.go`：

```go
func taskModifyAuditPayload(change projectChange, diff TaskChangeDiff) map[string]any
func taskFieldChanges(change projectChange, diff TaskChangeDiff) []map[string]any
```

规则：

- 先调用 `projectChangePayload(change)` 保留现有 project 字段。
- 追加 `changes` 数组。即使数组为空，也写 `changes: []`，让新写入的 `task.modify` payload 与历史缺字段区分开。
- assignees 复用 `diff.AddedAssignees` / `diff.RemovedAssignees`，序列化为完整 UserInfo。
- UserInfo 序列化必须包含 `display_name`。如果复用 `userInfoToEventPayload`，需要先补上 `display_name` 字段；否则历史里只能显示稳定 `name`，不符合前端人类可读要求。
- title/description 只进 audit payload，不进入 HookEvent。

保持通用 `taskAuditEntry(action, targetID, change)` 不变，避免给 `task.add`、`task.done`、`task.annotate` 等非本期 action 追加无意义的 `changes`。新增 task.modify 专用 helper：

```go
func taskModifyAuditEntry(targetID string, change projectChange, diff TaskChangeDiff) AuditEntry
```

`Service.Modify` 在 `hydrateAssigneeDiff` 后调用 `taskModifyAuditEntry(modified.UUID, change, diff)`。其他 task action 继续调用原有 `taskAuditEntry`。

### 5.3 audit 查询与 task 专用入口

`internal/storage/audit_repo.go`：

- `AuditListOptions` 新增：

```go
TargetType *string
TargetID   *string
Action     *string
```

- `List` 对非空字段追加参数绑定 where 条件，不做字符串拼接。
- 保持默认排序 `created_at DESC, id DESC`。

`internal/app/audit.go`：

- `AuditListInput` 新增 `TargetType` / `TargetID` / `Action`，通用 `ListAudit` 仍要求 `PermissionAuditRead`。
- 新增 task 专用入口：

```go
type TaskAuditInput struct {
    Limit  int
    Offset int
}

func (s *Service) ListTaskAudit(taskRef string, input TaskAuditInput) ([]AuditLogView, error)
```

- `ListTaskAudit` 先 `Require(PermissionTaskRead)`，再 `ResolveProtocolTarget(taskRef)`，最后用 `TargetType="task"`、`TargetID=resolved.UUID`、`Action="task.modify"` 查询。
- 解析 rows 时复用一个私有 helper，例如 `auditLogViewsFromRows(rows []storage.AuditLogEntry)`，避免 `ListAudit` 和 `ListTaskAudit` 复制 actor/delegator 解析逻辑。
- `AuditLogView` 新增 `Changes []TaskFieldChange`。解析 `PayloadJSON` 失败时 `Changes` 为空，不影响 audit 行返回。

`TaskFieldChange` 建议定义为 app view 结构。不要对标量的 `Previous` / `Current` 使用 `omitempty`，否则清空字段时会丢失显式 `null`：

```go
type TaskChangeDisplayValue struct {
    Raw  any
    Text string
}

type TaskFieldChange struct {
    Field    string
    Kind     string
    LabelKey string
    Previous *TaskChangeDisplayValue
    Current  *TaskChangeDisplayValue
    Added    []TaskChangeDisplayValue
    Removed  []TaskChangeDisplayValue
}
```

这只是 app view 的概念形状。HTTP response DTO 必须使用 snake_case JSON tag，例如 `label_key`、`previous`、`current`、`added`、`removed`、`raw`、`text`。

HTTP response 再通过 JSON tag 控制集合字段的空数组和标量字段的 presence。标量 `previous/current` 必须能表达 raw 为 `null` 的值；集合 `added/removed` 允许空数组。实现时不要把标量 `previous/current` 标成 `omitempty`。如需区分「字段缺失」和「字段存在但 raw 为 null」，response 层可以用 `json.RawMessage`、显式 presence wrapper，或保证 `Previous` / `Current` 指针在 scalar change 中永远非 nil 且内部 `Raw` 可为 nil。

### 5.4 HTTP API

新增路由：

```text
GET /api/v1/tasks/{taskRef}/audit?workspace=<workspace>&limit=50&offset=0
```

Handler 行为：

1. `requireTaskRef` 解析 path。
2. `scopedService(r, auth.ScopeTaskRead, app.PermissionTaskRead, "")` 建立 task-read 授权上下文。
3. 调 `scoped.ListTaskAudit(taskRef, app.TaskAuditInput{Limit, Offset})`。
4. 响应复用 audit response 结构，新增 `changes` 顶层字段，保留 `payload` 原始 JSON。

响应示例：

```json
{
  "data": [
    {
      "id": 42,
      "actor": {"id":"u1","name":"alice","display_name":"Alice","email":null,"external_ids":[]},
      "action": "task.modify",
      "target_type": "task",
      "target_id": "task-uuid",
      "payload": {"changes":[{"field":"title","previous":"旧","current":"新"}]},
      "changes": [
        {
          "field": "title",
          "kind": "scalar",
          "label_key": "projectWorkbench.taskHistory.field.title",
          "previous": {"raw":"旧","text":"旧"},
          "current": {"raw":"新","text":"新"}
        }
      ],
      "created_at": 1783036800
    }
  ]
}
```

`GET /api/v1/audit` 不改变授权语义，仍要求 `audit:read`。它也可以返回 `changes` 顶层字段，方便未来复用，但前端详情页不依赖 `audit:read`。

### 5.5 数据库索引

`internal/storage/models.go` 给 `AuditLog` 增加复合索引：

```go
WorkspaceID *string `gorm:"index;index:idx_audit_ws_time,priority:1;index:idx_audit_project_time,priority:1;index:idx_audit_target_time,priority:1"`
TargetType  string  `gorm:"index:idx_audit_target_time,priority:2"`
TargetID    string  `gorm:"index:idx_audit_target_time,priority:3"`
CreatedAt   int64   `gorm:"not null;index;index:idx_audit_ws_time,priority:2,sort:desc;index:idx_audit_project_time,priority:3,sort:desc;index:idx_audit_target_time,priority:4,sort:desc"`
```

验收重点：SQLite 和 PostgreSQL AutoMigrate 都能创建该索引；旧 schema 迁移 smoke 不需要手写 SQL migration。

## 6. 前端设计

### 6.1 API client

`web/src/features/workspace/project-workbench/api/task-api.ts`：

- 新增 `TaskFieldChange` / `TaskAuditEntry` 类型。
- 新增 path builder：

```ts
export function taskAuditPath(workspaceSlug: string, taskRef: string): string
```

- 新增请求：

```ts
export function getTaskAudit(
  workspaceSlug: string,
  taskRef: string
): Promise<TaskAuditEntry[]>
```

路径必须编码 taskRef，形状与现有 `taskPath` 保持一致：

```text
/api/v1/tasks/<task-ref>/audit?workspace=<workspace>
```

前端类型用 discriminated union 表达 `kind`，不要把标量的 `previous/current` 或集合的 `added/removed` 都做成可选字段：

```ts
type TaskScalarFieldChange = {
  field: Exclude<TaskChangeField, "assignees" | "tags">
  kind: "scalar"
  label_key: string
  previous: TaskChangeDisplayValue
  current: TaskChangeDisplayValue
}

type TaskSetFieldChange = {
  field: "assignees" | "tags"
  kind: "set"
  label_key: string
  added: TaskChangeDisplayValue[]
  removed: TaskChangeDisplayValue[]
}

type TaskFieldChange = TaskScalarFieldChange | TaskSetFieldChange
```

### 6.2 React Query hook

`web/src/features/workspace/project-workbench/hooks/use-task-detail-data.ts`：

- `taskQueryKeys` 增加 `audit(workspaceSlug, taskRef)`。
- 新增 `useTaskAuditQuery(workspaceSlug, taskRef)`，enabled 条件与 task detail query 一致。

写 mutation 成功后应 invalidate task detail 和 task audit 两个 query key，让用户编辑后能看到新历史。

### 6.3 任务详情页区块

`web/src/features/workspace/project-workbench/task-detail/task-detail-page.tsx`：

- 在 description 和 annotations 之后、links 之前加入 `<TaskChangeHistory>`。
- 桌面布局保持主列内容；移动 tab 可把历史放在 annotations tab 内，避免新增第四个移动 tab。
- 展示格式：
  - 标量：`{actor} 将 {field} 从 {format(previous)} 改为 {format(current)}`
  - 集合：`{actor} 更新了 {field}: 新增 {format(added[])}，移除 {format(removed[])}`
  - actor 缺失时显示系统/未知操作者文案。
  - UserInfo 显示按 `display_name` -> `name` -> `id` 回退；不要在 UI 上直接展示内部 UUID，除非这是最后兜底。
  - 时间用现有前端日期格式 helper；若没有统一 helper，首版用 `Intl.DateTimeFormat`。
  - 不直接展示 raw JSON、UUID、unix 秒或 `null`；raw 用于字段专用 locale 格式化，`text` 只作为兜底。
- 空状态：
  - 请求成功但没有任何带 changes 的行：`暂无字段级变更记录`
  - 403/404/网络错误：显示紧凑错误提示，不阻断任务详情主体。
- description 展示：
  - 列表行只展示纯文本截断，避免整段 Markdown 撑高 timeline。
  - 可用 `<details>` 或现有 Dialog 展开查看 before/after；首版不做并排 diff。

### 6.4 i18n

新增 zh-CN / en-US key，放在 `projectWorkbench.taskHistory` 下：

- `title`
- `empty`
- `unavailable`
- `unknownActor`
- `scalarChange`
- `setChange`
- `added`
- `removed`
- `unset`
- `changedDescription`
- `expandValue`
- `field.assignees`
- `field.tags`
- `field.due`
- `field.priority`
- `field.project`
- `field.title`
- `field.description`

## 7. 测试与验收

### 7.1 后端

- `internal/app/service_test.go` 或新 `internal/app/task_audit_payload_test.go`
  - 修改 assignees 后，audit payload `changes` 包含完整 UserInfo 的 added/removed。
  - 修改 title / description 后，audit payload 包含对应 previous/current。
  - 只改未覆盖字段时，`changes` 为空数组。
  - 清空 due / description 时，payload 和 HTTP view 都保留显式 `current: null` 语义。
- `internal/app/audit_test.go`
  - `ListTaskAudit` 只要求 `task:read`，不要求 `audit:read`。
  - `ListAudit` 仍要求 `audit:read`。
  - 旧 payload 或坏 payload 不导致 view 构造失败。
  - `AuditLogView.Changes` 输出 `kind`、`label_key` 和 `text`，前端不用猜字段类型。
- `internal/storage/audit_repo_test.go`
  - `List` 按 target type/id/action 过滤生效。
- `internal/storage/db_test.go`
  - AutoMigrate 后存在 `idx_audit_target_time`。
- `internal/httpapi/tasks_test.go`
  - `GET /api/v1/tasks/{task_slug}/audit` 用 `task:read` token 成功。
  - 响应包含顶层 `changes` 和原始 `payload`。
  - 数字 working-set ID 仍返回 `task_ref_invalid`。

### 7.2 前端

- `web/src/features/workspace/project-workbench/api/task-api.test.ts`
  - `taskAuditPath` 正确编码 workspace 和 taskRef。
- `web/src/features/workspace/project-workbench/task-detail/task-detail-page.test.tsx`
  - mock `getTaskAudit` 后渲染历史区块。
  - 标量历史渲染为自然语言句子，不展示 raw JSON / unix 秒 / `null`。
  - 集合历史用新增/移除文案渲染 UserInfo 和 tag。
  - 空历史显示占位。
  - audit 请求失败不影响任务详情主体。
- `web/src/features/workspace/project-workbench/hooks/use-task-mutations.test.tsx`
  - 修改 task 后 invalidates task audit query。

### 7.3 验证命令

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
pnpm --dir web test
pnpm --dir web typecheck
pnpm --dir web build
git diff --check
```

## 8. 风险与权衡

- **description payload 体积。** 大段 description 会进入 audit payload。首版接受这个成本，因为需求是可追溯的 before/after；如果后续体积成为问题，再设计 hash/摘要/快照表。
- **人类文案和持久化解耦。** 不把整句中文或英文写进 audit payload，避免后续 i18n 和文案调整需要迁移历史数据；HTTP view 提供 `kind` / `label_key` / `text`，前端用当前语言组合句子。
- **权限边界。** 任务详情历史用 `task:read`，不是 `audit:read`。这是有意选择：该接口只返回单 task 的 `task.modify` 字段变化，不提供 workspace/project 级审计检索能力。
- **旧数据兼容。** 历史 `task.modify` 无 `changes`，前端显示无字段级明细，不迁移。
- **查询索引写入成本。** 新增一个 audit 复合索引，会增加写入维护成本；task 修改频率低于查询收益，可接受。
- **字段范围有限。** 本期不覆盖 UDA / dependency / annotation / link。文档明确非范围，避免把首版历史做成无限扩张的活动流。

## 9. 后续可扩展

- 把 annotation / link / dependency / UDA 变化纳入统一任务活动流。
- CLI / MCP / Remote Client 暴露 task history。
- description 并排 diff 或逐行 diff。
- 对超长字段做摘要 + 展开加载。
