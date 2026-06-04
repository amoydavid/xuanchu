# Token Scope 通配符

## 背景

taskg 当前有 16 个 token scope（`task:read`、`task:write`、`project:read` 等），创建 token 时必须逐个列举。一个全权限 admin token 需要 15 个 scope（不含 `impersonate`），写起来冗长且容易遗漏或拼错。

同时，新增 scope 后没有统一的地方查看所有可用 scope，用户只能翻文档或源码。

## 目标

1. `--scope` 支持通配符，减少输入负担。
2. 新增 `taskg scope list` 命令，输出所有有效 scope。
3. 未来新增 scope 时，`scope list` 自动反映，通配符自动包含。
4. 运行时行为不变，存储和校验逻辑不变。

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
- 通配符展开不区分 `impersonate` 等裸 scope；`*` 会包含它，但后续 token type 和 role 校验仍然生效。
- 多个通配符和具体 scope 可以混合使用，展开后去重。

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

### 4. `taskg scope list` 命令

```
taskg scope list
taskg scope list --json
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

### 5. CLI 使用示例

```bash
# 全权限 admin token
taskg token create admin --type pat --scope '*' --expires-in 720h

# 只读 token
taskg token create viewer --type agent --scope '*:read' --project my-project --expires-in 720h

# task 全权限 + 项目只读
taskg token create task-worker --type agent --scope 'task:*,project:read' --expires-in 720h

# 查看可用 scope
taskg scope list
```

## 不改什么

- token 存储、运行时校验、MCP tool 权限检查 —— 完全不变。
- `impersonate` scope 对 PAT 的限制 —— 不变（`*` 展开后包含 `impersonate`，但 PAT 的 type 校验仍然拒绝它）。
- `impersonate` scope 对 admin/owner 的限制 —— 不变。
- 父子 token scope 子集校验 —— 不变（比较展开后的列表）。
- token 更新 —— 当前不支持 token scope 更新，本次不做。

## 验收标准

- `taskg token create admin --scope '*' --type pat` 能创建全权限 PAT（不含 `impersonate`）。
- `taskg token create agent --scope '*' --type agent` 能创建全权限 agent token（含 `impersonate`，需 admin/owner）。
- `taskg token create viewer --scope '*:read'` 能创建只读 token。
- `taskg token create worker --scope 'task:*'` 能创建 task 全权限 token。
- 无效通配符（如 `foo:*`）报 `token_scope_invalid`。
- `taskg scope list` 输出所有有效 scope。
- `taskg scope list --json` 输出结构化 JSON。
- 所有现有 scope 校验测试通过。
- `go test ./...`、`CGO_ENABLED=0 go test ./...`、`CGO_ENABLED=0 go build ./cmd/taskg` 通过。

## 影响范围

- `internal/auth/scope.go` — 主要改动：有序注册、`ParseScopes()` 通配符展开。
- `internal/cli/scope.go` — 新增：`scope list` 命令。
- `internal/cli/root.go` — 注册 `scope` 子命令。
- `internal/auth/scope_test.go` — 新增通配符测试。
- `tests/integration/cli_test.go` — 新增 CLI 集成测试。
- `docs/manual/reference/commands.md` — 补充 `scope list` 命令。
- `docs/manual/mcp.md` — 无需改动（MCP 层不变）。
