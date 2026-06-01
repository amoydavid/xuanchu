package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/dajee/taskg/internal/app"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ProjectListInput struct {
	Workspace       string `json:"workspace,omitempty"`
	IncludeArchived bool   `json:"include_archived,omitempty"`
}

type ProjectGetInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

type ProjectCurrentInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

func registerProjectTools(s *mcp.Server, opts Options) {
	addTool(s, &mcp.Tool{Name: "project.list", Description: "List projects in the effective workspace; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectListInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace}, "project:read", app.PermissionProjectRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		rows, err := svc.ListProjects(in.IncludeArchived)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		data := map[string]any{"projects": projectViewsFromApp(rows), "count": len(rows)}
		return successWithEnvelope(data, fmt.Sprintf("%d project(s)", len(rows)))
	})

	addTool(s, &mcp.Tool{Name: "project.get", Description: "Get one project and its agent-readable config; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectGetInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		ref := projectRefForScope(in.Project, in.ProjectID)
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}, "project:read", app.PermissionProjectRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		project, err := svc.ProjectInfo(ref)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		config, _ := filterAgentConfig(svc, ref)
		data := map[string]any{"project": projectViewFromApp(project), "config_summary": config}
		return successWithEnvelope(data, "project "+project.Slug)
	})

	addTool(s, &mcp.Tool{Name: "project.current", Description: "Read explicit effective project scope; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectCurrentInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		ref := projectRefForScope(in.Project, in.ProjectID)
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}, "project:read", app.PermissionProjectRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		if strings.TrimSpace(ref) == "" {
			return successWithEnvelope(map[string]any{"project": nil}, "no effective project")
		}
		project, err := svc.ProjectInfo(ref)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"project": projectViewFromApp(project)}, "project "+project.Slug)
	})
}
