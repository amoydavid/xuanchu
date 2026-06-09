package storage

import (
	"testing"

	"github.com/google/uuid"
)

func makeEventNotificationRule(wsID, name, sinkID, eventType string, overrides ...func(*EventNotificationRule)) EventNotificationRule {
	enabled := true
	row := EventNotificationRule{
		ID:                   uuid.NewString(),
		WorkspaceID:          wsID,
		Name:                 name,
		Enabled:              &enabled,
		EventType:            eventType,
		FilterSource:         "",
		AudienceType:         "assignees",
		RecipientUserIDsJSON: `[]`,
		SinkID:               sinkID,
		TemplateSubject:      "",
		TemplateBody:         "",
		CreatedBy:            "user-1",
		CreatedAt:            100,
		ModifiedAt:           100,
	}
	for _, fn := range overrides {
		fn(&row)
	}
	return row
}

func TestEventNotificationRuleRepositoryNameUniquePerWorkspace(t *testing.T) {
	store, wsID := newNotificationTestStore(t)
	sink := makeNotificationSink(wsID, "openclaw")
	if err := NewNotificationSinkRepository(store.DB()).Create(sink); err != nil {
		t.Fatal(err)
	}
	repo := NewEventNotificationRuleRepository(store.DB())

	if err := repo.Create(makeEventNotificationRule(wsID, "unblocked", sink.ID, "task.unblocked")); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := repo.Create(makeEventNotificationRule(wsID, "unblocked", sink.ID, "task.unblocked")); err == nil {
		t.Fatal("Create duplicate name error = nil, want unique constraint error")
	}
	otherWS := "ws-other"
	otherSink := makeNotificationSink(otherWS, "openclaw")
	if err := NewNotificationSinkRepository(store.DB()).Create(otherSink); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(makeEventNotificationRule(otherWS, "unblocked", otherSink.ID, "task.unblocked")); err != nil {
		t.Fatalf("Create same name in other workspace error = %v", err)
	}
}

func TestEventNotificationRuleRepositoryListMatching(t *testing.T) {
	store, wsID := newNotificationTestStore(t)
	sink := makeNotificationSink(wsID, "openclaw")
	if err := NewNotificationSinkRepository(store.DB()).Create(sink); err != nil {
		t.Fatal(err)
	}
	repo := NewEventNotificationRuleRepository(store.DB())
	projectID := "project-1"
	otherProjectID := "project-2"
	workspaceRule := makeEventNotificationRule(wsID, "workspace-unblocked", sink.ID, "task.unblocked")
	projectRule := makeEventNotificationRule(wsID, "project-unblocked", sink.ID, "task.unblocked", func(r *EventNotificationRule) {
		r.ProjectID = &projectID
	})
	disabledRule := makeEventNotificationRule(wsID, "disabled-unblocked", sink.ID, "task.unblocked", func(r *EventNotificationRule) {
		r.Enabled = boolPtr(false)
	})
	otherEventRule := makeEventNotificationRule(wsID, "annotated", sink.ID, "project.annotated")
	otherProjectRule := makeEventNotificationRule(wsID, "other-project-unblocked", sink.ID, "task.unblocked", func(r *EventNotificationRule) {
		r.ProjectID = &otherProjectID
	})
	for _, row := range []EventNotificationRule{workspaceRule, projectRule, disabledRule, otherEventRule, otherProjectRule} {
		if err := repo.Create(row); err != nil {
			t.Fatalf("Create(%s) error = %v", row.Name, err)
		}
	}

	rows, err := repo.ListMatching(wsID, &projectID, "task.unblocked")
	if err != nil {
		t.Fatalf("ListMatching() error = %v", err)
	}
	if len(rows) != 2 || rows[0].ID != workspaceRule.ID || rows[1].ID != projectRule.ID {
		t.Fatalf("ListMatching(project) = %#v", rows)
	}
	rows, err = repo.ListMatching(wsID, nil, "task.unblocked")
	if err != nil {
		t.Fatalf("ListMatching(workspace event) error = %v", err)
	}
	if len(rows) != 1 || rows[0].ID != workspaceRule.ID {
		t.Fatalf("ListMatching(workspace event) = %#v", rows)
	}
	rows, err = repo.ListMatching("ws-other", &projectID, "task.unblocked")
	if err != nil {
		t.Fatalf("ListMatching(other workspace) error = %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("ListMatching(other workspace) = %#v, want empty", rows)
	}
}
