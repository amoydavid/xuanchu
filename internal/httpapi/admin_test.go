package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/config"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

func TestAdminEndpointDisabled(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:write")
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces", `{}`, nil)
	assertHTTPErrorCode(t, rr, http.StatusNotFound, "route_not_found")
}

func TestAdminEndpointRequiresAdminToken(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:write")
	fixture.server.admin = config.AdminConfig{
		Enabled: true,
		Tokens:  []config.AdminTokenConfig{{Name: "ops", Hash: auth.HashAdminToken("secret"), Enabled: true}},
	}
	fixture.server.router = fixture.server.newRouter()
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces", `{}`, nil)
	assertHTTPErrorCode(t, rr, http.StatusUnauthorized, "admin_auth_required")
}

func TestAdminEndpointRejectsInvalidAdminToken(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:write")
	fixture.server.admin = config.AdminConfig{
		Enabled: true,
		Tokens:  []config.AdminTokenConfig{{Name: "ops", Hash: auth.HashAdminToken("secret"), Enabled: true}},
	}
	fixture.server.router = fixture.server.newRouter()
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces", `{}`, map[string]string{
		"Authorization": "Bearer wrong",
	})
	assertHTTPErrorCode(t, rr, http.StatusUnauthorized, "admin_auth_invalid")
}

func TestAdminSessionHTTP(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:write")
	adminRaw := "xuanchu_admin_secret"
	fixture.server.admin = config.AdminConfig{
		Enabled: true,
		Tokens:  []config.AdminTokenConfig{{Name: "ops-primary", Hash: auth.HashAdminToken(adminRaw), Enabled: true}},
	}
	fixture.server.router = fixture.server.newRouter()
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/admin/session", map[string]string{
		"Authorization": "Bearer " + adminRaw,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"token_name":"ops-primary"`) {
		t.Fatalf("body = %s", body)
	}
	if !strings.Contains(body, `"workspace:create"`) ||
		!strings.Contains(body, `"workspace_admin:create"`) ||
		!strings.Contains(body, `"agent_token:create"`) {
		t.Fatalf("body = %s", body)
	}
	if strings.Contains(body, adminRaw) || strings.Contains(body, "sha256:") {
		t.Fatalf("session leaked secret material: %s", body)
	}
}

func TestAdminSessionDisabledReturnsNotFound(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:write")
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/admin/session", map[string]string{
		"Authorization": "Bearer xuanchu_admin_secret",
	})
	assertHTTPErrorCode(t, rr, http.StatusNotFound, "route_not_found")
}

func TestNormalTokenCannotAccessAdminSession(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:write")
	adminRaw := "xuanchu_admin_secret"
	fixture.server.admin = config.AdminConfig{
		Enabled: true,
		Tokens:  []config.AdminTokenConfig{{Name: "ops", Hash: auth.HashAdminToken(adminRaw), Enabled: true}},
	}
	fixture.server.router = fixture.server.newRouter()
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/admin/session", map[string]string{
		"Authorization": "Bearer " + fixture.token,
	})
	assertHTTPErrorCode(t, rr, http.StatusUnauthorized, "admin_auth_invalid")
}

func TestAdminTokenCannotAccessMe(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:read")
	adminRaw := "xuanchu_admin_secret"
	fixture.server.admin = config.AdminConfig{
		Enabled: true,
		Tokens:  []config.AdminTokenConfig{{Name: "ops", Hash: auth.HashAdminToken(adminRaw), Enabled: true}},
	}
	fixture.server.router = fixture.server.newRouter()
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/me", map[string]string{
		"Authorization": "Bearer " + adminRaw,
	})
	assertHTTPErrorCode(t, rr, http.StatusUnauthorized, "auth_invalid_token")
}

func TestAdminCreateWorkspaceHTTP(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:write")
	adminRaw := "xuanchu_admin_secret"
	fixture.server.admin = config.AdminConfig{
		Enabled: true,
		Tokens:  []config.AdminTokenConfig{{Name: "ops", Hash: auth.HashAdminToken(adminRaw), Enabled: true}},
	}
	fixture.server.router = fixture.server.newRouter()
	body := `{"slug":"dajee","name":"Dajee","owner":{"name":"alice","email":"alice@example.com"}}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces", body, map[string]string{
		"Authorization": "Bearer " + adminRaw,
		"Content-Type":  "application/json",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"slug":"dajee"`) || !strings.Contains(rr.Body.String(), `"name":"alice"`) {
		t.Fatalf("body = %s", rr.Body.String())
	}
}

func TestAdminCreateWorkspaceHTTPDoesNotWriteEmptyWorkspaceConfigDefinitions(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:write")
	adminRaw := "xuanchu_admin_secret"
	fixture.server.admin = config.AdminConfig{
		Enabled: true,
		Tokens:  []config.AdminTokenConfig{{Name: "ops", Hash: auth.HashAdminToken(adminRaw), Enabled: true}},
	}
	fixture.server.router = fixture.server.newRouter()
	body := `{"slug":"dajee","name":"Dajee","owner":{"name":"alice","email":"alice@example.com"}}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces", body, map[string]string{
		"Authorization": "Bearer " + adminRaw,
		"Content-Type":  "application/json",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var count int64
	if err := fixture.server.store.DB().
		Model(&storage.ConfigDefinition{}).
		Where("workspace_id = ?", "").
		Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("empty workspace config definitions = %d", count)
	}
}

func TestAdminCreateWorkspaceHTTPRejectsDuplicateSlugAsConflict(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:write")
	adminRaw := "xuanchu_admin_secret"
	fixture.server.admin = config.AdminConfig{
		Enabled: true,
		Tokens:  []config.AdminTokenConfig{{Name: "ops", Hash: auth.HashAdminToken(adminRaw), Enabled: true}},
	}
	fixture.server.router = fixture.server.newRouter()
	body := `{"slug":"dajee","owner":{"name":"alice","email":"alice@example.com"}}`
	headers := map[string]string{
		"Authorization": "Bearer " + adminRaw,
		"Content-Type":  "application/json",
	}
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces", body, headers)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create workspace status = %d body=%s", rr.Code, rr.Body.String())
	}
	rr = requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces", body, headers)
	assertHTTPErrorCode(t, rr, http.StatusConflict, "admin_workspace_exists")
}

func TestAdminTokenCannotAccessNormalAPI(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:write", "task:read")
	adminRaw := "xuanchu_admin_secret"
	fixture.server.admin = config.AdminConfig{
		Enabled: true,
		Tokens:  []config.AdminTokenConfig{{Name: "ops", Hash: auth.HashAdminToken(adminRaw), Enabled: true}},
	}
	fixture.server.router = fixture.server.newRouter()
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/tasks", map[string]string{
		"Authorization": "Bearer " + adminRaw,
	})
	assertHTTPErrorCode(t, rr, http.StatusUnauthorized, "auth_invalid_token")
}

func TestNormalTokenCannotAccessAdminAPI(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:write")
	adminRaw := "xuanchu_admin_secret"
	fixture.server.admin = config.AdminConfig{
		Enabled: true,
		Tokens:  []config.AdminTokenConfig{{Name: "ops", Hash: auth.HashAdminToken(adminRaw), Enabled: true}},
	}
	fixture.server.router = fixture.server.newRouter()
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces", `{}`, map[string]string{
		"Authorization": "Bearer " + fixture.token,
	})
	assertHTTPErrorCode(t, rr, http.StatusUnauthorized, "admin_auth_invalid")
}

func TestAdminWorkspaceAdminCreateHTTP(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:write")
	adminRaw := "xuanchu_admin_secret"
	fixture.server.admin = config.AdminConfig{
		Enabled: true,
		Tokens:  []config.AdminTokenConfig{{Name: "ops", Hash: auth.HashAdminToken(adminRaw), Enabled: true}},
	}
	fixture.server.router = fixture.server.newRouter()
	headers := map[string]string{
		"Authorization": "Bearer " + adminRaw,
		"Content-Type":  "application/json",
	}
	createWS := `{"slug":"dajee","owner":{"name":"alice","email":"alice@example.com"}}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces", createWS, headers)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create workspace status = %d body=%s", rr.Code, rr.Body.String())
	}
	body := `{"name":"bob","email":"bob@example.com","role":"admin"}`
	rr = requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces/dajee/admins", body, headers)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create admin status = %d body=%s", rr.Code, rr.Body.String())
	}
	var envelope struct {
		Data struct {
			Workspace struct {
				Slug string `json:"slug"`
			} `json:"workspace"`
			Admin      map[string]any `json:"admin"`
			Membership struct {
				Role     string `json:"role"`
				JoinedAt int64  `json:"joined_at"`
			} `json:"membership"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Workspace.Slug != "dajee" || envelope.Data.Membership.Role != "admin" || envelope.Data.Membership.JoinedAt == 0 {
		t.Fatalf("response = %#v body=%s", envelope.Data, rr.Body.String())
	}
	if envelope.Data.Admin["id"] == "" || envelope.Data.Admin["name"] != "bob" || envelope.Data.Admin["email"] != "bob@example.com" {
		t.Fatalf("admin = %#v", envelope.Data.Admin)
	}
	if _, ok := envelope.Data.Admin["external_ids"]; !ok {
		t.Fatalf("admin missing external_ids: %#v", envelope.Data.Admin)
	}
	if strings.Contains(rr.Body.String(), `"admin":"`) {
		t.Fatalf("admin is a bare string: %s", rr.Body.String())
	}
}

func TestAdminWorkspaceAdminPromoteHTTP(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:write")
	adminRaw := "xuanchu_admin_secret"
	fixture.server.admin = config.AdminConfig{
		Enabled: true,
		Tokens:  []config.AdminTokenConfig{{Name: "ops", Hash: auth.HashAdminToken(adminRaw), Enabled: true}},
	}
	fixture.server.router = fixture.server.newRouter()
	headers := map[string]string{
		"Authorization": "Bearer " + adminRaw,
		"Content-Type":  "application/json",
	}
	createWS := `{"slug":"dajee","owner":{"name":"alice","email":"alice@example.com"}}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces", createWS, headers)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create workspace status = %d body=%s", rr.Code, rr.Body.String())
	}
	body := `{"name":"alice","email":"alice@example.com","role":"owner"}`
	rr = requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces/dajee/admins", body, headers)
	if rr.Code != http.StatusCreated {
		t.Fatalf("promote admin status = %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"role":"owner"`) {
		t.Fatalf("body = %s", rr.Body.String())
	}
}

func TestAdminWorkspaceAdminRejectsInvalidRole(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:write")
	adminRaw := "xuanchu_admin_secret"
	fixture.server.admin = config.AdminConfig{
		Enabled: true,
		Tokens:  []config.AdminTokenConfig{{Name: "ops", Hash: auth.HashAdminToken(adminRaw), Enabled: true}},
	}
	fixture.server.router = fixture.server.newRouter()
	headers := map[string]string{
		"Authorization": "Bearer " + adminRaw,
		"Content-Type":  "application/json",
	}
	createWS := `{"slug":"dajee","owner":{"name":"alice"}}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces", createWS, headers)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create workspace status = %d body=%s", rr.Code, rr.Body.String())
	}
	rr = requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces/dajee/admins", `{"name":"bob","role":"member"}`, headers)
	assertHTTPErrorCode(t, rr, http.StatusBadRequest, "admin_role_invalid")
}

func TestAdminWorkspaceAdminRejectsMissingWorkspace(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:write")
	adminRaw := "xuanchu_admin_secret"
	fixture.server.admin = config.AdminConfig{
		Enabled: true,
		Tokens:  []config.AdminTokenConfig{{Name: "ops", Hash: auth.HashAdminToken(adminRaw), Enabled: true}},
	}
	fixture.server.router = fixture.server.newRouter()
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces/missing/admins", `{"name":"bob"}`, map[string]string{
		"Authorization": "Bearer " + adminRaw,
		"Content-Type":  "application/json",
	})
	assertHTTPErrorCode(t, rr, http.StatusNotFound, "workspace_not_found")
}

func TestAdminWorkspaceAdminRejectsArchivedWorkspace(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:write")
	adminRaw := "xuanchu_admin_secret"
	fixture.server.admin = config.AdminConfig{
		Enabled: true,
		Tokens:  []config.AdminTokenConfig{{Name: "ops", Hash: auth.HashAdminToken(adminRaw), Enabled: true}},
	}
	fixture.server.router = fixture.server.newRouter()
	headers := map[string]string{
		"Authorization": "Bearer " + adminRaw,
		"Content-Type":  "application/json",
	}
	createWS := `{"slug":"dajee","owner":{"name":"alice"}}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces", createWS, headers)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create workspace status = %d body=%s", rr.Code, rr.Body.String())
	}
	if err := fixture.server.store.DB().Model(&storage.Workspace{}).Where("slug = ?", "dajee").Update("archived_at", int64(1234)).Error; err != nil {
		t.Fatal(err)
	}
	rr = requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces/dajee/admins", `{"name":"bob"}`, headers)
	assertHTTPErrorCode(t, rr, http.StatusBadRequest, "workspace_archived")
}

func TestAdminCreateWorkspaceAgentTokenHTTP(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:write", "token:write")
	adminRaw := "xuanchu_admin_secret"
	fixture.server.admin = config.AdminConfig{
		Enabled: true,
		Tokens:  []config.AdminTokenConfig{{Name: "ops", Hash: auth.HashAdminToken(adminRaw), Enabled: true}},
	}
	fixture.server.router = fixture.server.newRouter()
	createWS := `{"slug":"dajee","owner":{"name":"alice","email":"alice@example.com"}}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces", createWS, map[string]string{
		"Authorization": "Bearer " + adminRaw,
		"Content-Type":  "application/json",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create workspace status = %d body=%s", rr.Code, rr.Body.String())
	}
	body := `{"name":"openclaw","user":"alice@example.com","scopes":["task:read","task:write"],"expires_in":"24h"}`
	rr = requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces/dajee/agent-tokens", body, map[string]string{
		"Authorization": "Bearer " + adminRaw,
		"Content-Type":  "application/json",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create token status = %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"token":"xuanchu_agent_`) || !strings.Contains(rr.Body.String(), `"workspace_ids"`) {
		t.Fatalf("body = %s", rr.Body.String())
	}
}

func TestAdminCreateWorkspaceAgentTokenRejectsNegativeTTL(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:write", "token:write")
	adminRaw := "xuanchu_admin_secret"
	fixture.server.admin = config.AdminConfig{
		Enabled: true,
		Tokens:  []config.AdminTokenConfig{{Name: "ops", Hash: auth.HashAdminToken(adminRaw), Enabled: true}},
	}
	fixture.server.router = fixture.server.newRouter()
	createWS := `{"slug":"dajee","owner":{"name":"alice","email":"alice@example.com"}}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces", createWS, map[string]string{
		"Authorization": "Bearer " + adminRaw,
		"Content-Type":  "application/json",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create workspace status = %d body=%s", rr.Code, rr.Body.String())
	}
	body := `{"name":"openclaw","scopes":["task:read"],"expires_in_seconds":-1}`
	rr = requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces/dajee/agent-tokens", body, map[string]string{
		"Authorization": "Bearer " + adminRaw,
		"Content-Type":  "application/json",
	})
	assertHTTPErrorCode(t, rr, http.StatusBadRequest, "admin_token_ttl_invalid")
}

func TestAdminCreateWorkspaceAgentTokenRejectsInvalidScope(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:write", "token:write")
	adminRaw := "xuanchu_admin_secret"
	fixture.server.admin = config.AdminConfig{
		Enabled: true,
		Tokens:  []config.AdminTokenConfig{{Name: "ops", Hash: auth.HashAdminToken(adminRaw), Enabled: true}},
	}
	fixture.server.router = fixture.server.newRouter()
	createWS := `{"slug":"dajee","owner":{"name":"alice","email":"alice@example.com"}}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces", createWS, map[string]string{
		"Authorization": "Bearer " + adminRaw,
		"Content-Type":  "application/json",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create workspace status = %d body=%s", rr.Code, rr.Body.String())
	}
	body := `{"name":"openclaw","scopes":["bad:scope"]}`
	rr = requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces/dajee/agent-tokens", body, map[string]string{
		"Authorization": "Bearer " + adminRaw,
		"Content-Type":  "application/json",
	})
	assertHTTPErrorCode(t, rr, http.StatusBadRequest, "token_scope_invalid")
}

func TestAdminCreateWorkspaceAgentTokenRejectsInvalidProjectScope(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "workspace:write", "token:write")
	adminRaw := "xuanchu_admin_secret"
	fixture.server.admin = config.AdminConfig{
		Enabled: true,
		Tokens:  []config.AdminTokenConfig{{Name: "ops", Hash: auth.HashAdminToken(adminRaw), Enabled: true}},
	}
	fixture.server.router = fixture.server.newRouter()
	createWS := `{"slug":"dajee","owner":{"name":"alice","email":"alice@example.com"}}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces", createWS, map[string]string{
		"Authorization": "Bearer " + adminRaw,
		"Content-Type":  "application/json",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create workspace status = %d body=%s", rr.Code, rr.Body.String())
	}
	body := `{"name":"openclaw","scopes":["task:read"],"project_refs":["missing"]}`
	rr = requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces/dajee/agent-tokens", body, map[string]string{
		"Authorization": "Bearer " + adminRaw,
		"Content-Type":  "application/json",
	})
	assertHTTPErrorCode(t, rr, http.StatusBadRequest, "token_project_scope_invalid")
}

// newAdminTokenFixture 建一个启用 admin 的 server，预建一个 workspace + agent token。
// 返回 server、adminRaw token、已建 token 的 id（供 modify/revoke 测试）。
func newAdminTokenFixture(t *testing.T) (*Server, string, string) {
	t.Helper()
	fixture := newHTTPServerWithTokenFixture(t, "token:write")
	adminRaw := "xuanchu_admin_secret"
	fixture.server.admin = config.AdminConfig{
		Enabled: true,
		Tokens:  []config.AdminTokenConfig{{Name: "ops", Hash: auth.HashAdminToken(adminRaw), Enabled: true}},
	}
	fixture.server.router = fixture.server.newRouter()
	adminHeaders := map[string]string{
		"Authorization": "Bearer " + adminRaw,
		"Content-Type":  "application/json",
	}
	// 建 workspace + agent token
	requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces", `{"slug":"dajee","owner":{"name":"alice","email":"alice@example.com"}}`, adminHeaders)
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces/dajee/agent-tokens", `{"name":"ci","user":"alice@example.com","scopes":["task:read"]}`, adminHeaders)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create agent token status = %d body=%s", rr.Code, rr.Body.String())
	}
	var created struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	return fixture.server, adminRaw, created.Data.ID
}

func TestAdminTokenListHTTP(t *testing.T) {
	srv, adminRaw, tokenID := newAdminTokenFixture(t)
	rr := requestHTTP(t, srv, http.MethodGet, "/api/v1/admin/tokens?all=true", map[string]string{
		"Authorization": "Bearer " + adminRaw,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			User struct {
				Name string `json:"name"`
			} `json:"user"`
			Scopes       []string `json:"scopes"`
			WorkspaceIDs []string `json:"workspace_ids"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tk := range payload.Data {
		if tk.ID == tokenID {
			found = true
			if tk.User.Name != "alice" {
				t.Fatalf("user name = %q, want alice", tk.User.Name)
			}
			if len(tk.WorkspaceIDs) == 0 {
				t.Fatalf("workspace_ids should not be empty for agent token")
			}
		}
	}
	if !found {
		t.Fatalf("token %s not in list: %#v", tokenID, payload.Data)
	}
}

func TestAdminTokenRevokeHTTP(t *testing.T) {
	srv, adminRaw, tokenID := newAdminTokenFixture(t)
	rr := requestHTTP(t, srv, http.MethodDelete, "/api/v1/admin/tokens/"+tokenID, map[string]string{
		"Authorization": "Bearer " + adminRaw,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("revoke status = %d body=%s", rr.Code, rr.Body.String())
	}
	// 再次 list（all=true）应显示已吊销
	rr = requestHTTP(t, srv, http.MethodGet, "/api/v1/admin/tokens?all=true", map[string]string{
		"Authorization": "Bearer " + adminRaw,
	})
	var payload struct {
		Data []struct {
			ID        string `json:"id"`
			RevokedAt *int64 `json:"revoked_at"`
		} `json:"data"`
	}
	json.Unmarshal(rr.Body.Bytes(), &payload)
	for _, tk := range payload.Data {
		if tk.ID == tokenID && tk.RevokedAt == nil {
			t.Fatalf("token %s should have revoked_at set", tokenID)
		}
	}
}

func TestAdminCanListAndRevokeTenantAccessTokens(t *testing.T) {
	srv, adminRaw, _ := newAdminTokenFixture(t)
	svc, err := app.NewService(app.ServiceOptions{Store: srv.store})
	if err != nil {
		t.Fatal(err)
	}
	tenant, err := svc.CreateTenantAccessToken(app.CreateTenantAccessTokenInput{Name: "runtime", Scopes: []string{"task:read"}})
	if err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{"Authorization": "Bearer " + adminRaw}
	rr := requestHTTP(t, srv, http.MethodGet, "/api/v1/admin/tenant-access-tokens?all=true", headers)
	if rr.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), tenant.View.ID) || strings.Contains(rr.Body.String(), tenant.RawToken) {
		t.Fatalf("list body = %s", rr.Body.String())
	}
	rr = requestHTTP(t, srv, http.MethodDelete, "/api/v1/admin/tenant-access-tokens/"+tenant.View.ID, headers)
	if rr.Code != http.StatusOK {
		t.Fatalf("revoke status = %d body=%s", rr.Code, rr.Body.String())
	}
	rr = requestHTTP(t, srv, http.MethodGet, "/api/v1/admin/tenant-access-tokens?all=true", headers)
	if !strings.Contains(rr.Body.String(), `"revoked_at":`) {
		t.Fatalf("revoked token missing revoked_at: %s", rr.Body.String())
	}
}

func TestAdminCanModifyTenantAccessTokenProjects(t *testing.T) {
	srv, adminRaw, _ := newAdminTokenFixture(t)
	svc, err := app.NewService(app.ServiceOptions{Store: srv.store})
	if err != nil {
		t.Fatal(err)
	}
	project, err := svc.AddProject(app.AddProjectInput{Slug: "api", Name: "API"})
	if err != nil {
		t.Fatal(err)
	}
	tenant, err := svc.CreateTenantAccessToken(app.CreateTenantAccessTokenInput{Name: "runtime", Scopes: []string{"task:read"}})
	if err != nil {
		t.Fatal(err)
	}
	body := `{"projects":["api"]}`
	rr := requestHTTPBody(t, srv, http.MethodPatch, "/api/v1/admin/tenant-access-tokens/"+tenant.View.ID, body, map[string]string{
		"Authorization": "Bearer " + adminRaw,
		"Content-Type":  "application/json",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("modify status = %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"project_ids":["`+project.ID+`"]`) {
		t.Fatalf("project_ids not updated: %s", rr.Body.String())
	}
}

func TestAdminCanClearTenantAccessTokenExpiry(t *testing.T) {
	srv, adminRaw, _ := newAdminTokenFixture(t)
	svc, err := app.NewService(app.ServiceOptions{Store: srv.store})
	if err != nil {
		t.Fatal(err)
	}
	expiresIn := time.Hour
	tenant, err := svc.CreateTenantAccessToken(app.CreateTenantAccessTokenInput{Name: "runtime", Scopes: []string{"task:read"}, ExpiresIn: &expiresIn})
	if err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{"Authorization": "Bearer " + adminRaw, "Content-Type": "application/json"}
	rr := requestHTTPBody(t, srv, http.MethodPatch, "/api/v1/admin/tenant-access-tokens/"+tenant.View.ID, `{"expires_in_seconds":null}`, headers)
	if rr.Code != http.StatusOK {
		t.Fatalf("modify status = %d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Data struct {
			ExpiresAt *int64 `json:"expires_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.ExpiresAt != nil {
		t.Fatalf("expires_at = %v, want nil", *payload.Data.ExpiresAt)
	}
}

func TestAdminTokenModifyHTTP(t *testing.T) {
	srv, adminRaw, tokenID := newAdminTokenFixture(t)
	body := `{"scopes":["task:read","task:write"]}`
	rr := requestHTTPBody(t, srv, http.MethodPatch, "/api/v1/admin/tokens/"+tokenID, body, map[string]string{
		"Authorization": "Bearer " + adminRaw,
		"Content-Type":  "application/json",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("modify status = %d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Data struct {
			Scopes []string `json:"scopes"`
		} `json:"data"`
	}
	json.Unmarshal(rr.Body.Bytes(), &payload)
	if len(payload.Data.Scopes) != 2 {
		t.Fatalf("scopes = %v, want 2", payload.Data.Scopes)
	}
}

func TestAdminTokenRequiresAdminAuth(t *testing.T) {
	srv, adminRaw, _ := newAdminTokenFixture(t)
	// 用普通 PAT（fixture.token）而非 admin token 访问
	rr := requestHTTP(t, srv, http.MethodGet, "/api/v1/admin/tokens", map[string]string{
		"Authorization": "Bearer " + adminRaw + "-wrong",
	})
	assertHTTPErrorCode(t, rr, http.StatusUnauthorized, "admin_auth_invalid")
	_ = adminRaw
}
