# Token 工具

Token 是 Xuanchu 的身份凭证，用于 HTTP MCP 连接（Bearer Token）、远程 CLI 认证、API 调用。Token 创建时返回原始 secret，**仅此一次**，后续只能看到 prefix。

> 以下 token 操作均指**发给外部系统/agent 的凭证**；当前高权限接入方通常由后台预先发放 `["*"]` agent_token，不在本 skill 教的范围内。

## token_list — 列出 Token（只读）

已撤销的 Token 不显示。

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

## token_create — 创建 Token

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

## token_modify — 修改 Token

不能修改已撤销或已过期的 Token。

```json
{"workspace": "dajee", "token_ref": "token-uuid-xxx", "name": "renamed-agent"}
{"workspace": "dajee", "token_ref": "token-uuid-xxx", "scope": ["task:read"]}
{"workspace": "dajee", "token_ref": "token-uuid-xxx", "expires_in_seconds": 604800}
```

## token_revoke — 撤销 Token

```json
{"workspace": "dajee", "token_ref": "token-uuid-xxx"}
```

## 典型工作流

### 创建一个 MCP Token 给 Claude 用

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

### 轮换 Token

```json
// Step 1: 创建新 Token
token_create({"workspace": "dajee", "name": "claude-agent-v2", "scope": ["..."]})

// Step 2: 用户更新客户端配置

// Step 3: 撤销旧 Token
token_revoke({"workspace": "dajee", "token_ref": "旧 token ID"})
```
