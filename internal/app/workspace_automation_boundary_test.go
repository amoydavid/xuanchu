package app

import (
	"testing"
)

// TestProjectCreatedEventRejectedByHookWhitelist 验证 project.created 不在
// Hook 白名单中，避免 Workspace Automation 专用事件被错误订阅为 Hook。
// spec §5: "首版不把 project.created 同时开放给 Hook 和 Notification Rule"。
func TestProjectCreatedEventRejectedByHookWhitelist(t *testing.T) {
	if allowedHookEventTypes["project.created"] {
		t.Fatalf("project.created must NOT be in allowedHookEventTypes (Hook/Notification boundary)")
	}
	// 已存在的 Project 事件应保留在白名单中（回归保护）。
	for _, want := range []string{"project.archived", "project.transitioned", "project.annotated", "project.denotated"} {
		if !allowedHookEventTypes[want] {
			t.Fatalf("existing project event %q must remain in allowedHookEventTypes", want)
		}
	}
	// task.created 等核心 Hook 事件不受影响。
	if !allowedHookEventTypes["task.created"] {
		t.Fatalf("task.created must remain in allowedHookEventTypes")
	}
}

// TestProjectCreatedEventRejectedByHookCreate 验证创建订阅 project.created 的 Hook
// 会被服务端拒绝，返回稳定错误码 hook_event_types_invalid。
func TestProjectCreatedEventRejectedByHookCreate(t *testing.T) {
	f := newProjectAutomationServiceFixture(t)
	// 准备一个 sink 供 hook 引用。
	sink, err := f.svc.AddNotificationSink(NotificationSinkAddInput{
		Name:         "hook-sink",
		Type:         "webhook",
		EndpointMode: "static_url",
		URL:          "https://hook.example.com",
		AllowedHosts: []string{"hook.example.com"},
	})
	if err != nil {
		t.Fatalf("AddNotificationSink: %v", err)
	}
	_, err = f.svc.AddHook(HookAddInput{
		Name:       "project-created-hook",
		ScopeType:  HookScopeWorkspace,
		EventTypes: []string{"project.created"},
		SinkRef:    sink.ID,
	})
	if code := runtimeErrorCode(err); code != "hook_event_types_invalid" {
		t.Fatalf("err = %v, want hook_event_types_invalid", err)
	}
}

// TestProjectCreatedEventRejectedByNotificationRule 验证创建订阅 project.created
// 的 Notification Rule 会被服务端拒绝。
func TestProjectCreatedEventRejectedByNotificationRule(t *testing.T) {
	f := newProjectAutomationServiceFixture(t)
	sink, err := f.svc.AddNotificationSink(NotificationSinkAddInput{
		Name:         "rule-sink",
		Type:         "webhook",
		EndpointMode: "static_url",
		URL:          "https://rule.example.com",
		AllowedHosts: []string{"rule.example.com"},
	})
	if err != nil {
		t.Fatalf("AddNotificationSink: %v", err)
	}
	_, err = f.svc.AddEventNotificationRule(EventNotificationRuleAddInput{
		Name:      "project-created-rule",
		EventType: "project.created",
		SinkRef:   sink.ID,
	})
	if code := runtimeErrorCode(err); code != "notification_rule_invalid" {
		t.Fatalf("err = %v, want notification_rule_invalid", err)
	}
}

// TestProjectCreatedEventAllowedByWorkspaceAutomationWhitelist 验证 Workspace Automation
// 的事件白名单与 Hook/Notification 白名单互斥：project.created 只在 Workspace Automation
// 白名单中存在。
func TestProjectCreatedEventAllowedByWorkspaceAutomationWhitelist(t *testing.T) {
	if !workspaceAutomationAllowedEvents[AutomationEventTypeProjectCreated] {
		t.Fatalf("project.created must be in workspaceAutomationAllowedEvents")
	}
	// Workspace Automation 白名单与 Hook 白名单交集应为空。
	for event := range workspaceAutomationAllowedEvents {
		if allowedHookEventTypes[event] {
			t.Fatalf("event %q appears in both workspaceAutomationAllowedEvents and allowedHookEventTypes", event)
		}
	}
}
