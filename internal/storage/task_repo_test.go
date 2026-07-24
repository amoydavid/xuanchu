package storage

import (
	"errors"
	"fmt"
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
		Title:       "write spec",
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
	if len(tasks) != 1 || tasks[0].Title != "write spec" {
		t.Fatalf("tasks = %#v", tasks)
	}
	if len(tasks[0].Tags) != 1 || tasks[0].Tags[0] != "planning" {
		t.Fatalf("tags = %#v", tasks[0].Tags)
	}
}

func TestTaskTemplateCandidatePageForcesNormalProjectScopeAndStablePagination(t *testing.T) {
	store, repo, ws := newTestRepo(t)
	projectID, otherProjectID := "project-source", "project-other"
	for _, project := range []Project{
		{ID: projectID, WorkspaceID: ws.ID, Slug: "source", Name: "Source", Status: "active", CreatedAt: 1, ModifiedAt: 1},
		{ID: otherProjectID, WorkspaceID: ws.ID, Slug: "other", Name: "Other", Status: "active", CreatedAt: 1, ModifiedAt: 1},
	} {
		if err := store.DB().Create(&project).Error; err != nil {
			t.Fatal(err)
		}
	}
	rows := make([]Task, 0, 123)
	for i := 0; i < 120; i++ {
		seq := int64(i + 1)
		pid := projectID
		rows = append(rows, Task{UUID: fmt.Sprintf("task-%03d", i), WorkspaceID: ws.ID, Title: "上线准备", Status: domain.StatusPending, Entry: 100, Modified: 100, ProjectID: &pid, ProjectSeq: &seq})
	}
	seriesID, recurrenceAt, rule := "series-1", int64(100), "daily"
	pid := projectID
	rows = append(rows,
		Task{UUID: "occurrence", WorkspaceID: ws.ID, Title: "上线准备", Status: domain.StatusPending, Entry: 100, Modified: 100, ProjectID: &pid, SeriesID: &seriesID, RecurrenceAt: &recurrenceAt, RecurrenceRuleSnapshot: &rule},
		Task{UUID: "deleted", WorkspaceID: ws.ID, Title: "上线准备", Status: domain.StatusDeleted, Entry: 100, Modified: 100, ProjectID: &pid},
	)
	otherPID := otherProjectID
	rows = append(rows, Task{UUID: "other-project", WorkspaceID: ws.ID, Title: "上线准备", Status: domain.StatusPending, Entry: 100, Modified: 100, ProjectID: &otherPID})
	if err := store.DB().Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	page, err := repo.ListCandidatePage(TaskCandidateListOptions{
		WorkspaceID: ws.ID, ProjectID: projectID, Q: "上线", Status: "all", Sort: "entry",
	}, 50, 50)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 120 || len(page.Items) != 50 || page.Limit != 50 || page.Offset != 50 {
		t.Fatalf("page = %#v", page)
	}
	if page.Items[0].UUID != "task-050" || page.Items[49].UUID != "task-099" {
		t.Fatalf("stable page refs = %q...%q", page.Items[0].UUID, page.Items[49].UUID)
	}
	for _, item := range page.Items {
		if item.ProjectID == nil || *item.ProjectID != projectID || item.SeriesID != nil || item.Status == domain.StatusDeleted {
			t.Fatalf("candidate scope leaked: %#v", item)
		}
	}
}

func TestTaskRepositoryRoundTripsTitleAndOptionalDescription(t *testing.T) {
	_, repo, ws := newTestRepo(t)
	detail := "详细描述"

	created, err := repo.Create(domain.Task{
		UUID:        "task-title-description",
		WorkspaceID: ws.ID,
		Title:       "任务标题",
		Description: &detail,
		Status:      domain.StatusPending,
		Entry:       100,
		Modified:    100,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.Title != "任务标题" || created.Description == nil || *created.Description != detail {
		t.Fatalf("created text fields = title %q description %#v", created.Title, created.Description)
	}

	got, err := repo.GetByUUID(ws.ID, "task-title-description")
	if err != nil {
		t.Fatalf("GetByUUID() error = %v", err)
	}
	if got.Title != "任务标题" || got.Description == nil || *got.Description != detail {
		t.Fatalf("got text fields = title %q description %#v", got.Title, got.Description)
	}

	got.Description = nil
	got.Modified = 200
	if err := repo.Update(got); err != nil {
		t.Fatalf("Update(clear description) error = %v", err)
	}
	cleared, err := repo.GetByUUID(ws.ID, "task-title-description")
	if err != nil {
		t.Fatalf("GetByUUID(cleared) error = %v", err)
	}
	if cleared.Title != "任务标题" || cleared.Description != nil {
		t.Fatalf("cleared text fields = title %q description %#v", cleared.Title, cleared.Description)
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
		UUID: "task-1", WorkspaceID: ws.ID, Title: "write spec",
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
		Title:       "write spec",
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
		Title:       "write spec",
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
	parent := "parent-uuid"
	tsk := domain.Task{
		UUID: "u1", WorkspaceID: ws.ID, Title: "m2 task", Status: domain.StatusPending,
		Entry: 1, Modified: 2, Start: &start, Wait: &wait, Scheduled: &scheduled, Until: &until,
		Annotations: []domain.Annotation{{Entry: 3, Description: "note"}},
		Depends:     []string{"dep-1", "dep-2"},
		Parent:      &parent,
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
	if got.Parent == nil || *got.Parent != parent {
		t.Fatalf("parent field not roundtripped: %#v", got.Parent)
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
		Title:       "project task",
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

func TestTaskRepositoryPersistsProjectSeq(t *testing.T) {
	store, repo, ws := newTestRepo(t)
	projectRepo := NewProjectRepository(store.DB())
	project, err := projectRepo.Create(testProject("project-api", ws.ID, "api", 100))
	if err != nil {
		t.Fatalf("Create(project) error = %v", err)
	}

	projectSlug := "api"
	seq := int64(7)
	created, err := repo.Create(domain.Task{
		UUID:        "task-project-seq",
		WorkspaceID: ws.ID,
		Title:       "project task",
		Status:      domain.StatusPending,
		Entry:       100,
		Modified:    100,
		Project:     &projectSlug,
		ProjectID:   &project.ID,
		ProjectSeq:  &seq,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.ProjectSeq == nil || *created.ProjectSeq != 7 {
		t.Fatalf("created ProjectSeq = %#v, want 7", created.ProjectSeq)
	}

	got, err := repo.GetByUUID(ws.ID, created.UUID)
	if err != nil {
		t.Fatalf("GetByUUID() error = %v", err)
	}
	if got.ProjectSeq == nil || *got.ProjectSeq != 7 {
		t.Fatalf("got ProjectSeq = %#v, want 7", got.ProjectSeq)
	}

	nextSeq := int64(8)
	got.ProjectSeq = &nextSeq
	got.Modified = 200
	if err := repo.Update(got); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	updated, err := repo.GetByUUID(ws.ID, created.UUID)
	if err != nil {
		t.Fatalf("GetByUUID() after update error = %v", err)
	}
	if updated.ProjectSeq == nil || *updated.ProjectSeq != 8 {
		t.Fatalf("updated ProjectSeq = %#v, want 8", updated.ProjectSeq)
	}
}

func TestTaskRepositoryGetByProjectSeq(t *testing.T) {
	store, repo, ws := newTestRepo(t)
	projectRepo := NewProjectRepository(store.DB())
	api, err := projectRepo.Create(testProject("project-api", ws.ID, "api", 100))
	if err != nil {
		t.Fatalf("Create(api project) error = %v", err)
	}
	other := createTestWorkspace(t, store, "team")
	otherProject, err := projectRepo.Create(testProject("project-other", other.ID, "api", 100))
	if err != nil {
		t.Fatalf("Create(other project) error = %v", err)
	}
	apiSlug := "api"
	seq := int64(1)
	if _, err := repo.Create(domain.Task{
		UUID:        "task-api-1",
		WorkspaceID: ws.ID,
		Title:       "api task",
		Status:      domain.StatusPending,
		Entry:       100,
		Modified:    100,
		Project:     &apiSlug,
		ProjectID:   &api.ID,
		ProjectSeq:  &seq,
	}); err != nil {
		t.Fatalf("Create(api task) error = %v", err)
	}
	if _, err := repo.Create(domain.Task{
		UUID:        "task-other-1",
		WorkspaceID: other.ID,
		Title:       "other task",
		Status:      domain.StatusPending,
		Entry:       100,
		Modified:    100,
		Project:     &apiSlug,
		ProjectID:   &otherProject.ID,
		ProjectSeq:  &seq,
	}); err != nil {
		t.Fatalf("Create(other task) error = %v", err)
	}

	got, err := repo.GetByProjectSeq(ws.ID, api.ID, 1)
	if err != nil {
		t.Fatalf("GetByProjectSeq() error = %v", err)
	}
	if got.UUID != "task-api-1" {
		t.Fatalf("GetByProjectSeq() UUID = %s, want task-api-1", got.UUID)
	}
	if _, err := repo.GetByProjectSeq(ws.ID, api.ID, 2); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByProjectSeq(missing) error = %v, want ErrNotFound", err)
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
		Title:       "project task",
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
INSERT INTO tasks(uuid, workspace_id, title, status, entry, modified, project, project_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"cross-project-task", ws.ID, "bad project", domain.StatusPending, int64(1), int64(1), "api", project.ID,
	).Error
	if err == nil {
		t.Fatal("raw insert with cross-workspace project_id succeeded, want foreign key rejection")
	}
}

func TestTaskRepositoryAddAnnotationAppendsWithoutReplacingExisting(t *testing.T) {
	_, repo, ws := newTestRepo(t)
	if _, err := repo.Create(domain.Task{
		UUID:        "task-1",
		WorkspaceID: ws.ID,
		Title:       "annotated",
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

func TestTaskRepositoryUpdatePreservesAnnotationActor(t *testing.T) {
	store, repo, ws := newTestRepo(t)
	created, err := repo.Create(domain.Task{
		UUID: "task-actor", WorkspaceID: ws.ID, Title: "before",
		Status: domain.StatusPending, Entry: 100, Modified: 100,
		Annotations: []domain.Annotation{{ID: "annotation-actor", Entry: 101, Description: "note"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	userID := "user-original"
	if err := store.DB().Model(&TaskAnnotation{}).Where("id = ?", "annotation-actor").Updates(map[string]any{
		"created_by_actor_type": "user",
		"created_by_user_id":    userID,
		"created_at":            int64(101),
	}).Error; err != nil {
		t.Fatal(err)
	}

	created.Title = "after"
	created.Modified = 102
	if err := repo.Update(created); err != nil {
		t.Fatal(err)
	}

	var row TaskAnnotation
	if err := store.DB().Where("id = ?", "annotation-actor").First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.CreatedByActorType != "user" || row.CreatedByUserID == nil || *row.CreatedByUserID != userID {
		t.Fatalf("annotation actor after task update = %#v", row)
	}
	if row.CreatedAt != 101 {
		t.Fatalf("CreatedAt = %d, want 101", row.CreatedAt)
	}
}

func TestTaskRepositoryListAnnotationsPagination(t *testing.T) {
	_, repo, ws := newTestRepo(t)
	if _, err := repo.Create(domain.Task{
		UUID:        "task-1",
		WorkspaceID: ws.ID,
		Title:       "annotated",
		Status:      domain.StatusPending,
		Entry:       100,
		Modified:    100,
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	// 写入 7 条注解，entry 递增。ID 留空，由 AddAnnotation 自动生成。
	for i := 0; i < 7; i++ {
		entry := int64(200 + i)
		if err := repo.AddAnnotation(ws.ID, "task-1", domain.Annotation{
			Entry:       entry,
			Description: fmt.Sprintf("note-%d", i),
		}, entry); err != nil {
			t.Fatalf("AddAnnotation(%d) error = %v", i, err)
		}
	}

	// offset=3 limit=2 应返回 entry 倒序的第 4、3 条（note-3、note-2），total=7。
	got, total, err := repo.ListAnnotations(ws.ID, "task-1", 3, 2)
	if err != nil {
		t.Fatalf("ListAnnotations() error = %v", err)
	}
	if total != 7 {
		t.Fatalf("total = %d, want 7", total)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	// entry 倒序：note-3 (entry 203) 在前。
	if got[0].Description != "note-3" || got[1].Description != "note-2" {
		t.Fatalf("order = %#v", got)
	}

	// 越界 offset 返回空但 total 仍为 7。
	got, total, err = repo.ListAnnotations(ws.ID, "task-1", 100, 10)
	if err != nil {
		t.Fatalf("ListAnnotations(oob) error = %v", err)
	}
	if total != 7 || len(got) != 0 {
		t.Fatalf("oob: total=%d len=%d", total, len(got))
	}
}

func TestTaskRepositoryListAnnotationActivityCursor(t *testing.T) {
	store, repo, ws := newTestRepo(t)
	if _, err := repo.Create(domain.Task{
		UUID: "task-activity", WorkspaceID: ws.ID, Title: "activity",
		Status: domain.StatusPending, Entry: 1, Modified: 1,
	}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b", "c"} {
		if err := store.DB().Create(&TaskAnnotation{
			ID: id, TaskUUID: "task-activity", Entry: 100, Description: "note-" + id,
			CreatedByActorType: "unknown", CreatedAt: 100,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	cursorID := "c"
	rows, err := repo.ListAnnotationActivity(ws.ID, "task-activity", &TaskAnnotationListCursor{CreatedAt: 100, ID: &cursorID}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].ID != "b" || rows[1].ID != "a" {
		t.Fatalf("rows = %#v, want b,a", rows)
	}
}

func TestTaskRepositoryListByUUIDs(t *testing.T) {
	_, repo, ws := newTestRepo(t)
	if _, err := repo.Create(domain.Task{
		UUID: "t-a", WorkspaceID: ws.ID, Title: "alpha",
		Status: domain.StatusPending, Entry: 100, Modified: 100,
	}); err != nil {
		t.Fatalf("Create(a) error = %v", err)
	}
	if _, err := repo.Create(domain.Task{
		UUID: "t-b", WorkspaceID: ws.ID, Title: "beta",
		Status: domain.StatusPending, Entry: 100, Modified: 100,
	}); err != nil {
		t.Fatalf("Create(b) error = %v", err)
	}
	// deleted 任务不应返回
	if _, err := repo.Create(domain.Task{
		UUID: "t-c", WorkspaceID: ws.ID, Title: "gamma",
		Status: domain.StatusDeleted, Entry: 100, Modified: 100,
	}); err != nil {
		t.Fatalf("Create(c) error = %v", err)
	}

	// 查存在的 + 不存在的 + deleted 的，只应返回存在的活任务。
	got, err := repo.ListByUUIDs(ws.ID, []string{"t-a", "t-b", "missing", "t-c"})
	if err != nil {
		t.Fatalf("ListByUUIDs() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (deleted/missing excluded)", len(got))
	}
	descs := make(map[string]bool, len(got))
	for _, tsk := range got {
		descs[tsk.Title] = true
	}
	if !descs["alpha"] || !descs["beta"] {
		t.Fatalf("results = %#v, want alpha+beta", got)
	}

	// 空 uuids 不报错。
	if got, err := repo.ListByUUIDs(ws.ID, nil); err != nil || got != nil {
		t.Fatalf("empty: got=%v err=%v", got, err)
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
		UUID: uuid, WorkspaceID: wsID, Title: desc,
		Status: domain.StatusPending, Entry: 100, Modified: 100,
	}
}

func TestTaskRepositoryListDependents(t *testing.T) {
	_, repo, ws := newTestRepo(t)
	// a 被 b、c 依赖（b.depends=[a], c.depends=[a]）；d 不依赖任何。
	a, err := repo.Create(mkDomainTask("t-a", ws.ID, "alpha"))
	if err != nil {
		t.Fatalf("Create(a) error = %v", err)
	}
	if _, err := repo.Create(domain.Task{
		UUID: "t-b", WorkspaceID: ws.ID, Title: "beta",
		Status: domain.StatusPending, Entry: 100, Modified: 100,
		Depends: []string{a.UUID},
	}); err != nil {
		t.Fatalf("Create(b) error = %v", err)
	}
	if _, err := repo.Create(domain.Task{
		UUID: "t-c", WorkspaceID: ws.ID, Title: "gamma",
		Status: domain.StatusPending, Entry: 100, Modified: 100,
		Depends: []string{a.UUID},
	}); err != nil {
		t.Fatalf("Create(c) error = %v", err)
	}
	if _, err := repo.Create(mkDomainTask("t-d", ws.ID, "delta")); err != nil {
		t.Fatalf("Create(d) error = %v", err)
	}

	// a 阻塞了 b 和 c。
	got, err := repo.ListDependents(ws.ID, a.UUID)
	if err != nil {
		t.Fatalf("ListDependents() error = %v", err)
	}
	descs := make(map[string]bool, len(got))
	for _, tsk := range got {
		descs[tsk.Title] = true
	}
	if len(got) != 2 || !descs["beta"] || !descs["gamma"] {
		t.Fatalf("ListDependents(a) = %#v, want beta+gamma", got)
	}

	// d 不阻塞任何任务。
	got, err = repo.ListDependents(ws.ID, "t-d")
	if err != nil {
		t.Fatalf("ListDependents(d) error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ListDependents(d) = %#v, want empty", got)
	}
}

func TestTaskRepositorySearchTasksByTitleOrSlug(t *testing.T) {
	store, repo, ws := newTestRepo(t)
	projectID := "proj-search"
	otherProjectID := "other-project"
	// 预置两个 project，避免 task.project_id 外键约束失败。
	for _, p := range []Project{
		{ID: projectID, WorkspaceID: ws.ID, Slug: "search-proj", Name: "Search Project", Status: "active", SettingsJSON: "{}", CreatedAt: 1, ModifiedAt: 1},
		{ID: otherProjectID, WorkspaceID: ws.ID, Slug: "other-proj", Name: "Other Project", Status: "active", SettingsJSON: "{}", CreatedAt: 1, ModifiedAt: 1},
	} {
		if err := store.DB().Create(&p).Error; err != nil {
			t.Fatalf("create project: %v", err)
		}
	}
	// 当前项目里的任务。
	mustCreateTaskForSearch(t, repo, ws.ID, "t-title", "Implement API", projectID)
	mustCreateTaskForSearch(t, repo, ws.ID, "t-other", "Fix bug", projectID)
	// 另一个项目的任务。
	mustCreateTaskForSearch(t, repo, ws.ID, "t-cross", "Implement docs", otherProjectID)

	// 按 title 匹配。
	got, err := repo.SearchTasksByTitleOrSlug(ws.ID, "API", projectID, 20)
	if err != nil {
		t.Fatalf("SearchTasksByTitleOrSlug: %v", err)
	}
	if len(got) != 1 || got[0].Title != "Implement API" {
		t.Fatalf("title search got = %#v", got)
	}

	// 跨项目查询，当前项目优先排序。
	got, _ = repo.SearchTasksByTitleOrSlug(ws.ID, "Implement", projectID, 20)
	if len(got) != 2 {
		t.Fatalf("got = %#v", got)
	}
	if got[0].ProjectID == nil || *got[0].ProjectID != projectID {
		t.Fatalf("current project task should rank first: %#v", got[0])
	}
	if got[1].ProjectID == nil || *got[1].ProjectID != otherProjectID {
		t.Fatalf("other project task should rank second: %#v", got[1])
	}

	// 不返回其它 workspace 的任务。
	got, _ = repo.SearchTasksByTitleOrSlug("other-ws", "Implement", projectID, 20)
	if len(got) != 0 {
		t.Fatalf("leaked other-workspace tasks: %#v", got)
	}
}

func mustCreateTaskForSearch(t *testing.T, repo *TaskRepository, wsID, taskUUID, title, projectID string) {
	t.Helper()
	tsk := domain.Task{
		UUID:        taskUUID,
		WorkspaceID: wsID,
		Title:       title,
		Status:      domain.StatusPending,
		Entry:       1,
		Modified:    1,
	}
	tsk.ProjectID = &projectID
	if _, err := repo.Create(tsk); err != nil {
		t.Fatalf("Create: %v", err)
	}
}
