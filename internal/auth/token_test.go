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
	if !strings.HasPrefix(raw, "taskg_pat_") {
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
	err := ValidateTokenCreate(CreateTokenOptions{Type: TokenTypeAgent, Scopes: []string{"task:read"}})
	if err == nil || !strings.Contains(err.Error(), "workspace") {
		t.Fatalf("expected workspace error, got %v", err)
	}
	err = ValidateTokenCreate(CreateTokenOptions{Type: TokenTypeAgent, WorkspaceIDs: []string{"w1"}})
	if err == nil || !strings.Contains(err.Error(), "scope") {
		t.Fatalf("expected scope error, got %v", err)
	}
}
