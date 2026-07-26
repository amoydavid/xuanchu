package remote

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/app"
)

func TestRemoteListProjectTemplatesForInstantiationUsesActiveCurrentSurface(t *testing.T) {
	var query url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/project-templates" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		query = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"items":[{"id":"template-1","key":"launch","name":"启动模板","description":"发布流程","status":"active","current_snapshot":{"id":"snapshot-1","version":3,"hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","source_project_id":"source-1","counts":{"configs":1,"tasks":2,"series":3,"automations":4},"required_secret_keys":["agent.api_key"],"config_inputs":[{"key":"launch.region","label":"发布区域","description":"选择区域","value_type":"string","enum_values":["cn"],"required":true,"secret":false,"status":"ready"}],"created_by":{"type":"user","user":{"id":"user-1","name":"alice"}},"created_at":100},"created_by":{"type":"user","user":{"id":"user-1","name":"alice"}},"created_at":90,"modified_at":100}],"total":1,"limit":20,"offset":2}}`)
	}))
	defer srv.Close()
	client, err := NewClient(Options{BaseURL: srv.URL, Token: "token"})
	if err != nil {
		t.Fatal(err)
	}

	page, err := client.ListProjectTemplatesForInstantiation(context.Background(), "local", "启动", 20, 2)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"workspace": "local", "status": "active", "q": "启动", "limit": "20", "offset": "2"} {
		if query.Get(key) != want {
			t.Fatalf("query %s = %q, want %q", key, query.Get(key), want)
		}
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].CurrentSnapshot == nil {
		t.Fatalf("page = %#v", page)
	}
	got := page.Items[0]
	if got.Key != "launch" || got.CreatedBy.User == nil || got.CreatedBy.User.Name != "alice" || got.CurrentSnapshot.Counts.Automations != 4 {
		t.Fatalf("item = %#v", got)
	}
	if len(got.CurrentSnapshot.ConfigInputs) != 1 || got.CurrentSnapshot.ConfigInputs[0].Key != "launch.region" {
		t.Fatalf("config inputs = %#v", got.CurrentSnapshot.ConfigInputs)
	}
}

func TestRemoteInstantiateCurrentProjectTemplateSendsAtomicCurrentOnlyBody(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/project-templates/launch/instantiate" || r.URL.Query().Get("workspace") != "local" {
			t.Fatalf("request = %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"data":{"project":{"id":"project-1","workspace_id":"workspace-1","slug":"newproj","name":"新项目","description":"说明","status":"planning","task_count":2,"created_at":100,"modified_at":100},"counts":{"configs":1,"tasks":2,"series":0,"automations":0}}}`)
	}))
	defer srv.Close()
	client, err := NewClient(Options{BaseURL: srv.URL, Token: "token"})
	if err != nil {
		t.Fatal(err)
	}
	description := "说明"
	result, err := client.InstantiateCurrentProjectTemplate(context.Background(), "local", "launch", app.CurrentSnapshotInstantiateInput{
		SnapshotID: "snapshot-1", ExpectedHash: strings.Repeat("a", 64), ProjectSlug: "newproj", ProjectName: "新项目",
		Description: &description, StartDate: "2026-08-01", ConfigInputs: map[string]string{"launch.region": "cn"}, SecretInputs: map[string]string{"agent.api_key": "sk-test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if body["current_only"] != true || body["snapshot_id"] != "snapshot-1" || body["expected_snapshot_hash"] != strings.Repeat("a", 64) {
		t.Fatalf("body = %#v", body)
	}
	if body["config_inputs"].(map[string]any)["launch.region"] != "cn" {
		t.Fatalf("config_inputs = %#v", body["config_inputs"])
	}
	if result.Project.Slug != "newproj" || result.Counts.Tasks != 2 {
		t.Fatalf("result = %#v", result)
	}
}

func TestRemoteInstantiateCurrentProjectTemplateReturnsHashDrift(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["current_only"] != true {
			t.Fatalf("current_only = %#v", body["current_only"])
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `{"error":{"code":"project_template_snapshot_hash_mismatch","message":"template current snapshot changed"}}`)
	}))
	defer srv.Close()
	client, err := NewClient(Options{BaseURL: srv.URL, Token: "token"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.InstantiateCurrentProjectTemplate(context.Background(), "local", "launch", app.CurrentSnapshotInstantiateInput{
		SnapshotID: "historical", ExpectedHash: strings.Repeat("b", 64), ProjectSlug: "newproj", ProjectName: "新项目", StartDate: "2026-08-01",
	})
	apiErr, ok := err.(APIError)
	if !ok || apiErr.Status != http.StatusConflict || apiErr.Code != "project_template_snapshot_hash_mismatch" {
		t.Fatalf("error = %#v", err)
	}
}

func TestRemoteProjectTemplateSurfaceIsNarrow(t *testing.T) {
	typeOfClient := reflect.TypeOf((*Client)(nil))
	for _, want := range []string{"ListProjectTemplatesForInstantiation", "InstantiateCurrentProjectTemplate"} {
		if _, ok := typeOfClient.MethodByName(want); !ok {
			t.Errorf("missing method %s", want)
		}
	}
	for _, forbidden := range []string{"CaptureProjectTemplate", "GetProjectTemplate", "ModifyProjectTemplate", "ArchiveProjectTemplate", "PreviewProjectTemplateInstantiation", "ListProjectTemplateSnapshots"} {
		if _, ok := typeOfClient.MethodByName(forbidden); ok {
			t.Errorf("unexpected method %s", forbidden)
		}
	}
}
