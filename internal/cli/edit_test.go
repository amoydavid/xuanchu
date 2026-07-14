package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/edit"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

func TestEditableTaskFromProjectedViewUsesStableIDAndNullableMaterializedIdentity(t *testing.T) {
	due := int64(1893456000)
	view := app.TaskOccurrenceView{
		ID: "occ:series-1:1893456000", Title: "每日巡检",
		Status: task.StatusPending, Due: &due,
		RecurrenceInfo: &app.RecurrenceInfo{
			Role: "occurrence", SeriesID: "series-1", Rule: "daily",
			RecurrenceAt: due, Materialization: "projected",
		},
	}

	document := editableTaskFromView(view)
	if document.ID != view.ID || document.UUID != nil || document.Entry != nil {
		t.Fatalf("editable document = %#v", document)
	}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{`"id":"occ:series-1:1893456000"`, `"uuid":null`, `"entry":null`} {
		if !strings.Contains(text, want) {
			t.Fatalf("document JSON missing %s: %s", want, text)
		}
	}
	for _, forbidden := range []string{"series_id", "recurrence_at", "recurrence_rule_snapshot", "recurrence_overrides"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("document JSON contains %s: %s", forbidden, text)
		}
	}
}

func TestEditableFieldsEqualTreatsJSONReformatAsNoOp(t *testing.T) {
	original := edit.EditableTask{
		ID: "occ:series-1:100", Title: "巡检", Status: task.StatusPending,
		Tags: []string{"ops"},
	}
	edited, err := edit.Parse([]byte(`{
  "status": "pending",
  "title": "巡检",
  "tags": ["ops"],
  "entry": null,
  "uuid": null,
  "id": "occ:series-1:100"
}`), original)
	if err != nil {
		t.Fatal(err)
	}
	before, err := edit.Apply(original)
	if err != nil {
		t.Fatal(err)
	}
	after, err := edit.Apply(edited)
	if err != nil {
		t.Fatal(err)
	}
	if !editableFieldsEqual(before, after) {
		t.Fatalf("semantically equal edit treated as diff: before=%#v after=%#v", before, after)
	}
}

func TestTaskDescriptionWithTextAppendsAndPrependsWithoutUsingTitle(t *testing.T) {
	current := "已有说明"
	if got := taskDescriptionWithText(&current, "尾部", false); got != "已有说明 尾部" {
		t.Fatalf("append = %q", got)
	}
	if got := taskDescriptionWithText(&current, "前置", true); got != "前置 已有说明" {
		t.Fatalf("prepend = %q", got)
	}
	if got := taskDescriptionWithText(nil, "首次", false); got != "首次" {
		t.Fatalf("empty append = %q", got)
	}
}
