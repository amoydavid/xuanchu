package notificationruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/logging"
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

func TestNotificationDispatcherWritesOperationLog(t *testing.T) {
	var logBuf bytes.Buffer
	logger, closeLogger, err := logging.Setup(logging.LogConfig{Format: "text"}, &logBuf)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closeLogger() })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := newNotificationRuntimeStore(t)
	ws, _ := store.LocalWorkspace()
	sink := makeRuntimeSink(ws.ID, server.URL)
	mustCreateRuntimeSink(t, store, sink)
	delivery := makeRuntimeDelivery(ws.ID, sink.ID, server.URL)
	mustEnqueueRuntimeDelivery(t, store, delivery)

	dispatcher := NewDispatcher(DispatcherOptions{
		Store:    store,
		Clock:    testClock{now: 1000},
		Client:   server.Client(),
		Resolver: app.DefaultHookResolver(),
		Logger:   logger,
	})
	if err := dispatcher.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	logText := logBuf.String()
	for _, want := range []string{
		"component=notification_dispatcher",
		"operation=notification_delivery_attempt",
		"delivery_id=" + delivery.ID,
		"sink_id=" + sink.ID,
		"event_type=task.due_soon",
		"object_kind=task",
		"object_id=task-1",
		"result=success",
	} {
		if !strings.Contains(logText, want) {
			t.Fatalf("log = %q, want substring %q", logText, want)
		}
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

type blockingNotificationRoundTripper struct {
	started chan struct{}
	release chan struct{}
	once    *sync.Once
}

func (rt blockingNotificationRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.once.Do(func() { close(rt.started) })
	select {
	case <-rt.release:
	case <-req.Context().Done():
		return nil, req.Context().Err()
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("ok")),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func TestNotificationDispatcherRunOnceSkipsClaimWhenDraining(t *testing.T) {
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
	mustEnqueueRuntimeDelivery(t, store, delivery)

	shutdown := runtimeutil.NewShutdownCoordinator()
	shutdown.StopAccepting()
	dispatcher := NewDispatcher(DispatcherOptions{
		Store:    store,
		Clock:    testClock{now: 1000},
		Client:   server.Client(),
		Resolver: app.DefaultHookResolver(),
		Shutdown: shutdown,
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
	if got.AttemptCount != 0 {
		t.Fatalf("attempt_count = %d, want 0", got.AttemptCount)
	}
}

func TestNotificationDispatcherDrainWaitsAndRequeuesNotStartedDelivery(t *testing.T) {
	store := newNotificationRuntimeStore(t)
	ws, _ := store.LocalWorkspace()
	sink := makeRuntimeSink(ws.ID, "https://example.com/notify")
	mustCreateRuntimeSink(t, store, sink)
	ids := make([]string, 0, 2)
	for i := 0; i < 2; i++ {
		delivery := makeRuntimeDelivery(ws.ID, sink.ID, "https://example.com/notify")
		delivery.TaskUUID = "task-drain-" + strconv.Itoa(i)
		delivery.CreatedAt = int64(100 + i)
		delivery.ModifiedAt = delivery.CreatedAt
		ids = append(ids, delivery.ID)
		mustEnqueueRuntimeDelivery(t, store, delivery)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	shutdown := runtimeutil.NewShutdownCoordinator()
	dispatcher := NewDispatcher(DispatcherOptions{
		Store:          store,
		Clock:          testClock{now: 1000},
		Client:         &http.Client{Transport: blockingNotificationRoundTripper{started: started, release: release, once: &sync.Once{}}},
		Resolver:       app.DefaultHookResolver(),
		MaxConcurrency: 1,
		PrefetchFactor: 2,
		Shutdown:       shutdown,
	})
	runDone := make(chan error, 1)
	go func() {
		runDone <- dispatcher.RunOnce(context.Background())
	}()
	<-started

	drainDone := make(chan error, 1)
	go func() {
		drainDone <- shutdown.Drain(context.Background())
	}()
	select {
	case err := <-drainDone:
		t.Fatalf("Drain returned before worker finished: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("RunOnce() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunOnce did not return")
	}
	select {
	case err := <-drainDone:
		if err != nil {
			t.Fatalf("Drain() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Drain did not return")
	}

	repo := storage.NewNotificationDeliveryRepository(store.DB())
	first, err := repo.GetByID(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	second, err := repo.GetByID(ids[1])
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != storage.DeliveryStatusSucceeded {
		t.Fatalf("first status = %q, want succeeded", first.Status)
	}
	if second.Status != storage.DeliveryStatusQueued {
		t.Fatalf("second status = %q, want queued", second.Status)
	}
	if second.ClaimExpiresAt != nil {
		t.Fatalf("second claim_expires_at = %#v, want nil", second.ClaimExpiresAt)
	}
	if second.AttemptCount != 0 {
		t.Fatalf("second attempt_count = %d, want reverted to 0", second.AttemptCount)
	}
}

func TestNotificationDispatcherForceCancelCancelsStartedDelivery(t *testing.T) {
	store := newNotificationRuntimeStore(t)
	ws, _ := store.LocalWorkspace()
	sink := makeRuntimeSink(ws.ID, "https://example.com/notify")
	mustCreateRuntimeSink(t, store, sink)
	delivery := makeRuntimeDelivery(ws.ID, sink.ID, "https://example.com/notify")
	mustEnqueueRuntimeDelivery(t, store, delivery)

	started := make(chan struct{})
	release := make(chan struct{})
	shutdown := runtimeutil.NewShutdownCoordinator()
	dispatcher := NewDispatcher(DispatcherOptions{
		Store:    store,
		Clock:    testClock{now: 1000},
		Client:   &http.Client{Transport: blockingNotificationRoundTripper{started: started, release: release, once: &sync.Once{}}},
		Resolver: app.DefaultHookResolver(),
		Shutdown: shutdown,
	})
	runDone := make(chan error, 1)
	go func() {
		runDone <- dispatcher.RunOnce(context.Background())
	}()
	<-started
	shutdown.ForceCancel()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("RunOnce() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunOnce did not return after ForceCancel")
	}
	close(release)

	got, err := storage.NewNotificationDeliveryRepository(store.DB()).GetByID(delivery.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != storage.DeliveryStatusRetryWait {
		t.Fatalf("status = %q, want retry_wait", got.Status)
	}
	if !strings.Contains(got.LastError, "context canceled") {
		t.Fatalf("last_error = %q, want context canceled", got.LastError)
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
