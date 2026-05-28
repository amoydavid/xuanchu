package task

import (
	"encoding/json"
	"io"
	"slices"
	"strings"
	"testing"
)

func TestTaskJSONUsesTaskwarriorFieldNames(t *testing.T) {
	priority := "H"
	tsk := Task{
		UUID: "u1", Description: "write spec", Status: StatusPending,
		Entry: 100, Modified: 100, Priority: &priority, Tags: []string{"planning"},
	}
	dto := ToJSON(tsk)
	data, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	for _, field := range []string{`"uuid"`, `"description"`, `"status"`, `"entry"`, `"modified"`, `"priority"`, `"tags"`} {
		if !strings.Contains(string(data), field) {
			t.Fatalf("JSON %s missing field %s", data, field)
		}
	}
}

func TestJSONTaskM2RoundTrip(t *testing.T) {
	start, wait, scheduled, until := int64(10), int64(20), int64(30), int64(40)
	recur, parent, mask := "weekly", "parent", "mask"
	imask := 2
	tsk := Task{
		UUID: "u1", Description: "task", Status: StatusPending, Entry: 1, Modified: 2,
		Start: &start, Wait: &wait, Scheduled: &scheduled, Until: &until,
		Annotations: []Annotation{{Entry: 3, Description: "note"}},
		Depends:     []string{"dep"},
		Recur:       &recur, Parent: &parent, Mask: &mask, IMask: &imask,
	}
	got := FromJSON(ToJSON(tsk))
	if got.Start == nil || got.Wait == nil || got.Scheduled == nil || got.Until == nil {
		t.Fatalf("date fields lost: %#v", got)
	}
	if len(got.Annotations) != 1 || got.Annotations[0].Description != "note" || !slices.Equal(got.Depends, []string{"dep"}) {
		t.Fatalf("compound fields lost: %#v", got)
	}
	if got.Recur == nil || got.Parent == nil || got.Mask == nil || got.IMask == nil {
		t.Fatalf("recurrence fields lost: %#v", got)
	}
}

func TestTaskJSONCarriesUDAFields(t *testing.T) {
	tsk := Task{
		UUID: "u1", Description: "task", Status: StatusPending, Entry: 1, Modified: 2,
		UDAs: map[string]UDAValue{
			"estimate": {Name: "estimate", Raw: "3", Type: "numeric"},
			"reviewed": {Name: "reviewed", Raw: "2026-05-28T00:00:00Z", Type: "date"},
		},
	}
	data, err := json.Marshal(ToJSON(tsk))
	if err != nil {
		t.Fatalf("Marshal UDA task error = %v", err)
	}
	if !strings.Contains(string(data), `"estimate":"3"`) || !strings.Contains(string(data), `"reviewed":"2026-05-28T00:00:00Z"`) {
		t.Fatalf("UDA JSON missing fields: %s", data)
	}
	var dtos []JSONTask
	if err := UnmarshalJSONTasks(strings.NewReader("["+string(data)+"]"), &dtos); err != nil {
		t.Fatalf("Unmarshal UDA JSON error = %v", err)
	}
	got := FromJSON(dtos[0])
	if got.UDAs["estimate"].Raw != "3" || got.UDAs["reviewed"].Raw != "2026-05-28T00:00:00Z" {
		t.Fatalf("UDAs lost: %#v", got.UDAs)
	}
}

func TestUnmarshalJSONTasksPreservesOrphanUDA(t *testing.T) {
	var tasks []JSONTask
	err := UnmarshalJSONTasks(strings.NewReader(`[{"uuid":"u1","description":"task","status":"pending","entry":"1970-01-01T00:00:01Z","modified":"1970-01-01T00:00:02Z","legacy_field":{"x":1}}]`), &tasks)
	if err != nil {
		t.Fatalf("UnmarshalJSONTasks(orphan) error = %v", err)
	}
	got := FromJSON(tasks[0])
	legacy := got.UDAs["legacy_field"]
	if legacy.Raw != `{"x":1}` || !legacy.Orphan {
		t.Fatalf("legacy UDA = %#v", legacy)
	}
}

func TestFromJSONStrictRejectsInvalidDate(t *testing.T) {
	_, err := FromJSONStrict(JSONTask{
		UUID:        "u1",
		Description: "task",
		Status:      StatusPending,
		Entry:       "1970-01-01T00:00:01Z",
		Modified:    "not-a-date",
	})
	if err == nil {
		t.Fatal("FromJSONStrict() error = nil, want invalid date error")
	}
}

func TestUnmarshalJSONTasksPreservesNilVsEmptySlices(t *testing.T) {
	var missing []JSONTask
	if err := UnmarshalJSONTasks(strings.NewReader(`[{"uuid":"u1","description":"task","status":"pending","entry":"1970-01-01T00:00:01Z","modified":"1970-01-01T00:00:02Z"}]`), &missing); err != nil {
		t.Fatalf("UnmarshalJSONTasks(missing) error = %v", err)
	}
	if missing[0].Tags != nil {
		t.Fatalf("Tags when field missing = %#v, want nil", missing[0].Tags)
	}

	var empty []JSONTask
	if err := UnmarshalJSONTasks(strings.NewReader(`[{"uuid":"u1","description":"task","status":"pending","entry":"1970-01-01T00:00:01Z","modified":"1970-01-01T00:00:02Z","tags":[],"annotations":[],"depends":[]}]`), &empty); err != nil {
		t.Fatalf("UnmarshalJSONTasks(empty) error = %v", err)
	}
	if empty[0].Tags == nil || empty[0].Annotations == nil || empty[0].Depends == nil {
		t.Fatalf("explicit empty slices should stay non-nil: %#v", empty[0])
	}
	if len(empty[0].Tags) != 0 || len(empty[0].Annotations) != 0 || len(empty[0].Depends) != 0 {
		t.Fatalf("explicit empty slices should be empty: %#v", empty[0])
	}
}

func TestMarshalJSONTaskOmitsNilSlicesAndKeepsEmptyTagsWhenRequested(t *testing.T) {
	nilJSON, err := json.Marshal(JSONTask{
		UUID:        "u1",
		Description: "task",
		Status:      StatusPending,
		Entry:       "1970-01-01T00:00:01Z",
		Modified:    "1970-01-01T00:00:02Z",
	})
	if err != nil {
		t.Fatalf("Marshal(nil JSONTask) error = %v", err)
	}
	if strings.Contains(string(nilJSON), `"tags"`) {
		t.Fatalf("nil tags should be omitted: %s", nilJSON)
	}

	emptyJSON, err := json.Marshal(JSONTask{
		UUID:        "u1",
		Description: "task",
		Status:      StatusPending,
		Entry:       "1970-01-01T00:00:01Z",
		Modified:    "1970-01-01T00:00:02Z",
		Tags:        []string{},
	})
	if err != nil {
		t.Fatalf("Marshal(empty JSONTask) error = %v", err)
	}
	if !strings.Contains(string(emptyJSON), `"tags":[]`) {
		t.Fatalf("empty tags should be preserved for merge semantics: %s", emptyJSON)
	}
}

func TestUnmarshalJSONTasksRejectsInvalidJSON(t *testing.T) {
	var tasks []JSONTask
	if err := UnmarshalJSONTasks(strings.NewReader(`[`), &tasks); err == nil {
		t.Fatal("UnmarshalJSONTasks() error = nil, want error")
	}
}

func TestUnmarshalJSONTasksReadsArray(t *testing.T) {
	var tasks []JSONTask
	err := UnmarshalJSONTasks(strings.NewReader(`[{"uuid":"u1","description":"task","status":"pending","entry":"1970-01-01T00:00:01Z","modified":"1970-01-01T00:00:02Z"}]`), &tasks)
	if err != nil {
		t.Fatalf("UnmarshalJSONTasks() error = %v", err)
	}
	if len(tasks) != 1 || tasks[0].UUID != "u1" {
		t.Fatalf("tasks = %#v", tasks)
	}
}

func TestMarshalJSONTasksWritesArray(t *testing.T) {
	data, err := MarshalJSONTasks([]JSONTask{{
		UUID:        "u1",
		Description: "task",
		Status:      StatusPending,
		Entry:       "1970-01-01T00:00:01Z",
		Modified:    "1970-01-01T00:00:02Z",
	}})
	if err != nil {
		t.Fatalf("MarshalJSONTasks() error = %v", err)
	}
	if !strings.Contains(string(data), `"uuid":"u1"`) {
		t.Fatalf("MarshalJSONTasks() = %s", data)
	}
}

func TestUnmarshalJSONTasksFromReader(t *testing.T) {
	var tasks []JSONTask
	err := UnmarshalJSONTasks(io.NopCloser(strings.NewReader(`[{"uuid":"u1","description":"task","status":"pending","entry":"1970-01-01T00:00:01Z","modified":"1970-01-01T00:00:02Z"}]`)), &tasks)
	if err != nil {
		t.Fatalf("UnmarshalJSONTasks(reader) error = %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("len(tasks) = %d, want 1", len(tasks))
	}
}
