package app

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
)

func testSecretKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return key
}

func TestConfigSecretRoundTrip(t *testing.T) {
	key := testSecretKey(t)
	plain := "my-client-secret-123"
	enc, err := EncryptConfigSecret(key, plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if !strings.HasPrefix(enc, "enc:v1:") {
		t.Fatalf("envelope prefix missing: %s", enc)
	}
	if strings.Contains(enc, plain) {
		t.Fatalf("plaintext leaked into ciphertext")
	}
	got, err := DecryptConfigSecret(key, enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got != plain {
		t.Fatalf("got %q, want %q", got, plain)
	}
}

func TestConfigSecretNonceUnique(t *testing.T) {
	key := testSecretKey(t)
	plain := "same-secret"
	a, _ := EncryptConfigSecret(key, plain)
	b, _ := EncryptConfigSecret(key, plain)
	if a == b {
		t.Fatal("same plaintext produced same ciphertext; nonce not random")
	}
	ga, _ := DecryptConfigSecret(key, a)
	gb, _ := DecryptConfigSecret(key, b)
	if ga != plain || gb != plain {
		t.Fatalf("decrypt mismatch: %q %q", ga, gb)
	}
}

func TestConfigSecretKeyMissing(t *testing.T) {
	_, err := EncryptConfigSecret(nil, "x")
	if err == nil {
		t.Fatal("expected error when key missing")
	}
}

func TestConfigSecretKeyInvalid(t *testing.T) {
	// ParseConfigSecretKey 对非 base64 报错
	_, err := ParseConfigSecretKey("!!!not-base64!!!")
	if err == nil {
		t.Fatal("expected error when key invalid")
	}
}

func TestConfigSecretDecryptNonEnvelope(t *testing.T) {
	key := testSecretKey(t)
	_, err := DecryptConfigSecret(key, "plain-value")
	if err == nil {
		t.Fatal("expected error for non-envelope value")
	}
}

func TestParseConfigSecretKey(t *testing.T) {
	// 空 → missing
	if _, err := ParseConfigSecretKey(""); err == nil {
		t.Fatal("expected missing for empty key")
	}
	// 正确 base64 32 字节
	raw := base64.StdEncoding.EncodeToString(make([]byte, 32))
	key, err := ParseConfigSecretKey(raw)
	if err != nil {
		t.Fatalf("parse valid key: %v", err)
	}
	if len(key) != 32 {
		t.Fatalf("key len = %d", len(key))
	}
}
