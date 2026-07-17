package httpapi

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

// TestSsoSyncHandlerEndToEnd 测试 handleWorkspaceSsoSync 的完整链路：
// mock yaoguang IdP → discovery → client_credentials → directory API → 同步成功
func TestSsoSyncHandlerEndToEnd(t *testing.T) {
	// 生成 secret key
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	secretKeyB64 := base64.StdEncoding.EncodeToString(key)

	store := openHTTPTestStore(t)

	// 创建 server（注入 secret key）
	srv := NewServer(Options{
		Store:           store,
		ConfigSecretKey: secretKeyB64,
	})

	// 创建 PAT（owner）
	appSvc, err := app.NewService(app.ServiceOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	created, err := appSvc.CreateToken(app.CreateTokenInput{
		Name:          "sso-sync-test",
		WorkspaceRefs: []string{"local"},
		Scopes:        []string{"workspace:write"},
	})
	if err != nil {
		t.Fatal(err)
	}
	token := created.RawToken

	// Mock yaoguang IdP
	const clientID = "yga_test"
	const clientSecret = "secret123"
	const orgID = "org-test"
	mockSrv := newMockYaoguangForSync(t, clientID, clientSecret, orgID)
	defer mockSrv.Close()

	// 写入 SSO 配置（直接通过 service）
	ws, _ := store.LocalWorkspace()
	cfgSvc := app.NewOIDCConfigService(storage.NewConfigRepository(store.DB()), key)
	if err := cfgSvc.Set(ws.ID, app.OIDCConfigInput{
		Provider:      "yaoguang",
		IssuerBaseURL: mockSrv.URL + "/oidc/orgs/" + orgID,
		OrgID:         orgID,
		ClientID:      clientID,
		ClientSecret:  clientSecret,
	}); err != nil {
		t.Fatalf("set sso config: %v", err)
	}

	// POST /api/v1/workspaces/local/sso/sync
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/local/sso/sync", bytes.NewBufferString(`{}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data struct {
			Added   int `json:"added"`
			Removed int `json:"removed"`
			Updated int `json:"updated"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rr.Body.String())
	}
	if resp.Data.Added != 1 {
		t.Fatalf("added = %d, want 1", resp.Data.Added)
	}
	t.Logf("sync result: +Added:%d -Removed:%d ~Updated:%d", resp.Data.Added, resp.Data.Removed, resp.Data.Updated)
}

// newMockYaoguangForSync 启动一个支持 discovery + client_credentials + directory API 的 mock server。
func newMockYaoguangForSync(t *testing.T, clientID, clientSecret, orgID string) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/oidc/orgs/"+orgID+"/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                 srv.URL + "/oidc/orgs/" + orgID,
				"authorization_endpoint": srv.URL + "/oidc/orgs/" + orgID + "/authorize",
				"token_endpoint":         srv.URL + "/oidc/orgs/" + orgID + "/token",
				"jwks_uri":               srv.URL + "/oidc/orgs/" + orgID + "/jwks.json",
				"grant_types_supported":  []string{"authorization_code", "refresh_token", "client_credentials"},
			})
		case r.URL.Path == "/oidc/orgs/"+orgID+"/token" && r.Method == "POST":
			user, pass, ok := r.BasicAuth()
			if !ok || user != clientID || pass != clientSecret {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid_client"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":        "ygat_mock",
				"tenant_access_token": "ygat_mock",
				"token_type":          "Bearer",
				"expires_in":          3600,
			})
		case r.URL.Path == "/api/v1/orgs/"+orgID+"/directory/members" && r.Method == "GET":
			if r.Header.Get("Authorization") != "Bearer ygat_mock" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"code": "invalid_token"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{
					"members": []map[string]any{
						{"id": "m1", "sub": "yaoguang_member:m1", "display_name": "Test", "role": "member", "status": "active"},
					},
					"source": "organization_members",
					"stale":  false,
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	return srv
}
