package notificationruntime

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/runtimeutil"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
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

func mustCreateRuntimeTask(t *testing.T, store *storage.Store, wsID, taskUUID string) {
	t.Helper()
	if _, err := storage.NewTaskRepository(store.DB()).Create(task.Task{
		UUID:        taskUUID,
		WorkspaceID: wsID,
		Description: "runtime task",
		Status:      task.StatusPending,
		Entry:       100,
		Modified:    100,
	}); err != nil {
		t.Fatalf("Create task %s error = %v", taskUUID, err)
	}
}

func mustCreateRuntimeSink(t *testing.T, store *storage.Store, sink storage.NotificationSink) {
	t.Helper()
	if err := storage.NewNotificationSinkRepository(store.DB()).Create(sink); err != nil {
		t.Fatal(err)
	}
}

func mustEnqueueRuntimeDelivery(t *testing.T, store *storage.Store, delivery storage.NotificationDelivery) {
	t.Helper()
	mustCreateRuntimeTask(t, store, delivery.WorkspaceID, delivery.TaskUUID)
	if err := storage.NewNotificationDeliveryRepository(store.DB()).Enqueue([]storage.NotificationDelivery{delivery}); err != nil {
		t.Fatal(err)
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
	mustCreateRuntimeSink(t, store, sink)
	delivery := makeRuntimeDelivery(ws.ID, sink.ID, server.URL)
	mustEnqueueRuntimeDelivery(t, store, delivery)

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
	mustCreateRuntimeSink(t, store, sink)
	delivery := makeRuntimeDelivery(ws.ID, sink.ID, server.URL)
	delivery.Status = storage.DeliveryStatusRetryWait
	delivery.NextAttemptAt = int64Ptr(900)
	mustEnqueueRuntimeDelivery(t, store, delivery)

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
	mustCreateRuntimeSink(t, store, sink)
	delivery := makeRuntimeDelivery(ws.ID, sink.ID, server.URL)
	mustEnqueueRuntimeDelivery(t, store, delivery)
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
	mustCreateRuntimeSink(t, store, sink)
	delivery := makeRuntimeDelivery(ws.ID, sink.ID, "https://example.com/notify")
	mustEnqueueRuntimeDelivery(t, store, delivery)
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

func TestDispatcherClaimLimitUsesConcurrencyAndPrefetch(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := newNotificationRuntimeStore(t)
	ws, _ := store.LocalWorkspace()
	sink := makeRuntimeSink(ws.ID, server.URL)
	mustCreateRuntimeSink(t, store, sink)
	for i := 0; i < 5; i++ {
		delivery := makeRuntimeDelivery(ws.ID, sink.ID, server.URL)
		delivery.TaskUUID = "task-claim-" + strconv.Itoa(i)
		delivery.CreatedAt = int64(100 + i)
		delivery.ModifiedAt = delivery.CreatedAt
		mustEnqueueRuntimeDelivery(t, store, delivery)
	}

	dispatcher := NewDispatcher(DispatcherOptions{
		Store:          store,
		Clock:          testClock{now: 1000},
		Client:         server.Client(),
		Resolver:       app.DefaultHookResolver(),
		BatchSize:      10,
		MaxConcurrency: 2,
		PrefetchFactor: 2,
	})
	if err := dispatcher.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	if requests.Load() != 4 {
		t.Fatalf("requests = %d, want 4", requests.Load())
	}
	var queued int64
	if err := store.DB().Model(&storage.NotificationDelivery{}).Where("status = ?", storage.DeliveryStatusQueued).Count(&queued).Error; err != nil {
		t.Fatal(err)
	}
	if queued != 1 {
		t.Fatalf("queued = %d, want 1", queued)
	}
}

func TestDispatcherRunOnceUsesWorkerPoolAndWaits(t *testing.T) {
	var active atomic.Int64
	var maxActive atomic.Int64
	release := make(chan struct{})
	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := active.Add(1)
		defer active.Add(-1)
		for {
			previous := maxActive.Load()
			if current <= previous || maxActive.CompareAndSwap(previous, current) {
				break
			}
		}
		if current == 2 {
			once.Do(func() { close(release) })
		}
		select {
		case <-release:
		case <-time.After(300 * time.Millisecond):
			once.Do(func() { close(release) })
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := newNotificationRuntimeStore(t)
	ws, _ := store.LocalWorkspace()
	sink := makeRuntimeSink(ws.ID, server.URL)
	mustCreateRuntimeSink(t, store, sink)
	for i := 0; i < 2; i++ {
		delivery := makeRuntimeDelivery(ws.ID, sink.ID, server.URL)
		delivery.TaskUUID = "task-worker-" + strconv.Itoa(i)
		delivery.CreatedAt = int64(100 + i)
		delivery.ModifiedAt = delivery.CreatedAt
		mustEnqueueRuntimeDelivery(t, store, delivery)
	}

	dispatcher := NewDispatcher(DispatcherOptions{
		Store:          store,
		Clock:          testClock{now: 1000},
		Client:         server.Client(),
		Resolver:       app.DefaultHookResolver(),
		MaxConcurrency: 2,
	})
	if err := dispatcher.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if maxActive.Load() != 2 {
		t.Fatalf("max active requests = %d, want 2", maxActive.Load())
	}
	var succeeded int64
	if err := store.DB().Model(&storage.NotificationDelivery{}).Where("status = ?", storage.DeliveryStatusSucceeded).Count(&succeeded).Error; err != nil {
		t.Fatal(err)
	}
	if succeeded != 2 {
		t.Fatalf("succeeded = %d, want 2", succeeded)
	}
}

func TestDispatcherAttemptHeaderUsesClaimedAttemptCount(t *testing.T) {
	var attemptHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptHeader = r.Header.Get("X-Xuanchu-Attempt")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := newNotificationRuntimeStore(t)
	ws, _ := store.LocalWorkspace()
	sink := makeRuntimeSink(ws.ID, server.URL)
	mustCreateRuntimeSink(t, store, sink)
	delivery := makeRuntimeDelivery(ws.ID, sink.ID, server.URL)
	delivery.AttemptCount = 1
	delivery.Status = storage.DeliveryStatusRetryWait
	delivery.NextAttemptAt = int64Ptr(900)
	mustEnqueueRuntimeDelivery(t, store, delivery)

	dispatcher := NewDispatcher(DispatcherOptions{Store: store, Clock: testClock{now: 1000}, Client: server.Client(), Resolver: app.DefaultHookResolver()})
	if err := dispatcher.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if attemptHeader != "2" {
		t.Fatalf("X-Xuanchu-Attempt = %q, want 2", attemptHeader)
	}
}

func TestDispatcherSinkLimiterRequeuesWithoutHTTP(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := newNotificationRuntimeStore(t)
	ws, _ := store.LocalWorkspace()
	sink := makeRuntimeSink(ws.ID, server.URL)
	sink.MaxConcurrency = 1
	mustCreateRuntimeSink(t, store, sink)
	delivery := makeRuntimeDelivery(ws.ID, sink.ID, server.URL)
	mustEnqueueRuntimeDelivery(t, store, delivery)

	limiter := runtimeutil.NewSinkLimiter()
	if !limiter.TryAcquire(sink.ID, 1) {
		t.Fatal("pre-acquire sink token failed")
	}
	defer limiter.Release(sink.ID)

	dispatcher := NewDispatcher(DispatcherOptions{
		Store:       store,
		Clock:       testClock{now: 1000},
		Client:      server.Client(),
		Resolver:    app.DefaultHookResolver(),
		SinkLimiter: limiter,
	})
	if err := dispatcher.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("requests = %d, want 0", requests.Load())
	}
	got, err := storage.NewNotificationDeliveryRepository(store.DB()).GetByID(delivery.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != storage.DeliveryStatusQueued {
		t.Fatalf("status = %q, want queued", got.Status)
	}
	if got.ClaimExpiresAt != nil {
		t.Fatalf("claim_expires_at = %#v, want nil", got.ClaimExpiresAt)
	}
}

func TestDispatcherDoesNotLeaveSinkLimitedDeliveryDelivering(t *testing.T) {
	store := newNotificationRuntimeStore(t)
	ws, _ := store.LocalWorkspace()
	sink := makeRuntimeSink(ws.ID, "https://example.com/notify")
	sink.MaxConcurrency = 1
	mustCreateRuntimeSink(t, store, sink)
	delivery := makeRuntimeDelivery(ws.ID, sink.ID, "https://example.com/notify")
	mustEnqueueRuntimeDelivery(t, store, delivery)

	limiter := runtimeutil.NewSinkLimiter()
	if !limiter.TryAcquire(sink.ID, 1) {
		t.Fatal("pre-acquire sink token failed")
	}
	defer limiter.Release(sink.ID)

	dispatcher := NewDispatcher(DispatcherOptions{Store: store, Clock: testClock{now: 1000}, SinkLimiter: limiter})
	if err := dispatcher.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	var delivering int64
	if err := store.DB().Model(&storage.NotificationDelivery{}).Where("status = ?", storage.DeliveryStatusDelivering).Count(&delivering).Error; err != nil {
		t.Fatal(err)
	}
	if delivering != 0 {
		t.Fatalf("delivering = %d, want 0", delivering)
	}
}

func TestDispatcherStaleRecoveryClaimsRecoveredDelivery(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := newNotificationRuntimeStore(t)
	ws, _ := store.LocalWorkspace()
	sink := makeRuntimeSink(ws.ID, server.URL)
	mustCreateRuntimeSink(t, store, sink)
	delivery := makeRuntimeDelivery(ws.ID, sink.ID, server.URL)
	delivery.Status = storage.DeliveryStatusDelivering
	delivery.ClaimExpiresAt = int64Ptr(500)
	delivery.AttemptCount = 1
	mustEnqueueRuntimeDelivery(t, store, delivery)

	dispatcher := NewDispatcher(DispatcherOptions{Store: store, Clock: testClock{now: 1000}, Client: server.Client(), Resolver: app.DefaultHookResolver()})
	if err := dispatcher.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if requests.Load() != 1 {
		t.Fatalf("requests = %d, want 1", requests.Load())
	}
	got, err := storage.NewNotificationDeliveryRepository(store.DB()).GetByID(delivery.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != storage.DeliveryStatusSucceeded {
		t.Fatalf("status = %q, want succeeded", got.Status)
	}
}

func TestDispatcherFutureDeliveringIsNotRecovered(t *testing.T) {
	store := newNotificationRuntimeStore(t)
	ws, _ := store.LocalWorkspace()
	sink := makeRuntimeSink(ws.ID, "https://example.com/notify")
	mustCreateRuntimeSink(t, store, sink)
	delivery := makeRuntimeDelivery(ws.ID, sink.ID, "https://example.com/notify")
	delivery.Status = storage.DeliveryStatusDelivering
	delivery.ClaimExpiresAt = int64Ptr(1500)
	delivery.AttemptCount = 1
	mustEnqueueRuntimeDelivery(t, store, delivery)

	dispatcher := NewDispatcher(DispatcherOptions{Store: store, Clock: testClock{now: 1000}})
	if err := dispatcher.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	got, err := storage.NewNotificationDeliveryRepository(store.DB()).GetByID(delivery.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != storage.DeliveryStatusDelivering {
		t.Fatalf("status = %q, want delivering", got.Status)
	}
	if got.ClaimExpiresAt == nil || *got.ClaimExpiresAt != 1500 {
		t.Fatalf("claim_expires_at = %#v, want 1500", got.ClaimExpiresAt)
	}
}

func TestDispatcherRetryWaitFutureIsNotClaimed(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := newNotificationRuntimeStore(t)
	ws, _ := store.LocalWorkspace()
	sink := makeRuntimeSink(ws.ID, server.URL)
	mustCreateRuntimeSink(t, store, sink)
	delivery := makeRuntimeDelivery(ws.ID, sink.ID, server.URL)
	delivery.Status = storage.DeliveryStatusRetryWait
	delivery.NextAttemptAt = int64Ptr(1500)
	mustEnqueueRuntimeDelivery(t, store, delivery)

	dispatcher := NewDispatcher(DispatcherOptions{Store: store, Clock: testClock{now: 1000}, Client: server.Client(), Resolver: app.DefaultHookResolver()})
	if err := dispatcher.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("requests = %d, want 0", requests.Load())
	}
	got, err := storage.NewNotificationDeliveryRepository(store.DB()).GetByID(delivery.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != storage.DeliveryStatusRetryWait {
		t.Fatalf("status = %q, want retry_wait", got.Status)
	}
	if got.AttemptCount != 0 {
		t.Fatalf("attempt_count = %d, want 0", got.AttemptCount)
	}
}
