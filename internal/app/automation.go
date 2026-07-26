package app

import (
	"strings"

	"github.com/google/uuid"

	"git.dajee.net/dajee/xuanchu/internal/schedule"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

// AutomationScopeType 标识规则归属的作用域。
type AutomationScopeType string

const (
	AutomationScopeWorkspace = AutomationScopeType(storage.AutomationScopeWorkspace)
	AutomationScopeProject   = AutomationScopeType(storage.AutomationScopeProject)
)

// AutomationScope 是规则匹配/作用域解析的最小输入。
// workspace scope 下 ID 必须等于当前 workspace_id；project scope 下 ID 为 project_id。
type AutomationScope struct {
	Type AutomationScopeType
	ID   string
}

// AutomationEventType 是 Workspace Automation 支持的事件类型白名单。
const AutomationEventTypeProjectCreated = "project.created"

// workspaceAutomationAllowedEvents 是 Workspace scope 规则允许的事件类型。
// 首版只开放 project.created；Hook/Notification 白名单仍由 allowedHookEventTypes 维护，
// 两者故意分开，避免 project.created 被错误订阅为 Hook。
var workspaceAutomationAllowedEvents = map[string]bool{
	AutomationEventTypeProjectCreated: true,
}

// AutomationRuleInput 是 scope-aware 的规则写入参数，Project/Workspace scope 共用。
type AutomationRuleInput struct {
	Name                string
	Description         string
	Enabled             bool
	TriggerType         string
	TriggerConfig       ProjectAutomationTriggerConfig
	Condition           ProjectAutomationCondition
	Action              ProjectAutomationActionConfig
	Context             ProjectAutomationContextConfig
	InstructionTemplate string
	SystemPrompt        string
	presetID            string
}

// AutomationRuleView 是 scope-aware 规则视图。ProjectID 仅在 project scope 下非空。
type AutomationRuleView struct {
	ID                  string                         `json:"id"`
	WorkspaceID         string                         `json:"workspace_id"`
	ScopeType           string                         `json:"scope_type"`
	ScopeID             string                         `json:"scope_id"`
	ProjectID           string                         `json:"project_id,omitempty"`
	Name                string                         `json:"name"`
	Description         string                         `json:"description"`
	Enabled             bool                           `json:"enabled"`
	TriggerType         string                         `json:"trigger_type"`
	TriggerConfig       ProjectAutomationTriggerConfig `json:"trigger_config"`
	Condition           ProjectAutomationCondition     `json:"condition"`
	ActionType          string                         `json:"action_type"`
	Action              ProjectAutomationActionConfig  `json:"action"`
	Context             ProjectAutomationContextConfig `json:"context"`
	InstructionTemplate string                         `json:"instruction_template"`
	SystemPrompt        string                         `json:"system_prompt"`
	CreatedBy           task.UserInfo                  `json:"created_by"`
	CreatedAt           int64                          `json:"created_at"`
	ModifiedAt          int64                          `json:"modified_at"`
	LastDelivery        *AutomationRuleLastDelivery    `json:"last_delivery,omitempty"`
}

// AutomationRuleLastDelivery 是规则最近一次 Delivery 的安全摘要，
// 不含 secret、request body 或 response body。
type AutomationRuleLastDelivery struct {
	DeliveryID         string `json:"delivery_id"`
	Status             string `json:"status"`
	ResponseStatusCode *int   `json:"response_status_code,omitempty"`
	CreatedAt          int64  `json:"created_at"`
}

// AutomationRuleModifyInput 是 scope-aware 修改参数。
type AutomationRuleModifyInput struct {
	Name                *string
	Description         *string
	Enabled             *bool
	TriggerType         *string
	TriggerConfig       *ProjectAutomationTriggerConfig
	Condition           *ProjectAutomationCondition
	Action              *ProjectAutomationActionConfig
	Context             *ProjectAutomationContextConfig
	InstructionTemplate *string
	SystemPrompt        *string
}

// withPresetID 让 Project Template instantiate 等内部调用复用 normalization 并指定 ID。
func (input AutomationRuleInput) withPresetID(id string) AutomationRuleInput {
	input.presetID = id
	return input
}

// normalizeAutomationScope 校验 scope type/id 是否合法，并强制 workspace scope 下
// scope_id == workspace_id，避免同一 Workspace 下出现“作用域错位”的规则。
func (s *Service) normalizeAutomationScope(scope AutomationScope) (AutomationScope, error) {
	switch scope.Type {
	case AutomationScopeWorkspace:
		if scope.ID != "" && scope.ID != s.workspaceID {
			return scope, RuntimeError{Code: "automation_scope_invalid", Message: "workspace scope id must match current workspace"}
		}
		return AutomationScope{Type: AutomationScopeWorkspace, ID: s.workspaceID}, nil
	case AutomationScopeProject:
		if strings.TrimSpace(scope.ID) == "" {
			return scope, RuntimeError{Code: "automation_scope_invalid", Message: "project scope requires project id"}
		}
		return AutomationScope{Type: AutomationScopeProject, ID: scope.ID}, nil
	default:
		return scope, RuntimeError{Code: "automation_scope_invalid", Message: "unsupported scope type"}
	}
}

// normalizeAutomationRuleInput 校验规则输入。Workspace 与 Project scope 共用此函数，
// 仅在事件白名单上按 scope 区分：Workspace 只允许 project.created，Project 沿用 Hook 白名单。
func normalizeAutomationRuleInput(scope AutomationScope, input AutomationRuleInput) (AutomationRuleInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.TriggerType = strings.TrimSpace(input.TriggerType)
	input.InstructionTemplate = strings.TrimSpace(input.InstructionTemplate)
	input.SystemPrompt = strings.TrimSpace(input.SystemPrompt)
	if input.Name == "" {
		return input, RuntimeError{Code: "automation_rule_invalid", Message: "rule name is required"}
	}
	if input.TriggerType != ProjectAutomationTriggerSchedule && input.TriggerType != ProjectAutomationTriggerEvent {
		return input, RuntimeError{Code: "automation_rule_invalid", Message: "unsupported trigger"}
	}
	if input.TriggerType == ProjectAutomationTriggerSchedule {
		spec := schedule.Spec{
			Type:     input.TriggerConfig.ScheduleType,
			Value:    input.TriggerConfig.ScheduleValue,
			Timezone: input.TriggerConfig.Timezone,
		}
		if err := spec.Validate(); err != nil {
			return input, RuntimeError{Code: "automation_rule_invalid", Message: "invalid schedule: " + err.Error()}
		}
		if strings.TrimSpace(input.TriggerConfig.Timezone) == "" {
			input.TriggerConfig.Timezone = schedule.DefaultTimezone
		}
	}
	if input.TriggerType == ProjectAutomationTriggerEvent {
		allowed := false
		switch scope.Type {
		case AutomationScopeWorkspace:
			allowed = workspaceAutomationAllowedEvents[input.TriggerConfig.EventType]
		case AutomationScopeProject:
			allowed = allowedHookEventTypes[input.TriggerConfig.EventType]
		}
		if !allowed {
			return input, RuntimeError{Code: "automation_event_unsupported", Message: "unsupported event"}
		}
	}
	if input.Action.Protocol == "" {
		input.Action.Protocol = "chat_completions"
	}
	if input.Action.Protocol != "chat_completions" {
		return input, RuntimeError{Code: "automation_rule_invalid", Message: "unsupported protocol"}
	}
	if input.Action.BaseURLConfigKey == "" || input.Action.APIKeyConfigKey == "" || (input.Action.ModelConfigKey == "" && input.Action.ModelOverride == "") {
		return input, RuntimeError{Code: "automation_rule_invalid", Message: "provider config is required"}
	}
	if input.Action.Temperature == 0 {
		input.Action.Temperature = 0.2
	}
	if input.Action.MaxAttempts <= 0 {
		input.Action.MaxAttempts = 5
	}
	if input.Action.AllowedHostsConfigKey == "" {
		input.Action.AllowedHostsConfigKey = "agent.provider.allowed_hosts"
	}
	if input.Condition.MaxTasks <= 0 {
		input.Condition.MaxTasks = 50
	}
	if input.InstructionTemplate == "" {
		return input, RuntimeError{Code: "automation_rule_invalid", Message: "instruction template is required"}
	}
	return input, nil
}

// automationRuleInputFromRow 把 storage.AutomationRule 还原为 AutomationRuleInput，
// 供 modify/preview/test 复用同一套 normalization。
func automationRuleInputFromRow(row storage.AutomationRule) AutomationRuleInput {
	return AutomationRuleInput{
		Name:                row.Name,
		Description:         row.Description,
		Enabled:             row.Enabled == nil || *row.Enabled,
		TriggerType:         row.TriggerType,
		TriggerConfig:       decodeProjectAutomationTriggerConfig(row.TriggerConfigJSON),
		Condition:           decodeProjectAutomationCondition(row.ConditionJSON),
		Action:              decodeProjectAutomationActionConfig(row.ActionConfigJSON),
		Context:             decodeProjectAutomationContextConfig(row.ContextConfigJSON),
		InstructionTemplate: row.InstructionTemplate,
		SystemPrompt:        row.SystemPrompt,
	}
}

// defaultAutomationSystemPromptForScope 返回 scope 默认 system prompt。
// Workspace 使用“工作空间自动化”措辞，Project 保持现有文案以避免静默改变历史规则。
func defaultAutomationSystemPromptForScope(scope AutomationScope) string {
	if scope.Type == AutomationScopeWorkspace {
		return "你是工作空间自动化执行 Agent。你会收到来自璇础的工作空间、Project 与事件上下文，请按用户指令执行。需要调用外部系统时，使用你所在 Agent 平台已配置的工具、skill、MCP 或 CLI。"
	}
	return defaultAutomationSystemPrompt
}

// addAutomationRuleLocked 在调用方事务内创建规则。Project scope 强制 closed project 校验；
// Workspace scope 直接写入。供 Workspace Automation API 和 Project Template instantiate 共用。
func (s *Service) addAutomationRuleLocked(scope AutomationScope, input AutomationRuleInput) (AutomationRuleView, error) {
	normalized, err := normalizeAutomationRuleInput(scope, input)
	if err != nil {
		return AutomationRuleView{}, err
	}
	now := s.clock.Unix()
	row := storage.AutomationRule{
		ID:                  input.presetID,
		WorkspaceID:         s.workspaceID,
		ScopeType:           string(scope.Type),
		ScopeID:             scope.ID,
		Name:                normalized.Name,
		Description:         normalized.Description,
		Enabled:             &normalized.Enabled,
		TriggerType:         normalized.TriggerType,
		TriggerConfigJSON:   mustJSON(normalized.TriggerConfig),
		ConditionJSON:       mustJSON(normalized.Condition),
		ActionType:          ProjectAutomationActionOpenAI,
		ActionConfigJSON:    mustJSON(normalized.Action),
		ContextConfigJSON:   mustJSON(normalized.Context),
		InstructionTemplate: normalized.InstructionTemplate,
		SystemPrompt:        normalized.SystemPrompt,
		CreatedAt:           now,
		ModifiedAt:          now,
	}
	if row.ID == "" {
		row.ID = uuid.NewString()
	}
	actor := s.runtime.actorColumns()
	row.CreatedByActorType = actor.Type
	row.CreatedByUserID = actor.UserID
	row.CreatedByTokenID = actor.TokenID
	row.CreatedByTokenName = actor.TokenName
	row.CreatedByTokenPrefix = actor.TokenPrefix
	if err := s.projectAutomationRuleRepo.Create(row); err != nil {
		return AutomationRuleView{}, err
	}
	created, err := s.projectAutomationRuleRepo.GetByID(row.ID)
	if err != nil {
		return AutomationRuleView{}, err
	}
	scope.ID = row.ScopeID
	return s.automationRuleViewFromRow(created)
}

// automationRuleViewFromRow 是 scope-aware 视图构造，ProjectID 仅在 project scope 下填值。
func (s *Service) automationRuleViewFromRow(row storage.AutomationRule) (AutomationRuleView, error) {
	users, err := s.resolveUserInfos([]string{valueOrEmpty(row.CreatedByUserID)})
	if err != nil {
		return AutomationRuleView{}, err
	}
	createdBy := users[valueOrEmpty(row.CreatedByUserID)]
	if createdBy.ID == "" {
		createdBy = task.UserInfo{ID: valueOrEmpty(row.CreatedByUserID), Name: valueOrEmpty(row.CreatedByUserID)}
	}
	input := automationRuleInputFromRow(row)
	view := AutomationRuleView{
		ID:                  row.ID,
		WorkspaceID:         row.WorkspaceID,
		ScopeType:           row.ScopeType,
		ScopeID:             row.ScopeID,
		Name:                row.Name,
		Description:         row.Description,
		Enabled:             input.Enabled,
		TriggerType:         row.TriggerType,
		TriggerConfig:       input.TriggerConfig,
		Condition:           input.Condition,
		ActionType:          row.ActionType,
		Action:              input.Action,
		Context:             input.Context,
		InstructionTemplate: row.InstructionTemplate,
		SystemPrompt:        row.SystemPrompt,
		CreatedBy:           createdBy,
		CreatedAt:           row.CreatedAt,
		ModifiedAt:          row.ModifiedAt,
	}
	if row.ScopeType == storage.AutomationScopeProject {
		view.ProjectID = row.ScopeID
	}
	return view, nil
}

// fillAutomationRuleLastDeliveries 批量填充每条规则的 LastDelivery 摘要，
// 避免上层做 N+1 查询。无 Delivery 的规则不填 LastDelivery 字段。
func (s *Service) fillAutomationRuleLastDeliveries(views []AutomationRuleView) error {
	if len(views) == 0 {
		return nil
	}
	ruleIDs := make([]string, 0, len(views))
	for _, v := range views {
		ruleIDs = append(ruleIDs, v.ID)
	}
	latest, err := s.projectAutomationDeliveryRepo.LatestByRuleIDs(ruleIDs)
	if err != nil {
		return err
	}
	for i := range views {
		entry, ok := latest[views[i].ID]
		if !ok {
			continue
		}
		views[i].LastDelivery = &AutomationRuleLastDelivery{
			DeliveryID:         entry.DeliveryID,
			Status:             entry.Status,
			ResponseStatusCode: entry.ResponseStatusCode,
			CreatedAt:          entry.CreatedAt,
		}
	}
	return nil
}

// withAutomationRuleAudit 把 scope/changed fields 写进 audit payload，
// 不写入完整 prompt/request body/secret。
func automationRuleAuditPayload(scope AutomationScope, rule storage.AutomationRule, changedFields ...string) map[string]any {
	payload := map[string]any{
		"scope_type": string(scope.Type),
		"scope_id":   scope.ID,
		"rule_id":    rule.ID,
		"workspace":  rule.WorkspaceID,
	}
	if scope.Type == AutomationScopeProject {
		payload["project"] = scope.ID
	}
	if len(changedFields) > 0 {
		payload["changed"] = changedFields
	}
	return payload
}

// AutomationRuleListInput 是 scope-aware 列表查询条件。
type AutomationRuleListInput struct {
	Scope           AutomationScope
	IncludeDisabled bool
}

// ListAutomationRules 列出指定 scope 下的规则。
func (s *Service) ListAutomationRules(input AutomationRuleListInput) ([]AutomationRuleView, error) {
	if err := s.requireAutomationRead(input.Scope); err != nil {
		return nil, err
	}
	scope, err := s.normalizeAutomationScope(input.Scope)
	if err != nil {
		return nil, err
	}
	rows, err := s.projectAutomationRuleRepo.ListScope(s.workspaceID, string(scope.Type), scope.ID, input.IncludeDisabled)
	if err != nil {
		return nil, err
	}
	out := make([]AutomationRuleView, 0, len(rows))
	for _, row := range rows {
		view, err := s.automationRuleViewFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, view)
	}
	if err := s.fillAutomationRuleLastDeliveries(out); err != nil {
		return nil, err
	}
	return out, nil
}

// AutomationRuleInfo 读取单条规则详情。
func (s *Service) AutomationRuleInfo(scope AutomationScope, ruleID string) (AutomationRuleView, error) {
	if err := s.requireAutomationRead(scope); err != nil {
		return AutomationRuleView{}, err
	}
	normalized, err := s.normalizeAutomationScope(scope)
	if err != nil {
		return AutomationRuleView{}, err
	}
	row, err := s.projectAutomationRuleRepo.GetByID(ruleID)
	if err != nil {
		return AutomationRuleView{}, err
	}
	if row.WorkspaceID != s.workspaceID || row.ScopeType != string(normalized.Type) || row.ScopeID != normalized.ID {
		return AutomationRuleView{}, RuntimeError{Code: "automation_rule_not_found", Message: "automation rule not found"}
	}
	return s.automationRuleViewFromRow(row)
}

// AddAutomationRule 创建 scope-aware 规则并写 audit。Project scope 走 closed project 校验。
func (s *Service) AddAutomationRule(scope AutomationScope, input AutomationRuleInput) (AutomationRuleView, error) {
	if err := s.requireAutomationWrite(scope); err != nil {
		return AutomationRuleView{}, err
	}
	normalized, err := s.normalizeAutomationScope(scope)
	if err != nil {
		return AutomationRuleView{}, err
	}
	if normalized.Type == AutomationScopeProject {
		project, err := s.ResolveProject(normalized.ID)
		if err != nil {
			return AutomationRuleView{}, err
		}
		if isProjectClosed(project) {
			return AutomationRuleView{}, RuntimeError{Code: "project_closed", Message: "project is closed"}
		}
	}
	var created AutomationRuleView
	err = s.withAudit("automation.rule.created", func(tx *Service) (AuditEntry, error) {
		view, err := tx.addAutomationRuleLocked(normalized, input)
		if err != nil {
			return AuditEntry{}, err
		}
		created = view
		row, err := tx.projectAutomationRuleRepo.GetByID(view.ID)
		if err != nil {
			return AuditEntry{}, err
		}
		entry := AuditEntry{Action: "automation.rule.created", TargetType: "automation_rule", TargetID: view.ID}
		entry.WorkspaceID = &row.WorkspaceID
		if normalized.Type == AutomationScopeProject {
			projectID := row.ScopeID
			entry.ProjectID = &projectID
		}
		entry.Payload = automationRuleAuditPayload(normalized, row)
		return entry, nil
	})
	if err != nil {
		return AutomationRuleView{}, err
	}
	return created, nil
}

// ModifyAutomationRule 修改 scope-aware 规则。
func (s *Service) ModifyAutomationRule(scope AutomationScope, ruleID string, input AutomationRuleModifyInput) (AutomationRuleView, error) {
	if err := s.requireAutomationWrite(scope); err != nil {
		return AutomationRuleView{}, err
	}
	normalized, err := s.normalizeAutomationScope(scope)
	if err != nil {
		return AutomationRuleView{}, err
	}
	if normalized.Type == AutomationScopeProject {
		project, err := s.ResolveProject(normalized.ID)
		if err != nil {
			return AutomationRuleView{}, err
		}
		if isProjectClosed(project) {
			return AutomationRuleView{}, RuntimeError{Code: "project_closed", Message: "project is closed"}
		}
	}
	row, err := s.projectAutomationRuleRepo.GetByID(ruleID)
	if err != nil {
		return AutomationRuleView{}, err
	}
	if row.WorkspaceID != s.workspaceID || row.ScopeType != string(normalized.Type) || row.ScopeID != normalized.ID {
		return AutomationRuleView{}, RuntimeError{Code: "automation_rule_not_found", Message: "automation rule not found"}
	}
	next := automationRuleInputFromRow(row)
	changedFields := make([]string, 0, 10)
	if input.Name != nil {
		next.Name = *input.Name
		changedFields = append(changedFields, "name")
	}
	if input.Description != nil {
		next.Description = *input.Description
		changedFields = append(changedFields, "description")
	}
	if input.Enabled != nil {
		next.Enabled = *input.Enabled
		changedFields = append(changedFields, "enabled")
	}
	if input.TriggerType != nil {
		next.TriggerType = *input.TriggerType
		changedFields = append(changedFields, "trigger_type")
	}
	if input.TriggerConfig != nil {
		next.TriggerConfig = *input.TriggerConfig
		changedFields = append(changedFields, "trigger_config")
	}
	if input.Condition != nil {
		next.Condition = *input.Condition
		changedFields = append(changedFields, "condition")
	}
	if input.Action != nil {
		next.Action = *input.Action
		changedFields = append(changedFields, "action")
	}
	if input.Context != nil {
		next.Context = *input.Context
		changedFields = append(changedFields, "context")
	}
	if input.InstructionTemplate != nil {
		next.InstructionTemplate = *input.InstructionTemplate
		changedFields = append(changedFields, "instruction_template")
	}
	if input.SystemPrompt != nil {
		next.SystemPrompt = *input.SystemPrompt
		changedFields = append(changedFields, "system_prompt")
	}
	normalizedInput, err := normalizeAutomationRuleInput(normalized, next)
	if err != nil {
		return AutomationRuleView{}, err
	}
	row.Name = normalizedInput.Name
	row.Description = normalizedInput.Description
	row.Enabled = &normalizedInput.Enabled
	row.TriggerType = normalizedInput.TriggerType
	row.TriggerConfigJSON = mustJSON(normalizedInput.TriggerConfig)
	row.ConditionJSON = mustJSON(normalizedInput.Condition)
	row.ActionConfigJSON = mustJSON(normalizedInput.Action)
	row.ContextConfigJSON = mustJSON(normalizedInput.Context)
	row.InstructionTemplate = normalizedInput.InstructionTemplate
	row.SystemPrompt = normalizedInput.SystemPrompt
	row.ModifiedAt = s.clock.Unix()
	if err := s.projectAutomationRuleRepo.Update(row); err != nil {
		return AutomationRuleView{}, err
	}
	if err := s.appendAutomationRuleAudit("automation.rule.modified", normalized, row, changedFields); err != nil {
		return AutomationRuleView{}, err
	}
	return s.automationRuleViewFromRow(row)
}

// EnableAutomationRule 启用规则。
func (s *Service) EnableAutomationRule(scope AutomationScope, ruleID string) (AutomationRuleView, error) {
	enabled := true
	view, err := s.ModifyAutomationRule(scope, ruleID, AutomationRuleModifyInput{Enabled: &enabled})
	if err != nil {
		return AutomationRuleView{}, err
	}
	row, err := s.projectAutomationRuleRepo.GetByID(ruleID)
	if err != nil {
		return AutomationRuleView{}, err
	}
	normalized, _ := s.normalizeAutomationScope(scope)
	_ = s.appendAutomationRuleAudit("automation.rule.enabled", normalized, row, nil)
	return view, nil
}

// DisableAutomationRule 停用规则。
func (s *Service) DisableAutomationRule(scope AutomationScope, ruleID string) (AutomationRuleView, error) {
	enabled := false
	view, err := s.ModifyAutomationRule(scope, ruleID, AutomationRuleModifyInput{Enabled: &enabled})
	if err != nil {
		return AutomationRuleView{}, err
	}
	row, err := s.projectAutomationRuleRepo.GetByID(ruleID)
	if err != nil {
		return AutomationRuleView{}, err
	}
	normalized, _ := s.normalizeAutomationScope(scope)
	_ = s.appendAutomationRuleAudit("automation.rule.disabled", normalized, row, nil)
	return view, nil
}

// DeleteAutomationRule 删除 scope-aware 规则。
func (s *Service) DeleteAutomationRule(scope AutomationScope, ruleID string) error {
	if err := s.requireAutomationWrite(scope); err != nil {
		return err
	}
	normalized, err := s.normalizeAutomationScope(scope)
	if err != nil {
		return err
	}
	if normalized.Type == AutomationScopeProject {
		project, err := s.ResolveProject(normalized.ID)
		if err != nil {
			return err
		}
		if isProjectClosed(project) {
			return RuntimeError{Code: "project_closed", Message: "project is closed"}
		}
	}
	row, err := s.projectAutomationRuleRepo.GetByID(ruleID)
	if err != nil {
		return err
	}
	if row.WorkspaceID != s.workspaceID || row.ScopeType != string(normalized.Type) || row.ScopeID != normalized.ID {
		return RuntimeError{Code: "automation_rule_not_found", Message: "automation rule not found"}
	}
	if err := s.projectAutomationRuleRepo.Delete(ruleID); err != nil {
		return err
	}
	return s.appendAutomationRuleAudit("automation.rule.deleted", normalized, row, nil)
}

// requireAutomationRead 检查规则/运行记录读取权限。
// Project scope：project:read + hook:read；Workspace scope：workspace:read + hook:read。
func (s *Service) requireAutomationRead(scope AutomationScope) error {
	switch scope.Type {
	case AutomationScopeWorkspace:
		if err := s.Require(PermissionWorkspaceRead); err != nil {
			return err
		}
		return s.Require(PermissionHookRead)
	default:
		return s.requireProjectAutomationRead()
	}
}

// requireAutomationWrite 检查规则/preview/test/replay 写权限。
func (s *Service) requireAutomationWrite(scope AutomationScope) error {
	switch scope.Type {
	case AutomationScopeWorkspace:
		if err := s.Require(PermissionWorkspaceModify); err != nil {
			return err
		}
		return s.Require(PermissionHookWrite)
	default:
		return s.requireProjectAutomationWrite()
	}
}

// appendAutomationRuleAudit 把 scope-aware audit 写入；payload 不含 prompt/body/secret。
func (s *Service) appendAutomationRuleAudit(action string, scope AutomationScope, row storage.AutomationRule, changedFields []string) error {
	entry := AuditEntry{
		Action:     action,
		TargetType: "automation_rule",
		TargetID:   row.ID,
		WorkspaceID: &row.WorkspaceID,
		Payload:    automationRuleAuditPayload(scope, row, changedFields...),
	}
	if scope.Type == AutomationScopeProject {
		projectID := row.ScopeID
		entry.ProjectID = &projectID
	}
	return s.appendAuditEntry(entry)
}
