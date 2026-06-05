package app

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dajee/taskg/internal/storage"
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
	if out.View.User.ID != svc.Runtime().ActorUserID {
		t.Fatalf("view user id = %q want %q", out.View.User.ID, svc.Runtime().ActorUserID)
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
	other := mustCreateWorkspaceRecord(t, store, storage.Workspace{
		ID:           "ws-work",
		Slug:         "work",
		Name:         "Work",
		Visibility:   "team",
		SettingsJSON: "{}",
		CreatedAt:    100,
		ModifiedAt:   100,
	})
	mustUpsertMembershipRecord(t, store, storage.Membership{
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

func TestCreateTokenPATSilentlyDropsImpersonateScope(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	created, err := svc.CreateToken(CreateTokenInput{
		Name:          "pat-impersonate",
		Type:          "pat",
		Scopes:        []string{"task:read", "impersonate"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatalf("CreateToken() error = %v", err)
	}
	for _, s := range created.View.Scopes {
		if s == "impersonate" {
			t.Fatalf("PAT should not contain impersonate scope, got %v", created.View.Scopes)
		}
	}
}

func TestCreateTokenRejectsMemberCreatingImpersonateScope(t *testing.T) {
	store := newTestStore(t)
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace() error = %v", err)
	}
	memberUser := mustCreateUserRecord(t, store, storage.User{
		ID: "user-member", Name: "member", CreatedAt: 100, ModifiedAt: 100,
	})
	mustUpsertMembershipRecord(t, store, storage.Membership{
		UserID: memberUser.ID, WorkspaceID: ws.ID,
		Role: string(RoleMember), JoinedAt: 100, ModifiedAt: 100,
	})
	memberSvc := newTestServiceWithRuntime(t, store, 100, "member", "local")
	_, err = memberSvc.CreateToken(CreateTokenInput{
		Name:          "agent-impersonate",
		Type:          "agent",
		Scopes:        []string{"task:read", "impersonate"},
		WorkspaceRefs: []string{"local"},
	})
	// member 没有 token:write 权限，在 workspace 验证阶段就被拒绝
	if err == nil {
		t.Fatal("CreateToken() error = nil, want permission denied or scope denied")
	}
}

func TestCreateTokenRejectsImpersonateScopeOutsideParentToken(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	parent := &TokenView{
		Scopes:       []string{"task:read", "task:write"},
		WorkspaceIDs: []string{svc.Runtime().WorkspaceID},
	}
	_, err := svc.CreateToken(CreateTokenInput{
		Name:          "child-impersonate",
		Type:          "agent",
		Scopes:        []string{"task:read", "impersonate"},
		WorkspaceRefs: []string{"local"},
		ParentToken:   parent,
	})
	assertRuntimeCode(t, err, "token_scope_denied")
}

func TestCreateTokenAllowsAgentImpersonateScopeForOwner(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	created, err := svc.CreateToken(CreateTokenInput{
		Name:          "agent-impersonate",
		Type:          "agent",
		Scopes:        []string{"task:read", "task:write", "impersonate"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatalf("CreateToken() error = %v", err)
	}
	if created.View.Type != "agent" {
		t.Fatalf("type = %q, want agent", created.View.Type)
	}
	hasImpersonate := false
	for _, scope := range created.View.Scopes {
		if scope == "impersonate" {
			hasImpersonate = true
		}
	}
	if !hasImpersonate {
		t.Fatalf("scopes = %#v, want impersonate", created.View.Scopes)
	}
}

func TestCreateTokenAllowsChildImpersonateWhenParentHasIt(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	parent := &TokenView{
		Scopes:       []string{"task:read", "task:write", "impersonate"},
		WorkspaceIDs: []string{svc.Runtime().WorkspaceID},
	}
	_, err := svc.CreateToken(CreateTokenInput{
		Name:          "child-impersonate",
		Type:          "agent",
		Scopes:        []string{"task:read", "impersonate"},
		WorkspaceRefs: []string{"local"},
		ParentToken:   parent,
	})
	if err != nil {
		t.Fatalf("CreateToken() error = %v", err)
	}
}

func TestModifyTokenName(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	created, err := svc.CreateToken(CreateTokenInput{
		Name:          "old-name",
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}

	view, err := svc.ModifyToken(ModifyTokenInput{
		TokenID: created.View.ID,
		Name:    strptr("new-name"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if view.Name != "new-name" {
		t.Fatalf("name = %q, want new-name", view.Name)
	}
	audits := mustListAudit(t, svc)
	assertAuditAction(t, audits, "token.modified")
}

func TestModifyTokenScopes(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	created, err := svc.CreateToken(CreateTokenInput{
		Name:          "test",
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}

	view, err := svc.ModifyToken(ModifyTokenInput{
		TokenID: created.View.ID,
		Scopes:  []string{"task:read", "task:write"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Scopes) != 2 {
		t.Fatalf("scopes = %v, want 2", view.Scopes)
	}
}

func TestModifyTokenScopesWildcard(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	created, err := svc.CreateToken(CreateTokenInput{
		Name:          "test",
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}

	view, err := svc.ModifyToken(ModifyTokenInput{
		TokenID: created.View.ID,
		Scopes:  []string{"task:*"},
	})
	if err != nil {
		t.Fatal(err)
	}
	hasRead, hasWrite := false, false
	for _, s := range view.Scopes {
		if s == "task:read" {
			hasRead = true
		}
		if s == "task:write" {
			hasWrite = true
		}
	}
	if !hasRead || !hasWrite {
		t.Fatalf("scopes = %v, want task:read and task:write", view.Scopes)
	}
}

func TestModifyTokenPATSilentlyDropsImpersonate(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	created, err := svc.CreateToken(CreateTokenInput{
		Name:          "test",
		Type:          "pat",
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}

	view, err := svc.ModifyToken(ModifyTokenInput{
		TokenID: created.View.ID,
		Scopes:  []string{"task:read", "impersonate"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range view.Scopes {
		if s == "impersonate" {
			t.Fatal("PAT should not contain impersonate after modify")
		}
	}
}

func TestModifyTokenRejectsRevoked(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	created, err := svc.CreateToken(CreateTokenInput{
		Name:          "test",
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.RevokeToken(created.View.ID); err != nil {
		t.Fatal(err)
	}

	_, err = svc.ModifyToken(ModifyTokenInput{
		TokenID: created.View.ID,
		Name:    strptr("new-name"),
	})
	assertRuntimeCode(t, err, "token_revoked")
}

func TestModifyTokenRejectsExpired(t *testing.T) {
	svc, closeFn := newTestService(t, 200)
	defer closeFn()

	created, err := svc.CreateToken(CreateTokenInput{
		Name:          "test",
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}

	past := int64(100)
	if err := svc.tokenRepo.Update(created.View.ID, storage.TokenUpdates{ExpiresAt: &past}); err != nil {
		t.Fatal(err)
	}

	_, err = svc.ModifyToken(ModifyTokenInput{
		TokenID: created.View.ID,
		Name:    strptr("new-name"),
	})
	assertRuntimeCode(t, err, "token_expired")
}

func TestModifyTokenExpiresIn(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	created, err := svc.CreateToken(CreateTokenInput{
		Name:          "test",
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}

	ttl := 720 * time.Hour
	view, err := svc.ModifyToken(ModifyTokenInput{
		TokenID:   created.View.ID,
		ExpiresIn: &ttl,
	})
	if err != nil {
		t.Fatal(err)
	}
	if view.ExpiresAt == nil {
		t.Fatal("expires_at should not be nil")
	}
	expected := int64(100) + int64(ttl.Seconds())
	if *view.ExpiresAt != expected {
		t.Fatalf("expires_at = %d, want %d", *view.ExpiresAt, expected)
	}
}

func TestModifyTokenClearExpiry(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	created, err := svc.CreateToken(CreateTokenInput{
		Name:          "test",
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
		ExpiresIn:     ptrDuration(720 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	zero := time.Duration(0)
	view, err := svc.ModifyToken(ModifyTokenInput{
		TokenID:   created.View.ID,
		ExpiresIn: &zero,
	})
	if err != nil {
		t.Fatal(err)
	}
	if view.ExpiresAt != nil {
		t.Fatalf("expires_at should be nil after clear, got %d", *view.ExpiresAt)
	}
}

func TestModifyTokenRejectsNegativeDuration(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	created, err := svc.CreateToken(CreateTokenInput{
		Name:          "test",
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}

	neg := -1 * time.Hour
	_, err = svc.ModifyToken(ModifyTokenInput{
		TokenID:   created.View.ID,
		ExpiresIn: &neg,
	})
	assertRuntimeCode(t, err, "token_scope_invalid")
}

func TestModifyTokenRejectsInvalidScope(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	created, err := svc.CreateToken(CreateTokenInput{
		Name:          "test",
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.ModifyToken(ModifyTokenInput{
		TokenID: created.View.ID,
		Scopes:  []string{"invalid:scope"},
	})
	assertRuntimeCode(t, err, "token_scope_invalid")
}

func TestModifyTokenNoChangesReturnsCurrentView(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	created, err := svc.CreateToken(CreateTokenInput{
		Name:          "test",
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}

	view, err := svc.ModifyToken(ModifyTokenInput{
		TokenID: created.View.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if view.Name != "test" {
		t.Fatalf("name = %q, want test", view.Name)
	}
	if len(view.Scopes) != 1 || view.Scopes[0] != "task:read" {
		t.Fatalf("scopes = %v, want [task:read]", view.Scopes)
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
