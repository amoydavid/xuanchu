package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/logging"
	"git.dajee.net/dajee/xuanchu/internal/query"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ReminderSchedulerOptions struct {
	Store     *storage.Store
	Clock     Clock
	BatchSize int
	Logger    *logging.Logger
}

type ReminderSchedulerRunResult struct {
	RulesChecked       int
	DeliveriesEnqueued int
	RecipientsSkipped  int
}

type ReminderScheduler struct {
	store     *storage.Store
	clock     Clock
	batchSize int
	logger    *logging.Logger
}

func NewReminderScheduler(opts ReminderSchedulerOptions) *ReminderScheduler {
	if opts.Clock == nil {
		opts.Clock = RealClock{}
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = 500
	}
	return &ReminderScheduler{store: opts.Store, clock: opts.Clock, batchSize: opts.BatchSize, logger: opts.Logger}
}

func (s *ReminderScheduler) RunOnce(ctx context.Context) (result ReminderSchedulerRunResult, err error) {
	start := time.Now()
	defer func() {
		if s.logger == nil {
			return
		}
		args := []any{
			"component", "reminder_scheduler",
			"operation", "reminder_rule_scan",
			"rules_checked", result.RulesChecked,
			"deliveries_enqueued", result.DeliveriesEnqueued,
			"recipients_skipped", result.RecipientsSkipped,
			"duration_ms", time.Since(start).Milliseconds(),
		}
		if err != nil {
			args = append(args, "result", "error", "error", err.Error())
			s.logger.Warn("reminder rule scan", args...)
			return
		}
		args = append(args, "result", "success")
		s.logger.Info("reminder rule scan", args...)
	}()
	if ctx == nil {
		ctx = context.Background()
	}
	ruleRepo := storage.NewReminderRuleRepository(s.store.DB())
	sinkRepo := storage.NewNotificationSinkRepository(s.store.DB())
	deliveryRepo := storage.NewNotificationDeliveryRepository(s.store.DB())
	taskRepo := storage.NewTaskRepository(s.store.DB())
	workspaceRepo := storage.NewWorkspaceRepository(s.store.DB())
	projectRepo := storage.NewProjectRepository(s.store.DB())
	userRepo := storage.NewUserRepository(s.store.DB())
	extRepo := storage.NewExternalIDRepository(s.store.DB())
	configRepo := storage.NewConfigRepository(s.store.DB())
	configDefRepo := storage.NewConfigDefinitionRepository(s.store.DB())

	rules, err := ruleRepo.ListEnabled()
	if err != nil {
		return ReminderSchedulerRunResult{}, err
	}
	now := s.clock.Unix()
	result = ReminderSchedulerRunResult{RulesChecked: len(rules)}
	for _, rule := range rules {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		sink, err := sinkRepo.GetByID(rule.SinkID)
		if err != nil {
			return result, err
		}
		if sink.Enabled == nil || !*sink.Enabled {
			continue
		}
		workspace, err := workspaceRepo.GetByID(rule.WorkspaceID)
		if err != nil {
			return result, err
		}
		tasks, err := schedulerTasksForRule(taskRepo, rule, now, s.batchSize, s.store.Dialect())
		if err != nil {
			return result, err
		}
		for _, tsk := range tasks {
			if rule.ProjectID != nil && (tsk.ProjectID == nil || *tsk.ProjectID != *rule.ProjectID) {
				continue
			}
			if !reminderRuleMatchesTask(rule, tsk, now) {
				continue
			}
			projectCtx, err := schedulerProjectContext(projectRepo, tsk.ProjectID)
			if err != nil {
				return result, err
			}
			recipients := recipientIDsForRule(rule, tsk)
			if len(recipients) == 0 {
				result.RecipientsSkipped++
				continue
			}
			userInfos, err := schedulerUserInfos(userRepo, extRepo, recipients)
			if err != nil {
				return result, err
			}
			for _, userID := range recipients {
				userInfo := userInfos[userID]
				configValues, secretValues, err := schedulerNotificationConfigValues(configRepo, configDefRepo, rule.WorkspaceID, tsk.ProjectID, sink)
				if err != nil {
					return result, err
				}
				delivery, err := buildNotificationDeliveryForReminder(s.store.DB(), rule, sink, workspace, projectCtx, tsk, userInfo, configValues, secretValues, now)
				if err != nil {
					result.RecipientsSkipped++
					continue
				}
				inserted, err := schedulerEnqueueIfMissing(deliveryRepo, delivery)
				if err != nil {
					return result, err
				}
				if inserted {
					result.DeliveriesEnqueued++
				}
			}
		}
	}
	return result, nil
}

func schedulerTasksForRule(repo *storage.TaskRepository, rule storage.ReminderRule, now int64, limit int, dialect string) ([]task.Task, error) {
	if reminderRuleUsesFilter(rule) {
		if !scheduledRuleDueForRun(rule, now, time.Local) {
			return nil, nil
		}
		expr, err := query.ParseQuery(rule.FilterSource)
		if err != nil {
			return nil, err
		}
		if rule.ProjectID != nil && *rule.ProjectID != "" {
			expr = query.And(expr, query.Predicate{Attribute: query.AttrProjectID, Operator: query.OpEqual, Value: query.StringValue(*rule.ProjectID)})
		}
		return repo.List(rule.WorkspaceID, storage.ListOptions{
			Status:  task.StatusPending,
			Query:   expr,
			NowUnix: now,
			Limit:   limit,
			Dialect: dialect,
		})
	}
	return repo.List(rule.WorkspaceID, storage.ListOptions{Status: task.StatusPending, Limit: limit, Dialect: dialect})
}

func schedulerEnqueueIfMissing(repo *storage.NotificationDeliveryRepository, delivery storage.NotificationDelivery) (bool, error) {
	exists, err := repo.ExistsByDedupeKey(delivery.DedupeKey)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	if err := repo.Enqueue([]storage.NotificationDelivery{delivery}); err != nil {
		return false, err
	}
	return true, nil
}

func schedulerProjectContext(repo *storage.ProjectRepository, projectID *string) (*NotificationProjectContext, error) {
	if projectID == nil || *projectID == "" {
		return nil, nil
	}
	project, err := repo.GetByID(*projectID)
	if err != nil {
		return nil, err
	}
	return &NotificationProjectContext{ID: project.ID, Slug: project.Slug, Name: project.Name}, nil
}

func reminderRuleMatchesTask(rule storage.ReminderRule, tsk task.Task, now int64) bool {
	if reminderRuleUsesFilter(rule) {
		return true
	}
	if tsk.Status != task.StatusPending || tsk.Due == nil {
		return false
	}
	switch rule.TriggerType {
	case "due_before":
		return now >= *tsk.Due-rule.OffsetSeconds && now < *tsk.Due
	case "overdue":
		return now >= *tsk.Due+rule.AfterSeconds
	default:
		return false
	}
}

func reminderRuleUsesFilter(rule storage.ReminderRule) bool {
	return strings.TrimSpace(rule.FilterSource) != ""
}

func scheduledRuleDueForRun(rule storage.ReminderRule, now int64, loc *time.Location) bool {
	if !reminderRuleUsesFilter(rule) {
		return true
	}
	scheduleType := strings.TrimSpace(rule.ScheduleType)
	scheduleValue := strings.TrimSpace(rule.ScheduleValue)
	if strings.HasPrefix(scheduleType, "daily@") && scheduleValue == "" {
		scheduleValue = strings.TrimPrefix(scheduleType, "daily@")
		scheduleType = "daily_at"
	}
	if scheduleType != "daily_at" {
		return false
	}
	parsed, err := time.Parse("15:04", scheduleValue)
	if err != nil {
		return false
	}
	current := time.Unix(now, 0).In(loc)
	scheduled := time.Date(current.Year(), current.Month(), current.Day(), parsed.Hour(), parsed.Minute(), 0, 0, loc)
	return !current.Before(scheduled)
}

func recipientIDsForRule(rule storage.ReminderRule, tsk task.Task) []string {
	seen := map[string]bool{}
	var out []string
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	if rule.AudienceType == "assignees" || rule.AudienceType == "assignees_and_explicit_users" {
		for _, assignee := range tsk.Assignees {
			add(assignee.UserID)
		}
	}
	if rule.AudienceType == "explicit_users" || rule.AudienceType == "assignees_and_explicit_users" {
		for _, id := range decodeStringListNoError(rule.RecipientUserIDsJSON) {
			add(id)
		}
	}
	return out
}

func schedulerUserInfos(userRepo *storage.UserRepository, extRepo *storage.ExternalIDRepository, ids []string) (map[string]task.UserInfo, error) {
	result := make(map[string]task.UserInfo, len(ids))
	exts, err := extRepo.ListByUsers(ids)
	if err != nil {
		return nil, err
	}
	extByUser := map[string][]task.ExternalIDInfo{}
	for _, ext := range exts {
		extByUser[ext.UserID] = append(extByUser[ext.UserID], task.ExternalIDInfo{Provider: ext.Provider, ExternalID: ext.ExternalID})
	}
	for _, id := range ids {
		user, err := userRepo.GetByID(id)
		if err == storage.ErrNotFound {
			result[id] = task.UserInfo{ID: id, Name: id}
			continue
		}
		if err != nil {
			return nil, err
		}
		result[id] = task.UserInfo{ID: user.ID, Name: user.Name, DisplayName: user.DisplayName, Email: user.Email, ExternalIDs: extByUser[user.ID]}
	}
	return result, nil
}

func schedulerNotificationConfigValues(configRepo *storage.ConfigRepository, configDefRepo *storage.ConfigDefinitionRepository, workspaceID string, projectID *string, sink storage.NotificationSink) (map[string]string, map[string]string, error) {
	configValues := map[string]string{}
	secretValues := map[string]string{}
	if sink.EndpointMode == NotificationEndpointConfigValue && sink.ConfigKey != "" {
		value, ok, err := schedulerScopedConfigValue(configRepo, configDefRepo, workspaceID, projectID, sink.ConfigKey)
		if err != nil {
			return nil, nil, err
		}
		if ok {
			configValues[sink.ConfigKey] = value
		}
	}
	for _, ref := range decodeSecretRefsNoError(sink.SecretRefsJSON) {
		value, ok, err := schedulerScopedConfigValue(configRepo, configDefRepo, workspaceID, projectID, ref.ConfigKey)
		if err != nil {
			return nil, nil, err
		}
		if ok {
			secretValues[ref.Alias] = value
		}
	}
	return configValues, secretValues, nil
}

func schedulerScopedConfigValue(configRepo *storage.ConfigRepository, configDefRepo *storage.ConfigDefinitionRepository, workspaceID string, projectID *string, key string) (string, bool, error) {
	if projectID != nil && *projectID != "" {
		value, ok, err := configRepo.Get(storage.ConfigKey{
			WorkspaceID: workspaceID,
			Scope:       storage.ConfigScopeProject,
			ScopeID:     *projectID,
			Key:         key,
		})
		if err != nil || ok {
			return value, ok, err
		}
	}
	value, ok, err := configRepo.Get(storage.ConfigKey{
		WorkspaceID: workspaceID,
		Scope:       storage.ConfigScopeWorkspace,
		ScopeID:     workspaceID,
		Key:         key,
	})
	if err != nil || ok {
		return value, ok, err
	}
	def, ok, err := configDefRepo.Get(workspaceID, key)
	if err != nil || !ok || !def.HasDefault {
		return "", false, err
	}
	return def.DefaultValue, true, nil
}

func buildNotificationDeliveryForReminder(db *gorm.DB, rule storage.ReminderRule, sink storage.NotificationSink, workspace storage.Workspace, project *NotificationProjectContext, tsk task.Task, recipient task.UserInfo, configValues map[string]string, secretValues map[string]string, now int64) (storage.NotificationDelivery, error) {
	sinkView := notificationSinkViewFromRow(sink, task.UserInfo{ID: sink.CreatedBy, Name: sink.CreatedBy})
	eventType := reminderEventType(rule, tsk, now)
	deliveryID := uuid.NewString()
	eventID := uuid.NewString()
	windowStart, windowEnd := reminderWindow(rule, tsk, now, time.Local)
	sequence, err := reminderDeliverySequence(db, rule, tsk, recipient)
	if err != nil {
		return storage.NotificationDelivery{}, err
	}
	overdueSequence := int64(0)
	if reminderIsOverdue(rule, tsk, now, eventType) {
		overdueSequence = sequence
	}
	req, err := ResolveNotificationRequest(NotificationRequestResolveInput{
		Sink:         sinkView,
		Workspace:    NotificationWorkspaceContext{ID: workspace.ID, Slug: workspace.Slug, Name: workspace.Name},
		Project:      project,
		Rule:         NotificationRuleContext{ID: rule.ID, Name: rule.Name, TriggerType: rule.TriggerType},
		Task:         NotificationTaskContext{UUID: tsk.UUID, TaskSlug: taskRefForNotification(tsk), Title: tsk.Title, Description: optionalTextValue(tsk.Description), Status: tsk.Status, Due: tsk.Due},
		Recipient:    recipient,
		Reminder:     NotificationReminderContext{Sequence: sequence, OverdueSequence: overdueSequence, WindowStart: windowStart, WindowEnd: windowEnd},
		Event:        NotificationEventContext{ID: eventID, Type: eventType, Version: 1, ObjectKind: "task", ObjectID: tsk.UUID},
		EventType:    eventType,
		Delivery:     NotificationDeliveryContext{ID: deliveryID, Attempt: 1, WorkspaceID: rule.WorkspaceID, SinkID: sink.ID},
		Object:       NotificationObjectContext{Kind: "task", ID: tsk.UUID},
		ConfigValues: configValues,
		SecretValues: secretValues,
	})
	if err != nil {
		return storage.NotificationDelivery{}, err
	}
	window := scheduleDateKey(rule, tsk, now, time.Local)
	return storage.NotificationDelivery{
		ID:                          deliveryID,
		WorkspaceID:                 rule.WorkspaceID,
		ProjectID:                   rule.ProjectID,
		RuleID:                      rule.ID,
		SinkID:                      sink.ID,
		TaskUUID:                    tsk.UUID,
		ObjectKind:                  "task",
		ObjectID:                    tsk.UUID,
		RecipientUserID:             recipient.ID,
		EventID:                     eventID,
		EventType:                   eventType,
		DedupeKey:                   fmt.Sprintf("%s:%s:%s:%s:%s", rule.WorkspaceID, rule.ID, tsk.UUID, recipient.ID, window),
		ResolvedURL:                 req.ResolvedURL,
		ResolvedEndpointSource:      req.ResolvedEndpointSource,
		ResolvedEndpointFingerprint: req.ResolvedEndpointFingerprint,
		RenderedMethod:              req.RenderedMethod,
		RenderedHeadersJSON:         req.RenderedHeadersJSON,
		RenderedBody:                req.RenderedBody,
		RenderedContentType:         req.RenderedContentType,
		PayloadJSON:                 req.PayloadJSON,
		Status:                      storage.DeliveryStatusQueued,
		CreatedAt:                   now,
		ModifiedAt:                  now,
	}, nil
}

func reminderDeliverySequence(db *gorm.DB, rule storage.ReminderRule, tsk task.Task, recipient task.UserInfo) (int64, error) {
	var count int64
	err := db.Model(&storage.NotificationDelivery{}).
		Where("workspace_id = ? AND rule_id = ? AND task_uuid = ? AND recipient_user_id = ?", rule.WorkspaceID, rule.ID, tsk.UUID, recipient.ID).
		Count(&count).Error
	if err != nil {
		return 0, err
	}
	return count + 1, nil
}

func reminderEventType(rule storage.ReminderRule, tsk task.Task, now int64) string {
	if reminderIsOverdue(rule, tsk, now, "") {
		return "task.overdue"
	}
	return "task.due_soon"
}

func reminderIsOverdue(rule storage.ReminderRule, tsk task.Task, now int64, eventType string) bool {
	if eventType == "task.overdue" || rule.TriggerType == "overdue" {
		return true
	}
	if reminderRuleUsesFilter(rule) {
		return tsk.Due != nil && *tsk.Due < now
	}
	return false
}

func reminderFilterIsDueBeforeNow(filterSource string) bool {
	expr, err := query.ParseQuery(filterSource)
	if err != nil {
		return false
	}
	return reminderExprContainsDueBeforeNow(expr)
}

func reminderExprContainsDueBeforeNow(expr query.Expr) bool {
	switch e := expr.(type) {
	case nil:
		return false
	case query.Predicate:
		return e.Attribute == query.AttrDue && e.Operator == query.OpBefore && e.Value.Raw == "now"
	case query.Binary:
		return reminderExprContainsDueBeforeNow(e.Left) || reminderExprContainsDueBeforeNow(e.Right)
	case query.Unary:
		return false
	default:
		return false
	}
}

func reminderWindow(rule storage.ReminderRule, tsk task.Task, now int64, loc *time.Location) (int64, int64) {
	scheduleType := strings.TrimSpace(rule.ScheduleType)
	if reminderRuleUsesFilter(rule) && (scheduleType == "daily_at" || strings.HasPrefix(scheduleType, "daily@")) {
		return dailyReminderWindow(rule, now, loc)
	}
	start := reminderWindowStart(rule, tsk, now)
	return start, start + int64((24 * time.Hour).Seconds())
}

func dailyReminderWindowStart(rule storage.ReminderRule, now int64, loc *time.Location) int64 {
	start, _ := dailyReminderWindow(rule, now, loc)
	return start
}

func dailyReminderWindow(rule storage.ReminderRule, now int64, loc *time.Location) (int64, int64) {
	if loc == nil {
		loc = time.Local
	}
	scheduleType := strings.TrimSpace(rule.ScheduleType)
	scheduleValue := strings.TrimSpace(rule.ScheduleValue)
	if strings.HasPrefix(scheduleType, "daily@") && scheduleValue == "" {
		scheduleValue = strings.TrimPrefix(scheduleType, "daily@")
	}
	parsed, err := time.Parse("15:04", scheduleValue)
	if err != nil {
		return now, now + int64((24 * time.Hour).Seconds())
	}
	current := time.Unix(now, 0).In(loc)
	startTime := time.Date(current.Year(), current.Month(), current.Day(), parsed.Hour(), parsed.Minute(), 0, 0, loc)
	nextDay := startTime.AddDate(0, 0, 1)
	endTime := time.Date(nextDay.Year(), nextDay.Month(), nextDay.Day(), parsed.Hour(), parsed.Minute(), 0, 0, loc)
	return startTime.Unix(), endTime.Unix()
}

func reminderWindowStart(rule storage.ReminderRule, tsk task.Task, now int64) int64 {
	if reminderRuleUsesFilter(rule) {
		return now
	}
	if tsk.Due == nil {
		return now
	}
	switch {
	case rule.TriggerType == "due_before":
		return *tsk.Due - rule.OffsetSeconds
	case rule.RepeatPolicy == "once" || rule.RepeatPolicy == "":
		return *tsk.Due + rule.AfterSeconds
	case strings.HasPrefix(rule.RepeatPolicy, "every:"):
		duration, err := time.ParseDuration(strings.TrimPrefix(rule.RepeatPolicy, "every:"))
		if err != nil || duration <= 0 {
			return *tsk.Due + rule.AfterSeconds
		}
		base := *tsk.Due + rule.AfterSeconds
		step := int64(duration.Seconds())
		return base + ((now-base)/step)*step
	default:
		return *tsk.Due + rule.AfterSeconds
	}
}

func scheduleDateKey(rule storage.ReminderRule, tsk task.Task, now int64, loc *time.Location) string {
	scheduleType := strings.TrimSpace(rule.ScheduleType)
	if reminderRuleUsesFilter(rule) && (scheduleType == "daily_at" || strings.HasPrefix(scheduleType, "daily@")) {
		return time.Unix(now, 0).In(loc).Format("2006-01-02")
	}
	return fmt.Sprintf("%d", reminderWindowStart(rule, tsk, now))
}

func taskRefForNotification(tsk task.Task) string {
	if tsk.Project != nil && tsk.ProjectSeq != nil {
		return fmt.Sprintf("%s-%d", *tsk.Project, *tsk.ProjectSeq)
	}
	return tsk.UUID
}
