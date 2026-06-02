# taskg M9 设计规格

> **给 agentic workers 的要求：** 编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

**目标：** 为 `taskg` 增加任务多 assignee 能力，让本地 CLI、远程 CLI、HTTP API、MCP Server 和 Hook payload 都能在同一套 workspace 权限边界内读写、查询和导出任务执行者信息。

**范围策略：** M9 只解决“任务可以有多个执行者，并且这一事实能在所有现有任务入口中稳定表达”。M9 不借机扩成主责/协作者模型、不引入 assignee 通知编排，也不改动现有权限模型和 urgency 公式。能复用现有 `task.add`、`task.modify`、`task.query`、JSON export/import、Hook payload 的地方，优先复用，不平行造新接口。

**需求来源：** 本规格基于 [README.md](/Users/mac/code/projects/dajee/task/README.md)、[ROADMAP.md](/Users/mac/code/projects/dajee/task/ROADMAP.md)、[docs/requirements.md](/Users/mac/code/projects/dajee/task/docs/requirements.md)、当前 M7/M8 后代码结构，以及本轮关于“多 assignee 应是 task 基础模型能力，而不是新的通知或权限系统”的范围约束。

---

## 1. 当前基础

M8 完成后，`taskg` 已经具备：

- 本地 CLI、远程 CLI、HTTP API、MCP Server、server-side Hook。
- `workspace -> project -> task` 的严格 scope。
- `task.add` / `task.modify` / `task.query` / `task.get` 一套共用的 `internal/app` service。
- JSON export/import、Hook payload、MCP `taskData()` 都基于统一 `task.Task` / `task.JSONTask` 视图。

当前缺口也很明确：

- `docs/requirements.md` 里曾出现 `assignee_user_id` 单值字段设想，但当前实现中并没有 assignee 模型。
- 一个任务只能通过描述、tag、project 间接表达“谁在执行”，系统本身无法查询“某个人当前负责哪些任务”。
- CLI、HTTP API、MCP、Hook payload 现在都无法稳定输出 assignee 列表。

M9 的工作不是新增任务系统，而是在现有 task 主模型上补齐 assignee 这一缺失维度。

## 2. 范围与非目标

### 2.1 M9 进入范围

M9 必须进入范围的能力只有五类：

- task 与 user 的多对多 assignee 关系。
- `@ref` / `+@ref` / `-@ref` 与 `assignee:<ref>` 查询语法。
- assignee 在 JSON export/import、HTTP API、MCP、Hook payload 中的稳定数据契约。
- 在当前 effective workspace 内解析 assignee ref，并复用现有 `task:read` / `task:write` 权限边界。
- 与上述能力直接相关的 README / manual / roadmap / requirements 文档同步。

### 2.2 M9 明确不做

以下能力明确不进入 M9：

- 主 assignee / secondary assignee / follower 区分。
- assignee 级权限，或“被指派即自动获得写权限”。
- assignee 通知、提醒、自动消息、adapter 编排。
- urgency 公式变更。
- 跨 workspace 的全局 “assignee:me” 视图。
- 为列表报表新增一整套 assignee 列配置系统。

### 2.3 M9 首版的人类输出范围

M9 首版只要求：

- `task info` 的 human 输出补充 `Assignees` 行。
- JSON、HTTP API、MCP、Hook payload 补充结构化 assignee 数据。

`list` / `next` / report table 的默认列保持不变，不在 M9 首版里扩为通用 assignee 列配置系统，避免范围失控。

## 3. 核心产品决策

### 3.1 assignee 采用多对多关联，放弃单值 FK 方案

M9 使用独立的 `task_assignees` 关联表表达任务与用户的多对多关系，不在 `tasks` 表上新增 `assignee_user_id`。

原因：

- 现实协作里一个任务经常由多人共同执行，单值 FK 很快失真。
- 现有 task 读写路径已经接受 tag/depends/UDA 这类集合字段，扩成多对多比引入新的“主责语义”更自然。
- `docs/requirements.md` 中的单值方案可以在 M9 里正式标记为“被多对多实现取代”。

### 3.2 `user_id` 是内部规范值，输入允许人类可读 ref

系统内落库与查询的规范形态始终是 `user_id`。但为了让 CLI、API、MCP 和 import 都好用，M9 的“输入 ref”允许是：

- `user_id`
- `email`
- `name`

解析顺序固定为：

1. 精确匹配 `user_id`
2. 精确匹配 `email`
3. 精确匹配 `name`

解析边界：

- 本地模式：在本地 user 集合中解析。
- 服务端 / remote / HTTP / MCP 模式：只在当前 effective workspace 的成员中解析。
- 这是一条硬约束：assign 目标必须属于当前 task 所在 workspace；即使系统里存在同名用户、或该用户属于别的 workspace，也不能跨 workspace 指派。
- 找不到目标时直接报错，不静默忽略。

错误语义固定为：

- `assignee_not_found`：系统内找不到该 ref 对应的 user。
- `assignee_not_member`：能解析到 user，但该 user 不属于当前 effective workspace。

### 3.3 CLI 语法与现有 tag 修改语义对称

M9 复用当前 CLI “描述 + 修改 token” 风格：

```bash
taskg add "ship m9 docs" @alice @bob
taskg 1 modify +@carol -@alice
taskg list assignee:alice
taskg next assignee:me
```

规则：

- `@ref` 仅在 `add` 的 modification token 中表示初始 assignee。
- `+@ref` / `-@ref` 仅在 `modify` 中表示增减 assignee。
- `assignee:me` 是查询语法，不是写语法。

M9 不额外引入 `assign` 子命令，也不新增 `task.assign` 专用事件。

### 3.4 结构化读接口统一返回对象数组

对机器友好的读接口不能只返回裸 UUID。M9 统一返回：

```json
"assignees": [
  {
    "user_id": "u_123",
    "name": "alice",
    "email": "alice@example.com"
  }
]
```

约束：

- `email` 为可选字段，用户没有邮箱时可省略。
- 顺序稳定，按 `name`、`email`、`user_id` 的可读排序输出，避免同一任务无意义抖动。
- Hook payload、JSON export、HTTP API、MCP `taskData()` 复用同一 `task.JSONTask` 形态，不各自发明字段名。

### 3.5 `assignee:me` 只在当前 effective workspace 生效

M9 的 `assignee:me` 展开为“当前 runtime actor 在当前 effective workspace 中的稳定 user_id”。它不是跨 workspace 聚合语义。

这样做的原因：

- 当前 `taskg` 的查询、权限、remote、HTTP、MCP 都是 workspace-scoped。
- 如果在 M9 首版就引入跨 workspace “我的任务” 视图，会把 request scope、token allowlist、audit 语义一起拉进来。

跨 workspace 个人视图如果要做，应在后续 milestone 明确设计，而不是偷偷塞进 assignee 功能里。

### 3.6 assignee 变化仍然是普通 task 修改

M9 不新增 `task.assigned` 或 `task.unassigned` 事件类型。assignee 增减：

- 审计上仍然走现有 `task.add` / `task.modify` / `task.import`。
- Hook 上仍然走 `task.created` / `task.modified` / `task.completed` / `task.deleted`。
- 只是这些事件里的 task snapshot 会新增 `assignees` 字段。

这样可以避免在 M9 同时扩展事件矩阵和订阅面。

## 4. 数据模型与外部契约

### 4.1 SQLite 关系模型

新增关联表：

```sql
CREATE TABLE task_assignees (
  task_uuid TEXT NOT NULL REFERENCES tasks(uuid) ON DELETE CASCADE,
  user_id   TEXT NOT NULL REFERENCES users(id),
  PRIMARY KEY (task_uuid, user_id)
);

CREATE INDEX idx_task_assignees_user_id ON task_assignees(user_id);
```

约束：

- `task_uuid + user_id` 唯一，天然去重。
- 任务删除时 assignee 关联级联删除。
- `users` 仍然是 assignee 的唯一来源，不复制用户名快照进关联表。

### 4.2 领域模型

`internal/task/model.go` 中的 `task.Task` 增加：

```go
type AssigneeInfo struct {
    UserID string
    Name   string
    Email  *string
}

type Task struct {
    ...
    Assignees []AssigneeInfo
}
```

这里的 `AssigneeInfo` 不是新的顶层实体，只是 task snapshot 的组成部分。

### 4.3 JSON task 形态

`task.JSONTask` 增加：

```json
{
  "uuid": "...",
  "description": "...",
  "assignees": [
    {
      "user_id": "u_123",
      "name": "alice",
      "email": "alice@example.com"
    }
  ]
}
```

import 兼容要求：

- 没有 `assignees` 字段时，保持兼容，表示“不触碰 assignee”。
- `assignees: []` 表示显式清空 assignee。
- 为了兼容手工 payload 和旧脚本，允许 `assignees` 也写成 `["u_123", "alice@example.com"]` 这样的字符串数组，按统一 ref 解析规则导入。

### 4.4 HTTP API 与 remote CLI 契约

现有 HTTP task 路径继续保持：

- `POST /api/v1/tasks`
- `PATCH /api/v1/tasks/{taskID}`
- `GET /api/v1/tasks`
- `GET /api/v1/tasks/{taskID}`

新增字段：

- `POST /api/v1/tasks` 请求体增加 `assignees: []string`
- `PATCH /api/v1/tasks/{taskID}` 请求体增加：
  - `assignees: []string` 表示追加
  - `remove_assignees: []string`
  - `clear_assignees: boolean`
- 返回 task JSON 时补充 `assignees` 对象数组

remote CLI client 只是 HTTP API 的 typed client，因此字段名与行为必须完全一致。

### 4.5 MCP 契约

MCP 沿用当前工具名，不新增新工具：

- `task.add` 增加 `assignees []string`
- `task.modify` 增加 `assignees []string`、`remove_assignees []string`
- `task.modify.clear` 继续复用现有 `clear` 数组语义，并允许 `"assignees"`
- `task.get` / `task.query` 返回的 task 数据补充 `assignees`
- `rendered` 文本在 `task.get` 场景下展示 assignee 行

### 4.6 Hook payload 契约

Hook 事件 envelope 不变。变化只在 `data.task`：

- `task.created`
- `task.modified`
- `task.completed`
- `task.deleted`

这些事件里的 `data.task` 都会因为 `task.ToJSON()` 扩展而自动携带 `assignees` 字段。

## 5. 分层影响

### 5.1 Query / parser

`internal/query` 需要补齐两类入口：

- filter parser 新增 `assignee:<ref>`
- add / modify token parser 新增 `@ref`、`+@ref`、`-@ref`

其中 `assignee:me` 不是 parser 自己解析到 user_id，而是在 app runtime 中被替换成当前 actor 的稳定 user_id。

### 5.2 Storage

`internal/storage/sqlite` 需要承担：

- `TaskAssignee` model 与 migration。
- `TaskRepository.Create` / `Update` 的关联表写入。
- `GetByUUID` / `List` 的 assignee hydration。
- `query_scope.go` 里 `assignee` predicate 的 SQL 编译。

实现上优先用“预加载关联 + 批量查 users + 回填 domain.Task”的方式，避免在 task 查询主路径里引入过度复杂的多表扫描。

### 5.3 App service

`internal/app/service.go` 需要承担三件事：

- Add / Modify 前把 assignee ref 解析成稳定 `user_id`
- 在服务端模式下校验 assignee 属于当前 workspace 成员，并拒绝任何跨 workspace assign
- 在 `assignee:me` 查询场景下，把逻辑值 `me` 展开为 runtime actor

这里的错误码也要统一复用上面的约定：缺用户报 `assignee_not_found`，跨 workspace 报 `assignee_not_member`。

import 也必须走同一套 ref 解析，而不是在 CLI 或 HTTP handler 层自己解释字符串。

### 5.4 CLI / render

CLI 需要补齐：

- `add` / target-style `modify` 的 assignee token 解析与远程透传
- `task info` 的 human 输出
- `--json` 输出继续完全依赖 `task.ToJSON()`

M9 不要求修改 `list` / `next` 表格列定义。

### 5.5 HTTP API / remote client

`internal/httpapi/tasks.go`、`internal/remote/task.go`、`docs/openapi/taskg-v1.yaml` 需要同步更新，保证：

- handler request struct
- remote typed client struct
- OpenAPI schema

三者字段名完全一致，避免某个入口偷偷漂移。

### 5.6 MCP

`internal/mcpserver` 需要同步更新：

- tool input struct
- `applyClearFields`
- `taskData()` / `tasksData()`
- schema golden files

MCP 仍然复用 `internal/app`，不单独解析 assignee 权限或成员关系。

### 5.7 文档

M9 需要同步的文档至少包括：

- `README.md`
- `ROADMAP.md`
- `docs/manual/tasks.md`
- `docs/manual/mcp.md`
- `docs/manual/remote-cli-and-api.md`
- `docs/requirements.md`

否则代码与手册会立刻分叉。

## 6. 验收标准

M9 完成时，至少要满足：

- `taskg add "ship docs" @alice @bob` 能创建带 assignees 的任务。
- `taskg 1 modify +@carol -@alice` 能增减 assignee。
- `taskg list assignee:alice` 和 `taskg next assignee:me` 能稳定过滤。
- `taskg info <target>` 的 human 输出能看到 assignee 列表。
- JSON export 包含 `assignees` 对象数组；JSON import 支持对象数组、字符串数组和显式空数组。
- HTTP API / remote CLI / MCP 三个入口都能写入 assignees，并在读接口返回相同字段。
- Hook payload 的 `data.task.assignees` 可见。
- `go test ./...`、`CGO_ENABLED=0 go test ./...`、`CGO_ENABLED=0 go build ./cmd/taskg` 通过。

## 7. 明确不做

- 不区分主 assignee 与协作者。
- 不因为被指派而自动放宽 `task:write` 权限。
- 不新增 assignee 专用 audit action 或 Hook event type。
- 不把 `assignee:me` 扩展成跨 workspace 个人任务视图。
- 不在 M9 首版里改造 `list` / `next` / report 的列系统。
