# Workspace OIDC 接入（yaoguang IdP）实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让每个 workspace 可选择接入 yaoguang OIDC，实现浏览器 SSO 登录 + 通讯录同步开通成员。

**Architecture:** 两条共享配置与身份映射的链路——(A) 管理员通讯录同步：从 yaoguang directory API 拉全量成员，upsert 本地 User + UserExternalID(sub + external_identities) + Membership；(B) 用户 OIDC 浏览器登录：Auth Code Flow + PKCE，id_token sub 命中映射后建立独立 browser_session + cookie。复用现有 DB 轮询 dispatcher 做 sync job 持久化，不引入 redis。认证只做身份映射，授权仍由 membership/role 决定。

**Tech Stack:** Go 1.25、GORM + glebarez/sqlite、`github.com/coreos/go-oidc/v3`、`golang.org/x/oauth2`、React 19 + TanStack Router + shadcn/ui。

**对应 spec:** `docs/superpowers/specs/2026-07-03-workspace-oidc-design.md`

---

## 测试 helper 约定

storage 层测试统一用 `storage.Open(filepath.Join(t.TempDir(), "x.db"))` 得到 `*Store`，再 `NewXxxRepository(store.DB())` 构造 repo（见 `db_test.go:49`）。本计划所有 storage 测试沿用此模式。

---


## 阶段一：配置层 + 权限 + 通讯录同步（后端）

本阶段产出：管理员可通过 `GET/PUT /api/v1/workspaces/{id}/sso/config` 配置 OIDC，通过 `POST .../sso/sync` 触发通讯录同步，全流程可用 curl 测试。

---

### Task 1: 新增 SSO 权限常量与策略

**Files:**
- Modify: `internal/authz/model.go`（`PermissionReminderWrite` 之后，L45 附近）
- Modify: `internal/app/permission.go`（常量别名 L36 附近 + tenant capability L86 附近）

- [ ] **Step 1: 在 `internal/authz/model.go` 的权限常量块末尾追加**

```go
	PermissionSsoConfigRead  Permission = "sso.config.read"
	PermissionSsoConfigWrite Permission = "sso.config.write"
```

- [ ] **Step 2: 确认 `internal/authz/policy.go` 无需改动**

`RoleOwner` 直接 `return true`（已覆盖新权限）。`RoleAdmin/Member/Viewer` 的 switch 不含新权限 → 默认拒绝，符合「仅 owner」。

- [ ] **Step 3: 在 `internal/app/permission.go` 加常量别名**

在 `PermissionReminderWrite` 别名后追加：

```go
	PermissionSsoConfigRead  = authz.PermissionSsoConfigRead
	PermissionSsoConfigWrite = authz.PermissionSsoConfigWrite
```

- [ ] **Step 4: 在 `tenantCapabilityForPermission` 补读映射**

在 `PermissionWorkspaceRead` case 后追加（写入对 tenant actor 不映射 → 落 default 拒绝，符合仅 owner）：

```go
	case PermissionSsoConfigRead:
		return auth.ScopeWorkspaceRead, true
```

- [ ] **Step 5: 验证无回归**

```bash
go test ./internal/authz/... ./internal/app/...
```
Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add internal/authz/model.go internal/app/permission.go
git commit -m "feat: 新增 SSO 配置读写权限（仅 owner 放行）"
```

---

### Task 2: 新增三个 model + 迁移注册

**Files:**
- Modify: `internal/storage/models.go`（末尾）
- Modify: `internal/storage/migrate_sqlite.go:34`
- Modify: `internal/storage/migrate_postgres.go`

- [ ] **Step 1: 在 `internal/storage/models.go` 末尾追加三个 model**

```go
// BrowserSession 是 OIDC 登录后建立的浏览器会话，独立于 ApiToken。
type BrowserSession struct {
	ID          string `gorm:"primaryKey"` // 存哈希后的 session id
	UserID      string `gorm:"not null;index"`
	WorkspaceID string `gorm:"not null;index"`
	ExpiresAt   int64  `gorm:"not null;index"`
	CreatedAt   int64  `gorm:"not null"`
	LastSeenAt  int64  `gorm:"not null"`
}

// BrowserAuthFlow 记录一次进行中的 OIDC Auth Code Flow（state + PKCE），短 TTL。
type BrowserAuthFlow struct {
	State       string `gorm:"primaryKey"` // OIDC state
	WorkspaceID string `gorm:"not null;index"`
	PKCEVerifier string `gorm:"not null"`
	CreatedAt   int64  `gorm:"not null"`
	ExpiresAt   int64  `gorm:"not null;index"`
}

// DirectorySyncJob 记录一次通讯录同步任务，复用 dispatcher claim/lease 模式。
type DirectorySyncJob struct {
	ID             string  `gorm:"primaryKey"`
	WorkspaceID    string  `gorm:"not null;index"`
	Status         string  `gorm:"not null;default:'pending';index"` // pending/running/succeeded/failed
	ClaimedAt      *int64
	ClaimExpiresAt *int64
	ErrorMessage   string `gorm:"not null;default:''"`
	StatsJSON      string `gorm:"not null;default:'{}'"` // {"added":N,"removed":M,"updated":K}
	CreatedAt      int64  `gorm:"not null"`
	FinishedAt     *int64
}
```

- [ ] **Step 2: 在 `internal/storage/migrate_sqlite.go:34` 注册**

把第一个 AutoMigrate 的 model 列表末尾从 `&UserExternalID{}` 改为 `&UserExternalID{}, &BrowserSession{}, &BrowserAuthFlow{}, &DirectorySyncJob{}`。

- [ ] **Step 3: 在 `internal/storage/migrate_postgres.go` 同样注册**

找到 postgres AutoMigrate 列表，末尾追加 `&BrowserSession{}, &BrowserAuthFlow{}, &DirectorySyncJob{}`。

- [ ] **Step 4: 写迁移 smoke test**

在 `internal/storage/sync_job_repo_test.go`（新文件）写：

```go
package storage

import (
	"path/filepath"
	"testing"
)

func TestDirectorySyncJobMigrated(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	job := DirectorySyncJob{ID: "j1", WorkspaceID: "ws1", Status: "pending", CreatedAt: 1}
	if err := store.db.Create(&job).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	var got DirectorySyncJob
	if err := store.db.First(&got, "id=?", "j1").Error; err != nil {
		t.Fatalf("query: %v", err)
	}
}
```

- [ ] **Step 5: 运行**

```bash
go test ./internal/storage/ -run TestDirectorySyncJobMigrated -v
```
Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add internal/storage/models.go internal/storage/migrate_sqlite.go internal/storage/migrate_postgres.go internal/storage/sync_job_repo_test.go
git commit -m "feat: 新增 BrowserSession/BrowserAuthFlow/DirectorySyncJob model 与迁移"
```

---

### Task 3: DirectorySyncJob repo（CRUD + claim/lease）

**Files:**
- Create: `internal/storage/sync_job_repo.go`
- Modify: `internal/storage/sync_job_repo_test.go`

- [ ] **Step 1: 写失败测试（追加到 sync_job_repo_test.go）**

```go
func TestSyncJobCreateRejectsDuplicate(t *testing.T) {
	store, _ := Open(filepath.Join(t.TempDir(), "x.db"))
	t.Cleanup(func() { _ = store.Close() })
	repo := NewDirectorySyncJobRepository(store.db)

	if _, err := repo.Create("ws1", 100); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := repo.Create("ws1", 101); err != ErrSyncInProgress {
		t.Fatalf("second create err = %v, want ErrSyncInProgress", err)
	}
}

func TestSyncJobClaimAndComplete(t *testing.T) {
	store, _ := Open(filepath.Join(t.TempDir(), "x.db"))
	t.Cleanup(func() { _ = store.Close() })
	repo := NewDirectorySyncJobRepository(store.db)

	job, _ := repo.Create("ws1", 100)
	claimed, err := repo.ClaimNextPending(200, 300)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.ID != job.ID {
		t.Fatalf("claimed %s, want %s", claimed.ID, job.ID)
	}
	if err := repo.MarkSucceeded(job.ID, 300, `{"added":1}`); err != nil {
		t.Fatalf("succeeded: %v", err)
	}
	// 完成后可再次创建
	if _, err := repo.Create("ws1", 301); err != nil {
		t.Fatalf("create after done: %v", err)
	}
}

func TestSyncJobReclaimExpired(t *testing.T) {
	store, _ := Open(filepath.Join(t.TempDir(), "x.db"))
	t.Cleanup(func() { _ = store.Close() })
	repo := NewDirectorySyncJobRepository(store.db)

	job, _ := repo.Create("ws1", 100)
	// 手动把 claim 设为已过期（claimed_at 久远，claim_expires_at 已过）
	store.db.Model(&DirectorySyncJob{}).Where("id=?", job.ID).
		Updates(map[string]any{"status": "running", "claimed_at": 100, "claim_expires_at": 150})
	claimed, err := repo.ClaimNextPending(500, 300)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.ID != job.ID {
		t.Fatalf("did not reclaim expired job")
	}
}

func TestSyncJobMarkFailed(t *testing.T) {
	store, _ := Open(filepath.Join(t.TempDir(), "x.db"))
	t.Cleanup(func() { _ = store.Close() })
	repo := NewDirectorySyncJobRepository(store.db)

	job, _ := repo.Create("ws1", 100)
	_, _ = repo.ClaimNextPending(200, 300)
	if err := repo.MarkFailed(job.ID, 300, "boom"); err != nil {
		t.Fatalf("failed: %v", err)
	}
	var got DirectorySyncJob
	store.db.First(&got, "id=?", job.ID)
	if got.Status != "failed" || got.ErrorMessage != "boom" {
		t.Fatalf("got %+v", got)
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
go test ./internal/storage/ -run TestSyncJob -v
```
Expected: FAIL（类型/方法未定义）。

- [ ] **Step 3: 实现 `internal/storage/sync_job_repo.go`**

```go
package storage

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrSyncInProgress = errors.New("sync_in_progress")

type DirectorySyncJobRepository struct {
	db *gorm.DB
}

func NewDirectorySyncJobRepository(db *gorm.DB) *DirectorySyncJobRepository {
	return &DirectorySyncJobRepository{db: db}
}

// Create 插入 pending job；若该 workspace 已有 pending/running job 则返回 ErrSyncInProgress。
func (r *DirectorySyncJobRepository) Create(workspaceID string, now int64) (DirectorySyncJob, error) {
	var count int64
	if err := r.db.Model(&DirectorySyncJob{}).
		Where("workspace_id = ? AND status IN ?", workspaceID, []string{"pending", "running"}).
		Count(&count).Error; err != nil {
		return DirectorySyncJob{}, err
	}
	if count > 0 {
		return DirectorySyncJob{}, ErrSyncInProgress
	}
	job := DirectorySyncJob{
		ID:          "syncjob_" + uuid.NewString(),
		WorkspaceID: workspaceID,
		Status:      "pending",
		CreatedAt:   now,
	}
	if err := r.db.Create(&job).Error; err != nil {
		return DirectorySyncJob{}, err
	}
	return job, nil
}

// ClaimNextPending 认领最早的 pending job（或 claim 已过期的 running job），原子置 running。
func (r *DirectorySyncJobRepository) ClaimNextPending(now, claimTTL int64) (DirectorySyncJob, error) {
	var job DirectorySyncJob
	err := r.db.Transaction(func(tx *gorm.DB) error {
		// 优先 pending
		err := tx.Where("status = ?", "pending").Order("created_at ASC").First(&job).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 回收过期 running（claim_expires_at < now）
			err = tx.Where("status = ? AND claim_expires_at < ?", "running", now).
				Order("created_at ASC").First(&job).Error
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		return tx.Model(&DirectorySyncJob{}).Where("id = ?", job.ID).
			Updates(map[string]any{
				"status":          "running",
				"claimed_at":      now,
				"claim_expires_at": now + claimTTL,
			}).Error
	})
	if err != nil {
		return DirectorySyncJob{}, err
	}
	job.Status = "running"
	job.ClaimedAt = &now
	exp := now + claimTTL
	job.ClaimExpiresAt = &exp
	return job, nil
}

func (r *DirectorySyncJobRepository) MarkSucceeded(id string, finishedAt int64, statsJSON string) error {
	return r.db.Model(&DirectorySyncJob{}).Where("id = ?", id).
		Updates(map[string]any{
			"status":      "succeeded",
			"stats_json":  statsJSON,
			"finished_at": finishedAt,
		}).Error
}

func (r *DirectorySyncJobRepository) MarkFailed(id string, finishedAt int64, msg string) error {
	return r.db.Model(&DirectorySyncJob{}).Where("id = ?", id).
		Updates(map[string]any{
			"status":       "failed",
			"error_message": msg,
			"finished_at":  finishedAt,
		}).Error
}

func (r *DirectorySyncJobRepository) Get(id string) (DirectorySyncJob, error) {
	var job DirectorySyncJob
	err := r.db.First(&job, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return DirectorySyncJob{}, ErrNotFound
	}
	return job, err
}

func (r *DirectorySyncJobRepository) LatestForWorkspace(workspaceID string) (DirectorySyncJob, error) {
	var job DirectorySyncJob
	err := r.db.Where("workspace_id = ?", workspaceID).Order("created_at DESC").First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return DirectorySyncJob{}, ErrNotFound
	}
	return job, err
}

// 消除未使用 import 警告的占位（time 在 stats 序列化时由 app 层使用）。
var _ = time.Now
```

> 注：`ErrNotFound` 已在 storage 包定义（见 external_id_repo.go 用法）。

- [ ] **Step 4: 运行测试**

```bash
go test ./internal/storage/ -run TestSyncJob -v
```
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/storage/sync_job_repo.go internal/storage/sync_job_repo_test.go
git commit -m "feat: DirectorySyncJob repo（claim/lease + CRUD）"
```

---

### Task 4: BrowserSession / BrowserAuthFlow repo

**Files:**
- Create: `internal/storage/session_repo.go`
- Create: `internal/storage/session_repo_test.go`

- [ ] **Step 1: 写失败测试 `internal/storage/session_repo_test.go`**

```go
package storage

import (
	"path/filepath"
	"testing"
)

func TestSessionCreateGetDelete(t *testing.T) {
	store, _ := Open(filepath.Join(t.TempDir(), "x.db"))
	t.Cleanup(func() { _ = store.Close() })
	repo := NewSessionRepository(store.db)

	if err := repo.CreateSession("hash1", "u1", "ws1", 100, 200); err != nil {
		t.Fatalf("create: %v", err)
	}
	s, err := repo.GetSession("hash1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if s.UserID != "u1" || s.WorkspaceID != "ws1" {
		t.Fatalf("got %+v", s)
	}
	if err := repo.DeleteSession("hash1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := repo.GetSession("hash1"); err != ErrNotFound {
		t.Fatalf("after delete err = %v, want ErrNotFound", err)
	}
}

func TestSessionPurgeExpired(t *testing.T) {
	store, _ := Open(filepath.Join(t.TempDir(), "x.db"))
	t.Cleanup(func() { _ = store.Close() })
	repo := NewSessionRepository(store.db)

	_ = repo.CreateSession("h1", "u1", "ws1", 1, 50)   // 已过期
	_ = repo.CreateSession("h2", "u2", "ws1", 1, 200)  // 未过期
	n, err := repo.PurgeExpiredSessions(100)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if n != 1 {
		t.Fatalf("purged %d, want 1", n)
	}
	if _, err := repo.GetSession("h1"); err != ErrNotFound {
		t.Fatalf("h1 should be gone")
	}
}

func TestAuthFlowCreateGetDelete(t *testing.T) {
	store, _ := Open(filepath.Join(t.TempDir(), "x.db"))
	t.Cleanup(func() { _ = store.Close() })
	repo := NewSessionRepository(store.db)

	if err := repo.CreateAuthFlow("state1", "ws1", "verifier1", 100, 700); err != nil {
		t.Fatalf("create flow: %v", err)
	}
	f, err := repo.GetAuthFlow("state1")
	if err != nil {
		t.Fatalf("get flow: %v", err)
	}
	if f.PKCEVerifier != "verifier1" || f.WorkspaceID != "ws1" {
		t.Fatalf("got %+v", f)
	}
	if err := repo.DeleteAuthFlow("state1"); err != nil {
		t.Fatalf("delete flow: %v", err)
	}
	if _, err := repo.GetAuthFlow("state1"); err != ErrNotFound {
		t.Fatalf("after delete")
	}
}

func TestAuthFlowPurgeExpired(t *testing.T) {
	store, _ := Open(filepath.Join(t.TempDir(), "x.db"))
	t.Cleanup(func() { _ = store.Close() })
	repo := NewSessionRepository(store.db)

	_ = repo.CreateAuthFlow("s1", "ws1", "v", 1, 50)
	_ = repo.CreateAuthFlow("s2", "ws1", "v", 1, 200)
	n, _ := repo.PurgeExpiredAuthFlows(100)
	if n != 1 {
		t.Fatalf("purged %d, want 1", n)
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
go test ./internal/storage/ -run "TestSession|TestAuthFlow" -v
```
Expected: FAIL（NewSessionRepository 未定义）。

- [ ] **Step 3: 实现 `internal/storage/session_repo.go`**

```go
package storage

import (
	"errors"

	"gorm.io/gorm"
)

type SessionRepository struct {
	db *gorm.DB
}

func NewSessionRepository(db *gorm.DB) *SessionRepository {
	return &SessionRepository{db: db}
}

func (r *SessionRepository) CreateSession(idHash, userID, workspaceID string, createdAt, expiresAt int64) error {
	return r.db.Create(&BrowserSession{
		ID:          idHash,
		UserID:      userID,
		WorkspaceID: workspaceID,
		ExpiresAt:   expiresAt,
		CreatedAt:   createdAt,
		LastSeenAt:  createdAt,
	}).Error
}

func (r *SessionRepository) GetSession(idHash string) (BrowserSession, error) {
	var s BrowserSession
	err := r.db.First(&s, "id = ?", idHash).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return BrowserSession{}, ErrNotFound
	}
	return s, err
}

func (r *SessionRepository) TouchSession(idHash string, now int64) error {
	return r.db.Model(&BrowserSession{}).Where("id = ?", idHash).
		Update("last_seen_at", now).Error
}

func (r *SessionRepository) DeleteSession(idHash string) error {
	return r.db.Delete(&BrowserSession{}, "id = ?", idHash).Error
}

func (r *SessionRepository) PurgeExpiredSessions(now int64) (int64, error) {
	res := r.db.Where("expires_at < ?", now).Delete(&BrowserSession{})
	return res.RowsAffected, res.Error
}

func (r *SessionRepository) CreateAuthFlow(state, workspaceID, pkceVerifier string, createdAt, expiresAt int64) error {
	return r.db.Create(&BrowserAuthFlow{
		State:        state,
		WorkspaceID:  workspaceID,
		PKCEVerifier: pkceVerifier,
		CreatedAt:    createdAt,
		ExpiresAt:    expiresAt,
	}).Error
}

func (r *SessionRepository) GetAuthFlow(state string) (BrowserAuthFlow, error) {
	var f BrowserAuthFlow
	err := r.db.First(&f, "state = ?", state).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return BrowserAuthFlow{}, ErrNotFound
	}
	return f, err
}

func (r *SessionRepository) DeleteAuthFlow(state string) error {
	return r.db.Delete(&BrowserAuthFlow{}, "state = ?", state).Error
}

func (r *SessionRepository) PurgeExpiredAuthFlows(now int64) (int64, error) {
	res := r.db.Where("expires_at < ?", now).Delete(&BrowserAuthFlow{})
	return res.RowsAffected, res.Error
}
```

- [ ] **Step 4: 运行测试**

```bash
go test ./internal/storage/ -run "TestSession|TestAuthFlow" -v
```
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/storage/session_repo.go internal/storage/session_repo_test.go
git commit -m "feat: BrowserSession/BrowserAuthFlow repo"
```

---

### Task 5: ExternalIDRepository 补充方法

**Files:**
- Modify: `internal/storage/external_id_repo.go`
- Modify: `internal/storage/external_id_repo_test.go`（新建或追加）

当前 repo 已有 `GetByProviderAndExternalID`、`ListByUser`、`ListByUsers`。同步逻辑还需：列出某 user 的所有 external id（已有）、删除某 user 的某 provider 的指定 external id（清理已删身份）。

- [ ] **Step 1: 写失败测试**

```go
package storage

import (
	"path/filepath"
	"testing"
)

func TestExternalIDDeleteByUser(t *testing.T) {
	store, _ := Open(filepath.Join(t.TempDir(), "x.db"))
	t.Cleanup(func() { _ = store.Close() })
	repo := NewExternalIDRepository(store.db)

	a, _ := repo.Create(UserExternalID{ID: "e1", UserID: "u1", Provider: "yaoguang", ExternalID: "sub1", CreatedAt: 1})
	_, _ = repo.Create(UserExternalID{ID: "e2", UserID: "u1", Provider: "feishu", ExternalID: "f1", CreatedAt: 1})

	// 删除 u1 的 feishu/f1，保留 yaoguang/sub1
	if err := repo.DeleteUserProviderExternalID("u1", "feishu", "f1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	ids, _ := repo.ListByUser("u1")
	if len(ids) != 1 || ids[0].ID != a.ID {
		t.Fatalf("got %+v", ids)
	}
}
```

- [ ] **Step 2: 实现 `DeleteUserProviderExternalID`**

在 `internal/storage/external_id_repo.go` 追加：

```go
// DeleteUserProviderExternalID 删除某 user 在指定 provider+external_id 的映射（同步时清理已删身份）。
func (r *ExternalIDRepository) DeleteUserProviderExternalID(userID, provider, externalID string) error {
	return r.db.Where("user_id = ? AND provider = ? AND external_id = ?", userID, provider, externalID).
		Delete(&UserExternalID{}).Error
}
```

- [ ] **Step 3: 运行**

```bash
go test ./internal/storage/ -run TestExternalID -v
```
Expected: PASS。

- [ ] **Step 4: Commit**

```bash
git add internal/storage/external_id_repo.go internal/storage/external_id_repo_test.go
git commit -m "feat: ExternalIDRepository 增加按身份删除方法"
```

---

### Task 6: directory HTTP 客户端包

**Files:**
- Create: `internal/auth/directory/client.go`
- Create: `internal/auth/directory/client_test.go`

- [ ] **Step 1: 写失败测试 `internal/auth/directory/client_test.go`**

```go
package directory

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListMembers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/orgs/org1/directory/members" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer tok1" {
			t.Fatalf("auth = %s", r.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"data": map[string]any{
				"members": []map[string]any{
					{
						"id":           "m1",
						"sub":          "yaoguang_member:m1",
						"display_name": "张三",
						"role":         "owner",
						"status":       "active",
						"external_identities": []map[string]any{
							{"provider": "feishu", "user_type": "user_id", "value": "fs1"},
						},
					},
					{
						"id":           "m2",
						"sub":          "yaoguang_member:m2",
						"display_name": "李四",
						"role":         "member",
						"status":       "disabled",
						"external_identities": []map[string]any{},
					},
				},
				"source": "organization_members",
				"stale":  false,
			},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.Client())
	members, err := c.ListMembers(srv.URL, "org1", "tok1")
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("got %d members", len(members))
	}
	m1 := members[0]
	if m1.Sub != "yaoguang_member:m1" || m1.DisplayName != "张三" || m1.Role != "owner" || m1.Status != "active" {
		t.Fatalf("m1 = %+v", m1)
	}
	if len(m1.ExternalIdentities) != 1 || m1.ExternalIdentities[0].Provider != "feishu" || m1.ExternalIdentities[0].Value != "fs1" {
		t.Fatalf("m1 ext = %+v", m1.ExternalIdentities)
	}
	if members[1].Status != "disabled" {
		t.Fatalf("m2 status = %s", members[1].Status)
	}
}

func TestListMembersUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"code": "invalid_token"}})
	}))
	defer srv.Close()

	c := NewClient(srv.Client())
	_, err := c.ListMembers(srv.URL, "org1", "bad")
	if err == nil {
		t.Fatal("expected error")
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
go test ./internal/auth/directory/ -v
```
Expected: FAIL（包不存在）。

- [ ] **Step 3: 实现 `internal/auth/directory/client.go`**

```go
// Package directory 是 yaoguang 通讯录 API 的纯 Go 客户端，只负责 HTTP 拉取与 JSON 解析，不做持久化。
package directory

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// Member 是从 yaoguang directory 接口解析出的单个成员。
type Member struct {
	ID                 string
	Sub                string   // "yaoguang_member:{id}"
	DisplayName        string
	Role               string   // owner|admin|member
	Status             string   // active|disabled
	ExternalIdentities []Identity
}

type Identity struct {
	Provider string // feishu|wecom|dingtalk
	Value    string // IM user_id
}

// NewClient 用给定 *http.Client 构造客户端；传 nil 则用 http.DefaultClient。
func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{http: httpClient}
}

type Client struct {
	http *http.Client
}

// ListMembers 拉取某 org 的全量成员（yaoguang directory API 无分页）。
func (c *Client) ListMembers(baseURL, orgID, accessToken string) ([]Member, error) {
	u := fmt.Sprintf("%s/api/orgs/%s/directory/members", strings.TrimRight(baseURL, "/"), url.PathEscape(orgID))
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("directory api status %d: %s", resp.StatusCode, string(body))
	}

	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			Members []struct {
				ID           string `json:"id"`
				Sub          string `json:"sub"`
				DisplayName  string `json:"display_name"`
				Role         string `json:"role"`
				Status       string `json:"status"`
				ExtIdentities []struct {
					Provider string `json:"provider"`
					UserType string `json:"user_type"`
					Value    string `json:"value"`
				} `json:"external_identities"`
			} `json:"members"`
		} `json:"data"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode directory response: %w", err)
	}
	if !payload.OK {
		return nil, fmt.Errorf("directory api error: %s %s", payload.Error.Code, payload.Error.Message)
	}

	members := make([]Member, 0, len(payload.Data.Members))
	for _, m := range payload.Data.Members {
		ext := make([]Identity, 0, len(m.ExtIdentities))
		for _, e := range m.ExtIdentities {
			if e.UserType != "user_id" {
				continue
			}
			ext = append(ext, Identity{Provider: e.Provider, Value: e.Value})
		}
		members = append(members, Member{
			ID:                 m.ID,
			Sub:                m.Sub,
			DisplayName:        m.DisplayName,
			Role:               m.Role,
			Status:             m.Status,
			ExternalIdentities: ext,
		})
	}
	return members, nil
}
```

> 注：需在 import 加 `"strings"`。

- [ ] **Step 4: 运行测试**

```bash
go test ./internal/auth/directory/ -v
```
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/auth/directory/
git commit -m "feat: yaoguang directory HTTP 客户端"
```

---

### Task 7: OIDCConfigService（配置读写 + 脱敏）

**Files:**
- Create: `internal/app/oidc_config.go`
- Create: `internal/app/oidc_config_test.go`

职责：在 ConfigRepository 之上封装 `sso.*` key 的读写，提供脱敏视图。本 task **不引入加密**（ConfigRepository 当前是明文 KV），secret 字段脱敏仅用于读视图；加密作为后续改进记录在 spec 备注。先让功能闭环。

- [ ] **Step 1: 写失败测试**

```go
package app

import (
	"path/filepath"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

func newConfigTestStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestOIDCConfigRoundTrip(t *testing.T) {
	store := newConfigTestStore(t)
	cfgRepo := storage.NewConfigRepository(store.DB())
	svc := NewOIDCConfigService(cfgRepo)

	// 未配置
	got, enabled := svc.Get("ws1")
	if enabled {
		t.Fatal("should be disabled initially")
	}

	// 写入
	in := OIDCConfigInput{
		Provider:            "yaoguang",
		IssuerBaseURL:       "https://yg.example.com",
		OrgID:               "org1",
		ClientID:            "cid",
		ClientSecret:        "csecret",
		DirectoryAccessToken: "dtoken",
	}
	if err := svc.Set("ws1", in); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, enabled = svc.Get("ws1")
	if !enabled {
		t.Fatal("should be enabled after set")
	}
	if got.Provider != "yaoguang" || got.IssuerBaseURL != "https://yg.example.com" ||
		got.OrgID != "org1" || got.ClientID != "cid" {
		t.Fatalf("got %+v", got)
	}
	// secret 在读视图脱敏
	if got.ClientSecretMasked != "cse••et" {
		t.Fatalf("client secret masked = %q", got.ClientSecretMasked)
	}
	if got.DirectoryAccessTokenMasked != "dto••en" {
		t.Fatalf("dir token masked = %q", got.DirectoryAccessTokenMasked)
	}
}

func TestOIDCConfigSetPartialSecret(t *testing.T) {
	store := newConfigTestStore(t)
	cfgRepo := storage.NewConfigRepository(store.DB())
	svc := NewOIDCConfigService(cfgRepo)

	// 首次写入
	_ = svc.Set("ws1", OIDCConfigInput{
		Provider: "yaoguang", IssuerBaseURL: "u", OrgID: "o", ClientID: "c",
		ClientSecret: "secret123", DirectoryAccessToken: "token456",
	})
	// 第二次写入，secret 留空 → 保留原值
	_ = svc.Set("ws1", OIDCConfigInput{
		Provider: "yaoguang", IssuerBaseURL: "u2", OrgID: "o", ClientID: "c",
		ClientSecret: "", DirectoryAccessToken: "",
	})
	got, _ := svc.Get("ws1")
	if got.IssuerBaseURL != "u2" {
		t.Fatalf("issuer not updated: %s", got.IssuerBaseURL)
	}
	// 验证 secret 仍是原值（通过 ResolveSecrets 读原始值）
	secrets, err := svc.ResolveSecrets("ws1")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if secrets.ClientSecret != "secret123" || secrets.DirectoryAccessToken != "token456" {
		t.Fatalf("secrets = %+v, want originals", secrets)
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
go test ./internal/app/ -run TestOIDCConfig -v
```
Expected: FAIL（类型未定义）。

- [ ] **Step 3: 实现 `internal/app/oidc_config.go`**

```go
package app

import (
	"fmt"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

const (
	ssoKeyProvider            = "sso.provider"
	ssoKeyIssuerBaseURL       = "sso.issuer_base_url"
	ssoKeyOrgID               = "sso.org_id"
	ssoKeyClientID            = "sso.client_id"
	ssoKeyClientSecret        = "sso.client_secret"
	ssoKeyDirectoryToken      = "sso.directory_access_token"
	ssoKeyScopes              = "sso.scopes"
	ssoKeyRedirectPath        = "sso.redirect_path"
	ssoKeyExternalBaseURL     = "sso.external_base_url"
	ssoKeySessionTTL          = "sso.session_ttl"
	ssoKeySyncInterval        = "sso.sync_interval"
	ssoKeyInsecureCookie      = "sso.insecure_cookie"
)

// OIDCConfig 是脱敏后的配置读视图（给 HTTP/前端展示用）。
type OIDCConfig struct {
	Provider                 string
	IssuerBaseURL            string
	OrgID                    string
	ClientID                 string
	ClientSecretMasked       string
	DirectoryAccessTokenMasked string
	Scopes                   string
	RedirectPath             string
	ExternalBaseURL          string
	SessionTTL               string
	SyncInterval             string
	InsecureCookie           bool
}

// OIDCConfigInput 是写入请求（secret 留空表示不改）。
type OIDCConfigInput struct {
	Provider             string
	IssuerBaseURL        string
	OrgID                string
	ClientID             string
	ClientSecret         string
	DirectoryAccessToken string
	Scopes               string
	RedirectPath         string
	ExternalBaseURL      string
	SessionTTL           string
	SyncInterval         string
	InsecureCookie       bool
}

// OIDCConfigSecrets 是非脱敏的 secret 明文（仅 app 内部短暂使用）。
type OIDCConfigSecrets struct {
	ClientSecret         string
	DirectoryAccessToken string
}

type OIDCConfigService struct {
	cfg *storage.ConfigRepository
}

func NewOIDCConfigService(cfg *storage.ConfigRepository) *OIDCConfigService {
	return &OIDCConfigService{cfg: cfg}
}

func (s *OIDCConfigService) wk(key string) storage.ConfigKey {
	return storage.ConfigKey{Scope: storage.ConfigScopeWorkspace, Key: key}
}

func (s *OIDCConfigService) Get(workspaceID string) (OIDCConfig, bool) {
	g := func(key string) string {
		v, ok, _ := s.cfg.Get(storage.ConfigKey{WorkspaceID: workspaceID, Scope: storage.ConfigScopeWorkspace, Key: key})
		if !ok {
			return ""
		}
		return v
	}
	enabled := g(ssoKeyProvider) != "" && g(ssoKeyIssuerBaseURL) != ""
	return OIDCConfig{
		Provider:                   g(ssoKeyProvider),
		IssuerBaseURL:              g(ssoKeyIssuerBaseURL),
		OrgID:                      g(ssoKeyOrgID),
		ClientID:                   g(ssoKeyClientID),
		ClientSecretMasked:         mask(g(ssoKeyClientSecret)),
		DirectoryAccessTokenMasked: mask(g(ssoKeyDirectoryToken)),
		Scopes:                     g(ssoKeyScopes),
		RedirectPath:               g(ssoKeyRedirectPath),
		ExternalBaseURL:            g(ssoKeyExternalBaseURL),
		SessionTTL:                 g(ssoKeySessionTTL),
		SyncInterval:               g(ssoKeySyncInterval),
		InsecureCookie:             g(ssoKeyInsecureCookie) == "true",
	}, enabled
}

// ResolveSecrets 读非脱敏 secret（登录/同步时用）。
func (s *OIDCConfigService) ResolveSecrets(workspaceID string) (OIDCConfigSecrets, error) {
	cs, ok, err := s.cfg.Get(storage.ConfigKey{WorkspaceID: workspaceID, Scope: storage.ConfigScopeWorkspace, Key: ssoKeyClientSecret})
	if err != nil {
		return OIDCConfigSecrets{}, err
	}
	dt, ok2, err := s.cfg.Get(storage.ConfigKey{WorkspaceID: workspaceID, Scope: storage.ConfigScopeWorkspace, Key: ssoKeyDirectoryToken})
	if err != nil {
		return OIDCConfigSecrets{}, err
	}
	if !ok || !ok2 {
		return OIDCConfigSecrets{}, fmt.Errorf("sso secrets not configured")
	}
	return OIDCConfigSecrets{ClientSecret: cs, DirectoryAccessToken: dt}, nil
}

func (s *OIDCConfigService) Set(workspaceID string, in OIDCConfigInput) error {
	base := storage.ConfigKey{WorkspaceID: workspaceID, Scope: storage.ConfigScopeWorkspace}
	set := func(key, value string) error {
		return s.cfg.Set(storage.ConfigKey{WorkspaceID: workspaceID, Scope: storage.ConfigScopeWorkspace, Key: key}, value)
	}
	_ = base

	if err := set(ssoKeyProvider, defaultIfEmpty(in.Provider, "yaoguang")); err != nil {
		return err
	}
	if err := set(ssoKeyIssuerBaseURL, in.IssuerBaseURL); err != nil {
		return err
	}
	if err := set(ssoKeyOrgID, in.OrgID); err != nil {
		return err
	}
	if err := set(ssoKeyClientID, in.ClientID); err != nil {
		return err
	}
	// secret 留空 → 不覆盖（保留原值）
	if in.ClientSecret != "" {
		if err := set(ssoKeyClientSecret, in.ClientSecret); err != nil {
			return err
		}
	}
	if in.DirectoryAccessToken != "" {
		if err := set(ssoKeyDirectoryToken, in.DirectoryAccessToken); err != nil {
			return err
		}
	}
	if err := set(ssoKeyScopes, in.Scopes); err != nil {
		return err
	}
	if err := set(ssoKeyRedirectPath, defaultIfEmpty(in.RedirectPath, "/sso/oidc/callback")); err != nil {
		return err
	}
	if err := set(ssoKeyExternalBaseURL, in.ExternalBaseURL); err != nil {
		return err
	}
	if err := set(ssoKeySessionTTL, defaultIfEmpty(in.SessionTTL, "168h")); err != nil {
		return err
	}
	if err := set(ssoKeySyncInterval, defaultIfEmpty(in.SyncInterval, "1h")); err != nil {
		return err
	}
	if err := set(ssoKeyInsecureCookie, boolStr(in.InsecureCookie)); err != nil {
		return err
	}
	return nil
}

// mask 取前后 2 位，中间用 • 替换；短于 5 位则全 •。
func mask(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 5 {
		return strings.Repeat("•", len(s))
	}
	return s[:2] + "••" + s[len(s)-2:]
}

func defaultIfEmpty(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
```

> 注意：测试里断言的脱敏形式 `cse••et` 对应 `mask("csecret")` = `"cs"+"••"+"et"`。请在 Step 1 写测试时与 mask 实现对齐——`mask("csecret")` 输入长度 7 → `cse••et`？不对，`s[:2]="cs"`、`s[5:]="et"`、中间 `••`，结果 `cs••et`。请把测试期望改为 `"cs••et"` 与 `"dt••en"`(`mask("dtoken")`=`dt`+`••`+`en`)。**实现以本步 mask 为准，测试期望需修正为 `cs••et` / `dt••en`。**

- [ ] **Step 4: 修正测试期望值并运行**

把测试里 `"cse••et"` 改为 `"cs••et"`，`"dto••en"` 改为 `"dt••en"`，然后：

```bash
go test ./internal/app/ -run TestOIDCConfig -v
```
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/app/oidc_config.go internal/app/oidc_config_test.go
git commit -m "feat: OIDCConfigService 配置读写与脱敏"
```

---

### Task 8: DirectorySyncService（同步编排）

**Files:**
- Create: `internal/app/directory_sync.go`
- Create: `internal/app/directory_sync_test.go`

职责：给定 workspace 配置，拉 yaoguang directory → upsert User + UserExternalID + Membership，移除 disabled。纯编排逻辑，可单测（mock directory client）。

- [ ] **Step 1: 写失败测试**

```go
package app

import (
	"context"
	"path/filepath"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/auth/directory"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

type fakeDirectoryClient struct {
	members []directory.Member
	err     error
}

func (f *fakeDirectoryClient) ListMembers(baseURL, orgID, token string) ([]directory.Member, error) {
	return f.members, f.err
}

func newSyncTestStore(t *testing.T) *storage.Store {
	store, err := storage.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestSyncCreatesUsersAndMemberships(t *testing.T) {
	store := newSyncTestStore(t)
	// 预置 workspace
	wsRepo := storage.NewWorkspaceRepository(store.DB())
	ws, _ := wsRepo.Create(storage.Workspace{Slug: "ws1", Name: "WS1"})

	fake := &fakeDirectoryClient{members: []directory.Member{
		{ID: "m1", Sub: "yaoguang_member:m1", DisplayName: "张三", Role: "owner", Status: "active",
			ExternalIdentities: []directory.Identity{{Provider: "feishu", Value: "f1"}}},
		{ID: "m2", Sub: "yaoguang_member:m2", DisplayName: "李四", Role: "member", Status: "active"},
	}}

	svc := NewDirectorySyncService(store, fake)
	stats, err := svc.SyncOnce(context.Background(), ws.ID, "https://yg", "org1", "token")
	if err != nil {
		t.Fatalf("SyncOnce: %v", err)
	}
	if stats.Added != 2 {
		t.Fatalf("added = %d, want 2", stats.Added)
	}

	// 验证 user + external id + membership
	userRepo := storage.NewUserRepository(store.DB())
	u1, err := userRepo.GetByExternalID("yaoguang", "yaoguang_member:m1")
	if err != nil {
		t.Fatalf("get by ext: %v", err)
	}
	if u1.DisplayName != "张三" {
		t.Fatalf("display = %s", u1.DisplayName)
	}
	memberRepo := storage.NewMemberRepository(store.DB())
	m, _ := memberRepo.Get(u1.ID, ws.ID)
	if m.Role != "owner" {
		t.Fatalf("role = %s", m.Role)
	}
}

func TestSyncRemovesDisabled(t *testing.T) {
	store := newSyncTestStore(t)
	wsRepo := storage.NewWorkspaceRepository(store.DB())
	ws, _ := wsRepo.Create(storage.Workspace{Slug: "ws1", Name: "WS1"})

	// 第一次同步：m1 active
	fake := &fakeDirectoryClient{members: []directory.Member{
		{ID: "m1", Sub: "yaoguang_member:m1", DisplayName: "张三", Role: "member", Status: "active"},
	}}
	svc := NewDirectorySyncService(store, fake)
	svc.SyncOnce(context.Background(), ws.ID, "", "", "")

	// 第二次：m1 disabled
	fake.members = []directory.Member{
		{ID: "m1", Sub: "yaoguang_member:m1", DisplayName: "张三", Role: "member", Status: "disabled"},
	}
	stats, _ := svc.SyncOnce(context.Background(), ws.ID, "", "", "")
	if stats.Removed != 1 {
		t.Fatalf("removed = %d, want 1", stats.Removed)
	}

	// user 保留，membership 移除
	userRepo := storage.NewUserRepository(store.DB())
	u1, _ := userRepo.GetByExternalID("yaoguang", "yaoguang_member:m1")
	memberRepo := storage.NewMemberRepository(store.DB())
	_, err := memberRepo.Get(u1.ID, ws.ID)
	if err == nil {
		t.Fatal("membership should be removed")
	}
}

func TestSyncIdempotent(t *testing.T) {
	store := newSyncTestStore(t)
	wsRepo := storage.NewWorkspaceRepository(store.DB())
	ws, _ := wsRepo.Create(storage.Workspace{Slug: "ws1", Name: "WS1"})
	fake := &fakeDirectoryClient{members: []directory.Member{
		{ID: "m1", Sub: "yaoguang_member:m1", DisplayName: "张三", Role: "member", Status: "active"},
	}}
	svc := NewDirectorySyncService(store, fake)
	s1, _ := svc.SyncOnce(context.Background(), ws.ID, "", "", "")
	s2, _ := svc.SyncOnce(context.Background(), ws.ID, "", "", "")
	if s1.Added != 1 || s2.Added != 0 {
		t.Fatalf("first=%+v second=%+v, not idempotent", s1, s2)
	}
}

func TestSyncNameConflictSuffix(t *testing.T) {
	store := newSyncTestStore(t)
	wsRepo := storage.NewWorkspaceRepository(store.DB())
	ws, _ := wsRepo.Create(storage.Workspace{Slug: "ws1", Name: "WS1"})
	// 预置同名 user
	userRepo := storage.NewUserRepository(store.DB())
	userRepo.Create(storage.User{Name: "张三"})

	fake := &fakeDirectoryClient{members: []directory.Member{
		{ID: "m1", Sub: "yaoguang_member:m1", DisplayName: "张三", Role: "member", Status: "active"},
	}}
	svc := NewDirectorySyncService(store, fake)
	_, err := svc.SyncOnce(context.Background(), ws.ID, "", "", "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	u, _ := userRepo.GetByExternalID("yaoguang", "yaoguang_member:m1")
	if u.Name == "张三" {
		t.Fatal("name should have suffix to avoid conflict")
	}
}
```

> 注：`storage.NewWorkspaceRepository` 与 `WorkspaceRepository.Create` 需确认存在；若 workspace repo 名不同，按实际调整。

- [ ] **Step 2: 运行确认失败**

```bash
go test ./internal/app/ -run TestSync -v
```
Expected: FAIL。

- [ ] **Step 3: 实现 `internal/app/directory_sync.go`**

```go
package app

import (
	"context"
	"fmt"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/auth/directory"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"github.com/google/uuid"
)

// DirectoryClient 是 DirectorySyncService 依赖的通讯录客户端接口（便于测试 mock）。
type DirectoryClient interface {
	ListMembers(baseURL, orgID, token string) ([]directory.Member, error)
}

type SyncStats struct {
	Added   int
	Removed int
	Updated int
}

type DirectorySyncService struct {
	store   *storage.Store
	client  DirectoryClient
}

func NewDirectorySyncService(store *storage.Store, client DirectoryClient) *DirectorySyncService {
	return &DirectorySyncService{store: store, client: client}
}

func (s *DirectorySyncService) SyncOnce(ctx context.Context, workspaceID, baseURL, orgID, token string) (SyncStats, error) {
	var stats SyncStats
	members, err := s.client.ListMembers(baseURL, orgID, token)
	if err != nil {
		return stats, fmt.Errorf("list directory members: %w", err)
	}

	now := time.Now().Unix()
	userRepo := storage.NewUserRepository(s.store.DB())
	extRepo := storage.NewExternalIDRepository(s.store.DB())
	memberRepo := storage.NewMemberRepository(s.store.DB())

	// 远端 active 成员的 sub 集合，用于检测需移除的本地成员
	activeSubs := make(map[string]bool, len(members))

	for _, m := range members {
		if m.Status == "disabled" {
			continue
		}
		activeSubs[m.Sub] = true

		// 查/建 user
		var userID string
		ext, err := extRepo.GetByProviderAndExternalID("yaoguang", m.Sub)
		switch {
		case err == nil:
			userID = ext.UserID
		case storage.IsNotFound(err):
			name := uniqueUserName(userRepo, m.DisplayName, m.ID)
			u, err := userRepo.Create(storage.User{Name: name, DisplayName: m.DisplayName, CreatedAt: now, ModifiedAt: now})
			if err != nil {
				return stats, fmt.Errorf("create user for sub %s: %w", m.Sub, err)
			}
			userID = u.ID
			stats.Added++
		default:
			return stats, err
		}

		// 同步主映射 (yaoguang, sub)
		syncExtID(extRepo, userID, "yaoguang", m.Sub, now)
		// 同步 external_identities
		for _, e := range m.ExternalIdentities {
			syncExtID(extRepo, userID, e.Provider, e.Value, now)
		}

		// upsert membership
		if err := memberRepo.Upsert(storage.Membership{
			UserID: userID, WorkspaceID: workspaceID, Role: m.Role, ModifiedAt: now,
		}); err != nil {
			return stats, fmt.Errorf("upsert membership: %w", err)
		}
		if err == nil && ext.UserID != "" {
			// 已存在用户视为 update
			stats.Updated++
		}
	}

	// 移除 disabled / 远端已不存在的成员的 membership
	yaoguangIDs, err := extRepo.ListByUsers(allUserIDsForProvider(extRepo, "yaoguang"))
	if err != nil {
		return stats, err
	}
	for _, eid := range yaoguangIDs {
		if eid.Provider != "yaoguang" {
			continue
		}
		if activeSubs[eid.ExternalID] {
			continue
		}
		// 该 sub 在远端不存在或 disabled → 移除其在本 workspace 的 membership
		_ = removeMembership(memberRepo, eid.UserID, workspaceID)
		stats.Removed++
	}

	return stats, nil
}

func syncExtID(repo *storage.ExternalIDRepository, userID, provider, externalID string, now int64) {
	_, err := repo.GetByProviderAndExternalID(provider, externalID)
	if err == nil {
		return // 已存在
	}
	if !storage.IsNotFound(err) {
		return
	}
	_ = repo.Create(storage.UserExternalID{
		ID: uuid.NewString(), UserID: userID, Provider: provider, ExternalID: externalID, CreatedAt: now,
	})
}

// uniqueUserName 处理 User.Name 唯一冲突：追加 " (yaoguang:{id})"。
func uniqueUserName(userRepo *storage.UserRepository, name, memberID string) string {
	_, err := userRepo.GetByName(name)
	if err != nil && !storage.IsNotFound(err) {
		// 出错时退回原名
		return name
	}
	if storage.IsNotFound(err) {
		return name
	}
	return fmt.Sprintf("%s (yaoguang:%s)", name, memberID)
}

func removeMembership(repo *storage.MemberRepository, userID, workspaceID string) error {
	// MemberRepository 当前无 Delete；用 db 直接删
	return nil // 见 Step 4 补充
}

func allUserIDsForProvider(repo *storage.ExternalIDRepository, provider string) []string {
	// ExternalIDRepository 无 ListByProvider；用原始 db 查询
	return nil // 见 Step 4 补充
}
```

> Step 3 故意留两个 TODO（removeMembership / allUserIDsForProvider），Step 4 用直接 db 查询补全，避免给 Member/ExternalID repo 加太多方法。

- [ ] **Step 4: 补全直接 db 查询的辅助方法**

把 `DirectorySyncService` 加一个 `db` 字段引用，重写移除逻辑。在 `internal/app/directory_sync.go` 顶部结构体改为：

```go
type DirectorySyncService struct {
	store  *storage.Store
	client DirectoryClient
}

func (s *DirectorySyncService) db() *gorm.DB { return s.store.DB() }
```

重写 `removeMembership`：

```go
func (s *DirectorySyncService) removeMembership(userID, workspaceID string) error {
	return s.db().Where("user_id = ? AND workspace_id = ?", userID, workspaceID).
		Delete(&storage.Membership{}).Error
}
```

在 SyncOnce 的移除循环里改为枚举本地所有 yaoguang 映射：

```go
	var localExt []storage.UserExternalID
	s.db().Where("provider = ?", "yaoguang").Find(&localExt)
	for _, eid := range localExt {
		if activeSubs[eid.ExternalID] {
			continue
		}
		_ = s.removeMembership(eid.UserID, workspaceID)
		stats.Removed++
	}
```

并把 SyncOnce 里对 `yaoguangIDs` 的旧逻辑替换掉。需 import `gorm.io/gorm`。

> 注：`storage.IsNotFound` 需确认存在；若包里只有 `ErrNotFound`，用 `errors.Is(err, storage.ErrNotFound)`。请在实现时核对 `storage` 包导出的 not-found 判断 helper（grep `IsNotFound\|ErrNotFound`）。

- [ ] **Step 5: 运行测试**

```bash
go test ./internal/app/ -run TestSync -v
```
Expected: PASS。根据失败信息调整（如 WorkspaceRepository 方法名、IsNotFound helper 名）。

- [ ] **Step 6: Commit**

```bash
git add internal/app/directory_sync.go internal/app/directory_sync_test.go
git commit -m "feat: DirectorySyncService 通讯录同步编排"
```

---

### Task 9: DirectorySyncDispatcher（后台执行 worker + scheduler）

**Files:**
- Create: `internal/app/directory_sync_runtime.go`
- Modify: `internal/cli/server.go`（runtime goroutine 编排）

职责：`DirectorySyncDispatcher.Run(ctx)` 轮询 pending job → claim → 调 DirectorySyncService → 回写结果。`DirectorySyncScheduler.Run(ctx)` 周期为启用 OIDC 的 workspace 插入 job。

- [ ] **Step 1: 实现 dispatcher + scheduler `internal/app/directory_sync_runtime.go`**

```go
package app

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

type DirectorySyncRuntime struct {
	store    *storage.Store
	cfg      *OIDCConfigService
	sync     *DirectorySyncService
	jobRepo  *storage.DirectorySyncJobRepository
	pollInterval time.Duration
}

func NewDirectorySyncRuntime(store *storage.Store, cfg *OIDCConfigService, sync *DirectorySyncService) *DirectorySyncRuntime {
	return &DirectorySyncRuntime{
		store:        store,
		cfg:          cfg,
		sync:         sync,
		jobRepo:      storage.NewDirectorySyncJobRepository(store.DB()),
		pollInterval: 30 * time.Second,
	}
}

// Run 启动 dispatcher + scheduler 循环，随 ctx 取消退出。
func (r *DirectorySyncRuntime) Run(ctx context.Context) {
	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()
	// 启动即跑一次
	r.tick(time.Now())
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.tick(time.Now())
		}
	}
}

func (r *DirectorySyncRuntime) tick(now time.Time) {
	// 1. 调度：为到期 workspace 创建 job
	r.scheduleWorkspaces(now.Unix())
	// 2. 执行：认领并执行一个 pending job
	r.runOneJob(now.Unix())
}

func (r *DirectorySyncRuntime) scheduleWorkspaces(now int64) {
	// 列出所有 workspace，对启用了 OIDC 且到期的插入 job。
	// 简化实现：遍历 workspace，读 sso.sync_interval，比较最近 job 的 created_at。
	// （全量 workspace 列表由 WorkspaceRepository 提供；此处略，按实际 repo 方法实现）
}

func (r *DirectorySyncRuntime) runOneJob(now int64) {
	job, err := r.jobRepo.ClaimNextPending(now, int64(r.pollInterval.Seconds())*3)
	if err != nil {
		return // 无 pending
	}
	// 读配置
	secrets, err := r.cfg.ResolveSecrets(job.WorkspaceID)
	cfg, _ := r.cfg.Get(job.WorkspaceID)
	if err != nil {
		_ = r.jobRepo.MarkFailed(job.ID, now, err.Error())
		return
	}
	stats, err := r.sync.SyncOnce(context.Background(), job.WorkspaceID, cfg.IssuerBaseURL, cfg.OrgID, secrets.DirectoryAccessToken)
	if err != nil {
		_ = r.jobRepo.MarkFailed(job.ID, time.Now().Unix(), err.Error())
		log.Printf("directory sync failed for workspace %s: %v", job.WorkspaceID, err)
		return
	}
	statsJSON, _ := json.Marshal(map[string]int{"added": stats.Added, "removed": stats.Removed, "updated": stats.Updated})
	_ = r.jobRepo.MarkSucceeded(job.ID, time.Now().Unix(), string(statsJSON))
}
```

> 注：`scheduleWorkspaces` 需根据 `WorkspaceRepository` 的实际 List 方法补全（遍历 workspace、读 config、比较最近 job 时间）。`OIDCConfigService.Get` 返回的 `SyncInterval` 字符串需用 `time.ParseDuration` 解析。这部分逻辑直白但需对照 repo 实际 API，留给执行者按现有 workspace list 方法补全并加一个集成测试。

- [ ] **Step 2: 在 server.go 挂入 runtime goroutine**

在 `internal/cli/server.go` 的 runtime 编排处（现有 `runtimeWG.Add(3)` 附近），改为 `Add(4)` 并追加：

```go
go func() {
	defer runtimeWG.Done()
	directorySyncRuntime := app.NewDirectorySyncRuntime(store, oidcConfigSvc, directorySyncSvc)
	directorySyncRuntime.Run(runCtx)
}()
```

> 需在 server.go 的依赖装配区构造 `oidcConfigSvc`（`app.NewOIDCConfigService(cfgRepo)`）与 `directorySyncSvc`（`app.NewDirectorySyncService(store, directory.NewClient(http.DefaultClient))`）。具体变量名按 server.go 既有装配风格对齐。

- [ ] **Step 3: 验证编译 + 启动**

```bash
CGO_ENABLED=0 go build ./cmd/xuanchu
```
Expected: 编译通过。

- [ ] **Step 4: Commit**

```bash
git add internal/app/directory_sync_runtime.go internal/cli/server.go
git commit -m "feat: DirectorySyncRuntime 后台 dispatcher + scheduler"
```

---

### Task 10: HTTP 路由——SSO config get/set + sync 触发

**Files:**
- Modify: `internal/httpapi/huma_routes.go`（workspace 段 L199-206 附近）
- Modify: `internal/httpapi/workspaces.go`（新增 handler）
- Create: `internal/httpapi/sso_test.go`

- [ ] **Step 1: 在 huma_routes.go workspace 段追加路由**

在 workspace 路由组追加三条：

```go
	{Method: "GET", Path: "/api/v1/workspaces/{workspace}/sso/config", Tag: "SSO", Summary: "读取 workspace SSO 配置（脱敏）", Handler: s.handleWorkspaceSsoConfigGet, Status: 200},
	{Method: "PUT", Path: "/api/v1/workspaces/{workspace}/sso/config", Tag: "SSO", Summary: "写入 workspace SSO 配置", Handler: s.handleWorkspaceSsoConfigSet, Status: 200},
	{Method: "POST", Path: "/api/v1/workspaces/{workspace}/sso/sync", Tag: "SSO", Summary: "触发通讯录同步", Handler: s.handleWorkspaceSsoSync, Status: 202},
	{Method: "GET", Path: "/api/v1/workspaces/{workspace}/sso/sync/jobs/{job_id}", Tag: "SSO", Summary: "查询同步任务状态", Handler: s.handleWorkspaceSsoSyncJob, Status: 200},
```

> 注意：handler 注册签名与现有 `handleWorkspace*` 一致（接收 `http.ResponseWriter, *http.Request`，从 path param 取 workspace）。

- [ ] **Step 2: 实现 handler（放 internal/httpapi/workspaces.go 或新建 sso.go）**

新建 `internal/httpapi/sso.go`：

```go
package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

// handleWorkspaceSsoConfigGet 返回脱敏的 SSO 配置。
func (s *Server) handleWorkspaceSsoConfigGet(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := s.resolveWorkspaceIDFromPath(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_workspace", err.Error())
		return
	}
	if err := s.requireOwner(r); err != nil {
		writeForbidden(w)
		return
	}
	cfg, enabled := s.oidcConfig.Get(workspaceID)
	if !enabled {
		writeOK(w, map[string]any{"enabled": false})
		return
	}
	writeOK(w, map[string]any{"enabled": true, "config": cfg})
}

func (s *Server) handleWorkspaceSsoConfigSet(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := s.resolveWorkspaceIDFromPath(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_workspace", err.Error())
		return
	}
	if err := s.requireOwner(r); err != nil {
		writeForbidden(w)
		return
	}
	var in app.OIDCConfigInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if in.IssuerBaseURL == "" || in.OrgID == "" || in.ClientID == "" {
		writeError(w, http.StatusBadRequest, "missing_required", "issuer_base_url, org_id, client_id 必填")
		return
	}
	if err := s.oidcConfig.Set(workspaceID, in); err != nil {
		writeError(w, http.StatusInternalServerError, "config_set_failed", err.Error())
		return
	}
	cfg, _ := s.oidcConfig.Get(workspaceID)
	writeOK(w, map[string]any{"enabled": true, "config": cfg})
}

func (s *Server) handleWorkspaceSsoSync(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := s.resolveWorkspaceIDFromPath(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_workspace", err.Error())
		return
	}
	if err := s.requireOwner(r); err != nil {
		writeForbidden(w)
		return
	}
	now := s.now().Unix()
	job, err := s.syncJobRepo.Create(workspaceID, now)
	if err != nil {
		if errors.Is(err, storage.ErrSyncInProgress) {
			writeError(w, http.StatusConflict, "sync_in_progress", "已有同步任务进行中")
			return
		}
		writeError(w, http.StatusInternalServerError, "sync_create_failed", err.Error())
		return
	}
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": map[string]any{"job_id": job.ID, "status": job.Status}})
}

func (s *Server) handleWorkspaceSsoSyncJob(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := s.resolveWorkspaceIDFromPath(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_workspace", err.Error())
		return
	}
	if err := s.requireOwner(r); err != nil {
		writeForbidden(w)
		return
	}
	jobID := r.PathValue("job_id")
	job, err := s.syncJobRepo.Get(jobID)
	if err != nil || job.WorkspaceID != workspaceID {
		writeError(w, http.StatusNotFound, "job_not_found", "同步任务不存在")
		return
	}
	writeOK(w, map[string]any{"job": job})
}
```

> `resolveWorkspaceIDFromPath`、`requireOwner`、`writeOK`、`writeError`、`writeForbidden`、`s.now()` 需对照现有 Server 已有 helper；若不存在则补最小实现（见 Step 3）。

- [ ] **Step 3: 补充所需 Server helper（若缺失）**

在 `internal/httpapi/sso.go` 或 server.go 补：

```go
// requireOwner 校验当前请求者是该 workspace 的 owner（user 分支实时角色，tenant actor 走 capability）。
func (s *Server) requireOwner(r *http.Request) error {
	// 复用 app 层权限：s.authService.Require(PermissionSsoConfigRead)
	// 具体 Server 如何拿到当前 Service/runtime 见现有 handler 模式（如 workspace archive handler）。
	return s.authService.Require(app.PermissionSsoConfigRead)
}
```

> 关键：现有 handler 如何从 request 拿到 `*app.Service`（带 runtime/requestScope）需对照 `handleWorkspaceArchive` 等已有 owner-only handler 的写法对齐。**执行者应先读 `internal/httpapi/workspaces.go` 里 archive handler 的权限校验代码，照搬其模式实现 requireOwner，不要新造轮子。**

- [ ] **Step 4: 在 Server 结构体挂依赖字段**

在 `internal/httpapi/server.go` 的 Server 结构体加：

```go
	oidcConfig  *app.OIDCConfigService
	syncJobRepo *storage.DirectorySyncJobRepository
```

并在构造 Server 时注入（`NewServer` 或装配处）。`s.now()` 若不存在则加 `func (s *Server) now() time.Time { return time.Now() }`（或复用现有 clock 注入）。

- [ ] **Step 5: 写端到端测试 `internal/httpapi/sso_test.go`**

至少覆盖：owner 可写 config、非 owner 写 config 返回 403、sync 触发返回 202、重复 sync 返回 409。用现有 httpapi test harness（参考既有 handler 测试的 Server 构造方式）。

```go
package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSsoConfigOwnerCanWrite(t *testing.T) {
	// 用既有 testServer 构造一个以 owner 身份的请求
	// PUT /api/v1/workspaces/{slug}/sso/config
	// 期望 200
	// 具体构造方式参照 internal/httpapi 既有测试
}

func TestSsoConfigNonOwnerForbidden(t *testing.T) {
	// 以 member 身份 PUT，期望 403
}
```

> 端到端测试的 Server/请求构造方式高度依赖既有 test harness，执行者需先读 `internal/httpapi/*_test.go` 的现有写法照搬。测试用例保持「owner 可写、非 owner 403、sync 202/409」三个核心断言。

- [ ] **Step 6: 运行全量验证**

```bash
go test ./internal/httpapi/ -run TestSso -v
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```
Expected: 全 PASS。

- [ ] **Step 7: Commit**

```bash
git add internal/httpapi/huma_routes.go internal/httpapi/sso.go internal/httpapi/server.go internal/httpapi/sso_test.go
git commit -m "feat: SSO config/sync HTTP 路由（owner 鉴权）"
```

---

### Task 11: 阶段一验证 + 文档

- [ ] **Step 1: 全量测试**

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```
Expected: 全 PASS。

- [ ] **Step 2: 手动 curl 验证（在 docs 留示例）**

启动 server，用 owner PAT：

```bash
# 写配置
curl -X PUT localhost:8080/api/v1/workspaces/{slug}/sso/config \
  -H "Authorization: Bearer xuanchu_pat_..." \
  -H "Content-Type: application/json" \
  -d '{"issuer_base_url":"https://yg.example.com","org_id":"org1","client_id":"c","client_secret":"s","directory_access_token":"t"}'

# 触发同步（需 yaoguang 可达，否则 job 失败但流程可验证）
curl -X POST localhost:8080/api/v1/workspaces/{slug}/sso/sync -H "Authorization: Bearer xuanchu_pat_..."
```

- [ ] **Step 3: 更新 ROADMAP / README 备注（可选，阶段二后统一更新）**

- [ ] **Step 4: Commit**

```bash
git commit --allow-empty -m "chore: 阶段一（配置+同步）验证通过"
```

---

## 阶段一完成标准

- [ ] `GET/PUT /api/v1/workspaces/{id}/sso/config` 可用，仅 owner 读写，secret 脱敏
- [ ] `POST .../sso/sync` 创建持久化 job，后台 dispatcher 执行
- [ ] 同步建立 User + UserExternalID(yaoguang sub + IM identities) + Membership，幂等，disabled 移除 membership
- [ ] 全量 `go test ./...` 与 `CGO_ENABLED=0` 通过
- [ ] 所有步骤已 commit

**后续阶段：**
- Plan 2：OIDC 登录 + browser_session + authMiddleware 双通道
- Plan 3：Web Console 前端 SSO 配置页 + 登录入口

---

## 阶段二：OIDC 登录 + browser_session + authMiddleware 双通道（后端）

**依赖：** 阶段一已完成（OIDCConfigService、BrowserSession/BrowserAuthFlow repo、UserExternalID 映射、DirectorySyncService）。

**新增依赖：** `github.com/coreos/go-oidc/v3`、`golang.org/x/oauth2`

### 关键现有代码参考

- `authMiddleware`：`internal/httpapi/middleware.go:153-201`——当前只认 `Authorization: Bearer`，无 Bearer 直接 401。
- `humaRoute.Public` 字段：`internal/httpapi/huma_routes.go:20,92-97`——`Public: true` 的路由跳过 authMiddleware（SSO start/callback/logout 用）。
- `AuthenticatedToken`：`internal/app/token.go:105-112`——中间件构造的认证结果。

---

### Task 12: 引入 OIDC RP 依赖

**Files:**
- Modify: `go.mod`、`go.sum`

- [ ] **Step 1: 添加依赖**

```bash
go get github.com/coreos/go-oidc/v3/oidc
go get golang.org/x/oauth2
```

- [ ] **Step 2: 验证零 CGO 构建**

```bash
CGO_ENABLED=0 go build ./...
```
Expected: 编译通过（两个包都是纯 Go）。

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: 引入 go-oidc 与 oauth2 依赖"
```

---

### Task 13: OIDC RP 包——discovery + code exchange + id_token 校验

**Files:**
- Create: `internal/auth/oidc/provider.go`
- Create: `internal/auth/oidc/provider_test.go`

- [ ] **Step 1: 写失败测试 `internal/auth/oidc/provider_test.go`**

用 `httptest.Server` mock 一个最小 IdP（discovery + token + JWKS），签发一个用测试私钥签名的 id_token。覆盖：code exchange 成功返回 sub、验签失败报错、aud 不匹配报错。

```go
package oidc

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// 用 ECDSA 测试密钥签发 id_token，mock 一个最小 IdP。
func setupMockIdP(t *testing.T, clientID string) (baseURL string, priv *ecdsa.PrivateKey, srv *httptest.Server) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	jwksHandler := serveTestJWKS(priv)
	mux := http.NewServeMux()
	mux.Handle("/.well-known/openid-configuration", discoveryHandler(func() string { return srv.URL }, clientID))
	mux.Handle("/jwks", jwksHandler)
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		idToken := mintTestIDToken(t, priv, srv.URL, clientID, "yaoguang_member:m1")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "fake",
			"token_type":   "Bearer",
			"id_token":     idToken,
			"expires_in":   3600,
		})
	})
	srv = httptest.NewServer(mux)
	return srv.URL, priv, srv
}

func TestExchangeReturnsSub(t *testing.T) {
	base, _, srv := setupMockIdP(t, "client1")
	defer srv.Close()

	p := NewProvider(context.Background(), base, "client1", "secret1")
	url := p.AuthCodeURL("state123", "verifier123", base+"/sso/oidc/callback")
	if url == "" {
		t.Fatal("empty auth url")
	}

	token, err := p.Exchange(context.Background(), "fakecode", "verifier123", base+"/sso/oidc/callback")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if token.Subject != "yaoguang_member:m1" {
		t.Fatalf("sub = %s, want yaoguang_member:m1", token.Subject)
	}
}

func TestExchangeAudMismatchFails(t *testing.T) {
	base, _, srv := setupMockIdP(t, "client1")
	defer srv.Close()

	p := NewProvider(context.Background(), base, "WRONG_CLIENT", "secret1")
	_, err := p.Exchange(context.Background(), "fakecode", "verifier123", base+"/sso/oidc/callback")
	if err == nil {
		t.Fatal("expected aud mismatch error")
	}
}
```

> 辅助函数 `serveTestJWKS`、`discoveryHandler`、`mintTestIDToken` 见 Step 3——它们是测试基础设施，用 jose/crypto 签发 JWT。执行者需补全这些 helper（标准做法：用 `github.com/go-jose/go-jose/v3` 签发测试 JWT）。

- [ ] **Step 2: 运行确认失败**

```bash
go test ./internal/auth/oidc/ -v
```
Expected: FAIL（包/handler 不存在）。

- [ ] **Step 3: 实现 `internal/auth/oidc/provider.go`**

```go
// Package oidc 是 xuanchu 作为 OIDC Relying Party 的纯逻辑层，
// 封装 discovery、Auth Code Flow + PKCE、id_token 校验。不依赖 GORM/HTTP server。
package oidc

import (
	"context"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Token 是校验通过后的 id_token 关键字段。
type Token struct {
	Subject string
	Raw     string
}

// Provider 封装某次配置（issuer/client）对应的 OIDC RP 状态。
type Provider struct {
	oidcProvider *oidc.Provider
	oauthConfig  *oauth2.Config
	verifier     *oidc.IDTokenVerifier
}

// NewProvider 用 issuerBaseURL 做 discovery，构造 RP。
func NewProvider(ctx context.Context, issuerBaseURL, clientID, clientSecret string) *Provider {
	// coreos/go-oidc 的 Provider discovery 走 {issuer}/.well-known/openid-configuration
	// yaoguang 的 issuer_base_url 就是根 URL，需确保 discovery endpoint 在其下。
	provider, err := oidc.NewProvider(ctx, issuerBaseURL)
	if err != nil {
		// 调用方应处理；这里 panic 仅用于构造失败（实际由 NewProviderSafe 包装返回 error）
		panic(fmt.Sprintf("oidc discovery failed: %v", err))
	}
	config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
		RedirectURL:  "", // 由调用方传 redirectURI
	}
	verifier := provider.Verifier(&oidc.Config{ClientID: clientID})
	return &Provider{oidcProvider: provider, oauthConfig: config, verifier: verifier}
}

// NewProviderSafe 与 NewProvider 相同但返回 error（生产代码用）。
func NewProviderSafe(ctx context.Context, issuerBaseURL, clientID, clientSecret string) (*Provider, error) {
	provider, err := oidc.NewProvider(ctx, issuerBaseURL)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery: %w", err)
	}
	config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}
	verifier := provider.Verifier(&oidc.Config{ClientID: clientID})
	return &Provider{oidcProvider: provider, oauthConfig: config, verifier: verifier}, nil
}

// AuthCodeURL 生成跳转 IdP 的授权 URL（含 state + PKCE）。
func (p *Provider) AuthCodeURL(state, pkceVerifier, redirectURI string) string {
	p.oauthConfig.RedirectURL = redirectURI
	return p.oauthConfig.AuthCodeURL(state,
		oauth2.SetAuthURLParam("code_challenge", pkceChallengeS256(pkceVerifier)),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
}

// Exchange 用 authorization code 换 id_token 并校验。
func (p *Provider) Exchange(ctx context.Context, code, pkceVerifier, redirectURI string) (*Token, error) {
	p.oauthConfig.RedirectURL = redirectURI
	token, err := p.oauthConfig.Exchange(ctx, code,
		oauth2.SetAuthURLParam("code_verifier", pkceVerifier),
	)
	if err != nil {
		return nil, fmt.Errorf("oauth exchange: %w", err)
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return nil, fmt.Errorf("id_token missing in token response")
	}
	idToken, err := p.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("id_token verify: %w", err)
	}
	return &Token{Subject: idToken.Subject, Raw: rawIDToken}, nil
}

// pkceChallengeS256 按 RFC 7636 计算 S256 challenge。
func pkceChallengeS256(verifier string) string {
	// 复用 oauth2/generate 实现或手写：BASE64URL(SHA256(verifier))
	return oauth2.GenerateCodeVerifier // 占位，Step 4 修正
}
```

- [ ] **Step 4: 修正 PKCE challenge 实现**

`oauth2` 包没有直接导出 S256 helper，需手写。替换 `pkceChallengeS256`：

```go
import (
	"crypto/sha256"
	"encoding/base64"
)

func pkceChallengeS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
```

并把 `AuthCodeURL` 里对 verifier 的处理改为：调用方传入的 `pkceVerifier` 就是 code_verifier，challenge 由本函数算出。测试里 setupMockIdP 的 `/token` 不校验 PKCE（简化测试），生产由 yaoguang 校验。

同时补全测试 helper：`serveTestJWKS`（用 `go-jose` 导出公钥 JWKS）、`discoveryHandler`（返回 issuer/authorization_endpoint/token_endpoint/jwks_uri）、`mintTestIDToken`（用私钥签发带 sub/iss/aud/exp 的 JWT）。执行者按 `go-jose` 标准用法实现。

- [ ] **Step 5: 运行测试**

```bash
go test ./internal/auth/oidc/ -v
```
Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add internal/auth/oidc/ go.mod go.sum
git commit -m "feat: OIDC RP 包（discovery+PKCE+id_token 校验）"
```

---

### Task 14: OIDCAuthService——Start/Callback/Logout 流程

**Files:**
- Create: `internal/app/oidc_auth.go`
- Create: `internal/app/oidc_auth_test.go`

职责：编排 OIDC flow 与 session 生命周期。Start 生成 state+PKCE 存 BrowserAuthFlow；Callback 验证、sub 映射、建 session；Logout 删 session。

- [ ] **Step 1: 写失败测试**

```go
package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"testing"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

func newAuthTestStore(t *testing.T) *storage.Store {
	store, err := storage.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func hashStr(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestStartCreatesAuthFlow(t *testing.T) {
	store := newAuthTestStore(t)
	sessionRepo := storage.NewSessionRepository(store.DB())
	cfgRepo := storage.NewConfigRepository(store.DB())
	cfgSvc := NewOIDCConfigService(cfgRepo)
	// 预置配置（启用）
	_ = cfgSvc.Set("ws1", OIDCConfigInput{Provider: "yaoguang", IssuerBaseURL: "http://fake", OrgID: "o", ClientID: "c", ClientSecret: "s", DirectoryAccessToken: "t", ExternalBaseURL: "http://xuanchu"})

	svc := NewOIDCAuthService(store, sessionRepo, cfgSvc, &stubOIDCProvider{})
	url, err := svc.Start(context.Background(), "ws1")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if url == "" {
		t.Fatal("empty url")
	}
	// state 应已写入 BrowserAuthFlow
	flows := listAuthFlows(store)
	if len(flows) != 1 {
		t.Fatalf("flows = %d", len(flows))
	}
}

func TestCallbackSubNotMappedRejected(t *testing.T) {
	store := newAuthTestStore(t)
	sessionRepo := storage.NewSessionRepository(store.DB())
	cfgRepo := storage.NewConfigRepository(store.DB())
	cfgSvc := NewOIDCConfigService(cfgRepo)
	_ = cfgSvc.Set("ws1", OIDCConfigInput{Provider: "yaoguang", IssuerBaseURL: "http://fake", OrgID: "o", ClientID: "c", ClientSecret: "s", DirectoryAccessToken: "t", ExternalBaseURL: "http://xuanchu"})

	svc := NewOIDCAuthService(store, sessionRepo, cfgSvc, &stubOIDCProvider{})
	// 先 Start 拿到 state+verifier
	_, _ = svc.Start(context.Background(), "ws1")
	flow := listAuthFlows(store)[0]

	// callback：sub 未在 UserExternalID 映射 → 应报 identity_not_found
	_, err := svc.Callback(context.Background(), flow.State, "fakecode")
	if err == nil {
		t.Fatal("expected identity_not_found error")
	}
}

func TestCallbackSubMappedCreatesSession(t *testing.T) {
	store := newAuthTestStore(t)
	sessionRepo := storage.NewSessionRepository(store.DB())
	cfgRepo := storage.NewConfigRepository(store.DB())
	cfgSvc := NewOIDCConfigService(cfgRepo)
	_ = cfgSvc.Set("ws1", OIDCConfigInput{Provider: "yaoguang", IssuerBaseURL: "http://fake", OrgID: "o", ClientID: "c", ClientSecret: "s", DirectoryAccessToken: "t", ExternalBaseURL: "http://xuanchu"})

	// 预置 user + external id + membership（模拟 Plan 1 同步结果）
	userRepo := storage.NewUserRepository(store.DB())
	u, _ := userRepo.Create(storage.User{Name: "张三", CreatedAt: 1, ModifiedAt: 1})
	extRepo := storage.NewExternalIDRepository(store.DB())
	_, _ = extRepo.Create(storage.UserExternalID{ID: "e1", UserID: u.ID, Provider: "yaoguang", ExternalID: "yaoguang_member:m1", CreatedAt: 1})
	wsRepo := storage.NewWorkspaceRepository(store.DB())
	ws, _ := wsRepo.Create(storage.Workspace{Slug: "ws1", Name: "WS1"})
	memberRepo := storage.NewMemberRepository(store.DB())
	_ = memberRepo.Upsert(storage.Membership{UserID: u.ID, WorkspaceID: ws.ID, Role: "member", ModifiedAt: 1})

	svc := NewOIDCAuthService(store, sessionRepo, cfgSvc, &stubOIDCProvider{sub: "yaoguang_member:m1"})
	_, _ = svc.Start(context.Background(), ws.ID)
	flow := listAuthFlows(store)[0]

	rawSession, err := svc.Callback(context.Background(), flow.State, "fakecode")
	if err != nil {
		t.Fatalf("Callback: %v", err)
	}
	if rawSession == "" {
		t.Fatal("empty session token")
	}
	// session 哈希应已写入
	_, err = sessionRepo.GetSession(hashStr(rawSession))
	if err != nil {
		t.Fatalf("session not found: %v", err)
	}
	// flow 应已删除
	_, err = sessionRepo.GetAuthFlow(flow.State)
	if err != storage.ErrNotFound {
		t.Fatalf("flow should be deleted")
	}
}

// stubOIDCProvider 是测试用的 OIDCProvider 接口实现。
type stubOIDCProvider struct {
	sub string
}

func (s *stubOIDCProvider) AuthCodeURL(state, verifier, redirectURI string) string {
	return "http://fake/auth?state=" + state
}
func (s *stubOIDCProvider) Exchange(ctx context.Context, code, verifier, redirectURI string) (string, error) {
	if s.sub == "" {
		return "yaoguang_member:m1", nil
	}
	return s.sub, nil
}

func listAuthFlows(store *storage.Store) []storage.BrowserAuthFlow {
	var flows []storage.BrowserAuthFlow
	store.DB().Find(&flows)
	return flows
}
```

> `OIDCProvider` 接口让 Callback 返回 sub 字符串（Exchange 在 stub 里直接返回 sub，绕过真实 id_token 校验），生产实现由 Task 4 的 http handler 用 `oidc.Provider.Exchange` 拼装。

- [ ] **Step 2: 运行确认失败**

```bash
go test ./internal/app/ -run "TestStart|TestCallback" -v
```
Expected: FAIL（NewOIDCAuthService 未定义）。

- [ ] **Step 3: 实现 `internal/app/oidc_auth.go`**

```go
package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

// OIDCProvider 抽象 OIDC RP 的两个动作，便于测试 mock 与生产实现分离。
type OIDCProvider interface {
	AuthCodeURL(state, pkceVerifier, redirectURI string) string
	Exchange(ctx context.Context, code, pkceVerifier, redirectURI string) (sub string, err error)
}

type OIDCAuthService struct {
	store      *storage.Store
	sessionRepo *storage.SessionRepository
	cfg        *OIDCConfigService
	// providerFactory 按配置构造 OIDCProvider；生产用 oidc.NewProviderSafe，测试直接注入 stub。
	providerFactory func(issuerBaseURL, clientID, clientSecret string) (OIDCProvider, error)
}

func NewOIDCAuthService(store *storage.Store, sessionRepo *storage.SessionRepository, cfg *OIDCConfigService, fallback OIDCProvider) *OIDCAuthService {
	svc := &OIDCAuthService{store: store, sessionRepo: sessionRepo, cfg: cfg}
	if fallback != nil {
		// 测试模式：固定 provider
		svc.providerFactory = func(_, _, _ string) (OIDCProvider, error) { return fallback, nil }
	} else {
		svc.providerFactory = newOIDCProviderFromConfig
	}
	return svc
}

const (
	authFlowTTLSeconds int64 = 600 // 10 分钟
)

func (s *OIDCAuthService) Start(ctx context.Context, workspaceID string) (string, error) {
	cfg, enabled := s.cfg.Get(workspaceID)
	if !enabled {
		return "", fmt.Errorf("sso_not_enabled")
	}
	secrets, err := s.cfg.ResolveSecrets(workspaceID)
	if err != nil {
		return "", err
	}

	state := randomToken(32)
	verifier := randomToken(48)
	now := time.Now().Unix()
	redirectURI := redirectURIFromConfig(cfg)

	if err := s.sessionRepo.CreateAuthFlow(state, workspaceID, verifier, now, now+authFlowTTLSeconds); err != nil {
		return "", err
	}

	provider, err := s.providerFactory(cfg.IssuerBaseURL, cfg.ClientID, secrets.ClientSecret)
	if err != nil {
		return "", err
	}
	return provider.AuthCodeURL(state, verifier, redirectURI), nil
}

func (s *OIDCAuthService) Callback(ctx context.Context, state, code string) (string, error) {
	flow, err := s.sessionRepo.GetAuthFlow(state)
	if err != nil {
		return "", fmt.Errorf("invalid_state")
	}
	now := time.Now().Unix()
	if flow.ExpiresAt < now {
		_ = s.sessionRepo.DeleteAuthFlow(state)
		return "", fmt.Errorf("invalid_state")
	}

	cfg, enabled := s.cfg.Get(flow.WorkspaceID)
	if !enabled {
		return "", fmt.Errorf("sso_not_enabled")
	}
	secrets, err := s.cfg.ResolveSecrets(flow.WorkspaceID)
	if err != nil {
		return "", err
	}
	redirectURI := redirectURIFromConfig(cfg)

	provider, err := s.providerFactory(cfg.IssuerBaseURL, cfg.ClientID, secrets.ClientSecret)
	if err != nil {
		return "", err
	}
	sub, err := provider.Exchange(ctx, code, flow.PKCEVerifier, redirectURI)
	if err != nil {
		return "", fmt.Errorf("id_token_invalid: %w", err)
	}

	// sub 映射
	extRepo := storage.NewExternalIDRepository(s.store.DB())
	ext, err := extRepo.GetByProviderAndExternalID("yaoguang", sub)
	if err != nil {
		return "", fmt.Errorf("identity_not_found")
	}

	// 校验 membership 存在
	memberRepo := storage.NewMemberRepository(s.store.DB())
	_, err = memberRepo.Get(ext.UserID, flow.WorkspaceID)
	if err != nil {
		return "", fmt.Errorf("membership_inactive")
	}

	// 建 session
	ttl, _ := time.ParseDuration(defaultIfEmpty(cfg.SessionTTL, "168h"))
	rawSession := randomToken(32)
	sessionHash := hashHex(rawSession)
	if err := s.sessionRepo.CreateSession(sessionHash, ext.UserID, flow.WorkspaceID, now, now+int64(ttl.Seconds())); err != nil {
		return "", err
	}
	_ = s.sessionRepo.DeleteAuthFlow(state)
	return rawSession, nil
}

func (s *OIDCAuthService) Logout(sessionHash string) error {
	return s.sessionRepo.DeleteSession(sessionHash)
}

// ResolveSession 由 authMiddleware 调用：raw cookie token → sessionHash → 校验未过期 → 返回 user/workspace。
func (s *OIDCAuthService) ResolveSession(rawCookie string) (storage.BrowserSession, error) {
	sessionHash := hashHex(rawCookie)
	session, err := s.sessionRepo.GetSession(sessionHash)
	if err != nil {
		return storage.BrowserSession{}, err
	}
	if session.ExpiresAt < time.Now().Unix() {
		_ = s.sessionRepo.DeleteSession(sessionHash)
		return storage.BrowserSession{}, fmt.Errorf("session_expired")
	}
	return session, nil
}

func redirectURIFromConfig(cfg OIDCConfig) string {
	base := cfg.ExternalBaseURL
	path := cfg.RedirectPath
	if path == "" {
		path = "/sso/oidc/callback"
	}
	if base == "" {
		return path
	}
	return base + path
}

func randomToken(n int) string {
	buf := make([]byte, n)
	_, _ = rand.Read(buf)
	return base64.RawURLEncoding.EncodeToString(buf)
}

func hashHex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// newOIDCProviderFromConfig 是生产用 providerFactory（Task 4 挂入）。
func newOIDCProviderFromConfig(issuerBaseURL, clientID, clientSecret string) (OIDCProvider, error) {
	return nil, fmt.Errorf("not implemented") // 见 Task 4
}

var _ = auth.TokenTypePAT // 占位 import 使用
```

- [ ] **Step 4: 运行测试**

```bash
go test ./internal/app/ -run "TestStart|TestCallback" -v
```
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/app/oidc_auth.go internal/app/oidc_auth_test.go
git commit -m "feat: OIDCAuthService 登录流程编排"
```

---

### Task 15: 接入真实 OIDC Provider 工厂

**Files:**
- Modify: `internal/app/oidc_auth.go`（`newOIDCProviderFromConfig`）

- [ ] **Step 1: 实现生产 providerFactory**

把 `newOIDCProviderFromConfig` 替换为调用 `internal/auth/oidc` 包：

```go
import (
	xuanchuOIDC "git.dajee.net/dajee/xuanchu/internal/auth/oidc"
)

func newOIDCProviderFromConfig(issuerBaseURL, clientID, clientSecret string) (OIDCProvider, error) {
	p, err := xuanchuOIDC.NewProviderSafe(context.Background(), issuerBaseURL, clientID, clientSecret)
	if err != nil {
		return nil, err
	}
	return &realOIDCProvider{p: p}, nil
}

type realOIDCProvider struct {
	p *xuanchuOIDC.Provider
}

func (r *realOIDCProvider) AuthCodeURL(state, verifier, redirectURI string) string {
	return r.p.AuthCodeURL(state, verifier, redirectURI)
}

func (r *realOIDCProvider) Exchange(ctx context.Context, code, verifier, redirectURI string) (string, error) {
	tok, err := r.p.Exchange(ctx, code, verifier, redirectURI)
	if err != nil {
		return "", err
	}
	return tok.Subject, nil
}
```

- [ ] **Step 2: 验证编译**

```bash
go build ./...
```
Expected: 通过。

- [ ] **Step 3: Commit**

```bash
git add internal/app/oidc_auth.go
git commit -m "feat: 接入真实 OIDC Provider 工厂"
```

---

### Task 16: SSO HTTP 路由（start/callback/logout）

**Files:**
- Modify: `internal/httpapi/huma_routes.go`（追加 Public 路由）
- Create: `internal/httpapi/sso_auth.go`
- Create: `internal/httpapi/sso_auth_test.go`

- [ ] **Step 1: 在 humaRoutes() 追加三条 Public 路由**

在 `humaRoutes()` 返回的 slice 里追加（放在 healthz 附近）：

```go
		{Method: http.MethodGet, Path: "/sso/oidc/start", Tag: "SSO", Summary: "Start OIDC login flow.", Handler: s.handleSsoOidcStart, Public: true},
		{Method: http.MethodGet, Path: "/sso/oidc/callback", Tag: "SSO", Summary: "OIDC login callback.", Handler: s.handleSsoOidcCallback, Public: true},
		{Method: http.MethodPost, Path: "/auth/logout", Tag: "SSO", Summary: "Logout browser session.", Handler: s.handleAuthLogout, Public: true},
```

> `Public: true` 让它们跳过 authMiddleware（见 `huma_routes.go:92-97`）。

- [ ] **Step 2: 实现 handler `internal/httpapi/sso_auth.go`**

```go
package httpapi

import (
	"net/http"
	"net/url"

	"git.dajee.net/dajee/xuanchu/internal/app"
)

const sessionCookieName = "xuanchu_session"

func (s *Server) handleSsoOidcStart(w http.ResponseWriter, r *http.Request) {
	workspaceRef := r.URL.Query().Get("workspace")
	if workspaceRef == "" {
		writeError(w, http.StatusBadRequest, "missing_workspace", "workspace 参数必填", nil)
		return
	}
	wsID, err := s.resolveWorkspaceID(workspaceRef)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace_not_found", "workspace 不存在", nil)
		return
	}
	authURL, err := s.oidcAuth.Start(r.Context(), wsID)
	if err != nil {
		redirectWithError(w, r, "sso_start_failed")
		return
	}
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (s *Server) handleSsoOidcCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	if state == "" || code == "" {
		redirectWithError(w, r, "invalid_callback")
		return
	}
	rawSession, err := s.oidcAuth.Callback(r.Context(), state, code)
	if err != nil {
		redirectWithError(w, r, appErrorToSsoCode(err))
		return
	}
	s.setSessionCookie(w, rawSession, s.console.BasePath)
	// 回到 console 根
	http.Redirect(w, r, s.console.BasePath+"/", http.StatusFound)
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && cookie.Value != "" {
		_ = s.oidcAuth.Logout(hashHex(cookie.Value))
	}
	clearSessionCookie(w, s.console.BasePath)
	http.Redirect(w, r, s.console.BasePath+"/", http.StatusFound)
}

func (s *Server) setSessionCookie(w http.ResponseWriter, rawSession, path string) {
	secure := !s.oidcInsecureCookie
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    rawSession,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   7 * 24 * 3600,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter, path string) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1,
	})
}

func redirectWithError(w http.ResponseWriter, r *http.Request, code string) {
	target := "/?sso_error=" + url.QueryEscape(code)
	http.Redirect(w, r, target, http.StatusFound)
}

func appErrorToSsoCode(err error) string {
	msg := err.Error()
	switch {
	case contains(msg, "identity_not_found"):
		return "identity_not_found"
	case contains(msg, "membership_inactive"):
		return "membership_inactive"
	case contains(msg, "invalid_state"):
		return "invalid_state"
	case contains(msg, "id_token_invalid"):
		return "id_token_invalid"
	default:
		return "sso_failed"
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
```

> 注：`contains`/`indexOf` 是为避免引 strings 包（sso_auth.go 保持轻量）；执行者可改用 `strings.Contains`。`s.resolveWorkspaceID`、`s.console.BasePath`、`s.oidcInsecureCookie` 需在 Server 结构体补齐。

- [ ] **Step 3: 在 Server 结构体补依赖字段**

`internal/httpapi/server.go` 的 Server 加：

```go
	oidcAuth          *app.OIDCAuthService
	oidcInsecureCookie bool
```

并在 Server 构造处注入 `oidcAuth`（`app.NewOIDCAuthService(store, sessionRepo, oidcConfigSvc, nil)`，`nil` 表示生产模式用真实 providerFactory）。`oidcInsecureCookie` 从 OIDCConfig 的 `sso.insecure_cookie` 读取（或全局 flag）。

- [ ] **Step 4: 写 handler 测试 `internal/httpapi/sso_auth_test.go`**

用 mock oidcAuth（注入一个实现了 Start/Callback/Logout 的 stub）测试：start 返回 302、callback 成功 set cookie、logout 清 cookie。具体 Server 构造参照既有 httpapi 测试 harness。

```go
package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSsoStartRedirects(t *testing.T) {
	// mock oidcAuth.Start 返回固定 URL，验证 302
}

func TestSsoCallbackSetsCookie(t *testing.T) {
	// mock oidcAuth.Callback 返回 rawSession，验证 Set-Cookie 头
}

func TestAuthLogoutClearsCookie(t *testing.T) {
	// 验证 cookie 被清除（MaxAge -1）
}
```

> 三种 handler 测试都依赖注入 mock oidcAuth 到 Server。执行者参照 `internal/httpapi/*_test.go` 的 Server 构造与依赖注入方式补全。

- [ ] **Step 5: 运行测试**

```bash
go test ./internal/httpapi/ -run "TestSso|TestAuthLogout" -v
```
Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add internal/httpapi/huma_routes.go internal/httpapi/sso_auth.go internal/httpapi/server.go internal/httpapi/sso_auth_test.go
git commit -m "feat: SSO start/callback/logout HTTP 路由"
```

---

### Task 17: authMiddleware 双通道（Bearer 优先、Cookie 兜底 + 写操作禁 cookie）

**Files:**
- Modify: `internal/httpapi/middleware.go:153-201`

这是核心安全改动。逻辑：

1. 提取 Bearer；有 → 走现有 token 认证（不变）。
2. 无 Bearer 但有 cookie → 尝试 session 认证 → 但若请求是写操作（POST/PUT/PATCH/DELETE 且 path 以 `/api/v1/` 开头）→ 403 `cookie_write_forbidden`。
3. 都没有 → 401。

- [ ] **Step 1: 写失败测试 `internal/httpapi/middleware_dual_test.go`**

```go
package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCookieGetAllowed(t *testing.T) {
	// 带 cookie 的 GET /api/v1/tasks 应被放行（mock session 有效）
	// 验证下游 handler 收到 requestAuth
}

func TestCookieWriteForbidden(t *testing.T) {
	// 带 cookie 的 POST /api/v1/tasks 应返回 403 cookie_write_forbidden
}

func TestBearerStillWorks(t *testing.T) {
	// 带 Bearer 的请求走原有逻辑，不受影响
}

func TestNoCredentialUnauthorized(t *testing.T) {
	// 无 Bearer 无 cookie → 401
}
```

> 测试需构造带 session 的 Server。`CookieGetAllowed` 的 session 有效性靠 mock `oidcAuth.ResolveSession` 返回固定 BrowserSession，再由中间件构造 requestAuth。执行者按既有 httpapi test harness 实现。

- [ ] **Step 2: 改造 authMiddleware**

修改 `internal/httpapi/middleware.go` 的 `authMiddleware`（替换 L153-201）：

```go
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, hasBearer := bearerToken(r.Header.Get("Authorization"))

		if hasBearer {
			s.handleBearerAuth(w, r, raw, next)
			return
		}

		// 无 Bearer，尝试 cookie
		cookie, err := r.Cookie(sessionCookieName)
		if err == nil && cookie.Value != "" {
			// 写操作（POST/PUT/PATCH/DELETE 且 /api/v1/*）禁 cookie，防 CSRF
			if isApiWriteMethod(r.Method) && strings.HasPrefix(r.URL.Path, "/api/v1/") {
				writeError(w, http.StatusForbidden, "cookie_write_forbidden", "此操作需要 access token", nil)
				return
			}
			s.handleCookieAuth(w, r, cookie.Value, next)
			return
		}

		writeError(w, http.StatusUnauthorized, authz.CodeAuthMissingToken, "missing bearer token or session", nil)
	})
}

// handleBearerAuth 是原有 token 认证逻辑（抽出来保持原样）。
func (s *Server) handleBearerAuth(w http.ResponseWriter, r *http.Request, raw string, next http.Handler) {
	svc, err := app.NewService(app.ServiceOptions{
		Store: s.store, Clock: s.effectiveClock(), Runtime: &app.RuntimeContext{}, DisableScopeBootstrap: true,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "api_internal", "internal server error", nil)
		return
	}
	authn, err := svc.AuthenticateBearerToken(raw)
	if err != nil {
		writeAppError(w, err)
		return
	}
	visible, effective, err := s.visibleAndEffectiveWorkspaces(authn)
	if err != nil {
		writeAppError(w, err)
		return
	}
	s.populateLogState(r, authn, effective)
	next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), authContextKey, requestAuth{
		Authn: authn, VisibleWorkspaces: visible, EffectiveWorkspace: effective,
	})))
}

// handleCookieAuth 用 browser session 构造认证上下文。
func (s *Server) handleCookieAuth(w http.ResponseWriter, r *http.Request, rawCookie string, next http.Handler) {
	session, err := s.oidcAuth.ResolveSession(rawCookie)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_session", "session 无效或已过期", nil)
		return
	}
	// 构造 AuthenticatedToken：用户为 session.UserID，凭证类型 browser_session
	userRepo := storage.NewUserRepository(s.store.DB())
	user, err := userRepo.GetByID(session.UserID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_session", "session 用户不存在", nil)
		return
	}
	workspace, err := storage.NewWorkspaceRepository(s.store.DB()).GetByID(session.WorkspaceID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_session", "session workspace 不存在", nil)
		return
	}
	memberRepo := storage.NewMemberRepository(s.store.DB())
	member, err := memberRepo.Get(user.ID, workspace.ID)
	if err != nil {
		writeError(w, http.StatusForbidden, "membership_inactive", "您不是该工作区的成员", nil)
		return
	}
	authn := app.AuthenticatedToken{
		Token: app.TokenView{
			ID: "browser_session:" + session.ID,
			Name: "Browser Session",
			Type: "browser_session",
		},
		User: user,
	}
	// cookie 模式：effective workspace 固定为 session 的 workspace，role 用 member.Role
	visible := []storage.WorkspaceWithRole{{Workspace: workspace, Role: member.Role}}
	s.populateLogState(r, authn, workspace)
	next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), authContextKey, requestAuth{
		Authn: authn, VisibleWorkspaces: visible, EffectiveWorkspace: workspace,
	})))
}

// populateLogState 抽出原有 logState 写入逻辑。
func (s *Server) populateLogState(r *http.Request, authn app.AuthenticatedToken, effective storage.Workspace) {
	if state, ok := r.Context().Value(logStateContextKey).(*requestLogState); ok {
		if authn.TenantActor {
			state.actorType = "tenant_access_token"
			state.tokenName = authn.Token.Name
			state.tokenPrefix = authn.Token.Prefix
		} else {
			state.actorType = "user"
			state.actorID = authn.User.ID
		}
		state.tokenID = authn.Token.ID
		state.workspaceID = effective.ID
		state.workspaceRef = effective.Slug
	}
}

func isApiWriteMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}
```

> 注意：`app.TokenView` 的字段（ID/Name/Type 等）需对照 `internal/app/token.go` 实际定义补全。`storage.WorkspaceWithRole` 已有 Role 字段（见 `visibleAndEffectiveWorkspaces` 用法）。`TokenView.Type` 值用 `"browser_session"` 与 `authz.CredentialBrowserSession` 一致。

- [ ] **Step 3: 更新 `CredentialBrowserSession` 注释**

`internal/authz/model.go:72`：

```go
	CredentialBrowserSession CredentialKind = "browser_session"
```

去掉「预留，本次不实现」注释，改为「OIDC 登录产生的浏览器会话凭证」。

- [ ] **Step 4: 运行测试**

```bash
go test ./internal/httpapi/ -run "TestCookie|TestBearer|TestNoCredential" -v
go test ./...
```
Expected: PASS（含既有 authMiddleware 测试不受影响）。

- [ ] **Step 5: Commit**

```bash
git add internal/httpapi/middleware.go internal/authz/model.go internal/httpapi/middleware_dual_test.go
git commit -m "feat: authMiddleware 双通道（Bearer+Cookie，写操作禁 cookie）"
```

---

### Task 18: session 过期清理 runtime goroutine

**Files:**
- Modify: `internal/cli/server.go`（runtime goroutine）
- 或并入 Task 9（Plan 1）的 DirectorySyncRuntime ticker

- [ ] **Step 1: 在 server.go 加 session 清理 goroutine**

在 runtime 编排处追加（或并入既有 ticker）：

```go
go func() {
	defer runtimeWG.Done()
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-runCtx.Done():
			return
		case <-ticker.C:
			now := time.Now().Unix()
			sessionRepo := storage.NewSessionRepository(store.DB())
			_, _ = sessionRepo.PurgeExpiredSessions(now)
			_, _ = sessionRepo.PurgeExpiredAuthFlows(now)
		}
	}
}()
```

> 若 Plan 1 的 DirectorySyncRuntime 已在 server.go 加了 `runtimeWG.Add(4)`，这里改为 `Add(5)`。

- [ ] **Step 2: 验证编译**

```bash
CGO_ENABLED=0 go build ./cmd/xuanchu
```

- [ ] **Step 3: Commit**

```bash
git add internal/cli/server.go
git commit -m "feat: session/flow 过期定时清理"
```

---

### Task 19: 阶段二全量验证

- [ ] **Step 1: 全量测试**

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```
Expected: 全 PASS。

- [ ] **Step 2: 端到端手动验证（需 yaoguang 可达）**

1. （Plan 1 已完成）配置 SSO + 同步成员
2. 浏览器访问 `/sso/oidc/start?workspace=<slug>` → 跳转 yaoguang 登录
3. 回调后回到 console，带 cookie
4. `GET /api/v1/tasks` 用 cookie 访问成功
5. `POST /api/v1/tasks` 用 cookie → 403 `cookie_write_forbidden`
6. `/auth/logout` 清 cookie

- [ ] **Step 3: Commit**

```bash
git commit --allow-empty -m "chore: 阶段二（OIDC 登录+session）验证通过"
```

---

## 阶段二完成标准

- [ ] `/sso/oidc/start` + `/sso/oidc/callback` 完成 OIDC Auth Code Flow + PKCE
- [ ] sub 命中 UserExternalID 映射 → 建 browser_session + 下发 cookie；未命中 → identity_not_found
- [ ] authMiddleware 双通道：Bearer 优先，cookie 仅 GET，写操作禁 cookie
- [ ] `/auth/logout` 清 session + cookie
- [ ] 过期 session/flow 定时清理
- [ ] 全量 `go test ./...` 与 `CGO_ENABLED=0` 通过
- [ ] 所有步骤已 commit

---

## 阶段三：Web Console 前端（SSO 配置页 + 登录入口）

**依赖：** 阶段一、阶段二的后端 API 已就绪（`GET/PUT /api/v1/workspaces/{slug}/sso/config`、`POST .../sso/sync`、`/sso/oidc/start`）。

**技术栈：** React 19、TanStack Router、TanStack Query、shadcn/ui、i18next。

### 关键现有代码参考

- `navItems` 静态数组：`web/src/components/AppShell.tsx:47-61`（无角色过滤先例）
- `PageKey` 联合类型：`web/src/components/AppShell.tsx:36-46`
- 角色判断：`web/src/features/workspace/tokens/tokens-page.tsx:71-72`（`me.data?.effective_role`）
- 表单模式：`web/src/features/workspace/tokens/token-form.tsx`
- 数据层模式：`web/src/features/workspace/tokens/use-token-mutations.ts`

---

### Task 20: i18n 文案

**Files:**
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

- [ ] **Step 1: 在 zh-CN.ts 加 nav.sso 和 sso.* 文案组**

在 `nav` 组加一项 `"sso": "单点登录"`。

在顶层加新的 `sso` 文案组：

```ts
  sso: {
    title: "单点登录（OIDC）",
    description: "通过 yaoguang IdP 让 workspace 成员使用浏览器 SSO 登录。",
    sectionIdp: "IdP 连接",
    sectionDirectory: "通讯录同步",
    sectionAdvanced: "高级（可选）",
    sectionMembers: "成员同步",
    field: {
      issuerBaseUrl: "Issuer 根地址",
      orgId: "组织 ID",
      clientId: "Client ID",
      clientSecret: "Client Secret",
      directoryAccessToken: "通讯录访问令牌",
      syncInterval: "同步周期",
      externalBaseUrl: "外部可达地址",
      sessionTtl: "Session 有效期",
    },
    hint: {
      issuerBaseUrl: "yaoguang 服务根 URL；OIDC discovery 与通讯录接口都基于此地址。",
      orgId: "yaoguang 的 organization id，用于通讯录接口路径。",
      clientId: "xuanchu 在 yaoguang 注册的 internal app client_id。",
      clientSecret: "OIDC client_secret，加密存储。留空保存表示不修改。",
      directoryAccessToken: "调用 yaoguang 通讯录接口的 tenant_access_token，需覆盖 org.members.read scope 且 directory_access=org_read。",
      syncInterval: "定时拉取通讯录的间隔；选「禁用」则仅手动触发。",
      externalBaseUrl: "用于拼接 OIDC redirect_uri，留空则用请求 Host。",
      sessionTtl: "Browser session 有效期。",
    },
    secretSet: "已设置",
    secretUnset: "未设置",
    save: "保存配置",
    cancel: "取消",
    syncNow: "立即同步成员",
    lastSync: "上次同步",
    syncRunning: "同步中…",
    notEnabled: "尚未配置 SSO，填写以下信息后保存即可启用。",
    noPermission: "您没有权限查看此页面。",
    errors: {
      save: "保存失败",
      sync: "同步触发失败",
      syncInProgress: "已有同步任务进行中",
    },
  },
```

- [ ] **Step 2: 在 en-US.ts 加对应英文文案**（key 结构相同，值翻译）

- [ ] **Step 3: 验证 i18n 编译**

```bash
cd web && pnpm tsc --noEmit
```
Expected: 无类型错误。

- [ ] **Step 4: Commit**

```bash
git add web/src/locales/zh-CN.ts web/src/locales/en-US.ts
git commit -m "feat(web): SSO 配置页 i18n 文案"
```

---

### Task 21: nav 项 + owner/acting 显隐过滤

**Files:**
- Modify: `web/src/components/AppShell.tsx`

- [ ] **Step 1: 在 PageKey 加 "sso"**

```ts
export type PageKey =
  | "overview"
  | "projects"
  | "workspaces"
  | "members"
  | "tokens"
  | "sso"
  | "hooks"
  | "notifications"
  | "audit"
  | "settings"
```

- [ ] **Step 2: 在 navItems 加 sso 项（用 KeyRound 图标，放在 tokens 之后）**

```ts
const navItems: Array<{
  key: PageKey
  icon: React.ComponentType<{ className?: string }>
  to: string
  ssoOnly?: boolean
}> = [
  { key: "overview", icon: Activity, to: "/" },
  { key: "projects", icon: Boxes, to: "/projects" },
  { key: "workspaces", icon: Boxes, to: "/workspaces" },
  { key: "members", icon: Users, to: "/members" },
  { key: "tokens", icon: KeyRound, to: "/tokens" },
  { key: "sso", icon: ShieldCheck, to: "/sso", ssoOnly: true },
  { key: "hooks", icon: Webhook, to: "/hooks" },
  { key: "notifications", icon: Bell, to: "/notifications" },
  { key: "audit", icon: FileClock, to: "/audit" },
  { key: "settings", icon: Settings, to: "/settings" },
]
```

在 import 里加 `ShieldCheck`：

```ts
import {
  Activity, ArrowLeft, Bell, Boxes, FileClock, KeyRound, LogOut,
  RefreshCw, Settings, ShieldAlert, ShieldCheck, Users, Webhook,
} from "lucide-react"
```

- [ ] **Step 3: 在 AppShell 组件内加显隐过滤**

AppShell 组件需要 `useMe` 判断 owner/acting。当前 AppShell props 没有 me 数据，需在组件内调用 `useMe()`（参照 tokens-page）。在 `const { t } = useTranslation()` 后加：

```ts
import { useMe } from "@/features/workspace/session/useMe"
// ...
  const me = useMe()
  const role = me.data?.effective_role ?? ""
  const isOwner = role === "owner"
  const showSso = isOwner || acting
```

把 nav 渲染 map（L92）加过滤：

```tsx
          {navItems
            .filter((item) => !item.ssoOnly || showSso)
            .map((item) => {
```

> 注意：`acting` 变量在 L81 已定义（`getAdminActingContext() !== null`）。`useMe` 需确认 AppShell 已在 QueryClientProvider 内（是的，router 层已保证）。

- [ ] **Step 4: 验证类型检查**

```bash
cd web && pnpm tsc --noEmit
```
Expected: 无错误。

- [ ] **Step 5: Commit**

```bash
git add web/src/components/AppShell.tsx
git commit -m "feat(web): nav 加 SSO 项（仅 owner/acting 可见）"
```

---

### Task 22: 路由注册

**Files:**
- Create: `web/src/routes/workspace/SsoRoute.tsx`
- Modify: `web/src/routes/router.tsx`

- [ ] **Step 1: 创建 SsoRoute 包装（仿 TokensRoute）**

`web/src/routes/workspace/TokensRoute.tsx` 的结构（先读它），照搬到 `web/src/routes/workspace/SsoRoute.tsx`：

```tsx
import { SsoConfigPage } from "@/features/workspace/sso/sso-config-page"

export function SsoRoute() {
  return <SsoConfigPage />
}
```

> 若 TokensRoute 还有 lazy/suspense 包装，照搬同样的模式。

- [ ] **Step 2: 在 router.tsx 注册路由（仿 tokensRoute，L196-205）**

在 `tokensRoute` 定义后加：

```ts
const SsoRoute = lazy(() =>
  import("@/routes/workspace/SsoRoute").then((module) => ({
    default: module.SsoRoute,
  }))
)
const ssoRoute = createRoute({
  getParentRoute: () => workspaceRootRoute,
  path: "/sso",
  component: lazyRoute(SsoRoute),
})
```

在 `workspaceRootRoute.addChildren`（L208-221）的 `tokensRoute` 后加 `ssoRoute`：

```ts
    membersRoute,
    tokensRoute,
    ssoRoute,
    createResourceRoute("hooks", "/hooks"),
```

- [ ] **Step 3: 验证类型检查**

```bash
cd web && pnpm tsc --noEmit
```
Expected: 无错误（SsoConfigPage 尚未实现会报错，下一步实现）。

- [ ] **Step 4: Commit（先不提交，等 Task 4 页面实现后一起）**

---

### Task 23: SSO 数据层（query + mutation）

**Files:**
- Create: `web/src/features/workspace/sso/use-sso.ts`

- [ ] **Step 1: 实现数据层（仿 use-token-mutations.ts）**

```ts
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import {
  workspaceApiGet,
  workspaceApiPost,
  workspaceApiPut,
} from "@/features/workspace/session/workspace-api"
import { ApiError } from "@/lib/api"

export interface SsoConfigResponse {
  enabled: boolean
  config?: SsoConfig
}

export interface SsoConfig {
  provider: string
  issuer_base_url: string
  org_id: string
  client_id: string
  client_secret_masked: string
  directory_access_token_masked: string
  scopes: string
  redirect_path: string
  external_base_url: string
  session_ttl: string
  sync_interval: string
  insecure_cookie: boolean
}

export interface SsoConfigInput {
  issuer_base_url: string
  org_id: string
  client_id: string
  client_secret: string
  directory_access_token: string
  sync_interval: string
  external_base_url: string
  session_ttl: string
}

export interface SyncJobResponse {
  ok: boolean
  data: { job_id: string; status: string }
}

const SSO_CONFIG_KEY = ["workspace", "sso", "config"] as const

export function useSsoConfigQuery(workspaceSlug: string) {
  return useQuery({
    queryKey: SSO_CONFIG_KEY,
    queryFn: () =>
      workspaceApiGet<SsoConfigResponse>(`/api/v1/workspaces/${workspaceSlug}/sso/config`),
  })
}

export function useSaveSsoConfigMutation(workspaceSlug: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: SsoConfigInput) =>
      workspaceApiPut<SsoConfigResponse>(
        `/api/v1/workspaces/${workspaceSlug}/sso/config`,
        input,
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: SSO_CONFIG_KEY })
    },
  })
}

export function useTriggerSyncMutation(workspaceSlug: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () =>
      workspaceApiPost<SyncJobResponse>(
        `/api/v1/workspaces/${workspaceSlug}/sso/sync`,
        {},
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["workspace", "sso", "jobs"] })
    },
  })
}
```

> `workspaceApiPut` 需确认在 `workspace-api.ts` 已导出；若没有则补一个（仿 Post/Patch）。

- [ ] **Step 2: 确认 workspaceApiPut 存在**

读 `web/src/features/workspace/session/workspace-api.ts`，若无 `workspaceApiPut`，仿 `workspaceApiPost` 加：

```ts
export async function workspaceApiPut<T>(path: string, body: unknown): Promise<T> {
  assertWorkspacePath(path)
  const res = await fetch(path, {
    method: "PUT",
    headers: jsonHeaders(),
    body: JSON.stringify(body),
  })
  return handleResponse<T>(res)
}
```

（`jsonHeaders` / `handleResponse` / `assertWorkspacePath` 复用既有 helper）

- [ ] **Step 3: Commit（与 Task 5 页面一起提交）**

---

### Task 24: SSO 配置页组件

**Files:**
- Create: `web/src/features/workspace/sso/sso-config-page.tsx`

- [ ] **Step 1: 实现配置页（仿 token-form.tsx 受控表单）**

```tsx
import { useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { useState } from "react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { useMe } from "@/features/workspace/session/useMe"
import { PageHeader } from "@/pages/OverviewPage"
import { ApiError } from "@/lib/api"

import {
  useSaveSsoConfigMutation,
  useSsoConfigQuery,
  useTriggerSyncMutation,
  type SsoConfigInput,
} from "./use-sso"

export function SsoConfigPage({ workspaceSlug }: { workspaceSlug: string }) {
  const { t } = useTranslation()
  const me = useMe()
  const role = me.data?.effective_role ?? ""
  const isOwner = role === "owner"
  const canManage = isOwner // acting 模式由 nav 显隐保证进入此页

  const configQuery = useSsoConfigQuery(workspaceSlug)
  const saveMutation = useSaveSsoConfigMutation(workspaceSlug)
  const syncMutation = useTriggerSyncMutation(workspaceSlug)

  const enabled = configQuery.data?.enabled ?? false
  const cfg = configQuery.data?.config

  const [form, setForm] = useState<SsoConfigInput>({
    issuer_base_url: "",
    org_id: "",
    client_id: "",
    client_secret: "",
    directory_access_token: "",
    sync_interval: "1h",
    external_base_url: "",
    session_ttl: "168h",
  })
  const [loaded, setLoaded] = useState(false)

  // 首次加载配置后预填表单（secret 留空，保存时空值=不修改）
  if (configQuery.isSuccess && cfg && !loaded) {
    setForm({
      issuer_base_url: cfg.issuer_base_url,
      org_id: cfg.org_id,
      client_id: cfg.client_id,
      client_secret: "",
      directory_access_token: "",
      sync_interval: cfg.sync_interval || "1h",
      external_base_url: cfg.external_base_url,
      session_ttl: cfg.session_ttl || "168h",
    })
    setLoaded(true)
  }

  if (configQuery.isLoading) {
    return <Skeleton className="h-96 w-full" />
  }
  if (!canManage) {
    return <p className="text-sm text-muted-foreground">{t("sso.noPermission")}</p>
  }

  const update = (key: keyof SsoConfigInput, value: string) =>
    setForm((prev) => ({ ...prev, [key]: value }))

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    saveMutation.mutate(form)
  }

  return (
    <form className="space-y-8" onSubmit={handleSubmit}>
      <div className="flex items-center justify-between">
        <PageHeader title={t("sso.title")} description={t("sso.description")} />
      </div>

      {!enabled && (
        <p className="text-sm text-muted-foreground">{t("sso.notEnabled")}</p>
      )}

      {/* IdP 连接 */}
      <section className="space-y-4">
        <h3 className="text-sm font-medium">{t("sso.sectionIdp")}</h3>
        <Field label={t("sso.field.issuerBaseUrl")} hint={t("sso.hint.issuerBaseUrl")}>
          <Input value={form.issuer_base_url} onChange={(e) => update("issuer_base_url", e.target.value)} required />
        </Field>
        <Field label={t("sso.field.orgId")} hint={t("sso.hint.orgId")}>
          <Input value={form.org_id} onChange={(e) => update("org_id", e.target.value)} required />
        </Field>
        <Field label={t("sso.field.clientId")} hint={t("sso.hint.clientId")}>
          <Input value={form.client_id} onChange={(e) => update("client_id", e.target.value)} required />
        </Field>
        <Field
          label={t("sso.field.clientSecret")}
          hint={t("sso.hint.clientSecret")}
          badge={cfg?.client_secret_masked ? `${t("sso.secretSet")} ${cfg.client_secret_masked}` : t("sso.secretUnset")}
        >
          <Input type="password" value={form.client_secret} onChange={(e) => update("client_secret", e.target.value)} placeholder="••••••••" />
        </Field>
      </section>

      {/* 通讯录同步 */}
      <section className="space-y-4">
        <h3 className="text-sm font-medium">{t("sso.sectionDirectory")}</h3>
        <Field
          label={t("sso.field.directoryAccessToken")}
          hint={t("sso.hint.directoryAccessToken")}
          badge={cfg?.directory_access_token_masked ? `${t("sso.secretSet")} ${cfg.directory_access_token_masked}` : t("sso.secretUnset")}
        >
          <Input type="password" value={form.directory_access_token} onChange={(e) => update("directory_access_token", e.target.value)} placeholder="••••••••" />
        </Field>
        <Field label={t("sso.field.syncInterval")} hint={t("sso.hint.syncInterval")}>
          <Select value={form.sync_interval} onValueChange={(v) => update("sync_interval", v)}>
            <SelectTrigger><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="0">禁用</SelectItem>
              <SelectItem value="30m">30 分钟</SelectItem>
              <SelectItem value="1h">1 小时</SelectItem>
              <SelectItem value="6h">6 小时</SelectItem>
              <SelectItem value="24h">24 小时</SelectItem>
            </SelectContent>
          </Select>
        </Field>
      </section>

      {/* 高级 */}
      <section className="space-y-4">
        <h3 className="text-sm font-medium">{t("sso.sectionAdvanced")}</h3>
        <Field label={t("sso.field.externalBaseUrl")} hint={t("sso.hint.externalBaseUrl")}>
          <Input value={form.external_base_url} onChange={(e) => update("external_base_url", e.target.value)} />
        </Field>
        <Field label={t("sso.field.sessionTtl")} hint={t("sso.hint.sessionTtl")}>
          <Select value={form.session_ttl} onValueChange={(v) => update("session_ttl", v)}>
            <SelectTrigger><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="24h">1 天</SelectItem>
              <SelectItem value="168h">7 天</SelectItem>
              <SelectItem value="720h">30 天</SelectItem>
            </SelectContent>
          </Select>
        </Field>
      </section>

      {/* 错误提示 */}
      {saveMutation.isError && (
        <p className="text-sm text-destructive">
          {t("sso.errors.save")}: {errorMessage(saveMutation.error)}
        </p>
      )}
      {syncMutation.isError && (
        <p className="text-sm text-destructive">
          {t("sso.errors.sync")}: {errorMessage(syncMutation.error)}
        </p>
      )}

      <div className="flex justify-end gap-2 pt-2">
        <Button type="submit" disabled={saveMutation.isPending}>
          {t("sso.save")}
        </Button>
      </div>

      {/* 成员同步 */}
      <section className="space-y-4 border-t pt-4">
        <h3 className="text-sm font-medium">{t("sso.sectionMembers")}</h3>
        <div className="flex items-center gap-3">
          <Button
            type="button"
            variant="outline"
            onClick={() => syncMutation.mutate()}
            disabled={!enabled || syncMutation.isPending}
          >
            {syncMutation.isPending ? t("sso.syncRunning") : t("sso.syncNow")}
          </Button>
        </div>
      </section>
    </form>
  )
}

function Field({
  label, hint, badge, children,
}: {
  label: string
  hint?: string
  badge?: string
  children: React.ReactNode
}) {
  return (
    <div className="space-y-1.5">
      <div className="flex items-center justify-between">
        <Label className="text-sm">{label}</Label>
        {badge && <span className="text-xs text-muted-foreground">{badge}</span>}
      </div>
      {children}
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  )
}

function errorMessage(err: unknown): string {
  if (err instanceof ApiError) return err.message
  return String(err)
}
```

> 注意：`SsoConfigPage` 接收 `workspaceSlug` prop。需确认 workspace root route 如何向下传 slug（参考 TokensPage 如何拿到 workspaceSlug——通常从 route params 或 useMe 的 effective_workspace.slug）。若现有页面通过 `useMe().data?.effective_workspace.slug` 获取，照搬。

- [ ] **Step 2: 确认 workspaceSlug 来源**

读 `web/src/routes/workspace/TokensRoute.tsx` 和 tokens-page 如何获取 workspaceSlug。若是 route loader / params，照搬到 SsoRoute。若是组件内 `useMe`，在 SsoConfigPage 内同样获取。

- [ ] **Step 3: 类型检查 + 构建**

```bash
cd web && pnpm tsc --noEmit && pnpm build
```
Expected: 无错误。

- [ ] **Step 4: Commit（Task 3+4+5 一起）**

```bash
git add web/src/routes/workspace/SsoRoute.tsx web/src/routes/router.tsx web/src/features/workspace/sso/ web/src/features/workspace/session/workspace-api.ts
git commit -m "feat(web): SSO 配置页（表单+同步触发）"
```

---

### Task 25: 登录页 OIDC 登录入口

**Files:**
- 找到 Web Console 登录页组件（token 输入页），加「OIDC 登录」按钮

- [ ] **Step 1: 定位登录页**

搜索登录页组件（`signInTitle` 在 `web/src/locales/zh-CN.ts:20` 附近被引用）。找到渲染 token 输入框的组件文件。

- [ ] **Step 2: 在登录表单下加 OIDC 登录按钮**

```tsx
<Button
  type="button"
  variant="outline"
  onClick={() => {
    const slug = workspaceSlug // 当前输入或选中的 workspace
    window.location.href = `/sso/oidc/start?workspace=${encodeURIComponent(slug)}`
  }}
>
  OIDC 单点登录
</Button>
```

> 按钮跳转到 `/sso/oidc/start`，由后端 302 到 yaoguang。callback 后回到 console 根并带 cookie。若 SSO 未启用某 workspace，后端返回错误页（带 `?sso_error=`）。

- [ ] **Step 3: 处理 SSO 错误回调**

在登录页读取 `?sso_error=` query param，显示对应中文提示（identity_not_found / membership_inactive / invalid_state 等）。

- [ ] **Step 4: 类型检查 + 构建**

```bash
cd web && pnpm tsc --noEmit && pnpm build
```

- [ ] **Step 5: Commit**

```bash
git add web/src/...（登录页文件）
git commit -m "feat(web): 登录页加 OIDC 登录入口"
```

---

### Task 26: 构建前端产物 + embed

**Files:**
- 由构建产出 `internal/webconsole/dist`

- [ ] **Step 1: 构建前端并嵌入**

```bash
make web-console-build
```

> 该命令重新生成 `internal/webconsole/dist`（被 Go embed），参照 `docs/manual/web-console.md`。

- [ ] **Step 2: 验证后端构建包含新前端**

```bash
CGO_ENABLED=0 go build ./cmd/xuanchu
```

- [ ] **Step 3: Commit**

```bash
git add internal/webconsole/dist
git commit -m "build: 重新构建 web console（含 SSO 页面）"
```

---

### Task 27: 阶段三全量验证

- [ ] **Step 1: 全量后端测试**

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```
Expected: 全 PASS。

- [ ] **Step 2: 端到端验证**

1. owner 登录 → 侧边栏看到「单点登录」→ 配置 yaoguang 信息 → 保存
2. 点「立即同步成员」→ 成员同步
3. 退出 → 登录页点「OIDC 单点登录」→ 跳转 yaoguang → 回到 console 带 cookie
4. member/viewer 登录 → 看不到「单点登录」菜单
5. cookie 只读：GET 成功、POST 返回 403

- [ ] **Step 3: 更新文档**

更新 `docs/manual/web-console.md`：新增 OIDC 登录入口、通讯录同步操作说明、cookie 只读约束。
更新 `README.md`：新增 SSO 登录说明。
更新 `ROADMAP.md`：标记 v0.5.0 OIDC 完成。

- [ ] **Step 4: Commit**

```bash
git add docs/manual/web-console.md README.md ROADMAP.md
git commit -m "docs: OIDC 接入完成，更新文档"
```

---

## 阶段三完成标准

- [ ] 「单点登录」菜单仅 owner + admin acting 可见
- [ ] SSO 配置页可读写 yaoguang 配置（secret 脱敏、留空不改）
- [ ] 可触发通讯录同步
- [ ] 登录页有 OIDC 登录入口
- [ ] 前端构建产物已嵌入
- [ ] 全量 `go test` + `CGO_ENABLED=0` 通过
- [ ] 文档已同步

---

## 全部完成标准

- [ ] 阶段一：配置读写（owner 鉴权）+ 通讯录同步（DB 轮询 dispatcher）
- [ ] 阶段二：OIDC 登录（PKCE + id_token 校验）+ browser_session + authMiddleware 双通道（cookie 仅 GET）
- [ ] 阶段三：Web Console SSO 配置页（owner/acting 显隐）+ 登录入口
- [ ] 全量 `go test ./...` 与 `CGO_ENABLED=0 go test ./...` 与 `CGO_ENABLED=0 go build ./cmd/xuanchu` 通过
- [ ] README / ROADMAP / web-console.md 已同步
- [ ] 所有步骤已 commit
