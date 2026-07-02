package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

type httpTokenFixture struct {
	server *Server
	token  string
	id     string
}

func newHTTPServerWithTokenFixture(t *testing.T, scopes ...string) httpTokenFixture {
	t.Helper()
	store := openHTTPTestStore(t)
	svc, err := app.NewService(app.ServiceOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	createHTTPTestSink(t, svc, "hook-sink")
	created, err := svc.CreateToken(app.CreateTokenInput{
		Name:          "http-test",
		Scopes:        scopes,
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return httpTokenFixture{
		server: NewServer(Options{Store: store}),
		token:  created.RawToken,
		id:     created.View.ID,
	}
}

func requestHTTP(t *testing.T, srv *Server, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	return requestHTTPBody(t, srv, method, path, "", headers)
}

func requestHTTPBody(t *testing.T, srv *Server, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, req)
	return rr
}

func TestAuthRequiresBearerHeader(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t)
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/me", nil)
	assertHTTPErrorCode(t, rr, http.StatusUnauthorized, "auth_missing_token")

	rr = requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/me?token=bad", nil)
	assertHTTPErrorCode(t, rr, http.StatusUnauthorized, "auth_missing_token")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: "bad"})
	rr = httptest.NewRecorder()
	fixture.server.Router().ServeHTTP(rr, req)
	assertHTTPErrorCode(t, rr, http.StatusUnauthorized, "auth_missing_token")

	rr = requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/me", map[string]string{"Authorization": "Basic abc"})
	assertHTTPErrorCode(t, rr, http.StatusUnauthorized, "auth_missing_token")

	rr = requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/me", map[string]string{"Authorization": "bearer " + fixture.token})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestMeReturnsActorTokenAndWorkspace(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read")
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/me", map[string]string{"Authorization": "Bearer " + fixture.token})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Data struct {
			Actor struct {
				ID          string          `json:"id"`
				Name        string          `json:"name"`
				Email       *string         `json:"email"`
				ExternalIDs json.RawMessage `json:"external_ids"`
			} `json:"actor"`
			Token struct {
				Type string `json:"type"`
			} `json:"token"`
			EffectiveWorkspace struct {
				Slug string `json:"slug"`
			} `json:"effective_workspace"`
			EffectiveRole string `json:"effective_role"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.Actor.Name != "local" || payload.Data.Actor.ID == "" {
		t.Fatalf("actor = %#v body=%s", payload.Data.Actor, rr.Body.String())
	}
	if payload.Data.Actor.ExternalIDs == nil {
		t.Fatalf("actor.external_ids missing: %s", rr.Body.String())
	}
	if payload.Data.Token.Type != "pat" {
		t.Fatalf("token.type = %q, want pat", payload.Data.Token.Type)
	}
	if payload.Data.EffectiveWorkspace.Slug != "local" {
		t.Fatalf("effective_workspace.slug = %q, want local", payload.Data.EffectiveWorkspace.Slug)
	}
	if payload.Data.EffectiveRole != "owner" {
		t.Fatalf("effective_role = %q, want owner", payload.Data.EffectiveRole)
	}
}

func TestCredentialsCurrentReturnsTenantSystemOwner(t *testing.T) {
	store := openHTTPTestStore(t)
	svc, err := app.NewService(app.ServiceOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateTenantAccessToken(app.CreateTenantAccessTokenInput{
		Name:   "runtime",
		Scopes: []string{"*"},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(Options{Store: store})
	rr := requestHTTP(t, server, http.MethodGet, "/api/v1/credentials/current", map[string]string{
		"Authorization": "Bearer " + created.RawToken,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Data struct {
			ActorType string `json:"actor_type"`
			Actor     struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				DisplayName string `json:"display_name"`
			} `json:"actor"`
			Token struct {
				ID     string   `json:"id"`
				Name   string   `json:"name"`
				Type   string   `json:"type"`
				Prefix string   `json:"prefix"`
				Scopes []string `json:"scopes"`
			} `json:"token"`
			EffectiveWorkspace struct {
				ID   string `json:"id"`
				Slug string `json:"slug"`
				Name string `json:"name"`
			} `json:"effective_workspace"`
			EffectiveRole string   `json:"effective_role"`
			Capabilities  []string `json:"capabilities"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.ActorType != "tenant_access_token" {
		t.Fatalf("actor_type = %q body=%s", payload.Data.ActorType, rr.Body.String())
	}
	if payload.Data.Actor.ID != created.View.ID || payload.Data.Actor.Name != "runtime" {
		t.Fatalf("actor = %#v, token id = %q", payload.Data.Actor, created.View.ID)
	}
	if payload.Data.Actor.DisplayName != "系统身份 / runtime" {
		t.Fatalf("actor.display_name = %q", payload.Data.Actor.DisplayName)
	}
	if payload.Data.Token.Type != "tenant_access_token" || payload.Data.Token.Prefix == "" {
		t.Fatalf("token = %#v", payload.Data.Token)
	}
	if payload.Data.EffectiveWorkspace.Slug != "local" || payload.Data.EffectiveRole != "owner" {
		t.Fatalf("workspace=%#v role=%q", payload.Data.EffectiveWorkspace, payload.Data.EffectiveRole)
	}
	for _, want := range []string{"user:write", "member:write", "token:write", "workspace:write"} {
		if !slices.Contains(payload.Data.Capabilities, want) {
			t.Fatalf("capabilities missing %q: %#v", want, payload.Data.Capabilities)
		}
	}
	for _, forbidden := range []string{"hook:write", "notification:write", "reminder:write", "impersonate"} {
		if slices.Contains(payload.Data.Capabilities, forbidden) {
			t.Fatalf("capabilities include forbidden %q: %#v", forbidden, payload.Data.Capabilities)
		}
	}
}

func TestTenantTokenCanCallHTTPTaskAPI(t *testing.T) {
	store := openHTTPTestStore(t)
	svc, err := app.NewService(app.ServiceOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateTenantAccessToken(app.CreateTenantAccessTokenInput{
		Name:   "runtime",
		Scopes: []string{"task:write"},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(Options{Store: store})
	rr := requestHTTPBody(t, server, http.MethodPost, "/api/v1/tasks", `{"title":"from tenant http"}`, map[string]string{
		"Authorization": "Bearer " + created.RawToken,
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestTenantTokenCannotUseHTTPAssigneeMe(t *testing.T) {
	store := openHTTPTestStore(t)
	svc, err := app.NewService(app.ServiceOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateTenantAccessToken(app.CreateTenantAccessTokenInput{
		Name:   "runtime",
		Scopes: []string{"task:read", "task:write"},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(Options{Store: store})
	headers := map[string]string{"Authorization": "Bearer " + created.RawToken}

	rr := requestHTTP(t, server, http.MethodGet, "/api/v1/tasks?query=assignee:me", headers)
	assertHTTPErrorCode(t, rr, http.StatusBadRequest, "tenant_actor_not_user")

	rr = requestHTTPBody(t, server, http.MethodPost, "/api/v1/tasks", `{"title":"bad assignee","assignees":["me"]}`, headers)
	assertHTTPErrorCode(t, rr, http.StatusBadRequest, "tenant_actor_not_user")
}

func TestTenantTokenCannotCallHTTPMe(t *testing.T) {
	store := openHTTPTestStore(t)
	svc, err := app.NewService(app.ServiceOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateTenantAccessToken(app.CreateTenantAccessTokenInput{
		Name:   "runtime",
		Scopes: []string{"workspace:read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(Options{Store: store})
	rr := requestHTTP(t, server, http.MethodGet, "/api/v1/me", map[string]string{
		"Authorization": "Bearer " + created.RawToken,
	})
	assertHTTPErrorCode(t, rr, http.StatusBadRequest, "tenant_actor_not_user")
}

func TestTenantTokenCanManageHTTPUsersAndMembers(t *testing.T) {
	store := openHTTPTestStore(t)
	svc, err := app.NewService(app.ServiceOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateTenantAccessToken(app.CreateTenantAccessTokenInput{
		Name:   "runtime",
		Scopes: []string{"user:read", "user:write", "member:read", "member:write", "workspace:read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(Options{Store: store})
	headers := map[string]string{
		"Authorization": "Bearer " + created.RawToken,
		"Content-Type":  "application/json",
	}

	createUser := requestHTTPBody(t, server, http.MethodPost, "/api/v1/users", `{"name":"tenant-added","display_name":"Tenant Added"}`, headers)
	if createUser.Code != http.StatusCreated {
		t.Fatalf("create user status = %d body=%s", createUser.Code, createUser.Body.String())
	}

	listUsers := requestHTTP(t, server, http.MethodGet, "/api/v1/users", headers)
	if listUsers.Code != http.StatusOK || !strings.Contains(listUsers.Body.String(), "tenant-added") {
		t.Fatalf("list users status = %d body=%s", listUsers.Code, listUsers.Body.String())
	}

	addMember := requestHTTPBody(t, server, http.MethodPost, "/api/v1/workspaces/local/members", `{"user":"tenant-added","role":"member"}`, headers)
	if addMember.Code != http.StatusCreated {
		t.Fatalf("add member status = %d body=%s", addMember.Code, addMember.Body.String())
	}

	listMembers := requestHTTP(t, server, http.MethodGet, "/api/v1/workspaces/local/members", headers)
	if listMembers.Code != http.StatusOK || !strings.Contains(listMembers.Body.String(), "tenant-added") {
		t.Fatalf("list members status = %d body=%s", listMembers.Code, listMembers.Body.String())
	}
}

func TestTenantTokenCannotCallHTTPActiveContext(t *testing.T) {
	store := openHTTPTestStore(t)
	svc, err := app.NewService(app.ServiceOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateTenantAccessToken(app.CreateTenantAccessTokenInput{
		Name:   "runtime",
		Scopes: []string{"workspace:read", "context:read", "context:write"},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(Options{Store: store})
	headers := map[string]string{"Authorization": "Bearer " + created.RawToken}

	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/api/v1/contexts/none"},
		{method: http.MethodPost, path: "/api/v1/contexts/focus/use"},
	} {
		rr := requestHTTPBody(t, server, tc.method, tc.path, tc.body, headers)
		assertHTTPErrorCode(t, rr, http.StatusBadRequest, "tenant_actor_not_user")
	}
}

func TestTenantTokenContextListDoesNotReadActiveUserState(t *testing.T) {
	store := openHTTPTestStore(t)
	svc, err := app.NewService(app.ServiceOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.DefineContext("focus", "status:pending"); err != nil {
		t.Fatal(err)
	}
	if err := svc.UseContext("focus"); err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateTenantAccessToken(app.CreateTenantAccessTokenInput{
		Name:   "runtime",
		Scopes: []string{"context:read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(Options{Store: store})
	rr := requestHTTP(t, server, http.MethodGet, "/api/v1/contexts", map[string]string{
		"Authorization": "Bearer " + created.RawToken,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), `"active":true`) {
		t.Fatalf("tenant context list should not expose active user state: %s", rr.Body.String())
	}
}

func TestTenantTokenRejectsStoredDisallowedScopesAtAuthentication(t *testing.T) {
	store := openHTTPTestStore(t)
	svc, err := app.NewService(app.ServiceOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateTenantAccessToken(app.CreateTenantAccessTokenInput{
		Name:   "runtime",
		Scopes: []string{"task:read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DB().Model(&storage.ApiToken{}).
		Where("id = ?", created.View.ID).
		Update("scopes_json", `["`+auth.ScopeHookWrite+`"]`).Error; err != nil {
		t.Fatal(err)
	}
	_, err = svc.AuthenticateBearerToken(created.RawToken)
	if runtimeErr, ok := err.(app.RuntimeError); !ok || runtimeErr.Code != "auth_invalid_token" {
		t.Fatalf("AuthenticateBearerToken() error = %#v, want auth_invalid_token", err)
	}
}

func TestConfigEndpointDoesNotExposeOrWriteInternalMeta(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "config:read", "config:write")
	authHeader := map[string]string{"Authorization": "Bearer " + fixture.token}

	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/config", authHeader)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	for _, forbidden := range []string{"active_user_id", "active_workspace.", "active_context.", "database.path", "remote.token"} {
		if strings.Contains(rr.Body.String(), forbidden) {
			t.Fatalf("config response leaked %q: %s", forbidden, rr.Body.String())
		}
	}

	rr = requestHTTPBody(t, fixture.server, http.MethodPut, "/api/v1/config/active_user_id", `{"value":"evil"}`, authHeader)
	assertHTTPErrorCode(t, rr, http.StatusBadRequest, "config_scope_invalid")
	rr = requestHTTP(t, fixture.server, http.MethodDelete, "/api/v1/config/active_context.local.local", authHeader)
	assertHTTPErrorCode(t, rr, http.StatusBadRequest, "config_scope_invalid")
}

func TestWorkspaceScopeDeniedIsForbidden(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read")
	if err := fixture.server.store.DB().Exec(
		`UPDATE api_tokens SET workspace_ids_json = ? WHERE id = ?`,
		`["missing-workspace"]`,
		fixture.id,
	).Error; err != nil {
		t.Fatal(err)
	}

	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/me", map[string]string{"Authorization": "Bearer " + fixture.token})
	assertHTTPErrorCode(t, rr, http.StatusForbidden, "workspace_scope_denied")
}

func TestWorkspaceListRespectsTokenWorkspaceScope(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:read", "member:read")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	other := mustCreateHTTPWorkspace(t, fixture.server.store, storage.Workspace{
		ID:           "ws-other",
		Slug:         "other",
		Name:         "Other",
		Visibility:   "team",
		SettingsJSON: "{}",
		CreatedAt:    100,
		ModifiedAt:   100,
	})
	mustUpsertHTTPMembership(t, fixture.server.store, storage.Membership{
		UserID:      svc.Runtime().ActorUserID,
		WorkspaceID: other.ID,
		Role:        string(app.RoleOwner),
		JoinedAt:    100,
		ModifiedAt:  100,
	})

	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/workspaces", map[string]string{
		"Authorization": "Bearer " + fixture.token,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Data []struct {
			Slug string `json:"slug"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	slugs := make([]string, 0, len(payload.Data))
	for _, workspace := range payload.Data {
		slugs = append(slugs, workspace.Slug)
	}
	if !slices.Contains(slugs, "local") || slices.Contains(slugs, "other") {
		t.Fatalf("workspace list leaked outside token scope: %v body=%s", slugs, rr.Body.String())
	}
}

func TestWorkspaceAndMemberResponsesUseSnakeCase(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:read", "member:read")
	authHeader := map[string]string{"Authorization": "Bearer " + fixture.token}

	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/workspaces", authHeader)
	if rr.Code != http.StatusOK {
		t.Fatalf("workspace list status = %d body=%s", rr.Code, rr.Body.String())
	}
	var workspaces struct {
		Data []struct {
			Slug string `json:"slug"`
			Role string `json:"role"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &workspaces); err != nil {
		t.Fatal(err)
	}
	if len(workspaces.Data) == 0 || workspaces.Data[0].Slug == "" || workspaces.Data[0].Role == "" {
		t.Fatalf("workspace response missing snake_case fields: %s", rr.Body.String())
	}
	assertSnakeCaseResponse(t, rr.Body.String())

	rr = requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/workspaces/local/members", authHeader)
	if rr.Code != http.StatusOK {
		t.Fatalf("member list status = %d body=%s", rr.Code, rr.Body.String())
	}
	var members struct {
		Data []struct {
			UserID   string `json:"user_id"`
			JoinedAt int64  `json:"joined_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &members); err != nil {
		t.Fatal(err)
	}
	if len(members.Data) == 0 || members.Data[0].UserID == "" || members.Data[0].JoinedAt == 0 {
		t.Fatalf("member response missing snake_case fields: %s", rr.Body.String())
	}
	assertSnakeCaseResponse(t, rr.Body.String())
}

func TestConfigWriteRequiresRoleAndAuditsWorkspaceConfig(t *testing.T) {
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
		Scopes:        []string{"config:write"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(Options{Store: store})

	rr := requestHTTPBody(t, srv, http.MethodPut, "/api/v1/config/date.format", `{"value":"rfc3339"}`, map[string]string{
		"Authorization": "Bearer " + viewerToken.RawToken,
		"Content-Type":  "application/json",
	})
	assertHTTPErrorCode(t, rr, http.StatusForbidden, "permission_denied")

	ownerToken, err := ownerSvc.CreateToken(app.CreateTokenInput{
		Name:          "owner",
		Scopes:        []string{"config:write", "audit:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	rr = requestHTTPBody(t, srv, http.MethodPut, "/api/v1/config/date.format", `{"value":"rfc3339"}`, map[string]string{
		"Authorization": "Bearer " + ownerToken.RawToken,
		"Content-Type":  "application/json",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	rr = requestHTTP(t, srv, http.MethodGet, "/api/v1/audit", map[string]string{
		"Authorization": "Bearer " + ownerToken.RawToken,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("audit status = %d body=%s", rr.Code, rr.Body.String())
	}
	var audit struct {
		Data []struct {
			Action  string          `json:"action"`
			Payload json.RawMessage `json:"payload"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &audit); err != nil {
		t.Fatal(err)
	}
	for _, row := range audit.Data {
		if row.Action == "config.set" {
			var payload map[string]any
			if err := json.Unmarshal(row.Payload, &payload); err != nil {
				t.Fatalf("audit payload is not JSON: %s", row.Payload)
			}
			if _, ok := payload["value"]; ok {
				t.Fatalf("config.set audit leaked value field: %s", row.Payload)
			}
			if got, ok := payload["key"].(string); !ok || got != "date.format" {
				t.Fatalf("config.set audit missing key=date.format: %s", row.Payload)
			}
			return
		}
	}
	t.Fatalf("audit missing config.set: %s", rr.Body.String())
}

func TestAuditListRejectsInvalidLimit(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "audit:read")
	authHeader := map[string]string{"Authorization": "Bearer " + fixture.token}
	cases := []string{"0", "-1", "1001", "bad"}
	for _, limit := range cases {
		rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/audit?limit="+limit, authHeader)
		assertHTTPErrorCode(t, rr, http.StatusBadRequest, "api_bad_limit")
	}
}

func TestAuditListAcceptsEmptyLimitAndDefaults(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "audit:read")
	authHeader := map[string]string{"Authorization": "Bearer " + fixture.token}
	for _, path := range []string{"/api/v1/audit?limit=", "/api/v1/audit"} {
		rr := requestHTTP(t, fixture.server, http.MethodGet, path, authHeader)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status = %d body=%s", path, rr.Code, rr.Body.String())
		}
	}
}

func mustCreateHTTPWorkspace(t *testing.T, store *storage.Store, ws storage.Workspace) storage.Workspace {
	t.Helper()
	created, err := storage.NewWorkspaceRepository(store.DB()).Create(ws)
	if err != nil {
		t.Fatalf("Create(workspace %s) error = %v", ws.Slug, err)
	}
	return created
}

func mustUpsertHTTPMembership(t *testing.T, store *storage.Store, member storage.Membership) {
	t.Helper()
	if err := storage.NewMemberRepository(store.DB()).Upsert(member); err != nil {
		t.Fatalf("Upsert(membership %+v) error = %v", member, err)
	}
}
