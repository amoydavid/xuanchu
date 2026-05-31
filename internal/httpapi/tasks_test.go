package httpapi

import (
	"net/http"
	"testing"

	"github.com/dajee/taskg/internal/app"
)

func TestTaskInfoRejectsNonUUIDPath(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read")

	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/tasks/1", map[string]string{"Authorization": "Bearer " + fixture.token})
	assertHTTPErrorCode(t, rr, http.StatusBadRequest, "task_uuid_invalid")
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
