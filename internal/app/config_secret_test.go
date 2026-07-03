package app

import (
	"crypto/rand"
	"encoding/base64"
	"os"
	"strings"
	"testing"
)

func withSecretKey(t *testing.T, fn func()) {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand: %v", err)
	}
	old := os.Getenv(configSecretKeyEnv)
	t.Setenv(configSecretKeyEnv, base64.StdEncoding.EncodeToString(key))
	defer os.Setenv(configSecretKeyEnv, old)
	fn()
}

func TestConfigSecretRoundTrip(t *testing.T) {
	withSecretKey(t, func() {
		plain := "my-client-secret-123"
		enc, err := EncryptConfigSecret(plain)
		if err != nil {
			t.Fatalf("encrypt: %v", err)
		}
		if !strings.HasPrefix(enc, "enc:v1:") {
			t.Fatalf("envelope prefix missing: %s", enc)
		}
		if strings.Contains(enc, plain) {
			t.Fatalf("plaintext leaked into ciphertext")
		}
		got, err := DecryptConfigSecret(enc)
		if err != nil {
			t.Fatalf("decrypt: %v", err)
		}
		if got != plain {
			t.Fatalf("got %q, want %q", got, plain)
		}
	})
}

func TestConfigSecretNonceUnique(t *testing.T) {
	withSecretKey(t, func() {
		plain := "same-secret"
		a, _ := EncryptConfigSecret(plain)
		b, _ := EncryptConfigSecret(plain)
		if a == b {
			t.Fatal("same plaintext produced same ciphertext; nonce not random")
		}
		// 两者都应能解回原文
		ga, _ := DecryptConfigSecret(a)
		gb, _ := DecryptConfigSecret(b)
		if ga != plain || gb != plain {
			t.Fatalf("decrypt mismatch: %q %q", ga, gb)
		}
	})
}

func TestConfigSecretKeyMissing(t *testing.T) {
	t.Setenv(configSecretKeyEnv, "")
	_, err := EncryptConfigSecret("x")
	if err == nil {
		t.Fatal("expected error when key missing")
	}
	if !strings.Contains(err.Error(), "config_secret_key_missing") {
		t.Fatalf("err = %v", err)
	}
}

func TestConfigSecretKeyInvalid(t *testing.T) {
	t.Setenv(configSecretKeyEnv, "!!!not-base64!!!")
	_, err := EncryptConfigSecret("x")
	if err == nil {
		t.Fatal("expected error when key invalid")
	}
	if !strings.Contains(err.Error(), "config_secret_key_invalid") {
		t.Fatalf("err = %v", err)
	}
}

func TestConfigSecretDecryptNonEnvelope(t *testing.T) {
	// 非 enc:v1: 前缀的值（历史明文）应返回错误
	withSecretKey(t, func() {
		_, err := DecryptConfigSecret("plain-value")
		if err == nil {
			t.Fatal("expected error for non-envelope value")
		}
	})
}
