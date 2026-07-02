package app

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

// HookEvent 表示一个待投递的 hook 事件。
type HookEvent struct {
	EventID          string
	EventType        string
	EventVersion     int
	OccurredAt       int64
	ActorType        string
	ActorUserID      string
	ActorTokenID     string
	ActorTokenName   string
	ActorTokenPrefix string
	WorkspaceID      string
	WorkspaceSlug    string
	ProjectID        *string
	ProjectSlug      *string
	ObjectKind       string
	ObjectID         string
	Data             map[string]any
}

// buildTaskHookEvent 构建任务相关的 hook 事件。
func buildTaskHookEvent(eventType string, tsk task.Task, runtime RuntimeContext, now int64) HookEvent {
	var projectSlug *string
	if tsk.Project != nil {
		projectSlug = tsk.Project
	}
	event := HookEvent{
		EventID:       uuid.NewString(),
		EventType:     eventType,
		EventVersion:  1,
		OccurredAt:    now,
		WorkspaceID:   runtime.WorkspaceID,
		WorkspaceSlug: runtime.WorkspaceSlug,
		ProjectID:     tsk.ProjectID,
		ProjectSlug:   projectSlug,
		ObjectKind:    "task",
		ObjectID:      tsk.UUID,
		Data:          buildTaskPayload(tsk),
	}
	applyRuntimeActorToHookEvent(&event, runtime)
	return event
}

// buildProjectArchivedHookEvent 构建项目归档的 hook 事件。
func buildProjectArchivedHookEvent(pv ProjectView, runtime RuntimeContext, now int64) HookEvent {
	event := HookEvent{
		EventID:       uuid.NewString(),
		EventType:     "project.archived",
		EventVersion:  1,
		OccurredAt:    now,
		WorkspaceID:   pv.WorkspaceID,
		WorkspaceSlug: runtime.WorkspaceSlug,
		ProjectID:     &pv.ID,
		ProjectSlug:   &pv.Slug,
		ObjectKind:    "project",
		ObjectID:      pv.ID,
		Data:          buildProjectArchivedPayload(pv),
	}
	applyRuntimeActorToHookEvent(&event, runtime)
	return event
}

// buildProjectTransitionedHookEvent 构建项目状态转移的 hook 事件。
func buildProjectTransitionedHookEvent(pv ProjectView, fromStatus, toStatus string, runtime RuntimeContext, now int64) HookEvent {
	event := HookEvent{
		EventID:       uuid.NewString(),
		EventType:     "project.transitioned",
		EventVersion:  1,
		OccurredAt:    now,
		WorkspaceID:   pv.WorkspaceID,
		WorkspaceSlug: runtime.WorkspaceSlug,
		ProjectID:     &pv.ID,
		ProjectSlug:   &pv.Slug,
		ObjectKind:    "project",
		ObjectID:      pv.ID,
		Data: map[string]any{
			"project":     projectViewToMap(pv),
			"from_status": fromStatus,
			"to_status":   toStatus,
		},
	}
	applyRuntimeActorToHookEvent(&event, runtime)
	return event
}

func applyRuntimeActorToHookEvent(event *HookEvent, runtime RuntimeContext) {
	actor := runtime.actorColumns()
	event.ActorType = actor.Type
	if actor.UserID != nil {
		event.ActorUserID = *actor.UserID
	}
	if actor.TokenID != nil {
		event.ActorTokenID = *actor.TokenID
	}
	if actor.TokenName != nil {
		event.ActorTokenName = *actor.TokenName
	}
	if actor.TokenPrefix != nil {
		event.ActorTokenPrefix = *actor.TokenPrefix
	}
}

func buildTaskPayload(tsk task.Task) map[string]any {
	return map[string]any{
		"task":      task.ToJSON(tsk),
		"completed": tsk.Status == task.StatusCompleted,
		"deleted":   tsk.Status == task.StatusDeleted,
	}
}

func buildProjectArchivedPayload(pv ProjectView) map[string]any {
	return map[string]any{
		"project":  projectViewToMap(pv),
		"archived": true,
	}
}

func buildProjectAnnotatedHookEvent(pv ProjectView, annotation ProjectAnnotationInfo, runtime RuntimeContext, now int64) HookEvent {
	event := HookEvent{
		EventID:       uuid.NewString(),
		EventType:     "project.annotated",
		EventVersion:  1,
		OccurredAt:    now,
		WorkspaceID:   pv.WorkspaceID,
		WorkspaceSlug: runtime.WorkspaceSlug,
		ProjectID:     &pv.ID,
		ProjectSlug:   &pv.Slug,
		ObjectKind:    "project",
		ObjectID:      pv.ID,
		Data: map[string]any{
			"project": projectViewToMap(pv),
			"annotation": map[string]any{
				"id":              annotation.ID,
				"entry":           annotation.Entry,
				"content":         annotation.Content,
				"content_preview": truncateString(annotation.Content, 200),
				"created_by":      task.ActorInfoToJSON(annotation.CreatedBy),
				"created_at":      annotation.CreatedAt,
			},
		},
	}
	applyRuntimeActorToHookEvent(&event, runtime)
	return event
}

func buildProjectDenotatedHookEvent(pv ProjectView, annotationID string, runtime RuntimeContext, now int64) HookEvent {
	event := HookEvent{
		EventID:       uuid.NewString(),
		EventType:     "project.denotated",
		EventVersion:  1,
		OccurredAt:    now,
		WorkspaceID:   pv.WorkspaceID,
		WorkspaceSlug: runtime.WorkspaceSlug,
		ProjectID:     &pv.ID,
		ProjectSlug:   &pv.Slug,
		ObjectKind:    "project",
		ObjectID:      pv.ID,
		Data: map[string]any{
			"project":    projectViewToMap(pv),
			"annotation": map[string]any{"id": annotationID},
		},
	}
	applyRuntimeActorToHookEvent(&event, runtime)
	return event
}

func buildTaskUnblockedHookEvent(tsk task.Task, completedTask task.Task, runtime RuntimeContext, now int64) HookEvent {
	event := buildTaskHookEvent("task.unblocked", tsk, runtime, now)
	event.Data["dependency"] = map[string]any{
		"completed_task": task.ToJSON(completedTask),
	}
	return event
}

func buildTaskBlockedHookEvent(tsk task.Task, blockingDeps []string, runtime RuntimeContext, now int64) HookEvent {
	event := buildTaskHookEvent("task.blocked", tsk, runtime, now)
	event.Data["blocking_dependencies"] = blockingDeps
	return event
}

func projectViewToMap(pv ProjectView) map[string]any {
	return map[string]any{
		"id":           pv.ID,
		"workspace_id": pv.WorkspaceID,
		"slug":         pv.Slug,
		"name":         pv.Name,
		"description":  pv.Description,
		"status":       pv.Status,
		"task_count":   pv.TaskCount,
		"created_at":   pv.CreatedAt,
		"modified_at":  pv.ModifiedAt,
		"archived_at":  pv.ArchivedAt,
	}
}

// enqueueHookEvents 将事件匹配到 hook 并创建投递记录。
func (s *Service) enqueueHookEvents(events []HookEvent) error {
	if len(events) == 0 {
		return nil
	}
	for _, event := range events {
		if event.ActorType == "" {
			continue
		}
		hooks, err := s.matchingHooks(event)
		if err != nil {
			return err
		}
		if len(hooks) == 0 {
			continue
		}
		// 基础 headers（签名由 dispatcher 在发送时添加）
		headers := map[string]string{
			"X-Xuanchu-Event":         event.EventType,
			"X-Xuanchu-Event-Id":      event.EventID,
			"X-Xuanchu-Event-Version": "1",
		}
		headersBytes, err := json.Marshal(headers)
		if err != nil {
			return err
		}

		now := s.clock.Unix()
		var deliveries []storage.HookDelivery
		for _, hook := range hooks {
			deliveryID := uuid.NewString()
			deliveryCtx := NotificationDeliveryContext{ID: deliveryID, Attempt: 1, WorkspaceID: event.WorkspaceID, SinkID: hook.SinkID}
			objectCtx := NotificationObjectContext{Kind: event.ObjectKind, ID: event.ObjectID}
			envelope := map[string]any{
				"delivery_id":        deliveryID,
				"attempt":            1,
				"workspace_id":       event.WorkspaceID,
				"sink_id":            hook.SinkID,
				"hook_id":            hook.ID,
				"rule_id":            hook.ID,
				"created_at":         now,
				"delivery":           map[string]any{"id": deliveryCtx.ID, "attempt": deliveryCtx.Attempt, "workspace_id": deliveryCtx.WorkspaceID, "sink_id": deliveryCtx.SinkID},
				"object":             map[string]any{"kind": objectCtx.Kind, "id": objectCtx.ID},
				"event_id":           event.EventID,
				"event_type":         event.EventType,
				"event_version":      event.EventVersion,
				"occurred_at":        event.OccurredAt,
				"actor_type":         event.ActorType,
				"actor_user_id":      event.ActorUserID,
				"actor_token_id":     event.ActorTokenID,
				"actor_token_name":   event.ActorTokenName,
				"actor_token_prefix": event.ActorTokenPrefix,
				"workspace_slug":     event.WorkspaceSlug,
				"project_id":         event.ProjectID,
				"project_slug":       event.ProjectSlug,
				"object_kind":        event.ObjectKind,
				"object_id":          event.ObjectID,
				"data":               event.Data,
			}
			payloadBytes, err := json.Marshal(envelope)
			if err != nil {
				return err
			}
			req, err := s.resolveHookDeliveryRequest(hook, event, deliveryCtx, objectCtx, string(payloadBytes))
			if err != nil {
				return err
			}
			deliveries = append(deliveries, storage.HookDelivery{
				ID:                          deliveryID,
				HookID:                      hook.ID,
				EventID:                     event.EventID,
				EventType:                   event.EventType,
				WorkspaceID:                 event.WorkspaceID,
				ProjectID:                   event.ProjectID,
				ActorUserID:                 event.ActorUserID,
				ActorType:                   event.ActorType,
				ActorTokenID:                stringPtrOrNil(event.ActorTokenID),
				ActorTokenName:              stringPtrOrNil(event.ActorTokenName),
				ActorTokenPrefix:            stringPtrOrNil(event.ActorTokenPrefix),
				SinkID:                      hook.SinkID,
				ResolvedURL:                 req.ResolvedURL,
				ResolvedEndpointSource:      req.ResolvedEndpointSource,
				ResolvedEndpointFingerprint: req.ResolvedEndpointFingerprint,
				RenderedMethod:              req.RenderedMethod,
				RenderedHeadersJSON:         req.RenderedHeadersJSON,
				RenderedBody:                req.RenderedBody,
				RenderedContentType:         req.RenderedContentType,
				PayloadJSON:                 string(payloadBytes),
				HeadersJSON:                 string(headersBytes),
				Status:                      storage.DeliveryStatusQueued,
				CreatedAt:                   now,
				ModifiedAt:                  now,
			})
		}
		if err := s.hookDeliveryRepo.Enqueue(deliveries); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) resolveHookDeliveryRequest(hook storage.HookDefinition, event HookEvent, delivery NotificationDeliveryContext, object NotificationObjectContext, envelopeJSON string) (NotificationResolvedRequest, error) {
	sink, err := s.notificationSinkRepo.GetByID(hook.SinkID)
	if err == storage.ErrNotFound {
		return NotificationResolvedRequest{}, RuntimeError{Code: "notification_sink_not_found", Message: "notification sink not found"}
	}
	if err != nil {
		return NotificationResolvedRequest{}, err
	}
	if sink.WorkspaceID != event.WorkspaceID || sink.WorkspaceID != s.workspaceID {
		return NotificationResolvedRequest{}, RuntimeError{Code: "notification_sink_not_found", Message: "notification sink not found"}
	}
	sinkView := notificationSinkViewFromRow(sink, actorInfoFromColumns(notificationSinkActorColumns(sink), sink.CreatedBy, nil))

	workspace, err := s.workspaceRepo.GetByID(event.WorkspaceID)
	if err != nil {
		return NotificationResolvedRequest{}, err
	}
	var projectCtx *NotificationProjectContext
	if event.ProjectID != nil && *event.ProjectID != "" {
		project, err := s.projectRepo.GetByID(*event.ProjectID)
		if err != nil {
			return NotificationResolvedRequest{}, err
		}
		projectCtx = &NotificationProjectContext{ID: project.ID, Slug: project.Slug, Name: project.Name}
	}
	userInfos, err := s.resolveUserInfos(hookEventActorUserIDs(event))
	if err != nil {
		return NotificationResolvedRequest{}, err
	}
	actor := hookEventActorAsUserInfo(event, userInfos)
	configValues, secretValues, err := schedulerNotificationConfigValues(s.configRepo, s.configDefRepo, event.WorkspaceID, event.ProjectID, sink)
	if err != nil {
		return NotificationResolvedRequest{}, err
	}
	req, err := ResolveNotificationRequest(NotificationRequestResolveInput{
		Sink:      sinkView,
		Workspace: NotificationWorkspaceContext{ID: workspace.ID, Slug: workspace.Slug, Name: workspace.Name},
		Project:   projectCtx,
		Rule:      NotificationRuleContext{ID: hook.ID, Name: hook.Name, TriggerType: "hook"},
		Recipient: actor,
		Actor:     actor,
		Event: NotificationEventContext{
			ID:         event.EventID,
			Type:       event.EventType,
			Version:    event.EventVersion,
			OccurredAt: event.OccurredAt,
			ObjectKind: event.ObjectKind,
			ObjectID:   event.ObjectID,
			JSON:       envelopeJSON,
		},
		EventType:    event.EventType,
		Delivery:     delivery,
		Object:       object,
		ConfigValues: configValues,
		SecretValues: secretValues,
	})
	if err != nil {
		return NotificationResolvedRequest{}, err
	}
	if strings.TrimSpace(req.RenderedBody) == "" {
		req.RenderedBody = envelopeJSON
	}
	if strings.TrimSpace(req.RenderedMethod) == "" {
		req.RenderedMethod = "POST"
	}
	if req.RenderedHeadersJSON == "" {
		req.RenderedHeadersJSON = "{}"
	}
	if req.RenderedContentType == "" {
		req.RenderedContentType = "application/json"
	}
	if req.PayloadJSON == "" {
		req.PayloadJSON = envelopeJSON
	}
	if req.ResolvedURL == "" {
		return NotificationResolvedRequest{}, RuntimeError{Code: "endpoint_unresolved", Message: fmt.Sprintf("hook sink %q endpoint is empty", sink.Name)}
	}
	return req, nil
}

func hookEventActorUserIDs(event HookEvent) []string {
	if event.ActorType == actorTypeUser && event.ActorUserID != "" {
		return []string{event.ActorUserID}
	}
	return nil
}

func hookEventActorAsUserInfo(event HookEvent, users map[string]task.UserInfo) task.UserInfo {
	if event.ActorType == actorTypeUser {
		if ui, ok := users[event.ActorUserID]; ok {
			return ui
		}
		return task.UserInfo{ID: event.ActorUserID, Name: event.ActorUserID}
	}
	name := event.ActorTokenName
	if name == "" {
		name = event.ActorTokenID
	}
	return task.UserInfo{ID: event.ActorTokenID, Name: name}
}

// matchingHooks 返回应该接收该事件的所有 hook。
// workspace 范围的 hook 接收所有事件，project 范围的 hook 仅接收匹配 project 的事件。
func (s *Service) matchingHooks(event HookEvent) ([]storage.HookDefinition, error) {
	// 查询 workspace 下所有启用的、匹配事件类型的 hook
	allHooks, err := s.hookRepo.ListMatching(s.workspaceID, nil, event.EventType)
	if err != nil {
		return nil, err
	}
	var matched []storage.HookDefinition
	for _, hook := range allHooks {
		if hook.ProjectID == nil {
			// workspace 范围的 hook 接收所有事件
			matched = append(matched, hook)
			continue
		}
		// project 范围的 hook 仅在 event 的 project 匹配时接收
		if event.ProjectID != nil && *hook.ProjectID == *event.ProjectID {
			matched = append(matched, hook)
		}
	}
	return matched, nil
}
