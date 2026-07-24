package app

import (
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

func TestTaskActivityMapsAndMergesSemanticEvents(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	tsk, err := svc.Add(AddInput{Title: "before"})
	if err != nil {
		t.Fatal(err)
	}
	svc.clock = FixedClock{NowUnix: 110}
	if err := svc.Start(tsk.UUID); err != nil {
		t.Fatal(err)
	}
	svc.clock = FixedClock{NowUnix: 120}
	title := "after"
	if err := svc.Modify(tsk.UUID, ModifyInput{Title: &title}); err != nil {
		t.Fatal(err)
	}
	svc.clock = FixedClock{NowUnix: 130}
	if err := svc.Annotate(tsk.UUID, "timeline note"); err != nil {
		t.Fatal(err)
	}
	svc.clock = FixedClock{NowUnix: 140}
	if err := svc.Done(tsk.UUID); err != nil {
		t.Fatal(err)
	}
	svc.clock = FixedClock{NowUnix: 150}
	if err := svc.Reopen(tsk.UUID); err != nil {
		t.Fatal(err)
	}

	page, err := svc.ListTaskActivity(tsk.UUID, TaskActivityInput{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	wantActions := []string{"reopened", "completed", "commented", "fields_changed", "started", "created"}
	if len(page.Entries) != len(wantActions) {
		t.Fatalf("entries = %#v", page.Entries)
	}
	for i, want := range wantActions {
		if page.Entries[i].Action != want {
			t.Fatalf("entry[%d].Action = %q, want %q", i, page.Entries[i].Action, want)
		}
	}
	if page.Entries[2].Annotation == nil || page.Entries[2].Annotation.Description != "timeline note" {
		t.Fatalf("annotation entry = %#v", page.Entries[2])
	}
	if page.Entries[3].Kind != "change" || len(page.Entries[3].Changes) != 1 {
		t.Fatalf("change entry = %#v", page.Entries[3])
	}
	for _, entry := range page.Entries {
		if entry.Actor.Type == "user" && (entry.Actor.User == nil || entry.Actor.User.ID == "") {
			t.Fatalf("incomplete user actor = %#v", entry.Actor)
		}
	}
}

func TestTaskActivityUsesSnapshotCreatedWithoutInventingCompletion(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	end := int64(99)
	_, err := svc.repo.Create(task.Task{
		UUID: "legacy-task", WorkspaceID: svc.workspaceID, Title: "legacy",
		Status: task.StatusCompleted, Entry: 50, Modified: 99, End: &end,
	})
	if err != nil {
		t.Fatal(err)
	}
	page, err := svc.ListTaskActivity("legacy-task", TaskActivityInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Entries) != 1 || page.Entries[0].ID != "snapshot:created" || page.Entries[0].Action != "created" {
		t.Fatalf("entries = %#v", page.Entries)
	}
	if page.Entries[0].Actor.Type != "unknown" {
		t.Fatalf("snapshot actor = %#v", page.Entries[0].Actor)
	}
}

func TestTaskActivityCursorHasNoDuplicates(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	tsk, err := svc.Add(AddInput{Title: "cursor"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(tsk.UUID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Stop(tsk.UUID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Annotate(tsk.UUID, "same second"); err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}
	cursor := ""
	pageNumber := 0
	insertedID := ""
	for {
		page, err := svc.ListTaskActivity(tsk.UUID, TaskActivityInput{Limit: 2, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range page.Entries {
			if seen[entry.ID] {
				t.Fatalf("duplicate entry %q", entry.ID)
			}
			seen[entry.ID] = true
		}
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
		if pageNumber == 0 {
			svc.clock = FixedClock{NowUnix: 200}
			if err := svc.Annotate(tsk.UUID, "inserted after first page"); err != nil {
				t.Fatal(err)
			}
			annotations, _, err := svc.ListAnnotations(tsk.UUID, 0, 10)
			if err != nil {
				t.Fatal(err)
			}
			for _, annotation := range annotations {
				if annotation.Description == "inserted after first page" {
					insertedID = "annotation:" + annotation.ID
				}
			}
		}
		pageNumber++
	}
	if len(seen) != 4 {
		t.Fatalf("seen = %#v, want the original four activities", seen)
	}
	if insertedID == "" {
		t.Fatal("inserted annotation ID not found")
	}
	if seen[insertedID] {
		t.Fatalf("newer event crossed the existing cursor: %q", insertedID)
	}
	if _, err := svc.ListTaskActivity(tsk.UUID, TaskActivityInput{Cursor: "not-a-cursor"}); err == nil || runtimeErrorCode(err) != "api_bad_cursor" {
		t.Fatalf("bad cursor error = %v", err)
	}
}

func TestTaskActivityLinkEventsKeepStructuredSnapshots(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	tsk, err := svc.Add(AddInput{Title: "links"})
	if err != nil {
		t.Fatal(err)
	}
	svc.clock = FixedClock{NowUnix: 110}
	link, err := svc.TaskAddLink(tsk.UUID, "document", "https://example.com/one", "One")
	if err != nil {
		t.Fatal(err)
	}
	svc.clock = FixedClock{NowUnix: 120}
	if _, err := svc.TaskUpdateLink(tsk.UUID, link.ID, "pr", "https://example.com/two", "Two"); err != nil {
		t.Fatal(err)
	}
	svc.clock = FixedClock{NowUnix: 130}
	if err := svc.TaskRemoveLink(tsk.UUID, link.ID); err != nil {
		t.Fatal(err)
	}

	page, err := svc.ListTaskActivity(tsk.UUID, TaskActivityInput{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		action, linkType, url, title string
	}{
		{"link_removed", "pr", "https://example.com/two", "Two"},
		{"link_updated", "pr", "https://example.com/two", "Two"},
		{"link_added", "document", "https://example.com/one", "One"},
	}
	for index, expected := range want {
		entry := page.Entries[index]
		if entry.Action != expected.action || entry.Link == nil || entry.Link.ID != link.ID || entry.Link.Type != expected.linkType || entry.Link.URL != expected.url || entry.Link.Title != expected.title {
			t.Fatalf("entry[%d] = %#v, want %#v", index, entry, expected)
		}
	}
}

func TestResolveTaskActivityActorsPropagatesUserLookupError(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	userID := svc.runtime.ActorUserID
	candidates := []taskActivityCandidate{{
		Actor: actorColumns{Type: actorTypeUser, UserID: &userID},
	}}
	closeFn()

	if err := resolveTaskActivityActors(candidates, svc); err == nil {
		t.Fatal("resolveTaskActivityActors() error = nil, want user lookup error")
	}
}

func TestTaskActivitySuppressesCreatedForGeneratedOccurrence(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	tsk, err := svc.Add(AddInput{Title: "generated occurrence"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.auditRepo.Append(storage.AuditLogEntry{
		ActorType: "system", WorkspaceID: &svc.workspaceID,
		Action: "task.recurrence.generated", TargetType: "task", TargetID: tsk.UUID,
		PayloadJSON: `{}`, CreatedAt: 110,
	}); err != nil {
		t.Fatal(err)
	}

	page, err := svc.ListTaskActivity(tsk.UUID, TaskActivityInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Entries) != 1 || page.Entries[0].Action != "generated" {
		t.Fatalf("entries = %#v, want one generated event", page.Entries)
	}
	if page.Entries[0].Actor.Type != "system" {
		t.Fatalf("actor = %#v, want system", page.Entries[0].Actor)
	}
}

func TestTaskActivityResolvesTokenSystemAndUnknownActors(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	tsk, err := svc.Add(AddInput{Title: "actors"})
	if err != nil {
		t.Fatal(err)
	}
	tokenID, tokenName, tokenPrefix := "token-1", "automation", "xct_abc"
	rows := []storage.AuditLogEntry{
		{
			ActorType: auth.TokenTypeTenantAccess, ActorTokenID: &tokenID,
			ActorTokenName: &tokenName, ActorTokenPrefix: &tokenPrefix,
			WorkspaceID: &svc.workspaceID, Action: "task.start", TargetType: "task",
			TargetID: tsk.UUID, PayloadJSON: `{}`, CreatedAt: 130,
		},
		{
			ActorType: "system", WorkspaceID: &svc.workspaceID, Action: "task.stop",
			TargetType: "task", TargetID: tsk.UUID, PayloadJSON: `{}`, CreatedAt: 120,
		},
		{
			WorkspaceID: &svc.workspaceID, Action: "task.reopen", TargetType: "task",
			TargetID: tsk.UUID, PayloadJSON: `{}`, CreatedAt: 110,
		},
	}
	for _, row := range rows {
		if err := svc.auditRepo.Append(row); err != nil {
			t.Fatal(err)
		}
	}

	page, err := svc.ListTaskActivity(tsk.UUID, TaskActivityInput{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Entries) < 3 {
		t.Fatalf("entries = %#v", page.Entries)
	}
	if actor := page.Entries[0].Actor; actor.Type != auth.TokenTypeTenantAccess || actor.Token == nil || actor.Token.ID != tokenID || actor.Token.Name != tokenName || actor.Token.Prefix != tokenPrefix {
		t.Fatalf("token actor = %#v", actor)
	}
	if actor := page.Entries[1].Actor; actor.Type != "system" || actor.User != nil || actor.Token != nil {
		t.Fatalf("system actor = %#v", actor)
	}
	if actor := page.Entries[2].Actor; actor.Type != "unknown" || actor.User != nil || actor.Token != nil {
		t.Fatalf("unknown actor = %#v", actor)
	}
}

func TestTaskActivityDoesNotLeakDeletedAnnotation(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	tsk, err := svc.Add(AddInput{Title: "deleted annotation"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Annotate(tsk.UUID, "secret deleted body"); err != nil {
		t.Fatal(err)
	}
	annotations, _, err := svc.ListAnnotations(tsk.UUID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(annotations) != 1 {
		t.Fatalf("annotations = %#v", annotations)
	}
	if err := svc.Denotate(tsk.UUID, annotations[0].ID); err != nil {
		t.Fatal(err)
	}

	page, err := svc.ListTaskActivity(tsk.UUID, TaskActivityInput{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range page.Entries {
		if entry.Annotation != nil || entry.Action == "commented" {
			t.Fatalf("deleted annotation leaked in %#v", entry)
		}
	}
}
