package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"git.dajee.net/dajee/xuanchu/internal/query"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

type optionalLogger interface {
	Printf(string, ...any)
}

type ReminderSchedulerOptions struct {
	Store     *storage.Store
	Clock     Clock
	BatchSize int
	Logger    optionalLogger
}

type ReminderScheduler struct {
	store     *storage.Store
	clock     Clock
	batchSize int
	logger    optionalLogger
}

type ReminderSchedulerRunResult struct {
	RulesChecked       int
	DeliveriesEnqueued int
	RecipientsSkipped  int
}

func NewReminderScheduler(opts ReminderSchedulerOptions) *ReminderScheduler {
	if opts.Clock == nil {
		opts.Clock = RealClock{}
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = 200
	}
	return &ReminderScheduler{
		store:     opts.Store,
		clock:     opts.Clock,
		batchSize: opts.BatchSize,
		logger:    opts.Logger,
	}
}

func (s *ReminderScheduler) RunOnce(ctx context.Context) (ReminderSchedulerRunResult, error) {
	if s.store == nil {
		return ReminderSchedulerRunResult{}, fmt.Errorf("store is required")
	}
	var result ReminderSchedulerRunResult
	var workspaces []storage.Workspace
	if err := s.store.DB().Where("archived_at IS NULL").Order("slug ASC").Find(&workspaces).Error; err != nil {
		return result, err
	}
	now := s.clock.Unix()
	for _, ws := range workspaces {
		if ctx != nil && ctx.Err() != nil {
			return result, ctx.Err()
		}
		svc := s.workspaceService(ws)
		rules, err := svc.reminderRuleRepo.ListEnabled(ws.ID)
		if err != nil {
			return result, err
		}
		for _, rule := range rules {
			result.RulesChecked++
			created, skipped, err := svc.runReminderRule(ctx, rule, now, s.batchSize)
			if err != nil {
				return result, err
			}
			result.DeliveriesEnqueued += created
			result.RecipientsSkipped += skipped
		}
	}
	return result, nil
}

func (s *ReminderScheduler) workspaceService(ws storage.Workspace) *Service {
	return &Service{
		store:                s.store,
		repo:                 storage.NewTaskRepository(s.store.DB()),
		projectRepo:          storage.NewProjectRepository(s.store.DB()),
		configRepo:           storage.NewConfigRepository(s.store.DB()),
		configDefRepo:        storage.NewConfigDefinitionRepository(s.store.DB()),
		userRepo:             storage.NewUserRepository(s.store.DB()),
		workspaceRepo:        storage.NewWorkspaceRepository(s.store.DB()),
		memberRepo:           storage.NewMemberRepository(s.store.DB()),
		udaRepo:              storage.NewUDARepository(s.store.DB()),
		hookRepo:             storage.NewHookRepository(s.store.DB()),
		hookDeliveryRepo:     storage.NewHookDeliveryRepository(s.store.DB()),
		notificationSinkRepo: storage.NewNotificationSinkRepository(s.store.DB()),
		reminderRuleRepo:     storage.NewReminderRuleRepository(s.store.DB()),
		notificationRepo:     storage.NewNotificationDeliveryRepository(s.store.DB()),
		extIDRepo:            storage.NewExternalIDRepository(s.store.DB()),
		clock:                s.clock,
		workspaceID:          ws.ID,
		runtime: RuntimeContext{
			ActorUserID:   "",
			ActorName:     "",
			WorkspaceID:   ws.ID,
			WorkspaceSlug: ws.Slug,
			Role:          RoleOwner,
		},
	}
}

func (s *Service) runReminderRule(ctx context.Context, rule storage.ReminderRule, now int64, batchSize int) (int, int, error) {
	sink, err := s.notificationSinkRepo.GetByID(rule.SinkID)
	if err != nil {
		return 0, 0, err
	}
	if sink.WorkspaceID != s.workspaceID {
		return 0, 0, nil
	}
	if sink.Enabled != nil && !*sink.Enabled {
		return 0, 0, nil
	}
	tasks, err := s.listTasksForReminderRule(rule, batchSize)
	if err != nil {
		return 0, 0, err
	}
	explicitIDs, err := decodeStringListJSON(rule.RecipientUserIDsJSON)
	if err != nil {
		return 0, 0, err
	}
	eventType := "task.due_soon"
	if rule.TriggerType == "overdue" {
		eventType = "task.overdue"
	}
	created := 0
	skipped := 0
	for _, tsk := range tasks {
		if ctx != nil && ctx.Err() != nil {
			return created, skipped, ctx.Err()
		}
		if !reminderTaskEligible(tsk) {
			continue
		}
		matched, windowStart, err := reminderRuleMatchesTask(rule, tsk, now)
		if err != nil || !matched {
			if err != nil {
				return created, skipped, err
			}
			continue
		}
		recipients := reminderRecipientsForRule(rule, tsk, explicitIDs)
		if len(recipients) == 0 {
			continue
		}
		for _, recipientID := range recipients {
			recipientInfo, err := s.resolveUserInfos([]string{recipientID})
			if err != nil {
				skipped++
				continue
			}
			recipient := recipientInfo[recipientID]
			deliveryID := uuid.NewString()
			request, err := s.ResolveNotificationRequest(sink, rule, tsk, recipient, eventType, deliveryID)
			if err != nil {
				skipped++
				continue
			}
			row := storage.NotificationDelivery{
				ID:                          deliveryID,
				WorkspaceID:                 s.workspaceID,
				ProjectID:                   rule.ProjectID,
				RuleID:                      rule.ID,
				SinkID:                      sink.ID,
				TaskUUID:                    tsk.UUID,
				RecipientUserID:             recipientID,
				EventID:                     deliveryID,
				EventType:                   eventType,
				DedupeKey:                   reminderDedupeKey(s.workspaceID, rule.ID, tsk.UUID, recipientID, eventType, windowStart),
				ResolvedURL:                 request.ResolvedURL,
				ResolvedEndpointSource:      request.ResolvedEndpointSource,
				ResolvedEndpointFingerprint: request.ResolvedEndpointFingerprint,
				RenderedMethod:              request.RenderedMethod,
				RenderedHeadersJSON:         request.RenderedHeadersJSON,
				RenderedBody:                request.RenderedBody,
				RenderedContentType:         request.RenderedContentType,
				PayloadJSON:                 request.PayloadJSON,
				Status:                      storage.DeliveryStatusQueued,
				AttemptCount:                0,
				NextAttemptAt:               nil,
				CreatedAt:                   now,
				ModifiedAt:                  now,
			}
			if err := s.notificationRepo.Enqueue([]storage.NotificationDelivery{row}); err != nil {
				return created, skipped, err
			}
			created++
		}
	}
	return created, skipped, nil
}

func (s *Service) listTasksForReminderRule(rule storage.ReminderRule, batchSize int) ([]task.Task, error) {
	filterSource := decodeReminderTaskFilterSource(rule.TaskFilterJSON)
	expr, err := query.ParseQuery(filterSource)
	if err != nil {
		return nil, RuntimeError{Code: "reminder_rule_invalid", Message: err.Error()}
	}
	expr, err = s.resolveTaskQueryPredicates(expr)
	if err != nil {
		return nil, err
	}
	expr = query.And(
		query.Predicate{Attribute: query.AttrDue, Operator: query.OpNotNull},
		expr,
	)
	if rule.ProjectID != nil {
		expr = query.And(
			query.Predicate{Attribute: query.AttrProjectID, Operator: query.OpEqual, Value: query.StringValue(*rule.ProjectID)},
			expr,
		)
	}
	return s.repo.List(s.workspaceID, storage.ListOptions{
		Query:   expr,
		Limit:   batchSize,
		Dialect: s.store.Dialect(),
	})
}

func reminderTaskEligible(tsk task.Task) bool {
	return tsk.Due != nil
}

func reminderRuleMatchesTask(rule storage.ReminderRule, tsk task.Task, now int64) (bool, int64, error) {
	if tsk.Due == nil {
		return false, 0, nil
	}
	baseStart := *tsk.Due
	switch rule.TriggerType {
	case "due_before":
		baseStart = *tsk.Due - rule.OffsetSeconds
		if now < baseStart || now >= *tsk.Due {
			return false, 0, nil
		}
	case "overdue":
		baseStart = *tsk.Due + rule.AfterSeconds
		if now < baseStart {
			return false, 0, nil
		}
	default:
		return false, 0, RuntimeError{Code: "reminder_rule_invalid", Message: fmt.Sprintf("unsupported reminder trigger %q", rule.TriggerType)}
	}
	windowStart := baseStart
	if strings.HasPrefix(rule.RepeatPolicy, "every:") {
		repeatSeconds, err := parseRepeatDuration(strings.TrimPrefix(rule.RepeatPolicy, "every:"))
		if err != nil {
			return false, 0, err
		}
		if repeatSeconds > 0 && now >= baseStart {
			windowStart = baseStart + ((now-baseStart)/repeatSeconds)*repeatSeconds
		}
	}
	return true, windowStart, nil
}

func reminderRecipientsForRule(rule storage.ReminderRule, tsk task.Task, explicitIDs []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(explicitIDs)+len(tsk.Assignees))
	if rule.AudienceType == string(ReminderAudienceExplicitUsers) || rule.AudienceType == string(ReminderAudienceAssigneesAndUsers) {
		for _, id := range explicitIDs {
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, id)
		}
	}
	if rule.AudienceType == string(ReminderAudienceAssignees) || rule.AudienceType == string(ReminderAudienceAssigneesAndUsers) {
		for _, assignee := range tsk.Assignees {
			if assignee.UserID == "" || seen[assignee.UserID] {
				continue
			}
			seen[assignee.UserID] = true
			out = append(out, assignee.UserID)
		}
	}
	return out
}

func reminderDedupeKey(workspaceID, ruleID, taskUUID, recipientID, eventType string, windowStart int64) string {
	return fmt.Sprintf("%s:%s:%s:%s:%s:%d", workspaceID, ruleID, taskUUID, recipientID, eventType, windowStart)
}
