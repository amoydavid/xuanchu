package storage

import (
	"path/filepath"
	"testing"
)

func newMemberTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func seedMember(t *testing.T, store *Store, wsID, userID, name, displayName string, email *string) {
	t.Helper()
	if _, err := NewUserRepository(store.DB()).Create(User{ID: userID, Name: name, DisplayName: displayName, Email: email, CreatedAt: 1, ModifiedAt: 1}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := NewMemberRepository(store.DB()).Upsert(Membership{UserID: userID, WorkspaceID: wsID, Role: "member", JoinedAt: 1, ModifiedAt: 1}); err != nil {
		t.Fatalf("upsert membership: %v", err)
	}
}

func TestMemberRepositorySearchActiveMembers(t *testing.T) {
	store := newMemberTestStore(t)
	repo := NewMemberRepository(store.DB())
	wsID := "ws-search"
	seedMember(t, store, wsID, "u1", "alice", "Alice Lee", strPtr("alice@example.com"))
	seedMember(t, store, wsID, "u2", "bob", "Bob", strPtr("bob@example.com"))
	seedMember(t, store, wsID, "u3", "carol", "Carol", nil)

	// 按 display_name 匹配。
	got, err := repo.SearchActiveMembers(wsID, "Alice", 10)
	if err != nil {
		t.Fatalf("SearchActiveMembers: %v", err)
	}
	if len(got) != 1 || got[0].User.ID != "u1" {
		t.Fatalf("got = %#v", got)
	}

	// 按 name 匹配。
	got, _ = repo.SearchActiveMembers(wsID, "bob", 10)
	if len(got) != 1 || got[0].User.ID != "u2" {
		t.Fatalf("got = %#v", got)
	}

	// 按 email 匹配。
	got, _ = repo.SearchActiveMembers(wsID, "carol@example.com", 10)
	if len(got) != 0 {
		t.Fatalf("carol has no email; should not match email pattern, got %#v", got)
	}
	got, _ = repo.SearchActiveMembers(wsID, "alice@example.com", 10)
	if len(got) != 1 || got[0].User.ID != "u1" {
		t.Fatalf("got = %#v", got)
	}

	// 大小写不敏感。
	got, _ = repo.SearchActiveMembers(wsID, "ALICE", 10)
	if len(got) != 1 {
		t.Fatalf("case-insensitive match failed: %#v", got)
	}

	// 不返回其它 workspace 的成员。
	seedMember(t, store, "other-ws", "u4", "alice-other", "Alice Other", nil)
	got, _ = repo.SearchActiveMembers(wsID, "alice", 10)
	if len(got) != 1 {
		t.Fatalf("leaked other-workspace member: %#v", got)
	}
}

func TestMemberRepositoryListMembersByUserIDs(t *testing.T) {
	store := newMemberTestStore(t)
	repo := NewMemberRepository(store.DB())
	wsID := "ws-list"
	seedMember(t, store, wsID, "u1", "alice", "Alice", nil)
	seedMember(t, store, wsID, "u2", "bob", "Bob", nil)

	got, err := repo.ListMembersByUserIDs(wsID, []string{"u1", "u2", "missing"})
	if err != nil {
		t.Fatalf("ListMembersByUserIDs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got = %#v", got)
	}

	got, _ = repo.ListMembersByUserIDs(wsID, nil)
	if len(got) != 0 {
		t.Fatalf("nil ids should return empty, got %#v", got)
	}
}
