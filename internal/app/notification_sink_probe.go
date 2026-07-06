package app

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

// NotificationSinkTestKind 决定 sink 测试投递使用的 envelope 样例类型。
const (
	NotificationSinkTestKindHook         = "hook"
	NotificationSinkTestKindNotification = "notification"
)

// NotificationSinkTestInput 是 sink 测试投递的输入。
type NotificationSinkTestInput struct {
	Kind       string
	EventType  string
	Sample     string // 首版只支持 "default"
	ProjectRef string
}

// NotificationSinkTestView 是 sink 测试投递的结果。
type NotificationSinkTestView struct {
	Status                      string
	StatusCode                  *int
	DurationMS                  int64
	ResolvedEndpointSource      string
	ResolvedEndpointFingerprint string
	RenderedMethod              string
	RenderedHeaders             map[string][]string
	RenderedBodyPreview         string
	Error                       string
}

// sinkTestSender 抽象 sink 测试投递的 HTTP 发送，便于测试注入。
type sinkTestSender interface {
	Do(req *http.Request) (*http.Response, error)
}

// TestNotificationSink 对目标 sink 执行一次真实测试投递。
//
// 与正式 dispatch 不同：
//   - 不写入 hook_deliveries / notification_deliveries。
//   - 写一条 notification.sink.test audit；payload 不包含 secret / 完整 body。
//   - 请求头加 X-Xuanchu-Test: true，便于接收方识别和幂等。
//   - 仍受 SSRF / allowed hosts / endpoint 校验保护。
func (s *Service) TestNotificationSink(sinkID string, input NotificationSinkTestInput) (NotificationSinkTestView, error) {
	if err := s.Require(PermissionNotificationWrite); err != nil {
		return NotificationSinkTestView{}, err
	}

	kind := strings.TrimSpace(input.Kind)
	if kind == "" {
		kind = NotificationSinkTestKindHook
	}
	if kind != NotificationSinkTestKindHook && kind != NotificationSinkTestKindNotification {
		return NotificationSinkTestView{}, RuntimeError{Code: "notification_sink_test_invalid", Message: "kind must be hook or notification"}
	}
	eventType := strings.TrimSpace(input.EventType)
	if eventType == "" {
		eventType = "task.completed"
	}
	if kind == NotificationSinkTestKindHook {
		if !allowedHookEventTypes[eventType] {
			return NotificationSinkTestView{}, RuntimeError{Code: "notification_sink_test_invalid", Message: "event_type not allowed: " + eventType}
		}
	}

	// 解析 sink 并校验 workspace 隔离。
	row, err := s.notificationSinkRepo.GetByID(sinkID)
	if err == storage.ErrNotFound {
		return NotificationSinkTestView{}, RuntimeError{Code: "notification_sink_not_found", Message: "notification sink not found"}
	}
	if err != nil {
		return NotificationSinkTestView{}, err
	}
	if row.WorkspaceID != s.workspaceID {
		return NotificationSinkTestView{}, RuntimeError{Code: "notification_sink_not_found", Message: "notification sink not found"}
	}

	// 可选 project_ref：用于 config_value endpoint 在 project config 下的解析。
	var projectID *string
	if strings.TrimSpace(input.ProjectRef) != "" {
		project, err := s.ResolveProject(strings.TrimSpace(input.ProjectRef))
		if err != nil {
			return NotificationSinkTestView{}, err
		}
		projectID = &project.ID
	}

	// 加载 scoped config / secret values（与正式 scheduler 同路径）。
	configValues, secretValues, err := schedulerNotificationConfigValues(s.configRepo, s.configDefRepo, s.workspaceID, projectID, row)
	if err != nil {
		return NotificationSinkTestView{}, err
	}

	sinkView := notificationSinkViewFromRow(row, actorInfoFromColumns(notificationSinkActorColumns(row), row.CreatedBy, nil))

	workspace, err := s.workspaceRepo.GetByID(s.workspaceID)
	if err != nil {
		return NotificationSinkTestView{}, err
	}

	// 构造样例 resolve input。字段使用稳定的样例值，避免泄漏真实数据。
	deliveryID := "test-" + uuid.NewString()
	eventID := "test-" + uuid.NewString()
	resolveInput := NotificationRequestResolveInput{
		Sink: sinkView,
		Workspace: NotificationWorkspaceContext{
			ID:   workspace.ID,
			Slug: workspace.Slug,
			Name: workspace.Name,
		},
		Rule:      NotificationRuleContext{ID: "test", Name: "sink-test", TriggerType: "manual"},
		Recipient: task.UserInfo{ID: "test", Name: "sink-test"},
		Actor:     task.UserInfo{ID: s.runtime.ActorUserID, Name: s.runtime.ActorName},
		Event: NotificationEventContext{
			ID:         eventID,
			Type:       eventType,
			Version:    1,
			ObjectKind: "task",
			ObjectID:   "test-task",
			OccurredAt: s.clock.Unix(),
		},
		EventType: eventType,
		Delivery: NotificationDeliveryContext{
			ID:          deliveryID,
			Attempt:     1,
			WorkspaceID: s.workspaceID,
			SinkID:      sinkID,
		},
		Object:       NotificationObjectContext{Kind: "task", ID: "test-task"},
		ConfigValues: configValues,
		SecretValues: secretValues,
	}

	resolved, err := ResolveNotificationRequest(resolveInput)
	if err != nil {
		return NotificationSinkTestView{}, err
	}

	// SSRF 防护：对 resolved URL 再做一次解析+IP 校验（与正式 dispatch 一致）。
	// 仅当未注入自定义 client 时执行（生产路径使用默认 SSRF-safe client）；
	// 注入了 client 的测试场景由测试方负责目标可达性。
	if s.sinkTestClient == nil {
		if err := s.validateSinkTestEndpointURL(resolved.ResolvedURL); err != nil {
			return NotificationSinkTestView{}, err
		}
	}

	view, sendErr := s.sendNotificationSinkTest(resolved, sinkView, row, deliveryID, s.clock.Unix())
	view.ResolvedEndpointSource = resolved.ResolvedEndpointSource
	view.ResolvedEndpointFingerprint = resolved.ResolvedEndpointFingerprint
	view.RenderedMethod = resolved.RenderedMethod
	view.RenderedHeaders = decodeRenderedHeaders(resolved.RenderedHeadersJSON)
	view.RenderedBodyPreview = truncateBodyPreview(resolved.RenderedBody)

	// 写 audit（不包含 secret / 完整 body）。
	auditPayload := map[string]any{
		"kind":               kind,
		"event_type":         eventType,
		"status":             view.Status,
		"endpoint_source":    view.ResolvedEndpointSource,
		"endpoint_fingerprint": view.ResolvedEndpointFingerprint,
	}
	if view.StatusCode != nil {
		auditPayload["status_code"] = *view.StatusCode
	}
	if view.Error != "" {
		auditPayload["error"] = view.Error
	}
	if projectID != nil {
		auditPayload["project_ref"] = input.ProjectRef
	}
	if err := s.withAudit("notification.sink.test", func(tx *Service) (AuditEntry, error) {
		return AuditEntry{
			TargetType: "notification_sink",
			TargetID:   sinkID,
			Payload:    auditPayload,
		}, nil
	}); err != nil {
		return view, err
	}

	if sendErr != nil {
		return view, sendErr
	}
	return view, nil
}

// validateSinkTestEndpointURL 校验 resolved URL，使用 sink test 解析器（默认与生产一致）。
func (s *Service) validateSinkTestEndpointURL(raw string) error {
	resolver := s.sinkTestResolver
	if resolver == nil {
		return ValidateWebhookEndpointURLWithDefault(raw)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return ValidateWebhookEndpointURL(ctx, raw, resolver)
}

// sendNotificationSinkTest 执行真实 HTTP 投递并填充返回 view。
func (s *Service) sendNotificationSinkTest(resolved NotificationResolvedRequest, sinkView NotificationSinkView, sink storage.NotificationSink, deliveryID string, now int64) (NotificationSinkTestView, error) {
	resolver := s.sinkTestResolver
	if resolver == nil {
		resolver = defaultHookResolver
	}
	client := s.sinkTestClient
	if client == nil {
		client = defaultSinkTestClient(resolver)
	}

	timeout := sinkView.TimeoutSeconds
	if timeout <= 0 {
		timeout = 10
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	body := []byte(resolved.RenderedBody)
	req, err := http.NewRequestWithContext(ctx, resolved.RenderedMethod, resolved.ResolvedURL, bytes.NewReader(body))
	if err != nil {
		return NotificationSinkTestView{Status: "failed", Error: err.Error()}, nil
	}
	// 渲染 headers。
	for k, vs := range decodeRenderedHeaders(resolved.RenderedHeadersJSON) {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	// 标记为测试投递，并附带签名（与正式 dispatch 一致，便于接收方验证）。
	req.Header.Set("X-Xuanchu-Test", "true")
	req.Header.Set("X-Xuanchu-Delivery", deliveryID)
	req.Header.Set("X-Xuanchu-Event", "sink.test")
	req.Header.Set("User-Agent", "xuanchu-sink-test")
	if sink.Type == NotificationSinkTypeWebhook && sink.Secret != "" {
		// webhook secret 从 storage row 读取（view 不导出 secret）。
		req.Header.Set("X-Xuanchu-Timestamp", fmt.Sprintf("%d", now))
		sig := webhookSignature(sink.Secret, deliveryID, now, body)
		req.Header.Set("X-Xuanchu-Signature-256", sig)
	}

	start := time.Now()
	resp, err := client.Do(req)
	durationMS := time.Since(start).Milliseconds()
	if err != nil {
		return NotificationSinkTestView{
			Status:     "failed",
			DurationMS: durationMS,
			Error:      err.Error(),
		}, nil
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	statusCode := resp.StatusCode
	view := NotificationSinkTestView{
		Status:     "succeeded",
		StatusCode: &statusCode,
		DurationMS: durationMS,
	}
	if statusCode < 200 || statusCode >= 300 {
		view.Status = "failed"
		view.Error = fmt.Sprintf("target returned status %d", statusCode)
	}
	return view, nil
}

// defaultSinkTestClient 构造 SSRF-safe 的 HTTP client，与 notificationruntime dispatcher 的防护一致。
func defaultSinkTestClient(resolver HookHostResolver) *http.Client {
	if resolver == nil {
		resolver = DefaultHookResolver()
	}
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			addrs, err := resolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			for _, addr := range addrs {
				if IsBlockedWebhookIP(addr.IP) {
					continue
				}
				return dialer.DialContext(ctx, network, net.JoinHostPort(addr.IP.String(), port))
			}
			return nil, RuntimeError{Code: "notification_endpoint_invalid", Message: "endpoint resolves to no allowed addresses"}
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   120 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func decodeRenderedHeaders(raw string) map[string][]string {
	out := map[string][]string{}
	if strings.TrimSpace(raw) == "" {
		return out
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return out
	}
	return out
}

func truncateBodyPreview(body string) string {
	const max = 2048
	if len(body) <= max {
		return body
	}
	return body[:max] + "..."
}

// webhookSignature 与 notificationruntime.SignatureSHA256 算法一致，
// 用于 webhook sink 测试投递的 HMAC-SHA256 签名。
func webhookSignature(secret, deliveryID string, timestamp int64, body []byte) string {
	if secret == "" {
		return ""
	}
	message := fmt.Sprintf("%s.%d.%s", deliveryID, timestamp, string(body))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
