package app

import (
	"fmt"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/storage"
	"gorm.io/gorm"
)

const (
	ssoKeyProvider        = "sso.provider"
	ssoKeyIssuerBaseURL   = "sso.issuer_base_url"
	ssoKeyOrgID           = "sso.org_id"
	ssoKeyClientID        = "sso.client_id"
	ssoKeyClientSecret    = "sso.client_secret"
	ssoKeyScopes          = "sso.scopes"
	ssoKeyRedirectPath    = "sso.redirect_path"
	ssoKeyExternalBaseURL = "sso.external_base_url"
	ssoKeySessionTTL      = "sso.session_ttl"
	ssoKeySyncInterval    = "sso.sync_interval"
	ssoKeyInsecureCookie  = "sso.insecure_cookie"
)

// OIDCConfig 是脱敏后的配置读视图（给 HTTP/前端展示用）。
type OIDCConfig struct {
	Provider           string
	IssuerBaseURL      string
	OrgID              string
	ClientID           string
	ClientSecretMasked string
	Scopes             string
	RedirectPath       string
	ExternalBaseURL    string
	SessionTTL         string
	SyncInterval       string
	InsecureCookie     bool
}

// OIDCConfigInput 是写入请求（secret 留空表示不改）。
type OIDCConfigInput struct {
	Provider        string
	IssuerBaseURL   string
	OrgID           string
	ClientID        string
	ClientSecret    string
	Scopes          string
	RedirectPath    string
	ExternalBaseURL string
	SessionTTL      string
	SyncInterval    string
	InsecureCookie  bool
}

// OIDCConfigSecrets 是非脱敏的 secret 明文（仅 app 内部短暂使用）。
type OIDCConfigSecrets struct {
	ClientSecret string
}

type OIDCConfigService struct {
	cfg       *storage.ConfigRepository
	db        *gorm.DB
	secretKey []byte // AES-256 key for envelope encryption; 可能为空（缺密钥时加密会报错）
}

func NewOIDCConfigService(cfg *storage.ConfigRepository, secretKey []byte) *OIDCConfigService {
	return &OIDCConfigService{cfg: cfg, db: cfg.DB(), secretKey: secretKey}
}

func (s *OIDCConfigService) wk(workspaceID, key string) storage.ConfigKey {
	return storage.ConfigKey{WorkspaceID: workspaceID, Scope: storage.ConfigScopeWorkspace, Key: key}
}

func (s *OIDCConfigService) Get(workspaceID string) (OIDCConfig, bool) {
	g := func(key string) string {
		v, ok, _ := s.cfg.Get(s.wk(workspaceID, key))
		if !ok {
			return ""
		}
		return v
	}
	secretMask := func(key string) string {
		v := g(key)
		if v == "" {
			return ""
		}
		plain, err := DecryptConfigSecret(s.secretKey, v)
		if err != nil {
			return ""
		}
		return mask(plain)
	}
	enabled := g(ssoKeyProvider) != "" && g(ssoKeyIssuerBaseURL) != ""
	return OIDCConfig{
		Provider:           g(ssoKeyProvider),
		IssuerBaseURL:      g(ssoKeyIssuerBaseURL),
		OrgID:              g(ssoKeyOrgID),
		ClientID:           g(ssoKeyClientID),
		ClientSecretMasked: secretMask(ssoKeyClientSecret),
		Scopes:             g(ssoKeyScopes),
		RedirectPath:       g(ssoKeyRedirectPath),
		ExternalBaseURL:    g(ssoKeyExternalBaseURL),
		SessionTTL:         g(ssoKeySessionTTL),
		SyncInterval:       g(ssoKeySyncInterval),
		InsecureCookie:     g(ssoKeyInsecureCookie) == "true",
	}, enabled
}

// ResolveSecrets 读非脱敏 client_secret（登录/同步时用）。
func (s *OIDCConfigService) ResolveSecrets(workspaceID string) (OIDCConfigSecrets, error) {
	cs, ok, err := s.cfg.Get(s.wk(workspaceID, ssoKeyClientSecret))
	if err != nil {
		return OIDCConfigSecrets{}, err
	}
	if !ok {
		return OIDCConfigSecrets{}, fmt.Errorf("sso secrets not configured")
	}
	csPlain, err := DecryptConfigSecret(s.secretKey, cs)
	if err != nil {
		return OIDCConfigSecrets{}, err
	}
	return OIDCConfigSecrets{ClientSecret: csPlain}, nil
}

func (s *OIDCConfigService) Set(workspaceID string, in OIDCConfigInput) error {
	// secret 加密在事务前完成（避免事务内做重计算 + 可能报错导致半提交）
	var encClientSecret string
	if in.ClientSecret != "" {
		enc, err := EncryptConfigSecret(s.secretKey, in.ClientSecret)
		if err != nil {
			return err
		}
		encClientSecret = enc
	}

	// 所有 key 在同一事务内写入，保证配置一致性（全成功或全回滚）
	return s.db.Transaction(func(tx *gorm.DB) error {
		txRepo := storage.NewConfigRepository(tx)
		set := func(key, value string) error {
			return txRepo.Set(storage.ConfigKey{WorkspaceID: workspaceID, Scope: storage.ConfigScopeWorkspace, Key: key}, value)
		}
		if err := set(ssoKeyProvider, defaultIfEmpty(in.Provider, "yaoguang")); err != nil {
			return err
		}
		if err := set(ssoKeyIssuerBaseURL, in.IssuerBaseURL); err != nil {
			return err
		}
		if err := set(ssoKeyOrgID, in.OrgID); err != nil {
			return err
		}
		if err := set(ssoKeyClientID, in.ClientID); err != nil {
			return err
		}
		// secret 留空 → 不覆盖（保留原值）
		if encClientSecret != "" {
			if err := set(ssoKeyClientSecret, encClientSecret); err != nil {
				return err
			}
		}
		if err := set(ssoKeyScopes, in.Scopes); err != nil {
			return err
		}
		if err := set(ssoKeyRedirectPath, defaultIfEmpty(in.RedirectPath, "/sso/oidc/callback")); err != nil {
			return err
		}
		if err := set(ssoKeyExternalBaseURL, in.ExternalBaseURL); err != nil {
			return err
		}
		if err := set(ssoKeySessionTTL, defaultIfEmpty(in.SessionTTL, "168h")); err != nil {
			return err
		}
		if err := set(ssoKeySyncInterval, defaultIfEmpty(in.SyncInterval, "1h")); err != nil {
			return err
		}
		if err := set(ssoKeyInsecureCookie, boolStr(in.InsecureCookie)); err != nil {
			return err
		}
		return nil
	})
}

// mask 取前后 2 位，中间用 • 替换；短于 6 位则全 •。
func mask(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 5 {
		return strings.Repeat("•", len(s))
	}
	return s[:2] + "••" + s[len(s)-2:]
}

func defaultIfEmpty(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
