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
