package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/query"
	"git.dajee.net/dajee/xuanchu/internal/task"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type TaskAddInput struct {
	Workspace     string   `json:"workspace,omitempty" jsonschema:"workspace slug or UUID"`
	Project       string   `json:"project,omitempty" jsonschema:"project slug in the effective workspace"`
	ProjectID     string   `json:"project_id,omitempty" jsonschema:"stable project UUID"`
	Title         string   `json:"title" jsonschema:"task title"`
	Description   *string  `json:"description,omitempty" jsonschema:"task details"`
	Tags          []string `json:"tags,omitempty" jsonschema:"task tags to add"`
	Assignees     []string `json:"assignees,omitempty" jsonschema:"workspace user refs to assign"`
	Priority      string   `json:"priority,omitempty"`
	Due           *int64   `json:"due,omitempty" jsonschema:"unix seconds"`
	DueDate       string   `json:"due_date,omitempty" jsonschema:"deadline date as YYYY-MM-DD; stored at local 23:59:59"`
	Wait          *int64   `json:"wait,omitempty" jsonschema:"unix seconds"`
	WaitDate      string   `json:"wait_date,omitempty" jsonschema:"defer-until date as YYYY-MM-DD; stored at local 00:00:00"`
	Scheduled     *int64   `json:"scheduled,omitempty" jsonschema:"unix seconds"`
	ScheduledDate string   `json:"scheduled_date,omitempty" jsonschema:"scheduled start date as YYYY-MM-DD; stored at local 00:00:00"`
	Until         *int64   `json:"until,omitempty" jsonschema:"unix seconds"`
	UntilDate     string   `json:"until_date,omitempty" jsonschema:"effective-until date as YYYY-MM-DD; stored at local 23:59:59"`
	Annotations   []string `json:"annotations,omitempty" jsonschema:"initial annotations"`
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
	Offset           int    `json:"offset,omitempty"`
	IncludeCompleted bool   `json:"include_completed,omitempty"`
	IncludeDeleted   bool   `json:"include_deleted,omitempty"`
	// occurrence 查询参数（spec §13.3）。
	OccurrenceMode string `json:"occurrence_mode,omitempty" jsonschema:"auto|materialized|expand"`
	DueAfter       string `json:"due_after,omitempty" jsonschema:"YYYY-MM-DD"`
	DueBefore      string `json:"due_before,omitempty" jsonschema:"YYYY-MM-DD"`
}

func (in TaskQueryInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type TaskGetInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	ID        string `json:"id" jsonschema:"task reference: UUID or task_slug"`
}

func (in TaskGetInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type TaskModifyInput struct {
	Workspace       string            `json:"workspace,omitempty"`
	Project         string            `json:"project,omitempty"`
	ProjectID       string            `json:"project_id,omitempty"`
	ID              string            `json:"id" jsonschema:"task reference: UUID or task_slug"`
	Title           *string           `json:"title,omitempty"`
	Description     *string           `json:"description,omitempty"`
	Priority        *string           `json:"priority,omitempty"`
	Due             *int64            `json:"due,omitempty"`
	DueDate         string            `json:"due_date,omitempty" jsonschema:"deadline date as YYYY-MM-DD; stored at local 23:59:59"`
	Wait            *int64            `json:"wait,omitempty"`
	WaitDate        string            `json:"wait_date,omitempty" jsonschema:"defer-until date as YYYY-MM-DD; stored at local 00:00:00"`
	Scheduled       *int64            `json:"scheduled,omitempty"`
	ScheduledDate   string            `json:"scheduled_date,omitempty" jsonschema:"scheduled start date as YYYY-MM-DD; stored at local 00:00:00"`
	Until           *int64            `json:"until,omitempty"`
	UntilDate       string            `json:"until_date,omitempty" jsonschema:"effective-until date as YYYY-MM-DD; stored at local 23:59:59"`
	Tags            []string          `json:"tags,omitempty"`
	Assignees       []string          `json:"assignees,omitempty"`
	RemoveAssignees []string          `json:"remove_assignees,omitempty"`
	RemoveTags      []string          `json:"remove_tags,omitempty"`
	UDAs            map[string]string `json:"udas,omitempty"`
	Clear           []string          `json:"clear,omitempty"`
	Depends         []string          `json:"depends,omitempty"`
	ClearDepends    bool              `json:"clear_depends,omitempty"`
}

func (in TaskModifyInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type TaskIDInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	ID        string `json:"id" jsonschema:"task reference: UUID or task_slug"`
}

func (in TaskIDInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type TaskDoneInput = TaskIDInput
type TaskDeleteInput = TaskIDInput
type TaskStartInput = TaskIDInput
type TaskStopInput = TaskIDInput
type TaskReopenInput = TaskIDInput

type TaskAnnotateInput struct {
	Workspace  string `json:"workspace,omitempty"`
	Project    string `json:"project,omitempty"`
	ProjectID  string `json:"project_id,omitempty"`
	ID         string `json:"id" jsonschema:"task reference: UUID or task_slug"`
	Annotation string `json:"annotation"`
}

func (in TaskAnnotateInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type TaskDependsInput struct {
	Workspace    string   `json:"workspace,omitempty"`
	Project      string   `json:"project,omitempty"`
	ProjectID    string   `json:"project_id,omitempty"`
	ID           string   `json:"id" jsonschema:"task reference: UUID or task_slug"`
	Depends      []string `json:"depends,omitempty"`
	ClearDepends bool     `json:"clear_depends,omitempty"`
}

type TaskLinkAddInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Task      string `json:"task" jsonschema:"task reference: UUID or task_slug"`
	Type      string `json:"type" jsonschema:"link type (e.g. document, pr, ticket, design)"`
	URL       string `json:"url" jsonschema:"external resource URL"`
	Title     string `json:"title,omitempty" jsonschema:"optional display title"`
}

func (in TaskLinkAddInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type TaskLinkRemoveInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Task      string `json:"task" jsonschema:"task reference: UUID or task_slug"`
	LinkID    string `json:"link_id" jsonschema:"link ID to remove"`
}

func (in TaskLinkRemoveInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type TaskDenotateInput struct {
	Workspace    string `json:"workspace,omitempty"`
	Project      string `json:"project,omitempty"`
	ProjectID    string `json:"project_id,omitempty"`
	ID           string `json:"id" jsonschema:"task reference: UUID or task_slug"`
	AnnotationID string `json:"annotation_id" jsonschema:"annotation ID to remove"`
}

func (in TaskDenotateInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type TaskLinkListInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Task      string `json:"task" jsonschema:"task reference: UUID or task_slug"`
}

func (in TaskLinkListInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type TaskExportInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

func (in TaskExportInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type TaskImportInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	// native bundle 格式（spec §20）：{schema, exported_at, task_series, tasks}。
	Bundle app.TaskBundleV1 `json:"bundle"`
}

func (in TaskImportInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

func resolveToolDateField(field string, instant *int64, date string, endOfDay bool) (*int64, error) {
	date = strings.TrimSpace(date)
	if date == "" {
		return instant, nil
	}
	if instant != nil {
		return nil, fmt.Errorf("%s and %s_date cannot both be set", field, field)
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return nil, fmt.Errorf("%s_date must be YYYY-MM-DD", field)
	}
	var (
		value int64
		err   error
	)
	if endOfDay {
		value, err = query.ResolveDeadlineDateValue(query.ParseDateValue(date), time.Now().Unix(), time.Local)
	} else {
		value, err = query.ResolveStartDateValue(query.ParseDateValue(date), time.Now().Unix(), time.Local)
	}
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func resolveToolDateFields(due *int64, dueDate string, wait *int64, waitDate string, scheduled *int64, scheduledDate string, until *int64, untilDate string) (*int64, *int64, *int64, *int64, error) {
	resolvedDue, err := resolveToolDateField("due", due, dueDate, true)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	resolvedWait, err := resolveToolDateField("wait", wait, waitDate, false)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	resolvedScheduled, err := resolveToolDateField("scheduled", scheduled, scheduledDate, false)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	resolvedUntil, err := resolveToolDateField("until", until, untilDate, true)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return resolvedDue, resolvedWait, resolvedScheduled, resolvedUntil, nil
}

func registerTaskTools(s *mcp.Server, opts Options) {
	addTool(s, opts, &mcp.Tool{Name: "task_add", Description: "Create a task; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskAddInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:write", app.PermissionTaskWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		project, err := projectSlugForTask(svc, in.Project, in.ProjectID)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		priority := stringPtrFromValue(in.Priority)
		due, wait, scheduled, until, err := resolveToolDateFields(in.Due, in.DueDate, in.Wait, in.WaitDate, in.Scheduled, in.ScheduledDate, in.Until, in.UntilDate)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		created, err := svc.AddWithAnnotations(app.AddInput{
			Title:       strings.TrimSpace(in.Title),
			Description: in.Description,
			Project:     project,
			Priority:    priority,
			Due:         due,
			Assignees:   in.Assignees,
			Wait:        wait,
			Scheduled:   scheduled,
			Until:       until,
			Tags:        in.Tags,
		}, in.Annotations)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		data, err := taskData(created)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(data, "Created task "+created.UUID)
	})

	addTool(s, opts, &mcp.Tool{Name: "task_query", Description: "Query tasks; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskQueryInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		limit, err := limitOrDefault(in.Limit)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:read", app.PermissionTaskRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		input := app.ListInput{Status: taskQueryStatus(in), Limit: limit, Offset: in.Offset}
		statuses := taskQueryStatuses(in)
		if input.Status == "" && len(statuses) == 0 && (in.IncludeCompleted || in.IncludeDeleted) {
			input.ReportMode = true
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
		if taskQueryExcludeDeleted(in) {
			input.Query = query.And(input.Query, query.Predicate{Attribute: query.AttrStatus, Operator: query.OpNotEqual, Value: query.StringValue(task.StatusDeleted)})
		}
		rows, err := taskQueryList(svc, input, statuses)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(tasksData(rows), renderTaskList(rows))
	})

	addTool(s, opts, &mcp.Tool{Name: "task_get", Description: "Get one task; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskGetInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:read", app.PermissionTaskRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		// occurrence_ref 走 GetTaskView（projected 不物化，spec §13.4）。
		if app.IsOccurrenceRef(strings.TrimSpace(in.ID)) {
			view, verr := svc.GetTaskView(strings.TrimSpace(in.ID))
			if verr != nil {
				return businessErrorWithEnvelope(verr)
			}
			data := occurrenceViewToMCPJSON(view)
			return successWithEnvelope(data, formatOccurrenceViewText(view))
		}
		tsk, err := resolveToolTaskRef(svc, in.ID, "id", false)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		data, err := taskData(tsk)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(data, renderTaskInfo(tsk))
	})

	addTool(s, opts, &mcp.Tool{Name: "task_modify", Description: "Modify a task; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskModifyInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		return modifyTaskTool(ctx, req, opts, in, "modified task")
	})
	addTool(s, opts, &mcp.Tool{Name: "task_done", Description: "Complete a task; writes audit."}, taskActionHandler(opts, "task done", func(svc *app.Service, id string) error { return svc.Done(id) }))
	addTool(s, opts, &mcp.Tool{Name: "task_delete", Description: "Delete a task; writes audit."}, taskActionHandler(opts, "task deleted", func(svc *app.Service, id string) error { return svc.Delete(id) }))
	addTool(s, opts, &mcp.Tool{Name: "task_start", Description: "Start a task; writes audit."}, taskActionHandler(opts, "task started", func(svc *app.Service, id string) error { return svc.Start(id) }))
	addTool(s, opts, &mcp.Tool{Name: "task_stop", Description: "Stop a task; writes audit."}, taskActionHandler(opts, "task stopped", func(svc *app.Service, id string) error { return svc.Stop(id) }))
	addTool(s, opts, &mcp.Tool{Name: "task_reopen", Description: "Reopen a completed task; writes audit."}, taskActionHandler(opts, "task reopened", func(svc *app.Service, id string) error { return svc.Reopen(id) }))
	addTool(s, opts, &mcp.Tool{Name: "task_annotate", Description: "Annotate a task; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskAnnotateInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:write", app.PermissionTaskWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		resolved, err := resolveToolTaskRef(svc, in.ID, "id", true)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		if err := svc.Annotate(resolved.UUID, strings.TrimSpace(in.Annotation)); err != nil {
			return businessErrorWithEnvelope(err)
		}
		return taskAfterMutation(svc, resolved.UUID, "annotated task")
	})
	addTool(s, opts, &mcp.Tool{Name: "task_depends", Description: "Adjust task dependencies; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskDependsInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		mod := TaskModifyInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID, ID: in.ID, Depends: in.Depends, ClearDepends: in.ClearDepends}
		return modifyTaskTool(ctx, req, opts, mod, "updated dependencies")
	})
	addTool(s, opts, &mcp.Tool{Name: "task_link_add", Description: "Add an external link to a task; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskLinkAddInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:write", app.PermissionTaskWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		resolved, err := resolveToolTaskRef(svc, in.Task, "task", true)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		link, err := svc.TaskAddLink(resolved.UUID, strings.TrimSpace(in.Type), strings.TrimSpace(in.URL), strings.TrimSpace(in.Title))
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"link": taskLinkViewFromApp(link)}, "Added link to task")
	})
	addTool(s, opts, &mcp.Tool{Name: "task_link_remove", Description: "Remove an external link from a task; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskLinkRemoveInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:write", app.PermissionTaskWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		resolved, err := resolveToolTaskRef(svc, in.Task, "task", true)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		if err := svc.TaskRemoveLink(resolved.UUID, strings.TrimSpace(in.LinkID)); err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(nil, "Removed link from task")
	})
	addTool(s, opts, &mcp.Tool{Name: "task_denotate", Description: "Remove an annotation from a task; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskDenotateInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:write", app.PermissionTaskWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		resolved, err := resolveToolTaskRef(svc, in.ID, "id", true)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		if err := svc.Denotate(resolved.UUID, in.AnnotationID); err != nil {
			return businessErrorWithEnvelope(err)
		}
		return taskAfterMutation(svc, resolved.UUID, "removed annotation")
	})
	addTool(s, opts, &mcp.Tool{Name: "task_link_list", Description: "List external links on a task; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskLinkListInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:read", app.PermissionTaskRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		tsk, err := resolveToolTaskRef(svc, in.Task, "task", false)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		linkViews := make([]taskLinkView, len(tsk.Links))
		for i, l := range tsk.Links {
			linkViews[i] = taskLinkViewFromApp(l)
		}
		return successWithEnvelope(map[string]any{"links": linkViews, "count": len(linkViews)}, fmt.Sprintf("%d link(s)", len(linkViews)))
	})
	addTool(s, opts, &mcp.Tool{Name: "task_export", Description: "Export tasks as xuanchu.task-bundle/v1 JSON; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskExportInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:read", app.PermissionTaskRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		bundle, err := svc.ExportTaskBundle()
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(
			map[string]any{"bundle": bundle, "task_count": len(bundle.Tasks), "series_count": len(bundle.TaskSeries)},
			fmt.Sprintf("exported %d task(s), %d series", len(bundle.Tasks), len(bundle.TaskSeries)),
		)
	})
	addTool(s, opts, &mcp.Tool{Name: "task_import", Description: "Import tasks from xuanchu.task-bundle/v1 JSON; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskImportInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:write", app.PermissionTaskWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		result, err := svc.ImportTaskBundle(in.Bundle)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(
			map[string]any{"imported_tasks": result.TasksImported, "imported_series": result.SeriesImported},
			fmt.Sprintf("imported %d task(s), %d series", result.TasksImported, result.SeriesImported),
		)
	})
}

func modifyTaskTool(ctx context.Context, req *mcp.CallToolRequest, opts Options, in TaskModifyInput, rendered string) (*mcp.CallToolResult, ToolEnvelope, error) {
	svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:write", app.PermissionTaskWrite)
	if err != nil {
		return businessErrorWithEnvelope(err)
	}
	resolved, err := resolveToolTaskRef(svc, in.ID, "id", true)
	if err != nil {
		return businessErrorWithEnvelope(err)
	}
	project, err := projectSlugForTask(svc, in.Project, in.ProjectID)
	if err != nil {
		return businessErrorWithEnvelope(err)
	}
	due, wait, scheduled, until, err := resolveToolDateFields(in.Due, in.DueDate, in.Wait, in.WaitDate, in.Scheduled, in.ScheduledDate, in.Until, in.UntilDate)
	if err != nil {
		return businessErrorWithEnvelope(err)
	}
	mod := app.ModifyInput{
		Title:           in.Title,
		Description:     in.Description,
		Project:         project,
		Priority:        in.Priority,
		Due:             due,
		Wait:            wait,
		Scheduled:       scheduled,
		Until:           until,
		AddTags:         in.Tags,
		AddAssignees:    in.Assignees,
		RemoveAssignees: in.RemoveAssignees,
		RemoveTags:      in.RemoveTags,
		UDAs:            in.UDAs,
		AddDepends:      in.Depends,
		ClearDepends:    in.ClearDepends,
	}
	if err := applyClearFields(in.Clear, &mod); err != nil {
		return businessErrorWithEnvelope(err)
	}
	if err := svc.Modify(resolved.UUID, mod); err != nil {
		return businessErrorWithEnvelope(err)
	}
	return taskAfterMutation(svc, resolved.UUID, rendered)
}

func taskActionHandler(opts Options, rendered string, fn func(*app.Service, string) error) mcp.ToolHandlerFor[TaskIDInput, ToolEnvelope] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in TaskIDInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:write", app.PermissionTaskWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		resolved, err := resolveToolTaskRef(svc, in.ID, "id", true)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		if err := fn(svc, resolved.UUID); err != nil {
			return businessErrorWithEnvelope(err)
		}
		return taskAfterMutation(svc, resolved.UUID, rendered)
	}
}

func taskAfterMutation(svc *app.Service, id, rendered string) (*mcp.CallToolResult, ToolEnvelope, error) {
	tsk, err := svc.Info(strings.TrimSpace(id))
	if err != nil {
		return businessErrorWithEnvelope(err)
	}
	data, err := taskData(tsk)
	if err != nil {
		return businessErrorWithEnvelope(err)
	}
	return successWithEnvelope(data, rendered)
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

func taskQueryStatus(in TaskQueryInput) string {
	status := strings.TrimSpace(in.Status)
	if status != "" {
		return status
	}
	switch {
	case in.IncludeCompleted && !in.IncludeDeleted:
		return task.StatusCompleted
	case in.IncludeDeleted && !in.IncludeCompleted:
		return task.StatusDeleted
	case in.IncludeCompleted && in.IncludeDeleted:
		return ""
	default:
		return ""
	}
}

func taskQueryExcludeDeleted(in TaskQueryInput) bool {
	status := strings.TrimSpace(in.Status)
	if status != "" {
		return false
	}
	return !in.IncludeDeleted
}

func taskQueryStatuses(in TaskQueryInput) []string {
	if strings.TrimSpace(in.Status) != "" || !in.IncludeCompleted || !in.IncludeDeleted {
		return nil
	}
	return []string{task.StatusCompleted, task.StatusDeleted}
}

func taskQueryList(svc *app.Service, input app.ListInput, statuses []string) ([]task.Task, error) {
	if len(statuses) == 0 {
		return svc.List(input)
	}
	rows := make([]task.Task, 0)
	for _, status := range statuses {
		statusInput := input
		statusInput.Status = status
		statusRows, err := svc.List(statusInput)
		if err != nil {
			return nil, err
		}
		rows = append(rows, statusRows...)
		if input.Limit > 0 && len(rows) >= input.Limit {
			return rows[:input.Limit], nil
		}
	}
	return rows, nil
}

func applyClearFields(fields []string, mod *app.ModifyInput) error {
	for _, field := range fields {
		field = strings.TrimSpace(field)
		switch field {
		case "project":
			mod.ClearProject = true
		case "priority":
			mod.ClearPriority = true
		case "description":
			mod.ClearDescription = true
		case "due":
			mod.ClearDue = true
		case "wait":
			mod.ClearWait = true
		case "scheduled":
			mod.ClearScheduled = true
		case "until":
			mod.ClearUntil = true
		case "assignees":
			mod.ClearAssignees = true
		default:
			if strings.HasPrefix(field, "uda.") {
				mod.ClearUDAs = append(mod.ClearUDAs, strings.TrimPrefix(field, "uda."))
				continue
			}
			return app.RuntimeError{Code: "task_clear_field_unknown", Message: "unknown clear field " + field}
		}
	}
	return nil
}

func successWithEnvelope(data any, rendered string) (*mcp.CallToolResult, ToolEnvelope, error) {
	result, envelope, err := successResult(data, rendered)
	return result, envelope, err
}

func businessErrorWithEnvelope(err error) (*mcp.CallToolResult, ToolEnvelope, error) {
	return businessErrorResult(err), ToolEnvelope{}, nil
}
