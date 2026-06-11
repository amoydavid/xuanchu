package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.dajee.net/dajee/xuanchu/internal/logging"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

func createSchedulerAssignee(t *testing.T, svc *Service, store *storage.Store, name string) storage.User {
	t.Helper()
	user := mustCreateUserRecord(t, store, storage.User{ID: uuid.NewString(), Name: name, CreatedAt: 100, ModifiedAt: 100})
	mustUpsertMembershipRecord(t, store, storage.Membership{UserID: user.ID, WorkspaceID: svc.Runtime().WorkspaceID, Role: string(RoleMember), JoinedAt: 100, ModifiedAt: 100})
	if _, err := storage.NewExternalIDRepository(store.DB()).Create(storage.UserExternalID{ID: uuid.NewString(), UserID: user.ID, Provider: "openclaw", ExternalID: "openclaw-" + name, CreatedAt: 100}); err != nil {
		t.Fatalf("Create external id error = %v", err)
	}
	return user
}

func TestReminderSchedulerDueBeforeEnqueuesDelivery(t *testing.T) {
	store := newTestStore(t)
	svc, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: 1000}})
	if err != nil {
		t.Fatal(err)
	}
	alice := createSchedulerAssignee(t, svc, store, "alice")
	due := int64(2000)
	_, err = svc.Add(AddInput{Description: "完成 OAuth", Due: &due, Assignees: []string{"alice"}})
	if err != nil {
		t.Fatalf("Add task error = %v", err)
	}
	sink, err := svc.AddNotificationSink(defaultNotificationSinkInput())
	if err != nil {
		t.Fatalf("AddNotificationSink error = %v", err)
	}
	if _, err := svc.AddReminderRule(ReminderRuleAddInput{Name: "due-before", TriggerType: "due_before", OffsetSeconds: 1200, AudienceType: "assignees", SinkRef: sink.ID}); err != nil {
		t.Fatalf("AddReminderRule error = %v", err)
	}

	scheduler := NewReminderScheduler(ReminderSchedulerOptions{Store: store, Clock: FixedClock{NowUnix: 1000}})
	result, err := scheduler.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if result.DeliveriesEnqueued != 1 {
		t.Fatalf("DeliveriesEnqueued = %d, want 1", result.DeliveriesEnqueued)
	}
	rows, err := storage.NewNotificationDeliveryRepository(store.DB()).List(svc.Runtime().WorkspaceID, "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("delivery count = %d, want 1", len(rows))
	}
	if rows[0].RecipientUserID != alice.ID || rows[0].EventType != "task.due_soon" {
		t.Fatalf("delivery = %#v", rows[0])
	}
	if !strings.Contains(rows[0].PayloadJSON, "openclaw-alice") {
		t.Fatalf("payload missing recipient external id: %s", rows[0].PayloadJSON)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(rows[0].PayloadJSON), &payload); err != nil {
		t.Fatalf("PayloadJSON invalid: %v", err)
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
	if object["kind"] != "task" || object["id"] != rows[0].TaskUUID {
		t.Fatalf("object payload = %#v", object)
	}
}

func TestReminderSchedulerWritesOperationLog(t *testing.T) {
	var logBuf bytes.Buffer
	logger, closeLogger, err := logging.Setup(logging.LogConfig{Format: "text"}, &logBuf)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closeLogger() })

	store := newTestStore(t)
	svc, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: 1000}})
	if err != nil {
		t.Fatal(err)
	}
	_ = createSchedulerAssignee(t, svc, store, "alice")
	due := int64(2000)
	if _, err := svc.Add(AddInput{Description: "完成 OAuth", Due: &due, Assignees: []string{"alice"}}); err != nil {
		t.Fatalf("Add task error = %v", err)
	}
	sink, err := svc.AddNotificationSink(defaultNotificationSinkInput())
	if err != nil {
		t.Fatalf("AddNotificationSink error = %v", err)
	}
	if _, err := svc.AddReminderRule(ReminderRuleAddInput{Name: "due-before", TriggerType: "due_before", OffsetSeconds: 1200, AudienceType: "assignees", SinkRef: sink.ID}); err != nil {
		t.Fatalf("AddReminderRule error = %v", err)
	}

	scheduler := NewReminderScheduler(ReminderSchedulerOptions{Store: store, Clock: FixedClock{NowUnix: 1000}, Logger: logger})
	if _, err := scheduler.RunOnce(t.Context()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	logText := logBuf.String()
	for _, want := range []string{
		"component=reminder_scheduler",
		"operation=reminder_rule_scan",
		"rules_checked=1",
		"deliveries_enqueued=1",
		"result=success",
	} {
		if !strings.Contains(logText, want) {
			t.Fatalf("log = %q, want substring %q", logText, want)
		}
	}
}

func TestReminderSchedulerDedupePreventsDuplicateDelivery(t *testing.T) {
	store := newTestStore(t)
	svc, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: 1000}})
	if err != nil {
		t.Fatal(err)
	}
	_ = createSchedulerAssignee(t, svc, store, "alice")
	due := int64(2000)
	if _, err := svc.Add(AddInput{Description: "完成 OAuth", Due: &due, Assignees: []string{"alice"}}); err != nil {
		t.Fatal(err)
	}
	sink, _ := svc.AddNotificationSink(defaultNotificationSinkInput())
	if _, err := svc.AddReminderRule(ReminderRuleAddInput{Name: "due-before", TriggerType: "due_before", OffsetSeconds: 1200, AudienceType: "assignees", SinkRef: sink.ID}); err != nil {
		t.Fatal(err)
	}
	scheduler := NewReminderScheduler(ReminderSchedulerOptions{Store: store, Clock: FixedClock{NowUnix: 1000}})
	if _, err := scheduler.RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := scheduler.RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	rows, err := storage.NewNotificationDeliveryRepository(store.DB()).List(svc.Runtime().WorkspaceID, "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("delivery count = %d, want 1", len(rows))
	}
}

func TestReminderSchedulerDailyFilterEnqueuesOncePerDay(t *testing.T) {
	store := newTestStore(t)
	now := time.Date(2026, 6, 8, 8, 55, 0, 0, time.Local).Unix()
	svc, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: now}})
	if err != nil {
		t.Fatal(err)
	}
	alice := createSchedulerAssignee(t, svc, store, "alice")
	due := time.Date(2026, 6, 8, 17, 0, 0, 0, time.Local).Unix()
	if _, err := svc.Add(AddInput{Description: "今日要完成", Due: &due, Assignees: []string{"alice"}}); err != nil {
		t.Fatal(err)
	}
	sink, _ := svc.AddNotificationSink(defaultNotificationSinkInput())
	if _, err := svc.AddReminderRule(ReminderRuleAddInput{
		Name:          "daily-filter",
		ScheduleType:  "daily_at",
		ScheduleValue: "08:50",
		FilterSource:  "end.isnull and start.isnull and due.after:now and due.before:now+24h",
		AudienceType:  "assignees",
		SinkRef:       sink.ID,
	}); err != nil {
		t.Fatal(err)
	}

	scheduler := NewReminderScheduler(ReminderSchedulerOptions{Store: store, Clock: FixedClock{NowUnix: now}})
	first, err := scheduler.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("first RunOnce() error = %v", err)
	}
	second, err := scheduler.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("second RunOnce() error = %v", err)
	}
	if first.DeliveriesEnqueued != 1 || second.DeliveriesEnqueued != 0 {
		t.Fatalf("enqueued = (%d, %d), want (1, 0)", first.DeliveriesEnqueued, second.DeliveriesEnqueued)
	}
	rows, err := storage.NewNotificationDeliveryRepository(store.DB()).List(svc.Runtime().WorkspaceID, "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].RecipientUserID != alice.ID {
		t.Fatalf("deliveries = %#v", rows)
	}
	if !strings.Contains(rows[0].DedupeKey, "2026-06-08") {
		t.Fatalf("DedupeKey = %q, want schedule date", rows[0].DedupeKey)
	}
}

func TestReminderSchedulerDailyFilterReminderSequenceAndWindow(t *testing.T) {
	store := newTestStore(t)
	day1 := time.Date(2026, 6, 8, 8, 55, 0, 0, time.Local).Unix()
	day2 := time.Date(2026, 6, 9, 8, 55, 0, 0, time.Local).Unix()
	svc, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: day1}})
	if err != nil {
		t.Fatal(err)
	}
	alice := createSchedulerAssignee(t, svc, store, "alice")
	due := time.Date(2026, 6, 7, 17, 0, 0, 0, time.Local).Unix()
	if _, err := svc.Add(AddInput{Description: "已经逾期", Due: &due, Assignees: []string{"alice"}}); err != nil {
		t.Fatal(err)
	}
	sink, err := svc.AddNotificationSink(NotificationSinkAddInput{
		Name:            "third-party",
		Type:            NotificationSinkTypeHTTPTemplate,
		EndpointMode:    NotificationEndpointStaticURL,
		URL:             "https://example.com/notify",
		AllowedHosts:    []string{"example.com"},
		BodyContentType: "application/json",
		BodyTemplate:    `{"sequence":{{reminder.sequence}},"overdue_sequence":{{reminder.overdue_sequence}},"window_start":{{reminder.window_start}},"window_end":{{reminder.window_end}}}`,
	})
	if err != nil {
		t.Fatalf("AddNotificationSink() error = %v", err)
	}
	if _, err := svc.AddReminderRule(ReminderRuleAddInput{
		Name:          "daily-overdue",
		ScheduleType:  "daily_at",
		ScheduleValue: "08:50",
		FilterSource:  "due.before:now",
		AudienceType:  "assignees",
		SinkRef:       sink.ID,
	}); err != nil {
		t.Fatal(err)
	}

	firstScheduler := NewReminderScheduler(ReminderSchedulerOptions{Store: store, Clock: FixedClock{NowUnix: day1}})
	first, err := firstScheduler.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("first RunOnce() error = %v", err)
	}
	second, err := firstScheduler.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("same-day RunOnce() error = %v", err)
	}
	nextDayScheduler := NewReminderScheduler(ReminderSchedulerOptions{Store: store, Clock: FixedClock{NowUnix: day2}})
	third, err := nextDayScheduler.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("next-day RunOnce() error = %v", err)
	}
	if first.DeliveriesEnqueued != 1 || second.DeliveriesEnqueued != 0 || third.DeliveriesEnqueued != 1 {
		t.Fatalf("enqueued = (%d, %d, %d), want (1, 0, 1)", first.DeliveriesEnqueued, second.DeliveriesEnqueued, third.DeliveriesEnqueued)
	}
	rows, err := storage.NewNotificationDeliveryRepository(store.DB()).List(svc.Runtime().WorkspaceID, "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("delivery count = %d, want 2", len(rows))
	}
	var firstPayload, secondPayload map[string]any
	if err := json.Unmarshal([]byte(rows[1].PayloadJSON), &firstPayload); err != nil {
		t.Fatalf("first PayloadJSON invalid: %v", err)
	}
	if err := json.Unmarshal([]byte(rows[0].PayloadJSON), &secondPayload); err != nil {
		t.Fatalf("second PayloadJSON invalid: %v", err)
	}
	assertReminderPayload := func(t *testing.T, payload map[string]any, sequence int, start int64) {
		t.Helper()
		reminder := payload["reminder"].(map[string]any)
		if payload["event_type"] != "task.overdue" {
			t.Fatalf("event_type = %#v, want task.overdue", payload["event_type"])
		}
		if reminder["sequence"] != float64(sequence) {
			t.Fatalf("sequence = %#v, want %d", reminder["sequence"], sequence)
		}
		if reminder["overdue_sequence"] != float64(sequence) {
			t.Fatalf("overdue_sequence = %#v, want %d", reminder["overdue_sequence"], sequence)
		}
		if reminder["window_start"] != float64(start) || reminder["window_end"] != float64(start+24*60*60) {
			t.Fatalf("window = (%#v, %#v), want (%d, %d)", reminder["window_start"], reminder["window_end"], start, start+24*60*60)
		}
	}
	day1Start := time.Date(2026, 6, 8, 8, 50, 0, 0, time.Local).Unix()
	day2Start := time.Date(2026, 6, 9, 8, 50, 0, 0, time.Local).Unix()
	assertReminderPayload(t, firstPayload, 1, day1Start)
	assertReminderPayload(t, secondPayload, 2, day2Start)
	if !strings.Contains(rows[0].RenderedBody, `"sequence":2`) || !strings.Contains(rows[0].RenderedBody, `"overdue_sequence":2`) {
		t.Fatalf("RenderedBody = %s, want frozen second reminder context", rows[0].RenderedBody)
	}
	if rows[0].RecipientUserID != alice.ID {
		t.Fatalf("recipient = %s, want %s", rows[0].RecipientUserID, alice.ID)
	}
}

func TestReminderSchedulerDailyDueSoonReminderOverdueSequenceIsZero(t *testing.T) {
	store := newTestStore(t)
	now := time.Date(2026, 6, 8, 8, 55, 0, 0, time.Local).Unix()
	svc, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: now}})
	if err != nil {
		t.Fatal(err)
	}
	_ = createSchedulerAssignee(t, svc, store, "alice")
	due := time.Date(2026, 6, 8, 17, 0, 0, 0, time.Local).Unix()
	if _, err := svc.Add(AddInput{Description: "今日要完成", Due: &due, Assignees: []string{"alice"}}); err != nil {
		t.Fatal(err)
	}
	sink, _ := svc.AddNotificationSink(defaultNotificationSinkInput())
	if _, err := svc.AddReminderRule(ReminderRuleAddInput{
		Name:          "daily-due-soon",
		ScheduleType:  "daily_at",
		ScheduleValue: "08:50",
		FilterSource:  "due.after:now and due.before:now+24h",
		AudienceType:  "assignees",
		SinkRef:       sink.ID,
	}); err != nil {
		t.Fatal(err)
	}

	scheduler := NewReminderScheduler(ReminderSchedulerOptions{Store: store, Clock: FixedClock{NowUnix: now}})
	if _, err := scheduler.RunOnce(t.Context()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	rows, err := storage.NewNotificationDeliveryRepository(store.DB()).List(svc.Runtime().WorkspaceID, "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("delivery count = %d, want 1", len(rows))
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(rows[0].PayloadJSON), &payload); err != nil {
		t.Fatalf("PayloadJSON invalid: %v", err)
	}
	reminder := payload["reminder"].(map[string]any)
	if payload["event_type"] != "task.due_soon" {
		t.Fatalf("event_type = %#v, want task.due_soon", payload["event_type"])
	}
	if reminder["sequence"] != float64(1) {
		t.Fatalf("sequence = %#v, want 1", reminder["sequence"])
	}
	if reminder["overdue_sequence"] != float64(0) {
		t.Fatalf("overdue_sequence = %#v, want 0", reminder["overdue_sequence"])
	}
}

func TestReminderSchedulerDailyFilterNotDueBeforeNowIsNotOverdue(t *testing.T) {
	store := newTestStore(t)
	now := time.Date(2026, 6, 8, 8, 55, 0, 0, time.Local).Unix()
	svc, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: now}})
	if err != nil {
		t.Fatal(err)
	}
	_ = createSchedulerAssignee(t, svc, store, "alice")
	due := time.Date(2026, 6, 8, 17, 0, 0, 0, time.Local).Unix()
	if _, err := svc.Add(AddInput{Description: "未逾期任务", Due: &due, Assignees: []string{"alice"}}); err != nil {
		t.Fatal(err)
	}
	sink, _ := svc.AddNotificationSink(defaultNotificationSinkInput())
	if _, err := svc.AddReminderRule(ReminderRuleAddInput{
		Name:          "daily-not-overdue",
		ScheduleType:  "daily_at",
		ScheduleValue: "08:50",
		FilterSource:  "not due.before:now",
		AudienceType:  "assignees",
		SinkRef:       sink.ID,
	}); err != nil {
		t.Fatal(err)
	}

	scheduler := NewReminderScheduler(ReminderSchedulerOptions{Store: store, Clock: FixedClock{NowUnix: now}})
	if _, err := scheduler.RunOnce(t.Context()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	rows, err := storage.NewNotificationDeliveryRepository(store.DB()).List(svc.Runtime().WorkspaceID, "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("delivery count = %d, want 1", len(rows))
	}
	if rows[0].EventType != "task.due_soon" {
		t.Fatalf("EventType = %q, want task.due_soon", rows[0].EventType)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(rows[0].PayloadJSON), &payload); err != nil {
		t.Fatalf("PayloadJSON invalid: %v", err)
	}
	reminder := payload["reminder"].(map[string]any)
	if payload["event_type"] != "task.due_soon" {
		t.Fatalf("event_type = %#v, want task.due_soon", payload["event_type"])
	}
	if reminder["overdue_sequence"] != float64(0) {
		t.Fatalf("overdue_sequence = %#v, want 0", reminder["overdue_sequence"])
	}
}

func TestReminderSchedulerDailyFilterOrFutureBranchIsNotOverdue(t *testing.T) {
	store := newTestStore(t)
	now := time.Date(2026, 6, 8, 8, 55, 0, 0, time.Local).Unix()
	svc, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: now}})
	if err != nil {
		t.Fatal(err)
	}
	_ = createSchedulerAssignee(t, svc, store, "alice")
	due := time.Date(2026, 6, 10, 17, 0, 0, 0, time.Local).Unix()
	if _, err := svc.Add(AddInput{Description: "未来任务", Due: &due, Assignees: []string{"alice"}}); err != nil {
		t.Fatal(err)
	}
	sink, _ := svc.AddNotificationSink(defaultNotificationSinkInput())
	if _, err := svc.AddReminderRule(ReminderRuleAddInput{
		Name:          "daily-overdue-or-future",
		ScheduleType:  "daily_at",
		ScheduleValue: "08:50",
		FilterSource:  "due.before:now or due.after:now+24h",
		AudienceType:  "assignees",
		SinkRef:       sink.ID,
	}); err != nil {
		t.Fatal(err)
	}

	scheduler := NewReminderScheduler(ReminderSchedulerOptions{Store: store, Clock: FixedClock{NowUnix: now}})
	if _, err := scheduler.RunOnce(t.Context()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	rows, err := storage.NewNotificationDeliveryRepository(store.DB()).List(svc.Runtime().WorkspaceID, "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("delivery count = %d, want 1", len(rows))
	}
	if rows[0].EventType != "task.due_soon" {
		t.Fatalf("EventType = %q, want task.due_soon", rows[0].EventType)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(rows[0].PayloadJSON), &payload); err != nil {
		t.Fatalf("PayloadJSON invalid: %v", err)
	}
	reminder := payload["reminder"].(map[string]any)
	if payload["event_type"] != "task.due_soon" {
		t.Fatalf("event_type = %#v, want task.due_soon", payload["event_type"])
	}
	if reminder["overdue_sequence"] != float64(0) {
		t.Fatalf("overdue_sequence = %#v, want 0", reminder["overdue_sequence"])
	}
}

func TestReminderSchedulerSequenceCountsDifferentEventTypeHistory(t *testing.T) {
	store := newTestStore(t)
	now := time.Date(2026, 6, 8, 8, 55, 0, 0, time.Local).Unix()
	svc, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: now}})
	if err != nil {
		t.Fatal(err)
	}
	alice := createSchedulerAssignee(t, svc, store, "alice")
	due := time.Date(2026, 6, 8, 17, 0, 0, 0, time.Local).Unix()
	created, err := svc.Add(AddInput{Description: "今日要完成", Due: &due, Assignees: []string{"alice"}})
	if err != nil {
		t.Fatal(err)
	}
	sink, _ := svc.AddNotificationSink(defaultNotificationSinkInput())
	rule, err := svc.AddReminderRule(ReminderRuleAddInput{
		Name:          "daily-due-soon",
		ScheduleType:  "daily_at",
		ScheduleValue: "08:50",
		FilterSource:  "due.after:now and due.before:now+24h",
		AudienceType:  "assignees",
		SinkRef:       sink.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	history := makeAppNotificationDelivery(svc.Runtime().WorkspaceID, sink.ID, storage.DeliveryStatusSucceeded)
	history.RuleID = rule.ID
	history.TaskUUID = created.UUID
	history.RecipientUserID = alice.ID
	history.EventType = "task.overdue"
	history.DedupeKey = uuid.NewString()
	history.CreatedAt = now - 86400
	history.ModifiedAt = now - 86400
	if err := storage.NewNotificationDeliveryRepository(store.DB()).Enqueue([]storage.NotificationDelivery{history}); err != nil {
		t.Fatalf("enqueue history error = %v", err)
	}

	scheduler := NewReminderScheduler(ReminderSchedulerOptions{Store: store, Clock: FixedClock{NowUnix: now}})
	if _, err := scheduler.RunOnce(t.Context()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	rows, err := storage.NewNotificationDeliveryRepository(store.DB()).List(svc.Runtime().WorkspaceID, "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("delivery count = %d, want 2", len(rows))
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(rows[0].PayloadJSON), &payload); err != nil {
		t.Fatalf("PayloadJSON invalid: %v", err)
	}
	reminder := payload["reminder"].(map[string]any)
	if rows[0].EventType != "task.due_soon" {
		t.Fatalf("EventType = %q, want task.due_soon", rows[0].EventType)
	}
	if reminder["sequence"] != float64(2) {
		t.Fatalf("sequence = %#v, want 2", reminder["sequence"])
	}
}

func TestReminderSchedulerDedupeIgnoresEventType(t *testing.T) {
	store := newTestStore(t)
	now := time.Date(2026, 6, 8, 8, 55, 0, 0, time.Local).Unix()
	svc, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: now}})
	if err != nil {
		t.Fatal(err)
	}
	alice := createSchedulerAssignee(t, svc, store, "alice")
	due := time.Date(2026, 6, 8, 17, 0, 0, 0, time.Local).Unix()
	created, err := svc.Add(AddInput{Description: "今日要完成", Due: &due, Assignees: []string{"alice"}})
	if err != nil {
		t.Fatal(err)
	}
	sink, _ := svc.AddNotificationSink(defaultNotificationSinkInput())
	rule, err := svc.AddReminderRule(ReminderRuleAddInput{
		Name:          "daily-due-soon",
		ScheduleType:  "daily_at",
		ScheduleValue: "08:50",
		FilterSource:  "due.after:now and due.before:now+24h",
		AudienceType:  "assignees",
		SinkRef:       sink.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	history := makeAppNotificationDelivery(svc.Runtime().WorkspaceID, sink.ID, storage.DeliveryStatusSucceeded)
	history.RuleID = rule.ID
	history.TaskUUID = created.UUID
	history.RecipientUserID = alice.ID
	history.EventType = "task.overdue"
	history.DedupeKey = fmt.Sprintf("%s:%s:%s:%s:%s", svc.Runtime().WorkspaceID, rule.ID, created.UUID, alice.ID, "2026-06-08")
	history.CreatedAt = now - 60
	history.ModifiedAt = now - 60
	if err := storage.NewNotificationDeliveryRepository(store.DB()).Enqueue([]storage.NotificationDelivery{history}); err != nil {
		t.Fatalf("enqueue history error = %v", err)
	}

	scheduler := NewReminderScheduler(ReminderSchedulerOptions{Store: store, Clock: FixedClock{NowUnix: now}})
	result, err := scheduler.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if result.DeliveriesEnqueued != 0 {
		t.Fatalf("DeliveriesEnqueued = %d, want 0", result.DeliveriesEnqueued)
	}
	rows, err := storage.NewNotificationDeliveryRepository(store.DB()).List(svc.Runtime().WorkspaceID, "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("delivery count = %d, want existing history only", len(rows))
	}
}

func TestReminderSchedulerDailyFilterSkipsBeforeScheduleTime(t *testing.T) {
	store := newTestStore(t)
	now := time.Date(2026, 6, 8, 8, 45, 0, 0, time.Local).Unix()
	svc, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: now}})
	if err != nil {
		t.Fatal(err)
	}
	_ = createSchedulerAssignee(t, svc, store, "alice")
	due := time.Date(2026, 6, 8, 17, 0, 0, 0, time.Local).Unix()
	if _, err := svc.Add(AddInput{Description: "今日要完成", Due: &due, Assignees: []string{"alice"}}); err != nil {
		t.Fatal(err)
	}
	sink, _ := svc.AddNotificationSink(defaultNotificationSinkInput())
	if _, err := svc.AddReminderRule(ReminderRuleAddInput{
		Name:          "daily-filter",
		ScheduleType:  "daily_at",
		ScheduleValue: "08:50",
		FilterSource:  "end.isnull and start.isnull and due.after:now and due.before:now+24h",
		AudienceType:  "assignees",
		SinkRef:       sink.ID,
	}); err != nil {
		t.Fatal(err)
	}

	scheduler := NewReminderScheduler(ReminderSchedulerOptions{Store: store, Clock: FixedClock{NowUnix: now}})
	result, err := scheduler.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if result.DeliveriesEnqueued != 0 {
		t.Fatalf("DeliveriesEnqueued = %d, want 0", result.DeliveriesEnqueued)
	}
}

func TestReminderSchedulerDailyFilterHonorsProjectScope(t *testing.T) {
	store := newTestStore(t)
	now := time.Date(2026, 6, 8, 8, 55, 0, 0, time.Local).Unix()
	svc, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: now}})
	if err != nil {
		t.Fatal(err)
	}
	alice := createSchedulerAssignee(t, svc, store, "alice")
	projectA, err := svc.AddProject(AddProjectInput{Slug: "alpha", Name: "Alpha"})
	if err != nil {
		t.Fatal(err)
	}
	projectB, err := svc.AddProject(AddProjectInput{Slug: "beta", Name: "Beta"})
	if err != nil {
		t.Fatal(err)
	}
	due := time.Date(2026, 6, 8, 17, 0, 0, 0, time.Local).Unix()
	if _, err := svc.Add(AddInput{Description: "alpha task", Project: &projectA.Slug, Due: &due, Assignees: []string{"alice"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(AddInput{Description: "beta task", Project: &projectB.Slug, Due: &due, Assignees: []string{"alice"}}); err != nil {
		t.Fatal(err)
	}
	sink, _ := svc.AddNotificationSink(defaultNotificationSinkInput())
	if _, err := svc.AddReminderRule(ReminderRuleAddInput{
		Name:          "daily-alpha",
		ProjectRef:    projectA.ID,
		ScheduleType:  "daily_at",
		ScheduleValue: "08:50",
		FilterSource:  "end.isnull and start.isnull and due.after:now and due.before:now+24h",
		AudienceType:  "assignees",
		SinkRef:       sink.ID,
	}); err != nil {
		t.Fatal(err)
	}

	scheduler := NewReminderScheduler(ReminderSchedulerOptions{Store: store, Clock: FixedClock{NowUnix: now}})
	result, err := scheduler.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if result.DeliveriesEnqueued != 1 {
		t.Fatalf("DeliveriesEnqueued = %d, want 1", result.DeliveriesEnqueued)
	}
	rows, err := storage.NewNotificationDeliveryRepository(store.DB()).List(svc.Runtime().WorkspaceID, "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ProjectID == nil || *rows[0].ProjectID != projectA.ID || rows[0].RecipientUserID != alice.ID {
		t.Fatalf("deliveries = %#v", rows)
	}
}

func TestReminderSchedulerDailyFilterAppliesProjectScopeBeforeLimit(t *testing.T) {
	store := newTestStore(t)
	now := time.Date(2026, 6, 8, 8, 55, 0, 0, time.Local).Unix()
	svc, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: now}})
	if err != nil {
		t.Fatal(err)
	}
	_ = createSchedulerAssignee(t, svc, store, "alice")
	projectA, err := svc.AddProject(AddProjectInput{Slug: "alpha", Name: "Alpha"})
	if err != nil {
		t.Fatal(err)
	}
	projectB, err := svc.AddProject(AddProjectInput{Slug: "beta", Name: "Beta"})
	if err != nil {
		t.Fatal(err)
	}
	due := time.Date(2026, 6, 8, 17, 0, 0, 0, time.Local).Unix()
	if _, err := svc.Add(AddInput{Description: "beta first", Project: &projectB.Slug, Due: &due, Assignees: []string{"alice"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(AddInput{Description: "alpha target", Project: &projectA.Slug, Due: &due, Assignees: []string{"alice"}}); err != nil {
		t.Fatal(err)
	}
	sink, _ := svc.AddNotificationSink(defaultNotificationSinkInput())
	if _, err := svc.AddReminderRule(ReminderRuleAddInput{
		Name:          "daily-alpha-small-batch",
		ProjectRef:    projectA.ID,
		ScheduleType:  "daily_at",
		ScheduleValue: "08:50",
		FilterSource:  "end.isnull and start.isnull and due.after:now and due.before:now+24h",
		AudienceType:  "assignees",
		SinkRef:       sink.ID,
	}); err != nil {
		t.Fatal(err)
	}

	scheduler := NewReminderScheduler(ReminderSchedulerOptions{Store: store, Clock: FixedClock{NowUnix: now}, BatchSize: 1})
	result, err := scheduler.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if result.DeliveriesEnqueued != 1 {
		t.Fatalf("DeliveriesEnqueued = %d, want 1", result.DeliveriesEnqueued)
	}
	rows, err := storage.NewNotificationDeliveryRepository(store.DB()).List(svc.Runtime().WorkspaceID, "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ProjectID == nil || *rows[0].ProjectID != projectA.ID {
		t.Fatalf("deliveries = %#v", rows)
	}
}

func TestReminderSchedulerSkipsCompletedDeletedAndNoDue(t *testing.T) {
	store := newTestStore(t)
	svc, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: 1000}})
	if err != nil {
		t.Fatal(err)
	}
	_ = createSchedulerAssignee(t, svc, store, "alice")
	due := int64(900)
	completed, err := svc.Add(AddInput{Description: "done", Due: &due, Assignees: []string{"alice"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Done(completed.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(AddInput{Description: "no due", Assignees: []string{"alice"}}); err != nil {
		t.Fatal(err)
	}
	sink, _ := svc.AddNotificationSink(defaultNotificationSinkInput())
	if _, err := svc.AddReminderRule(ReminderRuleAddInput{Name: "overdue", TriggerType: "overdue", AudienceType: "assignees", SinkRef: sink.ID}); err != nil {
		t.Fatal(err)
	}
	scheduler := NewReminderScheduler(ReminderSchedulerOptions{Store: store, Clock: FixedClock{NowUnix: 1000}})
	if _, err := scheduler.RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	rows, err := storage.NewNotificationDeliveryRepository(store.DB()).List(svc.Runtime().WorkspaceID, "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("delivery count = %d, want 0", len(rows))
	}
}

func TestReminderSchedulerPayloadUsesFullUserInfo(t *testing.T) {
	store := newTestStore(t)
	svc, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: 1000}})
	if err != nil {
		t.Fatal(err)
	}
	alice := createSchedulerAssignee(t, svc, store, "alice")
	due := int64(900)
	if _, err := svc.Add(AddInput{Description: "逾期任务", Due: &due, Assignees: []string{"alice"}}); err != nil {
		t.Fatal(err)
	}
	sink, _ := svc.AddNotificationSink(defaultNotificationSinkInput())
	if _, err := svc.AddReminderRule(ReminderRuleAddInput{Name: "overdue", TriggerType: "overdue", AudienceType: "assignees", SinkRef: sink.ID}); err != nil {
		t.Fatal(err)
	}
	scheduler := NewReminderScheduler(ReminderSchedulerOptions{Store: store, Clock: FixedClock{NowUnix: 1000}})
	if _, err := scheduler.RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	rows, _ := storage.NewNotificationDeliveryRepository(store.DB()).List(svc.Runtime().WorkspaceID, "", 10, 0)
	var payload map[string]any
	if err := json.Unmarshal([]byte(rows[0].PayloadJSON), &payload); err != nil {
		t.Fatal(err)
	}
	recipient := payload["recipient"].(map[string]any)
	if recipient["id"] != alice.ID || recipient["name"] != "alice" {
		t.Fatalf("recipient = %#v", recipient)
	}
}

func TestReminderSchedulerResolvesProjectConfigEndpointAndSecrets(t *testing.T) {
	store := newTestStore(t)
	svc, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: 1000}})
	if err != nil {
		t.Fatal(err)
	}
	alice := createSchedulerAssignee(t, svc, store, "alice")
	project, err := svc.AddProject(AddProjectInput{Slug: "agentapi", Name: "Agent API"})
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}
	if err := svc.ConfigSchemaSet(ConfigSchemaInput{
		Key:           "integrations.notify.url",
		ValueType:     string(ConfigValueTypeString),
		AllowedScopes: []string{string(ConfigAllowedScopeWorkspace), string(ConfigAllowedScopeProject)},
	}); err != nil {
		t.Fatalf("ConfigSchemaSet(url) error = %v", err)
	}
	if err := svc.ConfigSchemaSet(ConfigSchemaInput{
		Key:           "integrations.notify.token",
		ValueType:     string(ConfigValueTypeString),
		AllowedScopes: []string{string(ConfigAllowedScopeWorkspace), string(ConfigAllowedScopeProject)},
		Secret:        true,
	}); err != nil {
		t.Fatalf("ConfigSchemaSet(token) error = %v", err)
	}
	if err := svc.SetConfig("integrations.notify.url", "https://example.com/workspace"); err != nil {
		t.Fatalf("SetConfig(url) error = %v", err)
	}
	if err := svc.SetConfig("integrations.notify.token", "workspace-token"); err != nil {
		t.Fatalf("SetConfig(token) error = %v", err)
	}
	if err := svc.ProjectConfigSet(project.ID, "integrations.notify.url", "https://example.com/project"); err != nil {
		t.Fatalf("ProjectConfigSet(url) error = %v", err)
	}
	if err := svc.ProjectConfigSet(project.ID, "integrations.notify.token", "project-token"); err != nil {
		t.Fatalf("ProjectConfigSet(token) error = %v", err)
	}
	sink, err := svc.AddNotificationSink(NotificationSinkAddInput{
		Name:            "third-party",
		Type:            NotificationSinkTypeHTTPTemplate,
		EndpointMode:    NotificationEndpointConfigValue,
		ConfigKey:       "integrations.notify.url",
		AllowedHosts:    []string{"example.com"},
		HeaderTemplates: []HTTPHeaderTemplateInput{{Name: "Authorization", Value: "Bearer {{secret.notify_token}}"}},
		BodyContentType: "application/json",
		BodyTemplate:    `{"text":"任务 {{task.task_slug}} 即将到期：{{task.description}}"}`,
		SecretRefs:      []HTTPTemplateSecretRefInput{{Alias: "notify_token", ConfigKey: "integrations.notify.token"}},
	})
	if err != nil {
		t.Fatalf("AddNotificationSink() error = %v", err)
	}
	due := int64(2000)
	if _, err := storage.NewTaskRepository(store.DB()).Create(task.Task{
		UUID:        uuid.NewString(),
		WorkspaceID: svc.Runtime().WorkspaceID,
		Description: "完成 OAuth",
		Status:      task.StatusPending,
		Entry:       1000,
		Modified:    1000,
		Due:         &due,
		Project:     &project.Slug,
		ProjectID:   &project.ID,
		ProjectSeq:  int64Ptr(1),
		Assignees:   []task.AssigneeInfo{{UserID: alice.ID, Name: alice.Name}},
	}); err != nil {
		t.Fatalf("Create task error = %v", err)
	}
	if _, err := svc.AddReminderRule(ReminderRuleAddInput{Name: "due-before", TriggerType: "due_before", OffsetSeconds: 1200, AudienceType: "assignees", SinkRef: sink.ID}); err != nil {
		t.Fatalf("AddReminderRule error = %v", err)
	}

	scheduler := NewReminderScheduler(ReminderSchedulerOptions{Store: store, Clock: FixedClock{NowUnix: 1000}})
	result, err := scheduler.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if result.DeliveriesEnqueued != 1 {
		t.Fatalf("DeliveriesEnqueued = %d, want 1", result.DeliveriesEnqueued)
	}
	rows, err := storage.NewNotificationDeliveryRepository(store.DB()).List(svc.Runtime().WorkspaceID, "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("delivery count = %d, want 1", len(rows))
	}
	if rows[0].ResolvedURL != "https://example.com/project" {
		t.Fatalf("ResolvedURL = %q, want project config value", rows[0].ResolvedURL)
	}
	if !strings.Contains(rows[0].RenderedHeadersJSON, "Bearer project-token") {
		t.Fatalf("RenderedHeadersJSON = %s, want project secret", rows[0].RenderedHeadersJSON)
	}
	if !strings.Contains(rows[0].RenderedBody, "agentapi-1") {
		t.Fatalf("RenderedBody = %s, want project task slug", rows[0].RenderedBody)
	}
}

func int64Ptr(v int64) *int64 { return &v }

var _ = task.StatusPending
