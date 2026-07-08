package app

import (
	"testing"
)

func TestNotificationTemplateVarSpecsCoversAllKnownVars(t *testing.T) {
	// schema 里每个非 prefix 变量，取值函数必须能命中（用 zero input 不报 unsupported）。
	known := []string{
		"workspace.id", "workspace.slug",
		"project.id", "project.slug",
		"rule.id", "rule.name",
		"recipient.id",
		"delivery.id", "delivery.attempt", "delivery.workspace_id", "delivery.sink_id",
		"object.kind", "object.id",
		"task.uuid", "task.task_slug", "task.title", "task.description", "task.status", "task.due",
		"reminder.sequence", "reminder.overdue_sequence", "reminder.window_start", "reminder.window_end",
		"event.id", "event.type", "event.version", "event.occurred_at", "event.object_kind", "event.object_id", "event.json",
		"actor.id", "actor.name",
	}
	for _, name := range known {
		if lookupTemplateVarSpec(name) == nil {
			t.Errorf("variable %q not declared in schema", name)
		}
	}
}

func TestEndpointFieldExcludesTaskAndSecret(t *testing.T) {
	// task.* 和 secret.* 不允许出现在 endpoint（URL 模板）。
	taskVars := []string{"task.uuid", "task.title", "task.due"}
	for _, name := range taskVars {
		if templateVarFieldAllowed(name, templateFieldEndpoint) {
			t.Errorf("%q must NOT be allowed in endpoint field", name)
		}
	}
	if templateVarFieldAllowed("secret.token", templateFieldEndpoint) {
		t.Error("secret.* must NOT be allowed in endpoint field")
	}
	// 通用变量允许 endpoint。
	if !templateVarFieldAllowed("workspace.id", templateFieldEndpoint) {
		t.Error("workspace.id should be allowed in endpoint field")
	}
}

// TestEndpointAllowedVarsMatchLegacyWhitelist 锁住 endpoint 字段集与现有
// allowedEndpointVariable 白名单逐项一致，保证 Task 2 切换派生后行为等价。
func TestEndpointAllowedVarsMatchLegacyWhitelist(t *testing.T) {
	allowed := []string{
		"workspace.id", "workspace.slug", "project.id", "project.slug",
		"rule.id", "rule.name", "recipient.id",
		"event.id", "event.type", "event.object_kind", "event.object_id", "actor.id",
		"delivery.id", "delivery.attempt", "delivery.workspace_id", "delivery.sink_id",
		"object.kind", "object.id",
	}
	for _, name := range allowed {
		if !templateVarFieldAllowed(name, templateFieldEndpoint) {
			t.Errorf("%q must be allowed in endpoint (legacy whitelist allowed it)", name)
		}
	}
	// 这些变量在旧 endpoint 白名单里不被允许，schema 必须保持一致。
	disallowed := []string{
		"event.version", "event.occurred_at", "event.json",
		"actor.name",
		"task.uuid", "task.title",
	}
	for _, name := range disallowed {
		if templateVarFieldAllowed(name, templateFieldEndpoint) {
			t.Errorf("%q must NOT be allowed in endpoint (legacy whitelist did not allow it)", name)
		}
	}
	// prefix 变量在 endpoint 的可用性：recipient.external_ids.* 允许，secret.* 不允许。
	if !templateVarFieldAllowed("recipient.external_ids.feishu", templateFieldEndpoint) {
		t.Error("recipient.external_ids.* must be allowed in endpoint")
	}
}

func TestTriggerGrouping(t *testing.T) {
	// reminder 组不含 event.* / actor.*。
	reminderVars := templateVarNamesForTrigger(templateTriggerReminder)
	for _, name := range reminderVars {
		if isPrefixVarName(name) {
			continue
		}
		if startsWith(name, "event.") || startsWith(name, "actor.") {
			t.Errorf("reminder trigger must not include %q", name)
		}
	}
	// event 组不含 task.* / reminder.*。
	eventVars := templateVarNamesForTrigger(templateTriggerEvent)
	for _, name := range eventVars {
		if isPrefixVarName(name) {
			continue
		}
		if startsWith(name, "task.") || startsWith(name, "reminder.") {
			t.Errorf("event trigger must not include %q", name)
		}
	}
	// 通用变量两组都有。
	foundCommon := false
	for _, name := range reminderVars {
		if name == "workspace.id" {
			foundCommon = true
		}
	}
	if !foundCommon {
		t.Error("workspace.id should appear in reminder trigger")
	}
}

func TestPrefixVarsDeclared(t *testing.T) {
	// recipient.external_ids.* 和 secret.* 是 prefix 变量。
	for _, prefix := range []string{"recipient.external_ids", "secret"} {
		if lookupPrefixTemplateVarSpec(prefix) == nil {
			t.Errorf("prefix variable %q not declared", prefix)
		}
	}
}

// 辅助：仅用于测试的可读性。
func startsWith(s, prefix string) bool { return len(s) >= len(prefix) && s[:len(prefix)] == prefix }
