package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestOrdinaryTaskImportUsesDedicatedVersionedContract(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	createHTTPTestProjectViaService(t, fixture)
	body := `{
		"schema":"xuanchu.task-import/v1",
		"tasks":[{
			"uuid":"11111111-1111-4111-8111-111111111111",
			"title":"普通导入任务",
			"status":"pending",
			"entry":"2030-01-01T00:00:00Z",
			"modified":"2030-01-01T00:00:00Z",
			"project":"ops"
		}]
	}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost,
		"/api/v1/task-imports?workspace=local&project=ops", body,
		map[string]string{"Authorization": "Bearer " + fixture.token})
	if rr.Code != http.StatusOK {
		t.Fatalf("ordinary import: status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Body.String(); got == "" || !strings.Contains(got, `"imported":1`) {
		t.Fatalf("ordinary import response = %s", got)
	}
}

func TestOrdinaryTaskImportRejectsOccurrenceIdentity(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	createHTTPTestProjectViaService(t, fixture)
	body := `{
		"schema":"xuanchu.task-import/v1",
		"tasks":[{
			"uuid":"11111111-1111-4111-8111-111111111111",
			"title":"伪造循环实例",
			"status":"pending",
			"entry":"2030-01-01T00:00:00Z",
			"modified":"2030-01-01T00:00:00Z",
			"project":"ops",
			"series_id":"22222222-2222-4222-8222-222222222222",
			"recurrence_at":"2030-01-01T23:59:59Z"
		}]
	}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost,
		"/api/v1/task-imports?workspace=local&project=ops", body,
		map[string]string{"Authorization": "Bearer " + fixture.token})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("occurrence import: status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "task_import_occurrence_not_allowed") {
		t.Fatalf("unexpected error: %s", rr.Body.String())
	}
}

// TestExportTaskBundleIncludesAttachmentWarnings 验证导出的 bundle 在 description
// 包含 ref://attachment/{id} 时返回非阻断 warnings（spec §20 / 计划 4 Task 11）。
func TestExportTaskBundleIncludesAttachmentWarnings(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	// 创建一个带 attachment 引用的 task。
	markdown := "![](ref://attachment/40af0185-316f-42bb-b52b-545d21f6f012)"
	body := `{"title":"ref-task","description":` + jsonStringQuote(markdown) + `}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/tasks?workspace=local", body, authHeader(fixture.token))
	if rr.Code != http.StatusCreated {
		t.Fatalf("create task: %d %s", rr.Code, rr.Body.String())
	}
	// 导出 bundle。
	rr = requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/export?workspace=local", authHeader(fixture.token))
	if rr.Code != http.StatusOK {
		t.Fatalf("export: %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "binary_not_included") {
		t.Fatalf("export missing warnings: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "40af0185-316f-42bb-b52b-545d21f6f012") {
		t.Fatalf("export missing attachment id in warnings: %s", rr.Body.String())
	}
}

// jsonStringQuote 把字符串转为 JSON 字符串字面量。
func jsonStringQuote(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}
