package mcpserver

import (
	"context"
	"fmt"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type MemberListInput struct {
	Workspace string `json:"workspace,omitempty"`
}

type MemberAddInput struct {
	Workspace string `json:"workspace,omitempty"`
	User      string `json:"user"`
	Role      string `json:"role,omitempty"`
}

type MemberRoleInput struct {
	Workspace string `json:"workspace,omitempty"`
	User      string `json:"user"`
	Role      string `json:"role"`
}

func registerMemberTools(s *mcp.Server, opts Options) {
	addTool(s, opts, &mcp.Tool{Name: "member_list", Description: "List members in the effective workspace; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in MemberListInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace}, "workspace:read", app.PermissionWorkspaceRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		wsRef := in.Workspace
		if wsRef == "" {
			wsRef = svc.Runtime().WorkspaceSlug
		}
		rows, err := svc.ListMembers(wsRef)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		data := map[string]any{"members": memberViewsFromApp(rows), "count": len(rows)}
		return successWithEnvelope(data, fmt.Sprintf("%d member(s)", len(rows)))
	})

	addTool(s, opts, &mcp.Tool{Name: "member_add", Description: "Add a user to the effective workspace as a member."}, func(ctx context.Context, req *mcp.CallToolRequest, in MemberAddInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace}, "workspace:write", app.PermissionMemberManage)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		wsRef := in.Workspace
		if wsRef == "" {
			wsRef = svc.Runtime().WorkspaceSlug
		}
		role := app.Role(in.Role)
		if role == "" {
			role = app.RoleMember
		}
		if err := svc.AddMember(app.AddMemberInput{
			WorkspaceRef: wsRef,
			UserRef:      in.User,
			Role:         role,
		}); err != nil {
			return businessErrorWithEnvelope(err)
		}
		data := map[string]any{"user": in.User, "role": string(role)}
		if userView, err := svc.UserInfo(in.User); err == nil {
			data["member"] = memberViewFromApp(app.MemberView{UserID: userView.ID, Name: userView.Name, Email: userView.Email, Role: role})
		}
		return successWithEnvelope(data, fmt.Sprintf("added %s as %s", in.User, role))
	})

	addTool(s, opts, &mcp.Tool{Name: "member_role", Description: "Change a member's role in the workspace."}, func(ctx context.Context, req *mcp.CallToolRequest, in MemberRoleInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{Workspace: in.Workspace}, "workspace:write", app.PermissionMemberManage)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		wsRef := in.Workspace
		if wsRef == "" {
			wsRef = svc.Runtime().WorkspaceSlug
		}
		if err := svc.ChangeMemberRole(app.ChangeMemberRoleInput{
			WorkspaceRef: wsRef,
			UserRef:      in.User,
			Role:         app.Role(in.Role),
		}); err != nil {
			return businessErrorWithEnvelope(err)
		}
		data := map[string]any{"user": in.User, "role": in.Role}
		if userView, err := svc.UserInfo(in.User); err == nil {
			data["member"] = memberViewFromApp(app.MemberView{UserID: userView.ID, Name: userView.Name, Email: userView.Email, Role: app.Role(in.Role)})
		}
		return successWithEnvelope(data, fmt.Sprintf("changed %s role to %s", in.User, in.Role))
	})
}
