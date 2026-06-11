package hookruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/logging"
	"git.dajee.net/dajee/xuanchu/internal/runtimeutil"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"github.com/google/uuid"
)

// --- 测试辅助工具 ---

type testClock struct {
	now int64
}

func (c testClock) Unix() int64              { return c.now }
func (c testClock) Location() *time.Location { return time.UTC }

// mockResolver 用于 SSRF 测试，始终返回指定的 IP。
type mockResolver struct {
	ips []net.IPAddr
	err error
}

func (m mockResolver) LookupIPAddr(_ context.Context, _ string) ([]net.IPAddr, error) {
	if len(m.ips) == 0 && m.err == nil {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}
	return m.ips, m.err
}

// noRedirectClient 返回一个不跟随重定向的 HTTP 客户端。
func noRedirectClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func newTestStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	wsID := mustLocalWorkspace(t, store)
	sink := makeTestSink(t, wsID, "https://example.com/webhook")
	if err := storage.NewNotificationSinkRepository(store.DB()).Create(sink); err != nil {
		t.Fatal(err)
	}
	return store
}

func mustLocalWorkspace(t *testing.T, store *storage.Store) string {
	t.Helper()
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	return ws.ID
}

func boolPtr(v bool) *bool    { return &v }
func int64Ptr(v int64) *int64 { return &v }
func intPtr(v int) *int       { return &v }
func strPtr(v string) *string { return &v }

func makeTestSink(t *testing.T, wsID string, endpointURL string, overrides ...func(*storage.NotificationSink)) storage.NotificationSink {
	t.Helper()
	s := storage.NotificationSink{
		ID:                  "sink-1",
		WorkspaceID:         wsID,
		Name:                "test-sink",
		Type:                "webhook",
		EndpointMode:        "static_url",
		URL:                 endpointURL,
		AllowedHostsJSON:    `[]`,
		HTTPMethod:          "POST",
		HeaderTemplatesJSON: `[]`,
		BodyContentType:     "application/json",
		SecretRefsJSON:      `{}`,
		Secret:              "test-secret",
		Enabled:             boolPtr(true),
		TimeoutSeconds:      10,
		MaxAttempts:         5,
		CreatedBy:           "user-1",
		CreatedAt:           100,
		ModifiedAt:          100,
	}
	for _, fn := range overrides {
		fn(&s)
	}
	return s
}

func makeTestHook(t *testing.T, wsID string, endpointURL string, overrides ...func(*storage.HookDefinition)) storage.HookDefinition {
	t.Helper()
	h := storage.HookDefinition{
		ID:             uuid.NewString(),
		Name:           "test-hook",
		ScopeType:      "workspace",
		WorkspaceID:    wsID,
		ActorUserID:    "user-1",
		EventTypesJSON: `["task.created"]`,
		SinkID:         "sink-1",
		Enabled:        boolPtr(true),
		TimeoutSeconds: 10,
		MaxAttempts:    5,
		CreatedAt:      100,
		ModifiedAt:     100,
	}
	for _, fn := range overrides {
		fn(&h)
	}
	return h
}

func makeTestDelivery(t *testing.T, hookID, wsID string, overrides ...func(*storage.HookDelivery)) storage.HookDelivery {
	t.Helper()
	d := storage.HookDelivery{
		ID:                  uuid.NewString(),
		HookID:              hookID,
		EventID:             uuid.NewString(),
		EventType:           "task.created",
		WorkspaceID:         wsID,
		ActorUserID:         "user-1",
		SinkID:              "sink-1",
		ResolvedURL:         "https://example.com/webhook",
		RenderedMethod:      http.MethodPost,
		RenderedHeadersJSON: `{"Content-Type":["application/json"]}`,
		RenderedBody:        `{"test":true}`,
		RenderedContentType: "application/json",
		PayloadJSON:         `{"test":true}`,
		HeadersJSON:         `{}`,
		Status:              storage.DeliveryStatusQueued,
		CreatedAt:           200,
		ModifiedAt:          200,
	}
	for _, fn := range overrides {
		fn(&d)
	}
	return d
}

func setupHookAndDelivery(t *testing.T, store *storage.Store, endpointURL string) (storage.HookDefinition, storage.HookDelivery) {
	t.Helper()
	wsID := mustLocalWorkspace(t, store)
	hookRepo := storage.NewHookRepository(store.DB())
	deliveryRepo := storage.NewHookDeliveryRepository(store.DB())
	hook := makeTestHook(t, wsID, endpointURL)
	if err := hookRepo.Create(hook); err != nil {
		t.Fatal(err)
	}

	delivery := makeTestDelivery(t, hook.ID, wsID, func(d *storage.HookDelivery) {
		d.ResolvedURL = endpointURL
	})
	if err := deliveryRepo.Enqueue([]storage.HookDelivery{delivery}); err != nil {
		t.Fatal(err)
	}

	return hook, delivery
}

func getDelivery(t *testing.T, store *storage.Store, id string) storage.HookDelivery {
	t.Helper()
	repo := storage.NewHookDeliveryRepository(store.DB())
	d, err := repo.GetByID(id)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// --- 签名测试 ---

func TestWebhookSignature(t *testing.T) {
	secret := "my-secret"
	deliveryID := "del-123"
	timestamp := int64(1700000000)
	body := []byte(`{"event":"test"}`)

	sig := SignatureSHA256(secret, deliveryID, timestamp, body)

	if sig == "" {
		t.Fatal("expected non-empty signature")
	}
	// 签名格式: sha256=<hex>
	if len(sig) < 8 || sig[:7] != "sha256=" {
		t.Fatalf("signature format = %q, want sha256=<hex>", sig)
	}

	// 稳定性验证：相同输入产生相同输出
	sig2 := SignatureSHA256(secret, deliveryID, timestamp, body)
	if sig != sig2 {
		t.Fatalf("signature not stable: %q vs %q", sig, sig2)
	}

	// 空 secret 返回空字符串
	emptySig := SignatureSHA256("", deliveryID, timestamp, body)
	if emptySig != "" {
		t.Fatalf("empty secret should produce empty signature, got %q", emptySig)
	}
}

func TestHeadersForDelivery(t *testing.T) {
	wsID := "ws-1"
	hook := storage.HookDefinition{
		ID:             "hook-1",
		Name:           "test",
		WorkspaceID:    wsID,
		Secret:         "my-secret",
		TimeoutSeconds: 10,
	}
	delivery := storage.HookDelivery{
		ID:           "del-1",
		HookID:       "hook-1",
		WorkspaceID:  wsID,
		AttemptCount: 2,
		HeadersJSON:  `{"X-Custom":"custom-value"}`,
	}
	body := []byte(`{"test":true}`)
	now := int64(1700000000)

	headers, err := HeadersForDelivery(delivery, hook.ID, hook.Secret, body, now, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}

	// 检查必需 headers
	if v := headers.Get("Content-Type"); v != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", v)
	}
	if v := headers.Get("X-Xuanchu-Delivery"); v != "del-1" {
		t.Fatalf("X-Xuanchu-Delivery = %q", v)
	}
	if v := headers.Get("X-Xuanchu-Hook-Id"); v != "hook-1" {
		t.Fatalf("X-Xuanchu-Hook-Id = %q", v)
	}
	if v := headers.Get("X-Xuanchu-Attempt"); v != "2" {
		t.Fatalf("X-Xuanchu-Attempt = %q", v)
	}
	if v := headers.Get("User-Agent"); v != "xuanchu-webhook/1.0.0" {
		t.Fatalf("User-Agent = %q", v)
	}

	// 签名 headers（有 secret 时）
	if v := headers.Get("X-Xuanchu-Timestamp"); v == "" {
		t.Fatal("expected X-Xuanchu-Timestamp header when secret exists")
	}
	if v := headers.Get("X-Xuanchu-Signature-256"); v == "" {
		t.Fatal("expected X-Xuanchu-Signature-256 header when secret exists")
	}

	// 存储的自定义 header 应该被保留
	if v := headers.Get("X-Custom"); v != "custom-value" {
		t.Fatalf("X-Custom = %q, want custom-value", v)
	}
}

func TestHeadersForDeliveryNoSecret(t *testing.T) {
	hook := storage.HookDefinition{
		ID:     "hook-1",
		Secret: "",
	}
	delivery := storage.HookDelivery{
		ID:           "del-1",
		HookID:       "hook-1",
		AttemptCount: 1,
		HeadersJSON:  `{}`,
	}
	body := []byte(`{}`)
	now := int64(1700000000)

	headers, err := HeadersForDelivery(delivery, hook.ID, hook.Secret, body, now, "dev")
	if err != nil {
		t.Fatal(err)
	}

	// 没有 secret 时不应该有 timestamp 和 signature
	if v := headers.Get("X-Xuanchu-Timestamp"); v != "" {
		t.Fatalf("expected no X-Xuanchu-Timestamp without secret, got %q", v)
	}
	if v := headers.Get("X-Xuanchu-Signature-256"); v != "" {
		t.Fatalf("expected no X-Xuanchu-Signature-256 without secret, got %q", v)
	}
}

// --- Dispatcher 测试 ---

func TestDispatcherRunOnceDeliversWebhook(t *testing.T) {
	var mu sync.Mutex
	var receivedRequest *http.Request
	var receivedBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		receivedRequest = r
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := newTestStore(t)
	hook, delivery := setupHookAndDelivery(t, store, server.URL)

	clock := testClock{now: 1000}
	d := NewDispatcher(DispatcherOptions{
		Store:    store,
		Clock:    clock,
		Client:   server.Client(),
		Resolver: mockResolver{}, // 空 resolver，SSRF 通过
	})

	if err := d.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	// 验证收到了请求
	if receivedRequest == nil {
		t.Fatal("expected webhook request")
	}

	// 验证请求方法和路径
	if receivedRequest.Method != http.MethodPost {
		t.Fatalf("method = %q, want POST", receivedRequest.Method)
	}

	// 验证 body
	var payload map[string]any
	if err := json.Unmarshal(receivedBody, &payload); err != nil {
		t.Fatalf("body parse error: %v", err)
	}
	if payload["test"] != true {
		t.Fatalf("payload = %v", payload)
	}

	// 验证必需 headers
	if v := receivedRequest.Header.Get("X-Xuanchu-Delivery"); v != delivery.ID {
		t.Fatalf("X-Xuanchu-Delivery = %q, want %q", v, delivery.ID)
	}
	if v := receivedRequest.Header.Get("X-Xuanchu-Hook-Id"); v != hook.ID {
		t.Fatalf("X-Xuanchu-Hook-Id = %q, want %q", v, hook.ID)
	}
	if v := receivedRequest.Header.Get("X-Xuanchu-Signature-256"); v == "" {
		t.Fatal("expected signature header")
	}

	// 验证投递状态为 succeeded
	got := getDelivery(t, store, delivery.ID)
	if got.Status != storage.DeliveryStatusSucceeded {
		t.Fatalf("status = %q, want succeeded", got.Status)
	}
}

func TestHookDispatcherWritesOperationLog(t *testing.T) {
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

	store := newTestStore(t)
	hook, delivery := setupHookAndDelivery(t, store, server.URL)
	delivery.PayloadJSON = `{"object_kind":"task","object_id":"task-1"}`
	if err := store.DB().Model(&storage.HookDelivery{}).Where("id = ?", delivery.ID).Update("payload_json", delivery.PayloadJSON).Error; err != nil {
		t.Fatal(err)
	}

	d := NewDispatcher(DispatcherOptions{
		Store:    store,
		Clock:    testClock{now: 1000},
		Client:   server.Client(),
		Resolver: mockResolver{},
		Logger:   logger,
	})
	if err := d.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	logText := logBuf.String()
	for _, want := range []string{
		"component=hook_dispatcher",
		"operation=hook_delivery_attempt",
		"delivery_id=" + delivery.ID,
		"hook_id=" + hook.ID,
		"object_kind=task",
		"object_id=task-1",
		"result=success",
	} {
		if !strings.Contains(logText, want) {
			t.Fatalf("log = %q, want substring %q", logText, want)
		}
	}
}

func TestDispatcherNetworkError(t *testing.T) {
	store := newTestStore(t)
	_, delivery := setupHookAndDelivery(t, store, "http://127.0.0.1:1/unreachable")

	clock := testClock{now: 1000}
	d := NewDispatcher(DispatcherOptions{
		Store: store,
		Clock: clock,
		Client: &http.Client{
			Timeout: 2 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		Resolver: mockResolver{},
	})

	if err := d.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	got := getDelivery(t, store, delivery.ID)
	if got.Status != storage.DeliveryStatusRetryWait {
		t.Fatalf("status = %q, want retry_wait", got.Status)
	}
	if got.LastError == "" {
		t.Fatal("expected non-empty last_error")
	}
}

func TestDispatcherTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := newTestStore(t)
	wsID := mustLocalWorkspace(t, store)
	hookRepo := storage.NewHookRepository(store.DB())
	deliveryRepo := storage.NewHookDeliveryRepository(store.DB())

	hook := makeTestHook(t, wsID, server.URL, func(h *storage.HookDefinition) {
		h.TimeoutSeconds = 1 // 1秒超时
	})
	if err := hookRepo.Create(hook); err != nil {
		t.Fatal(err)
	}

	delivery := makeTestDelivery(t, hook.ID, wsID, func(d *storage.HookDelivery) {
		d.ResolvedURL = server.URL
	})
	if err := deliveryRepo.Enqueue([]storage.HookDelivery{delivery}); err != nil {
		t.Fatal(err)
	}

	clock := testClock{now: 1000}
	d := NewDispatcher(DispatcherOptions{
		Store:          store,
		Clock:          clock,
		Client:         server.Client(),
		Resolver:       mockResolver{},
		MaxConcurrency: 3,
	})

	if err := d.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	got := getDelivery(t, store, delivery.ID)
	if got.Status != storage.DeliveryStatusRetryWait {
		t.Fatalf("status = %q, want retry_wait", got.Status)
	}
}

func TestDispatcherHTTP3xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "http://127.0.0.1:9999/private")
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()

	store := newTestStore(t)
	_, delivery := setupHookAndDelivery(t, store, server.URL)

	clock := testClock{now: 1000}
	d := NewDispatcher(DispatcherOptions{
		Store:    store,
		Clock:    clock,
		Client:   noRedirectClient(),
		Resolver: mockResolver{},
	})

	if err := d.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	got := getDelivery(t, store, delivery.ID)
	if got.Status != storage.DeliveryStatusDeadLettered {
		t.Fatalf("status = %q, want dead_lettered", got.Status)
	}
	if got.LastStatusCode == nil || *got.LastStatusCode != 302 {
		t.Fatalf("last_status_code = %v, want 302", got.LastStatusCode)
	}
}

func TestDispatcherHTTP500(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	store := newTestStore(t)
	_, delivery := setupHookAndDelivery(t, store, server.URL)

	clock := testClock{now: 1000}
	d := NewDispatcher(DispatcherOptions{
		Store:          store,
		Clock:          clock,
		Client:         server.Client(),
		Resolver:       mockResolver{},
		MaxConcurrency: 3,
	})

	if err := d.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	got := getDelivery(t, store, delivery.ID)
	if got.Status != storage.DeliveryStatusRetryWait {
		t.Fatalf("status = %q, want retry_wait", got.Status)
	}
	if got.LastStatusCode == nil || *got.LastStatusCode != 500 {
		t.Fatalf("last_status_code = %v, want 500", got.LastStatusCode)
	}
}

func TestDispatcherFirstFailureBackoffUsesFirstAttemptRange(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	store := newTestStore(t)
	_, delivery := setupHookAndDelivery(t, store, server.URL)

	clock := testClock{now: 1000}
	d := NewDispatcher(DispatcherOptions{
		Store:          store,
		Clock:          clock,
		Client:         server.Client(),
		Resolver:       mockResolver{},
		RetryBaseDelay: 30 * time.Second,
		JitterSeed:     42,
	})

	if err := d.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	got := getDelivery(t, store, delivery.ID)
	if got.NextAttemptAt == nil {
		t.Fatal("next_attempt_at is nil")
	}
	delay := time.Duration(*got.NextAttemptAt-clock.now) * time.Second
	if delay < 15*time.Second || delay > 30*time.Second {
		t.Fatalf("first failure delay = %v, want 15s..30s", delay)
	}
}

func TestDispatcherHTTP429(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	store := newTestStore(t)
	_, delivery := setupHookAndDelivery(t, store, server.URL)

	clock := testClock{now: 1000}
	d := NewDispatcher(DispatcherOptions{
		Store:          store,
		Clock:          clock,
		Client:         server.Client(),
		Resolver:       mockResolver{},
		MaxConcurrency: 3,
	})

	if err := d.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	got := getDelivery(t, store, delivery.ID)
	if got.Status != storage.DeliveryStatusRetryWait {
		t.Fatalf("status = %q, want retry_wait", got.Status)
	}
	if got.LastStatusCode == nil || *got.LastStatusCode != 429 {
		t.Fatalf("last_status_code = %v, want 429", got.LastStatusCode)
	}
}

func TestDispatcherHTTP429WithRetryAfter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	store := newTestStore(t)
	_, delivery := setupHookAndDelivery(t, store, server.URL)

	clock := testClock{now: 1000}
	d := NewDispatcher(DispatcherOptions{
		Store:    store,
		Clock:    clock,
		Client:   server.Client(),
		Resolver: mockResolver{},
	})

	if err := d.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	got := getDelivery(t, store, delivery.ID)
	if got.Status != storage.DeliveryStatusRetryWait {
		t.Fatalf("status = %q, want retry_wait", got.Status)
	}
	// next_attempt_at 应该是 now + 120 = 1120
	if got.NextAttemptAt == nil || *got.NextAttemptAt != 1120 {
		t.Fatalf("next_attempt_at = %v, want 1120", got.NextAttemptAt)
	}
}

func TestDispatcherHTTP400(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	store := newTestStore(t)
	_, delivery := setupHookAndDelivery(t, store, server.URL)

	clock := testClock{now: 1000}
	d := NewDispatcher(DispatcherOptions{
		Store:    store,
		Clock:    clock,
		Client:   server.Client(),
		Resolver: mockResolver{},
	})

	if err := d.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	got := getDelivery(t, store, delivery.ID)
	if got.Status != storage.DeliveryStatusDeadLettered {
		t.Fatalf("status = %q, want dead_lettered", got.Status)
	}
	if got.LastStatusCode == nil || *got.LastStatusCode != 400 {
		t.Fatalf("last_status_code = %v, want 400", got.LastStatusCode)
	}
}

func TestDispatcherMaxAttemptsDeadLetter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	store := newTestStore(t)
	wsID := mustLocalWorkspace(t, store)
	hookRepo := storage.NewHookRepository(store.DB())
	deliveryRepo := storage.NewHookDeliveryRepository(store.DB())

	hook := makeTestHook(t, wsID, server.URL, func(h *storage.HookDefinition) {
		h.MaxAttempts = 1
	})
	if err := hookRepo.Create(hook); err != nil {
		t.Fatal(err)
	}

	// 创建一个已经 attempt_count=1 的 delivery（即将超出 max_attempts=1）
	delivery := makeTestDelivery(t, hook.ID, wsID, func(d *storage.HookDelivery) {
		d.ResolvedURL = server.URL
		d.AttemptCount = 1 // ClaimDue 会再 +1，所以实际是 2
		d.Status = storage.DeliveryStatusRetryWait
		d.NextAttemptAt = int64Ptr(500) // 过去时间
	})
	if err := deliveryRepo.Enqueue([]storage.HookDelivery{delivery}); err != nil {
		t.Fatal(err)
	}

	clock := testClock{now: 1000}
	d := NewDispatcher(DispatcherOptions{
		Store:    store,
		Clock:    clock,
		Client:   server.Client(),
		Resolver: mockResolver{},
	})

	if err := d.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	got := getDelivery(t, store, delivery.ID)
	// attempt_count=2 >= max_attempts=1 -> dead_lettered
	if got.Status != storage.DeliveryStatusDeadLettered {
		t.Fatalf("status = %q, want dead_lettered (max attempts exhausted)", got.Status)
	}
}

func TestDispatcherDisabledHookSkipped(t *testing.T) {
	var received bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := newTestStore(t)
	wsID := mustLocalWorkspace(t, store)
	hookRepo := storage.NewHookRepository(store.DB())
	deliveryRepo := storage.NewHookDeliveryRepository(store.DB())

	hook := makeTestHook(t, wsID, server.URL, func(h *storage.HookDefinition) {
		h.Enabled = boolPtr(false)
	})
	if err := hookRepo.Create(hook); err != nil {
		t.Fatal(err)
	}

	delivery := makeTestDelivery(t, hook.ID, wsID, func(d *storage.HookDelivery) {
		d.ResolvedURL = server.URL
	})
	if err := deliveryRepo.Enqueue([]storage.HookDelivery{delivery}); err != nil {
		t.Fatal(err)
	}

	clock := testClock{now: 1000}
	d := NewDispatcher(DispatcherOptions{
		Store:    store,
		Clock:    clock,
		Client:   server.Client(),
		Resolver: mockResolver{},
	})

	if err := d.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	if received {
		t.Fatal("webhook should not be sent for disabled hook")
	}

	got := getDelivery(t, store, delivery.ID)
	if got.Status != storage.DeliveryStatusDisabledSkipped {
		t.Fatalf("status = %q, want disabled_skipped", got.Status)
	}
}

func TestDispatcherStaleRecovery(t *testing.T) {
	store := newTestStore(t)
	wsID := mustLocalWorkspace(t, store)
	hookRepo := storage.NewHookRepository(store.DB())
	deliveryRepo := storage.NewHookDeliveryRepository(store.DB())

	// 创建 hook（让 dispatchOne 能找到 hook 但不执行 SSRF 验证失败）
	hook := makeTestHook(t, wsID, "https://example.com/webhook")
	if err := hookRepo.Create(hook); err != nil {
		t.Fatal(err)
	}

	// 创建一个 stale delivering 状态的 delivery
	delivery := makeTestDelivery(t, hook.ID, wsID, func(d *storage.HookDelivery) {
		d.Status = storage.DeliveryStatusDelivering
		d.ClaimExpiresAt = int64Ptr(500) // 已过期
		d.AttemptCount = 1
	})
	if err := deliveryRepo.Enqueue([]storage.HookDelivery{delivery}); err != nil {
		t.Fatal(err)
	}

	// 先单独调用 RecoverStaleDelivering 验证恢复效果
	affected, err := deliveryRepo.RecoverStaleDelivering(1000)
	if err != nil {
		t.Fatalf("RecoverStaleDelivering() error = %v", err)
	}
	if affected != 1 {
		t.Fatalf("affected = %d, want 1", affected)
	}

	got := getDelivery(t, store, delivery.ID)
	if got.Status != storage.DeliveryStatusQueued {
		t.Fatalf("status after recovery = %q, want queued", got.Status)
	}
}

func TestDispatcherNoDueReturnsNil(t *testing.T) {
	store := newTestStore(t)

	clock := testClock{now: 1000}
	d := NewDispatcher(DispatcherOptions{
		Store:    store,
		Clock:    clock,
		Resolver: mockResolver{},
	})

	if err := d.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() with no deliveries error = %v", err)
	}
}

func TestDispatcherDNSRebinding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := newTestStore(t)
	wsID := mustLocalWorkspace(t, store)
	hookRepo := storage.NewHookRepository(store.DB())
	deliveryRepo := storage.NewHookDeliveryRepository(store.DB())

	hook := makeTestHook(t, wsID, "https://evil.example.com/webhook")
	if err := hookRepo.Create(hook); err != nil {
		t.Fatal(err)
	}

	delivery := makeTestDelivery(t, hook.ID, wsID, func(d *storage.HookDelivery) {
		d.ResolvedURL = "https://evil.example.com/webhook"
	})
	if err := deliveryRepo.Enqueue([]storage.HookDelivery{delivery}); err != nil {
		t.Fatal(err)
	}

	// mock resolver 返回一个私有 IP（模拟 DNS rebinding）
	privateIP := net.ParseIP("192.168.1.1")
	badResolver := mockResolver{
		ips: []net.IPAddr{{IP: privateIP}},
	}

	clock := testClock{now: 1000}
	d := NewDispatcher(DispatcherOptions{
		Store:    store,
		Clock:    clock,
		Client:   server.Client(),
		Resolver: badResolver,
	})

	if err := d.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	got := getDelivery(t, store, delivery.ID)
	if got.Status != storage.DeliveryStatusDeadLettered {
		t.Fatalf("status = %q, want dead_lettered (SSRF protection)", got.Status)
	}
	if got.LastError == "" {
		t.Fatal("expected non-empty last_error for SSRF failure")
	}
}

func TestDefaultWebhookClientRejectsPrivateDialAddress(t *testing.T) {
	client := defaultWebhookClient(mockResolver{ips: []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}})
	req, err := http.NewRequest(http.MethodPost, "http://example.test/hook", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected private dial address to be rejected")
	}
	if !strings.Contains(err.Error(), "no allowed addresses") {
		t.Fatalf("error = %v, want no allowed addresses", err)
	}
}

func TestDispatcherNoRedirect(t *testing.T) {
	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if r.URL.Path == "/redirect" {
			w.Header().Set("Location", "/target")
			w.WriteHeader(http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := newTestStore(t)
	_, delivery := setupHookAndDelivery(t, store, server.URL+"/redirect")

	clock := testClock{now: 1000}
	d := NewDispatcher(DispatcherOptions{
		Store:    store,
		Clock:    clock,
		Client:   noRedirectClient(),
		Resolver: mockResolver{},
	})

	if err := d.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	// 不应该跟随重定向
	if requestCount != 1 {
		t.Fatalf("request count = %d, want 1 (no redirect following)", requestCount)
	}

	got := getDelivery(t, store, delivery.ID)
	if got.Status != storage.DeliveryStatusDeadLettered {
		t.Fatalf("status = %q, want dead_lettered (3xx not followed)", got.Status)
	}
}

func TestDispatcherContextCancellation(t *testing.T) {
	store := newTestStore(t)

	clock := testClock{now: 1000}
	d := NewDispatcher(DispatcherOptions{
		Store:    store,
		Clock:    clock,
		Resolver: mockResolver{},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	if err := d.RunOnce(ctx); err != nil {
		// context 取消应该返回 nil 或 context 错误
		t.Logf("RunOnce with cancelled context: %v", err)
	}
}

func TestDispatcherExponentialBackoff(t *testing.T) {
	d := NewDispatcher(DispatcherOptions{
		Store:          newTestStore(t),
		Clock:          testClock{now: 1000},
		RetryBaseDelay: 30 * time.Second,
		JitterSeed:     42, // 固定种子，结果可预测
		Resolver:       mockResolver{},
	})

	// 验证退避范围
	for attempt := 1; attempt <= 6; attempt++ {
		delay := d.calculateBackoff(attempt)
		t.Logf("attempt %d: delay = %v", attempt, delay)
		// 延迟应该 > 0
		if delay <= 0 {
			t.Fatalf("attempt %d: delay should be positive", attempt)
		}
	}

	// 第一次重试 (attempt=1): 30s * 2^0 * jitter = ~15-30s
	delay1 := d.calculateBackoff(1)
	if delay1 < 15*time.Second || delay1 > 30*time.Second {
		t.Fatalf("attempt 1 delay = %v, expected ~15-30s", delay1)
	}

	// 高次重试不应超过 1 小时
	delayHigh := d.calculateBackoff(20)
	if delayHigh > time.Hour {
		t.Fatalf("high attempt delay = %v, should be capped at 1h", delayHigh)
	}
}

func TestDispatcherHookNotFoundDeadLetters(t *testing.T) {
	store := newTestStore(t)
	wsID := mustLocalWorkspace(t, store)
	deliveryRepo := storage.NewHookDeliveryRepository(store.DB())

	// 创建指向不存在 hook 的 delivery
	delivery := makeTestDelivery(t, "nonexistent-hook", wsID)
	if err := deliveryRepo.Enqueue([]storage.HookDelivery{delivery}); err != nil {
		t.Fatal(err)
	}

	clock := testClock{now: 1000}
	d := NewDispatcher(DispatcherOptions{
		Store:    store,
		Clock:    clock,
		Resolver: mockResolver{},
	})

	if err := d.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	got := getDelivery(t, store, delivery.ID)
	if got.Status != storage.DeliveryStatusDeadLettered {
		t.Fatalf("status = %q, want dead_lettered (hook not found)", got.Status)
	}
	if got.LastError != "hook not found" {
		t.Fatalf("last_error = %q, want 'hook not found'", got.LastError)
	}
}

func TestDispatcherRunStopsOnContextCancel(t *testing.T) {
	store := newTestStore(t)

	clock := testClock{now: 1000}
	d := NewDispatcher(DispatcherOptions{
		Store:        store,
		Clock:        clock,
		PollInterval: 100 * time.Millisecond,
		Resolver:     mockResolver{},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	err := d.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestParseRetryAfterSeconds(t *testing.T) {
	resp := &http.Response{
		Header: http.Header{
			"Retry-After": {"60"},
		},
	}
	dur := parseRetryAfter(resp, 1000)
	if dur != 60*time.Second {
		t.Fatalf("Retry-After 60 = %v, want 60s", dur)
	}
}

func TestParseRetryAfterEmpty(t *testing.T) {
	resp := &http.Response{
		Header: http.Header{},
	}
	dur := parseRetryAfter(resp, 1000)
	if dur != 0 {
		t.Fatalf("empty Retry-After = %v, want 0", dur)
	}
}

func TestParseRetryAfterInvalid(t *testing.T) {
	resp := &http.Response{
		Header: http.Header{
			"Retry-After": {"not-a-number"},
		},
	}
	dur := parseRetryAfter(resp, 1000)
	if dur != 0 {
		t.Fatalf("invalid Retry-After = %v, want 0", dur)
	}
}

func TestRetryAfterIsCappedByDispatcher(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "31536000")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	store := newTestStore(t)
	_, delivery := setupHookAndDelivery(t, store, server.URL)

	clock := testClock{now: 1000}
	d := NewDispatcher(DispatcherOptions{
		Store:    store,
		Clock:    clock,
		Client:   server.Client(),
		Resolver: mockResolver{},
	})

	if err := d.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	got := getDelivery(t, store, delivery.ID)
	if got.NextAttemptAt == nil || *got.NextAttemptAt != clock.now+int64(time.Hour.Seconds()) {
		t.Fatalf("next_attempt_at = %v, want %d", got.NextAttemptAt, clock.now+int64(time.Hour.Seconds()))
	}
}

func TestDispatcherSignatureHeadersMatch(t *testing.T) {
	var receivedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := newTestStore(t)
	_, delivery := setupHookAndDelivery(t, store, server.URL)

	clock := testClock{now: 1700000000}
	d := NewDispatcher(DispatcherOptions{
		Store:    store,
		Clock:    clock,
		Client:   server.Client(),
		Resolver: mockResolver{},
		Version:  "test-version",
	})

	if err := d.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	// 验证签名一致性
	body := []byte(delivery.RenderedBody)
	expectedSig := SignatureSHA256("test-secret", delivery.ID, 1700000000, body)
	gotSig := receivedHeaders.Get("X-Xuanchu-Signature-256")
	if gotSig != expectedSig {
		t.Fatalf("signature mismatch: got %q, want %q", gotSig, expectedSig)
	}

	// 验证 User-Agent
	if v := receivedHeaders.Get("User-Agent"); v != "xuanchu-webhook/test-version" {
		t.Fatalf("User-Agent = %q, want xuanchu-webhook/test-version", v)
	}

	// 验证时间戳
	if v := receivedHeaders.Get("X-Xuanchu-Timestamp"); v != "1700000000" {
		t.Fatalf("X-Xuanchu-Timestamp = %q, want 1700000000", v)
	}
}

func TestDispatcherRunOnceMultipleDeliveries(t *testing.T) {
	var mu sync.Mutex
	var count int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		count++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := newTestStore(t)
	wsID := mustLocalWorkspace(t, store)
	hookRepo := storage.NewHookRepository(store.DB())
	deliveryRepo := storage.NewHookDeliveryRepository(store.DB())

	hook := makeTestHook(t, wsID, server.URL)
	if err := hookRepo.Create(hook); err != nil {
		t.Fatal(err)
	}

	deliveries := make([]storage.HookDelivery, 3)
	for i := range deliveries {
		deliveries[i] = makeTestDelivery(t, hook.ID, wsID, func(d *storage.HookDelivery) {
			d.ResolvedURL = server.URL
		})
	}
	if err := deliveryRepo.Enqueue(deliveries); err != nil {
		t.Fatal(err)
	}

	clock := testClock{now: 1000}
	d := NewDispatcher(DispatcherOptions{
		Store:          store,
		Clock:          clock,
		Client:         server.Client(),
		Resolver:       mockResolver{},
		MaxConcurrency: 3,
	})

	if err := d.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if count != 3 {
		t.Fatalf("webhook count = %d, want 3", count)
	}

	// 验证所有 delivery 状态为 succeeded
	for _, del := range deliveries {
		got := getDelivery(t, store, del.ID)
		if got.Status != storage.DeliveryStatusSucceeded {
			t.Fatalf("delivery %s status = %q, want succeeded", del.ID, got.Status)
		}
	}
}

func TestDispatcherRunOnceRespectsBatchSize(t *testing.T) {
	var mu sync.Mutex
	var count int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		count++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := newTestStore(t)
	wsID := mustLocalWorkspace(t, store)
	hookRepo := storage.NewHookRepository(store.DB())
	deliveryRepo := storage.NewHookDeliveryRepository(store.DB())

	hook := makeTestHook(t, wsID, server.URL)
	if err := hookRepo.Create(hook); err != nil {
		t.Fatal(err)
	}

	// 创建 5 个 delivery
	deliveries := make([]storage.HookDelivery, 5)
	for i := range deliveries {
		deliveries[i] = makeTestDelivery(t, hook.ID, wsID, func(d *storage.HookDelivery) {
			d.ResolvedURL = server.URL
		})
	}
	if err := deliveryRepo.Enqueue(deliveries); err != nil {
		t.Fatal(err)
	}

	clock := testClock{now: 1000}
	d := NewDispatcher(DispatcherOptions{
		Store:          store,
		Clock:          clock,
		Client:         server.Client(),
		BatchSize:      2, // 每次只取 2 个
		MaxConcurrency: 2,
		Resolver:       mockResolver{},
	})

	// 第一次 RunOnce 只取 2 个
	if err := d.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	mu.Lock()
	firstCount := count
	mu.Unlock()
	if firstCount != 2 {
		t.Fatalf("first batch count = %d, want 2", firstCount)
	}
}

func TestHookDispatcherClaimLimitUsesMaxConcurrency(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := newTestStore(t)
	wsID := mustLocalWorkspace(t, store)
	hookRepo := storage.NewHookRepository(store.DB())
	deliveryRepo := storage.NewHookDeliveryRepository(store.DB())
	hook := makeTestHook(t, wsID, server.URL)
	if err := hookRepo.Create(hook); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		delivery := makeTestDelivery(t, hook.ID, wsID, func(d *storage.HookDelivery) {
			d.ResolvedURL = server.URL
			d.CreatedAt = int64(100 + i)
			d.ModifiedAt = d.CreatedAt
		})
		if err := deliveryRepo.Enqueue([]storage.HookDelivery{delivery}); err != nil {
			t.Fatal(err)
		}
	}

	d := NewDispatcher(DispatcherOptions{
		Store:          store,
		Clock:          testClock{now: 1000},
		Client:         server.Client(),
		Resolver:       mockResolver{},
		BatchSize:      50,
		MaxConcurrency: 3,
		PrefetchFactor: 1,
	})
	if err := d.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if requests.Load() != 3 {
		t.Fatalf("requests = %d, want 3", requests.Load())
	}
	var queued int64
	if err := store.DB().Model(&storage.HookDelivery{}).Where("status = ?", storage.DeliveryStatusQueued).Count(&queued).Error; err != nil {
		t.Fatal(err)
	}
	if queued != 7 {
		t.Fatalf("queued = %d, want 7", queued)
	}
}

func TestHookDispatcherMaxConcurrencyBoundsInflightRequests(t *testing.T) {
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

	store := newTestStore(t)
	wsID := mustLocalWorkspace(t, store)
	hookRepo := storage.NewHookRepository(store.DB())
	deliveryRepo := storage.NewHookDeliveryRepository(store.DB())
	hook := makeTestHook(t, wsID, server.URL)
	if err := hookRepo.Create(hook); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		delivery := makeTestDelivery(t, hook.ID, wsID, func(d *storage.HookDelivery) {
			d.ResolvedURL = server.URL
			d.CreatedAt = int64(100 + i)
			d.ModifiedAt = d.CreatedAt
		})
		if err := deliveryRepo.Enqueue([]storage.HookDelivery{delivery}); err != nil {
			t.Fatal(err)
		}
	}

	d := NewDispatcher(DispatcherOptions{Store: store, Clock: testClock{now: 1000}, Client: server.Client(), Resolver: mockResolver{}, MaxConcurrency: 2})
	if err := d.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if maxActive.Load() > 2 {
		t.Fatalf("max active requests = %d, want <= 2", maxActive.Load())
	}
}

func TestHookDispatcherSendsAttemptHeaderFromClaimedDelivery(t *testing.T) {
	var attemptHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptHeader = r.Header.Get("X-Xuanchu-Attempt")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := newTestStore(t)
	wsID := mustLocalWorkspace(t, store)
	hookRepo := storage.NewHookRepository(store.DB())
	deliveryRepo := storage.NewHookDeliveryRepository(store.DB())
	hook := makeTestHook(t, wsID, server.URL)
	if err := hookRepo.Create(hook); err != nil {
		t.Fatal(err)
	}
	delivery := makeTestDelivery(t, hook.ID, wsID, func(d *storage.HookDelivery) {
		d.ResolvedURL = server.URL
		d.Status = storage.DeliveryStatusRetryWait
		d.NextAttemptAt = int64Ptr(900)
		d.AttemptCount = 1
	})
	if err := deliveryRepo.Enqueue([]storage.HookDelivery{delivery}); err != nil {
		t.Fatal(err)
	}

	d := NewDispatcher(DispatcherOptions{Store: store, Clock: testClock{now: 1000}, Client: server.Client(), Resolver: mockResolver{}})
	if err := d.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if attemptHeader != "2" {
		t.Fatalf("X-Xuanchu-Attempt = %q, want 2", attemptHeader)
	}
}

type blockingRoundTripper struct {
	started chan struct{}
	release chan struct{}
}

func (rt blockingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	close(rt.started)
	select {
	case <-rt.release:
	case <-req.Context().Done():
		<-rt.release
		return nil, req.Context().Err()
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("ok")),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

type cancelingRoundTripper struct {
	started chan struct{}
}

func (rt cancelingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	close(rt.started)
	<-req.Context().Done()
	return nil, req.Context().Err()
}

func TestHookDispatcherRunOnceWaitsForStartedWorkersOnCancel(t *testing.T) {
	store := newTestStore(t)
	wsID := mustLocalWorkspace(t, store)
	hookRepo := storage.NewHookRepository(store.DB())
	deliveryRepo := storage.NewHookDeliveryRepository(store.DB())
	hook := makeTestHook(t, wsID, "https://example.com/webhook")
	if err := hookRepo.Create(hook); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		delivery := makeTestDelivery(t, hook.ID, wsID, func(d *storage.HookDelivery) {
			d.ID = uuid.NewString()
			d.ResolvedURL = "https://example.com/webhook"
			d.CreatedAt = int64(100 + i)
			d.ModifiedAt = d.CreatedAt
		})
		if err := deliveryRepo.Enqueue([]storage.HookDelivery{delivery}); err != nil {
			t.Fatal(err)
		}
	}

	started := make(chan struct{})
	release := make(chan struct{})
	client := &http.Client{Transport: blockingRoundTripper{started: started, release: release}}
	dispatcher := NewDispatcher(DispatcherOptions{
		Store:          store,
		Clock:          testClock{now: 1000},
		Client:         client,
		Resolver:       mockResolver{},
		MaxConcurrency: 1,
		PrefetchFactor: 2,
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- dispatcher.RunOnce(ctx)
	}()
	<-started
	cancel()

	select {
	case err := <-done:
		t.Fatalf("RunOnce returned before started worker finished: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("RunOnce() error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunOnce did not return after worker was released")
	}
}

func TestHookDispatcherRunOnceSkipsClaimWhenDraining(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := newTestStore(t)
	wsID := mustLocalWorkspace(t, store)
	if err := storage.NewNotificationSinkRepository(store.DB()).Update(makeTestSink(t, wsID, server.URL, func(s *storage.NotificationSink) {
		s.ID = "sink-1"
	})); err != nil {
		t.Fatal(err)
	}
	hookRepo := storage.NewHookRepository(store.DB())
	deliveryRepo := storage.NewHookDeliveryRepository(store.DB())
	hook := makeTestHook(t, wsID, server.URL)
	if err := hookRepo.Create(hook); err != nil {
		t.Fatal(err)
	}
	delivery := makeTestDelivery(t, hook.ID, wsID, func(d *storage.HookDelivery) {
		d.ResolvedURL = server.URL
	})
	if err := deliveryRepo.Enqueue([]storage.HookDelivery{delivery}); err != nil {
		t.Fatal(err)
	}

	shutdown := runtimeutil.NewShutdownCoordinator()
	shutdown.StopAccepting()
	dispatcher := NewDispatcher(DispatcherOptions{
		Store:    store,
		Clock:    testClock{now: 1000},
		Client:   server.Client(),
		Resolver: mockResolver{},
		Shutdown: shutdown,
	})
	if err := dispatcher.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("requests = %d, want 0", requests.Load())
	}
	got := getDelivery(t, store, delivery.ID)
	if got.Status != storage.DeliveryStatusQueued {
		t.Fatalf("status = %q, want queued", got.Status)
	}
	if got.AttemptCount != 0 {
		t.Fatalf("attempt_count = %d, want 0", got.AttemptCount)
	}
}

func TestHookDispatcherDrainWaitsAndRequeuesNotStartedDelivery(t *testing.T) {
	store := newTestStore(t)
	wsID := mustLocalWorkspace(t, store)
	hookRepo := storage.NewHookRepository(store.DB())
	deliveryRepo := storage.NewHookDeliveryRepository(store.DB())
	hook := makeTestHook(t, wsID, "https://example.com/webhook")
	if err := hookRepo.Create(hook); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, 2)
	for i := 0; i < 2; i++ {
		delivery := makeTestDelivery(t, hook.ID, wsID, func(d *storage.HookDelivery) {
			d.ResolvedURL = "https://example.com/webhook"
			d.CreatedAt = int64(100 + i)
			d.ModifiedAt = d.CreatedAt
		})
		ids = append(ids, delivery.ID)
		if err := deliveryRepo.Enqueue([]storage.HookDelivery{delivery}); err != nil {
			t.Fatal(err)
		}
	}

	started := make(chan struct{})
	release := make(chan struct{})
	shutdown := runtimeutil.NewShutdownCoordinator()
	dispatcher := NewDispatcher(DispatcherOptions{
		Store:          store,
		Clock:          testClock{now: 1000},
		Client:         &http.Client{Transport: blockingRoundTripper{started: started, release: release}},
		Resolver:       mockResolver{},
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

	first := getDelivery(t, store, ids[0])
	second := getDelivery(t, store, ids[1])
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

func TestHookDispatcherForceCancelCancelsStartedDelivery(t *testing.T) {
	store := newTestStore(t)
	wsID := mustLocalWorkspace(t, store)
	hookRepo := storage.NewHookRepository(store.DB())
	deliveryRepo := storage.NewHookDeliveryRepository(store.DB())
	hook := makeTestHook(t, wsID, "https://example.com/webhook")
	if err := hookRepo.Create(hook); err != nil {
		t.Fatal(err)
	}
	delivery := makeTestDelivery(t, hook.ID, wsID)
	if err := deliveryRepo.Enqueue([]storage.HookDelivery{delivery}); err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{})
	shutdown := runtimeutil.NewShutdownCoordinator()
	dispatcher := NewDispatcher(DispatcherOptions{
		Store:    store,
		Clock:    testClock{now: 1000},
		Client:   &http.Client{Transport: cancelingRoundTripper{started: started}},
		Resolver: mockResolver{},
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

	got := getDelivery(t, store, delivery.ID)
	if got.Status != storage.DeliveryStatusRetryWait {
		t.Fatalf("status = %q, want retry_wait", got.Status)
	}
	if !strings.Contains(got.LastError, "context canceled") {
		t.Fatalf("last_error = %q, want context canceled", got.LastError)
	}
}

func TestHookDispatcherSinkMaxConcurrencyBoundsInflightPerSink(t *testing.T) {
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
		once.Do(func() { close(release) })
		select {
		case <-release:
		case <-time.After(300 * time.Millisecond):
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := newTestStore(t)
	wsID := mustLocalWorkspace(t, store)
	sinkRepo := storage.NewNotificationSinkRepository(store.DB())
	if err := sinkRepo.Update(makeTestSink(t, wsID, server.URL, func(s *storage.NotificationSink) {
		s.ID = "sink-1"
		s.MaxConcurrency = 1
	})); err != nil {
		t.Fatal(err)
	}
	hookRepo := storage.NewHookRepository(store.DB())
	deliveryRepo := storage.NewHookDeliveryRepository(store.DB())
	hook := makeTestHook(t, wsID, server.URL)
	if err := hookRepo.Create(hook); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		delivery := makeTestDelivery(t, hook.ID, wsID, func(d *storage.HookDelivery) {
			d.ResolvedURL = server.URL
			d.CreatedAt = int64(100 + i)
			d.ModifiedAt = d.CreatedAt
		})
		if err := deliveryRepo.Enqueue([]storage.HookDelivery{delivery}); err != nil {
			t.Fatal(err)
		}
	}

	d := NewDispatcher(DispatcherOptions{Store: store, Clock: testClock{now: 1000}, Client: server.Client(), Resolver: mockResolver{}, MaxConcurrency: 4})
	if err := d.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if maxActive.Load() > 1 {
		t.Fatalf("max active requests = %d, want <= 1", maxActive.Load())
	}
}

func TestHookDispatcherDoesNotLeaveSinkLimitedDeliveriesDelivering(t *testing.T) {
	store := newTestStore(t)
	wsID := mustLocalWorkspace(t, store)
	sinkRepo := storage.NewNotificationSinkRepository(store.DB())
	if err := sinkRepo.Update(makeTestSink(t, wsID, "https://example.com/webhook", func(s *storage.NotificationSink) {
		s.ID = "sink-1"
		s.MaxConcurrency = 1
	})); err != nil {
		t.Fatal(err)
	}
	hookRepo := storage.NewHookRepository(store.DB())
	deliveryRepo := storage.NewHookDeliveryRepository(store.DB())
	hook := makeTestHook(t, wsID, "https://example.com/webhook")
	if err := hookRepo.Create(hook); err != nil {
		t.Fatal(err)
	}
	delivery := makeTestDelivery(t, hook.ID, wsID)
	if err := deliveryRepo.Enqueue([]storage.HookDelivery{delivery}); err != nil {
		t.Fatal(err)
	}
	limiter := runtimeutil.NewSinkLimiter()
	if !limiter.TryAcquire("sink-1", 1) {
		t.Fatal("pre-acquire sink token failed")
	}
	defer limiter.Release("sink-1")

	d := NewDispatcher(DispatcherOptions{Store: store, Clock: testClock{now: 1000}, SinkLimiter: limiter})
	if err := d.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	var delivering int64
	if err := store.DB().Model(&storage.HookDelivery{}).Where("status = ?", storage.DeliveryStatusDelivering).Count(&delivering).Error; err != nil {
		t.Fatal(err)
	}
	if delivering != 0 {
		t.Fatalf("delivering = %d, want 0", delivering)
	}
	got := getDelivery(t, store, delivery.ID)
	if got.Status != storage.DeliveryStatusQueued {
		t.Fatalf("status = %q, want queued", got.Status)
	}
}

func TestHookDispatcherRecoversStaleDeliveringOnRunOnce(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := newTestStore(t)
	wsID := mustLocalWorkspace(t, store)
	hookRepo := storage.NewHookRepository(store.DB())
	deliveryRepo := storage.NewHookDeliveryRepository(store.DB())
	hook := makeTestHook(t, wsID, server.URL)
	if err := hookRepo.Create(hook); err != nil {
		t.Fatal(err)
	}
	delivery := makeTestDelivery(t, hook.ID, wsID, func(d *storage.HookDelivery) {
		d.ResolvedURL = server.URL
		d.Status = storage.DeliveryStatusDelivering
		d.ClaimExpiresAt = int64Ptr(999)
		d.AttemptCount = 1
	})
	if err := deliveryRepo.Enqueue([]storage.HookDelivery{delivery}); err != nil {
		t.Fatal(err)
	}
	d := NewDispatcher(DispatcherOptions{Store: store, Clock: testClock{now: 1000}, Client: server.Client(), Resolver: mockResolver{}})
	if err := d.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if requests.Load() != 1 {
		t.Fatalf("requests = %d, want 1", requests.Load())
	}
	got := getDelivery(t, store, delivery.ID)
	if got.Status != storage.DeliveryStatusSucceeded {
		t.Fatalf("status = %q, want succeeded", got.Status)
	}
}

func TestHookDispatcherKeepsFutureDeliveringInvisible(t *testing.T) {
	store := newTestStore(t)
	wsID := mustLocalWorkspace(t, store)
	hookRepo := storage.NewHookRepository(store.DB())
	deliveryRepo := storage.NewHookDeliveryRepository(store.DB())
	hook := makeTestHook(t, wsID, "https://example.com/webhook")
	if err := hookRepo.Create(hook); err != nil {
		t.Fatal(err)
	}
	delivery := makeTestDelivery(t, hook.ID, wsID, func(d *storage.HookDelivery) {
		d.Status = storage.DeliveryStatusDelivering
		d.ClaimExpiresAt = int64Ptr(1200)
		d.AttemptCount = 1
	})
	if err := deliveryRepo.Enqueue([]storage.HookDelivery{delivery}); err != nil {
		t.Fatal(err)
	}
	d := NewDispatcher(DispatcherOptions{Store: store, Clock: testClock{now: 1000}})
	if err := d.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	got := getDelivery(t, store, delivery.ID)
	if got.Status != storage.DeliveryStatusDelivering {
		t.Fatalf("status = %q, want delivering", got.Status)
	}
}

func TestHookDispatcherRetryWaitSurvivesUntilNextAttempt(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := newTestStore(t)
	wsID := mustLocalWorkspace(t, store)
	hookRepo := storage.NewHookRepository(store.DB())
	deliveryRepo := storage.NewHookDeliveryRepository(store.DB())
	hook := makeTestHook(t, wsID, server.URL)
	if err := hookRepo.Create(hook); err != nil {
		t.Fatal(err)
	}
	delivery := makeTestDelivery(t, hook.ID, wsID, func(d *storage.HookDelivery) {
		d.ResolvedURL = server.URL
		d.Status = storage.DeliveryStatusRetryWait
		d.NextAttemptAt = int64Ptr(1200)
	})
	if err := deliveryRepo.Enqueue([]storage.HookDelivery{delivery}); err != nil {
		t.Fatal(err)
	}
	d := NewDispatcher(DispatcherOptions{Store: store, Clock: testClock{now: 1000}, Client: server.Client(), Resolver: mockResolver{}})
	if err := d.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("requests = %d, want 0", requests.Load())
	}
	got := getDelivery(t, store, delivery.ID)
	if got.Status != storage.DeliveryStatusRetryWait {
		t.Fatalf("status = %q, want retry_wait", got.Status)
	}
	if got.AttemptCount != 0 {
		t.Fatalf("attempt_count = %d, want 0", got.AttemptCount)
	}
}
