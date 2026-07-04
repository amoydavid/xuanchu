// Package oidc 是 xuanchu 作为 OIDC Relying Party 的纯逻辑层，
// 封装 discovery、Auth Code Flow + PKCE、id_token 校验。不依赖 GORM/HTTP server。
package oidc

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// NoProxyHTTPClient 是一个对 localhost/127.0.0.1 不走代理的 HTTP client。
// 开发环境下 http_proxy 会把 localhost 请求发到代理导致 502；
// OIDC/directory 是 server-to-server 通信，localhost 地址应直连。
var NoProxyHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			host := req.URL.Hostname()
			if host == "localhost" || host == "127.0.0.1" || host == "::1" {
				return nil, nil
			}
			return http.ProxyFromEnvironment(req)
		},
	},
}

// contextWithNoProxyClient 返回注入了 NoProxyHTTPClient 的 context。
// go-oidc 用 oidc.ClientContext 读 client，oauth2 用 oauth2.HTTPClient 读 client，
// 两个 key 都注入以覆盖所有调用路径。
func contextWithNoProxyClient(ctx context.Context) context.Context {
	ctx = oidc.ClientContext(ctx, NoProxyHTTPClient)
	ctx = context.WithValue(ctx, oauth2.HTTPClient, NoProxyHTTPClient)
	return ctx
}

// TokenEndpointURL 返回 OIDC discovery 拿到的 token endpoint URL（即 {base}/oidc/orgs/{org}/token）。
func (p *Provider) TokenEndpointURL() string {
	return p.oauthConfig.Endpoint.TokenURL
}

// ClientCredentialsToken 用 client_credentials grant 换取 access token（用于访问 yaoguang 通讯录 API）。
// yaoguang (Fosite) 要求 client_secret_basic 认证（HTTP Basic Auth），需显式设 AuthStyleInHeader。
// 使用 NoProxyHTTPClient 避免 localhost 请求走代理。
func (p *Provider) ClientCredentialsToken(ctx context.Context, scopes ...string) (string, error) {
	cfg := clientcredentials.Config{
		ClientID:     p.oauthConfig.ClientID,
		ClientSecret: p.oauthConfig.ClientSecret,
		TokenURL:     p.oauthConfig.Endpoint.TokenURL,
		Scopes:       scopes,
		AuthStyle:    oauth2.AuthStyleInHeader,
	}
	token, err := cfg.Token(contextWithNoProxyClient(ctx))
	if err != nil {
		return "", fmt.Errorf("client_credentials token: %w", err)
	}
	return token.AccessToken, nil
}

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

// NewProviderSafe 用 issuerBaseURL 做 discovery，构造 RP。
// 使用 NoProxyHTTPClient 避免 localhost discovery 请求走代理。
func NewProviderSafe(ctx context.Context, issuerBaseURL, clientID, clientSecret string) (*Provider, error) {
	provider, err := oidc.NewProvider(contextWithNoProxyClient(ctx), issuerBaseURL)
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

// AuthCodeURL 生成跳转 IdP 的授权 URL（含 state + PKCE S256）。
func (p *Provider) AuthCodeURL(state, pkceVerifier, redirectURI string) string {
	p.oauthConfig.RedirectURL = redirectURI
	return p.oauthConfig.AuthCodeURL(state,
		oauth2.SetAuthURLParam("code_challenge", pkceChallengeS256(pkceVerifier)),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
}

// Exchange 用 authorization code 换 id_token 并校验签名/iss/aud/exp。
func (p *Provider) Exchange(ctx context.Context, code, pkceVerifier, redirectURI string) (*Token, error) {
	p.oauthConfig.RedirectURL = redirectURI
	token, err := p.oauthConfig.Exchange(contextWithNoProxyClient(ctx), code,
		oauth2.SetAuthURLParam("code_verifier", pkceVerifier),
	)
	if err != nil {
		return nil, fmt.Errorf("oauth exchange: %w", err)
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return nil, fmt.Errorf("id_token missing in token response")
	}
	idToken, err := p.verifier.Verify(contextWithNoProxyClient(ctx), rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("id_token verify: %w", err)
	}
	return &Token{Subject: idToken.Subject, Raw: rawIDToken}, nil
}

// pkceChallengeS256 按 RFC 7636 计算 S256 challenge：BASE64URL(SHA256(verifier))。
func pkceChallengeS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
