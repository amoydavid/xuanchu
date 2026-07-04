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

	"github.com/go-jose/go-jose/v4"
)

// setupMockIdP 启动一个最小 OIDC IdP（discovery + token + JWKS），用 ECDSA P-256 签发 id_token。
func setupMockIdP(t *testing.T, clientID string) (*httptest.Server, string) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                 srv.URL,
				"authorization_endpoint": srv.URL + "/authorize",
				"token_endpoint":         srv.URL + "/token",
				"jwks_uri":               srv.URL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"ES256"},
			})
		case "/jwks":
			jwks := jose.JSONWebKeySet{
				Keys: []jose.JSONWebKey{
					{Key: &priv.PublicKey, KeyID: "test-key", Algorithm: string(jose.ES256), Use: "sig"},
				},
			}
			body, _ := json.Marshal(jwks)
			_, _ = w.Write(body)
		case "/token":
			idToken := mintTestIDToken(t, priv, srv.URL, clientID, "yaoguang_member:m1", "test-key")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "fake",
				"token_type":   "Bearer",
				"id_token":     idToken,
				"expires_in":   3600,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	return srv, clientID
}

func mintTestIDToken(t *testing.T, priv *ecdsa.PrivateKey, issuer, clientID, sub, keyID string) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.ES256, Key: priv},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", keyID))
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	claims := map[string]any{
		"iss": issuer,
		"sub": sub,
		"aud": clientID,
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	raw, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	obj, err := signer.Sign(raw)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	token, err := obj.CompactSerialize()
	if err != nil {
		t.Fatalf("compact serialize: %v", err)
	}
	return token
}

func TestExchangeReturnsSub(t *testing.T) {
	srv, _ := setupMockIdP(t, "client1")
	defer srv.Close()

	p, err := NewProviderSafe(context.Background(), srv.URL, "client1", "secret1")
	if err != nil {
		t.Fatalf("NewProviderSafe: %v", err)
	}
	url := p.AuthCodeURL("state123", "verifier123", srv.URL+"/sso/oidc/callback")
	if url == "" {
		t.Fatal("empty auth url")
	}

	token, err := p.Exchange(context.Background(), "fakecode", "verifier123", srv.URL+"/sso/oidc/callback")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if token.Subject != "yaoguang_member:m1" {
		t.Fatalf("sub = %s, want yaoguang_member:m1", token.Subject)
	}
}

func TestExchangeAudMismatchFails(t *testing.T) {
	srv, _ := setupMockIdP(t, "client1")
	defer srv.Close()

	// Provider 期望 WRONG_CLIENT，但 mock IdP 签发的 aud 是 client1
	p, err := NewProviderSafe(context.Background(), srv.URL, "WRONG_CLIENT", "secret1")
	if err != nil {
		t.Fatalf("NewProviderSafe: %v", err)
	}
	_, err = p.Exchange(context.Background(), "fakecode", "verifier123", srv.URL+"/sso/oidc/callback")
	if err == nil {
		t.Fatal("expected aud mismatch error")
	}
}

func TestNoProxyHTTPClientSkipsLocalhost(t *testing.T) {
	// NoProxyHTTPClient 的 Transport 对 localhost 应返回 nil proxy
	req, _ := http.NewRequest("GET", "http://localhost:5174/test", nil)
	transport, ok := NoProxyHTTPClient.Transport.(*http.Transport)
	if !ok {
		t.Fatal("transport is not *http.Transport")
	}
	proxy, err := transport.Proxy(req)
	if err != nil {
		t.Fatalf("Proxy error: %v", err)
	}
	if proxy != nil {
		t.Fatalf("expected nil proxy for localhost, got %v", proxy)
	}
	// 非 localhost 应走 ProxyFromEnvironment
	req2, _ := http.NewRequest("GET", "http://example.com/test", nil)
	_, _ = transport.Proxy(req2) // 不报错即可
}

func TestClientCredentialsTokenWithMockIdP(t *testing.T) {
	// 用一个支持 client_credentials 的 mock IdP 测试 token 获取
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/.well-known/openid-configuration" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                 srv.URL,
				"token_endpoint":         srv.URL + "/token",
				"grant_types_supported":  []string{"client_credentials"},
			})
			return
		}
		if r.URL.Path == "/token" && r.Method == "POST" {
			user, pass, ok := r.BasicAuth()
			if !ok || user != "test_client" || pass != "test_secret" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid_client"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "mock_access_token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	p, err := NewProviderSafe(context.Background(), srv.URL, "test_client", "test_secret")
	if err != nil {
		t.Fatalf("NewProviderSafe: %v", err)
	}

	token, err := p.ClientCredentialsToken(context.Background(), "org.members.read")
	if err != nil {
		t.Fatalf("ClientCredentialsToken: %v", err)
	}
	if token != "mock_access_token" {
		t.Fatalf("token = %s, want mock_access_token", token)
	}
}
