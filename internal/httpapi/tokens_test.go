package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/app"
)

// tokenModifyResponse 用于解析 PATCH /api/v1/tokens/{ref} 的响应。
type tokenModifyResponse struct {
	Data struct {
		ID           string   `json:"id"`
		Name         string   `json:"name"`
		Type         string   `json:"type"`
		WorkspaceIDs []string `json:"workspace_ids"`
		ProjectIDs   []string `json:"project_ids"`
		Scopes       []string `json:"scopes"`
		ExpiresAt    *int64   `json:"expires_at"`
	} `json:"data"`
}

// tokenCreateResponse 用于解析 POST /api/v1/tokens 的响应。
type tokenCreateResponse struct {
	Data struct {
		Token string `json:"token"`
		ID    string `json:"id"`
		Type  string `json:"type"`
	} `json:"data"`
}

// newHTTPServerWithAgentTokenFixture 建一个带 agent token（绑定 local workspace）的 fixture，
// 并预建第二个 workspace "team"，用于测试 workspace 修改。
func newHTTPServerWithAgentTokenFixture(t *testing.T, scopes ...string) (httpTokenFixture, string) {
	t.Helper()
	store := openHTTPTestStore(t)
	svc, err := app.NewService(app.ServiceOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	// 预建第二个 workspace，owner 默认加入
	team, err := svc.AddWorkspace(app.AddWorkspaceInput{Slug: "team", Name: "Team"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateToken(app.CreateTokenInput{
		Name:          "agent-test",
		Type:          "agent",
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
	}, team.ID
}

func TestTenantAccessTokenManagementAPI(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "token:read", "token:write", "task:read")
	body := `{"name":"runtime","scopes":["task:read"],"expires_in_seconds":3600}`
	authHeader := map[string]string{"Authorization": "Bearer " + fixture.token, "Content-Type": "application/json"}
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/tenant-access-tokens", body, authHeader)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"token":"xuanchu_tenant_`) {
		t.Fatalf("body=%s", rr.Body.String())
	}
	var created tokenCreateResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	rr = requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/tenant-access-tokens", authHeader)
	if rr.Code != http.StatusOK || strings.Contains(rr.Body.String(), `"token":"xuanchu_tenant_`) {
		t.Fatalf("list leaked raw token or failed: %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"type":"tenant_access_token"`) {
		t.Fatalf("list body=%s", rr.Body.String())
	}

	rr = requestHTTPBody(t, fixture.server, http.MethodPatch, "/api/v1/tenant-access-tokens/"+created.Data.ID, `{"expires_in_seconds":null}`, authHeader)
	if rr.Code != http.StatusOK {
		t.Fatalf("clear expires status=%d body=%s", rr.Code, rr.Body.String())
	}
	var modified tokenModifyResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &modified); err != nil {
		t.Fatal(err)
	}
	if modified.Data.ExpiresAt != nil {
		t.Fatalf("expires_at = %v, want nil", *modified.Data.ExpiresAt)
	}
}

func TestPATCannotManageTenantTokenBeyondOwnScope(t *testing.T) {
	limited := newHTTPServerWithTokenFixture(t, "token:read", "token:write")
	body := `{"name":"runtime","scopes":["task:read"],"expires_in_seconds":3600}`
	limitedHeaders := map[string]string{"Authorization": "Bearer " + limited.token, "Content-Type": "application/json"}
	rr := requestHTTPBody(t, limited.server, http.MethodPost, "/api/v1/tenant-access-tokens", body, limitedHeaders)
	assertHTTPErrorCode(t, rr, http.StatusForbidden, "token_scope_denied")

	full := newHTTPServerWithTokenFixture(t, "token:read", "token:write", "task:read")
	fullHeaders := map[string]string{"Authorization": "Bearer " + full.token, "Content-Type": "application/json"}
	rr = requestHTTPBody(t, full.server, http.MethodPost, "/api/v1/tenant-access-tokens", body, fullHeaders)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create full status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created tokenCreateResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	limitedSameStore, err := app.NewService(app.ServiceOptions{Store: full.server.store})
	if err != nil {
		t.Fatal(err)
	}
	limitedToken, err := limitedSameStore.CreateToken(app.CreateTokenInput{
		Name:          "limited-manager",
		Type:          "pat",
		UserRef:       "local",
		Scopes:        []string{"token:read", "token:write"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	limitedSameStoreHeaders := map[string]string{"Authorization": "Bearer " + limitedToken.RawToken, "Content-Type": "application/json"}
	rr = requestHTTPBody(t, full.server, http.MethodPatch, "/api/v1/tenant-access-tokens/"+created.Data.ID, `{"name":"escaped"}`, limitedSameStoreHeaders)
	assertHTTPErrorCode(t, rr, http.StatusForbidden, "token_scope_denied")
	rr = requestHTTP(t, full.server, http.MethodDelete, "/api/v1/tenant-access-tokens/"+created.Data.ID, limitedSameStoreHeaders)
	assertHTTPErrorCode(t, rr, http.StatusForbidden, "token_scope_denied")
}

func TestModifyTokenWorkspacesHTTPRejectsBeyondBearerScope(t *testing.T) {
	fixture, _ := newHTTPServerWithAgentTokenFixture(t, "task:read", "token:write")
	body := `{"workspaces":["team"]}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPatch, "/api/v1/tokens/"+fixture.id, body, map[string]string{
		"Authorization": "Bearer " + fixture.token,
	})
	assertHTTPErrorCode(t, rr, http.StatusForbidden, "workspace_scope_denied")
}

func TestModifyTokenClearWorkspacesHTTP(t *testing.T) {
	fixture, _ := newHTTPServerWithAgentTokenFixture(t, "task:read", "token:write")
	// agent token 清空 workspace 应被拒
	body := `{"workspaces":[]}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPatch, "/api/v1/tokens/"+fixture.id, body, map[string]string{
		"Authorization": "Bearer " + fixture.token,
	})
	assertHTTPErrorCode(t, rr, http.StatusBadRequest, "token_agent_requires_workspace")
}

func TestModifyTokenProjectsHTTP(t *testing.T) {
	store := openHTTPTestStore(t)
	svc, err := app.NewService(app.ServiceOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	// 在 local 建 project
	project, err := svc.AddProject(app.AddProjectInput{Slug: "demo", Name: "Demo"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateToken(app.CreateTokenInput{
		Name:          "agent-test",
		Type:          "agent",
		Scopes:        []string{"task:read", "token:write"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture := httpTokenFixture{
		server: NewServer(Options{Store: store}),
		token:  created.RawToken,
		id:     created.View.ID,
	}
	body := `{"projects":["demo"]}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPatch, "/api/v1/tokens/"+fixture.id, body, map[string]string{
		"Authorization": "Bearer " + fixture.token,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp tokenModifyResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Data.ProjectIDs) != 1 || resp.Data.ProjectIDs[0] != project.ID {
		t.Fatalf("project_ids = %v, want [%s]", resp.Data.ProjectIDs, project.ID)
	}
}

func TestModifyTokenOmitFieldHTTP(t *testing.T) {
	fixture, _ := newHTTPServerWithAgentTokenFixture(t, "task:read", "token:write")
	// 只传 name，workspace/project 字段缺省，不应被清空
	body := `{"name":"renamed"}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPatch, "/api/v1/tokens/"+fixture.id, body, map[string]string{
		"Authorization": "Bearer " + fixture.token,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp tokenModifyResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.Name != "renamed" {
		t.Fatalf("name = %q, want renamed", resp.Data.Name)
	}
	// workspace 不应被清空（仍含 local）
	if len(resp.Data.WorkspaceIDs) == 0 {
		t.Fatalf("workspace_ids should not be cleared when omitted, got %v", resp.Data.WorkspaceIDs)
	}
}
