# Web Console ConfigDefinition 与 Project Config Value 完整配置设计

> **给 agentic workers 的要求：** 编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

**日期：** 2026-07-07
**状态：** 草案
**目标：** 让 Web Console 能完整管理 workspace/project 两类 `ConfigDefinition` 视图，并让 project 级 `ConfigValue` 按定义可视化配置、校验和继承展示。

## 1. 结论先行

当前 Web Console 的 `/settings` 路由不是 `ConfigDefinition` 管理页，而是通过通用 `ResourcePage` 展示 `GET /api/v1/config` 返回的 workspace config value。也就是说，它现在更接近“workspace 显式配置值列表”，不是 schema/definition 控制面。

本次应把 `/settings` 改成 workspace 级 `ConfigDefinition` 控制面，展示当前 workspace 内的所有定义，并提供 scope 过滤；暂不提供 workspace value 编辑。Project 侧继续使用已经建立的 `/projects/{slug}/settings/...` 子路由体系，并补齐两个子页：

- `/projects/{slug}/settings/definitions`：project 级 `ConfigDefinition` 配置页，实际管理 workspace 内允许 `project` scope 的定义。
- `/projects/{slug}/settings/config`：project config value 配置页，基于 workspace 与 project scope 的 `ConfigDefinition` 列出可配置 key，并按定义渲染控件。

`ConfigDefinition` 本身仍然归属 workspace。所谓 workspace/project 两类定义，不是新增 project 私有 schema 表，而是同一张 `config_definitions` 在 Web Console 中按 `allowed_scopes` 分成两个配置入口：

- workspace 定义：`allowed_scopes` 包含 `workspace`。
- project 定义：`allowed_scopes` 包含 `project`。
- 同一个 key 可以同时属于两类，即 `allowed_scopes=["workspace","project"]`。

## 2. 背景与现状

### 2.1 现有 scoped config 能力

后端已经具备 shared scoped config 基础能力：

- `config_definitions`：按 workspace 隔离的 `ConfigDefinition`。
- `configs`：按 `(workspace_id, scope, scope_id, key)` 存储 workspace/project value。
- schema API：
  - `GET /api/v1/config-schema`
  - `GET /api/v1/config-schema/{key}`
  - `PUT /api/v1/config-schema/{key}`
  - `DELETE /api/v1/config-schema/{key}?purge=true|false`
- project value API：
  - `GET /api/v1/projects/{ref}/config`
  - `GET/PUT/DELETE /api/v1/projects/{ref}/config/{key}`
- workspace value API：
  - `GET/PUT/DELETE /api/v1/config/{key}`

现有删除定义逻辑已经支持 `purge=false` 时发现 value 后返回 `config_definition_in_use`，`purge=true` 时删除定义和所有同 key value。

### 2.2 Web Console 现状

当前分支已经有项目设置页基础：

- `/projects/{slug}/settings/config`：project value 列表、添加、编辑、删除。
- `/projects/{slug}/settings/notes`：项目备注。
- project value 控件已经初步读取 `ConfigDefinition`，但仍偏“显式值列表”，没有完整展示继承、缺失、默认值、workspace-only key、定义变更后的状态。

当前 `/settings` 仍是通用资源页：

```tsx
case "settings":
  return {
    title: t("page.settings"),
    path: "/api/v1/config",
    columns: [
      { key: "key", header: t("resource.key") },
      { key: "value", header: t("resource.value") },
    ],
  }
```

因此 `/settings` 目前展示 workspace value，而不是 `ConfigDefinition`。

## 3. 目标

1. `/settings` 改为 workspace 级 `ConfigDefinition` 配置页，不再展示 workspace value。
2. Project 设置下新增 `definitions` 子路由，用来配置允许 project scope 的 `ConfigDefinition`。
3. Project 设置下的 `config` 子路由完整展示 project value，包括显式值、workspace 继承值、schema default、缺失必填项、secret 遮掩和类型化控件。
4. `ConfigDefinition` 增加可选的 Web Console 首页展示开关，用于把非必要但常看的配置值展示到首页。
5. 新增/编辑 project value 时，key 必须从当前 workspace 的 `ConfigDefinition` 中选择；workspace-only key 可展示但不可写 project 显式值。
6. 修改 `ConfigDefinition.value_type` 时，如果 `configs` 值表中已有同 key value，后端必须拒绝修改。
7. 删除 `ConfigDefinition` 时，前端明确提示“会同时删除所有 workspace/project value”；用户确认后使用 `purge=true` 删除。
8. 类型、枚举、默认值、required、secret、allowed scopes、首页展示开关的 Web Console 表达必须和后端契约一致。

## 4. 非目标

- 本次不做 workspace value 配置页。`/settings` 只管理 `ConfigDefinition`。
- 不新增 project 私有 `config_definitions` 表。
- 不改 `configs` 值表结构。
- 不把 shared config 混入 task UDA、任务 JSON、query 或 urgency。
- 不做 config 模板市场、跨 workspace 复制或导入导出。
- 不改变 CLI/MCP/remote CLI 的既有命令名。

## 5. 路由设计

### 5.1 Workspace 设置

```text
/settings
  workspace ConfigDefinition 管理页
  暂不提供 workspace value 编辑
```

导航侧栏中“设置”仍指向 `/settings`，但页面标题和内容改为“配置定义”。这避免新增一个只有单页内容的中间页。

### 5.2 Project 设置

```text
/projects/{slug}/settings
  redirect -> /projects/{slug}/settings/config

/projects/{slug}/settings/config
  project value 管理页

/projects/{slug}/settings/definitions
  project ConfigDefinition 管理页

/projects/{slug}/settings/notes
  项目备注页，保留现有能力
```

Tab 顺序建议：

```text
[配置值] [配置定义] [项目备注]
```

理由：项目操作者最常用的是写 project value；定义是较低频的控制面；备注是辅助信息。

### 5.3 Workspace 与 Project 定义页的关系

两个定义页复用同一套 `ConfigDefinitionManager` 组件，只传入不同默认过滤和默认值：

| 页面 | 过滤 | 新建默认 allowed scopes |
|---|---|---|
| `/settings` | 默认展示当前 workspace 内全部定义，可按 `workspace/project/both` 过滤 | `["workspace"]` |
| `/projects/{slug}/settings/definitions` | 展示 `allowed_scopes` 包含 `project` 的定义，也允许展示 both | `["project"]` |

如果一个定义同时允许 workspace/project scope，两个页面都能看到它。编辑时仍编辑同一条定义。`/settings` 是全局控制面，project definitions 页是项目上下文里的 project-scope 快捷控制面。

## 6. 后端规则补强

### 6.1 禁止有值时修改 value_type

当前 `ConfigSchemaSet` 更新已有定义时应增加保护：

```text
existing.value_type != input.value_type
AND CountByKey(workspace_id, key).total > 0
=> config_definition_type_locked
```

错误语义：

```json
{
  "code": "config_definition_type_locked",
  "message": "config definition \"ads.budget\" has existing values; value_type cannot be changed"
}
```

原因：即使某些旧值可以被新类型解析，也不能让 schema 类型在已有 value 上静默漂移。类型变化应该通过新 key、迁移 value、删除旧定义三步显式完成。

### 6.2 删除定义时 purge 所有 value

删除定义保留现有两段式后端语义：

- `DELETE /api/v1/config-schema/{key}` 或 `purge=false`：如果已有 value，返回 `config_definition_in_use`。
- `DELETE /api/v1/config-schema/{key}?purge=true`：删除同 workspace 下所有 `configs.key = key` 的 workspace/project value，再删除 definition。

Web Console 不走“先失败再提示”的体验。删除按钮应先展示确认弹窗，确认文案明确说明会删除所有 value，然后直接发 `purge=true`。

### 6.3 allowed_scopes 收窄保护

本次建议顺手补齐作用域收窄保护，避免定义改完后留下不可解释的旧值：

- 如果从 `allowed_scopes` 中移除 `workspace`，但存在 workspace value，拒绝，返回 `config_definition_scope_locked`。
- 如果从 `allowed_scopes` 中移除 `project`，但存在 project value，拒绝，返回 `config_definition_scope_locked`。
- 扩大 scopes 允许。
- label/description/required/secret 可直接改。

### 6.4 enum_values 修改保护

如果已有 value，编辑枚举应采用“所有现有值仍合法”规则：

- 新枚举为空：允许，相当于取消枚举限制。
- 新枚举非空：必须包含所有已有 workspace/project value，否则返回 `config_definition_enum_locked`。

这样可以允许追加枚举项和改文案，但禁止把已有值变成非法值。

### 6.5 默认值修改

`default_value` 不存入 `configs`，只影响未显式配置时的 effective value。修改默认值允许，但必须符合当前类型和枚举规则。

### 6.6 首页展示元数据

`ConfigDefinition` 增加可选字段：

```go
ShowOnConsoleHome bool
```

HTTP JSON 字段为：

```json
{
  "show_on_console_home": true
}
```

语义：

- 默认 `false`，不改变现有定义。
- 只控制 Web Console 首页是否展示该 key 的 effective value。
- 不是必填约束，不影响 `required`、类型校验、枚举校验、默认值、继承链或 CLI/MCP 行为。
- 允许 workspace/project/both scope 的定义配置该开关。
- `secret=true` 的定义可以保存 `show_on_console_home=true`，但首页只展示遮掩值，不提供一键 reveal。需要查看 secret 时仍回到设置页。
- 排序首版按 key 升序，不新增排序字段。后续如果首页展示项变多，再单独设计 `console_home_order`。

## 7. API 补充

现有 schema CRUD 可复用，但需要扩展 `ConfigDefinitionView` / `configSchemaRequest`：

```json
{
  "key": "ads.roi_threshold",
  "value_type": "number",
  "allowed_scopes": ["workspace", "project"],
  "label": "ROI 阈值",
  "description": "",
  "enum_values": [],
  "default_value": "1.8",
  "required": false,
  "secret": false,
  "show_on_console_home": true,
  "created_at": 1780000000,
  "modified_at": 1780000000
}
```

此外，Web Console 需要知道 value 使用情况，避免删除和编辑时盲操作。

新增只读 usage API：

```http
GET /api/v1/config-schema/{key}/usage?workspace={slug}
```

响应：

```json
{
  "key": "ads.budget",
  "workspace_values": 1,
  "project_values": 12,
  "total_values": 13
}
```

可选优化：`GET /api/v1/config-schema/usage` 返回所有 key 的 count map，列表页一次加载计数。实现计划可根据复杂度选择；最低要求是删除/编辑前能查询单 key usage。

Project value 页需要 effective 视图。首选新增 endpoint：

```http
GET /api/v1/projects/{ref}/config/effective?workspace={slug}
```

响应数组按 key 排序：

```json
[
  {
    "key": "ads.budget",
    "value": "1000",
    "source": "project",
    "project_value": "1000",
    "workspace_value": "800",
    "default_value": "500",
    "definition": { "...": "ConfigDefinitionView" },
    "show_on_console_home": true,
    "missing_required": false
  }
]
```

如果后端暂不新增 effective endpoint，前端也可以用 `config-schema` + `project config list` + `workspace config list` 拼出同等视图。但为了权限、secret、继承链和 required 缺失语义长期稳定，建议把 effective 视图放在 app/http 层。

## 8. Workspace ConfigDefinition 页面

### 8.1 页面结构

```text
+--------------------------------------------------------------------+
| 设置                                                               |
| 配置定义控制哪些 workspace/project 配置 key 可以被写入。            |
+--------------------------------------------------------------------+
| [搜索 key / label] [作用域: 全部 v] [类型: 全部 v]        [+ 新建] |
+--------------------------------------------------------------------+
| Key                     类型     作用域       默认值      Values   |
| ads.budget              number   workspace    500         W:1 P:0  |
| integrations.webhook    string   project      -           W:0 P:8  |
| agent.background        string   project      -           W:0 P:3  |
+--------------------------------------------------------------------+
| 选中行右侧抽屉：                                                    |
| Key: ads.budget (不可改)                                            |
| Label: [广告预算]                                                   |
| Type:  (number v)                                                   |
| Scopes: [x] workspace [ ] project                                   |
| Enum:  [空]                                                         |
| Default: [500]                                                      |
| [x] Required   [ ] Secret   [x] 显示在首页                           |
| [保存] [删除定义...]                                                |
+--------------------------------------------------------------------+
```

### 8.2 列表

列：

- key：等宽显示，支持搜索。
- label/description：辅助说明。
- value_type：`string | number | boolean | json`。
- allowed_scopes：badge 显示 `workspace` / `project`。
- default_value：secret 默认值遮掩。
- required/secret：badge。
- 首页展示：badge 或小图标，表示该 key 会出现在 Web Console 首页。
- values：显示 usage count，例如 `W:1 P:8`。
- 操作：编辑、删除。

### 8.3 新建/编辑表单

字段：

- key：新建可填，编辑不可改。rename 必须通过新建新 key + 迁移 value + 删除旧 key。
- value_type：select。
- allowed_scopes：checkbox 组，至少选一个。
- label：短文本。
- description：textarea。
- enum_values：非 json 类型可填；一行一个值或逗号分隔，提交前按后端规则归一化。
- default_value：按 value_type 渲染控件。
- required：checkbox。
- secret：checkbox。
- show_on_console_home：checkbox，文案为“显示在首页”。说明文案：“仅影响 Web Console 首页展示；不是必填配置。”

编辑时，如果 usage total > 0：

- value_type 控件禁用，并提示“已有配置值，不能修改类型”。
- 移除已有 value 所在 scope 的 checkbox 禁用。
- enum_values 仍可编辑，但保存失败需展示后端错误。

### 8.4 删除确认

删除按钮打开 destructive dialog：

```text
删除 ConfigDefinition？

将删除定义 `ads.budget`，并同时删除此 workspace 下所有同 key 的配置值：
- workspace value: 1
- project value: 12

此操作不可撤销。请输入 key 确认。

[取消] [删除定义和值]
```

确认后调用 `DELETE /api/v1/config-schema/ads.budget?purge=true`。

## 9. Project ConfigDefinition 页面

Project 定义页复用 workspace 定义页的管理能力，但默认聚焦 project scope：

```text
+--------------------------------------------------------------------+
| agentapi / 设置 / 配置定义                                          |
| 这些定义决定本项目页面可配置哪些 project config value。              |
+--------------------------------------------------------------------+
| [搜索] [只看 project 可写: 开]                            [+ 新建] |
+--------------------------------------------------------------------+
| Key                          类型     作用域              Values   |
| agent.background             string   project             W:0 P:3  |
| integrations.feishu.webhook  string   project, workspace  W:1 P:6  |
+--------------------------------------------------------------------+
```

规则：

- 列表默认展示 `allowed_scopes` 包含 `project` 的定义。
- 新建默认勾选 `project`。
- 如果用户同时勾选 `workspace`，该定义也会出现在 `/settings`。
- 删除定义的影响仍是 workspace 全局的：会删除所有项目下同 key value，不只是当前 project 的 value。确认弹窗必须写清楚。

## 10. Project Config Value 页面

### 10.1 页面目标

Project value 页不是裸 key/value 表，而是“当前 project 的 effective 配置控制台”。它应同时展示：

- project 显式值。
- workspace 继承值。
- schema default。
- required 但缺失的项。
- workspace-only 不能被 project 覆盖的项。

### 10.2 ASCII 原型

```text
+--------------------------------------------------------------------------------+
| agentapi / 设置 / 配置值                                                        |
| 当前项目的配置值。读取顺序：project 显式值 > workspace 值 > schema default。      |
+--------------------------------------------------------------------------------+
| [搜索 key / label] [来源: 全部 v] [状态: 全部 v]                    [+ 添加值] |
+--------------------------------------------------------------------------------+
| Key                         类型      来源        当前值             状态       |
| agent.background            string    project     Owns MCP...        已覆盖     |
| integrations.feishu.url     string    workspace   https://...        继承       |
| ads.budget                  number    default     500                默认       |
| ads.owner                   string    missing     -                  必填缺失   |
| runtime.global_timeout      number    workspace   30                 只读       |
+--------------------------------------------------------------------------------+
| 行操作： [编辑] [恢复继承]                                                      |
+--------------------------------------------------------------------------------+
```

### 10.3 添加值

点击“添加值”打开 dialog：

```text
+------------------------------------------------------+
| 添加项目配置值                                       |
| Key                                                  |
| [选择 key v]                                         |
|                                                      |
| 可选 key 分组：                                      |
| Project 可写                                         |
|   agent.background        string                     |
|   integrations.feishu.url string secret              |
| Workspace-only（不可覆盖，仅展示继承）                |
|   runtime.global_timeout  number disabled            |
|                                                      |
| Value                                                |
| [根据 definition 自动切换控件]                        |
|                                                      |
| [取消] [保存]                                        |
+------------------------------------------------------+
```

key 列表来源：

- 全部当前 workspace `ConfigDefinition`。
- 可写项：`allowed_scopes` 包含 `project` 且当前 project 没有显式 value。
- 不可写项：仅包含 `workspace` 的定义，展示但 disabled，说明“此 key 只能在 workspace value 中设置；本阶段 Web Console 暂不支持 workspace value 编辑”。

### 10.4 行内编辑控件

控件按 definition 渲染：

| Definition | 控件 |
|---|---|
| `enum_values` 非空 | select |
| `value_type=string` | input；长文本可 textarea |
| `value_type=number` | number input |
| `value_type=boolean` | switch 或 true/false segmented control |
| `value_type=json` | textarea + JSON parse 错误提示 |
| `secret=true` | password input + reveal button；列表默认遮掩 |

保存时仍由后端做最终校验。前端校验只用于提前提示，不替代后端。

### 10.5 删除 project value

删除 project 显式值的按钮文案用“恢复继承”，而不是“删除 key”。行为：

- 调用 `DELETE /api/v1/projects/{ref}/config/{key}`。
- 删除后该行仍存在，但 source 变为 `workspace`、`default` 或 `missing`。
- 如果没有继承值且不是 required，可在过滤器中隐藏，但默认 effective 表仍展示所有 definition。

### 10.6 Definition 变化后的表现

因为 definition 是控件来源，project value 页必须监听 schema query：

- definition 更新后，value 控件即时按新 definition 渲染。
- definition 删除且 purge value 后，effective 列表不再显示该 key。
- 如果后端拒绝类型变更，前端保留旧 definition 并展示错误。
- 如果 enum/default/description 更新，刷新 query 后列表和编辑表单同步变化。

## 11. Web Console 首页展示

### 11.1 展示目标

Web Console 首页（当前 `/` 的 `OverviewPage`）增加一个轻量的“配置概览”区，只展示被 `ConfigDefinition.show_on_console_home=true` 标记的非必要配置。首页展示面向人阅读，主显示字段必须是 `ConfigDefinition.label` 加 effective config value，而不是裸 key 加 value。

这个区的目标是让操作者快速看到常用业务配置是否已经有值，例如默认通知地址、默认上下文、广告预算阈值等。它不是配置完整性告警区，也不负责写值。`key` 只作为辅助信息显示在 label 下方或 tooltip 中。

### 11.2 首页解析规则

首页没有单一 project 上下文，因此首版只稳定展示 workspace 可解析的值：

- `allowed_scopes` 包含 `workspace`：显示 workspace explicit value；没有 explicit value 时显示 schema default；两者都没有时显示“未配置”。
- `allowed_scopes` 仅包含 `project`：不在 workspace 首页展示具体值，因为无法无歧义选择 project；可以在定义管理页保留开关，但首页过滤掉此类 key。
- `allowed_scopes=["workspace","project"]`：首页显示 workspace 层 effective value，不展开每个 project 的覆盖值。

Project 级首页展示如果后续需要，应在 project 工作台单独设计，不混入本次 Web Console 首页。

### 11.3 ASCII 原型

```text
+--------------------------------------------------------------------+
| 配置概览                                             [去配置定义]   |
+--------------------------------------------------------------------+
| 配置                         值                         来源         |
| 默认通知地址                  https://example...        workspace    |
| integrations.default_sink                                            |
| ROI 阈值                      1.8                       default      |
| ads.roi_threshold                                                    |
| 默认上下文                    未配置                    missing      |
| agent.default_context                                                |
+--------------------------------------------------------------------+
```

展示规则：

- 第一列主文本显示 `definition.label`；如果 label 为空，fallback 为 key。
- raw key 作为次要文本显示，使用等宽小字号，不作为主视觉。
- secret 值显示为 `••••••`，不提供 reveal。
- JSON 值显示单行摘要，点击该配置行可进入 `/settings` 的定义详情。
- missing 不当成错误，只用弱提示；因为这个开关表达“可展示”，不是“必须配置”。
- 首页区只读；编辑入口统一回 `/settings` 或 project settings。

### 11.4 API 选择

首选新增 workspace effective config endpoint：

```http
GET /api/v1/config/effective?workspace={slug}&console_home=true
```

响应：

```json
[
  {
    "key": "ads.roi_threshold",
    "value": "1.8",
    "source": "default",
    "workspace_value": null,
    "default_value": "1.8",
    "definition": { "...": "ConfigDefinitionView" },
    "show_on_console_home": true,
    "missing_required": false
  }
]
```

如果实现计划为了减少后端面，也可以由前端使用 `GET /api/v1/config-schema` + `GET /api/v1/config` 拼出首版首页数据。但长期建议放在 app/http 层，因为 secret 遮掩、类型摘要和 source 语义都更适合后端统一。

## 12. 权限与只读

权限沿用现有 scope 与 permission：

| 操作 | Scope | Permission |
|---|---|---|
| 读 definition | `config:read` | `PermissionConfigSchemaRead` |
| 写 definition | `config:write` | `PermissionConfigSchemaWrite` |
| 读 project value | `project:read` / `config:read` | 现有 project config read |
| 写 project value | `project:write` / `config:write` | 现有 project config write |

只读状态：

- 无写权限时隐藏新建、保存、删除按钮。
- project `archived/cancelled` 时，project value 页只读；definition 页仍按 workspace 权限决定，因为定义不是项目实体的一部分。

## 13. 文案原则

页面文案用直接行为描述，不用类比。

建议中文文案：

- `配置定义`：定义哪些配置 key 可以被写入，以及值的类型、作用域和默认值。
- `配置值`：当前项目的实际配置。读取顺序：项目值 > 工作区值 > 默认值。
- `恢复继承`：删除当前项目的显式值，改为读取工作区值或默认值。
- `已有配置值，不能修改类型`：用于 value_type 锁定提示。
- `删除定义和值`：用于 purge 删除按钮。
- `显示在首页`：此配置值可出现在 Web Console 首页；不是必填配置。

## 14. 文件结构建议

```text
web/src/routes/workspace/
  SettingsRoute.tsx                         # /settings，不再使用 ResourceRoute
  ProjectSettingsDefinitionsRoute.tsx       # /projects/$projectSlug/settings/definitions

web/src/pages/
  OverviewPage.tsx                          # 增加首页配置概览区
  config-definitions-page.tsx               # workspace/project 两处复用
  project-config-tab.tsx                    # 升级为 effective value 视图

web/src/features/workspace/config/
  config-definition-api.ts                  # schema CRUD + usage
  config-definition-manager.tsx             # 列表、过滤、抽屉、删除确认
  config-definition-form.tsx                # 新建/编辑表单
  config-value-control.tsx                  # 按 definition 渲染 value 控件

web/src/features/workspace/project-workbench/api/
  project-api.ts                           # 增加 effective config API 或前端拼装
```

后端可能涉及：

```text
internal/app/scoped_config.go               # type/scope/enum 锁定规则
internal/storage/config_repo.go             # usage count/list helpers
internal/httpapi/context_config.go          # usage/effective endpoint
internal/httpapi/huma_routes.go             # 注册新路由
```

## 15. 测试与验收

### 15.1 后端测试

必须新增或补充：

- `ConfigSchemaSet`：已有 value 时修改 `value_type` 返回 `config_definition_type_locked`。
- `ConfigSchemaSet`：移除有 value 的 scope 返回 `config_definition_scope_locked`。
- `ConfigSchemaSet`：新 enum 不包含已有 value 返回 `config_definition_enum_locked`。
- `ConfigSchemaSet/Get/List`：`show_on_console_home` 可保存、读取，默认 false。
- `ConfigSchemaDelete(purge=true)`：删除 definition 后 workspace/project value 都被删除。
- effective config endpoint：按 `project > workspace > default > missing` 返回。
- workspace 首页 effective endpoint：只返回 `show_on_console_home=true` 且 workspace 可解析的定义。
- SQLite 与 PostgreSQL 现有 config 测试继续通过。

### 15.2 前端测试

必须覆盖：

- `/settings` 不再请求 `/api/v1/config`，改请求 `/api/v1/config-schema`。
- workspace definition 页能新建、编辑、删除定义。
- definition 表单包含“显示在首页”开关，并随保存请求提交 `show_on_console_home`。
- 删除定义弹窗展示 workspace/project value count，并调用 `purge=true`。
- 已有 value 时 type 控件禁用。
- project definition 页只展示 project-scope 定义。
- project value 页展示 project/workspace/default/missing 四类来源。
- 添加 value 的 key selector 只允许选择 project scope key。
- `boolean/number/json/enum/secret` 控件按 definition 切换。
- “恢复继承”删除 project 显式值后仍显示继承/default 行。
- 首页只展示 `show_on_console_home=true` 的 workspace 可解析配置；secret 值遮掩；project-only key 不在首页显示。

### 15.3 验证命令

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
pnpm --dir web typecheck
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web build
git diff --check
```

如果只落前端 spec 不实现代码，至少运行：

```bash
git diff --check
```

## 16. 迁移与兼容

- 现有 `/settings` 从 value 表变为 definition 表，是 Web Console 语义修正。CLI/API 的 `/api/v1/config` 不删除。
- 原有 project settings 的 `config` 子路由保留，避免破坏现有项目行菜单入口。
- 新增 `definitions` 子路由不会影响 `/projects/{slug}/settings/notes`。
- 删除 definition 的 `purge=true` 是已有后端能力的 UI 化；需要加强确认，不应做无提示删除。
- 类型锁定是后端行为收紧。如果已有自动化尝试在有 value 时改类型，会收到明确错误；这是预期保护。
- `show_on_console_home` 默认 false，迁移后不会改变任何首页展示；只有用户显式开启的定义才会出现在首页。

## 17. 推荐实施顺序

1. 先补后端 `ConfigDefinition` 更新锁定规则和 usage/effective API。
2. 再把 `/settings` 从 `ResourceRoute` 拆成专门的 `SettingsRoute`，落 workspace definition 管理页。
3. 再给 project settings 增加 `definitions` tab。
4. 最后升级 project value 页为 effective 视图和类型化控件。
5. 在首页增加配置概览区，接入 `show_on_console_home`。

这样做的好处是：先把规则守住，再做控制面；先让定义可控，再让 value 页面依赖定义做更完整的可视化。
