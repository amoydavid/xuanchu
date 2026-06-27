package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/app"
)

// projectListItem 对齐 projectResponse 的子集，用于断言聚合字段。
type projectListItem struct {
	TaskCount      int `json:"task_count"`
	PendingCount   int `json:"pending_count"`
	CompletedCount int `json:"completed_count"`
}

func TestHandleProjectListReturnsStatusBreakdown(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "project:read", "project:write", "task:read", "task:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddProject(app.AddProjectInput{Slug: "api", Name: "API"})
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}
	projectRef := created.Slug
	if _, err := svc.Add(app.AddInput{Title: "p1", Project: &projectRef}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(app.AddInput{Title: "p2", Project: &projectRef}); err != nil {
		t.Fatal(err)
	}
	done, err := svc.Add(app.AddInput{Title: "done", Project: &projectRef})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Done(done.UUID); err != nil {
		t.Fatal(err)
	}

	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/projects?all=true",
		map[string]string{"Authorization": "Bearer " + fixture.token})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data []projectListItem `json:"data"`
	}
	if err := json.Unmarshal([]byte(rr.Body.String()), &resp); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, p := range resp.Data {
		if p.TaskCount == 3 && p.PendingCount == 2 && p.CompletedCount == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("no project with counts 3/2/1, got %#v", resp.Data)
	}
}
