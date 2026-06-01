# taskg Roadmap

本文档是 `taskg` 的产品路线图。目标是逐步实现 README 中定义的最终产品形态：借鉴 Taskwarrior 设计思路、面向企业项目协作和 Agent MCP 的任务运行时。

- 单一二进制，同时承担本地 CLI、远程 CLI 客户端、HTTP/JSON API 服务端、MCP Server。
- 使用纯 Go SQLite 方案，保持零 CGO、可跨平台交叉编译。
- `workspace` 作为企业 / 租户级隔离边界，`project` 表示 workspace 内的真实企业项目。
- 支持多用户、权限、审计、Agent token、行级隔离。
- 借鉴 Taskwarrior 的核心命令名、JSON 迁移格式与 urgency 公式；企业 workspace/project/Agent 边界优先于完整兼容。

路线图按可独立交付、可测试、可回滚的 milestone 拆分。每个 milestone 开始前都应先写中文 spec，再用 `superpowers:writing-plans` 拆成实施计划。

## 状态总览

| Milestone | 状态 | 主题 |
|---|---|---|
| M0 | 已完成 | 本地单用户 CLI、SQLite 存储、核心任务生命周期 |
| M1 | 已完成 | 查询语言、内置报表、urgency、DOM 与 calc 基础 |
| M2 | 已完成 | Taskwarrior 核心任务模型补齐 |
| M3 | 已完成 | 配置系统、上下文、UDA、`.taskrc` 只读导入与脚本化 helper |
| M4 | 已完成 | 企业 Workspace、权限与审计基础 |
| M5 | 已完成 | Project 实体化与 Workspace/Project 配置边界 |
| M6 | 已完成 | HTTP/JSON API、远程 CLI 与 Agent Token |
| M7 | 待规划 | 企业 Agent MCP Server 与工具接口 |
| M8 | 待规划 | Agent 驱动的外部集成、触发器、发布与运维打磨 |

## M0：本地单用户 CLI

**状态：已完成。**

M0 已经把项目从设计文档推进到可运行的本地 CLI。当前能力包括：

- Go module 与 `cmd/taskg` 单二进制入口。
- Cobra CLI 基础结构。
- GORM 持久化层。
- 纯 Go SQLite driver：`github.com/glebarez/sqlite`。
- 隐式 local workspace。
- 本地数据库自动初始化。
- 任务核心生命周期：`add`、`list`、`next`、`info`、`modify`、`done`、`delete`。
- 基础 filter：status、project、priority、tag、自由文本、数字 working-set ID、UUID。
- 基础日期解析：`today`、`tomorrow`、`YYYY-MM-DD`、RFC3339、`eod`、`eow`、`eom`、`<N>days`。
- JSON import/export。
- `show`、`config get`、`config set`。
- human 输出与 `--json` 输出基础。
- 单元测试与 CLI 集成测试。

**M0 后续清理项：**

- README 已统一为“GORM + `github.com/glebarez/sqlite`，零 CGO”的数据库描述。
- 检查所有命令是否严格保持 stdout/stderr 分离。
- 继续用 `CGO_ENABLED=0 go test ./...` 作为每个 milestone 的必跑验收。

## M1：查询语言、报表、Urgency、DOM 与 Calc 基础

**目标：** 把 M0 的“基础列表过滤”升级为 Taskwarrior 风格的查询和排序核心，让 CLI 真正成为任务数据库的查询入口。

**范围：**

- 新增 `internal/query` AST，而不是继续堆叠简单 `Filter` 字段。
- 支持 Taskwarrior 风格常用 filter：
  - `project:ai-agent-platform`
  - `+urgent`
  - `-tag`
  - `status:pending`
  - `priority:H`
  - `due:today`
  - `due.before:tomorrow`
  - `due.after:2days`
  - `/pattern/`
  - 字符串引号：`project:'ERP Rewrite'`
- 支持布尔组合：
  - 默认 AND。
  - `and`、`or`、`xor`、`not`。
  - 括号分组。
- 将 AST 编译为 GORM 查询或安全 SQL 条件，所有值必须使用绑定参数。
- 内置报表：
  - `list`
  - `next`
  - `all`
  - `completed`
  - `waiting`
  - `active`
  - `ready`
  - `overdue`
  - `blocked`
  - `blocking`
- `internal/urgency`：
  - 复刻 Taskwarrior 默认 urgency 系数。
  - 支持 `+next`、due/overdue、priority、age、tag、project 等 M1 可计算项。
  - 提供 explain 结构，为后续 MCP `urgency.explain` 复用。
- `internal/dom` 第一版：
  - `_get 1.description`
  - `_get 1.uuid`
  - `_get 1.entry`
  - `_get 1.modified`
  - `_ids`
  - `_uuids`
  - `_projects`
  - `_tags`
- `internal/expr` / `calc` 第一版：
  - 数字、布尔、比较、基础算术。
  - 先满足 `taskg calc` 和后续 query/urgency 复用，不追求一次性覆盖完整 Taskwarrior calc。

**不进入 M1：**

- UDA。
- annotations。
- dependencies。
- recurring。
- 多用户和 HTTP。
- MCP Server。
- 完整 `.taskrc` 兼容。

**验收标准：**

- 常用 Taskwarrior filter 示例有单元测试和 CLI 集成测试。
- `taskg +next or due.before:tomorrow list` 这类布尔查询可用。
- `next` 默认按 urgency 排序。
- `taskg urgency 1` 或 `_urgency` 能输出任务 urgency；如果命令命名暂未确定，至少 app/service 层提供 explain。
- `_get`、`_ids`、`_uuids`、`_projects`、`_tags` 可脚本化使用。
- `go test ./...`、`CGO_ENABLED=0 go test ./...` 通过。

## M2：Taskwarrior 核心任务模型补齐

**状态：已完成。**

**目标：** 补齐 Taskwarrior 日常使用所需的任务字段和命令，让 taskg 不再只是简单 todo CLI。

**范围：**

- 扩展任务模型：
  - `start`
  - `wait`
  - `scheduled`
  - `until`
  - `annotations`
  - `depends`
  - `recur`
  - `parent`
  - `mask`
  - `imask`
- 新增命令：
  - `start`
  - `stop`
  - `annotate`
  - `denotate`
  - `append`
  - `prepend`
  - `edit`
- dependencies：
  - `depends:<uuid>` 修改语法。
  - blocked/blocking 报表与 urgency 联动。
- recurring：
  - 支持基础周期：daily、weekly、monthly、`<N>days`。
  - 父任务隐藏，子任务可见。
- waiting/ready/active：
  - `wait` 到期后任务可自动恢复 pending。
  - `scheduled` 到期后进入 ready 报表。
  - `start` 后进入 active 报表并影响 urgency。
- JSON import/export 覆盖 M2 字段。

**不进入 M2：**

- UDA。
- 多用户。
- 远程 API。
- op-log 同步。

**验收标准：**

- `start/stop`、`annotate/denotate`、`append/prepend`、`edit` 有集成测试。
- blocked/blocking 报表与 dependencies 一致。
- recurring 子任务生成规则有确定测试。
- M2 字段导入导出往返不丢失。
- urgency explain 能体现 active、blocked、blocking、annotations 等新增因素。

**M2 已交付内容：**

- 扩展任务模型：
  - `start`
  - `wait`
  - `scheduled`
  - `until`
  - `annotations`
  - `depends`
  - `recur`
  - `parent`
  - `mask`
  - `imask`
- 新增报表：
  - `waiting`
  - `active`
  - `ready`
  - `blocked`
  - `blocking`
- 新增命令：
  - `start`
  - `stop`
  - `annotate`
  - `denotate`
  - `append`
  - `prepend`
  - `edit`
- 查询 / DOM / urgency / JSON import-export 已贯通 M2 字段。
- 基础 recurring 已支持：
  - `daily`
  - `weekly`
  - `monthly`
  - `<N>days`
  - `<N>weeks`
  - `<N>months`
- recurring parent 默认隐藏，child 可见；完成 child 后自动生成下一个 child；`until` 会阻止继续生成。

## M3：配置系统、上下文、UDA 与兼容性增强

**状态：已完成。**

**目标：** 完善 Taskwarrior 风格的个性化能力和脚本化能力，为长期使用和迁移做准备。

**范围：**

- 配置系统升级：
  - `~/.config/taskg/taskg.toml`。
  - SQLite meta/config 与文件配置合并规则。
  - 命令行临时覆盖：`rc.x=y`。
  - 保留 `--db`、`TASKG_DB` 的优先级。
- `.taskrc` 只读导入第一版：
  - 能识别常见 key。
  - 不认识的 key 给出兼容性报告。
- context：
  - `context define <name> <filter>`。
  - `context use <name>`。
  - `context none`。
  - 所有报表自动叠加 active context。
- UDA：
  - schema 定义。
  - string/numeric/date/duration 类型。
  - 枚举值校验。
  - orphan UDA 保留但不可普通修改。
  - UDA 参与 JSON import/export。
- helper 命令补齐：
  - `_udas`
  - `_unique`
  - `_urgency`
  - `_show`
  - `_version`
- shell completion 第一版：
  - zsh。
  - bash。
  - fish。
  - powershell。

**不进入 M3：**

- 真正多用户。
- HTTP API。
- MCP。

**验收标准：**

- context 生效后，`list/next/all` 等报表结果自动受影响。
- UDA 能配置、写入、查询、导入导出。
- orphan UDA 能保留并在兼容性报告中出现。
- `_unique project`、`_tags`、`_udas` 等 helper 输出无装饰、可脚本解析。
- `.taskrc` 导入不会破坏现有配置。

**M3 已交付内容：**

- 配置系统升级：
  - 支持 `~/.config/taskg/taskg.toml` 与 `XDG_CONFIG_HOME`。
  - 支持 `rc.<key>=<value>`、`rc.<key>:`、`rc.context=none`。
  - `config get/set/unset/list` 可读取合并视图，并路由 UDA schema。
- context：
  - `context define/use/none/show/list/delete`。
  - active context 自动影响读路径；`--no-context` 可绕过。
- UDA：
  - `string`、`numeric`、`date`、`duration` 类型。
  - 枚举值校验、JSON top-level import/export、orphan UDA 保留。
  - UDA query、DOM `_get`、`_udas`、`_unique`、urgency 系数基础。
- `.taskrc` 只读导入：
  - 支持 `data.location`、`color`、`dateformat`、`context.<name>`、`uda.*`、`urgency.uda.*`。
  - `report.*`、`hooks.*` 等识别为 skipped；未知 key 进入 unknown 报告。
  - 支持 `--dry-run` 和 `--json` 报告。
- helper 与 completion：
  - `_show`
  - `_version`
  - `_udas`
  - `_unique`
  - `completion bash|zsh|fish|powershell`

## M4：企业 Workspace、权限与审计基础

**状态：已完成。**

**目标：** 在仍然不引入 HTTP 服务端的前提下，把运行时从“单用户单 workspace”升级成“actor + workspace + role”。M4 中的 workspace 是企业 / 租户级隔离边界；project 仍是任务字段，用来表达该 workspace 内的真实企业项目。

**M4 已交付内容：**

- 身份与审计模型：
  - `users`
  - `workspaces`
  - `memberships`
  - `audit_logs`
- 本地默认身份：
  - 自动创建 `local` user
  - 自动创建 `local` workspace
  - 自动创建 `local -> local` owner membership
  - 首次升级时自动迁移旧 `context.active` meta
- runtime context：
  - 每次命令解析 `ActorUserID + WorkspaceID + Role`
  - 支持 `active_user_id`
  - 支持 `active_workspace.<user_id>`
  - 支持 `active_context.<user_id>.<workspace_id>`
  - 支持全局 `--workspace <slug|uuid>` 一次性覆盖
  - 当前 CLI 使用裸 workspace slug，因此 workspace slug 在同一个 taskg 实例内保持唯一；project slug 只在 workspace 内唯一
- 权限边界：
  - `viewer` / `member` / `admin` / `owner`
  - task、context、UDA schema、workspace metadata、member role、audit read 都经过 app 层权限检查
- 审计事务：
  - spec 范围内的写操作在同一个 store-level transaction 中同时写业务表和 `audit_logs`
  - `task` / `context` / `workspace` / `member` / `UDA schema` 写路径全部接入 audit
- CLI 能力：
  - `user list/add/use/info`
  - `workspace list/add/use/info/modify/archive`
  - `member list/add/role`
  - `audit list`
- workspace 隔离：
  - task、project、tag、UDA、context、helper、working-set ID 都按 workspace 隔离
  - storage 层 SQL 显式绑定 workspace，不再依赖“隐式全局 local workspace”

**本阶段刻意不做：**

- HTTP API。
- 远程 CLI。
- PAT/JWT 登录鉴权。
- MCP。
- 多端同步。
- `member delete`、`user delete`、workspace hard delete。

**对 M5 的意义：**

- app service 已不再把 local workspace 当作默认业务前提。
- 权限检查、审计编排、runtime context 解析都已沉到可复用的 `internal/app` 边界。
- M5 应先把 project 从“字符串字段”抬成 workspace 内的一等对象，再让后续 API、Agent token 和 MCP scope 绑定稳定 project 身份。
- M5 还需要把配置边界收紧：TOML 只保留本机/启动配置，workspace/project 业务配置必须通过 DB、权限和 audit 管理。

## M5：Project 实体化与 Workspace/Project 配置边界

**状态：已完成。**

**目标：** 把 `project` 从任务自由字符串抬成企业项目对象。这样后续 HTTP API、Agent token、MCP tool 和外部集成都能围绕稳定的 project 身份授权，而不是依赖字符串约定。

**范围：**

- project 实体：
  - 新增 `projects` 表，归属于 workspace。
  - 字段至少包括 `id`、`workspace_id`、`slug`、`name`、`description`、`status`、`created_at`、`archived_at`。
  - `slug` 只在同一 workspace 内唯一；不同 workspace 可以有相同 slug 的 project，任何解析都必须带 workspace。
  - 迁移时按现有 `task.project` 值生成 project 草案；这是数据升级兼容，不代表运行时可自由创建 project。
- task 与 project 关系：
  - task 保留 `project` 字符串用于 human 输出和迁移导出。
  - 内部增加稳定 `project_id` 或等价映射，供权限、审计、API、MCP 使用。
  - `project:<slug>` 查询继续可用，并限定在当前 workspace 内解析。
- CLI：
  - M5 采用严格 project 注册：新增或修改任务引用不存在的 `project:<slug>` 必须报错，不自动创建 project。
  - 已归档 project 不允许被新任务引用；已有任务保留关联并可继续读取、完成、删除。
  - M5 不引入全局唯一 project slug。所有 project slug 都必须在 effective workspace 内解析。
  - effective workspace 的来源顺序：本次 `--workspace <slug|uuid>` > 当前 active workspace > 本地默认 workspace。后续远程 CLI 还要叠加 token workspace scope。
  - `taskg --workspace dajee project info ai-agent-platform` 表示 `dajee` workspace 下的 `ai-agent-platform`。
  - `taskg --workspace partner project info ai-agent-platform` 表示另一个 workspace 下的同名 project。
  - `taskg project info ai-agent-platform` 只在当前 active workspace 中查找，不做跨 workspace 搜索。
  - 脚本和 API 场景应优先保存和传递 `project_id`；slug 只做人类输入。
  - `project_id` 是全局稳定身份，但所有读取和写入仍必须校验 actor 对该 project 所属 workspace 的权限。
  - 如果命令同时给出 `--workspace <slug|uuid>` 和 `<project-id>`，该 project 必须属于这个 workspace；不属于时直接报错，不回退到 project 自己的 workspace。
  - `project list`
  - `project add <slug> name:<name>`
  - `project info <slug|project-id>`
  - `project modify <slug|project-id> ...`
  - `project archive <slug|project-id>`
  - `_projects` 继续输出脚本兼容列表。
  - `_projects` 默认只列 effective workspace 下的项目；跨 workspace 枚举必须显式指定 workspace，或等到 M6 API/M7 MCP 通过带权限的接口提供。
- 配置边界：
  - `taskg.toml` 只作为本机启动和显示配置来源，例如 `database.path`、`color`、`json`、`date.format`、远程 CLI 连接信息。
  - workspace 业务配置必须存 DB，并绑定 `workspace_id`，包括 UDA schema、urgency UDA 系数、context、report 默认配置。
  - project 级配置挂到 project/workspace 下，包括 project 默认 context、project 级 Agent 背景、project 级约束和后续 webhook 默认值。
  - project 配置只通过 `project config get/set/unset/list <project>` 访问；无 scope 的 `config get/set/list` 不显示 project 配置。
  - 本机显示/启动配置、workspace 业务配置、project 业务配置必须分开存储和审计。权限和业务规则不得从调用者本机 TOML 读取。
  - `.taskrc` 和 TOML 中的 UDA/context/urgency 业务 key 只作为迁移输入，不作为跨 workspace 的运行时全局配置。
- 权限与审计：
  - project 写操作进入 audit。
  - M5 可以先不做 project 成员表，但要为 M6 token 的 project allowlist 预留稳定 project id。
  - 如果实现 project owner/member，需要明确它与 workspace role 的优先级。

**不进入 M5：**

- HTTP API。
- 远程 CLI。
- PAT / Agent token。
- MCP。
- op-log 同步。
- 外部系统适配。
- 复杂项目管理功能，例如甘特图、预算、审批流。

**验收标准：**

- 已有 `task.project` 数据能平滑进入 project 实体化路径，迁移导出不丢 project 字符串。
- 同一 workspace 内 project slug 唯一；不同 workspace 内可以复用同名 project。
- 新增或修改任务时，`project:<slug>` 必须解析到当前 workspace 内已存在且未归档的 project；不存在时返回清晰错误。
- `tasks.project_id` 与 `tasks.workspace_id` 必须一致：不能把 `dajee` workspace 的任务绑定到 `partner` workspace 的 project。数据库迁移、repo 写入和 app/service 测试都要覆盖这条约束。
- `project:*` 查询、`_projects`、报表、context 与 UDA/urgency 都按 workspace/project 边界工作。
- workspace 业务配置与 project 配置互不污染；两个 workspace 可以拥有不同 UDA、urgency、context 和 project 默认配置。
- `taskg.toml` 不再被描述为业务配置来源。
- project 写操作有权限检查和审计记录。
- `go test ./...`、`CGO_ENABLED=0 go test ./...`、`CGO_ENABLED=0 go build ./cmd/taskg` 通过。

**当前已交付结果：**

- `projects` 表、`tasks.project_id`、`audit_logs.project_id`、project 级 `configs` 已落地。
- M4 旧任务的 `project` 字符串会在升级时自动迁移到 project 实体；无法安全归一化的旧值写入 `migration.m5.projects.skipped`，CLI 启动时会给出 warning。
- CLI 已支持：
  - `project list/add/info/modify/archive`
  - `project config get/set/unset/list`
  - `_projects --all`
  - `audit list --project <slug|project-id>`
- 任务写路径采用严格 project 注册：
  - `add project:<slug>` / `modify project:<slug>` 必须解析到当前 workspace 内已存在且未归档的 project
  - 不存在返回 `project_not_found`
  - 已归档返回 `project_archived`
- 查询、context、report、helper 都先把 `project:<slug>` rewrite 成稳定 `project_id` 再执行。
- `_projects` 读取 project 表；`_unique project` 继续表示“当前查询结果里的任务实际绑定了哪些 project”。
- project 配置与 workspace/general config 已分开；无 scope `config` 不再读写 project 配置。
- 后续 M6/M7 一律基于 `project_id` 做 token scope、API 参数和 MCP tool 身份。

## M6：HTTP/JSON API、远程 CLI 与 Agent Token

**状态：已完成。**

**目标：** 让同一个 `taskg` 二进制可以作为 HTTP 服务端运行，并让 CLI / Agent 通过远程 API 操作任务。M6 的重点是把“actor + workspace + project + token scope”固化成传输层协议。

**范围：**

- 服务端模式：
  - `taskg server --listen :8080`。
  - `--data-dir` / `--db` 指定服务端数据库。
  - graceful shutdown。
- HTTP API：
  - task CRUD/action、annotation、urgency。
  - query/report。
  - workspace/member/project/project config。
  - context/config。
  - workspace/member 基础管理。
  - import/export。
- OpenAPI 3 文档维护在 `docs/openapi/taskg-v1.yaml`。
- 鉴权：
  - PAT。
  - Agent token / service token。
  - JWT 登录可作为 M6.5，如果范围过大可拆分。
  - `Authorization: Bearer <token>`。
  - token 绑定 actor，并可限制可访问 workspace。
  - token 可限制 project allowlist；scope 应引用 M5 的稳定 project id。slug 只能作为带 workspace 的人类可读输入，不能作为全局唯一标识。
- 行级隔离：
  - 所有请求必须绑定 actor。
  - 所有查询必须绑定可见 workspace。
  - 如果 token 带 project scope，task/query/report/import/export 都必须叠加 project 限制。
- 远程 CLI：
  - `taskg --server URL --token TOKEN list`。
  - 本地/远程命令输出尽量一致。
  - 支持环境变量配置 server/token。
  - 核心 task/report/project/context/config/helper/token/import/export/audit 命令已远程化；`edit`、`.taskrc import` 等本机语义命令暂不支持远程。
  - 支持 `--workspace <slug|uuid>`，但不能突破 token 的 workspace scope。
  - 支持 `--project <slug>` 作为远程/API 场景的显式 project scope 便捷入口；slug 必须在 effective workspace 内解析，本地 Taskwarrior 风格 `project:<slug>` 查询继续可用。
  - 支持 `--project-id <uuid>` 作为无歧义 project scope。脚本、Agent token 和 MCP 推荐使用 project id。
  - 如果请求同时携带 `project` 和 `project_id`，`project` 必须在 effective workspace 内解析到同一个 id；不一致时返回参数错误。
  - 如果请求同时携带 `workspace` 和 `project_id`，该 project 必须属于这个 workspace；不一致时返回参数错误。
  - 如果 token 可见多个 workspace，且命令没有明确 effective workspace，则 `--project <slug>` 必须报错并提示补 `--workspace` 或改用 `--project-id`。
- 配置 API：
  - `config get/set/list` 在服务端和远程 CLI 下必须显式区分 local config、workspace config 与 project config。
  - HTTP/远程 CLI 不依赖操作者本机 TOML 来决定 workspace/project 业务规则。

**M6 留待 M7 补齐的远程管理命令：**

M6 已用 `remote_unsupported_command` 显式拦截下列远程 CLI 管理命令，避免服务端不可达或命令未接线时静默读写本地 SQLite：

- `taskg --server ... workspace add|list|info|modify|use|archive`
- `taskg --server ... user add|list|info|use`
- `taskg --server ... member list|add|role`
- `taskg --server ... show`

服务端对应的 `/api/v1/workspaces*`、`/api/v1/workspaces/{workspace}/members*`、`/api/v1/me` 已在 M6 实现。M7 应把 CLI 侧接到这些 endpoint，不新增 HTTP endpoint，并把现有远程 unsupported 集成测试拆成“命令远程成功”和“不会触碰本地 DB”两类验收。

**不进入 M6：**

- MCP。
- JWT / password login / refresh token。
- op-log 同步。
- Hook。
- 外部系统适配。

**验收标准：**

- 本地 CLI 与远程 CLI 在核心命令上行为一致。
- API 认证失败、权限不足、资源不存在有稳定错误结构。
- 不同 workspace/user 的数据无法越权访问。
- 带 project scope 的 token 不能读取或修改其它 project 的任务。
- 配置 API 能清楚区分本机配置、workspace 配置和 project 配置。
- OpenAPI 覆盖已实现 endpoint。
- 服务端和远程 CLI 都通过 CGO-free 测试和构建。

## M7：企业 Agent MCP Server 与工具接口

**目标：** 让企业 Agent 能通过 MCP 以结构化方式使用 taskg。MCP 请求必须落在明确的 workspace scope 内，并可进一步受 project scope 限制。Agent 不应该凭提示词决定自己能看什么，权限必须来自 token 和服务端校验。

**范围：**

- MCP transport：
  - stdio。
  - Streamable HTTP。
- MCP tools：
  - `task.add`
  - `task.modify`
  - `task.done`
  - `task.delete`
  - `task.query`
  - `task.get`
  - `task.annotate`
  - `task.depends`
  - `task.start`
  - `task.stop`
  - `report.run`
  - `urgency.explain`
  - `workspace.list`
  - `workspace.current`
  - `project.list`
  - `project.get`
  - `project.current`
  - `context.set`
  - `context.show`
  - `config.get`
  - `config.set`
- 返回格式统一：
  - `data`：结构化 JSON。
  - `rendered`：人类可读文本。
- MCP 鉴权：
  - stdio 可使用本地配置。
  - HTTP MCP 使用 PAT。
  - HTTP MCP 支持 Agent token / service token。
  - 每次 tool 调用都解析 actor、workspace scope、project scope。
- Agent 记忆与上下文：
  - Agent 可读取 workspace/project 的背景、约束和默认 context 摘要。
  - 这些信息来自服务端 DB，不来自操作者本机 TOML。
- tool schema 测试：
  - 参数校验。
  - 错误结构。
  - workspace scope。
  - project scope。

**不进入 M7：**

- 外部系统专用适配。
- op-log 同步。
- Hook / trigger 引擎。
- 复杂 Agent 编排平台。

**验收标准：**

- 本地 MCP stdio 能被 MCP 客户端调用。
- HTTP MCP 能鉴权并限制 workspace。
- 带 project scope 的 Agent 只能查询和修改授权 project 中的任务。
- MCP tool 与 CLI/API 复用同一 app service，不复制业务逻辑。
- 每个 tool 都有 schema 和集成测试。
- Agent 可以完成“查询项目待办、添加项目任务、解释 urgency、写入审计”的完整流程。

## M8：Agent 驱动的外部集成、触发器、发布与运维打磨

**目标：** 把 taskg 打磨成可交付、可迁移、可部署、可被外部系统驱动的完整产品。外部系统不是主角；它们负责产生事件或承载输出，真正的任务决策由 Agent 通过 taskg MCP/API 完成。

**范围：**

- 触发器：
  - webhook trigger。
  - schedule / heartbeat trigger。
  - 一次性 trigger。
  - 触发器必须绑定 actor、workspace 和可选 project scope。
- Agent 驱动的外部适配：
  - 飞书、GitHub、Jira、Slack 等都只是 adapter 示例。
  - adapter 负责接收事件、标准化 payload、调用 Agent 或 taskg MCP/API、回写外部系统。
  - 不把飞书作为唯一目标，也不把飞书业务逻辑写进 taskg 核心。
- Hook：
  - `on-launch`。
  - `on-add`。
  - `on-modify`。
  - `on-exit`。
  - 本地脚本 Hook 与服务端 Webhook。
  - project/workspace 级 webhook。
  - timeout、失败回滚、stdout/stderr 协议、Webhook 签名。
- 同步与 operation log：
  - 是否进入 M8 由 M8 spec 评估。如果进入，范围包括 `operations` 表、replica_id、断点续传、冲突策略和 tags 原子 add/remove。
  - 如果范围过大，应拆成 M9，不阻塞外部触发和 MCP 产品化。
- Agent 记忆：
  - workspace 级企业偏好。
  - project 级项目背景、约束和默认 context。
  - 个人偏好。
  - Agent 可读的 context/config 摘要。
- backup：
  - SQLite `VACUUM INTO`。
  - JSON 导出。
  - 恢复演练文档。
- Taskwarrior 迁移打磨：
  - 真实 `task export` 样本导入。
  - 兼容性报告。
  - 不支持字段保留策略。
- 发布：
  - linux/amd64。
  - linux/arm64。
  - darwin/amd64。
  - darwin/arm64。
  - windows/amd64。
  - 全部 CGO-free。
- 文档：
  - 安装。
  - 本地 CLI。
  - 服务端部署。
  - 远程 CLI。
  - HTTP API。
  - MCP。
  - 外部 adapter 编写指南。
  - 迁移。
  - 备份恢复。

**验收标准：**

- 新用户只靠 README 可以完成安装、添加任务、查询任务、导入 Taskwarrior 数据。
- 管理员只靠文档可以部署服务端、创建 token、配置远程 CLI。
- Agent 可以通过 MCP 完成常见任务管理流程。
- 至少一个外部 adapter 示例可在测试环境跑通；飞书可以是示例之一，但不是唯一目标。
- 触发器不会绕过 workspace/project/token 权限。
- 所有发布产物均通过 CGO-free 验证。

## 跨 Milestone 规则

- 每个 milestone 都必须有独立中文 spec。
- 每个 spec 通过后，再写 implementation plan。
- implementation plan 必须包含 TDD 步骤、测试命令和验收命令。
- 每个 milestone 完成后更新 README、ROADMAP 和必要的 docs。
- 数据迁移必须向前兼容，除非明确进入破坏性版本。
- CLI、HTTP API、MCP 必须复用同一 app service。
- JSON 字段语义在 CLI、HTTP API、MCP 中保持一致。
- 所有涉及权限、同步、Hook、导入导出的改动必须有端到端测试。
- 每个 milestone 必跑：

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/taskg
```

## 当前下一步

M5 需求规格与实现计划都已落地到：

```text
docs/superpowers/specs/2026-05-30-taskg-m5-design.md
docs/superpowers/plans/2026-05-30-taskg-m5-implementation.md
```

接下来的重点不再是 M5 设计，而是基于已完成的 project 实体能力推进：

- M6：HTTP/JSON API、远程 CLI、Agent token 全部统一采用 `project_id`。
- M7：MCP tool 优先接受 `project_id`，slug 只作为当前 workspace 内的人类输入。
- 更细的 project 级默认上下文、Agent 背景、约束模板扩展继续沿用 `project config`。
