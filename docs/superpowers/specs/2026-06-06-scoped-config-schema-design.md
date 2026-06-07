# Xuanchu Scoped Config Schema 设计规格

> **给 agentic workers 的要求：** 编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

**目标：** 设计一套可定义 schema 的 shared scoped config 机制，统一承载 `workspace` / `project` 两层共享业务配置，支持类型、枚举、默认值、作用域限制、审计与权限控制。

**范围策略：** 本规格只覆盖 shared config schema 与 shared config value，不重写 task UDA、本机启动配置、query/DOM/urgency 对 UDA 的现有语义，也不把 shared config 混入 task JSON。设计必须继续复用现有 `configs` 值表、workspace 隔离边界、project 实体化、HTTP/MCP/remote CLI 的 project scope 解析链路，并且必须同时支持 SQLite 与 PostgreSQL。

**需求来源：**

- [ROADMAP.md](/Users/mac/code/projects/dajee/task/ROADMAP.md) 中 M3 配置/UDA、M5 project config、M6/M7/M15 的 API/MCP/config 边界。
- [README.md](/Users/mac/code/projects/dajee/task/README.md) 与 `docs/manual/*` 中关于 workspace/project/config 的用户可见语义。
- 当前实现中的 `internal/storage/config_repo.go`、`internal/app/project_config.go`、UDA 设计与测试边界。
- 当前业务诉求：电商众筹团队希望一个产品一个 project，同时希望广告投放配置能在 project 下自由扩展，并且严格受 workspace 隔离。

---

## 1. 当前基础与问题

当前项目已经具备：

- `workspace` 作为租户级隔离边界。
- `project` 作为 workspace 内一等实体。
- 通用 `configs` 表，按 `(workspace_id, scope, scope_id, key)` 存共享配置值。
- `project config get/set/unset/list` CLI / HTTP / MCP / remote CLI 路径。
- `task UDA` 的 schema + value 独立机制。

当前 project config 的主要问题不是存储层，而是控制面：

- `configs` 表已经支持 `scope=project`，值存储能力已经存在。
- App 层通过硬编码白名单限制 project config key。
- 新业务一旦要扩展 config key，就必须改代码。
- `workspace config` 和 `project config` 没有统一 schema 定义能力。
- 没有统一的类型校验、枚举校验、默认值、secret 语义。

这导致当前 project config 只适合少量固定 key，不适合“一个众筹产品一个 project、不同产品需要不同广告配置”的场景。

## 2. 设计目标

本规格完成后，应支持以下能力：

```bash
# 在 workspace 内定义共享配置 schema
xuanchu config schema set ads.account_id type:string scopes:project label:"Meta Account ID"
xuanchu config schema set ads.roi_threshold type:number scopes:workspace,project default:1.8
xuanchu config schema set ads.channel_enabled type:boolean scopes:project default:true

# 写 workspace 值
xuanchu --workspace dajee config set ads.roi_threshold 2.0

# 写 project 值
xuanchu --workspace dajee project config set product-a ads.account_id act_123
xuanchu --workspace dajee project config set product-a ads.roi_threshold 1.6

# 读 project 值：project > workspace > default
xuanchu --workspace dajee project config get product-a ads.roi_threshold

# 列出 schema 和有效值
xuanchu --workspace dajee config schema list
xuanchu --workspace dajee project config list product-a
```

产品级广告配置典型示例：

- `ads.account_id`
- `ads.timezone`
- `ads.currency`
- `ads.owner_user_id`
- `ads.alert.roi_drop_threshold`
- `ads.alert.cpa_spike_threshold`
- `ads.agent.playbook`

设计目标总结：

- 允许在不改代码的情况下新增 shared config key。
- shared config schema 必须按 workspace 隔离。
- shared config value 必须按 workspace / project 作用域隔离。
- project 不能读取、写入、继承到其它 workspace 的 schema 或值。
- `project config` 的合法性不再由代码白名单决定，而由 workspace 内 schema 定义决定。

## 3. 非目标

本规格不做：

- 重写 task UDA schema/value。
- 让 shared config 进入 task JSON import/export。
- 让 shared config 进入任务查询语法、DOM `_get`、urgency 计算。
- 改造 `xuanchu.toml`、环境变量、`--db`、`--server` 等本机/启动配置来源。
- 自动生成广告领域 DSL、报表系统或策略引擎。
- 跨 workspace 共享 schema 或模板市场。
- project 级 RBAC 覆盖 workspace role。

## 4. 核心边界

### 4.1 shared config 与 UDA 的边界

两者都具有“schema + value”的外观，但语义不同：

- `UDA`
  - 作用于 task。
  - 进入 task JSON import/export。
  - 可参与 query、DOM、helper、urgency。
  - 允许 orphan UDA 保留兼容。

- `shared config`
  - 作用于 `workspace` / `project`。
  - 不进入 task JSON。
  - 不参与 task query / DOM / urgency。
  - 不允许 orphan config value。
  - 更像控制面元数据，不是任务字段。

shared config 不应复用 UDA value 表，也不应强行合并成一个超级通用字段系统。

### 4.2 workspace 隔离是硬约束

本规格把 workspace 隔离提升为 shared config schema/value 的硬约束：

- schema definition 归属于单一 workspace。
- workspace A 中定义的 key，不自动出现在 workspace B。
- project scope 的 key 解析必须先绑定 effective workspace，再在该 workspace 内解析 project。
- 不允许通过 project id、scope id、HTTP 参数、MCP 参数或 remote CLI 组合绕过 workspace 边界。
- 如果请求同时提供 `workspace` 和 `project_id`，该 project 必须属于该 workspace。
- 如果 schema 或值试图写入其它 workspace，必须返回清晰错误，而不是静默回退。

### 4.3 本机配置与 shared config 分离

shared config 只承载共享业务配置，不承载本机启动配置。

继续保留：

- `xuanchu.toml`
- 环境变量
- `--db` / `--db-url` / `--server`
- 本地显示配置

这些仍然是“调用者本机或运行时配置”，不进入 scoped shared config schema。

## 5. 数据模型

### 5.0 双数据库约束

本规格必须遵循 M14 已建立的多数据库边界：

- SQLite：`github.com/glebarez/sqlite`
- PostgreSQL：`gorm.io/driver/postgres`
- GORM 仍是唯一数据库抽象层
- 保持 `CGO_ENABLED=0`

因此，本规格中的“表结构”描述是**逻辑字段模型**，不是要求手写某一方言专用 DDL。实现时应优先通过 GORM model + dialect 适配落地，避免把 SQLite 专属类型或 PostgreSQL 专属类型硬编码进设计。

### 5.1 继续保留 `configs` 作为值表

本规格不重写现有值表，继续使用 M5 已引入的 `configs`：

逻辑字段：

- `workspace_id`
- `scope`
- `scope_id`
- `key`
- `value`

主键：

- `(workspace_id, scope, scope_id, key)`

作用域规则继续保持：

- `scope=workspace`
  - `workspace_id = <workspace_id>`
  - `scope_id = <workspace_id>`
- `scope=project`
  - `workspace_id = <workspace_id>`
  - `scope_id = <project_id>`

### 5.2 新增 `config_definitions` 表

新增 shared config schema 定义表，例如：

逻辑字段：

- `workspace_id`
- `key`
- `value_type`
- `allowed_scopes_json`
- `label`
- `description`
- `enum_values_json`
- `default_value`
- `has_default`
- `required`
- `secret`
- `created_at`
- `modified_at`

主键：

- `(workspace_id, key)`

规则：

- schema definition 只在 workspace 内唯一，不做全局 key 注册。
- 同名 key 可以存在于不同 workspace，但定义可以不同。
- `allowed_scopes_json` 必须至少包含 `workspace` 或 `project` 中的一项。
- `default_value` 是否生效由 `has_default` 决定，不用空字符串歧义表达。
- `secret` 表示值属于敏感配置，读写和审计要特殊处理。
- 布尔字段在 SQLite 与 PostgreSQL 下的具体物理类型可以不同，但语义必须一致；实现上应沿用 M14 对 GORM 布尔映射的处理方式，不写死某一数据库方言。
- `created_at` / `modified_at` 继续沿用当前项目在 storage model 中使用的整数时间戳语义，不在本规格中切换为数据库原生 timestamp 类型。

### 5.3 不新增 `project_configs` 表

不新增单独 `project_configs` 表，原因：

- 现有 `configs` 已经满足 project value 存储。
- 当前需求是补 schema 定义能力，不是重构值表。
- 继续复用 `configs` 能保持值模型简单，不必为了 schema 能力重做值表。

## 6. 类型系统

首版 shared config schema 支持以下类型：

- `string`
- `number`
- `boolean`
- `json`

规则：

- `string`：按原样存文本。
- `number`：接受十进制字符串；比较、校验和默认值都按 numeric 语义处理。
- `boolean`：固定接受 `true` / `false`。
- `json`：写入前必须能解析为合法 JSON；存储仍为文本。

首版不引入：

- `date`
- `duration`
- `secret-string` 单独类型
- list/array 专用类型

原因：

- `date` / `duration` 对 shared config 的当前业务价值不高，但会立刻引入格式、时区、显示语义分歧。
- `secret` 更适合作为 schema 属性，而不是独立类型。
- 复杂集合先用 `json` 即可。

## 7. schema 语义

每个 shared config definition 至少包含：

- `key`
- `value_type`
- `allowed_scopes`

可选元数据：

- `label`
- `description`
- `enum_values`
- `default_value`
- `required`
- `secret`

规则：

- `enum_values` 只对 `string` / `number` / `boolean` 生效；`json` 首版不支持枚举。
- `required=true` 表示该 key 在对应 scope 写值时必须存在明确值，不能只靠缺省读取。
- `default_value` 必须能通过类型校验和枚举校验。
- `secret=true` 的 key 不应在普通 list / audit / MCP rendered 文本中泄露明文。

## 8. 读取与继承规则

首版只支持简单、稳定、可解释的读取链：

### 8.1 workspace 读取

`workspace` scope 读取规则：

1. 显式 workspace value
2. schema default
3. 不存在

### 8.2 project 读取

`project` scope 读取规则：

1. 显式 project value
2. 显式 workspace value
3. schema default
4. 不存在

注意：

- 这里的 workspace value 指“同一个 workspace 下、同一个 key 的 workspace-scope 值”。
- 不存在跨 workspace 回退。
- 不存在跨 project 回退。
- 不存在读取本机 TOML 作为业务默认值。

### 8.3 project 只读允许 project scope 的 key

若某 key 的 `allowed_scopes` 不包含 `project`：

- `project config get/set/unset/list` 不应把它当作合法 project key。
- `project config get` 对此类 key 应返回稳定错误，而不是悄悄回退。

这是为了防止“所有 workspace config 都能从 project 侧访问”的语义泄露。

## 9. CLI 设计

### 9.1 schema 管理命令

新增命令组：

```bash
xuanchu config schema list
xuanchu config schema get <key>
xuanchu config schema set <key> type:<type> scopes:<workspace|project|workspace,project> [label:<text>] [description:<text>] [values:<csv>] [default:<value>] [required:true|false] [secret:true|false]
xuanchu config schema delete <key> [--purge]
```

说明：

- schema 命令的作用域由 effective workspace 决定。
- 没有 effective workspace 时，shared config schema 不允许创建或修改。
- schema 只在当前 workspace 内操作，不支持 `--workspace all`、不支持跨 workspace 批量下发。

### 9.2 value 命令

继续保留现有值命令心智：

- `config get/set/unset/list` 处理 workspace 值。
- `project config get/set/unset/list <project>` 处理 project 值。

变化点：

- 不再依赖 project config 代码白名单。
- workspace / project 值写入前，都必须先验证 schema 存在且 scope 合法。
- `list` 默认返回“该 scope 下显式值”；如需显示继承后的有效值，可以后续单独扩展，不在本版强行加入。

## 10. HTTP / MCP / remote CLI 设计

### 10.1 HTTP

新增 schema endpoint，例如：

- `GET /api/v1/config-schema`
- `GET /api/v1/config-schema/{key}`
- `PUT /api/v1/config-schema/{key}`
- `DELETE /api/v1/config-schema/{key}`

规则：

- 必须先完成 auth，再按 effective workspace 解析。
- schema 写入必须绑定 workspace。
- 不允许无 workspace 地创建 shared schema。

### 10.2 MCP

新增 tools：

- `config_schema_list`
- `config_schema_get`
- `config_schema_set`
- `config_schema_delete`

现有这些 tool 保持“值读写”语义：

- `config_get`
- `config_set`
- `config_unset`
- `project_config_set`
- `project_config_unset`
- `project_config_list`

Agent 语义：

- 管理型 agent 才应该拥有 schema 写权限。
- 日常巡检和诊断 agent 原则上只读写已有 value，不偷偷新增 schema。

### 10.3 remote CLI

remote CLI 延续现有命令形态，不新增另一套语法。  
唯一变化是：远程 `project config` 的 key 校验由服务端 workspace schema 决定，而不是本地二进制白名单。

## 11. 权限与审计

### 11.1 权限

建议新增共享配置 schema 专用权限：

- `PermissionConfigSchemaRead`
- `PermissionConfigSchemaWrite`

首版角色建议：

- `viewer`
  - 可读 schema
  - 可读 workspace/project config
- `member`
  - 可读 schema
  - 按既有边界继续写 task、context 等
  - 不增加 schema 写权限
- `admin`
  - 可写 schema
  - 可写 workspace shared config
  - 可写 project config
- `owner`
  - 拥有 admin 的 schema/config 权限

### 11.2 审计

必须进入 audit：

- `config.schema.set`
- `config.schema.delete`
- `config.set`
- `config.unset`
- `project.config.set`
- `project.config.unset`

审计要求：

- schema 写审计必须带 `workspace_id`。
- project config 写审计必须带 `workspace_id` 和 `project_id`。
- `secret=true` 的 key 写值时，audit payload 不记录明文 value，只记录：
  - key
  - scope
  - secret=true
  - changed=true

## 12. workspace 隔离与错误语义

本规格新增或固定以下边界：

- schema 不存在：`config_definition_not_found`
- scope 不允许：`config_scope_not_allowed`
- 类型不匹配：`config_value_invalid`
- 枚举不匹配：`config_value_invalid`
- 删除仍被值引用的 schema：`config_definition_in_use`
- 试图跨 workspace 访问 project/schema/value：沿用现有 `workspace_scope_denied`、`project_not_found`、`project_mismatch` 等稳定错误，不新增模糊兜底错误

特别要求：

- 不允许 project 值引用不属于当前 workspace 的 schema。
- 不允许用 project id 直接越过 workspace 去读写 project config。
- 不允许一个 workspace 的 schema 被另一个 workspace 的 config value 复用。

## 13. 删除与兼容规则

### 13.1 不支持 orphan config value

shared config 首版明确不支持 orphan value：

- 删除 schema 时，如果该 workspace 下仍存在任一 workspace/project 值，默认拒绝。
- 只有显式 `purge` 语义时，才允许连同所有对应值一起删除。

这点故意与 UDA 不同，因为 shared config 是控制面，静默保留孤儿值会让行为失控。

### 13.2 类型变更

修改 schema 时：

- 若已有值与新类型不兼容，拒绝修改。
- 若已有值与新枚举不兼容，拒绝修改。
- 不能通过“先改 schema、让旧值变脏”的方式进入不一致状态。

### 13.3 初始内置 key

首版可以预置少量系统内置 key，例如：

- `agent.background`
- `agent.constraints`
- `agent.default_context`
- `agent.handoff`
- `context.default`

这些 key 只是首版产品自带 definition，不是“唯一合法 key 来源”。

规则：

1. App 层不再用硬编码白名单作为最终合法性判断，而改为查询 schema definition。
2. 这些内置 key 可以在 workspace 初始化时自动写入 definition，也可以由安装脚本显式创建。
3. 新增业务 key 不应再要求改代码。
4. 当前实现选择预置上述少量系统 key，以保持既有 `project config` 常用入口可直接使用。

## 14. 未上线前提与初始化策略

当前项目尚未上线，因此本规格**不以兼容旧数据或平滑迁移为目标**。设计应优先选择更干净、更一致的目标模型。

初始化策略建议：

1. 创建 `config_definitions` 表。
2. 保留 `configs` 作为 shared config 值表。
3. 删除当前 App 层 project config 白名单主逻辑，统一改为 schema 驱动校验。
4. 首版预置极少量系统 key：
   - `agent.background`
   - `agent.constraints`
   - `agent.default_context`
   - `agent.handoff`
   - `context.default`
5. 不需要为历史 workspace、历史 project config 值、旧白名单行为设计保守迁移路径。

规则：

- 不要求保留现有 project config 白名单作为兼容层。
- 不要求保留旧 key 必然存在的假设。
- 如果现有开发数据库里有不符合新模型的测试数据，可以直接清理或重建。
- 首版实现优先保证目标模型一致性，而不是旧行为兼容性。
- SQLite 与 PostgreSQL 的初始化策略可以不同：
  - SQLite 路径允许继续沿用现有历史迁移入口之外追加新表创建
  - PostgreSQL 路径应继续走纯净 schema 创建
- 但两条路径最终暴露的 shared config schema/value 语义必须一致。

## 15. 实现边界建议

建议新增独立边界，而不是把所有逻辑挤进 `project_config.go`：

- `internal/storage/config_definition_repo.go`
  - schema definition CRUD
- `internal/app/config_schema.go`
  - schema 校验、权限、审计、类型验证
- `internal/app/scoped_config.go`
  - workspace/project value 读写与回退规则

现有 `project_config.go` 可收敛为：

- project scope 的 CLI / API / MCP 入口薄封装
- 具体读取/写入逻辑下沉到 scoped config service

## 16. 验收标准

本规格完成后，至少满足：

- 不改代码即可在某个 workspace 内新增合法 shared config key。
- shared config schema 按 workspace 隔离；不同 workspace 可定义同名但不同 schema。
- project config 只允许读取/写入当前 workspace 内允许 project 使用的 key。
- project 读取规则为 `project value > workspace value > schema default`。
- 不允许跨 workspace 读写 schema 或值。
- 首版不要求兼容旧白名单行为；系统会预置少量 `agent.*` / `context.default` schema，其余 key 由 workspace 自行定义。
- `secret` key 不在 audit 和普通 list 中泄露明文。
- shared config 不产生 orphan value。
- CLI / HTTP / MCP / remote CLI 语义一致。
- SQLite 与 PostgreSQL 下 shared config schema/value 行为一致。
- PostgreSQL 环境可用时，相关 storage / app / CLI 集成测试应通过。
- `go test ./...`
- `CGO_ENABLED=0 go test ./...`
- `CGO_ENABLED=0 go build ./cmd/xuanchu`

## 17. 对广告协作场景的直接价值

本规格落地后，你当前的电商众筹产品 project 可以自然承载如下配置，而不需要再改代码白名单：

- `ads.account_id`
- `ads.timezone`
- `ads.currency`
- `ads.owner_user_id`
- `ads.alert.roi_drop_threshold`
- `ads.alert.cpa_spike_threshold`
- `ads.alert.min_spend_gate`
- `ads.agent.playbook`
- `ads.dashboard_url`

并且这些配置具备：

- workspace 隔离
- project 继承 workspace 默认
- 类型校验
- 枚举校验
- secret 控制
- 审计可追踪

这正好适合“一个众筹产品一个 project，投手横跨多个产品协作，Agent 读取产品配置后巡检和写任务”的使用方式。
