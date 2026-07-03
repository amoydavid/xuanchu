package app

import (
	"context"
	"path/filepath"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/auth/directory"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"github.com/google/uuid"
)

type fakeDirectoryClient struct {
	members []directory.Member
	err     error
}

func (f *fakeDirectoryClient) ListMembersWithContext(ctx context.Context, baseURL, orgID, token string) ([]directory.Member, error) {
	return f.members, f.err
}

func newSyncTestStore(t *testing.T) *storage.Store {
	store, err := storage.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func createTestWorkspace(t *testing.T, store *storage.Store, slug string) storage.Workspace {
	t.Helper()
	now := int64(1000)
	wsRepo := storage.NewWorkspaceRepository(store.DB())
	ws, err := wsRepo.Create(storage.Workspace{ID: uuid.NewString(), Slug: slug, Name: slug, CreatedAt: now, ModifiedAt: now})
	if err != nil {
		t.Fatalf("create ws: %v", err)
	}
	return ws
}

func TestSyncCreatesUsersAndMemberships(t *testing.T) {
	store := newSyncTestStore(t)
	ws := createTestWorkspace(t, store, "ws1")

	fake := &fakeDirectoryClient{members: []directory.Member{
		{ID: "m1", Sub: "yaoguang_member:m1", DisplayName: "张三", Role: "owner", Status: "active",
			ExternalIdentities: []directory.Identity{{Provider: "feishu", Value: "f1"}}},
		{ID: "m2", Sub: "yaoguang_member:m2", DisplayName: "李四", Role: "member", Status: "active"},
	}}

	svc := NewDirectorySyncService(store, fake)
	stats, err := svc.SyncOnce(context.Background(), ws.ID, "https://yg", "org1", "token")
	if err != nil {
		t.Fatalf("SyncOnce: %v", err)
	}
	if stats.Added != 2 {
		t.Fatalf("added = %d, want 2", stats.Added)
	}

	// 验证 user + external id + membership
	userRepo := storage.NewUserRepository(store.DB())
	u1, err := userRepo.GetByExternalID("yaoguang", "yaoguang_member:m1")
	if err != nil {
		t.Fatalf("get by ext: %v", err)
	}
	if u1.DisplayName != "张三" {
		t.Fatalf("display = %s", u1.DisplayName)
	}
	memberRepo := storage.NewMemberRepository(store.DB())
	m, err := memberRepo.Get(u1.ID, ws.ID)
	if err != nil {
		t.Fatalf("get membership: %v", err)
	}
	if m.Role != "owner" {
		t.Fatalf("role = %s", m.Role)
	}
}

func TestSyncRemovesDisabled(t *testing.T) {
	store := newSyncTestStore(t)
	ws := createTestWorkspace(t, store, "ws1")

	// 第一次同步：m1 active
	fake := &fakeDirectoryClient{members: []directory.Member{
		{ID: "m1", Sub: "yaoguang_member:m1", DisplayName: "张三", Role: "member", Status: "active"},
	}}
	svc := NewDirectorySyncService(store, fake)
	svc.SyncOnce(context.Background(), ws.ID, "", "", "")

	// 第二次：m1 disabled
	fake.members = []directory.Member{
		{ID: "m1", Sub: "yaoguang_member:m1", DisplayName: "张三", Role: "member", Status: "disabled"},
	}
	stats, _ := svc.SyncOnce(context.Background(), ws.ID, "", "", "")
	if stats.Removed != 1 {
		t.Fatalf("removed = %d, want 1", stats.Removed)
	}

	// user 保留，membership 移除
	userRepo := storage.NewUserRepository(store.DB())
	u1, _ := userRepo.GetByExternalID("yaoguang", "yaoguang_member:m1")
	memberRepo := storage.NewMemberRepository(store.DB())
	_, err := memberRepo.Get(u1.ID, ws.ID)
	if err == nil {
		t.Fatal("membership should be removed")
	}
}

func TestSyncIdempotent(t *testing.T) {
	store := newSyncTestStore(t)
	ws := createTestWorkspace(t, store, "ws1")
	fake := &fakeDirectoryClient{members: []directory.Member{
		{ID: "m1", Sub: "yaoguang_member:m1", DisplayName: "张三", Role: "member", Status: "active"},
	}}
	svc := NewDirectorySyncService(store, fake)
	s1, _ := svc.SyncOnce(context.Background(), ws.ID, "", "", "")
	s2, _ := svc.SyncOnce(context.Background(), ws.ID, "", "", "")
	if s1.Added != 1 || s2.Added != 0 {
		t.Fatalf("first=%+v second=%+v, not idempotent", s1, s2)
	}
}

func TestSyncNameConflictSuffix(t *testing.T) {
	store := newSyncTestStore(t)
	ws := createTestWorkspace(t, store, "ws1")
	// 预置同名 user
	userRepo := storage.NewUserRepository(store.DB())
	_, _ = userRepo.Create(storage.User{Name: "张三", CreatedAt: 1, ModifiedAt: 1})

	fake := &fakeDirectoryClient{members: []directory.Member{
		{ID: "m1", Sub: "yaoguang_member:m1", DisplayName: "张三", Role: "member", Status: "active"},
	}}
	svc := NewDirectorySyncService(store, fake)
	_, err := svc.SyncOnce(context.Background(), ws.ID, "", "", "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	u, _ := userRepo.GetByExternalID("yaoguang", "yaoguang_member:m1")
	if u.Name == "张三" {
		t.Fatal("name should have suffix to avoid conflict")
	}
}
