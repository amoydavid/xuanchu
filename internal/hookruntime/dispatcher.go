package hookruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/logging"
	"git.dajee.net/dajee/xuanchu/internal/runtimeutil"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

// DispatcherOptions 配置 webhook 投递调度器。
type DispatcherOptions struct {
	Store                  *storage.Store
	Clock                  app.Clock
	Client                 *http.Client
	Resolver               app.HookHostResolver
	Version                string
	BatchSize              int
	PollInterval           time.Duration
	RetryBaseDelay         time.Duration
	ClaimTTL               time.Duration
	MaxConcurrency         int
	PrefetchFactor         int
	DefaultSinkConcurrency int
	SinkLimiter            *runtimeutil.SinkLimiter
	Shutdown               *runtimeutil.ShutdownCoordinator
	JitterSeed             int64
	Logger                 *logging.Logger
}

// Dispatcher 负责 webhook 投递的主循环。
type Dispatcher struct {
	opts         DispatcherOptions
	hookRepo     *storage.HookRepository
	sinkRepo     *storage.NotificationSinkRepository
	deliveryRepo *storage.HookDeliveryRepository
	rng          *rand.Rand
	rngMu        sync.Mutex
}

// NewDispatcher 创建一个新的 Dispatcher 实例，填充默认值。
func NewDispatcher(opts DispatcherOptions) *Dispatcher {
	if opts.BatchSize == 0 {
		opts.BatchSize = 50
	}
	if opts.PollInterval == 0 {
		opts.PollInterval = 5 * time.Second
	}
	if opts.RetryBaseDelay == 0 {
		opts.RetryBaseDelay = 30 * time.Second
	}
	if opts.ClaimTTL == 0 {
		opts.ClaimTTL = 5 * time.Minute
	}
	opts.MaxConcurrency = runtimeutil.EffectiveConcurrency(opts.MaxConcurrency)
	opts.PrefetchFactor = runtimeutil.EffectivePrefetchFactor(opts.PrefetchFactor)
	if opts.DefaultSinkConcurrency <= 0 {
		opts.DefaultSinkConcurrency = opts.MaxConcurrency
	}
	if opts.SinkLimiter == nil {
		opts.SinkLimiter = runtimeutil.NewSinkLimiter()
	}
	if opts.Shutdown == nil {
		opts.Shutdown = runtimeutil.NewShutdownCoordinator()
	}
	if opts.Version == "" {
		opts.Version = "dev"
	}
	if opts.Resolver == nil {
		opts.Resolver = app.DefaultHookResolver()
	}
	if opts.Client == nil {
		opts.Client = defaultWebhookClient(opts.Resolver)
	}
	if opts.Client.CheckRedirect == nil {
		opts.Client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // 不跟随重定向
		}
	}
	if opts.Client.Timeout == 0 {
		opts.Client.Timeout = 120 * time.Second
	}
	seed := opts.JitterSeed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	return &Dispatcher{
		opts:         opts,
		hookRepo:     storage.NewHookRepository(opts.Store.DB()),
		sinkRepo:     storage.NewNotificationSinkRepository(opts.Store.DB()),
		deliveryRepo: storage.NewHookDeliveryRepository(opts.Store.DB()),
		rng:          rand.New(rand.NewSource(seed)),
	}
}

func defaultWebhookClient(resolver app.HookHostResolver) *http.Client {
	if resolver == nil {
		resolver = app.DefaultHookResolver()
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
				if app.IsBlockedWebhookIP(addr.IP) {
					continue
				}
				ipAddr := net.JoinHostPort(addr.IP.String(), port)
				return dialer.DialContext(ctx, network, ipAddr)
			}
			return nil, app.RuntimeError{Code: "hook_endpoint_invalid", Message: "endpoint resolves to no allowed addresses"}
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

// RunOnce 执行一次投递循环：恢复过期 -> 领取 -> 逐个投递。
func (d *Dispatcher) RunOnce(ctx context.Context) error {
	now := d.opts.Clock.Unix()

	// 恢复过期的 delivering 状态
	if _, err := d.deliveryRepo.RecoverStaleDelivering(now); err != nil {
		return fmt.Errorf("recover stale deliveries: %w", err)
	}
	if !d.opts.Shutdown.Accepting() {
		return nil
	}

	// 计算领取消耗时间
	claimExpiresAt := now + int64(d.opts.ClaimTTL.Seconds())

	// 领取到期投递
	claimLimit := runtimeutil.ClaimLimit(d.opts.BatchSize, d.opts.MaxConcurrency, d.opts.PrefetchFactor)
	deliveries, err := d.deliveryRepo.ClaimDue(now, claimExpiresAt, claimLimit)
	if err != nil {
		return fmt.Errorf("claim due deliveries: %w", err)
	}
	if len(deliveries) == 0 {
		return nil
	}

	sem := make(chan struct{}, d.opts.MaxConcurrency)
	errCh := make(chan error, len(deliveries))
	var wg sync.WaitGroup
	startErr := error(nil)
	requeueFrom := len(deliveries)
	for i, delivery := range deliveries {
		if err := ctx.Err(); err != nil {
			startErr = err
			requeueFrom = i
			break
		}
		done, ok := d.opts.Shutdown.Begin()
		if !ok {
			if err := d.deliveryRepo.ReleaseClaim(delivery.ID, d.opts.Clock.Unix()); err != nil {
				errCh <- err
			}
			requeueFrom = i + 1
			break
		}
		select {
		case <-ctx.Done():
			startErr = ctx.Err()
			if err := d.deliveryRepo.ReleaseClaim(delivery.ID, d.opts.Clock.Unix()); err != nil {
				errCh <- err
			}
			done()
			requeueFrom = i + 1
			goto wait
		case sem <- struct{}{}:
		}
		if !d.opts.Shutdown.Accepting() {
			<-sem
			if err := d.deliveryRepo.ReleaseClaim(delivery.ID, d.opts.Clock.Unix()); err != nil {
				errCh <- err
			}
			done()
			requeueFrom = i + 1
			break
		}
		wg.Add(1)
		go func(delivery storage.HookDelivery, done func()) {
			defer wg.Done()
			defer func() { <-sem }()
			defer done()
			err := d.dispatchOne(d.opts.Shutdown.Context(), delivery, d.opts.Clock.Unix())
			d.logDeliveryAttempt(delivery, err)
			errCh <- err
		}(delivery, done)
	}
wait:
	for _, delivery := range deliveries[requeueFrom:] {
		if err := d.deliveryRepo.ReleaseClaim(delivery.ID, d.opts.Clock.Unix()); err != nil {
			errCh <- err
		}
	}
	wg.Wait()
	close(errCh)
	if startErr != nil {
		return startErr
	}
	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *Dispatcher) logDeliveryAttempt(delivery storage.HookDelivery, err error) {
	if d.opts.Logger == nil {
		return
	}
	args := []any{
		"component", "hook_dispatcher",
		"operation", "hook_delivery_attempt",
		"delivery_id", delivery.ID,
		"hook_id", delivery.HookID,
		"sink_id", delivery.SinkID,
		"event_type", delivery.EventType,
		"workspace_id", delivery.WorkspaceID,
		"attempt", delivery.AttemptCount,
	}
	if delivery.ProjectID != nil {
		args = append(args, "project_id", *delivery.ProjectID)
	}
	if objectKind, objectID := hookDeliveryObject(delivery); objectKind != "" || objectID != "" {
		args = append(args, "object_kind", objectKind, "object_id", objectID)
	}
	if err != nil {
		args = append(args, "result", "error", "error", err.Error())
		d.opts.Logger.Warn("hook delivery attempt", args...)
		return
	}
	updated, getErr := d.deliveryRepo.GetByID(delivery.ID)
	if getErr != nil {
		args = append(args, "result", "error", "error", getErr.Error())
		d.opts.Logger.Warn("hook delivery attempt", args...)
		return
	}
	result := string(updated.Status)
	if updated.Status == storage.DeliveryStatusSucceeded {
		result = "success"
	}
	if updated.NextAttemptAt != nil {
		args = append(args, "next_attempt_at", *updated.NextAttemptAt)
	}
	args = append(args, "result", result)
	if result == "success" {
		d.opts.Logger.Info("hook delivery attempt", args...)
		return
	}
	d.opts.Logger.Warn("hook delivery attempt", args...)
}

func hookDeliveryObject(delivery storage.HookDelivery) (string, string) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(delivery.PayloadJSON), &payload); err != nil {
		return "", ""
	}
	objectKind, _ := payload["object_kind"].(string)
	objectID, _ := payload["object_id"].(string)
	if objectKind != "" || objectID != "" {
		return objectKind, objectID
	}
	if object, ok := payload["object"].(map[string]any); ok {
		objectKind, _ = object["kind"].(string)
		objectID, _ = object["id"].(string)
	}
	return objectKind, objectID
}

// Run 启动持续的投递循环，直到 context 被取消。
func (d *Dispatcher) Run(ctx context.Context) error {
	// 启动时恢复过期投递
	now := d.opts.Clock.Unix()
	if _, err := d.deliveryRepo.RecoverStaleDelivering(now); err != nil {
		return fmt.Errorf("recover stale deliveries: %w", err)
	}

	ticker := time.NewTicker(d.opts.PollInterval)
	defer ticker.Stop()
	for {
		if !d.opts.Shutdown.Accepting() {
			return nil
		}
		if err := d.RunOnce(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (d *Dispatcher) dispatchOne(ctx context.Context, delivery storage.HookDelivery, now int64) error {
	// 加载 hook 定义
	hook, err := d.hookRepo.GetByID(delivery.HookID)
	if err != nil {
		return d.deliveryRepo.MarkDeadLettered(delivery.ID, now, nil, "hook not found")
	}

	// 检查 hook 是否启用
	if hook.Enabled == nil || !*hook.Enabled {
		return d.deliveryRepo.MarkDisabledSkipped(delivery.ID, now)
	}

	sink, err := d.sinkRepo.GetByID(delivery.SinkID)
	if err != nil {
		return d.deliveryRepo.MarkDeadLettered(delivery.ID, now, nil, "sink not found")
	}
	if sink.WorkspaceID != delivery.WorkspaceID {
		return d.deliveryRepo.MarkDeadLettered(delivery.ID, now, nil, "sink workspace mismatch")
	}
	if sink.Enabled == nil || !*sink.Enabled {
		return d.deliveryRepo.MarkDisabledSkipped(delivery.ID, now)
	}
	sinkLimit := runtimeutil.EffectiveSinkConcurrency(sink.MaxConcurrency, d.opts.DefaultSinkConcurrency)
	if !d.opts.SinkLimiter.TryAcquire(delivery.SinkID, sinkLimit) {
		return d.deliveryRepo.Requeue(delivery.ID, now)
	}
	defer d.opts.SinkLimiter.Release(delivery.SinkID)

	// SSRF 重新验证（防止 DNS rebinding 攻击）
	if err := app.ValidateWebhookEndpointURL(ctx, delivery.ResolvedURL, d.opts.Resolver); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return d.deliveryRepo.MarkDeadLettered(delivery.ID, now, nil, "endpoint SSRF validation failed: "+err.Error())
	}

	// 构造请求
	bodyText := delivery.RenderedBody
	if bodyText == "" {
		bodyText = delivery.PayloadJSON
	}
	body := []byte(bodyText)
	method := delivery.RenderedMethod
	if method == "" {
		method = http.MethodPost
	}

	timeout := time.Duration(hook.TimeoutSeconds) * time.Second
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, method, delivery.ResolvedURL, bytes.NewReader(body))
	if err != nil {
		return d.deliveryRepo.MarkDeadLettered(delivery.ID, now, nil, "build request: "+err.Error())
	}

	headers, err := HeadersForDelivery(delivery, hook.ID, sink.Secret, body, now, d.opts.Version)
	if err != nil {
		return d.deliveryRepo.MarkDeadLettered(delivery.ID, now, nil, "build headers: "+err.Error())
	}
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}

	resp, err := d.opts.Client.Do(req)
	if err != nil {
		return d.handleFailure(delivery, hook, now, nil, err)
	}
	defer resp.Body.Close()

	return d.handleResponse(delivery, hook, now, resp)
}

func (d *Dispatcher) handleResponse(delivery storage.HookDelivery, hook storage.HookDefinition, now int64, resp *http.Response) error {
	statusCode := resp.StatusCode

	if statusCode >= 200 && statusCode < 300 {
		return d.deliveryRepo.MarkSucceeded(delivery.ID, now, statusCode)
	}

	message := fmt.Sprintf("HTTP %d", statusCode)

	switch {
	case statusCode >= 300 && statusCode < 400:
		// 3xx: 死信，不跟随重定向
		return d.deliveryRepo.MarkDeadLettered(delivery.ID, now, &statusCode, message)
	case statusCode >= 400 && statusCode < 500 && statusCode != 429:
		// 其他 4xx: 死信
		return d.deliveryRepo.MarkDeadLettered(delivery.ID, now, &statusCode, message)
	default:
		// 5xx, 429: 重试
		retryAfter := parseRetryAfter(resp, now)
		return d.handleRetry(delivery, hook, now, &statusCode, message, retryAfter)
	}
}

func (d *Dispatcher) handleFailure(delivery storage.HookDelivery, hook storage.HookDefinition, now int64, statusCode *int, err error) error {
	message := err.Error()
	return d.handleRetry(delivery, hook, now, statusCode, message, 0)
}

func (d *Dispatcher) handleRetry(delivery storage.HookDelivery, hook storage.HookDefinition, now int64, statusCode *int, message string, retryAfter time.Duration) error {
	if delivery.AttemptCount >= hook.MaxAttempts {
		return d.deliveryRepo.MarkDeadLettered(delivery.ID, now, statusCode, message)
	}

	var nextAttempt int64
	if retryAfter > 0 {
		if retryAfter > time.Hour {
			retryAfter = time.Hour
		}
		nextAttempt = now + int64(retryAfter.Seconds())
	} else {
		delay := d.calculateBackoff(delivery.AttemptCount - 1)
		nextAttempt = now + int64(delay.Seconds())
	}

	return d.deliveryRepo.MarkRetry(delivery.ID, now, nextAttempt, statusCode, message)
}

// calculateBackoff 计算指数退避延迟，带 jitter。
// base = min(2^(attempt-1) * RetryBaseDelay, 1h)
// 最终延迟 = base * jitter(0.5..1.0)
func (d *Dispatcher) calculateBackoff(attemptCount int) time.Duration {
	exponent := attemptCount - 1
	if exponent < 0 {
		exponent = 0
	}
	base := d.opts.RetryBaseDelay
	if exponent > 0 {
		multiplier := math.Pow(2, float64(exponent))
		base = time.Duration(float64(base) * multiplier)
	}
	if base > time.Hour {
		base = time.Hour
	}
	// jitter: 0.5..1.0
	d.rngMu.Lock()
	jitter := 0.5 + d.rng.Float64()*0.5
	d.rngMu.Unlock()
	return time.Duration(float64(base) * jitter)
}

// parseRetryAfter 解析 HTTP Retry-After 头。
// 支持秒数和 HTTP 日期两种格式。
func parseRetryAfter(resp *http.Response, now int64) time.Duration {
	value := resp.Header.Get("Retry-After")
	if value == "" {
		return 0
	}
	// 先尝试秒数
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	}
	// 再尝试 HTTP 日期
	if t, err := http.ParseTime(value); err == nil {
		dur := t.Sub(time.Unix(now, 0))
		if dur > 0 {
			return dur
		}
	}
	return 0
}
