package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/app"
)

func authHeaderJSON(token string) map[string]string {
	return map[string]string{"Authorization": "bearer " + token, "Content-Type": "application/json"}
}

func TestHTTP_ConfigSchemaSetReflectsShowOnConsoleHome(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "config:write")
	rr := requestHTTPBody(t, fixture.server, http.MethodPut, "/api/v1/config-schema/ads.roi?workspace=local", `{
	  "value_type":"number",
	  "allowed_scopes":["workspace"],
	  "default_value":"1.8",
	  "show_on_console_home":true
	}`, authHeaderJSON(fixture.token))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	data := httpResponseDataMap(t, rr)
	show, _ := data["show_on_console_home"].(bool)
	if !show {
		t.Fatalf("show_on_console_home = %v, want true body=%s", data["show_on_console_home"], rr.Body.String())
	}
}

func TestHTTP_ConfigSchemaUsageCountsValues(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "config:write", "config:read", "project:write", "project:read")
	// 通过 service 层准备 schema 和 value，避免 HTTP business-key guard 限制
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store, ActorRef: "local", WorkspaceRef: "local"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ConfigSchemaSet(app.ConfigSchemaInput{
		Key: "ads.budget", ValueType: "number", AllowedScopes: []string{"workspace", "project"},
	}); err != nil {
		t.Fatalf("ConfigSchemaSet: %v", err)
	}
	if err := svc.SetConfig("ads.budget", "100"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	project, err := svc.AddProject(app.AddProjectInput{Slug: "api", Name: "API"})
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	if err := svc.ProjectConfigSet(project.ID, "ads.budget", "200"); err != nil {
		t.Fatalf("ProjectConfigSet: %v", err)
	}

	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/config-schema/ads.budget/usage?workspace=local", authHeaderJSON(fixture.token))
	if rr.Code != http.StatusOK {
		t.Fatalf("usage status = %d body=%s", rr.Code, rr.Body.String())
	}
	data := httpResponseDataMap(t, rr)
	if data["workspace_values"].(float64) != 1 || data["project_values"].(float64) != 1 || data["total_values"].(float64) != 2 {
		t.Fatalf("usage = %#v, want 1/1/2", data)
	}
}

func TestHTTP_ProjectConfigEffectiveResolvesSources(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "config:write", "config:read", "project:write", "project:read")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store, ActorRef: "local", WorkspaceRef: "local"})
	if err != nil {
		t.Fatal(err)
	}
	fallback := "fallback"
	for _, input := range []app.ConfigSchemaInput{
		{Key: "k.project", ValueType: "string", AllowedScopes: []string{"project"}},
		{Key: "k.workspace", ValueType: "string", AllowedScopes: []string{"workspace", "project"}},
		{Key: "k.default", ValueType: "string", AllowedScopes: []string{"project"}, DefaultValue: &fallback},
		{Key: "k.required", ValueType: "string", AllowedScopes: []string{"project"}, Required: true},
	} {
		if err := svc.ConfigSchemaSet(input); err != nil {
			t.Fatalf("ConfigSchemaSet(%s): %v", input.Key, err)
		}
	}
	project, err := svc.AddProject(app.AddProjectInput{Slug: "api", Name: "API"})
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	if err := svc.ProjectConfigSet(project.ID, "k.project", "project-value"); err != nil {
		t.Fatalf("ProjectConfigSet: %v", err)
	}
	if err := svc.SetConfig("k.workspace", "workspace-value"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}

	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/projects/api/config/effective?workspace=local", authHeaderJSON(fixture.token))
	if rr.Code != http.StatusOK {
		t.Fatalf("effective status = %d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rr.Body.String())
	}
	byKey := map[string]map[string]any{}
	for _, row := range payload.Data {
		byKey[row["key"].(string)] = row
	}
	if row, ok := byKey["k.project"]; !ok || row["source"] != "project" {
		t.Fatalf("k.project = %#v, want source project", byKey["k.project"])
	}
	if row, ok := byKey["k.workspace"]; !ok || row["source"] != "workspace" {
		t.Fatalf("k.workspace = %#v, want source workspace", byKey["k.workspace"])
	}
	if row, ok := byKey["k.default"]; !ok || row["source"] != "default" {
		t.Fatalf("k.default = %#v, want source default", byKey["k.default"])
	}
	if row, ok := byKey["k.required"]; !ok || row["source"] != "missing" {
		t.Fatalf("k.required = %#v, want source missing", byKey["k.required"])
	}
}

func TestHTTP_WorkspaceConfigEffectiveConsoleHomeMasksSecret(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "config:write", "config:read")
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store, ActorRef: "local", WorkspaceRef: "local"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ConfigSchemaSet(app.ConfigSchemaInput{
		Key: "ads.secret", ValueType: "string", AllowedScopes: []string{"workspace"},
		Secret: true, ShowOnConsoleHome: true, DefaultValue: strPtr("topsecret"),
	}); err != nil {
		t.Fatalf("ConfigSchemaSet: %v", err)
	}
	if err := svc.ConfigSchemaSet(app.ConfigSchemaInput{
		Key: "project.only", ValueType: "string", AllowedScopes: []string{"project"}, ShowOnConsoleHome: true,
	}); err != nil {
		t.Fatalf("ConfigSchemaSet: %v", err)
	}

	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/config/effective?workspace=local&console_home=true", authHeaderJSON(fixture.token))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rr.Body.String())
	}
	if len(payload.Data) != 1 {
		t.Fatalf("rows = %d, want 1 (only secret workspace key); body=%s", len(payload.Data), rr.Body.String())
	}
	row := payload.Data[0]
	if row["key"] != "ads.secret" {
		t.Fatalf("key = %v, want ads.secret", row["key"])
	}
	value, _ := row["value"].(string)
	if value == "topsecret" {
		t.Fatalf("secret value leaked in console home: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "•") {
		t.Fatalf("secret value not masked: %s", rr.Body.String())
	}
}

func strPtr(v string) *string { return &v }
