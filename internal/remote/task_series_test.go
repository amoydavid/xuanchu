package remote

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRemoteAddTaskSeriesSendsCorrectBody(t *testing.T) {
	var receivedPath, receivedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path + "?" + r.URL.RawQuery
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		io.WriteString(w, `{"data":{"series":{"id":"s1","title":"每日巡检","status":"active","recurrence_rule":"daily","first_due":123},"first_occurrence":{"id":"occ:s1:123","recurrence_info":{"materialization":"projected"}}}}`)
	}))
	defer srv.Close()
	client, err := NewClient(Options{BaseURL: srv.URL, Token: "tok"})
	if err != nil {
		t.Fatal(err)
	}
	firstDue := int64(123)
	result, err := client.AddTaskSeries(context.Background(), "local", AddTaskSeriesInput{
		Title: "每日巡检", Project: strPtrRemote("ops"),
		RecurrenceRule: "daily", FirstDue: &firstDue,
	})
	if err != nil {
		t.Fatalf("AddTaskSeries: %v", err)
	}
	if !strings.Contains(receivedPath, "/api/v1/task-series") {
		t.Fatalf("path = %q", receivedPath)
	}
	if !strings.Contains(receivedPath, "workspace=local") {
		t.Fatalf("path 缺少 workspace: %q", receivedPath)
	}
	if !strings.Contains(receivedBody, `"recurrence_rule":"daily"`) || !strings.Contains(receivedBody, `"title":"每日巡检"`) {
		t.Fatalf("body 缺少字段: %s", receivedBody)
	}
	if result.Series.ID != "s1" {
		t.Fatalf("series ID = %q want s1", result.Series.ID)
	}
	if result.FirstOccurrence == nil || result.FirstOccurrence.RecurrenceInfo.Materialization != "projected" {
		t.Fatalf("first_occurrence = %#v", result.FirstOccurrence)
	}
}

func TestRemoteListTaskSeriesBuildsQuery(t *testing.T) {
	var receivedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path + "?" + r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"data":{"items":[{"id":"s1"}],"total":1,"limit":0,"offset":0}}`)
	}))
	defer srv.Close()
	client, err := NewClient(Options{BaseURL: srv.URL, Token: "tok"})
	if err != nil {
		t.Fatal(err)
	}
	page, err := client.ListTaskSeries(context.Background(), TaskSeriesListInput{
		Workspace: "local", Status: "active", Q: "巡检", Sort: "next", Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListTaskSeries: %v", err)
	}
	if !strings.Contains(receivedPath, "status=active") || !strings.Contains(receivedPath, "sort=next") {
		t.Fatalf("query 缺少参数: %q", receivedPath)
	}
	if page.Total != 1 {
		t.Fatalf("total = %d want 1", page.Total)
	}
}

func TestRemoteModifyTaskSeriesForwardsClearAndUDAs(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"data":{"id":"s1","title":"每日巡检","status":"active","recurrence_rule":"daily","first_due":123}}`)
	}))
	defer srv.Close()
	client, err := NewClient(Options{BaseURL: srv.URL, Token: "tok"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ModifyTaskSeries(context.Background(), "local", "s1", ModifyTaskSeriesInput{
		Clear: []string{"priority", "uda.estimate"},
		UDAs:  map[string]string{"estimate": "3"},
	})
	if err != nil {
		t.Fatalf("ModifyTaskSeries: %v", err)
	}
	if _, ok := receivedBody["clear"]; !ok {
		t.Fatalf("body 缺少 clear: %#v", receivedBody)
	}
	if _, ok := receivedBody["udas"]; !ok {
		t.Fatalf("body 缺少 udas: %#v", receivedBody)
	}
}

func TestRemoteStopTaskSeriesSendsDelete(t *testing.T) {
	var receivedMethod, receivedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedPath = r.URL.Path + "?" + r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"data":{"id":"s1","status":"stopped"}}`)
	}))
	defer srv.Close()
	client, err := NewClient(Options{BaseURL: srv.URL, Token: "tok"})
	if err != nil {
		t.Fatal(err)
	}
	view, err := client.StopTaskSeries(context.Background(), "local", "s1", true)
	if err != nil {
		t.Fatalf("StopTaskSeries: %v", err)
	}
	if receivedMethod != http.MethodDelete {
		t.Fatalf("method = %q want DELETE", receivedMethod)
	}
	if !strings.Contains(receivedPath, "delete_open_occurrences=true") {
		t.Fatalf("缺少 delete_open: %q", receivedPath)
	}
	if view.Status != "stopped" {
		t.Fatalf("status = %q want stopped", view.Status)
	}
}

func TestRemoteListTaskSeriesOccurrencesForwardsRange(t *testing.T) {
	var receivedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path + "?" + r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"data":{"items":[],"total":0,"limit":10,"offset":0}}`)
	}))
	defer srv.Close()
	client, err := NewClient(Options{BaseURL: srv.URL, Token: "tok"})
	if err != nil {
		t.Fatal(err)
	}
	after, before := int64(100), int64(200)
	_, err = client.ListTaskSeriesOccurrences(context.Background(), TaskSeriesOccurrenceListInput{
		Workspace: "local", SeriesRef: "s1", Status: "pending",
		DueAfter: &after, DueBefore: &before, Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"due_after=100", "due_before=200", "status=pending"} {
		if !strings.Contains(receivedPath, want) {
			t.Fatalf("query 缺少 %s: %q", want, receivedPath)
		}
	}
}

func TestRemoteQueryTasksSendsOccurrenceMode(t *testing.T) {
	var receivedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path + "?" + r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"data":{"items":[],"total":0,"occurrence_mode":"expand"}}`)
	}))
	defer srv.Close()
	client, err := NewClient(Options{BaseURL: srv.URL, Token: "tok"})
	if err != nil {
		t.Fatal(err)
	}
	page, err := client.QueryTasks(context.Background(), TaskQueryInput{
		Workspace: "local", DueAfter: "2030-01-01", DueBefore: "2030-01-31",
		OccurrenceMode: "expand",
	})
	if err != nil {
		t.Fatalf("QueryTasks: %v", err)
	}
	if !strings.Contains(receivedPath, "occurrence_mode=expand") || !strings.Contains(receivedPath, "due_after=2030-01-01") {
		t.Fatalf("query 缺少参数: %q", receivedPath)
	}
	if page.OccurrenceMode != "expand" {
		t.Fatalf("mode = %q want expand", page.OccurrenceMode)
	}
}

func TestParseTaskOccurrenceDTOMaterializedShape(t *testing.T) {
	raw := json.RawMessage(`{
		"id":"occ:series-1:1000",
		"uuid":"task-1",
		"workspace_id":"workspace-1",
		"project_id":"project-1",
		"title":"每日巡检",
		"status":"pending",
		"entry":1000,
		"modified":1001,
		"due":2000,
		"tags":["ops"],
		"recurrence_info":{"role":"occurrence","series_id":"series-1","series_status":"active","rule":"daily","recurrence_at":2000,"materialization":"materialized"}
	}`)
	dto, err := parseTaskOccurrenceDTO(raw)
	if err != nil {
		t.Fatalf("parse materialized occurrence: %v", err)
	}
	if dto.ID != "occ:series-1:1000" || dto.UUID == nil || *dto.UUID != "task-1" {
		t.Fatalf("dto identity = %#v", dto)
	}
	if dto.Entry == nil || *dto.Entry != 1000 || len(dto.Tags) != 1 || dto.Tags[0] != "ops" {
		t.Fatalf("dto fields = %#v", dto)
	}
	if dto.RecurrenceInfo == nil || dto.RecurrenceInfo.Materialization != "materialized" {
		t.Fatalf("dto recurrence_info = %#v", dto.RecurrenceInfo)
	}
}

func strPtrRemote(s string) *string { return &s }
