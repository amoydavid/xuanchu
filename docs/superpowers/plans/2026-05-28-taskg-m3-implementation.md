# taskg M3 Implementation Plan

> **For agentic workers:** REQUIRED: Use `superpowers:subagent-driven-development` (if subagents available) or `superpowers:executing-plans` to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 完成 taskg M3：配置系统升级、context、UDA、`.taskrc` 只读导入、脚本化 helper 补齐和 shell completion。

**Architecture:** M3 在 M0-M2 已有 CLI / app / query / storage / JSON / DOM 分层之上继续增量扩展，不重写现有任务核心。实现时把“配置来源与合并”“context 过滤”“UDA schema 与值”“`.taskrc` 导入”“helper/completion”拆成独立边界，让 CLI 只负责参数路由，app 负责用例编排，storage 只负责持久化，query/DOM/urgency 只接收结构化输入。

**Tech Stack:** Go 1.22、Cobra、GORM、`github.com/glebarez/sqlite`、`github.com/pelletier/go-toml/v2`、标准库 `encoding/json` / `text/scanner` / `time` / `sort` / `os/exec`、Go test。M3 不新增 SQLite driver，不引入 CGO。

**Delivery:** 建议拆成 6 个可合并点：Chunk 1+2、Chunk 3、Chunk 4、Chunk 5、Chunk 6、Chunk 7。每个合并点都必须能单独验证，并满足 `go test ./...`、`CGO_ENABLED=0 go test ./...`、`CGO_ENABLED=0 go build ./cmd/taskg` 的要求。

---

## Chunk 1: 配置系统地基与 rc 覆盖

### 文件职责

- Modify: `internal/config/config.go`
  - 拆出数据库路径解析、运行时配置合并、键值视图。
- Create: `internal/config/runtime.go`
  - 运行时配置快照、合并规则、typed accessors。
- Create: `internal/config/toml.go`
  - 读取 `~/.config/taskg/taskg.toml` / `XDG_CONFIG_HOME`，解析平铺与分组 key。
- Modify: `internal/cli/root.go`
  - 解析 `rc.<key>=<value>` 与 `--no-context`，并把 rc 覆盖传入后续构造。
- Modify: `internal/cli/config.go`
  - 扩展 `config get/set/unset/list` 与 `show` 的合并视图。
- Modify: `internal/config/config_test.go`
  - 配置合并、数据库路径优先级、TOML 读取测试。
- Modify: `internal/cli/root_test.go`
  - rc 覆盖、`--no-context`、root 重排测试。
- Modify: `tests/integration/cli_test.go`
  - `show` / `config` 集成测试。

### Task 1: 写配置合并测试并固化优先级

**Files:**

- Modify: `internal/config/config_test.go`
- Modify: `internal/cli/root_test.go`
- Modify: `tests/integration/cli_test.go`

- [ ] **Step 1: 写失败测试**

在 `internal/config/config_test.go` 增加：

```go
func TestResolveMergesDatabasePathAndRuntimeConfig(t *testing.T) {
	// 1. explicit --db wins
	// 2. TASKG_DB wins over TOML
	// 3. TOML wins over default data dir fallback for non-path keys
	// 4. SQLite meta wins over TOML for user-set keys
}

func TestResolveLoadsTomlFile(t *testing.T) {
	// 读取 $XDG_CONFIG_HOME/taskg/taskg.toml 或 ~/.config/taskg/taskg.toml
	// 验证 date.format / color / context.active / uda.* 能被展开成 runtime key/value
}
```

在 `internal/cli/root_test.go` 增加：

```go
func TestRootParsesRcOverridesAndNoContext(t *testing.T) {
	// rc.date.format=epoch 不能进入 query parser
	// --no-context 必须从 root args 中被识别并影响后续执行
}

func TestRcOverrideEmptyClearsKey(t *testing.T) {
	// rc.context= 应清空本次运行的 context.active
	// rc.context: 也应清空，与等号空值同义
	// rc.context=none 是清空 context.active 的别名
}
```

在 `tests/integration/cli_test.go` 增加：

```go
func TestCLIShowUsesMergedConfig(t *testing.T) {
	// TOML + config set + rc override 的合并结果要稳定
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```bash
go test ./internal/config ./internal/cli -run 'Resolve|Root|Show|Config' -v
go test ./tests/integration -run TestCLIShowUsesMergedConfig -v
```

Expected: FAIL。

- [ ] **Step 3: 实现配置运行时模型**

在 `internal/config/runtime.go` 和 `internal/config/toml.go` 中：

- 引入纯 Go TOML 读取，推荐 `github.com/pelletier/go-toml/v2`。
- 将配置分成两层：
  - `ResolveDatabasePath(...)`：只负责在打开 SQLite 之前确定数据库路径。
  - `LoadRuntime(...)`：在数据库打开后，把 TOML、SQLite meta、环境变量和本次运行的 rc 覆盖合并成统一快照。
- `database.path` 是特殊项：
  - `config get database.path` 返回 `ResolveDatabasePath(...)` 的最终结果。
  - `config set database.path` / `config unset database.path` 在 M3 中不支持，路径只通过 flag、环境变量和 TOML 配置。
- 运行时快照必须提供：
  - `database.path`
  - `color`
  - `json`
  - `date.format`
  - `context.active`
  - `uda.*`
  - 任意 `.taskrc` 导入后落库的兼容键
- `config set` 写入的值继续保存在 SQLite meta 中，但最终读取顺序必须保持：
  - CLI flag
  - `rc.<key>=<value>`
  - env
  - SQLite meta
  - TOML
  - default
- config 路由优先级固定为：
  1. `uda.*` -> UDA app / repo
  2. `context.active` -> config get/list 从 context runtime 读取
  3. `urgency.*` -> 通用 config meta，供 urgency 引擎读取
  4. 其他 key -> 通用 config meta
- `config set context.active` / `config unset context.active` 在 M3 中不作为公开入口，避免绕过 context 命令的校验；`context use/none/delete` 负责写入和清理该值。

在 `internal/cli/root.go` 中：

- 扫描并剥离 `rc.*` token，保留给配置层。
- `rc.<key>=` 与 `rc.<key>:` 都表示本次运行清空该 key；`rc.context=none` 也归一为清空 `context.active`。
- 扫描 `--no-context` 并把它注入 root options。
- 继续保持两种入口：
  - `taskg <subcommand> ...`
  - `taskg <target> <action> ...`

- [ ] **Step 4: 运行测试**

Run:

```bash
go test ./internal/config ./internal/cli -run 'Resolve|Root|Show|Config' -v
go test ./tests/integration -run TestCLIShowUsesMergedConfig -v
```

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/config/config.go internal/config/runtime.go internal/config/toml.go internal/config/config_test.go internal/cli/root.go internal/cli/root_test.go internal/cli/config.go tests/integration/cli_test.go
git commit -m "feat: 配置系统升级"
```

### Task 2: config 命令与 show 输出改造

**Files:**

- Modify: `internal/cli/config.go`
- Modify: `internal/cli/root.go`
- Modify: `internal/config/runtime.go`
- Modify: `tests/integration/cli_test.go`

- [ ] **Step 1: 写失败测试**

在 `tests/integration/cli_test.go` 增加：

```go
func TestCLIConfigListUnsetAndShow(t *testing.T) {
	// config set / get / unset / list / show 的输出必须稳定
	// unset 后若 TOML 仍有值，应回退到 TOML
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./tests/integration -run TestCLIConfigListUnsetAndShow -v`  
Expected: FAIL。

- [ ] **Step 3: 扩展 config 子命令**

修改 `internal/cli/config.go`：

- `config get <key>` 读取合并后的 runtime 值。
- `config set <key> <value>` 继续写 SQLite meta，但对 `uda.*`、`context.*`、`urgency.*` 等 key 要预留后续路由。
- `config unset <key>` 删除 SQLite meta 层的覆盖值。
- `config list` 输出最终合并后的扁平键值，每行 `key=value`。
- `show` 输出人类可读摘要，至少包含：
  - `database.path`
  - `color`
  - `json`
  - `date.format`
  - `context.active`

`internal/config/runtime.go` 需要提供按 key 读取的统一视图，避免 `show`、`config get`、helper `_show` 各自拼一套。

- [ ] **Step 4: 运行测试**

Run:

```bash
go test ./internal/cli -run 'Config|Show' -v
go test ./tests/integration -run TestCLIConfigListUnsetAndShow -v
```

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/cli/config.go internal/cli/root.go internal/config/runtime.go tests/integration/cli_test.go
git commit -m "feat: 扩展 config 命令"
```

---

## Chunk 2: Context

### 文件职责

- Create: `internal/taskcontext/context.go`
  - context domain model、filter 校验、命名规则。
- Create: `internal/taskcontext/context_test.go`
  - context filter 与状态边界测试。
- Modify: `internal/storage/sqlite/models.go`
  - 新增 contexts 表模型。
- Modify: `internal/storage/sqlite/db.go`
  - AutoMigrate 新表。
- Create: `internal/storage/sqlite/context_repo.go`
  - context CRUD、active context 持久化读写。
- Create: `internal/app/context.go`
  - context 用例编排、active context 注入、读路径叠加。
- Modify: `internal/app/service.go`
  - List / Report / helper 读路径叠加 context。
- Modify: `internal/app/service_test.go`
  - context service 测试。
- Create: `internal/cli/context.go`
  - `context` 命令组。
- Modify: `internal/cli/root.go`
  - 注册 context 命令。
- Modify: `internal/cli/root_test.go`
  - `--no-context` 与 root 路由测试。
- Modify: `tests/integration/cli_test.go`
  - context 集成测试。

### Task 3: 写 context 模型和持久化测试

**Files:**

- Create: `internal/taskcontext/context_test.go`
- Modify: `internal/storage/sqlite/context_repo_test.go`（如需创建）
- Modify: `internal/app/service_test.go`

- [ ] **Step 1: 写失败测试**

```go
func TestContextDefineUseNoneDelete(t *testing.T) {
	// define -> use -> show -> none -> delete
	// name/filter 规则必须稳定
	// 没有 active context 时，context show 输出空字符串并 exit code 0
	// 删除 active context 时必须同步清空 context.active
}

func TestContextFilterAppliesToListAndReports(t *testing.T) {
	// table-driven 覆盖 spec 列出的所有受影响读路径：
	// list / next / all / completed / deleted / overdue / waiting / active
	// ready / blocked / blocking / _ids / _uuids / _projects / _tags / _unique
}

func TestContextDoesNotAffectExplicitMutationAndBackupPaths(t *testing.T) {
	// table-driven 覆盖 spec 列出的不受影响路径：
	// add / modify / done / delete / start / stop / annotate / denotate
	// append / prepend / edit / info / export / import / config / context
	// _get / _show / _version / completion
}

func TestNoContextBypassesActiveContext(t *testing.T) {
	// --no-context 或 rc.context=none 必须绕过 active context
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```bash
go test ./internal/taskcontext ./internal/app -run 'Context|NoContext' -v
```

Expected: FAIL。

- [ ] **Step 3: 实现 context domain + repo**

在 `internal/taskcontext/context.go` 和 `internal/storage/sqlite/context_repo.go` 中：

- 使用 `taskcontext` 包名，避免与标准库 `context` 混淆。
- 定义 `Context`：
  - `WorkspaceID`
  - `Name`
  - `FilterSource`
  - `CreatedAt`
  - `ModifiedAt`
- 校验规则：
  - 名称非空。
  - filter 必须能被现有 query parser 解析。
  - active context 不能指向不存在的 name。
- SQLite 需要新增 `contexts` 表，并保留 `workspace_id` 边界，为 M4 预留。
- M3 所有 context 行的 `workspace_id` 取现有 `store.LocalWorkspace().ID`；service 初始化时缓存 workspace ID，repo 调用必须显式传入，避免硬编码 `"local"`。
- active context 统一存入 SQLite meta 键 `context.active`，由 context 命令和 config/runtime 共同读取；不要再引入第二套存储位置。

在 `internal/app/context.go` 中：

- 提供 `DefineContext`、`UseContext`、`ContextNone`、`ContextShow`、`ContextList`、`ContextDelete`。
- `ContextShow` 在没有 active context 时返回空字符串并 exit code 0；脚本侧可用 `_show context.active` 配合空值判断。
- 提供 `activeContextFilter()` 或等价 helper，让读路径统一叠加 context。

- [ ] **Step 4: 运行测试**

Run:

```bash
go test ./internal/taskcontext ./internal/storage/sqlite ./internal/app -run 'Context|NoContext' -v
```

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/taskcontext/context.go internal/taskcontext/context_test.go internal/storage/sqlite/models.go internal/storage/sqlite/db.go internal/storage/sqlite/context_repo.go internal/app/context.go internal/app/service.go internal/app/service_test.go
git commit -m "feat: 添加 context 支持"
```

### Task 4: CLI context 命令与读路径叠加

**Files:**

- Create: `internal/cli/context.go`
- Modify: `internal/cli/root.go`
- Modify: `internal/cli/root_test.go`
- Modify: `tests/integration/cli_test.go`

- [ ] **Step 1: 写失败集成测试**

```go
func TestCLIContextCommands(t *testing.T) {
	// context define/use/show/list/delete
	// active context 应影响 spec 列出的所有读路径
	// context show 无 active context 时输出空字符串且成功退出
	// --no-context 应绕过
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./tests/integration -run TestCLIContextCommands -v`  
Expected: FAIL。

- [ ] **Step 3: 实现 context 命令**

创建 `internal/cli/context.go`：

- `context define <name> <filter...>`
- `context use <name>`
- `context none`
- `context show`
- `context list`
- `context delete <name>`

修改 `internal/cli/root.go`：

- 注册 `newContextCommand(opts)`。
- 让 `--no-context` 成为全局 flag 并传到 app service。

修改 `internal/app/service.go`：

- `List`、`ListReport`、`UUIDs`、`IDs`、`Projects`、`Tags`、`_unique`，以及所有 report 类读路径在不使用 `--no-context` 时，统一 AND 上 active context。这里应覆盖 `list`、`next`、`all`、`completed`、`deleted`、`overdue`、`waiting`、`active`、`ready`、`blocked`、`blocking` 和 helper 读路径。
- `add` / `modify` / `done` / `delete` / `start` / `stop` / `annotate` / `denotate` / `append` / `prepend` / `edit` 不被 context 过滤阻塞，始终按显式 target 操作。

- [ ] **Step 4: 运行测试**

Run:

```bash
go test ./internal/cli ./internal/app -run 'Context|NoContext' -v
go test ./tests/integration -run TestCLIContextCommands -v
```

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/cli/context.go internal/cli/root.go internal/cli/root_test.go internal/app/context.go internal/app/service.go tests/integration/cli_test.go
git commit -m "feat: 添加 context CLI"
```

---

## Chunk 3: UDA schema、值存储与写路径

### 文件职责

- Create: `internal/uda/schema.go`
  - UDA 定义、类型校验、值规范化、枚举规则。
- Create: `internal/uda/schema_test.go`
  - schema 校验和类型转换测试。
- Modify: `internal/task/model.go`
  - `UDAs` 字段、任务校验约束。
- Modify: `internal/task/modification.go`
  - 支持 UDA 修改 token。
- Modify: `internal/query/parser.go`
  - 允许 UDA 风格 token 进入 modification AST。
- Modify: `internal/query/parser_test.go`
  - UDA 修改语法测试。
- Modify: `internal/task/json.go`
  - UDA top-level field 导入导出。
- Modify: `internal/task/json_test.go`
  - UDA JSON 往返测试。
- Modify: `internal/storage/sqlite/models.go`
  - `uda_definitions`、`task_uda_values` 表模型。
- Modify: `internal/storage/sqlite/db.go`
  - AutoMigrate 新表。
- Create: `internal/storage/sqlite/uda_repo.go`
  - UDA schema 与任务值 CRUD。
- Create: `internal/app/uda.go`
  - UDA 定义、写入、清空、删除、查询的 app 层用例。
- Modify: `internal/app/service.go`
  - 任务创建/修改/导入时接入 UDA 校验与写入。
- Modify: `internal/app/service_test.go`
  - UDA 相关 service 测试。
- Modify: `internal/cli/config.go`
  - `config set` / `unset` 路由到 UDA schema 或 value 操作。

### Task 5: 写 UDA schema 和 JSON 往返测试

**Files:**

- Create: `internal/uda/schema_test.go`
- Modify: `internal/task/json_test.go`
- Modify: `internal/task/model_test.go`

- [ ] **Step 1: 写失败测试**

```go
func TestUDASchemaValidation(t *testing.T) {
	// string / numeric / date / duration / enum 校验
	// 枚举 spec 中所有禁止冲突的内置字段：
	// uuid / description / status / entry / modified / end / due / start / wait
	// scheduled / until / project / priority / depends / annotations / recur / parent / tag
	// 每个名称都必须让 ValidateDefinition 返回 error
}

func TestTaskJSONCarriesUDAFields(t *testing.T) {
	// UDA 应作为 top-level field 往返，不丢 orphan
	// date UDA 导出应保持 RFC3339 UTC 字符串
}

func TestTaskValidateDoesNotSilentlyDropUDAValues(t *testing.T) {
	// domain 层保留 UDAs；真正的 schema 校验由 app 层执行
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```bash
go test ./internal/uda ./internal/task -run 'UDA|JSON' -v
```

Expected: FAIL。

- [ ] **Step 3: 实现 UDA schema 模型**

在 `internal/uda/schema.go` 中：

- 定义 `Definition`：
  - `Name`
  - `Type`
  - `Label`
  - `Values`
  - `Default`
  - `OrphanAllowed`
- 定义 `Value` 或 `ParsedValue`，用于把 `string/numeric/date/duration` 规范化为可比较表示。
- 导出 `ValidateDefinition`、`ValidateValue`、`NormalizeValue`、`ParseValue`。
- date 类型 UDA 固定保存为 RFC3339 UTC，格式为 `2006-01-02T15:04:05Z`；`_get` 和 JSON export 直接返回这个字符串。
- duration 类型 UDA 固定保存为秒数的十进制字符串。
- enum `Values` 在内存中使用 `[]string`，持久化时统一转成 JSON array。
- `task.Task` 加入 `UDAs map[string]task.UDAValue`，`UDAValue` 至少包含：
  - `Raw`
  - `Type`
  - `Orphan`
- `task.Validate()` 只检查 map 结构和基本字符串安全，不做 schema 绑定校验。

在 `internal/task/json.go` 中：

- UDA 导出为 top-level field。
- 已定义 UDA 与 orphan UDA 都必须往返。
- 未定义 top-level field 导入时保留为 orphan UDA，而不是丢弃。

- [ ] **Step 4: 运行测试**

Run:

```bash
go test ./internal/uda ./internal/task -run 'UDA|JSON' -v
```

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/uda/schema.go internal/uda/schema_test.go internal/task/model.go internal/task/json.go internal/task/json_test.go
git commit -m "feat: 添加 UDA 模型"
```

### Task 6: UDA 持久化与任务写路径

**Files:**

- Modify: `internal/storage/sqlite/models.go`
- Modify: `internal/storage/sqlite/db.go`
- Create: `internal/storage/sqlite/uda_repo.go`
- Create: `internal/app/uda.go`
- Modify: `internal/app/service.go`
- Modify: `internal/app/service_test.go`
- Modify: `internal/cli/config.go`
- Modify: `tests/integration/cli_test.go`

- [ ] **Step 1: 写失败测试**

```go
func TestServicePersistsUDAValues(t *testing.T) {
	// define schema -> add task estimate:3 -> update -> clear -> reload
}

func TestConfigSetRoutesUDASchemaKeys(t *testing.T) {
	// config set uda.estimate.type numeric
	// config set uda.estimate.label Estimate
	// config set uda.estimate.values "1,2,3,5,8"
	// set -> get -> list -> unset -> get 行为必须闭环
}

func TestImportPreservesOrphanUDA(t *testing.T) {
	// JSON import 的未知字段必须保留为 orphan
}

func TestModifyRejectsOrphanUDA(t *testing.T) {
	// 1. JSON import 一个含未定义 UDA "legacy_field" 的任务
	// 2. taskg 1 modify legacy_field:newvalue 应返回 error
	// 3. taskg 1 modify legacy_field: 也应返回 error，不能靠清空绕过
	// 4. JSON import 修改同字段值应被允许，保证 round-trip
	// 5. taskg 1 edit 中清空 legacy_field 应被允许
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```bash
go test ./internal/app ./internal/storage/sqlite -run 'UDA|ConfigSet' -v
go test ./tests/integration -run 'UDA|Import' -v
```

Expected: FAIL。

- [ ] **Step 3: 实现 UDA 存储**

在 `internal/storage/sqlite/models.go` 和 `db.go` 中：

- 增加 `uda_definitions` 表。
- 增加 `task_uda_values` 表。
- 两张表都应带 `workspace_id`，为 M4 预留边界。
- M3 所有 UDA schema/value 行的 `workspace_id` 取现有 `store.LocalWorkspace().ID`；service 初始化时缓存 workspace ID，repo 调用必须显式传入，避免硬编码 `"local"`。

在 `internal/storage/sqlite/uda_repo.go` 中：

- 提供 `GetDefinition`、`ListDefinitions`、`UpsertDefinition`、`DeleteDefinition`。
- 提供 `GetTaskUDAs`、`ReplaceTaskUDAs`、`UniqueUDAValues`。
- `ReplaceTaskUDAs` 必须支持清空、覆盖、保留 orphan。

在 `internal/app/uda.go` 中：

- 提供 `DefineUDA`、`DeleteUDA`、`ListUDAs`、`SetUDAValue`、`ClearUDAValue`、`UniqueUDAValues`。
- `config set uda.*` 和 `config unset uda.*` 走这些方法，而不是直接写 meta。
- `config set uda.<name>.<field>` 写入 `uda_definitions`；`config get uda.<name>.<field>` 优先从 `uda_definitions` 反向构造平铺 key，回退到 meta；`config list` 必须包含从 `uda_definitions` 展开的所有 `uda.*` key。
- `config unset uda.<name>.<field>` 清除对应 schema 字段；如果 unset `uda.<name>.type`，等价于删除该 UDA definition，但不删除已有 orphan/imported task value。
- `config set uda.<name>.values "1,2,3"` 接受逗号分隔字符串，内部归一到 `values_json` JSON array；TOML 原生 array 和 `.taskrc` 逗号分隔同样归一到 `values_json`。
- 任务写入前先按 schema 校验 UDA 值，再写任务与 UDA value 表。

- [ ] **Step 4: 运行测试**

Run:

```bash
go test ./internal/app ./internal/storage/sqlite -run 'UDA|ConfigSet' -v
go test ./tests/integration -run 'UDA|Import' -v
```

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/storage/sqlite/models.go internal/storage/sqlite/db.go internal/storage/sqlite/uda_repo.go internal/app/uda.go internal/app/service.go internal/app/service_test.go internal/cli/config.go tests/integration/cli_test.go
git commit -m "feat: 持久化 UDA"
```

---

## Chunk 4: UDA 查询、DOM、helper 与 urgency

### 文件职责

- Modify: `internal/query/ast.go`
  - 增加动态 UDA 属性表达。
- Modify: `internal/query/parser_ast.go`
  - 不再对未知 `name:value` 立即报错，转为动态 UDA 候选。
- Modify: `internal/query/parser_ast_test.go`
  - 动态 UDA query 解析测试。
- Modify: `internal/query/parser.go`
  - `ParseModifyArgs` 识别 UDA 修改 token。
- Modify: `internal/query/parser_test.go`
  - UDA modify 语法测试。
- Modify: `internal/storage/sqlite/query_scope.go`
  - 动态 UDA 条件编译。
- Modify: `internal/storage/sqlite/query_scope_test.go`
  - UDA 查询 SQL 测试。
- Modify: `internal/dom/dom.go`
  - `_get` 支持 UDA。
- Modify: `internal/dom/dom_test.go`
  - UDA DOM 测试。
- Modify: `internal/urgency/urgency.go`
  - UDA urgency 因子。
- Modify: `internal/urgency/urgency_test.go`
  - UDA urgency 测试。
- Modify: `internal/cli/helper.go`
  - `_udas`、`_unique`。
- Modify: `internal/app/service.go`
  - 读取 UDA schema 注入 query / urgency / helper。
- Modify: `internal/app/service_test.go`
  - UDA query / unique / urgency 测试。

### Task 7: 写动态 UDA 查询测试

**Files:**

- Modify: `internal/query/parser_ast_test.go`
- Modify: `internal/query/parser_test.go`
- Modify: `internal/storage/sqlite/query_scope_test.go`

- [ ] **Step 1: 写失败测试**

```go
func TestParseQueryRecognizesDynamicUDA(t *testing.T) {
	// estimate:3 / estimate.notnull / estimate:
	// parser 不应立即报错，compile 阶段再结合 schema 判断
}

func TestParseModifyArgsAllowsUDAFields(t *testing.T) {
	// add / modify token 中 estimate:3、estimate: 都要被识别
}

func TestCompileQueryUDAFilters(t *testing.T) {
	// estimate:3 / estimate.notnull / estimate:
	// date UDA 的 eq 必须按自然日范围 [day_start, next_day_start) 编译
	// before / after 使用 RFC3339 UTC 字符串解析后的时间比较
}

func TestBuiltinDateEqUsesDayRange(t *testing.T) {
	// 追溯修正内置 due / wait / scheduled / until 等日期字段 eq 语义
	// due:2026-05-28 应匹配当天任意时间点，而不是只匹配 00:00:00
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```bash
go test ./internal/query ./internal/storage/sqlite -run 'UDA|ModifyArgs' -v
```

Expected: FAIL。

- [ ] **Step 3: 扩展 query AST 和编译器**

在 `internal/query/ast.go` 和 `internal/query/parser_ast.go` 中：

- 新增动态 UDA 表达能力，建议：
  - `Attribute` 增加 `AttrUDA`
  - `Predicate` 增加 `Field string`
- 规则：
  - `estimate:3` 先被解析成动态 UDA 候选，不在 parser 阶段报错。
  - `estimate.notnull` 解析为 UDA 非空测试。
  - `estimate:` 解析为 UDA 为空测试。
  - 如果字段最终不在 schema 中，compile 阶段返回 unknown UDA 错误。

在 `internal/storage/sqlite/query_scope.go` 中：

- 通过 `QueryCompileOptions` 注入 UDA schema。
- 编译 UDA 条件时使用参数绑定。
- 字符型 UDA 的 `/x/` 与 description 一样按子串处理，M3 不做正则 UDA。
- date UDA 的 `eq` 固定编译为自然日范围：
  - `name:2026-05-28` -> `value >= "2026-05-28T00:00:00Z" AND value < "2026-05-29T00:00:00Z"`
  - SQL 形态使用 `EXISTS (...)`，范围条件放在子查询内，并使用参数绑定。
- 同步修正内置日期字段的 `eq` 语义，避免 UDA 日期按自然日、内置日期按精确秒的双轨行为；内置 `due:date` / `wait:date` / `scheduled:date` / `until:date` 都使用 `[day_start, next_day_start)`。

在 `internal/query/parser.go` 中：

- `ParseModifyArgs` 需要把 UDA token 放进修改 AST，而不是丢给 unknown attribute。

- [ ] **Step 4: 运行测试**

Run:

```bash
go test ./internal/query ./internal/storage/sqlite -run 'UDA|ModifyArgs' -v
```

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/query/ast.go internal/query/parser_ast.go internal/query/parser_ast_test.go internal/query/parser.go internal/query/parser_test.go internal/storage/sqlite/query_scope.go internal/storage/sqlite/query_scope_test.go
git commit -m "feat: 查询支持 UDA"
```

### Task 8: DOM、helper 和 urgency 的 UDA 贯通

**Files:**

- Modify: `internal/dom/dom.go`
- Modify: `internal/dom/dom_test.go`
- Modify: `internal/urgency/urgency.go`
- Modify: `internal/urgency/urgency_test.go`
- Modify: `internal/cli/helper.go`
- Modify: `internal/app/service.go`
- Modify: `internal/app/service_test.go`

- [ ] **Step 1: 写失败测试**

```go
func TestResolveUDAFields(t *testing.T) {
	// _get 1.estimate / 1.uda.estimate / 1.reviewed
}

func TestUniqueHelperSupportsUDA(t *testing.T) {
	// _unique estimate 需要输出去重值
}

func TestExplainIncludesUDAContribution(t *testing.T) {
	// urgency explain 应体现 UDA 系数
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```bash
go test ./internal/dom ./internal/urgency ./internal/app -run 'UDA|Unique|Explain' -v
```

Expected: FAIL。

- [ ] **Step 3: 实现 DOM / helper / urgency**

在 `internal/dom/dom.go` 中：

- `_get` 支持读取已定义与 orphan UDA。
- 不存在的 UDA 返回空字符串，不报错。

在 `internal/cli/helper.go` 中：

- `_udas` 输出定义过的 UDA 名称，每行一个。
- `_unique` 支持 `project`、`priority`、`tags` 和 UDA 名称。
- `_unique` 在 M3 不支持 M2 日期字段，只支持 `project`、`priority`、`tags` 和已定义 UDA，避免日期格式和空值语义扩散。

在 `internal/urgency/urgency.go` 中：

- 加入 UDA contribution。
- 支持：
  - `urgency.uda.<name>.coefficient`
  - `urgency.uda.<name>.<value>.coefficient`
- 不配置时 UDA 不影响 urgency。

在 `internal/app/service.go` 中：

- `ExplainUrgency` 必须拿到 UDA schema 和 active context 之后再计算。
- `RunReport` 的 urgency 排序路径下，UDA 值必须通过一次性 IN 查询批量加载到任务上，避免每条任务 explain urgency 时触发 N+1 查询。
- `RunReport` / `_unique` 要共享同一套 UDA-aware 过滤和排序逻辑，避免 `urgency` 与 `next` 的解释不一致。

- [ ] **Step 4: 运行测试**

Run:

```bash
go test ./internal/dom ./internal/urgency ./internal/app -run 'UDA|Unique|Explain' -v
go test ./internal/cli -run 'Helper|Show|Version' -v
```

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/dom/dom.go internal/dom/dom_test.go internal/urgency/urgency.go internal/urgency/urgency_test.go internal/cli/helper.go internal/app/service.go internal/app/service_test.go
git commit -m "feat: UDA 贯通 DOM 和 urgency"
```

---

## Chunk 5: `.taskrc` 只读导入

### 文件职责

- Create: `internal/taskrc/import.go`
  - `.taskrc` 解析、include、报告结构。
- Create: `internal/taskrc/import_test.go`
  - `.taskrc` 解析和兼容报告测试。
- Modify: `internal/cli/config.go`
  - `config import-taskrc` 子命令。
- Modify: `internal/app/taskrc.go`
  - `.taskrc` 导入编排。
- Modify: `internal/app/service.go`
  - 导入写入 config / context / UDA / urgency 相关数据。
- Modify: `internal/app/service_test.go`
  - 导入 dry-run / overwrite / report 测试。
- Modify: `tests/integration/cli_test.go`
  - CLI 级 `.taskrc` 导入测试。

### Task 9: 写 `.taskrc` 解析和报告测试

**Files:**

- Create: `internal/taskrc/import_test.go`
- Modify: `internal/app/service_test.go`

- [ ] **Step 1: 写失败测试**

```go
func TestParseTaskRCRecognizesConfigContextAndUDAKeys(t *testing.T) {
	// name = value / include / comments / blank lines
	// imported / skipped / unknown 都要有稳定报告
	// uda.<name>.values 使用 Taskwarrior 逗号分隔格式，导入后归一为 values_json
}

func TestTaskRCDryRunDoesNotWriteState(t *testing.T) {
	// dry-run 下不应该改 meta、contexts、UDA schema
	// 非 dry-run 冲突时默认覆盖 SQLite meta/context/UDA schema 中的同 key
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/taskrc ./internal/app -run 'TaskRC|DryRun' -v`  
Expected: FAIL。

- [ ] **Step 3: 实现 parser 和 report**

在 `internal/taskrc/import.go` 中：

- 支持 Taskwarrior `.taskrc` 的简单 `name = value` 语法。
- 支持 `include <path>`，并处理递归 include 与循环 include。
- 识别并分类：
  - imported：M3 支持且能落库的 key
  - skipped：认识但 M3 不导入的 key；`data.location` 归为 skipped，因为 M3 的 `database.path` 只在启动前由 `--db`、`TASKG_DB`、`--data-dir` 或 TOML 决定
  - unknown：完全不认识的 key
- `.taskrc` 中 `uda.<name>.values=1,2,3` 按逗号分隔解析，写入 UDA schema 时归一到 `values_json` JSON array；这必须与 TOML array 和 CLI `config set uda.<name>.values "1,2,3"` 的最终结果一致。
- 报告结构必须能 human 输出，也能 JSON 输出。

在 `internal/app/taskrc.go` 中：

- 解析 `.taskrc` 后，按 key 路由到：
  - config meta
  - context store
  - UDA schema
  - urgency 系数
- `--dry-run` 只返回报告，不写入。
- 非 dry-run 导入遇到已有 config/context/UDA 同 key 时默认覆盖；M3 不实现 `--no-overwrite`。

- [ ] **Step 4: 运行测试**

Run:

```bash
go test ./internal/taskrc ./internal/app -run 'TaskRC|DryRun' -v
```

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/taskrc/import.go internal/taskrc/import_test.go internal/app/taskrc.go internal/app/service.go internal/app/service_test.go
git commit -m "feat: 添加 taskrc 导入"
```

### Task 10: CLI `config import-taskrc` 集成

**Files:**

- Modify: `internal/cli/config.go`
- Modify: `tests/integration/cli_test.go`
- Modify: `internal/cli/root.go`

- [ ] **Step 1: 写失败集成测试**

```go
func TestCLIImportTaskRC(t *testing.T) {
	// dry-run JSON 报告
	// 实际导入后，config/context/UDA/urgency 结果要可观测
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./tests/integration -run TestCLIImportTaskRC -v`  
Expected: FAIL。

- [ ] **Step 3: 实现命令**

修改 `internal/cli/config.go`：

- 增加 `config import-taskrc <path> [--dry-run] [--json]`。
- `--json` 输出导入报告 JSON。
- human 输出要能看出 imported / skipped / unknown 的分类。

修改 `internal/cli/root.go`：

- 注册新子命令。
- 保持与 `config get/set/unset/list/show` 一致的 flag 合并逻辑。

- [ ] **Step 4: 运行测试**

Run:

```bash
go test ./tests/integration -run TestCLIImportTaskRC -v
go test ./internal/cli -run 'Config|Show' -v
```

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/cli/config.go internal/cli/root.go tests/integration/cli_test.go
git commit -m "feat: CLI 支持 taskrc 导入"
```

---

## Chunk 6: `_show`、`_version` 与 completion

### 文件职责

- Modify: `internal/cli/helper.go`
  - `_show`、`_version`。
- Create: `internal/cli/completion.go`
  - shell completion 命令。
- Modify: `internal/cli/root.go`
  - 注册 completion 命令。
- Modify: `internal/cli/root_test.go`
  - completion 与 helper 的非数据库测试。
- Modify: `tests/integration/cli_test.go`
  - completion / helper 集成测试。

### Task 11: 写 helper 和 completion 测试

**Files:**

- Modify: `internal/cli/root_test.go`
- Modify: `tests/integration/cli_test.go`

- [ ] **Step 1: 写失败测试**

```go
func TestCLIShowHelperAndVersion(t *testing.T) {
	// _show / _version 输出稳定
	// _version 无 build flag 时输出 taskg dev
}

func TestCLICompletionDoesNotOpenDatabase(t *testing.T) {
	// completion bash|zsh|fish|powershell 不应初始化 DB
	// 用 --db 指向不存在父目录的路径验证：
	// 如果命令尝试打开 DB，会因父目录不存在而失败
	// 如果没有打开 DB，应正常输出 completion 脚本
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```bash
go test ./internal/cli -run 'Helper|Version|Completion' -v
go test ./tests/integration -run 'Helper|Version|Completion' -v
```

Expected: FAIL。

- [ ] **Step 3: 实现 helper 和 completion**

在 `internal/cli/helper.go` 中：

- `_show` 作为脚本版配置读取器：
  - 无参数输出所有合并后的 key/value，每行一个。
  - 有参数按请求顺序输出对应 value。
- `_version` 输出构建时注入的版本字符串；如果没有注入，输出 `taskg dev`。

创建 `internal/cli/completion.go`：

- `completion bash`
- `completion zsh`
- `completion fish`
- `completion powershell`

规则：

- completion 只输出脚本到 stdout。
- completion 不打开数据库。
- completion 不依赖当前 workspace 或 context。
- completion 命令必须能在业务命令初始化数据库之前返回；如果 root 使用 `PersistentPreRunE` 打开 DB，需要跳过 completion，或把 DB 打开下沉到具体业务命令的 `RunE`。
- 集成测试使用一个无法打开的 `--db` 路径，例如 `$TMP/nonexistent-dir/taskg.db`。`taskg --db "$bad" completion bash` 应成功输出 bash completion；如果尝试开库，测试应失败。

- [ ] **Step 4: 运行测试**

Run:

```bash
go test ./internal/cli -run 'Helper|Version|Completion' -v
go test ./tests/integration -run 'Helper|Version|Completion' -v
```

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/cli/helper.go internal/cli/completion.go internal/cli/root.go internal/cli/root_test.go tests/integration/cli_test.go
git commit -m "feat: 补齐 helper 和 completion"
```

---

## Chunk 7: 文档、验收与收尾

### 文件职责

- Modify: `README.md`
  - 增加 M3 用法和约束说明。
- Modify: `ROADMAP.md`
  - 将 M3 标为已完成，下一步切到 M4。
- Modify: `docs/superpowers/specs/2026-05-28-taskg-m3-design.md`
  - 若实现过程确认范围有微调，回写 spec。
- Test: 全量测试与 CGO-free 构建。

### Task 12: 更新 README 和 ROADMAP

**Files:**

- Modify: `README.md`
- Modify: `ROADMAP.md`

- [ ] **Step 1: 写失败检查**

先人工核对 README / ROADMAP 当前是否仍停留在 M2 或缺少 M3 入口说明。若已被改动，则按最终文件内容调整本任务的验收点。

- [ ] **Step 2: 更新文档**

README 必须补充：

- `taskg.toml` 与 `config` 用法。
- `context define/use/none/show/list/delete` 用法。
- UDA 的 `config set`、`_udas`、`_unique`、`_get` 示例。
- `.taskrc` 只读导入示例。
- `completion` 示例。

ROADMAP 必须补充：

- M3 状态改为“已完成”。
- 简述 M3 实际交付范围。
- 下一步指向 M4 多 workspace。

- [ ] **Step 3: 运行检查**

Run:

```bash
git diff --check
```

Expected: PASS。

- [ ] **Step 4: 提交**

```bash
git add README.md ROADMAP.md docs/superpowers/specs/2026-05-28-taskg-m3-design.md
git commit -m "docs: 更新 M3 文档与路线图"
```

### Task 13: 最终验收

**Files:**

- No code changes expected.

- [ ] **Step 1: 全量测试**

Run:

```bash
go test ./...
```

Expected: PASS。

- [ ] **Step 2: CGO-free 测试**

Run:

```bash
CGO_ENABLED=0 go test ./...
```

Expected: PASS。

- [ ] **Step 3: CGO-free build**

Run:

```bash
CGO_ENABLED=0 go build ./cmd/taskg
```

Expected: PASS。

- [ ] **Step 4: 确认没有引入 CGO SQLite driver**

Run:

```bash
if go list -m all | grep -E 'gorm.io/driver/sqlite|mattn/go-sqlite3'; then
  echo "unexpected CGO SQLite dependency" >&2
  exit 1
fi
```

Expected: 无输出，exit code 0。

- [ ] **Step 5: 手动冒烟**

Run:

```bash
tmp="$(mktemp -d)"
go run ./cmd/taskg --db "$tmp/taskg.db" show
go run ./cmd/taskg --db "$tmp/taskg.db" completion zsh
go run ./cmd/taskg --db "$tmp/taskg.db" config list
```

Expected:

- `show` 输出合并后的关键配置。
- `completion` 输出 shell completion 脚本，不报错。
- `config list` 输出稳定的 `key=value` 列表。

- [ ] **Step 6: 检查 git 状态**

Run:

```bash
git status --short --branch
```

Expected: 干净。

## 计划审阅说明

本计划根据 [docs/superpowers/specs/2026-05-28-taskg-m3-design.md](/Users/mac/code/projects/dajee/task/docs/superpowers/specs/2026-05-28-taskg-m3-design.md)、[README.md](/Users/mac/code/projects/dajee/task/README.md)、[ROADMAP.md](/Users/mac/code/projects/dajee/task/ROADMAP.md)、[AGENTS.md](/Users/mac/code/projects/dajee/task/AGENTS.md) 和 [docs/requirements.md](/Users/mac/code/projects/dajee/task/docs/requirements.md) 编写。

当前未执行 plan-document-reviewer subagent 审阅；本环境虽提供通用 multi-agent 工具，但未提供明确的 plan-document-reviewer 角色或提示文件，也没有 `plan-document-reviewer-prompt.md` 可引用。后续如需要严格执行 superpowers 审阅环节，请用专门 reviewer prompt 对每个 chunk 进行审阅。实施过程中若发现本计划与 spec 冲突，以 spec 为准并先更新计划。
