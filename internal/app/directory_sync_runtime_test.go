package app

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/auth/directory"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"github.com/go-jose/go-jose/v4"
)

// mockYaoguangIdP 启动一个模拟 yaoguang 后端的 httptest.Server，
// 同时支持 OIDC discovery + JWKS + client_credentials token + directory API。
func mockYaoguangIdP(t *testing.T, clientID, clientSecret, orgID string, members []directory.Member) *httptest.Server {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
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
		case r.URL.Path == "/oidc/orgs/"+orgID+"/jwks.json":
			jwks := jose.JSONWebKeySet{
				Keys: []jose.JSONWebKey{
					{Key: &priv.PublicKey, KeyID: "test-key", Algorithm: string(jose.ES256), Use: "sig"},
				},
			}
			body, _ := json.Marshal(jwks)
			_, _ = w.Write(body)
		case r.URL.Path == "/oidc/orgs/"+orgID+"/token" && r.Method == "POST":
			// 校验 client_credentials（Basic Auth）
			user, pass, ok := r.BasicAuth()
			if !ok || user != clientID || pass != clientSecret {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error":             "invalid_client",
					"error_description": "Client authentication failed",
				})
				return
			}
			_ = r.ParseForm()
			if r.Form.Get("grant_type") != "client_credentials" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "unsupported_grant_type"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":         "ygat_mock_token",
				"tenant_access_token":  "ygat_mock_token",
				"token_type":           "Bearer",
				"expires_in":           3600,
				"scope":                r.Form.Get("scope"),
			})
		case r.URL.Path == "/api/v1/orgs/"+orgID+"/directory/members" && r.Method == "GET":
			auth := r.Header.Get("Authorization")
			if auth != "Bearer ygat_mock_token" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"code": "invalid_token"}})
				return
			}
			memberViews := make([]map[string]any, len(members))
			for i, m := range members {
				exts := make([]map[string]any, len(m.ExternalIdentities))
				for j, e := range m.ExternalIdentities {
					exts[j] = map[string]any{"provider": e.Provider, "user_type": "user_id", "value": e.Value}
				}
				memberViews[i] = map[string]any{
					"id": m.ID, "sub": m.Sub, "display_name": m.DisplayName,
					"role": m.Role, "status": m.Status, "external_identities": exts,
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":   true,
				"data": map[string]any{"members": memberViews, "source": "organization_members", "stale": false},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	return srv
}

// TestDirectorySyncRuntimeFullChain 测试完整链路：
// fetchDirectoryToken（client_credentials）→ directory API → SyncOnce（upsert user/membership）
func TestDirectorySyncRuntimeFullChain(t *testing.T) {
	const clientID = "yga_test_client"
	const clientSecret = "test_secret_123"
	const orgID = "org-abc"

	members := []directory.Member{
		{ID: "m1", Sub: "yaoguang_member:m1", DisplayName: "张三", Role: "owner", Status: "active"},
		{ID: "m2", Sub: "yaoguang_member:m2", DisplayName: "李四", Role: "member", Status: "active",
			ExternalIdentities: []directory.Identity{{Provider: "feishu", Value: "fs_m2"}}},
	}
	srv := mockYaoguangIdP(t, clientID, clientSecret, orgID, members)
	defer srv.Close()

	store := newSyncTestStore(t)
	ws := createTestWorkspace(t, store, "ws1")

	// 写入 SSO 配置
	secretKey := testSecretKey(t)
	cfgSvc := NewOIDCConfigService(storage.NewConfigRepository(store.DB()), secretKey)
	if err := cfgSvc.Set(ws.ID, OIDCConfigInput{
		Provider:      "yaoguang",
		IssuerBaseURL: srv.URL + "/oidc/orgs/" + orgID,
		OrgID:         orgID,
		ClientID:      clientID,
		ClientSecret:  clientSecret,
	}); err != nil {
		t.Fatalf("Set config: %v", err)
	}

	// 用真实 directory client（不 mock DirectoryClient 接口）
	directoryClient := directory.NewClient(srv.Client())
	syncSvc := NewDirectorySyncService(store, directoryClient)
	runtime := NewDirectorySyncRuntime(store, cfgSvc, syncSvc)

	// 手动调用 runOneJob（需要先创建一个 job）
	jobRepo := storage.NewDirectorySyncJobRepository(store.DB())
	_, err := jobRepo.Create(ws.ID, time.Now().Unix())
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	runtime.runOneJob(context.Background(), time.Now().Unix())

	// 验证 job 结果
	jobs, _ := jobRepo.LatestForWorkspace(ws.ID)
	if jobs.Status != "succeeded" {
		t.Fatalf("job status = %s, want succeeded; error: %s", jobs.Status, jobs.ErrorMessage)
	}

	// 验证同步结果：2 个 user + membership
	userRepo := storage.NewUserRepository(store.DB())
	u1, err := userRepo.GetByExternalID("yaoguang", "yaoguang_member:m1")
	if err != nil {
		t.Fatalf("get user m1: %v", err)
	}
	memberRepo := storage.NewMemberRepository(store.DB())
	m, err := memberRepo.Get(u1.ID, ws.ID)
	if err != nil {
		t.Fatalf("get membership: %v", err)
	}
	if m.Role != "owner" {
		t.Fatalf("role = %s, want owner", m.Role)
	}

	u2, _ := userRepo.GetByExternalID("yaoguang", "yaoguang_member:m2")
	if u2.DisplayName != "李四" {
		t.Fatalf("display name = %s", u2.DisplayName)
	}
	// 验证 external identity
	_, err = userRepo.GetByExternalID("feishu", "fs_m2")
	if err != nil {
		t.Fatalf("get user by feishu id: %v", err)
	}
}

// TestDirectorySyncRuntimeWrongSecret 测试 client_secret 错误时 token 获取失败
func TestDirectorySyncRuntimeWrongSecret(t *testing.T) {
	const clientID = "yga_test_client"
	const orgID = "org-abc"

	srv := mockYaoguangIdP(t, clientID, "correct_secret", orgID, nil)
	defer srv.Close()

	store := newSyncTestStore(t)
	ws := createTestWorkspace(t, store, "ws1")

	secretKey := testSecretKey(t)
	cfgSvc := NewOIDCConfigService(storage.NewConfigRepository(store.DB()), secretKey)
	_ = cfgSvc.Set(ws.ID, OIDCConfigInput{
		Provider:      "yaoguang",
		IssuerBaseURL: srv.URL + "/oidc/orgs/" + orgID,
		OrgID:         orgID,
		ClientID:      clientID,
		ClientSecret:  "wrong_secret",
	})

	directoryClient := directory.NewClient(srv.Client())
	syncSvc := NewDirectorySyncService(store, directoryClient)
	runtime := NewDirectorySyncRuntime(store, cfgSvc, syncSvc)

	jobRepo := storage.NewDirectorySyncJobRepository(store.DB())
	_, _ = jobRepo.Create(ws.ID, time.Now().Unix())

	runtime.runOneJob(context.Background(), time.Now().Unix())

	jobs, _ := jobRepo.LatestForWorkspace(ws.ID)
	if jobs.Status != "failed" {
		t.Fatalf("job status = %s, want failed", jobs.Status)
	}
	if jobs.ErrorMessage != "directory_token_fetch_failed" {
		t.Fatalf("error = %s, want directory_token_fetch_failed", jobs.ErrorMessage)
	}
}
