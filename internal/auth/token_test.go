package auth

import (
	"strings"
	"testing"
)

func TestGenerateRawTokenAndHash(t *testing.T) {
	raw, prefix, hash, err := GenerateToken(TokenTypePAT)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(raw, "xuanchu_pat_") {
		t.Fatalf("raw = %q", raw)
	}
	if len(prefix) < 12 || !strings.HasPrefix(raw, prefix) {
		t.Fatalf("prefix = %q raw = %q", prefix, raw)
	}
	if strings.Contains(hash, raw) {
		t.Fatalf("hash contains raw token")
	}
	if !VerifyTokenHash(raw, hash) {
		t.Fatalf("hash did not verify")
	}
	if VerifyTokenHash(raw+"x", hash) {
		t.Fatalf("wrong token verified")
	}
}

func TestParseScopesFailClosed(t *testing.T) {
	scopes, err := ParseScopes(nil)
	if err != nil {
		t.Fatal(err)
	}
	if scopes.Has("task:read") {
		t.Fatalf("empty scope should not grant capability")
	}
	scopes, err = ParseScopes([]string{"task:read,task:write", "project:read"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"task:read", "task:write", "project:read"} {
		if !scopes.Has(want) {
			t.Fatalf("missing scope %s", want)
		}
	}
}

func TestParseScopesRejectsInvalidAndWildcardScopes(t *testing.T) {
	for _, raw := range [][]string{
		{"admin:*"},
		{"task:read", "bogus"},
	} {
		if _, err := ParseScopes(raw); err == nil {
			t.Fatalf("ParseScopes(%v) error = nil", raw)
		}
	}
}

func TestAgentTokenRequiresExplicitWorkspaceAndScope(t *testing.T) {
	_, err := ValidateTokenCreate(CreateTokenOptions{Type: TokenTypeAgent, Scopes: []string{"task:read"}})
	if err == nil || !strings.Contains(err.Error(), "workspace") {
		t.Fatalf("expected workspace error, got %v", err)
	}
	_, err = ValidateTokenCreate(CreateTokenOptions{Type: TokenTypeAgent, WorkspaceIDs: []string{"w1"}})
	if err == nil || !strings.Contains(err.Error(), "scope") {
		t.Fatalf("expected scope error, got %v", err)
	}
}

func TestGenerateActingToken(t *testing.T) {
	raw, prefix, hash, err := GenerateActingToken()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(raw, ActingTokenPrefix) {
		t.Fatalf("raw prefix = %q", raw)
	}
	// display prefix 截断到 16 字符，且必须是 raw 的前缀。
	if !strings.HasPrefix(prefix, ActingTokenPrefix) || len(prefix) > 16 {
		t.Fatalf("display prefix = %q", prefix)
	}
	if !strings.HasPrefix(raw, prefix) {
		t.Fatalf("prefix %q is not a prefix of raw %q", prefix, raw)
	}
	// hash 形态与 admin token verifier 一致。
	if !strings.HasPrefix(hash, "sha256:") {
		t.Fatalf("hash = %q", hash)
	}
	if strings.Contains(hash, raw) {
		t.Fatalf("hash leaked raw token")
	}
	if !VerifyActingToken(raw, hash) {
		t.Fatal("VerifyActingToken() = false")
	}
	if VerifyActingToken(raw+"x", hash) {
		t.Fatal("VerifyActingToken(invalid) = true")
	}
	// 非 sha256 前缀的 verifier 应拒绝。
	if VerifyActingToken(raw, "not-a-verifier") {
		t.Fatal("VerifyActingToken(invalid verifier) = true")
	}
}
