# Xuanchu M2 Implementation Plan

> **For agentic workers:** REQUIRED: Use `superpowers:subagent-driven-development` (if subagents available) or `superpowers:executing-plans` to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现 xuanchu M2：补齐 Taskwarrior 核心任务模型、状态报表、注释/依赖、描述编辑和基础循环任务，并贯通查询、urgency、DOM、JSON import/export。

**Architecture:** M2 在 M1 的 query/report/urgency/dom/app/storage 分层上增量扩展，不重写已有查询 AST 与报表系统。存储继续使用 GORM + `github.com/glebarez/sqlite` + AutoMigrate，保持零 CGO；复杂派生语义（waiting 到期刷新、blocked/blocking、recurrence 生成）在 app/service 或独立 domain 包中完成，再通过 repository 事务落库。实现顺序按 Core schema/domain → query/report → CLI commands → editing → recurrence → docs/验收推进，每个 chunk 独立测试和提交。

**Tech Stack:** Go 1.22、Cobra、GORM、`github.com/glebarez/sqlite`、标准库 `time`/`sort`/`os/exec`/`encoding/json`、Go test。M2 不新增 SQLite driver，不引入 CGO。

**Delivery:** M2 可按 Chunk 拆成 6 个 PR/合并点：Chunk 1+2、Chunk 3、Chunk 4、Chunk 5、Chunk 6、Chunk 7。每个合并点都必须满足 `go test ./...`、`CGO_ENABLED=0 go test ./...`、`CGO_ENABLED=0 go build ./cmd/xuanchu`。

---

## Chunk 1: Core 数据模型与存储

### 文件职责

- Modify: `internal/task/model.go`
  - 扩展 Task 字段、status、Annotation 类型、domain validation。
- Modify: `internal/task/modification.go`
  - 扩展 Modification，支持 wait/scheduled/until/start/depends/annotations/recurrence 字段。
- Modify: `internal/storage/models.go`
  - 扩展 GORM Task model，新增 TaskAnnotation、TaskDependency。
- Modify: `internal/storage/db.go`
  - AutoMigrate 新表和新列。
- Modify: `internal/storage/task_repo.go`
  - create/update/fromModel/toModel 支持 annotations、depends、M2 nullable columns。
- Test: `internal/task/model_test.go`
- Test: `internal/storage/task_repo_test.go`

### Task 1: 扩展 domain Task 模型

**Files:**

- Modify: `internal/task/model.go`
- Modify: `internal/task/model_test.go`

- [x] **Step 1: 写失败测试**

在 `internal/task/model_test.go` 增加：

```go
func TestValidateAllowsM2StatusesAndFields(t *testing.T) {
	tsk := Task{UUID: "u1", WorkspaceID: "w1", Description: "task", Status: StatusWaiting, Entry: 1, Modified: 1}
	if err := tsk.Validate(); err != nil {
		t.Fatalf("Validate(waiting) error = %v", err)
	}
	tsk.Status = StatusRecurring
	if err := tsk.Validate(); err != nil {
		t.Fatalf("Validate(recurring) error = %v", err)
	}
}

func TestAnnotationValidateRejectsEmptyDescription(t *testing.T) {
	tsk := Task{
		UUID: "u1", WorkspaceID: "w1", Description: "task", Status: StatusPending, Entry: 1, Modified: 1,
		Annotations: []Annotation{{Entry: 10, Description: "  "}},
	}
	if err := tsk.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want annotation error")
	}
}

func TestStartStopHelpers(t *testing.T) {
	tsk := Task{UUID: "u1", WorkspaceID: "w1", Description: "task", Status: StatusPending, Entry: 1, Modified: 1}
	tsk.StartTask(100)
	if tsk.Start == nil || *tsk.Start != 100 || tsk.Modified != 100 {
		t.Fatalf("StartTask did not set start/modified: %#v", tsk)
	}
	tsk.StopTask(200)
	if tsk.Start != nil || tsk.Modified != 200 {
		t.Fatalf("StopTask did not clear start/update modified: %#v", tsk)
	}
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./internal/task -run 'M2|Annotation|StartStop' -v`  
Expected: FAIL，`StatusWaiting`、`Annotation`、`StartTask` 等尚不存在。

- [x] **Step 3: 扩展 `internal/task/model.go`**

修改 `internal/task/model.go`：

```go
const (
	StatusPending   = "pending"
	StatusCompleted = "completed"
	StatusDeleted   = "deleted"
	StatusWaiting   = "waiting"
	StatusRecurring = "recurring"
)

type Annotation struct {
	Entry       int64
	Description string
}

type Task struct {
	UUID        string
	WorkspaceID string
	Description string
	Status      string
	Entry       int64
	Modified    int64
	Start       *int64
	End         *int64
	Due         *int64
	Wait        *int64
	Scheduled   *int64
	Until       *int64
	Project     *string
	Priority    *string
	Tags        []string
	Annotations []Annotation
	Depends     []string
	Recur       *string
	Parent      *string
	Mask        *string
	IMask       *int
}
```

更新 `Validate()`：

- status 允许 pending/completed/deleted/waiting/recurring。
- annotation description trim 后不能为空，不能包含 `\n` 或 `\r`。
- dependency UUID 字符串不能为空。
- Task 1 先只承载 `Recur` 字段，不校验具体周期表达式，避免写临时 parser；Task 16 引入 `internal/recurrence.Validate` 后统一接入。
- `Status == recurring` 的完整约束在 Task 16 接入：必须 `Recur != nil` 且 `Due != nil`。M2 明确要求 `recur` 必须配 `due`，这样 child due 生成有稳定锚点。

新增 helper：

```go
func (t *Task) StartTask(now int64) {
	t.Status = StatusPending
	t.Start = &now
	t.Wait = nil
	t.Modified = now
}

func (t *Task) StopTask(now int64) {
	t.Start = nil
	t.Modified = now
}
```

`Complete` / `Delete` 同时清空 `Start`：

```go
func (t *Task) Complete(now int64) {
	t.Status = StatusCompleted
	t.End = &now
	t.Start = nil
	t.Modified = now
}
```

- [x] **Step 4: 运行测试**

Run: `go test ./internal/task -v`  
Expected: PASS。

- [x] **Step 5: 提交**

```bash
git add internal/task/model.go internal/task/model_test.go
git commit -m "feat: 扩展 M2 任务模型"
```

### Task 2: 扩展 SQLite schema 和 repository roundtrip

**Files:**

- Modify: `internal/storage/models.go`
- Modify: `internal/storage/db.go`
- Modify: `internal/storage/task_repo.go`
- Modify: `internal/storage/task_repo_test.go`

- [x] **Step 1: 写失败测试**

在 `internal/storage/task_repo_test.go` 增加：

```go
func TestTaskRepositoryPersistsM2Fields(t *testing.T) {
	store, repo, ws := newTestRepo(t)
	defer store.Close()

	start, wait, scheduled, until := int64(10), int64(20), int64(30), int64(40)
	recur, parent, mask := "weekly", "parent-uuid", "mask"
	imask := 1
	tsk := task.Task{
		UUID: "u1", WorkspaceID: ws.ID, Description: "m2 task", Status: task.StatusPending,
		Entry: 1, Modified: 2, Start: &start, Wait: &wait, Scheduled: &scheduled, Until: &until,
		Annotations: []task.Annotation{{Entry: 3, Description: "note"}},
		Depends: []string{"dep-1", "dep-2"},
		Recur: &recur, Parent: &parent, Mask: &mask, IMask: &imask,
	}
	if _, err := repo.Create(tsk); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	got, err := repo.GetByUUID(ws.ID, "u1")
	if err != nil {
		t.Fatalf("GetByUUID() error = %v", err)
	}
	if got.Start == nil || *got.Start != start || got.Wait == nil || *got.Wait != wait || got.Scheduled == nil || *got.Scheduled != scheduled || got.Until == nil || *got.Until != until {
		t.Fatalf("M2 date fields not roundtripped: %#v", got)
	}
	if len(got.Annotations) != 1 || got.Annotations[0].Description != "note" {
		t.Fatalf("Annotations = %#v", got.Annotations)
	}
	if !slices.Equal(got.Depends, []string{"dep-1", "dep-2"}) {
		t.Fatalf("Depends = %#v", got.Depends)
	}
	if got.Recur == nil || *got.Recur != recur || got.Parent == nil || *got.Parent != parent || got.Mask == nil || *got.Mask != mask || got.IMask == nil || *got.IMask != imask {
		t.Fatalf("recurrence fields not roundtripped: %#v", got)
	}
}
```

如果测试文件尚无 `newTestRepo(t)` helper，参考 `internal/storage/query_scope_test.go` 中的 `newQueryTestStore(t)` 新增一个本文件 helper，返回 `(*Store, *TaskRepository, Workspace)`；如果测试文件尚无 `slices` import，添加标准库 `slices`。

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./internal/storage -run TestTaskRepositoryPersistsM2Fields -v`  
Expected: FAIL，storage model 尚不支持 M2 字段。

- [x] **Step 3: 扩展 GORM models**

修改 `internal/storage/models.go`：

```go
type Task struct {
	UUID        string `gorm:"primaryKey"`
	WorkspaceID string `gorm:"not null;index"`
	Description string `gorm:"not null"`
	Status      string `gorm:"not null;index"`
	Entry       int64  `gorm:"not null"`
	Modified    int64  `gorm:"not null"`
	Start       *int64 `gorm:"index"`
	EndTS       *int64
	Due         *int64
	Wait        *int64 `gorm:"index"`
	Scheduled   *int64 `gorm:"index"`
	Until       *int64 `gorm:"index"`
	Project     *string `gorm:"index"`
	Priority    *string
	Recur       *string `gorm:"index"`
	Parent      *string `gorm:"index"`
	Mask        *string
	IMask       *int
	Tags        []TaskTag        `gorm:"foreignKey:TaskUUID;constraint:OnDelete:CASCADE"`
	Annotations []TaskAnnotation `gorm:"foreignKey:TaskUUID;constraint:OnDelete:CASCADE"`
	Depends     []TaskDependency `gorm:"foreignKey:TaskUUID;constraint:OnDelete:CASCADE"`
}

type TaskAnnotation struct {
	TaskUUID    string `gorm:"primaryKey;not null"`
	Entry       int64  `gorm:"primaryKey;not null"`
	Description string `gorm:"primaryKey;not null"`
}

type TaskDependency struct {
	TaskUUID  string `gorm:"primaryKey;not null"`
	DependsOn string `gorm:"primaryKey;not null;index"`
}
```

- [x] **Step 4: 更新 AutoMigrate**

修改 `internal/storage/db.go`：

```go
func (s *Store) migrate() error {
	return s.db.AutoMigrate(&Meta{}, &Workspace{}, &Task{}, &TaskTag{}, &TaskAnnotation{}, &TaskDependency{})
}
```

- [x] **Step 5: 更新 repository 映射和事务**

修改 `internal/storage/task_repo.go`：

- `Preload("Tags")` 改为同时 preload annotations/depends，建议抽 helper：

```go
func (r *TaskRepository) withTaskPreloads() *gorm.DB {
	return r.db.Preload("Tags").Preload("Annotations").Preload("Depends")
}
```

- `toModel` 填充 Start/Wait/Scheduled/Until/Recur/Parent/Mask/IMask/Annotations/Depends。
- `fromModel` 排序 tags、depends、annotations 后返回 domain task。
- `Update` map 增加 M2 columns。
- `Update` transaction 删除并重建 `task_annotations` 和 `task_dependencies`，和 tags 一致。
- `Create` 继续使用 `db.Create(&model)`，让 associations 一起插入。

注意：`TaskDependency.DependsOn` 字段名与数据库列名会是 `depends_on`，测试中不要硬编码列名。

- [x] **Step 6: 运行测试**

Run:

```bash
go test ./internal/storage -run TestTaskRepositoryPersistsM2Fields -v
go test ./internal/storage ./internal/task -v
```

Expected: PASS。

- [x] **Step 7: 提交**

```bash
git add internal/storage/models.go internal/storage/db.go internal/storage/task_repo.go internal/storage/task_repo_test.go
git commit -m "feat: 持久化 M2 任务字段"
```

## Chunk 2: 修改解析、JSON 与基础字段命令

### 文件职责

- Modify: `internal/task/modification.go`
  - Modification 增加 wait/scheduled/until/depends/recur 等字段。
- Modify: `internal/query/parser.go`
  - ParseAddArgs/ParseModifyArgs 支持 M2 modification tokens。
- Modify: `internal/app/service.go`
  - AddInput/ModifyInput 支持 M2 字段，新增 Start/Stop/Annotate/Denotate/Append/Prepend 方法。
- Modify: `internal/task/json.go`
  - JSONTask 覆盖 M2 字段。
- Modify: `internal/task/json_test.go`
  - M2 JSON roundtrip。
- Test: `internal/query/parser_test.go`
- Test: `internal/app/service_test.go`

### Task 3: 扩展 modification parser

**Files:**

- Modify: `internal/task/modification.go`
- Modify: `internal/query/parser.go`
- Modify: `internal/query/parser_test.go`

- [x] **Step 1: 写失败测试**

在 `internal/query/parser_test.go` 增加：

```go
func TestParseModifyArgsM2Fields(t *testing.T) {
	mod, err := ParseModifyArgs([]string{"wait:tomorrow", "scheduled:eow", "until:2030-01-01", "depends:abc", "depends:", "recur:weekly"})
	if err != nil {
		t.Fatalf("ParseModifyArgs() error = %v", err)
	}
	if mod.Wait == nil || mod.Scheduled == nil || mod.Until == nil {
		t.Fatalf("date fields not parsed: %#v", mod)
	}
	if !mod.ClearDepends || len(mod.AddDepends) != 1 || mod.AddDepends[0] != "abc" {
		t.Fatalf("depends not parsed: %#v", mod)
	}
	if mod.Recur == nil || *mod.Recur != "weekly" {
		t.Fatalf("Recur = %#v", mod.Recur)
	}
}

func TestParseModifyArgsClearsM2DateFields(t *testing.T) {
	mod, err := ParseModifyArgs([]string{"wait:", "scheduled:", "until:"})
	if err != nil {
		t.Fatalf("ParseModifyArgs() error = %v", err)
	}
	if !mod.ClearWait || !mod.ClearScheduled || !mod.ClearUntil {
		t.Fatalf("clear flags not set: %#v", mod)
	}
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./internal/query -run 'M2Fields|ClearsM2' -v`  
Expected: FAIL。

- [x] **Step 3: 扩展 `task.Modification`**

修改 `internal/task/modification.go`：

```go
type Modification struct {
	Description *string
	Project     *string
	Priority    *string
	Due         *int64
	ClearDue    bool
	Wait        *int64
	ClearWait   bool
	Scheduled   *int64
	ClearScheduled bool
	Until       *int64
	ClearUntil  bool
	AddDepends  []string
	ClearDepends bool
	Recur       *string
	ClearRecur  bool
	AddTags     []string
	RemoveTags  []string
}
```

更新 `Empty()` 覆盖所有字段。

- [x] **Step 4: 扩展 parser**

修改 `internal/query/parser.go`：

- `due:` 空值设置 `ClearDue = true`。
- `wait:<date>` / `scheduled:<date>` / `until:<date>` 使用 `ResolveDeadlineDateValue(ParseDateValue(value), time.Now().Unix(), time.Local)`。
- `wait:` / `scheduled:` / `until:` 设置 clear flag。
- `depends:<value>`：
  - 空值设置 `ClearDepends = true`。
  - 非空 append 到 `AddDepends`。解析为 target 留给 app 层，因为需要 working-set。
- `recur:<value>`：
  - 空值设置 `ClearRecur = true`。
  - 非空保存字符串，由 domain validation 校验。

注意：M2 parser 暂继续使用 `time.Now()` 解析 modification date，这是 M1 已有行为；如果实施时要改为 service clock，需要把 ParseModifyArgs 签名整体调整，计划需同步。

- [x] **Step 6: 运行测试**

Run: `go test ./internal/query ./internal/task -v`  
Expected: PASS。

- [x] **Step 7: 提交**

```bash
git add internal/task/modification.go internal/query/parser.go internal/query/parser_test.go
git commit -m "feat: 解析 M2 修改字段"
```

### Task 4: Dependency cycle 检测

**Files:**

- Create: `internal/task/dependency.go`
- Create: `internal/task/dependency_test.go`

- [x] **Step 1: 写失败测试**

创建 `internal/task/dependency_test.go`：

```go
package task

import "testing"

func TestWouldCreateDependencyCycle(t *testing.T) {
	graph := map[string][]string{
		"a": []string{"b"},
		"b": []string{"c"},
	}
	if !WouldCreateDependencyCycle(graph, "c", "a") {
		t.Fatal("expected c -> a to create cycle")
	}
	if WouldCreateDependencyCycle(graph, "c", "d") {
		t.Fatal("c -> d should not create cycle")
	}
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./internal/task -run TestWouldCreateDependencyCycle -v`  
Expected: FAIL。

- [x] **Step 3: 实现 cycle helper**

创建 `internal/task/dependency.go`：

```go
package task

func WouldCreateDependencyCycle(graph map[string][]string, taskUUID, dependsOn string) bool {
	if taskUUID == dependsOn {
		return true
	}
	seen := map[string]bool{}
	var visit func(string) bool
	visit = func(current string) bool {
		if current == taskUUID {
			return true
		}
		if seen[current] {
			return false
		}
		seen[current] = true
		for _, next := range graph[current] {
			if visit(next) {
				return true
			}
		}
		return false
	}
	return visit(dependsOn)
}
```

- [x] **Step 4: 运行测试**

Run: `go test ./internal/task -run Dependency -v`  
Expected: PASS。

- [x] **Step 5: 提交**

```bash
git add internal/task/dependency.go internal/task/dependency_test.go
git commit -m "feat: 添加依赖环检测"
```

### Task 5: App Add/Modify 支持 M2 字段与基础服务方法

**Files:**

- Modify: `internal/app/service.go`
- Modify: `internal/app/service_test.go`

- [x] **Step 1: 写失败测试**

在 `internal/app/service_test.go` 增加：

```go
func TestServiceM2Mutations(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	dep, _ := svc.Add(AddInput{Description: "dep"})
	tsk, _ := svc.Add(AddInput{Description: "task"})

	if err := svc.Modify(tsk.UUID, ModifyInput{AddDepends: []string{dep.UUID}}); err != nil {
		t.Fatalf("Modify(depends) error = %v", err)
	}
	if err := svc.Start(tsk.UUID); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := svc.Annotate(tsk.UUID, "note"); err != nil {
		t.Fatalf("Annotate() error = %v", err)
	}
	if err := svc.AppendDescription(tsk.UUID, "suffix"); err != nil {
		t.Fatalf("AppendDescription() error = %v", err)
	}
	if err := svc.PrependDescription(tsk.UUID, "prefix"); err != nil {
		t.Fatalf("PrependDescription() error = %v", err)
	}
	got, err := svc.ResolveTarget(tsk.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Start == nil || len(got.Depends) != 1 || len(got.Annotations) != 1 || got.Description != "prefix task suffix" {
		t.Fatalf("M2 fields not updated: %#v", got)
	}
	if err := svc.Stop(tsk.UUID); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	got, _ = svc.ResolveTarget(tsk.UUID)
	if got.Start != nil {
		t.Fatalf("Start after Stop = %#v", got.Start)
	}
}

func TestServiceRejectsDependencyCycle(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	a, _ := svc.Add(AddInput{Description: "a"})
	b, _ := svc.Add(AddInput{Description: "b"})
	if err := svc.Modify(a.UUID, ModifyInput{AddDepends: []string{b.UUID}}); err != nil {
		t.Fatalf("Modify(a depends b) error = %v", err)
	}
	if err := svc.Modify(b.UUID, ModifyInput{AddDepends: []string{a.UUID}}); err == nil {
		t.Fatal("expected dependency cycle error")
	}
}
```

如果现有 tests 没有 `newTestService(t, now)` helper，按当前 service tests 增加一个本地 helper，使用 temp sqlite store 和 fixed clock。

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./internal/app -run 'TestServiceM2Mutations|TestServiceRejectsDependencyCycle' -v`  
Expected: FAIL。

- [x] **Step 3: 扩展 input structs**

修改 `internal/app/service.go`：

```go
type AddInput struct {
	Description string
	Project     *string
	Priority    *string
	Due         *int64
	Wait        *int64
	Scheduled   *int64
	Until       *int64
	Recur       *string
	Tags        []string
}

type ModifyInput struct {
	Description *string
	Project     *string
	ClearProject bool
	Priority    *string
	ClearPriority bool
	Due         *int64
	ClearDue    bool
	Wait        *int64
	ClearWait   bool
	Scheduled   *int64
	ClearScheduled bool
	Until       *int64
	ClearUntil  bool
	AddDepends  []string
	ClearDepends bool
	Recur       *string
	ClearRecur  bool
	AddTags     []string
	RemoveTags  []string
	ClearTags    bool
}
```

- [x] **Step 4: 更新 Add/Modify**

`Add`：

- 设置 Wait/Scheduled/Until/Recur。
- 如果 `Wait != nil && *Wait > now`，status 设置为 `task.StatusWaiting`。
- 如果 `Recur != nil`，本任务后续会由 recurrence chunk 改为 parent；Task 5 先只保存字段，不生成 child。

`Modify`：

- clear 和 set project/priority/due/wait/scheduled/until/recur。
- `ClearTags` 清空 tags；`AddTags` / `RemoveTags` 延续 M1 行为。
- wait 未来时 status 置 waiting；clear wait 时如果 status waiting 则 pending。
- depends：
  - `ClearDepends` 清空。
  - `AddDepends` 中每个 target 调用 `ResolveTarget` 得到 UUID。
  - 禁止 self-dependency。
  - 加载当前 workspace 所有任务，构建 `map[uuid][]depends`，调用 Task 4 的 `task.WouldCreateDependencyCycle`；cycle 返回错误 `invalid dependency: cycle detected`。

- [x] **Step 5: 新增服务方法**

在 `internal/app/service.go` 添加：

```go
func (s *Service) Start(target string) error
func (s *Service) Stop(target string) error
func (s *Service) Annotate(target, description string) error
func (s *Service) Denotate(target string, index int) error
func (s *Service) AppendDescription(target, suffix string) error
func (s *Service) PrependDescription(target, prefix string) error
```

规则：

- Start 拒绝 completed/deleted/recurring。
- Start 对已 active 返回清晰错误 `task is already active`。
- Stop 对未 active 幂等成功。
- Annotate trim 后不能为空，不允许换行。
- Denotate 使用 1-based index，按 Entry 升序。
- Append/Prepend trim 后不能为空，并用单空格连接。

- [x] **Step 6: 运行测试**

Run:

```bash
go test ./internal/app -run 'TestServiceM2Mutations|TestServiceRejectsDependencyCycle' -v
```

Expected: PASS。

- [x] **Step 7: 提交**

```bash
git add internal/app/service.go internal/app/service_test.go
git commit -m "feat: 添加 M2 服务方法"
```

### Task 6: JSON import/export 覆盖 M2 字段

**Files:**

- Modify: `internal/task/json.go`
- Modify: `internal/task/json_test.go`
- Modify: `internal/app/service.go`

- [x] **Step 1: 写失败测试**

在 `internal/task/json_test.go` 增加：

```go
func TestJSONTaskM2RoundTrip(t *testing.T) {
	start, wait, scheduled, until := int64(10), int64(20), int64(30), int64(40)
	recur, parent, mask := "weekly", "parent", "mask"
	imask := 2
	tsk := Task{
		UUID: "u1", Description: "task", Status: StatusPending, Entry: 1, Modified: 2,
		Start: &start, Wait: &wait, Scheduled: &scheduled, Until: &until,
		Annotations: []Annotation{{Entry: 3, Description: "note"}},
		Depends: []string{"dep"},
		Recur: &recur, Parent: &parent, Mask: &mask, IMask: &imask,
	}
	got := FromJSON(ToJSON(tsk))
	if got.Start == nil || got.Wait == nil || got.Scheduled == nil || got.Until == nil {
		t.Fatalf("date fields lost: %#v", got)
	}
	if len(got.Annotations) != 1 || got.Annotations[0].Description != "note" || !slices.Equal(got.Depends, []string{"dep"}) {
		t.Fatalf("compound fields lost: %#v", got)
	}
	if got.Recur == nil || got.Parent == nil || got.Mask == nil || got.IMask == nil {
		t.Fatalf("recurrence fields lost: %#v", got)
	}
}

func TestJSONImportDistinguishesMissingAndEmptySlices(t *testing.T) {
	existing := Task{
		UUID: "u1", Description: "task", Status: StatusPending, Entry: 1, Modified: 2,
		Annotations: []Annotation{{Entry: 3, Description: "keep"}},
		Depends: []string{"dep"},
	}
	missing := FromJSON(JSONTask{UUID: "u1", Description: "task", Status: "pending"})
	if missing.Annotations != nil || missing.Depends != nil {
		t.Fatalf("missing slice fields should stay nil for merge: %#v", missing)
	}
	empty := FromJSON(JSONTask{UUID: "u1", Description: "task", Status: "pending", Annotations: []JSONAnnotation{}, Depends: []string{}})
	if empty.Annotations == nil || empty.Depends == nil {
		t.Fatalf("empty slice fields should remain non-nil for clearing: %#v", empty)
	}
	_ = existing
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./internal/task -run TestJSONTaskM2RoundTrip -v`  
Expected: FAIL。

- [x] **Step 3: 扩展 JSON DTO**

修改 `internal/task/json.go`：

```go
type JSONAnnotation struct {
	Entry       string `json:"entry"`
	Description string `json:"description"`
}

type JSONTask struct {
	UUID        string `json:"uuid"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Entry       string `json:"entry"`
	Modified    string `json:"modified"`
	Start       *string `json:"start,omitempty"`
	End         *string `json:"end,omitempty"`
	Due         *string `json:"due,omitempty"`
	Wait        *string `json:"wait,omitempty"`
	Scheduled   *string `json:"scheduled,omitempty"`
	Until       *string `json:"until,omitempty"`
	Project     *string `json:"project,omitempty"`
	Priority    *string `json:"priority,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Annotations []JSONAnnotation `json:"annotations,omitempty"`
	Depends     []string `json:"depends,omitempty"`
	Recur       *string `json:"recur,omitempty"`
	Parent      *string `json:"parent,omitempty"`
	Mask        *string `json:"mask,omitempty"`
	IMask       *int    `json:"imask,omitempty"`
}
```

更新 `ToJSON` / `FromJSON`，保持 M0/M1 字段兼容。

- [x] **Step 4: 更新 Import merge**

修改 `internal/app/service.go` 的 `Import` 更新路径：

- existing.Start/Wait/Scheduled/Until/Recur/Parent/Mask/IMask 应从 dto 覆盖。
- annotations/depends 即使为空也要能覆盖。为避免无法区分“字段缺失”和“空数组”，M2 可采用简单策略：`dto.Annotations != nil` 才覆盖，`dto.Depends != nil` 才覆盖。若需要该能力，JSONTask slice 字段保持 nil 和 empty 的区别。
- 单测必须覆盖：缺省 `annotations`/`depends` 不覆盖 existing；显式 `"annotations":[]` / `"depends":[]` 清空 existing。

- [x] **Step 5: 运行测试**

Run:

```bash
go test ./internal/task -run JSON -v
go test ./internal/app -run Import -v
```

Expected: PASS。

- [x] **Step 6: 提交**

```bash
git add internal/task/json.go internal/task/json_test.go internal/app/service.go
git commit -m "feat: 扩展 M2 JSON 字段"
```

## Chunk 3: Query、Report、Urgency、DOM 扩展

### 文件职责

- Modify: `internal/query/ast.go`
  - 新增属性和 OpNotNull。
- Modify: `internal/query/parser_ast.go`
  - 识别 M2 属性、`.notnull` modifier。
- Modify: `internal/storage/query_scope.go`
  - 编译 M2 属性、depends/recur/parent、notnull。
- Modify: `internal/report/registry.go`
  - 注册 waiting/active/ready/blocked/blocking。
- Modify: `internal/app/service.go`
  - report 运行前刷新 waiting，并处理 until 隐藏和 app-level 派生过滤的报表。
- Modify: `internal/urgency/urgency.go`
  - 添加 active/waiting/blocked/blocking/annotations contribution。
- Modify: `internal/dom/dom.go`
  - M2 字段解析。
- Tests: query/storage/report/urgency/dom/app。

### Task 7: Query AST 支持 M2 属性和 notnull

**Files:**

- Modify: `internal/query/ast.go`
- Modify: `internal/query/parser_ast.go`
- Modify: `internal/query/parser_ast_test.go`
- Modify: `internal/storage/query_scope.go`
- Modify: `internal/storage/query_scope_test.go`

- [x] **Step 1: 写 query parser 失败测试**

在 `internal/query/parser_ast_test.go` 增加：

```go
func TestParseQueryM2Attributes(t *testing.T) {
	expr, err := ParseQuery(`wait: scheduled.before:eow start.notnull until: depends:abc annotations:note recur:weekly parent:p1`)
	if err != nil {
		t.Fatalf("ParseQuery() error = %v", err)
	}
	got := expr.String()
	for _, part := range []string{`wait is_null`, `scheduled before "eow"`, `start not_null`, `until is_null`, `depends eq "abc"`, `annotations contains "note"`, `recur eq "weekly"`, `parent eq "p1"`} {
		if !strings.Contains(got, part) {
			t.Fatalf("String() = %q, missing %q", got, part)
		}
	}
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./internal/query -run TestParseQueryM2Attributes -v`  
Expected: FAIL。

- [x] **Step 3: 扩展 AST**

修改 `internal/query/ast.go`：

```go
const (
	// existing...
	AttrStart Attribute = "start"
	AttrWait Attribute = "wait"
	AttrScheduled Attribute = "scheduled"
	AttrUntil Attribute = "until"
	AttrDepends Attribute = "depends"
	AttrAnnotations Attribute = "annotations"
	AttrRecur Attribute = "recur"
	AttrParent Attribute = "parent"
)

const (
	// existing...
	OpNotNull Operator = "not_null"
)
```

`Predicate.String()` 对 `OpNotNull` 和 `OpIsNull` 一样不打印 value；`annotations:foo` 使用 `OpContains`，避免误解成正则。

- [x] **Step 4: 扩展 parser**

修改 `internal/query/parser_ast.go`：

- `parseAttributeOperator` map 加入 start/wait/scheduled/until/depends/annotations/recur/parent。
- suffix 支持 `notnull`。
- 日期属性集合扩展为 due/entry/modified/end/start/wait/scheduled/until。
- `field:` 空值仍生成 OpIsNull。
- `annotations:<text>` 生成 `AttrAnnotations + OpContains`；`annotations:` 生成 `OpIsNull`，含义是没有任何 annotation。

- [x] **Step 5: 写 compiler 失败测试**

在 `internal/storage/query_scope_test.go` 增加：

```go
func TestCompileQueryM2Fields(t *testing.T) {
	expr, err := query.ParseQuery(`start.notnull wait: depends:dep annotations:note recur:weekly parent:p1`)
	if err != nil {
		t.Fatal(err)
	}
	sql, args, err := CompileQuery(expr, QueryCompileOptions{WorkspaceID: "w1", NowUnix: 100})
	if err != nil {
		t.Fatalf("CompileQuery() error = %v", err)
	}
	for _, part := range []string{"start IS NOT NULL", "wait IS NULL", "task_dependencies", "task_annotations", "recur = ?", "parent = ?"} {
		if !strings.Contains(sql, part) {
			t.Fatalf("sql = %s, missing %s", sql, part)
		}
	}
	if len(args) == 0 {
		t.Fatalf("args empty for sql %s", sql)
	}
}
```

- [x] **Step 6: 扩展 compiler**

修改 `internal/storage/query_scope.go`：

- start/wait/scheduled/until 调用 `compareDateColumn`。
- recur/parent 调用 `compareColumn`。
- depends 编译为：

```sql
uuid IN (SELECT task_uuid FROM task_dependencies WHERE depends_on = ?)
```

- annotations 编译为：

```sql
EXISTS (SELECT 1 FROM task_annotations WHERE task_uuid = uuid AND description LIKE ?)
```

- annotations 空值 `annotations:` 编译为：

```sql
NOT EXISTS (SELECT 1 FROM task_annotations WHERE task_uuid = uuid)
```

- OpNotNull 编译为 `column IS NOT NULL`。

- [x] **Step 7: 运行测试**

Run:

```bash
go test ./internal/query -run M2Attributes -v
go test ./internal/storage -run M2Fields -v
go test ./internal/query ./internal/storage -v
```

Expected: PASS。

- [x] **Step 8: 提交**

```bash
git add internal/query/ast.go internal/query/parser_ast.go internal/query/parser_ast_test.go internal/storage/query_scope.go internal/storage/query_scope_test.go
git commit -m "feat: 查询支持 M2 字段"
```

### Task 8: 状态刷新与 M2 报表

**Files:**

- Modify: `internal/report/registry.go`
- Modify: `internal/report/report_test.go`
- Modify: `internal/app/service.go`
- Modify: `internal/app/service_test.go`
- Modify: `internal/storage/task_repo.go`

- [x] **Step 1: 写 report registry 失败测试**

在 `internal/report/report_test.go` 更新 M1 中“不注册 waiting/active/ready/blocked/blocking”的断言，改为：

```go
func TestDefaultRegistryHasM2Reports(t *testing.T) {
	reg := DefaultRegistry()
	for _, name := range []string{"waiting", "active", "ready", "blocked", "blocking"} {
		if _, ok := reg.Get(name); !ok {
			t.Fatalf("missing M2 report %s", name)
		}
	}
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./internal/report -run M2Reports -v`  
Expected: FAIL。

- [x] **Step 3: 注册 M2 报表**

修改 `internal/report/registry.go`：

```go
type ScopeKind string

const (
	ScopeStatic ScopeKind = ""
	ScopeReady ScopeKind = "ready"
	ScopeBlocked ScopeKind = "blocked"
	ScopeBlocking ScopeKind = "blocking"
	ScopeHideUntilExpired ScopeKind = "hide_until_expired"
)

type Definition struct {
	Name         string
	Description  string
	FilterSource string
	Filter       query.Expr
	Sort         string
	Columns      []string
	Scope        ScopeKind
}

add(mustDefinition(Definition{Name: "waiting", Description: "Waiting tasks", FilterSource: "status:waiting", Sort: "wait", Columns: defaultColumns(), Scope: ScopeHideUntilExpired}))
add(mustDefinition(Definition{Name: "active", Description: "Active tasks", FilterSource: "status:pending start.notnull", Sort: "start", Columns: defaultColumns(), Scope: ScopeHideUntilExpired}))
add(mustDefinition(Definition{Name: "ready", Description: "Ready tasks", FilterSource: "status:pending", Sort: "urgency", Columns: defaultColumns(), Scope: ScopeReady}))
add(mustDefinition(Definition{Name: "blocked", Description: "Blocked tasks", FilterSource: "status:pending", Sort: "urgency", Columns: defaultColumns(), Scope: ScopeBlocked}))
add(mustDefinition(Definition{Name: "blocking", Description: "Blocking tasks", FilterSource: "status:pending", Sort: "urgency", Columns: defaultColumns(), Scope: ScopeBlocking}))
```

注意：

- `Definition.Scope` 应放在 `internal/report/report.go`，用于 app 层选择派生过滤，避免在 service 中到处用 report name 字符串判断。
- ready/blocked/blocking 需要 app-level 派生过滤，FilterSource 只是基础范围。
- 默认可见报表（list/next/ready/active/waiting/blocked/blocking）都应隐藏 `until <= now` 的 pending/waiting 任务，但不写库、不改 status；`all`、`completed`、`deleted` 不追加这个隐藏条件。

- [x] **Step 4: 写 app 报表失败测试**

在 `internal/app/service_test.go` 增加：

```go
func TestServiceM2Reports(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	waitUntil := int64(200)
	expiredUntil := int64(90)
	active, _ := svc.Add(AddInput{Description: "active"})
	waiting, _ := svc.Add(AddInput{Description: "waiting", Wait: &waitUntil})
	expired, _ := svc.Add(AddInput{Description: "expired", Until: &expiredUntil})
	dep, _ := svc.Add(AddInput{Description: "dep"})
	blocked, _ := svc.Add(AddInput{Description: "blocked"})
	_ = svc.Start(active.UUID)
	_ = svc.Modify(blocked.UUID, ModifyInput{AddDepends: []string{dep.UUID}})

	cases := map[string]string{
		"active": active.UUID,
		"waiting": waiting.UUID,
		"blocked": blocked.UUID,
		"blocking": dep.UUID,
	}
	for reportName, wantUUID := range cases {
		got, err := svc.ListReport(reportName, ListInput{})
		if err != nil {
			t.Fatalf("ListReport(%s) error = %v", reportName, err)
		}
		if !containsTask(got, wantUUID) {
			t.Fatalf("ListReport(%s) = %#v, missing %s", reportName, got, wantUUID)
		}
		if containsTask(got, expired.UUID) {
			t.Fatalf("ListReport(%s) includes expired until task: %#v", reportName, got)
		}
	}
	all, err := svc.ListReport("all", ListInput{})
	if err != nil {
		t.Fatalf("ListReport(all) error = %v", err)
	}
	if !containsTask(all, expired.UUID) {
		t.Fatalf("all should still include until-expired task without deleting it: %#v", all)
	}
}
```

- [x] **Step 5: 实现状态刷新和派生报表**

在 `internal/app/service.go`：

- 在 `List` / `RunReport` 开头调用 `s.refreshAutomaticState()`。
- `refreshAutomaticState`：
  - 查询 waiting 且 wait <= now 的任务，清空 wait，status pending。
- until 到期任务不在读路径物化为 deleted。M2 选择非破坏性隐藏：默认可见报表在 SQL filter 或 app-level filter 中追加 `until IS NULL OR until > now`；`all` 可以看到这些任务，用户仍可手动 modify/delete。
- `RunReport` 根据 `def.Scope` 做 app-level filter：
  - ready：pending、start nil、wait nil 或 wait <= now、scheduled nil 或 scheduled <= now、until nil 或 until > now、非 blocked。
  - blocked：有未完成依赖。
  - blocking：被未完成任务依赖。
  - blocked/blocking/active/waiting 也要排除 until 到期任务，除非 report 是 all/completed/deleted。

为了避免 O(n²) 到处散落，建议在 service 内新增 helper：

```go
func buildDependencyState(tasks []task.Task) (blocked map[string]bool, blocking map[string]bool)
```

依赖状态只考虑 status pending/waiting 的普通任务，不考虑 completed/deleted/recurring。

- [x] **Step 6: Storage sort 支持**

修改 `internal/storage/task_repo.go` sort：

- `wait`: `wait IS NULL ASC, wait ASC`
- `start`: `start DESC`
- urgency 继续 app 层排序，不在 repo sort。

- [x] **Step 7: 运行测试**

Run:

```bash
go test ./internal/report -v
go test ./internal/app -run M2Reports -v
go test ./internal/app ./internal/storage -v
```

Expected: PASS。

- [x] **Step 8: 提交**

```bash
git add internal/report/registry.go internal/report/report_test.go internal/app/service.go internal/app/service_test.go internal/storage/task_repo.go
git commit -m "feat: 添加 M2 状态报表"
```

### Task 9: Urgency 和 DOM 支持 M2 字段

**Files:**

- Modify: `internal/urgency/urgency.go`
- Modify: `internal/urgency/urgency_test.go`
- Modify: `internal/dom/dom.go`
- Modify: `internal/dom/dom_test.go`

- [x] **Step 1: 写 urgency 失败测试**

在 `internal/urgency/urgency_test.go` 增加：

```go
func TestExplainIncludesM2Contributions(t *testing.T) {
	start := int64(10)
	wait := int64(200)
	tsk := task.Task{
		UUID: "u1", Description: "task", Status: task.StatusWaiting, Entry: 0, Modified: 0,
		Start: &start, Wait: &wait,
		Annotations: []task.Annotation{{Entry: 1, Description: "note"}},
		Depends: []string{"dep"},
	}
	explain := Explain(tsk, Options{NowUnix: 100, Blocked: true, Blocking: true})
	for _, name := range []string{"active", "waiting", "blocked", "blocking", "annotations"} {
		if !hasItem(explain, name) {
			t.Fatalf("items missing %s: %#v", name, explain.Items)
		}
	}
}
```

- [x] **Step 2: 扩展 urgency Options 和实现**

修改 `internal/urgency/explain.go`：

```go
type Options struct {
	NowUnix int64
	Blocked bool
	Blocking bool
}
```

修改 `internal/urgency/urgency.go` 增加系数：

```go
coefBlocking = 8.0
coefActive = 4.0
coefAnnotations = 1.0
coefWaiting = -3.0
coefBlocked = -5.0
```

实现：

- start != nil：active contribution。
- Wait != nil && Wait > now 或 StatusWaiting：waiting negative。
- opts.Blocked：blocked negative。
- opts.Blocking：blocking positive。
- annotations：按数量 0.8/0.9/1.0。

注意：`app.RunReport` 对 urgency 排序时应传入 dependency state，否则 blocked/blocking 不会影响排序；更新 service 中调用 `urgency.Explain` 的地方。

在 `internal/app/service.go` 明确更新两条调用路径：

- `RunReport`：先加载当前 workspace 的完整普通任务集合，用 `buildDependencyState` 计算 blocked/blocking map；urgency 排序前为每个候选任务预计算 `urgency.Explain(tsk, urgency.Options{NowUnix: now, Blocked: blocked[tsk.UUID], Blocking: blocking[tsk.UUID]}).Total`，不要在 sort comparator 里重复 Explain。
- `ExplainUrgency(target)`：同样构建 dependency state 后传入 Options，保证 `xuanchu urgency <id>` 与 `next` 排序使用同一套 blocked/blocking 语义。

补充 app 单测：

```go
func TestUrgencyUsesDependencyState(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	blocker, _ := svc.Add(AddInput{Description: "blocker"})
	blocked, _ := svc.Add(AddInput{Description: "blocked"})
	plain, _ := svc.Add(AddInput{Description: "plain"})
	if err := svc.Modify(blocked.UUID, ModifyInput{AddDepends: []string{blocker.UUID}}); err != nil {
		t.Fatal(err)
	}
	blockedU, err := svc.ExplainUrgency(blocked.UUID)
	if err != nil {
		t.Fatal(err)
	}
	plainU, err := svc.ExplainUrgency(plain.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if blockedU.Total >= plainU.Total {
		t.Fatalf("blocked urgency = %.3f, plain = %.3f; blocked should be lower", blockedU.Total, plainU.Total)
	}
}
```

- [x] **Step 3: 写 DOM 失败测试**

在 `internal/dom/dom_test.go` 增加：

```go
func TestResolveM2Fields(t *testing.T) {
	start := int64(10)
	tsk := task.Task{
		UUID: "u1", Description: "task", Status: task.StatusPending, Start: &start,
		Depends: []string{"dep1", "dep2"},
		Annotations: []task.Annotation{{Entry: 1, Description: "note"}},
	}
	if got, _ := Resolve(tsk, "start", 0); got != "10" {
		t.Fatalf("start = %q", got)
	}
	if got, _ := Resolve(tsk, "depends", 0); got != "dep1,dep2" {
		t.Fatalf("depends = %q", got)
	}
	if got, _ := Resolve(tsk, "annotations", 0); !strings.Contains(got, "note") {
		t.Fatalf("annotations = %q", got)
	}
}
```

- [x] **Step 4: 扩展 DOM**

修改 `internal/dom/dom.go` 支持：

- start/wait/scheduled/until：nil 输出空，否则 Unix 秒字符串。
- depends：逗号分隔。
- annotations：M2 选择稳定格式 `entry:description`，多条用 `\n`。注意 `_get` 每个表达式自己 `Fprintln`，多行 annotations 可能产生多行；测试要接受这一点。
- recur/parent/mask/imask。

- [x] **Step 5: 运行测试**

Run:

```bash
go test ./internal/urgency -run M2 -v
go test ./internal/dom -run M2 -v
go test ./internal/urgency ./internal/dom ./internal/app -v
```

Expected: PASS。

- [x] **Step 6: 提交**

```bash
git add internal/urgency/urgency.go internal/urgency/explain.go internal/urgency/urgency_test.go internal/dom/dom.go internal/dom/dom_test.go internal/app/service.go
git commit -m "feat: urgency 和 DOM 支持 M2 字段"
```

## Chunk 4: Core CLI 命令

### 文件职责

- Modify: `internal/cli/root.go`
  - 注册 M2 命令和 target actions。
- Modify: `internal/cli/add.go`
  - AddInput 传递 M2 modification fields。
- Create: `internal/cli/activity.go`
  - start/stop commands 或 target actions helper。
- Create: `internal/cli/annotation.go`
  - annotate/denotate commands。
- Modify: `internal/cli/report.go`
  - 注册 waiting/active/ready/blocked/blocking commands。
- Modify: `tests/integration/cli_test.go`
  - Core CLI 集成测试。

### Task 10: Add/Modify CLI 传递 M2 字段

**Files:**

- Modify: `internal/cli/add.go`
- Modify: `internal/cli/root.go`
- Modify: `tests/integration/cli_test.go`

- [x] **Step 1: 写失败集成测试**

在 `tests/integration/cli_test.go` 增加：

```go
func TestCLIM2AddModifyFields(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	run(t, bin, "--db", db, "add", "waiting task", "wait:tomorrow", "scheduled:eow", "until:eom")
	waiting := run(t, bin, "--db", db, "waiting")
	if !strings.Contains(waiting, "waiting task") {
		t.Fatalf("waiting output = %q", waiting)
	}
	run(t, bin, "--db", db, "1", "modify", "wait:")
	list := run(t, bin, "--db", db, "list")
	if !strings.Contains(list, "waiting task") {
		t.Fatalf("list output = %q", list)
	}
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./tests/integration -run TestCLIM2AddModifyFields -v`  
Expected: FAIL。

- [x] **Step 3: 更新 add command**

修改 `internal/cli/add.go` 的 `svc.Add(app.AddInput{...})`，传递：

- Wait
- Scheduled
- Until
- Recur

- [x] **Step 4: 更新 root target action modify**

修改 `internal/cli/root.go` 的 `handleTargetAction`：

- knownActions 增加 start/stop/annotate/denotate/append/prepend。
- modify 分支传递 ClearDue/Wait/ClearWait/Scheduled/ClearScheduled/Until/ClearUntil/AddDepends/ClearDepends/Recur/ClearRecur。

本 Task 先确保 modify M2 fields 生效；其它 action 在后续任务实现。

- [x] **Step 5: 运行测试**

Run:

```bash
go test ./tests/integration -run TestCLIM2AddModifyFields -v
go test ./internal/cli ./tests/integration -v
```

Expected: PASS。

- [x] **Step 6: 提交**

```bash
git add internal/cli/add.go internal/cli/root.go tests/integration/cli_test.go
git commit -m "feat: CLI 支持 M2 修改字段"
```

### Task 11: start/stop/active CLI

**Files:**

- Create: `internal/cli/activity.go`
- Modify: `internal/cli/root.go`
- Modify: `tests/integration/cli_test.go`

- [x] **Step 1: 写失败集成测试**

在 `tests/integration/cli_test.go` 增加：

```go
func TestCLIStartStopActive(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	run(t, bin, "--db", db, "add", "active task")
	run(t, bin, "--db", db, "1", "start")
	active := run(t, bin, "--db", db, "active")
	if !strings.Contains(active, "active task") {
		t.Fatalf("active output = %q", active)
	}
	run(t, bin, "--db", db, "1", "stop")
	active = run(t, bin, "--db", db, "active")
	if strings.Contains(active, "active task") {
		t.Fatalf("stopped task still active: %q", active)
	}
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./tests/integration -run TestCLIStartStopActive -v`  
Expected: FAIL。

- [x] **Step 3: 实现 start/stop commands**

创建 `internal/cli/activity.go`：

- `newStartCommand(opts)`：`Use: "start <target>"`，调用 `svc.Start(args[0])`。
- `newStopCommand(opts)`：`Use: "stop <target>"`，调用 `svc.Stop(args[0])`。

同时 target action 支持：

- `xuanchu 1 start`
- `xuanchu 1 stop`

可以让 root target action 直接调用 service，而不是通过 Cobra command。

- [x] **Step 4: 注册 active report**

修改 `internal/cli/report.go`：

- 增加 `newActiveCommand(opts)`，使用现有 report command helper。
- 注册 waiting/ready/blocked/blocking 可以在 Task 13 完成；这里至少 active 可用。

修改 `internal/cli/root.go`：

- `cmd.AddCommand(newStartCommand(opts))`
- `cmd.AddCommand(newStopCommand(opts))`
- `cmd.AddCommand(newActiveCommand(opts))`
- knownSubcommands 增加 start/stop/active。

- [x] **Step 5: 运行测试**

Run:

```bash
go test ./tests/integration -run TestCLIStartStopActive -v
go test ./internal/cli ./tests/integration -v
```

Expected: PASS。

- [x] **Step 6: 提交**

```bash
git add internal/cli/activity.go internal/cli/root.go internal/cli/report.go tests/integration/cli_test.go
git commit -m "feat: 添加 start stop active CLI"
```

### Task 12: annotate/denotate CLI

**Files:**

- Create: `internal/cli/annotation.go`
- Modify: `internal/cli/root.go`
- Modify: `tests/integration/cli_test.go`

- [x] **Step 1: 写失败集成测试**

在 `tests/integration/cli_test.go` 增加：

```go
func TestCLIAnnotateDenotate(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	run(t, bin, "--db", db, "add", "annotated task")
	run(t, bin, "--db", db, "1", "annotate", "first note")
	got := run(t, bin, "--db", db, "_get", "1.annotations")
	if !strings.Contains(got, "first note") {
		t.Fatalf("annotations output = %q", got)
	}
	run(t, bin, "--db", db, "1", "denotate", "1")
	got = run(t, bin, "--db", db, "_get", "1.annotations")
	if strings.Contains(got, "first note") {
		t.Fatalf("annotation not removed: %q", got)
	}
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./tests/integration -run TestCLIAnnotateDenotate -v`  
Expected: FAIL。

- [x] **Step 3: 实现 annotation commands**

创建 `internal/cli/annotation.go`：

- `newAnnotateCommand(opts)`：`annotate <target> <description...>`，join args[1:]。
- `newDenotateCommand(opts)`：`denotate <target> <index>`。

支持 target action：

- `xuanchu 1 annotate "note"`
- `xuanchu 1 denotate 1`

修改 `handleTargetAction` 增加对应分支。

- [x] **Step 4: 注册命令**

修改 `internal/cli/root.go`：

- add commands。
- knownSubcommands/knownActions 增加 annotate/denotate。

- [x] **Step 5: 运行测试**

Run:

```bash
go test ./tests/integration -run TestCLIAnnotateDenotate -v
go test ./internal/cli ./tests/integration -v
```

Expected: PASS。

- [x] **Step 6: 提交**

```bash
git add internal/cli/annotation.go internal/cli/root.go tests/integration/cli_test.go
git commit -m "feat: 添加 annotate denotate CLI"
```

### Task 13: waiting/ready/blocked/blocking CLI

**Files:**

- Modify: `internal/cli/report.go`
- Modify: `internal/cli/root.go`
- Modify: `tests/integration/cli_test.go`

- [x] **Step 1: 写失败集成测试**

在 `tests/integration/cli_test.go` 增加：

```go
func TestCLIM2Reports(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	run(t, bin, "--db", db, "add", "blocker")
	blockerUUID := strings.TrimSpace(run(t, bin, "--db", db, "_uuids", "/blocker/"))
	run(t, bin, "--db", db, "add", "blocked", "depends:"+blockerUUID)
	run(t, bin, "--db", db, "add", "waiting", "wait:tomorrow")
	run(t, bin, "--db", db, "add", "ready", "scheduled:yesterday") // scheduled:today 会解析到当天 23:59:59，白天运行时还不是 ready。

	for name, want := range map[string]string{
		"blocked": "blocked",
		"blocking": "blocker",
		"waiting": "waiting",
		"ready": "ready",
	} {
		out := run(t, bin, "--db", db, name)
		if !strings.Contains(out, want) {
			t.Fatalf("%s output = %q, want %q", name, out, want)
		}
	}
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./tests/integration -run TestCLIM2Reports -v`  
Expected: FAIL。

- [x] **Step 3: 注册 report commands**

修改 `internal/cli/report.go`：

- `newWaitingCommand`
- `newReadyCommand`
- `newBlockedCommand`
- `newBlockingCommand`

如果 report command helper 可参数化，复用 helper。

修改 `internal/cli/root.go`：

- add commands。
- knownSubcommands 增加 waiting/ready/blocked/blocking。

- [x] **Step 4: 运行测试**

Run:

```bash
go test ./tests/integration -run TestCLIM2Reports -v
go test ./internal/cli ./tests/integration -v
```

Expected: PASS。

- [x] **Step 5: 提交**

```bash
git add internal/cli/report.go internal/cli/root.go tests/integration/cli_test.go
git commit -m "feat: 添加 M2 报表 CLI"
```

## Chunk 5: Editing

### 文件职责

- Create: `internal/edit/edit.go`
  - 编辑文件 DTO、marshal/unmarshal、校验。
- Create: `internal/edit/edit_test.go`
- Create: `internal/cli/edit.go`
  - append/prepend/edit CLI。
- Modify: `internal/cli/root.go`
- Modify: `tests/integration/cli_test.go`

### Task 14: append/prepend CLI

**Files:**

- Create: `internal/cli/edit.go`
- Modify: `internal/cli/root.go`
- Modify: `tests/integration/cli_test.go`

- [x] **Step 1: 写失败集成测试**

在 `tests/integration/cli_test.go` 增加：

```go
func TestCLIAppendPrepend(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	run(t, bin, "--db", db, "add", "middle")
	run(t, bin, "--db", db, "1", "append", "end")
	run(t, bin, "--db", db, "1", "prepend", "start")
	got := run(t, bin, "--db", db, "_get", "1.description")
	if !strings.Contains(got, "start middle end") {
		t.Fatalf("description = %q", got)
	}
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./tests/integration -run TestCLIAppendPrepend -v`  
Expected: FAIL。

- [x] **Step 3: 实现 commands**

创建 `internal/cli/edit.go`：

- `newAppendCommand(opts)`：`append <target> <text...>`。
- `newPrependCommand(opts)`：`prepend <target> <text...>`。

支持 target action：

- `xuanchu 1 append "end"`
- `xuanchu 1 prepend "start"`

修改 `internal/cli/root.go` 注册命令和 target actions。

- [x] **Step 4: 运行测试**

Run:

```bash
go test ./tests/integration -run TestCLIAppendPrepend -v
go test ./internal/cli ./tests/integration -v
```

Expected: PASS。

- [x] **Step 5: 提交**

```bash
git add internal/cli/edit.go internal/cli/root.go tests/integration/cli_test.go
git commit -m "feat: 添加 append prepend CLI"
```

### Task 15: edit 基础版

**Files:**

- Create: `internal/edit/edit.go`
- Create: `internal/edit/edit_test.go`
- Modify: `internal/cli/edit.go`
- Modify: `internal/cli/root.go`
- Modify: `tests/integration/cli_test.go`

- [x] **Step 1: 写 edit DTO 单元测试**

创建 `internal/edit/edit_test.go`：

```go
package edit

import (
	"strings"
	"testing"
)

func TestParseEditableTaskRejectsUUIDChange(t *testing.T) {
	original := EditableTask{UUID: "u1", Description: "task", Status: "pending"}
	input := `{"uuid":"u2","description":"task","status":"pending"}`
	if _, err := Parse([]byte(input), original); err == nil {
		t.Fatal("Parse() error = nil, want immutable uuid error")
	}
}

func TestParseEditableTaskUpdatesDescription(t *testing.T) {
	original := EditableTask{UUID: "u1", Description: "task", Status: "pending"}
	got, err := Parse([]byte(`{"uuid":"u1","description":"new","status":"pending"}`), original)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !strings.Contains(got.Description, "new") {
		t.Fatalf("Description = %q", got.Description)
	}
}
```

- [x] **Step 2: 实现 edit package**

创建 `internal/edit/edit.go`：

- M2 选择 JSON 临时文件格式，避免新增 TOML 依赖。
- `EditableTask` 包含 M2 可编辑字段，但不包含 workspace_id。
- `FromTask(task.Task) EditableTask`
- `Apply(original task.Task, edited EditableTask) (task.Task, error)`
- `Parse(data []byte, original EditableTask) (EditableTask, error)`，禁止 uuid/entry 修改。

- [x] **Step 3: 写 CLI 集成测试**

在 `tests/integration/cli_test.go` 增加：

```go
func TestCLIEditWithTestEditor(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	editor := buildEditorHelper(t, `package main
import (
	"encoding/json"
	"os"
)
func main() {
	path := os.Args[1]
	data, err := os.ReadFile(path)
	if err != nil { panic(err) }
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil { panic(err) }
	doc["description"] = "edited task"
	data, err = json.Marshal(doc)
	if err != nil { panic(err) }
	if err := os.WriteFile(path, data, 0o600); err != nil { panic(err) }
}`)
	run(t, bin, "--db", db, "add", "original task")
	cmd := exec.Command(bin, "--db", db, "1", "edit")
	cmd.Env = append(os.Environ(), "EDITOR="+editor)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("edit failed: %v\n%s", err, out)
	}
	got := run(t, bin, "--db", db, "_get", "1.description")
	if !strings.Contains(got, "edited task") {
		t.Fatalf("description = %q", got)
	}
}
```

- [x] **Step 4: 实现 CLI edit**

修改 `internal/cli/edit.go`：

- `newEditCommand(opts)`：`edit <target>`。
- Resolve target。
- 写 JSON 到 temp file。
- 调用 `$EDITOR` 或 `vi`。
- 读取、解析、校验。
- 调用 service 更新任务。可以新增 `svc.ReplaceEditableTask(target string, edited task.Task)`，或通过 ModifyInput 逐字段更新；推荐新增 service 方法，避免 edit CLI 直接操作 repo。
- 测试 editor 使用 `buildEditorHelper(t, source string)` 临时构建 Go 小程序，避免依赖 JSON 字段顺序、缩进、`sed`、`python3` 等外部条件。helper 可写入 `t.TempDir()` 后执行 `go build -o <editor> <main.go>`。

- [x] **Step 5: 注册 command 和 target action**

修改 `internal/cli/root.go`：

- add `newEditCommand(opts)`。
- knownSubcommands/knownActions 增加 `edit`。

- [x] **Step 6: 运行测试**

Run:

```bash
go test ./internal/edit -v
go test ./tests/integration -run TestCLIEditWithTestEditor -v
go test ./...
```

Expected: PASS。

- [x] **Step 7: 提交**

```bash
git add internal/edit/edit.go internal/edit/edit_test.go internal/cli/edit.go internal/cli/root.go internal/app/service.go tests/integration/cli_test.go
git commit -m "feat: 添加基础 edit 命令"
```

## Chunk 6: Recurrence

### 文件职责

- Create: `internal/recurrence/recurrence.go`
  - Validate、Parse、Next、child generation helpers。
- Create: `internal/recurrence/recurrence_test.go`
- Modify: `internal/app/service.go`
  - recurring add/done/report ensure。
- Modify: `internal/storage/task_repo.go`
  - 查询 parent/children helper，如有需要。
- Modify: `internal/cli/add.go`
  - recur add 已在前面传入，补行为测试。
- Modify: `tests/integration/cli_test.go`

### Task 16: recurrence 周期计算

**Files:**

- Create: `internal/recurrence/recurrence.go`
- Create: `internal/recurrence/recurrence_test.go`
- Modify: `internal/task/model.go`

- [x] **Step 1: 写失败测试**

创建 `internal/recurrence/recurrence_test.go`：

```go
package recurrence

import (
	"testing"
	"time"
)

func TestNextDailyWeeklyMonthlyAndN(t *testing.T) {
	loc := time.UTC
	base := time.Date(2030, 1, 1, 23, 59, 59, 0, loc).Unix()
	tests := map[string]time.Time{
		"daily": time.Date(2030, 1, 2, 23, 59, 59, 0, loc),
		"weekly": time.Date(2030, 1, 8, 23, 59, 59, 0, loc),
		"monthly": time.Date(2030, 2, 1, 23, 59, 59, 0, loc),
		"3days": time.Date(2030, 1, 4, 23, 59, 59, 0, loc),
	}
	for expr, want := range tests {
		got, err := Next(base, expr, loc)
		if err != nil {
			t.Fatalf("Next(%s) error = %v", expr, err)
		}
		if got != want.Unix() {
			t.Fatalf("Next(%s) = %v, want %v", expr, time.Unix(got, 0).UTC(), want)
		}
	}
}

func TestValidateRejectsUnsupportedRecurrence(t *testing.T) {
	if err := Validate("fortnightly"); err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
}

func TestTaskValidateRequiresRecurringRecurAndDue(t *testing.T) {
	tsk := task.Task{UUID: "u1", WorkspaceID: "w1", Description: "parent", Status: task.StatusRecurring, Entry: 1, Modified: 1}
	if err := tsk.Validate(); err == nil {
		t.Fatal("recurring without recur/due should fail")
	}
	recur := "weekly"
	tsk.Recur = &recur
	if err := tsk.Validate(); err == nil {
		t.Fatal("recurring without due should fail")
	}
	due := int64(100)
	tsk.Due = &due
	if err := tsk.Validate(); err != nil {
		t.Fatalf("recurring with recur and due should pass: %v", err)
	}
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./internal/recurrence -v`  
Expected: FAIL，package 不存在。

- [x] **Step 3: 实现 recurrence package**

创建 `internal/recurrence/recurrence.go`：

```go
package recurrence

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func Validate(expr string) error {
	_, err := parse(expr)
	return err
}

func Next(fromUnix int64, expr string, loc *time.Location) (int64, error) {
	if loc == nil {
		loc = time.Local
	}
	n, unit, err := parse(expr)
	if err != nil {
		return 0, err
	}
	from := time.Unix(fromUnix, 0).In(loc)
	switch unit {
	case "days":
		return from.AddDate(0, 0, n).Unix(), nil
	case "weeks":
		return from.AddDate(0, 0, 7*n).Unix(), nil
	case "months":
		return from.AddDate(0, n, 0).Unix(), nil
	default:
		return 0, fmt.Errorf("unsupported recurrence %q", expr)
	}
}

func parse(expr string) (int, string, error) {
	switch expr {
	case "daily":
		return 1, "days", nil
	case "weekly":
		return 1, "weeks", nil
	case "monthly":
		return 1, "months", nil
	}
	for _, unit := range []string{"days", "weeks", "months"} {
		if strings.HasSuffix(expr, unit) {
			n, err := strconv.Atoi(strings.TrimSuffix(expr, unit))
			if err != nil || n <= 0 {
				return 0, "", fmt.Errorf("invalid recurrence %q", expr)
			}
			return n, unit, nil
		}
	}
	return 0, "", fmt.Errorf("unsupported recurrence %q", expr)
}
```

修改 `internal/task/model.go` 的 recur validation：

- 如果 `t.Recur != nil`，调用 `recurrence.Validate(*t.Recur)`。
- 如果 `t.Status == task.StatusRecurring`，要求 `t.Recur != nil` 且 `t.Due != nil`，否则返回清晰错误；M2 的 recurring 父任务必须有 due 作为 child due 锚点。
- 为避免 import cycle，`internal/recurrence` 必须保持纯周期表达式包，不 import `internal/task`；这样 `task` → `recurrence` 是单向依赖，可以接受。
- `monthly` 使用 Go `time.AddDate(0, n, 0)`，M2 接受其月末边界行为，README 需在 Task 19 记录基础版限制。

- [x] **Step 4: 运行测试**

Run:

```bash
go test ./internal/recurrence -v
go test ./internal/task -v
```

Expected: PASS。

- [x] **Step 5: 提交**

```bash
git add internal/recurrence/recurrence.go internal/recurrence/recurrence_test.go internal/task/model.go
git commit -m "feat: 添加 recurrence 周期计算"
```

### Task 17: recurring add 和 done 生成 child

**Files:**

- Modify: `internal/app/service.go`
- Modify: `internal/app/service_test.go`
- Modify: `internal/storage/task_repo.go`
- Modify: `tests/integration/cli_test.go`

- [x] **Step 1: 写 app 失败测试**

在 `internal/app/service_test.go` 增加：

```go
func TestServiceRecurringAddAndDoneCreatesNextChild(t *testing.T) {
	svc, closeFn := newTestService(t, mustUnix(t, "2030-01-01T10:00:00Z"))
	defer closeFn()
	due := mustUnix(t, "2030-01-01T23:59:59Z")
	until := mustUnix(t, "2030-02-01T23:59:59Z")
	recur := "daily"
	parent, err := svc.Add(AddInput{Description: "daily task", Due: &due, Until: &until, Recur: &recur})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if parent.Status != task.StatusRecurring {
		t.Fatalf("parent status = %s", parent.Status)
	}
	tasks, _ := svc.List(ListInput{ReportMode: false})
	if len(tasks) != 1 || tasks[0].Parent == nil || *tasks[0].Parent != parent.UUID {
		t.Fatalf("visible child not created: %#v", tasks)
	}
	firstChild := tasks[0]
	if err := svc.Done(firstChild.UUID); err != nil {
		t.Fatalf("Done(child) error = %v", err)
	}
	tasks, _ = svc.List(ListInput{})
	if len(tasks) != 1 || tasks[0].UUID == firstChild.UUID {
		t.Fatalf("next child not generated: %#v", tasks)
	}
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./internal/app -run TestServiceRecurringAddAndDoneCreatesNextChild -v`  
Expected: FAIL。

- [x] **Step 3: 设计 app recurrence helpers**

在 `internal/app/service.go` 增加 helpers：

```go
func (s *Service) createRecurringParent(input AddInput, now int64) (task.Task, error)
func (s *Service) createNextRecurringChild(parent task.Task, previous *task.Task, now int64) (task.Task, error)
func (s *Service) ensureRecurringChildren() error
```

规则：

- `Add` 发现 `input.Recur != nil`：
  - 创建 parent：status recurring，recur set，普通字段保留。
  - 立即创建 child：status pending，parent = parent.UUID，继承 description/project/priority/tags/recur/until。
  - child due：如果 parent due 非空，用 parent due；后续 child 用 recurrence.Next(previous.Due)。
- `Done(child)`：
  - 原 done 流程完成 child。
  - 如果 child.Parent != nil，加载 parent；若 parent until 未到，创建 next child。
- 防重复：
  - 创建 child 前检查 parent 是否已有 pending/waiting child；有则不创建。

如果 repository 当前没有按 parent 查询方法，新增 `Children(workspaceID, parentUUID string) ([]task.Task, error)`。

- [x] **Step 4: 更新 storage helper**

在 `internal/storage/task_repo.go` 添加：

```go
func (r *TaskRepository) Children(workspaceID, parentUUID string) ([]domain.Task, error)
```

按 `workspace_id` 和 `parent` 查询，preload associations，entry ASC。

- [x] **Step 5: 写 CLI 集成测试**

在 `tests/integration/cli_test.go` 增加：

```go
func TestCLIRecurringDaily(t *testing.T) {
	bin := buildXuanchu(t)
	db := filepath.Join(t.TempDir(), "xuanchu.db")
	run(t, bin, "--db", db, "add", "daily task", "recur:daily", "due:2030-01-01", "until:2030-01-05")
	list := run(t, bin, "--db", db, "list")
	if !strings.Contains(list, "daily task") {
		t.Fatalf("list output = %q", list)
	}
	run(t, bin, "--db", db, "1", "done")
	list = run(t, bin, "--db", db, "list")
	if !strings.Contains(list, "daily task") {
		t.Fatalf("next recurring child missing: %q", list)
	}
	all := run(t, bin, "--db", db, "all")
	if strings.Contains(all, "recurring") {
		t.Fatalf("recurring parent should stay hidden from human table: %q", all)
	}
}
```

实施时如果 human table 不显示 status，改用 `export` JSON 验证 parent status recurring 且默认 reports 隐藏。

- [x] **Step 6: 运行测试**

Run:

```bash
go test ./internal/app -run Recurring -v
go test ./tests/integration -run TestCLIRecurringDaily -v
go test ./internal/app ./internal/storage ./internal/recurrence -v
```

Expected: PASS。

- [x] **Step 7: 提交**

```bash
git add internal/app/service.go internal/app/service_test.go internal/storage/task_repo.go tests/integration/cli_test.go
git commit -m "feat: 生成基础循环任务"
```

### Task 18: recurrence ensure、until 和 JSON roundtrip

**Files:**

- Modify: `internal/app/service.go`
- Modify: `internal/task/json.go`
- Modify: `internal/task/json_test.go`
- Modify: `tests/integration/cli_test.go`

- [x] **Step 1: 写 until 停止生成测试**

在 `internal/app/service_test.go` 增加：

```go
func TestRecurringStopsAtUntil(t *testing.T) {
	svc, closeFn := newTestService(t, mustUnix(t, "2030-01-01T10:00:00Z"))
	defer closeFn()
	due := mustUnix(t, "2030-01-01T23:59:59Z")
	until := due
	recur := "daily"
	_, err := svc.Add(AddInput{Description: "daily", Due: &due, Until: &until, Recur: &recur})
	if err != nil {
		t.Fatal(err)
	}
	tasks, _ := svc.List(ListInput{})
	if len(tasks) != 1 {
		t.Fatalf("children = %#v", tasks)
	}
	if err := svc.Done(tasks[0].UUID); err != nil {
		t.Fatal(err)
	}
	tasks, _ = svc.List(ListInput{})
	if len(tasks) != 0 {
		t.Fatalf("child generated after until: %#v", tasks)
	}
}
```

- [x] **Step 2: 实现 until guard 和 ensure**

在 `createNextRecurringChild`：

- 如果 parent.Until != nil 且 nextDue > *parent.Until，不创建。
- recurring parent 必须有 due；该规则已在 Task 16 接入 domain validation。不要用 previous.Entry 或 now 作为 fallback，否则 child due 会不可预测。

在 `List` / `RunReport` 的自动刷新中调用 `ensureRecurringChildren()`。

- [x] **Step 3: JSON recurring roundtrip**

确认 Task 6 已覆盖 recur/parent/mask/imask。补 integration：

```go
func TestCLIRecurringExportImport(t *testing.T) {
	// create recurring, export JSON, import into fresh db, ensure recur/parent visible in export
}
```

具体实现可复用现有 import/export helper。

- [x] **Step 4: 运行测试**

Run:

```bash
go test ./internal/app -run Recurring -v
go test ./internal/task -run JSON -v
go test ./tests/integration -run Recurring -v
go test ./...
```

Expected: PASS。

- [x] **Step 5: 提交**

```bash
git add internal/app/service.go internal/app/service_test.go internal/task/json.go internal/task/json_test.go tests/integration/cli_test.go
git commit -m "feat: 完成循环任务边界"
```

## Chunk 7: 文档、验收与收尾

### 文件职责

- Modify: `README.md`
- Modify: `ROADMAP.md`
- Modify: `docs/superpowers/specs/2026-05-28-xuanchu-m2-design.md`
  - 如实现中有范围决策变化，同步回 spec。
- Test: full suite。

### Task 19: README 和 ROADMAP 更新

**Files:**

- Modify: `README.md`
- Modify: `ROADMAP.md`

- [x] **Step 1: 更新 README**

在 `README.md` 增加 M2 用法：

````markdown
## M2 任务状态与循环任务用法

```bash
./xuanchu 1 start
./xuanchu active
./xuanchu 1 stop
./xuanchu add "Call vendor" wait:tomorrow
./xuanchu waiting
./xuanchu 1 annotate "called customer"
./xuanchu 1 denotate 1
./xuanchu add "Write docs" depends:<uuid>
./xuanchu blocked
./xuanchu blocking
./xuanchu 1 append "with examples"
./xuanchu 1 prepend "[draft]"
./xuanchu 1 edit
./xuanchu add "Weekly report" recur:weekly due:friday until:2030-12-31
```

M2 的 recurring 为基础兼容版：支持 daily/weekly/monthly/Ndays/Nweeks/Nmonths，父任务隐藏，完成子任务时按需生成下一个子任务。
Recurring 必须同时提供 `recur` 和 `due`；`monthly` 使用 Go `time.AddDate` 的基础月度推进，月末日期可能滚动到下月初。
默认可见报表会隐藏 `until` 到期的 pending/waiting 任务，但不会自动把它们改成 deleted；使用 `all` 仍可看到这些任务。
````

- [x] **Step 2: 更新 ROADMAP**

在 `ROADMAP.md`：

- 将 M2 状态改为“已完成”。
- 简述 M2 实际交付范围。
- 将当前下一步改为 M3。

- [x] **Step 3: 提交**

```bash
git add README.md ROADMAP.md
git commit -m "docs: 更新 M2 使用说明和路线图"
```

### Task 20: 最终验收

**Files:**

- No code changes expected.

- [x] **Step 1: 全量测试**

Run:

```bash
go test ./...
```

Expected: PASS。

- [x] **Step 2: CGO-free 测试**

Run:

```bash
CGO_ENABLED=0 go test ./...
```

Expected: PASS。

- [x] **Step 3: CGO-free build**

Run:

```bash
CGO_ENABLED=0 go build ./cmd/xuanchu
```

Expected: PASS。

- [x] **Step 4: 确认没有引入 CGO SQLite driver**

Run:

```bash
if go list -m all | grep -E 'gorm.io/driver/sqlite|mattn/go-sqlite3'; then
  echo "unexpected CGO SQLite dependency" >&2
  exit 1
fi
```

Expected: 无输出，exit code 0。

- [x] **Step 5: 手动冒烟**

Run:

```bash
tmp="$(mktemp -d)"
go run ./cmd/xuanchu --db "$tmp/xuanchu.db" add "active task"
go run ./cmd/xuanchu --db "$tmp/xuanchu.db" 1 start
go run ./cmd/xuanchu --db "$tmp/xuanchu.db" active
go run ./cmd/xuanchu --db "$tmp/xuanchu.db" 1 stop
go run ./cmd/xuanchu --db "$tmp/xuanchu.db" add "blocker"
blocker="$(go run ./cmd/xuanchu --db "$tmp/xuanchu.db" _uuids /blocker/)"
go run ./cmd/xuanchu --db "$tmp/xuanchu.db" add "blocked task" "depends:$blocker"
go run ./cmd/xuanchu --db "$tmp/xuanchu.db" blocked
go run ./cmd/xuanchu --db "$tmp/xuanchu.db" blocking
go run ./cmd/xuanchu --db "$tmp/xuanchu.db" 1 annotate "manual note"
go run ./cmd/xuanchu --db "$tmp/xuanchu.db" _get 1.annotations
go run ./cmd/xuanchu --db "$tmp/xuanchu.db" add "weekly report" recur:weekly due:tomorrow until:eom
go run ./cmd/xuanchu --db "$tmp/xuanchu.db" next
go run ./cmd/xuanchu --db "$tmp/xuanchu.db" export
```

Expected:

- active 显示 active task。
- blocked/blocking 分别显示对应任务。
- `_get 1.annotations` 输出 manual note。
- recurring 任务在 next/list 中显示 child，不显示 parent。
- export JSON 包含 M2 字段。

- [x] **Step 6: 检查 git 状态**

Run:

```bash
git status --short --branch
```

Expected: 干净。

## 计划审阅说明

本计划根据 [docs/superpowers/specs/2026-05-28-xuanchu-m2-design.md](/Users/mac/code/projects/dajee/task/docs/superpowers/specs/2026-05-28-xuanchu-m2-design.md)、[AGENTS.md](/Users/mac/code/projects/dajee/task/AGENTS.md) 和当前 M1 代码编写。

## 评审反馈

本计划已吸收人工评审中的阻塞和重要反馈：

- 补齐 `annotations` 查询属性，包含 parser、AST、SQL 编译和空值语义。
- 将 `until` 到期处理改为非破坏性隐藏，不在 list/report 读路径自动标记 deleted。
- 将 ready 集成测试从 `scheduled:today` 改为 `scheduled:yesterday`，避免日内时间敏感失败。
- 将依赖环检测前移到 app depends 接入之前，避免中间提交允许环依赖。
- 明确 urgency 的 blocked/blocking state 必须同时接入 `RunReport` 排序和 `ExplainUrgency`。
- 补充 `ClearProject`、`ClearPriority`、`ClearTags`、JSON nil/empty slice merge、recurring 必须同时提供 `recur` 和 `due`。
- 将 edit 集成测试改为临时 Go helper editor，避免依赖 JSON 字段顺序、`sed` 或 `python3`。

当前未执行 plan-document-reviewer subagent 审阅；本环境虽提供通用 multi-agent 工具，但未提供明确的 plan-document-reviewer 角色或提示文件，也没有 `plan-document-reviewer-prompt.md` 可引用。后续如需要严格执行 superpowers 审阅环节，请用专门 reviewer prompt 对每个 chunk 进行审阅。实施过程中若发现本计划与 spec 冲突，以 spec 为准并先更新计划。
