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
	TokenTypePAT   = "pat"
	TokenTypeAgent = "agent"
)

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

func VerifyTokenHash(raw, hash string) bool {
	sum := sha256.Sum256([]byte(raw))
	want := hex.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(want), []byte(hash)) == 1
}

func ValidateTokenCreate(opts CreateTokenOptions) (ScopeSet, error) {
	if opts.Type == "" {
		opts.Type = TokenTypePAT
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
		delete(scopes, "impersonate")
	}
	return scopes, nil
}

func tokenPrefixForType(tokenType string) (string, error) {
	switch tokenType {
	case TokenTypePAT:
		return "taskg_pat_", nil
	case TokenTypeAgent:
		return "taskg_agent_", nil
	default:
		return "", fmt.Errorf("invalid token type %q", tokenType)
	}
}
