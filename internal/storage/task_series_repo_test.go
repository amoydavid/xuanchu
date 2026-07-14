package storage

import (
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/taskseries"
)

// newTaskSeriesRepoFixture 建立一个含 workspace + project + user 的测试 fixture。
func newTaskSeriesRepoFixture(t *testing.T) (*Store, Workspace, Project, taskseries.Series) {
	t.Helper()
	store, _, ws := newTestRepo(t)
	proj := Project{
		ID: "project-1", WorkspaceID: ws.ID, Slug: "ops", Name: "Ops",
		Status: "active", CreatedAt: 100, ModifiedAt: 100,
	}
	if err := store.DB().Create(&proj).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	desc := "每日巡检"
	series := taskseries.Series{
		WorkspaceID: ws.ID, ProjectID: proj.ID, Title: "每日巡检",
		Description: &desc, Status: taskseries.StatusActive, RecurrenceRule: "daily",
		FirstDue: 1000, CreatedBy: "user-1", CreatedAt: 100, ModifiedAt: 100,
		AssigneeIDs: []string{"user-1"}, Tags: []string{"ops", "daily"},
		UDAs: map[string]string{"cost_center": "ops"},
	}
	repo := NewTaskSeriesRepository(store.DB())
	created, err := repo.Create(series)
	if err != nil {
		t.Fatalf("Create series: %v", err)
	}
	return store, ws, proj, created
}

func TestTaskSeriesRepositoryCreateAndGet(t *testing.T) {
	store, ws, _, created := newTaskSeriesRepoFixture(t)
	repo := NewTaskSeriesRepository(store.DB())

	got, err := repo.Get(ws.ID, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Title != "每日巡检" || got.Status != taskseries.StatusActive || got.RecurrenceRule != "daily" {
		t.Fatalf("series round-trip 失败: %#v", got)
	}
	if got.FirstDue != 1000 {
		t.Fatalf("FirstDue=%d want 1000", got.FirstDue)
	}
	// 初始 rule version 应存在，effective_from == first_due。
	if len(got.RuleVersions) != 1 {
		t.Fatalf("rule versions len=%d want 1: %#v", len(got.RuleVersions), got.RuleVersions)
	}
	if got.RuleVersions[0].EffectiveFrom != 1000 || got.RuleVersions[0].RecurrenceRule != "daily" {
		t.Fatalf("initial rule version = %#v", got.RuleVersions[0])
	}
	// 关联字段 round-trip。
	if len(got.AssigneeIDs) != 1 || got.AssigneeIDs[0] != "user-1" {
		t.Fatalf("assignees = %#v", got.AssigneeIDs)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "daily" || got.Tags[1] != "ops" {
		t.Fatalf("tags = %#v", got.Tags)
	}
	if got.UDAs["cost_center"] != "ops" {
		t.Fatalf("udas = %#v", got.UDAs)
	}
}

func TestTaskSeriesRepositoryAllowsMultipleActiveSeriesPerProject(t *testing.T) {
	store, ws, project, _ := newTaskSeriesRepoFixture(t)
	repo := NewTaskSeriesRepository(store.DB())
	created, err := repo.Create(taskseries.Series{
		WorkspaceID: ws.ID, ProjectID: project.ID, Title: "第二条循环任务",
		Status: taskseries.StatusActive, RecurrenceRule: "weekly", FirstDue: 2000,
		CreatedBy: "user-1", CreatedAt: 200, ModifiedAt: 200,
	})
	if err != nil {
		t.Fatalf("Create second active series: %v", err)
	}
	if created.ID == "" {
		t.Fatal("second active series ID is empty")
	}
	rows, err := repo.ListCandidates(TaskSeriesListOptions{
		WorkspaceID: ws.ID, ProjectID: project.ID, Status: taskseries.StatusActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("active series = %d, want 2", len(rows))
	}
}

func TestTaskSeriesRepositoryGetReturnsErrForMissing(t *testing.T) {
	store, ws, _, _ := newTaskSeriesRepoFixture(t)
	repo := NewTaskSeriesRepository(store.DB())
	if _, err := repo.Get(ws.ID, "nonexistent"); err != ErrSeriesNotFound {
		t.Fatalf("Get(missing) err = %v, want ErrSeriesNotFound", err)
	}
}

func TestTaskSeriesRepositoryListCandidatesFilters(t *testing.T) {
	store, ws, proj, _ := newTaskSeriesRepoFixture(t)
	repo := NewTaskSeriesRepository(store.DB())

	// 第二个 series，stopped 状态。
	end := int64(5000)
	reason := "user_stopped"
	if _, err := repo.Create(taskseries.Series{
		WorkspaceID: ws.ID, ProjectID: proj.ID, Title: "周报",
		Status: taskseries.StatusStopped, RecurrenceRule: "weekly", FirstDue: 2000,
		EffectiveEndAt: &end, StopReason: &reason,
		CreatedBy: "u2", CreatedAt: 200, ModifiedAt: 200,
	}); err != nil {
		t.Fatalf("Create second: %v", err)
	}

	// status=active 只返回第一个。
	active, err := repo.ListCandidates(TaskSeriesListOptions{WorkspaceID: ws.ID, ProjectID: proj.ID, Status: "active"})
	if err != nil {
		t.Fatalf("ListCandidates active: %v", err)
	}
	if len(active) != 1 || active[0].Title != "每日巡检" {
		t.Fatalf("active candidates = %#v", active)
	}

	// status=all 返回两个，按 id 升序。
	all, err := repo.ListCandidates(TaskSeriesListOptions{WorkspaceID: ws.ID, Status: "all"})
	if err != nil {
		t.Fatalf("ListCandidates all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("all candidates len=%d want 2", len(all))
	}

	// q 过滤 title。
	weeklyOnly, err := repo.ListCandidates(TaskSeriesListOptions{WorkspaceID: ws.ID, Q: "周报"})
	if err != nil {
		t.Fatalf("ListCandidates q: %v", err)
	}
	if len(weeklyOnly) != 1 || weeklyOnly[0].Title != "周报" {
		t.Fatalf("q candidates = %#v", weeklyOnly)
	}
}

func TestTaskSeriesRepositoryAppendRuleVersion(t *testing.T) {
	store, ws, _, created := newTaskSeriesRepoFixture(t)
	repo := NewTaskSeriesRepository(store.DB())

	// 追加 weekly 段。
	if err := repo.AppendRuleVersion(created.ID, taskseries.RuleVersion{
		EffectiveFrom: 5000, RecurrenceRule: "weekly", CreatedBy: "user-1",
	}); err != nil {
		t.Fatalf("AppendRuleVersion: %v", err)
	}

	got, err := repo.Get(ws.ID, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.RuleVersions) != 2 {
		t.Fatalf("rule versions len=%d want 2", len(got.RuleVersions))
	}
	// 按 effective_from 升序。
	if got.RuleVersions[0].EffectiveFrom != 1000 || got.RuleVersions[1].EffectiveFrom != 5000 {
		t.Fatalf("rule versions order = %#v", got.RuleVersions)
	}
	if got.RuleVersions[1].RecurrenceRule != "weekly" {
		t.Fatalf("appended rule = %q want weekly", got.RuleVersions[1].RecurrenceRule)
	}
}

func TestTaskSeriesRepositoryListActive(t *testing.T) {
	store, ws, proj, _ := newTaskSeriesRepoFixture(t)
	repo := NewTaskSeriesRepository(store.DB())

	// 第一个 fixture 是 active。
	active, err := repo.ListActive(ws.ID, 0, 0)
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("ListActive len=%d want 1", len(active))
	}

	// 停止一个 stopped series，确认不进入 ListActive。
	end := int64(5000)
	reason := "user_stopped"
	if _, err := repo.Create(taskseries.Series{
		WorkspaceID: ws.ID, ProjectID: proj.ID, Title: "周报",
		Status: taskseries.StatusStopped, RecurrenceRule: "weekly", FirstDue: 2000,
		EffectiveEndAt: &end, StopReason: &reason,
		CreatedBy: "u2", CreatedAt: 200, ModifiedAt: 200,
	}); err != nil {
		t.Fatalf("Create stopped: %v", err)
	}
	active2, _ := repo.ListActive(ws.ID, 0, 0)
	if len(active2) != 1 {
		t.Fatalf("ListActive after stopped len=%d want 1", len(active2))
	}
}

func TestTaskSeriesRepositoryStopProjectSeries(t *testing.T) {
	store, ws, proj, _ := newTaskSeriesRepoFixture(t)
	repo := NewTaskSeriesRepository(store.DB())

	affected, err := repo.StopProjectSeries(ws.ID, proj.ID, 9000, taskseries.StopReasonProjectArchived)
	if err != nil {
		t.Fatalf("StopProjectSeries: %v", err)
	}
	if len(affected) != 1 {
		t.Fatalf("affected len=%d want 1", len(affected))
	}
	if affected[0].Status != taskseries.StatusStopped {
		t.Fatalf("status=%q want stopped", affected[0].Status)
	}
	if affected[0].EffectiveEndAt == nil || *affected[0].EffectiveEndAt != 9000 {
		t.Fatalf("effective_end_at = %#v want 9000", affected[0].EffectiveEndAt)
	}
	if affected[0].StopReason == nil || *affected[0].StopReason != taskseries.StopReasonProjectArchived {
		t.Fatalf("stop_reason = %#v want project_archived", affected[0].StopReason)
	}

	// 二次调用应无新增（已全部 stopped）。
	affected2, _ := repo.StopProjectSeries(ws.ID, proj.ID, 9100, taskseries.StopReasonProjectArchived)
	if len(affected2) != 0 {
		t.Fatalf("second StopProjectSeries affected len=%d want 0", len(affected2))
	}
}
