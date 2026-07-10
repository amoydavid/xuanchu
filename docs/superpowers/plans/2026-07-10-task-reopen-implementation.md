# 实现计划：任务重新打开（reopen completed task）

日期：2026-07-10
对应 spec：`docs/superpowers/specs/2026-07-10-task-reopen-design.md`

## 任务拆解（按依赖顺序）

### Step 1：domain 层加 `Reopen` 方法
**文件**：`internal/task/model.go`（在 `StopTask` 之后，约 line 189）

```go
// Reopen 把已完成任务恢复为 pending。
// 仅作为 Complete 的逆操作：清掉 End（完成时间）与 Start（计时锚点），
// 回到普通待处理状态，用户可重新 start。
func (t *Task) Reopen(now int64) {
	t.Status = StatusPending
	t.End = nil
	t.Start = nil
	t.Modified = now
}
```

**验收**：`go build ./...` 通过。`Validate()` 对 pending 状态已放行，无需改。

---

### Step 2：app 层加 `Reopen` service 方法
**文件**：`internal/app/service.go`

照 `Stop`（line 968）+ `stopLocked`（line 983）模板：

```go
func (s *Service) Reopen(target string) error {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return err
	}
	return s.withAuditAndEvents(func(tx *Service) (*AuditEntry, []HookEvent, error) {
		reopenedTask, change, err := tx.reopenLocked(target)
		if err != nil {
			return nil, nil, err
		}
		event := buildTaskHookEvent("task.reopened", reopenedTask, tx.runtime, tx.clock.Unix())
		entry := taskAuditEntry("task.reopen", reopenedTask.UUID, change)
		return &entry, []HookEvent{event}, nil
	})
}

func (s *Service) reopenLocked(target string) (task.Task, projectChange, error) {
	tsk, err := s.resolveTargetForWrite(target)
	if err != nil {
		return task.Task{}, projectChange{}, err
	}
	change := projectChangeForTask(tsk)
	if tsk.Status != task.StatusCompleted {
		return task.Task{}, projectChange{}, fmt.Errorf("cannot reopen %s task", tsk.Status)
	}
	tsk.Reopen(s.clock.Unix())
	if err := s.repo.Update(tsk); err != nil {
		return task.Task{}, projectChange{}, err
	}
	return tsk, change, nil
}
```

**验收**：`go test ./internal/app/...`

---

### Step 3：app 层单元测试
**文件**：`internal/app/service_test.go`（照 `TestServiceDeleteAndStopRejectTerminalStates` line 4177 风格）

- `TestServiceReopen`：建任务 → Done → Reopen → 验证 `Status==StatusPending`、`End==nil`、`Start==nil`。
- `TestServiceReopenRejectsNonCompleted`：对 pending / deleted（先 Done 再 Delete 不可达，用新建 pending）任务调 Reopen 应报错。recurring 不在本轮范围但守卫会拦。

**验收**：`go test ./internal/app/ -run TestServiceReopen`

---

### Step 4：HTTP handler + 路由
**文件**：
- `internal/httpapi/tasks.go`（照 `handleTaskStop` line 519）：
  ```go
  func (s *Server) handleTaskReopen(w http.ResponseWriter, r *http.Request) {
  	s.handleTaskAction(w, r, func(svc *app.Service, id string) error { return svc.Reopen(id) })
  }
  ```
- `internal/httpapi/huma_routes.go`（line 186 后加一行）：
  ```go
  {Method: http.MethodPost, Path: "/api/v1/tasks/{taskRef}/reopen", Tag: "Tasks", Summary: "Reopen a completed task.", Handler: s.handleTaskReopen},
  ```

**验收**：`go build ./internal/httpapi/...`

---

### Step 5：HTTP 集成测试
**文件**：`internal/httpapi/tasks_test.go`
- `TestTaskHTTPAcceptsTaskSlugRefs`（line 21）cases 加 reopen（需 before 先 Done）。
- `TestTaskHTTPRejectsNumericTaskRef`（line 114）cases 加 reopen。

**验收**：`go test ./internal/httpapi/ -run TestTaskHTTP`

---

### Step 6：远程 client
**文件**：`internal/remote/task.go`（照 `StopTask` line 247）
```go
func (c *Client) ReopenTask(ctx context.Context, workspace, taskID string) error {
	return c.postTaskAction(ctx, workspace, taskID, "reopen", nil)
}
```

---

### Step 7：CLI target+action dispatch
**文件**：`internal/cli/root.go`
- `knownTargetActions()`（line 197-213）map 加 `"reopen": true`。
- `handleTargetAction`（line ~400-419）switch 加 `case "reopen": return svc.Reopen(target)`。
- `handleRemoteTargetAction`（line ~541-560）switch 加 `case "reopen": return client.ReopenTask(ctx, ws, id)`。

**验收**：`go build ./cmd/xuanchu`；`tests/integration` 相关用例（如有 done/start/stop 的对照，可顺手补 reopen）。

---

### Step 8：前端 API + mutations
**文件**：
- `web/src/features/workspace/project-workbench/api/task-api.ts`：加 `taskReopenPath` + `reopenTask`（照 `taskDonePath` line 159 / `doneTask` line 322）。
- `web/src/features/workspace/project-workbench/hooks/use-task-mutations.ts`：
  - `TaskAction` 类型（line 28）加 `"reopen"`。
  - `useTaskActionMutation`（line ~163-194）加分支。
  - `taskActionSuccessLabel`（line ~308-319）加 reopen 文案。

---

### Step 9：前端 UI 按钮
**文件**：
- `task-detail/task-action-bar.tsx`：completed 时显示「重新打开」按钮（当前 completed 分支什么都不显示，这里补 reopen 按钮）。
- `tasks/task-row-actions.tsx`：completed 行加 reopen 图标按钮（当前 `!isCompleted` 整组隐藏，需在 `isCompleted` 分支补 reopen 入口）。

---

### Step 10：前端 i18n（如 task-row-actions 用到）
**文件**：`web/src/locales/zh-CN.ts` + `en-US.ts` 同步加 `reopenTask` key（受 `i18n.test.ts` 对齐测试约束，必须两文件同步）。

**验收**：`cd web && npm test`（i18n 对齐测试）+ `npm run build`。

---

### Step 11：全量验证 + 文档
```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
cd web && npm run build
```
- README.md 补 reopen 命令说明。

## 验收标准
1. completed 任务经 reopen 后 status=pending、End=nil、Start=nil，可在 web console 正常 start/done/edit。
2. 非 completed（pending/deleted/recurring）任务调 reopen 报错。
3. CLI `xuanchu <ref> reopen` 和 `POST /api/v1/tasks/{ref}/reopen` 都可用。
4. `task.reopened` hook event 和 `task.reopen` audit 正确产生。
5. 全量 `go test` + `CGO_ENABLED=0` build 通过，web build 通过。
