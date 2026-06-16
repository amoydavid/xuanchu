# 可订阅事件类型

notification rule 和 hook 共用以下事件清单。

## task.* 事件

| 事件 | 含义 |
|---|---|
| `task.created` | 任务创建 |
| `task.modified` | 任务修改（通用） |
| `task.completed` | 任务完成 |
| `task.deleted` | 任务删除 |
| `task.started` | 任务开始 |
| `task.stopped` | 任务停止 |
| `task.assigned` | 任务新增负责人 |
| `task.unassigned` | 任务移除负责人 |
| `task.blocked` | 任务进入阻塞状态 |
| `task.unblocked` | 任务解除阻塞 |
| `task.due_changed` | 任务 due 字段变更 |
| `task.priority_changed` | 任务 priority 字段变更 |
| `task.project_changed` | 任务 project 字段变更 |
| `task.tags_changed` | 任务 tags 变更 |

## project.* 事件

| 事件 | 含义 |
|---|---|
| `project.archived` | 项目归档 |
| `project.annotated` | 项目新增注释 |
| `project.denotated` | 项目删除注释 |

## 当前不允许注册的候选事件（参考，不可用）

以下事件尚未开放注册，创建 rule/hook 时不要使用：

- `task.annotated` / `task.denotated`
- `task.link_added` / `task.link_removed`
- `project.created` / `project.updated`
- `workspace.member_added` / `workspace.member_removed` / `workspace.member_role_changed`

## 订阅建议

- **开始/停止**：`start` 只触发 `task.started`，`stop` 只触发 `task.stopped`。开始/停止通知应分别创建对应规则，不要只靠 `task.modified`。
- **普通编辑 vs 字段级**：普通编辑用 `task.modified`；字段级通知优先用 `task.due_changed`、`task.priority_changed`、`task.project_changed`、`task.tags_changed` 等细粒度事件。
- **阻塞状态**：`task.blocked` 表示从非 blocked 进入 blocked；`task.unblocked` 表示解除 blocked。
- **audience 限制**：`assignees` 和 `assignees_and_explicit_users` 只支持 `task.*` 事件；project 事件没有 assignee 语义，应使用 `actor` 或 `explicit_users`。
