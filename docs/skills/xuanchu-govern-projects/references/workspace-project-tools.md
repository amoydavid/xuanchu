# Workspace 与 Project 工具

所有写操作都会记审计日志。只读操作不记。

`description` 字段都是给使用者看的说明文本，默认按 Markdown 编写；技术上仍作为普通字符串传输和存储。

## Workspace 操作

### workspace_list — 列出 workspace（只读）

```json
// 输入
{}

// 输入：包含已归档
{"include_archived": true}

// 返回
{
  "data": {
    "workspaces": [
      {"id": "ws-uuid-xxx", "slug": "local", "name": "local", "visibility": "private", "role": "owner", "active": true},
      {"id": "ws-uuid-yyy", "slug": "engineering", "name": "工程团队", "visibility": "team", "role": "admin"}
    ],
    "count": 2
  },
  "rendered": "2 workspace(s)"
}
```

### workspace_get_current — 读取当前 workspace（只读）

可通过 `workspace` 参数显式指定。

```json
{}

// 显式指定
{"workspace": "engineering"}
```

### workspace_info — 查看 workspace 详情（只读）

`workspace` 必填。

```json
// 输入
{"workspace": "engineering"}

// 返回
{
  "data": {
    "workspace": {
      "id": "ws-uuid-xxx",
      "slug": "engineering",
      "name": "工程团队",
      "description": "公司核心工程团队\n\n- 后端平台\n- 前端体验",
      "visibility": "team",
      "created_by": {"id": "...", "name": "alice"},
      "role": "admin"
    }
  },
  "rendered": "workspace engineering"
}
```

### workspace_add — 创建 workspace

`slug` 必填，仅允许 `^[a-z0-9][a-z0-9_-]*$`。这是 workspace slug 规则，project slug 更严格（见 slug-rules.md）。

```json
// 输入
{"slug": "engineering", "name": "工程团队", "visibility": "team"}

// 返回
{
  "data": {"workspace": {"id": "ws-uuid-new", "slug": "engineering", "name": "工程团队", "visibility": "team"}},
  "rendered": "workspace engineering created"
}
```

### workspace_modify — 修改 workspace

```json
{"workspace": "engineering", "name": "工程部", "description": "合并后的工程部门\n\n- 平台组\n- 应用组"}
```

### workspace_use — 切换 active workspace

> 仅影响 stdio MCP 的隐式状态，HTTP MCP 不受影响。默认优先通过参数显式传 `workspace`。

```json
{"workspace": "engineering"}
```

### workspace_archive — 归档 workspace

```json
{"workspace": "old-team"}
```

## Project 操作

### project_add — 创建项目

`slug` 必填。项目 slug 会转小写，只允许 3-10 位小写字母或数字，必须以字母开头；`api-platform`、`ai_agent`、`p1`、`1api` 都不合法。

```json
// 输入
{
  "workspace": "dajee",
  "slug": "apiplat",
  "name": "API 平台",
  "description": "核心 API 服务\n\n- 对外 REST API\n- 内部 GraphQL 网关"
}

// 返回
{
  "data": {
    "project": {
      "id": "proj-uuid-xxx",
      "slug": "apiplat",
      "name": "API 平台",
      "description": "核心 API 服务\n\n- 对外 REST API\n- 内部 GraphQL 网关",
      "archived": false
    }
  },
  "rendered": "created project apiplat"
}
```

### project_list — 列出项目（只读）

```json
// 输入
{"workspace": "dajee"}

// 输入：包含已归档
{"workspace": "dajee", "include_archived": true}

// 返回
{
  "data": {
    "projects": [
      {"id": "proj-uuid-xxx", "slug": "apiplat", "name": "API 平台", "archived": false}
    ],
    "count": 1
  },
  "rendered": "1 project(s)"
}
```

### project_get — 读取项目详情（只读）

返回 `config_summary`（该 project 全部非 secret 配置：agent 指令、群绑定等集成键都可见；secret 键不回显，用 `config_get` scope=project 读）。

```json
// 输入：用 slug
{"workspace": "dajee", "project": "apiplat"}

// 输入：用 UUID
{"workspace": "dajee", "project_id": "proj-uuid-xxx"}

// 返回
{
  "data": {
    "project": {"id": "proj-uuid-xxx", "slug": "apiplat", "name": "API 平台"},
    "config_summary": {"agent.background": "你是一个 API 开发助手"}
  },
  "rendered": "project apiplat"
}
```

### project_get_current — 读取显式项目 scope（只读）

用于确认当前 tool call 参数中的 `project` / `project_id` 会解析到哪个项目；没有显式项目时返回 `project: null`。

```json
project_get_current({"workspace": "dajee", "project": "apiplat"})
project_get_current({"workspace": "dajee"})
```

### project_modify — 修改项目

```json
{
  "workspace": "dajee",
  "project": "apiplat",
  "name": "API 平台 v2",
  "description": "下一代 API 网关\n\n- 多租户路由\n- 鉴权策略下沉"
}
```

### project_archive — 归档项目

```json
{"workspace": "dajee", "project": "apiplat"}
```

## 项目注释

### project_annotate — 添加注释

```json
{
  "workspace": "dajee",
  "project": "apiplat",
  "content": "决定使用 GraphQL 替代 REST"
}
```

### project_list_annotations — 列出注释（只读）

```json
{"workspace": "dajee", "project": "apiplat"}
```

### project_denotate — 移除注释

```json
{
  "workspace": "dajee",
  "project": "apiplat",
  "annotation_id": "ann-uuid-xxx"
}
```

## 项目时间线

### project_list_timeline — 列出时间线（只读）

默认 50 条。

```json
{"workspace": "dajee", "project": "apiplat", "limit": 10}
```

## 项目配置快捷方式

项目配置也可通过专门 tool 操作（效果等同 `config_get/set` + `scope="project"`）。配置键语义详见 xuanchu-manage-access-and-config skill。

```json
// 设置
project_config_set({"workspace": "dajee", "project": "apiplat", "key": "agent.background", "value": "..."})

// 列出
project_config_list({"workspace": "dajee", "project": "apiplat"})

// 删除
project_config_unset({"workspace": "dajee", "project": "apiplat", "key": "agent.handoff"})
```
