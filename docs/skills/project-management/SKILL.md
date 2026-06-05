# 项目管理

通过 taskg MCP 管理项目实体、注释、时间线和配置。

## 重要原则

**每个 MCP 调用都必须显式指定 `workspace`。** 项目操作通过 `project`（slug）或 `project_id`（UUID）定位。如果不知道项目 ID，先 `project_list` 查询。

## 项目生命周期

### project_add — 创建项目

`slug` 必填，仅允许 `^[a-z0-9][a-z0-9_-]*$`。

```json
// 输入
{
  "workspace": "dajee",
  "slug": "api-platform",
  "name": "API 平台",
  "description": "核心 API 服务"
}

// 返回
{
  "data": {
    "project": {
      "id": "proj-uuid-xxx",
      "slug": "api-platform",
      "name": "API 平台",
      "description": "核心 API 服务",
      "archived": false
    }
  },
  "rendered": "created project api-platform"
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
      {"id": "proj-uuid-xxx", "slug": "api-platform", "name": "API 平台", "archived": false}
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
{"workspace": "dajee", "project": "api-platform"}

// 输入：用 UUID
{"workspace": "dajee", "project_id": "proj-uuid-xxx"}

// 返回
{
  "data": {
    "project": {"id": "proj-uuid-xxx", "slug": "api-platform", "name": "API 平台"},
    "config_summary": {"agent.background": "你是一个 API 开发助手"}
  },
  "rendered": "project api-platform"
}
```

### project_modify — 修改项目

```json
{
  "workspace": "dajee",
  "project": "api-platform",
  "name": "API 平台 v2",
  "description": "下一代 API 网关"
}
```

### project_archive — 归档项目

```json
{"workspace": "dajee", "project": "api-platform"}
```

## 注释

### project_annotate — 添加注释

```json
{
  "workspace": "dajee",
  "project": "api-platform",
  "content": "决定使用 GraphQL 替代 REST"
}
```

### project_list_annotations — 列出注释

只读。

```json
{"workspace": "dajee", "project": "api-platform"}
```

### project_denotate — 移除注释

```json
{
  "workspace": "dajee",
  "project": "api-platform",
  "annotation_id": "ann-uuid-xxx"
}
```

## 时间线

### project_list_timeline — 列出时间线

只读。默认 50 条。

```json
{"workspace": "dajee", "project": "api-platform", "limit": 10}
```

## 项目配置

以 `agent.` 开头的配置项会被 `project_get` 的 `config_summary` 暴露给 Agent。

### project_config_set

```json
{
  "workspace": "dajee",
  "project": "api-platform",
  "key": "agent.background",
  "value": "你是一个 API 开发助手，负责 review 所有 API 变更"
}
```

### project_config_list

只读。

```json
{"workspace": "dajee", "project": "api-platform"}
```

### project_config_unset

```json
{"workspace": "dajee", "project": "api-platform", "key": "agent.handoff"}
```

## 典型 Agent 工作流

**场景：用户说"帮我创建一个新项目并配置 Agent 指令"**

```json
// Step 1: 创建项目
project_add({"workspace": "dajee", "slug": "api-platform", "name": "API 平台"})

// Step 2: 配置 Agent 指令
project_config_set({
  "workspace": "dajee",
  "project": "api-platform",
  "key": "agent.background",
  "value": "你负责管理 API 平台项目的所有任务"
})

// Step 3: 创建首批任务
task_add({
  "workspace": "dajee",
  "project": "api-platform",
  "description": "搭建项目骨架"
})
```
