package app

import (
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/task"
)

func intptr(i int64) *int64 { return &i }

func TestDiffTaskChanges_NoChanges(t *testing.T) {
	before := task.Task{UUID: "a", Title: "test"}
	after := before
	diff := diffTaskChanges(before, after)
	if diff.PriorityChanged || diff.DueChanged || diff.ProjectChanged || diff.TagsChanged || diff.AssigneesChanged {
		t.Fatal("expected no changes")
	}
}

func TestDiffTaskChanges_PriorityChanged(t *testing.T) {
	before := task.Task{UUID: "a"}
	after := task.Task{UUID: "a", Priority: strptr("H")}
	diff := diffTaskChanges(before, after)
	if !diff.PriorityChanged {
		t.Fatal("expected PriorityChanged")
	}
	if diff.PreviousPriority != nil {
		t.Fatalf("PreviousPriority = %v, want nil", diff.PreviousPriority)
	}
	if *diff.CurrentPriority != "H" {
		t.Fatalf("CurrentPriority = %v, want H", diff.CurrentPriority)
	}
}

func TestDiffTaskChanges_DueChanged(t *testing.T) {
	before := task.Task{UUID: "a", Due: intptr(100)}
	after := task.Task{UUID: "a", Due: intptr(200)}
	diff := diffTaskChanges(before, after)
	if !diff.DueChanged {
		t.Fatal("expected DueChanged")
	}
	if *diff.PreviousDue != 100 {
		t.Fatalf("PreviousDue = %d, want 100", *diff.PreviousDue)
	}
	if *diff.CurrentDue != 200 {
		t.Fatalf("CurrentDue = %d, want 200", *diff.CurrentDue)
	}
}

func TestDiffTaskChanges_DueCleared(t *testing.T) {
	before := task.Task{UUID: "a", Due: intptr(100)}
	after := task.Task{UUID: "a"}
	diff := diffTaskChanges(before, after)
	if !diff.DueChanged {
		t.Fatal("expected DueChanged when due cleared")
	}
	if *diff.PreviousDue != 100 {
		t.Fatalf("PreviousDue = %d, want 100", *diff.PreviousDue)
	}
	if diff.CurrentDue != nil {
		t.Fatalf("CurrentDue = %v, want nil", diff.CurrentDue)
	}
}

func TestDiffTaskChanges_ProjectChanged(t *testing.T) {
	before := task.Task{UUID: "a", Project: strptr("old")}
	after := task.Task{UUID: "a", Project: strptr("new")}
	diff := diffTaskChanges(before, after)
	if !diff.ProjectChanged {
		t.Fatal("expected ProjectChanged")
	}
	if *diff.PreviousProject != "old" {
		t.Fatalf("PreviousProject = %v, want old", diff.PreviousProject)
	}
	if *diff.CurrentProject != "new" {
		t.Fatalf("CurrentProject = %v, want new", diff.CurrentProject)
	}
}

func TestDiffTaskChanges_TagsChanged(t *testing.T) {
	before := task.Task{UUID: "a", Tags: []string{"a", "b"}}
	after := task.Task{UUID: "a", Tags: []string{"b", "c"}}
	diff := diffTaskChanges(before, after)
	if !diff.TagsChanged {
		t.Fatal("expected TagsChanged")
	}
	if len(diff.AddedTags) != 1 || diff.AddedTags[0] != "c" {
		t.Fatalf("AddedTags = %v, want [c]", diff.AddedTags)
	}
	if len(diff.RemovedTags) != 1 || diff.RemovedTags[0] != "a" {
		t.Fatalf("RemovedTags = %v, want [a]", diff.RemovedTags)
	}
}

func TestDiffTaskChanges_AssigneesChanged(t *testing.T) {
	before := task.Task{UUID: "a", Assignees: []task.AssigneeInfo{{UserID: "u1"}, {UserID: "u2"}}}
	after := task.Task{UUID: "a", Assignees: []task.AssigneeInfo{{UserID: "u1"}, {UserID: "u3"}}}
	diff := diffTaskChanges(before, after)
	if !diff.AssigneesChanged {
		t.Fatal("expected AssigneesChanged")
	}
	if len(diff.AddedAssignees) != 0 {
		t.Fatalf("AddedAssignees should be empty before hydrate, got %v", diff.AddedAssignees)
	}
}

func TestBuildFineGrainedEvents_PriorityChanged(t *testing.T) {
	diff := TaskChangeDiff{
		PriorityChanged:  true,
		PreviousPriority: strptr("M"),
		CurrentPriority:  strptr("H"),
	}
	after := task.Task{UUID: "t1", Priority: strptr("H")}
	events := buildFineGrainedEvents(diff, after, RuntimeContext{}, 1000)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventType != "task.priority_changed" {
		t.Fatalf("event type = %q, want task.priority_changed", events[0].EventType)
	}
	if events[0].Data["previous_priority"] != "M" {
		t.Fatalf("previous_priority = %v, want M", events[0].Data["previous_priority"])
	}
	if events[0].Data["current_priority"] != "H" {
		t.Fatalf("current_priority = %v, want H", events[0].Data["current_priority"])
	}
}

func TestBuildFineGrainedEvents_MultipleChanges(t *testing.T) {
	diff := TaskChangeDiff{
		PriorityChanged:  true,
		PreviousPriority: strptr("L"),
		CurrentPriority:  strptr("H"),
		TagsChanged:      true,
		AddedTags:        []string{"urgent"},
		RemovedTags:      []string{"docs"},
	}
	after := task.Task{UUID: "t1", Priority: strptr("H"), Tags: []string{"urgent"}}
	events := buildFineGrainedEvents(diff, after, RuntimeContext{}, 1000)
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	types := map[string]bool{}
	for _, e := range events {
		types[e.EventType] = true
	}
	if !types["task.priority_changed"] || !types["task.tags_changed"] {
		t.Fatalf("expected task.priority_changed and task.tags_changed, got %v", types)
	}
}

func TestBuildFineGrainedEvents_NoChanges(t *testing.T) {
	diff := TaskChangeDiff{}
	after := task.Task{UUID: "t1"}
	events := buildFineGrainedEvents(diff, after, RuntimeContext{}, 1000)
	if len(events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(events))
	}
}

func TestBuildFineGrainedEvents_DueChanged(t *testing.T) {
	diff := TaskChangeDiff{
		DueChanged:  true,
		PreviousDue: intptr(100),
		CurrentDue:  intptr(200),
	}
	after := task.Task{UUID: "t1", Due: intptr(200)}
	events := buildFineGrainedEvents(diff, after, RuntimeContext{}, 1000)
	if len(events) != 1 || events[0].EventType != "task.due_changed" {
		t.Fatalf("expected task.due_changed, got %v", events)
	}
}

func TestBuildFineGrainedEvents_ProjectChanged(t *testing.T) {
	diff := TaskChangeDiff{
		ProjectChanged:  true,
		PreviousProject: strptr("old"),
		CurrentProject:  strptr("new"),
	}
	after := task.Task{UUID: "t1", Project: strptr("new")}
	events := buildFineGrainedEvents(diff, after, RuntimeContext{}, 1000)
	if len(events) != 1 || events[0].EventType != "task.project_changed" {
		t.Fatalf("expected task.project_changed, got %v", events)
	}
}
