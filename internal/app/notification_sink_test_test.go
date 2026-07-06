package app

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

// loopbackResolver 解析所有主机名为 127.0.0.1，用于 sink test 命中 httptest.Server。
type loopbackResolver struct{}

func (loopbackResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
}

// newSinkTestService 构造一个 sink 测试专用 Service：
// 注入 loopback resolver + httptest.Server client，绕过生产 SSRF 对 loopback 的限制。
func newSinkTestService(t *testing.T, server *httptest.Server) (*Service, func()) {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(ServiceOptions{
		Store:            store,
		Clock:            FixedClock{NowUnix: 1000},
		SinkTestClient:   server.Client(),
		SinkTestResolver: loopbackResolver{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc, func() { _ = store.Close() }
}

func TestNotificationSinkTestStaticURLSucceeds(t *testing.T) {
	var receivedHeaders http.Header
	var receivedBody []byte
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedHeaders = r.Header.Clone()
		receivedBody, _ = io.ReadAll(r.Body)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	svc, cleanup := newSinkTestService(t, server)
	defer cleanup()

	sink, err := svc.AddNotificationSink(NotificationSinkAddInput{
		Name:         "test-sink",
		Type:         NotificationSinkTypeWebhook,
		EndpointMode: NotificationEndpointStaticURL,
		URL:          server.URL,
		Secret:       "webhook-secret",
	})
	if err != nil {
		t.Fatalf("AddNotificationSink() error = %v", err)
	}

	view, err := svc.TestNotificationSink(sink.ID, NotificationSinkTestInput{Kind: "hook", EventType: "task.completed"})
	if err != nil {
		t.Fatalf("TestNotificationSink() error = %v", err)
	}
	if view.Status != "succeeded" {
		t.Fatalf("status = %q, want succeeded", view.Status)
	}
	if view.StatusCode == nil || *view.StatusCode != 200 {
		t.Fatalf("StatusCode = %v, want 200", view.StatusCode)
	}
	if view.ResolvedEndpointSource != NotificationEndpointStaticURL {
		t.Fatalf("endpoint source = %q", view.ResolvedEndpointSource)
	}
	if view.RenderedMethod != http.MethodPost {
		t.Fatalf("rendered method = %q", view.RenderedMethod)
	}
	mu.Lock()
	if receivedHeaders.Get("X-Xuanchu-Test") != "true" {
		t.Errorf("X-Xuanchu-Test header missing")
	}
	if !strings.HasPrefix(receivedHeaders.Get("X-Xuanchu-Delivery"), "test-") {
		t.Errorf("X-Xuanchu-Delivery = %q", receivedHeaders.Get("X-Xuanchu-Delivery"))
	}
	if receivedHeaders.Get("X-Xuanchu-Signature-256") == "" {
		t.Errorf("signature missing for webhook sink with secret")
	}
	if !strings.Contains(string(receivedBody), "task.completed") {
		t.Errorf("body should contain event type, got %s", string(receivedBody))
	}
	mu.Unlock()
}

func TestNotificationSinkTestTargetReturns500(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	svc, cleanup := newSinkTestService(t, server)
	defer cleanup()

	sink, err := svc.AddNotificationSink(NotificationSinkAddInput{
		Name:         "fail-sink",
		Type:         NotificationSinkTypeWebhook,
		EndpointMode: NotificationEndpointStaticURL,
		URL:          server.URL,
	})
	if err != nil {
		t.Fatalf("AddNotificationSink() error = %v", err)
	}

	view, err := svc.TestNotificationSink(sink.ID, NotificationSinkTestInput{Kind: "hook", EventType: "task.completed"})
	if err != nil {
		t.Fatalf("TestNotificationSink() error = %v (target failure should not be app error)", err)
	}
	if view.Status != "failed" {
		t.Fatalf("status = %q, want failed", view.Status)
	}
	if view.StatusCode == nil || *view.StatusCode != 500 {
		t.Fatalf("StatusCode = %v, want 500", view.StatusCode)
	}
}

func TestNotificationSinkTestUnknownSinkNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()

	svc, cleanup := newSinkTestService(t, server)
	defer cleanup()

	_, err := svc.TestNotificationSink("nonexistent", NotificationSinkTestInput{})
	if err == nil {
		t.Fatal("expected error for unknown sink")
	}
	code := RuntimeErrorCode(err)
	if !strings.Contains(code, "not_found") {
		t.Fatalf("expected not_found error code, got %q (%v)", code, err)
	}
}

func TestNotificationSinkTestBlocksLoopbackInProduction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// 不注入 resolver，使用生产默认（loopback 被拒）
	store, err := storage.Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: 1000}})
	if err != nil {
		t.Fatal(err)
	}
	sink, err := svc.AddNotificationSink(NotificationSinkAddInput{
		Name:         "loopback-sink",
		Type:         NotificationSinkTypeWebhook,
		EndpointMode: NotificationEndpointStaticURL,
		URL:          server.URL,
	})
	if err != nil {
		t.Fatalf("AddNotificationSink() error = %v", err)
	}
	_, err = svc.TestNotificationSink(sink.ID, NotificationSinkTestInput{Kind: "hook", EventType: "task.completed"})
	if err == nil {
		t.Fatal("expected SSRF block error for loopback URL")
	}
	code := RuntimeErrorCode(err)
	if !strings.Contains(code, "endpoint") && !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("expected endpoint/blocked error, got %v", err)
	}
}

func TestNotificationSinkTestRejectsInvalidEventType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()
	svc, cleanup := newSinkTestService(t, server)
	defer cleanup()
	sink, err := svc.AddNotificationSink(NotificationSinkAddInput{
		Name:         "ev-sink",
		Type:         NotificationSinkTypeWebhook,
		EndpointMode: NotificationEndpointStaticURL,
		URL:          server.URL,
	})
	if err != nil {
		t.Fatalf("AddNotificationSink() error = %v", err)
	}
	_, err = svc.TestNotificationSink(sink.ID, NotificationSinkTestInput{Kind: "hook", EventType: "task.done"})
	if err == nil {
		t.Fatal("expected error for invalid event type")
	}
}

func TestNotificationSinkTestWritesAuditWithoutSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	svc, cleanup := newSinkTestService(t, server)
	defer cleanup()

	sink, err := svc.AddNotificationSink(NotificationSinkAddInput{
		Name:         "audit-sink",
		Type:         NotificationSinkTypeWebhook,
		EndpointMode: NotificationEndpointStaticURL,
		URL:          server.URL,
		Secret:       "super-secret-value",
	})
	if err != nil {
		t.Fatalf("AddNotificationSink() error = %v", err)
	}

	if _, err := svc.TestNotificationSink(sink.ID, NotificationSinkTestInput{Kind: "hook", EventType: "task.completed"}); err != nil {
		t.Fatalf("TestNotificationSink() error = %v", err)
	}

	// 直接查 audit repo，避免 permission 检查。
	targetType := "notification_sink"
	targetID := sink.ID
	rows, err := svc.auditRepo.List(storage.AuditListOptions{
		WorkspaceID: &svc.workspaceID,
		TargetType:  &targetType,
		TargetID:    &targetID,
		Limit:       20,
	})
	if err != nil {
		t.Fatalf("audit list error = %v", err)
	}
	found := false
	for _, e := range rows {
		if e.Action != "notification.sink.test" {
			continue
		}
		found = true
		if strings.Contains(e.PayloadJSON, "super-secret-value") {
			t.Errorf("audit payload leaked secret: %s", e.PayloadJSON)
		}
		var payload map[string]any
		_ = json.Unmarshal([]byte(e.PayloadJSON), &payload)
		if payload["secret"] != nil {
			t.Errorf("audit payload should not contain secret field")
		}
		if payload["status"] != "succeeded" {
			t.Errorf("audit status = %v", payload["status"])
		}
	}
	if !found {
		t.Fatal("notification.sink.test audit entry not found")
	}
}

// RuntimeErrorCode 从 error 中提取 RuntimeError 的 Code。
func RuntimeErrorCode(err error) string {
	if re, ok := err.(RuntimeError); ok {
		return re.Code
	}
	return ""
}

func TestNotificationSinkTestCrossWorkspaceNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()

	// 在默认 workspace 创建 sink。
	svc1, cleanup1 := newSinkTestService(t, server)
	defer cleanup1()
	sink, err := svc1.AddNotificationSink(NotificationSinkAddInput{
		Name:         "ws1-sink",
		Type:         NotificationSinkTypeWebhook,
		EndpointMode: NotificationEndpointStaticURL,
		URL:          server.URL,
	})
	if err != nil {
		t.Fatalf("AddNotificationSink() error = %v", err)
	}

	// 用一个全新 store + 不同 workspace 的 Service 尝试访问，
	// 应命中 row.WorkspaceID != s.workspaceID 守护分支，返回 not_found。
	store2, err := storage.Open(filepath.Join(t.TempDir(), "xuanchu2.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()
	svc2, err := NewService(ServiceOptions{
		Store:          store2,
		Clock:          FixedClock{NowUnix: 1000},
		SinkTestClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc2.TestNotificationSink(sink.ID, NotificationSinkTestInput{Kind: "hook", EventType: "task.completed"})
	if err == nil {
		t.Fatal("expected cross-workspace test to be rejected")
	}
	if !strings.Contains(RuntimeErrorCode(err), "not_found") {
		t.Fatalf("expected not_found, got %q (%v)", RuntimeErrorCode(err), err)
	}
}
