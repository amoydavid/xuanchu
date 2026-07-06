package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

// seedWebLoginDisabledToken 直接在 store 里插入一个带 WebLoginDisabled 标记的 PAT，
// 返回 raw token（可用于 Bearer 鉴权）。绕过 app service，专注测 HTTP 拒绝逻辑。
func seedWebLoginDisabledToken(t *testing.T, store *storage.Store, userID string, disabled bool) string {
	t.Helper()
	raw, prefix, hash, err := auth.GenerateToken(auth.TokenTypePAT)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	entry := storage.ApiTokenEntry{
		ID:               prefix + "-id",
		UserID:           &userID,
		Name:             "sso-pat",
		Type:             auth.TokenTypePAT,
		TokenPrefix:      prefix,
		TokenHash:        hash,
		ScopesJSON:       `["task:read"]`,
		WorkspaceIDsJSON: `[]`,
		ProjectIDsJSON:   `[]`,
		WebLoginDisabled: disabled,
		CreatedAt:        100,
	}
	if err := storage.NewTokenRepository(store.DB()).Create(entry); err != nil {
		t.Fatalf("Create token: %v", err)
	}
	return raw
}

func TestCredentialsCurrentRejectsWebLoginDisabledToken(t *testing.T) {
	store := openHTTPTestStore(t)
	user, err := storage.NewUserRepository(store.DB()).GetByName("local")
	if err != nil {
		t.Fatal(err)
	}
	raw := seedWebLoginDisabledToken(t, store, user.ID, true)
	server := NewServer(Options{Store: store})

	rr := requestHTTP(t, server, http.MethodGet, "/api/v1/credentials/current", map[string]string{
		"Authorization": "Bearer " + raw,
	})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "token_web_login_disabled") {
		t.Fatalf("body missing code token_web_login_disabled: %s", rr.Body.String())
	}
}

func TestCredentialsCurrentAllowsNormalToken(t *testing.T) {
	store := openHTTPTestStore(t)
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	user, err := storage.NewUserRepository(store.DB()).GetByName("local")
	if err != nil {
		t.Fatal(err)
	}
	// 未打标 PAT 仍可登录 Console
	raw := seedWebLoginDisabledToken(t, store, user.ID, false)
	server := NewServer(Options{Store: store})

	rr := requestHTTP(t, server, http.MethodGet, "/api/v1/credentials/current", map[string]string{
		"Authorization": "Bearer " + raw,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rr.Code, rr.Body.String())
	}
	// 确保 WebLoginDisabled 不影响其他 API（task:read 仍可用）
	rr = requestHTTP(t, server, http.MethodGet, "/api/v1/tasks", map[string]string{
		"Authorization": "Bearer " + raw,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("tasks API status = %d, want 200 (WebLoginDisabled 不应影响 API), body=%s", rr.Code, rr.Body.String())
	}
	_ = ws
}

// TestWebLoginDisabledTokenStillWorksForAPI 验证打标 token 仅被 Console 登录拒绝，
// HTTP API 调用仍正常工作（这是 spec 的核心边界）。
func TestWebLoginDisabledTokenStillWorksForAPI(t *testing.T) {
	store := openHTTPTestStore(t)
	user, err := storage.NewUserRepository(store.DB()).GetByName("local")
	if err != nil {
		t.Fatal(err)
	}
	raw := seedWebLoginDisabledToken(t, store, user.ID, true)
	server := NewServer(Options{Store: store})

	rr := requestHTTP(t, server, http.MethodGet, "/api/v1/tasks", map[string]string{
		"Authorization": "Bearer " + raw,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("tasks API status = %d, want 200, body=%s", rr.Code, rr.Body.String())
	}
}
