package app

import (
	"slices"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

func TestTenantTokenAuditStoresMachineActor(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	created, err := svc.CreateTenantAccessToken(CreateTenantAccessTokenInput{
		Name:   "runtime",
		Scopes: []string{"task:write", "audit:read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	authn, err := svc.AuthenticateBearerToken(created.RawToken)
	if err != nil {
		t.Fatal(err)
	}
	authorized, err := svc.AuthorizeTokenRequest(RequestAuthorizationInput{
		Token:              authn,
		RequiredCapability: "task:write",
		RequiredPermission: PermissionTaskWrite,
		WorkspaceRef:       svc.Runtime().WorkspaceSlug,
	})
	if err != nil {
		t.Fatalf("AuthorizeTokenRequest() error = %v", err)
	}
	tenantSvc, err := NewService(ServiceOptions{
		Store:        svc.store,
		Clock:        FixedClock{NowUnix: 100},
		Runtime:      &authorized.Runtime,
		RequestScope: &authorized.Scope,
	})
	if err != nil {
		t.Fatalf("NewService(tenant) error = %v", err)
	}
	if _, err := tenantSvc.Add(AddInput{Title: "from tenant"}); err != nil {
		t.Fatal(err)
	}

	rows, err := svc.ListAudit(AuditListInput{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("audit rows empty")
	}
	if rows[0].ActorType != "tenant_access_token" || rows[0].ActorToken == nil {
		t.Fatalf("audit row = %#v", rows[0])
	}
	if rows[0].Actor != nil {
		t.Fatalf("tenant audit actor user = %#v, want nil", rows[0].Actor)
	}
	if rows[0].ActorToken.ID != created.View.ID || rows[0].ActorToken.Prefix != created.View.Prefix {
		t.Fatalf("actor token = %#v, want id %q prefix %q", rows[0].ActorToken, created.View.ID, created.View.Prefix)
	}
}

func TestTenantTokenRuntimeUsesScopeAndListsBoundWorkspace(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	created, err := svc.CreateTenantAccessToken(CreateTenantAccessTokenInput{
		Name:   "runtime",
		Scopes: []string{"workspace:read", "task:write"},
	})
	if err != nil {
		t.Fatal(err)
	}
	tenantSvc := mustTenantServiceForTest(t, svc, created.RawToken, "workspace:read", PermissionWorkspaceRead, 100)
	if tenantSvc.Runtime().Role != RoleOwner {
		t.Fatalf("tenant runtime role = %q, want owner", tenantSvc.Runtime().Role)
	}
	workspaces, err := tenantSvc.ListWorkspaces(false)
	if err != nil {
		t.Fatalf("ListWorkspaces() error = %v", err)
	}
	if len(workspaces) != 1 || workspaces[0].ID != svc.Runtime().WorkspaceID || workspaces[0].Role != RoleOwner {
		t.Fatalf("tenant workspaces = %#v, want bound workspace with owner role", workspaces)
	}
	if err := tenantSvc.Require(PermissionTaskWrite); err != nil {
		t.Fatalf("Require(task.write) error = %v", err)
	}
	if err := tenantSvc.Require(PermissionTokenWrite); err == nil {
		t.Fatal("Require(token.write) error = nil, want denied")
	} else {
		assertRuntimeCode(t, err, "permission_denied")
	}
}

func TestTenantTokenCanRequestP2WriteScopes(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	for _, scope := range []string{"notification:write", "hook:write", "reminder:write"} {
		created, err := svc.CreateTenantAccessToken(CreateTenantAccessTokenInput{
			Name:   "runtime",
			Scopes: []string{"task:write", scope},
		})
		if err != nil {
			t.Fatalf("CreateTenantAccessToken(%s) error = %v", scope, err)
		}
		if !slices.Contains(created.View.Scopes, scope) {
			t.Fatalf("tenant token scopes = %#v, missing %s", created.View.Scopes, scope)
		}
	}
}

func TestTenantTokenTaskEventsCreateSystemActorDeliveries(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	sink, err := svc.AddNotificationSink(NotificationSinkAddInput{
		Name:         "owner-sink",
		Type:         "webhook",
		EndpointMode: "static_url",
		URL:          "https://example.com/xuanchu",
	})
	if err != nil {
		t.Fatalf("AddNotificationSink() error = %v", err)
	}
	hook, err := svc.AddHook(HookAddInput{Name: "owner-hook", EventTypes: []string{"task.created"}, SinkRef: sink.ID})
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}
	created, err := svc.CreateTenantAccessToken(CreateTenantAccessTokenInput{
		Name:   "runtime",
		Scopes: []string{"task:write", "hook:read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	tenantSvc := mustTenantServiceForTest(t, svc, created.RawToken, "task:write", PermissionTaskWrite, 100)
	if _, err := tenantSvc.Add(AddInput{Title: "tenant task"}); err != nil {
		t.Fatalf("Add(task) error = %v", err)
	}
	deliveries, err := tenantSvc.ListHookDeliveries(hook.ID, "", 10, 0)
	if err != nil {
		t.Fatalf("ListHookDeliveries() error = %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("deliveries = %#v, want one tenant actor delivery", deliveries)
	}
	if deliveries[0].Actor.Type != "tenant_access_token" || deliveries[0].Actor.Token == nil || deliveries[0].Actor.Token.ID != created.View.ID {
		t.Fatalf("delivery actor = %#v, want tenant token %s", deliveries[0].Actor, created.View.ID)
	}
}

func TestTenantTokenProjectTransitionCreatesSystemActorAnnotation(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	project, err := svc.AddProject(AddProjectInput{Slug: "tenstat", Name: "Tenant Status"})
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}
	created, err := svc.CreateTenantAccessToken(CreateTenantAccessTokenInput{
		Name:   "runtime",
		Scopes: []string{"project:read", "project:write"},
	})
	if err != nil {
		t.Fatal(err)
	}
	tenantSvc := mustTenantServiceForTest(t, svc, created.RawToken, "project:write", PermissionProjectManage, 100)
	if _, err := tenantSvc.TransitionProject(project.ID, string(storage.ProjectStatusActive)); err != nil {
		t.Fatalf("TransitionProject() error = %v", err)
	}
	annotations, err := svc.ProjectAnnotations(project.ID)
	if err != nil {
		t.Fatalf("ProjectAnnotations() error = %v", err)
	}
	if len(annotations) != 1 {
		t.Fatalf("annotations = %#v, want one tenant actor annotation", annotations)
	}
	if annotations[0].CreatedBy.Type != "tenant_access_token" || annotations[0].CreatedBy.Token == nil || annotations[0].CreatedBy.Token.ID != created.View.ID {
		t.Fatalf("annotation actor = %#v, want tenant token %s", annotations[0].CreatedBy, created.View.ID)
	}
	timeline, err := svc.ProjectTimeline(project.ID, TimelineOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ProjectTimeline() error = %v", err)
	}
	if len(timeline) != 1 {
		t.Fatalf("timeline = %#v, want one tenant actor entry", timeline)
	}
	if timeline[0].CreatedBy.Type != "tenant_access_token" || timeline[0].CreatedBy.Token == nil || timeline[0].CreatedBy.Token.ID != created.View.ID {
		t.Fatalf("timeline actor = %#v, want tenant token %s", timeline[0].CreatedBy, created.View.ID)
	}
}

func TestTenantTokenCreatesNotificationReminderAndEventRulesAsSystemActor(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	created, err := svc.CreateTenantAccessToken(CreateTenantAccessTokenInput{
		Name:   "runtime",
		Scopes: []string{"notification:write", "reminder:write", "task:write"},
	})
	if err != nil {
		t.Fatal(err)
	}
	tenantSvc := mustTenantServiceForTest(t, svc, created.RawToken, "notification:write", PermissionNotificationWrite, 100)
	sink, err := tenantSvc.AddNotificationSink(NotificationSinkAddInput{
		Name:         "tenant-sink",
		Type:         "webhook",
		EndpointMode: "static_url",
		URL:          "https://example.com/xuanchu",
	})
	if err != nil {
		t.Fatalf("AddNotificationSink() error = %v", err)
	}
	if sink.CreatedBy.Type != "tenant_access_token" || sink.CreatedBy.Token == nil || sink.CreatedBy.Token.ID != created.View.ID {
		t.Fatalf("sink actor = %#v, want tenant token %s", sink.CreatedBy, created.View.ID)
	}
	reminder, err := tenantSvc.AddReminderRule(ReminderRuleAddInput{
		Name:          "tenant-reminder",
		TriggerType:   "due_before",
		OffsetSeconds: 60,
		AudienceType:  "assignees",
		SinkRef:       sink.ID,
	})
	if err != nil {
		t.Fatalf("AddReminderRule() error = %v", err)
	}
	if reminder.CreatedBy.Type != "tenant_access_token" || reminder.CreatedBy.Token == nil || reminder.CreatedBy.Token.ID != created.View.ID {
		t.Fatalf("reminder actor = %#v, want tenant token %s", reminder.CreatedBy, created.View.ID)
	}
	rule, err := tenantSvc.AddEventNotificationRule(EventNotificationRuleAddInput{
		Name:         "tenant-event",
		EventType:    "task.created",
		AudienceType: "explicit_users",
		Recipients:   []string{"local"},
		SinkRef:      sink.ID,
	})
	if err != nil {
		t.Fatalf("AddEventNotificationRule() error = %v", err)
	}
	if rule.CreatedBy.Type != "tenant_access_token" || rule.CreatedBy.Token == nil || rule.CreatedBy.Token.ID != created.View.ID {
		t.Fatalf("notification rule actor = %#v, want tenant token %s", rule.CreatedBy, created.View.ID)
	}

	taskSvc := mustTenantServiceForTest(t, svc, created.RawToken, "task:write", PermissionTaskWrite, 100)
	if _, err := taskSvc.Add(AddInput{Title: "tenant event task"}); err != nil {
		t.Fatalf("Add(task) error = %v", err)
	}
	deliveries, err := svc.ListNotificationDeliveries(sink.ID, "", 10, 0)
	if err != nil {
		t.Fatalf("ListNotificationDeliveries() error = %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("notification deliveries = %#v, want one tenant actor delivery", deliveries)
	}
	if deliveries[0].Actor.Type != "tenant_access_token" || deliveries[0].Actor.Token == nil || deliveries[0].Actor.Token.ID != created.View.ID {
		t.Fatalf("notification delivery actor = %#v, want tenant token %s", deliveries[0].Actor, created.View.ID)
	}
}

func mustTenantServiceForTest(t *testing.T, svc *Service, rawToken, capability string, permission Permission, now int64) *Service {
	t.Helper()
	authn, err := svc.AuthenticateBearerToken(rawToken)
	if err != nil {
		t.Fatal(err)
	}
	authorized, err := svc.AuthorizeTokenRequest(RequestAuthorizationInput{
		Token:              authn,
		RequiredCapability: capability,
		RequiredPermission: permission,
		WorkspaceRef:       svc.Runtime().WorkspaceSlug,
	})
	if err != nil {
		t.Fatalf("AuthorizeTokenRequest() error = %v", err)
	}
	tenantSvc, err := NewService(ServiceOptions{
		Store:        svc.store,
		Clock:        FixedClock{NowUnix: now},
		Runtime:      &authorized.Runtime,
		RequestScope: &authorized.Scope,
	})
	if err != nil {
		t.Fatalf("NewService(tenant) error = %v", err)
	}
	return tenantSvc
}

// TestParseTaskFieldChangesHandlesBadPayload 校验坏 payload / 旧 payload
// 不导致解析失败。
func TestParseTaskFieldChangesHandlesBadPayload(t *testing.T) {
	cases := []string{
		"",
		"{}",
		"{not json",
		`{"changes": "not array"}`,
		`{"changes": [{"field": ""}]}`,
	}
	for _, payload := range cases {
		changes := parseTaskFieldChanges(payload)
		// 不报错，且没有有效 change 条目。
		for _, c := range changes {
			if c.Field != "" {
				t.Fatalf("parseTaskFieldChanges(%q) = %#v, want no valid changes", payload, changes)
			}
		}
	}
}

// TestParseTaskFieldChangesScalarAndSet 校验标量和集合 change 的解析，
// 以及 current 为 null 时仍保留 presence。
func TestParseTaskFieldChangesScalarAndSet(t *testing.T) {
	payload := `{
	  "changes": [
	    {"field":"title","previous":"旧","current":"新"},
	    {"field":"due","previous":1783036800,"current":null},
	    {"field":"assignees","added":[{"id":"u2","name":"lisi","display_name":"李四"}],"removed":[]},
	    {"field":"tags","added":["a"],"removed":["b"]}
	  ]
	}`
	changes := parseTaskFieldChanges(payload)
	if len(changes) != 4 {
		t.Fatalf("changes = %d, want 4", len(changes))
	}

	title := changes[0]
	if title.Kind != "scalar" || title.Field != "title" {
		t.Fatalf("title change = %#v", title)
	}
	if title.Previous == nil || title.Current == nil {
		t.Fatal("scalar previous/current must be non-nil for presence")
	}
	if title.Current.Raw != "新" {
		t.Fatalf("title current raw = %#v", title.Current.Raw)
	}
	if title.LabelKey != "projectWorkbench.taskHistory.field.title" {
		t.Fatalf("title label key = %q", title.LabelKey)
	}

	due := changes[1]
	if due.Current == nil {
		t.Fatal("due current missing; explicit null must preserve presence")
	}
	if due.Current.Raw != nil {
		t.Fatalf("due current raw = %#v, want nil", due.Current.Raw)
	}

	assignees := changes[2]
	if assignees.Kind != "set" || len(assignees.Added) != 1 {
		t.Fatalf("assignees change = %#v", assignees)
	}
	if assignees.Added[0].Text != "李四" {
		t.Fatalf("assignee text = %q, want 李四（display_name 优先）", assignees.Added[0].Text)
	}

	tags := changes[3]
	if len(tags.Removed) != 1 || tags.Removed[0].Text != "b" {
		t.Fatalf("tags removed = %#v", tags.Removed)
	}
}

// TestParseTaskFieldChangesUDA 校验 UDA change 解析为 uda kind。
func TestParseTaskFieldChangesUDA(t *testing.T) {
	payload := `{
	  "changes": [
	    {
	      "field":"udas",
	      "entries":[
	        {"name":"effort","previous":null,"current":"2h"},
	        {"name":"budget","previous":"100","current":null}
	      ]
	    }
	  ]
	}`
	changes := parseTaskFieldChanges(payload)
	if len(changes) != 1 {
		t.Fatalf("changes = %d, want 1", len(changes))
	}
	uda := changes[0]
	if uda.Kind != "uda" || uda.Field != "udas" {
		t.Fatalf("uda change = %#v", uda)
	}
	if len(uda.Entries) != 2 {
		t.Fatalf("entries = %#v", uda.Entries)
	}
	// 新增 effort：before 存在但 raw nil，after raw = "2h"。
	effort := uda.Entries[0]
	if effort.Name != "effort" {
		t.Fatalf("entry[0] name = %q", effort.Name)
	}
	if effort.Before == nil || effort.Before.Raw != nil {
		t.Fatalf("effort before = %#v, want non-nil with nil raw", effort.Before)
	}
	if effort.After == nil || effort.After.Raw != "2h" {
		t.Fatalf("effort after = %#v, want raw 2h", effort.After)
	}
	// 删除 budget：before raw = "100"，after 存在但 raw nil。
	budget := uda.Entries[1]
	if budget.Before == nil || budget.Before.Raw != "100" {
		t.Fatalf("budget before = %#v", budget.Before)
	}
	if budget.After == nil || budget.After.Raw != nil {
		t.Fatalf("budget after = %#v, want non-nil with nil raw", budget.After)
	}
}
