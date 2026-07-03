package app

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

const configSecretEnvelopePrefix = "enc:v1:"

// ErrConfigSecretKeyMissing 与 ErrConfigSecretKeyInvalid 是密钥错误码，
// 由调用方映射为 HTTP 错误返回。
var (
	ErrConfigSecretKeyMissing = errors.New("config_secret_key_missing")
	ErrConfigSecretKeyInvalid = errors.New("config_secret_key_invalid")
)

// ParseConfigSecretKey 把 TOML 里 base64 编码的 secret key 解析成 32 字节 AES 密钥。
// 空字符串返回 ErrConfigSecretKeyMissing；非 32 字节返回 ErrConfigSecretKeyInvalid。
func ParseConfigSecretKey(raw string) ([]byte, error) {
	if raw == "" {
		return nil, ErrConfigSecretKeyMissing
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: base64 decode: %v", ErrConfigSecretKeyInvalid, err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("%w: expected 32 bytes, got %d", ErrConfigSecretKeyInvalid, len(key))
	}
	return key, nil
}

// EncryptConfigSecret 用 AES-256-GCM 加密明文，返回 enc:v1:<base64(nonce+ciphertext)> 形式的 envelope。
func EncryptConfigSecret(key []byte, plain string) (string, error) {
	if len(key) != 32 {
		return "", ErrConfigSecretKeyMissing
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("aes new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("gcm new: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("nonce: %w", err)
	}
	sealed := gcm.Seal(nil, nonce, []byte(plain), nil)
	combined := append(nonce, sealed...)
	return configSecretEnvelopePrefix + base64.StdEncoding.EncodeToString(combined), nil
}

// DecryptConfigSecret 还原 EncryptConfigSecret 的 envelope；非 envelope 值返回错误。
func DecryptConfigSecret(key []byte, envelope string) (string, error) {
	if len(envelope) < len(configSecretEnvelopePrefix) || envelope[:len(configSecretEnvelopePrefix)] != configSecretEnvelopePrefix {
		return "", errors.New("not an encrypted envelope")
	}
	raw := envelope[len(configSecretEnvelopePrefix):]
	combined, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return "", fmt.Errorf("envelope base64 decode: %w", err)
	}
	if len(key) != 32 {
		return "", ErrConfigSecretKeyMissing
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("aes new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("gcm new: %w", err)
	}
	if len(combined) < gcm.NonceSize() {
		return "", errors.New("envelope too short")
	}
	nonce := combined[:gcm.NonceSize()]
	ciphertext := combined[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("gcm open: %w", err)
	}
	return string(plain), nil
}
