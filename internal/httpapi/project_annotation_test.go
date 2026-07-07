package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/app"
)

func TestProjectAnnotationAddAndList(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "project:read", "project:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	project, err := svc.AddProject(app.AddProjectInput{Slug: "testproj", Name: "Test"})
	if err != nil {
		t.Fatal(err)
	}

	headers := map[string]string{
		"Authorization": "Bearer " + fixture.token,
		"Content-Type":  "application/json",
	}

	body := `{"content":"first note"}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/projects/"+project.Slug+"/annotations", body, headers)
	if rr.Code != http.StatusCreated {
		t.Fatalf("add annotation status = %d body = %s", rr.Code, rr.Body.String())
	}

	var addPayload struct {
		Data struct {
			ID        string `json:"id"`
			Content   string `json:"content"`
			ProjectID string `json:"project_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &addPayload); err != nil {
		t.Fatal(err)
	}
	if addPayload.Data.Content != "first note" {
		t.Fatalf("annotation content = %q, want %q", addPayload.Data.Content, "first note")
	}
	if addPayload.Data.ProjectID != project.ID {
		t.Fatalf("annotation project_id = %q, want %q", addPayload.Data.ProjectID, project.ID)
	}

	rr = requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/projects/"+project.Slug+"/annotations", headers)
	if rr.Code != http.StatusOK {
		t.Fatalf("list annotations status = %d body = %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "first note") {
		t.Fatalf("list annotations body = %s, want first note", rr.Body.String())
	}
}

func TestProjectAnnotationDelete(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "project:read", "project:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	project, err := svc.AddProject(app.AddProjectInput{Slug: "delproj", Name: "Delete"})
	if err != nil {
		t.Fatal(err)
	}

	annotation, err := svc.ProjectAnnotate(project.Slug, "will be removed")
	if err != nil {
		t.Fatal(err)
	}

	headers := map[string]string{
		"Authorization": "Bearer " + fixture.token,
	}

	rr := requestHTTP(t, fixture.server, http.MethodDelete, "/api/v1/projects/"+project.Slug+"/annotations/"+annotation.ID, headers)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete annotation status = %d body = %s", rr.Code, rr.Body.String())
	}

	rr = requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/projects/"+project.Slug+"/annotations", headers)
	if rr.Code != http.StatusOK {
		t.Fatalf("list annotations status = %d body = %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), annotation.ID) {
		t.Fatalf("annotation still present after deletion: %s", rr.Body.String())
	}
}

func TestProjectTimeline(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "project:read", "project:write", "task:read", "task:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	project, err := svc.AddProject(app.AddProjectInput{Slug: "tlproj", Name: "Timeline"})
	if err != nil {
		t.Fatal(err)
	}

	annotation, err := svc.ProjectAnnotate(project.Slug, "project note")
	if err != nil {
		t.Fatal(err)
	}

	tsk, err := svc.Add(app.AddInput{Title: "timeline task", Project: &project.Slug})
	if err != nil {
		t.Fatal(err)
	}
	_ = svc.Annotate(tsk.UUID, "task annotation")

	headers := map[string]string{
		"Authorization": "Bearer " + fixture.token,
	}

	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/projects/"+project.Slug+"/timeline?limit=50", headers)
	if rr.Code != http.StatusOK {
		t.Fatalf("timeline status = %d body = %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "project note") {
		t.Fatalf("timeline missing project note: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "task annotation") {
		t.Fatalf("timeline missing task annotation: %s", rr.Body.String())
	}
	// project timeline 的 source_id 必须是 annotation id（便于 Activity 去重和删除）。
	var payload struct {
		Data []struct {
			SourceType string `json:"source_type"`
			SourceID   string `json:"source_id"`
			Content    string `json:"content"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	var projectRow, taskRow *struct {
		SourceType string `json:"source_type"`
		SourceID   string `json:"source_id"`
		Content    string `json:"content"`
	}
	for i := range payload.Data {
		if payload.Data[i].SourceType == "project" && projectRow == nil {
			projectRow = &payload.Data[i]
		}
		if payload.Data[i].SourceType == "task" && taskRow == nil {
			taskRow = &payload.Data[i]
		}
	}
	if projectRow == nil || projectRow.SourceID != annotation.ID {
		t.Fatalf("timeline project source_id = %#v, want annotation id %q", projectRow, annotation.ID)
	}
	if taskRow == nil || taskRow.SourceID != tsk.UUID {
		t.Fatalf("timeline task source_id = %#v, want task uuid %q", taskRow, tsk.UUID)
	}
}
