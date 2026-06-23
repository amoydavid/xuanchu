package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"github.com/google/uuid"
)

func TestHTTPNotificationSinkLifecycle(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "notification:write", "notification:read")
	auth := map[string]string{"Authorization": "Bearer " + fixture.token, "Content-Type": "application/json"}

	body := `{"name":"openclaw","type":"webhook","endpoint_mode":"static_url","url":"https://example.com/notify","secret":"hunter2","max_concurrency":3}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/notification-sinks", body, auth)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "hunter2") {
		t.Fatalf("response leaks secret: %s", rr.Body.String())
	}
	var resp struct {
		Data struct {
			ID             string `json:"id"`
			Name           string `json:"name"`
			Type           string `json:"type"`
			MaxConcurrency int    `json:"max_concurrency"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.ID == "" || resp.Data.Name != "openclaw" || resp.Data.Type != "webhook" || resp.Data.MaxConcurrency != 3 {
		t.Fatalf("response = %#v", resp.Data)
	}

	patch := `{"max_concurrency":5}`
	rr = requestHTTPBody(t, fixture.server, http.MethodPatch, "/api/v1/notification-sinks/"+resp.Data.ID, patch, auth)
	if rr.Code != http.StatusOK {
		t.Fatalf("modify status = %d body=%s", rr.Code, rr.Body.String())
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.MaxConcurrency != 5 {
		t.Fatalf("modified max_concurrency = %d, want 5; body=%s", resp.Data.MaxConcurrency, rr.Body.String())
	}

	rr = requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/notification-sinks/"+resp.Data.ID+"/disable", "", auth)
	if rr.Code != http.StatusOK {
		t.Fatalf("disable status = %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHTTPReminderRuleLifecycle(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "notification:write", "notification:read", "reminder:write", "reminder:read")
	auth := map[string]string{"Authorization": "Bearer " + fixture.token, "Content-Type": "application/json"}
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	sink, err := svc.AddNotificationSink(app.NotificationSinkAddInput{Name: "openclaw", Type: "webhook", EndpointMode: "static_url", URL: "https://example.com/notify"})
	if err != nil {
		t.Fatal(err)
	}

	body := `{"name":"due-before","trigger_type":"due_before","offset_seconds":14400,"audience_type":"assignees","sink_ref":"` + sink.ID + `"}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/reminder-rules", body, auth)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data struct {
			ID          string `json:"id"`
			TriggerType string `json:"trigger_type"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.ID == "" || resp.Data.TriggerType != "due_before" {
		t.Fatalf("response = %#v", resp.Data)
	}
}

func TestHTTPReminderRuleScheduleFilterLifecycle(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "notification:write", "notification:read", "reminder:write", "reminder:read")
	auth := map[string]string{"Authorization": "Bearer " + fixture.token, "Content-Type": "application/json"}
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	sink, err := svc.AddNotificationSink(app.NotificationSinkAddInput{Name: "openclaw", Type: "webhook", EndpointMode: "static_url", URL: "https://example.com/notify"})
	if err != nil {
		t.Fatal(err)
	}

	filter := "end.isnull and start.isnull and due.after:now and due.before:now+24h"
	body := `{"name":"due-soon-24h","schedule_type":"daily_at","schedule_value":"08:50","filter_source":` + strconv.Quote(filter) + `,"audience_type":"assignees","sink_ref":"` + sink.ID + `"}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/reminder-rules", body, auth)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data struct {
			ID            string `json:"id"`
			ScheduleType  string `json:"schedule_type"`
			ScheduleValue string `json:"schedule_value"`
			FilterSource  string `json:"filter_source"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.ID == "" || resp.Data.ScheduleType != "daily_at" || resp.Data.ScheduleValue != "08:50" || resp.Data.FilterSource != filter {
		t.Fatalf("response = %#v", resp.Data)
	}
}

func TestHTTPEventNotificationRuleLifecycle(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "notification:write", "notification:read")
	auth := map[string]string{"Authorization": "Bearer " + fixture.token, "Content-Type": "application/json"}

	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	sink, err := svc.AddNotificationSink(app.NotificationSinkAddInput{Name: "openclaw", Type: "webhook", EndpointMode: "static_url", URL: "https://example.com/notify"})
	if err != nil {
		t.Fatal(err)
	}

	body := `{"name":"task-unblocked-openclaw","event_type":"task.unblocked","filter_source":"status:pending","audience_type":"assignees","sink":"` + sink.ID + `","template_subject":"任务已解除阻塞","template_body":"{{task.title}}"}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/notification-rules", body, auth)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", rr.Code, rr.Body.String())
	}
	var created struct {
		Data struct {
			ID              string `json:"id"`
			Name            string `json:"name"`
			EventType       string `json:"event_type"`
			FilterSource    string `json:"filter_source"`
			AudienceType    string `json:"audience_type"`
			SinkID          string `json:"sink_id"`
			TemplateSubject string `json:"template_subject"`
			Enabled         bool   `json:"enabled"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Data.ID == "" || created.Data.EventType != "task.unblocked" || created.Data.AudienceType != "assignees" || created.Data.SinkID != sink.ID || !created.Data.Enabled {
		t.Fatalf("created rule = %#v", created.Data)
	}

	rr = requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/notification-rules", auth)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "task-unblocked-openclaw") {
		t.Fatalf("list status = %d body=%s", rr.Code, rr.Body.String())
	}

	patch := `{"name":"task-unblocked-renamed","audience_type":"actor"}`
	rr = requestHTTPBody(t, fixture.server, http.MethodPatch, "/api/v1/notification-rules/"+created.Data.ID, patch, auth)
	if rr.Code != http.StatusOK {
		t.Fatalf("modify status = %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"audience_type":"actor"`) {
		t.Fatalf("modify body=%s", rr.Body.String())
	}

	rr = requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/notification-rules/"+created.Data.ID+"/disable", "", auth)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"enabled":false`) {
		t.Fatalf("disable status = %d body=%s", rr.Code, rr.Body.String())
	}
	rr = requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/notification-rules/"+created.Data.ID+"/enable", "", auth)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"enabled":true`) {
		t.Fatalf("enable status = %d body=%s", rr.Code, rr.Body.String())
	}
	rr = requestHTTPBody(t, fixture.server, http.MethodDelete, "/api/v1/notification-rules/"+created.Data.ID, "", auth)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d body=%s", rr.Code, rr.Body.String())
	}
	rr = requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/notification-rules/"+created.Data.ID, auth)
	assertHTTPErrorCode(t, rr, http.StatusNotFound, "notification_rule_not_found")
}

func TestHTTPEventNotificationRuleRejectsURL(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "notification:write")
	auth := map[string]string{"Authorization": "Bearer " + fixture.token, "Content-Type": "application/json"}

	body := `{"name":"bad-url","event_type":"task.unblocked","audience_type":"actor","url":"https://example.com/notify"}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/notification-rules", body, auth)
	assertHTTPErrorCode(t, rr, http.StatusBadRequest, "notification_rule_url_not_supported")

	patch := `{"url":"https://example.com/notify"}`
	rr = requestHTTPBody(t, fixture.server, http.MethodPatch, "/api/v1/notification-rules/missing", patch, auth)
	assertHTTPErrorCode(t, rr, http.StatusBadRequest, "notification_rule_url_not_supported")

	body = `{"name":"bad-endpoint-url","event_type":"task.unblocked","audience_type":"actor","endpoint_url":"https://example.com/notify"}`
	rr = requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/notification-rules", body, auth)
	assertHTTPErrorCode(t, rr, http.StatusBadRequest, "notification_rule_url_not_supported")

	patch = `{"endpoint_url":"https://example.com/notify"}`
	rr = requestHTTPBody(t, fixture.server, http.MethodPatch, "/api/v1/notification-rules/missing", patch, auth)
	assertHTTPErrorCode(t, rr, http.StatusBadRequest, "notification_rule_url_not_supported")
}

func TestHTTPEventNotificationRuleRejectsCrossWorkspaceSink(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "notification:write")
	auth := map[string]string{"Authorization": "Bearer " + fixture.token, "Content-Type": "application/json"}

	otherWS := storage.Workspace{ID: uuid.NewString(), Slug: "other", Name: "Other", CreatedAt: 1000, ModifiedAt: 1000}
	if _, err := storage.NewWorkspaceRepository(fixture.server.store.DB()).Create(otherWS); err != nil {
		t.Fatal(err)
	}
	enabled := true
	sink := storage.NotificationSink{
		ID:                  uuid.NewString(),
		WorkspaceID:         otherWS.ID,
		Name:                "foreign",
		Type:                "webhook",
		EndpointMode:        "static_url",
		URL:                 "https://example.com/notify",
		AllowedHostsJSON:    `["example.com"]`,
		HTTPMethod:          "POST",
		HeaderTemplatesJSON: `[]`,
		SecretRefsJSON:      `{}`,
		Enabled:             &enabled,
		TimeoutSeconds:      10,
		MaxAttempts:         5,
		CreatedBy:           getFirstUserID(t, fixture.server.store),
		CreatedAt:           1000,
		ModifiedAt:          1000,
	}
	if err := storage.NewNotificationSinkRepository(fixture.server.store.DB()).Create(sink); err != nil {
		t.Fatal(err)
	}

	body := `{"name":"foreign-sink","event_type":"task.unblocked","audience_type":"actor","sink":"` + sink.ID + `"}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/notification-rules", body, auth)
	assertHTTPErrorCode(t, rr, http.StatusNotFound, "notification_sink_not_found")
}

func TestHTTPNotificationDeliveryReplay(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "notification:write", "notification:read")
	auth := map[string]string{"Authorization": "Bearer " + fixture.token, "Content-Type": "application/json"}
	svc, err := app.NewService(app.ServiceOptions{Store: fixture.server.store})
	if err != nil {
		t.Fatal(err)
	}
	sink, err := svc.AddNotificationSink(app.NotificationSinkAddInput{Name: "openclaw", Type: "webhook", EndpointMode: "static_url", URL: "https://example.com/notify"})
	if err != nil {
		t.Fatal(err)
	}
	delivery := storage.NotificationDelivery{
		ID:                  uuid.NewString(),
		WorkspaceID:         getFirstWorkspaceID(t, fixture.server.store),
		RuleID:              "rule-1",
		SinkID:              sink.ID,
		TaskUUID:            "task-1",
		RecipientUserID:     getFirstUserID(t, fixture.server.store),
		EventID:             uuid.NewString(),
		EventType:           "task.overdue",
		DedupeKey:           uuid.NewString(),
		ResolvedURL:         "https://example.com/notify",
		RenderedMethod:      "POST",
		RenderedHeadersJSON: `{"Authorization":["Bearer secret-token"]}`,
		RenderedBody:        `{"token":"secret-token"}`,
		RenderedContentType: "application/json",
		PayloadJSON:         `{"event_type":"task.overdue"}`,
		Status:              storage.DeliveryStatusDeadLettered,
		CreatedAt:           1000,
		ModifiedAt:          1000,
	}
	if err := storage.NewNotificationDeliveryRepository(fixture.server.store.DB()).Enqueue([]storage.NotificationDelivery{delivery}); err != nil {
		t.Fatal(err)
	}

	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/notification-deliveries/"+delivery.ID+"/replay", "", auth)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "secret-token") {
		t.Fatalf("response leaks secret: %s", rr.Body.String())
	}
}
