package sqlite

import (
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestExternalIDRepoCreateAndGet(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	repo := NewExternalIDRepository(store.DB())
	userRepo := NewUserRepository(store.DB())

	user, err := userRepo.Create(User{
		ID:   uuid.NewString(),
		Name: "alice",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	extID := UserExternalID{
		ID:         uuid.NewString(),
		UserID:     user.ID,
		Provider:   "feishu",
		ExternalID: "ou_abc123",
		CreatedAt:  1700000000,
	}

	created, err := repo.Create(extID)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected non-empty ID")
	}

	found, err := repo.GetByProviderAndExternalID("feishu", "ou_abc123")
	if err != nil {
		t.Fatalf("get by provider+external_id: %v", err)
	}
	if found.UserID != user.ID {
		t.Fatalf("expected user_id %s, got %s", user.ID, found.UserID)
	}
}

func TestExternalIDRepoCreateDuplicateFails(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	repo := NewExternalIDRepository(store.DB())
	userRepo := NewUserRepository(store.DB())

	user, err := userRepo.Create(User{
		ID:   uuid.NewString(),
		Name: "bob",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	extID := UserExternalID{
		ID: uuid.NewString(), UserID: user.ID, Provider: "feishu", ExternalID: "ou_dup", CreatedAt: 1700000000,
	}
	if _, err := repo.Create(extID); err != nil {
		t.Fatalf("first create: %v", err)
	}

	extID2 := UserExternalID{
		ID: uuid.NewString(), UserID: user.ID, Provider: "feishu", ExternalID: "ou_dup", CreatedAt: 1700000001,
	}
	if _, err := repo.Create(extID2); err == nil {
		t.Fatal("expected duplicate (provider, external_id) to fail")
	}
}

func TestExternalIDRepoListByUser(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	repo := NewExternalIDRepository(store.DB())
	userRepo := NewUserRepository(store.DB())

	user, err := userRepo.Create(User{ID: uuid.NewString(), Name: "carol"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	for _, ext := range []UserExternalID{
		{ID: uuid.NewString(), UserID: user.ID, Provider: "feishu", ExternalID: "ou_carol", CreatedAt: 1700000000},
		{ID: uuid.NewString(), UserID: user.ID, Provider: "slack", ExternalID: "U_CAROL", CreatedAt: 1700000001},
	} {
		if _, err := repo.Create(ext); err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	ids, err := repo.ListByUser(user.ID)
	if err != nil {
		t.Fatalf("list by user: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 external IDs, got %d", len(ids))
	}
}

func TestExternalIDRepoDelete(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	repo := NewExternalIDRepository(store.DB())
	userRepo := NewUserRepository(store.DB())

	user, err := userRepo.Create(User{ID: uuid.NewString(), Name: "dave"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	_, err = repo.Create(UserExternalID{
		ID: uuid.NewString(), UserID: user.ID, Provider: "feishu", ExternalID: "ou_dave", CreatedAt: 1700000000,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := repo.Delete(user.ID, "feishu", "ou_dave"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err = repo.GetByProviderAndExternalID("feishu", "ou_dave")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got: %v", err)
	}
}

func TestExternalIDRepoDeleteNotFound(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	repo := NewExternalIDRepository(store.DB())
	userRepo := NewUserRepository(store.DB())

	user, err := userRepo.Create(User{ID: uuid.NewString(), Name: "eve"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	err = repo.Delete(user.ID, "feishu", "ou_nonexist")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestExternalIDRepoListByUsers(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	repo := NewExternalIDRepository(store.DB())
	userRepo := NewUserRepository(store.DB())

	user1, err := userRepo.Create(User{ID: uuid.NewString(), Name: "eve"})
	if err != nil {
		t.Fatalf("create user1: %v", err)
	}
	user2, err := userRepo.Create(User{ID: uuid.NewString(), Name: "frank"})
	if err != nil {
		t.Fatalf("create user2: %v", err)
	}

	repo.Create(UserExternalID{ID: uuid.NewString(), UserID: user1.ID, Provider: "feishu", ExternalID: "ou_eve", CreatedAt: 1700000000})
	repo.Create(UserExternalID{ID: uuid.NewString(), UserID: user1.ID, Provider: "slack", ExternalID: "U_EVE", CreatedAt: 1700000001})
	repo.Create(UserExternalID{ID: uuid.NewString(), UserID: user2.ID, Provider: "feishu", ExternalID: "ou_frank", CreatedAt: 1700000002})

	result, err := repo.ListByUsers([]string{user1.ID, user2.ID})
	if err != nil {
		t.Fatalf("list by users: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 external IDs, got %d", len(result))
	}
}

func TestExternalIDRepoListByUsersEmpty(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	repo := NewExternalIDRepository(store.DB())

	result, err := repo.ListByUsers(nil)
	if err != nil {
		t.Fatalf("list by users: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected 0 external IDs, got %d", len(result))
	}
}
