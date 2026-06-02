# taskg M9 Assignee Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 `taskg` 增加任务多 assignee 能力，并让 CLI、remote CLI、HTTP API、MCP、JSON export/import、Hook payload 在同一套 task 模型上读写 assignee。

**Architecture:** M9 不新增新的 task 入口或事件体系，而是在现有 `task.Task`、`task.JSONTask`、`internal/app.Service` 和 SQLite task repository 基础上补一层 `task_assignees` 关系，并让所有对外接口复用同一个 assignee 解析与序列化路径。写入路径统一先解析 assignee ref 到稳定 `user_id`，再通过 repository 持久化；读取路径统一在 repository hydrate 为 `[]AssigneeInfo`，再由 JSON/API/MCP/Hook 复用。

**Tech Stack:** Go 1.25、Cobra、GORM、`github.com/glebarez/sqlite`、现有 `internal/query` AST、`go test`、CLI integration tests、HTTP `httptest`、MCP integration/schema tests。

---

## 范围锁定

严格按 [M9 spec](/Users/mac/code/projects/dajee/task/docs/superpowers/specs/2026-06-02-taskg-m9-assignee-design.md) 实现。

必须进入 M9：

- `task_assignees` 关系表与 domain hydration。
- `@ref` / `+@ref` / `-@ref` 写语法。
- `assignee:<ref>` 与 `assignee:me` 查询语法。
- JSON export/import、HTTP API、remote CLI、MCP、Hook payload 中的 `assignees` 对象数组。
- `task info` 的 human 输出补充 assignee。
- README / manual / roadmap / requirements / OpenAPI 文档同步。
- 远程 / HTTP / MCP / 服务端模式下只允许 assign 当前 effective workspace 的成员。

明确不进入 M9：

- 主 assignee / secondary assignee。
- assignee 级权限与通知。
- 新的 `task.assigned` Hook event。
- `list` / `next` / report table 列系统改造。
- 跨 workspace 的全局 “我的任务” 视图。

## 文件结构

修改文件：

- `internal/task/model.go`
  增加 `AssigneeInfo` 与 `Task.Assignees`。
- `internal/task/modification.go`
  扩展 CLI / modify 语义使用的 assignee 字段。
- `internal/task/json.go`
  扩展 `JSONTask`、marshal/unmarshal、import 兼容逻辑。
- `internal/task/json_test.go`
  assignee JSON 序列化与兼容测试。
- `internal/task/modification_test.go`
  assignee modification 的空值判断测试。
- `internal/storage/sqlite/models.go`
  增加 `TaskAssignee` model 与 task 关联。
- `internal/storage/sqlite/db.go`
  迁移 `TaskAssignee`。
- `internal/storage/sqlite/db_test.go`
  验证新表已迁移。
- `internal/storage/sqlite/task_repo.go`
  持久化与 hydrate assignees。
- `internal/storage/sqlite/task_repo_test.go`
  repository 读写与去重测试。
- `internal/storage/sqlite/query_scope.go`
  `assignee` predicate 编译为 SQL。
- `internal/storage/sqlite/query_scope_test.go`
  `assignee` 查询编译和执行测试。
- `internal/query/expr.go`
  新增 `AttrAssignee`。
- `internal/query/parser.go`
  解析 `assignee:<ref>`。
- `internal/query/args.go`
  解析 `@ref` / `+@ref` / `-@ref`。
- `internal/query/parser_test.go`
  assignee filter parser 测试。
- `internal/query/args_test.go`
  assignee token parser 测试。
- `internal/app/service.go`
  Add / Modify / List / Import 的 assignee 解析与传递。
- `internal/app/service_test.go`
  assignee 解析、成员校验、`assignee:me`、import round-trip 测试。
- `internal/cli/add.go`
  本地 / remote add 透传 assignee。
- `internal/cli/root.go`
  target-style modify 透传 assignee。
- `internal/render/table.go`
  `TaskInfo()` 增加 assignee 行。
- `internal/remote/task.go`
  Add / Modify request struct 扩展 assignee 字段。
- `internal/httpapi/tasks.go`
  HTTP add/modify request struct 与 task list filter 透传。
- `internal/httpapi/tasks_test.go`
  assignee API 测试。
- `internal/mcpserver/tools_task.go`
  task tool input 与 modify clear 逻辑扩展。
- `internal/mcpserver/tools_common.go`
  `taskData()` / `tasksData()` 继续复用扩展后的 JSONTask。
- `internal/mcpserver/integration_test.go`
  assignee MCP 测试。
- `internal/mcpserver/schema_test.go`
  schema 校验。
- `internal/mcpserver/testdata/task.add.schema.json`
- `internal/mcpserver/testdata/task.modify.schema.json`
- `internal/mcpserver/testdata/list-tools-default.json`
  更新 MCP schema golden。
- `internal/app/hook_test.go`
  验证 Hook payload 中透出 assignees。
- `tests/integration/cli_test.go`
  CLI 黑盒测试。
- `docs/openapi/taskg-v1.yaml`
  task request/response schema 与 filter 说明。
- `docs/manual/tasks.md`
  任务语法与 `@ref` 示例。
- `docs/manual/remote-cli-and-api.md`
  HTTP / remote 字段说明。
- `docs/manual/mcp.md`
  MCP tool 参数与返回示例。
- `docs/requirements.md`
  标记单值 `assignee_user_id` 方案被取代。
- `README.md`
  user-facing 功能说明。
- `ROADMAP.md`
  M9 状态、范围与验收更新。

---

## Chunk 1: Domain、JSON 与 SQLite 关系模型

### Task 1: 先让 domain 与 JSONTask 能表达 assignees

**Files:**
- Modify: `internal/task/model.go`
- Modify: `internal/task/modification.go`
- Modify: `internal/task/json.go`
- Test: `internal/task/json_test.go`
- Test: `internal/task/modification_test.go`

- [ ] **Step 1: 写失败测试，锁定 JSON 与 modification 预期**

在 `internal/task/json_test.go` 增加至少这些测试：

```go
func TestJSONTaskExportsAssignees(t *testing.T) {}
func TestJSONTaskImportSupportsObjectAssignees(t *testing.T) {}
func TestJSONTaskImportSupportsStringAssignees(t *testing.T) {}
```

并在 `internal/task/modification_test.go` 增加一个 `Modification.Empty()` 相关断言，确保 assignee 字段会让 modify 不再被判定为空。

- [ ] **Step 2: 运行红测**

Run: `go test ./internal/task -run 'TestJSONTask|TestModification'`

Expected: FAIL，提示 `assignees` 字段或 assignee modification 还不存在。

- [ ] **Step 3: 增加 task domain 字段**

在 `internal/task/model.go` 增加：

```go
type AssigneeInfo struct {
	UserID string
	Name   string
	Email  *string
}
```

并把 `Assignees []AssigneeInfo` 加入 `task.Task`。

- [ ] **Step 4: 扩展 modification 结构**

在 `internal/task/modification.go` 增加：

```go
AddAssignees    []string
RemoveAssignees []string
ClearAssignees  bool
```

同步更新 `Empty()`。

- [ ] **Step 5: 扩展 JSONTask**

在 `internal/task/json.go`：

- 给 `JSONTask` 增加 `Assignees []JSONAssignee`
- `MarshalJSON()` 输出 `assignees`
- `UnmarshalJSON()` 支持对象数组与字符串数组两种输入
- `ToJSON()` / `FromJSONStrict()` 做 domain 映射

- [ ] **Step 6: 运行绿测**

Run: `go test ./internal/task/...`

Expected: PASS。

### Task 2: 增加 SQLite 关系表与 repository hydration

**Files:**
- Modify: `internal/storage/sqlite/models.go`
- Modify: `internal/storage/sqlite/db.go`
- Modify: `internal/storage/sqlite/task_repo.go`
- Test: `internal/storage/sqlite/db_test.go`
- Test: `internal/storage/sqlite/task_repo_test.go`

- [ ] **Step 1: 写失败测试，锁定迁移与 repo 行为**

增加至少这些测试：

```go
func TestTaskAssigneeTableMigrated(t *testing.T) {}
func TestTaskRepositoryCreateAndGetAssignees(t *testing.T) {}
func TestTaskRepositoryUpdateAssignees(t *testing.T) {}
```

- [ ] **Step 2: 运行红测**

Run: `go test ./internal/storage/sqlite -run 'TestTaskAssignee|TestTaskRepository.*Assignee'`

Expected: FAIL，提示 `task_assignees` 不存在或 domain 未 hydrate。

- [ ] **Step 3: 增加 model 与 migration**

在 `internal/storage/sqlite/models.go` 增加：

```go
type TaskAssignee struct {
	TaskUUID string `gorm:"primaryKey;not null"`
	UserID   string `gorm:"primaryKey;not null;index:idx_task_assignees_user_id"`
}
```

并把它挂到 `Task` model：

```go
Assignees []TaskAssignee `gorm:"foreignKey:TaskUUID;constraint:OnDelete:CASCADE"`
```

在 `internal/storage/sqlite/db.go` 的 AutoMigrate 列表里加入 `&TaskAssignee{}`。

- [ ] **Step 4: 写入与回填 assignees**

在 `internal/storage/sqlite/task_repo.go`：

- `preloadAssociations()` 加入 `Preload("Assignees")`
- `toModel()` 从 domain assignees 生成 `TaskAssignee`
- `fromModel()` 先回填 `user_id`，再批量查 `users` 构造 `[]AssigneeInfo`
- `Update()` 同 tags/depends 一样维护 `task_assignees`

不要在 task 主查询上做脆弱的手写多表 flatten；优先复用 GORM 关联 + 二次 hydration。

- [ ] **Step 5: 运行绿测**

Run: `go test ./internal/storage/sqlite/...`

Expected: PASS。

---

## Chunk 2: Query parser、SQL 编译与 app assignee 解析

### Task 3: 让 query 和 CLI token 能识别 assignee

**Files:**
- Modify: `internal/query/expr.go`
- Modify: `internal/query/parser.go`
- Modify: `internal/query/args.go`
- Test: `internal/query/parser_test.go`
- Test: `internal/query/args_test.go`

- [ ] **Step 1: 写失败测试**

增加至少这些测试：

```go
func TestParseQueryAssigneeAttribute(t *testing.T) {}
func TestParseAddArgsRecognizesAssignees(t *testing.T) {}
func TestParseModifyArgsRecognizesAssigneeMutations(t *testing.T) {}
```

- [ ] **Step 2: 运行红测**

Run: `go test ./internal/query -run 'TestParse.*Assignee'`

Expected: FAIL。

- [ ] **Step 3: 增加 attribute 与 parser 规则**

在 `internal/query/expr.go` 增加：

```go
AttrAssignee Attribute = "assignee"
```

在 `internal/query/parser.go` 支持 `assignee:<ref>`。

在 `internal/query/args.go` 支持：

- `@alice` -> `AddAssignees`
- `+@alice` -> `AddAssignees`
- `-@alice` -> `RemoveAssignees`

注意不要误伤邮箱形式 `@alice@example.com`。

- [ ] **Step 4: 运行绿测**

Run: `go test ./internal/query/...`

Expected: PASS。

### Task 4: 让 storage SQL 与 app service 解析 assignee ref

**Files:**
- Modify: `internal/storage/sqlite/query_scope.go`
- Test: `internal/storage/sqlite/query_scope_test.go`
- Modify: `internal/app/service.go`
- Test: `internal/app/service_test.go`

- [ ] **Step 1: 写失败测试**

增加至少这些测试：

```go
func TestCompileQueryAssigneePredicate(t *testing.T) {}
func TestServiceAddResolvesAssigneesInWorkspace(t *testing.T) {}
func TestServiceModifyRejectsUnknownAssignee(t *testing.T) {}
func TestServiceListExpandsAssigneeMe(t *testing.T) {}
```

- [ ] **Step 2: 运行红测**

Run: `go test ./internal/storage/sqlite ./internal/app -run 'Assignee|assignee'`

Expected: FAIL。

- [ ] **Step 3: 在 SQL 编译层支持 assignee**

在 `internal/storage/sqlite/query_scope.go` 为 `query.AttrAssignee` 编译 `EXISTS` 子查询，形如：

```go
EXISTS (
  SELECT 1
  FROM task_assignees
  WHERE task_assignees.task_uuid = tasks.uuid
    AND task_assignees.user_id = ?
)
```

- [ ] **Step 4: 在 app 层集中做 ref 解析**

在 `internal/app/service.go`：

- 给 `AddInput` / `ModifyInput` 增加 assignee 字段
- 抽一个私有 helper，统一做 `user_id -> email -> name` 解析
- 服务端模式下校验目标用户属于当前 workspace member，并固定错误语义：
  - 找不到用户返回 `RuntimeError{Code: "assignee_not_found"}`
  - 用户存在但不在当前 workspace 返回 `RuntimeError{Code: "assignee_not_member"}`
- `assignee:me` 在进入 repo 之前展开为 `runtime.ActorUserID`

解析逻辑必须只存在于 `internal/app`，不能散落在 CLI / HTTP / MCP。

- [ ] **Step 5: 运行绿测**

Run: `go test ./internal/storage/sqlite ./internal/app`

Expected: PASS。

---

## Chunk 3: CLI、remote、HTTP API、MCP 与 Hook payload

### Task 5: 打通 CLI、render 与 remote client

**Files:**
- Modify: `internal/cli/add.go`
- Modify: `internal/cli/root.go`
- Modify: `internal/render/table.go`
- Modify: `internal/remote/task.go`
- Test: `tests/integration/cli_test.go`

- [ ] **Step 1: 写失败测试**

在 `tests/integration/cli_test.go` 增加至少这些黑盒用例：

```go
func TestCLIAddWithAssignees(t *testing.T) {}
func TestCLIModifyAssignees(t *testing.T) {}
func TestCLIInfoShowsAssignees(t *testing.T) {}
func TestCLIListByAssignee(t *testing.T) {}
```

- [ ] **Step 2: 运行红测**

Run: `go test ./tests/integration -run 'Assignee'`

Expected: FAIL。

- [ ] **Step 3: 透传 add / modify assignee**

在 `internal/cli/add.go` 和 `internal/cli/root.go`：

- 本地模式把 parser 结果填入 `app.AddInput` / `app.ModifyInput`
- remote 模式把同一结果填入 `remote.AddTaskInput` / `remote.ModifyTaskInput`

在 `internal/render/table.go` 的 `TaskInfo()` 增加 `Assignees:` 行，渲染为 `@alice, @bob`。

- [ ] **Step 4: 运行绿测**

Run: `go test ./tests/integration -run 'Assignee'`

Expected: PASS。

### Task 6: 打通 HTTP API、MCP 与 Hook payload

**Files:**
- Modify: `internal/httpapi/tasks.go`
- Test: `internal/httpapi/tasks_test.go`
- Modify: `internal/mcpserver/tools_task.go`
- Modify: `internal/mcpserver/tools_common.go`
- Test: `internal/mcpserver/integration_test.go`
- Test: `internal/mcpserver/schema_test.go`
- Modify: `internal/mcpserver/testdata/task.add.schema.json`
- Modify: `internal/mcpserver/testdata/task.modify.schema.json`
- Modify: `internal/mcpserver/testdata/list-tools-default.json`
- Test: `internal/app/hook_test.go`

- [ ] **Step 1: 写失败测试**

增加至少这些测试：

```go
func TestHTTPTaskAddAssignees(t *testing.T) {}
func TestHTTPTaskModifyAssignees(t *testing.T) {}
func TestHTTPTaskListByAssignee(t *testing.T) {}
func TestMCPTaskAddAndGetAssignees(t *testing.T) {}
func TestHookPayloadIncludesAssignees(t *testing.T) {}
```

- [ ] **Step 2: 运行红测**

Run: `go test ./internal/httpapi ./internal/mcpserver ./internal/app -run 'Assignee|assignee'`

Expected: FAIL。

- [ ] **Step 3: 更新 HTTP request/response**

在 `internal/httpapi/tasks.go`：

- `addTaskRequest` 增加 `Assignees []string`
- `modifyTaskRequest` 增加 `Assignees []string`、`RemoveAssignees []string`、`ClearAssignees bool`
- list handler 仍复用现有 query parser，因此只需要保证 `assignee:<ref>` 能正常透传

- [ ] **Step 4: 更新 MCP tool**

在 `internal/mcpserver/tools_task.go`：

- `TaskAddInput` 增加 `Assignees []string`
- `TaskModifyInput` 增加 `Assignees []string`、`RemoveAssignees []string`
- `applyClearFields()` 允许 `"assignees"`

`tools_common.go` 不需要发明新 envelope；只需确保扩展后的 `task.ToJSON()` 自动透出 assignees。

- [ ] **Step 5: 更新 Hook payload 验证**

因为 Hook payload 已复用 `task.ToJSON()`，通常不需要再改 `internal/app/hook_event.go`，但必须补测试确认 `data.task.assignees` 实际存在。

- [ ] **Step 6: 运行绿测**

Run: `go test ./internal/httpapi ./internal/mcpserver ./internal/app`

Expected: PASS。

---

## Chunk 4: Import/Export、文档同步与全量验证

### Task 7: 收口 import/export 语义与文档

**Files:**
- Modify: `internal/app/service.go`
- Test: `internal/app/service_test.go`
- Modify: `docs/openapi/taskg-v1.yaml`
- Modify: `docs/manual/tasks.md`
- Modify: `docs/manual/remote-cli-and-api.md`
- Modify: `docs/manual/mcp.md`
- Modify: `docs/requirements.md`
- Modify: `README.md`
- Modify: `ROADMAP.md`

- [ ] **Step 1: 写失败测试，锁定 import/export 行为**

增加至少这些测试：

```go
func TestServiceImportAssigneesFromObjectArray(t *testing.T) {}
func TestServiceImportAssigneesFromStringArray(t *testing.T) {}
func TestServiceImportClearsAssigneesWithExplicitEmptyArray(t *testing.T) {}
func TestServiceExportIncludesAssignees(t *testing.T) {}
```

- [ ] **Step 2: 运行红测**

Run: `go test ./internal/app -run 'Import.*Assignee|Export.*Assignee'`

Expected: FAIL。

- [ ] **Step 3: 让 import/export 走统一 assignee 逻辑**

在 `internal/app/service.go`：

- import create/update 时处理 `tsk.Assignees`
- 缺省 `assignees` 字段时保持“不触碰 assignee”
- `assignees: []` 时清空 assignee

不要引入“warning 但继续提交”的半成功语义；保持当前 import 的原子失败模型。

- [ ] **Step 4: 更新对外文档**

同步更新：

- README 的 task 能力说明
- tasks / remote-cli-and-api / mcp manual
- OpenAPI task schema
- requirements 里旧的 `assignee_user_id` 说明
- roadmap 的 M9 范围与验收

- [ ] **Step 5: 运行全量验证**

Run: `go test ./...`
Expected: PASS

Run: `CGO_ENABLED=0 go test ./...`
Expected: PASS

Run: `CGO_ENABLED=0 go build ./cmd/taskg`
Expected: PASS

- [ ] **Step 6: 提交**

```bash
git add internal/task internal/storage/sqlite internal/query internal/app internal/cli internal/render internal/remote internal/httpapi internal/mcpserver tests/integration README.md ROADMAP.md docs
git commit -m "feat: 增加任务多 assignee 支持"
```

---

## 完成定义

只有在以下条件全部满足时，M9 才算完成：

- 本地 CLI、remote CLI、HTTP API、MCP、Hook payload 都能稳定读写 assignees。
- `assignee:me` 在当前 effective workspace 内正确工作。
- JSON export/import 保持兼容且原子。
- README / manual / roadmap / requirements / OpenAPI 全部同步。
- 三条标准验证命令全部通过：

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/taskg
```
