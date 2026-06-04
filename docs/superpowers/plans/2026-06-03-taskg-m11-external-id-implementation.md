# M11 用户外部 ID 绑定 实施计划

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 taskg 用户增加外部 ID 绑定能力，让 Agent 能通过 `feishu:ou_xxxxx` 这类标识符指派 assignee、查询用户，并在所有返回用户信息的地方一并返回外部 ID 列表。

**Architecture:** 新增 `user_external_ids` 表存储用户与外部系统的 ID 映射。在现有 `resolveUser` 中增加 `provider:value` 格式解析，使所有使用 assignee ref 的入口（CLI、HTTP API、MCP、JSON import）自动支持外部 ID。所有返回用户/assignee 的地方附带 `external_ids` 列表。

**Tech Stack:** Go 1.25, GORM + github.com/glebarez/sqlite, github.com/google/uuid, github.com/spf13/cobra, github.com/modelcontextprotocol/go-sdk

**Spec:** `docs/superpowers/specs/2026-06-03-taskg-m11-external-id-design.md`

---

## 文件结构

### 新建文件

| 文件 | 职责 |
|---|---|
| `internal/storage/external_id_repo.go` | UserExternalID 的 CRUD 操作 |

### 修改文件

| 文件 | 变更内容 |
|---|---|
| `internal/storage/models.go` | 新增 `UserExternalID` struct |
| `internal/storage/db.go` | AutoMigrate 加入 `UserExternalID` |
| `internal/storage/user_repo.go` | 新增 `GetByExternalID` 方法 |
| `internal/storage/task_repo.go` | `fromModel` / `loadAssigneeUsers` 扩展，hydrate 外部 ID |
| `internal/task/model.go` | `AssigneeInfo` 增加 `ExternalIDs` 字段 |
| `internal/task/json.go` | `JSONAssignee` 增加 `ExternalIDs`，export/import 扩展 |
| `internal/app/workspace.go` | `resolveUser` 扩展、`UserView` 扩展、新增 `BindExternalID` / `UnbindExternalID` |
| `internal/app/service.go` | assignee hydration 扩展 |
| `internal/cli/user.go` | 新增 `bind` / `unbind` 子命令，`info` / `list` 输出扩展 |
| `internal/httpapi/users.go` | 新增外部 ID CRUD handler，response 扩展 |
| `internal/httpapi/router.go` | 注册外部 ID 路由 |
| `internal/remote/user.go` | 新增外部 ID CRUD 方法，DTO 扩展 |
| `internal/mcpserver/tools_user.go` | 新增 `user.bind` / `user.unbind` tool |
| `internal/mcpserver/tools_views.go` | `userView` struct 和转换函数扩展 |
| `internal/render/table.go` | `TaskInfo` / `formatAssignees` 可选展示外部 ID |
| `docs/openapi/taskg-v1.yaml` | 新增外部 ID endpoint schema |

### 测试文件

| 文件 | 测试内容 |
|---|---|
| `internal/storage/db_test.go` | `UserExternalID` 表 migration 验证 |
| `internal/storage/user_repo_test.go` | `GetByExternalID` 单元测试 |
| `internal/storage/external_id_repo_test.go` | 外部 ID CRUD 单元测试 |
| `internal/storage/task_repo_test.go` | assignee 附带外部 ID 的 hydration 测试 |
| `internal/task/json_test.go` | JSON assignee export/import 外部 ID 测试 |
| `internal/app/service_test.go` | `resolveUser` 外部 ID 解析、`BindExternalID` / `UnbindExternalID`、assignee 外部 ID 测试 |
| `internal/httpapi/users_test.go` | 外部 ID CRUD endpoint 测试 |
| `internal/mcpserver/integration_test.go` | `user.bind` / `user.unbind` tool 测试 |
| `internal/mcpserver/schema_test.go` | golden file 更新 |
| `tests/integration/cli_test.go` | CLI `user bind/unbind` 和外部 ID assign 测试 |

---

## Chunk 1: 数据层 — model、migration、repo

### Task 1: 新增 UserExternalID model 和 migration

**Files:**
- Modify: `internal/storage/models.go` (末尾追加)
- Modify: `internal/storage/db.go:173`
- Test: `internal/storage/db_test.go`

- [ ] **Step 1: 在 models.go 末尾新增 UserExternalID struct**

在 `internal/storage/models.go` 末尾追加：

```go
type UserExternalID struct {
	ID          string `gorm:"primaryKey"`
	UserID      string `gorm:"not null;index:idx_user_ext_id_user"`
	Provider    string `gorm:"not null;uniqueIndex:idx_user_ext_id_provider_value,priority:1"`
	ExternalID  string `gorm:"not null;uniqueIndex:idx_user_ext_id_provider_value,priority:2"`
	CreatedAt   int64  `gorm:"not null"`
}
```

- [ ] **Step 2: 在 db.go AutoMigrate 注册新表**

在 `internal/storage/db.go:173` 的第一个 AutoMigrate 调用中加入 `&UserExternalID{}`：

```go
if err := s.db.AutoMigrate(&Meta{}, &User{}, &Workspace{}, &Membership{}, &AuditLog{}, &Project{}, &Config{}, &ApiToken{}, &Context{}, &UDADefinition{}, &HookDefinition{}, &HookDelivery{}, &UserExternalID{}); err != nil {
```

- [ ] **Step 3: 写 db_test.go 验证表已迁移**

在 `internal/storage/db_test.go` 新增测试：

```go
func TestUserExternalIDTableMigrated(t *testing.T) {
	_, db := setupTestDB(t)
	if !db.Migrator().HasTable(&UserExternalID{}) {
		t.Fatal("expected user_external_ids table to exist")
	}
}
```

- [ ] **Step 4: 运行测试验证**

Run: `CGO_ENABLED=0 go test ./internal/storage/ -run TestUserExternalIDTableMigrated -v`
Expected: PASS

- [ ] **Step 5: 运行全量 storage 测试确认无破坏**

Run: `CGO_ENABLED=0 go test ./internal/storage/ -v`
Expected: 全部 PASS

- [ ] **Step 6: 提交**

```bash
git add internal/storage/models.go internal/storage/db.go internal/storage/db_test.go
git commit -m "feat(m11): 新增 UserExternalID model 和 migration"
```

---

### Task 2: 新增 ExternalIDRepository

**Files:**
- Create: `internal/storage/external_id_repo.go`
- Test: `internal/storage/external_id_repo_test.go`

- [ ] **Step 1: 写测试**

创建 `internal/storage/external_id_repo_test.go`：

```go
package sqlite

import (
	"testing"

	"github.com/google/uuid"
)

func TestExternalIDRepoCreateAndGet(t *testing.T) {
	_, db := setupTestDB(t)
	repo := NewExternalIDRepository(db)

	user := mustCreateUserRecord(t, nil, User{
		ID:   uuid.NewString(),
		Name: "alice",
	})

	extID := UserExternalID{
		ID:         uuid.NewString(),
		UserID:     user.ID,
		Provider:   "feishu",
		ExternalID: "ou_abc123",
		CreatedAt:  1700000000,
	}

	created, err := repo.Create(extID)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected non-empty ID")
	}

	found, err := repo.GetByProviderAndExternalID("feishu", "ou_abc123")
	if err != nil {
		t.Fatalf("get by provider+external_id: %v", err)
	}
	if found.UserID != user.ID {
		t.Fatalf("expected user_id %s, got %s", user.ID, found.UserID)
	}
}

func TestExternalIDRepoCreateDuplicateFails(t *testing.T) {
	_, db := setupTestDB(t)
	repo := NewExternalIDRepository(db)

	user := mustCreateUserRecord(t, nil, User{
		ID:   uuid.NewString(),
		Name: "bob",
	})

	extID := UserExternalID{
		ID:         uuid.NewString(),
		UserID:     user.ID,
		Provider:   "feishu",
		ExternalID: "ou_dup",
		CreatedAt:  1700000000,
	}
	if _, err := repo.Create(extID); err != nil {
		t.Fatalf("first create: %v", err)
	}

	extID2 := UserExternalID{
		ID:         uuid.NewString(),
		UserID:     user.ID,
		Provider:   "feishu",
		ExternalID: "ou_dup",
		CreatedAt:  1700000001,
	}
	if _, err := repo.Create(extID2); err == nil {
		t.Fatal("expected duplicate (provider, external_id) to fail")
	}
}

func TestExternalIDRepoListByUser(t *testing.T) {
	_, db := setupTestDB(t)
	repo := NewExternalIDRepository(db)

	user := mustCreateUserRecord(t, nil, User{
		ID:   uuid.NewString(),
		Name: "carol",
	})

	for _, ext := range []UserExternalID{
		{ID: uuid.NewString(), UserID: user.ID, Provider: "feishu", ExternalID: "ou_carol", CreatedAt: 1700000000},
		{ID: uuid.NewString(), UserID: user.ID, Provider: "slack", ExternalID: "U_CAROL", CreatedAt: 1700000001},
	} {
		if _, err := repo.Create(ext); err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	ids, err := repo.ListByUser(user.ID)
	if err != nil {
		t.Fatalf("list by user: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 external IDs, got %d", len(ids))
	}
}

func TestExternalIDRepoDelete(t *testing.T) {
	_, db := setupTestDB(t)
	repo := NewExternalIDRepository(db)

	user := mustCreateUserRecord(t, nil, User{
		ID:   uuid.NewString(),
		Name: "dave",
	})

	_, err := repo.Create(UserExternalID{
		ID: uuid.NewString(), UserID: user.ID, Provider: "feishu", ExternalID: "ou_dave", CreatedAt: 1700000000,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := repo.Delete(user.ID, "feishu", "ou_dave"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err = repo.GetByProviderAndExternalID("feishu", "ou_dave")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got: %v", err)
	}
}

func TestExternalIDRepoListByUsers(t *testing.T) {
	_, db := setupTestDB(t)
	repo := NewExternalIDRepository(db)

	user1 := mustCreateUserRecord(t, nil, User{ID: uuid.NewString(), Name: "eve"})
	user2 := mustCreateUserRecord(t, nil, User{ID: uuid.NewString(), Name: "frank"})

	repo.Create(UserExternalID{ID: uuid.NewString(), UserID: user1.ID, Provider: "feishu", ExternalID: "ou_eve", CreatedAt: 1700000000})
	repo.Create(UserExternalID{ID: uuid.NewString(), UserID: user1.ID, Provider: "slack", ExternalID: "U_EVE", CreatedAt: 1700000001})
	repo.Create(UserExternalID{ID: uuid.NewString(), UserID: user2.ID, Provider: "feishu", ExternalID: "ou_frank", CreatedAt: 1700000002})

	result, err := repo.ListByUsers([]string{user1.ID, user2.ID})
	if err != nil {
		t.Fatalf("list by users: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 external IDs, got %d", len(result))
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `CGO_ENABLED=0 go test ./internal/storage/ -run TestExternalIDRepo -v`
Expected: FAIL（文件不存在）

- [ ] **Step 3: 实现 ExternalIDRepository**

创建 `internal/storage/external_id_repo.go`：

```go
package sqlite

import (
	"errors"

	"gorm.io/gorm"
)

type ExternalIDRepository struct {
	db *gorm.DB
}

func NewExternalIDRepository(db *gorm.DB) *ExternalIDRepository {
	return &ExternalIDRepository{db: db}
}

func (r *ExternalIDRepository) Create(extID UserExternalID) (UserExternalID, error) {
	if err := r.db.Create(&extID).Error; err != nil {
		return UserExternalID{}, err
	}
	return extID, nil
}

func (r *ExternalIDRepository) GetByProviderAndExternalID(provider, externalID string) (UserExternalID, error) {
	var extID UserExternalID
	err := r.db.Where("provider = ? AND external_id = ?", provider, externalID).First(&extID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return UserExternalID{}, ErrNotFound
	}
	if err != nil {
		return UserExternalID{}, err
	}
	return extID, nil
}

func (r *ExternalIDRepository) ListByUser(userID string) ([]UserExternalID, error) {
	var ids []UserExternalID
	if err := r.db.Where("user_id = ?", userID).Order("provider ASC, external_id ASC").Find(&ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *ExternalIDRepository) ListByUsers(userIDs []string) ([]UserExternalID, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	var ids []UserExternalID
	if err := r.db.Where("user_id IN ?", userIDs).Order("user_id ASC, provider ASC, external_id ASC").Find(&ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *ExternalIDRepository) Delete(userID, provider, externalID string) error {
	result := r.db.Where("user_id = ? AND provider = ? AND external_id = ?", userID, provider, externalID).Delete(&UserExternalID{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
```

- [ ] **Step 4: 在 user_repo.go 新增 GetByExternalID**

在 `internal/storage/user_repo.go` 的 `UserRepository` 上新增方法。这需要组合 `ExternalIDRepository`，或者直接在 `UserRepository` 上加一个 `GetByExternalID` 方法。

实际上 `resolveUser` 需要通过外部 ID 找到 user，最直接的做法是在 `UserRepository` 上新增方法：

在 `internal/storage/user_repo.go` 追加：

```go
func (r *UserRepository) GetByExternalID(provider, externalID string) (User, error) {
	var extID UserExternalID
	if err := r.db.Where("provider = ? AND external_id = ?", provider, externalID).First(&extID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return User{}, ErrNotFound
		}
		return User{}, err
	}
	return r.GetByID(extID.UserID)
}
```

- [ ] **Step 5: 运行测试确认通过**

Run: `CGO_ENABLED=0 go test ./internal/storage/ -run TestExternalIDRepo -v`
Expected: 全部 PASS

- [ ] **Step 6: 运行全量 storage 测试**

Run: `CGO_ENABLED=0 go test ./internal/storage/ -v`
Expected: 全部 PASS

- [ ] **Step 7: 提交**

```bash
git add internal/storage/external_id_repo.go internal/storage/external_id_repo_test.go internal/storage/user_repo.go
git commit -m "feat(m11): 新增 ExternalIDRepository 和 UserRepository.GetByExternalID"
```

---

### Task 3: 领域模型扩展 — AssigneeInfo.ExternalIDs 和 JSON DTO

**Files:**
- Modify: `internal/task/model.go:31-35`
- Modify: `internal/task/json.go:16-20, 222-234, 312-325`
- Test: `internal/task/json_test.go`

- [ ] **Step 1: 在 model.go 新增 ExternalIDInfo 和扩展 AssigneeInfo**

在 `internal/task/model.go` 中，在 `AssigneeInfo` 定义之前新增：

```go
type ExternalIDInfo struct {
	Provider   string
	ExternalID string
}
```

然后修改 `AssigneeInfo`：

```go
type AssigneeInfo struct {
	UserID      string
	Name        string
	Email       *string
	ExternalIDs []ExternalIDInfo
}
```

- [ ] **Step 2: 扩展 JSONAssignee**

在 `internal/task/json.go` 中扩展 `JSONAssignee`：

```go
type JSONExternalID struct {
	Provider   string `json:"provider"`
	ExternalID string `json:"external_id"`
}

type JSONAssignee struct {
	UserID      string           `json:"user_id,omitempty"`
	Name        string           `json:"name,omitempty"`
	Email       *string          `json:"email,omitempty"`
	ExternalIDs []JSONExternalID `json:"external_ids,omitempty"`
}
```

- [ ] **Step 3: 扩展 ToJSON 中的 assignee 构建**

在 `internal/task/json.go` 的 `ToJSON` 函数中，`Assignees` 构建块（约 222-234 行）改为：

```go
Assignees: func() []JSONAssignee {
	if tsk.Assignees == nil {
		return nil
	}
	out := make([]JSONAssignee, len(tsk.Assignees))
	for i, assignee := range tsk.Assignees {
		a := JSONAssignee{
			UserID: assignee.UserID,
			Name:   assignee.Name,
			Email:  assignee.Email,
		}
		if len(assignee.ExternalIDs) > 0 {
			a.ExternalIDs = make([]JSONExternalID, len(assignee.ExternalIDs))
			for j, eid := range assignee.ExternalIDs {
				a.ExternalIDs[j] = JSONExternalID{Provider: eid.Provider, ExternalID: eid.ExternalID}
			}
		}
		out[i] = a
	}
	return out
}(),
```

- [ ] **Step 4: 扩展 FromJSONStrict 中的 Assignees 构建**

在 `FromJSONStrict` 中（约 312-325 行），assignee 构建改为：

```go
Assignees: func() []AssigneeInfo {
	if dto.Assignees == nil {
		return nil
	}
	out := make([]AssigneeInfo, len(dto.Assignees))
	for i, assignee := range dto.Assignees {
		info := AssigneeInfo{
			UserID: assignee.UserID,
			Name:   assignee.Name,
			Email:  assignee.Email,
		}
		if len(assignee.ExternalIDs) > 0 {
			info.ExternalIDs = make([]ExternalIDInfo, len(assignee.ExternalIDs))
			for j, eid := range assignee.ExternalIDs {
				info.ExternalIDs[j] = ExternalIDInfo{Provider: eid.Provider, ExternalID: eid.ExternalID}
			}
		}
		out[i] = info
	}
	return out
}(),
```

- [ ] **Step 5: 写 JSON export/import 测试**

在 `internal/task/json_test.go` 追加测试：

```go
func TestJSONTaskExportsAssigneeExternalIDs(t *testing.T) {
	email := "alice@example.com"
	tsk := Task{
		UUID:        "u1",
		Description: "test",
		Status:      StatusPending,
		Entry:       1700000000,
		Modified:    1700000000,
		Assignees: []AssigneeInfo{
			{
				UserID: "user-1",
				Name:   "alice",
				Email:  &email,
				ExternalIDs: []ExternalIDInfo{
					{Provider: "feishu", ExternalID: "ou_abc"},
					{Provider: "slack", ExternalID: "U_ABC"},
				},
			},
		},
	}
	dto := ToJSON(tsk)
	if len(dto.Assignees) != 1 {
		t.Fatalf("expected 1 assignee, got %d", len(dto.Assignees))
	}
	if len(dto.Assignees[0].ExternalIDs) != 2 {
		t.Fatalf("expected 2 external IDs, got %d", len(dto.Assignees[0].ExternalIDs))
	}
	if dto.Assignees[0].ExternalIDs[0].Provider != "feishu" {
		t.Fatalf("expected provider feishu, got %s", dto.Assignees[0].ExternalIDs[0].Provider)
	}
}

func TestJSONTaskImportPreservesAssigneeExternalIDs(t *testing.T) {
	dto := JSONTask{
		UUID:        "u2",
		Description: "test",
		Status:      StatusPending,
		Entry:       "1700000000",
		Modified:    "1700000000",
		Assignees: []JSONAssignee{
			{
				UserID: "user-1",
				Name:   "bob",
				ExternalIDs: []JSONExternalID{
					{Provider: "feishu", ExternalID: "ou_bob"},
				},
			},
		},
	}
	tsk := FromJSON(dto)
	if len(tsk.Assignees) != 1 {
		t.Fatalf("expected 1 assignee, got %d", len(tsk.Assignees))
	}
	if len(tsk.Assignees[0].ExternalIDs) != 1 {
		t.Fatalf("expected 1 external ID, got %d", len(tsk.Assignees[0].ExternalIDs))
	}
	if tsk.Assignees[0].ExternalIDs[0].Provider != "feishu" {
		t.Fatalf("expected provider feishu, got %s", tsk.Assignees[0].ExternalIDs[0].Provider)
	}
}
```

- [ ] **Step 6: 运行测试**

Run: `CGO_ENABLED=0 go test ./internal/task/ -run TestJSONTask -v`
Expected: 全部 PASS（包括原有测试）

- [ ] **Step 7: 提交**

```bash
git add internal/task/model.go internal/task/json.go internal/task/json_test.go
git commit -m "feat(m11): AssigneeInfo 和 JSON DTO 扩展 ExternalIDs"
```

---

## Chunk 2: App 层 — resolveUser 扩展、绑定/解绑、hydrate

### Task 4: App service — resolveUser 扩展支持 provider:value

**Files:**
- Modify: `internal/app/workspace.go:592-608`
- Test: `internal/app/service_test.go`

- [ ] **Step 1: 写 resolveUser 外部 ID 解析测试**

在 `internal/app/service_test.go` 追加：

```go
func TestResolveUserByExternalID(t *testing.T) {
	fixture := setupServiceTest(t)
	user := fixture.createUser("alice", "")

	extRepo := sqlite.NewExternalIDRepository(fixture.store.DB())
	_, err := extRepo.Create(sqlite.UserExternalID{
		ID: uuid.NewString(), UserID: user.ID, Provider: "feishu", ExternalID: "ou_abc", CreatedAt: 1700000000,
	})
	require.NoError(t, err)

	resolved, err := fixture.service.resolveUser("feishu:ou_abc")
	require.NoError(t, err)
	if resolved.ID != user.ID {
		t.Fatalf("expected user %s, got %s", user.ID, resolved.ID)
	}
}

func TestResolveUserByExternalIDNotFound(t *testing.T) {
	fixture := setupServiceTest(t)
	fixture.createUser("alice", "")

	_, err := fixture.service.resolveUser("feishu:ou_nonexist")
	if err == nil {
		t.Fatal("expected error for non-existent external ID")
	}
	rtErr, ok := err.(app.RuntimeError)
	if !ok {
		t.Fatalf("expected RuntimeError, got %T", err)
	}
	if rtErr.Code != "user_not_found" {
		t.Fatalf("expected user_not_found, got %s", rtErr.Code)
	}
}
```

注意：测试需要能访问 `resolveUser`（它是 Service 的私有方法）。查看现有测试模式，确认 `fixture.service` 是否可以直接调用。如果 `resolveUser` 是私有方法，测试需要放在 `internal/app` 包内或导出一个测试辅助函数。

查看现有测试：`TestServiceAddResolvesAssigneesInWorkspace` 已经在 `service_test.go` 中间接测试了 `resolveAssigneeRef`，说明测试文件在同一个 `app` 包内。因此可以直接调用 `fixture.service.resolveUser`。

需要确保测试文件导入正确，包括 `sqlite.NewExternalIDRepository`。由于 `app` 包已经导入 `sqlite`，需要检查测试文件是否可以直接构造 repo。

查看 `setupServiceTest` 的实现，确认 `fixture.store` 是否暴露了 `*gorm.DB`。需要检查：

- [ ] **Step 2: 确认测试基础设施**

查看 `setupServiceTest` 返回的 fixture 是否暴露了 `store` 或其 `DB()`。如果没有，需要给 `Store` 添加一个 `DB()` 访问器，或者用其他方式构造 `ExternalIDRepository`。

在 `internal/storage/store.go` 或 `db.go` 中查看 `Store` 是否有暴露 `db` 的方法。

如果没有，在 `internal/storage/db.go` 的 `Store` struct 上新增：

```go
func (s *Store) DB() *gorm.DB {
	return s.db
}
```

- [ ] **Step 3: 实现 resolveUser 扩展**

在 `internal/app/workspace.go` 的 `resolveUser` 方法中，在 UUID 匹配后、email 匹配前，插入外部 ID 解析：

```go
func (s *Service) resolveUser(ref string) (sqlite.User, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return sqlite.User{}, fmt.Errorf("user reference is required")
	}
	if user, err := s.userRepo.GetByID(ref); err == nil {
		return user, nil
	}
	if idx := strings.Index(ref, ":"); idx > 0 {
		provider := ref[:idx]
		externalID := ref[idx+1:]
		if provider != "" && externalID != "" {
			if user, err := s.userRepo.GetByExternalID(provider, externalID); err == nil {
				return user, nil
			}
		}
	}
	if user, err := s.userRepo.GetByName(ref); err == nil {
		return user, nil
	}
	user, err := s.userRepo.GetByEmail(ref)
	if err == sqlite.ErrNotFound {
		return sqlite.User{}, RuntimeError{Code: "user_not_found", Message: fmt.Sprintf("user %q not found", ref)}
	}
	return user, err
}
```

关键点：`strings.Index` 找第一个 `:`，而不是 `strings.Split`。这样 `email:user@domain.com` 中 `@` 不影响解析。

- [ ] **Step 4: 运行测试**

Run: `CGO_ENABLED=0 go test ./internal/app/ -run TestResolveUser -v`
Expected: PASS

- [ ] **Step 5: 运行全量 app 测试确认无破坏**

Run: `CGO_ENABLED=0 go test ./internal/app/ -v`
Expected: 全部 PASS

- [ ] **Step 6: 提交**

```bash
git add internal/app/workspace.go internal/app/service_test.go internal/storage/db.go
git commit -m "feat(m11): resolveUser 支持 provider:value 外部 ID 解析"
```

---

### Task 5: App service — BindExternalID / UnbindExternalID

**Files:**
- Modify: `internal/app/workspace.go` (新增方法)
- Modify: `internal/app/service.go:20-44` (Service struct 新增 extIDRepo)
- Test: `internal/app/service_test.go`

- [ ] **Step 1: 在 Service struct 新增 extIDRepo 字段**

在 `internal/app/service.go` 的 `Service` struct 中（约 20-44 行），在 `hookDeliveryRepo` 之后新增：

```go
extIDRepo *sqlite.ExternalIDRepository
```

需要找到 Service 初始化的地方，确保 `extIDRepo` 被赋值。搜索 `Service{` 的构造位置。

- [ ] **Step 2: 在 Service 初始化中赋值 extIDRepo**

找到构造 `Service` 的地方（通常在 `buildServiceFromCmd` 或类似函数中），确保从 `Store` 创建 `ExternalIDRepository` 并赋给 `extIDRepo`。

具体位置需要搜索代码。检查 `internal/cli` 中 `buildService` 或 `internal/app` 中 `NewService` 的模式。

- [ ] **Step 3: 写绑定/解绑测试**

在 `internal/app/service_test.go` 追加：

```go
func TestBindExternalID(t *testing.T) {
	fixture := setupServiceTest(t)
	user := fixture.createUser("alice", "")
	fixture.service.runtime.ActorUserID = user.ID

	err := fixture.service.BindExternalID(user.ID, "feishu", "ou_alice")
	require.NoError(t, err)

	extIDs, err := fixture.service.extIDRepo.ListByUser(user.ID)
	require.NoError(t, err)
	if len(extIDs) != 1 {
		t.Fatalf("expected 1 external ID, got %d", len(extIDs))
	}
	if extIDs[0].Provider != "feishu" || extIDs[0].ExternalID != "ou_alice" {
		t.Fatalf("unexpected external ID: %+v", extIDs[0])
	}
}

func TestBindExternalIDRejectsDuplicate(t *testing.T) {
	fixture := setupServiceTest(t)
	user := fixture.createUser("alice", "")

	err := fixture.service.BindExternalID(user.ID, "feishu", "ou_dup")
	require.NoError(t, err)

	err = fixture.service.BindExternalID(user.ID, "feishu", "ou_dup")
	if err == nil {
		t.Fatal("expected duplicate bind to fail")
	}
}

func TestBindExternalIDRejectsOtherUserForNonAdmin(t *testing.T) {
	fixture := setupServiceTest(t)
	user1 := fixture.createUser("alice", "")
	user2 := fixture.createUser("bob", "")
	fixture.service.runtime.ActorUserID = user1.ID
	fixture.service.runtime.Role = app.RoleMember

	err := fixture.service.BindExternalID(user2.ID, "feishu", "ou_bob")
	if err == nil {
		t.Fatal("expected non-admin binding other user to fail")
	}
}

func TestUnbindExternalID(t *testing.T) {
	fixture := setupServiceTest(t)
	user := fixture.createUser("alice", "")

	err := fixture.service.BindExternalID(user.ID, "feishu", "ou_unbind")
	require.NoError(t, err)

	err = fixture.service.UnbindExternalID(user.ID, "feishu", "ou_unbind")
	require.NoError(t, err)

	extIDs, err := fixture.service.extIDRepo.ListByUser(user.ID)
	require.NoError(t, err)
	if len(extIDs) != 0 {
		t.Fatalf("expected 0 external IDs after unbind, got %d", len(extIDs))
	}
}

func TestUnbindExternalIDNotFound(t *testing.T) {
	fixture := setupServiceTest(t)
	user := fixture.createUser("alice", "")

	err := fixture.service.UnbindExternalID(user.ID, "feishu", "ou_nonexist")
	if err == nil {
		t.Fatal("expected unbind of non-existent ID to fail")
	}
}
```

- [ ] **Step 4: 运行测试确认失败**

Run: `CGO_ENABLED=0 go test ./internal/app/ -run TestBindExternalID -v`
Expected: FAIL（方法不存在）

- [ ] **Step 5: 实现 BindExternalID 和 UnbindExternalID**

在 `internal/app/workspace.go` 追加：

```go
func (s *Service) BindExternalID(userID, provider, externalID string) error {
	provider = strings.TrimSpace(provider)
	externalID = strings.TrimSpace(externalID)
	if provider == "" || externalID == "" {
		return RuntimeError{Code: "invalid_input", Message: "provider and external_id are required"}
	}
	if userID != s.runtime.ActorUserID {
		if err := requireRolePermission(s.runtime.Role, PermissionWorkspaceModify); err != nil {
			return RuntimeError{Code: "permission_denied", Message: "only admin/owner can bind external IDs for other users"}
		}
	}
	return s.withAudit("user.bind_external_id", func(tx *Service) (AuditEntry, error) {
		_, err := tx.extIDRepo.Create(sqlite.UserExternalID{
			ID:         uuid.NewString(),
			UserID:     userID,
			Provider:   provider,
			ExternalID: externalID,
			CreatedAt:  s.clock.Unix(),
		})
		if err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{
			TargetType: "user",
			TargetID:   userID,
			Payload:    map[string]any{"provider": provider, "external_id": externalID},
		}, nil
	})
}

func (s *Service) UnbindExternalID(userID, provider, externalID string) error {
	provider = strings.TrimSpace(provider)
	externalID = strings.TrimSpace(externalID)
	if provider == "" || externalID == "" {
		return RuntimeError{Code: "invalid_input", Message: "provider and external_id are required"}
	}
	if userID != s.runtime.ActorUserID {
		if err := requireRolePermission(s.runtime.Role, PermissionWorkspaceModify); err != nil {
			return RuntimeError{Code: "permission_denied", Message: "only admin/owner can unbind external IDs for other users"}
		}
	}
	return s.withAudit("user.unbind_external_id", func(tx *Service) (AuditEntry, error) {
		if err := tx.extIDRepo.Delete(userID, provider, externalID); err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{
			TargetType: "user",
			TargetID:   userID,
			Payload:    map[string]any{"provider": provider, "external_id": externalID},
		}, nil
	})
}

func (s *Service) ListExternalIDs(userID string) ([]task.ExternalIDInfo, error) {
	rows, err := s.extIDRepo.ListByUser(userID)
	if err != nil {
		return nil, err
	}
	out := make([]task.ExternalIDInfo, len(rows))
	for i, row := range rows {
		out[i] = task.ExternalIDInfo{Provider: row.Provider, ExternalID: row.ExternalID}
	}
	return out, nil
}
```

- [ ] **Step 6: 运行测试**

Run: `CGO_ENABLED=0 go test ./internal/app/ -run "TestBindExternalID|TestUnbindExternalID" -v`
Expected: 全部 PASS

- [ ] **Step 7: 提交**

```bash
git add internal/app/workspace.go internal/app/service.go internal/app/service_test.go
git commit -m "feat(m11): BindExternalID / UnbindExternalID / ListExternalIDs"
```

---

### Task 6: App 层 — UserView 和 hydrate 扩展

**Files:**
- Modify: `internal/app/workspace.go` (UserView 扩展、userViewFromRow 扩展、ListUsers/UserInfo hydrate)
- Modify: `internal/storage/task_repo.go:334-364` (fromModel hydrate external IDs)

- [ ] **Step 1: 扩展 UserView struct**

在 `internal/app/workspace.go` 的 `UserView` struct 中新增：

```go
ExternalIDs []task.ExternalIDInfo
```

- [ ] **Step 2: 修改 userViewFromRow 接受 externalIDs 参数**

修改 `userViewFromRow` 签名和实现：

```go
func userViewFromRow(user sqlite.User, active bool, externalIDs []task.ExternalIDInfo) UserView {
	return UserView{
		ID:                 user.ID,
		Name:               user.Name,
		Email:              user.Email,
		DefaultWorkspaceID: user.DefaultWorkspaceID,
		ExternalIDs:        externalIDs,
		Active:             active,
		CreatedAt:          user.CreatedAt,
		ModifiedAt:         user.ModifiedAt,
	}
}
```

注意：这需要更新所有调用 `userViewFromRow` 的地方。搜索所有调用点并逐一更新。

- [ ] **Step 3: 修改 ListUsers 和 UserInfo hydrate 外部 ID**

在 `ListUsers` 中，获取所有用户后批量加载外部 ID：

```go
func (s *Service) ListUsers() ([]UserView, error) {
	users, err := s.userRepo.List()
	if err != nil {
		return nil, err
	}
	userIDs := make([]string, len(users))
	for i, u := range users {
		userIDs[i] = u.ID
	}
	extByUser, err := s.loadExternalIDsByUsers(userIDs)
	if err != nil {
		return nil, err
	}
	out := make([]UserView, 0, len(users))
	for _, user := range users {
		out = append(out, userViewFromRow(user, user.ID == s.runtime.ActorUserID, extByUser[user.ID]))
	}
	return out, nil
}
```

类似修改 `UserInfo`。

新增辅助方法：

```go
func (s *Service) loadExternalIDsByUsers(userIDs []string) (map[string][]task.ExternalIDInfo, error) {
	rows, err := s.extIDRepo.ListByUsers(userIDs)
	if err != nil {
		return nil, err
	}
	result := make(map[string][]task.ExternalIDInfo)
	for _, row := range rows {
		result[row.UserID] = append(result[row.UserID], task.ExternalIDInfo{
			Provider:   row.Provider,
			ExternalID: row.ExternalID,
		})
	}
	return result, nil
}
```

- [ ] **Step 4: 扩展 task_repo.go 的 fromModel hydrate external IDs**

在 `internal/storage/task_repo.go` 的 `fromModel` 方法中，assignee 构建部分需要加载外部 ID。当前模式是 `loadAssigneeUsers` 返回 `map[string]User`。

最简洁的做法是新增一个 `loadExternalIDsForUsers` 方法，或者在 `loadAssigneeUsers` 中一并加载外部 ID。

修改 `loadAssigneeUsers`，返回值改为包含外部 ID 信息：

```go
type assigneeUserData struct {
	Name        string
	Email       *string
	ExternalIDs []domain.ExternalIDInfo
}

func (r *TaskRepository) loadAssigneeUsers(models []Task) (map[string]assigneeUserData, error) {
	// ... 收集 userIDs ...
	var users []User
	if err := r.db.Where("id IN ?", userIDs).Find(&users).Error; err != nil {
		return nil, err
	}

	var extIDs []UserExternalID
	if len(userIDs) > 0 {
		if err := r.db.Where("user_id IN ?", userIDs).Find(&extIDs).Error; err != nil {
			return nil, err
		}
	}
	extByUser := make(map[string][]domain.ExternalIDInfo)
	for _, eid := range extIDs {
		extByUser[eid.UserID] = append(extByUser[eid.UserID], domain.ExternalIDInfo{
			Provider:   eid.Provider,
			ExternalID: eid.ExternalID,
		})
	}

	result := make(map[string]assigneeUserData, len(users))
	for _, user := range users {
		result[user.ID] = assigneeUserData{
			Name:        user.Name,
			Email:       user.Email,
			ExternalIDs: extByUser[user.ID],
		}
	}
	return result, nil
}
```

然后 `fromModel` 中使用新的数据结构。

**注意**：这个改动影响面较大（`fromModel` 的签名、所有调用 `loadAssigneeUsers` 的地方）。需要仔细更新。

- [ ] **Step 5: 运行全量测试**

Run: `CGO_ENABLED=0 go test ./internal/storage/ ./internal/app/ ./internal/task/ -v`
Expected: 全部 PASS

- [ ] **Step 6: 提交**

```bash
git add internal/app/workspace.go internal/storage/task_repo.go
git commit -m "feat(m11): UserView 和 assignee hydrate 扩展 external IDs"
```

---

## Chunk 3: CLI — bind/unbind/info/list

### Task 7: CLI — user bind/unbind 子命令和 info/list 输出扩展

**Files:**
- Modify: `internal/cli/user.go`
- Test: `tests/integration/cli_test.go`

- [ ] **Step 1: 在 user.go 新增 bind 和 unbind 子命令**

在 `newUserCommand` 中注册新子命令：

```go
cmd.AddCommand(newUserBindCommand(opts))
cmd.AddCommand(newUserUnbindCommand(opts))
```

新增命令实现：

```go
func newUserBindCommand(opts Options) *cobra.Command {
	var userRef string
	cmd := &cobra.Command{
		Use:  "bind <provider:external_id>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			provider, externalID, ok := parseProviderExternalID(args[0])
			if !ok {
				return fmt.Errorf("invalid external ID format %q; expected provider:external_id", args[0])
			}
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				ref := userRef
				if ref == "" {
					ref = "local"
				}
				return client.BindExternalID(context.Background(), ref, provider, externalID)
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			targetUserID := svc.Runtime().ActorUserID
			if userRef != "" {
				user, err := svc.UserInfo(userRef)
				if err != nil {
					return err
				}
				targetUserID = user.ID
			}
			if err := svc.BindExternalID(targetUserID, provider, externalID); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Bound %s:%s to user %s\n", provider, externalID, targetUserID)
			return nil
		},
	}
	cmd.Flags().StringVar(&userRef, "user", "", "target user (name, email, or UUID); defaults to current user")
	return cmd
}

func newUserUnbindCommand(opts Options) *cobra.Command {
	var userRef string
	cmd := &cobra.Command{
		Use:  "unbind <provider:external_id>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			provider, externalID, ok := parseProviderExternalID(args[0])
			if !ok {
				return fmt.Errorf("invalid external ID format %q; expected provider:external_id", args[0])
			}
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				ref := userRef
				if ref == "" {
					ref = "local"
				}
				return client.UnbindExternalID(context.Background(), ref, provider, externalID)
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			targetUserID := svc.Runtime().ActorUserID
			if userRef != "" {
				user, err := svc.UserInfo(userRef)
				if err != nil {
					return err
				}
				targetUserID = user.ID
			}
			if err := svc.UnbindExternalID(targetUserID, provider, externalID); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Unbound %s:%s\n", provider, externalID)
			return nil
		},
	}
	cmd.Flags().StringVar(&userRef, "user", "", "target user (name, email, or UUID); defaults to current user")
	return cmd
}

func parseProviderExternalID(s string) (string, string, bool) {
	idx := strings.Index(s, ":")
	if idx <= 0 || idx == len(s)-1 {
		return "", "", false
	}
	return s[:idx], s[idx+1:], true
}
```

- [ ] **Step 2: 扩展 user info 输出**

在 `newUserInfoCommand` 的本地模式 human 输出部分，追加外部 ID 显示：

在 `fmt.Fprintf(cmd.OutOrStdout(), "Name: %s\nEmail: %s\n", user.Name, email)` 之后追加：

```go
if len(user.ExternalIDs) > 0 {
	fmt.Fprintf(cmd.OutOrStdout(), "External IDs:\n")
	for _, eid := range user.ExternalIDs {
		fmt.Fprintf(cmd.OutOrStdout(), "  %s:%s\n", eid.Provider, eid.ExternalID)
	}
}
```

对远程模式做同样处理（需要先确认远程 `UserInfo` 返回的 DTO 已包含 external IDs）。

- [ ] **Step 3: 扩展 userViewForJSON**

在 `userViewForJSON` 中追加 external IDs：

```go
func userViewForJSON(user app.UserView) map[string]any {
	extIDs := make([]map[string]string, 0, len(user.ExternalIDs))
	for _, eid := range user.ExternalIDs {
		extIDs = append(extIDs, map[string]string{"provider": eid.Provider, "external_id": eid.ExternalID})
	}
	return map[string]any{
		"id":                   user.ID,
		"name":                 user.Name,
		"email":                user.Email,
		"default_workspace_id": user.DefaultWorkspaceID,
		"external_ids":         extIDs,
		"active":               user.Active,
		"created_at":           user.CreatedAt,
		"modified_at":          user.ModifiedAt,
	}
}
```

- [ ] **Step 4: 写 CLI 集成测试**

在 `tests/integration/cli_test.go` 追加：

```go
func TestCLIUserBindAndUnbind(t *testing.T) {
	bin := buildTaskg(t)
	dir := t.TempDir()

	run := func(args ...string) string {
		args = append([]string{"--data-dir", dir}, args...)
		return mustRun(t, bin, args...)
	}

	run("user", "bind", "feishu:ou_test123")
	output := run("user", "info")
	if !strings.Contains(output, "feishu:ou_test123") {
		t.Fatalf("expected user info to show bound external ID, got:\n%s", output)
	}

	run("user", "unbind", "feishu:ou_test123")
	output = run("user", "info")
	if strings.Contains(output, "feishu:ou_test123") {
		t.Fatalf("expected user info to NOT show unbound external ID, got:\n%s", output)
	}
}

func TestCLIAssignByExternalID(t *testing.T) {
	bin := buildTaskg(t)
	dir := t.TempDir()

	run := func(args ...string) string {
		args = append([]string{"--data-dir", dir}, args...)
		return mustRun(t, bin, args...)
	}

	run("user", "bind", "feishu:ou_assign_test")
	run("add", "test task", "@feishu:ou_assign_test")
	output := run("--json", "list")
	if !strings.Contains(output, "ou_assign_test") {
		t.Fatalf("expected task list JSON to contain external ID, got:\n%s", output)
	}
}
```

- [ ] **Step 5: 运行测试**

Run: `CGO_ENABLED=0 go test ./tests/integration/ -run "TestCLIUserBind|TestCLIAssignByExternalID" -v`
Expected: PASS

- [ ] **Step 6: 运行全量集成测试确认无破坏**

Run: `CGO_ENABLED=0 go test ./tests/integration/ -v`
Expected: 全部 PASS

- [ ] **Step 7: 提交**

```bash
git add internal/cli/user.go tests/integration/cli_test.go
git commit -m "feat(m11): CLI user bind/unbind 和 info 输出扩展"
```

---

## Chunk 4: HTTP API 和 Remote Client

### Task 8: HTTP API — 外部 ID CRUD endpoint 和 response 扩展

**Files:**
- Modify: `internal/httpapi/users.go`
- Modify: `internal/httpapi/router.go`
- Modify: `internal/remote/user.go`
- Test: `internal/httpapi/users_test.go`

- [ ] **Step 1: 扩展 userResponse struct**

在 `internal/httpapi/users.go` 的 `userResponse` 中新增：

```go
ExternalIDs []externalIDResponse `json:"external_ids,omitempty"`
```

新增 response type：

```go
type externalIDResponse struct {
	Provider   string `json:"provider"`
	ExternalID string `json:"external_id"`
}
```

- [ ] **Step 2: 修改 userResponseFromView**

```go
func userResponseFromView(user app.UserView) userResponse {
	extIDs := make([]externalIDResponse, 0, len(user.ExternalIDs))
	for _, eid := range user.ExternalIDs {
		extIDs = append(extIDs, externalIDResponse{Provider: eid.Provider, ExternalID: eid.ExternalID})
	}
	return userResponse{
		ID:                 user.ID,
		Name:               user.Name,
		Email:              user.Email,
		DefaultWorkspaceID: user.DefaultWorkspaceID,
		ExternalIDs:        extIDs,
		Active:             user.Active,
		CreatedAt:          user.CreatedAt,
		ModifiedAt:         user.ModifiedAt,
	}
}
```

- [ ] **Step 3: 新增外部 ID CRUD handler**

在 `internal/httpapi/users.go` 新增：

```go
type bindExternalIDRequest struct {
	Provider   string `json:"provider"`
	ExternalID string `json:"external_id"`
}

func (s *Server) handleExternalIDBind(w http.ResponseWriter, r *http.Request) {
	userRef := chi.URLParam(r, "user")
	var req bindExternalIDRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	if req.Provider == "" || req.ExternalID == "" {
		writeError(w, http.StatusBadRequest, "invalid_input", "provider and external_id are required", nil)
		return
	}
	scoped, _, err := s.scopedService(r, "workspace:write", app.PermissionWorkspaceModify, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	user, err := scoped.UserInfo(userRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := scoped.BindExternalID(user.ID, req.Provider, req.ExternalID); err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, externalIDResponse{Provider: req.Provider, ExternalID: req.ExternalID}, nil)
}

func (s *Server) handleExternalIDUnbind(w http.ResponseWriter, r *http.Request) {
	userRef := chi.URLParam(r, "user")
	provider := chi.URLParam(r, "provider")
	externalID := chi.URLParam(r, "externalID")
	scoped, _, err := s.scopedService(r, "workspace:write", app.PermissionWorkspaceModify, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	user, err := scoped.UserInfo(userRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := scoped.UnbindExternalID(user.ID, provider, externalID); err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusNoContent, nil, nil)
}

func (s *Server) handleExternalIDList(w http.ResponseWriter, r *http.Request) {
	userRef := chi.URLParam(r, "user")
	scoped, _, err := s.scopedService(r, "workspace:read", app.PermissionWorkspaceRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	user, err := scoped.UserInfo(userRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	extIDs, err := scoped.ListExternalIDs(user.ID)
	if err != nil {
		writeAppError(w, err)
		return
	}
	out := make([]externalIDResponse, len(extIDs))
	for i, eid := range extIDs {
		out[i] = externalIDResponse{Provider: eid.Provider, ExternalID: eid.ExternalID}
	}
	writeSuccess(w, http.StatusOK, out, nil)
}
```

- [ ] **Step 4: 注册路由**

在 `internal/httpapi/router.go` 的路由注册中追加（在 `userInfo` 路由之后）：

```go
api.With(s.authMiddleware).Post("/api/v1/users/{user}/external-ids", s.handleExternalIDBind)
api.With(s.authMiddleware).Delete("/api/v1/users/{user}/external-ids/{provider}/{externalID}", s.handleExternalIDUnbind)
api.With(s.authMiddleware).Get("/api/v1/users/{user}/external-ids", s.handleExternalIDList)
```

- [ ] **Step 5: 扩展 remote client**

在 `internal/remote/user.go` 中：

扩展 `userDTO`：

```go
type externalIDDTO struct {
	Provider   string `json:"provider"`
	ExternalID string `json:"external_id"`
}

type userDTO struct {
	ID                 string           `json:"id"`
	Name               string           `json:"name"`
	Email              *string          `json:"email,omitempty"`
	DefaultWorkspaceID *string          `json:"default_workspace_id,omitempty"`
	ExternalIDs        []externalIDDTO  `json:"external_ids,omitempty"`
	Active             bool             `json:"active"`
	CreatedAt          int64            `json:"created_at"`
	ModifiedAt         int64            `json:"modified_at"`
}
```

修改 `userDTOToView` 转换外部 ID：

```go
func userDTOToView(row userDTO) app.UserView {
	extIDs := make([]task.ExternalIDInfo, 0, len(row.ExternalIDs))
	for _, eid := range row.ExternalIDs {
		extIDs = append(extIDs, task.ExternalIDInfo{Provider: eid.Provider, ExternalID: eid.ExternalID})
	}
	return app.UserView{
		ID:                 row.ID,
		Name:               row.Name,
		Email:              row.Email,
		DefaultWorkspaceID: row.DefaultWorkspaceID,
		ExternalIDs:        extIDs,
		Active:             row.Active,
		CreatedAt:          row.CreatedAt,
		ModifiedAt:         row.ModifiedAt,
	}
}
```

新增方法：

```go
func (c *Client) BindExternalID(ctx context.Context, userRef, provider, externalID string) error {
	path := "/api/v1/users/" + url.PathEscape(userRef) + "/external-ids"
	body := map[string]string{"provider": provider, "external_id": externalID}
	var envelope apiEnvelope[any]
	return c.post(ctx, path, body, &envelope)
}

func (c *Client) UnbindExternalID(ctx context.Context, userRef, provider, externalID string) error {
	path := "/api/v1/users/" + url.PathEscape(userRef) + "/external-ids/" + url.PathEscape(provider) + "/" + url.PathEscape(externalID)
	var envelope apiEnvelope[any]
	return c.delete(ctx, path, &envelope)
}
```

- [ ] **Step 6: 写 HTTP API 测试**

在 `internal/httpapi/users_test.go` 追加：

```go
func TestExternalIDBindUnbindAndList(t *testing.T) {
	fixture := setupHTTPTest(t)
	authHeader := fixture.authHeader

	body := `{"provider":"feishu","external_id":"ou_test_http"}`
	rr := requestHTTP(t, fixture.server, http.MethodPost, "/api/v1/users/local/external-ids", strings.NewReader(body), authHeader)
	if rr.Code != http.StatusCreated {
		t.Fatalf("bind: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	rr = requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/users/local/external-ids", nil, authHeader)
	if rr.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	rr = requestHTTP(t, fixture.server, http.MethodDelete, "/api/v1/users/local/external-ids/feishu/ou_test_http", nil, authHeader)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("unbind: expected 204, got %d: %s", rr.Code, rr.Body.String())
	}
}
```

- [ ] **Step 7: 运行测试**

Run: `CGO_ENABLED=0 go test ./internal/httpapi/ -run TestExternalID -v`
Expected: PASS

Run: `CGO_ENABLED=0 go test ./internal/httpapi/ -v`
Expected: 全部 PASS

- [ ] **Step 8: 提交**

```bash
git add internal/httpapi/users.go internal/httpapi/users_test.go internal/httpapi/router.go internal/remote/user.go
git commit -m "feat(m11): HTTP API 和 remote client 外部 ID 支持"
```

---

## Chunk 5: MCP tools 和 schema golden

### Task 9: MCP — user.bind / user.unbind tool 和 views 扩展

**Files:**
- Modify: `internal/mcpserver/tools_user.go`
- Modify: `internal/mcpserver/tools_views.go`
- Test: `internal/mcpserver/integration_test.go`
- Test: `internal/mcpserver/schema_test.go` (golden update)

- [ ] **Step 1: 扩展 userView struct**

在 `internal/mcpserver/tools_views.go` 的 `userView` 中新增：

```go
ExternalIDs []externalIDView `json:"external_ids,omitempty"`
```

新增 type：

```go
type externalIDView struct {
	Provider   string `json:"provider"`
	ExternalID string `json:"external_id"`
}
```

修改 `userViewFromApp`：

```go
func userViewFromApp(row app.UserView) userView {
	extIDs := make([]externalIDView, 0, len(row.ExternalIDs))
	for _, eid := range row.ExternalIDs {
		extIDs = append(extIDs, externalIDView{Provider: eid.Provider, ExternalID: eid.ExternalID})
	}
	return userView{
		ID:                 row.ID,
		Name:               row.Name,
		Email:              row.Email,
		DefaultWorkspaceID: row.DefaultWorkspaceID,
		ExternalIDs:        extIDs,
		Active:             row.Active,
		CreatedAt:          row.CreatedAt,
		ModifiedAt:         row.ModifiedAt,
	}
}
```

- [ ] **Step 2: 新增 MCP tool input struct 和 handler**

在 `internal/mcpserver/tools_user.go` 追加：

```go
type UserBindInput struct {
	User       string `json:"user"`
	Provider   string `json:"provider"`
	ExternalID string `json:"external_id"`
}

type UserUnbindInput struct {
	User       string `json:"user"`
	Provider   string `json:"provider"`
	ExternalID string `json:"external_id"`
}
```

在 `registerUserTools` 中注册：

```go
addTool(s, &mcp.Tool{Name: "user_bind", Description: "Bind an external ID (e.g. feishu:ou_xxxxx) to a taskg user. Admin/owner can bind for others; regular users can only bind to themselves."}, func(ctx context.Context, req *mcp.CallToolRequest, in UserBindInput) (*mcp.CallToolResult, ToolEnvelope, error) {
	svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{}, "workspace:write", app.PermissionWorkspaceModify)
	if err != nil {
		return businessErrorWithEnvelope(err)
	}
	user, err := svc.UserInfo(in.User)
	if err != nil {
		return businessErrorWithEnvelope(err)
	}
	if err := svc.BindExternalID(user.ID, in.Provider, in.ExternalID); err != nil {
		return businessErrorWithEnvelope(err)
	}
	return successWithEnvelope(
		map[string]any{"provider": in.Provider, "external_id": in.ExternalID},
		fmt.Sprintf("Bound %s:%s to %s", in.Provider, in.ExternalID, user.Name),
	)
})

addTool(s, &mcp.Tool{Name: "user_unbind", Description: "Unbind an external ID from a taskg user."}, func(ctx context.Context, req *mcp.CallToolRequest, in UserUnbindInput) (*mcp.CallToolResult, ToolEnvelope, error) {
	svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{}, "workspace:write", app.PermissionWorkspaceModify)
	if err != nil {
		return businessErrorWithEnvelope(err)
	}
	user, err := svc.UserInfo(in.User)
	if err != nil {
		return businessErrorWithEnvelope(err)
	}
	if err := svc.UnbindExternalID(user.ID, in.Provider, in.ExternalID); err != nil {
		return businessErrorWithEnvelope(err)
	}
	return successWithEnvelope(nil, fmt.Sprintf("Unbound %s:%s from %s", in.Provider, in.ExternalID, user.Name))
})
```

- [ ] **Step 3: 写 MCP integration 测试**

在 `internal/mcpserver/integration_test.go` 追加：

```go
func TestUserBindAndUnbind(t *testing.T) {
	fixture := setupMCPTest(t)

	_, env, err := fixture.callTool("user_bind", map[string]any{
		"user":        "local",
		"provider":    "feishu",
		"external_id": "ou_mcp_test",
	})
	if err != nil {
		t.Fatalf("user_bind: %v", err)
	}
	if env.Error != "" {
		t.Fatalf("user_bind error: %s", env.Error)
	}

	_, env, err = fixture.callTool("user_info", map[string]any{"user": "local"})
	if err != nil {
		t.Fatalf("user_info after bind: %v", err)
	}
	userData, ok := env.Data["user"].(map[string]any)
	if !ok {
		t.Fatal("expected user object in data")
	}
	extIDs, ok := userData["external_ids"].([]any)
	if !ok || len(extIDs) != 1 {
		t.Fatalf("expected 1 external ID, got %v", userData["external_ids"])
	}

	_, env, err = fixture.callTool("user_unbind", map[string]any{
		"user":        "local",
		"provider":    "feishu",
		"external_id": "ou_mcp_test",
	})
	if err != nil {
		t.Fatalf("user_unbind: %v", err)
	}
	if env.Error != "" {
		t.Fatalf("user_unbind error: %s", env.Error)
	}
}
```

- [ ] **Step 4: 更新 golden file**

Run: `CGO_ENABLED=0 go test ./internal/mcpserver/ -run TestListToolsDefaultServerHasTools -update -v`
Expected: PASS，golden file 被更新

- [ ] **Step 5: 运行全量 MCP 测试**

Run: `CGO_ENABLED=0 go test ./internal/mcpserver/ -v`
Expected: 全部 PASS

- [ ] **Step 6: 提交**

```bash
git add internal/mcpserver/tools_user.go internal/mcpserver/tools_views.go internal/mcpserver/integration_test.go internal/mcpserver/testdata/
git commit -m "feat(m11): MCP user.bind / user.unbind tool 和 views 扩展"
```

---

## Chunk 6: Render 扩展和端到端验证

### Task 10: Render — TaskInfo 展示外部 ID

**Files:**
- Modify: `internal/render/table.go:80-96`

- [ ] **Step 1: 扩展 formatAssignees 展示外部 ID**

在 `internal/render/table.go` 的 `formatAssignees` 中，如果有外部 ID，追加到名称后：

```go
func formatAssignees(assignees []task.AssigneeInfo) string {
	if len(assignees) == 0 {
		return ""
	}
	out := make([]string, 0, len(assignees))
	for _, assignee := range assignees {
		label := ""
		switch {
		case assignee.Name != "":
			label = assignee.Name
		case assignee.Email != nil && *assignee.Email != "":
			label = *assignee.Email
		case assignee.UserID != "":
			label = assignee.UserID
		}
		if len(assignee.ExternalIDs) > 0 {
			labels := make([]string, 0, len(assignee.ExternalIDs))
			for _, eid := range assignee.ExternalIDs {
				labels = append(labels, eid.Provider+":"+eid.ExternalID)
			}
			if label != "" {
				label += " [" + strings.Join(labels, ", ") + "]"
			} else {
				label = strings.Join(labels, ", ")
			}
		}
		if label != "" {
			out = append(out, "@"+label)
		}
	}
	return strings.Join(out, ", ")
}
```

- [ ] **Step 2: 运行 render 测试**

Run: `CGO_ENABLED=0 go test ./internal/render/ -v`
Expected: 全部 PASS（可能需要更新现有测试的期望值）

- [ ] **Step 3: 提交**

```bash
git add internal/render/table.go
git commit -m "feat(m11): TaskInfo human 输出展示 assignee 外部 ID"
```

---

### Task 11: 全量验证

- [ ] **Step 1: 运行全量测试**

```bash
CGO_ENABLED=0 go test ./... -v
```

Expected: 全部 PASS

- [ ] **Step 2: 运行构建验证**

```bash
CGO_ENABLED=0 go build ./cmd/taskg
```

Expected: 成功

- [ ] **Step 3: 手动端到端验证**

```bash
./taskg --data-dir /tmp/taskg-m11-test user bind feishu:ou_manual_test
./taskg --data-dir /tmp/taskg-m11-test user info
./taskg --data-dir /tmp/taskg-m11-test add "test task" @feishu:ou_manual_test
./taskg --data-dir /tmp/taskg-m11-test --json list
./taskg --data-dir /tmp/taskg-m11-test list assignee:feishu:ou_manual_test
./taskg --data-dir /tmp/taskg-m11-test user unbind feishu:ou_manual_test
```

Expected: 绑定 → info 可见 → add 成功 → list JSON 含 external_ids → query 成功 → unbind 成功

- [ ] **Step 4: 提交最终状态**

```bash
git add -A
git commit -m "feat(m11): 用户外部 ID 绑定 — 全量测试通过"
```

---

### Task 12: 文档同步

**Files:**
- Modify: `ROADMAP.md`
- Modify: `README.md`
- Modify: `docs/openapi/taskg-v1.yaml`

- [ ] **Step 1: 更新 ROADMAP.md**

在状态总览表中追加 M11 行，状态为已完成。新增 M11 详细描述段落。

- [ ] **Step 2: 更新 README.md**

在用户相关命令段落中补充 `user bind/unbind` 说明。

- [ ] **Step 3: 更新 OpenAPI schema**

在 `docs/openapi/taskg-v1.yaml` 中：
- 新增 `/api/v1/users/{user}/external-ids` 的 POST/GET/DELETE schema
- 扩展 user response schema 增加 `external_ids`

- [ ] **Step 4: 提交**

```bash
git add ROADMAP.md README.md docs/openapi/taskg-v1.yaml
git commit -m "docs(m11): 更新 ROADMAP、README 和 OpenAPI 文档"
```
