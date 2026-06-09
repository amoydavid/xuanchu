package notificationruntime

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"github.com/google/uuid"
)

type testClock struct{ now int64 }

func (c testClock) Unix() int64              { return c.now }
func (c testClock) Location() *time.Location { return time.UTC }

func newNotificationRuntimeStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func makeRuntimeSink(wsID, endpoint string) storage.NotificationSink {
	enabled := true
	return storage.NotificationSink{
		ID:                  uuid.NewString(),
		WorkspaceID:         wsID,
		Name:                "sink",
		Type:                "http_template",
		EndpointMode:        "static_url",
		URL:                 endpoint,
		AllowedHostsJSON:    `[]`,
		HTTPMethod:          http.MethodPost,
		HeaderTemplatesJSON: `[]`,
		SecretRefsJSON:      `{}`,
		Enabled:             &enabled,
		TimeoutSeconds:      10,
		MaxAttempts:         5,
		CreatedBy:           "user-1",
		CreatedAt:           100,
		ModifiedAt:          100,
	}
}

func makeRuntimeDelivery(wsID, sinkID string, endpoint string) storage.NotificationDelivery {
	return storage.NotificationDelivery{
		ID:                  uuid.NewString(),
		WorkspaceID:         wsID,
		RuleID:              "rule-1",
		SinkID:              sinkID,
		TaskUUID:            "task-1",
		RecipientUserID:     "user-1",
		EventID:             uuid.NewString(),
		EventType:           "task.due_soon",
		DedupeKey:           uuid.NewString(),
		ResolvedURL:         endpoint,
		RenderedMethod:      http.MethodPost,
		RenderedHeadersJSON: `{"X-Template":["frozen"]}`,
		RenderedBody:        `{"frozen":true}`,
		RenderedContentType: "application/json",
		PayloadJSON:         `{"event_type":"task.due_soon"}`,
		Status:              storage.DeliveryStatusQueued,
		CreatedAt:           100,
		ModifiedAt:          100,
	}
}

func TestNotificationDispatcherHTTPTemplateUsesRenderedRequestSnapshot(t *testing.T) {
	var receivedBody []byte
	var receivedHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		receivedHeader = r.Header.Get("X-Template")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := newNotificationRuntimeStore(t)
	ws, _ := store.LocalWorkspace()
	sink := makeRuntimeSink(ws.ID, server.URL)
	if err := storage.NewNotificationSinkRepository(store.DB()).Create(sink); err != nil {
		t.Fatal(err)
	}
	delivery := makeRuntimeDelivery(ws.ID, sink.ID, server.URL)
	if err := storage.NewNotificationDeliveryRepository(store.DB()).Enqueue([]storage.NotificationDelivery{delivery}); err != nil {
		t.Fatal(err)
	}

	dispatcher := NewDispatcher(DispatcherOptions{Store: store, Clock: testClock{now: 1000}, Client: server.Client(), Resolver: app.DefaultHookResolver()})
	if err := dispatcher.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if string(receivedBody) != `{"frozen":true}` {
		t.Fatalf("body = %s", receivedBody)
	}
	if receivedHeader != "frozen" {
		t.Fatalf("X-Template = %q", receivedHeader)
	}
	got, _ := storage.NewNotificationDeliveryRepository(store.DB()).GetByID(delivery.ID)
	if got.Status != storage.DeliveryStatusSucceeded {
		t.Fatalf("status = %q, want succeeded", got.Status)
	}
}

func TestNotificationDispatcherDoesNotUseCurrentSinkTemplateOnRetry(t *testing.T) {
	var receivedBodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBodies = append(receivedBodies, string(body))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := newNotificationRuntimeStore(t)
	ws, _ := store.LocalWorkspace()
	sink := makeRuntimeSink(ws.ID, server.URL)
	sink.BodyTemplate = `{"current":true}`
	if err := storage.NewNotificationSinkRepository(store.DB()).Create(sink); err != nil {
		t.Fatal(err)
	}
	delivery := makeRuntimeDelivery(ws.ID, sink.ID, server.URL)
	delivery.Status = storage.DeliveryStatusRetryWait
	delivery.NextAttemptAt = int64Ptr(900)
	if err := storage.NewNotificationDeliveryRepository(store.DB()).Enqueue([]storage.NotificationDelivery{delivery}); err != nil {
		t.Fatal(err)
	}

	dispatcher := NewDispatcher(DispatcherOptions{Store: store, Clock: testClock{now: 1000}, Client: server.Client(), Resolver: app.DefaultHookResolver()})
	if err := dispatcher.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if len(receivedBodies) != 1 || receivedBodies[0] != `{"frozen":true}` {
		t.Fatalf("received bodies = %#v", receivedBodies)
	}
}

func TestNotificationDispatcherHTTP500Retries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	store := newNotificationRuntimeStore(t)
	ws, _ := store.LocalWorkspace()
	sink := makeRuntimeSink(ws.ID, server.URL)
	if err := storage.NewNotificationSinkRepository(store.DB()).Create(sink); err != nil {
		t.Fatal(err)
	}
	delivery := makeRuntimeDelivery(ws.ID, sink.ID, server.URL)
	if err := storage.NewNotificationDeliveryRepository(store.DB()).Enqueue([]storage.NotificationDelivery{delivery}); err != nil {
		t.Fatal(err)
	}
	dispatcher := NewDispatcher(DispatcherOptions{Store: store, Clock: testClock{now: 1000}, Client: server.Client(), Resolver: app.DefaultHookResolver(), RetryBaseDelay: time.Second})
	if err := dispatcher.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	got, _ := storage.NewNotificationDeliveryRepository(store.DB()).GetByID(delivery.ID)
	if got.Status != storage.DeliveryStatusRetryWait {
		t.Fatalf("status = %q, want retry_wait", got.Status)
	}
}

func TestNotificationDispatcherDisabledSinkSkipped(t *testing.T) {
	store := newNotificationRuntimeStore(t)
	ws, _ := store.LocalWorkspace()
	sink := makeRuntimeSink(ws.ID, "https://example.com/notify")
	sink.Enabled = boolPtr(false)
	if err := storage.NewNotificationSinkRepository(store.DB()).Create(sink); err != nil {
		t.Fatal(err)
	}
	delivery := makeRuntimeDelivery(ws.ID, sink.ID, "https://example.com/notify")
	if err := storage.NewNotificationDeliveryRepository(store.DB()).Enqueue([]storage.NotificationDelivery{delivery}); err != nil {
		t.Fatal(err)
	}
	dispatcher := NewDispatcher(DispatcherOptions{Store: store, Clock: testClock{now: 1000}})
	if err := dispatcher.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	got, _ := storage.NewNotificationDeliveryRepository(store.DB()).GetByID(delivery.ID)
	if got.Status != storage.DeliveryStatusDisabledSkipped {
		t.Fatalf("status = %q, want disabled_skipped", got.Status)
	}
}

func int64Ptr(v int64) *int64 { return &v }
func boolPtr(v bool) *bool    { return &v }

func TestNotificationHeadersJSONShape(t *testing.T) {
	headers := makeBaseHeaders(storage.NotificationDelivery{ID: "d1", AttemptCount: 1, RenderedHeadersJSON: `{"X":["Y"]}`}, storage.NotificationSink{}, []byte("{}"), 1000, "dev")
	data, err := json.Marshal(headers)
	if err != nil || len(data) == 0 {
		t.Fatalf("headers marshal = %s, %v", data, err)
	}
}

