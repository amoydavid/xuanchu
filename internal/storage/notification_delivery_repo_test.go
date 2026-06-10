package storage

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func makeNotificationDelivery(wsID, sinkID, ruleID string, overrides ...func(*NotificationDelivery)) NotificationDelivery {
	row := NotificationDelivery{
		ID:                          uuid.NewString(),
		WorkspaceID:                 wsID,
		RuleID:                      ruleID,
		SinkID:                      sinkID,
		TaskUUID:                    uuid.NewString(),
		ObjectKind:                  "task",
		ObjectID:                    "",
		RecipientUserID:             "user-1",
		EventID:                     uuid.NewString(),
		EventType:                   "task.due_soon",
		DedupeKey:                   uuid.NewString(),
		ResolvedURL:                 "https://example.com/xuanchu/notifications",
		ResolvedEndpointSource:      "static_url",
		ResolvedEndpointFingerprint: "fp-1",
		RenderedMethod:              "POST",
		RenderedHeadersJSON:         `{"Content-Type":["application/json"]}`,
		RenderedBody:                `{"event_type":"task.due_soon"}`,
		RenderedContentType:         "application/json",
		PayloadJSON:                 `{"event_type":"task.due_soon"}`,
		Status:                      DeliveryStatusQueued,
		CreatedAt:                   100,
		ModifiedAt:                  100,
	}
	for _, fn := range overrides {
		fn(&row)
	}
	return row
}

func TestNotificationDeliveryRepositoryStoresEventObject(t *testing.T) {
	store, wsID := newNotificationTestStore(t)
	sink := makeNotificationSink(wsID, "openclaw")
	if err := NewNotificationSinkRepository(store.DB()).Create(sink); err != nil {
		t.Fatal(err)
	}
	ruleRepo := NewEventNotificationRuleRepository(store.DB())
	rule := makeEventNotificationRule(wsID, "project-note", sink.ID, "project.annotated", func(r *EventNotificationRule) {
		r.AudienceType = "actor"
	})
	if err := ruleRepo.Create(rule); err != nil {
		t.Fatal(err)
	}
	repo := NewNotificationDeliveryRepository(store.DB())

	delivery := makeNotificationDelivery(wsID, sink.ID, rule.ID, func(d *NotificationDelivery) {
		d.TaskUUID = ""
		d.ObjectKind = "project"
		d.ObjectID = "project-1"
		d.EventType = "project.annotated"
	})
	if err := repo.Enqueue([]NotificationDelivery{delivery}); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	got, err := repo.GetByID(delivery.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.TaskUUID != "" || got.ObjectKind != "project" || got.ObjectID != "project-1" {
		t.Fatalf("event object fields = task_uuid:%q object:%s/%s", got.TaskUUID, got.ObjectKind, got.ObjectID)
	}
}

func TestNotificationDeliveryRepositoryDedupeKeyUnique(t *testing.T) {
	store, wsID := newNotificationTestStore(t)
	sinkRepo := NewNotificationSinkRepository(store.DB())
	sink := makeNotificationSink(wsID, "openclaw")
	if err := sinkRepo.Create(sink); err != nil {
		t.Fatal(err)
	}
	ruleRepo := NewReminderRuleRepository(store.DB())
	rule := makeReminderRule(wsID, "due-before", sink.ID)
	if err := ruleRepo.Create(rule); err != nil {
		t.Fatal(err)
	}
	repo := NewNotificationDeliveryRepository(store.DB())

	delivery := makeNotificationDelivery(wsID, sink.ID, rule.ID, func(d *NotificationDelivery) {
		d.DedupeKey = "same-key"
	})
	if err := repo.Enqueue([]NotificationDelivery{delivery}); err != nil {
		t.Fatalf("first Enqueue() error = %v", err)
	}
	duplicate := makeNotificationDelivery(wsID, sink.ID, rule.ID, func(d *NotificationDelivery) {
		d.DedupeKey = "same-key"
	})
	if err := repo.Enqueue([]NotificationDelivery{duplicate}); err != nil {
		t.Fatalf("duplicate Enqueue() error = %v", err)
	}

	rows, err := repo.List(wsID, "", 10, 0)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("delivery count = %d, want 1", len(rows))
	}
}

func TestNotificationDeliveryRepositoryExistsByDedupeKey(t *testing.T) {
	store, wsID := newNotificationTestStore(t)
	sinkRepo := NewNotificationSinkRepository(store.DB())
	sink := makeNotificationSink(wsID, "openclaw")
	if err := sinkRepo.Create(sink); err != nil {
		t.Fatal(err)
	}
	ruleRepo := NewReminderRuleRepository(store.DB())
	rule := makeReminderRule(wsID, "due-before", sink.ID)
	if err := ruleRepo.Create(rule); err != nil {
		t.Fatal(err)
	}
	repo := NewNotificationDeliveryRepository(store.DB())

	delivery := makeNotificationDelivery(wsID, sink.ID, rule.ID, func(d *NotificationDelivery) {
		d.DedupeKey = "dedupe-key-1"
	})
	if err := repo.Enqueue([]NotificationDelivery{delivery}); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	exists, err := repo.ExistsByDedupeKey("dedupe-key-1")
	if err != nil {
		t.Fatalf("ExistsByDedupeKey(existing) error = %v", err)
	}
	if !exists {
		t.Fatal("ExistsByDedupeKey(existing) = false, want true")
	}
	exists, err = repo.ExistsByDedupeKey("missing-key")
	if err != nil {
		t.Fatalf("ExistsByDedupeKey(missing) error = %v", err)
	}
	if exists {
		t.Fatal("ExistsByDedupeKey(missing) = true, want false")
	}
}

func TestNotificationDeliveryRepositoryListFiltersSinkBeforePagination(t *testing.T) {
	store, wsID := newNotificationTestStore(t)
	sinkRepo := NewNotificationSinkRepository(store.DB())
	targetSink := makeNotificationSink(wsID, "target")
	otherSink := makeNotificationSink(wsID, "other")
	if err := sinkRepo.Create(targetSink); err != nil {
		t.Fatal(err)
	}
	if err := sinkRepo.Create(otherSink); err != nil {
		t.Fatal(err)
	}
	ruleRepo := NewReminderRuleRepository(store.DB())
	targetRule := makeReminderRule(wsID, "target-rule", targetSink.ID)
	otherRule := makeReminderRule(wsID, "other-rule", otherSink.ID)
	if err := ruleRepo.Create(targetRule); err != nil {
		t.Fatal(err)
	}
	if err := ruleRepo.Create(otherRule); err != nil {
		t.Fatal(err)
	}
	repo := NewNotificationDeliveryRepository(store.DB())

	targetOlder := makeNotificationDelivery(wsID, targetSink.ID, targetRule.ID, func(d *NotificationDelivery) {
		d.CreatedAt = 100
	})
	targetNewer := makeNotificationDelivery(wsID, targetSink.ID, targetRule.ID, func(d *NotificationDelivery) {
		d.CreatedAt = 200
	})
	otherNewest := makeNotificationDelivery(wsID, otherSink.ID, otherRule.ID, func(d *NotificationDelivery) {
		d.CreatedAt = 400
	})
	otherSecondNewest := makeNotificationDelivery(wsID, otherSink.ID, otherRule.ID, func(d *NotificationDelivery) {
		d.CreatedAt = 300
	})
	if err := repo.Enqueue([]NotificationDelivery{targetOlder, targetNewer, otherNewest, otherSecondNewest}); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	rows, err := repo.ListWithOptions(NotificationDeliveryListOptions{
		WorkspaceID: wsID,
		SinkID:      targetSink.ID,
		Limit:       2,
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("delivery count = %d, want 2; rows = %#v", len(rows), rows)
	}
	if rows[0].ID != targetNewer.ID || rows[1].ID != targetOlder.ID {
		t.Fatalf("List() ids = [%s %s], want [%s %s]", rows[0].ID, rows[1].ID, targetNewer.ID, targetOlder.ID)
	}
}

func TestNotificationDeliveryRepositoryClaimDue(t *testing.T) {
	store, wsID := newNotificationTestStore(t)
	sinkRepo := NewNotificationSinkRepository(store.DB())
	sink := makeNotificationSink(wsID, "openclaw")
	if err := sinkRepo.Create(sink); err != nil {
		t.Fatal(err)
	}
	ruleRepo := NewReminderRuleRepository(store.DB())
	rule := makeReminderRule(wsID, "due-before", sink.ID)
	if err := ruleRepo.Create(rule); err != nil {
		t.Fatal(err)
	}
	repo := NewNotificationDeliveryRepository(store.DB())

	due := makeNotificationDelivery(wsID, sink.ID, rule.ID)
	future := makeNotificationDelivery(wsID, sink.ID, rule.ID, func(d *NotificationDelivery) {
		d.Status = DeliveryStatusRetryWait
		d.NextAttemptAt = int64Ptr(500)
	})
	succeeded := makeNotificationDelivery(wsID, sink.ID, rule.ID, func(d *NotificationDelivery) {
		d.Status = DeliveryStatusSucceeded
	})
	if err := repo.Enqueue([]NotificationDelivery{due, future, succeeded}); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	claimed, err := repo.ClaimDue(200, 260, 10)
	if err != nil {
		t.Fatalf("ClaimDue() error = %v", err)
	}
	if len(claimed) != 1 || claimed[0].ID != due.ID {
		t.Fatalf("ClaimDue() = %#v", claimed)
	}
	if claimed[0].Status != DeliveryStatusDelivering || claimed[0].AttemptCount != 1 {
		t.Fatalf("claimed row status/attempt = %s/%d", claimed[0].Status, claimed[0].AttemptCount)
	}
}

func TestNotificationDeliveryClaimDueSQLAvoidsPostgresUntypedMax(t *testing.T) {
	sql := notificationClaimExpiresAtSQL()
	if strings.Contains(sql, "MAX(?") {
		t.Fatalf("claim expiry SQL contains untyped MAX parameter: %s", sql)
	}
	if !strings.Contains(sql, "CAST(? AS BIGINT)") || !strings.Contains(sql, "CASE") {
		t.Fatalf("claim expiry SQL should cast bind params and use CASE: %s", sql)
	}
}

func TestNotificationDeliveryRepositoryReplayKeepsResolvedRequestSnapshot(t *testing.T) {
	store, wsID := newNotificationTestStore(t)
	sinkRepo := NewNotificationSinkRepository(store.DB())
	sink := makeNotificationSink(wsID, "openclaw")
	if err := sinkRepo.Create(sink); err != nil {
		t.Fatal(err)
	}
	ruleRepo := NewReminderRuleRepository(store.DB())
	rule := makeReminderRule(wsID, "due-before", sink.ID)
	if err := ruleRepo.Create(rule); err != nil {
		t.Fatal(err)
	}
	repo := NewNotificationDeliveryRepository(store.DB())

	delivery := makeNotificationDelivery(wsID, sink.ID, rule.ID, func(d *NotificationDelivery) {
		d.Status = DeliveryStatusDeadLettered
		d.ResolvedURL = "https://old.example.com/notify"
		d.RenderedMethod = "POST"
		d.RenderedHeadersJSON = `{"Authorization":["Bearer frozen"]}`
		d.RenderedBody = `{"frozen":true}`
		d.RenderedContentType = "application/json"
	})
	if err := repo.Enqueue([]NotificationDelivery{delivery}); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	if err := repo.Requeue(delivery.ID, 300); err != nil {
		t.Fatalf("Requeue() error = %v", err)
	}
	got, err := repo.GetByID(delivery.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.ResolvedURL != delivery.ResolvedURL ||
		got.RenderedMethod != delivery.RenderedMethod ||
		got.RenderedHeadersJSON != delivery.RenderedHeadersJSON ||
		got.RenderedBody != delivery.RenderedBody ||
		got.RenderedContentType != delivery.RenderedContentType {
		t.Fatalf("request snapshot changed after Requeue(): %#v", got)
	}
	if got.Status != DeliveryStatusQueued {
		t.Fatalf("Status = %q, want queued", got.Status)
	}
}

func TestNotificationDeliveryReleaseClaimRevertsClaimAttempt(t *testing.T) {
	store, wsID := newNotificationTestStore(t)
	sinkRepo := NewNotificationSinkRepository(store.DB())
	sink := makeNotificationSink(wsID, "openclaw")
	if err := sinkRepo.Create(sink); err != nil {
		t.Fatal(err)
	}
	ruleRepo := NewReminderRuleRepository(store.DB())
	rule := makeReminderRule(wsID, "due-before", sink.ID)
	if err := ruleRepo.Create(rule); err != nil {
		t.Fatal(err)
	}
	repo := NewNotificationDeliveryRepository(store.DB())
	delivery := makeNotificationDelivery(wsID, sink.ID, rule.ID)
	if err := repo.Enqueue([]NotificationDelivery{delivery}); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	claimed, err := repo.ClaimDue(200, 500, 1)
	if err != nil {
		t.Fatalf("ClaimDue() error = %v", err)
	}
	if len(claimed) != 1 || claimed[0].AttemptCount != 1 {
		t.Fatalf("claimed = %#v, want one claimed attempt", claimed)
	}
	if err := repo.ReleaseClaim(delivery.ID, 250); err != nil {
		t.Fatalf("ReleaseClaim() error = %v", err)
	}
	got, err := repo.GetByID(delivery.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Status != DeliveryStatusQueued {
		t.Fatalf("Status = %q, want queued", got.Status)
	}
	if got.ClaimExpiresAt != nil {
		t.Fatalf("claim_expires_at = %#v, want nil", got.ClaimExpiresAt)
	}
	if got.AttemptCount != 0 {
		t.Fatalf("attempt_count = %d, want reverted to 0", got.AttemptCount)
	}
	if got.ModifiedAt != 250 {
		t.Fatalf("modified_at = %d, want 250", got.ModifiedAt)
	}
}
