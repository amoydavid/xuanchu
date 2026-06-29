package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

const (
	TokenTypePAT          = "pat"
	TokenTypeAgent        = "agent"
	TokenTypeTenantAccess = "tenant_access_token"
	TokenTypeAdminActing  = "admin_acting"
)

// ActingTokenPrefix 是 server admin 委托签发的短期 acting token 前缀。
// acting token 只面向浏览器 Workspace Console 的普通 HTTP API，
// 不进入普通 api_tokens 表，也不被 MCP 或 remote CLI 接受。
const ActingTokenPrefix = "xuanchu_act_"

type CreateTokenOptions struct {
	Type         string
	Scopes       []string
	WorkspaceIDs []string
}

func GenerateToken(tokenType string) (raw string, prefix string, hash string, err error) {
	prefixBase, err := tokenPrefixForType(tokenType)
	if err != nil {
		return "", "", "", err
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", "", fmt.Errorf("generate token entropy: %w", err)
	}
	raw = prefixBase + base64.RawURLEncoding.EncodeToString(buf)
	prefix = raw
	if len(prefix) > 16 {
		prefix = raw[:16]
	}
	sum := sha256.Sum256([]byte(raw))
	hash = hex.EncodeToString(sum[:])
	return raw, prefix, hash, nil
}

// GenerateActingToken 生成 server admin 委托的短期 acting token。
// 与 GenerateToken 的区别：前缀固定为 ActingTokenPrefix，
// hash 形态为 "sha256:<hex>"（与 admin token verifier 一致），
// 不落入普通 api_tokens，只写入 admin_acting_sessions。
func GenerateActingToken() (raw string, prefix string, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", "", fmt.Errorf("generate acting token entropy: %w", err)
	}
	raw = ActingTokenPrefix + base64.RawURLEncoding.EncodeToString(buf)
	prefix = raw
	if len(prefix) > 16 {
		prefix = raw[:16]
	}
	sum := sha256.Sum256([]byte(raw))
	return raw, prefix, "sha256:" + hex.EncodeToString(sum[:]), nil
}

// VerifyActingToken 用 sha256 verifier 校验 acting token 原文。
// 复用 admin token 的 verifier 语义，保持一致的比较方式。
func VerifyActingToken(raw, verifier string) bool {
	return VerifyAdminToken(raw, verifier)
}

func VerifyTokenHash(raw, hash string) bool {
	sum := sha256.Sum256([]byte(raw))
	want := hex.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(want), []byte(hash)) == 1
}

func ValidateTokenCreate(opts CreateTokenOptions) (ScopeSet, error) {
	if opts.Type == "" {
		opts.Type = TokenTypePAT
	}
	if opts.Type == TokenTypeTenantAccess {
		return nil, fmt.Errorf("tenant token must use tenant scope validation")
	}
	if _, err := tokenPrefixForType(opts.Type); err != nil {
		return nil, err
	}
	if opts.Type == TokenTypeAgent {
		if len(opts.WorkspaceIDs) == 0 {
			return nil, fmt.Errorf("agent token requires at least one workspace")
		}
		if len(opts.Scopes) == 0 {
			return nil, fmt.Errorf("agent token requires explicit scopes")
		}
	}
	scopes, err := ParseScopes(opts.Scopes)
	if err != nil {
		return nil, err
	}
	if opts.Type == TokenTypePAT {
		delete(scopes, ScopeImpersonate)
	}
	return scopes, nil
}

func tokenPrefixForType(tokenType string) (string, error) {
	switch tokenType {
	case TokenTypePAT:
		return "xuanchu_pat_", nil
	case TokenTypeAgent:
		return "xuanchu_agent_", nil
	case TokenTypeTenantAccess:
		return "xuanchu_tenant_", nil
	default:
		return "", fmt.Errorf("invalid token type %q", tokenType)
	}
}
