# Task 工具

任务全生命周期操作。所有写操作记审计日志。

## 核心 CRUD

所有参数必须属于 tool schema；未知参数不会被忽略。普通任务的 `task_add` / `task_modify`
不接受旧 `recur`、`clear_recur`、`mask` 或 `imask` 字段，循环任务使用 `task_series_*` tools。

### task_add — 创建任务

`title` 必填，`description` 是可选详细描述。面向使用者时，`description` 默认按 Markdown 文本来写；技术上仍作为普通字符串传输和存储。

```json
// 输入
{
  "workspace": "dajee",
  "project_id": "proj-uuid-xxx",
  "title": "修复登录页面白屏问题",
  "description": "客户反馈: Chrome 121 必现。\n\n- 需要先确认复现路径\n- 关联最近一次前端发布",
  "priority": "H",
  "tags": ["bug", "frontend"],
  "assignees": ["alice"],
  "due": 1748793600,
  "annotations": ["客户反馈: Chrome 121 必现"]
}

// 返回
{
  "data": {
    "id": "a1b2c3d4-...",
    "uuid": "a1b2c3d4-...",
    "task_slug": "apiplat-7",
    "project_seq": 7,
    "workspace_id": "workspace-uuid",
    "title": "修复登录页面白屏问题",
    "description": "客户反馈: Chrome 121 必现。\n\n- 需要先确认复现路径\n- 关联最近一次前端发布",
    "status": "pending",
    "entry": 1748707200,
    "modified": 1748707200,
    "recurrence_info": null,
    "task": {
      "id": "a1b2c3d4-...",
      "uuid": "a1b2c3d4-...",
      "task_slug": "apiplat-7",
      "title": "修复登录页面白屏问题",
      "description": "客户反馈: Chrome 121 必现。\n\n- 需要先确认复现路径\n- 关联最近一次前端发布",
      "status": "pending",
      "priority": "H",
      "tags": ["bug", "frontend"],
      "project": "apiplat",
      "assignees": [{"id": "...", "name": "alice"}],
      "annotations": [{"id": "annotation-uuid", "entry": 1748793600, "description": "客户反馈: Chrome 121 必现"}],
      "entry": 1748707200,
      "modified": 1748707200
    }
  },
  "rendered": "Created task a1b2c3d4-..."
}
```

### task_query — 查询任务（只读）

`query` 使用璇础任务过滤表达式（语法见 query-syntax.md）。裸字符串自动按 title 子串匹配。

状态可见性：

- 没有显式状态条件时，默认返回所有非 deleted 任务，包括 pending、waiting、completed。
- `include_deleted:true` 在默认集合上追加 deleted。
- `status` 参数或 `query` 中的 status 条件优先；此时不注入默认条件，并忽略 `include_deleted`。
- 不使用 `include_completed`；completed 已在默认结果中。

分页、排序和循环任务参数：`sort`、`limit`、`offset`、`due_after`、`due_before`、`occurrence_mode`（`auto`/`materialized`/`expand`）、`task_type`（`all`/`normal`/`occurrence`）。

```json
// 输入：查看所有非删除任务（包含已完成）
{
  "workspace": "dajee"
}

// 输入：在默认集合上追加已删除任务
{
  "workspace": "dajee",
  "include_deleted": true
}

// 输入：只看已删除任务；显式状态条件优先
{
  "workspace": "dajee",
  "query": "status:deleted"
}
```

```json
// 输入：查看高优先级待办
{
  "workspace": "dajee",
  "project_id": "proj-uuid-xxx",
  "query": "status:pending priority:H",
  "limit": 10
}

// 输入：搜索标题包含"白屏"的任务（裸字符串等价于 title:白屏）
{
  "workspace": "dajee",
  "query": "白屏"
}

// 输入：搜索详细描述包含"Chrome 121"的任务
{
  "workspace": "dajee",
  "query": "description:\"Chrome 121\""
}

// 返回
{
  "data": {
    "items": [
      {"id": "a1b2c3d4-...", "uuid": "a1b2c3d4-...", "task_slug": "apiplat-7", "title": "修复登录页面白屏问题", "status": "pending", "priority": "H", "recurrence_info": null}
    ],
    "total": 1,
    "limit": 10,
    "offset": 0,
    "occurrence_mode": "materialized"
  },
  "rendered": "1 task(s)"
}
```

### task_get — 读取单任务（只读）

任务引用使用 UUID、已物化 `task_slug` 或 occurrence_ref；projected 实例只有 occurrence_ref。不能用本地 working-set 数字 ID。

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
    "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "uuid": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "task_slug": "apiplat-7",
    "recurrence_info": null,
    "task": {
      "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
      "uuid": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
      "title": "修复登录页面白屏问题",
      "description": "客户反馈: Chrome 121 必现。\n\n- 需要先确认复现路径\n- 关联最近一次前端发布",
      "status": "pending",
      "priority": "H",
      "tags": ["bug", "frontend"],
      "assignees": [{"id": "...", "name": "alice"}],
      "annotations": [],
      "links": [],
      "entry": 1748707200,
      "modified": 1748707200
    }
  },
  "rendered": "修复登录页面白屏问题"
}
```

### task_modify — 修改任务

```json
// 修改标题和优先级
{
  "workspace": "dajee",
  "project_id": "proj-uuid-xxx",
  "id": "a1b2c3d4-...",
  "title": "修复登录页面白屏问题（已定位根因）",
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

`task_query` 和 `task_get` 返回的 `annotations[]` 都包含稳定 `id`。已选定单个任务时优先用 `task_get` 读取最新 annotations，再把目标 `id` 传给 `task_denotate`。

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

`task_import` 只接受版本化 `xuanchu.task-bundle/v1`，与 HTTP、Remote 和 CLI 的跨环境迁移格式相同。不接受裸任务数组或 Taskwarrior JSON。

```json
{
  "workspace": "dajee",
  "project_id": "proj-uuid-xxx",
  "bundle": {
    "schema": "xuanchu.task-bundle/v1",
    "exported_at": "2026-07-14T12:00:00Z",
    "task_series": [],
    "tasks": []
  }
}
```
