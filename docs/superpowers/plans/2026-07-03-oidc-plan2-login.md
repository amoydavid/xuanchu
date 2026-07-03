# OIDC 接入 Plan 2：OIDC 登录 + browser_session + authMiddleware 双通道

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax.

**Goal:** 用户通过 yaoguang OIDC（Auth Code Flow + PKCE）登录后，建立独立 browser_session 并下发 httpOnly Cookie；authMiddleware 改为「Bearer 优先、Cookie 兜底」双通道，且 cookie 仅允许 GET 读操作（写操作强制 Bearer 防 CSRF）。

**对应 spec:** `docs/superpowers/specs/2026-07-03-workspace-oidc-design.md`（第 6、7 节）

**依赖：** Plan 1 已完成（OIDCConfigService、BrowserSession/BrowserAuthFlow repo、UserExternalID 映射、DirectorySyncService）。

**新增依赖：** `github.com/coreos/go-oidc/v3`、`golang.org/x/oauth2`

---

## 关键现有代码参考

- `authMiddleware`：`internal/httpapi/middleware.go:153-201`——当前只认 `Authorization: Bearer`，无 Bearer 直接 401。
- `humaRoute.Public` 字段：`internal/httpapi/huma_routes.go:20,92-97`——`Public: true` 的路由跳过 authMiddleware（SSO start/callback/logout 用）。
- `AuthenticatedToken`：`internal/app/token.go:105-112`——中间件构造的认证结果。
- `requestAuth`：`internal/httpapi/middleware.go:22-26`——中间件写入 context 的结构。
- `visibleAndEffectiveWorkspaces`：`internal/httpapi/middleware.go:230-253`——从 authn 解析 workspace。
- Cookie 名：`xuanchu_session`。

---

### Task 1: 引入 OIDC RP 依赖

**Files:**
- Modify: `go.mod`、`go.sum`

- [ ] **Step 1: 添加依赖**

```bash
go get github.com/coreos/go-oidc/v3/oidc
go get golang.org/x/oauth2
```

- [ ] **Step 2: 验证零 CGO 构建**

```bash
CGO_ENABLED=0 go build ./...
```
Expected: 编译通过（两个包都是纯 Go）。

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: 引入 go-oidc 与 oauth2 依赖"
```

---

### Task 2: OIDC RP 包——discovery + code exchange + id_token 校验

**Files:**
- Create: `internal/auth/oidc/provider.go`
- Create: `internal/auth/oidc/provider_test.go`

- [ ] **Step 1: 写失败测试 `internal/auth/oidc/provider_test.go`**

用 `httptest.Server` mock 一个最小 IdP（discovery + token + JWKS），签发一个用测试私钥签名的 id_token。覆盖：code exchange 成功返回 sub、验签失败报错、aud 不匹配报错。

```go
package oidc

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// 用 ECDSA 测试密钥签发 id_token，mock 一个最小 IdP。
func setupMockIdP(t *testing.T, clientID string) (baseURL string, priv *ecdsa.PrivateKey, srv *httptest.Server) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	jwksHandler := serveTestJWKS(priv)
	mux := http.NewServeMux()
	mux.Handle("/.well-known/openid-configuration", discoveryHandler(func() string { return srv.URL }, clientID))
	mux.Handle("/jwks", jwksHandler)
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		idToken := mintTestIDToken(t, priv, srv.URL, clientID, "yaoguang_member:m1")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "fake",
			"token_type":   "Bearer",
			"id_token":     idToken,
			"expires_in":   3600,
		})
	})
	srv = httptest.NewServer(mux)
	return srv.URL, priv, srv
}

func TestExchangeReturnsSub(t *testing.T) {
	base, _, srv := setupMockIdP(t, "client1")
	defer srv.Close()

	p := NewProvider(context.Background(), base, "client1", "secret1")
	url := p.AuthCodeURL("state123", "verifier123", base+"/sso/oidc/callback")
	if url == "" {
		t.Fatal("empty auth url")
	}

	token, err := p.Exchange(context.Background(), "fakecode", "verifier123", base+"/sso/oidc/callback")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if token.Subject != "yaoguang_member:m1" {
		t.Fatalf("sub = %s, want yaoguang_member:m1", token.Subject)
	}
}

func TestExchangeAudMismatchFails(t *testing.T) {
	base, _, srv := setupMockIdP(t, "client1")
	defer srv.Close()

	p := NewProvider(context.Background(), base, "WRONG_CLIENT", "secret1")
	_, err := p.Exchange(context.Background(), "fakecode", "verifier123", base+"/sso/oidc/callback")
	if err == nil {
		t.Fatal("expected aud mismatch error")
	}
}
```

> 辅助函数 `serveTestJWKS`、`discoveryHandler`、`mintTestIDToken` 见 Step 3——它们是测试基础设施，用 jose/crypto 签发 JWT。执行者需补全这些 helper（标准做法：用 `github.com/go-jose/go-jose/v3` 签发测试 JWT）。

- [ ] **Step 2: 运行确认失败**

```bash
go test ./internal/auth/oidc/ -v
```
Expected: FAIL（包/handler 不存在）。

- [ ] **Step 3: 实现 `internal/auth/oidc/provider.go`**

```go
// Package oidc 是 xuanchu 作为 OIDC Relying Party 的纯逻辑层，
// 封装 discovery、Auth Code Flow + PKCE、id_token 校验。不依赖 GORM/HTTP server。
package oidc

import (
	"context"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Token 是校验通过后的 id_token 关键字段。
type Token struct {
	Subject string
	Raw     string
}

// Provider 封装某次配置（issuer/client）对应的 OIDC RP 状态。
type Provider struct {
	oidcProvider *oidc.Provider
	oauthConfig  *oauth2.Config
	verifier     *oidc.IDTokenVerifier
}

// NewProvider 用 issuerBaseURL 做 discovery，构造 RP。
func NewProvider(ctx context.Context, issuerBaseURL, clientID, clientSecret string) *Provider {
	// coreos/go-oidc 的 Provider discovery 走 {issuer}/.well-known/openid-configuration
	// yaoguang 的 issuer_base_url 就是根 URL，需确保 discovery endpoint 在其下。
	provider, err := oidc.NewProvider(ctx, issuerBaseURL)
	if err != nil {
		// 调用方应处理；这里 panic 仅用于构造失败（实际由 NewProviderSafe 包装返回 error）
		panic(fmt.Sprintf("oidc discovery failed: %v", err))
	}
	config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
		RedirectURL:  "", // 由调用方传 redirectURI
	}
	verifier := provider.Verifier(&oidc.Config{ClientID: clientID})
	return &Provider{oidcProvider: provider, oauthConfig: config, verifier: verifier}
}

// NewProviderSafe 与 NewProvider 相同但返回 error（生产代码用）。
func NewProviderSafe(ctx context.Context, issuerBaseURL, clientID, clientSecret string) (*Provider, error) {
	provider, err := oidc.NewProvider(ctx, issuerBaseURL)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery: %w", err)
	}
	config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}
	verifier := provider.Verifier(&oidc.Config{ClientID: clientID})
	return &Provider{oidcProvider: provider, oauthConfig: config, verifier: verifier}, nil
}

// AuthCodeURL 生成跳转 IdP 的授权 URL（含 state + PKCE）。
func (p *Provider) AuthCodeURL(state, pkceVerifier, redirectURI string) string {
	p.oauthConfig.RedirectURL = redirectURI
	return p.oauthConfig.AuthCodeURL(state,
		oauth2.SetAuthURLParam("code_challenge", pkceChallengeS256(pkceVerifier)),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
}

// Exchange 用 authorization code 换 id_token 并校验。
func (p *Provider) Exchange(ctx context.Context, code, pkceVerifier, redirectURI string) (*Token, error) {
	p.oauthConfig.RedirectURL = redirectURI
	token, err := p.oauthConfig.Exchange(ctx, code,
		oauth2.SetAuthURLParam("code_verifier", pkceVerifier),
	)
	if err != nil {
		return nil, fmt.Errorf("oauth exchange: %w", err)
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return nil, fmt.Errorf("id_token missing in token response")
	}
	idToken, err := p.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("id_token verify: %w", err)
	}
	return &Token{Subject: idToken.Subject, Raw: rawIDToken}, nil
}

// pkceChallengeS256 按 RFC 7636 计算 S256 challenge。
func pkceChallengeS256(verifier string) string {
	// 复用 oauth2/generate 实现或手写：BASE64URL(SHA256(verifier))
	return oauth2.GenerateCodeVerifier // 占位，Step 4 修正
}
```

- [ ] **Step 4: 修正 PKCE challenge 实现**

`oauth2` 包没有直接导出 S256 helper，需手写。替换 `pkceChallengeS256`：

```go
import (
	"crypto/sha256"
	"encoding/base64"
)

func pkceChallengeS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
```

并把 `AuthCodeURL` 里对 verifier 的处理改为：调用方传入的 `pkceVerifier` 就是 code_verifier，challenge 由本函数算出。测试里 setupMockIdP 的 `/token` 不校验 PKCE（简化测试），生产由 yaoguang 校验。

同时补全测试 helper：`serveTestJWKS`（用 `go-jose` 导出公钥 JWKS）、`discoveryHandler`（返回 issuer/authorization_endpoint/token_endpoint/jwks_uri）、`mintTestIDToken`（用私钥签发带 sub/iss/aud/exp 的 JWT）。执行者按 `go-jose` 标准用法实现。

- [ ] **Step 5: 运行测试**

```bash
go test ./internal/auth/oidc/ -v
```
Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add internal/auth/oidc/ go.mod go.sum
git commit -m "feat: OIDC RP 包（discovery+PKCE+id_token 校验）"
```

---

### Task 3: OIDCAuthService——Start/Callback/Logout 流程

**Files:**
- Create: `internal/app/oidc_auth.go`
- Create: `internal/app/oidc_auth_test.go`

职责：编排 OIDC flow 与 session 生命周期。Start 生成 state+PKCE 存 BrowserAuthFlow；Callback 验证、sub 映射、建 session；Logout 删 session。

- [ ] **Step 1: 写失败测试**

```go
package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"testing"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

func newAuthTestStore(t *testing.T) *storage.Store {
	store, err := storage.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func hashStr(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestStartCreatesAuthFlow(t *testing.T) {
	store := newAuthTestStore(t)
	sessionRepo := storage.NewSessionRepository(store.DB())
	cfgRepo := storage.NewConfigRepository(store.DB())
	cfgSvc := NewOIDCConfigService(cfgRepo)
	// 预置配置（启用）
	_ = cfgSvc.Set("ws1", OIDCConfigInput{Provider: "yaoguang", IssuerBaseURL: "http://fake", OrgID: "o", ClientID: "c", ClientSecret: "s", DirectoryAccessToken: "t", ExternalBaseURL: "http://xuanchu"})

	svc := NewOIDCAuthService(store, sessionRepo, cfgSvc, &stubOIDCProvider{})
	url, err := svc.Start(context.Background(), "ws1")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if url == "" {
		t.Fatal("empty url")
	}
	// state 应已写入 BrowserAuthFlow
	flows := listAuthFlows(store)
	if len(flows) != 1 {
		t.Fatalf("flows = %d", len(flows))
	}
}

func TestCallbackSubNotMappedRejected(t *testing.T) {
	store := newAuthTestStore(t)
	sessionRepo := storage.NewSessionRepository(store.DB())
	cfgRepo := storage.NewConfigRepository(store.DB())
	cfgSvc := NewOIDCConfigService(cfgRepo)
	_ = cfgSvc.Set("ws1", OIDCConfigInput{Provider: "yaoguang", IssuerBaseURL: "http://fake", OrgID: "o", ClientID: "c", ClientSecret: "s", DirectoryAccessToken: "t", ExternalBaseURL: "http://xuanchu"})

	svc := NewOIDCAuthService(store, sessionRepo, cfgSvc, &stubOIDCProvider{})
	// 先 Start 拿到 state+verifier
	_, _ = svc.Start(context.Background(), "ws1")
	flow := listAuthFlows(store)[0]

	// callback：sub 未在 UserExternalID 映射 → 应报 identity_not_found
	_, err := svc.Callback(context.Background(), flow.State, "fakecode")
	if err == nil {
		t.Fatal("expected identity_not_found error")
	}
}

func TestCallbackSubMappedCreatesSession(t *testing.T) {
	store := newAuthTestStore(t)
	sessionRepo := storage.NewSessionRepository(store.DB())
	cfgRepo := storage.NewConfigRepository(store.DB())
	cfgSvc := NewOIDCConfigService(cfgRepo)
	_ = cfgSvc.Set("ws1", OIDCConfigInput{Provider: "yaoguang", IssuerBaseURL: "http://fake", OrgID: "o", ClientID: "c", ClientSecret: "s", DirectoryAccessToken: "t", ExternalBaseURL: "http://xuanchu"})

	// 预置 user + external id + membership（模拟 Plan 1 同步结果）
	userRepo := storage.NewUserRepository(store.DB())
	u, _ := userRepo.Create(storage.User{Name: "张三", CreatedAt: 1, ModifiedAt: 1})
	extRepo := storage.NewExternalIDRepository(store.DB())
	_, _ = extRepo.Create(storage.UserExternalID{ID: "e1", UserID: u.ID, Provider: "yaoguang", ExternalID: "yaoguang_member:m1", CreatedAt: 1})
	wsRepo := storage.NewWorkspaceRepository(store.DB())
	ws, _ := wsRepo.Create(storage.Workspace{Slug: "ws1", Name: "WS1"})
	memberRepo := storage.NewMemberRepository(store.DB())
	_ = memberRepo.Upsert(storage.Membership{UserID: u.ID, WorkspaceID: ws.ID, Role: "member", ModifiedAt: 1})

	svc := NewOIDCAuthService(store, sessionRepo, cfgSvc, &stubOIDCProvider{sub: "yaoguang_member:m1"})
	_, _ = svc.Start(context.Background(), ws.ID)
	flow := listAuthFlows(store)[0]

	rawSession, err := svc.Callback(context.Background(), flow.State, "fakecode")
	if err != nil {
		t.Fatalf("Callback: %v", err)
	}
	if rawSession == "" {
		t.Fatal("empty session token")
	}
	// session 哈希应已写入
	_, err = sessionRepo.GetSession(hashStr(rawSession))
	if err != nil {
		t.Fatalf("session not found: %v", err)
	}
	// flow 应已删除
	_, err = sessionRepo.GetAuthFlow(flow.State)
	if err != storage.ErrNotFound {
		t.Fatalf("flow should be deleted")
	}
}

// stubOIDCProvider 是测试用的 OIDCProvider 接口实现。
type stubOIDCProvider struct {
	sub string
}

func (s *stubOIDCProvider) AuthCodeURL(state, verifier, redirectURI string) string {
	return "http://fake/auth?state=" + state
}
func (s *stubOIDCProvider) Exchange(ctx context.Context, code, verifier, redirectURI string) (string, error) {
	if s.sub == "" {
		return "yaoguang_member:m1", nil
	}
	return s.sub, nil
}

func listAuthFlows(store *storage.Store) []storage.BrowserAuthFlow {
	var flows []storage.BrowserAuthFlow
	store.DB().Find(&flows)
	return flows
}
```

> `OIDCProvider` 接口让 Callback 返回 sub 字符串（Exchange 在 stub 里直接返回 sub，绕过真实 id_token 校验），生产实现由 Task 4 的 http handler 用 `oidc.Provider.Exchange` 拼装。

- [ ] **Step 2: 运行确认失败**

```bash
go test ./internal/app/ -run "TestStart|TestCallback" -v
```
Expected: FAIL（NewOIDCAuthService 未定义）。

- [ ] **Step 3: 实现 `internal/app/oidc_auth.go`**

```go
package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

// OIDCProvider 抽象 OIDC RP 的两个动作，便于测试 mock 与生产实现分离。
type OIDCProvider interface {
	AuthCodeURL(state, pkceVerifier, redirectURI string) string
	Exchange(ctx context.Context, code, pkceVerifier, redirectURI string) (sub string, err error)
}

type OIDCAuthService struct {
	store      *storage.Store
	sessionRepo *storage.SessionRepository
	cfg        *OIDCConfigService
	// providerFactory 按配置构造 OIDCProvider；生产用 oidc.NewProviderSafe，测试直接注入 stub。
	providerFactory func(issuerBaseURL, clientID, clientSecret string) (OIDCProvider, error)
}

func NewOIDCAuthService(store *storage.Store, sessionRepo *storage.SessionRepository, cfg *OIDCConfigService, fallback OIDCProvider) *OIDCAuthService {
	svc := &OIDCAuthService{store: store, sessionRepo: sessionRepo, cfg: cfg}
	if fallback != nil {
		// 测试模式：固定 provider
		svc.providerFactory = func(_, _, _ string) (OIDCProvider, error) { return fallback, nil }
	} else {
		svc.providerFactory = newOIDCProviderFromConfig
	}
	return svc
}

const (
	authFlowTTLSeconds int64 = 600 // 10 分钟
)

func (s *OIDCAuthService) Start(ctx context.Context, workspaceID string) (string, error) {
	cfg, enabled := s.cfg.Get(workspaceID)
	if !enabled {
		return "", fmt.Errorf("sso_not_enabled")
	}
	secrets, err := s.cfg.ResolveSecrets(workspaceID)
	if err != nil {
		return "", err
	}

	state := randomToken(32)
	verifier := randomToken(48)
	now := time.Now().Unix()
	redirectURI := redirectURIFromConfig(cfg)

	if err := s.sessionRepo.CreateAuthFlow(state, workspaceID, verifier, now, now+authFlowTTLSeconds); err != nil {
		return "", err
	}

	provider, err := s.providerFactory(cfg.IssuerBaseURL, cfg.ClientID, secrets.ClientSecret)
	if err != nil {
		return "", err
	}
	return provider.AuthCodeURL(state, verifier, redirectURI), nil
}

func (s *OIDCAuthService) Callback(ctx context.Context, state, code string) (string, error) {
	flow, err := s.sessionRepo.GetAuthFlow(state)
	if err != nil {
		return "", fmt.Errorf("invalid_state")
	}
	now := time.Now().Unix()
	if flow.ExpiresAt < now {
		_ = s.sessionRepo.DeleteAuthFlow(state)
		return "", fmt.Errorf("invalid_state")
	}

	cfg, enabled := s.cfg.Get(flow.WorkspaceID)
	if !enabled {
		return "", fmt.Errorf("sso_not_enabled")
	}
	secrets, err := s.cfg.ResolveSecrets(flow.WorkspaceID)
	if err != nil {
		return "", err
	}
	redirectURI := redirectURIFromConfig(cfg)

	provider, err := s.providerFactory(cfg.IssuerBaseURL, cfg.ClientID, secrets.ClientSecret)
	if err != nil {
		return "", err
	}
	sub, err := provider.Exchange(ctx, code, flow.PKCEVerifier, redirectURI)
	if err != nil {
		return "", fmt.Errorf("id_token_invalid: %w", err)
	}

	// sub 映射
	extRepo := storage.NewExternalIDRepository(s.store.DB())
	ext, err := extRepo.GetByProviderAndExternalID("yaoguang", sub)
	if err != nil {
		return "", fmt.Errorf("identity_not_found")
	}

	// 校验 membership 存在
	memberRepo := storage.NewMemberRepository(s.store.DB())
	_, err = memberRepo.Get(ext.UserID, flow.WorkspaceID)
	if err != nil {
		return "", fmt.Errorf("membership_inactive")
	}

	// 建 session
	ttl, _ := time.ParseDuration(defaultIfEmpty(cfg.SessionTTL, "168h"))
	rawSession := randomToken(32)
	sessionHash := hashHex(rawSession)
	if err := s.sessionRepo.CreateSession(sessionHash, ext.UserID, flow.WorkspaceID, now, now+int64(ttl.Seconds())); err != nil {
		return "", err
	}
	_ = s.sessionRepo.DeleteAuthFlow(state)
	return rawSession, nil
}

func (s *OIDCAuthService) Logout(sessionHash string) error {
	return s.sessionRepo.DeleteSession(sessionHash)
}

// ResolveSession 由 authMiddleware 调用：raw cookie token → sessionHash → 校验未过期 → 返回 user/workspace。
func (s *OIDCAuthService) ResolveSession(rawCookie string) (storage.BrowserSession, error) {
	sessionHash := hashHex(rawCookie)
	session, err := s.sessionRepo.GetSession(sessionHash)
	if err != nil {
		return storage.BrowserSession{}, err
	}
	if session.ExpiresAt < time.Now().Unix() {
		_ = s.sessionRepo.DeleteSession(sessionHash)
		return storage.BrowserSession{}, fmt.Errorf("session_expired")
	}
	return session, nil
}

func redirectURIFromConfig(cfg OIDCConfig) string {
	base := cfg.ExternalBaseURL
	path := cfg.RedirectPath
	if path == "" {
		path = "/sso/oidc/callback"
	}
	if base == "" {
		return path
	}
	return base + path
}

func randomToken(n int) string {
	buf := make([]byte, n)
	_, _ = rand.Read(buf)
	return base64.RawURLEncoding.EncodeToString(buf)
}

func hashHex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// newOIDCProviderFromConfig 是生产用 providerFactory（Task 4 挂入）。
func newOIDCProviderFromConfig(issuerBaseURL, clientID, clientSecret string) (OIDCProvider, error) {
	return nil, fmt.Errorf("not implemented") // 见 Task 4
}

var _ = auth.TokenTypePAT // 占位 import 使用
```

- [ ] **Step 4: 运行测试**

```bash
go test ./internal/app/ -run "TestStart|TestCallback" -v
```
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/app/oidc_auth.go internal/app/oidc_auth_test.go
git commit -m "feat: OIDCAuthService 登录流程编排"
```

---

### Task 4: 接入真实 OIDC Provider 工厂

**Files:**
- Modify: `internal/app/oidc_auth.go`（`newOIDCProviderFromConfig`）

- [ ] **Step 1: 实现生产 providerFactory**

把 `newOIDCProviderFromConfig` 替换为调用 `internal/auth/oidc` 包：

```go
import (
	xuanchuOIDC "git.dajee.net/dajee/xuanchu/internal/auth/oidc"
)

func newOIDCProviderFromConfig(issuerBaseURL, clientID, clientSecret string) (OIDCProvider, error) {
	p, err := xuanchuOIDC.NewProviderSafe(context.Background(), issuerBaseURL, clientID, clientSecret)
	if err != nil {
		return nil, err
	}
	return &realOIDCProvider{p: p}, nil
}

type realOIDCProvider struct {
	p *xuanchuOIDC.Provider
}

func (r *realOIDCProvider) AuthCodeURL(state, verifier, redirectURI string) string {
	return r.p.AuthCodeURL(state, verifier, redirectURI)
}

func (r *realOIDCProvider) Exchange(ctx context.Context, code, verifier, redirectURI string) (string, error) {
	tok, err := r.p.Exchange(ctx, code, verifier, redirectURI)
	if err != nil {
		return "", err
	}
	return tok.Subject, nil
}
```

- [ ] **Step 2: 验证编译**

```bash
go build ./...
```
Expected: 通过。

- [ ] **Step 3: Commit**

```bash
git add internal/app/oidc_auth.go
git commit -m "feat: 接入真实 OIDC Provider 工厂"
```

---

### Task 5: SSO HTTP 路由（start/callback/logout）

**Files:**
- Modify: `internal/httpapi/huma_routes.go`（追加 Public 路由）
- Create: `internal/httpapi/sso_auth.go`
- Create: `internal/httpapi/sso_auth_test.go`

- [ ] **Step 1: 在 humaRoutes() 追加三条 Public 路由**

在 `humaRoutes()` 返回的 slice 里追加（放在 healthz 附近）：

```go
		{Method: http.MethodGet, Path: "/sso/oidc/start", Tag: "SSO", Summary: "Start OIDC login flow.", Handler: s.handleSsoOidcStart, Public: true},
		{Method: http.MethodGet, Path: "/sso/oidc/callback", Tag: "SSO", Summary: "OIDC login callback.", Handler: s.handleSsoOidcCallback, Public: true},
		{Method: http.MethodPost, Path: "/auth/logout", Tag: "SSO", Summary: "Logout browser session.", Handler: s.handleAuthLogout, Public: true},
```

> `Public: true` 让它们跳过 authMiddleware（见 `huma_routes.go:92-97`）。

- [ ] **Step 2: 实现 handler `internal/httpapi/sso_auth.go`**

```go
package httpapi

import (
	"net/http"
	"net/url"

	"git.dajee.net/dajee/xuanchu/internal/app"
)

const sessionCookieName = "xuanchu_session"

func (s *Server) handleSsoOidcStart(w http.ResponseWriter, r *http.Request) {
	workspaceRef := r.URL.Query().Get("workspace")
	if workspaceRef == "" {
		writeError(w, http.StatusBadRequest, "missing_workspace", "workspace 参数必填", nil)
		return
	}
	wsID, err := s.resolveWorkspaceID(workspaceRef)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace_not_found", "workspace 不存在", nil)
		return
	}
	authURL, err := s.oidcAuth.Start(r.Context(), wsID)
	if err != nil {
		redirectWithError(w, r, "sso_start_failed")
		return
	}
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (s *Server) handleSsoOidcCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	if state == "" || code == "" {
		redirectWithError(w, r, "invalid_callback")
		return
	}
	rawSession, err := s.oidcAuth.Callback(r.Context(), state, code)
	if err != nil {
		redirectWithError(w, r, appErrorToSsoCode(err))
		return
	}
	s.setSessionCookie(w, rawSession, s.console.BasePath)
	// 回到 console 根
	http.Redirect(w, r, s.console.BasePath+"/", http.StatusFound)
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && cookie.Value != "" {
		_ = s.oidcAuth.Logout(hashHex(cookie.Value))
	}
	clearSessionCookie(w, s.console.BasePath)
	http.Redirect(w, r, s.console.BasePath+"/", http.StatusFound)
}

func (s *Server) setSessionCookie(w http.ResponseWriter, rawSession, path string) {
	secure := !s.oidcInsecureCookie
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    rawSession,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   7 * 24 * 3600,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter, path string) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1,
	})
}

func redirectWithError(w http.ResponseWriter, r *http.Request, code string) {
	target := "/?sso_error=" + url.QueryEscape(code)
	http.Redirect(w, r, target, http.StatusFound)
}

func appErrorToSsoCode(err error) string {
	msg := err.Error()
	switch {
	case contains(msg, "identity_not_found"):
		return "identity_not_found"
	case contains(msg, "membership_inactive"):
		return "membership_inactive"
	case contains(msg, "invalid_state"):
		return "invalid_state"
	case contains(msg, "id_token_invalid"):
		return "id_token_invalid"
	default:
		return "sso_failed"
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
```

> 注：`contains`/`indexOf` 是为避免引 strings 包（sso_auth.go 保持轻量）；执行者可改用 `strings.Contains`。`s.resolveWorkspaceID`、`s.console.BasePath`、`s.oidcInsecureCookie` 需在 Server 结构体补齐。

- [ ] **Step 3: 在 Server 结构体补依赖字段**

`internal/httpapi/server.go` 的 Server 加：

```go
	oidcAuth          *app.OIDCAuthService
	oidcInsecureCookie bool
```

并在 Server 构造处注入 `oidcAuth`（`app.NewOIDCAuthService(store, sessionRepo, oidcConfigSvc, nil)`，`nil` 表示生产模式用真实 providerFactory）。`oidcInsecureCookie` 从 OIDCConfig 的 `sso.insecure_cookie` 读取（或全局 flag）。

- [ ] **Step 4: 写 handler 测试 `internal/httpapi/sso_auth_test.go`**

用 mock oidcAuth（注入一个实现了 Start/Callback/Logout 的 stub）测试：start 返回 302、callback 成功 set cookie、logout 清 cookie。具体 Server 构造参照既有 httpapi 测试 harness。

```go
package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSsoStartRedirects(t *testing.T) {
	// mock oidcAuth.Start 返回固定 URL，验证 302
}

func TestSsoCallbackSetsCookie(t *testing.T) {
	// mock oidcAuth.Callback 返回 rawSession，验证 Set-Cookie 头
}

func TestAuthLogoutClearsCookie(t *testing.T) {
	// 验证 cookie 被清除（MaxAge -1）
}
```

> 三种 handler 测试都依赖注入 mock oidcAuth 到 Server。执行者参照 `internal/httpapi/*_test.go` 的 Server 构造与依赖注入方式补全。

- [ ] **Step 5: 运行测试**

```bash
go test ./internal/httpapi/ -run "TestSso|TestAuthLogout" -v
```
Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add internal/httpapi/huma_routes.go internal/httpapi/sso_auth.go internal/httpapi/server.go internal/httpapi/sso_auth_test.go
git commit -m "feat: SSO start/callback/logout HTTP 路由"
```

---

### Task 6: authMiddleware 双通道（Bearer 优先、Cookie 兜底 + 写操作禁 cookie）

**Files:**
- Modify: `internal/httpapi/middleware.go:153-201`

这是核心安全改动。逻辑：

1. 提取 Bearer；有 → 走现有 token 认证（不变）。
2. 无 Bearer 但有 cookie → 尝试 session 认证 → 但若请求是写操作（POST/PUT/PATCH/DELETE 且 path 以 `/api/v1/` 开头）→ 403 `cookie_write_forbidden`。
3. 都没有 → 401。

- [ ] **Step 1: 写失败测试 `internal/httpapi/middleware_dual_test.go`**

```go
package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCookieGetAllowed(t *testing.T) {
	// 带 cookie 的 GET /api/v1/tasks 应被放行（mock session 有效）
	// 验证下游 handler 收到 requestAuth
}

func TestCookieWriteForbidden(t *testing.T) {
	// 带 cookie 的 POST /api/v1/tasks 应返回 403 cookie_write_forbidden
}

func TestBearerStillWorks(t *testing.T) {
	// 带 Bearer 的请求走原有逻辑，不受影响
}

func TestNoCredentialUnauthorized(t *testing.T) {
	// 无 Bearer 无 cookie → 401
}
```

> 测试需构造带 session 的 Server。`CookieGetAllowed` 的 session 有效性靠 mock `oidcAuth.ResolveSession` 返回固定 BrowserSession，再由中间件构造 requestAuth。执行者按既有 httpapi test harness 实现。

- [ ] **Step 2: 改造 authMiddleware**

修改 `internal/httpapi/middleware.go` 的 `authMiddleware`（替换 L153-201）：

```go
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, hasBearer := bearerToken(r.Header.Get("Authorization"))

		if hasBearer {
			s.handleBearerAuth(w, r, raw, next)
			return
		}

		// 无 Bearer，尝试 cookie
		cookie, err := r.Cookie(sessionCookieName)
		if err == nil && cookie.Value != "" {
			// 写操作（POST/PUT/PATCH/DELETE 且 /api/v1/*）禁 cookie，防 CSRF
			if isApiWriteMethod(r.Method) && strings.HasPrefix(r.URL.Path, "/api/v1/") {
				writeError(w, http.StatusForbidden, "cookie_write_forbidden", "此操作需要 access token", nil)
				return
			}
			s.handleCookieAuth(w, r, cookie.Value, next)
			return
		}

		writeError(w, http.StatusUnauthorized, authz.CodeAuthMissingToken, "missing bearer token or session", nil)
	})
}

// handleBearerAuth 是原有 token 认证逻辑（抽出来保持原样）。
func (s *Server) handleBearerAuth(w http.ResponseWriter, r *http.Request, raw string, next http.Handler) {
	svc, err := app.NewService(app.ServiceOptions{
		Store: s.store, Clock: s.effectiveClock(), Runtime: &app.RuntimeContext{}, DisableScopeBootstrap: true,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "api_internal", "internal server error", nil)
		return
	}
	authn, err := svc.AuthenticateBearerToken(raw)
	if err != nil {
		writeAppError(w, err)
		return
	}
	visible, effective, err := s.visibleAndEffectiveWorkspaces(authn)
	if err != nil {
		writeAppError(w, err)
		return
	}
	s.populateLogState(r, authn, effective)
	next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), authContextKey, requestAuth{
		Authn: authn, VisibleWorkspaces: visible, EffectiveWorkspace: effective,
	})))
}

// handleCookieAuth 用 browser session 构造认证上下文。
func (s *Server) handleCookieAuth(w http.ResponseWriter, r *http.Request, rawCookie string, next http.Handler) {
	session, err := s.oidcAuth.ResolveSession(rawCookie)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_session", "session 无效或已过期", nil)
		return
	}
	// 构造 AuthenticatedToken：用户为 session.UserID，凭证类型 browser_session
	userRepo := storage.NewUserRepository(s.store.DB())
	user, err := userRepo.GetByID(session.UserID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_session", "session 用户不存在", nil)
		return
	}
	workspace, err := storage.NewWorkspaceRepository(s.store.DB()).GetByID(session.WorkspaceID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_session", "session workspace 不存在", nil)
		return
	}
	memberRepo := storage.NewMemberRepository(s.store.DB())
	member, err := memberRepo.Get(user.ID, workspace.ID)
	if err != nil {
		writeError(w, http.StatusForbidden, "membership_inactive", "您不是该工作区的成员", nil)
		return
	}
	authn := app.AuthenticatedToken{
		Token: app.TokenView{
			ID: "browser_session:" + session.ID,
			Name: "Browser Session",
			Type: "browser_session",
		},
		User: user,
	}
	// cookie 模式：effective workspace 固定为 session 的 workspace，role 用 member.Role
	visible := []storage.WorkspaceWithRole{{Workspace: workspace, Role: member.Role}}
	s.populateLogState(r, authn, workspace)
	next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), authContextKey, requestAuth{
		Authn: authn, VisibleWorkspaces: visible, EffectiveWorkspace: workspace,
	})))
}

// populateLogState 抽出原有 logState 写入逻辑。
func (s *Server) populateLogState(r *http.Request, authn app.AuthenticatedToken, effective storage.Workspace) {
	if state, ok := r.Context().Value(logStateContextKey).(*requestLogState); ok {
		if authn.TenantActor {
			state.actorType = "tenant_access_token"
			state.tokenName = authn.Token.Name
			state.tokenPrefix = authn.Token.Prefix
		} else {
			state.actorType = "user"
			state.actorID = authn.User.ID
		}
		state.tokenID = authn.Token.ID
		state.workspaceID = effective.ID
		state.workspaceRef = effective.Slug
	}
}

func isApiWriteMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}
```

> 注意：`app.TokenView` 的字段（ID/Name/Type 等）需对照 `internal/app/token.go` 实际定义补全。`storage.WorkspaceWithRole` 已有 Role 字段（见 `visibleAndEffectiveWorkspaces` 用法）。`TokenView.Type` 值用 `"browser_session"` 与 `authz.CredentialBrowserSession` 一致。

- [ ] **Step 3: 更新 `CredentialBrowserSession` 注释**

`internal/authz/model.go:72`：

```go
	CredentialBrowserSession CredentialKind = "browser_session"
```

去掉「预留，本次不实现」注释，改为「OIDC 登录产生的浏览器会话凭证」。

- [ ] **Step 4: 运行测试**

```bash
go test ./internal/httpapi/ -run "TestCookie|TestBearer|TestNoCredential" -v
go test ./...
```
Expected: PASS（含既有 authMiddleware 测试不受影响）。

- [ ] **Step 5: Commit**

```bash
git add internal/httpapi/middleware.go internal/authz/model.go internal/httpapi/middleware_dual_test.go
git commit -m "feat: authMiddleware 双通道（Bearer+Cookie，写操作禁 cookie）"
```

---

### Task 7: session 过期清理 runtime goroutine

**Files:**
- Modify: `internal/cli/server.go`（runtime goroutine）
- 或并入 Task 9（Plan 1）的 DirectorySyncRuntime ticker

- [ ] **Step 1: 在 server.go 加 session 清理 goroutine**

在 runtime 编排处追加（或并入既有 ticker）：

```go
go func() {
	defer runtimeWG.Done()
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-runCtx.Done():
			return
		case <-ticker.C:
			now := time.Now().Unix()
			sessionRepo := storage.NewSessionRepository(store.DB())
			_, _ = sessionRepo.PurgeExpiredSessions(now)
			_, _ = sessionRepo.PurgeExpiredAuthFlows(now)
		}
	}
}()
```

> 若 Plan 1 的 DirectorySyncRuntime 已在 server.go 加了 `runtimeWG.Add(4)`，这里改为 `Add(5)`。

- [ ] **Step 2: 验证编译**

```bash
CGO_ENABLED=0 go build ./cmd/xuanchu
```

- [ ] **Step 3: Commit**

```bash
git add internal/cli/server.go
git commit -m "feat: session/flow 过期定时清理"
```

---

### Task 8: 阶段二全量验证

- [ ] **Step 1: 全量测试**

```bash
go test ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./cmd/xuanchu
```
Expected: 全 PASS。

- [ ] **Step 2: 端到端手动验证（需 yaoguang 可达）**

1. （Plan 1 已完成）配置 SSO + 同步成员
2. 浏览器访问 `/sso/oidc/start?workspace=<slug>` → 跳转 yaoguang 登录
3. 回调后回到 console，带 cookie
4. `GET /api/v1/tasks` 用 cookie 访问成功
5. `POST /api/v1/tasks` 用 cookie → 403 `cookie_write_forbidden`
6. `/auth/logout` 清 cookie

- [ ] **Step 3: Commit**

```bash
git commit --allow-empty -m "chore: 阶段二（OIDC 登录+session）验证通过"
```

---

## 阶段二完成标准

- [ ] `/sso/oidc/start` + `/sso/oidc/callback` 完成 OIDC Auth Code Flow + PKCE
- [ ] sub 命中 UserExternalID 映射 → 建 browser_session + 下发 cookie；未命中 → identity_not_found
- [ ] authMiddleware 双通道：Bearer 优先，cookie 仅 GET，写操作禁 cookie
- [ ] `/auth/logout` 清 session + cookie
- [ ] 过期 session/flow 定时清理
- [ ] 全量 `go test ./...` 与 `CGO_ENABLED=0` 通过
- [ ] 所有步骤已 commit
