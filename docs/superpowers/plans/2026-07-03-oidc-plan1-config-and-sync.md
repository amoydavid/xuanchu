# OIDC 接入 Plan 1：配置层 + 权限 + 通讯录同步（后端）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax.

**Goal:** 管理员可通过 `GET/PUT /api/v1/workspaces/{id}/sso/config` 配置 yaoguang OIDC，通过 `POST .../sso/sync` 触发通讯录同步（持久化 job + 后台 dispatcher 执行），建立本地 User + UserExternalID + Membership。本阶段产出可用 curl 完整测试。

**对应 spec:** `docs/superpowers/specs/2026-07-03-workspace-oidc-design.md`（第 4、5、7 节）

**依赖：** 无前置 plan。

---

## 测试 helper 约定

storage 层测试统一用 `storage.Open(filepath.Join(t.TempDir(), "x.db"))` 得到 `*Store`，再 `NewXxxRepository(store.DB())` 构造 repo（见 `db_test.go:49`）。本计划所有 storage 测试沿用此模式。

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
