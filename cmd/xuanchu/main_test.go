package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestWriteErrorHonorsRcJSONAliases(t *testing.T) {
	for _, args := range [][]string{
		{"rc.json:on", "--workspace", "missing", "list"},
		{"rc.json=yes", "--workspace", "missing", "list"},
		{"rc.json=1", "--workspace", "missing", "list"},
	} {
		var out bytes.Buffer
		writeError(&out, args, "workspace_not_found", "missing")
		var payload map[string]string
		if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
			t.Fatalf("writeError(%v) output is not JSON: %v\n%s", args, err, out.String())
		}
		if payload["code"] != "workspace_not_found" || payload["message"] != "missing" {
			t.Fatalf("writeError(%v) payload = %#v", args, payload)
		}
	}
}

func TestWriteErrorDoesNotTreatFalseRcJSONAsJSON(t *testing.T) {
	var out bytes.Buffer
	writeError(&out, []string{"rc.json=false", "--workspace", "missing", "list"}, "workspace_not_found", "missing")
	if json.Valid(out.Bytes()) {
		t.Fatalf("writeError(rc.json=false) output = %q, want text", out.String())
	}
}
