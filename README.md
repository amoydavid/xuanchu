# taskg — 面向企业项目与 Agent MCP 的 Taskwarrior 风格任务运行时（Go 版）

`taskg` 是一个用 **纯 Go** 实现的企业任务运行时。它借鉴 Taskwarrior 的 CLI、查询语言、任务字段和 urgency 思路，但产品目标不是做完整 Taskwarrior clone，而是服务企业项目协作和 Agent MCP：

- 单一二进制：同时承担 **本地 CLI / 远程 CLI 客户端 / HTTP API 服务端 / MCP Server** 四种形态
- 数据库：**SQLite（GORM + `github.com/glebarez/sqlite`，零 CGO）**，可跨平台交叉编译
- `workspace` 作为企业 / 租户级隔离边界；`project` 表示企业内的真实项目
- 支持多用户、权限、审计、行级隔离，并为 Agent token 和 MCP scope 预留边界
- 借鉴 Taskwarrior 的核心命令名、JSON 迁移格式与 urgency 公式；企业能力优先于完全兼容

## 详细需求

参见 `docs/requirements.md`。该文档梳理了：

- 上游 Taskwarrior 的数据模型 / CLI 语法 / 报表 / Urgency / DOM / Hook / 同步语义
- 企业 workspace、project、Agent MCP、外部系统触发等扩展需求
- SQLite 表结构草案、CLI 命令分级、MCP 工具 JSON Schema 草案
- 所有结论附参考来源链接

## 状态

完整 milestone 拆解与当前进度见 [ROADMAP.md](./ROADMAP.md)。

## 快速开始

```bash
go build -o taskg ./cmd/taskg

# 先注册一个项目，再创建第一条任务
./taskg project add ai-agent-platform name:"AI Agent Platform"
./taskg add "Write MCP task docs" project:ai-agent-platform +docs due:tomorrow

# 默认只看 pending 任务
./taskg list

# 查看详情。这里的 1 是列表里的 working-set ID
./taskg info 1

# 修改、完成、删除都可以用 <target> <action> 写法
./taskg 1 modify priority:H +next
./taskg 1 done
./taskg 1 delete
```

`target` 可以是列表里的数字 ID、完整 UUID，或足够长的 UUID 前缀。常用命令既支持 `taskg <subcommand> ...`，也支持 Taskwarrior 风格的 `taskg <target> <action> ...`。

常用全局参数：

```bash
./taskg --db ./taskg.db list          # 使用指定 SQLite 文件
./taskg --data-dir ./data list        # 数据库放到 ./data/taskg.db
./taskg --json list                   # 输出 JSON
./taskg --no-color list               # 关闭颜色
./taskg --no-context list             # 本次命令忽略 active context
./taskg --workspace dajee list        # 本次命令切到 dajee workspace
```

默认数据库路径是 `~/.local/share/taskg/taskg.db`。配置优先级按“本次命令参数优先”理解即可：CLI flag / `rc.*` 覆盖 > 环境变量 > SQLite meta > `taskg.toml` > 默认值。

## 命令和参数怎么写

任务属性一般写成 `key:value`，标签写成 `+tag` 或 `-tag`：

```bash
./taskg add "Ship MCP API" project:ai-agent-platform priority:H +next due:friday
./taskg 1 modify project:ai-agent-platform priority:M +review -next
./taskg 1 modify due:                  # 清空 due
```

查询可以放在报表命令前，也可以放在报表命令后：

```bash
./taskg +next list
./taskg list +next
./taskg '(project:ai-agent-platform and +review) or priority:H' next
```

Shell 会吃掉括号、空格和 `+` 等字符，复杂查询建议加引号。`/text/` 是 description 子串匹配，不是正则。

脚本里建议优先使用这些稳定接口：

```bash
./taskg --json export
./taskg _ids +next
./taskg _uuids project:ai-agent-platform
./taskg _get 1.uuid 1.description 1.urgency
./taskg _show database.path active.user active.workspace active.context
```

## M0 本地 CLI 用法

```bash
go build -o taskg ./cmd/taskg

# 添加任务
./taskg add "Write project spec" project:ai-agent-platform +planning due:tomorrow
./taskg add "Review PR" priority:H +review

# 查看任务列表
./taskg list

# 查看任务详情
./taskg info 1

# 修改任务
./taskg 1 modify priority:H +next
./taskg 1 modify project:ai-agent-platform

# 完成任务
./taskg 1 done

# 删除任务
./taskg 1 delete

# 导出为 JSON
./taskg export

# 导入 JSON
./taskg import tasks.json

# 查看配置
./taskg show

# 设置配置
./taskg config set date.format rfc3339
./taskg config get date.format
```

默认数据库路径为 `~/.local/share/taskg/taskg.db`，可用 `--db` 或 `TASKG_DB` 环境变量覆盖。

## M1 查询与报表用法

```bash
# 布尔组合查询
./taskg '+next or due.before:tomorrow' list
./taskg '(project:ai-agent-platform and +urgent) or priority:H' list

# 报表命令
./taskg all
./taskg completed
./taskg deleted
./taskg overdue

# 查看任务 urgency（human 或 JSON）
./taskg urgency 1
./taskg urgency 1 --json
./taskg _urgency 1

# DOM helper
./taskg _get 1.description 1.uuid 1.urgency 1.tag.next
./taskg _ids +next
./taskg _uuids project:ai-agent-platform
./taskg _projects
./taskg _tags

# 表达式计算
./taskg calc '1 + 2 * 3'
```

报表名等价于 `(默认 filter) AND (用户 filter)`。要绕过默认 status 限制，使用 `all`。

### 日期与 deadline 语义

`due:` 和 `end:` 表达的是「某天截止/结束」，写入时会自动落在**当地时区的当天 `23:59:59`**：

```bash
./taskg add "deadline" due:2030-01-01
# due 实际存储为 2030-01-01 23:59:59（本地时区），而非 00:00:00
```

查询时日期等值也用自然日范围，例如 `due:2030-01-01` 等价于 `[2030-01-01 00:00:00, 2030-01-01 23:59:59]`，跨 DST 与时区也稳定。

### description 子串匹配

`description:spec`、`description:/spec/` 和裸 `/spec/` **语义一致**，都按子串匹配；`description:` 不走字面相等。

## M2 核心任务模型用法

```bash
# waiting / active / ready / blocked / blocking 报表
./taskg add "Call vendor" wait:tomorrow scheduled:eow until:eom
./taskg waiting
./taskg 1 modify wait:
./taskg 1 start
./taskg active
./taskg 1 stop

# 注释与描述编辑
./taskg 1 annotate "called, left voicemail"
./taskg _get 1.annotations
./taskg 1 denotate 1
./taskg 1 append "with examples"
./taskg 1 prepend "[draft]"
./taskg 1 edit

# 依赖与 blocked / blocking
./taskg add "Prepare API"
./taskg add "Write docs" depends:<uuid-or-id>
./taskg blocked
./taskg blocking

# 基础循环任务
./taskg add "Submit weekly report" recur:weekly due:2030-01-05 until:2030-02-01
./taskg list
./taskg 1 done
./taskg list
```

M2 当前已经补齐这些能力：

- 任务字段：`start`、`wait`、`scheduled`、`until`、`annotations`、`depends`、`recur`、`parent`、`mask`、`imask`
- 报表命令：`waiting`、`active`、`ready`、`blocked`、`blocking`
- 动作命令：`start`、`stop`、`annotate`、`denotate`、`append`、`prepend`、`edit`
- 查询 / DOM / urgency / JSON import-export 对上述字段的贯通支持
- 基础 recurring：`daily`、`weekly`、`monthly`、`<N>days`、`<N>weeks`、`<N>months`

CLI 表格里的 `ID` 是默认 working set ID，跨 `list` / `next` / `ready` / `blocked` 等报表稳定，与排序无关；隐藏的 waiting 任务仍可用该 ID 操作，所以如果前面有 waiting 任务，`list` 中第一条可见 pending 任务可能显示为 `2`。`completed` / `deleted` 等不在 working set 中的报表，ID 列显示为 `-`。

`edit` 会打开缩进 JSON，保存后执行校验；非法日期、非法 status、换行 annotation 等错误不会写回。

当前 recurring 的基础约束：

- recurring parent 使用 `status:recurring` 持久化，默认 human 报表隐藏
- child 在创建 parent 时立即生成，完成 child 后自动生成下一个 child
- `until` 会阻止生成超过截止时间的新 child
- recurring parent 不接受 `wait`、`scheduled`、`depends`，避免模板字段被静默丢弃
- `monthly` 目前直接沿用 Go `time.AddDate(0, n, 0)` 的月末滚动语义

## M3 配置、Context、UDA 与 `.taskrc` 用法

```bash
# TOML / config / rc 覆盖
./taskg show
./taskg config set date.format rfc3339
./taskg config list
./taskg rc.date.format=epoch list

# context
./taskg context define agent 'project:ai-agent-platform status:pending'
./taskg context use agent
./taskg list
./taskg --no-context list
./taskg context show
./taskg context none

# UDA
./taskg config set uda.estimate.type numeric
./taskg config set uda.estimate.label Estimate
./taskg config set uda.estimate.values 1,2,3,5,8
./taskg add "Implement API" estimate:3
./taskg estimate:3 list
./taskg _get 1.estimate
./taskg _udas
./taskg _unique estimate

# .taskrc 只读导入
./taskg config import-taskrc ~/.taskrc --dry-run --json
./taskg config import-taskrc ~/.taskrc

# completion 与脚本 helper
./taskg completion zsh > ~/.zfunc/_taskg
./taskg _show date.format active.user active.workspace active.context
./taskg _version
```

M3 新增 `~/.config/taskg/taskg.toml` 作为文件配置来源。TOML 使用标准解析器，支持普通 TOML 字符串、数组、dotted key 和多行字符串。一个最小示例：

```toml
[database]
path = "/Users/me/.local/share/taskg/taskg.db"

[display]
color = true

[date]
format = "rfc3339"
```

### `taskg.toml` 可配置项

`taskg.toml` 是**本机配置**：它描述这台机器如何启动和显示 taskg，不描述某个企业 workspace 的业务规则。TOML 的 key 会被展开成点分格式，例如 `[database] path = "..."` 等价于 `database.path = "..."`。

当前建议只把这些 key 写进 TOML：

| TOML 写法 | 展开后的 key | 值 | 说明 |
|---|---|---|---|
| `[database] path = "..."` | `database.path` | 文件路径 | SQLite 数据库路径。只在启动打开数据库前生效；运行后不能用 `config set database.path` 修改。 |
| `[display] color = true` | `color` | `true` / `false` | 是否启用 human 输出颜色。也可直接写 `color = true`。 |
| `[display] json = false` | `json` | `true` / `false` | 默认是否输出 JSON。CLI 的 `--json` 优先级更高。也可直接写 `json = false`。 |
| `[date] format = "rfc3339"` | `date.format` | `rfc3339` / `epoch` | `_show`、`config get` 和部分脚本输出使用的日期格式。 |

一个完整的本机配置例子：

```toml
[database]
path = "/Users/me/.local/share/taskg/taskg.db"

[display]
color = true
json = false

[date]
format = "rfc3339"
```

不要把这些业务配置长期写进 TOML：

- `uda.*`
- `urgency.uda.*`
- `context.<name>`
- 未来的 report、hook、agent memory、project defaults

这些配置属于 workspace，应写入 SQLite，由权限和 audit 管理：

```bash
./taskg --workspace dajee config set uda.estimate.type numeric
./taskg --workspace dajee config set uda.estimate.values 1,2,3,5,8
./taskg --workspace dajee config set urgency.uda.estimate.coefficient 1.5
./taskg --workspace dajee context define agent 'project:ai-agent-platform status:pending'
```

不要把这些状态或内部 key 当作 TOML 配置写入：

- `active.user`、`active.workspace`、`active.context`：只读状态输出，用 `user use`、`workspace use`、`context use/none` 修改。
- `context.active`：M3 兼容旧 key，但 M4 起不再作为运行时 active context 来源。
- `active_user_id`、`active_workspace.<user>`、`active_context.<user>.<workspace>`：内部 SQLite meta，只用于迁移和运行时状态。

想确认当前最终生效值，用：

```bash
./taskg config list
./taskg _show database.path color json date.format active.user active.workspace active.context
```

数据库路径仍按 `--db`、`TASKG_DB`、`--data-dir`、TOML、XDG data、home fallback 的顺序解析。显示类配置按 CLI flag、`rc.*`、环境变量、SQLite meta、TOML、默认值合并。workspace 业务配置以数据库中的 workspace 记录为准；TOML 只作为本机默认值或迁移辅助。

`rc.*` 只影响本次命令，适合脚本临时覆盖：

```bash
./taskg rc.date.format=epoch list
./taskg rc.json:on --workspace missing list
./taskg rc.context=none list
```

`context` 会自动叠加到 `list`、`next`、各类报表和 `_ids/_uuids/_projects/_tags/_unique`。`--no-context` 和 `rc.context=none` 只影响本次运行；`context none` 会持久化清空 active context。

UDA 支持 `string`、`numeric`、`date`、`duration` 四种类型。date UDA 写入为 RFC3339 UTC 字符串；查询 `estimate:3`、`reviewed:2026-05-28` 会结合当前 schema 编译。未定义的 JSON top-level 字段会作为 orphan UDA 保留并导出，普通 `modify` 不能修改 orphan UDA。

`.taskrc` 在 M3 中的作用是**迁移和兼容性导入**：taskg 只读解析它，把支持的 key 导入到 SQLite 配置、context 或 UDA schema，并生成 imported/skipped/unknown 报告。taskg 不会修改原 `.taskrc`，也不会把 `.taskrc` 当成每次运行的完整配置源。

当前 `.taskrc` 支持范围：

- 支持导入：`color`、`dateformat`、`context.<name>`、`uda.<name>.type/label/values/default`、`urgency.uda.*`
- 识别但跳过：`data.location`、`report.*`、`calendar.*`、`burndown.*`、`news.*`、`sync.*`、`hooks.*`
- 其它 key 进入 unknown 报告，不会让导入失败

其中 `data.location` 会被识别但不会导入，因为 M3 的数据库路径只在启动前通过 `--db`、`TASKG_DB`、`--data-dir` 或 TOML 决定。M3 还不支持完整 Taskwarrior `.taskrc` 语义，不导入自定义 report DSL，也不运行 hooks。

## M4 企业 Workspace、权限与审计基础

```bash
# user
./taskg user list
./taskg user add alice email:alice@example.test
./taskg user use alice
./taskg user info

# workspace。这里的 dajee 表示企业 / 租户级工作空间
./taskg workspace list
./taskg workspace add dajee name:Dajee visibility:team
./taskg workspace use dajee
./taskg workspace info dajee
./taskg workspace modify dajee description:"Dajee enterprise workspace"
./taskg workspace archive old

# project 表示 workspace 内的真实企业项目
./taskg --workspace dajee project add ai-agent-platform name:"AI Agent Platform"
./taskg --workspace dajee project add erp-rewrite name:"ERP Rewrite"
./taskg --workspace dajee add "Design MCP task.query schema" project:ai-agent-platform +mcp
./taskg --workspace dajee add "Migrate invoice workflow" project:erp-rewrite +migration

# 在指定 workspace 中执行一次命令
./taskg --workspace local list
./taskg --workspace dajee _projects

# member
./taskg member list
./taskg member add bob role:viewer
./taskg member role bob member

# audit
./taskg audit list
./taskg audit list --limit 20 --json
```

M4 新增了企业运行时基础：

- `user list/add/use/info`
- `workspace list/add/use/info/modify/archive`
- `member list/add/role`
- `audit list`
- 全局 `--workspace <slug|uuid>` 一次性切到指定 workspace 执行命令

在当前版本里，`workspace` 是企业 / 租户级隔离边界；`project` 已经是 workspace 内的一等实体。任务上仍保留 `project` 字符串字段做人类可读输出，但运行时写入、查询、权限、审计和后续 API/MCP scope 都以稳定 `project_id` 为准。

同一个 project slug 可以出现在不同 workspace 中。也就是说，`dajee/ai-agent-platform` 和 `partner/ai-agent-platform` 是两个不同项目；权限、token 和 MCP scope 必须以 `workspace + project` 或稳定 `project_id` 为准，不能把 slug 当全局唯一标识。

当前 CLI 的 `--workspace <slug|uuid>` 使用裸 workspace slug，所以 workspace slug 在同一个 taskg 实例内应保持唯一。project slug 只在当前 workspace 内解析：

```bash
./taskg --workspace dajee list project:ai-agent-platform
./taskg --workspace partner list project:ai-agent-platform
```

上面两条命令访问的是两个不同 workspace 里的同名 project。当前版本已经采用严格 project 注册：`taskg add ... project:<slug>` 和 `taskg 1 modify project:<slug>` 只能引用当前 workspace 内已存在、未归档的 project，不会运行时自动创建。脚本、远程 API 和 MCP 应优先保存 `project_id`。如果同时指定 `--workspace` 和 `project_id`，该 project 必须属于这个 workspace；否则命令会报错，避免把任务写进错误租户。

后续路线会基于已完成的 project 实体化继续往外开放协议层：

- M5：已完成。project 已是 workspace 内的一等对象，采用严格 project 注册，并明确 workspace/project 配置边界。
- M6：在稳定 project scope 上提供 HTTP/JSON API、远程 CLI 和 Agent token。
- M7：提供企业 Agent MCP Server，让 Agent 通过受权限约束的 tool 操作任务。
- M8：接入外部系统触发器和 adapter。飞书、GitHub、Jira、Slack 都只是 adapter 示例，taskg 核心仍是 workspace/project/task/权限/审计。

权限模型：

- `viewer` 可以读任务、报表、helper、member list，并能切换自己的 active context
- `member` 额外可以写任务、import、定义/删除 context
- `admin` 额外可以改 workspace metadata、管理非 owner 成员、查看 audit、管理 UDA schema
- `owner` 额外可以授予/降级 owner、归档 workspace

`config list` 和 `_show` 只暴露用户可用的 public key，例如 `active.user`、`active.workspace`、`active.context`、`uda.*`、`urgency.*`。`active_user_id`、`active_workspace.<user>`、`active_context.<user>.<workspace>` 是内部 meta，不作为 CLI 接口使用。

`audit list` 的 human 输出包含时间、actor、action、target type 和 target id。`--json` 会额外输出 `actor_user_id`、`actor_name`、`workspace_id`、`payload` 等字段，适合脚本处理。

当前仍有一个刻意保留的限制：M4 **没有 `member delete`**。如果要临时撤销写权限，可用 `member role <user> viewer`。同时，`viewer` 仍能读取该 workspace 的任务、project、tag、UDA 和成员信息。

### M4 升级说明

从 M3 升级到 M4 时，不需要手动执行迁移命令；第一次运行 `taskg` 会自动完成迁移。

迁移后的默认状态：

- 现有任务保留在 `local` workspace
- 自动创建 `local` user
- 自动创建 `local` workspace
- 自动创建 `local user -> local workspace` 的 owner membership
- 旧的 `context.active` SQLite meta 会迁移到 `(local user, local workspace)` 作用域的 active context
- `local` user 的 email 保持空值，不会伪造测试邮箱

注意：M4 **不再把 TOML `context.active` 当作运行时 active context 来源**。如果你之前只依赖 TOML 里的 active context，需要在升级后执行一次：

```bash
./taskg context use <name>
```

## M5 Project 实体与配置边界

M5 现在已经落地。最重要的变化有四点：

- 任务引用 project 前，必须先在当前 workspace 注册 project。
- `project` 查询、context、helper、audit 都会先在当前 workspace 内把 slug 解析成稳定 `project_id`。
- `project config` 成为 project 级业务配置的唯一入口；无 scope 的 `config` 不再读写 project 配置。
- JSON export 继续输出可读的 `project` slug，但脚本、API、token、MCP 应优先持有 `project_id`。

一个完整的 M5 日常流大致是这样：

```bash
# 先建 workspace，再注册 project
./taskg workspace add dajee name:Dajee
./taskg --workspace dajee project add ai-agent-platform name:"AI Agent Platform" description:"Owns MCP work"
./taskg --workspace dajee project add erp-rewrite name:"ERP Rewrite"

# 用 project slug 创建和查询任务
./taskg --workspace dajee add "Design task.query schema" project:ai-agent-platform +mcp
./taskg --workspace dajee add "Review ERP migration" project:erp-rewrite
./taskg --workspace dajee list project:ai-agent-platform

# 管理 project 元数据
./taskg --workspace dajee project list
./taskg --workspace dajee project info ai-agent-platform --json
./taskg --workspace dajee project modify ai-agent-platform description:"Owns taskg MCP and API work"
./taskg --workspace dajee project archive erp-rewrite

# 归档后不能再被新任务引用
./taskg --workspace dajee add "Should fail" project:erp-rewrite
```

如果你直接写一个不存在的 project，命令会失败，而不是偷偷创建：

```bash
./taskg add "Ghost task" project:ghost
# taskg: project_not_found: project "ghost" not found ...
```

### `project` 命令组

当前支持这些命令：

```bash
./taskg project list [--all]
./taskg project add <slug> name:<name> [description:<text>]
./taskg project info <slug|project-id>
./taskg project modify <slug|project-id> [name:<name>] [description:<text>]
./taskg project archive <slug|project-id>
```

说明：

- `project list` 默认只列 active project；加 `--all` 才包含 archived。
- `project info` / `modify` / `archive` 既接受 slug，也接受稳定 `project_id`。
- slug 只在当前 effective workspace 内解析，不做跨 workspace 搜索。

### `project config` 命令组

project 级业务配置固定走 `project config`：

```bash
./taskg project config set ai-agent-platform agent.background "Owns taskg MCP integration."
./taskg project config get ai-agent-platform agent.background
./taskg project config list ai-agent-platform
./taskg project config unset ai-agent-platform agent.background
```

当前白名单 key：

- `agent.background`
- `context.default`
- `constraint.description`

如果你误用无 scope 的 `config`：

```bash
./taskg config set agent.background "..."
```

会返回 `project_config_scope_required`，并提示改用 `project config set <project> ...`。

### Helper 与 audit 的 M5 行为

```bash
./taskg _projects           # 当前 workspace 的 active project slug
./taskg _projects --all     # 包括 archived
./taskg _unique project     # 当前查询结果中实际被任务引用到的 project slug
./taskg audit list --project ai-agent-platform --json
```

这里有两个容易混的点：

- `_projects` 看的是 project 表，所以“已注册但暂时没有任务”的 project 也会出现。
- `_unique project` 看的是当前查询结果里的任务绑定，所以只会输出实际被命中的 project。

`audit list --project` 支持 slug 或 `project_id`，JSON 输出里会带 `project_id`，方便脚本继续串联。

### M5 升级提示

从 M4 升级到 M5 时，数据库会自动迁移：

- 原有 `tasks.project` 会尽量回填到 `projects` 与 `tasks.project_id`
- 不合法、冲突或无法安全归一化的旧 project 值会被跳过，并写入迁移报告

如果本次启动存在跳过项，CLI 会在 `stderr` 打一行 warning。详细报告可用：

```bash
./taskg config get migration.m5.projects.skipped
```

这个 key 只用于迁移排障，不是业务配置。

## M6 Server、Token 与远程 CLI

M6 已经把 HTTP/JSON API、PAT / Agent token 和远程 CLI 接到同一套 app service 上。本地 CLI 仍然可以直接打开 SQLite；远程 CLI 通过 HTTP API 访问服务端，不会在 remote mode 下写本机任务库。

启动服务端：

```bash
./taskg server --listen :8080
./taskg server --listen 127.0.0.1:8080 --db ./taskg.db
```

服务端不内置 TLS。生产部署应放在可信网络内，或使用 Nginx / Caddy 等反向代理做 TLS termination；不要把裸 HTTP token 服务直接暴露公网。服务端运行期间 SQLite 支持多进程读写排队，但生产建议同一时间只有一个主要写入口。

创建第一个 token 建议在 server 启动前用本地 CLI 完成：

```bash
./taskg --workspace local token create cli \
  --scope task:read,task:write,project:read,project:write,context:read,context:write,config:read,config:write,audit:read,token:read,token:write \
  --expires-in 720h
```

PAT raw token 以 `taskg_pat_` 开头，Agent token raw token 以 `taskg_agent_` 开头。raw token 只在创建时输出一次；数据库只保存 hash 和短 prefix。后续可以通过 HTTP/远程 CLI 管理 token：

```bash
./taskg --server http://127.0.0.1:8080 --token "$TASKG_TOKEN" token list
./taskg --server http://127.0.0.1:8080 --token "$TASKG_TOKEN" token revoke <token-id-or-prefix>
```

远程 CLI：

```bash
export TASKG_SERVER=http://127.0.0.1:8080
export TASKG_TOKEN=taskg_pat_xxx

./taskg --server "$TASKG_SERVER" --token "$TASKG_TOKEN" --workspace local list
./taskg --server "$TASKG_SERVER" --token "$TASKG_TOKEN" add "Review API docs" --project-id <project-id>
./taskg --server "$TASKG_SERVER" --token "$TASKG_TOKEN" 1 done
```

`--server` / `--token` 也可以来自环境变量 `TASKG_SERVER` / `TASKG_TOKEN`，或本机 `taskg.toml`：

```toml
[remote]
server = "http://127.0.0.1:8080"
token = "taskg_pat_xxx"
```

如果 `taskg.toml` 包含 `remote.token` 且权限比 `0600` 更宽，CLI 会向 stderr 输出 warning，但不会阻止执行。推荐优先用环境变量或系统 secret manager 注入 token，不要把含 token 的 TOML 提交到公共仓库。

Token scope 是收窄，不是放大。最终权限是：

```text
membership role 权限 ∩ token capability scope ∩ token workspace scope ∩ token project scope
```

常用 capability：

```text
task:read task:write project:read project:write context:read context:write config:read config:write audit:read token:read token:write workspace:read workspace:write
```

project-scoped token 只能看 allowlist 内的任务和 audit。单任务读取如果任务存在但不在 token project allowlist 内，HTTP/远程 CLI 返回 404 `task_not_found`，避免泄露资源存在性。HTTP path 中的 `{uuid}` 只接受真实 UUID；远程 `info 1` 和 `1 done` 这类 working-set ID 会先由客户端两跳解析为 UUID。

远程 CLI 已覆盖核心任务、报表、project、project config、context、config、import/export、audit、token 和 helper 命令。`edit`、`config import-taskrc` 等需要本地编辑器或本机文件语义的命令在 remote mode 下暂不支持。`_unique`、`_tags` 等 helper 通过已有 list/export endpoint 在客户端后处理，大 workspace 上可能较慢；M6 不新增 aggregation endpoint。
