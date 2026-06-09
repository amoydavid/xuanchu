# 项目管理

通过 Xuanchu MCP 管理项目实体、注释、时间线和配置。

## 重要原则

**每个 MCP 调用都必须显式指定 `workspace`。** 项目操作通过 `project`（项目 slug）或 `project_id`（UUID）定位。如果不知道项目 ID，先 `project_list` 查询。

项目 slug 规则与 workspace slug 不同：创建项目时会转小写，只允许 3-10 位小写字母或数字，必须以字母开头，不能包含 `-`、`_` 或中文。示例：`api`、`apiplat`、`api9`。

## 项目生命周期

### project_add — 创建项目

`slug` 必填。项目 slug 会转小写，只允许 3-10 位小写字母或数字，必须以字母开头；`api-platform`、`ai_agent`、`p1`、`1api` 都不是合法项目 slug。

```json
// 输入
{
  "workspace": "dajee",
  "slug": "apiplat",
  "name": "API 平台",
  "description": "核心 API 服务"
}

// 返回
{
  "data": {
    "project": {
      "id": "proj-uuid-xxx",
      "slug": "apiplat",
      "name": "API 平台",
      "description": "核心 API 服务",
      "archived": false
    }
  },
  "rendered": "created project apiplat"
}
```

### project_list — 列出项目

只读。

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

### project_get — 读取项目详情

只读。返回 `config_summary`（包含以 `agent.` 开头的配置项）。

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

### project_get_current — 读取显式项目 scope

只读。用于确认当前 tool call 参数中的 `project` / `project_id` 会解析到哪个项目；没有显式项目时返回 `project: null`。

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
  "description": "下一代 API 网关"
}
```

### project_archive — 归档项目

```json
{"workspace": "dajee", "project": "apiplat"}
```

## 注释

### project_annotate — 添加注释

```json
{
  "workspace": "dajee",
  "project": "apiplat",
  "content": "决定使用 GraphQL 替代 REST"
}
```

### project_list_annotations — 列出注释

只读。

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

## 时间线

### project_list_timeline — 列出时间线

只读。默认 50 条。

```json
{"workspace": "dajee", "project": "apiplat", "limit": 10}
```

## 项目配置

以 `agent.` 开头的配置项会被 `project_get` 的 `config_summary` 暴露给 Agent。

### project_config_set

```json
{
  "workspace": "dajee",
  "project": "apiplat",
  "key": "agent.background",
  "value": "你是一个 API 开发助手，负责 review 所有 API 变更"
}
```

### project_config_list

只读。

```json
{"workspace": "dajee", "project": "apiplat"}
```

### project_config_unset

```json
{"workspace": "dajee", "project": "apiplat", "key": "agent.handoff"}
```

## 典型 Agent 工作流

**场景：用户说"帮我创建一个新项目并配置 Agent 指令"**

```json
// Step 1: 创建项目
project_add({"workspace": "dajee", "slug": "apiplat", "name": "API 平台"})

// Step 2: 配置 Agent 指令
project_config_set({
  "workspace": "dajee",
  "project": "apiplat",
  "key": "agent.background",
  "value": "你负责管理 API 平台项目的所有任务"
})

// Step 3: 创建首批任务
task_add({
  "workspace": "dajee",
  "project": "apiplat",
  "description": "搭建项目骨架"
})
```
