package integration

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/storage"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func postgresE2EAdminURL(t *testing.T) string {
	t.Helper()
	adminURL := strings.TrimSpace(os.Getenv("XUANCHU_E2E_POSTGRES_ADMIN_URL"))
	if adminURL == "" {
		t.Skip("XUANCHU_E2E_POSTGRES_ADMIN_URL not set")
	}
	if _, err := exec.LookPath("psql"); err != nil {
		t.Skip("psql not found; skipping PostgreSQL E2E")
	}
	return adminURL
}

func createPostgresE2EDatabase(t *testing.T, adminURL string) string {
	t.Helper()
	dbName := fmt.Sprintf("xuanchu_e2e_%d_%d", time.Now().UnixNano(), os.Getpid())
	psqlExec(t, adminURL, "CREATE DATABASE "+dbName)
	t.Cleanup(func() {
		psqlExec(t, adminURL, "DROP DATABASE IF EXISTS "+dbName)
	})
	parsed, err := url.Parse(adminURL)
	if err != nil {
		t.Fatalf("parse admin postgres url: %v", err)
	}
	parsed.Path = "/" + dbName
	return parsed.String()
}

func psqlExec(t *testing.T, dbURL string, sql string) {
	t.Helper()
	cmd := exec.Command("psql", dbURL, "-v", "ON_ERROR_STOP=1", "-q", "-c", sql)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("psql %q error = %v\n%s", sql, err, out)
	}
}

func psqlScalar(t *testing.T, dbURL string, sql string) string {
	t.Helper()
	cmd := exec.Command("psql", dbURL, "-v", "ON_ERROR_STOP=1", "-q", "-t", "-A", "-c", sql)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("psql %q error = %v\n%s", sql, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestPostgresE2EProjectTemplateStorage(t *testing.T) {
	adminURL := postgresE2EAdminURL(t)
	dbURL := createPostgresE2EDatabase(t, adminURL)
	store, err := storage.Open(dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	sourceProject := storage.Project{
		ID: "source-project", WorkspaceID: ws.ID, Slug: "source", Name: "模板来源项目",
		Description: "", Status: "active", SettingsJSON: "{}", NextTaskSeq: 1, NextSeriesSeq: 1,
		CreatedAt: 100, ModifiedAt: 100,
	}
	if err := store.DB().Create(&sourceProject).Error; err != nil {
		t.Fatal(err)
	}
	repo := storage.NewProjectTemplateRepository(store.DB())
	template := storage.ProjectTemplate{
		ID: "postgres-template", WorkspaceID: ws.ID, Key: "launch", Name: "PostgreSQL 模板",
		Description: "", Status: "active", CreatedByActorType: "user", CreatedAt: 100, ModifiedAt: 100,
	}
	if err := repo.Create(template); err != nil {
		t.Fatal(err)
	}
	appendSnapshot := func(id, hash string) storage.ProjectTemplateSnapshot {
		t.Helper()
		var row storage.ProjectTemplateSnapshot
		err := store.Transaction(func(tx *storage.Store) error {
			var appendErr error
			row, appendErr = storage.NewProjectTemplateRepository(tx.DB()).AppendSnapshotLocked(ws.ID, template.ID, storage.ProjectTemplateSnapshot{
				ID: id, SourceProjectID: "source-project", SnapshotJSON: `{"schema":"fixture/v1","value":"opaque"}`,
				SnapshotHash: hash, CreatedByActorType: "user", CreatedAt: 100,
			})
			return appendErr
		})
		if err != nil {
			t.Fatal(err)
		}
		return row
	}
	if first := appendSnapshot("postgres-snapshot-1", "postgres-hash-1"); first.Version != 1 {
		t.Fatalf("first version = %d, want 1", first.Version)
	}
	if second := appendSnapshot("postgres-snapshot-2", "postgres-hash-2"); second.Version != 2 {
		t.Fatalf("second version = %d, want 2", second.Version)
	}

	var dataType string
	if err := store.DB().Raw(`SELECT data_type FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'project_template_snapshots' AND column_name = 'snapshot_json'`).Scan(&dataType).Error; err != nil {
		t.Fatal(err)
	}
	if dataType != "text" {
		t.Fatalf("snapshot_json data type = %q, want text", dataType)
	}
	var versions int64
	if err := store.DB().Raw("SELECT count(DISTINCT version) FROM project_template_snapshots WHERE template_id = ?", template.ID).Scan(&versions).Error; err != nil {
		t.Fatal(err)
	}
	if versions != 2 {
		t.Fatalf("distinct versions = %d, want 2", versions)
	}
	assertPostgresProjectTemplateForeignKey(t, store, "project_templates", "current_snapshot_id,id,workspace_id", "project_template_snapshots", "id,template_id,workspace_id")
	assertPostgresProjectTemplateForeignKey(t, store, "project_template_snapshots", "template_id,workspace_id", "project_templates", "id,workspace_id")
	assertPostgresProjectTemplateForeignKey(t, store, "project_template_snapshots", "source_project_id,workspace_id", "projects", "id,workspace_id")
	if err := store.DB().Where("id = ?", "postgres-snapshot-2").Delete(&storage.ProjectTemplateSnapshot{}).Error; err == nil {
		t.Fatal("delete current snapshot succeeded, want RESTRICT failure")
	}
	if err := store.DB().Where("id = ?", template.ID).Delete(&storage.ProjectTemplate{}).Error; err == nil {
		t.Fatal("delete template with snapshots succeeded, want RESTRICT failure")
	}
	if err := store.DB().Where("id = ?", sourceProject.ID).Delete(&storage.Project{}).Error; err == nil {
		t.Fatal("delete source project with snapshot succeeded, want RESTRICT failure")
	}
	if _, err := repo.GetByRef("other-workspace", template.ID); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("cross-workspace template lookup err=%v, want ErrNotFound", err)
	}
}

func TestPostgresE2EProjectTemplateLegacySchemaUpgrade(t *testing.T) {
	adminURL := postgresE2EAdminURL(t)
	dbURL := createPostgresE2EDatabase(t, adminURL)
	workspaceID := seedPostgresE2EProjectTemplateLegacyPrerequisites(t, dbURL)
	createPostgresE2ELegacyProjectTemplateSchema(t, dbURL, workspaceID, workspaceID)

	store, err := storage.Open(dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	assertPostgresProjectTemplateForeignKey(t, store, "project_templates", "current_snapshot_id,id,workspace_id", "project_template_snapshots", "id,template_id,workspace_id")
	assertPostgresProjectTemplateForeignKey(t, store, "project_template_snapshots", "template_id,workspace_id", "project_templates", "id,workspace_id")
	assertPostgresProjectTemplateForeignKey(t, store, "project_template_snapshots", "source_project_id,workspace_id", "projects", "id,workspace_id")
	if _, err := storage.NewProjectTemplateRepository(store.DB()).GetSnapshot(workspaceID, "legacy-template", "legacy-snapshot"); err != nil {
		t.Fatalf("legacy snapshot was not retained: %v", err)
	}
}

func TestPostgresE2EProjectTemplateLegacySchemaUpgradeRollsBackOnDirtyData(t *testing.T) {
	adminURL := postgresE2EAdminURL(t)
	dbURL := createPostgresE2EDatabase(t, adminURL)
	workspaceID := seedPostgresE2EProjectTemplateLegacyPrerequisites(t, dbURL)
	createPostgresE2ELegacyProjectTemplateSchema(t, dbURL, workspaceID, "legacy-other-workspace")

	store, err := storage.Open(dbURL)
	if store != nil {
		_ = store.Close()
	}
	if err == nil {
		t.Fatal("Open(dirty legacy schema) succeeded, want composite foreign key migration failure")
	}
	if got := psqlScalar(t, dbURL, `
SELECT string_agg(source_column.attname, ',' ORDER BY source_key.ordinality)
FROM pg_constraint AS pg_fk
JOIN pg_class AS source_table ON source_table.oid = pg_fk.conrelid
JOIN unnest(pg_fk.conkey) WITH ORDINALITY AS source_key(attnum, ordinality) ON TRUE
JOIN pg_attribute AS source_column ON source_column.attrelid = source_table.oid AND source_column.attnum = source_key.attnum
WHERE pg_fk.conname = 'fk_project_template_snapshots_template'`); got != "template_id" {
		t.Fatalf("legacy template foreign key columns after failed upgrade = %q, want template_id", got)
	}
	if got := psqlScalar(t, dbURL, `
SELECT string_agg(source_column.attname, ',' ORDER BY source_key.ordinality)
FROM pg_constraint AS pg_fk
JOIN pg_class AS source_table ON source_table.oid = pg_fk.conrelid
JOIN unnest(pg_fk.conkey) WITH ORDINALITY AS source_key(attnum, ordinality) ON TRUE
JOIN pg_attribute AS source_column ON source_column.attrelid = source_table.oid AND source_column.attnum = source_key.attnum
WHERE pg_fk.conname = 'fk_project_templates_current_snapshot'`); got != "current_snapshot_id" {
		t.Fatalf("legacy current snapshot foreign key columns after failed upgrade = %q, want current_snapshot_id", got)
	}
	if got := psqlScalar(t, dbURL, "SELECT to_regclass('idx_project_templates_id_workspace') IS NULL"); got != "t" {
		t.Fatalf("new index survived failed upgrade = %q, want t", got)
	}
}

func seedPostgresE2EProjectTemplateLegacyPrerequisites(t *testing.T, dbURL string) string {
	t.Helper()
	store, err := storage.Open(dbURL)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := store.LocalWorkspace()
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	project := storage.Project{
		ID: "legacy-source", WorkspaceID: workspace.ID, Slug: "legacy-source", Name: "旧模板来源",
		Description: "", Status: "active", SettingsJSON: "{}", NextTaskSeq: 1, NextSeriesSeq: 1,
		CreatedAt: 100, ModifiedAt: 100,
	}
	if err := store.DB().Create(&project).Error; err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	return workspace.ID
}

func createPostgresE2ELegacyProjectTemplateSchema(t *testing.T, dbURL, templateWorkspaceID, snapshotWorkspaceID string) {
	t.Helper()
	for _, statement := range []string{
		"DROP TABLE project_template_snapshots, project_templates",
		`CREATE TABLE project_templates (
id text PRIMARY KEY,
workspace_id text NOT NULL,
key text NOT NULL,
name text NOT NULL,
description text NOT NULL DEFAULT '',
status text NOT NULL,
current_snapshot_id text,
created_by_actor_type text NOT NULL DEFAULT 'user',
created_by_user_id text,
created_by_token_id text,
created_by_token_name text,
created_by_token_prefix text,
created_at bigint NOT NULL,
modified_at bigint NOT NULL,
archived_at bigint
)`,
		`CREATE TABLE project_template_snapshots (
id text PRIMARY KEY,
workspace_id text NOT NULL,
template_id text NOT NULL,
version bigint NOT NULL,
source_project_id text NOT NULL,
snapshot_json text NOT NULL,
snapshot_hash text NOT NULL,
created_by_actor_type text NOT NULL DEFAULT 'user',
created_by_user_id text,
created_by_token_id text,
created_by_token_name text,
created_by_token_prefix text,
created_at bigint NOT NULL,
CONSTRAINT fk_project_template_snapshots_template FOREIGN KEY (template_id) REFERENCES project_templates(id) ON DELETE RESTRICT
)`,
		"ALTER TABLE project_templates ADD CONSTRAINT fk_project_templates_current_snapshot FOREIGN KEY (current_snapshot_id) REFERENCES project_template_snapshots(id) ON DELETE RESTRICT",
		"CREATE UNIQUE INDEX idx_project_templates_ws_key ON project_templates(workspace_id, key)",
		"CREATE UNIQUE INDEX idx_project_template_snapshots_template_version ON project_template_snapshots(template_id, version)",
		"CREATE UNIQUE INDEX idx_project_template_snapshots_template_snapshot_hash ON project_template_snapshots(template_id, snapshot_hash)",
		"CREATE INDEX idx_project_template_snapshots_ws_template ON project_template_snapshots(workspace_id, template_id)",
		fmt.Sprintf("INSERT INTO project_templates (id, workspace_id, key, name, description, status, current_snapshot_id, created_by_actor_type, created_at, modified_at) VALUES ('legacy-template', '%s', 'legacy', '旧模板', '', 'active', NULL, 'user', 100, 100)", templateWorkspaceID),
		fmt.Sprintf("INSERT INTO project_template_snapshots (id, workspace_id, template_id, version, source_project_id, snapshot_json, snapshot_hash, created_by_actor_type, created_at) VALUES ('legacy-snapshot', '%s', 'legacy-template', 1, 'legacy-source', '{\"schema\":\"fixture/v1\"}', 'legacy-hash', 'user', 100)", snapshotWorkspaceID),
		"UPDATE project_templates SET current_snapshot_id = 'legacy-snapshot' WHERE id = 'legacy-template'",
	} {
		psqlExec(t, dbURL, statement)
	}
}

func assertPostgresProjectTemplateForeignKey(t *testing.T, store *storage.Store, table, columns, targetTable, targetColumns string) {
	t.Helper()
	rows, err := store.DB().Raw(`
SELECT
  string_agg(source_column.attname, ',' ORDER BY source_key.ordinality) AS columns,
  target_table.relname AS target_table,
  string_agg(target_column.attname, ',' ORDER BY source_key.ordinality) AS target_columns,
  pg_fk.confdeltype
FROM pg_constraint AS pg_fk
JOIN pg_class AS source_table ON source_table.oid = pg_fk.conrelid
JOIN pg_namespace AS source_schema ON source_schema.oid = source_table.relnamespace
JOIN pg_class AS target_table ON target_table.oid = pg_fk.confrelid
JOIN unnest(pg_fk.conkey) WITH ORDINALITY AS source_key(attnum, ordinality) ON TRUE
JOIN pg_attribute AS source_column ON source_column.attrelid = source_table.oid AND source_column.attnum = source_key.attnum
JOIN unnest(pg_fk.confkey) WITH ORDINALITY AS target_key(attnum, ordinality) ON target_key.ordinality = source_key.ordinality
JOIN pg_attribute AS target_column ON target_column.attrelid = target_table.oid AND target_column.attnum = target_key.attnum
WHERE pg_fk.contype = 'f'
  AND source_schema.nspname = current_schema()
  AND source_table.relname = ?
GROUP BY pg_fk.oid, target_table.relname, pg_fk.confdeltype`, table).Rows()
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var gotColumns, gotTargetTable, gotTargetColumns, deleteType string
		if err := rows.Scan(&gotColumns, &gotTargetTable, &gotTargetColumns, &deleteType); err != nil {
			t.Fatal(err)
		}
		if gotColumns == columns && gotTargetTable == targetTable && gotTargetColumns == targetColumns {
			if deleteType != "r" {
				t.Fatalf("%s.(%s) ON DELETE code = %q, want r (RESTRICT)", table, columns, deleteType)
			}
			return
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	t.Fatalf("%s.(%s) -> %s.(%s) foreign key missing", table, columns, targetTable, targetColumns)
}

func TestPostgresE2EServerHTTPMCPAndLogs(t *testing.T) {
	adminURL := postgresE2EAdminURL(t)
	dbURL := createPostgresE2EDatabase(t, adminURL)
	bin := buildXuanchu(t)
	dir := t.TempDir()
	configPath, logPath := writeE2ELogConfig(t, dir, "trusted.example")
	token := parseRawToken(t, createTokenJSON(t, bin, "--db-url", dbURL, "postgres-e2e", "*"))

	cmd, baseURL := startXuanchuServer(t, bin, "--config", configPath, "--db-url", dbURL)
	defer stopXuanchuServer(t, cmd)

	if status := httpStatus(t, http.MethodGet, baseURL+"/healthz", nil, nil); status != http.StatusOK {
		t.Fatalf("GET /healthz status = %d, want 200", status)
	}
	me := httpJSON(t, http.MethodGet, baseURL+"/api/v1/me", nil, authHeaders(token))
	if !strings.Contains(toJSONString(t, me["data"]), `"name":"local"`) {
		t.Fatalf("PostgreSQL /me data = %#v", me["data"])
	}

	initBody := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0.1.0"}}}`)
	if status := httpStatus(t, http.MethodPost, baseURL+"/mcp", initBody, map[string]string{"Content-Type": "application/json", "Host": "evil.example"}); status != http.StatusForbidden {
		t.Fatalf("POST /mcp with evil Host status = %d, want 403", status)
	}
	initBody = strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0.1.0"}}}`)
	if status := httpStatus(t, http.MethodPost, baseURL+"/mcp", initBody, map[string]string{"Content-Type": "application/json", "Host": "trusted.example"}); status != http.StatusUnauthorized {
		t.Fatalf("POST /mcp with trusted Host without token status = %d, want 401", status)
	}

	project := httpJSON(t, http.MethodPost, baseURL+"/api/v1/projects", map[string]any{
		"slug": "pge2e",
		"name": "PostgreSQL E2E",
	}, authHeaders(token))
	if nestedMap(t, project, "data")["slug"] != "pge2e" {
		t.Fatalf("PostgreSQL project add response = %#v", project)
	}
	task := httpJSON(t, http.MethodPost, baseURL+"/api/v1/tasks", map[string]any{
		"title":   "postgres http api task",
		"project": "pge2e",
	}, authHeaders(token))
	if nestedMap(t, task, "data")["title"] != "postgres http api task" {
		t.Fatalf("PostgreSQL task add response = %#v", task)
	}

	session, cancel := connectHTTPMCP(t, baseURL, token)
	defer cancel()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "task_add",
		Arguments: map[string]any{
			"title":   "postgres mcp task",
			"project": "pge2e",
		},
	})
	if err != nil {
		t.Fatalf("PostgreSQL HTTP MCP task_add error = %v", err)
	}
	env := mcpStructuredMap(t, result)
	if nestedMap(t, nestedMap(t, env, "data"), "task")["title"] != "postgres mcp task" {
		t.Fatalf("PostgreSQL HTTP MCP structured content = %#v", env)
	}

	seriesData := callMCPToolData(t, session, "task_series_add", map[string]any{
		"project":         "pge2e",
		"title":           "postgres daily inspection",
		"description":     "verify PostgreSQL recurrence persistence",
		"recurrence_rule": "daily",
		"first_due_date":  "2030-01-01",
		"priority":        "H",
		"tags":            []string{"postgres", "recurrence"},
	})
	series := nestedMap(t, seriesData, "series")
	seriesID, _ := series["id"].(string)
	if seriesID == "" || series["status"] != "active" || series["recurrence_rule"] != "daily" {
		t.Fatalf("PostgreSQL task_series_add data = %#v", seriesData)
	}
	firstOccurrence := nestedMap(t, seriesData, "first_occurrence")
	firstRef, _ := firstOccurrence["id"].(string)
	if firstRef == "" || nestedMap(t, firstOccurrence, "recurrence_info")["materialization"] != "projected" {
		t.Fatalf("PostgreSQL projected first occurrence = %#v", firstOccurrence)
	}

	queryData := callMCPToolData(t, session, "task_query", map[string]any{
		"project":         "pge2e",
		"due_after":       "2030-01-01",
		"due_before":      "2030-01-02",
		"occurrence_mode": "expand",
		"task_type":       "occurrence",
	})
	items, ok := queryData["items"].([]any)
	if !ok || queryData["total"] != float64(2) || len(items) != 2 {
		t.Fatalf("PostgreSQL expanded task_query data = %#v", queryData)
	}
	var projected map[string]any
	for _, raw := range items {
		item, itemOK := raw.(map[string]any)
		if itemOK && item["id"] == firstRef {
			projected = item
			break
		}
	}
	if projected == nil || projected["uuid"] != nil || projected["task_slug"] != nil {
		t.Fatalf("PostgreSQL projected first occurrence missing from %#v", items)
	}

	doneData := callMCPToolData(t, session, "task_done", map[string]any{"id": firstRef})
	doneTask := nestedMap(t, doneData, "task")
	taskSlug, _ := doneTask["task_slug"].(string)
	if doneTask["id"] != firstRef || doneTask["status"] != "completed" || taskSlug == "" {
		t.Fatalf("PostgreSQL materialized occurrence = %#v", doneTask)
	}
	if nestedMap(t, doneTask, "recurrence_info")["materialization"] != "materialized" {
		t.Fatalf("PostgreSQL materialized recurrence_info = %#v", doneTask["recurrence_info"])
	}

	aliasTask := nestedMap(t, httpJSON(t, http.MethodGet,
		baseURL+"/api/v1/tasks/"+url.PathEscape(taskSlug)+"?workspace=local",
		nil, authHeaders(token)), "data")
	if aliasTask["id"] != firstRef || aliasTask["task_slug"] != taskSlug || aliasTask["status"] != "completed" {
		t.Fatalf("PostgreSQL task_slug occurrence lookup = %#v", aliasTask)
	}

	modifiedSeries := callMCPToolData(t, session, "task_series_modify", map[string]any{
		"id":          seriesID,
		"description": "updated on PostgreSQL",
		"priority":    "M",
	})
	if modifiedSeries["description"] != "updated on PostgreSQL" || modifiedSeries["priority"] != "M" {
		t.Fatalf("PostgreSQL task_series_modify data = %#v", modifiedSeries)
	}
	occurrences := callMCPToolData(t, session, "task_series_list_occurrences", map[string]any{
		"id": seriesID, "status": "completed",
	})
	completed, ok := occurrences["items"].([]any)
	if !ok || occurrences["total"] != float64(1) || len(completed) != 1 {
		t.Fatalf("PostgreSQL task_series_list_occurrences data = %#v", occurrences)
	}
	stoppedSeries := callMCPToolData(t, session, "task_series_stop", map[string]any{"id": seriesID})
	if stoppedSeries["status"] != "stopped" {
		t.Fatalf("PostgreSQL task_series_stop data = %#v", stoppedSeries)
	}

	stopXuanchuServer(t, cmd)
	assertLogContains(t, logPath,
		"operation=http_request",
		"workspace_id=",
		"workspace_ref=local",
		"operation=mcp_tool_call",
		"tool=task_add",
	)
}
