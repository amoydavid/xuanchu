package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/task"
	"github.com/spf13/cobra"
)

func TestProjectTemplateCommandTreeIsNarrow(t *testing.T) {
	root := NewRootCommand(Options{})
	template, _, err := root.Find([]string{"project", "template"})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, child := range template.Commands() {
		names = append(names, child.Name())
	}
	sort.Strings(names)
	if strings.Join(names, ",") != "instantiate,list" {
		t.Fatalf("template children = %v", names)
	}
	for _, forbidden := range []string{"save", "info", "preview", "archive", "snapshot", "candidate", "capture", "modify"} {
		for _, child := range template.Commands() {
			if child.Name() == forbidden {
				t.Fatalf("unexpected command %s", forbidden)
			}
		}
	}
}

func TestReadProjectTemplateInstantiateInputAcceptsOnlySafeJSON(t *testing.T) {
	secret := "sk-input-must-not-be-rendered"
	raw := `{"description":"覆盖说明","config_inputs":{"launch.region":"cn-north"},"secret_inputs":{"agent.api_key":"` + secret + `"},"assignee_replacements":{"old-user":"new-user","removed":null}}`
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader(raw))
	input, err := readProjectTemplateInstantiateInput(cmd, "-")
	if err != nil {
		t.Fatal(err)
	}
	if input.Description == nil || *input.Description != "覆盖说明" || input.ConfigInputs["launch.region"] != "cn-north" || input.SecretInputs["agent.api_key"] != secret {
		t.Fatalf("input = %#v", input)
	}
	if input.AssigneeReplacements["removed"] != nil || input.AssigneeReplacements["old-user"] == nil || *input.AssigneeReplacements["old-user"] != "new-user" {
		t.Fatalf("replacements = %#v", input.AssigneeReplacements)
	}

	cmd.SetIn(strings.NewReader(`{"description":"ok","preview":true}`))
	if _, err := readProjectTemplateInstantiateInput(cmd, "-"); err == nil {
		t.Fatal("unknown input field succeeded")
	}
	cmd.SetIn(strings.NewReader(strings.Repeat("x", projectTemplateCLIInputLimitBytes+1)))
	if _, err := readProjectTemplateInstantiateInput(cmd, "-"); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("oversized input error = %v", err)
	}
}

func TestRenderProjectTemplateListIsStableAndSecretSafe(t *testing.T) {
	page := app.ProjectTemplatePage{Items: []app.ProjectTemplateSummaryView{{
		ID: "template-1", Key: "launch", Name: "启动模板", Status: "active",
		CurrentSnapshot: &app.ProjectTemplateSnapshotSummaryView{
			ID: "snapshot-1", Version: 3, Hash: strings.Repeat("a", 64),
			Counts:             app.ComponentCounts{Configs: 1, Tasks: 2, Series: 3, Automations: 4},
			RequiredSecretKeys: []string{"agent.api_key"},
			ConfigInputs:       []app.ProjectTemplateConfigInputView{{Key: "launch.region", Label: "发布区域", ValueType: "string", Required: true, Status: "ready"}, {Key: "launch.note", Label: "备注", ValueType: "string", Status: "ready"}},
			CreatedBy:          task.ActorInfo{Type: "user", User: &task.UserInfo{ID: "user-1", Name: "alice"}},
		},
		CreatedBy: task.ActorInfo{Type: "user", User: &task.UserInfo{ID: "user-1", Name: "alice"}},
	}}, Total: 1, Limit: 50, Offset: 0}

	var human bytes.Buffer
	if err := renderProjectTemplatePage(&human, false, page); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"launch", "启动模板", "v3", "snapshot-1", strings.Repeat("a", 64), "configs=1", "tasks=2", "series=3", "automations=4", "agent.api_key", "launch.region(required)", "launch.note(optional)"} {
		if !strings.Contains(human.String(), want) {
			t.Fatalf("human output missing %q: %s", want, human.String())
		}
	}

	var jsonOut bytes.Buffer
	if err := renderProjectTemplatePage(&jsonOut, true, page); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(jsonOut.Bytes(), &payload); err != nil {
		t.Fatalf("JSON output = %q: %v", jsonOut.String(), err)
	}
	items := payload["items"].([]any)
	createdBy := items[0].(map[string]any)["created_by"].(map[string]any)
	if createdBy["type"] != "user" || payload["total"] != float64(1) {
		t.Fatalf("payload = %#v", payload)
	}
	current := items[0].(map[string]any)["current_snapshot"].(map[string]any)
	if len(current["config_inputs"].([]any)) != 2 {
		t.Fatalf("config inputs = %#v", current["config_inputs"])
	}
	for _, forbidden := range []string{"snapshot_json", "Versions", "Snapshot"} {
		if strings.Contains(jsonOut.String(), forbidden) {
			t.Fatalf("JSON output exposed %s: %s", forbidden, jsonOut.String())
		}
	}
}

func TestRenderProjectTemplateInstantiateResultUsesStableJSON(t *testing.T) {
	var out bytes.Buffer
	result := app.InstantiateResult{
		Project: app.ProjectView{ID: "project-1", WorkspaceID: "workspace-1", Slug: "newproj", Name: "新项目", Status: "planning"},
		Counts:  app.ComponentCounts{Configs: 1, Tasks: 2, Series: 3, Automations: 4},
	}
	if err := renderProjectTemplateInstantiateResult(&out, true, result); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("JSON output = %q: %v", out.String(), err)
	}
	project := payload["project"].(map[string]any)
	counts := payload["counts"].(map[string]any)
	if project["slug"] != "newproj" || counts["tasks"] != float64(2) || counts["automations"] != float64(4) {
		t.Fatalf("payload = %#v", payload)
	}
	for _, forbidden := range []string{"Project", "Counts", "TaskCount"} {
		if strings.Contains(out.String(), forbidden) {
			t.Fatalf("JSON output exposed Go field %s: %s", forbidden, out.String())
		}
	}
}

func TestProjectTemplateInstantiateRemoteReadsSecretFromStdinWithoutEcho(t *testing.T) {
	secret := "sk-cli-secret-never-echo"
	var requestBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"data":{"project":{"id":"project-1","workspace_id":"workspace-1","slug":"newproj","name":"新项目","status":"planning","task_count":0,"created_at":100,"modified_at":100},"counts":{"configs":1,"tasks":0,"series":0,"automations":0}}}`)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	opts := Options{Stdout: &stdout, Stderr: &stderr, Stdin: strings.NewReader(`{"secret_inputs":{"agent.api_key":"` + secret + `"}}`), Server: srv.URL, Token: "token", Workspace: "local"}
	cmd := NewRootCommand(opts)
	cmd.SetArgs([]string{"project", "template", "instantiate", "launch", "newproj", "name:新项目", "--snapshot", "snapshot-1", "--snapshot-hash", strings.Repeat("a", 64), "--start-date", "2026-08-01", "--input", "-"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v stderr=%s", err, stderr.String())
	}
	if requestBody["current_only"] != true || requestBody["snapshot_id"] != "snapshot-1" {
		t.Fatalf("request body = %#v", requestBody)
	}
	if strings.Contains(stdout.String(), secret) || strings.Contains(stderr.String(), secret) {
		t.Fatalf("secret leaked: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "newproj") || stderr.Len() != 0 {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}
