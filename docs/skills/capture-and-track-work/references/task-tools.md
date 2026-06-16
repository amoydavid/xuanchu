# Task 工具

任务全生命周期操作。所有写操作记审计日志。

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
      "project": "apiplat",
      "assignees": [{"id": "...", "name": "alice"}],
      "annotations": [{"id": "annotation-uuid", "entry": 1748793600, "description": "客户反馈: Chrome 121 必现"}],
      "entry": 1748707200,
      "urgency": 8.9
    }
  },
  "rendered": "Created task a1b2c3d4-..."
}
```

### task_query — 查询任务（只读）

`query` 支持 Taskwarrior 风格表达式（语法见 query-syntax.md）。裸字符串自动按 description 子串匹配。

```json
// 输入：查看高优先级待办
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

### task_get — 读取单任务（只读）

任务引用只能用 UUID 或 `task_slug`，不能用本地 working-set 数字 ID。

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
      "annotations": [],
      "links": [],
      "urgency": 8.9
    }
  },
  "rendered": "修复登录页面白屏问题"
}
```

### task_modify — 修改任务

```json
// 修改描述和优先级
{
  "workspace": "dajee",
  "project_id": "proj-uuid-xxx",
  "id": "a1b2c3d4-...",
  "description": "修复登录页面白屏问题（已定位根因）",
  "priority": "M"
}

// 添加标签和执行者
{
  "workspace": "dajee",
  "project_id": "proj-uuid-xxx",
  "id": "a1b2c3d4-...",
  "tags": ["in-progress"],
  "assignees": ["bob"]
}

// 清空优先级和执行者
{
  "workspace": "dajee",
  "project_id": "proj-uuid-xxx",
  "id": "a1b2c3d4-...",
  "clear": ["priority", "assignees"]
}

// 设置 UDA
{
  "workspace": "dajee",
  "project_id": "proj-uuid-xxx",
  "id": "a1b2c3d4-...",
  "udas": {"estimate": "3h", "risk": "high"}
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

`annotation_id` 是任务注释的稳定 ID。先通过 `task_get`（返回的 annotation 含 `id`）读取 `annotations[].id`，不要使用显示顺序删除注释。

> `task_query` 的返回默认不带 annotations；要拿 annotation ID 用 `task_get`。

```json
{
  "workspace": "dajee",
  "project_id": "proj-uuid-xxx",
  "id": "a1b2c3d4-...",
  "annotation_id": "annotation-uuid"
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

### task_link_list（只读）

```json
{"workspace": "dajee", "project_id": "proj-uuid-xxx", "task": "a1b2c3d4-..."}
```

### task_link_remove

```json
{"workspace": "dajee", "project_id": "proj-uuid-xxx", "task": "a1b2c3d4-...", "link_id": "link-uuid-xxx"}
```

## 批量操作

### task_export — 导出（只读）

```json
{"workspace": "dajee", "project_id": "proj-uuid-xxx"}
```

### task_import — 导入

`entry` / `modified` 用 ISO8601 字符串（与 `task_add` / `task_modify` 的 `due` 用 Unix 秒不同）。

```json
{
  "workspace": "dajee",
  "project_id": "proj-uuid-xxx",
  "tasks": [
    {"uuid": "imported-001", "description": "导入的任务", "status": "pending", "entry": "2025-06-01T00:00:00Z", "modified": "2025-06-01T00:00:00Z"}
  ]
}
```
