package app

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dajee/taskg/internal/storage/sqlite"
)

func ptrDuration(value time.Duration) *time.Duration {
	return &value
}

func mustListAudit(t *testing.T, svc *Service) []AuditLogView {
	t.Helper()
	rows, err := svc.ListAudit(AuditListInput{Limit: 50})
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	return rows
}

func assertAuditAction(t *testing.T, rows []AuditLogView, want string) {
	t.Helper()
	for _, row := range rows {
		if row.Action == want {
			return
		}
	}
	t.Fatalf("audit actions missing %q: %#v", want, rows)
}

func assertRuntimeCode(t *testing.T, err error, want string) {
	t.Helper()
	switch e := err.(type) {
	case RuntimeError:
		if e.Code != want {
			t.Fatalf("RuntimeError code = %q, want %q", e.Code, want)
		}
	case PermissionError:
		if e.Code != want {
			t.Fatalf("PermissionError code = %q, want %q", e.Code, want)
		}
	default:
		t.Fatalf("err = %#v, want code %q", err, want)
	}
}

func TestCreateTokenStoresHashAndAudits(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	out, err := svc.CreateToken(CreateTokenInput{
		Name:          "cli",
		Type:          "pat",
		Scopes:        []string{"task:read", "task:write"},
		WorkspaceRefs: []string{"local"},
		ExpiresIn:     ptrDuration(720 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.RawToken, "taskg_pat_") {
		t.Fatalf("token = %q", out.RawToken)
	}
	if strings.Contains(out.Stored.TokenHash, out.RawToken) {
		t.Fatalf("raw token stored")
	}
	if out.View.UserID != svc.Runtime().ActorUserID {
		t.Fatalf("view user id = %q want %q", out.View.UserID, svc.Runtime().ActorUserID)
	}
	audits := mustListAudit(t, svc)
	assertAuditAction(t, audits, "token.create")
	var payload map[string]any
	if err := json.Unmarshal([]byte(audits[0].PayloadJSON), &payload); err != nil {
		t.Fatalf("json.Unmarshal(payload) error = %v", err)
	}
	if _, ok := payload["token"]; ok {
		t.Fatalf("audit payload leaked raw token: %#v", payload)
	}
}

func TestCreateAgentTokenRequiresExplicitWorkspaceAndScope(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	_, err := svc.CreateToken(CreateTokenInput{Name: "agent", Type: "agent", Scopes: []string{"task:read"}})
	assertRuntimeCode(t, err, "token_workspace_scope_invalid")
}

func TestCreateTokenRejectsProjectOutsideWorkspaceScope(t *testing.T) {
	store := newTestStore(t)
	owner := newTestServiceWithRuntime(t, store, 100, "local", "local")
	other := mustCreateWorkspaceRecord(t, store, sqlite.Workspace{
		ID:           "ws-work",
		Slug:         "work",
		Name:         "Work",
		Visibility:   "team",
		SettingsJSON: "{}",
		CreatedAt:    100,
		ModifiedAt:   100,
	})
	mustUpsertMembershipRecord(t, store, sqlite.Membership{
		UserID:      owner.Runtime().ActorUserID,
		WorkspaceID: other.ID,
		Role:        string(RoleAdmin),
		JoinedAt:    100,
		ModifiedAt:  100,
	})
	workSvc := newTestServiceWithRuntime(t, store, 100, "local", "work")
	project, err := workSvc.AddProject(AddProjectInput{Slug: "api", Name: "API"})
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}

	_, err = owner.CreateToken(CreateTokenInput{
		Name:          "cli",
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
		ProjectRefs:   []string{project.ID},
	})
	assertRuntimeCode(t, err, "token_project_scope_invalid")
}

func TestCreateTokenCannotExceedParentTokenScope(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	parent := &TokenView{
		Scopes:       []string{"task:read"},
		WorkspaceIDs: []string{svc.Runtime().WorkspaceID},
	}
	_, err := svc.CreateToken(CreateTokenInput{
		Name:          "too-broad-scope",
		Scopes:        []string{"task:read", "task:write"},
		WorkspaceRefs: []string{"local"},
		ParentToken:   parent,
	})
	assertRuntimeCode(t, err, "token_scope_denied")

	_, err = svc.CreateToken(CreateTokenInput{
		Name:        "global-workspace",
		Scopes:      []string{"task:read"},
		ParentToken: parent,
	})
	assertRuntimeCode(t, err, "workspace_scope_denied")

	if _, err := svc.CreateToken(CreateTokenInput{
		Name:          "narrow",
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
		ParentToken:   parent,
	}); err != nil {
		t.Fatalf("CreateToken(narrow) error = %v", err)
	}
}

func TestAuthenticateBearerToken(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	created, err := svc.CreateToken(CreateTokenInput{
		Name:          "cli",
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}

	authn, err := svc.AuthenticateBearerToken(created.RawToken)
	if err != nil {
		t.Fatalf("AuthenticateBearerToken() error = %v", err)
	}
	if authn.Token.ID != created.View.ID {
		t.Fatalf("token id = %q want %q", authn.Token.ID, created.View.ID)
	}
	listed, err := svc.ListTokens(ListTokensInput{})
	if err != nil {
		t.Fatalf("ListTokens() error = %v", err)
	}
	if len(listed) != 1 || listed[0].LastUsedAt == nil || *listed[0].LastUsedAt != 100 {
		t.Fatalf("ListTokens().LastUsedAt = %#v, want 100", listed)
	}
	if _, err := svc.AuthenticateBearerToken(created.RawToken + "x"); err == nil {
		t.Fatal("AuthenticateBearerToken(invalid) error = nil")
	} else {
		assertRuntimeCode(t, err, "auth_invalid_token")
	}
	if err := svc.RevokeToken(created.View.ID); err != nil {
		t.Fatalf("RevokeToken() error = %v", err)
	}
	if _, err := svc.AuthenticateBearerToken(created.RawToken); err == nil {
		t.Fatal("AuthenticateBearerToken(revoked) error = nil")
	} else {
		assertRuntimeCode(t, err, "auth_token_revoked")
	}
}
