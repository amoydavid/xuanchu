package app

import (
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

// newTokenSecretService 建一个带 secret key 的 service，用于 token reveal 测试。
func newTokenSecretService(t *testing.T, now int64) *Service {
	t.Helper()
	store := newTestStore(t)
	svc, err := NewService(ServiceOptions{
		Store:              store,
		Clock:              FixedClock{NowUnix: now},
		TokenSecretKey:     testSecretKey(t),
		RequireTokenSecret: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

// withBrowserSessionRuntime 把 service 标记为来自 SSO browser session，
// 模拟 HTTP 层 scopedServiceFor 对 authorized.Runtime.WebLoginDisabled 的注入。
func withBrowserSessionRuntime(svc *Service) {
	svc.runtime.WebLoginDisabled = true
}

func TestCreateTokenMarksWebLoginDisabledFromBrowserSession(t *testing.T) {
	svc := newTokenSecretService(t, 100)
	withBrowserSessionRuntime(svc)
	created, err := svc.CreateToken(CreateTokenInput{
		Name:          "sso-pat",
		Type:          auth.TokenTypePAT,
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !created.Stored.WebLoginDisabled {
		t.Fatal("PAT created from browser session should have WebLoginDisabled=true")
	}
}

func TestCreateTokenDoesNotMarkWebLoginDisabledFromBearer(t *testing.T) {
	svc := newTokenSecretService(t, 100)
	// 默认 runtime（Bearer token / CLI），WebLoginDisabled=false
	created, err := svc.CreateToken(CreateTokenInput{
		Name:          "cli-pat",
		Type:          auth.TokenTypePAT,
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Stored.WebLoginDisabled {
		t.Fatal("PAT created from Bearer should have WebLoginDisabled=false")
	}
}

func TestCreateTenantAccessTokenNeverMarksWebLoginDisabled(t *testing.T) {
	svc := newTokenSecretService(t, 100)
	withBrowserSessionRuntime(svc)
	created, err := svc.CreateTenantAccessToken(CreateTenantAccessTokenInput{
		Name:   "runtime",
		Scopes: []string{"task:read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Stored.WebLoginDisabled {
		t.Fatal("tenant token must never be marked WebLoginDisabled (machine identity)")
	}
}

func TestCreateAgentTokenMarksWebLoginDisabledFromBrowserSession(t *testing.T) {
	svc := newTokenSecretService(t, 100)
	withBrowserSessionRuntime(svc)
	created, err := svc.CreateToken(CreateTokenInput{
		Name:          "sso-agent",
		Type:          auth.TokenTypeAgent,
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !created.Stored.WebLoginDisabled {
		t.Fatal("Agent token created from browser session should have WebLoginDisabled=true")
	}
}

func TestCreateTokenRequiresSecretKeyWhenStrict(t *testing.T) {
	store := newTestStore(t)
	svc, err := NewService(ServiceOptions{
		Store:              store,
		Clock:              FixedClock{NowUnix: 100},
		RequireTokenSecret: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.CreateToken(CreateTokenInput{
		Name:          "agent",
		Type:          auth.TokenTypeAgent,
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	assertRuntimeCode(t, err, "config_secret_key_missing")
}

func TestCreateTokenStoresRecoverableCiphertext(t *testing.T) {
	key := testSecretKey(t)
	store := newTestStore(t)
	svc, err := NewService(ServiceOptions{
		Store:              store,
		Clock:              FixedClock{NowUnix: 100},
		TokenSecretKey:     key,
		RequireTokenSecret: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateToken(CreateTokenInput{
		Name:          "agent",
		Type:          auth.TokenTypeAgent,
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Stored.TokenSecretCiphertext == "" {
		t.Fatal("TokenSecretCiphertext is empty")
	}
	if strings.Contains(created.Stored.TokenSecretCiphertext, created.RawToken) {
		t.Fatal("ciphertext leaks raw token")
	}
	plain, err := DecryptConfigSecret(key, created.Stored.TokenSecretCiphertext)
	if err != nil {
		t.Fatal(err)
	}
	if plain != created.RawToken {
		t.Fatalf("decrypted token = %q, want raw token", plain)
	}
}

func TestCreateTenantAccessTokenStoresRecoverableCiphertext(t *testing.T) {
	key := testSecretKey(t)
	store := newTestStore(t)
	svc, err := NewService(ServiceOptions{
		Store:              store,
		Clock:              FixedClock{NowUnix: 100},
		TokenSecretKey:     key,
		RequireTokenSecret: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateTenantAccessToken(CreateTenantAccessTokenInput{
		Name:   "runtime",
		Scopes: []string{"task:read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Stored.TokenSecretCiphertext == "" {
		t.Fatal("tenant ciphertext is empty")
	}
	plain, err := DecryptConfigSecret(key, created.Stored.TokenSecretCiphertext)
	if err != nil {
		t.Fatal(err)
	}
	if plain != created.RawToken {
		t.Fatalf("decrypted token = %q, want raw tenant token", plain)
	}
}

func TestCreateTenantAccessTokenRequiresSecretKeyWhenStrict(t *testing.T) {
	store := newTestStore(t)
	svc, err := NewService(ServiceOptions{
		Store:              store,
		Clock:              FixedClock{NowUnix: 100},
		RequireTokenSecret: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.CreateTenantAccessToken(CreateTenantAccessTokenInput{
		Name:   "runtime",
		Scopes: []string{"task:read"},
	})
	assertRuntimeCode(t, err, "config_secret_key_missing")
}

func TestRevealTokenMCPConfigReturnsRawToken(t *testing.T) {
	svc := newTokenSecretService(t, 100)
	created, err := svc.CreateToken(CreateTokenInput{
		Name:          "agent",
		Type:          auth.TokenTypeAgent,
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	view, err := svc.RevealTokenMCPConfig(created.View.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.RawToken != created.RawToken || view.EndpointPath != "/mcp" || view.TokenType != auth.TokenTypeAgent {
		t.Fatalf("view = %#v", view)
	}
}

func TestRevealTokenMCPConfigRejectsTenantOnNormalEndpoint(t *testing.T) {
	svc := newTokenSecretService(t, 100)
	created, err := svc.CreateTenantAccessToken(CreateTenantAccessTokenInput{
		Name:   "runtime",
		Scopes: []string{"task:read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.RevealTokenMCPConfig(created.View.ID)
	assertRuntimeCode(t, err, "token_not_found")
}

func TestRevealTenantTokenMCPConfigReturnsRawToken(t *testing.T) {
	svc := newTokenSecretService(t, 100)
	created, err := svc.CreateTenantAccessToken(CreateTenantAccessTokenInput{
		Name:   "runtime",
		Scopes: []string{"task:read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	view, err := svc.RevealTenantTokenMCPConfig(created.View.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.RawToken != created.RawToken || view.EndpointPath != "/mcp" || view.TokenType != auth.TokenTypeTenantAccess {
		t.Fatalf("view = %#v", view)
	}
}

func TestRevealTenantTokenMCPConfigRejectsNormalToken(t *testing.T) {
	svc := newTokenSecretService(t, 100)
	created, err := svc.CreateToken(CreateTokenInput{
		Name:          "agent",
		Type:          auth.TokenTypeAgent,
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.RevealTenantTokenMCPConfig(created.View.ID)
	assertRuntimeCode(t, err, "tenant_token_not_found")
}

func TestRevealTokenMCPConfigMissingCiphertext(t *testing.T) {
	store := newTestStore(t)
	// 用一个不存 ciphertext 的 token：先以非 strict 方式创建（无 key），再切换到有 key 的 service reveal。
	svc, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: 100}})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateToken(CreateTokenInput{
		Name:          "agent",
		Type:          auth.TokenTypeAgent,
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Stored.TokenSecretCiphertext != "" {
		t.Fatalf("expected empty ciphertext without key, got %q", created.Stored.TokenSecretCiphertext)
	}
	revealSvc, err := NewService(ServiceOptions{
		Store:          store,
		Clock:          FixedClock{NowUnix: 100},
		TokenSecretKey: testSecretKey(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = revealSvc.RevealTokenMCPConfig(created.View.ID)
	assertRuntimeCode(t, err, "token_secret_unavailable")
}

func TestRevealTokenMCPConfigAuditExcludesRawToken(t *testing.T) {
	svc := newTokenSecretService(t, 100)
	created, err := svc.CreateToken(CreateTokenInput{
		Name:          "agent",
		Type:          auth.TokenTypeAgent,
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RevealTokenMCPConfig(created.View.ID); err != nil {
		t.Fatal(err)
	}
	rows := mustListAudit(t, svc)
	var found bool
	for _, row := range rows {
		if row.Action != "token.mcp_config_reveal" {
			continue
		}
		found = true
		if strings.Contains(row.PayloadJSON, created.RawToken) {
			t.Fatalf("audit payload leaks raw token: %s", row.PayloadJSON)
		}
	}
	if !found {
		t.Fatalf("audit missing token.mcp_config_reveal: %#v", rows)
	}
}

func TestRevealTokenMCPConfigRequiresRequestScope(t *testing.T) {
	store := newTestStore(t)
	ownerSvc, err := NewService(ServiceOptions{
		Store:              store,
		Clock:              FixedClock{NowUnix: 100},
		TokenSecretKey:     testSecretKey(t),
		RequireTokenSecret: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	target, err := ownerSvc.CreateToken(CreateTokenInput{
		Name:          "target",
		Type:          auth.TokenTypeAgent,
		Scopes:        []string{"task:read", "token:write", "impersonate"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// 创建一个 request scope 受限的 service：只允许 task:read，不能 reveal 含 impersonate 的 token。
	limitedSvc, err := NewService(ServiceOptions{
		Store:          store,
		Clock:          FixedClock{NowUnix: 100},
		TokenSecretKey: ownerSvc.tokenSecretKey,
		RequestScope: &RequestScope{
			WorkspaceIDs: []string{"local"},
			Capabilities: []string{"task:read", "token:read"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = limitedSvc.RevealTokenMCPConfig(target.View.ID)
	assertRuntimeCode(t, err, "token_scope_denied")
}

// 确保过期/吊销 token 仍可 reveal（只标注状态）。
func TestRevealTokenMCPConfigAllowsRevokedToken(t *testing.T) {
	svc := newTokenSecretService(t, 100)
	created, err := svc.CreateToken(CreateTokenInput{
		Name:          "agent",
		Type:          auth.TokenTypeAgent,
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.RevokeToken(created.View.ID); err != nil {
		t.Fatal(err)
	}
	view, err := svc.RevealTokenMCPConfig(created.View.ID)
	if err != nil {
		t.Fatalf("reveal revoked token error = %v", err)
	}
	if view.RevokedAt == nil {
		t.Fatal("expected RevokedAt set on revealed view")
	}
}

// TestRevealTokenMCPConfigRejectsOtherUsersToken 锁定 owner 边界：
// HTTP 层已用 PermissionTokenRead 拒绝 member/viewer，但 app 层仍复刻
// RevokeToken/ModifyToken 的 owner-or-admin 语义，防止越界 reveal 其他用户的 token。
func TestRevealTokenMCPConfigRejectsOtherUsersToken(t *testing.T) {
	store := newTestStore(t)
	key := testSecretKey(t)
	ownerSvc, err := NewService(ServiceOptions{
		Store:              store,
		Clock:              FixedClock{NowUnix: 100},
		TokenSecretKey:     key,
		RequireTokenSecret: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	// owner 创建第二个用户 "other" 并为其签发 PAT。
	other, err := ownerSvc.AddUser(AddUserInput{Name: "other"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := ownerSvc.CreateToken(CreateTokenInput{
		Name:          "target",
		Type:          auth.TokenTypePAT,
		UserRef:       other.Name,
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// 把 local 降级为 member，直接调用 app 层 RevealTokenMCPConfig
	// （绕过 HTTP 层的 PermissionTokenRead 校验），验证 owner-ID 边界。
	memberSvc := demoteLocalToMember(t, store, key)
	_, err = memberSvc.RevealTokenMCPConfig(target.View.ID)
	if err == nil {
		t.Fatal("expected error revealing other user's token as member")
	}
}

// demoteLocalToMember 把 local 用户在 local workspace 的角色降为 member，
// 并返回以 local（member）身份构建的 service（共享同一 secret key）。
// 用于直接调用 app 层方法，绕过 HTTP 层角色校验，验证 owner-ID 边界。
func demoteLocalToMember(t *testing.T, store *storage.Store, key []byte) *Service {
	t.Helper()
	localUser, err := storage.NewUserRepository(store.DB()).GetByName("local")
	if err != nil {
		t.Fatalf("get local user: %v", err)
	}
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatalf("get local workspace: %v", err)
	}
	if err := storage.NewMemberRepository(store.DB()).UpdateRole(localUser.ID, ws.ID, "member", 100); err != nil {
		t.Fatalf("demote local to member: %v", err)
	}
	svc, err := NewService(ServiceOptions{
		Store:          store,
		Clock:          FixedClock{NowUnix: 100},
		TokenSecretKey: key,
	})
	if err != nil {
		t.Fatalf("new member service: %v", err)
	}
	return svc
}
