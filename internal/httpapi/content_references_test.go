package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/attachments"
	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

// newHTTPContentRefFixture 构造一个带附件运行时和 task:* + workspace:* scope 的 fixture，
// 并预置一个 workspace member。
func newHTTPContentRefFixture(t *testing.T) (httpTokenFixture, storage.User) {
	t.Helper()
	store := openHTTPTestStore(t)
	cfg := attachments.DefaultConfig(filepath.Join(t.TempDir(), "attachments"))
	rt, err := app.NewAttachmentRuntime(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	_ = rt
	svc, err := app.NewService(app.ServiceOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateToken(app.CreateTokenInput{
		Name:          "http-contentref",
		Scopes:        []string{auth.ScopeTaskRead, auth.ScopeTaskWrite, "workspace:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// 预置一个 member。
	alice, err := storage.NewUserRepository(store.DB()).Create(storage.User{ID: uuid.NewString(), Name: "alice-contentref", DisplayName: "Alice ContentRef", CreatedAt: 1, ModifiedAt: 1})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := storage.NewMemberRepository(store.DB()).Upsert(storage.Membership{UserID: alice.ID, WorkspaceID: svc.Runtime().WorkspaceID, Role: string(app.RoleMember), JoinedAt: 1, ModifiedAt: 1}); err != nil {
		t.Fatalf("upsert membership: %v", err)
	}

	server := NewServer(Options{
		Store:           store,
		ResourceBaseURL: httpTestResourceBaseURL,
		ConfigSecretKey: httpTestSecretKeyBase64(),
	})
	return httpTokenFixture{server: server, token: created.RawToken, id: created.View.ID}, alice
}

func TestContentReferenceSuggestReturnsUsers(t *testing.T) {
	fixture, alice := newHTTPContentRefFixture(t)
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/content-references/suggestions?type=user&q=Alice&workspace=local", authHeader(fixture.token))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data struct {
			Results []struct {
				Type string `json:"type"`
				User struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"user"`
			} `json:"results"`
			Count int `json:"count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rr.Body.String())
	}
	if resp.Data.Count == 0 {
		t.Fatalf("no results, body=%s", rr.Body.String())
	}
	found := false
	for _, r := range resp.Data.Results {
		if r.Type == "user" && r.User.ID == alice.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("alice not in results: %#v", resp.Data.Results)
	}
}

// TestContentReferenceSuggestEmptyQueryReturnsMembers 验证空 query（不传 q）时
// 返回 workspace 全部成员，让用户敲 @ 就看到候选列表。
func TestContentReferenceSuggestEmptyQueryReturnsMembers(t *testing.T) {
	fixture, alice := newHTTPContentRefFixture(t)
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/content-references/suggestions?type=user&workspace=local", authHeader(fixture.token))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data struct {
			Results []struct {
				Type string `json:"type"`
				User struct {
					ID string `json:"id"`
				} `json:"user"`
			} `json:"results"`
			Count int `json:"count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rr.Body.String())
	}
	found := false
	for _, r := range resp.Data.Results {
		if r.Type == "user" && r.User.ID == alice.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("empty query should return all members, alice missing: %#v", resp.Data.Results)
	}
}

func TestContentReferenceSuggestRejectsInvalidType(t *testing.T) {
	fixture, _ := newHTTPContentRefFixture(t)
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/content-references/suggestions?type=project&q=x&workspace=local", authHeader(fixture.token))
	assertHTTPErrorCode(t, rr, http.StatusBadRequest, "content_reference_query_invalid")
}

func TestContentReferenceResolveReturnsUnavailableForMissing(t *testing.T) {
	fixture, _ := newHTTPContentRefFixture(t)
	body := `{"references":[{"type":"user","id":"00000000-0000-0000-0000-000000000099"},{"type":"task","id":"00000000-0000-0000-0000-000000000098"}]}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/content-references/resolve?workspace=local", body, authHeader(fixture.token))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Count(rr.Body.String(), `"status":"unavailable"`) != 2 {
		t.Fatalf("expected 2 unavailable, body=%s", rr.Body.String())
	}
}

func TestContentReferenceResolveAllowsMixedResultsForWorkspaceOnlyToken(t *testing.T) {
	fixture, alice := newHTTPContentRefFixture(t)
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	project, err := svc.AddProject(app.AddProjectInput{Slug: "resolvonl", Name: "Resolve only"})
	if err != nil {
		t.Fatal(err)
	}
	tsk, err := svc.Add(app.AddInput{Title: "not task readable", Project: &project.Slug})
	if err != nil {
		t.Fatal(err)
	}
	workspaceOnly, err := svc.CreateToken(app.CreateTokenInput{
		Name: "workspace-only-resolve", Scopes: []string{auth.ScopeWorkspaceRead}, WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := `{"references":[{"type":"user","id":"` + alice.ID + `"},{"type":"task","id":"` + tsk.UUID + `"}]}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/content-references/resolve?workspace=local", body, authHeader(workspaceOnly.RawToken))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"type":"user","id":"`+alice.ID+`","status":"resolved"`) {
		t.Fatalf("workspace-readable user should resolve: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"type":"task","id":"`+tsk.UUID+`","status":"unavailable"`) {
		t.Fatalf("task without task:read must be unavailable: %s", rr.Body.String())
	}
}

func TestContentReferenceResolveRejectsTooMany(t *testing.T) {
	fixture, _ := newHTTPContentRefFixture(t)
	var refs []string
	for i := 0; i < 201; i++ {
		refs = append(refs, `{"type":"user","id":"x"}`)
	}
	body := `{"references":[` + strings.Join(refs, ",") + `]}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/content-references/resolve?workspace=local", body, authHeader(fixture.token))
	assertHTTPErrorCode(t, rr, http.StatusBadRequest, "description_reference_limit_exceeded")
}
