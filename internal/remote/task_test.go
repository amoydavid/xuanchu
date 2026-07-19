package remote

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRemoteTaskResponseDecodesTaskSlug(t *testing.T) {
	raw := `{
		"id":"u1",
		"uuid":"u1",
		"url":"https://xuanchu.example.com/workspaces/local/projects/api/tasks/api-12",
		"title":"remote",
		"status":"pending",
		"entry":1,
		"modified":2,
		"project":"api",
		"task_slug":"api-12"
	}`
	dto, err := parseTaskOccurrenceDTO(json.RawMessage(raw))
	if err != nil {
		t.Fatalf("parseTaskOccurrenceDTO() error = %v", err)
	}
	if dto.ProjectSeq == nil || *dto.ProjectSeq != 12 {
		t.Fatalf("ProjectSeq = %#v, want 12", dto.ProjectSeq)
	}
	if dto.Project == nil || *dto.Project != "api" {
		t.Fatalf("Project = %#v, want api", dto.Project)
	}
	if dto.URL != "https://xuanchu.example.com/workspaces/local/projects/api/tasks/api-12" {
		t.Fatalf("URL = %q", dto.URL)
	}
}

func TestRemoteTaskResponseRejectsLegacyJSONTaskShape(t *testing.T) {
	raw := `{
		"uuid":"u1",
		"title":"legacy",
		"status":"pending",
		"entry":"1970-01-01T00:00:01Z",
		"modified":"1970-01-01T00:00:02Z"
	}`
	if _, err := parseTaskOccurrenceDTO(json.RawMessage(raw)); err == nil {
		t.Fatal("parseTaskOccurrenceDTO legacy JSONTask shape error = nil")
	}
}

func TestRemoteTaskResponseRejectsMismatchedTaskSlugProject(t *testing.T) {
	raw := `{
		"id":"u1",
		"uuid":"u1",
		"title":"remote",
		"status":"pending",
		"entry":1,
		"modified":2,
		"project":"api",
		"task_slug":"web-12"
	}`
	if _, err := parseTaskOccurrenceDTO(json.RawMessage(raw)); err == nil || !strings.Contains(err.Error(), "task_slug") {
		t.Fatalf("parseTaskOccurrenceDTO() error = %v, want task_slug mismatch", err)
	}
}

func TestRemoteUnifiedOccurrenceResponseValidatesSlugProject(t *testing.T) {
	raw := `{
		"id":"occ:series:1",
		"uuid":"u1",
		"title":"remote occurrence",
		"status":"pending",
		"entry":1,
		"modified":2,
		"project":"api",
		"task_slug":"web-12",
		"recurrence_info":{"role":"occurrence","series_id":"series","rule":"daily","recurrence_at":1,"materialization":"materialized"}
	}`
	if _, err := parseTaskOccurrenceDTO(json.RawMessage(raw)); err == nil || !strings.Contains(err.Error(), "task_slug") {
		t.Fatalf("parseTaskOccurrenceDTO() error = %v, want task_slug mismatch", err)
	}
}
