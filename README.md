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
./taskg add "Write docs" depends:<uuid>
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

当前 recurring 的基础约束：

- recurring parent 使用 `status:recurring` 持久化，默认 human 报表隐藏
- child 在创建 parent 时立即生成，完成 child 后自动生成下一个 child
- `until` 会阻止生成超过截止时间的新 child
- `monthly` 目前直接沿用 Go `time.AddDate(0, n, 0)` 的月末滚动语义
