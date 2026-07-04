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
		{name: "modify", method: http.MethodPatch, path: "/api/v1/tasks/api-1", body: `{"title":"updated"}`, wantStatus: http.StatusOK},
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
			created, err := svc.Add(app.AddInput{Title: "slug task", Project: &project.Slug})
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
		{name: "modify", method: http.MethodPatch, path: "/api/v1/tasks/1", body: `{"title":"updated"}`},
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

	body := `{"title":"mismatch","project":"alpha","project_id":"` + beta.ID + `"}`
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
	if _, err := workSvc.Add(app.AddInput{Title: "work task", Project: &project.Slug}); err != nil {
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
	if _, err := svc.Add(app.AddInput{Title: "next task", Tags: []string{"next"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(app.AddInput{Title: "plain task"}); err != nil {
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
	if _, err := svc.Add(app.AddInput{Title: "context task", Tags: []string{"ctx"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(app.AddInput{Title: "plain task"}); err != nil {
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

	body := `{"title":"deadline","due":1893456000}`
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

	body := `{"title":"assigned","assignees":["local"]}`
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
	created, err := svc.Add(app.AddInput{Title: "assigned later"})
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
	if _, err := svc.Add(app.AddInput{Title: "mine", Assignees: []string{"local"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(app.AddInput{Title: "plain"}); err != nil {
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
	created, err := svc.Add(app.AddInput{Title: "link test"})
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
	created, err := svc.Add(app.AddInput{Title: "link test"})
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
	created, err := svc.Add(app.AddInput{Title: "link test"})
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

func TestTaskLinkUpdate(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.Add(app.AddInput{Title: "link update"})
	if err != nil {
		t.Fatal(err)
	}
	linkInfo, err := svc.TaskAddLink(created.UUID, "document", "https://example.com/old", "Old")
	if err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{
		"Authorization": "Bearer " + fixture.token,
		"Content-Type":  "application/json",
	}

	body := `{"type":"spec","url":"https://example.com/spec","title":"Spec"}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPatch, "/api/v1/tasks/"+created.UUID+"/links/"+linkInfo.ID, body, headers)
	if rr.Code != http.StatusOK {
		t.Fatalf("update link status = %d body = %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"type":"spec"`) || !strings.Contains(rr.Body.String(), `"title":"Spec"`) {
		t.Fatalf("update link body = %s", rr.Body.String())
	}
}

func TestTaskLinkRemoveNotFound(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.Add(app.AddInput{Title: "link test"})
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
	pending, err := svc.Add(app.AddInput{Title: "restful pending task"})
	if err != nil {
		t.Fatal(err)
	}
	completed, err := svc.Add(app.AddInput{Title: "restful completed task"})
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
	if _, err := svc.Add(app.AddInput{Title: "restful high pri", Priority: &high}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(app.AddInput{Title: "restful no pri"}); err != nil {
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
	if _, err := svc.Add(app.AddInput{Title: "restful past due", Due: &past}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(app.AddInput{Title: "restful future due", Due: &future}); err != nil {
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
	if _, err := svc.Add(app.AddInput{Title: "needle in haystack"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(app.AddInput{Title: "completely unrelated"}); err != nil {
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
	if _, err := svc.Add(app.AddInput{Title: "restful tagged", Tags: []string{"web"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(app.AddInput{Title: "restful untagged"}); err != nil {
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
	if _, err := svc.Add(app.AddInput{Title: "restful assigned to me", Assignees: []string{"local"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(app.AddInput{Title: "restful unassigned"}); err != nil {
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
	created, err := svc.Add(app.AddInput{Title: "annotated for list"})
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

func TestHandleTaskAnnotationUpdate(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.Add(app.AddInput{Title: "annotation update"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Annotate(created.UUID, "old note"); err != nil {
		t.Fatal(err)
	}
	annotated, err := svc.Info(created.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if len(annotated.Annotations) != 1 || annotated.Annotations[0].ID == "" {
		t.Fatalf("annotations = %#v, want one annotation with id", annotated.Annotations)
	}
	headers := map[string]string{
		"Authorization": "Bearer " + fixture.token,
		"Content-Type":  "application/json",
	}

	body := `{"description":"updated note"}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPatch, "/api/v1/tasks/"+created.UUID+"/annotations/"+annotated.Annotations[0].ID, body, headers)
	if rr.Code != http.StatusOK {
		t.Fatalf("update annotation status = %d body = %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "updated note") || strings.Contains(rr.Body.String(), "old note") {
		t.Fatalf("update annotation body = %s", rr.Body.String())
	}
}

func TestHandleTaskAnnotationListBadLimit(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.Add(app.AddInput{Title: "x"})
	if err != nil {
		t.Fatal(err)
	}
	rr := requestHTTP(t, fixture.server, http.MethodGet,
		"/api/v1/tasks/"+created.UUID+"/annotations?limit=101", restfulFilterHeader(fixture.token))
	assertHTTPErrorCode(t, rr, http.StatusBadRequest, "api_bad_limit")
}

func TestHandleTaskInfoReturnsDependsInfo(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	proj, err := svc.AddProject(app.AddProjectInput{Slug: "api", Name: "API"})
	if err != nil {
		t.Fatal(err)
	}
	projectRef := proj.Slug
	dep, err := svc.Add(app.AddInput{Title: "dependency task", Project: &projectRef})
	if err != nil {
		t.Fatal(err)
	}
	// blockedTask 依赖 dep，因此 dep 的详情页应返回 blocked_by_info 含 blockedTask。
	if _, err := svc.Add(app.AddInput{Title: "blocked by dependency", Project: &projectRef, Depends: []string{dep.UUID}}); err != nil {
		t.Fatal(err)
	}
	// 主任务依赖 dep，验证返回体含 depends_info（可读描述 + task_slug）。
	main, err := svc.Add(app.AddInput{Title: "main task", Project: &projectRef, Depends: []string{dep.UUID}})
	if err != nil {
		t.Fatal(err)
	}

	// main 的详情：含 depends_info（指向 dep）。
	rr := requestHTTP(t, fixture.server, http.MethodGet,
		"/api/v1/tasks/"+main.UUID, restfulFilterHeader(fixture.token))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"depends_info"`) {
		t.Fatalf("response missing depends_info: %s", body)
	}
	if !strings.Contains(body, "dependency task") || !strings.Contains(body, "api-1") {
		t.Fatalf("depends_info not enriched with description/slug: %s", body)
	}

	// dep 的详情：含 blocked_by_info（被 main 和 blockedTask 阻塞）。
	rr = requestHTTP(t, fixture.server, http.MethodGet,
		"/api/v1/tasks/"+dep.UUID, restfulFilterHeader(fixture.token))
	if rr.Code != http.StatusOK {
		t.Fatalf("dep status = %d body=%s", rr.Code, rr.Body.String())
	}
	body = rr.Body.String()
	if !strings.Contains(body, `"blocked_by_info"`) {
		t.Fatalf("response missing blocked_by_info: %s", body)
	}
	if !strings.Contains(body, "main task") || !strings.Contains(body, "blocked by dependency") {
		t.Fatalf("blocked_by_info not enriched: %s", body)
	}
}

// TestTaskAuditUsesTaskReadScope 校验 /tasks/{ref}/audit 只要求 task:read。
func TestTaskAuditUsesTaskReadScope(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.Add(app.AddInput{Title: "before"})
	if err != nil {
		t.Fatal(err)
	}
	// 产生一条 task.modify audit（title change）。
	afterTitle := "after"
	if err := svc.Modify(created.UUID, app.ModifyInput{Title: &afterTitle}); err != nil {
		t.Fatal(err)
	}

	// 在同一 store 上创建一个只持 task:read 的 token，
	// 证明该接口不要求 task:write 或 audit:read。
	readOnly, err := svc.CreateToken(app.CreateTokenInput{
		Name:          "task-reader",
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatalf("CreateToken(read-only) error = %v", err)
	}
	headers := map[string]string{"Authorization": "Bearer " + readOnly.RawToken}
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/tasks/"+created.UUID+"/audit", headers)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"changes"`) {
		t.Fatalf("response missing changes: %s", body)
	}
	if !strings.Contains(body, `"payload"`) {
		t.Fatalf("response missing payload: %s", body)
	}
	if !strings.Contains(body, `"title"`) || !strings.Contains(body, "before") || !strings.Contains(body, "after") {
		t.Fatalf("response missing title change details: %s", body)
	}
}

// TestTaskAuditRejectsNumericRef 校验数字 task ref 仍返回 task_ref_invalid。
func TestTaskAuditRejectsNumericRef(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read")
	headers := map[string]string{"Authorization": "Bearer " + fixture.token}
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/tasks/1/audit", headers)
	assertHTTPErrorCode(t, rr, http.StatusBadRequest, "task_ref_invalid")
}

// TestTaskAuditClearDuePreservesNullRaw 校验清空 due 后
// 响应仍包含 current 的 raw: null。
func TestTaskAuditClearDuePreservesNullRaw(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	due := int64(1783036800)
	created, err := svc.Add(app.AddInput{Title: "task", Due: &due})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Modify(created.UUID, app.ModifyInput{ClearDue: true}); err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{"Authorization": "Bearer " + fixture.token}
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/tasks/"+created.UUID+"/audit", headers)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	// current 必须出现，且 raw 为 null，不能因为清空就把 key 省略。
	if !strings.Contains(body, `"current"`) {
		t.Fatalf("response missing current key: %s", body)
	}
	if !strings.Contains(body, `"raw":null`) {
		t.Fatalf("response missing raw:null: %s", body)
	}
}

// TestTaskAuditSetChangePreservesEmptyArrays 校验集合类 change
// 即使 added 或 removed 为空，也必须同时输出为 []，不能被 omitempty 丢掉。
func TestTaskAuditSetChangePreservesEmptyArrays(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.Add(app.AddInput{Title: "task", Tags: []string{"ads"}})
	if err != nil {
		t.Fatal(err)
	}
	// 只新增 tag、不移除：tags change 的 removed 应该是空数组。
	if err := svc.Modify(created.UUID, app.ModifyInput{AddTags: []string{"dashboard"}}); err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{"Authorization": "Bearer " + fixture.token}
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/tasks/"+created.UUID+"/audit", headers)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}

	var payload struct {
		Data []struct {
			Changes []struct {
				Field   string                   `json:"field"`
				Kind    string                   `json:"kind"`
				Added   []map[string]interface{} `json:"added"`
				Removed []map[string]interface{} `json:"removed"`
			} `json:"changes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rr.Body.String())
	}
	if len(payload.Data) == 0 || len(payload.Data[0].Changes) == 0 {
		t.Fatalf("no changes in response: %s", rr.Body.String())
	}
	var tagsChange *struct {
		Field   string                   `json:"field"`
		Kind    string                   `json:"kind"`
		Added   []map[string]interface{} `json:"added"`
		Removed []map[string]interface{} `json:"removed"`
	}
	for i, c := range payload.Data[0].Changes {
		if c.Field == "tags" {
			tagsChange = &payload.Data[0].Changes[i]
		}
	}
	if tagsChange == nil {
		t.Fatalf("tags change missing: %s", rr.Body.String())
	}
	if tagsChange.Kind != "set" {
		t.Fatalf("tags kind = %s, want set", tagsChange.Kind)
	}
	// removed 必须以空数组存在，而不是被 omitempty 省略。
	if tagsChange.Removed == nil {
		t.Fatalf("tags removed missing (omitempty dropped it): %s", rr.Body.String())
	}
	if len(tagsChange.Removed) != 0 {
		t.Fatalf("tags removed = %#v, want empty array", tagsChange.Removed)
	}
}
