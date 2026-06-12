package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

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
