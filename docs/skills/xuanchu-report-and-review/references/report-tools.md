# Report 工具

运行内置报表、解释 urgency。全部只读。

## report_run — 运行内置报表（只读）

`name` 必填。可用 `query` 在报表上叠加过滤条件，`limit` 控制数量。

### 内置报表名

| 报表名 | 含义 |
|---|---|
| `list` | 列表 |
| `next` | 下一批该做的（按 urgency 排序） |
| `all` | 全部 |
| `completed` | 已完成 |
| `deleted` | 已删除 |
| `waiting` | 等待中 |
| `active` | 进行中（已 start） |
| `ready` | 就绪可做 |
| `overdue` | 逾期 |
| `blocked` | 被阻塞 |
| `blocking` | 阻塞他人 |

### 示例

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

## urgency_explain — 解释 urgency（只读）

用于回答"为什么这个任务排在前面"。

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

`id` 用 UUID、已物化 `task_slug` 或 occurrence_ref，不用本地 working-set 数字 ID。

## 典型工作流

### 用户问"今天最该先做什么？"

```json
report_run({"workspace": "dajee", "name": "next", "limit": 10})
urgency_explain({"workspace": "dajee", "id": "排第一的 task_slug"})
```
