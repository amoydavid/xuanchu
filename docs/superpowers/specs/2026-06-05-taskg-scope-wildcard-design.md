# Token Scope 通配符与 Token 修改

## 背景

xuanchu 当前有 16 个 token scope（`task:read`、`task:write`、`project:read` 等），创建 token 时必须逐个列举。一个全权限 admin token 需要 15 个 scope（不含 `impersonate`），写起来冗长且容易遗漏或拼错。

同时，新增 scope 后没有统一的地方查看所有可用 scope，用户只能翻文档或源码。token 创建后无法修改 scope、名称、过期时间或 workspace/project 可见范围，只能撤销重建。

## 目标

1. `--scope` 支持通配符，减少输入负担。
2. 新增 `xuanchu scope list` 命令，输出所有有效 scope。
3. 新增 `xuanchu token modify` 命令和 `PATCH /api/v1/tokens/{id}` 接口，支持修改 token 的 scope、名称、过期时间和 workspace/project 可见范围。
4. 未来新增 scope 时，`scope list` 自动反映，通配符自动包含。
5. 运行时行为不变，存储和校验逻辑不变。

## 当前状态

### 有效 scope 列表

定义在 `internal/auth/scope.go` 的 `allowedScopes`：

| Scope | 说明 |
|---|---|
| `task:read` | 读任务 |
| `task:write` | 写任务 |
| `project:read` | 读项目 |
| `project:write` | 写项目 |
| `context:read` | 读 context |
| `context:write` | 写 context |
| `config:read` | 读配置 |
| `config:write` | 写配置 |
| `workspace:read` | 读 workspace |
| `workspace:write` | 写 workspace、管理成员 |
| `audit:read` | 读审计日志 |
| `token:read` | 列出 token |
| `token:write` | 创建/撤销 token |
| `hook:read` | 读 hook |
| `hook:write` | 写 hook |
| `impersonate` | 以他人身份操作（仅 agent token，仅 admin/owner） |

### 现有 scope 校验流程

`ParseScopes(values []string)` 逐条检查 `allowedScopes` map，不在就报 `token_scope_invalid`。token 创建时展开为 JSON 数组存储，运行时 `Has()` 逐条检查。

当前 token 只支持 create/list/revoke，不支持 scope 更新。

### Token 修改的当前空白

修改 token 需要撤销重建，操作成本高且会中断依赖该 token 的服务。常见的修改需求包括：缩小 scope（最小权限轮换）、续期、调整 workspace/project 可见范围、重命名。

## 设计

### 1. 通配符语法

| 通配符 | 含义 | 示例 |
|---|---|---|
| `resource:*` | 该 resource 的所有 action | `task:*` → `task:read,task:write` |
| `*:action` | 所有 resource 的该 action | `*:read` → 全部 `:read` scope |
| `*` | 全部 scope | `*` → 16 个 scope |

规则：

- 通配符仅支持上述三种模式，不支持 `task:re*` 等部分匹配。
- `resource:*` 匹配 `allowedScopes` 中所有以 `resource:` 开头的 scope。
- `*:action` 匹配所有以 `:action` 结尾的 scope。
- `*` 匹配全部 scope。
- 不含通配符的 scope 保持原有校验逻辑。
- 多个通配符和具体 scope 可以混合使用，展开后去重。

展开后按 token type 剔除不允许的 scope，而不是报错：

- PAT：展开结果自动剔除 `impersonate`。因此 `--scope '*' --type pat` 等价于 15 个 scope。
- Agent：保留全部展开结果，包含 `impersonate`（但后续仍需 admin/owner 角色才能实际创建）。
- 这意味着 `xuanchu token create admin --scope '*' --type pat` 能正常创建全权限 PAT。

### 2. 展开时机

通配符在 `ParseScopes()` 内部、校验之前展开。展开后得到的具体 scope 列表就是后续存储和运行时的 scope 列表。

这意味着：

- 存储不变：`ScopesJSON` 仍然是具体 scope 的 JSON 数组。
- 运行时不变：`ScopeSet.Has()` / `HasCapability()` 不需要改。
- 父子 token 校验不变：`enforceTokenCreateLimit()` 比较的是展开后的 scope。
- 未来新增 scope 时，已创建的 token 不会自动获得新 scope（最小权限）。

### 3. `allowedScopes` 改为有序注册

把 `map[string]struct{}` 改为有序 slice + lookup map：

```go
var scopeRegistry = []string{
    "task:read", "task:write",
    "project:read", "project:write",
    "context:read", "context:write",
    "config:read", "config:write",
    "workspace:read", "workspace:write",
    "audit:read",
    "token:read", "token:write",
    "hook:read", "hook:write",
    "impersonate",
}

var scopeLookup map[string]struct{}
```

新增 scope 只需在 `scopeRegistry` 中添加一项，`scopeLookup` 和 `scope list` 自动反映。

### 4. `xuanchu scope list` 命令

```
xuanchu scope list
xuanchu scope list --json
```

Human 输出按 resource 分组：

```text
RESOURCE    ACTIONS
task        read, write
project     read, write
context     read, write
config      read, write
workspace   read, write
audit       read
token       read, write
hook        read, write

STANDALONE
impersonate
```

JSON 输出：

```json
[
  {"scope": "task:read", "resource": "task", "action": "read"},
  {"scope": "task:write", "resource": "task", "action": "write"},
  ...
  {"scope": "impersonate", "resource": "", "action": "impersonate"}
]
```

不需要 `--json` 时以 human 格式输出。

### 5. Token 修改

#### 可修改字段

| 字段 | 允许修改 | 说明 |
|---|---|---|
| scope | 是 | 支持通配符 |
| name | 是 | 重命名 |
| expires_at | 是 | 续期或缩短，通过 `--expires-in` 指定新的相对时长 |
| workspace_ids | 是 | 通过 `--workspace-id` 整体替换 |
| project_ids | 是 | 通过 `--project`/`--project-id` 整体替换 |
| type | 否 | pat↔agent 会改变安全语义 |
| token hash / prefix | 否 | 换 secret = 重建 |

#### CLI

```bash
xuanchu token modify <id-or-prefix> [--scope ...] [--name ...] [--expires-in ...] [--workspace-id ...] [--project ...] [--project-id ...]
```

至多提供一个修改项。不提供任何修改项时不报错，直接返回当前 token 信息（等价于 info）。

#### `--expires-in` 语义

- 正值（如 `720h`）：设置过期时间为 `now + duration`。
- `0`：移除过期时间，token 永不过期。
- 负值：报错。

#### 远程 CLI

远程模式自动转发到 `PATCH /api/v1/tokens/{id}`：

```bash
xuanchu --server https://xuanchu.example.com --token "$XUANCHU_TOKEN" token modify abc123 --scope '*:read'
```

#### MCP

token modify 不暴露为 MCP tool。token 管理是 admin 操作，不属于 Agent 日常任务。

#### HTTP API

```
PATCH /api/v1/tokens/{id}
Authorization: Bearer <token>
Content-Type: application/json

{
  "scopes": ["task:read", "task:write"],
  "name": "renamed-token",
  "expires_in": "720h",
  "workspace_ids": ["<uuid>"],
  "project_ids": ["<uuid>"]
}
```

所有字段可选，至少提供一个。未提供的字段保持不变。

#### 权限约束

**本地 CLI（直接操作数据库）：**

- 需要 admin 或 owner 角色。
- scope 修改不受操作者自身 token scope 限制。
- `impersonate` scope 的添加仍受 admin/owner 限制。
- PAT 不允许添加 `impersonate` scope。

**远程 API（通过 token 操作）：**

- 请求者需要 `token:write` capability。
- 新 scope 必须是请求者自身 scope 的子集（复用 `enforceTokenCreateLimit` 同一逻辑）。
- 新 workspace_ids 必须是请求者自身可见 workspace 的子集。
- 新 project_ids 必须是请求者自身可见 project 的子集。
- 操作者只能修改自己创建的 token（同 user_id），admin/owner 可修改任意 token。

**通用约束：**

- 已撤销的 token 不允许修改，返回 `token_revoked`。
- 已过期的 token 不允许修改，返回 `token_expired`。
- `--expires-in` 设置为已过去的时间点等同于让 token 立即过期。
- workspace_ids / project_ids 是整体替换，不是增量追加。如需追加，先列出当前值再合在一起传。

#### App 层

`Service.ModifyToken(input ModifyTokenInput) (TokenView, error)`

```go
type ModifyTokenInput struct {
    TokenRef    string
    Scopes      []string // nil 表示不改
    Name        *string  // nil 表示不改
    ExpiresIn   *string  // Go duration string，nil 表示不改，"0" 表示永不过期
    WorkspaceIDs []string // nil 表示不改
    ProjectIDs  []string // nil 表示不改
}
```

流程：

1. 解析 token ref，查找 token。
2. 检查 token 未撤销且未过期。
3. 如果提供了 scopes：调用 `ParseScopes()`（支持通配符）展开并校验。展开后按目标 token 的 type 剔除不允许的 scope（PAT 剔除 `impersonate`）。
4. 如果远程模式：校验请求者权限（scope 子集、workspace 子集、project 子集、owner 约束）。
5. 如果提供了 `ExpiresIn`：解析 duration，`0` 表示移除过期时间，正值设置 `now + duration`，负值报错。
6. 合并修改项，写入数据库。
7. 写审计记录，action 为 `token.modified`，payload 包含修改前后 diff。
8. 返回更新后的 `TokenView`。

注意：scope 校验函数拆分为 `ValidateTokenScopes(scopes, tokenType)`，只校验 scope 字符串合法性和 type 约束，不检查 workspace 必填性。workspace 必填性仅在 `CreateToken` 中检查。

#### Storage 层

新增 `UpdateToken(id string, updates TokenUpdates) error`，只更新非零值字段。

#### 审计

token 修改写入 `audit_logs`，action 为 `token.modified`，payload 包含修改前后的 diff。

### 6. CLI 使用示例

```bash
# 全权限 admin token
xuanchu token create admin --type pat --scope '*' --expires-in 720h

# 只读 token
xuanchu token create viewer --type agent --scope '*:read' --project my-project --expires-in 720h

# task 全权限 + 项目只读
xuanchu token create task-worker --type agent --scope 'task:*,project:read' --expires-in 720h

# 查看可用 scope
xuanchu scope list

# 缩小 token scope
xuanchu token modify abc123 --scope 'task:read,project:read'

# 续期
xuanchu token modify abc123 --expires-in 720h

# 重命名
xuanchu token modify abc123 --name "production-agent"

# 通配符也可用于 modify
xuanchu token modify abc123 --scope '*:read'
```

## 不改什么

- token 运行时校验、MCP tool 权限检查 —— 完全不变。
- `impersonate` scope 对 PAT 的限制 —— 不变（`*` 展开后包含 `impersonate`，但 PAT 的 type 校验仍然拒绝它）。
- `impersonate` scope 对 admin/owner 的限制 —— 不变。
- 父子 token scope 子集校验 —— 不变（比较展开后的列表）。
- token type —— 不允许通过 modify 改变类型。
- token secret —— 不允许通过 modify 换 secret，需要撤销重建。

## 验收标准

### Scope 通配符

- `xuanchu token create admin --scope '*' --type pat` 能创建全权限 PAT（不含 `impersonate`）。
- `xuanchu token create agent --scope '*' --type agent` 能创建全权限 agent token（含 `impersonate`，需 admin/owner）。
- `xuanchu token create viewer --scope '*:read'` 能创建只读 token。
- `xuanchu token create worker --scope 'task:*'` 能创建 task 全权限 token。
- 无效通配符（如 `foo:*`）报 `token_scope_invalid`。
- `xuanchu scope list` 输出所有有效 scope。
- `xuanchu scope list --json` 输出结构化 JSON。
- 所有现有 scope 校验测试通过。

### Token 修改

- `xuanchu token modify <ref> --scope 'task:*'` 能修改 scope。
- `xuanchu token modify <ref> --name "new-name"` 能重命名。
- `xuanchu token modify <ref> --expires-in 720h` 能续期。
- `xuanchu token modify <ref> --project new-project` 能替换 project allowlist。
- 修改已撤销 token 返回 `token_revoked`。
- 修改已过期 token 返回 `token_expired`。
- 远程模式下 scope 扩张受请求者自身 scope 约束。
- token 修改写入 audit log。
- `PATCH /api/v1/tokens/{id}` 行为与 CLI 一致。

### 全局

- `go test ./...`、`CGO_ENABLED=0 go test ./...`、`CGO_ENABLED=0 go build ./cmd/xuanchu` 通过。

## 影响范围

- `internal/auth/scope.go` — 主要改动：有序注册、`ParseScopes()` 通配符展开。
- `internal/auth/scope_test.go` — 新增通配符测试。
- `internal/auth/token.go` — `ValidateTokenCreate()` 改为 `ValidateTokenScopes()` 复用于 modify。
- `internal/app/token.go` — 新增 `ModifyToken()`、`ModifyTokenInput`。
- `internal/app/token_test.go` — 新增 modify 测试。
- `internal/storage/token_repo.go` — 新增 `UpdateToken()`。
- `internal/cli/scope.go` — 新增：`scope list` 命令。
- `internal/cli/token.go` — 新增：`token modify` 子命令。
- `internal/cli/root.go` — 注册 `scope` 子命令。
- `internal/httpapi/tokens.go` — 新增 `PATCH /api/v1/tokens/{id}`。
- `internal/httpapi/tokens_test.go` — 新增 modify 测试。
- `tests/integration/cli_test.go` — 新增 CLI 集成测试。
- `docs/openapi/xuanchu-v1.yaml` — 新增 `PATCH /api/v1/tokens/{id}`。
- `docs/manual/reference/commands.md` — 补充 `scope list` 和 `token modify` 命令。
- `docs/manual/remote-cli-and-api.md` — 补充 token modify API。
- `docs/manual/reference/errors.md` — 新增 `token_revoked`、`token_expired` 错误码。
