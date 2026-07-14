package task

import (
	"bytes"
	"encoding/json"
	"io"
	"slices"
	"strings"
	"testing"
)

func TestTaskJSONUsesTitleAsPrimaryField(t *testing.T) {
	priority := "H"
	detail := "long form detail"
	tsk := Task{
		UUID: "u1", Title: "write spec", Description: &detail, Status: StatusPending,
		Entry: 100, Modified: 100, Priority: &priority, Tags: []string{"planning"},
	}
	dto := ToJSON(tsk)
	data, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	for _, field := range []string{`"uuid"`, `"title"`, `"description"`, `"status"`, `"entry"`, `"modified"`, `"priority"`, `"tags"`} {
		if !strings.Contains(string(data), field) {
			t.Fatalf("JSON %s missing field %s", data, field)
		}
	}
	if strings.Contains(string(data), `"description":"write spec"`) {
		t.Fatalf("description should not carry title: %s", data)
	}
}

func TestTaskJSONOmitsEmptyDescription(t *testing.T) {
	dto := ToJSON(Task{UUID: "u1", Title: "task", Status: StatusPending, Entry: 1, Modified: 2})
	data, err := json.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(`"description"`)) {
		t.Fatalf("description should be omitted when empty: %s", data)
	}
	if !bytes.Contains(data, []byte(`"title":"task"`)) {
		t.Fatalf("title missing: %s", data)
	}
}

func TestJSONTaskImportRequiresTitleAndAllowsOptionalDescription(t *testing.T) {
	var dto JSONTask
	raw := `{"uuid":"u1","title":"task title","description":"task detail","status":"pending","entry":"1970-01-01T00:00:01Z","modified":"1970-01-01T00:00:02Z"}`
	if err := json.Unmarshal([]byte(raw), &dto); err != nil {
		t.Fatalf("Unmarshal(title) error = %v", err)
	}
	got, err := FromJSONStrict(dto)
	if err != nil {
		t.Fatalf("FromJSONStrict(title) error = %v", err)
	}
	if got.Title != "task title" || got.Description == nil || *got.Description != "task detail" {
		t.Fatalf("task text = title %q description %#v", got.Title, got.Description)
	}

	raw = `{"uuid":"u1","description":"legacy title","status":"pending","entry":"1970-01-01T00:00:01Z","modified":"1970-01-01T00:00:02Z"}`
	if err := json.Unmarshal([]byte(raw), &dto); err != nil {
		t.Fatalf("Unmarshal(missing title) error = %v", err)
	}
	if _, err := FromJSONStrict(dto); err == nil || !strings.Contains(err.Error(), "title") {
		t.Fatalf("FromJSONStrict(missing title) error = %v, want title error", err)
	}
}

func TestJSONTaskExportsTaskSlugWhenProjectSeqPresent(t *testing.T) {
	project := "api"
	seq := int64(12)
	dto := ToJSON(Task{
		UUID:       "u1",
		Title:      "task",
		Status:     StatusPending,
		Entry:      1,
		Modified:   2,
		Project:    &project,
		ProjectSeq: &seq,
	})
	data, err := json.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"task_slug":"api-12"`)) {
		t.Fatalf("task_slug missing: %s", data)
	}
}

func TestJSONTaskOmitsTaskSlugWithoutProject(t *testing.T) {
	seq := int64(12)
	dto := ToJSON(Task{UUID: "u1", Title: "task", Status: StatusPending, Entry: 1, Modified: 2, ProjectSeq: &seq})
	data, err := json.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("task_slug")) {
		t.Fatalf("task_slug should be omitted without project: %s", data)
	}
}

func TestJSONTaskAcceptsExportedTaskSlugButDoesNotImportProjectSeq(t *testing.T) {
	var dto JSONTask
	raw := `{"uuid":"u1","title":"task","status":"pending","entry":"1970-01-01T00:00:01Z","modified":"1970-01-01T00:00:02Z","project":"api","task_slug":"api-12"}`
	if err := json.Unmarshal([]byte(raw), &dto); err != nil {
		t.Fatalf("Unmarshal(task_slug) error = %v", err)
	}
	got := FromJSON(dto)
	if got.ProjectSeq != nil {
		t.Fatalf("ProjectSeq = %#v, want nil because task_slug is derived", got.ProjectSeq)
	}
	if dto.TaskSlug == nil || *dto.TaskSlug != "api-12" {
		t.Fatalf("TaskSlug = %#v, want api-12", dto.TaskSlug)
	}
}

func TestJSONTaskRejectsReadonlyProjectSeq(t *testing.T) {
	var dto JSONTask
	raw := `{"uuid":"u1","title":"task","status":"pending","entry":"1970-01-01T00:00:01Z","modified":"1970-01-01T00:00:02Z","project_seq":1}`
	err := json.Unmarshal([]byte(raw), &dto)
	if err == nil || !strings.Contains(err.Error(), "project_seq") {
		t.Fatalf("Unmarshal(project_seq) error = %v, want readonly/reserved", err)
	}
}

func TestJSONTaskExportsAssignees(t *testing.T) {
	email := "alice@example.com"
	tsk := Task{
		UUID:     "u1",
		Title:    "write spec",
		Status:   StatusPending,
		Entry:    100,
		Modified: 100,
		Assignees: []AssigneeInfo{
			{UserID: "user-alice", Name: "alice", Email: &email},
			{UserID: "user-bob", Name: "bob"},
		},
	}

	data, err := json.Marshal(ToJSON(tsk))
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	for _, field := range []string{
		`"assignees":[`,
		`"id":"user-alice"`,
		`"name":"alice"`,
		`"email":"alice@example.com"`,
		`"id":"user-bob"`,
		`"name":"bob"`,
	} {
		if !strings.Contains(string(data), field) {
			t.Fatalf("JSON %s missing field %s", data, field)
		}
	}
}

func TestJSONTaskImportSupportsObjectAssignees(t *testing.T) {
	var dto JSONTask
	err := json.Unmarshal([]byte(`{
		"uuid":"u1",
		"title":"task",
		"status":"pending",
		"entry":"1970-01-01T00:00:01Z",
		"modified":"1970-01-01T00:00:02Z",
		"assignees":[
			{"id":"user-alice","name":"alice","email":"alice@example.com"},
			{"id":"user-bob","name":"bob"}
		]
	}`), &dto)
	if err != nil {
		t.Fatalf("Unmarshal(object assignees) error = %v", err)
	}

	got, err := FromJSONStrict(dto)
	if err != nil {
		t.Fatalf("FromJSONStrict(object assignees) error = %v", err)
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

func TestJSONTaskImportSupportsStringAssignees(t *testing.T) {
	var dto JSONTask
	err := json.Unmarshal([]byte(`{
		"uuid":"u1",
		"title":"task",
		"status":"pending",
		"entry":"1970-01-01T00:00:01Z",
		"modified":"1970-01-01T00:00:02Z",
		"assignees":["user-alice","alice@example.com"]
	}`), &dto)
	if err != nil {
		t.Fatalf("Unmarshal(string assignees) error = %v", err)
	}

	got, err := FromJSONStrict(dto)
	if err != nil {
		t.Fatalf("FromJSONStrict(string assignees) error = %v", err)
	}
	if len(got.Assignees) != 2 {
		t.Fatalf("Assignees = %#v, want 2 entries", got.Assignees)
	}
	if got.Assignees[0].UserID != "user-alice" || got.Assignees[0].Name != "" || got.Assignees[0].Email != nil {
		t.Fatalf("first assignee = %#v", got.Assignees[0])
	}
	if got.Assignees[1].UserID != "" || got.Assignees[1].Name != "" || got.Assignees[1].Email == nil || *got.Assignees[1].Email != "alice@example.com" {
		t.Fatalf("second assignee = %#v", got.Assignees[1])
	}
}

func TestJSONTaskExportsAssigneeExternalIDs(t *testing.T) {
	email := "alice@example.com"
	tsk := Task{
		UUID:     "u1",
		Title:    "test",
		Status:   StatusPending,
		Entry:    1700000000,
		Modified: 1700000000,
		Assignees: []AssigneeInfo{
			{
				UserID: "user-1",
				Name:   "alice",
				Email:  &email,
				ExternalIDs: []ExternalIDInfo{
					{Provider: "feishu", ExternalID: "ou_abc"},
					{Provider: "slack", ExternalID: "U_ABC"},
				},
			},
		},
	}
	dto := ToJSON(tsk)
	if len(dto.Assignees) != 1 {
		t.Fatalf("expected 1 assignee, got %d", len(dto.Assignees))
	}
	if len(dto.Assignees[0].ExternalIDs) != 2 {
		t.Fatalf("expected 2 external IDs, got %d", len(dto.Assignees[0].ExternalIDs))
	}
	if dto.Assignees[0].ExternalIDs[0].Provider != "feishu" {
		t.Fatalf("expected provider feishu, got %s", dto.Assignees[0].ExternalIDs[0].Provider)
	}
	if dto.Assignees[0].ExternalIDs[1].ExternalID != "U_ABC" {
		t.Fatalf("expected external_id U_ABC, got %s", dto.Assignees[0].ExternalIDs[1].ExternalID)
	}
}

func TestJSONTaskImportPreservesAssigneeExternalIDs(t *testing.T) {
	var dto JSONTask
	err := json.Unmarshal([]byte(`{
		"uuid":"u2",
		"title":"test",
		"status":"pending",
		"entry":"1970-01-01T00:00:01Z",
		"modified":"1970-01-01T00:00:02Z",
		"assignees":[{"id":"user-1","name":"bob","external_ids":[{"provider":"feishu","external_id":"ou_bob"}]}]
	}`), &dto)
	if err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	tsk := FromJSON(dto)
	if len(tsk.Assignees) != 1 {
		t.Fatalf("expected 1 assignee, got %d", len(tsk.Assignees))
	}
	if len(tsk.Assignees[0].ExternalIDs) != 1 {
		t.Fatalf("expected 1 external ID, got %d", len(tsk.Assignees[0].ExternalIDs))
	}
	if tsk.Assignees[0].ExternalIDs[0].Provider != "feishu" {
		t.Fatalf("expected provider feishu, got %s", tsk.Assignees[0].ExternalIDs[0].Provider)
	}
	if tsk.Assignees[0].ExternalIDs[0].ExternalID != "ou_bob" {
		t.Fatalf("expected external_id ou_bob, got %s", tsk.Assignees[0].ExternalIDs[0].ExternalID)
	}
}

func TestJSONTaskM2RoundTrip(t *testing.T) {
	start, wait, scheduled, until := int64(10), int64(20), int64(30), int64(40)
	parent := "parent"
	tsk := Task{
		UUID: "u1", Title: "task", Status: StatusPending, Entry: 1, Modified: 2,
		Start: &start, Wait: &wait, Scheduled: &scheduled, Until: &until,
		Annotations: []Annotation{{ID: "ann-1", Entry: 3, Description: "note"}},
		Depends:     []string{"dep"},
		Parent:      &parent,
	}
	got := FromJSON(ToJSON(tsk))
	if got.Start == nil || got.Wait == nil || got.Scheduled == nil || got.Until == nil {
		t.Fatalf("date fields lost: %#v", got)
	}
	if len(got.Annotations) != 1 || got.Annotations[0].ID != "ann-1" || got.Annotations[0].Description != "note" || !slices.Equal(got.Depends, []string{"dep"}) {
		t.Fatalf("compound fields lost: %#v", got)
	}
	if got.Parent == nil || *got.Parent != parent {
		t.Fatalf("parent field lost: %#v", got.Parent)
	}
}

// TestJSONTaskRejectsLegacyRecurField 锁定旧 recur/mask/imask 字段在 decode 时被拒绝（spec §11.1、§20.2）。
func TestJSONTaskRejectsLegacyRecurField(t *testing.T) {
	for _, legacy := range []string{`"recur":"daily"`, `"mask":"abc"`, `"imask":1`} {
		payload := `{"uuid":"u1","title":"t","status":"pending","entry":"1","modified":"2",` + legacy + `}`
		var dto JSONTask
		if err := json.Unmarshal([]byte(payload), &dto); err == nil {
			t.Fatalf("包含 %s 的 payload 应被拒绝", legacy)
		}
	}
}

// TestJSONTaskOccurrenceRoundTrip 锁定 occurrence 持久字段的 JSON round-trip（spec §7.2）。
func TestJSONTaskOccurrenceRoundTrip(t *testing.T) {
	seriesID := "series-1"
	slot := int64(1783785599)
	rule := "daily"
	tsk := Task{
		UUID: "occ-1", Title: "巡检", Status: StatusPending, Entry: 1, Modified: 2,
		SeriesID: &seriesID, RecurrenceAt: &slot, RecurrenceRuleSnapshot: &rule,
		RecurrenceOverrides: []string{"due", "title", "due"},
	}
	dto := ToJSON(tsk)
	if dto.SeriesID == nil || *dto.SeriesID != seriesID {
		t.Fatalf("series_id 丢失: %#v", dto.SeriesID)
	}
	if dto.RecurrenceAt == nil {
		t.Fatalf("recurrence_at 丢失")
	}
	if dto.RecurrenceRuleSnapshot == nil || *dto.RecurrenceRuleSnapshot != rule {
		t.Fatalf("recurrence_rule_snapshot 丢失: %#v", dto.RecurrenceRuleSnapshot)
	}
	// override 应被规范化为去重升序。
	if !slices.Equal(dto.RecurrenceOverrides, []string{"due", "title"}) {
		t.Fatalf("overrides 未规范化: %#v", dto.RecurrenceOverrides)
	}
	got := FromJSON(dto)
	if got.SeriesID == nil || *got.SeriesID != seriesID {
		t.Fatalf("round-trip series_id 丢失: %#v", got.SeriesID)
	}
	if got.RecurrenceAt == nil || *got.RecurrenceAt != slot {
		t.Fatalf("round-trip recurrence_at 丢失: %#v", got.RecurrenceAt)
	}
	if got.RecurrenceRuleSnapshot == nil || *got.RecurrenceRuleSnapshot != rule {
		t.Fatalf("round-trip snapshot 丢失: %#v", got.RecurrenceRuleSnapshot)
	}
	// 普通任务（全 nil）不应输出 occurrence 字段。
	plain := ToJSON(Task{UUID: "p1", Title: "普通任务", Status: StatusPending, Entry: 1, Modified: 2})
	data, err := json.Marshal(plain)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "series_id") || strings.Contains(string(data), "recurrence_at") {
		t.Fatalf("普通任务不应输出 occurrence 字段: %s", data)
	}
}

func TestTaskJSONCarriesUDAFields(t *testing.T) {
	tsk := Task{
		UUID: "u1", Title: "task", Status: StatusPending, Entry: 1, Modified: 2,
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

func TestJSONTaskSkipsReservedUDAFields(t *testing.T) {
	tsk := Task{
		UUID: "u1", Title: "task", Status: StatusPending, Entry: 1, Modified: 2,
		UDAs: map[string]UDAValue{
			"estimate":   {Name: "estimate", Raw: "3", Type: "numeric"},
			"project_id": {Name: "project_id", Raw: "project-1", Type: "string"},
		},
	}
	data, err := json.Marshal(ToJSON(tsk))
	if err != nil {
		t.Fatalf("Marshal UDA task error = %v", err)
	}
	if bytes.Contains(data, []byte("project_id")) {
		t.Fatalf("reserved UDA leaked into JSON: %s", data)
	}
	if !bytes.Contains(data, []byte(`"estimate":"3"`)) {
		t.Fatalf("non-reserved UDA missing from JSON: %s", data)
	}
}

func TestUnmarshalJSONTasksPreservesOrphanUDA(t *testing.T) {
	var tasks []JSONTask
	err := UnmarshalJSONTasks(strings.NewReader(`[{"uuid":"u1","title":"task","status":"pending","entry":"1970-01-01T00:00:01Z","modified":"1970-01-01T00:00:02Z","legacy_field":{"x":1}}]`), &tasks)
	if err != nil {
		t.Fatalf("UnmarshalJSONTasks(orphan) error = %v", err)
	}
	got := FromJSON(tasks[0])
	legacy := got.UDAs["legacy_field"]
	if legacy.Raw != `{"x":1}` || !legacy.Orphan {
		t.Fatalf("legacy UDA = %#v", legacy)
	}
}

func TestJSONTaskRejectsProjectIDAsReservedField(t *testing.T) {
	var dto JSONTask
	err := json.Unmarshal([]byte(`{"uuid":"u1","title":"task","status":"pending","entry":"1970-01-01T00:00:01Z","modified":"1970-01-01T00:00:02Z","project_id":"p1"}`), &dto)
	if err == nil || !strings.Contains(err.Error(), "project_id") {
		t.Fatalf("Unmarshal(project_id) error = %v, want reserved project_id", err)
	}
}

func TestJSONTaskDoesNotExportProjectID(t *testing.T) {
	project := "api"
	projectID := "project-1"
	data, err := json.Marshal(ToJSON(Task{
		UUID:      "u1",
		Title:     "task",
		Status:    StatusPending,
		Entry:     1,
		Modified:  2,
		Project:   &project,
		ProjectID: &projectID,
	}))
	if err != nil {
		t.Fatalf("Marshal(task with ProjectID) error = %v", err)
	}
	if bytes.Contains(data, []byte("project_id")) {
		t.Fatalf("export leaked project_id: %s", data)
	}
}

func TestJSONTaskKeepsOrphanUDAButRejectsReservedProjectID(t *testing.T) {
	var dto JSONTask
	err := json.Unmarshal([]byte(`{"uuid":"u1","title":"task","status":"pending","entry":"1970-01-01T00:00:01Z","modified":"1970-01-01T00:00:02Z","legacy_field":"kept"}`), &dto)
	if err != nil {
		t.Fatalf("Unmarshal(orphan UDA) error = %v", err)
	}
	got := dto.UDAs["legacy_field"]
	if got.Raw != "kept" || !got.Orphan {
		t.Fatalf("legacy UDA = %#v", got)
	}
	if _, ok := dto.UDAs["project_id"]; ok {
		t.Fatalf("project_id entered orphan UDA: %#v", dto.UDAs)
	}

	err = json.Unmarshal([]byte(`{"uuid":"u1","title":"task","status":"pending","entry":"1970-01-01T00:00:01Z","modified":"1970-01-01T00:00:02Z","project_id":"p1"}`), &dto)
	if err == nil || !strings.Contains(err.Error(), "project_id") {
		t.Fatalf("Unmarshal(project_id) error = %v, want reserved project_id", err)
	}
}

func TestFromJSONStrictRejectsInvalidDate(t *testing.T) {
	_, err := FromJSONStrict(JSONTask{
		UUID:     "u1",
		Title:    "task",
		Status:   StatusPending,
		Entry:    "1970-01-01T00:00:01Z",
		Modified: "not-a-date",
	})
	if err == nil {
		t.Fatal("FromJSONStrict() error = nil, want invalid date error")
	}
}

func TestUnmarshalJSONTasksPreservesNilVsEmptySlices(t *testing.T) {
	var missing []JSONTask
	if err := UnmarshalJSONTasks(strings.NewReader(`[{"uuid":"u1","title":"task","status":"pending","entry":"1970-01-01T00:00:01Z","modified":"1970-01-01T00:00:02Z"}]`), &missing); err != nil {
		t.Fatalf("UnmarshalJSONTasks(missing) error = %v", err)
	}
	if missing[0].Tags != nil {
		t.Fatalf("Tags when field missing = %#v, want nil", missing[0].Tags)
	}

	var empty []JSONTask
	if err := UnmarshalJSONTasks(strings.NewReader(`[{"uuid":"u1","title":"task","status":"pending","entry":"1970-01-01T00:00:01Z","modified":"1970-01-01T00:00:02Z","tags":[],"annotations":[],"depends":[]}]`), &empty); err != nil {
		t.Fatalf("UnmarshalJSONTasks(empty) error = %v", err)
	}
	if empty[0].Tags == nil || empty[0].Annotations == nil || empty[0].Depends == nil {
		t.Fatalf("explicit empty slices should stay non-nil: %#v", empty[0])
	}
	if len(empty[0].Tags) != 0 || len(empty[0].Annotations) != 0 || len(empty[0].Depends) != 0 {
		t.Fatalf("explicit empty slices should be empty: %#v", empty[0])
	}
}

func TestJSONTaskExportsLinks(t *testing.T) {
	tsk := Task{
		UUID: "u1", Title: "task", Status: StatusPending, Entry: 1, Modified: 2,
		Links: []TaskLinkInfo{
			{ID: "link-1", Type: "document", URL: "https://example.com/doc", Title: "需求文档", CreatedAt: 1700000000, CreatedBy: ActorInfo{Type: "user", User: &UserInfo{ID: "user-1", Name: "user-1"}}},
			{ID: "link-2", Type: "pr", URL: "https://github.com/pull/1", CreatedAt: 1700000001, CreatedBy: ActorInfo{Type: "user", User: &UserInfo{ID: "user-2", Name: "user-2"}}},
		},
	}
	data, err := json.Marshal(ToJSON(tsk))
	if err != nil {
		t.Fatalf("Marshal links task error = %v", err)
	}
	if !strings.Contains(string(data), `"links"`) || !strings.Contains(string(data), `"需求文档"`) {
		t.Fatalf("links JSON missing: %s", data)
	}
	var dto JSONTask
	if err := json.Unmarshal(data, &dto); err != nil {
		t.Fatalf("Unmarshal links error = %v", err)
	}
	if len(dto.Links) != 2 {
		t.Fatalf("expected 2 links, got %d", len(dto.Links))
	}
	if dto.Links[0].Type != "document" || dto.Links[0].URL != "https://example.com/doc" {
		t.Fatalf("link[0] = %#v", dto.Links[0])
	}
	if dto.Links[1].Title != "" {
		t.Fatalf("link[1] title should be omitted, got %q", dto.Links[1].Title)
	}
}

func TestJSONTaskImportLinks(t *testing.T) {
	var dto JSONTask
	err := json.Unmarshal([]byte(`{
		"uuid":"u1",
		"title":"test",
		"status":"pending",
		"entry":"1970-01-01T00:00:01Z",
		"modified":"1970-01-01T00:00:02Z",
		"links":[
			{"id":"link-1","type":"document","url":"https://example.com/doc","title":"需求文档","created_at":"2023-11-14T22:13:20Z","created_by":{"type":"user","user":{"id":"user-1","name":"user-1"}}},
			{"id":"link-2","type":"pr","url":"https://github.com/pull/1","created_at":"2023-11-14T22:13:21Z","created_by":{"type":"user","user":{"id":"user-2","name":"user-2"}}}
		]
	}`), &dto)
	if err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	tsk := FromJSON(dto)
	if len(tsk.Links) != 2 {
		t.Fatalf("expected 2 links, got %d", len(tsk.Links))
	}
	if tsk.Links[0].Type != "document" || tsk.Links[0].URL != "https://example.com/doc" {
		t.Fatalf("link[0] = %#v", tsk.Links[0])
	}
	if tsk.Links[0].CreatedAt != 1700000000 {
		t.Fatalf("link[0].CreatedAt = %d, want 1700000000", tsk.Links[0].CreatedAt)
	}
}

func TestJSONTaskLinksRoundTrip(t *testing.T) {
	tsk := Task{
		UUID: "u1", Title: "task", Status: StatusPending, Entry: 1, Modified: 2,
		Links: []TaskLinkInfo{
			{ID: "link-1", Type: "document", URL: "https://example.com/doc", Title: "需求文档", CreatedAt: 1700000000, CreatedBy: ActorInfo{Type: "user", User: &UserInfo{ID: "user-1", Name: "user-1"}}},
		},
	}
	got := FromJSON(ToJSON(tsk))
	if len(got.Links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(got.Links))
	}
	if got.Links[0].ID != "link-1" || got.Links[0].Type != "document" || got.Links[0].Title != "需求文档" {
		t.Fatalf("link round-trip lost data: %#v", got.Links[0])
	}
}

func TestJSONTaskNilLinksOmitted(t *testing.T) {
	tsk := Task{UUID: "u1", Title: "task", Status: StatusPending, Entry: 1, Modified: 2}
	data, err := json.Marshal(ToJSON(tsk))
	if err != nil {
		t.Fatalf("Marshal error = %v", err)
	}
	if strings.Contains(string(data), `"links"`) {
		t.Fatalf("nil links should be omitted: %s", data)
	}
}

func TestJSONTaskEmptyLinksPreserved(t *testing.T) {
	data, err := json.Marshal(JSONTask{
		UUID: "u1", Title: "task", Status: StatusPending,
		Entry: "1970-01-01T00:00:01Z", Modified: "1970-01-01T00:00:02Z",
		Links: []JSONTaskLink{},
	})
	if err != nil {
		t.Fatalf("Marshal error = %v", err)
	}
	if !strings.Contains(string(data), `"links":[]`) {
		t.Fatalf("empty links should be preserved: %s", data)
	}
}

func TestMarshalJSONTaskOmitsNilSlicesAndKeepsEmptyTagsWhenRequested(t *testing.T) {
	nilJSON, err := json.Marshal(JSONTask{
		UUID:     "u1",
		Title:    "task",
		Status:   StatusPending,
		Entry:    "1970-01-01T00:00:01Z",
		Modified: "1970-01-01T00:00:02Z",
	})
	if err != nil {
		t.Fatalf("Marshal(nil JSONTask) error = %v", err)
	}
	if strings.Contains(string(nilJSON), `"tags"`) {
		t.Fatalf("nil tags should be omitted: %s", nilJSON)
	}

	emptyJSON, err := json.Marshal(JSONTask{
		UUID:     "u1",
		Title:    "task",
		Status:   StatusPending,
		Entry:    "1970-01-01T00:00:01Z",
		Modified: "1970-01-01T00:00:02Z",
		Tags:     []string{},
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
	err := UnmarshalJSONTasks(strings.NewReader(`[{"uuid":"u1","title":"task","status":"pending","entry":"1970-01-01T00:00:01Z","modified":"1970-01-01T00:00:02Z"}]`), &tasks)
	if err != nil {
		t.Fatalf("UnmarshalJSONTasks() error = %v", err)
	}
	if len(tasks) != 1 || tasks[0].UUID != "u1" {
		t.Fatalf("tasks = %#v", tasks)
	}
}

func TestMarshalJSONTasksWritesArray(t *testing.T) {
	data, err := MarshalJSONTasks([]JSONTask{{
		UUID:     "u1",
		Title:    "task",
		Status:   StatusPending,
		Entry:    "1970-01-01T00:00:01Z",
		Modified: "1970-01-01T00:00:02Z",
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
	err := UnmarshalJSONTasks(io.NopCloser(strings.NewReader(`[{"uuid":"u1","title":"task","status":"pending","entry":"1970-01-01T00:00:01Z","modified":"1970-01-01T00:00:02Z"}]`)), &tasks)
	if err != nil {
		t.Fatalf("UnmarshalJSONTasks(reader) error = %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("len(tasks) = %d, want 1", len(tasks))
	}
}
