package app

import (
	"path/filepath"
	"testing"

	"github.com/dajee/taskg/internal/storage/sqlite"
)

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
