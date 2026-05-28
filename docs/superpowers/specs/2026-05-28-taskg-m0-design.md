# taskg M0 设计规格

> **给 agentic workers 的要求：** 编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

**目标：** 构建第一个可用的本地版 `taskg` 二进制程序：它是一个 Taskwarrior 风格的 CLI，使用 SQLite 存储，支持一个隐式本地用户、一个隐式 workspace，以及核心任务生命周期。

**范围：** 本文档只覆盖 M0。多用户鉴权、HTTP API、MCP Server、op-log 同步、Hook、循环任务、UDA、DOM、高级报表，以及完整 Taskwarrior 兼容能力均属于后续里程碑。

**需求来源：** 本规格从 [docs/requirements.md](/Users/mac/code/projects/dajee/task/docs/requirements.md) 中收束出第一条可以独立落地、独立测试、独立交付的实现切片。

---

## 1. 项目现状

当前仓库仍处于设计阶段，已有文件包括：

- [README.md](/Users/mac/code/projects/dajee/task/README.md)：项目概览。
- [docs/requirements.md](/Users/mac/code/projects/dajee/task/docs/requirements.md)：覆盖 M0-M6 的完整需求文档。

仓库里尚未创建 Go module、源码目录、数据库迁移、测试或 CLI 实现。因此 M0 需要包含必要的项目脚手架，但只搭建本地 CLI 所需的最小结构。

## 2. 产品边界

M0 需要让单个本地用户无需启动服务端，就能通过终端管理任务：

```bash
taskg add "Write project spec" project:taskg +planning due:tomorrow
taskg list
taskg 1 modify priority:H
taskg 1 done
taskg info 1
taskg export
```

在已支持的命令范围内，交互体验应尽量贴近 Taskwarrior；内部架构则应为后续里程碑保留清晰扩展点。

### 范围内

- 单一二进制，名称为 `taskg`。
- 本地 SQLite 数据库，驱动使用 `modernc.org/sqlite`。
- 禁止 CGO 依赖。
- Go 1.22+ module 与测试基础设施。
- 隐式本地用户与隐式本地 workspace。
- 数据库路径和输出偏好的基础配置解析。
- M0 所需的核心任务字段。
- 命令：`add`、`list`、`next`、`modify`、`done`、`delete`、`info`、`config`、`show`、`import`、`export`。
- 常见 Taskwarrior 风格修改参数解析。
- 基础过滤：数字 ID、UUID、status、project、tag、priority、due、自由文本。
- 人类可读表格输出和 `--json` 输出。
- 导出 JSON 使用与 Taskwarrior 尽量一致的字段名。
- 聚焦的单元测试和 CLI 集成测试。

### 范围外

- HTTP API 与远程 CLI 模式。
- MCP Server。
- JWT、PAT、用户、成员关系和权限控制。
- operation-log 复制与离线同步。
- Hook 执行。
- 循环任务。
- UDA schema 与 orphan UDA 处理。
- DOM `_get`、`calc` 子语言和完整表达式引擎。
- 高级报表定义与用户自定义报表配置。
- 完整 `.taskrc` 兼容。
- Cobra 默认能力之外的 shell completion。
- GUI 或 TUI。

## 3. 架构

M0 采用朴素的分层架构：

```text
cmd/taskg
  -> internal/cli
      -> internal/app
          -> internal/task
          -> internal/query
          -> internal/config
          -> internal/storage/sqlite
```

CLI 层负责命令行形态解析和输出格式化。App 层负责用例编排和事务边界。Domain 包负责任务生命周期规则。Storage 层负责 SQLite 细节。Query 解析在 M0 阶段保持很小，但需要返回类型化 filter，方便 M1 替换或扩展为真正的 AST。

Domain 包不应依赖 Cobra、Viper、SQLite driver 类型或终端渲染库。

## 4. 建议文件结构

实施计划应从以下文件切入：

- `go.mod`：Go module 定义。
- `cmd/taskg/main.go`：二进制入口。
- `internal/cli/root.go`：Cobra root command、全局 flags、命令注册。
- `internal/cli/add.go`：`add` 命令。
- `internal/cli/list.go`：`list` 与 `next` 命令。
- `internal/cli/modify.go`：`modify`、`done`、`delete`。
- `internal/cli/info.go`：`info` 命令。
- `internal/cli/config.go`：`config` 与 `show`。
- `internal/cli/import_export.go`：JSON 导入导出。
- `internal/app/service.go`：任务用例与事务边界。
- `internal/task/model.go`：任务模型、状态常量、校验。
- `internal/task/modification.go`：解析后的修改结构与生命周期操作。
- `internal/query/filter.go`：M0 filter 模型。
- `internal/query/parser.go`：M0 阶段的小型 Taskwarrior 风格解析器。
- `internal/config/config.go`：配置加载与路径解析。
- `internal/storage/sqlite/db.go`：SQLite 打开、pragma、迁移。
- `internal/storage/sqlite/schema.sql`：M0 数据库 schema。
- `internal/storage/sqlite/task_repo.go`：任务仓储。
- `internal/render/table.go`：表格渲染。
- `internal/render/json.go`：JSON 渲染辅助。
- `tests/integration/cli_test.go`：黑盒 CLI 集成测试。

这是一份设计目标，不要求一次性创建所有文件。后续实施计划应把它们拆成可以逐步验证的小任务。

## 5. 数据模型

M0 使用最小 schema，同时从第一天起保留 `workspace_id`，避免后续多 workspace 改造时推倒重来：

```sql
CREATE TABLE meta (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE workspaces (
  id TEXT PRIMARY KEY,
  slug TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  created_at INTEGER NOT NULL
);

CREATE TABLE tasks (
  uuid TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id),
  description TEXT NOT NULL,
  status TEXT NOT NULL CHECK(status IN ('pending','completed','deleted')),
  entry INTEGER NOT NULL,
  modified INTEGER NOT NULL,
  end_ts INTEGER,
  due INTEGER,
  project TEXT,
  priority TEXT CHECK(priority IN ('H','M','L') OR priority IS NULL)
);

CREATE TABLE task_tags (
  task_uuid TEXT NOT NULL REFERENCES tasks(uuid) ON DELETE CASCADE,
  tag TEXT NOT NULL,
  PRIMARY KEY (task_uuid, tag)
);
```

数据库初始化时应创建隐式本地 workspace：

- `id`：稳定 UUID，存储在数据库中。
- `slug`：`local`。
- `name`：`Local`。

任务的数字 `id` 不持久化。它是当前 working set 的展示序号，由报表排序结果动态派生。M0 解析数字 ID 时，必须使用与 `list` 相同的 working set 查询规则。

## 6. CLI 语义

### 全局参数

- `--data-dir PATH`：覆盖数据目录。
- `--db PATH`：覆盖 SQLite 数据库路径。
- `--json`：输出机器可读 JSON。
- `--no-color`：禁用颜色输出。

默认数据库路径优先级：

1. `$TASKG_DB`，如果已设置。
2. `$XDG_DATA_HOME/taskg/taskg.db`，如果 `XDG_DATA_HOME` 已设置。
3. `~/.local/share/taskg/taskg.db`。

### `add`

支持输入：

```bash
taskg add "description" project:work +tag priority:H due:tomorrow
taskg add description words project:work +tag
```

规则：

- 移除可识别 modification 后，剩余内容作为 description；description 必填。
- 新任务状态为 `pending`。
- `entry` 和 `modified` 写入当前时间。
- 支持 modification：`project:<value>`、`priority:H|M|L`、`due:<date>`、`+tag`、`-tag`。
- `add` 中的 `-tag` 可以接受，但不会产生效果。

### `list` 与 `next`

`list` 默认显示 pending 任务。`next` 在 M0 阶段可以复用 `list` 能力，但默认排序不同：

- `list`：`status=pending`，按 `entry ASC` 排序。
- `next`：`status=pending`，按 `due ASC NULLS LAST`、`priority DESC`、`entry ASC` 排序。

命令名前支持的 filter 示例：

```bash
taskg +work list
taskg project:taskg list
taskg status:completed list
taskg /spec/ list
```

M0 filter 只支持 AND 组合。布尔操作符和括号属于 M1。

### `modify`

支持输入：

```bash
taskg 1 modify priority:H due:eow +next
taskg <uuid> modify project:taskg
```

规则：

- 目标可以是数字 working-set ID 或 UUID。
- 至少需要一个 modification。
- 修改后更新 `modified`。
- 可修改 `description`、`project`、`priority`、`due` 和 tags。

description 替换语法：

```bash
taskg 1 modify description:"New description"
```

`append` 和 `prepend` 是后续命令，不进入 M0。

### `done` 与 `delete`

规则：

- 目标可以是数字 working-set ID 或 UUID。
- `done` 设置 `status=completed`、`end_ts=now`、`modified=now`。
- `delete` 设置 `status=deleted`、`end_ts=now`、`modified=now`。
- M0 不执行硬删除。

### `info`

展示单个任务的所有 M0 字段，包括 UUID 和 tags。JSON 模式返回单个对象。

### `config` 与 `show`

M0 的配置能力保持克制：

- `taskg show`：打印解析后的配置值。
- `taskg config get <key>`：读取配置值。
- `taskg config set <key> <value>`：写入用户级配置，可先落在 SQLite `meta` 表或配置文件中。

初始支持 key：

- `database.path`
- `color`
- `date.format`

完整 Taskwarrior `.taskrc` 兼容不属于 M0。

### `import` 与 `export`

`export` 向 stdout 输出 JSON array。字段名尽量与 Taskwarrior 保持一致：

- `uuid`
- `description`
- `status`
- `entry`
- `modified`
- `end`
- `due`
- `project`
- `priority`
- `tags`

JSON 中的日期使用 ISO-8601/RFC3339 字符串。`import` 支持从 stdin 或文件路径读取 JSON array，并按 UUID upsert。

## 7. 日期解析

M0 支持一组较小但实用的日期表达：

- RFC3339 timestamp。
- `YYYY-MM-DD`。
- `today`。
- `tomorrow`。
- `<N>days`，例如 `2days`。
- `eod`、`eow`、`eom`。

数据库内统一存 Unix seconds UTC。日期关键字按本地时区解释。

## 8. 错误处理

CLI 错误需要简洁，并适合脚本调用：

- 人类模式：stderr 输出短错误，返回非零 exit code。
- JSON 模式：stderr 输出结构化错误对象。
- stdout 只输出命令结果，不混入诊断信息。

推荐 exit code：

- `0`：成功。
- `1`：校验错误或运行时错误。
- `2`：命令行用法错误。
- `3`：任务不存在。

错误示例：

```text
taskg: no task matches "42"
taskg: unsupported priority "X"; expected H, M, or L
taskg: description is required
```

## 9. 测试策略

后续实施计划应使用 TDD。

### 单元测试

- 任务校验与生命周期转换。
- modification 解析。
- filter 解析。
- 日期解析。
- JSON import/export 转换。
- 配置路径解析。

### 存储测试

- migration 能创建 schema。
- 初始化会创建本地 workspace。
- 创建、更新、查询、完成、删除任务。
- tags 在事务内整体替换。
- import 按 UUID upsert。

### CLI 集成测试

使用临时数据库运行构建出的二进制：

- `add` 后 `list` 能看到任务。
- `modify` 能更新 priority、tag、project。
- `done` 后任务从默认 `list` 消失，但可通过 `status:completed` 查询。
- `delete` 执行软删除。
- `export` 后导入到新数据库，字段保持一致。
- `--json` 输出为合法 JSON。

### 跨平台检查

至少运行：

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test ./...
```

后续实施计划可以补充 Darwin 和 Windows 的 build-only 检查。

## 10. 后续扩展点

M0 需要为后续里程碑保留这些清晰边界：

- `internal/query` 暴露类型化 filter，方便 M1 将解析器替换为 AST。
- `internal/app` 接收 workspace ID，即使 M0 永远使用 local workspace。
- storage schema 从第一天起包含 `workspace_id`。
- JSON import/export 避免使用只适合 M0 的 DTO 命名，以免未来破坏 Taskwarrior 兼容。
- CLI 命令只调用 app service，不直接访问 repository。
- app/service 测试中应能注入当前时间。

## 11. 验收标准

M0 完成时应满足：

- `taskg` 可以构建为单一 CGO-free 二进制。
- 首次运行命令时自动初始化本地数据库。
- `add`、`list`、`next`、`modify`、`done`、`delete`、`info`、`config`、`show`、`import`、`export` 在人类模式下可用。
- 同一组核心命令支持 `--json`。
- 数字 working-set ID 和 UUID 都能定位任务。
- 导出的 JSON 可以导入全新的 M0 数据库。
- `go test ./...` 通过。
- `CGO_ENABLED=0 go test ./...` 通过。
- 实现完成后，README 补充简短 M0 使用说明。

## 12. 后续里程碑的开放问题

这些问题不阻塞 M0：

- M1 是否一次性实现完整 Taskwarrior filter parser，还是继续增量扩展。
- op-log 应该在 HTTP/MCP 前成为权威数据源，还是之后再切换。
- import/export 是否需要把暂不支持的字段保存在辅助 JSON 列中。
- `config set` 应该写 TOML 文件、SQLite meta row，还是两者都写。
- 完整 urgency 引擎落地前，`next` 是否需要提供一个轻量 urgency 近似排序。

## 13. 后续计划安排

M0 完成后，项目应按可独立验证的里程碑继续推进。每个里程碑开始前都应先写对应中文 spec，再用 `superpowers:writing-plans` 拆成实施计划；不要直接从本节开始编码。

### M1：查询语言、报表、Urgency 与 DOM 基础

**目标：** 让 `taskg` 从“能记录任务”升级为“能像 Taskwarrior 一样查询、排序和解释任务”。

**前置条件：**

- M0 CLI、SQLite 存储和 JSON 输出稳定。
- 数字 working-set ID 解析规则已被测试覆盖。
- `internal/query` 已经有可替换的 filter 边界。

**主要交付：**

- 完整度更高的 filter parser：属性匹配、日期比较、正则、`and/or/xor/not`、括号。
- 报表系统：`all`、`completed`、`waiting`、`active`、`ready`、`overdue`、`blocked`、`blocking`。
- Urgency 计算包 `internal/urgency`，复刻默认系数，并支持 explain 输出。
- DOM 基础包 `internal/dom`，支撑 `_get`、`_ids`、`_uuids`、`_projects`、`_tags`。
- `calc` 表达式求值器的第一版，可复用于 filter 和 DOM。

**验收信号：**

- 常见 Taskwarrior 查询示例能通过测试。
- `next` 使用 urgency 排序。
- `taskg _get 1.description`、`taskg _ids`、`taskg _tags` 可脚本化使用。
- 报表输出在 human 和 `--json` 模式下都可用。

### M2：任务细节模型补齐

**目标：** 补齐 Taskwarrior 核心任务能力，让任务模型可以承载真实日常使用。

**前置条件：**

- M1 query、report、urgency 已稳定。
- 数据迁移机制可重复运行，并有回归测试。

**主要交付：**

- UDA：schema 定义、值校验、orphan UDA 保留策略。
- annotations：`annotate`、`denotate`、导入导出。
- dependencies：依赖增删、blocked/blocking 报表与 urgency 联动。
- start/stop：active 状态字段与报表。
- recurring：`recur`、`parent`、`mask`、`imask` 与子任务生成。
- `append`、`prepend`、`edit`。
- `_udas`、`_unique`、`_urgency` helper。

**验收信号：**

- annotations、dependencies、UDA 能完整导入导出。
- blocked/blocking 与 urgency explain 结果一致。
- recurring 任务生成行为有确定测试。
- `edit` 能安全处理无效输入和保存失败。

### M3：多用户、多 Workspace 与 HTTP API

**目标：** 把本地任务引擎升级为服务端可用的多租户系统。

**前置条件：**

- M0-M2 的核心 domain service 不依赖 CLI。
- 所有任务查询和写入都已经显式接收 workspace ID。
- schema 已经包含或可迁移到 users、workspaces、memberships、api_tokens。

**主要交付：**

- users、workspaces、memberships、api_tokens、audit_logs 表与迁移。
- owner/admin/member/viewer 权限模型。
- PAT 与 JWT 鉴权。
- HTTP/JSON API，提供 OpenAPI 3 描述。
- 远程 CLI 模式：`taskg --server ... --token ...`。
- 行级隔离守卫：repository 或 service 层强制 workspace scope。
- 管理命令：`user`、`workspace`、`member`、`token`。

**验收信号：**

- 不同 workspace 的任务互不可见。
- viewer 无法写入，member 无法管理成员。
- 远程 CLI 与本地 CLI 在核心命令上的输出一致。
- API 错误结构稳定，便于 CLI 和后续 MCP 复用。

### M4：MCP Server、Webhook 与飞书触发示例

**目标：** 让 AI Agent 和外部系统可以通过稳定工具接口使用 taskg。

**前置条件：**

- M3 HTTP API 和鉴权稳定。
- app service 已经与传输层解耦。
- JSON schema 和错误模型稳定。

**主要交付：**

- MCP Server，支持 stdio 和 Streamable HTTP。
- MCP tools：`task.add`、`task.modify`、`task.done`、`task.delete`、`task.query`、`task.get`、`task.annotate`、`task.depends`、`task.start`、`task.stop`、`report.run`、`urgency.explain`、`workspace.list`、`workspace.switch`、`context.set`、`context.show`、`config.get`、`config.set`。
- 每个 MCP tool 返回 `{data, rendered}`。
- Webhook 触发入口。
- 飞书事件到任务动作的示例集成。
- workspace 级“团队记忆/偏好”挂载点设计。

**验收信号：**

- MCP stdio 可被本地 Agent 调用。
- MCP HTTP 传输可鉴权并限制 workspace。
- 飞书示例能把事件转为任务，并将结果回写或静默入库。
- MCP tool schema 有测试，避免参数漂移。

### M5：Operation Log 同步与 Hook 系统

**目标：** 让 taskg 支持多端收敛、离线写入和事件扩展。

**前置条件：**

- M3/M4 的写路径已经集中在 app service。
- 所有任务修改都能被表达为原子变更。
- 当前物化表和未来 operations 表之间的关系已明确。

**主要交付：**

- `operations` 权威写入模型或双写过渡模型。
- replica ID、op ID、parent op、actor、old/new value 记录。
- `sync` 命令：拉取、推送、冲突处理、断点续传。
- 本地脚本 Hook：`on-launch`、`on-add`、`on-modify`、`on-exit`。
- 服务端 Webhook：超时、重试、签名校验。
- WASM/插件式 Hook 只做设计或实验，不作为必须交付。

**验收信号：**

- 两个本地 replica 离线各自修改后可以收敛。
- 并发添加 tag 不丢失更新。
- Hook 拒绝修改时，任务写入能回滚并返回清晰错误。
- sync 和 hook 有端到端测试。

### M6：兼容性、迁移与发布打磨

**目标：** 把 taskg 从“功能可用”打磨成“可迁移、可发布、可长期维护”的工具。

**前置条件：**

- 核心功能已经跨本地、远程、MCP 三种入口稳定。
- import/export 已覆盖 M0-M5 的字段。
- 配置系统已经支持用户级和 workspace 级合并。

**主要交付：**

- Taskwarrior JSON 双向导入导出增强，尽量无损。
- `.taskrc` 只读导入与兼容报告。
- `backup`：SQLite `VACUUM INTO` + JSON 导出。
- shell completion：zsh、bash、fish、powershell。
- release 构建矩阵：linux/darwin/windows，amd64/arm64。
- 文档：安装、迁移、CLI 手册、API/MCP 手册、运维手册。
- 性能与可靠性测试：大任务量查询、SQLite WAL、并发写入、备份恢复。

**验收信号：**

- 真实 Taskwarrior export 样本可导入，并给出兼容性报告。
- `taskg export` 的结果可被后续版本稳定读取。
- 发布产物均为 CGO-free。
- 新用户能只靠 README 和 CLI help 完成安装、添加任务、迁移和备份。

### 跨里程碑工作规则

- 每个里程碑都先写独立 spec，再写 implementation plan。
- 每个 implementation plan 都按 TDD 拆分任务，并包含明确测试命令。
- 每个里程碑结束时更新 README 和 `docs/requirements.md` 的状态。
- 数据迁移必须向前兼容；除非明确进入破坏性版本，否则不能要求用户手工清库。
- 每次新增传输入口时，都必须复用 app service，不复制业务逻辑。
- CLI、HTTP、MCP 的 JSON 字段语义要保持一致。
- 涉及权限、同步、Hook 的改动必须有端到端测试。

## 14. 审阅说明

本文档所在目录当前不是 git 仓库，因此无法在此处提交。由于当前工具策略只允许在用户明确要求 delegation 时派发 subagent，本文档没有使用 subagent review，而是根据 [docs/requirements.md](/Users/mac/code/projects/dajee/task/docs/requirements.md) 做了本地自审，并刻意将范围限制在 M0。
