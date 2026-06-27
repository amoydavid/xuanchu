package storage

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"gorm.io/gorm"
)

func newProjectRepoTest(t *testing.T) (*Store, *ProjectRepository, Workspace) {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
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

func TestProjectRepositoryCreateInitializesNextTaskSeq(t *testing.T) {
	_, repo, ws := newProjectRepoTest(t)
	project, err := repo.Create(testProject("p1", ws.ID, "api", 100))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if project.NextTaskSeq != 1 {
		t.Fatalf("NextTaskSeq = %d, want 1", project.NextTaskSeq)
	}
}

func TestProjectRepositoryAllocateProjectTaskSeqIncrements(t *testing.T) {
	_, repo, ws := newProjectRepoTest(t)
	project, err := repo.Create(testProject("p1", ws.ID, "api", 100))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	first, err := repo.AllocateProjectTaskSeqLocked(ws.ID, project.ID)
	if err != nil {
		t.Fatalf("AllocateProjectTaskSeqLocked(first) error = %v", err)
	}
	second, err := repo.AllocateProjectTaskSeqLocked(ws.ID, project.ID)
	if err != nil {
		t.Fatalf("AllocateProjectTaskSeqLocked(second) error = %v", err)
	}
	if first != 1 || second != 2 {
		t.Fatalf("allocated seqs = %d,%d, want 1,2", first, second)
	}
	got, err := repo.GetByID(project.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.NextTaskSeq != 3 {
		t.Fatalf("NextTaskSeq = %d, want 3", got.NextTaskSeq)
	}
}

func TestProjectRepositoryAllocateProjectTaskSeqRollsBackWithOuterTx(t *testing.T) {
	store, repo, ws := newProjectRepoTest(t)
	project, err := repo.Create(testProject("p1", ws.ID, "api", 100))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	sentinel := errors.New("rollback")
	if err := store.DB().Transaction(func(tx *gorm.DB) error {
		txRepo := NewProjectRepository(tx)
		seq, err := txRepo.AllocateProjectTaskSeqLocked(ws.ID, project.ID)
		if err != nil {
			return err
		}
		if seq != 1 {
			t.Fatalf("seq inside rollback tx = %d, want 1", seq)
		}
		return sentinel
	}); !errors.Is(err, sentinel) {
		t.Fatalf("transaction error = %v, want sentinel", err)
	}

	seq, err := repo.AllocateProjectTaskSeqLocked(ws.ID, project.ID)
	if err != nil {
		t.Fatalf("AllocateProjectTaskSeqLocked(after rollback) error = %v", err)
	}
	if seq != 1 {
		t.Fatalf("seq after rollback = %d, want 1", seq)
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

func TestProjectRepositoryTaskStatusCounts(t *testing.T) {
	store, repo, ws := newProjectRepoTest(t)
	alpha, err := repo.Create(testProject("p-alpha", ws.ID, "alpha", 100))
	if err != nil {
		t.Fatalf("Create(alpha) error = %v", err)
	}
	beta, err := repo.Create(testProject("p-beta", ws.ID, "beta", 101))
	if err != nil {
		t.Fatalf("Create(beta) error = %v", err)
	}
	// alpha: 2 pending + 1 completed + 1 deleted(不计入) → total 3
	insertTaskForProjectCount(t, store, "a1", ws.ID, alpha.ID, "pending")
	insertTaskForProjectCount(t, store, "a2", ws.ID, alpha.ID, "pending")
	insertTaskForProjectCount(t, store, "a3", ws.ID, alpha.ID, "completed")
	insertTaskForProjectCount(t, store, "a4", ws.ID, alpha.ID, "deleted")
	// beta: 1 completed → total 1
	insertTaskForProjectCount(t, store, "b1", ws.ID, beta.ID, "completed")

	counts, err := repo.TaskStatusCounts(ws.ID, []string{alpha.ID, beta.ID})
	if err != nil {
		t.Fatalf("TaskStatusCounts() error = %v", err)
	}
	if counts[alpha.ID] != (ProjectTaskCounts{Total: 3, Pending: 2, Completed: 1}) {
		t.Fatalf("alpha counts = %#v, want {Total:3 Pending:2 Completed:1}", counts[alpha.ID])
	}
	if counts[beta.ID] != (ProjectTaskCounts{Total: 1, Pending: 0, Completed: 1}) {
		t.Fatalf("beta counts = %#v, want {Total:1 Pending:0 Completed:1}", counts[beta.ID])
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
		`INSERT INTO tasks(uuid, workspace_id, title, status, entry, modified, project, project_id) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
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

func TestProjectStatusHelpers(t *testing.T) {
	for _, s := range []string{"planning", "active", "archived", "cancelled"} {
		if !IsValidProjectStatus(s) {
			t.Fatalf("IsValidProjectStatus(%q) = false, want true", s)
		}
	}
	if IsValidProjectStatus("unknown") {
		t.Fatal("IsValidProjectStatus(unknown) = true, want false")
	}
	for _, closed := range []string{"archived", "cancelled"} {
		if !IsProjectClosedStatus(closed) {
			t.Fatalf("IsProjectClosedStatus(%q) = false, want true", closed)
		}
	}
	for _, open := range []string{"planning", "active"} {
		if IsProjectClosedStatus(open) {
			t.Fatalf("IsProjectClosedStatus(%q) = true, want false", open)
		}
	}
}

func TestProjectRepositoryListByStatus(t *testing.T) {
	_, repo, ws := newProjectRepoTest(t)
	alpha, err := repo.Create(testProject("p-alpha", ws.ID, "alpha", 100))
	if err != nil {
		t.Fatalf("Create(alpha) error = %v", err)
	}
	beta, err := repo.Create(testProject("p-beta", ws.ID, "beta", 101))
	if err != nil {
		t.Fatalf("Create(beta) error = %v", err)
	}
	gamma, err := repo.Create(testProject("p-gamma", ws.ID, "gamma", 102))
	if err != nil {
		t.Fatalf("Create(gamma) error = %v", err)
	}
	delta, err := repo.Create(testProject("p-delta", ws.ID, "delta", 103))
	if err != nil {
		t.Fatalf("Create(delta) error = %v", err)
	}
	_ = alpha
	if err := repo.UpdateStatus(ws.ID, beta.ID, string(ProjectStatusPlanning), 200); err != nil {
		t.Fatalf("UpdateStatus(beta->planning) error = %v", err)
	}
	if err := repo.UpdateStatus(ws.ID, gamma.ID, string(ProjectStatusArchived), 300); err != nil {
		t.Fatalf("UpdateStatus(gamma->archived) error = %v", err)
	}
	if err := repo.UpdateStatus(ws.ID, delta.ID, string(ProjectStatusCancelled), 400); err != nil {
		t.Fatalf("UpdateStatus(delta->cancelled) error = %v", err)
	}

	tests := []struct {
		name   string
		filter string
		want   []string
	}{
		{"open", "open", []string{"alpha", "beta"}},
		{"planning", "planning", []string{"beta"}},
		{"active", "active", []string{"alpha"}},
		{"archived", "archived", []string{"gamma"}},
		{"cancelled", "cancelled", []string{"delta"}},
		{"all", "all", []string{"alpha", "beta", "delta", "gamma"}},
		{"empty defaults to all", "", []string{"alpha", "beta", "delta", "gamma"}},
	}
	for _, tc := range tests {
		got, err := repo.ListByStatus(ws.ID, tc.filter)
		if err != nil {
			t.Fatalf("ListByStatus(%s) error = %v", tc.name, err)
		}
		if slugs := projectSlugsForRepo(got); !reflect.DeepEqual(slugs, tc.want) {
			t.Fatalf("ListByStatus(%s) = %#v, want %#v", tc.name, slugs, tc.want)
		}
	}
}

func TestProjectRepositoryUpdateStatus(t *testing.T) {
	_, repo, ws := newProjectRepoTest(t)
	created, err := repo.Create(testProject("p1", ws.ID, "alpha", 100))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := repo.UpdateStatus(ws.ID, created.ID, string(ProjectStatusArchived), 200); err != nil {
		t.Fatalf("UpdateStatus(->archived) error = %v", err)
	}
	archived, err := repo.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if archived.Status != string(ProjectStatusArchived) {
		t.Fatalf("Status = %q, want archived", archived.Status)
	}
	if archived.ArchivedAt == nil || *archived.ArchivedAt != 200 {
		t.Fatalf("ArchivedAt = %v, want 200", archived.ArchivedAt)
	}

	if err := repo.UpdateStatus(ws.ID, created.ID, string(ProjectStatusActive), 300); err != nil {
		t.Fatalf("UpdateStatus(->active) error = %v", err)
	}
	reactivated, err := repo.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if reactivated.Status != string(ProjectStatusActive) {
		t.Fatalf("Status = %q, want active", reactivated.Status)
	}
	if reactivated.ArchivedAt != nil {
		t.Fatalf("ArchivedAt = %v, want nil after reactivation", reactivated.ArchivedAt)
	}

	if err := repo.UpdateStatus(ws.ID, created.ID, "unknown", 400); err == nil {
		t.Fatal("UpdateStatus(unknown) error = nil, want error")
	}

	if err := repo.UpdateStatus(ws.ID, "nonexistent", string(ProjectStatusActive), 500); !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateStatus(nonexistent) error = %v, want ErrNotFound", err)
	}
}
