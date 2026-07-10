package httpapi

import (
	"bytes"
	"encoding/base64"
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

// httpTestSecretKeyBase64 返回一个固定的 32 字节 base64 secret key，供 token reveal 测试复用。
// Web Console 创建可恢复 token 要求服务端配置 [security].config_secret_key。
func httpTestSecretKeyBase64() string {
	return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x07}, 32))
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
		server: NewServer(Options{Store: store, ConfigSecretKey: httpTestSecretKeyBase64()}),
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

func httpResponseDataMap(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, rr.Body.String())
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("data type = %T body=%s", payload["data"], rr.Body.String())
	}
	return data
}

func assertHTTPSystemActor(t *testing.T, value any, tokenID string) {
	t.Helper()
	actor, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("actor type = %T, want object", value)
	}
	if actor["type"] != "tenant_access_token" {
		t.Fatalf("actor.type = %v, want tenant_access_token", actor["type"])
	}
	token, ok := actor["token"].(map[string]any)
	if !ok {
		t.Fatalf("actor.token type = %T, want object", actor["token"])
	}
	if token["id"] != tokenID || token["name"] != "runtime-p2" {
		t.Fatalf("actor.token = %#v, want id=%q name=runtime-p2", token, tokenID)
	}
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
	for _, want := range []string{"hook:write", "notification:write", "reminder:write"} {
		if !slices.Contains(payload.Data.Capabilities, want) {
			t.Fatalf("capabilities missing P2 scope %q: %#v", want, payload.Data.Capabilities)
		}
	}
	for _, forbidden := range []string{"impersonate"} {
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
	workspaces, err := storage.NewWorkspaceRepository(store.DB()).ListAll(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(workspaces) != 1 || workspaces[0].Slug != "local" {
		t.Fatalf("tenant user_add created unexpected workspace(s): %#v", workspaces)
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

func TestHTTPMemberPatchCanUpdateDisplayNameAndRole(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:read", "member:read", "member:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	user, err := svc.AddUser(app.AddUserInput{Name: "http-member-patch", DisplayName: "Old Name"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.AddMember(app.AddMemberInput{WorkspaceRef: "local", UserRef: user.Name, Role: app.RoleMember}); err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{
		"Authorization": "Bearer " + fixture.token,
		"Content-Type":  "application/json",
	}

	rr := requestHTTPBody(t, fixture.server, http.MethodPatch, "/api/v1/workspaces/local/members/"+user.ID, `{"role":"admin","display_name":"HTTP Patched"}`, headers)
	if rr.Code != http.StatusOK {
		t.Fatalf("patch member status = %d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Data struct {
			UserID      string `json:"id"`
			DisplayName string `json:"display_name"`
			Role        string `json:"role"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.UserID != user.ID || payload.Data.DisplayName != "HTTP Patched" || payload.Data.Role != "admin" {
		t.Fatalf("patched member = %#v", payload.Data)
	}
}

func TestHTTPMemberDeleteRemovesNonOwnerAndProtectsOwner(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:read", "member:read", "member:write")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	member, err := svc.AddUser(app.AddUserInput{Name: "http-member-delete"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.AddMember(app.AddMemberInput{WorkspaceRef: "local", UserRef: member.Name, Role: app.RoleMember}); err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{"Authorization": "Bearer " + fixture.token}

	rr := requestHTTP(t, fixture.server, http.MethodDelete, "/api/v1/workspaces/local/members/"+member.ID, headers)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete member status = %d body=%s", rr.Code, rr.Body.String())
	}
	ws, err := fixture.server.store.LocalWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storage.NewMemberRepository(fixture.server.store.DB()).Get(member.ID, ws.ID); err != storage.ErrNotFound {
		t.Fatalf("deleted membership err = %v, want ErrNotFound", err)
	}

	local, err := storage.NewUserRepository(fixture.server.store.DB()).GetByName("local")
	if err != nil {
		t.Fatal(err)
	}
	rr = requestHTTP(t, fixture.server, http.MethodDelete, "/api/v1/workspaces/local/members/"+local.ID, headers)
	assertHTTPErrorCode(t, rr, http.StatusForbidden, "permission_denied")
}

func TestHTTPMemberAddCanCreateNewUser(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:read", "member:read", "member:write")
	headers := map[string]string{
		"Authorization": "Bearer " + fixture.token,
		"Content-Type":  "application/json",
	}

	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/workspaces/local/members", `{"new_user":{"name":"http-new-member","display_name":"HTTP New Member","email":"http-new-member@example.com"},"role":"member"}`, headers)
	if rr.Code != http.StatusCreated {
		t.Fatalf("add new user member status = %d body=%s", rr.Code, rr.Body.String())
	}
	user, err := storage.NewUserRepository(fixture.server.store.DB()).GetByName("http-new-member")
	if err != nil {
		t.Fatalf("GetByName(http-new-member) error = %v", err)
	}
	ws, err := fixture.server.store.LocalWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	member, err := storage.NewMemberRepository(fixture.server.store.DB()).Get(user.ID, ws.ID)
	if err != nil {
		t.Fatalf("Get(new member) error = %v", err)
	}
	if member.Role != string(app.RoleMember) || user.DisplayName != "HTTP New Member" {
		t.Fatalf("user=%#v member=%#v", user, member)
	}
}

func TestTenantTokenCanManageHTTPWorkspaceAndTokens(t *testing.T) {
	store := openHTTPTestStore(t)
	svc, err := app.NewService(app.ServiceOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateTenantAccessToken(app.CreateTenantAccessTokenInput{
		Name:   "runtime-manager",
		Scopes: []string{"workspace:read", "workspace:write", "token:read", "token:write", "task:read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(Options{Store: store, ConfigSecretKey: httpTestSecretKeyBase64()})
	headers := map[string]string{
		"Authorization": "Bearer " + created.RawToken,
		"Content-Type":  "application/json",
	}

	modifyWorkspace := requestHTTPBody(t, server, http.MethodPatch, "/api/v1/workspaces/local", `{"name":"Tenant Local"}`, headers)
	if modifyWorkspace.Code != http.StatusOK {
		t.Fatalf("modify workspace status = %d body=%s", modifyWorkspace.Code, modifyWorkspace.Body.String())
	}
	if !strings.Contains(modifyWorkspace.Body.String(), `"name":"Tenant Local"`) {
		t.Fatalf("modified workspace body=%s", modifyWorkspace.Body.String())
	}

	createToken := requestHTTPBody(t, server, http.MethodPost, "/api/v1/tokens", `{"name":"tenant-created-pat","type":"pat","user":"local","scopes":["task:read"],"workspaces":["local"]}`, headers)
	if createToken.Code != http.StatusCreated {
		t.Fatalf("create token status = %d body=%s", createToken.Code, createToken.Body.String())
	}
	if !strings.Contains(createToken.Body.String(), `"type":"pat"`) || !strings.Contains(createToken.Body.String(), `"token":"xuanchu_pat_`) {
		t.Fatalf("created token body=%s", createToken.Body.String())
	}

	listTokens := requestHTTP(t, server, http.MethodGet, "/api/v1/tokens", headers)
	if listTokens.Code != http.StatusOK {
		t.Fatalf("list tokens status = %d body=%s", listTokens.Code, listTokens.Body.String())
	}
	if !strings.Contains(listTokens.Body.String(), `"name":"tenant-created-pat"`) || strings.Contains(listTokens.Body.String(), `"type":"tenant_access_token"`) {
		t.Fatalf("list tokens body=%s", listTokens.Body.String())
	}

	createWorkspace := requestHTTPBody(t, server, http.MethodPost, "/api/v1/workspaces", `{"slug":"tenant-created-workspace"}`, headers)
	assertHTTPErrorCode(t, createWorkspace, http.StatusBadRequest, "tenant_actor_not_user")
}

func TestTenantTokenCannotMintTenantTokenBeyondOwnScope(t *testing.T) {
	store := openHTTPTestStore(t)
	svc, err := app.NewService(app.ServiceOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	project, err := svc.AddProject(app.AddProjectInput{Slug: "limited", Name: "Limited"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateTenantAccessToken(app.CreateTenantAccessTokenInput{
		Name:        "limited-token-manager",
		Scopes:      []string{"token:write"},
		ProjectRefs: []string{"limited"},
	})
	if err != nil {
		t.Fatal(err)
	}
	wider, err := svc.CreateTenantAccessToken(app.CreateTenantAccessTokenInput{
		Name:   "wider-token",
		Scopes: []string{"token:write"},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(Options{Store: store, ConfigSecretKey: httpTestSecretKeyBase64()})
	headers := map[string]string{
		"Authorization": "Bearer " + created.RawToken,
		"Content-Type":  "application/json",
	}

	rr := requestHTTPBody(t, server, http.MethodPost, "/api/v1/tenant-access-tokens", `{"name":"escaped","scopes":["task:read"]}`, headers)
	assertHTTPErrorCode(t, rr, http.StatusForbidden, "token_scope_denied")

	createChild := requestHTTPBody(t, server, http.MethodPost, "/api/v1/tenant-access-tokens", `{"name":"child","scopes":["token:write"],"projects":["limited"]}`, headers)
	if createChild.Code != http.StatusCreated {
		t.Fatalf("create child status = %d body=%s", createChild.Code, createChild.Body.String())
	}
	var payload struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createChild.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	rr = requestHTTPBody(t, server, http.MethodPatch, "/api/v1/tenant-access-tokens/"+payload.Data.ID, `{"scopes":["token:write","task:read"]}`, headers)
	assertHTTPErrorCode(t, rr, http.StatusForbidden, "token_scope_denied")

	rr = requestHTTPBody(t, server, http.MethodPatch, "/api/v1/tenant-access-tokens/"+wider.View.ID, `{"projects":["limited"]}`, headers)
	assertHTTPErrorCode(t, rr, http.StatusForbidden, "project_scope_denied")

	rr = requestHTTPBody(t, server, http.MethodPatch, "/api/v1/tenant-access-tokens/"+wider.View.ID, `{}`, headers)
	assertHTTPErrorCode(t, rr, http.StatusForbidden, "project_scope_denied")

	rr = requestHTTP(t, server, http.MethodDelete, "/api/v1/tenant-access-tokens/"+wider.View.ID, headers)
	assertHTTPErrorCode(t, rr, http.StatusForbidden, "project_scope_denied")
	if len(created.View.ProjectIDs) != 1 || created.View.ProjectIDs[0] != project.ID {
		t.Fatalf("limited manager project_ids = %#v, want [%s]", created.View.ProjectIDs, project.ID)
	}
}

func TestTenantTokenListFiltersTenantTokensByProjectScope(t *testing.T) {
	store := openHTTPTestStore(t)
	svc, err := app.NewService(app.ServiceOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	one, err := svc.AddProject(app.AddProjectInput{Slug: "one", Name: "One"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddProject(app.AddProjectInput{Slug: "two", Name: "Two"}); err != nil {
		t.Fatal(err)
	}
	reader, err := svc.CreateTenantAccessToken(app.CreateTenantAccessTokenInput{
		Name:        "limited-reader",
		Scopes:      []string{"token:read"},
		ProjectRefs: []string{"one"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateTenantAccessToken(app.CreateTenantAccessTokenInput{
		Name:        "one-visible",
		Scopes:      []string{"token:read"},
		ProjectRefs: []string{"one"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateTenantAccessToken(app.CreateTenantAccessTokenInput{
		Name:        "two-hidden",
		Scopes:      []string{"token:read"},
		ProjectRefs: []string{"two"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateTenantAccessToken(app.CreateTenantAccessTokenInput{
		Name:   "wide-hidden",
		Scopes: []string{"token:read"},
	}); err != nil {
		t.Fatal(err)
	}
	server := NewServer(Options{Store: store})
	rr := requestHTTP(t, server, http.MethodGet, "/api/v1/tenant-access-tokens", map[string]string{
		"Authorization": "Bearer " + reader.RawToken,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Data []struct {
			Name       string   `json:"name"`
			ProjectIDs []string `json:"project_ids"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, token := range payload.Data {
		names[token.Name] = true
		for _, projectID := range token.ProjectIDs {
			if projectID != one.ID {
				t.Fatalf("listed token %q project_ids = %v, want only %s", token.Name, token.ProjectIDs, one.ID)
			}
		}
		if len(token.ProjectIDs) == 0 {
			t.Fatalf("listed token %q is workspace-wide under project-scoped reader", token.Name)
		}
	}
	if !names["limited-reader"] || !names["one-visible"] {
		t.Fatalf("listed names = %v, want limited-reader and one-visible", names)
	}
	if names["two-hidden"] || names["wide-hidden"] {
		t.Fatalf("listed names = %v, should hide two-hidden and wide-hidden", names)
	}
}

func TestTokenListFiltersRegularTokensByRequestScope(t *testing.T) {
	store := openHTTPTestStore(t)
	svc, err := app.NewService(app.ServiceOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	one, err := svc.AddProject(app.AddProjectInput{Slug: "one", Name: "One"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddProject(app.AddProjectInput{Slug: "two", Name: "Two"}); err != nil {
		t.Fatal(err)
	}
	reader, err := svc.CreateToken(app.CreateTokenInput{
		Name:          "limited-reader",
		Type:          "pat",
		UserRef:       "local",
		Scopes:        []string{"token:read"},
		WorkspaceRefs: []string{"local"},
		ProjectRefs:   []string{"one"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateToken(app.CreateTokenInput{
		Name:          "one-visible",
		Type:          "pat",
		UserRef:       "local",
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
		ProjectRefs:   []string{"one"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateToken(app.CreateTokenInput{
		Name:          "two-hidden",
		Type:          "pat",
		UserRef:       "local",
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
		ProjectRefs:   []string{"two"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateToken(app.CreateTokenInput{
		Name:          "wide-hidden",
		Type:          "pat",
		UserRef:       "local",
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	}); err != nil {
		t.Fatal(err)
	}
	server := NewServer(Options{Store: store})
	rr := requestHTTP(t, server, http.MethodGet, "/api/v1/tokens", map[string]string{
		"Authorization": "Bearer " + reader.RawToken,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Data []struct {
			Name       string   `json:"name"`
			ProjectIDs []string `json:"project_ids"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, token := range payload.Data {
		names[token.Name] = true
		for _, projectID := range token.ProjectIDs {
			if projectID != one.ID {
				t.Fatalf("listed token %q project_ids = %v, want only %s", token.Name, token.ProjectIDs, one.ID)
			}
		}
		if len(token.ProjectIDs) == 0 {
			t.Fatalf("listed token %q is workspace-wide under project-scoped reader", token.Name)
		}
	}
	if !names["limited-reader"] || !names["one-visible"] {
		t.Fatalf("listed names = %v, want limited-reader and one-visible", names)
	}
	if names["two-hidden"] || names["wide-hidden"] {
		t.Fatalf("listed names = %v, should hide two-hidden and wide-hidden", names)
	}
}

func TestTenantTokenCannotModifyHTTPTokenBeyondOwnScope(t *testing.T) {
	store := openHTTPTestStore(t)
	svc, err := app.NewService(app.ServiceOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	tenant, err := svc.CreateTenantAccessToken(app.CreateTenantAccessTokenInput{
		Name:   "limited-token-manager",
		Scopes: []string{"token:read", "token:write", "task:read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.CreateToken(app.CreateTokenInput{
		Name:          "managed-pat",
		Type:          "pat",
		UserRef:       "local",
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	widerChild, err := svc.CreateToken(app.CreateTokenInput{
		Name:          "wider-pat",
		Type:          "pat",
		UserRef:       "local",
		Scopes:        []string{"task:read", "project:write"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(Options{Store: store})
	headers := map[string]string{
		"Authorization": "Bearer " + tenant.RawToken,
		"Content-Type":  "application/json",
	}

	escalateScope := requestHTTPBody(t, server, http.MethodPatch, "/api/v1/tokens/"+child.View.ID, `{"scopes":["task:read","project:write"]}`, headers)
	assertHTTPErrorCode(t, escalateScope, http.StatusForbidden, "token_scope_denied")

	clearWorkspace := requestHTTPBody(t, server, http.MethodPatch, "/api/v1/tokens/"+child.View.ID, `{"workspaces":[]}`, headers)
	assertHTTPErrorCode(t, clearWorkspace, http.StatusForbidden, "workspace_scope_denied")

	revokeWiderScope := requestHTTP(t, server, http.MethodDelete, "/api/v1/tokens/"+widerChild.View.ID, headers)
	assertHTTPErrorCode(t, revokeWiderScope, http.StatusForbidden, "token_scope_denied")

	downgradeWiderScope := requestHTTPBody(t, server, http.MethodPatch, "/api/v1/tokens/"+widerChild.View.ID, `{"scopes":["task:read"]}`, headers)
	assertHTTPErrorCode(t, downgradeWiderScope, http.StatusForbidden, "token_scope_denied")

	allowedRename := requestHTTPBody(t, server, http.MethodPatch, "/api/v1/tokens/"+child.View.ID, `{"name":"managed-renamed"}`, headers)
	if allowedRename.Code != http.StatusOK {
		t.Fatalf("allowed rename status=%d body=%s", allowedRename.Code, allowedRename.Body.String())
	}
}

func TestTenantTokenCannotUseHTTPImpersonationHeader(t *testing.T) {
	store := openHTTPTestStore(t)
	svc, err := app.NewService(app.ServiceOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateTenantAccessToken(app.CreateTenantAccessTokenInput{
		Name:   "runtime",
		Scopes: []string{"workspace:write"},
	})
	if err != nil {
		t.Fatal(err)
	}
	rr := requestHTTPBody(t, NewServer(Options{Store: store}), http.MethodPatch, "/api/v1/workspaces/local", `{"name":"bad impersonation"}`, map[string]string{
		"Authorization": "Bearer " + created.RawToken,
		"Content-Type":  "application/json",
		"X-Xuanchu-As":  "local",
	})
	assertHTTPErrorCode(t, rr, http.StatusForbidden, "token_scope_denied")
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

func TestTenantTokenCanCreateHTTPSystemActorResources(t *testing.T) {
	store := openHTTPTestStore(t)
	svc, err := app.NewService(app.ServiceOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	project, err := svc.AddProject(app.AddProjectInput{Slug: "tenantp2", Name: "Tenant P2"})
	if err != nil {
		t.Fatal(err)
	}
	createdTask, err := svc.Add(app.AddInput{Title: "tenant p2 link target", Project: &project.Slug})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateTenantAccessToken(app.CreateTenantAccessTokenInput{
		Name: "runtime-p2",
		Scopes: []string{
			"task:write",
			"project:write",
			"hook:write",
			"notification:write",
			"reminder:write",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(Options{Store: store})
	headers := map[string]string{
		"Authorization": "Bearer " + created.RawToken,
		"Content-Type":  "application/json",
	}

	sinkBody := `{"name":"tenant-p2-sink","type":"webhook","endpoint_mode":"static_url","url":"https://example.com/webhook","allowed_hosts":["example.com"],"secret":"tenant-secret"}`
	rr := requestHTTPBody(t, server, http.MethodPost, "/api/v1/notification-sinks", sinkBody, headers)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create sink status = %d body=%s", rr.Code, rr.Body.String())
	}
	sink := httpResponseDataMap(t, rr)
	assertHTTPSystemActor(t, sink["created_by"], created.View.ID)

	hookBody := `{"name":"tenant-p2-hook","scope_type":"workspace","event_types":["task.created"],"sink":"tenant-p2-sink"}`
	rr = requestHTTPBody(t, server, http.MethodPost, "/api/v1/hooks", hookBody, headers)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create hook status = %d body=%s", rr.Code, rr.Body.String())
	}
	hook := httpResponseDataMap(t, rr)
	assertHTTPSystemActor(t, hook["created_by"], created.View.ID)

	reminderBody := `{"name":"tenant-p2-reminder","trigger_type":"overdue","repeat_policy":"once","audience_type":"explicit_users","recipients":["local"],"sink_ref":"tenant-p2-sink"}`
	rr = requestHTTPBody(t, server, http.MethodPost, "/api/v1/reminder-rules", reminderBody, headers)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create reminder status = %d body=%s", rr.Code, rr.Body.String())
	}
	reminder := httpResponseDataMap(t, rr)
	assertHTTPSystemActor(t, reminder["created_by"], created.View.ID)

	notificationRuleBody := `{"name":"tenant-p2-notification","event_type":"task.created","audience_type":"actor","sink":"tenant-p2-sink","template_subject":"Task created","template_body":"{{event.type}}"}`
	rr = requestHTTPBody(t, server, http.MethodPost, "/api/v1/notification-rules", notificationRuleBody, headers)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create notification rule status = %d body=%s", rr.Code, rr.Body.String())
	}
	notificationRule := httpResponseDataMap(t, rr)
	assertHTTPSystemActor(t, notificationRule["created_by"], created.View.ID)

	linkBody := `{"type":"document","url":"https://example.com/spec","title":"Spec"}`
	rr = requestHTTPBody(t, server, http.MethodPost, "/api/v1/tasks/"+createdTask.UUID+"/links", linkBody, headers)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create task link status = %d body=%s", rr.Code, rr.Body.String())
	}
	link := httpResponseDataMap(t, rr)
	assertHTTPSystemActor(t, link["created_by"], created.View.ID)

	annotationBody := `{"content":"tenant p2 annotation"}`
	rr = requestHTTPBody(t, server, http.MethodPost, "/api/v1/projects/"+project.Slug+"/annotations", annotationBody, headers)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create project annotation status = %d body=%s", rr.Code, rr.Body.String())
	}
	annotation := httpResponseDataMap(t, rr)
	assertHTTPSystemActor(t, annotation["created_by"], created.View.ID)
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
		Update("scopes_json", `["`+auth.ScopeImpersonate+`"]`).Error; err != nil {
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
			UserID   string `json:"id"`
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

// TestMemberAndViewerCanReadWorkspaceAudit 验证普通 member/viewer 在持有 audit:read scope 时
// 可以查看 workspace 全量审计（spec：所有可登录成员可查看 workspace 全量 audit）。
func TestMemberAndViewerCanReadWorkspaceAudit(t *testing.T) {
	store := openHTTPTestStore(t)
	ownerSvc, err := app.NewService(app.ServiceOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}

	// 触发一条审计：owner 修改 workspace 配置。
	ownerToken, err := ownerSvc.CreateToken(app.CreateTokenInput{
		Name:          "owner",
		Scopes:        []string{"config:write", "audit:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(Options{Store: store})
	rr := requestHTTPBody(t, srv, http.MethodPut, "/api/v1/config/date.format", `{"value":"rfc3339"}`, map[string]string{
		"Authorization": "Bearer " + ownerToken.RawToken,
		"Content-Type":  "application/json",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("owner config write status = %d body=%s", rr.Code, rr.Body.String())
	}

	cases := []struct {
		name string
		role app.Role
	}{
		{name: "member", role: app.RoleMember},
		{name: "viewer", role: app.RoleViewer},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			user, err := ownerSvc.AddUser(app.AddUserInput{Name: "audit-" + tc.name})
			if err != nil {
				t.Fatal(err)
			}
			if err := ownerSvc.AddMember(app.AddMemberInput{WorkspaceRef: "local", UserRef: user.ID, Role: tc.role}); err != nil {
				t.Fatal(err)
			}
			tok, err := ownerSvc.CreateToken(app.CreateTokenInput{
				Name:          "audit-" + tc.name,
				UserRef:       user.ID,
				Scopes:        []string{"audit:read"},
				WorkspaceRefs: []string{"local"},
			})
			if err != nil {
				t.Fatal(err)
			}
			rr := requestHTTP(t, srv, http.MethodGet, "/api/v1/audit?limit=10", map[string]string{
				"Authorization": "Bearer " + tok.RawToken,
			})
			if rr.Code != http.StatusOK {
				t.Fatalf("%s audit read status = %d body=%s", tc.name, rr.Code, rr.Body.String())
			}
			var payload struct {
				Data []struct {
					Action string `json:"action"`
				} `json:"data"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if len(payload.Data) == 0 {
				t.Fatalf("%s audit returned empty rows", tc.name)
			}
		})
	}
}
