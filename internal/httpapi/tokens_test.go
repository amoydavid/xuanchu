package httpapi

import (
	"encoding/json"
	"net/http"
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

func TestModifyTokenWorkspacesHTTP(t *testing.T) {
	fixture, teamID := newHTTPServerWithAgentTokenFixture(t, "task:read", "token:write")
	body := `{"workspaces":["team"]}`
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
	if len(resp.Data.WorkspaceIDs) != 1 || resp.Data.WorkspaceIDs[0] != teamID {
		t.Fatalf("workspace_ids = %v, want [%s]", resp.Data.WorkspaceIDs, teamID)
	}
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
