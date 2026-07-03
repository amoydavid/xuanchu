// Package oidc 是 xuanchu 作为 OIDC Relying Party 的纯逻辑层，
// 封装 discovery、Auth Code Flow + PKCE、id_token 校验。不依赖 GORM/HTTP server。
package oidc

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
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

// NewProviderSafe 用 issuerBaseURL 做 discovery，构造 RP。
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

// pkceChallengeS256 按 RFC 7636 计算 S256 challenge：BASE64URL(SHA256(verifier))。
func pkceChallengeS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
