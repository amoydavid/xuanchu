package mcpserver

import (
	"context"
	"fmt"

	"github.com/dajee/taskg/internal/app"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type UserListInput struct{}

type UserInfoInput struct {
	User string `json:"user"`
}

type UserBindInput struct {
	User       string `json:"user"`
	Provider   string `json:"provider"`
	ExternalID string `json:"external_id"`
}

type UserUnbindInput struct {
	User       string `json:"user"`
	Provider   string `json:"provider"`
	ExternalID string `json:"external_id"`
}

type UserAddInput struct {
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}

type UserUseInput struct {
	User string `json:"user"`
}

type UserRefInput struct {
	User string `json:"user"`
}

func registerUserTools(s *mcp.Server, opts Options) {
	addTool(s, &mcp.Tool{Name: "user_list", Description: "List all users; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in UserListInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{}, "workspace:read", app.PermissionWorkspaceRead)
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

	addTool(s, &mcp.Tool{Name: "user_get", Description: "Read a single user by name, email, or UUID; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in UserInfoInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{}, "workspace:read", app.PermissionWorkspaceRead)
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

	addTool(s, &mcp.Tool{Name: "user_bind", Description: "Bind an external ID (e.g. feishu:ou_xxxxx) to a taskg user. Admin/owner can bind for others; regular users can only bind to themselves."}, func(ctx context.Context, req *mcp.CallToolRequest, in UserBindInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{}, "workspace:write", app.PermissionWorkspaceModify)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		user, err := svc.UserInfo(in.User)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		if err := svc.BindExternalID(user.ID, in.Provider, in.ExternalID); err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(
			map[string]any{"provider": in.Provider, "external_id": in.ExternalID},
			fmt.Sprintf("Bound %s:%s to %s", in.Provider, in.ExternalID, user.Name),
		)
	})

	addTool(s, &mcp.Tool{Name: "user_unbind", Description: "Unbind an external ID from a taskg user."}, func(ctx context.Context, req *mcp.CallToolRequest, in UserUnbindInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{}, "workspace:write", app.PermissionWorkspaceModify)
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

	addTool(s, &mcp.Tool{Name: "user_add", Description: "Create a new user with a personal workspace."}, func(ctx context.Context, req *mcp.CallToolRequest, in UserAddInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{}, "workspace:write", app.PermissionWorkspaceModify)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.AddUser(app.AddUserInput{Name: in.Name, Email: in.Email})
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		data := map[string]any{"user": userViewFromApp(view)}
		return successWithEnvelope(data, "user "+view.Name)
	})

	addTool(s, &mcp.Tool{Name: "user_use", Description: "Switch the active user for subsequent operations."}, func(ctx context.Context, req *mcp.CallToolRequest, in UserUseInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{}, "workspace:write", app.PermissionWorkspaceModify)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		if err := svc.UseUser(in.User); err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"user": in.User}, "switched to "+in.User)
	})

	addTool(s, &mcp.Tool{Name: "user_list_external_ids", Description: "List external IDs bound to a user; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in UserRefInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{}, "workspace:read", app.PermissionWorkspaceRead)
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
		views := make([]externalIDView, len(extIDs))
		for i, eid := range extIDs {
			views[i] = externalIDView{Provider: eid.Provider, ExternalID: eid.ExternalID}
		}
		data := map[string]any{"external_ids": views, "count": len(views)}
		return successWithEnvelope(data, fmt.Sprintf("%d external ID(s)", len(views)))
	})
}
