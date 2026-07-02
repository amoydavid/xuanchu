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
	if len(prefix) != len("xuanchu_pat_")+tokenPrefixRandomChars || !strings.HasPrefix(raw, prefix) {
		t.Fatalf("prefix = %q raw = %q", prefix, raw)
	}
	if strings.Contains(hash, raw) {
		t.Fatalf("hash contains raw token")
	}
	if !VerifyTokenHash(raw, hash) {
		t.Fatalf("hash did not verify")
	}
	lookupPrefix, err := TokenLookupPrefix(raw)
	if err != nil {
		t.Fatalf("TokenLookupPrefix() error = %v", err)
	}
	if lookupPrefix != prefix {
		t.Fatalf("lookup prefix = %q, want %q", lookupPrefix, prefix)
	}
	if VerifyTokenHash(raw+"x", hash) {
		t.Fatalf("wrong token verified")
	}
}

func TestGenerateTenantAccessToken(t *testing.T) {
	raw, prefix, hash, err := GenerateToken(TokenTypeTenantAccess)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(raw, "xuanchu_tenant_") {
		t.Fatalf("raw prefix = %q", raw)
	}
	if !strings.HasPrefix(prefix, "xuanchu_tenant_") || len(prefix) != len("xuanchu_tenant_")+tokenPrefixRandomChars {
		t.Fatalf("prefix = %q", prefix)
	}
	lookupPrefix, err := TokenLookupPrefix(raw)
	if err != nil {
		t.Fatalf("TokenLookupPrefix() error = %v", err)
	}
	if lookupPrefix != prefix {
		t.Fatalf("lookup prefix = %q, want %q", lookupPrefix, prefix)
	}
	if hash == "" || strings.Contains(hash, raw) {
		t.Fatalf("hash leaks raw token")
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

func TestValidateTenantScopesFiltersWildcards(t *testing.T) {
	scopes, err := ValidateTenantTokenScopes([]string{"*", "*:read", "workspace:*"})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{ScopeImpersonate} {
		if scopes.Has(forbidden) {
			t.Fatalf("tenant scopes include forbidden scope %q: %#v", forbidden, scopes.Values())
		}
	}
	for _, want := range []string{
		ScopeWorkspaceRead, ScopeWorkspaceWrite,
		ScopeTaskRead,
		ScopeTokenRead, ScopeTokenWrite,
		"user:read", "user:write",
		"member:read", "member:write",
		ScopeHookRead, ScopeHookWrite,
		ScopeNotificationRead, ScopeNotificationWrite,
		ScopeReminderRead, ScopeReminderWrite,
	} {
		if !scopes.Has(want) {
			t.Fatalf("tenant scopes missing allowed scope %q: %#v", want, scopes.Values())
		}
	}
}

func TestValidateTenantScopesRejectsForbidden(t *testing.T) {
	for _, value := range []string{"impersonate"} {
		if _, err := ValidateTenantTokenScopes([]string{value}); err == nil {
			t.Fatalf("ValidateTenantTokenScopes(%q) expected error", value)
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
