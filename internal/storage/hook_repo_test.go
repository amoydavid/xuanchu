package storage

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

// --- helpers ---

func newHookTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	return store, ws.ID
}

func makeHookDef(t *testing.T, wsID, name, eventsJSON string, overrides ...func(*HookDefinition)) HookDefinition {
	t.Helper()
	h := HookDefinition{
		ID:             uuid.NewString(),
		Name:           name,
		ScopeType:      "workspace",
		WorkspaceID:    wsID,
		ActorUserID:    "user-1",
		EventTypesJSON: eventsJSON,
		EndpointURL:    "https://example.com/hook",
		Secret:         "s3cret",
		Enabled:        boolPtr(true),
		TimeoutSeconds: 10,
		MaxAttempts:    5,
		CreatedAt:      100,
		ModifiedAt:     100,
	}
	for _, fn := range overrides {
		fn(&h)
	}
	return h
}

func makeDelivery(t *testing.T, hookID, wsID string, overrides ...func(*HookDelivery)) HookDelivery {
	t.Helper()
	d := HookDelivery{
		ID:          uuid.NewString(),
		HookID:      hookID,
		EventID:     uuid.NewString(),
		EventType:   "task.created",
		WorkspaceID: wsID,
		ActorUserID: "user-1",
		PayloadJSON: `{"test":true}`,
		HeadersJSON: `{}`,
		Status:      DeliveryStatusQueued,
		CreatedAt:   200,
		ModifiedAt:  200,
	}
	for _, fn := range overrides {
		fn(&d)
	}
	return d
}

func int64Ptr(v int64) *int64 { return &v }
func strPtr(v string) *string { return &v }
func intPtr(v int) *int       { return &v }
func boolPtr(v bool) *bool    { return &v }

// --- TestHookTablesMigrated ---

func TestHookTablesMigrated(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	for _, table := range []any{&HookDefinition{}, &HookDelivery{}} {
		if !store.DB().Migrator().HasTable(table) {
			t.Fatalf("missing table for %T", table)
		}
	}

	// spot-check key columns
	hookDefCols := []string{
		"id", "name", "scope_type", "workspace_id", "project_id",
		"actor_user_id", "event_types_json", "endpoint_url", "secret",
		"enabled", "timeout_seconds", "max_attempts", "created_at", "modified_at",
	}
	for _, col := range hookDefCols {
		if !store.DB().Migrator().HasColumn(&HookDefinition{}, col) {
			t.Fatalf("hook_definitions missing column %q", col)
		}
	}

	hookDelCols := []string{
		"id", "hook_id", "event_id", "event_type", "workspace_id",
		"project_id", "actor_user_id", "payload_json", "headers_json",
		"status", "attempt_count", "next_attempt_at", "claim_expires_at",
		"last_attempt_at", "last_status_code", "last_error",
		"created_at", "modified_at",
	}
	for _, col := range hookDelCols {
		if !store.DB().Migrator().HasColumn(&HookDelivery{}, col) {
			t.Fatalf("hook_deliveries missing column %q", col)
		}
	}
}

// --- TestHookRepository ---

func TestHookRepositoryCreateAndList(t *testing.T) {
	store, wsID := newHookTestStore(t)
	repo := NewHookRepository(store.DB())

	h1 := makeHookDef(t, wsID, "hook-a", `["task.created"]`)
	h2 := makeHookDef(t, wsID, "hook-b", `["task.completed"]`)
	h2.CreatedAt = 200
	h2.ModifiedAt = 200

	if err := repo.Create(h1); err != nil {
		t.Fatalf("Create h1 error = %v", err)
	}
	if err := repo.Create(h2); err != nil {
		t.Fatalf("Create h2 error = %v", err)
	}

	rows, err := repo.List(wsID, nil, false)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("List() count = %d, want 2", len(rows))
	}
	if rows[0].Name != "hook-a" || rows[1].Name != "hook-b" {
		t.Fatalf("order = %q, %q; want hook-a, hook-b", rows[0].Name, rows[1].Name)
	}
}

func TestHookRepositoryListFiltersByProject(t *testing.T) {
	store, wsID := newHookTestStore(t)
	repo := NewHookRepository(store.DB())

	projectID := "proj-1"
	h1 := makeHookDef(t, wsID, "ws-hook", `["task.created"]`)
	h2 := makeHookDef(t, wsID, "proj-hook", `["task.created"]`, func(h *HookDefinition) {
		h.ProjectID = &projectID
	})

	if err := repo.Create(h1); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(h2); err != nil {
		t.Fatal(err)
	}

	// filter by project
	rows, err := repo.List(wsID, &projectID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Name != "proj-hook" {
		t.Fatalf("project-scoped list = %#v", rows)
	}

	// no project filter returns both
	all, err := repo.List(wsID, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("all hooks count = %d, want 2", len(all))
	}
}

func TestHookRepositoryListExcludesDisabled(t *testing.T) {
	store, wsID := newHookTestStore(t)
	repo := NewHookRepository(store.DB())

	h1 := makeHookDef(t, wsID, "enabled", `["task.created"]`)
	h2 := makeHookDef(t, wsID, "disabled", `["task.created"]`, func(h *HookDefinition) {
		h.Enabled = boolPtr(false)
	})

	if err := repo.Create(h1); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(h2); err != nil {
		t.Fatal(err)
	}

	rows, err := repo.List(wsID, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Name != "enabled" {
		t.Fatalf("enabled-only list = %#v", rows)
	}

	all, err := repo.List(wsID, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("include-disabled count = %d, want 2", len(all))
	}
}

func TestHookRepositoryUpdatePreservesFields(t *testing.T) {
	store, wsID := newHookTestStore(t)
	repo := NewHookRepository(store.DB())

	h := makeHookDef(t, wsID, "my-hook", `["task.created"]`)
	if err := repo.Create(h); err != nil {
		t.Fatal(err)
	}

	h.EndpointURL = "https://example.com/new-hook"
	h.ModifiedAt = 300
	if err := repo.Update(h); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got, err := repo.GetByID(h.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.EndpointURL != "https://example.com/new-hook" {
		t.Fatalf("endpoint = %q", got.EndpointURL)
	}
	if got.Secret != "s3cret" {
		t.Fatalf("secret changed unexpectedly = %q", got.Secret)
	}
	if got.ModifiedAt != 300 {
		t.Fatalf("modified_at = %d, want 300", got.ModifiedAt)
	}
}

func TestHookRepositoryDeleteMakesGetByIDNotFound(t *testing.T) {
	store, wsID := newHookTestStore(t)
	repo := NewHookRepository(store.DB())

	h := makeHookDef(t, wsID, "to-delete", `["task.created"]`)
	if err := repo.Create(h); err != nil {
		t.Fatal(err)
	}

	if err := repo.Delete(h.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err := repo.GetByID(h.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByID after delete error = %v, want ErrNotFound", err)
	}
}

func TestHookRepositoryListMatchingFiltersByEventType(t *testing.T) {
	store, wsID := newHookTestStore(t)
	repo := NewHookRepository(store.DB())

	h1 := makeHookDef(t, wsID, "create-hook", `["task.created","task.modified"]`)
	h2 := makeHookDef(t, wsID, "complete-hook", `["task.completed"]`)
	h3 := makeHookDef(t, wsID, "disabled-hook", `["task.created"]`, func(h *HookDefinition) {
		h.Enabled = boolPtr(false)
	})

	if err := repo.Create(h1); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(h2); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(h3); err != nil {
		t.Fatal(err)
	}

	matched, err := repo.ListMatching(wsID, nil, "task.created")
	if err != nil {
		t.Fatal(err)
	}
	if len(matched) != 1 || matched[0].Name != "create-hook" {
		t.Fatalf("ListMatching(task.created) = %#v", matched)
	}

	matchedCompleted, err := repo.ListMatching(wsID, nil, "task.completed")
	if err != nil {
		t.Fatal(err)
	}
	if len(matchedCompleted) != 1 || matchedCompleted[0].Name != "complete-hook" {
		t.Fatalf("ListMatching(task.completed) = %#v", matchedCompleted)
	}

	noMatch, err := repo.ListMatching(wsID, nil, "task.deleted")
	if err != nil {
		t.Fatal(err)
	}
	if len(noMatch) != 0 {
		t.Fatalf("ListMatching(task.deleted) = %#v, want empty", noMatch)
	}
}

// --- TestHookDeliveryRepository ---

func TestHookDeliveryEnqueueCreatesQueued(t *testing.T) {
	store, wsID := newHookTestStore(t)
	repo := NewHookDeliveryRepository(store.DB())
	hookID := "hook-1"

	deliveries := []HookDelivery{
		makeDelivery(t, hookID, wsID),
		makeDelivery(t, hookID, wsID),
	}
	if err := repo.Enqueue(deliveries); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	for _, d := range deliveries {
		got, err := repo.GetByID(d.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != DeliveryStatusQueued {
			t.Fatalf("status = %q, want queued", got.Status)
		}
		if got.AttemptCount != 0 {
			t.Fatalf("attempt_count = %d, want 0", got.AttemptCount)
		}
	}
}

func TestHookDeliveryEnqueueWritesScopeSnapshots(t *testing.T) {
	store, wsID := newHookTestStore(t)
	repo := NewHookDeliveryRepository(store.DB())
	projectID := "proj-1"

	d := makeDelivery(t, "hook-1", wsID, func(d *HookDelivery) {
		d.ProjectID = &projectID
		d.ActorUserID = "user-42"
		d.EventType = "task.completed"
	})
	if err := repo.Enqueue([]HookDelivery{d}); err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetByID(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.WorkspaceID != wsID {
		t.Fatalf("workspace_id = %q, want %q", got.WorkspaceID, wsID)
	}
	if got.ProjectID == nil || *got.ProjectID != projectID {
		t.Fatalf("project_id = %#v, want %q", got.ProjectID, projectID)
	}
	if got.ActorUserID != "user-42" {
		t.Fatalf("actor_user_id = %q", got.ActorUserID)
	}
	if got.EventType != "task.completed" {
		t.Fatalf("event_type = %q", got.EventType)
	}
}

func TestHookDeliveryClaimDueOnlyClaimsEligible(t *testing.T) {
	store, wsID := newHookTestStore(t)
	repo := NewHookDeliveryRepository(store.DB())
	hookID := "hook-1"

	// queued, no next_attempt_at -> eligible
	d1 := makeDelivery(t, hookID, wsID)

	// retry_wait, next_attempt_at in past -> eligible
	d2 := makeDelivery(t, hookID, wsID, func(d *HookDelivery) {
		d.Status = DeliveryStatusRetryWait
		d.NextAttemptAt = int64Ptr(50)
	})

	// queued, next_attempt_at in future -> not eligible
	d3 := makeDelivery(t, hookID, wsID, func(d *HookDelivery) {
		d.NextAttemptAt = int64Ptr(9999)
	})

	// already delivering -> not eligible
	d4 := makeDelivery(t, hookID, wsID, func(d *HookDelivery) {
		d.Status = DeliveryStatusDelivering
	})

	if err := repo.Enqueue([]HookDelivery{d1, d2, d3, d4}); err != nil {
		t.Fatal(err)
	}

	claimed, err := repo.ClaimDue(100, 200, 10)
	if err != nil {
		t.Fatalf("ClaimDue() error = %v", err)
	}
	if len(claimed) != 2 {
		t.Fatalf("claimed count = %d, want 2", len(claimed))
	}

	claimedIDs := map[string]bool{}
	for _, c := range claimed {
		claimedIDs[c.ID] = true
		if c.Status != DeliveryStatusDelivering {
			t.Fatalf("claimed status = %q, want delivering", c.Status)
		}
		if c.ClaimExpiresAt == nil || *c.ClaimExpiresAt != 200 {
			t.Fatalf("claim_expires_at = %#v, want 200", c.ClaimExpiresAt)
		}
		if c.AttemptCount != 1 {
			t.Fatalf("attempt_count = %d, want 1", c.AttemptCount)
		}
	}
	if !claimedIDs[d1.ID] || !claimedIDs[d2.ID] {
		t.Fatalf("expected d1 and d2 claimed, got IDs = %v", claimedIDs)
	}
	if claimedIDs[d3.ID] || claimedIDs[d4.ID] {
		t.Fatal("d3 or d4 should not be claimed")
	}

	claimedAgain, err := repo.ClaimDue(100, 300, 10)
	if err != nil {
		t.Fatalf("ClaimDue() second call error = %v", err)
	}
	if len(claimedAgain) != 0 {
		t.Fatalf("second claim got %d rows, want 0", len(claimedAgain))
	}
}

func TestHookDeliveryClaimDueUsesHookTimeoutForClaimExpiry(t *testing.T) {
	store, wsID := newHookTestStore(t)
	hookRepo := NewHookRepository(store.DB())
	repo := NewHookDeliveryRepository(store.DB())
	hook := makeHookDef(t, wsID, "slow-hook", `["task.created"]`, func(h *HookDefinition) {
		h.TimeoutSeconds = 120
	})
	if err := hookRepo.Create(hook); err != nil {
		t.Fatal(err)
	}
	delivery := makeDelivery(t, hook.ID, wsID)
	if err := repo.Enqueue([]HookDelivery{delivery}); err != nil {
		t.Fatal(err)
	}

	claimed, err := repo.ClaimDue(1000, 1030, 10)
	if err != nil {
		t.Fatalf("ClaimDue() error = %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed count = %d, want 1", len(claimed))
	}
	if claimed[0].ClaimExpiresAt == nil || *claimed[0].ClaimExpiresAt != 1180 {
		t.Fatalf("claim_expires_at = %v, want 1180", claimed[0].ClaimExpiresAt)
	}
}

func TestHookDeliveryRecoverStaleDelivering(t *testing.T) {
	store, wsID := newHookTestStore(t)
	repo := NewHookDeliveryRepository(store.DB())
	hookID := "hook-1"

	d := makeDelivery(t, hookID, wsID, func(del *HookDelivery) {
		del.Status = DeliveryStatusDelivering
		del.ClaimExpiresAt = int64Ptr(50) // expired
	})
	if err := repo.Enqueue([]HookDelivery{d}); err != nil {
		t.Fatal(err)
	}

	affected, err := repo.RecoverStaleDelivering(100)
	if err != nil {
		t.Fatalf("RecoverStaleDelivering() error = %v", err)
	}
	if affected != 1 {
		t.Fatalf("affected = %d, want 1", affected)
	}

	got, err := repo.GetByID(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != DeliveryStatusQueued {
		t.Fatalf("status = %q, want queued", got.Status)
	}
	if got.ClaimExpiresAt != nil {
		t.Fatalf("claim_expires_at = %#v, want nil", got.ClaimExpiresAt)
	}
}

func TestHookDeliveryMarkSucceeded(t *testing.T) {
	store, wsID := newHookTestStore(t)
	repo := NewHookDeliveryRepository(store.DB())
	hookID := "hook-1"

	d := makeDelivery(t, hookID, wsID, func(del *HookDelivery) {
		del.Status = DeliveryStatusDelivering
		del.ClaimExpiresAt = int64Ptr(200)
		del.AttemptCount = 1
	})
	if err := repo.Enqueue([]HookDelivery{d}); err != nil {
		t.Fatal(err)
	}

	if err := repo.MarkSucceeded(d.ID, 300, 200); err != nil {
		t.Fatalf("MarkSucceeded() error = %v", err)
	}

	got, err := repo.GetByID(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != DeliveryStatusSucceeded {
		t.Fatalf("status = %q, want succeeded", got.Status)
	}
	if got.LastStatusCode == nil || *got.LastStatusCode != 200 {
		t.Fatalf("last_status_code = %#v, want 200", got.LastStatusCode)
	}
	if got.LastAttemptAt == nil || *got.LastAttemptAt != 300 {
		t.Fatalf("last_attempt_at = %#v, want 300", got.LastAttemptAt)
	}
	if got.LastError != "" {
		t.Fatalf("last_error = %q, want empty", got.LastError)
	}
	if got.ClaimExpiresAt != nil {
		t.Fatalf("claim_expires_at = %#v, want nil", got.ClaimExpiresAt)
	}
}

func TestHookDeliveryMarkRetry(t *testing.T) {
	store, wsID := newHookTestStore(t)
	repo := NewHookDeliveryRepository(store.DB())
	hookID := "hook-1"

	d := makeDelivery(t, hookID, wsID, func(del *HookDelivery) {
		del.Status = DeliveryStatusDelivering
		del.ClaimExpiresAt = int64Ptr(200)
		del.AttemptCount = 1
	})
	if err := repo.Enqueue([]HookDelivery{d}); err != nil {
		t.Fatal(err)
	}

	statusCode := 500
	if err := repo.MarkRetry(d.ID, 300, 400, &statusCode, "internal error"); err != nil {
		t.Fatalf("MarkRetry() error = %v", err)
	}

	got, err := repo.GetByID(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != DeliveryStatusRetryWait {
		t.Fatalf("status = %q, want retry_wait", got.Status)
	}
	if got.NextAttemptAt == nil || *got.NextAttemptAt != 400 {
		t.Fatalf("next_attempt_at = %#v, want 400", got.NextAttemptAt)
	}
	if got.LastAttemptAt == nil || *got.LastAttemptAt != 300 {
		t.Fatalf("last_attempt_at = %#v, want 300", got.LastAttemptAt)
	}
	if got.LastStatusCode == nil || *got.LastStatusCode != 500 {
		t.Fatalf("last_status_code = %#v, want 500", got.LastStatusCode)
	}
	if got.LastError != "internal error" {
		t.Fatalf("last_error = %q", got.LastError)
	}
	if got.ClaimExpiresAt != nil {
		t.Fatalf("claim_expires_at should be nil")
	}
}

func TestHookDeliveryMarkDeadLettered(t *testing.T) {
	store, wsID := newHookTestStore(t)
	repo := NewHookDeliveryRepository(store.DB())
	hookID := "hook-1"

	d := makeDelivery(t, hookID, wsID, func(del *HookDelivery) {
		del.Status = DeliveryStatusDelivering
		del.ClaimExpiresAt = int64Ptr(200)
		del.AttemptCount = 5
	})
	if err := repo.Enqueue([]HookDelivery{d}); err != nil {
		t.Fatal(err)
	}

	statusCode := 503
	if err := repo.MarkDeadLettered(d.ID, 300, &statusCode, "max retries exceeded"); err != nil {
		t.Fatalf("MarkDeadLettered() error = %v", err)
	}

	got, err := repo.GetByID(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != DeliveryStatusDeadLettered {
		t.Fatalf("status = %q, want dead_lettered", got.Status)
	}
	if got.LastAttemptAt == nil || *got.LastAttemptAt != 300 {
		t.Fatalf("last_attempt_at = %#v, want 300", got.LastAttemptAt)
	}
	if got.LastStatusCode == nil || *got.LastStatusCode != 503 {
		t.Fatalf("last_status_code = %#v, want 503", got.LastStatusCode)
	}
	if got.LastError != "max retries exceeded" {
		t.Fatalf("last_error = %q", got.LastError)
	}
	if got.ClaimExpiresAt != nil {
		t.Fatalf("claim_expires_at should be nil")
	}
}

func TestHookDeliveryMarkDeadLetteredNilStatusCode(t *testing.T) {
	store, wsID := newHookTestStore(t)
	repo := NewHookDeliveryRepository(store.DB())
	hookID := "hook-1"

	d := makeDelivery(t, hookID, wsID, func(del *HookDelivery) {
		del.Status = DeliveryStatusDelivering
		del.AttemptCount = 5
	})
	if err := repo.Enqueue([]HookDelivery{d}); err != nil {
		t.Fatal(err)
	}

	if err := repo.MarkDeadLettered(d.ID, 300, nil, "connection refused"); err != nil {
		t.Fatalf("MarkDeadLettered() error = %v", err)
	}

	got, err := repo.GetByID(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != DeliveryStatusDeadLettered {
		t.Fatalf("status = %q, want dead_lettered", got.Status)
	}
	// last_status_code should remain whatever GORM default is (nil since not updated)
}

func TestHookDeliveryMarkDisabledSkipped(t *testing.T) {
	store, wsID := newHookTestStore(t)
	repo := NewHookDeliveryRepository(store.DB())
	hookID := "hook-1"

	d := makeDelivery(t, hookID, wsID)
	if err := repo.Enqueue([]HookDelivery{d}); err != nil {
		t.Fatal(err)
	}

	if err := repo.MarkDisabledSkipped(d.ID, 300); err != nil {
		t.Fatalf("MarkDisabledSkipped() error = %v", err)
	}

	got, err := repo.GetByID(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != DeliveryStatusDisabledSkipped {
		t.Fatalf("status = %q, want disabled_skipped", got.Status)
	}
	if got.LastError != "hook disabled" {
		t.Fatalf("last_error = %q, want 'hook disabled'", got.LastError)
	}
	if got.ClaimExpiresAt != nil {
		t.Fatalf("claim_expires_at should be nil")
	}
	if got.ModifiedAt != 300 {
		t.Fatalf("modified_at = %d, want 300", got.ModifiedAt)
	}
}

func TestHookDeliveryRequeue(t *testing.T) {
	store, wsID := newHookTestStore(t)
	repo := NewHookDeliveryRepository(store.DB())
	hookID := "hook-1"

	d := makeDelivery(t, hookID, wsID, func(del *HookDelivery) {
		del.Status = DeliveryStatusRetryWait
		del.NextAttemptAt = int64Ptr(500)
		del.AttemptCount = 3
	})
	if err := repo.Enqueue([]HookDelivery{d}); err != nil {
		t.Fatal(err)
	}

	if err := repo.Requeue(d.ID, 400); err != nil {
		t.Fatalf("Requeue() error = %v", err)
	}

	got, err := repo.GetByID(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != DeliveryStatusQueued {
		t.Fatalf("status = %q, want queued", got.Status)
	}
	if got.NextAttemptAt != nil {
		t.Fatalf("next_attempt_at = %#v, want nil", got.NextAttemptAt)
	}
	if got.ClaimExpiresAt != nil {
		t.Fatalf("claim_expires_at = %#v, want nil", got.ClaimExpiresAt)
	}
	if got.ModifiedAt != 400 {
		t.Fatalf("modified_at = %d, want 400", got.ModifiedAt)
	}
	// attempt_count is not reset by Requeue
	if got.AttemptCount != 3 {
		t.Fatalf("attempt_count = %d, want 3 (unchanged)", got.AttemptCount)
	}
}

func TestHookDeliveryListByHook(t *testing.T) {
	store, wsID := newHookTestStore(t)
	repo := NewHookDeliveryRepository(store.DB())
	hookID := "hook-1"
	otherHookID := "hook-2"

	d1 := makeDelivery(t, hookID, wsID)
	d2 := makeDelivery(t, hookID, wsID, func(d *HookDelivery) {
		d.Status = DeliveryStatusSucceeded
	})
	d3 := makeDelivery(t, otherHookID, wsID)

	if err := repo.Enqueue([]HookDelivery{d1, d2, d3}); err != nil {
		t.Fatal(err)
	}

	// filter by hook
	all, err := repo.ListByHook(hookID, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("ListByHook(hook-1) count = %d, want 2", len(all))
	}

	// filter by hook + status
	succeeded, err := repo.ListByHook(hookID, DeliveryStatusSucceeded, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(succeeded) != 1 {
		t.Fatalf("ListByHook(hook-1, succeeded) count = %d, want 1", len(succeeded))
	}

	// limit
	limited, err := repo.ListByHook(hookID, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 1 {
		t.Fatalf("ListByHook with limit count = %d, want 1", len(limited))
	}
}
