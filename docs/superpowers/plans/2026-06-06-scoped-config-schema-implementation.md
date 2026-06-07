# Scoped Config Schema Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 Xuanchu 增加按 workspace 隔离的 shared scoped config schema 机制，让 `workspace` / `project` 两层共享配置都通过 schema 驱动，而不是依赖 project config 白名单。

**Architecture:** 复用现有 `configs` 值表，新增 `config_definitions` 作为 schema 定义表；App 层新增 schema service 和 scoped config service，统一处理 workspace/project value 的校验、回退、权限和审计；CLI / HTTP / MCP / remote CLI 在现有 config/project config 路径上增量接入 schema 读写能力，不重写 task UDA、本机启动配置或 query/DOM/urgency。

**Tech Stack:** Go 1.25, GORM, SQLite (`github.com/glebarez/sqlite`), PostgreSQL (`gorm.io/driver/postgres`), Cobra, chi, MCP Go SDK

---

## Chunk 1: Storage 与领域边界

### Task 1: 为 scoped config schema 建立 storage model 和 repo

**Files:**
- Modify: `internal/storage/models.go`
- Create: `internal/storage/config_definition_repo.go`
- Modify: `internal/storage/config_repo.go`
- Test: `internal/storage/config_repo_test.go`
- Test: `internal/storage/db_test.go`
- Test: `internal/storage/postgres_test.go`

- [ ] **Step 1: 写 storage 层 failing test，定义 `config_definitions` 的最小能力**

在 `internal/storage/config_repo_test.go` 添加测试，覆盖：

- workspace 级 definition CRUD
- `(workspace_id, key)` 唯一约束
- `allowed_scopes_json` / `enum_values_json` 往返
- `has_default` / `required` / `secret` 布尔语义
- 同名 key 在不同 workspace 可并存

示例测试骨架：

```go
func TestConfigDefinitionRepositoryCRUD(t *testing.T) {
	store := openTestStore(t)
	repo := NewConfigDefinitionRepository(store.DB())

	def := ConfigDefinition{
		WorkspaceID:       "ws-a",
		Key:               "ads.roi_threshold",
		ValueType:         "number",
		AllowedScopesJSON: `["workspace","project"]`,
		DefaultValue:      "1.8",
		HasDefault:        true,
	}

	if err := repo.Set(def); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	got, ok, err := repo.Get("ws-a", "ads.roi_threshold")
	if err != nil || !ok {
		t.Fatalf("Get() = %v, %v, %v", got, ok, err)
	}
	if got.ValueType != "number" || got.DefaultValue != "1.8" || !got.HasDefault {
		t.Fatalf("Get() = %#v", got)
	}
}
```

- [ ] **Step 2: 运行 storage test，确认当前缺少 schema 实现而失败**

Run:

```bash
go test ./internal/storage -run 'ConfigDefinition|ConfigRepo' -count=1
```

Expected:

- FAIL，报缺少 `ConfigDefinition` model / repository / migration 相关错误

- [ ] **Step 3: 在 `models.go` 增加 `ConfigDefinition` GORM model**

在 `internal/storage/models.go` 中新增结构体，字段与 spec 对齐：

- `WorkspaceID string`
- `Key string`
- `ValueType string`
- `AllowedScopesJSON string`
- `Label string`
- `Description string`
- `EnumValuesJSON string`
- `DefaultValue string`
- `HasDefault bool`
- `Required bool`
- `Secret bool`
- `CreatedAt int64`
- `ModifiedAt int64`

约束要求：

- 复合主键：`workspace_id + key`
- 不引入数据库方言专用 tag
- 布尔字段沿用 GORM 统一映射，由 SQLite / PostgreSQL 各自落地

- [ ] **Step 4: 新建 `config_definition_repo.go`，实现 definition CRUD**

实现至少这些方法：

- `Get(workspaceID, key string) (ConfigDefinition, bool, error)`
- `Set(def ConfigDefinition) error`
- `Delete(workspaceID, key string) error`
- `List(workspaceID string) ([]ConfigDefinition, error)`
- `ListKeysInUse(workspaceID, key string) (workspaceCount int64, projectCount int64, err error)` 或等价接口

要求：

- 所有查询显式带 `workspace_id`
- 不允许 repo 层自动做跨 workspace 兜底
- `Set` 使用 upsert / on conflict update

- [ ] **Step 5: 扩展 `config_repo.go`，为后续“按 key 统计值引用”预留辅助方法**

新增最小辅助方法，例如：

- `CountScopeValues(workspaceID string, scope ConfigScope, key string) (int64, error)`
- `CountAllValuesByKey(workspaceID, key string) (workspaceCount int64, projectCount int64, err error)`

不要在这一步修改现有 `Get/Set/Unset/ListScope` 语义。

- [ ] **Step 6: 更新 SQLite / PostgreSQL 迁移覆盖**

确保：

- SQLite 路径会建出 `config_definitions`
- PostgreSQL 路径 `AutoMigrate` 会建出 `config_definitions`
- 不引入 SQLite-only 手写 DDL

测试补充：

- `internal/storage/db_test.go` 断言新表存在、主键正确
- `internal/storage/postgres_test.go` 在有 `XUANCHU_TEST_DB_URL` 时断言新表可建、CRUD 可跑

- [ ] **Step 7: 运行通过 storage 验证**

Run:

```bash
go test ./internal/storage -run 'ConfigDefinition|ConfigRepo|Postgres' -count=1
CGO_ENABLED=0 go test ./internal/storage -run 'ConfigDefinition|ConfigRepo' -count=1
```

Expected:

- SQLite 下全部 PASS
- PostgreSQL 环境存在时相关测试 PASS；无环境时明确 skip

- [ ] **Step 8: Commit**

```bash
git add internal/storage/models.go internal/storage/config_definition_repo.go internal/storage/config_repo.go internal/storage/config_repo_test.go internal/storage/db_test.go internal/storage/postgres_test.go
git commit -m "feat: 增加 scoped config schema 存储"
```

### Task 2: 在 app 层建立 schema 定义、校验与 value 解析的基础类型

**Files:**
- Create: `internal/app/config_schema.go`
- Create: `internal/app/scoped_config.go`
- Modify: `internal/app/service.go`
- Test: `internal/app/service_test.go`

- [ ] **Step 1: 写 app 层 failing test，锁定 schema/value 领域语义**

在 `internal/app/service_test.go` 添加测试，至少覆盖：

- schema 定义需要 workspace
- `allowed_scopes` 必须包含 workspace 或 project
- `project` 读取链：project value > workspace value > schema default
- 不存在 definition 时，workspace/project value 写入失败
- project 不能读写不允许 project scope 的 key
- schema 和 value 都不能跨 workspace

示例测试骨架：

```go
func TestScopedConfigProjectReadFallsBackWorkspaceThenDefault(t *testing.T) {
	svc := openOwnerService(t)
	project := mustAddProject(t, svc, "product-a")

	if err := svc.ConfigSchemaSet(app.ConfigSchemaInput{
		Key:           "ads.roi_threshold",
		ValueType:     "number",
		AllowedScopes: []string{"workspace", "project"},
		DefaultValue:  ptr("1.8"),
	}); err != nil {
		t.Fatalf("ConfigSchemaSet() error = %v", err)
	}

	value, source, ok, err := svc.ProjectConfigResolve(project.ID, "ads.roi_threshold")
	if err != nil || !ok || value != "1.8" || source != "default" {
		t.Fatalf("ProjectConfigResolve() = %q, %q, %v, %v", value, source, ok, err)
	}
}
```

- [ ] **Step 2: 运行 app 测试，确认当前失败**

Run:

```bash
go test ./internal/app -run 'ScopedConfig|ConfigSchema|ProjectConfig' -count=1
```

Expected:

- FAIL，报缺少 schema service / read chain / validation 接口

- [ ] **Step 3: 在 `config_schema.go` 定义 app 层 schema 输入输出与校验**

至少定义：

- `ConfigValueType`
- `ConfigAllowedScope`
- `ConfigDefinitionView`
- `ConfigSchemaInput`
- 解析 / 校验函数：
  - key 合法性
  - type 合法性
  - allowed scopes 合法性
  - enum / default / secret 组合校验

要求：

- 不依赖 CLI / HTTP 层
- 明确把 workspace 作为 schema definition 的所属边界

- [ ] **Step 4: 在 `scoped_config.go` 实现 shared config 的核心读写语义**

实现最小接口：

- `ConfigSchemaSet(input ConfigSchemaInput) error`
- `ConfigSchemaGet(key string) (ConfigDefinitionView, bool, error)`
- `ConfigSchemaList() ([]ConfigDefinitionView, error)`
- `ConfigSchemaDelete(key string, purge bool) error`
- `WorkspaceConfigGetResolved(key string) (value string, source string, ok bool, err error)`
- `ProjectConfigGetResolved(projectRef, key string) (value string, source string, ok bool, err error)`

value 写入校验必须包含：

- definition 必须存在
- scope 必须允许
- value 必须通过类型校验
- value 必须通过 enum 校验

- [ ] **Step 5: 在 `service.go` 或相关装配点注入新的 repo 和 service 依赖**

要求：

- 不破坏现有 service 构造和测试装配
- 保持 workspace 隔离链条完整
- 不把 schema 逻辑散落回 CLI / HTTP 层

- [ ] **Step 6: 运行 app 测试通过**

Run:

```bash
go test ./internal/app -run 'ScopedConfig|ConfigSchema|ProjectConfig' -count=1
CGO_ENABLED=0 go test ./internal/app -run 'ScopedConfig|ConfigSchema|ProjectConfig' -count=1
```

Expected:

- PASS

- [ ] **Step 7: Commit**

```bash
git add internal/app/config_schema.go internal/app/scoped_config.go internal/app/service.go internal/app/service_test.go
git commit -m "feat: 增加 scoped config schema 服务"
```

---

## Chunk 2: 现有 config / project config 路径切换到 schema 驱动

### Task 3: workspace config 读写改成 schema 驱动，但保持本机配置边界不变

**Files:**
- Modify: `internal/app/service.go`
- Modify: `internal/httpapi/context_config.go`
- Modify: `internal/cli/config.go`
- Modify: `internal/mcpserver/tools_config.go`
- Test: `internal/app/service_test.go`
- Test: `internal/httpapi/auth_test.go`
- Test: `tests/integration/cli_test.go`

- [ ] **Step 1: 写 failing test，锁定 workspace shared config 的新规则**

补测试覆盖：

- `config get/set/unset/list` 只处理 shared workspace config，不再偷偷接纳无 schema key
- 本机配置（如 `database.path`）仍不通过 HTTP / MCP 暴露
- HTTP `config` endpoint 继续拒绝 local-only config
- MCP `config_*` workspace 路径对无 schema key 返回稳定错误

- [ ] **Step 2: 运行相关测试确认失败**

Run:

```bash
go test ./internal/app ./internal/httpapi ./internal/mcpserver -run 'Config|ScopedConfig' -count=1
```

Expected:

- FAIL，当前实现仍按旧的 business config key / agent key 逻辑运行

- [ ] **Step 3: 在 app 层收敛 `GetConfig/SetConfig/UnsetConfig/ConfigValues`**

要求：

- 继续区分 local/runtime config 与 shared workspace config
- shared workspace config 的合法性改由 schema definition 决定
- `ConfigValues()` 明确只返回 workspace shared config 显式值，不混入 project config 和本机 config

- [ ] **Step 4: 更新 HTTP `handleConfig*` 逻辑**

在 `internal/httpapi/context_config.go` 中：

- 去掉依赖旧 `isHTTPBusinessConfigKey` 的核心合法性判断
- 保留“local config 不走 HTTP”的边界
- 把 key 校验交给 app 层 schema 驱动逻辑

不要在 handler 层重复写类型/枚举校验。

- [ ] **Step 4.1: 在这一步先锁定 list 语义，不允许实现时漂移**

把以下行为先写进测试并保持一致：

- `config list`：只列 workspace scope 的**显式值**
- 不自动混入 schema default
- 不自动混入 project scope 值
- 如果后续要提供“effective values”视图，必须另开接口或命令，不在本次实现里偷偷修改 `list` 的含义

- [ ] **Step 5: 更新 CLI `config` 命令行为**

在 `internal/cli/config.go` 中：

- 本地模式：
  - `show` 继续展示本机/运行时关键配置
  - `config get/set/unset/list` 对 shared workspace config 走 app service
- remote 模式：
  - 仍通过 HTTP 走 workspace shared config

不要让 `config list` 把 runtime 本机配置和 shared config 混在一起作为“同一种对象”展示。

- [ ] **Step 5.1: 更新 CLI 集成测试，锁定本机配置与 shared config 的分离**

在 `tests/integration/cli_test.go` 增加或修改测试，覆盖：

- 本地 `show` 继续显示 runtime/local config
- `config list` 只列 shared workspace config
- 未定义 schema 的 workspace config 写入失败
- remote `config list/get/set/unset` 仍工作，但只针对 shared config

- [ ] **Step 6: 更新 MCP `config_*` tool**

在 `internal/mcpserver/tools_config.go` 中：

- workspace `config_get/set/unset/list` 改为 schema 驱动
- project scope 逻辑先保持在现有 `project config` tool 下，不把 project scope 强塞回 `config_*`
- 继续保持 HTTP MCP 不暴露 local config

- [ ] **Step 7: 运行通过相关测试**

Run:

```bash
go test ./internal/app ./internal/httpapi ./internal/mcpserver -run 'Config|ScopedConfig' -count=1
CGO_ENABLED=0 go test ./internal/app ./internal/httpapi ./internal/mcpserver -run 'Config|ScopedConfig' -count=1
```

Expected:

- PASS

- [ ] **Step 8: Commit**

```bash
git add internal/app/service.go internal/httpapi/context_config.go internal/cli/config.go internal/mcpserver/tools_config.go internal/app/service_test.go internal/httpapi/auth_test.go tests/integration/cli_test.go
git commit -m "feat: workspace config 改为 schema 驱动"
```

### Task 4: project config 读写改成 schema 驱动，移除白名单主逻辑

**Files:**
- Modify: `internal/app/project_config.go`
- Modify: `internal/cli/project.go`
- Modify: `internal/httpapi/projects.go`
- Modify: `internal/mcpserver/tools_project.go`
- Modify: `internal/mcpserver/tools_config.go`
- Modify: `internal/mcpserver/resources.go`
- Modify: `internal/remote/config.go`
- Test: `internal/app/service_test.go`
- Test: `internal/httpapi/auth_test.go`
- Test: `tests/integration/cli_test.go`
- Test: `internal/mcpserver/auth_test.go`
- Test: `internal/mcpserver/resources_test.go`

- [ ] **Step 1: 写 failing test，锁定 project config 新规则**

补测试覆盖：

- `project config set/get/unset/list` 对任意已定义且允许 `project` scope 的 key 工作
- 未定义 key 返回 `config_definition_not_found`
- 定义存在但不允许 project scope 返回 `config_scope_not_allowed`
- `project config` 读取链支持 project value > workspace value > default
- project / workspace 不匹配时继续返回稳定 scope 错误

- [ ] **Step 2: 运行测试确认当前失败**

Run:

```bash
go test ./internal/app ./internal/httpapi ./internal/mcpserver -run 'ProjectConfig|ScopedConfig' -count=1
```

Expected:

- FAIL，当前 `projectConfigKeys` 白名单限制无法满足动态 schema

- [ ] **Step 3: 收敛 `project_config.go` 为薄封装**

要求：

- 移除 `projectConfigKeys` 作为最终合法性判断
- 保留 `project config requires project scope` 这一类用户侧错误语义
- 将 key 合法性与 value 校验统一交给 scoped config service
- project 归档检查仍保留

- [ ] **Step 4: 更新 CLI / HTTP / MCP / remote project config 入口**

分别调整：

- `internal/cli/project.go`
- `internal/httpapi/projects.go`
- `internal/mcpserver/tools_project.go`
- `internal/remote/config.go`
- `internal/mcpserver/resources.go`
- `tests/integration/cli_test.go`

要求：

- 不改变用户入口形态
- 不引入跨 workspace project config 操作
- 不在 transport 层复制 schema 校验逻辑
- remote CLI 集成测试必须覆盖新的 project > workspace > default 回退链
- `resources.go` 中 project resource 的 agent config 摘要也必须切到 schema 驱动读取，不能继续依赖旧的固定 key 假设

- [ ] **Step 4.1: 锁定 `project config list` 的语义**

把以下行为先写进测试并保持一致：

- `project config list`：只列 project scope 的**显式 project 值**
- 不自动展开 workspace fallback
- 不自动展开 schema default
- “effective project config” 如果未来需要，必须单独设计，不在本次实现中隐式改变现有 list 语义

- [ ] **Step 5: 运行通过 project config 测试**

Run:

```bash
go test ./internal/app ./internal/httpapi ./internal/mcpserver -run 'ProjectConfig|ScopedConfig|Resource' -count=1
CGO_ENABLED=0 go test ./internal/app ./internal/httpapi ./internal/mcpserver -run 'ProjectConfig|ScopedConfig|Resource' -count=1
```

Expected:

- PASS

- [ ] **Step 6: Commit**

```bash
git add internal/app/project_config.go internal/cli/project.go internal/httpapi/projects.go internal/mcpserver/tools_project.go internal/mcpserver/tools_config.go internal/mcpserver/resources.go internal/remote/config.go internal/app/service_test.go internal/httpapi/auth_test.go internal/mcpserver/auth_test.go internal/mcpserver/resources_test.go tests/integration/cli_test.go
git commit -m "feat: project config 改为 schema 驱动"
```

---

## Chunk 3: schema 管理接口、权限与 secret 语义

### Task 5: 增加 schema 管理的 app 权限、审计与删除规则

**Files:**
- Modify: `internal/app/service.go`
- Modify: `internal/app/workspace.go`
- Modify: `internal/app/config_schema.go`
- Test: `internal/app/service_test.go`

- [ ] **Step 1: 写 failing test，锁定 schema 权限和删除语义**

补测试覆盖：

- viewer 可读 schema，不可写
- admin / owner 可写 schema
- 删除仍被值引用的 schema 默认失败
- `purge=true` 时允许删除 schema 及其所有值
- `secret=true` 的 key 审计不记明文 value

- [ ] **Step 2: 运行测试确认失败**

Run:

```bash
go test ./internal/app -run 'ConfigSchema|Secret|Audit' -count=1
```

Expected:

- FAIL，缺少 schema 权限和 purge 规则

- [ ] **Step 3: 增加权限常量与角色映射**

在 app 权限体系中新增：

- `PermissionConfigSchemaRead`
- `PermissionConfigSchemaWrite`

角色建议：

- viewer / member：可读
- admin / owner：可写

要求：

- 不破坏既有 `PermissionWorkspaceRead`、`PermissionProjectConfigWrite` 等语义

- [ ] **Step 4: 在 schema service 中实现删除规则与 secret 审计规则**

要求：

- 删除 schema 默认检查值引用
- `purge` 才允许级联删除 workspace/project 对应值
- `secret=true` key 的 set/unset 审计不记录 value 明文

- [ ] **Step 5: 运行通过 app 权限/审计测试**

Run:

```bash
go test ./internal/app -run 'ConfigSchema|Secret|Audit' -count=1
CGO_ENABLED=0 go test ./internal/app -run 'ConfigSchema|Secret|Audit' -count=1
```

Expected:

- PASS

- [ ] **Step 6: Commit**

```bash
git add internal/app/service.go internal/app/workspace.go internal/app/config_schema.go internal/app/service_test.go
git commit -m "feat: 增加 config schema 权限与审计"
```

### Task 6: 增加 CLI / HTTP / MCP 的 config schema 管理入口

**Files:**
- Modify: `internal/cli/config.go`
- Modify: `internal/remote/config.go`
- Modify: `internal/httpapi/router.go`
- Modify: `internal/httpapi/context_config.go`
- Modify: `internal/mcpserver/tools_config.go`
- Modify: `internal/mcpserver/testdata/config_get.schema.json`
- Create: `internal/mcpserver/testdata/config_schema_list.schema.json`
- Create: `internal/mcpserver/testdata/config_schema_get.schema.json`
- Create: `internal/mcpserver/testdata/config_schema_set.schema.json`
- Create: `internal/mcpserver/testdata/config_schema_delete.schema.json`
- Test: `internal/mcpserver/resources_test.go`
- Test: `tests/integration/cli_test.go`

- [ ] **Step 1: 写 failing tests，锁定 transport 形态**

补测试覆盖：

- CLI `config schema list/get/set/delete`
- HTTP `config-schema` endpoints
- MCP `config_schema_*` tools schema 和业务行为

- [ ] **Step 2: 运行测试确认失败**

Run:

```bash
go test ./tests/integration ./internal/httpapi ./internal/mcpserver -run 'ConfigSchema|CLI|MCP' -count=1
```

Expected:

- FAIL，缺少命令、路由和 tool

- [ ] **Step 3: 在 CLI 中增加 `config schema` 子命令组**

在 `internal/cli/config.go` 中新增：

- `config schema list`
- `config schema get <key>`
- `config schema set <key> ...`
- `config schema delete <key> [--purge]`

要求：

- 保持现有 `config` 命令结构清晰，不把 schema 逻辑和 value 逻辑写在一大坨函数里
- 参数错误给出明确提示，不在 CLI 层复刻所有 app 校验

- [ ] **Step 3.1: 在 `internal/remote/config.go` 中补齐 schema 的 remote client 方法**

至少新增：

- `ListConfigSchema(ctx, workspace string) ([]app.ConfigDefinitionView, error)` 或等价 DTO
- `GetConfigSchema(ctx, workspace, key string) (...)`
- `SetConfigSchema(ctx, workspace, key string, input ...) error`
- `DeleteConfigSchema(ctx, workspace, key string, purge bool) error`

不要让 CLI remote mode 通过手拼 HTTP 请求绕过 remote client。

- [ ] **Step 4: 在 HTTP 中新增 schema endpoint**

在 `router.go` 和 `context_config.go` 中新增：

- `GET /api/v1/config-schema`
- `GET /api/v1/config-schema/{key}`
- `PUT /api/v1/config-schema/{key}`
- `DELETE /api/v1/config-schema/{key}`

要求：

- 必须走 authMiddleware
- 必须尊重 effective workspace
- 不允许无 workspace 写 schema

- [ ] **Step 5: 在 MCP 中新增 `config_schema_*` tools 和 schema golden**

在 `internal/mcpserver/tools_config.go` 中新增：

- `config_schema_list`
- `config_schema_get`
- `config_schema_set`
- `config_schema_delete`

并更新 `internal/mcpserver/testdata/` 对应 golden schema。

- [ ] **Step 6: 运行通过 transport 测试**

Run:

```bash
go test ./tests/integration ./internal/httpapi ./internal/mcpserver -run 'ConfigSchema|CLI|MCP' -count=1
CGO_ENABLED=0 go test ./tests/integration ./internal/httpapi ./internal/mcpserver -run 'ConfigSchema|CLI|MCP' -count=1
```

Expected:

- PASS
- remote CLI 在 HTTP 模式下可完整执行 `config schema` 生命周期

- [ ] **Step 7: Commit**

```bash
git add internal/cli/config.go internal/remote/config.go internal/httpapi/router.go internal/httpapi/context_config.go internal/mcpserver/tools_config.go internal/mcpserver/testdata tests/integration/cli_test.go internal/mcpserver/resources_test.go
git commit -m "feat: 暴露 config schema 管理接口"
```

---

## Chunk 4: 双数据库验证、文档收口与全量验收

### Task 7: 完成 SQLite / PostgreSQL 行为一致性测试

**Files:**
- Modify: `internal/storage/postgres_test.go`
- Modify: `internal/app/service_test.go`
- Modify: `tests/integration/cli_test.go`

- [ ] **Step 1: 写 PostgreSQL 专项覆盖**

增加测试覆盖：

- `config_definitions` 在 PostgreSQL 下可建、可 CRUD
- workspace/project config schema/value 读写链在 PostgreSQL 下成立
- project > workspace > default 回退在 PostgreSQL 下成立
- `secret` / `purge` 行为在 PostgreSQL 下语义一致

- [ ] **Step 2: 运行数据库专项测试**

Run:

```bash
go test ./internal/storage ./internal/app -run 'ConfigSchema|ScopedConfig|Postgres' -count=1
```

Expected:

- SQLite PASS
- PostgreSQL 环境存在时 PASS；无环境时相关测试 skip

- [ ] **Step 3: 运行 CLI 集成测试双后端验证**

Run:

```bash
go test ./tests/integration -run 'ConfigSchema|ProjectConfig|Config' -count=1
XUANCHU_TEST_DB_URL='postgres://user:pass@localhost:5432/xuanchu_test?sslmode=disable' go test ./tests/integration -run 'ConfigSchema|ProjectConfig|Config' -count=1
```

Expected:

- SQLite PASS
- PostgreSQL 环境存在时 PASS

- [ ] **Step 4: Commit**

```bash
git add internal/storage/postgres_test.go internal/app/service_test.go tests/integration/cli_test.go
git commit -m "test: 验证 scoped config 双数据库一致性"
```

### Task 8: 更新文档并跑全量验收

**Files:**
- Modify: `README.md`
- Modify: `ROADMAP.md`
- Modify: `docs/manual/team-workspaces-projects.md`
- Modify: `docs/manual/reference/commands.md`
- Modify: `docs/manual/reference/errors.md`

- [ ] **Step 1: 更新文档，反映 schema 驱动 shared config**

文档应覆盖：

- `config schema` 命令
- shared config 与 UDA 的边界
- workspace/project 读取链
- workspace 隔离硬约束
- SQLite / PostgreSQL 都支持此能力

- [ ] **Step 2: 运行文档和格式检查**

Run:

```bash
git diff --check
```

Expected:

- 无 trailing whitespace / patch 格式问题

- [ ] **Step 3: 运行全量验收**

Run:

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```

Expected:

- 全部 PASS

- [ ] **Step 4: Commit**

```bash
git add README.md ROADMAP.md docs/manual/team-workspaces-projects.md docs/manual/reference/commands.md docs/manual/reference/errors.md
git commit -m "docs: 更新 scoped config schema 文档"
```

---

Plan complete and saved to `docs/superpowers/plans/2026-06-06-scoped-config-schema-implementation.md`. Ready to execute?
