# Workspace 自定义字段与 Task UDA 设计

**日期：** 2026-07-23

**状态：** 待实现

**范围：** Workspace `UDADefinition`、Task / TaskSeries UDA value、Web Console 字段管理与填写体验，以及 HTTP / MCP 的缺口补齐。

## 1. 结论

自定义字段只保留两层：

```text
Workspace UDADefinition（字段定义）
        │
        │ type / values / default 约束
        ▼
Task / TaskSeries UDA value（具体值）
```

Project 只提供 Task / TaskSeries 的业务归属，不保存 UDA 可用范围，不复制 definition，也不决定哪些字段可写。

固定决策：

1. Workspace definition 与 Task / TaskSeries value 不是同一个东西。
2. Workspace Config 也不等于 Workspace UDA；`uda.*` 只是现有 Config 入口对 `UDADefinition` 的兼容视图。
3. Workspace 内所有 Project 共用同一套 effective definitions；无 Project Task 也使用同一套 definitions。
4. 不新增 `ProjectUDASettings`、`ProjectUDAField`、Project UDA API、Project UDA MCP tool 或 Project 自定义字段设置页。
5. Project Template 继续使用现有严格 Snapshot v1，只保存被选 Task / TaskSeries 的 UDA blueprint，不保存 Workspace definition，不引入 v2。
6. 模板实例化时按当前 Workspace definition 校验 UDA；缺失或不兼容时由现有 Preview issue 阻断，不自动创建或复制 definition。
7. 不新增 `uda_list`、`uda_set`、`uda_get_usage`；Workspace definition 继续可由 `config_list/get/set/unset` 管理。
8. MCP 只补齐现有 `task_add` 缺少的 `udas` 输入。
9. Web Console 新增 Workspace 自定义字段管理页；Task / Series 表单使用“已选择字段 + 添加自定义字段”减少噪声，不靠 Project allowlist 控制显示。
10. definition default 只作为 schema 元数据和输入提示，不自动写入 Task / Series。

## 2. 为什么不增加 Project UDA 设置

Project allowlist 能减少表单中的字段数量，但会引入一个长期存在的第三层状态：

```text
Workspace definition
  → Project mode / selected names / order / version
    → Task / Series value
```

这会进一步要求：

- 两张新表和跨数据库迁移；
- Project settings 的权限、并发版本和审计；
- Task move 后历史值的 unavailable 状态；
- active Series 阻止 Project 停用字段；
- HTTP / Remote / CLI / MCP / Web 的 Project 子资源；
- Project Template 新 Snapshot schema、source hash 和实例化恢复顺序；
- 旧 Project 缺省模式和回滚语义。

这些复杂度主要在维护“哪些字段被展示”，而不是维护业务数据。当前可以用更轻的表单交互解决：只展示已填写或用户主动选择的字段，其余 definition 放在可搜索的“添加自定义字段”入口中。

因此本规格明确不把 UI 降噪问题转化成 Project 持久化模型。

## 3. 现有代码与数据结构

### 3.1 `UDADefinition`：Workspace 字段定义

当前 storage model：

```go
type UDADefinition struct {
    WorkspaceID  string
    Name         string
    Type         string
    Label        string
    ValuesJSON   string
    DefaultValue string
    CreatedAt    int64
    ModifiedAt   int64
}
```

领域结构 `internal/uda.Definition`：

```go
type Definition struct {
    Name          string
    Type          Type // string | numeric | date | duration
    Label         string
    Values        []string
    Default       string
    OrphanAllowed bool
}
```

definition 只描述字段 schema，不保存任何 Task 的值。`values` 非空表示该类型下的枚举选项；`enum` 不是第五种 UDA type。

### 3.2 `TaskUDAValue`：普通 Task 的值

```go
type TaskUDAValue struct {
    WorkspaceID string
    TaskUUID    string
    Name        string
    Value       string
    ValueType   string
    Orphan      bool
}
```

普通 Task import 在 definition 不存在时会把值以 `Orphan=true` 保存。删除一个已有 definition 不会批量改写 `task_uda_values`，因此原值会成为 definition-missing 历史值，但其持久化 `Orphan` flag 不一定改变。两者在普通交互中都不能按当前 schema 编辑；import / bundle restore 继续负责保真。

### 3.3 `TaskSeriesUDAValue`：循环任务系列的值

TaskSeries 在 domain 中继续使用 `map[string]string`，storage 使用 `TaskSeriesUDAValue`。不要为了 Web 展示伪造一套尚不存在的 Series orphan/type 模型。

active Series 会继续生成 occurrence，因此 definition 删除或不兼容修改必须先保护 active Series；ended / stopped Series 不阻止 schema 生命周期操作。

### 3.4 Project 与 UDA 的边界

Project 当前没有 UDA settings、UDA relation 或 UDA override。这个现状保持不变：

```text
Project
  └─ Task / TaskSeries 归属

Workspace UDADefinition
  └─ 约束 Workspace 内所有 Task / TaskSeries 的 UDA value
```

Task 移动 Project 不改变 UDA 的可写性，也不产生 unavailable 状态。

### 3.5 `uda.*` Config 兼容视图

现有入口：

```text
uda.<name>.type
uda.<name>.label
uda.<name>.values
uda.<name>.default
```

这些 key 由 `internal/app/uda.go` 路由到 `UDARepository`，不是 `configs` row，也不是 `ConfigDefinition`：

```text
config set uda.estimate.type numeric
  → SetConfig
  → setUDAConfigLocked
  → UDARepository.UpsertDefinition
  → uda_definitions
```

`ConfigDefinition` 管理通用 Config key 的 scope、secret、required 等属性，与 UDA definition 无关。Web Console 不得复用 `ConfigDefinitionsPage` 管理 UDA。

### 3.6 DB 与 runtime definitions

`ListUDAs()` 的真实语义：

1. 读取 DB definitions；
2. 合并 runtime definitions；
3. 同名时 DB 覆盖 runtime；
4. 按 name 稳定排序。

所有结构化列表、Task / Series 表单和模板校验都必须基于 `ListUDAs()` 或同一 effective 语义，不能只查 `uda_definitions` 表。

runtime-only definition 是只读来源：

- structured list 标记 `runtime` / `database` / `database_override`；
- PUT 同名 definition 是显式创建 DB override，不修改 runtime config；
- DELETE runtime-only definition 返回 `uda_runtime_readonly`；
- 删除 DB override 后重新显露同名 runtime definition。

## 4. 用户旅程

### 4.1 Workspace 管理员定义字段

```text
Workspace 设置
  → 自定义字段
  → 新建 / 编辑 definition
  → 设置 name、type、label、values、default
  → 保存
```

owner/admin 可以管理；member/viewer 只读。页面显示 Task value、active Series value 等 usage，用于解释修改或删除的影响。

### 4.2 Task 协作者填写字段

```text
创建或编辑 Task
  → 看到当前已选择/已填写的自定义字段
  → 点击“添加自定义字段”
  → 搜索 Workspace definitions
  → 选择字段并填写
  → 服务端按 definition 归一化和校验
```

Task 属于哪个 Project 不影响候选 definitions。

### 4.3 TaskSeries 协作者填写字段

TaskSeries 使用同一字段选择器和 typed 控件。Series 保存的值会按现有规则同步/物化到 occurrence；本规格不改变 recurrence override 语义。

### 4.4 保存模板与根据模板创建项目

```text
选择 Task / TaskSeries
  → 现有 capture 校验其 UDA 有当前 definition 且不是 orphan
  → 写入 xuanchu.project-template-snapshot/v1

实例化 Preview
  → 按当前 Workspace definition 校验 blueprint type/value
  → 缺失或不兼容：project_template_uda_invalid，阻止创建
  → 校验通过：按现有事务创建 Project / Series / Task
```

模板流程没有 Project UDA settings，也不复制 Workspace definition。

### 4.5 Agent 与脚本

Agent 使用既有 `config_*` 管理/读取 Workspace definition，使用 Task / Series tools 写值。`task_add` 补齐 `udas` 后，add/get/query/modify/clear 形成完整闭环。

## 5. 目标与非目标

### 5.1 目标

1. 明确 Workspace definition 与 Task / Series value 的两层模型。
2. 提供结构化 Workspace UDA HTTP resource，Web 不再解析扁平 config 猜 schema。
3. 修复 definition default 与 `uda.*` 兼容入口校验不一致。
4. 保护 active Series 不被 definition 删除或不兼容修改破坏。
5. 让 Task / Series 表单按真实 type 编辑，并以字段选择器减少噪声。
6. Task 详情显示已填写值、可添加的 definition 和 definition-missing 历史值。
7. 补齐 MCP `task_add.udas`。
8. 保持 query、urgency、import、bundle、runtime UDA 和 Project Template v1 行为稳定。

### 5.2 非目标

本规格不做：

- Project UDA allowlist、mode、排序、版本或 override；
- Project UDA storage / API / CLI / Remote / MCP / Web 页面；
- Project Template Snapshot v2；
- 在模板中复制 Workspace definition；
- definition 的 `description`、`required`、`default_enabled`；
- default 自动写入 Task / Series；
- Project Header 或 Project Settings 信息架构调整；
- UDA query、urgency 公式或 Taskwarrior JSON 格式调整；
- 单独的 Workspace `uda_list` / `uda_set` MCP tools。

## 6. 信息架构与 ASCII 原型

### 6.1 Workspace 自定义字段页

固定路由：

```text
/workspaces/{workspaceSlug}/settings/custom-fields
```

Workspace settings nav 增加“自定义字段”；现有 `/settings` ConfigDefinition 页面和 `/settings/project-templates` 保持不变。

```text
Workspace 设置 / 自定义字段

自定义字段                                      [新建字段]
这些字段可用于 Workspace 内的所有任务和循环任务。

┌──────────┬──────────┬────────┬──────────┬──────────────┬────────┐
│ 名称     │ 显示名称 │ 类型   │ 默认值   │ 使用情况     │ 操作   │
├──────────┼──────────┼────────┼──────────┼──────────────┼────────┤
│ channel  │ 渠道     │ string │          │ 41 个任务    │ 编辑 … │
│ estimate │ 工作量   │ numeric│ 3        │ 18 / 2系列   │ 编辑 … │
│ source   │ 来源     │ string │          │ 运行时提供   │ 创建覆盖│
└──────────┴──────────┴────────┴──────────┴──────────────┴────────┘
```

### 6.2 Task 创建/编辑

```text
创建任务

标题       [________________________________]
项目       [demo ▾]

自定义字段
  渠道     [search ▾]                              [移除]
  工作量   [________]
           提示：默认值 3                          [移除]

  [+ 添加自定义字段]
      ┌────────────────────────────┐
      │ 搜索字段 [______________] │
      │ □ 截止类型                │
      │ □ 外部单号                │
      └────────────────────────────┘

                                      [取消] [创建]
```

Project 选择只决定任务归属，不过滤字段候选。

### 6.3 Task 详情

```text
自定义字段
  渠道       search                         [编辑]
  工作量     3                              [编辑]
  [+ 添加自定义字段]

历史字段
  legacy_id  EXT-123                 未定义，只读
```

definition-missing 历史值不猜类型，也不允许普通交互修改。

### 6.4 Project 页面不增加 UDA 设置

```text
Project Header / Project Settings
  ├─ 保持现有 Project 功能
  └─ 不增加“项目可用字段”或“自定义字段范围”

Workspace Settings
  └─ 自定义字段（唯一 definition 管理入口）
```

## 7. Workspace definition 生命周期

### 7.1 definition 形状保持现状

```text
name     稳定逻辑 key
type     string | numeric | date | duration
label    展示名称
values   可选 enum；保留用户顺序
default  可选输入提示
```

不新增 required 或 Project 级覆盖。

### 7.2 统一归一化与校验

当前 `DefineUDA` 会归一化 default，但 `SetConfig("uda.<name>.default", ...)` 可以绕过同等校验。所有写入口必须收敛到同一个 App helper：

```text
normalizeAndValidateUDADefinition
  → 校验 name / reserved name / type
  → values 逐项按 type canonicalize
  → 按首次出现顺序去重
  → default 按最终 type / values canonicalize 与校验
```

结构化 HTTP 与 `config set/unset uda.*` 必须调用同一写用例。

### 7.3 default

default 只用于：

- Workspace 字段管理页展示；
- Task / Series 表单 placeholder 或提示；
- API / Template schema 解释。

它不用于：

- 自动写入 Task / Series；
- query / urgency 的隐式值；
- 模板实例化时补值。

### 7.4 type / values 修改

修改后先验证 existing default。active Series 中该 name 的值若与新 schema 不兼容，返回：

```text
uda_active_series_incompatible
```

普通 Task 历史值不被批量改写。

### 7.5 删除

删除 DB definition：

- active Series 仍保存该 name 时返回 `uda_active_series_in_use`；
- 不级联删除普通 Task 或 ended / stopped Series value；
- 普通 Task 已有值按现有 import/orphan 兼容语义保留；
- `config unset uda.<name>.type` 走同一 App 删除路径；
- runtime-only definition 返回 `uda_runtime_readonly`；
- 删除 DB override 后同名 runtime definition 重新生效。

## 8. Task、TaskSeries 与 occurrence

### 8.1 普通 Task 写入

普通 add / modify 保持现有规则：

1. 非空 `udas[name]` 必须有 Workspace effective definition；
2. value 按 definition type / values 归一化；
3. 空字符串按现有语义删除值；
4. orphan 继续返回 `uda_orphan_readonly`；
5. Project 变化不参与 UDA 校验。

### 8.2 TaskSeries

Series add / modify 继续调用同一 Workspace definition 校验。active Series 的 schema 生命周期保护放在 definition 写用例中，不增加 Project 规则。

### 8.3 occurrence

- projected occurrence 继承 Series UDA；
- materialized occurrence 继续按现有规则保存 Task UDA；
- occurrence override 与 Series 同步语义不改变；
- 不产生 Project unavailable 状态。

### 8.4 Web typed 控件

```text
string    text input
numeric   decimal input
date      date input，提交时按现有 API 格式转换
duration  第一版 text input，并显示支持的单位
values 非空  select，优先于上述自由输入
```

表单只把用户实际填写的非空值放进 payload。选择字段但不填写不会写入 default 或空 value。

### 8.5 Task 详情合并

详情页数据来源：

```text
Workspace effective definitions
        +
Task 已保存 UDA values
```

- 已保存且 definition 存在：按 label/type 显示为 typed editor 候选；若该 row 是 import 持久化的 orphan，App 仍以 `uda_orphan_readonly` 为最终边界；
- definition 存在但未保存：不铺满详情，通过“添加自定义字段”选择；
- 已保存但 definition 不存在：无论持久化 `Orphan` flag 是否为 true，都进入“历史字段”，按 raw value 只读显示；
- 当前 Task HTTP DTO 不暴露 orphan flag，Web 不伪造该信息；收到 `uda_orphan_readonly` 时保留用户 draft 并显示服务端错误。

## 9. Query、urgency、helper 与导入导出

### 9.1 Query 与 urgency

Project 不再参与 UDA schema，因此现有 query compiler 和 urgency 继续使用 Workspace definitions 与实际保存值。不得因为 UI 没选中某字段而隐藏、忽略或改变已保存值的贡献。

### 9.2 CLI helper

现有 `uda.<name>.values` helper 和 config 兼容输出不改。结构化 HTTP 是 Web 的 typed resource，不要求新增同义 CLI helper。

### 9.3 普通 import

现有 `/api/v1/task-imports` 保持迁移语义：

- 已定义字段按 definition 归一化；
- 未定义字段允许作为普通 Task orphan 保存；
- 不增加 Project unavailable warning，因为不存在 Project 字段范围。

### 9.4 `xuanchu.task-bundle/v1`

bundle 保持严格保真，不新增 Workspace definition 或 Project UDA settings。CLI / MCP / Remote 的 bundle import/export contract 不变。

### 9.5 导出

Taskwarrior JSON、HTTP、MCP、Remote 继续输出真实保存的 UDA value。Project 不过滤值。

## 10. Project Template：保持 Snapshot v1

### 10.1 不新增 v2

现有契约保持：

```go
const SnapshotSchemaV1 = "xuanchu.project-template-snapshot/v1"

type TaskBlueprintV1 struct {
    UDAs map[string]UDABlueprintV1 `json:"udas,omitempty"`
}

type SeriesBlueprintV1 struct {
    UDAs map[string]UDABlueprintV1 `json:"udas,omitempty"`
}
```

不修改 `ProjectBlueprintV1`，不增加 `uda_settings`，不新增 `EncodeV2` / `SnapshotV2`，也不改写已有 JSON/hash。

### 10.2 Capture

当前 `captureUDADefinitions` 已覆盖本规格需要的边界：

- 收集被选 Task / Series 实际使用的 UDA names；
- name 必须有当前 effective definition；
- 普通 Task orphan 阻止 capture；
- value 必须能按 definition 归一化；
- Snapshot 保存 raw/type blueprint，不复制 definition。

只补回归测试，不增加 Project settings source hash 或 Wizard 组件。

### 10.3 Instantiate

当前 `validateInstantiateUDAs` 继续：

- definition 缺失：`project_template_uda_invalid`；
- blueprint type 与当前 definition 不兼容：阻断；
- raw value 与当前 type/values 不兼容：阻断；
- 校验通过后把 normalized value 交给现有 Series / Task 创建事务。

实例化顺序保持现状：

```text
Project → Config → Series → Task → links / automation
```

没有 Project UDA settings 恢复阶段。

## 11. HTTP API

### 11.1 Workspace definitions（新增 typed resource）

```text
GET    /api/v1/udas
PUT    /api/v1/udas/{name}
DELETE /api/v1/udas/{name}
```

响应：

```json
{
  "name": "estimate",
  "type": "numeric",
  "label": "工作量",
  "values": ["1", "2", "3", "5", "8"],
  "default": "3",
  "source": "database",
  "task_value_count": 128,
  "active_series_value_count": 2
}
```

规则：

- URL name 是权威 key，body 不得覆盖；
- effective list 包含 DB + runtime，DB 同名覆盖 runtime；
- usage 用 bounded aggregate query，handler 不直接查 storage；
- runtime-only DELETE 返回 409 `uda_runtime_readonly`；
- route tag 使用 `Custom Fields`，不混入 `Config Schema`。

### 11.2 不新增 Project UDA API

明确不存在：

```text
/api/v1/projects/{projectRef}/udas
```

Task / Series endpoint 的 `udas` / `clear_udas` payload 不改名。

### 11.3 错误码

| code | HTTP | 含义 |
|---|---:|---|
| `uda_not_defined` | 422 | 写入 name 没有 Workspace definition |
| `uda_value_invalid` | 422 | value 不符合 type/values |
| `uda_orphan_readonly` | 422 | 普通交互试图修改 orphan |
| `uda_active_series_in_use` | 409 | 删除 definition 会破坏 active Series |
| `uda_active_series_incompatible` | 409 | schema 更新与 active Series value 不兼容 |
| `uda_runtime_readonly` | 409 | runtime-only definition 没有可删除 DB row |
| `project_template_uda_invalid` | 422 | 模板 UDA 缺失或与当前 definition 不兼容 |

## 12. CLI、Remote 与 MCP

### 12.1 Workspace definition

CLI / Remote 不新增平行 Workspace UDA resource；继续复用现有 config contract：

```bash
xuanchu config set uda.estimate.type numeric
xuanchu config set uda.estimate.label 工作量
xuanchu config set uda.estimate.values 1,2,3,5,8
xuanchu config set uda.estimate.default 3
```

Web 使用 typed HTTP resource，避免解析扁平 config。

### 12.2 MCP 不新增同义 UDA tools

保留：

```text
config_list
config_get
config_set
config_unset
```

不新增：

```text
uda_list
uda_set
uda_get_usage
project_list_udas
project_uda_set
```

### 12.3 补齐 `task_add.udas`

```go
type TaskAddInput struct {
    // existing fields...
    UDAs map[string]string `json:"udas,omitempty"`
}
```

tool handler 把它传入现有 `app.AddInput.UDAs`。`task_get` / `task_query` / `task_modify` / clear 语义保持现状。

## 13. 权限与审计

| 操作 | App permission | 用户角色 | tenant capability |
|---|---|---|---|
| 读取 Workspace UDA | `PermissionWorkspaceRead` | owner/admin/member/viewer | `config:read` |
| 修改 Workspace UDA | `PermissionUDAManage` | owner/admin | `config:write` |
| Task / Series 写值 | `PermissionTaskWrite` | owner/admin/member | `task:write` |

浏览器 session 由 membership role 决定；PAT / Agent / tenant token 同时受 capability 和既有 Project allowlist 约束。Project allowlist 只限制可访问的 Task / Series，不改变 UDA definitions。

沿用审计动作：

```text
uda.schema.set
uda.schema.delete
task.add
task.modify
task.series.add
task.series.modify
project_template.snapshot.create
project_template.instantiate
```

不新增 `project.uda.set`。

## 14. 迁移与兼容

### 14.1 数据库

不新增表，不修改：

```text
uda_definitions
task_uda_values
task_series_uda_values
projects
project_templates
project_template_snapshots
```

因此没有 Project UDA migration、旧 Project 回填或 settings rollback。

### 14.2 旧客户端

- Task add/modify UDA payload 不变；
- `uda.*` Config keys 继续可用；
- query / urgency / export 不变；
- import / bundle 保真边界不变；
- Template Snapshot v1 JSON/hash 不变；
- 新 Web typed API 是增量能力，不要求旧客户端升级。

### 14.3 runtime

runtime definitions 仍由进程配置提供。结构化 Web 管理不能假装修改 runtime source；DB override 行为必须明确展示。

## 15. 测试与验收

### 15.1 Storage / App

- `DefineUDA` 与 `SetConfig(uda.*)` 使用同一完整 definition 校验；
- enum values canonicalize、稳定去重，default 与最终 enum 一致；
- active Series 阻止 definition 删除和不兼容修改；
- ended / stopped Series 不阻止；
- runtime-only 删除错误与 DB override 恢复；
- ordinary Task orphan / import 行为不回归；
- Task move Project 不改变 UDA value 或可写性；
- 不存在 Project UDA storage / resolver。

### 15.2 HTTP / MCP

- Workspace UDA typed CRUD、source、usage、权限和 OpenAPI；
- config 兼容入口与 typed resource 观察到同一 DB definition；
- MCP `task_add` schema 包含 `udas` 并能 get/query/modify/clear round trip；
- default tool list 不存在任何 `uda_*` 或 `project_*_udas` 同义工具。

### 15.3 Web

- `/settings/custom-fields` 可发现且不复用 ConfigDefinition 页面；
- owner/admin 可写，member/viewer 只读；
- Task / Series 使用字段选择器，不一次铺开所有未填写 definitions；
- 四种 type、values select 和 default hint 正确；
- Task 详情区分 defined value 与 definition-missing 历史值，并正确显示 `uda_orphan_readonly`；
- Project Header / Settings 不出现 UDA 可用范围。

### 15.4 Template v1

- capture 带 UDA 的 Task / Series 仍写 v1；
- orphan 或无 definition 阻止 capture；
- instantiate definition 缺失、type/value 不兼容时 Preview 阻断；
- 成功实例化后 Task / Series UDA 一致；
- 没有 v2 fixture、`uda_settings` 或 Project UDA 恢复阶段。

### 15.5 完整验证命令

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

### 15.6 产品验收场景

1. owner 在 Workspace 设置新建 `channel` 和 `estimate`。
2. 任意 Project 的 Task 表单都能通过“添加自定义字段”选择这两个字段。
3. 表单未选择/未填写的 definitions 不进入 payload。
4. default 只显示提示，不自动落库。
5. Task 移动到另一 Project 后 UDA 保持可读、可写。
6. active Series 使用字段时不能删除或改成不兼容 schema。
7. MCP `task_add` 写入 UDA，get/query/modify/clear 完成 round trip。
8. MCP 工具列表没有 `uda_list`、`project_list_udas` 或 `project_uda_set`。
9. 带 UDA 的 Task / Series capture 后仍是 Snapshot v1，实例化成功。
10. definition 删除后，模板实例化 Preview 阻断且不产生半成品 Project。
11. Project 页面没有 UDA 可用范围设置。

## 16. 实施边界

1. 先统一 definition App 语义，再提供 typed HTTP 和 Web 页面。
2. Web 字段选择器只解决展示/填写，不保存 Project 级偏好。
3. Template 只补回归测试；若实现需要修改 Snapshot schema，必须停止并重新审阅设计。
4. 不为“以后可能需要”预留 Project UDA 表、mode 字段或隐藏 API。
5. README 解释 definition/value 两层模型、`uda.*` compatibility view 和 MCP `task_add.udas`。
6. ROADMAP 只在实现和全量验证完成后更新状态。
