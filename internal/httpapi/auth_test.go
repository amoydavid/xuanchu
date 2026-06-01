package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/storage/sqlite"
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
	body := rr.Body.String()
	for _, want := range []string{`"name":"local"`, `"type":"pat"`, `"slug":"local"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %s, want %s", body, want)
		}
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
	fixture := newHTTPServerWithTokenFixture(t, "workspace:read")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	other := mustCreateHTTPWorkspace(t, fixture.server.store, sqlite.Workspace{
		ID:           "ws-other",
		Slug:         "other",
		Name:         "Other",
		Visibility:   "team",
		SettingsJSON: "{}",
		CreatedAt:    100,
		ModifiedAt:   100,
	})
	mustUpsertHTTPMembership(t, fixture.server.store, sqlite.Membership{
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
	fixture := newHTTPServerWithTokenFixture(t, "workspace:read")
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

func mustCreateHTTPWorkspace(t *testing.T, store *sqlite.Store, ws sqlite.Workspace) sqlite.Workspace {
	t.Helper()
	created, err := sqlite.NewWorkspaceRepository(store.DB()).Create(ws)
	if err != nil {
		t.Fatalf("Create(workspace %s) error = %v", ws.Slug, err)
	}
	return created
}

func mustUpsertHTTPMembership(t *testing.T, store *sqlite.Store, member sqlite.Membership) {
	t.Helper()
	if err := sqlite.NewMemberRepository(store.DB()).Upsert(member); err != nil {
		t.Fatalf("Upsert(membership %+v) error = %v", member, err)
	}
}
