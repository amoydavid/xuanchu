package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

func TestTaskActivityHTTPReturnsSemanticPage(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store, Clock: app.FixedClock{NowUnix: 100}})
	if err != nil {
		t.Fatal(err)
	}
	tsk, err := svc.Add(app.AddInput{Title: "HTTP activity"})
	if err != nil {
		t.Fatal(err)
	}
	annotationSvc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store, Clock: app.FixedClock{NowUnix: 101}})
	if err != nil {
		t.Fatal(err)
	}
	if err := annotationSvc.Annotate(tsk.UUID, "note from HTTP"); err != nil {
		t.Fatal(err)
	}

	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/tasks/"+tsk.UUID+"/activity?limit=1", map[string]string{"Authorization": "Bearer " + fixture.token})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var envelope struct {
		Data struct {
			Entries []struct {
				ID         string         `json:"id"`
				Kind       string         `json:"kind"`
				Action     string         `json:"action"`
				Actor      map[string]any `json:"actor"`
				OccurredAt string         `json:"occurred_at"`
			} `json:"entries"`
			NextCursor *string `json:"next_cursor"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Data.Entries) != 1 || envelope.Data.Entries[0].Action != "commented" {
		t.Fatalf("data = %#v", envelope.Data)
	}
	if envelope.Data.Entries[0].OccurredAt != "1970-01-01T00:01:41Z" {
		t.Fatalf("occurred_at = %q", envelope.Data.Entries[0].OccurredAt)
	}
	if envelope.Data.NextCursor == nil || *envelope.Data.NextCursor == "" {
		t.Fatalf("next_cursor = %#v", envelope.Data.NextCursor)
	}
	if envelope.Data.Entries[0].Actor["type"] != "user" {
		t.Fatalf("actor = %#v", envelope.Data.Entries[0].Actor)
	}
}

func TestTaskActivityHTTPValidatesLimitAndCursor(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	tsk, err := svc.Add(app.AddInput{Title: "validation"})
	if err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{"Authorization": "Bearer " + fixture.token}
	badLimit := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/tasks/"+tsk.UUID+"/activity?limit=101", headers)
	assertHTTPErrorCode(t, badLimit, http.StatusBadRequest, "api_bad_limit")
	badCursor := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/tasks/"+tsk.UUID+"/activity?cursor=bad", headers)
	assertHTTPErrorCode(t, badCursor, http.StatusBadRequest, "api_bad_cursor")
	badRef := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/tasks/1/activity", headers)
	assertHTTPErrorCode(t, badRef, http.StatusBadRequest, "task_ref_invalid")
}

func TestTaskActivityHTTPPreservesFieldChangeJSONContract(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	due := int64(1783036800)
	tsk, err := svc.Add(app.AddInput{Title: "before", Due: &due, Tags: []string{"ads"}})
	if err != nil {
		t.Fatal(err)
	}
	after := "after"
	if err := svc.Modify(tsk.UUID, app.ModifyInput{Title: &after}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Modify(tsk.UUID, app.ModifyInput{ClearDue: true}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Modify(tsk.UUID, app.ModifyInput{AddTags: []string{"dashboard"}}); err != nil {
		t.Fatal(err)
	}

	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/tasks/"+tsk.UUID+"/activity?limit=20", map[string]string{"Authorization": "Bearer " + fixture.token})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var envelope struct {
		Data struct {
			Entries []struct {
				Action  string           `json:"action"`
				Changes []map[string]any `json:"changes"`
			} `json:"entries"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rr.Body.String())
	}
	changes := make(map[string]map[string]any)
	for _, entry := range envelope.Data.Entries {
		if entry.Action != "fields_changed" {
			continue
		}
		for _, change := range entry.Changes {
			field, _ := change["field"].(string)
			if _, exists := changes[field]; !exists {
				changes[field] = change
			}
		}
	}
	dueChange := changes["due"]
	current, ok := dueChange["current"].(map[string]any)
	if !ok || current["raw"] != nil {
		t.Fatalf("due current = %#v, want explicit raw null; response=%s", dueChange["current"], rr.Body.String())
	}
	tagsChange := changes["tags"]
	removed, ok := tagsChange["removed"].([]any)
	if !ok || len(removed) != 0 {
		t.Fatalf("tags removed = %#v, want []", tagsChange["removed"])
	}
	titleChange := changes["title"]
	if _, exists := titleChange["added"]; exists {
		t.Fatalf("scalar title change unexpectedly contains added: %#v", titleChange)
	}
	if _, exists := titleChange["removed"]; exists {
		t.Fatalf("scalar title change unexpectedly contains removed: %#v", titleChange)
	}
}

func TestTaskAuditRouteIsRemoved(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read")
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/tasks/some-task/audit", map[string]string{"Authorization": "Bearer " + fixture.token})
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rr.Code, rr.Body.String())
	}
}

func TestTaskActivityHTTPRequiresTaskReadAndWorkspaceScope(t *testing.T) {
	withoutRead := newHTTPServerWithTokenFixture(t, "project:read")
	svc, err := app.NewService(app.ServiceOptions{Store: withoutRead.server.store})
	if err != nil {
		t.Fatal(err)
	}
	tsk, err := svc.Add(app.AddInput{Title: "permission boundary"})
	if err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{"Authorization": "Bearer " + withoutRead.token}
	denied := requestHTTP(t, withoutRead.server, http.MethodGet, "/api/v1/tasks/"+tsk.UUID+"/activity?workspace=local", headers)
	assertHTTPErrorCode(t, denied, http.StatusForbidden, "token_scope_denied")

	fixture := newHTTPServerWithTokenFixture(t, "task:read")
	localSvc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	localTask, err := localSvc.Add(app.AddInput{Title: "task reader activity"})
	if err != nil {
		t.Fatal(err)
	}
	other := mustCreateHTTPWorkspace(t, fixture.server.store, storage.Workspace{
		ID: "activity-other", Slug: "activity-other", Name: "Activity Other",
		Visibility: "team", SettingsJSON: "{}", CreatedAt: 100, ModifiedAt: 100,
	})
	mustUpsertHTTPMembership(t, fixture.server.store, storage.Membership{
		UserID: localSvc.Runtime().ActorUserID, WorkspaceID: other.ID,
		Role: string(app.RoleOwner), JoinedAt: 100, ModifiedAt: 100,
	})
	workspaceDenied := requestHTTP(
		t, fixture.server, http.MethodGet,
		"/api/v1/tasks/"+localTask.UUID+"/activity?workspace=activity-other",
		map[string]string{"Authorization": "Bearer " + fixture.token},
	)
	assertHTTPErrorCode(t, workspaceDenied, http.StatusForbidden, "workspace_scope_denied")

	activityAllowed := requestHTTP(
		t, fixture.server, http.MethodGet,
		"/api/v1/tasks/"+localTask.UUID+"/activity?workspace=local",
		map[string]string{"Authorization": "Bearer " + fixture.token},
	)
	if activityAllowed.Code != http.StatusOK {
		t.Fatalf("task reader activity status = %d body=%s", activityAllowed.Code, activityAllowed.Body.String())
	}
	auditDenied := requestHTTP(
		t, fixture.server, http.MethodGet, "/api/v1/audit?workspace=local",
		map[string]string{"Authorization": "Bearer " + fixture.token},
	)
	assertHTTPErrorCode(t, auditDenied, http.StatusForbidden, "token_scope_denied")
}
