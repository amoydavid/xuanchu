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

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/hookruntime"
	"git.dajee.net/dajee/xuanchu/internal/storage"
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
	sink := storage.NotificationSink{
		ID:                  uuid.NewString(),
		WorkspaceID:         h.wsID,
		Name:                "e2e-sink-" + uuid.NewString()[:8],
		Type:                app.NotificationSinkTypeWebhook,
		EndpointMode:        app.NotificationEndpointStaticURL,
		URL:                 endpointURL,
		AllowedHostsJSON:    `["127.0.0.1","localhost","example.com"]`,
		HTTPMethod:          "POST",
		HeaderTemplatesJSON: `[]`,
		SecretRefsJSON:      `{}`,
		Secret:              "e2e-secret",
		Enabled:             &enabled,
		TimeoutSeconds:      10,
		MaxAttempts:         5,
		CreatedBy:           h.userID,
		CreatedAt:           100,
		ModifiedAt:          100,
	}
	if err := storage.NewNotificationSinkRepository(h.store.DB()).Create(sink); err != nil {
		t.Fatal(err)
	}
	row := storage.HookDefinition{
		ID:             uuid.NewString(),
		Name:           "e2e-hook",
		ScopeType:      "workspace",
		WorkspaceID:    h.wsID,
		ActorUserID:    h.userID,
		EventTypesJSON: string(typesJSON),
		SinkID:         sink.ID,
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
	deliveries, err := h.svc.ListHookDeliveries(hook.ID, "", 10, 0)
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
	if v := req.Header.Get("Content-Type"); v != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", v)
	}
	if v := req.Header.Get("X-Xuanchu-Event"); v != "task.created" {
		t.Fatalf("X-Xuanchu-Event = %q, want task.created", v)
	}
	if v := req.Header.Get("X-Xuanchu-Delivery"); v == "" {
		t.Fatal("X-Xuanchu-Delivery header is empty")
	}
	if v := req.Header.Get("X-Xuanchu-Hook-Id"); v != hook.ID {
		t.Fatalf("X-Xuanchu-Hook-Id = %q, want %q", v, hook.ID)
	}
	if v := req.Header.Get("X-Xuanchu-Attempt"); v != "1" {
		t.Fatalf("X-Xuanchu-Attempt = %q, want 1", v)
	}
	if v := req.Header.Get("User-Agent"); !strings.HasPrefix(v, "xuanchu-webhook/") {
		t.Fatalf("User-Agent = %q, want xuanchu-webhook/*", v)
	}
	// secret 不为空时应有签名 headers
	if v := req.Header.Get("X-Xuanchu-Timestamp"); v == "" {
		t.Fatal("expected X-Xuanchu-Timestamp header")
	}
	if v := req.Header.Get("X-Xuanchu-Signature-256"); v == "" {
		t.Fatal("expected X-Xuanchu-Signature-256 header")
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
	deliveriesAfter, err := h.svc.ListHookDeliveries(hook.ID, "", 10, 0)
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
		Store: h.store, Clock: h.clock, Client: webhookTarget.Client(), Resolver: publicTestResolver{}, MaxConcurrency: 2,
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
	proj, _ := h.svc.AddProject(app.AddProjectInput{Slug: "e2eproj", Name: "E2E Project"})
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
	if projData["slug"] != "e2eproj" {
		t.Fatalf("project slug = %v, want e2eproj", projData["slug"])
	}
}

// ---------------------------------------------------------------------------
// E2E: project.transitioned 事件 + 转 archived 双事件向后兼容
// ---------------------------------------------------------------------------

func TestHookEndToEndProjectTransitionedPayload(t *testing.T) {
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

	h.createHookDirect(t, webhookTarget.URL, []string{"project.transitioned", "project.archived"})

	proj, _ := h.svc.AddProject(app.AddProjectInput{Slug: "trproj", Name: "TR Project"})
	_, _ = h.svc.TransitionProject(proj.Slug, "active")
	_, _ = h.svc.TransitionProject(proj.Slug, "archived")

	dispatcher := hookruntime.NewDispatcher(hookruntime.DispatcherOptions{
		Store: h.store, Clock: h.clock, Client: webhookTarget.Client(), Resolver: publicTestResolver{},
	})
	// 多次 RunOnce 确保所有 due delivery 投递完成
	for i := 0; i < 4; i++ {
		_ = dispatcher.RunOnce(context.Background())
	}

	mu.Lock()
	defer mu.Unlock()

	eventCounts := map[string]int{}
	var transitionedPayload map[string]any
	for _, body := range receivedBodies {
		var p map[string]any
		json.Unmarshal([]byte(body), &p)
		et, _ := p["event_type"].(string)
		eventCounts[et]++
		if et == "project.transitioned" {
			transitionedPayload = p
		}
	}
	if eventCounts["project.transitioned"] == 0 {
		t.Fatal("no project.transitioned event received")
	}
	if eventCounts["project.archived"] == 0 {
		t.Fatal("no project.archived event received (向后兼容)")
	}
	if transitionedPayload == nil {
		t.Fatal("no project.transitioned payload received")
	}
	data, _ := transitionedPayload["data"].(map[string]any)
	if data["from_status"] == nil || data["to_status"] == nil {
		t.Fatalf("transitioned payload missing from_status/to_status: %#v", data)
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

	secret := "e2e-secret"
	h.createHookDirect(t, webhookTarget.URL, []string{"task.created"})

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
	createHTTPTestSink(t, svc, "hook-sink")
	hook, err := svc.AddHook(app.HookAddInput{
		Name: "delivery-secret-hook", ScopeType: app.HookScopeWorkspace,
		EventTypes: []string{"task.created"}, SinkRef: "hook-sink",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, _ = svc.Add(app.AddInput{Description: "delivery secret test"})

	deliveries, _ := svc.ListHookDeliveries(hook.ID, "", 10, 0)
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
	sink := storage.NotificationSink{
		ID:                  uuid.NewString(),
		WorkspaceID:         wsID,
		Name:                "http-e2e-sink",
		Type:                app.NotificationSinkTypeWebhook,
		EndpointMode:        app.NotificationEndpointStaticURL,
		URL:                 webhookTarget.URL,
		AllowedHostsJSON:    `["127.0.0.1","localhost"]`,
		HTTPMethod:          "POST",
		HeaderTemplatesJSON: `[]`,
		SecretRefsJSON:      `{}`,
		Secret:              "http-secret",
		Enabled:             &enabled,
		TimeoutSeconds:      10,
		MaxAttempts:         5,
		CreatedBy:           userID,
		CreatedAt:           100,
		ModifiedAt:          100,
	}
	if err := storage.NewNotificationSinkRepository(fixture.server.store.DB()).Create(sink); err != nil {
		t.Fatal(err)
	}
	hook := storage.HookDefinition{
		ID: uuid.NewString(), Name: "http-e2e-hook", ScopeType: "workspace",
		WorkspaceID: wsID, ActorUserID: userID,
		EventTypesJSON: string(typesJSON), SinkID: sink.ID,
		Enabled: &enabled, TimeoutSeconds: 10, MaxAttempts: 5,
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
// Hook 不再接受直接 URL；SSRF 校验由 workspace sink 负责。
// ---------------------------------------------------------------------------

func TestHookDirectURLRejectedViaHTTPAPI(t *testing.T) {
	fixture := newHTTPServerWithTokenFixture(t, "hook:write")
	auth := map[string]string{"Authorization": "Bearer " + fixture.token, "Content-Type": "application/json"}

	cases := []struct {
		name string
		body string
	}{
		{"url", `{"name":"url-hook","scope_type":"workspace","event_types":["task.created"],"url":"https://example.com/webhook"}`},
		{"endpoint_url", `{"name":"endpoint-hook","scope_type":"workspace","event_types":["task.created"],"endpoint_url":"https://example.com/webhook"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/hooks", tc.body, auth)
			assertHTTPErrorCode(t, rr, http.StatusBadRequest, "hook_url_not_supported")
		})
	}
}
