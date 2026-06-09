package storage

import (
	"testing"

	"github.com/google/uuid"
)

func makeReminderRule(wsID, name, sinkID string, overrides ...func(*ReminderRule)) ReminderRule {
	enabled := true
	row := ReminderRule{
		ID:                   uuid.NewString(),
		WorkspaceID:          wsID,
		Name:                 name,
		Enabled:              &enabled,
		TriggerType:          "due_before",
		OffsetSeconds:        4 * 60 * 60,
		AfterSeconds:         0,
		RepeatPolicy:         "once",
		AudienceType:         "assignees",
		RecipientUserIDsJSON: `[]`,
		SinkID:               sinkID,
		CreatedBy:            "user-1",
		CreatedAt:            100,
		ModifiedAt:           100,
	}
	for _, fn := range overrides {
		fn(&row)
	}
	return row
}

func TestReminderRuleRepositoryListEnabledByWorkspace(t *testing.T) {
	store, wsID := newNotificationTestStore(t)
	sinkRepo := NewNotificationSinkRepository(store.DB())
	sink := makeNotificationSink(wsID, "openclaw")
	if err := sinkRepo.Create(sink); err != nil {
		t.Fatal(err)
	}
	repo := NewReminderRuleRepository(store.DB())

	enabledRule := makeReminderRule(wsID, "due-before", sink.ID)
	disabledRule := makeReminderRule(wsID, "overdue", sink.ID, func(r *ReminderRule) {
		r.TriggerType = "overdue"
		r.OffsetSeconds = 0
		r.Enabled = boolPtr(false)
	})
	if err := repo.Create(enabledRule); err != nil {
		t.Fatalf("Create enabled error = %v", err)
	}
	if err := repo.Create(disabledRule); err != nil {
		t.Fatalf("Create disabled error = %v", err)
	}

	rows, err := repo.ListEnabledByWorkspace(wsID)
	if err != nil {
		t.Fatalf("ListEnabledByWorkspace() error = %v", err)
	}
	if len(rows) != 1 || rows[0].ID != enabledRule.ID {
		t.Fatalf("ListEnabledByWorkspace() = %#v", rows)
	}
}

func TestReminderRuleRepositoryNameUniquePerWorkspace(t *testing.T) {
	store, wsID := newNotificationTestStore(t)
	sinkRepo := NewNotificationSinkRepository(store.DB())
	sink := makeNotificationSink(wsID, "openclaw")
	if err := sinkRepo.Create(sink); err != nil {
		t.Fatal(err)
	}
	repo := NewReminderRuleRepository(store.DB())

	if err := repo.Create(makeReminderRule(wsID, "due-before", sink.ID)); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(makeReminderRule(wsID, "due-before", sink.ID)); err == nil {
		t.Fatal("Create duplicate name error = nil, want unique constraint error")
	}
}

func TestReminderRuleRepositoryStoresScheduleAndFilterFields(t *testing.T) {
	store, wsID := newNotificationTestStore(t)
	sinkRepo := NewNotificationSinkRepository(store.DB())
	sink := makeNotificationSink(wsID, "openclaw")
	if err := sinkRepo.Create(sink); err != nil {
		t.Fatal(err)
	}
	repo := NewReminderRuleRepository(store.DB())

	row := makeReminderRule(wsID, "daily-due", sink.ID, func(r *ReminderRule) {
		r.ScheduleType = "daily_at"
		r.ScheduleValue = "08:50"
		r.FilterSource = "end.isnull and start.isnull and due.after:now and due.before:now+24h"
	})
	if err := repo.Create(row); err != nil {
		t.Fatalf("Create error = %v", err)
	}

	got, err := repo.GetByID(row.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.ScheduleType != "daily_at" || got.ScheduleValue != "08:50" || got.FilterSource != row.FilterSource {
		t.Fatalf("schedule/filter fields = (%q, %q, %q)", got.ScheduleType, got.ScheduleValue, got.FilterSource)
	}
}
