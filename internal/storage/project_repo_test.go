package storage

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	domain "git.dajee.net/dajee/xuanchu/internal/task"
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

func TestProjectRepositoryTaskSummary(t *testing.T) {
	store, repo, ws := newProjectRepoTest(t)
	project, err := repo.Create(testProject("p-summary", ws.ID, "ops", 100))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	other, err := repo.Create(testProject("p-other", ws.ID, "other", 101))
	if err != nil {
		t.Fatalf("Create(other) error = %v", err)
	}

	now := int64(1_800_000_000)
	duePast := now - 3600
	dueFuture := now + 3600
	dueEqualNow := now
	waitReady := now
	waitFuture := now + 3600
	priorityH := "H"
	priorityM := "M"

	// users
	zhang := "user-zhang"
	lisi := "user-lisi"
	for _, u := range []struct {
		id, name, displayName string
	}{
		{zhang, "zhangsan", "张三"},
		{lisi, "lisi", "李四"},
	} {
		if err := store.DB().Create(&User{ID: u.id, Name: u.name, DisplayName: u.displayName}).Error; err != nil {
			t.Fatalf("create user %q: %v", u.id, err)
		}
	}

	// 项目内任务：
	// overdue-h: 逾期 + 高优 + 未分配
	insertTaskWithOpts(t, store, "overdue-h", ws.ID, project.ID, "ops", 1, "pending", taskOpts{Due: &duePast, Priority: &priorityH})
	// high-only: 高优但未逾期
	insertTaskWithOpts(t, store, "high-only", ws.ID, project.ID, "ops", 2, "pending", taskOpts{Priority: &priorityH})
	// wait-ready: 等待已到期，分配给张三
	insertTaskWithOpts(t, store, "wait-ready", ws.ID, project.ID, "ops", 3, "waiting", taskOpts{Wait: &waitReady, Priority: &priorityM, Assignees: []string{zhang}})
	// future: 未到期、未等待、分配给张三
	insertTaskWithOpts(t, store, "future", ws.ID, project.ID, "ops", 4, "pending", taskOpts{Due: &dueFuture, Assignees: []string{zhang}})
	// unassigned-open: 未分配、未到期、未等待
	insertTaskWithOpts(t, store, "unassigned-open", ws.ID, project.ID, "ops", 5, "pending", taskOpts{})
	// wait-future: 等待未到期
	insertTaskWithOpts(t, store, "wait-future", ws.ID, project.ID, "ops", 6, "waiting", taskOpts{Wait: &waitFuture, Assignees: []string{lisi}})
	// recurring-open: 已物化的循环实例，分配给张三；应进入成员待办，但不改变普通任务风险。
	seriesID := "series-daily"
	insertTaskWithOpts(t, store, "recurring-open", ws.ID, project.ID, "ops", 10, "pending", taskOpts{Due: &duePast, Assignees: []string{zhang}, SeriesID: &seriesID})
	// due-equal-now: due == now，按规则不计入 overdue
	insertTaskWithOpts(t, store, "due-equal-now", ws.ID, project.ID, "ops", 7, "pending", taskOpts{Due: &dueEqualNow})
	// done-overdue: 已完成且高优且逾期，不计入任何开放计数
	insertTaskWithOpts(t, store, "done-overdue", ws.ID, project.ID, "ops", 8, "completed", taskOpts{Due: &duePast, Priority: &priorityH})
	// deleted-overdue: 已删除，不计入
	insertTaskWithOpts(t, store, "deleted-overdue", ws.ID, project.ID, "ops", 9, "deleted", taskOpts{Due: &duePast})

	// 其他项目任务，不应被聚合进来
	insertTaskWithOpts(t, store, "other-overdue", ws.ID, other.ID, "other", 1, "pending", taskOpts{Due: &duePast, Priority: &priorityH})

	summary, err := repo.TaskSummary(ws.ID, project.ID, now)
	if err != nil {
		t.Fatalf("TaskSummary() error = %v", err)
	}

	// overdue: overdue-h、duePast+pending；due-equal-now 不算；done/deleted 不算
	if summary.OverdueCount != 1 {
		t.Fatalf("OverdueCount = %d, want 1", summary.OverdueCount)
	}
	if len(summary.OverdueRefs) != 1 || summary.OverdueRefs[0].TaskSlug != "ops-1" {
		t.Fatalf("OverdueRefs = %#v, want [ops-1]", summary.OverdueRefs)
	}

	// high priority open: overdue-h + high-only
	if summary.HighPriorityOpenCount != 2 {
		t.Fatalf("HighPriorityOpenCount = %d, want 2", summary.HighPriorityOpenCount)
	}
	if len(summary.HighPriorityOpenRefs) != 2 {
		t.Fatalf("HighPriorityOpenRefs = %#v, want 2 refs", summary.HighPriorityOpenRefs)
	}

	// wait ready: 仅 wait-ready（wait == now 计入；wait-future 不算）
	if summary.WaitReadyCount != 1 {
		t.Fatalf("WaitReadyCount = %d, want 1", summary.WaitReadyCount)
	}
	if len(summary.WaitReadyRefs) != 1 || summary.WaitReadyRefs[0].TaskSlug != "ops-3" {
		t.Fatalf("WaitReadyRefs = %#v, want [ops-3]", summary.WaitReadyRefs)
	}

	// unassigned open: overdue-h、high-only、unassigned-open、due-equal-now
	if summary.UnassignedOpenCount != 4 {
		t.Fatalf("UnassignedOpenCount = %d, want 4", summary.UnassignedOpenCount)
	}

	// 负责人负载：未分配任务、张三、李四
	if got := workloadByUserID(summary.Workload, ""); got == nil || got.OpenCount != 4 {
		t.Fatalf("unassigned OpenCount = %v, want 4", got)
	}
	if got := workloadByUserID(summary.Workload, zhang); got == nil || got.OpenCount != 3 {
		t.Fatalf("zhang OpenCount = %v, want 3（含 1 条循环实例）", got)
	}
	if got := workloadByUserID(summary.Workload, zhang); got != nil && got.OverdueCount != 1 {
		t.Fatalf("zhang OverdueCount = %d, want 1（循环实例逾期）", got.OverdueCount)
	}
	if got := workloadByUserID(summary.Workload, lisi); got == nil || got.OpenCount != 1 {
		t.Fatalf("lisi OpenCount = %v, want 1", got)
	}
}

type taskOpts struct {
	Due       *int64
	Wait      *int64
	Priority  *string
	Assignees []string
	SeriesID  *string
}

func insertTaskWithOpts(t *testing.T, store *Store, uuid, workspaceID, projectID, projectSlug string, seq int, status string, opts taskOpts) {
	t.Helper()
	task := Task{
		UUID:        uuid,
		WorkspaceID: workspaceID,
		Title:       "task " + uuid,
		Status:      status,
		Entry:       int64(100),
		Modified:    int64(100),
		Project:     &projectSlug,
		ProjectID:   &projectID,
		ProjectSeq:  ptrInt64(int64(seq)),
		Due:         opts.Due,
		Wait:        opts.Wait,
		Priority:    opts.Priority,
		SeriesID:    opts.SeriesID,
	}
	if err := store.DB().Create(&task).Error; err != nil {
		t.Fatalf("insert task %q: %v", uuid, err)
	}
	for _, userID := range opts.Assignees {
		if err := store.DB().Create(&TaskAssignee{TaskUUID: uuid, UserID: userID}).Error; err != nil {
			t.Fatalf("insert assignee for %q: %v", uuid, err)
		}
	}
}

func ptrInt64(v int64) *int64 {
	return &v
}

func workloadByUserID(rows []ProjectAssigneeWorkloadRow, userID string) *ProjectAssigneeWorkloadRow {
	for i := range rows {
		if rows[i].UserID == userID {
			return &rows[i]
		}
	}
	return nil
}

var _ = domain.StatusPending
