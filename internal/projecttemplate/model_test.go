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

func TestValidateSnapshotRejectsDuplicateLocalRefsAndMissingBodyRef(t *testing.T) {
	cases := []struct {
		name string
		edit func(*SnapshotV1)
		code string
	}{
		{
			name: "duplicate task ref", code: "project_template_snapshot_invalid",
			edit: func(snapshot *SnapshotV1) {
				snapshot.Tasks = append(snapshot.Tasks, TaskBlueprintV1{Ref: "task-1", Title: "重复", Dates: TaskDatesV1{}})
			},
		},
		{
			name: "duplicate series ref", code: "project_template_snapshot_invalid",
			edit: func(snapshot *SnapshotV1) {
				series := validSeriesBlueprint()
				snapshot.Series = []SeriesBlueprintV1{series, series}
			},
		},
		{
			name: "duplicate automation ref", code: "project_template_snapshot_invalid",
			edit: func(snapshot *SnapshotV1) {
				snapshot.Automations = append(snapshot.Automations, snapshot.Automations[0])
			},
		},
		{
			name: "missing body task ref", code: "project_template_dependency_missing",
			edit: func(snapshot *SnapshotV1) {
				description := "[缺失任务](ref://task/task-2)"
				snapshot.Tasks[0].Description = &description
			},
		},
		{
			name: "missing series body task ref", code: "project_template_dependency_missing",
			edit: func(snapshot *SnapshotV1) {
				description := "[缺失任务](ref://task/task-2)"
				series := validSeriesBlueprint()
				series.Description = &description
				snapshot.Series = []SeriesBlueprintV1{series}
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

func TestValidateSnapshotChecksEveryRelativeLocalTime(t *testing.T) {
	cases := []struct {
		name string
		edit func(*SnapshotV1)
	}{
		{
			name: "task due",
			edit: func(snapshot *SnapshotV1) {
				snapshot.Tasks[0].Dates.Due = &RelativeLocalTimeV1{LocalTime: "9:00:00"}
			},
		},
		{
			name: "task wait",
			edit: func(snapshot *SnapshotV1) {
				snapshot.Tasks[0].Dates.Wait = &RelativeLocalTimeV1{LocalTime: "09:00"}
			},
		},
		{
			name: "task scheduled",
			edit: func(snapshot *SnapshotV1) {
				snapshot.Tasks[0].Dates.Scheduled = &RelativeLocalTimeV1{LocalTime: "24:00:00"}
			},
		},
		{
			name: "task until",
			edit: func(snapshot *SnapshotV1) {
				snapshot.Tasks[0].Dates.Until = &RelativeLocalTimeV1{LocalTime: "09:00:00Z"}
			},
		},
		{
			name: "series first due required",
			edit: func(snapshot *SnapshotV1) {
				snapshot.Series = []SeriesBlueprintV1{validSeriesBlueprint()}
				snapshot.Series[0].FirstDue.LocalTime = ""
			},
		},
		{
			name: "series until",
			edit: func(snapshot *SnapshotV1) {
				snapshot.Series = []SeriesBlueprintV1{validSeriesBlueprint()}
				snapshot.Series[0].Until = &RelativeLocalTimeV1{LocalTime: "09:00"}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := fixtureSnapshotV1()
			tc.edit(&snapshot)
			if err := ValidateSnapshot(snapshot, DefaultLimits); ErrorCode(err) != "project_template_snapshot_invalid" {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestValidateSnapshotAppliesTextLimitToPersistedStrings(t *testing.T) {
	tooLong := strings.Repeat("x", 65)
	cases := []struct {
		name string
		edit func(*SnapshotV1)
	}{
		{"schema", func(s *SnapshotV1) { s.Schema = tooLong }},
		{"anchor date", func(s *SnapshotV1) { s.AnchorDate = tooLong }},
		{"project description", func(s *SnapshotV1) { s.Project.Description = tooLong }},
		{"config key", func(s *SnapshotV1) { s.Configs[0].Key = tooLong }},
		{"config mode", func(s *SnapshotV1) { s.Configs[0].Mode = tooLong }},
		{"config value", func(s *SnapshotV1) { s.Configs[0].Value = &tooLong }},
		{"task ref", func(s *SnapshotV1) { s.Tasks[0].Ref = tooLong }},
		{"task title", func(s *SnapshotV1) { s.Tasks[0].Title = tooLong }},
		{"task description", func(s *SnapshotV1) { s.Tasks[0].Description = &tooLong }},
		{"task priority", func(s *SnapshotV1) { s.Tasks[0].Priority = &tooLong }},
		{"task tag", func(s *SnapshotV1) { s.Tasks[0].Tags = []string{tooLong} }},
		{"task assignee", func(s *SnapshotV1) { s.Tasks[0].AssigneeIDs = []string{tooLong} }},
		{"task parent ref", func(s *SnapshotV1) { s.Tasks[0].ParentRef = &tooLong }},
		{"task dependency ref", func(s *SnapshotV1) { s.Tasks[0].DependsRefs = []string{tooLong} }},
		{"task due local time", func(s *SnapshotV1) { s.Tasks[0].Dates.Due = &RelativeLocalTimeV1{LocalTime: tooLong} }},
		{"task wait local time", func(s *SnapshotV1) { s.Tasks[0].Dates.Wait = &RelativeLocalTimeV1{LocalTime: tooLong} }},
		{"task scheduled local time", func(s *SnapshotV1) { s.Tasks[0].Dates.Scheduled = &RelativeLocalTimeV1{LocalTime: tooLong} }},
		{"task until local time", func(s *SnapshotV1) { s.Tasks[0].Dates.Until = &RelativeLocalTimeV1{LocalTime: tooLong} }},
		{"task UDA key", func(s *SnapshotV1) { s.Tasks[0].UDAs = map[string]UDABlueprintV1{tooLong: {Raw: "1"}} }},
		{"task UDA raw", func(s *SnapshotV1) { s.Tasks[0].UDAs = map[string]UDABlueprintV1{"x": {Raw: tooLong}} }},
		{"task UDA type", func(s *SnapshotV1) { s.Tasks[0].UDAs = map[string]UDABlueprintV1{"x": {Raw: "1", Type: tooLong}} }},
		{"task link type", func(s *SnapshotV1) { s.Tasks[0].Links = []TaskLinkBlueprintV1{{Type: tooLong, URL: "x"}} }},
		{"task link URL", func(s *SnapshotV1) { s.Tasks[0].Links = []TaskLinkBlueprintV1{{Type: "x", URL: tooLong}} }},
		{"task link title", func(s *SnapshotV1) { s.Tasks[0].Links = []TaskLinkBlueprintV1{{Type: "x", URL: "x", Title: tooLong}} }},
		{"series description", func(s *SnapshotV1) {
			x := validSeriesBlueprint()
			x.Description = &tooLong
			s.Series = []SeriesBlueprintV1{x}
		}},
		{"series ref", func(s *SnapshotV1) {
			x := validSeriesBlueprint()
			x.Ref = tooLong
			s.Series = []SeriesBlueprintV1{x}
		}},
		{"series title", func(s *SnapshotV1) {
			x := validSeriesBlueprint()
			x.Title = tooLong
			s.Series = []SeriesBlueprintV1{x}
		}},
		{"series priority", func(s *SnapshotV1) {
			x := validSeriesBlueprint()
			x.Priority = &tooLong
			s.Series = []SeriesBlueprintV1{x}
		}},
		{"series recurrence rule", func(s *SnapshotV1) {
			x := validSeriesBlueprint()
			x.RecurrenceRule = tooLong
			s.Series = []SeriesBlueprintV1{x}
		}},
		{"series tag", func(s *SnapshotV1) {
			x := validSeriesBlueprint()
			x.Tags = []string{tooLong}
			s.Series = []SeriesBlueprintV1{x}
		}},
		{"series assignee", func(s *SnapshotV1) {
			x := validSeriesBlueprint()
			x.AssigneeIDs = []string{tooLong}
			s.Series = []SeriesBlueprintV1{x}
		}},
		{"series UDA key", func(s *SnapshotV1) {
			x := validSeriesBlueprint()
			x.UDAs = map[string]UDABlueprintV1{tooLong: {Raw: "1"}}
			s.Series = []SeriesBlueprintV1{x}
		}},
		{"series UDA raw", func(s *SnapshotV1) {
			x := validSeriesBlueprint()
			x.UDAs = map[string]UDABlueprintV1{"x": {Raw: tooLong}}
			s.Series = []SeriesBlueprintV1{x}
		}},
		{"series UDA type", func(s *SnapshotV1) {
			x := validSeriesBlueprint()
			x.UDAs = map[string]UDABlueprintV1{"x": {Raw: "1", Type: tooLong}}
			s.Series = []SeriesBlueprintV1{x}
		}},
		{"series first due local time", func(s *SnapshotV1) {
			x := validSeriesBlueprint()
			x.FirstDue.LocalTime = tooLong
			s.Series = []SeriesBlueprintV1{x}
		}},
		{"series until local time", func(s *SnapshotV1) {
			x := validSeriesBlueprint()
			x.Until = &RelativeLocalTimeV1{LocalTime: tooLong}
			s.Series = []SeriesBlueprintV1{x}
		}},
		{"automation ref", func(s *SnapshotV1) { s.Automations[0].Ref = tooLong }},
		{"automation name", func(s *SnapshotV1) { s.Automations[0].Name = tooLong }},
		{"automation description", func(s *SnapshotV1) { s.Automations[0].Description = tooLong }},
		{"automation trigger type", func(s *SnapshotV1) { s.Automations[0].TriggerType = tooLong }},
		{"automation trigger schedule type", func(s *SnapshotV1) { s.Automations[0].TriggerConfig.ScheduleType = tooLong }},
		{"automation trigger schedule value", func(s *SnapshotV1) { s.Automations[0].TriggerConfig.ScheduleValue = tooLong }},
		{"automation trigger timezone", func(s *SnapshotV1) { s.Automations[0].TriggerConfig.Timezone = tooLong }},
		{"automation trigger event type", func(s *SnapshotV1) { s.Automations[0].TriggerConfig.EventType = tooLong }},
		{"automation condition task filter", func(s *SnapshotV1) { s.Automations[0].Condition.TaskFilter = tooLong }},
		{"automation action protocol", func(s *SnapshotV1) { s.Automations[0].Action.Protocol = tooLong }},
		{"automation action base URL key", func(s *SnapshotV1) { s.Automations[0].Action.BaseURLConfigKey = tooLong }},
		{"automation action API key", func(s *SnapshotV1) { s.Automations[0].Action.APIKeyConfigKey = tooLong }},
		{"automation action model key", func(s *SnapshotV1) { s.Automations[0].Action.ModelConfigKey = tooLong }},
		{"automation action allowed hosts key", func(s *SnapshotV1) { s.Automations[0].Action.AllowedHostsConfigKey = tooLong }},
		{"automation action model override", func(s *SnapshotV1) { s.Automations[0].Action.ModelOverride = tooLong }},
		{"automation context include", func(s *SnapshotV1) { s.Automations[0].Context.Include = []string{tooLong} }},
		{"automation instruction", func(s *SnapshotV1) { s.Automations[0].InstructionTemplate = tooLong }},
		{"automation system prompt", func(s *SnapshotV1) { s.Automations[0].SystemPrompt = tooLong }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := fixtureSnapshotV1()
			tc.edit(&snapshot)
			limits := DefaultLimits
			limits.MaxTextBytes = 64
			err := ValidateSnapshot(snapshot, limits)
			if ErrorCode(err) != "project_template_snapshot_invalid" || !strings.Contains(err.Error(), "maximum text size") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestValidateSnapshotRejectsSeriesUDATrimCollision(t *testing.T) {
	snapshot := fixtureSnapshotV1()
	series := validSeriesBlueprint()
	series.UDAs = map[string]UDABlueprintV1{
		" estimate": {Raw: "1", Type: "number"},
		"estimate ": {Raw: "2", Type: "number"},
	}
	snapshot.Series = []SeriesBlueprintV1{series}

	if err := ValidateSnapshot(snapshot, DefaultLimits); ErrorCode(err) != "project_template_snapshot_invalid" {
		t.Fatalf("error = %v", err)
	}
}

func validSeriesBlueprint() SeriesBlueprintV1 {
	return SeriesBlueprintV1{
		Ref: "series-1", Title: "每日巡检", RecurrenceRule: "daily",
		FirstDue: RelativeLocalTimeV1{LocalTime: "09:00:00"},
	}
}
