# taskg M8 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现 M8：为 `taskg server` 增加 server-side post-commit webhook Hook 能力，并补齐投递恢复、CLI/HTTP 管理面、audit、OpenAPI、部署与恢复文档。

**Architecture:** M8 在现有 `internal/app` 事务边界内生成稳定事件和 delivery outbox；后台 dispatcher 从 SQLite 抢占待投递记录并发送 HTTP webhook。Hook 管理继续复用 `internal/app`、HTTP scoped service、远程 CLI 和 audit，不新增业务域 adapter、不引入 memory / replica / sync，也不把 Hook 写操作暴露到 MCP。

**Tech Stack:** Go 1.25、Cobra、GORM、`github.com/glebarez/sqlite`（保持 `CGO_ENABLED=0`）、`net/http`、`github.com/go-chi/chi/v5`、OpenAPI 3 YAML、Go test / httptest / CLI integration tests。

---

## 范围锁定

严格按 [M8 spec](/Users/mac/code/projects/dajee/task/docs/superpowers/specs/2026-06-02-taskg-m8-design.md) 实现。

必须进入 M8：

- server-side post-commit webhook Hook runtime。
- workspace / project 两级 Hook definition。
- `task.created`、`task.modified`、`task.completed`、`task.deleted`、`project.archived`。
- durable delivery queue / outbox。
- dispatcher、timeout、retry、dead-letter、disable、manual replay。
- Webhook HMAC-SHA256 签名。
- Hook 管理 CLI、本地与远程模式。
- Hook 管理 HTTP API 与 OpenAPI。
- Hook 配置变更和 manual replay audit。
- delivery 运行记录，不把自动投递尝试写进业务 audit。
- 服务端部署、backup / restore、release build 文档。

明确不进入 M8：

- 飞书 / GitHub / Jira / Slack 等业务域 adapter。
- agent memory / workspace memory / personal memory。
- replica / sync / operation log 同步。
- 本地 CLI shell hook。
- scheduler 平台。
- `schedule` / `heartbeat` / `one-shot` trigger。
- pre-commit hook。
- 外部 webhook 失败回滚本地 task/project 事务。
- MCP Hook 写操作。

## 执行注意事项

- 每个 chunk 完成后至少跑局部测试；跨 chunk 后跑完整验证。
- 不要在 `internal/cli` 或 `internal/httpapi` 复制 Hook 业务规则；需要业务能力就补 `internal/app`。
- Hook delivery 必须和业务写操作在同一个 SQLite transaction 内落盘；outbox enqueue 失败时业务写入也必须回滚。
- 本计划中的 “post-commit” 只指外部 HTTP webhook 投递在 SQLite commit 之后异步发生，不指业务写入和 outbox 分离提交。
- Dispatcher 可以直接使用 storage repo 处理抢占和状态更新，但不能承担 Hook 管理权限判断。
- Hook secret 是 write-only；任何 list/info/audit/log 都不能回显 secret 原文。M8 首版为 HMAC 签名会在 SQLite 中保存可用于签名的 secret 值，安全边界依赖数据库文件和备份文件权限。
- 远程 CLI 必须走 HTTP API，不得在 remote mode 触碰客户端本机 DB。
- OpenAPI 只描述 REST API；MCP 不增加 Hook tool。
- 所有新增 SQLite 结构必须保持 `github.com/glebarez/sqlite` 与 `CGO_ENABLED=0`。

## 文件结构

新增文件：

- `internal/app/hook.go`
  Hook definition 的 app 输入、view、增删改查、enable/disable、delivery list/info、manual replay。
- `internal/app/hook_event.go`
  App event 类型、payload builder、事件到 delivery outbox 的事务内 enqueue。
- `internal/app/hook_test.go`
  Hook 管理、权限、secret 不泄露、事件 enqueue 的 app 层测试。
- `internal/storage/sqlite/hook_repo.go`
  Hook definition repository。
- `internal/storage/sqlite/hook_delivery_repo.go`
  Delivery repository、claim、stale delivering 恢复、retry/dead-letter/success 状态更新。
- `internal/storage/sqlite/hook_repo_test.go`
  Hook repository 与 migration 测试。
- `internal/hookruntime/dispatcher.go`
  后台 dispatcher、`RunOnce`、抢占 delivery、发送 webhook、状态更新。
- `internal/hookruntime/signer.go`
  HMAC-SHA256 签名与 headers。
- `internal/hookruntime/dispatcher_test.go`
  httptest webhook、retry、dead-letter、disable、签名测试。
- `internal/httpapi/hooks.go`
  Hook REST handlers。
- `internal/httpapi/hooks_test.go`
  Hook HTTP API、scope、capability、secret 不回显、replay audit 测试。
- `internal/remote/hook.go`
  远程 Hook client。
- `internal/cli/hook.go`
  `taskg hook ...` 命令组。
- `docs/deployment.md`
  服务端部署、reverse proxy、outbound webhook、secret 注意事项。
- `docs/backup-restore.md`
  SQLite backup / restore 演练。
- `scripts/release-build.sh`
  CGO-free 多平台构建脚本。

修改文件：

- `internal/storage/sqlite/models.go`
  增加 `HookDefinition`、`HookDelivery` model。
- `internal/storage/sqlite/db.go`
  AutoMigrate Hook models；必要时补轻量 schema prepare。
- `internal/app/service.go`
  增加 hook repos；`withStore` 绑定事务内 repo。
- `internal/app/audit.go`
  增加事务内 audit + hook event outbox helper。
- `internal/app/permission.go`
  增加 `PermissionHookRead`、`PermissionHookWrite`，admin/owner 可管理。
- `internal/app/project.go`
  `ArchiveProject` 生成 `project.archived` delivery。
- `internal/app/service.go`
  task add/modify/done/delete 生成对应 delivery。
- `internal/httpapi/router.go`
  注册 `/api/v1/hooks*` endpoint。
- `internal/httpapi/envelope.go`
  新增 Hook 相关错误码 HTTP status 映射。
- `internal/httpapi/server.go`
  如需要为测试注入 dispatcher clock/client，则补 Options。
- `internal/cli/root.go`
  注册 `hook` 命令组。
- `internal/cli/server.go`
  启动 `taskg server` 时启动 Hook dispatcher，并随 server shutdown 停止。
- `docs/openapi/taskg-v1.yaml`
  增加 Hook API schema、endpoint、错误码、capability 说明。
- `README.md`
  增加 Hook 使用示例、部署/恢复文档链接。
- `ROADMAP.md`
  M8 状态与验收同步。
- `tests/integration/cli_test.go`
  Hook CLI 黑盒测试、remote CLI 不触碰本地 DB。

---

## Chunk 1: SQLite 模型与 Repository

### Task 1: 增加 Hook models 与 migration

**Files:**
- Modify: `internal/storage/sqlite/models.go`
- Modify: `internal/storage/sqlite/db.go`
- Test: `internal/storage/sqlite/db_test.go`

- [ ] **Step 1: 写失败测试，验证迁移创建 Hook 表与索引**

在 `internal/storage/sqlite/db_test.go` 新增测试：

```go
func TestM8HookTablesMigrated(t *testing.T) {
	store := openTestStore(t)
	db := store.DB()
	assertColumnExists(t, db, "hook_definitions", "id")
	assertColumnExists(t, db, "hook_definitions", "workspace_id")
	assertColumnExists(t, db, "hook_definitions", "project_id")
	assertColumnExists(t, db, "hook_deliveries", "id")
	assertColumnExists(t, db, "hook_deliveries", "workspace_id")
	assertColumnExists(t, db, "hook_deliveries", "project_id")
	assertColumnExists(t, db, "hook_deliveries", "actor_user_id")
	assertColumnExists(t, db, "hook_deliveries", "status")
	assertColumnExists(t, db, "hook_deliveries", "next_attempt_at")
	assertColumnExists(t, db, "hook_deliveries", "claim_expires_at")
}
```

- [ ] **Step 2: 运行红测**

Run: `go test ./internal/storage/sqlite -run TestM8HookTablesMigrated`

Expected: FAIL，提示 `hook_definitions` 或 `hook_deliveries` 缺失。

- [ ] **Step 3: 增加 GORM models**

在 `internal/storage/sqlite/models.go` 增加：

```go
type HookDefinition struct {
	ID             string  `gorm:"primaryKey"`
	Name           string  `gorm:"not null"`
	ScopeType      string  `gorm:"not null;index:idx_hooks_scope,priority:1"`
	WorkspaceID    string  `gorm:"not null;index:idx_hooks_scope,priority:2;index:idx_hooks_enabled"`
	ProjectID      *string `gorm:"index:idx_hooks_scope,priority:3"`
	ActorUserID    string  `gorm:"not null;index"`
	EventTypesJSON string  `gorm:"not null"`
	EndpointURL    string  `gorm:"not null"`
	Secret         string  `gorm:"not null;default:''"`
	Enabled        bool    `gorm:"not null;default:true;index:idx_hooks_enabled"`
	TimeoutSeconds int     `gorm:"not null;default:10"`
	MaxAttempts    int     `gorm:"not null;default:5"`
	CreatedAt      int64   `gorm:"not null"`
	ModifiedAt     int64   `gorm:"not null"`
}

type HookDelivery struct {
	ID             string  `gorm:"primaryKey"`
	HookID         string  `gorm:"not null;index:idx_deliveries_due,priority:2"`
	EventID        string  `gorm:"not null;index"`
	EventType      string  `gorm:"not null;index"`
	WorkspaceID    string  `gorm:"not null;index"`
	ProjectID      *string `gorm:"index"`
	ActorUserID    string  `gorm:"not null;index"`
	PayloadJSON    string  `gorm:"not null"`
	HeadersJSON    string  `gorm:"not null;default:'{}'"`
	Status         string  `gorm:"not null;index:idx_deliveries_due,priority:1"`
	AttemptCount   int     `gorm:"not null;default:0"`
	NextAttemptAt  *int64  `gorm:"index:idx_deliveries_due,priority:3"`
	ClaimExpiresAt *int64  `gorm:"index"`
	LastAttemptAt  *int64
	LastStatusCode *int
	LastError      string
	CreatedAt      int64 `gorm:"not null;index"`
	ModifiedAt     int64 `gorm:"not null"`
}
```

- [ ] **Step 4: AutoMigrate Hook models**

在 `Store.migrate()` 的核心 AutoMigrate 中加入 `&HookDefinition{}`、`&HookDelivery{}`。

- [ ] **Step 5: 运行绿测**

Run: `go test ./internal/storage/sqlite -run TestM8HookTablesMigrated`

Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add internal/storage/sqlite/models.go internal/storage/sqlite/db.go internal/storage/sqlite/db_test.go
git commit -m "feat: 增加 Hook 持久化表"
```

### Task 2: 实现 Hook repository

**Files:**
- Create: `internal/storage/sqlite/hook_repo.go`
- Create: `internal/storage/sqlite/hook_delivery_repo.go`
- Test: `internal/storage/sqlite/hook_repo_test.go`

- [ ] **Step 1: 写 HookDefinition CRUD 红测**

覆盖：

- create 后 list by workspace。
- project scoped hook 只在相同 project 查询命中。
- update 不改变 secret 时保留旧 secret。
- delete 后 info 返回 `sqlite.ErrNotFound`。

Run: `go test ./internal/storage/sqlite -run TestHookRepository`

Expected: FAIL，repo 未定义。

- [ ] **Step 2: 实现 `HookRepository`**

接口形态：

```go
type HookRepository struct { db *gorm.DB }

func NewHookRepository(db *gorm.DB) *HookRepository
func (r *HookRepository) Create(row HookDefinition) error
func (r *HookRepository) GetByID(id string) (HookDefinition, error)
func (r *HookRepository) List(workspaceID string, projectID *string, includeDisabled bool) ([]HookDefinition, error)
func (r *HookRepository) ListMatching(workspaceID string, projectID *string, eventType string) ([]HookDefinition, error)
func (r *HookRepository) Update(row HookDefinition) error
func (r *HookRepository) Delete(id string) error
```

`ListMatching` 先用 workspace/project/enabled 做 SQL 过滤，再在 Go 中解析 `EventTypesJSON` 判断 event type，避免 JSON 函数绑定到 SQLite 方言。

- [ ] **Step 3: 写 delivery repo 红测**

覆盖：

- enqueue 后 status 为 `queued`。
- enqueue 写入 `workspace_id`、`project_id`、`actor_user_id` scope snapshot。
- `ClaimDue` 只抢 `queued/retry_wait` 且到期记录。
- `ClaimDue` 设置 `status=delivering` 和 `claim_expires_at`。
- stale `delivering` 可以恢复为 `queued`。
- success 更新 `succeeded`。
- retry 更新 `retry_wait`、`attempt_count`、`next_attempt_at`。
- dead-letter 更新 `dead_lettered`。
- disabled hook pending delivery 更新 `disabled_skipped`。

Run: `go test ./internal/storage/sqlite -run TestHookDeliveryRepository`

Expected: FAIL，repo 未定义。

- [ ] **Step 4: 实现 `HookDeliveryRepository`**

接口形态：

```go
type HookDeliveryRepository struct { db *gorm.DB }

func NewHookDeliveryRepository(db *gorm.DB) *HookDeliveryRepository
func (r *HookDeliveryRepository) Enqueue(rows []HookDelivery) error
func (r *HookDeliveryRepository) GetByID(id string) (HookDelivery, error)
func (r *HookDeliveryRepository) ListByHook(hookID string, status string, limit int) ([]HookDelivery, error)
func (r *HookDeliveryRepository) ClaimDue(now int64, claimExpiresAt int64, limit int) ([]HookDelivery, error)
func (r *HookDeliveryRepository) RecoverStaleDelivering(now int64) (int64, error)
func (r *HookDeliveryRepository) MarkSucceeded(id string, now int64, statusCode int) error
func (r *HookDeliveryRepository) MarkRetry(id string, now int64, nextAttemptAt int64, statusCode *int, message string) error
func (r *HookDeliveryRepository) MarkDeadLettered(id string, now int64, statusCode *int, message string) error
func (r *HookDeliveryRepository) MarkDisabledSkipped(id string, now int64) error
func (r *HookDeliveryRepository) Requeue(id string, now int64) error
```

`ClaimDue` 的 M8 首版实现可以是 SQLite transaction 内 select + update 到 `delivering`。不要求跨多 server 完美公平，但同一进程内测试必须证明同一 delivery 不会被连续 claim 两次。`RecoverStaleDelivering` 将 `status=delivering AND claim_expires_at < now` 的记录重置为 `queued`，同时清空 `claim_expires_at`。

- [ ] **Step 5: 运行 repository 测试**

Run: `go test ./internal/storage/sqlite -run 'TestHookRepository|TestHookDeliveryRepository'`

Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add internal/storage/sqlite/hook_repo.go internal/storage/sqlite/hook_delivery_repo.go internal/storage/sqlite/hook_repo_test.go
git commit -m "feat: 增加 Hook 仓储"
```

---

## Chunk 2: App Hook 管理与权限

### Task 3: 增加 Hook 权限与 capability

**Files:**
- Modify: `internal/app/permission.go`
- Modify: `README.md`
- Modify: `docs/openapi/taskg-v1.yaml`
- Test: `internal/app/service_test.go`

- [ ] **Step 1: 写权限红测**

在 `internal/app/service_test.go` 新增测试：

- admin 可以创建 Hook。
- member/viewer 创建 Hook 返回 `permission_denied`。

Run: `go test ./internal/app -run TestHookPermission`

Expected: FAIL，Hook 方法或权限不存在。

- [ ] **Step 2: 增加权限常量**

在 `internal/app/permission.go` 增加：

```go
PermissionHookRead  Permission = "hook.read"
PermissionHookWrite Permission = "hook.write"
```

`RoleOwner` 继续全权限；`RoleAdmin` 加入 `PermissionHookRead`、`PermissionHookWrite`；member/viewer 不加入。

- [ ] **Step 3: 文档登记 capability**

在 README 常用 capability 和 OpenAPI 安全说明中加入：

```text
hook:read hook:write
```

HTTP handler 后续用 `hook:read` / `hook:write` 做 token capability 收窄。

- [ ] **Step 4: 运行权限测试**

Run: `go test ./internal/app -run TestHookPermission`

Expected: PASS。

### Task 4: 实现 app Hook 管理方法

**Files:**
- Create: `internal/app/hook.go`
- Create: `internal/app/hook_endpoint.go`
- Modify: `internal/app/service.go`
- Test: `internal/app/hook_test.go`

- [ ] **Step 1: 写 app CRUD 红测**

覆盖：

- `AddHook` 默认 `enabled=true`、`timeout_seconds=10`、`max_attempts=5`。
- `ListHooks` 不返回 secret 原文。
- `HookInfo` 不返回 secret 原文。
- `ModifyHook` 只有传入新 secret 时才更新 secret。
- `AddHook` / `ModifyHook` audit 不含 secret 原文，但包含 `secret_fingerprint=sha256(secret)[:8]`。
- `ModifyHook` 不传 `EventTypes` 时保留原 event list；传入 `EventTypes` 时整体替换，不做追加。
- `DisableHook` / `EnableHook` 改变 enabled。
- `DeleteHook` 后 info 返回 `hook_not_found`。
- `AddHook` project scope 项目必须属于当前 workspace。
- `timeout_seconds` 只接受 `1..120`。
- `max_attempts` 只接受 `1..20`。
- `endpoint_url` 拒绝 loopback / link-local / RFC1918 / RFC6598 / multicast / unspecified host。

Run: `go test ./internal/app -run TestHook`

Expected: FAIL。

- [ ] **Step 2: 定义 app input/view**

在 `internal/app/hook.go` 定义：

```go
type HookScopeType string

const (
	HookScopeWorkspace HookScopeType = "workspace"
	HookScopeProject   HookScopeType = "project"
)

type HookAddInput struct {
	Name           string
	ScopeType      HookScopeType
	ProjectRef     string
	EventTypes     []string
	EndpointURL    string
	Secret         string
	TimeoutSeconds int
	MaxAttempts    int
}

type HookModifyInput struct {
	Name           *string
	EventTypes     *[]string
	EndpointURL    *string
	Secret         *string
	TimeoutSeconds *int
	MaxAttempts    *int
}

type HookView struct {
	ID             string
	Name           string
	ScopeType      string
	WorkspaceID    string
	ProjectID      *string
	ActorUserID    string
	EventTypes     []string
	EndpointURL    string
	Enabled        bool
	TimeoutSeconds int
	MaxAttempts    int
	CreatedAt      int64
	ModifiedAt     int64
}

type HookDeliveryView struct {
	ID             string
	HookID         string
	EventID        string
	EventType      string
	WorkspaceID    string
	ProjectID      *string
	ActorUserID    string
	Payload        map[string]any
	Headers        map[string]string
	Status         string
	AttemptCount   int
	NextAttemptAt  *int64
	ClaimExpiresAt *int64
	LastAttemptAt  *int64
	LastStatusCode *int
	LastError      string
	CreatedAt      int64
	ModifiedAt     int64
}
```

不要在 `HookView` 或 `HookDeliveryView` 中暴露 secret 原文。`HookModifyInput.EventTypes == nil` 表示保留原列表；非 nil 表示用传入列表整体替换，传入空列表应返回参数错误。

- [ ] **Step 3: Service 增加 repos**

在 `Service` 中增加：

```go
hookRepo         *sqlite.HookRepository
hookDeliveryRepo *sqlite.HookDeliveryRepository
```

在 `NewService` 和 `withStore` 中初始化。

- [ ] **Step 4: 实现 CRUD 方法**

方法列表：

```go
func (s *Service) AddHook(input HookAddInput) (HookView, error)
func (s *Service) ListHooks(projectRef string, includeDisabled bool) ([]HookView, error)
func (s *Service) HookInfo(id string) (HookView, error)
func (s *Service) ModifyHook(id string, input HookModifyInput) (HookView, error)
func (s *Service) EnableHook(id string) (HookView, error)
func (s *Service) DisableHook(id string) (HookView, error)
func (s *Service) DeleteHook(id string) error
```

规则：

- read 方法 `Require(PermissionHookRead)`。
- write 方法 `Require(PermissionHookWrite)`。
- write 方法必须 audit：`hook.create`、`hook.modify`、`hook.enable`、`hook.disable`、`hook.delete`。
- audit payload 不包含 secret。
- create / secret modify 的 audit payload 包含不可逆短指纹 `secret_fingerprint`。
- event type 只能是 M8 spec 允许集合。
- endpoint URL 只接受 `http://` 或 `https://`，并通过 `ValidateWebhookEndpointURL` 做 SSRF 防护。
- `timeout_seconds` 与 `max_attempts` 必须按 M8 spec 范围校验。

- [ ] **Step 5: 实现 endpoint URL 安全校验**

在 `internal/app/hook_endpoint.go` 增加：

```go
type HookHostResolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

func ValidateWebhookEndpointURL(raw string, resolver HookHostResolver) error
```

规则：

- scheme 必须是 `http` 或 `https`。
- host 必须存在，不能是裸空字符串。
- 解析出的 IP 不能是 loopback、link-local、private RFC1918、RFC6598、multicast、unspecified。
- 域名在 create / modify 时解析一次；dispatcher 发送前还必须再次解析。
- M8 不提供默认内网 allowlist；如果未来要允许内网投递，应新增明确配置项和测试。

- [ ] **Step 6: 运行 app Hook 测试**

Run: `go test ./internal/app -run TestHook`

Expected: PASS。

- [ ] **Step 7: 提交**

```bash
git add internal/app/permission.go internal/app/service.go internal/app/hook.go internal/app/hook_endpoint.go internal/app/hook_test.go README.md docs/openapi/taskg-v1.yaml
git commit -m "feat: 增加 Hook 管理用例"
```

---

## Chunk 3: 事件生成与 Outbox 落盘

### Task 5: 增加 app event builder 与事务内 enqueue

**Files:**
- Create: `internal/app/hook_event.go`
- Modify: `internal/app/audit.go`
- Test: `internal/app/hook_test.go`

- [ ] **Step 1: 写红测，业务写入和 delivery 同事务**

新增测试：

- 创建 workspace hook 订阅 `task.created`。
- 调用 `svc.Add(...)`。
- 查询 delivery，确认有一条 `queued`，payload 包含 `event_type=task.created` 和 task snapshot。
- 注入失败 delivery repo 后，`svc.Add(...)` 返回错误且任务未创建。

Run: `go test ./internal/app -run TestHookDeliveryEnqueuedWithTaskTransaction`

Expected: FAIL。

- [ ] **Step 2: 定义事件结构**

在 `internal/app/hook_event.go`：

```go
type HookEvent struct {
	EventID       string
	EventType     string
	EventVersion  int
	OccurredAt    int64
	ActorUserID   string
	WorkspaceID   string
	WorkspaceSlug string
	ProjectID     *string
	ProjectSlug   *string
	ObjectKind    string
	ObjectID      string
	Data          map[string]any
}
```

提供：

```go
func (s *Service) enqueueHookEvents(events []HookEvent) error
func (s *Service) buildTaskHookEvent(eventType string, tsk task.Task) (HookEvent, error)
func (s *Service) buildProjectHookEvent(eventType string, project ProjectView) (HookEvent, error)
```

payload 结构必须固定：

```go
func buildTaskPayload(tsk task.Task) map[string]any {
	return map[string]any{
		"task":      task.ToJSON(tsk),
		"completed": tsk.Status == task.StatusCompleted,
		"deleted":   tsk.Status == task.StatusDeleted,
	}
}

func buildProjectArchivedPayload(pv ProjectView) map[string]any {
	return map[string]any{
		"project":  projectViewToMap(pv),
		"archived": true,
	}
}
```

`projectViewToMap` 只能包含已有 public `ProjectView` 字段，不得把 project config、agent secret 或内部 GORM model 原样塞进 payload。增加 payload schema golden test，冻结 `data.task` v1 字段集合；Hook payload 字段删除、重命名、类型变化必须修改测试并同步 bump `EventVersion`。

- [ ] **Step 3: 扩展 audit transaction helper**

保留现有 `withAuditEntries` / `withAudit` 调用方式，新增：

```go
func (s *Service) withAuditEntriesAndEvents(fn func(*Service) ([]AuditEntry, []HookEvent, error)) error
func (s *Service) withAuditAndEvents(fn func(*Service) (*AuditEntry, []HookEvent, error)) error
```

`withAuditEntries` 和 `withAudit` 改成调用新 helper，并传空 events，避免一次性重写所有旧用例。Start / Stop / Annotate 当前走 `withAudit`，不能只改 `withAuditEntries`。

- [ ] **Step 4: enqueue 匹配 Hook**

`enqueueHookEvents` 对每个 event：

- 用 workspaceID/projectID/eventType 查 matching hooks。
- disabled hook 不生成新 delivery。
- 每个 matched hook 生成一个 `HookDelivery`。
- delivery 写入 `workspace_id`、`project_id`、`actor_user_id` scope snapshot。
- `payload_json` 保存完整 event envelope。
- `headers_json` 保存 `X-Taskg-Event`、`X-Taskg-Event-Id`、`X-Taskg-Event-Version`，签名在 dispatcher 发送时计算。

匹配伪代码：

```go
for _, hook := range hooksInWorkspace(event.WorkspaceID) {
	if !hook.Enabled || !eventTypeAllowed(hook, event.EventType) {
		continue
	}
	if hook.ScopeType == "workspace" {
		match(hook)
		continue
	}
	if hook.ScopeType == "project" && event.ProjectID != nil && hook.ProjectID != nil && *hook.ProjectID == *event.ProjectID {
		match(hook)
	}
}
```

workspace 级 Hook 可以收到当前 workspace 内没有 project 归属的 task 事件；project 级 Hook 不匹配 `event.ProjectID == nil` 的事件。

- [ ] **Step 5: 运行事务测试**

Run: `go test ./internal/app -run TestHookDeliveryEnqueuedWithTaskTransaction`

Expected: PASS。

### Task 6: 为 task/project 写操作接入事件

**Files:**
- Modify: `internal/app/service.go`
- Modify: `internal/app/project.go`
- Test: `internal/app/hook_test.go`

- [ ] **Step 1: 写事件类型红测**

覆盖：

- `Add` 生成 `task.created`。
- `Modify` 生成 `task.modified`。
- `Start` 生成 `task.modified`。
- `Stop` 生成 `task.modified`。
- `Annotate` 生成 `task.modified`。
- `Done` 生成 `task.completed`。
- `Delete` 生成 `task.deleted`。
- `ArchiveProject` 生成 `project.archived`。
- project-scoped hook 只收到对应 project 事件。

Run: `go test ./internal/app -run TestHookEventsForWrites`

Expected: FAIL。

- [ ] **Step 2: 修改 `Add` / `AddWithAnnotations`**

改用 `withAuditEntriesAndEvents`，在 `addLocked` 成功后根据 created task 生成 `task.created`。

`AddWithAnnotations` 首版只要求产生 `task.created`；annotation 本身仍是 `task.modified` 候选，但 M8 v1 不强制为初始 annotations 生成额外事件。

- [ ] **Step 3: 修改 `Modify`**

`modifyLocked` 目前返回 targetID 和 projectChange。M8 中需要拿到更新后的 task snapshot。最小改法：

```go
func (s *Service) modifyLocked(target string, input ModifyInput) (task.Task, projectChange, error)
```

调用方用返回的 task UUID 生成 audit 和 `task.modified` event。

- [ ] **Step 4: 修改 `Start` / `Stop` / `Annotate`**

这些路径当前使用单条 audit helper。M8 中需要改为 `withAuditAndEvents`，并让 locked 方法返回更新后的 task snapshot：

```go
func (s *Service) startLocked(target string) (task.Task, projectChange, error)
func (s *Service) stopLocked(target string) (task.Task, projectChange, error)
func (s *Service) annotateLocked(target, description string) (task.Task, projectChange, error)
```

调用方用返回的 task 生成 `task.modified` event。

- [ ] **Step 5: 修改 `Done` / `Delete`**

`doneLocked` / `deleteLocked` 返回更新后的 task snapshot，生成 `task.completed` / `task.deleted`。

最小签名变更：

```go
func (s *Service) doneLocked(target string) (task.Task, projectChange, []AuditEntry, error)
func (s *Service) deleteLocked(target string) (task.Task, projectChange, error)
```

`doneLocked` 保留现有额外 `[]AuditEntry` 返回值，用于 recurring child warning 等附加 audit；只把原来的 target ID 字符串替换为更新后的 task snapshot。

保留 recurring child 的 audit warning 语义，不为自动 recurring child 生成 Hook 事件，除非实现阶段明确补测试证明语义稳定。

- [ ] **Step 6: 修改 `ArchiveProject`**

在 archive 成功后生成 `project.archived`，payload 使用 archived `ProjectView`。

- [ ] **Step 7: 运行 app 写入事件测试**

Run: `go test ./internal/app -run TestHookEventsForWrites`

Expected: PASS。

- [ ] **Step 8: 回归 app 测试**

Run: `go test ./internal/app`

Expected: PASS。

- [ ] **Step 9: 提交**

```bash
git add internal/app/audit.go internal/app/service.go internal/app/project.go internal/app/hook_event.go internal/app/hook_test.go
git commit -m "feat: 写操作生成 Hook delivery"
```

---

## Chunk 4: Dispatcher、签名与失败恢复

### Task 7: 实现签名与请求构造

**Files:**
- Create: `internal/hookruntime/signer.go`
- Test: `internal/hookruntime/dispatcher_test.go`

- [ ] **Step 1: 写签名红测**

验证：

- 有 secret 时生成 `X-Taskg-Signature-256`。
- 同一 body/secret/delivery_id/timestamp 签名稳定。
- 无 secret 时不生成签名。
- 有 secret 时生成 `X-Taskg-Timestamp`。
- headers 包含 `X-Taskg-Delivery`、`X-Taskg-Hook-Id`、`X-Taskg-Attempt`、`User-Agent`。
- replay 使用原始 payload/header，但用 Hook 当前 secret 重新计算签名。

Run: `go test ./internal/hookruntime -run TestWebhookSignature`

Expected: FAIL。

- [ ] **Step 2: 实现签名 helper**

```go
func SignatureSHA256(secret string, deliveryID string, timestamp int64, body []byte) string
func HeadersForDelivery(delivery sqlite.HookDelivery, hook sqlite.HookDefinition, body []byte, now int64, version string) (http.Header, error)
```

签名输入和格式固定为：

```text
<delivery_id>.<timestamp_unix_seconds>.<body>
```

```text
sha256=<hex>
```

最终 HTTP 请求 headers = delivery 入队时保存的 `headers_json` + runtime headers。runtime headers 包含 signature、timestamp、delivery id、hook id、attempt、User-Agent；合并时 runtime headers 覆盖同名 stored headers，避免旧 outbox 记录覆盖发送时生成的安全 header。

- [ ] **Step 3: 运行签名测试**

Run: `go test ./internal/hookruntime -run TestWebhookSignature`

Expected: PASS。

### Task 8: 实现 dispatcher RunOnce

**Files:**
- Create: `internal/hookruntime/dispatcher.go`
- Test: `internal/hookruntime/dispatcher_test.go`

- [ ] **Step 1: 写成功投递红测**

使用 `httptest.Server`：

- 插入 enabled hook + queued delivery。
- `RunOnce(ctx)`。
- webhook 收到 POST、JSON body、headers。
- delivery 变为 `succeeded`。

Run: `go test ./internal/hookruntime -run TestDispatcherRunOnceDeliversWebhook`

Expected: FAIL。

- [ ] **Step 2: 实现 dispatcher options**

```go
type DispatcherOptions struct {
	Store          *sqlite.Store
	Clock          app.Clock
	Client         *http.Client
	Resolver       app.HookHostResolver
	Version        string
	BatchSize      int
	PollInterval   time.Duration
	RetryBaseDelay time.Duration
	ClaimTTL       time.Duration
	JitterSeed     int64
}

type Dispatcher struct { ... }

func NewDispatcher(opts DispatcherOptions) *Dispatcher
func (d *Dispatcher) RunOnce(ctx context.Context) error
func (d *Dispatcher) Run(ctx context.Context) error
```

- [ ] **Step 3: 实现成功路径**

`RunOnce`：

- claim due deliveries。
- 如果没有 due delivery，返回 nil。
- load hook definition。
- 如果 hook disabled，mark `disabled_skipped`。
- 发送前重新解析 endpoint host 并执行 SSRF 防护，防止 DNS rebinding。
- HTTP client 禁止自动 follow redirect。
- create request with context timeout。
- send request。
- `2xx` mark succeeded。

默认 `ClaimTTL=5m`。实际 claim TTL 使用 `max(ClaimTTL, hook.timeout_seconds+60s)`；claim 时写入 `claim_expires_at=now+effectiveClaimTTL`，最终状态更新时清空 `claim_expires_at`。

- [ ] **Step 4: 运行成功路径测试**

Run: `go test ./internal/hookruntime -run TestDispatcherRunOnceDeliversWebhook`

Expected: PASS。

### Task 9: 实现 retry、dead-letter、disable

**Files:**
- Modify: `internal/hookruntime/dispatcher.go`
- Test: `internal/hookruntime/dispatcher_test.go`

- [ ] **Step 1: 写失败策略红测**

覆盖：

- network error -> `retry_wait`。
- timeout -> `retry_wait`。
- HTTP 3xx -> `dead_lettered`，不 follow redirect。
- HTTP 500 -> `retry_wait`。
- HTTP 429 -> `retry_wait`。
- HTTP 503 + `Retry-After` -> `retry_wait`，优先使用 `Retry-After`。
- HTTP 429 + `Retry-After` -> `retry_wait`，优先使用 `Retry-After`。
- HTTP 400 -> `dead_lettered`。
- attempt 达到 max 后 -> `dead_lettered`。
- disabled hook -> `disabled_skipped`。
- stale `delivering` -> 启动或 claim 前恢复为 `queued`。
- endpoint DNS rebinding 到私网地址 -> `dead_lettered`。

Run: `go test ./internal/hookruntime -run TestDispatcherFailurePolicy`

Expected: FAIL。

- [ ] **Step 2: 实现分类函数**

```go
func classifyDeliveryResult(statusCode int, err error) deliveryDecision
```

规则：

- network/timeout/5xx/429 retry。
- 429/503 如果有合法 `Retry-After`，用 `Retry-After` 计算下一次尝试时间。
- 3xx dead-letter，避免 redirect 绕过 endpoint 校验。
- 其他 4xx dead-letter。
- 2xx success。

- [ ] **Step 3: 实现退避**

M8 首版退避可以固定：

```text
base = min(2^attempt * RetryBaseDelay, 1h)
jittered = base * (0.5 + rand[0,0.5])
nextAttemptAt = now + jittered
```

默认 `RetryBaseDelay=30s`。`attempt` 使用本次失败后的 `attempt_count-1` 作为指数，因此第一次失败后的等待区间是 `15s..30s`，第二次是 `30s..60s`。实现阶段保持简单，不引入第三方 retry 库，并在指数计算前限制上界，避免大 attempt 溢出。`Retry-After` 合法时优先级高于 jitter backoff。

- [ ] **Step 4: 实现 stale delivering 恢复**

`Run` 启动时先调用 `RecoverStaleDelivering(now)`；`RunOnce` claim 前也可以调用一次，保证测试和手动触发路径都能恢复崩溃残留。恢复只处理 `status=delivering AND claim_expires_at < now`。

- [ ] **Step 5: 运行失败策略测试**

Run: `go test ./internal/hookruntime -run TestDispatcherFailurePolicy`

Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add internal/hookruntime internal/storage/sqlite/hook_delivery_repo.go
git commit -m "feat: 增加 Hook dispatcher"
```

### Task 10: `taskg server` 启动 dispatcher

**Files:**
- Modify: `internal/cli/server.go`
- Test: `internal/cli/root_test.go`

- [ ] **Step 1: 写 server 生命周期红测**

测试不需要真起长时间 server。新增一个可注入 fake dispatcher 的薄接口，验证 server run path 会启动并在 shutdown context 结束。

Run: `go test ./internal/cli -run TestServerStartsHookDispatcher`

Expected: FAIL。

- [ ] **Step 2: 抽出 dispatcher factory**

在 `internal/cli/server.go` 用包级变量或小接口支持测试注入：

```go
type hookDispatcher interface {
	Run(context.Context) error
}
```

默认 factory 创建 `hookruntime.NewDispatcher(...)`。

- [ ] **Step 3: server 启动时并行运行 dispatcher**

启动 HTTP server 后启动 dispatcher goroutine；shutdown 时共用 signal context 停止。dispatcher 错误应写 stderr 并使 server 返回错误，除非 context 正常取消。

- [ ] **Step 4: 运行 CLI server 测试**

Run: `go test ./internal/cli -run TestServerStartsHookDispatcher`

Expected: PASS。

---

## Chunk 5: HTTP API、Remote Client 与 CLI

### Task 11: 实现 Hook HTTP API

**Files:**
- Create: `internal/httpapi/hooks.go`
- Modify: `internal/httpapi/router.go`
- Modify: `internal/httpapi/envelope.go`
- Test: `internal/httpapi/hooks_test.go`

- [ ] **Step 1: 写 HTTP 红测**

覆盖：

- `POST /api/v1/hooks` 创建 workspace hook。
- `POST /api/v1/hooks` 创建 project hook。
- `GET /api/v1/hooks` 不回显 secret。
- `GET /api/v1/hooks/{hookID}`。
- `GET /api/v1/hooks/{hookID}/deliveries?status=dead_lettered`。
- `PATCH /api/v1/hooks/{hookID}`。
- `POST /api/v1/hooks/{hookID}/disable` / `enable`。
- `DELETE /api/v1/hooks/{hookID}`。
- 缺 `hook:write` 返回 403 `token_scope_denied`。
- member/viewer 返回 403 `permission_denied`。

Run: `go test ./internal/httpapi -run TestHookAPI`

Expected: FAIL。

- [ ] **Step 2: 注册 routes**

在 `router.go` 加：

```go
api.With(s.authMiddleware).Get("/api/v1/hooks", s.handleHookList)
api.With(s.authMiddleware).Post("/api/v1/hooks", s.handleHookCreate)
api.With(s.authMiddleware).Get("/api/v1/hooks/{hookID}", s.handleHookInfo)
api.With(s.authMiddleware).Patch("/api/v1/hooks/{hookID}", s.handleHookModify)
api.With(s.authMiddleware).Delete("/api/v1/hooks/{hookID}", s.handleHookDelete)
api.With(s.authMiddleware).Post("/api/v1/hooks/{hookID}/enable", s.handleHookEnable)
api.With(s.authMiddleware).Post("/api/v1/hooks/{hookID}/disable", s.handleHookDisable)
api.With(s.authMiddleware).Get("/api/v1/hooks/{hookID}/deliveries", s.handleHookDeliveries)
api.With(s.authMiddleware).Get("/api/v1/hook-deliveries/{deliveryID}", s.handleHookDeliveryInfo)
api.With(s.authMiddleware).Post("/api/v1/hook-deliveries/{deliveryID}/replay", s.handleHookDeliveryReplay)
```

- [ ] **Step 3: 实现 request/response DTO**

响应字段使用 snake_case，并和 app view 对齐。secret 只允许 request 中出现，不进入 response。`PATCH /api/v1/hooks/{hookID}` 中 `event_types` 不传表示保留原值；传入则整体替换，空数组返回 400。

- [ ] **Step 4: scoped service capability**

read endpoints 用：

```go
s.scopedService(r, "hook:read", app.PermissionHookRead, projectRef)
```

write/replay endpoints 用：

```go
s.scopedService(r, "hook:write", app.PermissionHookWrite, projectRef)
```

- [ ] **Step 5: 运行 HTTP 测试**

Run: `go test ./internal/httpapi -run TestHookAPI`

Expected: PASS。

### Task 12: 实现 delivery list/info/replay HTTP API

**Files:**
- Modify: `internal/app/hook.go`
- Modify: `internal/httpapi/hooks.go`
- Test: `internal/httpapi/hooks_test.go`

- [ ] **Step 1: 写 replay 红测**

覆盖：

- dead-letter delivery 可以 replay，状态回到 `queued`。
- delivery list 支持 `status` 过滤，至少覆盖 `dead_lettered`。
- replay 写 audit `hook.replay`。
- replay 不重建 payload。
- scope 外 delivery 返回 404 或 403，按现有资源隐藏策略收敛。

Run: `go test ./internal/httpapi -run TestHookDeliveryReplay`

Expected: FAIL。

- [ ] **Step 2: app 增加 delivery 方法**

```go
func (s *Service) ListHookDeliveries(hookID string, status string, limit int) ([]HookDeliveryView, error)
func (s *Service) HookDeliveryInfo(deliveryID string) (HookDeliveryView, error)
func (s *Service) ReplayHookDelivery(deliveryID string) (HookDeliveryView, error)
```

`ListHookDeliveries` 必须先用 Hook scope 校验当前 actor/token 是否可见，再按 status 过滤；`HookDeliveryInfo` / `ReplayHookDelivery` 使用 delivery record 上的 `workspace_id/project_id/actor_user_id` snapshot 做授权，不只依赖当前 Hook definition。`ReplayHookDelivery` 必须 audit，payload 不包含 secret。

- [ ] **Step 3: HTTP handler 接线**

按已有 `writeSuccess` / `writeAppError` 风格实现。

- [ ] **Step 4: 运行 replay 测试**

Run: `go test ./internal/httpapi -run TestHookDeliveryReplay`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/httpapi/hooks.go internal/httpapi/router.go internal/httpapi/envelope.go internal/httpapi/hooks_test.go internal/app/hook.go internal/app/hook_test.go
git commit -m "feat: 增加 Hook HTTP API"
```

### Task 13: 实现 remote client 与 CLI

**Files:**
- Create: `internal/remote/hook.go`
- Create: `internal/cli/hook.go`
- Modify: `internal/cli/root.go`
- Test: `internal/cli/root_test.go`
- Test: `tests/integration/cli_test.go`

- [ ] **Step 1: 写 CLI 本地红测**

覆盖：

- `taskg hook add ... --event task.created --url http://example.test/hook`
- `taskg hook list`
- `taskg hook info <id>`
- `taskg hook disable <id>`
- `taskg hook enable <id>`
- `taskg hook deliveries <id>`
- `taskg hook replay <delivery-id>`

Run: `go test ./internal/cli -run TestExecuteHook`

Expected: FAIL。

- [ ] **Step 2: 实现 remote client**

在 `internal/remote/hook.go` 增加：

```go
func (c *Client) ListHooks(ctx context.Context, workspace string, projectRef string, includeDisabled bool) ([]HookView, error)
func (c *Client) AddHook(ctx context.Context, workspace string, input AddHookInput) (HookView, error)
func (c *Client) HookInfo(ctx context.Context, id string) (HookView, error)
func (c *Client) ModifyHook(ctx context.Context, id string, input ModifyHookInput) (HookView, error)
func (c *Client) EnableHook(ctx context.Context, id string) (HookView, error)
func (c *Client) DisableHook(ctx context.Context, id string) (HookView, error)
func (c *Client) DeleteHook(ctx context.Context, id string) error
func (c *Client) ListHookDeliveries(ctx context.Context, hookID string, status string, limit int) ([]HookDeliveryView, error)
func (c *Client) ReplayHookDelivery(ctx context.Context, deliveryID string) (HookDeliveryView, error)
```

- [ ] **Step 3: 实现 CLI 命令组**

命令形态：

```text
taskg hook list [--project <slug|uuid>] [--all]
taskg hook add <name> --event <event> --url <url> [--project <slug|uuid>] [--secret <secret>|--secret-stdin|--secret-file <path>] [--timeout 10s] [--max-attempts 5]
taskg hook info <hook-id>
taskg hook modify <hook-id> [--name ...] [--event ...] [--url ...] [--secret <secret>|--secret-stdin|--secret-file <path>] [--timeout ...] [--max-attempts ...]
taskg hook enable <hook-id>
taskg hook disable <hook-id>
taskg hook delete <hook-id>
taskg hook deliveries <hook-id> [--status <status>] [--limit 50]
taskg hook replay <delivery-id>
```

本地与远程模式共用输出。`hook modify --event` 是整体替换语义；不传 `--event` 保留原 event list。`--secret-stdin` 从 stdin 读取一行并去掉末尾换行；`--secret-file` 从文件读取并去掉末尾换行；三种 secret 输入方式互斥。`--timeout` 只接受秒级整数 duration，例如 `10s`，拒绝 `10500ms` 这类亚秒值。`--json` 输出稳定结构。

- [ ] **Step 4: 运行 CLI 本地测试**

Run: `go test ./internal/cli -run TestExecuteHook`

Expected: PASS。

- [ ] **Step 5: 写远程 CLI 集成红测**

在 `tests/integration/cli_test.go` 增加：

- remote `hook add/list/disable/enable` 成功。
- remote hook 命令不触碰客户端本地 DB。
- token 缺 `hook:write` 失败。

Run: `go test ./tests/integration -run TestRemoteHookCommands`

Expected: FAIL。

- [ ] **Step 6: 修正 remote CLI 接线**

确保 `hook` 命令在 remote mode 只走 `internal/remote`。

- [ ] **Step 7: 运行远程 CLI 集成测试**

Run: `go test ./tests/integration -run TestRemoteHookCommands`

Expected: PASS。

- [ ] **Step 8: 提交**

```bash
git add internal/remote/hook.go internal/cli/hook.go internal/cli/root.go internal/cli/root_test.go tests/integration/cli_test.go
git commit -m "feat: 增加 Hook CLI"
```

---

## Chunk 6: OpenAPI、文档、发布脚本

### Task 14: 同步 OpenAPI

**Files:**
- Modify: `docs/openapi/taskg-v1.yaml`

- [ ] **Step 1: 增加 Hook schemas**

添加：

- `Hook`
- `HookCreateRequest`
- `HookUpdateRequest`
- `HookDelivery`
- `HookDeliveryReplayResponse`

要求：

- secret 只出现在 create/update request。
- response 不含 secret。
- status enum 包含 M8 全部 delivery 状态。
- `HookDelivery` schema 包含 `workspace_id`、`project_id`、`actor_user_id`、`claim_expires_at`。
- Webhook 协议文档列出 `X-Taskg-Delivery`、`X-Taskg-Hook-Id`、`X-Taskg-Attempt`、`X-Taskg-Timestamp`、`X-Taskg-Signature-256` 和签名输入格式。
- Hook create/update request 描述 endpoint SSRF 校验、`timeout_seconds` / `max_attempts` 范围。

- [ ] **Step 2: 增加 Hook paths**

覆盖 HTTP API 中所有 `/api/v1/hooks*` 与 `/api/v1/hook-deliveries/*`。

- [ ] **Step 3: 增加错误码**

OpenAPI common errors 增加：

- `hook_not_found`
- `hook_event_type_invalid`
- `hook_endpoint_invalid`
- `hook_delivery_not_found`
- `hook_delivery_not_replayable`

- [ ] **Step 4: YAML 基础校验**

增加或复用一个会 parse `docs/openapi/taskg-v1.yaml` 的测试，确保 YAML 语法和新增 path/schema 基本有效。

Run: `go test ./internal/httpapi -run TestOpenAPI`

Expected: PASS。

### Task 15: README / ROADMAP / 运维文档

**Files:**
- Modify: `README.md`
- Modify: `ROADMAP.md`
- Create: `docs/deployment.md`
- Create: `docs/backup-restore.md`
- Create: `scripts/release-build.sh`

- [ ] **Step 1: README 增加 Hook 快速示例**

包含：

```bash
./taskg hook add task-webhook --event task.created --event task.completed --url https://example.test/taskg
./taskg hook list
./taskg hook deliveries <hook-id>
./taskg hook replay <delivery-id>
```

说明 Hook 是 server-side webhook，不是本地 shell hook。

- [ ] **Step 2: ROADMAP 更新 M8 状态**

实现完成后把 M8 改为已完成，并列出实际交付内容。

- [ ] **Step 3: 编写 `docs/deployment.md`**

覆盖：

- `taskg server --listen :8080`
- reverse proxy / TLS termination。
- outbound webhook 网络要求。
- token scope 建议：`hook:read hook:write`。
- secret 不进入外部日志、audit、CLI/HTTP response。
- 建议使用 `--secret-stdin` 或 `--secret-file`，避免 `--secret` 进入 shell history / process list / CI log。
- Hook secret 会随 SQLite 数据库和 `VACUUM INTO` 备份保存；部署时建议数据库文件和备份文件权限为 `0600` 或等价的单用户可读写权限。
- 不要把带 query secret 的 webhook URL 写入 `endpoint_url`；如果外部系统要求密钥，应优先使用 Hook `secret` + HMAC 或外部系统自己的安全入口。
- 默认禁止 webhook 投递到 loopback、link-local、private RFC1918、RFC6598、multicast、unspecified 地址；`3xx` redirect 不会被自动跟随。

- [ ] **Step 4: 编写 `docs/backup-restore.md`**

覆盖：

- `VACUUM INTO` 示例。
- JSON export 的边界。
- restore 演练。
- Hook definition 与 delivery record 会随 SQLite 备份恢复。
- 备份文件包含 Hook secret，必须按生产密钥材料保护。

- [ ] **Step 5: 增加 release build 脚本**

`scripts/release-build.sh`：

```bash
#!/usr/bin/env bash
set -euo pipefail

mkdir -p dist
VERSION="${VERSION:-dev}"
for target in \
  linux/amd64 \
  linux/arm64 \
  darwin/amd64 \
  darwin/arm64 \
  windows/amd64
do
  GOOS="${target%/*}"
  GOARCH="${target#*/}"
  suffix=""
  if [ "$GOOS" = "windows" ]; then suffix=".exe"; fi
  CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" \
    go build -ldflags "-X main.version=${VERSION}" \
      -o "dist/taskg-${VERSION}-${GOOS}-${GOARCH}${suffix}" ./cmd/taskg
done
```

- [ ] **Step 6: 运行脚本验证**

Run: `VERSION=dev bash scripts/release-build.sh`

Expected: `dist/` 下生成 5 个二进制。

- [ ] **Step 7: 清理构建产物**

Run: `rm -rf dist`

Expected: 工作区不保留构建产物。

- [ ] **Step 8: 提交**

```bash
git add README.md ROADMAP.md docs/deployment.md docs/backup-restore.md scripts/release-build.sh docs/openapi/taskg-v1.yaml
git commit -m "docs: 补齐 M8 Hook 文档"
```

---

## Chunk 7: 端到端验收与安全回归

### Task 16: Hook end-to-end 验收测试

**Files:**
- Test: `tests/integration/cli_test.go`
- Test: `internal/httpapi/hooks_test.go`
- Test: `internal/hookruntime/dispatcher_test.go`

- [ ] **Step 1: 增加端到端测试**

场景：

- 启动 HTTP API test server。
- 创建 workspace hook，target 是 `httptest.Server`。
- 通过 HTTP API 创建 task。
- 调用 dispatcher `RunOnce`。
- webhook target 收到 `task.created` payload。
- webhook target 收到 `Content-Type: application/json; charset=utf-8`。
- webhook target 收到 `X-Taskg-Delivery`、`X-Taskg-Hook-Id`、`X-Taskg-Attempt`、`X-Taskg-Timestamp`、`X-Taskg-Signature-256`、`User-Agent`。
- `task.modified` 的 Modify / Start / Stop / Annotate 都能产生稳定 payload，payload 包含 `task`、`completed`、`deleted`。
- `project.archived` payload 包含 `project` 与 `archived: true`。

Run: `go test ./internal/httpapi -run TestHookEndToEnd`

Expected: FAIL until previous chunks complete，完成后 PASS。

- [ ] **Step 2: 增加 project scope 隔离测试**

场景：

- project A hook。
- project B 创建 task。
- dispatcher 不发送 A hook。
- project A 创建 task。
- dispatcher 发送 A hook。
- workspace hook 可以收到当前 workspace 内无 project 归属的 task 事件。
- project hook 不接收无 project 归属的 task 事件。

Run: `go test ./internal/httpapi -run TestHookProjectScopeIsolation`

Expected: PASS。

- [ ] **Step 3: 增加事务回滚测试**

场景：

- 注入 failing delivery repo。
- task add 返回错误。
- task 不存在。
- audit 不存在。
- delivery 不存在。

Run: `go test ./internal/app -run TestHookDeliveryFailureRollsBackTaskWrite`

Expected: PASS。

- [ ] **Step 4: 增加 dispatcher 恢复语义测试**

覆盖：

- hook disabled 后 pending delivery 标记为 `disabled_skipped`，不会自动投递。
- stale `delivering` delivery 在 `claim_expires_at` 过期后恢复为 `queued`。
- `RunOnce` 没有 due delivery 时返回 nil。
- `ClaimTTL` 晚于 hook timeout，不会在请求未完成时被 stale recovery 抢走。
- HTTP 3xx 不 follow redirect，并按 dead-letter 处理。
- 429/503 的合法 `Retry-After` 会影响 `next_attempt_at`。

Run: `go test ./internal/hookruntime -run 'TestDispatcherRecovery|TestDispatcherDisabled|TestDispatcherRunOnceNoDue'`

Expected: PASS。

- [ ] **Step 5: 增加 secret 泄露测试**

覆盖：

- Hook list/info 不包含 secret。
- audit payload 不包含 secret。
- secret create / rotation audit 包含短指纹，不包含原文。
- delivery payload/header 不包含 secret 原文。

Run: `go test ./internal/app ./internal/httpapi -run 'TestHookSecret|TestHookAuditSecret'`

Expected: PASS。

- [ ] **Step 6: 增加 SSRF 防护测试**

覆盖：

- `http://127.0.0.1/...`、`http://localhost/...`、`http://169.254.169.254/...`、`http://10.0.0.1/...`、`http://100.64.0.1/...` 创建 Hook 失败。
- 域名创建时解析为公网、发送前解析为私网时，dispatcher 拒绝投递并 dead-letter。
- 302 redirect 到私网地址不会被跟随。

Run: `go test ./internal/app ./internal/hookruntime -run 'TestHookEndpointSSRF|TestDispatcherRejectsReboundEndpoint|TestDispatcherDoesNotFollowRedirect'`

Expected: PASS。

### Task 17: 全量验证

**Files:**
- All changed files.

- [ ] **Step 1: gofmt**

Run: `gofmt -w internal cmd tests`

Expected: no output。

- [ ] **Step 2: Go tests**

Run: `go test ./...`

Expected: PASS。

- [ ] **Step 3: gofmt check + vet**

Run: `test -z "$(gofmt -l internal cmd tests)" && go vet ./...`

Expected: PASS。

- [ ] **Step 4: CGO-free tests**

Run: `CGO_ENABLED=0 go test ./...`

Expected: PASS。

- [ ] **Step 5: CGO-free build**

Run: `CGO_ENABLED=0 go build ./cmd/taskg`

Expected: PASS。

- [ ] **Step 6: 删除本地构建产物**

Run: `rm -f taskg`

Expected: `git status --short` 中没有 `taskg`。

- [ ] **Step 7: diff whitespace check**

Run: `git diff --check`

Expected: PASS。

- [ ] **Step 8: 最终提交**

```bash
git status --short
git add internal cmd tests docs README.md ROADMAP.md scripts go.mod go.sum
git commit -m "feat: 完成 M8 服务端 Hook"
```

## 执行顺序建议

优先顺序不可跳：

1. SQLite models/repository。
2. App Hook 管理与权限。
3. App 事务内事件 enqueue。
4. Dispatcher 与签名。
5. HTTP API。
6. Remote CLI / local CLI。
7. OpenAPI / README / 运维文档。
8. 端到端与全量验证。

如果执行中发现需要扩大范围到 adapter、memory、replica/sync、scheduler 或 MCP Hook 写操作，先停下来更新 spec，不要直接实现。
