package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/query"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type TaskAddInput struct {
	Workspace     string   `json:"workspace,omitempty"`
	Project       string   `json:"project,omitempty"`
	ProjectID     string   `json:"project_id,omitempty"`
	Title         string   `json:"title" jsonschema:"task title"`
	Description   *string  `json:"description,omitempty" jsonschema:"task details"`
	Tags          []string `json:"tags,omitempty" jsonschema:"task tags to add"`
	Assignees     []string `json:"assignees,omitempty" jsonschema:"workspace user refs to assign"`
	Priority      string   `json:"priority,omitempty" jsonschema:"task priority: H, M, or L (default M)"`
	Due           *int64   `json:"due,omitempty" jsonschema:"deadline time, unix seconds"`
	DueDate       string   `json:"due_date,omitempty" jsonschema:"deadline date as YYYY-MM-DD; stored at local 23:59:59"`
	Wait          *int64   `json:"wait,omitempty" jsonschema:"defer-until time, unix seconds"`
	WaitDate      string   `json:"wait_date,omitempty" jsonschema:"defer-until date as YYYY-MM-DD; stored at local 00:00:00"`
	Scheduled     *int64   `json:"scheduled,omitempty" jsonschema:"scheduled start time, unix seconds"`
	ScheduledDate string   `json:"scheduled_date,omitempty" jsonschema:"scheduled start date as YYYY-MM-DD; stored at local 00:00:00"`
	Until         *int64   `json:"until,omitempty" jsonschema:"effective-until time, unix seconds"`
	UntilDate     string   `json:"until_date,omitempty" jsonschema:"effective-until date as YYYY-MM-DD; stored at local 23:59:59"`
	Annotations   []string `json:"annotations,omitempty" jsonschema:"initial annotations"`
}

func (in TaskAddInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type TaskQueryInput struct {
	Workspace      string `json:"workspace,omitempty"`
	Project        string `json:"project,omitempty"`
	ProjectID      string `json:"project_id,omitempty"`
	Query          string `json:"query,omitempty" jsonschema:"task filter expression; an explicit status predicate overrides default visibility"`
	Status         string `json:"status,omitempty" jsonschema:"explicit task status filter; overrides default non-deleted visibility"`
	Sort           string `json:"sort,omitempty" jsonschema:"task sort expression, for example due, due-, urgency-"`
	Limit          int    `json:"limit,omitempty" jsonschema:"max tasks to return (default 200)"`
	Offset         int    `json:"offset,omitempty" jsonschema:"number of tasks to skip"`
	IncludeDeleted bool   `json:"include_deleted,omitempty" jsonschema:"include deleted tasks in addition to the default non-deleted set; ignored when status is explicit"`
	// occurrence 查询参数（spec §13.3）。
	OccurrenceMode string `json:"occurrence_mode,omitempty" jsonschema:"auto|materialized|expand"`
	DueAfter       string `json:"due_after,omitempty" jsonschema:"YYYY-MM-DD"`
	DueBefore      string `json:"due_before,omitempty" jsonschema:"YYYY-MM-DD"`
	TaskType       string `json:"task_type,omitempty" jsonschema:"all|normal|occurrence"`
}

func (in TaskQueryInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type TaskGetInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	ID        string `json:"id" jsonschema:"task reference: UUID, materialized task_slug, or occurrence_ref; projected occurrences only have occurrence_ref"`
}

func (in TaskGetInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type TaskModifyInput struct {
	Workspace       string            `json:"workspace,omitempty"`
	Project         string            `json:"project,omitempty"`
	ProjectID       string            `json:"project_id,omitempty"`
	ID              string            `json:"id" jsonschema:"task reference: UUID, materialized task_slug, or occurrence_ref; projected occurrences only have occurrence_ref"`
	Title           *string           `json:"title,omitempty" jsonschema:"new task title"`
	Description     *string           `json:"description,omitempty" jsonschema:"new task details; pass empty string to clear"`
	Priority        *string           `json:"priority,omitempty" jsonschema:"new task priority: H, M, or L"`
	Due             *int64            `json:"due,omitempty" jsonschema:"new deadline time, unix seconds"`
	DueDate         string            `json:"due_date,omitempty" jsonschema:"deadline date as YYYY-MM-DD; stored at local 23:59:59"`
	Wait            *int64            `json:"wait,omitempty" jsonschema:"new defer-until time, unix seconds"`
	WaitDate        string            `json:"wait_date,omitempty" jsonschema:"defer-until date as YYYY-MM-DD; stored at local 00:00:00"`
	Scheduled       *int64            `json:"scheduled,omitempty" jsonschema:"new scheduled start time, unix seconds"`
	ScheduledDate   string            `json:"scheduled_date,omitempty" jsonschema:"scheduled start date as YYYY-MM-DD; stored at local 00:00:00"`
	Until           *int64            `json:"until,omitempty" jsonschema:"new effective-until time, unix seconds"`
	UntilDate       string            `json:"until_date,omitempty" jsonschema:"effective-until date as YYYY-MM-DD; stored at local 23:59:59"`
	Tags            []string          `json:"tags,omitempty" jsonschema:"task tags to add"`
	Assignees       []string          `json:"assignees,omitempty" jsonschema:"workspace user refs to assign"`
	RemoveAssignees []string          `json:"remove_assignees,omitempty" jsonschema:"workspace user refs to unassign"`
	RemoveTags      []string          `json:"remove_tags,omitempty" jsonschema:"task tags to remove"`
	UDAs            map[string]string `json:"udas,omitempty" jsonschema:"user-defined attribute overrides"`
	Clear           []string          `json:"clear,omitempty" jsonschema:"fields to clear: title, description, priority, due, wait, scheduled, until"`
	Depends         []string          `json:"depends,omitempty" jsonschema:"task refs this task depends on (replaces existing)"`
	ClearDepends    bool              `json:"clear_depends,omitempty" jsonschema:"remove all dependency links"`
}

func (in TaskModifyInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type TaskIDInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	ID        string `json:"id" jsonschema:"task reference: UUID, materialized task_slug, or occurrence_ref; projected occurrences only have occurrence_ref"`
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
	ID         string `json:"id" jsonschema:"task reference: UUID, materialized task_slug, or occurrence_ref; projected occurrences only have occurrence_ref"`
	Annotation string `json:"annotation" jsonschema:"annotation body text"`
}

func (in TaskAnnotateInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type TaskDependsInput struct {
	Workspace    string   `json:"workspace,omitempty"`
	Project      string   `json:"project,omitempty"`
	ProjectID    string   `json:"project_id,omitempty"`
	ID           string   `json:"id" jsonschema:"task reference: UUID, materialized task_slug, or occurrence_ref; projected occurrences only have occurrence_ref"`
	Depends      []string `json:"depends,omitempty" jsonschema:"task refs this task depends on (replaces existing)"`
	ClearDepends bool     `json:"clear_depends,omitempty" jsonschema:"remove all dependency links"`
}

type TaskLinkAddInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Task      string `json:"task" jsonschema:"task reference: UUID, materialized task_slug, or occurrence_ref; projected occurrences only have occurrence_ref"`
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
	Task      string `json:"task" jsonschema:"task reference: UUID, materialized task_slug, or occurrence_ref; projected occurrences only have occurrence_ref"`
	LinkID    string `json:"link_id" jsonschema:"link ID to remove"`
}

func (in TaskLinkRemoveInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type TaskDenotateInput struct {
	Workspace    string `json:"workspace,omitempty"`
	Project      string `json:"project,omitempty"`
	ProjectID    string `json:"project_id,omitempty"`
	ID           string `json:"id" jsonschema:"task reference: UUID, materialized task_slug, or occurrence_ref; projected occurrences only have occurrence_ref"`
	AnnotationID string `json:"annotation_id" jsonschema:"annotation ID to remove"`
}

func (in TaskDenotateInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type TaskLinkListInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Task      string `json:"task" jsonschema:"task reference: UUID, materialized task_slug, or occurrence_ref; projected occurrences only have occurrence_ref"`
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
	Bundle app.TaskBundleV1 `json:"bundle" jsonschema:"exported task bundle; shape {schema, exported_at, task_series, tasks}"`
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
		created, err := svc.AddTaskViewWithAnnotations(app.AddInput{
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
		data := taskViewToMCPJSON(created)
		return successWithEnvelope(data, "Created task "+created.ID)
	})

	addTool(s, opts, &mcp.Tool{Name: "task_query", Description: "Query tasks; read-only. Defaults to all non-deleted tasks unless status is explicit."}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskQueryInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		limit, err := limitOrDefault(in.Limit)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:read", app.PermissionTaskRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		input := app.TaskViewQuery{
			Status: strings.TrimSpace(in.Status), Sort: strings.TrimSpace(in.Sort),
			Limit: limit, Offset: in.Offset,
			OccurrenceMode: app.OccurrenceMode(strings.TrimSpace(in.OccurrenceMode)),
		}
		if strings.TrimSpace(in.Query) != "" {
			expr, err := query.ParseFilterExpr([]string{in.Query})
			if err != nil {
				return businessErrorWithEnvelope(err)
			}
			input.Query = expr
		}
		hasExplicitStatus := input.Status != "" || query.ReferencesAttribute(input.Query, query.AttrStatus)
		if in.Project != "" || in.ProjectID != "" {
			project, err := svc.ProjectInfo(projectRefForScope(in.Project, in.ProjectID))
			if err != nil {
				return businessErrorWithEnvelope(err)
			}
			input.Query = query.And(input.Query, query.Predicate{Attribute: query.AttrProjectID, Operator: query.OpEqual, Value: query.StringValue(project.ID)})
		}
		if !hasExplicitStatus && in.IncludeDeleted {
			// 显式的非空状态谓词用于表达“所有状态”，并阻止 App 注入默认非删除条件。
			input.Query = query.And(input.Query, query.Predicate{Attribute: query.AttrStatus, Operator: query.OpNotNull})
		}
		if taskType := strings.TrimSpace(in.TaskType); taskType != "" && taskType != "all" {
			if taskType != "normal" && taskType != "occurrence" {
				return businessErrorWithEnvelope(app.RuntimeError{Code: "task_type_invalid", Message: "task_type must be all|normal|occurrence"})
			}
			input.Query = query.And(input.Query, query.Predicate{Attribute: query.AttrTaskType, Operator: query.OpEqual, Value: query.StringValue(taskType)})
		}
		if strings.TrimSpace(in.DueAfter) != "" || strings.TrimSpace(in.DueBefore) != "" {
			if dueAfter := strings.TrimSpace(in.DueAfter); dueAfter != "" {
				if _, err := time.Parse("2006-01-02", dueAfter); err != nil {
					return businessErrorWithEnvelope(app.RuntimeError{Code: "task_query_date_invalid", Message: "due_after must be YYYY-MM-DD"})
				}
				input.Query = query.And(input.Query, query.Or(
					query.Predicate{Attribute: query.AttrDue, Operator: query.OpEqual, Value: query.DateValue(dueAfter)},
					query.Predicate{Attribute: query.AttrDue, Operator: query.OpAfter, Value: query.DateValue(dueAfter)},
				))
			}
			if dueBefore := strings.TrimSpace(in.DueBefore); dueBefore != "" {
				parsed, err := time.Parse("2006-01-02", dueBefore)
				if err != nil {
					return businessErrorWithEnvelope(app.RuntimeError{Code: "task_query_date_invalid", Message: "due_before must be YYYY-MM-DD"})
				}
				input.Query = query.And(input.Query, query.Predicate{
					Attribute: query.AttrDue, Operator: query.OpBefore,
					Value: query.DateValue(parsed.AddDate(0, 0, 1).Format("2006-01-02")),
				})
			}
		}
		if in.DueAfter != "" && in.DueBefore != "" {
			start, err := resolveToolDateField("due_after", nil, in.DueAfter, false)
			if err != nil {
				return businessErrorWithEnvelope(err)
			}
			endOfDay, err := resolveToolDateField("due_before", nil, in.DueBefore, true)
			if err != nil {
				return businessErrorWithEnvelope(err)
			}
			input.Range = &app.TaskViewRange{Start: *start, End: *endOfDay + 1}
		}
		page, err := svc.QueryTaskViews(input)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		items := make([]map[string]any, 0, len(page.Items))
		for _, item := range page.Items {
			items = append(items, occurrenceViewToMCPJSON(item))
		}
		data := map[string]any{
			"items": items, "tasks": items, "total": page.Total, "count": page.Total,
			"limit": page.Limit, "offset": page.Offset, "occurrence_mode": page.OccurrenceMode,
		}
		if page.Range != nil {
			data["range"] = map[string]any{"start": page.Range.Start, "end": page.Range.End}
		}
		return successWithEnvelope(data, fmt.Sprintf("%d task(s)", page.Total))
	})

	addTool(s, opts, &mcp.Tool{Name: "task_get", Description: "Get one task; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskGetInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:read", app.PermissionTaskRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		if err := validateToolTaskRef(in.ID, "id"); err != nil {
			return businessErrorWithEnvelope(err)
		}
		resolved, err := svc.ResolveTaskReferenceForRead(strings.TrimSpace(in.ID))
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		rendered := formatOccurrenceViewText(resolved.View)
		if resolved.Kind == app.TaskResourceNormal && resolved.Task != nil {
			rendered = renderTaskInfo(*resolved.Task)
		}
		return successWithEnvelope(taskResolutionToMCPJSON(resolved), rendered)
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
		if err := validateToolTaskRef(in.ID, "id"); err != nil {
			return businessErrorWithEnvelope(err)
		}
		if err := svc.Annotate(strings.TrimSpace(in.ID), strings.TrimSpace(in.Annotation)); err != nil {
			return businessErrorWithEnvelope(err)
		}
		return taskAfterMutation(svc, in.ID, "annotated task")
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
		if err := validateToolTaskRef(in.Task, "task"); err != nil {
			return businessErrorWithEnvelope(err)
		}
		link, err := svc.TaskAddLink(strings.TrimSpace(in.Task), strings.TrimSpace(in.Type), strings.TrimSpace(in.URL), strings.TrimSpace(in.Title))
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
		if err := validateToolTaskRef(in.Task, "task"); err != nil {
			return businessErrorWithEnvelope(err)
		}
		if err := svc.TaskRemoveLink(strings.TrimSpace(in.Task), strings.TrimSpace(in.LinkID)); err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(nil, "Removed link from task")
	})
	addTool(s, opts, &mcp.Tool{Name: "task_denotate", Description: "Remove an annotation from a task; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskDenotateInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:write", app.PermissionTaskWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		if err := validateToolTaskRef(in.ID, "id"); err != nil {
			return businessErrorWithEnvelope(err)
		}
		if err := svc.Denotate(strings.TrimSpace(in.ID), in.AnnotationID); err != nil {
			return businessErrorWithEnvelope(err)
		}
		return taskAfterMutation(svc, in.ID, "removed annotation")
	})
	addTool(s, opts, &mcp.Tool{Name: "task_link_list", Description: "List external links on a task; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in TaskLinkListInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:read", app.PermissionTaskRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		if err := validateToolTaskRef(in.Task, "task"); err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.GetTaskView(strings.TrimSpace(in.Task))
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		linkViews := make([]taskLinkView, len(view.Links))
		for i, l := range view.Links {
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
	if err := validateToolTaskRef(in.ID, "id"); err != nil {
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
		if err := validateToolTaskRef(in.ID, "id"); err != nil {
			return businessErrorWithEnvelope(err)
		}
		if err := fn(svc, strings.TrimSpace(in.ID)); err != nil {
			return businessErrorWithEnvelope(err)
		}
		return taskAfterMutation(svc, in.ID, rendered)
	}
}

func taskAfterMutation(svc *app.Service, id, rendered string) (*mcp.CallToolResult, ToolEnvelope, error) {
	resolved, err := svc.ResolveTaskReferenceForRead(strings.TrimSpace(id))
	if err != nil {
		return businessErrorWithEnvelope(err)
	}
	return successWithEnvelope(taskResolutionToMCPJSON(resolved), rendered)
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
