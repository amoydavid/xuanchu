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
	if len(m1.ExternalIdentities) != 1 || m1.ExternalIdentities[0].Provider != "feishu" || m1.ExternalIdentities[0].Value != "fs1" {
		t.Fatalf("m1 ext = %+v", m1.ExternalIdentities)
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
