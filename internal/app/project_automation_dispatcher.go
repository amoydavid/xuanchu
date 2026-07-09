package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

// ProjectAutomationDispatcherOptions 描述投递 dispatcher 依赖。
type ProjectAutomationDispatcherOptions struct {
	Store          *storage.Store
	Clock          Clock
	Client         *http.Client
	BatchSize      int
	ServiceFactory func(workspaceID string) *Service
}

// ProjectAutomationDispatchResult 描述一次投递扫描的结果。
type ProjectAutomationDispatchResult struct {
	Claimed   int
	Succeeded int
	Retried   int
	Failed    int
}

// ProjectAutomationDispatcher 认领到期投递并发送 OpenAI 兼容请求。
type ProjectAutomationDispatcher struct {
	store          *storage.Store
	clock          Clock
	client         *http.Client
	batchSize      int
	serviceFactory func(workspaceID string) *Service
}

// NewProjectAutomationDispatcher 构建投递 dispatcher。
func NewProjectAutomationDispatcher(opts ProjectAutomationDispatcherOptions) *ProjectAutomationDispatcher {
	if opts.Clock == nil {
		opts.Clock = RealClock{}
	}
	if opts.Client == nil {
		opts.Client = &http.Client{Timeout: 120 * time.Second}
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = 50
	}
	if opts.ServiceFactory == nil {
		opts.ServiceFactory = NewAutomationBackgroundServiceFactory(opts.Store, opts.Clock)
	}
	return &ProjectAutomationDispatcher{store: opts.Store, clock: opts.Clock, client: opts.Client, batchSize: opts.BatchSize, serviceFactory: opts.ServiceFactory}
}

// RunOnce 认领到期投递并发送，按响应状态更新投递状态。
func (d *ProjectAutomationDispatcher) RunOnce(ctx context.Context) (ProjectAutomationDispatchResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	repo := storage.NewProjectAutomationDeliveryRepository(d.store.DB())
	now := d.clock.Unix()
	rows, err := repo.ClaimDue(now, now+120, d.batchSize)
	if err != nil {
		return ProjectAutomationDispatchResult{}, err
	}
	result := ProjectAutomationDispatchResult{Claimed: len(rows)}
	for _, row := range rows {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		statusCode, responsePreview, providerRequestID, usageJSON, sendErr := d.send(ctx, row)
		if sendErr == nil && statusCode >= 200 && statusCode < 300 {
			if err := repo.MarkSucceeded(row.ID, now, statusCode, providerRequestID, responsePreview, usageJSON); err != nil {
				return result, err
			}
			result.Succeeded++
			continue
		}
		message := safeError(sendErr)
		if message == "" {
			message = http.StatusText(statusCode)
		}
		maxAttempts := d.maxAttemptsFor(row)
		if (statusCode == 429 || statusCode >= 500) && row.AttemptCount < maxAttempts {
			next := now + int64(min(row.AttemptCount, 5))*60
			if err := repo.MarkRetry(row.ID, now, next, intPtr(statusCode), message, responsePreview); err != nil {
				return result, err
			}
			result.Retried++
			continue
		}
		if err := repo.MarkFailed(row.ID, now, intPtr(statusCode), message, responsePreview); err != nil {
			return result, err
		}
		result.Failed++
	}
	return result, nil
}

// Run 以 interval 间隔循环执行 RunOnce，直到 ctx 取消。供 server 后台调度使用。
func (d *ProjectAutomationDispatcher) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if _, err := d.RunOnce(ctx); err != nil && ctx.Err() == nil {
		return err
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if _, err := d.RunOnce(ctx); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
		}
	}
}

func (d *ProjectAutomationDispatcher) maxAttemptsFor(row storage.ProjectAutomationDelivery) int {
	ruleRepo := storage.NewProjectAutomationRuleRepository(d.store.DB())
	rule, err := ruleRepo.GetByID(row.RuleID)
	if err != nil {
		return 5
	}
	input := projectAutomationRuleAddInputFromRow(rule)
	if input.Action.MaxAttempts <= 0 {
		return 5
	}
	return input.Action.MaxAttempts
}

// send 执行 HTTP 投递，重新读取当前 secret config 生成真实 Authorization。
func (d *ProjectAutomationDispatcher) send(ctx context.Context, row storage.ProjectAutomationDelivery) (int, string, string, string, error) {
	ruleRepo := storage.NewProjectAutomationRuleRepository(d.store.DB())
	rule, err := ruleRepo.GetByID(row.RuleID)
	if err != nil {
		return 0, "", "", "{}", err
	}
	svc := d.serviceFactory(row.WorkspaceID)
	input := projectAutomationRuleAddInputFromRow(rule)
	_, apiKey, _, err := svc.resolveProjectAutomationProviderConfig(row.ProjectID, input.Action)
	if err != nil {
		return 0, "", "", "{}", err
	}
	if row.RequestBodyJSON == "" {
		return 0, "", "", "{}", RuntimeError{Code: "automation_delivery_body_missing", Message: "delivery request body is missing"}
	}
	if row.ResolvedURL == "" {
		return 0, "", "", "{}", RuntimeError{Code: "automation_delivery_url_missing", Message: "delivery resolved url is missing"}
	}
	method := row.RenderedMethod
	if method == "" {
		method = http.MethodPost
	}
	req, err := http.NewRequestWithContext(ctx, method, row.ResolvedURL, strings.NewReader(row.RequestBodyJSON))
	if err != nil {
		return 0, "", "", "{}", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.client.Do(req)
	if err != nil {
		return 0, "", "", "{}", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))
	preview := truncatePreview(string(raw), 12000)
	providerID, usageJSON := parseOpenAIProviderResponse(raw)
	return resp.StatusCode, preview, providerID, usageJSON, nil
}

func intPtr(value int) *int {
	return &value
}

func safeError(err error) string {
	if err == nil {
		return ""
	}
	return truncatePreview(err.Error(), 2000)
}

// parseOpenAIProviderResponse 从 OpenAI 兼容响应中提取 id 和 usage。
func parseOpenAIProviderResponse(raw []byte) (string, string) {
	var body struct {
		ID    string         `json:"id"`
		Usage map[string]any `json:"usage"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return "", "{}"
	}
	usageJSON := "{}"
	if body.Usage != nil {
		if b, err := json.Marshal(body.Usage); err == nil {
			usageJSON = string(b)
		}
	}
	return body.ID, usageJSON
}
