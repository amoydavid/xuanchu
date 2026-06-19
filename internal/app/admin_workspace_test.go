package app

import (
	"strings"
	"testing"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

func newAdminWorkspaceTestService(t *testing.T, store *storage.Store) *Service {
	t.Helper()
	svc, err := NewService(ServiceOptions{
		Store:                 store,
		Clock:                 FixedClock{NowUnix: 1000},
		Runtime:               &RuntimeContext{ActorName: "server-admin"},
		DisableScopeBootstrap: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

// seedAdminWorkspaces 构造两个 workspace：
// - dajee：有 owner alice + admin bob + 普通 member carol
// - legacy：已归档，有 owner alice
// 并返回 dajee workspace 视图。
func seedAdminWorkspaces(t *testing.T, svc *Service) AdminCreateWorkspaceResult {
	t.Helper()
	dajee, err := svc.AdminCreateWorkspace(AdminCreateWorkspaceInput{
		AdminTokenName: "ops",
		Slug:           "dajee",
		Name:           "Dajee",
		Visibility:     "team",
		Owner:          AdminOwnerInput{Name: "alice", Email: "alice@example.com"},
	})
	if err != nil {
		t.Fatalf("create dajee: %v", err)
	}
	if _, err := svc.AdminCreateWorkspaceAdmin(AdminCreateWorkspaceAdminInput{
		AdminTokenName: "ops",
		WorkspaceRef:   "dajee",
		Name:           "bob",
		Email:          "bob@example.com",
		Role:           RoleAdmin,
	}); err != nil {
		t.Fatalf("promote bob: %v", err)
	}
	legacy, err := svc.AdminCreateWorkspace(AdminCreateWorkspaceInput{
		AdminTokenName: "ops",
		Slug:           "legacy",
		Name:           "Legacy",
		Owner:          AdminOwnerInput{Name: "alice"},
	})
	if err != nil {
		t.Fatalf("create legacy: %v", err)
	}
	if err := storage.NewWorkspaceRepository(svc.store.DB()).Archive(legacy.Workspace.ID, svc.clock.Unix()); err != nil {
		t.Fatalf("archive legacy: %v", err)
	}
	return dajee
}

func TestAdminListWorkspacesExcludesArchivedByDefault(t *testing.T) {
	store := newTestStore(t)
	svc := newAdminWorkspaceTestService(t, store)
	seedAdminWorkspaces(t, svc)

	rows, err := svc.AdminListWorkspaces(false)
	if err != nil {
		t.Fatal(err)
	}
	// store.Open 总是初始化 local workspace，因此列表里至少有 local + dajee。
	// 这里只断言 dajee 行存在且摘要正确，且 legacy（archived）被排除。
	var dajee *AdminWorkspaceSummaryView
	hasLegacy := false
	for i := range rows {
		if rows[i].Slug == "dajee" {
			dajee = &rows[i]
		}
		if rows[i].Slug == "legacy" {
			hasLegacy = true
		}
	}
	if dajee == nil {
		t.Fatalf("dajee missing from rows = %#v", rows)
	}
	if hasLegacy {
		t.Fatalf("archived workspace should be excluded: rows = %#v", rows)
	}
	if dajee.Name != "Dajee" || dajee.Visibility != "team" {
		t.Fatalf("dajee row = %#v", dajee)
	}
	if dajee.MemberCounts.Owner != 1 || dajee.MemberCounts.Admin != 1 || dajee.MemberCounts.Member != 0 {
		t.Fatalf("member counts = %#v", dajee.MemberCounts)
	}
	// created_by 必须是完整 UserInfo，不能是裸 UUID。
	if dajee.CreatedBy == nil || dajee.CreatedBy.ID == "" || dajee.CreatedBy.Name == dajee.CreatedBy.ID {
		t.Fatalf("created_by = %#v", dajee.CreatedBy)
	}
}

func TestAdminListWorkspacesIncludesArchivedWhenRequested(t *testing.T) {
	store := newTestStore(t)
	svc := newAdminWorkspaceTestService(t, store)
	seedAdminWorkspaces(t, svc)

	rows, err := svc.AdminListWorkspaces(true)
	if err != nil {
		t.Fatal(err)
	}
	slugs := map[string]bool{}
	for _, row := range rows {
		slugs[row.Slug] = true
	}
	if !slugs["dajee"] || !slugs["legacy"] {
		t.Fatalf("rows = %#v", rows)
	}
}

func TestAdminWorkspaceInfoReturnsMembersAndActingCandidates(t *testing.T) {
	store := newTestStore(t)
	svc := newAdminWorkspaceTestService(t, store)
	seedAdminWorkspaces(t, svc)

	detail, err := svc.AdminWorkspaceInfo("dajee")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Workspace.Slug != "dajee" {
		t.Fatalf("workspace = %#v", detail.Workspace)
	}
	// 成员摘要：alice(owner) + bob(admin)
	if len(detail.Members) != 2 {
		t.Fatalf("members = %#v", detail.Members)
	}
	// acting_candidates 只包含 owner/admin
	candidates := map[string]string{}
	for _, c := range detail.ActingCandidates {
		candidates[c.User.ID] = string(c.Role)
	}
	if len(candidates) != 2 {
		t.Fatalf("acting candidates = %#v", detail.ActingCandidates)
	}
	// member 引用的 user 字段必须是完整 UserInfo
	for _, m := range detail.Members {
		if m.User.ID == "" || m.User.Name == m.User.ID {
			t.Fatalf("member user = %#v", m.User)
		}
	}
}

func TestAdminWorkspaceInfoTokenCountsClassifyActiveRevokedExpired(t *testing.T) {
	store := newTestStore(t)
	svc := newAdminWorkspaceTestService(t, store)
	dajee := seedAdminWorkspaces(t, svc)
	tokenRepo := storage.NewTokenRepository(store.DB())
	// 写一个绑定到 dajee 的过期 token（expires_at 早于 now=1000）。
	expiredJSON := `["` + dajee.Workspace.ID + `"]`
	if err := tokenRepo.Create(storage.ApiTokenEntry{
		ID:               "expired-token",
		UserID:           dajee.Owner.ID,
		Name:             "expired",
		Type:             auth.TokenTypeAgent,
		TokenPrefix:      "xuanchu_agent_exp",
		TokenHash:        "sha256:exp",
		ScopesJSON:       `["task:read"]`,
		WorkspaceIDsJSON: expiredJSON,
		ProjectIDsJSON:   `[]`,
		CreatedAt:        1,
		ExpiresAt:        ptrInt64(10),
	}); err != nil {
		t.Fatalf("create expired token: %v", err)
	}
	// active token
	if _, err := svc.AdminCreateWorkspaceAgentToken(AdminCreateAgentTokenInput{
		AdminTokenName: "ops",
		WorkspaceRef:   "dajee",
		Name:           "active",
		UserRef:        "alice@example.com",
		Scopes:         []string{"task:read"},
	}); err != nil {
		t.Fatalf("create active token: %v", err)
	}
	// 创建一个 token 然后吊销它
	created, err := svc.AdminCreateWorkspaceAgentToken(AdminCreateAgentTokenInput{
		AdminTokenName: "ops",
		WorkspaceRef:   "dajee",
		Name:           "will-revoke",
		UserRef:        "alice@example.com",
		Scopes:         []string{"task:read"},
	})
	if err != nil {
		t.Fatalf("create revoke target: %v", err)
	}
	if err := svc.AdminRevokeToken(created.View.ID, "ops"); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	detail, err := svc.AdminWorkspaceInfo("dajee")
	if err != nil {
		t.Fatal(err)
	}
	if detail.TokenCounts.Active != 1 || detail.TokenCounts.Revoked != 1 || detail.TokenCounts.Expired != 1 {
		t.Fatalf("token counts = %#v (workspace id=%s)", detail.TokenCounts, dajee.Workspace.ID)
	}
}

func TestAdminWorkspaceInfoReturnsUserInfoForActingCandidates(t *testing.T) {
	store := newTestStore(t)
	svc := newAdminWorkspaceTestService(t, store)
	seedAdminWorkspaces(t, svc)

	detail, err := svc.AdminWorkspaceInfo("dajee")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range detail.ActingCandidates {
		if c.User.ID == "" || c.User.Name == c.User.ID {
			t.Fatalf("acting candidate user = %#v", c.User)
		}
		if c.User.Email == nil {
			t.Fatalf("acting candidate %s missing email", c.User.Name)
		}
	}
}

// 确保我们引入的 auth 包测试期仍被使用（避免后续 chunk 删除后出现 unused import）。
var _ = auth.ActingTokenPrefix

func ptrInt64(v int64) *int64 { return &v }

// === Task 5: acting session service ===

func TestAdminCreateActingSessionDefaultsToFirstOwner(t *testing.T) {
	store := newTestStore(t)
	svc := newAdminWorkspaceTestService(t, store)
	seedAdminWorkspaces(t, svc)

	created, err := svc.AdminCreateActingSession(AdminCreateActingSessionInput{
		AdminTokenName: "ops",
		WorkspaceRef:   "dajee",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Token == "" || !strings.HasPrefix(created.Token, "xuanchu_act_") {
		t.Fatalf("token = %q", created.Token)
	}
	if created.Workspace.Slug != "dajee" {
		t.Fatalf("workspace = %#v", created.Workspace)
	}
	// 默认选择第一个 owner（alice）。
	if created.Actor.Name != "alice" {
		t.Fatalf("actor = %#v", created.Actor)
	}
	if created.Role != RoleOwner {
		t.Fatalf("role = %q", created.Role)
	}
	if created.AdminTokenName != "ops" {
		t.Fatalf("admin token name = %q", created.AdminTokenName)
	}
	if created.ExpiresAt <= svc.clock.Unix() {
		t.Fatalf("expires_at = %d", created.ExpiresAt)
	}

	// raw token 不得落库；hash 必须能验证 raw token。
	repo := storage.NewAdminActingSessionRepository(store.DB())
	session, err := repo.GetByPrefix(created.TokenPrefix)
	if err != nil {
		t.Fatalf("session missing: %v", err)
	}
	if session.TokenHash == created.Token {
		t.Fatalf("raw token persisted in hash")
	}
	if !auth.VerifyActingToken(created.Token, session.TokenHash) {
		t.Fatalf("hash does not verify raw token")
	}
}

func TestAdminCreateActingSessionFallsBackToFirstAdmin(t *testing.T) {
	store := newTestStore(t)
	svc := newAdminWorkspaceTestService(t, store)
	// 只有 admin，没有 owner 的 workspace。
	ws, err := svc.AdminCreateWorkspace(AdminCreateWorkspaceInput{
		AdminTokenName: "ops",
		Slug:           "no-owner",
		Owner:          AdminOwnerInput{Name: "creator"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// 把 creator 从 owner 降级为 member，再添加一个 admin。
	memberRepo := storage.NewMemberRepository(store.DB())
	if err := memberRepo.UpdateRole(ws.Owner.ID, ws.Workspace.ID, "member", svc.clock.Unix()); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AdminCreateWorkspaceAdmin(AdminCreateWorkspaceAdminInput{
		AdminTokenName: "ops",
		WorkspaceRef:   "no-owner",
		Name:           "theadmin",
		Role:           RoleAdmin,
	}); err != nil {
		t.Fatal(err)
	}

	created, err := svc.AdminCreateActingSession(AdminCreateActingSessionInput{
		AdminTokenName: "ops",
		WorkspaceRef:   "no-owner",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Actor.Name != "theadmin" {
		t.Fatalf("actor = %#v, want theadmin", created.Actor)
	}
	if created.Role != RoleAdmin {
		t.Fatalf("role = %q", created.Role)
	}
}

func TestAdminCreateActingSessionRejectsArchivedWorkspace(t *testing.T) {
	store := newTestStore(t)
	svc := newAdminWorkspaceTestService(t, store)
	seedAdminWorkspaces(t, svc)

	_, err := svc.AdminCreateActingSession(AdminCreateActingSessionInput{
		AdminTokenName: "ops",
		WorkspaceRef:   "legacy",
	})
	if err == nil {
		t.Fatal("expected error for archived workspace")
	}
	assertRuntimeCode(t, err, "workspace_archived")
}

func TestAdminCreateActingSessionRejectsNonOwnerAdminTarget(t *testing.T) {
	store := newTestStore(t)
	svc := newAdminWorkspaceTestService(t, store)
	seedAdminWorkspaces(t, svc)

	_, err := svc.AdminCreateActingSession(AdminCreateActingSessionInput{
		AdminTokenName: "ops",
		WorkspaceRef:   "dajee",
		UserRef:        "carol", // 不存在；解析失败也应被拒绝
	})
	if err == nil {
		t.Fatal("expected error for invalid target")
	}
	// carol 不存在，resolveUser 返回 user_not_found，acting 路径转 admin_acting_target_invalid。
	assertRuntimeCode(t, err, "admin_acting_target_invalid")
}

func TestAdminCreateActingSessionRejectsNegativeTTL(t *testing.T) {
	store := newTestStore(t)
	svc := newAdminWorkspaceTestService(t, store)
	seedAdminWorkspaces(t, svc)

	negative := time.Duration(-1)
	_, err := svc.AdminCreateActingSession(AdminCreateActingSessionInput{
		AdminTokenName: "ops",
		WorkspaceRef:   "dajee",
		ExpiresIn:      &negative,
	})
	assertRuntimeCode(t, err, "admin_token_ttl_invalid")
}

func TestAuthenticateBearerTokenAcceptsActingToken(t *testing.T) {
	store := newTestStore(t)
	svc := newAdminWorkspaceTestService(t, store)
	seedAdminWorkspaces(t, svc)

	created, err := svc.AdminCreateActingSession(AdminCreateActingSessionInput{
		AdminTokenName: "ops",
		WorkspaceRef:   "dajee",
	})
	if err != nil {
		t.Fatal(err)
	}

	// 用一个空 runtime 的 service 鉴权，模拟普通 HTTP middleware 入口。
	authSvc, err := NewService(ServiceOptions{
		Store:                 store,
		Clock:                 FixedClock{NowUnix: 1001},
		Runtime:               &RuntimeContext{},
		DisableScopeBootstrap: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	authn, err := authSvc.AuthenticateBearerToken(created.Token)
	if err != nil {
		t.Fatalf("AuthenticateBearerToken() error = %v", err)
	}
	if authn.Token.Type != auth.TokenTypeAdminActing {
		t.Fatalf("token type = %q", authn.Token.Type)
	}
	if authn.User.ID != created.Actor.ID {
		t.Fatalf("actor = %#v, want %s", authn.User, created.Actor.ID)
	}
	// acting token 的 workspace scope 必须只绑定到一个 workspace。
	if len(authn.Token.WorkspaceIDs) != 1 || authn.Token.WorkspaceIDs[0] != created.Workspace.ID {
		t.Fatalf("workspace ids = %#v", authn.Token.WorkspaceIDs)
	}
	// project allowlist 为空，表示该 workspace 内项目按角色权限可见。
	if len(authn.Token.ProjectIDs) != 0 {
		t.Fatalf("project ids = %#v", authn.Token.ProjectIDs)
	}
	// acting token 携带 server admin acting trace。
	if authn.AdminActingTrace == nil || authn.AdminActingTrace.SessionID == "" {
		t.Fatalf("admin acting trace missing: %#v", authn.AdminActingTrace)
	}
	if authn.AdminActingTrace.DelegatorAdminTokenName != "ops" {
		t.Fatalf("delegator admin token name = %#v", authn.AdminActingTrace)
	}
}

func TestAuthenticateBearerTokenRejectsExpiredActingToken(t *testing.T) {
	store := newTestStore(t)
	svc := newAdminWorkspaceTestService(t, store)
	seedAdminWorkspaces(t, svc)

	shortTTL := time.Duration(1) * time.Second
	created, err := svc.AdminCreateActingSession(AdminCreateActingSessionInput{
		AdminTokenName: "ops",
		WorkspaceRef:   "dajee",
		ExpiresIn:      &shortTTL,
	})
	if err != nil {
		t.Fatal(err)
	}
	// 推进时钟超过过期时间。
	authSvc, err := NewService(ServiceOptions{
		Store:                 store,
		Clock:                 FixedClock{NowUnix: svc.clock.Unix() + 100},
		Runtime:               &RuntimeContext{},
		DisableScopeBootstrap: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = authSvc.AuthenticateBearerToken(created.Token)
	assertRuntimeCode(t, err, "admin_acting_session_expired")
}

func TestAuthenticateBearerTokenRejectsRevokedActingToken(t *testing.T) {
	store := newTestStore(t)
	svc := newAdminWorkspaceTestService(t, store)
	seedAdminWorkspaces(t, svc)

	created, err := svc.AdminCreateActingSession(AdminCreateActingSessionInput{
		AdminTokenName: "ops",
		WorkspaceRef:   "dajee",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.AdminRevokeActingSession(created.SessionID, "ops"); err != nil {
		t.Fatal(err)
	}
	_, err = svc.AuthenticateBearerToken(created.Token)
	assertRuntimeCode(t, err, "auth_invalid_token")
}

func TestAuthorizeTokenRequestActingCannotCrossWorkspace(t *testing.T) {
	store := newTestStore(t)
	svc := newAdminWorkspaceTestService(t, store)
	seedAdminWorkspaces(t, svc)

	created, err := svc.AdminCreateActingSession(AdminCreateActingSessionInput{
		AdminTokenName: "ops",
		WorkspaceRef:   "dajee",
	})
	if err != nil {
		t.Fatal(err)
	}
	authSvc, err := NewService(ServiceOptions{
		Store:                 store,
		Clock:                 FixedClock{NowUnix: 1001},
		Runtime:               &RuntimeContext{},
		DisableScopeBootstrap: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	authn, err := authSvc.AuthenticateBearerToken(created.Token)
	if err != nil {
		t.Fatal(err)
	}
	// 指向另一个 workspace（local）应被拒绝。
	_, err = authSvc.AuthorizeTokenRequest(RequestAuthorizationInput{
		Token:              authn,
		RequiredCapability: "task:read",
		RequiredPermission: PermissionTaskRead,
		WorkspaceRef:       "local",
	})
	assertRuntimeCode(t, err, "workspace_scope_denied")
}

func TestAuthorizeTokenRequestActingUsesCurrentRole(t *testing.T) {
	store := newTestStore(t)
	svc := newAdminWorkspaceTestService(t, store)
	seedAdminWorkspaces(t, svc)

	created, err := svc.AdminCreateActingSession(AdminCreateActingSessionInput{
		AdminTokenName: "ops",
		WorkspaceRef:   "dajee",
	})
	if err != nil {
		t.Fatal(err)
	}
	// 把 alice 从 owner 降级为 viewer。acting token 仍应能认证，
	// 但写操作必须因权限不足失败（用当前 role 而不是创建时的 owner 快照）。
	memberRepo := storage.NewMemberRepository(store.DB())
	if err := memberRepo.UpdateRole(created.Actor.ID, created.Workspace.ID, "viewer", svc.clock.Unix()); err != nil {
		t.Fatal(err)
	}

	authSvc, err := NewService(ServiceOptions{
		Store:                 store,
		Clock:                 FixedClock{NowUnix: 1001},
		Runtime:               &RuntimeContext{},
		DisableScopeBootstrap: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	authn, err := authSvc.AuthenticateBearerToken(created.Token)
	if err != nil {
		t.Fatalf("authenticate still works after demotion: %v", err)
	}
	// 读权限（viewer 允许）。
	authzRead, err := authSvc.AuthorizeTokenRequest(RequestAuthorizationInput{
		Token:              authn,
		RequiredCapability: "task:read",
		RequiredPermission: PermissionTaskRead,
		WorkspaceRef:       "dajee",
	})
	if err != nil {
		t.Fatalf("read should be allowed for viewer: %v", err)
	}
	// 写权限（viewer 不允许）。
	_, err = authSvc.AuthorizeTokenRequest(RequestAuthorizationInput{
		Token:              authn,
		RequiredCapability: "task:write",
		RequiredPermission: PermissionTaskWrite,
		WorkspaceRef:       "dajee",
	})
	assertRuntimeCode(t, err, "permission_denied")
	// 授权 runtime 必须携带 acting trace。
	if authzRead.Runtime.AdminActingSessionID != created.SessionID {
		t.Fatalf("runtime acting session id = %q, want %q", authzRead.Runtime.AdminActingSessionID, created.SessionID)
	}
}

func TestAuthorizeTokenRequestActingRejectsImpersonation(t *testing.T) {
	store := newTestStore(t)
	svc := newAdminWorkspaceTestService(t, store)
	seedAdminWorkspaces(t, svc)

	created, err := svc.AdminCreateActingSession(AdminCreateActingSessionInput{
		AdminTokenName: "ops",
		WorkspaceRef:   "dajee",
	})
	if err != nil {
		t.Fatal(err)
	}
	authSvc, err := NewService(ServiceOptions{
		Store:                 store,
		Clock:                 FixedClock{NowUnix: 1001},
		Runtime:               &RuntimeContext{},
		DisableScopeBootstrap: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	authn, err := authSvc.AuthenticateBearerToken(created.Token)
	if err != nil {
		t.Fatal(err)
	}
	// X-Xuanchu-As 不能与 acting token 组合做 impersonation。
	_, err = authSvc.AuthorizeTokenRequest(RequestAuthorizationInput{
		Token:              authn,
		RequiredCapability: "task:read",
		RequiredPermission: PermissionTaskRead,
		WorkspaceRef:       "dajee",
		SubjectUserRef:     "bob@example.com",
	})
	assertRuntimeCode(t, err, "token_scope_denied")
}
