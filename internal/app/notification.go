package app

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.dajee.net/dajee/xuanchu/internal/query"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

type NotificationSinkType string

const (
	NotificationSinkTypeWebhook      NotificationSinkType = "webhook"
	NotificationSinkTypeHTTPTemplate NotificationSinkType = "http_template"
)

type NotificationEndpointMode string

const (
	NotificationEndpointStaticURL   NotificationEndpointMode = "static_url"
	NotificationEndpointTemplate    NotificationEndpointMode = "template"
	NotificationEndpointConfigValue NotificationEndpointMode = "config_value"
)

type HTTPHeaderTemplateInput struct {
	Name  string
	Value string
}

type HTTPTemplateSecretRefInput struct {
	Alias     string
	ConfigKey string
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
	Enabled         *bool
}

type ReminderAudienceType string

const (
	ReminderAudienceAssignees         ReminderAudienceType = "assignees"
	ReminderAudienceExplicitUsers     ReminderAudienceType = "explicit_users"
	ReminderAudienceAssigneesAndUsers ReminderAudienceType = "assignees_and_explicit_users"
)

const defaultReminderTaskFilter = "due.notnull and (status:pending or status:waiting)"

type reminderTaskFilterSpec struct {
	Source string `json:"source"`
}

type ReminderRuleAddInput struct {
	Name          string
	ProjectRef    string
	TriggerType   string
	OffsetSeconds int64
	AfterSeconds  int64
	RepeatPolicy  string
	TaskFilter    string
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
	TaskFilter    *string
	AudienceType  *string
	Recipients    *[]string
	SinkRef       *string
	Enabled       *bool
}

type HTTPHeaderTemplateView struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type HTTPTemplateSecretRefView struct {
	Alias     string `json:"alias"`
	ConfigKey string `json:"config_key"`
}

type NotificationSinkView struct {
	ID              string                      `json:"id"`
	WorkspaceID     string                      `json:"workspace_id"`
	Name            string                      `json:"name"`
	Type            string                      `json:"type"`
	EndpointMode    string                      `json:"endpoint_mode"`
	URL             string                      `json:"url,omitempty"`
	URLTemplate     string                      `json:"url_template,omitempty"`
	ConfigKey       string                      `json:"config_key,omitempty"`
	AllowedHosts    []string                    `json:"allowed_hosts,omitempty"`
	HTTPMethod      string                      `json:"http_method,omitempty"`
	HeaderTemplates []HTTPHeaderTemplateView    `json:"header_templates,omitempty"`
	BodyTemplate    string                      `json:"body_template,omitempty"`
	BodyContentType string                      `json:"body_content_type,omitempty"`
	SecretRefs      []HTTPTemplateSecretRefView `json:"secret_refs,omitempty"`
	Enabled         bool                        `json:"enabled"`
	TimeoutSeconds  int                         `json:"timeout_seconds"`
	MaxAttempts     int                         `json:"max_attempts"`
	CreatedBy       task.UserInfo               `json:"created_by"`
	CreatedAt       int64                       `json:"created_at"`
	ModifiedAt      int64                       `json:"modified_at"`
}

type ReminderRuleView struct {
	ID            string          `json:"id"`
	WorkspaceID   string          `json:"workspace_id"`
	ProjectID     *string         `json:"project_id,omitempty"`
	Name          string          `json:"name"`
	Enabled       bool            `json:"enabled"`
	TriggerType   string          `json:"trigger_type"`
	OffsetSeconds int64           `json:"offset_seconds"`
	AfterSeconds  int64           `json:"after_seconds"`
	RepeatPolicy  string          `json:"repeat_policy"`
	TaskFilter    string          `json:"task_filter"`
	AudienceType  string          `json:"audience_type"`
	Recipients    []task.UserInfo `json:"recipients,omitempty"`
	SinkID        string          `json:"sink_id"`
	SinkName      string          `json:"sink_name"`
	CreatedBy     task.UserInfo   `json:"created_by"`
	CreatedAt     int64           `json:"created_at"`
	ModifiedAt    int64           `json:"modified_at"`
}

type NotificationDeliveryView struct {
	ID                          string            `json:"id"`
	WorkspaceID                 string            `json:"workspace_id"`
	ProjectID                   *string           `json:"project_id,omitempty"`
	RuleID                      string            `json:"rule_id"`
	RuleName                    string            `json:"rule_name"`
	SinkID                      string            `json:"sink_id"`
	SinkName                    string            `json:"sink_name"`
	TaskUUID                    string            `json:"task_uuid"`
	Recipient                   task.UserInfo     `json:"recipient"`
	EventID                     string            `json:"event_id"`
	EventType                   string            `json:"event_type"`
	DedupeKey                   string            `json:"dedupe_key"`
	ResolvedURL                 string            `json:"resolved_url"`
	ResolvedEndpointSource      string            `json:"resolved_endpoint_source,omitempty"`
	ResolvedEndpointFingerprint string            `json:"resolved_endpoint_fingerprint,omitempty"`
	RenderedMethod              string            `json:"rendered_method"`
	RenderedHeaders             map[string]string `json:"rendered_headers,omitempty"`
	RenderedBody                string            `json:"rendered_body,omitempty"`
	RenderedContentType         string            `json:"rendered_content_type,omitempty"`
	Payload                     map[string]any    `json:"payload"`
	Status                      string            `json:"status"`
	AttemptCount                int               `json:"attempt_count"`
	NextAttemptAt               *int64            `json:"next_attempt_at,omitempty"`
	ClaimExpiresAt              *int64            `json:"claim_expires_at,omitempty"`
	LastAttemptAt               *int64            `json:"last_attempt_at,omitempty"`
	LastStatusCode              *int              `json:"last_status_code,omitempty"`
	LastError                   string            `json:"last_error,omitempty"`
	CreatedAt                   int64             `json:"created_at"`
	ModifiedAt                  int64             `json:"modified_at"`
}

func (s *Service) AddNotificationSink(input NotificationSinkAddInput) (NotificationSinkView, error) {
	if err := s.Require(PermissionNotificationWrite); err != nil {
		return NotificationSinkView{}, err
	}
	row, audit, err := s.prepareNotificationSinkRow(input, nil)
	if err != nil {
		return NotificationSinkView{}, err
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
		view = tx.notificationSinkViewFromRow(created)
		return AuditEntry{
			TargetType: "notification_sink",
			TargetID:   created.ID,
			Payload:    audit,
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
	out := make([]NotificationSinkView, 0, len(rows))
	for _, row := range rows {
		out = append(out, s.notificationSinkViewFromRow(row))
	}
	return out, nil
}

func (s *Service) NotificationSinkInfo(ref string) (NotificationSinkView, error) {
	if err := s.Require(PermissionNotificationRead); err != nil {
		return NotificationSinkView{}, err
	}
	row, err := s.resolveNotificationSink(ref)
	if err != nil {
		return NotificationSinkView{}, err
	}
	return s.notificationSinkViewFromRow(row), nil
}

func (s *Service) ModifyNotificationSink(ref string, input NotificationSinkModifyInput) (NotificationSinkView, error) {
	if err := s.Require(PermissionNotificationWrite); err != nil {
		return NotificationSinkView{}, err
	}
	row, err := s.resolveNotificationSink(ref)
	if err != nil {
		return NotificationSinkView{}, err
	}
	updated, audit, err := s.applyNotificationSinkModify(row, input)
	if err != nil {
		return NotificationSinkView{}, err
	}
	var view NotificationSinkView
	err = s.withAudit("notification.sink.modify", func(tx *Service) (AuditEntry, error) {
		if err := tx.notificationSinkRepo.Update(updated); err != nil {
			return AuditEntry{}, err
		}
		loaded, err := tx.notificationSinkRepo.GetByID(updated.ID)
		if err != nil {
			return AuditEntry{}, err
		}
		view = tx.notificationSinkViewFromRow(loaded)
		return AuditEntry{TargetType: "notification_sink", TargetID: loaded.ID, Payload: audit}, nil
	})
	return view, err
}

func (s *Service) EnableNotificationSink(ref string) (NotificationSinkView, error) {
	return s.setNotificationSinkEnabled(ref, true)
}

func (s *Service) DisableNotificationSink(ref string) (NotificationSinkView, error) {
	return s.setNotificationSinkEnabled(ref, false)
}

func (s *Service) DeleteNotificationSink(ref string) error {
	if err := s.Require(PermissionNotificationWrite); err != nil {
		return err
	}
	row, err := s.resolveNotificationSink(ref)
	if err != nil {
		return err
	}
	return s.withAudit("notification.sink.delete", func(tx *Service) (AuditEntry, error) {
		if err := tx.notificationSinkRepo.Delete(row.ID); err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{TargetType: "notification_sink", TargetID: row.ID, Payload: map[string]any{"name": row.Name}}, nil
	})
}

func (s *Service) AddReminderRule(input ReminderRuleAddInput) (ReminderRuleView, error) {
	if err := s.Require(PermissionReminderWrite); err != nil {
		return ReminderRuleView{}, err
	}
	row, audit, err := s.prepareReminderRuleRow(input, nil)
	if err != nil {
		return ReminderRuleView{}, err
	}
	var view ReminderRuleView
	err = s.withAudit("reminder.rule.create", func(tx *Service) (AuditEntry, error) {
		if err := tx.reminderRuleRepo.Create(row); err != nil {
			return AuditEntry{}, err
		}
		loaded, err := tx.reminderRuleRepo.GetByID(row.ID)
		if err != nil {
			return AuditEntry{}, err
		}
		view, err = tx.reminderRuleViewFromRow(loaded)
		if err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{TargetType: "reminder_rule", TargetID: loaded.ID, ProjectID: loaded.ProjectID, Payload: audit}, nil
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
	out := make([]ReminderRuleView, 0, len(rows))
	for _, row := range rows {
		view, err := s.reminderRuleViewFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, view)
	}
	return out, nil
}

func (s *Service) ReminderRuleInfo(ref string) (ReminderRuleView, error) {
	if err := s.Require(PermissionReminderRead); err != nil {
		return ReminderRuleView{}, err
	}
	row, err := s.resolveReminderRule(ref)
	if err != nil {
		return ReminderRuleView{}, err
	}
	return s.reminderRuleViewFromRow(row)
}

func (s *Service) ModifyReminderRule(ref string, input ReminderRuleModifyInput) (ReminderRuleView, error) {
	if err := s.Require(PermissionReminderWrite); err != nil {
		return ReminderRuleView{}, err
	}
	row, err := s.resolveReminderRule(ref)
	if err != nil {
		return ReminderRuleView{}, err
	}
	updated, audit, err := s.applyReminderRuleModify(row, input)
	if err != nil {
		return ReminderRuleView{}, err
	}
	var view ReminderRuleView
	err = s.withAudit("reminder.rule.modify", func(tx *Service) (AuditEntry, error) {
		if err := tx.reminderRuleRepo.Update(updated); err != nil {
			return AuditEntry{}, err
		}
		loaded, err := tx.reminderRuleRepo.GetByID(updated.ID)
		if err != nil {
			return AuditEntry{}, err
		}
		view, err = tx.reminderRuleViewFromRow(loaded)
		if err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{TargetType: "reminder_rule", TargetID: loaded.ID, ProjectID: loaded.ProjectID, Payload: audit}, nil
	})
	return view, err
}

func (s *Service) EnableReminderRule(ref string) (ReminderRuleView, error) {
	return s.setReminderRuleEnabled(ref, true)
}

func (s *Service) DisableReminderRule(ref string) (ReminderRuleView, error) {
	return s.setReminderRuleEnabled(ref, false)
}

func (s *Service) DeleteReminderRule(ref string) error {
	if err := s.Require(PermissionReminderWrite); err != nil {
		return err
	}
	row, err := s.resolveReminderRule(ref)
	if err != nil {
		return err
	}
	return s.withAudit("reminder.rule.delete", func(tx *Service) (AuditEntry, error) {
		if err := tx.reminderRuleRepo.Delete(row.ID); err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{TargetType: "reminder_rule", TargetID: row.ID, ProjectID: row.ProjectID, Payload: map[string]any{"name": row.Name}}, nil
	})
}

func (s *Service) ListNotificationDeliveries(status string, limit int, offset int) ([]NotificationDeliveryView, error) {
	if err := s.Require(PermissionNotificationRead); err != nil {
		return nil, err
	}
	rows, err := s.notificationRepo.List(s.workspaceID, status, limit, offset)
	if err != nil {
		return nil, err
	}
	return s.notificationDeliveryViewsFromRows(rows)
}

func (s *Service) NotificationDeliveryInfo(id string) (NotificationDeliveryView, error) {
	if err := s.Require(PermissionNotificationRead); err != nil {
		return NotificationDeliveryView{}, err
	}
	row, err := s.notificationRepo.GetByID(id)
	if err != nil {
		if err == storage.ErrNotFound {
			return NotificationDeliveryView{}, RuntimeError{Code: "notification_delivery_not_found", Message: "notification delivery not found"}
		}
		return NotificationDeliveryView{}, err
	}
	if row.WorkspaceID != s.workspaceID {
		return NotificationDeliveryView{}, RuntimeError{Code: "notification_delivery_not_found", Message: "notification delivery not found"}
	}
	views, err := s.notificationDeliveryViewsFromRows([]storage.NotificationDelivery{row})
	if err != nil {
		return NotificationDeliveryView{}, err
	}
	return views[0], nil
}

func (s *Service) ReplayNotificationDelivery(id string) (NotificationDeliveryView, error) {
	if err := s.Require(PermissionNotificationWrite); err != nil {
		return NotificationDeliveryView{}, err
	}
	row, err := s.notificationRepo.GetByID(id)
	if err != nil {
		if err == storage.ErrNotFound {
			return NotificationDeliveryView{}, RuntimeError{Code: "notification_delivery_not_found", Message: "notification delivery not found"}
		}
		return NotificationDeliveryView{}, err
	}
	if row.WorkspaceID != s.workspaceID {
		return NotificationDeliveryView{}, RuntimeError{Code: "notification_delivery_not_found", Message: "notification delivery not found"}
	}
	now := s.clock.Unix()
	var view NotificationDeliveryView
	err = s.withAudit("notification.delivery.replay", func(tx *Service) (AuditEntry, error) {
		if err := tx.notificationRepo.Requeue(id, now); err != nil {
			return AuditEntry{}, err
		}
		updated, err := tx.notificationRepo.GetByID(id)
		if err != nil {
			return AuditEntry{}, err
		}
		views, err := tx.notificationDeliveryViewsFromRows([]storage.NotificationDelivery{updated})
		if err != nil {
			return AuditEntry{}, err
		}
		view = views[0]
		return AuditEntry{TargetType: "notification_delivery", TargetID: id, ProjectID: updated.ProjectID, Payload: map[string]any{"replayed": true}}, nil
	})
	return view, err
}

func (s *Service) notificationSinkViewFromRow(row storage.NotificationSink) NotificationSinkView {
	enabled := row.Enabled != nil && *row.Enabled
	createdBy := task.UserInfo{ID: row.CreatedBy, Name: row.CreatedBy}
	if info, err := s.resolveUserInfos([]string{row.CreatedBy}); err == nil {
		if v, ok := info[row.CreatedBy]; ok {
			createdBy = v
		}
	}
	view := NotificationSinkView{
		ID:              row.ID,
		WorkspaceID:     row.WorkspaceID,
		Name:            row.Name,
		Type:            row.Type,
		EndpointMode:    row.EndpointMode,
		URL:             row.URL,
		URLTemplate:     row.URLTemplate,
		ConfigKey:       row.ConfigKey,
		AllowedHosts:    decodeJSONStringSlice(row.AllowedHostsJSON),
		HTTPMethod:      row.HTTPMethod,
		HeaderTemplates: decodeHeaderTemplateRows(row.HeaderTemplatesJSON),
		BodyTemplate:    row.BodyTemplate,
		BodyContentType: row.BodyContentType,
		SecretRefs:      decodeSecretRefRows(row.SecretRefsJSON),
		Enabled:         enabled,
		TimeoutSeconds:  row.TimeoutSeconds,
		MaxAttempts:     row.MaxAttempts,
		CreatedBy:       createdBy,
		CreatedAt:       row.CreatedAt,
		ModifiedAt:      row.ModifiedAt,
	}
	return view
}

func (s *Service) reminderRuleViewFromRow(row storage.ReminderRule) (ReminderRuleView, error) {
	enabled := row.Enabled != nil && *row.Enabled
	sink, err := s.notificationSinkRepo.GetByID(row.SinkID)
	if err != nil {
		return ReminderRuleView{}, err
	}
	recipients, err := decodeStringListJSON(row.RecipientUserIDsJSON)
	if err != nil {
		return ReminderRuleView{}, err
	}
	userInfos, err := s.resolveUserInfos(recipients)
	if err != nil {
		return ReminderRuleView{}, err
	}
	outRecipients := make([]task.UserInfo, 0, len(recipients))
	for _, id := range recipients {
		if ui, ok := userInfos[id]; ok {
			outRecipients = append(outRecipients, ui)
		}
	}
	createdBy := task.UserInfo{ID: row.CreatedBy, Name: row.CreatedBy}
	if info, err := s.resolveUserInfos([]string{row.CreatedBy}); err == nil {
		if v, ok := info[row.CreatedBy]; ok {
			createdBy = v
		}
	}
	return ReminderRuleView{
		ID:            row.ID,
		WorkspaceID:   row.WorkspaceID,
		ProjectID:     row.ProjectID,
		Name:          row.Name,
		Enabled:       enabled,
		TriggerType:   row.TriggerType,
		OffsetSeconds: row.OffsetSeconds,
		AfterSeconds:  row.AfterSeconds,
		RepeatPolicy:  row.RepeatPolicy,
		TaskFilter:    decodeReminderTaskFilterSource(row.TaskFilterJSON),
		AudienceType:  row.AudienceType,
		Recipients:    outRecipients,
		SinkID:        row.SinkID,
		SinkName:      sink.Name,
		CreatedBy:     createdBy,
		CreatedAt:     row.CreatedAt,
		ModifiedAt:    row.ModifiedAt,
	}, nil
}

func (s *Service) notificationDeliveryViewsFromRows(rows []storage.NotificationDelivery) ([]NotificationDeliveryView, error) {
	if len(rows) == 0 {
		return []NotificationDeliveryView{}, nil
	}
	sinkIDs := make([]string, 0, len(rows))
	userIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		sinkIDs = append(sinkIDs, row.SinkID)
		userIDs = append(userIDs, row.RecipientUserID)
	}
	sinks := map[string]storage.NotificationSink{}
	for _, sinkID := range slices.Compact(sinkIDs) {
		sink, err := s.notificationSinkRepo.GetByID(sinkID)
		if err == nil {
			sinks[sinkID] = sink
		}
	}
	userInfos, err := s.resolveUserInfos(userIDs)
	if err != nil {
		return nil, err
	}
	out := make([]NotificationDeliveryView, 0, len(rows))
	for _, row := range rows {
		sink := sinks[row.SinkID]
		view := NotificationDeliveryView{
			ID:                          row.ID,
			WorkspaceID:                 row.WorkspaceID,
			ProjectID:                   row.ProjectID,
			RuleID:                      row.RuleID,
			RuleName:                    "",
			SinkID:                      row.SinkID,
			SinkName:                    sink.Name,
			TaskUUID:                    row.TaskUUID,
			Recipient:                   userInfos[row.RecipientUserID],
			EventID:                     row.EventID,
			EventType:                   row.EventType,
			DedupeKey:                   row.DedupeKey,
			ResolvedURL:                 row.ResolvedURL,
			ResolvedEndpointSource:      row.ResolvedEndpointSource,
			ResolvedEndpointFingerprint: row.ResolvedEndpointFingerprint,
			RenderedMethod:              row.RenderedMethod,
			RenderedHeaders:             decodeHeadersMap(row.RenderedHeadersJSON),
			RenderedBody:                row.RenderedBody,
			RenderedContentType:         row.RenderedContentType,
			Payload:                     decodeAnyMap(row.PayloadJSON),
			Status:                      row.Status,
			AttemptCount:                row.AttemptCount,
			NextAttemptAt:               row.NextAttemptAt,
			ClaimExpiresAt:              row.ClaimExpiresAt,
			LastAttemptAt:               row.LastAttemptAt,
			LastStatusCode:              row.LastStatusCode,
			LastError:                   row.LastError,
			CreatedAt:                   row.CreatedAt,
			ModifiedAt:                  row.ModifiedAt,
		}
		rule, err := s.reminderRuleRepo.GetByID(row.RuleID)
		if err == nil {
			view.RuleName = rule.Name
		}
		if sink.Type == string(NotificationSinkTypeHTTPTemplate) {
			var project *storage.Project
			if row.ProjectID != nil {
				if p, err := s.projectRepo.GetByID(*row.ProjectID); err == nil {
					project = &p
				}
			}
			secrets := s.notificationSecretValuesForSink(sink, project)
			view.RenderedHeaders = redactNotificationStringMap(view.RenderedHeaders, secrets)
			view.RenderedBody = redactNotificationString(view.RenderedBody, secrets)
		}
		out = append(out, view)
	}
	return out, nil
}

func (s *Service) resolveNotificationSink(ref string) (storage.NotificationSink, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return storage.NotificationSink{}, RuntimeError{Code: "notification_sink_not_found", Message: "notification sink not found"}
	}
	if row, err := s.notificationSinkRepo.GetByID(ref); err == nil {
		if row.WorkspaceID != s.workspaceID {
			return storage.NotificationSink{}, RuntimeError{Code: "notification_sink_not_found", Message: "notification sink not found"}
		}
		return row, nil
	}
	row, err := s.notificationSinkRepo.GetByName(s.workspaceID, ref)
	if err != nil {
		return storage.NotificationSink{}, RuntimeError{Code: "notification_sink_not_found", Message: "notification sink not found"}
	}
	return row, nil
}

func (s *Service) resolveReminderRule(ref string) (storage.ReminderRule, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return storage.ReminderRule{}, RuntimeError{Code: "reminder_rule_not_found", Message: "reminder rule not found"}
	}
	if row, err := s.reminderRuleRepo.GetByID(ref); err == nil {
		if row.WorkspaceID != s.workspaceID {
			return storage.ReminderRule{}, RuntimeError{Code: "reminder_rule_not_found", Message: "reminder rule not found"}
		}
		return row, nil
	}
	row, err := s.reminderRuleRepo.GetByName(s.workspaceID, ref)
	if err != nil {
		return storage.ReminderRule{}, RuntimeError{Code: "reminder_rule_not_found", Message: "reminder rule not found"}
	}
	return row, nil
}

func (s *Service) setNotificationSinkEnabled(ref string, enabled bool) (NotificationSinkView, error) {
	if err := s.Require(PermissionNotificationWrite); err != nil {
		return NotificationSinkView{}, err
	}
	row, err := s.resolveNotificationSink(ref)
	if err != nil {
		return NotificationSinkView{}, err
	}
	now := s.clock.Unix()
	row.Enabled = &enabled
	row.ModifiedAt = now
	var view NotificationSinkView
	err = s.withAudit("notification.sink.modify", func(tx *Service) (AuditEntry, error) {
		if err := tx.notificationSinkRepo.Update(row); err != nil {
			return AuditEntry{}, err
		}
		loaded, err := tx.notificationSinkRepo.GetByID(row.ID)
		if err != nil {
			return AuditEntry{}, err
		}
		view = tx.notificationSinkViewFromRow(loaded)
		return AuditEntry{TargetType: "notification_sink", TargetID: loaded.ID, Payload: map[string]any{"enabled": enabled}}, nil
	})
	return view, err
}

func (s *Service) setReminderRuleEnabled(ref string, enabled bool) (ReminderRuleView, error) {
	if err := s.Require(PermissionReminderWrite); err != nil {
		return ReminderRuleView{}, err
	}
	row, err := s.resolveReminderRule(ref)
	if err != nil {
		return ReminderRuleView{}, err
	}
	now := s.clock.Unix()
	row.Enabled = &enabled
	row.ModifiedAt = now
	var view ReminderRuleView
	err = s.withAudit("reminder.rule.modify", func(tx *Service) (AuditEntry, error) {
		if err := tx.reminderRuleRepo.Update(row); err != nil {
			return AuditEntry{}, err
		}
		loaded, err := tx.reminderRuleRepo.GetByID(row.ID)
		if err != nil {
			return AuditEntry{}, err
		}
		view, err = tx.reminderRuleViewFromRow(loaded)
		if err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{TargetType: "reminder_rule", TargetID: loaded.ID, ProjectID: loaded.ProjectID, Payload: map[string]any{"enabled": enabled}}, nil
	})
	return view, err
}

func (s *Service) prepareNotificationSinkRow(input NotificationSinkAddInput, existing *storage.NotificationSink) (storage.NotificationSink, map[string]any, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return storage.NotificationSink{}, nil, RuntimeError{Code: "notification_sink_invalid", Message: "notification sink name is required"}
	}
	typ := strings.TrimSpace(input.Type)
	if typ == "" {
		typ = string(NotificationSinkTypeWebhook)
	}
	if typ != string(NotificationSinkTypeWebhook) && typ != string(NotificationSinkTypeHTTPTemplate) {
		return storage.NotificationSink{}, nil, RuntimeError{Code: "notification_sink_invalid", Message: fmt.Sprintf("unsupported notification sink type %q", input.Type)}
	}
	mode := strings.TrimSpace(input.EndpointMode)
	if mode == "" {
		mode = string(NotificationEndpointStaticURL)
	}
	if mode != string(NotificationEndpointStaticURL) && mode != string(NotificationEndpointTemplate) && mode != string(NotificationEndpointConfigValue) {
		return storage.NotificationSink{}, nil, RuntimeError{Code: "endpoint_mode_invalid", Message: fmt.Sprintf("unsupported endpoint mode %q", input.EndpointMode)}
	}
	allowedHosts := uniqueTrimmed(input.AllowedHosts)
	if mode != string(NotificationEndpointStaticURL) && len(allowedHosts) == 0 {
		return storage.NotificationSink{}, nil, RuntimeError{Code: "endpoint_host_denied", Message: "allowed hosts are required for dynamic endpoints"}
	}
	if mode == string(NotificationEndpointStaticURL) && len(allowedHosts) == 0 {
		allowedHosts = nil
	}
	if typ == string(NotificationSinkTypeWebhook) {
		if mode == string(NotificationEndpointStaticURL) && strings.TrimSpace(input.URL) == "" {
			return storage.NotificationSink{}, nil, RuntimeError{Code: "notification_sink_invalid", Message: "webhook sink requires url"}
		}
	}
	if typ == string(NotificationSinkTypeHTTPTemplate) {
		if strings.TrimSpace(input.HTTPMethod) == "" {
			input.HTTPMethod = "POST"
		}
		if strings.ToUpper(strings.TrimSpace(input.HTTPMethod)) != "POST" {
			return storage.NotificationSink{}, nil, RuntimeError{Code: "notification_sink_invalid", Message: "http template sink only supports POST"}
		}
		if strings.TrimSpace(input.BodyContentType) == "" {
			input.BodyContentType = "application/json"
		}
		if err := validateHTTPHeaderTemplates(input.HeaderTemplates); err != nil {
			return storage.NotificationSink{}, nil, err
		}
		if err := validateHTTPTemplateBody(input.BodyTemplate, input.BodyContentType); err != nil {
			return storage.NotificationSink{}, nil, err
		}
	}
	secretRefs, err := s.validateSecretRefs(input.SecretRefs)
	if err != nil {
		return storage.NotificationSink{}, nil, err
	}
	if mode == string(NotificationEndpointConfigValue) {
		if strings.TrimSpace(input.ConfigKey) == "" {
			return storage.NotificationSink{}, nil, RuntimeError{Code: "notification_sink_invalid", Message: "config key is required for config_value endpoint"}
		}
		if _, err := s.scopedConfigDefinition(input.ConfigKey); err != nil {
			return storage.NotificationSink{}, nil, err
		}
	}
	if mode == string(NotificationEndpointTemplate) && strings.TrimSpace(input.URLTemplate) == "" {
		return storage.NotificationSink{}, nil, RuntimeError{Code: "notification_sink_invalid", Message: "url template is required for template endpoint"}
	}

	enabled := true
	if existing != nil && existing.Enabled != nil {
		enabled = *existing.Enabled
	}
	now := s.clock.Unix()
	row := storage.NotificationSink{
		ID:                  uuid.NewString(),
		WorkspaceID:         s.workspaceID,
		Name:                name,
		Type:                typ,
		EndpointMode:        mode,
		URL:                 strings.TrimSpace(input.URL),
		URLTemplate:         strings.TrimSpace(input.URLTemplate),
		ConfigKey:           strings.TrimSpace(input.ConfigKey),
		AllowedHostsJSON:    mustJSON(allowedHosts),
		HTTPMethod:          strings.ToUpper(strings.TrimSpace(input.HTTPMethod)),
		HeaderTemplatesJSON: mustJSON(input.HeaderTemplates),
		BodyTemplate:        input.BodyTemplate,
		BodyContentType:     strings.TrimSpace(input.BodyContentType),
		SecretRefsJSON:      mustJSON(secretRefs),
		Secret:              input.Secret,
		Enabled:             &enabled,
		TimeoutSeconds:      defaultNotificationTimeout(input.TimeoutSeconds),
		MaxAttempts:         defaultNotificationMaxAttempts(input.MaxAttempts),
		CreatedBy:           s.runtime.ActorUserID,
		CreatedAt:           now,
		ModifiedAt:          now,
	}
	if typ == string(NotificationSinkTypeWebhook) {
		row.HTTPMethod = "POST"
	}
	audit := map[string]any{
		"name":              row.Name,
		"type":              row.Type,
		"endpoint_mode":     row.EndpointMode,
		"allowed_hosts":     allowedHosts,
		"http_method":       row.HTTPMethod,
		"body_content_type": row.BodyContentType,
	}
	if row.URL != "" {
		audit["url"] = row.URL
	}
	if row.URLTemplate != "" {
		audit["url_template"] = row.URLTemplate
	}
	if row.ConfigKey != "" {
		audit["config_key"] = row.ConfigKey
	}
	if len(input.HeaderTemplates) > 0 {
		audit["header_templates"] = input.HeaderTemplates
	}
	if len(secretRefs) > 0 {
		audit["secret_refs"] = secretRefs
	}
	if input.Secret != "" {
		audit["secret_fingerprint"] = secretFingerprint(input.Secret)
	}
	return row, audit, nil
}

func (s *Service) applyNotificationSinkModify(row storage.NotificationSink, input NotificationSinkModifyInput) (storage.NotificationSink, map[string]any, error) {
	next := row
	audit := map[string]any{"name": row.Name}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return storage.NotificationSink{}, nil, RuntimeError{Code: "notification_sink_invalid", Message: "notification sink name is required"}
		}
		next.Name = name
		audit["name"] = name
	}
	if input.Type != nil {
		typ := strings.TrimSpace(*input.Type)
		if typ != string(NotificationSinkTypeWebhook) && typ != string(NotificationSinkTypeHTTPTemplate) {
			return storage.NotificationSink{}, nil, RuntimeError{Code: "notification_sink_invalid", Message: fmt.Sprintf("unsupported notification sink type %q", typ)}
		}
		next.Type = typ
		audit["type"] = typ
	}
	if input.EndpointMode != nil {
		mode := strings.TrimSpace(*input.EndpointMode)
		if mode != string(NotificationEndpointStaticURL) && mode != string(NotificationEndpointTemplate) && mode != string(NotificationEndpointConfigValue) {
			return storage.NotificationSink{}, nil, RuntimeError{Code: "endpoint_mode_invalid", Message: fmt.Sprintf("unsupported endpoint mode %q", mode)}
		}
		next.EndpointMode = mode
		audit["endpoint_mode"] = mode
	}
	if input.URL != nil {
		next.URL = strings.TrimSpace(*input.URL)
		audit["url"] = next.URL
	}
	if input.URLTemplate != nil {
		next.URLTemplate = strings.TrimSpace(*input.URLTemplate)
		audit["url_template"] = next.URLTemplate
	}
	if input.ConfigKey != nil {
		next.ConfigKey = strings.TrimSpace(*input.ConfigKey)
		audit["config_key"] = next.ConfigKey
	}
	if input.AllowedHosts != nil {
		next.AllowedHostsJSON = mustJSON(uniqueTrimmed(*input.AllowedHosts))
		audit["allowed_hosts"] = uniqueTrimmed(*input.AllowedHosts)
	}
	if input.HTTPMethod != nil {
		next.HTTPMethod = strings.ToUpper(strings.TrimSpace(*input.HTTPMethod))
		audit["http_method"] = next.HTTPMethod
	}
	if input.HeaderTemplates != nil {
		if err := validateHTTPHeaderTemplates(*input.HeaderTemplates); err != nil {
			return storage.NotificationSink{}, nil, err
		}
		next.HeaderTemplatesJSON = mustJSON(*input.HeaderTemplates)
		audit["header_templates"] = *input.HeaderTemplates
	}
	if input.BodyTemplate != nil {
		next.BodyTemplate = *input.BodyTemplate
		audit["body_template"] = *input.BodyTemplate
	}
	if input.BodyContentType != nil {
		next.BodyContentType = strings.TrimSpace(*input.BodyContentType)
		audit["body_content_type"] = next.BodyContentType
	}
	if input.SecretRefs != nil {
		refs, err := s.validateSecretRefs(*input.SecretRefs)
		if err != nil {
			return storage.NotificationSink{}, nil, err
		}
		next.SecretRefsJSON = mustJSON(refs)
		audit["secret_refs"] = refs
	}
	if input.Secret != nil {
		next.Secret = *input.Secret
		audit["secret_fingerprint"] = secretFingerprint(*input.Secret)
	}
	if input.TimeoutSeconds != nil {
		next.TimeoutSeconds = defaultNotificationTimeout(*input.TimeoutSeconds)
		audit["timeout_seconds"] = next.TimeoutSeconds
	}
	if input.MaxAttempts != nil {
		next.MaxAttempts = defaultNotificationMaxAttempts(*input.MaxAttempts)
		audit["max_attempts"] = next.MaxAttempts
	}
	if input.Enabled != nil {
		next.Enabled = input.Enabled
		audit["enabled"] = *input.Enabled
	}
	if next.EndpointMode == string(NotificationEndpointStaticURL) && strings.TrimSpace(next.URL) == "" {
		return storage.NotificationSink{}, nil, RuntimeError{Code: "notification_sink_invalid", Message: "static_url endpoint requires url"}
	}
	if next.EndpointMode == string(NotificationEndpointTemplate) && strings.TrimSpace(next.URLTemplate) == "" {
		return storage.NotificationSink{}, nil, RuntimeError{Code: "notification_sink_invalid", Message: "template endpoint requires url template"}
	}
	if next.EndpointMode == string(NotificationEndpointConfigValue) && strings.TrimSpace(next.ConfigKey) == "" {
		return storage.NotificationSink{}, nil, RuntimeError{Code: "notification_sink_invalid", Message: "config_value endpoint requires config key"}
	}
	if next.EndpointMode != string(NotificationEndpointStaticURL) && len(decodeJSONStringSlice(next.AllowedHostsJSON)) == 0 {
		return storage.NotificationSink{}, nil, RuntimeError{Code: "endpoint_host_denied", Message: "allowed hosts are required for dynamic endpoints"}
	}
	if next.Type == string(NotificationSinkTypeHTTPTemplate) {
		if strings.ToUpper(next.HTTPMethod) != "POST" {
			return storage.NotificationSink{}, nil, RuntimeError{Code: "notification_sink_invalid", Message: "http template sink only supports POST"}
		}
		if next.BodyContentType == "" {
			next.BodyContentType = "application/json"
		}
		if err := validateHTTPTemplateBody(next.BodyTemplate, next.BodyContentType); err != nil {
			return storage.NotificationSink{}, nil, err
		}
	}
	if next.Type == string(NotificationSinkTypeWebhook) {
		next.HTTPMethod = "POST"
	}
	next.ModifiedAt = s.clock.Unix()
	return next, audit, nil
}

func (s *Service) prepareReminderRuleRow(input ReminderRuleAddInput, existing *storage.ReminderRule) (storage.ReminderRule, map[string]any, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return storage.ReminderRule{}, nil, RuntimeError{Code: "reminder_rule_invalid", Message: "reminder rule name is required"}
	}
	trigger := strings.TrimSpace(input.TriggerType)
	if trigger != "due_before" && trigger != "overdue" {
		return storage.ReminderRule{}, nil, RuntimeError{Code: "reminder_rule_invalid", Message: fmt.Sprintf("unsupported reminder trigger %q", input.TriggerType)}
	}
	if trigger == "due_before" && input.OffsetSeconds <= 0 {
		return storage.ReminderRule{}, nil, RuntimeError{Code: "reminder_rule_invalid", Message: "due_before rule requires positive offset"}
	}
	if trigger == "overdue" && input.AfterSeconds < 0 {
		return storage.ReminderRule{}, nil, RuntimeError{Code: "reminder_rule_invalid", Message: "overdue rule requires non-negative after seconds"}
	}
	repeatPolicy := strings.TrimSpace(input.RepeatPolicy)
	if repeatPolicy == "" {
		repeatPolicy = "once"
	}
	if repeatPolicy != "once" && !strings.HasPrefix(repeatPolicy, "every:") {
		return storage.ReminderRule{}, nil, RuntimeError{Code: "reminder_rule_invalid", Message: fmt.Sprintf("unsupported repeat policy %q", repeatPolicy)}
	}
	if strings.HasPrefix(repeatPolicy, "every:") {
		if _, err := parseRepeatDuration(strings.TrimPrefix(repeatPolicy, "every:")); err != nil {
			return storage.ReminderRule{}, nil, err
		}
	}
	taskFilter, err := normalizeReminderTaskFilter(input.TaskFilter)
	if err != nil {
		return storage.ReminderRule{}, nil, err
	}
	audience := strings.TrimSpace(input.AudienceType)
	switch audience {
	case string(ReminderAudienceAssignees), string(ReminderAudienceExplicitUsers), string(ReminderAudienceAssigneesAndUsers):
	default:
		return storage.ReminderRule{}, nil, RuntimeError{Code: "audience_unsupported", Message: fmt.Sprintf("unsupported audience %q", input.AudienceType)}
	}
	if (audience == string(ReminderAudienceExplicitUsers) || audience == string(ReminderAudienceAssigneesAndUsers)) && len(input.Recipients) == 0 {
		return storage.ReminderRule{}, nil, RuntimeError{Code: "reminder_rule_invalid", Message: "explicit recipient list is required"}
	}
	recipientIDs, err := s.resolveRecipientsInWorkspace(input.Recipients)
	if err != nil {
		return storage.ReminderRule{}, nil, err
	}
	if audience == string(ReminderAudienceAssignees) && len(recipientIDs) > 0 {
		return storage.ReminderRule{}, nil, RuntimeError{Code: "reminder_rule_invalid", Message: "assignees audience cannot set explicit recipients"}
	}
	sink, err := s.resolveNotificationSink(input.SinkRef)
	if err != nil {
		return storage.ReminderRule{}, nil, err
	}
	var projectID *string
	if strings.TrimSpace(input.ProjectRef) != "" {
		project, err := s.ResolveProject(input.ProjectRef)
		if err != nil {
			return storage.ReminderRule{}, nil, err
		}
		projectID = &project.ID
	}
	enabled := true
	if existing != nil && existing.Enabled != nil {
		enabled = *existing.Enabled
	}
	now := s.clock.Unix()
	row := storage.ReminderRule{
		ID:                   uuid.NewString(),
		WorkspaceID:          s.workspaceID,
		ProjectID:            projectID,
		Name:                 name,
		Enabled:              &enabled,
		TriggerType:          trigger,
		OffsetSeconds:        input.OffsetSeconds,
		AfterSeconds:         input.AfterSeconds,
		RepeatPolicy:         repeatPolicy,
		TaskFilterJSON:       encodeReminderTaskFilterJSON(taskFilter),
		AudienceType:         audience,
		RecipientUserIDsJSON: mustJSON(recipientIDs),
		SinkID:               sink.ID,
		CreatedBy:            s.runtime.ActorUserID,
		CreatedAt:            now,
		ModifiedAt:           now,
	}
	audit := map[string]any{
		"name":           row.Name,
		"project_id":     row.ProjectID,
		"trigger_type":   row.TriggerType,
		"offset_seconds": row.OffsetSeconds,
		"after_seconds":  row.AfterSeconds,
		"repeat_policy":  row.RepeatPolicy,
		"task_filter":    taskFilter,
		"audience_type":  row.AudienceType,
		"sink_id":        row.SinkID,
	}
	if len(recipientIDs) > 0 {
		audit["recipient_user_ids"] = recipientIDs
	}
	return row, audit, nil
}

func (s *Service) applyReminderRuleModify(row storage.ReminderRule, input ReminderRuleModifyInput) (storage.ReminderRule, map[string]any, error) {
	next := row
	audit := map[string]any{"name": row.Name}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return storage.ReminderRule{}, nil, RuntimeError{Code: "reminder_rule_invalid", Message: "reminder rule name is required"}
		}
		next.Name = name
		audit["name"] = name
	}
	if input.ProjectRef != nil {
		if strings.TrimSpace(*input.ProjectRef) == "" {
			next.ProjectID = nil
			audit["project_id"] = nil
		} else {
			project, err := s.ResolveProject(*input.ProjectRef)
			if err != nil {
				return storage.ReminderRule{}, nil, err
			}
			next.ProjectID = &project.ID
			audit["project_id"] = project.ID
		}
	}
	if input.TriggerType != nil {
		tr := strings.TrimSpace(*input.TriggerType)
		if tr != "due_before" && tr != "overdue" {
			return storage.ReminderRule{}, nil, RuntimeError{Code: "reminder_rule_invalid", Message: fmt.Sprintf("unsupported reminder trigger %q", tr)}
		}
		next.TriggerType = tr
		audit["trigger_type"] = tr
	}
	if input.OffsetSeconds != nil {
		next.OffsetSeconds = *input.OffsetSeconds
		audit["offset_seconds"] = *input.OffsetSeconds
	}
	if input.AfterSeconds != nil {
		next.AfterSeconds = *input.AfterSeconds
		audit["after_seconds"] = *input.AfterSeconds
	}
	if input.RepeatPolicy != nil {
		repeatPolicy := strings.TrimSpace(*input.RepeatPolicy)
		if repeatPolicy == "" {
			repeatPolicy = "once"
		}
		if repeatPolicy != "once" && !strings.HasPrefix(repeatPolicy, "every:") {
			return storage.ReminderRule{}, nil, RuntimeError{Code: "reminder_rule_invalid", Message: fmt.Sprintf("unsupported repeat policy %q", repeatPolicy)}
		}
		if strings.HasPrefix(repeatPolicy, "every:") {
			if _, err := parseRepeatDuration(strings.TrimPrefix(repeatPolicy, "every:")); err != nil {
				return storage.ReminderRule{}, nil, err
			}
		}
		next.RepeatPolicy = repeatPolicy
		audit["repeat_policy"] = repeatPolicy
	}
	if input.TaskFilter != nil {
		taskFilter, err := normalizeReminderTaskFilter(*input.TaskFilter)
		if err != nil {
			return storage.ReminderRule{}, nil, err
		}
		next.TaskFilterJSON = encodeReminderTaskFilterJSON(taskFilter)
		audit["task_filter"] = taskFilter
	}
	if input.AudienceType != nil {
		audience := strings.TrimSpace(*input.AudienceType)
		switch audience {
		case string(ReminderAudienceAssignees), string(ReminderAudienceExplicitUsers), string(ReminderAudienceAssigneesAndUsers):
		default:
			return storage.ReminderRule{}, nil, RuntimeError{Code: "audience_unsupported", Message: fmt.Sprintf("unsupported audience %q", audience)}
		}
		next.AudienceType = audience
		audit["audience_type"] = audience
	}
	if input.Recipients != nil {
		recipientIDs, err := s.resolveRecipientsInWorkspace(*input.Recipients)
		if err != nil {
			return storage.ReminderRule{}, nil, err
		}
		next.RecipientUserIDsJSON = mustJSON(recipientIDs)
		audit["recipient_user_ids"] = recipientIDs
	}
	if input.SinkRef != nil {
		sink, err := s.resolveNotificationSink(*input.SinkRef)
		if err != nil {
			return storage.ReminderRule{}, nil, err
		}
		next.SinkID = sink.ID
		audit["sink_id"] = sink.ID
	}
	if input.Enabled != nil {
		next.Enabled = input.Enabled
		audit["enabled"] = *input.Enabled
	}
	if next.TriggerType == "due_before" && next.OffsetSeconds <= 0 {
		return storage.ReminderRule{}, nil, RuntimeError{Code: "reminder_rule_invalid", Message: "due_before rule requires positive offset"}
	}
	if next.TriggerType == "overdue" && next.AfterSeconds < 0 {
		return storage.ReminderRule{}, nil, RuntimeError{Code: "reminder_rule_invalid", Message: "overdue rule requires non-negative after seconds"}
	}
	if next.AudienceType == string(ReminderAudienceAssignees) && strings.TrimSpace(next.RecipientUserIDsJSON) != "[]" {
		return storage.ReminderRule{}, nil, RuntimeError{Code: "reminder_rule_invalid", Message: "assignees audience cannot set explicit recipients"}
	}
	if next.AudienceType == string(ReminderAudienceExplicitUsers) || next.AudienceType == string(ReminderAudienceAssigneesAndUsers) {
		var ids []string
		_ = json.Unmarshal([]byte(next.RecipientUserIDsJSON), &ids)
		if len(ids) == 0 {
			return storage.ReminderRule{}, nil, RuntimeError{Code: "reminder_rule_invalid", Message: "explicit recipient list is required"}
		}
	}
	next.ModifiedAt = s.clock.Unix()
	return next, audit, nil
}

func decodeJSONStringSlice(raw string) []string {
	values, err := decodeStringListJSON(raw)
	if err != nil {
		return nil
	}
	return values
}

func normalizeReminderTaskFilter(source string) (string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		source = defaultReminderTaskFilter
	}
	if _, err := query.ParseQuery(source); err != nil {
		return "", RuntimeError{Code: "reminder_rule_invalid", Message: err.Error()}
	}
	return source, nil
}

func encodeReminderTaskFilterJSON(source string) string {
	return mustJSON(reminderTaskFilterSpec{Source: source})
}

func decodeReminderTaskFilterSource(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return defaultReminderTaskFilter
	}
	var spec reminderTaskFilterSpec
	if err := json.Unmarshal([]byte(raw), &spec); err == nil && strings.TrimSpace(spec.Source) != "" {
		return strings.TrimSpace(spec.Source)
	}
	var legacy string
	if err := json.Unmarshal([]byte(raw), &legacy); err == nil && strings.TrimSpace(legacy) != "" {
		return strings.TrimSpace(legacy)
	}
	return defaultReminderTaskFilter
}

func decodeHeaderTemplateRows(raw string) []HTTPHeaderTemplateView {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var rows []HTTPHeaderTemplateView
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return nil
	}
	return rows
}

func decodeSecretRefRows(raw string) []HTTPTemplateSecretRefView {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var rows []HTTPTemplateSecretRefView
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return nil
	}
	return rows
}

func decodeHeadersMap(raw string) map[string]string {
	if strings.TrimSpace(raw) == "" {
		return map[string]string{}
	}
	var rows map[string]string
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return map[string]string{}
	}
	return rows
}

func decodeAnyMap(raw string) map[string]any {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}
	}
	var rows map[string]any
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return map[string]any{}
	}
	return rows
}

func uniqueTrimmed(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func mustJSON(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

func defaultNotificationTimeout(v int) int {
	if v <= 0 {
		return 10
	}
	return v
}

func defaultNotificationMaxAttempts(v int) int {
	if v <= 0 {
		return 5
	}
	return v
}

func validateHTTPHeaderTemplates(rows []HTTPHeaderTemplateInput) error {
	for _, row := range rows {
		name := strings.TrimSpace(row.Name)
		if name == "" {
			return RuntimeError{Code: "notification_sink_invalid", Message: "header template name is required"}
		}
		if strings.ContainsAny(name, "\r\n") {
			return RuntimeError{Code: "notification_sink_invalid", Message: "header template name contains newline"}
		}
	}
	return nil
}

func validateHTTPTemplateBody(body, contentType string) error {
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/json"
	}
	if strings.EqualFold(contentType, "application/json") {
		re := regexp.MustCompile(`\{\{[^}]+\}\}`)
		normalized := re.ReplaceAllString(body, "XUANCHU_PLACEHOLDER")
		var payload any
		if err := json.Unmarshal([]byte(normalized), &payload); err != nil {
			return RuntimeError{Code: "notification_sink_invalid", Message: fmt.Sprintf("invalid json body template: %v", err)}
		}
	}
	return nil
}

func (s *Service) validateSecretRefs(rows []HTTPTemplateSecretRefInput) ([]HTTPTemplateSecretRefView, error) {
	out := make([]HTTPTemplateSecretRefView, 0, len(rows))
	for _, row := range rows {
		alias := strings.TrimSpace(row.Alias)
		key := strings.TrimSpace(row.ConfigKey)
		if alias == "" || key == "" {
			return nil, RuntimeError{Code: "notification_sink_invalid", Message: "secret ref alias and config key are required"}
		}
		def, ok, err := s.configDefRepo.Get(s.workspaceID, key)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, RuntimeError{Code: "config_definition_not_found", Message: fmt.Sprintf("config definition %q not found", key)}
		}
		view, err := configDefinitionViewFromRow(def)
		if err != nil {
			return nil, err
		}
		if !configDefinitionAllowsScope(view, storage.ConfigScopeWorkspace) && !configDefinitionAllowsScope(view, storage.ConfigScopeProject) {
			return nil, RuntimeError{Code: "notification_sink_invalid", Message: fmt.Sprintf("secret ref key %q must allow workspace or project scope", key)}
		}
		if !view.Secret {
			return nil, RuntimeError{Code: "notification_sink_invalid", Message: fmt.Sprintf("config key %q is not secret", key)}
		}
		out = append(out, HTTPTemplateSecretRefView{Alias: alias, ConfigKey: key})
	}
	return out, nil
}

func (s *Service) resolveRecipientsInWorkspace(refs []string) ([]string, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		user, err := s.resolveUser(strings.TrimSpace(ref))
		if err != nil {
			return nil, err
		}
		if _, err := s.memberRepo.Get(user.ID, s.runtime.WorkspaceID); err != nil {
			if err == storage.ErrNotFound {
				return nil, RuntimeError{Code: "membership_not_found", Message: fmt.Sprintf("user %q is not a member of workspace %q", user.Name, s.runtime.WorkspaceSlug)}
			}
			return nil, err
		}
		ids = append(ids, user.ID)
	}
	return uniqueTrimmed(ids), nil
}

func parseRepeatDuration(raw string) (int64, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, RuntimeError{Code: "reminder_rule_invalid", Message: "repeat duration is required"}
	}
	dur, err := time.ParseDuration(raw)
	if err != nil {
		return 0, RuntimeError{Code: "reminder_rule_invalid", Message: fmt.Sprintf("invalid repeat duration %q", raw)}
	}
	if dur <= 0 {
		return 0, RuntimeError{Code: "reminder_rule_invalid", Message: "repeat duration must be positive"}
	}
	return int64(dur.Seconds()), nil
}

func (s *Service) notificationSecretValuesForSink(sink storage.NotificationSink, project *storage.Project) []string {
	refs := decodeSecretRefRows(sink.SecretRefsJSON)
	if len(refs) == 0 {
		return nil
	}
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		value, _, err := s.resolveSecretConfigValue(ref.ConfigKey, project)
		if err != nil || value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func redactNotificationString(raw string, secrets []string) string {
	if raw == "" || len(secrets) == 0 {
		return raw
	}
	out := raw
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		out = strings.ReplaceAll(out, secret, "[redacted]")
	}
	return out
}

func redactNotificationStringMap(raw map[string]string, secrets []string) map[string]string {
	if len(raw) == 0 || len(secrets) == 0 {
		return raw
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		out[k] = redactNotificationString(v, secrets)
	}
	return out
}
