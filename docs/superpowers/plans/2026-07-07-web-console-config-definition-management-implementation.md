# ConfigDefinition Web Console Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Web Console 中完整落地 workspace/project `ConfigDefinition` 管理、project config effective value 可视化、以及首页配置概览。

**Architecture:** 后端先收紧 `ConfigDefinition` 数据契约和变更保护，提供 usage / effective config API；前端再以复用组件实现 `/settings`、`/projects/$projectSlug/settings/definitions` 和 `/projects/$projectSlug/settings/config`。`ConfigDefinition` 仍然只按 workspace 存储，project 页面只是 project-scope 过滤视图，不引入 project 私有 schema 表。

**Tech Stack:** Go 1.25、GORM、`github.com/glebarez/sqlite`、PostgreSQL driver、Cobra/HTTP API、React 19、TanStack Router、TanStack Query、shadcn/radix UI、Vitest。

---

**规格来源：** `docs/superpowers/specs/2026-07-07-web-console-config-definition-management-design.md`

**执行方式：**

- 先读本计划和规格文档。
- 每个任务按 TDD 顺序执行：先写失败测试，再最小实现，再跑对应测试。
- 每个 chunk 结束后跑 chunk 验证命令。
- 最终必须跑完整验证命令。
- 本计划涉及前后端共享契约，提交要小步、中文 commit message。

## File Structure

后端文件：

- Modify: `internal/storage/models.go` - `ConfigDefinition` 增加 `ShowOnConsoleHome`。
- Modify: `internal/storage/config_definition_repo.go` - upsert 字段补 `show_on_console_home`。
- Modify: `internal/storage/config_repo.go` - usage count、按 key 列出现有值、按 key 删除的 helper。
- Modify: `internal/storage/config_repo_test.go` - storage CRUD/usage helper 测试。
- Modify: `internal/storage/db_test.go` - migration column 测试。
- Modify: `internal/storage/postgres_test.go` - PostgreSQL 下新字段/CRUD 测试。
- Modify: `internal/app/config_schema.go` - input/view 增加 `ShowOnConsoleHome`。
- Modify: `internal/app/scoped_config.go` - update 锁定规则、usage、workspace/project effective view。
- Modify: `internal/app/service_test.go` - app 层 schema 锁定与 effective view 测试。
- Modify: `internal/httpapi/context_config.go` - request/response 字段、usage/effective handlers。
- Modify: `internal/httpapi/huma_routes.go` - 注册新 HTTP 路由。
- Modify/Create: `internal/httpapi/*_test.go` - schema/effective HTTP 契约测试。
- Modify as needed: `internal/remote/config.go`, `internal/cli/config.go`, `internal/mcpserver/tools_config.go` - 仅当当前 schema DTO 对新字段有显式 shape，需要补透传；CLI/MCP 不新增命令。

前端文件：

- Create: `web/src/features/workspace/config/config-definition-api.ts`
- Create: `web/src/features/workspace/config/config-definition-api.test.ts`
- Create: `web/src/features/workspace/config/config-value-control.tsx`
- Create: `web/src/features/workspace/config/config-value-control.test.tsx`
- Create: `web/src/features/workspace/config/config-definition-form.tsx`
- Create: `web/src/features/workspace/config/config-definition-form.test.tsx`
- Create: `web/src/features/workspace/config/config-definition-manager.tsx`
- Create: `web/src/features/workspace/config/config-definition-manager.test.tsx`
- Create: `web/src/pages/config-definitions-page.tsx`
- Create: `web/src/routes/workspace/SettingsRoute.tsx`
- Create: `web/src/routes/workspace/ProjectSettingsDefinitionsRoute.tsx`
- Modify: `web/src/routes/router.tsx`
- Modify: `web/src/pages/project-settings-layout.tsx`
- Modify: `web/src/pages/project-config-tab.tsx`
- Modify/Delete as needed: `web/src/features/workspace/project-workbench/api/config-schema-api.ts` - 迁移或 re-export 到新 API 模块。
- Modify: `web/src/features/workspace/project-workbench/api/project-api.ts`
- Modify: `web/src/pages/OverviewPage.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`
- Modify: `web/src/features/workspace/resources/resource-config.tsx` - 移除 `settings` 的 ResourcePage 配置使用路径，或保留但不再路由到它。
- Modify: `web/src/components/AppShell.tsx` - 如果 `PageKey`/导航仍可复用则不动；若 `settings` 不再走 ResourceRoute，保留 nav key 即可。

验证文件：

- Modify/Add front-end tests near touched components.
- Update docs only if implementation changes user-visible route semantics beyond this plan; likely no README change until feature完成。

## Chunk 1: 后端契约、迁移与变更保护

### Task 1: `ConfigDefinition` 增加首页展示字段并补 storage helper

**Files:**

- Modify: `internal/storage/models.go`
- Modify: `internal/storage/config_definition_repo.go`
- Modify: `internal/storage/config_repo.go`
- Modify: `internal/storage/config_repo_test.go`
- Modify: `internal/storage/db_test.go`
- Modify: `internal/storage/postgres_test.go`

- [ ] **Step 1: 写 storage 失败测试**

在 `internal/storage/config_repo_test.go` 的 `TestConfigDefinitionRepositoryCRUD` 中补断言：

```go
def.ShowOnConsoleHome = true
if err := repo.Set(def); err != nil {
    t.Fatalf("Set() error = %v", err)
}
got, ok, err := repo.Get("ws-a", "ads.roi_threshold")
if err != nil || !ok {
    t.Fatalf("Get() = (_, %v, %v), want found nil", ok, err)
}
if !got.ShowOnConsoleHome {
    t.Fatal("ShowOnConsoleHome = false, want true")
}
```

新增 usage helper 测试：

```go
func TestConfigRepositoryUsageAndValuesByKey(t *testing.T) {
    store := openIdentityTestStore(t)
    repo := NewConfigRepository(store.DB())
    ws := "ws-usage"
    projectID := "project-api"
    mustConfigSet(t, repo, ConfigKey{WorkspaceID: ws, Scope: ConfigScopeWorkspace, Key: "ads.budget"}, "100")
    mustConfigSet(t, repo, ConfigKey{WorkspaceID: ws, Scope: ConfigScopeProject, ScopeID: projectID, Key: "ads.budget"}, "200")

    workspaceCount, projectCount, err := repo.CountByKey(ws, "ads.budget")
    if err != nil {
        t.Fatalf("CountByKey() error = %v", err)
    }
    if workspaceCount != 1 || projectCount != 1 {
        t.Fatalf("CountByKey() = (%d, %d), want (1, 1)", workspaceCount, projectCount)
    }

    values, err := repo.ValuesByKey(ws, "ads.budget")
    if err != nil {
        t.Fatalf("ValuesByKey() error = %v", err)
    }
    if strings.Join(values, ",") != "100,200" {
        t.Fatalf("ValuesByKey() = %#v, want sorted 100,200", values)
    }
}
```

在 `internal/storage/db_test.go` 增加：

```go
if !store.DB().Migrator().HasColumn(&ConfigDefinition{}, "show_on_console_home") {
    t.Fatal("config_definitions.show_on_console_home column missing after migration")
}
```

Run:

```bash
go test ./internal/storage -run 'ConfigDefinition|ConfigRepositoryUsage|ConfigDefinitionTableMigrated' -count=1
```

Expected: FAIL，缺 `ShowOnConsoleHome` 字段或 `ValuesByKey` 方法。

- [ ] **Step 2: 实现 storage 字段**

在 `internal/storage/models.go` 的 `ConfigDefinition` 增加：

```go
ShowOnConsoleHome bool `gorm:"not null;default:false"`
```

在 `internal/storage/config_definition_repo.go` 的 `AssignmentColumns` 增加：

```go
"show_on_console_home",
```

- [ ] **Step 3: 实现 config value helper**

在 `internal/storage/config_repo.go` 增加：

```go
func (r *ConfigRepository) ValuesByKey(workspaceID, key string) ([]string, error) {
    var rows []Config
    if err := r.db.Where("workspace_id = ? AND key = ?", workspaceID, key).
        Order("scope ASC, scope_id ASC, value ASC").
        Find(&rows).Error; err != nil {
        return nil, err
    }
    values := make([]string, 0, len(rows))
    for _, row := range rows {
        values = append(values, row.Value)
    }
    return values, nil
}
```

如果 scope lock 需要分 scope 查值，再补：

```go
func (r *ConfigRepository) CountByKeyAndScope(workspaceID, key string, scope ConfigScope) (int64, error)
func (r *ConfigRepository) ValuesByKeyAndScope(workspaceID, key string, scope ConfigScope) ([]string, error)
```

保持参数绑定，不拼接用户输入。

- [ ] **Step 4: 跑 storage 测试**

Run:

```bash
go test ./internal/storage -run 'ConfigDefinition|ConfigRepositoryUsage|ConfigDefinitionTableMigrated' -count=1
```

Expected: PASS。

- [ ] **Step 5: PostgreSQL 测试补字段断言**

在 `internal/storage/postgres_test.go` 的 `TestPostgres_ConfigDefinitionCRUD` 中补 `ShowOnConsoleHome` 断言。

Run:

```bash
go test ./internal/storage -run Postgres_ConfigDefinitionCRUD -count=1
```

Expected: 在没有 PostgreSQL 环境时按现有 skip 语义跳过；有环境时 PASS。

- [ ] **Step 6: Commit**

```bash
git add internal/storage/models.go internal/storage/config_definition_repo.go internal/storage/config_repo.go internal/storage/config_repo_test.go internal/storage/db_test.go internal/storage/postgres_test.go
git commit -m "feat: 扩展配置定义首页展示字段"
```

### Task 2: App 层 schema update 锁定规则

**Files:**

- Modify: `internal/app/config_schema.go`
- Modify: `internal/app/scoped_config.go`
- Modify: `internal/app/service_test.go`

- [ ] **Step 1: 写失败测试：已有 value 时禁止改 type**

在 `internal/app/service_test.go` 新增：

```go
func TestConfigSchemaSetRejectsTypeChangeWhenValuesExist(t *testing.T) {
    svc, closeFn := newTestService(t, 100)
    defer closeFn()

    mustSetConfigSchema(t, svc, ConfigSchemaInput{
        Key: "ads.budget", ValueType: "number", AllowedScopes: []string{"workspace", "project"},
    })
    if err := svc.SetConfig("ads.budget", "100"); err != nil {
        t.Fatalf("SetConfig() error = %v", err)
    }

    err := svc.ConfigSchemaSet(ConfigSchemaInput{
        Key: "ads.budget", ValueType: "string", AllowedScopes: []string{"workspace", "project"},
    })
    assertRuntimeCode(t, err, "config_definition_type_locked")
}
```

Run:

```bash
go test ./internal/app -run ConfigSchemaSetRejectsTypeChangeWhenValuesExist -count=1
```

Expected: FAIL，当前允许改 type。

- [ ] **Step 2: 写失败测试：不能移除已有 value 的 scope**

新增：

```go
func TestConfigSchemaSetRejectsScopeRemovalWhenValuesExist(t *testing.T) {
    svc, closeFn := newTestService(t, 100)
    defer closeFn()
    project, err := svc.AddProject(AddProjectInput{Slug: "api", Name: "API"})
    if err != nil { t.Fatalf("AddProject() error = %v", err) }

    mustSetConfigSchema(t, svc, ConfigSchemaInput{
        Key: "ads.budget", ValueType: "number", AllowedScopes: []string{"workspace", "project"},
    })
    if err := svc.ProjectConfigSet(project.ID, "ads.budget", "200"); err != nil {
        t.Fatalf("ProjectConfigSet() error = %v", err)
    }

    err = svc.ConfigSchemaSet(ConfigSchemaInput{
        Key: "ads.budget", ValueType: "number", AllowedScopes: []string{"workspace"},
    })
    assertRuntimeCode(t, err, "config_definition_scope_locked")
}
```

Run:

```bash
go test ./internal/app -run ConfigSchemaSetRejectsScopeRemovalWhenValuesExist -count=1
```

Expected: FAIL。

- [ ] **Step 3: 写失败测试：新 enum 必须包含旧 value**

新增：

```go
func TestConfigSchemaSetRejectsEnumRemovalWhenValuesExist(t *testing.T) {
    svc, closeFn := newTestService(t, 100)
    defer closeFn()

    mustSetConfigSchema(t, svc, ConfigSchemaInput{
        Key: "ads.mode", ValueType: "string", AllowedScopes: []string{"workspace"}, EnumValues: []string{"auto", "manual"},
    })
    if err := svc.SetConfig("ads.mode", "manual"); err != nil {
        t.Fatalf("SetConfig() error = %v", err)
    }

    err := svc.ConfigSchemaSet(ConfigSchemaInput{
        Key: "ads.mode", ValueType: "string", AllowedScopes: []string{"workspace"}, EnumValues: []string{"auto"},
    })
    assertRuntimeCode(t, err, "config_definition_enum_locked")
}
```

Run:

```bash
go test ./internal/app -run ConfigSchemaSetRejectsEnumRemovalWhenValuesExist -count=1
```

Expected: FAIL。

- [ ] **Step 4: 写失败测试：首页展示字段可保存读取**

新增：

```go
func TestConfigSchemaSetPersistsShowOnConsoleHome(t *testing.T) {
    svc, closeFn := newTestService(t, 100)
    defer closeFn()

    if err := svc.ConfigSchemaSet(ConfigSchemaInput{
        Key: "ads.roi_threshold", ValueType: "number", AllowedScopes: []string{"workspace"}, ShowOnConsoleHome: true,
    }); err != nil {
        t.Fatalf("ConfigSchemaSet() error = %v", err)
    }
    got, ok, err := svc.ConfigSchemaGet("ads.roi_threshold")
    if err != nil || !ok {
        t.Fatalf("ConfigSchemaGet() = (_, %v, %v), want found nil", ok, err)
    }
    if !got.ShowOnConsoleHome {
        t.Fatal("ShowOnConsoleHome = false, want true")
    }
}
```

Run:

```bash
go test ./internal/app -run ConfigSchemaSetPersistsShowOnConsoleHome -count=1
```

Expected: FAIL。

- [ ] **Step 5: 实现 input/view 字段**

在 `internal/app/config_schema.go`：

```go
type ConfigSchemaInput struct {
    ...
    ShowOnConsoleHome bool
}

type ConfigDefinitionView struct {
    ...
    ShowOnConsoleHome bool
}
```

`normalizeConfigDefinitionInput` 返回值补：

```go
ShowOnConsoleHome: input.ShowOnConsoleHome,
```

`configDefinitionViewFromRow` 补：

```go
ShowOnConsoleHome: row.ShowOnConsoleHome,
```

- [ ] **Step 6: 实现锁定规则**

在 `internal/app/scoped_config.go` 的 `ConfigSchemaSet` 中，拿到 `existing, ok` 后、写入前调用 helper：

```go
if ok {
    if err := s.validateConfigDefinitionUpdate(existing, def); err != nil {
        return err
    }
}
```

新增 helper 语义：

- type 改变且 total values > 0：`config_definition_type_locked`
- 移除 workspace scope 且 workspace values > 0：`config_definition_scope_locked`
- 移除 project scope 且 project values > 0：`config_definition_scope_locked`
- 新 enum 非空且不包含所有 existing values：`config_definition_enum_locked`

注意：existing 是 storage row，def 是 normalized row；比较 scopes 时 decode JSON，比较 enum 时 decode JSON；已有 values 来自 `configRepo.ValuesByKey`。

- [ ] **Step 7: 跑 app schema 测试**

Run:

```bash
go test ./internal/app -run 'ConfigSchemaSetRejects|ConfigSchemaSetPersistsShowOnConsoleHome|ConfigSchemaDeleteRequiresPurge' -count=1
```

Expected: PASS。

- [ ] **Step 8: Commit**

```bash
git add internal/app/config_schema.go internal/app/scoped_config.go internal/app/service_test.go
git commit -m "feat: 加固配置定义更新规则"
```

### Task 3: App 层 usage 与 effective config view

**Files:**

- Modify: `internal/app/scoped_config.go`
- Modify: `internal/app/config_schema.go`
- Modify: `internal/app/service_test.go`

- [ ] **Step 1: 写失败测试：schema usage count**

新增：

```go
func TestConfigSchemaUsageCountsWorkspaceAndProjectValues(t *testing.T) {
    svc, closeFn := newTestService(t, 100)
    defer closeFn()
    project, err := svc.AddProject(AddProjectInput{Slug: "api", Name: "API"})
    if err != nil { t.Fatalf("AddProject() error = %v", err) }

    mustSetConfigSchema(t, svc, ConfigSchemaInput{
        Key: "ads.budget", ValueType: "number", AllowedScopes: []string{"workspace", "project"},
    })
    _ = svc.SetConfig("ads.budget", "100")
    _ = svc.ProjectConfigSet(project.ID, "ads.budget", "200")

    got, err := svc.ConfigSchemaUsage("ads.budget")
    if err != nil { t.Fatalf("ConfigSchemaUsage() error = %v", err) }
    if got.WorkspaceValues != 1 || got.ProjectValues != 1 || got.TotalValues != 2 {
        t.Fatalf("usage = %#v, want 1/1/2", got)
    }
}
```

Run:

```bash
go test ./internal/app -run ConfigSchemaUsageCountsWorkspaceAndProjectValues -count=1
```

Expected: FAIL，方法未定义。

- [ ] **Step 2: 写失败测试：project effective config**

新增：

```go
func TestProjectConfigEffectiveValuesResolveProjectWorkspaceDefaultAndMissing(t *testing.T) {
    svc, closeFn := newTestService(t, 100)
    defer closeFn()
    project, err := svc.AddProject(AddProjectInput{Slug: "api", Name: "API"})
    if err != nil { t.Fatalf("AddProject() error = %v", err) }

    mustSetConfigSchema(t, svc, ConfigSchemaInput{Key: "k.project", ValueType: "string", AllowedScopes: []string{"project"}})
    mustSetConfigSchema(t, svc, ConfigSchemaInput{Key: "k.workspace", ValueType: "string", AllowedScopes: []string{"workspace", "project"}})
    def := "fallback"
    mustSetConfigSchema(t, svc, ConfigSchemaInput{Key: "k.default", ValueType: "string", AllowedScopes: []string{"project"}, DefaultValue: &def})
    mustSetConfigSchema(t, svc, ConfigSchemaInput{Key: "k.required", ValueType: "string", AllowedScopes: []string{"project"}, Required: true})

    _ = svc.ProjectConfigSet(project.ID, "k.project", "project-value")
    _ = svc.SetConfig("k.workspace", "workspace-value")

    rows, err := svc.ProjectConfigEffectiveValues(project.ID)
    if err != nil { t.Fatalf("ProjectConfigEffectiveValues() error = %v", err) }
    assertEffectiveSource(t, rows, "k.project", "project", "project-value")
    assertEffectiveSource(t, rows, "k.workspace", "workspace", "workspace-value")
    assertEffectiveSource(t, rows, "k.default", "default", "fallback")
    assertEffectiveMissing(t, rows, "k.required")
}
```

Run:

```bash
go test ./internal/app -run ProjectConfigEffectiveValuesResolve -count=1
```

Expected: FAIL，方法未定义。

- [ ] **Step 3: 写失败测试：workspace console home effective**

新增：

```go
func TestWorkspaceConfigEffectiveValuesFiltersConsoleHome(t *testing.T) {
    svc, closeFn := newTestService(t, 100)
    defer closeFn()

    def := "1.8"
    mustSetConfigSchema(t, svc, ConfigSchemaInput{
        Key: "ads.roi", ValueType: "number", AllowedScopes: []string{"workspace"}, DefaultValue: &def, ShowOnConsoleHome: true,
    })
    mustSetConfigSchema(t, svc, ConfigSchemaInput{
        Key: "project.only", ValueType: "string", AllowedScopes: []string{"project"}, ShowOnConsoleHome: true,
    })
    rows, err := svc.WorkspaceConfigEffectiveValues(ConfigEffectiveFilter{ConsoleHomeOnly: true})
    if err != nil { t.Fatalf("WorkspaceConfigEffectiveValues() error = %v", err) }
    if len(rows) != 1 || rows[0].Key != "ads.roi" || rows[0].Source != "default" {
        t.Fatalf("rows = %#v, want only ads.roi default", rows)
    }
}
```

Run:

```bash
go test ./internal/app -run WorkspaceConfigEffectiveValuesFiltersConsoleHome -count=1
```

Expected: FAIL。

- [ ] **Step 4: 实现 app view types**

在 `internal/app/config_schema.go` 或新文件 `internal/app/config_effective.go`（推荐新文件，职责更清楚）增加：

```go
type ConfigSchemaUsageView struct {
    Key             string `json:"key"`
    WorkspaceValues int64  `json:"workspace_values"`
    ProjectValues   int64  `json:"project_values"`
    TotalValues     int64  `json:"total_values"`
}

type ConfigEffectiveFilter struct {
    ConsoleHomeOnly bool
}

type ConfigEffectiveValueView struct {
    Key               string               `json:"key"`
    Value             *string              `json:"value"`
    Source            string               `json:"source"` // project, workspace, default, missing
    ProjectValue      *string              `json:"project_value,omitempty"`
    WorkspaceValue    *string              `json:"workspace_value,omitempty"`
    DefaultValue      *string              `json:"default_value,omitempty"`
    Definition        ConfigDefinitionView `json:"definition"`
    ShowOnConsoleHome bool                 `json:"show_on_console_home"`
    MissingRequired   bool                 `json:"missing_required"`
}
```

Go 结构体不必须加 JSON tag 给 app 内部用，但加上能减少 HTTP response mirror 类型。

- [ ] **Step 5: 实现 usage/effective 方法**

在 `internal/app/scoped_config.go` 或 `config_effective.go` 实现：

```go
func (s *Service) ConfigSchemaUsage(key string) (ConfigSchemaUsageView, error)
func (s *Service) ProjectConfigEffectiveValues(projectRef string) ([]ConfigEffectiveValueView, error)
func (s *Service) WorkspaceConfigEffectiveValues(filter ConfigEffectiveFilter) ([]ConfigEffectiveValueView, error)
```

实现要点：

- 权限：usage/effective 走 `PermissionConfigSchemaRead` 和/或现有 config read，保持比写更宽。
- project effective 必须先 `resolveProject(projectRef)`，复用现有 project config 路径，不跨 workspace。
- project effective 遍历全部 definitions；source 顺序：project explicit > workspace explicit > default > missing。
- workspace effective 遍历全部 definitions；只解析 `allowed_scopes` 包含 workspace 的 key。
- `ConsoleHomeOnly` 过滤 `ShowOnConsoleHome=true`，且过滤掉 project-only key。
- secret 原始值仍可留给 HTTP handler 决定是否遮掩；如果直接在 app 层遮掩，必须确认 project settings 编辑页仍能读取显式 secret 值。建议 app 返回真实 value，HTTP effective handler 为首页请求遮掩。

- [ ] **Step 6: 跑 app effective 测试**

Run:

```bash
go test ./internal/app -run 'ConfigSchemaUsage|EffectiveValues' -count=1
```

Expected: PASS。

- [ ] **Step 7: Commit**

```bash
git add internal/app/config_schema.go internal/app/scoped_config.go internal/app/config_effective.go internal/app/service_test.go
git commit -m "feat: 增加配置使用量与有效值视图"
```

### Task 4: HTTP API 暴露 schema usage 与 effective config

**Files:**

- Modify: `internal/httpapi/context_config.go`
- Modify: `internal/httpapi/huma_routes.go`
- Modify/Create: `internal/httpapi/config_schema_test.go` or existing HTTP test file

- [ ] **Step 1: 写失败测试：schema request/response 透出 `show_on_console_home`**

在 HTTP 测试中新增：

```go
rr := requestHTTPBody(t, srv, http.MethodPut, "/api/v1/config-schema/ads.roi?workspace=local", `{
  "value_type":"number",
  "allowed_scopes":["workspace"],
  "default_value":"1.8",
  "show_on_console_home":true
}`, authHeader)
assertHTTPStatus(t, rr, http.StatusOK)
assertJSONPathBool(t, rr.Body.String(), "data.show_on_console_home", true)
```

Run:

```bash
go test ./internal/httpapi -run ConfigSchema -count=1
```

Expected: FAIL，字段未解析或未返回。

- [ ] **Step 2: 写失败测试：usage endpoint**

新增 GET `/api/v1/config-schema/{key}/usage` 测试，断言返回 workspace/project/total count。

Run:

```bash
go test ./internal/httpapi -run ConfigSchemaUsage -count=1
```

Expected: FAIL，路由不存在。

- [ ] **Step 3: 写失败测试：project effective endpoint**

新增 GET `/api/v1/projects/{projectRef}/config/effective` 测试，断言返回 `source` 为 `project/workspace/default/missing` 的数组。

Run:

```bash
go test ./internal/httpapi -run ProjectConfigEffective -count=1
```

Expected: FAIL。

- [ ] **Step 4: 写失败测试：workspace home effective endpoint**

新增 GET `/api/v1/config/effective?console_home=true` 测试，断言只返回 show_on_console_home 且 workspace 可解析的 key；secret value 应为遮掩值。

Run:

```bash
go test ./internal/httpapi -run WorkspaceConfigEffectiveConsoleHome -count=1
```

Expected: FAIL。

- [ ] **Step 5: 实现 request 字段与 handlers**

在 `configSchemaRequest` 增加：

```go
ShowOnConsoleHome bool `json:"show_on_console_home"`
```

组装 `app.ConfigSchemaInput` 时补：

```go
ShowOnConsoleHome: req.ShowOnConsoleHome,
```

新增 handlers：

```go
func (s *Server) handleConfigSchemaUsage(w http.ResponseWriter, r *http.Request)
func (s *Server) handleProjectConfigEffective(w http.ResponseWriter, r *http.Request)
func (s *Server) handleConfigEffective(w http.ResponseWriter, r *http.Request)
```

secret 遮掩规则：

- `/api/v1/config/effective?console_home=true`：如果 `row.Definition.Secret` 且 `row.Value != nil`，响应 value 用 `"••••••"`，具体 project/workspace/default value 也不要明文返回。
- project settings 的 `/projects/{ref}/config/effective` 首版可以返回明文以支持编辑现有 secret；前端默认遮掩。

- [ ] **Step 6: 注册 Huma routes**

在 `internal/httpapi/huma_routes.go` 注册，注意顺序必须让固定路径早于 `{key}`：

```go
{Method: http.MethodGet, Path: "/api/v1/config/effective", Tag: "Config", Summary: "List effective workspace config.", Handler: s.handleConfigEffective},
{Method: http.MethodGet, Path: "/api/v1/projects/{projectRef}/config/effective", Tag: "Project Config", Summary: "List effective project config.", Handler: s.handleProjectConfigEffective},
{Method: http.MethodGet, Path: "/api/v1/config-schema/{key}/usage", Tag: "Config Schema", Summary: "Get config schema usage.", Handler: s.handleConfigSchemaUsage},
```

保持 `/api/v1/config/{key}` 不吞掉 `/api/v1/config/effective`。若 chi/huma 路由顺序不可靠，使用不会冲突的路径 `/api/v1/config-effective` 要先回 spec 更新；默认先按 spec 路径实现。

- [ ] **Step 7: 跑 HTTP 测试**

Run:

```bash
go test ./internal/httpapi -run 'ConfigSchema|ConfigEffective|ProjectConfigEffective' -count=1
```

Expected: PASS。

- [ ] **Step 8: Commit**

```bash
git add internal/httpapi/context_config.go internal/httpapi/huma_routes.go internal/httpapi/*_test.go
git commit -m "feat: 暴露配置定义与有效值接口"
```

### Chunk 1 验证

Run:

```bash
go test ./internal/storage ./internal/app ./internal/httpapi -count=1
CGO_ENABLED=0 go test ./internal/storage ./internal/app ./internal/httpapi -count=1
```

Expected: PASS。

## Chunk 2: 前端 API 与共享控件

### Task 5: 新建 config definition API 模块

**Files:**

- Create: `web/src/features/workspace/config/config-definition-api.ts`
- Create: `web/src/features/workspace/config/config-definition-api.test.ts`
- Modify: `web/src/features/workspace/project-workbench/api/config-schema-api.ts`
- Modify: `web/src/features/workspace/project-workbench/api/config-schema-api.test.ts`
- Modify: `web/src/features/workspace/project-workbench/api/project-api.ts`
- Modify: `web/src/features/workspace/project-workbench/api/project-api.test.ts`

- [ ] **Step 1: 写失败测试**

`config-definition-api.test.ts` 覆盖：

```ts
expect(configSchemaPath()).toBe("/api/v1/config-schema")
expect(configSchemaKeyPath("ads.roi")).toBe("/api/v1/config-schema/ads.roi")
expect(configSchemaUsagePath("ads.roi")).toBe("/api/v1/config-schema/ads.roi/usage")
expect(workspaceConfigEffectivePath({ consoleHome: true })).toBe("/api/v1/config/effective?console_home=true")
expect(projectConfigEffectivePath("api")).toBe("/api/v1/projects/api/config/effective")
```

DTO shape 必须包含：

```ts
show_on_console_home: false,
```

Run:

```bash
pnpm --dir web test -- config-definition-api.test.ts
```

Expected: FAIL，文件不存在。

- [ ] **Step 2: 实现 API 模块**

定义：

```ts
export type ConfigValueType = "string" | "number" | "boolean" | "json"
export type ConfigAllowedScope = "workspace" | "project"

export type ConfigSchemaDefinition = {
  key: string
  value_type: ConfigValueType | string
  allowed_scopes: string[]
  label: string
  description: string
  enum_values: string[]
  default_value: string | null
  required: boolean
  secret: boolean
  show_on_console_home: boolean
  created_at: number
  modified_at: number
}

export type ConfigSchemaInput = {
  value_type: ConfigValueType
  allowed_scopes: ConfigAllowedScope[]
  label: string
  description: string
  enum_values: string[]
  default_value: string | null
  required: boolean
  secret: boolean
  show_on_console_home: boolean
}
```

函数：

```ts
listConfigSchema()
getConfigSchema(key)
setConfigSchema(key, input)
deleteConfigSchema(key, purge)
getConfigSchemaUsage(key)
listWorkspaceEffectiveConfig({ consoleHome })
listProjectEffectiveConfig(projectRef)
```

使用 `workspaceApiGet/Put/Delete`。

- [ ] **Step 3: 兼容旧导入路径**

把 `web/src/features/workspace/project-workbench/api/config-schema-api.ts` 改成 re-export：

```ts
export {
  configSchemaPath,
  listConfigSchema,
  type ConfigSchemaDefinition,
} from "@/features/workspace/config/config-definition-api"
```

这样当前 `ProjectConfigTab` 暂不需要一次性改完。

- [ ] **Step 4: project-api 增加 effective path re-export 或 wrapper**

在 `project-api.ts` 增加：

```ts
export { projectConfigEffectivePath, listProjectEffectiveConfig } from "@/features/workspace/config/config-definition-api"
```

如果 circular import 风险较高，就在 config API 模块独立维护 project path，不从 project-api re-export。

- [ ] **Step 5: 跑测试**

Run:

```bash
pnpm --dir web test -- config-definition-api.test.ts config-schema-api.test.ts project-api.test.ts
```

Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add web/src/features/workspace/config/config-definition-api.ts web/src/features/workspace/config/config-definition-api.test.ts web/src/features/workspace/project-workbench/api/config-schema-api.ts web/src/features/workspace/project-workbench/api/config-schema-api.test.ts web/src/features/workspace/project-workbench/api/project-api.ts web/src/features/workspace/project-workbench/api/project-api.test.ts
git commit -m "feat: 增加配置定义前端 API"
```

### Task 6: 共享 ConfigValueControl

**Files:**

- Create: `web/src/features/workspace/config/config-value-control.tsx`
- Create: `web/src/features/workspace/config/config-value-control.test.tsx`

- [ ] **Step 1: 写失败测试**

测试矩阵：

- enum values 渲染 select。
- number 渲染 number input。
- boolean 渲染 switch 或 select。
- json 渲染 textarea，非法 JSON 显示错误。
- secret 渲染 password input。

示例：

```tsx
render(<ConfigValueControl definition={def({ value_type: "boolean" })} value="true" onChange={onChange} />)
expect(screen.getByRole("switch")).toBeTruthy()
```

Run:

```bash
pnpm --dir web test -- config-value-control.test.tsx
```

Expected: FAIL。

- [ ] **Step 2: 实现组件**

Props：

```ts
type ConfigValueControlProps = {
  definition: Pick<ConfigSchemaDefinition, "value_type" | "enum_values" | "secret">
  value: string
  onChange: (value: string) => void
  disabled?: boolean
  id?: string
}
```

实现规则：

- enum 优先级最高。
- boolean 用 `Switch`，输出 `"true"` / `"false"`。
- json 用 `Textarea`，仅做 parse 提示，不阻止输入。
- secret 用 password input + 可选 reveal；如果按钮用图标，使用 lucide `Eye/EyeOff`。
- 不在组件里发请求。

- [ ] **Step 3: 跑组件测试**

Run:

```bash
pnpm --dir web test -- config-value-control.test.tsx
```

Expected: PASS。

- [ ] **Step 4: Commit**

```bash
git add web/src/features/workspace/config/config-value-control.tsx web/src/features/workspace/config/config-value-control.test.tsx
git commit -m "feat: 增加配置值类型控件"
```

### Task 7: ConfigDefinition 表单与管理器

**Files:**

- Create: `web/src/features/workspace/config/config-definition-form.tsx`
- Create: `web/src/features/workspace/config/config-definition-form.test.tsx`
- Create: `web/src/features/workspace/config/config-definition-manager.tsx`
- Create: `web/src/features/workspace/config/config-definition-manager.test.tsx`

- [ ] **Step 1: 写表单失败测试**

覆盖：

- 新建 key 可编辑，编辑 key 不可改。
- `show_on_console_home` checkbox 存在。
- usage total > 0 时 `value_type` disabled。
- workspace/project scope checkbox 至少一个。
- 删除按钮打开确认，需要输入 key 后才能确认。

Run:

```bash
pnpm --dir web test -- config-definition-form.test.tsx
```

Expected: FAIL。

- [ ] **Step 2: 实现 `ConfigDefinitionForm`**

Props：

```ts
type ConfigDefinitionFormProps = {
  mode: "create" | "edit"
  initial?: ConfigSchemaDefinition
  usage?: ConfigSchemaUsage
  defaultScopes: ConfigAllowedScope[]
  onSubmit: (key: string, input: ConfigSchemaInput) => Promise<void>
  onDelete?: (key: string) => Promise<void>
  canManage: boolean
}
```

表单字段对应 spec：

- key
- value_type
- allowed_scopes
- label
- description
- enum_values
- default_value
- required
- secret
- show_on_console_home

用 `ConfigValueControl` 渲染 default value。

- [ ] **Step 3: 写 manager 失败测试**

覆盖：

- workspace variant 默认显示所有 definitions，能按 scope filter。
- project variant 默认只显示 project scope definitions。
- 删除时调用 `deleteConfigSchema(key, true)`。
- 保存后 invalidate query。

Run:

```bash
pnpm --dir web test -- config-definition-manager.test.tsx
```

Expected: FAIL。

- [ ] **Step 4: 实现 `ConfigDefinitionManager`**

Props：

```ts
type ConfigDefinitionManagerProps = {
  variant: "workspace" | "project"
  title: string
  description: string
  defaultScopes: ConfigAllowedScope[]
  canManage: boolean
}
```

Query：

- `["config-schema"]` -> list definitions。
- usage 可懒加载：打开编辑/删除时 `getConfigSchemaUsage(key)`；列表若要显示 count 可并行加载 usage map（首版允许列表显示 `-`，删除弹窗必须加载精确 count）。

UI 不做嵌套卡片；使用 border section/table/dialog。

- [ ] **Step 5: 跑测试**

Run:

```bash
pnpm --dir web test -- config-definition-form.test.tsx config-definition-manager.test.tsx
```

Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add web/src/features/workspace/config/config-definition-form.tsx web/src/features/workspace/config/config-definition-form.test.tsx web/src/features/workspace/config/config-definition-manager.tsx web/src/features/workspace/config/config-definition-manager.test.tsx
git commit -m "feat: 增加配置定义管理组件"
```

### Chunk 2 验证

Run:

```bash
pnpm --dir web test -- config-definition
pnpm --dir web typecheck
```

Expected: PASS。

## Chunk 3: Workspace `/settings` 与 project definitions 路由

### Task 8: `/settings` 改为 ConfigDefinition 页面

**Files:**

- Create: `web/src/pages/config-definitions-page.tsx`
- Create: `web/src/routes/workspace/SettingsRoute.tsx`
- Modify: `web/src/routes/router.tsx`
- Modify: `web/src/features/workspace/resources/resource-config.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`
- Add/Modify tests near route/page

- [ ] **Step 1: 写失败测试**

在 route/page 测试中断言 `/settings` 页面请求 `/api/v1/config-schema` 而不是 `/api/v1/config`。如果现有测试 harness 不适合路由，先给 `ConfigDefinitionsPage` 写组件级测试。

Run:

```bash
pnpm --dir web test -- config-definitions-page
```

Expected: FAIL。

- [ ] **Step 2: 实现页面**

`config-definitions-page.tsx`：

```tsx
export function ConfigDefinitionsPage({ variant }: { variant: "workspace" | "project" }) {
  return (
    <ConfigDefinitionManager
      variant={variant}
      title={...}
      description={...}
      defaultScopes={variant === "workspace" ? ["workspace"] : ["project"]}
      canManage={...}
    />
  )
}
```

权限从 `useMe()` 中的 `effective_role` / token scopes 判定；可以先复用 `canProjectManage`，如果语义不准，新增 `canConfigManage` 到 `web/src/features/workspace/project-workbench/permissions/permissions.ts`。

- [ ] **Step 3: Router 替换 ResourceRoute**

在 `router.tsx`：

- lazy import `SettingsRoute`
- 新建：

```tsx
const settingsRoute = createRoute({
  getParentRoute: () => workspaceRootRoute,
  path: "/settings",
  component: lazyRoute(SettingsRoute),
})
```

- route tree 中把 `createResourceRoute("settings", "/settings")` 替换为 `settingsRoute`。

`resource-config.tsx` 中 `settings` case 可删除；如果 TS union 要求保留，改成 throw 或不再引用。

- [ ] **Step 4: i18n**

新增中文 key：

- `configDefinitions.title`: `配置定义`
- `configDefinitions.description`: `定义哪些配置 key 可以被写入，以及值的类型、作用域和默认值。`
- `configDefinitions.showOnHome`: `显示在首页`
- `configDefinitions.showOnHomeHelp`: `仅影响 Web Console 首页展示；不是必填配置。`
- 其他表单字段和删除确认。

英文 key 同步，保持 typecheck。

- [ ] **Step 5: 跑测试**

Run:

```bash
pnpm --dir web test -- config-definitions-page router
pnpm --dir web typecheck
```

Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add web/src/pages/config-definitions-page.tsx web/src/routes/workspace/SettingsRoute.tsx web/src/routes/router.tsx web/src/features/workspace/resources/resource-config.tsx web/src/locales/zh-CN.ts web/src/locales/en-US.ts
git commit -m "feat: 设置页改为配置定义管理"
```

### Task 9: project settings 增加 definitions tab

**Files:**

- Create: `web/src/routes/workspace/ProjectSettingsDefinitionsRoute.tsx`
- Modify: `web/src/routes/router.tsx`
- Modify: `web/src/pages/project-settings-layout.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`
- Modify tests for project settings layout/router

- [ ] **Step 1: 写失败测试**

测试 `ProjectSettingsLayout` 有三个 tab，点击“配置定义”导航到 `/projects/$projectSlug/settings/definitions`。

Run:

```bash
pnpm --dir web test -- project-settings
```

Expected: FAIL。

- [ ] **Step 2: 增加 route**

`router.tsx` 增加 lazy import 和 route：

```tsx
const ProjectSettingsDefinitionsRoute = lazy(...)
const projectSettingsDefinitionsRoute = createRoute({
  getParentRoute: () => projectSettingsRoute,
  path: "definitions",
  component: lazyRoute(ProjectSettingsDefinitionsRoute),
})
```

route tree 子节点包含：

```tsx
projectSettingsDefinitionsRoute
```

- [ ] **Step 3: 实现 route component**

`ProjectSettingsDefinitionsRoute.tsx` 和当前 config/notes route 一样读取 `useMe`、project params；definition 页不因 project closed 变只读，只按 config schema 权限控制。

渲染：

```tsx
<ConfigDefinitionsPage variant="project" />
```

- [ ] **Step 4: 更新 tabs**

`project-settings-layout.tsx`：

- activeTab 支持 definitions。
- tab 顺序：config, definitions, notes。
- navigate paths 全部使用 TanStack `navigate`，不要用手写 `href`。

- [ ] **Step 5: 跑测试**

Run:

```bash
pnpm --dir web test -- project-settings
pnpm --dir web typecheck
```

Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add web/src/routes/workspace/ProjectSettingsDefinitionsRoute.tsx web/src/routes/router.tsx web/src/pages/project-settings-layout.tsx web/src/locales/zh-CN.ts web/src/locales/en-US.ts
git commit -m "feat: 项目设置增加配置定义页"
```

### Chunk 3 验证

Run:

```bash
pnpm --dir web test -- project-settings config-definitions
pnpm --dir web typecheck
```

Expected: PASS。

## Chunk 4: Project config effective value 页面与首页概览

### Task 10: project config tab 升级为 effective value 视图

**Files:**

- Modify: `web/src/pages/project-config-tab.tsx`
- Modify/Create: `web/src/features/workspace/config/project-effective-config-table.tsx`
- Modify/Create tests for project config tab/effective table
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

- [ ] **Step 1: 写失败测试**

覆盖：

- 显示 project/workspace/default/missing 四种来源。
- project-only 且无值的 required key 显示“必填缺失”。
- workspace-only key 显示只读，不出现编辑按钮。
- 删除 project 显式值按钮文案是“恢复继承”。

Run:

```bash
pnpm --dir web test -- project-config
```

Expected: FAIL。

- [ ] **Step 2: 改 query 数据源**

`ProjectConfigTab` 从：

- `listProjectConfig`
- `listConfigSchema`

改为首选：

- `listProjectEffectiveConfig(projectSlug)`

如果后端 endpoint 已实现，前端不要再拼继承链。保留 `setProjectConfig/deleteProjectConfig` 用于写显式 project value。

- [ ] **Step 3: 实现列表**

列：

- key + label
- type
- source badge
- current value（secret 遮掩）
- status（已覆盖/继承/默认/必填缺失/只读）
- actions

用 `ConfigValueControl` 做编辑。编辑 project value 时仅允许 `allowed_scopes` 包含 project 的 key。

- [ ] **Step 4: 添加值 dialog**

Key selector 分组：

- Project 可写：`allowed_scopes.includes("project")` 且当前无 `project_value`。
- Workspace-only：disabled，展示说明。

保存调用 `setProjectConfig(workspaceSlug, projectSlug, key, value)`，成功 invalidate effective query。

- [ ] **Step 5: 恢复继承**

对有 `project_value` 的行显示“恢复继承”：

```ts
deleteProjectConfig(workspaceSlug, projectSlug, key)
```

成功后行仍存在，source 变成 workspace/default/missing。

- [ ] **Step 6: 跑测试**

Run:

```bash
pnpm --dir web test -- project-config
pnpm --dir web typecheck
```

Expected: PASS。

- [ ] **Step 7: Commit**

```bash
git add web/src/pages/project-config-tab.tsx web/src/features/workspace/config web/src/locales/zh-CN.ts web/src/locales/en-US.ts
git commit -m "feat: 项目配置页展示有效值"
```

### Task 11: 首页配置概览

**Files:**

- Modify: `web/src/pages/OverviewPage.tsx`
- Add/Modify: `web/src/pages/OverviewPage.test.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`

- [ ] **Step 1: 写失败测试**

在 `OverviewPage.test.tsx` 中 mock `/api/v1/config/effective?console_home=true`，断言：

- 只渲染响应中的 rows。
- 第一列主文本展示 `definition.label`，不是 raw key。
- raw key 只作为辅助文本展示；当 label 为空时 fallback 展示 raw key。
- secret value `••••••` 按原样显示。
- missing 显示“未配置”。
- 有“去配置定义”按钮/链接到 `/settings`。

Run:

```bash
pnpm --dir web test -- OverviewPage.test.tsx
```

Expected: FAIL。

- [ ] **Step 2: 增加 query**

在 `OverviewPage.tsx` 增加：

```ts
const homeConfig = useQuery({
  queryKey: ["overview", "config-effective", "console-home"],
  queryFn: () => listWorkspaceEffectiveConfig({ consoleHome: true }),
})
```

不要阻塞现有 metrics/audit/delivery 渲染；失败时只在配置概览 section 内显示错误。

- [ ] **Step 3: 增加只读 section**

放在 actor/scope cards 后、metrics 前或 metrics 后均可；推荐 metrics 后，避免首页顶端过重。

UI：

- title: 配置概览
- action: 去配置定义
- table: 配置/value/source，其中“配置”列主文本是 `definition.label`，raw key 用等宽小字作为辅助信息。
- empty: 没有标记为首页展示的配置

不要加大 hero，不要加说明性长文案。

- [ ] **Step 4: 跑测试**

Run:

```bash
pnpm --dir web test -- OverviewPage.test.tsx
pnpm --dir web typecheck
```

Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/OverviewPage.tsx web/src/pages/OverviewPage.test.tsx web/src/locales/zh-CN.ts web/src/locales/en-US.ts
git commit -m "feat: 首页展示配置概览"
```

### Chunk 4 验证

Run:

```bash
pnpm --dir web test -- project-config OverviewPage
pnpm --dir web typecheck
pnpm --dir web lint
```

Expected: PASS。

## Chunk 5: 文档同步、全量验证与收尾

### Task 12: 文档同步

**Files:**

- Modify: `README.md`
- Modify: `docs/manual/web-console.md`
- Modify: `docs/manual/reference/errors.md`
- Modify if needed: `docs/manual/remote-cli-and-api.md`

- [ ] **Step 1: README 更新**

更新 Web Console 设置页说明：

- `/settings` 是配置定义管理，不再说它展示 workspace config value。
- project settings 下有 config/definitions/notes。
- project config value 读取顺序：project > workspace > default。
- `show_on_console_home` 只影响首页展示，不是 required。

- [ ] **Step 2: 错误码文档**

`docs/manual/reference/errors.md` 增加：

- `config_definition_type_locked`
- `config_definition_scope_locked`
- `config_definition_enum_locked`

- [ ] **Step 3: 跑文档 diff check**

Run:

```bash
git diff --check
```

Expected: PASS。

- [ ] **Step 4: Commit**

```bash
git add README.md docs/manual/web-console.md docs/manual/reference/errors.md docs/manual/remote-cli-and-api.md
git commit -m "docs: 更新配置定义控制台说明"
```

### Task 13: 全量验证

**Files:** no code changes unless failures require fixes.

- [ ] **Step 1: 后端全量测试**

Run:

```bash
go test ./...
```

Expected: PASS。

- [ ] **Step 2: 零 CGO 测试**

Run:

```bash
CGO_ENABLED=0 go test ./...
```

Expected: PASS。

- [ ] **Step 3: 零 CGO build**

Run:

```bash
CGO_ENABLED=0 go build ./cmd/xuanchu
```

Expected: PASS。

- [ ] **Step 4: 前端验证**

Run:

```bash
pnpm --dir web typecheck
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web build
```

Expected: PASS。

- [ ] **Step 5: 编辑烟测**

因为本任务改 project settings 和 shared controls，跑：

```bash
pnpm --dir web run smoke:editing
```

Expected: PASS。如果失败且与本次页面无关，记录证据并判断是否已有环境问题；不要直接声称全量通过。

- [ ] **Step 6: 最终 diff 检查**

Run:

```bash
git diff --check
git status --short
```

Expected: `git diff --check` PASS；`git status` 只包含本任务相关文件或已提交干净。

## Risk Notes

- `/api/v1/config/effective` 与 `/api/v1/config/{key}` 路由可能冲突。实现时先用测试锁定；若冲突不可控，改路径前必须同步 spec 和 plan。
- secret 值在 project settings 需要可编辑，但首页必须遮掩。不要把 app 层统一遮掩成无法编辑的值。
- `ConfigDefinition` update 锁定规则会收紧后端行为；测试要覆盖错误码，避免前端只靠 disabled 控件守规则。
- project definitions 页不是 project 私有 schema。删除定义会影响整个 workspace，确认弹窗必须写清楚。
- `/settings` 从 value 列表改为 definition 管理是 Web Console 语义修正；不要删除 `/api/v1/config` API。

## Final Verification Commands

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
pnpm --dir web typecheck
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web build
pnpm --dir web run smoke:editing
git diff --check
```
