# Workspace 自定义字段 Implementation Plan

> **对应 spec：** [2026-07-23-workspace-custom-fields-design.md](../specs/2026-07-23-workspace-custom-fields-design.md)
>
> 本计划按依赖顺序拆成 7 个可独立验证、可独立提交的任务。Project 不保存 UDA 可用范围；如果实现需要 Project UDA 表、Project UDA API 或 Template Snapshot v2，必须停止并重新审阅 spec。

**Goal：** 保持 Workspace `UDADefinition` → Task / TaskSeries UDA value 两层模型，统一 definition 校验，提供 Web 所需 typed HTTP resource，用字段选择器降低表单噪声，并补齐 MCP `task_add.udas`。

**Architecture：** `internal/uda` 负责 definition/value 归一化；`internal/storage` 保留现有 definition/value 表并补 usage 查询；`internal/app` 统一 DB/runtime definition 生命周期与 active Series 保护；HTTP 提供 Workspace typed resource；Web 从 Workspace definition 构建 typed 控件；Project Template 保持严格 Snapshot v1 和现有 UDA 校验。

**Tech Stack：** Go 1.25、GORM、`github.com/glebarez/sqlite`、`gorm.io/driver/postgres`、Huma/OpenAPI、MCP Go SDK、React 19、TypeScript 6、TanStack Query/Router、Vitest、Playwright。

## 全局约束

- 只有 Workspace definition 与 Task / TaskSeries value 两层；Project 只提供业务归属。
- 不新增 `ProjectUDASettings`、`ProjectUDAField` 或任何 Project UDA migration。
- 不新增 `/api/v1/projects/{projectRef}/udas`、Project UDA CLI/Remote/MCP/Web。
- 不新增 Template Snapshot v2、`uda_settings`、Project UDA source hash 或实例化恢复阶段。
- 现有 `SnapshotV1` / `EncodeV1` / JSON / hash 保持不变。
- `uda.*` 是 Config compatibility view，不是 `configs` row 或 `ConfigDefinition`。
- effective definitions 必须保留 `ListUDAs()` 的 runtime + DB 合并以及 DB 同名覆盖 runtime。
- definition 仍只有 `name/type/label/values/default`；default 不自动写入 Task / Series。
- 普通 Task / Series 按 Workspace effective definition 校验，Task 移动 Project 不改变 UDA 语义。
- import / bundle restore 继续保真；不增加 Project unavailable warning。
- 不新增 `uda_list`、`uda_set`、`uda_get_usage`、`project_list_udas`、`project_uda_set`。
- MCP 只补现有 `task_add.udas`。
- SQLite/PostgreSQL 与 `CGO_ENABLED=0` 必须继续通过。
- 文档与注释使用中文；协议字段和错误码保持稳定。
- 每个任务先补失败测试，再实现，再执行本任务验收命令。

## 依赖关系

```text
1 definition 语义/usage/Series 保护
├──→ 2 Workspace typed HTTP/OpenAPI
│      └──→ 4 Web 字段库/API/路由
│              └──→ 5 Task/Series 表单与详情
└──→ 3 MCP task_add.udas

1 ─→ 6 Template v1 与导入兼容回归

全部 ─→ 7 golden/E2E/README/ROADMAP/全量验证
```

## 固定共享接口

实现时可微调私有函数名，但对外语义保持以下形状。

```go
// internal/app/uda.go
type WorkspaceUDAFieldView struct {
    Name                   string   `json:"name"`
    Type                   string   `json:"type"`
    Label                  string   `json:"label"`
    Values                 []string `json:"values"`
    Default                string   `json:"default"`
    Source                 string   `json:"source"` // database|runtime|database_override
    TaskValueCount         int64    `json:"task_value_count"`
    ActiveSeriesValueCount int64    `json:"active_series_value_count"`
}

type WorkspaceUDAInput struct {
    Type    string   `json:"type"`
    Label   string   `json:"label"`
    Values  []string `json:"values"`
    Default string   `json:"default"`
}

func (s *Service) WorkspaceListUDAs() ([]WorkspaceUDAFieldView, error)
func (s *Service) WorkspaceSetUDA(name string, input WorkspaceUDAInput) (WorkspaceUDAFieldView, error)
func (s *Service) WorkspaceDeleteUDA(name string) error
```

```text
GET    /api/v1/udas
PUT    /api/v1/udas/{name}
DELETE /api/v1/udas/{name}
```

明确不定义 Project UDA DTO、mode、version、selected names 或 Template v2 struct。

---

## 任务 1：统一 Workspace definition 生命周期、usage 与 active Series 保护

**目标：** 让 `DefineUDA`、结构化 API 和 `config set/unset uda.*` 共享同一完整校验，同时保护 active Series。

**改动文件：**

- `internal/uda/schema.go`、`schema_test.go`。
- `internal/storage/uda_repo.go`、`uda_repo_test.go`（新建或补充）。
- `internal/storage/task_series_repo.go`、`task_series_repo_test.go`。
- `internal/app/uda.go`、`service_test.go` 或 `uda_test.go`（新建）。
- `internal/app/permission.go`、`permission_test.go`。

**先写测试：**

- [ ] `DefineUDA` 与 `SetConfig(uda.*)` 对 type/values/default 得到相同归一化和错误。
- [ ] string/numeric/date/duration enum values 按 type canonicalize，按首次出现顺序去重。
- [ ] default 使用最终 canonical values 校验，不能保存“已归一化但不属于 enum”的状态。
- [ ] type/values 修改会重新校验 existing default。
- [ ] active Series value 与新 type/values 不兼容时返回 `uda_active_series_incompatible`。
- [ ] active Series 使用字段时删除返回 `uda_active_series_in_use`。
- [ ] ended / stopped Series 不阻止删除或 schema 修改。
- [ ] 删除 definition 不删除普通 Task 或 ended / stopped Series value。
- [ ] `config unset uda.<name>.type` 不能绕过同一保护。
- [ ] runtime-only definition 删除返回 `uda_runtime_readonly`。
- [ ] PUT runtime-only 同名 definition 创建 DB override；删除 override 后 runtime definition 重新成为 effective。
- [ ] usage 一次聚合返回 Task value count 与 active Series value count，Workspace 隔离正确。
- [ ] tenant actor 持有 `config:write` 时 `PermissionUDAManage` 通过，缺 capability 时拒绝。

**实现步骤：**

- [ ] 提取 `normalizeAndValidateUDADefinition(def)`：校验 name/reserved/type，canonicalize+stable-dedupe values，最后校验 default。
- [ ] `DefineUDA`、`setUDAConfigLocked`、`unsetUDAConfigLocked` 和后续 typed HTTP 写用例进入同一 App 路径。
- [ ] UDA repo 增加区分 DB row 是否存在的方法，不能从 effective definition 推断可删除性。
- [ ] TaskSeries repo 增加按 Workspace/name 查询 active Series values 的 bounded query。
- [ ] type/values 更新只扫描 active Series；不批量改写普通 Task 历史值。
- [ ] 删除检查、definition 写入/删除和 audit 放在同一 transaction。
- [ ] 增加 definition usage aggregate；禁止逐 definition 逐 Task 查询。
- [ ] `WorkspaceListUDAs` 合并 DB/runtime，计算 `database/runtime/database_override` source，并填充零值 usage。
- [ ] `tenantCapabilityForPermission(PermissionUDAManage)` 映射 `auth.ScopeConfigWrite`。

**明确不做：**

- 不查询或写入 Project UDA relation；
- 不增加 Project unavailable/active-Series 停用规则；
- 不改变 Task move Project 行为。

**验收标准：** definition 所有写入口行为一致，active scheduler 不会被 schema 变更破坏，runtime 来源和 usage 可被 typed view 准确表达。

**测试命令：**

```bash
go test ./internal/uda ./internal/storage ./internal/app -run 'UDA|TaskSeries|Permission'
CGO_ENABLED=0 go test ./internal/uda ./internal/storage ./internal/app
```

**建议提交：** `feat: 统一 Workspace 自定义字段语义`

---

## 任务 2：Workspace typed HTTP API 与 OpenAPI

**依赖：** 任务 1。

**目标：** 为 Web 提供结构化 definition resource，不再解析扁平 Config 猜 schema。

**改动文件：**

- `internal/httpapi/udas.go`、`udas_test.go`（新建）。
- `internal/httpapi/huma_routes.go`。
- `internal/httpapi/error.go` 或现有错误映射。
- OpenAPI/golden 相关测试文件。

**路由：**

```text
GET    /api/v1/udas
PUT    /api/v1/udas/{name}
DELETE /api/v1/udas/{name}
```

**先写测试：**

- [ ] GET 返回 DB + runtime effective definitions、source、usage，空 `values` 稳定为 `[]`。
- [ ] typed PUT 与 `config_get/list` 观察到同一 DB definition。
- [ ] `config set uda.*` 后 typed GET 观察到相同 type/label/values/default。
- [ ] URL name 是权威 key，body name 不能覆盖。
- [ ] runtime-only DELETE 映射 409 `uda_runtime_readonly`。
- [ ] active Series 的 in-use/incompatible 错误映射 409，definition/value 错误映射 422。
- [ ] member/viewer 可读；owner/admin 可写；tenant token 分别要求 `config:read` / `config:write`。
- [ ] OpenAPI request/response/error schema 与真实 handler 一致。
- [ ] 路由表中不存在 `/api/v1/projects/{projectRef}/udas`。

**实现步骤：**

- [ ] handler 只解析 request、调用任务 1 App 用例、序列化 view，不直接查询 storage。
- [ ] Workspace GET 走 `config:read + PermissionWorkspaceRead` 请求授权；App view 不再额外错误要求 tenant actor 具备 `workspace:read`。
- [ ] PUT/DELETE 统一进入 `PermissionUDAManage`，不复制 Config compatibility 的字段拼装逻辑。
- [ ] route tag 使用 `Custom Fields`，不混入 `Config Schema`。
- [ ] 注册 `uda_active_series_in_use`、`uda_active_series_incompatible`、`uda_runtime_readonly`。

**验收标准：** Web 可以只依赖 typed resource 完成 Workspace definition 管理，兼容 config 入口仍观察到同一数据。

**测试命令：**

```bash
go test ./internal/httpapi -run 'UDA|OpenAPI|Route'
CGO_ENABLED=0 go test ./internal/httpapi
```

**建议提交：** `feat: 新增 Workspace 自定义字段 HTTP 接口`

---

## 任务 3：补齐 MCP `task_add.udas`

**依赖：** 任务 1。

**目标：** 消除 MCP Task add 的真实缺口，不制造 Workspace/Project UDA 同义资源工具。

**改动文件：**

- `internal/mcpserver/tools_task.go`、`tools_task_test.go` 或 integration tests。
- `internal/mcpserver/schema_test.go`。
- `internal/mcpserver/testdata/task_add.schema.json`。
- `internal/mcpserver/testdata/list-tools-default.json`。

**先写测试：**

- [ ] `TaskAddInput` schema 包含 `udas: object<string,string>`。
- [ ] `task_add` 把 UDA 写入现有 `app.AddInput.UDAs`。
- [ ] `task_get` / `task_query` 能读回；`task_modify` / clear 既有语义不回归。
- [ ] undefined name 返回现有 `uda_not_defined`；invalid value 返回 `uda_value_invalid`。
- [ ] Project allowlist 仍限制 Task 访问，但不改变 UDA definition 集合。
- [ ] 默认工具列表明确不存在 `uda_list`、`uda_set`、`uda_get_usage`、`project_list_udas`、`project_uda_set`。

**实现步骤：**

- [ ] 在 `TaskAddInput` 增加 `UDAs map[string]string`。
- [ ] handler 构造 `app.AddInput` 时原样传入 UDAs，由 App 做 normalization/validation。
- [ ] 更新 task_add schema golden 和 default tool list。
- [ ] 不增加 Workspace UDA Remote/CLI/MCP client；config tools 已覆盖 definition 管理。

**测试命令：**

```bash
go test ./internal/mcpserver -run 'TaskAdd|TaskGet|TaskQuery|TaskModify|Schema|ToolList'
CGO_ENABLED=0 go test ./internal/mcpserver
```

**建议提交：** `feat: 补齐 MCP 创建任务自定义字段`

---

## 任务 4：Workspace 字段库 Web API、路由与管理页

**依赖：** 任务 2。

**目标：** 新增唯一的 definition 管理页面；不进入 Project Header/Settings。

**改动文件：**

- `web/src/features/workspace/custom-fields/custom-field-api.ts`、tests（新建）。
- `web/src/features/workspace/custom-fields/workspace-custom-fields-page.tsx`、tests（新建）。
- `web/src/features/workspace/custom-fields/custom-field-dialog.tsx`、tests（新建）。
- `web/src/routes/workspace/WorkspaceCustomFieldsRoute.tsx`（新建）。
- `web/src/routes/router.tsx`。
- `web/src/features/workspace/project-templates/workspace-settings-nav.tsx`、tests。
- 中英文 locale。

**固定路由：**

```text
/workspaces/$workspaceSlug/settings/custom-fields
```

**先写测试：**

- [ ] typed API URL 携带 effective workspace，解析四种 type、values/default/source/usage。
- [ ] Workspace settings nav 可发现“自定义字段”，active state 正确。
- [ ] `/settings` 仍是 ConfigDefinition 页面；新页面不复用 `ConfigDefinitionsPage`。
- [ ] owner/admin 可新建编辑删除 DB definition；member/viewer 只读。
- [ ] name/type/values/default validation 错误保留 dialog draft。
- [ ] runtime-only row 标注“运行时提供”，不显示删除；编辑说明创建 Workspace override。
- [ ] DB override 删除确认说明恢复 runtime definition。
- [ ] active Series in-use/incompatible typed error 显示明确中文。
- [ ] 删除普通 DB definition 的确认说明 Task 历史值不会被级联删除。

**实现步骤：**

- [ ] 建 TanStack Query keys/hooks，mutation 成功后失效 Workspace definitions 及 Task/Series form/detail queries。
- [ ] route 挂在现有 `workspaceRootRoute`，链接保留 `/workspaces/$workspaceSlug`。
- [ ] `WorkspaceSettingsNav` 增加 `customFields`，不改 Project settings tabs。
- [ ] source/usage 只用于解释动作；允许与否以服务端错误为准。
- [ ] 普通用户文案使用“自定义字段”，不显示 UDA。

**明确不改：**

- `project-header-editor.tsx`；
- `project-settings-layout.tsx`；
- Project settings routes/tabs；
- Project Template Wizard。

**测试命令：**

```bash
pnpm --dir web test -- custom-field workspace-settings-nav
pnpm --dir web typecheck
pnpm --dir web lint
```

**建议提交：** `feat: 实现 Workspace 自定义字段管理页`

---

## 任务 5：Task/Series 字段选择器、typed form 与详情

**依赖：** 任务 4。

**目标：** 所有 Project 共用 Workspace definitions，通过“已选字段 + 添加字段”降低表单噪声，而不是持久化 Project allowlist。

**改动文件：**

- `web/src/features/workspace/project-workbench/tasks/task-uda-definitions.ts`、tests。
- `task-common-fields.tsx`、tests。
- 新建可复用 `task-uda-field-picker.tsx`、tests（名称可按现有目录调整）。
- `task-series/task-series-form.tsx`、dialog tests。
- `task-detail/task-property-panel.tsx`、tests。
- `project-readonly/uda.ts`、tests（保留协议兼容 extraction）。
- Task create/edit dialog 调用点与 locale。

**先写测试：**

- [ ] Task / Series 候选始终来自 Workspace typed definitions，与当前 Project 无关。
- [ ] 初始只显示已有值或当前 draft 已选字段，不铺开所有未填写 definitions。
- [ ] “添加自定义字段”支持搜索、键盘选择、排除已选字段。
- [ ] 移除未保存字段不发送 clear；移除已有且 definition 仍存在的值使用现有 clear payload；definition-missing 历史值保持只读。
- [ ] string/numeric/date/duration 四种 type 使用正确控件。
- [ ] 任一 type 的 `values` 非空时使用 select；`enum` 不作为 type。
- [ ] default 只显示 placeholder/hint，不进入 payload。
- [ ] 选择字段但保持空值不写入 UDA。
- [ ] Task 详情合并 Workspace definitions 与 saved values。
- [ ] saved value 无 definition 时，无论持久化 orphan flag 为何都进入“历史字段”并只读。
- [ ] 当前 Task HTTP DTO 不暴露 orphan flag；Web 不根据 raw value 伪造标志，`uda_orphan_readonly` 返回时保留 draft 并显示明确错误。
- [ ] Task 移动 Project 后 UDA UI 和可写性不改变。
- [ ] Workspace definition mutation 后表单/详情 cache 刷新。

**实现步骤：**

- [ ] `TaskUDADefinition` 改由 typed HTTP DTO 构造；删除页面对 `uda.*` Config 的业务解析依赖。
- [ ] `parseTaskUDADefinitions(config)` 仅在仍有兼容消费者时保留；删除前先用 `rg` 确认。
- [ ] `CommonUDAFields` 不接受 Project UDA query/ref；只接受 Workspace、saved values、draft 和 clear callback。
- [ ] 字段选择器复用 Task 与 TaskSeries，同一处实现 filter/keyboard/empty/default 语义。
- [ ] date/duration 提交继续匹配当前 HTTP/App 格式，不在前端发明第二种 wire value。
- [ ] detail 用 name 作为提交 key，label 只做展示；definition-missing raw 不猜 boolean/type。

**验收标准：** 表单噪声明显降低，但任意 Project 仍可选择全部 Workspace definitions；没有 Project UDA state 或 cache key。

**测试命令：**

```bash
pnpm --dir web test -- task-uda task-common-fields task-property-panel task-series-dialog uda
pnpm --dir web typecheck
pnpm --dir web lint
```

**建议提交：** `feat: 优化任务自定义字段填写体验`

---

## 任务 6：Template Snapshot v1、import、query 与 urgency 兼容回归

**依赖：** 任务 1。

**目标：** 用测试证明收缩设计不需要 Template v2 或 Project UDA 语义，现有数据路径保持稳定。

**主要文件：**

- `internal/projecttemplate/codec_test.go`、`model_test.go`。
- `internal/app/project_template_capture_test.go`。
- `internal/app/project_template_instantiate_test.go`。
- `internal/app/service_test.go`、`task_bundle_test.go`、`export_warnings_test.go`。
- 必要的 HTTP/MCP Template 回归测试；生产代码仅在测试暴露真实缺陷时修改。

**先写/补充测试：**

- [ ] 新 capture 仍输出 `xuanchu.project-template-snapshot/v1`。
- [ ] `ProjectBlueprintV1` 不含 `uda_settings`，canonical JSON/hash golden 不变。
- [ ] 带 UDA 的 selected Task / Series capture 保存 raw/type blueprint。
- [ ] orphan、definition 缺失或 value 不兼容阻止 capture，沿用 `project_template_uda_invalid`。
- [ ] instantiate Preview 在 definition 缺失、type/value 不兼容时阻断且不写数据。
- [ ] 成功实例化后 Series / Task UDA 与 Snapshot 一致。
- [ ] 实例化顺序保持 Project → Config → Series → Task，不存在 UDA settings 阶段或 failure hook。
- [ ] 普通 import 仍可恢复 orphan；不返回 Project unavailable warning。
- [ ] `xuanchu.task-bundle/v1` round trip 不增加 definition/settings。
- [ ] Task move Project 保留 UDA，query/urgency/export 结果不因 Project 改变。

**实现纪律：**

- [ ] 不修改 `SnapshotSchemaV1`、`SnapshotV1`、`ProjectBlueprintV1`、`EncodeV1`。
- [ ] 不新增 `SnapshotV2`、`EncodeV2`、normalized UDA settings 或 v2 fixture。
- [ ] 不改 Capture/Instantiate Web DTO 或 Wizard；现有 UDA summary 来自 selected Task/Series 即可。
- [ ] 如果现有测试已经完整覆盖某项，只记录证据，不为了“有代码改动”重写生产代码。

**测试命令：**

```bash
go test ./internal/projecttemplate
go test ./internal/app -run 'ProjectTemplate.*UDA|Import.*UDA|TaskBundle|Urgency|Move'
CGO_ENABLED=0 go test ./internal/projecttemplate ./internal/app
```

**建议提交：** `test: 固化模板与自定义字段兼容边界`

---

## 任务 7：Golden、E2E、文档同步与全量验证

**依赖：** 任务 1-6。

**目标：** 完成 Workspace definition → Task/Series value 两层闭环，并证明没有残留 Project UDA 或 Template v2 契约。

**改动文件：**

- OpenAPI golden/snapshot（按仓库当前方式）。
- `internal/mcpserver/testdata/task_add.schema.json`、`list-tools-default.json`。
- `tests/integration/cli_test.go`（若 config compatibility 需要补充）。
- `web/scripts/playwright-editing-smoke.mjs`、`playwright-task-series-smoke.mjs`。
- `README.md`、`ROADMAP.md`、本 spec/plan 的实现偏差。
- 中英文 Web 文案。

**必须覆盖的端到端场景：**

- [ ] owner 在 Workspace 设置新建 `channel` / `estimate`。
- [ ] 两个不同 Project 的 Task 表单都能选择相同 Workspace definitions。
- [ ] 表单默认不铺开所有未填写字段；选择并填写后 Task 详情可编辑。
- [ ] default 只显示提示，不自动落库。
- [ ] Task move Project 后 UDA 不变。
- [ ] active Series 阻止 definition 删除；清除/停止 Series 后可删除。
- [ ] runtime-only definition 只读，DB override 创建/删除行为清楚。
- [ ] MCP `task_add` 写 UDA，query/get/modify/clear round trip。
- [ ] MCP 工具列表无 UDA/Project UDA 同义工具。
- [ ] Template capture/instantiate 仍使用 v1 并保留 Task/Series UDA。
- [ ] definition 缺失时 Instantiate Preview 阻断，数据库无半成品。
- [ ] Project Header/Settings 没有 UDA 可用范围入口。

**结构性反向检查：**

```bash
rg -n 'ProjectUDASettings|ProjectUDAField|project_list_udas|project_uda_set|SnapshotSchemaV2|EncodeV2|uda_settings' internal web
```

预期：不出现本功能新增实现；若命中其他既有文档/测试 fixture，逐条确认，不机械删除用户已有内容。

**文档同步：**

- [ ] README 解释 definition/value 两层模型和 `uda.*` compatibility view。
- [ ] README 增加 Workspace typed HTTP 示例与 MCP `task_add.udas` 示例。
- [ ] README 明确 Project 不限制 UDA，Template 仍为 Snapshot v1。
- [ ] ROADMAP 只在实现与全量验证通过后标记完成。
- [ ] OpenAPI、MCP schema、Web 帮助与错误文案一致。
- [ ] spec/plan 不残留 inherit_all/custom、Project UDA version 或 Template v2。

**最终验证命令：**

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
pnpm --dir web run smoke:editing
pnpm --dir web run smoke:task-series
pnpm --dir web run smoke:project-template
```

如果 smoke 需要本地 server，按仓库现有脚本启动；结束后关闭进程并确认端口释放。

**最终验收：**

- [ ] 所有命令通过并记录实际输出。
- [ ] 当前分支只包含本功能相关文件。
- [ ] worktree 无临时 DB、Playwright trace、auth cache 或构建二进制。
- [ ] SQLite/PostgreSQL 抽象和 `CGO_ENABLED=0` 有验证证据。
- [ ] 所有协议层复用 App definition/value 规则。
- [ ] runtime UDA、orphan、import、bundle、Template v1 均有回归证据。
- [ ] 代码和文档中没有 Project UDA settings 或 Template v2 半成品。

**建议提交：** `docs: 完成 Workspace 自定义字段文档与验证`

## 执行纪律

1. 任务 1 完成前，不实现 HTTP/Web typed resource。
2. 每个任务先写失败测试并观察预期失败，再实现。
3. Web 字段选择器只能保存 Task/Series draft，不得悄悄保存 Project 偏好。
4. Template 任务以回归证明为主，不为了扩范围修改 Snapshot schema。
5. 每个提交只包含相关文件，不纳入本地数据库、浏览器缓存或构建产物。
6. 发现需要 Project UDA state 时停止，先回到产品问题验证是否真的不能由表单交互解决。
7. 只有任务 7 全量验证通过后，才能把里程碑标记为完成或可合并。
