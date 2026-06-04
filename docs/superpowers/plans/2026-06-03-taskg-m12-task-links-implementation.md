# M12 任务外部关联 实施计划

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 taskg 任务增加外部资源轻关联能力，让 Agent 能通过 MCP 结构化地记录任务与外部资源（文档、PR、设计稿等）的关联。

**Architecture:** 新增 `task_links` 表存储任务与外部资源的关联（type + URL + title）。遵循现有 TaskTag/TaskAnnotation 子表模式。App 层提供 `TaskAddLink` / `TaskRemoveLink` 方法。CLI、HTTP API、MCP 三端复用同一 app service。Hook payload 中包含 links。

**Tech Stack:** Go 1.25, GORM + github.com/glebarez/sqlite, github.com/google/uuid, github.com/spf13/cobra, github.com/modelcontextprotocol/go-sdk

**Spec:** `docs/superpowers/specs/2026-06-03-taskg-m12-project-context-task-links-design.md`

---

## 文件结构

### 新建文件

| 文件 | 职责 |
|---|---|
| `internal/storage/task_link_repo.go` | TaskLink 的 CRUD 操作 |

### 修改文件

| 文件 | 变更内容 |
|---|---|
| `internal/storage/models.go` | 新增 `TaskLink` struct |
| `internal/storage/db.go` | AutoMigrate 加入 `TaskLink` |
| `internal/storage/task_repo.go` | `fromModel` 扩展 Links 字段，新增 `loadLinksByTask` 批量加载 |
| `internal/task/model.go` | 新增 `TaskLinkInfo` struct，`Task` 增加 `Links` 字段 |
| `internal/task/json.go` | 新增 `JSONTaskLink`，`JSONTask` 增加 `Links`，export/import 扩展 |
| `internal/app/workspace.go` | 新增 `TaskAddLink` / `TaskRemoveLink` 方法 |
| `internal/app/service.go` | Service struct 增加 `taskLinkRepo` 字段 |
| `internal/cli/link.go` | 新增 `link add` / `link list` / `link remove` 子命令 |
| `internal/cli/root.go` | 注册 link action 到 `handleTargetAction` |
| `internal/httpapi/tasks.go` | 新增 link CRUD handler |
| `internal/httpapi/router.go` | 注册 link 路由 |
| `internal/remote/task.go` | 新增 link CRUD 方法 |
| `internal/mcpserver/tools_task.go` | 新增 `task.link_add` / `task.link_remove` tool |
| `internal/render/table.go` | `TaskInfo` 展示 links 列表 |

### 测试文件

| 文件 | 测试内容 |
|---|---|
| `internal/storage/db_test.go` | `TaskLink` 表 migration 验证 |
| `internal/storage/task_link_repo_test.go` | TaskLink CRUD 单元测试 |
| `internal/storage/task_repo_test.go` | task 附带 links 的 hydration 测试 |
| `internal/task/json_test.go` | JSON task links export/import 测试 |
| `internal/app/service_test.go` | `TaskAddLink` / `TaskRemoveLink` 测试 |
| `internal/httpapi/tasks_test.go` | link CRUD endpoint 测试 |
| `internal/mcpserver/integration_test.go` | `task.link_add` / `task.link_remove` tool 测试 |
| `internal/mcpserver/schema_test.go` | golden file 更新 |
| `tests/integration/cli_test.go` | CLI `link add/remove/list` 端到端测试 |

---

## Chunk 1: 数据层 — model、migration、repo

### Task 1: 新增 TaskLink model 和 migration

**Files:**
- Modify: `internal/storage/models.go` (末尾追加)
- Modify: `internal/storage/db.go` (AutoMigrate 行)
- Test: `internal/storage/db_test.go`

- [ ] **Step 1: 在 models.go 末尾新增 TaskLink struct**

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

- [ ] **Step 2: 在 db.go AutoMigrate 注册新表**

在 `internal/storage/db.go` 的 AutoMigrate 调用中加入 `&TaskLink{}`。

- [ ] **Step 3: 写 db_test.go 验证表已迁移**

```go
func TestTaskLinkTableMigrated(t *testing.T) {
	_, db := setupTestDB(t)
	if !db.Migrator().HasTable(&TaskLink{}) {
		t.Fatal("expected task_links table to exist")
	}
}
```

- [ ] **Step 4: 运行测试验证**

Run: `CGO_ENABLED=0 go test ./internal/storage/ -run TestTaskLinkTableMigrated -v`
Expected: PASS

- [ ] **Step 5: 运行全量 storage 测试确认无破坏**

Run: `CGO_ENABLED=0 go test ./internal/storage/ -v`
Expected: 全部 PASS

- [ ] **Step 6: 提交**

```bash
git add internal/storage/models.go internal/storage/db.go internal/storage/db_test.go
git commit -m "feat(m12): 新增 TaskLink model 和 migration"
```

---

### Task 2: 新增 TaskLinkRepository

**Files:**
- Create: `internal/storage/task_link_repo.go`
- Test: `internal/storage/task_link_repo_test.go`

- [ ] **Step 1: 写测试**

创建 `internal/storage/task_link_repo_test.go`，测试以下场景：

1. Create 成功并返回完整记录
2. Create 重复 (TaskUUID, URL) 失败
3. ListByTaskUUID 返回指定任务的 links（按 CreatedAt 排序）
4. GetByID 正常返回
5. GetByID 不存在返回 ErrNotFound
6. Delete 成功
7. Delete 不存在返回 ErrNotFound
8. loadLinksByTaskUUIDs 批量加载多个任务的 links

- [ ] **Step 2: 实现 TaskLinkRepository**

创建 `internal/storage/task_link_repo.go`：

```go
type TaskLinkRepository struct {
    db *gorm.DB
}

func NewTaskLinkRepository(db *gorm.DB) *TaskLinkRepository {
    return &TaskLinkRepository{db: db}
}

func (r *TaskLinkRepository) Create(link TaskLink) (TaskLink, error)
func (r *TaskLinkRepository) GetByID(id string) (TaskLink, error)
func (r *TaskLinkRepository) ListByTaskUUID(taskUUID string) ([]TaskLink, error)
func (r *TaskLinkRepository) Delete(id string) error
func (r *TaskLinkRepository) LoadByTaskUUIDs(taskUUIDs []string) (map[string][]TaskLink, error)
```

- [ ] **Step 3: 运行测试**

Run: `CGO_ENABLED=0 go test ./internal/storage/ -run TestTaskLinkRepo -v`
Expected: 全部 PASS

- [ ] **Step 4: 提交**

```bash
git add internal/storage/task_link_repo.go internal/storage/task_link_repo_test.go
git commit -m "feat(m12): 新增 TaskLinkRepository CRUD"
```

---

## Chunk 2: Domain + App 层

### Task 3: Domain 模型和 JSON DTO 扩展

**Files:**
- Modify: `internal/task/model.go`
- Modify: `internal/task/json.go`
- Test: `internal/task/json_test.go`

- [ ] **Step 1: 在 model.go 新增 TaskLinkInfo struct，Task 增加 Links 字段**

```go
type TaskLinkInfo struct {
    ID        string
    Type      string
    URL       string
    Title     string
    CreatedAt int64
    CreatedBy string
}
```

在 `Task` struct 中增加 `Links []TaskLinkInfo` 字段。

- [ ] **Step 2: 在 json.go 新增 JSONTaskLink，JSONTask 增加 Links 字段**

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

- [ ] **Step 3: 扩展 export/import**

export: `taskToJSON` 中将 `TaskLinkInfo` 转为 `JSONTaskLink`。
import: `jsonToTask` 中将 `JSONTaskLink` 转为 `TaskLinkInfo`。
MarshalJSON: 确保 Links 字段出现在 JSON 输出中。

- [ ] **Step 4: 写 json_test.go 测试**

测试 export 和 import 中 Links 字段的往返转换。

Run: `CGO_ENABLED=0 go test ./internal/task/ -v`
Expected: 全部 PASS

- [ ] **Step 5: 提交**

```bash
git add internal/task/model.go internal/task/json.go internal/task/json_test.go
git commit -m "feat(m12): domain 模型和 JSON DTO 扩展 TaskLinkInfo"
```

---

### Task 4: task_repo 扩展 — fromModel 加载 links

**Files:**
- Modify: `internal/storage/task_repo.go`
- Test: `internal/storage/task_repo_test.go`

- [ ] **Step 1: 新增 loadLinksByTask 函数**

遵循 `loadAssigneeUsers` 的批量加载模式：

```go
func (r *TaskRepository) loadLinksByTask(models []Task) (map[string][]domain.TaskLinkInfo, error) {
    taskUUIDs := make([]string, 0, len(models))
    for _, m := range models {
        taskUUIDs = append(taskUUIDs, m.UUID)
    }
    if len(taskUUIDs) == 0 {
        return nil, nil
    }
    linksMap, err := r.taskLinkRepo.LoadByTaskUUIDs(taskUUIDs)
    if err != nil {
        return nil, err
    }
    result := make(map[string][]domain.TaskLinkInfo, len(linksMap))
    for uuid, links := range linksMap {
        infos := make([]domain.TaskLinkInfo, 0, len(links))
        for _, l := range links {
            infos = append(infos, domain.TaskLinkInfo{
                ID: l.ID, Type: l.Type, URL: l.URL,
                Title: l.Title, CreatedAt: l.CreatedAt, CreatedBy: l.CreatedBy,
            })
        }
        result[uuid] = infos
    }
    return result, nil
}
```

- [ ] **Step 2: 修改 fromModel 签名，增加 links 参数**

`fromModel` 增加 `linksByTask map[string][]domain.TaskLinkInfo` 参数，在返回的 `domain.Task` 中设置 `Links`。

- [ ] **Step 3: 修改所有调用 fromModel 的地方**

`GetByID`、`List`、`GetByUUIDs` 等方法中，先调用 `loadLinksByTask`，再传入 `fromModel`。

- [ ] **Step 4: TaskRepository 构造函数增加 taskLinkRepo 参数**

确保 `TaskRepository` 持有 `taskLinkRepo` 引用。更新 `db.go` 中的初始化代码。

- [ ] **Step 5: 写 task_repo_test.go 测试**

测试 task 查询结果中包含 links 数据。

Run: `CGO_ENABLED=0 go test ./internal/storage/ -run TestTask -v`
Expected: 全部 PASS

- [ ] **Step 6: 提交**

```bash
git add internal/storage/task_repo.go internal/storage/task_repo_test.go internal/storage/db.go
git commit -m "feat(m12): task_repo 扩展加载 links"
```

---

### Task 5: App 层 — TaskAddLink / TaskRemoveLink

**Files:**
- Modify: `internal/app/workspace.go`
- Modify: `internal/app/service.go`
- Test: `internal/app/service_test.go`

- [ ] **Step 1: Service struct 增加 taskLinkRepo 字段**

在 `internal/app/service.go` 的 Service struct 中增加 `taskLinkRepo *sqlite.TaskLinkRepository`。更新初始化逻辑。

- [ ] **Step 2: 实现 TaskAddLink**

```go
func (s *Service) TaskAddLink(taskRef, linkType, url, title string) (domain.TaskLinkInfo, error) {
    if err := s.Require(PermissionTaskWrite); err != nil {
        return domain.TaskLinkInfo{}, err
    }
    // resolve task
    // check not completed/deleted
    // create TaskLink record
    // audit: task.link.add
    // return TaskLinkInfo
}
```

- [ ] **Step 3: 实现 TaskRemoveLink**

```go
func (s *Service) TaskRemoveLink(taskRef, linkID string) error {
    if err := s.Require(PermissionTaskWrite); err != nil {
        return err
    }
    // resolve task
    // verify link belongs to task
    // delete link
    // audit: task.link.remove
}
```

- [ ] **Step 4: 写 service_test.go 测试**

测试场景：
1. TaskAddLink 成功
2. TaskAddLink 重复 URL 失败
3. TaskRemoveLink 成功
4. TaskRemoveLink 不存在的 link 失败
5. TaskRemoveLink link 不属于该 task 失败
6. 已完成/已删除的任务不能添加 link

Run: `CGO_ENABLED=0 go test ./internal/app/ -run TestTaskLink -v`
Expected: 全部 PASS

- [ ] **Step 5: 运行全量 app 测试确认无破坏**

Run: `CGO_ENABLED=0 go test ./internal/app/ -v`
Expected: 全部 PASS

- [ ] **Step 6: 提交**

```bash
git add internal/app/workspace.go internal/app/service.go internal/app/service_test.go
git commit -m "feat(m12): App 层 TaskAddLink/TaskRemoveLink"
```

---

## Chunk 3: CLI

### Task 6: CLI link 子命令

**Files:**
- Create: `internal/cli/link.go`
- Modify: `internal/cli/root.go`
- Test: `tests/integration/cli_test.go`

- [ ] **Step 1: 创建 link.go**

实现三个子命令：

- `link add` — 参数：`<task-ref> --type <type> --url <url> [--title <title>]`
- `link list` — 参数：`<task-ref>`
- `link remove` — 参数：`<task-ref> <link-id>`

遵循现有 annotation.go 的模式：支持本地和远程两种模式。

- [ ] **Step 2: 在 root.go 注册 link action**

在 `handleTargetAction` 的 switch 中增加 `link` case，分发到 `newLinkCommand`。

- [ ] **Step 3: 写集成测试**

测试场景：
1. `taskg add "test task"`
2. `taskg 1 link add --type document --url https://example.com/doc`
3. `taskg 1 link list` — 输出包含 type 和 URL
4. `taskg 1 info` — 输出包含 links 部分
5. `taskg 1 --json` — JSON 中包含 links 数组
6. 重复 URL 失败
7. `taskg 1 link remove <link-id>` — 成功
8. `taskg 1 link list` — 输出为空

Run: `CGO_ENABLED=0 go test ./tests/integration/ -run TestLink -v`
Expected: 全部 PASS

- [ ] **Step 4: 提交**

```bash
git add internal/cli/link.go internal/cli/root.go tests/integration/cli_test.go
git commit -m "feat(m12): CLI link add/remove/list 子命令"
```

---

## Chunk 4: HTTP API 和 Remote Client

### Task 7: HTTP API link endpoints 和 Remote Client

**Files:**
- Modify: `internal/httpapi/tasks.go`
- Modify: `internal/httpapi/router.go`
- Modify: `internal/remote/task.go`
- Test: `internal/httpapi/tasks_test.go`

- [ ] **Step 1: 在 router.go 注册路由**

```go
api.With(s.authMiddleware).Post("/api/v1/tasks/{taskID}/links", s.handleTaskLinkAdd)
api.With(s.authMiddleware).Get("/api/v1/tasks/{taskID}/links", s.handleTaskLinkList)
api.With(s.authMiddleware).Delete("/api/v1/tasks/{taskID}/links/{linkID}", s.handleTaskLinkRemove)
```

- [ ] **Step 2: 实现 handlers**

遵循现有 annotate handler 的模式：
- `handleTaskLinkAdd` — 解析 `{type, url, title?}` JSON body，调用 `svc.TaskAddLink`
- `handleTaskLinkList` — 获取 task info，返回 links 数组
- `handleTaskLinkRemove` — 提取 linkID path param，调用 `svc.TaskRemoveLink`

- [ ] **Step 3: Remote Client 扩展**

在 `internal/remote/task.go` 新增：
- `AddTaskLink(ctx, taskUUID, linkType, url, title string) (TaskLinkDTO, error)`
- `RemoveTaskLink(ctx, taskUUID, linkID string) error`
- `ListTaskLinks(ctx, taskUUID string) ([]TaskLinkDTO, error)`

- [ ] **Step 4: 写 HTTP API 测试**

测试 CRUD endpoint 的正常和异常路径。

Run: `CGO_ENABLED=0 go test ./internal/httpapi/ -run TestTaskLink -v`
Expected: 全部 PASS

- [ ] **Step 5: 提交**

```bash
git add internal/httpapi/tasks.go internal/httpapi/router.go internal/httpapi/tasks_test.go internal/remote/task.go
git commit -m "feat(m12): HTTP API link endpoints 和 remote client"
```

---

## Chunk 5: MCP tools

### Task 8: MCP task.link_add / task.link_remove

**Files:**
- Modify: `internal/mcpserver/tools_task.go`
- Modify: `internal/mcpserver/schema_test.go` (golden file 更新)
- Test: `internal/mcpserver/integration_test.go`

- [ ] **Step 1: 定义输入 struct**

```go
type TaskLinkAddInput struct {
    Scope RequestScopeInput `json:"scope" jsonschema:"required"`
    Task  string            `json:"task" jsonschema:"required,description=Task reference (UUID or working-set ID)"`
    Type  string            `json:"type" jsonschema:"required,description=Link type (e.g. document, pr, ticket, design)"`
    URL   string            `json:"url" jsonschema:"required,description=External resource URL"`
    Title string            `json:"title,omitempty" jsonschema:"description=Optional display title"`
}

type TaskLinkRemoveInput struct {
    Scope  RequestScopeInput `json:"scope" jsonschema:"required"`
    Task   string            `json:"task" jsonschema:"required,description=Task reference (UUID or working-set ID)"`
    LinkID string            `json:"link_id" jsonschema:"required,description=Link ID to remove"`
}
```

- [ ] **Step 2: 注册 tools**

在 `registerTaskTools` 中新增：

```go
addTool(s, &mcp.Tool{
    Name:        "task.link_add",
    Description: "Add an external link to a task",
}, handleTaskLinkAdd)
addTool(s, &mcp.Tool{
    Name:        "task.link_remove",
    Description: "Remove an external link from a task",
}, handleTaskLinkRemove)
```

- [ ] **Step 3: 实现 handlers**

`handleTaskLinkAdd`: 获取 scoped service → resolve task → 调用 `svc.TaskAddLink` → 返回 link info
`handleTaskLinkRemove`: 获取 scoped service → resolve task → 调用 `svc.TaskRemoveLink` → 返回成功

- [ ] **Step 4: 更新 golden file**

Run: `CGO_ENABLED=0 go test ./internal/mcpserver/ -run TestSchema -v`
Expected: golden file 自动更新

- [ ] **Step 5: 写 integration test**

测试 MCP tool 的正常调用路径。

Run: `CGO_ENABLED=0 go test ./internal/mcpserver/ -run TestTaskLink -v`
Expected: 全部 PASS

- [ ] **Step 6: 提交**

```bash
git add internal/mcpserver/tools_task.go internal/mcpserver/integration_test.go internal/mcpserver/schema_test.go
git commit -m "feat(m12): MCP task.link_add/task.link_remove tools"
```

---

## Chunk 6: Render 和全量验证

### Task 9: Render 扩展

**Files:**
- Modify: `internal/render/table.go`

- [ ] **Step 1: TaskInfo 渲染增加 links 部分**

在 `TaskInfo` 输出中，annotations 之后、UDAs 之前，增加 links 展示：

```
Links:
  [document] 需求文档  https://feishu.cn/docx/xxx
  [pr] PR #123        https://github.com/xxx/pull/123
```

格式：`[type] title url`，如果 title 为空则只显示 `[type] url`。

- [ ] **Step 2: 提交**

```bash
git add internal/render/table.go
git commit -m "feat(m12): TaskInfo 渲染展示 links"
```

---

### Task 10: 全量验证

- [ ] **Step 1: 运行全量测试**

```bash
CGO_ENABLED=0 go test ./...
```

Expected: 全部 PASS

- [ ] **Step 2: 构建**

```bash
CGO_ENABLED=0 go build ./cmd/taskg
```

Expected: 成功

- [ ] **Step 3: 端到端手动验证**

```bash
./taskg workspace add test-ws
./taskg project add test-ws test-project --name "Test Project"
./taskg project test-ws test-project modify --description "这是一个测试项目，用于验证任务外部关联功能。项目约定：任务描述必须包含验收标准。飞书群：https://feishu.cn/group/xxx"
./taskg add "实现用户登录功能"
./taskg 1 link add --type document --url "https://feishu.cn/docx/abc" --title "需求文档"
./taskg 1 link add --type pr --url "https://github.com/org/repo/pull/123" --title "PR #123"
./taskg 1 info
./taskg 1 --json
./taskg 1 link list
./taskg 1 link remove <link-id>
```

验证：
1. `info` 输出包含 links 部分
2. `--json` 输出包含 links 数组
3. `link list` 正确列出
4. `link remove` 成功删除
5. 项目 description 可包含任意长文本

- [ ] **Step 4: 提交**

如有修复，提交修复。

---

### Task 11: 文档同步

**Files:**
- Modify: `ROADMAP.md`
- Modify: `README.md`（如有用户可见命令变化）

- [ ] **Step 1: 更新 ROADMAP.md**

M12 状态从"设计中"改为"已完成"。

- [ ] **Step 2: 更新 README.md**

在命令列表中增加 `link add`、`link list`、`link remove`。

- [ ] **Step 3: 提交**

```bash
git add ROADMAP.md README.md
git commit -m "docs(m12): 更新 ROADMAP 和 README"
```
