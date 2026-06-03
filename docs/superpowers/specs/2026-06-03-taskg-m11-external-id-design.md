# taskg M11 设计规格：用户外部 ID 绑定

> **给 agentic workers 的要求：** 编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

**目标：** 为 taskg 用户增加外部 ID 绑定能力，让 Agent 能通过 `feishu:ou_xxxxx` 这类标识符指派 assignee、查询用户，并在所有返回用户信息的地方一并返回外部 ID 列表。

**范围策略：** M11 只解决"用户可以绑定外部系统 ID，所有入口都能用它来标识用户"。不引入自动用户创建、不引入 OAuth、不改变现有权限模型。

**需求来源：** 本规格基于 [README.md](/Users/mac/code/projects/dajee/task/README.md)、[ROADMAP.md](/Users/mac/code/projects/dajee/task/ROADMAP.md)、M9 assignee 设计规格、M10 impersonation 设计规格，以及 Agent 通过 MCP 操作 taskg 时的实际身份映射需求。

---

## 1. 当前基础

M10 完成后，taskg 已经具备：

- 完整的 assignee 体系（M9）：多对多关系、`@ref` 写语法、`assignee:<ref>` 查询、全入口覆盖。
- Impersonation（M10）：Agent token 可以代表 workspace 成员操作。
- 用户解析（`internal/app/workspace.go` `resolveUser`）：按 `user_id` → `email` → `name` 顺序解析。

当前缺口：

- Agent 只持有外部系统 ID（如飞书 `ou_xxxxx`），无法将其映射到 taskg 用户。
- `resolveUser` 不识别外部 ID 格式，Agent 无法用 `feishu:ou_xxxxx` 作为 assignee ref。
- 所有返回用户信息的接口（`user info`、`task.get` 的 assignees、`user list`）都不包含外部 ID，Agent 拿到任务后无法反向查找飞书 ID 去发通知。

## 2. 范围与非目标

### 2.1 M11 进入范围

- `user_external_ids` 表：存储用户与外部系统 ID 的绑定关系。
- CLI：`user bind`、`user unbind` 子命令；`user info` / `user list` 显示外部 ID。
- HTTP API：外部 ID 的 CRUD endpoint；所有返回用户的 endpoint 附带外部 ID。
- MCP：`user.bind` / `user.unbind` tool；`task.get` / `task.query` 的 assignee 数据附带外部 ID。
- App service：`resolveUser` 扩展，支持 `provider:value` 格式；assignee ref 解析自动支持外部 ID。
- JSON DTO：`AssigneeInfo` / `UserView` 扩展外部 ID 字段。
- 审计：绑定/解绑操作写入 audit log。
- 文档同步。

### 2.2 M11 明确不做

- 自动用户创建（遇到未绑定的外部 ID 时严格返回 `assignee_not_found`）。
- OAuth / OIDC / 飞书授权流程。
- 外部系统 API 调用（taskg 不主动查飞书通讯录）。
- provider 插件系统（早期硬编码 `feishu`、`email`、`slack`、`wechat` 即可）。
- 批量导入外部 ID（CSV / API batch）。
- 外部 ID 变更同步（外部系统用户 ID 变了，需要手动重新绑定）。

## 3. 核心产品决策

### 3.1 标识符格式：`provider:external_id`

所有外部 ID 采用 `provider:external_id` 格式：

```
feishu:ou_xxxxx
slack:Uxxxxx
email:zhangsan@example.com
wechat:wx_id_xxxxx
```

选择 `:` 的原因：

- 不是 shell 特殊字符（`|` 是管道符）。
- 不和 email 的 `@` 冲突（`email:zhangsan@example.com` 中 `@` 只是值的一部分）。
- 语义类似 URL scheme，表达命名空间。
- 解析简单：按第一个 `:` 做 split，左边 provider，右边是完整外部 ID 值。

provider 值为小写字母开头的字符串，早期支持的常量：

- `feishu`：飞书 open_id
- `email`：邮箱地址（与 `users.email` 字段不同，这是显式的外部身份绑定）
- `slack`：Slack user ID
- `wechat`：企业微信 user ID

### 3.2 一个外部 ID 只能绑定一个用户

`(provider, external_id)` 联合唯一。理由：

- 一个外部 ID 代表一个真实的人。
- 同一个 taskg 实例内，一个真实的人应该只有一个 taskg 用户。
- 如果出现"同一个飞书 ID 想绑两个 taskg 用户"的需求，说明数据模型有问题，应该先修正用户数据，而不是放开唯一约束。

一个用户可以绑定多个外部 ID（飞书 + Slack + 邮箱，多端接入）。

### 3.3 绑定是全局操作，解析受 workspace 约束

- 外部 ID 绑定跨 workspace：`feishu:ou_xxxxx` 绑到 user A 后，所有 workspace 都能通过这个外部 ID 找到 user A。
- 但 assignee 解析仍受 workspace 成员约束：找到 user A 后，如果 A 不是当前 workspace 成员，仍然返回 `assignee_not_member`。
- 绑定/解绑权限：admin/owner 可以给任何用户绑定；普通用户只能给自己绑定。

### 3.4 严格模式：不自动创建用户

M11 采用严格模式：遇到未绑定的外部 ID，返回 `assignee_not_found`，不悄悄创建幽灵用户。

自动创建用户涉及命名规则、workspace 成员关系、权限边界等复杂问题，留给后续 milestone。

### 3.5 `resolveUser` 扩展优先级

现有解析顺序（M9）：

1. 精确匹配 `user_id`（UUID 格式）
2. 精确匹配 `email`
3. 精确匹配 `name`

M11 扩展为：

1. 精确匹配 `user_id`（UUID 格式）
2. **如果 ref 包含 `:`，尝试按 `provider:external_id` 解析**
3. 精确匹配 `email`
4. 精确匹配 `name`

将外部 ID 解析放在 email/name 之前，是因为：

- `provider:value` 格式本身是确定性的，不存在歧义。
- 优先用精确的外部 ID 匹配，避免碰巧和某个用户的 name 相同导致误匹配。
- UUID 匹配仍然最优先，保证内部引用的稳定性。

## 4. 数据模型与外部契约

### 4.1 SQLite 关系模型

新增表：

```sql
CREATE TABLE user_external_ids (
  id           TEXT NOT NULL PRIMARY KEY,
  user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  provider     TEXT NOT NULL,
  external_id  TEXT NOT NULL,
  created_at   INTEGER NOT NULL
);

CREATE UNIQUE INDEX idx_user_ext_id_provider_value ON user_external_ids(provider, external_id);
CREATE INDEX idx_user_ext_id_user ON user_external_ids(user_id);
```

### 4.2 GORM model

```go
type UserExternalID struct {
    ID          string `gorm:"primaryKey"`
    UserID      string `gorm:"not null;index:idx_user_ext_id_user"`
    Provider    string `gorm:"not null;uniqueIndex:idx_user_ext_id_provider_value,priority:1"`
    ExternalID  string `gorm:"not null;uniqueIndex:idx_user_ext_id_provider_value,priority:2"`
    CreatedAt   int64  `gorm:"not null"`
}
```

### 4.3 领域模型扩展

`internal/task/model.go`：

```go
type ExternalIDInfo struct {
    Provider   string `json:"provider"`
    ExternalID string `json:"external_id"`
}

type AssigneeInfo struct {
    UserID      string
    Name        string
    Email       *string
    ExternalIDs []ExternalIDInfo  // 新增
}
```

`internal/app/workspace.go`：

```go
type UserView struct {
    ID                 string
    Name               string
    Email              *string
    DefaultWorkspaceID *string
    ExternalIDs        []ExternalIDInfo  // 新增
    Active             bool
    CreatedAt          int64
    ModifiedAt         int64
}
```

### 4.4 JSON DTO 扩展

`internal/task/json.go` `JSONAssignee`：

```json
{
  "user_id": "a1b2c3...",
  "name": "张三",
  "email": "zhangsan@example.com",
  "external_ids": [
    {"provider": "feishu", "external_id": "ou_xxxxx"},
    {"provider": "email", "external_id": "zhangsan@example.com"}
  ]
}
```

`external_ids` 为空时省略（`omitempty`），保持向后兼容。

HTTP API `userResponse`：

```json
{
  "id": "a1b2c3...",
  "name": "张三",
  "email": "zhangsan@example.com",
  "external_ids": [
    {"provider": "feishu", "external_id": "ou_xxxxx"}
  ]
}
```

### 4.5 HTTP API 契约

新增 endpoint：

- `POST /api/v1/users/{user}/external-ids`
  - 请求体：`{"provider": "feishu", "external_id": "ou_xxxxx"}`
  - 简写格式：也接受 `{"external_id": "feishu:ou_xxxxx"}`，自动拆分。
  - 返回：创建的外部 ID 对象。
- `DELETE /api/v1/users/{user}/external-ids/{provider}/{external_id}`
  - 返回：204 No Content。
- `GET /api/v1/users/{user}/external-ids`
  - 返回：外部 ID 列表。

修改 endpoint（返回值扩展，不含 breaking change）：

- `GET /api/v1/users`：每个用户对象增加 `external_ids`。
- `GET /api/v1/users/{user}`：增加 `external_ids`。
- 所有返回 task 的 endpoint：`assignees` 中每个对象增加 `external_ids`。

### 4.6 MCP 契约

新增 tool：

- `user.bind`：`{"user": "alice", "provider": "feishu", "external_id": "ou_xxxxx"}`
  - 也接受简写：`{"user": "alice", "external_id": "feishu:ou_xxxxx"}`
- `user.unbind`：`{"user": "alice", "external_id": "feishu:ou_xxxxx"}`

修改 tool（返回值扩展）：

- `task.get` / `task.query`：assignee 数据增加 `external_ids`。

### 4.7 Hook payload 契约

`task.created` / `task.modified` / `task.completed` / `task.deleted` 的 `data.task.assignees` 自动因 `JSONAssignee` 扩展而携带 `external_ids`。

### 4.8 JSON import/export

import 兼容：

- `assignees` 的 ref 支持 `feishu:ou_xxxxx` 格式，走统一 `resolveAssigneeRef` 解析。
- 现有的 `user_id` / `email` / `name` ref 继续可用。

export 行为：

- `task.ToJSON()` 的 `assignees` 自动携带 `external_ids`。

### 4.9 CLI 契约

新增子命令：

```bash
taskg user bind feishu:ou_xxxxx                    # 绑定到当前用户
taskg user bind feishu:ou_xxxxx --user alice       # admin/owner 给其他用户绑定
taskg user unbind feishu:ou_xxxxx                  # 解绑
taskg user unbind feishu:ou_xxxxx --user alice     # admin/owner 给其他用户解绑
```

已有命令行为变更：

- `taskg user info`：human 输出增加外部 ID 列表行。
- `taskg user list`：human 输出可选显示外部 ID（`--verbose` 或默认不显示，`--json` 一定包含）。
- `taskg task info`：assignee 行如果有关联外部 ID，可选择性显示（首版不强制，JSON 输出一定包含）。
- assignee ref 语法自动支持：`taskg add "做这件事" @feishu:ou_xxxxx`
- 查询语法自动支持：`taskg list assignee:feishu:ou_xxxxx`

## 5. 分层影响

### 5.1 Storage（`internal/storage/sqlite`）

- `models.go`：新增 `UserExternalID` struct。
- `db.go`：AutoMigrate 加入 `UserExternalID`。
- `user_repo.go`：
  - 新增 `GetByExternalID(provider, externalID string) (User, error)`
  - 新增 `ListExternalIDsByUser(userID string) ([]UserExternalID, error)`
  - 新增 `CreateExternalID(externalID UserExternalID) (UserExternalID, error)`
  - 新增 `DeleteExternalID(userID, provider, externalID string) error`
- `task_repo.go`：`loadAssigneeUsers` 扩展，批量加载用户的外部 ID。

### 5.2 App service（`internal/app`）

- `workspace.go`：
  - `resolveUser` 扩展：在 UUID 匹配后、email 匹配前，检测 `:` 并查外部 ID。
  - 新增 `BindExternalID(userID, provider, externalID string) error`
  - 新增 `UnbindExternalID(userID, provider, externalID string) error`
  - `UserView` 增加 `ExternalIDs` 字段。
  - `userViewFromRow` 扩展，hydrate 外部 ID。
  - `ListUsers` / `UserInfo` 扩展返回外部 ID。
- `service.go`：
  - `resolveAssigneeRef` 的底层 `resolveUser` 自动获得外部 ID 解析能力，不需要单独改。
  - assignee hydration 扩展，附带外部 ID。

### 5.3 Task domain（`internal/task`）

- `model.go`：`AssigneeInfo` 增加 `ExternalIDs []ExternalIDInfo`。
- `json.go`：`JSONAssignee` 增加 `ExternalIDs` 字段，export/import 相应扩展。

### 5.4 CLI（`internal/cli`）

- `user.go`：新增 `bind`、`unbind` 子命令。`info` / `list` 输出扩展。

### 5.5 HTTP API（`internal/httpapi`）

- `users.go`：新增外部 ID CRUD handler，`userResponse` 扩展。
- `router.go`：注册新路由。

### 5.6 MCP（`internal/mcpserver`）

- 新增 `user.bind` / `user.unbind` tool。
- `taskData()` 的 assignee 构建扩展。
- Schema golden file 更新。

### 5.7 Remote client（`internal/remote`）

- 新增外部 ID CRUD 方法。
- 用户相关 response struct 扩展。

### 5.8 文档

至少需要同步更新：

- `README.md`
- `ROADMAP.md`
- `docs/manual/mcp.md`
- `docs/manual/remote-cli-and-api.md`
- `docs/openapi/taskg-v1.yaml`
- `docs/requirements.md`

## 6. 验收标准

M11 完成时，至少要满足：

- `taskg user bind feishu:ou_xxxxx` 能绑定外部 ID，`taskg user unbind feishu:ou_xxxxx` 能解绑。
- `taskg user info` human 输出显示外部 ID 列表。
- `taskg user info --json` 返回 `external_ids` 数组。
- `taskg user list --json` 每个用户包含 `external_ids`。
- `taskg add "做这件事" @feishu:ou_xxxxx` 能创建带外部 ID assignee 的任务。
- `taskg list assignee:feishu:ou_xxxxx` 能按外部 ID 查询任务。
- JSON export 的 assignee 包含 `external_ids`；JSON import 支持 `feishu:ou_xxxxx` 作为 ref。
- HTTP API `POST /api/v1/users/{user}/external-ids` 能绑定，`DELETE` 能解绑，`GET` 能列出。
- HTTP API 所有返回用户/assignee 的 endpoint 附带 `external_ids`。
- MCP `user.bind` / `user.unbind` 可用；`task.get` / `task.query` 的 assignee 包含 `external_ids`。
- 同一个 `(provider, external_id)` 不能绑两次；解绑后可以重新绑定。
- 绑定/解绑操作写入 audit log。
- admin/owner 可以给其他用户绑定；普通用户只能给自己绑定。
- 未绑定的外部 ID 在 assignee 解析时返回 `assignee_not_found`。
- Hook payload 的 `data.task.assignees` 自动携带 `external_ids`。
- `go test ./...`、`CGO_ENABLED=0 go test ./...`、`CGO_ENABLED=0 go build ./cmd/taskg` 通过。

## 7. 明确不做

- 不自动创建用户。
- 不引入 OAuth / OIDC。
- 不调用外部系统 API。
- 不做 provider 插件系统。
- 不做批量导入。
- 不做外部 ID 变更同步。
