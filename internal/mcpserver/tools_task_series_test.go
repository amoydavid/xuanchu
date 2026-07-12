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
