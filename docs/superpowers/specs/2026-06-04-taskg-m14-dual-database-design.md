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

`internal/storage/sqlite` 重命名为 `internal/storage`。文件组织：

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
- 不执行任何历史迁移
- 不使用 PRAGMA
- GORM 对 PostgreSQL 自动处理类型映射（`TEXT → text`、`INTEGER → bigint`、`NUMERIC → numeric`）
- M5 手写 DDL 不需要，因为 `AutoMigrate` 直接创建正确的 schema
- M5 的关联表 DDL（`m5TaskRelationSchemas`）在 PostgreSQL 下由 `AutoMigrate` 等价处理

## Query 编译方言适配

`QueryCompileOptions` 新增 `Dialect string` 字段。

需要适配的 SQL 差异：

| SQLite | PostgreSQL | 用途 |
|---|---|---|
| `LIKE` | `ILIKE` | 大小写不敏感的文本搜索 |
| `CAST(... AS REAL)` | `CAST(... AS DOUBLE PRECISION)` | UDA numeric/duration 比较 |

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

优先级：CLI flag > 环境变量 > TOML > 默认值。

与 `--db` 的关系：

- `--db` 保持现有行为（SQLite 文件路径）
- `--db-url` 和 `--db` 互斥，同时指定返回错误
- 两者都不指定：走默认 SQLite 路径 `~/.local/share/taskg/taskg.db`
- 只指定 `--db-url`：按 scheme 路由

`internal/config` 改动：

- `Config` struct 新增 `DatabaseURL string`
- 数据库路径解析逻辑扩展：有 `DatabaseURL` 时返回 URL，否则走现有路径解析

`internal/app` 改动：

- 从 config 取 `DatabaseURL`，传给 `storage.Open(dbURL)`

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
  5. Query 编译使用 `ILIKE` / `DOUBLE PRECISION`

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
