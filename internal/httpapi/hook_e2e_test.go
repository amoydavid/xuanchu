package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/hookruntime"
	"github.com/dajee/taskg/internal/storage"
	"github.com/google/uuid"
)

// publicTestResolver 用于 E2E 测试，让 dispatcher 的 SSRF 检查通过。
type publicTestResolver struct{}

func (publicTestResolver) LookupIPAddr(_ context.Context, _ string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
}

// e2eTestHelper 创建 E2E 测试所需的公共状态。
type e2eTestHelper struct {
	store  *storage.Store
	svc    *app.Service
	clock  app.FixedClock
	wsID   string
	userID string
}

func newE2EHelper(t *testing.T) *e2eTestHelper {
	t.Helper()
	store := openHTTPTestStore(t)
	clock := app.FixedClock{NowUnix: 1000}
	svc, err := app.NewService(app.ServiceOptions{Store: store, Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	rt := svc.Runtime()
	return &e2eTestHelper{store: store, svc: svc, clock: clock, wsID: rt.WorkspaceID, userID: rt.ActorUserID}
}

// createHookDirect 通过 repo 直接创建 hook，绕过 SSRF 验证。
// httptest.Server 监听 127.0.0.1，SSRF 检查会拒绝它。
func (h *e2eTestHelper) createHookDirect(t *testing.T, endpointURL string, eventTypes []string, overrides ...func(*storage.HookDefinition)) storage.HookDefinition {
	t.Helper()
	enabled := true
	typesJSON, _ := json.Marshal(eventTypes)
	row := storage.HookDefinition{
		ID:             uuid.NewString(),
		Name:           "e2e-hook",
		ScopeType:      "workspace",
		WorkspaceID:    h.wsID,
		ActorUserID:    h.userID,
		EventTypesJSON: string(typesJSON),
		EndpointURL:    endpointURL,
		Secret:         "e2e-secret",
		Enabled:        &enabled,
		TimeoutSeconds: 10,
		MaxAttempts:    5,
		CreatedAt:      100,
		ModifiedAt:     100,
	}
	for _, fn := range overrides {
		fn(&row)
	}
	if err := storage.NewHookRepository(h.store.DB()).Create(row); err != nil {
		t.Fatal(err)
	}
	return row
}

// ---------------------------------------------------------------------------
// E2E: 完整的 Hook 端到端测试
// 创建 hook -> 创建任务 -> dispatcher 投递 -> webhook target 收到
// ---------------------------------------------------------------------------

func TestHookEndToEnd(t *testing.T) {
	h := newE2EHelper(t)

	// 创建 httptest.Server 作为 webhook target
	var mu sync.Mutex
	var receivedRequests []http.Request
	var receivedBodies []string
	webhookTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		receivedBodies = append(receivedBodies, string(body))
		receivedRequests = append(receivedRequests, *r)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookTarget.Close()

	// 通过 repo 直接创建 hook（绕过 SSRF，因为 httptest.Server 是 localhost）
	hook := h.createHookDirect(t, webhookTarget.URL, []string{"task.created"})

	// 通过 app service 创建任务
	created, err := h.svc.Add(app.AddInput{Description: "e2e test task"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// 验证 delivery 已入队
	deliveries, err := h.svc.ListHookDeliveries(hook.ID, "", 10)
	if err != nil {
		t.Fatalf("ListHookDeliveries() error = %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("deliveries count = %d, want 1", len(deliveries))
	}
	d := deliveries[0]
	if d.EventType != "task.created" {
		t.Fatalf("event_type = %q, want task.created", d.EventType)
	}
	if d.Status != "queued" {
		t.Fatalf("status = %q, want queued", d.Status)
	}

	// 运行 dispatcher
	dispatcher := hookruntime.NewDispatcher(hookruntime.DispatcherOptions{
		Store:    h.store,
		Clock:    h.clock,
		Client:   webhookTarget.Client(),
		Resolver: publicTestResolver{},
	})
	if err := dispatcher.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	// 验证 webhook target 收到了请求
	mu.Lock()
	defer mu.Unlock()
	if len(receivedRequests) != 1 {
		t.Fatalf("received requests = %d, want 1", len(receivedRequests))
	}

	// 验证 headers
	req := receivedRequests[0]
	if v := req.Header.Get("Content-Type"); v != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want application/json; charset=utf-8", v)
	}
	if v := req.Header.Get("X-Taskg-Event"); v != "task.created" {
		t.Fatalf("X-Taskg-Event = %q, want task.created", v)
	}
	if v := req.Header.Get("X-Taskg-Delivery"); v == "" {
		t.Fatal("X-Taskg-Delivery header is empty")
	}
	if v := req.Header.Get("X-Taskg-Hook-Id"); v != hook.ID {
		t.Fatalf("X-Taskg-Hook-Id = %q, want %q", v, hook.ID)
	}
	if v := req.Header.Get("X-Taskg-Attempt"); v != "1" {
		t.Fatalf("X-Taskg-Attempt = %q, want 1", v)
	}
	if v := req.Header.Get("User-Agent"); !strings.HasPrefix(v, "taskg-webhook/") {
		t.Fatalf("User-Agent = %q, want taskg-webhook/*", v)
	}
	// secret 不为空时应有签名 headers
	if v := req.Header.Get("X-Taskg-Timestamp"); v == "" {
		t.Fatal("expected X-Taskg-Timestamp header")
	}
	if v := req.Header.Get("X-Taskg-Signature-256"); v == "" {
		t.Fatal("expected X-Taskg-Signature-256 header")
	}

	// 验证 body 是合法 JSON 且包含 task.created 事件和任务快照
	var payload map[string]any
	if err := json.Unmarshal([]byte(receivedBodies[0]), &payload); err != nil {
		t.Fatalf("body JSON parse error: %v", err)
	}
	if payload["event_type"] != "task.created" {
		t.Fatalf("payload event_type = %v, want task.created", payload["event_type"])
	}
	if payload["object_id"] != created.UUID {
		t.Fatalf("payload object_id = %v, want %s", payload["object_id"], created.UUID)
	}
	if payload["object_kind"] != "task" {
		t.Fatalf("payload object_kind = %v, want task", payload["object_kind"])
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("payload data type = %T, want map[string]any", payload["data"])
	}
	taskData, ok := data["task"].(map[string]any)
	if !ok {
		t.Fatalf("data.task type = %T, want map[string]any", data["task"])
	}
	if taskData["uuid"] != created.UUID {
		t.Fatalf("task.uuid = %v, want %s", taskData["uuid"], created.UUID)
	}
	if taskData["description"] != "e2e test task" {
		t.Fatalf("task.description = %v, want e2e test task", taskData["description"])
	}

	// 验证 delivery 状态变为 succeeded
	deliveriesAfter, err := h.svc.ListHookDeliveries(hook.ID, "", 10)
	if err != nil {
		t.Fatalf("ListHookDeliveries() after dispatch error = %v", err)
	}
	if deliveriesAfter[0].Status != "succeeded" {
		t.Fatalf("delivery status after dispatch = %q, want succeeded", deliveriesAfter[0].Status)
	}
}

// ---------------------------------------------------------------------------
// E2E: Payload 快照稳定性 - task.modified 事件
// ---------------------------------------------------------------------------

func TestHookEndToEndModifiedPayloadStability(t *testing.T) {
	h := newE2EHelper(t)

	var mu sync.Mutex
	var receivedBodies []string
	webhookTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		receivedBodies = append(receivedBodies, string(body))
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookTarget.Close()

	h.createHookDirect(t, webhookTarget.URL, []string{"task.created", "task.modified"})

	// 创建并修改任务
	created, _ := h.svc.Add(app.AddInput{Description: "original"})
	modified := "modified"
	_ = h.svc.Modify(created.UUID, app.ModifyInput{Description: &modified})

	dispatcher := hookruntime.NewDispatcher(hookruntime.DispatcherOptions{
		Store: h.store, Clock: h.clock, Client: webhookTarget.Client(), Resolver: publicTestResolver{},
	})
	_ = dispatcher.RunOnce(context.Background())

	mu.Lock()
	defer mu.Unlock()

	// 找到 task.modified 事件的 payload
	var modifiedPayload map[string]any
	for _, body := range receivedBodies {
		var p map[string]any
		json.Unmarshal([]byte(body), &p)
		if p["event_type"] == "task.modified" {
			modifiedPayload = p
			break
		}
	}
	if modifiedPayload == nil {
		t.Fatal("task.modified event not found in received payloads")
	}

	data := modifiedPayload["data"].(map[string]any)
	// task.modified payload 必须包含 task、completed、deleted 字段
	if _, ok := data["task"]; !ok {
		t.Fatal("task.modified payload missing 'task' field in data")
	}
	if _, ok := data["completed"]; !ok {
		t.Fatal("task.modified payload missing 'completed' field in data")
	}
	if _, ok := data["deleted"]; !ok {
		t.Fatal("task.modified payload missing 'deleted' field in data")
	}
	// 任务未完成、未删除
	if data["completed"] != false {
		t.Fatalf("completed = %v, want false", data["completed"])
	}
	if data["deleted"] != false {
		t.Fatalf("deleted = %v, want false", data["deleted"])
	}
}

// ---------------------------------------------------------------------------
// E2E: Payload 快照稳定性 - project.archived 事件
// ---------------------------------------------------------------------------

func TestHookEndToEndProjectArchivedPayloadStability(t *testing.T) {
	h := newE2EHelper(t)

	var mu sync.Mutex
	var receivedBodies []string
	webhookTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		receivedBodies = append(receivedBodies, string(body))
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookTarget.Close()

	h.createHookDirect(t, webhookTarget.URL, []string{"project.archived"})

	// 创建并归档 project
	proj, _ := h.svc.AddProject(app.AddProjectInput{Slug: "e2e-proj", Name: "E2E Project"})
	_, _ = h.svc.ArchiveProject(proj.Slug)

	dispatcher := hookruntime.NewDispatcher(hookruntime.DispatcherOptions{
		Store: h.store, Clock: h.clock, Client: webhookTarget.Client(), Resolver: publicTestResolver{},
	})
	_ = dispatcher.RunOnce(context.Background())

	mu.Lock()
	defer mu.Unlock()

	if len(receivedBodies) != 1 {
		t.Fatalf("received bodies = %d, want 1", len(receivedBodies))
	}
	var payload map[string]any
	json.Unmarshal([]byte(receivedBodies[0]), &payload)
	if payload["event_type"] != "project.archived" {
		t.Fatalf("event_type = %v, want project.archived", payload["event_type"])
	}
	if payload["object_kind"] != "project" {
		t.Fatalf("object_kind = %v, want project", payload["object_kind"])
	}
	data := payload["data"].(map[string]any)
	if data["archived"] != true {
		t.Fatalf("archived = %v, want true", data["archived"])
	}
	projData := data["project"].(map[string]any)
	if projData["slug"] != "e2e-proj" {
		t.Fatalf("project slug = %v, want e2e-proj", projData["slug"])
	}
}

// ---------------------------------------------------------------------------
// Secret 泄露测试: webhook payload 和 headers 不包含 secret 原文
// ---------------------------------------------------------------------------

func TestHookSecretNotInWebhookPayload(t *testing.T) {
	h := newE2EHelper(t)

	var mu sync.Mutex
	var receivedBodies []string
	var receivedHeaders []http.Header
	webhookTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		receivedBodies = append(receivedBodies, string(body))
		receivedHeaders = append(receivedHeaders, r.Header.Clone())
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookTarget.Close()

	secret := "super-secret-value-12345"
	h.createHookDirect(t, webhookTarget.URL, []string{"task.created"}, func(hd *storage.HookDefinition) {
		hd.Secret = secret
	})

	_, _ = h.svc.Add(app.AddInput{Description: "secret test task"})

	dispatcher := hookruntime.NewDispatcher(hookruntime.DispatcherOptions{
		Store: h.store, Clock: h.clock, Client: webhookTarget.Client(), Resolver: publicTestResolver{},
	})
	_ = dispatcher.RunOnce(context.Background())

	mu.Lock()
	defer mu.Unlock()

	if len(receivedBodies) == 0 {
		t.Fatal("no webhook received")
	}

	// payload 中不应包含 secret 原文
	if strings.Contains(receivedBodies[0], secret) {
		t.Fatalf("webhook payload contains secret: %s", receivedBodies[0])
	}

	// headers 中不应包含 secret 原文
	if len(receivedHeaders) > 0 {
		for k, vs := range receivedHeaders[0] {
			for _, v := range vs {
				if strings.Contains(v, secret) {
					t.Fatalf("header %q contains secret: %s", k, v)
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Secret 泄露测试: delivery 的 payload 和 headers 不包含 secret
// ---------------------------------------------------------------------------

func TestHookSecretNotInDeliveryView(t *testing.T) {
	store := openHTTPTestStore(t)

	secret := "delivery-secret-999"
	svc, err := app.NewService(app.ServiceOptions{Store: store, Clock: app.FixedClock{NowUnix: 1000}})
	if err != nil {
		t.Fatal(err)
	}
	hook, err := svc.AddHook(app.HookAddInput{
		Name: "delivery-secret-hook", ScopeType: app.HookScopeWorkspace,
		EventTypes: []string{"task.created"}, EndpointURL: "https://example.com/hook",
		Secret: secret,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, _ = svc.Add(app.AddInput{Description: "delivery secret test"})

	deliveries, _ := svc.ListHookDeliveries(hook.ID, "", 10)
	if len(deliveries) != 1 {
		t.Fatalf("deliveries = %d, want 1", len(deliveries))
	}

	d := deliveries[0]
	// 序列化整个 delivery view 然后检查不包含 secret
	viewJSON, _ := json.Marshal(d)
	if strings.Contains(string(viewJSON), secret) {
		t.Fatalf("delivery view JSON contains secret: %s", viewJSON)
	}

	// 检查 payload 字段
	payloadJSON, _ := json.Marshal(d.Payload)
	if strings.Contains(string(payloadJSON), secret) {
		t.Fatalf("delivery payload contains secret: %s", payloadJSON)
	}

	// 检查 headers 字段
	headersJSON, _ := json.Marshal(d.Headers)
	if strings.Contains(string(headersJSON), secret) {
		t.Fatalf("delivery headers contains secret: %s", headersJSON)
	}
}

// ---------------------------------------------------------------------------
// E2E: 通过 HTTP API 创建任务触发 Hook（HTTP 层 E2E）
// ---------------------------------------------------------------------------

func TestHookEndToEndViaHTTPAPI(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "hook:write", "hook:read", "task:write", "task:read")
	auth := map[string]string{"Authorization": "Bearer " + fixture.token, "Content-Type": "application/json"}

	var mu sync.Mutex
	var receivedBodies []string
	webhookTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		receivedBodies = append(receivedBodies, string(body))
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookTarget.Close()

	// 通过 repo 直接创建 hook（绕过 SSRF 验证）
	wsID := getFirstWorkspaceID(t, fixture.server.store)
	userID := getFirstUserID(t, fixture.server.store)
	enabled := true
	typesJSON, _ := json.Marshal([]string{"task.created"})
	hook := storage.HookDefinition{
		ID: uuid.NewString(), Name: "http-e2e-hook", ScopeType: "workspace",
		WorkspaceID: wsID, ActorUserID: userID,
		EventTypesJSON: string(typesJSON), EndpointURL: webhookTarget.URL,
		Secret: "http-secret", Enabled: &enabled, TimeoutSeconds: 10, MaxAttempts: 5,
		CreatedAt: 100, ModifiedAt: 100,
	}
	if err := storage.NewHookRepository(fixture.server.store.DB()).Create(hook); err != nil {
		t.Fatal(err)
	}

	// 通过 HTTP API 创建任务
	taskBody := `{"description":"http e2e task"}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/tasks", taskBody, auth)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create task status = %d body=%s", rr.Code, rr.Body.String())
	}

	// 运行 dispatcher
	dispatcher := hookruntime.NewDispatcher(hookruntime.DispatcherOptions{
		Store: fixture.server.store, Clock: app.FixedClock{NowUnix: 1000}, Client: webhookTarget.Client(), Resolver: publicTestResolver{},
	})
	_ = dispatcher.RunOnce(context.Background())

	// 验证 webhook 收到了 task.created 事件
	mu.Lock()
	defer mu.Unlock()
	if len(receivedBodies) != 1 {
		t.Fatalf("received bodies = %d, want 1", len(receivedBodies))
	}
	var payload map[string]any
	json.Unmarshal([]byte(receivedBodies[0]), &payload)
	if payload["event_type"] != "task.created" {
		t.Fatalf("event_type = %v, want task.created", payload["event_type"])
	}
	// webhook payload 不包含 secret
	if strings.Contains(receivedBodies[0], "http-secret") {
		t.Fatalf("webhook payload contains secret: %s", receivedBodies[0])
	}
}

// ---------------------------------------------------------------------------
// SSRF 测试: 通过 HTTP API 创建 hook 时拒绝私有地址
// ---------------------------------------------------------------------------

func TestHookEndpointSSRFViaHTTPAPI(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "hook:write")
	auth := map[string]string{"Authorization": "Bearer " + fixture.token, "Content-Type": "application/json"}

	ssrfURLs := []struct {
		name string
		url  string
	}{
		{"loopback", "https://localhost/webhook"},
		{"127.0.0.1", "https://127.0.0.1/webhook"},
		{"10.x", "https://10.0.0.1/webhook"},
		{"172.16.x", "https://172.16.0.1/webhook"},
		{"192.168.x", "https://192.168.1.1/webhook"},
		{"169.254.x", "https://169.254.169.254/webhook"},
		{"100.64.x", "https://100.64.0.1/webhook"},
	}

	for _, tc := range ssrfURLs {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"name":"ssrf-hook","scope_type":"workspace","event_types":["task.created"],"endpoint_url":"` + tc.url + `"}`
			rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/hooks", body, auth)
			assertHTTPErrorCode(t, rr, http.StatusBadRequest, "hook_endpoint_invalid")
		})
	}
}
