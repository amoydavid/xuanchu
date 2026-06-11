# P1 语义事件补齐 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** 为 Xuanchu 补齐 9 个 Priority 1 语义事件，让 Hook 和 notification rule 可以按字段级粒度精确订阅任务变更。

**Architecture:** 新增统一 TaskChangeDiff 构建器，各写路径在修改前保存 before 快照，修改后对比 diff 并构建细粒度事件列表。start/stop 替换 task.modified 为 task.started/task.stopped；modify 叠加细粒度事件。blocked 事件通过 buildDependencyState 前后对比检测。

**Tech Stack:** Go 1.25，纯 Go SQLite，GORM

**Spec:** `docs/superpowers/specs/2026-06-10-xuanchu-p1-semantic-events-design.md`

---

## 文件结构

| 操作 | 文件 | 职责 |
|---|---|---|
| 新建 | `internal/app/task_change_events.go` | TaskChangeDiff 结构体、diffTaskChanges、buildFineGrainedEvents、细粒度事件 payload 构建 |
| 新建 | `internal/app/task_change_events_test.go` | diff 和事件构建的单元测试 |
| 修改 | `internal/app/hook.go:24-33` | allowedHookEventTypes 白名单扩展 |
| 修改 | `internal/app/service.go` | Modify/Start/Stop/Add/Done 写路径改造 |
| 修改 | `internal/app/hook_event.go` | 新增 buildTaskStartedEvent/buildTaskStoppedEvent/buildTaskBlockedEvent |
| 修改 | `internal/app/hook_test.go` | 更新 TestHookEventsForWriteOperations、新增细粒度事件测试 |
| 修改 | `internal/mcpserver/tools_hook.go:29` | Events jsonschema description 更新 |
| 修改 | `internal/mcpserver/tools_notification.go:160` | Event jsonschema description 更新 |

---

## Chunk 1: TaskChangeDiff 构建器与白名单扩展

### Task 1: 新建 TaskChangeDiff 结构体和 diffTaskChanges 函数

**Files:**
- Create: `internal/app/task_change_events.go`
- Create: `internal/app/task_change_events_test.go`

- [x] **Step 1: 写 TaskChangeDiff 结构体和 diffTaskChanges 的测试**

```go
// internal/app/task_change_events_test.go
package app

import (
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/task"
)

func strptr(s string) *string { return &s }
func intptr(i int64) *int64   { return &i }

func TestDiffTaskChanges_NoChanges(t *testing.T) {
	before := task.Task{UUID: "a", Description: "test"}
	after := before
	diff := diffTaskChanges(before, after)
	if diff.PriorityChanged || diff.DueChanged || diff.ProjectChanged || diff.TagsChanged || diff.AssigneesChanged {
		t.Fatal("expected no changes")
	}
}

func TestDiffTaskChanges_PriorityChanged(t *testing.T) {
	before := task.Task{UUID: "a"}
	after := task.Task{UUID: "a", Priority: strptr("H")}
	diff := diffTaskChanges(before, after)
	if !diff.PriorityChanged {
		t.Fatal("expected PriorityChanged")
	}
	if diff.PreviousPriority != nil {
		t.Fatalf("PreviousPriority = %v, want nil", diff.PreviousPriority)
	}
	if *diff.CurrentPriority != "H" {
		t.Fatalf("CurrentPriority = %v, want H", diff.CurrentPriority)
	}
}

func TestDiffTaskChanges_DueChanged(t *testing.T) {
	before := task.Task{UUID: "a", Due: intptr(100)}
	after := task.Task{UUID: "a", Due: intptr(200)}
	diff := diffTaskChanges(before, after)
	if !diff.DueChanged {
		t.Fatal("expected DueChanged")
	}
	if *diff.PreviousDue != 100 {
		t.Fatalf("PreviousDue = %d, want 100", *diff.PreviousDue)
	}
	if *diff.CurrentDue != 200 {
		t.Fatalf("CurrentDue = %d, want 200", *diff.CurrentDue)
	}
}

func TestDiffTaskChanges_DueCleared(t *testing.T) {
	before := task.Task{UUID: "a", Due: intptr(100)}
	after := task.Task{UUID: "a"}
	diff := diffTaskChanges(before, after)
	if !diff.DueChanged {
		t.Fatal("expected DueChanged when due cleared")
	}
	if *diff.PreviousDue != 100 {
		t.Fatalf("PreviousDue = %d, want 100", *diff.PreviousDue)
	}
	if diff.CurrentDue != nil {
		t.Fatalf("CurrentDue = %v, want nil", diff.CurrentDue)
	}
}

func TestDiffTaskChanges_ProjectChanged(t *testing.T) {
	before := task.Task{UUID: "a", Project: strptr("old")}
	after := task.Task{UUID: "a", Project: strptr("new")}
	diff := diffTaskChanges(before, after)
	if !diff.ProjectChanged {
		t.Fatal("expected ProjectChanged")
	}
	if *diff.PreviousProject != "old" {
		t.Fatalf("PreviousProject = %v, want old", diff.PreviousProject)
	}
	if *diff.CurrentProject != "new" {
		t.Fatalf("CurrentProject = %v, want new", diff.CurrentProject)
	}
}

func TestDiffTaskChanges_TagsChanged(t *testing.T) {
	before := task.Task{UUID: "a", Tags: []string{"a", "b"}}
	after := task.Task{UUID: "a", Tags: []string{"b", "c"}}
	diff := diffTaskChanges(before, after)
	if !diff.TagsChanged {
		t.Fatal("expected TagsChanged")
	}
	if len(diff.AddedTags) != 1 || diff.AddedTags[0] != "c" {
		t.Fatalf("AddedTags = %v, want [c]", diff.AddedTags)
	}
	if len(diff.RemovedTags) != 1 || diff.RemovedTags[0] != "a" {
		t.Fatalf("RemovedTags = %v, want [a]", diff.RemovedTags)
	}
}

func TestDiffTaskChanges_AssigneesChanged(t *testing.T) {
	before := task.Task{UUID: "a", Assignees: []task.AssigneeInfo{{UserID: "u1"}, {UserID: "u2"}}}
	after := task.Task{UUID: "a", Assignees: []task.AssigneeInfo{{UserID: "u1"}, {UserID: "u3"}}}
	diff := diffTaskChanges(before, after)
	if !diff.AssigneesChanged {
		t.Fatal("expected AssigneesChanged")
	}
	if len(diff.AddedAssignees) != 0 {
		t.Fatalf("AddedAssignees should be empty before hydrate, got %v", diff.AddedAssignees)
	}
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `cd /Users/mac/code/projects/dajee/task && go test ./internal/app/ -run TestDiffTaskChanges -v`
Expected: 编译失败，diffTaskChanges 未定义

- [x] **Step 3: 实现 diffTaskChanges**

```go
// internal/app/task_change_events.go
package app

import (
	"git.dajee.net/dajee/xuanchu/internal/task"
)

type TaskChangeDiff struct {
	AssigneesChanged bool
	AddedAssignees   []task.UserInfo
	RemovedAssignees []task.UserInfo

	DueChanged  bool
	PreviousDue *int64
	CurrentDue  *int64

	PriorityChanged  bool
	PreviousPriority *string
	CurrentPriority  *string

	ProjectChanged  bool
	PreviousProject *string
	CurrentProject  *string

	TagsChanged bool
	AddedTags   []string
	RemovedTags []string
}

func diffTaskChanges(before, after task.Task) TaskChangeDiff {
	var diff TaskChangeDiff

	if ptrStringDiff(before.Priority, after.Priority) {
		diff.PriorityChanged = true
		diff.PreviousPriority = before.Priority
		diff.CurrentPriority = after.Priority
	}

	if ptrInt64Diff(before.Due, after.Due) {
		diff.DueChanged = true
		diff.PreviousDue = before.Due
		diff.CurrentDue = after.Due
	}

	if ptrStringDiff(before.Project, after.Project) {
		diff.ProjectChanged = true
		diff.PreviousProject = before.Project
		diff.CurrentProject = after.Project
	}

	beforeTags := tagSet(before.Tags)
	afterTags := tagSet(after.Tags)
	if !stringSetEqual(beforeTags, afterTags) {
		diff.TagsChanged = true
		diff.AddedTags = stringSetDifference(afterTags, beforeTags)
		diff.RemovedTags = stringSetDifference(beforeTags, afterTags)
	}

	beforeAssignees := assigneeIDSet(before.Assignees)
	afterAssignees := assigneeIDSet(after.Assignees)
	if !stringSetEqual(beforeAssignees, afterAssignees) {
		diff.AssigneesChanged = true
	}

	return diff
}

func ptrStringDiff(a, b *string) bool {
	if a == nil && b == nil {
		return false
	}
	if a == nil || b == nil {
		return true
	}
	return *a != *b
}

func ptrInt64Diff(a, b *int64) bool {
	if a == nil && b == nil {
		return false
	}
	if a == nil || b == nil {
		return true
	}
	return *a != *b
}

func tagSet(tags []string) map[string]bool {
	m := make(map[string]bool, len(tags))
	for _, t := range tags {
		m[t] = true
	}
	return m
}

func assigneeIDSet(assignees []task.AssigneeInfo) map[string]bool {
	m := make(map[string]bool, len(assignees))
	for _, a := range assignees {
		m[a.UserID] = true
	}
	return m
}

func stringSetEqual(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

func stringSetDifference(a, b map[string]bool) []string {
	var diff []string
	for k := range a {
		if !b[k] {
			diff = append(diff, k)
		}
	}
	return diff
}
```

- [x] **Step 4: 运行测试确认通过**

Run: `cd /Users/mac/code/projects/dajee/task && go test ./internal/app/ -run TestDiffTaskChanges -v`
Expected: PASS

- [x] **Step 5: 提交**

```bash
git add internal/app/task_change_events.go internal/app/task_change_events_test.go
git commit -m "feat: 新增 TaskChangeDiff 结构体和 diffTaskChanges"
```

### Task 2: 实现 buildFineGrainedEvents 函数

**Files:**
- Modify: `internal/app/task_change_events.go`
- Modify: `internal/app/task_change_events_test.go`

- [x] **Step 1: 写 buildFineGrainedEvents 的测试**

在 `task_change_events_test.go` 中追加：

```go
func TestBuildFineGrainedEvents_PriorityChanged(t *testing.T) {
	diff := TaskChangeDiff{
		PriorityChanged:  true,
		PreviousPriority: strptr("M"),
		CurrentPriority:  strptr("H"),
	}
	after := task.Task{UUID: "t1", Priority: strptr("H")}
	events := buildFineGrainedEvents(diff, after, RuntimeContext{}, 1000)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventType != "task.priority_changed" {
		t.Fatalf("event type = %q, want task.priority_changed", events[0].EventType)
	}
	if events[0].Data["previous_priority"] != "M" {
		t.Fatalf("previous_priority = %v, want M", events[0].Data["previous_priority"])
	}
	if events[0].Data["current_priority"] != "H" {
		t.Fatalf("current_priority = %v, want H", events[0].Data["current_priority"])
	}
}

func TestBuildFineGrainedEvents_MultipleChanges(t *testing.T) {
	diff := TaskChangeDiff{
		PriorityChanged:  true,
		PreviousPriority: strptr("L"),
		CurrentPriority:  strptr("H"),
		TagsChanged:      true,
		AddedTags:        []string{"urgent"},
		RemovedTags:      []string{"docs"},
	}
	after := task.Task{UUID: "t1", Priority: strptr("H"), Tags: []string{"urgent"}}
	events := buildFineGrainedEvents(diff, after, RuntimeContext{}, 1000)
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	types := map[string]bool{}
	for _, e := range events {
		types[e.EventType] = true
	}
	if !types["task.priority_changed"] || !types["task.tags_changed"] {
		t.Fatalf("expected task.priority_changed and task.tags_changed, got %v", types)
	}
}

func TestBuildFineGrainedEvents_NoChanges(t *testing.T) {
	diff := TaskChangeDiff{}
	after := task.Task{UUID: "t1"}
	events := buildFineGrainedEvents(diff, after, RuntimeContext{}, 1000)
	if len(events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(events))
	}
}

func TestBuildFineGrainedEvents_DueChanged(t *testing.T) {
	diff := TaskChangeDiff{
		DueChanged:  true,
		PreviousDue: intptr(100),
		CurrentDue:  intptr(200),
	}
	after := task.Task{UUID: "t1", Due: intptr(200)}
	events := buildFineGrainedEvents(diff, after, RuntimeContext{}, 1000)
	if len(events) != 1 || events[0].EventType != "task.due_changed" {
		t.Fatalf("expected task.due_changed, got %v", events)
	}
}

func TestBuildFineGrainedEvents_ProjectChanged(t *testing.T) {
	diff := TaskChangeDiff{
		ProjectChanged:  true,
		PreviousProject: strptr("old"),
		CurrentProject:  strptr("new"),
	}
	after := task.Task{UUID: "t1", Project: strptr("new")}
	events := buildFineGrainedEvents(diff, after, RuntimeContext{}, 1000)
	if len(events) != 1 || events[0].EventType != "task.project_changed" {
		t.Fatalf("expected task.project_changed, got %v", events)
	}
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `cd /Users/mac/code/projects/dajee/task && go test ./internal/app/ -run TestBuildFineGrainedEvents -v`
Expected: 编译失败

- [x] **Step 3: 实现 buildFineGrainedEvents**

在 `task_change_events.go` 中追加：

```go
func buildFineGrainedEvents(diff TaskChangeDiff, after task.Task, runtime RuntimeContext, now int64) []HookEvent {
	var events []HookEvent

	if diff.PriorityChanged {
		e := buildTaskHookEvent("task.priority_changed", after, runtime, now)
		e.Data["previous_priority"] = diff.PreviousPriority
		e.Data["current_priority"] = diff.CurrentPriority
		events = append(events, e)
	}

	if diff.DueChanged {
		e := buildTaskHookEvent("task.due_changed", after, runtime, now)
		e.Data["previous_due"] = diff.PreviousDue
		e.Data["current_due"] = diff.CurrentDue
		events = append(events, e)
	}

	if diff.ProjectChanged {
		e := buildTaskHookEvent("task.project_changed", after, runtime, now)
		e.Data["previous_project"] = diff.PreviousProject
		e.Data["current_project"] = diff.CurrentProject
		events = append(events, e)
	}

	if diff.TagsChanged {
		e := buildTaskHookEvent("task.tags_changed", after, runtime, now)
		e.Data["added_tags"] = diff.AddedTags
		e.Data["removed_tags"] = diff.RemovedTags
		e.Data["current_tags"] = after.Tags
		events = append(events, e)
	}

	if diff.AssigneesChanged {
		if len(diff.AddedAssignees) > 0 {
			e := buildTaskHookEvent("task.assigned", after, runtime, now)
			e.Data["added_assignees"] = userInfosToJSONList(diff.AddedAssignees)
			e.Data["current_assignees"] = userInfosToJSONList(currentAssigneeInfos(after.Assignees, diff.AddedAssignees, diff.RemovedAssignees))
			events = append(events, e)
		}
		if len(diff.RemovedAssignees) > 0 {
			e := buildTaskHookEvent("task.unassigned", after, runtime, now)
			e.Data["removed_assignees"] = userInfosToJSONList(diff.RemovedAssignees)
			e.Data["current_assignees"] = userInfosToJSONList(currentAssigneeInfos(after.Assignees, diff.AddedAssignees, diff.RemovedAssignees))
			events = append(events, e)
		}
	}

	return events
}

func userInfosToJSONList(infos []task.UserInfo) []map[string]any {
	out := make([]map[string]any, len(infos))
	for i, info := range infos {
		out[i] = task.UserInfoToJSON(info)
	}
	return out
}

func currentAssigneeInfos(current []task.AssigneeInfo, added, removed []task.UserInfo) []task.UserInfo {
	removedSet := make(map[string]bool, len(removed))
	for _, r := range removed {
		removedSet[r.ID] = true
	}
	var result []task.UserInfo
	for _, a := range current {
		if !removedSet[a.UserID] {
			result = append(result, task.UserInfo{ID: a.UserID, Name: a.Name, Email: a.Email})
		}
	}
	return result
}
```

注意：`buildTaskHookEvent` 已在 `hook_event.go` 中定义。`task.UserInfoToJSON` 已在 `task` 包中定义。

- [x] **Step 4: 运行测试确认通过**

Run: `cd /Users/mac/code/projects/dajee/task && go test ./internal/app/ -run TestBuildFineGrainedEvents -v`
Expected: PASS

- [x] **Step 5: 提交**

```bash
git add internal/app/task_change_events.go internal/app/task_change_events_test.go
git commit -m "feat: 实现 buildFineGrainedEvents 细粒度事件构建器"
```

### Task 3: 扩展 allowedHookEventTypes 白名单

**Files:**
- Modify: `internal/app/hook.go:24-33`

- [x] **Step 1: 扩展白名单**

将 `internal/app/hook.go` 中的 `allowedHookEventTypes` 从：

```go
var allowedHookEventTypes = map[string]bool{
	"task.created":      true,
	"task.modified":     true,
	"task.completed":    true,
	"task.deleted":      true,
	"project.archived":  true,
	"project.annotated": true,
	"project.denotated": true,
	"task.unblocked":    true,
}
```

改为：

```go
var allowedHookEventTypes = map[string]bool{
	"task.created":         true,
	"task.modified":        true,
	"task.completed":       true,
	"task.deleted":         true,
	"task.started":         true,
	"task.stopped":         true,
	"task.assigned":        true,
	"task.unassigned":      true,
	"task.blocked":         true,
	"task.due_changed":     true,
	"task.priority_changed":  true,
	"task.project_changed":   true,
	"task.tags_changed":      true,
	"task.unblocked":         true,
	"project.archived":       true,
	"project.annotated":      true,
	"project.denotated":      true,
}
```

- [x] **Step 2: 验证现有测试不受影响**

Run: `cd /Users/mac/code/projects/dajee/task && go test ./internal/app/ -run TestHook -v -count=1 | tail -30`
Expected: PASS

- [x] **Step 3: 提交**

```bash
git add internal/app/hook.go
git commit -m "feat: 扩展 allowedHookEventTypes 白名单至 17 个事件类型"
```

---

## Chunk 2: 写路径改造 — Start / Stop

### Task 4: Start/Stop 改用 task.started / task.stopped

**Files:**
- Modify: `internal/app/service.go` — `Start` 方法（约 line 758-770）
- Modify: `internal/app/service.go` — `Stop` 方法（约 line 792-804）

- [x] **Step 1: 修改 Start 方法**

在 `internal/app/service.go` 中，将 `Start` 方法从：

```go
func (s *Service) Start(target string) error {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return err
	}
	return s.withAuditAndEvents(func(tx *Service) (*AuditEntry, []HookEvent, error) {
		startedTask, change, err := tx.startLocked(target)
		if err != nil {
			return nil, nil, err
		}
		event := buildTaskHookEvent("task.modified", startedTask, tx.runtime, tx.clock.Unix())
		entry := taskAuditEntry("task.start", startedTask.UUID, change)
		return &entry, []HookEvent{event}, nil
	})
}
```

改为：

```go
func (s *Service) Start(target string) error {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return err
	}
	return s.withAuditAndEvents(func(tx *Service) (*AuditEntry, []HookEvent, error) {
		startedTask, change, err := tx.startLocked(target)
		if err != nil {
			return nil, nil, err
		}
		event := buildTaskHookEvent("task.started", startedTask, tx.runtime, tx.clock.Unix())
		entry := taskAuditEntry("task.start", startedTask.UUID, change)
		return &entry, []HookEvent{event}, nil
	})
}
```

- [x] **Step 2: 修改 Stop 方法**

将 `Stop` 方法从：

```go
event := buildTaskHookEvent("task.modified", stoppedTask, tx.runtime, tx.clock.Unix())
```

改为：

```go
event := buildTaskHookEvent("task.stopped", stoppedTask, tx.runtime, tx.clock.Unix())
```

- [x] **Step 3: 更新 TestHookEventsForWriteOperations 测试**

在 `internal/app/hook_test.go` 的 `TestHookEventsForWriteOperations` 中：

1. 将 hook 的 EventTypes 扩展为包含新事件类型：

```go
EventTypes: []string{
	"task.created", "task.modified", "task.completed", "task.deleted",
	"task.started", "task.stopped",
},
```

2. 将 `// Start -> task.modified` 改为 `// Start -> task.started`，断言改为：

```go
assertDeliveryEventType(t, svc, hook.ID, "task.started")
```

3. 将 `// Stop -> task.modified` 改为 `// Stop -> task.stopped`，断言改为：

```go
assertDeliveryEventType(t, svc, hook.ID, "task.stopped")
```

- [x] **Step 4: 运行测试**

Run: `cd /Users/mac/code/projects/dajee/task && go test ./internal/app/ -run TestHookEventsForWriteOperations -v -count=1`
Expected: PASS

- [x] **Step 5: 提交**

```bash
git add internal/app/service.go internal/app/hook_test.go
git commit -m "feat: start/stop 改用 task.started/task.stopped 事件"
```

---

## Chunk 3: 写路径改造 — Modify 细粒度事件

### Task 5: Modify 写路径保存 before 快照并叠加细粒度事件

**Files:**
- Modify: `internal/app/service.go` — `Modify` 方法（约 line 493-506）

- [x] **Step 1: 修改 Modify 方法**

将 `Modify` 方法从：

```go
func (s *Service) Modify(target string, input ModifyInput) error {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return err
	}
	return s.withAuditAndEvents(func(tx *Service) (*AuditEntry, []HookEvent, error) {
		modified, change, err := tx.modifyLocked(target, input)
		if err != nil {
			return nil, nil, err
		}
		event := buildTaskHookEvent("task.modified", modified, tx.runtime, tx.clock.Unix())
		entry := taskAuditEntry("task.modify", modified.UUID, change)
		return &entry, []HookEvent{event}, nil
	})
}
```

改为：

```go
func (s *Service) Modify(target string, input ModifyInput) error {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return err
	}
	return s.withAuditAndEvents(func(tx *Service) (*AuditEntry, []HookEvent, error) {
		before, err := tx.resolveTargetForWrite(target)
		if err != nil {
			return nil, nil, err
		}
		modified, change, err := tx.modifyLocked(target, input)
		if err != nil {
			return nil, nil, err
		}
		now := tx.clock.Unix()
		diff := diffTaskChanges(before, modified)
		diff = tx.hydrateAssigneeDiff(diff, before.Assignees, modified.Assignees)
		fineGrained := buildFineGrainedEvents(diff, modified, tx.runtime, now)
		blocked := tx.detectBlockedEventsAfterModify(before, modified)
		events := []HookEvent{buildTaskHookEvent("task.modified", modified, tx.runtime, now)}
		events = append(events, fineGrained...)
		events = append(events, blocked...)
		entry := taskAuditEntry("task.modify", modified.UUID, change)
		return &entry, events, nil
	})
}
```

- [x] **Step 2: 实现 hydrateAssigneeDiff**

在 `task_change_events.go` 中追加：

```go
func (s *Service) hydrateAssigneeDiff(diff TaskChangeDiff, before, after []task.AssigneeInfo) TaskChangeDiff {
	if !diff.AssigneesChanged {
		return diff
	}
	beforeIDs := assigneeIDSet(before)
	afterIDs := assigneeIDSet(after)
	var addedIDs, removedIDs []string
	for id := range afterIDs {
		if !beforeIDs[id] {
			addedIDs = append(addedIDs, id)
		}
	}
	for id := range beforeIDs {
		if !afterIDs[id] {
			removedIDs = append(removedIDs, id)
		}
	}
	allIDs := append(addedIDs, removedIDs...)
	infos, err := s.resolveUserInfos(allIDs)
	if err != nil {
		return diff
	}
	for _, id := range addedIDs {
		diff.AddedAssignees = append(diff.AddedAssignees, infos[id])
	}
	for _, id := range removedIDs {
		diff.RemovedAssignees = append(diff.RemovedAssignees, infos[id])
	}
	return diff
}
```

- [x] **Step 3: 写 Modify 细粒度事件的集成测试**

在 `internal/app/hook_test.go` 中追加新测试：

```go
func TestHookModifyFineGrainedEvents(t *testing.T) {
	svc, _, cleanup := hookTestEnv(t)
	defer cleanup()

	user := createHookTestUser(t, svc, "modifier")
	assignee := createHookTestUser(t, svc, "assignee")
	svc.memberRepo.Add(assignee.ID, svc.workspaceID, "member")

	hook, err := svc.AddHook(HookAddInput{
		Name: "all-events", ScopeType: HookScopeWorkspace,
		EventTypes: []string{
			"task.modified", "task.priority_changed", "task.tags_changed",
			"task.due_changed", "task.project_changed", "task.assigned", "task.unassigned",
		},
		SinkRef: "hook-sink", TimeoutSeconds: 10, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatal(err)
	}

	created, err := svc.Add(AddInput{Description: "test task", Assignees: []string{assignee.ID}})
	if err != nil {
		t.Fatal(err)
	}

	err = svc.Modify(created.UUID, ModifyInput{
		Priority:      strptr("H"),
		AddTags:       []string{"urgent"},
		RemoveTags:    []string{},
		Due:           intptr(2000000000),
		AddAssignees:  []string{user.ID},
	})
	if err != nil {
		t.Fatal(err)
	}

	deliveries, err := svc.hookDeliveryRepo.ListByHook(hook.ID, "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}

	eventTypes := map[string]bool{}
	for _, d := range deliveries {
		if d.EventType == "task.created" {
			continue
		}
		eventTypes[d.EventType] = true
	}

	for _, want := range []string{"task.modified", "task.priority_changed", "task.tags_changed", "task.due_changed", "task.assigned"} {
		if !eventTypes[want] {
			t.Errorf("missing event type %q in deliveries; got events: %v", want, eventTypes)
		}
	}
}
```

- [x] **Step 4: 运行测试**

Run: `cd /Users/mac/code/projects/dajee/task && go test ./internal/app/ -run TestHookModifyFineGrainedEvents -v -count=1`
Expected: PASS

- [x] **Step 5: 提交**

```bash
git add internal/app/service.go internal/app/task_change_events.go internal/app/hook_test.go
git commit -m "feat: modify 写路径叠加细粒度事件"
```

---

## Chunk 4: 写路径改造 — Blocked 事件

### Task 6: 实现 blocked 事件检测

**Files:**
- Modify: `internal/app/task_change_events.go`
- Modify: `internal/app/service.go` — Modify 方法补充 blocked 检测
- Modify: `internal/app/service.go` — Add 方法补充 blocked 检测
- Modify: `internal/app/hook_test.go`

- [x] **Step 1: 在 hook_event.go 中新增 buildTaskBlockedHookEvent**

在 `internal/app/hook_event.go` 中追加：

```go
func buildTaskBlockedHookEvent(tsk task.Task, blockingDeps []string, runtime RuntimeContext, now int64) HookEvent {
	event := buildTaskHookEvent("task.blocked", tsk, runtime, now)
	event.Data["blocking_dependencies"] = blockingDeps
	return event
}
```

- [x] **Step 2: 实现 detectBlockedEventsAfterModify 和 detectBlockedEventsAfterAdd**

在 `task_change_events.go` 中追加：

```go
func (s *Service) detectBlockedEventsAfterModify(before, after task.Task) []HookEvent {
	if len(before.Depends) == len(after.Depends) {
		return nil
	}
	beforeDepSet := map[string]bool{}
	for _, d := range before.Depends {
		beforeDepSet[d] = true
	}
	var newDeps []string
	for _, d := range after.Depends {
		if !beforeDepSet[d] {
			newDeps = append(newDeps, d)
		}
	}
	if len(newDeps) == 0 {
		return nil
	}
	var blockingDeps []string
	for _, depUUID := range newDeps {
		dep, err := s.repo.GetByUUID(s.workspaceID, depUUID)
		if err != nil {
			continue
		}
		if dep.Status != task.StatusCompleted && dep.Status != task.StatusDeleted {
			blockingDeps = append(blockingDeps, depUUID)
		}
	}
	if len(blockingDeps) == 0 {
		return nil
	}
	now := s.clock.Unix()
	return []HookEvent{buildTaskBlockedHookEvent(after, blockingDeps, s.runtime, now)}
}

func (s *Service) detectBlockedEventsAfterAdd(created task.Task) []HookEvent {
	if len(created.Depends) == 0 {
		return nil
	}
	var blockingDeps []string
	for _, depUUID := range created.Depends {
		dep, err := s.repo.GetByUUID(s.workspaceID, depUUID)
		if err != nil {
			continue
		}
		if dep.Status != task.StatusCompleted && dep.Status != task.StatusDeleted {
			blockingDeps = append(blockingDeps, depUUID)
		}
	}
	if len(blockingDeps) == 0 {
		return nil
	}
	now := s.clock.Unix()
	return []HookEvent{buildTaskBlockedHookEvent(created, blockingDeps, s.runtime, now)}
}
```

- [x] **Step 3: 修改 Add 方法补充 blocked 事件**

在 `service.go` 的 `Add` 方法中，将：

```go
event := buildTaskHookEvent("task.created", created, tx.runtime, tx.clock.Unix())
entry := taskAuditEntry("task.add", created.UUID, change)
return &entry, []HookEvent{event}, nil
```

改为：

```go
event := buildTaskHookEvent("task.created", created, tx.runtime, tx.clock.Unix())
blocked := tx.detectBlockedEventsAfterAdd(created)
events := []HookEvent{event}
events = append(events, blocked...)
entry := taskAuditEntry("task.add", created.UUID, change)
return &entry, events, nil
```

同样修改 `AddWithAnnotations` 中的 `return entries, []HookEvent{event}, nil`。

- [x] **Step 4: 写 blocked 事件测试**

在 `hook_test.go` 中追加：

```go
func TestHookBlockedEventOnAddWithDependency(t *testing.T) {
	svc, _, cleanup := hookTestEnv(t)
	defer cleanup()

	hook, err := svc.AddHook(HookAddInput{
		Name: "blocked-hook", ScopeType: HookScopeWorkspace,
		EventTypes:     []string{"task.blocked"},
		SinkRef:        "hook-sink",
		TimeoutSeconds: 10, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatal(err)
	}

	blocker, err := svc.Add(AddInput{Description: "blocker"})
	if err != nil {
		t.Fatal(err)
	}

	blocked, err := svc.Add(AddInput{
		Description: "blocked task",
		Depends:     []string{blocker.UUID},
	})
	if err != nil {
		t.Fatal(err)
	}

	deliveries, err := svc.hookDeliveryRepo.ListByHook(hook.ID, "", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(deliveries) == 0 {
		t.Fatal("expected task.blocked delivery")
	}
	if deliveries[0].EventType != "task.blocked" {
		t.Fatalf("event_type = %q, want task.blocked", deliveries[0].EventType)
	}
	_ = blocked
}

func TestHookBlockedEventOnModifyAddDependency(t *testing.T) {
	svc, _, cleanup := hookTestEnv(t)
	defer cleanup()

	hook, err := svc.AddHook(HookAddInput{
		Name: "blocked-hook", ScopeType: HookScopeWorkspace,
		EventTypes:     []string{"task.blocked"},
		SinkRef:        "hook-sink",
		TimeoutSeconds: 10, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatal(err)
	}

	blocker, err := svc.Add(AddInput{Description: "blocker"})
	if err != nil {
		t.Fatal(err)
	}

	target, err := svc.Add(AddInput{Description: "target"})
	if err != nil {
		t.Fatal(err)
	}

	err = svc.Modify(target.UUID, ModifyInput{
		AddDepends: []string{blocker.UUID},
	})
	if err != nil {
		t.Fatal(err)
	}

	assertDeliveryEventType(t, svc, hook.ID, "task.blocked")
}
```

- [x] **Step 5: 运行测试**

Run: `cd /Users/mac/code/projects/dajee/task && go test ./internal/app/ -run TestHookBlockedEvent -v -count=1`
Expected: PASS

- [x] **Step 6: 提交**

```bash
git add internal/app/task_change_events.go internal/app/hook_event.go internal/app/service.go internal/app/hook_test.go
git commit -m "feat: 实现 task.blocked 事件检测"
```

---

## Chunk 5: 外部接口更新与全量验证

### Task 7: 更新 MCP tool schema description

**Files:**
- Modify: `internal/mcpserver/tools_hook.go:29`
- Modify: `internal/mcpserver/tools_notification.go:160`

- [x] **Step 1: 更新 tools_hook.go Events 字段 description**

将 `Events` 字段的 jsonschema description 更新为包含全部 17 个事件类型：

```go
Events    []string `json:"events" jsonschema:"event types (task.created, task.modified, task.completed, task.deleted, task.started, task.stopped, task.assigned, task.unassigned, task.blocked, task.due_changed, task.priority_changed, task.project_changed, task.tags_changed, task.unblocked, project.archived, project.annotated, project.denotated)"`
```

- [x] **Step 2: 更新 tools_notification.go Event 字段 description**

将 `Event` 字段的 jsonschema description 更新：

```go
Event           string   `json:"event" jsonschema:"event type (task.created, task.modified, task.completed, task.deleted, task.started, task.stopped, task.assigned, task.unassigned, task.blocked, task.due_changed, task.priority_changed, task.project_changed, task.tags_changed, task.unblocked, project.archived, project.annotated, project.denotated)"`
```

- [x] **Step 3: 验证 MCP golden test**

Run: `cd /Users/mac/code/projects/dajee/task && go test ./internal/mcpserver/ -run TestToolSchema -v -count=1`
Expected: 可能需要更新 golden 文件。如果失败，用 `-update` 更新。

- [x] **Step 4: 提交**

```bash
git add internal/mcpserver/tools_hook.go internal/mcpserver/tools_notification.go
git commit -m "feat: 更新 MCP tool schema description 包含新事件类型"
```

### Task 8: 更新 MCP golden test 文件

**Files:**
- Modify: `internal/mcpserver/testdata/*.json`（如有 golden test 需要更新）

- [x] **Step 1: 运行 golden test 确认是否需要更新**

Run: `cd /Users/mac/code/projects/dajee/task && go test ./internal/mcpserver/ -run TestToolSchema -v -count=1`

如果失败，检查输出中的 diff，然后用 `-update` flag 更新：

Run: `cd /Users/mac/code/projects/dajee/task && go test ./internal/mcpserver/ -run TestToolSchema -update -count=1`

- [x] **Step 2: 提交更新的 golden 文件（如有）**

```bash
git add internal/mcpserver/testdata/
git commit -m "chore: 更新 MCP schema golden test"
```

### Task 9: 全量测试验证

- [x] **Step 1: 运行全量测试**

```bash
cd /Users/mac/code/projects/dajee/task && go test ./... -count=1
```

Expected: PASS

- [x] **Step 2: 运行 CGO_ENABLED=0 测试**

```bash
cd /Users/mac/code/projects/dajee/task && CGO_ENABLED=0 go test ./... -count=1
```

Expected: PASS

- [x] **Step 3: 运行 CGO_ENABLED=0 构建**

```bash
cd /Users/mac/code/projects/dajee/task && CGO_ENABLED=0 go build ./cmd/xuanchu
```

Expected: 成功

- [x] **Step 4: 提交所有改动（如有遗漏）**

检查 `git status`，确保所有改动已提交。

---

## 验收清单

- [x] `allowedHookEventTypes` 包含全部 17 个事件类型
- [x] Hook 和 notification rule 可订阅全部新事件类型
- [x] `Modify` 按实际变更字段发射 `task.modified` + 细粒度事件
- [x] `Start` 只发射 `task.started`，不再发射 `task.modified`
- [x] `Stop` 只发射 `task.stopped`，不再发射 `task.modified`
- [x] 细粒度事件 payload 包含标准 task 快照和差异字段
- [x] `task.blocked` 在 add 带依赖和 modify 新增依赖时正确触发
- [x] `task.unblocked` 继续正常工作
- [x] `annotate`、`denotate`、`append`、`prepend`、`edit` 继续只发射 `task.modified`
- [x] 现有 `task.created`、`task.completed`、`task.deleted` 行为不变
- [x] `go test ./...` 通过
- [x] `CGO_ENABLED=0 go test ./...` 通过
- [x] `CGO_ENABLED=0 go build ./cmd/xuanchu` 通过
