package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

// helper: 创建 workspace hook 并返回 hook ID
func createTestHook(t *testing.T, scoped *app.Service, name string) app.HookView {
	t.Helper()
	createHTTPTestSink(t, scoped, "hook-sink")
	hook, err := scoped.AddHook(app.HookAddInput{
		Name:       name,
		ScopeType:  app.HookScopeWorkspace,
		EventTypes: []string{"task.created"},
		SinkRef:    "hook-sink",
	})
	if err != nil {
		t.Fatal(err)
	}
	return hook
}

// helper: 创建 project hook 并返回 hook ID
func createTestProjectHook(t *testing.T, scoped *app.Service, projectRef string) app.HookView {
	t.Helper()
	createHTTPTestSink(t, scoped, "hook-sink")
	hook, err := scoped.AddHook(app.HookAddInput{
		Name:       "proj-hook",
		ScopeType:  app.HookScopeProject,
		ProjectRef: projectRef,
		EventTypes: []string{"task.created"},
		SinkRef:    "hook-sink",
	})
	if err != nil {
		t.Fatal(err)
	}
	return hook
}

func createHTTPTestSink(t *testing.T, svc *app.Service, name string) app.NotificationSinkView {
	t.Helper()
	existing, err := svc.ListNotificationSinks(true)
	if err == nil {
		for _, sink := range existing {
			if sink.Name == name {
				return sink
			}
		}
	}
	sink, err := svc.AddNotificationSink(app.NotificationSinkAddInput{
		Name:         name,
		Type:         app.NotificationSinkTypeWebhook,
		EndpointMode: app.NotificationEndpointStaticURL,
		URL:          "https://example.com/webhook",
		AllowedHosts: []string{"example.com"},
		Secret:       "s3cret",
	})
	if err != nil {
		t.Fatalf("AddNotificationSink(%s) error = %v", name, err)
	}
	return sink
}

// helper: 创建 dead-lettered delivery
func createDeadLetteredDelivery(t *testing.T, store *storage.Store, hookID string) storage.HookDelivery {
	t.Helper()
	delivery := storage.HookDelivery{
		ID:           "delivery-001",
		HookID:       hookID,
		EventID:      "event-001",
		EventType:    "task.created",
		WorkspaceID:  getFirstWorkspaceID(t, store),
		ActorUserID:  getFirstUserID(t, store),
		PayloadJSON:  `{"task":{"uuid":"t1"}}`,
		HeadersJSON:  `{"X-Xuanchu-Signature":"sha256=abc"}`,
		Status:       storage.DeliveryStatusDeadLettered,
		AttemptCount: 3,
		LastError:    "connection refused",
		CreatedAt:    1000,
		ModifiedAt:   1000,
	}
	repo := storage.NewHookDeliveryRepository(store.DB())
	if err := repo.Enqueue([]storage.HookDelivery{delivery}); err != nil {
		t.Fatal(err)
	}
	return delivery
}

func getFirstWorkspaceID(t *testing.T, store *storage.Store) string {
	t.Helper()
	var ws storage.Workspace
	if err := store.DB().First(&ws).Error; err != nil {
		t.Fatal(err)
	}
	return ws.ID
}

func getFirstUserID(t *testing.T, store *storage.Store) string {
	t.Helper()
	var user storage.User
	if err := store.DB().First(&user).Error; err != nil {
		t.Fatal(err)
	}
	return user.ID
}

func TestHookCreateWorkspaceHook(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "hook:write", "hook:read")
	auth := map[string]string{"Authorization": "Bearer " + fixture.token, "Content-Type": "application/json"}

	body := `{"name":"my-hook","scope_type":"workspace","event_types":["task.created","task.completed"],"sink":"hook-sink"}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/hooks", body, auth)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data struct {
			ID         string   `json:"id"`
			Name       string   `json:"name"`
			ScopeType  string   `json:"scope_type"`
			EventTypes []string `json:"event_types"`
			SinkID     string   `json:"sink_id"`
			Enabled    bool     `json:"enabled"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.Name != "my-hook" {
		t.Fatalf("name = %q, want my-hook", resp.Data.Name)
	}
	if resp.Data.ScopeType != "workspace" {
		t.Fatalf("scope_type = %q, want workspace", resp.Data.ScopeType)
	}
	if len(resp.Data.EventTypes) != 2 {
		t.Fatalf("event_types = %v, want 2", resp.Data.EventTypes)
	}
	if !resp.Data.Enabled {
		t.Fatalf("enabled = false, want true")
	}
	// 验证响应不包含 secret
	if strings.Contains(rr.Body.String(), "hunter2") {
		t.Fatalf("response contains secret: %s", rr.Body.String())
	}
	assertSnakeCaseResponse(t, rr.Body.String())
}

func TestHookCreateProjectHook(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "hook:write", "hook:read", "project:write", "project:read")
	auth := map[string]string{"Authorization": "Bearer " + fixture.token, "Content-Type": "application/json"}

	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	project, err := svc.AddProject(app.AddProjectInput{Slug: "myproj", Name: "My Project"})
	if err != nil {
		t.Fatal(err)
	}

	body := `{"name":"proj-hook","scope_type":"project","project_ref":"myproj","event_types":["task.created"],"sink":"hook-sink"}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/hooks", body, auth)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data struct {
			ID        string  `json:"id"`
			ScopeType string  `json:"scope_type"`
			ProjectID *string `json:"project_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.ScopeType != "project" {
		t.Fatalf("scope_type = %q, want project", resp.Data.ScopeType)
	}
	if resp.Data.ProjectID == nil || *resp.Data.ProjectID != project.ID {
		t.Fatalf("project_id = %v, want %s", resp.Data.ProjectID, project.ID)
	}
}

func TestHookListNoSecretInResponse(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "hook:write", "hook:read")
	auth := map[string]string{"Authorization": "Bearer " + fixture.token, "Content-Type": "application/json"}

	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	createTestHook(t, svc, "secret-hook")

	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/hooks", auth)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "s3cret") {
		t.Fatalf("hook list contains secret: %s", rr.Body.String())
	}
}

func TestHookInfoReturnsHookDetails(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "hook:write", "hook:read")
	auth := map[string]string{"Authorization": "Bearer " + fixture.token, "Content-Type": "application/json"}

	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	hook := createTestHook(t, svc, "info-hook")

	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/hooks/"+hook.ID, auth)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.Name != "info-hook" {
		t.Fatalf("name = %q, want info-hook", resp.Data.Name)
	}
}

func TestHookModify(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "hook:write", "hook:read")
	auth := map[string]string{"Authorization": "Bearer " + fixture.token, "Content-Type": "application/json"}

	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	hook := createTestHook(t, svc, "modify-hook")

	body := `{"name":"modified-hook","timeout_seconds":30}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPatch, "/api/v1/hooks/"+hook.ID, body, auth)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data struct {
			Name           string `json:"name"`
			TimeoutSeconds int    `json:"timeout_seconds"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.Name != "modified-hook" {
		t.Fatalf("name = %q, want modified-hook", resp.Data.Name)
	}
	if resp.Data.TimeoutSeconds != 30 {
		t.Fatalf("timeout_seconds = %d, want 30", resp.Data.TimeoutSeconds)
	}
}

func TestHookModifyAuthenticatesBeforeDecodingBody(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "hook:write", "hook:read")

	rr := requestHTTPBody(t, fixture.server, http.MethodPatch, "/api/v1/hooks/some-hook", "{", map[string]string{
		"Content-Type": "application/json",
	})
	assertHTTPErrorCode(t, rr, http.StatusUnauthorized, "auth_missing_token")
}

func TestHookEnableDisable(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "hook:write", "hook:read")
	auth := map[string]string{"Authorization": "Bearer " + fixture.token, "Content-Type": "application/json"}

	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	hook := createTestHook(t, svc, "toggle-hook")

	// disable
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/hooks/"+hook.ID+"/disable", "", auth)
	if rr.Code != http.StatusOK {
		t.Fatalf("disable status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data struct {
			Enabled bool `json:"enabled"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.Enabled {
		t.Fatalf("enabled = true after disable")
	}

	// enable
	rr = requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/hooks/"+hook.ID+"/enable", "", auth)
	if rr.Code != http.StatusOK {
		t.Fatalf("enable status = %d body=%s", rr.Code, rr.Body.String())
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Data.Enabled {
		t.Fatalf("enabled = false after enable")
	}
}

func TestHookDelete(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "hook:write", "hook:read")
	auth := map[string]string{"Authorization": "Bearer " + fixture.token, "Content-Type": "application/json"}

	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	hook := createTestHook(t, svc, "delete-hook")

	rr := requestHTTP(t, fixture.server, http.MethodDelete, "/api/v1/hooks/"+hook.ID, auth)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d body=%s", rr.Code, rr.Body.String())
	}

	// 验证已删除
	rr = requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/hooks/"+hook.ID, auth)
	assertHTTPErrorCode(t, rr, http.StatusNotFound, "hook_not_found")
}

func TestHookWriteRequiresHookWriteScope(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "hook:read")
	auth := map[string]string{"Authorization": "Bearer " + fixture.token, "Content-Type": "application/json"}

	body := `{"name":"fail","scope_type":"workspace","event_types":["task.created"],"sink":"hook-sink"}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/hooks", body, auth)
	assertHTTPErrorCode(t, rr, http.StatusForbidden, "token_scope_denied")
}

func TestHookWriteRequiresPermission(t *testing.T) {
	store := openHTTPTestStore(t)
	ownerSvc, err := app.NewService(app.ServiceOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	viewer, err := ownerSvc.AddUser(app.AddUserInput{Name: "viewer"})
	if err != nil {
		t.Fatal(err)
	}
	if err := ownerSvc.AddMember(app.AddMemberInput{WorkspaceRef: "local", UserRef: viewer.ID, Role: app.RoleViewer}); err != nil {
		t.Fatal(err)
	}
	viewerToken, err := ownerSvc.CreateToken(app.CreateTokenInput{
		Name:          "viewer",
		UserRef:       viewer.ID,
		Scopes:        []string{"hook:write"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(Options{Store: store})

	auth := map[string]string{"Authorization": "Bearer " + viewerToken.RawToken, "Content-Type": "application/json"}
	body := `{"name":"fail","scope_type":"workspace","event_types":["task.created"],"sink":"hook-sink"}`
	rr := requestHTTPBody(t, srv, http.MethodPost, "/api/v1/hooks", body, auth)
	assertHTTPErrorCode(t, rr, http.StatusForbidden, "permission_denied")
}

func TestHookDeliveryListWithStatusFilter(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "hook:write", "hook:read")
	auth := map[string]string{"Authorization": "Bearer " + fixture.token}

	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	hook := createTestHook(t, svc, "delivery-hook")

	// 手动插入一条 dead-lettered delivery
	createDeadLetteredDelivery(t, fixture.server.store, hook.ID)

	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/hooks/"+hook.ID+"/deliveries?status=dead_lettered", auth)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			HookID string `json:"hook_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("deliveries = %d, want 1; body=%s", len(resp.Data), rr.Body.String())
	}
	if resp.Data[0].Status != "dead_lettered" {
		t.Fatalf("status = %q, want dead_lettered", resp.Data[0].Status)
	}
	assertSnakeCaseResponse(t, rr.Body.String())
}

func TestHookDeliveryInfo(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "hook:write", "hook:read")
	auth := map[string]string{"Authorization": "Bearer " + fixture.token}

	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	hook := createTestHook(t, svc, "delivery-info-hook")
	delivery := createDeadLetteredDelivery(t, fixture.server.store, hook.ID)

	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/hook-deliveries/"+delivery.ID, auth)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data struct {
			ID           string `json:"id"`
			HookID       string `json:"hook_id"`
			Status       string `json:"status"`
			EventType    string `json:"event_type"`
			AttemptCount int    `json:"attempt_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.Status != "dead_lettered" {
		t.Fatalf("status = %q, want dead_lettered", resp.Data.Status)
	}
	if resp.Data.AttemptCount != 3 {
		t.Fatalf("attempt_count = %d, want 3", resp.Data.AttemptCount)
	}
}

func TestHookDeliveryReplayDeadLettered(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "hook:write", "hook:read")
	auth := map[string]string{"Authorization": "Bearer " + fixture.token}

	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	hook := createTestHook(t, svc, "replay-hook")
	delivery := createDeadLetteredDelivery(t, fixture.server.store, hook.ID)

	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/hook-deliveries/"+delivery.ID+"/replay", "", auth)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.Status != "queued" {
		t.Fatalf("status = %q after replay, want queued", resp.Data.Status)
	}
}

func TestHookDeliveryReplayWritesAudit(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "hook:write", "hook:read", "audit:read")
	auth := map[string]string{"Authorization": "Bearer " + fixture.token}

	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	hook := createTestHook(t, svc, "audit-replay-hook")
	delivery := createDeadLetteredDelivery(t, fixture.server.store, hook.ID)

	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/hook-deliveries/"+delivery.ID+"/replay", "", auth)
	if rr.Code != http.StatusOK {
		t.Fatalf("replay status = %d body=%s", rr.Code, rr.Body.String())
	}

	// 检查审计日志
	rr = requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/audit", auth)
	if rr.Code != http.StatusOK {
		t.Fatalf("audit status = %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "hook.replay") {
		t.Fatalf("audit missing hook.replay action: %s", rr.Body.String())
	}
}

func TestHookDeliveryReplayNonReplayableRejected(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "hook:write", "hook:read")
	auth := map[string]string{"Authorization": "Bearer " + fixture.token}

	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	hook := createTestHook(t, svc, "nonreplay-hook")

	// 创建一个 succeeded delivery（不可重试）
	wsID := getFirstWorkspaceID(t, fixture.server.store)
	userID := getFirstUserID(t, fixture.server.store)
	delivery := storage.HookDelivery{
		ID:           "delivery-succeeded",
		HookID:       hook.ID,
		EventID:      "event-002",
		EventType:    "task.created",
		WorkspaceID:  wsID,
		ActorUserID:  userID,
		PayloadJSON:  `{}`,
		HeadersJSON:  `{}`,
		Status:       storage.DeliveryStatusSucceeded,
		AttemptCount: 1,
		CreatedAt:    1000,
		ModifiedAt:   1000,
	}
	repo := storage.NewHookDeliveryRepository(fixture.server.store.DB())
	if err := repo.Enqueue([]storage.HookDelivery{delivery}); err != nil {
		t.Fatal(err)
	}

	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/hook-deliveries/"+delivery.ID+"/replay", "", auth)
	assertHTTPErrorCode(t, rr, http.StatusBadRequest, "hook_delivery_not_replayable")
}

func TestHookDeliveryInfoNotFound(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "hook:read")
	auth := map[string]string{"Authorization": "Bearer " + fixture.token}

	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/hook-deliveries/nonexistent-id", auth)
	assertHTTPErrorCode(t, rr, http.StatusNotFound, "hook_delivery_not_found")
}

func TestHookDeliveryInfoRespectsProjectTokenScope(t *testing.T) {
	store := openHTTPTestStore(t)
	ownerSvc, err := app.NewService(app.ServiceOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	projectA, err := ownerSvc.AddProject(app.AddProjectInput{Slug: "alpha", Name: "Alpha"})
	if err != nil {
		t.Fatal(err)
	}
	projectB, err := ownerSvc.AddProject(app.AddProjectInput{Slug: "beta", Name: "Beta"})
	if err != nil {
		t.Fatal(err)
	}
	createHTTPTestSink(t, ownerSvc, "hook-sink")
	hook, err := ownerSvc.AddHook(app.HookAddInput{
		Name:       "beta-hook",
		ScopeType:  app.HookScopeProject,
		ProjectRef: projectB.ID,
		EventTypes: []string{"task.created"},
		SinkRef:    "hook-sink",
	})
	if err != nil {
		t.Fatal(err)
	}
	delivery := storage.HookDelivery{
		ID:           "delivery-beta",
		HookID:       hook.ID,
		EventID:      "event-beta",
		EventType:    "task.created",
		WorkspaceID:  projectB.WorkspaceID,
		ProjectID:    &projectB.ID,
		ActorUserID:  getFirstUserID(t, store),
		PayloadJSON:  `{}`,
		HeadersJSON:  `{}`,
		Status:       storage.DeliveryStatusDeadLettered,
		AttemptCount: 1,
		CreatedAt:    1000,
		ModifiedAt:   1000,
	}
	if err := storage.NewHookDeliveryRepository(store.DB()).Enqueue([]storage.HookDelivery{delivery}); err != nil {
		t.Fatal(err)
	}
	token, err := ownerSvc.CreateToken(app.CreateTokenInput{
		Name:          "alpha-hook-token",
		Scopes:        []string{"hook:read", "hook:write"},
		WorkspaceRefs: []string{"local"},
		ProjectRefs:   []string{projectA.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(Options{Store: store})
	auth := map[string]string{"Authorization": "Bearer " + token.RawToken}

	rr := requestHTTP(t, srv, http.MethodGet, "/api/v1/hook-deliveries/"+delivery.ID, auth)
	assertHTTPErrorCode(t, rr, http.StatusNotFound, "hook_delivery_not_found")

	rr = requestHTTPBody(t, srv, http.MethodPost, "/api/v1/hook-deliveries/"+delivery.ID+"/replay", "", auth)
	assertHTTPErrorCode(t, rr, http.StatusNotFound, "hook_delivery_not_found")
}

func TestProjectScopedTokenCannotCreateWorkspaceHook(t *testing.T) {
	store := openHTTPTestStore(t)
	ownerSvc, err := app.NewService(app.ServiceOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	project, err := ownerSvc.AddProject(app.AddProjectInput{Slug: "alpha", Name: "Alpha"})
	if err != nil {
		t.Fatal(err)
	}
	token, err := ownerSvc.CreateToken(app.CreateTokenInput{
		Name:          "project-hook-token",
		Scopes:        []string{"hook:write", "hook:read"},
		WorkspaceRefs: []string{"local"},
		ProjectRefs:   []string{project.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(Options{Store: store})
	body := `{"name":"wide-hook","scope_type":"workspace","event_types":["task.created"],"sink":"hook-sink"}`
	rr := requestHTTPBody(t, srv, http.MethodPost, "/api/v1/hooks", body, map[string]string{
		"Authorization": "Bearer " + token.RawToken,
		"Content-Type":  "application/json",
	})
	assertHTTPErrorCode(t, rr, http.StatusForbidden, "project_scope_denied")
}

func TestHookInfoNotFound(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "hook:read")
	auth := map[string]string{"Authorization": "Bearer " + fixture.token}

	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/hooks/nonexistent-id", auth)
	assertHTTPErrorCode(t, rr, http.StatusNotFound, "hook_not_found")
}

func TestHookCreateRejectsInvalidEventType(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "hook:write")
	auth := map[string]string{"Authorization": "Bearer " + fixture.token, "Content-Type": "application/json"}

	body := `{"name":"bad-event","scope_type":"workspace","event_types":["invalid.event"],"sink":"hook-sink"}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/hooks", body, auth)
	assertHTTPErrorCode(t, rr, http.StatusBadRequest, "hook_event_types_invalid")
}

func TestHookCreateRejectsUnknownSink(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "hook:write")
	auth := map[string]string{"Authorization": "Bearer " + fixture.token, "Content-Type": "application/json"}

	body := `{"name":"bad-url","scope_type":"workspace","event_types":["task.created"],"sink":"missing-sink"}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/hooks", body, auth)
	assertHTTPErrorCode(t, rr, http.StatusNotFound, "notification_sink_not_found")
}

func TestHookCreateRejectsDirectURL(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "hook:write")
	auth := map[string]string{"Authorization": "Bearer " + fixture.token, "Content-Type": "application/json"}

	body := `{"name":"bad-url","scope_type":"workspace","event_types":["task.created"],"endpoint_url":"https://example.com/hook"}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/hooks", body, auth)
	assertHTTPErrorCode(t, rr, http.StatusBadRequest, "hook_url_not_supported")
}
