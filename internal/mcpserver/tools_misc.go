package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/task"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type AuditListInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Limit     int    `json:"limit,omitempty" jsonschema:"max audit entries to return (default 20)"`
	Offset    int    `json:"offset,omitempty" jsonschema:"number of audit entries to skip"`
	Actor     string `json:"actor,omitempty" jsonschema:"filter by actor user ref"`
}

type ScopeListInput struct{}

type MeGetInput struct{}

type auditLogView struct {
	ID               int64              `json:"id"`
	ActorType        string             `json:"actor_type,omitempty"`
	Actor            *task.JSONUserInfo `json:"actor,omitempty"`
	ActorToken       *tokenActorView    `json:"actor_token,omitempty"`
	WorkspaceID      *string            `json:"workspace_id,omitempty"`
	ProjectID        *string            `json:"project_id,omitempty"`
	Action           string             `json:"action"`
	TargetType       string             `json:"target_type"`
	TargetID         string             `json:"target_id"`
	Payload          json.RawMessage    `json:"payload,omitempty"`
	DelegatorTokenID *string            `json:"delegator_token_id,omitempty"`
	DelegatorUser    *task.JSONUserInfo `json:"delegator_user,omitempty"`
	CreatedAt        int64              `json:"created_at"`
}

type tokenActorView struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Prefix string `json:"prefix"`
}

func auditLogViewFromApp(row app.AuditLogView) auditLogView {
	var actor *task.JSONUserInfo
	if row.Actor != nil {
		j := task.UserInfoToJSON(*row.Actor)
		actor = &j
	}
	var delegatorUser *task.JSONUserInfo
	if row.DelegatorUser != nil {
		j := task.UserInfoToJSON(*row.DelegatorUser)
		delegatorUser = &j
	}
	var payload json.RawMessage
	if row.PayloadJSON != "" {
		payload = json.RawMessage(row.PayloadJSON)
	}
	return auditLogView{
		ID:               row.ID,
		ActorType:        row.ActorType,
		Actor:            actor,
		ActorToken:       tokenActorViewFromApp(row.ActorToken),
		WorkspaceID:      row.WorkspaceID,
		ProjectID:        row.ProjectID,
		Action:           row.Action,
		TargetType:       row.TargetType,
		TargetID:         row.TargetID,
		Payload:          payload,
		DelegatorTokenID: row.DelegatorTokenID,
		DelegatorUser:    delegatorUser,
		CreatedAt:        row.CreatedAt,
	}
}

func tokenActorViewFromApp(actor *app.TokenActorInfo) *tokenActorView {
	if actor == nil {
		return nil
	}
	return &tokenActorView{ID: actor.ID, Name: actor.Name, Prefix: actor.Prefix}
}

type meView struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	DisplayName string                `json:"display_name,omitempty"`
	Email       *string               `json:"email,omitempty"`
	ExternalIDs []task.JSONExternalID `json:"external_ids,omitempty"`
}

func registerMiscTools(s *mcp.Server, opts Options) {
	addTool(s, opts, &mcp.Tool{
		Name:        "audit_list",
		Description: "List audit log entries for the current workspace.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in AuditListInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		scopeInput := RequestScopeInput{
			Workspace: in.Workspace,
			Project:   in.Project,
			ProjectID: in.ProjectID,
		}
		svc, err := serviceForTool(ctx, req, opts, scopeInput, "audit:read", app.PermissionAuditRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		limit, err := limitOrDefault(in.Limit)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		rows, err := svc.ListAudit(app.AuditListInput{
			WorkspaceRef: in.Workspace,
			ProjectRef:   in.Project,
			Limit:        limit,
			Offset:       in.Offset,
		})
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		views := make([]auditLogView, len(rows))
		for i, row := range rows {
			views[i] = auditLogViewFromApp(row)
		}
		data := map[string]any{"entries": views, "count": len(views)}
		return successWithEnvelope(data, fmt.Sprintf("%d audit log entry(s)", len(views)))
	})

	addTool(s, opts, &mcp.Tool{
		Name:        "scope_list",
		Description: "List all available token scopes.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in ScopeListInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		scopes := auth.ScopeRegistryValues()
		data := map[string]any{"scopes": scopes}
		return successWithEnvelope(data, fmt.Sprintf("%d scope(s)", len(scopes)))
	})

	addTool(s, opts, &mcp.Tool{
		Name:        "me_get",
		Description: "Get the current authenticated user info.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in MeGetInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, RequestScopeInput{}, "workspace:read", app.PermissionWorkspaceRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		rt := svc.Runtime()
		if rt.ActorUserID == "" {
			return businessErrorWithEnvelope(app.RuntimeError{Code: "auth_no_user", Message: "no authenticated user"})
		}
		userInfo, err := svc.UserInfo(rt.ActorUserID)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		extIDs := make([]task.JSONExternalID, 0, len(userInfo.ExternalIDs))
		for _, eid := range userInfo.ExternalIDs {
			extIDs = append(extIDs, task.JSONExternalID{Provider: eid.Provider, UserType: eid.UserType, ExternalID: eid.ExternalID})
		}
		view := meView{
			ID:          userInfo.ID,
			Name:        userInfo.Name,
			DisplayName: userInfo.DisplayName,
			Email:       userInfo.Email,
			ExternalIDs: extIDs,
		}
		data := map[string]any{"user": view}
		return successWithEnvelope(data, "user "+view.Name)
	})
}
