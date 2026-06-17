package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/app"
)

func TestTaskInfoRejectsNonUUIDPath(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read")

	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/tasks/1", map[string]string{"Authorization": "Bearer " + fixture.token})
	assertHTTPErrorCode(t, rr, http.StatusBadRequest, "task_ref_invalid")
}

func TestTaskHTTPAcceptsTaskSlugRefs(t *testing.T) {
	cases := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
		before     func(t *testing.T, svc *app.Service, taskUUID string) string
	}{
		{name: "info", method: http.MethodGet, path: "/api/v1/tasks/api-1", wantStatus: http.StatusOK},
		{name: "list target", method: http.MethodGet, path: "/api/v1/tasks?target=api-1", wantStatus: http.StatusOK},
		{name: "modify", method: http.MethodPatch, path: "/api/v1/tasks/api-1", body: `{"description":"updated"}`, wantStatus: http.StatusOK},
		{name: "done", method: http.MethodPost, path: "/api/v1/tasks/api-1/done", wantStatus: http.StatusOK},
		{name: "delete", method: http.MethodDelete, path: "/api/v1/tasks/api-1", wantStatus: http.StatusOK},
		{name: "start", method: http.MethodPost, path: "/api/v1/tasks/api-1/start", wantStatus: http.StatusOK},
		{name: "stop", method: http.MethodPost, path: "/api/v1/tasks/api-1/stop", wantStatus: http.StatusOK, before: func(t *testing.T, svc *app.Service, taskUUID string) string {
			t.Helper()
			if err := svc.Start(taskUUID); err != nil {
				t.Fatal(err)
			}
			return ""
		}},
		{name: "annotate", method: http.MethodPost, path: "/api/v1/tasks/api-1/annotations", body: `{"description":"note"}`, wantStatus: http.StatusOK},
		{name: "denotate", method: http.MethodDelete, path: "/api/v1/tasks/api-1/annotations/{annotationID}", wantStatus: http.StatusOK, before: func(t *testing.T, svc *app.Service, taskUUID string) string {
			t.Helper()
			if err := svc.Annotate(taskUUID, "note"); err != nil {
				t.Fatal(err)
			}
			tsk, err := svc.Info(taskUUID)
			if err != nil {
				t.Fatal(err)
			}
			if len(tsk.Annotations) != 1 || tsk.Annotations[0].ID == "" {
				t.Fatalf("annotations = %#v, want one annotation with id", tsk.Annotations)
			}
			return tsk.Annotations[0].ID
		}},
		{name: "urgency", method: http.MethodGet, path: "/api/v1/tasks/api-1/urgency", wantStatus: http.StatusOK},
		{name: "link add", method: http.MethodPost, path: "/api/v1/tasks/api-1/links", body: `{"type":"document","url":"https://example.com","title":"Example"}`, wantStatus: http.StatusCreated},
		{name: "link list", method: http.MethodGet, path: "/api/v1/tasks/api-1/links", wantStatus: http.StatusOK, before: func(t *testing.T, svc *app.Service, taskUUID string) string {
			t.Helper()
			if _, err := svc.TaskAddLink(taskUUID, "document", "https://example.com", "Example"); err != nil {
				t.Fatal(err)
			}
			return ""
		}},
		{name: "link remove", method: http.MethodDelete, path: "/api/v1/tasks/api-1/links/{linkID}", wantStatus: http.StatusOK, before: func(t *testing.T, svc *app.Service, taskUUID string) string {
			t.Helper()
			link, err := svc.TaskAddLink(taskUUID, "document", "https://example.com", "Example")
			if err != nil {
				t.Fatal(err)
			}
			return link.ID
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write", "project:read", "project:write")
			svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
			if err != nil {
				t.Fatal(err)
			}
			project, err := svc.AddProject(app.AddProjectInput{Slug: "api", Name: "API"})
			if err != nil {
				t.Fatal(err)
			}
			created, err := svc.Add(app.AddInput{Description: "slug task", Project: &project.Slug})
			if err != nil {
				t.Fatal(err)
			}
			resourceID := ""
			if tc.before != nil {
				resourceID = tc.before(t, svc, created.UUID)
			}
			path := strings.ReplaceAll(tc.path, "{linkID}", resourceID)
			path = strings.ReplaceAll(path, "{annotationID}", resourceID)
			headers := map[string]string{
				"Authorization": "Bearer " + fixture.token,
				"Content-Type":  "application/json",
			}
			rr := requestHTTPBody(t, fixture.server, tc.method, path, tc.body, headers)
			if rr.Code != tc.wantStatus {
				t.Fatalf("%s %s status = %d body=%s", tc.method, path, rr.Code, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), "slug task") && !strings.Contains(rr.Body.String(), "updated") && !strings.Contains(rr.Body.String(), "document") && tc.name != "urgency" {
				t.Fatalf("%s %s body missing expected task/link data: %s", tc.method, path, rr.Body.String())
			}
		})
	}
}

func TestTaskHTTPRejectsNumericTaskRefs(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	headers := map[string]string{
		"Authorization": "Bearer " + fixture.token,
		"Content-Type":  "application/json",
	}
	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "info", method: http.MethodGet, path: "/api/v1/tasks/1"},
		{name: "info huge numeric", method: http.MethodGet, path: "/api/v1/tasks/999999999999999999999999999999"},
		{name: "list target", method: http.MethodGet, path: "/api/v1/tasks?target=1"},
		{name: "list target huge numeric", method: http.MethodGet, path: "/api/v1/tasks?target=999999999999999999999999999999"},
		{name: "modify", method: http.MethodPatch, path: "/api/v1/tasks/1", body: `{"description":"updated"}`},
		{name: "delete", method: http.MethodDelete, path: "/api/v1/tasks/1"},
		{name: "done", method: http.MethodPost, path: "/api/v1/tasks/1/done"},
		{name: "start", method: http.MethodPost, path: "/api/v1/tasks/1/start"},
		{name: "stop", method: http.MethodPost, path: "/api/v1/tasks/1/stop"},
		{name: "annotate", method: http.MethodPost, path: "/api/v1/tasks/1/annotations", body: `{"description":"note"}`},
		{name: "denotate", method: http.MethodDelete, path: "/api/v1/tasks/1/annotations/1"},
		{name: "urgency", method: http.MethodGet, path: "/api/v1/tasks/1/urgency"},
		{name: "link list", method: http.MethodGet, path: "/api/v1/tasks/1/links"},
		{name: "link add", method: http.MethodPost, path: "/api/v1/tasks/1/links", body: `{"type":"document","url":"https://example.com"}`},
		{name: "link remove", method: http.MethodDelete, path: "/api/v1/tasks/1/links/link"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := requestHTTPBody(t, fixture.server, tc.method, tc.path, tc.body, headers)
			assertHTTPErrorCode(t, rr, http.StatusBadRequest, "task_ref_invalid")
		})
	}
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

func TestTaskLinkAddAndList(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.Add(app.AddInput{Description: "link test"})
	if err != nil {
		t.Fatal(err)
	}

	headers := map[string]string{
		"Authorization": "Bearer " + fixture.token,
		"Content-Type":  "application/json",
	}

	body := `{"type":"document","url":"https://example.com","title":"Example"}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/tasks/"+created.UUID+"/links", body, headers)
	if rr.Code != http.StatusCreated {
		t.Fatalf("add link status = %d body = %s", rr.Code, rr.Body.String())
	}

	rr = requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/tasks/"+created.UUID+"/links", headers)
	if rr.Code != http.StatusOK {
		t.Fatalf("list links status = %d body = %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "document") || !strings.Contains(rr.Body.String(), "https://example.com") {
		t.Fatalf("list links body = %s, want link data", rr.Body.String())
	}
}

func TestTaskLinkAddRejectsMissingType(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.Add(app.AddInput{Description: "link test"})
	if err != nil {
		t.Fatal(err)
	}

	headers := map[string]string{
		"Authorization": "Bearer " + fixture.token,
		"Content-Type":  "application/json",
	}

	body := `{"url":"https://example.com","title":"Example"}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/tasks/"+created.UUID+"/links", body, headers)
	assertHTTPErrorCode(t, rr, http.StatusBadRequest, "link_type_required")
}

func TestTaskLinkRemoveAndVerify(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.Add(app.AddInput{Description: "link test"})
	if err != nil {
		t.Fatal(err)
	}

	linkInfo, err := svc.TaskAddLink(created.UUID, "document", "https://example.com", "Example")
	if err != nil {
		t.Fatal(err)
	}

	headers := map[string]string{
		"Authorization": "Bearer " + fixture.token,
	}

	rr := requestHTTP(t, fixture.server, http.MethodDelete, "/api/v1/tasks/"+created.UUID+"/links/"+linkInfo.ID, headers)
	if rr.Code != http.StatusOK {
		t.Fatalf("remove link status = %d body = %s", rr.Code, rr.Body.String())
	}

	rr = requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/tasks/"+created.UUID+"/links", headers)
	if rr.Code != http.StatusOK {
		t.Fatalf("list links status = %d body = %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), linkInfo.ID) {
		t.Fatalf("link still present after removal: %s", rr.Body.String())
	}
}

func TestTaskLinkRemoveNotFound(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.Add(app.AddInput{Description: "link test"})
	if err != nil {
		t.Fatal(err)
	}

	headers := map[string]string{
		"Authorization": "Bearer " + fixture.token,
	}

	rr := requestHTTP(t, fixture.server, http.MethodDelete, "/api/v1/tasks/"+created.UUID+"/links/nonexistent-link-id", headers)
	assertHTTPErrorCode(t, rr, http.StatusBadRequest, "link_not_found")
}

// restfulFilterHeader 构造带 token 的认证头，供 restful 过滤测试复用。
func restfulFilterHeader(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

func TestHandleTaskList_RestfulStatusFilter(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	pending, err := svc.Add(app.AddInput{Description: "restful pending task"})
	if err != nil {
		t.Fatal(err)
	}
	completed, err := svc.Add(app.AddInput{Description: "restful completed task"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Done(completed.UUID); err != nil {
		t.Fatal(err)
	}
	_ = pending

	rr := requestHTTP(t, fixture.server, http.MethodGet,
		"/api/v1/tasks?status=pending&no_context=true", restfulFilterHeader(fixture.token))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "restful pending task") || strings.Contains(body, "restful completed task") {
		t.Fatalf("status filter failed: %s", body)
	}
}

func TestHandleTaskList_RestfulPriorityFilter(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	high := "H"
	if _, err := svc.Add(app.AddInput{Description: "restful high pri", Priority: &high}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(app.AddInput{Description: "restful no pri"}); err != nil {
		t.Fatal(err)
	}

	rr := requestHTTP(t, fixture.server, http.MethodGet,
		"/api/v1/tasks?priority=H&no_context=true", restfulFilterHeader(fixture.token))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "restful high pri") || strings.Contains(body, "restful no pri") {
		t.Fatalf("priority filter failed: %s", body)
	}
}

func TestHandleTaskList_RestfulDueFilters(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	past := int64(1)
	future := int64(1893456000) // 2030-01-01
	if _, err := svc.Add(app.AddInput{Description: "restful past due", Due: &past}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(app.AddInput{Description: "restful future due", Due: &future}); err != nil {
		t.Fatal(err)
	}
	hdr := restfulFilterHeader(fixture.token)

	rr := requestHTTP(t, fixture.server, http.MethodGet,
		"/api/v1/tasks?due_before=2025-01-01&no_context=true", hdr)
	if rr.Code != http.StatusOK {
		t.Fatalf("due_before status = %d body=%s", rr.Code, rr.Body.String())
	}
	if b := rr.Body.String(); !strings.Contains(b, "restful past due") || strings.Contains(b, "restful future due") {
		t.Fatalf("due_before filter failed: %s", b)
	}

	rr = requestHTTP(t, fixture.server, http.MethodGet,
		"/api/v1/tasks?due_after=2025-01-01&no_context=true", hdr)
	if rr.Code != http.StatusOK {
		t.Fatalf("due_after status = %d body=%s", rr.Code, rr.Body.String())
	}
	if b := rr.Body.String(); !strings.Contains(b, "restful future due") || strings.Contains(b, "restful past due") {
		t.Fatalf("due_after filter failed: %s", b)
	}
}

func TestHandleTaskList_RestfulQFilter(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(app.AddInput{Description: "needle in haystack"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(app.AddInput{Description: "completely unrelated"}); err != nil {
		t.Fatal(err)
	}

	rr := requestHTTP(t, fixture.server, http.MethodGet,
		"/api/v1/tasks?q=needle&no_context=true", restfulFilterHeader(fixture.token))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "needle in haystack") || strings.Contains(body, "completely unrelated") {
		t.Fatalf("q filter failed: %s", body)
	}
}

func TestHandleTaskList_RestfulTagsFilter(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(app.AddInput{Description: "restful tagged", Tags: []string{"web"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(app.AddInput{Description: "restful untagged"}); err != nil {
		t.Fatal(err)
	}

	rr := requestHTTP(t, fixture.server, http.MethodGet,
		"/api/v1/tasks?tags=web&no_context=true", restfulFilterHeader(fixture.token))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "restful tagged") || strings.Contains(body, "restful untagged") {
		t.Fatalf("tags filter failed: %s", body)
	}
}

func TestHandleTaskList_RestfulAssigneeFilter(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(app.AddInput{Description: "restful assigned to me", Assignees: []string{"local"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(app.AddInput{Description: "restful unassigned"}); err != nil {
		t.Fatal(err)
	}

	// assignee=me 会由 app 层解析为当前 actor（fixture token 即 workspace "local" 成员）。
	rr := requestHTTP(t, fixture.server, http.MethodGet,
		"/api/v1/tasks?assignee=me&no_context=true", restfulFilterHeader(fixture.token))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "restful assigned to me") || strings.Contains(body, "restful unassigned") {
		t.Fatalf("assignee filter failed: %s", body)
	}
}

func TestHandleTaskList_RestfulBadDate(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read")
	rr := requestHTTP(t, fixture.server, http.MethodGet,
		"/api/v1/tasks?due_after=not-a-date", restfulFilterHeader(fixture.token))
	assertHTTPErrorCode(t, rr, http.StatusBadRequest, "api_bad_filter")
}

func TestHandleTaskAnnotationListPagination(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.Add(app.AddInput{Description: "annotated for list"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err := svc.Annotate(created.UUID, "ann-"+string(rune('a'+i))); err != nil {
			t.Fatal(err)
		}
	}
	hdr := restfulFilterHeader(fixture.token)

	// 第一页 limit=2。
	rr := requestHTTP(t, fixture.server, http.MethodGet,
		"/api/v1/tasks/"+created.UUID+"/annotations?offset=0&limit=2", hdr)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var page struct {
		Data struct {
			Annotations []map[string]any `json:"annotations"`
			Total       int              `json:"total"`
			Offset      int              `json:"offset"`
			Limit       int              `json:"limit"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(rr.Body.String()), &page); err != nil {
		t.Fatal(err)
	}
	if page.Data.Total != 5 || len(page.Data.Annotations) != 2 || page.Data.Offset != 0 || page.Data.Limit != 2 {
		t.Fatalf("page1 = %+v", page.Data)
	}

	// 第二页 offset=2 limit=2。
	rr = requestHTTP(t, fixture.server, http.MethodGet,
		"/api/v1/tasks/"+created.UUID+"/annotations?offset=2&limit=2", hdr)
	if rr.Code != http.StatusOK {
		t.Fatalf("page2 status = %d body=%s", rr.Code, rr.Body.String())
	}
	if err := json.Unmarshal([]byte(rr.Body.String()), &page); err != nil {
		t.Fatal(err)
	}
	if page.Data.Total != 5 || len(page.Data.Annotations) != 2 || page.Data.Offset != 2 {
		t.Fatalf("page2 = %+v", page.Data)
	}

	// 第三页 offset=4 limit=2，只剩 1 条。
	rr = requestHTTP(t, fixture.server, http.MethodGet,
		"/api/v1/tasks/"+created.UUID+"/annotations?offset=4&limit=2", hdr)
	if err := json.Unmarshal([]byte(rr.Body.String()), &page); err != nil {
		t.Fatal(err)
	}
	if page.Data.Total != 5 || len(page.Data.Annotations) != 1 {
		t.Fatalf("page3 = %+v", page.Data)
	}
}

func TestHandleTaskAnnotationListBadLimit(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.Add(app.AddInput{Description: "x"})
	if err != nil {
		t.Fatal(err)
	}
	rr := requestHTTP(t, fixture.server, http.MethodGet,
		"/api/v1/tasks/"+created.UUID+"/annotations?limit=101", restfulFilterHeader(fixture.token))
	assertHTTPErrorCode(t, rr, http.StatusBadRequest, "api_bad_limit")
}
