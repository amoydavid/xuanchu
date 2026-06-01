package mcpserver

import (
	"context"
	"strings"

	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/query"
	"github.com/dajee/taskg/internal/task"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type TaskAddInput struct {
	Workspace   string   `json:"workspace,omitempty" jsonschema:"workspace slug or UUID"`
	Project     string   `json:"project,omitempty" jsonschema:"project slug in the effective workspace"`
	ProjectID   string   `json:"project_id,omitempty" jsonschema:"stable project UUID"`
	Description string   `json:"description" jsonschema:"task description"`
	Tags        []string `json:"tags,omitempty"`
	Priority    string   `json:"priority,omitempty"`
	Due         *int64   `json:"due,omitempty" jsonschema:"unix seconds"`
	Wait        *int64   `json:"wait,omitempty" jsonschema:"unix seconds"`
	Scheduled   *int64   `json:"scheduled,omitempty" jsonschema:"unix seconds"`
	Until       *int64   `json:"until,omitempty" jsonschema:"unix seconds"`
	Annotations []string `json:"annotations,omitempty"`
}

func (in TaskAddInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type TaskQueryInput struct {
	Workspace        string `json:"workspace,omitempty"`
	Project          string `json:"project,omitempty"`
	ProjectID        string `json:"project_id,omitempty"`
	Query            string `json:"query,omitempty"`
	Status           string `json:"status,omitempty"`
	Limit            int    `json:"limit,omitempty"`
	IncludeCompleted bool   `json:"include_completed,omitempty"`
	IncludeDeleted   bool   `json:"include_deleted,omitempty"`
}

func (in TaskQueryInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type TaskGetInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	ID        string `json:"id" jsonschema:"task UUID; stdio may also use working-set ID"`
}

func (in TaskGetInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type TaskModifyInput struct {
	Workspace    string            `json:"workspace,omitempty"`
	Project      string            `json:"project,omitempty"`
	ProjectID    string            `json:"project_id,omitempty"`
	ID           string            `json:"id"`
	Description  *string           `json:"description,omitempty"`
	Priority     *string           `json:"priority,omitempty"`
	Due          *int64            `json:"due,omitempty"`
	Wait         *int64            `json:"wait,omitempty"`
	Scheduled    *int64            `json:"scheduled,omitempty"`
	Until        *int64            `json:"until,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
	RemoveTags   []string          `json:"remove_tags,omitempty"`
	UDAs         map[string]string `json:"udas,omitempty"`
	Clear        []string          `json:"clear,omitempty"`
	Depends      []string          `json:"depends,omitempty"`
	ClearDepends bool              `json:"clear_depends,omitempty"`
}

func (in TaskModifyInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type TaskIDInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	ID        string `json:"id"`
}

func (in TaskIDInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type TaskDoneInput = TaskIDInput
type TaskDeleteInput = TaskIDInput
type TaskStartInput = TaskIDInput
type TaskStopInput = TaskIDInput

type TaskAnnotateInput struct {
	Workspace   string `json:"workspace,omitempty"`
	Project     string `json:"project,omitempty"`
	ProjectID   string `json:"project_id,omitempty"`
	ID          string `json:"id"`
	Annotation  string `json:"annotation,omitempty"`
	Description string `json:"description,omitempty"`
}

func (in TaskAnnotateInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type TaskDependsInput struct {
	Workspace    string   `json:"workspace,omitempty"`
	ID           string   `json:"id"`
	Depends      []string `json:"depends,omitempty"`
	ClearDepends bool     `json:"clear_depends,omitempty"`
}

func (in TaskDependsInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace}
}

func registerTaskTools(s *mcp.Server, opts Options) {
	addTool(s, &mcp.Tool{Name: "task.add", Description: "Create a task; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskAddInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:write", app.PermissionTaskWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		project, err := projectSlugForTask(svc, in.Project, in.ProjectID)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		priority := stringPtrFromValue(in.Priority)
		created, err := svc.AddWithAnnotations(app.AddInput{
			Description: strings.TrimSpace(in.Description),
			Project:     project,
			Priority:    priority,
			Due:         in.Due,
			Wait:        in.Wait,
			Scheduled:   in.Scheduled,
			Until:       in.Until,
			Tags:        in.Tags,
		}, in.Annotations)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(taskData(created), "Created task "+created.UUID)
	})

	addTool(s, &mcp.Tool{Name: "task.query", Description: "Query tasks; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskQueryInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		limit, err := limitOrDefault(in.Limit)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:read", app.PermissionTaskRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		input := app.ListInput{Status: strings.TrimSpace(in.Status), Limit: limit}
		if input.Status == "" && (in.IncludeCompleted || in.IncludeDeleted) {
			input.ReportMode = true
		}
		if in.IncludeCompleted && !in.IncludeDeleted {
			input.Status = task.StatusCompleted
		}
		if in.IncludeDeleted {
			input.Status = task.StatusDeleted
		}
		if strings.TrimSpace(in.Query) != "" {
			expr, err := query.ParseFilterExpr([]string{in.Query})
			if err != nil {
				return businessErrorWithEnvelope(err)
			}
			input.Query = expr
		}
		if in.Project != "" || in.ProjectID != "" {
			project, err := svc.ProjectInfo(projectRefForScope(in.Project, in.ProjectID))
			if err != nil {
				return businessErrorWithEnvelope(err)
			}
			input.Query = query.And(input.Query, query.Predicate{Attribute: query.AttrProjectID, Operator: query.OpEqual, Value: query.StringValue(project.ID)})
		}
		rows, err := svc.List(input)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(tasksData(rows), renderTaskList(rows))
	})

	addTool(s, &mcp.Tool{Name: "task.get", Description: "Get one task; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskGetInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		if opts.Mode == ModeHTTP {
			if _, err := uuid.Parse(strings.TrimSpace(in.ID)); err != nil {
				return businessErrorWithEnvelope(app.RuntimeError{Code: "task_uuid_invalid", Message: "task UUID is invalid"})
			}
		}
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:read", app.PermissionTaskRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		tsk, err := svc.Info(strings.TrimSpace(in.ID))
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(taskData(tsk), renderTaskInfo(tsk))
	})

	addTool(s, &mcp.Tool{Name: "task.modify", Description: "Modify a task; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskModifyInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		return modifyTaskTool(ctx, req, opts, in, "modified task")
	})
	addTool(s, &mcp.Tool{Name: "task.done", Description: "Complete a task; writes audit."}, taskActionHandler(opts, "task done", func(svc *app.Service, id string) error { return svc.Done(id) }))
	addTool(s, &mcp.Tool{Name: "task.delete", Description: "Delete a task; writes audit."}, taskActionHandler(opts, "task deleted", func(svc *app.Service, id string) error { return svc.Delete(id) }))
	addTool(s, &mcp.Tool{Name: "task.start", Description: "Start a task; writes audit."}, taskActionHandler(opts, "task started", func(svc *app.Service, id string) error { return svc.Start(id) }))
	addTool(s, &mcp.Tool{Name: "task.stop", Description: "Stop a task; writes audit."}, taskActionHandler(opts, "task stopped", func(svc *app.Service, id string) error { return svc.Stop(id) }))
	addTool(s, &mcp.Tool{Name: "task.annotate", Description: "Annotate a task; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskAnnotateInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:write", app.PermissionTaskWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		text := strings.TrimSpace(in.Annotation)
		if text == "" {
			text = strings.TrimSpace(in.Description)
		}
		if err := svc.Annotate(strings.TrimSpace(in.ID), text); err != nil {
			return businessErrorWithEnvelope(err)
		}
		return taskAfterMutation(svc, in.ID, "annotated task")
	})
	addTool(s, &mcp.Tool{Name: "task.depends", Description: "Adjust task dependencies; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskDependsInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		mod := TaskModifyInput{Workspace: in.Workspace, ID: in.ID, Depends: in.Depends, ClearDepends: in.ClearDepends}
		return modifyTaskTool(ctx, req, opts, mod, "updated dependencies")
	})
}

func modifyTaskTool(ctx context.Context, req *mcp.CallToolRequest, opts Options, in TaskModifyInput, rendered string) (*mcp.CallToolResult, ToolEnvelope, error) {
	svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:write", app.PermissionTaskWrite)
	if err != nil {
		return businessErrorWithEnvelope(err)
	}
	project, err := projectSlugForTask(svc, in.Project, in.ProjectID)
	if err != nil {
		return businessErrorWithEnvelope(err)
	}
	mod := app.ModifyInput{
		Description:  in.Description,
		Project:      project,
		Priority:     in.Priority,
		Due:          in.Due,
		Wait:         in.Wait,
		Scheduled:    in.Scheduled,
		Until:        in.Until,
		AddTags:      in.Tags,
		RemoveTags:   in.RemoveTags,
		UDAs:         in.UDAs,
		AddDepends:   in.Depends,
		ClearDepends: in.ClearDepends,
	}
	applyClearFields(in.Clear, &mod)
	if err := svc.Modify(strings.TrimSpace(in.ID), mod); err != nil {
		return businessErrorWithEnvelope(err)
	}
	return taskAfterMutation(svc, in.ID, rendered)
}

func taskActionHandler(opts Options, rendered string, fn func(*app.Service, string) error) mcp.ToolHandlerFor[TaskIDInput, ToolEnvelope] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in TaskIDInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:write", app.PermissionTaskWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		if err := fn(svc, strings.TrimSpace(in.ID)); err != nil {
			return businessErrorWithEnvelope(err)
		}
		return taskAfterMutation(svc, in.ID, rendered)
	}
}

func taskAfterMutation(svc *app.Service, id, rendered string) (*mcp.CallToolResult, ToolEnvelope, error) {
	tsk, err := svc.Info(strings.TrimSpace(id))
	if err != nil {
		return businessErrorWithEnvelope(err)
	}
	return successWithEnvelope(taskData(tsk), rendered)
}

func projectSlugForTask(svc *app.Service, project, projectID string) (*string, error) {
	project = strings.TrimSpace(project)
	projectID = strings.TrimSpace(projectID)
	if project != "" && projectID != "" {
		if err := ensureProjectRefsMatch(svc, project, projectID); err != nil {
			return nil, err
		}
	}
	if project == "" && projectID != "" {
		view, err := svc.ProjectInfo(projectID)
		if err != nil {
			return nil, err
		}
		project = view.Slug
	}
	return stringPtrFromValue(project), nil
}

func projectRefForScope(project, projectID string) string {
	if strings.TrimSpace(projectID) != "" {
		return strings.TrimSpace(projectID)
	}
	return strings.TrimSpace(project)
}

func applyClearFields(fields []string, mod *app.ModifyInput) {
	for _, field := range fields {
		switch strings.TrimSpace(field) {
		case "project":
			mod.ClearProject = true
		case "priority":
			mod.ClearPriority = true
		case "due":
			mod.ClearDue = true
		case "wait":
			mod.ClearWait = true
		case "scheduled":
			mod.ClearScheduled = true
		case "until":
			mod.ClearUntil = true
		case "recur":
			mod.ClearRecur = true
		default:
			if strings.HasPrefix(field, "uda.") {
				mod.ClearUDAs = append(mod.ClearUDAs, strings.TrimPrefix(field, "uda."))
			}
		}
	}
}

func successWithEnvelope(data any, rendered string) (*mcp.CallToolResult, ToolEnvelope, error) {
	result, envelope, err := successResult(data, rendered)
	return result, envelope, err
}

func businessErrorWithEnvelope(err error) (*mcp.CallToolResult, ToolEnvelope, error) {
	return businessErrorResult(err), ToolEnvelope{}, nil
}
