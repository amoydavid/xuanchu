package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/task"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// automationProjectServiceForTool 复刻 HTTP scopedProjectAutomationService 的双 scope 校验：
// project 与 hook 两个维度各自要求 capability/permission，返回 project 维度构造的 scoped service。
// write=true 对应 project:write + PermissionProjectManage 与 hook:write + PermissionHookWrite。
func automationProjectServiceForTool(ctx context.Context, req *mcp.CallToolRequest, opts Options, input RequestScopeInput, write bool) (*app.Service, error) {
	projectCapability, projectPermission := "project:read", app.PermissionProjectRead
	hookCapability, hookPermission := "hook:read", app.PermissionHookRead
	if write {
		projectCapability, projectPermission = "project:write", app.PermissionProjectManage
		hookCapability, hookPermission = "hook:write", app.PermissionHookWrite
	}
	scoped, err := serviceForTool(ctx, req, opts, input, projectCapability, projectPermission)
	if err != nil {
		return nil, err
	}
	if _, err := serviceForTool(ctx, req, opts, input, hookCapability, hookPermission); err != nil {
		return nil, err
	}
	return scoped, nil
}

type ProjectAutomationListInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`

	IncludeDisabled *bool `json:"include_disabled,omitempty" jsonschema:"include disabled rules (default false)"`
}

func (in ProjectAutomationListInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type ProjectAutomationRefInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`

	RuleID string `json:"rule_id" jsonschema:"automation rule ID"`
}

func (in ProjectAutomationRefInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

// automationTriggerConfigInput 镜像 app.ProjectAutomationTriggerConfig 的 JSON 形状。
type automationTriggerConfigInput struct {
	ScheduleType  string `json:"schedule_type,omitempty" jsonschema:"schedule trigger type: daily_at or cron"`
	ScheduleValue string `json:"schedule_value,omitempty" jsonschema:"HH:MM for daily_at, or a 5-field cron expression"`
	Timezone      string `json:"timezone,omitempty" jsonschema:"IANA timezone for schedule evaluation"`
	EventType     string `json:"event_type,omitempty" jsonschema:"hook event type for event trigger (task.assigned, task.completed, project.transitioned, ...)"`
}

// automationConditionInput 镜像 app.ProjectAutomationCondition 的 JSON 形状。
type automationConditionInput struct {
	TaskFilter         string `json:"task_filter,omitempty" jsonschema:"task filter applied to the matched task set"`
	MaxTasks           int    `json:"max_tasks,omitempty" jsonschema:"max tasks included in delivery context (default 50)"`
	OnlyAddedAssignees bool   `json:"only_added_assignees,omitempty" jsonschema:"for task.assigned events, only fire when assignees were actually added"`
}

// automationActionInput 镜像 app.ProjectAutomationActionConfig 的 JSON 形状。
type automationActionInput struct {
	Protocol              string  `json:"protocol,omitempty" jsonschema:"delivery protocol, only chat_completions (default)"`
	BaseURLConfigKey      string  `json:"base_url_config_key" jsonschema:"scoped config key holding the provider base URL"`
	APIKeyConfigKey       string  `json:"api_key_config_key" jsonschema:"scoped config key holding the provider API key"`
	ModelConfigKey        string  `json:"model_config_key,omitempty" jsonschema:"scoped config key holding the model name (or use model_override)"`
	AllowedHostsConfigKey string  `json:"allowed_hosts_config_key,omitempty" jsonschema:"scoped config key holding the JSON host allowlist (default agent.provider.allowed_hosts)"`
	ModelOverride         string  `json:"model_override,omitempty" jsonschema:"fixed model name, bypassing model_config_key"`
	Temperature           float64 `json:"temperature,omitempty" jsonschema:"sampling temperature (default 0.2)"`
	MaxAttempts           int     `json:"max_attempts,omitempty" jsonschema:"max delivery attempts including retries (default 5)"`
	AttachMetadata        bool    `json:"attach_metadata,omitempty" jsonschema:"attach xuanchu source metadata to the request body"`
}

// automationContextInput 镜像 app.ProjectAutomationContextConfig 的 JSON 形状。
type automationContextInput struct {
	Include []string `json:"include,omitempty" jsonschema:"context include keys (legacy; template variables are always filled)"`
}

type ProjectAutomationAddInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`

	Name                string                      `json:"name" jsonschema:"rule name"`
	Description         string                      `json:"description,omitempty" jsonschema:"rule description"`
	Enabled             *bool                       `json:"enabled,omitempty" jsonschema:"whether the rule starts enabled (default true)"`
	TriggerType         string                      `json:"trigger_type" jsonschema:"trigger type: schedule or event"`
	TriggerConfig       automationTriggerConfigInput `json:"trigger_config" jsonschema:"trigger configuration"`
	Condition           automationConditionInput    `json:"condition,omitempty" jsonschema:"task filter and event conditions"`
	Action              automationActionInput       `json:"action" jsonschema:"OpenAI-compatible delivery action"`
	Context             automationContextInput      `json:"context,omitempty" jsonschema:"delivery context include list"`
	InstructionTemplate string                      `json:"instruction_template" jsonschema:"user message template with {{var}} placeholders"`
	SystemPrompt        string                      `json:"system_prompt,omitempty" jsonschema:"system prompt template (default built-in agent prompt)"`
}

func (in ProjectAutomationAddInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type ProjectAutomationModifyInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`

	RuleID                string                       `json:"rule_id" jsonschema:"automation rule ID"`
	Name                  *string                      `json:"name,omitempty" jsonschema:"new rule name"`
	Description           *string                      `json:"description,omitempty" jsonschema:"new rule description"`
	Enabled               *bool                        `json:"enabled,omitempty" jsonschema:"whether the rule is enabled"`
	TriggerType           *string                      `json:"trigger_type,omitempty" jsonschema:"new trigger type: schedule or event"`
	TriggerConfig         *automationTriggerConfigInput `json:"trigger_config,omitempty" jsonschema:"new trigger configuration"`
	Condition             *automationConditionInput    `json:"condition,omitempty" jsonschema:"new task filter and event conditions"`
	Action                *automationActionInput       `json:"action,omitempty" jsonschema:"new OpenAI-compatible delivery action"`
	Context               *automationContextInput      `json:"context,omitempty" jsonschema:"new delivery context include list"`
	InstructionTemplate   *string                      `json:"instruction_template,omitempty" jsonschema:"new user message template"`
	SystemPrompt          *string                      `json:"system_prompt,omitempty" jsonschema:"new system prompt template"`
}

func (in ProjectAutomationModifyInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type ProjectAutomationPreviewSavedInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`

	RuleID string `json:"rule_id" jsonschema:"automation rule ID"`
}

func (in ProjectAutomationPreviewSavedInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type ProjectAutomationDeliveryListInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`

	RuleID string `json:"rule_id,omitempty" jsonschema:"filter by automation rule ID"`
	Status string `json:"status,omitempty" jsonschema:"filter by delivery status (queued, claiming, succeeded, retried, failed)"`
	Limit  int    `json:"limit,omitempty" jsonschema:"max deliveries to return (default 20)"`
	Offset int    `json:"offset,omitempty" jsonschema:"number of deliveries to skip"`
}

func (in ProjectAutomationDeliveryListInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type ProjectAutomationDeliveryRefInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`

	DeliveryID string `json:"delivery_id" jsonschema:"automation delivery ID"`
}

func (in ProjectAutomationDeliveryRefInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

type ProjectAutomationTemplateVarsInput struct {
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

func (in ProjectAutomationTemplateVarsInput) scopeInput() RequestScopeInput {
	return RequestScopeInput{Workspace: in.Workspace, Project: in.Project, ProjectID: in.ProjectID}
}

func automationTriggerConfigFromInput(in automationTriggerConfigInput) app.ProjectAutomationTriggerConfig {
	return app.ProjectAutomationTriggerConfig{
		ScheduleType:  in.ScheduleType,
		ScheduleValue: in.ScheduleValue,
		Timezone:      in.Timezone,
		EventType:     in.EventType,
	}
}

func automationConditionFromInput(in automationConditionInput) app.ProjectAutomationCondition {
	return app.ProjectAutomationCondition{
		TaskFilter:         in.TaskFilter,
		MaxTasks:           in.MaxTasks,
		OnlyAddedAssignees: in.OnlyAddedAssignees,
	}
}

func automationActionFromInput(in automationActionInput) app.ProjectAutomationActionConfig {
	return app.ProjectAutomationActionConfig{
		Protocol:              in.Protocol,
		BaseURLConfigKey:      in.BaseURLConfigKey,
		APIKeyConfigKey:       in.APIKeyConfigKey,
		ModelConfigKey:        in.ModelConfigKey,
		AllowedHostsConfigKey: in.AllowedHostsConfigKey,
		ModelOverride:         in.ModelOverride,
		Temperature:           in.Temperature,
		MaxAttempts:           in.MaxAttempts,
		AttachMetadata:        in.AttachMetadata,
	}
}

func automationContextFromInput(in automationContextInput) app.ProjectAutomationContextConfig {
	return app.ProjectAutomationContextConfig{Include: in.Include}
}

func automationAddInputFromMCP(in ProjectAutomationAddInput) app.ProjectAutomationRuleAddInput {
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	return app.ProjectAutomationRuleAddInput{
		Name:                in.Name,
		Description:         in.Description,
		Enabled:             enabled,
		TriggerType:         in.TriggerType,
		TriggerConfig:       automationTriggerConfigFromInput(in.TriggerConfig),
		Condition:           automationConditionFromInput(in.Condition),
		Action:              automationActionFromInput(in.Action),
		Context:             automationContextFromInput(in.Context),
		InstructionTemplate: in.InstructionTemplate,
		SystemPrompt:        in.SystemPrompt,
	}
}

func automationModifyInputFromMCP(in ProjectAutomationModifyInput) app.ProjectAutomationRuleModifyInput {
	out := app.ProjectAutomationRuleModifyInput{
		Name:                in.Name,
		Description:         in.Description,
		Enabled:             in.Enabled,
		TriggerType:         in.TriggerType,
		InstructionTemplate: in.InstructionTemplate,
		SystemPrompt:        in.SystemPrompt,
	}
	if in.TriggerConfig != nil {
		cfg := automationTriggerConfigFromInput(*in.TriggerConfig)
		out.TriggerConfig = &cfg
	}
	if in.Condition != nil {
		cond := automationConditionFromInput(*in.Condition)
		out.Condition = &cond
	}
	if in.Action != nil {
		action := automationActionFromInput(*in.Action)
		out.Action = &action
	}
	if in.Context != nil {
		ctxCfg := automationContextFromInput(*in.Context)
		out.Context = &ctxCfg
	}
	return out
}

func registerProjectAutomationTools(s *mcp.Server, opts Options) {
	addTool(s, opts, &mcp.Tool{Name: "project_automation_list", Description: "List project automation rules; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectAutomationListInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := automationProjectServiceForTool(ctx, req, opts, in.scopeInput(), false)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		includeDisabled := in.IncludeDisabled != nil && *in.IncludeDisabled
		rows, err := svc.ListProjectAutomationRules(projectRefForScope(in.Project, in.ProjectID), includeDisabled)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		data := map[string]any{"automations": projectAutomationRuleViewsFromApp(rows), "count": len(rows)}
		return successWithEnvelope(data, fmt.Sprintf("%d automation rule(s)", len(rows)))
	})

	addTool(s, opts, &mcp.Tool{Name: "project_automation_get", Description: "Get one project automation rule; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectAutomationRefInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := automationProjectServiceForTool(ctx, req, opts, in.scopeInput(), false)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.ProjectAutomationRuleInfo(projectRefForScope(in.Project, in.ProjectID), strings.TrimSpace(in.RuleID))
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"automation": projectAutomationRuleViewFromApp(view)}, "automation rule "+view.Name)
	})

	addTool(s, opts, &mcp.Tool{Name: "project_automation_add", Description: "Create a project automation rule with a schedule or event trigger and an OpenAI-compatible delivery action."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectAutomationAddInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := automationProjectServiceForTool(ctx, req, opts, in.scopeInput(), true)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.AddProjectAutomationRule(projectRefForScope(in.Project, in.ProjectID), automationAddInputFromMCP(in))
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"automation": projectAutomationRuleViewFromApp(view)}, "created automation rule "+view.Name)
	})

	addTool(s, opts, &mcp.Tool{Name: "project_automation_modify", Description: "Modify a project automation rule; omitted fields stay unchanged."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectAutomationModifyInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := automationProjectServiceForTool(ctx, req, opts, in.scopeInput(), true)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.ModifyProjectAutomationRule(projectRefForScope(in.Project, in.ProjectID), strings.TrimSpace(in.RuleID), automationModifyInputFromMCP(in))
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"automation": projectAutomationRuleViewFromApp(view)}, "modified automation rule "+view.Name)
	})

	addTool(s, opts, &mcp.Tool{Name: "project_automation_remove", Description: "Delete a project automation rule."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectAutomationRefInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := automationProjectServiceForTool(ctx, req, opts, in.scopeInput(), true)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		ruleID := strings.TrimSpace(in.RuleID)
		if err := svc.DeleteProjectAutomationRule(projectRefForScope(in.Project, in.ProjectID), ruleID); err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"removed": ruleID}, "deleted automation rule")
	})

	addTool(s, opts, &mcp.Tool{Name: "project_automation_enable", Description: "Enable a project automation rule."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectAutomationRefInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := automationProjectServiceForTool(ctx, req, opts, in.scopeInput(), true)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.EnableProjectAutomationRule(projectRefForScope(in.Project, in.ProjectID), strings.TrimSpace(in.RuleID))
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"automation": projectAutomationRuleViewFromApp(view)}, "enabled automation rule "+view.Name)
	})

	addTool(s, opts, &mcp.Tool{Name: "project_automation_disable", Description: "Disable a project automation rule."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectAutomationRefInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := automationProjectServiceForTool(ctx, req, opts, in.scopeInput(), true)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.DisableProjectAutomationRule(projectRefForScope(in.Project, in.ProjectID), strings.TrimSpace(in.RuleID))
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"automation": projectAutomationRuleViewFromApp(view)}, "disabled automation rule "+view.Name)
	})

	addTool(s, opts, &mcp.Tool{Name: "project_automation_test", Description: "Enqueue a manual test delivery for a rule; the background dispatcher sends it to the provider."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectAutomationRefInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := automationProjectServiceForTool(ctx, req, opts, in.scopeInput(), true)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.TestProjectAutomationRule(projectRefForScope(in.Project, in.ProjectID), strings.TrimSpace(in.RuleID))
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"delivery": projectAutomationDeliveryViewFromApp(view)}, "queued test delivery "+view.ID)
	})

	addTool(s, opts, &mcp.Tool{Name: "project_automation_preview", Description: "Preview the rendered OpenAI-compatible request for an unsaved rule; no delivery is written; secrets are masked."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectAutomationAddInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := automationProjectServiceForTool(ctx, req, opts, in.scopeInput(), true)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.PreviewProjectAutomation(projectRefForScope(in.Project, in.ProjectID), app.ProjectAutomationPreviewInput(automationAddInputFromMCP(in)))
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"preview": projectAutomationPreviewViewFromApp(view)}, "preview request for "+view.URL)
	})

	addTool(s, opts, &mcp.Tool{Name: "project_automation_preview_saved", Description: "Preview the rendered request for a saved rule as-is; to preview hypothetical changes use project_automation_preview; secrets are masked."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectAutomationPreviewSavedInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := automationProjectServiceForTool(ctx, req, opts, in.scopeInput(), true)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.PreviewSavedProjectAutomation(projectRefForScope(in.Project, in.ProjectID), strings.TrimSpace(in.RuleID), nil)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"preview": projectAutomationPreviewViewFromApp(view)}, "preview request for "+view.URL)
	})

	addTool(s, opts, &mcp.Tool{Name: "project_automation_delivery_list", Description: "List project automation delivery records; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectAutomationDeliveryListInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := automationProjectServiceForTool(ctx, req, opts, in.scopeInput(), false)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		limit := in.Limit
		if limit <= 0 {
			limit = 20
		}
		rows, err := svc.ListProjectAutomationDeliveries(projectRefForScope(in.Project, in.ProjectID), app.ProjectAutomationDeliveryListInput{
			RuleID: strings.TrimSpace(in.RuleID),
			Status: strings.TrimSpace(in.Status),
			Limit:  limit,
			Offset: in.Offset,
		})
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		data := map[string]any{"deliveries": projectAutomationDeliveryViewsFromApp(rows), "count": len(rows)}
		return successWithEnvelope(data, fmt.Sprintf("%d delivery record(s)", len(rows)))
	})

	addTool(s, opts, &mcp.Tool{Name: "project_automation_delivery_get", Description: "Get one automation delivery record; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectAutomationDeliveryRefInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := automationProjectServiceForTool(ctx, req, opts, in.scopeInput(), false)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.ProjectAutomationDeliveryInfo(projectRefForScope(in.Project, in.ProjectID), strings.TrimSpace(in.DeliveryID))
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"delivery": projectAutomationDeliveryViewFromApp(view)}, "delivery "+view.ID)
	})

	addTool(s, opts, &mcp.Tool{Name: "project_automation_delivery_replay", Description: "Re-enqueue a past delivery as a new queued delivery; the original record is kept unchanged."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectAutomationDeliveryRefInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		svc, err := automationProjectServiceForTool(ctx, req, opts, in.scopeInput(), true)
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		view, err := svc.ReplayProjectAutomationDelivery(projectRefForScope(in.Project, in.ProjectID), strings.TrimSpace(in.DeliveryID))
		if err != nil {
			return businessErrorWithEnvelope(err)
		}
		return successWithEnvelope(map[string]any{"delivery": projectAutomationDeliveryViewFromApp(view)}, "replayed as delivery "+view.ID)
	})

	addTool(s, opts, &mcp.Tool{Name: "project_automation_list_template_vars", Description: "List template variables available in automation instruction and system prompt templates; read-only."}, func(ctx context.Context, req *mcp.CallToolRequest, in ProjectAutomationTemplateVarsInput) (*mcp.CallToolResult, ToolEnvelope, error) {
		if _, err := automationProjectServiceForTool(ctx, req, opts, in.scopeInput(), false); err != nil {
			return businessErrorWithEnvelope(err)
		}
		vars := app.AutomationTemplateVars()
		data := map[string]any{"triggers": automationTemplateVarViewsFromApp(vars)}
		return successWithEnvelope(data, "automation template variables")
	})
}

type automationTriggerConfigView struct {
	ScheduleType  string `json:"schedule_type,omitempty"`
	ScheduleValue string `json:"schedule_value,omitempty"`
	Timezone      string `json:"timezone,omitempty"`
	EventType     string `json:"event_type,omitempty"`
}

type automationConditionView struct {
	TaskFilter         string `json:"task_filter,omitempty"`
	MaxTasks           int    `json:"max_tasks,omitempty"`
	OnlyAddedAssignees bool   `json:"only_added_assignees,omitempty"`
}

type automationActionView struct {
	Protocol              string  `json:"protocol"`
	BaseURLConfigKey      string  `json:"base_url_config_key"`
	APIKeyConfigKey       string  `json:"api_key_config_key"`
	ModelConfigKey        string  `json:"model_config_key,omitempty"`
	AllowedHostsConfigKey string  `json:"allowed_hosts_config_key,omitempty"`
	ModelOverride         string  `json:"model_override,omitempty"`
	Temperature           float64 `json:"temperature"`
	MaxAttempts           int     `json:"max_attempts"`
	AttachMetadata        bool    `json:"attach_metadata,omitempty"`
}

type automationContextView struct {
	Include []string `json:"include"`
}

type projectAutomationRuleView struct {
	ID                  string                   `json:"id"`
	WorkspaceID         string                   `json:"workspace_id"`
	ProjectID           string                   `json:"project_id"`
	Name                string                   `json:"name"`
	Description         string                   `json:"description"`
	Enabled             bool                     `json:"enabled"`
	TriggerType         string                   `json:"trigger_type"`
	TriggerConfig       automationTriggerConfigView `json:"trigger_config"`
	Condition           automationConditionView  `json:"condition"`
	ActionType          string                   `json:"action_type"`
	Action              automationActionView     `json:"action"`
	Context             automationContextView    `json:"context"`
	InstructionTemplate string                   `json:"instruction_template"`
	SystemPrompt        string                   `json:"system_prompt"`
	CreatedBy           task.JSONUserInfo        `json:"created_by"`
	CreatedAt           int64                    `json:"created_at"`
	ModifiedAt          int64                    `json:"modified_at"`
}

type projectAutomationDeliveryView struct {
	ID                  string              `json:"id"`
	WorkspaceID         string              `json:"workspace_id"`
	ProjectID           string              `json:"project_id"`
	RuleID              string              `json:"rule_id"`
	TriggerType         string              `json:"trigger_type"`
	EventID             string              `json:"event_id"`
	EventType           string              `json:"event_type"`
	Status              string              `json:"status"`
	ResolvedURL         string              `json:"resolved_url"`
	RenderedMethod      string              `json:"rendered_method"`
	RenderedHeaders     map[string][]string `json:"rendered_headers"`
	RequestBodyPreview  string              `json:"request_body_preview"`
	RequestBodyHash     string              `json:"request_body_hash"`
	ResponseStatusCode  *int                `json:"response_status_code,omitempty"`
	ResponseBodyPreview string              `json:"response_body_preview,omitempty"`
	ProviderRequestID   string              `json:"provider_request_id,omitempty"`
	Usage               map[string]any      `json:"usage"`
	AttemptCount        int                 `json:"attempt_count"`
	NextAttemptAt       *int64              `json:"next_attempt_at,omitempty"`
	LastError           string              `json:"last_error,omitempty"`
	CreatedAt           int64               `json:"created_at"`
	ModifiedAt          int64               `json:"modified_at"`
}

type projectAutomationPreviewView struct {
	Method   string            `json:"method"`
	URL      string            `json:"url"`
	Headers  map[string]string `json:"headers"`
	Body     map[string]any    `json:"body"`
	Warnings []string          `json:"warnings,omitempty"`
}

type automationTemplateVarView struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsPrefix    bool   `json:"is_prefix"`
}

type automationTemplateTriggerView struct {
	Trigger string                     `json:"trigger"`
	Vars    []automationTemplateVarView `json:"vars"`
}

func projectAutomationRuleViewFromApp(v app.ProjectAutomationRuleView) projectAutomationRuleView {
	return projectAutomationRuleView{
		ID:          v.ID,
		WorkspaceID: v.WorkspaceID,
		ProjectID:   v.ProjectID,
		Name:        v.Name,
		Description: v.Description,
		Enabled:     v.Enabled,
		TriggerType: v.TriggerType,
		TriggerConfig: automationTriggerConfigView{
			ScheduleType:  v.TriggerConfig.ScheduleType,
			ScheduleValue: v.TriggerConfig.ScheduleValue,
			Timezone:      v.TriggerConfig.Timezone,
			EventType:     v.TriggerConfig.EventType,
		},
		Condition: automationConditionView{
			TaskFilter:         v.Condition.TaskFilter,
			MaxTasks:           v.Condition.MaxTasks,
			OnlyAddedAssignees: v.Condition.OnlyAddedAssignees,
		},
		ActionType: v.ActionType,
		Action: automationActionView{
			Protocol:              v.Action.Protocol,
			BaseURLConfigKey:      v.Action.BaseURLConfigKey,
			APIKeyConfigKey:       v.Action.APIKeyConfigKey,
			ModelConfigKey:        v.Action.ModelConfigKey,
			AllowedHostsConfigKey: v.Action.AllowedHostsConfigKey,
			ModelOverride:         v.Action.ModelOverride,
			Temperature:           v.Action.Temperature,
			MaxAttempts:           v.Action.MaxAttempts,
			AttachMetadata:        v.Action.AttachMetadata,
		},
		Context: automationContextView{Include: v.Context.Include},
		InstructionTemplate: v.InstructionTemplate,
		SystemPrompt:        v.SystemPrompt,
		CreatedBy:           task.UserInfoToJSON(v.CreatedBy),
		CreatedAt:           v.CreatedAt,
		ModifiedAt:          v.ModifiedAt,
	}
}

func projectAutomationRuleViewsFromApp(rows []app.ProjectAutomationRuleView) []projectAutomationRuleView {
	out := make([]projectAutomationRuleView, 0, len(rows))
	for _, row := range rows {
		out = append(out, projectAutomationRuleViewFromApp(row))
	}
	return out
}

func projectAutomationDeliveryViewFromApp(v app.ProjectAutomationDeliveryView) projectAutomationDeliveryView {
	return projectAutomationDeliveryView{
		ID:                  v.ID,
		WorkspaceID:         v.WorkspaceID,
		ProjectID:           v.ProjectID,
		RuleID:              v.RuleID,
		TriggerType:         v.TriggerType,
		EventID:             v.EventID,
		EventType:           v.EventType,
		Status:              v.Status,
		ResolvedURL:         v.ResolvedURL,
		RenderedMethod:      v.RenderedMethod,
		RenderedHeaders:     v.RenderedHeaders,
		RequestBodyPreview:  v.RequestBodyPreview,
		RequestBodyHash:     v.RequestBodyHash,
		ResponseStatusCode:  v.ResponseStatusCode,
		ResponseBodyPreview: v.ResponseBodyPreview,
		ProviderRequestID:   v.ProviderRequestID,
		Usage:               v.Usage,
		AttemptCount:        v.AttemptCount,
		NextAttemptAt:       v.NextAttemptAt,
		LastError:           v.LastError,
		CreatedAt:           v.CreatedAt,
		ModifiedAt:          v.ModifiedAt,
	}
}

func projectAutomationDeliveryViewsFromApp(rows []app.ProjectAutomationDeliveryView) []projectAutomationDeliveryView {
	out := make([]projectAutomationDeliveryView, 0, len(rows))
	for _, row := range rows {
		out = append(out, projectAutomationDeliveryViewFromApp(row))
	}
	return out
}

func projectAutomationPreviewViewFromApp(v app.ProjectAutomationPreviewView) projectAutomationPreviewView {
	return projectAutomationPreviewView{
		Method:   v.Method,
		URL:      v.URL,
		Headers:  v.Headers,
		Body:     v.Body,
		Warnings: v.Warnings,
	}
}

func automationTemplateVarViewsFromApp(v app.AutomationTemplateVarsView) []automationTemplateTriggerView {
	out := make([]automationTemplateTriggerView, 0, len(v.Triggers))
	for _, trigger := range v.Triggers {
		vars := make([]automationTemplateVarView, 0, len(trigger.Vars))
		for _, variable := range trigger.Vars {
			vars = append(vars, automationTemplateVarView{
				Name:        variable.Name,
				Description: variable.Description,
				IsPrefix:    variable.IsPrefix,
			})
		}
		out = append(out, automationTemplateTriggerView{Trigger: trigger.Trigger, Vars: vars})
	}
	return out
}
