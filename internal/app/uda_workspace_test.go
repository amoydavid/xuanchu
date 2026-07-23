package app

import (
	"errors"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/auth"
)

func TestWorkspaceUDAProtectsActiveSeriesAndReportsUsage(t *testing.T) {
	svc, closeFn := newTestService(t, 5000)
	defer closeFn()
	if err := svc.DefineUDA("estimate", "numeric", "工作量", []string{"1", "2", "3"}, "2"); err != nil {
		t.Fatal(err)
	}
	project, err := svc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	series, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "每日巡检", ProjectID: project.ID, RecurrenceRule: "daily", FirstDue: 5000,
		UDAs: map[string]string{"estimate": "3"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ordinary, err := svc.Add(AddInput{Title: "普通任务", Project: &project.Slug, UDAs: map[string]string{"estimate": "1"}})
	if err != nil {
		t.Fatal(err)
	}

	rows, err := svc.WorkspaceListUDAs()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ActiveSeriesValueCount != 1 || rows[0].TaskValueCount != 2 {
		t.Fatalf("usage=%#v, want task=2 active_series=1", rows)
	}

	_, err = svc.WorkspaceSetUDA("estimate", WorkspaceUDAInput{Type: "numeric", Values: []string{"1", "2"}})
	assertUDARuntimeErrorCode(t, err, "uda_active_series_incompatible")
	_, err = svc.WorkspaceSetUDA("estimate", WorkspaceUDAInput{Type: "string"})
	assertUDARuntimeErrorCode(t, err, "uda_active_series_incompatible")
	assertUDARuntimeErrorCode(t, svc.WorkspaceDeleteUDA("estimate"), "uda_active_series_in_use")

	if _, err := svc.StopTaskSeries(series.Series.ID, StopTaskSeriesInput{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.WorkspaceDeleteUDA("estimate"); err != nil {
		t.Fatalf("stopped series should not block delete: %v", err)
	}
	if task, err := svc.Info(ordinary.UUID); err != nil || task.UDAs["estimate"].Raw != "1" {
		t.Fatalf("historical task UDA changed after definition delete: task=%#v err=%v", task, err)
	}
}

func TestWorkspaceUDAWriteOnlyTenantCanSetWithoutReadCapability(t *testing.T) {
	store := newTestStore(t)
	owner := newTestServiceWithRuntime(t, store, 100, "local", "local")
	runtime := RuntimeContext{
		ActorType: auth.TokenTypeTenantAccess, ActorTokenID: "tenant-token", ActorTokenName: "tenant",
		WorkspaceID: owner.workspaceID, WorkspaceSlug: owner.Runtime().WorkspaceSlug,
	}
	scope := RequestScope{WorkspaceIDs: []string{owner.workspaceID}, Capabilities: []string{auth.ScopeConfigWrite}}
	tenant, err := NewService(ServiceOptions{
		Store: store, Clock: FixedClock{NowUnix: 100}, Runtime: &runtime, RequestScope: &scope, DisableScopeBootstrap: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	row, err := tenant.WorkspaceSetUDA("source", WorkspaceUDAInput{Type: "string", Values: []string{"paid", "organic"}})
	if err != nil {
		t.Fatalf("WorkspaceSetUDA with config:write: %v", err)
	}
	if row.Name != "source" || row.Source != "database" {
		t.Fatalf("row=%#v", row)
	}
	if _, err := tenant.WorkspaceListUDAs(); err == nil {
		t.Fatal("WorkspaceListUDAs with only config:write should fail")
	}
}

func assertUDARuntimeErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	var runtimeErr RuntimeError
	if !errors.As(err, &runtimeErr) || runtimeErr.Code != code {
		t.Fatalf("error=%v, want RuntimeError(%s)", err, code)
	}
}
