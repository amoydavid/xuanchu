package app

import (
	"context"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

// TestUserMentionEventGeneratesDelivery 验证用户 mention 事件产生 notification delivery。
func TestUserMentionEventGeneratesDelivery(t *testing.T) {
	svc, store := eventNotificationTestEnv(t)
	alice := createEventNotificationAssignee(t, svc, store, "alice-mention")
	sink, _ := svc.AddNotificationSink(defaultNotificationSinkInput())
	rule, err := svc.AddEventNotificationRule(EventNotificationRuleAddInput{
		Name:         "mention",
		EventType:    "task.user_mentioned",
		AudienceType: "mentioned_users",
		SinkRef:      sink.ID,
	})
	if err != nil {
		t.Fatalf("AddEventNotificationRule: %v", err)
	}

	tsk, err := svc.Add(AddInput{Title: "mention-task"})
	if err != nil {
		t.Fatal(err)
	}
	desc := "[@Alice](ref://user/" + alice.ID + ")"
	before := tsk
	after := tsk
	after.Description = &desc

	event, ok := svc.buildUserMentionedEventIfNeeded(before, after, svc.clock.Unix())
	if !ok {
		t.Fatal("expected mention event to be generated")
	}
	if err := svc.enqueueEventNotificationDeliveries([]HookEvent{event}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	rows, _ := storage.NewNotificationDeliveryRepository(store.DB()).List(svc.workspaceID, "", 100, 0)
	// 过滤到当前 rule
	var mine []storage.NotificationDelivery
	for _, r := range rows {
		if r.RuleID == rule.ID {
			mine = append(mine, r)
		}
	}
	if len(mine) != 1 {
		t.Fatalf("deliveries = %d, want 1", len(mine))
	}
	if mine[0].RecipientUserID != alice.ID {
		t.Fatalf("recipient = %s, want %s", mine[0].RecipientUserID, alice.ID)
	}
}

// TestUserMentionEventSkipsOnNoNewMentions 验证 label 变化不重复触发。
func TestUserMentionEventSkipsOnNoNewMentions(t *testing.T) {
	svc, _ := eventNotificationTestEnv(t)
	tsk, _ := svc.Add(AddInput{Title: "no-dup"})
	desc := "[@Alice](ref://user/00000000-0000-0000-0000-000000000001)"
	tsk.Description = &desc
	// before 和 after 都含同一 mention → no added → no event
	event, ok := svc.buildUserMentionedEventIfNeeded(tsk, tsk, svc.clock.Unix())
	if ok {
		t.Fatalf("expected no event, got %#v", event)
	}
}

// TestMentionedUsersAudienceRejectsOtherEventTypes 验证 mentioned_users 只支持 task.user_mentioned。
func TestMentionedUsersAudienceRejectsOtherEventTypes(t *testing.T) {
	svc, _ := eventNotificationTestEnv(t)
	sink, _ := svc.AddNotificationSink(defaultNotificationSinkInput())
	_, err := svc.AddEventNotificationRule(EventNotificationRuleAddInput{
		Name: "bad", EventType: "task.modified", AudienceType: "mentioned_users", SinkRef: sink.ID,
	})
	if err == nil {
		t.Fatal("expected audience_unsupported_for_event error")
	}
	code, ok := IsRuntimeErrorCode(err)
	if !ok || code != "audience_unsupported_for_event" {
		t.Fatalf("err = %v, want code audience_unsupported_for_event", err)
	}
}

// 占位：context 在后续扩展中会使用。
var _ = context.Background
