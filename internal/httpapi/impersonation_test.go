package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

func newHTTPImpersonationFixture(t *testing.T) (store *storage.Store, srv *Server, ownerSvc *app.Service, agentToken string, agentTokenID string, aliceUser storage.User) {
	t.Helper()
	store = openHTTPTestStore(t)
	ownerSvc, err := app.NewService(app.ServiceOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	aliceUser = mustCreateHTTPUser(t, store, storage.User{
		ID: "user-alice-imp", Name: "alice-imp", CreatedAt: 100, ModifiedAt: 100,
	})
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	mustUpsertHTTPMembership(t, store, storage.Membership{
		UserID: aliceUser.ID, WorkspaceID: ws.ID,
		Role: string(app.RoleMember), JoinedAt: 100, ModifiedAt: 100,
	})
	created, err := ownerSvc.CreateToken(app.CreateTokenInput{
		Name:          "imp-agent",
		Type:          "agent",
		Scopes:        []string{"task:read", "task:write", "impersonate"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv = NewServer(Options{Store: store})
	return store, srv, ownerSvc, created.RawToken, created.View.ID, aliceUser
}

func TestImpersonationRejectsTokenWithoutScope(t *testing.T) {
	store := openHTTPTestStore(t)
	svc, err := app.NewService(app.ServiceOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateToken(app.CreateTokenInput{
		Name:          "no-impersonate",
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(Options{Store: store})

	rr := requestHTTP(t, srv, http.MethodGet, "/api/v1/tasks", map[string]string{
		"Authorization": "Bearer " + created.RawToken,
		"X-Xuanchu-As":  "alice",
	})
	assertHTTPErrorCode(t, rr, http.StatusForbidden, "token_scope_denied")
}

func TestImpersonationRejectsPAT(t *testing.T) {
	store := openHTTPTestStore(t)
	svc, err := app.NewService(app.ServiceOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateToken(app.CreateTokenInput{
		Name:          "pat-imp",
		Type:          "pat",
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(Options{Store: store})

	rr := requestHTTP(t, srv, http.MethodGet, "/api/v1/tasks", map[string]string{
		"Authorization": "Bearer " + created.RawToken,
		"X-Xuanchu-As":  "alice",
	})
	assertHTTPErrorCode(t, rr, http.StatusForbidden, "token_scope_denied")
}

func TestImpersonationReturnsMembershipNotFoundForUnknownUser(t *testing.T) {
	_, srv, _, agentToken, _, _ := newHTTPImpersonationFixture(t)

	rr := requestHTTP(t, srv, http.MethodGet, "/api/v1/tasks", map[string]string{
		"Authorization": "Bearer " + agentToken,
		"X-Xuanchu-As":  "nonexistent-user",
	})
	assertHTTPErrorCode(t, rr, http.StatusForbidden, "membership_not_found")
}

func TestImpersonationTaskActionUsesSubjectIdentity(t *testing.T) {
	_, srv, ownerSvc, agentToken, _, aliceUser := newHTTPImpersonationFixture(t)

	ownerSvc.Add(app.AddInput{Title: "alice task", Assignees: []string{aliceUser.ID}})

	rr := requestHTTP(t, srv, http.MethodGet, "/api/v1/tasks", map[string]string{
		"Authorization": "Bearer " + agentToken,
		"X-Xuanchu-As":  "alice-imp",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var list struct {
		Data []struct {
			Title     string `json:"title"`
			Assignees []struct {
				Name string `json:"name"`
			} `json:"assignees"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Data) != 1 || list.Data[0].Title != "alice task" {
		t.Fatalf("tasks = %s", rr.Body.String())
	}
}

// TestHTTPImpersonationUsesDecisionForAccessLog 验证授权 Decision 驱动 access log：
// impersonation 成功后，stderr 中应出现 delegator_user_id / delegator_token_id，
// 且 actor 为 subject 用户。这是 Decision 被消费的回归断言。
func TestHTTPImpersonationUsesDecisionForAccessLog(t *testing.T) {
	store, _, ownerSvc, agentToken, agentTokenID, aliceUser := newHTTPImpersonationFixture(t)
	_ = ownerSvc

	var stderr bytes.Buffer
	srv := NewServer(Options{Store: store, Stderr: &stderr})

	rr := requestHTTP(t, srv, http.MethodGet, "/api/v1/tasks", map[string]string{
		"Authorization": "Bearer " + agentToken,
		"X-Xuanchu-As":  "alice-imp",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}

	logOutput := stderr.String()
	if !strings.Contains(logOutput, "delegator_user_id=") {
		t.Fatalf("access log missing delegator_user_id: %s", logOutput)
	}
	if !strings.Contains(logOutput, "delegator_token_id="+agentTokenID) {
		t.Fatalf("access log missing delegator_token_id=%s: %s", agentTokenID, logOutput)
	}
	if !strings.Contains(logOutput, "actor_user_id="+aliceUser.ID) {
		t.Fatalf("access log missing actor_user_id=%s: %s", aliceUser.ID, logOutput)
	}
}

func TestImpersonationWithoutHeaderUsesTokenIdentity(t *testing.T) {
	_, srv, ownerSvc, agentToken, _, _ := newHTTPImpersonationFixture(t)

	ownerSvc.Add(app.AddInput{Title: "owner task"})

	rr := requestHTTP(t, srv, http.MethodGet, "/api/v1/tasks", map[string]string{
		"Authorization": "Bearer " + agentToken,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestImpersonationWorkspaceRequiredForMultiWorkspace(t *testing.T) {
	store := openHTTPTestStore(t)
	svc, err := app.NewService(app.ServiceOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	work, err := svc.AddWorkspace(app.AddWorkspaceInput{Slug: "work-imp", Name: "Work Imp"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateToken(app.CreateTokenInput{
		Name:          "multi-ws-agent",
		Type:          "agent",
		Scopes:        []string{"task:read", "impersonate"},
		WorkspaceRefs: []string{"local", work.Slug},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(Options{Store: store})

	rr := requestHTTP(t, srv, http.MethodGet, "/api/v1/tasks", map[string]string{
		"Authorization": "Bearer " + created.RawToken,
		"X-Xuanchu-As":  "local",
	})
	assertHTTPErrorCode(t, rr, http.StatusBadRequest, "workspace_required")
}

func mustCreateHTTPUser(t *testing.T, store *storage.Store, user storage.User) storage.User {
	t.Helper()
	created, err := storage.NewUserRepository(store.DB()).Create(user)
	if err != nil {
		t.Fatalf("Create(user %s) error = %v", user.Name, err)
	}
	return created
}
