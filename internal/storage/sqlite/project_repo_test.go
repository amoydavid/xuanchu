package sqlite

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

func newProjectRepoTest(t *testing.T) (*Store, *ProjectRepository, Workspace) {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	return store, NewProjectRepository(store.DB()), ws
}

func TestProjectRepositoryEnforcesUniqueSlugPerWorkspace(t *testing.T) {
	_, repo, ws := newProjectRepoTest(t)
	project := testProject("p1", ws.ID, "customer", 100)
	if _, err := repo.Create(project); err != nil {
		t.Fatalf("Create(first) error = %v", err)
	}
	if _, err := repo.Create(testProject("p2", ws.ID, "customer", 101)); err == nil {
		t.Fatal("Create(duplicate slug) error = nil, want unique constraint error")
	}
}

func TestProjectRepositoryAllowsSameSlugInDifferentWorkspaces(t *testing.T) {
	store, repo, ws := newProjectRepoTest(t)
	other := createTestWorkspace(t, store, "team")

	if _, err := repo.Create(testProject("p1", ws.ID, "customer", 100)); err != nil {
		t.Fatalf("Create(local) error = %v", err)
	}
	if _, err := repo.Create(testProject("p2", other.ID, "customer", 101)); err != nil {
		t.Fatalf("Create(other workspace same slug) error = %v", err)
	}
}

func TestProjectRepositoryResolveInWorkspaceBySlug(t *testing.T) {
	_, repo, ws := newProjectRepoTest(t)
	created, err := repo.Create(testProject("p1", ws.ID, "customer", 100))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	bySlug, err := repo.GetBySlug(ws.ID, "customer")
	if err != nil {
		t.Fatalf("GetBySlug() error = %v", err)
	}
	if bySlug.ID != created.ID {
		t.Fatalf("GetBySlug().ID = %q, want %q", bySlug.ID, created.ID)
	}

	byRef, err := repo.GetByRef(ws.ID, "customer")
	if err != nil {
		t.Fatalf("GetByRef() error = %v", err)
	}
	if byRef.ID != created.ID {
		t.Fatalf("GetByRef().ID = %q, want %q", byRef.ID, created.ID)
	}

	resolved, err := repo.ResolveInWorkspace(ws.ID, "customer")
	if err != nil {
		t.Fatalf("ResolveInWorkspace(slug) error = %v", err)
	}
	if resolved.ID != created.ID {
		t.Fatalf("ResolveInWorkspace(slug).ID = %q, want %q", resolved.ID, created.ID)
	}
}

func TestProjectRepositoryResolveInWorkspaceRejectsProjectIDFromAnotherWorkspace(t *testing.T) {
	store, repo, ws := newProjectRepoTest(t)
	other := createTestWorkspace(t, store, "team")
	created, err := repo.Create(testProject("p-other", other.ID, "customer", 100))
	if err != nil {
		t.Fatalf("Create(other) error = %v", err)
	}

	_, err = repo.ResolveInWorkspace(ws.ID, created.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("ResolveInWorkspace(cross workspace id) error = %v, want ErrNotFound", err)
	}
}

func TestProjectRepositoryArchiveAlreadyArchived(t *testing.T) {
	_, repo, ws := newProjectRepoTest(t)
	created, err := repo.Create(testProject("p1", ws.ID, "customer", 100))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := repo.Archive(ws.ID, created.ID, 200); err != nil {
		t.Fatalf("Archive(first) error = %v", err)
	}
	if err := repo.Archive(ws.ID, created.ID, 300); !errors.Is(err, ErrAlreadyArchived) {
		t.Fatalf("Archive(already archived) error = %v, want ErrAlreadyArchived", err)
	}
}

func TestProjectRepositoryListOrdersBySlugAndFiltersArchived(t *testing.T) {
	_, repo, ws := newProjectRepoTest(t)
	for _, project := range []Project{
		testProject("p-z", ws.ID, "zeta", 100),
		testProject("p-a", ws.ID, "alpha", 101),
		testProject("p-m", ws.ID, "middle", 102),
	} {
		if _, err := repo.Create(project); err != nil {
			t.Fatalf("Create(%s) error = %v", project.ID, err)
		}
	}
	if err := repo.Archive(ws.ID, "p-m", 200); err != nil {
		t.Fatalf("Archive() error = %v", err)
	}

	active, err := repo.List(ws.ID, false)
	if err != nil {
		t.Fatalf("List(active) error = %v", err)
	}
	if got := projectSlugsForRepo(active); !reflect.DeepEqual(got, []string{"alpha", "zeta"}) {
		t.Fatalf("List(active) slugs = %#v", got)
	}

	all, err := repo.List(ws.ID, true)
	if err != nil {
		t.Fatalf("List(all) error = %v", err)
	}
	if got := projectSlugsForRepo(all); !reflect.DeepEqual(got, []string{"alpha", "middle", "zeta"}) {
		t.Fatalf("List(all) slugs = %#v", got)
	}
}

func TestProjectRepositoryTaskCountsIgnoresDeletedTasks(t *testing.T) {
	store, repo, ws := newProjectRepoTest(t)
	alpha, err := repo.Create(testProject("p-alpha", ws.ID, "alpha", 100))
	if err != nil {
		t.Fatalf("Create(alpha) error = %v", err)
	}
	beta, err := repo.Create(testProject("p-beta", ws.ID, "beta", 101))
	if err != nil {
		t.Fatalf("Create(beta) error = %v", err)
	}
	other := createTestWorkspace(t, store, "team")
	otherProject, err := repo.Create(testProject("p-other", other.ID, "alpha", 102))
	if err != nil {
		t.Fatalf("Create(other) error = %v", err)
	}
	insertTaskForProjectCount(t, store, "t1", ws.ID, alpha.ID, "pending")
	insertTaskForProjectCount(t, store, "t2", ws.ID, alpha.ID, "completed")
	insertTaskForProjectCount(t, store, "t3", ws.ID, alpha.ID, "deleted")
	insertTaskForProjectCount(t, store, "t4", ws.ID, beta.ID, "pending")
	insertTaskForProjectCount(t, store, "t5", other.ID, otherProject.ID, "pending")

	counts, err := repo.TaskCounts(ws.ID, []string{alpha.ID, beta.ID, otherProject.ID})
	if err != nil {
		t.Fatalf("TaskCounts() error = %v", err)
	}
	want := map[string]int{alpha.ID: 2, beta.ID: 1, otherProject.ID: 0}
	if !reflect.DeepEqual(counts, want) {
		t.Fatalf("TaskCounts() = %#v, want %#v", counts, want)
	}
}

func testProject(id, workspaceID, slug string, now int64) Project {
	return Project{
		ID:           id,
		WorkspaceID:  workspaceID,
		Slug:         slug,
		Name:         slug,
		Description:  "",
		Status:       string(ProjectStatusActive),
		SettingsJSON: "{}",
		CreatedAt:    now,
		ModifiedAt:   now,
	}
}

func createTestWorkspace(t *testing.T, store *Store, slug string) Workspace {
	t.Helper()
	ws := Workspace{
		ID:           "ws-" + slug,
		Slug:         slug,
		Name:         slug,
		Description:  "",
		Visibility:   "private",
		SettingsJSON: "{}",
		CreatedAt:    100,
		ModifiedAt:   100,
	}
	if err := store.DB().Create(&ws).Error; err != nil {
		t.Fatalf("create workspace %q: %v", slug, err)
	}
	return ws
}

func insertTaskForProjectCount(t *testing.T, store *Store, uuid, workspaceID, projectID, status string) {
	t.Helper()
	if err := store.DB().Exec(
		`INSERT INTO tasks(uuid, workspace_id, description, status, entry, modified, project, project_id) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid, workspaceID, "task "+uuid, status, int64(100), int64(100), "project", projectID,
	).Error; err != nil {
		t.Fatalf("insert task %q: %v", uuid, err)
	}
}

func projectSlugsForRepo(projects []Project) []string {
	slugs := make([]string, 0, len(projects))
	for _, project := range projects {
		slugs = append(slugs, project.Slug)
	}
	return slugs
}
