package mcpserver

import (
	"encoding/json"
	"testing"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/taskseries"
	"github.com/google/jsonschema-go/jsonschema"
)

func TestMCPResourceMappersKeepURL(t *testing.T) {
	projectURL := "https://xuanchu.example.com/workspaces/local/projects/ops"
	project := projectViewFromApp(app.ProjectView{URL: projectURL})
	if project.URL != projectURL {
		t.Fatalf("project URL = %q", project.URL)
	}

	seriesURL := "https://xuanchu.example.com/workspaces/local/projects/ops/series/ops-s-1"
	series := seriesViewToMCPJSON(app.TaskSeriesView{
		Series: taskseries.Series{ID: "series-1"},
		URL:    seriesURL,
	})
	if series["url"] != seriesURL {
		t.Fatalf("series URL = %#v", series["url"])
	}

	taskURL := "https://xuanchu.example.com/workspaces/local/projects/ops/tasks/ops-1"
	task := occurrenceViewToMCPJSON(app.TaskOccurrenceView{ID: "task-1", URL: taskURL})
	if task["url"] != taskURL {
		t.Fatalf("task URL = %#v", task["url"])
	}
}

func TestMCPResourceURLIsEmptyWithoutPublicBaseURL(t *testing.T) {
	srv, _ := newTestServerWithOptions(t, Options{})
	session := connectClient(t, srv)
	created := callTool(t, session, "project_add", map[string]any{"slug": "emptyurl", "name": "Empty URL"})
	if created.IsError {
		t.Fatalf("project_add error: %v", parseError(t, created))
	}
	data := parseEnvelope(t, created).Data.(map[string]any)
	project := data["project"].(map[string]any)
	if project["url"] != "" {
		t.Fatalf("url = %#v, want empty", project["url"])
	}
}

func TestMCPTaskSeriesListSchemasDocumentPaginationContract(t *testing.T) {
	for _, schema := range []*jsonschema.Schema{
		mustTaskSeriesSchema[TaskSeriesListInput](t),
		mustTaskSeriesSchema[TaskSeriesOccurrenceListInput](t),
	} {
		limit := schema.Properties["limit"]
		if string(limit.Default) != "200" || limit.Minimum == nil || *limit.Minimum != 1 || limit.Maximum == nil || *limit.Maximum != 1000 {
			t.Fatalf("limit schema = %#v, want default=200 range=1..1000", limit)
		}
		offset := schema.Properties["offset"]
		if string(offset.Default) != "0" || offset.Minimum == nil || *offset.Minimum != 0 {
			t.Fatalf("offset schema = %#v, want default=0 minimum=0", offset)
		}
	}
}

func mustTaskSeriesSchema[T any](t *testing.T) *jsonschema.Schema {
	t.Helper()
	schema, err := jsonschema.For[T](nil)
	if err != nil {
		t.Fatal(err)
	}
	patchInputSchema[T](schema)
	if _, err := json.Marshal(schema); err != nil {
		t.Fatal(err)
	}
	return schema
}

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
	if series["url"] != mcpTestResourceBaseURL+"/workspaces/local/projects/ops/series/ops-s-1" {
		t.Fatalf("series URL = %#v", series["url"])
	}
	var textEnvelope ToolEnvelope
	if err := json.Unmarshal([]byte(renderedText(result)), &textEnvelope); err != nil {
		t.Fatalf("text envelope: %v", err)
	}
	textData, ok := textEnvelope.Data.(map[string]any)
	if !ok {
		t.Fatalf("text data = %T", textEnvelope.Data)
	}
	textSeries, ok := textData["series"].(map[string]any)
	if !ok || textSeries["url"] != series["url"] {
		t.Fatalf("text/structured URL diverged: text=%#v structured=%#v", textSeries, series)
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
		if firstOcc["url"] == nil || firstOcc["url"] == "" {
			t.Fatal("first_occurrence url 为空")
		}
		for _, field := range []string{"uuid", "task_slug", "project_seq", "entry", "modified", "start", "end"} {
			value, exists := firstOcc[field]
			if !exists || value != nil {
				t.Fatalf("projected first_occurrence %s = %#v, want explicit null", field, value)
			}
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

	badLimit := callTool(t, session, "task_series_list", map[string]any{"status": "active", "limit": 0})
	if !badLimit.IsError {
		t.Fatal("explicit limit=0 should be rejected by the MCP pagination contract")
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
	date := time.Unix(firstDue, 0).In(time.Local).Format("2006-01-02")
	listed := callTool(t, session, "task_series_list_occurrences", TaskSeriesOccurrenceListInput{
		ID: seriesID, Status: "all", DueAfter: date, DueBefore: date,
	})
	if listed.IsError {
		t.Fatalf("list occurrences error: %v", parseError(t, listed))
	}
	listedData := parseEnvelope(t, listed).Data.(map[string]any)
	if listedData["total"] != float64(1) {
		t.Fatalf("date range list = %#v, want skipped occurrence", listedData)
	}
}

func TestMCPTaskModifyRejectsProjectedOccurrenceProjectMoveWithoutMaterializing(t *testing.T) {
	srv, _ := newTestServer(t)
	session := connectClient(t, srv)
	callTool(t, session, "project_add", map[string]any{"slug": "ops", "name": "Ops"})
	callTool(t, session, "project_add", map[string]any{"slug": "other", "name": "Other"})
	firstDue := int64(1893456000)
	addResult := callTool(t, session, "task_series_add", TaskSeriesAddInput{
		Project: "ops", Title: "每日巡检", RecurrenceRule: "daily", FirstDue: &firstDue,
	})
	if addResult.IsError {
		t.Fatalf("series_add error: %v", parseError(t, addResult))
	}
	occurrenceRef := parseEnvelope(t, addResult).Data.(map[string]any)["first_occurrence"].(map[string]any)["id"].(string)

	result := callTool(t, session, "task_modify", TaskModifyInput{ID: occurrenceRef, Project: "other"})
	if !result.IsError {
		t.Fatal("task_modify expected error")
	}
	if got := parseError(t, result).Code; got != "task_occurrence_project_immutable" {
		t.Fatalf("error code = %q, want task_occurrence_project_immutable", got)
	}
	getResult := callTool(t, session, "task_get", TaskGetInput{ID: occurrenceRef})
	if getResult.IsError {
		t.Fatalf("task_get error: %v", parseError(t, getResult))
	}
	view, ok := parseEnvelope(t, getResult).Data.(map[string]any)
	if !ok {
		t.Fatalf("task_get data type = %T", parseEnvelope(t, getResult).Data)
	}
	if view["uuid"] != nil || view["task_slug"] != nil || nestedMap(t, view, "recurrence_info")["materialization"] != "projected" {
		t.Fatalf("failed task_modify changed projected occurrence: %#v", view)
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
