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

type ReportRunInput struct {
	Workspace      string `json:"workspace,omitempty"`
	Project        string `json:"project,omitempty"`
	ProjectID      string `json:"project_id,omitempty"`
	Name           string `json:"name"`
	Query          string `json:"query,omitempty"`
	DueAfter       string `json:"due_after,omitempty" jsonschema:"YYYY-MM-DD"`
	DueBefore      string `json:"due_before,omitempty" jsonschema:"YYYY-MM-DD"`
	OccurrenceMode string `json:"occurrence_mode,omitempty" jsonschema:"auto|materialized|expand"`
	TaskType       string `json:"task_type,omitempty" jsonschema:"all|normal|occurrence"`
	Sort           string `json:"sort,omitempty"`
	Limit          int    `json:"limit,omitempty"`
	Offset         int    `json:"offset,omitempty"`
}

func (in ReportRunInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type UrgencyExplainInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	ID        string `json:"id" jsonschema:"task reference: UUID, materialized task_slug, or occurrence_ref; projected occurrences only have occurrence_ref"`
}

func (in UrgencyExplainInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

func registerReportTools(s *mcp.Server, opts Options) {
	addTool(s, opts, &mcp.Tool{Name: "report_run", Description: "Run a task report; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in ReportRunInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		limit, err := limitOrDefault(in.Limit)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:read", app.PermissionTaskRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		var queryExpr query.Expr
		if strings.TrimSpace(in.Query) != "" {
			expr, err := query.ParseFilterExpr([]string{in.Query})
			if err != nil {
				return businessErrorWithEnvelope(err)
			}
			queryExpr = expr
		}
		if in.Project != "" || in.ProjectID != "" {
			project, err := svc.ProjectInfo(projectRefForScope(in.Project, in.ProjectID))
			if err != nil {
				return businessErrorWithEnvelope(err)
			}
			queryExpr = query.And(queryExpr, query.Predicate{Attribute: query.AttrProjectID, Operator: query.OpEqual, Value: query.StringValue(project.ID)})
		}
		if taskType := strings.TrimSpace(in.TaskType); taskType != "" && taskType != "all" {
			if taskType != "normal" && taskType != "occurrence" {
				return businessErrorWithEnvelope(app.RuntimeError{Code: "task_type_invalid", Message: "task_type must be all|normal|occurrence"})
			}
			queryExpr = query.And(queryExpr, query.Predicate{Attribute: query.AttrTaskType, Operator: query.OpEqual, Value: query.StringValue(taskType)})
		}
		var dateRange *app.TaskViewRange
		if strings.TrimSpace(in.DueAfter) != "" {
			dueAfter, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(in.DueAfter), time.Local)
			if err != nil {
				return businessErrorWithEnvelope(app.RuntimeError{Code: "task_query_date_invalid", Message: "due_after must be YYYY-MM-DD"})
			}
			queryExpr = query.And(queryExpr, query.Or(
				query.Predicate{Attribute: query.AttrDue, Operator: query.OpEqual, Value: query.DateValue(in.DueAfter)},
				query.Predicate{Attribute: query.AttrDue, Operator: query.OpAfter, Value: query.DateValue(in.DueAfter)},
			))
			if strings.TrimSpace(in.DueBefore) != "" {
				dueBefore, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(in.DueBefore), time.Local)
				if err != nil {
					return businessErrorWithEnvelope(app.RuntimeError{Code: "task_query_date_invalid", Message: "due_before must be YYYY-MM-DD"})
				}
				end := dueBefore.AddDate(0, 0, 1)
				queryExpr = query.And(queryExpr, query.Predicate{Attribute: query.AttrDue, Operator: query.OpBefore, Value: query.DateValue(end.Format("2006-01-02"))})
				dateRange = &app.TaskViewRange{Start: dueAfter.Unix(), End: end.Unix()}
			}
		} else if strings.TrimSpace(in.DueBefore) != "" {
			dueBefore, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(in.DueBefore), time.Local)
			if err != nil {
				return businessErrorWithEnvelope(app.RuntimeError{Code: "task_query_date_invalid", Message: "due_before must be YYYY-MM-DD"})
			}
			end := dueBefore.AddDate(0, 0, 1)
			queryExpr = query.And(queryExpr, query.Predicate{Attribute: query.AttrDue, Operator: query.OpBefore, Value: query.DateValue(end.Format("2006-01-02"))})
		}
		page, err := svc.RunTaskViewReport(app.ReportViewInput{
			Name: strings.TrimSpace(in.Name), Query: queryExpr, Range: dateRange,
			OccurrenceMode: app.OccurrenceMode(strings.TrimSpace(in.OccurrenceMode)),
			Sort:           strings.TrimSpace(in.Sort), Limit: limit, Offset: in.Offset,
		})
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		items := make([]map[string]any, 0, len(page.Items))
		for _, item := range page.Items {
			items = append(items, occurrenceViewToMCPJSON(item))
		}
		data := map[string]any{
			"items": items, "total": page.Total, "limit": page.Limit,
			"offset": page.Offset, "occurrence_mode": page.OccurrenceMode,
		}
		if page.Range != nil {
			data["range"] = map[string]any{"start": page.Range.Start, "end": page.Range.End}
		}
		return successWithEnvelope(data, fmt.Sprintf("%d task(s)", page.Total))
	})

	addTool(s, opts, &mcp.Tool{Name: "urgency_explain", Description: "Explain task urgency; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in UrgencyExplainInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:read", app.PermissionTaskRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		if err := validateToolTaskRef(in.ID, "id"); err != nil {
			return businessErrorWithEnvelope(err)
		}
		explain, err := svc.ExplainUrgency(strings.TrimSpace(in.ID))
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		data := map[string]any{
			"id": explain.ID, "uuid": explain.UUID,
			"total": explain.Total, "items": explain.Items,
		}
		return successWithEnvelope(data, "urgency "+explain.ID)
	})
}
