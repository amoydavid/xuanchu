package mcpserver

import (
	"context"
	"fmt"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type WorkspaceListInput struct {
	IncludeArchived bool `json:"include_archived,omitempty" jsonschema:"include archived workspaces in the result"`
}

type WorkspaceCurrentInput struct {
	Workspace string `json:"workspace,omitempty"`
}

func registerWorkspaceTools(s *mcp.Server, opts Options) {
	addTool(s, opts, &mcp.Tool{Name: "workspace_list", Description: "List visible workspaces; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in WorkspaceListInput) (*mcp.CallToolResult, ToolEnvelope, error) {
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

	addTool(s, opts, &mcp.Tool{Name: "workspace_get_current", Description: "Read the effective workspace; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in WorkspaceCurrentInput) (*mcp.CallToolResult, ToolEnvelope, error) {
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

	addTool(s, opts, &mcp.Tool{Name: "workspace_add", Description: "Create a new workspace."}, func(ctx context.Context, req *mcp.CallToolRequest, in WorkspaceAddInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{}, "workspace:write", app.PermissionWorkspaceModify)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.AddWorkspace(app.AddWorkspaceInput{
			Slug:        in.Slug,
			Name:        in.Name,
			Description: in.Description,
			Visibility:  in.Visibility,
		})
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		data := map[string]any{"workspace": workspaceViewFromApp(view)}
		return successWithEnvelope(data, "workspace "+view.Slug+" created")
	})

	addTool(s, opts, &mcp.Tool{Name: "workspace_info", Description: "Get workspace details."}, func(ctx context.Context, req *mcp.CallToolRequest, in WorkspaceRefInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace}, "workspace:read", app.PermissionWorkspaceRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.WorkspaceInfo(in.Workspace)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		data := map[string]any{"workspace": workspaceViewFromApp(view)}
		return successWithEnvelope(data, "workspace "+view.Slug)
	})

	addTool(s, opts, &mcp.Tool{Name: "workspace_modify", Description: "Modify workspace name, description, or visibility."}, func(ctx context.Context, req *mcp.CallToolRequest, in WorkspaceModifyInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace}, "workspace:write", app.PermissionWorkspaceModify)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		err = svc.ModifyWorkspace(in.Workspace, app.ModifyWorkspaceInput{
			Name:        in.Name,
			Description: in.Description,
			Visibility:  in.Visibility,
		})
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.WorkspaceInfo(in.Workspace)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		data := map[string]any{"workspace": workspaceViewFromApp(view)}
		return successWithEnvelope(data, "workspace "+view.Slug+" updated")
	})

	addTool(s, opts, &mcp.Tool{Name: "workspace_archive", Description: "Archive a workspace."}, func(ctx context.Context, req *mcp.CallToolRequest, in WorkspaceRefInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace}, "workspace:write", app.PermissionWorkspaceArchive)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		err = svc.ArchiveWorkspace(in.Workspace)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.WorkspaceInfo(in.Workspace)
		if err != nil {
			return successWithEnvelope(nil, "workspace archived")
		}
		return successWithEnvelope(map[string]any{"workspace": workspaceViewFromApp(view)}, "workspace archived")
	})

	addTool(s, opts, &mcp.Tool{Name: "workspace_use", Description: "Switch active workspace."}, func(ctx context.Context, req *mcp.CallToolRequest, in WorkspaceRefInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace}, "workspace:write", app.PermissionWorkspaceModify)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		err = svc.UseWorkspace(in.Workspace)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.WorkspaceInfo(in.Workspace)
		if err != nil {
			return successWithEnvelope(nil, "workspace switched to "+in.Workspace)
		}
		return successWithEnvelope(map[string]any{"workspace": workspaceViewFromApp(view)}, "workspace switched to "+in.Workspace)
	})
}

type WorkspaceAddInput struct {
	Slug        string `json:"slug" jsonschema:"unique workspace slug"`
	Name        string `json:"name,omitempty" jsonschema:"human-readable workspace name"`
	Description string `json:"description,omitempty" jsonschema:"workspace description"`
	Visibility  string `json:"visibility,omitempty" jsonschema:"visibility: public or private (default private)"`
}

type WorkspaceRefInput struct {
	Workspace string `json:"workspace" jsonschema:"workspace slug or UUID"`
}

type WorkspaceModifyInput struct {
	Workspace   string  `json:"workspace" jsonschema:"workspace slug or UUID"`
	Name        *string `json:"name,omitempty" jsonschema:"new human-readable workspace name"`
	Description *string `json:"description,omitempty" jsonschema:"new workspace description; pass empty string to clear"`
	Visibility  *string `json:"visibility,omitempty" jsonschema:"new visibility: public or private"`
}
