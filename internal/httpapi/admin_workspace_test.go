package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/config"
)

// newAdminHTTPFixture 搭建一个已启用 admin token 的 HTTP server，
// 并预先创建一个有 owner alice + admin bob 的 dajee workspace。
func newAdminHTTPFixture(t *testing.T) (httpTokenFixture, string) {
	t.Helper()
	fixture := newHTTPServerWithTokenFixture(t, "workspace:write")
	adminRaw := "xuanchu_admin_secret"
	fixture.server.admin = config.AdminConfig{
		Enabled: true,
		Tokens:  []config.AdminTokenConfig{{Name: "ops-primary", Hash: auth.HashAdminToken(adminRaw), Enabled: true}},
	}
	// 创建 dajee workspace + owner alice。
	body := `{"slug":"dajee","name":"Dajee","visibility":"team","owner":{"name":"alice","email":"alice@example.com"}}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces", body, map[string]string{
		"Authorization": "Bearer " + adminRaw,
		"Content-Type":  "application/json",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("seed workspace: status=%d body=%s", rr.Code, rr.Body.String())
	}
	// 提升 bob 为 admin。
	bobBody := `{"name":"bob","email":"bob@example.com","role":"admin"}`
	rr = requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces/dajee/admins", bobBody, map[string]string{
		"Authorization": "Bearer " + adminRaw,
		"Content-Type":  "application/json",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("seed admin: status=%d body=%s", rr.Code, rr.Body.String())
	}
	fixture.server.router = fixture.server.newRouter()
	return fixture, adminRaw
}

func TestAdminSessionCapabilitiesIncludeWorkspaceAndActing(t *testing.T) {
	fixture, adminRaw := newAdminHTTPFixture(t)
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/admin/session", map[string]string{
		"Authorization": "Bearer " + adminRaw,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, capability := range []string{"workspace:list", "workspace:read", "acting_session:create", "acting_session:revoke"} {
		if !strings.Contains(body, `"`+capability+`"`) {
			t.Fatalf("missing capability %q in body: %s", capability, body)
		}
	}
}

func TestAdminWorkspaceListHTTP(t *testing.T) {
	fixture, adminRaw := newAdminHTTPFixture(t)
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/admin/workspaces", map[string]string{
		"Authorization": "Bearer " + adminRaw,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"slug":"dajee"`) {
		t.Fatalf("missing dajee: %s", body)
	}
	// member_counts 必须使用完整对象，created_by 必须是 UserInfo（含 name）。
	if !strings.Contains(body, `"member_counts":{`) {
		t.Fatalf("missing member_counts: %s", body)
	}
	if !strings.Contains(body, `"created_by":{"id":`) || !strings.Contains(body, `"name":"alice"`) {
		t.Fatalf("created_by not full UserInfo: %s", body)
	}
}

func TestAdminWorkspaceDetailHTTP(t *testing.T) {
	fixture, adminRaw := newAdminHTTPFixture(t)
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/admin/workspaces/dajee", map[string]string{
		"Authorization": "Bearer " + adminRaw,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"acting_candidates":[`) {
		t.Fatalf("missing acting_candidates: %s", body)
	}
	// members 中的 user 字段必须是 UserInfo 对象。
	if !strings.Contains(body, `"user":{"id":`) {
		t.Fatalf("member user not UserInfo: %s", body)
	}
}

func TestAdminActingSessionCreateHTTP(t *testing.T) {
	fixture, adminRaw := newAdminHTTPFixture(t)
	body := `{}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces/dajee/acting-sessions", body, map[string]string{
		"Authorization": "Bearer " + adminRaw,
		"Content-Type":  "application/json",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var envelope struct {
		Data adminActingSessionResponse `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	created := envelope.Data
	if !strings.HasPrefix(created.Token, "xuanchu_act_") {
		t.Fatalf("token = %q", created.Token)
	}
	if created.Workspace.Slug != "dajee" {
		t.Fatalf("workspace = %#v", created.Workspace)
	}
	if created.Actor.ID == "" || created.Actor.Name != "alice" {
		t.Fatalf("actor = %#v", created.Actor)
	}
	if created.AdminTokenName != "ops-primary" {
		t.Fatalf("admin token name = %q", created.AdminTokenName)
	}

	// acting token 必须能通过普通 /api/v1/me。
	meRR := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/me", map[string]string{
		"Authorization": "Bearer " + created.Token,
	})
	if meRR.Code != http.StatusOK {
		t.Fatalf("acting token /me status = %d body=%s", meRR.Code, meRR.Body.String())
	}
}

func TestAdminActingSessionCreateRejectsArchivedWorkspace(t *testing.T) {
	fixture, adminRaw := newAdminHTTPFixture(t)
	// 创建并归档一个 workspace。
	body := `{"slug":"legacy","owner":{"name":"legacyowner"}}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces", body, map[string]string{
		"Authorization": "Bearer " + adminRaw,
		"Content-Type":  "application/json",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("seed legacy: status=%d body=%s", rr.Code, rr.Body.String())
	}
	// 用普通 token 归档（owner 是 legacyowner，需要其 token；这里直接走 storage archive）。
	if err := fixture.server.store.DB().Exec("UPDATE workspaces SET archived_at = 1 WHERE slug = 'legacy'").Error; err != nil {
		t.Fatal(err)
	}

	createRR := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces/legacy/acting-sessions", `{}`, map[string]string{
		"Authorization": "Bearer " + adminRaw,
		"Content-Type":  "application/json",
	})
	assertHTTPErrorCode(t, createRR, http.StatusBadRequest, "workspace_archived")
}

func TestAdminActingSessionRawTokenNotInAuditPayload(t *testing.T) {
	fixture, adminRaw := newAdminHTTPFixture(t)
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces/dajee/acting-sessions", `{}`, map[string]string{
		"Authorization": "Bearer " + adminRaw,
		"Content-Type":  "application/json",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var envelope struct {
		Data adminActingSessionResponse `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	rawToken := envelope.Data.Token

	// 用 acting token 读 audit；payload 不能包含 raw token。
	auditRR := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/audit", map[string]string{
		"Authorization": "Bearer " + rawToken,
	})
	// 即使 audit 读取受限，只要成功就不能泄漏 raw token。
	if auditRR.Code == http.StatusOK {
		if strings.Contains(auditRR.Body.String(), rawToken) {
			t.Fatalf("audit payload leaked raw acting token: %s", auditRR.Body.String())
		}
	}
}

func TestAdminActingSessionExpiredReturnsUnauthorizedHTTP(t *testing.T) {
	fixture, adminRaw := newAdminHTTPFixture(t)
	// 创建一个 1 秒 TTL 的 acting session。
	createRR := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/admin/workspaces/dajee/acting-sessions", `{"expires_in":"1s"}`, map[string]string{
		"Authorization": "Bearer " + adminRaw,
		"Content-Type":  "application/json",
	})
	if createRR.Code != http.StatusCreated {
		t.Fatalf("create acting session: status=%d body=%s", createRR.Code, createRR.Body.String())
	}
	var envelope struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createRR.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	actingToken := envelope.Data.Token

	// 直接把 session 标记过期（避免真实等待 1 秒带来的 flake）。
	if err := fixture.server.store.DB().Exec("UPDATE admin_acting_sessions SET expires_at = 1").Error; err != nil {
		t.Fatal(err)
	}

	// 过期的 acting token 访问普通 API 必须返回 401 + admin_acting_session_expired。
	meRR := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/me", map[string]string{
		"Authorization": "Bearer " + actingToken,
	})
	assertHTTPErrorCode(t, meRR, http.StatusUnauthorized, "admin_acting_session_expired")
}

func TestNormalTokenCannotAccessAdminWorkspaceEndpoints(t *testing.T) {
	fixture, _ := newAdminHTTPFixture(t)
	// fixture.token 是普通 workspace token。
	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/admin/workspaces"},
		{http.MethodGet, "/api/v1/admin/workspaces/dajee"},
		{http.MethodPost, "/api/v1/admin/workspaces/dajee/acting-sessions"},
	} {
		rr := requestHTTPBody(t, fixture.server, tc.method, tc.path, `{}`, map[string]string{
			"Authorization": "Bearer " + fixture.token,
			"Content-Type":  "application/json",
		})
		if rr.Code != http.StatusUnauthorized || !strings.Contains(rr.Body.String(), "admin_auth_invalid") {
			t.Fatalf("%s %s: expected admin_auth_invalid, got status=%d body=%s", tc.method, tc.path, rr.Code, rr.Body.String())
		}
	}
}
