package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

const projectTemplateFixtureSecret = "fixture-template-secret-literal"

func TestProjectTemplateCaptureRequiresAllSelectionArrays(t *testing.T) {
	fixture := newHTTPProjectTemplateFixture(t)
	body := `{"key":"launch","name":"启动","capture":{"source_project":"ops","anchor_date":"2026-07-20","selection":{"task_refs":[]}}}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/project-templates?workspace=local", body, authHeader(fixture.token))
	assertHTTPErrorCode(t, rr, http.StatusUnprocessableEntity, "project_template_selection_invalid")
}

func TestProjectTemplateRequestMapsConfigPoliciesAndInputs(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{
		"source_project":"ops",
		"anchor_date":"2026-07-20",
		"selection":{"config_keys":["launch.region"],"task_refs":[],"series_refs":[],"automation_rule_ids":[]},
		"config_policies":[{"key":"launch.region","strategy":"prompt","required":true}]
	}`))
	rr := httptest.NewRecorder()
	var capture projectTemplateCaptureRequest
	if !decodeProjectTemplateJSON(rr, req, &capture) {
		t.Fatalf("decode capture failed: status=%d body=%s", rr.Code, rr.Body.String())
	}
	input := capture.appInput()
	if len(input.ConfigPolicies) != 1 || input.ConfigPolicies[0].Key != "launch.region" || !input.ConfigPolicies[0].Required {
		t.Fatalf("capture policies = %#v", input.ConfigPolicies)
	}

	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{
		"expected_snapshot_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"project_slug":"launch","project_name":"发布","start_date":"2026-07-20",
		"config_inputs":{"launch.region":"cn-north"}
	}`))
	rr = httptest.NewRecorder()
	var instantiate projectTemplateInstantiateRequest
	if !decodeProjectTemplateJSON(rr, req, &instantiate) {
		t.Fatalf("decode instantiate failed: status=%d body=%s", rr.Code, rr.Body.String())
	}
	if instantiate.ConfigInputs["launch.region"] != "cn-north" {
		t.Fatalf("config inputs = %#v", instantiate.ConfigInputs)
	}
}

func TestProjectTemplateWorkspaceIsExplicit(t *testing.T) {
	fixture := newHTTPProjectTemplateFixture(t)
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/project-templates", authHeader(fixture.token))
	assertHTTPErrorCode(t, rr, http.StatusBadRequest, "workspace_required")
}

func TestProjectTemplateSaveDefaultsKeyAndVersionsExistingTemplate(t *testing.T) {
	fixture := newHTTPProjectTemplateFixture(t)
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store, ActorRef: "local", WorkspaceRef: "local", TokenSecretKey: fixture.server.secretKey})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddProject(app.AddProjectInput{Slug: "ops", Name: "运维"}); err != nil {
		t.Fatal(err)
	}
	previewBody := `{"source_project":"ops","anchor_date":"2026-07-20","selection":{"config_keys":[],"task_refs":[],"series_refs":[],"automation_rule_ids":[]}}`
	previewRequest := func() string {
		rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/project-templates/capture-preview?workspace=local", previewBody, authHeader(fixture.token))
		if rr.Code != http.StatusOK {
			t.Fatalf("preview status=%d body=%s", rr.Code, rr.Body.String())
		}
		var envelope struct {
			Data struct {
				SourceHash string `json:"source_hash"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		return envelope.Data.SourceHash
	}
	save := func(name, description, hash string, includeKey bool) projectTemplateDetailResponse {
		body := map[string]any{
			"name": name, "description": description,
			"capture": map[string]any{
				"source_project": "ops", "anchor_date": "2026-07-20", "expected_source_hash": hash,
				"selection": map[string]any{"config_keys": []string{}, "task_refs": []string{}, "series_refs": []string{}, "automation_rule_ids": []string{}},
			},
		}
		if includeKey {
			body["key"] = "ops"
		}
		raw, _ := json.Marshal(body)
		rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/project-templates?workspace=local", string(raw), authHeader(fixture.token))
		if rr.Code != http.StatusCreated {
			t.Fatalf("save status=%d body=%s", rr.Code, rr.Body.String())
		}
		var envelope struct {
			Data projectTemplateDetailResponse `json:"data"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		return envelope.Data
	}

	first := save("运维模板", "第一版", previewRequest(), false)
	if first.Template.Key != "ops" || first.Template.CurrentSnapshot == nil || first.Template.CurrentSnapshot.Version != 1 {
		t.Fatalf("first = %#v", first.Template)
	}
	description := "来源项目第二版"
	if err := svc.ModifyProject("ops", app.ModifyProjectInput{Description: &description}); err != nil {
		t.Fatal(err)
	}
	second := save("最新运维模板", "第二版", previewRequest(), true)
	if second.Template.ID != first.Template.ID || second.Template.Name != "最新运维模板" || second.Template.CurrentSnapshot == nil || second.Template.CurrentSnapshot.Version != 2 {
		t.Fatalf("second = %#v", second.Template)
	}
	list := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/project-templates?workspace=local&status=all", authHeader(fixture.token))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"total":1`) {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
}

func TestProjectTemplateResponsesNeverExposeRawSnapshotOrSecrets(t *testing.T) {
	fixture := newHTTPProjectTemplateFixture(t)
	created := seedHTTPProjectTemplate(t, fixture)

	requests := []struct {
		name, method, path, body string
	}{
		{name: "list", method: http.MethodGet, path: "/api/v1/project-templates?workspace=local&status=all"},
		{name: "detail", method: http.MethodGet, path: "/api/v1/project-templates/launch?workspace=local&snapshot_id=" + created.Template.CurrentSnapshot.ID},
		{name: "capture preview", method: http.MethodPost, path: "/api/v1/project-templates/capture-preview?workspace=local", body: captureRequestJSON("ops", "")},
		{name: "instantiate preview", method: http.MethodPost, path: "/api/v1/project-templates/launch/instantiate-preview?workspace=local", body: instantiateRequestJSON(created, "preview1")},
		{name: "instantiate", method: http.MethodPost, path: "/api/v1/project-templates/launch/instantiate?workspace=local", body: instantiateRequestJSON(created, "created1")},
	}
	for _, test := range requests {
		t.Run(test.name, func(t *testing.T) {
			rr := requestHTTPBody(t, fixture.server, test.method, test.path, test.body, authHeader(fixture.token))
			if rr.Code < 200 || rr.Code >= 300 {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
			body := rr.Body.String()
			for _, forbidden := range []string{"snapshot_json", projectTemplateFixtureSecret} {
				if strings.Contains(body, forbidden) {
					t.Fatalf("response exposed %q: %s", forbidden, body)
				}
			}
			assertSnakeCaseResponse(t, body)
		})
	}
}

func TestProjectTemplateInstantiateCurrentOnlyRejectsHistoricalSnapshot(t *testing.T) {
	fixture := newHTTPProjectTemplateFixture(t)
	historical := seedHTTPProjectTemplate(t, fixture)

	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store, ActorRef: "local", WorkspaceRef: "local", TokenSecretKey: fixture.server.secretKey})
	if err != nil {
		t.Fatal(err)
	}
	description := "第二版项目说明"
	if err := svc.ModifyProject("ops", app.ModifyProjectInput{Description: &description}); err != nil {
		t.Fatal(err)
	}
	input := app.CaptureInput{
		SourceProjectRef: "ops", AnchorDate: "2026-07-20",
		Selection:         app.CaptureSelection{ConfigKeys: []string{"template.api_key"}, TaskRefs: []string{}, SeriesRefs: []string{}, AutomationRuleIDs: []string{}},
		SelectionPresence: app.SelectionPresence{ConfigKeys: true, TaskRefs: true, SeriesRefs: true, AutomationRuleIDs: true},
	}
	preview, err := svc.PreviewProjectTemplateCapture(input)
	if err != nil {
		t.Fatal(err)
	}
	input.ExpectedSourceHash = preview.SourceHash
	current, err := svc.CreateProjectTemplateSnapshot(historical.Template.Key, input)
	if err != nil {
		t.Fatal(err)
	}
	if historical.Template.CurrentSnapshot.ID == current.Template.CurrentSnapshot.ID {
		t.Fatal("second snapshot did not advance current snapshot")
	}

	body := map[string]any{}
	if err := json.Unmarshal([]byte(instantiateRequestJSON(historical, "remoteold")), &body); err != nil {
		t.Fatal(err)
	}
	body["current_only"] = true
	raw, _ := json.Marshal(body)
	rr := requestHTTPBody(t, fixture.server, http.MethodPost,
		"/api/v1/project-templates/launch/instantiate?workspace=local", string(raw), authHeader(fixture.token))
	assertHTTPErrorCode(t, rr, http.StatusConflict, "project_template_snapshot_hash_mismatch")

	// Web 治理调用未声明 current_only 时继续允许固定历史 Snapshot。
	rr = requestHTTPBody(t, fixture.server, http.MethodPost,
		"/api/v1/project-templates/launch/instantiate?workspace=local", instantiateRequestJSON(historical, "webold"), authHeader(fixture.token))
	if rr.Code != http.StatusCreated {
		t.Fatalf("historical web instantiate status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestProjectTemplateRequestBodyLimitIsNineMiB(t *testing.T) {
	fixture := newHTTPProjectTemplateFixture(t)
	body := `{"key":"launch","name":"` + strings.Repeat("x", (9<<20)+1) + `"}`
	for _, path := range []string{
		"/api/v1/project-templates?workspace=local",
		"/api/v1/project-templates/launch/archive?workspace=local",
	} {
		rr := requestHTTPBody(t, fixture.server, http.MethodPost, path, body, authHeader(fixture.token))
		assertHTTPErrorCode(t, rr, http.StatusRequestEntityTooLarge, "api_payload_too_large")
	}
}

func TestProjectTemplateResolveSelectionPreservesSnakeCaseFilters(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{
		"kind":"task",
		"task":{"q":"launch","status":"pending","priority":"H","assignees":["alice"],"tags":["ops"],"due_after":"2026-07-20","due_before":"2026-07-21","query":"priority:H","sort":"due"}
	}`))
	rr := httptest.NewRecorder()
	var body projectTemplateResolveSelectionRequest
	if !decodeProjectTemplateJSON(rr, req, &body) {
		t.Fatalf("decode failed: status=%d body=%s", rr.Code, rr.Body.String())
	}
	if body.Task == nil || body.Task.DueAfter != "2026-07-20" || body.Task.DueBefore != "2026-07-21" || len(body.Task.Assignees) != 1 || len(body.Task.Tags) != 1 {
		t.Fatalf("task filter was not preserved: %#v", body.Task)
	}
}

func TestProjectTemplateErrorStatusMapping(t *testing.T) {
	tests := map[string]int{
		"project_template_not_found":                   http.StatusNotFound,
		"project_template_snapshot_not_found":          http.StatusNotFound,
		"project_template_key_invalid":                 http.StatusConflict,
		"project_template_key_conflict":                http.StatusConflict,
		"project_template_snapshot_hash_mismatch":      http.StatusConflict,
		"project_template_snapshot_version_conflict":   http.StatusConflict,
		"project_template_source_changed":              http.StatusConflict,
		"project_template_archived":                    http.StatusConflict,
		"project_template_concurrency_conflict":        http.StatusConflict,
		"project_template_snapshot_too_large":          http.StatusRequestEntityTooLarge,
		"project_template_candidate_limit_exceeded":    http.StatusUnprocessableEntity,
		"project_template_candidate_invalid":           http.StatusUnprocessableEntity,
		"project_template_snapshot_schema_unsupported": http.StatusUnprocessableEntity,
		"project_template_snapshot_invalid":            http.StatusUnprocessableEntity,
		"project_template_dependency_missing":          http.StatusUnprocessableEntity,
		"project_template_attachment_unsupported":      http.StatusUnprocessableEntity,
		"project_template_member_unavailable":          http.StatusUnprocessableEntity,
		"project_template_config_invalid":              http.StatusUnprocessableEntity,
		"project_template_secret_required":             http.StatusUnprocessableEntity,
		"project_template_uda_invalid":                 http.StatusUnprocessableEntity,
		"project_template_automation_invalid":          http.StatusUnprocessableEntity,
		"project_template_date_out_of_range":           http.StatusUnprocessableEntity,
		"project_template_ref_cycle":                   http.StatusUnprocessableEntity,
		"project_template_selection_invalid":           http.StatusUnprocessableEntity,
	}
	for code, want := range tests {
		if got := statusForAppErrorCode(code); got != want {
			t.Errorf("statusForAppErrorCode(%q)=%d, want %d", code, got, want)
		}
	}
}

func TestProjectTemplateValidationErrorReturnsTypedIssues(t *testing.T) {
	rr := httptest.NewRecorder()
	writeProjectTemplateAppError(rr, app.ProjectTemplateValidationError{Issues: []app.ProjectTemplateIssue{{
		Code: "project_template_config_invalid", Severity: "blocking", Component: "config", SourceRef: "agent.api_key", Field: "value", Message: "invalid config",
	}}})
	assertHTTPErrorCode(t, rr, http.StatusUnprocessableEntity, "project_template_config_invalid")
	for _, want := range []string{`"issues"`, `"severity":"blocking"`, `"component":"config"`, `"source_ref":"agent.api_key"`, `"field":"value"`} {
		if !strings.Contains(rr.Body.String(), want) {
			t.Fatalf("typed issue response missing %s: %s", want, rr.Body.String())
		}
	}
}

func TestProjectTemplateProjectScopedTokenIsForbidden(t *testing.T) {
	fixture := newHTTPProjectTemplateFixture(t)
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store, ActorRef: "local", WorkspaceRef: "local", TokenSecretKey: fixture.server.secretKey})
	if err != nil {
		t.Fatal(err)
	}
	project, err := svc.AddProject(app.AddProjectInput{Slug: "scoped", Name: "Scoped"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateToken(app.CreateTokenInput{
		Name: "project-scoped-template", Scopes: []string{"project:read", "project:write"},
		WorkspaceRefs: []string{"local"}, ProjectRefs: []string{project.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/project-templates?workspace=local", authHeader(created.RawToken))
	assertHTTPErrorCode(t, rr, http.StatusForbidden, "project_scope_denied")
}

func TestProjectTemplateCandidateRoutesWorkForWorkspaceToken(t *testing.T) {
	fixture := newHTTPProjectTemplateFixture(t)
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store, ActorRef: "local", WorkspaceRef: "local"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddProject(app.AddProjectInput{Slug: "ops", Name: "Ops"}); err != nil {
		t.Fatal(err)
	}
	requests := []struct {
		method, path, body string
	}{
		{http.MethodGet, "/api/v1/projects/ops/template-candidates/tasks?workspace=local", ""},
		{http.MethodGet, "/api/v1/projects/ops/template-candidates/series?workspace=local", ""},
		{http.MethodGet, "/api/v1/projects/ops/template-candidates/configs?workspace=local", ""},
		{http.MethodGet, "/api/v1/projects/ops/template-candidates/automations?workspace=local", ""},
		{http.MethodPost, "/api/v1/projects/ops/template-candidates/resolve-selection?workspace=local", `{"kind":"task","task":{}}`},
	}
	for _, test := range requests {
		rr := requestHTTPBody(t, fixture.server, test.method, test.path, test.body, authHeader(fixture.token))
		if rr.Code != http.StatusOK {
			t.Errorf("%s %s status=%d body=%s", test.method, test.path, rr.Code, rr.Body.String())
		}
	}
}

func TestProjectTemplateAutomationCandidateReturnsJSONActorInfo(t *testing.T) {
	fixture := newHTTPProjectTemplateFixture(t)
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store, ActorRef: "local", WorkspaceRef: "local"})
	if err != nil {
		t.Fatal(err)
	}
	project, err := svc.AddProject(app.AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	enabled := true
	tokenID, tokenName, tokenPrefix := "token-1", "部署机器人", "xuanchu_tat_1234"
	if err := storage.NewProjectAutomationRuleRepository(fixture.server.store.DB()).Create(storage.AutomationRule{
		ID: "token-rule", WorkspaceID: project.WorkspaceID, ScopeType: storage.AutomationScopeProject, ScopeID: project.ID, Name: "Token 规则", Enabled: &enabled,
		TriggerType: "event", TriggerConfigJSON: `{}`, ConditionJSON: `{}`, ActionConfigJSON: `{}`, ContextConfigJSON: `{}`,
		CreatedByActorType: auth.TokenTypeTenantAccess, CreatedByTokenID: &tokenID, CreatedByTokenName: &tokenName, CreatedByTokenPrefix: &tokenPrefix,
		CreatedAt: 1, ModifiedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	rr := requestHTTP(t, fixture.server, http.MethodGet,
		"/api/v1/projects/ops/template-candidates/automations?workspace=local&status=all&trigger_type=all", authHeader(fixture.token))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var envelope struct {
		Data struct {
			Items []struct {
				CreatedBy task.JSONActorInfo `json:"created_by"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Data.Items) != 1 {
		t.Fatalf("items=%#v", envelope.Data.Items)
	}
	actor := envelope.Data.Items[0].CreatedBy
	if actor.Type != auth.TokenTypeTenantAccess || actor.User != nil || actor.Token == nil || actor.Token.ID != tokenID || actor.Token.Name != tokenName || actor.Token.Prefix != tokenPrefix {
		t.Fatalf("created_by=%#v", actor)
	}
}

func TestProjectTemplateResolveSelectionRejectsMismatchedFilter(t *testing.T) {
	fixture := newHTTPProjectTemplateFixture(t)
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store, ActorRef: "local", WorkspaceRef: "local"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddProject(app.AddProjectInput{Slug: "ops", Name: "Ops"}); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		`{"kind":"task","config":{"q":"all"}}`,
		`{"kind":"task","task":{"mode":"secret"}}`,
	} {
		rr := requestHTTPBody(t, fixture.server, http.MethodPost,
			"/api/v1/projects/ops/template-candidates/resolve-selection?workspace=local", body, authHeader(fixture.token))
		assertHTTPErrorCode(t, rr, http.StatusUnprocessableEntity, "project_template_candidate_invalid")
	}
}

func TestProjectTemplateSnapshotCapturePreviewChecksTemplateLifecycle(t *testing.T) {
	fixture := newHTTPProjectTemplateFixture(t)
	created := seedHTTPProjectTemplate(t, fixture)
	body := captureRequestJSON("ops", "")
	headers := authHeader(fixture.token)

	missing := requestHTTPBody(t, fixture.server, http.MethodPost,
		"/api/v1/project-templates/missing/snapshots/capture-preview?workspace=local", body, headers)
	assertHTTPErrorCode(t, missing, http.StatusNotFound, "project_template_not_found")
	byID := requestHTTPBody(t, fixture.server, http.MethodPost,
		"/api/v1/project-templates/"+created.Template.ID+"/snapshots/capture-preview?workspace=local", body, headers)
	if byID.Code != http.StatusOK {
		t.Fatalf("UUID lookup status=%d body=%s", byID.Code, byID.Body.String())
	}

	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store, ActorRef: "local", WorkspaceRef: "local"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ArchiveProjectTemplate(created.Template.ID); err != nil {
		t.Fatal(err)
	}
	archived := requestHTTPBody(t, fixture.server, http.MethodPost,
		"/api/v1/project-templates/launch/snapshots/capture-preview?workspace=local", body, headers)
	assertHTTPErrorCode(t, archived, http.StatusConflict, "project_template_archived")
}

func TestProjectTemplatePaginationRejectsOutOfRangeValues(t *testing.T) {
	fixture := newHTTPProjectTemplateFixture(t)
	for _, query := range []string{"limit=-1", "limit=101", "offset=-1"} {
		rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/project-templates?workspace=local&"+query, authHeader(fixture.token))
		assertHTTPErrorCode(t, rr, http.StatusUnprocessableEntity, "project_template_candidate_invalid")
	}
}

func newHTTPProjectTemplateFixture(t *testing.T) httpTokenFixture {
	t.Helper()
	return newHTTPServerWithTokenFixture(t,
		"project:read", "project:write", "task:read", "task:write",
		"config:read", "config:write", "hook:read", "hook:write",
	)
}

func seedHTTPProjectTemplate(t *testing.T, fixture httpTokenFixture) app.ProjectTemplateView {
	t.Helper()
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store, ActorRef: "local", WorkspaceRef: "local", TokenSecretKey: fixture.server.secretKey})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddProject(app.AddProjectInput{Slug: "ops", Name: "运维"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.ConfigSchemaSet(app.ConfigSchemaInput{
		Key: "template.api_key", Label: "模板密钥", ValueType: "string", AllowedScopes: []string{"project"}, Secret: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.ProjectConfigSet("ops", "template.api_key", projectTemplateFixtureSecret); err != nil {
		t.Fatal(err)
	}
	input := app.CaptureInput{
		SourceProjectRef: "ops", AnchorDate: "2026-07-20",
		Selection:         app.CaptureSelection{ConfigKeys: []string{"template.api_key"}, TaskRefs: []string{}, SeriesRefs: []string{}, AutomationRuleIDs: []string{}},
		SelectionPresence: app.SelectionPresence{ConfigKeys: true, TaskRefs: true, SeriesRefs: true, AutomationRuleIDs: true},
	}
	preview, err := svc.PreviewProjectTemplateCapture(input)
	if err != nil {
		t.Fatal(err)
	}
	input.ExpectedSourceHash = preview.SourceHash
	created, err := svc.CreateProjectTemplate(app.CreateTemplateInput{Key: "launch", Name: "启动", Description: "模板", Capture: input})
	if err != nil {
		t.Fatal(err)
	}
	return created
}

func captureRequestJSON(sourceProject, expectedHash string) string {
	body := map[string]any{
		"source_project": sourceProject,
		"anchor_date":    "2026-07-20",
		"selection":      map[string]any{"config_keys": []string{"template.api_key"}, "task_refs": []string{}, "series_refs": []string{}, "automation_rule_ids": []string{}},
	}
	if expectedHash != "" {
		body["expected_source_hash"] = expectedHash
	}
	raw, _ := json.Marshal(body)
	return string(raw)
}

func instantiateRequestJSON(created app.ProjectTemplateView, slug string) string {
	body := map[string]any{
		"snapshot_id":            created.Template.CurrentSnapshot.ID,
		"expected_snapshot_hash": created.Template.CurrentSnapshot.Hash,
		"project_slug":           slug,
		"project_name":           "从模板创建",
		"start_date":             "2026-07-20",
	}
	raw, _ := json.Marshal(body)
	return string(raw)
}
