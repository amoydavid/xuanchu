package mcpserver

import (
	"context"
	"fmt"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/task"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type TokenListInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

func (in TokenListInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type TokenCreateInput struct {
	Workspace        string   `json:"workspace,omitempty"`
	Project          string   `json:"project,omitempty"`
	ProjectID        string   `json:"project_id,omitempty"`
	Name             string   `json:"name" jsonschema:"token name"`
	Scope            []string `json:"scope,omitempty" jsonschema:"token scopes"`
	ExpiresInSeconds *int64   `json:"expires_in_seconds,omitempty" jsonschema:"token expiration in seconds from now"`
}

func (in TokenCreateInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type TokenModifyInput struct {
	Workspace        string   `json:"workspace,omitempty"`
	Project          string   `json:"project,omitempty"`
	ProjectID        string   `json:"project_id,omitempty"`
	TokenRef         string   `json:"token_ref" jsonschema:"token ID or prefix"`
	Name             *string  `json:"name,omitempty"`
	Scope            []string `json:"scope,omitempty"`
	ExpiresInSeconds *int64   `json:"expires_in_seconds,omitempty"`
}

func (in TokenModifyInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type TokenRevokeInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	TokenRef  string `json:"token_ref" jsonschema:"token ID or prefix"`
}

func (in TokenRevokeInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

func registerTokenTools(s *mcp.Server, opts Options) {
	addTool(s, &mcp.Tool{Name: "token_list", Description: "List API tokens; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in TokenListInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "token:read", app.PermissionTokenRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		rows, err := svc.ListTokens(app.ListTokensInput{})
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		data := map[string]any{"tokens": tokenViewsFromApp(rows), "count": len(rows)}
		return successWithEnvelope(data, fmt.Sprintf("%d token(s)", len(rows)))
	})

	addTool(s, &mcp.Tool{Name: "token_create", Description: "Create an API token. The raw secret is returned only at creation time."}, func(ctx context.Context, req *mcp.CallToolRequest, in TokenCreateInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "token:write", app.PermissionTokenWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		var expiresIn *time.Duration
		if in.ExpiresInSeconds != nil {
			d := time.Duration(*in.ExpiresInSeconds) * time.Second
			expiresIn = &d
		}
		created, err := svc.CreateToken(app.CreateTokenInput{
			Name:      in.Name,
			Scopes:    in.Scope,
			ExpiresIn: expiresIn,
		})
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view := tokenViewFromApp(created.View)
		view["raw_token"] = created.RawToken
		data := map[string]any{"token": view}
		return successWithEnvelope(data, "token "+created.View.Name+" created")
	})

	addTool(s, &mcp.Tool{Name: "token_modify", Description: "Modify an API token's name, scope, or expiration."}, func(ctx context.Context, req *mcp.CallToolRequest, in TokenModifyInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "token:write", app.PermissionTokenWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		var expiresIn *time.Duration
		if in.ExpiresInSeconds != nil {
			d := time.Duration(*in.ExpiresInSeconds) * time.Second
			expiresIn = &d
		}
		result, err := svc.ModifyToken(app.ModifyTokenInput{
			TokenID:   in.TokenRef,
			Name:      in.Name,
			Scopes:    in.Scope,
			ExpiresIn: expiresIn,
		})
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		data := map[string]any{"token": tokenViewFromApp(*result)}
		return successWithEnvelope(data, "token "+result.Name+" updated")
	})

	addTool(s, &mcp.Tool{Name: "token_revoke", Description: "Revoke an API token."}, func(ctx context.Context, req *mcp.CallToolRequest, in TokenRevokeInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "token:write", app.PermissionTokenWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		if err := svc.RevokeToken(in.TokenRef); err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(nil, "token revoked")
	})
}

func tokenViewFromApp(v app.TokenView) map[string]any {
	out := map[string]any{
		"id":            v.ID,
		"prefix":        v.Prefix,
		"name":          v.Name,
		"type":          v.Type,
		"user":          task.UserInfoToJSON(v.User),
		"scopes":        v.Scopes,
		"workspace_ids": v.WorkspaceIDs,
		"project_ids":   v.ProjectIDs,
		"created_at":    v.CreatedAt,
	}
	if v.ExpiresAt != nil {
		out["expires_at"] = *v.ExpiresAt
	}
	if v.RevokedAt != nil {
		out["revoked_at"] = *v.RevokedAt
	}
	if v.LastUsedAt != nil {
		out["last_used_at"] = *v.LastUsedAt
	}
	return out
}

func tokenViewsFromApp(rows []app.TokenView) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, tokenViewFromApp(row))
	}
	return out
}
