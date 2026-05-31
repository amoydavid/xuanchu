# taskg M5 设计规格

> **给 agentic workers 的要求：** 编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

**目标：** 把 `project` 从任务上的自由字符串升级为 workspace 内的一等企业项目实体，并建立 project scope、project 配置边界和后续 API/MCP/Agent token 可复用的稳定 `project_id`。

**范围策略：** M5 不再把“完全兼容 Taskwarrior”作为优先目标。taskg 继续借用 Taskwarrior 的 CLI 风格、查询思路、任务字段和 JSON 迁移经验，但企业场景下 `project` 必须是受权限、审计、配置和作用域约束的业务对象。M5 的关键决策是：运行时采用**严格 project 注册**，新增或修改任务引用不存在的 `project:<slug>` 必须报错，不自动创建 project。

**需求来源：** 本规格从 [ROADMAP.md](/Users/mac/code/projects/dajee/task/ROADMAP.md)、[README.md](/Users/mac/code/projects/dajee/task/README.md)、[docs/requirements.md](/Users/mac/code/projects/dajee/task/docs/requirements.md)、M4 当前实现，以及本轮关于 workspace/project/tenant/Agent MCP 方向的讨论中收束。

---

## 1. 当前基础

M4 已完成并合并到 `main`。当前项目已有：

- Go 1.22 module。
- Cobra CLI。
- GORM + `github.com/glebarez/sqlite`，保持零 CGO。
- 本地多用户、多 workspace、membership、权限和 audit。
- `workspace` 已作为企业 / 租户级隔离边界。
- 全局 `--workspace <slug|uuid>` 可临时指定 effective workspace。
- 任务、context、UDA、helper、working-set ID 都已按 workspace 隔离。
- `project` 仍是任务上的字符串字段。
- `_projects` 仍从任务字符串聚合。
- M6/M7 已在路线图中要求 token、API、MCP 以稳定 project scope 工作。

M5 的工作不是新增网络层，而是先把 project 的身份、配置和边界做成 app/service 可复用能力。

## 2. 核心产品决策

### 2.1 taskg 与 Taskwarrior 的关系

taskg 借用 Taskwarrior 的设计思路，但不再承诺完全兼容：

- 保留：
  - CLI 的 `key:value` 修改风格。
  - 查询表达式、报表、urgency、DOM/helper 的基本思想。
  - JSON import/export 中常用字段的迁移能力。
- 不再强求：
  - `project` 永远是自由字符串。
  - 用户无需预先建项目即可随意写 `project:<name>`。
  - 所有 Taskwarrior project 层级语义都原样复制。

企业场景下，project 是权限、审计、Agent scope、配置和外部系统映射的核心对象。因此 M5 起，运行时写任务必须解析到有效 project 实体。

### 2.2 严格 project 注册

M5 采用严格注册规则：

- `project add <slug> name:<name>` 创建 project。
- `taskg add "..." project:<slug>` 必须在 effective workspace 内找到 active project。
- `taskg 1 modify project:<slug>` 必须在 effective workspace 内找到 active project。
- 找不到 project 时返回非零 exit code，错误信息应提示先创建 project。
- 不允许在任务写路径中自动创建 project。
- 已归档 project 不允许被新任务引用。
- 已有任务如果已经关联归档 project，可以继续读取、导出、完成、删除；但不能把其它任务改到该归档 project。

迁移是例外：打开旧数据库时，现有 `tasks.project` 字符串可以被自动物化为 project 实体。这是数据升级兼容，不是运行时自由创建。

### 2.3 workspace 与 project 身份

- `workspace` 是企业 / 租户级隔离边界。
- `workspace.slug` 在同一个 taskg 实例内唯一，因为 CLI 使用裸 `--workspace <slug|uuid>`。
- `project.slug` 只在 `(workspace_id, slug)` 内唯一。
- 不同 workspace 可以拥有同名 project，例如：
  - `dajee/ai-agent-platform`
  - `partner/ai-agent-platform`
- `project_id` 是全局稳定身份，用于：
  - app service 内部引用。
  - M6 API。
  - M6 Agent token project allowlist。
  - M7 MCP tool scope。
  - audit payload。
- slug 只是 human input。任何 slug 解析都必须发生在明确 effective workspace 内。

## 3. M5 产品体验

M5 完成后，应支持这些使用方式：

```bash
# 创建项目
taskg --workspace dajee project add ai-agent-platform name:"AI Agent Platform"
taskg --workspace dajee project add erp-rewrite name:"ERP Rewrite"

# 列出当前 workspace 的项目
taskg --workspace dajee project list
taskg --workspace dajee project list --all

# 查看、修改、归档项目
taskg --workspace dajee project info ai-agent-platform
taskg --workspace dajee project modify ai-agent-platform description:"Agent MCP platform"
taskg --workspace dajee project archive erp-rewrite

# 引用已注册 project 创建任务
taskg --workspace dajee add "Design task.query MCP schema" project:ai-agent-platform +mcp

# 未注册 project 必须失败
taskg --workspace dajee add "Unknown work" project:ghost
# error: project "ghost" not found in workspace "dajee"; create it with: taskg project add ghost name:<name>

# 同名 project 在不同 workspace 中互不冲突
taskg --workspace partner project add ai-agent-platform name:"Partner Agent Platform"
taskg --workspace partner add "Partner integration" project:ai-agent-platform

# 脚本优先拿 project_id
taskg --workspace dajee project info ai-agent-platform --json
taskg --workspace dajee _projects
```

M5 仍需保持脚本友好：

- stdout 只输出结果。
- stderr 输出错误。
- `--json` 输出稳定 snake_case 结构。
- helper 命令无装饰、每行一个值。
- 未注册 project、跨 workspace project、归档 project 写入都要有稳定错误文本。
- 全局 flag 继续沿用 M4/Cobra 语义，可出现在根命令之后或子命令之后；README 示例统一把 `--workspace` 放在子命令之前，便于阅读。

## 4. 分层范围

### Layer 1：Project 实体与迁移

必须进入 M5：

- 新增 `projects` 表。
- 新增 `tasks.project_id`。
- project slug 在 `(workspace_id, slug)` 内唯一。
- `tasks(project_id, workspace_id)` 必须引用 `projects(id, workspace_id)`。
- M5 必须真正启用 SQLite 外键检查：打开数据库连接后执行 `PRAGMA foreign_keys = ON`，并用测试验证约束生效。
- 因 SQLite 不能通过 `ALTER TABLE` 追加复合外键，M5 迁移必须用 raw SQL 重建 `tasks` 表、复制数据、再 rename，以声明 `FOREIGN KEY (project_id, workspace_id) REFERENCES projects(id, workspace_id)`。
- DB 层约束不是唯一防线：app service 在每次任务写入前仍必须验证 `project_id` 属于当前 `workspace_id`，并在发现不变量损坏时返回稳定错误。
- 打开 M4 数据库时自动迁移：
  - 按每个 workspace 内已有非空 `tasks.project` 去重创建 project。
  - 将对应任务回填 `project_id`。
  - `tasks.project` 字符串继续保留，便于 human 输出和迁移导出。
- 新增 project 仓储和 app service。
- project 写操作进入 audit。

不进入 M5：

- project hard delete。
- 跨 workspace project move。
- project slug rename。
- 复杂 project hierarchy。

### Layer 2：严格 project 解析接入任务写路径

必须进入 M5：

- `add` 中的 `project:<slug>` 解析为 `project_id`。
- `modify project:<slug>` 解析为 `project_id`。
- `modify project:` 清空任务 project 与 `project_id`。
- import 任务时：
  - 如果 JSON 中有 `project` 且当前 workspace 没有对应 project，导入失败。
  - 如果 JSON 中 `project` 为空或缺失，则任务不绑定 project。
  - M5 不支持从 JSON 导入 `project_id` 作为权威字段；`project_id` 属于内部身份。
- edit 任务时：
  - 修改 `project` 字符串必须重新解析。
  - 不允许直接编辑 `project_id`；M5 的 edit JSON 不应暴露 `project_id`。如果输入 JSON 中出现 `project_id` 字段，保存必须拒绝并提示通过 `project:<slug>` 修改。
  - `project_id` 是保留字段，不视为 UDA 或 orphan UDA；import/edit 遇到该字段必须走显式拒绝路径。
- 已有任务关联归档 project 时：
  - 允许读取、导出、完成、删除。
  - 不允许把新任务或其它任务改到归档 project。
  - 不允许把任务从归档 project A 改到归档 project B；如果任务当前属于归档 project，只能清空 project，或改到一个 active project。

### Layer 3：查询、报表、context 与 helper

必须进入 M5：

- `project:<slug>` 查询在 effective workspace 内解析 project。
- `project:<slug>` 查询应优先编译为 `tasks.project_id = ?`。
- 对迁移前或损坏数据中 `project_id` 为空但 `project` 字符串存在的任务，不 fallback 到字符串匹配；这类数据应通过迁移报告或 invariant 错误暴露。
- active context 中的 `project:<slug>` 必须在执行时解析。
- 如果 context 引用的 project 被归档：
  - 读路径允许继续使用该 context，只返回已有关联任务。
  - 写路径仍禁止绑定到归档 project。
- `_projects` 默认列出当前 workspace 中未归档 project slug。
- `_projects --all` 必须支持，输出当前 workspace 的 active + archived project slug，仍保持每行一个值。
- `project list` 是人类命令；`_projects` 是脚本兼容 helper。

### Layer 4：Project CLI

必须进入 M5：

- `project list [--all]`
- `project add <slug> name:<name> [description:<text>]`
- `project info <slug|project-id>`
- `project modify <slug|project-id> [name:<name>] [description:<text>]`
- `project archive <slug|project-id>`

规则：

- `project info/modify/archive` 接收 slug 或 project id。
- slug 只在 effective workspace 内解析。
- project id 是全局稳定身份，但如果命令同时传了 `--workspace`，project id 必须属于该 workspace。
- `project add` 不自动切换 active project，因为 M5 不引入 active project 概念。
- `project archive` 不删除任务。
- `project archive` 对已经归档的 project 返回错误，行为与 M4 workspace archive 保持一致。
- `project modify` 不支持改 slug，避免破坏脚本和外部系统引用。

### Layer 5：Project 配置边界

必须进入 M5：

- 建立 project 级配置存储边界。
- 业务配置不再依赖本机 TOML。
- project 配置至少要支持：
  - `agent.background`
  - `agent.constraints`
  - `context.default`
- M5 固定使用 `project config` 子命令访问 project 配置，不复用无 scope 的 `config get/set/list`。
- `config get/set/list/unset` 无 scope 时继续只处理本机 / workspace 既有配置，不显示 project 配置。
- 如果用户执行 `config set agent.background ...` 这类缺少 project scope 的 project 配置写入，应返回错误并提示使用 `project config set <project> agent.background ...`。
- project 配置必须写入 DB，带 `workspace_id` 和 `project_id`。
- project 配置写操作进入 audit。

不进入 M5：

- 复杂配置继承引擎。
- report DSL 的完整 project override。
- Agent memory 的自动生成或总结。
- webhook 默认值执行逻辑。

M5 只需要把边界和最小读写能力建立起来，让 M6/M7 可以读取 project 背景和约束。

### Layer 6：权限与审计

必须进入 M5：

- `viewer`：
  - 可读 project list/info。
  - 可读 project 配置。
  - 不可创建、修改、归档 project。
- `member`：
  - 可创建/修改任务并绑定 active project。
  - 不可创建、修改、归档 project。
- `admin`：
  - 可创建、修改、归档 project。
  - 可写 project 配置。
- `owner`：
  - 拥有 admin 的 project 权限。
- 所有 project 写操作必须进入 audit：
  - `project.add`
  - `project.modify`
  - `project.archive`
  - `project.config.set`
  - `project.config.unset`
- task 写操作绑定 project 时，audit payload 应包含 `project_id` 与 `project_slug`，便于后续追踪。
- `audit_logs` 增加可空 `project_id` 列和 `(workspace_id, project_id, created_at DESC)` 索引。
- 任何 task 写 audit 都应在 payload 中带上当前任务的 `project_id` 与 `project_slug`（如果任务有 project），不只是在修改 project 字段时带。
- task 写 audit 如果任务有 project，也应填充 `audit_logs.project_id`，便于 M7 做 project timeline 时直接按列查询。
- task 写 audit 的 payload 必须记录 `before_project_id` / `before_project_slug` 和 `after_project_id` / `after_project_slug`（如适用）。如果操作会清空或移动 project，`audit_logs.project_id` 填操作前的 project id，确保项目时间线能看到任务被移出；如果操作是从无 project 绑定到 project，则填操作后的 project id。
- project 配置写 audit 必须填充 `audit_logs.project_id`。

不进入 M5：

- project 成员表。
- project 级 RBAC 覆盖 workspace role。
- project owner。

## 5. 数据模型

### projects

M5 固定模型：

```text
id TEXT PRIMARY KEY
workspace_id TEXT NOT NULL
slug TEXT NOT NULL
name TEXT NOT NULL
description TEXT NOT NULL DEFAULT ''
status TEXT NOT NULL DEFAULT 'active'
settings_json TEXT NOT NULL DEFAULT '{}'
created_at INTEGER NOT NULL
modified_at INTEGER NOT NULL
archived_at INTEGER
UNIQUE(workspace_id, slug)
UNIQUE(id, workspace_id)
```

规则：

- `id` 使用 UUID v4。
- `slug` 必须匹配 `^[a-z0-9][a-z0-9_-]*$`。
- `slug` 长度必须为 1–64。
- `slug` 保留字至少包括：
  - `all`
  - `none`
  - `current`
  - `default`
  - `archived`
- `name` 必填，可包含空格和中文。
- `description` 沿用 M4 workspace 的惯例，空字符串表示无描述；DB 层固定为 `TEXT NOT NULL DEFAULT ''`，不使用 nullable string。
- `status` M5 只支持 `active` / `archived`。
- `archived_at != NULL` 时视为 archived。
- `settings_json` 保留给未来小型结构化状态；project 配置的权威读写进入 `configs` 表，避免把所有配置塞入单个 JSON。
- M5 不引入 project visibility。workspace 内任何 member 都可读所有 active project；更细的 project 可见性由 M6 token project allowlist 和后续 project RBAC 解决。

### tasks

新增或确认：

```text
project_id TEXT
FOREIGN KEY (project_id, workspace_id) REFERENCES projects(id, workspace_id)
```

规则：

- `tasks.project_id` 与 `tasks.workspace_id` 必须一致。
- `tasks.project` 保留为 denormalized human 字段。
- 写任务时，如果 `project_id` 非空，`tasks.project` 必须同步为 project slug。
- 清空 project 时一律写成 `project_id = NULL` 且 `project = NULL`。
- M5 迁移必须把旧数据里的空字符串 project 归一成 `NULL`。
- JSON export 默认输出 `project` 字符串，不输出 `project_id`，避免把内部身份混进迁移格式。
- `--json` 的 project 管理命令必须输出 `id`。
- `project_id != NULL` 时，`tasks.project` 必须等于对应 `projects.slug`。如果发现不一致，视为数据损坏并返回 `project_invariant_violation`，不要在 export 中静默兜底修正。

### configs

M5 采用通用 `configs` 表承载 project 配置，不使用 `project_configs` 过渡表：

```text
workspace_id TEXT NOT NULL DEFAULT ''  -- server/user scope 可为空串；project/workspace scope 写真实 workspace id
scope TEXT NOT NULL                    -- server/workspace/project/user
scope_id TEXT NOT NULL DEFAULT ''      -- server scope 用空串；project scope 使用 project id
key TEXT NOT NULL
value TEXT NOT NULL
PRIMARY KEY(workspace_id, scope, scope_id, key)
```

规则：

- `workspace_id` 和 `scope_id` 都不使用 `NULL`，避免 SQLite 中复合 primary key 与 `NULL` 的唯一性陷阱。
- `workspace_id` 必须进入 primary key，避免不同 workspace 的 workspace-scope 配置或未来 user/project scope 配置在同一 `scope/scope_id/key` 组合下互相覆盖。
- `server` scope 使用 `workspace_id = ''` 且 `scope_id = ''`。
- `workspace` scope 使用 `workspace_id = <workspace_id>` 且 `scope_id = <workspace_id>`。
- `project` scope 使用 `workspace_id = <workspace_id>` 且 `scope_id = <project_id>`。
- 现有 SQLite `meta` 表继续作为本地运行时状态 / legacy config 存储，不在 M5 被删除或替代。
- M5 新增的 project 业务配置只写 `configs`，不写 `meta`。

## 6. 迁移策略

M5 必须向前兼容 M4 数据库。

迁移步骤：

1. 创建 `projects` 表。
2. 为每个 workspace 内现有非空 `tasks.project` 按规范化结果去重创建 project：
   - `raw = tasks.project`
   - `slug = strings.ToLower(strings.TrimSpace(raw))`
   - `name = strings.TrimSpace(raw)`，保留原大小写和中文等可读名称
   - `description = ""`
   - `status = active`
3. 重建 `tasks` 表以声明 `project_id` 列和复合外键，并在复制数据时写入归一化后的 `project` 与 `project_id`。
4. 增加必要索引：
   - `idx_tasks_ws_project_id(workspace_id, project_id)`
   - `idx_projects_ws_status(workspace_id, status)`
   - 不再保留 `tasks.project` 字符串索引；M5 查询统一走 `project_id`，`tasks.project` 仅是 denormalized human 字段。
5. 创建 `configs` 表。
6. 给 `audit_logs` 增加 `project_id` 列和 `idx_audit_project_time(workspace_id, project_id, created_at)` 索引。

迁移规则：

- 迁移规范化规则固定为：`trim` 前后空白，并转为小写 slug。
- 如果旧 `tasks.project` 为空字符串，迁移为 `project = NULL` 且 `project_id = NULL`。
- 如果旧 `tasks.project` 不符合新 slug 规则，必须清空该任务的 `project` 和 `project_id`，并写入迁移报告。
- 如果同一 workspace 内多个不同原始 project 值规范化后冲突，例如 `API`、`api`、` api `，M5 不自动选择 winner。冲突集合内所有相关任务都清空 `project` 和 `project_id`，并写入迁移报告，让用户显式 `project add api name:<name>` 后再手动修复。
- 迁移报告固定写入 SQLite meta key `migration.m5.projects.skipped`，值为 JSON array，至少包含 `workspace_id`、`task_uuid`、`raw_project`、`reason`。
- 如果迁移报告非空，CLI 在该次命令 stderr 输出一行 warning，提示通过 `taskg config get migration.m5.projects.skipped --json` 查看详情；M5 不新增专门的 migration report 命令。
- 对被跳过的非法 / 冲突任务，新查询语义下视为无 project 任务；`project:<old-value>` 不再匹配它们，`project:` 可以匹配。
- 迁移必须幂等。
- 迁移失败不得部分写坏数据库。
- 重建 `tasks` 表和回填 project 期间必须使用 `BEGIN IMMEDIATE` 或等价数据库写锁，保证同一 SQLite 文件同一时刻只有一个进程执行 M5 迁移；其它进程应等待或收到清晰的 database locked 错误，不允许交错执行 DDL。

## 7. CLI 详细语义

### project list

```bash
taskg project list
taskg project list --all
taskg project list --json
```

human 输出固定列：

```text
SLUG                 NAME                 STATUS     TASKS
ai-agent-platform    AI Agent Platform    active     12
```

`TASKS` 表示当前 workspace 中 `status != deleted` 且 `project_id` 指向该 project 的任务总数，不受 active context 影响。

JSON 输出字段：

```json
{
  "id": "uuid",
  "workspace_id": "uuid",
  "slug": "ai-agent-platform",
  "name": "AI Agent Platform",
  "description": "",
  "status": "active",
  "archived_at": null,
  "task_count": 12,
  "created_at": 1770000000,
  "modified_at": 1770000000
}
```

### project add

```bash
taskg project add ai-agent-platform name:"AI Agent Platform" description:"Agent MCP platform"
```

规则：

- `name:` 必填。
- slug 不能重复。
- 当前 actor 必须是 admin 或 owner。
- 新建后不自动切换任何 active project。

### project info

```bash
taskg project info ai-agent-platform
taskg project info <project-id>
taskg project info ai-agent-platform --json
```

规则：

- slug 在 effective workspace 内解析。
- project id 仍要校验 actor 对该 project 所属 workspace 有权限。
- 如果同时传 `--workspace` 和 project id，project 必须属于该 workspace。

### project modify

```bash
taskg project modify ai-agent-platform name:"Agent Platform" description:"..."
```

规则：

- 不支持修改 slug。
- 至少提供一个字段。
- 当前 actor 必须是 admin 或 owner。
- archived project 不允许修改 metadata；如需恢复，M5 不提供 restore，留后续版本。

### project archive

```bash
taskg project archive ai-agent-platform
```

规则：

- 不删除任务。
- 归档后不出现在默认 `project list` 和 `_projects`。
- 归档后不允许被新任务引用。
- 当前 actor 必须是 admin 或 owner。
- 已归档 project 再次 archive 返回错误，行为与 M4 workspace archive 保持一致。
- 如果 project 下仍有 pending 任务，M5 允许归档，但输出 warning 到 stderr；脚本可用 `--json` 查看 task_count。

### project config

M5 固定使用以下 CLI 形态：

```bash
taskg project config get ai-agent-platform agent.background
taskg project config set ai-agent-platform agent.background "This project owns taskg MCP integration."
taskg project config unset ai-agent-platform agent.background
taskg project config list ai-agent-platform
```

原因：

- scope 明确。
- 不污染已有 workspace `config set` 语义。
- M6/M7 可以直接复用 project config service。

无 scope 的 `config get/set/list/unset` 不包含 project 配置。查看 project 配置必须使用 `project config list <slug|project-id>`。

## 8. 与任务写路径的交互

### add

```bash
taskg add "Implement API" project:ai-agent-platform
```

流程：

1. 解析 effective workspace。
2. 解析 actor role。
3. 解析 `project:ai-agent-platform`。
4. 若 project 不存在，返回错误。
5. 若 project 已归档，返回错误。
6. 创建任务，写入 `project_id` 和 `project`。

### modify

```bash
taskg 1 modify project:ai-agent-platform
taskg 1 modify project:
```

规则：

- 设置 project 时同 add。
- 清空 project 时无需 project 权限，只需 task 写权限。
- 如果任务当前属于归档 project，仍可清空 project。

### import

导入 JSON 时：

- `project` 字段存在且非空：必须解析到 active project。
- 解析失败：整个 import 返回错误；是否支持 `--dry-run` 留给后续。
- 不自动创建 project。
- 如果用户要迁移大量 Taskwarrior 数据，应先运行一个 project materialization 或 import planning 工具；M5 不做完整迁移向导，但错误信息必须可操作。
- 导入失败必须整批回滚，不允许部分写入；这与 M3 import 原子性保持一致。

### export

导出 JSON 时：

- 默认输出 `project` 字符串。
- 不输出 `project_id`。
- 如果任务 `project_id` 非空但 `project` 字符串为空或与 project 实体 slug 不一致，导出应失败并返回 `project_invariant_violation`，而不是静默兜底。

### recurrence 与 project

recurrence tick 创建子任务时使用 parent 的 `project_id` 和 `project`。

- 如果 parent 的 project 仍 active，正常创建 child。
- 如果 parent 的 project 已归档，tick 仍创建 child，并保留 parent 的 project 关联；同时写入 `task.recurrence.archived_project` audit warning，payload 包含 `parent_uuid`、`child_uuid`、`project_id`、`project_slug`。
- 后续手工修改该 child 时仍受严格写路径约束：可以清空 project，或改到 active project；不能改到另一个归档 project。

## 9. 查询与上下文

### project filter

```bash
taskg project:ai-agent-platform list
taskg context define agent 'project:ai-agent-platform status:pending'
```

规则：

- `project:<slug>` 在执行时解析。
- 查询不存在 project 必须报 `project_not_found`，不允许静默返回空结果。
- `project:` 空值表示无 project 任务。
- `project.any:` 或类似扩展不进入 M5。

### `_projects`

```bash
taskg _projects
```

规则：

- 默认输出当前 workspace 未归档 project slug。
- 输出按 slug 升序。
- 每行一个 slug，无列头，无空行。
- 不从任务表聚合。
- `_projects --all` 输出 active + archived project slug。

### `_unique project`

`_unique project` 保持 helper 的“从当前查询结果聚合任务实际值”语义：

- 只输出当前查询结果中任务实际绑定的 project slug。
- 只认可 `project_id` 指向有效 project 且 `tasks.project` 与实体 slug 一致的任务。
- 不回落到损坏的 `tasks.project` 字符串。
- 不输出没有任何任务引用的 project；这类 project 由 `_projects` / `project list` 展示。

## 10. 配置边界

M5 后配置分成四类：

1. 本机配置：`taskg.toml`、env、CLI flag、`rc.*`。
2. workspace 业务配置：UDA schema、workspace context、report 默认值、workspace Agent 背景。
3. project 业务配置：project Agent 背景、约束、默认 context、后续 webhook 默认值。
4. user 偏好：后续可加入，不进入 M5。

M5 必须做到：

- README 中关于 TOML 的描述必须与本节配置分类保持一致；如果发现不一致条目要在 M5 一起修复。
- project 配置写 DB。
- project 配置受权限和 audit 约束。
- M6/M7 可以通过 app service 读取 project 配置，而不依赖 CLI。

## 11. 错误语义

M5 应明确这些错误码与文本：

| Code | Message |
|---|---|
| `project_slug_required` | `project slug is required` |
| `project_invalid_slug` | `invalid project slug` |
| `project_not_found` | `project "x" not found in workspace "w"` |
| `project_archived` | `project "x" is archived; cannot modify or assign new tasks` |
| `project_workspace_mismatch` | `project id "..." does not belong to workspace "w"` |
| `project_already_exists` | `project "x" already exists in workspace "w"` |
| `project_name_required` | `project name is required` |
| `project_slug_immutable` | `project slug cannot be modified` |
| `project_invariant_violation` | `project invariant violation` |
| `permission_denied` | `permission denied: admin or owner required` |
| `project_config_scope_required` | `project config requires project scope; use project config set <project> <key> <value>` |

JSON error 格式继续沿用当前 CLI 的 `--json` 错误输出策略。

## 12. 非目标

M5 不做：

- HTTP API。
- 远程 CLI。
- PAT / Agent token。
- MCP。
- op-log 同步。
- 外部系统适配。
- project 成员表。
- project owner / project role override。
- project hard delete。
- project restore。
- project slug rename。
- 自动创建 project。
- 完整 Taskwarrior 兼容。
- 甘特图、预算、审批流等复杂项目管理功能。

## 13. 测试要求

M5 至少需要覆盖：

- 数据迁移：
  - M4 数据库升级创建 projects。
  - 旧 `tasks.project` 回填 `project_id`。
  - 非法旧 project 值不破坏迁移。
  - 规范化冲突的旧 project 值进入 `migration.m5.projects.skipped`。
  - 迁移幂等，重复 open 不重复创建 projects。
- project repository：
  - `(workspace_id, slug)` 唯一。
  - 不同 workspace 可复用 slug。
  - `tasks.project_id` 不能跨 workspace。
  - SQLite `PRAGMA foreign_keys = ON` 后复合 FK 生效。
- app service：
  - admin/owner 可 add/modify/archive project。
  - member/viewer 不可写 project。
  - project id + workspace 不一致时报错。
  - archived project 不可被新任务引用。
  - 人工构造 `tasks.project_id` 与 `tasks.workspace_id` 不一致时，读写路径返回 `project_invariant_violation` 或等价错误。
  - actor 在 workspace A 下尝试引用只存在于 workspace B 的 slug、project id 都必须失败。
- CLI：
  - `project list/add/info/modify/archive` human 与 JSON 输出。
  - `taskg add ... project:missing` 失败并提示创建 project。
  - `taskg add ... project:existing` 写入成功。
  - `taskg 1 modify project:` 清空 project。
  - `project archive` 对已归档 project 返回错误。
- 查询：
  - `project:<slug>` 命中 project_id。
  - `project:missing` 报错。
  - `project:` 查询无 project 任务。
  - context 引用 project 后读路径生效。
  - `project:<slug-only-in-other-workspace>` 在当前 workspace 报错。
- helper：
  - `_projects` 来自 project 表，不来自任务聚合。
  - `_projects` 每行一个 slug，无列头。
- import/export：
  - export 输出 project slug。
  - import 未注册 project 失败。
  - import 已注册 project 成功。
  - 未注册 project 导致 import 失败时整批回滚。
  - 归档 project 关联任务 export 后在同 workspace import 成功；跨 workspace import 因 project 未注册失败。
- audit：
  - project 写操作写 audit。
  - task 绑定 project 时 audit payload 包含 project 信息。
  - task 写 audit 填充 `audit_logs.project_id`。
  - project 配置写 audit 填充 `audit_logs.project_id`。
  - 归档 project 上的 recurrence tick 写入 `task.recurrence.archived_project` audit warning。

implementation plan 必须把以下命令列入最终验收；如果 `go test -race ./...` 因环境耗时无法常规执行，必须在计划和交付说明中记录原因：

```bash
go vet ./...
go test -race ./...
```

验证命令：

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/taskg
```

## 14. 文档更新要求

M5 完成后必须更新：

- [README.md](/Users/mac/code/projects/dajee/task/README.md)
  - 增加 M5 project 命令用法。
  - 明确 taskg 不是完整 Taskwarrior clone。
  - 明确 project 必须先注册。
- [ROADMAP.md](/Users/mac/code/projects/dajee/task/ROADMAP.md)
  - M5 状态改为已完成。
  - M6/M7 继续基于 `project_id` scope。
- [docs/requirements.md](/Users/mac/code/projects/dajee/task/docs/requirements.md)
  - 更新 project 严格注册和配置边界。
  - 降低“完全兼容 Taskwarrior”的表述。

## 15. 与 M6/M7 的衔接

M5 必须暴露与 M4 workspace 命名风格一致的 app service 方法，供 M6 直接复用：

- `AddProject(input)`。
- `ListProjects(includeArchived)`。
- `ProjectInfo(ref)`。
- `ModifyProject(ref, input)`。
- `ArchiveProject(ref)`。
- `ResolveProject(ref)`，隐式使用当前 effective workspace；需要跨 workspace 校验时提供显式 helper。
- `ResolveProjectInWorkspace(workspaceID, ref)`，显式在指定 workspace 内解析 slug 或 project id，供 M6 API / token scope 复用。
- `ProjectConfigGet/Set/Unset/List` 或等价接口。
- `project_id` scope 校验。

M7 MCP tool 应基于 M5 能力提供：

- `project.list`
- `project.get`
- `project.current` 或 `project.resolve`
- task tools 中的 `project_id` 优先参数。

M6/M7 不应再围绕裸 project slug 做权限判断。

## 16. 进入 implementation plan 前的检查

进入 M5 implementation plan 前，应再次检查：

- 本文 A 类决策没有被 plan 重新打开。
- `configs` 表、`audit_logs.project_id`、`tasks` 复合 FK 的迁移路径已具体化。
- 迁移失败、非法旧 project、规范化冲突都有可测试的处理路径。
- plan 中每个 chunk 都能独立通过 `go test ./...` 和 `CGO_ENABLED=0 go test ./...`。
