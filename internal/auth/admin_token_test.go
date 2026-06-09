package auth

import (
	"strings"
	"testing"
)

func TestAdminTokenHashAndVerify(t *testing.T) {
	raw := "xuanchu_admin_test_token_123"
	hash := HashAdminToken(raw)
	if !strings.HasPrefix(hash, "sha256:") {
		t.Fatalf("hash = %q, want sha256 prefix", hash)
	}
	if !VerifyAdminToken(raw, hash) {
		t.Fatal("VerifyAdminToken(valid) = false")
	}
	if VerifyAdminToken(raw+"x", hash) {
		t.Fatal("VerifyAdminToken(invalid) = true")
	}
}

func TestGenerateAdminToken(t *testing.T) {
	raw, hash, err := GenerateAdminToken()
	if err != nil {
		t.Fatalf("GenerateAdminToken() error = %v", err)
	}
	if !strings.HasPrefix(raw, "xuanchu_admin_") {
		t.Fatalf("raw token = %q, want xuanchu_admin_ prefix", raw)
	}
	if !VerifyAdminToken(raw, hash) {
		t.Fatal("generated token does not verify")
	}
}

func TestVerifyAdminTokenRejectsUnsupportedHash(t *testing.T) {
	if VerifyAdminToken("token", "md5:bad") {
		t.Fatal("VerifyAdminToken(unsupported) = true")
	}
	if VerifyAdminToken("token", "sha256:not-hex") {
		t.Fatal("VerifyAdminToken(malformed) = true")
	}
}
