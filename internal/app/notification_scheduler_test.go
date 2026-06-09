package app

import (
	"path/filepath"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

func TestReminderRuleTaskFilterLimitsSchedulerCandidates(t *testing.T) {
	store, err := storage.Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	now := int64(1_700_000_000)
	svc, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: now}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	user := svc.Runtime().ActorUserID
	due := now - 60
	if _, err := svc.Add(AddInput{Description: "pending overdue", Due: &due, Assignees: []string{user}}); err != nil {
		t.Fatalf("Add(pending) error = %v", err)
	}
	wait := now + 3600
	waiting, err := svc.Add(AddInput{Description: "waiting overdue", Due: &due, Wait: &wait, Assignees: []string{user}})
	if err != nil {
		t.Fatalf("Add(waiting) error = %v", err)
	}
	if waiting.Status != task.StatusWaiting {
		t.Fatalf("waiting task status = %q, want %q", waiting.Status, task.StatusWaiting)
	}
	sink, err := svc.AddNotificationSink(NotificationSinkAddInput{
		Name:         "openclaw",
		Type:         string(NotificationSinkTypeWebhook),
		EndpointMode: string(NotificationEndpointStaticURL),
		URL:          "http://93.184.216.34/notifications",
	})
	if err != nil {
		t.Fatalf("AddNotificationSink() error = %v", err)
	}
	rule, err := svc.AddReminderRule(ReminderRuleAddInput{
		Name:          "pending-only",
		TriggerType:   "overdue",
		RepeatPolicy:  "once",
		TaskFilter:    "status:pending",
		AudienceType:  string(ReminderAudienceAssignees),
		SinkRef:       sink.ID,
		AfterSeconds:  0,
		OffsetSeconds: 0,
	})
	if err != nil {
		t.Fatalf("AddReminderRule() error = %v", err)
	}
	if rule.TaskFilter != "status:pending" {
		t.Fatalf("TaskFilter = %q, want status:pending", rule.TaskFilter)
	}

	scheduler := NewReminderScheduler(ReminderSchedulerOptions{Store: store, Clock: FixedClock{NowUnix: now}, BatchSize: 10})
	result, err := scheduler.RunOnce(nil)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if result.DeliveriesEnqueued != 1 {
		t.Fatalf("RunOnce result = %#v, want 1 delivery", result)
	}
	deliveries, err := svc.ListNotificationDeliveries("", 10, 0)
	if err != nil {
		t.Fatalf("ListNotificationDeliveries() error = %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("deliveries = %d, want 1", len(deliveries))
	}
	if deliveries[0].TaskUUID == "" {
		t.Fatal("delivery task UUID is empty")
	}
}
