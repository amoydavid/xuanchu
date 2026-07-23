# 项目自定义字段：Workspace 定义、Project 选择、Task 保存值

> **给 agentic workers 的要求：** 本规格通过后，编码前必须先编写独立 implementation plan。不要直接从本文档开始实现。

**日期：** 2026-07-23

**状态：** 草案

**范围：** Workspace UDA definition、Project 字段可用范围、Task / TaskSeries UDA value、Web Console 设置入口，以及 CLI / HTTP / MCP / Remote Client 的一致语义。

## 1. 结论先行

当前代码中不存在三种彼此等价的 UDA。准确模型是：

```text
Workspace UDADefinition（字段定义）
  ├── name / type / label / values / default
  ├── 通过 uda.<name>.* 暴露为 Config 兼容视图
  └── 不是 configs 行，也不是 ConfigDefinition
                 │
                 │ 本规格新增：Project 决定可写字段范围
                 ▼
Project UDA settings（跟随全部 / 自定义选择）
                 │
                 ▼
TaskUDAValue / TaskSeriesUDAValue（具体值）
```

因此：

1. Task 的 UDA value 和 Workspace 的 UDA definition 不是同一个东西；前者是值，后者是值必须遵守的 schema。
2. Workspace Config 也不等于 Workspace UDA。`uda.*` 只是现有 Config API、CLI、MCP 对 `UDADefinition` 的兼容管理入口。
3. `ConfigDefinition` 是 shared workspace/project config 的 schema，与 Task UDA 无关，不能互相代替。
4. 本规格只在现有两层之间增加 Project 可用范围，不复制 definition，也不改变 Task UDA 的持久化格式。
5. Project 默认处于“跟随 Workspace 全部字段”模式，保持升级前行为；只有切换成“自定义选择”后，不同 Project 才有不同字段集合。
6. Project 页面 Header 增加带文字的“项目设置”入口，并在现有 Project Settings 中增加“自定义字段”页。
7. Web Console 用户文案统一使用“自定义字段”；`UDA` 保留在代码、兼容配置键、CLI 和技术文档中。
8. 不新增同义 MCP 工具 `uda_list`、`uda_set` 或 `uda_get_usage`。Workspace definition 继续由 `config_list/get/set/unset` 管理。
9. Project 范围是新资源，新增 `project_list_udas` 和 `project_uda_set`。
10. 现有 `task_query`、`task_get`、`task_modify` 已覆盖 UDA 读取和修改；只需给现有 `task_add` 补上缺失的 `udas` 输入。
11. Project Template 新 Snapshot 必须保存 Project UDA mode 与 custom ordered names；实例化时先恢复 Project UDA settings，再创建 Series 和 Task。

## 2. 代码与数据结构审计

本节描述 2026-07-23 当前 `main`（`372ddcc`）的事实。后续章节凡标注“新增”才属于本规格要实现的行为。

### 2.1 四类容易混淆的数据

#### A. 通用 Config value

`internal/storage/models.go` 中的 `Config`：

```go
type Config struct {
    WorkspaceID string // 复合主键
    Scope       string // workspace | project
    ScopeID     string
    Key         string
    Value       string
}
```

它保存 shared workspace/project config value，例如 Agent、Automation、集成使用的配置。UDA definition 不以 `Config` row 保存。

#### B. 通用 ConfigDefinition

`ConfigDefinition` 定义通用 Config key 的类型、scope、默认值、secret、required 等属性。Web Console 当前 `/settings` 和 Project Settings 的 `definitions` 页管理的是它。

它不是 Task UDA definition，也不会让 Task JSON 多出可查询字段。

#### C. Workspace UDADefinition

`UDADefinition` 是独立表，主键为 `(workspace_id, name)`：

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

当前 domain 只支持四种类型：

- `string`
- `numeric`
- `date`
- `duration`

当前 definition 字段只有 `name`、`type`、`label`、`values`、`default`。本规格不额外增加 `description`、`required` 或 `default_enabled`，避免让结构化 API 与现有 `uda.*` 兼容入口出现能力差异。

#### D. Task / TaskSeries UDA value

普通 Task 的值保存在 `task_uda_values`：

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

TaskSeries 的值保存在 `task_series_uda_values`：

```go
type TaskSeriesUDAValue struct {
    SeriesID  string
    Name      string
    Value     string
    ValueType string
    Orphan    bool
}
```

需要特别注意：

- `TaskSeriesUDAValue` 当前没有 `WorkspaceID` 列，Workspace 隔离由所属 `TaskSeries` 保证；
- 当前 `taskseries.Series.UDAs` 是 `map[string]string`；repo 实际只读写 `name/value`，没有把 Series 的 `ValueType/Orphan` 映射进 domain；
- 因此不能把普通 Task 的完整 orphan 模型直接套到 TaskSeries 上。

### 2.2 `uda.*` 是兼容视图，不是 Config row

`internal/app/uda.go` 的真实路由是：

```text
SetConfig("uda.estimate.type", "numeric")
  → setUDAConfigLocked
  → UDARepository.UpsertDefinition
  → uda_definitions

GetConfig("uda.estimate.type")
  → getUDAConfig
  → UDARepository.GetDefinition

ConfigValues()
  → 合并普通 config
  → ListUDAs()
  → 平铺为 uda.<name>.type|label|values|default
```

当前兼容键严格只有：

```text
uda.<name>.type
uda.<name>.label
uda.<name>.values
uda.<name>.default
```

`config unset uda.<name>.type` 会删除整条 definition；unset 另外三个字段只清空该属性。

### 2.3 DB definition 与 runtime definition

`ListUDAs()` 会合并：

1. runtime config 中的 `uda.*`；
2. 当前 Workspace 数据库中的 `uda_definitions`；
3. 同名时 DB definition 覆盖 runtime definition。

因此 Project 字段解析必须基于 `ListUDAs()` 的 effective definitions，不能只查 `uda_definitions` 表，否则 runtime UDA 会在 Web 和 Project 校验中消失。

### 2.4 当前值写入行为

`normalizeUDAModifications` 已被普通 Task add/modify、TaskSeries add/modify 和 occurrence 物化复用：

- definition 不存在：`uda_not_defined`；
- 值按 definition type 归一化；
- enum 不匹配：`uda_value_invalid`；
- 空字符串：删除值；
- 普通写入不能修改或清除 orphan：`uda_orphan_readonly`；
- 当前没有 Project 是否启用该字段的检查；
- definition 的 `default` 不会在 App 创建 Task / TaskSeries 时自动写入。

definition 写入本身还有一个现存不一致：`DefineUDA` 会归一化 default，但 `SetConfig("uda.<name>.default", ...)` 当前直接 upsert，`ValidateDefinition` 也不检查 default。因此兼容 Config 入口可能保存与 type/enum 不兼容的 default。本规格的结构化 HTTP resource 与 Config 兼容入口必须收敛到同一个完整 definition 校验函数，修复这处差异。

类型的实际归一化结果是：

| 类型 | 保存格式 |
|---|---|
| `string` | trim 后的单行字符串 |
| `numeric` | 规范数字字符串 |
| `date` | UTC RFC3339 |
| `duration` | 秒数字符串 |

### 2.5 当前导入与 orphan 行为

普通 JSON/XLSX 导入走 `normalizeImportedUDAs`：

- 已定义字段按当前 definition 归一化；
- 未定义字段保留为普通 Task 的 `Orphan=true` 历史值。

原生 `xuanchu.task-bundle/v1` 是跨环境迁移格式，会直接恢复 Task / Series 中携带的 UDA 数据。它不是普通交互式写入接口。

因此本规格必须继续区分：

- 普通 add/modify：严格执行 definition 和 Project 可用范围；
- import/bundle restore：优先保真，允许恢复 orphan 或 Project 当前未启用的历史值。

### 2.6 当前读取、query 与 urgency

- `TaskOccurrenceView`、HTTP Task/Series DTO、Remote Client 和 MCP occurrence 输出使用嵌套 `udas` map；
- Taskwarrior 兼容 `task.JSONTask` 仍把 UDA 平铺到 Task JSON 顶层；
- `task_query` 和 `task_get` 已返回 `udas`；
- UDA value query 需要 Workspace definition；orphan 只支持 `uda.<name>.notnull` 这类存在性查询；
- urgency 直接遍历 Task 已保存的 UDA value，并读取 `urgency.uda.*` 系数，不检查 Project 或 definition 是否启用。

### 2.7 当前接口覆盖

| 入口 | 读取 Task UDA | 新建 Task 写 UDA | 修改 Task UDA | Workspace definition |
|---|---:|---:|---:|---:|
| App | 是 | 是 | 是 | 是 |
| CLI | 是 | 是 | 是 | `config` / `_udas` |
| HTTP | 是 | 是 | 是 | `config` 兼容接口 |
| Remote Client | 是 | 是 | 是 | `config` 兼容接口 |
| MCP | 是 | **否** | 是 | `config_list/get/set/unset` |
| Web Console | 是 | 是 | 仅已有值 | 仅通过 config 列表间接读取 |

MCP 的缺口只在 `TaskAddInput` 没有 `UDAs`，不是缺少一套 UDA 资源工具。

### 2.8 当前 Web Console 断点

1. Task / TaskSeries 表单从 `/api/v1/config` 解析 Workspace 全部 `uda.*`，不区分 Project。
2. Task 详情只遍历当前 Task 已有值；已定义但未填写的字段不显示。
3. Task 详情根据当前 value 猜 boolean/number/date，而不是读取 definition type；但 domain 实际并不支持 boolean UDA。
4. Project Settings 已有 `config`、`definitions`、`notes` 三个 tab，尚无自定义字段 tab。
5. Project Header 的齿轮只打开 slug 编辑 Dialog，不进入完整 Project Settings。
6. `/settings` 当前是通用 ConfigDefinition 页面，不是 UDA definition 页面。

## 3. 要解决的产品问题

当前 Workspace 定义一个 UDA 后，所有 Project 的创建表单都显示它。Workspace 内 Project 一多，字段会越来越杂；用户也无法从当前 Project 判断“这个项目应该用哪些字段”。

问题不在于缺少第三种 UDA，而在于缺少一个 Project 级可用范围：

```text
Workspace：字段是什么意思？
Project：这个项目使用哪些 Workspace 字段？
Task / Series：这个对象的具体值是什么？
```

## 4. 用户旅程

### 4.1 Workspace 管理员定义字段

```text
Workspace 设置
  → 自定义字段
  → 新建字段
  → 填写 name / label / type / values / default
  → 保存
```

管理员不需要理解 `uda.estimate.type` 等兼容键。

### 4.2 Project 管理员从当前项目配置字段

```text
/workspaces/demo/projects/demo
  → 项目设置
  → 自定义字段
  → 选择：跟随 Workspace 全部字段 / 自定义选择
  → 自定义模式下勾选并排序
  → 保存
```

如果所需字段不存在，owner/admin 可从本页创建 Workspace 字段；创建后：

- 跟随全部模式：字段立即生效；
- 自定义选择模式：字段加入当前未保存草稿，由用户确认后保存。

### 4.3 Task 协作者填写值

```text
创建 Task / Series
  → 只显示当前 Project effective fields
  → 空值不落库
  → 非空值由服务端归一化

打开 Task 详情
  → 显示 effective fields，包括尚未填写的字段
  → 额外显示 Task 已保存但 Project 当前未启用的历史值
  → 历史值只允许清除，不允许改成另一个非空值
```

### 4.4 Agent 与脚本

- 使用现有 Task 工具读取和写值；
- 使用现有 Config 工具管理 Workspace definition；
- 使用 Project UDA 工具读取或修改 Project 模式与选择；
- 所有普通写入口由 App 层执行同一规则。

## 5. 目标与非目标

### 5.1 目标

1. Workspace definition、Project 可用范围、Task/Series value 三层职责清晰。
2. 旧 Project 升级后行为不变。
3. Project 可切换为自定义字段集合，并维护显示顺序。
4. 从 Project Header 一步进入设置。
5. Web Task 表单与详情使用结构化 definition，不再猜类型。
6. CLI、HTTP、Remote、MCP、Web 的普通写入共享 App 校验。
7. 保留 import、orphan、query、urgency、Taskwarrior JSON 和 runtime UDA 兼容语义。
8. Project Template 保存并恢复 Project UDA settings，且保持旧 v1 Snapshot 可实例化。

### 5.2 非目标

1. 不新增 Project 私有 definition。
2. 不允许 Project 覆盖 type、label、values 或 default。
3. 不把 UDA 合并进 `ConfigDefinition` / Project Config。
4. 不新增 `description`、`required`、字段级权限、条件显示、公式字段或跨字段校验。
5. 不改变 default 的当前语义，不在创建时自动写默认值。
6. 不因 Project 停用字段而删除、改写或隐藏已有值。
7. 不改变 query 或 urgency 对已保存值的解释。
8. 不把 Workspace UDA definition 复制进 Project Template；模板只保存 Project 对 definition name 的引用。
9. 不新增 `uda_list` / `uda_set` / `uda_get_usage` MCP 工具。

## 6. 产品术语

| 技术术语 | Web Console 文案 | 准确定义 |
|---|---|---|
| `UDADefinition` | 自定义字段 | Workspace 级字段 schema |
| `uda.* config view` | 不直接展示 | definition 的兼容管理入口 |
| Project UDA mode | 字段使用方式 | 跟随全部或自定义选择 |
| Project effective UDA | 项目可用字段 | 当前 Project 普通写入可使用的 definition |
| Task/Series UDA value | 自定义字段值 | 具体对象保存的值 |
| unavailable saved value | 历史字段值 | 已保存但 Project 当前未启用 |
| orphan UDA | 未定义历史字段 | Workspace 已没有 definition 的普通 Task 历史值 |

避免使用“Workspace 的 UDA 值”描述 definition。Workspace 只有定义，值属于 Task / TaskSeries。

## 7. 信息架构与入口

### 7.1 Workspace 设置

新增：

```text
/workspaces/$workspaceSlug/settings/custom-fields
```

现有 `/settings` 继续管理 `ConfigDefinition`，不能改名或暗示它管理 Task UDA。

### 7.2 Project 设置

在现有 Project Settings layout 中增加：

```text
/workspaces/$workspaceSlug/projects/$projectSlug/settings/custom-fields
```

现有 `config`、`definitions`、`notes` 保留。为承接当前 Header slug Dialog，新增 `general`：

```text
/workspaces/$workspaceSlug/projects/$projectSlug/settings/general
```

Project Settings 默认入口重定向到 `general`。

### 7.3 Project Header

当前 icon-only 齿轮改为带文字的“项目设置”链接，目标为 `settings/general`。slug 编辑迁入 General；Project name/description 的 Header inline edit 保留。

## 8. ASCII 原型

### 8.1 Project Header

```text
工作区 / 项目 / demo

Acme 内容增长                         [状态：进行中 ▾] [复制链接] [项目设置]
围绕搜索与内容分发建立稳定增长闭环
──────────────────────────────────────────────────────────────────────────────
概览   任务   循环任务   活动   自动化
```

入口必须有文字，不能依赖齿轮图标或项目列表行操作。

### 8.2 Project 自定义字段

```text
demo / 项目设置

[常规] [自定义字段] [项目配置] [配置定义] [项目说明]

自定义字段
决定本项目创建和编辑任务时可使用哪些 Workspace 字段。

(•) 跟随 Workspace 全部字段
    自动使用当前及以后新增的全部 Workspace 字段。

( ) 自定义选择
    只使用下方选中的字段。

┌────────────────────────────────────────────────────────────────────┐
│ ☑ 渠道       channel     字符串     search / social       [拖动] │
│ ☑ 工作量     estimate    数字       1 / 2 / 3 / 5 / 8     [拖动] │
│ ☐ 外部单号   external_id 字符串     18 个任务已有值               │
└────────────────────────────────────────────────────────────────────┘

[创建 Workspace 字段]                                      [保存]
```

跟随全部模式下列表只读，用于解释 effective fields；切换到自定义选择后才可勾选和排序。

### 8.3 Workspace 字段库

```text
Workspace 设置 / 自定义字段

自定义字段                                      [新建字段]
这些定义可被 Workspace 内的项目选择。

┌────────────────────────────────────────────────────────────────────┐
│ 工作量       estimate       数字       1,2,3,5,8       [编辑]    │
│ 渠道         channel        字符串     search,social    [编辑]    │
│ 评审日期     reviewed       日期                         [编辑]    │
└────────────────────────────────────────────────────────────────────┘

编辑字段
显示名称  [工作量________________]
字段名    [estimate______________]  创建后不可修改
类型      [数字 ▾]
可选值    [1, 2, 3, 5, 8________]
默认值    [3_____________________]
                                    [取消] [保存]
```

### 8.4 Task 创建与详情

```text
创建任务                               任务详情 / 自定义字段
┌──────────────────────────────┐      ┌──────────────────────────────┐
│ 标题 [____________________]  │      │ 渠道       search           │
│                              │      │ 工作量     -       [填写]   │
│ 自定义字段                   │      │                              │
│ 渠道   [未设置 ▾]            │      │ 历史字段                     │
│ 工作量 [___________]         │      │ 外部单号   EXT-123 [清除]   │
│        提示：默认值 3        │      └──────────────────────────────┘
└──────────────────────────────┘
```

default 只作为提示，不预填提交值；用户不填写时不保存 UDA value。

## 9. Project UDA 数据模型（新增）

### 9.1 设置模式

Project 有两种模式：

```text
inherit_all  跟随 Workspace 全部 effective definitions
custom       只启用显式保存的 ordered names
```

这是兼容性的关键：当前系统语义就是所有 Project 使用 Workspace 全部 definitions。不能在升级时把旧 Project 变成空集合，也不能只复制 DB definitions 而漏掉 runtime definitions。

### 9.2 存储结构

```go
type ProjectUDASettings struct {
    WorkspaceID string `gorm:"primaryKey;not null"`
    ProjectID   string `gorm:"primaryKey;not null"`
    Mode        string `gorm:"not null"` // inherit_all | custom
    Version     int64  `gorm:"not null"`
    CreatedAt   int64  `gorm:"not null"`
    ModifiedAt  int64  `gorm:"not null"`
}

type ProjectUDAField struct {
    WorkspaceID string `gorm:"primaryKey;not null"`
    ProjectID   string `gorm:"primaryKey;not null"`
    Name        string `gorm:"primaryKey;not null"`
    Position    int    `gorm:"not null"`
}
```

约束：

- Project 必须属于同一 `WorkspaceID`；
- `custom` 模式下 position 在 Project 内唯一且从 0 连续；
- relation 按 logical UDA name 保存，不对 `uda_definitions` 建数据库外键，因为 effective definition 还可能来自 runtime config；
- App 保存时必须确认所有 names 当前都存在于 `ListUDAs()`；
- `inherit_all` 模式不保存 field rows。

### 9.3 旧数据与缺省行

为避免迁移窗口改变行为：

- 没有 `ProjectUDASettings` row 的 Project 永久按 `inherit_all` 解释；
- 新建 Project 时显式创建 `inherit_all, version=1`；
- 不为旧 Project 批量复制 UDA relation；
- 因此 runtime UDA、以后新增的 definition 都会自动出现在未自定义的 Project 中。

### 9.4 effective resolver

App 层提供单一 resolver：

```go
ProjectEffectiveUDAs(projectID string) (ProjectUDAView, error)
```

算法：

1. 校验 Project 属于 effective Workspace 和 token project scope；
2. 调用 `ListUDAs()` 得到 DB + runtime effective definitions；
3. settings 缺失或 mode=`inherit_all`：返回全部 definitions，按 label/name 稳定排序；
4. mode=`custom`：按 relation position 返回仍存在的 definitions；
5. relation 指向当前缺失的 runtime definition 时不报 500，放入 `missing_names` diagnostics；
6. 所有 Task、Series、Web、HTTP、MCP 写校验复用该 resolver。

### 9.5 原子保存与并发

Project 设置保存为一个 App transaction：

```text
校验 permission / Project scope
  → 校验 expected_version
  → 校验 mode 和 names
  → custom 时原子替换 ordered rows
  → version + 1
  → 写 audit
```

version 冲突返回 `project_uda_version_conflict`（409）。Web 保留用户 draft，提示刷新后重试。

## 10. Workspace definition 生命周期

### 10.1 字段定义保持现状

本规格继续使用：

```go
type uda.Definition struct {
    Name          string
    Type          uda.Type
    Label         string
    Values        []string
    Default       string
    OrphanAllowed bool
}
```

其中 `OrphanAllowed` 当前不持久化，也不是 Web 可编辑属性。

字段 name：

- trim 后不能为空；
- 不得与内建 Task 属性冲突；
- 创建后不可改名；
- 允许点号，兼容键继续以最后一个点拆分 field suffix。

### 10.2 default 语义不改变

当前代码保存 definition default，但 `DefineUDA` 与 `SetConfig` 的校验并不一致；两条路径都不会在 Task / Series 创建时应用 default。本规格固定保持“不自动应用”，同时统一 definition 写入校验：

- default 是输入提示和兼容 schema 元数据；
- 非空 default 必须按当前 type 归一化并通过当前 enum 校验；
- 更新 type / values 时也必须重新校验已有 default；
- Web 可以显示“默认值：3”，但不能把它伪装成已保存值；
- 调用方没提交该字段时，Task / Series 不保存值；
- Projected occurrence 只继承 Series 实际保存的值，不继承 definition default。

如果未来要把 default 改成服务端自动值，必须另写 spec，覆盖导入、模板、Series 和已有客户端兼容性。

### 10.3 type / values 更新

当前代码不会扫描历史 Task / Series value。本规格只增加一条防止调度失败的保护：

- 更新后的 definition 必须能归一化所有 active TaskSeries 中该字段的值；
- 不兼容时返回 `uda_active_series_incompatible`，不修改 definition；
- 普通 Task 历史值不做批量改写，读取时若与新 definition 不兼容，Web 标记为历史值并允许清除；
- ended/stopped Series 不会再物化，可保留历史 raw value。

### 10.4 删除

当前删除 definition 会让普通 Task 已有值成为 definition 缺失的历史/orphan 语义；本规格不禁止这一兼容能力，也不级联删除值。

新增保护：

- active TaskSeries 仍保存该 name 时禁止删除，返回 `uda_active_series_in_use`；
- 删除 DB definition 时，同一 transaction 删除 `custom` Project 中对应 relation；
- 普通 Task、ended/stopped Series 和 Project Template snapshot 中的 raw value 不删除；
- `config unset uda.<name>.type` 必须走同一 App 删除路径，不能绕过 active Series 保护。

### 10.5 runtime definition

runtime UDA 保持现有优先级与只读来源特征：

- `ListUDAs()` 继续返回；
- `inherit_all` Project 自动包含；
- `custom` Project 可以按 name 选择；
- runtime 配置被移除后，custom relation 暂时进入 `missing_names`；恢复同名 runtime definition 后重新生效；
- 本规格不自动把 runtime definition 落库。

## 11. Task、TaskSeries 与 occurrence

### 11.1 普通 Task add/modify

普通写入新增 Project 校验：

1. 先解析 Task 最终所属 Project；修改 Project 与 UDA 同时提交时，以目标 Project 为准；
2. 对每个非空 `udas[name]`，要求 definition 存在；
3. Task 有 Project 时，要求 name 属于该 Project effective fields；
4. Task 没有 Project 时，允许 Workspace 全部 effective definitions，保持当前行为；
5. 清空已保存值不受 Project 可用范围限制，但继续遵守现有 orphan readonly 规则；
6. 最后按现有 `NormalizeValue` 保存。

未启用字段写非空值返回：

```json
{
  "code": "project_uda_not_enabled",
  "message": "custom field \"estimate\" is not enabled for project \"demo\""
}
```

### 11.2 Task 移动 Project

移动只改变归属，不改写 UDA value：

- 只移动、不提交 UDA：允许，所有值保留；
- 同时提交 UDA：新值按目标 Project 校验；
- 目标 Project 未启用但已保存的值成为“历史字段值”；
- 之后可清除，不能改成另一个非空值；
- query、export、urgency 继续读取该值。

### 11.3 TaskSeries

TaskSeries 必须有 Project，因此 add/modify 的非空 UDA 都按 Series Project effective fields 校验。

Project 从 `custom` 集合移除字段前：

- 若 active Series 仍保存该字段，拒绝保存 Project 设置；
- 返回 `project_uda_active_series_in_use` 和 Series 数量；
- 用户必须先清除 active Series 上的值，或结束/停止 Series；
- 不做隐式级联清除。

这一约束避免 Project 显示“已停用”，但 scheduler 又持续从 Series 生成带该值的新 occurrence。

ended/stopped Series 的值是历史数据，不阻止 Project 停用。

### 11.4 occurrence

- Projected occurrence 继续从 Series 实际值派生；
- materialization 继续用现有 `normalizeUDAModifications`；
- 因 Project 停用前已阻止 active Series 持有该值，不会产生新的 unavailable value；
- 已物化 occurrence 的旧值按普通 Task 历史值处理；
- 单次 occurrence override 的非空 UDA 按所属 Project effective fields 校验。

### 11.5 Task 详情渲染

详情页合并两组数据：

```text
effective definitions
  LEFT JOIN task saved values

+ saved values whose name is not effective
```

渲染规则：

- effective + 无值：显示空控件，可填写；
- effective + 有值：按 definition type 编辑；
- unavailable + definition 存在：显示“历史字段”，只允许清除；
- orphan：沿用现有只读展示，不声称可清除；
- 不再识别 boolean，因为 UDA domain 没有 boolean type。

## 12. Query、urgency、helper 与导入导出

### 12.1 Query

Project UDA settings 是“普通写入可用范围”，不是数据可见性过滤器：

- definition 存在时，value query 继续跨 Project 查询所有已保存值；
- Project custom 模式不改变 query compiler 的 `UDADefinitions`；
- orphan 的现有 notnull 特例保持不变；
- 不给 query 增加隐式 Project enabled 条件。

这样用户仍能定位和清理历史值。

### 12.2 Urgency

urgency 继续按已保存值和 `urgency.uda.*` 系数计算，不检查 Project enabled 状态。

原因：停用字段不会删除 Task value；若在读取时悄悄忽略，会让同一 Task 因 Project 设置变化而改变排序，却仍在 API 中返回原值。要消除 contribution，应显式清除值或调整 urgency 系数。

### 12.3 CLI helper

- `_udas` 继续列出 Workspace effective definitions；
- `_unique <uda>` 继续统计已保存值；
- Project effective fields 由 Project API / MCP 工具读取，不改变旧 helper 输出。

### 12.4 普通导入

`/api/v1/task-imports` 和现有 JSON import 保持数据迁移语义：

- 已定义字段按 definition 归一化；
- 未定义字段可作为普通 Task orphan 保存；
- Project 未启用但 definition 存在的值允许作为历史值导入；
- 导入结果返回 warning，列出 `project_unavailable_udas`，但不丢数据。

Web 创建表单和普通 Task add API 不是导入，不得使用此旁路。

### 12.5 原生 bundle

`xuanchu.task-bundle/v1` 保持严格保真，不新增 Project UDA settings：

- Task / Series raw UDA 原样恢复；
- settings 不是该 bundle 当前 contract 的一部分；
- 恢复到 custom Project 时，值可能被标记为历史；
- bundle 导入不能被拿来替代普通 Task add。

### 12.6 导出

- 已保存值全部导出，不按 Project effective fields 过滤；
- Taskwarrior 兼容 JSON 继续顶层平铺；
- HTTP/MCP/Remote 继续使用现有嵌套 `udas`；
- Project UDA settings 通过自己的 Project resource 读取。

## 13. Project Template 同步

Project Template 是 Workspace 内复用的不可变、版本化 Snapshot，不是 task bundle。既然 Project UDA settings 已成为 Project 配置的一部分，保存模板和实例化模板都必须覆盖它。

当前 `xuanchu.project-template-snapshot/v1` 使用严格 `DisallowUnknownFields` codec，只保存 Task / Series UDA blueprint，不保存 Project 字段设置。不能向 v1 JSON 静默增加字段，因此本规格新增 v2，同时永久保留 v1 解码兼容。

### 13.1 Snapshot v2

新增：

```go
const SnapshotSchemaV2 = "xuanchu.project-template-snapshot/v2"

type SnapshotV2 struct {
    Schema      string                  `json:"schema"`
    AnchorDate  string                  `json:"anchor_date"`
    Project     ProjectBlueprintV2      `json:"project"`
    Configs     []ConfigBlueprintV1     `json:"configs"`
    Tasks       []TaskBlueprintV1       `json:"tasks"`
    Series      []SeriesBlueprintV1     `json:"series"`
    Automations []AutomationBlueprintV1 `json:"automations"`
}

type ProjectBlueprintV2 struct {
    Description string                        `json:"description"`
    UDASettings ProjectUDASettingsBlueprintV2 `json:"uda_settings"`
}

type ProjectUDASettingsBlueprintV2 struct {
    Mode  string   `json:"mode"`  // inherit_all | custom
    Names []string `json:"names"` // custom 下为有序 name；inherit_all 必须为空
}
```

设计边界：

- Snapshot 只保存 Project 的 mode 和 ordered names；
- 不复制 type、label、values、default，不制造 Project 私有 definition；
- definition 仍引用实例化时同一 Workspace 的当前 effective definitions；
- `inherit_all` 保存的是模式，不展开为当时的 name 列表，因此以后新增的 Workspace definition 仍会自动生效；
- 如果用户需要固定字段集合，应先把源 Project 切成 `custom`。

### 13.2 codec 兼容

保留冻结的 `SnapshotV1` / `EncodeV1`，新增 `SnapshotV2` / `EncodeV2`。`Decode` 按 schema 分派并转换成统一的内部 `Snapshot`：

```text
v1 → Project.UDASettings = {mode: inherit_all, names: []}
v2 → 严格读取并校验 snapshot 中的 uda_settings
```

两种 schema 都必须：

- `DisallowUnknownFields`；
- 拒绝 trailing JSON；
- 经过各自的 canonical normalization；
- 使用 canonical JSON 计算 SHA-256；
- 不修改调用方传入 struct；
- 对 required array 保持稳定的 `[]`，不漂移为 `null`。

本功能上线后，新 capture 一律 `EncodeV2`；已有 v1 Snapshot 不重写、不换 hash、不生成隐式新版本。

### 13.3 保存项目模板

Capture Preview 和最终 Capture 都必须从源 Project 读取 raw UDA settings，而不是仅看 effective fields：

```text
源 Project inherit_all
  → snapshot.project.uda_settings = {mode: inherit_all, names: []}

源 Project custom(channel, estimate)
  → snapshot.project.uda_settings = {
      mode: custom,
      names: [channel, estimate]
}
```

UDA settings 与 Project description 一样属于 Project blueprint 核心状态，保存模板时始终捕获，不放进可选组件清单。用户可以逐项选择 Task / Series，但不能生成一份故意遗漏 Project UDA settings、随后又携带 UDA value 的自相矛盾 Snapshot。

Project UDA settings 必须进入 `captureSourceFingerprint`。这样 Preview 后若有人修改 mode、勾选或顺序，最终保存会命中现有 `expected_source_hash` 冲突，而不是保存过期设置。

Capture 校验：

1. custom names 必须在源 Project 当前 effective Workspace definitions 中存在；
2. names 去重，保留 Project position 顺序；
3. 被选择的普通 Task / TaskSeries 若包含不在 custom names 中的历史 UDA value，阻止保存并返回 `project_template_uda_not_enabled`；
4. 第一版不静默丢弃历史 UDA，也不自动扩大 Project custom 集合；用户应先清理历史值、重新启用字段，或不选择该 Task/Series；
5. inherit_all 下仍按现有逻辑验证 Task/Series UDA definition 和 type。

Template Snapshot 详情、版本比较和 Capture Preview 都要显示 UDA mode；custom 模式还要显示 ordered names。

### 13.4 根据模板创建项目

Instantiate Preview 在写入前校验：

- v1 已归一化为 `inherit_all`；
- v2 `inherit_all` 不需要解析 name 列表；
- v2 `custom` 的每个 name 必须存在于实例化 Workspace 的 `ListUDAs()`；
- 缺失时返回 blocking issue `project_template_uda_definition_missing`，列出 field name；
- Task/Series blueprint 中每个 UDA name 必须属于将要创建的 Project effective fields；不一致说明 Snapshot 损坏，返回 `project_template_uda_not_enabled`；
- 不自动创建、覆盖或修改 Workspace definition。

最终实例化仍在当前单一事务内完成，顺序调整为：

```text
锁定 Template / Snapshot 并重建 plan
  → 创建 Project
  → 写入 Project UDA settings 与 ordered fields
  → 写 Project Config
  → 创建 TaskSeries
  → 创建普通 Task
  → 回填正文、链接、Automation
  → audit / event
```

Project UDA settings 必须在 Series / Task 之前写入，否则本规格新增的 App 写校验会把模板自身的 UDA value 当成未启用字段拒绝。

实例化产生 `project.uda.set` audit，payload 带 `template_id`、`snapshot_id`、`snapshot_hash`，与现有 template source metadata 一致。任一步失败，Project、UDA settings、Task、Series 和其他组件全部回滚。

### 13.5 Web 原型

```text
保存为项目模板 / 项目内容

项目设置
  自定义字段：自定义选择
  将保存 2 个字段及顺序：
    1. 渠道（channel）
    2. 工作量（estimate）

任务与循环任务
  ☑ 任务  OPS-12  落地页验收
  ☑ 系列  每周复盘

若所选任务含项目未启用的历史字段：
  ! OPS-12 含历史字段 external_id
    请重新启用、清除该值，或取消选择此任务。
```

```text
从模板创建项目 / 预览

自定义字段
  模式：自定义选择
  渠道（channel）
  工作量（estimate）

若 Workspace definition 缺失：
  ! 缺少自定义字段 estimate，无法创建项目
```

### 13.6 回滚边界

旧版本代码无法解码 v2 Snapshot。代码回滚不会破坏旧 v1，但回滚期间不能预览或实例化新 v2 Snapshot。发布说明和回滚手册必须明确这一点；不得通过降级重写 v2 为 v1，因为那会丢失 Project UDA settings 并改变 immutable Snapshot hash。

## 14. HTTP API

### 14.1 Workspace definitions（新增结构化接口）

Web 不再从扁平 config 猜结构：

```text
GET    /api/v1/udas
PUT    /api/v1/udas/{name}
DELETE /api/v1/udas/{name}
```

请求/响应字段严格对应现有 definition：

```json
{
  "name": "estimate",
  "type": "numeric",
  "label": "工作量",
  "values": ["1", "2", "3", "5", "8"],
  "default": "3"
}
```

结构化 HTTP resource 与 `config_*` 兼容入口调用同一 App service；不能形成两套校验。

列表可以附加只读 usage：

```json
{
  "task_value_count": 128,
  "active_series_value_count": 2,
  "custom_project_count": 6
}
```

usage 是 UI 提示，不新增单独 MCP tool。

### 14.2 Project settings（新增）

```text
GET /api/v1/projects/{projectRef}/udas
PUT /api/v1/projects/{projectRef}/udas
```

GET：

```json
{
  "project": {"id": "...", "slug": "demo"},
  "mode": "custom",
  "version": 3,
  "selected_names": ["channel", "estimate"],
  "effective_fields": [
    {
      "name": "channel",
      "type": "string",
      "label": "渠道",
      "values": ["search", "social"],
      "default": "",
      "task_value_count": 41,
      "active_series_value_count": 0
    }
  ],
  "available_fields": [],
  "missing_names": []
}
```

PUT：

```json
{
  "mode": "custom",
  "names": ["channel", "estimate"],
  "expected_version": 3
}
```

`inherit_all` 请求的 `names` 必须为空。

### 14.3 现有 Task API

- add/modify payload 的 `udas` / `clear_udas` 不改名；
- read DTO 的 `udas` 不改形状；
- OpenAPI 补 Project UDA resource 与错误码；
- Remote Client 增加 Project UDA get/set，不增加重复 Workspace UDA client 方法也可以继续复用 config；Web 使用结构化 HTTP resource。

### 14.4 Project Template API

现有 Capture Preview、Capture、Snapshot Detail、Instantiate Preview 的 endpoint 不增加平行接口，但其 typed DTO 和 OpenAPI schema 必须增加：

```json
{
  "project": {
    "description": "...",
    "uda_settings": {
      "mode": "custom",
      "names": ["channel", "estimate"]
    }
  }
}
```

HTTP、Remote Client、CLI 和 MCP 的模板读取/预览结果必须使用同一 DTO，不能只在 Web 私有解析 Snapshot JSON。

### 14.5 错误码

| code | HTTP | 含义 |
|---|---:|---|
| `project_uda_mode_invalid` | 422 | mode 非法 |
| `project_uda_not_enabled` | 422 | 普通写入使用 Project 未启用字段 |
| `project_uda_definition_missing` | 422 | custom names 包含当前不存在的 definition |
| `project_uda_version_conflict` | 409 | settings 并发版本冲突 |
| `project_uda_active_series_in_use` | 409 | 停用字段仍被 active Series 使用 |
| `uda_active_series_in_use` | 409 | 删除 definition 会破坏 active Series |
| `uda_active_series_incompatible` | 409 | type/values 更新与 active Series value 不兼容 |
| `project_template_uda_definition_missing` | 422 | v2 custom setting 引用的 Workspace definition 已缺失 |
| `project_template_uda_not_enabled` | 422 | 模板组件 UDA 与 Snapshot Project custom 设置矛盾 |

现有 `uda_not_defined`、`uda_value_invalid`、`uda_orphan_readonly` 保持。

## 15. CLI、Remote Client 与 MCP

### 15.1 CLI

Workspace definition 兼容命令保持：

```text
xuanchu config set uda.estimate.type numeric
xuanchu config set uda.estimate.label 工作量
xuanchu config set uda.estimate.values 1,2,3,5,8
xuanchu config set uda.estimate.default 3
xuanchu config unset uda.estimate.type
xuanchu _udas
```

Project settings 增加面向脚本的命令：

```text
xuanchu project uda list demo --json
xuanchu project uda set demo --mode inherit-all
xuanchu project uda set demo --mode custom --fields channel,estimate --expected-version 3
```

命令内部调用 App，不在 CLI 复制规则。

### 15.2 MCP：不新增 Workspace UDA 同义工具

当前以下工具已经覆盖 Workspace definition：

```text
config_list
config_get
config_set
config_unset
```

它们能够读写全部现有 definition 属性，因此不需要：

```text
uda_list
uda_set
uda_get_usage
```

需要修复现有 tenant capability 映射：`PermissionUDAManage` 映射到 `config:write`，否则 tenant token 即使持有 `config:write`，App 的二次权限检查仍会拒绝 `uda.*`。

### 15.3 MCP：Task 工具

现状：

- `task_query` / `task_get` 已输出 `udas`；
- `task_modify` 已接收 `udas`；
- `clear: ["uda.<name>"]` 已清除值；
- `task_add` 唯一缺少 `udas`。

本规格只给 `TaskAddInput` 增加：

```go
UDAs map[string]string `json:"udas,omitempty"`
```

并传入现有 `app.AddInput.UDAs`。

### 15.4 MCP：Project 工具

新增：

```text
project_list_udas
project_uda_set
```

不使用 `project_uda_list`，因为项目子资源读操作沿用 `project_list_annotations` / `project_list_timeline` 的 `list` 前缀规范。

`project_list_udas` 返回与 Project HTTP GET 等价的结构；`project_uda_set` 接收 Project ref、mode、ordered names、expected version。

## 16. 权限

| 操作 | App permission | 用户角色 | tenant capability |
|---|---|---|---|
| 读取 Workspace UDA | `PermissionWorkspaceRead` | owner/admin/member/viewer | `config:read` |
| 修改 Workspace UDA | `PermissionUDAManage` | owner/admin | `config:write` |
| 读取 Project UDA | `PermissionProjectRead` | 有 Project 读取权限 | `project:read` |
| 修改 Project UDA | `PermissionProjectManage` | owner/admin | `project:write` |
| Task/Series 写值 | `PermissionTaskWrite` | owner/admin/member | `task:write` |

浏览器 session 继续由当前 membership role 决定；PAT/Agent/tenant token 同时受 capability 和 Project allowlist 限制。

Project archived/cancelled 时：

- settings 可读；
- UDA settings 不可写，沿用 Project closed 行为；
- 历史 Task/Series value 可读。

## 17. 审计

Workspace definition 继续使用：

```text
uda.schema.set
uda.schema.delete
```

Project settings 新增：

```text
project.uda.set
```

payload 保存机器语义：

```json
{
  "mode_before": "inherit_all",
  "mode_after": "custom",
  "added": ["channel", "estimate"],
  "removed": ["external_id"],
  "order_before": [],
  "order_after": ["channel", "estimate"],
  "version_before": 1,
  "version_after": 2
}
```

不把 Task value 写入 Project settings audit；Task 自身已有 UDA change audit。

## 18. Web Console 实现边界

### 18.1 Workspace 页面

- 使用结构化 `/api/v1/udas`；
- owner/admin 可新建、编辑、删除；member/viewer 只读；
- 显示 active Series 风险；
- 删除确认明确说明普通 Task 值不会删除，会成为未定义历史字段。

### 18.2 Project 页面

- Header 始终展示“项目设置”；无 manage 权限时仍可进入只读设置；
- 自定义字段页显示 mode、effective fields、usage；
- 切换 custom 不自动保存；初始勾选当前全部 effective fields，避免误清空；
- 取消勾选有 Task 历史值的字段时显示“值会保留并变为只读”；
- active Series 使用时禁用保存并给出处理入口；
- 保存 version 冲突时不丢 draft。

### 18.3 Task / Series 表单

- 从 Project UDA endpoint 读取 effective fields；
- 无 Project 的普通 Task 从 Workspace UDA endpoint 读取全部 fields；
- 按 definition type 渲染；
- `values` 非空时使用 select；
- numeric 使用 decimal input；date 使用 date input；duration 第一版仍使用 text 并显示保存单位说明；
- default 只作为提示，不进入 payload；
- 不再推断或渲染 boolean UDA。

### 18.4 Task 详情

- 同时请求 Task 与 Project effective fields；
- 空的 effective field 也显示；
- unavailable saved value 单独放在“历史字段”区；
- orphan 保持只读；
- label 用于展示，name 作为次要技术信息和提交 key。

## 19. 迁移与兼容

### 19.1 数据库迁移

只新增：

```text
project_uda_settings
project_uda_fields
```

迁移不修改：

```text
uda_definitions
task_uda_values
task_series_uda_values
configs
config_definitions
```

旧 Project 没有 settings row 时按 `inherit_all`，因此不需要批量 relation 回填，也不会漏 runtime definition。

Project Template 数据库表不迁移；已有 Snapshot JSON 和 hash 原样保留。变化只在 typed codec：旧 v1 继续读取，新 capture 写 v2。

### 19.2 旧客户端

- 读接口形状不变；
- Task add/modify 的 UDA payload 不变；
- Project 处于 inherit_all 时行为完全不变；
- Project 切换 custom 后，旧客户端写未启用字段会收到稳定 422；
- import/bundle 仍可做保真迁移；
- `uda.*` Config 兼容键继续可用。

### 19.3 删除与回滚

若代码回滚到不认识 Project settings 的旧版本：

- 新表被忽略；
- Task/Series value schema 未改变；
- 旧版本会恢复 Workspace 全部 definitions 可写。
- 旧版本仍能读取 v1 Snapshot，但不能解码本版本新建的 v2 Snapshot。

因此这次迁移不破坏既有数据，但回滚会临时失去 Project 写限制，并暂停新 v2 模板的预览和实例化，发布说明必须明确。

## 20. 测试与验收

### 20.1 Storage / App

- Config、ConfigDefinition、UDADefinition 三类数据不混表；
- Project settings 缺失按 inherit_all；
- inherit_all 包含 DB + runtime definitions，DB 同名覆盖 runtime；
- custom 按 position 返回选中 definitions；
- custom 保存原子替换并检查 version；
- cross-workspace / token Project allowlist 被拒；
- Task add/modify 按最终 Project 校验；
- Task move 保留历史值；
- 无 Project Task 仍允许全部 Workspace definitions；
- 清除 unavailable value 成功，写非空值失败；
- active Series 阻止 Project 停用、definition 删除和不兼容 schema 修改；
- stopped/ended Series 不阻止停用；
- Projected/materialized occurrence 不产生新的 unavailable value；
- import 保留 orphan/unavailable 值并返回 warning；
- query、urgency 和 export 不因 Project settings 改变；
- Snapshot v2 canonical encode/decode、strict unknown-field/trailing-JSON 拒绝和 hash 稳定性；
- v1 Decode 归一化为 inherit_all，且旧 Snapshot JSON/hash 不改写；
- capture 保存 inherit_all/custom mode、ordered names，并把 settings 纳入 source hash；
- custom Project 所选组件含未启用历史 UDA 时 capture 阻断；
- instantiate preview 检查缺失 definition 和 Snapshot 内部 UDA 一致性；
- instantiate 在 Task/Series 前写 Project UDA settings，任一步失败整体回滚。

### 20.2 HTTP / Remote / MCP / CLI

- Workspace UDA 结构化 HTTP CRUD 与 config 兼容入口结果一致；
- Project UDA GET/PUT、错误码和 OpenAPI；
- `task_add` MCP schema 包含 `udas` 并成功落库；
- `task_query`、`task_get`、`task_modify` 现有 UDA 合约回归；
- MCP 不注册 `uda_list` / `uda_set`；
- `project_list_udas` / `project_uda_set` 命名和权限正确；
- `PermissionUDAManage` tenant mapping 使用 `config:write`；
- Remote / CLI Project UDA round trip；
- HTTP/OpenAPI、Remote、CLI、MCP 的 Template DTO 都暴露 `project.uda_settings`；
- 新 capture 返回 v2，历史 v1 仍可 preview/instantiate。

### 20.3 Web

- `/workspaces/demo/projects/demo` Header 有带文字设置入口；
- General 能编辑 slug，原 Header inline name/description 不回归；
- Workspace definition 列表与编辑；
- Project inherit_all/custom 切换、排序、usage、只读态、version 冲突；
- Task create/detail 使用 effective definitions；
- 空字段可补填；历史字段和 orphan 区分；
- numeric/date/duration/string typed editor；
- 不再猜 boolean；
- Capture Wizard 展示将保存的 UDA mode/names，并阻断 selected component 的历史未启用 UDA；
- Instantiate Preview 展示将恢复的 UDA settings 和缺失 definition issue；
- `pnpm --dir web run smoke:editing` 覆盖设置入口与 Task 写值。

### 20.4 完整验证命令

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
```

### 20.5 产品验收场景

1. 旧 Project 升级后继续看到 Workspace 全部 UDA；
2. Workspace 新增字段后，inherit_all Project 自动看到；custom Project 不自动看到；
3. owner 从 `/workspaces/demo/projects/demo` 点击“项目设置”进入自定义字段；
4. Project 切 custom，只保留 `channel`、`estimate`；
5. 新 Task 表单只显示这两个字段；
6. MCP `task_add` 能写两个字段，写未启用字段返回 `project_uda_not_enabled`；
7. 已有 Task 的未启用值仍可读、query、export、参与现有 urgency，可清除不可改写；
8. active Series 使用字段时 Project 不能停用，清除 Series 值后可停用；
9. 无 Project Task 仍可使用 Workspace 全部 definitions；
10. `config_set uda.*` 与 Workspace 结构化页面读到同一条 definition；
11. MCP 工具列表没有重复的 `uda_list` / `uda_set`；
12. custom Project 保存为模板后，Snapshot v2 保留 mode、name 顺序；
13. 根据该模板创建 Project 时，settings 先恢复，Task/Series UDA 随后成功创建；
14. v2 引用的 definition 被删除后，Instantiate Preview 阻断且不产生半成品 Project；
15. 旧 v1 模板仍按 inherit_all 创建 Project。

## 21. 文档同步

实现完成后同步：

- `README.md`：三层模型、Project 模式、API/CLI/MCP 示例；
- `ROADMAP.md`：Project 自定义字段里程碑状态；
- OpenAPI 与 MCP golden schemas；
- Project Template Snapshot v2 schema、codec 与跨入口示例；
- Web Console 帮助与中英文文案；
- implementation plan；
- 本 spec 的最终实现偏差。

## 22. 实施顺序边界

implementation plan 应至少拆为：

1. Storage migration、repo 与 effective resolver；
2. Workspace definition 结构化 App/HTTP 资源及 active Series 保护；
3. Project settings App/HTTP、权限、审计与并发；
4. Task/Series/occurrence 普通写校验与 import warning；
5. MCP `task_add` 补齐和 Project tools；
6. CLI / Remote Client；
7. Workspace / Project Settings Web 页面与 Header 入口；
8. Task / Series typed form 和 Task detail 合并渲染；
9. Project Template Snapshot v2 codec、capture source hash、preview 与 transaction restore；
10. Capture / Instantiate Web、HTTP、Remote、CLI、MCP 契约同步；
11. OpenAPI、golden、smoke、全量验证与文档同步。

不要在同一实现中顺带引入自动 default、Project 私有 definition、跨 Workspace definition 复制或 ConfigDefinition 合并。
