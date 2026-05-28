package app

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dajee/taskg/internal/storage/sqlite"
	"github.com/dajee/taskg/internal/task"
)

func newTestService(t *testing.T, now int64) (*Service, func()) {
	t.Helper()
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(ServiceOptions{Store: store, Clock: fixedClock{NowUnix: now}})
	if err != nil {
		t.Fatal(err)
	}
	return svc, func() { _ = store.Close() }
}

func TestServiceAddListInfo(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	svc, err := NewService(ServiceOptions{Store: store, Clock: fixedClock{NowUnix: 100}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	created, err := svc.Add(AddInput{Description: "write spec", Tags: []string{"planning"}})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if created.UUID == "" {
		t.Fatal("created UUID is empty")
	}

	tasks, err := svc.List(ListInput{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tasks) != 1 || tasks[0].Description != "write spec" {
		t.Fatalf("tasks = %#v", tasks)
	}

	got, err := svc.Info(created.UUID)
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if got.UUID != created.UUID {
		t.Fatalf("Info UUID = %q, want %q", got.UUID, created.UUID)
	}
}

func TestServiceModifyDoneDeleteByNumber(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	svc, _ := NewService(ServiceOptions{Store: store, Clock: fixedClock{NowUnix: 100}})

	_, err = svc.Add(AddInput{Description: "write spec"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	priority := "H"
	if err := svc.Modify("1", ModifyInput{Priority: &priority}); err != nil {
		t.Fatalf("Modify() error = %v", err)
	}
	got, err := svc.ResolveTarget("1")
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if got.Priority == nil || *got.Priority != "H" {
		t.Fatalf("Priority = %#v", got.Priority)
	}
	if err := svc.Done("1"); err != nil {
		t.Fatalf("Done() error = %v", err)
	}
	tasks, _ := svc.List(ListInput{})
	if len(tasks) != 0 {
		t.Fatalf("pending tasks = %#v, want empty", tasks)
	}
}

func TestServiceM2Mutations(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	dep, _ := svc.Add(AddInput{Description: "dep"})
	tsk, _ := svc.Add(AddInput{Description: "task"})

	if err := svc.Modify(tsk.UUID, ModifyInput{AddDepends: []string{dep.UUID}}); err != nil {
		t.Fatalf("Modify(depends) error = %v", err)
	}
	if err := svc.Start(tsk.UUID); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := svc.Annotate(tsk.UUID, "note"); err != nil {
		t.Fatalf("Annotate() error = %v", err)
	}
	if err := svc.AppendDescription(tsk.UUID, "suffix"); err != nil {
		t.Fatalf("AppendDescription() error = %v", err)
	}
	if err := svc.PrependDescription(tsk.UUID, "prefix"); err != nil {
		t.Fatalf("PrependDescription() error = %v", err)
	}
	got, err := svc.ResolveTarget(tsk.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Start == nil || len(got.Depends) != 1 || len(got.Annotations) != 1 || got.Description != "prefix task suffix" {
		t.Fatalf("M2 fields not updated: %#v", got)
	}
	if err := svc.Stop(tsk.UUID); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	got, _ = svc.ResolveTarget(tsk.UUID)
	if got.Start != nil {
		t.Fatalf("Start after Stop = %#v", got.Start)
	}
}

func TestServiceRejectsDependencyCycle(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	a, _ := svc.Add(AddInput{Description: "a"})
	b, _ := svc.Add(AddInput{Description: "b"})
	if err := svc.Modify(a.UUID, ModifyInput{AddDepends: []string{b.UUID}}); err != nil {
		t.Fatalf("Modify(a depends b) error = %v", err)
	}
	if err := svc.Modify(b.UUID, ModifyInput{AddDepends: []string{a.UUID}}); err == nil {
		t.Fatal("expected dependency cycle error")
	}
}

func TestServiceAddResolvesDependencyTargets(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	dep, _ := svc.Add(AddInput{Description: "dep"})
	tsk, err := svc.Add(AddInput{Description: "task", Depends: []string{"1"}})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	got, err := svc.ResolveTarget(tsk.UUID)
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if len(got.Depends) != 1 || got.Depends[0] != dep.UUID {
		t.Fatalf("Depends = %#v, want %q", got.Depends, dep.UUID)
	}
}

func TestServiceRejectsRecurringUnsupportedFields(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	dep, _ := svc.Add(AddInput{Description: "dep"})
	due := int64(200)
	wait := int64(150)
	scheduled := int64(160)
	recur := "daily"
	for _, tc := range []struct {
		name  string
		input AddInput
	}{
		{name: "wait", input: AddInput{Description: "task", Due: &due, Recur: &recur, Wait: &wait}},
		{name: "scheduled", input: AddInput{Description: "task", Due: &due, Recur: &recur, Scheduled: &scheduled}},
		{name: "depends", input: AddInput{Description: "task", Due: &due, Recur: &recur, Depends: []string{dep.UUID}}},
	} {
		if _, err := svc.Add(tc.input); err == nil {
			t.Fatalf("Add(%s) error = nil, want error", tc.name)
		}
	}
}

func TestServiceRejectsDeepDependencyCycle(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	a, _ := svc.Add(AddInput{Description: "a"})
	b, _ := svc.Add(AddInput{Description: "b"})
	c, _ := svc.Add(AddInput{Description: "c"})
	d, _ := svc.Add(AddInput{Description: "d"})
	if err := svc.Modify(a.UUID, ModifyInput{AddDepends: []string{b.UUID}}); err != nil {
		t.Fatalf("Modify(a depends b) error = %v", err)
	}
	if err := svc.Modify(b.UUID, ModifyInput{AddDepends: []string{c.UUID}}); err != nil {
		t.Fatalf("Modify(b depends c) error = %v", err)
	}
	if err := svc.Modify(c.UUID, ModifyInput{AddDepends: []string{d.UUID}}); err != nil {
		t.Fatalf("Modify(c depends d) error = %v", err)
	}
	if err := svc.Modify(d.UUID, ModifyInput{AddDepends: []string{a.UUID}}); err == nil {
		t.Fatal("expected deep dependency cycle error")
	}
}

func TestServiceModifyClearDependsBeforeAdding(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	dep1, _ := svc.Add(AddInput{Description: "dep1"})
	dep2, _ := svc.Add(AddInput{Description: "dep2"})
	tsk, _ := svc.Add(AddInput{Description: "task"})
	if err := svc.Modify(tsk.UUID, ModifyInput{AddDepends: []string{dep1.UUID}}); err != nil {
		t.Fatalf("Modify(initial depends) error = %v", err)
	}
	if err := svc.Modify(tsk.UUID, ModifyInput{ClearDepends: true, AddDepends: []string{dep2.UUID}}); err != nil {
		t.Fatalf("Modify(clear then add depends) error = %v", err)
	}
	got, err := svc.ResolveTarget(tsk.UUID)
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if len(got.Depends) != 1 || got.Depends[0] != dep2.UUID {
		t.Fatalf("Depends = %#v, want only %q", got.Depends, dep2.UUID)
	}
}

func TestServiceRejectsBlankAnnotateAppendAndPrepend(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	tsk, _ := svc.Add(AddInput{Description: "task"})
	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{name: "annotate", run: func() error { return svc.Annotate(tsk.UUID, " \t ") }},
		{name: "append", run: func() error { return svc.AppendDescription(tsk.UUID, "   ") }},
		{name: "prepend", run: func() error { return svc.PrependDescription(tsk.UUID, "\n\t") }},
	} {
		if err := tc.run(); err == nil {
			t.Fatalf("%s() error = nil, want error", tc.name)
		}
	}
	got, err := svc.ResolveTarget(tsk.UUID)
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if got.Description != "task" {
		t.Fatalf("Description = %q, want unchanged", got.Description)
	}
	if len(got.Annotations) != 0 {
		t.Fatalf("Annotations = %#v, want empty", got.Annotations)
	}
}

func TestServiceRejectsAnnotationNewline(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	tsk, _ := svc.Add(AddInput{Description: "task"})
	if err := svc.Annotate(tsk.UUID, "line1\nline2"); err == nil {
		t.Fatal("Annotate() error = nil, want newline validation error")
	}
	got, err := svc.ResolveTarget(tsk.UUID)
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if len(got.Annotations) != 0 {
		t.Fatalf("Annotations = %#v, want empty", got.Annotations)
	}
}

func TestServiceAllowsDuplicateAnnotationsInSameSecond(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	tsk, _ := svc.Add(AddInput{Description: "task"})
	if err := svc.Annotate(tsk.UUID, "note"); err != nil {
		t.Fatalf("Annotate(first) error = %v", err)
	}
	if err := svc.Annotate(tsk.UUID, "note"); err != nil {
		t.Fatalf("Annotate(second) error = %v", err)
	}
	got, err := svc.ResolveTarget(tsk.UUID)
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if len(got.Annotations) != 2 {
		t.Fatalf("Annotations = %#v, want two duplicate notes", got.Annotations)
	}
	if got.Annotations[0].Entry == got.Annotations[1].Entry {
		t.Fatalf("duplicate annotations kept same entry: %#v", got.Annotations)
	}
}

func TestServiceImportClearsTagsWithExplicitEmptyArray(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	created, err := svc.Add(AddInput{Description: "task", Tags: []string{"one", "two"}})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if _, err := svc.Import([]task.JSONTask{{
		UUID:        created.UUID,
		Description: created.Description,
		Status:      task.StatusPending,
		Entry:       "1970-01-01T00:01:40Z",
		Modified:    "1970-01-01T00:01:40Z",
		Tags:        []string{},
	}}); err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	got, err := svc.ResolveTarget(created.UUID)
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if got.Tags == nil {
		t.Fatal("Tags = nil, want explicit empty slice after clearing")
	}
	if len(got.Tags) != 0 {
		t.Fatalf("Tags = %#v, want empty", got.Tags)
	}
}

func TestServiceImportDoesNotClearTagsWhenFieldMissing(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	created, err := svc.Add(AddInput{Description: "task", Tags: []string{"one", "two"}})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	raw := `[
		{
			"uuid": "` + created.UUID + `",
			"description": "task",
			"status": "pending",
			"entry": "1970-01-01T00:01:40Z",
			"modified": "1970-01-01T00:01:40Z"
		}
	]`
	var payload []task.JSONTask
	if err := task.UnmarshalJSONTasks(strings.NewReader(raw), &payload); err != nil {
		t.Fatalf("UnmarshalJSONTasks() error = %v", err)
	}
	if _, err := svc.Import(payload); err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	got, err := svc.ResolveTarget(created.UUID)
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if len(got.Tags) != 2 {
		t.Fatalf("Tags = %#v, want preserved tags", got.Tags)
	}
}

func TestDefaultWorkingSetKeepsWaitingTasksAddressable(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	wait := int64(200)
	if _, err := svc.Add(AddInput{Description: "hidden wait", Wait: &wait}); err != nil {
		t.Fatalf("Add(waiting) error = %v", err)
	}
	visible, err := svc.Add(AddInput{Description: "visible"})
	if err != nil {
		t.Fatalf("Add(visible) error = %v", err)
	}
	got, err := svc.ResolveTarget("1")
	if err != nil {
		t.Fatalf("ResolveTarget(1) error = %v", err)
	}
	if got.UUID == visible.UUID {
		t.Fatalf("ResolveTarget(1) should keep waiting task addressable before visible %q", visible.Description)
	}
}

func TestServiceM2Reports(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	waitUntil := int64(200)
	expiredUntil := int64(90)
	active, _ := svc.Add(AddInput{Description: "active"})
	waiting, _ := svc.Add(AddInput{Description: "waiting", Wait: &waitUntil})
	expired, _ := svc.Add(AddInput{Description: "expired", Until: &expiredUntil})
	dep, _ := svc.Add(AddInput{Description: "dep"})
	blocked, _ := svc.Add(AddInput{Description: "blocked"})
	if err := svc.Start(active.UUID); err != nil {
		t.Fatalf("Start(active) error = %v", err)
	}
	if err := svc.Modify(blocked.UUID, ModifyInput{AddDepends: []string{dep.UUID}}); err != nil {
		t.Fatalf("Modify(blocked depends) error = %v", err)
	}

	cases := map[string]string{
		"active":   active.UUID,
		"waiting":  waiting.UUID,
		"blocked":  blocked.UUID,
		"blocking": dep.UUID,
	}
	for reportName, wantUUID := range cases {
		got, err := svc.ListReport(reportName, ListInput{})
		if err != nil {
			t.Fatalf("ListReport(%s) error = %v", reportName, err)
		}
		if !containsTask(got, wantUUID) {
			t.Fatalf("ListReport(%s) = %#v, missing %s", reportName, got, wantUUID)
		}
		if containsTask(got, expired.UUID) {
			t.Fatalf("ListReport(%s) includes expired until task: %#v", reportName, got)
		}
	}
	all, err := svc.ListReport("all", ListInput{})
	if err != nil {
		t.Fatalf("ListReport(all) error = %v", err)
	}
	if !containsTask(all, expired.UUID) {
		t.Fatalf("all should include until-expired task: %#v", all)
	}
}

func TestUntilExpiredDependencyDoesNotBlockLiveTask(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	expiredUntil := int64(90)
	expired, _ := svc.Add(AddInput{Description: "expired", Until: &expiredUntil})
	live, _ := svc.Add(AddInput{Description: "live"})
	if err := svc.Modify(live.UUID, ModifyInput{AddDepends: []string{expired.UUID}}); err != nil {
		t.Fatalf("Modify(live depends expired) error = %v", err)
	}
	blocked, err := svc.ListReport("blocked", ListInput{})
	if err != nil {
		t.Fatalf("ListReport(blocked) error = %v", err)
	}
	if containsTask(blocked, live.UUID) {
		t.Fatalf("blocked includes live task with expired dependency: %#v", blocked)
	}
	ready, err := svc.ListReport("ready", ListInput{})
	if err != nil {
		t.Fatalf("ListReport(ready) error = %v", err)
	}
	if !containsTask(ready, live.UUID) {
		t.Fatalf("ready missing live task with expired dependency: %#v", ready)
	}
}

func TestUrgencyUsesDependencyState(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	blocker, _ := svc.Add(AddInput{Description: "blocker"})
	blocked, _ := svc.Add(AddInput{Description: "blocked"})
	plain, _ := svc.Add(AddInput{Description: "plain"})
	if err := svc.Modify(blocked.UUID, ModifyInput{AddDepends: []string{blocker.UUID}}); err != nil {
		t.Fatal(err)
	}
	blockedU, err := svc.ExplainUrgency(blocked.UUID)
	if err != nil {
		t.Fatal(err)
	}
	plainU, err := svc.ExplainUrgency(plain.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if blockedU.Total >= plainU.Total {
		t.Fatalf("blocked urgency = %.3f, plain = %.3f; blocked should be lower", blockedU.Total, plainU.Total)
	}
}

func TestDoneChildDoesNotRecurWhenParentDeleted(t *testing.T) {
	svc, closeFn := newTestService(t, mustUnix(t, "2030-01-01T10:00:00Z"))
	defer closeFn()
	due := mustUnix(t, "2030-01-01T23:59:59Z")
	until := mustUnix(t, "2030-02-01T23:59:59Z")
	recur := "daily"
	parent, err := svc.Add(AddInput{Description: "daily task", Due: &due, Until: &until, Recur: &recur})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	tasks, err := svc.List(ListInput{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("children = %#v", tasks)
	}
	child := tasks[0]
	if err := svc.Delete(parent.UUID); err != nil {
		t.Fatalf("Delete(parent) error = %v", err)
	}
	if err := svc.Done(child.UUID); err != nil {
		t.Fatalf("Done(child) error = %v", err)
	}
	tasks, err = svc.List(ListInput{})
	if err != nil {
		t.Fatalf("List() after Done(child) error = %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("child generated after deleted parent: %#v", tasks)
	}
}

func TestServiceRecurringAddAndDoneCreatesNextChild(t *testing.T) {
	svc, closeFn := newTestService(t, mustUnix(t, "2030-01-01T10:00:00Z"))
	defer closeFn()
	due := mustUnix(t, "2030-01-01T23:59:59Z")
	until := mustUnix(t, "2030-02-01T23:59:59Z")
	recur := "daily"
	parent, err := svc.Add(AddInput{Description: "daily task", Due: &due, Until: &until, Recur: &recur})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if parent.Status != task.StatusRecurring {
		t.Fatalf("parent status = %s", parent.Status)
	}
	tasks, err := svc.List(ListInput{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tasks) != 1 || tasks[0].Parent == nil || *tasks[0].Parent != parent.UUID {
		t.Fatalf("visible child not created: %#v", tasks)
	}
	firstChild := tasks[0]
	if err := svc.Done(firstChild.UUID); err != nil {
		t.Fatalf("Done(child) error = %v", err)
	}
	tasks, err = svc.List(ListInput{})
	if err != nil {
		t.Fatalf("List() after done error = %v", err)
	}
	if len(tasks) != 1 || tasks[0].UUID == firstChild.UUID {
		t.Fatalf("next child not generated: %#v", tasks)
	}
}

func TestRecurringStopsAtUntil(t *testing.T) {
	svc, closeFn := newTestService(t, mustUnix(t, "2030-01-01T10:00:00Z"))
	defer closeFn()
	due := mustUnix(t, "2030-01-01T23:59:59Z")
	until := due
	recur := "daily"
	if _, err := svc.Add(AddInput{Description: "daily", Due: &due, Until: &until, Recur: &recur}); err != nil {
		t.Fatal(err)
	}
	tasks, err := svc.List(ListInput{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("children = %#v", tasks)
	}
	if err := svc.Done(tasks[0].UUID); err != nil {
		t.Fatal(err)
	}
	tasks, err = svc.List(ListInput{})
	if err != nil {
		t.Fatalf("List() after done error = %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("child generated after until: %#v", tasks)
	}
}

func containsTask(tasks []task.Task, uuid string) bool {
	for _, tsk := range tasks {
		if tsk.UUID == uuid {
			return true
		}
	}
	return false
}

func mustUnix(t *testing.T, value string) int64 {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("time.Parse(%q) error = %v", value, err)
	}
	return parsed.Unix()
}
