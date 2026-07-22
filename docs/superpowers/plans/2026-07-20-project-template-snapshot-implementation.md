# 项目模板与版本化快照 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在同一 workspace 内把项目中显式选择的 config、普通 task、TaskSeries 和 project automation 保存为不可变 JSON Snapshot，并通过 Web Console 完整治理、通过 CLI/Remote/MCP 的受限入口从 current Snapshot 原子创建新项目。

**Architecture:** `internal/projecttemplate` 定义带 schema 版本的纯 Go Snapshot 契约、严格 codec、canonical hash、相对日期和本地引用校验；`internal/storage` 只持久化 Template 元数据与原始 `snapshot_json TEXT`，并提供有界候选查询；`internal/app` 统一编排权限、Capture Preview/Capture、Instantiate Preview/Instantiate、审计与事件。HTTP/Web 提供完整治理，CLI、Remote 和 MCP 只能 list active Template 与实例化 list 返回的 current Snapshot。

**Tech Stack:** Go 1.25、GORM、`github.com/glebarez/sqlite`、`gorm.io/driver/postgres`、Goldmark、Huma/OpenAPI、Cobra、MCP Go SDK、React 19、TypeScript 6、TanStack Query/Router、Vitest、Playwright。

## Global Constraints

- 设计依据固定为 `docs/superpowers/specs/2026-07-20-project-template-snapshot-design.md`；实现中发现需要改变产品边界时先修改并重新审阅 spec。
- 只允许同一 workspace 复用；所有 Template/Snapshot repository 查询必须同时带 `workspace_id`。
- Snapshot 内容只保存于 `project_template_snapshots.snapshot_json TEXT`；禁止建立 task/series/config/automation 模板子表，禁止依赖 SQLite/PostgreSQL 方言特有 JSON 类型。
- Snapshot 使用 `xuanchu.project-template-snapshot/v1` Go struct 严格编解码；拒绝未知字段、未知 schema、trailing JSON、非法 ref 和超限内容。
- Snapshot 不可变；更新 Template 只能追加 version 并原子更新 `current_snapshot_id`。
- 普通 task 生成新 UUID、开放状态和新 project seq；Series 生成 active Series 与一条 initial RuleVersion，不复制 occurrence、历史 RuleVersion、tombstone、skip 或 backlog。
- 只保存 project 显式 config；automation 选择会在服务端自动闭包其 provider config 依赖。secret 以 `secret_copy` 的 AES-GCM 密文保存到原始 Snapshot，实例化时直接复制；明文和密文均不得进入 view、audit、日志、错误或 preview response，旧 `secret_input` 仅兼容历史 Snapshot。
- Automation 只复制规则定义，新规则固定 `enabled=false`，不复制 delivery/retry/request/response。
- Capture 只接受四个显式选择数组；filter/query 只用于找候选和展开 refs，不进入 Snapshot。候选 GET 支持重复 `ref` 精确重取（最多 100 个）；来源 drift 后 Web 必须按已选 stable ref 分批重取摘要，不能只刷新当前分页或退化为模糊搜索。
- 候选默认 `limit=50`、最大 100；Snapshot 上限 task 1,000、series 200、config 500、automation 200、canonical JSON 8 MiB。
- 日期使用 workspace timezone 的 `day_offset + local_time` 日历运算；禁止用固定秒数模拟天数。
- Instantiate 必须再次完整预检并在单一 `Store.Transaction` 中创建；任一步失败不得留下 Project 或任何子资源。
- project-scoped token 对所有 Template API 返回 `project_scope_denied`。
- 对外用户身份统一使用 `task.UserInfo` / `task.JSONUserInfo`，actor 使用 `task.ActorInfo` / `task.JSONActorInfo`，禁止输出裸用户 UUID。
- CLI 精确只有 `xuanchu project template list` 和 `xuanchu project template instantiate`；Remote Client 精确只有对应两个 typed 方法。
- MCP 精确只新增 `project_template_list`、`project_template_instantiate`，名称使用下划线；两者每次显式传 workspace，且只能使用 current Snapshot。
- SQLite 继续使用 `github.com/glebarez/sqlite`，不得引入 CGO SQLite；必须保持 `CGO_ENABLED=0` 测试和构建通过。
- 文档和代码注释以中文为主；stdout 只放结果，stderr 放错误，JSON DTO 保持稳定。

---

## 文件职责与共享接口

### 新增文件

- `internal/projecttemplate/model.go`：V1 Snapshot、blueprint、limits 和 current model。
- `internal/projecttemplate/codec.go`：schema dispatch、strict decode、canonical encode 和 SHA-256。
- `internal/projecttemplate/validate.go`：字段、数组上限、local ref、依赖图和 secret invariant。
- `internal/projecttemplate/date.go`：anchor/start date 与相对本地时间互转。
- `internal/projecttemplate/reference.go`：Snapshot 专用 Markdown task/user/attachment ref 扫描与改写。
- `internal/projecttemplate/*_test.go`：纯函数契约测试。
- `internal/storage/project_template_repo.go`：Template/Snapshot repository、版本锁和分页。
- `internal/storage/project_template_repo_test.go`：SQLite 生命周期、隔离、唯一性和并发测试。
- `internal/app/project_template.go`：公共 DTO、metadata lifecycle、受限 list/current instantiate 用例。
- `internal/app/project_template_candidates.go`：四类候选、count/pagination 和全匹配选择展开。
- `internal/app/project_template_capture.go`：Capture source load、preview、source hash 和 Snapshot 写入。
- `internal/app/project_template_reference.go`：Capture resolution、local ref 和正文引用映射。
- `internal/app/project_template_instantiate.go`：Instantiate preview、预分配身份和原子创建。
- `internal/app/project_template_test.go`、`project_template_candidates_test.go`、`project_template_capture_test.go`、`project_template_instantiate_test.go`：App 行为测试。
- `internal/httpapi/project_templates.go`、`project_templates_test.go`：完整 Web 治理 HTTP API。
- `internal/remote/project_template.go`、`project_template_test.go`：仅 list/current instantiate ��� Remote Client。
- `internal/mcpserver/tools_project_template.go`、`tools_project_template_test.go`：两个 MCP tool。
- `web/src/features/workspace/project-templates/api/project-template-api.ts`：HTTP DTO 与 client。
- `web/src/features/workspace/project-templates/api/project-template-api.test.ts`：URL、body 和 secret redaction 测试。
- `web/src/features/workspace/project-templates/project-template-library-page.tsx`：模板库、版本摘要和生命周期操作。
- `web/src/features/workspace/project-templates/project-template-library-page.test.tsx`：模板库状态测试。
- `web/src/features/workspace/project-templates/capture/project-template-capture-wizard.tsx`：四步 Capture 流程状态机。
- `web/src/features/workspace/project-templates/capture/candidate-picker.tsx`：服务端筛选、分页和本页选择。
- `web/src/features/workspace/project-templates/capture/selected-items-sheet.tsx`：跨页显式已选清单。
- `web/src/features/workspace/project-templates/capture/project-template-capture-wizard.test.tsx`：Capture 交互测试。
- `web/src/features/workspace/project-templates/instantiate/project-template-instantiate-wizard.tsx`：模板选择、项目输入、secret/member 处理和创建。
- `web/src/features/workspace/project-templates/instantiate/project-template-instantiate-wizard.test.tsx`：Instantiate 交互测试。
- `web/src/features/workspace/project-templates/workspace-settings-nav.tsx`：workspace 设置内“配置定义/项目模板”导航。
- `web/src/routes/workspace/ProjectTemplatesRoute.tsx`：`/settings/project-templates` 路由适配。
- `web/scripts/playwright-project-template-smoke.mjs`：桌面/移动端关键流程 smoke。

### 跨任务固定接口

```go
// internal/projecttemplate/model.go
const SnapshotSchemaV1 = "xuanchu.project-template-snapshot/v1"

type SnapshotV1 struct {
    Schema      string                  `json:"schema"`
    AnchorDate  string                  `json:"anchor_date"`
    Project     ProjectBlueprintV1      `json:"project"`
    Configs     []ConfigBlueprintV1     `json:"configs"`
    Tasks       []TaskBlueprintV1       `json:"tasks"`
    Series      []SeriesBlueprintV1     `json:"series"`
    Automations []AutomationBlueprintV1 `json:"automations"`
}

type Snapshot = SnapshotV1

type Limits struct {
    MaxTasks, MaxSeries, MaxConfigs, MaxAutomations int
    MaxJSONBytes, MaxTextBytes int
}

var DefaultLimits = Limits{
    MaxTasks: 1000, MaxSeries: 200, MaxConfigs: 500, MaxAutomations: 200,
    MaxJSONBytes: 8 << 20, MaxTextBytes: 512 << 10,
}

func EncodeV1(SnapshotV1, Limits) (canonical []byte, sha256Hex string, err error)
func Decode([]byte, Limits) (Snapshot, error)
func ValidateSnapshot(Snapshot, Limits) error
func ToRelativeLocalTime(unix int64, anchorDate string, loc *time.Location) (RelativeLocalTimeV1, error)
func FromRelativeLocalTime(RelativeLocalTimeV1, startDate string, loc *time.Location) (int64, error)

type Error struct { Code, Message string }
func (e Error) Error() string
func ErrorCode(error) string
```

```go
// internal/storage/project_template_repo.go
type ProjectTemplateListOptions struct { WorkspaceID, Status, Q string; Limit, Offset int }
type ProjectTemplatePage struct { Items []ProjectTemplate; Total, Limit, Offset int }

type ProjectTemplateRepository struct { db *gorm.DB }
func NewProjectTemplateRepository(*gorm.DB) *ProjectTemplateRepository
func (r *ProjectTemplateRepository) Create(ProjectTemplate) error
func (r *ProjectTemplateRepository) GetByRef(workspaceID, ref string) (ProjectTemplate, error)
func (r *ProjectTemplateRepository) List(ProjectTemplateListOptions) (ProjectTemplatePage, error)
func (r *ProjectTemplateRepository) UpdateMetadata(workspaceID, id, name, description string, modifiedAt int64) error
func (r *ProjectTemplateRepository) SetStatus(workspaceID, id, status string, archivedAt *int64, modifiedAt int64) error
func (r *ProjectTemplateRepository) AppendSnapshotLocked(workspaceID, templateID string, row ProjectTemplateSnapshot) (ProjectTemplateSnapshot, error)
func (r *ProjectTemplateRepository) GetSnapshot(workspaceID, templateID, snapshotID string) (ProjectTemplateSnapshot, error)
func (r *ProjectTemplateRepository) ListSnapshots(workspaceID, templateID string) ([]ProjectTemplateSnapshot, error)
```

```go
// internal/app/project_template.go
type ComponentCounts struct { Configs, Tasks, Series, Automations int }
type ProjectTemplateSummaryView struct {
    ID, Key, Name, Description, Status string
    CurrentSnapshot *ProjectTemplateSnapshotSummaryView
    CreatedBy task.ActorInfo
    CreatedAt, ModifiedAt int64
    ArchivedAt *int64
}
type ProjectTemplateSnapshotSummaryView struct {
    ID string; Version int64; Hash, SourceProjectID string
    Counts ComponentCounts; RequiredSecretKeys []string
    CreatedBy task.ActorInfo; CreatedAt int64
}
type ProjectTemplatePage struct { Items []ProjectTemplateSummaryView; Total, Limit, Offset int }
type ProjectTemplateView struct { Template ProjectTemplateSummaryView; Snapshot *ProjectTemplateSnapshotView; Versions []ProjectTemplateSnapshotSummaryView }
type ProjectTemplateSnapshotView struct {
    Project projecttemplate.ProjectBlueprintV1
    Configs []ProjectTemplateConfigView
    Tasks []ProjectTemplateTaskView
    Series []ProjectTemplateSeriesView
    Automations []ProjectTemplateAutomationView
}
type ProjectTemplateConfigView struct { Key, Mode string; Value *string }
type ProjectTemplateTaskView struct {
    Ref, Title string
    Description, Priority *string
    Tags []string
    Assignees []task.UserInfo
    UDAs map[string]projecttemplate.UDABlueprintV1
    Dates projecttemplate.TaskDatesV1
    ParentRef *string
    DependsRefs []string
    Links []projecttemplate.TaskLinkBlueprintV1
}
type ProjectTemplateSeriesView struct {
    Ref, Title, RecurrenceRule string
    Description, Priority *string
    Tags []string
    Assignees []task.UserInfo
    UDAs map[string]projecttemplate.UDABlueprintV1
    FirstDue projecttemplate.RelativeLocalTimeV1
    Until *projecttemplate.RelativeLocalTimeV1
}
type ProjectTemplateAutomationView = projecttemplate.AutomationBlueprintV1
type ProjectTemplateDatePreview struct {
    Component, SourceRef, Field string
    SourceUnix int64
    Relative projecttemplate.RelativeLocalTimeV1
}
type TemplateInstantiationListInput struct { Q string; Limit, Offset int }
type ModifyTemplateInput struct { Name, Description *string }
type ProjectTemplateIssue struct {
    Code, Severity, Component, SourceRef, TargetRef, Relation, Field, Message string
}
type ProjectTemplateValidationError struct { Issues []ProjectTemplateIssue }
func (e ProjectTemplateValidationError) Error() string
func (e ProjectTemplateValidationError) PrimaryCode() string

func (s *Service) ListProjectTemplates(status, q string, limit, offset int) (ProjectTemplatePage, error)
func (s *Service) ListProjectTemplatesForInstantiation(TemplateInstantiationListInput) (ProjectTemplatePage, error)
func (s *Service) ProjectTemplateInfo(templateRef string, snapshotID *string) (ProjectTemplateView, error)
func (s *Service) ModifyProjectTemplate(templateRef string, input ModifyTemplateInput) (ProjectTemplateView, error)
func (s *Service) ArchiveProjectTemplate(templateRef string) (ProjectTemplateView, error)
func (s *Service) ReactivateProjectTemplate(templateRef string) (ProjectTemplateView, error)
```

```go
// internal/app/project_template_capture.go
type CaptureSelection struct {
    ConfigKeys []string `json:"config_keys"`
    TaskRefs []string `json:"task_refs"`
    SeriesRefs []string `json:"series_refs"`
    AutomationRuleIDs []string `json:"automation_rule_ids"`
}
type CaptureSelectionPresence struct { ConfigKeys, TaskRefs, SeriesRefs, AutomationRuleIDs bool }
type CaptureInput struct {
    SourceProjectRef, AnchorDate, ExpectedSourceHash string
    Selection CaptureSelection
    SelectionPresence CaptureSelectionPresence
    Resolution CaptureResolution
}
type CreateTemplateInput struct { Key, Name, Description string; Capture CaptureInput }
type CapturePreview struct {
    NormalizedSelection CaptureSelection
    SourceHash string
    Counts ComponentCounts
    DatePreviews []ProjectTemplateDatePreview
    Issues []ProjectTemplateIssue
    Warnings []ProjectTemplateIssue
}

func (s *Service) PreviewProjectTemplateCapture(CaptureInput) (CapturePreview, error)
func (s *Service) CreateProjectTemplate(CreateTemplateInput) (ProjectTemplateView, error)
func (s *Service) CreateProjectTemplateSnapshot(templateRef string, input CaptureInput) (ProjectTemplateView, error)
```

```go
// internal/app/project_template_instantiate.go
type InstantiateInput struct {
    SnapshotID, ExpectedHash, ProjectSlug, ProjectName, StartDate string
    Description *string
    SecretInputs map[string]string
    AssigneeReplacements map[string]*string
}
type CurrentSnapshotInstantiateInput = InstantiateInput
type InstantiatePreview struct {
    Template ProjectTemplateSummaryView
    Snapshot ProjectTemplateSnapshotSummaryView
    Project ProjectTemplateProjectPreview
    Counts ComponentCounts
    SecretResolutions []SecretResolutionView
    AssigneeIssues []AssigneeIssueView
    Issues []ProjectTemplateIssue
    Warnings []ProjectTemplateIssue
}
type SecretResolutionView struct { Key, ResolvedFrom string }
type AssigneeIssueView struct { User task.UserInfo; AffectedRefs []string; Resolution string }
type ProjectTemplateProjectPreview struct { Slug, Name, Description, StartDate string }
type InstantiateResult struct { Project ProjectView; Counts ComponentCounts }

func (s *Service) PreviewProjectTemplateInstantiation(templateRef string, input InstantiateInput) (InstantiatePreview, error)
func (s *Service) InstantiateProjectTemplate(templateRef string, input InstantiateInput) (InstantiateResult, error)
func (s *Service) InstantiateCurrentProjectTemplate(templateRef string, input CurrentSnapshotInstantiateInput) (InstantiateResult, error)
```

---

### Task 1: 建立 V1 Snapshot 领域契约、strict codec、hash、日期与引用纯函数

**Files:**
- Create: `internal/projecttemplate/model.go`
- Create: `internal/projecttemplate/codec.go`
- Create: `internal/projecttemplate/validate.go`
- Create: `internal/projecttemplate/date.go`
- Create: `internal/projecttemplate/reference.go`
- Create: `internal/projecttemplate/model_test.go`
- Create: `internal/projecttemplate/codec_test.go`
- Create: `internal/projecttemplate/date_test.go`
- Create: `internal/projecttemplate/reference_test.go`

**Interfaces:**
- Consumes: spec §9、§11、§12；现有 Goldmark 依赖；不依赖 App、Storage、HTTP、Cobra 或 GORM。
- Produces: “跨任务固定接口”中的 `projecttemplate` 类型和函数，供 Capture、Instantiate 和 Storage decoding 使用。

- [x] **Step 1: 写 V1 round-trip、严格拒绝和 canonical hash 失败测试**

```go
func TestCodecV1StrictAndStable(t *testing.T) {
    a := fixtureSnapshotV1()
    b := fixtureSnapshotV1()
    b.Tasks[0].Tags = []string{"ops", "api", "ops"}
    a.Tasks[0].Tags = []string{"api", "ops"}
    ja, ha, err := EncodeV1(a, DefaultLimits)
    if err != nil { t.Fatal(err) }
    jb, hb, err := EncodeV1(b, DefaultLimits)
    if err != nil { t.Fatal(err) }
    if string(ja) != string(jb) || ha != hb { t.Fatalf("canonical mismatch\na=%s\nb=%s", ja, jb) }
    got, err := Decode(ja, DefaultLimits)
    if err != nil || got.Schema != SnapshotSchemaV1 { t.Fatalf("got=%#v err=%v", got, err) }
}

func TestDecodeRejectsUnknownSchemaFieldAndTrailingJSON(t *testing.T) {
    cases := []string{
        `{"schema":"xuanchu.project-template-snapshot/v2"}`,
        `{"schema":"xuanchu.project-template-snapshot/v1","anchor_date":"2026-07-20","project":{"description":""},"configs":[],"tasks":[],"series":[],"automations":[],"extra":1}`,
        `{"schema":"xuanchu.project-template-snapshot/v1"}{}`,
    }
    for _, raw := range cases {
        if _, err := Decode([]byte(raw), DefaultLimits); err == nil { t.Fatalf("accepted %s", raw) }
    }
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./internal/projecttemplate -run 'Codec|Decode' -count=1`

Expected: FAIL，package 或 `EncodeV1` / `Decode` 尚不存在。

- [x] **Step 3: 实现完整 V1 struct、normalize、strict schema dispatch 与 SHA-256**

`model.go` 必须逐字段定义 spec §9 的 Project/Config/Task/Series/Automation blueprint；Automation 的 trigger/condition/action/context 使用 snapshot-specific struct，不允许 `json.RawMessage` 或 `map[string]any`。`EncodeV1` 先复制并 normalize：trim 标量；tags、assignee IDs、depends refs、config keys 稳定排序去重；tasks/series/automation 保持 Capture 分配后的规范顺序；空集合编码为 `[]` 而不是 `null`；再 `json.Marshal` 并校验 8 MiB。`Decode` 先读取仅含 `schema` 的 header，再以 `json.Decoder.DisallowUnknownFields()` 解码，第二次 `Decode` 必须得到 `io.EOF`；缺失或显式 `null` 的顶层四类数组返回 `project_template_snapshot_invalid`。unknown schema 返回 `project_template_snapshot_schema_unsupported`，其它 pure package error 通过 `projecttemplate.Error` 保留 spec 稳定 code，App 再映射为 `RuntimeError`。

```go
func EncodeV1(in SnapshotV1, limits Limits) ([]byte, string, error) {
    normalized, err := normalizeV1(in)
    if err != nil { return nil, "", err }
    if err := ValidateSnapshot(normalized, limits); err != nil { return nil, "", err }
    raw, err := json.Marshal(normalized)
    if err != nil { return nil, "", err }
    if len(raw) > limits.MaxJSONBytes { return nil, "", invalid("snapshot exceeds 8 MiB") }
    sum := sha256.Sum256(raw)
    return raw, hex.EncodeToString(sum[:]), nil
}
```

- [x] **Step 4: 写并实现日期、local ref、dependency cycle、secret invariant 与 Markdown 改写测试**

```go
func TestRelativeTimeKeepsWallClockAcrossDST(t *testing.T) {
    loc, _ := time.LoadLocation("America/New_York")
    source := time.Date(2026, 3, 9, 9, 30, 0, 0, loc).Unix()
    rel, err := ToRelativeLocalTime(source, "2026-03-07", loc)
    if err != nil { t.Fatal(err) }
    got, err := FromRelativeLocalTime(rel, "2026-10-31", loc)
    if err != nil { t.Fatal(err) }
    if time.Unix(got, 0).In(loc).Format("2006-01-02 15:04:05") != "2026-11-02 09:30:00" { t.Fatal(got) }
}

func TestValidateSnapshotRejectsMissingCycleSecretAndAttachment(t *testing.T) {
    // 表驱动构造 missing depends、task-1/task-2 cycle、literal secret value、
    // ref://attachment UUID，分别断言稳定的 project_template_* code。
}
```

`reference.go` 必须用 Goldmark Link/Image AST 识别 Markdown destination；Capture 将已选 UUID 改写为 `ref://task/task-N`，未选择 target 返回 issue，attachment 返回 blocking issue；Instantiate 只把合法 `task-N` 改回预分配 UUID。普通 HTTP 链接和 code block 原样保留。

- [x] **Step 5: 运行纯函数测试并提交**

Run: `go test ./internal/projecttemplate -count=1`

Expected: PASS，覆盖 unknown schema/field、trailing JSON、hash 稳定、count/size、DST、missing/cycle ref、secret literal 和 attachment。

```bash
git add internal/projecttemplate
git commit -m "feat: 定义项目模板快照契约"
```

### Task 2: 持久化 Template/Snapshot、迁移、隔离、唯一性与并发版本

**Files:**
- Modify: `internal/storage/models.go`
- Modify: `internal/storage/migrate_sqlite.go`
- Modify: `internal/storage/migrate_postgres.go`
- Create: `internal/storage/project_template_repo.go`
- Create: `internal/storage/project_template_repo_test.go`
- Modify: `internal/storage/db_test.go`
- Modify: `tests/integration/postgres_e2e_test.go`

**Interfaces:**
- Consumes: Task 1 只用于 repository 测试生成合法 JSON；Storage 生产代码不解释 Snapshot 内容。
- Produces: 固定 repository 接口、两张表和 dialect-neutral version allocation，供 App transaction 使用。

- [x] **Step 1: 写 SQLite migration、workspace 隔离、不可变追加和唯一约束失败测试**

```go
func TestProjectTemplateRepositoryAppendIsImmutableAndScoped(t *testing.T) {
    store := openTestStore(t)
    repo := NewProjectTemplateRepository(store.DB())
    tpl := templateRow("tpl-1", "ws-1", "launch")
    if err := repo.Create(tpl); err != nil { t.Fatal(err) }
    var first, second ProjectTemplateSnapshot
    err := store.Transaction(func(tx *Store) error {
        txRepo := NewProjectTemplateRepository(tx.DB())
        var appendErr error
        first, appendErr = txRepo.AppendSnapshotLocked("ws-1", tpl.ID, snapshotRow("snap-1", "hash-1"))
        return appendErr
    })
    if err != nil || first.Version != 1 { t.Fatalf("first=%#v err=%v", first, err) }
    err = store.Transaction(func(tx *Store) error {
        txRepo := NewProjectTemplateRepository(tx.DB())
        var appendErr error
        second, appendErr = txRepo.AppendSnapshotLocked("ws-1", tpl.ID, snapshotRow("snap-2", "hash-2"))
        return appendErr
    })
    if err != nil || second.Version != 2 { t.Fatalf("second=%#v err=%v", second, err) }
    got, _ := repo.GetSnapshot("ws-1", tpl.ID, first.ID)
    if got.SnapshotJSON != first.SnapshotJSON { t.Fatal("old snapshot mutated") }
    if _, err := repo.GetByRef("ws-2", tpl.ID); !errors.Is(err, ErrNotFound) { t.Fatalf("err=%v", err) }
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./internal/storage -run 'ProjectTemplate|Migration.*Template' -count=1`

Expected: FAIL，model/repository/table 尚不存在。

- [x] **Step 3: 增加两张 model、AutoMigrate 和 repository**

```go
type ProjectTemplate struct {
    ID, WorkspaceID, Key, Name, Description, Status string
    CurrentSnapshotID *string
    CreatedByActorType string
    CreatedByUserID, CreatedByTokenID, CreatedByTokenName, CreatedByTokenPrefix *string
    CreatedAt, ModifiedAt int64
    ArchivedAt *int64
}

type ProjectTemplateSnapshot struct {
    ID, WorkspaceID, TemplateID string
    Version int64
    SourceProjectID, SnapshotJSON, SnapshotHash, CreatedByActorType string
    CreatedByUserID, CreatedByTokenID, CreatedByTokenName, CreatedByTokenPrefix *string
    CreatedAt int64
}
```

GORM tags 必须建立 `(workspace_id,key)`、`(template_id,version)`、`(template_id,snapshot_hash)` 唯一索引；migration 显式创建 Template→current Snapshot 与 Snapshot→Template 的 `ON DELETE RESTRICT` 外键并在 SQLite/PostgreSQL 测试中检查。`AppendSnapshotLocked` 只允许在 `Store.Transaction` 内调用：PostgreSQL 对 Template row 使用 `clause.Locking{Strength:"UPDATE"}`，SQLite 依赖外层写事务；读取当前最大 version 后插入并更新 `current_snapshot_id/modified_at`。唯一冲突映射为 package-level `ErrProjectTemplateKeyConflict`、`ErrProjectTemplateVersionConflict`、`ErrProjectTemplateHashConflict`，不得向 App 泄漏方言错误文本。Repository 不提供 Snapshot update/delete。

- [x] **Step 4: 写并运行双数据库并发验证**

SQLite 使用两个 goroutine/独立连接并发追加，断言最终 version 为 1、2 且 current 指向 v2；PostgreSQL E2E 在临时数据库中追加两个 Snapshot，断言 `snapshot_json` 列为 text、版本唯一、workspace 隔离。PostgreSQL 环境变量未设置时沿用现有 skip 规则。

Run: `go test ./internal/storage -run ProjectTemplate -count=1`

Expected: PASS；SQLite 并发测试没有 duplicate version 或 database-specific error 泄漏。

- [x] **Step 5: 运行 Storage 回归并提交**

Run: `go test ./internal/storage -count=1`

Expected: PASS。

```bash
git add internal/storage/models.go internal/storage/migrate_sqlite.go internal/storage/migrate_postgres.go internal/storage/project_template_repo.go internal/storage/project_template_repo_test.go internal/storage/db_test.go tests/integration/postgres_e2e_test.go
git commit -m "feat: 持久化项目模板与快照"
```

### Task 3: 接入 Service 并完成 Template 元数据、权限、生命周期和审计

**Files:**
- Modify: `internal/app/service.go`
- Create: `internal/app/project_template.go`
- Create: `internal/app/project_template_test.go`
- Modify: `internal/app/audit.go`

**Interfaces:**
- Consumes: Task 1 decode/hash、Task 2 repository、现有 `Require`、`resolveUserInfos`、actor columns 和 audit transaction。
- Produces: metadata list/info/modify/archive/reactivate、受限 instantiation list；Task 5 为 create/append 填充 Snapshot，Task 6/7 复用 template resolution。

- [x] **Step 1: 写权限、project scope、metadata redaction 和生命周期失败测试**

```go
func TestProjectTemplateMetadataLifecycleAndScope(t *testing.T) {
    owner := templateServiceFixture(t)
    seedTemplateWithSnapshot(t, owner, "launch", fixtureSnapshotV1())
    page, err := owner.ListProjectTemplates("all", "launch", 50, 0)
    if err != nil || len(page.Items) != 1 { t.Fatalf("page=%#v err=%v", page, err) }
    if strings.Contains(mustJSON(page), "snapshot_json") { t.Fatal("raw snapshot leaked") }
    archived, err := owner.ArchiveProjectTemplate("launch")
    if err != nil || archived.Template.Status != "archived" { t.Fatalf("got=%#v err=%v", archived, err) }
    if _, err := owner.ReactivateProjectTemplate("launch"); err != nil { t.Fatal(err) }
    projectScoped := projectScopedTemplateService(t, owner)
    if _, err := projectScoped.ListProjectTemplates("active", "", 50, 0); errorCode(err) != "project_scope_denied" { t.Fatalf("err=%v", err) }
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./internal/app -run ProjectTemplateMetadata -count=1`

Expected: FAIL，Service 尚未装配 repository 和用例。

- [x] **Step 3: 装配 repository、解析 ref、权限矩阵和 typed/redacted view**

在 `Service` 加 `projectTemplateRepo *storage.ProjectTemplateRepository`，同时修改 `NewService` 和 `withStore`。`resolveProjectTemplate(ref)` 只按当前 workspace 的 UUID/key 查找。所有 Template 用例先拒绝 `hasProjectScope()`；metadata list/info 基础要求 `PermissionProjectRead`，modify/archive/reactivate 要 `PermissionProjectManage`；detail 根据解码后的 component counts 再要求 `PermissionTaskRead`、`PermissionProjectConfigRead`、`PermissionHookRead`。actor view 必须通过现有批量 user resolution 构造 `task.ActorInfo`。

- [x] **Step 4: 实现 metadata 修改、archive/reactivate 和受限 list**

`ModifyProjectTemplate` 只接受 `Name *string`、`Description *string`，拒绝 key。普通 list 以 `modified_at DESC,id ASC` 稳定分页；`ListProjectTemplatesForInstantiation` 固定 active，只返回 current ID/version/hash、counts、required secret keys，不返回旧 versions 或 Snapshot detail。归档写 `project_template.archive` audit；重新激活复用 `project_template.modify` 并在 payload 写 `status_before/status_after`；名称/说明修改写 `project_template.modify`。

```go
func (s *Service) rejectProjectTemplateProjectScope() error {
    if s.hasProjectScope() {
        return RuntimeError{Code: authz.CodeProjectScopeDenied, Message: "project-scoped token cannot access project templates"}
    }
    return nil
}
```

- [x] **Step 5: 运行测试并提交**

Run: `go test ./internal/app -run 'ProjectTemplateMetadata|ProjectTemplatePermission|ProjectTemplateAudit' -count=1`

Expected: PASS，含 viewer/owner/tenant capability、跨 workspace、project-scoped token、archive/reactivate 和 raw JSON redaction。

```bash
git add internal/app/service.go internal/app/project_template.go internal/app/project_template_test.go internal/app/audit.go
git commit -m "feat: 管理项目模板生命周期"
```

### Task 4: 实现四类候选的服务端筛选、count/pagination 和显式选择展开

**Files:**
- Modify: `internal/storage/task_repo.go`
- Modify: `internal/storage/task_repo_test.go`
- Modify: `internal/storage/task_series_repo.go`
- Modify: `internal/storage/task_series_repo_test.go`
- Modify: `internal/storage/config_repo.go`
- Modify: `internal/storage/config_repo_test.go`
- Modify: `internal/storage/project_automation_rule_repo.go`
- Modify: `internal/storage/project_automation_repo_test.go`
- Create: `internal/app/project_template_candidates.go`
- Create: `internal/app/project_template_candidates_test.go`

**Interfaces:**
- Consumes: 现有 task query AST/compiler、TaskSeries filters、ConfigDefinition、automation rule view 和 Task 3 permission helpers。
- Produces: 四个 candidate page 方法和 `ResolveProjectTemplateCandidateSelection`，供 HTTP 和 Capture wizard 使用。

- [x] **Step 1: 写大数据集、强制 scope、稳定分页和 count 失败测试**

```go
func TestTaskTemplateCandidatesForceNormalProjectScopeAndDatabasePage(t *testing.T) {
    svc, source, other := candidateFixture(t, 240)
    seedOccurrenceDeletedAndOtherProjectTasks(t, svc, source, other)
    page, err := svc.ListProjectTemplateTaskCandidates(TaskCandidateListInput{
        SourceProjectRef: source.Slug, Q: "上线", Status: "all", Sort: "entry", Limit: 50, Offset: 50,
    })
    if err != nil { t.Fatal(err) }
    if page.Total != 120 || len(page.Items) != 50 { t.Fatalf("page=%#v", page) }
    for _, item := range page.Items {
        if item.ProjectID != source.ID || item.SeriesID != nil || item.Status == "deleted" { t.Fatalf("leak=%#v", item) }
    }
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./internal/storage ./internal/app -run 'TemplateCandidate|CandidateSelection' -count=1`

Expected: FAIL，分页 repository API 和 App candidate 用例不存在。

- [x] **Step 3: 在 Storage 增加四类 bounded query**

Task repository 的 base query 必须先固定 `workspace_id=? AND project_id=? AND series_id IS NULL AND status<>deleted`，再应用现有 AST 和 q/status/priority/assignee/tags/date filters；同一个 query clone 做 `Count` 和 `Limit/Offset`，排序最后总是加 `uuid ASC` tie-break。Series 将现有 `ListCandidates` 改为共享 query builder，并新增 `ListCandidatePage(opts, limit, offset)`，不得 App 内存切页。Config 只查 `scope=project AND scope_id=source project` 并 join/批量加载 definition；Automation 按 workspace/project/name/description/enabled/trigger type 筛选并以 `created_at,id` 稳定排序。

- [x] **Step 4: 实现 App candidate page 与“全部匹配”展开**

```go
type CandidateSelectionQuery struct {
    SourceProjectRef string
    Kind string // task|series|config|automation
    Task *TaskCandidateListInput
    Series *SeriesCandidateListInput
    Config *ConfigCandidateListInput
    Automation *AutomationCandidateListInput
}
type ResolvedCandidateSelection struct { Refs []string; Total int; SourceHash string }
```

四类 list 默认 50、最大 100；批量解析 assignee/user，候选只返回摘要、warning count 和 stable source ref。Resolve 复用同一 query builder，将 limit 设为对应 Snapshot 上限 + 1；超过上限返回 `project_template_candidate_limit_exceeded`，否则返回按规范顺序的显式 UUID/ID/key 与 SHA-256 selection source hash。Task `Query` 通过 `query.ParseQuery` 和现有 project predicate resolution，调用方表达式不能覆盖强制 scope。

- [x] **Step 5: 运行测试并提交**

Run: `go test ./internal/storage ./internal/app -run 'TemplateCandidate|CandidateSelection' -count=1`

Expected: PASS，包含四类 filter/count/page、跨页无重复、query 注入 scope、全��匹配上限、无 N+1 和显式 refs。

```bash
git add internal/storage/task_repo.go internal/storage/task_repo_test.go internal/storage/task_series_repo.go internal/storage/task_series_repo_test.go internal/storage/config_repo.go internal/storage/config_repo_test.go internal/storage/project_automation_rule_repo.go internal/storage/project_automation_repo_test.go internal/app/project_template_candidates.go internal/app/project_template_candidates_test.go
git commit -m "feat: 查询项目模板候选内容"
```

### Task 5: 实现 Capture Preview、resolution、source hash 和不可变 Snapshot 追加

**Files:**
- Create: `internal/app/project_template_capture.go`
- Create: `internal/app/project_template_reference.go`
- Create: `internal/app/project_template_capture_test.go`
- Modify: `internal/app/project_template.go`
- Modify: `internal/storage/task_series_repo.go`
- Modify: `internal/storage/config_repo.go`
- Modify: `internal/storage/project_automation_rule_repo.go`

**Interfaces:**
- Consumes: Tasks 1–4；Storage 批量读取显式 refs；现有 UDA/config/automation validation；workspace clock/location。
- Produces: Capture preview/create/append，用于 HTTP/Web；写出的 canonical JSON 供 Instantiate 使用。

- [x] **Step 1: 写精确选择、缺失依赖、attachment、secret 和 source drift 失败测试**

```go
func TestCapturePreviewAndCreateUseOnlyExplicitSelection(t *testing.T) {
    svc, project := captureFixture(t)
    keep, omitted, parent := seedCaptureTasks(t, svc, project)
    input := completeCaptureInput(project.Slug, "2026-07-20", CaptureSelection{TaskRefs: []string{keep.UUID, parent.UUID}})
    preview, err := svc.PreviewProjectTemplateCapture(input)
    if err != nil { t.Fatal(err) }
    if preview.Counts.Tasks != 2 || strings.Contains(mustJSON(preview), omitted.UUID) { t.Fatalf("preview=%#v", preview) }
    input.ExpectedSourceHash = preview.SourceHash
    got, err := svc.CreateProjectTemplate(CreateTemplateInput{Key:"launch", Name:"启动流程", Capture:input})
    if err != nil { t.Fatal(err) }
    if got.Template.CurrentSnapshot.Counts.Tasks != 2 { t.Fatalf("got=%#v", got) }
}

func TestCaptureRejectsChangedSourceAndNeverPersistsSecret(t *testing.T) {
    // Preview 后修改选中 task，Capture 断言 project_template_source_changed；
    // secret config 的明文在 snapshot_json、audit JSON、view JSON 中均不存在。
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./internal/app -run 'ProjectTemplateCapture|CapturePreview' -count=1`

Expected: FAIL，Capture 用例尚不存在。

- [x] **Step 3: 批量读取源、验证 presence/权限并计算 source hash**

四个 `SelectionPresence` 必须全为 true；每个数组按集合处理并按 source project_seq/ID、key、created_at/ID 规范排序。批量加载后逐项验证属于 source Project、task 非 deleted/occurrence、Series 合法、config 是 project 显式 row、automation 属于 source。source fingerprint 必须覆盖 source Project description、选中完整资源、相关 Config/UDA definitions 和 assignee membership 状态；只序列化服务端读取的 normalized source state，不含 UI filter 或 resolution。Preview 和 Capture 都重算，hash 不同返回 `project_template_source_changed`。

- [x] **Step 4: 映射 blueprint、local refs、日期和结构化 resolution**

```go
type TaskRelationResolution struct { SourceTaskRef, Relation, TargetTaskRef string }
type ContentRefResolution struct { SourceKind, SourceRef, TargetTaskRef string }
type TaskDateOverride struct { SourceTaskRef, Field string; Value *projecttemplate.RelativeLocalTimeV1 }
type SeriesScheduleOverride struct {
    SourceSeriesRef string
    FirstDue projecttemplate.RelativeLocalTimeV1
    Until *projecttemplate.RelativeLocalTimeV1
    ClearUntil bool
}
type CaptureResolution struct {
    DropParentTaskRefs []string
    DropDepends []TaskRelationResolution
    DropContentTaskRefs []ContentRefResolution
    TaskDateOverrides []TaskDateOverride
    SeriesScheduleOverrides []SeriesScheduleOverride
}
```

服务端只能接受与当前 Preview blocking issue 精确匹配的 drop/override；未提示的关系不能删除。`ContentRefResolution.SourceKind` 只允许 `task|series`，因此 Task 和 Series description 的 task ref 都能被精确处理。普通 task 全部变成开放 blueprint，不保存 status/start/annotation/occurrence/attachment。active Series 用现有 recurrence engine 取 anchor date 当天或之后的第一个合法槽位；源 until 早于新 first_due 时清空并 warning。ended/stopped 无未来槽位时以 anchor date D+0 为 first_due、清空 until，并要求提交一个与默认值相同或经用户修改的 `SeriesScheduleOverride` 才解除 blocking confirmation。选择 automation 后，服务端从归一化 action 计算 provider config 闭包，将来源项目中存在的显式 config 自动加入选择；secret config 写 `{mode:"secret_copy",secret_ciphertext:"enc:v1:..."}`，ciphertext 只存在原始 Snapshot，历史 `secret_input` 保持可读。automation 显式映射 typed struct且忽略 enabled/delivery。正文 task ref 改 local ref，user ref 保留同 workspace user ID，attachment 阻断。

- [x] **Step 5: 同事务创建首个 Snapshot/追加版本、审计并提交**

创建前按 `^[a-z][a-z0-9-]{2,31}$` 校验 key，trim name/description 并检查 workspace key conflict。Template create + v1 Snapshot + current pointer + `project_template.create`/`project_template.snapshot.create` audit 必须同一 `withAuditEntries` transaction；append 时 archived 拒绝。若 hash 等于 current Snapshot，按幂等重试返回 current view且不新增 version/audit；若与历史非 current Snapshot 重复，返回 `project_template_snapshot_invalid` 且不切换 current。成功追加后 current 原子切换。audit 只含 template/snapshot/source IDs、version/hash/counts。

Run: `go test ./internal/app -run 'ProjectTemplateCapture|CapturePreview|ProjectTemplateSnapshot' -count=1`

Expected: PASS，覆盖空模板、四类任意组合、completed reset、Series history 排除、resolution、date warning、source drift、secret redaction 和 audit。

```bash
git add internal/app/project_template_capture.go internal/app/project_template_reference.go internal/app/project_template_capture_test.go internal/app/project_template.go internal/storage/task_series_repo.go internal/storage/config_repo.go internal/storage/project_automation_rule_repo.go
git commit -m "feat: 保存项目模板快照"
```

### Task 6: 实现 Instantiate Preview 的权限、成员、Config/UDA、Automation、secret 和日期预检

**Files:**
- Create: `internal/app/project_template_instantiate.go`
- Create: `internal/app/project_template_instantiate_test.go`
- Modify: `internal/app/project_template.go`
- Modify: `internal/app/config_effective.go`
- Modify: `internal/app/project_automation_preview.go`

**Interfaces:**
- Consumes: Task 1 Decode/date/reference、Task 3 template resolution、Task 5 Snapshot；现有 project/config/UDA/automation validators。
- Produces: deterministic `InstantiatePreview` 和 transaction-ready `instantiatePlan`，Task 7 只能执行这个 plan。

- [x] **Step 1: 写 hash、archived、slug、member、secret、schema 和日期失败测试**

```go
func TestInstantiatePreviewReturnsIssuesWithoutSecrets(t *testing.T) {
    svc := instantiateFixture(t)
    seedTemplateRequiringUnavailableMemberAndSecret(t, svc)
    preview, err := svc.PreviewProjectTemplateInstantiation("launch", InstantiateInput{
        SnapshotID: currentSnapshotID(t, svc, "launch"), ExpectedHash: currentHash(t, svc, "launch"),
        ProjectSlug: "newproj", ProjectName: "新项目", StartDate: "2026-08-01",
    })
    if err != nil { t.Fatal(err) }
    if !hasIssue(preview.Issues, "project_template_member_unavailable") || !hasIssue(preview.Issues, "project_template_secret_required") { t.Fatalf("preview=%#v", preview) }
    raw := mustJSON(preview)
    if strings.Contains(raw, "sk-secret") || strings.Contains(raw, `"value"`) { t.Fatalf("secret leaked: %s", raw) }
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./internal/app -run ProjectTemplateInstantiatePreview -count=1`

Expected: FAIL，preview/plan builder 尚不存在。

- [x] **Step 3: 实现确定 Snapshot 解析、current 限制和完整权限矩阵**

`PreviewProjectTemplateInstantiation` 允许 HTTP/Web 指定属于 Template 的历史 Snapshot；空 ID 解析 current 并在响应固定 ID/hash。`InstantiateCurrentProjectTemplate` 额外要求输入 ID 等于 current ID。两者都验证 64 位小写 hex expected hash 并 constant-time compare。基础权限 `PermissionProjectManage`；按内容增加 `PermissionTaskWrite`、`PermissionProjectConfigWrite`、`PermissionHookWrite`；project-scoped token 统一拒绝。

- [x] **Step 4: 构造 transaction-ready plan 并执行所有兼容性检查**

```go
type instantiatePlan struct {
    Template storage.ProjectTemplate
    Snapshot storage.ProjectTemplateSnapshot
    ProjectSlug, ProjectName, Description string
    ConfigValues map[string]string
    TaskIDs, SeriesIDs map[string]string
    Tasks []plannedTask
    Series []plannedSeries
    Automations []plannedAutomation
    Preview InstantiatePreview
}
```

先校验 Template active、Snapshot workspace/template、slug/name、slug 未占用；再验证 active membership 和只允许 issue 对应的 assignee replacement（null=移除）；UDA definition/value；project config scope/type/secret 解析来源；automation provider/config keys/allowed hosts；所有恢复日期；local ref 图。secret resolution 只返回 `input|workspace|default|missing`。历史 literal key 现已变 secret 时丢弃 Snapshot literal，强制按 secret 解析。由 input 解析的 secret 会写入新项目显式 config；由 workspace/default 解析的 secret 只保留继承，不复制成 project row。`Counts.Configs` 表示成功应用的 blueprint 数，不等同于实际新增 config row 数。任何 blocking issue 存在时 Preview 可返回 200 typed issues；最终 Instantiate 返回 `ProjectTemplateValidationError`，保留全部 typed issues，HTTP/MCP 用 `PrimaryCode()` 映射稳定主错误码而不丢失结构化问题清单。

- [x] **Step 5: 运行 Preview 测试并提交**

Run: `go test ./internal/app -run 'ProjectTemplateInstantiatePreview|ProjectTemplatePermission' -count=1`

Expected: PASS，含 old/current Snapshot、hash mismatch、archived、slug conflict、member replacement、UDA/config drift、automation invalid、secret inheritance、DST/out-of-range 和 ref cycle。

```bash
git add internal/app/project_template_instantiate.go internal/app/project_template_instantiate_test.go internal/app/project_template.go internal/app/config_effective.go internal/app/project_automation_preview.go
git commit -m "feat: 预检项目模板实例化"
```

### Task 7: 原子实例化 Project、Config、Series、Task、关系、Link 与 disabled Automation

**Files:**
- Modify: `internal/app/project_template_instantiate.go`
- Modify: `internal/app/project_template_instantiate_test.go`
- Modify: `internal/app/service.go`
- Modify: `internal/app/task_series.go`
- Modify: `internal/app/task_series_test.go`
- Modify: `internal/app/project_automation.go`
- Modify: `internal/app/project_automation_test.go`
- Modify: `internal/app/hook_event.go`

**Interfaces:**
- Consumes: Task 6 `instantiatePlan`；现有 `addProjectLocked`、task config validators、audit/event pipeline。
- Produces: `InstantiateProjectTemplate`/`InstantiateCurrentProjectTemplate` 的原子实现和来源 metadata。

- [x] **Step 1: 写成功身份映射、Series 无 occurrence、disabled automation 和回滚失败测试**

```go
func TestInstantiateCreatesFreshGraphWithoutHistory(t *testing.T) {
    svc := instantiateFixture(t)
    seedFullTemplate(t, svc)
    input := validInstantiateInput(t, svc, "launch", "newproj")
    got, err := svc.InstantiateCurrentProjectTemplate("launch", input)
    if err != nil { t.Fatal(err) }
    if got.Project.Status != "planning" || got.Counts != (ComponentCounts{Configs:2, Tasks:3, Series:1, Automations:1}) { t.Fatalf("got=%#v", got) }
    assertFreshTaskIDsAndMappedRelations(t, svc, got.Project.ID)
    assertSingleRuleVersionAndNoOccurrence(t, svc, got.Project.ID)
    assertAllAutomationsDisabledAndNoDeliveries(t, svc, got.Project.ID)
}

func TestInstantiateRollbackLeavesNothing(t *testing.T) {
    svc := instantiateFixtureWithInjectedFailure(t, "automation-create")
    input := validInstantiateInput(t, svc, "launch", "rollback")
    if _, err := svc.InstantiateCurrentProjectTemplate("launch", input); err == nil { t.Fatal("expected failure") }
    assertNoProjectOrChildren(t, svc, "rollback")
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./internal/app -run 'ProjectTemplateInstantiateCreates|ProjectTemplateInstantiateRollback' -count=1`

Expected: FAIL，transaction executor 尚未实现。

- [x] **Step 3: 提取可复用的 transaction 内创建 helper**

`addLockedWithUUID(input AddInput, presetUUID string)` 复用普通 task validation/seq 分配但允许 Template 预分配 UUID；普通 `addLocked` 传空字符串保持现有行为。Task 模板第一阶段以 nil description/parent/depends 创建，全部 Task 存在后第二阶段写最终 description、parent、depends 和 links，解决前向 `ref://task`。Task created event 在第二阶段完成后从最终 row 构造。

把 Series 创建核心提取为 `addTaskSeriesLocked(input AddTaskSeriesInput, materializeFirst bool)`；公开 Add 传 true，Template 传 false，确保 start date 已到也不在初始化事务物化 occurrence。把 Automation 核心提取为 `addProjectAutomationRuleLocked(project storage.Project, input ProjectAutomationRuleAddInput)`；Template 强制覆盖 `Enabled=false`。

- [x] **Step 4: 在一个 audit/events transaction 中按固定顺序执行 plan**

```go
err := s.withAuditEntriesAndEvents(func(tx *Service) ([]AuditEntry, []HookEvent, error) {
    plan, err := tx.buildInstantiatePlan(templateRef, input, currentOnly)
    if err != nil { return nil, nil, err }
    project, err := tx.addProjectLocked(plan.ProjectSlug, plan.ProjectName, plan.Description)
    if err != nil { return nil, nil, err }
    // config -> preallocated Series/Task -> Series(no occurrence) -> Task shells
    // -> final task refs/relations/links -> disabled automation -> audits/events
    return instantiateAuditEntries(plan, project), instantiateEvents(plan, project), nil
})
```

最终执行前在同一 transaction 再跑完整 plan builder，防止 Preview 后成员/config/current hash 漂移。config 使用当前 validator 并写 project scope；Task/Series seq 按 Snapshot 顺序；所有 links 生成新 ID、created_by=Instantiate actor；aggregate audit `project_template.instantiate` 带 template/snapshot/hash/counts，子资源继续写现有 audit。实际 Project/Task/Series 创建事件 payload 增加 `source_template_id/source_template_snapshot_id/source_template_snapshot_hash`，不新增 hook event type。

- [x] **Step 5: 运行原子性与现有创建路径回归并提交**

Run: `go test ./internal/app -run 'ProjectTemplateInstantiate|TaskSeriesAdd|ProjectAutomationRule' -count=1`

Expected: PASS，普通 Series Add 仍按原行为物化，模板 Series 不物化；故障注入每个阶段均完整回滚；secret 不出现在 audit/events。

```bash
git add internal/app/project_template_instantiate.go internal/app/project_template_instantiate_test.go internal/app/service.go internal/app/task_series.go internal/app/task_series_test.go internal/app/project_automation.go internal/app/project_automation_test.go internal/app/hook_event.go
git commit -m "feat: 原子实例化项目模板"
```

### Task 8: 暴露完整 HTTP 治理 API、OpenAPI 契约和错误状态

**Files:**
- Create: `internal/httpapi/project_templates.go`
- Create: `internal/httpapi/project_templates_test.go`
- Modify: `internal/httpapi/router.go`
- Modify: `internal/httpapi/huma_routes.go`
- Modify: `internal/httpapi/error_status.go`
- Modify: `internal/httpapi/server_test.go`

**Interfaces:**
- Consumes: Tasks 3–7 全部 App 用例。
- Produces: spec §18.1 的 16 个 HTTP operations，供 Web 使用；Remote 只调用其中 list/current instantiate 子集。

- [x] **Step 1: 写 route completeness、required selection arrays、typed issue 和 raw JSON/secret redaction 失败测试**

```go
func TestProjectTemplateCaptureRequiresAllSelectionArrays(t *testing.T) {
    body := `{"key":"launch","name":"启动","capture":{"source_project":"ops","anchor_date":"2026-07-20","selection":{"task_refs":[]}}}`
    rr := requestTemplateAPI(t, http.MethodPost, "/api/v1/project-templates?workspace=local", body)
    if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "project_template_selection_invalid") { t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String()) }
}

func TestProjectTemplateResponsesNeverExposeRawSnapshotOrSecrets(t *testing.T) {
    // GET detail/list/preview/instantiate responses 均断言不含 snapshot_json 和 fixture secret。
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./internal/httpapi -run ProjectTemplate -count=1`

Expected: FAIL，routes 尚未注册。

- [x] **Step 3: 实现 handlers、DTO、presence conversion 和 body limit**

精确注册 spec §18.1 路径。四个 selection 数组的 HTTP DTO 使用 `*[]string`，nil 表示字段缺失并转成 `CaptureSelectionPresence=false`，空 slice 表示明确不选。candidate query 参数与 Task/Series/Config/Automation App 输入一一映射；重复 `ref` 是最多 100 个 stable ref 的精确筛选，并在 Huma/OpenAPI 以 form/explode array 声明；`resolve-selection` body 使用 `kind` 和对应 filter object。所有 template POST/PATCH body 用 `http.MaxBytesReader(..., 9<<20)`；secret input 只传 App，不进入 request/error logging。

- [x] **Step 4: 完成 Huma request/success schema 和错误码 status mapping**

Huma 明确 required selection arrays、date format、UUID/hash pattern、typed preview issues、page metadata 和 `task.JSONUserInfo`/`task.JSONActorInfo`。错误状态：not found=404；key/hash/version/source/archived/concurrency conflict=409；request body/Snapshot JSON 超限=413；candidate selection 上限、invalid schema/ref/config/UDA/date/selection=422；permission/project scope=403。OpenAPI 测试断言 16 个 operations 和 required fields，raw `snapshot_json` 不在 schema。

- [x] **Step 5: 运行 HTTP/OpenAPI 测试并提交**

Run: `go test ./internal/httpapi -run 'ProjectTemplate|OpenAPI|Route' -count=1`

Expected: PASS，含完整治理、历史 Snapshot instantiate、current hash drift、permission、redaction 和 response shape。

```bash
git add internal/httpapi/project_templates.go internal/httpapi/project_templates_test.go internal/httpapi/router.go internal/httpapi/huma_routes.go internal/httpapi/error_status.go internal/httpapi/server_test.go
git commit -m "feat: 暴露项目模板治理接口"
```

### Task 9: 建立 Web typed API、模板库路由、版本与生命周期界面

**Files:**
- Create: `web/src/features/workspace/project-templates/api/project-template-api.ts`
- Create: `web/src/features/workspace/project-templates/api/project-template-api.test.ts`
- Create: `web/src/features/workspace/project-templates/project-template-library-page.tsx`
- Create: `web/src/features/workspace/project-templates/project-template-library-page.test.tsx`
- Create: `web/src/features/workspace/project-templates/workspace-settings-nav.tsx`
- Create: `web/src/routes/workspace/ProjectTemplatesRoute.tsx`
- Modify: `web/src/routes/workspace/SettingsRoute.tsx`
- Modify: `web/src/routes/router.tsx`
- Modify: `web/src/components/AppShell.tsx`
- Modify: `web/src/components/AppShell.test.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

**Interfaces:**
- Consumes: Task 8 HTTP typed contract。
- Produces: `/settings/project-templates` Template Library、query keys 和 mutations；Tasks 10/11 复用 API 与 selected snapshot state。

- [x] **Step 1: 写 API URL/body、route/nav active、loading/error/empty/archive 失败测试**

```ts
it("keeps template API scoped to workspace and never requests raw JSON", async () => {
  await getProjectTemplate("acme", "launch", "snap-1")
  expect(fetchMock).toHaveBeenCalledWith(
    "/api/v1/project-templates/launch?workspace=acme&snapshot_id=snap-1",
    expect.anything()
  )
})

it("distinguishes API error from an empty template library", async () => {
  renderLibrary({ response: 500 })
  expect(await screen.findByText("加载项目模板失败")).toBeTruthy()
  expect(screen.queryByText("还没有项目模板")).toBeNull()
})
```

- [x] **Step 2: 运行测试确认失败**

Run: `pnpm --dir web test -- project-template-api project-template-library AppShell`

Expected: FAIL，API/page/route 尚不存在。

- [x] **Step 3: 实现 typed API 和 query/mutation cache policy**

定义 candidate page、capture/instantiate input/preview、Template summary/detail/version、issue、secret resolution、UserInfo/ActorInfo TS 类型；所有 mutation 使用 `workspaceApiPost/Patch`。query key 固定 `['project-templates', workspaceSlug, status, q, limit, offset]` 和 `['project-template', workspaceSlug, ref, snapshotID]`；create/append/modify/archive/reactivate/instantiate 成功后精确 invalidate templates/projects。

- [x] **Step 4: 实现 Template Library 与导航**

route path 固定 `/settings/project-templates`；`WorkspaceSettingsNav` 在现有 `/settings` ConfigDefinition 页面和 Template Library 顶部提供“配置定义/项目模板”两个入口，AppShell 的 Settings active 判定覆盖子路径。Library 左侧稳定分页/搜索，右侧 current 摘要、来源、版本列表和动作；旧 Snapshot 只读但可启动历史版本 Instantiate wizard；archived 显示 Banner 和“重新激活”，隐藏 Capture/Instantiate。四种状态必须独立：loading skeleton、error retry、empty CTA、no-search-result。

- [x] **Step 5: 运行 Web 测试并提交**

Run: `pnpm --dir web test -- project-template-api project-template-library AppShell`

Expected: PASS，含权限隐藏、version 切换、archive/reactivate、移动布局和中英文 key 完整性。

```bash
git add web/src/features/workspace/project-templates web/src/routes/workspace/ProjectTemplatesRoute.tsx web/src/routes/workspace/SettingsRoute.tsx web/src/routes/router.tsx web/src/components/AppShell.tsx web/src/components/AppShell.test.tsx web/src/locales/zh-CN.ts web/src/locales/en-US.ts
git commit -m "feat: 增加项目模板库"
```

### Task 10: 实现 Capture wizard、服务端筛选、跨页已选清单和冲突处理

**Files:**
- Create: `web/src/features/workspace/project-templates/capture/project-template-capture-wizard.tsx`
- Create: `web/src/features/workspace/project-templates/capture/candidate-picker.tsx`
- Create: `web/src/features/workspace/project-templates/capture/selected-items-sheet.tsx`
- Create: `web/src/features/workspace/project-templates/capture/project-template-capture-wizard.test.tsx`
- Modify: `web/src/features/workspace/project-templates/project-template-library-page.tsx`
- Modify: `web/src/features/workspace/project-workbench/project/project-header-editor.tsx`
- Modify: `web/src/features/workspace/project-workbench/project/project-header-editor.test.tsx`

**Interfaces:**
- Consumes: Task 9 API；candidate refs 是 task UUID/series ID/config key/automation ID。
- Produces: 新 Template 与追加 Snapshot 的同一四步 wizard。

- [x] **Step 1: 写筛选不丢 selection、当前页全选、全部匹配、抽屉移除和 Preview issue 失败测试**

```ts
it("keeps explicit selections while filters and pages change", async () => {
  renderCaptureWizard()
  await selectCandidate("task-a")
  await userEvent.type(screen.getByLabelText("搜索任务"), "上线")
  await gotoPage(2)
  await selectCandidate("task-z")
  expect(screen.getByRole("button", { name: "已选 2 项" })).toBeTruthy()
  await clearCurrentFilteredSelection()
  expect(selectedRefs()).toEqual(["task-a"])
})

it("sends only explicit arrays and preview source hash", async () => {
  // 断言 capture body 四数组都存在，且不包含 filter/query；最终 body 携带 expected_source_hash。
})
```

- [x] **Step 2: 运行测试确认失败**

Run: `pnpm --dir web test -- project-template-capture-wizard project-header-editor`

Expected: FAIL，wizard/components 尚不存在。

- [x] **Step 3: 实现单一 wizard state 和四类 candidate picker**

state 固定为 `Record<'task'|'series'|'config'|'automation', Map<string,CandidateSummary>>`，不放进分页 query cache。新建 Template 首次加载时分别 resolve pending、waiting Task 和 active Series，合并成默认显式选择；若默认匹配超过对应 Snapshot 上限，则该类保持未选择并显示“请先筛选再选择”的限制提示，不截断。completed Task、ended/stopped Series 默认不选。每类 filter/page 独立；header checkbox 文案“选择本页 N 项”，只操作当前响应；“选择全部 N 条匹配结果”调用 resolve endpoint 后合并显式 refs，超限保留现有选择并展示服务端错误。切换 tab/filter/page 不清选择。

- [x] **Step 4: 实现 selected Sheet、Preview resolution 和 source drift 恢复**

桌面右侧 sticky drawer、窄屏/移动端全屏 Sheet 共用同一 selected store；支持搜索、逐项移除、“清除当前筛选结果的选择”和“清除全部已选”。Preview blocking issue 未清零时禁用保存；补选、drop relation/content ref、date/series override 都写结构化 resolution。`project_template_source_changed` 返回选择步骤、清理过期 resolution、保留显式 refs，并以重复 `ref` 精确重取所有已选摘要后要求重新 Preview。

- [x] **Step 5: 接入口、运行测试并提交**

Project Header 更多菜单对具备管理权限者显示“另存为模板”；Template Library “从项目更新快照”先选 source Project 后打开同一 wizard。pending 时禁用关闭/返回/重复提交，非 pending 支持 Esc，Cmd/Ctrl+Enter 只在当前步骤可提交时生效。

Run: `pnpm --dir web test -- project-template-capture-wizard project-header-editor project-template-library`

Expected: PASS，含 1,000+ candidate 模拟分页、selection persistence、secret 行不显示值、issue focus 和移动 Sheet。

```bash
git add web/src/features/workspace/project-templates/capture web/src/features/workspace/project-templates/project-template-library-page.tsx web/src/features/workspace/project-workbench/project/project-header-editor.tsx web/src/features/workspace/project-workbench/project/project-header-editor.test.tsx
git commit -m "feat: 增加项目模板保存向导"
```

### Task 11: 实现从模板创建 wizard、secret/member 处理与成功跳转

**Files:**
- Create: `web/src/features/workspace/project-templates/instantiate/project-template-instantiate-wizard.tsx`
- Create: `web/src/features/workspace/project-templates/instantiate/project-template-instantiate-wizard.test.tsx`
- Modify: `web/src/features/workspace/project-templates/project-template-library-page.tsx`
- Modify: `web/src/features/workspace/project-workbench/projects/projects-list-page.tsx`
- Modify: `web/src/features/workspace/project-workbench/projects/projects-list-page.test.tsx`

**Interfaces:**
- Consumes: Task 9 typed API；Template Library 可传历史 Snapshot，Projects 页面使用 active current Snapshot list。
- Produces: 四步 Instantiate UX 和创建后 project navigation。

- [x] **Step 1: 写模板选择、preview hash、secret 不回显、member replacement 和 double submit 失败测试**

```ts
it("pins preview snapshot and never renders secret values", async () => {
  renderInstantiateWizard({ template: "launch" })
  await fillProject({ slug: "newproj", name: "新项目", startDate: "2026-08-01" })
  await preview()
  expect(screen.getByText("当前无有效值")).toBeTruthy()
  expect(screen.queryByDisplayValue("sk-fixture")).toBeNull()
  await userEvent.type(screen.getByLabelText("agent.provider.api_key"), "sk-input")
  await submit()
  expect(lastInstantiateBody()).toMatchObject({ snapshot_id:"snap-3", expected_snapshot_hash:"hash-3" })
})
```

- [x] **Step 2: 运行测试确认失败**

Run: `pnpm --dir web test -- project-template-instantiate-wizard projects-list-page`

Expected: FAIL，wizard 和“从模板创建”入口不存在。

- [x] **Step 3: 实现模板/项目输入和 Preview 状态机**

Projects 页将“从模板创建”与“新建项目”并列；选择 active Template current Snapshot。Template Library 可固定任一 version。start date 默认 workspace 本地当天；description 预填 Snapshot 默认并允许覆盖。每次改 slug/name/start date/secret/replacement 后标记 preview stale，最终提交前自动重跑 preview 或要求用户再次确认，不能静默换 current Snapshot。

- [x] **Step 4: 实现 secret resolution、member replacement、disabled automation 提示和提交保护**

secret 输入使用 password control，不写 local/session storage、query cache、Toast 或 error message；只显示 resolved source。不可用 assignee 只允许 Preview issue 指定的 replacement/null removal。确认页明确“自动化创建后保持停用”“不会复制附件”。提交 pending 时 modal 不可关闭、按钮 disabled；hash mismatch 返回选择模板步骤并刷新 list，不自动使用新 version。

- [x] **Step 5: 成功跳转、运行测试并提交**

成功后清除 wizard secret state，invalidate projects/templates，Toast 显示准确 counts，跳转 `/workspaces/$workspaceSlug/projects/$projectSlug` Overview。

Run: `pnpm --dir web test -- project-template-instantiate-wizard projects-list-page project-template-library`

Expected: PASS，含 current/history、secret missing、member replace/remove、hash drift、server error 保留非 secret 输入、移动全屏 Sheet 和单次提交。

```bash
git add web/src/features/workspace/project-templates/instantiate web/src/features/workspace/project-templates/project-template-library-page.tsx web/src/features/workspace/project-workbench/projects/projects-list-page.tsx web/src/features/workspace/project-workbench/projects/projects-list-page.test.tsx
git commit -m "feat: 从模板创建项目"
```

### Task 12: 仅为 Remote 与 CLI 增加 list/current instantiate

**Files:**
- Create: `internal/remote/project_template.go`
- Create: `internal/remote/project_template_test.go`
- Modify: `internal/cli/project.go`
- Create: `internal/cli/project_template_test.go`
- Modify: `tests/integration/cli_test.go`
- Modify: `tests/integration/e2e_remote_test.go`

**Interfaces:**
- Consumes: Task 8 list/current instantiate HTTP endpoint、Tasks 3/7 App narrowed use cases。
- Produces: 两个 Remote typed 方法和两个 CLI command；不产生治理方法/命令。

- [x] **Step 1: 写 command tree/Remote surface、JSON/human 输出和 hash drift 失败测试**

```go
func TestProjectTemplateCommandTreeIsNarrow(t *testing.T) {
    root := newRootCommand(testOptions())
    template, _, err := root.Find([]string{"project", "template"})
    if err != nil { t.Fatal(err) }
    names := childCommandNames(template)
    if diff := cmp.Diff([]string{"instantiate", "list"}, names); diff != "" { t.Fatal(diff) }
    for _, forbidden := range []string{"save","info","preview","archive","snapshot","candidate"} {
        if cmd, _, err := template.Find([]string{forbidden}); err == nil && cmd != template { t.Fatalf("unexpected %s", forbidden) }
    }
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./internal/remote ./internal/cli ./tests/integration -run 'ProjectTemplate|TemplateCommand' -count=1`

Expected: FAIL，methods/commands 尚不存在。

- [x] **Step 3: 实现 Remote 的两个 typed 方法**

```go
func (c *Client) ListProjectTemplatesForInstantiation(ctx context.Context, workspace, q string, limit, offset int) (app.ProjectTemplatePage, error)
func (c *Client) InstantiateCurrentProjectTemplate(ctx context.Context, workspace, templateRef string, input app.CurrentSnapshotInstantiateInput) (app.InstantiateResult, error)
```

Remote 只能调用 active list 和 current-only instantiate endpoint；不得新增 candidate/detail/preview/capture/modify/archive/version 方法。DTO 与 local App JSON 字段一致。

- [x] **Step 4: 实现 CLI 命令和安全 input JSON**

`list` 支持 `--q/--limit/--offset`；human 每行输出 key/name/current version/ID/hash/counts/required secret keys，JSON 输出稳定 page。`instantiate <template-ref> <new-project-slug> name:<name>` 必填 `--snapshot`、`--snapshot-hash`、`--start-date`；`--input <path|->` JSON 只允许 description、secret_inputs、assignee_replacements，`-` 读 `cmd.InOrStdin()`，文件/STDIN body 最大 1 MiB。secret 不允许 flag 形式，避免 shell history。local/remote 共用同一 parse 和 render。

- [x] **Step 5: 运行集成测试并提交**

Run: `go test ./internal/remote ./internal/cli ./tests/integration -run 'ProjectTemplate|TemplateCommand' -count=1`

Expected: PASS，stdout/stderr 分离、stdin secret 不回显、local/remote 等价、旧/current mismatch 和不存在额外命令/Remote 方法。

```bash
git add internal/remote/project_template.go internal/remote/project_template_test.go internal/cli/project.go internal/cli/project_template_test.go tests/integration/cli_test.go tests/integration/e2e_remote_test.go
git commit -m "feat: 增加模板列表与实例化命令"
```

### Task 13: 只注册两个 MCP Tool、golden、ToolEnvelope 和 Agent Skill 文档

**Files:**
- Create: `internal/mcpserver/tools_project_template.go`
- Create: `internal/mcpserver/tools_project_template_test.go`
- Modify: `internal/mcpserver/server.go`
- Modify: `internal/mcpserver/schema_test.go`
- Create: `internal/mcpserver/testdata/project_template_list.schema.json`
- Create: `internal/mcpserver/testdata/project_template_instantiate.schema.json`
- Modify: `internal/mcpserver/testdata/list-tools-default.json`
- Modify: `tests/integration/e2e_mcp_test.go`
- Modify: `docs/skills/xuanchu-govern-projects/SKILL.md`
- Modify: `docs/skills/xuanchu-govern-projects/references/workspace-project-tools.md`

**Interfaces:**
- Consumes: Tasks 3/7 narrowed App use cases和现有 `ToolEnvelope`。
- Produces: `project_template_list`、`project_template_instantiate`；不得注册任何治理同义 tool。

- [x] **Step 1: 写 schema、list-tools denylist、workspace required 和 envelope 等价失败测试**

```go
func TestMCPProjectTemplateToolsAreExactlyNarrowSurface(t *testing.T) {
    names := listToolNames(t, NewServer(Options{Version:"test"}))
    for _, want := range []string{"project_template_list", "project_template_instantiate"} {
        if !slices.Contains(names, want) { t.Fatalf("missing %s", want) }
    }
    for _, suffix := range []string{"capture","candidate","get","modify","archive","snapshot","preview"} {
        if slices.Contains(names, "project_template_"+suffix) { t.Fatalf("unexpected %s", suffix) }
    }
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./internal/mcpserver ./tests/integration -run 'MCPProjectTemplate|ProjectTemplateMCP' -count=1`

Expected: FAIL，tools/schema/golden 尚不存在。

- [x] **Step 3: 实现两个输入 schema 与 handlers**

```go
type ProjectTemplateListInput struct {
    Workspace string `json:"workspace" jsonschema:"workspace slug or UUID; required for every template call"`
    Q string `json:"q,omitempty"`
    Limit int `json:"limit,omitempty"`
    Offset int `json:"offset,omitempty"`
}
type ProjectTemplateInstantiateInput struct {
    Workspace string `json:"workspace" jsonschema:"workspace slug or UUID; required for every template call"`
    Template string `json:"template" jsonschema:"template stable key or UUID"`
    SnapshotID string `json:"snapshot_id"`
    ExpectedSnapshotHash string `json:"expected_snapshot_hash"`
    ProjectSlug string `json:"project_slug"`
    ProjectName string `json:"project_name"`
    StartDate string `json:"start_date"`
    Description *string `json:"description,omitempty"`
    SecretInputs map[string]string `json:"secret_inputs,omitempty"`
    AssigneeReplacements map[string]*string `json:"assignee_replacements,omitempty"`
}
type ProjectTemplateToolError struct {
    Code string `json:"code"`
    Message string `json:"message"`
    Issues []app.ProjectTemplateIssue `json:"issues,omitempty"`
}
```

workspace 在 JSON schema required；list 固定 active/current；instantiate 必须带 current Snapshot ID/hash。handler 通过 `serviceForTool` 使用 workspace scope，不传 project scope；success 使用 `successWithEnvelope`。Instantiate 捕获 `ProjectTemplateValidationError` 并返回 `ProjectTemplateToolError`，使 `structuredContent` 保留全部 issues，text 只写主 code/message 且不含 secret。用户/actor JSON 使用统一 converter。

- [x] **Step 4: 更新 schema golden、list-tools snapshot 和 Agent Skill**

Run: `go test ./internal/mcpserver -run 'Schema|ListTools|MCPProjectTemplate' -update -count=1`

Expected: PASS 并只生成两个 schema golden；人工检查 `list-tools-default.json` 没有治理 tool。Skill 文档明确 Agent 流程只能 list → 收集项目字段/secret/replacement → instantiate；版本变化必须重新 list，Web Console 才能 Capture/preview/archive/version governance。

- [x] **Step 5: 运行 MCP E2E 并提交**

Run: `go test ./internal/mcpserver ./tests/integration -run 'MCPProjectTemplate|ProjectTemplateMCP|ListTools' -count=1`

Expected: PASS，`content[0].text` JSON 与 `structuredContent` 等价、current mismatch structured error、workspace 隔离、无多余 tool。

```bash
git add internal/mcpserver/tools_project_template.go internal/mcpserver/tools_project_template_test.go internal/mcpserver/server.go internal/mcpserver/schema_test.go internal/mcpserver/testdata/project_template_list.schema.json internal/mcpserver/testdata/project_template_instantiate.schema.json internal/mcpserver/testdata/list-tools-default.json tests/integration/e2e_mcp_test.go docs/skills/xuanchu-govern-projects/SKILL.md docs/skills/xuanchu-govern-projects/references/workspace-project-tools.md
git commit -m "feat: 暴露项目模板 MCP 工具"
```

### Task 14: 端到端 smoke、双数据库验证、README/ROADMAP 与完整交付检查

**Files:**
- Create: `web/scripts/playwright-project-template-smoke.mjs`
- Modify: `web/package.json`
- Modify: `tests/integration/postgres_e2e_test.go`
- Modify: `README.md`
- Modify: `ROADMAP.md`
- Modify: `docs/superpowers/specs/2026-07-20-project-template-snapshot-design.md`
- Modify: `docs/superpowers/plans/2026-07-20-project-template-snapshot-implementation.md`

**Interfaces:**
- Consumes: Tasks 1–13 完整实现。
- Produces: 可重复的 browser smoke、PostgreSQL workflow 和 v0.6.0 用户/开发文档闭环。

- [x] **Step 1: 写 Playwright desktop/mobile smoke**

脚本启动真实 server + Web，创建 source Project 和超过一页的 task/series/config/automation fixture；桌面路径验证“另存为模板”→筛选→跨页选择→Preview resolution→保存→Template Library version；再“从模板创建”填写 secret/member replacement，断言新项目 counts、task relation、Series 无 occurrence、automation disabled。移动 viewport 验证全屏 Sheet、已选清单和返回聚焦。脚本 finally 必须终止子进程并检查端口释放。

在 `web/package.json` 增加：

```json
"smoke:project-template": "node scripts/playwright-project-template-smoke.mjs"
```

- [x] **Step 2: 扩充 PostgreSQL E2E 的真实 HTTP/MCP current workflow**

同一临时 PostgreSQL DB 通过 HTTP Capture 一个含 task/Series/config/automation 的 Template；MCP list 读取 current ID/hash并 instantiate；数据库断言 `snapshot_json` 是 text、旧 snapshot 未变化、new automation disabled、无 copied occurrence/delivery。再次 append version 后用旧 hash 调 MCP，必须得到 `project_template_snapshot_hash_mismatch`。

2026-07-21 本地执行时 `XUANCHU_E2E_POSTGRES_ADMIN_URL` 未设置；测试已编译并按约定明确 SKIP，未把该次执行记录为 PostgreSQL 现场通过。

- [x] **Step 3: 同步 README、ROADMAP、spec/plan 状态**

README 写 Web 治理入口、CLI 两个命令完整示例、stdin input JSON、安全提示和 MCP 两 tool；明确 CLI/MCP 不提供 Capture/preview/version governance。ROADMAP 将 v0.6.0 从“设计阶段”改为完成并列出 Snapshot JSON、筛选/跨页选择、原子 instantiate、Web/CLI/MCP 边界。spec 状态改“已实现”，plan 勾选实际完成项；未执行项不能预先勾选。

- [x] **Step 4: 运行后端、零 CGO、静态检查和 Web 全套验证**

Run:

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
go vet ./...
git diff --check
pnpm --dir web typecheck
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web build
pnpm --dir web run smoke:project-template
pnpm --dir web run smoke:task-series
pnpm --dir web run smoke:editing
```

Expected: 全部 exit 0；smoke 结束后无残留 server/Vite 进程和监听端口。若配置了 `XUANCHU_E2E_POSTGRES_ADMIN_URL`，再运行 `go test ./tests/integration -run PostgresE2E -count=1` 并要求 PASS；未配置时明确记录 SKIP，不能声称 PostgreSQL 已现场通过。

2026-07-21 实际执行上述全部命令均 exit 0；Web 单测为 139 files / 817 tests，三条 smoke 均通过且 Task 14 worktree 无残留进程。PostgreSQL opt-in 命令 exit 0，但因环境变量未设置而明确 SKIP。

- [x] **Step 5: 检查范围与提交**

Run: `git status --short && git diff --stat && git diff --check`

Expected: 只包含 project template v0.6.0 相关代码、测试和文档；没有数据库、token、构建产物或临时截图。

```bash
git add README.md ROADMAP.md docs/superpowers/specs/2026-07-20-project-template-snapshot-design.md docs/superpowers/plans/2026-07-20-project-template-snapshot-implementation.md web/package.json web/scripts/playwright-project-template-smoke.mjs tests/integration/postgres_e2e_test.go
git commit -m "docs: 完成项目模板交付文档"
```

---

## 最终验收映射

| Spec 验收项 | 实施任务 |
|---|---|
| workspace 隔离、project-scoped token 拒绝 | 2、3、8、12、13 |
| 单 JSON 字段、Go struct、strict codec、hash | 1、2、5 |
| 四类逐项选择、大项目筛选、分页、跨页已选 | 4、8、10 |
| 普通 task 新身份与 relation/ref 映射 | 5、6、7 |
| Series current definition、单 RuleVersion、无 occurrence/history | 5、7 |
| 显式 config、secret placeholder/redaction | 5、6、7、8、11 |
| disabled automation、无 delivery | 5、6、7 |
| anchor/start date 与 DST | 1、5、6、7 |
| Capture/Instantiate Preview 与 source/hash drift | 5、6、8、10、11 |
| 单事务回滚、审计和来源事件字段 | 7、14 |
| HTTP/Web 完整治理 | 8–11 |
| CLI/Remote/MCP 仅 list/current instantiate | 12、13 |
| UserInfo/ActorInfo 统一 | 3、7、8、12、13 |
| SQLite/PostgreSQL、零 CGO 和完整验证 | 2、14 |
