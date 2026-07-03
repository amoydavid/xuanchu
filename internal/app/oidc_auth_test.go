package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

func newAuthTestStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func hashStr(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// stubOIDCProvider 是测试用的 OIDCProvider 接口实现，Exchange 直接返回固定 sub。
type stubOIDCProvider struct {
	sub string
}

func (s *stubOIDCProvider) AuthCodeURL(state, verifier, redirectURI string) string {
	return "http://fake/auth?state=" + state
}
func (s *stubOIDCProvider) Exchange(ctx context.Context, code, verifier, redirectURI string) (string, error) {
	if s.sub == "" {
		return "yaoguang_member:m1", nil
	}
	return s.sub, nil
}

func listAuthFlows(store *storage.Store) []storage.BrowserAuthFlow {
	var flows []storage.BrowserAuthFlow
	store.DB().Find(&flows)
	return flows
}

func TestStartCreatesAuthFlow(t *testing.T) {
	key := testSecretKey(t)
	store := newAuthTestStore(t)
	sessionRepo := storage.NewSessionRepository(store.DB())
	cfgRepo := storage.NewConfigRepository(store.DB())
	cfgSvc := NewOIDCConfigService(cfgRepo, key)
	ws := createTestWorkspace(t, store, "ws1")
	_ = cfgSvc.Set(ws.ID, OIDCConfigInput{Provider: "yaoguang", IssuerBaseURL: "http://fake", OrgID: "o", ClientID: "c", ClientSecret: "s", ExternalBaseURL: "http://xuanchu"})

	svc := NewOIDCAuthService(store, sessionRepo, cfgSvc, &stubOIDCProvider{})
	url, err := svc.Start(context.Background(), ws.ID)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if url == "" {
		t.Fatal("empty url")
	}
	flows := listAuthFlows(store)
	if len(flows) != 1 {
		t.Fatalf("flows = %d", len(flows))
	}
}

func TestCallbackSubNotMappedRejected(t *testing.T) {
	key := testSecretKey(t)
	store := newAuthTestStore(t)
	sessionRepo := storage.NewSessionRepository(store.DB())
	cfgRepo := storage.NewConfigRepository(store.DB())
	cfgSvc := NewOIDCConfigService(cfgRepo, key)
	ws := createTestWorkspace(t, store, "ws1")
	_ = cfgSvc.Set(ws.ID, OIDCConfigInput{Provider: "yaoguang", IssuerBaseURL: "http://fake", OrgID: "o", ClientID: "c", ClientSecret: "s", ExternalBaseURL: "http://xuanchu"})

	svc := NewOIDCAuthService(store, sessionRepo, cfgSvc, &stubOIDCProvider{})
	_, _ = svc.Start(context.Background(), ws.ID)
	flow := listAuthFlows(store)[0]

	_, err := svc.Callback(context.Background(), flow.State, "fakecode")
	if err == nil {
		t.Fatal("expected identity_not_found error")
	}
}

func TestCallbackSubMappedCreatesSession(t *testing.T) {
	key := testSecretKey(t)
	store := newAuthTestStore(t)
	sessionRepo := storage.NewSessionRepository(store.DB())
	cfgRepo := storage.NewConfigRepository(store.DB())
	cfgSvc := NewOIDCConfigService(cfgRepo, key)
	ws := createTestWorkspace(t, store, "ws1")
	_ = cfgSvc.Set(ws.ID, OIDCConfigInput{Provider: "yaoguang", IssuerBaseURL: "http://fake", OrgID: "o", ClientID: "c", ClientSecret: "s", ExternalBaseURL: "http://xuanchu"})

	// 预置 user + external id + membership（模拟同步结果）
	userRepo := storage.NewUserRepository(store.DB())
	u, _ := userRepo.Create(storage.User{ID: "u1", Name: "张三", CreatedAt: 1, ModifiedAt: 1})
	extRepo := storage.NewExternalIDRepository(store.DB())
	_, _ = extRepo.Create(storage.UserExternalID{ID: "e1", UserID: u.ID, Provider: "yaoguang", ExternalID: "yaoguang_member:m1", CreatedAt: 1})
	memberRepo := storage.NewMemberRepository(store.DB())
	_ = memberRepo.Upsert(storage.Membership{UserID: u.ID, WorkspaceID: ws.ID, Role: "member", JoinedAt: 1, ModifiedAt: 1})

	svc := NewOIDCAuthService(store, sessionRepo, cfgSvc, &stubOIDCProvider{sub: "yaoguang_member:m1"})
	_, _ = svc.Start(context.Background(), ws.ID)
	flow := listAuthFlows(store)[0]

	login, err := svc.Callback(context.Background(), flow.State, "fakecode")
	if err != nil {
		t.Fatalf("Callback: %v", err)
	}
	if login.RawSession == "" || login.CSRFToken == "" {
		t.Fatal("empty session/csrf token")
	}
	_, err = sessionRepo.GetSession(hashStr(login.RawSession))
	if err != nil {
		t.Fatalf("session not found: %v", err)
	}
	_, err = sessionRepo.GetAuthFlow(flow.State)
	if err != storage.ErrNotFound {
		t.Fatalf("flow should be deleted")
	}
}
