package app

import (
	"context"
	"testing"
)

func TestSuggestContentReferencesReturnsActiveMembers(t *testing.T) {
	svc, store := eventNotificationTestEnv(t)
	alice := createEventNotificationAssignee(t, svc, store, "alice-suggest")
	createEventNotificationAssignee(t, svc, store, "bob-suggest")

	results, err := svc.SuggestContentReferences(context.Background(), ContentReferenceSuggestionInput{
		Type: "user", Query: "alice", Limit: 10,
	})
	if err != nil {
		t.Fatalf("SuggestContentReferences: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].User == nil || results[0].User.ID != alice.ID {
		t.Fatalf("got = %#v", results[0])
	}
}

// TestSuggestContentReferencesEmptyQueryReturnsAllMembers 验证空 query 时返回
// workspace 全部 active member（按 limit 截断），让用户敲 @ 就看到候选列表。
func TestSuggestContentReferencesEmptyQueryReturnsAllMembers(t *testing.T) {
	svc, store := eventNotificationTestEnv(t)
	alice := createEventNotificationAssignee(t, svc, store, "alice-empty")
	bob := createEventNotificationAssignee(t, svc, store, "bob-empty")

	results, err := svc.SuggestContentReferences(context.Background(), ContentReferenceSuggestionInput{
		Type: "user", Query: "", Limit: 10,
	})
	if err != nil {
		t.Fatalf("SuggestContentReferences: %v", err)
	}
	ids := make(map[string]bool, len(results))
	for _, r := range results {
		if r.User != nil {
			ids[r.User.ID] = true
		}
	}
	if !ids[alice.ID] || !ids[bob.ID] {
		t.Fatalf("empty query should return all members, got ids=%v", ids)
	}
}

func TestSuggestContentReferencesReturnsTasks(t *testing.T) {
	svc, _ := eventNotificationTestEnv(t)
	proj, err := svc.AddProject(AddProjectInput{Name: "Suggest", Slug: "sgt"})
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	tsk, err := svc.Add(AddInput{Title: "Implement suggest API", Project: &proj.Slug})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	results, err := svc.SuggestContentReferences(context.Background(), ContentReferenceSuggestionInput{
		Type: "task", Query: "suggest", ProjectRef: proj.Slug, Limit: 10,
	})
	if err != nil {
		t.Fatalf("SuggestContentReferences: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Task == nil || results[0].Task.ID != tsk.UUID {
		t.Fatalf("got = %#v", results[0])
	}
	if results[0].Task.Title != "Implement suggest API" {
		t.Fatalf("title = %q", results[0].Task.Title)
	}
}

func TestSuggestContentReferencesRejectsInvalidType(t *testing.T) {
	svc, _ := eventNotificationTestEnv(t)
	_, err := svc.SuggestContentReferences(context.Background(), ContentReferenceSuggestionInput{
		Type: "project", Query: "x",
	})
	code, ok := IsRuntimeErrorCode(err)
	if !ok || code != "content_reference_query_invalid" {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveContentReferencesReturnsResolvedAndUnavailable(t *testing.T) {
	svc, store := eventNotificationTestEnv(t)
	alice := createEventNotificationAssignee(t, svc, store, "alice-resolve")
	proj, err := svc.AddProject(AddProjectInput{Name: "Resolve", Slug: "rsv"})
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	tsk, err := svc.Add(AddInput{Title: "Resolve task", Project: &proj.Slug})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	missingUserID := "00000000-0000-0000-0000-000000000099"
	missingTaskID := "00000000-0000-0000-0000-000000000098"

	results, err := svc.ResolveContentReferences(context.Background(), []ContentReferenceKeyInput{
		{Type: "user", ID: alice.ID},
		{Type: "user", ID: missingUserID},
		{Type: "task", ID: tsk.UUID},
		{Type: "task", ID: missingTaskID},
	})
	if err != nil {
		t.Fatalf("ResolveContentReferences: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("got %d results, want 4", len(results))
	}
	// alice 解析成功
	if results[0].Status != "resolved" || results[0].User == nil || results[0].User.ID != alice.ID {
		t.Fatalf("results[0] = %#v", results[0])
	}
	// missing user unavailable，且不暴露目标是否存在
	if results[1].Status != "unavailable" {
		t.Fatalf("results[1] = %#v", results[1])
	}
	// task 解析成功
	if results[2].Status != "resolved" || results[2].Task == nil || results[2].Task.ID != tsk.UUID {
		t.Fatalf("results[2] = %#v", results[2])
	}
	if results[2].Task.Title != "Resolve task" {
		t.Fatalf("title = %q", results[2].Task.Title)
	}
	// task_slug 在 project + seq 都存在时派生为 "rsv-1"。
	if results[2].Task.TaskSlug != "rsv-1" {
		t.Fatalf("task_slug = %q, want rsv-1", results[2].Task.TaskSlug)
	}
	// URL 在 resourceBaseURL 为空时也为空（spec §6.3），这里只断言不崩溃。
	// missing task unavailable
	if results[3].Status != "unavailable" {
		t.Fatalf("results[3] = %#v", results[3])
	}
}

func TestResolveContentReferencesTreatsMissingReadCapabilitiesAsUnavailable(t *testing.T) {
	svc, store := eventNotificationTestEnv(t)
	alice := createEventNotificationAssignee(t, svc, store, "alice-resolve-scope")
	project, err := svc.AddProject(AddProjectInput{Name: "Resolve scope", Slug: "rscope"})
	if err != nil {
		t.Fatal(err)
	}
	tsk, err := svc.Add(AddInput{Title: "Hidden from workspace-only token", Project: &project.Slug})
	if err != nil {
		t.Fatal(err)
	}
	token, err := svc.CreateTenantAccessToken(CreateTenantAccessTokenInput{
		Name: "workspace-only-resolve", Scopes: []string{"workspace:read"}, WorkspaceRef: svc.Runtime().WorkspaceSlug,
	})
	if err != nil {
		t.Fatal(err)
	}
	workspaceOnly := mustTenantServiceForTest(t, svc, token.RawToken, "workspace:read", PermissionWorkspaceRead, 1000)
	results, err := workspaceOnly.ResolveContentReferences(context.Background(), []ContentReferenceKeyInput{
		{Type: "user", ID: alice.ID},
		{Type: "task", ID: tsk.UUID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Status != "resolved" || results[0].User == nil || results[0].User.ID != alice.ID {
		t.Fatalf("user resolution = %#v, want resolved", results[0])
	}
	if results[1].Status != "unavailable" || results[1].Task != nil {
		t.Fatalf("task resolution leaked without task:read: %#v", results[1])
	}
}

func TestResolveContentReferencesRejectsTooMany(t *testing.T) {
	svc, _ := eventNotificationTestEnv(t)
	keys := make([]ContentReferenceKeyInput, 201)
	for i := range keys {
		keys[i] = ContentReferenceKeyInput{Type: "user", ID: "x"}
	}
	_, err := svc.ResolveContentReferences(context.Background(), keys)
	code, ok := IsRuntimeErrorCode(err)
	if !ok || code != "description_reference_limit_exceeded" {
		t.Fatalf("err = %v", err)
	}
}
