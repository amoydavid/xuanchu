package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/app"
)

func TestHTTPHomeReturnsPersonalWorkAndProjectAttention(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "project:read", "project:write", "task:read", "task:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	project, err := svc.AddProject(app.AddProjectInput{Slug: "home", Name: "Home"})
	if err != nil {
		t.Fatal(err)
	}
	high := "H"
	due := int64(1)
	if _, err := svc.AddTaskView(app.AddInput{
		Title: "home task", Project: &project.Slug, Priority: &high, Due: &due, Assignees: []string{"local"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ProjectAnnotate(project.Slug, "home update"); err != nil {
		t.Fatal(err)
	}

	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/home", map[string]string{"Authorization": "Bearer " + fixture.token})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
	var envelope struct {
		Data struct {
			Today     string `json:"today"`
			ActorType string `json:"actor_type"`
			MyWork    *struct {
				OpenCount int `json:"open_count"`
				Items     []struct {
					Reasons []string `json:"reasons"`
					Task    struct {
						Title     string `json:"title"`
						Assignees []struct {
							ID, Name string
						} `json:"assignees"`
					} `json:"task"`
				} `json:"items"`
			} `json:"my_work"`
			ProjectAttention []struct {
				OverdueCount int `json:"overdue_count"`
				Project      struct {
					Slug string `json:"slug"`
				} `json:"project"`
				LatestUpdate *struct {
					Content   string          `json:"content"`
					CreatedBy json.RawMessage `json:"created_by"`
				} `json:"latest_update"`
			} `json:"project_attention"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Today == "" || envelope.Data.ActorType != "user" || envelope.Data.MyWork == nil {
		t.Fatalf("home data = %#v", envelope.Data)
	}
	if envelope.Data.MyWork.OpenCount != 1 || len(envelope.Data.MyWork.Items) != 1 {
		t.Fatalf("my_work = %#v", envelope.Data.MyWork)
	}
	assignees := envelope.Data.MyWork.Items[0].Task.Assignees
	if len(assignees) != 1 || assignees[0].ID == "" || assignees[0].Name != "local" {
		t.Fatalf("assignees = %#v", assignees)
	}
	if len(envelope.Data.ProjectAttention) != 1 || envelope.Data.ProjectAttention[0].Project.Slug != "home" || envelope.Data.ProjectAttention[0].OverdueCount != 1 {
		t.Fatalf("project_attention = %#v", envelope.Data.ProjectAttention)
	}
	if envelope.Data.ProjectAttention[0].LatestUpdate == nil || envelope.Data.ProjectAttention[0].LatestUpdate.Content != "home update" {
		t.Fatalf("latest_update = %#v", envelope.Data.ProjectAttention[0].LatestUpdate)
	}
	if strings.Contains(rr.Body.String(), `"user_id"`) {
		t.Fatalf("response leaked user_id: %s", rr.Body.String())
	}
	assertSnakeCaseResponse(t, rr.Body.String())
}

func TestHTTPHomeTenantActorDegradesByCapability(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "project:read", "project:write", "task:read")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	project, err := svc.AddProject(app.AddProjectInput{Slug: "tenant", Name: "Tenant"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateTenantAccessToken(app.CreateTenantAccessTokenInput{
		Name: "project-reader", Scopes: []string{"project:read"}, WorkspaceRef: "local",
	})
	if err != nil {
		t.Fatal(err)
	}
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/home", map[string]string{"Authorization": "Bearer " + created.RawToken})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
	var envelope struct {
		Data struct {
			ActorType        string          `json:"actor_type"`
			MyWork           json.RawMessage `json:"my_work"`
			ProjectAttention []struct {
				Project struct {
					Slug string `json:"slug"`
				} `json:"project"`
			} `json:"project_attention"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.ActorType != "tenant_access_token" || string(envelope.Data.MyWork) != "null" {
		t.Fatalf("tenant home = %#v", envelope.Data)
	}
	if len(envelope.Data.ProjectAttention) != 1 || envelope.Data.ProjectAttention[0].Project.Slug != project.Slug {
		t.Fatalf("project_attention = %#v", envelope.Data.ProjectAttention)
	}
}

func TestOpenAPIDocumentsHome(t *testing.T) {
	srv := NewServer(Options{Store: openHTTPTestStore(t)})
	rr := requestHTTP(t, srv, http.MethodGet, "/openapi.json", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"/api/v1/home"`) || !strings.Contains(rr.Body.String(), `"due_today"`) {
		t.Fatalf("OpenAPI missing home contract: %s", rr.Body.String())
	}
}
