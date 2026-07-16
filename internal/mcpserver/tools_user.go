package mcpserver

import (
	"context"
	"fmt"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/task"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type UserListInput struct{}

type UserInfoInput struct {
	User string `json:"user" jsonschema:"user name, email, or UUID"`
}

type UserBindInput struct {
	User       string `json:"user" jsonschema:"user name, email, or UUID"`
	Provider   string `json:"provider" jsonschema:"external ID provider, e.g. feishu, wecom, dingtalk"`
	UserType   string `json:"user_type,omitempty" jsonschema:"ID type within the provider, e.g. user_id, open_id, union_id. Defaults to user_id"`
	ExternalID string `json:"external_id" jsonschema:"external ID value"`
}

type UserUnbindInput struct {
	User       string `json:"user" jsonschema:"user name, email, or UUID"`
	Provider   string `json:"provider" jsonschema:"external ID provider, e.g. feishu, wecom, dingtalk"`
	ExternalID string `json:"external_id" jsonschema:"external ID value"`
}

type UserAddInput struct {
	Name        string `json:"name" jsonschema:"login name, unique within the instance"`
	DisplayName string `json:"display_name,omitempty" jsonschema:"human-readable display name"`
	Email       string `json:"email,omitempty" jsonschema:"user email"`
}

type UserUseInput struct {
	User string `json:"user" jsonschema:"user name, email, or UUID"`
}

type UserRefInput struct {
	User string `json:"user" jsonschema:"user name, email, or UUID"`
}

func registerUserTools(s *mcp.Server, opts Options) {
	addTool(s, opts, &mcp.Tool{Name: "user_list", Description: "List all users; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in UserListInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{}, "user:read", app.PermissionWorkspaceRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		rows, err := svc.ListUsers()
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		data := map[string]any{"users": userViewsFromApp(rows), "count": len(rows)}
		return successWithEnvelope(data, fmt.Sprintf("%d user(s)", len(rows)))
	})

	addTool(s, opts, &mcp.Tool{Name: "user_get", Description: "Read a single user by name, email, or UUID; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in UserInfoInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{}, "user:read", app.PermissionWorkspaceRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.UserInfo(in.User)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		data := map[string]any{"user": userViewFromApp(view)}
		return successWithEnvelope(data, "user "+view.Name)
	})

	addTool(s, opts, &mcp.Tool{Name: "user_bind", Description: "Bind an external ID to a xuanchu user. Use provider + user_type to describe the ID (e.g. provider=feishu, user_type=user_id). Admin/owner can bind for others; regular users can only bind to themselves."}, func(ctx context.Context, req *mcp.CallToolRequest, in UserBindInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{}, "user:write", app.PermissionWorkspaceModify)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		user, err := svc.UserInfo(in.User)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		if err := svc.BindExternalID(user.ID, in.Provider, in.UserType, in.ExternalID); err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(
			map[string]any{"provider": in.Provider, "user_type": in.UserType, "external_id": in.ExternalID},
			fmt.Sprintf("Bound %s:%s to %s", in.Provider, in.ExternalID, user.Name),
		)
	})

	addTool(s, opts, &mcp.Tool{Name: "user_unbind", Description: "Unbind an external ID from a xuanchu user."}, func(ctx context.Context, req *mcp.CallToolRequest, in UserUnbindInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{}, "user:write", app.PermissionWorkspaceModify)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		user, err := svc.UserInfo(in.User)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		if err := svc.UnbindExternalID(user.ID, in.Provider, in.ExternalID); err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(nil, fmt.Sprintf("Unbound %s:%s from %s", in.Provider, in.ExternalID, user.Name))
	})

	addTool(s, opts, &mcp.Tool{Name: "user_add", Description: "Create a new user. User actors also create a personal workspace; tenant actors only create the user."}, func(ctx context.Context, req *mcp.CallToolRequest, in UserAddInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{}, "user:write", app.PermissionWorkspaceModify)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.AddUser(app.AddUserInput{Name: in.Name, DisplayName: in.DisplayName, Email: in.Email})
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		data := map[string]any{"user": userViewFromApp(view)}
		return successWithEnvelope(data, "user "+view.Name)
	})

	addTool(s, opts, &mcp.Tool{Name: "user_use", Description: "Switch the active user for subsequent operations."}, func(ctx context.Context, req *mcp.CallToolRequest, in UserUseInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{}, "user:write", app.PermissionWorkspaceModify)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		if err := svc.UseUser(in.User); err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"user": in.User}, "switched to "+in.User)
	})

	addTool(s, opts, &mcp.Tool{Name: "user_list_external_ids", Description: "List external IDs bound to a user; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in UserRefInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{}, "user:read", app.PermissionWorkspaceRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		user, err := svc.UserInfo(in.User)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		extIDs, err := svc.ListExternalIDs(user.ID)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		views := make([]task.JSONExternalID, len(extIDs))
		for i, eid := range extIDs {
			views[i] = task.JSONExternalID{Provider: eid.Provider, UserType: eid.UserType, ExternalID: eid.ExternalID}
		}
		data := map[string]any{"external_ids": views, "count": len(views)}
		return successWithEnvelope(data, fmt.Sprintf("%d external ID(s)", len(views)))
	})
}
