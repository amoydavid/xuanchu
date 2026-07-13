package mcpserver

import (
	"testing"
)

func TestMCPTaskSeriesAddAndGet(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)

	// 先建项目。
	callTool(t, session, "project_add", map[string]any{"slug": "ops", "name": "Ops"})

	// series_add。
	firstDue := int64(1893456000)
	result := callTool(t, session, "task_series_add", TaskSeriesAddInput{
		Project: "ops", Title: "每日巡检", RecurrenceRule: "daily", FirstDue: &firstDue,
		Assignees: []string{"local"},
	})
	if result.IsError {
		t.Fatalf("series_add error: %v", parseError(t, result))
	}
	env := parseEnvelope(t, result)
	data, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("data type = %T", env.Data)
	}
	series, ok := data["series"].(map[string]any)
	if !ok {
		t.Fatalf("series type = %T", data)
	}
	if series["title"] != "每日巡检" || series["status"] != "active" {
		t.Fatalf("series = %#v", series)
	}
	assignees, ok := series["assignees"].([]any)
	if !ok || len(assignees) != 1 || assignees[0].(map[string]any)["id"] == nil {
		t.Fatalf("assignees 未使用统一 UserInfo JSON: %#v", series["assignees"])
	}
	createdBy, ok := series["created_by"].(map[string]any)
	if !ok || createdBy["id"] == nil {
		t.Fatalf("created_by 未使用统一 UserInfo JSON: %#v", series["created_by"])
	}
	seriesID, _ := series["id"].(string)
	if seriesID == "" {
		t.Fatal("series id 为空")
	}
	if firstOcc, ok := data["first_occurrence"].(map[string]any); ok {
		if firstOcc["id"] == nil {
			t.Fatal("first_occurrence id 为空")
		}
	}

	// series_get。
	result2 := callTool(t, session, "task_series_get", TaskSeriesRefInput{ID: seriesID})
	if result2.IsError {
		t.Fatalf("series_get error: %v", parseError(t, result2))
	}
	env2 := parseEnvelope(t, result2)
	data2, _ := env2.Data.(map[string]any)
	if data2["title"] != "每日巡检" {
		t.Fatalf("get title = %v", data2["title"])
	}
	if data2["suggested_rule_effective_from"] == nil {
		t.Fatalf("series_get 缺少 suggested_rule_effective_from: %#v", data2)
	}
	for _, field := range []string{"open_occurrences", "recent_completed", "recent_skipped"} {
		if _, ok := data2[field]; !ok {
			t.Fatalf("series_get 缺少实例分组 %s: %#v", field, data2)
		}
	}
}

func TestMCPTaskSeriesList(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)
	callTool(t, session, "project_add", map[string]any{"slug": "ops", "name": "Ops"})
	firstDue := int64(1893456000)
	callTool(t, session, "task_series_add", TaskSeriesAddInput{
		Project: "ops", Title: "每日巡检", RecurrenceRule: "daily", FirstDue: &firstDue,
	})

	result := callTool(t, session, "task_series_list", TaskSeriesListInput{Status: "active"})
	if result.IsError {
		t.Fatalf("series_list error: %v", parseError(t, result))
	}
	env := parseEnvelope(t, result)
	data, _ := env.Data.(map[string]any)
	total, _ := data["total"].(float64)
	if total != 1 {
		t.Fatalf("total = %v want 1", total)
	}
}

func TestMCPTaskSeriesModifyAppliesClearFields(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)
	callTool(t, session, "project_add", map[string]any{"slug": "ops", "name": "Ops"})
	firstDue := int64(1893456000)
	until := int64(1896134400)
	description, priority := "旧说明", "H"
	addResult := callTool(t, session, "task_series_add", TaskSeriesAddInput{
		Project: "ops", Title: "每日巡检", Description: &description,
		RecurrenceRule: "daily", FirstDue: &firstDue, Until: &until,
		Priority: &priority, Tags: []string{"ops"},
	})
	seriesID := parseEnvelope(t, addResult).Data.(map[string]any)["series"].(map[string]any)["id"].(string)

	result := callTool(t, session, "task_series_modify", map[string]any{
		"id": seriesID, "clear": []string{"description", "priority", "tags", "until"},
	})
	if result.IsError {
		t.Fatalf("series_modify error: %v", parseError(t, result))
	}
	data := parseEnvelope(t, result).Data.(map[string]any)
	for _, field := range []string{"description", "priority", "tags", "until"} {
		if _, ok := data[field]; ok {
			t.Fatalf("clear 后仍返回 %s: %#v", field, data)
		}
	}
}

func TestMCPTaskSeriesStop(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)
	callTool(t, session, "project_add", map[string]any{"slug": "ops", "name": "Ops"})
	firstDue := int64(1893456000)
	addResult := callTool(t, session, "task_series_add", TaskSeriesAddInput{
		Project: "ops", Title: "每日巡检", RecurrenceRule: "daily", FirstDue: &firstDue,
	})
	addEnv := parseEnvelope(t, addResult)
	seriesID := addEnv.Data.(map[string]any)["series"].(map[string]any)["id"].(string)

	result := callTool(t, session, "task_series_stop", TaskSeriesStopInput{ID: seriesID})
	if result.IsError {
		t.Fatalf("series_stop error: %v", parseError(t, result))
	}
	env := parseEnvelope(t, result)
	data, _ := env.Data.(map[string]any)
	if data["status"] != "stopped" {
		t.Fatalf("status = %v want stopped", data["status"])
	}
}

func TestMCPTaskSeriesOccurrenceSkip(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)
	callTool(t, session, "project_add", map[string]any{"slug": "ops", "name": "Ops"})
	firstDue := int64(1893456000)
	addResult := callTool(t, session, "task_series_add", TaskSeriesAddInput{
		Project: "ops", Title: "每日巡检", RecurrenceRule: "daily", FirstDue: &firstDue,
	})
	addEnv := parseEnvelope(t, addResult)
	seriesID := addEnv.Data.(map[string]any)["series"].(map[string]any)["id"].(string)
	occRef := addEnv.Data.(map[string]any)["first_occurrence"].(map[string]any)["id"].(string)

	result := callTool(t, session, "task_series_occurrence_skip", TaskSeriesOccurrenceSkipInput{
		SeriesID: seriesID, OccurrenceID: occRef,
	})
	if result.IsError {
		t.Fatalf("occurrence_skip error: %v", parseError(t, result))
	}
	env := parseEnvelope(t, result)
	data, _ := env.Data.(map[string]any)
	if data["status"] != "deleted" {
		t.Fatalf("status = %v want deleted", data["status"])
	}
}

func TestMCPTaskOccurrenceAliasesReturnOneResourceShape(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)
	callTool(t, session, "project_add", map[string]any{"slug": "ops", "name": "Ops"})
	firstDue := int64(1893456000)
	addResult := callTool(t, session, "task_series_add", TaskSeriesAddInput{
		Project: "ops", Title: "每日巡检", RecurrenceRule: "daily", FirstDue: &firstDue,
	})
	occurrenceRef := parseEnvelope(t, addResult).Data.(map[string]any)["first_occurrence"].(map[string]any)["id"].(string)

	done := callTool(t, session, "task_done", TaskIDInput{ID: occurrenceRef})
	if done.IsError {
		t.Fatalf("task_done occurrence error: %v", parseError(t, done))
	}
	doneData := parseEnvelope(t, done).Data.(map[string]any)
	slug, _ := doneData["task_slug"].(string)
	uuid, _ := doneData["uuid"].(string)
	if doneData["id"] != occurrenceRef || slug == "" || uuid == "" || doneData["recurrence_info"] == nil {
		t.Fatalf("done data = %#v", doneData)
	}

	for _, ref := range []string{occurrenceRef, uuid, slug} {
		got := callTool(t, session, "task_get", TaskGetInput{ID: ref})
		if got.IsError {
			t.Fatalf("task_get(%q): %v", ref, parseError(t, got))
		}
		data := parseEnvelope(t, got).Data.(map[string]any)
		if data["id"] != occurrenceRef || data["task_slug"] != slug || data["recurrence_info"] == nil {
			t.Fatalf("task_get(%q) = %#v", ref, data)
		}
	}

	reopened := callTool(t, session, "task_reopen", TaskIDInput{ID: slug})
	if reopened.IsError {
		t.Fatalf("task_reopen slug error: %v", parseError(t, reopened))
	}
	reopenedData := parseEnvelope(t, reopened).Data.(map[string]any)
	if reopenedData["id"] != occurrenceRef || reopenedData["recurrence_info"] == nil {
		t.Fatalf("reopen data = %#v", reopenedData)
	}
}

func TestMCPTaskSeriesAddRejectsInvalidRule(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)
	callTool(t, session, "project_add", map[string]any{"slug": "ops", "name": "Ops"})
	firstDue := int64(1893456000)
	result := callTool(t, session, "task_series_add", TaskSeriesAddInput{
		Project: "ops", Title: "x", RecurrenceRule: "biweekly", FirstDue: &firstDue,
	})
	if !result.IsError {
		t.Fatal("biweekly 应被拒绝")
	}
}
