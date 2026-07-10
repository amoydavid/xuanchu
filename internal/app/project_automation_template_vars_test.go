package app

import (
	"testing"
)

func TestRenderAutomationTemplate(t *testing.T) {
	vars := map[string]string{
		"project.slug":   "adsops",
		"project.name":   "广告投放优化",
		"project_config": `{"feishu.chat_id":"oc_xxx"}`,
	}
	got := renderAutomationTemplate("请检查项目 {{project.name}}（{{project.slug}}）\n配置：{{project_config}}", vars)
	want := "请检查项目 广告投放优化（adsops）\n配置：{\"feishu.chat_id\":\"oc_xxx\"}"
	if got != want {
		t.Fatalf("renderAutomationTemplate = %q, want %q", got, want)
	}
}

func TestRenderAutomationTemplateUndefinedVar(t *testing.T) {
	got := renderAutomationTemplate("hello {{undefined.var}} world", map[string]string{})
	if got != "hello  world" {
		t.Fatalf("undefined var should be empty string, got %q", got)
	}
}

func TestRenderAutomationTemplateNoVars(t *testing.T) {
	got := renderAutomationTemplate("plain text no vars", map[string]string{"project.slug": "x"})
	if got != "plain text no vars" {
		t.Fatalf("no-vars template should be unchanged, got %q", got)
	}
}

func TestRenderAutomationTemplateProjectConfigKey(t *testing.T) {
	vars := map[string]string{
		"project_config":                 `{"feishu.chat_id":"oc_xxx","agent.provider.model":"op"}`,
		"project_config:feishu.chat_id":  "oc_xxx",
		"project_config:agent.provider.model": "op",
	}
	got := renderAutomationTemplate("飞书群：{{project_config:feishu.chat_id}}，模型：{{project_config:agent.provider.model}}", vars)
	want := "飞书群：oc_xxx，模型：op"
	if got != want {
		t.Fatalf("renderAutomationTemplate project_config:key = %q, want %q", got, want)
	}
}

func TestAutomationTemplateVars(t *testing.T) {
	view := AutomationTemplateVars()
	if len(view.Triggers) != 2 {
		t.Fatalf("expected 2 triggers, got %d", len(view.Triggers))
	}
	// schedule 组必须包含 tasks 和 project.slug
	var scheduleVars, eventVars []string
	for _, trig := range view.Triggers {
		switch trig.Trigger {
		case "schedule":
			for _, v := range trig.Vars {
				scheduleVars = append(scheduleVars, v.Name)
			}
		case "event":
			for _, v := range trig.Vars {
				eventVars = append(eventVars, v.Name)
			}
		}
	}
	if !containsStr(scheduleVars, "tasks") || !containsStr(scheduleVars, "project.slug") {
		t.Fatalf("schedule vars missing key, got %v", scheduleVars)
	}
	if !containsStr(eventVars, "event.type") || !containsStr(eventVars, "added_assignees") {
		t.Fatalf("event vars missing key, got %v", eventVars)
	}
	// schedule 不应包含 event.type
	if containsStr(scheduleVars, "event.type") {
		t.Fatalf("schedule should not contain event.type")
	}
}

func containsStr(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}
