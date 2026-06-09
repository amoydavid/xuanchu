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
	EventID       string
	EventType     string
	EventVersion  int
	OccurredAt    int64
	ActorUserID   string
	WorkspaceID   string
	WorkspaceSlug string
	ProjectID     *string
	ProjectSlug   *string
	ObjectKind    string
	ObjectID      string
	Data          map[string]any
}

// buildTaskHookEvent 构建任务相关的 hook 事件。
func buildTaskHookEvent(eventType string, tsk task.Task, runtime RuntimeContext, now int64) HookEvent {
	var projectSlug *string
	if tsk.Project != nil {
		projectSlug = tsk.Project
	}
	return HookEvent{
		EventID:       uuid.NewString(),
		EventType:     eventType,
		EventVersion:  1,
		OccurredAt:    now,
		ActorUserID:   runtime.ActorUserID,
		WorkspaceID:   runtime.WorkspaceID,
		WorkspaceSlug: runtime.WorkspaceSlug,
		ProjectID:     tsk.ProjectID,
		ProjectSlug:   projectSlug,
		ObjectKind:    "task",
		ObjectID:      tsk.UUID,
		Data:          buildTaskPayload(tsk),
	}
}

// buildProjectArchivedHookEvent 构建项目归档的 hook 事件。
func buildProjectArchivedHookEvent(pv ProjectView, runtime RuntimeContext, now int64) HookEvent {
	return HookEvent{
		EventID:       uuid.NewString(),
		EventType:     "project.archived",
		EventVersion:  1,
		OccurredAt:    now,
		ActorUserID:   runtime.ActorUserID,
		WorkspaceID:   pv.WorkspaceID,
		WorkspaceSlug: runtime.WorkspaceSlug,
		ProjectID:     &pv.ID,
		ProjectSlug:   &pv.Slug,
		ObjectKind:    "project",
		ObjectID:      pv.ID,
		Data:          buildProjectArchivedPayload(pv),
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
	return HookEvent{
		EventID:       uuid.NewString(),
		EventType:     "project.annotated",
		EventVersion:  1,
		OccurredAt:    now,
		ActorUserID:   runtime.ActorUserID,
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
				"created_by":      task.UserInfoToJSON(annotation.CreatedBy),
				"created_at":      annotation.CreatedAt,
			},
		},
	}
}

func buildProjectDenotatedHookEvent(pv ProjectView, annotationID string, runtime RuntimeContext, now int64) HookEvent {
	return HookEvent{
		EventID:       uuid.NewString(),
		EventType:     "project.denotated",
		EventVersion:  1,
		OccurredAt:    now,
		ActorUserID:   runtime.ActorUserID,
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
}

func buildTaskUnblockedHookEvent(tsk task.Task, completedTask task.Task, runtime RuntimeContext, now int64) HookEvent {
	event := buildTaskHookEvent("task.unblocked", tsk, runtime, now)
	event.Data["dependency"] = map[string]any{
		"completed_task": task.ToJSON(completedTask),
	}
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
		hooks, err := s.matchingHooks(event)
		if err != nil {
			return err
		}
		if len(hooks) == 0 {
			continue
		}
		envelope := map[string]any{
			"event_id":       event.EventID,
			"event_type":     event.EventType,
			"event_version":  event.EventVersion,
			"occurred_at":    event.OccurredAt,
			"actor_user_id":  event.ActorUserID,
			"workspace_id":   event.WorkspaceID,
			"workspace_slug": event.WorkspaceSlug,
			"project_id":     event.ProjectID,
			"project_slug":   event.ProjectSlug,
			"object_kind":    event.ObjectKind,
			"object_id":      event.ObjectID,
			"data":           event.Data,
		}
		payloadBytes, err := json.Marshal(envelope)
		if err != nil {
			return err
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
			req, err := s.resolveHookDeliveryRequest(hook, event, string(payloadBytes))
			if err != nil {
				return err
			}
			deliveries = append(deliveries, storage.HookDelivery{
				ID:                          uuid.NewString(),
				HookID:                      hook.ID,
				EventID:                     event.EventID,
				EventType:                   event.EventType,
				WorkspaceID:                 event.WorkspaceID,
				ProjectID:                   event.ProjectID,
				ActorUserID:                 event.ActorUserID,
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

func (s *Service) resolveHookDeliveryRequest(hook storage.HookDefinition, event HookEvent, envelopeJSON string) (NotificationResolvedRequest, error) {
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
	sinkView := notificationSinkViewFromRow(sink, task.UserInfo{ID: sink.CreatedBy, Name: sink.CreatedBy})

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
	userInfos, err := s.resolveUserInfos([]string{event.ActorUserID})
	if err != nil {
		return NotificationResolvedRequest{}, err
	}
	actor := userInfos[event.ActorUserID]
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
