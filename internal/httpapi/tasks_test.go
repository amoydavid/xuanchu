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

func TestTaskListProjectIDSelectsOwningWorkspace(t *testing.T) {
	store := openHTTPTestStore(t)
	svc, err := app.NewService(app.ServiceOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	work, err := svc.AddWorkspace(app.AddWorkspaceInput{Slug: "work", Name: "Work"})
	if err != nil {
		t.Fatal(err)
	}
	workSvc, err := app.NewService(app.ServiceOptions{Store: store, WorkspaceRef: work.Slug})
	if err != nil {
		t.Fatal(err)
	}
	project, err := workSvc.AddProject(app.AddProjectInput{Slug: "api", Name: "API"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workSvc.Add(app.AddInput{Description: "work task", Project: &project.Slug}); err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateToken(app.CreateTokenInput{
		Name:        "project-id",
		Scopes:      []string{"task:read"},
		ProjectRefs: []string{project.ID},
	})
	if err != nil {
		t.Fatal(err)
	}

	srv := NewServer(Options{Store: store})
	rr := requestHTTP(t, srv, http.MethodGet, "/api/v1/tasks?project_id="+project.ID, map[string]string{
		"Authorization": "Bearer " + created.RawToken,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if body := rr.Body.String(); !strings.Contains(body, "work task") {
		t.Fatalf("project_id did not select owning workspace: %s", body)
	}
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

func TestTaskListRejectsInvalidLimit(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read")
	authHeader := map[string]string{"Authorization": "Bearer " + fixture.token}
	cases := []string{"0", "-1", "1001", "bad"}
	for _, limit := range cases {
		rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/tasks?limit="+limit, authHeader)
		assertHTTPErrorCode(t, rr, http.StatusBadRequest, "api_bad_limit")
	}
}

func TestTaskListDefaultLimitDoesNotRejectEmptyLimit(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read")
	authHeader := map[string]string{"Authorization": "Bearer " + fixture.token}
	for _, path := range []string{"/api/v1/tasks?limit=", "/api/v1/tasks"} {
		rr := requestHTTP(t, fixture.server, http.MethodGet, path, authHeader)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status = %d body=%s", path, rr.Code, rr.Body.String())
		}
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

func TestTaskAddAssignees(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")

	body := `{"description":"assigned","assignees":["local"]}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/tasks", body, map[string]string{
		"Authorization": "Bearer " + fixture.token,
		"Content-Type":  "application/json",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Data struct {
			Assignees []struct {
				Name string `json:"name"`
			} `json:"assignees"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data.Assignees) != 1 || payload.Data.Assignees[0].Name != "local" {
		t.Fatalf("assignees = %#v, want [local]", payload.Data.Assignees)
	}
}

func TestTaskModifyAssignees(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.Add(app.AddInput{Description: "assigned later"})
	if err != nil {
		t.Fatal(err)
	}

	body := `{"assignees":["local"]}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPatch, "/api/v1/tasks/"+created.UUID, body, map[string]string{
		"Authorization": "Bearer " + fixture.token,
		"Content-Type":  "application/json",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Data struct {
			Assignees []struct {
				Name string `json:"name"`
			} `json:"assignees"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data.Assignees) != 1 || payload.Data.Assignees[0].Name != "local" {
		t.Fatalf("assignees = %#v, want [local]", payload.Data.Assignees)
	}
}

func TestTaskListByAssignee(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(app.AddInput{Description: "mine", Assignees: []string{"local"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(app.AddInput{Description: "plain"}); err != nil {
		t.Fatal(err)
	}

	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/tasks?query=assignee:me", map[string]string{
		"Authorization": "Bearer " + fixture.token,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "mine") || strings.Contains(body, "plain") {
		t.Fatalf("assignee query did not filter tasks: %s", body)
	}
}
