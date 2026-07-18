package app

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/query"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	domain "git.dajee.net/dajee/xuanchu/internal/task"
	"git.dajee.net/dajee/xuanchu/internal/taskseries"
	"gorm.io/gorm"
)

func TestOccurrenceRefRoundTrip(t *testing.T) {
	ref := OccurrenceRef("11111111-1111-1111-1111-111111111111", 1783785599)
	if !IsOccurrenceRef(ref) {
		t.Fatalf("IsOccurrenceRef(%q) = false", ref)
	}
	seriesID, slot, err := ParseOccurrenceRef(ref)
	if err != nil {
		t.Fatalf("ParseOccurrenceRef: %v", err)
	}
	if seriesID != "11111111-1111-1111-1111-111111111111" || slot != 1783785599 {
		t.Fatalf("ParseOccurrenceRef = %s %d", seriesID, slot)
	}
}

func TestAddTaskViewReturnsUnifiedNormalTaskView(t *testing.T) {
	svc, closeFn := newTestService(t, 1_750_000_000)
	defer closeFn()

	project, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	due := int64(1_800_000_000)
	view, err := svc.AddTaskView(AddInput{
		Title: "normal view", Project: &project.Slug, Due: &due,
		Assignees: []string{"local"},
	})
	if err != nil {
		t.Fatalf("AddTaskView: %v", err)
	}
	if view.UUID == nil || view.ID != *view.UUID {
		t.Fatalf("identity = id:%q uuid:%v", view.ID, view.UUID)
	}
	if view.TaskSlug == nil || *view.TaskSlug != "ops-1" {
		t.Fatalf("task_slug = %v, want ops-1", view.TaskSlug)
	}
	if view.Due == nil || *view.Due != due || view.Entry == nil || view.Modified == nil {
		t.Fatalf("timestamps = due:%v entry:%v modified:%v", view.Due, view.Entry, view.Modified)
	}
	if view.RecurrenceInfo != nil {
		t.Fatalf("recurrence_info = %#v, want nil", view.RecurrenceInfo)
	}
	if len(view.Assignees) != 1 || view.Assignees[0].Name != "local" {
		t.Fatalf("assignees = %#v", view.Assignees)
	}
}

func TestAppViewsExposeCanonicalURLs(t *testing.T) {
	svc, closeFn := newTestService(t, 1_750_000_000)
	defer closeFn()

	project, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	if project.URL != "/workspaces/local/projects/ops" {
		t.Fatalf("project URL = %q", project.URL)
	}

	projectRef := project.Slug
	created, err := svc.AddTaskView(AddInput{Title: "project task", Project: &projectRef})
	if err != nil {
		t.Fatal(err)
	}
	if created.URL != "/workspaces/local/projects/ops/tasks/ops-1" {
		t.Fatalf("task URL = %q", created.URL)
	}

	standalone, err := svc.AddTaskView(AddInput{Title: "standalone"})
	if err != nil {
		t.Fatal(err)
	}
	if standalone.URL != StandaloneTaskURL(*standalone.UUID) {
		t.Fatalf("standalone URL = %q", standalone.URL)
	}
}

func TestTaskViewEvaluatorMatchesSQLCompilerOnMaterializedFixture(t *testing.T) {
	loc := time.Local
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, loc).Unix()
	svc, closeFn := newTestService(t, now)
	defer closeFn()

	project, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetConfig("uda.estimate.type", "numeric"); err != nil {
		t.Fatal(err)
	}
	for name, typ := range map[string]string{
		"effort":   "duration",
		"reviewed": "date",
		"summary":  "string",
	} {
		if err := svc.SetConfig("uda."+name+".type", typ); err != nil {
			t.Fatal(err)
		}
	}
	dep, err := svc.Add(AddInput{Title: "dependency", Project: &project.Slug})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := svc.Add(AddInput{Title: "parent", Project: &project.Slug})
	if err != nil {
		t.Fatal(err)
	}
	description := "contains needle in description"
	priority := "H"
	due := time.Date(2025, 6, 1, 23, 59, 59, 0, loc).Unix()
	scheduled := time.Date(2025, 6, 1, 0, 0, 0, 0, loc).Unix()
	until := time.Date(2025, 6, 2, 23, 59, 59, 0, loc).Unix()
	rich, err := svc.Add(AddInput{
		Title: "rich task", Description: &description, Project: &project.Slug,
		Priority: &priority, Due: &due, Scheduled: &scheduled, Until: &until,
		Assignees: []string{"local"}, Depends: []string{dep.UUID}, Parent: &parent.UUID,
		Tags: []string{"daily"}, UDAs: map[string]string{
			"estimate": "3", "effort": "3h", "reviewed": "2025-06-01", "summary": "Alpha launch review",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(rich.UUID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Annotate(rich.UUID, "review note"); err != nil {
		t.Fatal(err)
	}
	other, err := svc.Add(AddInput{Title: "other task"})
	if err != nil {
		t.Fatal(err)
	}

	allIDs := []string{dep.UUID, parent.UUID, rich.UUID, other.UUID}
	slices.Sort(allIDs)
	tests := []struct {
		query string
		want  []string
	}{
		{query: "uuid:" + rich.UUID, want: []string{rich.UUID}},
		{query: "title:ich", want: []string{rich.UUID}},
		{query: "description:needle", want: []string{rich.UUID}},
		{query: "due:today scheduled:today until.after:today", want: []string{rich.UUID}},
		{query: "project:ops priority:H +daily", want: []string{rich.UUID}},
		{query: "depends:" + dep.UUID, want: []string{rich.UUID}},
		{query: "annotations:note", want: []string{rich.UUID}},
		{query: "parent:" + parent.UUID, want: []string{rich.UUID}},
		{query: "assignee:local", want: []string{rich.UUID}},
		{query: "estimate.after:2", want: []string{rich.UUID}},
		{query: "effort.after:7200", want: []string{rich.UUID}},
		{query: "reviewed:today", want: []string{rich.UUID}},
		{query: "summary:launch", want: []string{rich.UUID}},
		{query: "assignee.notnull depends.notnull annotations.notnull", want: []string{rich.UUID}},
		{query: "assignee.isnull depends.isnull annotations.isnull", want: []string{dep.UUID, parent.UUID, other.UUID}},
		{query: "entry.notnull modified.notnull start.notnull end.isnull", want: []string{rich.UUID}},
		{query: "-daily", want: []string{dep.UUID, parent.UUID, other.UUID}},
		{query: "task_type:normal series_id.isnull recurrence_at.isnull", want: allIDs},
	}

	for _, tc := range tests {
		t.Run(tc.query, func(t *testing.T) {
			expr, err := query.ParseQuery(tc.query)
			if err != nil {
				t.Fatal(err)
			}
			materialized, err := svc.QueryTaskViews(TaskViewQuery{Query: expr, OccurrenceMode: OccurrenceModeMaterialized})
			if err != nil {
				t.Fatalf("materialized query: %v", err)
			}
			expanded, err := svc.QueryTaskViews(TaskViewQuery{
				Query: expr, OccurrenceMode: OccurrenceModeExpand,
				Range: &TaskViewRange{Start: now - 86400, End: now + 7*86400},
			})
			if err != nil {
				t.Fatalf("expand query: %v", err)
			}
			ids := func(page TaskViewPage) []string {
				out := make([]string, 0, len(page.Items))
				for _, item := range page.Items {
					out = append(out, item.ID)
				}
				slices.Sort(out)
				return out
			}
			gotSQL, gotEvaluator := ids(materialized), ids(expanded)
			want := append([]string(nil), tc.want...)
			slices.Sort(want)
			if !slices.Equal(gotSQL, want) {
				t.Fatalf("SQL ids = %#v, want %#v", gotSQL, want)
			}
			if !slices.Equal(gotEvaluator, gotSQL) {
				t.Fatalf("evaluator ids = %#v, SQL ids = %#v", gotEvaluator, gotSQL)
			}
		})
	}
}

func TestTaskViewEvaluatorMatchesSQLCompilerOnOccurrenceAttributes(t *testing.T) {
	loc := time.Local
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, loc).Unix()
	svc, closeFn := newTestService(t, now)
	defer closeFn()

	project, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	ordinary, err := svc.Add(AddInput{Title: "ordinary", Project: &project.Slug})
	if err != nil {
		t.Fatal(err)
	}
	series, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "daily occurrence", Project: &project.Slug,
		RecurrenceRule: "daily", FirstDue: now - 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	if series.FirstOccurrence == nil || series.FirstOccurrence.UUID == nil ||
		series.FirstOccurrence.RecurrenceInfo == nil ||
		series.FirstOccurrence.RecurrenceInfo.Materialization != "materialized" {
		t.Fatalf("first occurrence = %#v", series.FirstOccurrence)
	}
	occurrence := *series.FirstOccurrence

	tests := []struct {
		query string
		want  []string
	}{
		{query: "task_type:occurrence", want: []string{occurrence.ID}},
		{query: "series_id:" + series.Series.ID, want: []string{occurrence.ID}},
		{query: "series_id.notnull recurrence_at.notnull", want: []string{occurrence.ID}},
		{query: "recurrence_at:today", want: []string{occurrence.ID}},
		{query: "uuid:" + *occurrence.UUID, want: []string{occurrence.ID}},
		{query: "task_type:normal series_id.isnull recurrence_at.isnull", want: []string{ordinary.UUID}},
	}
	for _, tc := range tests {
		t.Run(tc.query, func(t *testing.T) {
			expr, err := query.ParseQuery(tc.query)
			if err != nil {
				t.Fatal(err)
			}
			materialized, err := svc.QueryTaskViews(TaskViewQuery{
				Query: expr, OccurrenceMode: OccurrenceModeMaterialized,
			})
			if err != nil {
				t.Fatalf("materialized query: %v", err)
			}
			expanded, err := svc.QueryTaskViews(TaskViewQuery{
				Query: expr, OccurrenceMode: OccurrenceModeExpand,
				Range: &TaskViewRange{Start: now - 86400, End: now},
			})
			if err != nil {
				t.Fatalf("expand query: %v", err)
			}
			ids := func(page TaskViewPage) []string {
				out := make([]string, 0, len(page.Items))
				for _, item := range page.Items {
					out = append(out, item.ID)
				}
				slices.Sort(out)
				return out
			}
			gotSQL, gotEvaluator := ids(materialized), ids(expanded)
			want := append([]string(nil), tc.want...)
			slices.Sort(want)
			if !slices.Equal(gotSQL, want) {
				t.Fatalf("SQL ids = %#v, want %#v", gotSQL, want)
			}
			if !slices.Equal(gotEvaluator, gotSQL) {
				t.Fatalf("evaluator ids = %#v, SQL ids = %#v", gotEvaluator, gotSQL)
			}
		})
	}
}

func TestProjectedOccurrenceMatchesPersistedIdentityNullPredicates(t *testing.T) {
	loc := time.Local
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, loc).Unix()
	svc, closeFn := newTestService(t, now)
	defer closeFn()

	project, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	firstDue := time.Date(2025, 6, 3, 23, 59, 59, 0, loc).Unix()
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "projected", Project: &project.Slug, RecurrenceRule: "daily", FirstDue: firstDue,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.FirstOccurrence == nil || created.FirstOccurrence.RecurrenceInfo == nil || created.FirstOccurrence.RecurrenceInfo.Materialization != "projected" {
		t.Fatalf("first occurrence = %#v", created.FirstOccurrence)
	}

	expr, err := query.ParseQuery("uuid.isnull entry.isnull modified.isnull start.isnull end.isnull")
	if err != nil {
		t.Fatal(err)
	}
	page, err := svc.QueryTaskViews(TaskViewQuery{
		Query: expr, OccurrenceMode: OccurrenceModeExpand,
		Range: &TaskViewRange{Start: now, End: time.Date(2025, 6, 4, 0, 0, 0, 0, loc).Unix()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != created.FirstOccurrence.ID {
		t.Fatalf("items = %#v", page.Items)
	}
}

func TestParseOccurrenceRefRejectsMalformed(t *testing.T) {
	for _, bad := range []string{"not-an-occ-ref", "occ:", "occ:onlyonepart", "occ:series:abc", "occ::123"} {
		if _, _, err := ParseOccurrenceRef(bad); err == nil {
			t.Fatalf("ParseOccurrenceRef(%q) 应失败", bad)
		}
	}
}

func TestTaskToViewOrdinaryTask(t *testing.T) {
	tsk := domain.Task{
		UUID: "task-1", WorkspaceID: "ws", Title: "普通任务", Status: domain.StatusPending,
		Entry: 100, Modified: 200, Project: strPtr("ops"), ProjectSeq: int64Ptr(17),
	}
	view := taskToView("local", tsk, nil)
	if view.ID != "task-1" {
		t.Fatalf("ID = %q want task-1", view.ID)
	}
	if view.UUID == nil || *view.UUID != "task-1" {
		t.Fatalf("UUID = %#v", view.UUID)
	}
	if view.TaskSlug == nil || *view.TaskSlug != "ops-17" {
		t.Fatalf("TaskSlug = %#v want ops-17", view.TaskSlug)
	}
	if view.RecurrenceInfo != nil {
		t.Fatalf("普通任务不应有 RecurrenceInfo: %#v", view.RecurrenceInfo)
	}
	if view.Entry == nil || *view.Entry != 100 {
		t.Fatalf("Entry = %#v", view.Entry)
	}
}

func TestTaskToViewMaterializedOccurrence(t *testing.T) {
	seriesID := "series-1"
	slot := int64(1783785599)
	rule := "daily"
	project := "ops"
	projectSeq := int64(17)
	tsk := domain.Task{
		UUID: "occ-uuid", WorkspaceID: "ws", Title: "巡检", Status: domain.StatusPending,
		Entry: 100, Modified: 200,
		Project: &project, ProjectSeq: &projectSeq,
		SeriesID: &seriesID, RecurrenceAt: &slot, RecurrenceRuleSnapshot: &rule,
		RecurrenceOverrides: []string{"due"},
	}
	view := taskToView("local", tsk, nil)
	// 已物化 occurrence 的 ID 仍是 occurrence_ref（不变）。
	wantRef := OccurrenceRef(seriesID, slot)
	if view.ID != wantRef {
		t.Fatalf("ID = %q want %q", view.ID, wantRef)
	}
	if view.UUID == nil || *view.UUID != "occ-uuid" {
		t.Fatalf("UUID = %#v want occ-uuid", view.UUID)
	}
	if view.TaskSlug == nil || *view.TaskSlug != "ops-17" {
		t.Fatalf("TaskSlug = %#v want ops-17", view.TaskSlug)
	}
	if view.URL != "/workspaces/local/projects/ops/tasks/ops-17" {
		t.Fatalf("URL = %q", view.URL)
	}
	if view.RecurrenceInfo == nil {
		t.Fatal("RecurrenceInfo 为空")
	}
	if view.RecurrenceInfo.Materialization != "materialized" {
		t.Fatalf("Materialization = %q want materialized", view.RecurrenceInfo.Materialization)
	}
	if view.RecurrenceInfo.RecurrenceAt != slot {
		t.Fatalf("RecurrenceAt = %d want %d", view.RecurrenceInfo.RecurrenceAt, slot)
	}
	if !reflect.DeepEqual(view.RecurrenceInfo.Overrides, []string{"due"}) {
		t.Fatalf("Overrides = %#v", view.RecurrenceInfo.Overrides)
	}
}

func TestProjectedOccurrenceViewDoesNotAllocateIdentity(t *testing.T) {
	series := taskseries.Series{
		ID: "series-1", WorkspaceID: "ws", ProjectID: "proj", ProjectSlug: "ops", Title: "巡检",
		Status: taskseries.StatusActive, RecurrenceRule: "daily", FirstDue: 1783785599,
	}
	slot := taskseries.Slot{RecurrenceAt: 1783785599, Rule: "daily"}
	view := projectedOccurrenceView("local", series, slot, nil)
	wantRef := OccurrenceRef("series-1", 1783785599)
	if view.ID != wantRef {
		t.Fatalf("ID = %q want %q", view.ID, wantRef)
	}
	if view.UUID != nil {
		t.Fatalf("projected UUID 应为 nil: %#v", view.UUID)
	}
	if view.TaskSlug != nil {
		t.Fatalf("projected TaskSlug 应为 nil")
	}
	if view.ProjectSeq != nil {
		t.Fatalf("projected ProjectSeq 应为 nil")
	}
	if view.Entry != nil || view.Modified != nil {
		t.Fatalf("projected Entry/Modified 应为 nil: %#v %#v", view.Entry, view.Modified)
	}
	if view.Status != domain.StatusPending {
		t.Fatalf("projected Status = %q want pending", view.Status)
	}
	if view.RecurrenceInfo == nil || view.RecurrenceInfo.Materialization != "projected" {
		t.Fatalf("RecurrenceInfo = %#v", view.RecurrenceInfo)
	}
	if view.Due == nil || *view.Due != 1783785599 {
		t.Fatalf("projected Due 应等于 recurrence_at: %#v", view.Due)
	}
}

// 辅助
func strPtr(s string) *string { return &s }

// newOccurrenceMergeFixture 建立一个含 series + 部分物化 occurrence 的 fixture。
func newOccurrenceMergeFixture(t *testing.T) (*Service, taskseries.Series, int64, int64, int64) {
	t.Helper()
	svc, closeFn := newTestService(t, 1783900000)
	t.Cleanup(closeFn)
	store := svc.store
	ws, _ := store.LocalWorkspace()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	// 创建 daily series，first_due 在未来（不立即物化）。
	day1 := int64(1783785599) // 2026-07-11 23:59:59 +08:00
	day2 := day1 + 86400
	day3 := day2 + 86400
	series, err := svc.taskSeriesRepo.Create(taskseries.Series{
		WorkspaceID: ws.ID, ProjectID: proj.ID, Title: "每日巡检",
		Status: taskseries.StatusActive, RecurrenceRule: "daily", FirstDue: day1,
		CreatedBy: "local", CreatedAt: 1783785599, ModifiedAt: 1783785599,
	})
	if err != nil {
		t.Fatalf("Create series: %v", err)
	}
	// 物化 day1（pending）。
	rule := "daily"
	seriesID := series.ID
	_, _, err = svc.taskOccurrenceRepo.CreateOccurrence(domain.Task{
		UUID: "occ-day1", WorkspaceID: ws.ID, Title: "每日巡检", Status: domain.StatusPending,
		Entry: 1, Modified: 1, ProjectID: &proj.ID,
		SeriesID: &seriesID, RecurrenceAt: &day1, RecurrenceRuleSnapshot: &rule,
	})
	if err != nil {
		t.Fatalf("materialize day1: %v", err)
	}
	return svc, series, day1, day2, day3
}

func TestQueryTaskViewsMaterializedModeReturnsOnlyMaterialized(t *testing.T) {
	svc, _, day1, _, _ := newOccurrenceMergeFixture(t)
	page, err := svc.QueryTaskViews(TaskViewQuery{OccurrenceMode: OccurrenceModeMaterialized})
	if err != nil {
		t.Fatalf("QueryTaskViews: %v", err)
	}
	if page.OccurrenceMode != OccurrenceModeMaterialized {
		t.Fatalf("mode = %q want materialized", page.OccurrenceMode)
	}
	// 只应有 day1 的 materialized occurrence。
	if len(page.Items) != 1 {
		t.Fatalf("items len = %d want 1: %#v", len(page.Items), page.Items)
	}
	if page.Items[0].RecurrenceInfo == nil || page.Items[0].RecurrenceInfo.Materialization != "materialized" {
		t.Fatalf("item[0] 应为 materialized: %#v", page.Items[0].RecurrenceInfo)
	}
	if page.Items[0].RecurrenceInfo.RecurrenceAt != day1 {
		t.Fatalf("recurrence_at = %d want %d", page.Items[0].RecurrenceInfo.RecurrenceAt, day1)
	}
}

func TestOccurrenceCannotMoveAwayFromSeriesProject(t *testing.T) {
	svc, series, day1, day2, _ := newOccurrenceMergeFixture(t)
	other, err := svc.AddProject(AddProjectInput{Slug: "other", Name: "Other"})
	if err != nil {
		t.Fatal(err)
	}
	before := occurrenceRowCount(t, svc, svc.workspaceID)
	projectedRef := OccurrenceRef(series.ID, day2)
	err = svc.Modify(projectedRef, ModifyInput{Project: &other.Slug})
	assertRuntimeCode(t, err, "task_occurrence_project_immutable")
	if after := occurrenceRowCount(t, svc, svc.workspaceID); after != before {
		t.Fatalf("failed projected move materialized row: before=%d after=%d", before, after)
	}

	materializedRef := OccurrenceRef(series.ID, day1)
	err = svc.Modify(materializedRef, ModifyInput{Project: &other.Slug})
	assertRuntimeCode(t, err, "task_occurrence_project_immutable")
	view, err := svc.GetTaskView(materializedRef)
	if err != nil {
		t.Fatal(err)
	}
	if view.ProjectID == nil || *view.ProjectID != series.ProjectID {
		t.Fatalf("materialized occurrence project changed: %#v", view.ProjectID)
	}
}

func TestQueryTaskViewsDefaultsToAllNonDeletedTasks(t *testing.T) {
	svc, closeFn := newTestService(t, 1783900000)
	t.Cleanup(closeFn)

	pending, err := svc.Add(AddInput{Title: "pending item"})
	if err != nil {
		t.Fatalf("Add pending: %v", err)
	}
	completed, err := svc.Add(AddInput{Title: "completed item"})
	if err != nil {
		t.Fatalf("Add completed: %v", err)
	}
	deleted, err := svc.Add(AddInput{Title: "deleted item"})
	if err != nil {
		t.Fatalf("Add deleted: %v", err)
	}
	if err := svc.Done(completed.UUID); err != nil {
		t.Fatalf("Done: %v", err)
	}
	if err := svc.Delete(deleted.UUID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	page, err := svc.QueryTaskViews(TaskViewQuery{})
	if err != nil {
		t.Fatalf("QueryTaskViews: %v", err)
	}
	ids := make(map[string]bool, len(page.Items))
	for _, item := range page.Items {
		ids[item.ID] = true
	}
	if !ids[pending.UUID] || !ids[completed.UUID] || ids[deleted.UUID] {
		t.Fatalf("default ids = %#v, want pending/completed and no deleted", ids)
	}
}

func TestQueryTaskViewsExpandMergesProjectedAndMaterializedWithoutWrites(t *testing.T) {
	svc, _, day1, day2, day3 := newOccurrenceMergeFixture(t)
	store := svc.store
	ws, _ := store.LocalWorkspace()
	beforeCount := occurrenceRowCount(t, svc, ws.ID)
	createHookTestSink(t, store, ws.ID, svc.runtime.ActorUserID, "recurrence-read-sink")
	hook, err := svc.AddHook(HookAddInput{
		Name: "recurrence-read-hook", ScopeType: HookScopeWorkspace,
		EventTypes: []string{"task.created"}, SinkRef: "recurrence-read-sink",
		TimeoutSeconds: 10, MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("AddHook: %v", err)
	}
	beforeAudit, err := svc.ListAudit(AuditListInput{Limit: 1000})
	if err != nil {
		t.Fatalf("ListAudit before expand: %v", err)
	}
	beforeDeliveries, err := svc.ListHookDeliveries(hook.ID, "", 1000, 0)
	if err != nil {
		t.Fatalf("ListHookDeliveries before expand: %v", err)
	}

	page, err := svc.QueryTaskViews(TaskViewQuery{
		OccurrenceMode: OccurrenceModeExpand,
		Range:          &TaskViewRange{Start: day1, End: day3 + 1},
	})
	if err != nil {
		t.Fatalf("QueryTaskViews expand: %v", err)
	}
	if page.OccurrenceMode != OccurrenceModeExpand {
		t.Fatalf("mode = %q want expand", page.OccurrenceMode)
	}
	// day1 materialized + day2/day3 projected = 3 条。
	if len(page.Items) != 3 {
		refs := make([]string, 0)
		for _, it := range page.Items {
			refs = append(refs, it.ID)
		}
		t.Fatalf("items len = %d want 3: %v", len(page.Items), refs)
	}
	// 不写库。
	if after := occurrenceRowCount(t, svc, ws.ID); after != beforeCount {
		t.Fatalf("expand 写库了: before=%d after=%d", beforeCount, after)
	}
	afterAudit, err := svc.ListAudit(AuditListInput{Limit: 1000})
	if err != nil {
		t.Fatalf("ListAudit after expand: %v", err)
	}
	afterDeliveries, err := svc.ListHookDeliveries(hook.ID, "", 1000, 0)
	if err != nil {
		t.Fatalf("ListHookDeliveries after expand: %v", err)
	}
	if len(afterAudit) != len(beforeAudit) || len(afterDeliveries) != len(beforeDeliveries) {
		t.Fatalf("expand produced side effects: audit %d->%d deliveries %d->%d", len(beforeAudit), len(afterAudit), len(beforeDeliveries), len(afterDeliveries))
	}
	// day1 为 materialized，day2/day3 为 projected。
	bySlot := map[int64]TaskOccurrenceView{}
	for _, it := range page.Items {
		if it.RecurrenceInfo != nil {
			bySlot[it.RecurrenceInfo.RecurrenceAt] = it
		}
	}
	if bySlot[day1].RecurrenceInfo.Materialization != "materialized" {
		t.Fatalf("day1 应为 materialized: %#v", bySlot[day1].RecurrenceInfo)
	}
	if bySlot[day2].RecurrenceInfo == nil || bySlot[day2].RecurrenceInfo.Materialization != "projected" {
		t.Fatalf("day2 应为 projected: %#v", bySlot[day2].RecurrenceInfo)
	}
	if bySlot[day2].UUID != nil {
		t.Fatalf("projected UUID 应为 nil: %#v", bySlot[day2].UUID)
	}
}

func TestQueryTaskViewsExpandRejectsMissingRange(t *testing.T) {
	svc, _, _, _, _ := newOccurrenceMergeFixture(t)
	_, err := svc.QueryTaskViews(TaskViewQuery{OccurrenceMode: OccurrenceModeExpand})
	if err == nil {
		t.Fatal("expand 缺范围应失败")
	}
}

func TestQueryTaskViewsExpandRejectsRangeTooLarge(t *testing.T) {
	svc, _, day1, _, _ := newOccurrenceMergeFixture(t)
	// 400 天 > 366。
	_, err := svc.QueryTaskViews(TaskViewQuery{
		OccurrenceMode: OccurrenceModeExpand,
		Range:          &TaskViewRange{Start: day1, End: day1 + 400*86400},
	})
	if err == nil {
		t.Fatal("超出 366 天应失败")
	}
}

func TestGetTaskViewProjectedDoesNotWrite(t *testing.T) {
	svc, series, _, day2, _ := newOccurrenceMergeFixture(t)
	store := svc.store
	ws, _ := store.LocalWorkspace()
	beforeCount := occurrenceRowCount(t, svc, ws.ID)

	ref := OccurrenceRef(series.ID, day2)
	view, err := svc.GetTaskView(ref)
	if err != nil {
		t.Fatalf("GetTaskView(projected): %v", err)
	}
	if view.RecurrenceInfo == nil || view.RecurrenceInfo.Materialization != "projected" {
		t.Fatalf("应为 projected: %#v", view.RecurrenceInfo)
	}
	if view.RecurrenceInfo.SeriesTitle != "每日巡检" {
		t.Fatalf("projected series title = %q", view.RecurrenceInfo.SeriesTitle)
	}
	if view.UUID != nil {
		t.Fatalf("projected UUID 应为 nil")
	}
	if view.Project == nil || *view.Project != "ops" {
		t.Fatalf("projected project = %#v", view.Project)
	}
	wantURL := ProjectTaskURL("local", "ops", view.ID)
	if view.URL != wantURL {
		t.Fatalf("projected URL = %q, want %q", view.URL, wantURL)
	}
	if after := occurrenceRowCount(t, svc, ws.ID); after != beforeCount {
		t.Fatalf("GetTaskView(projected) 写库了: before=%d after=%d", beforeCount, after)
	}
}

func TestGetTaskViewMaterialized(t *testing.T) {
	svc, series, day1, _, _ := newOccurrenceMergeFixture(t)
	ref := OccurrenceRef(series.ID, day1)
	view, err := svc.GetTaskView(ref)
	if err != nil {
		t.Fatalf("GetTaskView(materialized): %v", err)
	}
	if view.RecurrenceInfo == nil || view.RecurrenceInfo.Materialization != "materialized" {
		t.Fatalf("应为 materialized: %#v", view.RecurrenceInfo)
	}
	if view.RecurrenceInfo.SeriesTitle != "每日巡检" {
		t.Fatalf("materialized series title = %q", view.RecurrenceInfo.SeriesTitle)
	}
}

func TestGetTaskViewRejectsInvalidOccurrenceRef(t *testing.T) {
	svc, _, _, _, _ := newOccurrenceMergeFixture(t)
	// 不存在的 series。
	_, err := svc.GetTaskView("occ:nonexistent-series:9999999")
	if err == nil {
		t.Fatal("不存在的 occurrence_ref 应失败")
	}
}

// occurrenceRowCount 统计 tasks 表中 occurrence 行数。
func occurrenceRowCount(t *testing.T, svc *Service, workspaceID string) int {
	t.Helper()
	tasks, err := svc.repo.List(workspaceID, storage.ListOptions{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	count := 0
	for _, tk := range tasks {
		if tk.SeriesID != nil {
			count++
		}
	}
	return count
}

// --- Task 5: Series CRUD 测试 ---

func TestAddTaskSeriesCreatesFutureProjectedFirstOccurrence(t *testing.T) {
	svc, closeFn := newTestService(t, 1783785599-86400) // now 在 first_due 之前
	defer closeFn()
	store := svc.store
	ws, _ := store.LocalWorkspace()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	firstDue := int64(1783785599)
	result, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: firstDue, Assignees: []string{"local"},
	})
	if err != nil {
		t.Fatalf("AddTaskSeries: %v", err)
	}
	// series 存在。
	if result.Series.ID == "" {
		t.Fatal("series ID 为空")
	}
	if result.Series.Status != taskseries.StatusActive {
		t.Fatalf("status = %q want active", result.Series.Status)
	}
	// 初始 rule version。
	if len(result.Series.RuleVersions) != 1 {
		t.Fatalf("rule versions len = %d want 1", len(result.Series.RuleVersions))
	}
	// 无 task 行（first_due 在未来）。
	if count := occurrenceRowCount(t, svc, ws.ID); count != 0 {
		t.Fatalf("future first_due 不应物化, count = %d", count)
	}
	// first occurrence 是 projected。
	if result.FirstOccurrence == nil {
		t.Fatal("first occurrence 为空")
	}
	if result.FirstOccurrence.RecurrenceInfo == nil || result.FirstOccurrence.RecurrenceInfo.Materialization != "projected" {
		t.Fatalf("first occurrence 应为 projected: %#v", result.FirstOccurrence.RecurrenceInfo)
	}
	if result.FirstOccurrence.UUID != nil {
		t.Fatal("projected first occurrence UUID 应为 nil")
	}
}

func TestAddTaskSeriesMaterializesFirstOccurrenceWhenDueEntered(t *testing.T) {
	svc, closeFn := newTestService(t, 1783785599+86400) // now 在 first_due 之后
	defer closeFn()
	store := svc.store
	ws, _ := store.LocalWorkspace()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	firstDue := int64(1783785599)
	result, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: firstDue, Assignees: []string{"local"},
	})
	if err != nil {
		t.Fatalf("AddTaskSeries: %v", err)
	}
	// first_due 已进入执行期：物化。
	if result.FirstOccurrence == nil || result.FirstOccurrence.RecurrenceInfo == nil {
		t.Fatal("first occurrence / info 为空")
	}
	if result.FirstOccurrence.RecurrenceInfo.Materialization != "materialized" {
		t.Fatalf("应物化: %#v", result.FirstOccurrence.RecurrenceInfo)
	}
	if result.FirstOccurrence.UUID == nil {
		t.Fatal("materialized 应有 UUID")
	}
	if count := occurrenceRowCount(t, svc, ws.ID); count != 1 {
		t.Fatalf("应物化 1 条, count = %d", count)
	}
	persisted, err := svc.GetTaskView(*result.FirstOccurrence.UUID)
	if err != nil {
		t.Fatalf("GetTaskView(first occurrence): %v", err)
	}
	if len(persisted.Assignees) != 1 || persisted.Assignees[0].Name != "local" {
		t.Fatalf("首个实例负责人未持久化: %#v", persisted.Assignees)
	}
}

func TestAddTaskSeriesRejectsInvalidRule(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "x", ProjectID: proj.ID, RecurrenceRule: "biweekly", FirstDue: 2000,
	})
	if err == nil {
		t.Fatal("biweekly 应被拒绝")
	}
}

func TestAddTaskSeriesRejectsUnsupportedFields(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	wait := int64(500)
	_, err = svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "x", ProjectID: proj.ID, RecurrenceRule: "daily", FirstDue: 2000, Wait: &wait,
	})
	if err == nil {
		t.Fatal("wait 不被支持")
	}
}

func TestListTaskSeriesReturnsCreated(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID, RecurrenceRule: "daily", FirstDue: 2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	page, err := svc.ListTaskSeries(TaskSeriesListInput{ProjectID: proj.ID})
	if err != nil {
		t.Fatalf("ListTaskSeries: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("items len = %d want 1", len(page.Items))
	}
	if page.Total != 1 {
		t.Fatalf("total = %d want 1", page.Total)
	}
	if page.Items[0].URL != "/workspaces/local/projects/ops/series/ops-s-1" {
		t.Fatalf("series URL = %q", page.Items[0].URL)
	}
}

func TestListTaskSeriesLoadsDerivedDataWithoutNPlusOne(t *testing.T) {
	svc, closeFn := newTestService(t, time.Date(2026, 7, 14, 12, 0, 0, 0, time.Local).Unix())
	defer closeFn()
	project, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		_, err := svc.AddTaskSeries(AddTaskSeriesInput{
			Title: fmt.Sprintf("series-%d", i), ProjectID: project.ID,
			RecurrenceRule: "daily",
			FirstDue:       time.Date(2030, 1, i+1, 23, 59, 59, 0, time.Local).Unix(),
			Assignees:      []string{"local"},
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	queries := 0
	callbackName := "test:list-task-series-query-count"
	db := svc.store.DB()
	if err := db.Callback().Query().Before("gorm:query").Register(callbackName, func(*gorm.DB) {
		queries++
	}); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Query().Remove(callbackName)
	page, err := svc.ListTaskSeries(TaskSeriesListInput{ProjectID: project.ID, Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 5 {
		t.Fatalf("total = %d, want 5", page.Total)
	}
	if queries > 15 {
		t.Fatalf("ListTaskSeries executed %d queries for 5 series, want <= 15", queries)
	}
}

func TestGetTaskSeriesReturnsDetail(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID, RecurrenceRule: "daily", FirstDue: 100000,
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := svc.GetTaskSeries(created.Series.ID)
	if err != nil {
		t.Fatalf("GetTaskSeries: %v", err)
	}
	if detail.Series.ID != created.Series.ID {
		t.Fatalf("series ID 不匹配")
	}
	want, err := taskseries.Next(100000, "daily", svc.clock.Location())
	if err != nil {
		t.Fatal(err)
	}
	if detail.Series.SuggestedRuleEffectiveFrom == nil || *detail.Series.SuggestedRuleEffectiveFrom != want {
		t.Fatalf("suggested_rule_effective_from = %#v want %d", detail.Series.SuggestedRuleEffectiveFrom, want)
	}
}

func TestTaskSeriesPaginationUsesSharedDefaultAndBounds(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	project, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: project.ID, RecurrenceRule: "daily", FirstDue: 100000,
	})
	if err != nil {
		t.Fatal(err)
	}

	seriesPage, err := svc.ListTaskSeries(TaskSeriesListInput{ProjectID: project.ID})
	if err != nil {
		t.Fatal(err)
	}
	if seriesPage.Limit != 200 {
		t.Fatalf("series default limit = %d, want 200", seriesPage.Limit)
	}
	occurrencePage, err := svc.ListTaskSeriesOccurrences(created.Series.ID, TaskSeriesOccurrenceListInput{})
	if err != nil {
		t.Fatal(err)
	}
	if occurrencePage.Limit != 200 {
		t.Fatalf("occurrence default limit = %d, want 200", occurrencePage.Limit)
	}

	for _, tc := range []struct {
		name  string
		limit int
		off   int
		code  string
	}{
		{name: "series limit", limit: 1001, code: "api_bad_limit"},
		{name: "series offset", off: -1, code: "api_bad_offset"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.ListTaskSeries(TaskSeriesListInput{ProjectID: project.ID, Limit: tc.limit, Offset: tc.off})
			assertTaskSeriesRuntimeErrorCode(t, err, tc.code)
		})
		t.Run("occurrence "+tc.name, func(t *testing.T) {
			_, err := svc.ListTaskSeriesOccurrences(created.Series.ID, TaskSeriesOccurrenceListInput{Limit: tc.limit, Offset: tc.off})
			assertTaskSeriesRuntimeErrorCode(t, err, tc.code)
		})
	}
}

func assertTaskSeriesRuntimeErrorCode(t *testing.T, err error, want string) {
	t.Helper()
	got, ok := err.(RuntimeError)
	if !ok || got.Code != want {
		t.Fatalf("error = %#v, want RuntimeError code %q", err, want)
	}
}

func TestGetTaskSeriesCapsOpenAndReturnsNewestRecentOccurrences(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	project, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: project.ID, RecurrenceRule: "daily", FirstDue: 100000,
	})
	if err != nil {
		t.Fatal(err)
	}
	seriesID, rule := created.Series.ID, "daily"
	createOccurrence := func(index int, status string) {
		t.Helper()
		slot := int64(200000 + index)
		due := slot
		_, _, err := svc.taskOccurrenceRepo.CreateOccurrence(domain.Task{
			UUID: fmt.Sprintf("occurrence-%03d-%s", index, status), WorkspaceID: svc.workspaceID,
			ProjectID: &project.ID, Title: "每日巡检", Status: status, Entry: slot, Modified: slot, Due: &due,
			SeriesID: &seriesID, RecurrenceAt: &slot, RecurrenceRuleSnapshot: &rule,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for index := range 205 {
		createOccurrence(index, domain.StatusPending)
	}
	for index := 205; index < 217; index++ {
		createOccurrence(index, domain.StatusCompleted)
	}
	for index := 217; index < 229; index++ {
		createOccurrence(index, domain.StatusDeleted)
	}

	detail, err := svc.GetTaskSeries(seriesID)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(detail.OpenOccurrences); got != 200 {
		t.Fatalf("open occurrences = %d, want 200", got)
	}
	if got := len(detail.RecentCompleted); got != 10 {
		t.Fatalf("recent completed = %d, want 10", got)
	}
	if got := len(detail.RecentSkipped); got != 10 {
		t.Fatalf("recent skipped = %d, want 10", got)
	}
	if got := detail.RecentCompleted[0].RecurrenceInfo.RecurrenceAt; got != 200216 {
		t.Fatalf("newest completed recurrence_at = %d, want 200216", got)
	}
	if got := detail.RecentSkipped[0].RecurrenceInfo.RecurrenceAt; got != 200228 {
		t.Fatalf("newest skipped recurrence_at = %d, want 200228", got)
	}
}

func TestGetTaskSeriesSuggestsSlotAfterMaterializedOccurrence(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID, RecurrenceRule: "daily", FirstDue: 2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	secondSlot, err := taskseries.Next(2000, "daily", svc.clock.Location())
	if err != nil {
		t.Fatal(err)
	}
	seriesID := created.Series.ID
	rule := "daily"
	if _, _, err := svc.taskOccurrenceRepo.CreateOccurrence(domain.Task{
		UUID: "future-occurrence", WorkspaceID: svc.workspaceID, ProjectID: &proj.ID,
		Title: "每日巡检", Status: domain.StatusPending, Entry: 1000, Modified: 1000,
		SeriesID: &seriesID, RecurrenceAt: &secondSlot, RecurrenceRuleSnapshot: &rule,
	}); err != nil {
		t.Fatal(err)
	}
	want, err := taskseries.Next(secondSlot, "daily", svc.clock.Location())
	if err != nil {
		t.Fatal(err)
	}
	detail, err := svc.GetTaskSeries(seriesID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Series.SuggestedRuleEffectiveFrom == nil || *detail.Series.SuggestedRuleEffectiveFrom != want {
		t.Fatalf("suggested_rule_effective_from = %#v want %d", detail.Series.SuggestedRuleEffectiveFrom, want)
	}
}

func TestGetTaskSeriesSuggestionUsesCurrentRuleVersion(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "巡检", ProjectID: proj.ID, RecurrenceRule: "daily", FirstDue: 2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	weekly := "weekly"
	effectiveFrom, err := taskseries.Next(2000, "daily", svc.clock.Location())
	if err != nil {
		t.Fatal(err)
	}
	updated, err := svc.ModifyTaskSeries(created.Series.ID, ModifyTaskSeriesInput{
		RecurrenceRule: &weekly, EffectiveFrom: &effectiveFrom,
	})
	if err != nil {
		t.Fatal(err)
	}
	want, err := taskseries.Next(effectiveFrom, weekly, svc.clock.Location())
	if err != nil {
		t.Fatal(err)
	}
	if updated.SuggestedRuleEffectiveFrom == nil || *updated.SuggestedRuleEffectiveFrom != want {
		t.Fatalf("suggested_rule_effective_from = %#v want %d", updated.SuggestedRuleEffectiveFrom, want)
	}
}

func TestGetTaskSeriesSuggestionIsNilWithoutFutureLegalSlot(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	until := int64(2000)
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "一次巡检", ProjectID: proj.ID, RecurrenceRule: "daily", FirstDue: 2000, Until: &until,
	})
	if err != nil {
		t.Fatal(err)
	}
	seriesID := created.Series.ID
	rule := "daily"
	if _, _, err := svc.taskOccurrenceRepo.CreateOccurrence(domain.Task{
		UUID: "only-occurrence", WorkspaceID: svc.workspaceID, ProjectID: &proj.ID,
		Title: "一次巡检", Status: domain.StatusPending, Entry: 1000, Modified: 1000,
		SeriesID: &seriesID, RecurrenceAt: &until, RecurrenceRuleSnapshot: &rule,
	}); err != nil {
		t.Fatal(err)
	}
	detail, err := svc.GetTaskSeries(seriesID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Series.SuggestedRuleEffectiveFrom != nil {
		t.Fatalf("suggested_rule_effective_from = %#v want nil", detail.Series.SuggestedRuleEffectiveFrom)
	}
}

func TestStoppedTaskSeriesHasNoSuggestedRuleEffectiveFrom(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID, RecurrenceRule: "daily", FirstDue: 2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	stopped, err := svc.StopTaskSeries(created.Series.ID, StopTaskSeriesInput{})
	if err != nil {
		t.Fatal(err)
	}
	if stopped.SuggestedRuleEffectiveFrom != nil {
		t.Fatalf("suggested_rule_effective_from = %#v want nil", stopped.SuggestedRuleEffectiveFrom)
	}
}

// --- Task 6: 写前物化测试 ---

func TestMaterializeOccurrenceForWriteCreatesTaskForProjectedRef(t *testing.T) {
	svc, series, _, day2, _ := newOccurrenceMergeFixture(t)
	store := svc.store
	ws, _ := store.LocalWorkspace()
	beforeCount := occurrenceRowCount(t, svc, ws.ID)
	ref := OccurrenceRef(series.ID, day2)

	tsk, existed, err := svc.MaterializeOccurrenceForWrite(ref)
	if err != nil {
		t.Fatalf("MaterializeOccurrenceForWrite: %v", err)
	}
	if existed {
		t.Fatal("projected 不应 existed=true")
	}
	if tsk.UUID == "" {
		t.Fatal("物化后应有 UUID")
	}
	if tsk.SeriesID == nil || *tsk.SeriesID != series.ID {
		t.Fatalf("series_id 不匹配: %#v", tsk.SeriesID)
	}
	if tsk.RecurrenceAt == nil || *tsk.RecurrenceAt != day2 {
		t.Fatalf("recurrence_at 不匹配: %#v", tsk.RecurrenceAt)
	}
	if tsk.ProjectSeq == nil {
		t.Fatal("物化应分配 project_seq")
	}
	if tsk.Project == nil || *tsk.Project != "ops" {
		t.Fatalf("物化后的 Project = %#v，期望 ops", tsk.Project)
	}
	if tsk.ProjectID == nil || *tsk.ProjectID != series.ProjectID {
		t.Fatalf("物化后的 ProjectID = %#v，期望 %q", tsk.ProjectID, series.ProjectID)
	}
	view, err := svc.GetTaskView(ref)
	if err != nil {
		t.Fatalf("GetTaskView(materialized): %v", err)
	}
	if view.TaskSlug == nil || *view.TaskSlug != "ops-1" {
		t.Fatalf("物化后的 task_slug = %#v，期望 ops-1", view.TaskSlug)
	}
	if after := occurrenceRowCount(t, svc, ws.ID); after != beforeCount+1 {
		t.Fatalf("应新增 1 行: before=%d after=%d", beforeCount, after)
	}
}

func TestMaterializeOccurrenceForWriteIdempotentForExistingSlot(t *testing.T) {
	svc, series, day1, _, _ := newOccurrenceMergeFixture(t)
	// day1 已物化（fixture 中物化了 day1）。
	ref := OccurrenceRef(series.ID, day1)
	tsk, existed, err := svc.MaterializeOccurrenceForWrite(ref)
	if err != nil {
		t.Fatalf("MaterializeOccurrenceForWrite: %v", err)
	}
	if !existed {
		t.Fatal("已物化槽位应 existed=true")
	}
	if tsk.UUID != "occ-day1" {
		t.Fatalf("应返回已存在的 UUID occ-day1, got %q", tsk.UUID)
	}
}

func TestMaterializeOccurrenceForWriteRejectsNonOccurrenceRef(t *testing.T) {
	svc, _, _, _, _ := newOccurrenceMergeFixture(t)
	_, _, err := svc.MaterializeOccurrenceForWrite("not-an-occ-ref")
	if err == nil {
		t.Fatal("非 occurrence_ref 应失败")
	}
}

func TestMaterializeOccurrenceForWriteRejectsInvalidSlot(t *testing.T) {
	svc, series, _, _, _ := newOccurrenceMergeFixture(t)
	// 槽位 9999 不在 daily 序列的合法位置（first_due 之后但需属于规则段）。
	// 实际 daily 每天一个槽位，9999 作为任意时间戳可能不在合法序列；用明显非法的负数。
	_, _, err := svc.MaterializeOccurrenceForWrite(OccurrenceRef(series.ID, -1))
	if err == nil {
		t.Fatal("非法槽位应失败")
	}
}

func TestWithTaskForWriteExecutesActionAndMaterializes(t *testing.T) {
	svc, series, _, day2, _ := newOccurrenceMergeFixture(t)
	ref := OccurrenceRef(series.ID, day2)
	store := svc.store
	ws, _ := store.LocalWorkspace()
	beforeCount := occurrenceRowCount(t, svc, ws.ID)

	actionCalled := false
	view, err := svc.WithTaskForWrite(ref, func(s *Service, tsk domain.Task) error {
		actionCalled = true
		if tsk.UUID == "" {
			t.Fatal("action 收到的 task 应已物化")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithTaskForWrite: %v", err)
	}
	if !actionCalled {
		t.Fatal("action 未被调用")
	}
	if view.UUID == nil {
		t.Fatal("返回 view 应有 UUID")
	}
	if view.RecurrenceInfo == nil || view.RecurrenceInfo.Materialization != "materialized" {
		t.Fatalf("应为 materialized: %#v", view.RecurrenceInfo)
	}
	if after := occurrenceRowCount(t, svc, ws.ID); after != beforeCount+1 {
		t.Fatalf("应新增 1 行: before=%d after=%d", beforeCount, after)
	}
}

func TestWithTaskForWriteRollsBackOnActionError(t *testing.T) {
	svc, series, _, day2, _ := newOccurrenceMergeFixture(t)
	ref := OccurrenceRef(series.ID, day2)
	store := svc.store
	ws, _ := store.LocalWorkspace()
	beforeCount := occurrenceRowCount(t, svc, ws.ID)

	_, err := svc.WithTaskForWrite(ref, func(s *Service, tsk domain.Task) error {
		return fmt.Errorf("simulated failure")
	})
	if err == nil {
		t.Fatal("action 错误应传播")
	}
	// 物化应回滚（无新行）。
	if after := occurrenceRowCount(t, svc, ws.ID); after != beforeCount {
		t.Fatalf("action 失败应回滚物化: before=%d after=%d", beforeCount, after)
	}
}

func TestWithTaskForWriteWorksWithOrdinaryTaskUUID(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	created, err := svc.Add(AddInput{Title: "普通任务"})
	if err != nil {
		t.Fatal(err)
	}
	actionCalled := false
	view, err := svc.WithTaskForWrite(created.UUID, func(s *Service, tsk domain.Task) error {
		actionCalled = true
		return nil
	})
	if err != nil {
		t.Fatalf("WithTaskForWrite(ordinary): %v", err)
	}
	if !actionCalled {
		t.Fatal("action 未被调用")
	}
	if view.ID != created.UUID {
		t.Fatalf("view ID = %q want %q", view.ID, created.UUID)
	}
	if view.RecurrenceInfo != nil {
		t.Fatal("普通任务不应有 RecurrenceInfo")
	}
}

func TestReplaceEditableTaskRecordsMaterializedOccurrenceOverrides(t *testing.T) {
	svc, series, day1, _, _ := newOccurrenceMergeFixture(t)
	ref := OccurrenceRef(series.ID, day1)
	resolution, err := svc.ResolveTaskReferenceForRead(ref)
	if err != nil {
		t.Fatalf("ResolveTaskReferenceForRead: %v", err)
	}
	if resolution.Task == nil {
		t.Fatal("materialized occurrence should resolve to a task row")
	}
	edited := *resolution.Task
	edited.Title = "本次单独标题"
	due := day1 + 3600
	edited.Due = &due

	if err := svc.ReplaceEditableTask(ref, domain.EditableFieldsFromTask(edited)); err != nil {
		t.Fatalf("ReplaceEditableTask: %v", err)
	}
	view, err := svc.GetTaskView(ref)
	if err != nil {
		t.Fatalf("GetTaskView: %v", err)
	}
	if view.Title != "本次单独标题" || view.Due == nil || *view.Due != due {
		t.Fatalf("edited view = %#v", view)
	}
	wantOverrides := []string{"due", "title"}
	if view.RecurrenceInfo == nil || !reflect.DeepEqual(view.RecurrenceInfo.Overrides, wantOverrides) {
		t.Fatalf("overrides = %#v, want %#v", view.RecurrenceInfo, wantOverrides)
	}
}

func TestReplaceEditableTaskMaterializesProjectedOccurrenceAndKeepsStableIdentity(t *testing.T) {
	svc, series, _, day2, _ := newOccurrenceMergeFixture(t)
	ref := OccurrenceRef(series.ID, day2)
	workspace, _ := svc.store.LocalWorkspace()
	beforeCount := occurrenceRowCount(t, svc, workspace.ID)
	due := day2 + 3600

	err := svc.ReplaceEditableTask(ref, domain.EditableFields{
		Title: "只修改本次", Status: domain.StatusPending, Due: &due,
		Project: strPtr("ops"), Tags: []string{"edited"},
	})
	if err != nil {
		t.Fatalf("ReplaceEditableTask(projected): %v", err)
	}
	if after := occurrenceRowCount(t, svc, workspace.ID); after != beforeCount+1 {
		t.Fatalf("occurrence rows = %d, want %d", after, beforeCount+1)
	}
	view, err := svc.GetTaskView(ref)
	if err != nil {
		t.Fatalf("GetTaskView: %v", err)
	}
	if view.ID != ref || view.UUID == nil || view.TaskSlug == nil {
		t.Fatalf("materialized identity = %#v", view)
	}
	if view.RecurrenceInfo == nil || view.RecurrenceInfo.RecurrenceAt != day2 {
		t.Fatalf("recurrence info = %#v", view.RecurrenceInfo)
	}
	wantOverrides := []string{"due", "tags", "title"}
	if !reflect.DeepEqual(view.RecurrenceInfo.Overrides, wantOverrides) {
		t.Fatalf("overrides = %#v, want %#v", view.RecurrenceInfo.Overrides, wantOverrides)
	}
}

func TestReplaceEditableTaskRejectsProjectedOccurrenceProjectMoveWithoutMaterializing(t *testing.T) {
	svc, series, _, day2, _ := newOccurrenceMergeFixture(t)
	other, err := svc.AddProject(AddProjectInput{Slug: "other", Name: "Other"})
	if err != nil {
		t.Fatal(err)
	}
	ref := OccurrenceRef(series.ID, day2)
	beforeRows := occurrenceRowCount(t, svc, svc.workspaceID)
	beforeAudit, err := svc.ListAudit(AuditListInput{Limit: 1000})
	if err != nil {
		t.Fatal(err)
	}

	err = svc.ReplaceEditableTask(ref, domain.EditableFields{
		Title: "非法迁移", Status: domain.StatusPending, Due: &day2, Project: &other.Slug,
	})
	assertRuntimeCode(t, err, "task_occurrence_project_immutable")
	if afterRows := occurrenceRowCount(t, svc, svc.workspaceID); afterRows != beforeRows {
		t.Fatalf("failed external edit materialized occurrence: before=%d after=%d", beforeRows, afterRows)
	}
	afterAudit, err := svc.ListAudit(AuditListInput{Limit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if len(afterAudit) != len(beforeAudit) {
		t.Fatalf("failed external edit wrote audit: before=%d after=%d", len(beforeAudit), len(afterAudit))
	}
	view, err := svc.GetTaskView(ref)
	if err != nil {
		t.Fatal(err)
	}
	if view.UUID != nil || view.TaskSlug != nil || view.RecurrenceInfo == nil || view.RecurrenceInfo.Materialization != "projected" {
		t.Fatalf("failed external edit changed projected view: %#v", view)
	}
}

func TestReplaceEditableTaskProjectedRollsBackMaterializationWhenAuditFails(t *testing.T) {
	svc, series, _, day2, _ := newOccurrenceMergeFixture(t)
	ref := OccurrenceRef(series.ID, day2)
	workspace, _ := svc.store.LocalWorkspace()
	beforeCount := occurrenceRowCount(t, svc, workspace.ID)
	originalAuditRepo := svc.auditRepo
	svc.auditRepo = &failingAuditRepo{}

	err := svc.ReplaceEditableTask(ref, domain.EditableFields{
		Title: "不能提交", Status: domain.StatusPending, Due: &day2,
		Project: strPtr("ops"),
	})
	svc.auditRepo = originalAuditRepo
	if err == nil {
		t.Fatal("ReplaceEditableTask() error = nil, want audit failure")
	}
	if after := occurrenceRowCount(t, svc, workspace.ID); after != beforeCount {
		t.Fatalf("failed edit materialized occurrence: before=%d after=%d", beforeCount, after)
	}
	view, err := svc.GetTaskView(ref)
	if err != nil {
		t.Fatalf("GetTaskView after rollback: %v", err)
	}
	if view.UUID != nil || view.RecurrenceInfo == nil || view.RecurrenceInfo.Materialization != "projected" {
		t.Fatalf("view after rollback = %#v", view)
	}
}

func TestAppendPrependDescriptionMaterializeProjectedOccurrenceAndRecordOverride(t *testing.T) {
	svc, series, _, day2, _ := newOccurrenceMergeFixture(t)
	ref := OccurrenceRef(series.ID, day2)

	if err := svc.AppendDescription(ref, "尾部说明"); err != nil {
		t.Fatalf("AppendDescription(projected): %v", err)
	}
	if err := svc.PrependDescription(ref, "前置说明"); err != nil {
		t.Fatalf("PrependDescription(materialized): %v", err)
	}
	view, err := svc.GetTaskView(ref)
	if err != nil {
		t.Fatalf("GetTaskView: %v", err)
	}
	if view.Title != "每日巡检" {
		t.Fatalf("title = %q, append/prepend must not change title", view.Title)
	}
	if view.Description == nil || *view.Description != "前置说明 尾部说明" {
		t.Fatalf("description = %#v", view.Description)
	}
	if view.RecurrenceInfo == nil || !reflect.DeepEqual(view.RecurrenceInfo.Overrides, []string{"description"}) {
		t.Fatalf("recurrence info = %#v", view.RecurrenceInfo)
	}
}

func TestEmptyAppendDoesNotMaterializeProjectedOccurrence(t *testing.T) {
	svc, series, _, day2, _ := newOccurrenceMergeFixture(t)
	workspace, _ := svc.store.LocalWorkspace()
	before := occurrenceRowCount(t, svc, workspace.ID)
	if err := svc.AppendDescription(OccurrenceRef(series.ID, day2), "   "); err == nil {
		t.Fatal("AppendDescription(empty) error = nil")
	}
	if after := occurrenceRowCount(t, svc, workspace.ID); after != before {
		t.Fatalf("empty append materialized occurrence: before=%d after=%d", before, after)
	}
}

func TestWithExistingTaskForSubresourceWriteProjectedReturnsNotFound(t *testing.T) {
	svc, series, _, day2, _ := newOccurrenceMergeFixture(t)
	ref := OccurrenceRef(series.ID, day2)
	store := svc.store
	ws, _ := store.LocalWorkspace()
	beforeCount := occurrenceRowCount(t, svc, ws.ID)

	_, err := svc.WithExistingTaskForSubresourceWrite(ref, func(s *Service, tsk domain.Task) error {
		t.Fatal("projected 子资源写不应调用 action")
		return nil
	})
	if err == nil {
		t.Fatal("projected 子资源写应返回 not found")
	}
	// 不物化。
	if after := occurrenceRowCount(t, svc, ws.ID); after != beforeCount {
		t.Fatalf("projected 子资源写不应物化: before=%d after=%d", beforeCount, after)
	}
}

func TestWithExistingTaskForSubresourceWorksWithMaterialized(t *testing.T) {
	svc, series, day1, _, _ := newOccurrenceMergeFixture(t)
	ref := OccurrenceRef(series.ID, day1)
	actionCalled := false
	_, err := svc.WithExistingTaskForSubresourceWrite(ref, func(s *Service, tsk domain.Task) error {
		actionCalled = true
		if tsk.UUID != "occ-day1" {
			t.Fatalf("UUID = %q want occ-day1", tsk.UUID)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithExistingTaskForSubresourceWrite: %v", err)
	}
	if !actionCalled {
		t.Fatal("materialized 子资源写 action 未被调用")
	}
}

func TestResolveTaskForReadReturnsProjectedWithoutWrites(t *testing.T) {
	svc, series, _, day2, _ := newOccurrenceMergeFixture(t)
	store := svc.store
	ws, _ := store.LocalWorkspace()
	beforeCount := occurrenceRowCount(t, svc, ws.ID)
	ref := OccurrenceRef(series.ID, day2)

	view, err := svc.ResolveTaskForRead(ref)
	if err != nil {
		t.Fatalf("ResolveTaskForRead: %v", err)
	}
	if view.RecurrenceInfo == nil || view.RecurrenceInfo.Materialization != "projected" {
		t.Fatalf("应为 projected: %#v", view.RecurrenceInfo)
	}
	if after := occurrenceRowCount(t, svc, ws.ID); after != beforeCount {
		t.Fatalf("读取不应物化: before=%d after=%d", beforeCount, after)
	}
}

// --- Task 7: reconcile + scheduler 测试 ---

func TestReconcileTaskSeriesMaterializesDueSlots(t *testing.T) {
	// daily series，first_due 在 3 天前，now 在今天。reconcile 应补齐 3 天。
	svc, closeFn := newTestService(t, 1783785599+2*86400) // now = first_due + 2天
	defer closeFn()
	store := svc.store
	ws, _ := store.LocalWorkspace()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	firstDue := int64(1783785599)
	series, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: firstDue,
	})
	if err != nil {
		t.Fatalf("AddTaskSeries: %v", err)
	}
	// first_due 已进入执行期，AddTaskSeries 已物化 1 条。再 reconcile 应补齐后 2 天。
	result, err := svc.ReconcileTaskSeries(series.Series.ID, svc.clock.Unix(), 100)
	if err != nil {
		t.Fatalf("ReconcileTaskSeries: %v", err)
	}
	// 至少创建 2 条（day2、day3）。
	if result.Created < 2 {
		t.Fatalf("Created = %d want >= 2: %#v", result.Created, result)
	}
	if count := occurrenceRowCount(t, svc, ws.ID); count < 3 {
		t.Fatalf("总物化数 = %d want >= 3", count)
	}
}

func TestDailySeriesKeepsYesterdayPendingWhenTodayOccurrenceIsGenerated(t *testing.T) {
	firstDue := int64(1783785599)
	svc, closeFn := newTestService(t, firstDue)
	defer closeFn()
	project, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: project.ID,
		RecurrenceRule: "daily", FirstDue: firstDue,
	})
	if err != nil {
		t.Fatal(err)
	}
	yesterday, err := svc.taskOccurrenceRepo.GetOccurrence(svc.workspaceID, created.Series.ID, firstDue)
	if err != nil {
		t.Fatalf("GetOccurrence(yesterday): %v", err)
	}
	if yesterday.Status != domain.StatusPending {
		t.Fatalf("yesterday status = %q", yesterday.Status)
	}

	today := firstDue + 86400
	svc.clock = FixedClock{NowUnix: today}
	if _, err := svc.ReconcileTaskSeries(created.Series.ID, today, 100); err != nil {
		t.Fatalf("ReconcileTaskSeries(today): %v", err)
	}
	todayTask, err := svc.taskOccurrenceRepo.GetOccurrence(svc.workspaceID, created.Series.ID, today)
	if err != nil {
		t.Fatalf("GetOccurrence(today): %v", err)
	}
	yesterdayAfter, err := svc.taskOccurrenceRepo.GetOccurrence(svc.workspaceID, created.Series.ID, firstDue)
	if err != nil {
		t.Fatalf("GetOccurrence(yesterday after): %v", err)
	}
	if todayTask.UUID == yesterday.UUID || todayTask.Status != domain.StatusPending || yesterdayAfter.Status != domain.StatusPending {
		t.Fatalf("daily occurrences not independent: yesterday=%#v today=%#v", yesterdayAfter, todayTask)
	}
}

func TestFiveDayBacklogRemainsFullyVisibleAndReconcileCompletesIdempotently(t *testing.T) {
	firstDue := int64(1783785599)
	svc, closeFn := newTestService(t, firstDue-86400)
	defer closeFn()
	project, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: project.ID,
		RecurrenceRule: "daily", FirstDue: firstDue,
	})
	if err != nil {
		t.Fatal(err)
	}
	day5 := firstDue + 4*86400
	svc.clock = FixedClock{NowUnix: day5}
	firstRun, err := svc.ReconcileTaskSeries(created.Series.ID, day5, 2)
	if err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	if firstRun.Created != 2 || firstRun.BacklogRemaining != 3 {
		t.Fatalf("first reconcile = %#v, want created=2 backlog=3", firstRun)
	}

	page, err := svc.QueryTaskViews(TaskViewQuery{
		OccurrenceMode: OccurrenceModeExpand,
		Range:          &TaskViewRange{Start: firstDue, End: day5 + 1},
	})
	if err != nil {
		t.Fatalf("QueryTaskViews expand backlog: %v", err)
	}
	if len(page.Items) != 5 {
		t.Fatalf("visible backlog items = %d, want 5", len(page.Items))
	}
	materialized := 0
	for _, item := range page.Items {
		if item.RecurrenceInfo != nil && item.RecurrenceInfo.Materialization == "materialized" {
			materialized++
		}
	}
	if materialized != 2 {
		t.Fatalf("materialized visible items = %d, want 2", materialized)
	}

	secondRun, err := svc.ReconcileTaskSeries(created.Series.ID, day5, 100)
	if err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	if secondRun.Created != 3 || secondRun.BacklogRemaining != 0 {
		t.Fatalf("second reconcile = %#v, want created=3 backlog=0", secondRun)
	}
	thirdRun, err := svc.ReconcileTaskSeries(created.Series.ID, day5, 100)
	if err != nil {
		t.Fatalf("third reconcile: %v", err)
	}
	if thirdRun.Created != 0 || occurrenceRowCount(t, svc, svc.workspaceID) != 5 {
		t.Fatalf("third reconcile not idempotent: result=%#v rows=%d", thirdRun, occurrenceRowCount(t, svc, svc.workspaceID))
	}
}

func TestReconcileTaskSeriesIdempotent(t *testing.T) {
	svc, closeFn := newTestService(t, 1783785599+2*86400)
	defer closeFn()
	store := svc.store
	ws, _ := store.LocalWorkspace()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	series, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: 1783785599,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = svc.ReconcileTaskSeries(series.Series.ID, svc.clock.Unix(), 100)
	after1 := occurrenceRowCount(t, svc, ws.ID)
	// 二次 reconcile 不应新增。
	_, err = svc.ReconcileTaskSeries(series.Series.ID, svc.clock.Unix(), 100)
	if err != nil {
		t.Fatalf("二次 reconcile: %v", err)
	}
	after2 := occurrenceRowCount(t, svc, ws.ID)
	if after2 != after1 {
		t.Fatalf("二次 reconcile 应幂等: after1=%d after2=%d", after1, after2)
	}
}

func TestReconcileTaskSeriesRespectsUntilAndEndsSeries(t *testing.T) {
	// until 在今天之前，所有槽位应已过，series ended。
	firstDue := int64(1783785599)
	until := firstDue + 86400 // first_due + 1 天
	now := firstDue + 3*86400 // 3 天后
	svc, closeFn := newTestService(t, now)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	series, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: firstDue, Until: &until,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.ReconcileTaskSeries(series.Series.ID, now, 100)
	if err != nil {
		t.Fatalf("ReconcileTaskSeries: %v", err)
	}
	// until 是包含式：first_due + 1 天，共 2 个槽位。
	if result.Created < 1 {
		t.Fatalf("应补齐至少 1 条: %#v", result)
	}
	if !result.Ended {
		t.Fatalf("until 已过应 ended: %#v", result)
	}
}

func TestReconcileTaskSeriesRespectsPerSeriesLimit(t *testing.T) {
	// first_due 在 200 天前，daily，限制每轮 50 条 → backlog。
	firstDue := int64(1783785599)
	now := firstDue + 200*86400
	svc, closeFn := newTestService(t, now)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	series, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: firstDue,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.ReconcileTaskSeries(series.Series.ID, now, 50)
	if err != nil {
		t.Fatalf("ReconcileTaskSeries: %v", err)
	}
	if result.BacklogRemaining == 0 {
		t.Fatalf("应有余留 backlog: %#v", result)
	}
}

func TestStopTaskSeriesMarksStoppedAndStopsGeneration(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: 2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	view, err := svc.StopTaskSeries(created.Series.ID, StopTaskSeriesInput{})
	if err != nil {
		t.Fatalf("StopTaskSeries: %v", err)
	}
	if view.Status != taskseries.StatusStopped {
		t.Fatalf("status = %q want stopped", view.Status)
	}
	if view.EffectiveEndAt == nil {
		t.Fatal("stopped 应有 effective_end_at")
	}
	// reconcile stopped series 不应生成。
	result, err := svc.ReconcileTaskSeries(created.Series.ID, 10000, 100)
	if err != nil {
		t.Fatalf("reconcile stopped: %v", err)
	}
	if result.Created != 0 {
		t.Fatalf("stopped series 不应生成: %#v", result)
	}
}

func TestStopTaskSeriesDeleteOpenOccurrences(t *testing.T) {
	// 用真实时间戳避免 1970 年边界问题。first_due 在 now 之前 3 天。
	firstDue := int64(1783785599) // 2026-07-11 23:59:59 +08:00
	now := firstDue + 3*86400     // 3 天后
	svc, closeFn := newTestService(t, now)
	defer closeFn()
	store := svc.store
	ws, _ := store.LocalWorkspace()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: firstDue,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = svc.ReconcileTaskSeries(created.Series.ID, now, 100)
	beforeCount := occurrenceRowCount(t, svc, ws.ID)
	if beforeCount < 2 {
		t.Fatalf("前置物化不足: %d", beforeCount)
	}
	deleteOpen := true
	_, err = svc.StopTaskSeries(created.Series.ID, StopTaskSeriesInput{DeleteOpenOccurrences: &deleteOpen})
	if err != nil {
		t.Fatalf("StopTaskSeries delete_open: %v", err)
	}
	for slot := firstDue; slot <= now; slot += 86400 {
		occ, err := svc.taskOccurrenceRepo.GetOccurrence(ws.ID, created.Series.ID, slot)
		if err != nil {
			continue
		}
		if occ.Status == domain.StatusPending || occ.Status == domain.StatusWaiting {
			t.Fatalf("slot %d 仍为 open: %s", slot, occ.Status)
		}
	}
}

func TestSkipTaskSeriesOccurrenceCreatesTombstone(t *testing.T) {
	// first_due 在未来，AddTaskSeries 不物化；skip 物化为 tombstone。
	firstDue := int64(1783785599 + 30*86400) // 30 天后
	now := int64(1783785599)
	svc, closeFn := newTestService(t, now)
	defer closeFn()
	store := svc.store
	ws, _ := store.LocalWorkspace()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: firstDue,
	})
	if err != nil {
		t.Fatal(err)
	}
	ref := OccurrenceRef(created.Series.ID, firstDue)
	beforeCount := occurrenceRowCount(t, svc, ws.ID)
	if beforeCount != 0 {
		t.Fatalf("未来 first_due 不应预物化: before=%d", beforeCount)
	}
	view, err := svc.SkipTaskSeriesOccurrence(created.Series.ID, ref)
	if err != nil {
		t.Fatalf("SkipTaskSeriesOccurrence: %v", err)
	}
	if view.Status != domain.StatusDeleted {
		t.Fatalf("status = %q want deleted", view.Status)
	}
	if after := occurrenceRowCount(t, svc, ws.ID); after != 1 {
		t.Fatalf("应新增 1 条 tombstone: after=%d", after)
	}
}

func TestListTaskSeriesOccurrencesPaged(t *testing.T) {
	firstDue := time.Date(2030, time.January, 1, 23, 59, 59, 0, time.Local).Unix()
	now := time.Date(2030, time.January, 5, 23, 59, 59, 0, time.Local).Unix()
	svc, closeFn := newTestService(t, now)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: firstDue,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = svc.ReconcileTaskSeries(created.Series.ID, now, 100)
	page, err := svc.ListTaskSeriesOccurrences(created.Series.ID, TaskSeriesOccurrenceListInput{
		Status: "pending", Limit: 2, Offset: 1,
	})
	if err != nil {
		t.Fatalf("ListTaskSeriesOccurrences: %v", err)
	}
	if page.Total != 5 || page.Limit != 2 || page.Offset != 1 || len(page.Items) != 2 {
		t.Fatalf("page = %#v, want total=5 limit=2 offset=1 items=2", page)
	}
	for index, it := range page.Items {
		if it.Status != domain.StatusPending {
			t.Fatalf("status = %q want pending", it.Status)
		}
		if it.RecurrenceInfo == nil {
			t.Fatal("occurrence 应有 recurrence_info")
		}
		wantSlot := firstDue + int64(index+1)*24*60*60
		if it.RecurrenceInfo.RecurrenceAt != wantSlot {
			t.Fatalf("item %d recurrence_at = %d, want %d", index, it.RecurrenceInfo.RecurrenceAt, wantSlot)
		}
	}
}

func TestListTaskSeriesOccurrencesUsesLeftClosedRightOpenRange(t *testing.T) {
	firstDue := time.Date(2030, time.January, 1, 23, 59, 59, 0, time.Local).Unix()
	now := time.Date(2030, time.January, 2, 23, 59, 59, 0, time.Local).Unix()
	svc, closeFn := newTestService(t, now)
	defer closeFn()
	project, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: project.ID,
		RecurrenceRule: "daily", FirstDue: firstDue,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReconcileTaskSeries(created.Series.ID, now, 100); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2030, time.January, 1, 0, 0, 0, 0, time.Local).Unix()
	end := time.Date(2030, time.January, 2, 0, 0, 0, 0, time.Local).Unix()
	page, err := svc.ListTaskSeriesOccurrences(created.Series.ID, TaskSeriesOccurrenceListInput{
		Status: "all", DueAfter: &start, DueBefore: &end,
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].Due == nil || *page.Items[0].Due != firstDue {
		t.Fatalf("range page = %#v, want only Jan 1 occurrence", page)
	}
}

func TestListTaskSeriesOccurrencesWithoutRangeIncludesAllMaterializedHistory(t *testing.T) {
	now := time.Date(2026, time.July, 14, 12, 0, 0, 0, time.Local).Unix()
	firstDue := time.Date(2050, time.January, 1, 23, 59, 59, 0, time.Local).Unix()
	svc, closeFn := newTestService(t, now)
	defer closeFn()
	project, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "远期巡检", ProjectID: project.ID,
		RecurrenceRule: "daily", FirstDue: firstDue,
	})
	if err != nil {
		t.Fatal(err)
	}
	ref := OccurrenceRef(created.Series.ID, firstDue)
	if _, err := svc.SkipTaskSeriesOccurrence(created.Series.ID, ref); err != nil {
		t.Fatal(err)
	}
	page, err := svc.ListTaskSeriesOccurrences(created.Series.ID, TaskSeriesOccurrenceListInput{Status: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != ref {
		t.Fatalf("unbounded history = %#v, want far-future tombstone", page)
	}
}

func TestProjectTransitionStopsActiveSeries(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: 5000,
	})
	if err != nil {
		t.Fatal(err)
	}
	// archive 项目应停止 series。
	if _, err := svc.TransitionProject(proj.Slug, "archived"); err != nil {
		t.Fatalf("TransitionProject archived: %v", err)
	}
	series, err := svc.taskSeriesRepo.Get(svc.workspaceID, created.Series.ID)
	if err != nil {
		t.Fatalf("Get series: %v", err)
	}
	if series.Status != taskseries.StatusStopped {
		t.Fatalf("series status = %q want stopped", series.Status)
	}
	if series.StopReason == nil || *series.StopReason != taskseries.StopReasonProjectArchived {
		t.Fatalf("stop_reason = %#v want project_archived", series.StopReason)
	}
	// stopped series 不再生成。
	result, err := svc.ReconcileTaskSeries(created.Series.ID, 10000, 100)
	if err != nil {
		t.Fatalf("reconcile after stop: %v", err)
	}
	if result.Created != 0 {
		t.Fatalf("stopped series 不应生成: %#v", result)
	}
}

func TestTaskSeriesSchedulerRunOnce(t *testing.T) {
	// daily series，first_due 在 3 天前。scheduler RunOnce 应补齐。
	firstDue := int64(1783785599)
	now := firstDue + 3*86400
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, now, "local", "local")
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: firstDue,
	})
	if err != nil {
		t.Fatal(err)
	}
	scheduler := NewTaskSeriesScheduler(TaskSeriesSchedulerOptions{
		Store: store,
		Clock: FixedClock{NowUnix: now},
		ServiceFactory: func(workspaceID string) *Service {
			return svc
		},
	})
	result, err := scheduler.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if result.Created == 0 {
		t.Fatalf("应补齐至少 1 条: %#v", result)
	}
	_ = created
}

func TestTaskSeriesSchedulerRunOnceUsesDefaultServiceFactory(t *testing.T) {
	firstDue := int64(1783785599)
	now := firstDue + 2*86400
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, now, "local", "local")
	proj, err := svc.AddProject(AddProjectInput{Slug: "scheddef", Name: "Scheduler Default"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日默认工厂巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: firstDue,
	}); err != nil {
		t.Fatal(err)
	}

	scheduler := NewTaskSeriesScheduler(TaskSeriesSchedulerOptions{
		Store: store,
		Clock: FixedClock{NowUnix: now},
	})
	result, err := scheduler.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce with default factory: %v", err)
	}
	if result.Created == 0 {
		t.Fatalf("default factory should reconcile occurrences: %#v", result)
	}
}

func TestTaskSeriesSchedulerRunOnceRejectsMissingStore(t *testing.T) {
	scheduler := NewTaskSeriesScheduler(TaskSeriesSchedulerOptions{})
	if _, err := scheduler.RunOnce(t.Context()); err == nil {
		t.Fatal("missing store 应返回错误")
	}
}

func TestProjectScopedServiceCannotReadOrSkipForeignOccurrence(t *testing.T) {
	now := int64(1783785599)
	store := newTestStore(t)
	base := newTestServiceWithRuntime(t, store, now, "local", "local")
	allowed, err := base.AddProject(AddProjectInput{Slug: "allowed", Name: "Allowed"})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := base.AddProject(AddProjectInput{Slug: "foreign", Name: "Foreign"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := base.AddTaskSeries(AddTaskSeriesInput{
		Title: "外部项目巡检", ProjectID: foreign.ID,
		RecurrenceRule: "daily", FirstDue: now + 86400,
	})
	if err != nil {
		t.Fatal(err)
	}
	ref := created.FirstOccurrence.ID
	scope := RequestScope{ProjectIDs: []string{allowed.ID}, Capabilities: []string{"task:read", "task:write"}}
	scoped, err := NewService(ServiceOptions{
		Store: store, Clock: FixedClock{NowUnix: now}, RequestScope: &scope,
		Runtime:               &RuntimeContext{ActorType: "user", ActorUserID: "local", WorkspaceID: base.workspaceID, Role: RoleOwner},
		DisableScopeBootstrap: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scoped.GetTaskView(ref); err == nil {
		t.Fatal("GetTaskView foreign occurrence error = nil")
	}
	if _, err := scoped.ListTaskSeriesOccurrences(created.Series.ID, TaskSeriesOccurrenceListInput{}); err == nil {
		t.Fatal("ListTaskSeriesOccurrences foreign series error = nil")
	}
	if _, err := scoped.SkipTaskSeriesOccurrence(created.Series.ID, ref); err == nil {
		t.Fatal("SkipTaskSeriesOccurrence foreign series error = nil")
	}
	if _, err := base.taskOccurrenceRepo.GetOccurrence(base.workspaceID, created.Series.ID, *created.FirstOccurrence.Due); err != storage.ErrOccurrenceNotFound {
		t.Fatalf("denied skip must not materialize occurrence, got %v", err)
	}
}

// --- Task 5: ModifyTaskSeries 测试 ---

func TestModifyTaskSeriesUpdatesSharedFields(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: 5000,
	})
	if err != nil {
		t.Fatal(err)
	}
	newTitle := "每周巡检"
	view, err := svc.ModifyTaskSeries(created.Series.ID, ModifyTaskSeriesInput{Title: &newTitle})
	if err != nil {
		t.Fatalf("ModifyTaskSeries: %v", err)
	}
	if view.Title != "每周巡检" {
		t.Fatalf("title = %q want 每周巡检", view.Title)
	}
}

func TestModifyTaskSeriesRejectsInactive(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: 5000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StopTaskSeries(created.Series.ID, StopTaskSeriesInput{}); err != nil {
		t.Fatal(err)
	}
	newTitle := "x"
	_, err = svc.ModifyTaskSeries(created.Series.ID, ModifyTaskSeriesInput{Title: &newTitle})
	if err == nil {
		t.Fatal("stopped series 应拒绝修改")
	}
}

func TestModifyTaskSeriesRuleRequiresEffectiveFrom(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: 5000,
	})
	if err != nil {
		t.Fatal(err)
	}
	newRule := "weekly"
	_, err = svc.ModifyTaskSeries(created.Series.ID, ModifyTaskSeriesInput{RecurrenceRule: &newRule})
	if err == nil {
		t.Fatal("修改 rule 缺 effective_from 应失败")
	}
}

func TestModifyTaskSeriesRuleRejectsExistingRuleVersionStart(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: 100000,
	})
	if err != nil {
		t.Fatal(err)
	}
	newRule := "weekly"
	_, err = svc.ModifyTaskSeries(created.Series.ID, ModifyTaskSeriesInput{
		RecurrenceRule: &newRule, EffectiveFrom: &created.Series.FirstDue,
	})
	runtimeErr, ok := err.(RuntimeError)
	if !ok || runtimeErr.Code != "task_series_invalid_effective_from" {
		t.Fatalf("error = %#v want task_series_invalid_effective_from", err)
	}
}

func TestModifyTaskSeriesRuleAppendsVersion(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: 5000,
	})
	if err != nil {
		t.Fatal(err)
	}
	newRule := "weekly"
	eff := int64(10000)
	view, err := svc.ModifyTaskSeries(created.Series.ID, ModifyTaskSeriesInput{
		RecurrenceRule: &newRule, EffectiveFrom: &eff,
	})
	if err != nil {
		t.Fatalf("ModifyTaskSeries rule: %v", err)
	}
	if view.RecurrenceRule != "weekly" {
		t.Fatalf("rule = %q want weekly", view.RecurrenceRule)
	}
	// rule version 应有 2 段。
	if len(view.RuleVersions) != 2 {
		t.Fatalf("rule versions = %d want 2", len(view.RuleVersions))
	}
}

func TestModifyTaskSeriesRuleRejectsPastEffectiveFrom(t *testing.T) {
	svc, closeFn := newTestService(t, 5000)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: 6000,
	})
	if err != nil {
		t.Fatal(err)
	}
	newRule := "weekly"
	pastEff := int64(4000) // 早于 now=5000
	_, err = svc.ModifyTaskSeries(created.Series.ID, ModifyTaskSeriesInput{
		RecurrenceRule: &newRule, EffectiveFrom: &pastEff,
	})
	if err == nil {
		t.Fatal("effective_from 早于 now 应失败")
	}
}

func TestModifyTaskSeriesSyncsSharedFieldsToOpenOccurrences(t *testing.T) {
	// first_due 在过去，物化 first occurrence；修改 title 应同步到该 occurrence。
	firstDue := int64(1000)
	now := int64(5000)
	svc, closeFn := newTestService(t, now)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: firstDue,
	})
	if err != nil {
		t.Fatal(err)
	}
	// AddTaskSeries 已物化 first occurrence。修改 title。
	newTitle := "每周巡检"
	if _, err := svc.ModifyTaskSeries(created.Series.ID, ModifyTaskSeriesInput{Title: &newTitle}); err != nil {
		t.Fatalf("ModifyTaskSeries: %v", err)
	}
	// 验证 occurrence title 同步。
	occ, err := svc.taskOccurrenceRepo.GetOccurrence(svc.workspaceID, created.Series.ID, firstDue)
	if err != nil {
		t.Fatalf("GetOccurrence: %v", err)
	}
	if occ.Title != "每周巡检" {
		t.Fatalf("occurrence title = %q want 每周巡检", occ.Title)
	}
}

func TestModifyTaskSeriesAssigneesDoNotRewriteMaterializedOccurrences(t *testing.T) {
	firstDue := int64(1000)
	svc, closeFn := newTestService(t, 5000)
	defer closeFn()
	ws, err := svc.store.LocalWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	alice := mustCreateUserRecord(t, svc.store, storage.User{
		ID: "user-alice-series", Name: "alice-series", CreatedAt: 100, ModifiedAt: 100,
	})
	mustUpsertMembershipRecord(t, svc.store, storage.Membership{
		UserID: alice.ID, WorkspaceID: ws.ID,
		Role: string(RoleMember), JoinedAt: 100, ModifiedAt: 100,
	})
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID, RecurrenceRule: "daily", FirstDue: firstDue,
		Assignees: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}

	updated, err := svc.ModifyTaskSeries(created.Series.ID, ModifyTaskSeriesInput{
		Assignees: []string{"alice-series"},
	})
	if err != nil {
		t.Fatalf("ModifyTaskSeries: %v", err)
	}
	if len(updated.Assignees) != 1 || updated.Assignees[0].ID != alice.ID {
		t.Fatalf("series assignees = %#v, want alice", updated.Assignees)
	}

	occurrence, err := svc.taskOccurrenceRepo.GetOccurrence(svc.workspaceID, created.Series.ID, firstDue)
	if err != nil {
		t.Fatal(err)
	}
	if len(occurrence.Assignees) != 1 || occurrence.Assignees[0].Name != "local" {
		t.Fatalf("materialized occurrence assignees = %#v, want original local assignee", occurrence.Assignees)
	}
}

func TestModifyTaskSeriesRollsBackWhenAuditFails(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	project, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: project.ID, RecurrenceRule: "daily", FirstDue: 5000,
	})
	if err != nil {
		t.Fatal(err)
	}
	svc.auditRepo = &failingAuditRepo{}
	newTitle := "不应保留的标题"
	if _, err := svc.ModifyTaskSeries(created.Series.ID, ModifyTaskSeriesInput{Title: &newTitle}); err == nil {
		t.Fatal("audit 失败时 ModifyTaskSeries 应失败")
	}
	persisted, err := svc.taskSeriesRepo.Get(svc.workspaceID, created.Series.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Title != "每日巡检" {
		t.Fatalf("series title = %q want rollback to 每日巡检", persisted.Title)
	}
}

func TestModifyTaskSeriesSyncsOccurrenceBeyondOneYear(t *testing.T) {
	now := int64(1000)
	svc, closeFn := newTestService(t, now)
	defer closeFn()
	project, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "长期巡检", ProjectID: project.ID, RecurrenceRule: "daily", FirstDue: 5000,
	})
	if err != nil {
		t.Fatal(err)
	}
	farSlot := now + 2*365*86400
	rule := "daily"
	seriesID := created.Series.ID
	if _, _, err := svc.taskOccurrenceRepo.CreateOccurrence(domain.Task{
		UUID: "far-occurrence", WorkspaceID: svc.workspaceID, ProjectID: &project.ID,
		Title: "长期巡检", Status: domain.StatusPending, Entry: now, Modified: now,
		Due: &farSlot, SeriesID: &seriesID, RecurrenceAt: &farSlot,
		RecurrenceRuleSnapshot: &rule,
	}); err != nil {
		t.Fatal(err)
	}
	newTitle := "长期巡检已更新"
	if _, err := svc.ModifyTaskSeries(seriesID, ModifyTaskSeriesInput{Title: &newTitle}); err != nil {
		t.Fatal(err)
	}
	occurrence, err := svc.taskOccurrenceRepo.GetOccurrence(svc.workspaceID, seriesID, farSlot)
	if err != nil {
		t.Fatal(err)
	}
	if occurrence.Title != newTitle {
		t.Fatalf("far occurrence title = %q want %q", occurrence.Title, newTitle)
	}
}

func TestTaskSeriesOccurrencesInheritUDAs(t *testing.T) {
	svc, closeFn := newTestService(t, 5000)
	defer closeFn()
	if err := svc.DefineUDA("estimate", "numeric", "Estimate", nil, ""); err != nil {
		t.Fatal(err)
	}
	project, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: project.ID, RecurrenceRule: "daily", FirstDue: 5000,
		UDAs: map[string]string{"estimate": "3"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.FirstOccurrence == nil || created.FirstOccurrence.UDAs["estimate"].Raw != "3" {
		t.Fatalf("first occurrence UDAs = %#v", created.FirstOccurrence)
	}
	persisted, err := svc.taskOccurrenceRepo.GetOccurrence(svc.workspaceID, created.Series.ID, 5000)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.UDAs["estimate"].Raw != "3" || persisted.UDAs["estimate"].Type != "numeric" {
		t.Fatalf("persisted occurrence UDAs = %#v", persisted.UDAs)
	}
}

func TestModifyTaskSeriesClearsSharedFields(t *testing.T) {
	svc, closeFn := newTestService(t, 5000)
	defer closeFn()
	project, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	description, priority, until := "旧说明", "H", int64(90000)
	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", Description: &description, ProjectID: project.ID,
		RecurrenceRule: "daily", FirstDue: 6000, Until: &until,
		Priority: &priority, Tags: []string{"ops"},
	})
	if err != nil {
		t.Fatal(err)
	}
	view, err := svc.ModifyTaskSeries(created.Series.ID, ModifyTaskSeriesInput{
		ClearDescription: true, ClearPriority: true, ClearTags: true, ClearUntil: true,
	})
	if err != nil {
		t.Fatalf("ModifyTaskSeries clear: %v", err)
	}
	if view.Description != nil || view.Priority != nil || view.Until != nil || len(view.Tags) != 0 {
		t.Fatalf("clear result = %#v", view)
	}
}

// --- Task 4: RunTaskViewReport 测试 ---

func TestRunTaskViewReportReturnsPage(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	// 创建普通任务。
	if _, err := svc.Add(AddInput{Title: "普通任务", Project: &proj.Slug}); err != nil {
		t.Fatal(err)
	}
	// 运行 list report（status:pending）。
	page, err := svc.RunTaskViewReport(ReportViewInput{
		Name: "list", OccurrenceMode: OccurrenceModeMaterialized,
	})
	if err != nil {
		t.Fatalf("RunTaskViewReport: %v", err)
	}
	if page.Total == 0 {
		t.Fatal("应至少有 1 个任务")
	}
	if len(page.Items) != page.Total {
		t.Fatalf("items=%d total=%d 应一致（无分页）", len(page.Items), page.Total)
	}
	// 验证返回的是 TaskOccurrenceView。
	for _, it := range page.Items {
		if it.ID == "" {
			t.Fatal("item ID 为空")
		}
	}
}

func TestRunTaskViewReportAppliesQueryFilter(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(AddInput{Title: "巡检A", Project: &proj.Slug}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(AddInput{Title: "报告B", Project: &proj.Slug}); err != nil {
		t.Fatal(err)
	}
	// 用 list report + bare text 过滤只留"巡检"。
	queryExpr, err := query.ParseQuery("巡检")
	if err != nil {
		t.Fatal(err)
	}
	page, err := svc.RunTaskViewReport(ReportViewInput{
		Name: "list", Query: queryExpr, OccurrenceMode: OccurrenceModeMaterialized,
	})
	if err != nil {
		t.Fatalf("RunTaskViewReport: %v", err)
	}
	if page.Total != 1 {
		t.Fatalf("total=%d want 1（只匹配巡检A）", page.Total)
	}
	if len(page.Items) != 1 || page.Items[0].Title != "巡检A" {
		t.Fatalf("items = %#v", page.Items)
	}
}

func TestRunTaskViewReportPreservesDependencyScopes(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	blocker, err := svc.Add(AddInput{Title: "blocker"})
	if err != nil {
		t.Fatal(err)
	}
	blockedTask, err := svc.Add(AddInput{Title: "blocked", Depends: []string{blocker.UUID}})
	if err != nil {
		t.Fatal(err)
	}

	blocked, err := svc.RunTaskViewReport(ReportViewInput{Name: "blocked"})
	if err != nil {
		t.Fatal(err)
	}
	if len(blocked.Items) != 1 || blocked.Items[0].UUID == nil || *blocked.Items[0].UUID != blockedTask.UUID {
		t.Fatalf("blocked report = %#v, want %s", blocked.Items, blockedTask.UUID)
	}
	ready, err := svc.RunTaskViewReport(ReportViewInput{Name: "ready"})
	if err != nil {
		t.Fatal(err)
	}
	if len(ready.Items) != 1 || ready.Items[0].UUID == nil || *ready.Items[0].UUID != blocker.UUID {
		t.Fatalf("ready report = %#v, want %s", ready.Items, blocker.UUID)
	}
}

func TestQueryTaskViewsExpandAppliesQueryFilter(t *testing.T) {
	svc, _, day1, _, _ := newOccurrenceMergeFixture(t)
	// day1 物化的 occurrence 标题为"每日巡检"。
	// 用 bare text query 过滤只匹配"每日巡检"。
	expr, err := query.ParseQuery("每日巡检")
	if err != nil {
		t.Fatal(err)
	}
	page, err := svc.QueryTaskViews(TaskViewQuery{
		OccurrenceMode: OccurrenceModeExpand,
		Range:          &TaskViewRange{Start: day1, End: day1 + 3*86400},
		Query:          expr,
	})
	if err != nil {
		t.Fatalf("QueryTaskViews: %v", err)
	}
	// 所有结果都应含"每日巡检"。
	for _, it := range page.Items {
		if !strings.Contains(it.Title, "每日巡检") {
			t.Fatalf("query 过滤失败，出现不匹配项: %q", it.Title)
		}
	}
	if page.Total == 0 {
		t.Fatal("应至少匹配 1 项")
	}
}

// TestProjectTaskSummaryIncludesOccurrencesAndReportsSeriesMetrics 验证 §17.4：
// 已物化的循环 occurrence 与普通任务一同计入风险计数和完成度，
// series 运行情况仍单独通过 SeriesMetrics 报告。
func TestProjectTaskSummaryIncludesOccurrencesAndReportsSeriesMetrics(t *testing.T) {
	now := int64(1783785599)
	svc, closeFn := newTestService(t, now)
	defer closeFn()

	proj, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	duePast := now - 3600
	// 普通逾期任务。
	if _, err := svc.Add(AddInput{Title: "普通逾期", Project: &proj.Slug, Due: &duePast}); err != nil {
		t.Fatalf("Add(普通逾期): %v", err)
	}
	// 创建 series，first_due 在过去 → 物化 occurrence（pending）。
	firstDue := now - 86400
	if _, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: proj.ID,
		RecurrenceRule: "daily", FirstDue: firstDue, Assignees: []string{"local"},
	}); err != nil {
		t.Fatalf("AddTaskSeries: %v", err)
	}

	view, err := svc.ProjectTaskSummary("ops")
	if err != nil {
		t.Fatalf("ProjectTaskSummary: %v", err)
	}
	// 物化的 occurrence（due 在过去、pending）与普通逾期任务一同计入。
	if view.OverdueCount != 2 {
		t.Fatalf("OverdueCount = %d, want 2（普通逾期 + occurrence）", view.OverdueCount)
	}
	if view.HighPriorityOpenCount != 0 {
		t.Fatalf("HighPriorityOpenCount = %d, want 0", view.HighPriorityOpenCount)
	}
	// 未分配只统计无 assignee 的任务；occurrence 已分配 local，故仍是 1 条。
	if view.UnassignedOpenCount != 1 {
		t.Fatalf("UnassignedOpenCount = %d, want 1", view.UnassignedOpenCount)
	}
	// 成员待办：已物化且未完成的循环实例按其负责人计入。
	var localWorkload *ProjectSummaryWorkloadView
	for i := range view.Workload {
		if view.Workload[i].User != nil && view.Workload[i].User.ID == svc.runtime.ActorUserID {
			localWorkload = &view.Workload[i]
			break
		}
	}
	if localWorkload == nil || localWorkload.OpenCount != 1 || localWorkload.OverdueCount != 1 {
		t.Fatalf("local workload = %#v, want one overdue materialized occurrence", localWorkload)
	}
	// 项目完成度：普通任务 + 物化 occurrence 一起计入（2 条 pending）。
	projectView, err := svc.ProjectInfo("ops")
	if err != nil {
		t.Fatalf("ProjectInfo: %v", err)
	}
	if projectView.TaskCount != 2 || projectView.PendingCount != 2 {
		t.Fatalf("project counts = total:%d pending:%d, want total:2 pending:2", projectView.TaskCount, projectView.PendingCount)
	}
	// series metrics：循环运行情况仍独立报告。
	if view.SeriesMetrics.RecurringSeriesCount != 1 {
		t.Fatalf("RecurringSeriesCount = %d, want 1", view.SeriesMetrics.RecurringSeriesCount)
	}
	if view.SeriesMetrics.ActiveRecurringSeriesCount != 1 {
		t.Fatalf("ActiveRecurringSeriesCount = %d, want 1", view.SeriesMetrics.ActiveRecurringSeriesCount)
	}
	// 物化的 occurrence pending（due 在过去）→ open=1, overdue=1。
	if view.SeriesMetrics.OpenRecurringOccurrenceCount != 1 {
		t.Fatalf("OpenRecurringOccurrenceCount = %d, want 1", view.SeriesMetrics.OpenRecurringOccurrenceCount)
	}
	if view.SeriesMetrics.OverdueRecurringOccurrenceCount != 1 {
		t.Fatalf("OverdueRecurringOccurrenceCount = %d, want 1", view.SeriesMetrics.OverdueRecurringOccurrenceCount)
	}
}

// TestQueryTaskViewsUrgencySortAndScore 验证 web console 任务列表页的默认排序路径
// （QueryTaskViews + Sort=urgency）按 urgency 降序，并把分数回填到 view.Urgency。
// 同时验证 next 是 urgency 的别名（UI 工具栏沿用 next 语义）。
func TestQueryTaskViewsUrgencySortAndScore(t *testing.T) {
	now := time.Date(2025, 7, 16, 12, 0, 0, 0, time.Local).Unix()
	svc, closeFn := newTestService(t, now)
	defer closeFn()

	project, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	// 高优 + 已逾期 due → 预期 urgency 最高。
	overdue := now - 86400
	high, err := svc.AddTaskView(AddInput{
		Title: "high overdue", Project: &project.Slug, Priority: strptr("H"), Due: &overdue,
	})
	if err != nil {
		t.Fatalf("AddTaskView high: %v", err)
	}
	// 低优 + 未来 due → urgency 次之。
	future := now + 7*86400
	low, err := svc.AddTaskView(AddInput{
		Title: "low future", Project: &project.Slug, Priority: strptr("L"), Due: &future,
	})
	if err != nil {
		t.Fatalf("AddTaskView low: %v", err)
	}
	// 无 due 无 priority → urgency 最低。
	plain, err := svc.AddTaskView(AddInput{Title: "plain", Project: &project.Slug})
	if err != nil {
		t.Fatalf("AddTaskView plain: %v", err)
	}

	// urgency 排序应让 high > low > plain。
	page, err := svc.QueryTaskViews(TaskViewQuery{Sort: "urgency", ProjectID: project.ID})
	if err != nil {
		t.Fatalf("QueryTaskViews urgency: %v", err)
	}
	wantOrder := []string{high.ID, low.ID, plain.ID}
	if got := viewIDs(page.Items); !slices.Equal(got, wantOrder) {
		t.Fatalf("urgency order = %v, want %v", got, wantOrder)
	}
	// 排序时应回填 urgency 分数。
	for _, v := range page.Items {
		if v.Urgency == nil {
			t.Fatalf("view %s Urgency not populated", v.ID)
		}
	}
	if page.Items[0].Urgency == nil || page.Items[1].Urgency == nil ||
		*page.Items[0].Urgency <= *page.Items[1].Urgency {
		t.Fatalf("urgency not descending: %v vs %v", page.Items[0].Urgency, page.Items[1].Urgency)
	}

	// next 是 urgency 的别名，应得到相同顺序。
	nextPage, err := svc.QueryTaskViews(TaskViewQuery{Sort: "next", ProjectID: project.ID})
	if err != nil {
		t.Fatalf("QueryTaskViews next: %v", err)
	}
	if got := viewIDs(nextPage.Items); !slices.Equal(got, wantOrder) {
		t.Fatalf("next order = %v, want %v (alias of urgency)", got, wantOrder)
	}
}

// TestQueryTaskViewsUrgencyScoreNotPopulatedByOtherSorts 验证非 urgency 排序不回填分数。
func TestQueryTaskViewsUrgencyScoreNotPopulatedByOtherSorts(t *testing.T) {
	now := time.Date(2025, 7, 16, 12, 0, 0, 0, time.Local).Unix()
	svc, closeFn := newTestService(t, now)
	defer closeFn()

	project, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddTaskView(AddInput{Title: "t1", Project: &project.Slug}); err != nil {
		t.Fatal(err)
	}
	page, err := svc.QueryTaskViews(TaskViewQuery{Sort: "entry", ProjectID: project.ID})
	if err != nil {
		t.Fatalf("QueryTaskViews entry: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(page.Items))
	}
	if page.Items[0].Urgency != nil {
		t.Fatalf("entry sort should not populate Urgency, got %v", page.Items[0].Urgency)
	}
}

func viewIDs(items []TaskOccurrenceView) []string {
	out := make([]string, 0, len(items))
	for _, v := range items {
		out = append(out, v.ID)
	}
	return out
}

// TestEntryLessIsEntryThenID 验证 tie-breaker：同主键时按 entry 升序，
// entry 相同/缺失时退回 ID。entry 为 nil 排最后。
func TestEntryLessIsEntryThenID(t *testing.T) {
	e1 := int64(100)
	e2 := int64(200)
	cases := []struct {
		name string
		a, b TaskOccurrenceView
		want bool // a 是否应排在 b 前
	}{
		{"entry earlier first", TaskOccurrenceView{ID: "z", Entry: &e1}, TaskOccurrenceView{ID: "a", Entry: &e2}, true},
		{"entry later second", TaskOccurrenceView{ID: "a", Entry: &e2}, TaskOccurrenceView{ID: "z", Entry: &e1}, false},
		{"same entry id asc", TaskOccurrenceView{ID: "b", Entry: &e1}, TaskOccurrenceView{ID: "a", Entry: &e1}, false},
		{"nil entry last", TaskOccurrenceView{ID: "a", Entry: nil}, TaskOccurrenceView{ID: "z", Entry: &e1}, false},
		{"both nil id asc", TaskOccurrenceView{ID: "a", Entry: nil}, TaskOccurrenceView{ID: "z", Entry: nil}, true},
	}
	for _, tc := range cases {
		if got := entryLess(tc.a, tc.b); got != tc.want {
			t.Fatalf("%s: entryLess = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestSortByTimestampThenEntryTieBreaker 验证 due/wait 等时间戳排序的同值兜底：
// 主键相同（这里 due 相同）时按 entry 升序，而非 UUID 字典序。
func TestSortByTimestampThenEntryTieBreaker(t *testing.T) {
	due := int64(500)
	e1, e2 := int64(100), int64(200)
	// 两条任务 due 相同、urgency 也相同；B 的 UUID 字典序更小但 entry 更晚。
	// 期望按 entry 排：A（entry 早）在前。
	items := []TaskOccurrenceView{
		{ID: "bbb", Entry: &e2, Due: &due}, // entry 晚
		{ID: "aaa", Entry: &e1, Due: &due}, // entry 早
	}
	sortByTimestampThenEntry(items, func(v TaskOccurrenceView) (int64, bool) {
		if v.Due == nil {
			return 0, false
		}
		return *v.Due, true
	}, false)
	if items[0].ID != "aaa" {
		t.Fatalf("tie-break by entry failed: order = %s,%s; want aaa(entry early) first", items[0].ID, items[1].ID)
	}
}
