package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ContextShowInput struct {
	Workspace string `json:"workspace,omitempty"`
	Name      string `json:"name,omitempty"`
}

type ContextSetInput struct {
	Workspace string `json:"workspace,omitempty"`
	Name      string `json:"name"`
}

type ContextDeleteInput struct {
	Workspace string `json:"workspace,omitempty"`
	Name      string `json:"name"`
}

func registerContextTools(s *mcp.Server, opts Options) {
	addTool(s, opts, &mcp.Tool{Name: "context_get", Description: "Show active or named context; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in ContextShowInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace}, "context:read", app.PermissionContextUse)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := contextViewFor(svc, in.Name)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"context": view}, "context "+view.Name)
	})

	addTool(s, opts, &mcp.Tool{Name: "context_set", Description: "Set active context; writes actor workspace state."}, func(ctx context.Context, req *mcp.CallToolRequest, in ContextSetInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace}, "context:write", app.PermissionContextUse)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		name := strings.TrimSpace(in.Name)
		if name == "none" || name == "" {
			if err := svc.ContextNone(); err != nil {
				return businessErrorWithEnvelope(err)
			}
			return successWithEnvelope(map[string]any{"context": nil}, "context none")
		}
		if err := svc.UseContext(name); err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := contextViewFor(svc, name)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"context": view}, "context "+name)
	})

	addTool(s, opts, &mcp.Tool{Name: "context_none", Description: "Clear the active context."}, func(ctx context.Context, req *mcp.CallToolRequest, in ContextShowInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace}, "config:write", app.PermissionContextManage)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		if err := svc.ContextNone(); err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"context": nil}, "context none")
	})

	addTool(s, opts, &mcp.Tool{Name: "context_list", Description: "List all contexts in the workspace; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in ContextShowInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace}, "config:read", app.PermissionContextUse)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		rows, err := svc.ContextList()
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		activeName, _, _ := svc.ActiveContextName()
		views := make([]contextView, len(rows))
		for i, row := range rows {
			views[i] = contextView{Name: row.Name, Filter: row.FilterSource, Active: row.Name == activeName, CreatedAt: row.CreatedAt, ModifiedAt: row.ModifiedAt}
		}
		data := map[string]any{"contexts": views, "count": len(views)}
		return successWithEnvelope(data, fmt.Sprintf("%d context(s)", len(views)))
	})

	addTool(s, opts, &mcp.Tool{Name: "context_delete", Description: "Delete a named context."}, func(ctx context.Context, req *mcp.CallToolRequest, in ContextDeleteInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace}, "config:write", app.PermissionContextManage)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		if err := svc.ContextDelete(in.Name); err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(nil, "deleted context "+in.Name)
	})
}

func contextViewFor(svc *app.Service, name string) (contextView, error) {
	activeName, _, err := svc.ActiveContextName()
	if err != nil {
		return contextView{}, err
	}
	if strings.TrimSpace(name) == "" {
		name = activeName
	}
	rows, err := svc.ContextList()
	if err != nil {
		return contextView{}, err
	}
	for _, row := range rows {
		if row.Name == name {
			return contextView{Name: row.Name, Filter: row.FilterSource, Active: row.Name == activeName, CreatedAt: row.CreatedAt, ModifiedAt: row.ModifiedAt}, nil
		}
	}
	if name == "" {
		return contextView{}, nil
	}
	return contextView{}, app.RuntimeError{Code: "context_not_found", Message: "context not found"}
}
