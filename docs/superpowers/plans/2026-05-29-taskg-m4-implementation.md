# taskg M4 实施计划

> **给 agentic workers 的要求：** 必须使用 `superpowers:subagent-driven-development`（如果可用）或 `superpowers:executing-plans` 执行本计划。所有步骤使用 checkbox（`- [ ]`）语法跟踪。

**目标：** 实现 M4：本地多用户、多 workspace 运行时上下文、membership 权限、审计日志，以及所有 CLI 路径的 workspace 隔离；不引入 HTTP、远程 CLI、PAT/JWT、MCP 或同步。

**架构：** 在 `internal/storage/sqlite` 中增加 user/workspace/member/audit 持久化；在 `internal/app` 中按每次命令解析 runtime context，并把 service 绑定到 `ActorUserID + WorkspaceID + Role`。权限判断和审计编排放在 app 层，SQL/GORM repo 只负责持久化与 workspace-scoped 数据访问。CLI 命令保持薄层，只做参数解析和渲染。

**技术栈：** Go 1.22、Cobra、GORM、`github.com/glebarez/sqlite`（保持 `CGO_ENABLED=0`）、现有 `internal/query`、`internal/task`、`internal/uda`、`internal/render` 和黑盒 CLI 集成测试。

---

## 范围锁定

严格按 [M4 spec](/Users/mac/code/projects/dajee/task/docs/superpowers/specs/2026-05-29-taskg-m4-design.md) 实现：

- users、workspaces、memberships、audit logs。
- active user、active workspace、active context 都按 user/workspace 维度隔离。
- 全局 `--workspace` 临时覆盖。
- `user`、`workspace`、`member`、`audit` CLI 命令组。
- viewer/member/admin/owner 的真实权限校验。
- spec 中列出的写操作必须在同一个 store-level transaction 中同时写业务表和 audit log。

明确不做：

- HTTP server。
- 远程 CLI。
- token 命令。
- 登录/鉴权。
- member delete。
- user delete。
- workspace hard delete。
- op-log sync。
- MCP。
- Hook。
- 跨 workspace/global context 查询。

## 文件结构

新增文件：

- `internal/app/runtime.go`
  runtime context 类型、actor/workspace 解析、active meta key helper。
- `internal/app/permission.go`
  role 常量、permission 常量、`Require(permission)`。
- `internal/app/workspace.go`
  user/workspace/member app 方法。
- `internal/app/audit.go`
  audit app 方法和写路径审计 helper。
- `internal/storage/sqlite/user_repo.go`
  user CRUD 与 lookup。
- `internal/storage/sqlite/workspace_repo.go`
  workspace CRUD、归档、可见 workspace 查询。
- `internal/storage/sqlite/member_repo.go`
  membership CRUD、role 更新、owner 计数、membership lookup。
- `internal/storage/sqlite/audit_repo.go`
  audit append/list。
- `internal/cli/user.go`
  `taskg user ...` 命令。
- `internal/cli/workspace.go`
  `taskg workspace ...` 命令。
- `internal/cli/member.go`
  `taskg member ...` 命令。
- `internal/cli/audit.go`
  `taskg audit list`。

修改文件：

- `internal/storage/sqlite/models.go`
  新增/扩展 GORM models：users、workspaces、memberships、audit_logs。
- `internal/storage/sqlite/db.go`
  AutoMigrate 新表，初始化 local user/workspace/membership，迁移 active context meta。
- `internal/storage/sqlite/db_test.go`
  migration/idempotency 测试。
- `internal/app/service.go`
  service 绑定 runtime context，移除业务路径中的 `Store.LocalWorkspace()` 依赖，把写方法改为 permission + audit transaction 模式。
- `internal/app/context.go`
  把 `context.active` 替换为 `active_context.<user_id>.<workspace_id>`。
- `internal/app/uda.go`
  UDA schema 写操作增加权限与审计。
- `internal/app/service_test.go`
  runtime context、permission、audit、isolation 单元测试。
- `internal/cli/root.go`
  增加 `--workspace`，停止持久读写旧 `context.active`，保留 `rc.context=...` 的 in-memory override 行为，注册新命令组。
- `internal/cli/config.go`
  展示新的 active key，停止读写旧 `context.active`。
- `internal/cli/context.go`
  区分 `context use/none` 与 `define/delete` 的权限。
- `internal/cli/helper.go`
  确认 helper 通过 root opts 继承 `--workspace`。
- `tests/integration/cli_test.go`
  M4 黑盒 CLI 流程测试。
- `README.md`、`ROADMAP.md`
  实现完成后同步文档。

---

## Chunk 1：Storage Schema、迁移与 Repository

### Task 1：扩展 SQLite Models 与迁移

**Files:**
- Modify: `internal/storage/sqlite/models.go`
- Modify: `internal/storage/sqlite/db.go`
- Test: `internal/storage/sqlite/db_test.go`

- [x] **Step 1：写失败的迁移测试**

在 `internal/storage/sqlite/db_test.go` 中新增：

```go
func TestOpenInitializesLocalUserWorkspaceAndMembership(t *testing.T) {
    store, err := Open(filepath.Join(t.TempDir(), "taskg.db"))
    if err != nil { t.Fatal(err) }
    t.Cleanup(func() { _ = store.Close() })

    user, err := NewUserRepository(store.DB()).GetByName("local")
    if err != nil { t.Fatal(err) }
    if user.Email != nil { t.Fatalf("local email = %q, want nil", *user.Email) }

    ws, err := store.LocalWorkspace()
    if err != nil { t.Fatal(err) }
    if user.DefaultWorkspaceID == nil || *user.DefaultWorkspaceID != ws.ID {
        t.Fatalf("default workspace = %#v, want %s", user.DefaultWorkspaceID, ws.ID)
    }

    member, err := NewMemberRepository(store.DB()).Get(user.ID, ws.ID)
    if err != nil { t.Fatal(err) }
    if member.Role != "owner" { t.Fatalf("role = %q, want owner", member.Role) }
}

func TestOpenMigratesContextActiveMeta(t *testing.T) {
    dbPath := filepath.Join(t.TempDir(), "taskg.db")
    store, err := Open(dbPath)
    if err != nil { t.Fatal(err) }
    if err := store.SetMeta("context.active", "work"); err != nil { t.Fatal(err) }
    _ = store.Close()

    reopened, err := Open(dbPath)
    if err != nil { t.Fatal(err) }
    defer reopened.Close()

    user, _ := NewUserRepository(reopened.DB()).GetByName("local")
    ws, _ := reopened.LocalWorkspace()
    if _, ok, _ := reopened.GetMeta("context.active"); ok {
        t.Fatal("old context.active key still exists")
    }
    got, ok, err := reopened.GetMeta("active_context." + user.ID + "." + ws.ID)
    if err != nil || !ok || got != "work" {
        t.Fatalf("migrated active context = %q, %v, %v", got, ok, err)
    }
}
```

- [x] **Step 2：运行测试确认失败**

Run:

```bash
go test ./internal/storage/sqlite -run 'TestOpenInitializesLocalUserWorkspaceAndMembership|TestOpenMigratesContextActiveMeta' -count=1
```

Expected: FAIL，因为 repository/model 还不存在，迁移也还没有创建 users/memberships。

- [x] **Step 3：更新 GORM models**

在 `models.go` 中新增/修改：

```go
type User struct {
    ID                 string  `gorm:"primaryKey"`
    Name               string  `gorm:"not null;uniqueIndex"`
    Email              *string `gorm:"uniqueIndex"`
    DefaultWorkspaceID *string
    CreatedAt          int64 `gorm:"not null"`
    ModifiedAt         int64 `gorm:"not null"`
}

type Workspace struct {
    ID              string `gorm:"primaryKey"`
    Slug            string `gorm:"not null;uniqueIndex"`
    Name            string `gorm:"not null"`
    CreatedByUserID *string
    Description     string
    Visibility      string `gorm:"not null;default:'private'"`
    SettingsJSON    string `gorm:"not null;default:'{}'"`
    ArchivedAt      *int64
    CreatedAt       int64 `gorm:"not null"`
    ModifiedAt      int64 `gorm:"not null"`
}

type Membership struct {
    UserID      string `gorm:"primaryKey;not null"`
    WorkspaceID string `gorm:"primaryKey;not null;index"`
    Role        string `gorm:"not null;index"`
    JoinedAt    int64  `gorm:"not null"`
    ModifiedAt  int64  `gorm:"not null"`
}

type AuditLog struct {
    ID          int64 `gorm:"primaryKey;autoIncrement"`
    ActorUserID *string `gorm:"index"`
    WorkspaceID *string `gorm:"index;index:idx_audit_ws_time,priority:1"`
    Action      string  `gorm:"not null;index"`
    TargetType  string
    TargetID    string
    PayloadJSON string
    CreatedAt   int64 `gorm:"not null;index;index:idx_audit_ws_time,priority:2,sort:desc"`
}
```

保留现有 `Task`、`Context`、`UDADefinition`、`TaskUDAValue` 的语义。除非 GORM 关系确实需要，不要扩大任务模型边界。

- [x] **Step 4：实现迁移 helper**

在 `db.go` 中：

- AutoMigrate `User`、更新后的 `Workspace`、`Membership`、`AuditLog`。
- 用 `ensureLocalIdentity()` 替代 `ensureLocalWorkspace()`：
  - 创建 email 为 nil 的 local user。
  - 创建或更新 local workspace：`Slug: "local"`、`Visibility: "private"`、`SettingsJSON: "{}"`、`CreatedByUserID: &localUser.ID`。
  - 对历史 workspace，如果 `modified_at = 0`，设置 `ModifiedAt = CreatedAt`。
  - 设置 local user 的 `DefaultWorkspaceID`。
  - 创建 owner membership。
  - 把旧 `context.active` 迁移到 `active_context.<local_user_id>.<local_workspace_id>`，并删除旧 key。
- `LocalWorkspace()` 可以作为 storage migration/tests 的兼容 helper 保留，但 app 业务路径后续必须停止调用它。

- [x] **Step 5：运行 storage 迁移测试**

Run:

```bash
go test ./internal/storage/sqlite -run 'TestOpenInitializesLocalUserWorkspaceAndMembership|TestOpenMigratesContextActiveMeta' -count=1
```

Expected: PASS。

- [x] **Step 6：提交**

```bash
git add internal/storage/sqlite/models.go internal/storage/sqlite/db.go internal/storage/sqlite/db_test.go
git commit -m "feat: 初始化 M4 本地身份模型"
```

### Task 2：新增 User、Workspace、Membership、Audit Repositories

**Files:**
- Create: `internal/storage/sqlite/user_repo.go`
- Create: `internal/storage/sqlite/workspace_repo.go`
- Create: `internal/storage/sqlite/member_repo.go`
- Create: `internal/storage/sqlite/audit_repo.go`
- Test: `internal/storage/sqlite/db_test.go` 或新增 `internal/storage/sqlite/identity_repo_test.go`

- [x] **Step 1：写失败的 repository 测试**

覆盖：

- 按 name/email/UUID 查找 user。
- workspace slug 校验和全局唯一。
- visible workspaces 默认排除 archived。
- membership role 更新保留 `JoinedAt`，更新 `ModifiedAt`。
- `CountOwners(workspaceID)` 保护最后一个 owner。
- audit list 按最新优先，支持 limit。

示例：

```go
func TestAuditRepositoryListsNewestFirst(t *testing.T) {
    store := openTestStore(t)
    repo := NewAuditRepository(store.DB())
    ws := mustLocalWorkspace(t, store)
    actor := mustLocalUser(t, store)
    for i, action := range []string{"a", "b", "c"} {
        if err := repo.Append(AuditLogEntry{ActorUserID: &actor.ID, WorkspaceID: &ws.ID, Action: action, CreatedAt: int64(100+i)}); err != nil {
            t.Fatal(err)
        }
    }
    rows, err := repo.List(AuditListOptions{WorkspaceID: &ws.ID, Limit: 2})
    if err != nil { t.Fatal(err) }
    if got := []string{rows[0].Action, rows[1].Action}; !reflect.DeepEqual(got, []string{"c", "b"}) {
        t.Fatalf("actions = %#v", got)
    }
}
```

- [x] **Step 2：运行测试确认失败**

Run:

```bash
go test ./internal/storage/sqlite -run 'UserRepository|WorkspaceRepository|MemberRepository|AuditRepository' -count=1
```

Expected: FAIL，因为 repo 还不存在。

- [x] **Step 3：实现 repository DTO 和方法**

先使用 storage-level DTO，不要把 user/workspace/member 概念塞进 `internal/task`。

最小方法：

```go
type UserRepository struct { db *gorm.DB }
func (r *UserRepository) Create(User) (User, error)
func (r *UserRepository) GetByID(id string) (User, error)
func (r *UserRepository) GetByName(name string) (User, error)
func (r *UserRepository) GetByEmail(email string) (User, error)
func (r *UserRepository) List() ([]User, error)
func (r *UserRepository) UpdateDefaultWorkspace(userID, workspaceID string, modifiedAt int64) error

type WorkspaceRepository struct { db *gorm.DB }
func (r *WorkspaceRepository) Create(Workspace) (Workspace, error)
func (r *WorkspaceRepository) GetByID(id string) (Workspace, error)
func (r *WorkspaceRepository) GetBySlug(slug string) (Workspace, error)
func (r *WorkspaceRepository) ListVisibleForUser(userID string, includeArchived bool) ([]WorkspaceWithRole, error)
func (r *WorkspaceRepository) UpdateMetadata(workspaceID string, input WorkspaceMetadataUpdate, modifiedAt int64) error
func (r *WorkspaceRepository) Archive(workspaceID string, archivedAt int64) error

type MemberRepository struct { db *gorm.DB }
func (r *MemberRepository) Get(userID, workspaceID string) (Membership, error)
func (r *MemberRepository) Upsert(Membership) error
func (r *MemberRepository) UpdateRole(userID, workspaceID, role string, modifiedAt int64) error
func (r *MemberRepository) List(workspaceID string) ([]MemberWithUser, error)
func (r *MemberRepository) CountOwners(workspaceID string) (int64, error)
func (r *MemberRepository) OtherUnarchivedWorkspaces(userID, excludeWorkspaceID string) ([]Workspace, error)

type AuditRepository struct { db *gorm.DB }
func (r *AuditRepository) Append(AuditLogEntry) error
func (r *AuditRepository) List(AuditListOptions) ([]AuditLogEntry, error)
```

统一使用 `sqlite.ErrNotFound` 表示找不到。

- [x] **Step 4：运行 repository 测试**

Run:

```bash
go test ./internal/storage/sqlite -run 'UserRepository|WorkspaceRepository|MemberRepository|AuditRepository' -count=1
```

Expected: PASS。

- [x] **Step 5：Chunk 1 构建检查**

Run:

```bash
CGO_ENABLED=0 go build ./cmd/taskg
```

Expected: PASS。Chunk 1 引入新 schema/repo 后必须保持中间状态可构建，避免把编译错误拖到后续 runtime 接线阶段。

- [x] **Step 6：提交**

```bash
git add internal/storage/sqlite/*repo.go internal/storage/sqlite/*test.go
git commit -m "feat: 增加 M4 身份仓储"
```

---

## Chunk 2：Runtime Context、权限与审计事务

### Chunk 2+ 架构决策

所有写路径必须遵守这个形状：

```go
func (s *Service) Add(input AddInput) (task.Task, error) {
    if err := s.Require(PermissionTaskWrite); err != nil {
        return task.Task{}, err
    }
    var out task.Task
    err := s.withAudit("task.add", func(tx *Service) (AuditEntry, error) {
        created, err := tx.addLocked(input)
        if err != nil {
            return AuditEntry{}, err
        }
        out = created
        return AuditEntry{
            TargetType: "task",
            TargetID: created.UUID,
            Payload: map[string]any{"description": created.Description},
        }, nil
    })
    return out, err
}

func (s *Service) addLocked(input AddInput) (task.Task, error) {
    // 旧 Add 方法主体；只能使用当前 service 实例上的 repo
}
```

规则：

- public 写方法先做一次权限检查，再调用 `withAudit`。
- `withAudit` 打开 store-level transaction，并把 tx-bound `*Service` 传给闭包。
- 闭包必须调用 tx-bound service 上的 `xxxLocked` 方法，不能调用外层 service 的 public 方法。
- `xxxLocked` 方法不调用 `Require`，也不打开新的 app-level audit transaction。
- `withAudit` 从闭包接收 `AuditEntry`，这样 task UUID 这类运行时生成的 target ID 也能被审计。
- 需要写多条审计记录的业务路径使用 `withAuditEntries`，闭包返回 `[]AuditEntry`，helper 在同一个 store-level transaction 内逐条 append。禁止通过连续调用两次 `withAudit` 实现一个业务操作的多审计。
- 本计划统一使用局部变量 capture 从 audit 闭包带出业务返回值；不新增泛型 helper。
- 自动状态维护（`refreshAutomaticStateLocked`、recurring child creation）属于内部维护，绕过用户权限检查，但必须保持 workspace-scoped。除非 spec 明确列出，否则不写独立 public audit action。
- M4 不读取 TOML `context.active`。如果用户只依赖 TOML active context，升级后需要手动运行一次 `taskg context use <name>`。

### Task 3：引入 Runtime Context，并按命令绑定 Service

**Files:**
- Create: `internal/app/runtime.go`
- Modify: `internal/app/service.go`
- Modify: `internal/cli/root.go`
- Modify: `internal/cli/config.go`
- Test: `internal/app/service_test.go`
- Test: `internal/cli/root_test.go`

- [x] **Step 1：写失败的 runtime 测试**

新增 app 测试：

```go
func TestNewServiceResolvesLocalRuntimeContext(t *testing.T) {
    svc, closeFn := newTestService(t, 100)
    defer closeFn()
    if svc.Runtime().ActorUserID == "" || svc.Runtime().WorkspaceID == "" {
        t.Fatalf("runtime = %#v", svc.Runtime())
    }
    if svc.Runtime().Role != RoleOwner {
        t.Fatalf("role = %q, want owner", svc.Runtime().Role)
    }
}

func TestNewServiceWorkspaceOverride(t *testing.T) {
    // 创建 workspace "work"；用 WorkspaceRef: "work" 调 NewService；
    // 断言 svc.Runtime().WorkspaceSlug == "work"
}
```

新增 CLI root 测试：`--workspace work` 会作为 string flag 被解析。

- [x] **Step 2：运行测试确认失败**

Run:

```bash
go test ./internal/app ./internal/cli -run 'Runtime|WorkspaceOverride|ParsesWorkspace' -count=1
```

Expected: FAIL，因为 runtime context 和 `--workspace` 尚不存在。

- [x] **Step 3：添加 app runtime 类型**

在 `internal/app/runtime.go` 中：

```go
type Role string
const (
    RoleOwner Role = "owner"
    RoleAdmin Role = "admin"
    RoleMember Role = "member"
    RoleViewer Role = "viewer"
)

type RuntimeContext struct {
    ActorUserID   string
    ActorName     string
    WorkspaceID   string
    WorkspaceSlug string
    Role          Role
}

type ServiceOptions struct {
    Store         *sqlite.Store
    Clock         Clock
    NoContext     bool
    RuntimeConfig map[string]string
    RuntimeOverrides map[string]string
    ActorRef      string
    WorkspaceRef  string
}
```

可以把 `ServiceOptions` 从 `service.go` 移到 `runtime.go`，也可以原地扩展。零值行为必须兼容 M3：不传 actor/workspace ref 时，默认 local user + local/default workspace。现有 `NewService(ServiceOptions{Store: store, Clock: ...})` 调用点不应被迫修改。

- [x] **Step 4：实现 runtime 解析**

解析顺序：

1. active user meta `active_user_id`；不存在则 fallback `local`。
2. `WorkspaceRef` 来自 `--workspace`；否则 `active_workspace.<user_id>`；否则 `users.default_workspace_id`。
3. 拒绝 archived workspace。
4. 拒绝缺少 membership 的 actor。
5. 读取 role 并绑定到 service。

新增 helper：

```go
type RuntimeError struct { Code, Message string }
func (e RuntimeError) Error() string { return e.Message }

func activeWorkspaceMetaKey(userID string) string
func activeContextMetaKey(userID, workspaceID string) string
func (s *Service) Runtime() RuntimeContext
```

`ResolveRuntimeContext` 必须返回带 code 的错误，至少覆盖：

- workspace 不存在：`workspace_not_found`
- workspace 已归档：`workspace_archived`
- actor 不是该 workspace member：`membership_not_found`

这些 code 是 Task 9 CLI 错误语义测试的来源，不要只返回普通字符串错误。

- [x] **Step 5：更新 service 构造**

在 `NewService` 中初始化新 repo：

```go
userRepo := sqlite.NewUserRepository(opts.Store.DB())
workspaceRepo := sqlite.NewWorkspaceRepository(opts.Store.DB())
memberRepo := sqlite.NewMemberRepository(opts.Store.DB())
auditRepo := sqlite.NewAuditRepository(opts.Store.DB())
rt, err := ResolveRuntimeContext(opts.Store, userRepo, workspaceRepo, memberRepo, opts.ActorRef, opts.WorkspaceRef)
```

短期内可以继续设置 `s.workspaceID = rt.WorkspaceID`，降低改动面；新代码应优先使用 `s.runtime.WorkspaceID`。

`withStore` 必须保留同一个 runtime context，只重建 tx-bound repo，不得调用 `LocalWorkspace()`。

硬要求：Step 5 完成后，下列命令必须没有匹配：

```bash
rg "LocalWorkspace" internal/app
```

`Store.LocalWorkspace()` 可以留在 `internal/storage/sqlite` 和 storage tests 中作为迁移兼容 helper，但 app 业务路径不能调用它。

- [x] **Step 6：添加 root `--workspace`**

在 `Options` 中增加：

```go
Workspace string
```

root persistent flag：

```go
cmd.PersistentFlags().StringVar(&opts.Workspace, "workspace", opts.Workspace, "workspace slug or UUID")
```

在 `splitFlagsRcAndPositional` 中把 `--workspace` 加入 `stringFlags`。

在 `buildServiceFromOpts` 中传入 `WorkspaceRef: opts.Workspace`。

- [x] **Step 7：更新 config/show 路径**

`runtimeFromResolvedConfig` 当前会构造 service 读取 UDA config。这里也要传入 workspace override。

`show` 不再输出旧 `context.active`，改为：

- `active.user`
- `active.workspace`
- `active.context`

可以继续输出 `date.format`、`color`、`json`、`database.path`。

- [x] **Step 8：运行 runtime 测试**

Run:

```bash
go test ./internal/app ./internal/cli -run 'Runtime|WorkspaceOverride|ParsesWorkspace|Show' -count=1
```

Expected: PASS。

- [ ] **Step 9：提交**

```bash
git add internal/app/runtime.go internal/app/service.go internal/cli/root.go internal/cli/config.go internal/app/service_test.go internal/cli/root_test.go
git commit -m "feat: 解析 M4 运行时上下文"
```

### Task 4：增加权限检查

**Files:**
- Create: `internal/app/permission.go`
- Modify: `internal/app/service.go`
- Modify: `internal/app/context.go`
- Modify: `internal/app/uda.go`
- Test: `internal/app/service_test.go`

- [x] **Step 1：写失败的权限测试**

覆盖：

```go
func TestViewerCannotModifyTasks(t *testing.T) { /* owner 创建任务；切到 viewer runtime；Modify 返回 permission_denied */ }
func TestViewerCanUseOwnContextButCannotDefineContext(t *testing.T) { /* context 已存在时 UseContext 可用，DefineContext 被拒绝 */ }
func TestMemberCannotManageMembersOrWorkspaceMetadata(t *testing.T) { /* app 方法返回 denied */ }
func TestAdminCannotArchiveWorkspace(t *testing.T) { /* denied */ }
```

- [x] **Step 2：运行测试确认失败**

Run:

```bash
go test ./internal/app -run 'Viewer|MemberCannot|AdminCannot|Permission' -count=1
```

Expected: FAIL，因为权限尚未实现。

- [x] **Step 3：实现权限原语**

在 `permission.go` 中：

```go
type Permission string
const (
    PermissionTaskWrite Permission = "task.write"
    PermissionTaskRead Permission = "task.read"
    PermissionContextUse Permission = "context.use"
    PermissionContextManage Permission = "context.manage"
    PermissionUDAManage Permission = "uda.manage"
    PermissionWorkspaceModify Permission = "workspace.modify"
    PermissionWorkspaceArchive Permission = "workspace.archive"
    PermissionMemberManage Permission = "member.manage"
    PermissionMemberManageOwner Permission = "member.manage.owner"
    PermissionAuditRead Permission = "audit.read"
)

type PermissionError struct { Code, Message string }
func (e PermissionError) Error() string { return e.Message }

func (s *Service) Require(p Permission) error
```

权限矩阵：

- viewer：read/export/context use/member list。
- member：task write/import/context define/delete/use。
- admin：member manage（不含 owner 变更）、UDA schema、workspace metadata、audit read。
- owner：全部。

`PermissionMemberManageOwner` 专门用于提升其他用户为 owner 或降级 owner。admin 不能通过这个检查。

- [x] **Step 4：保护 app 方法**

给 public 方法加 `Require`：

- task 写方法：`Add`、`Modify`、`Done`、`Delete`、`Start`、`Stop`、`Annotate`、`Denotate`、`AppendDescription`、`PrependDescription`、`ReplaceEditableTask`、`Import`。
- context：`UseContext`、`ContextNone` 需要 `context.use`；`DefineContext`、`ContextDelete` 需要 `context.manage`。
- UDA schema：`DefineUDA`、`DeleteUDA`、`setUDAConfig`、`unsetUDAConfig` 修改 schema 时需要 `uda.manage`。
- Export/List/Info/Reports/helpers 对 viewer 可读。

不要在 `xxxLocked` 方法里放权限检查；这些方法是 public 方法授权后的内部 transaction body。

- [x] **Step 5：运行权限测试**

Run:

```bash
go test ./internal/app -run 'Viewer|MemberCannot|AdminCannot|Permission' -count=1
```

Expected: PASS。

- [ ] **Step 6：提交**

```bash
git add internal/app/permission.go internal/app/service.go internal/app/context.go internal/app/uda.go internal/app/service_test.go
git commit -m "feat: 增加 M4 权限边界"
```

### Task 5：在同一 store-level transaction 中写 Audit

**Files:**
- Create: `internal/app/audit.go`
- Modify: `internal/app/service.go`
- Modify: `internal/app/context.go`
- Modify: `internal/app/uda.go`
- Test: `internal/app/service_test.go`

- [ ] **Step 1：写失败的 audit 测试**

示例：

```go
func TestTaskWriteCreatesAuditInSameTransaction(t *testing.T) {
    svc := newOwnerService(t)
    created, err := svc.Add(AddInput{Description: "audit me"})
    if err != nil { t.Fatal(err) }
    logs, err := svc.ListAudit(AuditListInput{Limit: 10})
    if err != nil { t.Fatal(err) }
    if logs[0].Action != "task.add" || logs[0].TargetID != created.UUID {
        t.Fatalf("logs = %#v", logs)
    }
}
```

还必须有 rollback 测试。推荐做法：

- 给 audit append 增加 test-only hook 或小接口。
- 让 audit append 在业务写成功后返回错误。
- 断言 task/config/member/workspace 的业务写没有持久化。

不要把“业务写和 audit 同事务”留成未测试假设。如果引入 fake repo interface 太重，可以用 transaction-level test helper 插入非法 audit row，并确认业务写回滚。

- [ ] **Step 2：运行测试确认失败**

Run:

```bash
go test ./internal/app -run 'Audit' -count=1
```

Expected: FAIL，因为 audit 方法还不存在。

- [ ] **Step 3：实现 audit helper**

在 `audit.go` 中：

```go
type AuditListInput struct {
    WorkspaceRef string
    Limit int
}

type AuditEntry struct {
    Action string
    TargetType string
    TargetID string
    Payload map[string]any
}

func (s *Service) withAudit(action string, fn func(*Service) (AuditEntry, error)) error {
    return s.store.Transaction(func(txStore *sqlite.Store) error {
        txSvc, err := s.withStore(txStore)
        if err != nil { return err }
        entry, err := fn(txSvc)
        if err != nil { return err }
        return txSvc.auditRepo.Append(sqlite.AuditLogEntry{
            ActorUserID: &txSvc.runtime.ActorUserID,
            WorkspaceID: &txSvc.runtime.WorkspaceID,
            Action: action,
            TargetType: entry.TargetType,
            TargetID: entry.TargetID,
            PayloadJSON: marshalAuditPayload(entry.Payload),
            CreatedAt: txSvc.clock.Unix(),
        })
    })
}
```

同时实现多审计 helper：

```go
func (s *Service) withAuditEntries(fn func(*Service) ([]AuditEntry, error)) error {
    return s.store.Transaction(func(txStore *sqlite.Store) error {
        txSvc, err := s.withStore(txStore)
        if err != nil { return err }
        entries, err := fn(txSvc)
        if err != nil { return err }
        for _, entry := range entries {
            if err := txSvc.auditRepo.Append(sqlite.AuditLogEntry{
                ActorUserID: &txSvc.runtime.ActorUserID,
                WorkspaceID: &txSvc.runtime.WorkspaceID,
                Action: entry.Action,
                TargetType: entry.TargetType,
                TargetID: entry.TargetID,
                PayloadJSON: marshalAuditPayload(entry.Payload),
                CreatedAt: txSvc.clock.Unix(),
            }); err != nil {
                return err
            }
        }
        return nil
    })
}
```

注意：

- 闭包必须调用 tx-bound 的 `xxxLocked` 方法，不能调用外层 service 的 public 方法。
- 闭包在拿到运行时生成的 ID 后返回 `AuditEntry`。
- 如果 `TaskRepository.Update` 内部开启嵌套 transaction，GORM savepoint 可以接受；但闭包里不能使用外层 `s` 的非 tx-bound repo。
- 如果需要从闭包返回业务值，使用局部变量 capture。不要新增泛型 helper。
- 单条审计的 `withAudit(action, fn)` 可以作为 `withAuditEntries` 的薄包装，包装时给 `AuditEntry.Action` 补上 action。
- 需要多条审计的业务路径必须调用 `withAuditEntries` 一次完成，不能连续调用两次 `withAudit`。

- [ ] **Step 4：包裹写路径**

使用 spec 中的 action：

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
- `task.import`
- `context.define`
- `context.delete`
- `context.use`
- `context.none`
- `uda.schema.set`
- `uda.schema.delete`

不要审计 `Export`。

每个写方法：

- 把旧方法主体移动到 `xxxLocked`。
- public 方法先 `Require`，再 `withAudit`，闭包里调用 `tx.xxxLocked`。
- `xxxLocked` 只能使用 receiver 上的 repo。

- [ ] **Step 5：实现 `ListAudit`**

需要 `audit.read` 权限，并按当前 workspace 调 audit repo。CLI 默认 limit 是 50；app 层也应把 `<=0` 规整为 50。

- [ ] **Step 6：运行 audit 和 app 测试**

Run:

```bash
go test ./internal/app -run 'Audit|Add|Modify|Context|UDA' -count=1
```

Expected: PASS。

- [ ] **Step 7：提交**

```bash
git add internal/app/audit.go internal/app/service.go internal/app/context.go internal/app/uda.go internal/app/service_test.go
git commit -m "feat: 审计 M4 写操作"
```

---

## Chunk 3：Active Context 与 User/Workspace/Member App APIs

### Task 6：把 Active Context 迁移到 User+Workspace 维度

**Files:**
- Modify: `internal/app/context.go`
- Modify: `internal/cli/root.go`
- Modify: `internal/cli/config.go`
- Test: `internal/app/service_test.go`
- Test: `tests/integration/cli_test.go`

- [ ] **Step 1：写失败的 active context 隔离测试**

用 app-level 测试覆盖 active context 隔离；CLI 旧 key 兼容行为放到 Step 5/Step 6 的 CLI 测试里覆盖：

1. Owner define context `work`。
2. 添加 Bob，并把 Bob 加到同一个 workspace。
3. Alice 执行 `context use work`。
4. Bob 的 `context show` 为空。
5. 如果 context 已存在，Bob 作为 viewer 可以执行 `context use work`。

- [ ] **Step 2：运行测试确认失败**

Run:

```bash
go test ./internal/app ./tests/integration -run 'ActiveContext|ContextIsolation' -count=1
```

Expected: FAIL，因为当前 active context 是单一 `context.active` key。

- [ ] **Step 3：替换 `activeContextMetaKey` 常量**

移除用于写入的全局 `const activeContextMetaKey = "context.active"`。

改为：

```go
func (s *Service) activeContextMetaKey() string {
    return activeContextMetaKey(s.runtime.ActorUserID, s.runtime.WorkspaceID)
}
```

`activeContextName` 顺序：

1. `activeContextOverride`。
2. `runtimeOverrides["context.active"]`，只用于 `rc.context=none` / `rc.context:<value>` 这种本次运行覆盖，不持久化。这个 key 只是 in-memory override key，为兼容现有 root parser 保留；它不是 SQLite meta key。
3. Store meta `active_context.<user_id>.<workspace_id>`。

M4 不读取 TOML `context.active`。如果用户依赖 TOML `context.active`，升级后需要运行一次 `taskg context use <name>`。这样避免重新引入机器级共享 active context。

- [ ] **Step 4：更新 RC 处理**

在 root：

- 从 public config allowlist 中移除 `"context.active"`。
- 保留 `rc.context=none`、`rc.context:`、`rc.context=`，映射到本次运行的 no-context/override 行为，内部仍可落到 `RuntimeOverrides["context.active"]`。
- 拒绝 `rc.context.active=...`。

- [ ] **Step 5：更新 show/config 行为**

`config set context.active ...` 和 `config unset context.active` 返回 “managed by context commands” 或 “unsupported legacy key”。

`show` 显示 `active.context=<name>` 或空值，不显示 `context.active`。

`_get rc.context.active`、`_show context.active` 和 `config get context.active` 应返回清晰的 unsupported legacy key 错误。不要在旧 key 下静默返回 scoped active context，也不要静默接受 `config set context.active ...`。

清理检查：

```bash
rg "context\\.active" internal/cli internal/app tests/integration
```

Task 6 之后，剩余匹配必须是迁移/拒绝旧 key 的测试断言，或说明迁移的注释。命令实现不得读写持久化 `context.active`。

- [ ] **Step 6：运行 context 测试**

Run:

```bash
go test ./internal/app ./internal/cli ./tests/integration -run 'Context|RcOverride|Show' -count=1
```

Expected: PASS。同步更新 M3 中断言 `context.active` 出现在 `show`、`_show`、config 或集成输出中的测试。

- [ ] **Step 7：提交**

```bash
git add internal/app/context.go internal/cli/root.go internal/cli/config.go internal/app/service_test.go internal/cli/root_test.go tests/integration/cli_test.go
git commit -m "feat: 按用户和 workspace 隔离 active context"
```

### Task 7：实现 User、Workspace、Member、Audit App APIs

**Files:**
- Create: `internal/app/workspace.go`
- Modify: `internal/app/audit.go`
- Modify: `internal/app/service.go`
- Test: `internal/app/service_test.go`

- [ ] **Step 1：写失败的 app API 测试**

覆盖：

- `AddUser` 创建 user + private personal workspace + owner membership。
- `UseUser` 写入 `active_user_id`。
- `UseUser` 忽略 `ServiceOptions.WorkspaceRef`；`taskg --workspace work user use alice` 不得设置 Alice 的 active workspace。CLI 可以之后选择拒绝该组合，但 app 语义是“只切换 user”。
- `AddWorkspace` 创建 workspace，并让当前 actor 成为 owner。
- `UseWorkspace` 写入 `active_workspace.<user_id>`。
- `ModifyWorkspace` 需要 admin/owner，并写 audit `workspace.modify`。
- `ArchiveWorkspace` 在任一受影响 user 没有其他未归档 workspace 时拒绝。
- `AddMember` 和 `ChangeMemberRole` 执行 owner/admin 规则和最后 owner 保护。

- [ ] **Step 2：运行测试确认失败**

Run:

```bash
go test ./internal/app -run 'User|Workspace|Member|Archive|LastOwner' -count=1
```

Expected: FAIL，因为 app APIs 还不存在。

- [ ] **Step 3：实现 user APIs**

```go
func (s *Service) ListUsers() ([]UserView, error)
func (s *Service) AddUser(input AddUserInput) (UserView, error)
func (s *Service) UseUser(ref string) error
func (s *Service) UserInfo(ref string) (UserView, error)
```

`AddUser` 必须在一个 transaction 中完成：

- 创建 user。
- 创建 `visibility=private` 的 personal workspace。
- 创建 owner membership。
- 设置 user default workspace。
- 使用 `withAuditEntries` 一次写 audit `user.add` 和 `workspace.add`。不要连续调用两次 `withAudit`。

权限：`AddUser` 是本地身份管理动作，不套用当前 workspace role permission；但必须有非空 actor，并在 audit 中记录 actor。不要用 `PermissionWorkspaceModify` 或 `PermissionMemberManageOwner` 来保护它，否则会把 user 管理错误地绑定到当前 workspace。

`UseUser` 写入 `active_user_id` 并审计 `user.use`。它不需要当前 service 的 workspace membership，只需要解析目标 user 的 default/active workspace。

`UseUser` 必须忽略当前 service 的 workspace override。它只切换 active user，不能顺手设置目标 user 的 active workspace。

- [ ] **Step 4：实现 workspace APIs**

```go
func (s *Service) ListWorkspaces(includeArchived bool) ([]WorkspaceView, error)
func (s *Service) AddWorkspace(input AddWorkspaceInput) (WorkspaceView, error)
func (s *Service) UseWorkspace(ref string) error
func (s *Service) WorkspaceInfo(ref string) (WorkspaceView, error)
func (s *Service) ModifyWorkspace(ref string, input ModifyWorkspaceInput) error
func (s *Service) ArchiveWorkspace(ref string) error
```

Archive 算法：

1. 需要 `s.Require(PermissionWorkspaceArchive)`，只允许 owner。
2. 找出所有受影响 users：`default_workspace_id = target`，或 `active_workspace.<user_id>` 指向 target。
3. 对每个受影响 user，查找其他未归档 membership workspace，按 slug 排序，并明确排除 target。
4. 如果任一 user 没有可替代 workspace，整个 archive 失败。
5. 更新受影响 user 的 default workspace。
6. 如果受影响 user 的 active meta 指向 target，则更新为该 user 选中的替代 workspace。
7. 归档 target。
8. 写 audit `workspace.archive`。不要为 default workspace 的副作用更新额外写 `user.default_changed`；把被重新分配的 user 和目标 workspace 摘要放进 `workspace.archive` payload。

其他 workspace 方法权限：

- `AddWorkspace` 不调用 role `Require`；它只要求当前 actor 非空，创建后当前 actor 成为 owner，并审计 `workspace.add`。
- `UseWorkspace` 不调用 role `Require`；它要求当前 actor 是目标 workspace member、目标 workspace 未归档，写 `active_workspace.<user_id>` 并审计 `workspace.use`。
- `ModifyWorkspace` 需要 `PermissionWorkspaceModify`，并审计 `workspace.modify`。
- 上述需要权限的检查必须在 `withAudit/withAuditEntries` 外层执行。

- [ ] **Step 5：实现 member APIs**

```go
func (s *Service) ListMembers(workspaceRef string) ([]MemberView, error)
func (s *Service) AddMember(input AddMemberInput) error
func (s *Service) ChangeMemberRole(input ChangeMemberRoleInput) error
```

规则：

- user ref 支持 name/email/UUID。
- admin 可以添加/改为 viewer/member/admin，不能授予或降级 owner。
- owner 可以授予 owner。
- 不能降级最后一个 owner。
- M4 不提供 member delete。

权限检查必须在 audit transaction 外层执行：

- `ListMembers` 对 viewer 开放，不需要 member 管理权限。
- `AddMember` 如果目标 role 是 owner，需要 `PermissionMemberManageOwner`；否则需要 `PermissionMemberManage`。
- `ChangeMemberRole` 如果 `newRole == owner` 或当前 role 是 owner，需要 `PermissionMemberManageOwner`；否则需要 `PermissionMemberManage`。

- [ ] **Step 6：运行 app API 测试**

Run:

```bash
go test ./internal/app -run 'User|Workspace|Member|Archive|LastOwner' -count=1
```

Expected: PASS。

- [ ] **Step 7：提交**

```bash
git add internal/app/workspace.go internal/app/service.go internal/app/audit.go internal/app/service_test.go
git commit -m "feat: 增加本地团队 app 接口"
```

---

## Chunk 4：CLI 命令组

### Task 8：新增 User 和 Workspace CLI 命令

**Files:**
- Create: `internal/cli/user.go`
- Create: `internal/cli/workspace.go`
- Modify: `internal/cli/root.go`
- Test: `tests/integration/cli_test.go`

- [ ] **Step 1：写失败的 CLI 集成测试**

新增测试：

```go
func TestCLIUserWorkspaceLifecycle(t *testing.T) {
    bin := buildTaskg(t)
    db := filepath.Join(t.TempDir(), "taskg.db")
    run(t, bin, "--db", db, "user", "add", "alice", "email:alice@example.test")
    out := run(t, bin, "--db", db, "user", "list")
    assertContains(t, out, "alice")
    run(t, bin, "--db", db, "user", "use", "alice")
    out = run(t, bin, "--db", db, "workspace", "list")
    assertContains(t, out, "alice")
    run(t, bin, "--db", db, "workspace", "add", "work", "name:Work", "visibility:team")
    run(t, bin, "--db", db, "workspace", "modify", "work", "description:Team")
    info := run(t, bin, "--db", db, "workspace", "info", "work")
    assertContains(t, info, "Team")
}
```

- [ ] **Step 2：运行测试确认失败**

Run:

```bash
go test ./tests/integration -run 'TestCLIUserWorkspaceLifecycle' -count=1
```

Expected: FAIL，因为命令还不存在。

- [ ] **Step 3：实现 user commands**

`user list`：

- Human columns: ACTIVE、NAME、EMAIL、DEFAULT。
- `--json` 输出 JSON array。

`user add`：

- 解析 `email:<email>`。
- human 模式输出创建结果。

`user use`：

- 调 app。
- 输出空或 `Using user <name>`；建议短确认。
- `taskg --workspace work user use alice` 必须忽略 `--workspace`；只切换 active user，不设置 Alice 的 active workspace。

`user info`：

- 无参数时显示当前 actor。
- `--json` 输出 JSON。

- [ ] **Step 4：实现 workspace commands**

`workspace list [--all]`、`add`、`use`、`info`、`modify`、`archive`。

modifier 解析沿用现有 `add/modify` 命令里的本地风格，不要发明新 DSL。

支持脚本用 JSON 输出。

- [ ] **Step 5：注册命令**

在 root：

```go
cmd.AddCommand(newUserCommand(opts))
cmd.AddCommand(newWorkspaceCommand(opts))
```

必要时更新 `knownSubcommands` 相关测试。

- [ ] **Step 6：运行 user/workspace CLI 测试**

Run:

```bash
go test ./tests/integration -run 'TestCLIUserWorkspaceLifecycle' -count=1
```

Expected: PASS。

- [ ] **Step 7：提交**

```bash
git add internal/cli/user.go internal/cli/workspace.go internal/cli/root.go tests/integration/cli_test.go
git commit -m "feat: 增加 user 和 workspace 命令"
```

### Task 9：新增 Member 和 Audit CLI 命令

**Files:**
- Create: `internal/cli/member.go`
- Create: `internal/cli/audit.go`
- Modify: `internal/cli/root.go`
- Test: `tests/integration/cli_test.go`

- [ ] **Step 1：写失败的集成测试**

覆盖：

- `--workspace missing list` 非零退出，错误码 `workspace_not_found`。
- `--workspace <archived> list` 非零退出，错误码 `workspace_archived`。
- `--workspace <non-member-workspace> list` 非零退出，错误码 `membership_not_found` 或 `permission_denied`。
- `member add bob role:viewer`。
- viewer 能 list，但不能 add task。
- member 不能管理 members。
- admin 不能 archive workspace。
- owner 在有另一个 workspace 时可以 archive。
- `audit list --json` 包含 `task.add`、`member.add`、`workspace.modify`。

- [ ] **Step 2：运行测试确认失败**

Run:

```bash
go test ./tests/integration -run 'TestCLIWorkspaceErrorSemantics|TestCLIMemberPermissions|TestCLIAuditList' -count=1
```

Expected: FAIL，因为命令还不存在或权限尚未接到 CLI。

- [ ] **Step 3：实现 member commands**

`member list [--workspace]`：

- 尽量复用全局 `--workspace`。如确实需要 command-local flag，把它传给 app input，不要修改全局 opts。
- Human columns: USER、EMAIL、ROLE、JOINED。

`member add <user> [role:<role>]`。

`member role <user> <role>`。

- [ ] **Step 4：实现 audit command**

`audit list [--limit N] [--workspace <ref>]`。

规则：

- 默认 limit 50。
- human 输出 newest first。
- JSON array 字段按 spec。
- JSON `payload` 字段应是从 `payload_json` 解码出来的 object；如果 payload 为空或解码失败，输出 `null`。
- 不做复杂 filter。

- [ ] **Step 5：注册命令和 JSON 错误**

注册 `member` 和 `audit`。

如果现有 root error renderer 不支持 JSON error，可以只给新命令加最小处理；若改动太大，保留普通 error，并把完整 JSON error 统一化记为后续事项，不阻塞 M4。

- [ ] **Step 6：运行 member/audit 测试**

Run:

```bash
go test ./tests/integration -run 'TestCLIWorkspaceErrorSemantics|TestCLIMemberPermissions|TestCLIAuditList' -count=1
```

Expected: PASS。

- [ ] **Step 7：提交**

```bash
git add internal/cli/member.go internal/cli/audit.go internal/cli/root.go tests/integration/cli_test.go
git commit -m "feat: 增加 member 和 audit 命令"
```

---

## Chunk 5：Workspace 隔离与现有命令接线

### Task 10：确保所有现有 CLI 路径尊重 Runtime Workspace

**Files:**
- Modify: `internal/cli/root.go`
- Modify: `internal/app/service.go`
- Modify: `internal/app/context.go`
- Modify: `internal/app/uda.go`
- Test: `tests/integration/cli_test.go`
- Test: `internal/storage/sqlite/query_scope_test.go`

- [ ] **Step 1：写失败的跨 workspace 集成测试**

流程：

1. 在默认 local workspace 添加 "local task"，project/tag 为 `same`。
2. 创建 workspace `work` 并 use。
3. 添加 "work task"，使用相同 project/tag/UDA/context 名称。
4. 断言：
   - `taskg list` 只显示 work task。
   - `taskg --workspace local list` 只显示 local task。
   - `_projects`、`_tags`、`_unique estimate`、`_udas`、`_ids`、`_uuids`、`_get`、`_urgency` 都尊重 `--workspace`。
   - working-set ID `1` 在不同 workspace 中独立解析。

- [ ] **Step 2：运行测试确认失败**

Run:

```bash
go test ./tests/integration -run 'TestCLIWorkspaceIsolation' -count=1
```

Expected: FAIL，直到 `--workspace` 和 runtime context 完整贯通。

- [ ] **Step 3：修复仍绕过 service 的命令**

搜索：

```bash
rg "\\.LocalWorkspace\\b|context\\.active" internal/app internal/cli
```

修复：

- app 业务路径中任何 `LocalWorkspace` 调用。
- config 路径中任何旧 `context.active` 读取。
- helper 路径中任何没有走 `buildServiceFromCmd` 的命令。

`LocalWorkspace()` 可以留在 storage migration/tests 中，但不得出现在 app 业务代码里。

- [ ] **Step 4：验证 storage-level scoping**

扩展 `query_scope_test.go`，断言 UDA subquery、tag/dependency/annotation subquery 都包含 workspace predicate。

现有测试已经覆盖不少场景；如果缺 UDA schema/current workspace 回归，补一条。

- [ ] **Step 5：运行隔离测试**

Run:

```bash
go test ./tests/integration -run 'TestCLIWorkspaceIsolation' -count=1
```

Expected: PASS。

- [ ] **Step 6：提交**

```bash
git add internal tests
git commit -m "fix: 贯通 workspace 隔离"
```

### Task 11：处理自动状态推进、循环任务与审计/权限边界

**Files:**
- Modify: `internal/app/service.go`
- Modify: `internal/app/audit.go`
- Test: `internal/app/service_test.go`

- [ ] **Step 1：识别自动写入路径**

当前自动写入：

- `refreshAutomaticState` 把 waiting 转回 pending。
- `ensureRecurringChildren` 创建下一个 child。
- `Done` 完成任务后可能创建 recurring child。

- [ ] **Step 2：确定 audit/permission 规则**

规则：

- 用户触发的命令 audit action 仍是命令本身，例如 `task.done`、`task.add`。
- 命令内部的自动写入尽量放在同一 transaction 中，但不需要独立 public audit action，除非 spec 已列出。
- 自动状态维护绕过用户权限检查。viewer 执行 `list` 时，不能因为 `refreshAutomaticStateLocked` 把 waiting task 推进到 pending 而失败。
- 自动维护必须保持 workspace-scoped，不能暴露或修改其他 workspace。
- 如果 recurring child creation 可以从 read path 触发，它也作为内部维护绕过权限检查。如果实现时觉得范围过宽，可以缩窄触发点，让 read path 不创建 recurring child；但不能让 viewer read 失败。

- [ ] **Step 3：补自动维护 scope 和 recurring audit 测试**

测试：

- 两个 workspace 各有一个 waiting task；actor/viewer 在其中一个 workspace 执行 `List`，只有当前 workspace 的 waiting task 被推进。
- 对 recurring child 执行 `Done` 会写 `task.done` audit，并且下一个 child 仍在同一 workspace。

- [ ] **Step 4：按需重构 transaction 边界**

如果 `Done` 当前先更新 task、再创建 child 是分散写入，把两者和 audit 包进 `withAudit`。

避免对内部 child 创建重复写 audit。

- [ ] **Step 5：运行 recurrence/app 测试**

Run:

```bash
go test ./internal/app ./internal/recurrence -run 'Recurring|Audit|Done' -count=1
```

Expected: PASS。

- [ ] **Step 6：提交**

```bash
git add internal/app/service.go internal/app/audit.go internal/app/service_test.go
git commit -m "feat: 完善循环任务审计路径"
```

---

## Chunk 6：文档、路线图与完整验证

### Task 12：更新 README 和 ROADMAP

**Files:**
- Modify: `README.md`
- Modify: `ROADMAP.md`
- Modify: `docs/superpowers/specs/2026-05-29-taskg-m4-design.md` only if implementation changes the spec.

- [ ] **Step 1：更新 README M4 用法**

新增章节：

- `user list/add/use/info`
- `workspace list/add/use/info/modify/archive`
- `--workspace`
- `member list/add/role`
- `audit list`
- role 概览与警告：M4 没有 `member delete`；viewer 仍能读取 workspace 数据。
- migrated `local` user 的 email 是 empty/null。
- M3 到 M4 升级行为：现有任务留在 local workspace；自动创建 local user/workspace/membership；旧 `context.active` meta 会迁移到 `(local user, local workspace)` scoped active context。
- M3 用户升级后第一次运行 `taskg` 会自动完成迁移，不需要手动执行迁移命令。

- [ ] **Step 2：更新 ROADMAP**

把 M4 标为已完成，并列出交付内容：

- Users/workspaces/memberships。
- Runtime context。
- Permissions。
- Audit logs。
- Tasks/context/UDA/helpers 的 workspace isolation。

下一步指向 M5。

- [ ] **Step 3：运行文档 diff 检查**

Run:

```bash
git diff --check
```

Expected: no output。

- [ ] **Step 4：提交**

```bash
git add README.md ROADMAP.md docs/superpowers/specs/2026-05-29-taskg-m4-design.md
git commit -m "docs: 更新 M4 使用说明"
```

### Task 13：完整 M4 验证

**Files:**
- 无计划改动；除非验证发现问题。

- [ ] **Step 1：运行单元与集成测试**

Run:

```bash
go test ./...
```

Expected: PASS。

- [ ] **Step 2：运行 CGO-free 测试**

Run:

```bash
CGO_ENABLED=0 go test ./...
```

Expected: PASS。

- [ ] **Step 3：运行 CGO-free build**

Run:

```bash
CGO_ENABLED=0 go build ./cmd/taskg
```

Expected: PASS。

- [ ] **Step 4：运行重点 CLI 集成测试**

Run:

```bash
go test ./tests/integration -run TestCLI -count=1
```

Expected: PASS。

- [ ] **Step 5：检查最终 git 状态**

Run:

```bash
git status --short
git log --oneline -8
```

Expected: 工作树干净，除了可选本地构建产物 `taskg`。如果出现未跟踪 `taskg`，只有确认它是本任务 build 生成物后才删除。

- [ ] **Step 6：如果验证修复了问题，做最终提交**

如果验证阶段有修复：

```bash
git add <fixed-files>
git commit -m "fix: 完成 M4 验证收尾"
```

---

## 执行前 Review Checklist

开始实现前必须阅读：

- [M4 spec](/Users/mac/code/projects/dajee/task/docs/superpowers/specs/2026-05-29-taskg-m4-design.md)
- [AGENTS.md](/Users/mac/code/projects/dajee/task/AGENTS.md)
- [internal/app/service.go](/Users/mac/code/projects/dajee/task/internal/app/service.go)
- [internal/storage/sqlite/db.go](/Users/mac/code/projects/dajee/task/internal/storage/sqlite/db.go)
- [internal/cli/root.go](/Users/mac/code/projects/dajee/task/internal/cli/root.go)

实现必须保持：

- 只使用 `github.com/glebarez/sqlite`，不要引入 CGO SQLite driver。
- stdout/stderr 分离。
- 新命令有稳定 `--json` 输出。
- 数字 working-set ID 按 workspace 隔离。
- service 业务路径不依赖 `store.LocalWorkspace()`。
- 不再持久读写旧 `context.active`。
- audit 写入与对应业务写入在同一个 store-level transaction 中完成。
