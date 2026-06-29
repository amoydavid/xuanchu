package app

import (
	"encoding/json"
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

func TestTenantTokenAppendAuditEntryStoresMachineActor(t *testing.T) {
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, mustUnix(t, "2030-01-01T10:00:00Z"), "local", "local")
	project, err := svc.AddProject(AddProjectInput{Slug: "legacy", Name: "Legacy"})
	if err != nil {
		t.Fatalf("AddProject(legacy) error = %v", err)
	}
	due := mustUnix(t, "2030-01-01T23:59:59Z")
	until := mustUnix(t, "2030-01-03T23:59:59Z")
	recur := "daily"
	if _, err := svc.Add(AddInput{
		Title:   "legacy recurring task",
		Project: &project.Slug,
		Due:     &due,
		Until:   &until,
		Recur:   &recur,
	}); err != nil {
		t.Fatalf("Add(recurring) error = %v", err)
	}
	if _, err := svc.ArchiveProject(project.ID); err != nil {
		t.Fatalf("ArchiveProject(legacy) error = %v", err)
	}
	children, err := svc.List(ListInput{})
	if err != nil || len(children) != 1 {
		t.Fatalf("List(children before done) = (%#v, %v), want one child", children, err)
	}
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
		Clock:        FixedClock{NowUnix: mustUnix(t, "2030-01-02T10:00:00Z")},
		Runtime:      &authorized.Runtime,
		RequestScope: &authorized.Scope,
	})
	if err != nil {
		t.Fatalf("NewService(tenant) error = %v", err)
	}
	if err := tenantSvc.Done(children[0].UUID); err != nil {
		t.Fatalf("Done(first child) error = %v", err)
	}

	rows, err := svc.ListAudit(AuditListInput{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Action != "task.recurrence.archived_project" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(row.PayloadJSON), &payload); err != nil {
			t.Fatalf("warning payload json = %q, err = %v", row.PayloadJSON, err)
		}
		if payload["project_id"] != project.ID || payload["project_slug"] != project.Slug {
			t.Fatalf("warning payload = %#v", payload)
		}
		if row.ActorType != "tenant_access_token" || row.Actor != nil || row.ActorToken == nil {
			t.Fatalf("warning actor = type %q user %#v token %#v", row.ActorType, row.Actor, row.ActorToken)
		}
		if row.ActorToken.ID != created.View.ID || row.ActorToken.Prefix != created.View.Prefix {
			t.Fatalf("warning actor token = %#v, want id %q prefix %q", row.ActorToken, created.View.ID, created.View.Prefix)
		}
		return
	}
	t.Fatal("missing task.recurrence.archived_project audit warning")
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
	if tenantSvc.Runtime().Role != "" {
		t.Fatalf("tenant runtime role = %q, want empty", tenantSvc.Runtime().Role)
	}
	workspaces, err := tenantSvc.ListWorkspaces(false)
	if err != nil {
		t.Fatalf("ListWorkspaces() error = %v", err)
	}
	if len(workspaces) != 1 || workspaces[0].ID != svc.Runtime().WorkspaceID || workspaces[0].Role != "" {
		t.Fatalf("tenant workspaces = %#v, want bound workspace with empty role", workspaces)
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

func TestTenantTokenCannotCreateUserShapedActorResources(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	created, err := svc.CreateTenantAccessToken(CreateTenantAccessTokenInput{
		Name:   "runtime",
		Scopes: []string{"task:write", "project:write", "notification:read", "notification:write", "hook:read", "hook:write", "reminder:write"},
	})
	if err != nil {
		t.Fatal(err)
	}
	tenantSvc := mustTenantServiceForTest(t, svc, created.RawToken, "notification:write", PermissionNotificationWrite, 100)
	if _, err := tenantSvc.AddNotificationSink(NotificationSinkAddInput{
		Name:         "tenant-sink",
		Type:         "webhook",
		EndpointMode: "static_url",
		URL:          "https://example.com/xuanchu",
	}); err == nil {
		t.Fatal("AddNotificationSink() error = nil, want tenant_actor_not_user")
	} else {
		assertRuntimeCode(t, err, "tenant_actor_not_user")
	}
	if _, err := tenantSvc.AddHook(HookAddInput{
		Name:       "tenant-hook",
		EventTypes: []string{"task.created"},
		SinkRef:    "missing",
	}); err == nil {
		t.Fatal("AddHook() error = nil, want tenant_actor_not_user")
	} else {
		assertRuntimeCode(t, err, "tenant_actor_not_user")
	}
	if _, err := tenantSvc.AddReminderRule(ReminderRuleAddInput{Name: "tenant-reminder", TriggerType: "overdue", AudienceType: "assignees", SinkRef: "missing"}); err == nil {
		t.Fatal("AddReminderRule() error = nil, want tenant_actor_not_user")
	} else {
		assertRuntimeCode(t, err, "tenant_actor_not_user")
	}
	if _, err := tenantSvc.AddEventNotificationRule(EventNotificationRuleAddInput{Name: "tenant-rule", EventType: "task.created", AudienceType: "assignees", SinkRef: "missing"}); err == nil {
		t.Fatal("AddEventNotificationRule() error = nil, want tenant_actor_not_user")
	} else {
		assertRuntimeCode(t, err, "tenant_actor_not_user")
	}
	taskRow, err := tenantSvc.Add(AddInput{Title: "tenant task"})
	if err != nil {
		t.Fatalf("Add(task) error = %v", err)
	}
	if _, err := tenantSvc.TaskAddLink(taskRow.UUID, "document", "https://example.com/doc", "Doc"); err == nil {
		t.Fatal("TaskAddLink() error = nil, want tenant_actor_not_user")
	} else {
		assertRuntimeCode(t, err, "tenant_actor_not_user")
	}
	project, err := tenantSvc.AddProject(AddProjectInput{Slug: "tenproj", Name: "Tenant Project"})
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}
	if _, err := tenantSvc.ProjectAnnotate(project.ID, "tenant note"); err == nil {
		t.Fatal("ProjectAnnotate() error = nil, want tenant_actor_not_user")
	} else {
		assertRuntimeCode(t, err, "tenant_actor_not_user")
	}
}

func TestTenantTokenTaskEventsDoNotCreateUserShapedDeliveries(t *testing.T) {
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
	if len(deliveries) != 0 {
		t.Fatalf("deliveries = %#v, want none for tenant actor", deliveries)
	}
}

func TestTenantTokenProjectTransitionDoesNotCreateUserShapedAnnotation(t *testing.T) {
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
	if len(annotations) != 0 {
		t.Fatalf("annotations = %#v, want none for tenant actor transition", annotations)
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
