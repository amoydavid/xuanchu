package storage

import (
	"path/filepath"
	"slices"
	"testing"

	domain "git.dajee.net/dajee/xuanchu/internal/task"
)

func newTestRepo(t *testing.T) (*Store, *TaskRepository, Workspace) {
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
	return store, NewTaskRepository(store.DB()), ws
}

func TestTaskRepositoryCreateAndList(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ws, _ := store.LocalWorkspace()
	repo := NewTaskRepository(store.DB())

	created, err := repo.Create(domain.Task{
		UUID:        "task-1",
		WorkspaceID: ws.ID,
		Description: "write spec",
		Status:      domain.StatusPending,
		Entry:       100,
		Modified:    100,
		Tags:        []string{"planning"},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.UUID != "task-1" {
		t.Fatalf("UUID = %q", created.UUID)
	}

	tasks, err := repo.List(ws.ID, ListOptions{Status: domain.StatusPending})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tasks) != 1 || tasks[0].Description != "write spec" {
		t.Fatalf("tasks = %#v", tasks)
	}
	if len(tasks[0].Tags) != 1 || tasks[0].Tags[0] != "planning" {
		t.Fatalf("tags = %#v", tasks[0].Tags)
	}
}

func TestTaskRepositoryUpdateReplacesTags(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ws, _ := store.LocalWorkspace()
	repo := NewTaskRepository(store.DB())

	_, err = repo.Create(domain.Task{
		UUID: "task-1", WorkspaceID: ws.ID, Description: "write spec",
		Status: domain.StatusPending, Entry: 100, Modified: 100,
		Tags: []string{"old"},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	tsk, err := repo.GetByUUID(ws.ID, "task-1")
	if err != nil {
		t.Fatalf("GetByUUID() error = %v", err)
	}
	tsk.Tags = []string{"new"}
	tsk.Modified = 200
	if err := repo.Update(tsk); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got, err := repo.GetByUUID(ws.ID, "task-1")
	if err != nil {
		t.Fatalf("GetByUUID() after update error = %v", err)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "new" {
		t.Fatalf("Tags = %#v", got.Tags)
	}
}

func TestTaskRepositoryCreateAndGetAssignees(t *testing.T) {
	store, repo, ws := newTestRepo(t)
	createTestUser(t, store, User{ID: "user-bob", Name: "bob", CreatedAt: 100, ModifiedAt: 100})
	createTestUser(t, store, User{ID: "user-alice", Name: "alice", CreatedAt: 100, ModifiedAt: 100, Email: stringPtr("alice@example.com")})

	created, err := repo.Create(domain.Task{
		UUID:        "task-assignees",
		WorkspaceID: ws.ID,
		Description: "write spec",
		Status:      domain.StatusPending,
		Entry:       100,
		Modified:    100,
		Assignees: []domain.AssigneeInfo{
			{UserID: "user-bob"},
			{UserID: "user-alice"},
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if len(created.Assignees) != 2 {
		t.Fatalf("created assignees = %#v, want 2 entries", created.Assignees)
	}

	got, err := repo.GetByUUID(ws.ID, "task-assignees")
	if err != nil {
		t.Fatalf("GetByUUID() error = %v", err)
	}
	if len(got.Assignees) != 2 {
		t.Fatalf("Assignees = %#v, want 2 entries", got.Assignees)
	}
	if got.Assignees[0].UserID != "user-alice" || got.Assignees[0].Name != "alice" || got.Assignees[0].Email == nil || *got.Assignees[0].Email != "alice@example.com" {
		t.Fatalf("first assignee = %#v", got.Assignees[0])
	}
	if got.Assignees[1].UserID != "user-bob" || got.Assignees[1].Name != "bob" || got.Assignees[1].Email != nil {
		t.Fatalf("second assignee = %#v", got.Assignees[1])
	}
}

func TestTaskRepositoryUpdateAssignees(t *testing.T) {
	store, repo, ws := newTestRepo(t)
	createTestUser(t, store, User{ID: "user-alice", Name: "alice", CreatedAt: 100, ModifiedAt: 100})
	createTestUser(t, store, User{ID: "user-bob", Name: "bob", CreatedAt: 100, ModifiedAt: 100})
	createTestUser(t, store, User{ID: "user-carol", Name: "carol", CreatedAt: 100, ModifiedAt: 100, Email: stringPtr("carol@example.com")})

	if _, err := repo.Create(domain.Task{
		UUID:        "task-update-assignees",
		WorkspaceID: ws.ID,
		Description: "write spec",
		Status:      domain.StatusPending,
		Entry:       100,
		Modified:    100,
		Assignees: []domain.AssigneeInfo{
			{UserID: "user-bob"},
			{UserID: "user-alice"},
		},
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	tsk, err := repo.GetByUUID(ws.ID, "task-update-assignees")
	if err != nil {
		t.Fatalf("GetByUUID() before update error = %v", err)
	}
	tsk.Assignees = []domain.AssigneeInfo{
		{UserID: "user-carol"},
	}
	tsk.Modified = 200
	if err := repo.Update(tsk); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got, err := repo.GetByUUID(ws.ID, "task-update-assignees")
	if err != nil {
		t.Fatalf("GetByUUID() after update error = %v", err)
	}
	if len(got.Assignees) != 1 {
		t.Fatalf("Assignees = %#v, want 1 entry", got.Assignees)
	}
	if got.Assignees[0].UserID != "user-carol" || got.Assignees[0].Name != "carol" || got.Assignees[0].Email == nil || *got.Assignees[0].Email != "carol@example.com" {
		t.Fatalf("assignee after update = %#v", got.Assignees[0])
	}
}

func TestTaskRepositoryPersistsM2Fields(t *testing.T) {
	_, repo, ws := newTestRepo(t)

	start, wait, scheduled, until := int64(10), int64(20), int64(30), int64(40)
	recur, parent, mask := "weekly", "parent-uuid", "mask"
	imask := 1
	tsk := domain.Task{
		UUID: "u1", WorkspaceID: ws.ID, Description: "m2 task", Status: domain.StatusPending,
		Entry: 1, Modified: 2, Start: &start, Wait: &wait, Scheduled: &scheduled, Until: &until,
		Annotations: []domain.Annotation{{Entry: 3, Description: "note"}},
		Depends:     []string{"dep-1", "dep-2"},
		Recur:       &recur, Parent: &parent, Mask: &mask, IMask: &imask,
	}
	if _, err := repo.Create(tsk); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	got, err := repo.GetByUUID(ws.ID, "u1")
	if err != nil {
		t.Fatalf("GetByUUID() error = %v", err)
	}
	if got.Start == nil || *got.Start != start || got.Wait == nil || *got.Wait != wait || got.Scheduled == nil || *got.Scheduled != scheduled || got.Until == nil || *got.Until != until {
		t.Fatalf("M2 date fields not roundtripped: %#v", got)
	}
	if len(got.Annotations) != 1 || got.Annotations[0].Description != "note" {
		t.Fatalf("Annotations = %#v", got.Annotations)
	}
	if !slices.Equal(got.Depends, []string{"dep-1", "dep-2"}) {
		t.Fatalf("Depends = %#v", got.Depends)
	}
	if got.Recur == nil || *got.Recur != recur || got.Parent == nil || *got.Parent != parent || got.Mask == nil || *got.Mask != mask || got.IMask == nil || *got.IMask != imask {
		t.Fatalf("recurrence fields not roundtripped: %#v", got)
	}
}

func TestTaskRepositoryCreateUpdateAndListProjectID(t *testing.T) {
	store, repo, ws := newTestRepo(t)
	projectRepo := NewProjectRepository(store.DB())
	api, err := projectRepo.Create(testProject("project-api", ws.ID, "api", 100))
	if err != nil {
		t.Fatalf("Create(api project) error = %v", err)
	}
	web, err := projectRepo.Create(testProject("project-web", ws.ID, "web", 101))
	if err != nil {
		t.Fatalf("Create(web project) error = %v", err)
	}

	projectSlug := "api"
	created, err := repo.Create(domain.Task{
		UUID:        "task-project",
		WorkspaceID: ws.ID,
		Description: "project task",
		Status:      domain.StatusPending,
		Entry:       1,
		Modified:    1,
		Project:     &projectSlug,
		ProjectID:   &api.ID,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.ProjectID == nil || *created.ProjectID != api.ID {
		t.Fatalf("created ProjectID = %#v, want %q", created.ProjectID, api.ID)
	}

	webSlug := "web"
	created.Project = &webSlug
	created.ProjectID = &web.ID
	created.Modified = 2
	if err := repo.Update(created); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got, err := repo.GetByUUID(ws.ID, "task-project")
	if err != nil {
		t.Fatalf("GetByUUID() error = %v", err)
	}
	if got.ProjectID == nil || *got.ProjectID != web.ID {
		t.Fatalf("got ProjectID = %#v, want %q", got.ProjectID, web.ID)
	}

	listed, err := repo.List(ws.ID, ListOptions{Status: domain.StatusPending})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(listed) != 1 || listed[0].ProjectID == nil || *listed[0].ProjectID != web.ID {
		t.Fatalf("listed tasks = %#v, want ProjectID %q", listed, web.ID)
	}
}

func TestTaskRepositoryUpdateClearsProjectID(t *testing.T) {
	store, repo, ws := newTestRepo(t)
	projectRepo := NewProjectRepository(store.DB())
	project, err := projectRepo.Create(testProject("project-api", ws.ID, "api", 100))
	if err != nil {
		t.Fatalf("Create(project) error = %v", err)
	}

	projectSlug := "api"
	created, err := repo.Create(domain.Task{
		UUID:        "task-clear-project",
		WorkspaceID: ws.ID,
		Description: "project task",
		Status:      domain.StatusPending,
		Entry:       1,
		Modified:    1,
		Project:     &projectSlug,
		ProjectID:   &project.ID,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	created.Project = nil
	created.ProjectID = nil
	created.Modified = 2
	if err := repo.Update(created); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got, err := repo.GetByUUID(ws.ID, "task-clear-project")
	if err != nil {
		t.Fatalf("GetByUUID() error = %v", err)
	}
	if got.Project != nil || got.ProjectID != nil {
		t.Fatalf("got project = (%#v, %#v), want NULLs", got.Project, got.ProjectID)
	}
}

func TestTaskRepositoryForeignKeyRejectsCrossWorkspaceProjectID(t *testing.T) {
	store, _, ws := newTestRepo(t)
	projectRepo := NewProjectRepository(store.DB())
	other := createTestWorkspace(t, store, "team")
	project, err := projectRepo.Create(testProject("project-other", other.ID, "api", 100))
	if err != nil {
		t.Fatalf("Create(other project) error = %v", err)
	}

	err = store.DB().Exec(`
INSERT INTO tasks(uuid, workspace_id, description, status, entry, modified, project, project_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"cross-project-task", ws.ID, "bad project", domain.StatusPending, int64(1), int64(1), "api", project.ID,
	).Error
	if err == nil {
		t.Fatal("raw insert with cross-workspace project_id succeeded, want foreign key rejection")
	}
}

func TestTaskRepositoryRecurringChildUniqueByParentAndDue(t *testing.T) {
	_, repo, ws := newTestRepo(t)
	parent := "parent"
	recur := "daily"
	due := int64(100)
	first, existing, err := repo.CreateRecurringChild(domain.Task{
		UUID: "child-1", WorkspaceID: ws.ID, Description: "child", Status: domain.StatusPending,
		Entry: 1, Modified: 1, Due: &due, Parent: &parent, Recur: &recur,
	})
	if err != nil {
		t.Fatalf("CreateRecurringChild(first) error = %v", err)
	}
	if existing {
		t.Fatal("CreateRecurringChild(first) existing = true, want false")
	}
	second, existing, err := repo.CreateRecurringChild(domain.Task{
		UUID: "child-2", WorkspaceID: ws.ID, Description: "child", Status: domain.StatusPending,
		Entry: 2, Modified: 2, Due: &due, Parent: &parent, Recur: &recur,
	})
	if err != nil {
		t.Fatalf("CreateRecurringChild(second) error = %v", err)
	}
	if !existing {
		t.Fatal("CreateRecurringChild(second) existing = false, want true")
	}
	if second.UUID != first.UUID {
		t.Fatalf("second UUID = %q, want existing %q", second.UUID, first.UUID)
	}
}

func TestTaskRepositoryAddAnnotationAppendsWithoutReplacingExisting(t *testing.T) {
	_, repo, ws := newTestRepo(t)
	if _, err := repo.Create(domain.Task{
		UUID:        "task-1",
		WorkspaceID: ws.ID,
		Description: "annotated",
		Status:      domain.StatusPending,
		Entry:       100,
		Modified:    100,
		Annotations: []domain.Annotation{{Entry: 101, Description: "first"}},
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := repo.AddAnnotation(ws.ID, "task-1", domain.Annotation{Entry: 102, Description: "second"}, 102); err != nil {
		t.Fatalf("AddAnnotation() error = %v", err)
	}
	got, err := repo.GetByUUID(ws.ID, "task-1")
	if err != nil {
		t.Fatalf("GetByUUID() error = %v", err)
	}
	if len(got.Annotations) != 2 {
		t.Fatalf("Annotations = %#v, want both entries", got.Annotations)
	}
	if got.Annotations[0].Description != "first" || got.Annotations[1].Description != "second" {
		t.Fatalf("Annotations order/content = %#v", got.Annotations)
	}
}

func createTestUser(t *testing.T, store *Store, user User) {
	t.Helper()
	if err := store.DB().Create(&user).Error; err != nil {
		t.Fatalf("create user %q: %v", user.ID, err)
	}
}

func stringPtr(value string) *string {
	return &value
}

func mkDomainTask(uuid, wsID, desc string) domain.Task {
	return domain.Task{
		UUID: uuid, WorkspaceID: wsID, Description: desc,
		Status: domain.StatusPending, Entry: 100, Modified: 100,
	}
}
