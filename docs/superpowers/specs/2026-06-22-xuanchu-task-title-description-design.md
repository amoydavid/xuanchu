# 任务 title / description 语义重构设计

日期：2026-06-22

> **给 agentic workers 的要求：** 编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

## 1. 背景

当前 Xuanchu 沿用 Taskwarrior 命名，把 `description` 当作任务主体文本。随着系统面向企业项目协作、Web Console、REST API 和 MCP Agent 调用，`description` 同时表达“列表标题”和“详细描述”会造成语义混乱：

- 用户和外部系统通常把 `title` 理解为任务标题，把 `description` 理解为详细说明。
- JSON 导入需要承载会议纪要、依赖、外部链接和扩展字段，只有 `description` 会让“标题”和“详情”难以区分。
- Web Console 列表和详情页也需要标题/详情分层展示。

项目尚未上线，不需要为旧生产数据保留迁移兼容。测试数据和本地开发库可以按新模型重建。

## 2. 目标

- 任务模型显式区分：
  - `title`：必填，任务标题。
  - `description`：选填，任务详细描述。
- CLI、REST API、Remote Client、MCP、JSON import/export、Hook/Notification payload、Web Console 都使用新语义。
- 导入 JSON 使用 `title` 作为必填字段；`description` 可为空或省略。
- `annotations` 保持时间线备注语义，不再承担任务详情职责。
- `links[].title` 保持外部链接标题语义，不受本次重构影响。
- 不做旧数据迁移兼容；数据库模型可以直接改列/字段语义。

## 3. 非目标

- 不保留“旧 `description` 等于标题”的长期兼容。
- 不把详细描述塞进 annotation。
- 不改变 workspace、project、task_slug、assignee、UDA、dependency、recurrence 的既有语义。
- 不重命名 project/workspace/annotation/link 的 `description` 字段；本次只改 task 主实体。
- 不实现富文本或 Markdown 渲染。`description` 先作为普通字符串保存和返回。

## 4. 数据模型

### 4.1 Domain

`task.Task` 改为：

```go
type Task struct {
    Title       string
    Description *string
    // 其它字段保持不变
}
```

校验规则：

- `Title` trim 后不能为空。
- `Description` 可为 `nil` 或空字符串；服务层输入应 trim，空字符串归一为 `nil`。

### 4.2 Storage

`tasks` 表使用：

```text
title TEXT NOT NULL
description TEXT NULL
```

不需要从旧 `description` 列迁移生产数据。迁移测试只需证明新表结构能创建并读写。

## 5. API / JSON 契约

### 5.1 Task JSON

标准任务 JSON：

```json
{
  "uuid": "task-uuid",
  "title": "输出接口设计",
  "description": "补充 REST/MCP 字段、错误码、权限边界和测试用例",
  "status": "pending",
  "entry": "2026-06-22T09:30:00Z",
  "modified": "2026-06-22T09:30:00Z"
}
```

`title` 必填；`description` 省略表示无详细描述。

### 5.2 REST API

- `POST /api/v1/tasks` 请求体使用 `title` 必填、`description` 可选。
- `PATCH /api/v1/tasks/{taskRef}` 支持修改 `title`、`description`，并支持清空 `description`。
- `GET /api/v1/tasks*` 输出 `title`，仅在有详情时输出 `description`。
- `POST /api/v1/import` 只接受新任务 JSON 契约。

### 5.3 MCP

- `task_add` 使用 `title` 必填、`description` 可选。
- `task_modify` 支持 `title`、`description` 和 `clear` 中的 `description`。
- `task_query`、`task_get`、`task_export` structured content 输出 `title` 和可选 `description`。

### 5.4 CLI

- `xuanchu add "标题"` 写入 `title`。
- `modify` 中原本修改任务主体的参数改为修改 `title`。
- `append` / `prepend` 语义改为 `append title` / `prepend title`，命名可在后续单独优化。
- 详细描述优先通过 REST/MCP/JSON 导入设置；CLI 可以接受 `description:<text>` modifier 写入详情。

## 6. 查询和渲染

- `title:<text>` 和裸 `/text/` 应匹配任务标题。
- `description:<text>` 匹配详细描述。
- 列表、人类可读表格、通知摘要默认展示 `title`。
- 详情页展示标题和详情两个区域。

## 7. 验收标准

- `go test ./...` 通过。
- `CGO_ENABLED=0 go test ./...` 通过。
- `CGO_ENABLED=0 go build ./cmd/xuanchu` 通过。
- 如修改 Web Console 类型或组件，运行对应前端 typecheck/test。
- 代码中 task 主实体不再把 `Description` 当标题使用；任务标题字段统一为 `Title`。
