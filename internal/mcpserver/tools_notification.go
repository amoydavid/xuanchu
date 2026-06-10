package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/task"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type NotificationSinkListInput struct {
	Workspace       string `json:"workspace,omitempty"`
	IncludeDisabled bool   `json:"include_disabled,omitempty"`
}

func (in NotificationSinkListInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace}
}

type NotificationSinkAddInput struct {
	Workspace       string                           `json:"workspace,omitempty"`
	Name            string                           `json:"name" jsonschema:"sink name"`
	Type            string                           `json:"type,omitempty" jsonschema:"webhook or http_template"`
	EndpointMode    string                           `json:"endpoint_mode,omitempty" jsonschema:"static_url, template, or config_value"`
	URL             string                           `json:"url,omitempty" jsonschema:"static endpoint URL"`
	URLTemplate     string                           `json:"url_template,omitempty" jsonschema:"controlled URL template"`
	ConfigKey       string                           `json:"config_key,omitempty" jsonschema:"config key for config_value endpoint mode"`
	AllowedHosts    []string                         `json:"allowed_hosts,omitempty" jsonschema:"allowed endpoint hosts for dynamic endpoints"`
	HeaderTemplates []app.HTTPHeaderTemplateInput    `json:"header_templates,omitempty" jsonschema:"HTTP header templates persisted in the database"`
	BodyTemplate    string                           `json:"body_template,omitempty" jsonschema:"HTTP body template persisted in the database"`
	BodyContentType string                           `json:"body_content_type,omitempty" jsonschema:"rendered body content type"`
	SecretRefs      []app.HTTPTemplateSecretRefInput `json:"secret_refs,omitempty" jsonschema:"secret aliases backed by secret config keys"`
	Secret          string                           `json:"secret,omitempty" jsonschema:"webhook signing secret"`
	TimeoutSeconds  int                              `json:"timeout_seconds,omitempty"`
	MaxAttempts     int                              `json:"max_attempts,omitempty"`
	MaxConcurrency  int                              `json:"max_concurrency,omitempty"`
}

func (in NotificationSinkAddInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace}
}

type NotificationSinkRefInput struct {
	Workspace string `json:"workspace,omitempty"`
	Sink      string `json:"sink" jsonschema:"notification sink ID"`
}

func (in NotificationSinkRefInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace}
}

type NotificationSinkModifyInput struct {
	Workspace       string                            `json:"workspace,omitempty"`
	Sink            string                            `json:"sink" jsonschema:"notification sink ID"`
	Name            *string                           `json:"name,omitempty"`
	Type            *string                           `json:"type,omitempty"`
	EndpointMode    *string                           `json:"endpoint_mode,omitempty"`
	URL             *string                           `json:"url,omitempty"`
	URLTemplate     *string                           `json:"url_template,omitempty"`
	ConfigKey       *string                           `json:"config_key,omitempty"`
	AllowedHosts    *[]string                         `json:"allowed_hosts,omitempty"`
	HeaderTemplates *[]app.HTTPHeaderTemplateInput    `json:"header_templates,omitempty"`
	BodyTemplate    *string                           `json:"body_template,omitempty"`
	BodyContentType *string                           `json:"body_content_type,omitempty"`
	SecretRefs      *[]app.HTTPTemplateSecretRefInput `json:"secret_refs,omitempty"`
	Secret          *string                           `json:"secret,omitempty"`
	TimeoutSeconds  *int                              `json:"timeout_seconds,omitempty"`
	MaxAttempts     *int                              `json:"max_attempts,omitempty"`
	MaxConcurrency  *int                              `json:"max_concurrency,omitempty"`
}

func (in NotificationSinkModifyInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace}
}

type ReminderRuleListInput struct {
	Workspace       string `json:"workspace,omitempty"`
	Project         string `json:"project,omitempty"`
	ProjectID       string `json:"project_id,omitempty"`
	IncludeDisabled bool   `json:"include_disabled,omitempty"`
}

func (in ReminderRuleListInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type ReminderRuleAddInput struct {
	Workspace     string   `json:"workspace,omitempty"`
	Project       string   `json:"project,omitempty"`
	ProjectID     string   `json:"project_id,omitempty"`
	Name          string   `json:"name" jsonschema:"rule name"`
	TriggerType   string   `json:"trigger_type,omitempty" jsonschema:"due_before or overdue"`
	OffsetSeconds int64    `json:"offset_seconds,omitempty" jsonschema:"seconds before due for due_before"`
	AfterSeconds  int64    `json:"after_seconds,omitempty" jsonschema:"seconds after due for overdue"`
	RepeatPolicy  string   `json:"repeat_policy,omitempty" jsonschema:"once or every:<duration>"`
	ScheduleType  string   `json:"schedule_type,omitempty" jsonschema:"daily_at or daily@HH:MM"`
	ScheduleValue string   `json:"schedule_value,omitempty" jsonschema:"daily schedule time such as 08:50"`
	FilterSource  string   `json:"filter_source,omitempty" jsonschema:"task filter expression for scheduled rules"`
	AudienceType  string   `json:"audience_type" jsonschema:"assignees, explicit_users, or assignees_and_explicit_users"`
	Recipients    []string `json:"recipients,omitempty" jsonschema:"explicit recipient user refs"`
	Sink          string   `json:"sink" jsonschema:"notification sink name or ID"`
}

func (in ReminderRuleAddInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type ReminderRuleRefInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Rule      string `json:"rule" jsonschema:"reminder rule ID"`
}

func (in ReminderRuleRefInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type ReminderRuleModifyInput struct {
	Workspace     string    `json:"workspace,omitempty"`
	Project       string    `json:"project,omitempty"`
	ProjectID     string    `json:"project_id,omitempty"`
	Rule          string    `json:"rule" jsonschema:"reminder rule ID"`
	Name          *string   `json:"name,omitempty"`
	ProjectRef    *string   `json:"project_ref,omitempty" jsonschema:"new rule project scope; empty clears project scope"`
	TriggerType   *string   `json:"trigger_type,omitempty"`
	OffsetSeconds *int64    `json:"offset_seconds,omitempty"`
	AfterSeconds  *int64    `json:"after_seconds,omitempty"`
	RepeatPolicy  *string   `json:"repeat_policy,omitempty"`
	ScheduleType  *string   `json:"schedule_type,omitempty"`
	ScheduleValue *string   `json:"schedule_value,omitempty"`
	FilterSource  *string   `json:"filter_source,omitempty"`
	AudienceType  *string   `json:"audience_type,omitempty"`
	Recipients    *[]string `json:"recipients,omitempty"`
	Sink          *string   `json:"sink,omitempty" jsonschema:"notification sink name or ID"`
}

func (in ReminderRuleModifyInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type NotificationRuleListInput struct {
	Workspace       string `json:"workspace,omitempty"`
	Project         string `json:"project,omitempty"`
	ProjectID       string `json:"project_id,omitempty"`
	IncludeDisabled bool   `json:"include_disabled,omitempty"`
}

func (in NotificationRuleListInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type NotificationRuleAddInput struct {
	Workspace       string   `json:"workspace,omitempty"`
	Project         string   `json:"project,omitempty"`
	ProjectID       string   `json:"project_id,omitempty"`
	Name            string   `json:"name" jsonschema:"rule name"`
	Event           string   `json:"event" jsonschema:"event type such as task.unblocked"`
	Filter          string   `json:"filter,omitempty" jsonschema:"task filter expression for task events"`
	Audience        string   `json:"audience" jsonschema:"actor, assignees, explicit_users, or assignees_and_explicit_users"`
	Recipients      []string `json:"recipients,omitempty" jsonschema:"explicit recipient user refs"`
	Sink            string   `json:"sink" jsonschema:"notification sink name or ID"`
	TemplateSubject string   `json:"template_subject,omitempty"`
	TemplateBody    string   `json:"template_body,omitempty"`
}

func (in NotificationRuleAddInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type NotificationRuleRefInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Rule      string `json:"rule" jsonschema:"notification rule ID"`
}

func (in NotificationRuleRefInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type NotificationRuleModifyInput struct {
	Workspace       string    `json:"workspace,omitempty"`
	Project         string    `json:"project,omitempty"`
	ProjectID       string    `json:"project_id,omitempty"`
	Rule            string    `json:"rule" jsonschema:"notification rule ID"`
	Name            *string   `json:"name,omitempty"`
	ProjectRef      *string   `json:"project_ref,omitempty" jsonschema:"new rule project scope; empty clears project scope"`
	Event           *string   `json:"event,omitempty"`
	Filter          *string   `json:"filter,omitempty"`
	Audience        *string   `json:"audience,omitempty"`
	Recipients      *[]string `json:"recipients,omitempty"`
	Sink            *string   `json:"sink,omitempty" jsonschema:"notification sink name or ID"`
	TemplateSubject *string   `json:"template_subject,omitempty"`
	TemplateBody    *string   `json:"template_body,omitempty"`
}

func (in NotificationRuleModifyInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type NotificationDeliveryListInput struct {
	Workspace string `json:"workspace,omitempty"`
	Sink      string `json:"sink,omitempty" jsonschema:"notification sink ID"`
	Status    string `json:"status,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`
}

func (in NotificationDeliveryListInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace}
}

type NotificationDeliveryRefInput struct {
	Workspace  string `json:"workspace,omitempty"`
	DeliveryID string `json:"delivery_id" jsonschema:"notification delivery ID"`
}

func (in NotificationDeliveryRefInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace}
}

func registerNotificationTools(s *mcp.Server, opts Options) {
	addTool(s, &mcp.Tool{Name: "notification_sink_list", Description: "List notification sinks; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in NotificationSinkListInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "notification:read", app.PermissionNotificationRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		rows, err := svc.ListNotificationSinks(in.IncludeDisabled)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"sinks": notificationSinkViewsForMCP(rows), "count": len(rows)}, fmt.Sprintf("%d notification sink(s)", len(rows)))
	})

	addTool(s, &mcp.Tool{Name: "notification_sink_add", Description: "Create a notification sink; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in NotificationSinkAddInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "notification:write", app.PermissionNotificationWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.AddNotificationSink(app.NotificationSinkAddInput{
			Name:            strings.TrimSpace(in.Name),
			Type:            strings.TrimSpace(in.Type),
			EndpointMode:    strings.TrimSpace(in.EndpointMode),
			URL:             strings.TrimSpace(in.URL),
			URLTemplate:     in.URLTemplate,
			ConfigKey:       strings.TrimSpace(in.ConfigKey),
			AllowedHosts:    in.AllowedHosts,
			HTTPMethod:      "POST",
			HeaderTemplates: in.HeaderTemplates,
			BodyTemplate:    in.BodyTemplate,
			BodyContentType: in.BodyContentType,
			SecretRefs:      in.SecretRefs,
			Secret:          in.Secret,
			TimeoutSeconds:  in.TimeoutSeconds,
			MaxAttempts:     in.MaxAttempts,
			MaxConcurrency:  in.MaxConcurrency,
		})
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"sink": notificationSinkViewForMCP(view)}, "created notification sink "+view.Name)
	})

	addTool(s, &mcp.Tool{Name: "notification_sink_info", Description: "Get notification sink details; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in NotificationSinkRefInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "notification:read", app.PermissionNotificationRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.NotificationSinkInfo(strings.TrimSpace(in.Sink))
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"sink": notificationSinkViewForMCP(view)}, "notification sink "+view.Name)
	})

	addTool(s, &mcp.Tool{Name: "notification_sink_modify", Description: "Modify a notification sink; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in NotificationSinkModifyInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "notification:write", app.PermissionNotificationWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.ModifyNotificationSink(strings.TrimSpace(in.Sink), app.NotificationSinkModifyInput{
			Name:            in.Name,
			Type:            in.Type,
			EndpointMode:    in.EndpointMode,
			URL:             in.URL,
			URLTemplate:     in.URLTemplate,
			ConfigKey:       in.ConfigKey,
			AllowedHosts:    in.AllowedHosts,
			HeaderTemplates: in.HeaderTemplates,
			BodyTemplate:    in.BodyTemplate,
			BodyContentType: in.BodyContentType,
			SecretRefs:      in.SecretRefs,
			Secret:          in.Secret,
			TimeoutSeconds:  in.TimeoutSeconds,
			MaxAttempts:     in.MaxAttempts,
			MaxConcurrency:  in.MaxConcurrency,
		})
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"sink": notificationSinkViewForMCP(view)}, "modified notification sink "+view.Name)
	})

	addTool(s, &mcp.Tool{Name: "notification_sink_enable", Description: "Enable a notification sink; writes audit."}, notificationSinkToggleHandler(opts, true))
	addTool(s, &mcp.Tool{Name: "notification_sink_disable", Description: "Disable a notification sink; writes audit."}, notificationSinkToggleHandler(opts, false))

	addTool(s, &mcp.Tool{Name: "notification_sink_remove", Description: "Delete a notification sink; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in NotificationSinkRefInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "notification:write", app.PermissionNotificationWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		sinkID := strings.TrimSpace(in.Sink)
		if err := svc.DeleteNotificationSink(sinkID); err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"removed": sinkID}, "deleted notification sink")
	})

	addTool(s, &mcp.Tool{Name: "reminder_rule_list", Description: "List reminder rules; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in ReminderRuleListInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "reminder:read", app.PermissionReminderRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		projectRef := projectRefForScope(in.Project, in.ProjectID)
		rows, err := svc.ListReminderRules(projectRef, in.IncludeDisabled)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"rules": reminderRuleViewsForMCP(rows), "count": len(rows)}, fmt.Sprintf("%d reminder rule(s)", len(rows)))
	})

	addTool(s, &mcp.Tool{Name: "reminder_rule_add", Description: "Create a reminder rule; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in ReminderRuleAddInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "reminder:write", app.PermissionReminderWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.AddReminderRule(app.ReminderRuleAddInput{
			Name:          strings.TrimSpace(in.Name),
			ProjectRef:    projectRefForScope(in.Project, in.ProjectID),
			TriggerType:   strings.TrimSpace(in.TriggerType),
			OffsetSeconds: in.OffsetSeconds,
			AfterSeconds:  in.AfterSeconds,
			RepeatPolicy:  strings.TrimSpace(in.RepeatPolicy),
			ScheduleType:  strings.TrimSpace(in.ScheduleType),
			ScheduleValue: strings.TrimSpace(in.ScheduleValue),
			FilterSource:  strings.TrimSpace(in.FilterSource),
			AudienceType:  strings.TrimSpace(in.AudienceType),
			Recipients:    in.Recipients,
			SinkRef:       strings.TrimSpace(in.Sink),
		})
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"rule": reminderRuleViewForMCP(view)}, "created reminder rule "+view.Name)
	})

	addTool(s, &mcp.Tool{Name: "reminder_rule_info", Description: "Get reminder rule details; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in ReminderRuleRefInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "reminder:read", app.PermissionReminderRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.ReminderRuleInfo(strings.TrimSpace(in.Rule))
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"rule": reminderRuleViewForMCP(view)}, "reminder rule "+view.Name)
	})

	addTool(s, &mcp.Tool{Name: "reminder_rule_modify", Description: "Modify a reminder rule; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in ReminderRuleModifyInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "reminder:write", app.PermissionReminderWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.ModifyReminderRule(strings.TrimSpace(in.Rule), app.ReminderRuleModifyInput{
			Name:          in.Name,
			ProjectRef:    in.ProjectRef,
			TriggerType:   in.TriggerType,
			OffsetSeconds: in.OffsetSeconds,
			AfterSeconds:  in.AfterSeconds,
			RepeatPolicy:  in.RepeatPolicy,
			ScheduleType:  in.ScheduleType,
			ScheduleValue: in.ScheduleValue,
			FilterSource:  in.FilterSource,
			AudienceType:  in.AudienceType,
			Recipients:    in.Recipients,
			SinkRef:       in.Sink,
		})
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"rule": reminderRuleViewForMCP(view)}, "modified reminder rule "+view.Name)
	})

	addTool(s, &mcp.Tool{Name: "reminder_rule_enable", Description: "Enable a reminder rule; writes audit."}, reminderRuleToggleHandler(opts, true))
	addTool(s, &mcp.Tool{Name: "reminder_rule_disable", Description: "Disable a reminder rule; writes audit."}, reminderRuleToggleHandler(opts, false))

	addTool(s, &mcp.Tool{Name: "reminder_rule_remove", Description: "Delete a reminder rule; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in ReminderRuleRefInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "reminder:write", app.PermissionReminderWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		ruleID := strings.TrimSpace(in.Rule)
		if err := svc.DeleteReminderRule(ruleID); err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"removed": ruleID}, "deleted reminder rule")
	})

	addTool(s, &mcp.Tool{Name: "notification_rule_list", Description: "List event notification rules; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in NotificationRuleListInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "notification:read", app.PermissionNotificationRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		projectRef := projectRefForScope(in.Project, in.ProjectID)
		rows, err := svc.ListEventNotificationRules(projectRef, in.IncludeDisabled)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"rules": notificationRuleViewsForMCP(rows), "count": len(rows)}, fmt.Sprintf("%d notification rule(s)", len(rows)))
	})

	addTool(s, &mcp.Tool{Name: "notification_rule_add", Description: "Create an event notification rule; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in NotificationRuleAddInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "notification:write", app.PermissionNotificationWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.AddEventNotificationRule(app.EventNotificationRuleAddInput{
			Name:            strings.TrimSpace(in.Name),
			ProjectRef:      projectRefForScope(in.Project, in.ProjectID),
			EventType:       strings.TrimSpace(in.Event),
			FilterSource:    strings.TrimSpace(in.Filter),
			AudienceType:    strings.TrimSpace(in.Audience),
			Recipients:      in.Recipients,
			SinkRef:         strings.TrimSpace(in.Sink),
			TemplateSubject: in.TemplateSubject,
			TemplateBody:    in.TemplateBody,
		})
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"rule": notificationRuleViewForMCP(view)}, "created notification rule "+view.Name)
	})

	addTool(s, &mcp.Tool{Name: "notification_rule_info", Description: "Get event notification rule details; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in NotificationRuleRefInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "notification:read", app.PermissionNotificationRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.EventNotificationRuleInfo(strings.TrimSpace(in.Rule))
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"rule": notificationRuleViewForMCP(view)}, "notification rule "+view.Name)
	})

	addTool(s, &mcp.Tool{Name: "notification_rule_modify", Description: "Modify an event notification rule; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in NotificationRuleModifyInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "notification:write", app.PermissionNotificationWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.ModifyEventNotificationRule(strings.TrimSpace(in.Rule), app.EventNotificationRuleModifyInput{
			Name:            in.Name,
			ProjectRef:      in.ProjectRef,
			EventType:       in.Event,
			FilterSource:    in.Filter,
			AudienceType:    in.Audience,
			Recipients:      in.Recipients,
			SinkRef:         in.Sink,
			TemplateSubject: in.TemplateSubject,
			TemplateBody:    in.TemplateBody,
		})
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"rule": notificationRuleViewForMCP(view)}, "modified notification rule "+view.Name)
	})

	addTool(s, &mcp.Tool{Name: "notification_rule_enable", Description: "Enable an event notification rule; writes audit."}, notificationRuleToggleHandler(opts, true))
	addTool(s, &mcp.Tool{Name: "notification_rule_disable", Description: "Disable an event notification rule; writes audit."}, notificationRuleToggleHandler(opts, false))

	addTool(s, &mcp.Tool{Name: "notification_rule_remove", Description: "Delete an event notification rule; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in NotificationRuleRefInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "notification:write", app.PermissionNotificationWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		ruleID := strings.TrimSpace(in.Rule)
		if err := svc.DeleteEventNotificationRule(ruleID); err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"removed": ruleID}, "deleted notification rule")
	})

	addTool(s, &mcp.Tool{Name: "notification_delivery_list", Description: "List notification deliveries; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in NotificationDeliveryListInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "notification:read", app.PermissionNotificationRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		limit := in.Limit
		if limit <= 0 {
			limit = 50
		}
		rows, err := svc.ListNotificationDeliveries(strings.TrimSpace(in.Sink), strings.TrimSpace(in.Status), limit, in.Offset)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"deliveries": notificationDeliveryViewsForMCP(rows), "count": len(rows)}, fmt.Sprintf("%d notification delivery/ies", len(rows)))
	})

	addTool(s, &mcp.Tool{Name: "notification_delivery_info", Description: "Get notification delivery details; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in NotificationDeliveryRefInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "notification:read", app.PermissionNotificationRead)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.NotificationDeliveryInfo(strings.TrimSpace(in.DeliveryID))
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"delivery": notificationDeliveryViewForMCP(view)}, "notification delivery "+view.ID)
	})

	addTool(s, &mcp.Tool{Name: "notification_delivery_replay", Description: "Replay a dead-lettered or skipped notification delivery; writes audit."}, func(ctx context.Context, req *mcp.CallToolRequest, in NotificationDeliveryRefInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "notification:write", app.PermissionNotificationWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.ReplayNotificationDelivery(strings.TrimSpace(in.DeliveryID))
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"delivery": notificationDeliveryViewForMCP(view)}, "replayed notification delivery "+view.ID)
	})
}

func notificationSinkToggleHandler(opts Options, enabled bool) mcp.ToolHandlerFor[NotificationSinkRefInput, ToolEnvelope] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in NotificationSinkRefInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "notification:write", app.PermissionNotificationWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		var view app.NotificationSinkView
		if enabled {
			view, err = svc.EnableNotificationSink(strings.TrimSpace(in.Sink))
		} else {
			view, err = svc.DisableNotificationSink(strings.TrimSpace(in.Sink))
		}
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"sink": notificationSinkViewForMCP(view)}, "notification sink "+view.Name)
	}
}

func reminderRuleToggleHandler(opts Options, enabled bool) mcp.ToolHandlerFor[ReminderRuleRefInput, ToolEnvelope] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in ReminderRuleRefInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "reminder:write", app.PermissionReminderWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		var view app.ReminderRuleView
		if enabled {
			view, err = svc.EnableReminderRule(strings.TrimSpace(in.Rule))
		} else {
			view, err = svc.DisableReminderRule(strings.TrimSpace(in.Rule))
		}
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"rule": reminderRuleViewForMCP(view)}, "reminder rule "+view.Name)
	}
}

func notificationRuleToggleHandler(opts Options, enabled bool) mcp.ToolHandlerFor[NotificationRuleRefInput, ToolEnvelope] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in NotificationRuleRefInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := serviceForTool(ctx, req, opts, in.scopeInput(), "notification:write", app.PermissionNotificationWrite)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		var view app.EventNotificationRuleView
		if enabled {
			view, err = svc.EnableEventNotificationRule(strings.TrimSpace(in.Rule))
		} else {
			view, err = svc.DisableEventNotificationRule(strings.TrimSpace(in.Rule))
		}
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"rule": notificationRuleViewForMCP(view)}, "notification rule "+view.Name)
	}
}

func notificationSinkViewsForMCP(rows []app.NotificationSinkView) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, notificationSinkViewForMCP(row))
	}
	return out
}

func notificationSinkViewForMCP(row app.NotificationSinkView) map[string]any {
	return map[string]any{
		"id":                row.ID,
		"workspace_id":      row.WorkspaceID,
		"name":              row.Name,
		"type":              row.Type,
		"endpoint_mode":     row.EndpointMode,
		"url":               row.URL,
		"url_template":      row.URLTemplate,
		"config_key":        row.ConfigKey,
		"allowed_hosts":     row.AllowedHosts,
		"http_method":       row.HTTPMethod,
		"header_templates":  row.HeaderTemplates,
		"body_template":     row.BodyTemplate,
		"body_content_type": row.BodyContentType,
		"secret_refs":       row.SecretRefs,
		"enabled":           row.Enabled,
		"timeout_seconds":   row.TimeoutSeconds,
		"max_attempts":      row.MaxAttempts,
		"max_concurrency":   row.MaxConcurrency,
		"created_by":        task.UserInfoToJSON(row.CreatedBy),
		"created_at":        row.CreatedAt,
		"modified_at":       row.ModifiedAt,
	}
}

func reminderRuleViewsForMCP(rows []app.ReminderRuleView) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, reminderRuleViewForMCP(row))
	}
	return out
}

func reminderRuleViewForMCP(row app.ReminderRuleView) map[string]any {
	return map[string]any{
		"id":              row.ID,
		"workspace_id":    row.WorkspaceID,
		"project_id":      row.ProjectID,
		"name":            row.Name,
		"enabled":         row.Enabled,
		"trigger_type":    row.TriggerType,
		"offset_seconds":  row.OffsetSeconds,
		"after_seconds":   row.AfterSeconds,
		"repeat_policy":   row.RepeatPolicy,
		"schedule_type":   row.ScheduleType,
		"schedule_value":  row.ScheduleValue,
		"filter_source":   row.FilterSource,
		"audience_type":   row.AudienceType,
		"recipient_users": notificationUserInfosForMCP(row.RecipientUsers),
		"sink_id":         row.SinkID,
		"created_by":      task.UserInfoToJSON(row.CreatedBy),
		"created_at":      row.CreatedAt,
		"modified_at":     row.ModifiedAt,
	}
}

func notificationRuleViewsForMCP(rows []app.EventNotificationRuleView) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, notificationRuleViewForMCP(row))
	}
	return out
}

func notificationRuleViewForMCP(row app.EventNotificationRuleView) map[string]any {
	return map[string]any{
		"id":               row.ID,
		"workspace_id":     row.WorkspaceID,
		"project_id":       row.ProjectID,
		"name":             row.Name,
		"enabled":          row.Enabled,
		"event_type":       row.EventType,
		"filter_source":    row.FilterSource,
		"audience_type":    row.AudienceType,
		"recipient_users":  notificationUserInfosForMCP(row.RecipientUsers),
		"sink_id":          row.SinkID,
		"template_subject": row.TemplateSubject,
		"template_body":    row.TemplateBody,
		"created_by":       task.UserInfoToJSON(row.CreatedBy),
		"created_at":       row.CreatedAt,
		"modified_at":      row.ModifiedAt,
	}
}

func notificationDeliveryViewsForMCP(rows []app.NotificationDeliveryView) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, notificationDeliveryViewForMCP(row))
	}
	return out
}

func notificationDeliveryViewForMCP(row app.NotificationDeliveryView) map[string]any {
	return map[string]any{
		"id":                            row.ID,
		"workspace_id":                  row.WorkspaceID,
		"project_id":                    row.ProjectID,
		"rule_id":                       row.RuleID,
		"sink_id":                       row.SinkID,
		"task_uuid":                     row.TaskUUID,
		"object_kind":                   row.ObjectKind,
		"object_id":                     row.ObjectID,
		"recipient":                     task.UserInfoToJSON(row.Recipient),
		"event_id":                      row.EventID,
		"event_type":                    row.EventType,
		"resolved_url":                  row.ResolvedURL,
		"resolved_endpoint_source":      row.ResolvedEndpointSource,
		"resolved_endpoint_fingerprint": row.ResolvedEndpointFingerprint,
		"rendered_method":               row.RenderedMethod,
		"rendered_headers":              row.RenderedHeaders,
		"rendered_body":                 row.RenderedBody,
		"rendered_content_type":         row.RenderedContentType,
		"payload":                       row.Payload,
		"status":                        row.Status,
		"attempt_count":                 row.AttemptCount,
		"next_attempt_at":               row.NextAttemptAt,
		"claim_expires_at":              row.ClaimExpiresAt,
		"last_attempt_at":               row.LastAttemptAt,
		"last_status_code":              row.LastStatusCode,
		"last_error":                    row.LastError,
		"created_at":                    row.CreatedAt,
		"modified_at":                   row.ModifiedAt,
	}
}

func notificationUserInfosForMCP(rows []task.UserInfo) []task.JSONUserInfo {
	out := make([]task.JSONUserInfo, 0, len(rows))
	for _, row := range rows {
		out = append(out, task.UserInfoToJSON(row))
	}
	return out
}
