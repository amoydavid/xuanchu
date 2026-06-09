package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"github.com/google/uuid"
)

func TestHTTPNotificationSinkLifecycle(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "notification:write", "notification:read")
	auth := map[string]string{"Authorization": "Bearer " + fixture.token, "Content-Type": "application/json"}

	body := `{"name":"openclaw","type":"webhook","endpoint_mode":"static_url","url":"https://example.com/notify","secret":"hunter2"}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/notification-sinks", body, auth)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "hunter2") {
		t.Fatalf("response leaks secret: %s", rr.Body.String())
	}
	var resp struct {
		Data struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.ID == "" || resp.Data.Name != "openclaw" || resp.Data.Type != "webhook" {
		t.Fatalf("response = %#v", resp.Data)
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
