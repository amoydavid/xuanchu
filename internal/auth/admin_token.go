package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"strings"
)

const AdminTokenPrefix = "xuanchu_admin_"

func GenerateAdminToken() (raw string, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	raw = AdminTokenPrefix + base64.RawURLEncoding.EncodeToString(buf)
	return raw, HashAdminToken(raw), nil
}

func HashAdminToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func VerifyAdminToken(raw, verifier string) bool {
	verifier = strings.TrimSpace(verifier)
	if !strings.HasPrefix(verifier, "sha256:") {
		return false
	}
	wantHex := strings.TrimPrefix(verifier, "sha256:")
	want, err := hex.DecodeString(wantHex)
	if err != nil || len(want) != sha256.Size {
		return false
	}
	sum := sha256.Sum256([]byte(raw))
	return subtle.ConstantTimeCompare(sum[:], want) == 1
}

func ValidAdminTokenHash(verifier string) bool {
	verifier = strings.TrimSpace(verifier)
	if !strings.HasPrefix(verifier, "sha256:") {
		return false
	}
	want, err := hex.DecodeString(strings.TrimPrefix(verifier, "sha256:"))
	return err == nil && len(want) == sha256.Size
}
