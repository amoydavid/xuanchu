---
title: "远程 CLI 与 HTTP API"
weight: 70
---

# 远程 CLI 与 HTTP API

Xuanchu 通过 `xuanchu server` 提供 HTTP/JSON API。远程 CLI 通过同一套 API 访问服务端，不复制业务逻辑。

远程 CLI 的 actor 来自 Bearer token 绑定的 user，不来自本机 `xuanchu user use`。如果你还不熟悉 user/workspace/member 初始化，先读 [身份与初始化](identity-and-initialization.md)。

## 启动服务端

```bash
xuanchu server --listen :8080
xuanchu server --listen 127.0.0.1:8080 --db ./xuanchu.db
xuanchu server --listen :8080 --db-url "postgres://user:pass@localhost:5432/xuanchu?sslmode=disable"
```

生产环境建议放在反向代理之后做 TLS termination。详见 [部署指南](deployment.md)。

## 创建远程访问 token

远程 CLI 和普通 HTTP API 使用 PAT 或 Agent token，不使用 server admin token。第一个远程访问 PAT 可以在 server 启动前用本地 CLI 创建：

```bash
xuanchu token create admin \
  --type pat \
  --scope '*' \
  --expires-in 720h
```

输出的 raw token 只显示一次。请妥善保存。

创建 workspace-scoped Agent token：

```bash
xuanchu --workspace dajee token create mcp-agent \
  --type agent \
  --scope '*' \
  --expires-in 720h
```

通用 workspace Agent token 建议直接授予 `*` scope，让它覆盖 MCP/远程 API 中的用户、成员、项目、任务、配置、通知、token 等完整工作流。`*` 只是 capability 上限，最终仍会被 token workspace/project allowlist 和绑定用户的 membership role 收窄。只允许访问单个 project 时，再加 `--project agentapi` 或 `--project-id <project-uuid>`。

查看和撤销：

```bash
xuanchu token list
xuanchu token revoke <id-or-prefix>
```

## 使用远程 CLI

```bash
export XUANCHU_TOKEN="..."
xuanchu --server https://xuanchu.example.com --token "$XUANCHU_TOKEN" --workspace dajee list
```

这条命令中的真实 actor 是 `$XUANCHU_TOKEN` 绑定的 user。`--workspace dajee` 只是在 token 允许范围内选择 effective workspace。

也可以每次显式指定 project scope：

```bash
xuanchu --server https://xuanchu.example.com --token "$XUANCHU_TOKEN" \
  --workspace dajee --project agentapi list
```

脚本和 Agent 推荐使用 `--project-id`，避免 slug 歧义：

```bash
xuanchu --server https://xuanchu.example.com --token "$XUANCHU_TOKEN" \
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
| `config:read` / `config:write` | 配置值和 shared config schema 定义 |
| `audit:read` | audit list |
| `token:read` / `token:write` | token list/create/modify/revoke |
| `workspace:read` / `workspace:write` | workspace/member 管理 |
| `hook:read` / `hook:write` | hook definition、delivery、replay |
| `notification:read` / `notification:write` | notification sink、delivery、replay |
| `reminder:read` / `reminder:write` | reminder rule 管理 |
| `impersonate` | 以其他用户身份操作（仅 agent token） |

workspace-scoped token 没有 project allowlist，可访问该 workspace 内多个 project。project-scoped token 读不到 scope 外的任务；单任务越界读取返回 `task_not_found`，避免泄露资源存在性。

### Scope 通配符

创建或修改 token 时，scope 支持通配符展开：

| 通配符 | 含义 | 示例 |
|---|---|---|
| `*` | 所有 scope | `--scope '*'` |
| `resource:*` | 该资源的所有动作 | `task:*` → `task:read,task:write` |
| `*:action` | 所有资源的指定动作 | `*:read` → `task:read,project:read,context:read,...` |

PAT 使用 `*` 通配符时自动剔除 `impersonate`（仅限 agent token）。

使用 `xuanchu scope list` 查看当前系统所有可用 scope。

### Token 修改

已创建的 token 可以修改名称、scope 和过期时间：

```bash
xuanchu token modify <id> --name "新名称"
xuanchu token modify <id> --scope '*:read' --scope 'task:write'
xuanchu token modify <id> --expires-in 0
```

修改 token 需要 `token:write` 权限。已撤销或已过期的 token 不能修改。

## HTTP API 基础

认证：

```bash
curl -H "Authorization: Bearer $XUANCHU_TOKEN" \
  https://xuanchu.example.com/api/v1/me
```

列任务：

```bash
curl -H "Authorization: Bearer $XUANCHU_TOKEN" \
  'https://xuanchu.example.com/api/v1/tasks?workspace=dajee&project=agentapi&limit=20'

curl -H "Authorization: Bearer $XUANCHU_TOKEN" \
  'https://xuanchu.example.com/api/v1/tasks?workspace=dajee&query=assignee:me'
```

创建任务：

```bash
curl -X POST \
  -H "Authorization: Bearer $XUANCHU_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"title":"Review MCP docs","description":"Read the latest API notes","project":"agentapi","tags":["review"],"assignees":["alice"]}' \
  'https://xuanchu.example.com/api/v1/tasks?workspace=dajee'
```

更新 assignee：

```bash
curl -X PATCH \
  -H "Authorization: Bearer $XUANCHU_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"assignees":["alice"],"remove_assignees":["bob"]}' \
  'https://xuanchu.example.com/api/v1/tasks/agentapi-1?workspace=dajee'
```

用户对象会同时返回稳定引用名 `name` 和展示姓名 `display_name`。`name` 继续用于 CLI/API/MCP 中的用户引用和唯一性判断；`display_name` 可为空，可与 `name` 不同，适合 Web Console 或集成端展示真实姓名/昵称。

单任务 path 使用 `{taskRef}`，接受完整 UUID、已物化任务的 `task_slug` 或 URL 编码后的 `occurrence_ref`。projected 实例只能使用 occurrence_ref；已物化实例三种引用等价，响应都包含 `recurrence_info`。HTTP API 不接受纯数字 working-set ID；传入 `1` 会返回 `task_ref_invalid`。远程 CLI 仍支持 `info 1`、`1 done` 这类交互写法，但会先在客户端按当前 working set 解析，再调用 HTTP API。

`POST /tasks`、`GET/PATCH/DELETE /tasks/{taskRef}` 和任务生命周期动作统一返回
`TaskOccurrenceView`：普通任务的 `id` 等于 `uuid`且 `recurrence_info` 为空；循环实例的
`id` 始终是 occurrence_ref。`entry/modified/due/start/end` 等时间字段使用 Unix 秒，
Remote 不再根据时间字符串猜测另一种 JSON 形态。

`GET /api/v1/task-series` 与 `GET /api/v1/task-series/{seriesRef}/occurrences` 的分页和 MCP、Remote CLI
使用同一契约：默认 `limit=200, offset=0`，limit 取值 1–1000，offset 不得为负数。

远程 CLI 走同一套字段语义：

```bash
xuanchu --server https://xuanchu.example.com --token "$XUANCHU_TOKEN" \
  add "Ship docs" @alice
xuanchu --server https://xuanchu.example.com --token "$XUANCHU_TOKEN" \
  1 modify +@alice -@bob
xuanchu --server https://xuanchu.example.com --token "$XUANCHU_TOKEN" \
  list assignee:me
```

服务端模式下，`assignees` 只能引用当前 effective workspace 的成员；不存在的用户返回 `assignee_not_found`，跨 workspace 成员返回 `assignee_not_member`。

### Notification / Reminder API

notification sink：

```bash
curl -X POST \
  -H "Authorization: Bearer $XUANCHU_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"openclaw","type":"webhook","endpoint_mode":"static_url","url":"https://openclaw.example.com/xuanchu/notifications","secret":"..."}' \
  'https://xuanchu.example.com/api/v1/notification-sinks?workspace=dajee'

curl -X PATCH \
  -H "Authorization: Bearer $XUANCHU_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"openclaw-prod"}' \
  'https://xuanchu.example.com/api/v1/notification-sinks/<sink-id>?workspace=dajee'
```

reminder rule：

```bash
curl -X POST \
  -H "Authorization: Bearer $XUANCHU_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"due-soon-24h","schedule_type":"daily_at","schedule_value":"08:50","filter_source":"end.isnull and start.isnull and due.after:now and due.before:now+24h","audience_type":"assignees","sink_ref":"openclaw"}' \
  'https://xuanchu.example.com/api/v1/reminder-rules?workspace=dajee'
```

delivery：

```bash
curl -H "Authorization: Bearer $XUANCHU_TOKEN" \
  'https://xuanchu.example.com/api/v1/notification-deliveries?workspace=dajee&status=dead_lettered'

curl -X POST \
  -H "Authorization: Bearer $XUANCHU_TOKEN" \
  'https://xuanchu.example.com/api/v1/notification-deliveries/<delivery-id>/replay?workspace=dajee'
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

OpenAPI 文档由 Huma code-first route 注册在运行时生成，不提交静态 YAML 产物。启动 `xuanchu server` 后可以访问：

```text
http://127.0.0.1:8080/docs
http://127.0.0.1:8080/openapi.json
http://127.0.0.1:8080/openapi.yaml
http://127.0.0.1:8080/openapi-3.0.json
http://127.0.0.1:8080/openapi-3.0.yaml
```

## 任务附件

任务附件是 workspace 级通用二进制资源，首期挂在 task 上。本地 CLI 直接读写存储后端；
远程 CLI 使用同一 HTTP API，上传/下载走 multipart/stream，不把二进制放进 JSON。

### CLI 命令

```bash
xuanchu attachment add <task-ref> <file> [--display-name <name>]
xuanchu attachment list <task-ref>
xuanchu attachment info <attachment-id>
xuanchu attachment download <attachment-id> [--output <path>|-]
xuanchu attachment rename <attachment-id> <display-name>
xuanchu attachment remove <attachment-id>
```

- `add` 走 multipart 上传，本地与远程一致；远程模式上传进度写 stderr。
- `download` 默认写入当前目录的展示名（`O_CREAT|O_EXCL`，已存在返回 `attachment_output_exists`）。
  `--output -` 写 stdout，进度信息写 stderr；禁止与 `--json` 同时使用。
- `--json` 仅用于 metadata/list/info/write result，不与二进制 stdout 混用。

### HTTP API

```text
POST   /api/v1/tasks/{taskRef}/attachments              multipart/form-data: file, mode, display_name?
POST   /api/v1/tasks/{taskRef}/attachments/import-url   JSON: source_url, mode=description_draft, display_name?
GET    /api/v1/tasks/{taskRef}/attachments              ?include_drafts=true
GET    /api/v1/attachments/{attachmentID}
GET    /api/v1/attachments/{attachmentID}/content       流式，返回 Content-Type/Disposition/ETag
PATCH  /api/v1/attachments/{attachmentID}               JSON: display_name
DELETE /api/v1/attachments/{attachmentID}               204
```

content 响应固定带 `X-Content-Type-Options: nosniff` 与 `Cache-Control: private, no-store`；
非内联类型统一 `Content-Disposition: attachment`，文件名按 RFC 5987 编码。详细错误码见
spec §18。

## Impersonation（M10）

远程 CLI 和 HTTP API 支持 impersonation：持有带 `impersonate` scope 的 agent token 的请求可以指定目标用户，以该用户身份执行操作。

### 远程 CLI

```bash
xuanchu --server https://xuanchu.example.com \
  --token "$AGENT_TOKEN" \
  --workspace dajee \
  --as alice \
  list assignee:me
```

`--as` 只在远程模式生效，值为目标用户的 name、email 或 UUID。所有 HTTP 子请求都会携带 `X-Xuanchu-As` header。

### HTTP API

```
GET /api/v1/tasks
Authorization: Bearer xuanchu_agent_...
X-Xuanchu-As: alice
```

只有 agent token 且拥有 `impersonate` scope 时，`X-Xuanchu-As` 才生效。权限以目标用户在 workspace 的 membership role 与 token scope 的交集为准。

如果 token 可见多个 workspace 且请求未显式指定 workspace，返回 `workspace_required`。目标用户不存在或不是 workspace 成员时返回 `membership_not_found`。

Audit log 会同时记录 `actor_user_id`（目标用户）和 `delegator_token_id`/`delegator_user_id`（发起 impersonation 的 agent token）。
