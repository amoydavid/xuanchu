package app

import (
	"path/filepath"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

func newConfigTestStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestOIDCConfigRoundTrip(t *testing.T) {
	withSecretKey(t, func() {
		store := newConfigTestStore(t)
		cfgRepo := storage.NewConfigRepository(store.DB())
		svc := NewOIDCConfigService(cfgRepo)

		// 未配置
		got, enabled := svc.Get("ws1")
		if enabled {
			t.Fatal("should be disabled initially")
		}

		// 写入
		in := OIDCConfigInput{
			Provider:             "yaoguang",
			IssuerBaseURL:        "https://yg.example.com",
			OrgID:                "org1",
			ClientID:             "cid",
			ClientSecret:         "csecret",
			DirectoryAccessToken: "dtoken",
		}
		if err := svc.Set("ws1", in); err != nil {
			t.Fatalf("Set: %v", err)
		}

		got, enabled = svc.Get("ws1")
		if !enabled {
			t.Fatal("should be enabled after set")
		}
		if got.Provider != "yaoguang" || got.IssuerBaseURL != "https://yg.example.com" ||
			got.OrgID != "org1" || got.ClientID != "cid" {
			t.Fatalf("got %+v", got)
		}
		// secret 在读视图脱敏：mask("csecret") = cs••et, mask("dtoken") = dt••en
		if got.ClientSecretMasked != "cs••et" {
			t.Fatalf("client secret masked = %q, want cs••et", got.ClientSecretMasked)
		}
		if got.DirectoryAccessTokenMasked != "dt••en" {
			t.Fatalf("dir token masked = %q, want dt••en", got.DirectoryAccessTokenMasked)
		}
	})
}

func TestOIDCConfigSetPartialSecret(t *testing.T) {
	withSecretKey(t, func() {
		store := newConfigTestStore(t)
		cfgRepo := storage.NewConfigRepository(store.DB())
		svc := NewOIDCConfigService(cfgRepo)

		// 首次写入
		if err := svc.Set("ws1", OIDCConfigInput{
			Provider: "yaoguang", IssuerBaseURL: "u", OrgID: "o", ClientID: "c",
			ClientSecret: "secret123", DirectoryAccessToken: "token456",
		}); err != nil {
			t.Fatalf("first set: %v", err)
		}
		// 第二次写入，secret 留空 → 保留原值
		if err := svc.Set("ws1", OIDCConfigInput{
			Provider: "yaoguang", IssuerBaseURL: "u2", OrgID: "o", ClientID: "c",
			ClientSecret: "", DirectoryAccessToken: "",
		}); err != nil {
			t.Fatalf("second set: %v", err)
		}
		got, _ := svc.Get("ws1")
		if got.IssuerBaseURL != "u2" {
			t.Fatalf("issuer not updated: %s", got.IssuerBaseURL)
		}
		// 验证 secret 仍是原值（通过 ResolveSecrets 解密读原始值）
		secrets, err := svc.ResolveSecrets("ws1")
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if secrets.ClientSecret != "secret123" || secrets.DirectoryAccessToken != "token456" {
			t.Fatalf("secrets = %+v, want originals", secrets)
		}
	})
}

func TestOIDCConfigSetWithoutSecretKeyFails(t *testing.T) {
	t.Setenv(configSecretKeyEnv, "")
	store := newConfigTestStore(t)
	cfgRepo := storage.NewConfigRepository(store.DB())
	svc := NewOIDCConfigService(cfgRepo)
	err := svc.Set("ws1", OIDCConfigInput{
		Provider: "yaoguang", IssuerBaseURL: "u", OrgID: "o", ClientID: "c",
		ClientSecret: "s", DirectoryAccessToken: "t",
	})
	if err == nil {
		t.Fatal("expected config_secret_key_missing error")
	}
}
