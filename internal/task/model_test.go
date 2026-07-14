package task

import "testing"

func TestValidateRequiresTitle(t *testing.T) {
	tsk := Task{Title: "   ", Status: StatusPending}
	if err := tsk.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
}

func TestValidateAllowsEmptyDescription(t *testing.T) {
	tsk := Task{Title: "hello", Status: StatusPending}
	if err := tsk.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	empty := ""
	tsk.Description = &empty
	if err := tsk.Validate(); err != nil {
		t.Fatalf("Validate(empty detailed description) error = %v, want nil", err)
	}
}

func TestValidateRejectsInvalidPriority(t *testing.T) {
	priority := "X"
	tsk := Task{Title: "hello", Status: StatusPending, Priority: &priority}
	if err := tsk.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
}

func TestValidateAllowsM2StatusesAndFields(t *testing.T) {
	tsk := Task{UUID: "u1", WorkspaceID: "w1", Title: "task", Status: StatusWaiting, Entry: 1, Modified: 1}
	if err := tsk.Validate(); err != nil {
		t.Fatalf("Validate(waiting) error = %v", err)
	}
}

func TestValidateRejectsLegacyRecurringStatus(t *testing.T) {
	// spec 2026-07-11：Task.status 不再含 recurring。
	tsk := Task{UUID: "u1", WorkspaceID: "w1", Title: "parent", Status: "recurring", Entry: 1, Modified: 1}
	if err := tsk.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want invalid status error")
	}
}

func TestAnnotationValidateRejectsEmptyDescription(t *testing.T) {
	tsk := Task{
		UUID: "u1", WorkspaceID: "w1", Title: "task", Status: StatusPending, Entry: 1, Modified: 1,
		Annotations: []Annotation{{Entry: 10, Description: "  "}},
	}
	if err := tsk.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want annotation error")
	}
}

func TestStartStopHelpers(t *testing.T) {
	tsk := Task{UUID: "u1", WorkspaceID: "w1", Title: "task", Status: StatusPending, Entry: 1, Modified: 1}
	tsk.StartTask(100)
	if tsk.Start == nil || *tsk.Start != 100 || tsk.Modified != 100 {
		t.Fatalf("StartTask did not set start/modified: %#v", tsk)
	}
	tsk.StopTask(200)
	if tsk.Start != nil || tsk.Modified != 200 {
		t.Fatalf("StopTask did not clear start/update modified: %#v", tsk)
	}
}

func TestCompleteSetsStatusAndEnd(t *testing.T) {
	now := int64(100)
	tsk := Task{Title: "hello", Status: StatusPending}
	tsk.Complete(now)
	if tsk.Status != StatusCompleted {
		t.Fatalf("Status = %q", tsk.Status)
	}
	if tsk.End == nil || *tsk.End != now {
		t.Fatalf("End = %#v, want %d", tsk.End, now)
	}
	if tsk.Modified != now {
		t.Fatalf("Modified = %d, want %d", tsk.Modified, now)
	}
}

// TestTaskValidateOccurrenceInvariant 锁定 occurrence 持久字段的 invariant（spec §7.2）：
// series_id / recurrence_at / recurrence_rule_snapshot 三者要么全 nil（普通任务），
// 要么全非 nil（已物化 occurrence）。recurrence_overrides 必须去重排序且字段名受限。
func TestTaskValidateOccurrenceInvariant(t *testing.T) {
	seriesID := "series-1"
	slot := int64(100)
	rule := "daily"
	base := Task{UUID: "task-1", WorkspaceID: "ws-1", Title: "巡检", Status: StatusPending}
	for name, mutate := range map[string]func(*Task){
		"series without slot":         func(v *Task) { v.SeriesID = &seriesID },
		"slot without series":         func(v *Task) { v.RecurrenceAt = &slot },
		"series without snapshot":     func(v *Task) { v.SeriesID = &seriesID; v.RecurrenceAt = &slot },
		"snapshot without series":     func(v *Task) { v.RecurrenceRuleSnapshot = &rule },
	} {
		t.Run(name, func(t *testing.T) {
			value := base
			mutate(&value)
			if err := value.Validate(); err == nil {
				t.Fatal("Validate 成功，但应因 occurrence invariant 失败")
			}
		})
	}
	// 全 nil：合法普通任务。
	if err := base.Validate(); err != nil {
		t.Fatalf("普通任务 Validate 失败: %v", err)
	}
	// 全非 nil：合法 occurrence。
	occ := base
	occ.SeriesID = &seriesID
	occ.RecurrenceAt = &slot
	occ.RecurrenceRuleSnapshot = &rule
	if err := occ.Validate(); err != nil {
		t.Fatalf("occurrence Validate 失败: %v", err)
	}
}

func TestNormalizeRecurrenceOverrides(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"nil", nil, []string{}},
		{"dedupe and sort", []string{"due", "title", "due", "assignees"}, []string{"assignees", "due", "title"}},
		{"empty", []string{}, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeRecurrenceOverrides(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("len=%d want=%d: %#v", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("[%d]=%q want=%q: %#v", i, got[i], tc.want[i], got)
				}
			}
		})
	}
}

func TestValidateRecurrenceOverridesRejectsUnknownField(t *testing.T) {
	if err := ValidateRecurrenceOverrides([]string{"title", "status"}); err == nil {
		t.Fatal("status 不在允许字段内，应被拒绝")
	}
	if err := ValidateRecurrenceOverrides([]string{"title", "due"}); err != nil {
		t.Fatalf("合法字段被拒绝: %v", err)
	}
}
