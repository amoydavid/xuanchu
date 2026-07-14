# 面向企业项目与 Agent MCP 的 Taskwarrior 风格任务运行时（Go 版）— 需求文档

> 项目代号暂定：**xuanchu**（command-line binary），仓库根名：`task`
> 目标语言：**Go 1.25+**
> 主存储：**SQLite（纯 Go 驱动，零 CGO）**
> 形态：**单一二进制**，可同时充当 ① 本地 CLI ② 远程 CLI 客户端 ③ HTTP/JSON API 服务端 ④ MCP Server

本文档基于对上游 [Taskwarrior](https://github.com/GothenburgBitFactory/taskwarrior) 项目的特性梳理（见文末「参考来源」），叠加企业 workspace、真实项目、Agent MCP、多用户权限等扩展需求形成。xuanchu 只借鉴 Taskwarrior 的命令、查询和任务管理思路；公开 JSON、数据库、循环任务和跨入口契约采用璇础原生模型，不再承诺 Taskwarrior 兼容。每一条带 `[n]` 的脚注对应文末同号参考链接。

---

## 0. 设计原则与跨平台约束

### 0.1 核心原则（继承 Taskwarrior 哲学）

- **结构化任务而非纯文本**：每个任务是一组属性化的结构化对象，可"以外科手术般精度"过滤组织 `[19]`。
- **CLI 即查询语言**：CLI 不只是任务管理，而是个人数据库查询系统，过滤语法直观且强大 `[19]`。
- **数据主权 / 本地优先**：用户掌控数据与备份；格式开放、可随时导出迁移 `[19]`。
- **CLI 与数据模型是稳定 API，存储与同步是可替换实现**：Taskchampion 3.0.0 的演进路径已经证明该分层方案的可行性 `[6][21]`。

### 0.2 跨平台与纯 Go 约束

| 项 | 选型 | 理由 |
|---|---|---|
| 数据库引擎 | **GORM + `github.com/glebarez/sqlite`** | 当前实现已采用这套纯 Go SQLite 方案，底层走 modernc SQLite，无 CGO。不要改成 `gorm.io/driver/sqlite` 或 `github.com/mattn/go-sqlite3`。 |
| SQL / ORM | **`gorm.io/gorm` + 少量参数化 SQL** | app 层不直接接触 GORM；复杂查询通过 AST 编译到安全 SQL 条件。后续热路径如需绕开 GORM，必须先有明确性能理由和测试。 |
| CLI 框架 | **`spf13/cobra`** | 与现有实现一致，负责命令树、flag 和 completion。配置合并由 `internal/config` 和 app service 完成。 |
| HTTP 服务 | **`net/http` + `chi`** | 极简、无 CGO。 |
| MCP SDK | **官方 Go MCP SDK**（`github.com/modelcontextprotocol/go-sdk` v1.6.1） | 支持 stdio + Streamable HTTP 双传输。M7 已接入。 |
| TOML 配置 | `github.com/BurntSushi/toml` | 使用成熟 parser，不手写 TOML 语法。 |
| 表格输出 | `olekukonko/tablewriter` | 当前通过依赖链使用，human 输出必须保持脚本友好。 |
| 颜色输出 | `fatih/color` | 仅用于 human 输出；`--no-color` 和非 TTY 场景必须可关闭。 |
| 表达式解析 | 自研递归下降 + `expr-lang/expr` 用于 `calc` | 查询语言需要 Taskwarrior 语义；calc 使用成熟表达式库。 |
| 时间解析 | 自研日期解析 | 已支持 `today`、`tomorrow`、`eom/eow`、`<N>days` 等关键字。 |

**编译目标**（最小集合）：
`linux/amd64`、`linux/arm64`、`darwin/amd64`、`darwin/arm64`、`windows/amd64`。
**所有平台共享同一份代码，禁止任何 `cgo` 依赖**。

### 0.3 存储位置约定

- **本地模式**：单文件 `~/.local/share/xuanchu/xuanchu.db`（或 `$XDG_DATA_HOME/xuanchu/xuanchu.db`），与 Taskwarrior 3 的单 SQLite 文件方案对齐 `[2]`。
- **服务端模式**：服务端可指定 `--data-dir`，每个企业 workspace 仍落同一个 SQLite 实例（依靠表内 `workspace_id` 行级隔离）。当前阶段 workspace 承担 effective tenant scope；如果未来做 SaaS 多企业共用一个服务端，可在 workspace 上方增加 organization/tenant 层，且不改变 task/query/MCP 的核心语义。
- 配置文件：`~/.config/xuanchu/xuanchu.toml`（沿用 Taskwarrior 的 `XDG_CONFIG_HOME` 与 `TASKRC`/`TASKDATA` 习惯）`[13]`。

---

## 1. 企业 Workspace、Project 与 Agent 模型

Taskwarrior 原生为**单用户**模型 `[24]`。我们在其上叠加以下扩展（这些是新增需求，不来自上游）：

### 1.0 概念边界

- **Workspace**：企业 / 租户级隔离边界。一个 workspace 通常对应一个企业、团队或独立业务域，例如 `dajee`。所有 task、project、context、UDA、audit、Agent token scope 都必须落在 workspace 内。
- **Project**：workspace 内的真实企业项目，例如 `agentapi`、`erpflow`、`lark-integration`。M4 阶段 project 仍是任务字段；M5 起抬成一等实体，后续 API、token、MCP scope 都应绑定稳定 project 身份。
- **Agent**：通过 PAT / service token / MCP 连接进来的非人类 actor。Agent 的权限来自 token 和 membership，不来自提示词。token 可限制 workspace，也可限制 project allowlist。
- **Context**：人或 Agent 的当前视图过滤器，例如 `project:agentapi and +next`。context 不是权限边界，只是查询默认条件。

当前实现中没有单独的 organization/tenant 表。`workspace` 就是请求执行时的租户级作用域。未来如需 SaaS 多企业模型，可在不破坏 workspace/project/task 关系的前提下增加更上层的 organization。

### 1.1 角色

| 实体 | 关键字段 | 说明 |
|---|---|---|
| `User` | `id, name, email, password_hash, default_workspace_id, created_at` | 全局账号 |
| `Workspace` | `id, owner_user_id, slug, name, description, visibility(private/team/public), settings_json` | 企业 / 租户级任务空间 |
| `Membership` | `user_id, workspace_id, role(owner/admin/member/viewer), joined_at` | 多对多关系 |
| `ApiToken` | `id, user_id, name, type, token_prefix, token_hash, scopes_json, workspace_ids_json, project_ids_json, expires_at, revoked_at, last_used_at` | PAT 或 Agent token，用于 CLI/API/MCP 鉴权；project scope 引用稳定 project id，slug 只能在明确 workspace 后解析 |
| `AuditLog` | `id, actor_user_id, workspace_id, project_id, action, target_uuid, payload_json, created_at` | 服务端模式必备；M5 起支持按 project 查询时间线 |

### 1.2 鉴权

- **本地模式**：无鉴权，单用户隐式为 `local`。
- **服务端模式**：
  - M6 只支持 PAT / Agent token；用户名密码登录、JWT、refresh token 留给 M6.5 或后续里程碑。
  - CLI 与 MCP 客户端统一使用 `Authorization: Bearer <token>`。
  - Token capability 空数组表示无能力；workspace/project allowlist 空数组表示不额外收窄。
  - 最终权限是 `membership role 权限 ∩ token capability scope ∩ token workspace scope ∩ token project scope`。
  - 如果 token 带 project allowlist，所有 task/query/report/import/export/audit 都必须叠加 project 限制；单任务越界读返回 404 `task_not_found`，避免泄露资源存在性。

M6 已实现的 capability：

| Capability | 说明 |
|---|---|
| `task:read` / `task:write` | 任务、报表、import/export、urgency 与 task action |
| `project:read` / `project:write` | project 与 project config |
| `context:read` / `context:write` | context list/show/define/use/delete/none |
| `config:read` / `config:write` | workspace 业务配置，不含本机 TOML / `remote.token` |
| `audit:read` | audit list，需要 admin/owner role |
| `token:read` / `token:write` | token list/create/revoke |
| `workspace:read` / `workspace:write` | workspace/member 基础管理 |

### 1.3 Workspace 与 Project 的关系

- **Workspace** = 企业 / 租户级数据隔离边界。
- **Project** = 企业里的真实项目。Taskwarrior 支持可读 project 名 `[19]`，我们借鉴其人类可读性，但不再把 project 视为自由字符串。v0.1.1 起 project slug 只允许 3-10 位 ASCII 英文字母和数字，必须以字母开头，存储和输出统一小写；slug 只在所属 workspace 内有意义，不同 workspace 可以有相同 slug。M5 前 project 只是任务字段；M5 后 project 是实体，M6 token 与 M7 MCP scope 优先使用 project id，slug 只能作为带 workspace 的人类可读输入。

---

## 2. 数据模型（Task 核心 Schema）

> 字段命名和基础任务语义参考 Taskwarrior RFC `[15]` 与官方 Task 表示文档 `[4]`。璇础原生扩展和已经明确分叉的字段以本文、当前 milestone spec 与 OpenAPI 为准。

### 2.1 内置属性

| 字段 | 类型 | 来源 / 备注 |
|---|---|---|
| `uuid` | UUID v4 | 永久唯一 ID `[7]` |
| `id` | int (派生) | working set 中的行号，可与 UUID 互换使用 `[7]` |
| `title` | string | 必填；UTF-8；不允许换行 |
| `description` | string? | 可选详细描述；允许为空 |
| `status` | enum | `pending`(P) / `completed`(C) / `deleted`(D) / `waiting`(W)；`active` 是 `pending + start` 的派生状态，不是持久枚举 |
| `entry` | timestamp | 创建时间 |
| `modified` | timestamp | 最近修改 |
| `start` | timestamp | 设置后任务进入 active 状态，urgency 提升 `[7]` |
| `end` | timestamp | 完成或删除时写入 `[7]` |
| `due` | timestamp | 临近（默认 7 天内）视为 due，超期视为 overdue `[7]` |
| `wait` | timestamp | 隐藏 pending；到期客户端必须自动清空 `wait` 并改为 pending `[15]` |
| `scheduled` | timestamp | 过 `scheduled` 后任务为 ready `[15]` |
| `until` | timestamp | 到期任务自动消失 `[7]` |
| `project` | string | 任务上的可读 project slug；M5 后内部必须映射到 workspace 内的 project 实体 |
| `project_seq` | int? | project 内递增序号；与 `project/project_id` 要么同时为空，要么同时存在 |
| `task_slug` | string (派生) | v0.1.1 起输出的稳定短任务引用，格式为 `<projectSlug>-<seq>`；无 project 的任务省略 |
| `tags` | []string | 标签数组；`+tag` / `-tag` 修改语法 `[8]` |
| `priority` | enum | 默认 `H/M/L/<空>`，本质上是内置 UDA `[5][12]` |
| `depends` | []UUID | 依赖列表 `[11]` |
| `annotations` | []{entry,description} | 每条注释含时间戳与文本 `[8]` |
| `parent` | UUID? | 只表示手工父子任务，不承载循环归属 |
| `series_id` / `recurrence_at` / `recurrence_rule_snapshot` | UUID? / timestamp? / string? | 已物化循环实例的三元关联；普通任务三者均为空，occurrence 三者均存在 |
| `recurrence_overrides` | []string | occurrence 相对 Series 共享字段的单次覆盖集合 |
| `urgency` | float (派生) | 不持久化，按 §6 公式计算 |

### 2.2 多租户扩展字段（新增）

`workspace_id`(FK) · `project_id`(FK, M5+) · `creator_user_id`(FK) · `task_assignees`(task↔user 多对多关系) · `followers`([]user_id) · `external_refs`(JSON, e.g. `{"github":"owner/repo#123"}`)。循环定义是独立 `task_series` 聚合，保存 project、共享字段、canonical recurrence、rule versions 和生命周期；它不是 Task，也不能执行 done/start。

### 2.3 UDA（用户自定义属性）

继承 Taskwarrior UDA 机制 `[12][13]`：

- 类型：`string` / `numeric` / `date` / `duration`。
- 可配置 `label`、`values`（白名单枚举）、`default`、`urgency.uda.<name>.coefficient`。
- **孤儿 UDA**：任务带未在当前实例配置的 UDA 时，**必须保留数据但不允许操作**；仅 `edit` 可清空 `[12]`。
- 推荐命名空间 `<ns>.<key>`，例如 `devsync.github.issue-id` `[4]`。

### 2.4 同步语义关键约束

> Taskwarrior 同步**不支持读-改-写**整记录覆盖 `[4]`。若 tags 以「完整列表」表达，两端并发各加一标签会丢失更新。

→ 我们的存储以 **操作日志（operation log）** 为主，记录形如 `(uuid, key, old, new, op_id, ts, actor)` 的 CRDT 友好原子操作；任务当前态由 op-log 物化得到（也以单独表存储，便于查询）。

---

## 3. CLI 命令体系

### 3.1 参数三分法

参考 Taskwarrior 命令行语法 `[18]`：

- **filter**：出现在命令名之前。
- **modification**：出现在命令名之后。
- **misc**：覆盖配置（如 `rc.color=off`），任意位置；命令本身看不到这些。

多个 filter 默认 `and` 组合：
`task +home status:pending modify priority:H due:eom` `[18]`。

### 3.2 核心命令

| 命令 | 说明 |
|---|---|
| `add` | 新增任务 |
| `modify` / `mod` | 修改 |
| `done` | 完成 |
| `delete` | 软删除 |
| `start` / `stop` | 起停（写 active） |
| `annotate` / `denotate` | 注释增删 |
| `append` / `prepend` | 追加/前置描述 |
| `info` | 详情 |
| `edit` | 全字段编辑（弹 `$EDITOR`） |
| `list` / `next` / `all` / `completed` / `waiting` / `active` / `ready` / `overdue` / `blocked` / `blocking` | 报表 `[22]` |
| `import` / `export` | 版本化 `xuanchu.task-bundle/v1` 原生 bundle；不接受 Taskwarrior 任务数组 |
| `config` / `show` | 配置读写 |
| `context` | 设置默认过滤 `[22]` |
| `calc` | 表达式求值（见 §3.5） |
| `project` | M5 起管理 workspace 内的真实项目实体 |
| `series` | 管理循环任务：`add/list/info/modify/occurrences/skip/stop` |
| `user` / `workspace` / `member` / `audit` | M4 起的企业运行时命令 |
| `server` / `token` | M6 起的 HTTP 服务端与访问令牌命令 |
| `sync` | 后续同步命令，是否进入 M8 由对应 spec 决定 |
| `calendar` / `burndown.daily` / `.weekly` / `.monthly` | 后续报表增强，不作为 M0-M8 主干阻塞项 |

### 3.3 Helper（脚本可解析）

以下划线开头的命令输出**无装饰**，方便脚本与 shell 补全 `[8]`：

`_get` · `_ids` · `_uuids` · `_projects` · `_tags` · `_udas` · `_unique` · `_urgency` · `_show` · `_version` · `_zshcommands` · `_zshattributes` · `_zshids`。

### 3.4 过滤表达式（解析器需求）

支持示例（全部来自上游）`[8][19]`：

- 属性匹配：`project:agentapi` · `+urgent` · `-waiting` · `due:today` · `due.before:tomorrow` · `due.after:2days`
- 日期关键字：`today` / `tomorrow` / `eow` / `eom` / `sod` / `eod` / `<N>days` 等
- 文本与正则：`/pattern/`，受 `rc.search.case.sensitive` 控制
- 布尔代数：`and` / `or` / `xor` / `not`，括号转义：`\( ... \)` `[8]`
- 字符串引号：`project:'ERP Rewrite'`
- 状态：`pending` / `completed` / `deleted` / `waiting`
- 循环属性：`series_id` / `recurrence_at` / `task_type:normal|occurrence`；`recur/mask/imask` 不再进入查询 DSL

**解析器实现要点**：递归下降；产出 AST → SQL 翻译层（绑定参数，防注入）。AST 同时也被 §7 DOM 与 §6 Urgency 引擎共享。

### 3.5 calc 子语言

Taskwarrior `calc` 命令暴露过滤器/表达式共用的代数求值器，支持 `+ - * /`、整数与实数、科学计数法、`^`（指数）、`%`（取模）、布尔 `and/or/xor`、比较 `== != ~ !~ <= < >= >` `[20]`。

→ Go 版本应将其实现为一个可复用的表达式引擎包 `internal/expr`，被 CLI、过滤、urgency、DOM 共用。

### 3.6 报表（Report）

参考 list 命令文档 `[3]`，每份报表由 5 项配置定义：
1. `description`（文字描述）
2. `labels`（列名标题）
3. `columns`（列与格式化器，例如 `description.count`、`due.relative`）
4. `filter`（自动应用的过滤器）
5. `sort`（排序规则，例如 `start-,due+,project+/,urgency-`，`+/` 表示按该列分组分割线）`[3]`

→ Go 版本以 TOML/YAML 描述报表；内置同名报表保持与上游一致；用户可在 workspace / user 配置中覆盖。

### 3.7 en-passant 修改

允许在执行某个动作的同时附带修改/注释 `[7]`，例如：

```
task 12 done /typo/fix typo/ +reviewed
```

---

## 4. 上下文（Context）

继承 `task context` 语义 `[22]`：命名的默认过滤器，激活后所有报表自动叠加。

扩展：

- **Workspace context**：默认显示某 project / 某 sprint。
- **Global context**：跨 workspace 视图，如「所有 assignee=我 的任务」。
- 切换：`xuanchu context use sprint-23` / `xuanchu context none`。

---

## 5. 注释、依赖、循环

- **Annotations**：复合结构 `{entry, description}` `[8]`；报表中支持 `description.count` 仅显示注释数 `[3]`。
- **Dependencies**：`task <id> modify depends:<uuid>` `[11]`；衍生 `blocked` / `blocking` 报表；进入 §6 urgency 计算。
- **循环任务**：独立 `task_series` 保存规则和共享字段；有界日期查询计算 projected occurrence，进入执行期或首次有效写入时物化为带 `series_id + recurrence_at` 的 Task。每个日期槽位独立完成或跳过，前一次未完成不阻塞下一次。`parent` 只表示手工子任务，旧 `recur/mask/imask` 模型不再兼容。

---

## 6. Urgency 计算（必须忠实复刻）

### 6.1 默认系数（与上游一致）`[1]`

| 配置项 | 默认值 | 含义 |
|---|---|---|
| `urgency.user.tag.next.coefficient` | **15.0** | `+next` 特殊标签 |
| `urgency.due.coefficient` | **12.0** | 临近 due / overdue |
| `urgency.blocking.coefficient` | **8.0** | 阻塞他人 |
| `urgency.uda.priority.H.coefficient` | 6.0 | |
| `urgency.scheduled.coefficient` | 5.0 | |
| `urgency.active.coefficient` | 4.0 | |
| `urgency.uda.priority.M.coefficient` | 3.9 | |
| `urgency.age.coefficient` | 2.0 | |
| `urgency.uda.priority.L.coefficient` | 1.8 | |
| `urgency.annotations.coefficient` | 1.0 | |
| `urgency.tags.coefficient` | 1.0 | |
| `urgency.project.coefficient` | 1.0 | |
| `urgency.waiting.coefficient` | **-3.0** | |
| `urgency.blocked.coefficient` | **-5.0** | |
| `urgency.age.max` | 365 | 年龄上限（天） `[9]` |
| `urgency.inherit.coefficient` | — | 从依赖链继承的 urgency `[9]` |

### 6.2 修正规则 `[1]`

- 标签/注释计数：1 个为 0.8，2 个为 0.9，≥3 个为 1.0。
- 调参建议每次 ±1.0 小步迭代，避免任何一项主导。

### 6.3 UDA 参与紧急度 `[12]`

`urgency.uda.<name>.coefficient` 在 UDA 非空时贡献；`urgency.uda.<name>.<value>.coefficient` 可精确到某个值（含空值）。

### 6.4 用户自定义系数 `[16]`

- `urgency.user.tag.<tag>.coefficient` —— 标签级。
- `urgency.user.project.<project>.coefficient` —— 项目级。
- 支持负值（如 `urgency.user.tag.later.coefficient -6.0`）。

### 6.5 引擎实现要求

- 单独 `internal/urgency` 包。
- 提供 `Explain(uuid)` → 返回每一项系数贡献，对应 §11 中的 `urgency.explain` MCP 工具。
- 不持久化最终值，但建议持久化"显式系数快照"以便排序索引。

---

## 7. DOM（Document Object Model）

Taskwarrior 提供一套类似浏览器 DOM 的引用语法 `[8]`，例如：

- `task _get 12.title` · `task _get 12.entry 12.modified`（多字段）
- `<date>.weekday`（0=Sun~6=Sat）· `<date>.julian`（年内日序）`[8]`
- 虚拟标签：`task _get 1.tag.DUE` 在虚拟标签满足时输出 `DUE`，否则空且非零退出码 `[8]`

**Go 实现要求**：

- 将 DOM 实现为查询子语言，独立包 `internal/dom`。
- 同一引擎服务 CLI（`_get`）、MCP（`task.get`）、过滤表达式（属性引用部分）、Hook 上下文渲染。

---

## 8. Hook 系统

Taskwarrior 支持事件驱动 hooks `[25]`：

| 事件 | 触发时机 |
|---|---|
| `on-launch` | CLI 启动 |
| `on-add` | 新增任务 |
| `on-modify` | 修改任务 |
| `on-exit` | CLI 退出 |

我们将 hook 扩展为三种执行形态：

1. **本地脚本 Hook**（CLI 模式）：与上游兼容；stdin 收任务 JSON，stdout 返回更新后的任务或拒绝。
2. **服务端 Webhook**：HTTP POST 到 URL；超时与重试策略可配；用于外部通知、CI 触发、协作系统回写等。
3. **内嵌处理器**：Go plugin / WASM 插件（M8+ 评估），用于沙箱化扩展。

---

## 9. 存储与同步

### 9.1 存储

- 引擎：**SQLite + WAL 模式**，通过 GORM + `github.com/glebarez/sqlite` 使用纯 Go SQLite。
- 表结构由 §附录 A DDL 描述；每个表均带 `workspace_id` 用于行级隔离。
- 当前迁移由 `internal/storage` 聚合，继续保持幂等和向前兼容。后续如引入独立迁移工具，必须兼容纯 Go SQLite。

### 9.2 同步

- 不复刻 taskserver/taskd（3.0 已抛弃）`[6][21]`。
- 同步模型：**operation-log 复制**。
  - 每次写入产出一条不可变 op：`(op_id, replica_id, parent_op_id, uuid, key, old, new, ts)`。
  - 多端通过比较 op-log 收敛，避免读-改-写丢失 `[4]`。
- 服务端 = 权威 op-log；客户端可离线累计 op，重连后批量推送。
- 跨环境迁移使用 `xuanchu.task-bundle/v1`，完整保存 Series、rule versions、已物化 occurrence、tombstone 和普通任务；projected occurrence 不导出。Taskwarrior JSON 数组和旧 `recur/mask/imask` 字段被明确拒绝，不做猜测性转换。

### 9.3 备份

- `xuanchu backup` → 复制 SQLite 文件（VACUUM INTO）+ 导出 JSON 双格式；管理员可在服务端定时任务。

---

## 10. 配置系统

### 10.1 兼容上游 `[13]`

- `.taskrc` 简单 `name = value` 语法；支持 `include`。
- 环境变量优先级：`TASKDATA` > `TASKRC` > `XDG_CONFIG_HOME`；命令行 `rc.x=y` 覆盖文件。

### 10.2 配置分层（新增）

配置分成三类，不混用：

1. **本机配置**：来自 `xuanchu.toml`、环境变量、CLI flag 和 `rc.*`。只描述当前机器如何启动和显示 Xuanchu，例如 `database.path`、`color`、`json`、`date.format`、远程 CLI 的 server/token 路径。
2. **Workspace 业务配置**：存 DB，带 `workspace_id`，受权限和 audit 约束。包括 UDA schema、urgency UDA 系数、context、report 默认配置、workspace 级 Agent 记忆。
3. **Project 配置**：M5 project 实体化后引入，挂在 project/workspace 下。包括 project 默认 context、project 级 webhook、project 级 Agent 背景和约束。

`rc.x=y` 只影响本次命令。它可以覆盖本机显示和连接行为，也可以作为显式请求参数参与一次操作，但不能变成跨 workspace 的业务默认值。

M5 起，project 配置只通过 `project config get/set/unset/list <project>` 访问；无 scope 的 `config get/set/list` 不显示 project 配置。

### 10.3 文件格式

- 本机配置采用 **TOML**（Go 友好）。
- `.taskrc` 保留为**只读迁移输入**。导入时应把 UDA、context、urgency 等业务 key 写入当前 workspace 的 DB 配置，而不是作为全局运行时配置长期读取。
- 任何服务端 HTTP/MCP 请求都不得依赖调用者本机 TOML 来决定 workspace 业务规则。

---

## 11. 服务端 + MCP 设计

### 11.1 API 分层

```
┌──────────────────────────────────────┐
│ MCP Server (stdio + Streamable HTTP) │  ← AI Agent
├──────────────────────────────────────┤
│ HTTP/JSON API (OpenAPI 3)            │  ← Web / 第三方 / 远程 CLI
├──────────────────────────────────────┤
│ Core Domain Service (Go)             │
│   - TaskService                      │
│   - QueryEngine (DOM + filter AST)   │
│   - UrgencyCalculator                │
│   - HookDispatcher                   │
│   - SyncEngine (op-log)              │
├──────────────────────────────────────┤
│ Storage Layer (GORM + SQLite)        │
└──────────────────────────────────────┘
```

> 设计上**核心服务不依赖传输层**；CLI 本地模式可直接 in-process 调用 Core，无需启动 HTTP/MCP。

### 11.2 MCP 工具暴露

| Tool | 描述 | 关键参数 |
|---|---|---|
| `task.add` | 添加任务 | `workspace_id, title, description?, project?, tags?, due?, ...` |
| `task.modify` | 修改 | `filter \| uuid`, modifications |
| `task.done` | 完成 | `uuid \| id` |
| `task.delete` | 删除 | 同上 |
| `task.query` | 通用查询 | filter 字符串或结构化 AST |
| `task.get` | DOM 取值 | DOM 表达式（§7） |
| `task.annotate` | 加注释 | `uuid, text` |
| `task.depends` | 加/移依赖 | `uuid, add[], remove[]` |
| `task.start` / `task.stop` | 起停 | `uuid` |
| `report.run` | 跑预定义报表 | `name, extra_filter?` |
| `urgency.explain` | 解释 urgency 构成 | `uuid` |
| `workspace.list` / `workspace.current` | 企业 workspace | — |
| `project.list` | 当前 workspace 内的项目 | `workspace_id` |
| `project.get` | 读取单个项目 | `workspace_id, project_id \| project` |
| `project.current` | 当前 project scope | `workspace_id` |
| `context.set` / `context.show` | 上下文 | — |
| `config.get` / `config.set` | 配置 | — |

每个工具：**返回 `{data: 结构化JSON, rendered: 文本}`**，便于 Agent 解析又便于人读。

### 11.3 鉴权与隔离

- MCP 客户端连接需带 PAT 或 Agent token；token 决定可见 workspace，也可以限制 project allowlist。
- 数据库层强制注入 `workspace_id`；推荐用 SQL view + 触发器或仓储层守卫两种手段双重校验。
- project scope 在 M5 后应基于 project 实体；`task.project` 字符串继续用于 human 输出、迁移导出和 CLI 查询输入，内部权限与 MCP/API scope 不应依赖裸 slug。所有 slug 解析都必须发生在明确 workspace 内。

---

## 12. CLI 工程要求

- 二进制名：`xuanchu`。
- 本地模式：`xuanchu add ...`（直连 SQLite）。
- 远程模式：`xuanchu --server https://... --token ... add ...`。
- 远程连接配置优先级：CLI flag > `XUANCHU_SERVER` / `XUANCHU_TOKEN` > `xuanchu.toml` > 空值。
- `xuanchu.toml` 中的 `remote.token` 是本机便利配置；如果文件权限比 `0600` 更宽，CLI 应输出 warning。
- 远程 CLI 覆盖核心 task/report/project/project config/context/config/helper/import/export/audit/token 命令；`edit`、`.taskrc import` 等本机语义命令暂不支持远程。
- 全命令支持 `--json` 输出（脚本化）。
- 提供 shell 补全：`xuanchu completion zsh|bash|fish|powershell`，利用 `_xxx` helper 命令。
- 所有命令必须 100% 可脚本化，stderr/stdout 严格分离。

---

## 13. Agent 驱动的外部系统集成

> 不在 Taskwarrior 上游范围内，列出以便后续展开。

- 触发源：外部 webhook、协作系统事件、代码托管事件、定时（heartbeat）、一次性触发。
- 触发动作：adapter 标准化事件后交给 Agent；Agent 通过 Xuanchu MCP/API 调 `task.add` / `task.query` / `task.modify`，再把结果写回外部系统或静默入库。
- adapter 示例：飞书、GitHub、Jira、Slack 等都可以接入，但它们不是 Xuanchu 的核心目标。Xuanchu 核心只关心 actor、workspace、project、task、权限和审计。
- 记忆机制：workspace 挂企业偏好，project 挂项目背景和约束，Agent 读取的是服务端 DB 中的配置/记忆摘要，不读取操作者本机 TOML。

---

## 14. 兼容性目标

1. **JSON 互通**：`task export | xuanchu import` 与反向均无损 `[4]`。
2. **CLI 同形**：核心命令名/参数与上游一致。
3. **Urgency 默认公式与上游一致** `[1]`。
4. **`.taskrc` 只读兼容**：解析能识别绝大多数上游配置项 `[13]`。

---

## 15. 明确不做（非目标）

- 不复刻 taskserver/taskd 协议 `[6][21]`。
- 第一版不做 GUI。
- 第一版不做时间跟踪（与上游一致，让出给类 Timewarrior 工具）`[7]`。
- 不引入任何 CGO 依赖。

---

## 16. 里程碑

| 阶段 | 范围 |
|---|---|
| **M0** | 单用户单 workspace；SQLite 存储；`add/list/modify/done/delete/info`；TOML 配置 |
| **M1** | 过滤表达式 + 报表系统 + Urgency 计算 + DOM `_get` + `calc` |
| **M2** | Annotations、Dependencies、Recurring、waiting/active/ready 等核心任务模型 |
| **M3** | 配置系统、context、UDA、`.taskrc` 只读导入 |
| **M4** | 企业 workspace、用户、成员、权限、审计、行级隔离 |
| **M5** | Project 实体化与 workspace/project 配置边界 |
| **M6** | HTTP/JSON API、远程 CLI、PAT/Agent token（已实现） |
| **M7** | 企业 Agent MCP Server（stdio + Streamable HTTP） |
| **M8** | Agent 驱动的外部集成、trigger、Hook、发布、迁移与备份打磨 |

---

## 附录 A：SQLite 表结构（草案）

```sql
-- 多租户
CREATE TABLE users (
  id            TEXT PRIMARY KEY,        -- UUID
  email         TEXT UNIQUE NOT NULL,
  name          TEXT NOT NULL,
  password_hash TEXT,                    -- 服务端模式
  default_workspace_id TEXT,
  created_at    INTEGER NOT NULL
);

CREATE TABLE workspaces (
  id            TEXT PRIMARY KEY,
  slug          TEXT NOT NULL,
  name          TEXT NOT NULL,
  owner_user_id TEXT NOT NULL REFERENCES users(id),
  visibility    TEXT NOT NULL DEFAULT 'private',
  settings_json TEXT NOT NULL DEFAULT '{}',
  created_at    INTEGER NOT NULL,
  UNIQUE(slug)
);

CREATE TABLE memberships (
  user_id      TEXT NOT NULL REFERENCES users(id),
  workspace_id TEXT NOT NULL REFERENCES workspaces(id),
  role         TEXT NOT NULL CHECK(role IN ('owner','admin','member','viewer')),
  joined_at    INTEGER NOT NULL,
  PRIMARY KEY (user_id, workspace_id)
);

CREATE TABLE api_tokens (
  id              TEXT PRIMARY KEY,
  user_id         TEXT NOT NULL REFERENCES users(id),
  name            TEXT NOT NULL,
  type            TEXT NOT NULL CHECK(type IN ('pat','agent')),
  token_prefix    TEXT NOT NULL,
  token_hash      TEXT NOT NULL UNIQUE,
  scopes_json     TEXT NOT NULL DEFAULT '[]',
  workspace_ids_json TEXT NOT NULL DEFAULT '[]',
  project_ids_json   TEXT NOT NULL DEFAULT '[]',
  expires_at      INTEGER,
  revoked_at      INTEGER,
  last_used_at    INTEGER,
  created_at      INTEGER NOT NULL
);

-- Project 实体（M5+）
CREATE TABLE projects (
  id            TEXT PRIMARY KEY,
  workspace_id  TEXT NOT NULL REFERENCES workspaces(id),
  slug          TEXT NOT NULL,
  name          TEXT NOT NULL,
  description   TEXT NOT NULL DEFAULT '',
  status        TEXT NOT NULL DEFAULT 'active',
  settings_json TEXT NOT NULL DEFAULT '{}',
  created_at    INTEGER NOT NULL,
  modified_at   INTEGER NOT NULL,
  archived_at   INTEGER,
  UNIQUE(workspace_id, slug),             -- slug 只在 workspace 内唯一
  UNIQUE(id, workspace_id)                -- 供 tasks 复合外键校验同 workspace
);
CREATE INDEX idx_projects_ws_status ON projects(workspace_id, status);

-- 任务核心（物化态；权威来自 operations）
CREATE TABLE tasks (
  uuid          TEXT PRIMARY KEY,
  workspace_id  TEXT NOT NULL REFERENCES workspaces(id),
  project_id    TEXT,
  title         TEXT NOT NULL,
  description   TEXT,
  status        TEXT NOT NULL,            -- pending/completed/deleted/waiting
  entry         INTEGER NOT NULL,
  modified      INTEGER NOT NULL,
  end_ts        INTEGER,
  due           INTEGER,
  wait          INTEGER,
  scheduled     INTEGER,
  until         INTEGER,
  project       TEXT,
  project_seq   INTEGER,
  priority      TEXT,                     -- H/M/L/NULL
  start         INTEGER,
  parent        TEXT,
  series_id     TEXT,
  recurrence_at INTEGER,
  recurrence_rule_snapshot TEXT,
  recurrence_overrides_json TEXT NOT NULL DEFAULT '[]',
  creator_user_id  TEXT REFERENCES users(id),
  external_refs_json TEXT NOT NULL DEFAULT '{}',
  FOREIGN KEY (project_id, workspace_id) REFERENCES projects(id, workspace_id)
);
CREATE INDEX idx_tasks_ws_status ON tasks(workspace_id, status);
CREATE INDEX idx_tasks_ws_project_id ON tasks(workspace_id, project_id);
CREATE INDEX idx_tasks_ws_due ON tasks(workspace_id, due);
CREATE UNIQUE INDEX idx_tasks_ws_project_seq
  ON tasks(workspace_id, project_id, project_seq);
CREATE UNIQUE INDEX idx_tasks_ws_series_slot
  ON tasks(workspace_id, series_id, recurrence_at)
  WHERE series_id IS NOT NULL AND recurrence_at IS NOT NULL;

CREATE TABLE task_series (
  id              TEXT PRIMARY KEY,
  workspace_id    TEXT NOT NULL REFERENCES workspaces(id),
  project_id      TEXT NOT NULL,
  title           TEXT NOT NULL,
  description     TEXT,
  status          TEXT NOT NULL,          -- active/ended/stopped
  recurrence_rule TEXT NOT NULL,
  first_due       INTEGER NOT NULL,
  until           INTEGER,
  effective_end_at INTEGER,
  stop_reason     TEXT,
  priority        TEXT,
  created_by      TEXT NOT NULL,
  created_at      INTEGER NOT NULL,
  modified_at     INTEGER NOT NULL,
  FOREIGN KEY (project_id, workspace_id) REFERENCES projects(id, workspace_id)
);
CREATE INDEX idx_task_series_ws_project_status
  ON task_series(workspace_id, project_id, status);

CREATE TABLE task_series_rule_versions (
  id              TEXT PRIMARY KEY,
  series_id       TEXT NOT NULL REFERENCES task_series(id) ON DELETE RESTRICT,
  effective_from  INTEGER NOT NULL,
  recurrence_rule TEXT NOT NULL,
  created_by      TEXT NOT NULL,
  created_at      INTEGER NOT NULL,
  UNIQUE(series_id, effective_from)
);

CREATE TABLE task_assignees (
  task_uuid TEXT NOT NULL REFERENCES tasks(uuid) ON DELETE CASCADE,
  user_id   TEXT NOT NULL REFERENCES users(id),
  PRIMARY KEY (task_uuid, user_id)
);
CREATE INDEX idx_task_assignees_user_id ON task_assignees(user_id);

CREATE TABLE task_tags (
  task_uuid TEXT NOT NULL REFERENCES tasks(uuid) ON DELETE CASCADE,
  tag       TEXT NOT NULL,
  PRIMARY KEY (task_uuid, tag)
);

CREATE TABLE task_dependencies (
  task_uuid TEXT NOT NULL REFERENCES tasks(uuid) ON DELETE CASCADE,
  depends_on TEXT NOT NULL,
  PRIMARY KEY (task_uuid, depends_on)
);

CREATE TABLE task_annotations (
  task_uuid TEXT NOT NULL REFERENCES tasks(uuid) ON DELETE CASCADE,
  entry     INTEGER NOT NULL,
  description TEXT NOT NULL,
  PRIMARY KEY (task_uuid, entry, description)
);

CREATE TABLE task_uda_values (
  workspace_id TEXT NOT NULL,
  task_uuid TEXT NOT NULL REFERENCES tasks(uuid) ON DELETE CASCADE,
  name      TEXT NOT NULL,
  value     TEXT NOT NULL,                 -- 统一以字符串存，类型由 schema 解释
  value_type TEXT,
  orphan    INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (task_uuid, name)
);

CREATE TABLE task_followers (
  task_uuid TEXT NOT NULL REFERENCES tasks(uuid) ON DELETE CASCADE,
  user_id   TEXT NOT NULL REFERENCES users(id),
  PRIMARY KEY (task_uuid, user_id)
);

-- UDA Schema（workspace 级）
CREATE TABLE uda_schemas (
  workspace_id TEXT NOT NULL REFERENCES workspaces(id),
  name         TEXT NOT NULL,
  type         TEXT NOT NULL,             -- string/numeric/date/duration
  label        TEXT,
  values_json  TEXT,                       -- 枚举白名单
  default_val  TEXT,
  PRIMARY KEY (workspace_id, name)
);

-- Operation Log（同步与审计基石）
CREATE TABLE operations (
  op_id         TEXT PRIMARY KEY,           -- UUIDv7 / ULID
  replica_id    TEXT NOT NULL,
  parent_op_id  TEXT,
  workspace_id  TEXT NOT NULL REFERENCES workspaces(id),
  task_uuid     TEXT NOT NULL,
  key           TEXT NOT NULL,
  old_value     TEXT,
  new_value     TEXT,
  actor_user_id TEXT REFERENCES users(id),
  created_at    INTEGER NOT NULL
);
CREATE INDEX idx_ops_ws_task ON operations(workspace_id, task_uuid, created_at);

-- 配置（三层）
CREATE TABLE configs (
  workspace_id  TEXT NOT NULL DEFAULT '',  -- server/user scope 可为空串；project/workspace 写真实 workspace id
  scope         TEXT NOT NULL,             -- server/workspace/project/user
  scope_id      TEXT NOT NULL DEFAULT '',  -- server scope 用空串，project scope 用 project id
  key           TEXT NOT NULL,
  value         TEXT NOT NULL,
  PRIMARY KEY (workspace_id, scope, scope_id, key)
);

-- 报表与上下文
CREATE TABLE reports (
  workspace_id TEXT NOT NULL REFERENCES workspaces(id),
  name         TEXT NOT NULL,
  spec_json    TEXT NOT NULL,              -- description/labels/columns/filter/sort
  PRIMARY KEY (workspace_id, name)
);

CREATE TABLE contexts (
  user_id      TEXT NOT NULL REFERENCES users(id),
  workspace_id TEXT,
  name         TEXT NOT NULL,
  filter_expr  TEXT NOT NULL,
  active       INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, workspace_id, name)
);

-- 审计
CREATE TABLE audit_logs (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  actor_user_id TEXT REFERENCES users(id),
  workspace_id  TEXT REFERENCES workspaces(id),
  project_id    TEXT,
  action        TEXT NOT NULL,
  target_uuid   TEXT,
  payload_json  TEXT,
  created_at    INTEGER NOT NULL
);
CREATE INDEX idx_audit_project_time ON audit_logs(workspace_id, project_id, created_at);
```

> 注：`operations` 是权威表，`tasks` 与衍生表是物化视图。可用触发器或服务层在写入 op 时同步更新物化态；亦可只在物化态写入并后台异步生成 op-log（在 M0–M2 简化路径下可接受）。
>
> `workspaces.slug` 设计为实例内唯一，因为当前 CLI/API 使用裸 `--workspace <slug|uuid>` 解析 workspace。如果未来需要同一实例内多 owner 复用 workspace slug，必须先引入 `owner/slug` 或 org scope 形式，不能悄悄放宽唯一性。
>
> `projects.slug` 只在 `(workspace_id, slug)` 内唯一；不同 workspace 可以复用同名 project。v0.1.1 起 slug 只允许 3-10 位 ASCII 英文字母和数字，必须以字母开头，并统一小写。`tasks.project_id` 非空时必须指向同一个 `workspace_id` 下的 project，数据库复合外键和 app/service 层都要校验这一点。`tasks.project` 字符串仅用于 human 输出、迁移导出和人类输入。
>
> M5 起 SQLite 连接必须启用 `PRAGMA foreign_keys = ON`；如果实现发现既有库需要重建 `tasks` 才能声明复合外键，应在 M5 迁移中完成，不把跨 workspace project 引用只留给文档约定。
>
> `configs.workspace_id` 和 `configs.scope_id` 固定使用非 NULL 空字符串表示全局 scope，且 `workspace_id` 进入主键，避免 SQLite 复合主键中的 NULL 唯一性陷阱，也避免不同 workspace 的配置 key 互相覆盖。

---

## 附录 B：CLI 命令分级清单

- **M0**：`add` `modify` `done` `delete` `info` `list` `next` `config` `show` `import` `export`
- **M1**：过滤表达式与报表基础：`all` `completed` `overdue` `calc` `urgency` `_urgency` `_get` `_ids` `_uuids` `_projects` `_tags`
- **M2**：任务核心模型扩展：`waiting` `active` `ready` `blocked` `blocking` `annotate` `denotate` `append` `prepend` `edit` `start` `stop`。M2 曾引入的 `recur` 任务字段已在 v0.5.7 删除。
- **M3**：配置、context 与 UDA：`context` `_udas` `_unique` `_show` `_version` `completion` `config import-taskrc`
- **M4**：`user` `workspace` `member` `audit`
- **M5**：`project`
- **M6**：`server` `token`，以及 `--server` / `--token` 远程模式
- **M7**：MCP Server 入口
- **v0.5.7**：独立 `task_series`、日历范围投影、按需物化、专用 CLI/HTTP/MCP/Remote/Web Console 闭环
- **M8+**：`sync` `burndown.*` 外部 trigger / adapter 相关命令

---

## 附录 C：MCP 工具 JSON Schema 关键字段（草案）

> 完整 JSON Schema 文件后续放 `mcp/schemas/*.json`。

```jsonc
// task.add
{
  "type": "object",
  "required": ["workspace_id", "title"],
  "properties": {
    "workspace_id": {"type": "string", "format": "uuid"},
    "title":        {"type": "string", "minLength": 1},
    "description":  {"type": "string"},
    "project":      {"type": "string", "description": "当前 workspace 内的企业项目 slug；不同 workspace 可以重复"},
    "project_id":   {"type": "string", "format": "uuid", "description": "M5 后优先使用的稳定 project 身份"},
    "tags":         {"type": "array", "items": {"type": "string"}},
    "assignees":    {"type": "array", "items": {"type": "string"}},
    "priority":     {"type": "string", "enum": ["H","M","L"]},
    "due":          {"type": "string"},      // 接受 ISO8601 或 Taskwarrior 关键字
    "scheduled":    {"type": "string"},
    "wait":         {"type": "string"},
    "depends":      {"type": "array", "items": {"type":"string","format":"uuid"}},
    "udas":         {"type": "object", "additionalProperties": {"type":"string"}}
  }
}
```

`project_id` 是 API/MCP 的优先 project 身份。M5 起采用严格 project 注册：请求中的 `project` 必须在有效 workspace 内解析到已存在 project，不存在时返回参数错误，不自动创建。如果请求同时传入 `project` 和 `project_id`，服务端必须先在有效 workspace 内解析 `project`，并要求解析结果与 `project_id` 相同；不一致时返回参数错误（HTTP 400 / MCP invalid_params）。如果只传 `project_id`，仍需校验调用者对该 project 所属 workspace 有权限。若请求同时带 `--workspace` / `workspace_id` 与 `project_id`，该 project 必须属于该 workspace，否则返回错误。

---

## 参考来源

> 所有引用的脚注与正文中 `[n]` 对应。

1. Taskwarrior — Urgency. <https://taskwarrior.org/docs/urgency/>
2. Taskwarrior — Upgrading to Taskwarrior 3. <https://taskwarrior.org/docs/upgrade-3/>
3. Taskwarrior — List Report. <https://taskwarrior.org/docs/commands/list/>
4. Taskwarrior — Task Representation. <https://taskwarrior.org/docs/task/>
5. Taskwarrior — Priority. <https://taskwarrior.org/docs/priority/>
6. Taskwarrior — Release v3.0.0. <https://github.com/GothenburgBitFactory/taskwarrior/releases/tag/v3.0.0>
7. Taskwarrior — Terminology. <https://taskwarrior.org/docs/terminology/>
8. Taskwarrior — DOM / Examples / Commands. <https://taskwarrior.org/docs/dom/> · <https://taskwarrior.org/docs/examples/> · <https://taskwarrior.org/docs/commands/>
9. vim-taskwarrior issue #106（urgency 系数明细）. <https://github.com/blindFS/vim-taskwarrior/issues/106>
10. Discussion #3381（v3.0 task DB 接口讨论）. <https://github.com/GothenburgBitFactory/taskwarrior/discussions/3381>
11. Taskwarrior 2.3.0 Command Reference. <https://taskwarrior.org/download/task-2.3.0.ref.pdf>
12. Taskwarrior — UDA. <https://taskwarrior.org/docs/udas/>
13. Taskwarrior — taskrc.5. <https://taskwarrior.org/docs/man/taskrc.5/>
14. Taskchampion repo. <https://github.com/GothenburgBitFactory/taskchampion>
15. Taskwarrior RFC: task.md. <https://github.com/GothenburgBitFactory/taskwarrior/blob/develop/doc/devel/rfcs/task.md>
16. Taskwarrior — Best Practices. <https://taskwarrior.org/docs/best-practices/>
17. How to setup Taskwarrior 3.0 sync. <https://www.leonardgomez.com/blogs/how-to-setup-taskwarrior-sync.html>
18. Taskwarrior — Command Line Syntax. <https://taskwarrior.org/docs/syntax/>
19. Smin Rana — Taskwarrior: The Power and Pain of Terminal-Based Task Management. <https://sminrana.com/taskwarrior-the-power-and-pain-of-terminal-based-task-management/>
20. Taskwarrior — Calc Command. <https://taskwarrior.org/docs/commands/calc/>
21. Discussion #3291（v3.0.0 综述）. <https://github.com/GothenburgBitFactory/taskwarrior/discussions/3291>
22. WingTask — Taskwarrior Commands. <https://docs.wingtask.com/docs/taskwarrior_commands/>
23. tasklib（Python 客户端结构参考）. <https://tasklib.readthedocs.io/en/latest/>
24. Taskwarrior — Documentation 总入口. <https://taskwarrior.org/docs/>
25. Mastering Taskwarrior（Hook 一章）. <https://wired.wasql.com/articles/Mastering_Taskwarrior>
26. Taskwarrior — Syncing Tasks. <https://taskwarrior.org/docs/sync/>
27. Taskwarrior — 30-Second Tutorial. <https://taskwarrior.org/docs/30second/>
28. modernc.org/sqlite（纯 Go SQLite 驱动）. <https://pkg.go.dev/modernc.org/sqlite>
29. Model Context Protocol — 规范. <https://modelcontextprotocol.io/>
30. Cobra CLI 框架. <https://github.com/spf13/cobra>
