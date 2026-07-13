package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/app"
)

// createHTTPTestProjectViaService 直接通过 service 创建测试项目（token 无 project:write scope）。
func createHTTPTestProjectViaService(t *testing.T, fixture httpTokenFixture) string {
	t.Helper()
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddProject(app.AddProjectInput{Slug: "ops", Name: "Ops"}); err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	return "ops"
}

func TestTaskSeriesHTTPCreateAndGet(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	createHTTPTestProjectViaService(t, fixture)

	// POST 创建 series。
	body := `{"title":"每日巡检","project":"ops","recurrence_rule":"daily","first_due_date":"2030-01-01"}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/task-series?workspace=local",
		body, map[string]string{"Authorization": "Bearer " + fixture.token})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create series: status=%d body=%s", rr.Code, rr.Body.String())
	}
	var createResp struct {
		Data struct {
			Series struct {
				ID     string `json:"id"`
				Status string `json:"status"`
				Rule   string `json:"recurrence_rule"`
			} `json:"series"`
			FirstOccurrence *struct {
				ID             string `json:"id"`
				RecurrenceInfo *struct {
					Materialization string `json:"materialization"`
				} `json:"recurrence_info"`
			} `json:"first_occurrence"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rr.Body.String())
	}
	if createResp.Data.Series.ID == "" {
		t.Fatal("series ID 为空")
	}
	if createResp.Data.Series.Status != "active" || createResp.Data.Series.Rule != "daily" {
		t.Fatalf("series = %#v", createResp.Data.Series)
	}
	if createResp.Data.FirstOccurrence == nil {
		t.Fatal("first_occurrence 为空")
	}
	if createResp.Data.FirstOccurrence.RecurrenceInfo == nil || createResp.Data.FirstOccurrence.RecurrenceInfo.Materialization != "projected" {
		t.Fatalf("first_occurrence 应为 projected: %#v", createResp.Data.FirstOccurrence.RecurrenceInfo)
	}

	// GET 单个 series。
	rr2 := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/task-series/"+createResp.Data.Series.ID+"?workspace=local",
		map[string]string{"Authorization": "Bearer " + fixture.token})
	if rr2.Code != http.StatusOK {
		t.Fatalf("get series: status=%d body=%s", rr2.Code, rr2.Body.String())
	}
}

func TestTaskSeriesHTTPModifyAppliesClearFields(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	createHTTPTestProjectViaService(t, fixture)
	body := `{"title":"每日巡检","description":"旧说明","project":"ops","recurrence_rule":"daily","first_due_date":"2030-01-01","until_date":"2030-02-01","priority":"H","tags":["ops"]}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/task-series?workspace=local",
		body, map[string]string{"Authorization": "Bearer " + fixture.token})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create series: status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created struct {
		Data struct {
			Series struct {
				ID string `json:"id"`
			} `json:"series"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	rr = requestHTTPBody(t, fixture.server, http.MethodPatch,
		"/api/v1/task-series/"+created.Data.Series.ID+"?workspace=local",
		`{"clear":["description","priority","tags","until"]}`,
		map[string]string{"Authorization": "Bearer " + fixture.token})
	if rr.Code != http.StatusOK {
		t.Fatalf("modify series: status=%d body=%s", rr.Code, rr.Body.String())
	}
	var modified struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &modified); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"description", "priority", "tags", "until"} {
		if _, ok := modified.Data[field]; ok {
			t.Fatalf("clear 后仍返回 %s: %#v", field, modified.Data)
		}
	}
}

func TestTaskSeriesHTTPList(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	createHTTPTestProjectViaService(t, fixture)
	body := `{"title":"每日巡检","project":"ops","recurrence_rule":"daily","first_due_date":"2030-01-01"}`
	requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/task-series?workspace=local",
		body, map[string]string{"Authorization": "Bearer " + fixture.token})

	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/task-series?workspace=local&status=active",
		map[string]string{"Authorization": "Bearer " + fixture.token})
	if rr.Code != http.StatusOK {
		t.Fatalf("list series: status=%d body=%s", rr.Code, rr.Body.String())
	}
	var page struct {
		Data struct {
			Items []map[string]any `json:"items"`
			Total int              `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &page); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if page.Data.Total != 1 {
		t.Fatalf("total = %d want 1", page.Data.Total)
	}
	if len(page.Data.Items) != 1 {
		t.Fatalf("items len = %d want 1", len(page.Data.Items))
	}
}

func TestTaskSeriesHTTPStop(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	createHTTPTestProjectViaService(t, fixture)
	body := `{"title":"每日巡检","project":"ops","recurrence_rule":"daily","first_due_date":"2030-01-01"}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/task-series?workspace=local",
		body, map[string]string{"Authorization": "Bearer " + fixture.token})
	var createResp struct {
		Data struct {
			Series struct {
				ID string `json:"id"`
			} `json:"series"`
		} `json:"data"`
	}
	json.Unmarshal(rr.Body.Bytes(), &createResp)

	// DELETE 停止 series。
	rr2 := requestHTTP(t, fixture.server, http.MethodDelete, "/api/v1/task-series/"+createResp.Data.Series.ID+"?workspace=local",
		map[string]string{"Authorization": "Bearer " + fixture.token})
	if rr2.Code != http.StatusOK {
		t.Fatalf("stop series: status=%d body=%s", rr2.Code, rr2.Body.String())
	}
	var stopResp struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	json.Unmarshal(rr2.Body.Bytes(), &stopResp)
	if stopResp.Data.Status != "stopped" {
		t.Fatalf("status = %q want stopped", stopResp.Data.Status)
	}
}

func TestTaskSeriesHTTPCreateRejectsInvalidRule(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	createHTTPTestProjectViaService(t, fixture)
	body := `{"title":"x","project":"ops","recurrence_rule":"biweekly","first_due_date":"2030-01-01"}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/task-series?workspace=local",
		body, map[string]string{"Authorization": "Bearer " + fixture.token})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("biweekly 应返回 400, got status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "task_series_invalid_rule") {
		t.Fatalf("应包含 task_series_invalid_rule: %s", rr.Body.String())
	}
}

func TestTaskSeriesHTTPOccurrencesList(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	createHTTPTestProjectViaService(t, fixture)
	// first_due 在过去，物化 first occurrence。
	body := `{"title":"每日巡检","project":"ops","recurrence_rule":"daily","first_due_date":"2020-01-01"}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/task-series?workspace=local",
		body, map[string]string{"Authorization": "Bearer " + fixture.token})
	var createResp struct {
		Data struct {
			Series struct {
				ID string `json:"id"`
			} `json:"series"`
		} `json:"data"`
	}
	json.Unmarshal(rr.Body.Bytes(), &createResp)

	rr2 := requestHTTP(t, fixture.server, http.MethodGet,
		"/api/v1/task-series/"+createResp.Data.Series.ID+"/occurrences?workspace=local&status=pending",
		map[string]string{"Authorization": "Bearer " + fixture.token})
	if rr2.Code != http.StatusOK {
		t.Fatalf("list occurrences: status=%d body=%s", rr2.Code, rr2.Body.String())
	}
	var page struct {
		Data struct {
			Items []map[string]any `json:"items"`
			Total int              `json:"total"`
		} `json:"data"`
	}
	json.Unmarshal(rr2.Body.Bytes(), &page)
	if page.Data.Total < 1 {
		t.Fatalf("应有 occurrence: total=%d", page.Data.Total)
	}
}

func TestTaskSeriesHTTPOccurrencesRejectsInvalidRange(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	createHTTPTestProjectViaService(t, fixture)
	body := `{"title":"每日巡检","project":"ops","recurrence_rule":"daily","first_due_date":"2030-01-01"}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/task-series?workspace=local",
		body, map[string]string{"Authorization": "Bearer " + fixture.token})
	var created struct {
		Data struct {
			Series struct {
				ID string `json:"id"`
			} `json:"series"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &created)

	rr = requestHTTP(t, fixture.server, http.MethodGet,
		"/api/v1/task-series/"+created.Data.Series.ID+"/occurrences?workspace=local&due_after=not-a-date",
		map[string]string{"Authorization": "Bearer " + fixture.token})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid range status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestTaskSeriesHTTPOccurrenceSkip(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	createHTTPTestProjectViaService(t, fixture)
	body := `{"title":"每日巡检","project":"ops","recurrence_rule":"daily","first_due_date":"2030-01-01"}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/task-series?workspace=local",
		body, map[string]string{"Authorization": "Bearer " + fixture.token})
	var createResp struct {
		Data struct {
			Series struct {
				ID string `json:"id"`
			} `json:"series"`
			FirstOccurrence *struct {
				ID string `json:"id"`
			} `json:"first_occurrence"`
		} `json:"data"`
	}
	json.Unmarshal(rr.Body.Bytes(), &createResp)
	if createResp.Data.FirstOccurrence == nil {
		t.Fatal("first_occurrence 为空")
	}
	occRef := createResp.Data.FirstOccurrence.ID
	// skip。
	rr2 := requestHTTP(t, fixture.server, http.MethodPost,
		"/api/v1/task-series/"+createResp.Data.Series.ID+"/occurrences/"+occRef+"/skip?workspace=local",
		map[string]string{"Authorization": "Bearer " + fixture.token})
	if rr2.Code != http.StatusOK {
		t.Fatalf("skip: status=%d body=%s", rr2.Code, rr2.Body.String())
	}
	var skipResp struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	json.Unmarshal(rr2.Body.Bytes(), &skipResp)
	if skipResp.Data.Status != "deleted" {
		t.Fatalf("status = %q want deleted", skipResp.Data.Status)
	}
}

func TestTaskHTTPQueryAutoExpandsOccurrencesAndAppliesTaskType(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "task:read", "task:write")
	createHTTPTestProjectViaService(t, fixture)
	headers := map[string]string{"Authorization": "Bearer " + fixture.token}
	body := `{"title":"每日巡检","project":"ops","recurrence_rule":"daily","first_due_date":"2030-01-01"}`
	created := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/task-series?workspace=local", body, headers)
	if created.Code != http.StatusCreated {
		t.Fatalf("create series: status=%d body=%s", created.Code, created.Body.String())
	}

	queryPath := "/api/v1/tasks?workspace=local&project=ops&due_after=2030-01-01&due_before=2030-01-02&task_type=occurrence"
	rr := requestHTTP(t, fixture.server, http.MethodGet, queryPath, headers)
	if rr.Code != http.StatusOK {
		t.Fatalf("query occurrences: status=%d body=%s", rr.Code, rr.Body.String())
	}
	var response struct {
		Data struct {
			Items          []taskOccurrenceJSON `json:"items"`
			Total          int                  `json:"total"`
			OccurrenceMode string               `json:"occurrence_mode"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.OccurrenceMode != "expand" || response.Data.Total == 0 {
		t.Fatalf("query response = %#v want expanded occurrence", response.Data)
	}
	for _, item := range response.Data.Items {
		if item.RecurrenceInfo == nil {
			t.Fatalf("task_type=occurrence 返回普通任务: %#v", item)
		}
	}

	normal := requestHTTP(t, fixture.server, http.MethodGet,
		"/api/v1/tasks?workspace=local&project=ops&due_after=2030-01-01&due_before=2030-01-02&task_type=normal", headers)
	if normal.Code != http.StatusOK {
		t.Fatalf("query normal: status=%d body=%s", normal.Code, normal.Body.String())
	}
	if err := json.Unmarshal(normal.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.Total != 0 {
		t.Fatalf("task_type=normal total=%d want 0", response.Data.Total)
	}

	missingRange := requestHTTP(t, fixture.server, http.MethodGet,
		"/api/v1/tasks?workspace=local&project=ops&occurrence_mode=expand&due_after=2030-01-01", headers)
	assertHTTPErrorCode(t, missingRange, http.StatusBadRequest, "task_occurrence_range_required")
}
