---
title: "MCP 使用指南"
weight: 80
---

# MCP 使用指南

taskg 可以作为 MCP Server，让 Agent 通过结构化 tools/resources 访问任务系统。

MCP 调用必须有明确身份。stdio MCP 使用本机 active user/workspace；HTTP MCP 使用 Bearer token 绑定的 user 和 scope。完整身份初始化流程见 [身份与初始化](identity-and-initialization.md)。

## 运行模式

本地 stdio MCP：

```bash
taskg mcp stdio
```

HTTP MCP：

```bash
taskg server --listen :8080
```

HTTP MCP endpoint：

```text
/mcp
```

HTTP MCP 使用 Bearer token 鉴权，复用远程 CLI 和 HTTP API 的 token scope。

## 接入前准备

如果使用本地 stdio MCP，先确认 `taskg` 在 PATH 中，或记录完整路径：

```bash
which taskg
```

如果使用 HTTP MCP，先启动服务端并创建 token：

```bash
taskg server --listen :8080

taskg --workspace dajee token create mcp-agent \
  --type agent \
  --scope task:read,task:write,project:read,context:read,config:read \
  --project ai-agent-platform \
  --expires-in 720h
```

HTTP MCP 推荐使用 project-scoped Agent token。这样 Agent 即使拿到 MCP tools，也只能访问 token allowlist 内的 project。

## 在 Claude Code 中使用

Claude Code 支持 stdio 和 HTTP MCP。推荐优先使用 HTTP MCP 连接生产服务端，使用 stdio MCP 连接本机开发库。

### Claude Code 本地 stdio

```bash
claude mcp add --transport stdio taskg -- taskg mcp stdio
```

如果 `taskg` 不在 PATH 中，使用完整路径：

```bash
claude mcp add --transport stdio taskg -- /path/to/taskg mcp stdio
```

指定本地数据库：

```bash
claude mcp add --transport stdio taskg -- /path/to/taskg --db /path/to/taskg.db mcp stdio
claude mcp add --transport stdio taskg -- /path/to/taskg --db-url "postgres://user:pass@localhost:5432/taskg?sslmode=disable" mcp stdio
```

### Claude Code 远程 HTTP

```bash
claude mcp add \
  --transport http \
  --header "Authorization: Bearer $TASKG_TOKEN" \
  taskg \
  https://taskg.example.com/mcp
```

检查连接状态：

```bash
claude mcp list
claude mcp get taskg
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
    "taskg": {
      "type": "stdio",
      "command": "/path/to/taskg",
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
    "taskg": {
      "type": "http",
      "url": "https://taskg.example.com/mcp",
      "headers": {
        "Authorization": "Bearer ${TASKG_TOKEN}"
      }
    }
  }
}
```

如果客户端不展开 `${TASKG_TOKEN}`，请改用该客户端支持的 secret manager、header helper，或只把配置放在用户级私有配置里。

## 在 OpenClaw 中使用

OpenClaw 的 `openclaw mcp serve` 是“OpenClaw 自己作为 MCP server”。这里要做的是相反方向：让 OpenClaw 托管的 agent 使用 taskg MCP server，所以应使用 OpenClaw 的 MCP client registry，也就是 `openclaw mcp add/set/configure/probe`。

### OpenClaw 本地 stdio

```bash
openclaw mcp add taskg \
  --command /path/to/taskg \
  --arg mcp \
  --arg stdio
```

指定数据库：

```bash
openclaw mcp add taskg \
  --command /path/to/taskg \
  --arg --db \
  --arg /path/to/taskg.db \
  --arg mcp \
  --arg stdio
```

### OpenClaw 远程 HTTP

```bash
openclaw mcp set taskg '{
  "url": "https://taskg.example.com/mcp",
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
openclaw mcp doctor taskg --probe
openclaw mcp probe taskg --json
```

如果只希望 OpenClaw 暴露一部分 taskg MCP tools，可以配置 tool filter。例如只让 Agent 查询任务和读取项目：

```bash
openclaw mcp tools taskg --include 'task_query,task_get,project_list,project_get,workspace_get_current'
```

注意：OpenClaw 文档中 `streamable-http` 是 Streamable HTTP 的规范写法；taskg 的 `/mcp` 就是这个 HTTP MCP endpoint。

## 在 Hermes Agent 中使用

Hermes Agent 从 `~/.hermes/config.yaml` 的 `mcp_servers` 读取 MCP 配置。它支持本地 stdio server 和远程 HTTP MCP server。

### Hermes 本地 stdio

```yaml
mcp_servers:
  taskg:
    command: "/path/to/taskg"
    args: ["mcp", "stdio"]
```

指定数据库：

```yaml
mcp_servers:
  taskg:
    command: "/path/to/taskg"
    args: ["--db", "/path/to/taskg.db", "mcp", "stdio"]
```

### Hermes 远程 HTTP

```yaml
mcp_servers:
  taskg:
    url: "https://taskg.example.com/mcp"
    headers:
      Authorization: "Bearer ${TASKG_TOKEN}"
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
mcp_taskg_task_query
mcp_taskg_task_add
mcp_taskg_project_list
```

通常不需要手动调用这些名字；让 Agent 用自然语言描述目标即可。

## Scope

stdio MCP 使用本地 runtime context。HTTP MCP 使用请求 token 解析 actor、workspace 和 project scope。

Agent 不应该靠提示词决定权限。权限来自：

- token capability
- workspace scope
- project scope
- membership role

stdio MCP 启动前，建议先确认本机 active user/workspace：

```bash
taskg _show active.user active.workspace
```

HTTP MCP 接入前，建议为 Agent 创建 project-scoped token：

```bash
taskg --workspace dajee token create mcp-agent \
  --type agent \
  --scope task:read,task:write,project:read,context:read,config:read \
  --project ai-agent-platform \
  --expires-in 720h
```

## 建议给 Agent 的提示词

可以在 Agent 系统提示词或项目说明中加入：

```text
你可以使用 taskg MCP 管理任务。优先使用 project_get_current / workspace_get_current 确认作用域；
查询任务用 task_query，读取单任务用 task_get，新增任务用 task_add。
如果任务有执行者，请在 task_add / task_modify 里显式传 assignees。
不要尝试访问 token scope 之外的 workspace/project。
写入任务前，如果 project 不明确，先询问用户或调用 project_list。
```

## Tools

当前提供 33 个 tools。

### 任务

#### `task_add`

创建任务。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | workspace slug 或 UUID |
| `project` | string | 否 | project slug |
| `project_id` | string | 否 | project UUID |
| `description` | string | 是 | 任务描述 |
| `tags` | string[] | 否 | 要添加的标签 |
| `assignees` | string[] | 否 | workspace 用户引用（name/email/UUID/外部ID） |
| `priority` | string | 否 | `H`/`M`/`L` |
| `due` | int64 | 否 | unix 秒 |
| `wait` | int64 | 否 | unix 秒 |
| `scheduled` | int64 | 否 | unix 秒 |
| `until` | int64 | 否 | unix 秒 |
| `annotations` | string[] | 否 | 初始注释 |

#### `task_query`

查询任务。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | workspace slug 或 UUID |
| `project` | string | 否 | project slug |
| `project_id` | string | 否 | project UUID |
| `query` | string | 否 | Taskwarrior 风格查询表达式 |
| `status` | string | 否 | 按状态过滤 |
| `limit` | int | 否 | 最大返回数，默认 200，最大 1000 |
| `include_completed` | bool | 否 | 包含已完成 |
| `include_deleted` | bool | 否 | 包含已删除 |

#### `task_get`

读取单个任务。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |
| `id` | string | 是 | 任务 UUID；stdio 可用 working-set ID |

#### `task_modify`

修改任务。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |
| `id` | string | 是 | 任务 UUID 或 working-set ID |
| `description` | string | 否 | 新描述 |
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
| `clear` | string[] | 否 | 要清空的字段：`project`/`priority`/`due`/`wait`/`scheduled`/`until`/`recur`/`assignees`/`uda.*` |

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
| `task` | string | 是 | 任务引用（UUID 或 working-set ID） |
| `type` | string | 是 | 链接类型（document/pr/ticket/design 等） |
| `url` | string | 是 | 外部资源 URL |
| `title` | string | 否 | 显示标题 |

#### `task_link_remove`

移除任务的外部关联链接。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | |
| `project_id` | string | 否 | |
| `task` | string | 是 | 任务引用 |
| `link_id` | string | 是 | 要移除的链接 ID |

### 报表与 Urgency

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

### Workspace

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

### Project

#### `project_list`

列出 effective workspace 内的项目。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `include_archived` | bool | 否 | 包含已归档 |

#### `project_get`

读取单个项目及其 agent 配置。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `workspace` | string | 否 | |
| `project` | string | 否 | project slug |
| `project_id` | string | 否 | project UUID |

#### `project_get_current`

读取当前生效的 project scope。只读。

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

### Member

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

### User

#### `user_list`

列出所有用户。只读。

无参数。

#### `user_get`

读取单个用户。只读。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `user` | string | 是 | name/email/UUID |

#### `user_bind`

为用户绑定外部 ID（如 `feishu:ou_xxxxx`）。admin/owner 可操作其他用户；普通用户只能绑定自己。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `user` | string | 是 | |
| `provider` | string | 是 | 提供商标识（如 `feishu`） |
| `external_id` | string | 是 | 外部系统 ID |

#### `user_unbind`

解绑用户的外部 ID。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `user` | string | 是 | |
| `provider` | string | 是 | |
| `external_id` | string | 是 | |

### Context

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

### Config

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

## Resources

当前提供 4 个 resources：

- `taskg://workspace/current`
- `taskg://workspace/{workspace_id}`
- `taskg://project/{project_id}`
- `taskg://context/current`

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

1. `workspace_get_current` 确认当前 workspace。
2. `project_list` 或 `project_get_current` 确认项目。
3. `task_query` 查看待办。
4. `task_add` 或 `task_modify` 写入任务。
5. `urgency_explain` 理解排序原因。

M9 之后，常见 assignee 用法：

- `task_add` 支持 `assignees: ["alice"]`
- `task_modify` 支持 `assignees`、`remove_assignees`
- `task_modify.clear` 允许 `"assignees"`
- `task_query` 支持 `query: "assignee:me"`

## 注意事项

- HTTP MCP 的 `task_get` 只承诺 UUID，不使用本地 working-set ID。
- HTTP MCP 不读取调用者本机 TOML。
- HTTP MCP 不能写 local config。

## Impersonation（M10）

HTTP MCP 支持 request-scoped impersonation。每个 tool call 的 HTTP 请求携带 `X-Taskg-As` header，值为目标用户的 name、email 或 UUID：

```
Authorization: Bearer taskg_agent_...
X-Taskg-As: alice
```

agent token 必须拥有 `impersonate` scope。权限以目标用户在 workspace 的 membership role 与 token scope 的交集为准。

限制：

- stdio MCP **不支持** impersonation。
- HTTP MCP 的 impersonation 是 request-scoped，不是连接级状态。
- 每个 tool call 必须独立携带 `Authorization` 和 `X-Taskg-As` header。
- `/mcp` 不在 OpenAPI 文档中；MCP schema 由 MCP server 暴露。
