# Task Title / Description Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 Xuanchu 任务主模型重构为 `title` 必填、`description` 选填，并贯通 CLI、REST、MCP、JSON import/export、storage、Web Console 与文档。

**Architecture:** 以 `internal/task.Task` 为语义源头新增 `Title` 与可空 `Description`；storage 表直接使用 `title` 和 nullable `description`，不做旧数据迁移兼容；各入口的旧“任务主体 description”统一改为 `title`，详细描述作为独立可选字段传递。

**Tech Stack:** Go 1.25, Cobra, GORM, `github.com/glebarez/sqlite`, `gorm.io/driver/postgres`, MCP Go SDK, React/TypeScript Web Console

**Spec:** `docs/superpowers/specs/2026-06-22-xuanchu-task-title-description-design.md`

**Current status:** 已实现并通过验收。2026-06-22 复验：

- `go test ./tests/integration`
- 手工端到端烟测：CLI 创建 title+description、title-only 创建、JSON title-only 导入、description-only 导入拒绝、`_get title/description`、title/description 查询
- `go test ./...`
- `CGO_ENABLED=0 go test ./...`
- `CGO_ENABLED=0 go build ./cmd/xuanchu`
- `pnpm --dir web build`
- `pnpm --dir web typecheck`
- `pnpm --dir web test`
- `git diff --check`

下面 checklist 保留原实施分解；其中“先确认失败”的 TDD 步骤是开发过程要求，不作为当前状态复验清单。

---

## Chunk 1: Core Model / Storage / JSON

### Task 1: 写失败测试定义新 JSON 契约

**Files:**
- Modify: `internal/task/json_test.go`
- Modify: `internal/task/json.go`
- Modify: `internal/task/model.go`

- [ ] 新增测试：`ToJSON` 输出 `title`，有详情时输出 `description`，无详情时省略 `description`。
- [ ] 新增测试：`FromJSONStrict` 要求 `title` 非空，允许 `description` 省略。
- [ ] 运行 `go test ./internal/task -run 'Title|Description|JSON' -v`，确认失败。
- [ ] 实现 `Task.Title`、`Task.Description *string`、`JSONTask.Title`、`JSONTask.Description *string`。
- [ ] 运行同一测试确认通过。

### Task 2: 重构 storage 模型和 round-trip

**Files:**
- Modify: `internal/storage/models.go`
- Modify: `internal/storage/task_repo.go`
- Modify: `internal/storage/task_repo_test.go`
- Modify: `internal/storage/db_test.go`

- [ ] 新增 repository 测试：任务 title 必填、description 可空、round-trip 保留详情。
- [ ] 运行 `go test ./internal/storage -run 'TaskRepository.*Title|Task.*Description|DB' -v`，确认失败。
- [ ] 将 storage `Task` 的任务主体列改为 `Title string` 与 `Description *string`。
- [ ] 更新 model/domain 映射和 AutoMigrate 相关测试。
- [ ] 运行 storage 测试确认通过。

## Chunk 2: App / Query / Render

### Task 3: 重构 App task service

**Files:**
- Modify: `internal/app/service.go`
- Modify: `internal/app/service_test.go`
- Modify: `internal/app/task_change_events.go`
- Modify: `internal/app/event_notification.go`

- [ ] 新增/更新 app 测试：`AddInput{Title}` 创建任务，`ModifyInput{Title}` 修改标题，`Description` 可设置和清空。
- [ ] 运行 `go test ./internal/app -run 'Title|Description|Task' -v`，确认失败。
- [ ] 将 app task 输入从标题语义的 `Description` 改为 `Title`，新增可选 `Description`。
- [ ] 更新 audit、hook、notification payload 使用 title。
- [ ] 运行 app 测试确认通过。

### Task 4: 更新查询、渲染和 helper 字段

**Files:**
- Modify: `internal/query`
- Modify: `internal/storage/query_scope.go`
- Modify: `internal/render`
- Modify: `internal/cli/helper.go`
- Modify: tests covering `_get`, list, filters

- [ ] 新增测试：`title:` 匹配标题，`description:` 匹配详情，裸 `/text/` 匹配标题。
- [ ] 运行相关测试确认失败。
- [ ] 实现 query attr 和渲染字段更新。
- [ ] 运行相关测试确认通过。

## Chunk 3: Protocol Surfaces

### Task 5: 更新 REST 和 remote client

**Files:**
- Modify: `internal/httpapi/tasks.go`
- Modify: `internal/httpapi/import_audit.go`
- Modify: `internal/httpapi/tasks_test.go`
- Modify: `internal/remote/task.go`
- Modify: `internal/remote/*_test.go`
- Modify: `docs/openapi/xuanchu-v1.yaml`

- [ ] 新增 REST 测试：create/import 使用 `title` 必填、`description` 可选。
- [ ] 运行 HTTP 测试确认失败。
- [ ] 更新 handler request/response DTO 和 remote client DTO。
- [ ] 运行 HTTP/remote 测试确认通过。

### Task 6: 更新 MCP tools 和 schema golden

**Files:**
- Modify: `internal/mcpserver/tools_task.go`
- Modify: `internal/mcpserver/tools_views.go`
- Modify: `internal/mcpserver/integration_test.go`
- Modify: `internal/mcpserver/testdata/*.schema.json`
- Modify: `docs/manual/mcp.md`

- [ ] 新增/更新 MCP 集成测试：`task_add` 使用 `title`，`task_import` 使用 `title`。
- [ ] 运行 MCP 测试确认失败。
- [ ] 更新 tool input、structured content 和 golden schema。
- [ ] 运行 MCP 测试确认通过。

## Chunk 4: CLI / Web / Docs / Verification

### Task 7: 更新 CLI 和集成测试

**Files:**
- Modify: `internal/cli`
- Modify: `tests/integration`
- Modify: `README.md`
- Modify: `docs/manual/*.md`

- [ ] 更新 CLI 测试：`add "标题"` 写入 title，`description:<text>` 写入详情。
- [ ] 运行 CLI/integration 相关测试确认失败。
- [ ] 更新 Cobra 解析、输出文案和文档。
- [ ] 运行相关测试确认通过。

### Task 8: 更新 Web Console

**Files:**
- Modify: `web/src/features/workspace/project-readonly/*`
- Modify: `web/src/locales/*`

- [ ] 更新前端类型和组件测试：列表显示 `title`，详情页显示 title + description。
- [ ] 运行前端测试/typecheck 确认失败。
- [ ] 更新组件、UDA 标准字段列表和本地化文案。
- [ ] 运行前端验证确认通过。

### Task 9: 全量验证

- [ ] Run: `go test ./...`
- [ ] Run: `CGO_ENABLED=0 go test ./...`
- [ ] Run: `CGO_ENABLED=0 go build ./cmd/xuanchu`
- [ ] Run: `git diff --check`
- [ ] 如果 Web Console 变更，运行 `pnpm --dir web typecheck` 或仓库已有等价命令。
