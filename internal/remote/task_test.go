package remote

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRemoteTaskResponseDecodesTaskSlug(t *testing.T) {
	raw := `{
		"uuid":"u1",
		"description":"remote",
		"status":"pending",
		"entry":"1970-01-01T00:00:01Z",
		"modified":"1970-01-01T00:00:02Z",
		"project":"api",
		"task_slug":"api-12"
	}`
	var dto taskResponseJSON
	if err := json.Unmarshal([]byte(raw), &dto); err != nil {
		t.Fatalf("Unmarshal response task error = %v", err)
	}
	tsk, err := dto.toTask()
	if err != nil {
		t.Fatalf("toTask() error = %v", err)
	}
	if tsk.ProjectSeq == nil || *tsk.ProjectSeq != 12 {
		t.Fatalf("ProjectSeq = %#v, want 12", tsk.ProjectSeq)
	}
	if tsk.Project == nil || *tsk.Project != "api" {
		t.Fatalf("Project = %#v, want api", tsk.Project)
	}
}

func TestRemoteTaskResponseRejectsMismatchedTaskSlugProject(t *testing.T) {
	raw := `{
		"uuid":"u1",
		"description":"remote",
		"status":"pending",
		"entry":"1970-01-01T00:00:01Z",
		"modified":"1970-01-01T00:00:02Z",
		"project":"api",
		"task_slug":"web-12"
	}`
	var dto taskResponseJSON
	if err := json.Unmarshal([]byte(raw), &dto); err != nil {
		t.Fatalf("Unmarshal response task error = %v", err)
	}
	if _, err := dto.toTask(); err == nil || !strings.Contains(err.Error(), "task_slug") {
		t.Fatalf("toTask() error = %v, want task_slug mismatch", err)
	}
}
