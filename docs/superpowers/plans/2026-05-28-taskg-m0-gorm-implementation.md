# Xuanchu M0 GORM Implementation Plan

> **For agentic workers:** REQUIRED: Use `superpowers:subagent-driven-development` (if subagents available) or `superpowers:executing-plans` to implement this plan. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** 实现 `xuanchu` M0：一个本地单用户、单 workspace、SQLite 持久化的 Taskwarrior 风格 CLI，支持核心任务生命周期和 JSON 导入导出。

**Architecture:** 采用 `cmd/xuanchu -> internal/cli -> internal/app -> internal/{task,query,config} -> internal/storage` 的分层结构。CLI 只负责参数解析和输出，app service 负责用例与事务，storage 使用 GORM 封装 SQLite，domain/query/config 保持不依赖 GORM 和 Cobra。M0 保留 `workspace_id`，但只创建和使用隐式 `local` workspace。

**Tech Stack:** Go 1.22+、Cobra、GORM、`github.com/glebarez/sqlite`（纯 Go SQLite GORM driver，替代需要 CGO 的 `gorm.io/driver/sqlite`）、`github.com/google/uuid`、标准库 `encoding/json`、Go test。

---

## Chunk 1: 项目骨架、配置与数据库基础

### 文件职责总览

- 创建：`go.mod`  
  定义 Go module 与依赖。
- 创建：`cmd/xuanchu/main.go`  
  二进制入口，只调用 CLI root command。
- 创建：`internal/cli/root.go`  
  Cobra root、全局 flags、stdout/stderr 注入。
- 创建：`internal/config/config.go`  
  解析数据库路径、数据目录、JSON/color 偏好。
- 创建：`internal/config/config_test.go`  
  配置路径和环境变量优先级测试。
- 创建：`internal/storage/models.go`  
  GORM model：`Meta`、`Workspace`、`Task`、`TaskTag`。
- 创建：`internal/storage/db.go`  
  使用 `github.com/glebarez/sqlite` 打开数据库、设置 pragma、AutoMigrate、初始化 local workspace。
- 创建：`internal/storage/db_test.go`  
  数据库初始化、CGO-free driver、local workspace 测试。
- 创建：`internal/task/model.go`  
  domain task 类型与状态常量。
- 创建：`internal/app/service.go`  
  app service 空壳与依赖注入，为后续任务提供稳定入口。

### Task 1: 初始化 Go module 与最小 CLI

**Files:**

- Create: `go.mod`
- Create: `cmd/xuanchu/main.go`
- Create: `internal/cli/root.go`

- [x] **Step 1: 创建失败测试，验证 root command 可执行**

创建 `internal/cli/root_test.go`：

```go
package cli

import (
	"bytes"
	"testing"
)

func TestRootCommandVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd := NewRootCommand(Options{
		Stdout: &stdout,
		Stderr: &stderr,
		Version: "test",
	})
	cmd.SetArgs([]string{"--version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := stdout.String(); got != "xuanchu test\n" {
		t.Fatalf("stdout = %q, want %q", got, "xuanchu test\n")
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}
```

- [x] **Step 2: 运行测试，确认失败**

Run: `go test ./internal/cli -run TestRootCommandVersion -v`  
Expected: FAIL，原因是 module/package/function 尚不存在。

- [x] **Step 3: 添加最小实现**

创建 `go.mod`：

```go
module github.com/dajee/xuanchu

go 1.22

require github.com/spf13/cobra v1.8.1
```

创建 `internal/cli/root.go`：

```go
package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

type Options struct {
	Stdout io.Writer
	Stderr io.Writer
	Version string
}

func NewRootCommand(opts Options) *cobra.Command {
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	if opts.Version == "" {
		opts.Version = "dev"
	}

	cmd := &cobra.Command{
		Use:           "xuanchu",
		Short:         "Taskwarrior-style task manager",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       opts.Version,
	}
	cmd.SetOut(opts.Stdout)
	cmd.SetErr(opts.Stderr)
	cmd.SetVersionTemplate(fmt.Sprintf("xuanchu %s\n", opts.Version))
	return cmd
}
```

创建 `cmd/xuanchu/main.go`：

```go
package main

import (
	"fmt"
	"os"

	"github.com/dajee/xuanchu/internal/cli"
)

var version = "dev"

func main() {
	cmd := cli.NewRootCommand(cli.Options{
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Version: version,
	})
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "xuanchu:", err)
		os.Exit(1)
	}
}
```

- [x] **Step 4: 运行测试，确认通过**

Run: `go test ./internal/cli -run TestRootCommandVersion -v`  
Expected: PASS。

- [x] **Step 5: 整体测试**

Run: `go test ./...`  
Expected: PASS。

- [x] **Step 6: 提交**

```bash
git add go.mod go.sum cmd/xuanchu/main.go internal/cli/root.go internal/cli/root_test.go
git commit -m "chore: scaffold xuanchu cli"
```

如果当前目录不是 git 仓库，记录“跳过提交：not a git repository”，继续执行后续步骤。

### Task 2: 实现配置路径解析

**Files:**

- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Modify: `internal/cli/root.go`

- [x] **Step 1: 写失败测试**

创建 `internal/config/config_test.go`：

```go
package config

import (
	"path/filepath"
	"testing"
)

func TestResolveDatabasePathPrefersExplicitDB(t *testing.T) {
	env := map[string]string{
		"XUANCHU_DB": "/env/xuanchu.db",
	}
	cfg, err := Resolve(Options{
		DBPath: "/explicit/xuanchu.db",
		Env:    env,
		HomeDir: "/home/alice",
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if cfg.DatabasePath != "/explicit/xuanchu.db" {
		t.Fatalf("DatabasePath = %q", cfg.DatabasePath)
	}
}

func TestResolveDatabasePathUsesXuanchuDB(t *testing.T) {
	cfg, err := Resolve(Options{
		Env: map[string]string{
			"XUANCHU_DB": "/env/xuanchu.db",
		},
		HomeDir: "/home/alice",
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if cfg.DatabasePath != "/env/xuanchu.db" {
		t.Fatalf("DatabasePath = %q", cfg.DatabasePath)
	}
}

func TestResolveDatabasePathUsesXDGDataHome(t *testing.T) {
	cfg, err := Resolve(Options{
		Env: map[string]string{
			"XDG_DATA_HOME": "/xdg",
		},
		HomeDir: "/home/alice",
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	want := filepath.Join("/xdg", "xuanchu", "xuanchu.db")
	if cfg.DatabasePath != want {
		t.Fatalf("DatabasePath = %q, want %q", cfg.DatabasePath, want)
	}
}

func TestResolveDatabasePathUsesHomeFallback(t *testing.T) {
	cfg, err := Resolve(Options{
		Env:     map[string]string{},
		HomeDir: "/home/alice",
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	want := filepath.Join("/home/alice", ".local", "share", "xuanchu", "xuanchu.db")
	if cfg.DatabasePath != want {
		t.Fatalf("DatabasePath = %q, want %q", cfg.DatabasePath, want)
	}
}
```

- [x] **Step 2: 运行测试，确认失败**

Run: `go test ./internal/config -v`  
Expected: FAIL，原因是 package/function 尚不存在。

- [x] **Step 3: 实现配置解析**

创建 `internal/config/config.go`：

```go
package config

import (
	"errors"
	"os"
	"path/filepath"
)

type Config struct {
	DatabasePath string
	JSON         bool
	Color        bool
}

type Options struct {
	DataDir string
	DBPath  string
	JSON    bool
	NoColor bool
	Env     map[string]string
	HomeDir string
}

func Resolve(opts Options) (Config, error) {
	home := opts.HomeDir
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return Config{}, err
		}
	}
	if home == "" {
		return Config{}, errors.New("home directory is required")
	}

	env := opts.Env
	if env == nil {
		env = environ()
	}

	dbPath := opts.DBPath
	if dbPath == "" {
		dbPath = env["XUANCHU_DB"]
	}
	if dbPath == "" && opts.DataDir != "" {
		dbPath = filepath.Join(opts.DataDir, "xuanchu.db")
	}
	if dbPath == "" && env["XDG_DATA_HOME"] != "" {
		dbPath = filepath.Join(env["XDG_DATA_HOME"], "xuanchu", "xuanchu.db")
	}
	if dbPath == "" {
		dbPath = filepath.Join(home, ".local", "share", "xuanchu", "xuanchu.db")
	}

	return Config{
		DatabasePath: dbPath,
		JSON:         opts.JSON,
		Color:        !opts.NoColor,
	}, nil
}

func environ() map[string]string {
	values := map[string]string{}
	for _, key := range []string{"XUANCHU_DB", "XDG_DATA_HOME"} {
		if value := os.Getenv(key); value != "" {
			values[key] = value
		}
	}
	return values
}
```

- [x] **Step 4: 给 root command 添加全局 flags**

在 `internal/cli/root.go` 的 `Options` 增加：

```go
DataDir string
DBPath  string
JSON    bool
NoColor bool
```

在 `NewRootCommand` 中添加：

```go
cmd.PersistentFlags().StringVar(&opts.DataDir, "data-dir", opts.DataDir, "data directory")
cmd.PersistentFlags().StringVar(&opts.DBPath, "db", opts.DBPath, "SQLite database path")
cmd.PersistentFlags().BoolVar(&opts.JSON, "json", opts.JSON, "render JSON output")
cmd.PersistentFlags().BoolVar(&opts.NoColor, "no-color", opts.NoColor, "disable colored output")
```

- [x] **Step 5: 运行测试**

Run: `go test ./internal/config ./internal/cli -v`  
Expected: PASS。

- [x] **Step 6: 提交**

```bash
git add internal/config/config.go internal/config/config_test.go internal/cli/root.go
git commit -m "feat: resolve local xuanchu config"
```

### Task 3: 使用 GORM + 纯 Go SQLite 初始化数据库

**Files:**

- Create: `internal/storage/models.go`
- Create: `internal/storage/db.go`
- Create: `internal/storage/db_test.go`
- Modify: `go.mod`

- [x] **Step 1: 写失败测试**

创建 `internal/storage/db_test.go`：

```go
package sqlite

import (
	"path/filepath"
	"testing"
)

func TestOpenInitializesLocalWorkspace(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "xuanchu.db")

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace() error = %v", err)
	}
	if ws.ID == "" {
		t.Fatal("workspace ID is empty")
	}
	if ws.Slug != "local" {
		t.Fatalf("Slug = %q, want local", ws.Slug)
	}
	if ws.Name != "Local" {
		t.Fatalf("Name = %q, want Local", ws.Name)
	}
}

func TestOpenCanReopenExistingDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "xuanchu.db")

	store1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open first error = %v", err)
	}
	ws1, err := store1.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace first error = %v", err)
	}
	_ = store1.Close()

	store2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open second error = %v", err)
	}
	t.Cleanup(func() { _ = store2.Close() })
	ws2, err := store2.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace second error = %v", err)
	}
	if ws2.ID != ws1.ID {
		t.Fatalf("workspace ID changed: %q -> %q", ws1.ID, ws2.ID)
	}
}
```

- [x] **Step 2: 添加依赖并运行测试，确认失败**

Run:

```bash
go get gorm.io/gorm@latest github.com/glebarez/sqlite@latest github.com/google/uuid@latest
go test ./internal/storage -v
```

Expected: FAIL，原因是 storage package 尚未实现。

注意：不要使用 `gorm.io/driver/sqlite`，它默认依赖 `github.com/mattn/go-sqlite3`，会引入 CGO。M0 必须使用 `github.com/glebarez/sqlite`。

- [x] **Step 3: 实现 GORM models**

创建 `internal/storage/models.go`：

```go
package sqlite

type Meta struct {
	Key   string `gorm:"primaryKey"`
	Value string `gorm:"not null"`
}

type Workspace struct {
	ID        string `gorm:"primaryKey"`
	Slug      string `gorm:"not null;uniqueIndex"`
	Name      string `gorm:"not null"`
	CreatedAt int64  `gorm:"not null"`
}

type Task struct {
	UUID        string `gorm:"primaryKey"`
	WorkspaceID string `gorm:"not null;index"`
	Description string `gorm:"not null"`
	Status      string `gorm:"not null;index"`
	Entry       int64  `gorm:"not null"`
	Modified    int64  `gorm:"not null"`
	EndTS       *int64
	Due         *int64
	Project     *string `gorm:"index"`
	Priority    *string
	Tags        []TaskTag `gorm:"foreignKey:TaskUUID;constraint:OnDelete:CASCADE"`
}

type TaskTag struct {
	TaskUUID string `gorm:"primaryKey;not null"`
	Tag      string `gorm:"primaryKey;not null"`
}
```

- [x] **Step 4: 实现数据库打开、迁移、初始化**

创建 `internal/storage/db.go`：

```go
package sqlite

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const localWorkspaceSlug = "local"

type Store struct {
	db *gorm.DB
}

func Open(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("database path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	store := &Store{db: db}
	if err := store.configure(); err != nil {
		_ = store.Close()
		return nil, err
	}
	if err := store.migrate(); err != nil {
		_ = store.Close()
		return nil, err
	}
	if err := store.ensureLocalWorkspace(); err != nil {
		_ = store.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	sqlDB, err := s.sqlDB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (s *Store) DB() *gorm.DB {
	return s.db
}

func (s *Store) LocalWorkspace() (Workspace, error) {
	var ws Workspace
	err := s.db.Where("slug = ?", localWorkspaceSlug).First(&ws).Error
	return ws, err
}

func (s *Store) configure() error {
	return s.db.Exec("PRAGMA foreign_keys = ON").Error
}

func (s *Store) migrate() error {
	return s.db.AutoMigrate(&Meta{}, &Workspace{}, &Task{}, &TaskTag{})
}

func (s *Store) ensureLocalWorkspace() error {
	var count int64
	if err := s.db.Model(&Workspace{}).Where("slug = ?", localWorkspaceSlug).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return s.db.Create(&Workspace{
		ID:        uuid.NewString(),
		Slug:      localWorkspaceSlug,
		Name:      "Local",
		CreatedAt: time.Now().Unix(),
	}).Error
}

func (s *Store) sqlDB() (*sql.DB, error) {
	return s.db.DB()
}
```

- [x] **Step 5: 运行测试**

Run: `go test ./internal/storage -v`  
Expected: PASS。

- [x] **Step 6: 验证纯 Go / CGO-free**

Run: `CGO_ENABLED=0 go test ./internal/storage -v`  
Expected: PASS。  
如果失败并出现 `go-sqlite3` 或 CGO 相关错误，检查是否误引入了 `gorm.io/driver/sqlite`。

- [x] **Step 7: 提交**

```bash
git add go.mod go.sum internal/storage/models.go internal/storage/db.go internal/storage/db_test.go
git commit -m "feat: initialize gorm sqlite store"
```

## Chunk 2: Domain、Parser 与核心任务仓储

### 文件职责总览

- 创建：`internal/task/model.go`  
  任务 domain 类型、状态、校验、生命周期方法。
- 创建：`internal/task/modification.go`  
  modification 类型。
- 创建：`internal/query/filter.go`  
  M0 filter 类型。
- 创建：`internal/query/parser.go`  
  解析 `+tag`、`-tag`、`project:x`、`priority:H`、`status:completed`、`due:x`、`/text/`。
- 创建：`internal/query/parser_test.go`  
  filter 与 modification 解析测试。
- 创建：`internal/storage/task_repo.go`  
  GORM task CRUD、list、tag replace、ID resolution。
- 创建：`internal/storage/task_repo_test.go`  
  仓储集成测试。
- 修改：`internal/app/service.go`  
  Add/List/Modify/Done/Delete/Info use cases。

### Task 4: 定义 task domain 与校验

**Files:**

- Create: `internal/task/model.go`
- Create: `internal/task/model_test.go`

- [x] **Step 1: 写失败测试**

创建 `internal/task/model_test.go`：

```go
package task

import "testing"

func TestValidateRequiresDescription(t *testing.T) {
	tsk := Task{Description: "   ", Status: StatusPending}
	if err := tsk.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
}

func TestValidateRejectsInvalidPriority(t *testing.T) {
	priority := "X"
	tsk := Task{Description: "hello", Status: StatusPending, Priority: &priority}
	if err := tsk.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
}

func TestCompleteSetsStatusAndEnd(t *testing.T) {
	now := int64(100)
	tsk := Task{Description: "hello", Status: StatusPending}
	tsk.Complete(now)
	if tsk.Status != StatusCompleted {
		t.Fatalf("Status = %q", tsk.Status)
	}
	if tsk.End == nil || *tsk.End != now {
		t.Fatalf("End = %#v, want %d", tsk.End, now)
	}
	if tsk.Modified != now {
		t.Fatalf("Modified = %d, want %d", tsk.Modified, now)
	}
}
```

- [x] **Step 2: 运行测试，确认失败**

Run: `go test ./internal/task -v`  
Expected: FAIL。

- [x] **Step 3: 实现 domain model**

创建 `internal/task/model.go`：

```go
package task

import (
	"errors"
	"strings"
)

const (
	StatusPending   = "pending"
	StatusCompleted = "completed"
	StatusDeleted   = "deleted"
)

type Task struct {
	UUID        string
	WorkspaceID string
	Description string
	Status      string
	Entry       int64
	Modified    int64
	End         *int64
	Due         *int64
	Project     *string
	Priority    *string
	Tags        []string
}

func (t Task) Validate() error {
	if strings.TrimSpace(t.Description) == "" {
		return errors.New("description is required")
	}
	switch t.Status {
	case StatusPending, StatusCompleted, StatusDeleted:
	default:
		return errors.New("invalid status")
	}
	if t.Priority != nil {
		switch *t.Priority {
		case "H", "M", "L":
		default:
			return errors.New("invalid priority")
		}
	}
	return nil
}

func (t *Task) Complete(now int64) {
	t.Status = StatusCompleted
	t.End = &now
	t.Modified = now
}

func (t *Task) Delete(now int64) {
	t.Status = StatusDeleted
	t.End = &now
	t.Modified = now
}
```

- [x] **Step 4: 运行测试**

Run: `go test ./internal/task -v`  
Expected: PASS。

- [x] **Step 5: 提交**

```bash
git add internal/task/model.go internal/task/model_test.go
git commit -m "feat: add task domain model"
```

### Task 5: 实现 M0 参数解析

**Files:**

- Create: `internal/task/modification.go`
- Create: `internal/query/filter.go`
- Create: `internal/query/parser.go`
- Create: `internal/query/parser_test.go`

- [x] **Step 1: 写失败测试**

创建 `internal/query/parser_test.go`：

```go
package query

import "testing"

func TestParseAddArgsSeparatesDescriptionAndMods(t *testing.T) {
	parsed, err := ParseAddArgs([]string{"write", "spec", "project:xuanchu", "+planning", "priority:H"})
	if err != nil {
		t.Fatalf("ParseAddArgs() error = %v", err)
	}
	if parsed.Description != "write spec" {
		t.Fatalf("Description = %q", parsed.Description)
	}
	if parsed.Mod.Project == nil || *parsed.Mod.Project != "xuanchu" {
		t.Fatalf("Project = %#v", parsed.Mod.Project)
	}
	if parsed.Mod.Priority == nil || *parsed.Mod.Priority != "H" {
		t.Fatalf("Priority = %#v", parsed.Mod.Priority)
	}
	if len(parsed.Mod.AddTags) != 1 || parsed.Mod.AddTags[0] != "planning" {
		t.Fatalf("AddTags = %#v", parsed.Mod.AddTags)
	}
}

func TestParseFilters(t *testing.T) {
	filter, err := ParseFilters([]string{"+work", "project:xuanchu", "status:completed", "/spec/"})
	if err != nil {
		t.Fatalf("ParseFilters() error = %v", err)
	}
	if len(filter.Tags) != 1 || filter.Tags[0] != "work" {
		t.Fatalf("Tags = %#v", filter.Tags)
	}
	if filter.Project == nil || *filter.Project != "xuanchu" {
		t.Fatalf("Project = %#v", filter.Project)
	}
	if filter.Status == nil || *filter.Status != "completed" {
		t.Fatalf("Status = %#v", filter.Status)
	}
	if filter.Text == nil || *filter.Text != "spec" {
		t.Fatalf("Text = %#v", filter.Text)
	}
}

func TestParseModifyArgsRequiresModification(t *testing.T) {
	if _, err := ParseModifyArgs([]string{}); err == nil {
		t.Fatal("ParseModifyArgs() error = nil, want error")
	}
}
```

- [x] **Step 2: 运行测试，确认失败**

Run: `go test ./internal/query -v`  
Expected: FAIL。

- [x] **Step 3: 实现 modification 类型**

创建 `internal/task/modification.go`：

```go
package task

type Modification struct {
	Description *string
	Project     *string
	Priority    *string
	Due         *int64
	AddTags     []string
	RemoveTags  []string
}

func (m Modification) Empty() bool {
	return m.Description == nil &&
		m.Project == nil &&
		m.Priority == nil &&
		m.Due == nil &&
		len(m.AddTags) == 0 &&
		len(m.RemoveTags) == 0
}
```

- [x] **Step 4: 实现 filter 和 parser**

创建 `internal/query/filter.go`：

```go
package query

import "github.com/dajee/xuanchu/internal/task"

type Filter struct {
	Target  *string
	Status  *string
	Project *string
	Priority *string
	Tags    []string
	Text    *string
}

type ParsedAdd struct {
	Description string
	Mod         task.Modification
}
```

创建 `internal/query/parser.go`：

```go
package query

import (
	"errors"
	"strings"

	"github.com/dajee/xuanchu/internal/task"
)

func ParseAddArgs(args []string) (ParsedAdd, error) {
	var desc []string
	mod := task.Modification{}
	for _, arg := range args {
		if applyModificationToken(arg, &mod) {
			continue
		}
		desc = append(desc, arg)
	}
	description := strings.TrimSpace(strings.Join(desc, " "))
	if description == "" {
		return ParsedAdd{}, errors.New("description is required")
	}
	return ParsedAdd{Description: description, Mod: mod}, nil
}

func ParseModifyArgs(args []string) (task.Modification, error) {
	mod := task.Modification{}
	for _, arg := range args {
		if !applyModificationToken(arg, &mod) {
			return task.Modification{}, errors.New("unsupported modification: " + arg)
		}
	}
	if mod.Empty() {
		return task.Modification{}, errors.New("at least one modification is required")
	}
	return mod, nil
}

func ParseFilters(args []string) (Filter, error) {
	filter := Filter{}
	for _, arg := range args {
		switch {
		case strings.HasPrefix(arg, "+") && len(arg) > 1:
			filter.Tags = append(filter.Tags, strings.TrimPrefix(arg, "+"))
		case strings.HasPrefix(arg, "status:"):
			value := strings.TrimPrefix(arg, "status:")
			filter.Status = &value
		case strings.HasPrefix(arg, "project:"):
			value := strings.TrimPrefix(arg, "project:")
			filter.Project = &value
		case strings.HasPrefix(arg, "priority:"):
			value := strings.TrimPrefix(arg, "priority:")
			filter.Priority = &value
		case strings.HasPrefix(arg, "/") && strings.HasSuffix(arg, "/") && len(arg) >= 2:
			value := strings.TrimSuffix(strings.TrimPrefix(arg, "/"), "/")
			filter.Text = &value
		default:
			value := arg
			filter.Target = &value
		}
	}
	return filter, nil
}

func applyModificationToken(arg string, mod *task.Modification) bool {
	switch {
	case strings.HasPrefix(arg, "+") && len(arg) > 1:
		mod.AddTags = append(mod.AddTags, strings.TrimPrefix(arg, "+"))
		return true
	case strings.HasPrefix(arg, "-") && len(arg) > 1:
		mod.RemoveTags = append(mod.RemoveTags, strings.TrimPrefix(arg, "-"))
		return true
	case strings.HasPrefix(arg, "project:"):
		value := strings.TrimPrefix(arg, "project:")
		mod.Project = &value
		return true
	case strings.HasPrefix(arg, "priority:"):
		value := strings.TrimPrefix(arg, "priority:")
		mod.Priority = &value
		return true
	case strings.HasPrefix(arg, "description:"):
		value := strings.TrimPrefix(arg, "description:")
		mod.Description = &value
		return true
	default:
		return false
	}
}
```

日期解析会在 Task 8 添加，届时补上 `due:`。

- [x] **Step 5: 运行测试**

Run: `go test ./internal/query ./internal/task -v`  
Expected: PASS。

- [x] **Step 6: 提交**

```bash
git add internal/task/modification.go internal/query/filter.go internal/query/parser.go internal/query/parser_test.go
git commit -m "feat: parse m0 task arguments"
```

### Task 6: 实现 GORM 任务仓储

**Files:**

- Create: `internal/storage/task_repo.go`
- Create: `internal/storage/task_repo_test.go`
- Modify: `internal/storage/models.go`

- [x] **Step 1: 写失败测试**

创建 `internal/storage/task_repo_test.go`：

```go
package sqlite

import (
	"path/filepath"
	"testing"

	domain "github.com/dajee/xuanchu/internal/task"
)

func TestTaskRepositoryCreateAndList(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ws, _ := store.LocalWorkspace()
	repo := NewTaskRepository(store.DB())

	created, err := repo.Create(domain.Task{
		UUID:        "task-1",
		WorkspaceID: ws.ID,
		Description: "write spec",
		Status:      domain.StatusPending,
		Entry:       100,
		Modified:    100,
		Tags:        []string{"planning"},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.UUID != "task-1" {
		t.Fatalf("UUID = %q", created.UUID)
	}

	tasks, err := repo.List(ws.ID, ListOptions{Status: domain.StatusPending})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tasks) != 1 || tasks[0].Description != "write spec" {
		t.Fatalf("tasks = %#v", tasks)
	}
	if len(tasks[0].Tags) != 1 || tasks[0].Tags[0] != "planning" {
		t.Fatalf("tags = %#v", tasks[0].Tags)
	}
}

func TestTaskRepositoryUpdateReplacesTags(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ws, _ := store.LocalWorkspace()
	repo := NewTaskRepository(store.DB())

	_, err = repo.Create(domain.Task{
		UUID: "task-1", WorkspaceID: ws.ID, Description: "write spec",
		Status: domain.StatusPending, Entry: 100, Modified: 100,
		Tags: []string{"old"},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	tsk, err := repo.GetByUUID(ws.ID, "task-1")
	if err != nil {
		t.Fatalf("GetByUUID() error = %v", err)
	}
	tsk.Tags = []string{"new"}
	tsk.Modified = 200
	if err := repo.Update(tsk); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got, err := repo.GetByUUID(ws.ID, "task-1")
	if err != nil {
		t.Fatalf("GetByUUID() after update error = %v", err)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "new" {
		t.Fatalf("Tags = %#v", got.Tags)
	}
}
```

- [x] **Step 2: 运行测试，确认失败**

Run: `go test ./internal/storage -run TaskRepository -v`  
Expected: FAIL。

- [x] **Step 3: 实现 repository**

创建 `internal/storage/task_repo.go`：

```go
package sqlite

import (
	"errors"
	"sort"

	domain "github.com/dajee/xuanchu/internal/task"
	"gorm.io/gorm"
)

type TaskRepository struct {
	db *gorm.DB
}

type ListOptions struct {
	Status   string
	Project  *string
	Priority *string
	Tags     []string
	Text     *string
}

func NewTaskRepository(db *gorm.DB) *TaskRepository {
	return &TaskRepository{db: db}
}

func (r *TaskRepository) Create(tsk domain.Task) (domain.Task, error) {
	if err := tsk.Validate(); err != nil {
		return domain.Task{}, err
	}
	model := toModel(tsk)
	if err := r.db.Create(&model).Error; err != nil {
		return domain.Task{}, err
	}
	return fromModel(model), nil
}

func (r *TaskRepository) List(workspaceID string, opts ListOptions) ([]domain.Task, error) {
	var models []Task
	q := r.db.Preload("Tags").Where("workspace_id = ?", workspaceID)
	if opts.Status != "" {
		q = q.Where("status = ?", opts.Status)
	}
	if opts.Project != nil {
		q = q.Where("project = ?", *opts.Project)
	}
	if opts.Priority != nil {
		q = q.Where("priority = ?", *opts.Priority)
	}
	if opts.Text != nil {
		q = q.Where("description LIKE ?", "%"+*opts.Text+"%")
	}
	for _, tag := range opts.Tags {
		q = q.Where("uuid IN (SELECT task_uuid FROM task_tags WHERE tag = ?)", tag)
	}
	if err := q.Order("entry ASC").Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Task, 0, len(models))
	for _, model := range models {
		out = append(out, fromModel(model))
	}
	return out, nil
}

func (r *TaskRepository) GetByUUID(workspaceID, uuid string) (domain.Task, error) {
	var model Task
	err := r.db.Preload("Tags").Where("workspace_id = ? AND uuid = ?", workspaceID, uuid).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Task{}, ErrNotFound
	}
	if err != nil {
		return domain.Task{}, err
	}
	return fromModel(model), nil
}

func (r *TaskRepository) Update(tsk domain.Task) error {
	if err := tsk.Validate(); err != nil {
		return err
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		model := toModel(tsk)
		if err := tx.Model(&Task{}).Where("uuid = ? AND workspace_id = ?", tsk.UUID, tsk.WorkspaceID).Updates(map[string]any{
			"description": model.Description,
			"status":      model.Status,
			"modified":    model.Modified,
			"end_ts":      model.EndTS,
			"due":         model.Due,
			"project":     model.Project,
			"priority":    model.Priority,
		}).Error; err != nil {
			return err
		}
		if err := tx.Where("task_uuid = ?", tsk.UUID).Delete(&TaskTag{}).Error; err != nil {
			return err
		}
		for _, tag := range sortedUnique(tsk.Tags) {
			if err := tx.Create(&TaskTag{TaskUUID: tsk.UUID, Tag: tag}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

var ErrNotFound = errors.New("task not found")

func toModel(tsk domain.Task) Task {
	tags := make([]TaskTag, 0, len(tsk.Tags))
	for _, tag := range sortedUnique(tsk.Tags) {
		tags = append(tags, TaskTag{TaskUUID: tsk.UUID, Tag: tag})
	}
	return Task{
		UUID: tsk.UUID, WorkspaceID: tsk.WorkspaceID, Description: tsk.Description,
		Status: tsk.Status, Entry: tsk.Entry, Modified: tsk.Modified,
		EndTS: tsk.End, Due: tsk.Due, Project: tsk.Project, Priority: tsk.Priority,
		Tags: tags,
	}
}

func fromModel(model Task) domain.Task {
	tags := make([]string, 0, len(model.Tags))
	for _, tag := range model.Tags {
		tags = append(tags, tag.Tag)
	}
	sort.Strings(tags)
	return domain.Task{
		UUID: model.UUID, WorkspaceID: model.WorkspaceID, Description: model.Description,
		Status: model.Status, Entry: model.Entry, Modified: model.Modified,
		End: model.EndTS, Due: model.Due, Project: model.Project, Priority: model.Priority,
		Tags: tags,
	}
}

func sortedUnique(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
```

- [x] **Step 4: 运行测试**

Run: `go test ./internal/storage -run TaskRepository -v`  
Expected: PASS。

- [x] **Step 5: CGO-free 测试**

Run: `CGO_ENABLED=0 go test ./internal/storage -v`  
Expected: PASS。

- [x] **Step 6: 提交**

```bash
git add internal/storage/task_repo.go internal/storage/task_repo_test.go internal/storage/models.go
git commit -m "feat: persist tasks with gorm"
```

## Chunk 3: App Service 与 CLI 核心命令

### 文件职责总览

- 修改：`internal/app/service.go`  
  实现 Add/List/Modify/Done/Delete/Info。
- 创建：`internal/app/service_test.go`  
  use case 测试。
- 创建：`internal/app/clock.go`  
  可注入时钟。
- 创建：`internal/cli/add.go`  
  `xuanchu add`。
- 创建：`internal/cli/list.go`  
  `xuanchu list`、`xuanchu next`。
- 创建：`internal/cli/modify.go`  
  `xuanchu <target> modify`、`done`、`delete`。
- 创建：`internal/cli/info.go`  
  `xuanchu info <target>`。
- 创建：`internal/render/json.go`  
  JSON 输出。
- 创建：`internal/render/table.go`  
  简单表格输出。

### Task 7: 实现 app service 的 Add/List/Info

**Files:**

- Modify: `internal/app/service.go`
- Create: `internal/app/service_test.go`
- Create: `internal/app/clock.go`

- [x] **Step 1: 写失败测试**

创建 `internal/app/service_test.go`，使用 SQLite 临时库做 service 级测试：

```go
package app

import (
	"path/filepath"
	"testing"

	"github.com/dajee/xuanchu/internal/storage"
)

func TestServiceAddListInfo(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	svc, err := NewService(ServiceOptions{Store: store, Clock: fixedClock{NowUnix: 100}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	created, err := svc.Add(AddInput{Description: "write spec", Tags: []string{"planning"}})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if created.UUID == "" {
		t.Fatal("created UUID is empty")
	}

	tasks, err := svc.List(ListInput{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tasks) != 1 || tasks[0].Description != "write spec" {
		t.Fatalf("tasks = %#v", tasks)
	}

	got, err := svc.Info(created.UUID)
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if got.UUID != created.UUID {
		t.Fatalf("Info UUID = %q, want %q", got.UUID, created.UUID)
	}
}
```

- [x] **Step 2: 运行测试，确认失败**

Run: `go test ./internal/app -v`  
Expected: FAIL。

- [x] **Step 3: 实现 service**

在 `internal/app/clock.go`：

```go
package app

import "time"

type Clock interface {
	Unix() int64
}

type realClock struct{}

func (realClock) Unix() int64 { return time.Now().Unix() }

type fixedClock struct {
	NowUnix int64
}

func (f fixedClock) Unix() int64 { return f.NowUnix }
```

在 `internal/app/service.go`：

```go
package app

import (
	"github.com/google/uuid"

	"github.com/dajee/xuanchu/internal/storage"
	"github.com/dajee/xuanchu/internal/task"
)

type Service struct {
	store       *sqlite.Store
	repo        *sqlite.TaskRepository
	workspaceID string
	clock       Clock
}

type ServiceOptions struct {
	Store *sqlite.Store
	Clock Clock
}

type AddInput struct {
	Description string
	Project     *string
	Priority    *string
	Due         *int64
	Tags        []string
}

type ListInput struct {
	Status   string
	Project  *string
	Priority *string
	Tags     []string
	Text     *string
}

func NewService(opts ServiceOptions) (*Service, error) {
	if opts.Clock == nil {
		opts.Clock = realClock{}
	}
	ws, err := opts.Store.LocalWorkspace()
	if err != nil {
		return nil, err
	}
	return &Service{
		store: opts.Store,
		repo: sqlite.NewTaskRepository(opts.Store.DB()),
		workspaceID: ws.ID,
		clock: opts.Clock,
	}, nil
}

func (s *Service) Add(input AddInput) (task.Task, error) {
	now := s.clock.Unix()
	tsk := task.Task{
		UUID: uuid.NewString(), WorkspaceID: s.workspaceID, Description: input.Description,
		Status: task.StatusPending, Entry: now, Modified: now, Due: input.Due,
		Project: input.Project, Priority: input.Priority, Tags: input.Tags,
	}
	return s.repo.Create(tsk)
}

func (s *Service) List(input ListInput) ([]task.Task, error) {
	status := input.Status
	if status == "" {
		status = task.StatusPending
	}
	return s.repo.List(s.workspaceID, sqlite.ListOptions{
		Status: status, Project: input.Project, Priority: input.Priority,
		Tags: input.Tags, Text: input.Text,
	})
}

func (s *Service) Info(target string) (task.Task, error) {
	return s.repo.GetByUUID(s.workspaceID, target)
}
```

- [x] **Step 4: 运行测试**

Run: `go test ./internal/app ./internal/storage -v`  
Expected: PASS。

- [x] **Step 5: 提交**

```bash
git add internal/app/service.go internal/app/service_test.go internal/app/clock.go
git commit -m "feat: add task app service"
```

### Task 8: 添加日期解析和 due 支持

**Files:**

- Create: `internal/query/date.go`
- Create: `internal/query/date_test.go`
- Modify: `internal/query/parser.go`
- Modify: `internal/query/parser_test.go`

- [x] **Step 1: 写失败测试**

创建 `internal/query/date_test.go`：

```go
package query

import (
	"testing"
	"time"
)

func TestParseDateTomorrow(t *testing.T) {
	loc := time.FixedZone("TEST", 8*60*60)
	now := time.Date(2026, 5, 28, 10, 0, 0, 0, loc)
	got, err := ParseDate("tomorrow", now, loc)
	if err != nil {
		t.Fatalf("ParseDate() error = %v", err)
	}
	want := time.Date(2026, 5, 29, 0, 0, 0, 0, loc).Unix()
	if got != want {
		t.Fatalf("got %d, want %d", got, want)
	}
}

func TestParseDateYYYYMMDD(t *testing.T) {
	loc := time.UTC
	got, err := ParseDate("2026-05-28", time.Now(), loc)
	if err != nil {
		t.Fatalf("ParseDate() error = %v", err)
	}
	want := time.Date(2026, 5, 28, 0, 0, 0, 0, loc).Unix()
	if got != want {
		t.Fatalf("got %d, want %d", got, want)
	}
}
```

- [x] **Step 2: 运行测试，确认失败**

Run: `go test ./internal/query -run ParseDate -v`  
Expected: FAIL。

- [x] **Step 3: 实现日期解析**

创建 `internal/query/date.go`：

```go
package query

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func ParseDate(value string, now time.Time, loc *time.Location) (int64, error) {
	if loc == nil {
		loc = time.Local
	}
	if ts, err := time.Parse(time.RFC3339, value); err == nil {
		return ts.Unix(), nil
	}
	if ts, err := time.ParseInLocation("2006-01-02", value, loc); err == nil {
		return ts.Unix(), nil
	}
	start := time.Date(now.In(loc).Year(), now.In(loc).Month(), now.In(loc).Day(), 0, 0, 0, 0, loc)
	switch value {
	case "today":
		return start.Unix(), nil
	case "tomorrow":
		return start.AddDate(0, 0, 1).Unix(), nil
	case "eod":
		return start.Add(24*time.Hour - time.Second).Unix(), nil
	case "eow":
		days := (7 - int(start.Weekday())) % 7
		return start.AddDate(0, 0, days).Add(24*time.Hour - time.Second).Unix(), nil
	case "eom":
		return time.Date(start.Year(), start.Month()+1, 1, 0, 0, 0, 0, loc).Add(-time.Second).Unix(), nil
	}
	if strings.HasSuffix(value, "days") {
		n, err := strconv.Atoi(strings.TrimSuffix(value, "days"))
		if err != nil {
			return 0, err
		}
		return start.AddDate(0, 0, n).Unix(), nil
	}
	return 0, fmt.Errorf("unsupported date %q", value)
}
```

- [x] **Step 4: 修改 parser 支持 `due:`**

在 `applyModificationToken` 中识别 `due:`。为了让测试稳定，新增 `ParseAddArgsWithNow(args, now, loc)` 和 `ParseModifyArgsWithNow(args, now, loc)`，原函数调用当前时间版本。示例：

```go
case strings.HasPrefix(arg, "due:"):
	value := strings.TrimPrefix(arg, "due:")
	due, err := ParseDate(value, time.Now(), time.Local)
	if err != nil {
		return false
	}
	mod.Due = &due
	return true
```

实施时不要吞掉日期错误；应让 parser 返回 `unsupported date "..."`。如果当前 helper 返回 bool 不够表达错误，把它改为 `(bool, error)` 并更新测试。

- [x] **Step 5: 运行测试**

Run: `go test ./internal/query -v`  
Expected: PASS。

- [x] **Step 6: 提交**

```bash
git add internal/query/date.go internal/query/date_test.go internal/query/parser.go internal/query/parser_test.go
git commit -m "feat: parse task due dates"
```

### Task 9: 实现 CLI add/list/info

**Files:**

- Create: `internal/cli/add.go`
- Create: `internal/cli/list.go`
- Create: `internal/cli/info.go`
- Create: `internal/render/json.go`
- Create: `internal/render/table.go`
- Modify: `internal/cli/root.go`
- Create: `tests/integration/cli_test.go`

- [x] **Step 1: 写失败集成测试**

创建 `tests/integration/cli_test.go`：

```go
package integration

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIAddListInfo(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")

	run(t, bin, "--db", db, "add", "write", "spec", "+planning")
	out := run(t, bin, "--db", db, "list")
	if !strings.Contains(out, "write spec") {
		t.Fatalf("list output = %q", out)
	}
	if !strings.Contains(out, "planning") {
		t.Fatalf("list output missing tag = %q", out)
	}
	info := run(t, bin, "--db", db, "info", "1")
	if !strings.Contains(info, "UUID") || !strings.Contains(info, "write spec") {
		t.Fatalf("info output = %q", info)
	}
}

func buildXuanchu(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "xuanchu")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/xuanchu")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build error = %v\n%s", err, out)
	}
	return bin
}

func run(t *testing.T, bin string, args ...string) string {
	t.Helper()
	cmd := exec.Command(bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v error = %v\n%s", bin, args, err, out)
	}
	return string(out)
}
```

- [x] **Step 2: 运行测试，确认失败**

Run: `go test ./tests/integration -run TestCLIAddListInfo -v`  
Expected: FAIL，命令尚未注册。

- [x] **Step 3: 实现渲染辅助**

创建 `internal/render/json.go`：

```go
package render

import (
	"encoding/json"
	"io"
)

func JSON(w io.Writer, value any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}
```

创建 `internal/render/table.go`，保持 M0 简单，不引入 tablewriter：

```go
package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/dajee/xuanchu/internal/task"
)

func TaskList(w io.Writer, tasks []task.Task) {
	fmt.Fprintln(w, "ID  UUID      PRI  PROJECT  TAGS  DESCRIPTION")
	for i, tsk := range tasks {
		priority := ""
		if tsk.Priority != nil {
			priority = *tsk.Priority
		}
		project := ""
		if tsk.Project != nil {
			project = *tsk.Project
		}
		uuid := tsk.UUID
		if len(uuid) > 8 {
			uuid = uuid[:8]
		}
		fmt.Fprintf(w, "%-3d %-8s %-4s %-8s %-5s %s\n",
			i+1, uuid, priority, project, strings.Join(tsk.Tags, ","), tsk.Description)
	}
}

func TaskInfo(w io.Writer, tsk task.Task) {
	fmt.Fprintf(w, "UUID: %s\n", tsk.UUID)
	fmt.Fprintf(w, "Status: %s\n", tsk.Status)
	fmt.Fprintf(w, "Description: %s\n", tsk.Description)
	fmt.Fprintf(w, "Tags: %s\n", strings.Join(tsk.Tags, ","))
}
```

- [x] **Step 4: 注册 service factory**

在 `internal/cli/root.go` 中增加运行时初始化逻辑：根据 flags 调用 `config.Resolve`，打开 `sqlite.Open`，创建 `app.NewService`。建议封装：

```go
func buildService(opts Options) (*app.Service, func() error, error)
```

每个命令调用该 helper，defer close。

- [x] **Step 5: 实现 add/list/info 命令**

`add`：

```go
cmd := &cobra.Command{
	Use: "add [description] [modifications...]",
	RunE: func(cmd *cobra.Command, args []string) error {
		parsed, err := query.ParseAddArgs(args)
		if err != nil { return err }
		svc, closeFn, err := buildService(opts)
		if err != nil { return err }
		defer closeFn()
		created, err := svc.Add(app.AddInput{
			Description: parsed.Description,
			Project: parsed.Mod.Project,
			Priority: parsed.Mod.Priority,
			Due: parsed.Mod.Due,
			Tags: parsed.Mod.AddTags,
		})
		if err != nil { return err }
		if opts.JSON { return render.JSON(cmd.OutOrStdout(), created) }
		fmt.Fprintf(cmd.OutOrStdout(), "Created task %s\n", created.UUID)
		return nil
	},
}
```

`list` 调用 `svc.List` 并使用 `render.TaskList`。`info` 暂时支持 target 为 `"1"` 时通过 `svc.List` 取第一项；UUID 直接 `svc.Info(uuid)`。后续 Task 10 会把 target resolution 收进 service/repository。

- [x] **Step 6: 运行集成测试**

Run: `go test ./tests/integration -run TestCLIAddListInfo -v`  
Expected: PASS。

- [x] **Step 7: 整体测试**

Run: `go test ./...`  
Expected: PASS。

- [x] **Step 8: CGO-free 测试**

Run: `CGO_ENABLED=0 go test ./...`  
Expected: PASS。

- [x] **Step 9: 提交**

```bash
git add internal/cli/add.go internal/cli/list.go internal/cli/info.go internal/cli/root.go internal/render/json.go internal/render/table.go tests/integration/cli_test.go
git commit -m "feat: add core read cli commands"
```

### Task 10: 实现 modify/done/delete 与数字 ID 解析

**Files:**

- Modify: `internal/app/service.go`
- Modify: `internal/app/service_test.go`
- Modify: `internal/storage/task_repo.go`
- Modify: `internal/cli/modify.go`
- Modify: `tests/integration/cli_test.go`

- [x] **Step 1: 写失败测试**

在 `internal/app/service_test.go` 增加：

```go
func TestServiceModifyDoneDeleteByNumber(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	svc, _ := NewService(ServiceOptions{Store: store, Clock: fixedClock{NowUnix: 100}})

	_, err = svc.Add(AddInput{Description: "write spec"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	priority := "H"
	if err := svc.Modify("1", ModifyInput{Priority: &priority}); err != nil {
		t.Fatalf("Modify() error = %v", err)
	}
	got, err := svc.ResolveTarget("1")
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if got.Priority == nil || *got.Priority != "H" {
		t.Fatalf("Priority = %#v", got.Priority)
	}
	if err := svc.Done("1"); err != nil {
		t.Fatalf("Done() error = %v", err)
	}
	tasks, _ := svc.List(ListInput{})
	if len(tasks) != 0 {
		t.Fatalf("pending tasks = %#v, want empty", tasks)
	}
}
```

- [x] **Step 2: 运行测试，确认失败**

Run: `go test ./internal/app -run TestServiceModifyDoneDeleteByNumber -v`  
Expected: FAIL。

- [x] **Step 3: 实现 target resolution**

在 service 增加：

```go
func (s *Service) ResolveTarget(target string) (task.Task, error)
```

规则：

- 如果 target 可解析为正整数 N，则调用 `List(ListInput{})` 得到默认 pending working set，返回第 N 个。
- 否则按 UUID 查询。
- N 小于 1 或超出范围时返回 `sqlite.ErrNotFound` 或 app 层 `ErrNotFound`。

- [x] **Step 4: 实现 Modify/Done/Delete**

新增：

```go
type ModifyInput struct {
	Description *string
	Project     *string
	Priority    *string
	Due         *int64
	AddTags     []string
	RemoveTags  []string
}
```

`Modify` 逻辑：

- ResolveTarget。
- 应用 description/project/priority/due。
- AddTags 加入 set，RemoveTags 删除。
- 更新 Modified。
- repo.Update。

`Done` 调用 domain `Complete(now)`，`Delete` 调用 domain `Delete(now)`。

- [x] **Step 5: 实现 CLI modify/done/delete**

创建 `internal/cli/modify.go`：

- 支持 `xuanchu 1 modify priority:H +next`。
- 支持 `xuanchu 1 done`。
- 支持 `xuanchu 1 delete`。

实现方式：在 root command 上增加一个隐藏/通用分发逻辑会复杂；M0 可以注册 Cobra command：

```go
Use: "<target> <action>"
```

更稳妥做法是在 root `Args` 前置解析中识别第一位 target，动态 dispatch 到 `modify/done/delete`。实施时保持测试覆盖，不为优雅牺牲可用性。

- [x] **Step 6: 增加集成测试**

在 `tests/integration/cli_test.go` 增加：

```go
func TestCLIModifyDoneDelete(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	run(t, bin, "--db", db, "add", "write", "spec")
	run(t, bin, "--db", db, "1", "modify", "priority:H", "+next")
	out := run(t, bin, "--db", db, "list")
	if !strings.Contains(out, "H") || !strings.Contains(out, "next") {
		t.Fatalf("list output = %q", out)
	}
	run(t, bin, "--db", db, "1", "done")
	out = run(t, bin, "--db", db, "list")
	if strings.Contains(out, "write spec") {
		t.Fatalf("done task still in default list: %q", out)
	}
}
```

- [x] **Step 7: 运行测试**

Run:

```bash
go test ./internal/app -v
go test ./tests/integration -v
go test ./...
CGO_ENABLED=0 go test ./...
```

Expected: 全部 PASS。

- [x] **Step 8: 提交**

```bash
git add internal/app/service.go internal/app/service_test.go internal/cli/modify.go internal/storage/task_repo.go tests/integration/cli_test.go
git commit -m "feat: modify and complete tasks"
```

## Chunk 4: JSON 导入导出、config/show、README 与验收

### 文件职责总览

- 创建：`internal/cli/import_export.go`  
  `import` / `export`。
- 创建：`internal/task/json.go`  
  Taskwarrior 风格 JSON DTO。
- 创建：`internal/task/json_test.go`  
  JSON 字段和日期格式测试。
- 创建：`internal/cli/config.go`  
  `show`、`config get`、`config set`。
- 修改：`README.md`  
  M0 使用说明。

### Task 11: 实现 Taskwarrior 风格 JSON import/export

**Files:**

- Create: `internal/task/json.go`
- Create: `internal/task/json_test.go`
- Create: `internal/cli/import_export.go`
- Modify: `internal/app/service.go`
- Modify: `tests/integration/cli_test.go`

- [x] **Step 1: 写 JSON DTO 单元测试**

创建 `internal/task/json_test.go`：

```go
package task

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTaskJSONUsesTaskwarriorFieldNames(t *testing.T) {
	priority := "H"
	tsk := Task{
		UUID: "u1", Description: "write spec", Status: StatusPending,
		Entry: 100, Modified: 100, Priority: &priority, Tags: []string{"planning"},
	}
	dto := ToJSON(tsk)
	data, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	for _, field := range []string{`"uuid"`, `"description"`, `"status"`, `"entry"`, `"modified"`, `"priority"`, `"tags"`} {
		if !strings.Contains(string(data), field) {
			t.Fatalf("JSON %s missing field %s", data, field)
		}
	}
}
```

记得 import `strings`。

- [x] **Step 2: 运行测试，确认失败**

Run: `go test ./internal/task -run TaskJSON -v`  
Expected: FAIL。

- [x] **Step 3: 实现 JSON DTO**

创建 `internal/task/json.go`：

```go
package task

import "time"

type JSONTask struct {
	UUID        string   `json:"uuid"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Entry       string   `json:"entry"`
	Modified    string   `json:"modified"`
	End         *string  `json:"end,omitempty"`
	Due         *string  `json:"due,omitempty"`
	Project     *string  `json:"project,omitempty"`
	Priority    *string  `json:"priority,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

func ToJSON(tsk Task) JSONTask {
	return JSONTask{
		UUID: tsk.UUID, Description: tsk.Description, Status: tsk.Status,
		Entry: formatUnix(tsk.Entry), Modified: formatUnix(tsk.Modified),
		End: formatUnixPtr(tsk.End), Due: formatUnixPtr(tsk.Due),
		Project: tsk.Project, Priority: tsk.Priority, Tags: tsk.Tags,
	}
}

func formatUnix(sec int64) string {
	return time.Unix(sec, 0).UTC().Format(time.RFC3339)
}

func formatUnixPtr(sec *int64) *string {
	if sec == nil {
		return nil
	}
	value := formatUnix(*sec)
	return &value
}
```

后续可补 `FromJSON`，用于 import。

- [x] **Step 4: 实现 service Export/Import**

新增：

```go
func (s *Service) Export(input ListInput) ([]task.Task, error)
func (s *Service) Import(tasks []task.JSONTask) error
```

M0 import 行为：

- JSON array 来自 stdin 或文件。
- 如果 UUID 已存在则 update，否则 create。
- 如果 JSON 未提供 UUID，生成新 UUID。
- 日期解析 RFC3339。

- [x] **Step 5: 实现 CLI import/export**

`export`：

- 默认导出所有 status，不只 pending。
- human 和 JSON 模式都输出 JSON array，因为该命令本质是机器接口。

`import`：

- `xuanchu import path.json` 或 `cat path.json | xuanchu import`。
- 成功后输出 `Imported N tasks`。

- [x] **Step 6: 增加集成测试**

在 `tests/integration/cli_test.go` 增加：

```go
func TestCLIExportImportRoundTrip(t *testing.T) {
	bin := buildXuanchu(t)
	db1 := filepath.Join(t.TempDir(), "one.db")
	db2 := filepath.Join(t.TempDir(), "two.db")
	run(t, bin, "--db", db1, "add", "write", "spec", "+planning")
	exported := run(t, bin, "--db", db1, "export")
	path := filepath.Join(t.TempDir(), "tasks.json")
	if err := os.WriteFile(path, []byte(exported), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, bin, "--db", db2, "import", path)
	out := run(t, bin, "--db", db2, "list")
	if !strings.Contains(out, "write spec") {
		t.Fatalf("list output = %q", out)
	}
}
```

记得 import `os`。

- [x] **Step 7: 运行测试**

Run:

```bash
go test ./internal/task -v
go test ./tests/integration -run TestCLIExportImportRoundTrip -v
go test ./...
```

Expected: PASS。

- [x] **Step 8: 提交**

```bash
git add internal/task/json.go internal/task/json_test.go internal/cli/import_export.go internal/app/service.go tests/integration/cli_test.go
git commit -m "feat: import and export task json"
```

### Task 12: 实现 show/config

**Files:**

- Create: `internal/cli/config.go`
- Modify: `internal/storage/models.go`
- Modify: `internal/storage/db.go`
- Modify: `tests/integration/cli_test.go`

- [x] **Step 1: 写失败集成测试**

在 `tests/integration/cli_test.go` 增加：

```go
func TestCLIShowAndConfig(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	out := run(t, bin, "--db", db, "show")
	if !strings.Contains(out, "database.path") {
		t.Fatalf("show output = %q", out)
	}
	run(t, bin, "--db", db, "config", "set", "date.format", "rfc3339")
	out = run(t, bin, "--db", db, "config", "get", "date.format")
	if strings.TrimSpace(out) != "rfc3339" {
		t.Fatalf("config get output = %q", out)
	}
}
```

- [x] **Step 2: 运行测试，确认失败**

Run: `go test ./tests/integration -run TestCLIShowAndConfig -v`  
Expected: FAIL。

- [x] **Step 3: 实现 meta get/set**

在 `internal/storage/db.go` 增加：

```go
func (s *Store) GetMeta(key string) (string, bool, error)
func (s *Store) SetMeta(key, value string) error
```

使用 GORM `First` 和 `Save` 或 `Clauses(OnConflict...)`。

- [x] **Step 4: 实现 CLI**

`show` 输出：

```text
database.path=/abs/path/xuanchu.db
color=true
date.format=rfc3339
```

`config get <key>`：

- 先查 meta。
- 没有则返回当前解析配置中的默认值。

`config set <key> <value>`：

- 只允许 `color`、`date.format`。
- `database.path` 只读，提示使用 `--db` 或 `XUANCHU_DB`。

- [x] **Step 5: 运行测试**

Run:

```bash
go test ./tests/integration -run TestCLIShowAndConfig -v
go test ./...
CGO_ENABLED=0 go test ./...
```

Expected: PASS。

- [x] **Step 6: 提交**

```bash
git add internal/cli/config.go internal/storage/db.go internal/storage/models.go tests/integration/cli_test.go
git commit -m "feat: add basic config commands"
```

### Task 13: README 与最终验收

**Files:**

- Modify: `README.md`

- [x] **Step 1: 更新 README**

在 `README.md` 添加 M0 用法：

````markdown
## M0 本地 CLI 用法

```bash
go build -o xuanchu ./cmd/xuanchu
./xuanchu add "Write project spec" project:xuanchu +planning due:tomorrow
./xuanchu list
./xuanchu 1 modify priority:H
./xuanchu 1 done
./xuanchu export
```

默认数据库路径为 `~/.local/share/xuanchu/xuanchu.db`，可用 `--db` 或 `XUANCHU_DB` 覆盖。
````

- [x] **Step 2: 运行最终测试**

Run:

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test ./...
go build ./cmd/xuanchu
CGO_ENABLED=0 go build ./cmd/xuanchu
```

Expected: 全部 PASS，build 成功。

- [x] **Step 3: 检查依赖中没有 CGO SQLite driver**

Run:

```bash
go list -m all | grep -E 'gorm.io/driver/sqlite|mattn/go-sqlite3' || true
```

Expected: 无输出。  
如果出现 `gorm.io/driver/sqlite` 或 `github.com/mattn/go-sqlite3`，移除误用依赖并重新测试。

- [x] **Step 4: 手动冒烟测试**

Run:

```bash
tmp="$(mktemp -d)"
go run ./cmd/xuanchu --db "$tmp/xuanchu.db" add "write final smoke" +smoke priority:H
go run ./cmd/xuanchu --db "$tmp/xuanchu.db" list
go run ./cmd/xuanchu --db "$tmp/xuanchu.db" 1 done
go run ./cmd/xuanchu --db "$tmp/xuanchu.db" status:completed list
go run ./cmd/xuanchu --db "$tmp/xuanchu.db" export
```

Expected:

- add 输出 created 信息。
- list 能看到 `write final smoke`。
- done 后默认 list 不再显示该任务。
- `status:completed list` 能看到该任务。
- export 输出 JSON array。

- [x] **Step 5: 提交**

```bash
git add README.md
git commit -m "docs: document m0 local usage"
```

## 计划审阅说明

本计划根据 [docs/superpowers/specs/2026-05-28-xuanchu-m0-design.md](/Users/mac/code/projects/dajee/task/docs/superpowers/specs/2026-05-28-xuanchu-m0-design.md) 编写，并按用户要求将数据库实现改为 GORM。SQLite driver 选择 `github.com/glebarez/sqlite`，因为它是 GORM 可用的纯 Go SQLite driver，不需要 CGO；不要使用 `gorm.io/driver/sqlite`。

当前目录不是 git 仓库时，计划中的 commit 步骤应记录为跳过，不应阻塞实施。当前工具策略没有用户明确授权 subagent delegation，因此未执行 plan-document-reviewer subagent 审阅；实施前如需要严格执行 superpowers 审阅环节，请先授权使用 subagent。
