package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

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

// RunOnce 扫描所有启用规则，对到期的 daily_at 规则入队，按 dedupe_key 去重。
func (s *ProjectAutomationScheduler) RunOnce(ctx context.Context) (ProjectAutomationSchedulerRunResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ruleRepo := storage.NewProjectAutomationRuleRepository(s.store.DB())
	deliveryRepo := storage.NewProjectAutomationDeliveryRepository(s.store.DB())
	rules, err := ruleRepo.ListEnabled()
	if err != nil {
		return ProjectAutomationSchedulerRunResult{}, err
	}
	now := s.clock.Unix()
	result := ProjectAutomationSchedulerRunResult{RulesChecked: len(rules)}
	for _, rule := range rules {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if rule.TriggerType != ProjectAutomationTriggerSchedule {
			continue
		}
		cfg := decodeProjectAutomationTriggerConfig(rule.TriggerConfigJSON)
		if cfg.ScheduleType != "daily_at" || !automationScheduleDue(cfg, now) {
			continue
		}
		key := fmt.Sprintf("%s:%s:%s:%s:%s", rule.WorkspaceID, rule.ProjectID, rule.ID, automationLocalDate(cfg, now), cfg.ScheduleValue)
		exists, err := deliveryRepo.ExistsByDedupeKey(key)
		if err != nil {
			return result, err
		}
		if exists {
			continue
		}
		svc := s.serviceFactory(rule.WorkspaceID)
		delivery, err := svc.buildProjectAutomationDelivery(rule, ProjectAutomationTriggerSchedule, "", "", key, now, nil)
		if err != nil {
			return result, err
		}
		if err := deliveryRepo.Enqueue([]storage.ProjectAutomationDelivery{delivery}); err != nil {
			return result, err
		}
		result.DeliveriesEnqueued++
	}
	return result, nil
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

// automationScheduleDue 判断当前时间是否已经过了规则今天的计划时间。
// 为保证调度窗口能覆盖到点，规则计划时间到后 1 小时内都视为到期。
func automationScheduleDue(cfg ProjectAutomationTriggerConfig, now int64) bool {
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		loc = time.Local
	}
	local := time.Unix(now, 0).In(loc)
	scheduled, err := time.ParseInLocation("15:04", cfg.ScheduleValue, loc)
	if err != nil {
		return false
	}
	// 取今天的计划时间点。
	today := time.Date(local.Year(), local.Month(), local.Day(), scheduled.Hour(), scheduled.Minute(), 0, 0, loc)
	// 计划时间已到、但未超过 1 小时调度窗口。
	return !local.Before(today) && local.Sub(today) <= time.Hour
}

// automationLocalDate 返回规则时区下的本地日期字符串，用于 dedupe_key。
func automationLocalDate(cfg ProjectAutomationTriggerConfig, now int64) string {
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		loc = time.Local
	}
	return time.Unix(now, 0).In(loc).Format("2006-01-02")
}

// EnqueueProjectAutomationForEvents 在事件提交后匹配事件规则并入队。
func (s *Service) EnqueueProjectAutomationForEvents(events []HookEvent) error {
	for _, event := range events {
		if event.ProjectID == nil || event.EventID == "" {
			continue
		}
		rules, err := s.projectAutomationRuleRepo.List(s.workspaceID, event.ProjectID, false)
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
			key := rule.WorkspaceID + ":" + rule.ProjectID + ":" + rule.ID + ":" + event.EventID
			delivery, err := s.buildProjectAutomationDelivery(rule, ProjectAutomationTriggerEvent, event.EventID, event.EventType, key, event.OccurredAt, &event)
			if err != nil {
				return err
			}
			if err := s.projectAutomationDeliveryRepo.Enqueue([]storage.ProjectAutomationDelivery{delivery}); err != nil {
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
func (s *Service) buildProjectAutomationDelivery(rule storage.ProjectAutomationRule, triggerType string, eventID string, eventType string, dedupeKey string, now int64, event *HookEvent) (storage.ProjectAutomationDelivery, error) {
	projectRow, err := s.ResolveProject(rule.ProjectID)
	if err != nil {
		return storage.ProjectAutomationDelivery{}, err
	}
	projectView, err := s.projectViewForRow(projectRow)
	if err != nil {
		return storage.ProjectAutomationDelivery{}, err
	}
	input := projectAutomationRuleAddInputFromRow(rule)
	deliveryID := uuid.NewString()
	rendered, err := s.renderProjectAutomationRequest(projectView, rule.ID, input, triggerType, deliveryID, event)
	if err != nil {
		return storage.ProjectAutomationDelivery{}, err
	}
	maskedHeaders := make(map[string][]string, len(rendered.MaskedHeaders))
	for key, value := range rendered.MaskedHeaders {
		maskedHeaders[key] = []string{value}
	}
	headersJSON, err := json.Marshal(maskedHeaders)
	if err != nil {
		return storage.ProjectAutomationDelivery{}, err
	}
	return storage.ProjectAutomationDelivery{
		ID:                  deliveryID,
		WorkspaceID:         rule.WorkspaceID,
		ProjectID:           rule.ProjectID,
		RuleID:              rule.ID,
		TriggerType:         triggerType,
		EventID:             eventID,
		EventType:           eventType,
		DedupeKey:           dedupeKey,
		Status:              storage.DeliveryStatusQueued,
		ResolvedURL:         rendered.URL,
		RenderedMethod:      rendered.Method,
		RenderedHeadersJSON: string(headersJSON),
		RequestBodyJSON:     rendered.BodyJSON,
		RequestBodyPreview:  rendered.BodyPreview,
		RequestBodyHash:     rendered.BodyHash,
		UsageJSON:           "{}",
		CreatedAt:           now,
		ModifiedAt:          now,
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
	if row.WorkspaceID != s.workspaceID || row.ProjectID != project.ID {
		return ProjectAutomationDeliveryView{}, RuntimeError{Code: "automation_rule_not_found", Message: "automation rule not found"}
	}
	now := s.clock.Unix()
	// 测试投递使用唯一 dedupe_key，避免与正式投递冲突。
	key := fmt.Sprintf("%s:%s:%s:test:%d", row.WorkspaceID, row.ProjectID, row.ID, now)
	delivery, err := s.buildProjectAutomationDelivery(row, ProjectAutomationTriggerManual, "", "", key, now, nil)
	if err != nil {
		return ProjectAutomationDeliveryView{}, err
	}
	if err := s.projectAutomationDeliveryRepo.Enqueue([]storage.ProjectAutomationDelivery{delivery}); err != nil {
		return ProjectAutomationDeliveryView{}, err
	}
	return projectAutomationDeliveryViewFromRow(delivery), nil
}
