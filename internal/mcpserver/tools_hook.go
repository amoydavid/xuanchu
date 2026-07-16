package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/task"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type HookListInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

func (in HookListInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type HookAddInput struct {
	Workspace string   `json:"workspace,omitempty"`
	Project   string   `json:"project,omitempty"`
	ProjectID string   `json:"project_id,omitempty"`
	Name      string   `json:"name" jsonschema:"hook name"`
	Sink      string   `json:"sink" jsonschema:"workspace notification sink name or ID"`
	Events    []string `json:"events" jsonschema:"event types (task.created, task.modified, task.completed, task.deleted, task.started, task.stopped, task.reopened, task.assigned, task.unassigned, task.blocked, task.due_changed, task.priority_changed, task.project_changed, task.tags_changed, task.unblocked, project.archived, project.annotated, project.denotated)"`
	Active    *bool    `json:"active,omitempty" jsonschema:"whether the hook is enabled (default true)"`
}

func (in HookAddInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type HookRefInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Hook      string `json:"hook" jsonschema:"hook ID"`
}

func (in HookRefInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type HookModifyInput struct {
	Workspace string   `json:"workspace,omitempty"`
	Project   string   `json:"project,omitempty"`
	ProjectID string   `json:"project_id,omitempty"`
	Hook      string   `json:"hook" jsonschema:"hook ID"`
	Name      *string  `json:"name,omitempty" jsonschema:"new hook name"`
	Sink      *string  `json:"sink,omitempty" jsonschema:"new notification sink name or ID"`
	Events    []string `json:"events,omitempty" jsonschema:"event types that trigger the hook"`
	Active    *bool    `json:"active,omitempty" jsonschema:"whether the hook is enabled"`
}

func (in HookModifyInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type HookDeliveryListInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Hook      string `json:"hook" jsonschema:"hook ID"`
	Limit     int    `json:"limit,omitempty" jsonschema:"max deliveries to return (default 20)"`
	Offset    int    `json:"offset,omitempty" jsonschema:"number of deliveries to skip"`
}

func (in HookDeliveryListInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type HookDeliveryRefInput struct {
	Workspace  string `json:"workspace,omitempty"`
	Project    string `json:"project,omitempty"`
	ProjectID  string `json:"project_id,omitempty"`
	Hook       string `json:"hook" jsonschema:"hook ID"`
	DeliveryID string `json:"delivery_id" jsonschema:"delivery ID"`
}

func (in HookDeliveryRefInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

func registerHookTools(s *mcp.Server, opts Options) {
	addTool(s, opts, &mcp.Tool{Name: "hook_list", Description: "List all hooks; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in HookListInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "hook:read", app.PermissionHookRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		projectRef := projectRefForScope(in.Project, in.ProjectID)
		rows, err := svc.ListHooks(projectRef)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		data := map[string]any{"hooks": hookViewsFromApp(rows), "count": len(rows)}
		return successWithEnvelope(data, fmt.Sprintf("%d hook(s)", len(rows)))
	})

	addTool(s, opts, &mcp.Tool{Name: "hook_add", Description: "Create a hook; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in HookAddInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "hook:write", app.PermissionHookWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		var scopeType app.HookScopeType
		projectRef := projectRefForScope(in.Project, in.ProjectID)
		if projectRef != "" {
			scopeType = app.HookScopeProject
		}
		input := app.HookAddInput{
			Name:       strings.TrimSpace(in.Name),
			ScopeType:  scopeType,
			ProjectRef: projectRef,
			EventTypes: in.Events,
			SinkRef:    strings.TrimSpace(in.Sink),
		}
		view, err := svc.AddHook(input)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		if in.Active != nil && !*in.Active {
			view, err = svc.DisableHook(view.ID)
			if err != nil {
				return businessErrorWithEnvelope(err)
			}
		}
		return successWithEnvelope(map[string]any{"hook": hookViewFromApp(view)}, "created hook "+view.Name)
	})

	addTool(s, opts, &mcp.Tool{Name: "hook_info", Description: "Get hook details; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in HookRefInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "hook:read", app.PermissionHookRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.HookInfo(strings.TrimSpace(in.Hook))
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"hook": hookViewFromApp(view)}, "hook "+view.Name)
	})

	addTool(s, opts, &mcp.Tool{Name: "hook_modify", Description: "Modify a hook; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in HookModifyInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "hook:write", app.PermissionHookWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		hookID := strings.TrimSpace(in.Hook)
		mod := app.HookModifyInput{
			Name:    in.Name,
			SinkRef: in.Sink,
		}
		if len(in.Events) > 0 {
			mod.EventTypes = &in.Events
		}
		view, err := svc.ModifyHook(hookID, mod)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		if in.Active != nil {
			if *in.Active {
				view, err = svc.EnableHook(view.ID)
			} else {
				view, err = svc.DisableHook(view.ID)
			}
			if err != nil {
				return businessErrorWithEnvelope(err)
			}
		}
		return successWithEnvelope(map[string]any{"hook": hookViewFromApp(view)}, "modified hook "+view.Name)
	})

	addTool(s, opts, &mcp.Tool{Name: "hook_remove", Description: "Delete a hook; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in HookRefInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "hook:write", app.PermissionHookWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		if err := svc.DeleteHook(strings.TrimSpace(in.Hook)); err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"removed": strings.TrimSpace(in.Hook)}, "deleted hook")
	})

	addTool(s, opts, &mcp.Tool{Name: "hook_delivery_list", Description: "List hook deliveries; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in HookDeliveryListInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "hook:read", app.PermissionHookRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		limit := in.Limit
		if limit <= 0 {
			limit = 20
		}
		rows, err := svc.ListHookDeliveries(strings.TrimSpace(in.Hook), "", limit, in.Offset)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		data := map[string]any{"deliveries": hookDeliveryViewsFromApp(rows), "count": len(rows)}
		return successWithEnvelope(data, fmt.Sprintf("%d delivery/ies", len(rows)))
	})

	addTool(s, opts, &mcp.Tool{Name: "hook_delivery_info", Description: "Get delivery details; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in HookDeliveryRefInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "hook:read", app.PermissionHookRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.HookDeliveryInfo(strings.TrimSpace(in.DeliveryID))
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"delivery": hookDeliveryViewFromApp(view)}, "delivery "+view.ID)
	})

	addTool(s, opts, &mcp.Tool{Name: "hook_delivery_redeliver", Description: "Redeliver a hook delivery; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in HookDeliveryRefInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "hook:write", app.PermissionHookWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.ReplayHookDelivery(strings.TrimSpace(in.DeliveryID))
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"delivery": hookDeliveryViewFromApp(view)}, "replayed delivery "+view.ID)
	})

	addTool(s, opts, &mcp.Tool{Name: "hook_test", Description: "Get hook details for testing; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in HookRefInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "hook:read", app.PermissionHookRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.HookInfo(strings.TrimSpace(in.Hook))
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"hook": hookViewFromApp(view), "status": "reachable"}, "hook "+view.Name+" is configured")
	})

	addTool(s, opts, &mcp.Tool{Name: "hook_ping", Description: "Ping a hook by fetching its info; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in HookRefInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "hook:write", app.PermissionHookWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.HookInfo(strings.TrimSpace(in.Hook))
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"hook": hookViewFromApp(view), "pong": true}, "hook "+view.Name+" pinged")
	})
}

type hookView struct {
	ID             string             `json:"id"`
	Name           string             `json:"name"`
	ScopeType      string             `json:"scope_type"`
	WorkspaceID    string             `json:"workspace_id"`
	ProjectID      *string            `json:"project_id,omitempty"`
	EventTypes     []string           `json:"event_types"`
	SinkID         string             `json:"sink_id"`
	SinkName       string             `json:"sink_name"`
	SinkType       string             `json:"sink_type"`
	Enabled        bool               `json:"enabled"`
	TimeoutSeconds int                `json:"timeout_seconds"`
	MaxAttempts    int                `json:"max_attempts"`
	CreatedBy      task.JSONActorInfo `json:"created_by"`
	CreatedAt      int64              `json:"created_at"`
	ModifiedAt     int64              `json:"modified_at"`
}

type hookDeliveryView struct {
	ID             string             `json:"id"`
	HookID         string             `json:"hook_id"`
	EventID        string             `json:"event_id"`
	EventType      string             `json:"event_type"`
	WorkspaceID    string             `json:"workspace_id"`
	ProjectID      *string            `json:"project_id,omitempty"`
	Actor          task.JSONActorInfo `json:"actor"`
	Payload        map[string]any     `json:"payload,omitempty"`
	Headers        map[string]string  `json:"headers,omitempty"`
	Status         string             `json:"status"`
	AttemptCount   int                `json:"attempt_count"`
	NextAttemptAt  *int64             `json:"next_attempt_at,omitempty"`
	ClaimExpiresAt *int64             `json:"claim_expires_at,omitempty"`
	LastAttemptAt  *int64             `json:"last_attempt_at,omitempty"`
	LastStatusCode *int               `json:"last_status_code,omitempty"`
	LastError      string             `json:"last_error,omitempty"`
	CreatedAt      int64              `json:"created_at"`
	ModifiedAt     int64              `json:"modified_at"`
}

func hookViewFromApp(v app.HookView) hookView {
	return hookView{
		ID:             v.ID,
		Name:           v.Name,
		ScopeType:      v.ScopeType,
		WorkspaceID:    v.WorkspaceID,
		ProjectID:      v.ProjectID,
		EventTypes:     v.EventTypes,
		SinkID:         v.SinkID,
		SinkName:       v.SinkName,
		SinkType:       v.SinkType,
		Enabled:        v.Enabled,
		TimeoutSeconds: v.TimeoutSeconds,
		MaxAttempts:    v.MaxAttempts,
		CreatedBy:      task.ActorInfoToJSON(v.Actor),
		CreatedAt:      v.CreatedAt,
		ModifiedAt:     v.ModifiedAt,
	}
}

func hookViewsFromApp(rows []app.HookView) []hookView {
	out := make([]hookView, 0, len(rows))
	for _, row := range rows {
		out = append(out, hookViewFromApp(row))
	}
	return out
}

func hookDeliveryViewFromApp(v app.HookDeliveryView) hookDeliveryView {
	return hookDeliveryView{
		ID:             v.ID,
		HookID:         v.HookID,
		EventID:        v.EventID,
		EventType:      v.EventType,
		WorkspaceID:    v.WorkspaceID,
		ProjectID:      v.ProjectID,
		Actor:          task.ActorInfoToJSON(v.Actor),
		Payload:        v.Payload,
		Headers:        v.Headers,
		Status:         v.Status,
		AttemptCount:   v.AttemptCount,
		NextAttemptAt:  v.NextAttemptAt,
		ClaimExpiresAt: v.ClaimExpiresAt,
		LastAttemptAt:  v.LastAttemptAt,
		LastStatusCode: v.LastStatusCode,
		LastError:      v.LastError,
		CreatedAt:      v.CreatedAt,
		ModifiedAt:     v.ModifiedAt,
	}
}

func hookDeliveryViewsFromApp(rows []app.HookDeliveryView) []hookDeliveryView {
	out := make([]hookDeliveryView, 0, len(rows))
	for _, row := range rows {
		out = append(out, hookDeliveryViewFromApp(row))
	}
	return out
}
