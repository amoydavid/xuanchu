# M15 MCP Tool 全量覆盖 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan.

**Goal:** 将 CLI/HTTP 已有但 MCP 未暴露的 41 项操作全部补齐为 MCP tool，使 Agent 通过 MCP 可完成所有 CLI/HTTP 能完成的操作。

**Architecture:** 每个 MCP tool 是纯薄壳——鉴权（`serviceForTool`）→ 调用 `internal/app` 已有方法 → 格式化输出（`successWithEnvelope`/`businessErrorWithEnvelope`）。不引入任何业务逻辑。新增 4 个文件（hook、token、audit、misc），修改 8 个现有文件。

**Tech Stack:** Go 1.25, MCP Go SDK (`github.com/modelcontextprotocol/go-sdk`), Cobra

---

## Chunk 1: Task 补充（4 个 tool）

### Task 1: task_denotate + task_link_list

**Files:**
- Modify: `internal/mcpserver/tools_task.go`
- Modify: `internal/mcpserver/integration_test.go`

- [ ] **Step 1: 添加 Input struct 和 tool 注册到 `tools_task.go`**

在 `registerTaskTools` 函数末尾（`task_link_remove` handler 之后）添加：

```go
type TaskDenotateInput struct {
	Workspace        string `json:"workspace,omitempty"`
	Project          string `json:"project,omitempty"`
	ProjectID        string `json:"project_id,omitempty"`
	ID               string `json:"id"`
	AnnotationIndex  int    `json:"annotation_index"`
}

func (in TaskDenotateInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type TaskLinkListInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Task      string `json:"task" jsonschema:"task reference (UUID or working-set ID)"`
}

func (in TaskLinkListInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}
```

在 `registerTaskTools` 的 `task_link_remove` 之后添加两个 `addTool` 调用：

```go
addTool(s, &mcp.Tool{Name: "task_denotate", Description: "Remove an annotation from a task; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskDenotateInput) (*mcp.CallToolResult, ToolEnvelope, error) {
	svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:write", app.PermissionTaskWrite)
	if err != nil {
		return businessErrorWithEnvelope(err)
	}
	if err := svc.Denotate(strings.TrimSpace(in.ID), in.AnnotationIndex); err != nil {
		return businessErrorWithEnvelope(err)
	}
	return taskAfterMutation(svc, in.ID, "removed annotation")
})

addTool(s, &mcp.Tool{Name: "task_link_list", Description: "List external links on a task; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskLinkListInput) (*mcp.CallToolResult, ToolEnvelope, error) {
	svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:read", app.PermissionTaskRead)
	if err != nil {
		return businessErrorWithEnvelope(err)
	}
	tsk, err := svc.Info(strings.TrimSpace(in.Task))
	if err != nil {
		return businessErrorWithEnvelope(err)
	}
	return successWithEnvelope(map[string]any{"links": tsk.Links, "count": len(tsk.Links)}, fmt.Sprintf("%d link(s)", len(tsk.Links)))
})
```

注意：需要在文件顶部 import 中确认 `"fmt"` 已存在。

- [ ] **Step 2: 构建验证**

```bash
go build ./cmd/taskg
```

### Task 2: task_export + task_import

**Files:**
- Modify: `internal/mcpserver/tools_task.go`

- [ ] **Step 1: 添加 task_export 和 task_import**

在 `registerTaskTools` 末尾添加：

```go
type TaskExportInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

func (in TaskExportInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type TaskImportInput struct {
	Workspace string           `json:"workspace,omitempty"`
	Project   string           `json:"project,omitempty"`
	ProjectID string           `json:"project_id,omitempty"`
	Tasks     []task.JSONTask  `json:"tasks"`
}

func (in TaskImportInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}
```

在 `registerTaskTools` 中添加：

```go
addTool(s, &mcp.Tool{Name: "task_export", Description: "Export tasks as JSON; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskExportInput) (*mcp.CallToolResult, ToolEnvelope, error) {
	svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:read", app.PermissionTaskRead)
	if err != nil {
		return businessErrorWithEnvelope(err)
	}
	tasks, err := svc.Export()
	if err != nil {
		return businessErrorWithEnvelope(err)
	}
	dto := make([]task.JSONTask, len(tasks))
	for i, t := range tasks {
		dto[i] = task.ToJSON(t)
	}
	return successWithEnvelope(map[string]any{"tasks": dto, "count": len(dto)}, fmt.Sprintf("exported %d task(s)", len(dto)))
})

addTool(s, &mcp.Tool{Name: "task_import", Description: "Import tasks from JSON; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskImportInput) (*mcp.CallToolResult, ToolEnvelope, error) {
	svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:write", app.PermissionTaskWrite)
	if err != nil {
		return businessErrorWithEnvelope(err)
	}
	count, err := svc.Import(in.Tasks)
	if err != nil {
		return businessErrorWithEnvelope(err)
	}
	return successWithEnvelope(map[string]any{"imported": count}, fmt.Sprintf("imported %d task(s)", count))
})
```

注意：需在 import 中加入 `"github.com/dajee/taskg/internal/task"`（如果还没有）。

- [ ] **Step 2: 构建并测试**

```bash
go build ./cmd/taskg
go test ./internal/mcpserver/ -count=1
```

- [ ] **Step 3: 提交**

```bash
git add internal/mcpserver/tools_task.go
git commit -m "feat: MCP task_denotate/task_link_list/task_export/task_import"
```

---

## Chunk 2: Project 补充（6 个 tool）

### Task 3: project_add / project_modify / project_archive

**Files:**
- Modify: `internal/mcpserver/tools_project.go`

- [ ] **Step 1: 添加 Input struct**

在文件中已有 struct 之后添加：

```go
type ProjectAddInput struct {
	Workspace   string `json:"workspace,omitempty"`
	Slug        string `json:"slug"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

type ProjectModifyInput struct {
	Workspace   string  `json:"workspace,omitempty"`
	Project     string  `json:"project,omitempty"`
	ProjectID   string  `json:"project_id,omitempty"`
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

type ProjectArchiveInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}
```

- [ ] **Step 2: 在 `registerProjectTools` 末尾添加 3 个 tool**

```go
addTool(s, &mcp.Tool{Name: "project_add", Description: "Create a project in the effective workspace; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectAddInput) (*mcp.CallToolResult, ToolEnvelope, error) {
	svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace}, "project:write", app.PermissionProjectManage)
	if err != nil {
		return businessErrorWithEnvelope(err)
	}
	view, err := svc.AddProject(app.AddProjectInput{Slug: in.Slug, Name: in.Name, Description: in.Description})
	if err != nil {
		return businessErrorWithEnvelope(err)
	}
	return successWithEnvelope(map[string]any{"project": projectViewFromApp(view)}, "created project "+view.Slug)
})

addTool(s, &mcp.Tool{Name: "project_modify", Description: "Modify a project; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectModifyInput) (*mcp.CallToolResult, ToolEnvelope, error) {
	ref := projectRefForScope(in.Project, in.ProjectID)
	if strings.TrimSpace(ref) == "" {
		return businessErrorWithEnvelope(app.RuntimeError{Code: "project_not_found", Message: "project reference is required"})
	}
	svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}, "project:write", app.PermissionProjectManage)
	if err != nil {
		return businessErrorWithEnvelope(err)
	}
	if err := svc.ModifyProject(ref, app.ModifyProjectInput{Name: in.Name, Description: in.Description}); err != nil {
		return businessErrorWithEnvelope(err)
	}
	view, err := svc.ProjectInfo(ref)
	if err != nil {
		return businessErrorWithEnvelope(err)
	}
	return successWithEnvelope(map[string]any{"project": projectViewFromApp(view)}, "modified project "+view.Slug)
})

addTool(s, &mcp.Tool{Name: "project_archive", Description: "Archive a project; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectArchiveInput) (*mcp.CallToolResult, ToolEnvelope, error) {
	ref := projectRefForScope(in.Project, in.ProjectID)
	if strings.TrimSpace(ref) == "" {
		return businessErrorWithEnvelope(app.RuntimeError{Code: "project_not_found", Message: "project reference is required"})
	}
	svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}, "project:write", app.PermissionProjectManage)
	if err != nil {
		return businessErrorWithEnvelope(err)
	}
	view, err := svc.ArchiveProject(ref)
	if err != nil {
		return businessErrorWithEnvelope(err)
	}
	return successWithEnvelope(map[string]any{"project": projectViewFromApp(view)}, "archived project "+view.Slug)
})
```

- [ ] **Step 3: 构建**

```bash
go build ./cmd/taskg
```

### Task 4: project_config_set / project_config_unset / project_config_list

**Files:**
- Modify: `internal/mcpserver/tools_project.go`

- [ ] **Step 1: 添加 Input struct 和 tool**

```go
type ProjectConfigSetInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Key       string `json:"key"`
	Value     string `json:"value"`
}

type ProjectConfigUnsetInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Key       string `json:"key"`
}

type ProjectConfigListInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}
```

在 `registerProjectTools` 中添加：

```go
addTool(s, &mcp.Tool{Name: "project_config_set", Description: "Set a project config value; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectConfigSetInput) (*mcp.CallToolResult, ToolEnvelope, error) {
	ref := projectRefForScope(in.Project, in.ProjectID)
	if strings.TrimSpace(ref) == "" {
		return businessErrorWithEnvelope(app.RuntimeError{Code: "project_not_found", Message: "project reference is required"})
	}
	svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}, "config:write", app.PermissionProjectManage)
	if err != nil {
		return businessErrorWithEnvelope(err)
	}
	if err := svc.ProjectConfigSet(ref, in.Key, in.Value); err != nil {
		return businessErrorWithEnvelope(err)
	}
	return successWithEnvelope(map[string]any{"key": in.Key, "value": in.Value}, "set config "+in.Key)
})

addTool(s, &mcp.Tool{Name: "project_config_unset", Description: "Remove a project config value; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectConfigUnsetInput) (*mcp.CallToolResult, ToolEnvelope, error) {
	ref := projectRefForScope(in.Project, in.ProjectID)
	if strings.TrimSpace(ref) == "" {
		return businessErrorWithEnvelope(app.RuntimeError{Code: "project_not_found", Message: "project reference is required"})
	}
	svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}, "config:write", app.PermissionProjectManage)
	if err != nil {
		return businessErrorWithEnvelope(err)
	}
	if err := svc.ProjectConfigUnset(ref, in.Key); err != nil {
		return businessErrorWithEnvelope(err)
	}
	return successWithEnvelope(map[string]any{"key": in.Key}, "unset config "+in.Key)
})

addTool(s, &mcp.Tool{Name: "project_config_list", Description: "List project config values; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectConfigListInput) (*mcp.CallToolResult, ToolEnvelope, error) {
	ref := projectRefForScope(in.Project, in.ProjectID)
	if strings.TrimSpace(ref) == "" {
		return businessErrorWithEnvelope(app.RuntimeError{Code: "project_not_found", Message: "project reference is required"})
	}
	svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}, "config:read", app.PermissionProjectRead)
	if err != nil {
		return businessErrorWithEnvelope(err)
	}
	config, err := svc.ProjectConfigList(ref)
	if err != nil {
		return businessErrorWithEnvelope(err)
	}
	return successWithEnvelope(map[string]any{"config": config, "count": len(config)}, fmt.Sprintf("%d config value(s)", len(config)))
})
```

- [ ] **Step 2: 构建并测试**

```bash
go build ./cmd/taskg
go test ./internal/mcpserver/ -count=1
```

- [ ] **Step 3: 提交**

```bash
git add internal/mcpserver/tools_project.go
git commit -m "feat: MCP project_add/modify/archive/config_set/config_unset/config_list"
```

---

## Chunk 3: Workspace 补充（5 个 tool）

### Task 5: workspace_add / workspace_info / workspace_modify / workspace_archive / workspace_use

**Files:**
- Modify: `internal/mcpserver/tools_workspace.go`

先读取该文件，理解现有 `workspace_list` 和 `workspace_get_current` 的实现模式。然后：

- [ ] **Step 1: 添加 Input struct**

```go
type WorkspaceAddInput struct {
	Slug        string `json:"slug"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Visibility  string `json:"visibility,omitempty"`
}

type WorkspaceRefInput struct {
	Workspace string `json:"workspace,omitempty"`
}

type WorkspaceModifyInput struct {
	Workspace   string  `json:"workspace,omitempty"`
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Visibility  *string `json:"visibility,omitempty"`
}
```

- [ ] **Step 2: 在 `registerWorkspaceTools` 中添加 5 个 tool**

`workspace_add`：scope = `workspace:write`，permission = `PermissionWorkspaceModify`（只有 owner/admin 能创建）。

`workspace_info`：scope = `workspace:read`，permission = `PermissionWorkspaceRead`。

`workspace_modify`：scope = `workspace:write`，permission = `PermissionWorkspaceModify`。

`workspace_archive`：scope = `workspace:write`，permission = `PermissionWorkspaceArchive`。

`workspace_use`：scope = `workspace:write`，permission = `PermissionWorkspaceModify`。

每个 tool 调用对应的 app service 方法：`AddWorkspace`、`WorkspaceInfo`、`ModifyWorkspace`、`ArchiveWorkspace`、`UseWorkspace`。

`workspace_add` 不需要 workspace scope（创建新 workspace），所以 `serviceForTool` 的 `RequestScopeInput` 只有 workspace 参数在 workspace_use 场景下才需要。

- [ ] **Step 3: 构建并测试**

```bash
go build ./cmd/taskg
go test ./internal/mcpserver/ -count=1
```

- [ ] **Step 4: 提交**

```bash
git add internal/mcpserver/tools_workspace.go
git commit -m "feat: MCP workspace_add/info/modify/archive/use"
```

---

## Chunk 4: User / Member / Context / Config 补充（9 个 tool）

### Task 6: user_add / user_use / user_list_external_ids

**Files:**
- Modify: `internal/mcpserver/tools_user.go`

- [ ] **Step 1: 添加 3 个 tool**

`user_add`：scope = `workspace:write`，permission = `PermissionWorkspaceModify`。调用 `AddUser(input)`。

`user_use`：scope = `workspace:write`，permission = `PermissionWorkspaceModify`。调用 `UseUser(ref)`。

`user_list_external_ids`：scope = `workspace:read`，permission = `PermissionWorkspaceRead`。需要先解析 user ref 为 user ID。调用 `ListExternalIDs(userID)`。

- [ ] **Step 2: 构建验证**

### Task 7: member_role

**Files:**
- Modify: `internal/mcpserver/tools_member.go`

- [ ] **Step 1: 添加 member_role tool**

```go
type MemberRoleInput struct {
	Workspace string `json:"workspace,omitempty"`
	User      string `json:"user"`
	Role      string `json:"role"`
}
```

scope = `workspace:write`，permission = `PermissionMemberManage`。调用 `ChangeMemberRole(input)`。

- [ ] **Step 2: 构建验证**

### Task 8: context_none / context_list / context_delete

**Files:**
- Modify: `internal/mcpserver/tools_context.go`

- [ ] **Step 1: 添加 3 个 tool**

`context_none`：scope = `config:write`，permission = `PermissionContextManage`。调用 `ContextNone()`。

`context_list`：scope = `config:read`，permission = `PermissionContextUse`。调用 `ContextList()`。

`context_delete`：scope = `config:write`，permission = `PermissionContextManage`。调用 `ContextDelete(name)`。

- [ ] **Step 2: 构建验证**

### Task 9: config_unset / config_list

**Files:**
- Modify: `internal/mcpserver/tools_config.go`

- [ ] **Step 1: 添加 2 个 tool**

`config_unset`：scope = `config:write`，permission = `PermissionUDAManage`。调用 `UnsetConfig(key)`。

`config_list`：scope = `config:read`，permission = `PermissionContextUse`。调用 `ConfigValues()`。

- [ ] **Step 2: 构建并测试**

```bash
go test ./internal/mcpserver/ -count=1
```

- [ ] **Step 3: 提交**

```bash
git add internal/mcpserver/tools_user.go internal/mcpserver/tools_member.go internal/mcpserver/tools_context.go internal/mcpserver/tools_config.go
git commit -m "feat: MCP user_add/use/list_external_ids + member_role + context_none/list/delete + config_unset/list"
```

---

## Chunk 5: Hook 全域（10 个 tool）

### Task 10: 新建 tools_hook.go

**Files:**
- Create: `internal/mcpserver/tools_hook.go`
- Modify: `internal/mcpserver/server.go` — 注册 `registerHookTools`

- [ ] **Step 1: 创建 tools_hook.go**

参考 `tools_project.go` 的模式。10 个 Input struct + 10 个 `addTool` 调用。

Input struct 设计：

```go
type HookListInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

type HookAddInput struct {
	Workspace string   `json:"workspace,omitempty"`
	Project   string   `json:"project,omitempty"`
	ProjectID string   `json:"project_id,omitempty"`
	Name      string   `json:"name"`
	URL       string   `json:"url"`
	Events    []string `json:"events"`
	Secret    string   `json:"secret,omitempty"`
}

type HookRefInput struct {
	HookID string `json:"hook_id"`
}

type HookModifyInput struct {
	HookID string   `json:"hook_id"`
	Name   *string  `json:"name,omitempty"`
	URL    *string  `json:"url,omitempty"`
	Events []string `json:"events,omitempty"`
	Secret *string  `json:"secret,omitempty"`
}

type HookListDeliveriesInput struct {
	HookID string `json:"hook_id"`
	Status string `json:"status,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type HookDeliveryRefInput struct {
	DeliveryID string `json:"delivery_id"`
}
```

Tool 注册（`registerHookTools` 函数）：

| Tool | App Method |
|---|---|
| `hook_list` | `ListHooks(projectRef)` |
| `hook_add` | `AddHook(app.HookAddInput{...})` |
| `hook_info` | `HookInfo(hookID)` |
| `hook_modify` | `ModifyHook(hookID, app.HookModifyInput{...})` |
| `hook_delete` | `DeleteHook(hookID)` |
| `hook_enable` | `EnableHook(hookID)` |
| `hook_disable` | `DisableHook(hookID)` |
| `hook_list_deliveries` | `ListHookDeliveries(hookID, status, limit)` |
| `hook_delivery_info` | `HookDeliveryInfo(deliveryID)` |
| `hook_delivery_replay` | `ReplayHookDelivery(deliveryID)` |

读操作的 scope = `hook:read`，permission = `PermissionHookRead`。
写操作的 scope = `hook:write`，permission = `PermissionHookWrite`。

注意：`hook_list` 和 `hook_add` 支持 project scope。其他通过 hookID 定位的 tool 不需要 project scope，但鉴权仍检查 actor 对 hook 所属 workspace 的权限（由 app 层保证）。

需要读取 `internal/app/hook.go` 确认 `HookAddInput`、`HookModifyInput`、`HookView`、`HookDeliveryView` 的结构。

- [ ] **Step 2: 在 server.go 中注册**

在 `registerConfigTools(s, opts)` 之后添加：
```go
registerHookTools(s, opts)
```

- [ ] **Step 3: 构建并测试**

```bash
go build ./cmd/taskg
go test ./internal/mcpserver/ -count=1
```

- [ ] **Step 4: 提交**

```bash
git add internal/mcpserver/tools_hook.go internal/mcpserver/server.go
git commit -m "feat: MCP hook 全域 10 个 tool"
```

---

## Chunk 6: Token 全域（4 个 tool）

### Task 11: 新建 tools_token.go

**Files:**
- Create: `internal/mcpserver/tools_token.go`
- Modify: `internal/mcpserver/server.go` — 注册 `registerTokenTools`

- [ ] **Step 1: 创建 tools_token.go**

```go
type TokenCreateInput struct {
	Name             string   `json:"name"`
	Type             string   `json:"type,omitempty"`
	Scopes           []string `json:"scopes,omitempty"`
	ExpiresInSeconds *int64   `json:"expires_in_seconds,omitempty"`
}

type TokenListInput struct {
	IncludeRevoked bool `json:"include_revoked,omitempty"`
}

type TokenModifyInput struct {
	TokenID          string   `json:"token_id"`
	Name             *string  `json:"name,omitempty"`
	Scopes           []string `json:"scopes,omitempty"`
	ExpiresInSeconds *int64   `json:"expires_in_seconds,omitempty"`
}

type TokenRevokeInput struct {
	TokenID string `json:"token_id"`
}
```

Tool 注册：

| Tool | App Method | Scope | Permission |
|---|---|---|---|
| `token_create` | `CreateToken(input)` | token:write | PermissionTokenWrite |
| `token_list` | `ListTokens(input)` | token:read | PermissionTokenRead |
| `token_modify` | `ModifyToken(input)` | token:write | PermissionTokenWrite |
| `token_revoke` | `RevokeToken(ref)` | token:write | PermissionTokenWrite |

`token_create` 需要转换 `ExpiresInSeconds` → `time.Duration`（与 HTTP handler 一致）。
`token_list` 不需要 workspace scope。
`token_revoke` 使用 `RevokeToken`（非 `RevokeTokenWithLimit`，MCP 不做父 token 委托）。

需要 import `"time"` 和 `"github.com/dajee/taskg/internal/app"`。

- [ ] **Step 2: 在 server.go 中注册**

```go
registerTokenTools(s, opts)
```

- [ ] **Step 3: 构建并测试**

```bash
go build ./cmd/taskg
go test ./internal/mcpserver/ -count=1
```

- [ ] **Step 4: 提交**

```bash
git add internal/mcpserver/tools_token.go internal/mcpserver/server.go
git commit -m "feat: MCP token_create/list/modify/revoke"
```

---

## Chunk 7: Audit + Scope + Me（3 个 tool）

### Task 12: 新建 tools_audit.go + tools_misc.go

**Files:**
- Create: `internal/mcpserver/tools_audit.go`
- Create: `internal/mcpserver/tools_misc.go`
- Modify: `internal/mcpserver/server.go` — 注册

- [ ] **Step 1: 创建 tools_audit.go**

```go
type AuditListInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Action    string `json:"action,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}
```

`audit_list`：scope = `audit:read`，permission = `PermissionAuditRead`。调用 `ListAudit(app.AuditListInput{...})`。

- [ ] **Step 2: 创建 tools_misc.go**

```go
type ScopeListInput struct{}

type MeGetInput struct {
	Workspace string `json:"workspace,omitempty"`
}
```

`scope_list`：无 scope，无 permission。直接调用 `auth.ScopeRegistryValues()`，返回 scope 列表。

`me_get`：需要获取当前认证的 actor 信息。从 `serviceForTool` 获取 svc，然后从 `svc.Runtime()` 提取 actorUserID、workspaceID 等。返回 `{user_id, workspace_id, scopes}`。

对于 stdio 模式，`me_get` 返回本地用户信息。对于 HTTP 模式，返回 token 绑定的用户。

- [ ] **Step 3: 在 server.go 中注册**

```go
registerAuditTools(s, opts)
registerMiscTools(s, opts)
```

- [ ] **Step 4: 构建并测试**

```bash
go build ./cmd/taskg
go test ./internal/mcpserver/ -count=1
```

- [ ] **Step 5: 提交**

```bash
git add internal/mcpserver/tools_audit.go internal/mcpserver/tools_misc.go internal/mcpserver/server.go
git commit -m "feat: MCP audit_list/scope_list/me_get"
```

---

## Chunk 8: Schema golden test + 文档更新 + 最终验证

### Task 13: 更新 schema golden test

**Files:**
- Modify: `internal/mcpserver/integration_test.go`

- [ ] **Step 1: 更新 `TestAllToolNames` 中的 tool 名称列表**

从 33 个扩展到 74 个。添加所有新 tool 名称。

- [ ] **Step 2: 运行 golden test 确认 schema 正确**

```bash
go test ./internal/mcpserver/ -run TestAllToolNames -v
```

如果 schema 自动生成有问题（如 required 字段），修复 Input struct 的 `jsonschema` tag。

### Task 14: 更新 mcp.md 文档

**Files:**
- Modify: `docs/manual/mcp.md`

- [ ] **Step 1: 更新 tool 列表为 74 个**

在参数表中补充所有新 tool。格式与现有一致。

- [ ] **Step 2: 更新 tool 计数**

搜索 "33" 或 "当前提供" 相关文字，改为 74。

- [ ] **Step 3: 提交**

```bash
git add docs/manual/mcp.md internal/mcpserver/integration_test.go
git commit -m "docs: MCP 文档更新为 74 个 tool"
```

### Task 15: 最终验证

- [ ] **Step 1: 全量测试**

```bash
CGO_ENABLED=0 go build ./cmd/taskg
CGO_ENABLED=0 go test ./... -count=1
```

- [ ] **Step 2: 确认 tool 数量**

```bash
grep -c 'addTool(s,' internal/mcpserver/tools_*.go
```

预期 74。

- [ ] **Step 3: 更新 ROADMAP**

在 `ROADMAP.md` 的 M14.1 之后添加 M15 条目，状态表新增一行。

```bash
git add ROADMAP.md
git commit -m "docs: ROADMAP 更新 M15 MCP tool 全量覆盖"
```
