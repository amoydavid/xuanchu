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
```

创建任务：

```bash
curl -X POST \
  -H "Authorization: Bearer $TASKG_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"description":"Review MCP docs","project":"ai-agent-platform","tags":["review"]}' \
  'https://taskg.example.com/api/v1/tasks?workspace=dajee'
```

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
