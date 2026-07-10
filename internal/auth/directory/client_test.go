package directory

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListMembers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/orgs/org1/directory/members" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer tok1" {
			t.Fatalf("auth = %s", r.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"data": map[string]any{
				"members": []map[string]any{
					{
						"id":           "m1",
						"sub":          "yaoguang_member:m1",
						"display_name": "张三",
						"role":         "owner",
						"status":       "active",
						"external_identities": []map[string]any{
							{"provider": "feishu", "user_type": "user_id", "value": "fs1"},
							{"provider": "feishu", "user_type": "open_id", "value": "ou_fs1"},
						},
					},
					{
						"id":                "m2",
						"sub":               "yaoguang_member:m2",
						"display_name":      "李四",
						"role":              "member",
						"status":            "disabled",
						"external_identities": []map[string]any{},
					},
				},
				"source": "organization_members",
				"stale":  false,
			},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.Client())
	members, err := c.ListMembers(srv.URL, "org1", "tok1")
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("got %d members", len(members))
	}
	m1 := members[0]
	if m1.Sub != "yaoguang_member:m1" || m1.DisplayName != "张三" || m1.Role != "owner" || m1.Status != "active" {
		t.Fatalf("m1 = %+v", m1)
	}
	if len(m1.ExternalIdentities) != 2 {
		t.Fatalf("m1 ext = %+v", m1.ExternalIdentities)
	}
	if m1.ExternalIdentities[0] != (Identity{Provider: "feishu", UserType: "user_id", Value: "fs1"}) {
		t.Fatalf("m1 ext[0] = %+v", m1.ExternalIdentities[0])
	}
	if m1.ExternalIdentities[1] != (Identity{Provider: "feishu", UserType: "open_id", Value: "ou_fs1"}) {
		t.Fatalf("m1 ext[1] = %+v", m1.ExternalIdentities[1])
	}
	if members[1].Status != "disabled" {
		t.Fatalf("m2 status = %s", members[1].Status)
	}
}

func TestListMembersUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"code": "invalid_token"}})
	}))
	defer srv.Close()

	c := NewClient(srv.Client())
	_, err := c.ListMembers(srv.URL, "org1", "bad")
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestListMembersWithIssuerURL 验证传入 OIDC issuer_base_url（含 /oidc/orgs/{org_id}）时，
// 能正确提取根地址拼接 directory API。
func TestListMembersWithIssuerURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/orgs/org1/directory/members" {
			t.Fatalf("path = %s, want /api/orgs/org1/directory/members", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": map[string]any{"members": []any{}}})
	}))
	defer srv.Close()

	c := NewClient(srv.Client())
	// issuer_base_url 形式：{root}/oidc/orgs/{org_id}
	issuerURL := srv.URL + "/oidc/orgs/org1"
	_, err := c.ListMembers(issuerURL, "org1", "tok1")
	if err != nil {
		t.Fatalf("ListMembers with issuer URL: %v", err)
	}
}

func TestYaoguangRootURL(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"https://yaoguang.example.com/oidc/orgs/abc-123", "https://yaoguang.example.com"},
		{"http://localhost:5174/oidc/orgs/019ee2ce", "http://localhost:5174"},
		{"https://yaoguang.example.com", "https://yaoguang.example.com"},
	}
	for _, tc := range cases {
		got := yaoguangRootURL(tc.input)
		if got != tc.want {
			t.Errorf("yaoguangRootURL(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
