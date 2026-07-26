package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.dajee.net/dajee/xuanchu/internal/schedule"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

// ProjectAutomationSchedulerOptions 描述调度器依赖。
type ProjectAutomationSchedulerOptions struct {
	Store          *storage.Store
	Clock          Clock
	ServiceFactory func(workspaceID string) *Service
	BatchSize      int
}

// ProjectAutomationSchedulerRunResult 描述一次调度扫描的结果。
type ProjectAutomationSchedulerRunResult struct {
	RulesChecked       int
	DeliveriesEnqueued int
}

// ProjectAutomationScheduler 负责扫描 daily_at 规则并入队投递。
type ProjectAutomationScheduler struct {
	store          *storage.Store
	clock          Clock
	serviceFactory func(workspaceID string) *Service
	batchSize      int
}

// NewProjectAutomationScheduler 构建调度器。
func NewProjectAutomationScheduler(opts ProjectAutomationSchedulerOptions) *ProjectAutomationScheduler {
	if opts.Clock == nil {
		opts.Clock = RealClock{}
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = 100
	}
	if opts.ServiceFactory == nil {
		opts.ServiceFactory = NewAutomationBackgroundServiceFactory(opts.Store, opts.Clock)
	}
	return &ProjectAutomationScheduler{store: opts.Store, clock: opts.Clock, serviceFactory: opts.ServiceFactory, batchSize: opts.BatchSize}
}

// NewAutomationBackgroundServiceFactory 返回一个后台 service 工厂，按 workspaceID 构建
// 系统 service，用于调度器和投递 dispatcher 解析 config secret 和渲染上下文。
// 不走 actor 鉴权链路，只用于后台投递。
func NewAutomationBackgroundServiceFactory(store *storage.Store, clock Clock) func(workspaceID string) *Service {
	return func(workspaceID string) *Service {
		slug := workspaceID
		if ws, err := storage.NewWorkspaceRepository(store.DB()).GetByID(workspaceID); err == nil && ws.Slug != "" {
			slug = ws.Slug
		}
		svc, err := NewService(ServiceOptions{
			Store:                 store,
			Clock:                 clock,
			DisableScopeBootstrap: true,
			Runtime: &RuntimeContext{
				ActorType:     "system",
				WorkspaceID:   workspaceID,
				WorkspaceSlug: slug,
				Role:          RoleAdmin,
			},
		})
		if err != nil {
			return nil
		}
		return svc
	}
}

// RunOnce 扫描所有启用规则，对到期的 schedule 规则（daily_at 或 cron）入队，按 dedupe_key 去重。
// 触发点后 1 小时调度窗口内视为到期，防调度间隙漏触发。dedupe key 粒度：daily_at 按天、cron 按分钟。
func (s *ProjectAutomationScheduler) RunOnce(ctx context.Context) (ProjectAutomationSchedulerRunResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ruleRepo := storage.NewAutomationRuleRepository(s.store.DB())
	deliveryRepo := storage.NewAutomationDeliveryRepository(s.store.DB())
	rules, err := ruleRepo.ListEnabled()
	if err != nil {
		return ProjectAutomationSchedulerRunResult{}, err
	}
	now := s.clock.Unix()
	nowTime := time.Unix(now, 0)
	result := ProjectAutomationSchedulerRunResult{RulesChecked: len(rules)}
	for _, rule := range rules {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if rule.TriggerType != ProjectAutomationTriggerSchedule {
			continue
		}
		cfg := decodeProjectAutomationTriggerConfig(rule.TriggerConfigJSON)
		spec := schedule.Spec{Type: cfg.ScheduleType, Value: cfg.ScheduleValue, Timezone: cfg.Timezone}
		fire, ok, err := spec.LastFireAt(nowTime)
		if err != nil || !ok {
			continue
		}
		// 保留 1 小时调度窗口：触发点后 1 小时内才算到期，防调度间隙漏触发。
		if nowTime.Sub(fire) > time.Hour {
			continue
		}
		dedupe, err := spec.DedupeKey(nowTime)
		if err != nil {
			continue
		}
		key := fmt.Sprintf("schedule:%s:%s:%s:%s", rule.ScopeType, rule.ScopeID, rule.ID, dedupe)
		exists, err := deliveryRepo.ExistsByDedupeKey(key)
		if err != nil {
			return result, err
		}
		if exists {
			continue
		}
		svc := s.serviceFactory(rule.WorkspaceID)
		delivery, err := svc.buildAutomationScheduleDelivery(rule, key, now)
		if err != nil {
			return result, err
		}
		if err := deliveryRepo.Enqueue([]storage.AutomationDelivery{delivery}); err != nil {
			return result, err
		}
		result.DeliveriesEnqueued++
	}
	return result, nil
}

// buildAutomationScheduleDelivery 按 rule.ScopeType 分发到 Workspace/Project 渲染路径。
// Workspace schedule 不绑定 Project，project_id=nil，且不查询 Project/Task；
// Project schedule 继续走 buildProjectAutomationDelivery，保留历史行为。
func (s *Service) buildAutomationScheduleDelivery(rule storage.AutomationRule, dedupeKey string, now int64) (storage.AutomationDelivery, error) {
	if rule.ScopeType == storage.AutomationScopeWorkspace {
		return s.buildWorkspaceScheduleDelivery(rule, dedupeKey, now)
	}
	return s.buildProjectAutomationDelivery(rule, ProjectAutomationTriggerSchedule, "", "", dedupeKey, now, nil)
}

// buildWorkspaceScheduleDelivery 渲染 Workspace schedule Delivery。
// 上下文节点只有 _xuanchu + workspace；Agent 通过 MCP 自行查询 Project/Task。
func (s *Service) buildWorkspaceScheduleDelivery(rule storage.AutomationRule, dedupeKey string, now int64) (storage.AutomationDelivery, error) {
	input := automationRuleInputFromRow(rule)
	systemPrompt := input.SystemPrompt
	if strings.TrimSpace(systemPrompt) == "" {
		systemPrompt = defaultAutomationSystemPromptForScope(AutomationScope{Type: AutomationScopeWorkspace, ID: s.workspaceID})
	}
	baseURL, _, model, err := s.resolveWorkspaceAutomationProviderConfig(input.Action)
	if err != nil {
		return storage.AutomationDelivery{}, err
	}
	ws, err := s.workspaceRepo.GetByID(s.workspaceID)
	if err != nil {
		return storage.AutomationDelivery{}, err
	}
	xuanchuNode := map[string]any{
		"automation_rule_id": rule.ID,
		"automation_scope":   storage.AutomationScopeWorkspace,
		"trigger_type":       ProjectAutomationTriggerSchedule,
		"triggered_at":       now,
	}
	workspaceNode := map[string]any{"id": ws.ID, "slug": ws.Slug, "name": ws.Name}
	contextNode := map[string]any{"_xuanchu": xuanchuNode, "workspace": workspaceNode}
	body := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": renderAutomationTemplate(input.InstructionTemplate, map[string]string{
				"workspace.id":   ws.ID,
				"workspace.slug": ws.Slug,
				"workspace.name": ws.Name,
				"trigger_type":   ProjectAutomationTriggerSchedule,
			})},
		},
		"temperature": input.Action.Temperature,
		"metadata":    xuanchuNode,
		"input":       []map[string]any{{"type": "input_json", "input_json": contextNode}},
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return storage.AutomationDelivery{}, err
	}
	if len(bodyBytes) > automationRequestBodyLimit {
		return storage.AutomationDelivery{}, RuntimeError{Code: "automation_context_too_large", Message: "frozen request body exceeds limit"}
	}
	sum := sha256.Sum256(bodyBytes)
	maskedHeaders := map[string][]string{
		"Authorization": {"Bearer ****"},
		"Content-Type":  {"application/json"},
	}
	headersJSON, _ := json.Marshal(maskedHeaders)
	maxAttempts := input.Action.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	return storage.AutomationDelivery{
		ID:                    uuid.NewString(),
		WorkspaceID:           s.workspaceID,
		RuleScopeType:         storage.AutomationScopeWorkspace,
		RuleScopeID:           s.workspaceID,
		ProjectID:             nil,
		RuleID:                rule.ID,
		TriggerType:           ProjectAutomationTriggerSchedule,
		DedupeKey:             dedupeKey,
		APIKeyConfigKey:       input.Action.APIKeyConfigKey,
		AllowedHostsConfigKey: input.Action.AllowedHostsConfigKey,
		MaxAttempts:           maxAttempts,
		Status:                storage.DeliveryStatusQueued,
		ResolvedURL:           strings.TrimRight(baseURL, "/") + "/v1/chat/completions",
		RenderedMethod:        "POST",
		RenderedHeadersJSON:   string(headersJSON),
		RequestBodyJSON:       string(bodyBytes),
		RequestBodyPreview:    truncatePreview(string(bodyBytes), 12000),
		RequestBodyHash:       "sha256:" + hex.EncodeToString(sum[:]),
		UsageJSON:             "{}",
		CreatedAt:             now,
		ModifiedAt:            now,
	}, nil
}

// Run 以 interval 间隔循环执行 RunOnce，直到 ctx 取消。供 server 后台调度使用。
func (s *ProjectAutomationScheduler) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	if _, err := s.RunOnce(ctx); err != nil && ctx.Err() == nil {
		return err
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if _, err := s.RunOnce(ctx); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
		}
	}
}

// EnqueueProjectAutomationForEvents 在事件提交后匹配事件规则并入队。
func (s *Service) EnqueueProjectAutomationForEvents(events []HookEvent) error {
	for _, event := range events {
		if event.ProjectID == nil || event.EventID == "" {
			continue
		}
		rules, err := s.projectAutomationRuleRepo.ListScope(s.workspaceID, storage.AutomationScopeProject, *event.ProjectID, false)
		if err != nil {
			return err
		}
		for _, rule := range rules {
			if rule.TriggerType != ProjectAutomationTriggerEvent {
				continue
			}
			cfg := decodeProjectAutomationTriggerConfig(rule.TriggerConfigJSON)
			if cfg.EventType != event.EventType {
				continue
			}
			condition := decodeProjectAutomationCondition(rule.ConditionJSON)
			if condition.OnlyAddedAssignees && event.EventType == "task.assigned" && automationEventListLen(event.Data["added_assignees"]) == 0 {
				continue
			}
			key := fmt.Sprintf("event:%s:%s:%s:%s", rule.ScopeType, rule.ScopeID, rule.ID, event.EventID)
			delivery, err := s.buildProjectAutomationDelivery(rule, ProjectAutomationTriggerEvent, event.EventID, event.EventType, key, event.OccurredAt, &event)
			if err != nil {
				return err
			}
			if err := s.projectAutomationDeliveryRepo.Enqueue([]storage.AutomationDelivery{delivery}); err != nil {
				return err
			}
		}
	}
	return nil
}

func automationEventListLen(value any) int {
	switch typed := value.(type) {
	case []any:
		return len(typed)
	case []map[string]any:
		return len(typed)
	default:
		return 0
	}
}

// buildProjectAutomationDelivery 渲染规则并构造投递记录，不包含 secret 明文。
// 仅支持 Project scope 规则；Workspace scope 规则的渲染由 automation_runtime.go 负责。
func (s *Service) buildProjectAutomationDelivery(rule storage.AutomationRule, triggerType string, eventID string, eventType string, dedupeKey string, now int64, event *HookEvent) (storage.AutomationDelivery, error) {
	if rule.ScopeType != storage.AutomationScopeProject {
		return storage.AutomationDelivery{}, RuntimeError{Code: "automation_scope_invalid", Message: "only project scope rules can be rendered here"}
	}
	projectRow, err := s.ResolveProject(rule.ScopeID)
	if err != nil {
		return storage.AutomationDelivery{}, err
	}
	projectView, err := s.projectViewForRow(projectRow)
	if err != nil {
		return storage.AutomationDelivery{}, err
	}
	input := projectAutomationRuleAddInputFromRow(rule)
	deliveryID := uuid.NewString()
	rendered, err := s.renderProjectAutomationRequest(projectView, rule.ID, input, triggerType, deliveryID, event)
	if err != nil {
		return storage.AutomationDelivery{}, err
	}
	maskedHeaders := make(map[string][]string, len(rendered.MaskedHeaders))
	for key, value := range rendered.MaskedHeaders {
		maskedHeaders[key] = []string{value}
	}
	headersJSON, err := json.Marshal(maskedHeaders)
	if err != nil {
		return storage.AutomationDelivery{}, err
	}
	projectID := rule.ScopeID
	maxAttempts := input.Action.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	return storage.AutomationDelivery{
		ID:                    deliveryID,
		WorkspaceID:           rule.WorkspaceID,
		RuleScopeType:         rule.ScopeType,
		RuleScopeID:           rule.ScopeID,
		ProjectID:             &projectID,
		RuleID:                rule.ID,
		TriggerType:           triggerType,
		EventID:               eventID,
		EventType:             eventType,
		DedupeKey:             dedupeKey,
		APIKeyConfigKey:       input.Action.APIKeyConfigKey,
		AllowedHostsConfigKey: input.Action.AllowedHostsConfigKey,
		MaxAttempts:           maxAttempts,
		Status:                storage.DeliveryStatusQueued,
		ResolvedURL:           rendered.URL,
		RenderedMethod:        rendered.Method,
		RenderedHeadersJSON:   string(headersJSON),
		RequestBodyJSON:       rendered.BodyJSON,
		RequestBodyPreview:    rendered.BodyPreview,
		RequestBodyHash:       rendered.BodyHash,
		UsageJSON:             "{}",
		CreatedAt:             now,
		ModifiedAt:            now,
	}, nil
}

// TestProjectAutomationRule 立即测试规则，写入 manual_test 投递记录。
func (s *Service) TestProjectAutomationRule(projectRef string, ruleID string) (ProjectAutomationDeliveryView, error) {
	if err := s.requireProjectAutomationWrite(); err != nil {
		return ProjectAutomationDeliveryView{}, err
	}
	project, err := s.ResolveProject(projectRef)
	if err != nil {
		return ProjectAutomationDeliveryView{}, err
	}
	if isProjectClosed(project) {
		return ProjectAutomationDeliveryView{}, RuntimeError{Code: "project_closed", Message: "project is closed"}
	}
	row, err := s.projectAutomationRuleRepo.GetByID(ruleID)
	if err != nil {
		return ProjectAutomationDeliveryView{}, err
	}
	if row.WorkspaceID != s.workspaceID || row.ScopeType != storage.AutomationScopeProject || row.ScopeID != project.ID {
		return ProjectAutomationDeliveryView{}, RuntimeError{Code: "automation_rule_not_found", Message: "automation rule not found"}
	}
	now := s.clock.Unix()
	// 测试投递使用唯一 dedupe_key，避免与正式投递冲突。
	key := fmt.Sprintf("test:%s:%s:%s:%d", row.ScopeType, row.ScopeID, row.ID, now)
	delivery, err := s.buildProjectAutomationDelivery(row, ProjectAutomationTriggerManual, "", "", key, now, nil)
	if err != nil {
		return ProjectAutomationDeliveryView{}, err
	}
	if err := s.projectAutomationDeliveryRepo.Enqueue([]storage.AutomationDelivery{delivery}); err != nil {
		return ProjectAutomationDeliveryView{}, err
	}
	return projectAutomationDeliveryViewFromRow(delivery), nil
}
