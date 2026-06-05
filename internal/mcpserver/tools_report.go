package mcpserver

import (
	"context"
	"strings"

	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/query"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ReportRunInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Name      string `json:"name"`
	Query     string `json:"query,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`
}

func (in ReportRunInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type UrgencyExplainInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	ID        string `json:"id"`
}

func (in UrgencyExplainInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

func registerReportTools(s *mcp.Server, opts Options) {
	addTool(s, &mcp.Tool{Name: "report_run", Description: "Run a task report; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in ReportRunInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		limit, err := limitOrDefault(in.Limit)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:read", app.PermissionTaskRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		input := app.ListInput{Limit: limit, Offset: in.Offset}
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
		rows, err := svc.ListReport(strings.TrimSpace(in.Name), input)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		data := tasksData(rows)
		data["report"] = map[string]any{"name": strings.TrimSpace(in.Name)}
		return successWithEnvelope(data, renderTaskList(rows))
	})

	addTool(s, &mcp.Tool{Name: "urgency_explain", Description: "Explain task urgency; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in UrgencyExplainInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		if err := requireUUID(in.ID, "id"); err != nil {
			return businessErrorWithEnvelope(err)
		}
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "task:read", app.PermissionTaskRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		explain, err := svc.ExplainUrgency(strings.TrimSpace(in.ID))
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		data := map[string]any{
			"urgency": explain.Total,
			"factors": explain.Items,
			"uuid":    explain.UUID,
		}
		return successWithEnvelope(data, "urgency "+explain.UUID)
	})
}
