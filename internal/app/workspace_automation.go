package app

// workspace_automation.go 把 AutomationScope=workspace 的 App 入口集中暴露，
// HTTP/MCP 层通过这些方法调用，不再直接接触通用 Automation service 的 scope 字段。

// WorkspaceAutomationScope 返回当前 Workspace 的 AutomationScope。
func (s *Service) WorkspaceAutomationScope() AutomationScope {
	return AutomationScope{Type: AutomationScopeWorkspace, ID: s.workspaceID}
}

// ProjectAutomationScope 返回指定 project 的 AutomationScope。
// 调用方需要先 ResolveProject 拿到 project.ID；这里只做组装。
func (s *Service) ProjectAutomationScope(projectID string) AutomationScope {
	return AutomationScope{Type: AutomationScopeProject, ID: projectID}
}

// AddWorkspaceAutomationRule 在当前 Workspace 创建规则（首版只接受 project.created 事件
// 或 workspace schedule）。底层走通用 addAutomationRuleLocked + scope=workspace。
func (s *Service) AddWorkspaceAutomationRule(input AutomationRuleInput) (AutomationRuleView, error) {
	return s.AddAutomationRule(s.WorkspaceAutomationScope(), input)
}

// ListWorkspaceAutomationRules 列出当前 Workspace 的规则。
func (s *Service) ListWorkspaceAutomationRules(includeDisabled bool) ([]AutomationRuleView, error) {
	return s.ListAutomationRules(AutomationRuleListInput{Scope: s.WorkspaceAutomationScope(), IncludeDisabled: includeDisabled})
}

// WorkspaceAutomationRuleInfo 读取单条 Workspace 规则。
func (s *Service) WorkspaceAutomationRuleInfo(ruleID string) (AutomationRuleView, error) {
	return s.AutomationRuleInfo(s.WorkspaceAutomationScope(), ruleID)
}

// ModifyWorkspaceAutomationRule 修改 Workspace 规则。
func (s *Service) ModifyWorkspaceAutomationRule(ruleID string, input AutomationRuleModifyInput) (AutomationRuleView, error) {
	return s.ModifyAutomationRule(s.WorkspaceAutomationScope(), ruleID, input)
}

// DeleteWorkspaceAutomationRule 删除 Workspace 规则。
func (s *Service) DeleteWorkspaceAutomationRule(ruleID string) error {
	return s.DeleteAutomationRule(s.WorkspaceAutomationScope(), ruleID)
}

// EnableWorkspaceAutomationRule 启用 Workspace 规则。
func (s *Service) EnableWorkspaceAutomationRule(ruleID string) (AutomationRuleView, error) {
	return s.EnableAutomationRule(s.WorkspaceAutomationScope(), ruleID)
}

// DisableWorkspaceAutomationRule 停用 Workspace 规则。
func (s *Service) DisableWorkspaceAutomationRule(ruleID string) (AutomationRuleView, error) {
	return s.DisableAutomationRule(s.WorkspaceAutomationScope(), ruleID)
}

// WorkspaceAutomationProviderConfig 返回 Workspace scope 的 Provider config facade。
func (s *Service) WorkspaceAutomationProviderConfig() AutomationProviderConfig {
	return NewAutomationProviderConfig(s, s.WorkspaceAutomationScope())
}

// ProjectAutomationProviderConfig 返回 Project scope 的 Provider config facade。
func (s *Service) ProjectAutomationProviderConfig(projectID string) AutomationProviderConfig {
	return NewAutomationProviderConfig(s, s.ProjectAutomationScope(projectID))
}

// WorkspaceAutomationTemplateVars 返回 Workspace 场景下的模板变量描述，
// 按 schedule / event(project.created) 分组。不读取 Project 值或 secret。
func WorkspaceAutomationTemplateVars() AutomationTemplateVarsView {
	triggers := []string{"schedule", "event"}
	out := AutomationTemplateVarsView{}
	for _, trigger := range triggers {
		entry := AutomationTemplateTriggerView{Trigger: trigger}
		for _, spec := range workspaceAutomationTemplateVarSpecs {
			for _, t := range spec.Triggers {
				if t == trigger {
					entry.Vars = append(entry.Vars, AutomationTemplateVarView{
						Name:        spec.Name,
						Description: spec.Description,
						IsPrefix:    spec.IsPrefix,
					})
					break
				}
			}
		}
		out.Triggers = append(out.Triggers, entry)
	}
	return out
}

// workspaceAutomationTemplateVarSpecs 是 Workspace 自动化的变量描述。
// 与 Project automationTemplateVarSpecs 区分：不包含 task_id/task.* 等 Project
// 任务级变量，因为 Workspace 规则默认不绑定 Project。
var workspaceAutomationTemplateVarSpecs = []automationTemplateVarSpec{
	{Name: "workspace.id", Description: "workspace UUID", Triggers: []string{"schedule", "event"}},
	{Name: "workspace.slug", Description: "workspace slug", Triggers: []string{"schedule", "event"}},
	{Name: "workspace.name", Description: "workspace 名称", Triggers: []string{"schedule", "event"}},
	{Name: "project.id", Description: "项目 UUID（event 上下文）", Triggers: []string{"event"}},
	{Name: "project.slug", Description: "项目 slug（event 上下文）", Triggers: []string{"event"}},
	{Name: "project.name", Description: "项目名称（event 上下文）", Triggers: []string{"event"}},
	{Name: "project.status", Description: "项目状态（event 上下文）", Triggers: []string{"event"}},
	{Name: "project_config", Description: "项目非 secret 配置 JSON 对象（event 上下文）", Triggers: []string{"event"}},
	{Name: "project_config.*", Description: "项目配置项（按 key 插入单个值，如 project_config:feishu.chat_id）", Triggers: []string{"event"}, IsPrefix: true},
	{Name: "delivery_id", Description: "本次投递 ID", Triggers: []string{"schedule", "event"}},
	{Name: "trigger_type", Description: "触发类型 schedule/event", Triggers: []string{"schedule", "event"}},
	{Name: "event.type", Description: "事件类型（如 project.created）", Triggers: []string{"event"}},
	{Name: "event.id", Description: "事件 ID", Triggers: []string{"event"}},
	{Name: "event.metadata", Description: "事件来源 metadata（source/initial_task_count 等）", Triggers: []string{"event"}, IsPrefix: true},
}
