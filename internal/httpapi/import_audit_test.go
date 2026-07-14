package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

func TestOrdinaryTaskImportUsesDedicatedVersionedContract(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	createHTTPTestProjectViaService(t, fixture)
	body := `{
		"schema":"xuanchu.task-import/v1",
		"tasks":[{
			"uuid":"11111111-1111-4111-8111-111111111111",
			"title":"普通导入任务",
			"status":"pending",
			"entry":"2030-01-01T00:00:00Z",
			"modified":"2030-01-01T00:00:00Z",
			"project":"ops"
		}]
	}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost,
		"/api/v1/task-imports?workspace=local&project=ops", body,
		map[string]string{"Authorization": "Bearer " + fixture.token})
	if rr.Code != http.StatusOK {
		t.Fatalf("ordinary import: status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Body.String(); got == "" || !strings.Contains(got, `"imported":1`) {
		t.Fatalf("ordinary import response = %s", got)
	}
}

func TestOrdinaryTaskImportRejectsOccurrenceIdentity(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	createHTTPTestProjectViaService(t, fixture)
	body := `{
		"schema":"xuanchu.task-import/v1",
		"tasks":[{
			"uuid":"11111111-1111-4111-8111-111111111111",
			"title":"伪造循环实例",
			"status":"pending",
			"entry":"2030-01-01T00:00:00Z",
			"modified":"2030-01-01T00:00:00Z",
			"project":"ops",
			"series_id":"22222222-2222-4222-8222-222222222222",
			"recurrence_at":"2030-01-01T23:59:59Z"
		}]
	}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost,
		"/api/v1/task-imports?workspace=local&project=ops", body,
		map[string]string{"Authorization": "Bearer " + fixture.token})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("occurrence import: status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "task_import_occurrence_not_allowed") {
		t.Fatalf("unexpected error: %s", rr.Body.String())
	}
}
