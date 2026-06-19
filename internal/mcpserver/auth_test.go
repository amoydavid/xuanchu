package mcpserver

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

type testClock struct {
	now int64
}

func (c testClock) Unix() int64 { return c.now }

func (c testClock) Location() *time.Location {
	return time.Local
}

func newMCPTestStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func newMCPTestService(t *testing.T, store *storage.Store) *app.Service {
	t.Helper()
	svc, err := app.NewService(app.ServiceOptions{Store: store, Clock: testClock{now: 100}})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func mustCreateMCPToken(t *testing.T, svc *app.Service, scopes []string, workspaceRefs []string, projectRefs []string) string {
	t.Helper()
	created, err := svc.CreateToken(app.CreateTokenInput{
		Name:          "mcp-test",
		Scopes:        scopes,
		WorkspaceRefs: workspaceRefs,
		ProjectRefs:   projectRefs,
	})
	if err != nil {
		t.Fatal(err)
	}
	return created.RawToken
}

func TestRuntimeFactoryServiceForStdioUsesLocalRuntime(t *testing.T) {
	store := newMCPTestStore(t)
	factory := RuntimeFactory{Store: store, Clock: testClock{now: 100}}

	svc, err := factory.ServiceForStdio(context.Background(), RequestScopeInput{}, "", app.PermissionTaskRead)
	if err != nil {
		t.Fatalf("ServiceForStdio() error = %v", err)
	}
	rt := svc.Runtime()
	if rt.ActorName != "local" || rt.WorkspaceSlug != "local" {
		t.Fatalf("runtime = %#v, want local actor/workspace", rt)
	}
}

func TestRuntimeFactoryServiceForStdioWorkspaceOverride(t *testing.T) {
	store := newMCPTestStore(t)
	owner := newMCPTestService(t, store)
	if _, err := owner.AddWorkspace(app.AddWorkspaceInput{Slug: "team", Name: "Team"}); err != nil {
		t.Fatal(err)
	}
	factory := RuntimeFactory{Store: store, Clock: testClock{now: 100}}

	svc, err := factory.ServiceForStdio(context.Background(), RequestScopeInput{Workspace: "team"}, "", app.PermissionWorkspaceRead)
	if err != nil {
		t.Fatalf("ServiceForStdio() error = %v", err)
	}
	if got := svc.Runtime().WorkspaceSlug; got != "team" {
		t.Fatalf("workspace = %q, want team", got)
	}
}

func TestRuntimeFactoryServiceForStdioProjectMismatch(t *testing.T) {
	store := newMCPTestStore(t)
	owner := newMCPTestService(t, store)
	project, err := owner.AddProject(app.AddProjectInput{Slug: "alpha", Name: "Alpha"})
	if err != nil {
		t.Fatal(err)
	}
	factory := RuntimeFactory{Store: store, Clock: testClock{now: 100}}

	_, err = factory.ServiceForStdio(context.Background(), RequestScopeInput{Project: "missing", ProjectID: project.ID}, "", app.PermissionProjectRead)
	assertMCPRuntimeCode(t, err, "project_not_found")
}

func TestRuntimeFactoryServiceForHTTPRejectsMissingToken(t *testing.T) {
	store := newMCPTestStore(t)
	factory := RuntimeFactory{Store: store, Clock: testClock{now: 100}}
	req, _ := http.NewRequest(http.MethodPost, "/mcp", nil)

	_, err := factory.ServiceForHTTP(req, RequestScopeInput{}, "task:read", app.PermissionTaskRead)
	assertMCPRuntimeCode(t, err, "auth_missing_token")
}

func TestRuntimeFactoryServiceForHTTPRejectsProjectOutsideAllowlist(t *testing.T) {
	store := newMCPTestStore(t)
	owner := newMCPTestService(t, store)
	alpha, err := owner.AddProject(app.AddProjectInput{Slug: "alpha", Name: "Alpha"})
	if err != nil {
		t.Fatal(err)
	}
	beta, err := owner.AddProject(app.AddProjectInput{Slug: "beta", Name: "Beta"})
	if err != nil {
		t.Fatal(err)
	}
	token := mustCreateMCPToken(t, owner, []string{"project:read"}, []string{"local"}, []string{alpha.ID})
	req, _ := http.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	factory := RuntimeFactory{Store: store, Clock: testClock{now: 100}}

	_, err = factory.ServiceForHTTP(req, RequestScopeInput{Workspace: "local", ProjectID: beta.ID}, "project:read", app.PermissionProjectRead)
	assertMCPRuntimeCode(t, err, "project_scope_denied")
}

func TestRuntimeFactoryServiceForHTTPRequiresWorkspaceForAmbiguousProjectSlug(t *testing.T) {
	store := newMCPTestStore(t)
	owner := newMCPTestService(t, store)
	if _, err := owner.AddWorkspace(app.AddWorkspaceInput{Slug: "team", Name: "Team"}); err != nil {
		t.Fatal(err)
	}
	token := mustCreateMCPToken(t, owner, []string{"project:read"}, nil, nil)
	req, _ := http.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	factory := RuntimeFactory{Store: store, Clock: testClock{now: 100}}

	_, err := factory.ServiceForHTTP(req, RequestScopeInput{Project: "alpha"}, "project:read", app.PermissionProjectRead)
	assertMCPRuntimeCode(t, err, "workspace_required")
}

func TestRuntimeFactoryServiceForHTTPProjectIDSelectsOwningWorkspace(t *testing.T) {
	store := newMCPTestStore(t)
	owner := newMCPTestService(t, store)
	work, err := owner.AddWorkspace(app.AddWorkspaceInput{Slug: "work", Name: "Work"})
	if err != nil {
		t.Fatal(err)
	}
	workSvc, err := app.NewService(app.ServiceOptions{Store: store, Clock: testClock{now: 100}, WorkspaceRef: work.Slug})
	if err != nil {
		t.Fatal(err)
	}
	project, err := workSvc.AddProject(app.AddProjectInput{Slug: "api", Name: "API"})
	if err != nil {
		t.Fatal(err)
	}
	token := mustCreateMCPToken(t, owner, []string{"project:read"}, nil, nil)
	req, _ := http.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	factory := RuntimeFactory{Store: store, Clock: testClock{now: 100}}

	svc, err := factory.ServiceForHTTP(req, RequestScopeInput{ProjectID: project.ID}, "project:read", app.PermissionProjectRead)
	if err != nil {
		t.Fatalf("ServiceForHTTP() error = %v", err)
	}
	if got := svc.Runtime().WorkspaceID; got != work.ID {
		t.Fatalf("workspace = %q, want %q", got, work.ID)
	}
}

// TestServiceForHTTPUsesImpersonationDecision 验证 HTTP MCP 经授权 Decision 后，
// runtime 的 actor 为 subject，且 delegator 字段被填充。
// TestServiceForHTTPImpersonationWorkspaceRequired 验证 HTTP MCP 在 impersonation +
// 多 workspace 歧义场景下，与 HTTP API 一样返回 workspace_required。
// 这是 spec §11.1「HTTP MCP 与 HTTP API 行为一致」的回归断言。
func TestServiceForHTTPImpersonationWorkspaceRequired(t *testing.T) {
	store := newMCPTestStore(t)
	owner := newMCPTestService(t, store)
	if _, err := owner.AddWorkspace(app.AddWorkspaceInput{Slug: "team", Name: "Team"}); err != nil {
		t.Fatal(err)
	}
	created, err := owner.CreateToken(app.CreateTokenInput{
		Name:          "multi-ws-agent",
		Type:          "agent",
		Scopes:        []string{"task:read", "impersonate"},
		WorkspaceRefs: []string{"local", "team"},
	})
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+created.RawToken)
	req.Header.Set("X-Xuanchu-As", "local")
	factory := RuntimeFactory{Store: store, Clock: testClock{now: 100}}

	// 不显式 workspace，token 可见 local + team，应返回 workspace_required，
	// 与 HTTP API TestImpersonationWorkspaceRequiredForMultiWorkspace 一致。
	_, err = factory.ServiceForHTTP(req, RequestScopeInput{}, "task:read", app.PermissionTaskRead)
	assertMCPRuntimeCode(t, err, "workspace_required")
}

func TestServiceForHTTPUsesImpersonationDecision(t *testing.T) {
	store := newMCPTestStore(t)
	owner := newMCPTestService(t, store)
	alice, err := storage.NewUserRepository(store.DB()).Create(storage.User{
		ID: "user-alice-mcp", Name: "alice-mcp", CreatedAt: 100, ModifiedAt: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.NewMemberRepository(store.DB()).Upsert(storage.Membership{
		UserID: alice.ID, WorkspaceID: ws.ID,
		Role: string(app.RoleMember), JoinedAt: 100, ModifiedAt: 100,
	}); err != nil {
		t.Fatal(err)
	}
	created, err := owner.CreateToken(app.CreateTokenInput{
		Name:          "imp-agent",
		Type:          "agent",
		Scopes:        []string{"task:read", "impersonate"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+created.RawToken)
	req.Header.Set("X-Xuanchu-As", "alice-mcp")
	factory := RuntimeFactory{Store: store, Clock: testClock{now: 100}}

	svc, err := factory.ServiceForHTTP(req, RequestScopeInput{Workspace: "local"}, "task:read", app.PermissionTaskRead)
	if err != nil {
		t.Fatalf("ServiceForHTTP() error = %v", err)
	}
	rt := svc.Runtime()
	if rt.ActorUserID != alice.ID {
		t.Fatalf("actor = %q, want subject %q", rt.ActorUserID, alice.ID)
	}
	if rt.DelegatorTokenID != created.View.ID {
		t.Fatalf("delegator token = %q, want %q", rt.DelegatorTokenID, created.View.ID)
	}
	if rt.DelegatorUserID == "" {
		t.Fatal("delegator user id is empty")
	}
	if rt.Role != app.RoleMember {
		t.Fatalf("role = %q, want member", rt.Role)
	}
}

func assertMCPRuntimeCode(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want %s", want)
	}
	runtimeErr, ok := err.(app.RuntimeError)
	if !ok {
		t.Fatalf("error = %#v, want app.RuntimeError(%s)", err, want)
	}
	if runtimeErr.Code != want {
		t.Fatalf("code = %q, want %q (err=%v)", runtimeErr.Code, want, err)
	}
}
