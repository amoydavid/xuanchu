# taskg Roadmap

本文档是 `taskg` 的产品路线图。目标是逐步实现 README 中定义的最终产品形态：

- 单一二进制，同时承担本地 CLI、远程 CLI 客户端、HTTP/JSON API 服务端、MCP Server。
- 使用纯 Go SQLite 方案，保持零 CGO、可跨平台交叉编译。
- 支持多用户、多 workspace、行级隔离。
- 兼容 Taskwarrior 的核心命令名、JSON 数据格式与 urgency 公式。

路线图按可独立交付、可测试、可回滚的 milestone 拆分。每个 milestone 开始前都应先写中文 spec，再用 `superpowers:writing-plans` 拆成实施计划。

## 状态总览

| Milestone | 状态 | 主题 |
|---|---|---|
| M0 | 已完成 | 本地单用户 CLI、SQLite 存储、核心任务生命周期 |
| M1 | 已完成 | 查询语言、内置报表、urgency、DOM 与 calc 基础 |
| M2 | 已完成 | Taskwarrior 核心任务模型补齐 |
| M3 | 已完成 | 配置系统、上下文、UDA、`.taskrc` 只读导入与脚本化 helper |
| M4 | 已完成 | 多 workspace、本地团队模型与权限边界 |
| M5 | 待规划 | HTTP/JSON API 与远程 CLI |
| M6 | 待规划 | MCP Server 与 Agent 工具接口 |
| M7 | 待规划 | Operation log 同步、离线复制与 Hook |
| M8 | 待规划 | 外部触发集成、飞书示例、发布与迁移打磨 |

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
  - `project:work`
  - `+urgent`
  - `-tag`
  - `status:pending`
  - `priority:H`
  - `due:today`
  - `due.before:tomorrow`
  - `due.after:2days`
  - `/pattern/`
  - 字符串引号：`project:'Home & Garden'`
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

## M4：多 Workspace、本地团队模型与权限边界

**状态：已完成。**

**目标：** 在仍然不引入 HTTP 服务端的前提下，把运行时从“单用户单 workspace”升级成“actor + workspace + role”，为 M5 的服务端化保留稳定边界。

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
- M5 可以在不重写 M4 app service 的前提下接入 HTTP/JSON API 与远程 CLI。

## M5：HTTP/JSON API 与远程 CLI

**目标：** 让同一个 `taskg` 二进制可以作为 HTTP 服务端运行，并让 CLI 通过远程 API 操作任务。

**范围：**

- 服务端模式：
  - `taskg server --listen :8080`。
  - `--data-dir` / `--db` 指定服务端数据库。
  - graceful shutdown。
- HTTP API：
  - task CRUD。
  - query/report。
  - context/config。
  - workspace/member 基础管理。
  - urgency explain。
  - import/export。
- OpenAPI 3 文档生成或维护。
- 鉴权：
  - PAT。
  - JWT 登录可作为 M5.5，如果范围过大可拆分。
  - `Authorization: Bearer <token>`。
- 行级隔离：
  - 所有请求必须绑定 actor。
  - 所有查询必须绑定可见 workspace。
- 远程 CLI：
  - `taskg --server URL --token TOKEN list`。
  - 本地/远程命令输出尽量一致。
  - 支持环境变量配置 server/token。

**不进入 M5：**

- MCP。
- op-log 同步。
- Hook。
- 飞书集成。

**验收标准：**

- 本地 CLI 与远程 CLI 在核心命令上行为一致。
- API 认证失败、权限不足、资源不存在有稳定错误结构。
- 不同 workspace/user 的数据无法越权访问。
- OpenAPI 覆盖已实现 endpoint。
- 服务端和远程 CLI 都通过 CGO-free 测试和构建。

## M6：MCP Server 与 Agent 工具接口

**目标：** 让 AI Agent 能通过 MCP 以结构化方式使用 taskg，同时保留人类可读渲染。

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
  - `workspace.switch`
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
- tool schema 测试：
  - 参数校验。
  - 错误结构。
  - workspace scope。

**不进入 M6：**

- 飞书专用适配。
- op-log 同步。
- Hook。

**验收标准：**

- 本地 MCP stdio 能被 MCP 客户端调用。
- HTTP MCP 能鉴权并限制 workspace。
- MCP tool 与 CLI/API 复用同一 app service，不复制业务逻辑。
- 每个 tool 都有 schema 和集成测试。
- Agent 可以完成“查询待办、添加任务、解释 urgency、切换 workspace”的完整流程。

## M7：Operation Log 同步与 Hook

**目标：** 支持多端离线写入、同步收敛，并提供 Taskwarrior 风格事件扩展。

**范围：**

- operation log：
  - `operations` 表。
  - op_id。
  - replica_id。
  - parent_op_id。
  - actor_user_id。
  - workspace_id。
  - task_uuid。
  - key。
  - old_value / new_value。
  - created_at。
- 写路径调整：
  - 任务修改生成 op。
  - 物化 task 表由 op 应用得到，或采用明确的双写过渡策略。
- sync：
  - `sync pull`。
  - `sync push`。
  - `sync status`。
  - 断点续传。
  - 冲突策略文档化。
- 并发安全：
  - tags 以原子 add/remove op 表达，避免完整列表覆盖丢更新。
- Hook：
  - `on-launch`。
  - `on-add`。
  - `on-modify`。
  - `on-exit`。
  - 本地脚本 Hook 与服务端 Webhook。
- Hook 安全：
  - timeout。
  - 失败回滚。
  - stderr/stdout 协议。
  - Webhook 签名。

**不进入 M7：**

- 飞书场景定制。
- WASM 插件正式化。可保留实验文档。

**验收标准：**

- 两个 replica 离线修改后可同步收敛。
- 并发添加不同 tag 不丢失。
- Hook 拒绝写入时，任务修改回滚。
- sync 与 Hook 都有端到端测试。
- 现有 CLI/API/MCP 行为不因 op-log 改造退化。

## M8：外部触发、飞书示例、发布与迁移打磨

**目标：** 把 taskg 打磨成可交付、可迁移、可部署、可被外部系统驱动的完整产品。

**范围：**

- 外部触发：
  - webhook trigger。
  - 定时 trigger。
  - 一次性 trigger。
- 飞书示例：
  - 接收飞书事件。
  - 通过 MCP 或 API 调用 `task.add` / `task.query`。
  - 回写飞书消息卡片或静默入库。
- workspace 记忆：
  - 团队偏好。
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
  - 迁移。
  - 备份恢复。

**验收标准：**

- 新用户只靠 README 可以完成安装、添加任务、查询任务、导入 Taskwarrior 数据。
- 管理员只靠文档可以部署服务端、创建 token、配置远程 CLI。
- Agent 可以通过 MCP 完成常见任务管理流程。
- 飞书示例可在测试环境跑通。
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

下一步应为 M4 编写独立需求规格：

```text
docs/superpowers/specs/YYYY-MM-DD-taskg-m4-design.md
```

M4 spec 应重点明确：

- 多 workspace 的数据模型与迁移策略。
- active workspace 与 existing local workspace 的兼容规则。
- context、UDA、config 在 workspace 维度的隔离方式。
- 本地团队模型、membership 与后续 HTTP 权限边界。
