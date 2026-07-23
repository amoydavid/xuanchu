package app

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"

	"git.dajee.net/dajee/xuanchu/internal/schedule"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

const (
	ProjectAutomationTriggerSchedule = "schedule"
	ProjectAutomationTriggerEvent    = "event"
	ProjectAutomationActionOpenAI    = "openai_compatible"
	ProjectAutomationTriggerManual   = "manual_test"
)

// ProjectAutomationTriggerConfig 描述规则的触发配置。
type ProjectAutomationTriggerConfig struct {
	ScheduleType  string `json:"schedule_type,omitempty"`
	ScheduleValue string `json:"schedule_value,omitempty"`
	Timezone      string `json:"timezone,omitempty"`
	EventType     string `json:"event_type,omitempty"`
}

// ProjectAutomationCondition 描述规则的任务筛选和事件过滤条件。
type ProjectAutomationCondition struct {
	TaskFilter         string `json:"task_filter,omitempty"`
	MaxTasks           int    `json:"max_tasks,omitempty"`
	OnlyAddedAssignees bool   `json:"only_added_assignees,omitempty"`
}

// ProjectAutomationActionConfig 描述 OpenAI 兼容投递动作的配置。
type ProjectAutomationActionConfig struct {
	Protocol              string  `json:"protocol"`
	BaseURLConfigKey      string  `json:"base_url_config_key"`
	APIKeyConfigKey       string  `json:"api_key_config_key"`
	ModelConfigKey        string  `json:"model_config_key"`
	AllowedHostsConfigKey string  `json:"allowed_hosts_config_key,omitempty"`
	ModelOverride         string  `json:"model_override,omitempty"`
	Temperature           float64 `json:"temperature"`
	MaxAttempts           int     `json:"max_attempts,omitempty"`
	AttachMetadata        bool    `json:"attach_metadata,omitempty"`
}

// ProjectAutomationContextConfig 描述投递上下文包含哪些信息。
type ProjectAutomationContextConfig struct {
	Include []string `json:"include"`
}

// ProjectAutomationRuleAddInput 创建规则的输入。
type ProjectAutomationRuleAddInput struct {
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

// ProjectAutomationRuleModifyInput 修改规则的输入，nil 字段表示不更新。
type ProjectAutomationRuleModifyInput struct {
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

// ProjectAutomationRuleView 是对外暴露的规则视图，用户身份使用 task.UserInfo。
type ProjectAutomationRuleView struct {
	ID                  string                         `json:"id"`
	WorkspaceID         string                         `json:"workspace_id"`
	ProjectID           string                         `json:"project_id"`
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
}

// AddProjectAutomationRule 创建项目自动化规则。
func (s *Service) AddProjectAutomationRule(projectRef string, input ProjectAutomationRuleAddInput) (ProjectAutomationRuleView, error) {
	if err := s.requireProjectAutomationWrite(); err != nil {
		return ProjectAutomationRuleView{}, err
	}
	project, err := s.ResolveProject(projectRef)
	if err != nil {
		return ProjectAutomationRuleView{}, err
	}
	return s.addProjectAutomationRuleLocked(project, input)
}

// addProjectAutomationRuleLocked 在调用方事务内创建规则，供普通 Add 和模板
// 实例化共用同一套 normalization、closed-project 与 actor 规则。
func (s *Service) addProjectAutomationRuleLocked(project storage.Project, input ProjectAutomationRuleAddInput) (ProjectAutomationRuleView, error) {
	if isProjectClosed(project) {
		return ProjectAutomationRuleView{}, RuntimeError{Code: "project_closed", Message: "project is closed"}
	}
	normalized, err := normalizeProjectAutomationAddInput(input)
	if err != nil {
		return ProjectAutomationRuleView{}, err
	}
	now := s.clock.Unix()
	row := storage.ProjectAutomationRule{
		ID:                  input.presetID,
		WorkspaceID:         s.workspaceID,
		ProjectID:           project.ID,
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
		return ProjectAutomationRuleView{}, err
	}
	created, err := s.projectAutomationRuleRepo.GetByID(row.ID)
	if err != nil {
		return ProjectAutomationRuleView{}, err
	}
	return s.projectAutomationRuleViewFromRow(created)
}

// ListProjectAutomationRules 列出项目规则，includeDisabled 控制是否包含停用规则。
func (s *Service) ListProjectAutomationRules(projectRef string, includeDisabled bool) ([]ProjectAutomationRuleView, error) {
	if err := s.requireProjectAutomationRead(); err != nil {
		return nil, err
	}
	project, err := s.ResolveProject(projectRef)
	if err != nil {
		return nil, err
	}
	rows, err := s.projectAutomationRuleRepo.List(s.workspaceID, &project.ID, includeDisabled)
	if err != nil {
		return nil, err
	}
	out := make([]ProjectAutomationRuleView, 0, len(rows))
	for _, row := range rows {
		view, err := s.projectAutomationRuleViewFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, view)
	}
	return out, nil
}

// ProjectAutomationRuleInfo 读取单条规则详情。
func (s *Service) ProjectAutomationRuleInfo(projectRef string, ruleID string) (ProjectAutomationRuleView, error) {
	if err := s.requireProjectAutomationRead(); err != nil {
		return ProjectAutomationRuleView{}, err
	}
	project, err := s.ResolveProject(projectRef)
	if err != nil {
		return ProjectAutomationRuleView{}, err
	}
	row, err := s.projectAutomationRuleRepo.GetByID(ruleID)
	if err != nil {
		return ProjectAutomationRuleView{}, err
	}
	if row.WorkspaceID != s.workspaceID || row.ProjectID != project.ID {
		return ProjectAutomationRuleView{}, RuntimeError{Code: "automation_rule_not_found", Message: "automation rule not found"}
	}
	return s.projectAutomationRuleViewFromRow(row)
}

// ModifyProjectAutomationRule 修改规则，closed project 拒绝。
func (s *Service) ModifyProjectAutomationRule(projectRef string, ruleID string, input ProjectAutomationRuleModifyInput) (ProjectAutomationRuleView, error) {
	if err := s.requireProjectAutomationWrite(); err != nil {
		return ProjectAutomationRuleView{}, err
	}
	project, err := s.ResolveProject(projectRef)
	if err != nil {
		return ProjectAutomationRuleView{}, err
	}
	if isProjectClosed(project) {
		return ProjectAutomationRuleView{}, RuntimeError{Code: "project_closed", Message: "project is closed"}
	}
	row, err := s.projectAutomationRuleRepo.GetByID(ruleID)
	if err != nil {
		return ProjectAutomationRuleView{}, err
	}
	if row.WorkspaceID != s.workspaceID || row.ProjectID != project.ID {
		return ProjectAutomationRuleView{}, RuntimeError{Code: "automation_rule_not_found", Message: "automation rule not found"}
	}
	next := projectAutomationRuleAddInputFromRow(row)
	if input.Name != nil {
		next.Name = *input.Name
	}
	if input.Description != nil {
		next.Description = *input.Description
	}
	if input.Enabled != nil {
		next.Enabled = *input.Enabled
	}
	if input.TriggerType != nil {
		next.TriggerType = *input.TriggerType
	}
	if input.TriggerConfig != nil {
		next.TriggerConfig = *input.TriggerConfig
	}
	if input.Condition != nil {
		next.Condition = *input.Condition
	}
	if input.Action != nil {
		next.Action = *input.Action
	}
	if input.Context != nil {
		next.Context = *input.Context
	}
	if input.InstructionTemplate != nil {
		next.InstructionTemplate = *input.InstructionTemplate
	}
	if input.SystemPrompt != nil {
		next.SystemPrompt = *input.SystemPrompt
	}
	normalized, err := normalizeProjectAutomationAddInput(next)
	if err != nil {
		return ProjectAutomationRuleView{}, err
	}
	row.Name = normalized.Name
	row.Description = normalized.Description
	row.Enabled = &normalized.Enabled
	row.TriggerType = normalized.TriggerType
	row.TriggerConfigJSON = mustJSON(normalized.TriggerConfig)
	row.ConditionJSON = mustJSON(normalized.Condition)
	row.ActionConfigJSON = mustJSON(normalized.Action)
	row.ContextConfigJSON = mustJSON(normalized.Context)
	row.InstructionTemplate = normalized.InstructionTemplate
	row.SystemPrompt = normalized.SystemPrompt
	row.ModifiedAt = s.clock.Unix()
	if err := s.projectAutomationRuleRepo.Update(row); err != nil {
		return ProjectAutomationRuleView{}, err
	}
	return s.projectAutomationRuleViewFromRow(row)
}

// EnableProjectAutomationRule 启用规则。
func (s *Service) EnableProjectAutomationRule(projectRef string, ruleID string) (ProjectAutomationRuleView, error) {
	enabled := true
	return s.ModifyProjectAutomationRule(projectRef, ruleID, ProjectAutomationRuleModifyInput{Enabled: &enabled})
}

// DisableProjectAutomationRule 停用规则。
func (s *Service) DisableProjectAutomationRule(projectRef string, ruleID string) (ProjectAutomationRuleView, error) {
	enabled := false
	return s.ModifyProjectAutomationRule(projectRef, ruleID, ProjectAutomationRuleModifyInput{Enabled: &enabled})
}

// DeleteProjectAutomationRule 删除规则，closed project 拒绝。
func (s *Service) DeleteProjectAutomationRule(projectRef string, ruleID string) error {
	if err := s.requireProjectAutomationWrite(); err != nil {
		return err
	}
	project, err := s.ResolveProject(projectRef)
	if err != nil {
		return err
	}
	if isProjectClosed(project) {
		return RuntimeError{Code: "project_closed", Message: "project is closed"}
	}
	row, err := s.projectAutomationRuleRepo.GetByID(ruleID)
	if err != nil {
		return err
	}
	if row.WorkspaceID != s.workspaceID || row.ProjectID != project.ID {
		return RuntimeError{Code: "automation_rule_not_found", Message: "automation rule not found"}
	}
	return s.projectAutomationRuleRepo.Delete(ruleID)
}

func (s *Service) requireProjectAutomationRead() error {
	if err := s.Require(PermissionProjectRead); err != nil {
		return err
	}
	return s.Require(PermissionHookRead)
}

func (s *Service) requireProjectAutomationWrite() error {
	if err := s.Require(PermissionProjectManage); err != nil {
		return err
	}
	return s.Require(PermissionHookWrite)
}

func normalizeProjectAutomationAddInput(input ProjectAutomationRuleAddInput) (ProjectAutomationRuleAddInput, error) {
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
		if !allowedHookEventTypes[input.TriggerConfig.EventType] {
			return input, RuntimeError{Code: "automation_rule_invalid", Message: "unsupported event"}
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

func projectAutomationRuleAddInputFromRow(row storage.ProjectAutomationRule) ProjectAutomationRuleAddInput {
	return ProjectAutomationRuleAddInput{
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

func decodeProjectAutomationTriggerConfig(raw string) ProjectAutomationTriggerConfig {
	var cfg ProjectAutomationTriggerConfig
	if raw == "" {
		return cfg
	}
	_ = json.Unmarshal([]byte(raw), &cfg)
	return cfg
}

func decodeProjectAutomationCondition(raw string) ProjectAutomationCondition {
	var cfg ProjectAutomationCondition
	if raw == "" {
		return cfg
	}
	_ = json.Unmarshal([]byte(raw), &cfg)
	return cfg
}

func decodeProjectAutomationActionConfig(raw string) ProjectAutomationActionConfig {
	var cfg ProjectAutomationActionConfig
	if raw == "" {
		return cfg
	}
	_ = json.Unmarshal([]byte(raw), &cfg)
	return cfg
}

func decodeProjectAutomationContextConfig(raw string) ProjectAutomationContextConfig {
	var cfg ProjectAutomationContextConfig
	if raw == "" {
		return cfg
	}
	_ = json.Unmarshal([]byte(raw), &cfg)
	return cfg
}

func (s *Service) projectAutomationRuleViewFromRow(row storage.ProjectAutomationRule) (ProjectAutomationRuleView, error) {
	users, err := s.resolveUserInfos([]string{valueOrEmpty(row.CreatedByUserID)})
	if err != nil {
		return ProjectAutomationRuleView{}, err
	}
	createdBy := users[valueOrEmpty(row.CreatedByUserID)]
	if createdBy.ID == "" {
		createdBy = task.UserInfo{ID: valueOrEmpty(row.CreatedByUserID), Name: valueOrEmpty(row.CreatedByUserID)}
	}
	input := projectAutomationRuleAddInputFromRow(row)
	return ProjectAutomationRuleView{
		ID:                  row.ID,
		WorkspaceID:         row.WorkspaceID,
		ProjectID:           row.ProjectID,
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
	}, nil
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
