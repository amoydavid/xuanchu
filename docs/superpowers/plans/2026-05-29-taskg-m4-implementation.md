# taskg M4 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement M4: local multi-user, multi-workspace runtime context, membership permissions, audit logging, and workspace-scoped CLI behavior without adding HTTP, PAT/JWT, MCP, or sync.

**Architecture:** Add user/workspace/member/audit persistence in `internal/storage/sqlite`, resolve a per-command runtime context in `internal/app`, and bind each service instance to `ActorUserID + WorkspaceID + Role`. Keep permission checks and audit orchestration in `internal/app`; keep SQL/GORM repositories focused on persistence and workspace-scoped data access. CLI commands stay thin and call app services.

**Tech Stack:** Go 1.22, Cobra, GORM, `github.com/glebarez/sqlite` with `CGO_ENABLED=0`, existing `internal/query`, `internal/task`, `internal/uda`, `internal/render`, and black-box CLI integration tests.

---

## Scope Lock

Build exactly what [M4 spec](/Users/mac/code/projects/dajee/task/docs/superpowers/specs/2026-05-29-taskg-m4-design.md) defines:

- Users, workspaces, memberships, audit logs.
- Active user, active workspace, and active context keyed by user/workspace.
- `--workspace` global override.
- `user`, `workspace`, `member`, and `audit` CLI command groups.
- Real role checks for viewer/member/admin/owner.
- Audit for write operations listed in the spec, in the same store-level transaction as the business write.

Do not add HTTP server, remote CLI, token commands, auth, member delete, user delete, workspace hard delete, op-log sync, MCP, hooks, or cross-workspace/global context query.

## File Structure

Create:

- `internal/app/runtime.go`  
  Runtime context types, actor/workspace resolution, active meta key helpers.
- `internal/app/permission.go`  
  Role constants, permission constants, `Require(permission)` checks.
- `internal/app/workspace.go`  
  User/workspace/member app methods.
- `internal/app/audit.go`  
  Audit app methods and audit helper used by write paths.
- `internal/storage/sqlite/user_repo.go`  
  User CRUD, active/default helpers where storage-specific.
- `internal/storage/sqlite/workspace_repo.go`  
  Workspace CRUD, archive, visible workspace queries.
- `internal/storage/sqlite/member_repo.go`  
  Membership CRUD, role update, owner counting, membership lookups.
- `internal/storage/sqlite/audit_repo.go`  
  Append/list audit records.
- `internal/cli/user.go`  
  `taskg user ...` commands.
- `internal/cli/workspace.go`  
  `taskg workspace ...` commands.
- `internal/cli/member.go`  
  `taskg member ...` commands.
- `internal/cli/audit.go`  
  `taskg audit list`.

Modify:

- `internal/storage/sqlite/models.go`  
  Add/extend GORM models for users, workspaces, memberships, audit logs.
- `internal/storage/sqlite/db.go`  
  AutoMigrate new tables, ensure local user/workspace/membership, migrate active context meta.
- `internal/storage/sqlite/db_test.go`  
  Migration/idempotency tests.
- `internal/app/service.go`  
  Bind service to runtime context, remove business-path dependency on `Store.LocalWorkspace()`, route write methods through permission+audit transaction helpers.
- `internal/app/context.go`  
  Replace `context.active` with `active_context.<user_id>.<workspace_id>`.
- `internal/app/uda.go`  
  Permission checks and audit for UDA schema writes.
- `internal/app/service_test.go`  
  Runtime context, permissions, audit, isolation unit tests.
- `internal/cli/root.go`  
  Add `--workspace`, remove `rc.context.active` as an accepted key, add command groups.
- `internal/cli/config.go`  
  Show new active keys, stop reading/writing old `context.active`.
- `internal/cli/context.go`  
  Respect split permissions for `context use/none` vs `define/delete`.
- `internal/cli/helper.go`  
  Ensure helpers inherit `--workspace` through root options.
- `tests/integration/cli_test.go`  
  CLI black-box tests for M4 flows.
- `README.md`, `ROADMAP.md`  
  Update after implementation.

---

## Chunk 1: Storage Schema, Migration, and Repositories

### Task 1: Extend SQLite Models and Migration

**Files:**
- Modify: `internal/storage/sqlite/models.go`
- Modify: `internal/storage/sqlite/db.go`
- Test: `internal/storage/sqlite/db_test.go`

- [ ] **Step 1: Write failing migration tests**

Add tests:

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

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/storage/sqlite -run 'TestOpenInitializesLocalUserWorkspaceAndMembership|TestOpenMigratesContextActiveMeta' -count=1`

Expected: FAIL because repositories/models do not exist and migration does not create users/memberships.

- [ ] **Step 3: Update models**

In `models.go`:

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
    Visibility      string `gorm:"not null;default:private"`
    SettingsJSON    string `gorm:"not null;default:{}"`
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

Keep existing `Task`, `Context`, `UDADefinition`, and `TaskUDAValue` unchanged except for relationships if needed.

- [ ] **Step 4: Add migration helpers**

In `db.go`:

- AutoMigrate `User`, updated `Workspace`, `Membership`, `AuditLog`.
- Replace `ensureLocalWorkspace()` with `ensureLocalIdentity()` that:
  - Creates local user with nil email.
  - Creates or updates local workspace with `Slug: "local"`, `Visibility: "private"`, `SettingsJSON: "{}"`, `CreatedByUserID: &localUser.ID`.
  - For historical workspaces with `modified_at = 0`, set `ModifiedAt = CreatedAt`.
  - Sets local user `DefaultWorkspaceID` to local workspace.
  - Creates owner membership.
  - Migrates `context.active` to `active_context.<local_user_id>.<local_workspace_id>` and deletes old key.
- Keep `LocalWorkspace()` as a compatibility helper, but app business paths must stop using it in later tasks.

- [ ] **Step 5: Run storage migration tests**

Run: `go test ./internal/storage/sqlite -run 'TestOpenInitializesLocalUserWorkspaceAndMembership|TestOpenMigratesContextActiveMeta' -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/storage/sqlite/models.go internal/storage/sqlite/db.go internal/storage/sqlite/db_test.go
git commit -m "feat: 初始化 M4 本地身份模型"
```

### Task 2: Add User, Workspace, Membership, and Audit Repositories

**Files:**
- Create: `internal/storage/sqlite/user_repo.go`
- Create: `internal/storage/sqlite/workspace_repo.go`
- Create: `internal/storage/sqlite/member_repo.go`
- Create: `internal/storage/sqlite/audit_repo.go`
- Test: `internal/storage/sqlite/db_test.go` or new `internal/storage/sqlite/identity_repo_test.go`

- [ ] **Step 1: Write failing repository tests**

Create tests covering:

- User lookup by name/email/UUID.
- Workspace slug validation and global uniqueness.
- Visible workspaces exclude archived by default.
- Membership role updates preserve `JoinedAt` and update `ModifiedAt`.
- `CountOwners(workspaceID)` protects last owner.
- Audit list returns newest first and respects limit.

Example:

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

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/storage/sqlite -run 'UserRepository|WorkspaceRepository|MemberRepository|AuditRepository' -count=1`

Expected: FAIL because repos do not exist.

- [ ] **Step 3: Implement repository DTOs and methods**

Use storage-level DTOs first. Do not add these concepts to `internal/task`.

Minimum methods:

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

Return `sqlite.ErrNotFound` consistently.

- [ ] **Step 4: Run repository tests**

Run: `go test ./internal/storage/sqlite -run 'UserRepository|WorkspaceRepository|MemberRepository|AuditRepository' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/storage/sqlite/*repo.go internal/storage/sqlite/*test.go
git commit -m "feat: 增加 M4 身份仓储"
```

---

## Chunk 2: Runtime Context, Permissions, and Audit Transactions

### Architecture Decisions for Chunk 2+

All write paths must follow this shape:

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
    // old Add body; only uses repositories on this service instance
}
```

Rules:

- Public write methods perform permission checks once, then call `withAudit`.
- `withAudit` opens the store-level transaction and passes a tx-bound `*Service` to the closure.
- The closure must call `xxxLocked` methods on the tx-bound service, never public methods on the outer service.
- `xxxLocked` methods do not call `Require` and do not open their own app-level audit transaction.
- `withAudit` receives the audit entry from the closure so runtime-generated IDs such as task UUIDs can be audited.
- Automatic state maintenance (`refreshAutomaticStateLocked`, recurring child creation) is internal maintenance and bypasses user permission checks. It must still stay workspace-scoped. It does not create separate public audit actions unless the spec lists one.
- TOML `context.active` is not read in M4. Users should run `taskg context use <name>` once after upgrade if they relied on TOML-only active context.

### Task 3: Introduce Runtime Context and Bind Service Per Command

**Files:**
- Create: `internal/app/runtime.go`
- Modify: `internal/app/service.go`
- Modify: `internal/cli/root.go`
- Modify: `internal/cli/config.go`
- Test: `internal/app/service_test.go`
- Test: `internal/cli/root_test.go`

- [ ] **Step 1: Write failing runtime tests**

Add app tests:

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
    // create workspace "work"; call NewService with WorkspaceRef: "work"
    // assert svc.Runtime().WorkspaceSlug == "work"
}
```

Add CLI root test for splitting `--workspace work`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/app ./internal/cli -run 'Runtime|WorkspaceOverride|ParsesWorkspace' -count=1`

Expected: FAIL because runtime context and `--workspace` do not exist.

- [ ] **Step 3: Add app runtime types**

In `internal/app/runtime.go`:

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

Move `ServiceOptions` definition from `service.go` to `runtime.go` or extend it in place. Keep API-compatible zero values: no actor/workspace ref means local active user/default workspace.

Zero-value `ActorRef` and `WorkspaceRef` must preserve M3 behavior for existing tests: local user, local/default workspace, no caller changes required for `NewService(ServiceOptions{Store: store, Clock: ...})`.

- [ ] **Step 4: Implement runtime resolution**

Resolution order:

1. Active user meta `active_user_id`; fallback `local`.
2. `WorkspaceRef` from `--workspace`; otherwise `active_workspace.<user_id>`; otherwise `users.default_workspace_id`.
3. Reject archived workspace.
4. Reject missing membership.
5. Bind role to service.

Add helper methods:

```go
func activeWorkspaceMetaKey(userID string) string
func activeContextMetaKey(userID, workspaceID string) string
func (s *Service) Runtime() RuntimeContext
```

- [ ] **Step 5: Update service construction**

In `NewService`, initialize repositories:

```go
userRepo := sqlite.NewUserRepository(opts.Store.DB())
workspaceRepo := sqlite.NewWorkspaceRepository(opts.Store.DB())
memberRepo := sqlite.NewMemberRepository(opts.Store.DB())
auditRepo := sqlite.NewAuditRepository(opts.Store.DB())
rt, err := ResolveRuntimeContext(opts.Store, userRepo, workspaceRepo, memberRepo, opts.ActorRef, opts.WorkspaceRef)
```

Set `s.workspaceID = rt.WorkspaceID` for incremental compatibility, but all new code should use `s.runtime.WorkspaceID`.

In `withStore`, keep the same runtime context and rebuild repositories against the transaction DB. Do not call `LocalWorkspace()`.

Hard requirement for this task: after Step 5, this command must return no matches:

```bash
rg "LocalWorkspace" internal/app
```

`Store.LocalWorkspace()` may remain in `internal/storage/sqlite` and storage tests as a migration compatibility helper, but app business paths must not call it.

- [ ] **Step 6: Add root `--workspace`**

In `Options`, add `Workspace string`.

In root flags:

```go
cmd.PersistentFlags().StringVar(&opts.Workspace, "workspace", opts.Workspace, "workspace slug or UUID")
```

In `splitFlagsRcAndPositional`, add `--workspace` to `stringFlags`.

In `buildServiceFromOpts`, pass `WorkspaceRef: opts.Workspace`.

- [ ] **Step 7: Update config/show path**

`runtimeFromResolvedConfig` currently builds a service to read UDA config. Pass workspace override into `NewService`.

Do not include old `context.active` in `show` output. Replace with:

- `active.user`
- `active.workspace`
- `active.context`

Plan can keep `date.format`, `color`, `json`, `database.path`.

- [ ] **Step 8: Run runtime tests**

Run: `go test ./internal/app ./internal/cli -run 'Runtime|WorkspaceOverride|ParsesWorkspace|Show' -count=1`

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/app/runtime.go internal/app/service.go internal/cli/root.go internal/cli/config.go internal/app/service_test.go internal/cli/root_test.go
git commit -m "feat: 解析 M4 运行时上下文"
```

### Task 4: Add Permission Checks

**Files:**
- Create: `internal/app/permission.go`
- Modify: `internal/app/service.go`
- Modify: `internal/app/context.go`
- Modify: `internal/app/uda.go`
- Test: `internal/app/service_test.go`

- [ ] **Step 1: Write failing permission tests**

Test matrix:

```go
func TestViewerCannotModifyTasks(t *testing.T) { /* Add as owner, switch runtime to viewer, Modify returns permission_denied */ }
func TestViewerCanUseOwnContextButCannotDefineContext(t *testing.T) { /* UseContext ok if context exists, DefineContext denied */ }
func TestMemberCannotManageMembersOrWorkspaceMetadata(t *testing.T) { /* app methods return denied */ }
func TestAdminCannotArchiveWorkspace(t *testing.T) { /* denied */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/app -run 'Viewer|MemberCannot|AdminCannot|Permission' -count=1`

Expected: FAIL because permissions are not implemented.

- [ ] **Step 3: Implement permission primitives**

In `permission.go`:

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

Implement the spec matrix:

- viewer: read/export/context use/member list only.
- member: task write/import/context define/delete/use.
- admin: member manage except owner changes, UDA schema, workspace metadata, audit read.
- owner: all.

Use `PermissionMemberManageOwner` for promoting another user to owner or downgrading an owner. Admin must not pass that check.

- [ ] **Step 4: Guard app methods**

Add `Require` calls to:

- Task write methods: public `Add`, `Modify`, `Done`, `Delete`, `Start`, `Stop`, `Annotate`, `Denotate`, `AppendDescription`, `PrependDescription`, `ReplaceEditableTask`, `Import`.
- Context: `UseContext` and `ContextNone` require `context.use`; `DefineContext` and `ContextDelete` require `context.manage`.
- UDA schema: `DefineUDA`, `DeleteUDA`, `setUDAConfig`, `unsetUDAConfig` require `uda.manage` when they mutate schema.
- Export/List/Info/Reports/helpers remain readable by viewer.

Do not put permission checks in `xxxLocked` methods; those are internal transaction bodies used after the public method has already authorized the operation.

- [ ] **Step 5: Run permission tests**

Run: `go test ./internal/app -run 'Viewer|MemberCannot|AdminCannot|Permission' -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/app/permission.go internal/app/service.go internal/app/context.go internal/app/uda.go internal/app/service_test.go
git commit -m "feat: 增加 M4 权限边界"
```

### Task 5: Add Audit Logging in Same Store-Level Transaction

**Files:**
- Create: `internal/app/audit.go`
- Modify: `internal/app/service.go`
- Modify: `internal/app/context.go`
- Modify: `internal/app/uda.go`
- Test: `internal/app/service_test.go`

- [ ] **Step 1: Write failing audit tests**

Tests:

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

Add a rollback test. Preferred shape:

- Add a test-only hook or small interface around audit append.
- Force audit append to return an error after the business write succeeds.
- Assert the task/config/member/workspace change is not persisted.

Do not leave same-transaction audit as an untested assumption. If adding a fake repository interface becomes too invasive, use a transaction-level test helper that inserts an invalid audit row and verifies the business write rolls back.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/app -run 'Audit' -count=1`

Expected: FAIL because audit methods do not exist.

- [ ] **Step 3: Implement audit helper**

In `audit.go`:

```go
type AuditListInput struct {
    WorkspaceRef string
    Limit int
}

type AuditEntry struct {
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

Important:

- The closure must call tx-bound `xxxLocked` methods, not public methods on the outer service.
- The closure returns `AuditEntry` after it knows runtime-generated IDs such as task UUIDs.
- If `TaskRepository.Update` opens a nested transaction, GORM savepoints are acceptable, but do not call non-transaction-bound repos from the outer `s` inside the closure.
- `withAudit` should have a sibling helper for actions that need to return a value:

```go
func (s *Service) withAuditValue[T any](action string, fn func(*Service) (T, AuditEntry, error)) (T, error)
```

If the codebase should avoid generics in app helpers, use explicit local capture as shown in the architecture decision and keep `withAudit` non-generic.

- [ ] **Step 4: Wrap write paths**

Use actions from spec:

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

Do not audit `Export`.

For each wrapped method:

- Move the old method body into `xxxLocked`.
- Public method: `Require`, then `withAudit`, then call `tx.xxxLocked`.
- `xxxLocked` must use repositories on its receiver only.

- [ ] **Step 5: Implement `ListAudit`**

Require `audit.read` and call audit repo with current workspace. Default limit is handled by CLI, but app should coerce `<=0` to 50.

- [ ] **Step 6: Run audit and app tests**

Run: `go test ./internal/app -run 'Audit|Add|Modify|Context|UDA' -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/app/audit.go internal/app/service.go internal/app/context.go internal/app/uda.go internal/app/service_test.go
git commit -m "feat: 审计 M4 写操作"
```

---

## Chunk 3: Active Context, User/Workspace/Member App APIs

### Task 6: Migrate Active Context to User+Workspace Scope

**Files:**
- Modify: `internal/app/context.go`
- Modify: `internal/cli/root.go`
- Modify: `internal/cli/config.go`
- Test: `internal/app/service_test.go`
- Test: `tests/integration/cli_test.go`

- [ ] **Step 1: Write failing active context isolation tests**

Create app or CLI test:

1. Owner defines context `work`.
2. Add user Bob, add Bob to same workspace.
3. Alice runs `context use work`.
4. Bob's `context show` is empty.
5. Bob can run `context use work` as viewer if context exists.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/app ./tests/integration -run 'ActiveContext|ContextIsolation' -count=1`

Expected: FAIL because active context is a single `context.active` key.

- [ ] **Step 3: Replace `activeContextMetaKey` constant**

Remove global `const activeContextMetaKey = "context.active"` for writes.

Use:

```go
func (s *Service) activeContextMetaKey() string {
    return activeContextMetaKey(s.runtime.ActorUserID, s.runtime.WorkspaceID)
}
```

`activeContextName` order:

1. `activeContextOverride`.
2. `runtimeOverrides["context"]` only for `rc.context=none` / `rc.context:<value>` compatibility during CLI parsing, not persisted.
3. Store meta `active_context.<user_id>.<workspace_id>`.

Do not read TOML `context.active` in M4. If a user relied only on TOML `context.active`, they must run `taskg context use <name>` once after upgrade. This avoids reintroducing machine-wide active context state.

- [ ] **Step 4: Update RC handling**

In root:

- Remove `"context.active"` from allowed public config keys.
- Keep `rc.context=none`, `rc.context:`, and `rc.context=` support by mapping to the in-memory no-context/override behavior.
- Reject `rc.context.active=...` for M4.

- [ ] **Step 5: Update show/config behavior**

`config set context.active ...` and `config unset context.active` should return "managed by context commands" or "unsupported legacy key".

`show` should display `active.context=<name>` or empty, not `context.active`.

`_show context.active` and `config get context.active` should return a clear unsupported legacy key error. Do not silently fall back to scoped active context under the old key.

Add a cleanup check:

```bash
rg "context\\.active" internal/cli internal/app tests/integration
```

After Task 6, remaining matches must be intentional test assertions for rejecting/migrating the legacy key, or comments documenting the migration. Command implementations must not read or write persistent `context.active`.

- [ ] **Step 6: Run context tests**

Run: `go test ./internal/app ./internal/cli ./tests/integration -run 'Context|RcOverride|Show' -count=1`

Expected: PASS, with existing M3 tests updated to M4 output. Specifically search and update tests that assert `context.active` in `show`, `_show`, config, or integration output.

- [ ] **Step 7: Commit**

```bash
git add internal/app/context.go internal/cli/root.go internal/cli/config.go internal/app/service_test.go internal/cli/root_test.go tests/integration/cli_test.go
git commit -m "feat: 按用户和 workspace 隔离 active context"
```

### Task 7: Implement User, Workspace, Member, and Audit App APIs

**Files:**
- Create: `internal/app/workspace.go`
- Modify: `internal/app/audit.go`
- Modify: `internal/app/service.go`
- Test: `internal/app/service_test.go`

- [ ] **Step 1: Write failing app API tests**

Cover:

- `AddUser` creates user + private personal workspace + owner membership.
- `UseUser` writes `active_user_id`.
- `UseUser` ignores `ServiceOptions.WorkspaceRef`; `taskg --workspace work user use alice` must not set Alice's active workspace. CLI may reject that combination later, but app semantics are "switch user only".
- `AddWorkspace` creates workspace with current actor as owner.
- `UseWorkspace` writes `active_workspace.<user_id>`.
- `ModifyWorkspace` requires admin/owner and audits `workspace.modify`.
- `ArchiveWorkspace` refuses if any affected user lacks another unarchived workspace.
- `AddMember` and `ChangeMemberRole` enforce owner/admin rules and last-owner protection.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/app -run 'User|Workspace|Member|Archive|LastOwner' -count=1`

Expected: FAIL because app APIs do not exist.

- [ ] **Step 3: Implement user APIs**

```go
func (s *Service) ListUsers() ([]UserView, error)
func (s *Service) AddUser(input AddUserInput) (UserView, error)
func (s *Service) UseUser(ref string) error
func (s *Service) UserInfo(ref string) (UserView, error)
```

`AddUser` must run in one transaction:

- Create user.
- Create personal workspace with `visibility=private`.
- Create owner membership.
- Set user default workspace.
- Audit `user.add` and `workspace.add`.

`UseUser` writes `active_user_id` and audits `user.use`. It does not require workspace membership beyond resolving that user's default/active workspace.

`UseUser` must ignore any workspace override on the current service. It switches active user only; it must not opportunistically set the target user's active workspace.

- [ ] **Step 4: Implement workspace APIs**

```go
func (s *Service) ListWorkspaces(includeArchived bool) ([]WorkspaceView, error)
func (s *Service) AddWorkspace(input AddWorkspaceInput) (WorkspaceView, error)
func (s *Service) UseWorkspace(ref string) error
func (s *Service) WorkspaceInfo(ref string) (WorkspaceView, error)
func (s *Service) ModifyWorkspace(ref string, input ModifyWorkspaceInput) error
func (s *Service) ArchiveWorkspace(ref string) error
```

Archive algorithm:

1. Require owner.
2. Find all affected users: `default_workspace_id = target` OR `active_workspace.<user_id>` points to target.
3. For each affected user, find other unarchived membership workspaces ordered by slug, explicitly excluding target.
4. If any user has none, abort.
5. For affected users, update default workspace.
6. If active meta points to target for any affected user, update it to that user's chosen replacement.
7. Archive target.
8. Audit `workspace.archive`.

- [ ] **Step 5: Implement member APIs**

```go
func (s *Service) ListMembers(workspaceRef string) ([]MemberView, error)
func (s *Service) AddMember(input AddMemberInput) error
func (s *Service) ChangeMemberRole(input ChangeMemberRoleInput) error
```

Rules:

- User refs support name/email/UUID.
- Admin can add/change viewer/member/admin but not owner.
- Owner can assign owner.
- Cannot downgrade last owner.
- No member delete.

- [ ] **Step 6: Run app API tests**

Run: `go test ./internal/app -run 'User|Workspace|Member|Archive|LastOwner' -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/app/workspace.go internal/app/service.go internal/app/audit.go internal/app/service_test.go
git commit -m "feat: 增加本地团队 app 接口"
```

---

## Chunk 4: CLI Command Groups

### Task 8: Add User and Workspace CLI Commands

**Files:**
- Create: `internal/cli/user.go`
- Create: `internal/cli/workspace.go`
- Modify: `internal/cli/root.go`
- Test: `tests/integration/cli_test.go`

- [ ] **Step 1: Write failing CLI integration tests**

Add tests:

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

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./tests/integration -run 'TestCLIUserWorkspaceLifecycle' -count=1`

Expected: FAIL because commands do not exist.

- [ ] **Step 3: Implement user commands**

`user list`:

- Human columns: ACTIVE, NAME, EMAIL, DEFAULT.
- JSON array when `--json`.

`user add`:

- Parse `email:<email>`.
- Print created user in human mode.

`user use`:

- Call app, print nothing or `Using user <name>`; prefer a short human confirmation.
- `taskg --workspace work user use alice` must ignore `--workspace`; it only switches active user and must not set Alice's active workspace.

`user info`:

- Default current actor when no arg.
- JSON when `--json`.

- [ ] **Step 4: Implement workspace commands**

`workspace list [--all]`, `add`, `use`, `info`, `modify`, `archive`.

Parse modifiers with existing local patterns from `add/modify` command parsers; do not invent a config DSL.

Use JSON output for scripts.

- [ ] **Step 5: Register commands**

In root:

```go
cmd.AddCommand(newUserCommand(opts))
cmd.AddCommand(newWorkspaceCommand(opts))
```

Update `knownSubcommands` tests if needed.

- [ ] **Step 6: Run user/workspace CLI tests**

Run: `go test ./tests/integration -run 'TestCLIUserWorkspaceLifecycle' -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/cli/user.go internal/cli/workspace.go internal/cli/root.go tests/integration/cli_test.go
git commit -m "feat: 增加 user 和 workspace 命令"
```

### Task 9: Add Member and Audit CLI Commands

**Files:**
- Create: `internal/cli/member.go`
- Create: `internal/cli/audit.go`
- Modify: `internal/cli/root.go`
- Test: `tests/integration/cli_test.go`

- [ ] **Step 1: Write failing integration tests**

Cover:

- `--workspace missing list` exits non-zero with `workspace_not_found`.
- `--workspace <archived> list` exits non-zero with `workspace_archived`.
- `--workspace <non-member-workspace> list` exits non-zero with `membership_not_found` or `permission_denied`.
- `member add bob role:viewer`.
- Viewer can list but cannot add task.
- Member cannot manage members.
- Admin cannot archive workspace.
- Owner can archive when another workspace exists.
- `audit list --json` contains `task.add`, `member.add`, and `workspace.modify`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./tests/integration -run 'TestCLIWorkspaceErrorSemantics|TestCLIMemberPermissions|TestCLIAuditList' -count=1`

Expected: FAIL because commands do not exist or permissions not wired through CLI.

- [ ] **Step 3: Implement member commands**

`member list [--workspace]`:

- For `--workspace`, reuse global `--workspace` if possible. If command-local flag is needed, pass it to app input without mutating global opts.
- Human columns: USER, EMAIL, ROLE, JOINED.

`member add <user> [role:<role>]`.

`member role <user> <role>`.

- [ ] **Step 4: Implement audit command**

`audit list [--limit N] [--workspace <ref>]`.

Rules:

- Default limit 50.
- Human newest first.
- JSON array with fields from spec.
- JSON `payload` field should be an object decoded from `payload_json`; if payload is empty or cannot be decoded, output `null`.
- No complex filters.

- [ ] **Step 5: Register commands and JSON errors**

Register `member` and `audit`.

If existing root error renderer does not support JSON errors, add minimal handling for new command errors only if not too invasive. Otherwise return normal errors and note broader JSON error unification as a follow-up; do not block M4.

- [ ] **Step 6: Run member/audit tests**

Run: `go test ./tests/integration -run 'TestCLIWorkspaceErrorSemantics|TestCLIMemberPermissions|TestCLIAuditList' -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/cli/member.go internal/cli/audit.go internal/cli/root.go tests/integration/cli_test.go
git commit -m "feat: 增加 member 和 audit 命令"
```

---

## Chunk 5: Workspace Isolation and Existing Command Wiring

### Task 10: Ensure All Existing CLI Paths Respect Runtime Workspace

**Files:**
- Modify: `internal/cli/root.go`
- Modify: `internal/app/service.go`
- Modify: `internal/app/context.go`
- Modify: `internal/app/uda.go`
- Test: `tests/integration/cli_test.go`
- Test: `internal/storage/sqlite/query_scope_test.go`

- [ ] **Step 1: Write failing cross-workspace integration test**

Flow:

1. In default local workspace, add task "local task" with project `same` and tag `same`.
2. Create workspace `work`, use it.
3. Add task "work task" with same project/tag/UDA/context names.
4. Assert:
   - `taskg list` shows only work task.
   - `taskg --workspace local list` shows only local task.
   - `_projects`, `_tags`, `_unique estimate`, `_udas`, `_ids`, `_uuids`, `_get`, `_urgency` respect `--workspace`.
   - Working-set ID `1` resolves independently in each workspace.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./tests/integration -run 'TestCLIWorkspaceIsolation' -count=1`

Expected: FAIL until `--workspace` and runtime context are fully wired.

- [ ] **Step 3: Fix any command still bypassing service**

Search:

```bash
rg "store\\.LocalWorkspace|LocalWorkspace\\(|context\\.active" internal/app internal/cli
```

Fix:

- Any app business path using `LocalWorkspace`.
- Any config path reading old `context.active`.
- Any helper path not using `buildServiceFromCmd`.

`LocalWorkspace()` may still appear in storage migration/tests, but must not appear in app business code.

- [ ] **Step 4: Verify storage-level scoping**

Add or extend `query_scope_test.go` to assert UDA subqueries, tag/dependency/annotation subqueries include workspace predicates.

Existing tests already cover many cases; add a regression for UDA schema/current workspace if missing.

- [ ] **Step 5: Run isolation tests**

Run: `go test ./tests/integration -run 'TestCLIWorkspaceIsolation' -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal tests
git commit -m "fix: 贯通 workspace 隔离"
```

### Task 11: Update Automatic State and Recurrence Writes for Audit/Permissions

**Files:**
- Modify: `internal/app/service.go`
- Modify: `internal/app/audit.go`
- Test: `internal/app/service_test.go`

- [ ] **Step 1: Identify automatic writes**

Current automatic writes:

- `refreshAutomaticState` turns waiting into pending.
- `ensureRecurringChildren` creates next child.
- `Done` can create recurring child after completing a task.

- [ ] **Step 2: Decide audit treatment**

Use this rule:

- User-triggered command audit action remains the command action (`task.done`, `task.add`, etc.).
- Internal automatic writes inside that command occur in the same transaction when practical, but do not need separate public audit actions unless already listed in spec.
- Automatic state maintenance bypasses user permission checks. A viewer running `list` must not fail just because `refreshAutomaticStateLocked` advances a waiting task to pending.
- Automatic maintenance must remain workspace-scoped and must not expose or modify another workspace.
- If recurring child creation is reachable from a read path, it also bypasses permission checks as internal maintenance. If this feels too broad during implementation, narrow the trigger so read paths do not create recurring children, but do not make viewer reads fail.

- [ ] **Step 3: Add tests for recurring child with audit**

Test `Done` on a recurring child creates `task.done` audit and keeps next child in same workspace.

- [ ] **Step 4: Refactor transaction boundaries if needed**

If `Done` updates task and creates next child in separate writes, wrap both plus audit in `withAudit`.

Avoid double-auditing internal child creation.

- [ ] **Step 5: Run recurrence/app tests**

Run: `go test ./internal/app ./internal/recurrence -run 'Recurring|Audit|Done' -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/app/service.go internal/app/audit.go internal/app/service_test.go
git commit -m "feat: 完善循环任务审计路径"
```

---

## Chunk 6: Documentation, Roadmap, and Full Verification

### Task 12: Update README and ROADMAP

**Files:**
- Modify: `README.md`
- Modify: `ROADMAP.md`
- Modify: `docs/superpowers/specs/2026-05-29-taskg-m4-design.md` only if implementation changes the spec.

- [ ] **Step 1: Update README M4 usage**

Add section:

- `user list/add/use/info`
- `workspace list/add/use/info/modify/archive`
- `--workspace`
- `member list/add/role`
- `audit list`
- Role summary and warning: M4 has no `member delete`; viewer still reads workspace data.
- Local user email for migrated `local` user is empty/null.
- M3 to M4 upgrade behavior: existing tasks stay in local workspace, local user/workspace/membership are created automatically, and old `context.active` meta is migrated to `(local user, local workspace)` scoped active context.

- [ ] **Step 2: Update ROADMAP**

Mark M4 complete and add delivered bullets:

- Users/workspaces/memberships.
- Runtime context.
- Permissions.
- Audit logs.
- Workspace isolation for tasks/context/UDA/helpers.

Set next step to M5.

- [ ] **Step 3: Run doc diff check**

Run: `git diff --check`

Expected: no output.

- [ ] **Step 4: Commit**

```bash
git add README.md ROADMAP.md docs/superpowers/specs/2026-05-29-taskg-m4-design.md
git commit -m "docs: 更新 M4 使用说明"
```

### Task 13: Full M4 Verification

**Files:**
- No planned edits unless verification reveals issues.

- [ ] **Step 1: Run unit and integration tests**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 2: Run CGO-free tests**

Run:

```bash
CGO_ENABLED=0 go test ./...
```

Expected: PASS.

- [ ] **Step 3: Run CGO-free build**

Run:

```bash
CGO_ENABLED=0 go build ./cmd/taskg
```

Expected: PASS.

- [ ] **Step 4: Run focused CLI integration**

Run:

```bash
go test ./tests/integration -run TestCLI -count=1
```

Expected: PASS.

- [ ] **Step 5: Inspect final git status**

Run:

```bash
git status --short
git log --oneline -8
```

Expected: clean worktree except optional local build artifact `taskg`. If `taskg` exists and is untracked, remove it only if confirmed it was generated by the build command in this task.

- [ ] **Step 6: Final commit if verification fixes were needed**

If verification required fixes:

```bash
git add <fixed-files>
git commit -m "fix: 完成 M4 验证收尾"
```

---

## Review Checklist Before Execution

Before implementing, read:

- [M4 spec](/Users/mac/code/projects/dajee/task/docs/superpowers/specs/2026-05-29-taskg-m4-design.md)
- [AGENTS.md](/Users/mac/code/projects/dajee/task/AGENTS.md)
- [internal/app/service.go](/Users/mac/code/projects/dajee/task/internal/app/service.go)
- [internal/storage/sqlite/db.go](/Users/mac/code/projects/dajee/task/internal/storage/sqlite/db.go)
- [internal/cli/root.go](/Users/mac/code/projects/dajee/task/internal/cli/root.go)

Implementation must preserve:

- `github.com/glebarez/sqlite` only; no CGO SQLite driver.
- stdout/stderr separation.
- Stable `--json` output for new commands.
- Workspace-scoped numeric working-set IDs.
- No service business-path dependency on `store.LocalWorkspace()`.
- No old persistent `context.active` key.
- Audit writes in the same store-level transaction as the write operation.
