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

func TestHTTPProjectTaskSummary(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "project:read", "project:write", "task:read", "task:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddProject(app.AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}
	projectRef := created.Slug
	priorityH := "H"
	if _, err := svc.Add(app.AddInput{Title: "overdue", Project: &projectRef, Priority: &priorityH, Due: int64Ptr(1)}); err != nil {
		t.Fatalf("Add(overdue) error = %v", err)
	}
	if _, err := svc.Add(app.AddInput{Title: "high only", Project: &projectRef, Priority: &priorityH}); err != nil {
		t.Fatalf("Add(high only) error = %v", err)
	}

	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/projects/ops/task-summary?workspace=local",
		map[string]string{"Authorization": "Bearer " + fixture.token})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
	var envelope struct {
		Data struct {
			OverdueCount          int `json:"overdue_count"`
			HighPriorityOpenCount int `json:"high_priority_open_count"`
			WaitReadyCount        int `json:"wait_ready_count"`
			UnassignedOpenCount   int `json:"unassigned_open_count"`
			OverdueRefs           []struct {
				TaskSlug string `json:"task_slug"`
				Label    string `json:"label"`
			} `json:"overdue_refs"`
			Workload []struct {
				Label string          `json:"label"`
				User  json.RawMessage `json:"user"`
			} `json:"workload"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.OverdueCount != 1 {
		t.Fatalf("overdue_count = %d, want 1", envelope.Data.OverdueCount)
	}
	if envelope.Data.HighPriorityOpenCount != 2 {
		t.Fatalf("high_priority_open_count = %d, want 2", envelope.Data.HighPriorityOpenCount)
	}
	if len(envelope.Data.OverdueRefs) != 1 || envelope.Data.OverdueRefs[0].Label != "ops-1" {
		t.Fatalf("overdue_refs = %#v, want label ops-1", envelope.Data.OverdueRefs)
	}
	// 负载包含未分配行（user 为 null）
	var hasUnassigned bool
	for _, row := range envelope.Data.Workload {
		if string(row.User) == "null" || len(row.User) == 0 {
			hasUnassigned = true
		}
	}
	if !hasUnassigned {
		t.Fatalf("workload missing unassigned row: %#v", envelope.Data.Workload)
	}

	// 缺 task:read 的 token 不能获取摘要。
	projectOnlyToken, err := svc.CreateTenantAccessToken(app.CreateTenantAccessTokenInput{
		Name:   "project-only",
		Scopes: []string{"project:read"},
	})
	if err != nil {
		t.Fatalf("CreateTenantAccessToken(project-only) error = %v", err)
	}
	rr = requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/projects/ops/task-summary?workspace=local",
		map[string]string{"Authorization": "Bearer " + projectOnlyToken.RawToken})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("project-only status = %d body = %s, want 403", rr.Code, rr.Body.String())
	}
}

func int64Ptr(v int64) *int64 {
	return &v
}
