# Token 管理

通过 Xuanchu MCP 管理 API Token——用于 HTTP MCP 和远程 CLI 的身份凭证。

## 基本概念

Token 是 Xuanchu 的身份凭证，用于：
- HTTP MCP 连接（Bearer Token）
- 远程 CLI 认证
- API 调用

Token 创建时返回原始 secret，**仅此一次**。后续只能看到 prefix。

## Token 操作

### token_list — 列出 Token

只读。已撤销的 Token 不显示。

```json
// 输入
{"workspace": "dajee"}

// 返回
{
  "data": {
    "tokens": [
      {
        "id": "token-uuid-xxx",
        "prefix": "xuanchu_agent_abc123",
        "name": "mcp-agent",
        "type": "agent",
        "user": {"id": "user-uuid-xxx", "name": "local"},
        "scopes": ["task:read", "task:write"],
        "workspace_ids": ["ws-uuid-xxx"],
        "project_ids": ["proj-uuid-xxx"],
        "created_at": 1748707200
      }
    ],
    "count": 1
  }
}
```

### token_create — 创建 Token

`name` 必填。返回中包含 `raw_token`，**必须保存**。

```json
// 输入：创建 Agent token
{
  "workspace": "dajee",
  "name": "claude-agent",
  "scope": ["*"],
  "expires_in_seconds": 2592000
}

// 输入：创建全权限短命 token
{
  "workspace": "dajee",
  "name": "ci-token",
  "scope": ["*"],
  "expires_in_seconds": 86400
}

// 返回
{
  "data": {
    "token": {
      "id": "token-uuid-new",
      "prefix": "xuanchu_agent_def456",
      "name": "claude-agent",
      "type": "agent",
      "user": {"id": "user-uuid-xxx", "name": "local"},
      "scopes": ["task:read", "task:write", "project:read", "..."],
      "raw_token": "xuanchu_agent_def456xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
    }
  },
  "rendered": "token claude-agent created"
}
```

### token_modify — 修改 Token

不能修改已撤销或已过期的 Token。

```json
{"workspace": "dajee", "token_ref": "token-uuid-xxx", "name": "renamed-agent"}
{"workspace": "dajee", "token_ref": "token-uuid-xxx", "scope": ["task:read"]}
{"workspace": "dajee", "token_ref": "token-uuid-xxx", "expires_in_seconds": 604800}
```

### token_revoke — 撤销 Token

```json
{"workspace": "dajee", "token_ref": "token-uuid-xxx"}
```

## Scope 说明

用 `scope_list` 查看所有可用 scope（无需鉴权）。

常用 scope：
- `task:read` / `task:write` — 任务读写
- `project:read` / `project:write` — 项目读写
- `context:read` / `context:write` — context 读写
- `workspace:read` / `workspace:write` — workspace 读写
- `config:read` / `config:write` — 配置读写
- `hook:read` / `hook:write` — Hook 读写
- `notification:read` / `notification:write` — notification sink 与 delivery 读写
- `reminder:read` / `reminder:write` — reminder rule 读写
- `token:read` / `token:write` — Token 读写
- `audit:read` — 审计日志
- `impersonate` — 委托操作
- `*` — 所有权限

通用 HTTP MCP / workspace Agent token 默认使用 `["*"]`，让 Agent 覆盖用户、成员、项目、任务、配置、通知、token 等完整工具集。`*` 是 token capability 上限，实际权限仍会被 workspace/project allowlist 和绑定用户的 membership role 收窄。只有专用自动化 token 才按场景改成最小 scope。

通配符扩展：
- `*` → 所有 scope，包括 `impersonate`
- `task:*` → `task:read` + `task:write`
- `*:read` → 所有 `:read` scope

## 典型 Agent 工作流

**场景：用户说"帮我创建一个 MCP Token 给 Claude 用"**

```json
// Step 1: 创建 Token
token_create({
  "workspace": "dajee",
  "name": "claude-agent",
  "scope": ["*"],
  "expires_in_seconds": 2592000
})

// Step 2: 把返回的 raw_token 告诉用户，用于配置 MCP 客户端
// raw_token 只出现一次，必须保存
```

**场景：轮换 Token**

```json
// Step 1: 创建新 Token
token_create({"workspace": "dajee", "name": "claude-agent-v2", "scope": [...]})

// Step 2: 用户更新客户端配置

// Step 3: 撤销旧 Token
token_revoke({"workspace": "dajee", "token_ref": "旧 token ID"})
```
