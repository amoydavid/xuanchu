package mcpserver

import (
	"context"
	"strings"

	"github.com/dajee/taskg/internal/app"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ConfigGetInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Key       string `json:"key"`
	Scope     string `json:"scope,omitempty"`
}

type ConfigSetInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Key       string `json:"key"`
	Value     string `json:"value"`
	Scope     string `json:"scope"`
}

func registerConfigTools(s *mcp.Server, opts Options) {
	addTool(s, &mcp.Tool{Name: "config.get", Description: "Read workspace, project, or stdio local config."}, func(ctx context.Context, req *mcp.CallToolRequest, in ConfigGetInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		scope := normalizeConfigScope(in.Scope)
		key := strings.TrimSpace(in.Key)
		switch scope {
		case "local":
			if opts.Mode == ModeHTTP {
				return businessErrorWithEnvelope(app.RuntimeError{Code: "config_scope_invalid", Message: "local config is not available over HTTP MCP"})
			}
			value, ok := opts.LocalRuntimeValues[key]
			if !ok {
				return businessErrorWithEnvelope(app.RuntimeError{Code: "config_not_found", Message: "config not found"})
			}
			return configValueResult(key, value, scope)
		case "project":
			ref := projectRefForScope(in.Project, in.ProjectID)
			svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}, "project:read", app.PermissionProjectConfigRead)
			if err != nil {
				return businessErrorWithEnvelope(err)
			}
			value, ok, err := svc.ProjectConfigGet(ref, key)
			if err != nil {
				return businessErrorWithEnvelope(err)
			}
			if !ok {
				return businessErrorWithEnvelope(app.RuntimeError{Code: "config_not_found", Message: "config not found"})
			}
			return configValueResult(key, value, scope)
		default:
			if !app.IsBusinessConfigKey(key) || strings.HasPrefix(key, "agent.") {
				return businessErrorWithEnvelope(app.RuntimeError{Code: "config_key_unsupported", Message: "config key is not supported for workspace scope"})
			}
			svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace}, "config:read", app.PermissionWorkspaceRead)
			if err != nil {
				return businessErrorWithEnvelope(err)
			}
			value, ok, err := svc.GetConfig(key)
			if err != nil {
				return businessErrorWithEnvelope(err)
			}
			if !ok {
				return businessErrorWithEnvelope(app.RuntimeError{Code: "config_not_found", Message: "config not found"})
			}
			return configValueResult(key, value, "workspace")
		}
	})

	addTool(s, &mcp.Tool{Name: "config.set", Description: "Write workspace or project config; writes audit for shared config."}, func(ctx context.Context, req *mcp.CallToolRequest, in ConfigSetInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		scope := normalizeConfigScope(in.Scope)
		key := strings.TrimSpace(in.Key)
		switch scope {
		case "local":
			return businessErrorWithEnvelope(app.RuntimeError{Code: "config_scope_invalid", Message: "local config is not writable over MCP"})
		case "project":
			ref := projectRefForScope(in.Project, in.ProjectID)
			svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}, "project:write", app.PermissionProjectConfigWrite)
			if err != nil {
				return businessErrorWithEnvelope(err)
			}
			if err := svc.ProjectConfigSet(ref, key, in.Value); err != nil {
				return businessErrorWithEnvelope(err)
			}
			return configValueResult(key, in.Value, scope)
		default:
			if !app.IsBusinessConfigKey(key) || strings.HasPrefix(key, "agent.") {
				return businessErrorWithEnvelope(app.RuntimeError{Code: "config_key_unsupported", Message: "config key is not supported for workspace scope"})
			}
			svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace}, "config:write", app.PermissionWorkspaceModify)
			if err != nil {
				return businessErrorWithEnvelope(err)
			}
			if err := svc.SetConfig(key, in.Value); err != nil {
				return businessErrorWithEnvelope(err)
			}
			return configValueResult(key, in.Value, "workspace")
		}
	})
}

func normalizeConfigScope(scope string) string {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return "workspace"
	}
	return scope
}

func configValueResult(key, value, scope string) (*mcp.CallToolResult, ToolEnvelope, error) {
	data := map[string]any{"key": key, "value": value, "scope": scope}
	return successWithEnvelope(data, key+"="+value)
}
