package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/remote"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type notificationSinkToolInput struct {
	Workspace       string                                `json:"workspace,omitempty"`
	Project         string                                `json:"project,omitempty"`
	ProjectID       string                                `json:"project_id,omitempty"`
	Name            string                                `json:"name" jsonschema:"notification sink name"`
	Type            string                                `json:"type,omitempty" jsonschema:"webhook or http_template"`
	EndpointMode    string                                `json:"endpoint_mode,omitempty"`
	URL             string                                `json:"url,omitempty"`
	URLTemplate     string                                `json:"url_template,omitempty"`
	ConfigKey       string                                `json:"config_key,omitempty"`
	AllowedHosts    []string                              `json:"allowed_hosts,omitempty"`
	HTTPMethod      string                                `json:"http_method,omitempty"`
	HeaderTemplates []remote.HTTPHeaderTemplateRequest    `json:"header_templates,omitempty"`
	BodyTemplate    string                                `json:"body_template,omitempty"`
	BodyContentType string                                `json:"body_content_type,omitempty"`
	SecretRefs      []remote.HTTPTemplateSecretRefRequest `json:"secret_refs,omitempty"`
	Secret          string                                `json:"secret,omitempty"`
	TimeoutSeconds  int                                   `json:"timeout_seconds,omitempty"`
	MaxAttempts     int                                   `json:"max_attempts,omitempty"`
	Enabled         *bool                                 `json:"enabled,omitempty"`
}

func (in notificationSinkToolInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type notificationSinkRefToolInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Sink      string `json:"sink" jsonschema:"sink ID or name"`
}

func (in notificationSinkRefToolInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type reminderRuleToolInput struct {
	Workspace     string   `json:"workspace,omitempty"`
	Project       string   `json:"project,omitempty"`
	ProjectID     string   `json:"project_id,omitempty"`
	Name          string   `json:"name" jsonschema:"reminder rule name"`
	ProjectRef    string   `json:"project_ref,omitempty"`
	TriggerType   string   `json:"trigger_type,omitempty"`
	OffsetSeconds int64    `json:"offset_seconds,omitempty"`
	AfterSeconds  int64    `json:"after_seconds,omitempty"`
	RepeatPolicy  string   `json:"repeat_policy,omitempty"`
	TaskFilter    string   `json:"task_filter,omitempty"`
	AudienceType  string   `json:"audience_type,omitempty"`
	Recipients    []string `json:"recipients,omitempty"`
	SinkRef       string   `json:"sink_ref,omitempty"`
	Enabled       *bool    `json:"enabled,omitempty"`
}

func (in reminderRuleToolInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type reminderRuleRefToolInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Rule      string `json:"rule" jsonschema:"reminder rule ID or name"`
}

func (in reminderRuleRefToolInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type notificationDeliveryToolInput struct {
	Workspace string `json:"workspace,omitempty"`
	Delivery  string `json:"delivery" jsonschema:"notification delivery ID"`
	Status    string `json:"status,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`
}

func (in notificationDeliveryToolInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace}
}

func registerNotificationTools(s *mcp.Server, opts Options) {
	addTool(s, &mcp.Tool{Name: "notification_sink_add", Description: "Create a notification sink."}, func(ctx context.Context, req *mcp.CallToolRequest, in notificationSinkToolInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "notification:write", app.PermissionNotificationWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.AddNotificationSink(app.NotificationSinkAddInput{
			Name:            strings.TrimSpace(in.Name),
			Type:            strings.TrimSpace(in.Type),
			EndpointMode:    strings.TrimSpace(in.EndpointMode),
			URL:             strings.TrimSpace(in.URL),
			URLTemplate:     strings.TrimSpace(in.URLTemplate),
			ConfigKey:       strings.TrimSpace(in.ConfigKey),
			AllowedHosts:    in.AllowedHosts,
			HTTPMethod:      strings.TrimSpace(in.HTTPMethod),
			HeaderTemplates: headerTemplatesFromRemote(in.HeaderTemplates),
			BodyTemplate:    in.BodyTemplate,
			BodyContentType: strings.TrimSpace(in.BodyContentType),
			SecretRefs:      secretRefsFromRemote(in.SecretRefs),
			Secret:          in.Secret,
			TimeoutSeconds:  in.TimeoutSeconds,
			MaxAttempts:     in.MaxAttempts,
		})
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"sink": view}, "created notification sink "+view.Name)
	})

	addTool(s, &mcp.Tool{Name: "notification_sink_list", Description: "List notification sinks."}, func(ctx context.Context, req *mcp.CallToolRequest, in notificationSinkToolInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "notification:read", app.PermissionNotificationRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		rows, err := svc.ListNotificationSinks(false)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"sinks": rows, "count": len(rows)}, fmt.Sprintf("%d notification sink(s)", len(rows)))
	})

	addTool(s, &mcp.Tool{Name: "notification_sink_info", Description: "Get notification sink details."}, func(ctx context.Context, req *mcp.CallToolRequest, in notificationSinkRefToolInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "notification:read", app.PermissionNotificationRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.NotificationSinkInfo(strings.TrimSpace(in.Sink))
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"sink": view}, "notification sink "+view.Name)
	})

	addTool(s, &mcp.Tool{Name: "notification_sink_modify", Description: "Modify a notification sink."}, func(ctx context.Context, req *mcp.CallToolRequest, in notificationSinkToolInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "notification:write", app.PermissionNotificationWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.ModifyNotificationSink(strings.TrimSpace(in.Name), app.NotificationSinkModifyInput{
			Type:            stringPtrIfSet(in.Type),
			EndpointMode:    stringPtrIfSet(in.EndpointMode),
			URL:             stringPtrIfSet(in.URL),
			URLTemplate:     stringPtrIfSet(in.URLTemplate),
			ConfigKey:       stringPtrIfSet(in.ConfigKey),
			AllowedHosts:    slicePtrIfSet(in.AllowedHosts),
			HTTPMethod:      stringPtrIfSet(in.HTTPMethod),
			HeaderTemplates: headerTemplatesPtrToAppIfSet(headerTemplatesFromRemote(in.HeaderTemplates)),
			BodyTemplate:    stringPtrIfSet(in.BodyTemplate),
			BodyContentType: stringPtrIfSet(in.BodyContentType),
			SecretRefs:      secretRefsPtrToAppIfSet(secretRefsFromRemote(in.SecretRefs)),
			Secret:          stringPtrIfSet(in.Secret),
			TimeoutSeconds:  intPtrIfSet(in.TimeoutSeconds),
			MaxAttempts:     intPtrIfSet(in.MaxAttempts),
			Enabled:         in.Enabled,
		})
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"sink": view}, "modified notification sink "+view.Name)
	})

	addTool(s, &mcp.Tool{Name: "notification_sink_enable", Description: "Enable a notification sink."}, func(ctx context.Context, req *mcp.CallToolRequest, in notificationSinkRefToolInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "notification:write", app.PermissionNotificationWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.EnableNotificationSink(strings.TrimSpace(in.Sink))
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"sink": view}, "enabled notification sink "+view.Name)
	})

	addTool(s, &mcp.Tool{Name: "notification_sink_disable", Description: "Disable a notification sink."}, func(ctx context.Context, req *mcp.CallToolRequest, in notificationSinkRefToolInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "notification:write", app.PermissionNotificationWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.DisableNotificationSink(strings.TrimSpace(in.Sink))
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"sink": view}, "disabled notification sink "+view.Name)
	})

	addTool(s, &mcp.Tool{Name: "notification_sink_remove", Description: "Remove a notification sink."}, func(ctx context.Context, req *mcp.CallToolRequest, in notificationSinkRefToolInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "notification:write", app.PermissionNotificationWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		if err := svc.DeleteNotificationSink(strings.TrimSpace(in.Sink)); err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"removed": strings.TrimSpace(in.Sink)}, "deleted notification sink")
	})

	addTool(s, &mcp.Tool{Name: "reminder_rule_add", Description: "Create a reminder rule."}, func(ctx context.Context, req *mcp.CallToolRequest, in reminderRuleToolInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "reminder:write", app.PermissionReminderWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.AddReminderRule(app.ReminderRuleAddInput{
			Name:          strings.TrimSpace(in.Name),
			ProjectRef:    in.ProjectRef,
			TriggerType:   in.TriggerType,
			OffsetSeconds: in.OffsetSeconds,
			AfterSeconds:  in.AfterSeconds,
			RepeatPolicy:  in.RepeatPolicy,
			TaskFilter:    in.TaskFilter,
			AudienceType:  in.AudienceType,
			Recipients:    in.Recipients,
			SinkRef:       in.SinkRef,
		})
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"rule": view}, "created reminder rule "+view.Name)
	})

	addTool(s, &mcp.Tool{Name: "reminder_rule_list", Description: "List reminder rules."}, func(ctx context.Context, req *mcp.CallToolRequest, in reminderRuleToolInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "reminder:read", app.PermissionReminderRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		rows, err := svc.ListReminderRules(in.ProjectRef, false)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"rules": rows, "count": len(rows)}, fmt.Sprintf("%d reminder rule(s)", len(rows)))
	})

	addTool(s, &mcp.Tool{Name: "reminder_rule_info", Description: "Get reminder rule details."}, func(ctx context.Context, req *mcp.CallToolRequest, in reminderRuleRefToolInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "reminder:read", app.PermissionReminderRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.ReminderRuleInfo(strings.TrimSpace(in.Rule))
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"rule": view}, "reminder rule "+view.Name)
	})

	addTool(s, &mcp.Tool{Name: "reminder_rule_modify", Description: "Modify a reminder rule."}, func(ctx context.Context, req *mcp.CallToolRequest, in reminderRuleToolInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "reminder:write", app.PermissionReminderWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.ModifyReminderRule(strings.TrimSpace(in.Name), app.ReminderRuleModifyInput{
			ProjectRef:    stringPtrIfSet(in.ProjectRef),
			TriggerType:   stringPtrIfSet(in.TriggerType),
			OffsetSeconds: int64PtrIfSet(in.OffsetSeconds),
			AfterSeconds:  int64PtrIfSet(in.AfterSeconds),
			RepeatPolicy:  stringPtrIfSet(in.RepeatPolicy),
			TaskFilter:    stringPtrIfSet(in.TaskFilter),
			AudienceType:  stringPtrIfSet(in.AudienceType),
			Recipients:    slicePtrIfSet(in.Recipients),
			SinkRef:       stringPtrIfSet(in.SinkRef),
			Enabled:       in.Enabled,
		})
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"rule": view}, "modified reminder rule "+view.Name)
	})

	addTool(s, &mcp.Tool{Name: "reminder_rule_enable", Description: "Enable a reminder rule."}, func(ctx context.Context, req *mcp.CallToolRequest, in reminderRuleRefToolInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "reminder:write", app.PermissionReminderWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.EnableReminderRule(strings.TrimSpace(in.Rule))
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"rule": view}, "enabled reminder rule "+view.Name)
	})

	addTool(s, &mcp.Tool{Name: "reminder_rule_disable", Description: "Disable a reminder rule."}, func(ctx context.Context, req *mcp.CallToolRequest, in reminderRuleRefToolInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "reminder:write", app.PermissionReminderWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.DisableReminderRule(strings.TrimSpace(in.Rule))
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"rule": view}, "disabled reminder rule "+view.Name)
	})

	addTool(s, &mcp.Tool{Name: "reminder_rule_remove", Description: "Remove a reminder rule."}, func(ctx context.Context, req *mcp.CallToolRequest, in reminderRuleRefToolInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "reminder:write", app.PermissionReminderWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		if err := svc.DeleteReminderRule(strings.TrimSpace(in.Rule)); err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"removed": strings.TrimSpace(in.Rule)}, "deleted reminder rule")
	})

	addTool(s, &mcp.Tool{Name: "notification_delivery_list", Description: "List notification deliveries."}, func(ctx context.Context, req *mcp.CallToolRequest, in notificationDeliveryToolInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "notification:read", app.PermissionNotificationRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		limit := in.Limit
		if limit <= 0 {
			limit = 20
		}
		rows, err := svc.ListNotificationDeliveries(in.Status, limit, in.Offset)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"deliveries": rows, "count": len(rows)}, fmt.Sprintf("%d notification delivery(ies)", len(rows)))
	})

	addTool(s, &mcp.Tool{Name: "notification_delivery_info", Description: "Get notification delivery details."}, func(ctx context.Context, req *mcp.CallToolRequest, in notificationDeliveryToolInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "notification:read", app.PermissionNotificationRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.NotificationDeliveryInfo(strings.TrimSpace(in.Delivery))
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"delivery": view}, "notification delivery "+view.ID)
	})

	addTool(s, &mcp.Tool{Name: "notification_delivery_replay", Description: "Replay notification delivery."}, func(ctx context.Context, req *mcp.CallToolRequest, in notificationDeliveryToolInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "notification:write", app.PermissionNotificationWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.ReplayNotificationDelivery(strings.TrimSpace(in.Delivery))
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"delivery": view}, "replayed notification delivery "+view.ID)
	})
}

func headerTemplatesFromRemote(rows []remote.HTTPHeaderTemplateRequest) []app.HTTPHeaderTemplateInput {
	out := make([]app.HTTPHeaderTemplateInput, 0, len(rows))
	for _, row := range rows {
		out = append(out, app.HTTPHeaderTemplateInput{Name: row.Name, Value: row.Value})
	}
	return out
}

func secretRefsFromRemote(rows []remote.HTTPTemplateSecretRefRequest) []app.HTTPTemplateSecretRefInput {
	out := make([]app.HTTPTemplateSecretRefInput, 0, len(rows))
	for _, row := range rows {
		out = append(out, app.HTTPTemplateSecretRefInput{Alias: row.Alias, ConfigKey: row.ConfigKey})
	}
	return out
}

func stringPtrIfSet(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func slicePtrIfSet(values []string) *[]string {
	if len(values) == 0 {
		return nil
	}
	return &values
}

func intPtrIfSet(value int) *int {
	if value == 0 {
		return nil
	}
	return &value
}

func int64PtrIfSet(value int64) *int64 {
	if value == 0 {
		return nil
	}
	return &value
}

func headerTemplatesPtrToAppIfSet(rows []app.HTTPHeaderTemplateInput) *[]app.HTTPHeaderTemplateInput {
	if len(rows) == 0 {
		return nil
	}
	return &rows
}

func secretRefsPtrToAppIfSet(rows []app.HTTPTemplateSecretRefInput) *[]app.HTTPTemplateSecretRefInput {
	if len(rows) == 0 {
		return nil
	}
	return &rows
}
