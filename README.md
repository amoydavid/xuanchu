# 璇础 (Xuán Chǔ) — 面向企业项目与 Agent MCP 的 Taskwarrior 风格任务运行时（Go 版）

`璇`：取自《尚书·舜典》 “在璇玑玉衡，以齐七政”，指北斗星中运转、校准的枢机，象征规则与秩序的核心。
`础`：取自《淮南子·说林训》 “山云蒸，柱础润”，指房屋柱下的基石。汉代《说文解字》注：“础，磶也”，即承载重量的基础。

命令行输出为 `xuanchu`，Xuanchu 是一个用 **纯 Go** 实现的企业任务运行时。它借鉴 Taskwarrior 的 CLI、查询语言、任务字段和 urgency 思路，但产品目标不是做完整 Taskwarrior clone，而是服务企业项目协作和 Agent MCP：

- 单一二进制：同时承担 **本地 CLI / 远程 CLI 客户端 / HTTP API 服务端 / MCP Server** 四种形态
- 数据库：**SQLite（GORM + `github.com/glebarez/sqlite`，零 CGO）**，可跨平台交叉编译
- `workspace` 作为企业 / 租户级隔离边界；`project` 表示企业内的真实项目
- 支持多用户、权限、审计、行级隔离，并为 Agent token 和 MCP scope 预留边界
- 支持服务端 Hook、定时通知、事件通知规则、动态 endpoint 和 HTTP request template sink
- 借鉴 Taskwarrior 的核心命令名、JSON 迁移格式与 urgency 公式；企业能力优先于完全兼容

## 详细需求

参见 `docs/requirements.md`。该文档梳理了：

- 上游 Taskwarrior 的数据模型 / CLI 语法 / 报表 / Urgency / DOM / Hook / 同步语义
- 企业 workspace、project、Agent MCP、外部系统触发等扩展需求
- SQLite 表结构草案、CLI 命令分级、MCP 工具 JSON Schema 草案
- 所有结论附参考来源链接

## 状态

README 只描述当前代码已经具备的能力和常用入口。完整 milestone、版本历史和后续计划见 [ROADMAP.md](./ROADMAP.md)；更细的变更历史以 git 记录为准。

## 快速开始

```bash
go build -o xuanchu ./cmd/xuanchu

# 先注册一个项目，再创建第一条任务
./xuanchu project add agentapi name:"AI Agent Platform"
./xuanchu add "Write MCP task docs" project:agentapi +docs due:tomorrow

# 默认只看 pending 任务
./xuanchu list

# 查看详情。1 是列表里的 working-set ID；agentapi-1 是稳定短任务引用 task_slug
./xuanchu info 1
./xuanchu info agentapi-1

# 修改、完成、删除都可以用 <target> <action> 写法
./xuanchu agentapi-1 modify priority:H +next
./xuanchu agentapi-1 done
./xuanchu agentapi-1 delete
```

`project.slug` 会统一转成小写，只允许 3-10 位 ASCII 英文字母和数字，且必须以字母开头；同一 workspace 内不能重复，跨 workspace 可以重复。带 project 的任务会输出短标识 `task_slug`，格式为 `<projectSlug>-<seq>`，例如 `agentapi-1`。`--json` 输出中也会包含 `"task_slug":"agentapi-1"`。

本地 CLI 的 `target` 可以是列表里的数字 ID、完整 UUID、足够长的 UUID 前缀，或 `task_slug`。HTTP API 和 MCP tool 属于协议入口，只接受 UUID 或 `task_slug`，不会接受本地 working-set 数字 ID。常用命令既支持 `xuanchu <subcommand> ...`，也支持 Taskwarrior 风格的 `xuanchu <target> <action> ...`。

常用全局参数：

```bash
./xuanchu --db ./xuanchu.db list          # 使用指定 SQLite 文件
./xuanchu --data-dir ./data list        # 数据库放到 ./data/xuanchu.db
./xuanchu --db-url "postgres://user:pass@localhost:5432/xuanchu?sslmode=disable" list
./xuanchu --config /path/to/config.toml list  # 指定 TOML 配置文件
./xuanchu --json list                   # 输出 JSON
./xuanchu --no-color list               # 关闭颜色
./xuanchu --no-context list             # 本次命令忽略 active context
./xuanchu --workspace dajee list        # 本次命令切到 dajee workspace
./xuanchu --version                     # 显示版本号
```

默认数据库路径是 `~/.local/share/xuanchu/xuanchu.db`。配置优先级按“本次命令参数优先”理解即可：CLI flag / `rc.*` 覆盖 > 环境变量 > SQLite meta > `xuanchu.toml` > 默认值。

`--config` 可以指定任意 TOML 文件路径（环境变量 `XUANCHU_CONFIG` 等价），不指定时仍从 XDG 默认路径加载。版本号可通过 `--version` 查看，`go install` 构建的版本会从 git 信息自动推断。

## PostgreSQL

Xuanchu 支持 PostgreSQL 作为数据库后端，通过 `--db-url` 指定连接字符串：

```bash
./xuanchu --db-url "postgres://user:pass@localhost:5432/xuanchu?sslmode=disable" server
```

也可以通过环境变量或 TOML 配置：

```bash
export XUANCHU_DB_URL="postgres://user:pass@localhost:5432/xuanchu"
./xuanchu list
```

```toml
[database]
url = "postgres://user:pass@localhost:5432/xuanchu?sslmode=disable"
```

`--db-url` 和 `--db` 互斥。未指定 `--db-url` 时使用 SQLite（默认行为不变）。

## 命令和参数怎么写

任务属性一般写成 `key:value`，标签写成 `+tag` 或 `-tag`：

```bash
./xuanchu add "Ship MCP API" project:agentapi priority:H +next due:friday
./xuanchu add "Review release note" @alice
./xuanchu 1 modify project:agentapi priority:M +review -next
./xuanchu 1 modify +@alice -@bob
./xuanchu 1 modify due:                  # 清空 due
```

多 assignee 查询与展示：

```bash
./xuanchu list assignee:alice
./xuanchu next assignee:me
./xuanchu info 1
```

查询可以放在报表命令前，也可以放在报表命令后：

```bash
./xuanchu +next list
./xuanchu list +next
./xuanchu '(project:agentapi and +review) or priority:H' next
```

Shell 会吃掉括号、空格和 `+` 等字符，复杂查询建议加引号。`/text/` 是 description 子串匹配，不是正则。

脚本里建议优先使用这些稳定接口：

```bash
./xuanchu --json export
./xuanchu _ids +next
./xuanchu _uuids project:agentapi
./xuanchu _get 1.uuid 1.description 1.urgency
./xuanchu _show database.path active.user active.workspace active.context
```

## 本地 CLI 基础用法

```bash
go build -o xuanchu ./cmd/xuanchu

# 添加任务
./xuanchu add "Write project spec" project:agentapi +planning due:tomorrow
./xuanchu add "Review PR" priority:H +review

# 查看任务列表
./xuanchu list

# 查看任务详情
./xuanchu info 1

# 修改任务
./xuanchu 1 modify priority:H +next
./xuanchu 1 modify project:agentapi

# 完成任务
./xuanchu 1 done

# 删除任务
./xuanchu 1 delete

# 导出为 JSON
./xuanchu export

# 导入 JSON
./xuanchu import tasks.json

# 查看配置
./xuanchu show

# 设置配置
./xuanchu config set date.format rfc3339
./xuanchu config get date.format
```

默认数据库路径为 `~/.local/share/xuanchu/xuanchu.db`，可用 `--db` 或 `XUANCHU_DB` 环境变量覆盖。

## 查询、报表、Urgency 与 Helper

```bash
# 布尔组合查询
./xuanchu '+next or due.before:tomorrow' list
./xuanchu '(project:agentapi and +urgent) or priority:H' list

# 报表命令
./xuanchu all
./xuanchu completed
./xuanchu deleted
./xuanchu overdue

# 查看任务 urgency（human 或 JSON）
./xuanchu urgency 1
./xuanchu urgency 1 --json
./xuanchu _urgency 1

# DOM helper
./xuanchu _get 1.description 1.uuid 1.urgency 1.tag.next
./xuanchu _ids +next
./xuanchu _uuids project:agentapi
./xuanchu _projects
./xuanchu _tags

# 表达式计算
./xuanchu calc '1 + 2 * 3'
```

报表名等价于 `(默认 filter) AND (用户 filter)`。要绕过默认 status 限制，使用 `all`。

### 日期与 deadline 语义

`due:` 和 `end:` 表达的是「某天截止/结束」，写入时会自动落在**当地时区的当天 `23:59:59`**：

```bash
./xuanchu add "deadline" due:2030-01-01
# due 实际存储为 2030-01-01 23:59:59（本地时区），而非 00:00:00
```

查询时日期等值也用自然日范围，例如 `due:2030-01-01` 等价于 `[2030-01-01 00:00:00, 2030-01-01 23:59:59]`，跨 DST 与时区也稳定。

### description 子串匹配

`description:spec`、`description:/spec/` 和裸 `/spec/` **语义一致**，都按子串匹配；`description:` 不走字面相等。

## 核心任务模型

```bash
# waiting / active / ready / blocked / blocking 报表
./xuanchu add "Call vendor" wait:tomorrow scheduled:eow until:eom
./xuanchu waiting
./xuanchu 1 modify wait:
./xuanchu 1 start
./xuanchu active
./xuanchu 1 stop

# 注释与描述编辑
./xuanchu 1 annotate "called, left voicemail"
./xuanchu _get 1.annotations
./xuanchu 1 denotate <annotation-id>
./xuanchu 1 append "with examples"
./xuanchu 1 prepend "[draft]"
./xuanchu 1 edit

# 依赖与 blocked / blocking
./xuanchu add "Prepare API"
./xuanchu add "Write docs" depends:<uuid-or-id>
./xuanchu blocked
./xuanchu blocking

# 基础循环任务
./xuanchu add "Submit weekly report" recur:weekly due:2030-01-05 until:2030-02-01
./xuanchu list
./xuanchu 1 done
./xuanchu list
```

当前支持这些任务能力：

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

## 配置、Context、UDA 与 `.taskrc`

```bash
# TOML / config / rc 覆盖
./xuanchu show
./xuanchu config set date.format rfc3339
./xuanchu config list
./xuanchu rc.date.format=epoch list

# context
./xuanchu context define agent 'project:agentapi status:pending'
./xuanchu context use agent
./xuanchu list
./xuanchu --no-context list
./xuanchu context show
./xuanchu context none

# UDA
./xuanchu config set uda.estimate.type numeric
./xuanchu config set uda.estimate.label Estimate
./xuanchu config set uda.estimate.values 1,2,3,5,8
./xuanchu add "Implement API" estimate:3
./xuanchu estimate:3 list
./xuanchu _get 1.estimate
./xuanchu _udas
./xuanchu _unique estimate

# .taskrc 只读导入
./xuanchu config import-taskrc ~/.taskrc --dry-run --json
./xuanchu config import-taskrc ~/.taskrc

# completion 与脚本 helper
./xuanchu completion zsh > ~/.zfunc/_xuanchu
./xuanchu _show date.format active.user active.workspace active.context
./xuanchu _version
```

Xuanchu 支持用 `~/.config/xuanchu/xuanchu.toml` 作为文件配置来源。TOML 使用标准解析器，支持普通 TOML 字符串、数组、dotted key 和多行字符串。一个最小示例：

```toml
[database]
path = "/Users/me/.local/share/xuanchu/xuanchu.db"

[display]
color = true

[date]
format = "rfc3339"
```

### `xuanchu.toml` 可配置项

`xuanchu.toml` 是**本机配置**：它描述这台机器如何启动和显示 Xuanchu，不描述某个企业 workspace 的业务规则。TOML 的 key 会被展开成点分格式，例如 `[database] path = "..."` 等价于 `database.path = "..."`。

当前建议只把这些 key 写进 TOML：

| TOML 写法 | 展开后的 key | 值 | 说明 |
|---|---|---|---|
| `[database] path = "..."` | `database.path` | 文件路径 | SQLite 数据库路径。只在启动打开数据库前生效；运行后不能用 `config set database.path` 修改。 |
| `[display] color = true` | `color` | `true` / `false` | 是否启用 human 输出颜色。也可直接写 `color = true`。 |
| `[display] json = false` | `json` | `true` / `false` | 默认是否输出 JSON。CLI 的 `--json` 优先级更高。也可直接写 `json = false`。 |
| `[date] format = "rfc3339"` | `date.format` | `rfc3339` / `epoch` | `_show`、`config get` 和部分脚本输出使用的日期格式。 |
| `[log] level = "info"` | `log.level` | `debug` / `info` / `warn` / `error` | 日志级别。环境变量 `XUANCHU_LOG_LEVEL` 优先。 |
| `[log] format = "text"` | `log.format` | `text` / `json` | 日志格式。 |
| `[log] file = "..."` | `log.file` | 文件路径 | 日志文件路径。支持 `~` 展开。环境变量 `XUANCHU_LOG_FILE` 优先。 |
| `[log] rotate = "daily"` | `log.rotate` | `daily` / `size` / `none` | 日志轮转模式。`daily` 按日期切割，`size` 按 10MB 切割，`none` 不轮转。 |

一个完整的本机配置例子：

```toml
[database]
path = "/Users/me/.local/share/xuanchu/xuanchu.db"

[display]
color = true
json = false

[date]
format = "rfc3339"

[log]
level = "info"
format = "text"
file = "~/.local/share/xuanchu/logs/xuanchu.log"
rotate = "daily"
```

不要把这些业务配置长期写进 TOML：

- `uda.*`
- `urgency.uda.*`
- `context.<name>`
- 未来的 report、hook、project defaults

这些配置属于 workspace，应写入 SQLite，由权限和 audit 管理：

```bash
./xuanchu --workspace dajee config set uda.estimate.type numeric
./xuanchu --workspace dajee config set uda.estimate.values 1,2,3,5,8
./xuanchu --workspace dajee config set urgency.uda.estimate.coefficient 1.5
./xuanchu --workspace dajee context define agent 'project:agentapi status:pending'
```

不要把这些状态或内部 key 当作 TOML 配置写入：

- `active.user`、`active.workspace`、`active.context`：只读状态输出，用 `user use`、`workspace use`、`context use/none` 修改。
- `context.active`：兼容旧 key，但不作为运行时 active context 来源。
- `active_user_id`、`active_workspace.<user>`、`active_context.<user>.<workspace>`：内部 SQLite meta，只用于迁移和运行时状态。

想确认当前最终生效值，用：

```bash
./xuanchu config list
./xuanchu _show database.path color json date.format active.user active.workspace active.context
```

数据库路径仍按 `--db`、`XUANCHU_DB`、`--data-dir`、TOML、XDG data、home fallback 的顺序解析。显示类配置按 CLI flag、`rc.*`、环境变量、SQLite meta、TOML、默认值合并。workspace 业务配置以数据库中的 workspace 记录为准；TOML 只作为本机默认值或迁移辅助。

`rc.*` 只影响本次命令，适合脚本临时覆盖：

```bash
./xuanchu rc.date.format=epoch list
./xuanchu rc.json:on --workspace missing list
./xuanchu rc.context=none list
```

`context` 会自动叠加到 `list`、`next`、各类报表和 `_ids/_uuids/_projects/_tags/_unique`。`--no-context` 和 `rc.context=none` 只影响本次运行；`context none` 会持久化清空 active context。

UDA 支持 `string`、`numeric`、`date`、`duration` 四种类型。date UDA 写入为 RFC3339 UTC 字符串；查询 `estimate:3`、`reviewed:2026-05-28` 会结合当前 schema 编译。未定义的 JSON top-level 字段会作为 orphan UDA 保留并导出，普通 `modify` 不能修改 orphan UDA。

`.taskrc` 的作用是**迁移和兼容性导入**：Xuanchu 只读解析它，把支持的 key 导入到 SQLite 配置、context 或 UDA schema，并生成 imported/skipped/unknown 报告。Xuanchu 不会修改原 `.taskrc`，也不会把 `.taskrc` 当成每次运行的完整配置源。

当前 `.taskrc` 支持范围：

- 支持导入：`color`、`dateformat`、`context.<name>`、`uda.<name>.type/label/values/default`、`urgency.uda.*`
- 识别但跳过：`data.location`、`report.*`、`calendar.*`、`burndown.*`、`news.*`、`sync.*`、`hooks.*`
- 其它 key 进入 unknown 报告，不会让导入失败

其中 `data.location` 会被识别但不会导入，因为数据库路径只在启动前通过 `--db`、`XUANCHU_DB`、`--data-dir` 或 TOML 决定。Xuanchu 不导入自定义 Taskwarrior report DSL，也不运行本地 shell hooks。

## 企业 Workspace、权限与审计

```bash
# user
./xuanchu user list
./xuanchu user add alice email:alice@example.test
./xuanchu user use alice
./xuanchu user info

# workspace。这里的 dajee 表示企业 / 租户级工作空间
./xuanchu workspace list
./xuanchu workspace add dajee name:Dajee visibility:team
./xuanchu workspace use dajee
./xuanchu workspace info dajee
./xuanchu workspace modify dajee description:"Dajee enterprise workspace"
./xuanchu workspace archive old

# project 表示 workspace 内的真实企业项目
./xuanchu --workspace dajee project add agentapi name:"AI Agent Platform"
./xuanchu --workspace dajee project add erpflow name:"ERP Rewrite"
./xuanchu --workspace dajee add "Design MCP task.query schema" project:agentapi +mcp
./xuanchu --workspace dajee add "Migrate invoice workflow" project:erpflow +migration

# 在指定 workspace 中执行一次命令
./xuanchu --workspace local list
./xuanchu --workspace dajee _projects

# member
./xuanchu member list
./xuanchu member add bob role:viewer
./xuanchu member role bob member

# audit
./xuanchu audit list
./xuanchu audit list --limit 20 --json
```

企业运行时支持：

- `user list/add/use/info`
- `workspace list/add/use/info/modify/archive`
- `member list/add/role`
- `audit list`
- 全局 `--workspace <slug|uuid>` 一次性切到指定 workspace 执行命令

`workspace` 是企业 / 租户级隔离边界；`project` 是 workspace 内的一等实体。任务上仍保留 `project` 字符串字段做人类可读输出，但运行时写入、查询、权限、审计和 API/MCP scope 都以稳定 `project_id` 为准。

同一个 project slug 可以出现在不同 workspace 中。也就是说，`dajee/agentapi` 和 `partner/agentapi` 是两个不同项目；权限、token 和 MCP scope 必须以 `workspace + project` 或稳定 `project_id` 为准，不能把 slug 当全局唯一标识。

当前 CLI 的 `--workspace <slug|uuid>` 使用裸 workspace slug，所以 workspace slug 在同一个 Xuanchu 实例内应保持唯一。project slug 只在当前 workspace 内解析：

```bash
./xuanchu --workspace dajee list project:agentapi
./xuanchu --workspace partner list project:agentapi
```

上面两条命令访问的是两个不同 workspace 里的同名 project。Xuanchu 采用严格 project 注册：`xuanchu add ... project:<slug>` 和 `xuanchu 1 modify project:<slug>` 只能引用当前 workspace 内已存在、未归档的 project，不会运行时自动创建。脚本、远程 API 和 MCP 应优先保存 `project_id`。如果同时指定 `--workspace` 和 `project_id`，该 project 必须属于这个 workspace；否则命令会报错，避免把任务写进错误租户。

权限模型：

- `viewer` 可以读任务、报表、helper、member list，并能切换自己的 active context
- `member` 额外可以写任务、import、定义/删除 context
- `admin` 额外可以改 workspace metadata、管理非 owner 成员、查看 audit、管理 UDA schema、管理服务端 Hook
- `owner` 额外可以授予/降级 owner、归档 workspace

`config list` 和 `_show` 只暴露用户可用的 public key，例如 `active.user`、`active.workspace`、`active.context`、`uda.*`、`urgency.*`。`active_user_id`、`active_workspace.<user>`、`active_context.<user>.<workspace>` 是内部 meta，不作为 CLI 接口使用。

`audit list` 的 human 输出包含时间、actor、action、target type 和 target id。`--json` 会额外输出 `actor_user_id`、`actor_name`、`workspace_id`、`payload` 等字段，适合脚本处理。

当前没有 `member delete`。如果要临时撤销写权限，可用 `member role <user> viewer`。同时，`viewer` 仍能读取该 workspace 的任务、project、tag、UDA 和成员信息。

## Project 实体与配置边界

Project 与配置边界的关键规则：

- 任务引用 project 前，必须先在当前 workspace 注册 project。
- `project` 查询、context、helper、audit 都会先在当前 workspace 内把 slug 解析成稳定 `project_id`。
- `project config` 继续是 project 级业务配置入口，但 key 的合法性现在由 workspace 内 `config schema` 决定，不再靠代码白名单。
- JSON export 继续输出可读的 `project` slug，但脚本、API、token、MCP 应优先持有 `project_id`。

一个完整的日常流大致是这样：

```bash
# 先建 workspace，再注册 project
./xuanchu workspace add dajee name:Dajee
./xuanchu --workspace dajee project add agentapi name:"AI Agent Platform" description:"Owns MCP work"
./xuanchu --workspace dajee project add erpflow name:"ERP Rewrite"

# 用 project slug 创建和查询任务
./xuanchu --workspace dajee add "Design task.query schema" project:agentapi +mcp
./xuanchu --workspace dajee add "Review ERP migration" project:erpflow
./xuanchu --workspace dajee list project:agentapi

# 管理 project 元数据
./xuanchu --workspace dajee project list
./xuanchu --workspace dajee project info agentapi --json
./xuanchu --workspace dajee project modify agentapi description:"Owns Xuanchu MCP and API work"
./xuanchu --workspace dajee project archive erpflow

# 归档后不能再被新任务引用
./xuanchu --workspace dajee add "Should fail" project:erpflow
```

如果你直接写一个不存在的 project，命令会失败，而不是偷偷创建：

```bash
./xuanchu add "Ghost task" project:ghost
# xuanchu: project_not_found: project "ghost" not found ...
```

### `project` 命令组

当前支持这些命令：

```bash
./xuanchu project list [--all]
./xuanchu project add <slug> name:<name> [description:<text>]
./xuanchu project info <slug|project-id>
./xuanchu project modify <slug|project-id> [name:<name>] [description:<text>]
./xuanchu project archive <slug|project-id>
```

说明：

- `project list` 默认只列 active project；加 `--all` 才包含 archived。
- `project info` / `modify` / `archive` 既接受 slug，也接受稳定 `project_id`。
- slug 只在当前 effective workspace 内解析，不做跨 workspace 搜索。

### `config schema` 与 `project config`

shared config 现在分成两层：

- `config schema`
  - 定义 key、类型、允许作用域、默认值等 schema
- `config`
  - 写 workspace scope 的显式值
- `project config`
  - 写 project scope 的显式值

典型流程：

```bash
./xuanchu --workspace dajee config schema set agent.background type:string scopes:project label:"Agent Background"
./xuanchu --workspace dajee config schema set ads.roi_threshold type:number scopes:workspace,project default:1.8

./xuanchu --workspace dajee config set ads.roi_threshold 2.0
./xuanchu project config set agentapi agent.background "Owns Xuanchu MCP integration."
./xuanchu project config get agentapi agent.background
./xuanchu project config list agentapi
./xuanchu project config unset agentapi agent.background
```

也可以直接管理 schema：

```bash
./xuanchu --workspace dajee config schema list
./xuanchu --workspace dajee config schema get agent.background
./xuanchu --workspace dajee config schema delete ads.roi_threshold
./xuanchu --workspace dajee config schema delete ads.roi_threshold --purge
```

说明：

- schema 严格按 workspace 隔离；不同 workspace 可以定义同名 key，但定义互不影响。
- `project config get` 的读取链是：`project 显式值 > workspace 显式值 > schema default`。
- `project config list` 和 `config list` 仍然只列当前 scope 的显式值，不展开继承结果。
- 删除 schema 时，如果当前 workspace 下还存在对应 workspace/project 值，默认返回 `config_definition_in_use`；只有显式 `--purge` 才会连值一起删。

当前系统会自动为这些 key 预置 schema：

- `agent.background`
- `agent.constraints`
- `agent.default_context`
- `agent.handoff`
- `context.default`

如果你误用无 scope 的 `config`：

```bash
./xuanchu config set agent.background "..."
```

现在会返回 `config_scope_not_allowed`，因为 `agent.background` 已有 schema，但只允许 `project` scope。

### Helper 与 audit 行为

```bash
./xuanchu _projects           # 当前 workspace 的 active project slug
./xuanchu _projects --all     # 包括 archived
./xuanchu _unique project     # 当前查询结果中实际被任务引用到的 project slug
./xuanchu audit list --project agentapi --json
```

这里有两个容易混的点：

- `_projects` 看的是 project 表，所以“已注册但暂时没有任务”的 project 也会出现。
- `_unique project` 看的是当前查询结果里的任务绑定，所以只会输出实际被命中的 project。

`audit list --project` 支持 slug 或 `project_id`，JSON 输出里会带 `project_id`，方便脚本继续串联。

## Server、Token 与远程 CLI

HTTP/JSON API、PAT / Agent token 和远程 CLI 接到同一套 app service 上。本地 CLI 可以直接打开 SQLite 或 PostgreSQL；远程 CLI 通过 HTTP API 访问服务端，不会在 remote mode 下写本机任务库。

启动服务端：

```bash
./xuanchu server --listen :8080
./xuanchu server --listen 127.0.0.1:8080 --db ./xuanchu.db
```

服务端不内置 TLS。生产部署应放在可信网络内，或使用 Nginx / Caddy 等反向代理做 TLS termination；不要把裸 HTTP token 服务直接暴露公网。服务端运行期间 SQLite 支持多进程读写排队，但生产建议同一时间只有一个主要写入口。

### Server Admin Bootstrap

Server admin token 是服务端控制面 bootstrap token，只能访问 `/api/v1/admin/*`，不会写入 `api_tokens`，也不能访问普通任务、workspace、token API。它用于在自动化部署或 Agent 平台初始化时创建 workspace，并给指定 workspace 创建受限 Agent token。

先生成明文 token 和 hash：

```bash
./xuanchu admin token generate
```

把 hash 写入 `xuanchu.toml`，明文只交给部署系统或控制面调用方：

```toml
[server.admin]
enabled = true

[[server.admin.tokens]]
name = "ops-primary"
hash = "sha256:replace-with-token-hash"
enabled = true
```

也可以用 `hash_env` 从环境变量读取 hash，便于轮换：

```toml
[[server.admin.tokens]]
name = "ops-rotation"
hash_env = "XUANCHU_ADMIN_TOKEN_HASH"
enabled = true
```

明文丢失后不能从数据库或配置恢复，只能生成新 token 并替换 hash。普通 API token 不能访问 admin endpoint；admin token 也不能访问普通 API。

典型 bootstrap 流程是先创建 workspace，再为该 workspace 创建 Agent token：

```bash
curl -X POST http://127.0.0.1:8080/api/v1/admin/workspaces \
  -H "Authorization: Bearer $XUANCHU_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"slug":"team","owner":{"name":"alice","email":"alice@example.com"}}'

curl -X POST http://127.0.0.1:8080/api/v1/admin/workspaces/team/agent-tokens \
  -H "Authorization: Bearer $XUANCHU_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"openclaw","user":"alice@example.com","scopes":["task:read","task:write"],"expires_in":"24h"}'
```

创建 Agent token 的响应只会返回一次明文 `xuanchu_agent_...`，之后只能重新签发。常见错误包括：未启用 admin bootstrap 时返回 `route_not_found`，重复 workspace 返回 `admin_workspace_exists`，owner name/email 指向不同用户返回 `admin_owner_invalid`，非法 scope 返回 `token_scope_invalid`。

创建第一个 token 建议在 server 启动前用本地 CLI 完成：

```bash
./xuanchu --workspace local token create cli \
  --scope task:read,task:write,project:read,project:write,context:read,context:write,config:read,config:write,audit:read,token:read,token:write \
  --expires-in 720h
```

PAT raw token 以 `xuanchu_pat_` 开头，Agent token raw token 以 `xuanchu_agent_` 开头。raw token 只在创建时输出一次；数据库只保存 hash 和短 prefix。后续可以通过 HTTP/远程 CLI 管理 token：

```bash
./xuanchu --server http://127.0.0.1:8080 --token "$XUANCHU_TOKEN" token list
./xuanchu --server http://127.0.0.1:8080 --token "$XUANCHU_TOKEN" token revoke <token-id-or-prefix>
```

远程 CLI：

```bash
export XUANCHU_SERVER=http://127.0.0.1:8080
export XUANCHU_TOKEN=xuanchu_pat_xxx

./xuanchu --server "$XUANCHU_SERVER" --token "$XUANCHU_TOKEN" --workspace local list
./xuanchu --server "$XUANCHU_SERVER" --token "$XUANCHU_TOKEN" add "Review API docs" --project-id <project-id>
./xuanchu --server "$XUANCHU_SERVER" --token "$XUANCHU_TOKEN" 1 done
```

`--server` / `--token` 也可以来自环境变量 `XUANCHU_SERVER` / `XUANCHU_TOKEN`，或本机 `xuanchu.toml`：

```toml
[remote]
server = "http://127.0.0.1:8080"
token = "xuanchu_pat_xxx"
```

如果 `xuanchu.toml` 包含 `remote.token` 且权限比 `0600` 更宽，CLI 会向 stderr 输出 warning，但不会阻止执行。推荐优先用环境变量或系统 secret manager 注入 token，不要把含 token 的 TOML 提交到公共仓库。

Token scope 是收窄，不是放大。最终权限是：

```text
membership role 权限 ∩ token capability scope ∩ token workspace scope ∩ token project scope
```

常用 capability：

```text
task:read task:write project:read project:write context:read context:write config:read config:write audit:read token:read token:write workspace:read workspace:write hook:read hook:write notification:read notification:write reminder:read reminder:write impersonate
```

project-scoped token 只能看 allowlist 内的任务和 audit。单任务读取如果任务存在但不在 token project allowlist 内，HTTP/远程 CLI 返回 404 `task_not_found`，避免泄露资源存在性。HTTP path 中的 `{taskRef}` 接受 UUID 或 `task_slug`，纯数字 working-set ID 会返回 `task_ref_invalid`；远程 `info 1` 和 `1 done` 这类 working-set ID 会先由客户端两跳解析，再调用 HTTP API。

远程 CLI 覆盖核心任务、报表、project、project config、context、config、import/export、audit、token 和 helper 命令。`edit`、`config import-taskrc` 等需要本地编辑器或本机文件语义的命令在 remote mode 下暂不支持。`_unique`、`_tags` 等 helper 通过已有 list/export endpoint 在客户端后处理，大 workspace 上可能较慢。

## MCP Server

Agent 可以通过 MCP 协议以结构化方式使用 Xuanchu。MCP 支持 stdio 和 HTTP 两种传输方式，所有 tool 调用都经过与 CLI/API 相同的 `internal/app` service、权限和审计路径。

### MCP stdio 模式

本地 Agent 直接通过标准输入输出连接 Xuanchu：

```bash
# 本地 MCP，使用默认本地数据库
./xuanchu mcp stdio

# 指定数据库
./xuanchu --db ./xuanchu.db mcp stdio
```

stdio 模式使用本地 actor 和 workspace，不需要 token。stdout 只输出 MCP JSON-RPC 协议帧，不会混入迁移 warning 或日志。

### MCP HTTP 模式

`xuanchu server` 在 `/mcp` 路径暴露 Streamable HTTP MCP endpoint：

```bash
# 启动服务端
./xuanchu server --listen :8080

# MCP 客户端连接
# POST http://127.0.0.1:8080/mcp
# Authorization: Bearer xuanchu_pat_xxx
```

HTTP MCP 需要 Bearer token 鉴权，权限规则与 REST API 一致：`membership role 权限 ∩ token capability ∩ token workspace scope ∩ token project scope`。给 Agent 的默认建议是 workspace-scoped Agent token，让它服务同一 workspace 内多个 project；只服务单项目时再用 project allowlist 收窄。`/mcp` 不在 OpenAPI 文档中。

### MCP tools 列表

| Tool | 说明 |
|---|---|
| `task_add` | 添加任务 |
| `task_modify` | 修改任务 |
| `task_done` | 完成任务 |
| `task_delete` | 删除任务 |
| `task_query` | 通用查询，支持 filter、status、limit |
| `task_get` | 按 UUID 或 `task_slug` 读取任务 |
| `task_annotate` | 添加注释 |
| `task_depends` | 添加依赖 |
| `task_start` | 开始任务 |
| `task_stop` | 停止任务 |
| `report_run` | 运行预定义报表 |
| `urgency_explain` | 解释 urgency 构成 |
| `workspace_list` | 列出可见 workspace |
| `workspace_get_current` | 当前 workspace |
| `project_list` | 列出当前 workspace 项目 |
| `project_get` | 读取单个项目 |
| `project_get_current` | 当前 project scope |
| `context_set` | 设置 active context |
| `context_get` | 显示 active context |
| `config_get` | 读取配置 |
| `config_set` | 写入配置 |
| `notification_sink_add` | 创建通知 sink |
| `reminder_rule_add` | 创建定时提醒规则 |
| `notification_delivery_replay` | 重放失败通知投递 |

每个 tool 返回 `{data, rendered}` 双格式：`data` 是结构化 JSON，`rendered` 是人类可读文本。

### MCP resources

- `xuanchu://workspace/current` — 当前 workspace 概要
- `xuanchu://workspace/{workspace_id}` — 指定 workspace 信息
- `xuanchu://project/{project_id}` — 项目元数据与 Agent 背景
- `xuanchu://context/current` — 当前 context 与 scope

### 远程管理命令

以下管理命令支持远程模式，不会触碰客户端本地数据库：

```bash
./xuanchu --server http://127.0.0.1:8080 --token "$XUANCHU_TOKEN" workspace list
./xuanchu --server http://127.0.0.1:8080 --token "$XUANCHU_TOKEN" workspace add team name:Team
./xuanchu --server http://127.0.0.1:8080 --token "$XUANCHU_TOKEN" user list
./xuanchu --server http://127.0.0.1:8080 --token "$XUANCHU_TOKEN" member list
./xuanchu --server http://127.0.0.1:8080 --token "$XUANCHU_TOKEN" show date.format
```

## 服务端 Webhook Hook

`xuanchu server` 支持服务端 post-commit webhook hook。这里的 hook 是服务端出站 webhook，不是 Taskwarrior 的本地 shell hook。Hook 使用 workspace 级 outbound sink；`--sink <sink-ref>` 可以是当前 workspace 内的 sink 名称或 ID，不能跨 workspace 引用。

```bash
# 先创建出站 sink
./xuanchu notification sink add audit-stream \
  --type webhook \
  --url https://example.test/xuanchu

# 创建 workspace 级 hook
./xuanchu hook add task-webhook --event task.created --event task.completed --sink audit-stream

# 创建 project 级 hook
./xuanchu hook add proj-webhook --scope project --project myproject --event task.modified --sink audit-stream

# 查看 hook 列表
./xuanchu hook list

# 查看投递记录
./xuanchu hook deliveries <hook-id>

# 重试失败投递
./xuanchu hook replay <delivery-id>
```

Hook 支持的 event type：`task.created`、`task.modified`、`task.completed`、`task.deleted`、`project.archived`、`project.annotated`、`project.denotated`、`task.unblocked`。投递失败不会回滚已提交的 task/project 事务。生成 delivery 时会冻结 sink 渲染后的请求快照，后续 retry/replay 不重新渲染当前 sink。所有 hook 配置变更和人工 replay 都会写入 audit log。

## 通知、提醒与第三方通知

通知系统复用 notification sink，但规则分两类：

- reminder rule：定时扫描任务过滤器，适合到期前和逾期后的提醒。
- notification rule：监听事件并解析 audience，适合 `task.unblocked`、项目 annotation 等事件通知。

管理员先创建 notification sink，再创建 reminder rule 或 notification rule；`xuanchu server` 的后台 scheduler / dispatcher 命中规则后生成 delivery。

```bash
./xuanchu notification sink add openclaw \
  --type webhook \
  --url https://openclaw.example.com/xuanchu/notifications \
  --secret "$WEBHOOK_SECRET" \
  --max-concurrency 0

./xuanchu reminder rule add due-soon-24h \
  --schedule daily@08:50 \
  --filter 'end.isnull and start.isnull and due.after:now and due.before:now+24h' \
  --audience assignees \
  --sink openclaw

./xuanchu reminder rule add overdue-daily \
  --schedule daily@09:00 \
  --filter 'status:pending and end.isnull and due.before:now' \
  --repeat every:24h \
  --audience assignees \
  --sink openclaw

./xuanchu notification rule add task-unblocked \
  --event task.unblocked \
  --audience assignees \
  --sink openclaw

./xuanchu notification delivery list --status dead_lettered
./xuanchu notification delivery replay <delivery-id>
```

Notification rule 支持的事件类型和 Hook 当前白名单一致：`task.created`、`task.modified`、`task.completed`、`task.deleted`、`project.archived`、`project.annotated`、`project.denotated`、`task.unblocked`。第三方固定 Web API 使用 `http_template` sink。header/body 模板保存在数据库中，secret 通过 secret config 引用；生成 delivery 时会冻结 `resolved_url`、header、body 和 content type，retry/replay 不重新渲染当前模板。

notification / hook delivery 表是出站投递的可靠队列；进程内 worker 只做短暂执行协调。dispatcher 默认 `max_concurrency=1`，`batch_size` 只是每轮查询上限。sink 的 `max_concurrency=0` 表示继承默认 sink 并发；`xuanchu server` 内 notification dispatcher 和 hook dispatcher 共享同一个 sink limiter。同一个 delivery payload 会带稳定 `delivery_id`，接收方可据此幂等去重；本次 HTTP 请求真实尝试次数看 `X-Xuanchu-Attempt` header。详见 [定时通知与第三方通知](docs/manual/notifications.md)。

## Impersonation

Agent 平台可以持有一个带 `impersonate` scope 的 Agent token，以 workspace 成员的身份发起请求。

```bash
# 创建可 impersonate 的 Agent token（需要 admin/owner）
./xuanchu --workspace local token create pm-agent \
  --type agent \
  --scope task:read,task:write,impersonate \
  --expires-in 8760h

# 远程 CLI 使用 impersonation
./xuanchu --server http://127.0.0.1:8080 \
  --token "$AGENT_TOKEN" \
  --workspace local \
  --as alice \
  list assignee:me
```

HTTP API 通过 `X-Xuanchu-As` header：

```
GET /api/v1/tasks
Authorization: Bearer xuanchu_agent_...
X-Xuanchu-As: alice
```

权限以目标用户的 membership role 与 token scope 的交集为准，impersonation 不能提权。Token 可见多个 workspace 且请求未显式指定 workspace 时返回 `workspace_required`。目标用户不存在或不是成员时返回 `membership_not_found`。

Audit log 同时记录 `actor_user_id`（目标用户）和 `delegator_token_id`/`delegator_user_id`（发起方 Agent token）。

HTTP MCP 的每个 tool call 都支持 `X-Xuanchu-As` header 透传。stdio MCP 不支持 impersonation。

部署、TLS、备份恢复请参考 [`docs/deployment.md`](./docs/deployment.md) 和 [`docs/backup-restore.md`](./docs/backup-restore.md)。
