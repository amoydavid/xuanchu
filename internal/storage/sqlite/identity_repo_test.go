package sqlite

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

func openIdentityTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func mustLocalUser(t *testing.T, store *Store) User {
	t.Helper()
	user, err := NewUserRepository(store.DB()).GetByName("local")
	if err != nil {
		t.Fatalf("GetByName(local) error = %v", err)
	}
	return user
}

func TestUserRepositoryLookupByNameEmailAndID(t *testing.T) {
	store := openIdentityTestStore(t)
	repo := NewUserRepository(store.DB())

	email := "alice@example.test"
	created, err := repo.Create(User{
		ID:         "user-alice",
		Name:       "alice",
		Email:      &email,
		CreatedAt:  100,
		ModifiedAt: 100,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	byID, err := repo.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	byName, err := repo.GetByName("alice")
	if err != nil {
		t.Fatalf("GetByName() error = %v", err)
	}
	byEmail, err := repo.GetByEmail(email)
	if err != nil {
		t.Fatalf("GetByEmail() error = %v", err)
	}
	if byID.ID != created.ID || byName.ID != created.ID || byEmail.ID != created.ID {
		t.Fatalf("lookup mismatch: id=%q name=%q email=%q want %q", byID.ID, byName.ID, byEmail.ID, created.ID)
	}
}

func TestWorkspaceRepositoryListVisibleExcludesArchived(t *testing.T) {
	store := openIdentityTestStore(t)
	user := mustLocalUser(t, store)
	repo := NewWorkspaceRepository(store.DB())
	memberRepo := NewMemberRepository(store.DB())

	team, err := repo.Create(Workspace{
		ID:         "ws-team",
		Slug:       "team",
		Name:       "Team",
		Visibility: "team",
		CreatedAt:  100,
		ModifiedAt: 100,
	})
	if err != nil {
		t.Fatalf("Create(team) error = %v", err)
	}
	archivedAt := int64(300)
	old, err := repo.Create(Workspace{
		ID:         "ws-old",
		Slug:       "old",
		Name:       "Old",
		Visibility: "private",
		ArchivedAt: &archivedAt,
		CreatedAt:  200,
		ModifiedAt: 200,
	})
	if err != nil {
		t.Fatalf("Create(old) error = %v", err)
	}
	if err := memberRepo.Upsert(Membership{UserID: user.ID, WorkspaceID: team.ID, Role: "admin", JoinedAt: 100, ModifiedAt: 100}); err != nil {
		t.Fatalf("Upsert(team membership) error = %v", err)
	}
	if err := memberRepo.Upsert(Membership{UserID: user.ID, WorkspaceID: old.ID, Role: "viewer", JoinedAt: 200, ModifiedAt: 200}); err != nil {
		t.Fatalf("Upsert(old membership) error = %v", err)
	}

	visible, err := repo.ListVisibleForUser(user.ID, false)
	if err != nil {
		t.Fatalf("ListVisibleForUser(false) error = %v", err)
	}
	if len(visible) != 2 {
		t.Fatalf("len(visible) = %d, want 2 (local + team)", len(visible))
	}
	if visible[0].Workspace.Slug != "local" || visible[1].Workspace.Slug != "team" {
		t.Fatalf("visible slugs = %#v", []string{visible[0].Workspace.Slug, visible[1].Workspace.Slug})
	}

	all, err := repo.ListVisibleForUser(user.ID, true)
	if err != nil {
		t.Fatalf("ListVisibleForUser(true) error = %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("len(all) = %d, want 3", len(all))
	}
	if got := []string{all[0].Workspace.Slug, all[1].Workspace.Slug, all[2].Workspace.Slug}; !reflect.DeepEqual(got, []string{"local", "old", "team"}) {
		t.Fatalf("all slugs = %#v", got)
	}
}

func TestMemberRepositoryUpdateRolePreservesJoinedAtAndCountsOwners(t *testing.T) {
	store := openIdentityTestStore(t)
	local := mustLocalUser(t, store)
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace() error = %v", err)
	}
	userRepo := NewUserRepository(store.DB())
	memberRepo := NewMemberRepository(store.DB())

	bob, err := userRepo.Create(User{ID: "user-bob", Name: "bob", CreatedAt: 100, ModifiedAt: 100})
	if err != nil {
		t.Fatalf("Create(bob) error = %v", err)
	}
	if err := memberRepo.Upsert(Membership{UserID: bob.ID, WorkspaceID: ws.ID, Role: "owner", JoinedAt: 200, ModifiedAt: 200}); err != nil {
		t.Fatalf("Upsert(bob) error = %v", err)
	}
	if err := memberRepo.UpdateRole(bob.ID, ws.ID, "admin", 300); err != nil {
		t.Fatalf("UpdateRole() error = %v", err)
	}
	got, err := memberRepo.Get(bob.ID, ws.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.JoinedAt != 200 || got.ModifiedAt != 300 || got.Role != "admin" {
		t.Fatalf("membership = %#v", got)
	}
	count, err := memberRepo.CountOwners(ws.ID)
	if err != nil {
		t.Fatalf("CountOwners() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("owner count = %d, want 1", count)
	}
	if local.ID == "" {
		t.Fatal("local user ID is empty")
	}
}

func TestAuditRepositoryListsNewestFirst(t *testing.T) {
	store := openIdentityTestStore(t)
	repo := NewAuditRepository(store.DB())
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace() error = %v", err)
	}
	actor := mustLocalUser(t, store)

	for i, action := range []string{"a", "b", "c"} {
		if err := repo.Append(AuditLogEntry{
			ActorUserID: &actor.ID,
			WorkspaceID: &ws.ID,
			Action:      action,
			CreatedAt:   int64(100 + i),
		}); err != nil {
			t.Fatalf("Append(%q) error = %v", action, err)
		}
	}
	rows, err := repo.List(AuditListOptions{WorkspaceID: &ws.ID, Limit: 2})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got := []string{rows[0].Action, rows[1].Action}; !reflect.DeepEqual(got, []string{"c", "b"}) {
		t.Fatalf("actions = %#v", got)
	}
}

func TestAuditRepositoryPreservesAndFiltersProjectID(t *testing.T) {
	store := openIdentityTestStore(t)
	repo := NewAuditRepository(store.DB())
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace() error = %v", err)
	}
	projectID := "project-api"
	otherProjectID := "project-web"

	entries := []AuditLogEntry{
		{WorkspaceID: &ws.ID, ProjectID: &projectID, Action: "task.modify", CreatedAt: 100},
		{WorkspaceID: &ws.ID, ProjectID: &otherProjectID, Action: "task.modify", CreatedAt: 200},
		{WorkspaceID: &ws.ID, ProjectID: &projectID, Action: "task.done", CreatedAt: 300},
	}
	for _, entry := range entries {
		if err := repo.Append(entry); err != nil {
			t.Fatalf("Append(%q) error = %v", entry.Action, err)
		}
	}

	rows, err := repo.List(AuditListOptions{WorkspaceID: &ws.ID, ProjectID: &projectID, Limit: 10})
	if err != nil {
		t.Fatalf("List(project) error = %v", err)
	}
	if got := []string{rows[0].Action, rows[1].Action}; !reflect.DeepEqual(got, []string{"task.done", "task.modify"}) {
		t.Fatalf("project actions = %#v", got)
	}
	for _, row := range rows {
		if row.ProjectID == nil || *row.ProjectID != projectID {
			t.Fatalf("row ProjectID = %#v, want %q", row.ProjectID, projectID)
		}
	}
}

func TestAuditRepositoryRejectsProjectFilterWithoutWorkspace(t *testing.T) {
	store := openIdentityTestStore(t)
	repo := NewAuditRepository(store.DB())
	projectID := "project-api"

	_, err := repo.List(AuditListOptions{ProjectID: &projectID})
	if !errors.Is(err, ErrInvalidAuditScope) {
		t.Fatalf("List(project without workspace) error = %v, want errors.Is ErrInvalidAuditScope", err)
	}
}

func TestAuditRepositoryProjectFilterRequiresMatchingWorkspace(t *testing.T) {
	store := openIdentityTestStore(t)
	repo := NewAuditRepository(store.DB())
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace() error = %v", err)
	}
	otherWorkspaceID := "ws-other"
	if err := store.DB().Create(&Workspace{
		ID:         otherWorkspaceID,
		Slug:       "other",
		Name:       "Other",
		Visibility: "team",
		CreatedAt:  100,
		ModifiedAt: 100,
	}).Error; err != nil {
		t.Fatalf("Create(other workspace) error = %v", err)
	}
	projectID := "project-shared"

	entries := []AuditLogEntry{
		{WorkspaceID: &ws.ID, ProjectID: &projectID, Action: "local.project", CreatedAt: 100},
		{WorkspaceID: &otherWorkspaceID, ProjectID: &projectID, Action: "other.project", CreatedAt: 200},
	}
	for _, entry := range entries {
		if err := repo.Append(entry); err != nil {
			t.Fatalf("Append(%q) error = %v", entry.Action, err)
		}
	}

	rows, err := repo.List(AuditListOptions{WorkspaceID: &ws.ID, ProjectID: &projectID, Limit: 10})
	if err != nil {
		t.Fatalf("List(workspace project) error = %v", err)
	}
	if len(rows) != 1 || rows[0].Action != "local.project" {
		t.Fatalf("rows = %#v, want only local.project", rows)
	}
	if rows[0].WorkspaceID == nil || *rows[0].WorkspaceID != ws.ID {
		t.Fatalf("row WorkspaceID = %#v, want %q", rows[0].WorkspaceID, ws.ID)
	}
}
