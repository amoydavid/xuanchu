# Xuanchu Project 生命周期状态机 Implementation Plan

> **For agentic workers:** REQUIRED: 使用 superpowers:subagent-driven-development（若可用）或 superpowers:executing-plans 执行本计划。步骤用 checkbox（`- [ ]`）跟踪。

**Goal:** 为 Project 增加 planning / active / archived / cancelled 四态状态机，提供显式转移操作、写权限收敛、webhook 与审计。

**Architecture:** 不改 schema、无数据迁移。在 `storage` 层新增状态常量与 List 过滤；在 `app` 层新增 `TransitionProject`、抽取 `isProjectClosed` 收敛散落的归档判断、`AddProject` 默认改为 planning；在 `cli` / `httpapi` / `mcpserver` 三端暴露 transition 入口；新增 `project.transitioned` hook 事件与审计动作。

**Tech Stack:** Go 1.25、GORM、github.com/glebarez/sqlite、Cobra、chi、MCP SDK

**Spec:** `docs/superpowers/specs/2026-06-15-xuanchu-project-lifecycle-design.md`

---

## Chunk 1：Storage 层

### Task 1：新增状态常量 + List 过滤语义

**Files:**
- Modify: `internal/storage/project_repo.go`

- [ ] **Step 1：** 新增常量 `ProjectStatusPlanning = "planning"`、`ProjectStatusCancelled = "cancelled"`
- [ ] **Step 2：** 新增 `isProjectClosed(status string) bool`（status ∈ {archived, cancelled}）与 `isProjectActiveLike(status string) bool`（status ∈ {planning, active}）helper
- [ ] **Step 3：** `List` 增加可选 status 过滤参数，支持 `open`（planning+active）/ `planning` / `active` / `archived` / `cancelled` / `all`；默认行为改为 `open`
- [ ] **Step 4：** 新增 `UpdateStatus(workspaceID, projectID, status string, now int64) error` 方法（仅更新 status 与 modified_at）
- [ ] **Step 5：** 验证 `CGO_ENABLED=0 go build ./internal/storage/...`

### Task 2：storage 层测试

**Files:**
- Modify: `internal/storage/project_repo_test.go`

- [ ] **Step 1：** 覆盖 `List` 各 status 过滤组合（含 open=planning+active、all）
- [ ] **Step 2：** 覆盖 `UpdateStatus` 成功与 project 不存在
- [ ] **Step 3：** 验证 `CGO_ENABLED=0 go test ./internal/storage/... -run TestProject`

---

## Chunk 2：App 层

### Task 3：TransitionProject + 默认态 + isProjectClosed 收敛

**Files:**
- Modify: `internal/app/project.go`
- Modify: `internal/app/project_query.go`
- Modify: `internal/app/project_config.go`
- Modify: `internal/app/service.go`

- [ ] **Step 1：** 在 `app` 层新增 `isProjectClosed(project) bool` 包装（调用 storage helper），替换 `project.go:149,177,429,490`、`project_config.go:187`、`project_query.go:47`、`service.go:1957` 共 7 处 `status == archived || ArchivedAt != nil` 判断
- [ ] **Step 2：** `AddProject`（`project.go:228 addProjectLocked`）默认 status 改为 `ProjectStatusPlanning`
- [ ] **Step 3：** 新增 `TransitionProject(ref, toStatus string) (ProjectView, error)`：
  - 校验 `toStatus` 是合法四态之一，否则 `project_invalid_status`
  - 解析 project；**不校验转移方向**（任意状态可转任意状态）
  - 在 `withAuditAndEvents` 内调用 `projectRepo.UpdateStatus`
  - 转入 archived 时设置 `ArchivedAt`；从 archived 转出到非 archived 时清空 `ArchivedAt`
  - **自动追加一条项目变更注解**（复用 `projectAnnotateLocked` 的写入路径，但绕过 isProjectClosed 检查，因为 transition 在任何状态都允许），content 形如「状态变更：planning → active」
  - 权限：`PermissionProjectManage`
- [ ] **Step 4：** `recurringArchivedProjectWarning`（`service.go:1949`）的判断扩展为 `isProjectClosed`，使 cancelled project 上的 recurring 同样告警
- [ ] **Step 5：** `ArchiveProject`（`project.go:169`）保留，内部可改为调用 `TransitionProject(ref, archived)` 复用逻辑（保持公开 API 与 hook `project.archived` 向后兼容）
- [ ] **Step 6：** 验证 `CGO_ENABLED=0 go build ./...`

### Task 4：hook 事件 + 审计

**Files:**
- Modify: `internal/app/hook_event.go`
- Modify: `internal/app/hook.go`

- [ ] **Step 1：** 新增 `buildProjectTransitionedHookEvent(view, fromStatus, toStatus, runtime, now)`，事件类型 `project.transitioned`，payload 含 `from_status` / `to_status`
- [ ] **Step 2：** `hook.go` 注册 `project.transitioned` 为合法事件类型
- [ ] **Step 3：** `TransitionProject` 内：转移到 archived 时同时发射 `project.archived`（向后兼容）与 `project.transitioned`；其它转移只发射 `project.transitioned`
- [ ] **Step 4：** 审计动作 `project.transition`，payload 含 `from_status` / `to_status`
- [ ] **Step 5：** 验证 `CGO_ENABLED=0 go test ./internal/app/...`

### Task 5：App 层测试

**Files:**
- Modify: `internal/app/service_test.go`

- [ ] **Step 1：** 覆盖 `TestProjectTransition`：任意方向转移，含 planning→active、active→archived、**archived→active（重新激活）**、archived→cancelled、cancelled→active
- [ ] **Step 2：** 覆盖非法 `toStatus`（未知值）返回 `project_invalid_status`
- [ ] **Step 3：** 覆盖 transition 自动写入项目变更注解，可通过 `ProjectAnnotations` / `ProjectTimeline` 查询到
- [ ] **Step 4：** 覆盖 planning 状态可 add task / annotate / config（视同 active）
- [ ] **Step 5：** 覆盖 cancelled 状态写操作被拒（isProjectClosed）
- [ ] **Step 6：** 覆盖 `AddProject` 默认创建为 planning
- [ ] **Step 7：** 覆盖 transition 触发 `project.transitioned` hook 事件与审计
- [ ] **Step 8：** 验证 `CGO_ENABLED=0 go test ./internal/app/... -run TestProject`

---

## Chunk 3：CLI / HTTP / MCP

### Task 6：CLI transition 子命令 + list --status

**Files:**
- Modify: `internal/cli/project.go`

- [ ] **Step 1：** 新增 `project transition <ref> <planning|active|archived|cancelled>` 子命令（本地 + remote 双模式）
- [ ] **Step 2：** `project list` 增加 `--status <open|planning|active|archived|cancelled|all>` flag，默认 `open`
- [ ] **Step 3：** 验证 `CGO_ENABLED=0 go build ./cmd/xuanchu`

### Task 7：HTTP transition 端点

**Files:**
- Modify: `internal/httpapi/projects.go`
- Modify: `internal/httpapi/router.go`

- [ ] **Step 1：** 新增 `POST /api/v1/projects/{projectRef}/transition`，body `{"status":"..."}`，返回更新后的 project view
- [ ] **Step 2：** `GET /api/v1/projects` 支持 `status` 查询参数（默认 open）
- [ ] **Step 3：** 补 HTTP 测试覆盖 transition 合法/非法与 status 过滤
- [ ] **Step 4：** 验证 `CGO_ENABLED=0 go test ./internal/httpapi/...`

### Task 8：MCP project_transition tool

**Files:**
- Modify: `internal/mcpserver/tools_project.go`
- Modify: `internal/mcpserver/testdata/list-tools-default.json`
- Modify: `internal/mcpserver/integration_test.go`（tool count）

- [ ] **Step 1：** 新增 `project_transition` tool（input: project/project_id/workspace/status）
- [ ] **Step 2：** 更新 golden file 与 tool count 断言
- [ ] **Step 3：** 验证 `CGO_ENABLED=0 go test ./internal/mcpserver/...`

---

## Chunk 4：集成测试 + 收尾

### Task 9：CLI 集成测试

**Files:**
- Modify: `tests/integration/cli_test.go`

- [ ] **Step 1：** `TestCLIProjectTransition`：创建（默认 planning）→ transition active → transition archived → transition cancelled → 复活 active 全流程
- [ ] **Step 2：** `TestCLIProjectTransitionInvalid`：非法转移报错
- [ ] **Step 3：** `TestCLIProjectListStatus`：`--status open` / `cancelled` / `all` 过滤
- [ ] **Step 4：** `TestCLIProjectClosedRejectsWrite`：cancelled project 不能 annotate / config set
- [ ] **Step 5：** 验证 `CGO_ENABLED=0 go test ./tests/integration/... -run TestCLIProject`

### Task 10：全量验证 + 文档同步

- [ ] **Step 1：** 全量验证
  ```bash
  go test ./...
  CGO_ENABLED=0 go test ./...
  CGO_ENABLED=0 go build ./cmd/xuanchu
  ```
- [ ] **Step 2：** 更新 `README.md`（project 状态说明：四态 + transition 命令）与 `docs/manual/team-workspaces-projects.md`（产品属性使用建议另述）
- [ ] **Step 3：** 更新 `ROADMAP.md`
- [ ] **Step 4：** Commit `docs: 更新项目生命周期状态机文档`
