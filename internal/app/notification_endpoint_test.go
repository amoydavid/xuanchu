package app

import (
	"encoding/json"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/task"
)

func TestNotificationEndpointTemplateWorkspaceAndProject(t *testing.T) {
	req, err := ResolveNotificationRequest(NotificationRequestResolveInput{
		Sink: NotificationSinkView{
			Type:         "webhook",
			EndpointMode: "template",
			URLTemplate:  "https://example.com/{{workspace.slug}}/{{project.slug}}/notifications",
			AllowedHosts: []string{"example.com"},
			HTTPMethod:   "POST",
		},
		Workspace: NotificationWorkspaceContext{ID: "ws-1", Slug: "dajee", Name: "Dajee"},
		Project:   &NotificationProjectContext{ID: "proj-1", Slug: "agentapi", Name: "Agent API"},
		Rule:      NotificationRuleContext{ID: "rule-1", Name: "due-before", TriggerType: "due_before"},
		Task:      NotificationTaskContext{UUID: "task-1", TaskSlug: "agentapi-1", Description: "完成 OAuth", Status: "pending"},
		Recipient: task.UserInfo{ID: "u-1", Name: "Alice"},
	})
	if err != nil {
		t.Fatalf("ResolveNotificationRequest() error = %v", err)
	}
	if req.ResolvedURL != "https://example.com/dajee/agentapi/notifications" {
		t.Fatalf("ResolvedURL = %q", req.ResolvedURL)
	}
	if req.RenderedBody == "" || req.PayloadJSON == "" {
		t.Fatalf("request body/payload not rendered: %#v", req)
	}
}

func TestNotificationEndpointRejectsSecretVariableInURL(t *testing.T) {
	_, err := ResolveNotificationRequest(NotificationRequestResolveInput{
		Sink: NotificationSinkView{
			Type:         "webhook",
			EndpointMode: "template",
			URLTemplate:  "https://example.com/{{secret.token}}",
			AllowedHosts: []string{"example.com"},
		},
		Workspace: NotificationWorkspaceContext{ID: "ws-1", Slug: "dajee"},
		Rule:      NotificationRuleContext{ID: "rule-1", Name: "due-before"},
		Task:      NotificationTaskContext{UUID: "task-1", TaskSlug: "agentapi-1"},
		Recipient: task.UserInfo{ID: "u-1", Name: "Alice"},
	})
	assertRuntimeCode(t, err, "endpoint_template_invalid")
}

func TestNotificationRequestTemplateHTTPTemplateRendersHeadersAndBody(t *testing.T) {
	req, err := ResolveNotificationRequest(NotificationRequestResolveInput{
		Sink: NotificationSinkView{
			Type:            "http_template",
			EndpointMode:    "static_url",
			URL:             "https://open.feishu.cn/open-apis/bot/v2/hook/test",
			AllowedHosts:    []string{"open.feishu.cn"},
			HTTPMethod:      "POST",
			HeaderTemplates: []HTTPHeaderTemplateInput{{Name: "Content-Type", Value: "application/json"}, {Name: "Authorization", Value: "Bearer {{secret.feishu_bot_token}}"}},
			BodyTemplate:    `{"msg_type":"text","content":{"text":"任务 {{task.task_slug}} 即将到期：{{task.description}}"}}`,
			BodyContentType: "application/json",
			SecretRefs:      []HTTPTemplateSecretRefInput{{Alias: "feishu_bot_token", ConfigKey: "integrations.feishu.bot_token"}},
		},
		Workspace: NotificationWorkspaceContext{ID: "ws-1", Slug: "dajee", Name: "Dajee"},
		Rule:      NotificationRuleContext{ID: "rule-1", Name: "due-before", TriggerType: "due_before"},
		Task:      NotificationTaskContext{UUID: "task-1", TaskSlug: "agentapi-1", Description: "完成 OAuth", Status: "pending"},
		Recipient: task.UserInfo{ID: "u-1", Name: "Alice"},
		SecretValues: map[string]string{
			"feishu_bot_token": "token-123",
		},
	})
	if err != nil {
		t.Fatalf("ResolveNotificationRequest() error = %v", err)
	}
	if req.RenderedBody != `{"msg_type":"text","content":{"text":"任务 agentapi-1 即将到期：完成 OAuth"}}` {
		t.Fatalf("RenderedBody = %q", req.RenderedBody)
	}
	var headers map[string][]string
	if err := json.Unmarshal([]byte(req.RenderedHeadersJSON), &headers); err != nil {
		t.Fatalf("headers json invalid: %v", err)
	}
	if got := headers["Authorization"]; len(got) != 1 || got[0] != "Bearer token-123" {
		t.Fatalf("Authorization header = %#v", got)
	}
	if req.PayloadJSON == "" || req.PayloadJSON == req.RenderedBody {
		t.Fatalf("PayloadJSON should keep Xuanchu envelope separately, got %#v", req)
	}
}

func TestNotificationRequestTemplateRendersReminderContext(t *testing.T) {
	req, err := ResolveNotificationRequest(NotificationRequestResolveInput{
		Sink: NotificationSinkView{
			Type:            "http_template",
			EndpointMode:    "static_url",
			URL:             "https://example.com/notify",
			AllowedHosts:    []string{"example.com"},
			HeaderTemplates: []HTTPHeaderTemplateInput{{Name: "X-Reminder-Sequence", Value: "{{reminder.sequence}}"}},
			BodyTemplate:    `{"sequence":{{reminder.sequence}},"overdue_sequence":{{reminder.overdue_sequence}},"window_start":{{reminder.window_start}},"window_end":{{reminder.window_end}}}`,
			BodyContentType: "application/json",
		},
		Workspace: NotificationWorkspaceContext{ID: "ws-1", Slug: "dajee", Name: "Dajee"},
		Rule:      NotificationRuleContext{ID: "rule-1", Name: "daily", TriggerType: ""},
		Task:      NotificationTaskContext{UUID: "task-1", TaskSlug: "agentapi-1", Description: "完成 OAuth", Status: "pending"},
		Recipient: task.UserInfo{ID: "u-1", Name: "Alice"},
		Reminder:  NotificationReminderContext{Sequence: 2, OverdueSequence: 0, WindowStart: 1780879800, WindowEnd: 1780966200},
	})
	if err != nil {
		t.Fatalf("ResolveNotificationRequest() error = %v", err)
	}
	if req.RenderedBody != `{"sequence":2,"overdue_sequence":0,"window_start":1780879800,"window_end":1780966200}` {
		t.Fatalf("RenderedBody = %q", req.RenderedBody)
	}
	var headers map[string][]string
	if err := json.Unmarshal([]byte(req.RenderedHeadersJSON), &headers); err != nil {
		t.Fatalf("headers json invalid: %v", err)
	}
	if got := headers["X-Reminder-Sequence"]; len(got) != 1 || got[0] != "2" {
		t.Fatalf("X-Reminder-Sequence = %#v", got)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(req.PayloadJSON), &payload); err != nil {
		t.Fatalf("PayloadJSON invalid: %v", err)
	}
	reminder := payload["reminder"].(map[string]any)
	if reminder["sequence"] != float64(2) || reminder["overdue_sequence"] != float64(0) || reminder["window_start"] != float64(1780879800) || reminder["window_end"] != float64(1780966200) {
		t.Fatalf("reminder payload = %#v", reminder)
	}
}

func TestNotificationRequestTemplateMissingSecretDeadLettersRecipient(t *testing.T) {
	_, err := ResolveNotificationRequest(NotificationRequestResolveInput{
		Sink: NotificationSinkView{
			Type:            "http_template",
			EndpointMode:    "static_url",
			URL:             "https://open.feishu.cn/open-apis/bot/v2/hook/test",
			AllowedHosts:    []string{"open.feishu.cn"},
			HTTPMethod:      "POST",
			HeaderTemplates: []HTTPHeaderTemplateInput{{Name: "Authorization", Value: "Bearer {{secret.feishu_bot_token}}"}},
			BodyTemplate:    `{"ok":true}`,
			BodyContentType: "application/json",
			SecretRefs:      []HTTPTemplateSecretRefInput{{Alias: "feishu_bot_token", ConfigKey: "integrations.feishu.bot_token"}},
		},
		Workspace: NotificationWorkspaceContext{ID: "ws-1", Slug: "dajee"},
		Rule:      NotificationRuleContext{ID: "rule-1", Name: "due-before"},
		Task:      NotificationTaskContext{UUID: "task-1", TaskSlug: "agentapi-1"},
		Recipient: task.UserInfo{ID: "u-1", Name: "Alice"},
	})
	assertRuntimeCode(t, err, "template_unresolved")
}
