---
title: "MCP 使用指南"
weight: 80
---

# MCP 使用指南

xuanchu 可以作为 MCP Server，让 Agent 通过结构化 tools/resources 访问任务系统。

MCP 调用必须有明确身份。stdio MCP 使用本机 active user/workspace；HTTP MCP 使用 Bearer token 绑定的 user 和 scope。完整身份初始化流程见 [身份与初始化](identity-and-initialization.md)。

**重要原则：** 每个 MCP 调用都应通过参数显式指定 `workspace`，不要依赖隐式上下文状态（如 `context_set`/`workspace_use`）。任务要归属到某个 project，或查询需要收窄到某个 project 时，再显式传 `project`/`project_id`。如果不知道 workspace 或 project，先调用 `workspace_list`/`project_list` 发现。

面向 Agent 的可复用操作说明放在 [`docs/skills`](../skills/)；那里按任务管理、项目管理、通知提醒、Token、审计等场景拆分，字段名以当前 MCP tool schema 为准。

## 运行模式

本地 stdio MCP：

```bash
xuanchu mcp stdio
```

stdio MCP 支持可靠停机参数：

```bash
xuanchu mcp stdio --shutdown-timeout 30s --shutdown-force-timeout 5s
```

收到 SIGTERM / SIGINT 后，stdio MCP 会停止开始新的 tool call，等待已开始的 tool call 在 `shutdown-timeout` 内完成；超时后才强制取消。停机日志只写 stderr，不会污染 stdout 中的 MCP JSON-RPC 协议帧。

HTTP MCP：

```bash
xuanchu server --listen :8080
```

HTTP MCP endpoint：

```text
/mcp
```

HTTP MCP 使用 Bearer token 鉴权，复用远程 CLI 和 HTTP API 的 token scope。Bearer token 可以是绑定用户的 PAT / Agent token，也可以是 workspace 级 `tenant_access_token`。

需要复制 Bearer token 或 MCP 客户端配置片段时，在 Web Console `/tokens` 页面点击对应行的「MCP 配置」按钮即可按需 reveal（服务端需配置 `[security].config_secret_key`）。详见 [Web Console 手册](web-console.md)。

HTTP MCP 与 HTTP API 共享同一授权决策：Bearer token、workspace/project 参数、`X-Xuanchu-As` impersonation、token scope 和 membership role 的结果一致。同一个 token 和 workspace/project 组合下，`task_query`（MCP）与 `GET /api/v1/tasks`（HTTP API）返回相同的可见任务集；权限不足时返回相同的错误码与 HTTP status。`tenant_access_token` 没有用户 principal，不支持 `X-Xuanchu-As`，可按 scope 调用 user/member/token 管理 tools、hook/notification/reminder 写 tools，以及当前绑定 workspace 的读取和修改类 tools（例如 `workspace_info`、`workspace_modify`）；但不能调用 `me_get`、`user_use`、`workspace_use`、`workspace_list`、`workspace_add`、`workspace_archive` 或 active context 写入。

stdio MCP 继续沿用本地 active user/workspace，不支持 impersonation。

如果 HTTP MCP 通过 nginx/Caddy 暴露公网域名，并且后端只监听 `127.0.0.1:<port>`，需要显式配置可信反代 Host：

```toml
[server.mcp]
trusted_proxy_hosts = ["xuanchu.example.com"]
```

反向代理应保留真实 Host，例如 nginx：

```nginx
proxy_set_header Host $host;
```

未配置 allowlist 时，MCP SDK 会在 loopback 后端上拒绝公网 Host，返回 `403 Forbidden: invalid Host header`。不要把 Host 改写成后端地址作为长期部署方案。

## 接入前准备

如果使用本地 stdio MCP，先确认 `xuanchu` 在 PATH 中，或记录完整路径：

```bash
which xuanchu
```

如果使用 HTTP MCP，先启动服务端并创建 token：

```bash
xuanchu server --listen :8080

xuanchu --workspace dajee token create mcp-agent \
  --type agent \
  --scope '*' \
  --expires-in 720h
```

HTTP MCP 有两种常见凭证：

- workspace-scoped Agent token：适合需要用户身份、成员管理、token 管理或 impersonation 的 Agent。通用 Agent 可用 `*` scope，再用 workspace/project allowlist 和 membership role 收窄。
- `tenant_access_token`：适合类似 OpenAI API key 的机器访问，不绑定用户，只能在绑定 workspace 内调用租户白名单能力。它可执行任务、项目、配置、审计，并可按 scope 管理 user、member、tenant token、workspace 设置、hook、notification sink、reminder rule、event notification rule、task link 和 project annotation；这些 `created_by` / `actor` 字段会输出为 `{"type":"tenant_access_token","token":{...}}`。

如果只允许服务单个 project，再加 `--project <slug>` 或 `--project-id <uuid>` 收窄 allowlist。

## 在 Claude Code 中使用

Claude Code 支持 stdio 和 HTTP MCP。推荐优先使用 HTTP MCP 连接生产服务端，使用 stdio MCP 连接本机开发库。

### Claude Code 本地 stdio

```bash
claude mcp add --transport stdio xuanchu -- xuanchu mcp stdio
```

如果 `xuanchu` 不在 PATH 中，使用完整路径：

```bash
claude mcp add --transport stdio xuanchu -- /path/to/xuanchu mcp stdio
```

指定本地数据库：

```bash
claude mcp add --transport stdio xuanchu -- /path/to/xuanchu --db /path/to/xuanchu.db mcp stdio
claude mcp add --transport stdio xuanchu -- /path/to/xuanchu --db-url "postgres://user:pass@localhost:5432/xuanchu?sslmode=disable" mcp stdio
```

### Claude Code 远程 HTTP

```bash
claude mcp add \
  --transport http \
  --header "Authorization: Bearer $XUANCHU_TOKEN" \
  xuanchu \
  https://xuanchu.example.com/mcp
```

检查连接状态：

```bash
claude mcp list
claude mcp get xuanchu
```

在 Claude Code 交互界面中也可以运行：

```text
/mcp
```

### Claude Code 项目级配置

如果希望把配置随项目提交，可以在项目根目录放 `.mcp.json`。不要把真实 token 提交进仓库。

```json
{
  "mcpServers": {
    "xuanchu": {
      "type": "stdio",
      "command": "/path/to/xuanchu",
      "args": ["mcp", "stdio"],
      "env": {}
    }
  }
}
```

HTTP 示例：

```json
{
  "mcpServers": {
    "xuanchu": {
      "type": "http",
      "url": "https://xuanchu.example.com/mcp",
      "headers": {
        "Authorization": "Bearer ${XUANCHU_TOKEN}"
      }
    }
  }
}
```

如果客户端不展开 `${XUANCHU_TOKEN}`，请改用该客户端支持的 secret manager、header helper，或只把配置放在用户级私有配置里。

## 在 OpenClaw 中使用

OpenClaw 的 `openclaw mcp serve` 是"OpenClaw 自己作为 MCP server"。这里要做的是相反方向：让 OpenClaw 托管的 agent 使用 Xuanchu MCP server，所以应使用 OpenClaw 的 MCP client registry，也就是 `openclaw mcp add/set/configure/probe`。

### OpenClaw 本地 stdio

```bash
openclaw mcp add xuanchu \
  --command /path/to/xuanchu \
  --arg mcp \
  --arg stdio
```

指定数据库：

```bash
openclaw mcp add xuanchu \
  --command /path/to/xuanchu \
  --arg --db \
  --arg /path/to/xuanchu.db \
  --arg mcp \
  --arg stdio
```

### OpenClaw 远程 HTTP

```bash
openclaw mcp set xuanchu '{
  "url": "https://xuanchu.example.com/mcp",
  "transport": "streamable-http",
  "headers": {
    "Authorization": "Bearer <token>"
  },
  "timeout": 30,
  "connectTimeout": 10
}'
```

验证：

```bash
openclaw mcp status --verbose
openclaw mcp doctor xuanchu --probe
openclaw mcp probe xuanchu --json
```

如果只希望 OpenClaw 暴露一部分 Xuanchu MCP tools，可以配置 tool filter。例如只让 Agent 查询任务和读取项目：

```bash
openclaw mcp tools xuanchu --include 'task_query,task_get,project_list,project_get,workspace_get_current'
```

注意：OpenClaw 文档中 `streamable-http` 是 Streamable HTTP 的规范写法；Xuanchu 的 `/mcp` 就是这个 HTTP MCP endpoint。

## 在 Hermes Agent 中使用

Hermes Agent 从 `~/.hermes/config.yaml` 的 `mcp_servers` 读取 MCP 配置。它支持本地 stdio server 和远程 HTTP MCP server。

### Hermes 本地 stdio

```yaml
mcp_servers:
  xuanchu:
    command: "/path/to/xuanchu"
    args: ["mcp", "stdio"]
```

指定数据库：

```yaml
mcp_servers:
  xuanchu:
    command: "/path/to/xuanchu"
    args: ["--db", "/path/to/xuanchu.db", "mcp", "stdio"]
```

### Hermes 远程 HTTP

```yaml
mcp_servers:
  xuanchu:
    url: "https://xuanchu.example.com/mcp"
    headers:
      Authorization: "Bearer ${XUANCHU_TOKEN}"
    timeout: 30
    connect_timeout: 10
```

Hermes 会在启动时发现 MCP tools：

```bash
hermes chat
```

如果正在运行 Hermes，改完配置后执行：

```text
/reload-mcp
```

Hermes 会给 MCP tool 名加前缀，形如：

```text
mcp_xuanchu_task_query
mcp_xuanchu_task_add
mcp_xuanchu_project_list
```

通常不需要手动调用这些名字；让 Agent 用自然语言描述目标即可。

## Scope

stdio MCP 使用本地 runtime context。HTTP MCP 使用请求 token 解析 actor、workspace 和 project scope。

Agent 不应该靠提示词决定权限。权限来自：

- token capability
- workspace scope
- project scope（可为空；为空表示不按 project 收窄）
- membership role

stdio MCP 启动前，建议先确认本机 active user/workspace：

```bash
xuanchu _show active.user active.workspace
```

HTTP MCP 接入前，如需用户身份能力，建议为 Agent 创建 workspace-scoped token：

```bash
xuanchu --workspace dajee token create mcp-agent \
  --type agent \
  --scope '*' \
  --expires-in 720h
```

这个 `*` 是 token capability 上限，实际权限仍会被 workspace/project allowlist 和绑定用户的 membership role 收窄。如果这个 Agent 只允许服务单个 project，可以额外加 `--project agentapi` 或 `--project-id <project-uuid>`。

如果只是给外部自动化一个租户 API key，可在 Web Console `/tokens` 的「租户访问令牌」tab 创建 `tenant_access_token`，然后在 MCP 客户端中作为 Bearer token 使用。

## 建议给 Agent 的提示词

可以在 Agent 系统提示词或项目说明中加入：

```text
你可以使用 Xuanchu MCP 管理任务。每次调用都必须通过参数显式指定 workspace，
不要依赖隐式上下文。任务应归属某个 project 或查询要按 project 收窄时，再显式传 project_id。
如果不知道 workspace 或 project，先调用 workspace_list / project_list 发现。
查询任务用 task_query，读取单任务用 task_get，新增任务用 task_add。
如果任务有执行者，请在 task_add / task_modify 里显式传 assignees。
不要尝试访问 token scope 之外的 workspace/project。
写入任务前，如果 project 不明确，先询问用户或调用 project_list。
```

## Tools

当前提供 106 个 tools。

### 任务与循环任务（23 tools）

所有 tool 输入都是 closed schema：未声明的参数会在服务端被拒绝，不会被静默忽略。
普通任务的 `task_add` / `task_modify` 不接受 `recur`、`clear_recur`、`mask` 或 `imask`；
传入这些旧字段会返回 `task_series_endpoint_required`，循环任务必须使用 `task_series_*` tools。

#### `task_add`

创建任务。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | workspace slug 或 UUID |
| `project` | string | 否 | project slug |
| `project_id` | string | 否 | project UUID |
| `title` | string | 是 | 任务标题 |
| `description` | string | 否 | 详细描述 |
| `tags` | string[] | 否 | 要添加的标签 |
| `assignees` | string[] | 否 | workspace 用户引用（name/email/UUID/外部ID） |
| `priority` | string | 否 | `H`/`M`/`L` |
| `due` | int64 | 否 | unix 秒 |
| `wait` | int64 | 否 | unix 秒 |
| `scheduled` | int64 | 否 | unix 秒 |
| `until` | int64 | 否 | unix 秒 |
| `annotations` | string[] | 否 | 初始注释 |

`task_add`、`task_get` 和返回任务的写操作都以 App `TaskOccurrenceView` 为准：
`data` 顶层包含稳定 `id`、可空 `uuid/task_slug/project_seq`、Unix 秒时间字段和
可空 `recurrence_info`。现有 task tools 同时保留内层 `data.task` 镜像，两处字段来自同一 view，
不再返回 RFC3339 时间的旧 `task.JSONTask` 变体。

#### `task_query`

查询任务。只读。默认返回所有非删除任务；显式 `status` 或 `query` 中的状态条件优先。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | workspace slug 或 UUID |
| `project` | string | 否 | project slug |
| `project_id` | string | 否 | project UUID |
| `query` | string | 否 | 任务过滤表达式 |
| `status` | string | 否 | 按状态过滤 |
| `sort` | string | 否 | 排序表达式，例如 `due`、`due-`、`urgency-` |
| `limit` | int | 否 | 最大返回数，默认 200，最大 1000 |
| `offset` | int | 否 | 分页偏移 |
| `include_deleted` | bool | 否 | 在默认非删除集合上追加 deleted；显式状态条件存在时忽略 |
| `due_after` | string | 否 | 截止范围起始日期，`YYYY-MM-DD` |
| `due_before` | string | 否 | 截止范围结束日期，`YYYY-MM-DD`，包含当天 |
| `occurrence_mode` | string | 否 | `auto`/`materialized`/`expand` |
| `task_type` | string | 否 | `all`/`normal`/`occurrence` |

#### `task_query_help`

返回 `task_query` / `report_run` 的 `query` 参数所用 filter 表达式的完整语法手册（Markdown）。只读，无参数，无副作用。

`data.syntax` 是手册正文，覆盖：高频场景速查、逻辑组合（`and`/`or`/`not` 必须小写、隐式 AND、括号）、谓词形态（`+tag`、`-tag`、`/text/`、`field:value`、`field.before/after/isnull/notnull`）、字段表（含 `assignee` 四种引用形式：UUID / 用户名 / email / `provider:external_id`）、日期值语法（`today`/`eow`/`now+24h` 等）、UDA 规则、循环任务字段（`task_type`/`series_id`/`recurrence_at`）和常见陷阱清单。

Agent 在构造 `query` 前如不确定语法，应先调用本 tool 获取权威手册，不要凭记忆拼表达式。

#### `task_get`

读取单个任务。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |
| `id` | string | 是 | UUID、已物化 `task_slug` 或 `occurrence_ref`；projected 仅 occurrence_ref |

#### `task_modify`

修改任务。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | 限定本次调用的 workspace |
| `project` | string | 否 | 限定本次调用的 project slug |
| `project_id` | string | 否 | 限定本次调用的 project ID |
| `id` | string | 是 | UUID、已物化 `task_slug` 或 `occurrence_ref`；projected 仅 occurrence_ref |
| `title` | string | 否 | 新标题 |
| `description` | string | 否 | 新详细描述 |
| `priority` | string | 否 | `H`/`M`/`L` |
| `due` | int64 | 否 | unix 秒 |
| `wait` | int64 | 否 | unix 秒 |
| `scheduled` | int64 | 否 | unix 秒 |
| `until` | int64 | 否 | unix 秒 |
| `tags` | string[] | 否 | 要添加的标签 |
| `remove_tags` | string[] | 否 | 要移除的标签 |
| `assignees` | string[] | 否 | 要添加的执行者 |
| `remove_assignees` | string[] | 否 | 要移除的执行者 |
| `udas` | map | 否 | UDA 键值对 |
| `depends` | string[] | 否 | 要添加的依赖 |
| `clear_depends` | bool | 否 | 清空所有依赖 |
| `clear` | string[] | 否 | 要清空的字段：`project`/`description`/`priority`/`due`/`wait`/`scheduled`/`until`/`assignees`/`uda.*` |

#### `task_done`

完成任务。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |
| `id` | string | 是 | |

#### `task_delete`

删除任务。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |
| `id` | string | 是 | |

#### `task_start`

开始任务。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |
| `id` | string | 是 | |

#### `task_stop`

停止任务。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |
| `id` | string | 是 | |

#### `task_annotate`

为任务添加注释。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |
| `id` | string | 是 | |
| `annotation` | string | 是 | 注释内容 |

#### `task_denotate`

移除任务注释。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |
| `id` | string | 是 | |
| `annotation_id` | string | 是 | 任务注释 ID |

#### `task_depends`

调整任务依赖。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `id` | string | 是 | |
| `depends` | string[] | 否 | 要添加的依赖 |
| `clear_depends` | bool | 否 | 清空所有依赖 |

#### `task_link_add`

为任务添加外部关联链接。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |
| `task` | string | 是 | 任务引用（UUID、已物化 `task_slug` 或 `occurrence_ref`） |
| `type` | string | 是 | 链接类型（document/pr/ticket/design 等） |
| `url` | string | 是 | 外部资源 URL |
| `title` | string | 否 | 显示标题 |

#### `task_link_list`

列出任务的外部关联链接。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |
| `task` | string | 是 | 任务引用 |

#### `task_link_remove`

移除任务的外部关联链接。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |
| `task` | string | 是 | 任务引用 |
| `link_id` | string | 是 | 要移除的链接 ID |

#### `task_export`

按当前 workspace/project scope 导出 `xuanchu.task-bundle/v1` 原生 bundle。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |

#### `task_import`

原子导入 `xuanchu.task-bundle/v1` 原生 bundle。失败时整批回滚；不接受 Taskwarrior JSON 数组。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |
| `bundle` | object | 是 | `schema=xuanchu.task-bundle/v1`，包含 task_series、rule_versions、普通任务、occurrence、tombstone 及关联数据。 |

#### `task_series_add`

创建循环任务。必填 `title`、`recurrence_rule`，以及 `first_due`（unix 秒）或
`first_due_date`（`YYYY-MM-DD`）；可传 `project/project_id`、`description`、
`until/until_date`、`priority`、`assignees`、`tags`、`udas`。

#### `task_series_list`

分页列出循环任务。支持 `status=active|ended|stopped|all`、`q`、`assignee`、
`sort=next|title|modified`、`limit/offset`，并遵守 workspace/project scope。分页默认
`limit=200, offset=0`，limit 最大 1000。

#### `task_series_get`

读取一个循环任务详情。参数为 `id`，返回共享字段、统计、负责人、下一实例时间和
最近实例分组。

#### `task_series_modify`

修改共享字段或未来规则。支持 `title/description/priority/assignees/tags/udas`、
`recurrence_rule`、`effective_from/effective_from_date`、`until/until_date`；修改规则时
必须提供 `effective_from`。`clear` 可包含 `description`、`priority`、`assignees`、
`tags`、`until`、`uda.<name>`。

#### `task_series_stop`

停止循环任务，不再生成后续实例。`delete_open_occurrences=true` 同时删除未完成实例。

#### `task_series_list_occurrences`

分页列出实例。支持 `status=pending|waiting|completed|deleted|all`、
`due_after/due_before`（`YYYY-MM-DD`，包含边界当天）及 `limit/offset`。分页默认
`limit=200, offset=0`，limit 最大 1000。

#### `task_series_occurrence_skip`

跳过一次实例。传 `series_id` 与稳定的 `occurrence_id`；结果是 deleted tombstone，
不会停止整个循环任务。

### 任务附件（4 tools）

只读 metadata 工具。下载附件二进制内容需要使用返回的 `content_url`，配合同一 Bearer
token 走 HTTP `GET`（MCP 不提供 base64 upload/download）。

#### `task_attachment_list`

列出任务的 active 附件。`include_drafts=true` 只返回当前调用者自己的 draft。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |
| `task` | string | 是 | 任务引用 |

#### `task_attachment_get`

读取单个附件 metadata。响应含 `content_url`，需用同一 Bearer token 走 HTTP 下载内容。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |
| `attachment_id` | string | 是 | 附件 ID |

#### `task_attachment_rename`

修改附件展示名。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |
| `attachment_id` | string | 是 | 附件 ID |
| `display_name` | string | 是 | 新展示名 |

#### `task_attachment_remove`

删除附件。若附件仍被 description 引用，返回 `attachment_in_use`。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |
| `attachment_id` | string | 是 | 附件 ID |

### 报表与 Urgency（2 tools）

#### `report_run`

运行内置报表。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |
| `name` | string | 是 | 报表名（list/next/all/completed/deleted/waiting/active/ready/overdue/blocked/blocking） |
| `query` | string | 否 | 附加过滤表达式 |
| `limit` | int | 否 | 最大返回数 |

#### `urgency_explain`

解释任务 urgency 评分。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |
| `id` | string | 是 | |

### Project（13 tools）

#### `project_add`

创建项目。`slug` 仅允许 `^[a-z0-9][a-z0-9_-]*$`。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `slug` | string | 是 | 项目 slug |
| `name` | string | 否 | 显示名称 |
| `description` | string | 否 | 项目描述 |

#### `project_list`

列出 effective workspace 内的项目。只读。每个项目的 `task_count` 统计普通任务和已物化循环实例，并带有 `task_count_scope: "all_tasks"`；如需仅查循环实例，用 `task_query` 的 `task_type=occurrence`。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `include_archived` | bool | 否 | 包含已归档 |

#### `project_get`

读取单个项目及其 agent 配置。只读。返回 `config_summary`（包含 `agent.*` 配置项）。项目对象中的 `task_count` 统计普通任务和已物化循环实例；同时返回 `task_count_scope: "all_tasks"` 明确该范围，如需仅查循环实例用 `task_query` 的 `task_type=occurrence`。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | project slug |
| `project_id` | string | 否 | project UUID |

#### `project_get_current`

读取当前生效的 project scope。只读。返回项目时，`task_count` 统计普通任务和已物化循环实例，并带有 `task_count_scope: "all_tasks"`。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |

#### `project_modify`

修改项目。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |
| `name` | string | 否 | |
| `description` | string | 否 | |

#### `project_archive`

归档项目。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |

#### `project_annotate`

为项目添加注释。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |
| `content` | string | 是 | 注释内容 |

#### `project_denotate`

移除项目注释。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |
| `annotation_id` | string | 是 | 要移除的注释 ID |

#### `project_list_annotations`

列出项目注释。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |

#### `project_list_timeline`

列出项目时间线（project + task 注释聚合）。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |
| `limit` | int | 否 | 默认 50 |

#### `project_config_set`

设置项目配置。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |
| `key` | string | 是 | 配置键 |
| `value` | string | 是 | 配置值 |

#### `project_config_list`

列出项目配置。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |

#### `project_config_unset`

删除项目配置。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |
| `key` | string | 是 | 配置键 |

### Project Automation（14 tools）

项目自动化的管理与观测。规则由 schedule/event 触发，向 OpenAI 兼容 Provider 投递 `chat_completions` 请求。读操作要求 `project:read + hook:read` scope，写操作要求 `project:write + hook:write` scope。provider 配置（`agent.provider.*`）仍通过 `project_config_*` 工具管理。设计见 `docs/superpowers/specs/2026-08-20-project-automation-mcp-tools-design.md`。

所有工具都要求 `project` / `project_id` 至少传一个，可选 `workspace`，下表省略这三个公共参数。

#### `project_automation_list`

列出项目自动化规则。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `include_disabled` | boolean | 否 | 是否包含停用规则（默认 false） |

#### `project_automation_get`

查看单条规则详情。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `rule_id` | string | 是 | 规则 ID |

#### `project_automation_add`

创建规则。`trigger_type` 支持 `schedule`（`trigger_config.schedule_type=daily_at|cron`）和 `event`（`trigger_config.event_type` 为 hook 事件类型）。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `name` | string | 是 | 规则名 |
| `description` | string | 否 | 描述 |
| `enabled` | boolean | 否 | 是否启用（默认 true） |
| `trigger_type` | string | 是 | `schedule` 或 `event` |
| `trigger_config` | object | 是 | `{schedule_type, schedule_value, timezone, event_type}` |
| `condition` | object | 否 | `{task_filter, max_tasks, only_added_assignees}` |
| `action` | object | 是 | `{protocol, base_url_config_key, api_key_config_key, model_config_key, allowed_hosts_config_key, model_override, temperature, max_attempts, attach_metadata}` |
| `context` | object | 否 | `{include: []string}`（遗留字段，模板变量始终全量填充） |
| `instruction_template` | string | 是 | user message 模板，支持 `{{var}}` |
| `system_prompt` | string | 否 | system prompt 模板，默认内置 Agent prompt |

#### `project_automation_modify`

修改规则，省略的字段保持不变。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `rule_id` | string | 是 | 规则 ID |
| 其余字段 | 同 `project_automation_add` | 否 | 全部可选，指针语义 |

#### `project_automation_remove`

删除规则。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `rule_id` | string | 是 | 规则 ID |

#### `project_automation_enable` / `project_automation_disable`

启用/停用规则。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `rule_id` | string | 是 | 规则 ID |

#### `project_automation_test`

立即入队一条 `manual_test` 投递，由后台 dispatcher 真实发送到 Provider。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `rule_id` | string | 是 | 规则 ID |

#### `project_automation_preview`

预览未保存规则渲染出的 OpenAI 兼容请求。secret 脱敏（`Authorization: Bearer ****`），不写任何记录。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| 同 `project_automation_add` | | 是 | 完整规则输入 |

#### `project_automation_preview_saved`

按当前配置预览已保存规则（不支持覆盖字段；预览假设性变更请用 `project_automation_preview`）。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `rule_id` | string | 是 | 规则 ID |

#### `project_automation_delivery_list`

列出投递记录。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `rule_id` | string | 否 | 按规则过滤 |
| `status` | string | 否 | 按状态过滤（queued/claiming/succeeded/retried/failed） |
| `limit` | integer | 否 | 默认 20 |
| `offset` | integer | 否 | |

#### `project_automation_delivery_get`

查看单条投递记录（含 response、usage、重试信息）。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `delivery_id` | string | 是 | 投递 ID |

#### `project_automation_delivery_replay`

基于历史投递新建一条 queued 投递重新发送，原记录不变。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `delivery_id` | string | 是 | 投递 ID |

#### `project_automation_list_template_vars`

列出指令模板可用变量（按 schedule/event 触发器分组）。只读。无业务参数。

### Workspace（7 tools）

#### `workspace_list`

列出可见 workspace。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `include_archived` | bool | 否 | 包含已归档 |

#### `workspace_get_current`

读取当前生效的 workspace。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | 可显式指定 |

#### `workspace_info`

查看 workspace 详情。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 是 | workspace slug 或 UUID |

#### `workspace_add`

创建 workspace。`slug` 仅允许 `^[a-z0-9][a-z0-9_-]*$`。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `slug` | string | 是 | workspace slug |
| `name` | string | 否 | 显示名称 |
| `visibility` | string | 否 | `private`（默认）/`team`/`public` |

#### `workspace_modify`

修改 workspace。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 是 | |
| `name` | string | 否 | |
| `description` | string | 否 | |

#### `workspace_use`

切换 active workspace。仅影响 stdio MCP 隐式状态，Agent 应优先通过参数显式传 `workspace`。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 是 | workspace slug 或 UUID |

#### `workspace_archive`

归档 workspace。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 是 | workspace slug 或 UUID |

### User（7 tools）

#### `user_list`

列出所有用户。只读。

无参数。

#### `user_get`

读取单个用户。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `user` | string | 是 | name/email/UUID |

#### `user_add`

创建用户。用户名支持中文等非 ASCII 字符，系统自动生成 workspace slug。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `name` | string | 是 | 用户名，保持稳定引用语义 |
| `display_name` | string | 否 | 展示姓名，可与 `name` 不同 |
| `email` | string | 否 | 邮箱 |

#### `user_use`

切换 active user。仅影响 stdio MCP 隐式状态。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `user` | string | 是 | name/email/UUID |

#### `user_bind`

为用户绑定外部 ID（如 `feishu_user_id:d8c6g9xx`）。绑定飞书用户时，推荐使用 `feishu_user_id` 作为 `provider`，并将飞书 `user_id` 作为 `external_id`，便于跨应用统一身份。admin/owner 可操作其他用户；普通用户只能绑定自己。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `user` | string | 是 | |
| `provider` | string | 是 | 提供商标识（飞书用户推荐 `feishu_user_id`） |
| `external_id` | string | 是 | 外部系统 ID |

#### `user_unbind`

解绑用户的外部 ID。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `user` | string | 是 | |
| `provider` | string | 是 | |
| `external_id` | string | 是 | |

#### `user_list_external_ids`

列出用户的外部 ID。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `user` | string | 是 | name/email/UUID |

### Member（3 tools）

#### `member_list`

列出 effective workspace 的成员。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |

#### `member_add`

添加成员到 effective workspace。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `user` | string | 是 | 用户引用（name/email/UUID） |
| `role` | string | 否 | `viewer`/`member`/`admin`/`owner`，默认 `member` |

#### `member_role`

修改成员角色。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `user` | string | 是 | 用户引用 |
| `role` | string | 是 | `viewer`/`member`/`admin`/`owner` |

### Context（5 tools）

Context 是预定义的查询过滤器，供 CLI 交互使用。Agent 可以读取 context 了解用户偏好，但不应通过 `context_set`/`context_none` 管理隐式状态。需要过滤时，直接在 `task_query` 的 `query` 参数中指定。

#### `context_get`

查看 active 或指定 context。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `name` | string | 否 | 留空查看 active context |

#### `context_set`

设置 active context。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `name` | string | 是 | context 名；传 `"none"` 清空 |

#### `context_list`

列出所有 context。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |

#### `context_none`

清空 active context。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |

#### `context_delete`

删除 context。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `name` | string | 是 | context 名 |

### Config（4 tools）

#### `config_get`

读取配置。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |
| `key` | string | 是 | 配置键 |
| `scope` | string | 否 | `workspace`（默认）/`project`/`local` |

#### `config_set`

写入配置。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |
| `key` | string | 是 | 配置键 |
| `value` | string | 是 | 配置值 |
| `scope` | string | 是 | `workspace`/`project` |

#### `config_list`

列出配置。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |

#### `config_unset`

删除配置。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `key` | string | 是 | 配置键 |

### Hook（10 tools）

Hook 是事件驱动的机器到机器出站集成。Hook 使用 workspace 级 notification sink，不直接接收 URL 或 secret。支持的事件：`task.created`、`task.modified`、`task.completed`、`task.deleted`、`task.started`、`task.stopped`、`task.assigned`、`task.unassigned`、`task.blocked`、`task.due_changed`、`task.priority_changed`、`task.project_changed`、`task.tags_changed`、`task.unblocked`、`project.archived`、`project.transitioned`、`project.annotated`、`project.denotated`。

#### `hook_add`

创建 Hook。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |
| `name` | string | 是 | Hook 名称 |
| `sink` | string | 是 | 当前 workspace 内的 notification sink 名称或 ID |
| `events` | string[] | 是 | 事件类型列表 |
| `active` | bool | 否 | 是否启用，默认 true |

#### `hook_list`

列出 Hook。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |

#### `hook_info`

查看 Hook 详情。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `hook` | string | 是 | Hook ID |

#### `hook_modify`

修改 Hook。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `hook` | string | 是 | Hook ID |
| `name` | string | 否 | |
| `sink` | string | 否 | 当前 workspace 内的 notification sink 名称或 ID |
| `events` | string[] | 否 | |
| `active` | bool | 否 | |

#### `hook_remove`

删除 Hook。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `hook` | string | 是 | Hook ID |

#### `hook_delivery_list`

列出 Hook 投递记录。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `hook` | string | 是 | Hook ID |
| `limit` | int | 否 | 默认 20 |

#### `hook_delivery_info`

查看投递详情。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `hook` | string | 是 | Hook ID |
| `delivery_id` | string | 是 | 投递 ID |

#### `hook_delivery_redeliver`

重试投递。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `hook` | string | 是 | Hook ID |
| `delivery_id` | string | 是 | 投递 ID |

#### `hook_test`

测试 Hook 配置，生成一次测试投递。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `hook` | string | 是 | Hook ID |

#### `hook_ping`

Ping Hook，生成一次 ping 投递并写审计日志。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `hook` | string | 是 | Hook ID |

### 通知与提醒（24 tools）

通知与提醒包含三类能力：`notification_sink_*` 管理出站 sink，`reminder_rule_*` 管理基于时间和任务过滤器的提醒规则，`notification_rule_*` 管理基于事件的用户通知规则，`notification_delivery_*` 查看和 replay 投递记录。

Notification sink 支持 `max_concurrency` 控制同一 sink 的单进程出站并发。`0` 表示继承 dispatcher 默认 sink 并发；显式大于 `0` 时限制该 workspace 内同一 sink 的并发。`notification_sink_list` / `notification_sink_info` 返回的 sink 对象会包含该字段。

#### `notification_sink_add`

创建 notification sink。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `name` | string | 是 | sink 名称 |
| `type` | string | 否 | `webhook` 或 `http_template` |
| `endpoint_mode` | string | 否 | `static_url`、`template` 或 `config_value` |
| `url` | string | 否 | static endpoint URL |
| `url_template` | string | 否 | 动态 endpoint 模板 |
| `config_key` | string | 否 | `config_value` endpoint 对应配置 key |
| `allowed_hosts` | string[] | 否 | 动态 endpoint 允许的 host |
| `header_templates` | object[] | 否 | HTTP header 模板，保存到数据库 |
| `body_template` | string | 否 | HTTP body 模板，保存到数据库 |
| `body_content_type` | string | 否 | body content type |
| `secret_refs` | object[] | 否 | secret alias 到 secret config key 的映射 |
| `secret` | string | 否 | webhook 签名 secret |
| `timeout_seconds` | int | 否 | |
| `max_attempts` | int | 否 | |
| `max_concurrency` | int | 否 | sink 级并发；`0` 表示继承 dispatcher 默认 sink 并发 |

#### `notification_sink_list`

列出 notification sink。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `include_disabled` | bool | 否 | 是否包含 disabled sink |

#### `notification_sink_info`

查看 notification sink。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `sink` | string | 是 | sink ID |

#### `notification_sink_modify`

修改 notification sink。未传字段保持不变。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `sink` | string | 是 | sink ID |
| `name` | string | 否 | |
| `type` | string | 否 | |
| `endpoint_mode` | string | 否 | |
| `url` | string | 否 | |
| `url_template` | string | 否 | |
| `config_key` | string | 否 | |
| `allowed_hosts` | string[] | 否 | |
| `header_templates` | object[] | 否 | |
| `body_template` | string | 否 | |
| `body_content_type` | string | 否 | |
| `secret_refs` | object[] | 否 | |
| `secret` | string | 否 | |
| `timeout_seconds` | int | 否 | |
| `max_attempts` | int | 否 | |
| `max_concurrency` | int | 否 | sink 级并发；`0` 表示继承 dispatcher 默认 sink 并发 |

#### `notification_sink_enable` / `notification_sink_disable` / `notification_sink_remove`

启用、禁用或删除 notification sink。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `sink` | string | 是 | sink ID |

#### `reminder_rule_add`

创建 reminder rule。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | project slug |
| `project_id` | string | 否 | project UUID |
| `name` | string | 是 | 规则名称 |
| `trigger_type` | string | 否 | 兼容路径：`due_before` 或 `overdue` |
| `offset_seconds` | int64 | 否 | `due_before` 提前秒数 |
| `after_seconds` | int64 | 否 | `overdue` 延迟秒数 |
| `repeat_policy` | string | 否 | `once` 或 `every:<duration>` |
| `schedule_type` | string | 否 | 新规则推荐：`daily_at`；也可用 `daily@HH:MM` |
| `schedule_value` | string | 否 | 每日执行时间，例如 `08:50` |
| `filter_source` | string | 否 | task filter 表达式，例如 `end.isnull and due.before:now` |
| `audience_type` | string | 是 | `assignees`、`explicit_users`、`assignees_and_explicit_users` |
| `recipients` | string[] | 否 | 显式用户引用 |
| `sink` | string | 是 | sink 名称或 ID |

新规则应优先传 `schedule_type/schedule_value/filter_source`。只有使用兼容触发器时才需要 `trigger_type`、`offset_seconds` 或 `after_seconds`。

#### `reminder_rule_list`

列出 reminder rule。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |
| `include_disabled` | bool | 否 | 是否包含 disabled rule |

#### `reminder_rule_info`

查看 reminder rule。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |
| `rule` | string | 是 | rule ID |

#### `reminder_rule_modify`

修改 reminder rule。未传字段保持不变。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | 当前访问 scope |
| `project_id` | string | 否 | 当前访问 scope |
| `rule` | string | 是 | rule ID |
| `name` | string | 否 | |
| `project_ref` | string | 否 | 新规则项目范围，空字符串表示清除项目范围 |
| `trigger_type` | string | 否 | |
| `offset_seconds` | int64 | 否 | |
| `after_seconds` | int64 | 否 | |
| `repeat_policy` | string | 否 | |
| `schedule_type` | string | 否 | |
| `schedule_value` | string | 否 | |
| `filter_source` | string | 否 | |
| `audience_type` | string | 否 | |
| `recipients` | string[] | 否 | |
| `sink` | string | 否 | sink 名称或 ID |

#### `reminder_rule_enable` / `reminder_rule_disable` / `reminder_rule_remove`

启用、禁用或删除 reminder rule。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |
| `rule` | string | 是 | rule ID |

#### `notification_rule_add`

创建事件通知规则。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | project slug |
| `project_id` | string | 否 | project UUID |
| `name` | string | 是 | 规则名称 |
| `event` | string | 是 | 事件类型，例如 `task.unblocked` |
| `filter` | string | 否 | task 事件可用的任务过滤表达式 |
| `audience` | string | 是 | `actor`、`assignees`、`explicit_users`、`assignees_and_explicit_users` |
| `recipients` | string[] | 否 | 显式用户引用 |
| `sink` | string | 是 | sink 名称或 ID |
| `template_subject` | string | 否 | 通知标题模板 |
| `template_body` | string | 否 | 通知正文模板 |

当前允许的事件：`task.created`、`task.modified`、`task.completed`、`task.deleted`、`task.started`、`task.stopped`、`task.assigned`、`task.unassigned`、`task.blocked`、`task.due_changed`、`task.priority_changed`、`task.project_changed`、`task.tags_changed`、`task.unblocked`、`project.archived`、`project.transitioned`、`project.annotated`、`project.denotated`。`assignees` 相关 audience 只支持 `task.*` 事件。

#### `notification_rule_list`

列出事件通知规则。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |
| `include_disabled` | bool | 否 | 是否包含 disabled rule |

#### `notification_rule_info`

查看事件通知规则。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |
| `rule` | string | 是 | notification rule ID |

#### `notification_rule_modify`

修改事件通知规则。未传字段保持不变。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | 当前访问 scope |
| `project_id` | string | 否 | 当前访问 scope |
| `rule` | string | 是 | notification rule ID |
| `name` | string | 否 | |
| `project_ref` | string | 否 | 新规则项目范围，空字符串表示清除项目范围 |
| `event` | string | 否 | |
| `filter` | string | 否 | |
| `audience` | string | 否 | |
| `recipients` | string[] | 否 | |
| `sink` | string | 否 | sink 名称或 ID |
| `template_subject` | string | 否 | |
| `template_body` | string | 否 | |

#### `notification_rule_enable` / `notification_rule_disable` / `notification_rule_remove`

启用、禁用或删除事件通知规则。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |
| `rule` | string | 是 | notification rule ID |

#### `notification_delivery_list`

列出 notification delivery。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `sink` | string | 否 | sink ID |
| `status` | string | 否 | delivery 状态 |
| `limit` | int | 否 | 默认 50 |
| `offset` | int | 否 | |

#### `notification_delivery_info`

查看 notification delivery。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `delivery_id` | string | 是 | delivery ID |

#### `notification_delivery_replay`

重放 `dead_lettered` 或 `disabled_skipped` notification delivery。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `delivery_id` | string | 是 | delivery ID |

### Token（4 tools）

#### `token_list`

列出 Token。只读。HTTP MCP 使用 tenant token 调用时，返回当前 workspace 的 tenant access token 列表；admin-switch 短期 token 默认不出现在常规列表中。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | 限定本次调用的 workspace |
| `project` | string | 否 | 限定本次调用的 project slug |
| `project_id` | string | 否 | 限定本次调用的 project ID |

#### `token_create`

创建 Token。返回中包含 `raw_token`，仅此一次。HTTP MCP 使用 tenant token 调用时，创建的是 `tenant_access_token`。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | 创建目标 workspace；HTTP 模式下也会限定本次调用的 workspace |
| `project` | string | 否 | 创建目标 project slug；HTTP 模式下也会限定本次调用的 project |
| `project_id` | string | 否 | 创建目标 project ID；HTTP 模式下也会限定本次调用的 project |
| `name` | string | 是 | Token 名称 |
| `scope` | string[] | 否 | 权限 scope 列表 |
| `expires_in_seconds` | int | 否 | 过期时间（秒） |

#### `token_modify`

修改 Token。不能修改已撤销或已过期的 Token。HTTP MCP 使用 tenant token 调用时，修改的是 `tenant_access_token`。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | 限定本次调用的 workspace |
| `project` | string | 否 | 限定本次调用的 project slug，不用于修改目标 Token 绑定 |
| `project_id` | string | 否 | 限定本次调用的 project ID，不用于修改目标 Token 绑定 |
| `workspaces` | string[] | 否 | 修改目标 Token 的 workspace 绑定；缺省表示不修改 |
| `projects` | string[] | 否 | 修改目标 Token 的 project slug 绑定；缺省表示不修改 |
| `project_ids` | string[] | 否 | 修改目标 Token 的 project ID 绑定；缺省表示不修改 |
| `token_ref` | string | 是 | Token ID 或 prefix |
| `name` | string | 否 | |
| `scope` | string[] | 否 | |
| `expires_in_seconds` | int | 否 | |

#### `token_revoke`

撤销 Token。HTTP MCP 使用 tenant token 调用时，撤销的是 `tenant_access_token`。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | 限定本次调用的 workspace |
| `project` | string | 否 | 限定本次调用的 project slug |
| `project_id` | string | 否 | 限定本次调用的 project ID |
| `token_ref` | string | 是 | Token ID 或 prefix |

### 系统（3 tools）

#### `audit_list`

列出审计条目。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project_id` | string | 否 | |
| `actor` | string | 否 | 按操作者过滤 |
| `limit` | int | 否 | 默认 50 |
| `offset` | int | 否 | |

#### `scope_list`

列出所有可用 scope。只读。无需鉴权。

无参数。

#### `me_get`

获取当前用户信息。用于确认"我是谁"。无需鉴权。

无参数。

## Resources

当前提供 4 个 resources：

- `xuanchu://workspace/current`
- `xuanchu://workspace/{workspace_id}`
- `xuanchu://project/{project_id}`
- `xuanchu://context/current`

## 返回格式

MCP tool 返回统一 envelope：

```json
{
  "data": {},
  "rendered": "human readable text"
}
```

`data` 给程序读取，`rendered` 给 Agent 或用户直接阅读。

## 常见 Agent 流程

1. `me_get` 确认当前身份。
2. `workspace_list` 发现可用 workspace。
3. `project_list`（带 `workspace` 参数）发现项目。
4. `task_query`（带 `workspace`；需要按项目收窄时再带 `project_id`）查看待办。
5. `task_add` / `task_modify`（带 `workspace`；任务要归属项目时再带 `project_id`）写入任务。
6. `urgency_explain`（带 `workspace`；需要按项目收窄时再带 `project_id`）理解排序原因。

Assignee 用法（M9+）：

- `task_add` 支持 `assignees: ["alice"]`
- `task_modify` 支持 `assignees`、`remove_assignees`
- `task_modify.clear` 允许 `"assignees"`
- `task_query` 支持 `query: "assignee:me"`

## 注意事项

- MCP task tool 接受 UUID、已物化任务的 `task_slug` 或 occurrence_ref；projected 实例只有 occurrence_ref。不要使用本地 working-set ID。
- HTTP MCP 不读取调用者本机 TOML。
- HTTP MCP 不能写 local config。
- Agent 不应依赖 `context_set`/`workspace_use` 等隐式状态操作，每次调用都应显式传参。

## Impersonation（M10）

HTTP MCP 支持 request-scoped impersonation。每个 tool call 的 HTTP 请求携带 `X-Xuanchu-As` header，值为目标用户的 name、email 或 UUID：

```
Authorization: Bearer xuanchu_agent_...
X-Xuanchu-As: alice
```

agent token 必须拥有 `impersonate` scope。权限以目标用户在 workspace 的 membership role 与 token scope 的交集为准。

限制：

- stdio MCP **不支持** impersonation。
- HTTP MCP 的 impersonation 是 request-scoped，不是连接级状态。
- 每个 tool call 必须独立携带 `Authorization` 和 `X-Xuanchu-As` header。
- `/mcp` 不在 OpenAPI 文档中；MCP schema 由 MCP server 暴露。
