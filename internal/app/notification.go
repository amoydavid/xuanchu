package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.dajee.net/dajee/xuanchu/internal/query"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

const (
	NotificationSinkTypeWebhook      = "webhook"
	NotificationSinkTypeHTTPTemplate = "http_template"

	NotificationEndpointStaticURL   = "static_url"
	NotificationEndpointTemplate    = "template"
	NotificationEndpointConfigValue = "config_value"
)

type HTTPHeaderTemplateInput struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type HTTPTemplateSecretRefInput struct {
	Alias     string `json:"alias"`
	ConfigKey string `json:"config_key"`
}

type NotificationSinkAddInput struct {
	Name            string
	Type            string
	EndpointMode    string
	URL             string
	URLTemplate     string
	ConfigKey       string
	AllowedHosts    []string
	HTTPMethod      string
	HeaderTemplates []HTTPHeaderTemplateInput
	BodyTemplate    string
	BodyContentType string
	SecretRefs      []HTTPTemplateSecretRefInput
	Secret          string
	TimeoutSeconds  int
	MaxAttempts     int
}

type NotificationSinkModifyInput struct {
	Name            *string
	Type            *string
	EndpointMode    *string
	URL             *string
	URLTemplate     *string
	ConfigKey       *string
	AllowedHosts    *[]string
	HTTPMethod      *string
	HeaderTemplates *[]HTTPHeaderTemplateInput
	BodyTemplate    *string
	BodyContentType *string
	SecretRefs      *[]HTTPTemplateSecretRefInput
	Secret          *string
	TimeoutSeconds  *int
	MaxAttempts     *int
}

type NotificationSinkView struct {
	ID              string                       `json:"id"`
	WorkspaceID     string                       `json:"workspace_id"`
	Name            string                       `json:"name"`
	Type            string                       `json:"type"`
	EndpointMode    string                       `json:"endpoint_mode"`
	URL             string                       `json:"url"`
	URLTemplate     string                       `json:"url_template"`
	ConfigKey       string                       `json:"config_key"`
	AllowedHosts    []string                     `json:"allowed_hosts"`
	HTTPMethod      string                       `json:"http_method"`
	HeaderTemplates []HTTPHeaderTemplateInput    `json:"header_templates"`
	BodyTemplate    string                       `json:"body_template"`
	BodyContentType string                       `json:"body_content_type"`
	SecretRefs      []HTTPTemplateSecretRefInput `json:"secret_refs"`
	Enabled         bool                         `json:"enabled"`
	TimeoutSeconds  int                          `json:"timeout_seconds"`
	MaxAttempts     int                          `json:"max_attempts"`
	CreatedBy       task.UserInfo                `json:"created_by"`
	CreatedAt       int64                        `json:"created_at"`
	ModifiedAt      int64                        `json:"modified_at"`
}

type ReminderRuleAddInput struct {
	Name          string
	ProjectRef    string
	TriggerType   string
	OffsetSeconds int64
	AfterSeconds  int64
	RepeatPolicy  string
	ScheduleType  string
	ScheduleValue string
	FilterSource  string
	AudienceType  string
	Recipients    []string
	SinkRef       string
}

type ReminderRuleModifyInput struct {
	Name          *string
	ProjectRef    *string
	TriggerType   *string
	OffsetSeconds *int64
	AfterSeconds  *int64
	RepeatPolicy  *string
	ScheduleType  *string
	ScheduleValue *string
	FilterSource  *string
	AudienceType  *string
	Recipients    *[]string
	SinkRef       *string
}

type ReminderRuleView struct {
	ID               string          `json:"id"`
	WorkspaceID      string          `json:"workspace_id"`
	ProjectID        *string         `json:"project_id"`
	Name             string          `json:"name"`
	Enabled          bool            `json:"enabled"`
	TriggerType      string          `json:"trigger_type"`
	OffsetSeconds    int64           `json:"offset_seconds"`
	AfterSeconds     int64           `json:"after_seconds"`
	RepeatPolicy     string          `json:"repeat_policy"`
	ScheduleType     string          `json:"schedule_type"`
	ScheduleValue    string          `json:"schedule_value"`
	FilterSource     string          `json:"filter_source"`
	AudienceType     string          `json:"audience_type"`
	RecipientUserIDs []string        `json:"-"`
	RecipientUsers   []task.UserInfo `json:"recipient_users"`
	SinkID           string          `json:"sink_id"`
	CreatedBy        task.UserInfo   `json:"created_by"`
	CreatedAt        int64           `json:"created_at"`
	ModifiedAt       int64           `json:"modified_at"`
}

type NotificationDeliveryView struct {
	ID                          string              `json:"id"`
	WorkspaceID                 string              `json:"workspace_id"`
	ProjectID                   *string             `json:"project_id"`
	RuleID                      string              `json:"rule_id"`
	SinkID                      string              `json:"sink_id"`
	TaskUUID                    string              `json:"task_uuid"`
	RecipientUserID             string              `json:"-"`
	Recipient                   task.UserInfo       `json:"recipient"`
	EventID                     string              `json:"event_id"`
	EventType                   string              `json:"event_type"`
	ResolvedURL                 string              `json:"resolved_url"`
	ResolvedEndpointSource      string              `json:"resolved_endpoint_source"`
	ResolvedEndpointFingerprint string              `json:"resolved_endpoint_fingerprint"`
	RenderedMethod              string              `json:"rendered_method"`
	RenderedHeaders             map[string][]string `json:"rendered_headers"`
	RenderedBody                string              `json:"rendered_body"`
	RenderedContentType         string              `json:"rendered_content_type"`
	Payload                     map[string]any      `json:"payload"`
	Status                      string              `json:"status"`
	AttemptCount                int                 `json:"attempt_count"`
	NextAttemptAt               *int64              `json:"next_attempt_at"`
	ClaimExpiresAt              *int64              `json:"claim_expires_at"`
	LastAttemptAt               *int64              `json:"last_attempt_at"`
	LastStatusCode              *int                `json:"last_status_code"`
	LastError                   string              `json:"last_error"`
	CreatedAt                   int64               `json:"created_at"`
	ModifiedAt                  int64               `json:"modified_at"`
}

func (s *Service) AddNotificationSink(input NotificationSinkAddInput) (NotificationSinkView, error) {
	if err := s.Require(PermissionNotificationWrite); err != nil {
		return NotificationSinkView{}, err
	}
	normalized, err := normalizeNotificationSinkInput(s, input)
	if err != nil {
		return NotificationSinkView{}, err
	}
	now := s.clock.Unix()
	enabled := true
	row := storage.NotificationSink{
		ID:                  uuid.NewString(),
		WorkspaceID:         s.workspaceID,
		Name:                normalized.Name,
		Type:                normalized.Type,
		EndpointMode:        normalized.EndpointMode,
		URL:                 normalized.URL,
		URLTemplate:         normalized.URLTemplate,
		ConfigKey:           normalized.ConfigKey,
		AllowedHostsJSON:    mustJSON(normalized.AllowedHosts),
		HTTPMethod:          normalized.HTTPMethod,
		HeaderTemplatesJSON: mustJSON(normalized.HeaderTemplates),
		BodyTemplate:        normalized.BodyTemplate,
		BodyContentType:     normalized.BodyContentType,
		SecretRefsJSON:      mustJSON(secretRefsJSONMap(normalized.SecretRefs)),
		Secret:              normalized.Secret,
		Enabled:             &enabled,
		TimeoutSeconds:      normalized.TimeoutSeconds,
		MaxAttempts:         normalized.MaxAttempts,
		CreatedBy:           s.runtime.ActorUserID,
		CreatedAt:           now,
		ModifiedAt:          now,
	}
	var view NotificationSinkView
	err = s.withAudit("notification.sink.create", func(tx *Service) (AuditEntry, error) {
		if err := tx.notificationSinkRepo.Create(row); err != nil {
			return AuditEntry{}, err
		}
		created, err := tx.notificationSinkRepo.GetByID(row.ID)
		if err != nil {
			return AuditEntry{}, err
		}
		view, err = tx.notificationSinkViewFromRow(created)
		if err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{
			TargetType: "notification_sink",
			TargetID:   created.ID,
			Payload: map[string]any{
				"name":               created.Name,
				"type":               created.Type,
				"endpoint_mode":      created.EndpointMode,
				"secret_fingerprint": secretFingerprint(input.Secret),
				"secret_refs":        normalized.SecretRefs,
			},
		}, nil
	})
	return view, err
}

func (s *Service) ListNotificationSinks(includeDisabled bool) ([]NotificationSinkView, error) {
	if err := s.Require(PermissionNotificationRead); err != nil {
		return nil, err
	}
	rows, err := s.notificationSinkRepo.List(s.workspaceID, includeDisabled)
	if err != nil {
		return nil, err
	}
	userInfos, err := s.resolveUserInfos(notificationSinkCreatedByIDs(rows))
	if err != nil {
		return nil, err
	}
	out := make([]NotificationSinkView, 0, len(rows))
	for _, row := range rows {
		out = append(out, notificationSinkViewFromRow(row, userInfos[row.CreatedBy]))
	}
	return out, nil
}

func (s *Service) NotificationSinkInfo(sinkID string) (NotificationSinkView, error) {
	if err := s.Require(PermissionNotificationRead); err != nil {
		return NotificationSinkView{}, err
	}
	row, err := s.notificationSinkRepo.GetByID(sinkID)
	if err == storage.ErrNotFound {
		return NotificationSinkView{}, RuntimeError{Code: "notification_sink_not_found", Message: "notification sink not found"}
	}
	if err != nil {
		return NotificationSinkView{}, err
	}
	if row.WorkspaceID != s.workspaceID {
		return NotificationSinkView{}, RuntimeError{Code: "notification_sink_not_found", Message: "notification sink not found"}
	}
	return s.notificationSinkViewFromRow(row)
}

func (s *Service) ModifyNotificationSink(sinkID string, input NotificationSinkModifyInput) (NotificationSinkView, error) {
	if err := s.Require(PermissionNotificationWrite); err != nil {
		return NotificationSinkView{}, err
	}
	row, err := s.notificationSinkRepo.GetByID(sinkID)
	if err == storage.ErrNotFound {
		return NotificationSinkView{}, RuntimeError{Code: "notification_sink_not_found", Message: "notification sink not found"}
	}
	if err != nil {
		return NotificationSinkView{}, err
	}
	if row.WorkspaceID != s.workspaceID {
		return NotificationSinkView{}, RuntimeError{Code: "notification_sink_not_found", Message: "notification sink not found"}
	}
	candidate := notificationSinkAddInputFromRow(row)
	applyNotificationSinkModifyInput(&candidate, input)
	normalized, err := normalizeNotificationSinkInput(s, candidate)
	if err != nil {
		return NotificationSinkView{}, err
	}
	now := s.clock.Unix()
	row.Name = normalized.Name
	row.Type = normalized.Type
	row.EndpointMode = normalized.EndpointMode
	row.URL = normalized.URL
	row.URLTemplate = normalized.URLTemplate
	row.ConfigKey = normalized.ConfigKey
	row.AllowedHostsJSON = mustJSON(normalized.AllowedHosts)
	row.HTTPMethod = normalized.HTTPMethod
	row.HeaderTemplatesJSON = mustJSON(normalized.HeaderTemplates)
	row.BodyTemplate = normalized.BodyTemplate
	row.BodyContentType = normalized.BodyContentType
	row.SecretRefsJSON = mustJSON(secretRefsJSONMap(normalized.SecretRefs))
	row.Secret = normalized.Secret
	row.TimeoutSeconds = normalized.TimeoutSeconds
	row.MaxAttempts = normalized.MaxAttempts
	row.ModifiedAt = now
	var view NotificationSinkView
	err = s.withAudit("notification.sink.modify", func(tx *Service) (AuditEntry, error) {
		if err := tx.notificationSinkRepo.Update(row); err != nil {
			return AuditEntry{}, err
		}
		updated, err := tx.notificationSinkRepo.GetByID(sinkID)
		if err != nil {
			return AuditEntry{}, err
		}
		view, err = tx.notificationSinkViewFromRow(updated)
		if err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{
			TargetType: "notification_sink",
			TargetID:   sinkID,
			Payload: map[string]any{
				"name":               updated.Name,
				"type":               updated.Type,
				"endpoint_mode":      updated.EndpointMode,
				"secret_fingerprint": secretFingerprint(normalized.Secret),
				"secret_refs":        normalized.SecretRefs,
			},
		}, nil
	})
	return view, err
}

func (s *Service) EnableNotificationSink(sinkID string) (NotificationSinkView, error) {
	if err := s.Require(PermissionNotificationWrite); err != nil {
		return NotificationSinkView{}, err
	}
	return s.toggleNotificationSink(sinkID, true, "notification.sink.enable")
}

func (s *Service) DisableNotificationSink(sinkID string) (NotificationSinkView, error) {
	if err := s.Require(PermissionNotificationWrite); err != nil {
		return NotificationSinkView{}, err
	}
	return s.toggleNotificationSink(sinkID, false, "notification.sink.disable")
}

func (s *Service) toggleNotificationSink(sinkID string, enabled bool, action string) (NotificationSinkView, error) {
	row, err := s.notificationSinkRepo.GetByID(sinkID)
	if err == storage.ErrNotFound {
		return NotificationSinkView{}, RuntimeError{Code: "notification_sink_not_found", Message: "notification sink not found"}
	}
	if err != nil {
		return NotificationSinkView{}, err
	}
	if row.WorkspaceID != s.workspaceID {
		return NotificationSinkView{}, RuntimeError{Code: "notification_sink_not_found", Message: "notification sink not found"}
	}
	row.Enabled = &enabled
	row.ModifiedAt = s.clock.Unix()
	var view NotificationSinkView
	err = s.withAudit(action, func(tx *Service) (AuditEntry, error) {
		if err := tx.notificationSinkRepo.Update(row); err != nil {
			return AuditEntry{}, err
		}
		updated, err := tx.notificationSinkRepo.GetByID(sinkID)
		if err != nil {
			return AuditEntry{}, err
		}
		view, err = tx.notificationSinkViewFromRow(updated)
		if err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{TargetType: "notification_sink", TargetID: sinkID, Payload: map[string]any{"enabled": enabled}}, nil
	})
	return view, err
}

func (s *Service) DeleteNotificationSink(sinkID string) error {
	if err := s.Require(PermissionNotificationWrite); err != nil {
		return err
	}
	row, err := s.notificationSinkRepo.GetByID(sinkID)
	if err == storage.ErrNotFound {
		return RuntimeError{Code: "notification_sink_not_found", Message: "notification sink not found"}
	}
	if err != nil {
		return err
	}
	if row.WorkspaceID != s.workspaceID {
		return RuntimeError{Code: "notification_sink_not_found", Message: "notification sink not found"}
	}
	return s.withAudit("notification.sink.delete", func(tx *Service) (AuditEntry, error) {
		if err := tx.notificationSinkRepo.Delete(sinkID); err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{TargetType: "notification_sink", TargetID: sinkID, Payload: map[string]any{"name": row.Name}}, nil
	})
}

func (s *Service) AddReminderRule(input ReminderRuleAddInput) (ReminderRuleView, error) {
	if err := s.Require(PermissionReminderWrite); err != nil {
		return ReminderRuleView{}, err
	}
	var projectID *string
	if input.ProjectRef != "" {
		project, err := s.ResolveProject(input.ProjectRef)
		if err != nil {
			return ReminderRuleView{}, err
		}
		projectID = &project.ID
	}
	name, sink, recipientIDs, scheduleType, scheduleValue, filterSource, err := s.normalizeReminderRuleFields(input)
	if err != nil {
		return ReminderRuleView{}, err
	}
	trigger := strings.TrimSpace(input.TriggerType)
	repeat := strings.TrimSpace(input.RepeatPolicy)
	if repeat == "" {
		repeat = "once"
	}
	audience := strings.TrimSpace(input.AudienceType)
	now := s.clock.Unix()
	enabled := true
	row := storage.ReminderRule{
		ID:                   uuid.NewString(),
		WorkspaceID:          s.workspaceID,
		ProjectID:            projectID,
		Name:                 name,
		Enabled:              &enabled,
		TriggerType:          trigger,
		OffsetSeconds:        input.OffsetSeconds,
		AfterSeconds:         input.AfterSeconds,
		RepeatPolicy:         repeat,
		ScheduleType:         scheduleType,
		ScheduleValue:        scheduleValue,
		FilterSource:         filterSource,
		AudienceType:         audience,
		RecipientUserIDsJSON: mustJSON(recipientIDs),
		SinkID:               sink.ID,
		CreatedBy:            s.runtime.ActorUserID,
		CreatedAt:            now,
		ModifiedAt:           now,
	}
	var view ReminderRuleView
	err = s.withAudit("reminder.rule.create", func(tx *Service) (AuditEntry, error) {
		if err := tx.reminderRuleRepo.Create(row); err != nil {
			return AuditEntry{}, err
		}
		created, err := tx.reminderRuleRepo.GetByID(row.ID)
		if err != nil {
			return AuditEntry{}, err
		}
		view, err = tx.reminderRuleViewFromRow(created)
		if err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{
			TargetType: "reminder_rule",
			TargetID:   created.ID,
			Payload: map[string]any{
				"name":         created.Name,
				"trigger_type": created.TriggerType,
				"audience":     created.AudienceType,
				"sink_id":      created.SinkID,
			},
		}, nil
	})
	return view, err
}

func (s *Service) ListReminderRules(projectRef string, includeDisabled bool) ([]ReminderRuleView, error) {
	if err := s.Require(PermissionReminderRead); err != nil {
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
	rows, err := s.reminderRuleRepo.List(s.workspaceID, projectID, includeDisabled)
	if err != nil {
		return nil, err
	}
	userInfos, err := s.resolveUserInfos(reminderRuleUserIDs(rows))
	if err != nil {
		return nil, err
	}
	out := make([]ReminderRuleView, 0, len(rows))
	for _, row := range rows {
		out = append(out, reminderRuleViewFromRow(row, userInfos))
	}
	return out, nil
}

func (s *Service) ReminderRuleInfo(ruleID string) (ReminderRuleView, error) {
	if err := s.Require(PermissionReminderRead); err != nil {
		return ReminderRuleView{}, err
	}
	row, err := s.reminderRuleRepo.GetByID(ruleID)
	if err == storage.ErrNotFound {
		return ReminderRuleView{}, RuntimeError{Code: "reminder_rule_not_found", Message: "reminder rule not found"}
	}
	if err != nil {
		return ReminderRuleView{}, err
	}
	if row.WorkspaceID != s.workspaceID {
		return ReminderRuleView{}, RuntimeError{Code: "reminder_rule_not_found", Message: "reminder rule not found"}
	}
	if !s.allowsProjectID(row.ProjectID) {
		return ReminderRuleView{}, RuntimeError{Code: "reminder_rule_not_found", Message: "reminder rule not found"}
	}
	return s.reminderRuleViewFromRow(row)
}

func (s *Service) ModifyReminderRule(ruleID string, input ReminderRuleModifyInput) (ReminderRuleView, error) {
	if err := s.Require(PermissionReminderWrite); err != nil {
		return ReminderRuleView{}, err
	}
	row, err := s.reminderRuleRepo.GetByID(ruleID)
	if err == storage.ErrNotFound {
		return ReminderRuleView{}, RuntimeError{Code: "reminder_rule_not_found", Message: "reminder rule not found"}
	}
	if err != nil {
		return ReminderRuleView{}, err
	}
	if row.WorkspaceID != s.workspaceID {
		return ReminderRuleView{}, RuntimeError{Code: "reminder_rule_not_found", Message: "reminder rule not found"}
	}
	if !s.allowsProjectID(row.ProjectID) {
		return ReminderRuleView{}, RuntimeError{Code: "project_scope_denied", Message: "token cannot access project"}
	}
	candidate := reminderRuleAddInputFromRow(row)
	var projectID = row.ProjectID
	if input.ProjectRef != nil {
		candidate.ProjectRef = strings.TrimSpace(*input.ProjectRef)
		if candidate.ProjectRef == "" {
			projectID = nil
		} else {
			project, err := s.ResolveProject(candidate.ProjectRef)
			if err != nil {
				return ReminderRuleView{}, err
			}
			projectID = &project.ID
		}
	}
	applyReminderRuleModifyInput(&candidate, input)
	name, sink, recipientIDs, scheduleType, scheduleValue, filterSource, err := s.normalizeReminderRuleFields(candidate)
	if err != nil {
		return ReminderRuleView{}, err
	}
	if input.ProjectRef == nil {
		projectID = row.ProjectID
	}
	now := s.clock.Unix()
	row.Name = name
	row.ProjectID = projectID
	row.TriggerType = strings.TrimSpace(candidate.TriggerType)
	row.OffsetSeconds = candidate.OffsetSeconds
	row.AfterSeconds = candidate.AfterSeconds
	row.RepeatPolicy = strings.TrimSpace(candidate.RepeatPolicy)
	if row.RepeatPolicy == "" {
		row.RepeatPolicy = "once"
	}
	row.ScheduleType = scheduleType
	row.ScheduleValue = scheduleValue
	row.FilterSource = filterSource
	row.AudienceType = strings.TrimSpace(candidate.AudienceType)
	row.RecipientUserIDsJSON = mustJSON(recipientIDs)
	row.SinkID = sink.ID
	row.ModifiedAt = now
	var view ReminderRuleView
	err = s.withAudit("reminder.rule.modify", func(tx *Service) (AuditEntry, error) {
		if err := tx.reminderRuleRepo.Update(row); err != nil {
			return AuditEntry{}, err
		}
		updated, err := tx.reminderRuleRepo.GetByID(ruleID)
		if err != nil {
			return AuditEntry{}, err
		}
		view, err = tx.reminderRuleViewFromRow(updated)
		if err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{
			TargetType: "reminder_rule",
			TargetID:   ruleID,
			Payload: map[string]any{
				"name":         updated.Name,
				"trigger_type": updated.TriggerType,
				"audience":     updated.AudienceType,
				"sink_id":      updated.SinkID,
			},
		}, nil
	})
	return view, err
}

func (s *Service) EnableReminderRule(ruleID string) (ReminderRuleView, error) {
	if err := s.Require(PermissionReminderWrite); err != nil {
		return ReminderRuleView{}, err
	}
	return s.toggleReminderRule(ruleID, true, "reminder.rule.enable")
}

func (s *Service) DisableReminderRule(ruleID string) (ReminderRuleView, error) {
	if err := s.Require(PermissionReminderWrite); err != nil {
		return ReminderRuleView{}, err
	}
	return s.toggleReminderRule(ruleID, false, "reminder.rule.disable")
}

func (s *Service) toggleReminderRule(ruleID string, enabled bool, action string) (ReminderRuleView, error) {
	row, err := s.reminderRuleRepo.GetByID(ruleID)
	if err == storage.ErrNotFound {
		return ReminderRuleView{}, RuntimeError{Code: "reminder_rule_not_found", Message: "reminder rule not found"}
	}
	if err != nil {
		return ReminderRuleView{}, err
	}
	if row.WorkspaceID != s.workspaceID {
		return ReminderRuleView{}, RuntimeError{Code: "reminder_rule_not_found", Message: "reminder rule not found"}
	}
	if !s.allowsProjectID(row.ProjectID) {
		return ReminderRuleView{}, RuntimeError{Code: "project_scope_denied", Message: "token cannot access project"}
	}
	row.Enabled = &enabled
	row.ModifiedAt = s.clock.Unix()
	var view ReminderRuleView
	err = s.withAudit(action, func(tx *Service) (AuditEntry, error) {
		if err := tx.reminderRuleRepo.Update(row); err != nil {
			return AuditEntry{}, err
		}
		updated, err := tx.reminderRuleRepo.GetByID(ruleID)
		if err != nil {
			return AuditEntry{}, err
		}
		view, err = tx.reminderRuleViewFromRow(updated)
		if err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{TargetType: "reminder_rule", TargetID: ruleID, Payload: map[string]any{"enabled": enabled}}, nil
	})
	return view, err
}

func (s *Service) DeleteReminderRule(ruleID string) error {
	if err := s.Require(PermissionReminderWrite); err != nil {
		return err
	}
	row, err := s.reminderRuleRepo.GetByID(ruleID)
	if err == storage.ErrNotFound {
		return RuntimeError{Code: "reminder_rule_not_found", Message: "reminder rule not found"}
	}
	if err != nil {
		return err
	}
	if row.WorkspaceID != s.workspaceID {
		return RuntimeError{Code: "reminder_rule_not_found", Message: "reminder rule not found"}
	}
	if !s.allowsProjectID(row.ProjectID) {
		return RuntimeError{Code: "project_scope_denied", Message: "token cannot access project"}
	}
	return s.withAudit("reminder.rule.delete", func(tx *Service) (AuditEntry, error) {
		if err := tx.reminderRuleRepo.Delete(ruleID); err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{TargetType: "reminder_rule", TargetID: ruleID, Payload: map[string]any{"name": row.Name}}, nil
	})
}

func (s *Service) ListNotificationDeliveries(sinkID string, status string, limit int, offset int) ([]NotificationDeliveryView, error) {
	if err := s.Require(PermissionNotificationRead); err != nil {
		return nil, err
	}
	rows, err := s.notificationDeliveryRepo.ListWithOptions(storage.NotificationDeliveryListOptions{
		WorkspaceID: s.workspaceID,
		SinkID:      sinkID,
		Status:      status,
		Limit:       limit,
		Offset:      offset,
	})
	if err != nil {
		return nil, err
	}
	out := make([]NotificationDeliveryView, 0, len(rows))
	userInfos, err := s.resolveUserInfos(notificationDeliveryRecipientIDs(rows))
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		view, err := notificationDeliveryViewFromRow(row, userInfos[row.RecipientUserID])
		if err != nil {
			return nil, err
		}
		out = append(out, view)
	}
	return out, nil
}

func (s *Service) NotificationDeliveryInfo(deliveryID string) (NotificationDeliveryView, error) {
	if err := s.Require(PermissionNotificationRead); err != nil {
		return NotificationDeliveryView{}, err
	}
	row, err := s.notificationDeliveryRepo.GetByID(deliveryID)
	if err == storage.ErrNotFound {
		return NotificationDeliveryView{}, RuntimeError{Code: "notification_delivery_not_found", Message: "notification delivery not found"}
	}
	if err != nil {
		return NotificationDeliveryView{}, err
	}
	if row.WorkspaceID != s.workspaceID {
		return NotificationDeliveryView{}, RuntimeError{Code: "notification_delivery_not_found", Message: "notification delivery not found"}
	}
	return s.notificationDeliveryViewFromRow(row)
}

func (s *Service) ReplayNotificationDelivery(deliveryID string) (NotificationDeliveryView, error) {
	if err := s.Require(PermissionNotificationWrite); err != nil {
		return NotificationDeliveryView{}, err
	}
	row, err := s.notificationDeliveryRepo.GetByID(deliveryID)
	if err == storage.ErrNotFound {
		return NotificationDeliveryView{}, RuntimeError{Code: "notification_delivery_not_found", Message: "notification delivery not found"}
	}
	if err != nil {
		return NotificationDeliveryView{}, err
	}
	if row.WorkspaceID != s.workspaceID {
		return NotificationDeliveryView{}, RuntimeError{Code: "notification_delivery_not_found", Message: "notification delivery not found"}
	}
	if row.Status != storage.DeliveryStatusDeadLettered && row.Status != storage.DeliveryStatusDisabledSkipped {
		return NotificationDeliveryView{}, RuntimeError{Code: "notification_delivery_not_replayable", Message: "notification delivery is not in a replayable state"}
	}
	var view NotificationDeliveryView
	err = s.withAudit("notification.delivery.replay", func(tx *Service) (AuditEntry, error) {
		if err := tx.notificationDeliveryRepo.Requeue(deliveryID, tx.clock.Unix()); err != nil {
			return AuditEntry{}, err
		}
		updated, err := tx.notificationDeliveryRepo.GetByID(deliveryID)
		if err != nil {
			return AuditEntry{}, err
		}
		view, err = tx.notificationDeliveryViewFromRow(updated)
		if err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{
			TargetType: "notification_delivery",
			TargetID:   deliveryID,
			Payload:    map[string]any{"sink_id": row.SinkID, "rule_id": row.RuleID, "event_type": row.EventType},
		}, nil
	})
	return view, err
}

func normalizeNotificationSinkInput(s *Service, input NotificationSinkAddInput) (NotificationSinkAddInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return input, RuntimeError{Code: "notification_sink_invalid", Message: "sink name is required"}
	}
	input.Type = strings.TrimSpace(input.Type)
	if input.Type == "" {
		input.Type = NotificationSinkTypeWebhook
	}
	if input.Type != NotificationSinkTypeWebhook && input.Type != NotificationSinkTypeHTTPTemplate {
		return input, RuntimeError{Code: "notification_sink_invalid", Message: "unsupported sink type"}
	}
	input.EndpointMode = strings.TrimSpace(input.EndpointMode)
	if input.EndpointMode == "" {
		input.EndpointMode = NotificationEndpointStaticURL
	}
	switch input.EndpointMode {
	case NotificationEndpointStaticURL:
		if strings.TrimSpace(input.URL) == "" {
			return input, RuntimeError{Code: "notification_sink_invalid", Message: "url is required"}
		}
	case NotificationEndpointTemplate:
		if strings.TrimSpace(input.URLTemplate) == "" {
			return input, RuntimeError{Code: "notification_sink_invalid", Message: "url template is required"}
		}
		if err := validateEndpointTemplateVariables(input.URLTemplate); err != nil {
			return input, err
		}
	case NotificationEndpointConfigValue:
		if strings.TrimSpace(input.ConfigKey) == "" {
			return input, RuntimeError{Code: "notification_sink_invalid", Message: "config key is required"}
		}
	default:
		return input, RuntimeError{Code: "notification_sink_invalid", Message: "unsupported endpoint mode"}
	}
	if input.EndpointMode != NotificationEndpointStaticURL && len(input.AllowedHosts) == 0 {
		return input, RuntimeError{Code: "notification_sink_invalid", Message: "allowed host is required for dynamic endpoint"}
	}
	if input.HTTPMethod == "" {
		input.HTTPMethod = http.MethodPost
	}
	if strings.ToUpper(input.HTTPMethod) != http.MethodPost {
		return input, RuntimeError{Code: "notification_sink_invalid", Message: "only POST is supported"}
	}
	input.HTTPMethod = http.MethodPost
	if input.Type == NotificationSinkTypeHTTPTemplate {
		if input.BodyContentType == "application/json" && strings.TrimSpace(input.BodyTemplate) != "" {
			if err := validateJSONBodyTemplate(input.BodyTemplate); err != nil {
				return input, RuntimeError{Code: "notification_sink_invalid", Message: "invalid json body template"}
			}
		}
		for _, ref := range input.SecretRefs {
			if err := s.validateHTTPTemplateSecretRef(ref); err != nil {
				return input, err
			}
		}
	}
	if input.TimeoutSeconds == 0 {
		input.TimeoutSeconds = 10
	}
	if err := validateTimeout(input.TimeoutSeconds); err != nil {
		return input, err
	}
	if input.MaxAttempts == 0 {
		input.MaxAttempts = 5
	}
	if err := validateMaxAttempts(input.MaxAttempts); err != nil {
		return input, err
	}
	return input, nil
}

func notificationSinkAddInputFromRow(row storage.NotificationSink) NotificationSinkAddInput {
	return NotificationSinkAddInput{
		Name:            row.Name,
		Type:            row.Type,
		EndpointMode:    row.EndpointMode,
		URL:             row.URL,
		URLTemplate:     row.URLTemplate,
		ConfigKey:       row.ConfigKey,
		AllowedHosts:    decodeStringListNoError(row.AllowedHostsJSON),
		HTTPMethod:      row.HTTPMethod,
		HeaderTemplates: decodeHeaderTemplatesNoError(row.HeaderTemplatesJSON),
		BodyTemplate:    row.BodyTemplate,
		BodyContentType: row.BodyContentType,
		SecretRefs:      decodeSecretRefsNoError(row.SecretRefsJSON),
		Secret:          row.Secret,
		TimeoutSeconds:  row.TimeoutSeconds,
		MaxAttempts:     row.MaxAttempts,
	}
}

func applyNotificationSinkModifyInput(input *NotificationSinkAddInput, mod NotificationSinkModifyInput) {
	if mod.Name != nil {
		input.Name = *mod.Name
	}
	if mod.Type != nil {
		input.Type = *mod.Type
	}
	if mod.EndpointMode != nil {
		input.EndpointMode = *mod.EndpointMode
	}
	if mod.URL != nil {
		input.URL = *mod.URL
	}
	if mod.URLTemplate != nil {
		input.URLTemplate = *mod.URLTemplate
	}
	if mod.ConfigKey != nil {
		input.ConfigKey = *mod.ConfigKey
	}
	if mod.AllowedHosts != nil {
		input.AllowedHosts = *mod.AllowedHosts
	}
	if mod.HTTPMethod != nil {
		input.HTTPMethod = *mod.HTTPMethod
	}
	if mod.HeaderTemplates != nil {
		input.HeaderTemplates = *mod.HeaderTemplates
	}
	if mod.BodyTemplate != nil {
		input.BodyTemplate = *mod.BodyTemplate
	}
	if mod.BodyContentType != nil {
		input.BodyContentType = *mod.BodyContentType
	}
	if mod.SecretRefs != nil {
		input.SecretRefs = *mod.SecretRefs
	}
	if mod.Secret != nil {
		input.Secret = *mod.Secret
	}
	if mod.TimeoutSeconds != nil {
		input.TimeoutSeconds = *mod.TimeoutSeconds
	}
	if mod.MaxAttempts != nil {
		input.MaxAttempts = *mod.MaxAttempts
	}
}

func reminderRuleAddInputFromRow(row storage.ReminderRule) ReminderRuleAddInput {
	return ReminderRuleAddInput{
		Name:          row.Name,
		TriggerType:   row.TriggerType,
		OffsetSeconds: row.OffsetSeconds,
		AfterSeconds:  row.AfterSeconds,
		RepeatPolicy:  row.RepeatPolicy,
		ScheduleType:  row.ScheduleType,
		ScheduleValue: row.ScheduleValue,
		FilterSource:  row.FilterSource,
		AudienceType:  row.AudienceType,
		Recipients:    decodeStringListNoError(row.RecipientUserIDsJSON),
		SinkRef:       row.SinkID,
	}
}

func applyReminderRuleModifyInput(input *ReminderRuleAddInput, mod ReminderRuleModifyInput) {
	if mod.Name != nil {
		input.Name = *mod.Name
	}
	if mod.TriggerType != nil {
		input.TriggerType = *mod.TriggerType
	}
	if mod.OffsetSeconds != nil {
		input.OffsetSeconds = *mod.OffsetSeconds
	}
	if mod.AfterSeconds != nil {
		input.AfterSeconds = *mod.AfterSeconds
	}
	if mod.RepeatPolicy != nil {
		input.RepeatPolicy = *mod.RepeatPolicy
	}
	if mod.ScheduleType != nil {
		input.ScheduleType = *mod.ScheduleType
	}
	if mod.ScheduleValue != nil {
		input.ScheduleValue = *mod.ScheduleValue
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
}

func (s *Service) normalizeReminderRuleFields(input ReminderRuleAddInput) (string, storage.NotificationSink, []string, string, string, string, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return "", storage.NotificationSink{}, nil, "", "", "", RuntimeError{Code: "reminder_rule_invalid", Message: "rule name is required"}
	}
	scheduleType, scheduleValue, filterSource, usesScheduleFilter, err := normalizeReminderScheduleFilter(input.ScheduleType, input.ScheduleValue, input.FilterSource)
	if err != nil {
		return "", storage.NotificationSink{}, nil, "", "", "", err
	}
	if !usesScheduleFilter {
		trigger := strings.TrimSpace(input.TriggerType)
		switch trigger {
		case "due_before":
			if input.OffsetSeconds <= 0 {
				return "", storage.NotificationSink{}, nil, "", "", "", RuntimeError{Code: "reminder_rule_invalid", Message: "due_before requires positive offset"}
			}
		case "overdue":
		default:
			return "", storage.NotificationSink{}, nil, "", "", "", RuntimeError{Code: "reminder_rule_invalid", Message: "unsupported trigger"}
		}
	}
	repeat := strings.TrimSpace(input.RepeatPolicy)
	if repeat == "" {
		repeat = "once"
	}
	if repeat != "once" && !strings.HasPrefix(repeat, "every:") {
		return "", storage.NotificationSink{}, nil, "", "", "", RuntimeError{Code: "reminder_rule_invalid", Message: "unsupported repeat policy"}
	}
	audience := strings.TrimSpace(input.AudienceType)
	switch audience {
	case "assignees", "explicit_users", "assignees_and_explicit_users":
	case "project_owner", "project_maintainer", "assignees_and_project_owner":
		return "", storage.NotificationSink{}, nil, "", "", "", RuntimeError{Code: "audience_unsupported", Message: "audience is not supported"}
	default:
		return "", storage.NotificationSink{}, nil, "", "", "", RuntimeError{Code: "reminder_rule_invalid", Message: "unsupported audience"}
	}
	sink, err := s.resolveNotificationSink(input.SinkRef)
	if err != nil {
		return "", storage.NotificationSink{}, nil, "", "", "", err
	}
	recipientIDs, err := s.resolveReminderRecipientIDs(input.Recipients)
	if err != nil {
		return "", storage.NotificationSink{}, nil, "", "", "", err
	}
	return name, sink, recipientIDs, scheduleType, scheduleValue, filterSource, nil
}

func normalizeReminderScheduleFilter(scheduleType, scheduleValue, filterSource string) (string, string, string, bool, error) {
	scheduleType = strings.TrimSpace(scheduleType)
	scheduleValue = strings.TrimSpace(scheduleValue)
	filterSource = strings.TrimSpace(filterSource)
	usesScheduleFilter := scheduleType != "" || scheduleValue != "" || filterSource != ""
	if !usesScheduleFilter {
		return "", "", "", false, nil
	}
	if strings.HasPrefix(scheduleType, "daily@") && scheduleValue == "" {
		scheduleValue = strings.TrimPrefix(scheduleType, "daily@")
		scheduleType = "daily_at"
	}
	if scheduleType != "daily_at" {
		return "", "", "", true, RuntimeError{Code: "reminder_rule_invalid", Message: "unsupported schedule"}
	}
	if _, err := time.Parse("15:04", scheduleValue); err != nil {
		return "", "", "", true, RuntimeError{Code: "reminder_rule_invalid", Message: "invalid daily schedule"}
	}
	if filterSource == "" {
		return "", "", "", true, RuntimeError{Code: "reminder_rule_invalid", Message: "filter is required"}
	}
	expr, err := query.ParseQuery(filterSource)
	if err != nil {
		return "", "", "", true, RuntimeError{Code: "reminder_rule_invalid", Message: "invalid filter"}
	}
	if reminderFilterContainsProjectPredicate(expr) {
		return "", "", "", true, RuntimeError{Code: "reminder_rule_invalid", Message: "filter project predicate is not supported"}
	}
	return scheduleType, scheduleValue, filterSource, true, nil
}

func reminderFilterContainsProjectPredicate(expr query.Expr) bool {
	switch e := expr.(type) {
	case nil:
		return false
	case query.Predicate:
		return e.Attribute == query.AttrProject
	case query.Binary:
		return reminderFilterContainsProjectPredicate(e.Left) || reminderFilterContainsProjectPredicate(e.Right)
	case query.Unary:
		return reminderFilterContainsProjectPredicate(e.Expr)
	default:
		return false
	}
}

func (s *Service) validateHTTPTemplateSecretRef(ref HTTPTemplateSecretRefInput) error {
	alias := strings.TrimSpace(ref.Alias)
	key := strings.TrimSpace(ref.ConfigKey)
	if alias == "" || key == "" {
		return RuntimeError{Code: "notification_sink_invalid", Message: "secret ref alias and config key are required"}
	}
	def, err := s.scopedConfigDefinition(key)
	if err != nil {
		return err
	}
	if !def.Secret {
		return RuntimeError{Code: "notification_sink_invalid", Message: "secret ref config key must be secret"}
	}
	return nil
}

func (s *Service) resolveNotificationSink(ref string) (storage.NotificationSink, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return storage.NotificationSink{}, RuntimeError{Code: "notification_sink_invalid", Message: "sink is required"}
	}
	if row, err := s.notificationSinkRepo.GetByID(ref); err == nil {
		if row.WorkspaceID != s.workspaceID {
			return storage.NotificationSink{}, RuntimeError{Code: "notification_sink_not_found", Message: "notification sink not found"}
		}
		return row, nil
	}
	row, err := s.notificationSinkRepo.GetByName(s.workspaceID, ref)
	if err == storage.ErrNotFound {
		return storage.NotificationSink{}, RuntimeError{Code: "notification_sink_not_found", Message: "notification sink not found"}
	}
	return row, err
}

func (s *Service) resolveReminderRecipientIDs(refs []string) ([]string, error) {
	out := make([]string, 0, len(refs))
	seen := map[string]bool{}
	for _, ref := range refs {
		user, err := s.resolveUser(ref)
		if err != nil {
			return nil, err
		}
		if _, err := s.memberRepo.Get(user.ID, s.workspaceID); err != nil {
			return nil, err
		}
		if !seen[user.ID] {
			seen[user.ID] = true
			out = append(out, user.ID)
		}
	}
	return out, nil
}

func (s *Service) notificationSinkViewFromRow(row storage.NotificationSink) (NotificationSinkView, error) {
	userInfos, err := s.resolveUserInfos([]string{row.CreatedBy})
	if err != nil {
		return NotificationSinkView{}, err
	}
	return notificationSinkViewFromRow(row, userInfos[row.CreatedBy]), nil
}

func notificationSinkViewFromRow(row storage.NotificationSink, createdBy task.UserInfo) NotificationSinkView {
	enabled := row.Enabled != nil && *row.Enabled
	return NotificationSinkView{
		ID:              row.ID,
		WorkspaceID:     row.WorkspaceID,
		Name:            row.Name,
		Type:            row.Type,
		EndpointMode:    row.EndpointMode,
		URL:             row.URL,
		URLTemplate:     row.URLTemplate,
		ConfigKey:       row.ConfigKey,
		AllowedHosts:    decodeStringListNoError(row.AllowedHostsJSON),
		HTTPMethod:      row.HTTPMethod,
		HeaderTemplates: decodeHeaderTemplatesNoError(row.HeaderTemplatesJSON),
		BodyTemplate:    row.BodyTemplate,
		BodyContentType: row.BodyContentType,
		SecretRefs:      decodeSecretRefsNoError(row.SecretRefsJSON),
		Enabled:         enabled,
		TimeoutSeconds:  row.TimeoutSeconds,
		MaxAttempts:     row.MaxAttempts,
		CreatedBy:       createdBy,
		CreatedAt:       row.CreatedAt,
		ModifiedAt:      row.ModifiedAt,
	}
}

func (s *Service) reminderRuleViewFromRow(row storage.ReminderRule) (ReminderRuleView, error) {
	userIDs := []string{row.CreatedBy}
	userIDs = append(userIDs, decodeStringListNoError(row.RecipientUserIDsJSON)...)
	userInfos, err := s.resolveUserInfos(userIDs)
	if err != nil {
		return ReminderRuleView{}, err
	}
	return reminderRuleViewFromRow(row, userInfos), nil
}

func reminderRuleViewFromRow(row storage.ReminderRule, userInfos map[string]task.UserInfo) ReminderRuleView {
	enabled := row.Enabled != nil && *row.Enabled
	recipientIDs := decodeStringListNoError(row.RecipientUserIDsJSON)
	recipients := make([]task.UserInfo, 0, len(recipientIDs))
	for _, id := range recipientIDs {
		recipients = append(recipients, userInfos[id])
	}
	return ReminderRuleView{
		ID:               row.ID,
		WorkspaceID:      row.WorkspaceID,
		ProjectID:        row.ProjectID,
		Name:             row.Name,
		Enabled:          enabled,
		TriggerType:      row.TriggerType,
		OffsetSeconds:    row.OffsetSeconds,
		AfterSeconds:     row.AfterSeconds,
		RepeatPolicy:     row.RepeatPolicy,
		ScheduleType:     row.ScheduleType,
		ScheduleValue:    row.ScheduleValue,
		FilterSource:     row.FilterSource,
		AudienceType:     row.AudienceType,
		RecipientUserIDs: recipientIDs,
		RecipientUsers:   recipients,
		SinkID:           row.SinkID,
		CreatedBy:        userInfos[row.CreatedBy],
		CreatedAt:        row.CreatedAt,
		ModifiedAt:       row.ModifiedAt,
	}
}

func notificationSinkCreatedByIDs(rows []storage.NotificationSink) []string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.CreatedBy)
	}
	return ids
}

func reminderRuleCreatedByIDs(rows []storage.ReminderRule) []string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.CreatedBy)
	}
	return ids
}

func reminderRuleUserIDs(rows []storage.ReminderRule) []string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.CreatedBy)
		ids = append(ids, decodeStringListNoError(row.RecipientUserIDsJSON)...)
	}
	return ids
}

func (s *Service) notificationDeliveryViewFromRow(row storage.NotificationDelivery) (NotificationDeliveryView, error) {
	userInfos, err := s.resolveUserInfos([]string{row.RecipientUserID})
	if err != nil {
		return NotificationDeliveryView{}, err
	}
	return notificationDeliveryViewFromRow(row, userInfos[row.RecipientUserID])
}

func notificationDeliveryViewFromRow(row storage.NotificationDelivery, recipient task.UserInfo) (NotificationDeliveryView, error) {
	var payload map[string]any
	if row.PayloadJSON != "" {
		if err := json.Unmarshal([]byte(row.PayloadJSON), &payload); err != nil {
			return NotificationDeliveryView{}, fmt.Errorf("parse notification delivery payload: %w", err)
		}
	}
	headers := map[string][]string{}
	if row.RenderedHeadersJSON != "" {
		if err := json.Unmarshal([]byte(row.RenderedHeadersJSON), &headers); err != nil {
			return NotificationDeliveryView{}, fmt.Errorf("parse notification delivery headers: %w", err)
		}
	}
	return NotificationDeliveryView{
		ID:                          row.ID,
		WorkspaceID:                 row.WorkspaceID,
		ProjectID:                   row.ProjectID,
		RuleID:                      row.RuleID,
		SinkID:                      row.SinkID,
		TaskUUID:                    row.TaskUUID,
		RecipientUserID:             row.RecipientUserID,
		Recipient:                   recipient,
		EventID:                     row.EventID,
		EventType:                   row.EventType,
		ResolvedURL:                 row.ResolvedURL,
		ResolvedEndpointSource:      row.ResolvedEndpointSource,
		ResolvedEndpointFingerprint: row.ResolvedEndpointFingerprint,
		RenderedMethod:              row.RenderedMethod,
		RenderedHeaders:             redactHeaderValues(headers),
		RenderedBody:                redactSensitiveText(row.RenderedBody),
		RenderedContentType:         row.RenderedContentType,
		Payload:                     payload,
		Status:                      row.Status,
		AttemptCount:                row.AttemptCount,
		NextAttemptAt:               row.NextAttemptAt,
		ClaimExpiresAt:              row.ClaimExpiresAt,
		LastAttemptAt:               row.LastAttemptAt,
		LastStatusCode:              row.LastStatusCode,
		LastError:                   redactSensitiveText(row.LastError),
		CreatedAt:                   row.CreatedAt,
		ModifiedAt:                  row.ModifiedAt,
	}, nil
}

func notificationDeliveryRecipientIDs(rows []storage.NotificationDelivery) []string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.RecipientUserID)
	}
	return ids
}

func redactHeaderValues(headers map[string][]string) map[string][]string {
	out := make(map[string][]string, len(headers))
	for name, values := range headers {
		redacted := make([]string, len(values))
		copy(redacted, values)
		if isSensitiveHeader(name) {
			for i := range redacted {
				redacted[i] = "[REDACTED]"
			}
		}
		out[name] = redacted
	}
	return out
}

func isSensitiveHeader(name string) bool {
	lower := strings.ToLower(name)
	return lower == "authorization" || lower == "proxy-authorization" || strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "key")
}

func redactSensitiveText(text string) string {
	if text == "" {
		return ""
	}
	return strings.ReplaceAll(text, "secret-token", "[REDACTED]")
}

func secretRefsJSONMap(refs []HTTPTemplateSecretRefInput) map[string]map[string]string {
	out := map[string]map[string]string{}
	for _, ref := range refs {
		out[ref.Alias] = map[string]string{"config_key": ref.ConfigKey}
	}
	return out
}

func decodeStringListNoError(raw string) []string {
	var out []string
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func decodeHeaderTemplatesNoError(raw string) []HTTPHeaderTemplateInput {
	var out []HTTPHeaderTemplateInput
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func decodeSecretRefsNoError(raw string) []HTTPTemplateSecretRefInput {
	var refs map[string]map[string]string
	if err := json.Unmarshal([]byte(raw), &refs); err != nil {
		return nil
	}
	out := make([]HTTPTemplateSecretRefInput, 0, len(refs))
	for alias, ref := range refs {
		out = append(out, HTTPTemplateSecretRefInput{Alias: alias, ConfigKey: ref["config_key"]})
	}
	return out
}

func mustJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("marshal notification json: %v", err))
	}
	return string(data)
}
