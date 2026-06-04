package mcpserver

import (
	"context"
	"fmt"

	"github.com/dajee/taskg/internal/app"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type WorkspaceListInput struct {
	IncludeArchived bool `json:"include_archived,omitempty"`
}

type WorkspaceCurrentInput struct {
	Workspace string `json:"workspace,omitempty"`
}

func registerWorkspaceTools(s *mcp.Server, opts Options) {
	addTool(s, &mcp.Tool{Name: "workspace_list", Description: "List visible workspaces; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in WorkspaceListInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{}, "workspace:read", app.PermissionWorkspaceRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		rows, err := svc.ListWorkspaces(in.IncludeArchived)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		data := map[string]any{"workspaces": workspaceViewsFromApp(rows), "count": len(rows)}
		return successWithEnvelope(data, fmt.Sprintf("%d workspace(s)", len(rows)))
	})

	addTool(s, &mcp.Tool{Name: "workspace_get_current", Description: "Read the effective workspace; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in WorkspaceCurrentInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace}, "workspace:read", app.PermissionWorkspaceRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		ref := in.Workspace
		if ref == "" {
			ref = svc.Runtime().WorkspaceSlug
		}
		view, err := svc.WorkspaceInfo(ref)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		data := map[string]any{"workspace": workspaceViewFromApp(view)}
		return successWithEnvelope(data, "workspace "+view.Slug)
	})
}
