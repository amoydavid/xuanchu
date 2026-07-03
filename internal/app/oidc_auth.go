package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

// OIDC 登录流程的 typed errors，供 HTTP 层用 errors.Is 精确映射错误码（spec §7.5）。
var (
	ErrSsoNotEnabled    = errors.New("sso_not_enabled")
	ErrInvalidState     = errors.New("invalid_state")
	ErrIDTokenInvalid   = errors.New("id_token_invalid")
	ErrIdentityNotFound = errors.New("identity_not_found")
	ErrMembershipInactive = errors.New("membership_inactive")
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

// BrowserLoginResult 是 OIDC 登录成功后返回的凭证：session 写 HttpOnly cookie，CSRF 写可读 cookie。
type BrowserLoginResult struct {
	RawSession     string
	CSRFToken      string
	InsecureCookie bool   // 来自 workspace sso.insecure_cookie，控制 session/csrf cookie 的 Secure 属性
	SessionMaxAge  int    // 秒，来自 sso.session_ttl，控制 cookie MaxAge
}

// NewOIDCAuthService 构造 OIDCAuthService。fallback 非 nil 时用于测试注入固定 provider。
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

const authFlowTTLSeconds int64 = 600 // 10 分钟

// Start 生成 state + PKCE verifier，存 BrowserAuthFlow，返回 IdP 授权跳转 URL。
func (s *OIDCAuthService) Start(ctx context.Context, workspaceID string) (string, error) {
	cfg, enabled := s.cfg.Get(workspaceID)
	if !enabled {
		return "", ErrSsoNotEnabled
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

// Callback 验证 state + id_token，sub 命中映射后建 browser session。
func (s *OIDCAuthService) Callback(ctx context.Context, state, code string) (BrowserLoginResult, error) {
	flow, err := s.sessionRepo.GetAuthFlow(state)
	if err != nil {
		return BrowserLoginResult{}, ErrInvalidState
	}
	now := time.Now().Unix()
	if flow.ExpiresAt < now {
		_ = s.sessionRepo.DeleteAuthFlow(state)
		return BrowserLoginResult{}, ErrInvalidState
	}

	cfg, enabled := s.cfg.Get(flow.WorkspaceID)
	if !enabled {
		return BrowserLoginResult{}, ErrSsoNotEnabled
	}
	secrets, err := s.cfg.ResolveSecrets(flow.WorkspaceID)
	if err != nil {
		return BrowserLoginResult{}, err
	}
	redirectURI := redirectURIFromConfig(cfg)

	provider, err := s.providerFactory(cfg.IssuerBaseURL, cfg.ClientID, secrets.ClientSecret)
	if err != nil {
		return BrowserLoginResult{}, err
	}
	sub, err := provider.Exchange(ctx, code, flow.PKCEVerifier, redirectURI)
	if err != nil {
		return BrowserLoginResult{}, fmt.Errorf("%w: %v", ErrIDTokenInvalid, err)
	}

	// sub 映射
	extRepo := storage.NewExternalIDRepository(s.store.DB())
	ext, err := extRepo.GetByProviderAndExternalID("yaoguang", sub)
	if err != nil {
		return BrowserLoginResult{}, ErrIdentityNotFound
	}

	// 校验 membership 存在
	memberRepo := storage.NewMemberRepository(s.store.DB())
	_, err = memberRepo.Get(ext.UserID, flow.WorkspaceID)
	if err != nil {
		return BrowserLoginResult{}, ErrMembershipInactive
	}

	// 建 session
	ttl, _ := time.ParseDuration(defaultIfEmpty(cfg.SessionTTL, "168h"))
	rawSession := randomToken(32)
	csrfToken := randomToken(32)
	sessionHash := hashHex(rawSession)
	csrfHash := hashHex(csrfToken)
	if err := s.sessionRepo.CreateSession(sessionHash, ext.UserID, flow.WorkspaceID, csrfHash, now, now+int64(ttl.Seconds())); err != nil {
		return BrowserLoginResult{}, err
	}
	_ = s.sessionRepo.DeleteAuthFlow(state)
	return BrowserLoginResult{
		RawSession:     rawSession,
		CSRFToken:      csrfToken,
		InsecureCookie: cfg.InsecureCookie,
		SessionMaxAge:  int(ttl.Seconds()),
	}, nil
}

// Logout 删除 session。
func (s *OIDCAuthService) Logout(sessionHash string) error {
	return s.sessionRepo.DeleteSession(sessionHash)
}

// ResolveSession 由 authMiddleware 调用：raw cookie token → sessionHash → 校验未过期 → 返回 session。
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
