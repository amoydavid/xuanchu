package projecttemplate

import (
	"strings"
	"testing"
)

func TestValidateSnapshotRejectsMissingCycleSecretAndAttachment(t *testing.T) {
	value := "must-not-be-present"
	cases := []struct {
		name string
		edit func(*SnapshotV1)
		code string
	}{
		{
			name: "missing dependency", code: "project_template_dependency_missing",
			edit: func(snapshot *SnapshotV1) { snapshot.Tasks[0].DependsRefs = []string{"task-2"} },
		},
		{
			name: "dependency cycle", code: "project_template_ref_cycle",
			edit: func(snapshot *SnapshotV1) {
				snapshot.Tasks[0].DependsRefs = []string{"task-2"}
				snapshot.Tasks = append(snapshot.Tasks, TaskBlueprintV1{Ref: "task-2", Title: "后续任务", Dates: TaskDatesV1{}, DependsRefs: []string{"task-1"}})
			},
		},
		{
			name: "secret has literal", code: "project_template_snapshot_invalid",
			edit: func(snapshot *SnapshotV1) {
				snapshot.Configs[0].Mode, snapshot.Configs[0].Value = "secret_input", &value
			},
		},
		{
			name: "attachment reference", code: "project_template_attachment_unsupported",
			edit: func(snapshot *SnapshotV1) {
				description := "![文档](ref://attachment/40af0185-316f-42bb-b52b-545d21f6f012)"
				snapshot.Tasks[0].Description = &description
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := fixtureSnapshotV1()
			tc.edit(&snapshot)
			if err := ValidateSnapshot(snapshot, DefaultLimits); ErrorCode(err) != tc.code {
				t.Fatalf("error = %v, code = %q, want %q", err, ErrorCode(err), tc.code)
			}
		})
	}
}

func TestValidateSnapshotRejectsComponentAndTextLimit(t *testing.T) {
	snapshot := fixtureSnapshotV1()
	limits := DefaultLimits
	limits.MaxTasks = 0
	if err := ValidateSnapshot(snapshot, limits); ErrorCode(err) != "project_template_snapshot_invalid" {
		t.Fatalf("count error = %v", err)
	}

	limits = DefaultLimits
	limits.MaxTextBytes = 4
	snapshot.Tasks[0].Title = strings.Repeat("任", 3)
	if err := ValidateSnapshot(snapshot, limits); ErrorCode(err) != "project_template_snapshot_invalid" {
		t.Fatalf("text error = %v", err)
	}
}
