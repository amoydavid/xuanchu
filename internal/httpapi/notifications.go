package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

type notificationSinkRequest struct {
	Name            string                           `json:"name"`
	Type            string                           `json:"type"`
	EndpointMode    string                           `json:"endpoint_mode"`
	URL             string                           `json:"url"`
	URLTemplate     string                           `json:"url_template"`
	ConfigKey       string                           `json:"config_key"`
	AllowedHosts    []string                         `json:"allowed_hosts"`
	HeaderTemplates []app.HTTPHeaderTemplateInput    `json:"header_templates"`
	BodyTemplate    string                           `json:"body_template"`
	BodyContentType string                           `json:"body_content_type"`
	SecretRefs      []app.HTTPTemplateSecretRefInput `json:"secret_refs"`
	Secret          string                           `json:"secret,omitempty"`
	TimeoutSeconds  int                              `json:"timeout_seconds,omitempty"`
	MaxAttempts     int                              `json:"max_attempts,omitempty"`
	MaxConcurrency  int                              `json:"max_concurrency,omitempty"`
}

type notificationSinkModifyRequest struct {
	Name            *string                           `json:"name,omitempty"`
	Type            *string                           `json:"type,omitempty"`
	EndpointMode    *string                           `json:"endpoint_mode,omitempty"`
	URL             *string                           `json:"url,omitempty"`
	URLTemplate     *string                           `json:"url_template,omitempty"`
	ConfigKey       *string                           `json:"config_key,omitempty"`
	AllowedHosts    *[]string                         `json:"allowed_hosts,omitempty"`
	HeaderTemplates *[]app.HTTPHeaderTemplateInput    `json:"header_templates,omitempty"`
	BodyTemplate    *string                           `json:"body_template,omitempty"`
	BodyContentType *string                           `json:"body_content_type,omitempty"`
	SecretRefs      *[]app.HTTPTemplateSecretRefInput `json:"secret_refs,omitempty"`
	Secret          *string                           `json:"secret,omitempty"`
	TimeoutSeconds  *int                              `json:"timeout_seconds,omitempty"`
	MaxAttempts     *int                              `json:"max_attempts,omitempty"`
	MaxConcurrency  *int                              `json:"max_concurrency,omitempty"`
}

type reminderRuleRequest struct {
	Name          string   `json:"name"`
	ProjectRef    string   `json:"project_ref"`
	TriggerType   string   `json:"trigger_type"`
	OffsetSeconds int64    `json:"offset_seconds"`
	AfterSeconds  int64    `json:"after_seconds"`
	RepeatPolicy  string   `json:"repeat_policy"`
	ScheduleType  string   `json:"schedule_type"`
	ScheduleValue string   `json:"schedule_value"`
	Timezone      string   `json:"timezone"`
	FilterSource  string   `json:"filter_source"`
	AudienceType  string   `json:"audience_type"`
	Recipients    []string `json:"recipients"`
	SinkRef       string   `json:"sink_ref"`
}

type reminderRuleModifyRequest struct {
	Name          *string   `json:"name,omitempty"`
	ProjectRef    *string   `json:"project_ref,omitempty"`
	TriggerType   *string   `json:"trigger_type,omitempty"`
	OffsetSeconds *int64    `json:"offset_seconds,omitempty"`
	AfterSeconds  *int64    `json:"after_seconds,omitempty"`
	RepeatPolicy  *string   `json:"repeat_policy,omitempty"`
	ScheduleType  *string   `json:"schedule_type,omitempty"`
	ScheduleValue *string   `json:"schedule_value,omitempty"`
	Timezone      *string   `json:"timezone,omitempty"`
	FilterSource  *string   `json:"filter_source,omitempty"`
	AudienceType  *string   `json:"audience_type,omitempty"`
	Recipients    *[]string `json:"recipients,omitempty"`
	SinkRef       *string   `json:"sink_ref,omitempty"`
}

type eventNotificationRuleRequest struct {
	Name            string   `json:"name"`
	ProjectRef      string   `json:"project_ref"`
	EventType       string   `json:"event_type"`
	FilterSource    string   `json:"filter_source"`
	AudienceType    string   `json:"audience_type"`
	Recipients      []string `json:"recipients"`
	Sink            string   `json:"sink"`
	URL             string   `json:"url,omitempty"`
	EndpointURL     string   `json:"endpoint_url,omitempty"`
	TemplateSubject string   `json:"template_subject"`
	TemplateBody    string   `json:"template_body"`
}

type eventNotificationRuleModifyRequest struct {
	Name            *string   `json:"name,omitempty"`
	ProjectRef      *string   `json:"project_ref,omitempty"`
	EventType       *string   `json:"event_type,omitempty"`
	FilterSource    *string   `json:"filter_source,omitempty"`
	AudienceType    *string   `json:"audience_type,omitempty"`
	Recipients      *[]string `json:"recipients,omitempty"`
	Sink            *string   `json:"sink,omitempty"`
	URL             *string   `json:"url,omitempty"`
	EndpointURL     *string   `json:"endpoint_url,omitempty"`
	TemplateSubject *string   `json:"template_subject,omitempty"`
	TemplateBody    *string   `json:"template_body,omitempty"`
}

func (s *Server) handleNotificationSinkList(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, auth.ScopeNotificationRead, app.PermissionNotificationRead, "")
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	rows, err := scoped.ListNotificationSinks(r.URL.Query().Get("all") == "true")
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, notificationSinkResponse(row))
	}
	writeSuccess(w, http.StatusOK, out, nil)
}

func (s *Server) handleNotificationSinkCreate(w http.ResponseWriter, r *http.Request) {
	var req notificationSinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	scoped, _, err := s.scopedService(r, auth.ScopeNotificationWrite, app.PermissionNotificationWrite, "")
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	view, err := scoped.AddNotificationSink(app.NotificationSinkAddInput{
		Name:            req.Name,
		Type:            req.Type,
		EndpointMode:    req.EndpointMode,
		URL:             req.URL,
		URLTemplate:     req.URLTemplate,
		ConfigKey:       req.ConfigKey,
		AllowedHosts:    req.AllowedHosts,
		HTTPMethod:      "POST",
		HeaderTemplates: req.HeaderTemplates,
		BodyTemplate:    req.BodyTemplate,
		BodyContentType: req.BodyContentType,
		SecretRefs:      req.SecretRefs,
		Secret:          req.Secret,
		TimeoutSeconds:  req.TimeoutSeconds,
		MaxAttempts:     req.MaxAttempts,
		MaxConcurrency:  req.MaxConcurrency,
	})
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, notificationSinkResponse(view), nil)
}

func (s *Server) handleNotificationSinkInfo(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, auth.ScopeNotificationRead, app.PermissionNotificationRead, "")
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	view, err := scoped.NotificationSinkInfo(chi.URLParam(r, "sinkID"))
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, notificationSinkResponse(view), nil)
}

func (s *Server) handleNotificationSinkModify(w http.ResponseWriter, r *http.Request) {
	var req notificationSinkModifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	scoped, _, err := s.scopedService(r, auth.ScopeNotificationWrite, app.PermissionNotificationWrite, "")
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	view, err := scoped.ModifyNotificationSink(chi.URLParam(r, "sinkID"), app.NotificationSinkModifyInput{
		Name:            req.Name,
		Type:            req.Type,
		EndpointMode:    req.EndpointMode,
		URL:             req.URL,
		URLTemplate:     req.URLTemplate,
		ConfigKey:       req.ConfigKey,
		AllowedHosts:    req.AllowedHosts,
		HeaderTemplates: req.HeaderTemplates,
		BodyTemplate:    req.BodyTemplate,
		BodyContentType: req.BodyContentType,
		SecretRefs:      req.SecretRefs,
		Secret:          req.Secret,
		TimeoutSeconds:  req.TimeoutSeconds,
		MaxAttempts:     req.MaxAttempts,
		MaxConcurrency:  req.MaxConcurrency,
	})
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, notificationSinkResponse(view), nil)
}

func (s *Server) handleNotificationSinkEnable(w http.ResponseWriter, r *http.Request) {
	s.handleNotificationSinkToggle(w, r, true)
}

func (s *Server) handleNotificationSinkDisable(w http.ResponseWriter, r *http.Request) {
	s.handleNotificationSinkToggle(w, r, false)
}

func (s *Server) handleNotificationSinkToggle(w http.ResponseWriter, r *http.Request, enabled bool) {
	scoped, _, err := s.scopedService(r, auth.ScopeNotificationWrite, app.PermissionNotificationWrite, "")
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	if enabled {
		view, err := scoped.EnableNotificationSink(chi.URLParam(r, "sinkID"))
		if err != nil {
			s.writeAppError(w, err)
			return
		}
		writeSuccess(w, http.StatusOK, notificationSinkResponse(view), nil)
		return
	}
	view, err := scoped.DisableNotificationSink(chi.URLParam(r, "sinkID"))
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, notificationSinkResponse(view), nil)
}

func (s *Server) handleNotificationSinkDelete(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, auth.ScopeNotificationWrite, app.PermissionNotificationWrite, "")
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	if err := scoped.DeleteNotificationSink(chi.URLParam(r, "sinkID")); err != nil {
		s.writeAppError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type notificationSinkTestRequest struct {
	Kind       string `json:"kind"`
	EventType  string `json:"event_type"`
	Sample     string `json:"sample"`
	ProjectRef string `json:"project_ref"`
}

func (s *Server) handleNotificationSinkTest(w http.ResponseWriter, r *http.Request) {
	var req notificationSinkTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	scoped, _, err := s.scopedService(r, auth.ScopeNotificationWrite, app.PermissionNotificationWrite, "")
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	view, err := scoped.TestNotificationSink(chi.URLParam(r, "sinkID"), app.NotificationSinkTestInput{
		Kind:       req.Kind,
		EventType:  req.EventType,
		Sample:     req.Sample,
		ProjectRef: req.ProjectRef,
	})
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, notificationSinkTestResponse(view), nil)
}

func notificationSinkTestResponse(view app.NotificationSinkTestView) map[string]any {
	out := map[string]any{
		"status":                        view.Status,
		"duration_ms":                   view.DurationMS,
		"resolved_endpoint_source":      view.ResolvedEndpointSource,
		"resolved_endpoint_fingerprint": view.ResolvedEndpointFingerprint,
		"rendered_method":               view.RenderedMethod,
		"rendered_headers":              view.RenderedHeaders,
		"rendered_body_preview":         view.RenderedBodyPreview,
		"error":                         view.Error,
	}
	if view.StatusCode != nil {
		out["status_code"] = *view.StatusCode
	}
	return out
}

func (s *Server) handleReminderRuleList(w http.ResponseWriter, r *http.Request) {
	projectRef := requestProjectRef(r)
	scoped, _, err := s.scopedService(r, auth.ScopeReminderRead, app.PermissionReminderRead, projectRef)
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	rows, err := scoped.ListReminderRules(projectRef, r.URL.Query().Get("all") == "true")
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, reminderRuleResponse(row))
	}
	writeSuccess(w, http.StatusOK, out, nil)
}

func (s *Server) handleReminderRuleCreate(w http.ResponseWriter, r *http.Request) {
	var req reminderRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	projectRef := req.ProjectRef
	scoped, _, err := s.scopedService(r, auth.ScopeReminderWrite, app.PermissionReminderWrite, projectRef)
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	view, err := scoped.AddReminderRule(app.ReminderRuleAddInput{
		Name:          req.Name,
		ProjectRef:    req.ProjectRef,
		TriggerType:   req.TriggerType,
		OffsetSeconds: req.OffsetSeconds,
		AfterSeconds:  req.AfterSeconds,
		RepeatPolicy:  req.RepeatPolicy,
		ScheduleType:  req.ScheduleType,
		ScheduleValue: req.ScheduleValue,
		Timezone:      req.Timezone,
		FilterSource:  req.FilterSource,
		AudienceType:  req.AudienceType,
		Recipients:    req.Recipients,
		SinkRef:       req.SinkRef,
	})
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, reminderRuleResponse(view), nil)
}

func (s *Server) handleReminderRuleInfo(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, auth.ScopeReminderRead, app.PermissionReminderRead, "")
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	view, err := scoped.ReminderRuleInfo(chi.URLParam(r, "ruleID"))
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, reminderRuleResponse(view), nil)
}

func (s *Server) handleReminderRuleModify(w http.ResponseWriter, r *http.Request) {
	var req reminderRuleModifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	projectRef := ""
	if req.ProjectRef != nil {
		projectRef = *req.ProjectRef
	}
	scoped, _, err := s.scopedService(r, auth.ScopeReminderWrite, app.PermissionReminderWrite, projectRef)
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	view, err := scoped.ModifyReminderRule(chi.URLParam(r, "ruleID"), app.ReminderRuleModifyInput{
		Name:          req.Name,
		ProjectRef:    req.ProjectRef,
		TriggerType:   req.TriggerType,
		OffsetSeconds: req.OffsetSeconds,
		AfterSeconds:  req.AfterSeconds,
		RepeatPolicy:  req.RepeatPolicy,
		ScheduleType:  req.ScheduleType,
		ScheduleValue: req.ScheduleValue,
		Timezone:      req.Timezone,
		FilterSource:  req.FilterSource,
		AudienceType:  req.AudienceType,
		Recipients:    req.Recipients,
		SinkRef:       req.SinkRef,
	})
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, reminderRuleResponse(view), nil)
}

func (s *Server) handleReminderRuleEnable(w http.ResponseWriter, r *http.Request) {
	s.handleReminderRuleToggle(w, r, true)
}

func (s *Server) handleReminderRuleDisable(w http.ResponseWriter, r *http.Request) {
	s.handleReminderRuleToggle(w, r, false)
}

func (s *Server) handleReminderRuleToggle(w http.ResponseWriter, r *http.Request, enabled bool) {
	scoped, _, err := s.scopedService(r, auth.ScopeReminderWrite, app.PermissionReminderWrite, "")
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	if enabled {
		view, err := scoped.EnableReminderRule(chi.URLParam(r, "ruleID"))
		if err != nil {
			s.writeAppError(w, err)
			return
		}
		writeSuccess(w, http.StatusOK, reminderRuleResponse(view), nil)
		return
	}
	view, err := scoped.DisableReminderRule(chi.URLParam(r, "ruleID"))
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, reminderRuleResponse(view), nil)
}

func (s *Server) handleReminderRuleDelete(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, auth.ScopeReminderWrite, app.PermissionReminderWrite, "")
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	if err := scoped.DeleteReminderRule(chi.URLParam(r, "ruleID")); err != nil {
		s.writeAppError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleEventNotificationRuleList(w http.ResponseWriter, r *http.Request) {
	projectRef := requestProjectRef(r)
	scoped, _, err := s.scopedService(r, auth.ScopeNotificationRead, app.PermissionNotificationRead, projectRef)
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	rows, err := scoped.ListEventNotificationRules(projectRef, r.URL.Query().Get("all") == "true")
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, eventNotificationRuleResponse(row))
	}
	writeSuccess(w, http.StatusOK, out, nil)
}

func (s *Server) handleEventNotificationRuleCreate(w http.ResponseWriter, r *http.Request) {
	var req eventNotificationRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	if req.URL != "" || req.EndpointURL != "" {
		writeError(w, http.StatusBadRequest, "notification_rule_url_not_supported", "notification rule uses sink, not url", nil)
		return
	}
	scoped, _, err := s.scopedService(r, auth.ScopeNotificationWrite, app.PermissionNotificationWrite, req.ProjectRef)
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	view, err := scoped.AddEventNotificationRule(app.EventNotificationRuleAddInput{
		Name:            req.Name,
		ProjectRef:      req.ProjectRef,
		EventType:       req.EventType,
		FilterSource:    req.FilterSource,
		AudienceType:    req.AudienceType,
		Recipients:      req.Recipients,
		SinkRef:         req.Sink,
		TemplateSubject: req.TemplateSubject,
		TemplateBody:    req.TemplateBody,
	})
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, eventNotificationRuleResponse(view), nil)
}

func (s *Server) handleEventNotificationRuleInfo(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, auth.ScopeNotificationRead, app.PermissionNotificationRead, "")
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	view, err := scoped.EventNotificationRuleInfo(chi.URLParam(r, "ruleID"))
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, eventNotificationRuleResponse(view), nil)
}

func (s *Server) handleEventNotificationRuleModify(w http.ResponseWriter, r *http.Request) {
	var req eventNotificationRuleModifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	if req.URL != nil || req.EndpointURL != nil {
		writeError(w, http.StatusBadRequest, "notification_rule_url_not_supported", "notification rule uses sink, not url", nil)
		return
	}
	projectRef := ""
	if req.ProjectRef != nil {
		projectRef = *req.ProjectRef
	}
	scoped, _, err := s.scopedService(r, auth.ScopeNotificationWrite, app.PermissionNotificationWrite, projectRef)
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	view, err := scoped.ModifyEventNotificationRule(chi.URLParam(r, "ruleID"), app.EventNotificationRuleModifyInput{
		Name:            req.Name,
		ProjectRef:      req.ProjectRef,
		EventType:       req.EventType,
		FilterSource:    req.FilterSource,
		AudienceType:    req.AudienceType,
		Recipients:      req.Recipients,
		SinkRef:         req.Sink,
		TemplateSubject: req.TemplateSubject,
		TemplateBody:    req.TemplateBody,
	})
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, eventNotificationRuleResponse(view), nil)
}

func (s *Server) handleEventNotificationRuleEnable(w http.ResponseWriter, r *http.Request) {
	s.handleEventNotificationRuleToggle(w, r, true)
}

func (s *Server) handleEventNotificationRuleDisable(w http.ResponseWriter, r *http.Request) {
	s.handleEventNotificationRuleToggle(w, r, false)
}

func (s *Server) handleEventNotificationRuleToggle(w http.ResponseWriter, r *http.Request, enabled bool) {
	scoped, _, err := s.scopedService(r, auth.ScopeNotificationWrite, app.PermissionNotificationWrite, "")
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	if enabled {
		view, err := scoped.EnableEventNotificationRule(chi.URLParam(r, "ruleID"))
		if err != nil {
			s.writeAppError(w, err)
			return
		}
		writeSuccess(w, http.StatusOK, eventNotificationRuleResponse(view), nil)
		return
	}
	view, err := scoped.DisableEventNotificationRule(chi.URLParam(r, "ruleID"))
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, eventNotificationRuleResponse(view), nil)
}

func (s *Server) handleEventNotificationRuleDelete(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, auth.ScopeNotificationWrite, app.PermissionNotificationWrite, "")
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	if err := scoped.DeleteEventNotificationRule(chi.URLParam(r, "ruleID")); err != nil {
		s.writeAppError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleNotificationDeliveryList(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, auth.ScopeNotificationRead, app.PermissionNotificationRead, "")
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			writeError(w, http.StatusBadRequest, "api_bad_limit", "invalid limit", nil)
			return
		}
		limit = parsed
	}
	rows, err := scoped.ListNotificationDeliveries(r.URL.Query().Get("sink"), r.URL.Query().Get("status"), limit, 0)
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, notificationDeliveryResponse(row))
	}
	writeSuccess(w, http.StatusOK, out, nil)
}

func (s *Server) handleNotificationDeliveryInfo(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, auth.ScopeNotificationRead, app.PermissionNotificationRead, "")
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	view, err := scoped.NotificationDeliveryInfo(chi.URLParam(r, "deliveryID"))
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, notificationDeliveryResponse(view), nil)
}

func (s *Server) handleNotificationDeliveryReplay(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, auth.ScopeNotificationWrite, app.PermissionNotificationWrite, "")
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	view, err := scoped.ReplayNotificationDelivery(chi.URLParam(r, "deliveryID"))
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, notificationDeliveryResponse(view), nil)
}

func notificationSinkResponse(row app.NotificationSinkView) map[string]any {
	return map[string]any{
		"id":                row.ID,
		"workspace_id":      row.WorkspaceID,
		"name":              row.Name,
		"type":              row.Type,
		"endpoint_mode":     row.EndpointMode,
		"url":               row.URL,
		"url_template":      row.URLTemplate,
		"config_key":        row.ConfigKey,
		"allowed_hosts":     row.AllowedHosts,
		"http_method":       row.HTTPMethod,
		"header_templates":  row.HeaderTemplates,
		"body_template":     row.BodyTemplate,
		"body_content_type": row.BodyContentType,
		"secret_refs":       row.SecretRefs,
		"enabled":           row.Enabled,
		"timeout_seconds":   row.TimeoutSeconds,
		"max_attempts":      row.MaxAttempts,
		"max_concurrency":   row.MaxConcurrency,
		"created_by":        task.ActorInfoToJSON(row.CreatedBy),
		"created_at":        row.CreatedAt,
		"modified_at":       row.ModifiedAt,
	}
}

func reminderRuleResponse(row app.ReminderRuleView) map[string]any {
	return map[string]any{
		"id":              row.ID,
		"workspace_id":    row.WorkspaceID,
		"project_id":      row.ProjectID,
		"name":            row.Name,
		"enabled":         row.Enabled,
		"trigger_type":    row.TriggerType,
		"offset_seconds":  row.OffsetSeconds,
		"after_seconds":   row.AfterSeconds,
		"repeat_policy":   row.RepeatPolicy,
		"schedule_type":   row.ScheduleType,
		"schedule_value":  row.ScheduleValue,
		"timezone":        row.Timezone,
		"filter_source":   row.FilterSource,
		"audience_type":   row.AudienceType,
		"recipient_users": userInfosToJSON(row.RecipientUsers),
		"sink_id":         row.SinkID,
		"created_by":      task.ActorInfoToJSON(row.CreatedBy),
		"created_at":      row.CreatedAt,
		"modified_at":     row.ModifiedAt,
	}
}

func eventNotificationRuleResponse(row app.EventNotificationRuleView) map[string]any {
	return map[string]any{
		"id":               row.ID,
		"workspace_id":     row.WorkspaceID,
		"project_id":       row.ProjectID,
		"name":             row.Name,
		"enabled":          row.Enabled,
		"event_type":       row.EventType,
		"filter_source":    row.FilterSource,
		"audience_type":    row.AudienceType,
		"recipient_users":  userInfosToJSON(row.RecipientUsers),
		"sink_id":          row.SinkID,
		"template_subject": row.TemplateSubject,
		"template_body":    row.TemplateBody,
		"created_by":       task.ActorInfoToJSON(row.CreatedBy),
		"created_at":       row.CreatedAt,
		"modified_at":      row.ModifiedAt,
	}
}

func notificationDeliveryResponse(row app.NotificationDeliveryView) map[string]any {
	return map[string]any{
		"id":                            row.ID,
		"workspace_id":                  row.WorkspaceID,
		"project_id":                    row.ProjectID,
		"rule_id":                       row.RuleID,
		"sink_id":                       row.SinkID,
		"task_uuid":                     row.TaskUUID,
		"object_kind":                   row.ObjectKind,
		"object_id":                     row.ObjectID,
		"recipient":                     task.UserInfoToJSON(row.Recipient),
		"event_id":                      row.EventID,
		"event_type":                    row.EventType,
		"actor":                         task.ActorInfoToJSON(row.Actor),
		"resolved_url":                  row.ResolvedURL,
		"resolved_endpoint_source":      row.ResolvedEndpointSource,
		"resolved_endpoint_fingerprint": row.ResolvedEndpointFingerprint,
		"rendered_method":               row.RenderedMethod,
		"rendered_headers":              row.RenderedHeaders,
		"rendered_body":                 row.RenderedBody,
		"rendered_content_type":         row.RenderedContentType,
		"payload":                       row.Payload,
		"status":                        row.Status,
		"attempt_count":                 row.AttemptCount,
		"next_attempt_at":               row.NextAttemptAt,
		"claim_expires_at":              row.ClaimExpiresAt,
		"last_attempt_at":               row.LastAttemptAt,
		"last_status_code":              row.LastStatusCode,
		"last_error":                    row.LastError,
		"created_at":                    row.CreatedAt,
		"modified_at":                   row.ModifiedAt,
	}
}

func userInfosToJSON(rows []task.UserInfo) []task.JSONUserInfo {
	out := make([]task.JSONUserInfo, 0, len(rows))
	for _, row := range rows {
		out = append(out, task.UserInfoToJSON(row))
	}
	return out
}
