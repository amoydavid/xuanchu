package app

import "slices"

// 模板变量出现的字段位置。
type templateField string

const (
	templateFieldEndpoint templateField = "endpoint" // URL 模板
	templateFieldBody     templateField = "body"     // body / header 模板
)

// 模板变量的触发来源。
type templateTrigger string

const (
	templateTriggerReminder templateTrigger = "reminder" // reminder rule 触发
	templateTriggerEvent    templateTrigger = "event"    // event notification rule 触发
)

// templateVarSpec 描述一个模板变量。是变量体系的唯一真相源：
// 取值函数和字段位置校验都从此表派生。
type templateVarSpec struct {
	Name        string            // 完整变量名；prefix 变量用 "<group>.*"，如 "recipient.external_ids.*"
	Description string            // 中文说明
	Fields      []templateField   // 可出现的字段位置
	Triggers    []templateTrigger // 有有效值的触发来源
	IsPrefix    bool              // 是否动态前缀变量
	PrefixGroup string            // prefix 族名（IsPrefix=true 时填），如 "recipient.external_ids"
}

// notificationTemplateVarSpecs 是所有模板变量的声明。
// 与 notificationTemplateValue 的取值逻辑必须保持一致，由测试守护。
var notificationTemplateVarSpecs = []templateVarSpec{
	// --- 通用：workspace ---
	{Name: "workspace.id", Description: "工作区 ID", Fields: bothFields(), Triggers: bothTriggers()},
	{Name: "workspace.slug", Description: "工作区 slug", Fields: bothFields(), Triggers: bothTriggers()},

	// --- 通用：project（无项目时取值报 template_unresolved） ---
	{Name: "project.id", Description: "项目 ID", Fields: bothFields(), Triggers: bothTriggers()},
	{Name: "project.slug", Description: "项目 slug", Fields: bothFields(), Triggers: bothTriggers()},

	// --- 通用：rule ---
	{Name: "rule.id", Description: "规则 ID", Fields: bothFields(), Triggers: bothTriggers()},
	{Name: "rule.name", Description: "规则名称", Fields: bothFields(), Triggers: bothTriggers()},

	// --- 通用：recipient ---
	{Name: "recipient.id", Description: "接收人用户 ID", Fields: bothFields(), Triggers: bothTriggers()},
	{Name: "recipient.external_ids.*", Description: "接收人外部 ID（按 provider）", Fields: bothFields(), Triggers: bothTriggers(), IsPrefix: true, PrefixGroup: "recipient.external_ids"},

	// --- 通用：delivery ---
	{Name: "delivery.id", Description: "投递 ID", Fields: bothFields(), Triggers: bothTriggers()},
	{Name: "delivery.attempt", Description: "投递尝试序号", Fields: bothFields(), Triggers: bothTriggers()},
	{Name: "delivery.workspace_id", Description: "投递工作区 ID", Fields: bothFields(), Triggers: bothTriggers()},
	{Name: "delivery.sink_id", Description: "投递 sink ID", Fields: bothFields(), Triggers: bothTriggers()},

	// --- 通用：object ---
	{Name: "object.kind", Description: "对象类型", Fields: bothFields(), Triggers: bothTriggers()},
	{Name: "object.id", Description: "对象 ID", Fields: bothFields(), Triggers: bothTriggers()},

	// --- 仅 reminder：task（仅 body，endpoint 禁止） ---
	{Name: "task.uuid", Description: "任务 UUID", Fields: []templateField{templateFieldBody}, Triggers: []templateTrigger{templateTriggerReminder}},
	{Name: "task.task_slug", Description: "任务 slug（如 proj-12）", Fields: []templateField{templateFieldBody}, Triggers: []templateTrigger{templateTriggerReminder}},
	{Name: "task.title", Description: "任务标题", Fields: []templateField{templateFieldBody}, Triggers: []templateTrigger{templateTriggerReminder}},
	{Name: "task.description", Description: "任务描述", Fields: []templateField{templateFieldBody}, Triggers: []templateTrigger{templateTriggerReminder}},
	{Name: "task.status", Description: "任务状态", Fields: []templateField{templateFieldBody}, Triggers: []templateTrigger{templateTriggerReminder}},
	{Name: "task.due", Description: "任务截止时间（unix，可空）", Fields: []templateField{templateFieldBody}, Triggers: []templateTrigger{templateTriggerReminder}},

	// --- 仅 reminder：reminder（仅 body） ---
	{Name: "reminder.sequence", Description: "提醒序号", Fields: []templateField{templateFieldBody}, Triggers: []templateTrigger{templateTriggerReminder}},
	{Name: "reminder.overdue_sequence", Description: "逾期提醒序号", Fields: []templateField{templateFieldBody}, Triggers: []templateTrigger{templateTriggerReminder}},
	{Name: "reminder.window_start", Description: "提醒窗口起始（unix）", Fields: []templateField{templateFieldBody}, Triggers: []templateTrigger{templateTriggerReminder}},
	{Name: "reminder.window_end", Description: "提醒窗口结束（unix）", Fields: []templateField{templateFieldBody}, Triggers: []templateTrigger{templateTriggerReminder}},

	// --- 仅 event：event（id/type/object_kind/object_id 可在 endpoint；version/occurred_at/json 仅 body，与现有 endpoint 白名单一致） ---
	{Name: "event.id", Description: "事件 ID", Fields: bothFields(), Triggers: []templateTrigger{templateTriggerEvent}},
	{Name: "event.type", Description: "事件类型", Fields: bothFields(), Triggers: []templateTrigger{templateTriggerEvent}},
	{Name: "event.version", Description: "事件版本", Fields: []templateField{templateFieldBody}, Triggers: []templateTrigger{templateTriggerEvent}},
	{Name: "event.occurred_at", Description: "事件发生时间（unix）", Fields: []templateField{templateFieldBody}, Triggers: []templateTrigger{templateTriggerEvent}},
	{Name: "event.object_kind", Description: "事件对象类型", Fields: bothFields(), Triggers: []templateTrigger{templateTriggerEvent}},
	{Name: "event.object_id", Description: "事件对象 ID", Fields: bothFields(), Triggers: []templateTrigger{templateTriggerEvent}},
	{Name: "event.json", Description: "原始事件 JSON", Fields: []templateField{templateFieldBody}, Triggers: []templateTrigger{templateTriggerEvent}},

	// --- 仅 event：actor（id 可在 endpoint；name 仅 body） ---
	{Name: "actor.id", Description: "操作者 ID", Fields: bothFields(), Triggers: []templateTrigger{templateTriggerEvent}},
	{Name: "actor.name", Description: "操作者名称", Fields: []templateField{templateFieldBody}, Triggers: []templateTrigger{templateTriggerEvent}},

	// --- 通用：secret（仅 body，需在 sink secret_refs 声明） ---
	{Name: "secret.*", Description: "在 sink secret_refs 声明的密钥", Fields: []templateField{templateFieldBody}, Triggers: bothTriggers(), IsPrefix: true, PrefixGroup: "secret"},
}

func bothFields() []templateField {
	return []templateField{templateFieldEndpoint, templateFieldBody}
}

func bothTriggers() []templateTrigger {
	return []templateTrigger{templateTriggerReminder, templateTriggerEvent}
}

// lookupTemplateVarSpec 按 name 精确查找非 prefix 变量。
func lookupTemplateVarSpec(name string) *templateVarSpec {
	for i := range notificationTemplateVarSpecs {
		s := &notificationTemplateVarSpecs[i]
		if s.IsPrefix {
			continue
		}
		if s.Name == name {
			return s
		}
	}
	return nil
}

// lookupPrefixTemplateVarSpec 按 prefix 族名查找 prefix 变量。
func lookupPrefixTemplateVarSpec(group string) *templateVarSpec {
	for i := range notificationTemplateVarSpecs {
		s := &notificationTemplateVarSpecs[i]
		if s.IsPrefix && s.PrefixGroup == group {
			return s
		}
	}
	return nil
}

// templateVarFieldAllowed 判断变量是否允许出现在指定字段位置。
// 支持精确变量和 prefix 变量（如 "secret.token" 命中 "secret.*"）。
func templateVarFieldAllowed(name string, field templateField) bool {
	if spec := lookupTemplateVarSpec(name); spec != nil {
		return slices.Contains(spec.Fields, field)
	}
	for i := range notificationTemplateVarSpecs {
		s := &notificationTemplateVarSpecs[i]
		if s.IsPrefix && hasVarPrefix(name, s.PrefixGroup) {
			return slices.Contains(s.Fields, field)
		}
	}
	return false
}

// templateVarNamesForTrigger 返回某 trigger 下所有变量的 name（prefix 变量返回 "<group>.*" 形式）。
func templateVarNamesForTrigger(trigger templateTrigger) []string {
	var out []string
	for _, s := range notificationTemplateVarSpecs {
		if slices.Contains(s.Triggers, trigger) {
			out = append(out, s.Name)
		}
	}
	return out
}

// hasVarPrefix 判断 name 是否以 "<group>." 开头。
func hasVarPrefix(name, group string) bool {
	return len(name) > len(group)+1 && name[:len(group)+1] == group+"."
}

// isPrefixVarName 判断 schema name 是否是 prefix 形式（含 ".*"）。
func isPrefixVarName(name string) bool {
	return len(name) > 2 && name[len(name)-2:] == ".*"
}

// NotificationTemplateVarsView 构造给 HTTP API 返回的视图，按 trigger 分组、再按 field 分组。
func NotificationTemplateVarsView() map[string]any {
	fieldOrder := []templateField{templateFieldEndpoint, templateFieldBody}
	triggerOrder := []templateTrigger{templateTriggerReminder, templateTriggerEvent}

	triggersOut := make([]map[string]any, 0, len(triggerOrder))
	for _, trigger := range triggerOrder {
		fieldsOut := make([]map[string]any, 0, len(fieldOrder))
		for _, field := range fieldOrder {
			vars := make([]map[string]any, 0)
			for _, spec := range notificationTemplateVarSpecs {
				if !slices.Contains(spec.Triggers, trigger) {
					continue
				}
				if !slices.Contains(spec.Fields, field) {
					continue
				}
				vars = append(vars, map[string]any{
					"name":         spec.Name,
					"description":  spec.Description,
					"dynamic":      spec.IsPrefix,
					"prefix_group": spec.PrefixGroup,
				})
			}
			fieldsOut = append(fieldsOut, map[string]any{
				"field": string(field),
				"vars":  vars,
			})
		}
		triggersOut = append(triggersOut, map[string]any{
			"trigger": string(trigger),
			"fields":  fieldsOut,
		})
	}
	return map[string]any{"triggers": triggersOut}
}
