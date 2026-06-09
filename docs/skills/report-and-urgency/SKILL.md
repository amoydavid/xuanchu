# 报表与 Urgency

通过 Xuanchu MCP 运行任务报表，并解释单个任务的 urgency 评分。

## 重要原则

- 每次调用都显式传 `workspace`。
- 需要按项目收窄时传 `project` 或 `project_id`。
- `report_run.query` 使用 Xuanchu 查询表达式，可叠加在内置报表上。
- 任务引用只使用 UUID 或 `task_slug`，不要使用本地 working-set 数字 ID。

## report_run — 运行内置报表

只读。`name` 必填。内置报表包括：`list`、`next`、`all`、`completed`、`deleted`、`waiting`、`active`、`ready`、`overdue`、`blocked`、`blocking`。

```json
// 查看下一批待办
report_run({
  "workspace": "dajee",
  "project_id": "proj-uuid-xxx",
  "name": "next",
  "limit": 10
})

// 在报表上叠加查询条件
report_run({
  "workspace": "dajee",
  "name": "list",
  "query": "assignee:me priority:H",
  "limit": 20
})

// 逾期任务
report_run({"workspace": "dajee", "name": "overdue"})
```

返回 envelope 的 `data` 中包含 `tasks`、`count` 和 `report.name`。

## urgency_explain — 解释 urgency

只读。用于回答“为什么这个任务排在前面”。

```json
urgency_explain({
  "workspace": "dajee",
  "project_id": "proj-uuid-xxx",
  "id": "task-uuid-or-task-slug"
})
```

返回：

```json
{
  "data": {
    "uuid": "task-uuid",
    "urgency": 8.9,
    "factors": [
      {"name": "priority", "value": 6.0},
      {"name": "due", "value": 2.0}
    ]
  }
}
```

## 典型 Agent 工作流

**场景：用户问“今天最该先做什么？”**

```json
report_run({"workspace": "dajee", "name": "next", "limit": 10})
urgency_explain({"workspace": "dajee", "id": "排第一的 task_slug"})
```
