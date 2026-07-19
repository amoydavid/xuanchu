package app

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"git.dajee.net/dajee/xuanchu/internal/authz"
	"git.dajee.net/dajee/xuanchu/internal/query"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

type EventNotificationRuleAddInput struct {
	Name            string
	ProjectRef      string
	EventType       string
	FilterSource    string
	AudienceType    string
	Recipients      []string
	SinkRef         string
	TemplateSubject string
	TemplateBody    string
}

type EventNotificationRuleModifyInput struct {
	Name            *string
	ProjectRef      *string
	EventType       *string
	FilterSource    *string
	AudienceType    *string
	Recipients      *[]string
	SinkRef         *string
	TemplateSubject *string
	TemplateBody    *string
}

type EventNotificationRuleView struct {
	ID               string          `json:"id"`
	WorkspaceID      string          `json:"workspace_id"`
	ProjectID        *string         `json:"project_id"`
	Name             string          `json:"name"`
	Enabled          bool            `json:"enabled"`
	EventType        string          `json:"event_type"`
	FilterSource     string          `json:"filter_source"`
	AudienceType     string          `json:"audience_type"`
	RecipientUserIDs []string        `json:"-"`
	RecipientUsers   []task.UserInfo `json:"recipient_users"`
	SinkID           string          `json:"sink_id"`
	TemplateSubject  string          `json:"template_subject"`
	TemplateBody     string          `json:"template_body"`
	CreatedBy        task.ActorInfo  `json:"created_by"`
	CreatedAt        int64           `json:"created_at"`
	ModifiedAt       int64           `json:"modified_at"`
}

func (s *Service) AddEventNotificationRule(input EventNotificationRuleAddInput) (EventNotificationRuleView, error) {
	if err := s.Require(PermissionNotificationWrite); err != nil {
		return EventNotificationRuleView{}, err
	}
	var projectID *string
	if input.ProjectRef != "" {
		project, err := s.ResolveProject(input.ProjectRef)
		if err != nil {
			return EventNotificationRuleView{}, err
		}
		projectID = &project.ID
	} else if s.hasProjectScope() {
		return EventNotificationRuleView{}, RuntimeError{Code: authz.CodeProjectScopeDenied, Message: "token cannot create workspace scoped notification rule"}
	}
	name, sink, recipientIDs, eventType, filterSource, audience, err := s.normalizeEventNotificationRuleFields(input)
	if err != nil {
		return EventNotificationRuleView{}, err
	}
	now := s.clock.Unix()
	enabled := true
	row := storage.EventNotificationRule{
		ID:                   uuid.NewString(),
		WorkspaceID:          s.workspaceID,
		ProjectID:            projectID,
		Name:                 name,
		Enabled:              &enabled,
		EventType:            eventType,
		FilterSource:         filterSource,
		AudienceType:         audience,
		RecipientUserIDsJSON: mustJSON(recipientIDs),
		SinkID:               sink.ID,
		TemplateSubject:      strings.TrimSpace(input.TemplateSubject),
		TemplateBody:         strings.TrimSpace(input.TemplateBody),
		CreatedBy:            s.runtime.ActorUserID,
		CreatedAt:            now,
		ModifiedAt:           now,
	}
	actor := s.runtime.actorColumns()
	row.CreatedByActorType = actor.Type
	row.CreatedByUserID = actor.UserID
	row.CreatedByTokenID = actor.TokenID
	row.CreatedByTokenName = actor.TokenName
	row.CreatedByTokenPrefix = actor.TokenPrefix
	var view EventNotificationRuleView
	err = s.withAudit("notification.rule.create", func(tx *Service) (AuditEntry, error) {
		if err := tx.eventNotificationRuleRepo.Create(row); err != nil {
			return AuditEntry{}, err
		}
		created, err := tx.eventNotificationRuleRepo.GetByID(row.ID)
		if err != nil {
			return AuditEntry{}, err
		}
		view, err = tx.eventNotificationRuleViewFromRow(created)
		if err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{
			TargetType: "event_notification_rule",
			TargetID:   created.ID,
			Payload: map[string]any{
				"name":       created.Name,
				"event_type": created.EventType,
				"audience":   created.AudienceType,
				"sink_id":    created.SinkID,
			},
		}, nil
	})
	return view, err
}

func (s *Service) ListEventNotificationRules(projectRef string, includeDisabled bool) ([]EventNotificationRuleView, error) {
	if err := s.Require(PermissionNotificationRead); err != nil {
		return nil, err
	}
	var projectID *string
	if projectRef != "" {
		project, err := s.ResolveProject(projectRef)
		if err != nil {
			return nil, err
		}
		projectID = &project.ID
	}
	rows, err := s.eventNotificationRuleRepo.List(s.workspaceID, projectID, includeDisabled)
	if err != nil {
		return nil, err
	}
	userInfos, err := s.resolveUserInfos(eventNotificationRuleUserIDs(rows))
	if err != nil {
		return nil, err
	}
	out := make([]EventNotificationRuleView, 0, len(rows))
	for _, row := range rows {
		if !s.allowsProjectID(row.ProjectID) {
			continue
		}
		out = append(out, eventNotificationRuleViewFromRow(row, userInfos))
	}
	return out, nil
}

func (s *Service) EventNotificationRuleInfo(ruleID string) (EventNotificationRuleView, error) {
	if err := s.Require(PermissionNotificationRead); err != nil {
		return EventNotificationRuleView{}, err
	}
	row, err := s.eventNotificationRuleRepo.GetByID(ruleID)
	if err == storage.ErrNotFound {
		return EventNotificationRuleView{}, RuntimeError{Code: "notification_rule_not_found", Message: "notification rule not found"}
	}
	if err != nil {
		return EventNotificationRuleView{}, err
	}
	if row.WorkspaceID != s.workspaceID {
		return EventNotificationRuleView{}, RuntimeError{Code: "notification_rule_not_found", Message: "notification rule not found"}
	}
	if !s.allowsProjectID(row.ProjectID) {
		return EventNotificationRuleView{}, RuntimeError{Code: "notification_rule_not_found", Message: "notification rule not found"}
	}
	return s.eventNotificationRuleViewFromRow(row)
}

func (s *Service) ModifyEventNotificationRule(ruleID string, input EventNotificationRuleModifyInput) (EventNotificationRuleView, error) {
	if err := s.Require(PermissionNotificationWrite); err != nil {
		return EventNotificationRuleView{}, err
	}
	row, err := s.eventNotificationRuleRepo.GetByID(ruleID)
	if err == storage.ErrNotFound {
		return EventNotificationRuleView{}, RuntimeError{Code: "notification_rule_not_found", Message: "notification rule not found"}
	}
	if err != nil {
		return EventNotificationRuleView{}, err
	}
	if row.WorkspaceID != s.workspaceID {
		return EventNotificationRuleView{}, RuntimeError{Code: "notification_rule_not_found", Message: "notification rule not found"}
	}
	if !s.allowsProjectID(row.ProjectID) {
		return EventNotificationRuleView{}, RuntimeError{Code: authz.CodeProjectScopeDenied, Message: "token cannot access project"}
	}
	candidate := eventNotificationRuleAddInputFromRow(row)
	var projectID = row.ProjectID
	if input.ProjectRef != nil {
		candidate.ProjectRef = strings.TrimSpace(*input.ProjectRef)
		if candidate.ProjectRef == "" {
			if s.hasProjectScope() {
				return EventNotificationRuleView{}, RuntimeError{Code: authz.CodeProjectScopeDenied, Message: "token cannot create workspace scoped notification rule"}
			}
			projectID = nil
		} else {
			project, err := s.ResolveProject(candidate.ProjectRef)
			if err != nil {
				return EventNotificationRuleView{}, err
			}
			projectID = &project.ID
		}
	}
	applyEventNotificationRuleModifyInput(&candidate, input)
	name, sink, recipientIDs, eventType, filterSource, audience, err := s.normalizeEventNotificationRuleFields(candidate)
	if err != nil {
		return EventNotificationRuleView{}, err
	}
	if input.ProjectRef == nil {
		projectID = row.ProjectID
	}
	row.Name = name
	row.ProjectID = projectID
	row.EventType = eventType
	row.FilterSource = filterSource
	row.AudienceType = audience
	row.RecipientUserIDsJSON = mustJSON(recipientIDs)
	row.SinkID = sink.ID
	row.TemplateSubject = strings.TrimSpace(candidate.TemplateSubject)
	row.TemplateBody = strings.TrimSpace(candidate.TemplateBody)
	row.ModifiedAt = s.clock.Unix()
	var view EventNotificationRuleView
	err = s.withAudit("notification.rule.modify", func(tx *Service) (AuditEntry, error) {
		if err := tx.eventNotificationRuleRepo.Update(row); err != nil {
			return AuditEntry{}, err
		}
		updated, err := tx.eventNotificationRuleRepo.GetByID(ruleID)
		if err != nil {
			return AuditEntry{}, err
		}
		view, err = tx.eventNotificationRuleViewFromRow(updated)
		if err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{
			TargetType: "event_notification_rule",
			TargetID:   ruleID,
			Payload: map[string]any{
				"name":       updated.Name,
				"event_type": updated.EventType,
				"audience":   updated.AudienceType,
				"sink_id":    updated.SinkID,
			},
		}, nil
	})
	return view, err
}

func (s *Service) EnableEventNotificationRule(ruleID string) (EventNotificationRuleView, error) {
	if err := s.Require(PermissionNotificationWrite); err != nil {
		return EventNotificationRuleView{}, err
	}
	return s.toggleEventNotificationRule(ruleID, true, "notification.rule.enable")
}

func (s *Service) DisableEventNotificationRule(ruleID string) (EventNotificationRuleView, error) {
	if err := s.Require(PermissionNotificationWrite); err != nil {
		return EventNotificationRuleView{}, err
	}
	return s.toggleEventNotificationRule(ruleID, false, "notification.rule.disable")
}

func (s *Service) DeleteEventNotificationRule(ruleID string) error {
	if err := s.Require(PermissionNotificationWrite); err != nil {
		return err
	}
	row, err := s.eventNotificationRuleRepo.GetByID(ruleID)
	if err == storage.ErrNotFound {
		return RuntimeError{Code: "notification_rule_not_found", Message: "notification rule not found"}
	}
	if err != nil {
		return err
	}
	if row.WorkspaceID != s.workspaceID {
		return RuntimeError{Code: "notification_rule_not_found", Message: "notification rule not found"}
	}
	if !s.allowsProjectID(row.ProjectID) {
		return RuntimeError{Code: authz.CodeProjectScopeDenied, Message: "token cannot access project"}
	}
	return s.withAudit("notification.rule.delete", func(tx *Service) (AuditEntry, error) {
		if err := tx.eventNotificationRuleRepo.Delete(ruleID); err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{TargetType: "event_notification_rule", TargetID: ruleID, Payload: map[string]any{"name": row.Name}}, nil
	})
}

func (s *Service) toggleEventNotificationRule(ruleID string, enabled bool, action string) (EventNotificationRuleView, error) {
	row, err := s.eventNotificationRuleRepo.GetByID(ruleID)
	if err == storage.ErrNotFound {
		return EventNotificationRuleView{}, RuntimeError{Code: "notification_rule_not_found", Message: "notification rule not found"}
	}
	if err != nil {
		return EventNotificationRuleView{}, err
	}
	if row.WorkspaceID != s.workspaceID {
		return EventNotificationRuleView{}, RuntimeError{Code: "notification_rule_not_found", Message: "notification rule not found"}
	}
	if !s.allowsProjectID(row.ProjectID) {
		return EventNotificationRuleView{}, RuntimeError{Code: authz.CodeProjectScopeDenied, Message: "token cannot access project"}
	}
	row.Enabled = &enabled
	row.ModifiedAt = s.clock.Unix()
	var view EventNotificationRuleView
	err = s.withAudit(action, func(tx *Service) (AuditEntry, error) {
		if err := tx.eventNotificationRuleRepo.Update(row); err != nil {
			return AuditEntry{}, err
		}
		updated, err := tx.eventNotificationRuleRepo.GetByID(ruleID)
		if err != nil {
			return AuditEntry{}, err
		}
		view, err = tx.eventNotificationRuleViewFromRow(updated)
		if err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{TargetType: "event_notification_rule", TargetID: ruleID, Payload: map[string]any{"enabled": enabled}}, nil
	})
	return view, err
}

func (s *Service) normalizeEventNotificationRuleFields(input EventNotificationRuleAddInput) (string, storage.NotificationSink, []string, string, string, string, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return "", storage.NotificationSink{}, nil, "", "", "", RuntimeError{Code: "notification_rule_invalid", Message: "rule name is required"}
	}
	eventType := strings.TrimSpace(input.EventType)
	if !allowedHookEventTypes[eventType] {
		return "", storage.NotificationSink{}, nil, "", "", "", RuntimeError{Code: "notification_rule_invalid", Message: "unsupported event"}
	}
	filterSource := strings.TrimSpace(input.FilterSource)
	if filterSource != "" {
		if !strings.HasPrefix(eventType, "task.") {
			return "", storage.NotificationSink{}, nil, "", "", "", RuntimeError{Code: "notification_rule_invalid", Message: "filter is only supported for task events"}
		}
		if err := validateEventNotificationFilter(filterSource); err != nil {
			return "", storage.NotificationSink{}, nil, "", "", "", err
		}
	}
	audience := strings.TrimSpace(input.AudienceType)
	if err := validateEventNotificationAudience(eventType, audience); err != nil {
		return "", storage.NotificationSink{}, nil, "", "", "", err
	}
	sink, err := s.resolveNotificationSink(input.SinkRef)
	if err != nil {
		return "", storage.NotificationSink{}, nil, "", "", "", err
	}
	recipientIDs, err := s.resolveReminderRecipientIDs(input.Recipients)
	if err != nil {
		return "", storage.NotificationSink{}, nil, "", "", "", err
	}
	return name, sink, recipientIDs, eventType, filterSource, audience, nil
}

func validateEventNotificationAudience(eventType, audience string) error {
	switch audience {
	case "actor", "explicit_users":
		return nil
	case "assignees", "assignees_and_explicit_users":
		if strings.HasPrefix(eventType, "task.") {
			return nil
		}
		return RuntimeError{Code: "audience_unsupported_for_event", Message: "audience is unsupported for this event"}
	case "mentioned_users":
		// mentioned_users 只允许用于 task.user_mentioned。
		if eventType == "task.user_mentioned" {
			return nil
		}
		return RuntimeError{Code: "audience_unsupported_for_event", Message: "audience is unsupported for this event"}
	default:
		return RuntimeError{Code: "notification_rule_invalid", Message: "unsupported audience"}
	}
}

func validateEventNotificationFilter(source string) error {
	if _, err := query.ParseQuery(source); err != nil {
		return RuntimeError{Code: "notification_rule_invalid", Message: "invalid filter"}
	}
	return nil
}

func eventNotificationRuleAddInputFromRow(row storage.EventNotificationRule) EventNotificationRuleAddInput {
	return EventNotificationRuleAddInput{
		Name:            row.Name,
		EventType:       row.EventType,
		FilterSource:    row.FilterSource,
		AudienceType:    row.AudienceType,
		Recipients:      decodeStringListNoError(row.RecipientUserIDsJSON),
		SinkRef:         row.SinkID,
		TemplateSubject: row.TemplateSubject,
		TemplateBody:    row.TemplateBody,
	}
}

func applyEventNotificationRuleModifyInput(input *EventNotificationRuleAddInput, mod EventNotificationRuleModifyInput) {
	if mod.Name != nil {
		input.Name = *mod.Name
	}
	if mod.EventType != nil {
		input.EventType = *mod.EventType
	}
	if mod.FilterSource != nil {
		input.FilterSource = *mod.FilterSource
	}
	if mod.AudienceType != nil {
		input.AudienceType = *mod.AudienceType
	}
	if mod.Recipients != nil {
		input.Recipients = *mod.Recipients
	}
	if mod.SinkRef != nil {
		input.SinkRef = *mod.SinkRef
	}
	if mod.TemplateSubject != nil {
		input.TemplateSubject = *mod.TemplateSubject
	}
	if mod.TemplateBody != nil {
		input.TemplateBody = *mod.TemplateBody
	}
}

func (s *Service) enqueueEventNotificationDeliveries(events []HookEvent) error {
	if len(events) == 0 {
		return nil
	}
	for _, event := range events {
		if event.ActorType == "" {
			continue
		}
		rules, err := s.eventNotificationRuleRepo.ListMatching(event.WorkspaceID, event.ProjectID, event.EventType)
		if err != nil {
			return err
		}
		if len(rules) == 0 {
			continue
		}
		var deliveries []storage.NotificationDelivery
		for _, rule := range rules {
			if rule.WorkspaceID != s.workspaceID || !s.allowsProjectID(rule.ProjectID) {
				continue
			}
			matches, err := s.eventNotificationRuleMatchesFilter(rule, event)
			if err != nil {
				return err
			}
			if !matches {
				continue
			}
			rows, err := s.eventNotificationDeliveriesForRule(rule, event)
			if err != nil {
				return err
			}
			deliveries = append(deliveries, rows...)
		}
		if err := s.notificationDeliveryRepo.Enqueue(deliveries); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) eventNotificationRuleMatchesFilter(rule storage.EventNotificationRule, event HookEvent) (bool, error) {
	filterSource := strings.TrimSpace(rule.FilterSource)
	if filterSource == "" {
		return true, nil
	}
	if event.ObjectKind != "task" || event.ObjectID == "" {
		return false, nil
	}
	expr, err := query.ParseQuery(filterSource)
	if err != nil {
		return false, RuntimeError{Code: "notification_rule_invalid", Message: "invalid filter"}
	}
	expr, err = s.resolveTaskQueryPredicates(expr)
	if err != nil {
		return false, err
	}
	expr = query.And(expr, query.Predicate{Attribute: query.AttrUUID, Operator: query.OpEqual, Value: query.StringValue(event.ObjectID)})
	if rule.ProjectID != nil {
		expr = query.And(expr, query.Predicate{Attribute: query.AttrProjectID, Operator: query.OpEqual, Value: query.StringValue(*rule.ProjectID)})
	}
	rows, err := s.repo.List(s.workspaceID, storage.ListOptions{
		Query:   query.And(s.projectScopeExpr(), expr),
		NowUnix: s.clock.Unix(),
		Dialect: s.store.Dialect(),
		Limit:   1,
	})
	if err != nil {
		return false, mapProjectQueryCompileError(err)
	}
	return len(rows) > 0, nil
}

func (s *Service) eventNotificationDeliveriesForRule(rule storage.EventNotificationRule, event HookEvent) ([]storage.NotificationDelivery, error) {
	sink, err := s.notificationSinkRepo.GetByID(rule.SinkID)
	if err == storage.ErrNotFound {
		return nil, RuntimeError{Code: "notification_sink_not_found", Message: "notification sink not found"}
	}
	if err != nil {
		return nil, err
	}
	if sink.WorkspaceID != event.WorkspaceID || sink.WorkspaceID != s.workspaceID {
		return nil, RuntimeError{Code: "notification_sink_not_found", Message: "notification sink not found"}
	}
	workspace, err := s.workspaceRepo.GetByID(event.WorkspaceID)
	if err != nil {
		return nil, err
	}
	var projectCtx *NotificationProjectContext
	if event.ProjectID != nil && *event.ProjectID != "" {
		project, err := s.projectRepo.GetByID(*event.ProjectID)
		if err != nil {
			return nil, err
		}
		projectCtx = &NotificationProjectContext{ID: project.ID, Slug: project.Slug, Name: project.Name}
	}
	recipientIDs, err := s.eventNotificationRecipientIDs(rule, event)
	if err != nil {
		return nil, err
	}
	if len(recipientIDs) == 0 {
		return nil, nil
	}
	userIDs := append(hookEventActorUserIDs(event), recipientIDs...)
	userInfos, err := s.resolveUserInfos(userIDs)
	if err != nil {
		return nil, err
	}
	actor := hookEventActorAsUserInfo(event, userInfos)
	configValues, secretValues, err := schedulerNotificationConfigValues(s.configRepo, s.configDefRepo, event.WorkspaceID, event.ProjectID, sink)
	if err != nil {
		return nil, err
	}
	sinkView := notificationSinkViewFromRow(sink, actorInfoFromColumns(notificationSinkActorColumns(sink), sink.CreatedBy, nil))
	now := s.clock.Unix()
	out := make([]storage.NotificationDelivery, 0, len(recipientIDs))
	for _, recipientID := range recipientIDs {
		recipient := userInfos[recipientID]
		deliveryID := uuid.NewString()
		eventJSON, err := buildEventNotificationPayloadJSON(rule, event, workspace, projectCtx, actor, recipient, NotificationDeliveryContext{
			ID:          deliveryID,
			Attempt:     1,
			WorkspaceID: event.WorkspaceID,
			SinkID:      sink.ID,
		})
		if err != nil {
			return nil, err
		}
		req, err := ResolveNotificationRequest(NotificationRequestResolveInput{
			Sink:      sinkView,
			Workspace: NotificationWorkspaceContext{ID: workspace.ID, Slug: workspace.Slug, Name: workspace.Name},
			Project:   projectCtx,
			Rule:      NotificationRuleContext{ID: rule.ID, Name: rule.Name, TriggerType: "event"},
			Task:      eventNotificationTaskContext(event),
			Recipient: recipient,
			Actor:     actor,
			Event: NotificationEventContext{
				ID:         event.EventID,
				Type:       event.EventType,
				Version:    event.EventVersion,
				OccurredAt: event.OccurredAt,
				ObjectKind: event.ObjectKind,
				ObjectID:   event.ObjectID,
				JSON:       eventJSON,
			},
			EventType:    event.EventType,
			Delivery:     NotificationDeliveryContext{ID: deliveryID, Attempt: 1, WorkspaceID: event.WorkspaceID, SinkID: sink.ID},
			Object:       NotificationObjectContext{Kind: event.ObjectKind, ID: event.ObjectID},
			ConfigValues: configValues,
			SecretValues: secretValues,
		})
		if err != nil {
			return nil, err
		}
		out = append(out, storage.NotificationDelivery{
			ID:                          deliveryID,
			WorkspaceID:                 event.WorkspaceID,
			ProjectID:                   event.ProjectID,
			RuleID:                      rule.ID,
			SinkID:                      sink.ID,
			TaskUUID:                    eventNotificationTaskUUID(event),
			ObjectKind:                  event.ObjectKind,
			ObjectID:                    event.ObjectID,
			RecipientUserID:             recipient.ID,
			EventID:                     event.EventID,
			EventType:                   event.EventType,
			ActorType:                   event.ActorType,
			ActorUserID:                 stringPtrOrNil(event.ActorUserID),
			ActorTokenID:                stringPtrOrNil(event.ActorTokenID),
			ActorTokenName:              stringPtrOrNil(event.ActorTokenName),
			ActorTokenPrefix:            stringPtrOrNil(event.ActorTokenPrefix),
			DedupeKey:                   fmt.Sprintf("%s:%s:%s:%s", event.WorkspaceID, rule.ID, event.EventID, recipient.ID),
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
		})
	}
	return out, nil
}

func (s *Service) eventNotificationRecipientIDs(rule storage.EventNotificationRule, event HookEvent) ([]string, error) {
	seen := map[string]bool{}
	var ids []string
	add := func(id string) {
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if rule.AudienceType == "actor" {
		add(event.ActorUserID)
	}
	if rule.AudienceType == "explicit_users" || rule.AudienceType == "assignees_and_explicit_users" {
		for _, id := range decodeStringListNoError(rule.RecipientUserIDsJSON) {
			add(id)
		}
	}
	if rule.AudienceType == "assignees" || rule.AudienceType == "assignees_and_explicit_users" {
		tsk, ok := eventPrimaryTask(event)
		if !ok {
			return nil, RuntimeError{Code: "audience_unsupported_for_event", Message: "audience is unsupported for this event"}
		}
		for _, assignee := range tsk.Assignees {
			add(assignee.UserID)
		}
	}
	if rule.AudienceType == "mentioned_users" {
		// spec §16.2：从 event.Data["mentioned_users"] 解析 JSONUserInfo 列表。
		// actor 自己默认排除（actor 是 user 时）；actor 是 tenant/agent token 时 ActorUserID 为空，不排除。
		raw, ok := event.Data["mentioned_users"]
		if !ok {
			return []string{}, nil
		}
		bytes, err := json.Marshal(raw)
		if err != nil {
			return []string{}, nil
		}
		var jsonUsers []task.JSONUserInfo
		if err := json.Unmarshal(bytes, &jsonUsers); err != nil {
			return []string{}, nil
		}
		for _, ju := range jsonUsers {
			if ju.ID == "" {
				continue
			}
			if ju.ID == event.ActorUserID && event.ActorUserID != "" {
				continue
			}
			add(ju.ID)
		}
	}
	activeIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, err := s.memberRepo.Get(id, s.workspaceID); err == storage.ErrNotFound {
			continue
		} else if err != nil {
			return nil, err
		}
		activeIDs = append(activeIDs, id)
	}
	return activeIDs, nil
}

func buildEventNotificationPayloadJSON(rule storage.EventNotificationRule, event HookEvent, workspace storage.Workspace, project *NotificationProjectContext, actor task.UserInfo, recipient task.UserInfo, delivery NotificationDeliveryContext) (string, error) {
	payload := map[string]any{
		"delivery_id":   delivery.ID,
		"attempt":       delivery.Attempt,
		"workspace_id":  delivery.WorkspaceID,
		"sink_id":       delivery.SinkID,
		"rule_id":       rule.ID,
		"created_at":    event.OccurredAt,
		"delivery":      map[string]any{"id": delivery.ID, "attempt": delivery.Attempt, "workspace_id": delivery.WorkspaceID, "sink_id": delivery.SinkID},
		"object":        map[string]any{"kind": event.ObjectKind, "id": event.ObjectID},
		"event_id":      event.EventID,
		"event_type":    event.EventType,
		"event_version": event.EventVersion,
		"occurred_at":   event.OccurredAt,
		"workspace":     map[string]any{"id": workspace.ID, "slug": workspace.Slug, "name": workspace.Name},
		"rule":          map[string]any{"id": rule.ID, "name": rule.Name, "trigger_type": "event"},
		"actor":         task.UserInfoToJSON(actor),
		"recipient":     task.UserInfoToJSON(recipient),
		"object_kind":   event.ObjectKind,
		"object_id":     event.ObjectID,
		"data":          event.Data,
		"template":      map[string]any{"subject": rule.TemplateSubject, "body": rule.TemplateBody},
	}
	if project != nil {
		payload["project"] = map[string]any{"id": project.ID, "slug": project.Slug, "name": project.Name}
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func eventNotificationTaskContext(event HookEvent) NotificationTaskContext {
	tsk, ok := eventPrimaryTask(event)
	if !ok {
		return NotificationTaskContext{}
	}
	return NotificationTaskContext{
		UUID:        tsk.UUID,
		TaskSlug:    taskRefForNotification(tsk),
		Title:       tsk.Title,
		Description: optionalTextValue(tsk.Description),
		Status:      tsk.Status,
		Due:         tsk.Due,
	}
}

func eventNotificationTaskUUID(event HookEvent) string {
	if event.ObjectKind != "task" {
		return ""
	}
	return event.ObjectID
}

func eventPrimaryTask(event HookEvent) (task.Task, bool) {
	raw, ok := event.Data["task"]
	if !ok {
		return task.Task{}, false
	}
	jsonable, err := json.Marshal(raw)
	if err != nil {
		return task.Task{}, false
	}
	var dto task.JSONTask
	if err := json.Unmarshal(jsonable, &dto); err != nil {
		return task.Task{}, false
	}
	return task.FromJSON(dto), true
}

func (s *Service) eventNotificationRuleViewFromRow(row storage.EventNotificationRule) (EventNotificationRuleView, error) {
	userIDs := eventNotificationRuleCreatedByIDs([]storage.EventNotificationRule{row})
	userIDs = append(userIDs, decodeStringListNoError(row.RecipientUserIDsJSON)...)
	userInfos, err := s.resolveUserInfos(userIDs)
	if err != nil {
		return EventNotificationRuleView{}, err
	}
	return eventNotificationRuleViewFromRow(row, userInfos), nil
}

func eventNotificationRuleViewFromRow(row storage.EventNotificationRule, userInfos map[string]task.UserInfo) EventNotificationRuleView {
	enabled := row.Enabled != nil && *row.Enabled
	recipientIDs := decodeStringListNoError(row.RecipientUserIDsJSON)
	recipients := make([]task.UserInfo, 0, len(recipientIDs))
	for _, id := range recipientIDs {
		recipients = append(recipients, userInfos[id])
	}
	return EventNotificationRuleView{
		ID:               row.ID,
		WorkspaceID:      row.WorkspaceID,
		ProjectID:        row.ProjectID,
		Name:             row.Name,
		Enabled:          enabled,
		EventType:        row.EventType,
		FilterSource:     row.FilterSource,
		AudienceType:     row.AudienceType,
		RecipientUserIDs: recipientIDs,
		RecipientUsers:   recipients,
		SinkID:           row.SinkID,
		TemplateSubject:  row.TemplateSubject,
		TemplateBody:     row.TemplateBody,
		CreatedBy:        actorInfoFromColumns(eventNotificationRuleActorColumns(row), row.CreatedBy, userInfos),
		CreatedAt:        row.CreatedAt,
		ModifiedAt:       row.ModifiedAt,
	}
}

func eventNotificationRuleUserIDs(rows []storage.EventNotificationRule) []string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, eventNotificationRuleCreatedByIDs([]storage.EventNotificationRule{row})...)
		ids = append(ids, decodeStringListNoError(row.RecipientUserIDsJSON)...)
	}
	return ids
}

func eventNotificationRuleCreatedByIDs(rows []storage.EventNotificationRule) []string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		actorType := row.CreatedByActorType
		if actorType == "" {
			actorType = actorTypeUser
		}
		if actorType != actorTypeUser {
			continue
		}
		id := row.CreatedBy
		if row.CreatedByUserID != nil && *row.CreatedByUserID != "" {
			id = *row.CreatedByUserID
		}
		ids = append(ids, id)
	}
	return ids
}
