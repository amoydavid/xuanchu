package notificationruntime

import (
	"bytes"
	"context"
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
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

type DispatcherOptions struct {
	Store          *storage.Store
	Clock          app.Clock
	Client         *http.Client
	Resolver       app.HookHostResolver
	Version        string
	BatchSize      int
	PollInterval   time.Duration
	RetryBaseDelay time.Duration
	ClaimTTL       time.Duration
	JitterSeed     int64
}

type Dispatcher struct {
	opts         DispatcherOptions
	deliveryRepo *storage.NotificationDeliveryRepository
	sinkRepo     *storage.NotificationSinkRepository
	taskRepo     *storage.TaskRepository
	rng          *rand.Rand
	rngMu        sync.Mutex
}

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
	if opts.Version == "" {
		opts.Version = "dev"
	}
	if opts.Resolver == nil {
		opts.Resolver = app.DefaultHookResolver()
	}
	if opts.Client == nil {
		opts.Client = defaultClient(opts.Resolver)
	}
	if opts.Client.CheckRedirect == nil {
		opts.Client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
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
		deliveryRepo: storage.NewNotificationDeliveryRepository(opts.Store.DB()),
		sinkRepo:     storage.NewNotificationSinkRepository(opts.Store.DB()),
		taskRepo:     storage.NewTaskRepository(opts.Store.DB()),
		rng:          rand.New(rand.NewSource(seed)),
	}
}

func defaultClient(resolver app.HookHostResolver) *http.Client {
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
				return dialer.DialContext(ctx, network, net.JoinHostPort(addr.IP.String(), port))
			}
			return nil, app.RuntimeError{Code: "notification_endpoint_invalid", Message: "endpoint resolves to no allowed addresses"}
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

func (d *Dispatcher) RunOnce(ctx context.Context) error {
	now := d.opts.Clock.Unix()
	if _, err := d.deliveryRepo.RecoverStaleDelivering(now); err != nil {
		return fmt.Errorf("recover stale deliveries: %w", err)
	}
	claimExpiresAt := now + int64(d.opts.ClaimTTL.Seconds())
	deliveries, err := d.deliveryRepo.ClaimDue(now, claimExpiresAt, d.opts.BatchSize)
	if err != nil {
		return fmt.Errorf("claim due deliveries: %w", err)
	}
	for _, delivery := range deliveries {
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		if err := d.dispatchOne(ctx, delivery, now); err != nil {
			return err
		}
	}
	return nil
}

func (d *Dispatcher) Run(ctx context.Context) error {
	now := d.opts.Clock.Unix()
	if _, err := d.deliveryRepo.RecoverStaleDelivering(now); err != nil {
		return fmt.Errorf("recover stale deliveries: %w", err)
	}
	ticker := time.NewTicker(d.opts.PollInterval)
	defer ticker.Stop()
	for {
		if err := d.RunOnce(ctx); err != nil {
			if ctx != nil && ctx.Err() != nil {
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

func (d *Dispatcher) dispatchOne(ctx context.Context, delivery storage.NotificationDelivery, now int64) error {
	sink, err := d.sinkRepo.GetByID(delivery.SinkID)
	if err != nil {
		return d.deliveryRepo.MarkDeadLettered(delivery.ID, now, nil, "notification sink not found")
	}
	if sink.WorkspaceID != delivery.WorkspaceID {
		return d.deliveryRepo.MarkDeadLettered(delivery.ID, now, nil, "notification sink workspace mismatch")
	}
	if sink.Enabled != nil && !*sink.Enabled {
		return d.deliveryRepo.MarkDisabledSkipped(delivery.ID, now, "sink disabled")
	}
	taskRow, err := d.taskRepo.GetByUUID(delivery.WorkspaceID, delivery.TaskUUID)
	if err != nil {
		return d.deliveryRepo.MarkDisabledSkipped(delivery.ID, now, "task no longer matches")
	}
	if taskRow.Status != task.StatusPending && taskRow.Status != task.StatusWaiting {
		return d.deliveryRepo.MarkDisabledSkipped(delivery.ID, now, "task no longer matches")
	}
	if taskRow.Due == nil {
		return d.deliveryRepo.MarkDisabledSkipped(delivery.ID, now, "task no longer matches")
	}
	if err := app.ValidateWebhookEndpointURL(ctx, delivery.ResolvedURL, d.opts.Resolver); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return d.deliveryRepo.MarkDeadLettered(delivery.ID, now, nil, "endpoint validation failed: "+err.Error())
	}
	body := []byte(delivery.RenderedBody)
	timeout := time.Duration(sink.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, delivery.RenderedMethod, delivery.ResolvedURL, bytes.NewReader(body))
	if err != nil {
		return d.deliveryRepo.MarkDeadLettered(delivery.ID, now, nil, "build request: "+err.Error())
	}
	headers, err := HeadersForDelivery(delivery, sink, body, now, d.opts.Version)
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
		return d.handleFailure(delivery, sink, now, nil, err)
	}
	defer resp.Body.Close()
	return d.handleResponse(delivery, sink, now, resp)
}

func (d *Dispatcher) handleResponse(delivery storage.NotificationDelivery, sink storage.NotificationSink, now int64, resp *http.Response) error {
	statusCode := resp.StatusCode
	if statusCode >= 200 && statusCode < 300 {
		return d.deliveryRepo.MarkSucceeded(delivery.ID, now, statusCode)
	}
	message := fmt.Sprintf("HTTP %d", statusCode)
	switch {
	case statusCode >= 300 && statusCode < 400:
		return d.deliveryRepo.MarkDeadLettered(delivery.ID, now, &statusCode, message)
	case statusCode >= 400 && statusCode < 500 && statusCode != 429:
		return d.deliveryRepo.MarkDeadLettered(delivery.ID, now, &statusCode, message)
	default:
		retryAfter := parseRetryAfter(resp, now)
		return d.handleRetry(delivery, sink, now, &statusCode, message, retryAfter)
	}
}

func (d *Dispatcher) handleFailure(delivery storage.NotificationDelivery, sink storage.NotificationSink, now int64, statusCode *int, err error) error {
	return d.handleRetry(delivery, sink, now, statusCode, err.Error(), 0)
}

func (d *Dispatcher) handleRetry(delivery storage.NotificationDelivery, sink storage.NotificationSink, now int64, statusCode *int, message string, retryAfter time.Duration) error {
	if delivery.AttemptCount >= sink.MaxAttempts {
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
	d.rngMu.Lock()
	jitter := 0.5 + d.rng.Float64()*0.5
	d.rngMu.Unlock()
	return time.Duration(float64(base) * jitter)
}

func parseRetryAfter(resp *http.Response, now int64) time.Duration {
	value := resp.Header.Get("Retry-After")
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if t, err := http.ParseTime(value); err == nil {
		dur := t.Sub(time.Unix(now, 0))
		if dur > 0 {
			return dur
		}
	}
	return 0
}
