package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/dajee/taskg/internal/app"
)

func TestTaskInfoRejectsNonUUIDPath(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read")

	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/tasks/1", map[string]string{"Authorization": "Bearer " + fixture.token})
	assertHTTPErrorCode(t, rr, http.StatusBadRequest, "task_uuid_invalid")
}

func TestTaskInfoMissingReturnsTaskNotFound(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read")

	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/tasks/00000000-0000-0000-0000-000000000001", map[string]string{
		"Authorization": "Bearer " + fixture.token,
	})
	assertHTTPErrorCode(t, rr, http.StatusNotFound, "task_not_found")
}

func TestTaskAddRejectsProjectMismatch(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write", "project:read", "project:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	alpha, err := svc.AddProject(app.AddProjectInput{Slug: "alpha", Name: "Alpha"})
	if err != nil {
		t.Fatal(err)
	}
	beta, err := svc.AddProject(app.AddProjectInput{Slug: "beta", Name: "Beta"})
	if err != nil {
		t.Fatal(err)
	}

	body := `{"description":"mismatch","project":"alpha","project_id":"` + beta.ID + `"}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/tasks", body, map[string]string{
		"Authorization": "Bearer " + fixture.token,
		"Content-Type":  "application/json",
	})
	assertHTTPErrorCode(t, rr, http.StatusBadRequest, "project_mismatch")

	_ = alpha
}

func TestTaskListAcceptsQueryParameter(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(app.AddInput{Description: "next task", Tags: []string{"next"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(app.AddInput{Description: "plain task"}); err != nil {
		t.Fatal(err)
	}

	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/tasks?query=%2Bnext", map[string]string{
		"Authorization": "Bearer " + fixture.token,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "next task") || strings.Contains(body, "plain task") {
		t.Fatalf("query parameter did not filter tasks: %s", body)
	}
}

func TestTaskListNoContextBypassesActiveContext(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write", "context:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(app.AddInput{Description: "context task", Tags: []string{"ctx"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(app.AddInput{Description: "plain task"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.DefineContext("ctx", "+ctx"); err != nil {
		t.Fatal(err)
	}
	if err := svc.UseContext("ctx"); err != nil {
		t.Fatal(err)
	}

	authHeader := map[string]string{"Authorization": "Bearer " + fixture.token}
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/tasks", authHeader)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if body := rr.Body.String(); !strings.Contains(body, "context task") || strings.Contains(body, "plain task") {
		t.Fatalf("context-filtered list = %s", body)
	}

	rr = requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/tasks?no_context=true", authHeader)
	if rr.Code != http.StatusOK {
		t.Fatalf("no_context status = %d body=%s", rr.Code, rr.Body.String())
	}
	if body := rr.Body.String(); !strings.Contains(body, "context task") || !strings.Contains(body, "plain task") {
		t.Fatalf("no_context list = %s", body)
	}
}

func TestTaskAddAcceptsDueField(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")

	body := `{"description":"deadline","due":1893456000}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/tasks", body, map[string]string{
		"Authorization": "Bearer " + fixture.token,
		"Content-Type":  "application/json",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Data struct {
			Due *string `json:"due"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.Due == nil || *payload.Data.Due == "" {
		t.Fatalf("due = %#v, want non-empty due; body=%s", payload.Data.Due, rr.Body.String())
	}
}
