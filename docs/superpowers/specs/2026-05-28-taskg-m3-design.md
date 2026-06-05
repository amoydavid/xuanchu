# xuanchu M3 设计规格

> **给 agentic workers 的要求：** 编码前必须先使用 `superpowers:writing-plans` 将本文档拆成实施计划。不要直接从本规格开始写代码。

**目标：** 在 M2 已补齐核心任务模型后，完成 Taskwarrior 风格的长期使用能力：配置系统升级、context、UDA、`.taskrc` 只读导入、脚本化 helper 补齐和 shell completion。

**范围策略：** M3 是“兼容性与个性化”里程碑，不引入多 workspace、多用户、HTTP API 或 MCP。实现必须继续复用 M0-M2 的 CLI、app service、query AST、JSON、DOM/helper 和 SQLite/GORM 分层。

**需求来源：** 本规格从 [ROADMAP.md](/Users/mac/code/projects/dajee/task/ROADMAP.md)、[README.md](/Users/mac/code/projects/dajee/task/README.md)、[docs/requirements.md](/Users/mac/code/projects/dajee/task/docs/requirements.md)、M2 当前实现和 Taskwarrior 兼容目标中收束。

---

## 1. 当前基础

M2 已完成并合并到 `main`。当前项目已有：

- Go 1.22 module。
- Cobra CLI。
- GORM + `github.com/glebarez/sqlite`，保持零 CGO。
- 隐式 `local` workspace。
- 核心任务生命周期、查询 AST、报表、urgency、DOM/helper、calc。
- M2 字段：
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
- M2 命令：
  - `start`
  - `stop`
  - `annotate`
  - `denotate`
  - `append`
  - `prepend`
  - `edit`
- M2 报表：
  - `waiting`
  - `active`
  - `ready`
  - `blocked`
  - `blocking`
- JSON import/export 已覆盖 M0-M2 字段。

M3 不重写这些基础设施，而是在已有结构上扩展配置、context、UDA 和脚本接口。

## 2. M3 产品体验

M3 完成后，应支持这些使用方式：

```bash
# TOML 配置与临时 rc 覆盖
xuanchu show
xuanchu config get date.format
xuanchu config set date.format rfc3339
xuanchu rc.date.format=epoch list

# context
xuanchu context define work 'project:work status:pending'
xuanchu context use work
xuanchu list
xuanchu --no-context list
xuanchu context show
xuanchu context none

# UDA schema 与值
xuanchu config set uda.estimate.type numeric
xuanchu config set uda.estimate.label "Estimate"
xuanchu add "Implement API" estimate:3
xuanchu estimate:3 list
xuanchu _get 1.estimate
xuanchu _udas
xuanchu _unique estimate

# .taskrc 只读导入
xuanchu config import-taskrc ~/.taskrc --dry-run
xuanchu config import-taskrc ~/.taskrc

# completion
xuanchu completion zsh > ~/.zfunc/_xuanchu
xuanchu completion bash
```

M3 仍需保持脚本友好：

- stdout 只输出结果。
- stderr 输出错误。
- helper 命令无装饰、每行一个值。
- `--json` 输出稳定结构。
- `.taskrc` import 的兼容性报告可 human 输出，也可 JSON 输出。

## 3. 分层范围

### Layer 1：配置系统升级

必须进入 M3：

- 支持 `~/.config/xuanchu/xuanchu.toml`。
- 支持 `XDG_CONFIG_HOME`。
- 继续支持 `--db`、`--data-dir`、`XUANCHU_DB`、`XDG_DATA_HOME`。
- 支持 Taskwarrior 风格临时覆盖参数：
  - `rc.<key>=<value>`
  - `rc.<key>:` 或 `rc.<key>=` 表示本次运行清空该 key。
- `config get`、`config set`、`config unset`、`config list`。
- `show` 输出最终合并后的关键配置。

### Layer 2：Context

必须进入 M3：

- `context define <name> <filter...>`
- `context use <name>`
- `context none`
- `context show`
- `context list`
- `context delete <name>`
- active context 自动叠加到读路径查询和报表。
- 提供 `--no-context` 全局 flag 绕过 active context。

### Layer 3：UDA

必须进入 M3：

- UDA schema 定义。
- 支持类型：
  - `string`
  - `numeric`
  - `date`
  - `duration`
- 支持枚举值校验。
- UDA 写入、清空、查询、DOM、JSON import/export。
- orphan UDA 保留但普通 modify 不允许修改。
- `_udas`、`_unique` helper 读取 UDA。

### Layer 4：`.taskrc` 只读导入

必须进入 M3：

- 解析常见 Taskwarrior `.taskrc` key。
- 能导入常见配置、context 和 UDA schema。
- 未识别 key 不报 fatal error，应进入兼容性报告。
- 支持 `--dry-run`。
- 支持 `--json` 输出报告。

### Layer 5：Helper 与 completion 补齐

必须进入 M3：

- `_udas`
- `_unique`
- `_show`
- `_version`
- `completion bash|zsh|fish|powershell`

## 4. 非目标

M3 不做：

- 多 workspace。
- 多用户。
- HTTP API。
- MCP Server。
- PAT/JWT 鉴权。
- op-log sync。
- Hook。
- Webhook。
- `.taskrc` 完整兼容。
- 自定义 report DSL 的完整导入。
- Taskwarrior calendar / burndown。

这些留给 M4+。

## 5. 配置系统

### 配置来源

M3 引入 TOML 文件：

```text
~/.config/xuanchu/xuanchu.toml
```

如果 `XDG_CONFIG_HOME` 存在，则使用：

```text
$XDG_CONFIG_HOME/xuanchu/xuanchu.toml
```

建议 TOML 结构：

```toml
[database]
path = "/path/to/xuanchu.db"

[display]
color = true
json = false

[date]
format = "rfc3339"

[context]
active = "work"
```

UDA schema 可使用平铺 key 或 TOML table。M3 推荐内部规范为平铺 key，TOML 只是其中一种来源：

```toml
[uda.estimate]
type = "numeric"
label = "Estimate"
values = ["1", "2", "3", "5", "8"]

[uda.reviewed]
type = "date"
label = "Reviewed"
```

### 配置优先级

数据库路径是特殊配置，因为必须先解析路径才能打开 SQLite。路径优先级：

1. `--db`
2. `XUANCHU_DB`
3. `--data-dir`
4. TOML `database.path`
5. `XDG_DATA_HOME/xuanchu/xuanchu.db`
6. `~/.local/share/xuanchu/xuanchu.db`

非路径配置优先级：

1. CLI flag。
2. 本次运行的 `rc.<key>=<value>` 覆盖。
3. 环境变量。
4. SQLite meta 中由 `xuanchu config set` 写入的值。
5. TOML 文件。
6. 默认值。

选择 SQLite meta 高于 TOML，是为了保留 M0-M2 已有 `xuanchu config set` 行为：用户通过命令显式设置的值，不会被较早写入的 TOML 默认值悄悄覆盖。

### rc 临时覆盖

M3 支持 Taskwarrior 风格临时覆盖：

```bash
xuanchu rc.date.format=epoch list
xuanchu rc.context=none list
xuanchu rc.color=false next
```

规则：

- `rc.` 参数可出现在 root args 中，执行前由 CLI root 解析并从 args 中移除。
- `rc.context=none` 等价于本次运行禁用 active context。
- `rc.<key>=<value>` 只影响本次命令，不写入 TOML 或 SQLite meta。
- 未知 `rc` key 返回非零 exit code。

### config 命令

M3 扩展：

```bash
xuanchu config get <key>
xuanchu config set <key> <value>
xuanchu config unset <key>
xuanchu config list
xuanchu config import-taskrc <path> [--dry-run]
```

规则：

- `config set` 继续写入 SQLite meta 或配置专用表，不直接编辑 TOML 文件。
- `config unset` 删除 SQLite meta 覆盖；如果 TOML 中仍有同 key，最终值会回退到 TOML。
- `config list` 输出最终合并后的 key/value，每行 `key=value`。
- `show` 面向人类，`_show` 面向脚本。

## 6. Context

### 数据模型

建议新增 SQLite 表：

```sql
CREATE TABLE contexts (
  workspace_id TEXT NOT NULL,
  name TEXT NOT NULL,
  filter_source TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  modified_at INTEGER NOT NULL,
  PRIMARY KEY (workspace_id, name)
);
```

M3 仍然只有隐式 `local` workspace，但 context 表必须带 `workspace_id`，为 M4 多 workspace 做准备。

active context 可以存入 SQLite meta：

```text
context.active=<name>
```

### 命令

```bash
xuanchu context define work 'project:work status:pending'
xuanchu context use work
xuanchu context none
xuanchu context show
xuanchu context list
xuanchu context delete work
```

规则：

- `context define` 必须调用 M1 query parser 校验 filter。
- `context use` 只能使用已定义 context。
- `context none` 清空 active context。
- `context show` 输出当前 active context 名称和 filter；没有 active context 时输出空或 `none`，implementation plan 需固定。
- `context delete` 如果删除 active context，应同时清空 active context。

### 生效范围

active context 自动叠加到这些读路径：

- `list`
- `next`
- `all`
- `completed`
- `deleted`
- `overdue`
- `waiting`
- `active`
- `ready`
- `blocked`
- `blocking`
- `_ids`
- `_uuids`
- `_projects`
- `_tags`
- `_unique`

active context 不影响这些路径：

- `add`
- `modify`
- `done`
- `delete`
- `start`
- `stop`
- `annotate`
- `denotate`
- `append`
- `prepend`
- `edit`
- `info <target>`
- `export`
- `import`
- `config`
- `context`
- `_get <target.field>`
- `_show`
- `_version`
- `completion`

原因：target mutation 应该能操作用户明确指定的任务，不应因为 context 隐藏而找不到。导入导出默认应处理全量数据，避免备份不完整。

### 绕过 context

M3 新增全局 flag：

```bash
--no-context
```

规则：

- `--no-context` 只影响本次运行。
- `rc.context=none` 与 `--no-context` 等价。
- `context none` 是持久化清空 active context。

## 7. UDA

### Schema

建议新增 SQLite 表：

```sql
CREATE TABLE uda_definitions (
  workspace_id TEXT NOT NULL,
  name TEXT NOT NULL,
  type TEXT NOT NULL,
  label TEXT,
  values_json TEXT,
  default_value TEXT,
  created_at INTEGER NOT NULL,
  modified_at INTEGER NOT NULL,
  PRIMARY KEY (workspace_id, name)
);
```

字段规则：

- `name`：
  - 非空。
  - 不能与内置属性冲突：`uuid`、`description`、`status`、`entry`、`modified`、`end`、`due`、`start`、`wait`、`scheduled`、`until`、`project`、`priority`、`depends`、`annotations`、`recur`、`parent`、`tag`。
  - 建议允许点号命名，例如 `dev.github.issue`。
- `type`：
  - `string`
  - `numeric`
  - `date`
  - `duration`
- `values_json`：
  - 可选枚举。
  - 非空时写入值必须属于枚举。

### Value 存储

建议新增 SQLite 表：

```sql
CREATE TABLE task_uda_values (
  task_uuid TEXT NOT NULL,
  name TEXT NOT NULL,
  value TEXT NOT NULL,
  value_type TEXT,
  orphan BOOLEAN NOT NULL DEFAULT false,
  PRIMARY KEY (task_uuid, name)
);
```

domain 建议扩展：

```go
type UDAValue struct {
	Name   string
	Value  string
	Type   string
	Orphan bool
}

type Task struct {
	// existing fields...
	UDAs map[string]UDAValue
}
```

M3 先以 `value TEXT` 保存用户输入的规范化字符串。查询时按 schema type 做解析和比较；如后续需要性能优化，可在 M4+ 增加 normalized numeric/date columns。

### 修改语法

M3 支持：

```bash
xuanchu add "Implement API" estimate:3
xuanchu 1 modify estimate:5
xuanchu 1 modify estimate:
xuanchu uda.estimate:3 list
xuanchu estimate:3 list
```

规则：

- `uda.<name>:<value>` 永远按 UDA 解析。
- `<name>:<value>` 如果 `<name>` 不是内置属性，且当前 workspace 有同名 UDA schema，则按 UDA 解析。
- 未定义 UDA 的 `<name>:<value>` 继续报 unknown attribute，不自动创建 schema。
- `<name>:` 清空该 UDA 值。
- orphan UDA 只能在 `edit` 或 JSON import 中被保留/清空，普通 `modify` 不允许修改。

### 类型校验

`string`：

- 任意非换行字符串。
- 空值用于清空，不作为值保存。

`numeric`：

- 支持整数和小数。
- 内部保存用户规范化后的十进制字符串。
- 查询支持 `eq`、`before`、`after`，分别对应 `=`、`<`、`>`。

`date`：

- 复用 M1/M2 日期 parser。
- 写入时保存 RFC3339 UTC 或 Unix 秒，implementation plan 必须固定一种；推荐保存 RFC3339 UTC，JSON 也更接近 Taskwarrior。
- 查询支持 `eq` 自然日范围、`before`、`after`。

`duration`：

- 支持 `1h`、`30min`、`2days`、`1w` 等常见形式。
- 内部保存秒数的十进制字符串。
- 查询支持数值比较。

### Query

query AST 需要支持动态 UDA 属性：

- parser 遇到未知 attribute 时，不应立即报错；应生成 `AttrUDA` 或 `DynamicAttribute`，由 app/service 层结合当前 UDA schema 判断。
- compile 阶段必须拿到 UDA schema，所有值使用参数绑定。
- `uda.<name>:` 或 `<name>:` 表示 UDA 值不存在。
- `uda.<name>.notnull` 表示 UDA 值存在。

建议 SQL 形态：

```sql
EXISTS (
  SELECT 1 FROM task_uda_values
  WHERE task_uuid = tasks.uuid AND name = ? AND value = ?
)
```

M3 不要求 UDA 查询支持复杂正则；字符串 UDA 的 `/x/` 与 description 一样按子串匹配。

### JSON

Taskwarrior UDA 在 JSON 中表现为 top-level field。M3 采用同样规则：

```json
{
  "uuid": "...",
  "description": "Implement API",
  "status": "pending",
  "estimate": "3",
  "reviewed": "2026-05-28T00:00:00Z"
}
```

导出：

- 已定义 UDA 作为 top-level field 输出。
- orphan UDA 也作为 top-level field 输出，保证往返不丢失。
- 如果 UDA 名与 core JSON 字段冲突，禁止定义该 UDA。

导入：

- 已定义 UDA：按 schema 校验。
- 未定义 top-level field：
  - 如果不是 core field，作为 orphan UDA 保留。
  - scalar 值保存为字符串。
  - object/array 值保存为 compact JSON 字符串，并标记 orphan。
- orphan UDA 不参与普通查询类型校验，但可用 `uda.<name>.notnull` 查询是否存在。

### DOM / helper / urgency

DOM：

```bash
xuanchu _get 1.estimate
xuanchu _get 1.uda.estimate
```

规则：

- 已定义和 orphan UDA 都可通过 `_get` 读取。
- 不存在输出空字符串，不报错。

helper：

- `_udas` 输出已定义 UDA 名称，每行一个。
- `_unique <attr> [filters...]` 输出某属性的唯一值，每行一个，按字典序排序。
- `_unique estimate` 支持 UDA。

urgency：

- M3 支持 UDA urgency 配置：
  - `urgency.uda.<name>.coefficient`
  - `urgency.uda.<name>.<value>.coefficient`
- 如果任务有该 UDA 且值匹配，加入 urgency explain。
- 不配置时 UDA 不影响 urgency。

## 8. `.taskrc` 只读导入

### 命令

```bash
xuanchu config import-taskrc ~/.taskrc
xuanchu config import-taskrc ~/.taskrc --dry-run
xuanchu config import-taskrc ~/.taskrc --json
```

### 解析范围

M3 应导入：

- `color`
- `dateformat`
- `context.<name>`
- `uda.<name>.type`
- `uda.<name>.label`
- `uda.<name>.values`
- `uda.<name>.default`
- `urgency.uda.<name>.coefficient`
- `urgency.uda.<name>.<value>.coefficient`

M3 可识别但暂不导入：

- `data.location`：M3 只在启动前通过 `--db`、`XUANCHU_DB`、`--data-dir` 或 TOML 决定数据库路径，`.taskrc` import 不改写运行时数据库路径
- `report.<name>.*`
- `calendar.*`
- `burndown.*`
- `news.*`
- `sync.*`
- `hooks.*`

这些应写入 skipped 或 unsupported report，而不是报 fatal error。

### 报告格式

human 输出建议：

```text
imported:
  date.format
  context.work
  uda.estimate.type
skipped:
  report.next.columns (custom reports are not supported in M3)
unknown:
  foo.bar
```

JSON 输出建议：

```json
{
  "imported": [{"key":"date.format","target":"date.format"}],
  "skipped": [{"key":"report.next.columns","reason":"custom reports are not supported in M3"}],
  "unknown": [{"key":"foo.bar","reason":"unknown key"}],
  "dry_run": true
}
```

规则：

- `--dry-run` 不写入 SQLite meta、contexts 或 UDA schema。
- import 不应删除已有 config/context/UDA。
- 冲突时默认覆盖 SQLite meta 中的同 key；implementation plan 可增加 `--no-overwrite`，但 M3 spec 不要求。

## 9. Helper 补齐

### `_udas`

```bash
xuanchu _udas
```

输出已定义 UDA 名称，每行一个，按字典序。

### `_unique`

```bash
xuanchu _unique project
xuanchu _unique tags
xuanchu _unique estimate project:work
```

规则：

- 第一个参数是属性名。
- 后续参数是 filter。
- active context 默认生效；`--no-context` 可绕过。
- 输出唯一值，每行一个，按字典序。
- 空值不输出。

支持属性：

- `project`
- `priority`
- `tags`
- 已定义 UDA
- M2 日期字段可选进入，implementation plan 根据复杂度决定；如果进入，输出 RFC3339。

### `_show`

```bash
xuanchu _show
xuanchu _show date.format context.active
```

规则：

- 无参数：输出最终合并配置，每行 `key=value`。
- 有参数：按参数顺序输出 value，每行一个。
- 未知 key 返回非零 exit code。

### `_version`

```bash
xuanchu _version
```

输出当前版本字符串。M3 可以先输出：

```text
xuanchu 0.3.0
```

如果构建时没有注入版本，使用 `dev`：

```text
xuanchu dev
```

## 10. Shell completion

M3 使用 Cobra 内置 completion：

```bash
xuanchu completion bash
xuanchu completion zsh
xuanchu completion fish
xuanchu completion powershell
```

规则：

- completion 输出到 stdout。
- 不自动写用户 shell 配置。
- completion 命令不打开数据库。
- M3 不要求动态补全项目、tag、UDA；只要求命令、flag 和静态参数补全。

## 11. CLI Root 与 rc 参数

M3 的 root 参数重排需要识别 `rc.<key>=<value>`：

```bash
xuanchu rc.context=none +next list
xuanchu +next rc.date.format=epoch list
```

规则：

- `rc.*` token 不应进入 query parser。
- root reorder 后仍保留原有两种入口：
  - `xuanchu <subcommand> ...`
  - `xuanchu <target> <action> ...`
- 如果 `rc.*` 与 `--no-context` 同时出现，`--no-context` 和 `rc.context=none` 等价，不冲突。
- 如果 `rc.context=<name>` 指向不存在的 context，应返回错误。

## 12. 存储与分层

M3 建议新增 packages：

- `internal/config`
  - 扩展 TOML file loading、merge、rc override。
- `internal/taskcontext`
  - context domain model 和 filter validation helper。
- `internal/uda`
  - UDA schema、value validation、type parsing。
- `internal/taskrc`
  - `.taskrc` parser 和 import report。

也可根据 implementation plan 合并为更小步改造，但必须遵守：

- `internal/cli` 不放业务规则。
- `internal/app` 负责拼装 config/context/UDA/query。
- `internal/storage` 只做持久化。
- `internal/query` 不直接访问 SQLite 或 config。
- UDA dynamic attribute 的 schema 解析应由 app 层注入 compile options。

## 13. 错误处理

必须有清晰错误：

- unknown config key。
- invalid context filter。
- context not found。
- UDA type invalid。
- UDA value invalid。
- UDA enum violation。
- modifying orphan UDA is not allowed。
- `.taskrc` 文件不存在或无法读取。
- `.taskrc` 行语法错误。

脚本友好规则：

- 错误写 stderr。
- stdout 不混入提示文本。
- helper 命令遇错返回非零 exit code。
- `--json` 下错误结构如果当前项目已有统一格式则复用；若没有，M3 不强制新增全局错误 JSON。

## 14. JSON Import / Export

M3 的 JSON import/export 需要继续满足：

- M0-M2 字段往返不退化。
- UDA top-level field 往返不丢失。
- orphan UDA 往返不丢失。
- 已定义 UDA 导入时必须校验。
- 未定义 UDA 导入为 orphan，不自动创建 schema。
- 导出顺序不作为语义要求，但测试应解析 JSON 后比较结构，不依赖字段顺序。

## 15. 测试策略

### 单元测试

必须覆盖：

- TOML config 读取和合并优先级。
- `rc.<key>=<value>` 解析。
- context define/use/none/delete。
- context filter 与用户 query/report 的 AND 合并。
- `--no-context` 绕过。
- UDA schema validation。
- UDA string/numeric/date/duration value validation。
- UDA query compile。
- orphan UDA import/export。
- `.taskrc` parser imported/skipped/unknown 报告。
- helper `_udas`、`_unique`、`_show`、`_version`。
- completion 命令不打开数据库。

### 集成测试

必须覆盖：

```bash
xuanchu context define work 'project:work'
xuanchu context use work
xuanchu add "work task" project:work
xuanchu add "home task" project:home
xuanchu list
xuanchu --no-context list
```

```bash
xuanchu config set uda.estimate.type numeric
xuanchu add "estimated" estimate:3
xuanchu estimate:3 list
xuanchu _get 1.estimate
xuanchu _unique estimate
```

```bash
xuanchu config import-taskrc ./testdata/taskrc --dry-run --json
xuanchu config import-taskrc ./testdata/taskrc
xuanchu _udas
xuanchu context list
```

```bash
xuanchu completion zsh
xuanchu _version
```

### 必跑验收

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```

还必须确认：

```bash
if go list -m all | grep -E 'gorm.io/driver/sqlite|mattn/go-sqlite3'; then
  echo "unexpected CGO SQLite dependency" >&2
  exit 1
fi
```

## 16. 文档更新

M3 完成时必须更新：

- [README.md](/Users/mac/code/projects/dajee/task/README.md)
  - 增加 M3 配置、context、UDA、`.taskrc`、helper、completion 用法。
- [ROADMAP.md](/Users/mac/code/projects/dajee/task/ROADMAP.md)
  - 将 M3 标记为已完成。
  - 将下一步指向 M4 多 workspace。
- 如 implementation plan 对本 spec 做范围调整，必须同步更新本 spec。

## 17. 验收标准

M3 完成时应满足：

- M0/M1/M2 全部测试继续通过。
- `xuanchu.toml` 可读取，且与 SQLite meta、env、CLI flag、rc override 按规则合并。
- `context define/use/none/show/list/delete` 可用。
- active context 自动影响读路径，并可用 `--no-context` 绕过。
- UDA schema 可定义，UDA 值可写入、清空、查询、导入导出。
- orphan UDA 可保留、导出、读取，但普通 modify 不允许修改。
- `.taskrc` import 能生成 imported/skipped/unknown 报告，`--dry-run` 不写库。
- `_udas`、`_unique`、`_show`、`_version` 输出稳定、可脚本解析。
- `completion bash|zsh|fish|powershell` 可输出 completion 脚本。
- `go test ./...`、`CGO_ENABLED=0 go test ./...`、`CGO_ENABLED=0 go build ./cmd/xuanchu` 通过。

## 18. 后续衔接

M3 完成后，M4 应聚焦：

- 多 workspace。
- 本地团队模型。
- membership。
- 权限边界。
- workspace 隔离下的 context/UDA/config。

M3 的配置、context 和 UDA schema 必须为 M4 预留 `workspace_id` 边界，但 M3 不暴露多 workspace UI。

## 19. 审阅说明

本文档根据当前 M2 实现、[ROADMAP.md](/Users/mac/code/projects/dajee/task/ROADMAP.md)、[README.md](/Users/mac/code/projects/dajee/task/README.md)、[AGENTS.md](/Users/mac/code/projects/dajee/task/AGENTS.md) 和 [docs/requirements.md](/Users/mac/code/projects/dajee/task/docs/requirements.md) 编写。

当前未执行 spec-document-reviewer subagent 审阅；本环境虽提供通用 multi-agent 工具，但未提供明确的 spec-document-reviewer 角色或提示文件，也没有 `spec-document-reviewer-prompt.md` 可引用。后续写 implementation plan 前，如需要严格执行 superpowers 审阅环节，请用专门 reviewer prompt 对本文档进行审阅。
