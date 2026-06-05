# xuanchu M4 设计规格

> **给 agentic workers 的要求：** 编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

**目标：** 在不引入 HTTP 服务端、远程 CLI、PAT/JWT 或 MCP 的前提下，把 `xuanchu` 从隐式单用户单 workspace 升级为本地多用户、多 workspace 的团队模型，并在 app service 层建立真实权限边界。

**范围策略：** M4 是“本地多 workspace 与权限骨架”里程碑。它必须让本地 CLI 真实使用 `actor_user_id + workspace_id` 上下文，但不能提前实现网络鉴权或服务端运行模式。后续 M5 只应替换 actor 来源，而不是重写任务、context、UDA、workspace、member 的业务边界。

**需求来源：** 本规格从 [ROADMAP.md](/Users/mac/code/projects/dajee/task/ROADMAP.md)、[README.md](/Users/mac/code/projects/dajee/task/README.md)、[docs/requirements.md](/Users/mac/code/projects/dajee/task/docs/requirements.md)、M3 当前实现和本轮 M4 需求讨论中收束。

---

## 1. 当前基础

M3 已完成。当前项目已有：

- Go 1.22 module。
- Cobra CLI。
- GORM + `github.com/glebarez/sqlite`，保持零 CGO。
- SQLite schema 中已有 `workspaces` 表，但只有隐式 `local` workspace。
- 任务表已有 `workspace_id`，任务仓储大多显式接收 workspace ID。
- `contexts` 和 UDA schema/value 已带 `workspace_id`。
- app service 内部仍缓存单一 workspace ID，并依赖 `store.LocalWorkspace()` 初始化。
- M3 已有配置系统、active context、UDA、`.taskrc` 只读导入、helper 和 completion。
- 现有 CLI 默认面向单个本地用户、单个 active workspace。

M4 的主要工作不是重写任务模型，而是把现有隐式 workspace 假设升级成显式运行时上下文：

```text
actor user + selected workspace + role permission
```

## 2. M4 产品体验

M4 完成后，应支持这些使用方式：

```bash
# 本地用户
xuanchu user list
xuanchu user add alice email:alice@example.test
xuanchu user use alice
xuanchu user info

# workspace
xuanchu workspace list
xuanchu workspace add work name:"Work"
xuanchu workspace use work
xuanchu workspace info
xuanchu workspace archive old

# 临时 workspace 覆盖
xuanchu --workspace work add "Write API spec" project:xuanchu
xuanchu --workspace personal list

# membership
xuanchu member list
xuanchu member add bob role:viewer
xuanchu member role bob member

# 权限真实生效
xuanchu user use bob
xuanchu --workspace work list
xuanchu --workspace work add "This fails if bob is viewer"

# 审计
xuanchu audit list
xuanchu audit list --json
```

M4 仍需保持脚本友好：

- stdout 只输出结果。
- stderr 输出错误。
- `--json` 输出稳定结构。
- helper 命令无装饰、每行一个值。
- 现有 M0-M3 命令在默认迁移后的 `local` user 和 `local` workspace 中继续可用。

## 3. 分层范围

### Layer 1：本地用户模型

必须进入 M4：

- 新增 `users` 表。
- 自动创建默认 `local` user。
- 支持 active user 持久切换。
- 支持最小 user 命令：
  - `user list`
  - `user add <name> [email:<email>]`
  - `user use <name|email|uuid>`
  - `user info [name|email|uuid]`
- `user add` 必须自动创建同名 personal workspace，并把该 workspace 设为新用户 default workspace。
- app service 初始化时必须解析 actor user。

不进入 M4：

- 密码。
- 登录。
- user delete。
- user rename。
- 完整个人资料管理。

### Layer 2：多 workspace 与 active workspace

必须进入 M4：

- 扩展 `workspaces` 表。
- 支持 active workspace 持久切换。
- 支持全局 `--workspace <slug|uuid>` 单次覆盖。
- workspace slug 全局唯一。
- 支持 workspace 命令：
  - `workspace list [--all]`
  - `workspace add <slug> [name:<name>] [description:<text>] [visibility:private|team|public]`
  - `workspace use <slug|uuid>`
  - `workspace info [slug|uuid]`
  - `workspace modify <slug|uuid> [name:<name>] [description:<text>] [visibility:private|team|public]`
  - `workspace archive <slug|uuid>`
- 归档 workspace 后不可作为 active workspace，默认列表和默认查询不可进入。

不进入 M4：

- workspace 硬删除。
- 跨 workspace/global context 查询。
- workspace 导入导出搬迁工具。

### Layer 3：membership 与权限边界

必须进入 M4：

- 新增 `memberships` 表。
- 支持角色：
  - `owner`
  - `admin`
  - `member`
  - `viewer`
- 支持 member 命令：
  - `member list [--workspace <slug|uuid>]`
  - `member add <user> [role:member] [--workspace <slug|uuid>]`
  - `member role <user> <owner|admin|member|viewer> [--workspace <slug|uuid>]`
- 在 app service 层执行权限校验。
- repository 保持数据访问职责，不包含角色业务判断。

不进入 M4：

- 邀请流程。
- 成员删除。
- owner 转移专门命令。
- 网络鉴权。

### Layer 4：workspace 隔离贯通

必须进入 M4：

- 任务读写显式绑定 actor 和 workspace。
- 报表显式绑定 workspace。
- helper 显式绑定 workspace：
  - `_ids`
  - `_uuids`
  - `_projects`
  - `_tags`
  - `_unique`
  - `_udas`
  - `_get`
  - `_urgency`
- context 和 UDA 按 workspace 隔离。
- active context 是 `(user_id, workspace_id)` 二维状态。
- import/export 只作用于当前 workspace 或 `--workspace` 指定 workspace。
- 数字 working-set ID 按 workspace 独立计算。

### Layer 5：audit log

必须进入 M4：

- 新增 `audit_logs` 表。
- 记录本地关键写操作。
- audit 范围内的写操作必须在同一个 store-level 事务内同时写业务表和 audit log。
- 支持只读查看命令：
  - `audit list [--workspace <slug|uuid>]`
  - `audit list --json`

不进入 M4：

- audit log 回滚。
- audit log 作为同步来源。
- 复杂 audit filter DSL。

## 4. 数据模型

### users

M4 推荐模型：

```text
id TEXT PRIMARY KEY
name TEXT NOT NULL
email TEXT UNIQUE
default_workspace_id TEXT
created_at INTEGER NOT NULL
modified_at INTEGER NOT NULL
```

规则：

- `id` 使用 UUID v4。
- `name` 本地唯一，便于 CLI 使用；大小写是否敏感由 spec plan 明确，建议先大小写敏感，保持实现简单。
- `email` 可为空；非空时唯一。SQLite 允许 `UNIQUE` 列中存在多个 `NULL`，M4 依赖这个行为。
- `default_workspace_id` 指向该用户默认 workspace。
- 迁移创建的 `local` user 的 `email` 必须为 `NULL`，不要写入假造邮箱。
- 密码字段不进入 M4。

### workspaces

在现有 `workspaces` 基础上扩展：

```text
id TEXT PRIMARY KEY
slug TEXT NOT NULL
name TEXT NOT NULL
created_by_user_id TEXT
description TEXT
visibility TEXT NOT NULL DEFAULT 'private'
settings_json TEXT NOT NULL DEFAULT '{}'
archived_at INTEGER
created_at INTEGER NOT NULL
modified_at INTEGER NOT NULL
```

规则：

- `slug` 是 CLI 主要引用方式。
- workspace slug 建议全局唯一，避免本地 CLI 中同名 workspace 因 owner 不同而产生歧义。
- workspace slug 必须匹配 `^[a-z0-9][a-z0-9_-]*$`，建议长度上限 64。
- workspace slug 保留字至少包括 `all`、`none`、`current`。
- `local` 是迁移产物的合法 slug，不是保留字；不能新建第二个 `local` 由 slug 全局唯一约束自然保证。
- `visibility` 支持 `private/team/public`，M4 只保存和展示，不做 public discovery。
- `archived_at != NULL` 表示归档。
- `created_by_user_id` 只表示创建者，不参与权限判定。
- 当前 owner 完全由 `memberships.role = owner` 决定。

### memberships

```text
user_id TEXT NOT NULL
workspace_id TEXT NOT NULL
role TEXT NOT NULL
joined_at INTEGER NOT NULL
modified_at INTEGER NOT NULL
PRIMARY KEY (user_id, workspace_id)
```

规则：

- `role` 只能是 `owner/admin/member/viewer`。
- 每个非归档 workspace 至少保留一个 owner。
- `workspace add` 自动给当前 actor 创建 owner membership。
- `member role` 可以把其他用户提升为 owner。
- `member role` 禁止把最后一个 owner 降级。
- `joined_at` 表示成员首次加入时间；`modified_at` 表示最近角色修改时间。

### audit_logs

```text
id INTEGER PRIMARY KEY AUTOINCREMENT
actor_user_id TEXT
workspace_id TEXT
action TEXT NOT NULL
target_type TEXT
target_id TEXT
payload_json TEXT
created_at INTEGER NOT NULL
```

规则：

- `actor_user_id` 在本地模式通常存在；为兼容迁移和系统动作，可允许为空。
- `workspace_id` 对 user-level 动作可为空，例如 `user add`。
- `payload_json` 存储必要摘要，不存秘密。
- audit 写入失败应让对应写操作失败，避免用户误以为审计完整。
- 所有 audit 范围内的写操作必须在同一个 store-level 事务内同时写业务表和 `audit_logs`。禁止先提交业务表、再尝试补写 audit。
- 建议为常用查询建立复合索引：`(workspace_id, created_at DESC, id DESC)`。

### api_tokens

M4 不启用 token 行为。可以在迁移中建立占位表，也可以留到 M5。

建议：M4 不创建 `api_tokens` 表，避免产生“已有 token 模型但无法使用”的误导。M5 在设计 PAT/JWT 时一次性定义 hash、scope、过期和创建命令。

## 5. 迁移与兼容

M4 必须向前兼容 M0-M3 数据库。

迁移规则：

- 打开老数据库时自动创建 `local` user。
- 若现有 `local` workspace 没有 owner，则绑定到 `local` user。
- 为 `local` user 和 `local` workspace 创建 owner membership。
- 为 `local` user 设置 default workspace。
- 迁移创建的 `local` user 不设置 email。
- 如果已有 `contexts`、UDA schema/value、tasks，它们继续使用原 `workspace_id`。
- 不修改任务 UUID、entry、modified、working-set 可见语义。
- 所有迁移必须幂等，重复打开数据库不重复创建 user/workspace/member。
- M4 必须保证每个 active/default workspace 都是未归档 workspace。

兼容体验：

- 用户升级后直接运行 `xuanchu list`，行为应与 M3 一致。
- 用户没有创建额外 workspace 前，现有命令仍默认使用 `local` workspace。
- 旧配置中的 `context.active` 状态迁移为 `active_context.<local_user_id>.<local_workspace_id>`。
- 迁移后必须删除老的 `context.active` meta key；M4 不再支持读取或写入该 key。

## 6. 运行时上下文

M4 引入统一运行时上下文：

```text
ActorUserID
WorkspaceID
WorkspaceSlug
Role
NoContext
Now
```

解析顺序：

1. CLI root 解析数据库路径、TOML、SQLite meta、`rc.*`。
2. 打开 store，完成迁移和默认 user/workspace 初始化。
3. 解析 active user。
4. 根据 `--workspace` 或 active/default workspace 解析 workspace。
5. 校验 actor 是否是该 workspace member。
6. 读取 actor 在 workspace 中的 role。
7. 初始化 app service。

active user：

- `user use` 持久写入 `active_user_id`。
- 如果 `active_user_id` 不存在，默认使用 `local` user。
- 如果 active user 被外部破坏性删除，CLI 应给出明确错误；M4 内部不提供 user delete。

active workspace：

- `workspace use` 持久写入当前 actor 的 active workspace。
- 建议使用 meta key：`active_workspace.<user_id>`。
- 如果用户没有 active workspace，则使用 `users.default_workspace_id`。
- `--workspace` 只影响本次运行，不写 meta。
- active workspace 必须是当前 actor 可见且未归档 workspace。

active context：

- active context 是 `(user_id, workspace_id)` 二维状态。
- 建议使用 meta key：`active_context.<user_id>.<workspace_id>`。
- `context use`、`context none` 只影响当前 actor 在当前 workspace 中的 active context。
- `--no-context` 和 `rc.context=none` 继续只影响本次运行。
- 禁止 workspace 级共享 active context，避免 Alice 切换 context 后影响 Bob。

## 7. 权限矩阵

M4 的权限必须真实生效。

| 能力 | viewer | member | admin | owner |
|---|---:|---:|---:|---:|
| 读取任务、报表、helper | 是 | 是 | 是 | 是 |
| 添加、修改、完成、删除任务 | 否 | 是 | 是 | 是 |
| import 任务 | 否 | 是 | 是 | 是 |
| export 任务 | 是 | 是 | 是 | 是 |
| 切换自己的 active context | 是 | 是 | 是 | 是 |
| 定义/删除 workspace context | 否 | 是 | 是 | 是 |
| 读取 UDA schema/value | 是 | 是 | 是 | 是 |
| 修改 UDA schema | 否 | 否 | 是 | 是 |
| 修改 workspace metadata | 否 | 否 | 是 | 是 |
| 添加成员、调整非 owner 角色 | 否 | 否 | 是 | 是 |
| 提升其他用户为 owner | 否 | 否 | 否 | 是 |
| 降级 owner | 否 | 否 | 否 | 是，且不能降级最后一个 owner |
| 归档 workspace | 否 | 否 | 否 | 是 |
| 查看 audit log | 否 | 否 | 是 | 是 |

说明：

- M4 不做成员删除。若必须撤销访问，应通过 `member role <user> viewer` 临时降权，成员删除留给后续 spec。
- `context use/none` 修改当前 actor 在当前 workspace 的 active context，viewer 可以执行。
- `context define/delete` 修改 workspace 共享 context，viewer 不可执行。
- 所有权限错误使用稳定错误语义，human 输出到 stderr，JSON 输出结构化错误。

## 8. CLI 命令细节

### user list

```bash
xuanchu user list
xuanchu user list --json
```

human 输出建议列：

```text
ACTIVE  NAME   EMAIL               DEFAULT
*       local                      local
```

JSON 字段：

```json
{
  "id": "uuid",
  "name": "local",
  "email": null,
  "default_workspace_id": "uuid",
  "active": true,
  "created_at": "..."
}
```

### user add

```bash
xuanchu user add alice email:alice@example.test
```

规则：

- `name` 必填。
- `email:<email>` 可选。
- 新用户默认不自动加入所有 workspace。
- 新用户必须自动获得自己的 personal workspace。
- personal workspace 默认 slug 为 user name，name 也默认等于 user name。
- personal workspace 默认 `visibility:private`。
- 如果 personal workspace slug 冲突，`user add` 返回错误；plan 可增加 `workspace:<slug>` 参数解决冲突，但不能创建一个没有 default workspace 的 user。
- 新 user 对 personal workspace 自动拥有 owner membership。

### user use

```bash
xuanchu user use alice
```

规则：

- 切换 active user。
- 如果该 user 有 active workspace，使用该 workspace。
- 否则使用 default workspace。
- 如果 default workspace 已归档或不可见，报错并提示先修复 default workspace 或创建新 workspace。

### workspace list

```bash
xuanchu workspace list
xuanchu workspace list --all
xuanchu workspace list --json
```

规则：

- 默认只显示当前 actor 可见且未归档 workspace。
- `--all` 显示归档 workspace。
- human 输出显示 active 标记、slug、name、role、archived。

### workspace add

```bash
xuanchu workspace add work name:"Work" description:"Team work" visibility:team
```

规则：

- `slug` 必填。
- `name` 默认等于 slug。
- 当前 actor 自动成为 owner。
- 如果 actor 没有 default workspace，可把新 workspace 设为 default。
- 是否自动切换到新 workspace 由 plan 决定；推荐不自动切换，避免脚本副作用。

### workspace use

```bash
xuanchu workspace use work
```

规则：

- 目标 workspace 必须未归档。
- actor 必须是 member。
- 持久写入当前 actor 的 active workspace。

### workspace modify

```bash
xuanchu workspace modify work name:"Work" description:"Team work" visibility:team
```

规则：

- 可修改字段为 `name`、`description`、`visibility`。
- 不允许修改 `slug`。
- admin 和 owner 可执行。
- 修改必须写入 audit action `workspace.modify`。

### workspace archive

```bash
xuanchu workspace archive old
```

规则：

- 只有 owner 可执行。
- 只设置 `archived_at`。
- 归档后不可 `workspace use`。
- 如果归档的是当前 active workspace，推荐自动切换到 actor 的第一个未归档 workspace；如果没有可切换 workspace，则拒绝归档。
- `local` workspace 可以归档，但必须满足上面的可切换条件。
- M4 必须保持不变量：每个 user 的 active/default workspace 都不能指向归档 workspace。
- 归档前必须扫描所有 `users WHERE default_workspace_id = <target_workspace_id>`。
- 对每个受影响 user，如果该 user 还有其他未归档 membership workspace，则选择 slug 排序最小的 workspace 写回 `default_workspace_id`。
- 如果任一受影响 user 没有可替代的未归档 workspace，则整个归档操作失败。
- active workspace 同样必须被重新分配到该 actor 可见的未归档 workspace；没有可替代 workspace 时拒绝归档。

### member list

```bash
xuanchu member list
xuanchu member list --workspace work
xuanchu member list --json
```

规则：

- 默认使用当前 workspace。
- viewer 可以读取 member list。
- member list 可以显示成员 name、email、role、joined_at；M4 是本地工具，email 不做额外脱敏。

### member add

```bash
xuanchu member add alice
xuanchu member add alice role:viewer
```

规则：

- 默认 role 为 `member`。
- admin 可添加 `viewer/member/admin`。
- owner 可添加任意角色，包括 owner。
- 目标 user 必须已存在。
- `<user>` 解析应复用统一 user resolver，支持 name、email、UUID。

### member role

```bash
xuanchu member role alice viewer
xuanchu member role bob owner
```

规则：

- admin 不能授予或降级 owner。
- owner 可以授予 owner。
- owner 不能把最后一个 owner 降级。
- M4 不提供 member delete。

### audit list

```bash
xuanchu audit list
xuanchu audit list --workspace work
xuanchu audit list --json
```

规则：

- 默认查看当前 workspace 的 audit。
- 需要 admin 或 owner。
- 默认按 `created_at DESC, id DESC` 排序。
- human 输出只显示最近记录；必须支持 `--limit`，默认 50。
- `--since <date>` 可在 plan 阶段决定是否进入 M4；不是 spec 必须项。
- JSON 输出稳定字段：
  - `id`
  - `actor_user_id`
  - `workspace_id`
  - `action`
  - `target_type`
  - `target_id`
  - `payload`
  - `created_at`

## 9. Workspace 隔离细节

### 任务

- `add` 使用当前 workspace ID 写入任务。
- `modify/done/delete/start/stop/annotate/denotate/append/prepend/edit/info` 只能解析当前 workspace 内的 UUID 或 working-set ID。
- dependencies 只能引用当前 workspace 内任务。
- recurring parent/child 只能在同一 workspace。
- import 只导入到当前 workspace。
- export 只导出当前 workspace。
- export 是只读操作，不写 audit。

### 报表和 helper

- 所有报表自动限定当前 workspace。
- helper 命令尊重全局 `--workspace <slug|uuid>` 覆盖。
- `_ids` 和 `_uuids` 只返回当前 workspace。
- `_projects` 和 `_tags` 只聚合当前 workspace。
- `_unique <attr>` 只聚合当前 workspace。
- `_get <target.field>` 只解析当前 workspace。

### Context

- context 定义按 workspace 隔离。
- active context 按 `(user_id, workspace_id)` 隔离。
- Alice 在某 workspace 中执行 `context use`，不得影响 Bob 在同一 workspace 的 active context。
- 切换 workspace 后，读取当前 actor 在新 workspace 中的 active context。
- `--no-context` 继续只影响本次运行。

### UDA

- UDA schema 按 workspace 隔离。
- 同名 UDA 在不同 workspace 可以有不同类型或枚举。
- UDA query 编译必须使用当前 workspace 的 schema。
- orphan UDA 保留规则不变，但只能在所属 workspace 中导出。

### Config

M4 不引入完整 server/workspace/user 三层配置系统。只增加必要 active 状态：

- `active_user_id`
- `active_workspace.<user_id>`
- `active_context.<user_id>.<workspace_id>`

普通配置继续沿用 M3 合并规则。workspace 级配置体系可在 M5/M7 结合服务端和 report DSL 再完整设计。

## 10. Audit 记录范围

M4 必须记录这些动作：

- task:
  - `task.add`
  - `task.modify`
  - `task.done`
  - `task.delete`
  - `task.start`
  - `task.stop`
  - `task.annotate`
  - `task.denotate`
  - `task.append`
  - `task.prepend`
  - `task.edit`
  - `task.import`
- context:
  - `context.define`
  - `context.delete`
  - `context.use`
  - `context.none`
- UDA:
  - `uda.schema.set`
  - `uda.schema.delete`
- workspace:
  - `workspace.add`
  - `workspace.use`
  - `workspace.modify`
  - `workspace.archive`
- member:
  - `member.add`
  - `member.role`
- user:
  - `user.add`
  - `user.use`

审计 payload 只保留决策所需摘要，例如：

```json
{
  "description": "Write API spec",
  "changes": ["priority", "tags"]
}
```

不要在 audit payload 中保存 token、密码或未来可能出现的秘密字段。

## 11. App 与 Storage 边界

M4 必须保持分层清晰：

- `internal/cli` 只负责参数解析、命令路由、输出。
- `internal/app` 负责 actor/workspace 解析后的用例编排和权限校验。
- `internal/task` 不依赖 user、workspace、membership、GORM 或 Cobra。
- `internal/storage` 负责 GORM model、迁移、repo 查询，不负责解释角色权限。
- 新增 user/workspace/member/audit repo 应在 `internal/storage` 聚合。
- 后续 HTTP/MCP 必须复用 M4 app service。

建议新增 app 级概念：

```text
RuntimeContext
Actor
WorkspaceRef
Role
Permission
```

不要把 `local` workspace 或 `local` user 硬编码散落在 CLI 命令里。

M4 应倾向“每个命令执行前解析 runtime context，再创建绑定该 context 的 service”。这与 M5 HTTP 的“每个请求一个 actor/workspace 上下文”一致，避免把 service 设计成长期持有可变全局状态。

## 12. 错误语义

M4 需要稳定区分这些错误：

- user not found。
- workspace not found。
- workspace archived。
- membership not found。
- permission denied。
- cannot archive last workspace。
- cannot downgrade last owner。
- ambiguous target。

human 输出：

- 错误写 stderr。
- exit code 非 0。
- 信息简洁，不输出栈。

JSON 输出：

```json
{
  "error": {
    "code": "permission_denied",
    "message": "viewer cannot modify tasks"
  }
}
```

M4 不要求所有旧命令都统一 JSON 错误结构，但新增 user/workspace/member/audit 命令应按这个方向实现。

## 13. 非目标

M4 不做：

- HTTP server。
- 远程 CLI。
- PAT/JWT。
- `token` 命令。
- MCP Server。
- op-log sync。
- Hook。
- workspace 硬删除。
- user delete/password/rename。
- member delete。
- owner 转移专门命令。
- 跨 workspace/global context 查询。
- 完整 server/workspace/user 三层 config。
- Taskwarrior 自定义 report DSL。

## 14. M4 完成判定标准

M4 只有同时满足以下条件，才算真正完成：

- 任意业务命令初始化后都能解析出非空 `ActorUserID` 和 `WorkspaceID`，除非该命令明确是数据库初始化前的纯本地辅助命令，例如 `help`、`completion`、`_version`。
- app service 的业务路径不再依赖 `store.LocalWorkspace()` 作为默认 workspace 来源。
- 所有 task 写命令都经过权限检查，并在同一事务中写入业务表和 audit log。
- context active 状态按 `(user_id, workspace_id)` 隔离，不存在 workspace 级共享 active context。
- 两个 workspace 中同名 task/project/tag/UDA/context 的隔离由 storage 查询中的 workspace scope 保证，而不是靠 service 层事后过滤。
- 归档逻辑保持“每个 user 的 active/default workspace 均未归档”的不变量。
- viewer/member/admin/owner 的关键权限差异有单元测试和 CLI 集成测试覆盖。
- M5 可以用 HTTP 鉴权解析出的 actor 替换本地 active user，而不重写 task/context/UDA/workspace/member 的业务逻辑。

## 15. 测试要求

### 单元测试

必须覆盖：

- 默认 local user/workspace/member 迁移幂等。
- workspace active 解析和 `--workspace` 覆盖。
- active context 按 `(user_id, workspace_id)` 隔离。
- role permission matrix。
- owner 最后一人保护。
- archived workspace 不可 use。
- 归档时 default workspace 不指向归档 workspace。
- UDA schema 跨 workspace 隔离。
- context 跨 workspace 隔离。
- audit log 写入。
- audit 范围内写操作和 audit log 同事务提交或回滚。

### Storage 测试

重点覆盖：

- `users` CRUD。
- `workspaces` CRUD/归档。
- `memberships` role 更新。
- `audit_logs` append/list。
- 旧数据库打开后的自动迁移。
- 所有新增表在 `CGO_ENABLED=0` 下可迁移。

### CLI 集成测试

必须覆盖：

- 升级后默认 `xuanchu add/list` 行为不破坏。
- `workspace add/use/list/info/modify/archive`。
- `user add/use/list/info`。
- `member add/role/list`。
- viewer 写任务失败。
- member 管理成员失败。
- admin 归档失败。
- owner 归档成功。
- 两个 workspace 中同名任务、project、tag、UDA、context 不互相污染。
- working-set ID 在 workspace 切换后独立。
- helper 命令尊重 `--workspace`。
- `audit list --json` 能看到关键写操作。

### 必跑验证

每次声称 M4 完成前必须运行：

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```

如果改动影响 CLI 行为，还应重点运行：

```bash
go test ./tests/integration -run TestCLI -count=1
```

## 16. 文档同步

M4 完成后必须更新：

- `README.md`
  - 增加本地 user/workspace/member/audit 用法。
  - 说明 `--workspace`。
  - 说明 workspace 归档而非删除。
  - 说明 M4 不支持 member delete，viewer 仍可读取 workspace 数据。
- `ROADMAP.md`
  - 将 M4 标为已完成。
  - 下一步指向 M5 HTTP/JSON API 与远程 CLI。
- 本 spec
  - 如果 implementation plan 中调整范围，必须回写规格。

## 17. M5 衔接

M4 完成后，M5 应能直接复用：

- users/workspaces/memberships/audit_logs schema。
- app service 的 actor/workspace/role 上下文。
- 权限矩阵。
- workspace 隔离下的 task/context/UDA/report 行为。

M5 新增内容应集中在：

- HTTP server 生命周期。
- PAT/JWT 鉴权。
- `Authorization: Bearer <token>` 到 actor 的解析。
- 远程 CLI。
- OpenAPI。

如果 M5 需要大规模重写 M4 app service，说明 M4 的权限边界没有真正完成。
