package app

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/authz"
	"git.dajee.net/dajee/xuanchu/internal/storage"
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
	if !strings.HasPrefix(out.RawToken, "xuanchu_pat_") {
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

func TestAuthenticateBearerTokenAcceptsLegacyShortPrefix(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	created, err := svc.CreateToken(CreateTokenInput{
		Name:          "legacy",
		Type:          "pat",
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	legacyPrefix := created.RawToken[:16]
	if legacyPrefix == created.View.Prefix {
		t.Fatalf("legacy prefix %q unexpectedly equals current prefix", legacyPrefix)
	}
	if err := svc.store.DB().Exec("UPDATE api_tokens SET token_prefix = ? WHERE id = ?", legacyPrefix, created.View.ID).Error; err != nil {
		t.Fatal(err)
	}

	authn, err := svc.AuthenticateBearerToken(created.RawToken)
	if err != nil {
		t.Fatalf("AuthenticateBearerToken() error = %v", err)
	}
	if authn.Token.ID != created.View.ID || authn.Token.Prefix != legacyPrefix {
		t.Fatalf("auth token = %#v, want id=%s prefix=%s", authn.Token, created.View.ID, legacyPrefix)
	}
}

func TestModifyTokenAcceptsPrefix(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	created, err := svc.CreateToken(CreateTokenInput{
		Name:          "cli",
		Type:          "pat",
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	renamed := "renamed-by-prefix"
	view, err := svc.ModifyToken(ModifyTokenInput{
		TokenID: created.View.Prefix,
		Name:    &renamed,
	})
	if err != nil {
		t.Fatalf("ModifyToken(prefix) error = %v", err)
	}
	if view.ID != created.View.ID || view.Name != renamed {
		t.Fatalf("view = %#v, want id=%s name=%s", view, created.View.ID, renamed)
	}
}

func TestModifyTokenAcceptsLegacyShortPrefix(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	created, err := svc.CreateToken(CreateTokenInput{
		Name:          "legacy",
		Type:          "pat",
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	legacyPrefix := created.RawToken[:16]
	if err := svc.store.DB().Exec("UPDATE api_tokens SET token_prefix = ? WHERE id = ?", legacyPrefix, created.View.ID).Error; err != nil {
		t.Fatal(err)
	}
	renamed := "legacy-renamed"
	view, err := svc.ModifyToken(ModifyTokenInput{
		TokenID: legacyPrefix,
		Name:    &renamed,
	})
	if err != nil {
		t.Fatalf("ModifyToken(legacy prefix) error = %v", err)
	}
	if view.ID != created.View.ID || view.Name != renamed {
		t.Fatalf("view = %#v, want id=%s name=%s", view, created.View.ID, renamed)
	}
}

func TestModifyTokenRejectsAmbiguousPrefix(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	if _, err := svc.CreateToken(CreateTokenInput{Name: "a", Type: "pat", Scopes: []string{"task:read"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateToken(CreateTokenInput{Name: "b", Type: "pat", Scopes: []string{"task:read"}}); err != nil {
		t.Fatal(err)
	}
	renamed := "ambiguous"
	_, err := svc.ModifyToken(ModifyTokenInput{
		TokenID: "xuanchu",
		Name:    &renamed,
	})
	assertRuntimeCode(t, err, "token_ambiguous_ref")
}

func TestCreateTenantAccessTokenStoresAPIKeyWithoutUser(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	created, err := svc.CreateTenantAccessToken(CreateTenantAccessTokenInput{
		Name:         "runtime-prod",
		Scopes:       []string{"task:read", "task:write"},
		WorkspaceRef: svc.Runtime().WorkspaceSlug,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.RawToken, "xuanchu_tenant_") {
		t.Fatalf("raw token = %q", created.RawToken)
	}
	if created.Stored.UserID != nil {
		t.Fatalf("tenant token user_id = %#v, want nil", created.Stored.UserID)
	}
	if created.View.WorkspaceID != svc.Runtime().WorkspaceID {
		t.Fatalf("workspace id = %q, want %q", created.View.WorkspaceID, svc.Runtime().WorkspaceID)
	}
	if created.View.Type != "tenant_access_token" {
		t.Fatalf("type = %q", created.View.Type)
	}
}

func TestTenantTokenModifyRejectsPATRef(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	pat, err := svc.CreateToken(CreateTokenInput{Name: "cli", Scopes: []string{"task:read"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.ModifyTenantAccessToken(ModifyTenantAccessTokenInput{TokenRef: pat.View.ID, Name: strptr("bad")})
	assertRuntimeCode(t, err, "tenant_token_not_found")
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
		Scopes:  &[]string{"task:read", "task:write"},
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
		Scopes:  &[]string{"task:*"},
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
		Scopes:  &[]string{"task:read", "impersonate"},
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
		Scopes:  &[]string{"invalid:scope"},
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

func TestModifyTokenWorkspaces(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	// 建第二个 workspace（owner 默认在 local，有权）
	other, err := svc.AddWorkspace(AddWorkspaceInput{Slug: "team", Name: "Team"})
	if err != nil {
		t.Fatal(err)
	}

	created, err := svc.CreateToken(CreateTokenInput{
		Name:          "agent",
		Type:          "agent",
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}

	view, err := svc.ModifyToken(ModifyTokenInput{
		TokenID:       created.View.ID,
		WorkspaceRefs: &[]string{"team"},
	})
	if err != nil {
		t.Fatalf("ModifyToken() error = %v", err)
	}
	if len(view.WorkspaceIDs) != 1 || view.WorkspaceIDs[0] != other.ID {
		t.Fatalf("workspace_ids = %v, want [%s]", view.WorkspaceIDs, other.ID)
	}
	// 审计记录应含 workspace_ids 变更
	audits := mustListAudit(t, svc)
	found := false
	for _, row := range audits {
		if row.Action != "token.modified" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(row.PayloadJSON), &payload); err != nil {
			continue
		}
		changes, _ := payload["changes"].(map[string]any)
		if _, ok := changes["workspace_ids"]; ok {
			found = true
		}
	}
	if !found {
		t.Fatalf("audit missing workspace_ids change: %#v", audits)
	}
}

func TestModifyTokenProjects(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	// 建一个属于 local（当前 runtime workspace）的 project
	project, err := svc.AddProject(AddProjectInput{Slug: "demo", Name: "Demo"})
	if err != nil {
		t.Fatal(err)
	}

	created, err := svc.CreateToken(CreateTokenInput{
		Name:          "agent",
		Type:          "agent",
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}

	view, err := svc.ModifyToken(ModifyTokenInput{
		TokenID:     created.View.ID,
		ProjectRefs: &[]string{"demo"},
	})
	if err != nil {
		t.Fatalf("ModifyToken() error = %v", err)
	}
	if len(view.ProjectIDs) != 1 || view.ProjectIDs[0] != project.ID {
		t.Fatalf("project_ids = %v, want [%s]", view.ProjectIDs, project.ID)
	}
}

func TestModifyTokenAgentRejectsEmptyWorkspace(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	created, err := svc.CreateToken(CreateTokenInput{
		Name:          "agent",
		Type:          "agent",
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.ModifyToken(ModifyTokenInput{
		TokenID:       created.View.ID,
		WorkspaceRefs: &[]string{},
	})
	assertRuntimeCode(t, err, "token_agent_requires_workspace")
}

func TestModifyTokenProjectNotInWorkspace(t *testing.T) {
	store := newTestStore(t)
	// owner 在 local
	ownerLocal := newTestServiceWithRuntime(t, store, 100, "local", "local")
	// 建第二个 workspace team，owner 加入
	team, err := ownerLocal.AddWorkspace(AddWorkspaceInput{Slug: "team", Name: "Team"})
	if err != nil {
		t.Fatal(err)
	}
	// 在 team 下建 project（切 runtime 到 team）
	ownerTeam := newTestServiceWithRuntime(t, store, 100, "local", "team")
	teamProject, err := ownerTeam.AddProject(AddProjectInput{Slug: "teamproj", Name: "TeamProj"})
	if err != nil {
		t.Fatal(err)
	}

	// token 绑定 local，尝试把 team 的 project 加进去应失败
	created, err := ownerLocal.CreateToken(CreateTokenInput{
		Name:          "agent",
		Type:          "agent",
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = ownerLocal.ModifyToken(ModifyTokenInput{
		TokenID:     created.View.ID,
		ProjectRefs: &[]string{teamProject.ID},
	})
	if err == nil {
		t.Fatalf("ModifyToken() error = nil, want project scope invalid (project not in local, team=%s)", team.ID)
	}
}

func TestModifyTokenRejectsEmptyScopes(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	created, err := svc.CreateToken(CreateTokenInput{
		Name:          "agent",
		Type:          "agent",
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.ModifyToken(ModifyTokenInput{
		TokenID: created.View.ID,
		Scopes:  &[]string{},
	})
	if err == nil {
		t.Fatal("ModifyToken() error = nil, want scope invalid (empty scopes rejected)")
	}
}

func TestModifyTokenRejectsNonOwner(t *testing.T) {
	store := newTestStore(t)
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	// owner 在 local 建一个 token
	ownerSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	created, err := ownerSvc.CreateToken(CreateTokenInput{
		Name:          "owner-agent",
		Type:          "agent",
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// 另一个 member 用户加入 local（member role，无 token:write）
	memberUser := mustCreateUserRecord(t, store, storage.User{
		ID: "user-other", Name: "other", CreatedAt: 100, ModifiedAt: 100,
	})
	mustUpsertMembershipRecord(t, store, storage.Membership{
		UserID: memberUser.ID, WorkspaceID: ws.ID,
		Role: string(RoleMember), JoinedAt: 100, ModifiedAt: 100,
	})
	memberSvc := newTestServiceWithRuntime(t, store, 100, "other", "local")

	// member 尝试修改 owner 的 token：非 owner 且 role 非 admin/owner → 拒绝
	newName := "hijacked"
	_, err = memberSvc.ModifyToken(ModifyTokenInput{
		TokenID: created.View.ID,
		Name:    &newName,
	})
	assertRuntimeCode(t, err, "permission_denied")

	// owner 自己改则成功
	view, err := ownerSvc.ModifyToken(ModifyTokenInput{
		TokenID: created.View.ID,
		Name:    &newName,
	})
	if err != nil {
		t.Fatalf("owner ModifyToken() error = %v", err)
	}
	if view.Name != "hijacked" {
		t.Fatalf("name = %q, want hijacked", view.Name)
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

// newScopedTokenService 构造一个带指定 requestScope 和 runtime 的 service，
// 用于模拟 HTTP/MCP 请求路径下（带 capability 集合）的调用者。
// actorTokenType 决定子集校验语义：
//   - BrowserSessionTokenType：capability 是交互层人为收紧，跳过子集校验
//   - auth.TokenTypePAT / auth.TokenTypeAgent / auth.TokenTypeTenantAccess：真实授权边界，子集校验生效
func newScopedTokenService(t *testing.T, store *storage.Store, capabilities []string, actorTokenType string) *Service {
	t.Helper()
	ws, err := storage.NewWorkspaceRepository(store.DB()).GetBySlug("local")
	if err != nil {
		t.Fatalf("GetBySlug(local) error = %v", err)
	}
	rt := RuntimeContext{
		ActorType:      string(authz.ActorUser),
		ActorTokenType: actorTokenType,
		WorkspaceID:    ws.ID,
		WorkspaceSlug:  ws.Slug,
		Role:           RoleOwner,
	}
	if actorTokenType == auth.TokenTypeTenantAccess {
		rt.ActorType = auth.TokenTypeTenantAccess
		rt.Role = RoleOwner
	}
	scope := &RequestScope{
		WorkspaceIDs: []string{ws.ID},
		Capabilities: append([]string(nil), capabilities...),
	}
	svc, err := NewService(ServiceOptions{
		Store:                 store,
		Clock:                 FixedClock{NowUnix: 100},
		Runtime:               &rt,
		RequestScope:          scope,
		DisableScopeBootstrap: true,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return svc
}

// browserSessionOwnerCapabilities 镜像 internal/httpapi.browserSessionScopes()，
// 刻意排除 workspace:write / user:write / impersonate，与 Web Console owner 的实际 capability 集一致。
func browserSessionOwnerCapabilities() []string {
	return []string{
		auth.ScopeTaskRead, auth.ScopeTaskWrite,
		auth.ScopeProjectRead, auth.ScopeProjectWrite,
		auth.ScopeContextRead, auth.ScopeContextWrite,
		auth.ScopeConfigRead, auth.ScopeConfigWrite,
		auth.ScopeWorkspaceRead,
		auth.ScopeAuditRead,
		auth.ScopeUserRead,
		auth.ScopeMemberRead, auth.ScopeMemberWrite,
		auth.ScopeTokenRead, auth.ScopeTokenWrite,
		auth.ScopeHookRead, auth.ScopeHookWrite,
		auth.ScopeNotificationRead, auth.ScopeNotificationWrite,
		auth.ScopeReminderRead, auth.ScopeReminderWrite,
	}
}

// 回归测试：browser session 登录的 owner 用户，其 capability 集刻意不含 workspace:write/user:write，
// 但在 Web Console 给 tenant token 勾选这两个 scope 时不应被 enforceTenantTokenWriteLimit 拦截。
// browser session 的 capability 是交互层人为收紧，真实授权由 membership role 决定。
func TestTenantTokenCreateAllowsBrowserSessionOwnerToGrantBroadScopes(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	ownerSvc := newScopedTokenService(t, svc.store, browserSessionOwnerCapabilities(), BrowserSessionTokenType)

	created, err := ownerSvc.CreateTenantAccessToken(CreateTenantAccessTokenInput{
		Name:         "broad",
		Scopes:       []string{"workspace:write", "user:write", "member:read"},
		WorkspaceRef: ownerSvc.Runtime().WorkspaceSlug,
	})
	if err != nil {
		t.Fatalf("CreateTenantAccessToken with broad scopes by browser session owner error = %v", err)
	}
	want := map[string]bool{"workspace:write": true, "user:write": true, "member:read": true}
	for _, s := range created.View.Scopes {
		delete(want, s)
	}
	if len(want) > 0 {
		t.Fatalf("missing scopes in created token: %v", want)
	}
}

func TestTenantTokenModifyAllowsBrowserSessionOwnerToAddBroadScopes(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	// 先用一个无 requestScope 的 owner service 建一个窄 scope 的 tenant token。
	created, err := svc.CreateTenantAccessToken(CreateTenantAccessTokenInput{
		Name:         "narrow",
		Scopes:       []string{"task:read"},
		WorkspaceRef: svc.Runtime().WorkspaceSlug,
	})
	if err != nil {
		t.Fatalf("CreateTenantAccessToken error = %v", err)
	}

	// 用 browser session owner（capability 不含 workspace:write/user:write）给它加宽 scope。
	ownerSvc := newScopedTokenService(t, svc.store, browserSessionOwnerCapabilities(), BrowserSessionTokenType)
	newScopes := []string{"task:read", "workspace:write", "user:write"}
	updated, err := ownerSvc.ModifyTenantAccessToken(ModifyTenantAccessTokenInput{
		TokenRef: created.View.ID,
		Scopes:   &newScopes,
	})
	if err != nil {
		t.Fatalf("ModifyTenantAccessToken add broad scopes by browser session owner error = %v", err)
	}
	got := map[string]bool{}
	for _, s := range updated.Scopes {
		got[s] = true
	}
	for _, want := range newScopes {
		if !got[want] {
			t.Fatalf("scope %q missing after modify, got %v", want, updated.Scopes)
		}
	}
}

// 对照测试：受限 PAT（真实授权边界）创建超出自身 capability 的 tenant token 仍应被拒。
// PAT 的 scope 是用户自愿授予的真实权限边界，子集校验必须生效（防提权）。
func TestTenantTokenCreateRejectsLimitedPATExceedingOwnScope(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	// PAT 只有 token:read/token:write，没有 task:read。
	patSvc := newScopedTokenService(t, svc.store, []string{auth.ScopeTokenRead, auth.ScopeTokenWrite}, auth.TokenTypePAT)

	_, err := patSvc.CreateTenantAccessToken(CreateTenantAccessTokenInput{
		Name:         "escalation",
		Scopes:       []string{"task:read"},
		WorkspaceRef: patSvc.Runtime().WorkspaceSlug,
	})
	assertRuntimeCode(t, err, authz.CodeTokenScopeDenied)
}

// 对照测试：tenant actor 创建超出自身 capability 的 tenant token 仍应被拒。
func TestTenantTokenCreateRejectsTenantActorExceedingOwnScope(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	tenantSvc := newScopedTokenService(t, svc.store, []string{auth.ScopeTaskRead, auth.ScopeTokenWrite}, auth.TokenTypeTenantAccess)

	_, err := tenantSvc.CreateTenantAccessToken(CreateTenantAccessTokenInput{
		Name:         "escalation",
		Scopes:       []string{"workspace:write"},
		WorkspaceRef: tenantSvc.Runtime().WorkspaceSlug,
	})
	assertRuntimeCode(t, err, authz.CodeTokenScopeDenied)
}
