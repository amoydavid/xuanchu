# M12：任务外部关联

## 概述

本 milestone 为 Xuanchu 增加任务外部资源轻关联能力：任务可以关联多个外部资源（文档目录、PR、设计稿、会议纪要等），以轻量方式记录"这个任务和外部世界的什么东西有关"。

## 动机

任务执行过程中经常涉及外部资源：

- 任务关联飞书文档目录（相关资料放在这里）
- 任务关联 GitHub PR（代码在这里）
- 任务关联 Jira ticket / 飞书妙记 / Figma 设计稿
- 任务关联会议纪要

当前 task 模型没有"链接"或"附件"字段。UDAs 理论上可以存，但没有 schema 约束，不在 MCP 工具和 Hook payload 中体现，Agent 无法结构化地读写。

## 设计

### 一、数据模型

新增 `task_links` 表：

```go
type TaskLink struct {
    ID        string `gorm:"primaryKey"`
    TaskUUID  string `gorm:"not null;uniqueIndex:idx_task_links_task_url,priority:1;index:idx_task_links_task"`
    Type      string `gorm:"not null"`
    URL       string `gorm:"not null;uniqueIndex:idx_task_links_task_url,priority:2"`
    Title     string `gorm:"not null;default:''"`
    CreatedAt int64  `gorm:"not null"`
    CreatedBy string `gorm:"not null"`
}
```

设计决策：

- `Type` 是自由文本，不做枚举限制。Agent 可以自定义类型（`document`、`pr`、`ticket`、`design`、`meeting_minutes`、`directory`、`feishu_doc` 等）。
- `Title` 可选，用于展示（如"PR #123: fix login bug"）。
- `(TaskUUID, URL)` 联合唯一——同一任务不重复挂同一个 URL。
- 不追踪链接状态（轻关联）。
- 不做双向同步。
- `CreatedBy` 记录添加者的 user ID。

### 二、Domain 层

在 Task domain model 中增加：

```go
type TaskLinkInfo struct {
    ID        string
    Type      string
    URL       string
    Title     string
    CreatedAt int64
    CreatedBy string
}

type Task struct {
    // ... 现有字段 ...
    Links []TaskLinkInfo
}
```

JSON DTO 增加：

```go
type JSONTaskLink struct {
    ID        string `json:"id"`
    Type      string `json:"type"`
    URL       string `json:"url"`
    Title     string `json:"title,omitempty"`
    CreatedAt string `json:"created_at"`
    CreatedBy string `json:"created_by"`
}
```

`JSONTask` 增加 `Links []JSONTaskLink` 字段。

### 三、App 层

- `TaskAddLink(taskRef, linkType, url, title string) (TaskLinkInfo, error)` — 添加关联，需要 `task:write`
- `TaskRemoveLink(taskRef, linkID string) error` — 删除关联，需要 `task:write`

写入时写 audit log（`task.link.add`、`task.link.remove`）。

`task.get` 和 `task.query` 的结果中包含 links。查询时批量加载（与 assignees 外部 ID 的加载方式类似）。

添加/删除关联触发 `task.modified` hook 事件（复用现有事件类型）。

### 四、CLI

```
xuanchu <task> link add --type <type> --url <url> [--title <title>]
xuanchu <task> link list
xuanchu <task> link remove <link-id>
```

`task info` 输出中展示 links 列表。`--json` 输出中包含 links。

### 五、HTTP API

- `POST /api/v1/tasks/:ref/links` — 添加关联（body: `{type, url, title?}`）
- `GET /api/v1/tasks/:ref/links` — 列出关联
- `DELETE /api/v1/tasks/:ref/links/:link-id` — 删除关联

### 六、MCP

- `task.link_add` — 添加外部关联（参数：`task`、`type`、`url`、`title?`）
- `task.link_remove` — 删除外部关联（参数：`task`、`link_id`）

`task.get` 和 `task.query` 返回中包含 links 数组。

### 七、Hook payload

`task.modified` 的 hook payload 中 `data.task.links` 包含任务的所有外部关联。

### 八、Render

- `task info` 输出中展示 links 列表（type + title/url）
- `task list` 的 `--json` 输出中包含 links
- Table 渲染中不展示 links（避免列过宽），只在 `info` 详细视图中展示

### 九、迁移与兼容

GORM AutoMigrate 自动处理：新增 `task_links` 表。无破坏性变更。

- 现有 task JSON 格式新增 `links` 字段（客户端忽略未知字段即可）
- 现有 hook payload 新增 `links` 字段（消费者忽略即可）

## 验收标准

1. 任务能添加/删除/查看外部关联（type + URL + title）
2. `task.get` 和 `task.query` 返回中包含 links
3. CLI 能添加/删除/列出任务外部关联
4. HTTP API 能操作任务外部关联
5. MCP 能操作任务外部关联
6. Hook payload 中包含 links
7. `task info` 渲染中展示 links
8. 所有现有测试继续通过
9. `CGO_ENABLED=0 go build ./cmd/xuanchu` 和 `CGO_ENABLED=0 go test ./...` 通过

## 不做什么

- 不做链接状态追踪（如 PR 是否 merged）
- 不做双向同步（只记录，不主动推送到外部系统）
- 不做链接类型的枚举限制（自由文本）
- 不新增 hook 事件类型（复用 `task.modified`）
