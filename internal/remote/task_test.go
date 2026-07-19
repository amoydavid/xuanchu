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

// TestRemoteTaskDescriptionIsMarkdownString 验证 description 仍是纯 Markdown 字符串，
// 不出现 ProseMirror JSON、HTML、blob URL 或预签名 URL（spec §20 / 计划 4 Task 11）。
func TestRemoteTaskDescriptionIsMarkdownString(t *testing.T) {
	markdown := "请 [@Alice](ref://user/8c8b1bed-2e75-4de8-8d5f-c94cbf2b3001) 看\n\n![图](ref://attachment/40af0185-316f-42bb-b52b-545d21f6f012)"
	raw := `{
		"id":"u1",
		"uuid":"u1",
		"title":"ref-task",
		"status":"pending",
		"entry":1,
		"modified":2,
		"description":` + jsonQuote(markdown) + `
	}`
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatal(err)
	}
	desc, ok := payload["description"].(string)
	if !ok {
		t.Fatalf("description type = %T, want string", payload["description"])
	}
	if desc != markdown {
		t.Fatalf("description = %q, want %q", desc, markdown)
	}
	// 不应出现非 Markdown 内容。
	for _, forbidden := range []string{"<img", "<div", "\"type\":\"doc\"", "blob:", "X-Amz-Signature", "data:image"} {
		if strings.Contains(desc, forbidden) {
			t.Fatalf("description leaked %q: %s", forbidden, desc)
		}
	}
}

// jsonQuote 把字符串转为 JSON 字符串字面量。
func jsonQuote(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
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
