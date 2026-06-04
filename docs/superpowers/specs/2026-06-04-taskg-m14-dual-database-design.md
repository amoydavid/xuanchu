# M14：多数据库支持（SQLite / PostgreSQL）

## 背景

taskg 当前只支持 SQLite（`github.com/glebarez/sqlite`，纯 Go，零 CGO）。企业部署场景中，部分客户要求使用 PostgreSQL。本 milestone 在保持零 CGO、零接口定义的前提下，让 taskg 同时支持 SQLite 和 PostgreSQL，通过配置选择后端。

## 目标

- `internal/storage` 包同时支持 SQLite 和 PostgreSQL
- GORM 作为唯一的数据库抽象层，不引入 repository interface
- PostgreSQL 使用纯 Go driver（`gorm.io/driver/postgres` / `github.com/jackc/pgx/v5`）
- 全局保持 `CGO_ENABLED=0` 可构建、可测试
- 现有 SQLite 行为完全不变

## 不进入 M14

- SQLite → PostgreSQL 数据迁移工具
- 双向实时同步
- MySQL / CockroachDB 等其他数据库
- repository interface 抽象
- 修改 GORM model 结构

## 包结构变更

`internal/storage/sqlite` 重命名为 `internal/storage`。这是本 milestone 最大的机械性改动，涉及 36+ 个文件的 import 路径替换（`sqlite.Open` → `storage.Open`、`*sqlite.Store` → `*storage.Store` 等），但每个文件的改动仅限于 import 路径，不涉及业务逻辑变更。

文件组织：

```
internal/storage/
  db.go                — Open(dbURL), Store struct, Close, Transaction, DB, Dialect, ensureLocalIdentity
  db_sqlite.go         — openSQLite(), configureSQLite(), sqliteDSN()
  db_postgres.go       — openPostgres(), configurePostgres()
  models.go            — 所有 GORM model（不改动）
  migrate.go           — migrate() 入口，按 dialect 分发
  migrate_sqlite.go    — SQLite 专有：PRAGMA foreign_keys、M5 迁移
  migrate_postgres.go  — PostgreSQL 专有：纯净建表
  query_scope.go       — CompileQuery，新增 dialect 参数适配 SQL 方言
  task_repo.go         — 不改动
  project_repo.go      — 不改动
  ... 其余 repo 文件不改动
```

## Open 路由

`storage.Open(dbURL string) (*Store, error)` 根据 dbURL 格式选择后端：

- `dbURL == ""` 或是文件路径（不含 `://`）：走 SQLite
- `dbURL` 以 `postgres://` 或 `postgresql://` 开头：走 PostgreSQL
- 其他：返回 `unsupported database scheme` 错误

`Store` struct 新增 `dialect string` 字段（值为 `"sqlite"` 或 `"postgres"`），用于 migration 分发和 query 编译方言选择。

## Migration 策略

### SQLite（migrate_sqlite.go）

保持现有全部逻辑不变：

- `PRAGMA foreign_keys = ON`
- GORM `AutoMigrate` 全部 model
- M5 迁移（`prepareProjectSchemaForM5` 及其全部辅助函数）
- `ensureLocalIdentity`（两种数据库通用，留在 `db.go`）

### PostgreSQL（migrate_postgres.go）

纯净建表：

- 只使用 GORM `AutoMigrate` 创建全部表和索引
- 不执行 `prepareWorkspaceSchemaForM4`（M4 历史迁移仅 SQLite 路径）
- 不执行 `prepareProjectSchemaForM5`（M5 历史迁移仅 SQLite 路径）
- 不使用 PRAGMA
- GORM 对 PostgreSQL 自动处理类型映射（`TEXT → text`、`INTEGER → bigint`、`NUMERIC → numeric`）
- M5 手写 DDL 不需要，因为 `AutoMigrate` 直接创建正确的 schema
- M5 的关联表 DDL（`m5TaskRelationSchemas`）在 PostgreSQL 下由 `AutoMigrate` 等价处理

`migrate()` 入口按 dialect 分发，SQLite 路径调用 `prepareWorkspaceSchemaForM4` + `prepareProjectSchemaForM5` + `AutoMigrate`，PostgreSQL 路径只调用 `AutoMigrate`。

## Query 编译方言适配

`QueryCompileOptions` 新增 `Dialect string` 字段。

需要适配的 SQL 差异（均位于 `query_scope.go` 的 `compileUDAPredicate` 函数内）：

| SQLite | PostgreSQL | 用途 | 出现位置 |
|---|---|---|---|
| `LIKE` | `ILIKE` | 大小写不敏感的文本搜索（description、bare text、UDA string、tag、annotation） | `compilePredicate` 多处 |
| `CAST(... AS REAL)` | `CAST(... AS DOUBLE PRECISION)` | UDA numeric/duration 比较 | `compileUDAPredicate` 第 241-244 行，共 3 处（`=`、`<`、`>`） |

提供 dialect helper 函数：

```go
func likeOp(dialect string) string
func realCastType(dialect string) string
```

其余 SQL 均为标准语法（`=`, `<`, `>`, `IS NULL`, `EXISTS`, `NOT EXISTS` 等），无需适配。

`ApplyQuery` 调用路径：app 层从 `Store.Dialect()` 取值传入 `QueryCompileOptions`。

## 配置入口

新增配置项：

| 来源 | 键 | 示例 |
|---|---|---|
| CLI flag | `--db-url` | `postgres://user:pass@localhost:5432/taskg?sslmode=disable` |
| 环境变量 | `TASKG_DB_URL` | 同上 |
| TOML | `[database]` 段 `url = "..."` | 同上 |

### 优先级

完整解析优先级（从高到低）：

1. `--db-url` CLI flag（如果指定，直接使用，忽略其他所有来源）
2. `TASKG_DB_URL` 环境变量（如果指定且 `--db-url` 未指定）
3. `[database] url = "..."` TOML 配置（如果上述都未指定）
4. `--db` CLI flag（如果指定，走 SQLite 文件路径）
5. `TASKG_DB` 环境变量
6. `[database] path = "..."` TOML 配置
7. 默认 SQLite 路径 `~/.local/share/taskg/taskg.db`

**互斥规则：**

- `--db-url` 和 `--db` 同时指定 → 返回错误
- `TASKG_DB_URL` 和 `TASKG_DB` 同时存在 → `TASKG_DB_URL` 优先（与环境变量优先级一致）
- 只指定 `--db-url` 或 `TASKG_DB_URL` → 按 scheme 路由（`postgres://` → PostgreSQL，否则报错）
- 只指定 `--db` 或 `TASKG_DB` → 走 SQLite 文件路径
- `--db` 接收到含 `://` 的值时返回错误，防止误用

### TOML 示例

```toml
# PostgreSQL（优先于 database.path）
[database]
url = "postgres://user:pass@localhost:5432/taskg?sslmode=disable"

# SQLite（url 未设置时生效）
[database]
path = "/path/to/taskg.db"
```

### 代码改动

`internal/config`：

- `Config` struct 新增 `DatabaseURL string`
- `ResolveDBURL()` 新增：按上述优先级返回有效的数据库 URL 或空字符串
- 现有 `ResolveDBPath()` 保持不变，仅在 `ResolveDBURL()` 返回空时被调用

`internal/app`：

- 从 config 取 `ResolveDBURL()` 结果，非空时传给 `storage.Open(dbURL)`，否则传 `storage.Open(ResolveDBPath())`

## 依赖变更

`go.mod` 新增：

- `gorm.io/driver/postgres` — GORM PostgreSQL dialector
- 间接依赖 `github.com/jackc/pgx/v5` — 纯 Go PostgreSQL driver

两者均为纯 Go，不引入 CGO。

## 测试策略

### 现有测试

随包重命名从 `internal/storage/sqlite/*_test.go` 迁移到 `internal/storage/*_test.go`，继续作为 SQLite 测试运行，行为不变。

### PostgreSQL 测试

- 新增 `internal/storage/postgres_test.go`
- 通过环境变量 `TASKG_TEST_DB_URL` 指定 PostgreSQL 连接字符串
- 无 PostgreSQL 环境时 skip
- 测试重点：
  1. `Open(postgres://...)` 能建库
  2. `AutoMigrate` 创建正确的表和索引
  3. `ensureLocalIdentity` 正常执行
  4. 核心 repo CRUD 通过（Task、Project、Workspace）
  5. Query 编译方言验证（纯单元测试，不依赖 PostgreSQL 实例：`CompileQuery` 对 SQLite/PostgreSQL dialect 输出正确的 SQL，验证 `ILIKE` / `DOUBLE PRECISION`）

### 集成测试

- `tests/integration/cli_test.go` 默认继续用 SQLite
- 新增环境变量 `TASKG_TEST_DB_URL` 支持对 PostgreSQL 运行同一套 CLI 集成测试

### 验收命令

```bash
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/taskg
```

## 验收标准

- `taskg --db-url postgres://user:pass@host:5432/taskg server` 能启动并正常工作
- `taskg --db ./taskg.db list` 行为与改动前完全一致
- `CGO_ENABLED=0 go test ./...` 通过
- `CGO_ENABLED=0 go build ./cmd/taskg` 通过
- 现有 SQLite 全部测试不回退
- PostgreSQL 下 CLI 集成测试通过（有 PostgreSQL 环境时）
- `internal/app`、`internal/cli` 等消费者只改 import 路径，不改业务逻辑

## 对 ROADMAP 的更新

M14 完成后更新 ROADMAP.md 状态总览表，新增：

| M14 | 已完成 | 多数据库支持（SQLite / PostgreSQL） |
