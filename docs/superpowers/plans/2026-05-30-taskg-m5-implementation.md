# taskg M5 实施计划

> **给 agentic workers 的要求：** 必须使用 `superpowers:subagent-driven-development`（如果可用）或 `superpowers:executing-plans` 执行本计划。所有步骤使用 checkbox（`- [ ]`）语法跟踪。

**目标：** 实现 M5：把 `project` 从任务自由字符串升级为 workspace 内一等实体，建立稳定 `project_id`、严格 project 注册、project 配置边界、project audit 维度，并为 M6 API / token 与 M7 MCP project scope 提供可复用 app service。

**架构：** 在 `internal/storage/sqlite` 中新增 `projects`、`configs`、`tasks.project_id` 和 `audit_logs.project_id`，并用 M5 raw SQL 迁移重建 `tasks` 以落地复合 FK。`internal/app` 负责 project 解析、权限、不变量校验、task 写路径绑定和 audit payload；`internal/cli` 只增加 `project` 命令组和薄参数路由。查询层保持结构化 AST，在 app 层把 `project:<slug>` 解析为 project id 后交给 storage 编译。

**技术栈：** Go 1.22、Cobra、GORM、`github.com/glebarez/sqlite`（保持 `CGO_ENABLED=0`）、现有 `internal/query`、`internal/task`、`internal/app`、`internal/render`、SQLite raw migration、黑盒 CLI 集成测试。

---

## 范围锁定

严格按 [M5 spec](/Users/mac/code/projects/dajee/task/docs/superpowers/specs/2026-05-30-taskg-m5-design.md) 实现。

必须进入 M5：

- `projects` 表、project repository、project app service。
- `tasks.project_id`，并保持 `tasks.project` 为 denormalized human 字段。
- SQLite `PRAGMA foreign_keys = ON` 可验证生效。
- `FOREIGN KEY (project_id, workspace_id) REFERENCES projects(id, workspace_id)` 通过重建 `tasks` 表落地。
- M4 数据库自动迁移 project 字符串到实体；非法 / 冲突 project 写入 `migration.m5.projects.skipped`。
- 严格 project 注册：`add/modify/edit/import/query/context` 中的 project 必须在 effective workspace 内解析。
- `project list/add/info/modify/archive`。
- `project config get/set/unset/list <project> ...`。
- `_projects --all`，且 `_projects` 不再从任务聚合。
- `_unique project` 保持任务聚合语义，只聚合当前查询结果中有效 `project_id` 绑定的 slug。
- task/project/config audit 中的 project payload 与 `audit_logs.project_id`。
- `ResolveProject(ref)` 与 `ResolveProjectInWorkspace(workspaceID, ref)` app service 方法。

不进入 M5：

- HTTP API、远程 CLI、PAT / Agent token、MCP。
- project 成员表、project owner、project RBAC 覆盖 workspace role。
- project hard delete、restore、slug rename、workspace 间 project move。
- 完整 Taskwarrior project 兼容或自动创建 project。

## 执行注意事项

- 如果实现中任何细节和 spec 有差异，优先改实现；只有发现 spec 本身不合理才更新 spec，并在提交说明中解释。
- 不要把 project 自动创建带回任务写路径；迁移是唯一例外。
- 不要让 `project_id` 进入 JSON export、edit JSON 或 orphan UDA。
- 不要让 storage repo 自行决定 effective workspace；workspace/project 解析在 app 层。
- 不要让 `config get/set/list` 无 scope 访问 project 配置。
- SQLite FK 必须真实打开，测试要能证明跨 workspace project_id 被拒绝。
- 每个 chunk 完成后至少跑对应包测试；跨 chunk 交界处跑 `go test ./...`。

## 文件结构

新增文件：

- `internal/storage/sqlite/project_repo.go`
  project CRUD、slug/id 解析、archive、task count、迁移用批量创建。
- `internal/storage/sqlite/config_repo.go`
  通用 `configs` scope 存储；M5 只使用 project scope，但 DDL 支持 server/workspace/project/user。
- `internal/storage/sqlite/migration_m5_tasks.go`
  M5 `tasks` 表 raw DDL、M4 列清单与索引快照、重建 tasks 表、迁移关联表完整性检查。
- `internal/app/project.go`
  `AddProject/ListProjects/ProjectInfo/ModifyProject/ArchiveProject/ResolveProject/ResolveProjectInWorkspace`。
- `internal/app/project_config.go`
  `ProjectConfigGet/Set/Unset/List` 和 project config 权限、audit。
- `internal/app/project_query.go`
  project query AST rewrite、project invariant 校验 helper。
- `internal/cli/project.go`
  `taskg project ...` 命令组和 `project config ...` 子命令。

修改文件：

- `internal/storage/sqlite/models.go`
  新增 `Project`、`Config`；Task 2 后再扩展 `Task.ProjectID`；Task 1 扩展 `AuditLog.ProjectID`。
- `internal/storage/sqlite/db.go`
  M5 迁移入口、`BEGIN IMMEDIATE` raw migration、`tasks` 表重建、FK/索引创建、迁移报告 meta。
- `internal/storage/sqlite/db_test.go`
  M5 迁移、FK、幂等测试。
- `internal/storage/sqlite/task_repo.go`
  持久化 `ProjectID`；`Projects` 改为 project repo 能力后删或仅保留兼容调用；列表查询支持 project invariant。
- `internal/storage/sqlite/query_scope.go`
  `AttrProject` 改为基于 `project_id` 编译；`project:` 空值编译为 `project_id IS NULL`。
- `internal/storage/sqlite/audit_repo.go`
  `AuditLogEntry.ProjectID`、按 project 过滤的预留字段。
- `internal/query/ast.go`
  新增内部属性 `AttrProjectID`，仅供 app 层 rewrite 后交给 storage 编译。
- `internal/query/errors.go`
  新增 `ErrProjectPredicateUnresolved` sentinel error，防止未 rewrite 的 `project:<slug>` 谓词泄漏到 SQL 编译层。
- `internal/task/model.go`
  `Task.ProjectID *string`，validate 不做 DB 解析。
- `internal/task/json.go`
  export 不输出 `project_id`；import/edit 遇到 `project_id` 显式拒绝，不当 orphan UDA。
- `internal/app/service.go`
  add/modify/edit/import/export/recurrence 接入 project 解析、绑定、invariant、audit payload。
- `internal/app/audit.go`
  audit entry 增加 `ProjectID`，payload helper 记录 before/after project。
- `internal/app/permission.go`
  新增 project manage/config 权限。
- `internal/app/workspace.go`
  role permission map 增加 project 权限。
- `internal/app/context.go`
  context 中 project filter 的执行时解析。
- `internal/app/service_test.go`
  project app、task write path、query/context、audit、recurrence 单元测试。
- `internal/cli/root.go`
  注册 project 命令，确保 root reorder 识别 `project` 子命令。
- `internal/cli/helper.go`
  `_projects --all`；默认从 project service 读取 active project。
- `internal/cli/config.go`
  无 scope 的 project config key 报 `project_config_scope_required`。
- `internal/cli/import_export.go`
  import/export 错误透传；project_id 保留字段测试。
- `tests/integration/cli_test.go`
  M5 黑盒 CLI 流程。
- `README.md`、`ROADMAP.md`、`docs/requirements.md`
  M5 完成后同步状态与用户用法。

---

## Chunk 1：Storage Schema 与 M5 迁移

### Task 1：新增 Project / Config / Audit ProjectID 模型

**Files:**
- Modify: `internal/storage/sqlite/models.go`
- Test: `internal/storage/sqlite/db_test.go`

- [x] **Step 1：写失败的 schema 测试**

在 `internal/storage/sqlite/db_test.go` 新增：

```go
func TestOpenCreatesM5ProjectAndConfigSchema(t *testing.T) {
    store, err := Open(filepath.Join(t.TempDir(), "taskg.db"))
    if err != nil { t.Fatal(err) }
    t.Cleanup(func() { _ = store.Close() })

    for _, table := range []any{&Project{}, &Config{}} {
        if !store.DB().Migrator().HasTable(table) {
            t.Fatalf("missing table for %T", table)
        }
    }
    if !store.DB().Migrator().HasColumn(&AuditLog{}, "project_id") {
        t.Fatalf("missing audit_logs.project_id")
    }

    assertRawDDLContains(t, store, "configs", "PRIMARY KEY (`workspace_id`,`scope`,`scope_id`,`key`)")
    assertRawInsertNullRejected(t, store, "INSERT INTO configs(workspace_id, scope, scope_id, key, value) VALUES(NULL, 'server', '', 'x', 'y')")
    assertIndexColumns(t, store, "idx_projects_ws_slug", []string{"workspace_id", "slug"})
    assertIndexColumns(t, store, "idx_projects_id_ws", []string{"id", "workspace_id"})
}
```

- [x] **Step 2：运行测试确认失败**

Run:

```bash
go test ./internal/storage/sqlite -run TestOpenCreatesM5ProjectAndConfigSchema -count=1
```

Expected: FAIL，`Project` / `Config` / `audit_logs.project_id` 不存在，或 `configs` DDL 不满足非 NULL / 复合主键要求。

- [x] **Step 3：扩展 GORM models**

在 `models.go` 增加：

```go
type Project struct {
    ID           string `gorm:"primaryKey;uniqueIndex:idx_projects_id_ws,priority:1"`
    WorkspaceID  string `gorm:"not null;uniqueIndex:idx_projects_ws_slug;uniqueIndex:idx_projects_id_ws,priority:2;index:idx_projects_ws_status,priority:1"`
    Slug         string `gorm:"not null;uniqueIndex:idx_projects_ws_slug"`
    Name         string `gorm:"not null"`
    Description  string `gorm:"not null;default:''"`
    Status       string `gorm:"not null;default:'active';index:idx_projects_ws_status,priority:2"`
    SettingsJSON string `gorm:"not null;default:'{}'"`
    CreatedAt    int64  `gorm:"not null"`
    ModifiedAt   int64  `gorm:"not null"`
    ArchivedAt   *int64
}

type Config struct {
    WorkspaceID string `gorm:"primaryKey;not null;default:''"`
    Scope       string `gorm:"primaryKey;not null"`
    ScopeID     string `gorm:"primaryKey;not null;default:''"`
    Key         string `gorm:"primaryKey;not null"`
    Value       string `gorm:"not null"`
}
```

修改：

```go
type AuditLog struct {
    // 保留现有字段。
    ProjectID *string `gorm:"index:idx_audit_project_time,priority:2"`
    WorkspaceID *string `gorm:"index;index:idx_audit_ws_time,priority:1;index:idx_audit_project_time,priority:1"`
    CreatedAt int64 `gorm:"not null;index;index:idx_audit_ws_time,priority:2,sort:desc;index:idx_audit_project_time,priority:3,sort:desc"`
}
```

注意：

- Task 1 不修改 `Task` model，不让 AutoMigrate 抢先 `ALTER TABLE tasks ADD COLUMN project_id`。
- `tasks.project_id`、`idx_tasks_ws_project_id` 和复合 FK 全部由 Task 2 raw migration 一次性落地。
- `Project.Description` 沿用 M4 workspace 约定，空字符串表示无描述，DB 固定 `TEXT NOT NULL DEFAULT ''`。

- [x] **Step 4：AutoMigrate 新表和可 ALTER 新列**

在 `db.go` 的非 task schema 迁移中加入 `Project{}`、`Config{}`，并让 `AuditLog{}` 通过 AutoMigrate 增加 `project_id`。不要在 Task 1 把 `Task.ProjectID` 加入 model，也不要依赖 AutoMigrate 处理 `tasks.project_id`。

- [x] **Step 5：运行 schema 测试通过**

Run:

```bash
go test ./internal/storage/sqlite -run TestOpenCreatesM5ProjectAndConfigSchema -count=1
```

Expected: PASS。

- [x] **Step 6：提交**

```bash
git add internal/storage/sqlite/models.go internal/storage/sqlite/db.go internal/storage/sqlite/db_test.go
git commit -m "feat: 添加 M5 project 存储模型"
```

### Task 2：实现 M5 raw migration、FK 和迁移报告

**Files:**
- Modify: `internal/storage/sqlite/db.go`
- Create: `internal/storage/sqlite/migration_m5_tasks.go`
- Modify: `internal/storage/sqlite/db_test.go`
- Modify: `internal/storage/sqlite/task_repo.go`

- [x] **Step 1：写 M4 升级迁移测试**

在 `db_test.go` 中用旧 schema 手工建 M4 形态数据库：

```go
func TestOpenMigratesM4ProjectStringsToProjects(t *testing.T) {
    dbPath := filepath.Join(t.TempDir(), "taskg.db")
    seedM4DatabaseWithTasks(t, dbPath, []seedTask{
        {WorkspaceSlug: "local", UUID: "t1", Project: ptr("Customer-A"), Entry: 10},
        {WorkspaceSlug: "local", UUID: "t2", Project: ptr("customer-b"), Entry: 20},
        {WorkspaceSlug: "local", UUID: "t3", Project: nil, Entry: 30},
    })

    store, err := Open(dbPath)
    if err != nil { t.Fatal(err) }
    defer store.Close()

    var projects []Project
    if err := store.DB().Order("slug").Find(&projects).Error; err != nil { t.Fatal(err) }
    if got := projectSlugs(projects); !reflect.DeepEqual(got, []string{"customer-a", "customer-b"}) {
        t.Fatalf("project slugs = %#v", got)
    }
    assertTaskProject(t, store, "t1", "customer-a", projects[0].ID)
}
```

再加：

- `TestOpenMigratesM5ProjectStringsIdempotently`
- `TestOpenMigratesInvalidAndConflictingProjectsToReport`
- `TestOpenEnablesForeignKeyChecksForTaskProject`
- `TestM5MigrationColumnsMatchM4Snapshot`
- `TestM5MigrationIndexesMatchM5Snapshot`
- `TestM5MigrationPreservesTaskRelations`

`TestM5MigrationPreservesTaskRelations` 必须验证迁移前后的 `task_tags`、`task_annotations`、`task_dependencies`、`task_uda_values` 都能 round-trip。

- [x] **Step 2：运行测试确认失败**

Run:

```bash
go test ./internal/storage/sqlite -run 'TestOpenMigratesM4ProjectStrings|TestOpenEnablesForeignKeyChecks|TestOpenMigratesInvalidAndConflictingProjects' -count=1
```

Expected: FAIL。

- [x] **Step 3：实现迁移入口**

在 `db.go` 中把顺序固定为：

```go
func (s *Store) migrate() error {
    if err := s.prepareWorkspaceSchemaForM4(); err != nil { return err }
    // 先迁移非 task schema。Task 表由 prepareProjectSchemaForM5 负责创建或重建，
    // 避免 AutoMigrate 在 SQLite 上先添加 project_id，导致复合 FK 无法可靠落地。
    if err := s.db.AutoMigrate(&Meta{}, &User{}, &Workspace{}, &Membership{}, &AuditLog{}, &Project{}, &Config{}, &Context{}, &UDADefinition{}); err != nil { return err }
    if err := s.prepareProjectSchemaForM5(); err != nil { return err }
    return s.db.AutoMigrate(&TaskTag{}, &TaskAnnotation{}, &TaskDependency{}, &TaskUDAValue{})
}
```

`prepareProjectSchemaForM5()` 必须：

- 用 raw SQL `BEGIN IMMEDIATE` 获取写锁。
- 在代码注释中说明使用 `BEGIN IMMEDIATE` 的原因：重建 `tasks` 表期间必须在 DDL 前拿到写锁，避免两个进程交错执行迁移。不要求写驱动相关的并发锁测试。
- 检查 `tasks` 是否已经有复合 FK marker。固定使用 `meta` key `migration.m5.projects.applied=true`，并结合 `PRAGMA foreign_key_list(tasks)` 防止假阳性。
- 如果是全新数据库且 `tasks` 表不存在，直接用 M5 raw DDL 创建 `tasks` 表和索引，写入 marker 后返回；不要再让 GORM AutoMigrate 创建 tasks。
- 收集 `tasks.project`，按 workspace 分组，`raw -> slug/name`。
- 空字符串归一为空。
- 非法 slug 或规范化冲突写入 `migration.m5.projects.skipped` JSON array。
- 创建合法 projects。
- 重建 `tasks`，在 `INSERT INTO ... SELECT ...` 时通过 `LEFT JOIN projects` 写入归一化后的 `project` 与 `project_id`：
  - `ALTER TABLE tasks RENAME TO tasks_old_m5`
  - `CREATE TABLE tasks (...)` 使用 `migration_m5_tasks.go` 中固定的 M4 列清单 + `project_id TEXT` + `FOREIGN KEY (project_id, workspace_id) REFERENCES projects(id, workspace_id)`
  - `INSERT INTO tasks (...) SELECT ... FROM tasks_old_m5 LEFT JOIN projects ...`，合法 project 写入 project id 和规范化 slug，非法 / 冲突 project 写入 `NULL`，旧空字符串 project 也写成 `NULL`
  - `DROP TABLE tasks_old_m5`
  - 重建索引。
- `COMMIT`；失败时 `ROLLBACK`。

不要在这个迁移中创建 project winner。冲突集合全部进入 skipped report。

`migration_m5_tasks.go` 必须硬编码 M4 时刻 `tasks` 表完整列定义，而不是用字符串占位：

```go
var m5TaskColumns = []string{
    "uuid", "workspace_id", "description", "status", "entry", "modified",
    "end_ts", "due", "project", "priority",
    "start", "wait", "scheduled", "until",
    "recur", "parent", "mask", "imask",
}
```

如果实际 M4 schema 列名与上面不同，以当前代码为准修正；`TestM5MigrationColumnsMatchM4Snapshot` 用 `PRAGMA table_info(tasks)` 锁定这份快照，防止迁移代码和 model 漂移。

推荐的复制 SQL 形态如下，避免先插入再多次 UPDATE：

```sql
INSERT INTO tasks (
  uuid, workspace_id, description, status, entry, modified,
  end_ts, due, project, priority, start, wait, scheduled, until,
  recur, parent, mask, imask, project_id
)
SELECT
  t.uuid, t.workspace_id, t.description, t.status, t.entry, t.modified,
  t.end_ts, t.due,
  CASE WHEN p.id IS NOT NULL THEN p.slug ELSE NULL END AS project,
  t.priority, t.start, t.wait, t.scheduled, t.until,
  t.recur, t.parent, t.mask, t.imask,
  p.id AS project_id
FROM tasks_old_m5 t
LEFT JOIN projects p
  ON p.workspace_id = t.workspace_id
 AND p.slug = LOWER(TRIM(t.project));
```

非法 slug 和规范化冲突的 project 不会创建 project 行，因此 `LEFT JOIN` 得到 `NULL`，自然清空任务 project。实现中如需区分非法和冲突原因，应在插入前生成 `migration.m5.projects.skipped`，不要靠最终 tasks 行反推。

`migration_m5_tasks.go` 还必须硬编码 M5 后 `tasks` 索引快照：

```go
var m5TaskIndexes = []string{
    "CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status)",
    "CREATE INDEX IF NOT EXISTS idx_tasks_wait ON tasks(wait)",
    "CREATE INDEX IF NOT EXISTS idx_tasks_scheduled ON tasks(scheduled)",
    "CREATE INDEX IF NOT EXISTS idx_tasks_until ON tasks(until)",
    "CREATE INDEX IF NOT EXISTS idx_tasks_recur ON tasks(recur)",
    "CREATE INDEX IF NOT EXISTS idx_tasks_parent ON tasks(parent)",
    "CREATE INDEX IF NOT EXISTS idx_tasks_ws_project_id ON tasks(workspace_id, project_id)",
    "CREATE UNIQUE INDEX IF NOT EXISTS idx_task_parent_due_open ON tasks(parent, due) WHERE status IN ('pending', 'waiting') AND parent IS NOT NULL AND due IS NOT NULL",
}
```

M5 明确删除 `tasks.project` 的运行时索引：

- `tasks.project` 列继续保留，作为 human 输出、迁移导出和 invariant 检查的 denormalized 字段。
- M5 查询不再走 `tasks.project`，而是走 `project_id`；因此不要保留或重建 `idx_tasks_project`。
- 在 Task 5 给 `Task.Project` 去掉 `gorm:"index"` tag，避免后续 AutoMigrate 重新创建死索引。
- 迁移结束后执行 `DROP INDEX IF EXISTS idx_tasks_workspace_id` 与 `DROP INDEX IF EXISTS idx_tasks_project`。

- [x] **Step 4：实现 migration report 类型**

在 `db.go` 中定义私有结构：

```go
type m5SkippedProject struct {
    WorkspaceID string `json:"workspace_id"`
    TaskUUID    string `json:"task_uuid"`
    RawProject  string `json:"raw_project"`
    Reason      string `json:"reason"`
}
```

写入 meta key `migration.m5.projects.skipped`，值为空时写 `[]` 或删除 key 二选一。本计划固定为写 `[]`，便于 `config get` 稳定读取。

- [x] **Step 5：运行迁移测试通过**

Run:

```bash
go test ./internal/storage/sqlite -run 'TestOpenMigratesM4ProjectStrings|TestOpenEnablesForeignKeyChecks|TestOpenMigratesInvalidAndConflictingProjects|TestOpenMigratesM5ProjectStringsIdempotently' -count=1
```

Expected: PASS。

- [x] **Step 6：提交**

```bash
git add internal/storage/sqlite/db.go internal/storage/sqlite/migration_m5_tasks.go internal/storage/sqlite/db_test.go internal/storage/sqlite/task_repo.go
git commit -m "feat: 迁移 project 实体和任务外键"
```

---

## Chunk 2：Project / Config Repository

### Task 3：ProjectRepository

**Files:**
- Create: `internal/storage/sqlite/project_repo.go`
- Test: `internal/storage/sqlite/project_repo_test.go`

- [x] **Step 1：写 repository 测试**

覆盖：

- `(workspace_id, slug)` 唯一。
- 不同 workspace 可复用 slug。
- `GetByRef(workspaceID, slug)`。
- `GetByRef(workspaceID, projectID)` 且 projectID 不属于 workspace 返回 `ErrNotFound` 或 typed mismatch error。
- `Archive` 已归档返回 `ErrAlreadyArchived`。
- `List(includeArchived)` 按 slug 升序。
- `TaskCounts(workspaceID, projectIDs)` 统计 `status != deleted`。

- [x] **Step 2：运行测试失败**

```bash
go test ./internal/storage/sqlite -run ProjectRepository -count=1
```

- [x] **Step 3：实现 repository**

核心 API：

```go
type ProjectStatus string

const (
    ProjectStatusActive   = "active"
    ProjectStatusArchived = "archived"
)

type ProjectRepository struct { db *gorm.DB }

func NewProjectRepository(db *gorm.DB) *ProjectRepository
func (r *ProjectRepository) Create(Project) (Project, error)
func (r *ProjectRepository) List(workspaceID string, includeArchived bool) ([]Project, error)
func (r *ProjectRepository) GetByID(id string) (Project, error)
func (r *ProjectRepository) GetBySlug(workspaceID, slug string) (Project, error)
func (r *ProjectRepository) ResolveInWorkspace(workspaceID, ref string) (Project, error)
func (r *ProjectRepository) Update(Project) error
func (r *ProjectRepository) Archive(workspaceID, id string, now int64) error
func (r *ProjectRepository) TaskCounts(workspaceID string, projectIDs []string) (map[string]int, error)
```

Repository 层错误固定：

- project 不存在：返回现有 `ErrNotFound` 或同等 storage sentinel。
- project 已归档：返回 `ErrAlreadyArchived`，service 层包装为 `RuntimeError{Code:"project_archived"}`。

- [x] **Step 4：运行测试通过并提交**

```bash
go test ./internal/storage/sqlite -run ProjectRepository -count=1
git add internal/storage/sqlite/project_repo.go internal/storage/sqlite/project_repo_test.go
git commit -m "feat: 添加 project repository"
```

### Task 4：ConfigRepository 和 Audit project_id

**Files:**
- Create: `internal/storage/sqlite/config_repo.go`
- Modify: `internal/storage/sqlite/audit_repo.go`
- Test: `internal/storage/sqlite/config_repo_test.go`
- Test: `internal/storage/sqlite/identity_repo_test.go`

- [x] **Step 1：写配置和 audit 测试**

覆盖：

- project scope 写入：`workspace_id=<ws> scope=project scope_id=<projectID>`。
- server scope 用空串，不允许重复 key。
- `configs` raw DDL 包含 `PRIMARY KEY(workspace_id, scope, scope_id, key)`。
- raw SQL 写入 `workspace_id = NULL` 或 `scope_id = NULL` 必须失败。
- `ListScope` 按 key 升序。
- audit append/list 保留 `ProjectID`。
- `AuditListOptions` 预留 `ProjectID *string` filter，按 `(workspace_id, project_id, created_at)` 查询。

- [x] **Step 2：实现 ConfigRepository**

API：

```go
type ConfigScope string

const (
    ConfigScopeServer    ConfigScope = "server"
    ConfigScopeWorkspace ConfigScope = "workspace"
    ConfigScopeProject   ConfigScope = "project"
    ConfigScopeUser      ConfigScope = "user"
)

type ConfigKey struct {
    WorkspaceID string
    Scope       ConfigScope
    ScopeID     string
    Key         string
}

func (r *ConfigRepository) Get(ConfigKey) (string, bool, error)
func (r *ConfigRepository) Set(ConfigKey, value string) error
func (r *ConfigRepository) Unset(ConfigKey) error
func (r *ConfigRepository) ListScope(workspaceID string, scope ConfigScope, scopeID string) (map[string]string, error)
```

`ConfigRepository` 必须始终把 `WorkspaceID` 和 `ScopeID` 归一成非 NULL 字符串：

- server scope：`WorkspaceID=""`、`ScopeID=""`
- workspace scope：`WorkspaceID=<workspace_id>`、`ScopeID=<workspace_id>`
- project scope：`WorkspaceID=<workspace_id>`、`ScopeID=<project_id>`
- user scope：M5 不使用，若实现预留也必须显式传入非空 `ScopeID`

`Set` / `Get` / `Unset` 传入 `WorkspaceID == ""` 但 scope 不是 server 时应返回 typed error，避免调用方无意写到全局空 workspace。

- [x] **Step 3：扩展 AuditLogEntry**

`AuditLogEntry` 和 `AuditLog` 双向映射新增 `ProjectID *string`。

`AuditRepository.List` / `AuditListOptions` 增加 `ProjectID *string` 可选过滤字段。M5 CLI 暂不一定暴露该参数，但 repository 层要铺好 M7 project timeline 的索引使用路径。

- [x] **Step 4：运行测试通过并提交**

```bash
go test ./internal/storage/sqlite -run 'ConfigRepository|AuditRepository' -count=1
git add internal/storage/sqlite/config_repo.go internal/storage/sqlite/config_repo_test.go internal/storage/sqlite/audit_repo.go internal/storage/sqlite/identity_repo_test.go
git commit -m "feat: 添加 scoped config 存储"
```

---

## Chunk 3：Domain DTO 与 Task Repository 接入 ProjectID

### Task 5：Task.ProjectID 与 JSON 保留字段

**Files:**
- Modify: `internal/task/model.go`
- Modify: `internal/task/json.go`
- Test: `internal/task/json_test.go`
- Test: `internal/task/model_test.go`

- [x] **Step 1：写失败测试**

新增：

```go
func TestJSONTaskRejectsProjectIDAsReservedField(t *testing.T) {
    var dto JSONTask
    err := json.Unmarshal([]byte(`{"uuid":"u","description":"x","entry":"1970-01-01T00:00:01Z","modified":"1970-01-01T00:00:02Z","project_id":"p1"}`), &dto)
    if err == nil || !strings.Contains(err.Error(), "project_id") {
        t.Fatalf("err = %v, want reserved project_id", err)
    }
}

func TestJSONTaskDoesNotExportProjectID(t *testing.T) {
    project := "api"
    projectID := "p1"
    data, err := json.Marshal(ToJSON(Task{UUID:"u", Description:"x", Status:StatusPending, Entry:1, Modified:1, Project:&project, ProjectID:&projectID}))
    if err != nil { t.Fatal(err) }
    if bytes.Contains(data, []byte("project_id")) {
        t.Fatalf("export leaked project_id: %s", data)
    }
}

func TestJSONTaskKeepsOrphanUDAButRejectsReservedProjectID(t *testing.T) {
    var dto JSONTask
    err := json.Unmarshal([]byte(`{"uuid":"u","description":"x","entry":"1970-01-01T00:00:01Z","modified":"1970-01-01T00:00:02Z","legacy_field":"kept"}`), &dto)
    if err != nil { t.Fatal(err) }
    if got := dto.UDAs["legacy_field"]; got != "kept" {
        t.Fatalf("legacy_field = %q", got)
    }

    err = json.Unmarshal([]byte(`{"uuid":"u","description":"x","entry":"1970-01-01T00:00:01Z","modified":"1970-01-01T00:00:02Z","project_id":"p1"}`), &dto)
    if err == nil || !strings.Contains(err.Error(), "project_id") {
        t.Fatalf("err = %v, want reserved project_id", err)
    }
}
```

- [x] **Step 2：实现**

`Task` 增加 `ProjectID *string`。`Task.Project` 去掉 `gorm:"index"`，因为 M5 查询不再使用 `tasks.project` 字符串列。`JSONTask` 不增加 `ProjectID` 字段。

在 `internal/task/json.go` 新增 `reservedJSONFields()` 或等价集中列表：

```go
func reservedJSONFields() map[string]struct{} {
    return map[string]struct{}{
        "project_id": {},
    }
}
```

`UnmarshalJSON` 删除 core fields 前先检测 reserved field，命中返回 `project_id is reserved; use project:<slug> to modify project`。`project_id` 不加入 orphan UDA；其它未知字段继续按 M3 orphan UDA 保留。

- [x] **Step 3：运行测试通过并提交**

```bash
go test ./internal/task -run 'ProjectID|JSONTask' -count=1
git add internal/task/model.go internal/task/json.go internal/task/json_test.go internal/task/model_test.go
git commit -m "feat: 保留内部 project_id 字段"
```

### Task 6：TaskRepository 持久化 ProjectID

**Files:**
- Modify: `internal/storage/sqlite/task_repo.go`
- Test: `internal/storage/sqlite/task_repo_test.go`

- [x] **Step 1：写失败测试**

覆盖：

- Create/Update round-trip `Task.ProjectID`。
- `project_id != nil` 但跨 workspace 的 raw insert 被 FK 拒绝。
- `List` 返回 ProjectID。

- [x] **Step 2：实现 toModel/fromModel/Update**

在 `TaskRepository.Update` 的 Updates map 加 `project_id`。

- [x] **Step 3：运行测试通过并提交**

```bash
go test ./internal/storage/sqlite -run 'TaskRepository.*ProjectID|ForeignKey' -count=1
git add internal/storage/sqlite/task_repo.go internal/storage/sqlite/task_repo_test.go
git commit -m "feat: 任务持久化 project_id"
```

---

## Chunk 4：Project App Service、权限与配置

### Task 7：Project app service

**Files:**
- Create: `internal/app/project.go`
- Modify: `internal/app/service.go`
- Modify: `internal/app/permission.go`
- Modify: `internal/app/workspace.go`
- Test: `internal/app/service_test.go`

- [x] **Step 1：写 service 测试**

覆盖：

- owner/admin 可 `AddProject/ModifyProject/ArchiveProject`。
- member/viewer 不可写 project。
- viewer/member 可 `ListProjects/ProjectInfo`。
- `ProjectInfo(projectID)` 携带 `--workspace` 等效 runtime 时必须校验 workspace。
- 同 slug 不同 workspace 互不冲突。
- 已归档 project 再次 archive 返回 `project_archived`。
- archived project 的 `TaskCount` 仍然统计 `status != deleted` 且绑定该 project 的任务，不因 project status 归零。

- [x] **Step 2：扩展 Service 初始化**

`Service` 增加：

```go
projectRepo *sqlite.ProjectRepository
configRepo  *sqlite.ConfigRepository
```

`NewService` 和 `withStore` 初始化它们。

- [x] **Step 3：实现 project 方法**

核心类型：

```go
type ProjectView struct {
    ID string
    WorkspaceID string
    Slug string
    Name string
    Description string
    Status string
    ArchivedAt *int64
    TaskCount int
    CreatedAt int64
    ModifiedAt int64
}

type AddProjectInput struct { Slug, Name, Description string }
type ModifyProjectInput struct { Name *string; Description *string }
```

实现：

- `AddProject`
- `ListProjects(includeArchived bool)`
- `ProjectInfo(ref string)`
- `ModifyProject(ref string, input ModifyProjectInput)`
- `ArchiveProject(ref string)`
- `ResolveProject(ref string)`
- `ResolveProjectInWorkspace(workspaceID, ref string)`

`ListProjects` 组装方式固定为：先 `ProjectRepository.List` 一次取出 project，再调用 `TaskCounts(workspaceID, projectIDs)` 一次性 `GROUP BY project_id` 获取计数，最后合并成 `ProjectView`；不要在循环里逐个 project 计数。

错误使用 `RuntimeError` code：

- `project_invalid_slug`
- `project_not_found`
- `project_archived`
- `project_workspace_mismatch`
- `project_already_exists`
- `project_name_required`
- `project_slug_immutable`
- `permission_denied`

- [x] **Step 4：权限映射**

新增：

```go
PermissionProjectRead
PermissionProjectManage
PermissionProjectConfigRead
PermissionProjectConfigWrite
```

role 规则：

- viewer/member: read + config read。
- admin/owner: manage + config write。

`PermissionProjectManage` 只控制 project 实体写操作（add/modify/archive），不影响 member 在 task write 中引用已存在的 active project。member 仍可创建或修改任务并绑定 active project，前提是拥有既有 task write 权限。

- [x] **Step 5：audit**

`project.add/modify/archive` 必须写 audit。`AuditEntry.ProjectID` 填 project id。

- [x] **Step 6：运行测试通过并提交**

```bash
go test ./internal/app -run 'Project|Permission' -count=1
git add internal/app/project.go internal/app/service.go internal/app/permission.go internal/app/workspace.go internal/app/service_test.go
git commit -m "feat: 添加 project app service"
```

### Task 8：Project config app service

**Files:**
- Create: `internal/app/project_config.go`
- Modify: `internal/app/uda.go`
- Test: `internal/app/service_test.go`

- [x] **Step 1：写失败测试**

覆盖：

- admin/owner 可 set/unset。
- viewer/member 可 get/list 不可 set。
- `project config` 对 archived project 读允许，写拒绝。
- `SetConfig("agent.background", "...")` 无 project scope 报 `project_config_scope_required`。
- project config audit 填 `ProjectID`。

- [x] **Step 2：实现 project config key 白名单**

M5 允许：

```go
var projectConfigKeys = map[string]bool{
    "agent.background": true,
    "agent.constraints": true,
    "context.default": true,
}
```

无 scope 的 `SetConfig/UnsetConfig/GetConfig` 遇到这些 key 返回 `RuntimeError{Code:"project_config_scope_required"}`。

- [x] **Step 3：实现 service 方法**

```go
func (s *Service) ProjectConfigGet(projectRef, key string) (string, bool, error)
func (s *Service) ProjectConfigSet(projectRef, key, value string) error
func (s *Service) ProjectConfigUnset(projectRef, key string) error
func (s *Service) ProjectConfigList(projectRef string) (map[string]string, error)
```

- [x] **Step 4：运行测试通过并提交**

```bash
go test ./internal/app -run 'ProjectConfig|ConfigScope' -count=1
go vet ./...
git add internal/app/project_config.go internal/app/uda.go internal/app/service_test.go
git commit -m "feat: 添加 project 配置服务"
```

---

## Chunk 5：Task 写路径严格 Project 绑定

### Task 9：Add / Modify / Edit 接入 project 解析

**Files:**
- Modify: `internal/app/service.go`
- Create: `internal/app/project_query.go`
- Test: `internal/app/service_test.go`

- [x] **Step 1：写失败测试**

覆盖：

- `Add(project:missing)` 返回 `project_not_found`。
- `Add(project:archived)` 返回 `project_archived`。
- `Add(project:active)` 写入 `ProjectID` 和规范化 `Project` slug。
- `Modify(project:)` 清空 `ProjectID` 和 `Project`。
- 已归档 project 上的任务只能清空或改到 active project，不能改到另一个 archived project。
- `ReplaceEditableTask` 修改 project 字符串会重新解析。
- `ReplaceEditableTask` 输入含 `ProjectID` 不允许通过 JSON 路径进入。

- [x] **Step 2：实现 helper**

在 `project_query.go` 或 `project.go`：

```go
type projectBinding struct {
    ID *string
    Slug *string
    Archived bool
}

type projectChange struct {
    Before projectBinding
    After  projectBinding
}

func (s *Service) resolveActiveProjectBinding(slug *string) (projectBinding, error)
func (s *Service) clearProjectBinding(tsk *task.Task)
func (s *Service) applyProjectBinding(tsk *task.Task, slug *string) (projectChange, error)
func (s *Service) validateTaskProjectInvariant(tsk task.Task) error
```

规则：

- nil 表示不修改，`projectChange.Before == projectChange.After`。
- empty string 表示清空。
- 非空必须 active。
- `ProjectID != nil` 时 `Project` 必须等于 project slug。
- audit 直接消费 `projectChange`，不要在每个 task 写路径手工重复计算 before/after。

- [x] **Step 3：接入 Add/Modify/Edit**

`addLocked`、`createRecurringParent`、`modifyLocked`、`replaceEditableTaskLocked` 调用 project helper。

- [x] **Step 4：audit before/after**

修改 `AuditEntry`：

```go
type AuditEntry struct {
    // 保留现有字段。
    ProjectID *string
    Payload map[string]any
}
```

task 写 audit payload 添加：

- `before_project_id`
- `before_project_slug`
- `after_project_id`
- `after_project_slug`

`audit_logs.project_id` 填写规则按 spec：移出/移动填 before；从无到有填 after；普通任务写填当前 project。

精确定义：

- 操作不修改 project：填当前 project id（before 与 after 相同）。
- 操作清空 project：填 before project id。
- 操作从无 project 绑定到 project：填 after project id。
- 操作从 project A 改到 project B：填 before project id，让 project A 的时间线能看到任务离开。

必须逐个 retro-fit 所有 M4 task 写 audit 调用点：

- `task.add`
- `task.modify`
- `task.done`
- `task.delete`
- `task.start`
- `task.stop`
- `task.annotate`
- `task.denotate`
- `task.append`
- `task.prepend`
- `task.edit`
- `task.recurrence.archived_project`

`task.import` 是批量 audit，没有单一 project 维度，固定不填 `audit_logs.project_id`。如果需要记录 project 信息，payload 可增加 per-project breakdown，例如 `{"projects":{"api":3,"web":2}}`，但不要随意填某一个任务的 project id。

Task 9 完成后运行：

```bash
rg 'withAudit\("task\.' internal/app
```

检查所有 task audit 调用都填充了 `AuditEntry.ProjectID` 或明确传入 nil，并且 payload 包含 before/after project 信息。

- [x] **Step 5：运行测试通过并提交**

```bash
go test ./internal/app -run 'Project.*Task|Task.*Project|Audit.*Project' -count=1
git add internal/app/service.go internal/app/project_query.go internal/app/audit.go internal/app/service_test.go
git commit -m "feat: 任务写路径绑定 project 实体"
```

### Task 10：Import / Export / Recurrence project 语义

**Files:**
- Modify: `internal/app/service.go`
- Modify: `internal/task/json.go`
- Test: `internal/app/service_test.go`
- Test: `internal/task/json_test.go`

- [x] **Step 1：写失败测试**

覆盖：

- import 未注册 project 整批回滚。
- import 已注册 project 成功。
- import archived project 成功仅限“任务已有关联归档 project 的 round-trip”：实现方式固定为同 workspace export 后 import 可更新同一 UUID；新建任务指向 archived project 仍失败。
- export 遇到 `ProjectID != nil` 但 slug 不一致返回 `project_invariant_violation`。
- recurrence parent project archived 时仍创建 child，保留 project，并写 `task.recurrence.archived_project` audit warning。

- [x] **Step 2：实现 Import project 解析**

`importOneLocked`：

- DTO 中 `Project == nil`：不绑定。
- DTO 中 `Project != nil && *Project == ""`：清空。
- 新建任务：project 必须 active。
- 更新已有任务：如果 existing 已绑定同一个 archived project 且 DTO project 等于 slug，允许 round-trip；否则 archived 仍拒绝。

- [x] **Step 3：实现 Export invariant**

`Export()` 返回前逐个 `validateTaskProjectInvariant`。失败返回 `RuntimeError{Code:"project_invariant_violation"}`。

- [x] **Step 4：实现 recurrence audit warning**

`createNextRecurringChild` 检测 parent project archived：

- 仍调用 `CreateRecurringChild`。
- 额外写 audit entry `task.recurrence.archived_project`，`ProjectID` 填 parent project id。
- payload 包含 parent/child/project 信息。

如果当前结构不适合在 `createNextRecurringChild` 内写 audit，新增 `createNextRecurringChildWithAudit` 并只在 service 写路径 / automatic refresh 中使用。

- [x] **Step 5：运行测试通过并提交**

```bash
go test ./internal/app ./internal/task -run 'Import.*Project|Export.*Project|Recurring.*Project|ProjectID' -count=1
git add internal/app/service.go internal/task/json.go internal/app/service_test.go internal/task/json_test.go
git commit -m "feat: 导入导出和循环任务支持 project"
```

---

## Chunk 6：Query、Context、Report 与 Helper

### Task 11：Project 查询解析为 project_id

**Files:**
- Modify: `internal/query/ast.go`
- Modify: `internal/storage/sqlite/query_scope.go`
- Modify: `internal/app/project_query.go`
- Test: `internal/storage/sqlite/query_scope_test.go`
- Test: `internal/app/service_test.go`

- [x] **Step 1：写失败测试**

覆盖：

- `project:<slug>` 在 app 层解析成 `project_id = ?`。
- `project:` 查询无 project 任务。
- `project:missing` 报 `project_not_found`，不静默空集。
- `project:<slug-only-in-other-workspace>` 在当前 workspace 报错。
- context filter `project:api` 执行时解析。

- [x] **Step 2：实现 AST rewrite**

在 app 层新增：

```go
func (s *Service) resolveProjectPredicates(expr query.Expr) (query.Expr, error)
```

规则：

- `AttrProject + OpEqual + Value.Text != ""`：解析 project，改写为内部 project id predicate。
- `AttrProject + OpIsNull` 或 `project:`：保留为 no-project。
- 不支持 contains/before/after project。

实现方式：

- 固定在 `query` 包新增 `AttrProjectID`，storage 编译 `project_id = ?`。
- `AttrProject` 保留给 parser 输入；进入 storage 前应被 app 改写成 `AttrProjectID` 或 `OpIsNull`。
- 在 `internal/query/errors.go` 中定义：

```go
var ErrProjectPredicateUnresolved = errors.New("project predicate must be resolved before SQL compilation")
```

该错误只表示内部不变量失败；app/CLI 层必须转换成 `project_invariant_violation`。

- [x] **Step 3：storage 编译**

`query_scope.go`：

```go
case query.AttrProjectID:
    return compareColumn("project_id", p.Operator, value, nil)
case query.AttrProject:
    if p.Operator == query.OpIsNull { return "project_id IS NULL", nil, nil }
    return "", nil, query.ErrProjectPredicateUnresolved
```

`ErrProjectPredicateUnresolved` 必须是稳定 typed error 或可被 app 层识别的 sentinel error。进入 CLI 时转为 `RuntimeError{Code:"project_invariant_violation"}`，不要把裸错误文本 `project predicate must be resolved before SQL compilation` 泄漏给用户。

- [x] **Step 4：接入 List / RunReport / context**

在 `List` 合并 active context 和 input query 后调用 `resolveProjectPredicates`，再传给 repo。

- [x] **Step 5：运行测试通过并提交**

```bash
go test ./internal/app ./internal/storage/sqlite ./internal/query -run 'Project.*Query|AttrProject|Context.*Project' -count=1
git add internal/query/ast.go internal/app/project_query.go internal/app/service.go internal/storage/sqlite/query_scope.go internal/storage/sqlite/query_scope_test.go internal/app/service_test.go
git commit -m "feat: project 查询使用实体解析"
```

### Task 12：`_projects` 与 `_unique project`

**Files:**
- Modify: `internal/app/service.go`
- Modify: `internal/cli/helper.go`
- Test: `internal/app/service_test.go`
- Test: `tests/integration/cli_test.go`

- [x] **Step 1：写失败测试**

覆盖：

- `_projects` 输出 active project slug，每行一个，无列头。
- `_projects --all` 包含 archived。
- `_projects` 不从任务聚合：无任务 project 也应出现。
- `_unique project` 只从当前查询结果中的任务绑定聚合 project slug，不输出无任务 project。
- `_unique project` 只接受 `ProjectID` 指向有效 project 且 `tasks.project` 与实体 slug 一致的任务；不能回落到损坏字符串。

- [x] **Step 2：更新 app helper**

`Projects(includeArchived bool)` 改为从 `ProjectRepository.List` 返回 slug。CLI 默认 false。

`Unique("project", query)` 保持任务聚合语义：

- 先按当前 workspace、active context 和用户 query 取任务。
- 对每个任务调用 `validateTaskProjectInvariant`。
- 只输出有有效 `ProjectID` 的 project slug。
- 不查询 project 表补全无任务 project；无任务 project 只属于 `_projects` / `project list`。

- [x] **Step 3：CLI 加 flag**

`newProjectsCommand` 增加：

```go
var includeArchived bool
cmd.Flags().BoolVar(&includeArchived, "all", false, "include archived projects")
```

- [x] **Step 4：运行测试通过并提交**

```bash
go test ./internal/app ./tests/integration -run 'Projects|_projects|Unique.*Project' -count=1
go vet ./...
git add internal/app/service.go internal/cli/helper.go internal/app/service_test.go tests/integration/cli_test.go
git commit -m "feat: helper 使用 project 表"
```

---

## Chunk 7：Project CLI

### Task 13：Project command group

**Files:**
- Create: `internal/cli/project.go`
- Modify: `internal/cli/root.go`
- Test: `tests/integration/cli_test.go`
- Test: `internal/cli/root_test.go`

- [x] **Step 1：写黑盒 CLI 测试**

新增 `TestCLIProjectLifecycle`：

```bash
taskg project add ai-agent-platform name:"AI Agent Platform"
taskg project list
taskg project info ai-agent-platform --json
taskg project modify ai-agent-platform description:"Agent MCP platform"
taskg add "Design schema" project:ai-agent-platform
taskg project archive ai-agent-platform
taskg add "Should fail" project:ai-agent-platform   # project_archived
taskg project archive ai-agent-platform             # project_archived
```

新增跨 workspace 测试：

```bash
taskg workspace add partner name:Partner
taskg --workspace local project add api name:API
taskg --workspace partner project info api          # project_not_found
taskg --workspace partner project info <local-id>   # project_workspace_mismatch
```

- [x] **Step 2：实现 CLI**

`internal/cli/project.go`：

- `newProjectCommand(opts)`
- `newProjectListCommand`
- `newProjectAddCommand`
- `newProjectInfoCommand`
- `newProjectModifyCommand`
- `newProjectArchiveCommand`
- `newProjectConfigCommand`

参数解析沿用 `workspace` 命令的 `parseKeyValueArgs` 风格，未知 key 报错。

- [x] **Step 3：JSON 输出**

human 用 table 或固定列；JSON 使用 snake_case map：

```go
func projectViewForJSON(p app.ProjectView) map[string]any
```

- [x] **Step 4：root 注册和 reorder 测试**

`NewRootCommand` 注册 `newProjectCommand(opts)`。补 root 测试确认：

- `taskg project:list` 不支持，正确形式是 `project list`。
- `taskg --workspace x project info api` 和 `taskg project info api --workspace x` 都被 Cobra 接受。
- `taskg project info project` 在 project slug 名为 `project` 时按 project 子命令解析，不被 root target-action reorder 吞掉。M5 保留字不包含 `project`，该 slug 允许存在。
- `project info api --json --workspace x` 与 `--workspace x project info api --json` 输出一致。

- [x] **Step 5：运行测试通过并提交**

```bash
go test ./internal/cli ./tests/integration -run 'Project|WorkspaceEquals|Root' -count=1
git add internal/cli/project.go internal/cli/root.go internal/cli/root_test.go tests/integration/cli_test.go
git commit -m "feat: 添加 project CLI"
```

### Task 14：Project config CLI

**Files:**
- Modify: `internal/cli/project.go`
- Modify: `internal/cli/config.go`
- Test: `tests/integration/cli_test.go`

- [x] **Step 1：写黑盒测试**

覆盖：

- `project config set ai-agent-platform agent.background "..."`
- `project config get ai-agent-platform agent.background`
- `project config list ai-agent-platform`
- `project config unset ai-agent-platform agent.background`
- viewer/member set 失败。
- `config set agent.background "..."` 失败，错误 code/message 指向 `project config set`。
- `config get agent.background` 和 `config unset agent.background` 也失败，统一返回 code `project_config_scope_required`；message 可按动词提示 `project config get/set/unset <project> ...`。
- `config list` 不显示 project config。

- [x] **Step 2：实现 CLI 命令**

`project config` 子命令只接受 `<slug|project-id>` 明确 scope，不支持 `--project` flag。

- [x] **Step 3：更新 config.go scope 错误**

无 scope `config set/get/unset/list` 不处理 project config。`config set agent.background ...` 直接返回 service 的 `project_config_scope_required`。

- [x] **Step 4：运行测试通过并提交**

```bash
go test ./tests/integration -run 'ProjectConfig|ConfigList' -count=1
git add internal/cli/project.go internal/cli/config.go tests/integration/cli_test.go
git commit -m "feat: 添加 project 配置 CLI"
```

---

## Chunk 8：Integration、Docs 与最终验收

### Task 15：端到端安全与迁移集成测试

**Files:**
- Test: `tests/integration/cli_test.go`
- Test: `internal/storage/sqlite/db_test.go`
- Test: `internal/app/service_test.go`

- [x] **Step 1：补集成测试**

覆盖 spec §13 剩余项：

- migration warning 可通过 `config get migration.m5.projects.skipped --json` 读取。
- migration report 非空时，首次打开 DB 的 CLI 命令必须在 stderr 输出一行 warning，提示查看 `migration.m5.projects.skipped`。
- `project:` 无 project 查询。
- context 引用 project 生效。
- import 未注册 project 整批回滚。
- archived project export/import round-trip。
- `audit list --json` 包含 project_id。
- `audit list --project <slug|project-id> --json` 如本轮选择暴露 CLI filter，应只返回该 project 的 audit；若不暴露 CLI filter，至少 repository 层测试 `ProjectID` filter。

实现路径固定为 storage `Store` 暴露只读方法：

```go
func (s *Store) M5ProjectMigrationReport() ([]MigrationWarning, error)
```

CLI runtime 打开 DB 并完成迁移后调用一次；报告为空不输出，报告非空只向 stderr 输出一行 warning，stdout 不受影响。

- [x] **Step 2：补 invariant 坏数据测试**

手工 SQL 写入：

- `tasks.project_id` 指向其它 workspace。
- `tasks.project` 与 `projects.slug` 不一致。

期望：

- 读写路径返回 `project_invariant_violation` 或 FK error。
- export 返回 `project_invariant_violation`。

- [x] **Step 3：运行集成测试通过**

```bash
go test ./tests/integration -run 'Project|Import|Audit|Context' -count=1
go test ./internal/app ./internal/storage/sqlite -run 'Project|M5|Invariant|Migration' -count=1
```

- [x] **Step 4：提交**

```bash
git add tests/integration/cli_test.go internal/storage/sqlite/db_test.go internal/app/service_test.go
git commit -m "test: 补齐 M5 project 集成覆盖"
```

### Task 16：README / ROADMAP / requirements 更新

**Files:**
- Modify: `README.md`
- Modify: `ROADMAP.md`
- Modify: `docs/requirements.md`
- Modify: `docs/superpowers/specs/2026-05-30-taskg-m5-design.md`

- [x] **Step 1：README 增加 M5 用法**

新增内容必须覆盖：

- `project add/list/info/modify/archive`。
- `project config get/set/unset/list`。
- project 必须先注册。
- `project_id` 是 API/MCP/token 推荐身份。
- `_projects --all`。
- 同名 project 可跨 workspace 复用，CLI 必须配合 effective workspace 理解。

- [x] **Step 2：ROADMAP 标 M5 已完成**

把 M5 状态改为“已完成”，并注明 M6/M7 基于 `project_id` scope。

- [x] **Step 3：requirements 与 spec 微调**

如果实现中任何已固定细节和 spec 有差异，优先改实现；只有发现 spec 本身不合理才更新文档并在提交说明中解释。

- [x] **Step 4：提交**

```bash
git add README.md ROADMAP.md docs/requirements.md docs/superpowers/specs/2026-05-30-taskg-m5-design.md
git commit -m "docs: 更新 M5 project 文档"
```

### Task 17：最终验证

**Files:**
- No source change unless validation finds a bug.

- [x] **Step 1：格式与静态检查**

```bash
test -z "$(gofmt -l internal cmd tests)"
go vet ./...
git diff --check
```

Expected: PASS。

- [x] **Step 2：常规测试**

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/taskg
```

Expected: PASS。

- [x] **Step 3：race 验证**

```bash
go test -race ./...
```

Expected: PASS。若本地耗时或环境限制导致无法常规执行，必须在交付说明中明确记录原因和已执行的替代测试。

- [x] **Step 4：手动冒烟**

```bash
tmp=$(mktemp -d)
go build -o "$tmp/taskg" ./cmd/taskg
"$tmp/taskg" --data-dir "$tmp" project add api name:"API"
"$tmp/taskg" --data-dir "$tmp" add "Design endpoint" project:api
"$tmp/taskg" --data-dir "$tmp" project list
"$tmp/taskg" --data-dir "$tmp" _projects
"$tmp/taskg" --data-dir "$tmp" project config set api agent.background "Owns API work"
"$tmp/taskg" --data-dir "$tmp" project config get api agent.background
"$tmp/taskg" --data-dir "$tmp" project archive api
"$tmp/taskg" --data-dir "$tmp" add "Should fail" project:api
```

Expected:

- 前 6 条成功。
- archive 后 add 返回非零，错误包含 `project "api" is archived`。

- [x] **Step 5：最终提交或合并前状态**

```bash
git status --short
```

Expected: clean working tree。
