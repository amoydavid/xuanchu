---
title: "远程 CLI 与 HTTP API"
weight: 70
---

# 远程 CLI 与 HTTP API

taskg server 提供 HTTP/JSON API。远程 CLI 通过同一套 API 访问服务端，不复制业务逻辑。

远程 CLI 的 actor 来自 Bearer token 绑定的 user，不来自本机 `taskg user use`。如果你还不熟悉 user/workspace/member 初始化，先读 [身份与初始化](identity-and-initialization.md)。

## 启动服务端

```bash
taskg server --listen :8080
taskg server --listen 127.0.0.1:8080 --db ./taskg.db
taskg server --listen :8080 --db-url "postgres://user:pass@localhost:5432/taskg?sslmode=disable"
```

生产环境建议放在反向代理之后做 TLS termination。详见 [部署指南](deployment.md)。

## 创建 token

第一个 admin token 建议在 server 启动前用本地 CLI 创建：

```bash
taskg token create admin \
  --type pat \
  --scope task:read,task:write,project:read,project:write,workspace:read,workspace:write,token:read,token:write,audit:read,hook:read,hook:write \
  --expires-in 720h
```

输出的 raw token 只显示一次。请妥善保存。

创建 project-scoped Agent token：

```bash
taskg --workspace dajee token create mcp-agent \
  --type agent \
  --scope task:read,task:write,project:read,context:read,config:read \
  --project ai-agent-platform \
  --expires-in 720h
```

查看和撤销：

```bash
taskg token list
taskg token revoke <id-or-prefix>
```

## 使用远程 CLI

```bash
export TASKG_TOKEN="..."
taskg --server https://taskg.example.com --token "$TASKG_TOKEN" --workspace dajee list
```

这条命令中的真实 actor 是 `$TASKG_TOKEN` 绑定的 user。`--workspace dajee` 只是在 token 允许范围内选择 effective workspace。

也可以每次显式指定 project scope：

```bash
taskg --server https://taskg.example.com --token "$TASKG_TOKEN" \
  --workspace dajee --project ai-agent-platform list
```

脚本和 Agent 推荐使用 `--project-id`，避免 slug 歧义：

```bash
taskg --server https://taskg.example.com --token "$TASKG_TOKEN" \
  --project-id <project-uuid> list
```

远程模式不支持依赖本机编辑器或本机文件语义的命令，例如：

- `edit`
- `config import-taskrc`

## Token scope

最终权限是这些条件的交集：

- membership role
- token capability
- token workspace allowlist
- token project allowlist

常用 capability：

| Capability | 说明 |
|---|---|
| `task:read` / `task:write` | task、report、import/export、urgency |
| `project:read` / `project:write` | project 与 project config |
| `context:read` / `context:write` | context |
| `config:read` / `config:write` | workspace 业务配置 |
| `audit:read` | audit list |
| `token:read` / `token:write` | token list/create/revoke |
| `workspace:read` / `workspace:write` | workspace/member 管理 |
| `hook:read` / `hook:write` | hook definition、delivery、replay |

project-scoped token 读不到 scope 外的任务。单任务越界读取返回 `task_not_found`，避免泄露资源存在性。

## HTTP API 基础

认证：

```bash
curl -H "Authorization: Bearer $TASKG_TOKEN" \
  https://taskg.example.com/api/v1/me
```

列任务：

```bash
curl -H "Authorization: Bearer $TASKG_TOKEN" \
  'https://taskg.example.com/api/v1/tasks?workspace=dajee&project=ai-agent-platform&limit=20'

curl -H "Authorization: Bearer $TASKG_TOKEN" \
  'https://taskg.example.com/api/v1/tasks?workspace=dajee&query=assignee:me'
```

创建任务：

```bash
curl -X POST \
  -H "Authorization: Bearer $TASKG_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"description":"Review MCP docs","project":"ai-agent-platform","tags":["review"],"assignees":["alice"]}' \
  'https://taskg.example.com/api/v1/tasks?workspace=dajee'
```

更新 assignee：

```bash
curl -X PATCH \
  -H "Authorization: Bearer $TASKG_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"assignees":["alice"],"remove_assignees":["bob"]}' \
  'https://taskg.example.com/api/v1/tasks/<task-uuid>?workspace=dajee'
```

远程 CLI 走同一套字段语义：

```bash
taskg --server https://taskg.example.com --token "$TASKG_TOKEN" \
  add "Ship docs" @alice
taskg --server https://taskg.example.com --token "$TASKG_TOKEN" \
  1 modify +@alice -@bob
taskg --server https://taskg.example.com --token "$TASKG_TOKEN" \
  list assignee:me
```

服务端模式下，`assignees` 只能引用当前 effective workspace 的成员；不存在的用户返回 `assignee_not_found`，跨 workspace 成员返回 `assignee_not_member`。

API 使用统一 envelope：

```json
{
  "data": {},
  "meta": {}
}
```

错误响应：

```json
{
  "error": {
    "code": "project_not_found",
    "message": "project not found"
  }
}
```

OpenAPI 文件在：

```text
docs/openapi/taskg-v1.yaml
```

## Impersonation（M10）

远程 CLI 和 HTTP API 支持 impersonation：持有带 `impersonate` scope 的 agent token 的请求可以指定目标用户，以该用户身份执行操作。

### 远程 CLI

```bash
taskg --server https://taskg.example.com \
  --token "$AGENT_TOKEN" \
  --workspace dajee \
  --as alice \
  list assignee:me
```

`--as` 只在远程模式生效，值为目标用户的 name、email 或 UUID。所有 HTTP 子请求都会携带 `X-Taskg-As` header。

### HTTP API

```
GET /api/v1/tasks
Authorization: Bearer taskg_agent_...
X-Taskg-As: alice
```

只有 agent token 且拥有 `impersonate` scope 时，`X-Taskg-As` 才生效。权限以目标用户在 workspace 的 membership role 与 token scope 的交集为准。

如果 token 可见多个 workspace 且请求未显式指定 workspace，返回 `workspace_required`。目标用户不存在或不是 workspace 成员时返回 `membership_not_found`。

Audit log 会同时记录 `actor_user_id`（目标用户）和 `delegator_token_id`/`delegator_user_id`（发起 impersonation 的 agent token）。
