package app

import (
	"regexp"
	"strings"
)

// automationTemplateVarSpec 声明一个模板变量的元信息。
type automationTemplateVarSpec struct {
	Name        string
	Description string
	Triggers    []string // "schedule", "event"
	IsPrefix    bool     // true 表示这是前缀变量（如 project_config.*），展示为提示而非可插入项
}

// automationTemplateVarSpecs 是全部可用变量的声明式定义，单一真相源。
var automationTemplateVarSpecs = []automationTemplateVarSpec{
	{Name: "project.id", Description: "项目 UUID", Triggers: []string{"schedule", "event"}},
	{Name: "project.slug", Description: "项目 slug", Triggers: []string{"schedule", "event"}},
	{Name: "project.name", Description: "项目名称", Triggers: []string{"schedule", "event"}},
	{Name: "project.status", Description: "项目状态", Triggers: []string{"schedule", "event"}},
	{Name: "workspace.id", Description: "workspace UUID", Triggers: []string{"schedule", "event"}},
	{Name: "workspace.slug", Description: "workspace slug", Triggers: []string{"schedule", "event"}},
	{Name: "workspace.name", Description: "workspace 名称", Triggers: []string{"schedule", "event"}},
	{Name: "project_config", Description: "项目非 secret 配置 JSON 对象", Triggers: []string{"schedule", "event"}},
	{Name: "project_config.*", Description: "项目配置项（按 key 插入单个值，如 project_config:feishu.chat_id）", Triggers: []string{"schedule", "event"}, IsPrefix: true},
	{Name: "tasks", Description: "匹配任务列表 JSON 数组", Triggers: []string{"schedule"}},
	{Name: "task_summary", Description: "任务统计摘要 JSON", Triggers: []string{"schedule"}},
	{Name: "delivery_id", Description: "本次投递 ID", Triggers: []string{"schedule", "event"}},
	{Name: "trigger_type", Description: "触发类型 schedule/event/manual_test", Triggers: []string{"schedule", "event"}},
	{Name: "event.type", Description: "事件类型（如 task.assigned）", Triggers: []string{"event"}},
	{Name: "event.id", Description: "事件 ID", Triggers: []string{"event"}},
	{Name: "task", Description: "触发事件的任务 JSON", Triggers: []string{"event"}},
	{Name: "added_assignees", Description: "新增负责人 JSON 数组（task.assigned）", Triggers: []string{"event"}},
}

// AutomationTemplateVarView 是对外暴露的单个变量信息。
type AutomationTemplateVarView struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsPrefix    bool   `json:"is_prefix"`
}

// AutomationTemplateTriggerView 是按触发器分组的变量列表。
type AutomationTemplateTriggerView struct {
	Trigger string                      `json:"trigger"`
	Vars    []AutomationTemplateVarView `json:"vars"`
}

// AutomationTemplateVarsView 是模板变量 API 的顶层返回结构。
type AutomationTemplateVarsView struct {
	Triggers []AutomationTemplateTriggerView `json:"triggers"`
}

// AutomationTemplateVars 构建按触发器分组的变量列表。
func AutomationTemplateVars() AutomationTemplateVarsView {
	triggers := []string{"schedule", "event"}
	out := AutomationTemplateVarsView{}
	for _, trigger := range triggers {
		entry := AutomationTemplateTriggerView{Trigger: trigger}
		for _, spec := range automationTemplateVarSpecs {
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

// automationTemplateVarPattern 匹配 {{变量名}} 占位符。
var automationTemplateVarPattern = regexp.MustCompile(`\{\{([^}]+)\}\}`)

// renderAutomationTemplate 用 vars map 替换模板中的 {{变量}} 占位符。
// 未定义变量替换为空字符串，不报错。
func renderAutomationTemplate(tpl string, vars map[string]string) string {
	return automationTemplateVarPattern.ReplaceAllStringFunc(tpl, func(match string) string {
		name := strings.TrimSpace(match[2 : len(match)-2])
		if v, ok := vars[name]; ok {
			return v
		}
		return ""
	})
}
