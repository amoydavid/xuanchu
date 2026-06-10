package app

import (
	"encoding/json"
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/storage"
	"github.com/google/uuid"
)

func notificationTestEnv(t *testing.T) (*Service, func()) {
	t.Helper()
	svc, cleanup := newTestService(t, 1000)
	return svc, cleanup
}

func defaultNotificationSinkInput() NotificationSinkAddInput {
	return NotificationSinkAddInput{
		Name:           "openclaw-project",
		Type:           "webhook",
		EndpointMode:   "static_url",
		URL:            "https://example.com/xuanchu/notifications",
		AllowedHosts:   []string{"example.com"},
		Secret:         "webhook-secret",
		TimeoutSeconds: 10,
		MaxAttempts:    5,
	}
}

func TestAddNotificationSinkStaticURL(t *testing.T) {
	svc, cleanup := notificationTestEnv(t)
	defer cleanup()

	view, err := svc.AddNotificationSink(defaultNotificationSinkInput())
	if err != nil {
		t.Fatalf("AddNotificationSink() error = %v", err)
	}
	if view.Name != "openclaw-project" || view.Type != "webhook" {
		t.Fatalf("view = %#v", view)
	}
	if view.URL != "https://example.com/xuanchu/notifications" {
		t.Fatalf("URL = %q", view.URL)
	}

	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "webhook-secret") {
		t.Fatalf("view leaks secret: %s", encoded)
	}
}

func TestAddNotificationSinkMaxConcurrency(t *testing.T) {
	svc, cleanup := notificationTestEnv(t)
	defer cleanup()

	input := defaultNotificationSinkInput()
	input.MaxConcurrency = 3
	view, err := svc.AddNotificationSink(input)
	if err != nil {
		t.Fatalf("AddNotificationSink() error = %v", err)
	}
	if view.MaxConcurrency != 3 {
		t.Fatalf("MaxConcurrency = %d, want 3", view.MaxConcurrency)
	}

	info, err := svc.NotificationSinkInfo(view.ID)
	if err != nil {
		t.Fatalf("NotificationSinkInfo() error = %v", err)
	}
	if info.MaxConcurrency != 3 {
		t.Fatalf("info MaxConcurrency = %d, want 3", info.MaxConcurrency)
	}
}

func TestAddNotificationSinkRejectsNegativeMaxConcurrency(t *testing.T) {
	svc, cleanup := notificationTestEnv(t)
	defer cleanup()

	input := defaultNotificationSinkInput()
	input.MaxConcurrency = -1
	_, err := svc.AddNotificationSink(input)
	assertRuntimeCode(t, err, "notification_sink_invalid")
}

func TestAddNotificationSinkHTTPTemplateStoresHeaderAndBodyTemplates(t *testing.T) {
	svc, cleanup := notificationTestEnv(t)
	defer cleanup()

	if err := svc.ConfigSchemaSet(ConfigSchemaInput{
		Key:           "integrations.feishu.webhook_url",
		ValueType:     string(ConfigValueTypeString),
		AllowedScopes: []string{string(ConfigAllowedScopeProject)},
	}); err != nil {
		t.Fatalf("ConfigSchemaSet(webhook_url) error = %v", err)
	}
	if err := svc.ConfigSchemaSet(ConfigSchemaInput{
		Key:           "integrations.feishu.bot_token",
		ValueType:     string(ConfigValueTypeString),
		AllowedScopes: []string{string(ConfigAllowedScopeWorkspace), string(ConfigAllowedScopeProject)},
		Secret:        true,
	}); err != nil {
		t.Fatalf("ConfigSchemaSet(bot_token) error = %v", err)
	}

	view, err := svc.AddNotificationSink(NotificationSinkAddInput{
		Name:            "feishu-bot",
		Type:            "http_template",
		EndpointMode:    "config_value",
		ConfigKey:       "integrations.feishu.webhook_url",
		AllowedHosts:    []string{"open.feishu.cn"},
		HTTPMethod:      "POST",
		HeaderTemplates: []HTTPHeaderTemplateInput{{Name: "Content-Type", Value: "application/json"}, {Name: "Authorization", Value: "Bearer {{secret.feishu_bot_token}}"}},
		BodyContentType: "application/json",
		BodyTemplate:    `{"msg_type":"text","content":{"text":"任务 {{task.task_slug}} 即将到期：{{task.description}}"}}`,
		SecretRefs:      []HTTPTemplateSecretRefInput{{Alias: "feishu_bot_token", ConfigKey: "integrations.feishu.bot_token"}},
	})
	if err != nil {
		t.Fatalf("AddNotificationSink(http_template) error = %v", err)
	}
	if view.Type != "http_template" {
		t.Fatalf("Type = %q", view.Type)
	}
	if len(view.HeaderTemplates) != 2 {
		t.Fatalf("HeaderTemplates = %#v", view.HeaderTemplates)
	}
	if view.BodyTemplate == "" {
		t.Fatal("BodyTemplate is empty")
	}
	if len(view.SecretRefs) != 1 || view.SecretRefs[0].Alias != "feishu_bot_token" {
		t.Fatalf("SecretRefs = %#v", view.SecretRefs)
	}
}

func TestAddNotificationSinkHTTPTemplateRejectsSecretInURLTemplate(t *testing.T) {
	svc, cleanup := notificationTestEnv(t)
	defer cleanup()

	_, err := svc.AddNotificationSink(NotificationSinkAddInput{
		Name:         "bad",
		Type:         "http_template",
		EndpointMode: "template",
		URLTemplate:  "https://example.com/{{secret.token}}",
		AllowedHosts: []string{"example.com"},
		HTTPMethod:   "POST",
		BodyTemplate: `{"ok":true}`,
		SecretRefs:   []HTTPTemplateSecretRefInput{{Alias: "token", ConfigKey: "integrations.token"}},
	})
	assertRuntimeCode(t, err, "endpoint_template_invalid")
}

func TestAddNotificationSinkHTTPTemplateRejectsInvalidJSONBodyTemplate(t *testing.T) {
	svc, cleanup := notificationTestEnv(t)
	defer cleanup()

	_, err := svc.AddNotificationSink(NotificationSinkAddInput{
		Name:            "bad-json",
		Type:            "http_template",
		EndpointMode:    "static_url",
		URL:             "https://example.com/notify",
		AllowedHosts:    []string{"example.com"},
		HTTPMethod:      "POST",
		BodyContentType: "application/json",
		BodyTemplate:    `{"msg":`,
	})
	assertRuntimeCode(t, err, "notification_sink_invalid")
}

func TestAddReminderRuleDueBeforeRequiresOffset(t *testing.T) {
	svc, cleanup := notificationTestEnv(t)
	defer cleanup()
	sink, err := svc.AddNotificationSink(defaultNotificationSinkInput())
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.AddReminderRule(ReminderRuleAddInput{
		Name:         "due-before",
		TriggerType:  "due_before",
		AudienceType: "assignees",
		SinkRef:      sink.ID,
	})
	assertRuntimeCode(t, err, "reminder_rule_invalid")
}

func TestAddReminderRuleWithScheduleAndFilterReturnsViewFields(t *testing.T) {
	svc, cleanup := notificationTestEnv(t)
	defer cleanup()
	sink, err := svc.AddNotificationSink(defaultNotificationSinkInput())
	if err != nil {
		t.Fatal(err)
	}

	view, err := svc.AddReminderRule(ReminderRuleAddInput{
		Name:          "daily-due",
		ScheduleType:  "daily_at",
		ScheduleValue: "08:50",
		FilterSource:  "end.isnull and start.isnull and due.after:now and due.before:now+24h",
		AudienceType:  "assignees",
		SinkRef:       sink.ID,
	})
	if err != nil {
		t.Fatalf("AddReminderRule() error = %v", err)
	}
	if view.ScheduleType != "daily_at" || view.ScheduleValue != "08:50" || view.FilterSource == "" {
		t.Fatalf("schedule/filter view = %#v", view)
	}
}

func TestAddReminderRuleWithScheduleRejectsInvalidFilter(t *testing.T) {
	svc, cleanup := notificationTestEnv(t)
	defer cleanup()
	sink, err := svc.AddNotificationSink(defaultNotificationSinkInput())
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.AddReminderRule(ReminderRuleAddInput{
		Name:          "bad-filter",
		ScheduleType:  "daily_at",
		ScheduleValue: "08:50",
		FilterSource:  "(",
		AudienceType:  "assignees",
		SinkRef:       sink.ID,
	})
	assertRuntimeCode(t, err, "reminder_rule_invalid")
}

func TestAddReminderRuleWithScheduleRejectsProjectPredicateFilter(t *testing.T) {
	svc, cleanup := notificationTestEnv(t)
	defer cleanup()
	sink, err := svc.AddNotificationSink(defaultNotificationSinkInput())
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.AddReminderRule(ReminderRuleAddInput{
		Name:          "bad-project-filter",
		ScheduleType:  "daily_at",
		ScheduleValue: "08:50",
		FilterSource:  "project:alpha and due.before:now+24h",
		AudienceType:  "assignees",
		SinkRef:       sink.ID,
	})
	assertRuntimeCode(t, err, "reminder_rule_invalid")
}

func TestModifyReminderRuleCanUpgradeToScheduleAndFilter(t *testing.T) {
	svc, cleanup := notificationTestEnv(t)
	defer cleanup()
	sink, err := svc.AddNotificationSink(defaultNotificationSinkInput())
	if err != nil {
		t.Fatal(err)
	}
	rule, err := svc.AddReminderRule(ReminderRuleAddInput{
		Name:          "due-before",
		TriggerType:   "due_before",
		OffsetSeconds: 1200,
		AudienceType:  "assignees",
		SinkRef:       sink.ID,
	})
	if err != nil {
		t.Fatal(err)
	}

	scheduleType := "daily_at"
	scheduleValue := "08:50"
	filterSource := "end.isnull and start.isnull and due.after:now and due.before:now+24h"
	view, err := svc.ModifyReminderRule(rule.ID, ReminderRuleModifyInput{
		ScheduleType:  &scheduleType,
		ScheduleValue: &scheduleValue,
		FilterSource:  &filterSource,
	})
	if err != nil {
		t.Fatalf("ModifyReminderRule() error = %v", err)
	}
	if view.ScheduleType != scheduleType || view.ScheduleValue != scheduleValue || view.FilterSource != filterSource {
		t.Fatalf("modified schedule/filter view = %#v", view)
	}
}

func TestAddReminderRuleRejectsUnsupportedProjectOwnerAudience(t *testing.T) {
	svc, cleanup := notificationTestEnv(t)
	defer cleanup()
	sink, err := svc.AddNotificationSink(defaultNotificationSinkInput())
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.AddReminderRule(ReminderRuleAddInput{
		Name:         "owner",
		TriggerType:  "overdue",
		RepeatPolicy: "once",
		AudienceType: "project_owner",
		SinkRef:      sink.ID,
	})
	assertRuntimeCode(t, err, "audience_unsupported")
}

func TestNotificationScopeIsolation(t *testing.T) {
	svc, cleanup := notificationTestEnv(t)
	defer cleanup()

	memberSvc, _, memberCleanup := hookTestEnvWithRole(t, "member")
	defer memberCleanup()

	if _, err := svc.AddNotificationSink(defaultNotificationSinkInput()); err != nil {
		t.Fatal(err)
	}
	if _, err := memberSvc.ListNotificationSinks(false); err == nil {
		t.Fatal("member ListNotificationSinks error = nil, want permission denied")
	}
	if _, err := memberSvc.ListReminderRules("", false); err != nil {
		t.Fatalf("member ListReminderRules() error = %v", err)
	}
}

func TestNotificationSinkInfoDisableEnableDelete(t *testing.T) {
	svc, cleanup := notificationTestEnv(t)
	defer cleanup()

	sink, err := svc.AddNotificationSink(defaultNotificationSinkInput())
	if err != nil {
		t.Fatal(err)
	}
	info, err := svc.NotificationSinkInfo(sink.ID)
	if err != nil {
		t.Fatalf("NotificationSinkInfo() error = %v", err)
	}
	if info.Name != sink.Name {
		t.Fatalf("sink info = %#v", info)
	}
	disabled, err := svc.DisableNotificationSink(sink.ID)
	if err != nil {
		t.Fatalf("DisableNotificationSink() error = %v", err)
	}
	if disabled.Enabled {
		t.Fatal("disabled sink Enabled = true")
	}
	enabled, err := svc.EnableNotificationSink(sink.ID)
	if err != nil {
		t.Fatalf("EnableNotificationSink() error = %v", err)
	}
	if !enabled.Enabled {
		t.Fatal("enabled sink Enabled = false")
	}
	if err := svc.DeleteNotificationSink(sink.ID); err != nil {
		t.Fatalf("DeleteNotificationSink() error = %v", err)
	}
	_, err = svc.NotificationSinkInfo(sink.ID)
	assertRuntimeCode(t, err, "notification_sink_not_found")
}

func TestModifyNotificationSink(t *testing.T) {
	svc, cleanup := notificationTestEnv(t)
	defer cleanup()

	sink, err := svc.AddNotificationSink(defaultNotificationSinkInput())
	if err != nil {
		t.Fatal(err)
	}
	name := "openclaw-renamed"
	url := "https://example.com/renamed"
	timeout := 20
	modified, err := svc.ModifyNotificationSink(sink.ID, NotificationSinkModifyInput{
		Name:           &name,
		URL:            &url,
		TimeoutSeconds: &timeout,
	})
	if err != nil {
		t.Fatalf("ModifyNotificationSink() error = %v", err)
	}
	if modified.Name != name || modified.URL != url || modified.TimeoutSeconds != timeout {
		t.Fatalf("modified sink = %#v", modified)
	}
}

func TestModifyNotificationSinkMaxConcurrency(t *testing.T) {
	svc, cleanup := notificationTestEnv(t)
	defer cleanup()

	sink, err := svc.AddNotificationSink(defaultNotificationSinkInput())
	if err != nil {
		t.Fatal(err)
	}
	maxConcurrency := 7
	modified, err := svc.ModifyNotificationSink(sink.ID, NotificationSinkModifyInput{
		MaxConcurrency: &maxConcurrency,
	})
	if err != nil {
		t.Fatalf("ModifyNotificationSink() error = %v", err)
	}
	if modified.MaxConcurrency != maxConcurrency {
		t.Fatalf("MaxConcurrency = %d, want %d", modified.MaxConcurrency, maxConcurrency)
	}

	inheritDefault := 0
	modified, err = svc.ModifyNotificationSink(sink.ID, NotificationSinkModifyInput{
		MaxConcurrency: &inheritDefault,
	})
	if err != nil {
		t.Fatalf("ModifyNotificationSink(reset) error = %v", err)
	}
	if modified.MaxConcurrency != 0 {
		t.Fatalf("reset MaxConcurrency = %d, want 0", modified.MaxConcurrency)
	}
}

func TestModifyNotificationSinkRejectsNegativeMaxConcurrency(t *testing.T) {
	svc, cleanup := notificationTestEnv(t)
	defer cleanup()

	sink, err := svc.AddNotificationSink(defaultNotificationSinkInput())
	if err != nil {
		t.Fatal(err)
	}
	maxConcurrency := -1
	_, err = svc.ModifyNotificationSink(sink.ID, NotificationSinkModifyInput{
		MaxConcurrency: &maxConcurrency,
	})
	assertRuntimeCode(t, err, "notification_sink_invalid")
}

func TestNotificationDeliveryListInfoReplay(t *testing.T) {
	svc, cleanup := notificationTestEnv(t)
	defer cleanup()

	sink, err := svc.AddNotificationSink(defaultNotificationSinkInput())
	if err != nil {
		t.Fatal(err)
	}
	delivery := makeAppNotificationDelivery(svc.Runtime().WorkspaceID, sink.ID, storage.DeliveryStatusDeadLettered)
	if err := storage.NewNotificationDeliveryRepository(svc.store.DB()).Enqueue([]storage.NotificationDelivery{delivery}); err != nil {
		t.Fatal(err)
	}
	rows, err := svc.ListNotificationDeliveries("", storage.DeliveryStatusDeadLettered, 20, 0)
	if err != nil {
		t.Fatalf("ListNotificationDeliveries() error = %v", err)
	}
	if len(rows) != 1 || rows[0].ID != delivery.ID {
		t.Fatalf("deliveries = %#v", rows)
	}
	info, err := svc.NotificationDeliveryInfo(delivery.ID)
	if err != nil {
		t.Fatalf("NotificationDeliveryInfo() error = %v", err)
	}
	if info.Payload["event_type"] != "task.overdue" {
		t.Fatalf("payload = %#v", info.Payload)
	}
	if strings.Contains(info.RenderedHeaders["Authorization"][0], "secret-token") {
		t.Fatalf("headers leak secret: %#v", info.RenderedHeaders)
	}
	replayed, err := svc.ReplayNotificationDelivery(delivery.ID)
	if err != nil {
		t.Fatalf("ReplayNotificationDelivery() error = %v", err)
	}
	if replayed.Status != storage.DeliveryStatusQueued {
		t.Fatalf("replayed status = %q", replayed.Status)
	}
}

func TestNotificationDeliveryRespectsProjectScope(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 1000, "local", "local")
	projectA, err := ownerSvc.AddProject(AddProjectInput{Slug: "alpha", Name: "Alpha"})
	if err != nil {
		t.Fatal(err)
	}
	projectB, err := ownerSvc.AddProject(AddProjectInput{Slug: "beta", Name: "Beta"})
	if err != nil {
		t.Fatal(err)
	}
	sink, err := ownerSvc.AddNotificationSink(defaultNotificationSinkInput())
	if err != nil {
		t.Fatal(err)
	}
	deliveryA := makeAppNotificationDelivery(ownerSvc.Runtime().WorkspaceID, sink.ID, storage.DeliveryStatusDeadLettered)
	deliveryA.ProjectID = &projectA.ID
	deliveryB := makeAppNotificationDelivery(ownerSvc.Runtime().WorkspaceID, sink.ID, storage.DeliveryStatusDeadLettered)
	deliveryB.ProjectID = &projectB.ID
	if err := storage.NewNotificationDeliveryRepository(store.DB()).Enqueue([]storage.NotificationDelivery{deliveryA, deliveryB}); err != nil {
		t.Fatal(err)
	}
	created, err := ownerSvc.CreateToken(CreateTokenInput{
		Name:          "alpha-notification-token",
		Scopes:        []string{"notification:read", "notification:write"},
		WorkspaceRefs: []string{"local"},
		ProjectRefs:   []string{projectA.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	authn, err := ownerSvc.AuthenticateBearerToken(created.RawToken)
	if err != nil {
		t.Fatal(err)
	}
	authorized, err := ownerSvc.AuthorizeTokenRequest(RequestAuthorizationInput{
		Token:              authn,
		RequiredCapability: "notification:read",
		RequiredPermission: PermissionNotificationRead,
		WorkspaceRef:       "local",
	})
	if err != nil {
		t.Fatal(err)
	}
	scopedSvc, err := NewService(ServiceOptions{
		Store:        store,
		Clock:        FixedClock{NowUnix: 1000},
		Runtime:      &authorized.Runtime,
		RequestScope: &authorized.Scope,
	})
	if err != nil {
		t.Fatal(err)
	}

	rows, err := scopedSvc.ListNotificationDeliveries("", storage.DeliveryStatusDeadLettered, 20, 0)
	if err != nil {
		t.Fatalf("ListNotificationDeliveries() error = %v", err)
	}
	if len(rows) != 1 || rows[0].ID != deliveryA.ID {
		t.Fatalf("deliveries = %#v", rows)
	}
	_, err = scopedSvc.NotificationDeliveryInfo(deliveryB.ID)
	assertRuntimeCode(t, err, "notification_delivery_not_found")
	_, err = scopedSvc.ReplayNotificationDelivery(deliveryB.ID)
	assertRuntimeCode(t, err, "notification_delivery_not_found")
}

func TestReminderRuleInfoDisableEnableDelete(t *testing.T) {
	svc, cleanup := notificationTestEnv(t)
	defer cleanup()

	sink, err := svc.AddNotificationSink(defaultNotificationSinkInput())
	if err != nil {
		t.Fatal(err)
	}
	rule, err := svc.AddReminderRule(ReminderRuleAddInput{
		Name:          "due-before",
		TriggerType:   "due_before",
		OffsetSeconds: 3600,
		AudienceType:  "assignees",
		SinkRef:       sink.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	info, err := svc.ReminderRuleInfo(rule.ID)
	if err != nil {
		t.Fatalf("ReminderRuleInfo() error = %v", err)
	}
	if info.Name != rule.Name {
		t.Fatalf("rule info = %#v", info)
	}
	disabled, err := svc.DisableReminderRule(rule.ID)
	if err != nil {
		t.Fatalf("DisableReminderRule() error = %v", err)
	}
	if disabled.Enabled {
		t.Fatal("disabled rule Enabled = true")
	}
	enabled, err := svc.EnableReminderRule(rule.ID)
	if err != nil {
		t.Fatalf("EnableReminderRule() error = %v", err)
	}
	if !enabled.Enabled {
		t.Fatal("enabled rule Enabled = false")
	}
	if err := svc.DeleteReminderRule(rule.ID); err != nil {
		t.Fatalf("DeleteReminderRule() error = %v", err)
	}
	_, err = svc.ReminderRuleInfo(rule.ID)
	assertRuntimeCode(t, err, "reminder_rule_not_found")
}

func TestModifyReminderRule(t *testing.T) {
	svc, cleanup := notificationTestEnv(t)
	defer cleanup()

	sink, err := svc.AddNotificationSink(defaultNotificationSinkInput())
	if err != nil {
		t.Fatal(err)
	}
	rule, err := svc.AddReminderRule(ReminderRuleAddInput{
		Name:          "due-before",
		TriggerType:   "due_before",
		OffsetSeconds: 3600,
		AudienceType:  "assignees",
		SinkRef:       sink.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	name := "overdue-once"
	trigger := "overdue"
	offset := int64(0)
	modified, err := svc.ModifyReminderRule(rule.ID, ReminderRuleModifyInput{
		Name:          &name,
		TriggerType:   &trigger,
		OffsetSeconds: &offset,
	})
	if err != nil {
		t.Fatalf("ModifyReminderRule() error = %v", err)
	}
	if modified.Name != name || modified.TriggerType != "overdue" || modified.OffsetSeconds != 0 {
		t.Fatalf("modified rule = %#v", modified)
	}
}

func makeAppNotificationDelivery(workspaceID, sinkID, status string) storage.NotificationDelivery {
	return storage.NotificationDelivery{
		ID:                  uuid.NewString(),
		WorkspaceID:         workspaceID,
		RuleID:              "rule-1",
		SinkID:              sinkID,
		TaskUUID:            "task-1",
		RecipientUserID:     "user-1",
		EventID:             uuid.NewString(),
		EventType:           "task.overdue",
		DedupeKey:           uuid.NewString(),
		ResolvedURL:         "https://example.com/notify",
		RenderedMethod:      "POST",
		RenderedHeadersJSON: `{"Authorization":["Bearer secret-token"],"X-Plain":["ok"]}`,
		RenderedBody:        `{"token":"secret-token","ok":true}`,
		RenderedContentType: "application/json",
		PayloadJSON:         `{"event_type":"task.overdue"}`,
		Status:              status,
		CreatedAt:           1000,
		ModifiedAt:          1000,
	}
}
