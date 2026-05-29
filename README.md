# taskg — Taskwarrior 风格的多用户多 Workspace 任务管理系统（Go 版）

`taskg` 是一个用 **纯 Go** 实现的 Taskwarrior 风格任务管理系统：

- 单一二进制：同时承担 **本地 CLI / 远程 CLI 客户端 / HTTP API 服务端 / MCP Server** 四种形态
- 数据库：**SQLite（GORM + `github.com/glebarez/sqlite`，零 CGO）**，可跨平台交叉编译
- 多用户、多 workspace、行级隔离
- 兼容 Taskwarrior 的核心命令名、JSON 数据格式与 urgency 公式

## 详细需求

参见 `docs/requirements.md`。该文档梳理了：

- 上游 Taskwarrior 的数据模型 / CLI 语法 / 报表 / Urgency / DOM / Hook / 同步语义
- 多用户多 workspace、MCP、飞书触发等扩展需求
- SQLite 表结构草案、CLI 命令分级、MCP 工具 JSON Schema 草案
- 所有结论附参考来源链接

## 状态

完整 milestone 拆解与当前进度见 [ROADMAP.md](./ROADMAP.md)。

## M0 本地 CLI 用法

```bash
go build -o taskg ./cmd/taskg

# 添加任务
./taskg add "Write project spec" project:taskg +planning due:tomorrow
./taskg add "Review PR" priority:H +review

# 查看任务列表
./taskg list

# 查看任务详情
./taskg info 1

# 修改任务
./taskg 1 modify priority:H +next
./taskg 1 modify project:backend

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
./taskg '(project:work and +urgent) or priority:H' list

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
./taskg _uuids project:work
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
./taskg context define work 'project:work status:pending'
./taskg context use work
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
./taskg _show date.format active.context
./taskg _version
```

M3 新增 `~/.config/taskg/taskg.toml` 作为文件配置来源。数据库路径仍按 `--db`、`TASKG_DB`、`--data-dir`、TOML、XDG data、home fallback 的顺序解析；普通配置按 CLI flag、`rc.*`、环境变量、SQLite meta、TOML、默认值合并。

`context` 会自动叠加到 `list`、`next`、各类报表和 `_ids/_uuids/_projects/_tags/_unique`。`--no-context` 和 `rc.context=none` 只影响本次运行；`context none` 会持久化清空 active context。

UDA 支持 `string`、`numeric`、`date`、`duration` 四种类型。date UDA 写入为 RFC3339 UTC 字符串；查询 `estimate:3`、`reviewed:2026-05-28` 会结合当前 schema 编译。未定义的 JSON top-level 字段会作为 orphan UDA 保留并导出，普通 `modify` 不能修改 orphan UDA。

`.taskrc` 在 M3 中的作用是**迁移和兼容性导入**：taskg 只读解析它，把支持的 key 导入到 SQLite 配置、context 或 UDA schema，并生成 imported/skipped/unknown 报告。taskg 不会修改原 `.taskrc`，也不会把 `.taskrc` 当成每次运行的完整配置源。

当前 `.taskrc` 支持范围：

- 支持导入：`color`、`dateformat`、`context.<name>`、`uda.<name>.type/label/values/default`、`urgency.uda.*`
- 识别但跳过：`data.location`、`report.*`、`calendar.*`、`burndown.*`、`news.*`、`sync.*`、`hooks.*`
- 其它 key 进入 unknown 报告，不会让导入失败

其中 `data.location` 会被识别但不会导入，因为 M3 的数据库路径只在启动前通过 `--db`、`TASKG_DB`、`--data-dir` 或 TOML 决定。M3 还不支持完整 Taskwarrior `.taskrc` 语义，不导入自定义 report DSL，也不运行 hooks。

## M4 本地多用户、多 Workspace、权限与审计

```bash
# user
./taskg user list
./taskg user add alice email:alice@example.test
./taskg user use alice
./taskg user info

# workspace
./taskg workspace list
./taskg workspace add work name:Work visibility:team
./taskg workspace use work
./taskg workspace info work
./taskg workspace modify work description:"Team workspace"
./taskg workspace archive old

# 在指定 workspace 中执行一次命令
./taskg --workspace local list
./taskg --workspace work _projects

# member
./taskg member list
./taskg member add bob role:viewer
./taskg member role bob member

# audit
./taskg audit list
./taskg audit list --limit 20 --json
```

M4 新增了本地团队运行时：

- `user list/add/use/info`
- `workspace list/add/use/info/modify/archive`
- `member list/add/role`
- `audit list`
- 全局 `--workspace <slug|uuid>` 一次性切到指定 workspace 执行命令

权限模型：

- `viewer` 可以读任务、报表、helper、member list，并能切换自己的 active context
- `member` 额外可以写任务、import、定义/删除 context
- `admin` 额外可以改 workspace metadata、管理非 owner 成员、查看 audit、管理 UDA schema
- `owner` 额外可以授予/降级 owner、归档 workspace

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
