package app

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// configSecretKeyEnv 是读取 config secret envelope 密钥的环境变量。
// 密钥格式固定为 32 字节随机值的 base64（标准编码）。
const configSecretKeyEnv = "XUANCHU_CONFIG_SECRET_KEY"

const configSecretEnvelopePrefix = "enc:v1:"

// configSecretKeyMissing 与 configSecretKeyInvalid 是密钥错误码，
// 由调用方映射为 HTTP 错误返回。
var (
	errConfigSecretKeyMissing = errors.New("config_secret_key_missing")
	errConfigSecretKeyInvalid = errors.New("config_secret_key_invalid")
)

// loadConfigSecretKey 从 XUANCHU_CONFIG_SECRET_KEY 读取并解码 32 字节 AES 密钥。
func loadConfigSecretKey() ([]byte, error) {
	raw := os.Getenv(configSecretKeyEnv)
	if raw == "" {
		return nil, errConfigSecretKeyMissing
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: base64 decode: %v", errConfigSecretKeyInvalid, err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("%w: expected 32 bytes, got %d", errConfigSecretKeyInvalid, len(key))
	}
	return key, nil
}

// EncryptConfigSecret 用 AES-256-GCM 加密明文，返回 enc:v1:<base64(nonce+ciphertext)> 形式的 envelope。
func EncryptConfigSecret(plain string) (string, error) {
	key, err := loadConfigSecretKey()
	if err != nil {
		return "", err
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
func DecryptConfigSecret(envelope string) (string, error) {
	if !strings.HasPrefix(envelope, configSecretEnvelopePrefix) {
		return "", errors.New("not an encrypted envelope")
	}
	raw := strings.TrimPrefix(envelope, configSecretEnvelopePrefix)
	combined, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return "", fmt.Errorf("envelope base64 decode: %w", err)
	}
	key, err := loadConfigSecretKey()
	if err != nil {
		return "", err
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
