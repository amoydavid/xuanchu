# Xuanchu Roadmap

本文档是 Xuanchu 的产品路线图。目标是逐步实现 README 中定义的最终产品形态：借鉴 Taskwarrior 设计思路、面向企业项目协作和 Agent MCP 的任务运行时。

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
| M7 | 已完成 | 企业 Agent MCP Server 与工具接口 |
| M8 | 已完成 | 服务端 Hook / 自动化扩展与运维交付打磨 |
| M9 | 已完成 | 任务多 Assignee |
| M10 | 已完成 | Token 委托与 Impersonation |
| M11 | 已完成 | 用户外部 ID 绑定 |
| M12 | 已完成 | 任务外部关联 |
| M13 | 已完成 | 项目 Annotation 与 Timeline |
| M14 | 已完成 | 多数据库支持（SQLite / PostgreSQL） |
| M14.1 | 已完成 | Token Scope 通配符与 Token Modify |
| M15 | 已完成 | MCP Tool 全量覆盖（74 tool）与 Agent Skill 文档 |
| M16 | 已完成 | 定时通知、动态 endpoint 与 HTTP request template sink |
| v0.1.1 | 已完成 | 稳定短任务标识 `task_slug` |
| v0.2.0 | 已完成 | 定时通知、动态 endpoint、HTTP request template sink 与 MCP Skill 文档整理 |
| v0.3.0 | 进行中 | 事件通知、Hook sink 化与下一批语义事件 |

## v0.2.0：定时通知、第三方通知与 Agent Skill 文档

**状态：已完成。**

v0.2.0 在 v0.1.1 已具备的稳定短任务引用、CLI / HTTP / MCP / Remote 基础上，把 M16 的定时通知能力整理为当前版本主线，并补齐面向 Agent 的 `docs/skills` 操作说明。

核心能力：

- 定时提醒规则优先使用 `schedule + task filter`，可表达每日固定时刻、未来 24 小时即将到期、已逾期、未开始、进行中等条件。
- `notification sink` 支持标准 webhook 和 `http_template`；第三方固定 Web API 的 URL、header、body 模板和 secret ref 都保存在数据库中。
- 动态 endpoint 支持 `template` 与 `config_value`，并通过 allowed host 做 SSRF 防护。
- delivery 生成时冻结 resolved URL、method、headers、body、content type；retry/replay 不按当前 sink 模板重新渲染。
- CLI、HTTP API、Remote Client、MCP 全部暴露 `schedule_type`、`schedule_value`、`filter_source`。
- MCP tool 从 74 扩展到 95，并新增/整理 `docs/skills`，覆盖通知提醒、报表与 urgency、配置 schema、token scope、审计 action、project slug 规则等 Agent 调用说明。

对应 milestone：

- M16：定时通知、动态 endpoint 与 HTTP request template sink。
- Agent Skill 文档整理：`docs/skills/*/SKILL.md` 已覆盖 v0.2.0 时的 95 个 MCP tool。

## v0.3.0：事件通知、Hook sink 化与语义事件补齐

**状态：进行中。**

v0.3.0 的当前实现重点是把 Hook 从直接 URL 收敛到 workspace 级 outbound sink，并补齐事件触发的用户通知规则。当前规格和实现计划只实现首批事件，不一次性补完所有语义事件。

当前实现范围：

- Hook 使用 `--sink <sink-ref>`，不再使用 `--url`。
- `sink-ref` 按 workspace 隔离，Hook / Notification Rule 保存 `sink_id`。
- 新增或补齐 `project.annotated`、`project.denotated`、`task.unblocked`。
- 新增 event notification rule，让 `task.unblocked` 等事件可以通过 OpenClaw 或其他 sink 通知用户。
- notification delivery 增加 `object_kind` / `object_id`，同一张投递表可以承载 task、project 等事件对象；旧的 reminder delivery 继续使用 task 语义。
- Hook delivery 生成时冻结 sink 渲染后的 URL、method、headers、body 和 content type，retry/replay 使用历史快照，不读取当前 sink 配置重渲染。
- Notification / Hook dispatcher 已补齐单进程并发、背压和可靠投递运行时：DB delivery 表是唯一可靠队列，进程内 worker 只做短暂执行协调；claim 数量受可用并发约束，系统重启后通过 stale recovery 恢复过期 `delivering`。单进程内两个 dispatcher 共享 sink limiter；多实例部署暂不提供全局严格并发上限。

当前事件白名单：

- `task.created`
- `task.modified`
- `task.completed`
- `task.deleted`
- `task.started`
- `task.stopped`
- `task.assigned`
- `task.unassigned`
- `task.blocked`
- `task.due_changed`
- `task.priority_changed`
- `task.project_changed`
- `task.tags_changed`
- `project.archived`
- `project.annotated`
- `project.denotated`
- `task.unblocked`

已补齐的 Priority 1 语义事件：

- `task.assigned`
- `task.unassigned`
- `task.started`
- `task.stopped`
- `task.blocked`
- `task.due_changed`
- `task.priority_changed`
- `task.project_changed`
- `task.tags_changed`

下一大版本候选的 Priority 2 事件：

- `task.annotated`
- `task.denotated`
- `task.link_added`
- `task.link_removed`
- `project.created`
- `project.updated`
- `workspace.member_added`
- `workspace.member_removed`
- `workspace.member_role_changed`

事件 payload 要求：

- 所有 `task.*` 语义事件必须携带标准 task 快照，不只给 task UUID。
- `task` 快照至少包含 `uuid`、`working_id`、`workspace_id`、`description`、`status`、`project`、`priority`、核心时间字段、`tags`、`assignees`、`depends`、`blocked`，可稳定计算时包含 `urgency`。
- 事件差异信息放在 `task` 之外，例如 `previous_due` / `current_due`、`added_tags` / `removed_tags`、`previous_assignees` / `current_assignees`。
- 用户字段继续使用 `task.UserInfo`，不输出裸 UUID。

定时条件事件边界：

- `task.due_soon`、`task.overdue` 如果进入统一事件体系，应由 scheduler / reminder rule 对时间条件扫描后生成。
- `due` 字段被修改只触发 `task.due_changed`，不直接触发 `task.due_soon` 或 `task.overdue`。

规格与实施计划：

```text
docs/superpowers/specs/2026-06-09-xuanchu-event-notification-and-hook-events-design.md
docs/superpowers/plans/2026-06-09-xuanchu-event-notification-and-hook-events-implementation.md
docs/superpowers/specs/2026-06-10-xuanchu-dispatcher-concurrency-backpressure-design.md
docs/superpowers/plans/2026-06-10-xuanchu-dispatcher-concurrency-backpressure-implementation.md
```

## v0.1.1：稳定短任务标识 task_slug

**状态：已完成。**

v0.1.1 在 v0.1.0 已具备的 CLI / HTTP / MCP / Remote 基础上，补齐面向人类和 Agent 协作的稳定短任务引用。

核心能力：

- `project.slug` 收紧为 3-10 位 ASCII 英文字母和数字，必须以字母开头，存储和输出统一小写。
- `project.slug` 继续只在同一 workspace 内唯一；不同 workspace 可以复用同名 project。
- 带 project 的任务按 project 内递增序号生成 `task_slug`，格式为 `<projectSlug>-<seq>`，例如 `agentapi-1`。
- task JSON 输出新增只读字段 `task_slug`；无 project 的任务省略该字段。
- 本地 CLI 可用数字 working-set ID、UUID、UUID 前缀和 `task_slug` 定位任务。
- 远程 CLI 的纯数字 target 仍由客户端两跳解析；UUID 和 `task_slug` 直接传给服务端。
- HTTP API 与 MCP tool 只接受 UUID 或 `task_slug`，纯数字 working-set ID 返回 `task_ref_invalid`。

## M0：本地单用户 CLI

**状态：已完成。**

M0 已经把项目从设计文档推进到可运行的本地 CLI。当前能力包括：

- Go module 与 `cmd/xuanchu` 单二进制入口。
- Cobra CLI 基础结构。
- GORM 持久化层。
- 纯 Go SQLite driver：`github.com/glebarez/sqlite`。
- 隐式 local workspace。
- 本地数据库自动初始化。
- 任务核心生命周期：`add`、`list`、`next`、`info`、`modify`、`done`、`delete`。
- 基础 filter：status、project、priority、tag、自由文本、数字 working-set ID、UUID。v0.1.1 起任务引用额外支持 `task_slug`。
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
  - `project:agentapi`
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
  - 先满足 `xuanchu calc` 和后续 query/urgency 复用，不追求一次性覆盖完整 Taskwarrior calc。

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
- `xuanchu +next or due.before:tomorrow list` 这类布尔查询可用。
- `next` 默认按 urgency 排序。
- `xuanchu urgency 1` 或 `_urgency` 能输出任务 urgency；如果命令命名暂未确定，至少 app/service 层提供 explain。
- `_get`、`_ids`、`_uuids`、`_projects`、`_tags` 可脚本化使用。
- `go test ./...`、`CGO_ENABLED=0 go test ./...` 通过。

## M2：Taskwarrior 核心任务模型补齐

**状态：已完成。**

**目标：** 补齐 Taskwarrior 日常使用所需的任务字段和命令，让 Xuanchu 不再只是简单 todo CLI。

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
  - `~/.config/xuanchu/xuanchu.toml`。
  - SQLite meta/config 与文件配置合并规则。
  - 命令行临时覆盖：`rc.x=y`。
  - 保留 `--db`、`XUANCHU_DB` 的优先级。
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
  - 支持 `~/.config/xuanchu/xuanchu.toml` 与 `XDG_CONFIG_HOME`。
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
  - 当前 CLI 使用裸 workspace slug，因此 workspace slug 在同一个 Xuanchu 实例内保持唯一；project slug 只在 workspace 内唯一
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
  - `xuanchu --workspace dajee project info agentapi` 表示 `dajee` workspace 下的 `agentapi`。
  - `xuanchu --workspace partner project info agentapi` 表示另一个 workspace 下的同名 project。
  - `xuanchu project info agentapi` 只在当前 active workspace 中查找，不做跨 workspace 搜索。
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
  - `xuanchu.toml` 只作为本机启动和显示配置来源，例如 `database.path`、`color`、`json`、`date.format`、远程 CLI 连接信息。
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
- `xuanchu.toml` 不再被描述为业务配置来源。
- project 写操作有权限检查和审计记录。
- `go test ./...`、`CGO_ENABLED=0 go test ./...`、`CGO_ENABLED=0 go build ./cmd/xuanchu` 通过。

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

**目标：** 让同一个 `xuanchu` 二进制可以作为 HTTP 服务端运行，并让 CLI / Agent 通过远程 API 操作任务。M6 的重点是把“actor + workspace + project + token scope”固化成传输层协议。

**范围：**

- 服务端模式：
  - `xuanchu server --listen :8080`。
  - `--data-dir` / `--db` 指定服务端数据库。
  - graceful shutdown。
- HTTP API：
  - task CRUD/action、annotation、urgency。
  - query/report。
  - workspace/member/project/project config。
  - context/config。
  - workspace/member 基础管理。
  - import/export。
- OpenAPI 3 文档维护在 `docs/openapi/xuanchu-v1.yaml`。
- 鉴权：
  - PAT。
  - Agent token。
  - JWT 登录可作为 M6.5，如果范围过大可拆分。
  - `Authorization: Bearer <token>`。
  - token 绑定 actor，并可限制可访问 workspace。
  - token 可限制 project allowlist；scope 应引用 M5 的稳定 project id。slug 只能作为带 workspace 的人类可读输入，不能作为全局唯一标识。
- 行级隔离：
  - 所有请求必须绑定 actor。
  - 所有查询必须绑定可见 workspace。
  - 如果 token 带 project scope，task/query/report/import/export 都必须叠加 project 限制。
- 远程 CLI：
  - `xuanchu --server URL --token TOKEN list`。
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

- `xuanchu --server ... workspace add|list|info|modify|use|archive`
- `xuanchu --server ... user add|list|info|use`
- `xuanchu --server ... member list|add|role`
- `xuanchu --server ... show`

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

**状态：已完成。**

**目标：** 让企业 Agent 能通过 MCP 以结构化方式使用 Xuanchu。MCP 请求必须落在明确的 workspace scope 内，并可进一步受 project scope 限制。Agent 不应该凭提示词决定自己能看什么，权限必须来自 token 和服务端校验。

**M7 已交付内容：**

- Go 版本升级到 1.25。
- 官方 MCP Go SDK (`github.com/modelcontextprotocol/go-sdk` v1.6.1) 接入。
- `xuanchu mcp stdio` 命令，本地 MCP 通过标准输入输出运行。
- `xuanchu server` 暴露 `/mcp`，使用 Streamable HTTP 传输。
- HTTP MCP Bearer token 鉴权，复用 M6 PAT/Agent token 与 workspace/project scope。
- 21 个 MCP tools：
  - 任务：`task.add`、`task.modify`、`task.done`、`task.delete`、`task.query`、`task.get`、`task.annotate`、`task.depends`、`task.start`、`task.stop`
  - 报表：`report.run`
  - Urgency：`urgency.explain`
  - Workspace：`workspace.list`、`workspace.current`
  - Project：`project.list`、`project.get`、`project.current`
  - Context：`context.set`、`context.show`
  - Config：`config.get`、`config.set`
- 4 个 MCP resources：
  - `xuanchu://workspace/current`
  - `xuanchu://workspace/{workspace_id}`
  - `xuanchu://project/{project_id}`
  - `xuanchu://context/current`
- M6 遗留远程管理命令收口：`workspace`、`user`、`member`、`show` 均已支持远程模式。
- `/api/v1/tasks` 统一 limit：默认 200，最大 1000。
- 新增 REST endpoint：`GET/POST /api/v1/users`、`GET /api/v1/users/{user}`、`PUT /api/v1/me/active_workspace`、`POST /api/v1/workspaces/{workspace}/archive`。
- MCP tool schema golden tests 覆盖全部 tools。
- MCP tool 与 CLI/API 复用同一 `internal/app` service，不复制业务逻辑。
- `/mcp` 不进入 OpenAPI 文档。

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
  - HTTP MCP 支持 Agent token。
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
 - Agent 可以完成"查询项目待办、添加项目任务、解释 urgency、写入审计"的完整流程。

M7 规格与实现计划：

```text
docs/superpowers/specs/2026-06-01-xuanchu-m7-design.md
docs/superpowers/plans/2026-06-01-xuanchu-m7-implementation.md
```

## M8：服务端 Hook / 自动化扩展与运维交付打磨

**状态：已完成。**

**目标：** 为 `xuanchu server` 增加可审计、可控、可恢复的服务端 Hook / automation 能力，让内部事件发生后可以稳定触发外部 webhook，同时补齐与该能力直接相关的部署、发布与恢复文档。`xuanchu` 核心仍然是 workspace/project/task/权限/审计运行时，而不是业务域 adapter 市场或通用工作流编排平台。

**范围：**

- 服务端 Hook：
  - 仅做 server-side hook runtime。
  - 仅做 post-commit 异步投递语义。
  - 支持 workspace 级和 project 级 webhook hook。
  - hook 必须绑定 actor、workspace 和可选 project scope。
  - 事件集合首版聚焦稳定内部事件，例如 `task.created`、`task.modified`、`task.completed`、`task.deleted`、`project.archived`。
- 投递与恢复：
  - durable delivery queue / outbox。
  - timeout、retry、disable、dead-letter、manual replay。
  - Webhook HMAC 签名与稳定事件 envelope。
  - hook 失败不回滚已经提交成功的 task/project 事务。
- 管理面：
  - CLI 与 HTTP API 管理 hook definition、delivery state 和 replay。
  - hook 配置变更与人工 replay 必须写 audit。
  - delivery 级状态进入专用运行记录，不把每次投递尝试都膨胀成业务 audit。
- 运维交付：
  - 服务端部署文档。
  - backup / restore 演练文档。
  - CGO-free 发布产物与安装说明。

**不进入 M8：**

- 飞书 / GitHub / Jira / Slack 等业务域 adapter。
- Agent memory 系统。
- replica / sync / operation log 同步。
- 本地 CLI shell hook。
- `heartbeat` trigger。
- 没有明确业务事件支撑的 `schedule` / `one-shot` trigger。
- 事务内外部回调强一致 / 外部失败回滚本地事务。

**验收标准：**

- 管理员可以为 workspace 或 project 配置 webhook hook，并在命中事件后收到稳定 JSON payload。
- project-scoped hook 不会收到 scope 外事件。
- hook 投递失败不会回滚已提交的 task/project 事务。
- timeout、retry、disable、dead-letter、manual replay 都有端到端测试。
- hook 配置变更和人工 replay 都有 audit。
- 管理员只靠 README/部署文档即可完成服务端部署、token 配置、hook 启用和基础恢复演练。
- 所有发布产物均通过 CGO-free 验证。

**M8 已交付内容：**

- 服务端 post-commit webhook Hook runtime
- Workspace/project scoped Hook 定义
- 5 个稳定 event type：`task.created`、`task.modified`、`task.completed`、`task.deleted`、`project.archived`
- Durable delivery queue，支持 retry、dead-letter、disable、manual replay
- Webhook HMAC-SHA256 签名（`X-Xuanchu-Signature-256` header）
- Hook 管理 CLI（`hook add/list/deliveries/replay`）
- Hook 管理 HTTP API（`/api/v1/hooks/*`、`/api/v1/hook-deliveries/*`）
- Hook 配置变更与人工 replay 写入 audit log
- Secret 安全：不暴露在 CLI/HTTP response、audit、server log 中
- 出站网络防护：禁止投递到 loopback、link-local、RFC1918、RFC6598、multicast、unspecified 地址
- 部署文档：`docs/deployment.md`
- 备份恢复文档：`docs/backup-restore.md`
- OpenAPI hook schemas 和 paths
- CGO-free 交叉编译发布脚本：`scripts/release-build.sh`

M8 规格与实现计划：

```text
docs/superpowers/specs/2026-06-02-xuanchu-m8-design.md
docs/superpowers/plans/2026-06-02-xuanchu-m8-implementation.md
```

## M9：任务多 Assignee

**状态：已完成。**

**目标：** 为任务增加多 assignee 支持，并让本地 CLI、远程 CLI、HTTP API、MCP、JSON export/import、Hook payload 都能在当前 workspace 边界内稳定读写和查询任务执行者。

**范围：**

- 新增 `task_assignees` 关联表，任务与用户改为多对多，不引入主 assignee 概念。
- CLI：`@ref`、`+@ref`、`-@ref` 写语法，以及 `assignee:<ref>` / `assignee:me` 查询语法。
- App / storage：统一把 assignee ref 解析到稳定 `user_id`，并在 repo hydrate 为结构化 assignee 列表；服务端模式下只允许 assign 当前 workspace 成员。
- 错误语义固定：缺用户报 `assignee_not_found`，跨 workspace assign 报 `assignee_not_member`。
- HTTP API、remote CLI、MCP：统一支持 assignee 写字段与结构化读字段。
- JSON export/import、Hook payload：`assignees` 统一为对象数组视图。
- README / manual / OpenAPI / requirements 同步更新。

**不进入 M9：**

- 主 assignee / 协作者区分。
- assignee 级权限。
- assignee 通知/提醒（属于外部 adapter 范畴）。
- urgency 公式修改。
- 跨 workspace 的全局 “assignee:me” 视图。
- `list` / `next` / report table 的 assignee 列系统改造。

**验收标准：**

- CLI `add @alice`、`modify +@bob -@alice`、`list assignee:me` 可用。
- `task info` human 输出可显示 assignee。
- export/import 往返不丢失 assignee 数据，并兼容对象数组与字符串数组输入。
- MCP `task.add` / `task.modify` 支持 assignee 参数，`task.get` / `task.query` 返回结构化 assignees。
- HTTP API / remote CLI 支持 assignee 参数与过滤。
- Hook payload 的 `data.task.assignees` 可见。
- `go test ./...`、`CGO_ENABLED=0 go test ./...`、`CGO_ENABLED=0 go build ./cmd/xuanchu` 通过。

M9 规格与实现计划：

```text
docs/superpowers/specs/2026-06-02-xuanchu-m9-assignee-design.md
docs/superpowers/plans/2026-06-02-xuanchu-m9-assignee-implementation.md
```

## M10：Token 委托与 Impersonation

**状态：已完成。**

**目标：** 让 Agent 平台（外部 HTTP 服务）可以持有一个带 `impersonate` scope 的 Agent token，以 workspace 成员的身份发起请求，audit log 清晰区分"名义 actor（员工）"和"实际委托方（Agent 平台 service account）"。主要满足个人助理 Agent 场景：员工和 Agent 对话，Agent 代表员工操作，行为归属员工。

**范围：**

- 沿用现有 `agent` token 模型，仅新增 `impersonate` scope；M10 不新增 `service` token type，也不允许 `pat` 做 impersonation
- 新增 `impersonate` scope：只有 workspace `admin` / `owner` 才能创建带此 scope 的 Agent token，且远程/API 创建时新 token 仍必须是当前 bearer token 的子集
- 请求头 `X-Xuanchu-As: <user-name | email | uuid>`：仅带 `impersonate` scope 的 Agent token 可使用；workspace 仍按 M6 既有规则解析，若存在多 workspace 歧义则返回 `workspace_required`
- 权限交集：`subject.membership_role ∩ agent_token.scopes ∩ agent_token.workspace_allowlist ∩ agent_token.project_allowlist`，impersonation 不能提权
- Audit / access log 双重 actor：subject 驱动权限、`assignee:me` 和 active context；delegator 仅用于追责与日志
- 远程 CLI 新增 `--as` flag，作为 remote client 级配置透传 `X-Xuanchu-As`，同一条命令内所有子请求必须一致
- HTTP MCP 仅支持 request-scoped `X-Xuanchu-As` header 透传；stdio MCP 不支持 impersonation

**不进入 M10：**

- OAuth 2.0 / OIDC 完整授权流程
- 员工自助授权 Web UI
- 跨 workspace 全局 impersonation
- impersonation 时间窗口 / 审批流

**验收标准：**

- Agent token + `X-Xuanchu-As` 可以以目标 user 身份执行请求，权限受 subject role、token scope、workspace allowlist 和 project allowlist 共同约束
- 无 `impersonate` scope 时携带 `X-Xuanchu-As` 返回 `token_scope_denied`
- token 可见多个 workspace 且请求未显式指定 workspace / project_id 时返回 `workspace_required`
- 目标 user 不存在或不是 workspace 成员时统一返回 `membership_not_found`
- audit log 同时记录 actor 和 delegator
- 普通 member 无法创建带 `impersonate` scope 的 token，PAT 也不能持有该 scope
- `go test ./...`、`CGO_ENABLED=0 go test ./...`、`CGO_ENABLED=0 go build ./cmd/xuanchu` 通过

M10 规格：

```text
docs/superpowers/specs/2026-06-03-xuanchu-m10-impersonation-design.md
```

## M11：用户外部 ID 绑定

**状态：已完成。**

**目标：** 为 Xuanchu 用户增加外部 ID 绑定能力，让 Agent 能通过 `feishu:ou_xxxxx` 这类标识符指派 assignee、查询用户，并在所有返回用户信息的地方一并返回外部 ID 列表。

**M11 已交付内容：**

- `user_external_ids` 表：存储用户与外部系统 ID 的绑定关系，`(provider, external_id)` 联合唯一。
- CLI：`user bind`、`user unbind` 子命令；`user info` / `user list` 显示外部 ID。
- HTTP API：`POST/DELETE/GET /api/v1/users/{user}/external-ids`；所有返回用户的 endpoint 附带 `external_ids`。
- MCP：`user_bind` / `user_unbind` tool；`task.get` / `task.query` 的 assignee 数据附带 `external_ids`。
- App service：`resolveUser` 支持 `provider:value` 格式解析；assignee ref 自动支持外部 ID。
- JSON DTO：`AssigneeInfo` / `UserView` / `JSONAssignee` 扩展 `external_ids` 字段。
- 审计：绑定/解绑操作写入 audit log。
- Hook payload：assignee 数据自动携带 `external_ids`。

**不进入 M11：**

- 自动用户创建。
- OAuth / OIDC。
- 外部系统 API 调用。
- provider 插件系统。

**验收标准：**

- `xuanchu user bind feishu:ou_xxxxx` 能绑定外部 ID，`xuanchu user unbind feishu:ou_xxxxx` 能解绑。
- `xuanchu user info` human 输出显示外部 ID 列表。
- `xuanchu user info --json` 返回 `external_ids` 数组。
- `xuanchu add "做这件事" @feishu:ou_xxxxx` 能创建带外部 ID assignee 的任务。
- `xuanchu list assignee:feishu:ou_xxxxx` 能按外部 ID 查询任务。
- HTTP API / MCP / remote CLI 统一支持外部 ID 绑定和 assignee 解析。
- 同一个 `(provider, external_id)` 不能绑两次。
- 绑定/解绑操作写入 audit log。
- admin/owner 可以给其他用户绑定；普通用户只能给自己绑定。
- `go test ./...`、`CGO_ENABLED=0 go test ./...`、`CGO_ENABLED=0 go build ./cmd/xuanchu` 通过。

M11 规格与实现计划：

```text
docs/superpowers/specs/2026-06-03-xuanchu-m11-external-id-design.md
docs/superpowers/plans/2026-06-03-xuanchu-m11-external-id-implementation.md
```

## M12：任务外部关联

**状态：已完成。**

为任务增加外部资源轻关联能力（type + URL + title），让 Agent 能结构化地记录"这个任务和外部世界的什么东西有关"。

核心能力：

- 新增 `task_links` 表，支持自由类型的轻量关联（document、pr、ticket、design 等）
- Agent 通过 MCP 读写（`task.link_add` / `task.link_remove`），CLI 和 HTTP API 均可操作
- Hook payload 和 `--json` 输出中包含 links
- `task info` 渲染中展示 links 列表

M12 规格与实现计划：

```text
docs/superpowers/specs/2026-06-03-xuanchu-m12-project-context-task-links-design.md
docs/superpowers/plans/2026-06-03-xuanchu-m12-task-links-implementation.md
```

## M14.1：Token Scope 通配符与 Token Modify

**状态：已完成。**

**目标：** 为 token scope 增加通配符展开能力，新增 `scope list` 和 `token modify` 命令。

**已交付内容：**

- Scope 通配符展开：`*`（全部）、`resource:*`（如 `task:*`）、`*:action`（如 `*:read`）
- `scope list` / `scope ls` 命令（含 `--json`）
- `token modify` 命令：修改名称、scope、过期时间
- PAT 使用 `*` 通配符时自动剔除 `impersonate`
- `ModifyToken` 包含 revoked/expired 检查和审计日志
- HTTP API `PATCH /api/v1/tokens/{tokenRef}`
- 远程客户端 `ModifyToken`
- OpenAPI spec 更新
- 全量 CLI 命令中文 Short 描述补全

**不进入 M14.1：**

- Token workspace/project 修改
- 远程 API token scope 子集校验

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
CGO_ENABLED=0 go build ./cmd/xuanchu
```

## M13：项目 Annotation 与 Timeline

**状态：已完成。**

为项目增加 annotation 能力和聚合时间线接口。核心认知：project annotation + 该项目下所有 task annotation = 项目完整时间线。

核心能力：

- 新增 `project_annotations` 表，支持多行文本 annotation（type:text）
- Agent 通过 MCP 读写（`project.annotate` / `project.denotate` / `project.annotations` / `project.timeline`）
- CLI 和 HTTP API 均可操作
- Timeline 聚合接口通过 SQL UNION 合并 project + task annotations
- `project info` 展示最近 5 条 annotation
- `normalizeProjectSlug` 强化：只允许 3-10 位 ASCII 英文字母和数字，必须以字母开头，统一小写，确保可作为 `task_slug` 前缀并与数字 working-set ID 不冲突
- 目标风格 `xuanchu <slug> annotate <content>` 自动回退到 project

验收：

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```

## M15：MCP Tool 全量覆盖与 Agent Skill 文档

**状态：已完成。**

**目标：** 把 MCP tool 从 33 个扩展到 74 个，覆盖 CLI/HTTP 已有的全部操作，补齐集成测试，并生成面向 Agent 的 Skill 文档。同时确保 MCP 调用模式为"每次显式传 workspace/project，不依赖隐式上下文"。

**已交付内容：**

- MCP tool 从 33 扩展到 74，按领域拆分到独立文件：
  - `tools_task.go` — 16 task tool（含 denotate、link_list、export、import）
  - `tools_project.go` — 13 project tool（含 add/modify/archive/annotate/denotate/config 全套）
  - `tools_workspace.go` — 7 workspace tool（含 add/info/modify/archive/use）
  - `tools_user.go` — 7 user tool（含 add/use/list/get/bind/unbind/list_external_ids）
  - `tools_member.go` — 3 member tool（含 member_role）
  - `tools_context.go` — 5 context tool（含 get/list/delete）
  - `tools_config.go` — 4 config tool（含 get/set/list/unset）
  - `tools_hook.go` — 新文件，10 hook tool（add/list/info/modify/remove + delivery_list/delivery_info/delivery_redeliver + test/ping）
  - `tools_token.go` — 新文件，4 token tool（list/create/modify/revoke）
  - `tools_misc.go` — 新文件，audit_list + scope_list + me_get
- MCP tool 命名统一使用 `_` 分隔（如 `task_add`、`project_annotate`、`hook_delivery_redeliver`）
- AddUser 支持中文/非 ASCII 用户名：slug 推导失败时自动 fallback 到 `user-{uuid[:8]}`
- 集成测试全覆盖：17 个新测试函数覆盖所有 74 tool
- 74 tool schema golden test（`internal/mcpserver/testdata/`）
- 8 个 Agent Skill 文档（`docs/skills/*/SKILL.md`），每个 tool 含 MCP 调用示例（输入/输出 JSON）
- Skill 文档明确指导 Agent "每次调用显式传 workspace/project，不依赖 context_set / workspace_use 隐式状态"

**不进入 M15：**

- `task_search` 全文搜索 tool（已讨论，方案 A：新增 Agent 友好搜索 tool，尚未实施）

## M16：定时通知、动态 endpoint 与 HTTP request template sink

**状态：已完成。**

**目标：** 让 Xuanchu 能基于任务 `due` 和 reminder rule 生成定时通知，在到期前或逾期后提醒 assignee，并通过 OpenClaw webhook 或第三方固定 HTTP API 投递。

**已交付内容：**

- 新增 `notification_sinks`、`reminder_rules`、`notification_deliveries` 三类存储模型和 repository。
- notification sink 支持 `webhook` 和 `http_template`。
- endpoint 支持 `static_url`、`template`、`config_value` 三种模式；动态 endpoint 必须通过 allowed host 与 SSRF 校验。
- HTTP request template 的 header/body/secret ref 保存在数据库中；delivery 生成时冻结 `resolved_url`、method、headers、body、content type。
- reminder rule 支持兼容型 `due_before` / `overdue`，并支持新规则使用 `schedule + task filter` 表达每日固定时刻、即将到期、逾期、未开始、进行中等条件；audience 首版支持 `assignees`、`explicit_users`、`assignees_and_explicit_users`。
- `xuanchu server` 启动 reminder scheduler 和 notification dispatcher 后台循环。
- CLI、HTTP API、Remote Client、MCP 全部贯通；MCP tool 从 74 扩展到 95。
- delivery 支持 retry、dead-letter、disabled-skip 和人工 replay；replay 使用冻结请求快照，不重新渲染当前 sink 模板。
- 新增中文手册 [定时通知与第三方通知](docs/manual/notifications.md)。

**不进入 M16：**

- 不内置 OpenClaw、飞书、Slack、邮件 adapter。
- 不执行任意 shell、JS 或本地脚本。
- 不支持 project owner / maintainer audience。
- 不因为提醒而修改任务状态或 urgency。

规格与实施计划：

```text
docs/superpowers/specs/2026-06-08-xuanchu-scheduled-notification-design.md
docs/superpowers/plans/2026-06-08-xuanchu-scheduled-notification-implementation.md
docs/superpowers/specs/2026-06-08-xuanchu-scheduled-notification-rule-filter-extension-design.md
docs/superpowers/plans/2026-06-08-xuanchu-scheduled-notification-rule-filter-extension-implementation.md
```

## v0.1.0：基础设施与发布准备

**状态：已完成。**

**目标：** 补齐运维和可观测性基础设施，使 Xuanchu 达到可正式发布的质量标准。

**已交付内容：**

- `--config` / `XUANCHU_CONFIG` 指定 TOML 配置文件路径
- 通用日志框架（`internal/logging`）：基于 `log/slog`，支持 stderr + 文件双输出、text/json 格式、日志级别过滤
- 日志文件轮转：daily / size / none 三种模式，自动过期清理
- CLI / HTTP / MCP 三层全覆盖 panic recovery，panic 时记录堆栈到日志文件
- 版本号自动化：`debug.ReadBuildInfo()` 读取 git commit/time，ldflags 仅用于正式发布覆盖
- `[log]` TOML 配置区块，`XUANCHU_LOG_LEVEL` / `XUANCHU_LOG_FILE` 环境变量覆盖
- 集成测试覆盖 `--config`、`--version` 行为
- 全量测试 `CGO_ENABLED=0 go test ./...` 通过

v0.1.0 规格与实施计划：

```text
docs/superpowers/specs/2026-06-05-v0.1.0-infra-design.md
docs/superpowers/plans/2026-06-05-v0.1.0-infra-implementation.md
```

## 当前下一步

v0.2.0 已完成。Xuanchu 已具备从本地 CLI 到远程 CLI、HTTP API、MCP Server（95 tool）、服务端 Webhook Hook、定时通知、Token 委托、用户外部 ID 绑定、任务外部关联、项目 Annotation 与 Timeline、多数据库支持（SQLite / PostgreSQL）、通用日志与全层 panic recovery 的完整能力栈。

后续方向待定，可能包括：

- `task_search` Agent 友好全文搜索 tool
- 性能优化与大 workspace 场景验证
- 外部系统 adapter 生态
- 多端同步与 replica
