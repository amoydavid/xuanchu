package notificationruntime

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
	"net/url"
	"slices"
	"strconv"
	"sync"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/runtimeutil"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

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
}

type Dispatcher struct {
	opts         DispatcherOptions
	sinkRepo     *storage.NotificationSinkRepository
	deliveryRepo *storage.NotificationDeliveryRepository
	taskRepo     *storage.TaskRepository
	validateURL  bool
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
	validateURL := opts.Client == nil
	if opts.Client == nil {
		opts.Client = defaultNotificationClient(opts.Resolver)
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
		sinkRepo:     storage.NewNotificationSinkRepository(opts.Store.DB()),
		deliveryRepo: storage.NewNotificationDeliveryRepository(opts.Store.DB()),
		taskRepo:     storage.NewTaskRepository(opts.Store.DB()),
		validateURL:  validateURL,
		rng:          rand.New(rand.NewSource(seed)),
	}
}

func defaultNotificationClient(resolver app.HookHostResolver) *http.Client {
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
		return fmt.Errorf("recover stale notification deliveries: %w", err)
	}
	if !d.opts.Shutdown.Accepting() {
		return nil
	}
	claimExpiresAt := now + int64(d.opts.ClaimTTL.Seconds())
	claimLimit := runtimeutil.ClaimLimit(d.opts.BatchSize, d.opts.MaxConcurrency, d.opts.PrefetchFactor)
	deliveries, err := d.deliveryRepo.ClaimDue(now, claimExpiresAt, claimLimit)
	if err != nil {
		return fmt.Errorf("claim due notification deliveries: %w", err)
	}
	if len(deliveries) == 0 {
		return nil
	}

	var wg sync.WaitGroup
	var firstErr error
	var errOnce sync.Once
	recordErr := func(err error) {
		if err == nil {
			return
		}
		errOnce.Do(func() { firstErr = err })
	}

	sem := make(chan struct{}, d.opts.MaxConcurrency)
	requeueFrom := len(deliveries)
	for i, delivery := range deliveries {
		if err := ctx.Err(); err != nil {
			recordErr(err)
			requeueFrom = i
			break
		}
		done, ok := d.opts.Shutdown.Begin()
		if !ok {
			if err := d.deliveryRepo.ReleaseClaim(delivery.ID, d.opts.Clock.Unix()); err != nil {
				recordErr(err)
			}
			requeueFrom = i + 1
			break
		}
		select {
		case <-ctx.Done():
			recordErr(ctx.Err())
			if err := d.deliveryRepo.ReleaseClaim(delivery.ID, d.opts.Clock.Unix()); err != nil {
				recordErr(err)
			}
			done()
			requeueFrom = i + 1
			goto wait
		case sem <- struct{}{}:
		}
		if !d.opts.Shutdown.Accepting() {
			<-sem
			if err := d.deliveryRepo.ReleaseClaim(delivery.ID, d.opts.Clock.Unix()); err != nil {
				recordErr(err)
			}
			done()
			requeueFrom = i + 1
			break
		}
		wg.Add(1)
		go func(delivery storage.NotificationDelivery, done func()) {
			defer wg.Done()
			defer func() { <-sem }()
			defer done()
			recordErr(d.dispatchOne(d.opts.Shutdown.Context(), delivery, d.opts.Clock.Unix()))
		}(delivery, done)
	}
wait:
	for _, delivery := range deliveries[requeueFrom:] {
		if err := d.deliveryRepo.ReleaseClaim(delivery.ID, d.opts.Clock.Unix()); err != nil {
			recordErr(err)
		}
	}
	wg.Wait()
	return firstErr
}

func (d *Dispatcher) Run(ctx context.Context) error {
	now := d.opts.Clock.Unix()
	if _, err := d.deliveryRepo.RecoverStaleDelivering(now); err != nil {
		return fmt.Errorf("recover stale notification deliveries: %w", err)
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

func (d *Dispatcher) dispatchOne(ctx context.Context, delivery storage.NotificationDelivery, now int64) error {
	sink, err := d.sinkRepo.GetByID(delivery.SinkID)
	if err != nil {
		return d.deliveryRepo.MarkDeadLettered(delivery.ID, now, nil, "notification sink not found")
	}
	if sink.Enabled == nil || !*sink.Enabled {
		return d.deliveryRepo.MarkDisabledSkipped(delivery.ID, now, "notification sink disabled")
	}
	if err := d.ensureTaskStillPending(delivery, now); err != nil {
		return err
	}
	sinkLimit := runtimeutil.EffectiveSinkConcurrency(sink.MaxConcurrency, d.opts.DefaultSinkConcurrency)
	if !d.opts.SinkLimiter.TryAcquire(sink.ID, sinkLimit) {
		return d.deliveryRepo.Requeue(delivery.ID, now)
	}
	defer d.opts.SinkLimiter.Release(sink.ID)

	if err := d.validateEndpoint(ctx, delivery, sink); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return d.deliveryRepo.MarkDeadLettered(delivery.ID, now, nil, "endpoint validation failed: "+err.Error())
	}

	body := []byte(delivery.RenderedBody)
	method := delivery.RenderedMethod
	if method == "" {
		method = http.MethodPost
	}
	timeout := time.Duration(sink.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, method, delivery.ResolvedURL, bytes.NewReader(body))
	if err != nil {
		return d.deliveryRepo.MarkDeadLettered(delivery.ID, now, nil, "build request: "+err.Error())
	}
	headers, err := headersForDelivery(delivery, sink, body, now, d.opts.Version)
	if err != nil {
		return d.deliveryRepo.MarkDeadLettered(delivery.ID, now, nil, "build headers: "+err.Error())
	}
	for name, values := range headers {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}

	resp, err := d.opts.Client.Do(req)
	if err != nil {
		return d.handleFailure(delivery, sink, now, nil, err)
	}
	defer resp.Body.Close()
	return d.handleResponse(delivery, sink, now, resp)
}

func (d *Dispatcher) ensureTaskStillPending(delivery storage.NotificationDelivery, now int64) error {
	tsk, err := d.taskRepo.GetByUUID(delivery.WorkspaceID, delivery.TaskUUID)
	if err == storage.ErrNotFound {
		return d.deliveryRepo.MarkDisabledSkipped(delivery.ID, now, "condition no longer matches")
	}
	if err != nil {
		return err
	}
	if tsk.Status != task.StatusPending {
		return d.deliveryRepo.MarkDisabledSkipped(delivery.ID, now, "condition no longer matches")
	}
	return nil
}

func (d *Dispatcher) validateEndpoint(ctx context.Context, delivery storage.NotificationDelivery, sink storage.NotificationSink) error {
	parsed, err := url.Parse(delivery.ResolvedURL)
	if err != nil || parsed.Hostname() == "" {
		return app.RuntimeError{Code: "notification_endpoint_invalid", Message: "invalid endpoint url"}
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return app.RuntimeError{Code: "notification_endpoint_invalid", Message: "unsupported endpoint scheme"}
	}
	var allowedHosts []string
	if sink.AllowedHostsJSON != "" {
		if err := jsonUnmarshalStringList(sink.AllowedHostsJSON, &allowedHosts); err != nil {
			return err
		}
	}
	if len(allowedHosts) > 0 && !slices.Contains(allowedHosts, parsed.Hostname()) {
		return app.RuntimeError{Code: "endpoint_host_denied", Message: "endpoint host is not allowed"}
	}
	if d.validateURL {
		return app.ValidateWebhookEndpointURL(ctx, delivery.ResolvedURL, d.opts.Resolver)
	}
	return nil
}

func jsonUnmarshalStringList(raw string, out *[]string) error {
	if raw == "" {
		return nil
	}
	return json.Unmarshal([]byte(raw), out)
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
		return d.handleRetry(delivery, sink, now, &statusCode, message, parseRetryAfter(resp, now))
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
		base = time.Duration(float64(base) * math.Pow(2, float64(exponent)))
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
		if dur := t.Sub(time.Unix(now, 0)); dur > 0 {
			return dur
		}
	}
	return 0
}
