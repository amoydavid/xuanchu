# 任务管理

通过 Xuanchu MCP 管理任务的生命周期、依赖、关联和批量操作。

## 重要原则

**每个 MCP 调用都必须显式指定 workspace。** 不要依赖"当前上下文"或隐式状态。所有 task tool 都支持 `workspace`、`project`、`project_id` 参数；任务要归属某个 project，或查询要按 project 收窄时，再把 `project`/`project_id` 带上。

正确做法：
```json
task_add({"description": "修复白屏", "workspace": "dajee", "project_id": "proj-uuid-xxx"})
task_query({"workspace": "dajee"})
```

错误做法：
```json
// 不依赖 context_set 或 workspace_use 的隐式状态
task_add({"description": "修复白屏"})
```

## 发现作用域

如果不知道 workspace 或 project，先查询：

```json
// 1. 发现可用 workspace
workspace_list({})

// 2. 发现 workspace 下的项目
project_list({"workspace": "dajee"})

// 3. 任务需要归属或收窄到具体 project 时，带上 project_id
```

## 核心 CRUD

### task_add — 创建任务

`description` 必填。

```json
// 输入
{
  "workspace": "dajee",
  "project_id": "proj-uuid-xxx",
  "description": "修复登录页面白屏问题",
  "priority": "H",
  "tags": ["bug", "frontend"],
  "assignees": ["alice"],
  "due": 1748793600,
  "annotations": ["客户反馈: Chrome 121 必现"]
}

// 返回
{
  "data": {
    "task": {
      "uuid": "a1b2c3d4-...",
      "description": "修复登录页面白屏问题",
      "status": "pending",
      "priority": "H",
      "tags": ["bug", "frontend"],
      "project": "api-platform",
      "assignees": [{"id": "...", "name": "alice"}],
      "annotations": [{"entry": 1748793600, "description": "客户反馈: Chrome 121 必现"}],
      "entry": 1748707200,
      "urgency": 8.9
    }
  },
  "rendered": "Created task a1b2c3d4-..."
}
```

### task_query — 查询任务

只读。`query` 支持 Taskwarrior 风格表达式：`description:关键词`、`tag:xxx`、`assignee:me`、`project:slug`、`priority:H`、`due.before:today`、`annotations contains "备注"`。裸字符串自动按 description 子串匹配。

```json
// 输入：查看 dajee workspace 下 api-platform 项目的高优先级待办
{
  "workspace": "dajee",
  "project_id": "proj-uuid-xxx",
  "query": "status:pending priority:H",
  "limit": 10
}

// 输入：搜索描述包含"白屏"的任务
{
  "workspace": "dajee",
  "query": "白屏"
}

// 返回
{
  "data": {
    "tasks": [
      {"uuid": "a1b2c3d4-...", "description": "修复登录页面白屏问题", "status": "pending", "priority": "H", "urgency": 8.9}
    ],
    "count": 1
  },
  "rendered": "1 task(s)"
}
```

### task_get — 读取单任务

只读。HTTP 和 stdio MCP 模式都只能用 UUID 或 `task_slug`，不能使用本地 working-set 数字 ID。

```json
// 输入
{
  "workspace": "dajee",
  "project_id": "proj-uuid-xxx",
  "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
}

// 返回
{
  "data": {
    "task": {
      "uuid": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
      "description": "修复登录页面白屏问题",
      "status": "pending",
      "priority": "H",
      "tags": ["bug", "frontend"],
      "assignees": [{"id": "...", "name": "alice"}],
      "annotations": [...],
      "links": [],
      "urgency": 8.9
    }
  },
  "rendered": "修复登录页面白屏问题"
}
```

### task_modify — 修改任务

```json
// 输入：修改描述和优先级
{
  "workspace": "dajee",
  "project_id": "proj-uuid-xxx",
  "id": "a1b2c3d4-...",
  "description": "修复登录页面白屏问题（已定位根因）",
  "priority": "M"
}

// 输入：添加标签和执行者
{
  "workspace": "dajee",
  "project_id": "proj-uuid-xxx",
  "id": "a1b2c3d4-...",
  "tags": ["in-progress"],
  "assignees": ["bob"]
}

// 输入：清空优先级和执行者
{
  "workspace": "dajee",
  "project_id": "proj-uuid-xxx",
  "id": "a1b2c3d4-...",
  "clear": ["priority", "assignees"]
}
```

### task_done — 完成任务

```json
{"workspace": "dajee", "project_id": "proj-uuid-xxx", "id": "a1b2c3d4-..."}
```

### task_delete — 删除任务

```json
{"workspace": "dajee", "project_id": "proj-uuid-xxx", "id": "a1b2c3d4-..."}
```

## 时间追踪

### task_start / task_stop

```json
task_start({"workspace": "dajee", "project_id": "proj-uuid-xxx", "id": "a1b2c3d4-..."})
task_stop({"workspace": "dajee", "project_id": "proj-uuid-xxx", "id": "a1b2c3d4-..."})
```

## 注释

### task_annotate — 添加注释

```json
{
  "workspace": "dajee",
  "project_id": "proj-uuid-xxx",
  "id": "a1b2c3d4-...",
  "annotation": "进度更新：已完成接口开发，等待联调"
}
```

### task_denotate — 移除注释

`annotation_index` 是 1-based（按时间排序）。

```json
{
  "workspace": "dajee",
  "project_id": "proj-uuid-xxx",
  "id": "a1b2c3d4-...",
  "annotation_index": 1
}
```

## 依赖关系

### task_depends — 调整依赖

```json
// B 依赖 A
{
  "workspace": "dajee",
  "id": "任务B-uuid",
  "depends": ["任务A-uuid"]
}

// 清空所有依赖
{
  "workspace": "dajee",
  "id": "任务B-uuid",
  "clear_depends": true
}
```

## 外部关联

`type` 常用值：`document`、`pr`、`ticket`、`design`、`issue`。

### task_link_add

```json
{
  "workspace": "dajee",
  "project_id": "proj-uuid-xxx",
  "task": "a1b2c3d4-...",
  "type": "pr",
  "url": "https://github.com/org/repo/pull/42",
  "title": "PR #42: 修复白屏问题"
}
```

### task_link_list

```json
{"workspace": "dajee", "project_id": "proj-uuid-xxx", "task": "a1b2c3d4-..."}
```

### task_link_remove

```json
{"workspace": "dajee", "project_id": "proj-uuid-xxx", "task": "a1b2c3d4-...", "link_id": "link-uuid-xxx"}
```

## 批量操作

### task_export — 导出

只读。

```json
{"workspace": "dajee", "project_id": "proj-uuid-xxx"}
```

### task_import — 导入

```json
{
  "workspace": "dajee",
  "project_id": "proj-uuid-xxx",
  "tasks": [
    {"uuid": "imported-001", "description": "导入的任务", "status": "pending", "entry": "2025-06-01T00:00:00Z", "modified": "2025-06-01T00:00:00Z"}
  ]
}
```

## 报表与评分

### report_run — 运行内置报表

只读。内置报表：`list`、`next`、`all`、`completed`、`deleted`、`waiting`、`active`、`ready`、`overdue`、`blocked`、`blocking`。

```json
{"workspace": "dajee", "project_id": "proj-uuid-xxx", "name": "next", "limit": 10}
```

### urgency_explain — 解释 urgency 评分

只读。

```json
{"workspace": "dajee", "project_id": "proj-uuid-xxx", "id": "a1b2c3d4-..."}
```

## 典型 Agent 工作流

**场景：用户说"帮我在 API 平台项目下创建一个修复白屏的任务"**

```json
// Step 1: 确认 workspace 和项目
project_list({"workspace": "dajee"})
// → 找到 api-platform 项目，拿到 project_id

// Step 2: 创建任务
task_add({
  "workspace": "dajee",
  "project_id": "proj-api-platform-uuid",
  "description": "修复登录页面白屏问题",
  "priority": "H",
  "tags": ["bug"]
})

// Step 3: 记录进展
task_annotate({
  "workspace": "dajee",
  "project_id": "proj-api-platform-uuid",
  "id": "新创建的任务 UUID",
  "annotation": "已创建，等待分配"
})
```
