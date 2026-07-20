package projecttemplate

import (
	"bytes"
	"testing"
)

func fixtureSnapshotV1() SnapshotV1 {
	description := "  初始化说明  "
	priority := " H "
	configValue := " https://api.example.test/v1 "
	return SnapshotV1{
		Schema:     SnapshotSchemaV1,
		AnchorDate: "2026-07-20",
		Project:    ProjectBlueprintV1{Description: "  默认项目说明  "},
		Configs: []ConfigBlueprintV1{{
			Key: " agent.base_url ", Mode: "literal", Value: &configValue,
		}},
		Tasks: []TaskBlueprintV1{{
			Ref:         "task-1",
			Title:       "  准备发布  ",
			Description: &description,
			Priority:    &priority,
			Tags:        []string{"ops", "api"},
			AssigneeIDs: []string{"user-b", "user-a"},
			UDAs: map[string]UDABlueprintV1{
				"estimate": {Raw: " 3 ", Type: "number"},
			},
			Dates:       TaskDatesV1{},
			DependsRefs: []string{},
			Links:       []TaskLinkBlueprintV1{},
		}},
		Series: []SeriesBlueprintV1{},
		Automations: []AutomationBlueprintV1{{
			Ref:         "automation-1",
			Name:        "发布巡检",
			TriggerType: "schedule",
			TriggerConfig: AutomationTriggerV1{
				ScheduleType: "daily_at", ScheduleValue: "09:30", Timezone: "Asia/Shanghai",
			},
			Condition: AutomationConditionV1{TaskFilter: "status:pending", MaxTasks: 50},
			Action: AutomationActionV1{
				Protocol: "chat_completions", BaseURLConfigKey: "agent.base_url", APIKeyConfigKey: "agent.api_key",
				ModelConfigKey: "agent.model", Temperature: 0.2,
			},
			Context:             AutomationContextV1{Include: []string{"project", "tasks"}},
			InstructionTemplate: "生成巡检报告",
		}},
	}
}

func TestCodecV1StrictAndStable(t *testing.T) {
	a := fixtureSnapshotV1()
	b := fixtureSnapshotV1()
	b.Tasks[0].Tags = []string{"ops", "api", "ops"}
	a.Tasks[0].Tags = []string{"api", "ops"}

	ja, ha, err := EncodeV1(a, DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	jb, hb, err := EncodeV1(b, DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	if string(ja) != string(jb) || ha != hb {
		t.Fatalf("canonical mismatch\na=%s\nb=%s", ja, jb)
	}
	got, err := Decode(ja, DefaultLimits)
	if err != nil || got.Schema != SnapshotSchemaV1 {
		t.Fatalf("got=%#v err=%v", got, err)
	}
}

func TestDecodeRejectsUnknownSchemaFieldAndTrailingJSON(t *testing.T) {
	cases := []string{
		`{"schema":"xuanchu.project-template-snapshot/v2"}`,
		`{"schema":"xuanchu.project-template-snapshot/v1","anchor_date":"2026-07-20","project":{"description":""},"configs":[],"tasks":[],"series":[],"automations":[],"extra":1}`,
		`{"schema":"xuanchu.project-template-snapshot/v1"}{}`,
	}
	for _, raw := range cases {
		if _, err := Decode([]byte(raw), DefaultLimits); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestDecodeClassifiesInvalidAndUnsupportedSchema(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		code string
	}{
		{name: "missing", raw: `{}`, code: "project_template_snapshot_invalid"},
		{name: "null", raw: `{"schema":null}`, code: "project_template_snapshot_invalid"},
		{name: "number", raw: `{"schema":1}`, code: "project_template_snapshot_invalid"},
		{name: "unknown", raw: `{"schema":"xuanchu.project-template-snapshot/v2"}`, code: "project_template_snapshot_schema_unsupported"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Decode([]byte(tc.raw), DefaultLimits)
			if ErrorCode(err) != tc.code {
				t.Fatalf("error = %v, code = %q, want %q", err, ErrorCode(err), tc.code)
			}
		})
	}
}

func TestEncodeV1RejectsUDATrimCollision(t *testing.T) {
	snapshot := fixtureSnapshotV1()
	snapshot.Tasks[0].UDAs = map[string]UDABlueprintV1{
		" estimate": {Raw: "1", Type: "number"},
		"estimate ": {Raw: "2", Type: "number"},
	}
	if _, _, err := EncodeV1(snapshot, DefaultLimits); ErrorCode(err) != "project_template_snapshot_invalid" {
		t.Fatalf("error = %v", err)
	}
}

func TestEncodeV1KeepsRequiredEmptySlicesAsArrays(t *testing.T) {
	snapshot := fixtureSnapshotV1()
	snapshot.Configs = nil
	snapshot.Tasks = nil
	snapshot.Series = nil
	snapshot.Automations[0].Context.Include = nil
	raw, _, err := EncodeV1(snapshot, DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range [][]byte{
		[]byte(`"configs":[]`), []byte(`"tasks":[]`), []byte(`"series":[]`), []byte(`"include":[]`),
	} {
		if !bytes.Contains(raw, want) {
			t.Fatalf("canonical JSON missing %s: %s", want, raw)
		}
	}
	snapshot = fixtureSnapshotV1()
	snapshot.Automations = nil
	raw, _, err = EncodeV1(snapshot, DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`"automations":[]`)) {
		t.Fatalf("canonical JSON missing empty automations array: %s", raw)
	}
}

func TestDecodeRejectsNullRequiredAutomationContextInclude(t *testing.T) {
	snapshot := fixtureSnapshotV1()
	snapshot.Automations[0].Context.Include = []string{}
	raw, _, err := EncodeV1(snapshot, DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	raw = bytes.Replace(raw, []byte(`"include":[]`), []byte(`"include":null`), 1)
	if _, err := Decode(raw, DefaultLimits); ErrorCode(err) != "project_template_snapshot_invalid" {
		t.Fatalf("error = %v", err)
	}
}

func TestEncodeV1CanonicalizesDuplicateConfigKeys(t *testing.T) {
	snapshot := fixtureSnapshotV1()
	snapshot.Configs = append(snapshot.Configs, snapshot.Configs[0])
	raw, _, err := EncodeV1(snapshot, DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(raw, DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Configs) != 1 || decoded.Configs[0].Key != "agent.base_url" {
		t.Fatalf("configs = %#v", decoded.Configs)
	}
}

func TestEncodeV1DoesNotMutateCallerSnapshot(t *testing.T) {
	snapshot := fixtureSnapshotV1()
	snapshot.Tasks[0].Links = []TaskLinkBlueprintV1{{Type: " docs ", URL: " https://example.test ", Title: " 文档 "}}
	if _, _, err := EncodeV1(snapshot, DefaultLimits); err != nil {
		t.Fatal(err)
	}
	if got := snapshot.Tasks[0].Links[0]; got.Type != " docs " || got.URL != " https://example.test " || got.Title != " 文档 " {
		t.Fatalf("input link mutated: %#v", got)
	}
}
