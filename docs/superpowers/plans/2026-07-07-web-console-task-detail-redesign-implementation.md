# Web Console 任务详情页重构 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将任务详情页重构为「主叙事区 + 右侧分组属性栏」，并补齐手动子任务（sub-task）创建与列表读取的最小闭环。

**Architecture:** 后端复用现有 `app.Service.Add`，新增 `parent` 入参与校验（自引用/环/跨 workspace/deleted/completed/recurring 拒绝），新增专用 `GET /tasks/{ref}/children` 端点；前端在现有布局新增子任务区与就地 composer（Enter 连续创建），随后做纯结构重排（主区顺序、右栏分组、Activity 视觉合并）。分两阶段提交，分别可回退。

**Tech Stack:** Go 1.25 / GORM + glebarez/sqlite（零 CGO）/ chi+huma HTTP；React + TypeScript + TanStack Query + shadcn-ui。

**承接 spec：** `docs/superpowers/specs/2026-07-07-web-console-task-detail-redesign-design.md`

**阶段划分（与 spec §12 一致，提交必须拆开）：**
- **阶段一（Task 1–8）：** 手动 sub-task 能力闭环（后端 parent 写入 + 校验、children 端点、前端 composer + 列表）。
- **阶段二（Task 9–11）：** 纯结构重排（主区顺序、右栏分组、Activity 视觉合并）。

---

## 关键约定（所有任务共同遵守）

1. **术语统一**（spec §1.1）：产品叫「任务/子任务」，不改名为 issue。
2. **recurring 判定**：`task.Status == task.StatusRecurring`（"recurring"），不是单独 bool。
3. **错误码风格**：`RuntimeError{Code: "...", Message: "..."}`（见 `internal/app/runtime.go:67`）。
4. **新错误码**（本计划引入）：
   - `task_invalid_parent` — parent 不存在/跨 workspace/自引用/环。
   - `task_parent_deleted` — parent 是 deleted。
   - `task_parent_completed` — parent 是 completed（文案附替代动作）。
   - `task_parent_recurring` — parent 是 recurring parent（不允许手动子任务）。
5. **子任务读取端点**（spec §8.3）：必须用专用 `/children` 端点，不用 `parent:<uuid>` query（后者带 query 后默认 pending 过滤失效）。
6. **提交信息**用中文，`feat:` / `test:` / `refactor:` / `docs:`。

---

## 文件结构总览

### 后端（阶段一）
- 修改 `internal/app/service.go` — `AddInput` 加 `Parent *string`；`addLocked` 解析并校验 parent。
- 修改 `internal/app/request_scope.go` — 新增 `validateParent` 辅助。
- 修改 `internal/httpapi/tasks.go` — `addTaskRequest` 加 `Parent`；`handleTaskAdd` 映射；新增 `handleTaskChildren`。
- 修改 `internal/httpapi/huma_routes.go` — 注册 `/api/v1/tasks/{taskRef}/children`。
- 修改 `internal/app/service.go` — 新增 `ListChildren(target string, input ListChildrenInput)`。
- 测试：`internal/app/service_test.go`、`internal/httpapi/tasks_test.go`。

### 前端（阶段一）
- 修改 `web/src/features/workspace/project-workbench/api/task-api.ts` — `TaskCreateInput` 加 `parent?`；新增 `listTaskChildren`。
- 修改 `web/src/features/workspace/project-workbench/hooks/use-task-detail-data.ts` — 新增 `taskQueryKeys.children` 与 `useTaskChildrenQuery`。
- 修改 `web/src/features/workspace/project-workbench/hooks/use-task-mutations.ts` — 新增 `useCreateSubTaskMutation`（成功后失效 children + task + projectTasksPrefix）。
- 新建 `web/src/features/workspace/project-workbench/task-detail/sub-task-list.tsx` — 子任务列表 + 就地 composer。
- 修改 `web/src/features/workspace/project-workbench/task-detail/task-detail-page.tsx` — 挂载 `SubTaskList`。
- 修改 `web/src/locales/zh-CN.ts` 与 `web/src/locales/en-US.ts` — 新增文案。

### 前端（阶段二）
- 修改 `task-detail-page.tsx` — 主区顺序、移动端 tab。
- 修改 `task-property-panel.tsx` — Properties/Schedule/Relations/System/Custom 分组，空组隐身。
- 新建 `web/src/features/workspace/project-workbench/task-detail/activity-section.tsx` — Activity 视觉合并容器。

---

# 阶段一：手动 sub-task 能力闭环

## Task 1: 后端 AddInput 增加 Parent 字段与解析

**Files:**
- Modify: `internal/app/service.go:74-88`（`AddInput`）、`internal/app/service.go:444-478`（`addLocked`）
- Test: `internal/app/service_test.go`

- [ ] **Step 1: 写失败测试 — parent 用 UUID 创建子任务**

在 `internal/app/service_test.go` 末尾追加（参照 `TestServiceAddListInfo` 的 `newTestService` fixture 模式，`newTestService(t, now)` 返回 `(*Service, func())`）：

```go
func TestServiceAddWithParent(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	// 建项目，父任务放入项目内，验证子任务继承 project。
	project, err := svc.AddProject(AddProjectInput{Slug: "demo", Name: "Demo"})
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}

	parent, err := svc.Add(AddInput{Title: "parent task", Project: &project.Slug})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}

	child, err := svc.Add(AddInput{Title: "child task", Parent: &parent.UUID})
	if err != nil {
		t.Fatalf("Add with parent: %v", err)
	}
	if child.Parent == nil || *child.Parent != parent.UUID {
		t.Fatalf("child.Parent = %v, want %s", child.Parent, parent.UUID)
	}
	if child.Project == nil || *child.Project != project.Slug {
		t.Fatalf("child.Project = %v, want inherited %q", child.Project, project.Slug)
	}

	got, err := svc.Info(child.UUID)
	if err != nil {
		t.Fatalf("Info(child): %v", err)
	}
	if got.Parent == nil || *got.Parent != parent.UUID {
		t.Fatalf("Info child.Parent = %v, want %s", got.Parent, parent.UUID)
	}
}

// 额外：parent 无 project 时，子任务 project 也为 nil（不 panic）。
func TestServiceAddWithParentNoProject(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	parent, err := svc.Add(AddInput{Title: "parent no project"})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	child, err := svc.Add(AddInput{Title: "child", Parent: &parent.UUID})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if child.Parent == nil || *child.Parent != parent.UUID {
		t.Fatalf("child.Parent = %v, want %s", child.Parent, parent.UUID)
	}
	if child.Project != nil {
		t.Fatalf("child.Project = %v, want nil", child.Project)
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./internal/app/ -run TestServiceAddWithParent -v`
Expected: FAIL — 编译错误（`AddInput` 无 `Parent` 字段）。

- [ ] **Step 3: 在 AddInput 加 Parent 字段**

修改 `internal/app/service.go:74-88`，在 `UDAs` 字段后加一行：

```go
type AddInput struct {
	Title       string
	Description *string
	Project     *string
	Priority    *string
	Due         *int64
	Assignees   []string
	Depends     []string
	Wait        *int64
	Scheduled   *int64
	Until       *int64
	Recur       *string
	Tags        []string
	UDAs        map[string]string
	Parent      *string
}
```

- [ ] **Step 4: 在 addLocked 解析 parent 并写入 task**

修改 `internal/app/service.go` 的 `addLocked`（约 444-478 行）。在 `depends, err := s.resolveDependencyTargets(input.Depends)` 之后、构造 `tsk` 之前，插入 parent 解析与校验，并在构造 `tsk` 时写入 `Parent`。

由于 parent 校验逻辑较多，先写一个占位校验函数（Task 2 再补全所有拒绝分支），此处只做基础解析 + project 继承：

在 `addLocked` 内 `depends` 解析之后插入：

```go
	parentUUID, parentProject, err := s.resolveParentForAdd(input.Parent, input.Project)
	if err != nil {
		return task.Task{}, projectChange{}, err
	}
```

并把构造 `tsk` 的项目绑定改为用继承后的 project：

将原来：
```go
	tsk := task.Task{
		UUID: uuid.NewString(), WorkspaceID: s.workspaceID, Title: strings.TrimSpace(input.Title), Description: normalizeOptionalText(input.Description),
		Status: task.StatusPending, Entry: now, Modified: now,
		Due: input.Due, Priority: input.Priority, Tags: input.Tags,
		Assignees: assignees, Depends: depends,
		Wait: input.Wait, Scheduled: input.Scheduled, Until: input.Until, Recur: input.Recur,
	}
	projChange, err := s.applyProjectBinding(&tsk, input.Project)
```

改为：
```go
	tsk := task.Task{
		UUID: uuid.NewString(), WorkspaceID: s.workspaceID, Title: strings.TrimSpace(input.Title), Description: normalizeOptionalText(input.Description),
		Status: task.StatusPending, Entry: now, Modified: now,
		Due: input.Due, Priority: input.Priority, Tags: input.Tags,
		Assignees: assignees, Depends: depends,
		Wait: input.Wait, Scheduled: input.Scheduled, Until: input.Until, Recur: input.Recur,
		Parent: parentUUID,
	}
	projChange, err := s.applyProjectBinding(&tsk, parentProject)
```

注意 `applyProjectBinding` 接受 `*string`，传入 `parentProject`（即「子任务最终应使用的 project ref」）。

- [ ] **Step 5: 新增 resolveParentForAdd（基础版）**

在 `internal/app/service.go` 中 `resolveDependencyTargets`（约 1814 行）附近新增：

```go
// resolveParentForAdd 解析创建子任务时的 parent 引用，返回 (parentUUID 指针, 子任务应使用的 project ref)。
// 当 parent 为空时返回 (nil, inputProject)，行为与原 addLocked 一致。
// 拒绝分支在 Task 2 中补齐。
func (s *Service) resolveParentForAdd(parentRef *string, inputProject *string) (*string, *string, error) {
	if parentRef == nil || strings.TrimSpace(*parentRef) == "" {
		return nil, inputProject, nil
	}
	parent, err := s.resolveTargetForWrite(*parentRef)
	if err != nil {
		return nil, nil, RuntimeError{Code: "task_invalid_parent", Message: "parent task not found"}
	}
	// 子任务 project 继承父任务 project；显式 input project 的一致性在 applyProjectBinding 之后由后续 Task 校验。
	project := parent.Project
	if project == nil || strings.TrimSpace(*project) == "" {
		// 父任务无 project：沿用调用方传入的 project（可为 nil）。
		return &parent.UUID, inputProject, nil
	}
	return &parent.UUID, project, nil
}
```

- [ ] **Step 6: 运行测试验证通过**

Run: `go test ./internal/app/ -run TestServiceAddWithParent -v`
Expected: PASS。

- [ ] **Step 7: 提交**

```bash
git add internal/app/service.go internal/app/service_test.go
git commit -m "feat: app.AddInput 增加 parent 字段并支持 project 继承"
```

---

## Task 2: 后端 parent 校验（拒绝分支）

**Files:**
- Modify: `internal/app/service.go`（`resolveParentForAdd`）
- Test: `internal/app/service_test.go`

spec §8.2 要求的拒绝分支：parent 不存在/跨 workspace、自引用、环、deleted parent、completed parent、recurring parent；以及显式 project 与父 project 不一致。

- [ ] **Step 1: 写失败测试 — 覆盖所有拒绝分支**

在 `internal/app/service_test.go` 追加表驱动测试。

**先确认已有 helper**（不要重复定义）：
```bash
grep -n "func strptr\|func runtimeErrorCode" internal/app/service_test.go internal/app/*_test.go
```
`strptr` 已在 `service_test.go:21` 定义（`func strptr(v string) *string { return &v }`）。`runtimeErrorCode` 不存在，本测试内联用 `errors.As`。若文件未 import `"errors"`，补上。

```go
func TestServiceAddWithParentRejections(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	// 建项目（AddInput.Project 需要已存在的 project slug）。
	project, err := svc.AddProject(AddProjectInput{Slug: "demo", Name: "Demo"})
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}

	// 正常父任务（在项目内）。
	normal, err := svc.Add(AddInput{Title: "normal", Project: &project.Slug})
	if err != nil {
		t.Fatalf("create normal: %v", err)
	}

	// deleted 父任务。
	deletedTask, err := svc.Add(AddInput{Title: "to-delete", Project: &project.Slug})
	if err != nil {
		t.Fatalf("create to-delete: %v", err)
	}
	if err := svc.Delete(deletedTask.UUID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// completed 父任务。
	completedTask, err := svc.Add(AddInput{Title: "to-complete", Project: &project.Slug})
	if err != nil {
		t.Fatalf("create to-complete: %v", err)
	}
	if err := svc.Done(completedTask.UUID); err != nil {
		t.Fatalf("done: %v", err)
	}

	// recurring 父任务（需要 Due + Recur + project）。
	due := int64(200)
	recur := "weekly"
	recurringParent, err := svc.Add(AddInput{
		Title: "recurring", Project: &project.Slug, Due: &due, Recur: &recur,
	})
	if err != nil {
		t.Fatalf("create recurring parent: %v", err)
	}

	cases := []struct {
		name    string
		input   AddInput
		wantErr string // 空表示期望成功
	}{
		{name: "parent not found", input: AddInput{Title: "x", Parent: strptr("nonexistent-uuid")}, wantErr: "task_invalid_parent"},
		{name: "deleted parent", input: AddInput{Title: "x", Parent: &deletedTask.UUID}, wantErr: "task_parent_deleted"},
		{name: "completed parent", input: AddInput{Title: "x", Parent: &completedTask.UUID}, wantErr: "task_parent_completed"},
		{name: "recurring parent", input: AddInput{Title: "x", Parent: &recurringParent.UUID}, wantErr: "task_parent_recurring"},
		{name: "project mismatch", input: AddInput{Title: "x", Parent: &normal.UUID, Project: strptr("other-project")}, wantErr: "task_invalid_parent"},
		{name: "valid child inherits project", input: AddInput{Title: "ok child", Parent: &normal.UUID}, wantErr: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			created, err := svc.Add(tc.input)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if created.Parent == nil || *created.Parent != *tc.input.Parent {
					t.Fatalf("created.Parent = %v, want %s", created.Parent, *tc.input.Parent)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error code %s, got nil", tc.wantErr)
			}
			var re RuntimeError
			if !errors.As(err, &re) || re.Code != tc.wantErr {
				t.Fatalf("error = %v, want code %q", err, tc.wantErr)
			}
		})
	}
}
```

说明：自引用/环在 service 层无法构造（新任务 UUID 创建时才生成，不可能指向自己；parent 是树形单父关系，新建任务不可能是已存在任务的祖先）。因此本测试不覆盖自引用环，与 `task.Validate()` 现状一致。如未来开放 Modify parent，再补环检测。

- [ ] **Step 2: 补全 resolveParentForAdd 拒绝分支**

替换 Task 1 Step 5 的 `resolveParentForAdd`：

```go
// resolveParentForAdd 解析创建子任务时的 parent 引用。
// 返回 (parentUUID 指针, 子任务应使用的 project ref)。
// 拒绝：parent 不存在/跨 workspace、自引用、环、deleted/completed/recurring parent、project 不一致。
func (s *Service) resolveParentForAdd(parentRef *string, inputProject *string) (*string, *string, error) {
	if parentRef == nil || strings.TrimSpace(*parentRef) == "" {
		return nil, inputProject, nil
	}
	parent, err := s.resolveTargetForWrite(*parentRef)
	if err != nil {
		return nil, nil, RuntimeError{Code: "task_invalid_parent", Message: "parent task not found"}
	}
	switch parent.Status {
	case task.StatusDeleted:
		return nil, nil, RuntimeError{Code: "task_parent_deleted", Message: "parent task is deleted"}
	case task.StatusCompleted:
		// 完成任务下继续拆任务容易造成状态语义混乱；文案给出替代动作（spec §8.2 规则 6）。
		return nil, nil, RuntimeError{Code: "task_parent_completed", Message: "parent task is completed; please reopen the parent before adding sub-tasks"}
	case task.StatusRecurring:
		// recurring parent 不允许手动子任务（spec §8.1 / §9.1）。
		return nil, nil, RuntimeError{Code: "task_parent_recurring", Message: "manual sub-tasks cannot be added to a recurring parent"}
	}

	// 显式 project 必须与父任务 project 一致（spec §8.2 规则 3）。
	if inputProject != nil && strings.TrimSpace(*inputProject) != "" &&
		parent.Project != nil && strings.TrimSpace(*parent.Project) != "" &&
		*inputProject != *parent.Project {
		return nil, nil, RuntimeError{Code: "task_invalid_parent", Message: "sub-task project must match parent project"}
	}

	// 环检测：新子任务的 parent 不能是「以新任务为祖先」的任务。
	// 由于新任务 UUID 此时尚未确定，无法做严格的「新任务作为父」环检测；
	// 但 parent 字段语义是树形（单父），天然无环，除非存在历史脏数据。
	// 这里做祖先链回溯：若 parent 的祖先链中已存在「指向当前正在创建任务」的引用，则脏数据情况下拒绝。
	// 新建场景下 parent 链不可能包含未创建的 UUID，故此处是安全网，留空实现（Task 不强制）。

	project := parent.Project
	if project == nil || strings.TrimSpace(*project) == "" {
		return &parent.UUID, inputProject, nil
	}
	return &parent.UUID, project, nil
}
```

说明：
- `resolveTargetForWrite` 已做 workspace + project scope 校验（跨 workspace 会 `task_not_found`），此处统一转成 `task_invalid_parent`。
- 环检测：parent 关系是树形（每个任务单父），新建任务的 UUID 尚未存在，不可能形成「新任务是自己祖先」的环；唯一风险是脏数据，此处不额外做 DFS（与 `task.Validate()` 现状一致，不为脏数据过度设计）。
- 自引用：`resolveTargetForWrite(parentRef)` 解析的是已存在任务，新任务 UUID 还没生成，无法自引用。

- [ ] **Step 3: 运行测试验证通过**

Run: `go test ./internal/app/ -run TestServiceAddWithParentRejections -v`
Expected: PASS（self reference 用例 skip）。

- [ ] **Step 4: 运行全量 app 测试确认无回归**

Run: `go test ./internal/app/... -count=1`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/app/service.go internal/app/service_test.go
git commit -m "feat: app.Add 校验 parent 状态与 project 一致性"
```

---

## Task 3: 后端 HTTP 层 — addTaskRequest 加 parent

**Files:**
- Modify: `internal/httpapi/tasks.go:20-39`（`addTaskRequest`）、`internal/httpapi/tasks.go:333-347`（`handleTaskAdd` 映射）
- Test: `internal/httpapi/tasks_test.go`

- [ ] **Step 1: 写失败测试 — HTTP 提交 parent**

在 `internal/httpapi/tasks_test.go` 追加（参照 `TestTaskHTTPAcceptsTaskSlugRefs` 的 `newHTTPServerWithTokenFixture` + `requestHTTP` 模式）：

```go
func TestTaskHTTPAddWithParent(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:write")
	authHeader := map[string]string{"Authorization": "Bearer " + fixture.token}

	// 先建一个父任务（带 body 必须用 requestHTTPBody，不是 requestHTTP）
	parentRR := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/tasks", `{"title":"parent task"}`, authHeader)
	if parentRR.Code != http.StatusCreated {
		t.Fatalf("create parent status = %d, body = %s", parentRR.Code, parentRR.Body.String())
	}
	// 成功响应是 {data, meta} envelope，data 里才是 task JSON。
	var parentEnvelope struct {
		Data struct {
			UUID string `json:"uuid"`
		} `json:"data"`
	}
	if err := json.Unmarshal(parentRR.Body.Bytes(), &parentEnvelope); err != nil {
		t.Fatalf("decode parent: %v", err)
	}

	// 建子任务，带 parent
	childBody := fmt.Sprintf(`{"title":"child task","parent":%q}`, parentEnvelope.Data.UUID)
	childRR := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/tasks", childBody, authHeader)
	if childRR.Code != http.StatusCreated {
		t.Fatalf("create child status = %d, body = %s", childRR.Code, childRR.Body.String())
	}
	var childEnvelope struct {
		Data struct {
			UUID   string  `json:"uuid"`
			Parent *string `json:"parent"`
		} `json:"data"`
	}
	if err := json.Unmarshal(childRR.Body.Bytes(), &childEnvelope); err != nil {
		t.Fatalf("decode child: %v", err)
	}
	if childEnvelope.Data.Parent == nil || *childEnvelope.Data.Parent != parentEnvelope.Data.UUID {
		t.Fatalf("child.parent = %v, want %s", childEnvelope.Data.Parent, parentEnvelope.Data.UUID)
	}
}
```

**重要：** 带 body 的请求必须用 `requestHTTPBody`（`auth_test.go:58`），`requestHTTP` 不接受 body。成功响应是 `successEnvelope{Data, Meta}`（`envelope.go:26`），task 字段在 `.data` 下。

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./internal/httpapi/ -run TestTaskHTTPAddWithParent -v`
Expected: FAIL — parent 字段被忽略（childResp.Parent 为 nil）。

- [ ] **Step 3: addTaskRequest 加 Parent 字段**

修改 `internal/httpapi/tasks.go:20-39`，在 `UDAs` 后加：

```go
type addTaskRequest struct {
	Title         string            `json:"title"`
	Description   *string           `json:"description,omitempty"`
	Project       string            `json:"project,omitempty"`
	ProjectID     string            `json:"project_id,omitempty"`
	Priority      string            `json:"priority,omitempty"`
	Due           *int64            `json:"due,omitempty"`
	DueDate       string            `json:"due_date,omitempty"`
	Assignees     []string          `json:"assignees,omitempty"`
	Depends       []string          `json:"depends,omitempty"`
	Wait          *int64            `json:"wait,omitempty"`
	WaitDate      string            `json:"wait_date,omitempty"`
	Scheduled     *int64            `json:"scheduled,omitempty"`
	ScheduledDate string            `json:"scheduled_date,omitempty"`
	Until         *int64            `json:"until,omitempty"`
	UntilDate     string            `json:"until_date,omitempty"`
	Recur         *string           `json:"recur,omitempty"`
	Tags          []string          `json:"tags,omitempty"`
	UDAs          map[string]string `json:"udas,omitempty"`
	Parent        string            `json:"parent,omitempty"`
}
```

- [ ] **Step 4: handleTaskAdd 映射 Parent**

修改 `internal/httpapi/tasks.go:333-347` 的 `scoped.Add(app.AddInput{...})` 调用，在 `UDAs: req.UDAs,` 后加：

```go
	created, err := scoped.Add(app.AddInput{
		Title:       strings.TrimSpace(req.Title),
		Description: req.Description,
		Project:     projectPtr,
		Priority:    priority,
		Due:         due,
		Assignees:   req.Assignees,
		Depends:     req.Depends,
		Wait:        wait,
		Scheduled:   scheduled,
		Until:       until,
		Recur:       req.Recur,
		Tags:        req.Tags,
		UDAs:        req.UDAs,
		Parent:      stringPtrIfPresent(req.Parent),
	})
```

在 `tasks.go` 末尾（其他 helper 附近）加 helper：

```go
func stringPtrIfPresent(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
```

- [ ] **Step 5: 运行测试验证通过**

Run: `go test ./internal/httpapi/ -run TestTaskHTTPAddWithParent -v`
Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add internal/httpapi/tasks.go internal/httpapi/tasks_test.go
git commit -m "feat: HTTP 创建任务支持 parent 字段"
```

---

## Task 4: 后端 children 读取端点

**Files:**
- Modify: `internal/app/service.go`（新增 `ListChildren`）
- Modify: `internal/httpapi/tasks.go`（新增 `handleTaskChildren`）
- Modify: `internal/httpapi/huma_routes.go:191`（注册路由）
- Test: `internal/httpapi/tasks_test.go`

spec §8.3：专用 `/children` 端点，支持 `include_closed` 控制 completed/deleted 可见性，避免 `parent:<uuid>` query 的状态陷阱（spec §8.3 / service.go:494-497）。

- [ ] **Step 1: 写失败测试 — children 端点返回子任务**

在 `internal/httpapi/tasks_test.go` 追加：

```go
func TestTaskHTTPChildren(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:write")
	authHeader := map[string]string{"Authorization": "Bearer " + fixture.token}

	// 建父任务（带 body 用 requestHTTPBody）
	parentRR := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/tasks", `{"title":"parent"}`, authHeader)
	var parentEnv struct {
		Data struct {
			UUID string `json:"uuid"`
		} `json:"data"`
	}
	json.Unmarshal(parentRR.Body.Bytes(), &parentEnv)

	// 建两个子任务（一个 pending，一个 done）
	requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/tasks", fmt.Sprintf(`{"title":"open child","parent":%q}`, parentEnv.Data.UUID), authHeader)
	doneChildRR := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/tasks", fmt.Sprintf(`{"title":"done child","parent":%q}`, parentEnv.Data.UUID), authHeader)
	var doneEnv struct {
		Data struct {
			UUID string `json:"uuid"`
		} `json:"data"`
	}
	json.Unmarshal(doneChildRR.Body.Bytes(), &doneEnv)
	// done 路径用 POST 无 body，可用 requestHTTP
	requestHTTP(t, fixture.server, http.MethodPost, "/api/v1/tasks/"+doneEnv.Data.UUID+"/done", authHeader)

	// 默认 include_closed=false：只返回 open（GET 用 requestHTTP，query 拼在 path）
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/tasks/"+parentEnv.Data.UUID+"/children", authHeader)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var defaultResp struct {
		Data []struct {
			Title  string `json:"title"`
			Status string `json:"status"`
		} `json:"data"`
	}
	json.Unmarshal(rr.Body.Bytes(), &defaultResp)
	if len(defaultResp.Data) != 1 || defaultResp.Data[0].Title != "open child" {
		t.Fatalf("default children = %#v, want only open child", defaultResp.Data)
	}

	// include_closed=true：返回全部
	rrAll := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/tasks/"+parentEnv.Data.UUID+"/children?include_closed=true", authHeader)
	var allResp struct {
		Data []struct {
			Title  string `json:"title"`
			Status string `json:"status"`
		} `json:"data"`
	}
	json.Unmarshal(rrAll.Body.Bytes(), &allResp)
	if len(allResp.Data) != 2 {
		t.Fatalf("include_closed children count = %d, want 2", len(allResp.Data))
	}
}
```

**重要：** 成功响应是 `{data, meta}` envelope（`envelope.go:26`），列表在 `.data` 下；带 body 用 `requestHTTPBody`，GET/无 body 用 `requestHTTP`。query string 直接拼在 path 里即可。

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./internal/httpapi/ -run TestTaskHTTPChildren -v`
Expected: FAIL — 404（路由未注册）。

- [ ] **Step 3: app.Service 新增 ListChildren**

在 `internal/app/service.go` 中 `Info` 方法（约 549 行）附近新增：

```go
// ListChildren 列出指定任务的直接子任务。
// include_closed=false 时过滤 completed/deleted（spec §8.3）。
func (s *Service) ListChildren(target string, includeClosed bool) ([]task.Task, error) {
	if err := s.Require(PermissionTaskRead); err != nil {
		return nil, err
	}
	parent, err := s.ResolveProtocolTarget(target)
	if err != nil {
		return nil, err
	}
	children, err := s.repo.Children(s.workspaceID, parent.UUID)
	if err != nil {
		return nil, err
	}
	if includeClosed {
		return children, nil
	}
	out := children[:0]
	for _, c := range children {
		if c.Status == task.StatusCompleted || c.Status == task.StatusDeleted {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}
```

说明：复用现有 `repo.Children`（task_repo.go:420，已按 workspace + parent 过滤、按 entry 排序、预加载 assignees/links）。过滤在内存做（子任务量级小，不引入额外查询路径）。

- [ ] **Step 4: 新增 handleTaskChildren**

在 `internal/httpapi/tasks.go` 中 `handleTaskLinkList`（约 655 行）附近新增（仿其结构）：

```go
func (s *Server) handleTaskChildren(w http.ResponseWriter, r *http.Request) {
	taskRef, ok := requireTaskRef(w, r)
	if !ok {
		return
	}
	scoped, _, err := s.scopedService(r, auth.ScopeTaskRead, app.PermissionTaskRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	includeClosed := isTruthyQueryValue(r.URL.Query().Get("include_closed"))
	children, err := scoped.ListChildren(taskRef, includeClosed)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, tasksToJSON(children), nil)
}
```

确认 `isTruthyQueryValue` 存在（spec 探索报告 tasks.go:281-284 提到）；`tasksToJSON` 在 tasks.go:861-867 已存在，序列化每个任务为 `task.ToJSON`。

- [ ] **Step 5: 注册路由**

修改 `internal/httpapi/huma_routes.go:191`（urgency 那行之后）加一行：

```go
		{Method: http.MethodGet, Path: "/api/v1/tasks/{taskRef}/urgency", Tag: "Tasks", Summary: "Explain task urgency.", Handler: s.handleTaskUrgency},
		{Method: http.MethodGet, Path: "/api/v1/tasks/{taskRef}/children", Tag: "Tasks", Summary: "List child tasks of a task.", Handler: s.handleTaskChildren},
```

- [ ] **Step 6: 运行测试验证通过**

Run: `go test ./internal/httpapi/ -run TestTaskHTTPChildren -v`
Expected: PASS。

- [ ] **Step 7: 把 children 端点加入 task_slug ref 兼容测试**

在 `TestTaskHTTPAcceptsTaskSlugRefs`（tasks_test.go:20）的 cases 切片中追加：

```go
		{name: "children", method: http.MethodGet, path: "/api/v1/tasks/api-1/children", wantStatus: http.StatusOK},
```

- [ ] **Step 8: 运行全量后端测试**

Run: `go test ./internal/... -count=1`
Expected: PASS。

- [ ] **Step 9: 提交**

```bash
git add internal/app/service.go internal/httpapi/tasks.go internal/httpapi/huma_routes.go internal/httpapi/tasks_test.go
git commit -m "feat: 新增 GET /tasks/{ref}/children 子任务读取端点"
```

---

## Task 5: 前端 task-api 加 parent + listTaskChildren

**Files:**
- Modify: `web/src/features/workspace/project-workbench/api/task-api.ts`

- [ ] **Step 1: TaskCreateInput 加 parent**

修改 `web/src/features/workspace/project-workbench/api/task-api.ts:16-35`，在 `udas?` 后加：

```ts
export type TaskCreateInput = {
  title: string
  description?: string | null
  project?: string
  project_id?: string
  priority?: string
  due?: number | null
  due_date?: string
  assignees?: string[]
  depends?: string[]
  wait?: number | null
  wait_date?: string
  scheduled?: number | null
  scheduled_date?: string
  until?: number | null
  until_date?: string
  recur?: string | null
  tags?: string[]
  udas?: Record<string, string>
  parent?: string
}
```

- [ ] **Step 2: 新增 listTaskChildren**

在 `task-api.ts` 中 `getTask`（约 279 行）附近新增：

```ts
export function listTaskChildren(
  workspaceSlug: string,
  taskRef: string,
  includeClosed = false
): Promise<ProjectTask[]> {
  const params = new URLSearchParams()
  params.set("workspace", workspaceSlug)
  if (includeClosed) {
    params.set("include_closed", "true")
  }
  return workspaceApiGet<ProjectTask[]>(
    `/api/v1/tasks/${encodeSegment(taskRef)}/children?${params.toString()}`
  )
}
```

确认 `workspaceApiGet` 与 `encodeSegment` 的导出（二者已在文件内使用，见 `getTask` 实现）。`workspaceApiGet` 返回的可能是 envelope 解包后的 data；若该 helper 返回 `{ data }`，需取 `.data`——参照 `getTask` 的写法保持一致。

- [ ] **Step 3: typecheck 验证**

Run: `pnpm --dir web typecheck`
Expected: 通过。

- [ ] **Step 4: 提交**

```bash
git add web/src/features/workspace/project-workbench/api/task-api.ts
git commit -m "feat: 前端 task-api 增加 parent 与 listTaskChildren"
```

---

## Task 6: 前端 hooks — children query 与 createSubTask mutation

**Files:**
- Modify: `web/src/features/workspace/project-workbench/hooks/use-task-detail-data.ts`
- Modify: `web/src/features/workspace/project-workbench/hooks/use-task-mutations.ts`

- [ ] **Step 1: use-task-detail-data 增加 children query key 与 hook**

修改 `web/src/features/workspace/project-workbench/hooks/use-task-detail-data.ts`，在 `taskQueryKeys` 中加 `children`，并新增 `useTaskChildrenQuery`：

```ts
import { useQuery } from "@tanstack/react-query"

import {
  getTask,
  listTaskChildren,
  type ProjectTask,
} from "../api/task-api"

export const taskQueryKeys = {
  task: (workspaceSlug: string, taskRef: string) =>
    ["task", workspaceSlug, taskRef] as const,
  children: (workspaceSlug: string, taskRef: string, includeClosed: boolean) =>
    ["task", workspaceSlug, taskRef, "children", includeClosed ? "all" : "open"] as const,
}

export function useTaskDetailQuery(
  workspaceSlug: string,
  taskRef: string,
  initialData?: ProjectTask
) {
  return useQuery({
    queryKey: taskQueryKeys.task(workspaceSlug, taskRef),
    queryFn: () => getTask(workspaceSlug, taskRef),
    enabled: workspaceSlug.length > 0 && taskRef.length > 0,
    initialData,
  })
}

export function useTaskChildrenQuery(
  workspaceSlug: string,
  taskRef: string,
  includeClosed: boolean
) {
  return useQuery<ProjectTask[]>({
    queryKey: taskQueryKeys.children(workspaceSlug, taskRef, includeClosed),
    queryFn: () => listTaskChildren(workspaceSlug, taskRef, includeClosed),
    enabled: workspaceSlug.length > 0 && taskRef.length > 0,
  })
}
```

- [ ] **Step 2: use-task-mutations 增加 useCreateSubTaskMutation**

在 `web/src/features/workspace/project-workbench/hooks/use-task-mutations.ts` 中 `useCreateTaskMutation`（约 74-96 行）之后新增。子任务创建成功后需刷新：父任务详情、父任务 children、项目任务列表前缀。

```ts
export function useCreateSubTaskMutation(
  workspaceSlug: string,
  projectSlug: string,
  parentRef: string
) {
  const queryClient = useQueryClient()
  const feedback = useEditFeedback()
  return useMutation({
    mutationFn: (input: TaskCreateInput) => createTask(workspaceSlug, input),
    onSuccess: () => {
      // 父任务详情（刷新 children 计数等）
      void queryClient.invalidateQueries({
        queryKey: taskQueryKeys.task(workspaceSlug, parentRef),
      })
      // 子任务列表（open 与 closed 两个 key 都失效）
      void queryClient.invalidateQueries({
        queryKey: ["task", workspaceSlug, parentRef, "children"],
      })
      // 项目任务列表（子任务也会出现在项目列表里）
      void queryClient.invalidateQueries({
        queryKey: projectQueryKeys.projectTasksPrefix(workspaceSlug, projectSlug),
      })
      feedback.success("已创建：子任务")
    },
  })
}
```

注意 `taskQueryKeys` 需从 `use-task-detail-data` 导入（确认该文件顶部 import）。`projectQueryKeys` 已在该文件 import。

- [ ] **Step 3: typecheck 验证**

Run: `pnpm --dir web typecheck`
Expected: 通过。

- [ ] **Step 4: 提交**

```bash
git add web/src/features/workspace/project-workbench/hooks/use-task-detail-data.ts web/src/features/workspace/project-workbench/hooks/use-task-mutations.ts
git commit -m "feat: 新增子任务读取 query 与创建 mutation"
```

---

## Task 7: 前端 SubTaskList 组件（列表 + 就地 composer）

**Files:**
- Create: `web/src/features/workspace/project-workbench/task-detail/sub-task-list.tsx`

这是阶段一核心交互组件。spec §9.2：标题 Enter 直接提交并保持 composer 打开（连续创建）；描述用 Cmd/Ctrl+Enter。spec §9.3：行内展示 status/slug/title/priority/assignee/due。

- [ ] **Step 1: 新建 sub-task-list.tsx**

创建 `web/src/features/workspace/project-workbench/task-detail/sub-task-list.tsx`：

```tsx
import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { MarkdownEditor } from "@/components/markdown"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { ApiError } from "@/lib/api"
import { taskStatusLabel } from "@/features/workspace/shared/task-labels"
import type { ProjectTask } from "../api/task-api"
import { useCreateSubTaskMutation } from "../hooks/use-task-mutations"
import { useTaskChildrenQuery } from "../hooks/use-task-detail-data"
import { InlineDatePicker } from "../shared/inline-date-picker"
import { AssigneePicker } from "./assignee-picker"
import { TagPicker } from "./tag-picker"

const PRIORITIES = ["H", "M", "L"] as const

type SubTaskListProps = {
  canCreate: boolean
  parentRef: string
  parentUUID: string
  projectSlug: string
  workspaceSlug: string
}

export function SubTaskList({
  canCreate,
  parentRef,
  parentUUID,
  projectSlug,
  workspaceSlug,
}: SubTaskListProps) {
  const { t } = useTranslation()
  const [showCompleted, setShowCompleted] = useState(false)

  const openChildren = useTaskChildrenQuery(workspaceSlug, parentRef, false)
  const allChildren = useTaskChildrenQuery(workspaceSlug, parentRef, true)

  // 列表展示：默认 open；展开已完成时用 allChildren。
  const children = showCompleted ? allChildren.data : openChildren.data
  const isLoading = showCompleted ? allChildren.isPending : openChildren.isPending
  const error = showCompleted ? allChildren.error : openChildren.error

  // 计算已完成数量（用于折叠入口文案）。
  const openCount = openChildren.data?.length ?? 0
  const totalCount = allChildren.data?.length ?? 0
  const completedCount = totalCount - openCount

  return (
    <section className="space-y-3 border bg-card p-4">
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-sm font-medium">{t("taskDetail.subTasks")}</h2>
      </div>

      {canCreate ? (
        <SubTaskComposer
          parentRef={parentRef}
          parentUUID={parentUUID}
          projectSlug={projectSlug}
          workspaceSlug={workspaceSlug}
        />
      ) : null}

      {isLoading ? (
        <div className="space-y-2">
          {Array.from({ length: 2 }).map((_, i) => (
            <Skeleton className="h-8" key={i} />
          ))}
        </div>
      ) : error ? (
        <div className="space-y-2 text-sm text-destructive">
          <p>{t("taskDetail.subTasksError")}</p>
          <Button
            onClick={() =>
              showCompleted ? allChildren.refetch() : openChildren.refetch()
            }
            size="sm"
            variant="outline"
          >
            {t("common.retry")}
          </Button>
        </div>
      ) : !children || children.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {canCreate ? t("taskDetail.subTasksEmpty") : t("taskDetail.subTasksNone")}
        </p>
      ) : (
        <ul className="space-y-1">
          {children.map((child) => (
            <SubTaskRow
              key={child.uuid}
              projectSlug={projectSlug}
              task={child}
              workspaceSlug={workspaceSlug}
            />
          ))}
        </ul>
      )}

      {completedCount > 0 ? (
        <Button
          onClick={() => setShowCompleted((v) => !v)}
          size="sm"
          variant="ghost"
        >
          {showCompleted
            ? t("taskDetail.hideCompleted")
            : t("taskDetail.showCompleted", { count: completedCount })}
        </Button>
      ) : null}
    </section>
  )
}

function SubTaskRow({
  projectSlug,
  task,
  workspaceSlug,
}: {
  projectSlug: string
  task: ProjectTask
  workspaceSlug: string
}) {
  const { t } = useTranslation()
  const href = `/workspaces/${workspaceSlug}/projects/${projectSlug}/tasks/${task.task_slug || task.uuid}`
  return (
    <li className="flex items-center gap-2 rounded px-2 py-1.5 text-sm hover:bg-muted/50">
      <Badge variant="outline">{taskStatusLabel(task.status, t)}</Badge>
      <a className="min-w-0 flex-1 truncate hover:underline" href={href}>
        {task.title}
      </a>
      {task.priority ? <Badge variant="secondary">{task.priority}</Badge> : null}
      {task.assignees && task.assignees.length > 0 ? (
        <span className="truncate text-xs text-muted-foreground">
          {task.assignees.map((a) => a.name ?? a.user_id).join(", ")}
        </span>
      ) : null}
      {task.due ? (
        <span className="text-xs text-muted-foreground">{formatDue(task.due)}</span>
      ) : null}
    </li>
  )
}

function formatDue(due: string | number): string {
  // 简化展示：优先用 locale 日期。due 可能是 unix 秒（number）或 ISO（string）。
  const ms = typeof due === "number" ? due * 1000 : Date.parse(due)
  if (!Number.isFinite(ms)) return String(due)
  return new Date(ms).toLocaleDateString()
}

function SubTaskComposer({
  parentRef,
  parentUUID,
  projectSlug,
  workspaceSlug,
}: {
  parentRef: string
  parentUUID: string
  projectSlug: string
  workspaceSlug: string
}) {
  const { t } = useTranslation()
  const createSubTask = useCreateSubTaskMutation(
    workspaceSlug,
    projectSlug,
    parentRef
  )
  const [title, setTitle] = useState("")
  const [description, setDescription] = useState("")
  const [priority, setPriority] = useState("")
  const [due, setDue] = useState<number | null>(null)
  const [assignees, setAssignees] = useState<string[]>([])
  const [tags, setTags] = useState<string[]>([])
  const [error, setError] = useState<string | null>(null)

  // 连续创建：提交后清空标题与临时字段，但保持 composer 打开（spec §9.2）。
  const resetFields = () => {
    setTitle("")
    setDescription("")
    setPriority("")
    setDue(null)
    setAssignees([])
    setTags([])
  }

  const submit = async (keepOpen: boolean) => {
    const normalizedTitle = title.trim()
    if (!normalizedTitle) {
      setError(t("taskDetail.subTaskTitleRequired"))
      return
    }
    setError(null)
    try {
      await createSubTask.mutateAsync({
        title: normalizedTitle,
        parent: parentUUID,
        project: projectSlug,
        ...(description.trim() ? { description: description.trim() } : {}),
        ...(priority ? { priority } : {}),
        ...(due !== null ? { due } : {}),
        ...(assignees.length > 0 ? { assignees } : {}),
        ...(tags.length > 0 ? { tags } : {}),
      })
      if (keepOpen) {
        resetFields()
      }
    } catch (err) {
      const message =
        err instanceof ApiError ? err.message : err instanceof Error ? err.message : String(err)
      setError(message)
      // 失败时保留草稿（spec §10）。
    }
  }

  return (
    <div className="space-y-3 rounded border p-3">
      <Input
        aria-label={t("taskDetail.subTaskTitle")}
        autoFocus
        onChange={(e) => {
          setTitle(e.target.value)
          setError(null)
        }}
        onKeyDown={(e) => {
          // 标题输入框 Enter 直接提交并保持 composer 打开（连续创建）。
          if (e.key === "Enter" && !e.shiftKey) {
            e.preventDefault()
            void submit(true)
          }
        }}
        placeholder={t("taskDetail.subTaskTitlePlaceholder")}
        value={title}
      />
      <MarkdownEditor
        ariaLabel={t("taskDetail.subTaskDescription")}
        minHeight={120}
        onChange={setDescription}
        onModEnter={() => {
          void submit(true)
        }}
        placeholder={t("taskDetail.subTaskDescriptionPlaceholder")}
        value={description}
      />
      <p className="text-xs text-muted-foreground">
        {t("taskDetail.subTaskInheritProject", { project: projectSlug })}
      </p>
      <div className="flex flex-wrap items-center gap-2">
        <Select onValueChange={setPriority} value={priority}>
          <SelectTrigger aria-label={t("taskDetail.subTaskPriority")} className="w-28">
            <SelectValue placeholder={t("taskDetail.subTaskPriority")} />
          </SelectTrigger>
          <SelectContent>
            {PRIORITIES.map((item) => (
              <SelectItem key={item} value={item}>
                {item}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <InlineDatePicker
          ariaLabel={t("taskDetail.subTaskDue")}
          boundary="end"
          emptyLabel={t("taskDetail.subTaskDue")}
          onSave={setDue}
          value={due}
        />
        <AssigneePicker
          onSave={async (next) => setAssignees(next)}
          value={assignees.map((id) => ({ user_id: id, name: id })) as never}
          workspaceSlug={workspaceSlug}
        />
        <TagPicker
          onSave={async (next) => setTags(next)}
          projectSlug={projectSlug}
          value={tags}
          workspaceSlug={workspaceSlug}
        />
        <Button
          disabled={createSubTask.isPending}
          onClick={() => void submit(true)}
          size="sm"
          type="button"
        >
          {t("taskDetail.createSubTask")}
        </Button>
      </div>
      {error ? <p className="text-sm text-destructive">{error}</p> : null}
    </div>
  )
}
```

注意：
- `AssigneePicker` 的 `value` 类型是 `ProjectWorkbenchAssignee[]`——上面的 `as never` 是占位，实际应按 `assignee-picker.tsx:21-26` 的真实 props 类型构造。实施时先 `grep "type ProjectWorkbenchAssignee" web/src/features/workspace/project-workbench/api/project-api.ts` 取真实类型，用 `{ user_id, name }[]` 形式。如果类型不匹配，改为受控单选或简化为只填 `assignees`（id 数组）后由父组件维护。
- 翻译 key 全部新增（Task 8 补 locale）。

- [ ] **Step 2: 确认 AssigneePicker/TagPicker 真实 props 并修正类型**

Run:
```bash
grep -n "type ProjectWorkbenchAssignee" web/src/features/workspace/project-workbench/api/project-api.ts
grep -n "ProjectWorkbenchAssignee\|value" web/src/features/workspace/project-workbench/task-detail/assignee-picker.tsx | head
grep -n "value" web/src/features/workspace/project-workbench/task-detail/tag-picker.tsx | head
```
按真实类型调整 `AssigneePicker` 的 `value` 与 `TagPicker` 的 `value`。`AssigneePicker.value` 期望 `ProjectWorkbenchAssignee[]`，但子任务 composer 里只存 id 数组；可改为维护一个 `assigneeObjects` state（`ProjectWorkbenchAssignee[]`），或确认 picker 是否接受只含 `user_id` 的子集。

- [ ] **Step 3: typecheck**

Run: `pnpm --dir web typecheck`
Expected: 通过（Task 8 补 locale 前可能有 missing key 警告，typecheck 一般不报 i18n key，应能通过）。

- [ ] **Step 4: 提交**

```bash
git add web/src/features/workspace/project-workbench/task-detail/sub-task-list.tsx
git commit -m "feat: 新增子任务列表与就地 composer 组件"
```

---

## Task 8: 前端挂载 SubTaskList + 新增文案 + 权限门控

**Files:**
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.tsx`
- Modify: `web/src/locales/zh-CN.ts`
- Modify: `web/src/locales/en-US.ts`
- Test: `web/src/features/workspace/project-workbench/task-detail/__tests__/sub-task-list.test.tsx`（新建）

- [ ] **Step 1: 新增 zh-CN 文案**

在 `web/src/locales/zh-CN.ts` 的 `taskDetail`（约 421-424 行）对象中补全：

```ts
  taskDetail: {
    urgency: "紧迫度",
    urgencyUnavailable: "紧迫度暂不可用",
    subTasks: "子任务",
    subTasksEmpty: "还没有子任务，点击上方创建第一个。",
    subTasksNone: "暂无子任务",
    subTasksError: "子任务加载失败",
    subTaskTitle: "子任务标题",
    subTaskTitlePlaceholder: "子任务标题",
    subTaskTitleRequired: "子任务标题不能为空",
    subTaskDescription: "子任务说明",
    subTaskDescriptionPlaceholder: "补充背景、验收标准或处理说明",
    subTaskInheritProject: "继承项目：{{project}}",
    subTaskPriority: "优先级",
    subTaskDue: "截止日期",
    createSubTask: "创建子任务",
    showCompleted: "显示 {{count}} 个已完成",
    hideCompleted: "隐藏已完成",
    description: "正文",
    editDescription: "编辑正文",
    links: "关联资源",
  },
```

`description`/`editDescription`/`links` 若在 `projectReadonly.*` 已存在，阶段二再统一迁移；此处先放 `taskDetail` 下供阶段二组件使用（避免阶段一改坏现有引用）。

- [ ] **Step 2: 新增 en-US 文案（同步）**

在 `web/src/locales/en-US.ts` 对应 `taskDetail` 中加：

```ts
  taskDetail: {
    urgency: "Urgency",
    urgencyUnavailable: "Urgency unavailable",
    subTasks: "Sub-tasks",
    subTasksEmpty: "No sub-tasks yet. Create the first one above.",
    subTasksNone: "No sub-tasks",
    subTasksError: "Failed to load sub-tasks",
    subTaskTitle: "Sub-task title",
    subTaskTitlePlaceholder: "Sub-task title",
    subTaskTitleRequired: "Sub-task title is required",
    subTaskDescription: "Sub-task description",
    subTaskDescriptionPlaceholder: "Add context, acceptance criteria, or notes",
    subTaskInheritProject: "Inherits project: {{project}}",
    subTaskPriority: "Priority",
    subTaskDue: "Due date",
    createSubTask: "Create sub-task",
    showCompleted: "Show {{count}} completed",
    hideCompleted: "Hide completed",
    description: "Description",
    editDescription: "Edit description",
    links: "Linked resources",
  },
```

- [ ] **Step 3: 在 TaskDetailPage 主区挂载 SubTaskList**

修改 `web/src/features/workspace/project-workbench/task-detail/task-detail-page.tsx`。

在顶部 import 区加：
```tsx
import { SubTaskList } from "./sub-task-list"
```

在 `TaskDetailPageContent` 中（约 199 行 `</section>` 之后、`MobileDetailTabs` 之前），计算子任务创建可用性：

```tsx
  // 子任务创建门控（spec §9.1）：可写 + 任务状态可写 + 非 recurring parent。
  const canCreateSubTask =
    taskWritable && taskData.status !== "recurring"
```

在主叙事区（annotations 那个 `div` 内，`TaskAnnotationsEditor` 之前）插入 `SubTaskList`，使其位于「正文 → 子任务 → 活动」顺序：

将原：
```tsx
          <div className={mobilePanelClass(activeMobileTab, "annotations")}>
            <TaskDescriptionBlock ... />
            <TaskAnnotationsEditor ... />
            <TaskChangeHistory ... />
          </div>
```
改为：
```tsx
          <div className={mobilePanelClass(activeMobileTab, "annotations")}>
            <TaskDescriptionBlock
              canWrite={taskWritable}
              onSave={async (description) => {
                await modifyTask.mutateAsync(
                  description ? { description } : { clear_description: true }
                )
              }}
              value={taskData.description ?? ""}
            />
            <SubTaskList
              canCreate={canCreateSubTask}
              parentRef={taskRef}
              parentUUID={taskData.uuid}
              projectSlug={effectiveProjectSlug ?? ""}
              workspaceSlug={workspaceSlug}
            />
            <TaskAnnotationsEditor
              annotations={taskData.annotations}
              canWrite={taskWritable}
              projectSlug={effectiveProjectSlug ?? ""}
              taskRef={taskRef}
              workspaceSlug={workspaceSlug}
            />
            <TaskChangeHistory
              taskRef={taskRef}
              workspaceSlug={workspaceSlug}
            />
          </div>
```

注意：`SubTaskList` 依赖 `projectSlug`。从全局 `/tasks/:ref` 进入时 `effectiveProjectSlug` 可能为 `undefined`，此时子任务列表的链接会缺失 project 段。处理：`SubTaskList` 内部 `href` 用 `task.project ?? projectSlug` 兜底——已在 `SubTaskRow` 中用 `task.project` 优先；若 `task.project` 也为空，链接回退到 `/workspaces/$ws/tasks/$ref`（无 project 段）。在 `SubTaskRow` 中调整：

```tsx
  const projectSegment = task.project || projectSlug
  const href = projectSegment
    ? `/workspaces/${workspaceSlug}/projects/${projectSegment}/tasks/${task.task_slug || task.uuid}`
    : `/workspaces/${workspaceSlug}/tasks/${task.task_slug || task.uuid}`
```

- [ ] **Step 4: 写组件测试 — composer Enter 连续创建**

新建 `web/src/features/workspace/project-workbench/task-detail/__tests__/sub-task-list.test.tsx`（参照现有 web 测试目录约定，先 `grep -rn "vitest\|@testing-library" web/src/features/workspace/project-workbench/tasks/*.test.*` 找现有测试模式）。

核心断言（spec §11.2）：
- 点击/输入后 composer 展开存在。
- Enter 提交后 composer 清空标题但保持打开；payload 含 `project` 和 `parent`。
- completed/recurring 状态下不显示创建入口。

```tsx
import { describe, expect, it, vi, beforeEach } from "vitest"
import { render, screen, fireEvent, waitFor } from "@testing-library/react"
import { SubTaskList } from "../sub-task-list"
// mock createTask / listTaskChildren / useTranslation 等

// 占位：实际 mock 形态以仓库现有 web 测试为准（先看 my-tasks-table.test.tsx 或现有 detail 测试）。
describe("SubTaskList", () => {
  it("Enter 提交后清空标题并保持 composer 打开", async () => {
    const createFn = vi.fn().mockResolvedValue({ uuid: "child-1" })
    // ...render with mocked useCreateSubTaskMutation
    const input = await screen.findByPlaceholderText("子任务标题")
    fireEvent.change(input, { target: { value: "新子任务" } })
    fireEvent.keyDown(input, { key: "Enter" })
    await waitFor(() => {
      expect(createFn).toHaveBeenCalledWith(expect.objectContaining({
        title: "新子任务",
        parent: "parent-uuid",
        project: "proj",
      }))
    })
    // composer 仍在，标题已清空
    expect((input as HTMLInputElement).value).toBe("")
    expect(screen.getByPlaceholderText("子任务标题")).toBeInTheDocument()
  })
})
```

由于 mock 方式依赖仓库现有测试基础设施（MSW？vi.mock？），实施时先读 `web/src/features/workspace/my-tasks/my-tasks-table.test.tsx`（git status 显示这是新加的测试）或现有 detail 测试，照搬 mock 模式补全。

- [ ] **Step 5: 运行 web 测试**

Run: `pnpm --dir web test`
Expected: 新测试通过，无回归。

- [ ] **Step 6: typecheck + lint + build**

Run: `pnpm --dir web typecheck && pnpm --dir web lint && pnpm --dir web build`
Expected: 全部通过。

- [ ] **Step 7: smoke**

Run: `pnpm --dir web run smoke:editing`
Expected: 通过。

- [ ] **Step 8: 阶段一提交（合并挂载 + 文案 + 测试）**

```bash
git add web/src/features/workspace/project-workbench/task-detail/task-detail-page.tsx web/src/features/workspace/project-workbench/task-detail/sub-task-list.tsx web/src/features/workspace/project-workbench/task-detail/__tests__/sub-task-list.test.tsx web/src/locales/zh-CN.ts web/src/locales/en-US.ts
git commit -m "feat: 任务详情页挂载子任务列表与就地创建 composer"
```

---

# 阶段二：结构重排与 Activity 合并

> 阶段二是纯视觉/结构改动，不触碰后端与子任务能力。提交独立，可单独回退。

## Task 9: 主叙事区顺序重排 + 移动端 tab

**Files:**
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.tsx`

spec §7.1 桌面主区顺序：正文 → 子任务 → 活动。spec §7.3 移动端 tab：正文/子任务/属性/活动。

阶段一已把 `SubTaskList` 放在正文与注解之间。本任务把「注解 + 变更历史」收拢进一个 Activity 视觉容器，并调整移动端 tab。

- [ ] **Step 1: 调整移动端 tab 为四段**

修改 `task-detail-page.tsx` 的 `MobileDetailTab` 类型与 `MobileDetailTabs`：

```tsx
type MobileDetailTab = "description" | "subtasks" | "properties" | "activity"

// MobileDetailTabs.tabs:
const tabs: Array<{ label: string; value: MobileDetailTab }> = [
  { label: t("taskDetail.description"), value: "description" },
  { label: t("taskDetail.subTasks"), value: "subtasks" },
  { label: t("projectReadonly.attributes"), value: "properties" },
  { label: t("taskDetail.activity"), value: "activity" },
]
// TabsList grid-cols-4
```

`taskDetail.activity` 文案在 Task 11 补（先在 locale 加 `"activity": "活动"` / `"Activity"`）。

- [ ] **Step 2: 按移动端 tab 重排主区容器**

将主区三个子区块分别放进对应 `mobilePanelClass` 容器：

```tsx
        <div className="contents md:block md:min-w-0 md:space-y-5">
          <div className={mobilePanelClass(activeMobileTab, "description")}>
            <TaskDescriptionBlock ... />
          </div>
          <div className={mobilePanelClass(activeMobileTab, "subtasks")}>
            <SubTaskList ... />
          </div>
          <div className={mobilePanelClass(activeMobileTab, "activity")}>
            <ActivitySection ... />
          </div>
          <div className={mobilePanelClass(activeMobileTab, "links") /* 保留兼容或合并入 activity */}>
            <TaskLinksEditor ... />
          </div>
        </div>
```

注意：`MobileDetailTab` 现在是四值，原来的 `"links"` 不再是独立 tab。spec §9.5 把 links 归入 `Relations` 右栏分组，但 spec §7.1 又把「添加链接」放在正文附近。决策（与 spec §9.4 一致）：桌面端 `TaskLinksEditor` 保留在正文区域底部作为「关联资源」；移动端把它并入 activity tab 或单独保留——本任务暂并入 activity tab 以减少 tab 数。即把 `<TaskLinksEditor>` 移入 activity 容器顶部。

- [ ] **Step 3: 默认移动端 tab 改为 description**

```tsx
const [activeMobileTab, setActiveMobileTab] =
  useState<MobileDetailTab>("description")
```

- [ ] **Step 4: typecheck + build**

Run: `pnpm --dir web typecheck && pnpm --dir web build`
Expected: 通过。

- [ ] **Step 5: 提交**

```bash
git add web/src/features/workspace/project-workbench/task-detail/task-detail-page.tsx web/src/locales/zh-CN.ts web/src/locales/en-US.ts
git commit -m "refactor: 任务详情页主区按正文/子任务/活动顺序重排并扩展移动端 tab"
```

---

## Task 10: 右侧属性栏分组（Properties/Schedule/Relations/System/Custom）

**Files:**
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-property-panel.tsx`

spec §9.5 分组规则；空组隐身（不渲染标题）。

- [ ] **Step 1: 抽取分组组件 PropertyGroup**

在 `task-property-panel.tsx` 顶部新增可折叠分组容器：

```tsx
import { useState } from "react"
import { ChevronDownIcon, ChevronRightIcon } from "lucide-react"

function PropertyGroup({
  defaultOpen = true,
  title,
  hasContent,
  children,
}: {
  defaultOpen?: boolean
  title: string
  hasContent: boolean
  children: React.ReactNode
}) {
  const [open, setOpen] = useState(defaultOpen)
  if (!hasContent) return null
  return (
    <div className="space-y-2">
      <button
        className="flex w-full items-center gap-1 text-xs font-medium uppercase tracking-wide text-muted-foreground"
        onClick={() => setOpen((v) => !v)}
        type="button"
      >
        {open ? <ChevronDownIcon className="size-3" /> : <ChevronRightIcon className="size-3" />}
        {title}
      </button>
      {open ? <div className="space-y-1">{children}</div> : null}
    </div>
  )
}
```

- [ ] **Step 2: 重排面板字段到分组**

将 `TaskPropertyPanel` 主体改为按分组渲染（保留现有每个 `PropertyRow` 的实现，只改外层分组）。`hasContent` 依据字段是否有值：

```tsx
  const hasSchedule = [task.wait, task.scheduled, task.until, task.recur].some(
    (v) => v !== undefined && v !== null && v !== ""
  )
  const hasRelations =
    !!task.parent || (task.depends && task.depends.length > 0) ||
    (task.blocked_by_info && task.blocked_by_info.length > 0) ||
    (task.links && task.links.length > 0)
  const hasUDA = task.udas && Object.keys(task.udas).length > 0

  return (
    <aside className="space-y-4 border bg-card p-4">
      <PropertyGroup title={t("taskDetail.groupProperties")} hasContent={true}>
        {/* status, urgency, priority, assignees, tags — 现有 PropertyRow */}
      </PropertyGroup>
      <PropertyGroup
        defaultOpen
        title={t("taskDetail.groupSchedule")}
        hasContent={hasSchedule}
      >
        {/* due, wait, scheduled, until, recur */}
      </PropertyGroup>
      <PropertyGroup
        defaultOpen
        title={t("taskDetail.groupRelations")}
        hasContent={hasRelations}
      >
        {/* parent, depends, blocking, links（links 在桌面端也可放这里，与正文处链接编辑器二选一） */}
      </PropertyGroup>
      <PropertyGroup
        defaultOpen={false}
        title={t("taskDetail.groupSystem")}
        hasContent={true}
      >
        {/* task_slug, entry, modified */}
      </PropertyGroup>
      {hasUDA ? (
        <PropertyGroup
          title={t("taskDetail.groupCustom")}
          hasContent={hasUDA}
        >
          {/* UDA 字段 */}
        </PropertyGroup>
      ) : null}
    </aside>
  )
```

文案 key（`groupProperties` 等）在 Task 11 补 locale。

- [ ] **Step 3: typecheck + build**

Run: `pnpm --dir web typecheck && pnpm --dir web build`
Expected: 通过。

- [ ] **Step 4: 提交**

```bash
git add web/src/features/workspace/project-workbench/task-detail/task-property-panel.tsx
git commit -m "refactor: 右侧属性栏按 Properties/Schedule/Relations/System/Custom 分组"
```

---

## Task 11: Activity 视觉合并 + 补全文案

**Files:**
- Create: `web/src/features/workspace/project-workbench/task-detail/activity-section.tsx`
- Modify: `web/src/features/workspace/project-workbench/task-detail/task-detail-page.tsx`
- Modify: `web/src/locales/zh-CN.ts`、`web/src/locales/en-US.ts`

spec §9.4 合并契约：避免「上半段全是注解、下半段全是变更」的假合并；第一版若无法按统一时间戳交错，必须显式标注为过渡态并列待办。

- [ ] **Step 1: 新建 activity-section.tsx（过渡态：注解 + 变更上下分区，但共用标题与时间轴样式）**

```tsx
import { useTranslation } from "react-i18next"

import type { ProjectTask } from "../api/task-api"
import { TaskAnnotationsEditor } from "./task-annotations-editor"
import { TaskChangeHistory } from "./task-change-history"

type ActivitySectionProps = {
  annotations?: ProjectTask["annotations"]
  canWrite: boolean
  projectSlug: string
  taskRef: string
  workspaceSlug: string
}

// 过渡态实现（spec §9.4 合并契约）：
// 当前注解与变更历史是两个独立数据源，未按统一时间戳交错排序。
// 视觉上归入同一 Activity 区块、共用标题与间距，先达成「同一时间轴感」。
// TODO（后续）：合并数据源，按统一时间戳倒序交错渲染。
export function ActivitySection({
  annotations,
  canWrite,
  projectSlug,
  taskRef,
  workspaceSlug,
}: ActivitySectionProps) {
  const { t } = useTranslation()
  return (
    <section className="space-y-4 border bg-card p-4">
      <h2 className="text-sm font-medium">{t("taskDetail.activity")}</h2>
      <TaskAnnotationsEditor
        annotations={annotations}
        canWrite={canWrite}
        projectSlug={projectSlug}
        taskRef={taskRef}
        workspaceSlug={workspaceSlug}
      />
      <TaskChangeHistory taskRef={taskRef} workspaceSlug={workspaceSlug} />
    </section>
  )
}
```

- [ ] **Step 2: 在 task-detail-page 用 ActivitySection 替换散装注解+历史**

修改 `task-detail-page.tsx`：把 `<TaskAnnotationsEditor>` 与 `<TaskChangeHistory>` 合并为 `<ActivitySection>`（在 Task 9 的 activity 容器内）。

- [ ] **Step 3: 补全 locale 文案**

zh-CN `taskDetail`：
```ts
    activity: "活动",
    groupProperties: "属性",
    groupSchedule: "计划",
    groupRelations: "关系",
    groupSystem: "系统",
    groupCustom: "自定义字段",
```

en-US `taskDetail`：
```ts
    activity: "Activity",
    groupProperties: "Properties",
    groupSchedule: "Schedule",
    groupRelations: "Relations",
    groupSystem: "System",
    groupCustom: "Custom fields",
```

- [ ] **Step 4: 全量验证**

Run:
```bash
pnpm --dir web typecheck
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web build
pnpm --dir web run smoke:editing
```
Expected: 全部通过。

- [ ] **Step 5: 提交**

```bash
git add web/src/features/workspace/project-workbench/task-detail/activity-section.tsx web/src/features/workspace/project-workbench/task-detail/task-detail-page.tsx web/src/locales/zh-CN.ts web/src/locales/en-US.ts
git commit -m "refactor: 注解与变更历史归入统一 Activity 区块（过渡态）"
```

---

## Task 12: 后端全量验证与文档更新

**Files:**
- Verify: 全仓库
- Update: `docs/superpowers/specs/2026-07-07-web-console-task-detail-redesign-design.md`（状态）、`ROADMAP.md`（如需）

- [ ] **Step 1: Go 全量测试（含 CGO=0）**

Run:
```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```
Expected: 全部通过。

- [ ] **Step 2: 集成测试**

Run: `go test ./tests/integration/... -count=1`
Expected: 通过（确认新增 parent 行为不破坏 CLI 黑盒）。

- [ ] **Step 3: 更新 spec 状态**

修改 `docs/superpowers/specs/2026-07-07-web-console-task-detail-redesign-design.md` 顶部：
```
**状态：** 草案待审
```
改为：
```
**状态：** 已实现（阶段一、二）
```

- [ ] **Step 4: 更新 ROADMAP（如该 milestone 在路线图中）**

Run: `grep -n "任务详情\|task detail\|子任务\|sub-task" ROADMAP.md`
若存在对应条目，标记为已完成；若无，跳过。

- [ ] **Step 5: git diff --check**

Run: `git diff --check`
Expected: 无空白错误。

- [ ] **Step 6: 最终提交**

```bash
git add docs/superpowers/specs/2026-07-07-web-console-task-detail-redesign-design.md ROADMAP.md
git commit -m "docs: 任务详情页重构 spec 标记为已实现"
```

---

## 验收清单（对应 spec §11）

- [ ] `app.Service.Add` 支持 parent，`Info` 返回 parent（Task 1-2）。
- [ ] parent 自引用/跨 workspace/deleted/completed/recurring/project 不一致均拒绝（Task 2）。
- [ ] HTTP `POST /tasks` 支持 parent 并返回 JSON（Task 3）。
- [ ] `GET /tasks/{ref}/children` 返回子任务，`include_closed` 控制可见性（Task 4）。
- [ ] children 端点遵守 workspace/project scope 与 task:read 权限（Task 4，复用 `scopedService`）。
- [ ] `CGO_ENABLED=0 go test ./...` 通过（Task 12）。
- [ ] 前端 `TaskDetailPage` 显示子任务区（Task 8）。
- [ ] 点击「添加子任务」展开 composer（Task 7-8）。
- [ ] 只填标题可创建，payload 含 project 和 parent（Task 7）。
- [ ] Enter 提交后 composer 清空但保持打开（Task 7、Task 8 测试）。
- [ ] completed/deleted/recurring/no-write 状态不显示创建入口（Task 8 门控）。
- [ ] 空态：可写引导、不可写「暂无子任务」（Task 7）。
- [ ] 默认隐藏 deleted、折叠 completed，提供「显示已完成」（Task 7）。
- [ ] 右栏属性分组渲染，空组隐身（Task 10）。
- [ ] 移动端 tab 切换（Task 9）。
- [ ] Activity 视觉合并（过渡态，已标注 TODO）（Task 11）。
