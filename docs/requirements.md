# Taskwarrior 风格任务管理系统（Go 重构版）— 需求文档

> 项目代号暂定：**taskg**（command-line binary），仓库根名：`task`
> 目标语言：**Go 1.22+**
> 主存储：**SQLite（纯 Go 驱动，零 CGO）**
> 形态：**单一二进制**，可同时充当 ① 本地 CLI ② 远程 CLI 客户端 ③ HTTP/JSON API 服务端 ④ MCP Server

本文档基于对上游 [Taskwarrior](https://github.com/GothenburgBitFactory/taskwarrior) 项目的特性梳理（见文末「参考来源」），叠加多用户 / 多 workspace / MCP 等扩展需求形成。每一条带 `[n]` 的脚注对应文末同号参考链接。

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
| 数据库引擎 | **`modernc.org/sqlite`** | 纯 Go 翻译版的 SQLite，无 CGO，可跨平台交叉编译到 linux/darwin/windows 的 amd64/arm64。 |
| SQL Builder | **`database/sql` + `sqlc`** 或 **`ent`** | 优先 `sqlc`，保留手写 SQL；表结构演进用 `goose` / `golang-migrate`（均支持 modernc 驱动）。 |
| CLI 框架 | **`spf13/cobra`** + **`spf13/viper`** | 与 Go 生态对齐，便于子命令组织与配置合并。 |
| HTTP 服务 | **`net/http` + `chi`** | 极简、无 CGO。 |
| MCP SDK | **官方 Go MCP SDK**（`github.com/modelcontextprotocol/go-sdk`）或自研适配层 | 支持 stdio + Streamable HTTP/SSE 双传输。 |
| 表格输出 | `olekukonko/tablewriter` | |
| 颜色输出 | `fatih/color`（自动按 TTY 关闭） | |
| 表达式解析 | 自研（PEG/手写递归下降） | 因需要实现 Taskwarrior 过滤语法和 calc 子语言。 |
| 时间解析 | `araddon/dateparse` 辅助 + 自研 `eom/eow/sod/...` 关键字 | |

**编译目标**（最小集合）：
`linux/amd64`、`linux/arm64`、`darwin/amd64`、`darwin/arm64`、`windows/amd64`。
**所有平台共享同一份代码，禁止任何 `cgo` 依赖**。

### 0.3 存储位置约定

- **本地模式**：单文件 `~/.local/share/taskg/taskg.db`（或 `$XDG_DATA_HOME/taskg/taskg.db`），与 Taskwarrior 3 的单 SQLite 文件方案对齐 `[2]`。
- **服务端模式**：服务端可指定 `--data-dir`，每个 tenant/workspace 仍落同一个 SQLite 实例（依靠表内 `workspace_id` 行级隔离）。如未来需要扩展，可在不变更上层 API 的前提下替换为 PostgreSQL。
- 配置文件：`~/.config/taskg/taskg.toml`（沿用 Taskwarrior 的 `XDG_CONFIG_HOME` 与 `TASKRC`/`TASKDATA` 习惯）`[13]`。

---

## 1. 角色与多租户模型

Taskwarrior 原生为**单用户**模型 `[24]`。我们在其上叠加以下扩展（这些是新增需求，不来自上游）：

### 1.1 角色

| 实体 | 关键字段 | 说明 |
|---|---|---|
| `User` | `id, name, email, password_hash, default_workspace_id, created_at` | 全局账号 |
| `Workspace` | `id, owner_user_id, slug, name, description, visibility(private/team/public), settings_json` | 任务容器，对应"团队 / 项目空间" |
| `Membership` | `user_id, workspace_id, role(owner/admin/member/viewer), joined_at` | 多对多关系 |
| `ApiToken` | `id, user_id, name, hashed_token, scopes_json, workspace_scope(NULL=全部), expires_at` | 个人访问令牌（PAT），用于 CLI/MCP 鉴权 |
| `AuditLog` | `id, actor_user_id, workspace_id, action, target_uuid, payload_json, created_at` | 服务端模式必备 |

### 1.2 鉴权

- **本地模式**：无鉴权，单用户隐式为 `local`。
- **服务端模式**：
  - 登录：用户名+密码 → 颁发短期 JWT；或直接使用 PAT。
  - CLI 与 MCP 客户端统一使用 `Authorization: Bearer <token>`。
  - 行级权限：所有任务查询自动叠加 `workspace_id IN (用户可见集合)`。

### 1.3 Workspace 与 Project 的关系

- **Workspace** = 数据隔离边界（≈ 团队、公司、个人）。
- **Project** = 任务属性，Taskwarrior 支持点号分层 `work.client.acme` `[19]`，我们继承该层级表示法，但 project 字符串只在所属 workspace 内有意义。

---

## 2. 数据模型（Task 核心 Schema）

> 字段语义严格对齐 Taskwarrior RFC `[15]` 与官方 Task 表示文档 `[4]`。

### 2.1 内置属性

| 字段 | 类型 | 来源 / 备注 |
|---|---|---|
| `uuid` | UUID v4 | 永久唯一 ID `[7]` |
| `id` | int (派生) | working set 中的行号，可与 UUID 互换使用 `[7]` |
| `description` | string | 必填；UTF-8；不允许换行 `[15]` |
| `status` | enum | `pending`(P) / `completed`(C) / `deleted`(D) / `recurring`(R) / `waiting`(W) `[4][15]` |
| `entry` | timestamp | 创建时间 |
| `modified` | timestamp | 最近修改 |
| `start` | timestamp | 设置后任务进入 active 状态，urgency 提升 `[7]` |
| `end` | timestamp | 完成或删除时写入 `[7]` |
| `due` | timestamp | 临近（默认 7 天内）视为 due，超期视为 overdue `[7]` |
| `wait` | timestamp | 隐藏 pending；到期客户端必须自动清空 `wait` 并改为 pending `[15]` |
| `scheduled` | timestamp | 过 `scheduled` 后任务为 ready `[15]` |
| `until` | timestamp | 到期任务自动消失 `[7]` |
| `project` | string | 支持 `a.b.c` 点号层级 `[19]` |
| `tags` | []string | 标签数组；`+tag` / `-tag` 修改语法 `[8]` |
| `priority` | enum | 默认 `H/M/L/<空>`，本质上是内置 UDA `[5][12]` |
| `depends` | []UUID | 依赖列表 `[11]` |
| `annotations` | []{entry,description} | 每条注释含时间戳与文本 `[8]` |
| `recur` | string | 周期：`daily/weekly/3days/monthly/...` |
| `parent` / `mask` / `imask` | UUID / string / int | 循环任务父子关系 `[15]` |
| `urgency` | float (派生) | 不持久化，按 §6 公式计算 |

### 2.2 多租户扩展字段（新增）

`workspace_id`(FK) · `creator_user_id`(FK) · `assignee_user_id`(FK, nullable) · `followers`([]user_id) · `external_refs`(JSON, e.g. `{"github":"owner/repo#123"}`)。

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

### 3.2 必须实现的命令（M0–M2 范围）

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
| `import` / `export` | JSON 互导（兼容 Taskwarrior 格式） |
| `config` / `show` | 配置读写 |
| `context` | 设置默认过滤 `[22]` |
| `calc` | 表达式求值（见 §3.5） |
| `calendar` | 日历视图 |
| `burndown.daily` / `.weekly` / `.monthly` | 燃尽图 |
| `sync` | 同步（服务端模式） |

### 3.3 Helper（脚本可解析）

以下划线开头的命令输出**无装饰**，方便脚本与 shell 补全 `[8]`：

`_get` · `_ids` · `_uuids` · `_projects` · `_tags` · `_udas` · `_unique` · `_urgency` · `_show` · `_version` · `_zshcommands` · `_zshattributes` · `_zshids`。

### 3.4 过滤表达式（解析器需求）

支持示例（全部来自上游）`[8][19]`：

- 属性匹配：`project:work` · `+urgent` · `-waiting` · `due:today` · `due.before:tomorrow` · `due.after:2days`
- 日期关键字：`today` / `tomorrow` / `eow` / `eom` / `sod` / `eod` / `<N>days` 等
- 文本与正则：`/pattern/`，受 `rc.search.case.sensitive` 控制
- 布尔代数：`and` / `or` / `xor` / `not`，括号转义：`\( ... \)` `[8]`
- 字符串引号：`project:'Home & Garden'`
- 状态：`pending` / `completed` / `deleted` / `waiting` / `recurring` `[19]`

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
- 切换：`taskg context use sprint-23` / `taskg context none`。

---

## 5. 注释、依赖、循环

- **Annotations**：复合结构 `{entry, description}` `[8]`；报表中支持 `description.count` 仅显示注释数 `[3]`。
- **Dependencies**：`task <id> modify depends:<uuid>` `[11]`；衍生 `blocked` / `blocking` 报表；进入 §6 urgency 计算。
- **Recurring**：父任务对用户隐藏，子任务通过 `parent` + `mask`/`imask` 关联 `[15]`；字段 `recur`（周期）与 `until`（终止）。

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

- `task _get 12.description` · `task _get 12.entry 12.modified`（多字段）
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
2. **服务端 Webhook**：HTTP POST 到 URL；超时与重试策略可配；用于飞书通知、CI 触发等。
3. **内嵌处理器**：Go plugin / WASM 插件（M5+），用于沙箱化的扩展。

---

## 9. 存储与同步

### 9.1 存储

- 引擎：**SQLite + WAL 模式**（modernc.org/sqlite，纯 Go）。
- 表结构由 §附录 A DDL 描述；每个表均带 `workspace_id` 用于行级隔离。
- 迁移：`golang-migrate` 或 `goose`（modernc 驱动）；版本化、可回滚。

### 9.2 同步

- 不复刻 taskserver/taskd（3.0 已抛弃）`[6][21]`。
- 同步模型：**operation-log 复制**。
  - 每次写入产出一条不可变 op：`(op_id, replica_id, parent_op_id, uuid, key, old, new, ts)`。
  - 多端通过比较 op-log 收敛，避免读-改-写丢失 `[4]`。
- 服务端 = 权威 op-log；客户端可离线累计 op，重连后批量推送。
- 兼容性：保留 `task export` / `task import` 的 JSON 字段名与上游一致 `[4]`，确保从 Taskwarrior 平滑迁移。

### 9.3 备份

- `taskg backup` → 复制 SQLite 文件（VACUUM INTO）+ 导出 JSON 双格式；管理员可在服务端定时任务。

---

## 10. 配置系统

### 10.1 兼容上游 `[13]`

- `.taskrc` 简单 `name = value` 语法；支持 `include`。
- 环境变量优先级：`TASKDATA` > `TASKRC` > `XDG_CONFIG_HOME`；命令行 `rc.x=y` 覆盖文件。

### 10.2 三层合并（新增）

1. **服务端默认**（管理员）
2. **Workspace 级**（团队约定，存 DB）
3. **用户级**（个人偏好，文件 + DB）

CLI 启动时按 1→2→3 顺序合并；`rc.x=y` 临时覆盖最高优先级。

### 10.3 文件格式

- 默认采用 **TOML**（Go 友好），同时保留对 Taskwarrior `.taskrc` 文本格式的**只读**导入能力。

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
│ Storage Layer (SQLite, modernc)      │
└──────────────────────────────────────┘
```

> 设计上**核心服务不依赖传输层**；CLI 本地模式可直接 in-process 调用 Core，无需启动 HTTP/MCP。

### 11.2 MCP 工具暴露

| Tool | 描述 | 关键参数 |
|---|---|---|
| `task.add` | 添加任务 | `workspace_id, description, project?, tags?, due?, ...` |
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
| `workspace.list` / `workspace.switch` | 工作区 | — |
| `context.set` / `context.show` | 上下文 | — |
| `config.get` / `config.set` | 配置 | — |

每个工具：**返回 `{data: 结构化JSON, rendered: 文本}`**，便于 Agent 解析又便于人读。

### 11.3 鉴权与隔离

- MCP 客户端连接需带 PAT；token 决定可见 workspace。
- 数据库层强制注入 `workspace_id`；推荐用 SQL view + 触发器或仓储层守卫两种手段双重校验。

---

## 12. CLI 工程要求

- 二进制名：`taskg`。
- 本地模式：`taskg add ...`（直连 SQLite）。
- 远程模式：`taskg --server https://... --token ... add ...`。
- 全命令支持 `--json` 输出（脚本化）。
- 提供 shell 补全：`taskg completion zsh|bash|fish|powershell`，利用 `_xxx` helper 命令。
- 所有命令必须 100% 可脚本化，stderr/stdout 严格分离。

---

## 13. 飞书 / 外部触发集成（私有云场景）

> 不在 Taskwarrior 上游范围内，列出以便后续展开。

- 触发源：飞书事件、webhook、定时（heartbeat）、一次性触发。
- 触发动作：Agent 收到事件后经 MCP 调 `task.add` / `task.query`，结果回写。
- 输出策略：飞书消息卡片 / 静默入库。
- 记忆机制：每 workspace 挂载「个人偏好 + 团队记忆」，hook 自动维护。

---

## 14. 兼容性目标

1. **JSON 互通**：`task export | taskg import` 与反向均无损 `[4]`。
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
| **M2** | UDA + Annotations + Dependencies + Recurring |
| **M3** | 多用户多 workspace + 鉴权（PAT/JWT）+ 行级隔离 + HTTP API |
| **M4** | MCP Server（stdio + Streamable HTTP）+ Webhook + 飞书触发示例 |
| **M5** | Op-log 同步引擎 + Hook（脚本 / Webhook / WASM）|
| **M6** | Taskwarrior JSON 双向导入导出 + `.taskrc` 兼容读 |

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
  UNIQUE(owner_user_id, slug)
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
  hashed_token    TEXT NOT NULL UNIQUE,
  scopes_json     TEXT NOT NULL DEFAULT '[]',
  workspace_scope TEXT,                  -- NULL = 全部
  expires_at      INTEGER,
  created_at      INTEGER NOT NULL
);

-- 任务核心（物化态；权威来自 operations）
CREATE TABLE tasks (
  uuid          TEXT PRIMARY KEY,
  workspace_id  TEXT NOT NULL REFERENCES workspaces(id),
  description   TEXT NOT NULL,
  status        TEXT NOT NULL,            -- pending/completed/deleted/waiting/recurring
  entry         INTEGER NOT NULL,
  modified      INTEGER NOT NULL,
  start_ts      INTEGER,
  end_ts        INTEGER,
  due           INTEGER,
  wait          INTEGER,
  scheduled     INTEGER,
  until         INTEGER,
  project       TEXT,
  priority      TEXT,                     -- H/M/L/NULL
  recur         TEXT,
  parent_uuid   TEXT,
  mask          TEXT,
  imask         INTEGER,
  creator_user_id  TEXT REFERENCES users(id),
  assignee_user_id TEXT REFERENCES users(id),
  external_refs_json TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX idx_tasks_ws_status ON tasks(workspace_id, status);
CREATE INDEX idx_tasks_ws_project ON tasks(workspace_id, project);
CREATE INDEX idx_tasks_ws_due ON tasks(workspace_id, due);

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
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  task_uuid TEXT NOT NULL REFERENCES tasks(uuid) ON DELETE CASCADE,
  entry     INTEGER NOT NULL,
  description TEXT NOT NULL
);

CREATE TABLE task_udas (
  task_uuid TEXT NOT NULL REFERENCES tasks(uuid) ON DELETE CASCADE,
  key       TEXT NOT NULL,
  value     TEXT,                          -- 统一以字符串存，类型由 schema 解释
  PRIMARY KEY (task_uuid, key)
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
  scope         TEXT NOT NULL,             -- server/workspace/user
  scope_id      TEXT,                      -- NULL for server
  key           TEXT NOT NULL,
  value         TEXT NOT NULL,
  PRIMARY KEY (scope, scope_id, key)
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
  action        TEXT NOT NULL,
  target_uuid   TEXT,
  payload_json  TEXT,
  created_at    INTEGER NOT NULL
);
```

> 注：`operations` 是权威表，`tasks` 与衍生表是物化视图。可用触发器或服务层在写入 op 时同步更新物化态；亦可只在物化态写入并后台异步生成 op-log（在 M0–M2 简化路径下可接受）。

---

## 附录 B：CLI 命令分级清单

- **M0**：`add` `modify` `done` `delete` `info` `list` `next` `config` `show` `import` `export`
- **M1**：`all` `completed` `waiting` `active` `ready` `overdue` `blocked` `blocking` `calendar` `calc` `context` `_get` `_ids` `_uuids` `_projects` `_tags`
- **M2**：`annotate` `denotate` `append` `prepend` `edit` `start` `stop` `recur` `_udas` `_unique` `_urgency`
- **M3+**：`user` `workspace` `member` `token` `sync` `burndown.*` `_show` `_version` `_zsh*`

---

## 附录 C：MCP 工具 JSON Schema 关键字段（草案）

> 完整 JSON Schema 文件后续放 `mcp/schemas/*.json`。

```jsonc
// task.add
{
  "type": "object",
  "required": ["workspace_id", "description"],
  "properties": {
    "workspace_id": {"type": "string", "format": "uuid"},
    "description":  {"type": "string", "minLength": 1},
    "project":      {"type": "string"},
    "tags":         {"type": "array", "items": {"type": "string"}},
    "priority":     {"type": "string", "enum": ["H","M","L"]},
    "due":          {"type": "string"},      // 接受 ISO8601 或 Taskwarrior 关键字
    "scheduled":    {"type": "string"},
    "wait":         {"type": "string"},
    "depends":      {"type": "array", "items": {"type":"string","format":"uuid"}},
    "udas":         {"type": "object", "additionalProperties": {"type":"string"}}
  }
}
```

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

