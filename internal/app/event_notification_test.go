package app

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

func eventNotificationTestEnv(t *testing.T) (*Service, *storage.Store) {
	t.Helper()
	store := newTestStore(t)
	svc, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: 1000}})
	if err != nil {
		t.Fatal(err)
	}
	return svc, store
}

func createEventNotificationAssignee(t *testing.T, svc *Service, store *storage.Store, name string) storage.User {
	t.Helper()
	user := mustCreateUserRecord(t, store, storage.User{ID: uuid.NewString(), Name: name, CreatedAt: 100, ModifiedAt: 100})
	mustUpsertMembershipRecord(t, store, storage.Membership{UserID: user.ID, WorkspaceID: svc.Runtime().WorkspaceID, Role: string(RoleMember), JoinedAt: 100, ModifiedAt: 100})
	return user
}

func TestAddEventNotificationRuleCreatesRule(t *testing.T) {
	svc, _ := eventNotificationTestEnv(t)
	sink, err := svc.AddNotificationSink(defaultNotificationSinkInput())
	if err != nil {
		t.Fatalf("AddNotificationSink() error = %v", err)
	}

	rule, err := svc.AddEventNotificationRule(EventNotificationRuleAddInput{
		Name:         "task-unblocked",
		EventType:    "task.unblocked",
		AudienceType: "assignees",
		SinkRef:      sink.ID,
	})
	if err != nil {
		t.Fatalf("AddEventNotificationRule() error = %v", err)
	}
	if rule.EventType != "task.unblocked" || rule.AudienceType != "assignees" || rule.SinkID != sink.ID {
		t.Fatalf("rule = %#v", rule)
	}
}

func TestAddEventNotificationRuleRejectsUnsupportedProjectAudience(t *testing.T) {
	svc, _ := eventNotificationTestEnv(t)
	sink, _ := svc.AddNotificationSink(defaultNotificationSinkInput())

	_, err := svc.AddEventNotificationRule(EventNotificationRuleAddInput{
		Name:         "bad-project-rule",
		EventType:    "project.annotated",
		AudienceType: "assignees",
		SinkRef:      sink.ID,
	})
	assertRuntimeCode(t, err, "audience_unsupported_for_event")
}

func TestAddEventNotificationRuleRejectsCrossWorkspaceSink(t *testing.T) {
	store := newTestStore(t)
	svcA := newTestServiceWithRuntime(t, store, 1000, "local", "local")
	wsB := mustCreateWorkspaceRecord(t, store, storage.Workspace{
		ID: "ws-event-b", Slug: "event-b", Name: "Event B", Visibility: "private", CreatedAt: 100, ModifiedAt: 100,
	})
	mustUpsertMembershipRecord(t, store, storage.Membership{
		UserID: svcA.Runtime().ActorUserID, WorkspaceID: wsB.ID, Role: string(RoleOwner), JoinedAt: 100, ModifiedAt: 100,
	})
	svcB := newTestServiceWithRuntime(t, store, 1000, "local", "event-b")
	sinkB, err := svcB.AddNotificationSink(defaultNotificationSinkInput())
	if err != nil {
		t.Fatalf("AddNotificationSink(B) error = %v", err)
	}

	_, err = svcA.AddEventNotificationRule(EventNotificationRuleAddInput{
		Name:         "cross-workspace",
		EventType:    "task.unblocked",
		AudienceType: "assignees",
		SinkRef:      sinkB.ID,
	})
	assertRuntimeCode(t, err, "notification_sink_not_found")
}

func TestEventNotificationDeliveryDedupesSameEventRuleRecipient(t *testing.T) {
	svc, store := eventNotificationTestEnv(t)
	assignee := createEventNotificationAssignee(t, svc, store, "alice-event-dedupe")
	sink, _ := svc.AddNotificationSink(defaultNotificationSinkInput())
	rule, err := svc.AddEventNotificationRule(EventNotificationRuleAddInput{
		Name:         "unblocked",
		EventType:    "task.unblocked",
		AudienceType: "assignees",
		SinkRef:      sink.ID,
	})
	if err != nil {
		t.Fatalf("AddEventNotificationRule() error = %v", err)
	}
	blocker, _ := svc.Add(AddInput{Description: "blocker"})
	blocked, _ := svc.Add(AddInput{Description: "blocked", Assignees: []string{assignee.Name}})
	if err := svc.Modify(blocked.UUID, ModifyInput{AddDepends: []string{blocker.UUID}}); err != nil {
		t.Fatalf("Modify(depends) error = %v", err)
	}
	event := buildTaskUnblockedHookEvent(blocked, blocker, svc.Runtime(), 1000)

	if err := svc.enqueueEventNotificationDeliveries([]HookEvent{event, event}); err != nil {
		t.Fatalf("enqueueEventNotificationDeliveries() error = %v", err)
	}
	rows, err := storage.NewNotificationDeliveryRepository(store.DB()).List(svc.Runtime().WorkspaceID, "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("delivery count = %d, want 1", len(rows))
	}
	if rows[0].RuleID != rule.ID || rows[0].RecipientUserID != assignee.ID {
		t.Fatalf("delivery = %#v", rows[0])
	}
}

func TestDoneUnblockedEnqueuesEventNotificationForAssignee(t *testing.T) {
	svc, store := eventNotificationTestEnv(t)
	assignee := createEventNotificationAssignee(t, svc, store, "alice-event-unblocked")
	sink, _ := svc.AddNotificationSink(defaultNotificationSinkInput())
	if _, err := svc.AddEventNotificationRule(EventNotificationRuleAddInput{
		Name:         "unblocked",
		EventType:    "task.unblocked",
		AudienceType: "assignees",
		SinkRef:      sink.ID,
	}); err != nil {
		t.Fatalf("AddEventNotificationRule() error = %v", err)
	}
	blocker, _ := svc.Add(AddInput{Description: "prepare api"})
	blocked, _ := svc.Add(AddInput{Description: "integrate client", Assignees: []string{assignee.Name}})
	if err := svc.Modify(blocked.UUID, ModifyInput{AddDepends: []string{blocker.UUID}}); err != nil {
		t.Fatalf("Modify(depends) error = %v", err)
	}
	if err := svc.Done(blocker.UUID); err != nil {
		t.Fatalf("Done(blocker) error = %v", err)
	}

	rows, err := storage.NewNotificationDeliveryRepository(store.DB()).List(svc.Runtime().WorkspaceID, "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("delivery count = %d, want 1", len(rows))
	}
	if rows[0].EventType != "task.unblocked" || rows[0].ObjectKind != "task" || rows[0].ObjectID != blocked.UUID || rows[0].RecipientUserID != assignee.ID {
		t.Fatalf("delivery = %#v", rows[0])
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(rows[0].PayloadJSON), &payload); err != nil {
		t.Fatalf("payload json error = %v", err)
	}
	if payload["event_type"] != "task.unblocked" {
		t.Fatalf("payload event_type = %v", payload["event_type"])
	}
	if payload["delivery_id"] != rows[0].ID || payload["workspace_id"] != rows[0].WorkspaceID || payload["sink_id"] != rows[0].SinkID {
		t.Fatalf("top-level delivery payload = %#v, delivery = %#v", payload, rows[0])
	}
	if payload["attempt"] != float64(1) {
		t.Fatalf("payload attempt = %v, want 1", payload["attempt"])
	}
	delivery := payload["delivery"].(map[string]any)
	if delivery["id"] != rows[0].ID || delivery["workspace_id"] != rows[0].WorkspaceID || delivery["sink_id"] != rows[0].SinkID {
		t.Fatalf("delivery payload = %#v, delivery = %#v", delivery, rows[0])
	}
	object := payload["object"].(map[string]any)
	if object["kind"] != "task" || object["id"] != rows[0].ObjectID {
		t.Fatalf("object payload = %#v", object)
	}
	recipient := payload["recipient"].(map[string]any)
	if recipient["id"] != assignee.ID {
		t.Fatalf("recipient = %#v", recipient)
	}
}

func TestEventNotificationSkipsStaleRecipientWithoutRollback(t *testing.T) {
	svc, store := eventNotificationTestEnv(t)
	assignee := createEventNotificationAssignee(t, svc, store, "alice-event-stale")
	sink, _ := svc.AddNotificationSink(defaultNotificationSinkInput())
	if _, err := svc.AddEventNotificationRule(EventNotificationRuleAddInput{
		Name:         "stale-explicit",
		EventType:    "task.unblocked",
		AudienceType: "explicit_users",
		Recipients:   []string{assignee.ID},
		SinkRef:      sink.ID,
	}); err != nil {
		t.Fatalf("AddEventNotificationRule() error = %v", err)
	}
	if err := store.DB().Where("user_id = ? AND workspace_id = ?", assignee.ID, svc.Runtime().WorkspaceID).Delete(&storage.Membership{}).Error; err != nil {
		t.Fatalf("Delete stale membership error = %v", err)
	}
	blocker, _ := svc.Add(AddInput{Description: "stale blocker"})
	blocked, _ := svc.Add(AddInput{Description: "stale blocked"})
	if err := svc.Modify(blocked.UUID, ModifyInput{AddDepends: []string{blocker.UUID}}); err != nil {
		t.Fatalf("Modify(depends) error = %v", err)
	}

	if err := svc.Done(blocker.UUID); err != nil {
		t.Fatalf("Done(blocker) error = %v", err)
	}

	rows, err := storage.NewNotificationDeliveryRepository(store.DB()).List(svc.Runtime().WorkspaceID, "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("delivery count = %d, want 0: %#v", len(rows), rows)
	}
	done, err := svc.Info(blocker.UUID)
	if err != nil {
		t.Fatalf("Get(blocker) error = %v", err)
	}
	if done.Status != "completed" {
		t.Fatalf("blocker status = %q, want completed", done.Status)
	}
}

func TestEventNotificationEndpointFailureDoesNotRollbackBusinessWrite(t *testing.T) {
	svc, store := eventNotificationTestEnv(t)
	sink, err := svc.AddNotificationSink(NotificationSinkAddInput{
		Name:         "missing-config",
		Type:         "webhook",
		EndpointMode: NotificationEndpointConfigValue,
		ConfigKey:    "integrations.missing.webhook_url",
		AllowedHosts: []string{"example.com"},
	})
	if err != nil {
		t.Fatalf("AddNotificationSink() error = %v", err)
	}
	if _, err := svc.AddEventNotificationRule(EventNotificationRuleAddInput{
		Name:         "project-note-missing-endpoint",
		EventType:    "project.annotated",
		AudienceType: "actor",
		SinkRef:      sink.ID,
	}); err != nil {
		t.Fatalf("AddEventNotificationRule() error = %v", err)
	}
	project, err := svc.AddProject(AddProjectInput{Slug: "missend", Name: "Missing Endpoint"})
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}

	annotation, err := svc.ProjectAnnotate(project.Slug, "note survives notification failure")
	if err != nil {
		t.Fatalf("ProjectAnnotate() error = %v", err)
	}

	annotations, err := svc.ProjectAnnotations(project.Slug)
	if err != nil {
		t.Fatalf("ProjectAnnotations() error = %v", err)
	}
	if len(annotations) != 1 || annotations[0].ID != annotation.ID {
		t.Fatalf("annotations = %#v", annotations)
	}
	rows, err := storage.NewNotificationDeliveryRepository(store.DB()).List(svc.Runtime().WorkspaceID, "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("delivery count = %d, want 0: %#v", len(rows), rows)
	}
}

func TestProjectScopedTokenCannotCreateWorkspaceEventNotificationRule(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 1000, "local", "local")
	project, err := ownerSvc.AddProject(AddProjectInput{Slug: "scoped", Name: "Scoped"})
	if err != nil {
		t.Fatal(err)
	}
	sink, err := ownerSvc.AddNotificationSink(defaultNotificationSinkInput())
	if err != nil {
		t.Fatal(err)
	}
	created, err := ownerSvc.CreateToken(CreateTokenInput{
		Name:          "scoped-notification-token",
		Scopes:        []string{"notification:read", "notification:write"},
		WorkspaceRefs: []string{"local"},
		ProjectRefs:   []string{project.ID},
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
		RequiredCapability: "notification:write",
		RequiredPermission: PermissionNotificationWrite,
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

	_, err = scopedSvc.AddEventNotificationRule(EventNotificationRuleAddInput{
		Name:         "workspace-wide",
		EventType:    "task.unblocked",
		AudienceType: "actor",
		SinkRef:      sink.ID,
	})
	assertRuntimeCode(t, err, "project_scope_denied")

	projectRule, err := scopedSvc.AddEventNotificationRule(EventNotificationRuleAddInput{
		Name:         "project-only",
		ProjectRef:   project.ID,
		EventType:    "task.unblocked",
		AudienceType: "actor",
		SinkRef:      sink.ID,
	})
	if err != nil {
		t.Fatalf("AddEventNotificationRule(project) error = %v", err)
	}
	emptyProject := ""
	_, err = scopedSvc.ModifyEventNotificationRule(projectRule.ID, EventNotificationRuleModifyInput{ProjectRef: &emptyProject})
	assertRuntimeCode(t, err, "project_scope_denied")
}

func TestProjectAnnotateEnqueuesActorEventNotification(t *testing.T) {
	svc, store := eventNotificationTestEnv(t)
	sink, _ := svc.AddNotificationSink(defaultNotificationSinkInput())
	if _, err := svc.AddEventNotificationRule(EventNotificationRuleAddInput{
		Name:         "project-note",
		EventType:    "project.annotated",
		AudienceType: "actor",
		SinkRef:      sink.ID,
	}); err != nil {
		t.Fatalf("AddEventNotificationRule() error = %v", err)
	}
	project, err := svc.AddProject(AddProjectInput{Slug: "notesev", Name: "Notes Event"})
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}
	if _, err := svc.ProjectAnnotate(project.Slug, "project note"); err != nil {
		t.Fatalf("ProjectAnnotate() error = %v", err)
	}

	rows, err := storage.NewNotificationDeliveryRepository(store.DB()).List(svc.Runtime().WorkspaceID, "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("delivery count = %d, want 1", len(rows))
	}
	if rows[0].EventType != "project.annotated" || rows[0].ObjectKind != "project" || rows[0].ObjectID != project.ID || rows[0].TaskUUID != "" {
		t.Fatalf("delivery = %#v", rows[0])
	}
}
