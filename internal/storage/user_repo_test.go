package storage

import (
	"path/filepath"
	"testing"
)

func TestUserRepositoryListByIDs(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	repo := NewUserRepository(store.DB())

	users := []User{
		{ID: "u1", Name: "alice", CreatedAt: 100, ModifiedAt: 100},
		{ID: "u2", Name: "bob", CreatedAt: 100, ModifiedAt: 100},
		{ID: "u3", Name: "carol", CreatedAt: 100, ModifiedAt: 100},
	}
	for _, u := range users {
		if _, err := repo.Create(u); err != nil {
			t.Fatalf("Create(%s) error = %v", u.ID, err)
		}
	}

	// 批量查子集
	got, err := repo.ListByIDs([]string{"u1", "u3"})
	if err != nil {
		t.Fatalf("ListByIDs() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %#v", len(got), got)
	}
	names := map[string]bool{}
	for _, u := range got {
		names[u.Name] = true
	}
	if !names["alice"] || !names["carol"] {
		t.Fatalf("got users = %#v, want alice+carol", got)
	}

	// 空 ids 不查 DB，返回空切片
	empty, err := repo.ListByIDs([]string{})
	if err != nil {
		t.Fatalf("ListByIDs([]) error = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("empty len = %d, want 0", len(empty))
	}

	// 含不存在的 ID：只返回存在的
	partial, err := repo.ListByIDs([]string{"u1", "nonexistent"})
	if err != nil {
		t.Fatalf("ListByIDs(partial) error = %v", err)
	}
	if len(partial) != 1 || partial[0].ID != "u1" {
		t.Fatalf("partial = %#v, want [u1]", partial)
	}
}
