package task

import (
	"encoding/json"
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
