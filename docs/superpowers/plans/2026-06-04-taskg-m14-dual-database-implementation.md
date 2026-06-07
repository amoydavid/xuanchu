# M14 多数据库支持 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 Xuanchu 同时支持 SQLite 和 PostgreSQL 后端，通过配置选择，GORM 作为唯一抽象层。

**Architecture:** 重命名 `internal/storage` 为 `internal/storage`，拆分 SQLite/PostgreSQL 的 Open 和 Migration 逻辑，在 `query_scope.go` 中适配 SQL 方言。不引入 repository interface，所有 repo 保持具体 struct 直接使用 `*gorm.DB`。

**Tech Stack:** Go 1.25, GORM, `github.com/glebarez/sqlite`（SQLite）, `gorm.io/driver/postgres` + `github.com/jackc/pgx/v5`（PostgreSQL）, Cobra

**Spec:** `docs/superpowers/specs/2026-06-04-xuanchu-m14-dual-database-design.md`

---

## Chunk 1: 包重命名 + 依赖引入

### Task 1: 机械重命名 internal/storage → internal/storage

**Files:**
- Move: `internal/storage/*` → `internal/storage/*`
- Modify: 所有 import `github.com/dajee/xuanchu/internal/storage` 的文件（36 个 .go 源文件）
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: 执行包重命名**

```bash
mkdir -p internal/storage
git mv internal/storage/* internal/storage/
rm -rf internal/storage
```

- [ ] **Step 2: 替换包声明和 import 路径**

```bash
# 替换 storage 目录内的包声明
find internal/storage -name '*.go' -exec sed -i '' 's/^package sqlite$/package storage/' {} +

# 替换所有 Go 源文件的 import 路径
find . -name '*.go' -exec sed -i '' 's|"github.com/dajee/xuanchu/internal/storage"|"github.com/dajee/xuanchu/internal/storage"|g' {} +
```

- [ ] **Step 3: 替换所有 `sqlite.` qualified identifier 为 `storage.`**

在所有 Go 源文件中，将 `sqlite.Open` → `storage.Open`、`sqlite.Store` → `storage.Store`、`sqlite.NewTaskRepository` → `storage.NewTaskRepository` 等。使用 `sed` 进行批量替换：

```bash
# 在非 storage 目录的 Go 文件中替换 sqlite. → storage.
find . -name '*.go' -not -path './internal/storage/*' -exec sed -i '' 's/sqlite\./storage./g' {} +
```

注意：此替换可能误伤包含 "sqlite" 字样的字符串内容（如错误消息 "sqlite"）。替换后需手动检查以下场景：
- `github.com/glebarez/sqlite` import 路径（仅在 `db_sqlite.go` 中使用，不会被匹配）
- 字符串中的 "sqlite" 字样（如测试中的 driver 名、错误消息）—— 这些不应被替换

- [ ] **Step 4: 验证替换结果**

```bash
# 确认没有残留的 sqlite. 引用（排除合理的字符串内容）
grep -rn 'sqlite\.' --include='*.go' . | grep -v '_test.go' | grep -v vendor | grep -v '//.*sqlite'
```

预期：无残留的 `sqlite.` 限定符引用（字符串字面量中的 "sqlite" 除外）。

- [ ] **Step 5: 更新文档中的引用**

```bash
find docs -name '*.md' -exec sed -i '' 's|internal/storage|internal/storage|g' {} +
```

同步更新 `AGENTS.md` 中所有 `internal/storage` 引用为 `internal/storage`。

- [ ] **Step 6: 编译验证**

```bash
CGO_ENABLED=0 go build ./...
```

预期：编译通过，无 import 错误。

- [ ] **Step 7: 运行全量测试**

```bash
CGO_ENABLED=0 go test ./...
```

预期：全部测试通过，行为不变。

- [ ] **Step 8: 提交**

```bash
git add -A
git commit -m "refactor: 重命名 internal/storage → internal/storage"
```

### Task 2: 引入 PostgreSQL 依赖

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: 添加 PostgreSQL GORM driver**

```bash
go get gorm.io/driver/postgres
```

这会自动拉取 `github.com/jackc/pgx/v5` 及其依赖。

- [ ] **Step 2: 验证零 CGO 构建**

```bash
CGO_ENABLED=0 go build ./cmd/xuanchu
CGO_ENABLED=0 go test ./...
```

预期：编译通过，测试通过。`pgx` 是纯 Go，不影响 CGO 状态。

- [ ] **Step 3: 提交**

```bash
git add go.mod go.sum
git commit -m "chore: 引入 gorm.io/driver/postgres 依赖"
```

---

## Chunk 2: Open 路由 + Store dialect

### Task 3: 拆分 Open 逻辑为 db_sqlite.go 和 db_postgres.go

**Files:**
- Modify: `internal/storage/db.go` — 抽取 dialect 判断，保留公共逻辑
- Create: `internal/storage/db_sqlite.go` — `openSQLite()`, `configureSQLite()`, `sqliteDSN()`
- Create: `internal/storage/db_postgres.go` — `openPostgres()`, `configurePostgres()`
- Create: `internal/storage/db_test.go` 补充路由测试

`Store` struct 变更：

```go
type Store struct {
    db      *gorm.DB
    dialect string // "sqlite" | "postgres"
}

func (s *Store) Dialect() string { return s.dialect }
```

`Open` 函数路由逻辑（含非 postgres URL 的 scheme 校验）：

```go
func Open(dbURL string) (*Store, error) {
    if isPostgresURL(dbURL) {
        return openPostgres(dbURL)
    }
    if strings.Contains(dbURL, "://") {
        return nil, fmt.Errorf("unsupported database scheme: %s", dbURL)
    }
    return openSQLite(dbURL)
}

func isPostgresURL(s string) bool {
    return strings.HasPrefix(s, "postgres://") || strings.HasPrefix(s, "postgresql://")
}
```

`openSQLite(path string)` 包含当前 `Open` 的全部 SQLite 逻辑（`os.MkdirAll`、`sqlite.Open`、`configureSQLite`、`migrate`、`ensureLocalIdentity`）。`openSQLite` 保留空路径检查（`"database path is required"`），`Open("")` 会自然落入 `openSQLite("")` 并报错。

`openPostgres(dbURL string)`：

```go
func openPostgres(dbURL string) (*Store, error) {
    db, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
    if err != nil {
        return nil, err
    }
    store := &Store{db: db, dialect: "postgres"}
    if err := configurePostgres(store); err != nil {
        _ = store.Close()
        return nil, err
    }
    if err := store.migrate(); err != nil {
        _ = store.Close()
        return nil, err
    }
    if err := store.ensureLocalIdentity(); err != nil {
        _ = store.Close()
        return nil, err
    }
    return store, nil
}

func configurePostgres(s *Store) error {
    // 预留：连接池配置（SetMaxOpenConns, SetMaxIdleConns, ConnMaxLifetime）
    // 当前使用 pgx/GORM 默认值
    return nil
}
```

- [ ] **Step 1: 修改 Store struct，新增 dialect 字段和 Dialect() 方法**

在 `internal/storage/db.go` 中：
- `Store` struct 新增 `dialect string`
- 新增 `func (s *Store) Dialect() string`
- 当前 `Open` 函数设置 `dialect: "sqlite"`

- [ ] **Step 2: 从 db.go 抽取 SQLite 专有函数到 db_sqlite.go**

将以下函数从 `db.go` 移到 `internal/storage/db_sqlite.go`：
- `sqliteDSN(path string) string`
- `func (s *Store) configureSQLite() error`（原 `configure()`，重命名）
- `func openSQLite(path string) (*Store, error)` — 包含原 `Open` 的全部 SQLite 逻辑，内部调用 `s.configureSQLite()` 而非 `s.configure()`

`db_sqlite.go` 的 import 包含 `github.com/glebarez/sqlite`。

- [ ] **Step 3: 创建 db_postgres.go**

创建 `internal/storage/db_postgres.go`，包含 `openPostgres` 和 `configurePostgres`（见上方代码）。

`db_postgres.go` 的 import 包含 `gorm.io/driver/postgres`。

- [ ] **Step 4: 重写 db.go 中的 Open 为路由入口**

将 `db.go` 中的 `Open` 替换为路由版本（见上方代码），删除旧的 `Open` 实现。

- [ ] **Step 5: 写路由测试**

在 `internal/storage/db_test.go` 新增：

```go
func TestIsPostgresURL(t *testing.T) {
    tests := []struct{ input string; want bool }{
        {"postgres://user:pass@host:5432/db", true},
        {"postgresql://user:pass@host:5432/db", true},
        {"", false},
        {"/path/to/xuanchu.db", false},
        {"xuanchu.db", false},
        {"mysql://host/db", false},
    }
    for _, tt := range tests {
        if got := isPostgresURL(tt.input); got != tt.want {
            t.Errorf("isPostgresURL(%q) = %v, want %v", tt.input, got, tt.want)
        }
    }
}

func TestOpen_UnsupportedScheme(t *testing.T) {
    _, err := Open("mysql://host/db")
    if err == nil {
        t.Error("expected error for unsupported scheme")
    }
    if !strings.Contains(err.Error(), "unsupported database scheme") {
        t.Errorf("unexpected error: %v", err)
    }
}
```

- [ ] **Step 6: 编译 + 全量测试**

```bash
CGO_ENABLED=0 go build ./cmd/xuanchu
CGO_ENABLED=0 go test ./...
```

预期：全部通过。此时只有 SQLite 路径被使用，行为不变。

- [ ] **Step 7: 提交**

```bash
git add internal/storage/
git commit -m "feat: 拆分 Open 路由，支持 SQLite/PostgreSQL dialect 选择"
```

---

## Chunk 3: Migration 分离

### Task 4: 拆分 Migration 逻辑

**Files:**
- Create: `internal/storage/migrate.go` — `migrate()` 入口
- Create: `internal/storage/migrate_sqlite.go` — SQLite 专有 M4/M5 迁移
- Create: `internal/storage/migrate_postgres.go` — PostgreSQL 纯净建表
- Modify: `internal/storage/db.go` — 移除 migrate 相关代码

- [ ] **Step 1: 创建 migrate.go**

从 `db.go` 中提取 `migrate()` 方法，改为按 dialect 分发：

```go
func (s *Store) migrate() error {
    switch s.dialect {
    case "sqlite":
        return s.migrateSQLite()
    case "postgres":
        return s.migratePostgres()
    default:
        return fmt.Errorf("unsupported dialect: %s", s.dialect)
    }
}
```

- [ ] **Step 2: 创建 migrate_sqlite.go**

将 `db.go` 中以下函数精确移到 `migrate_sqlite.go`：
- `func (s *Store) migrateSQLite() error` — 包含 `prepareWorkspaceSchemaForM4` + 第一批 `AutoMigrate`（Meta/User/Workspace/.../UserExternalID）+ `prepareProjectSchemaForM5`
- `func (s *Store) prepareWorkspaceSchemaForM4() error`
- `func (s *Store) prepareProjectSchemaForM5() error`
- `type m5MigrationTx struct { ... }` 及其方法 `exec`, `query`, `queryRow`
- `func migrateM4TaskProjectsToM5(tx m5MigrationTx) error`
- `func collectM5ProjectPlans(tx m5MigrationTx) ([]m5ProjectPlan, m5SkippedResult)`
- `func copyM4TasksToM5(tx m5MigrationTx) error`
- `func createM5TasksSchema(tx m5MigrationTx) error`
- `func dropM4TaskIndexes(tx m5MigrationTx) error`
- `func dropLegacyTaskProjectIndexes(tx m5MigrationTx) error`
- `func rebuildM5TaskRelationForeignKeys(tx m5MigrationTx) error`
- `func rebuildM5TaskRelationTable(tx m5MigrationTx, schema m5TaskRelationSchema) error`
- `func dropM5TaskRelationIndexes(tx m5MigrationTx, table string) error`
- `func relationHasTaskForeignKey(tx m5MigrationTx, table string) (bool, error)`
- `func assertNoM5ForeignKeyViolations(tx m5MigrationTx) error`
- `func writeM5SkippedReport(tx m5MigrationTx, rows []m5SkippedProject) error`
- `func setMetaInTx(tx m5MigrationTx, key, value string) error`
- `func tableExists(tx m5MigrationTx, table string) (bool, error)`
- `func m5ProjectsMarkerApplied(tx m5MigrationTx) (bool, error)`
- `func tasksHasM5ProjectForeignKey(tx m5MigrationTx) (bool, error)`
- `func isValidM5ProjectSlug(slug string) bool`
- `func skippedProject(candidate m5ProjectCandidate, reason string) m5SkippedProject`
- 全部 M5 相关类型：`m5ProjectPlan`, `m5SkippedResult`, `m5ProjectCandidate`, `m5SkippedProject`
- 全局变量 `var m5TaskRelationSchemas []m5TaskRelationSchema` 和 `type m5TaskRelationSchema struct { ... }`

`migration_m5_tasks.go` 保持不变（只包含 DDL 常量字符串 `m5TasksDDL` 和 `m5TaskIndexes`、`m4TaskIndexNames`）。

- [ ] **Step 3: 创建 migrate_postgres.go**

PostgreSQL 纯净建表，显式包含 `&Task{}`：

```go
func (s *Store) migratePostgres() error {
    if err := s.db.AutoMigrate(
        &Meta{}, &User{}, &Workspace{}, &Membership{},
        &AuditLog{}, &Project{}, &ProjectAnnotation{}, &Config{}, &ApiToken{},
        &Context{}, &UDADefinition{}, &HookDefinition{}, &HookDelivery{},
        &UserExternalID{}, &Task{},
    ); err != nil {
        return err
    }
    return s.db.AutoMigrate(
        &TaskTag{}, &TaskAnnotation{}, &TaskDependency{},
        &TaskAssignee{}, &TaskUDAValue{}, &TaskLink{},
    )
}
```

两批 `AutoMigrate` 的顺序与当前 SQLite 路径一致：主表先建，关联表后建。

- [ ] **Step 4: 确认 models.go 无需改动**

`TaskUDAValue.Orphan` 的 GORM tag `gorm:"not null;default:false"` — GORM 在 SQLite 映射为 `NUMERIC`，PostgreSQL 映射为 `boolean`。手写 DDL 中的 `orphan NUMERIC` 只在 SQLite M5 迁移中使用，PostgreSQL 不执行。无需改动 models.go。

- [ ] **Step 5: 编译 + 全量测试**

```bash
CGO_ENABLED=0 go build ./cmd/xuanchu
CGO_ENABLED=0 go test ./...
```

预期：全部通过。

- [ ] **Step 6: 提交**

```bash
git add internal/storage/
git commit -m "feat: 拆分 migration 为 SQLite/PostgreSQL 独立路径"
```

---

## Chunk 4: Query 方言适配

### Task 5: query_scope.go 方言适配

**Files:**
- Modify: `internal/storage/query_scope.go` — 新增 dialect 参数，替换 LIKE/CAST
- Modify: `internal/storage/task_repo.go` — `ListOptions` 新增 Dialect 字段
- Modify: `internal/storage/query_scope_test.go` — 新增方言测试

- [ ] **Step 1: 在 QueryCompileOptions 新增 Dialect 字段**

```go
type QueryCompileOptions struct {
    WorkspaceID    string
    NowUnix        int64
    Location       *time.Location
    UDADefinitions map[string]string
    Dialect        string // "sqlite" | "postgres"
}
```

- [ ] **Step 2: 添加 dialect helper 函数**

```go
func likeOp(dialect string) string {
    if dialect == "postgres" {
        return "ILIKE"
    }
    return "LIKE"
}

func realCastType(dialect string) string {
    if dialect == "postgres" {
        return "DOUBLE PRECISION"
    }
    return "REAL"
}
```

- [ ] **Step 3: 修改 compareColumn 签名，加入 dialect 参数**

将 `compareColumn(column string, op query.Operator, value string, intValue *int64)` 改为 `compareColumn(column string, op query.Operator, value string, intValue *int64, dialect string)`。

更新所有 `compareColumn` 调用点（约 8 处：`compilePredicate` 中的 AttrStatus/AttrProjectID/AttrPriority/AttrUUID/AttrDescription/AttrRecur/AttrParent 和 `compareDateColumn` 中的最终调用），传入 `opts.Dialect`。

在 `compareColumn` 的 `OpContains` case 中使用 `likeOp(dialect)`：
```go
case query.OpContains:
    return fmt.Sprintf("%s %s ?", column, likeOp(dialect)), []any{"%" + value + "%"}, nil
```

- [ ] **Step 4: 替换 compilePredicate 中的 LIKE**

共 3 处：

1. AttrBare（第 101 行）：`"description LIKE ?"` → `fmt.Sprintf("description %s ?", likeOp(opts.Dialect))`
2. AttrDescription OpEqual/OpContains（第 108 行）：同上
3. AttrAnnotations OpContains（第 152 行）：`"... task_annotations.description LIKE ?"` → 使用 `likeOp(opts.Dialect)`

- [ ] **Step 5: 替换 compileUDAPredicate 中的 LIKE 和 CAST**

LIKE 1 处（UDA string，第 279 行）：`"task_uda_values.value LIKE ?"` → 使用 `likeOp`

CAST 3 处（第 241-244 行，`=`, `<`, `>`）：

```go
rt := realCastType(opts.Dialect)
compareSQL = fmt.Sprintf("CAST(task_uda_values.value AS %s) = CAST(? AS %s)", rt, rt)
```

- [ ] **Step 6: 在 ListOptions 中新增 Dialect 并传递到 QueryCompileOptions**

在 `internal/storage/task_repo.go` 中：

```go
type ListOptions struct {
    Status         string
    Sort           string
    Query          query.Expr
    NowUnix        int64
    UDADefinitions map[string]string
    Limit          int
    Dialect        string // 新增
}
```

在 `List()` 方法中构建 `QueryCompileOptions` 时加入 `Dialect: opts.Dialect`。

- [ ] **Step 7: 写方言单元测试**

在 `internal/storage/query_scope_test.go` 新增：

```go
func TestCompileQuery_PostgresDialect(t *testing.T) {
    opts := QueryCompileOptions{WorkspaceID: "ws1", NowUnix: 1000000, Dialect: "postgres"}
    expr := query.Predicate{Attribute: query.AttrBare, Operator: query.OpEqual, Value: query.ParseDateValue("hello")}
    sql, _, err := CompileQuery(expr, opts)
    if err != nil {
        t.Fatal(err)
    }
    if !strings.Contains(sql, "ILIKE") {
        t.Errorf("expected ILIKE in SQL for postgres, got: %s", sql)
    }
}

func TestCompileQuery_SQLiteDialect(t *testing.T) {
    opts := QueryCompileOptions{WorkspaceID: "ws1", NowUnix: 1000000, Dialect: "sqlite"}
    expr := query.Predicate{Attribute: query.AttrBare, Operator: query.OpEqual, Value: query.ParseDateValue("hello")}
    sql, _, err := CompileQuery(expr, opts)
    if err != nil {
        t.Fatal(err)
    }
    if !strings.Contains(sql, " LIKE ") {
        t.Errorf("expected LIKE in SQL for sqlite, got: %s", sql)
    }
}

func TestCompileQuery_UDANumericPostgres(t *testing.T) {
    opts := QueryCompileOptions{
        WorkspaceID: "ws1", NowUnix: 1000000, Dialect: "postgres",
        UDADefinitions: map[string]string{"score": "numeric"},
    }
    expr := query.Predicate{Attribute: query.AttrUDA, Field: "score", Operator: query.OpEqual, Value: query.ParseDateValue("42")}
    sql, _, err := CompileQuery(expr, opts)
    if err != nil {
        t.Fatal(err)
    }
    if !strings.Contains(sql, "DOUBLE PRECISION") {
        t.Errorf("expected DOUBLE PRECISION for postgres UDA numeric, got: %s", sql)
    }
}

func TestCompileQuery_CompareColumnContains(t *testing.T) {
    opts := QueryCompileOptions{WorkspaceID: "ws1", NowUnix: 1000000, Dialect: "postgres"}
    expr := query.Predicate{Attribute: query.AttrStatus, Operator: query.OpContains, Value: query.ParseDateValue("pend")}
    sql, _, err := CompileQuery(expr, opts)
    if err != nil {
        t.Fatal(err)
    }
    if !strings.Contains(sql, "ILIKE") {
        t.Errorf("expected ILIKE for postgres OpContains, got: %s", sql)
    }
}
```

- [ ] **Step 8: 运行测试**

```bash
go test ./internal/storage/ -run "TestCompileQuery_" -v
```

预期：全部通过。

- [ ] **Step 9: 全量编译 + 测试**

```bash
CGO_ENABLED=0 go build ./cmd/xuanchu
CGO_ENABLED=0 go test ./...
```

预期：全部通过。

- [ ] **Step 10: 提交**

```bash
git add internal/storage/query_scope.go internal/storage/query_scope_test.go internal/storage/task_repo.go
git commit -m "feat: query 编译适配 SQLite/PostgreSQL 方言（LIKE/ILIKE, CAST 类型）"
```

### Task 6: App 层传入 Dialect

**Files:**
- Modify: `internal/app/service.go` — 在所有 `storage.ListOptions{...}` 构造中传入 `Dialect: s.store.Dialect()`

**关键认知：** `QueryCompileOptions` 不在 `app/service.go` 中构造，而是在 `task_repo.go` 的 `List()` 方法中从 `ListOptions` 构建。因此 app 层需要将 dialect 传入 `ListOptions.Dialect`。

- [ ] **Step 1: 在 service.go 中所有 `storage.ListOptions{...}` 构造处加入 `Dialect: s.store.Dialect()`**

搜索 `storage.ListOptions` 的所有构造点（约 8 处），每处加入 `Dialect: s.store.Dialect()`。

- [ ] **Step 2: 编译 + 测试**

```bash
CGO_ENABLED=0 go build ./cmd/xuanchu
CGO_ENABLED=0 go test ./...
```

- [ ] **Step 3: 提交**

```bash
git add internal/app/service.go
git commit -m "feat: app 层传入 database dialect 到 query 编译"
```

---

## Chunk 5: 配置入口

### Task 7: 配置层新增 DatabaseURL

**Files:**
- Modify: `internal/config/config.go` — 新增 `DatabaseURL` 字段和解析逻辑
- Modify: `internal/config/config_test.go` — 新增 URL 优先级测试

- [ ] **Step 1: 扩展 Config 和 Options struct**

```go
type Config struct {
    DatabasePath string
    DatabaseURL  string
    RemoteServer string
    RemoteToken  string
    JSON         bool
    Color        bool
}

type Options struct {
    DataDir    string
    DBPath     string
    DBURL      string
    Server     string
    Token      string
    JSON       bool
    NoColor    bool
    Env        map[string]string
    HomeDir    string
}
```

- [ ] **Step 2: 修改 Resolve 函数 — 重构 TOML 加载为无条件执行**

当前 `Resolve` 中 TOML 加载是惰性的（`dbPath == ""` 时才加载）。新增 `database.url` 需要无条件读取 TOML。

先将现有 Resolve 中的 TOML 加载改为无条件（移到 `dbPath` 解析之前）：

```go
func Resolve(opts Options) (Config, error) {
    home := opts.HomeDir
    // ... 现有 home/env 初始化 ...
    
    // 无条件加载 TOML（database.url 需要读取）
    var tomlValues map[string]string
    if values, err := loadTomlConfig(configDir(home, env)); err == nil {
        tomlValues = values
    } else if !errors.Is(err, os.ErrNotExist) {
        return Config{}, err
    }
    
    // DatabaseURL 优先级链
    dbURL := opts.DBURL
    if dbURL == "" {
        dbURL = env["XUANCHU_DB_URL"]
    }
    if dbURL == "" && tomlValues != nil {
        dbURL = tomlValues["database.url"]
    }
    // 仅 CLI flag 层面互斥
    if opts.DBURL != "" && opts.DBPath != "" {
        return Config{}, errors.New("--db-url and --db are mutually exclusive")
    }
    
    // DatabasePath（仅当 DatabaseURL 为空时解析）
    dbPath := ""
    if dbURL == "" {
        dbPath = opts.DBPath
        if dbPath == "" {
            dbPath = env["XUANCHU_DB"]
        }
        if dbPath == "" && opts.DataDir != "" {
            dbPath = filepath.Join(opts.DataDir, "xuanchu.db")
        }
        if dbPath == "" && tomlValues != nil {
            dbPath = tomlValues["database.path"]
        }
        if dbPath == "" && env["XDG_DATA_HOME"] != "" {
            dbPath = filepath.Join(env["XDG_DATA_HOME"], "xuanchu", "xuanchu.db")
        }
        if dbPath == "" {
            dbPath = filepath.Join(home, ".local", "share", "xuanchu", "xuanchu.db")
        }
    }
    
    if dbPath != "" && strings.Contains(dbPath, "://") {
        return Config{}, errors.New("--db accepts file paths only; use --db-url for database URLs")
    }
    
    // ... 现有 server/token 解析（不变） ...
    
    return Config{DatabasePath: dbPath, DatabaseURL: dbURL, RemoteServer: server, RemoteToken: token, JSON: opts.JSON, Color: !opts.NoColor}, nil
}
```

注意：需要新增 `"strings"` import。

- [ ] **Step 3: 更新 environ() 函数**

```go
func environ() map[string]string {
    values := map[string]string{}
    for _, key := range []string{"XUANCHU_DB", "XUANCHU_DB_URL", "XUANCHU_SERVER", "XUANCHU_TOKEN", "XDG_DATA_HOME", "XDG_CONFIG_HOME"} {
        if value := os.Getenv(key); value != "" {
            values[key] = value
        }
    }
    return values
}
```

- [ ] **Step 4: 写配置测试**

```go
func TestResolve_DBURLAndDBPathMutualExclusion(t *testing.T) {
    _, err := Resolve(Options{DBURL: "postgres://host/db", DBPath: "/path/to.db", HomeDir: "/home"})
    if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
        t.Errorf("expected mutual exclusion error, got: %v", err)
    }
}

func TestResolve_DBURLOnly(t *testing.T) {
    cfg, err := Resolve(Options{DBURL: "postgres://host/db", HomeDir: "/home"})
    if err != nil {
        t.Fatal(err)
    }
    if cfg.DatabaseURL != "postgres://host/db" {
        t.Errorf("expected DatabaseURL, got %q", cfg.DatabaseURL)
    }
    if cfg.DatabasePath != "" {
        t.Errorf("expected empty DatabasePath, got %q", cfg.DatabasePath)
    }
}

func TestResolve_DBURLEnvVar(t *testing.T) {
    cfg, err := Resolve(Options{
        HomeDir: "/home",
        Env:     map[string]string{"XUANCHU_DB_URL": "postgres://env/db"},
    })
    if err != nil {
        t.Fatal(err)
    }
    if cfg.DatabaseURL != "postgres://env/db" {
        t.Errorf("expected DatabaseURL from env, got %q", cfg.DatabaseURL)
    }
}

func TestResolve_DBPathRejectsURL(t *testing.T) {
    _, err := Resolve(Options{DBPath: "postgres://host/db", HomeDir: "/home"})
    if err == nil {
        t.Error("expected error when --db receives a URL")
    }
}

func TestResolve_DBPathRejectsAnyScheme(t *testing.T) {
    _, err := Resolve(Options{DBPath: "mysql://host/db", HomeDir: "/home"})
    if err == nil {
        t.Error("expected error when --db receives any URL scheme")
    }
}

func TestResolve_DBURLEnvOverridesDBEnv(t *testing.T) {
    cfg, err := Resolve(Options{
        HomeDir: "/home",
        Env:     map[string]string{"XUANCHU_DB_URL": "postgres://env/db", "XUANCHU_DB": "/path/to.db"},
    })
    if err != nil {
        t.Fatal(err)
    }
    if cfg.DatabaseURL != "postgres://env/db" {
        t.Errorf("expected XUANCHU_DB_URL to take precedence, got %q", cfg.DatabaseURL)
    }
    if cfg.DatabasePath != "" {
        t.Errorf("expected empty DatabasePath when DatabaseURL is set, got %q", cfg.DatabasePath)
    }
}
```

- [ ] **Step 5: 运行配置测试**

```bash
go test ./internal/config/ -v
```

预期：全部通过。

- [ ] **Step 6: 全量编译 + 测试**

```bash
CGO_ENABLED=0 go build ./cmd/xuanchu
CGO_ENABLED=0 go test ./...
```

- [ ] **Step 7: 提交**

```bash
git add internal/config/
git commit -m "feat: 配置层新增 DatabaseURL 支持（--db-url / XUANCHU_DB_URL / TOML）"
```

### Task 8: CLI 层注册 --db-url flag

**Files:**
- Modify: `internal/cli/root.go` — 注册 flag、更新 `stringFlags` map、更新 `optionsFromCmd`
- Modify: `internal/cli/server.go` — 使用 `cfg.DatabaseURL` 或 `cfg.DatabasePath`
- Modify: `internal/cli/config.go` — 同上
- Modify: `internal/cli/mcp.go` — 同上
- Modify: `cmd/xuanchu/main.go` — 更新 M5 migration warning 逻辑、使用新配置

- [ ] **Step 1: 在 Options struct 新增 DBURL 字段**

在 `internal/cli/root.go` 的 `Options` struct 中新增 `DBURL string`。

- [ ] **Step 2: 注册 --db-url flag**

在 root command 的 `PersistentFlags` 中新增：

```go
cmd.PersistentFlags().StringVar(&opts.DBURL, "db-url", opts.DBURL, "Database URL (postgres://...); mutually exclusive with --db")
```

- [ ] **Step 3: 将 `"--db-url"` 加入 stringFlags map**

在 `splitFlagsRcAndPositional` 和 `handleTargetAction` 中的 `stringFlags` map 中加入 `"--db-url": true`（注意：代码库中 stringFlags 使用 `--` 前缀格式，如 `"--db": true`）。

- [ ] **Step 4: 在 optionsFromCmd 中提取 --db-url 值**

在 `optionsFromCmd` 函数中加入：

```go
opts.DBURL = getCmdStringFlag(cmd, "db-url", opts.DBURL)
```

- [ ] **Step 5: 传递 DBURL 到 config.Options**

在所有构建 `config.Options` 的地方，加入 `DBURL: opts.DBURL`。精确位置：
- `internal/cli/root.go` 的 `buildServiceFromOpts` 函数（约第 708 行）
- `internal/cli/server.go` 的 `newServerCommand` 函数（约第 34 行）
- `internal/cli/mcp.go` 的 `newMCPStdioCommand` 函数（约第 34 行）
- `internal/cli/config.go` 的 `runtimeFromOptions` 函数（约第 277 行）
- `cmd/xuanchu/main.go` 的 `warningOptionsFromArgs` 函数（约第 47 行）

- [ ] **Step 6: 更新 `cli.RuntimeEnv()` 以包含 `XUANCHU_DB_URL`**

在 `internal/cli/helper.go`（或 `RuntimeEnv()` 定义所在的文件）的 `RuntimeEnv()` 函数中，新增 `"XUANCHU_DB_URL"` 到环境变量列表。这很关键：所有生产代码路径通过 `opts.Env` 传入 `cli.RuntimeEnv()` 返回值，如果不加，`XUANCHU_DB_URL` 环境变量在运行时不会被读取。

- [ ] **Step 7: 修改 store 初始化逻辑**

在所有调用 `storage.Open(...)` 的地方（`root.go`、`server.go`、`config.go`、`mcp.go`），改为：

```go
dbTarget := cfg.DatabaseURL
if dbTarget == "" {
    dbTarget = cfg.DatabasePath
}
store, err := storage.Open(dbTarget)
```

- [ ] **Step 8: 更新 cmd/xuanchu/main.go 中的 M5 migration warning**

`maybeWarnM5Migration` 的实际签名为 `func maybeWarnM5Migration(w io.Writer, args []string, opts cli.Options)`。

修改内容：
1. 更新 `warningOptionsFromArgs` 函数以解析 `--db-url` 参数（仿照 `--db` 的解析方式）
2. 在 `maybeWarnM5Migration` 内部，通过 `warningOptionsFromArgs` 获取完整的 `cli.Options`（含 DBURL），然后构建 `config.Options` 并调用 `config.Resolve`
3. 从返回的 `config.Config` 中取 `DatabaseURL || DatabasePath` 作为 dbTarget
4. 如果 dbTarget 是 PostgreSQL URL（`strings.HasPrefix(dbTarget, "postgres://")`），直接 return

```go
func maybeWarnM5Migration(w io.Writer, args []string, opts cli.Options) {
    // 从 opts 构建 config
    cfg, err := config.Resolve(config.Options{
        DBPath:  opts.DBPath,
        DBURL:   opts.DBURL,
        HomeDir: opts.HomeDir,
        Env:     opts.Env,
    })
    if err != nil {
        return
    }
    dbTarget := cfg.DatabaseURL
    if dbTarget == "" {
        dbTarget = cfg.DatabasePath
    }
    if strings.HasPrefix(dbTarget, "postgres://") || strings.HasPrefix(dbTarget, "postgresql://") {
        return
    }
    // ... 现有 SQLite M5 迁移 warning 逻辑（使用 dbTarget 打开 store） ...
}
```

- [ ] **Step 9: 全量编译 + 测试**

```bash
CGO_ENABLED=0 go build ./cmd/xuanchu
CGO_ENABLED=0 go test ./...
```

- [ ] **Step 10: 手动验证**

```bash
./xuanchu --db-url "invalid://host/db" list
# 预期：错误 "unsupported database scheme"

./xuanchu --db-url "postgres://host/db" --db ./xuanchu.db list
# 预期：错误 "mutually exclusive"

./xuanchu --db "./xuanchu.db" list
# 预期：正常工作（SQLite 行为不变）

./xuanchu --db "postgres://host/db" list
# 预期：错误 "--db accepts file paths only"
```

- [ ] **Step 10: 提交**

```bash
git add internal/cli/ cmd/xuanchu/
git commit -m "feat: CLI 注册 --db-url flag，store 初始化支持 PostgreSQL"
```

---

## Chunk 6: PostgreSQL 集成测试 + 文档更新

### Task 9: PostgreSQL 集成测试

**Files:**
- Create: `internal/storage/postgres_test.go`

- [ ] **Step 1: 创建 postgres_test.go**

```go
package storage

import (
    "os"
    "testing"
    "time"

    "github.com/dajee/xuanchu/internal/task"
    "github.com/google/uuid"
)

func postgresTestURL(t *testing.T) string {
    t.Helper()
    url := os.Getenv("XUANCHU_TEST_DB_URL")
    if url == "" {
        t.Skip("XUANCHU_TEST_DB_URL not set, skipping PostgreSQL tests")
    }
    return url
}

func TestOpen_Postgres(t *testing.T) {
    dbURL := postgresTestURL(t)
    store, err := Open(dbURL)
    if err != nil {
        t.Fatalf("Open(%q): %v", dbURL, err)
    }
    defer store.Close()
    if store.Dialect() != "postgres" {
        t.Errorf("expected dialect postgres, got %q", store.Dialect())
    }
}

func TestPostgres_Migration(t *testing.T) {
    dbURL := postgresTestURL(t)
    store, err := Open(dbURL)
    if err != nil {
        t.Fatal(err)
    }
    defer store.Close()
    db := store.DB()
    tables := []string{"meta", "users", "workspaces", "memberships",
        "audit_logs", "projects", "project_annotations", "configs",
        "api_tokens", "contexts", "uda_definitions", "hook_definitions",
        "hook_deliveries", "user_external_ids", "tasks",
        "task_tags", "task_annotations", "task_dependencies",
        "task_assignees", "task_uda_values", "task_links"}
    for _, table := range tables {
        if !db.Migrator().HasTable(table) {
            t.Errorf("table %q not found after migration", table)
        }
    }
}

func TestPostgres_EnsureLocalIdentity(t *testing.T) {
    dbURL := postgresTestURL(t)
    store, err := Open(dbURL)
    if err != nil {
        t.Fatal(err)
    }
    defer store.Close()
    ws, err := store.LocalWorkspace()
    if err != nil {
        t.Fatalf("LocalWorkspace(): %v", err)
    }
    if ws.Slug != "local" {
        t.Errorf("expected local workspace slug, got %q", ws.Slug)
    }
}

func TestPostgres_TaskCRUD(t *testing.T) {
    dbURL := postgresTestURL(t)
    store, err := Open(dbURL)
    if err != nil {
        t.Fatal(err)
    }
    defer store.Close()
    repo := NewTaskRepository(store.DB())
    ws, _ := store.LocalWorkspace()
    now := time.Now().Unix()
    tsk := task.Task{
        UUID:        uuid.NewString(),
        WorkspaceID: ws.ID,
        Description: "PostgreSQL test task",
        Status:      "pending",
        Entry:       now,
        Modified:    now,
    }
    created, err := repo.Create(tsk)
    if err != nil {
        t.Fatalf("Create(): %v", err)
    }
    if created.UUID != tsk.UUID {
        t.Errorf("UUID mismatch: %q vs %q", created.UUID, tsk.UUID)
    }
}
```

- [ ] **Step 2: 运行 SQLite 测试确认无回归**

```bash
CGO_ENABLED=0 go test ./internal/storage/ -v
```

预期：全部 SQLite 测试通过，PostgreSQL 测试被 skip（无 `XUANCHU_TEST_DB_URL`）。

- [ ] **Step 3: 提交**

```bash
git add internal/storage/postgres_test.go
git commit -m "test: 新增 PostgreSQL 集成测试（XUANCHU_TEST_DB_URL 驱动）"
```

### Task 10: CLI 集成测试 PostgreSQL 支持（可选）

**Files:**
- Modify: `tests/integration/cli_test.go` — 支持通过环境变量切换数据库

此任务为可选。M14 验收不要求 CLI 集成测试在 PostgreSQL 下通过，只要求 SQLite 集成测试不回退。

- [ ] **Step 1: 在测试 helper 中检测 XUANCHU_TEST_DB_URL**

在集成测试的 DB 初始化 helper 中：

```go
func testDBArgs(t *testing.T) (args []string, cleanup func()) {
    t.Helper()
    if dbURL := os.Getenv("XUANCHU_TEST_DB_URL"); dbURL != "" {
        return []string{"--db-url", dbURL}, func() {}
    }
    dir := t.TempDir()
    return []string{"--db", filepath.Join(dir, "xuanchu.db")}, func() {}
}
```

PostgreSQL 集成测试间的数据清理方案为后续改进项。M14 仅确保 SQLite 集成测试不回退。

- [ ] **Step 2: 运行集成测试**

```bash
CGO_ENABLED=0 go test ./tests/integration/ -v
```

预期：SQLite 集成测试全部通过。

- [ ] **Step 3: 提交**

```bash
git add tests/integration/
git commit -m "test: 集成测试预留 XUANCHU_TEST_DB_URL 支持（可选）"
```

### Task 11: 文档更新

**Files:**
- Modify: `README.md` — 新增 `--db-url` 文档、PostgreSQL 说明
- Modify: `ROADMAP.md` — 更新 M14 状态
- Modify: `AGENTS.md` — 更新技术栈、存储层路径、全局 flag 列表

- [ ] **Step 1: 更新 README.md**

在全局参数部分新增 `--db-url` 说明，在单独小节介绍 PostgreSQL 支持。

- [ ] **Step 2: 更新 ROADMAP.md**

M14 行状态改为"已完成"。更新"当前下一步"节。

- [ ] **Step 3: 更新 AGENTS.md**

- 第 2 节技术栈：新增 `gorm.io/driver/postgres`、`github.com/jackc/pgx/v5`（纯 Go PostgreSQL driver）
- 第 8 节存储层：`internal/storage`（不再是 `internal/storage`），新增 PostgreSQL 支持说明
- 第 10 节全局 flag：新增 `--db-url`、`XUANCHU_DB_URL`

- [ ] **Step 4: 提交**

```bash
git add README.md ROADMAP.md AGENTS.md
git commit -m "docs: M14 多数据库支持文档更新"
```

### Task 12: 最终验证

- [ ] **Step 1: 全量编译 + 测试 + vet**

```bash
CGO_ENABLED=0 go vet ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```

预期：全部通过。

- [ ] **Step 2: 手动端到端验证**

```bash
# SQLite（行为不变）
./xuanchu --db /tmp/test.db project add test-project name:"Test"
./xuanchu --db /tmp/test.db add "Test task" project:test-project
./xuanchu --db /tmp/test.db list

# PostgreSQL（如果环境可用）
./xuanchu --db-url "postgres://user:pass@localhost:5432/xuanchu_test?sslmode=disable" project add test-project name:"Test"
./xuanchu --db-url "postgres://user:pass@localhost:5432/xuanchu_test?sslmode=disable" add "Test task" project:test-project
./xuanchu --db-url "postgres://user:pass@localhost:5432/xuanchu_test?sslmode=disable" list

# 互斥检查
./xuanchu --db-url "postgres://..." --db /tmp/test.db list
# 预期：错误

# --db URL 误用检查
./xuanchu --db "postgres://..." list
# 预期：错误

# 不支持的 scheme
./xuanchu --db-url "mysql://..." list
# 预期：错误 "unsupported database scheme"
```

- [ ] **Step 3: 提交最终状态**

```bash
git add -A
git commit -m "chore: M14 多数据库支持（SQLite / PostgreSQL）完成"
```
