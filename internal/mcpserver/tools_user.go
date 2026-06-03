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

	addTool(s, &mcp.Tool{Name: "user_info", Description: "Read a single user by name, email, or UUID; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in UserInfoInput) (*mcp.CallToolResult, ToolEnvelope, error) {
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
}
