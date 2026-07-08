# Web Console 项目自动化 OpenAI 兼容投递 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在项目详情页新增「自动化」tab，支持项目级定时触发和事件触发，并把项目上下文按 OpenAI-compatible `chat/completions` 请求投递给外部 Agent Provider。

**Architecture:** 新增独立的 project automation rule / delivery 存储与 app service，避免把项目级 Agent 编排塞进面向 recipient 的 notification rule。后端负责规则 CRUD、预览渲染、定时/事件入队、投递执行和记录；前端在项目页新增 tab，提供规则表单、JSON 预览弹窗、立即测试和运行记录。OpenAI-compatible 请求保持最小兼容 body，追踪信息放入 `<context>` 里的 `_xuanchu` 节点。

**Tech Stack:** Go 1.25 / GORM / SQLite `github.com/glebarez/sqlite` / PostgreSQL `gorm.io/driver/postgres` / Cobra + chi/huma HTTP API / React + TypeScript + TanStack Router + React Query + shadcn/ui + lucide-react / vitest + testing-library

**Spec:** `docs/superpowers/specs/2026-07-08-web-console-project-automation-openai-compatible-design.md`

## Global Constraints

- 主要语言：文档、注释和提交信息使用中文。
- SQLite 必须继续使用 `github.com/glebarez/sqlite`，不能引入 `gorm.io/driver/sqlite` 或 `github.com/mattn/go-sqlite3`。
- 每次实现后必须满足 `go test ./...`、`CGO_ENABLED=0 go test ./...`、`CGO_ENABLED=0 go build ./cmd/xuanchu`。
- Web 改动必须满足 `pnpm --dir web test`、`pnpm --dir web typecheck`、`pnpm --dir web lint`、`pnpm --dir web build`。
- 用户身份 JSON 输出必须使用 `task.UserInfo` / `task.UserInfoToJSON()`，不能退化为裸 UUID。
- 璇础不直接调用飞书，不判断 Agent 是否真的发群消息、拉人或完成外部系统操作。
- 自动化首版只支持 project-scoped 规则。
- 自动化首版触发器只支持 `schedule.daily_at` 和 `event`。
- 自动化首版动作只支持 `openai_compatible`。
- 默认 OpenAI-compatible 接口为 `POST {base_url}/v1/chat/completions`。
- 预览和 delivery 详情不能回显 API key 或 secret config 明文。
- Agent Provider URL 必须命中 `agent.provider.allowed_hosts` 或等价 allowlist；preview、立即测试、正式投递和 replay 使用同一校验。
- delivery 入队时冻结 `resolved_url` 和 `request_body_json`；dispatcher 发送时只重新读取当前 secret config 生成 Authorization。
- delivery 状态复用现有投递状态：`queued`、`delivering`、`retry_wait`、`succeeded`、`dead_lettered`。
- 自动化读操作必须同时具备 `project:read` + `hook:read`；写、预览、测试、replay 必须同时具备 `project:write` + `hook:write`。
- Provider 429/5xx 重试必须有最大尝试次数，默认 `max_attempts = 5`，超过后进入 `dead_lettered`。
- closed project 禁止新增、编辑、启用、立即测试和 replay；允许查看规则和投递记录。
- 不提交 `web/dist`、本地构建产物、临时 token、密钥或运行缓存。

---

## 文件结构

**新建后端文件：**
- `internal/storage/project_automation_rule_repo.go` — project automation rule CRUD、list、enabled scan。
- `internal/storage/project_automation_delivery_repo.go` — delivery 入队、查询、claim、状态更新、replay。
- `internal/storage/project_automation_repo_test.go` — SQLite repository 契约测试。
- `internal/app/project_automation.go` — app service 输入/输出、CRUD、enable/disable/delete、权限、closed project 检查。
- `internal/app/project_automation_preview.go` — OpenAI-compatible 请求渲染、context 构造、secret 遮掩、预览。
- `internal/app/project_automation_scheduler.go` — daily_at 扫描入队、event 入队、测试入队。
- `internal/app/project_automation_dispatcher.go` — 投递 claim、HTTP 执行、响应摘要、重试。
- `internal/app/project_automation_test.go` — service + preview + event/schedule 行为测试。
- `internal/httpapi/project_automations.go` — HTTP request/response DTO 与 handler。
- `internal/httpapi/project_automations_test.go` — HTTP 生命周期、预览、权限、closed project 测试。

**修改后端文件：**
- `internal/storage/models.go` — 新增 `ProjectAutomationRule`、`ProjectAutomationDelivery`。
- `internal/storage/migrate_postgres.go` — AutoMigrate 新模型，必要 actor columns 迁移。
- `internal/storage/migrate_sqlite.go` — AutoMigrate 新模型。
- `internal/app/service.go` — 增加 automation repositories，clone 时保持一致。
- `internal/app/config_schema.go` — 增加自动化首版内置 config definitions。
- `internal/app/service_permissions.go` 或现有权限定义文件 — 复用 `hook:read/write` 到 app 权限映射。
- `internal/app/event_notification.go` 或 `internal/app/service.go` 事件提交处 — 在事件产生后调用 automation event enqueue。
- `internal/httpapi/huma_routes.go` — 注册 project automation API。
- `internal/httpapi/app_service.go` — 如已有 scheduler/dispatcher 注入点，在 server service options 中补自动化 dispatcher client。
- `cmd/xuanchu` 相关 serve wiring — 启动自动化调度器和投递 dispatcher。

**新建前端文件：**
- `web/src/routes/workspace/ProjectAutomationsRoute.tsx` — 项目自动化 tab 路由。
- `web/src/features/workspace/project-workbench/automations/project-automations-api.ts` — TS 类型和 API client。
- `web/src/features/workspace/project-workbench/automations/project-automations-page.tsx` — 规则列表、详情区、运行记录。
- `web/src/features/workspace/project-workbench/automations/automation-rule-form.tsx` — 定时/事件规则表单。
- `web/src/features/workspace/project-workbench/automations/automation-preview-dialog.tsx` — 「预览投递 JSON」弹窗。
- `web/src/features/workspace/project-workbench/automations/automation-delivery-list.tsx` — 运行记录表与详情。
- `web/src/features/workspace/project-workbench/automations/project-automations-page.test.tsx` — 页面集成测试。
- `web/src/features/workspace/project-workbench/automations/automation-preview-dialog.test.tsx` — 预览弹窗测试。

**修改前端文件：**
- `web/src/routes/router.tsx` — lazy import + `/workspaces/$workspaceSlug/projects/$projectSlug/automations` route。
- `web/src/features/workspace/project-workbench/project/project-tabs.tsx` — `ProjectTabKey` 新增 `automations`，tab 文案和链接。
- `web/src/features/workspace/project-workbench/project/project-layout.test.tsx` — tab 测试补自动化。
- `web/src/features/workspace/project-workbench/permissions/permissions.ts` — 新增自动化读写权限 helper，复用 hook/project scope。
- `web/src/locales/zh-CN.ts` / `web/src/locales/en-US.ts` — 新增 UI 文案。
- `README.md` — 自动化 tab 和 config key 示例。
- `ROADMAP.md` — milestone 状态同步。

---

## Task 1: 存储模型与 repository

**目标：** 新增独立的规则表和投递表，支持 SQLite/PostgreSQL 迁移、规则 CRUD、project-scoped 查询、enabled scan、delivery 入队与状态流转。

**Files:**
- Modify: `internal/storage/models.go`
- Modify: `internal/storage/migrate_sqlite.go`
- Modify: `internal/storage/migrate_postgres.go`
- Modify: `internal/app/config_schema.go`
- Create: `internal/storage/project_automation_rule_repo.go`
- Create: `internal/storage/project_automation_delivery_repo.go`
- Create: `internal/storage/project_automation_repo_test.go`

**Interfaces:**
- Produces:
  - `type ProjectAutomationRule struct`
  - `type ProjectAutomationDelivery struct`
  - `func NewProjectAutomationRuleRepository(db *gorm.DB) *ProjectAutomationRuleRepository`
  - `func (r *ProjectAutomationRuleRepository) Create(row ProjectAutomationRule) error`
  - `func (r *ProjectAutomationRuleRepository) GetByID(id string) (ProjectAutomationRule, error)`
  - `func (r *ProjectAutomationRuleRepository) List(workspaceID string, projectID *string, includeDisabled bool) ([]ProjectAutomationRule, error)`
  - `func (r *ProjectAutomationRuleRepository) ListEnabled() ([]ProjectAutomationRule, error)`
  - `func (r *ProjectAutomationRuleRepository) Update(row ProjectAutomationRule) error`
  - `func (r *ProjectAutomationRuleRepository) Delete(id string) error`
  - `func NewProjectAutomationDeliveryRepository(db *gorm.DB) *ProjectAutomationDeliveryRepository`
  - `func (r *ProjectAutomationDeliveryRepository) Enqueue(rows []ProjectAutomationDelivery) error`
  - `func (r *ProjectAutomationDeliveryRepository) ExistsByDedupeKey(key string) (bool, error)`
  - `func (r *ProjectAutomationDeliveryRepository) GetByID(id string) (ProjectAutomationDelivery, error)`
  - `func (r *ProjectAutomationDeliveryRepository) List(opts ProjectAutomationDeliveryListOptions) ([]ProjectAutomationDelivery, error)`
  - `func (r *ProjectAutomationDeliveryRepository) ClaimDue(now int64, claimExpiresAt int64, limit int) ([]ProjectAutomationDelivery, error)`
  - `func (r *ProjectAutomationDeliveryRepository) MarkSucceeded(id string, now int64, statusCode int, providerRequestID string, responsePreview string, usageJSON string) error`
  - `func (r *ProjectAutomationDeliveryRepository) MarkRetry(id string, now int64, nextAttemptAt int64, statusCode *int, message string, responsePreview string) error`
  - `func (r *ProjectAutomationDeliveryRepository) MarkFailed(id string, now int64, statusCode *int, message string, responsePreview string) error`
  - `func (r *ProjectAutomationDeliveryRepository) Requeue(id string, now int64) error`

- Consumed by later tasks:
  - app service uses repositories for CRUD, preview, enqueue and dispatcher.
  - HTTP handlers use app service views, not repositories directly.

- [ ] **Step 1: 写失败测试 — rule repository**

Create `internal/storage/project_automation_repo_test.go` with:

```go
package storage

import (
	"path/filepath"
	"testing"
)

func newProjectAutomationRepoTest(t *testing.T, workspaceSlug string, projectSlug string) (*Store, Workspace, Project) {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ws := createTestWorkspace(t, store, workspaceSlug)
	project, err := NewProjectRepository(store.DB()).Create(testProject("project-"+projectSlug, ws.ID, projectSlug, 100))
	if err != nil {
		t.Fatalf("Create project: %v", err)
	}
	return store, ws, project
}

func TestProjectAutomationRuleRepositoryLifecycle(t *testing.T) {
	store, ws, project := newProjectAutomationRepoTest(t, "auto", "adsops")
	repo := NewProjectAutomationRuleRepository(store.DB())
	enabled := true
	row := ProjectAutomationRule{
		ID:                "rule-1",
		WorkspaceID:       ws.ID,
		ProjectID:         project.ID,
		Name:              "每日项目巡检",
		Description:       "每天检查项目",
		Enabled:           &enabled,
		TriggerType:       "schedule",
		TriggerConfigJSON: `{"schedule_type":"daily_at","schedule_value":"09:30","timezone":"Asia/Shanghai"}`,
		ConditionJSON:     `{"task_filter":"status:pending","max_tasks":50}`,
		ActionType:        "openai_compatible",
		ActionConfigJSON:  `{"protocol":"chat_completions","base_url_config_key":"agent.provider.base_url","api_key_config_key":"agent.provider.api_key","model_config_key":"agent.provider.model","temperature":0.2}`,
		ContextConfigJSON: `{"include":["workspace","project","task_summary","matched_tasks","project_config"]}`,
		InstructionTemplate: "生成项目巡检报告",
		CreatedByActorType: "user",
		CreatedByUserID:    stringPtr("user-1"),
		CreatedAt:          100,
		ModifiedAt:         100,
	}
	if err := repo.Create(row); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.GetByID(row.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != row.Name || got.ProjectID != project.ID || got.ActionType != "openai_compatible" {
		t.Fatalf("rule mismatch: %#v", got)
	}
	list, err := repo.List(ws.ID, &project.ID, false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].ID != row.ID {
		t.Fatalf("List = %#v", list)
	}
	disabled := false
	got.Enabled = &disabled
	got.ModifiedAt = 200
	if err := repo.Update(got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	list, err = repo.List(ws.ID, &project.ID, false)
	if err != nil {
		t.Fatalf("List disabled: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("disabled rule should be hidden, got %#v", list)
	}
	list, err = repo.List(ws.ID, &project.ID, true)
	if err != nil {
		t.Fatalf("List include disabled: %v", err)
	}
	if len(list) != 1 || list[0].ModifiedAt != 200 {
		t.Fatalf("include disabled list = %#v", list)
	}
}
```

- [ ] **Step 2: 写失败测试 — delivery repository**

Append to `internal/storage/project_automation_repo_test.go`:

```go
func TestProjectAutomationDeliveryRepositoryQueueAndState(t *testing.T) {
	store, ws, project := newProjectAutomationRepoTest(t, "auto-delivery", "adsops")
	repo := NewProjectAutomationDeliveryRepository(store.DB())
	row := ProjectAutomationDelivery{
		ID:                   "delivery-1",
		WorkspaceID:          ws.ID,
		ProjectID:            project.ID,
		RuleID:               "rule-1",
		TriggerType:          "schedule",
		DedupeKey:            ws.ID + ":" + project.ID + ":rule-1:2026-07-08:09:30",
		Status:               DeliveryStatusQueued,
		ResolvedURL:          "https://agent.example.com/v1/chat/completions",
		RenderedMethod:       "POST",
		RenderedHeadersJSON:  `{"Content-Type":["application/json"],"Authorization":["Bearer ****"]}`,
		RequestBodyJSON:      `{"model":"project-operator"}`,
		RequestBodyPreview:   `{"model":"project-operator"}`,
		RequestBodyHash:      "sha256:abc",
		ResponseBodyPreview:  "",
		CreatedAt:            100,
		ModifiedAt:           100,
	}
	if err := repo.Enqueue([]ProjectAutomationDelivery{row}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if err := repo.Enqueue([]ProjectAutomationDelivery{row}); err != nil {
		t.Fatalf("Enqueue duplicate: %v", err)
	}
	list, err := repo.List(ProjectAutomationDeliveryListOptions{WorkspaceID: ws.ID, ProjectID: &project.ID, Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("dedupe failed, got %d", len(list))
	}
	claimed, err := repo.ClaimDue(120, 240, 10)
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	if len(claimed) != 1 || claimed[0].AttemptCount != 1 || claimed[0].Status != DeliveryStatusDelivering {
		t.Fatalf("claimed = %#v", claimed)
	}
	if err := repo.MarkSucceeded(row.ID, 130, 200, "run_123", `{"id":"run_123"}`, `{"prompt_tokens":10}`); err != nil {
		t.Fatalf("MarkSucceeded: %v", err)
	}
	got, err := repo.GetByID(row.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Status != DeliveryStatusSucceeded || got.ProviderRequestID != "run_123" || got.ResponseStatusCode == nil || *got.ResponseStatusCode != 200 {
		t.Fatalf("succeeded row = %#v", got)
	}
}
```

- [ ] **Step 3: 运行测试确认失败**

Run: `go test ./internal/storage -run 'TestProjectAutomation' -v`

Expected: FAIL，`ProjectAutomationRule`、`ProjectAutomationDelivery`、repository 类型未定义。

- [ ] **Step 4: 新增 storage models**

Modify `internal/storage/models.go` after `EventNotificationRule`:

```go
type ProjectAutomationRule struct {
	ID                  string  `gorm:"primaryKey"`
	WorkspaceID         string  `gorm:"not null;index:idx_project_automation_rules_scope,priority:1;uniqueIndex:idx_project_automation_rules_ws_project_name,priority:1"`
	ProjectID           string  `gorm:"not null;index:idx_project_automation_rules_scope,priority:2;uniqueIndex:idx_project_automation_rules_ws_project_name,priority:2"`
	Name                string  `gorm:"not null;uniqueIndex:idx_project_automation_rules_ws_project_name,priority:3"`
	Description         string  `gorm:"not null;default:''"`
	Enabled             *bool   `gorm:"not null;default:true;index"`
	TriggerType         string  `gorm:"not null;index"`
	TriggerConfigJSON   string  `gorm:"not null;default:'{}'"`
	ConditionJSON       string  `gorm:"not null;default:'{}'"`
	ActionType          string  `gorm:"not null;default:'openai_compatible';index"`
	ActionConfigJSON    string  `gorm:"not null;default:'{}'"`
	ContextConfigJSON   string  `gorm:"not null;default:'{}'"`
	InstructionTemplate string  `gorm:"not null;default:''"`
	CreatedByActorType  string  `gorm:"not null;default:'user';index"`
	CreatedByUserID     *string `gorm:"index"`
	CreatedByTokenID    *string `gorm:"index"`
	CreatedByTokenName  *string
	CreatedByTokenPrefix *string
	CreatedAt           int64 `gorm:"not null"`
	ModifiedAt          int64 `gorm:"not null"`
}

type ProjectAutomationDelivery struct {
	ID                   string  `gorm:"primaryKey"`
	WorkspaceID          string  `gorm:"not null;index:idx_project_automation_deliveries_scope,priority:1"`
	ProjectID            string  `gorm:"not null;index:idx_project_automation_deliveries_scope,priority:2"`
	RuleID               string  `gorm:"not null;index"`
	TriggerType          string  `gorm:"not null;index"`
	EventID              string  `gorm:"not null;default:'';index"`
	EventType            string  `gorm:"not null;default:'';index"`
	DedupeKey            string  `gorm:"not null;uniqueIndex"`
	Status               string  `gorm:"not null;index:idx_project_automation_deliveries_due,priority:1"`
	ResolvedURL          string  `gorm:"not null;default:''"`
	RenderedMethod       string  `gorm:"not null;default:'POST'"`
	RenderedHeadersJSON  string  `gorm:"not null;default:'{}'"`
	RequestBodyJSON      string  `gorm:"not null;default:''"`
	RequestBodyPreview   string  `gorm:"not null;default:''"`
	RequestBodyHash      string  `gorm:"not null;default:''"`
	ResponseStatusCode   *int
	ResponseBodyPreview  string  `gorm:"not null;default:''"`
	ProviderRequestID    string  `gorm:"not null;default:''"`
	UsageJSON            string  `gorm:"not null;default:'{}'"`
	AttemptCount         int     `gorm:"not null;default:0"`
	NextAttemptAt        *int64  `gorm:"index:idx_project_automation_deliveries_due,priority:2"`
	ClaimExpiresAt       *int64  `gorm:"index"`
	LastAttemptAt        *int64
	LastError            string  `gorm:"not null;default:''"`
	CreatedAt            int64   `gorm:"not null;index"`
	ModifiedAt           int64   `gorm:"not null"`
}
```

Keep field names exactly; later HTTP and frontend contracts depend on them.

- [ ] **Step 5: Wire migrations**

Modify both migration files:

```go
// internal/storage/migrate_sqlite.go and internal/storage/migrate_postgres.go
&NotificationSink{}, &ReminderRule{}, &EventNotificationRule{}, &NotificationDelivery{},
&ProjectAutomationRule{}, &ProjectAutomationDelivery{},
```

For PostgreSQL, append idempotent actor-column SQL in `prepareActorColumnsForP2Postgres()`:

```go
"ALTER TABLE project_automation_rules ADD COLUMN IF NOT EXISTS created_by_actor_type text NOT NULL DEFAULT 'user'",
"ALTER TABLE project_automation_rules ADD COLUMN IF NOT EXISTS created_by_user_id text",
"ALTER TABLE project_automation_rules ADD COLUMN IF NOT EXISTS created_by_token_id text",
"ALTER TABLE project_automation_rules ADD COLUMN IF NOT EXISTS created_by_token_name text",
"ALTER TABLE project_automation_rules ADD COLUMN IF NOT EXISTS created_by_token_prefix text",
```

- [ ] **Step 6: Implement repositories**

Modify `internal/app/config_schema.go` and append these built-in config definitions to `builtinScopedConfigSchemas`:

```go
{Key: "agent.provider.base_url", ValueType: string(ConfigValueTypeString), AllowedScopes: []string{string(ConfigAllowedScopeWorkspace), string(ConfigAllowedScopeProject)}},
{Key: "agent.provider.api_key", ValueType: string(ConfigValueTypeString), AllowedScopes: []string{string(ConfigAllowedScopeWorkspace), string(ConfigAllowedScopeProject)}, Secret: true},
{Key: "agent.provider.model", ValueType: string(ConfigValueTypeString), AllowedScopes: []string{string(ConfigAllowedScopeWorkspace), string(ConfigAllowedScopeProject)}},
{Key: "agent.provider.protocol", ValueType: string(ConfigValueTypeString), AllowedScopes: []string{string(ConfigAllowedScopeWorkspace), string(ConfigAllowedScopeProject)}, DefaultValue: stringPtr("chat_completions")},
{Key: "agent.provider.allowed_hosts", ValueType: string(ConfigValueTypeJSON), AllowedScopes: []string{string(ConfigAllowedScopeWorkspace), string(ConfigAllowedScopeProject)}, DefaultValue: stringPtr("[]")},
{Key: "feishu.chat_id", ValueType: string(ConfigValueTypeString), AllowedScopes: []string{string(ConfigAllowedScopeProject)}},
```

Add a focused test in `internal/app/config_schema_test.go` or the existing config schema test file:

```go
func TestBuiltinConfigDefinitionsIncludeProjectAutomationProvider(t *testing.T) {
	svc, cleanup := newTestService(t, 100)
	defer cleanup()
	rows, err := svc.ConfigSchemaList()
	if err != nil {
		t.Fatalf("ConfigSchemaList: %v", err)
	}
	keys := map[string]bool{}
	for _, row := range rows {
		keys[row.Key] = true
	}
	for _, key := range []string{"agent.provider.base_url", "agent.provider.api_key", "agent.provider.model", "agent.provider.protocol", "agent.provider.allowed_hosts", "feishu.chat_id"} {
		if !keys[key] {
			t.Fatalf("missing builtin config key %s", key)
		}
	}
}
```

- [ ] **Step 6: Implement repositories**

Create `internal/storage/project_automation_rule_repo.go`:

```go
package storage

import (
	"errors"

	"gorm.io/gorm"
)

type ProjectAutomationRuleRepository struct{ db *gorm.DB }

func NewProjectAutomationRuleRepository(db *gorm.DB) *ProjectAutomationRuleRepository {
	return &ProjectAutomationRuleRepository{db: db}
}

func (r *ProjectAutomationRuleRepository) Create(row ProjectAutomationRule) error {
	return r.db.Create(&row).Error
}

func (r *ProjectAutomationRuleRepository) GetByID(id string) (ProjectAutomationRule, error) {
	var row ProjectAutomationRule
	err := r.db.Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ProjectAutomationRule{}, ErrNotFound
	}
	return row, err
}

func (r *ProjectAutomationRuleRepository) List(workspaceID string, projectID *string, includeDisabled bool) ([]ProjectAutomationRule, error) {
	query := r.db.Where("workspace_id = ?", workspaceID)
	if projectID != nil {
		query = query.Where("project_id = ?", *projectID)
	}
	if !includeDisabled {
		query = query.Where("enabled = ?", true)
	}
	var rows []ProjectAutomationRule
	err := query.Order("created_at ASC").Find(&rows).Error
	return rows, err
}

func (r *ProjectAutomationRuleRepository) ListEnabled() ([]ProjectAutomationRule, error) {
	var rows []ProjectAutomationRule
	err := r.db.Where("enabled = ?", true).Order("created_at ASC").Find(&rows).Error
	return rows, err
}

func (r *ProjectAutomationRuleRepository) Update(row ProjectAutomationRule) error {
	return r.db.Save(&row).Error
}

func (r *ProjectAutomationRuleRepository) Delete(id string) error {
	return r.db.Where("id = ?", id).Delete(&ProjectAutomationRule{}).Error
}
```

Create `internal/storage/project_automation_delivery_repo.go`:

```go
package storage

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ProjectAutomationStatusQueued     = DeliveryStatusQueued
	ProjectAutomationStatusDelivering = DeliveryStatusDelivering
	ProjectAutomationStatusRetryWait  = DeliveryStatusRetryWait
	ProjectAutomationStatusSucceeded  = DeliveryStatusSucceeded
	ProjectAutomationStatusFailed     = DeliveryStatusDeadLettered
)

type ProjectAutomationDeliveryRepository struct{ db *gorm.DB }

type ProjectAutomationDeliveryListOptions struct {
	WorkspaceID string
	ProjectID   *string
	RuleID      string
	Status      string
	Limit       int
	Offset      int
}

func NewProjectAutomationDeliveryRepository(db *gorm.DB) *ProjectAutomationDeliveryRepository {
	return &ProjectAutomationDeliveryRepository{db: db}
}

func (r *ProjectAutomationDeliveryRepository) Enqueue(rows []ProjectAutomationDelivery) error {
	if len(rows) == 0 {
		return nil
	}
	return r.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "dedupe_key"}}, DoNothing: true}).Create(&rows).Error
}

func (r *ProjectAutomationDeliveryRepository) ExistsByDedupeKey(key string) (bool, error) {
	if key == "" {
		return false, nil
	}
	var count int64
	err := r.db.Model(&ProjectAutomationDelivery{}).Where("dedupe_key = ?", key).Limit(1).Count(&count).Error
	return count > 0, err
}

func (r *ProjectAutomationDeliveryRepository) GetByID(id string) (ProjectAutomationDelivery, error) {
	var row ProjectAutomationDelivery
	err := r.db.Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ProjectAutomationDelivery{}, ErrNotFound
	}
	return row, err
}

func (r *ProjectAutomationDeliveryRepository) List(opts ProjectAutomationDeliveryListOptions) ([]ProjectAutomationDelivery, error) {
	query := r.db.Where("workspace_id = ?", opts.WorkspaceID)
	if opts.ProjectID != nil {
		query = query.Where("project_id = ?", *opts.ProjectID)
	}
	if opts.RuleID != "" {
		query = query.Where("rule_id = ?", opts.RuleID)
	}
	if opts.Status != "" {
		query = query.Where("status = ?", opts.Status)
	}
	if opts.Offset > 0 {
		query = query.Offset(opts.Offset)
	}
	if opts.Limit > 0 {
		query = query.Limit(opts.Limit)
	}
	var rows []ProjectAutomationDelivery
	err := query.Order("created_at DESC").Find(&rows).Error
	return rows, err
}

func (r *ProjectAutomationDeliveryRepository) ClaimDue(now int64, claimExpiresAt int64, limit int) ([]ProjectAutomationDelivery, error) {
	if limit <= 0 {
		return nil, nil
	}
	var rows []ProjectAutomationDelivery
	err := r.db.Raw(`
UPDATE project_automation_deliveries
SET status = ?, claim_expires_at = ?, attempt_count = attempt_count + 1, modified_at = ?
WHERE id IN (
	SELECT id
	FROM project_automation_deliveries
	WHERE status IN (?, ?)
	  AND (next_attempt_at IS NULL OR next_attempt_at <= ?)
	ORDER BY created_at ASC
	LIMIT ?
)
RETURNING *`,
		ProjectAutomationStatusDelivering, claimExpiresAt, now,
		ProjectAutomationStatusQueued, ProjectAutomationStatusRetryWait, now, limit,
	).Scan(&rows).Error
	return rows, err
}

func (r *ProjectAutomationDeliveryRepository) MarkSucceeded(id string, now int64, statusCode int, providerRequestID string, responsePreview string, usageJSON string) error {
	return r.db.Model(&ProjectAutomationDelivery{}).Where("id = ?", id).Updates(map[string]any{
		"status":               ProjectAutomationStatusSucceeded,
		"response_status_code": statusCode,
		"response_body_preview": responsePreview,
		"provider_request_id":  providerRequestID,
		"usage_json":           usageJSON,
		"last_attempt_at":      now,
		"last_error":           "",
		"claim_expires_at":     nil,
		"modified_at":          now,
	}).Error
}

func (r *ProjectAutomationDeliveryRepository) MarkRetry(id string, now int64, nextAttemptAt int64, statusCode *int, message string, responsePreview string) error {
	updates := map[string]any{
		"status":                ProjectAutomationStatusRetryWait,
		"next_attempt_at":       nextAttemptAt,
		"response_body_preview": responsePreview,
		"last_attempt_at":       now,
		"last_error":            message,
		"claim_expires_at":      nil,
		"modified_at":           now,
	}
	if statusCode != nil {
		updates["response_status_code"] = *statusCode
	}
	return r.db.Model(&ProjectAutomationDelivery{}).Where("id = ?", id).Updates(updates).Error
}

func (r *ProjectAutomationDeliveryRepository) MarkFailed(id string, now int64, statusCode *int, message string, responsePreview string) error {
	updates := map[string]any{
		"status":                ProjectAutomationStatusFailed,
		"response_body_preview": responsePreview,
		"last_attempt_at":       now,
		"last_error":            message,
		"claim_expires_at":      nil,
		"modified_at":           now,
	}
	if statusCode != nil {
		updates["response_status_code"] = *statusCode
	}
	return r.db.Model(&ProjectAutomationDelivery{}).Where("id = ?", id).Updates(updates).Error
}

func (r *ProjectAutomationDeliveryRepository) Requeue(id string, now int64) error {
	return r.db.Model(&ProjectAutomationDelivery{}).Where("id = ?", id).Updates(map[string]any{
		"status":           ProjectAutomationStatusQueued,
		"next_attempt_at":  nil,
		"claim_expires_at": nil,
		"modified_at":      now,
	}).Error
}
```

- [ ] **Step 7: 运行 storage 测试**

Run: `go test ./internal/storage -run 'TestProjectAutomation' -v`

Expected: PASS。

- [ ] **Step 8: Commit**

```bash
git add internal/storage/models.go internal/storage/migrate_sqlite.go internal/storage/migrate_postgres.go internal/app/config_schema.go internal/storage/project_automation_rule_repo.go internal/storage/project_automation_delivery_repo.go internal/storage/project_automation_repo_test.go
git commit -m "feat: 增加项目自动化存储模型"
```

---

## Task 2: App service 规则 CRUD 与 OpenAI 请求预览

**目标：** 在 app 层实现规则输入验证、权限、closed project 限制、view 序列化、OpenAI-compatible body 渲染、project_config 脱敏和未保存规则预览。

**Files:**
- Modify: `internal/app/service.go`
- Create: `internal/app/project_automation.go`
- Create: `internal/app/project_automation_preview.go`
- Create: `internal/app/project_automation_test.go`

**Interfaces:**
- Consumes:
  - Task 1 repositories and models.
  - Existing `ResolveProject`, config repositories, task repo and `resolveUserInfos`.
- Produces:
  - `type ProjectAutomationRuleAddInput`
  - `type ProjectAutomationRuleModifyInput`
  - `type ProjectAutomationRuleView`
  - `type ProjectAutomationPreviewInput`
  - `type ProjectAutomationPreviewView`
  - `func (s *Service) AddProjectAutomationRule(projectRef string, input ProjectAutomationRuleAddInput) (ProjectAutomationRuleView, error)`
  - `func (s *Service) ProjectAutomationRuleInfo(projectRef string, ruleID string) (ProjectAutomationRuleView, error)`
  - `func (s *Service) ListProjectAutomationRules(projectRef string, includeDisabled bool) ([]ProjectAutomationRuleView, error)`
  - `func (s *Service) ModifyProjectAutomationRule(projectRef string, ruleID string, input ProjectAutomationRuleModifyInput) (ProjectAutomationRuleView, error)`
  - `func (s *Service) EnableProjectAutomationRule(projectRef string, ruleID string) (ProjectAutomationRuleView, error)`
  - `func (s *Service) DisableProjectAutomationRule(projectRef string, ruleID string) (ProjectAutomationRuleView, error)`
  - `func (s *Service) DeleteProjectAutomationRule(projectRef string, ruleID string) error`
  - `func (s *Service) PreviewProjectAutomation(projectRef string, input ProjectAutomationPreviewInput) (ProjectAutomationPreviewView, error)`
  - `func (s *Service) PreviewSavedProjectAutomation(projectRef string, ruleID string, override *ProjectAutomationPreviewInput) (ProjectAutomationPreviewView, error)`
  - `func (s *Service) renderProjectAutomationRequest(project ProjectView, ruleID string, input ProjectAutomationRuleAddInput, triggerType string, deliveryID string, event *HookEvent) (ProjectAutomationRenderedRequest, error)`

- [ ] **Step 1: 写失败测试 — CRUD 和 closed project**

Create `internal/app/project_automation_test.go`:

```go
package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

type projectAutomationServiceFixture struct {
	store          *storage.Store
	svc            *Service
	clock          FixedClock
	serviceFactory func(workspaceID string) *Service
}

func newServiceFixture(t *testing.T) *projectAutomationServiceFixture {
	t.Helper()
	store := newTestStore(t)
	clock := FixedClock{NowUnix: 1000}
	svc, err := NewService(ServiceOptions{Store: store, Clock: clock})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	f := &projectAutomationServiceFixture{store: store, svc: svc, clock: clock}
	f.serviceFactory = func(workspaceID string) *Service {
		next, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: f.clock.NowUnix}, WorkspaceRef: workspaceID})
		if err != nil {
			t.Fatalf("NewService(%s): %v", workspaceID, err)
		}
		return next
	}
	return f
}

func defineConfigForTest(t *testing.T, svc *Service, key string, secret bool, value string) {
	t.Helper()
	valueType := string(ConfigValueTypeString)
	if key == "agent.provider.allowed_hosts" {
		valueType = string(ConfigValueTypeJSON)
	}
	if err := svc.ConfigSchemaSet(ConfigSchemaInput{Key: key, ValueType: valueType, AllowedScopes: []string{string(ConfigAllowedScopeWorkspace), string(ConfigAllowedScopeProject)}, Secret: secret}); err != nil {
		t.Fatalf("ConfigSchemaSet(%s): %v", key, err)
	}
	if err := svc.ProjectConfigSet("adsops", key, value); err != nil {
		t.Fatalf("ProjectConfigSet(%s): %v", key, err)
	}
}

func defineProviderConfigForTest(t *testing.T, svc *Service) {
	t.Helper()
	defineProviderConfigForTestWithBaseURL(t, svc, "https://agent.example.com")
}

func defineProviderConfigForTestWithBaseURL(t *testing.T, svc *Service, baseURL string) {
	t.Helper()
	defineConfigForTest(t, svc, "agent.provider.base_url", false, baseURL)
	defineConfigForTest(t, svc, "agent.provider.api_key", true, "sk-test")
	defineConfigForTest(t, svc, "agent.provider.model", false, "project-operator")
	host := strings.TrimPrefix(strings.TrimPrefix(baseURL, "https://"), "http://")
	host = strings.Split(host, "/")[0]
	defineConfigForTest(t, svc, "agent.provider.allowed_hosts", false, `["`+host+`"]`)
}

func defaultAutomationActionForTest() ProjectAutomationActionConfig {
	return ProjectAutomationActionConfig{Protocol: "chat_completions", BaseURLConfigKey: "agent.provider.base_url", APIKeyConfigKey: "agent.provider.api_key", ModelConfigKey: "agent.provider.model", Temperature: 0.2}
}

func createWorkspaceMemberForTest(t *testing.T, svc *Service, name string) storage.User {
	t.Helper()
	user := mustCreateUserRecord(t, svc.store, storage.User{ID: uuid.NewString(), Name: name, CreatedAt: 100, ModifiedAt: 100})
	mustUpsertMembershipRecord(t, svc.store, storage.Membership{UserID: user.ID, WorkspaceID: svc.Runtime().WorkspaceID, Role: string(RoleMember), JoinedAt: 100, ModifiedAt: 100})
	return user
}

func mustUnix(t *testing.T, value string) int64 {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse time: %v", err)
	}
	return ts.Unix()
}

func mustJSONBytes(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	return raw
}

func contains(value string, substr string) bool {
	return strings.Contains(value, substr)
}

func runtimeErrorCode(err error) string {
	var rt RuntimeError
	if errors.As(err, &rt) {
		return rt.Code
	}
	return ""
}

func TestProjectAutomationRuleLifecycleRejectsClosedProjectWrites(t *testing.T) {
	f := newServiceFixture(t)
	project, err := f.svc.AddProject(AddProjectInput{Slug: "adsops", Name: "广告投放优化"})
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	input := ProjectAutomationRuleAddInput{
		Name:                "每日项目巡检",
		Enabled:             true,
		TriggerType:         "schedule",
		TriggerConfig:       ProjectAutomationTriggerConfig{ScheduleType: "daily_at", ScheduleValue: "09:30", Timezone: "Asia/Shanghai"},
		Condition:           ProjectAutomationCondition{TaskFilter: "status:pending", MaxTasks: 50},
		Action:              ProjectAutomationActionConfig{Protocol: "chat_completions", BaseURLConfigKey: "agent.provider.base_url", APIKeyConfigKey: "agent.provider.api_key", ModelConfigKey: "agent.provider.model", Temperature: 0.2},
		Context:             ProjectAutomationContextConfig{Include: []string{"workspace", "project", "task_summary", "matched_tasks", "project_config"}},
		InstructionTemplate: "生成项目巡检报告",
	}
	created, err := f.svc.AddProjectAutomationRule(project.Slug, input)
	if err != nil {
		t.Fatalf("AddProjectAutomationRule: %v", err)
	}
	if created.ID == "" || created.ProjectID != project.ID || created.TriggerType != "schedule" {
		t.Fatalf("created = %#v", created)
	}
	rows, err := f.svc.ListProjectAutomationRules(project.Slug, false)
	if err != nil {
		t.Fatalf("ListProjectAutomationRules: %v", err)
	}
	if len(rows) != 1 || rows[0].Name != "每日项目巡检" {
		t.Fatalf("rows = %#v", rows)
	}
	if _, err := f.svc.TransitionProject(project.Slug, string(storage.ProjectStatusArchived)); err != nil {
		t.Fatalf("archive project: %v", err)
	}
	_, err = f.svc.AddProjectAutomationRule(project.Slug, input)
	if err == nil || runtimeErrorCode(err) != "project_closed" {
		t.Fatalf("closed project add err = %v", err)
	}
	_, err = f.svc.ModifyProjectAutomationRule(project.Slug, created.ID, ProjectAutomationRuleModifyInput{Name: stringPtr("改名")})
	if err == nil || runtimeErrorCode(err) != "project_closed" {
		t.Fatalf("closed project modify err = %v", err)
	}
}
```

- [ ] **Step 2: 写失败测试 — preview 不泄露 secret**

Append:

```go
func TestProjectAutomationPreviewMasksSecretAndBuildsContext(t *testing.T) {
	f := newServiceFixture(t)
	project, err := f.svc.AddProject(AddProjectInput{Slug: "adsops", Name: "广告投放优化"})
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	defineConfigForTest(t, f.svc, "agent.provider.base_url", false, "https://agent.example.com")
	defineConfigForTest(t, f.svc, "agent.provider.api_key", true, "sk-real-secret")
	defineConfigForTest(t, f.svc, "agent.provider.model", false, "project-operator")
	defineConfigForTest(t, f.svc, "agent.provider.allowed_hosts", false, `["agent.example.com"]`)
	defineConfigForTest(t, f.svc, "feishu.chat_id", false, "oc_xxx")

	view, err := f.svc.PreviewProjectAutomation(project.Slug, ProjectAutomationPreviewInput{
		Name:          "每日项目巡检",
		TriggerType:   "schedule",
		TriggerConfig: ProjectAutomationTriggerConfig{ScheduleType: "daily_at", ScheduleValue: "09:30", Timezone: "Asia/Shanghai"},
		Action:        ProjectAutomationActionConfig{Protocol: "chat_completions", BaseURLConfigKey: "agent.provider.base_url", APIKeyConfigKey: "agent.provider.api_key", ModelConfigKey: "agent.provider.model", Temperature: 0.2},
		Context:       ProjectAutomationContextConfig{Include: []string{"workspace", "project", "project_config"}},
		InstructionTemplate: "请生成项目巡检报告",
	})
	if err != nil {
		t.Fatalf("PreviewProjectAutomation: %v", err)
	}
	if view.URL != "https://agent.example.com/v1/chat/completions" {
		t.Fatalf("url = %q", view.URL)
	}
	if view.Headers["Authorization"] != "Bearer ****" {
		t.Fatalf("Authorization = %q", view.Headers["Authorization"])
	}
	body := string(mustJSONBytes(t, view.Body))
	if contains(body, "sk-real-secret") {
		t.Fatalf("preview leaked secret: %s", body)
	}
	if !contains(body, `"model":"project-operator"`) || !contains(body, `"feishu.chat_id":"oc_xxx"`) {
		t.Fatalf("preview body missing expected fields: %s", body)
	}
}
```

- [ ] **Step 3: 运行测试确认失败**

Run: `go test ./internal/app -run 'TestProjectAutomation' -v`

Expected: FAIL，automation service types and methods are undefined.

- [ ] **Step 4: Wire repositories into Service**

Modify `internal/app/service.go`:

```go
type Service struct {
	// 保留当前所有已有字段，并在结构体末尾新增这两个 repository。
	projectAutomationRuleRepo     *storage.ProjectAutomationRuleRepository
	projectAutomationDeliveryRepo *storage.ProjectAutomationDeliveryRepository
}

// in NewService:
projectAutomationRuleRepo:     storage.NewProjectAutomationRuleRepository(opts.Store.DB()),
projectAutomationDeliveryRepo: storage.NewProjectAutomationDeliveryRepository(opts.Store.DB()),

// in WithStore/clone path:
clone.projectAutomationRuleRepo = storage.NewProjectAutomationRuleRepository(store.DB())
clone.projectAutomationDeliveryRepo = storage.NewProjectAutomationDeliveryRepository(store.DB())
```

- [ ] **Step 5: Implement app input/view types and CRUD**

Create `internal/app/project_automation.go`:

```go
package app

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

const (
	ProjectAutomationTriggerSchedule = "schedule"
	ProjectAutomationTriggerEvent    = "event"
	ProjectAutomationActionOpenAI    = "openai_compatible"
)

type ProjectAutomationTriggerConfig struct {
	ScheduleType  string `json:"schedule_type,omitempty"`
	ScheduleValue string `json:"schedule_value,omitempty"`
	Timezone      string `json:"timezone,omitempty"`
	EventType     string `json:"event_type,omitempty"`
}

type ProjectAutomationCondition struct {
	TaskFilter          string `json:"task_filter,omitempty"`
	MaxTasks            int    `json:"max_tasks,omitempty"`
	OnlyAddedAssignees  bool   `json:"only_added_assignees,omitempty"`
}

type ProjectAutomationActionConfig struct {
	Protocol         string  `json:"protocol"`
	BaseURLConfigKey string  `json:"base_url_config_key"`
	APIKeyConfigKey  string  `json:"api_key_config_key"`
	ModelConfigKey   string  `json:"model_config_key"`
	AllowedHostsConfigKey string `json:"allowed_hosts_config_key,omitempty"`
	ModelOverride    string  `json:"model_override,omitempty"`
	Temperature      float64 `json:"temperature"`
	MaxAttempts      int     `json:"max_attempts,omitempty"`
	AttachMetadata   bool    `json:"attach_metadata,omitempty"`
}

type ProjectAutomationContextConfig struct {
	Include []string `json:"include"`
}

type ProjectAutomationRuleAddInput struct {
	Name                string
	Description         string
	Enabled             bool
	TriggerType         string
	TriggerConfig       ProjectAutomationTriggerConfig
	Condition           ProjectAutomationCondition
	Action              ProjectAutomationActionConfig
	Context             ProjectAutomationContextConfig
	InstructionTemplate string
}

type ProjectAutomationRuleModifyInput struct {
	Name                *string
	Description         *string
	Enabled             *bool
	TriggerType         *string
	TriggerConfig       *ProjectAutomationTriggerConfig
	Condition           *ProjectAutomationCondition
	Action              *ProjectAutomationActionConfig
	Context             *ProjectAutomationContextConfig
	InstructionTemplate *string
}

type ProjectAutomationRuleView struct {
	ID                  string         `json:"id"`
	WorkspaceID         string         `json:"workspace_id"`
	ProjectID           string         `json:"project_id"`
	Name                string         `json:"name"`
	Description         string         `json:"description"`
	Enabled             bool           `json:"enabled"`
	TriggerType         string         `json:"trigger_type"`
	TriggerConfig       ProjectAutomationTriggerConfig `json:"trigger_config"`
	Condition           ProjectAutomationCondition     `json:"condition"`
	ActionType          string         `json:"action_type"`
	Action              ProjectAutomationActionConfig  `json:"action"`
	Context             ProjectAutomationContextConfig `json:"context"`
	InstructionTemplate string         `json:"instruction_template"`
	CreatedBy           task.UserInfo  `json:"created_by"`
	CreatedAt           int64          `json:"created_at"`
	ModifiedAt          int64          `json:"modified_at"`
}

func (s *Service) AddProjectAutomationRule(projectRef string, input ProjectAutomationRuleAddInput) (ProjectAutomationRuleView, error) {
	if err := s.requireProjectAutomationWrite(); err != nil {
		return ProjectAutomationRuleView{}, err
	}
	project, err := s.ResolveProject(projectRef)
	if err != nil {
		return ProjectAutomationRuleView{}, err
	}
	if isClosedProjectStatus(project.Status) {
		return ProjectAutomationRuleView{}, RuntimeError{Code: "project_closed", Message: "project is closed"}
	}
	normalized, err := normalizeProjectAutomationAddInput(input)
	if err != nil {
		return ProjectAutomationRuleView{}, err
	}
	now := s.clock.Unix()
	row := storage.ProjectAutomationRule{
		ID:                  uuid.NewString(),
		WorkspaceID:         s.workspaceID,
		ProjectID:           project.ID,
		Name:                normalized.Name,
		Description:         normalized.Description,
		Enabled:             &normalized.Enabled,
		TriggerType:         normalized.TriggerType,
		TriggerConfigJSON:   mustJSON(normalized.TriggerConfig),
		ConditionJSON:       mustJSON(normalized.Condition),
		ActionType:          ProjectAutomationActionOpenAI,
		ActionConfigJSON:    mustJSON(normalized.Action),
		ContextConfigJSON:   mustJSON(normalized.Context),
		InstructionTemplate: normalized.InstructionTemplate,
		CreatedAt:           now,
		ModifiedAt:          now,
	}
	actor := s.runtime.actorColumns()
	row.CreatedByActorType = actor.Type
	row.CreatedByUserID = actor.UserID
	row.CreatedByTokenID = actor.TokenID
	row.CreatedByTokenName = actor.TokenName
	row.CreatedByTokenPrefix = actor.TokenPrefix
	if err := s.projectAutomationRuleRepo.Create(row); err != nil {
		return ProjectAutomationRuleView{}, err
	}
	created, err := s.projectAutomationRuleRepo.GetByID(row.ID)
	if err != nil {
		return ProjectAutomationRuleView{}, err
	}
	return s.projectAutomationRuleViewFromRow(created)
}
```

Add the following functions in the same file:

```go
func (s *Service) ListProjectAutomationRules(projectRef string, includeDisabled bool) ([]ProjectAutomationRuleView, error) {
	if err := s.requireProjectAutomationRead(); err != nil {
		return nil, err
	}
	project, err := s.ResolveProject(projectRef)
	if err != nil {
		return nil, err
	}
	rows, err := s.projectAutomationRuleRepo.List(s.workspaceID, &project.ID, includeDisabled)
	if err != nil {
		return nil, err
	}
	out := make([]ProjectAutomationRuleView, 0, len(rows))
	for _, row := range rows {
		view, err := s.projectAutomationRuleViewFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, view)
	}
	return out, nil
}

func normalizeProjectAutomationAddInput(input ProjectAutomationRuleAddInput) (ProjectAutomationRuleAddInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.TriggerType = strings.TrimSpace(input.TriggerType)
	input.InstructionTemplate = strings.TrimSpace(input.InstructionTemplate)
	if input.Name == "" {
		return input, RuntimeError{Code: "automation_rule_invalid", Message: "rule name is required"}
	}
	if input.TriggerType != ProjectAutomationTriggerSchedule && input.TriggerType != ProjectAutomationTriggerEvent {
		return input, RuntimeError{Code: "automation_rule_invalid", Message: "unsupported trigger"}
	}
	if input.TriggerType == ProjectAutomationTriggerSchedule {
		if input.TriggerConfig.ScheduleType != "daily_at" || !validHHMM(input.TriggerConfig.ScheduleValue) {
			return input, RuntimeError{Code: "automation_rule_invalid", Message: "invalid daily schedule"}
		}
		if strings.TrimSpace(input.TriggerConfig.Timezone) == "" {
			input.TriggerConfig.Timezone = "Asia/Shanghai"
		}
	}
	if input.TriggerType == ProjectAutomationTriggerEvent {
		if !allowedHookEventTypes[input.TriggerConfig.EventType] {
			return input, RuntimeError{Code: "automation_rule_invalid", Message: "unsupported event"}
		}
	}
	if input.Action.Protocol == "" {
		input.Action.Protocol = "chat_completions"
	}
	if input.Action.Protocol != "chat_completions" {
		return input, RuntimeError{Code: "automation_rule_invalid", Message: "unsupported protocol"}
	}
	if input.Action.BaseURLConfigKey == "" || input.Action.APIKeyConfigKey == "" || (input.Action.ModelConfigKey == "" && input.Action.ModelOverride == "") {
		return input, RuntimeError{Code: "automation_rule_invalid", Message: "provider config is required"}
	}
	if input.Action.Temperature == 0 {
		input.Action.Temperature = 0.2
	}
	if input.Action.MaxAttempts <= 0 {
		input.Action.MaxAttempts = 5
	}
	if input.Action.AllowedHostsConfigKey == "" {
		input.Action.AllowedHostsConfigKey = "agent.provider.allowed_hosts"
	}
	if input.Condition.MaxTasks <= 0 {
		input.Condition.MaxTasks = 50
	}
	if input.InstructionTemplate == "" {
		return input, RuntimeError{Code: "automation_rule_invalid", Message: "instruction template is required"}
	}
	return input, nil
}

func validHHMM(value string) bool {
	if len(value) != 5 || value[2] != ':' {
		return false
	}
	_, err := time.Parse("15:04", value)
	return err == nil
}

func projectAutomationRuleAddInputFromRow(row storage.ProjectAutomationRule) ProjectAutomationRuleAddInput {
	return ProjectAutomationRuleAddInput{
		Name:                row.Name,
		Description:         row.Description,
		Enabled:             row.Enabled == nil || *row.Enabled,
		TriggerType:         row.TriggerType,
		TriggerConfig:       decodeProjectAutomationTriggerConfig(row.TriggerConfigJSON),
		Condition:           decodeProjectAutomationCondition(row.ConditionJSON),
		Action:              decodeProjectAutomationActionConfig(row.ActionConfigJSON),
		Context:             decodeProjectAutomationContextConfig(row.ContextConfigJSON),
		InstructionTemplate: row.InstructionTemplate,
	}
}

func (s *Service) requireProjectAutomationRead() error {
	if err := s.Require(PermissionProjectRead); err != nil {
		return err
	}
	return s.Require(PermissionHookRead)
}

func (s *Service) requireProjectAutomationWrite() error {
	if err := s.Require(PermissionProjectManage); err != nil {
		return err
	}
	return s.Require(PermissionHookWrite)
}
```

In the same file, implement the remaining rule lifecycle methods used by HTTP and UI:

```go
func (s *Service) ProjectAutomationRuleInfo(projectRef string, ruleID string) (ProjectAutomationRuleView, error) {
	if err := s.requireProjectAutomationRead(); err != nil {
		return ProjectAutomationRuleView{}, err
	}
	project, err := s.ResolveProject(projectRef)
	if err != nil {
		return ProjectAutomationRuleView{}, err
	}
	row, err := s.projectAutomationRuleRepo.GetByID(ruleID)
	if err != nil {
		return ProjectAutomationRuleView{}, err
	}
	if row.WorkspaceID != s.workspaceID || row.ProjectID != project.ID {
		return ProjectAutomationRuleView{}, RuntimeError{Code: "automation_rule_not_found", Message: "automation rule not found"}
	}
	return s.projectAutomationRuleViewFromRow(row)
}

func (s *Service) ModifyProjectAutomationRule(projectRef string, ruleID string, input ProjectAutomationRuleModifyInput) (ProjectAutomationRuleView, error) {
	if err := s.requireProjectAutomationWrite(); err != nil {
		return ProjectAutomationRuleView{}, err
	}
	project, err := s.ResolveProject(projectRef)
	if err != nil {
		return ProjectAutomationRuleView{}, err
	}
	if isProjectClosed(project) {
		return ProjectAutomationRuleView{}, RuntimeError{Code: "project_closed", Message: "project is closed"}
	}
	row, err := s.projectAutomationRuleRepo.GetByID(ruleID)
	if err != nil {
		return ProjectAutomationRuleView{}, err
	}
	if row.WorkspaceID != s.workspaceID || row.ProjectID != project.ID {
		return ProjectAutomationRuleView{}, RuntimeError{Code: "automation_rule_not_found", Message: "automation rule not found"}
	}
	next := projectAutomationRuleAddInputFromRow(row)
	if input.Name != nil { next.Name = *input.Name }
	if input.Description != nil { next.Description = *input.Description }
	if input.Enabled != nil { next.Enabled = *input.Enabled }
	if input.TriggerType != nil { next.TriggerType = *input.TriggerType }
	if input.TriggerConfig != nil { next.TriggerConfig = *input.TriggerConfig }
	if input.Condition != nil { next.Condition = *input.Condition }
	if input.Action != nil { next.Action = *input.Action }
	if input.Context != nil { next.Context = *input.Context }
	if input.InstructionTemplate != nil { next.InstructionTemplate = *input.InstructionTemplate }
	normalized, err := normalizeProjectAutomationAddInput(next)
	if err != nil {
		return ProjectAutomationRuleView{}, err
	}
	row.Name = normalized.Name
	row.Description = normalized.Description
	row.Enabled = &normalized.Enabled
	row.TriggerType = normalized.TriggerType
	row.TriggerConfigJSON = mustJSON(normalized.TriggerConfig)
	row.ConditionJSON = mustJSON(normalized.Condition)
	row.ActionConfigJSON = mustJSON(normalized.Action)
	row.ContextConfigJSON = mustJSON(normalized.Context)
	row.InstructionTemplate = normalized.InstructionTemplate
	row.ModifiedAt = s.clock.Unix()
	if err := s.projectAutomationRuleRepo.Update(row); err != nil {
		return ProjectAutomationRuleView{}, err
	}
	return s.projectAutomationRuleViewFromRow(row)
}

func (s *Service) EnableProjectAutomationRule(projectRef string, ruleID string) (ProjectAutomationRuleView, error) {
	enabled := true
	return s.ModifyProjectAutomationRule(projectRef, ruleID, ProjectAutomationRuleModifyInput{Enabled: &enabled})
}

func (s *Service) DisableProjectAutomationRule(projectRef string, ruleID string) (ProjectAutomationRuleView, error) {
	enabled := false
	return s.ModifyProjectAutomationRule(projectRef, ruleID, ProjectAutomationRuleModifyInput{Enabled: &enabled})
}

func (s *Service) DeleteProjectAutomationRule(projectRef string, ruleID string) error {
	if err := s.requireProjectAutomationWrite(); err != nil {
		return err
	}
	project, err := s.ResolveProject(projectRef)
	if err != nil {
		return err
	}
	if isProjectClosed(project) {
		return RuntimeError{Code: "project_closed", Message: "project is closed"}
	}
	row, err := s.projectAutomationRuleRepo.GetByID(ruleID)
	if err != nil {
		return err
	}
	if row.WorkspaceID != s.workspaceID || row.ProjectID != project.ID {
		return RuntimeError{Code: "automation_rule_not_found", Message: "automation rule not found"}
	}
	return s.projectAutomationRuleRepo.Delete(ruleID)
}

func (s *Service) projectAutomationRuleViewFromRow(row storage.ProjectAutomationRule) (ProjectAutomationRuleView, error) {
	users, err := s.resolveUserInfos([]string{valueOrEmpty(row.CreatedByUserID)})
	if err != nil {
		return ProjectAutomationRuleView{}, err
	}
	createdBy := users[valueOrEmpty(row.CreatedByUserID)]
	if createdBy.ID == "" {
		createdBy = task.UserInfo{ID: valueOrEmpty(row.CreatedByUserID), Name: valueOrEmpty(row.CreatedByUserID)}
	}
	input := projectAutomationRuleAddInputFromRow(row)
	return ProjectAutomationRuleView{
		ID: row.ID, WorkspaceID: row.WorkspaceID, ProjectID: row.ProjectID,
		Name: row.Name, Description: row.Description, Enabled: input.Enabled,
		TriggerType: row.TriggerType, TriggerConfig: input.TriggerConfig, Condition: input.Condition,
		ActionType: row.ActionType, Action: input.Action, Context: input.Context,
		InstructionTemplate: row.InstructionTemplate, CreatedBy: createdBy,
		CreatedAt: row.CreatedAt, ModifiedAt: row.ModifiedAt,
	}, nil
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
```

- [ ] **Step 6: Implement preview renderer**

Create `internal/app/project_automation_preview.go`:

```go
package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

type ProjectAutomationPreviewInput = ProjectAutomationRuleAddInput

type ProjectAutomationPreviewView struct {
	Method   string         `json:"method"`
	URL      string         `json:"url"`
	Headers  map[string]string `json:"headers"`
	Body     map[string]any `json:"body"`
	Warnings []string       `json:"warnings"`
}

type ProjectAutomationRenderedRequest struct {
	Method              string
	URL                 string
	Headers             map[string]string
	MaskedHeaders       map[string]string
	BodyJSON            string
	BodyPreview         string
	BodyHash            string
	Model               string
}

func (s *Service) PreviewProjectAutomation(projectRef string, input ProjectAutomationPreviewInput) (ProjectAutomationPreviewView, error) {
	if err := s.requireProjectAutomationWrite(); err != nil {
		return ProjectAutomationPreviewView{}, err
	}
	project, err := s.ResolveProject(projectRef)
	if err != nil {
		return ProjectAutomationPreviewView{}, err
	}
	projectView, err := s.projectViewForRow(project)
	if err != nil {
		return ProjectAutomationPreviewView{}, err
	}
	normalized, err := normalizeProjectAutomationAddInput(ProjectAutomationRuleAddInput(input))
	if err != nil {
		return ProjectAutomationPreviewView{}, err
	}
	rendered, err := s.renderProjectAutomationRequest(projectView, "", normalized, "preview", "", nil)
	if err != nil {
		return ProjectAutomationPreviewView{}, err
	}
	body := map[string]any{}
	if err := json.Unmarshal([]byte(rendered.BodyJSON), &body); err != nil {
		return ProjectAutomationPreviewView{}, err
	}
	return ProjectAutomationPreviewView{
		Method:   rendered.Method,
		URL:      rendered.URL,
		Headers:  rendered.MaskedHeaders,
		Body:     body,
		Warnings: nil,
	}, nil
}

func (s *Service) PreviewSavedProjectAutomation(projectRef string, ruleID string, override *ProjectAutomationPreviewInput) (ProjectAutomationPreviewView, error) {
	if err := s.requireProjectAutomationWrite(); err != nil {
		return ProjectAutomationPreviewView{}, err
	}
	row, err := s.projectAutomationRuleRepo.GetByID(ruleID)
	if err != nil {
		return ProjectAutomationPreviewView{}, err
	}
	project, err := s.ResolveProject(projectRef)
	if err != nil {
		return ProjectAutomationPreviewView{}, err
	}
	if row.WorkspaceID != s.workspaceID || row.ProjectID != project.ID {
		return ProjectAutomationPreviewView{}, RuntimeError{Code: "automation_rule_not_found", Message: "automation rule not found"}
	}
	input := projectAutomationRuleAddInputFromRow(row)
	if override != nil {
		input = ProjectAutomationRuleAddInput(*override)
	}
	return s.PreviewProjectAutomation(projectRef, ProjectAutomationPreviewInput(input))
}

func (s *Service) renderProjectAutomationRequest(project ProjectView, ruleID string, input ProjectAutomationRuleAddInput, triggerType string, deliveryID string, event *HookEvent) (ProjectAutomationRenderedRequest, error) {
	baseURL, apiKey, model, err := s.resolveProjectAutomationProviderConfig(project.ID, input.Action)
	if err != nil {
		return ProjectAutomationRenderedRequest{}, err
	}
	ctx, err := s.buildProjectAutomationContext(project, ruleID, input, triggerType, deliveryID, event)
	if err != nil {
		return ProjectAutomationRenderedRequest{}, err
	}
	ctxJSON, err := json.Marshal(ctx)
	if err != nil {
		return ProjectAutomationRenderedRequest{}, err
	}
	body := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": "你是项目自动化执行 Agent。你会收到来自璇础的项目上下文，请按用户指令执行。需要调用外部系统时，使用你所在 Agent 平台已配置的工具、skill、MCP 或 CLI。",
			},
			{
				"role":    "user",
				"content": input.InstructionTemplate + "\n\n<context>" + string(ctxJSON) + "</context>",
			},
		},
		"temperature": input.Action.Temperature,
	}
	bodyJSONBytes, err := json.Marshal(body)
	if err != nil {
		return ProjectAutomationRenderedRequest{}, err
	}
	sum := sha256.Sum256(bodyJSONBytes)
	return ProjectAutomationRenderedRequest{
		Method: http.MethodPost,
		URL: strings.TrimRight(baseURL, "/") + "/v1/chat/completions",
		Headers: map[string]string{
			"Authorization": "Bearer " + apiKey,
			"Content-Type":  "application/json",
		},
		MaskedHeaders: map[string]string{
			"Authorization": "Bearer ****",
			"Content-Type":  "application/json",
		},
		BodyJSON:    string(bodyJSONBytes),
		BodyPreview: truncateString(string(bodyJSONBytes), 12000),
		BodyHash:    "sha256:" + hex.EncodeToString(sum[:]),
		Model:       model,
	}, nil
}
```

Implement helper functions in the same file:

```go
func (s *Service) resolveProjectAutomationProviderConfig(projectID string, action ProjectAutomationActionConfig) (baseURL string, apiKey string, model string, err error) {
	baseURL, err = s.projectAutomationEffectiveConfigValue(projectID, action.BaseURLConfigKey)
	if err != nil || strings.TrimSpace(baseURL) == "" {
		return "", "", "", RuntimeError{Code: "automation_provider_config_missing", Message: "missing agent.provider.base_url"}
	}
	apiKey, err = s.projectAutomationEffectiveConfigValue(projectID, action.APIKeyConfigKey)
	if err != nil || strings.TrimSpace(apiKey) == "" {
		return "", "", "", RuntimeError{Code: "automation_provider_config_missing", Message: "missing agent.provider.api_key"}
	}
	model = strings.TrimSpace(action.ModelOverride)
	if model == "" {
		model, err = s.projectAutomationEffectiveConfigValue(projectID, action.ModelConfigKey)
		if err != nil || strings.TrimSpace(model) == "" {
			return "", "", "", RuntimeError{Code: "automation_provider_config_missing", Message: "missing agent.provider.model"}
		}
	}
	allowedHosts, err := s.projectAutomationAllowedHosts(projectID, action.AllowedHostsConfigKey)
	if err != nil {
		return "", "", "", err
	}
	if err := validateResolvedNotificationURL(strings.TrimRight(baseURL, "/")+"/v1/chat/completions", allowedHosts); err != nil {
		return "", "", "", err
	}
	return strings.TrimSpace(baseURL), strings.TrimSpace(apiKey), strings.TrimSpace(model), nil
}

func truncateString(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "[truncated]"
}

func (s *Service) projectAutomationEffectiveConfigValue(projectID string, key string) (string, error) {
	key, err := normalizeScopedConfigKey(key)
	if err != nil {
		return "", err
	}
	def, err := s.scopedConfigDefinition(key)
	if err != nil {
		return "", err
	}
	if v, ok, err := s.configRepo.Get(storage.ConfigKey{WorkspaceID: s.workspaceID, Scope: storage.ConfigScopeProject, ScopeID: projectID, Key: key}); err != nil || ok {
		return v, err
	}
	if v, ok, err := s.configRepo.Get(storage.ConfigKey{WorkspaceID: s.workspaceID, Scope: storage.ConfigScopeWorkspace, ScopeID: s.workspaceID, Key: key}); err != nil || ok {
		return v, err
	}
	if def.DefaultValue != nil {
		return *def.DefaultValue, nil
	}
	return "", nil
}

func (s *Service) projectAutomationAllowedHosts(projectID string, key string) ([]string, error) {
	if strings.TrimSpace(key) == "" {
		key = "agent.provider.allowed_hosts"
	}
	raw, err := s.projectAutomationEffectiveConfigValue(projectID, key)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw) == "" || strings.TrimSpace(raw) == "[]" {
		return nil, RuntimeError{Code: "automation_provider_allowed_hosts_missing", Message: "agent.provider.allowed_hosts is required"}
	}
	var hosts []string
	if err := json.Unmarshal([]byte(raw), &hosts); err != nil {
		return nil, RuntimeError{Code: "automation_provider_allowed_hosts_invalid", Message: "agent.provider.allowed_hosts must be a JSON string array"}
	}
	if len(hosts) == 0 {
		return nil, RuntimeError{Code: "automation_provider_allowed_hosts_missing", Message: "agent.provider.allowed_hosts is required"}
	}
	return hosts, nil
}
```

- [ ] **Step 7: Run app tests**

Run: `go test ./internal/app -run 'TestProjectAutomation' -v`

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/app/service.go internal/app/project_automation.go internal/app/project_automation_preview.go internal/app/project_automation_test.go
git commit -m "feat: 增加项目自动化规则服务"
```

---

## Task 3: 定时触发、事件触发与投递 dispatcher

**目标：** 实现 schedule daily_at 入队、project event 入队、manual test 入队、delivery dispatcher HTTP 投递、状态更新、重试和 provider request id / usage 提取。

**Files:**
- Create: `internal/app/project_automation_scheduler.go`
- Create: `internal/app/project_automation_dispatcher.go`
- Modify: `internal/app/event_notification.go` 或 `internal/app/service.go`
- Test: `internal/app/project_automation_test.go`
- Modify: server startup wiring under `internal/httpapi` or command serve files

**Interfaces:**
- Consumes:
  - `renderProjectAutomationRequest`
  - `ProjectAutomationRuleRepository`
  - `ProjectAutomationDeliveryRepository`
- Produces:
  - `type ProjectAutomationScheduler`
  - `func NewProjectAutomationScheduler(opts ProjectAutomationSchedulerOptions) *ProjectAutomationScheduler`
  - `func (s *ProjectAutomationScheduler) RunOnce(ctx context.Context) (ProjectAutomationSchedulerRunResult, error)`
  - `func (s *Service) EnqueueProjectAutomationForEvents(events []HookEvent) error`
  - `func (s *Service) TestProjectAutomationRule(projectRef string, ruleID string) (ProjectAutomationDeliveryView, error)`
  - `func (s *Service) ListProjectAutomationDeliveries(projectRef string, input ProjectAutomationDeliveryListInput) ([]ProjectAutomationDeliveryView, error)`
  - `func (s *Service) ProjectAutomationDeliveryInfo(projectRef string, deliveryID string) (ProjectAutomationDeliveryView, error)`
  - `func (s *Service) ReplayProjectAutomationDelivery(projectRef string, deliveryID string) (ProjectAutomationDeliveryView, error)`
  - `type ProjectAutomationDispatcher`
  - `func (d *ProjectAutomationDispatcher) RunOnce(ctx context.Context) (ProjectAutomationDispatchResult, error)`

- [ ] **Step 1: 写失败测试 — schedule 入队去重**

Append to `internal/app/project_automation_test.go`:

```go
func TestProjectAutomationSchedulerEnqueuesDailyRuleOnce(t *testing.T) {
	f := newServiceFixture(t)
	project, err := f.svc.AddProject(AddProjectInput{Slug: "adsops", Name: "广告投放优化"})
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	rule, err := f.svc.AddProjectAutomationRule(project.Slug, ProjectAutomationRuleAddInput{
		Name:          "每日项目巡检",
		Enabled:       true,
		TriggerType:   "schedule",
		TriggerConfig: ProjectAutomationTriggerConfig{ScheduleType: "daily_at", ScheduleValue: "09:30", Timezone: "Asia/Shanghai"},
		Action:        defaultAutomationActionForTest(),
		Context:       ProjectAutomationContextConfig{Include: []string{"workspace", "project"}},
		InstructionTemplate: "生成巡检",
	})
	if err != nil {
		t.Fatalf("AddProjectAutomationRule: %v", err)
	}
	defineProviderConfigForTest(t, f.svc)
	f.clock.NowUnix = mustUnix(t, "2026-07-08T09:31:00+08:00")
	scheduler := NewProjectAutomationScheduler(ProjectAutomationSchedulerOptions{Store: f.store, Clock: f.clock, ServiceFactory: f.serviceFactory})
	result, err := scheduler.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if result.DeliveriesEnqueued != 1 {
		t.Fatalf("DeliveriesEnqueued = %d", result.DeliveriesEnqueued)
	}
	result, err = scheduler.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce duplicate: %v", err)
	}
	if result.DeliveriesEnqueued != 0 {
		t.Fatalf("duplicate run enqueued %d deliveries for rule %s", result.DeliveriesEnqueued, rule.ID)
	}
}
```

- [ ] **Step 2: 写失败测试 — event task.assigned 只包含新增负责人**

Append:

```go
func TestProjectAutomationEventEnqueueUsesAddedAssignees(t *testing.T) {
	f := newServiceFixture(t)
	project, _ := f.svc.AddProject(AddProjectInput{Slug: "adsops", Name: "广告投放优化"})
	alice := createWorkspaceMemberForTest(t, f.svc, "alice")
	rule, err := f.svc.AddProjectAutomationRule(project.Slug, ProjectAutomationRuleAddInput{
		Name:          "分配任务后拉群",
		Enabled:       true,
		TriggerType:   "event",
		TriggerConfig: ProjectAutomationTriggerConfig{EventType: "task.assigned"},
		Condition:     ProjectAutomationCondition{OnlyAddedAssignees: true},
		Action:        defaultAutomationActionForTest(),
		Context:       ProjectAutomationContextConfig{Include: []string{"event", "task", "added_assignees", "project"}},
		InstructionTemplate: "处理新增负责人",
	})
	if err != nil {
		t.Fatalf("AddProjectAutomationRule: %v", err)
	}
	defineProviderConfigForTest(t, f.svc)
	task, err := f.svc.Add(AddInput{Title: "调整预算策略", Project: &project.Slug})
	if err != nil {
		t.Fatalf("Add task: %v", err)
	}
	err = f.svc.Modify(task.UUID, ModifyInput{AddAssignees: []string{alice.Name}})
	if err != nil {
		t.Fatalf("Modify assignee: %v", err)
	}
	deliveries, err := f.svc.ListProjectAutomationDeliveries(project.Slug, ProjectAutomationDeliveryListInput{RuleID: rule.ID})
	if err != nil {
		t.Fatalf("ListProjectAutomationDeliveries: %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("deliveries = %#v", deliveries)
	}
	if !contains(deliveries[0].RequestBodyPreview, `"added_assignees"`) || !contains(deliveries[0].RequestBodyPreview, alice.ID) {
		t.Fatalf("delivery preview missing added assignee: %s", deliveries[0].RequestBodyPreview)
	}
}
```

- [ ] **Step 3: 写失败测试 — dispatcher 投递成功**

Append:

```go
func TestProjectAutomationDispatcherSendsOpenAIRequest(t *testing.T) {
	f := newServiceFixture(t)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["model"] != "project-operator" {
			t.Fatalf("model = %#v", body["model"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_123","usage":{"prompt_tokens":10,"completion_tokens":4}}`))
	}))
	defer target.Close()
	project, _ := f.svc.AddProject(AddProjectInput{Slug: "adsops", Name: "广告投放优化"})
	defineProviderConfigForTestWithBaseURL(t, f.svc, target.URL)
	rule, _ := f.svc.AddProjectAutomationRule(project.Slug, ProjectAutomationRuleAddInput{
		Name: "每日项目巡检", Enabled: true, TriggerType: "schedule",
		TriggerConfig: ProjectAutomationTriggerConfig{ScheduleType: "daily_at", ScheduleValue: "09:30", Timezone: "Asia/Shanghai"},
		Action: defaultAutomationActionForTest(), Context: ProjectAutomationContextConfig{Include: []string{"project"}},
		InstructionTemplate: "巡检",
	})
	delivery, err := f.svc.TestProjectAutomationRule(project.Slug, rule.ID)
	if err != nil {
		t.Fatalf("TestProjectAutomationRule: %v", err)
	}
	dispatcher := NewProjectAutomationDispatcher(ProjectAutomationDispatcherOptions{Store: f.store, Clock: f.clock, Client: target.Client(), ServiceFactory: f.serviceFactory})
	result, err := dispatcher.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if result.Succeeded != 1 {
		t.Fatalf("dispatch result = %#v", result)
	}
	got, err := f.svc.ProjectAutomationDeliveryInfo(project.Slug, delivery.ID)
	if err != nil {
		t.Fatalf("ProjectAutomationDeliveryInfo: %v", err)
	}
	if got.Status != "succeeded" || got.ProviderRequestID != "chatcmpl_123" {
		t.Fatalf("delivery = %#v", got)
	}
}
```

- [ ] **Step 4: Run tests to verify failures**

Run: `go test ./internal/app -run 'TestProjectAutomation(Scheduler|Event|Dispatcher)' -v`

Expected: FAIL，scheduler/dispatcher/list delivery/test methods undefined.

- [ ] **Step 5: Implement scheduler**

Create `internal/app/project_automation_scheduler.go`:

```go
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/storage"
	"github.com/google/uuid"
)

type ProjectAutomationSchedulerOptions struct {
	Store          *storage.Store
	Clock          Clock
	ServiceFactory func(workspaceID string) *Service
	BatchSize      int
}

type ProjectAutomationSchedulerRunResult struct {
	RulesChecked       int
	DeliveriesEnqueued int
}

type ProjectAutomationScheduler struct {
	store          *storage.Store
	clock          Clock
	serviceFactory func(workspaceID string) *Service
	batchSize      int
}

func NewProjectAutomationScheduler(opts ProjectAutomationSchedulerOptions) *ProjectAutomationScheduler {
	if opts.Clock == nil {
		opts.Clock = RealClock{}
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = 100
	}
	return &ProjectAutomationScheduler{store: opts.Store, clock: opts.Clock, serviceFactory: opts.ServiceFactory, batchSize: opts.BatchSize}
}

func (s *ProjectAutomationScheduler) RunOnce(ctx context.Context) (ProjectAutomationSchedulerRunResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ruleRepo := storage.NewProjectAutomationRuleRepository(s.store.DB())
	deliveryRepo := storage.NewProjectAutomationDeliveryRepository(s.store.DB())
	rules, err := ruleRepo.ListEnabled()
	if err != nil {
		return ProjectAutomationSchedulerRunResult{}, err
	}
	now := s.clock.Unix()
	result := ProjectAutomationSchedulerRunResult{RulesChecked: len(rules)}
	for _, rule := range rules {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if rule.TriggerType != ProjectAutomationTriggerSchedule {
			continue
		}
		cfg := decodeProjectAutomationTriggerConfig(rule.TriggerConfigJSON)
		if cfg.ScheduleType != "daily_at" || !automationScheduleDue(cfg, now) {
			continue
		}
		key := fmt.Sprintf("%s:%s:%s:%s:%s", rule.WorkspaceID, rule.ProjectID, rule.ID, automationLocalDate(cfg, now), cfg.ScheduleValue)
		exists, err := deliveryRepo.ExistsByDedupeKey(key)
		if err != nil || exists {
			return result, err
		}
		svc := s.serviceFactory(rule.WorkspaceID)
		delivery, err := svc.buildProjectAutomationDelivery(rule, ProjectAutomationTriggerSchedule, "", "", key, now, nil)
		if err != nil {
			return result, err
		}
		if err := deliveryRepo.Enqueue([]storage.ProjectAutomationDelivery{delivery}); err != nil {
			return result, err
		}
		result.DeliveriesEnqueued++
	}
	return result, nil
}
```

Implement `automationScheduleDue`, `automationLocalDate`, and JSON decode helpers in the same file. Use `time.LoadLocation(cfg.Timezone)`, fallback to `time.Local` only when timezone is empty after validation.

- [ ] **Step 6: Implement event enqueue and delivery views**

In `internal/app/project_automation_scheduler.go` or `project_automation.go`, add:

```go
type ProjectAutomationDeliveryListInput struct {
	RuleID string
	Status string
	Limit  int
	Offset int
}

type ProjectAutomationDeliveryView struct {
	ID                  string         `json:"id"`
	WorkspaceID         string         `json:"workspace_id"`
	ProjectID           string         `json:"project_id"`
	RuleID              string         `json:"rule_id"`
	TriggerType         string         `json:"trigger_type"`
	EventID             string         `json:"event_id"`
	EventType           string         `json:"event_type"`
	Status              string         `json:"status"`
	ResolvedURL         string         `json:"resolved_url"`
	RenderedMethod      string         `json:"rendered_method"`
	RenderedHeaders     map[string][]string `json:"rendered_headers"`
	RequestBodyPreview  string         `json:"request_body_preview"`
	RequestBodyHash     string         `json:"request_body_hash"`
	ResponseStatusCode  *int           `json:"response_status_code"`
	ResponseBodyPreview string         `json:"response_body_preview"`
	ProviderRequestID   string         `json:"provider_request_id"`
	Usage               map[string]any `json:"usage"`
	AttemptCount        int            `json:"attempt_count"`
	NextAttemptAt       *int64         `json:"next_attempt_at"`
	LastError           string         `json:"last_error"`
	CreatedAt           int64          `json:"created_at"`
	ModifiedAt          int64          `json:"modified_at"`
}

func (s *Service) EnqueueProjectAutomationForEvents(events []HookEvent) error {
	for _, event := range events {
		if event.ProjectID == nil || event.EventID == "" {
			continue
		}
		rules, err := s.projectAutomationRuleRepo.List(s.workspaceID, event.ProjectID, false)
		if err != nil {
			return err
		}
		for _, rule := range rules {
			if rule.TriggerType != ProjectAutomationTriggerEvent {
				continue
			}
			cfg := decodeProjectAutomationTriggerConfig(rule.TriggerConfigJSON)
			if cfg.EventType != event.EventType {
				continue
			}
			condition := decodeProjectAutomationCondition(rule.ConditionJSON)
			if condition.OnlyAddedAssignees && event.EventType == "task.assigned" && automationEventListLen(event.Data["added_assignees"]) == 0 {
				continue
			}
			key := rule.WorkspaceID + ":" + rule.ProjectID + ":" + rule.ID + ":" + event.EventID
			delivery, err := s.buildProjectAutomationDelivery(rule, ProjectAutomationTriggerEvent, event.EventID, event.EventType, key, event.OccurredAt, &event)
			if err != nil {
				return err
			}
			if err := s.projectAutomationDeliveryRepo.Enqueue([]storage.ProjectAutomationDelivery{delivery}); err != nil {
				return err
			}
		}
	}
	return nil
}

func automationEventListLen(value any) int {
	switch typed := value.(type) {
	case []any:
		return len(typed)
	case []map[string]any:
		return len(typed)
	default:
		return 0
	}
}

func (s *Service) buildProjectAutomationDelivery(rule storage.ProjectAutomationRule, triggerType string, eventID string, eventType string, dedupeKey string, now int64, event *HookEvent) (storage.ProjectAutomationDelivery, error) {
	projectRow, err := s.ResolveProject(rule.ProjectID)
	if err != nil {
		return storage.ProjectAutomationDelivery{}, err
	}
	projectView, err := s.projectViewForRow(projectRow)
	if err != nil {
		return storage.ProjectAutomationDelivery{}, err
	}
	input := projectAutomationRuleAddInputFromRow(rule)
	deliveryID := uuid.NewString()
	rendered, err := s.renderProjectAutomationRequest(projectView, rule.ID, input, triggerType, deliveryID, event)
	if err != nil {
		return storage.ProjectAutomationDelivery{}, err
	}
	maskedHeaders := make(map[string][]string, len(rendered.MaskedHeaders))
	for key, value := range rendered.MaskedHeaders {
		maskedHeaders[key] = []string{value}
	}
	headersJSON, err := json.Marshal(maskedHeaders)
	if err != nil {
		return storage.ProjectAutomationDelivery{}, err
	}
	return storage.ProjectAutomationDelivery{
		ID:                  deliveryID,
		WorkspaceID:         rule.WorkspaceID,
		ProjectID:           rule.ProjectID,
		RuleID:              rule.ID,
		TriggerType:         triggerType,
		EventID:             eventID,
		EventType:           eventType,
		DedupeKey:           dedupeKey,
		Status:              storage.DeliveryStatusQueued,
		ResolvedURL:         rendered.URL,
		RenderedMethod:      rendered.Method,
		RenderedHeadersJSON: string(headersJSON),
		RequestBodyJSON:     rendered.BodyJSON,
		RequestBodyPreview:  rendered.BodyPreview,
		RequestBodyHash:     rendered.BodyHash,
		UsageJSON:           "{}",
		CreatedAt:           now,
		ModifiedAt:          now,
	}, nil
}
```

In the same app file, implement delivery reads and replay:

```go
func (s *Service) ListProjectAutomationDeliveries(projectRef string, input ProjectAutomationDeliveryListInput) ([]ProjectAutomationDeliveryView, error) {
	if err := s.requireProjectAutomationRead(); err != nil {
		return nil, err
	}
	project, err := s.ResolveProject(projectRef)
	if err != nil {
		return nil, err
	}
	rows, err := s.projectAutomationDeliveryRepo.List(storage.ProjectAutomationDeliveryListOptions{WorkspaceID: s.workspaceID, ProjectID: &project.ID, RuleID: input.RuleID, Status: input.Status, Limit: input.Limit, Offset: input.Offset})
	if err != nil {
		return nil, err
	}
	out := make([]ProjectAutomationDeliveryView, 0, len(rows))
	for _, row := range rows {
		view, err := projectAutomationDeliveryViewFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, view)
	}
	return out, nil
}

func (s *Service) ProjectAutomationDeliveryInfo(projectRef string, deliveryID string) (ProjectAutomationDeliveryView, error) {
	if err := s.requireProjectAutomationRead(); err != nil {
		return ProjectAutomationDeliveryView{}, err
	}
	project, err := s.ResolveProject(projectRef)
	if err != nil {
		return ProjectAutomationDeliveryView{}, err
	}
	row, err := s.projectAutomationDeliveryRepo.GetByID(deliveryID)
	if err != nil {
		return ProjectAutomationDeliveryView{}, err
	}
	if row.WorkspaceID != s.workspaceID || row.ProjectID != project.ID {
		return ProjectAutomationDeliveryView{}, RuntimeError{Code: "automation_delivery_not_found", Message: "automation delivery not found"}
	}
	return projectAutomationDeliveryViewFromRow(row)
}

func (s *Service) ReplayProjectAutomationDelivery(projectRef string, deliveryID string) (ProjectAutomationDeliveryView, error) {
	if err := s.requireProjectAutomationWrite(); err != nil {
		return ProjectAutomationDeliveryView{}, err
	}
	project, err := s.ResolveProject(projectRef)
	if err != nil {
		return ProjectAutomationDeliveryView{}, err
	}
	if isProjectClosed(project) {
		return ProjectAutomationDeliveryView{}, RuntimeError{Code: "project_closed", Message: "project is closed"}
	}
	row, err := s.projectAutomationDeliveryRepo.GetByID(deliveryID)
	if err != nil {
		return ProjectAutomationDeliveryView{}, err
	}
	if row.WorkspaceID != s.workspaceID || row.ProjectID != project.ID {
		return ProjectAutomationDeliveryView{}, RuntimeError{Code: "automation_delivery_not_found", Message: "automation delivery not found"}
	}
	if err := s.projectAutomationDeliveryRepo.Requeue(deliveryID, s.clock.Unix()); err != nil {
		return ProjectAutomationDeliveryView{}, err
	}
	requeued, err := s.projectAutomationDeliveryRepo.GetByID(deliveryID)
	if err != nil {
		return ProjectAutomationDeliveryView{}, err
	}
	return projectAutomationDeliveryViewFromRow(requeued)
}

func projectAutomationDeliveryViewFromRow(row storage.ProjectAutomationDelivery) (ProjectAutomationDeliveryView, error) {
	headers := map[string][]string{}
	if row.RenderedHeadersJSON != "" {
		if err := json.Unmarshal([]byte(row.RenderedHeadersJSON), &headers); err != nil {
			return ProjectAutomationDeliveryView{}, err
		}
	}
	usage := map[string]any{}
	if row.UsageJSON != "" {
		if err := json.Unmarshal([]byte(row.UsageJSON), &usage); err != nil {
			return ProjectAutomationDeliveryView{}, err
		}
	}
	return ProjectAutomationDeliveryView{
		ID: row.ID, WorkspaceID: row.WorkspaceID, ProjectID: row.ProjectID, RuleID: row.RuleID,
		TriggerType: row.TriggerType, EventID: row.EventID, EventType: row.EventType, Status: row.Status,
		ResolvedURL: row.ResolvedURL, RenderedMethod: row.RenderedMethod, RenderedHeaders: headers,
		RequestBodyPreview: row.RequestBodyPreview, RequestBodyHash: row.RequestBodyHash,
		ResponseStatusCode: row.ResponseStatusCode, ResponseBodyPreview: row.ResponseBodyPreview,
		ProviderRequestID: row.ProviderRequestID, Usage: usage, AttemptCount: row.AttemptCount,
		NextAttemptAt: row.NextAttemptAt, LastError: row.LastError, CreatedAt: row.CreatedAt, ModifiedAt: row.ModifiedAt,
	}, nil
}
```

Call `tx.EnqueueProjectAutomationForEvents(events)` in the same transaction path that already enqueues event notification deliveries after task/project events are built. Keep hook delivery behavior unchanged.

- [ ] **Step 7: Implement dispatcher**

Create `internal/app/project_automation_dispatcher.go`:

```go
package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

type ProjectAutomationDispatcherOptions struct {
	Store     *storage.Store
	Clock     Clock
	Client    *http.Client
	BatchSize int
	ServiceFactory func(workspaceID string) *Service
}

type ProjectAutomationDispatchResult struct {
	Claimed   int
	Succeeded int
	Retried   int
	Failed    int
}

type ProjectAutomationDispatcher struct {
	store     *storage.Store
	clock     Clock
	client    *http.Client
	batchSize int
	serviceFactory func(workspaceID string) *Service
}

func NewProjectAutomationDispatcher(opts ProjectAutomationDispatcherOptions) *ProjectAutomationDispatcher {
	if opts.Clock == nil {
		opts.Clock = RealClock{}
	}
	if opts.Client == nil {
		opts.Client = &http.Client{Timeout: 20 * time.Second}
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = 50
	}
	return &ProjectAutomationDispatcher{store: opts.Store, clock: opts.Clock, client: opts.Client, batchSize: opts.BatchSize, serviceFactory: opts.ServiceFactory}
}

func (d *ProjectAutomationDispatcher) RunOnce(ctx context.Context) (ProjectAutomationDispatchResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	repo := storage.NewProjectAutomationDeliveryRepository(d.store.DB())
	now := d.clock.Unix()
	rows, err := repo.ClaimDue(now, now+120, d.batchSize)
	if err != nil {
		return ProjectAutomationDispatchResult{}, err
	}
	result := ProjectAutomationDispatchResult{Claimed: len(rows)}
	for _, row := range rows {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		statusCode, responsePreview, providerRequestID, usageJSON, sendErr := d.send(ctx, row)
		if sendErr == nil && statusCode >= 200 && statusCode < 300 {
			if err := repo.MarkSucceeded(row.ID, now, statusCode, providerRequestID, responsePreview, usageJSON); err != nil {
				return result, err
			}
			result.Succeeded++
			continue
		}
		message := safeError(sendErr)
		if message == "" {
			message = http.StatusText(statusCode)
		}
		maxAttempts := d.maxAttemptsFor(row)
		if (statusCode == 429 || statusCode >= 500) && row.AttemptCount < maxAttempts {
			next := now + int64(min(row.AttemptCount, 5))*60
			if err := repo.MarkRetry(row.ID, now, next, intPtr(statusCode), message, responsePreview); err != nil {
				return result, err
			}
			result.Retried++
			continue
		}
		if err := repo.MarkFailed(row.ID, now, intPtr(statusCode), message, responsePreview); err != nil {
			return result, err
		}
		result.Failed++
	}
	return result, nil
}

func (d *ProjectAutomationDispatcher) maxAttemptsFor(row storage.ProjectAutomationDelivery) int {
	ruleRepo := storage.NewProjectAutomationRuleRepository(d.store.DB())
	rule, err := ruleRepo.GetByID(row.RuleID)
	if err != nil {
		return 5
	}
	input := projectAutomationRuleAddInputFromRow(rule)
	if input.Action.MaxAttempts <= 0 {
		return 5
	}
	return input.Action.MaxAttempts
}
```

Implement `send`:

```go
func (d *ProjectAutomationDispatcher) send(ctx context.Context, row storage.ProjectAutomationDelivery) (int, string, string, string, error) {
	ruleRepo := storage.NewProjectAutomationRuleRepository(d.store.DB())
	rule, err := ruleRepo.GetByID(row.RuleID)
	if err != nil {
		return 0, "", "", "{}", err
	}
	svc := d.serviceFactory(row.WorkspaceID)
	input := projectAutomationRuleAddInputFromRow(rule)
	_, apiKey, _, err := svc.resolveProjectAutomationProviderConfig(row.ProjectID, input.Action)
	if err != nil {
		return 0, "", "", "{}", err
	}
	if row.RequestBodyJSON == "" {
		return 0, "", "", "{}", RuntimeError{Code: "automation_delivery_body_missing", Message: "delivery request body is missing"}
	}
	if row.ResolvedURL == "" {
		return 0, "", "", "{}", RuntimeError{Code: "automation_delivery_url_missing", Message: "delivery resolved url is missing"}
	}
	method := row.RenderedMethod
	if method == "" {
		method = http.MethodPost
	}
	req, err := http.NewRequestWithContext(ctx, method, row.ResolvedURL, strings.NewReader(row.RequestBodyJSON))
	if err != nil {
		return 0, "", "", "{}", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.client.Do(req)
	if err != nil {
		return 0, "", "", "{}", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))
	preview := truncateString(string(raw), 12000)
	providerID, usageJSON := parseOpenAIProviderResponse(raw)
	return resp.StatusCode, preview, providerID, usageJSON, nil
}

func intPtr(value int) *int {
	return &value
}

func safeError(err error) string {
	if err == nil {
		return ""
	}
	return truncateString(err.Error(), 2000)
}

func parseOpenAIProviderResponse(raw []byte) (string, string) {
	var body struct {
		ID    string         `json:"id"`
		Usage map[string]any `json:"usage"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return "", "{}"
	}
	usageJSON := "{}"
	if body.Usage != nil {
		if b, err := json.Marshal(body.Usage); err == nil {
			usageJSON = string(b)
		}
	}
	return body.ID, usageJSON
}
```

Dispatcher 必须重新解析规则和 config secret 来生成真实 Authorization。`project_automation_deliveries.rendered_headers_json` 只保存遮掩 headers，不能保存 `Bearer sk-*` 明文。

- [ ] **Step 8: Run automation app tests**

Run: `go test ./internal/app -run 'TestProjectAutomation' -v`

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/app/project_automation_scheduler.go internal/app/project_automation_dispatcher.go internal/app/project_automation_test.go internal/app/event_notification.go internal/app/service.go
git commit -m "feat: 增加项目自动化触发和投递"
```

---

## Task 4: HTTP API

**目标：** 暴露项目自动化 CRUD、preview、test、delivery list/detail/replay API，并覆盖权限、project scope、closed project 和 secret redaction。

**Files:**
- Create: `internal/httpapi/project_automations.go`
- Create: `internal/httpapi/project_automations_test.go`
- Modify: `internal/httpapi/huma_routes.go`

**Interfaces:**
- Consumes app service methods from Tasks 2-3.
- Produces HTTP JSON contract for frontend:
  - `/api/v1/projects/{projectRef}/automations`
  - `/api/v1/projects/{projectRef}/automations/preview`
  - `/api/v1/projects/{projectRef}/automations/{ruleID}/test`
  - `/api/v1/projects/{projectRef}/automation-deliveries`

- [ ] **Step 1: 写失败测试 — HTTP preview masks secret**

Create `internal/httpapi/project_automations_test.go`:

```go
package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

func TestHTTPProjectAutomationPreviewMasksSecret(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "project:read", "project:write", "hook:read", "hook:write", "config:read", "config:write", "task:read")
	projectID := createHTTPProject(t, fixture, "adsops")
	createHTTPConfigDefinition(t, fixture, "agent.provider.base_url", false, "https://agent.example.com")
	createHTTPConfigDefinition(t, fixture, "agent.provider.api_key", true, "sk-real-secret")
	createHTTPConfigDefinition(t, fixture, "agent.provider.model", false, "project-operator")
	createHTTPConfigDefinition(t, fixture, "agent.provider.allowed_hosts", false, `["agent.example.com"]`)
	body := `{
		"name":"每日项目巡检",
		"enabled":true,
		"trigger_type":"schedule",
		"trigger_config":{"schedule_type":"daily_at","schedule_value":"09:30","timezone":"Asia/Shanghai"},
		"action":{"protocol":"chat_completions","base_url_config_key":"agent.provider.base_url","api_key_config_key":"agent.provider.api_key","model_config_key":"agent.provider.model","temperature":0.2},
		"context":{"include":["workspace","project","project_config"]},
		"instruction_template":"生成巡检"
	}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/projects/adsops/automations/preview", body, restfulFilterHeader(fixture.token))
	if rr.Code != http.StatusOK {
		t.Fatalf("preview status = %d body=%s project=%s", rr.Code, rr.Body.String(), projectID)
	}
	if strings.Contains(rr.Body.String(), "sk-real-secret") {
		t.Fatalf("preview leaked secret: %s", rr.Body.String())
	}
	for _, want := range []string{`"method":"POST"`, `"url":"https://agent.example.com/v1/chat/completions"`, `"Authorization":"Bearer ****"`, `"model":"project-operator"`} {
		if !strings.Contains(rr.Body.String(), want) {
			t.Fatalf("response missing %s: %s", want, rr.Body.String())
		}
	}
}
```

- [ ] **Step 2: 写失败测试 — lifecycle and closed project**

Append:

```go
func TestHTTPProjectAutomationLifecycle(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "project:read", "project:write", "hook:read", "hook:write")
	createHTTPProject(t, fixture, "adsops")
	body := `{
		"name":"分配任务后拉群",
		"enabled":true,
		"trigger_type":"event",
		"trigger_config":{"event_type":"task.assigned"},
		"condition":{"only_added_assignees":true},
		"action":{"protocol":"chat_completions","base_url_config_key":"agent.provider.base_url","api_key_config_key":"agent.provider.api_key","model_config_key":"agent.provider.model","temperature":0.2},
		"context":{"include":["event","task","added_assignees","project","project_config"]},
		"instruction_template":"处理新增负责人"
	}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/projects/adsops/automations", body, restfulFilterHeader(fixture.token))
	if rr.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", rr.Code, rr.Body.String())
	}
	created := httpResponseDataMap(t, rr)
	if created["trigger_type"] != "event" || created["action_type"] != "openai_compatible" {
		t.Fatalf("created = %#v", created)
	}
	ruleID := created["id"].(string)
	rr = requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/projects/adsops/automations", restfulFilterHeader(fixture.token))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), ruleID) {
		t.Fatalf("list status=%d body=%s", rr.Code, rr.Body.String())
	}
	rr = requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/projects/adsops/automations/"+ruleID+"/disable", `{}`, restfulFilterHeader(fixture.token))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"enabled":false`) {
		t.Fatalf("disable status=%d body=%s", rr.Code, rr.Body.String())
	}
}
```

Append a permissions regression test:

```go
func TestHTTPProjectAutomationRequiresProjectAndHookScopes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		scopes []string
	}{
		{name: "project-only", scopes: []string{"project:read", "project:write"}},
		{name: "hook-only", scopes: []string{"hook:read", "hook:write"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newHTTPServerWithTokenFixture(t, tc.scopes...)
			createHTTPProject(t, fixture, "adsops")
			rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/projects/adsops/automations", restfulFilterHeader(fixture.token))
			if rr.Code != http.StatusForbidden {
				t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
			}
		})
	}
}
```

- [ ] **Step 3: Run tests to verify failures**

Run: `go test ./internal/httpapi -run 'TestHTTPProjectAutomation' -v`

Expected: FAIL, route not found.

- [ ] **Step 4: Implement HTTP handlers**

Create `internal/httpapi/project_automations.go`:

```go
package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/auth"
)

type projectAutomationRuleRequest struct {
	Name                string `json:"name"`
	Description         string `json:"description"`
	Enabled             bool   `json:"enabled"`
	TriggerType         string `json:"trigger_type"`
	TriggerConfig       app.ProjectAutomationTriggerConfig `json:"trigger_config"`
	Condition           app.ProjectAutomationCondition `json:"condition"`
	Action              app.ProjectAutomationActionConfig `json:"action"`
	Context             app.ProjectAutomationContextConfig `json:"context"`
	InstructionTemplate string `json:"instruction_template"`
}

func (s *Server) handleProjectAutomationList(w http.ResponseWriter, r *http.Request) {
	scoped, err := s.scopedProjectAutomationService(r, chi.URLParam(r, "projectRef"), false)
	if err != nil {
		writeAppError(w, err)
		return
	}
	rows, err := scoped.ListProjectAutomationRules(chi.URLParam(r, "projectRef"), r.URL.Query().Get("all") == "true")
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, rows, nil)
}

func (s *Server) handleProjectAutomationCreate(w http.ResponseWriter, r *http.Request) {
	var req projectAutomationRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	scoped, err := s.scopedProjectAutomationService(r, chi.URLParam(r, "projectRef"), true)
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.AddProjectAutomationRule(chi.URLParam(r, "projectRef"), projectAutomationAddInput(req))
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, view, nil)
}

func (s *Server) handleProjectAutomationPreview(w http.ResponseWriter, r *http.Request) {
	var req projectAutomationRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	scoped, err := s.scopedProjectAutomationService(r, chi.URLParam(r, "projectRef"), true)
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.PreviewProjectAutomation(chi.URLParam(r, "projectRef"), app.ProjectAutomationPreviewInput(projectAutomationAddInput(req)))
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, view, nil)
}

func projectAutomationAddInput(req projectAutomationRuleRequest) app.ProjectAutomationRuleAddInput {
	return app.ProjectAutomationRuleAddInput{
		Name: req.Name, Description: req.Description, Enabled: req.Enabled,
		TriggerType: req.TriggerType, TriggerConfig: req.TriggerConfig,
		Condition: req.Condition, Action: req.Action, Context: req.Context,
		InstructionTemplate: req.InstructionTemplate,
	}
}

func (s *Server) scopedProjectAutomationService(r *http.Request, projectRef string, write bool) (*app.Service, error) {
	projectScope := auth.ScopeProjectRead
	projectPermission := app.PermissionProjectRead
	hookScope := auth.ScopeHookRead
	hookPermission := app.PermissionHookRead
	if write {
		projectScope = auth.ScopeProjectWrite
		projectPermission = app.PermissionProjectManage
		hookScope = auth.ScopeHookWrite
		hookPermission = app.PermissionHookWrite
	}
	scoped, _, err := s.scopedService(r, projectScope, projectPermission, projectRef)
	if err != nil {
		return nil, err
	}
	if _, _, err := s.scopedService(r, hookScope, hookPermission, projectRef); err != nil {
		return nil, err
	}
	return scoped, nil
}
```

Add handlers in the same file for `info`, `patch`, `delete`, `enable`, `disable`, `test`, saved-rule `preview`, delivery list/detail/replay. Each handler calls the matching app service method, scopes through `scopedProjectAutomationService` with `write=false` for reads and `write=true` for writes, and returns `writeSuccess` / `writeAppError` using the same envelope as `handleProjectAutomationCreate`. Parse `limit` with this helper:

```go
func queryLimit(r *http.Request, fallback int) int {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		return fallback
	}
	return limit
}
```

- [ ] **Step 5: Register routes**

Modify `internal/httpapi/huma_routes.go`:

```go
{Method: http.MethodGet, Path: "/api/v1/projects/{projectRef}/automations", Tag: "Project Automations", Summary: "List project automations.", Handler: s.handleProjectAutomationList},
{Method: http.MethodPost, Path: "/api/v1/projects/{projectRef}/automations", Tag: "Project Automations", Summary: "Create a project automation.", Handler: s.handleProjectAutomationCreate, Status: http.StatusCreated},
{Method: http.MethodPost, Path: "/api/v1/projects/{projectRef}/automations/preview", Tag: "Project Automations", Summary: "Preview a project automation delivery.", Handler: s.handleProjectAutomationPreview},
{Method: http.MethodPost, Path: "/api/v1/projects/{projectRef}/automations/{ruleID}/preview", Tag: "Project Automations", Summary: "Preview a saved project automation delivery.", Handler: s.handleProjectAutomationSavedPreview},
{Method: http.MethodGet, Path: "/api/v1/projects/{projectRef}/automations/{ruleID}", Tag: "Project Automations", Summary: "Get project automation details.", Handler: s.handleProjectAutomationInfo},
{Method: http.MethodPatch, Path: "/api/v1/projects/{projectRef}/automations/{ruleID}", Tag: "Project Automations", Summary: "Modify a project automation.", Handler: s.handleProjectAutomationModify},
{Method: http.MethodDelete, Path: "/api/v1/projects/{projectRef}/automations/{ruleID}", Tag: "Project Automations", Summary: "Delete a project automation.", Handler: s.handleProjectAutomationDelete},
{Method: http.MethodPost, Path: "/api/v1/projects/{projectRef}/automations/{ruleID}/enable", Tag: "Project Automations", Summary: "Enable a project automation.", Handler: s.handleProjectAutomationEnable},
{Method: http.MethodPost, Path: "/api/v1/projects/{projectRef}/automations/{ruleID}/disable", Tag: "Project Automations", Summary: "Disable a project automation.", Handler: s.handleProjectAutomationDisable},
{Method: http.MethodPost, Path: "/api/v1/projects/{projectRef}/automations/{ruleID}/test", Tag: "Project Automations", Summary: "Test a project automation.", Handler: s.handleProjectAutomationTest},
{Method: http.MethodGet, Path: "/api/v1/projects/{projectRef}/automation-deliveries", Tag: "Project Automations", Summary: "List project automation deliveries.", Handler: s.handleProjectAutomationDeliveryList},
{Method: http.MethodGet, Path: "/api/v1/projects/{projectRef}/automation-deliveries/{deliveryID}", Tag: "Project Automations", Summary: "Get project automation delivery details.", Handler: s.handleProjectAutomationDeliveryInfo},
{Method: http.MethodPost, Path: "/api/v1/projects/{projectRef}/automation-deliveries/{deliveryID}/replay", Tag: "Project Automations", Summary: "Replay project automation delivery.", Handler: s.handleProjectAutomationDeliveryReplay},
```

- [ ] **Step 6: Run HTTP tests**

Run: `go test ./internal/httpapi -run 'TestHTTPProjectAutomation' -v`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/httpapi/project_automations.go internal/httpapi/project_automations_test.go internal/httpapi/huma_routes.go
git commit -m "feat: 暴露项目自动化 HTTP 接口"
```

---

## Task 5: 前端 API、路由与项目 tab

**目标：** 新增前端 API client、项目自动化路由、tab 链接和权限 helper，不实现完整表单。

**Files:**
- Create: `web/src/routes/workspace/ProjectAutomationsRoute.tsx`
- Create: `web/src/features/workspace/project-workbench/automations/project-automations-api.ts`
- Modify: `web/src/routes/router.tsx`
- Modify: `web/src/features/workspace/project-workbench/project/project-tabs.tsx`
- Modify: `web/src/features/workspace/project-workbench/project/project-layout.test.tsx`
- Modify: `web/src/features/workspace/project-workbench/permissions/permissions.ts`
- Modify: `web/src/features/workspace/project-workbench/permissions/permissions.test.ts`

**Interfaces:**
- Produces:
  - `ProjectAutomationRule`, `ProjectAutomationDelivery`, `ProjectAutomationPreview`
  - `listProjectAutomations(projectSlug, includeDisabled)`
  - `previewProjectAutomation(projectSlug, input)`
  - route `/workspaces/$workspaceSlug/projects/$projectSlug/automations`

- [ ] **Step 1: 写失败测试 — tab link**

Modify `web/src/features/workspace/project-workbench/project/project-layout.test.tsx`:

```tsx
it("renders project automation tab link", async () => {
  renderProjectLayout({ activeTab: "automations" })
  const automationLink = await screen.findByRole("link", { name: "自动化" })
  expect(automationLink.getAttribute("href")).toBe("/workspaces/local/projects/ops/automations")
  expect(automationLink.getAttribute("aria-current")).toBe("page")
})
```

- [ ] **Step 2: 写失败测试 — permissions helper**

Modify `web/src/features/workspace/project-workbench/permissions/permissions.test.ts`:

```ts
it("allows project automation write only with project and hook write", () => {
  expect(canProjectAutomationWrite({ role: "admin", scopes: ["project:write", "hook:write"] })).toBe(true)
  expect(canProjectAutomationWrite({ role: "admin", scopes: ["project:write"] })).toBe(false)
  expect(canProjectAutomationWrite({ role: "viewer", scopes: ["project:write", "hook:write"] })).toBe(false)
})
```

- [ ] **Step 3: Run tests to verify failures**

Run: `pnpm --dir web test -- project-layout.test.tsx permissions.test.ts --run`

Expected: FAIL, tab key/helper missing.

- [ ] **Step 4: Implement API client**

Create `web/src/features/workspace/project-workbench/automations/project-automations-api.ts`:

```ts
import {
  workspaceApiDelete,
  workspaceApiGet,
  workspaceApiPatch,
  workspaceApiPost,
} from "@/features/workspace/session/workspace-api"

export type AutomationTriggerType = "schedule" | "event"
export type AutomationActionType = "openai_compatible"

export type AutomationTriggerConfig = {
  schedule_type?: "daily_at"
  schedule_value?: string
  timezone?: string
  event_type?: string
}

export type AutomationCondition = {
  task_filter?: string
  max_tasks?: number
  only_added_assignees?: boolean
}

export type AutomationActionConfig = {
  protocol: "chat_completions"
  base_url_config_key: string
  api_key_config_key: string
  model_config_key: string
  allowed_hosts_config_key?: string
  model_override?: string
  temperature: number
  max_attempts?: number
  attach_metadata?: boolean
}

export type AutomationContextConfig = {
  include: string[]
}

export type ProjectAutomationRule = {
  id: string
  workspace_id: string
  project_id: string
  name: string
  description: string
  enabled: boolean
  trigger_type: AutomationTriggerType
  trigger_config: AutomationTriggerConfig
  condition: AutomationCondition
  action_type: AutomationActionType
  action: AutomationActionConfig
  context: AutomationContextConfig
  instruction_template: string
  created_at: number
  modified_at: number
}

export type ProjectAutomationRuleInput = Omit<
  ProjectAutomationRule,
  "id" | "workspace_id" | "project_id" | "action_type" | "created_at" | "modified_at"
>

export type ProjectAutomationPreview = {
  method: string
  url: string
  headers: Record<string, string>
  body: unknown
  warnings: string[]
}

export type ProjectAutomationDelivery = {
  id: string
  workspace_id: string
  project_id: string
  rule_id: string
  trigger_type: AutomationTriggerType
  event_id: string
  event_type: string
  status: "queued" | "delivering" | "retry_wait" | "succeeded" | "dead_lettered"
  resolved_url: string
  rendered_method: string
  rendered_headers: Record<string, string[]>
  request_body_preview: string
  request_body_hash: string
  response_status_code?: number
  response_body_preview: string
  provider_request_id: string
  usage: Record<string, unknown>
  attempt_count: number
  next_attempt_at?: number
  last_error: string
  created_at: number
  modified_at: number
}

export async function listProjectAutomations(projectSlug: string, includeDisabled = true) {
  const q = includeDisabled ? "?all=true" : ""
  return workspaceApiGet<ProjectAutomationRule[]>(`/api/v1/projects/${projectSlug}/automations${q}`)
}

export async function previewProjectAutomation(projectSlug: string, input: ProjectAutomationRuleInput) {
  return workspaceApiPost<ProjectAutomationPreview>(`/api/v1/projects/${projectSlug}/automations/preview`, input)
}

export async function createProjectAutomation(projectSlug: string, input: ProjectAutomationRuleInput) {
  return workspaceApiPost<ProjectAutomationRule>(`/api/v1/projects/${projectSlug}/automations`, input)
}

export async function updateProjectAutomation(projectSlug: string, ruleID: string, input: Partial<ProjectAutomationRuleInput>) {
  return workspaceApiPatch<ProjectAutomationRule>(`/api/v1/projects/${projectSlug}/automations/${ruleID}`, input)
}

export async function deleteProjectAutomation(projectSlug: string, ruleID: string) {
  return workspaceApiDelete<{ deleted: true }>(`/api/v1/projects/${projectSlug}/automations/${ruleID}`)
}

export async function enableProjectAutomationRule(projectSlug: string, ruleID: string) {
  return workspaceApiPost<ProjectAutomationRule>(`/api/v1/projects/${projectSlug}/automations/${ruleID}/enable`)
}

export async function disableProjectAutomationRule(projectSlug: string, ruleID: string) {
  return workspaceApiPost<ProjectAutomationRule>(`/api/v1/projects/${projectSlug}/automations/${ruleID}/disable`)
}

export async function testProjectAutomationRule(projectSlug: string, ruleID: string) {
  return workspaceApiPost<ProjectAutomationDelivery>(`/api/v1/projects/${projectSlug}/automations/${ruleID}/test`)
}

export async function listProjectAutomationDeliveries(projectSlug: string, ruleID?: string) {
  const q = ruleID ? `?rule=${encodeURIComponent(ruleID)}` : ""
  return workspaceApiGet<ProjectAutomationDelivery[]>(`/api/v1/projects/${projectSlug}/automation-deliveries${q}`)
}
```

- [ ] **Step 5: Add route and tab**

Modify `web/src/features/workspace/project-workbench/project/project-tabs.tsx`:

```tsx
export type ProjectTabKey = "overview" | "tasks" | "activity" | "automations"
```

Add item:

```tsx
{
  key: "automations",
  label: t("projectSubpages.automations"),
  to: "/workspaces/$workspaceSlug/projects/$projectSlug/automations",
}
```

Create `web/src/routes/workspace/ProjectAutomationsRoute.tsx`:

```tsx
import { useParams } from "@tanstack/react-router"

import { ProjectAutomationsPage } from "@/features/workspace/project-workbench/automations/project-automations-page"
import { ProjectLayout } from "@/features/workspace/project-workbench/project/project-layout"

export function ProjectAutomationsRoute() {
  const params = useParams({ strict: false }) as {
    projectSlug: string
    workspaceSlug: string
  }

  return (
    <ProjectLayout
      activeTab="automations"
      projectSlug={params.projectSlug}
      workspaceSlug={params.workspaceSlug}
    >
      <ProjectAutomationsPage
        projectSlug={params.projectSlug}
        workspaceSlug={params.workspaceSlug}
      />
    </ProjectLayout>
  )
}
```

Modify `web/src/routes/router.tsx` with lazy import and route:

```tsx
const ProjectAutomationsRoute = lazy(() =>
  import("@/routes/workspace/ProjectAutomationsRoute").then((module) => ({
    default: module.ProjectAutomationsRoute,
  }))
)

const projectAutomationsRoute = createRoute({
  getParentRoute: () => workspaceRootRoute,
  path: "/workspaces/$workspaceSlug/projects/$projectSlug/automations",
  component: lazyRoute(ProjectAutomationsRoute),
})
```

Add `projectAutomationsRoute` next to other project subpage routes.

- [ ] **Step 6: Add permissions helper**

Modify `web/src/features/workspace/project-workbench/permissions/permissions.ts`:

```ts
export function canProjectAutomationRead(input: PermissionInput): boolean {
  return canProjectRead(input) && hasScope(input.scopes, "hook:read")
}

export function canProjectAutomationWrite(input: PermissionInput): boolean {
  return canProjectManage(input) && hasScope(input.scopes, "hook:write")
}
```

If `hasScope` is private, keep helper in same file. Preserve existing `*` scope semantics.

- [ ] **Step 7: Run frontend focused tests**

Run: `pnpm --dir web test -- project-layout.test.tsx permissions.test.ts --run`

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add web/src/routes/router.tsx web/src/routes/workspace/ProjectAutomationsRoute.tsx web/src/features/workspace/project-workbench/automations/project-automations-api.ts web/src/features/workspace/project-workbench/project/project-tabs.tsx web/src/features/workspace/project-workbench/project/project-layout.test.tsx web/src/features/workspace/project-workbench/permissions/permissions.ts web/src/features/workspace/project-workbench/permissions/permissions.test.ts
git commit -m "feat: 增加项目自动化路由"
```

---

## Task 6: 前端自动化页面、表单、预览弹窗和运行记录

**目标：** 实现项目自动化页面 UI，包括规则列表、模板创建、编辑表单、预览投递 JSON 弹窗、立即测试和运行记录。

**Files:**
- Create: `web/src/features/workspace/project-workbench/automations/project-automations-page.tsx`
- Create: `web/src/features/workspace/project-workbench/automations/automation-rule-form.tsx`
- Create: `web/src/features/workspace/project-workbench/automations/automation-preview-dialog.tsx`
- Create: `web/src/features/workspace/project-workbench/automations/automation-delivery-list.tsx`
- Create: `web/src/features/workspace/project-workbench/automations/project-automations-page.test.tsx`
- Create: `web/src/features/workspace/project-workbench/automations/automation-preview-dialog.test.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

**Interfaces:**
- Consumes Task 5 API client and permissions.
- Produces visible UI matching spec ASCII shape.

- [ ] **Step 1: 写失败测试 — page renders rules and preview button**

Create `web/src/features/workspace/project-workbench/automations/project-automations-page.test.tsx`:

```tsx
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import { ProjectAutomationsPage } from "./project-automations-page"
import {
  createProjectAutomation,
  testProjectAutomationRule,
} from "./project-automations-api"

vi.mock("./project-automations-api", async () => {
  return {
    listProjectAutomations: vi.fn(async () => [
      {
        id: "rule-1",
        workspace_id: "ws",
        project_id: "proj",
        name: "每日项目巡检",
        description: "每天检查",
        enabled: true,
        trigger_type: "schedule",
        trigger_config: { schedule_type: "daily_at", schedule_value: "09:30", timezone: "Asia/Shanghai" },
        condition: { task_filter: "status:pending", max_tasks: 50 },
        action_type: "openai_compatible",
        action: {
          protocol: "chat_completions",
          base_url_config_key: "agent.provider.base_url",
          api_key_config_key: "agent.provider.api_key",
          model_config_key: "agent.provider.model",
          temperature: 0.2,
        },
        context: { include: ["workspace", "project", "task_summary", "matched_tasks", "project_config"] },
        instruction_template: "生成巡检",
        created_at: 1,
        modified_at: 1,
      },
    ]),
    previewProjectAutomation: vi.fn(async () => ({
      method: "POST",
      url: "https://agent.example.com/v1/chat/completions",
      headers: { Authorization: "Bearer ****", "Content-Type": "application/json" },
      body: { model: "project-operator", messages: [] },
      warnings: [],
    })),
    createProjectAutomation: vi.fn(async () => ({ id: "rule-2" })),
    updateProjectAutomation: vi.fn(async () => ({ id: "rule-1" })),
    testProjectAutomationRule: vi.fn(async () => ({ id: "delivery-1" })),
    enableProjectAutomationRule: vi.fn(async () => ({ id: "rule-1", enabled: true })),
    disableProjectAutomationRule: vi.fn(async () => ({ id: "rule-1", enabled: false })),
    deleteProjectAutomation: vi.fn(async () => ({ deleted: true })),
    listProjectAutomationDeliveries: vi.fn(async () => []),
  }
})

function renderPage(options: { projectStatus?: "active" | "archived" } = {}) {
  const qc = new QueryClient()
  return render(
    <QueryClientProvider client={qc}>
      <ProjectAutomationsPage projectSlug="adsops" workspaceSlug="local" projectStatus={options.projectStatus ?? "active"} />
    </QueryClientProvider>
  )
}

describe("ProjectAutomationsPage", () => {
  it("shows rules and opens delivery JSON preview", async () => {
    renderPage()
    expect(await screen.findByText("每日项目巡检")).toBeTruthy()
    expect(screen.getByText("schedule / 每天 09:30")).toBeTruthy()
    await userEvent.click(screen.getByRole("button", { name: "预览投递 JSON" }))
    expect(await screen.findByRole("dialog", { name: "预览投递 JSON" })).toBeTruthy()
    await waitFor(() => expect(screen.getByText(/https:\/\/agent.example.com\/v1\/chat\/completions/)).toBeTruthy())
  })
})
```

- [ ] **Step 2: 写失败测试 — dialog masks auth and copies JSON**

Create `web/src/features/workspace/project-workbench/automations/automation-preview-dialog.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import { AutomationPreviewDialog } from "./automation-preview-dialog"

describe("AutomationPreviewDialog", () => {
  it("renders masked authorization and body JSON", async () => {
    const writeText = vi.fn()
    Object.assign(navigator, { clipboard: { writeText } })
    render(
      <AutomationPreviewDialog
        open
        onOpenChange={() => undefined}
        preview={{
          method: "POST",
          url: "https://agent.example.com/v1/chat/completions",
          headers: { Authorization: "Bearer ****", "Content-Type": "application/json" },
          body: { model: "project-operator", messages: [] },
          warnings: [],
        }}
      />
    )
    expect(screen.getByText("Authorization: Bearer ****")).toBeTruthy()
    expect(screen.getByText(/"model": "project-operator"/)).toBeTruthy()
    await userEvent.click(screen.getByRole("button", { name: "复制 JSON" }))
    expect(writeText).toHaveBeenCalledWith(expect.stringContaining('"model": "project-operator"'))
    expect(writeText.mock.calls[0][0]).not.toContain("sk-")
  })
})
```

- [ ] **Step 3: Run tests to verify failures**

Run: `pnpm --dir web test -- project-automations-page.test.tsx automation-preview-dialog.test.tsx --run`

Expected: FAIL, components missing.

- [ ] **Step 4: 写失败测试 — templates、row actions、closed project**

Append to `project-automations-page.test.tsx`:

```tsx
it("creates from template, tests a rule, and disables writes for closed projects", async () => {
  renderPage({ projectStatus: "archived" })
  expect(await screen.findByText("每日项目巡检")).toBeTruthy()
  expect(screen.getByRole("button", { name: "新建规则" })).toBeDisabled()
  expect(screen.getByRole("button", { name: "立即测试" })).toBeDisabled()

  renderPage({ projectStatus: "active" })
  await userEvent.click(await screen.findByRole("button", { name: "从模板创建" }))
  expect(screen.getByDisplayValue("分配任务后拉群")).toBeTruthy()
  expect(screen.getByDisplayValue("task.assigned")).toBeTruthy()
  await userEvent.click(screen.getByRole("button", { name: "保存" }))
  expect(createProjectAutomation).toHaveBeenCalled()
  await userEvent.click(screen.getByRole("button", { name: "立即测试" }))
  expect(testProjectAutomationRule).toHaveBeenCalledWith("adsops", "rule-1")
})
```

- [ ] **Step 5: Implement preview dialog**

Create `automation-preview-dialog.tsx`:

```tsx
import { CopyIcon, TerminalIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"

import type { ProjectAutomationPreview } from "./project-automations-api"

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  preview: ProjectAutomationPreview | null
}

export function AutomationPreviewDialog({ open, onOpenChange, preview }: Props) {
  const bodyJSON = preview ? JSON.stringify(preview.body, null, 2) : ""
  const curl = preview
    ? [
        `curl -s -X ${preview.method} ${JSON.stringify(preview.url)}`,
        `  -H "Authorization: Bearer \${AGENT_PROVIDER_API_KEY}"`,
        `  -H "Content-Type: application/json"`,
        `  -d ${JSON.stringify(bodyJSON)}`,
      ].join(" \\\n")
    : ""

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>预览投递 JSON</DialogTitle>
        </DialogHeader>
        {preview ? (
          <div className="space-y-3">
            <div className="space-y-1 text-sm">
              <div>
                {preview.method} {preview.url}
              </div>
              {Object.entries(preview.headers).map(([key, value]) => (
                <div key={key}>
                  {key}: {value}
                </div>
              ))}
            </div>
            <pre className="max-h-[420px] overflow-auto rounded-md border bg-muted p-3 text-xs">
              {bodyJSON}
            </pre>
            <div className="flex justify-end gap-2">
              <Button variant="outline" onClick={() => void navigator.clipboard?.writeText(bodyJSON)}>
                <CopyIcon className="mr-2 h-4 w-4" />
                复制 JSON
              </Button>
              <Button variant="outline" onClick={() => void navigator.clipboard?.writeText(curl)}>
                <TerminalIcon className="mr-2 h-4 w-4" />
                复制 curl
              </Button>
            </div>
          </div>
        ) : null}
      </DialogContent>
    </Dialog>
  )
}
```

- [ ] **Step 6: Implement form and page**

Create `automation-rule-form.tsx` with controlled form props:

```tsx
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"

import type { ProjectAutomationRuleInput } from "./project-automations-api"

export const defaultScheduleAutomationInput: ProjectAutomationRuleInput = {
  name: "每日项目巡检",
  description: "",
  enabled: true,
  trigger_type: "schedule",
  trigger_config: { schedule_type: "daily_at", schedule_value: "09:30", timezone: "Asia/Shanghai" },
  condition: { task_filter: "status:pending or status:waiting", max_tasks: 50 },
  action: {
    protocol: "chat_completions",
    base_url_config_key: "agent.provider.base_url",
    api_key_config_key: "agent.provider.api_key",
    model_config_key: "agent.provider.model",
    temperature: 0.2,
  },
  context: { include: ["workspace", "project", "task_summary", "matched_tasks", "project_config"] },
  instruction_template: "请读取这个项目的任务执行情况，生成项目巡检报告。如果项目配置中包含飞书群信息，请自行处理发送。",
}

export const assigneeFeishuTemplateInput: ProjectAutomationRuleInput = {
  name: "分配任务后拉群",
  description: "",
  enabled: true,
  trigger_type: "event",
  trigger_config: { event_type: "task.assigned" },
  condition: { only_added_assignees: true },
  action: defaultScheduleAutomationInput.action,
  context: { include: ["event", "task", "added_assignees", "project", "project_config"] },
  instruction_template: "有任务分配给了新负责人。请根据 added_assignees 和项目配置，完成后续协作动作。",
}

type Props = {
  value: ProjectAutomationRuleInput
  onChange: (value: ProjectAutomationRuleInput) => void
  onPreview: () => void
  onSave: () => void
  onTest: () => void
  disabled?: boolean
}

export function AutomationRuleForm({ value, onChange, onPreview, onSave, onTest, disabled }: Props) {
  return (
    <form className="space-y-4" onSubmit={(event) => event.preventDefault()}>
      <div className="grid gap-2">
        <label className="text-sm font-medium">名称</label>
        <Input
          value={value.name}
          onChange={(event) => onChange({ ...value, name: event.target.value })}
          disabled={disabled}
        />
      </div>
      <div className="grid gap-2">
        <label className="text-sm font-medium">触发类型</label>
        <select
          value={value.trigger_type}
          onChange={(event) =>
            onChange({
              ...value,
              trigger_type: event.target.value as ProjectAutomationRuleInput["trigger_type"],
            })
          }
          disabled={disabled}
        >
          <option value="schedule">定时触发</option>
          <option value="event">事件触发</option>
        </select>
      </div>
      <div className="grid gap-2">
        <label className="text-sm font-medium">事件</label>
        <Input
          value={value.trigger_config.event_type ?? ""}
          onChange={(event) =>
            onChange({
              ...value,
              trigger_config: { ...value.trigger_config, event_type: event.target.value },
            })
          }
          disabled={disabled || value.trigger_type !== "event"}
        />
      </div>
      <div className="grid gap-2">
        <label className="text-sm font-medium">指令模板</label>
        <Textarea
          value={value.instruction_template}
          onChange={(event) => onChange({ ...value, instruction_template: event.target.value })}
          disabled={disabled}
          rows={5}
        />
      </div>
      <div className="flex gap-2">
        <Button type="button" variant="outline" onClick={onPreview} disabled={disabled}>预览投递 JSON</Button>
        <Button type="button" onClick={onSave} disabled={disabled}>保存</Button>
        <Button type="button" variant="outline" onClick={onTest} disabled={disabled}>立即测试</Button>
      </div>
    </form>
  )
}
```

Create `project-automations-page.tsx`:

```tsx
import { useMutation, useQuery } from "@tanstack/react-query"
import { useState } from "react"

import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { useProjectLayout } from "@/features/workspace/project-workbench/project/project-layout"

import {
  listProjectAutomations,
  createProjectAutomation,
  previewProjectAutomation,
  testProjectAutomationRule,
  type ProjectAutomationPreview,
  type ProjectAutomationRuleInput,
} from "./project-automations-api"
import { AutomationPreviewDialog } from "./automation-preview-dialog"
import { AutomationDeliveryList } from "./automation-delivery-list"
import { AutomationRuleForm, assigneeFeishuTemplateInput, defaultScheduleAutomationInput } from "./automation-rule-form"

type Props = {
  projectSlug: string
  workspaceSlug: string
  projectStatus?: "active" | "archived" | "cancelled"
}

export function ProjectAutomationsPage({ projectSlug, projectStatus = "active" }: Props) {
  const layout = useProjectLayout()
  const writeDisabled = layout.closed || projectStatus === "archived" || projectStatus === "cancelled"
  const [draft, setDraft] = useState<ProjectAutomationRuleInput>(defaultScheduleAutomationInput)
  const [preview, setPreview] = useState<ProjectAutomationPreview | null>(null)
  const [previewOpen, setPreviewOpen] = useState(false)
  const rules = useQuery({
    queryKey: ["project", projectSlug, "automations"],
    queryFn: () => listProjectAutomations(projectSlug, true),
  })
  const previewMutation = useMutation({
    mutationFn: (input: ProjectAutomationRuleInput) => previewProjectAutomation(projectSlug, input),
    onSuccess: (data) => {
      setPreview(data)
      setPreviewOpen(true)
    },
  })
  const createMutation = useMutation({
    mutationFn: (input: ProjectAutomationRuleInput) => createProjectAutomation(projectSlug, input),
    onSuccess: () => rules.refetch(),
  })
  const testMutation = useMutation({
    mutationFn: (ruleID: string) => testProjectAutomationRule(projectSlug, ruleID),
  })

  if (rules.isPending) return <Skeleton className="h-48 w-full" />

  return (
    <section className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-base font-semibold">自动化</h2>
        <div className="flex gap-2">
          <Button type="button" variant="outline" disabled={writeDisabled} onClick={() => setDraft(assigneeFeishuTemplateInput)}>从模板创建</Button>
          <Button type="button" disabled={writeDisabled}>新建规则</Button>
        </div>
      </div>
      <div className="overflow-hidden rounded-md border">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b bg-muted/40 text-left">
              <th className="p-2">状态</th>
              <th className="p-2">名称</th>
              <th className="p-2">触发器</th>
              <th className="p-2">动作</th>
            </tr>
          </thead>
          <tbody>
            {(rules.data ?? []).map((rule) => (
              <tr key={rule.id} className="border-b">
                <td className="p-2">{rule.enabled ? "启用" : "停用"}</td>
                <td className="p-2 font-medium">{rule.name}</td>
                <td className="p-2">{triggerSummary(rule)}</td>
                <td className="p-2">Agent Provider</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <AutomationRuleForm
        value={draft}
        onChange={setDraft}
        onPreview={() => previewMutation.mutate(draft)}
        onSave={() => createMutation.mutate(draft)}
        onTest={() => {
          const firstRule = rules.data?.[0]
          if (firstRule) testMutation.mutate(firstRule.id)
        }}
        disabled={writeDisabled}
      />
      <AutomationDeliveryList projectSlug={projectSlug} />
      <AutomationPreviewDialog open={previewOpen} onOpenChange={setPreviewOpen} preview={preview} />
    </section>
  )
}

function triggerSummary(rule: { trigger_type: string; trigger_config: { schedule_value?: string; event_type?: string } }) {
  if (rule.trigger_type === "schedule") return `schedule / 每天 ${rule.trigger_config.schedule_value ?? ""}`
  return `event / ${rule.trigger_config.event_type ?? ""}`
}
```

Add row buttons for enable/disable/delete/test in the rule table. Each button calls the matching mutation from `project-automations-api.ts`, then refetches `["project", projectSlug, "automations"]`. Closed project state must disable `新建规则`、`保存`、`启用/停用`、`删除`、`立即测试` and `replay`; read-only delivery list remains visible.

- [ ] **Step 7: Add delivery list**

Create `automation-delivery-list.tsx`:

```tsx
import { useQuery } from "@tanstack/react-query"

import { listProjectAutomationDeliveries } from "./project-automations-api"

type Props = {
  projectSlug: string
  ruleID?: string
}

export function AutomationDeliveryList({ projectSlug, ruleID }: Props) {
  const deliveries = useQuery({
    queryKey: ["project", projectSlug, "automation-deliveries", ruleID ?? ""],
    queryFn: () => listProjectAutomationDeliveries(projectSlug, ruleID),
  })
  const rows = deliveries.data ?? []

  return (
    <section className="space-y-2">
      <h3 className="text-sm font-medium">运行记录</h3>
      {rows.length === 0 ? (
        <div className="rounded-md border p-4 text-sm text-muted-foreground">
          暂无运行记录
        </div>
      ) : (
        <div className="overflow-hidden rounded-md border">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b bg-muted/40 text-left">
                <th className="p-2">状态</th>
                <th className="p-2">触发</th>
                <th className="p-2">尝试</th>
                <th className="p-2">Provider ID</th>
                <th className="p-2">错误</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => (
                <tr key={row.id} className="border-b">
                  <td className="p-2">{row.status}</td>
                  <td className="p-2">{row.event_type || row.trigger_type}</td>
                  <td className="p-2">{row.attempt_count}</td>
                  <td className="p-2">{row.provider_request_id || "-"}</td>
                  <td className="p-2">{row.last_error || "-"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  )
}
```

- [ ] **Step 8: Add i18n keys**

Modify `zh-CN.ts`:

```ts
projectSubpages: {
  automations: "自动化",
}
```

Modify `en-US.ts`:

```ts
projectSubpages: {
  automations: "Automations",
}
```

Preserve existing keys; only add missing key.

- [ ] **Step 9: Run frontend tests**

Run: `pnpm --dir web test -- project-automations-page.test.tsx automation-preview-dialog.test.tsx --run`

Expected: PASS.

Run: `pnpm --dir web typecheck`

Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add web/src/features/workspace/project-workbench/automations web/src/locales/zh-CN.ts web/src/locales/en-US.ts
git commit -m "feat: 增加项目自动化页面"
```

---

## Task 7: 文档、全量验证和收口

**目标：** 同步 README/ROADMAP，跑完整后端和前端验证，确保 spec、实现和用户可见文档一致。

**Files:**
- Modify: `README.md`
- Modify: `ROADMAP.md`
- Verify: all touched files

**Interfaces:**
- Consumes all previous tasks.
- Produces shippable feature branch.

- [ ] **Step 1: 更新 README**

Add to Web Console section in `README.md`:

```md
- **项目自动化页**（`/workspaces/<workspace-slug>/projects/<project-slug>/automations`）：项目详情新增「自动化」tab，可配置 project-scoped 定时规则和事件规则。首版动作统一为调用 OpenAI 兼容 Agent Provider，默认投递到 `{agent.provider.base_url}/v1/chat/completions`，凭据和默认 model 从 project effective config 读取；规则编辑页可点击「预览投递 JSON」查看脱敏后的最终请求体。璇础只负责触发、上下文构造、投递和记录，不直接调用飞书，也不判断 Agent 是否完成外部动作。
```

Add config examples near project config section:

```md
./xuanchu config schema set agent.provider.base_url --type string --scope workspace,project
./xuanchu config schema set agent.provider.api_key --type string --scope workspace,project --secret
./xuanchu config schema set agent.provider.model --type string --scope workspace,project
./xuanchu config schema set agent.provider.protocol --type string --scope workspace,project
./xuanchu config schema set agent.provider.allowed_hosts --type json --scope workspace,project
./xuanchu config schema set feishu.chat_id --type string --scope project
```

- [ ] **Step 2: 更新 ROADMAP**

Add a short line under current Web Console milestone:

```md
- 项目自动化：在项目详情页配置定时/事件触发，按 OpenAI 兼容接口投递项目上下文给外部 Agent Provider，并记录投递结果。
```

- [ ] **Step 3: 后端完整验证**

Run:

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
go vet ./...
```

Expected:

```text
PASS for all go test packages
build exits 0
go vet exits 0
```

- [ ] **Step 4: 前端完整验证**

Run:

```bash
pnpm --dir web test
pnpm --dir web typecheck
pnpm --dir web lint
pnpm --dir web build
```

Expected:

```text
tests pass
typecheck exits 0
lint exits 0
build exits 0
```

- [ ] **Step 5: Diff hygiene**

Run:

```bash
git diff --check
git status --short
```

Expected:

```text
git diff --check exits 0
status shows only intended implementation/docs files
```

If unrelated user changes are present, leave them unstaged and note them in the handoff. Do not revert them.

- [ ] **Step 6: Commit docs and final polish**

```bash
git add README.md ROADMAP.md
git commit -m "docs: 补充项目自动化说明"
```

---

## Self-Review Checklist

- [ ] Spec §0 产品结论：Task 2/3/6 cover project event/context orchestration and OpenAI-compatible provider, not Yaoguang-specific behavior.
- [ ] Spec §2 goals: Task 2 covers rule CRUD and preview; Task 3 covers trigger/delivery; Task 4 covers APIs; Task 6 covers UI.
- [ ] Spec §3 non-goals: No Feishu direct calls appear in any task.
- [ ] Spec §4 decisions: Task 1 uses project-scoped tables; Task 2 defaults chat completions; Task 6 preview is button-triggered dialog.
- [ ] Spec §7 preview: Task 2/4/6 implement unsaved-form preview and masked headers.
- [ ] Spec §8 OpenAI body: Task 2 renders minimal `model/messages/temperature`, `_xuanchu` inside context, no default metadata.
- [ ] Spec §9 config keys: Task 2 and Task 7 use exact `agent.provider.*` and `feishu.chat_id` keys.
- [ ] Spec §10 data model: Task 1 creates rule and delivery tables.
- [ ] Spec §11 API: Task 4 registers all routes.
- [ ] Spec §12 schedule/event semantics: Task 3 covers daily dedupe and `task.assigned` added assignees.
- [ ] Spec §13 security: Task 2/4/6 mask secrets; Task 2 blocks closed project writes.
- [ ] Spec §16 tests: each task has focused tests; Task 7 runs full verification.
