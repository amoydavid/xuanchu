package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"git.dajee.net/dajee/xuanchu/internal/app"
)

type httpHeaderTemplateRequest struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type httpTemplateSecretRefRequest struct {
	Alias     string `json:"alias"`
	ConfigKey string `json:"config_key"`
}

type notificationSinkAddRequest struct {
	Name            string                         `json:"name"`
	Type            string                         `json:"type"`
	EndpointMode    string                         `json:"endpoint_mode"`
	URL             string                         `json:"url,omitempty"`
	URLTemplate     string                         `json:"url_template,omitempty"`
	ConfigKey       string                         `json:"config_key,omitempty"`
	AllowedHosts    []string                       `json:"allowed_hosts,omitempty"`
	HTTPMethod      string                         `json:"http_method,omitempty"`
	HeaderTemplates []httpHeaderTemplateRequest    `json:"header_templates,omitempty"`
	BodyTemplate    string                         `json:"body_template,omitempty"`
	BodyContentType string                         `json:"body_content_type,omitempty"`
	SecretRefs      []httpTemplateSecretRefRequest `json:"secret_refs,omitempty"`
	Secret          string                         `json:"secret,omitempty"`
	TimeoutSeconds  int                            `json:"timeout_seconds,omitempty"`
	MaxAttempts     int                            `json:"max_attempts,omitempty"`
}

type notificationSinkModifyRequest struct {
	Name            *string                         `json:"name,omitempty"`
	Type            *string                         `json:"type,omitempty"`
	EndpointMode    *string                         `json:"endpoint_mode,omitempty"`
	URL             *string                         `json:"url,omitempty"`
	URLTemplate     *string                         `json:"url_template,omitempty"`
	ConfigKey       *string                         `json:"config_key,omitempty"`
	AllowedHosts    *[]string                       `json:"allowed_hosts,omitempty"`
	HTTPMethod      *string                         `json:"http_method,omitempty"`
	HeaderTemplates *[]httpHeaderTemplateRequest    `json:"header_templates,omitempty"`
	BodyTemplate    *string                         `json:"body_template,omitempty"`
	BodyContentType *string                         `json:"body_content_type,omitempty"`
	SecretRefs      *[]httpTemplateSecretRefRequest `json:"secret_refs,omitempty"`
	Secret          *string                         `json:"secret,omitempty"`
	TimeoutSeconds  *int                            `json:"timeout_seconds,omitempty"`
	MaxAttempts     *int                            `json:"max_attempts,omitempty"`
	Enabled         *bool                           `json:"enabled,omitempty"`
}

type reminderRuleAddRequest struct {
	Name          string   `json:"name"`
	ProjectRef    string   `json:"project_ref,omitempty"`
	TriggerType   string   `json:"trigger_type"`
	OffsetSeconds int64    `json:"offset_seconds,omitempty"`
	AfterSeconds  int64    `json:"after_seconds,omitempty"`
	RepeatPolicy  string   `json:"repeat_policy,omitempty"`
	TaskFilter    string   `json:"task_filter,omitempty"`
	AudienceType  string   `json:"audience_type"`
	Recipients    []string `json:"recipients,omitempty"`
	SinkRef       string   `json:"sink_ref"`
}

type reminderRuleModifyRequest struct {
	Name          *string   `json:"name,omitempty"`
	ProjectRef    *string   `json:"project_ref,omitempty"`
	TriggerType   *string   `json:"trigger_type,omitempty"`
	OffsetSeconds *int64    `json:"offset_seconds,omitempty"`
	AfterSeconds  *int64    `json:"after_seconds,omitempty"`
	RepeatPolicy  *string   `json:"repeat_policy,omitempty"`
	TaskFilter    *string   `json:"task_filter,omitempty"`
	AudienceType  *string   `json:"audience_type,omitempty"`
	Recipients    *[]string `json:"recipients,omitempty"`
	SinkRef       *string   `json:"sink_ref,omitempty"`
	Enabled       *bool     `json:"enabled,omitempty"`
}

func (s *Server) handleNotificationSinkList(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, "notification:read", app.PermissionNotificationRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	includeDisabled := r.URL.Query().Get("all") == "1" || r.URL.Query().Get("all") == "true"
	rows, err := scoped.ListNotificationSinks(includeDisabled)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, rows, nil)
}

func (s *Server) handleNotificationSinkCreate(w http.ResponseWriter, r *http.Request) {
	var req notificationSinkAddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	scoped, _, err := s.scopedService(r, "notification:write", app.PermissionNotificationWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	created, err := scoped.AddNotificationSink(app.NotificationSinkAddInput{
		Name:            req.Name,
		Type:            req.Type,
		EndpointMode:    req.EndpointMode,
		URL:             req.URL,
		URLTemplate:     req.URLTemplate,
		ConfigKey:       req.ConfigKey,
		AllowedHosts:    req.AllowedHosts,
		HTTPMethod:      req.HTTPMethod,
		HeaderTemplates: notificationHeaderTemplatesToApp(req.HeaderTemplates),
		BodyTemplate:    req.BodyTemplate,
		BodyContentType: req.BodyContentType,
		SecretRefs:      notificationSecretRefsToApp(req.SecretRefs),
		Secret:          req.Secret,
		TimeoutSeconds:  req.TimeoutSeconds,
		MaxAttempts:     req.MaxAttempts,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, created, nil)
}

func (s *Server) handleNotificationSinkInfo(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, "notification:read", app.PermissionNotificationRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.NotificationSinkInfo(chi.URLParam(r, "sinkID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, view, nil)
}

func (s *Server) handleNotificationSinkModify(w http.ResponseWriter, r *http.Request) {
	var req notificationSinkModifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	scoped, _, err := s.scopedService(r, "notification:write", app.PermissionNotificationWrite, "")
	if err != nil {
		writeAppError(w, err)
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
		HTTPMethod:      req.HTTPMethod,
		HeaderTemplates: notificationHeaderTemplatesPtrToApp(req.HeaderTemplates),
		BodyTemplate:    req.BodyTemplate,
		BodyContentType: req.BodyContentType,
		SecretRefs:      notificationSecretRefsPtrToApp(req.SecretRefs),
		Secret:          req.Secret,
		TimeoutSeconds:  req.TimeoutSeconds,
		MaxAttempts:     req.MaxAttempts,
		Enabled:         req.Enabled,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, view, nil)
}

func (s *Server) handleNotificationSinkEnable(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, "notification:write", app.PermissionNotificationWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.EnableNotificationSink(chi.URLParam(r, "sinkID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, view, nil)
}

func (s *Server) handleNotificationSinkDisable(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, "notification:write", app.PermissionNotificationWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.DisableNotificationSink(chi.URLParam(r, "sinkID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, view, nil)
}

func (s *Server) handleNotificationSinkDelete(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, "notification:write", app.PermissionNotificationWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := scoped.DeleteNotificationSink(chi.URLParam(r, "sinkID")); err != nil {
		writeAppError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleReminderRuleList(w http.ResponseWriter, r *http.Request) {
	projectRef := r.URL.Query().Get("project")
	scoped, _, err := s.scopedService(r, "reminder:read", app.PermissionReminderRead, projectRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	includeDisabled := r.URL.Query().Get("all") == "1" || r.URL.Query().Get("all") == "true"
	rows, err := scoped.ListReminderRules(projectRef, includeDisabled)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, rows, nil)
}

func (s *Server) handleReminderRuleCreate(w http.ResponseWriter, r *http.Request) {
	var req reminderRuleAddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	scoped, _, err := s.scopedService(r, "reminder:write", app.PermissionReminderWrite, req.ProjectRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.AddReminderRule(app.ReminderRuleAddInput{
		Name:          req.Name,
		ProjectRef:    req.ProjectRef,
		TriggerType:   req.TriggerType,
		OffsetSeconds: req.OffsetSeconds,
		AfterSeconds:  req.AfterSeconds,
		RepeatPolicy:  req.RepeatPolicy,
		TaskFilter:    req.TaskFilter,
		AudienceType:  req.AudienceType,
		Recipients:    req.Recipients,
		SinkRef:       req.SinkRef,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, view, nil)
}

func (s *Server) handleReminderRuleInfo(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, "reminder:read", app.PermissionReminderRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.ReminderRuleInfo(chi.URLParam(r, "ruleID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, view, nil)
}

func (s *Server) handleReminderRuleModify(w http.ResponseWriter, r *http.Request) {
	var req reminderRuleModifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	scoped, _, err := s.scopedService(r, "reminder:write", app.PermissionReminderWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.ModifyReminderRule(chi.URLParam(r, "ruleID"), app.ReminderRuleModifyInput{
		Name:          req.Name,
		ProjectRef:    req.ProjectRef,
		TriggerType:   req.TriggerType,
		OffsetSeconds: req.OffsetSeconds,
		AfterSeconds:  req.AfterSeconds,
		RepeatPolicy:  req.RepeatPolicy,
		TaskFilter:    req.TaskFilter,
		AudienceType:  req.AudienceType,
		Recipients:    req.Recipients,
		SinkRef:       req.SinkRef,
		Enabled:       req.Enabled,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, view, nil)
}

func (s *Server) handleReminderRuleEnable(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, "reminder:write", app.PermissionReminderWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.EnableReminderRule(chi.URLParam(r, "ruleID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, view, nil)
}

func (s *Server) handleReminderRuleDisable(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, "reminder:write", app.PermissionReminderWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.DisableReminderRule(chi.URLParam(r, "ruleID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, view, nil)
}

func (s *Server) handleReminderRuleDelete(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, "reminder:write", app.PermissionReminderWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := scoped.DeleteReminderRule(chi.URLParam(r, "ruleID")); err != nil {
		writeAppError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleNotificationDeliveryList(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, "notification:read", app.PermissionNotificationRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	status := r.URL.Query().Get("status")
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			writeError(w, http.StatusBadRequest, "api_bad_limit", "invalid limit", nil)
			return
		}
		limit = parsed
	}
	offset := 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			writeError(w, http.StatusBadRequest, "api_bad_offset", "invalid offset", nil)
			return
		}
		offset = parsed
	}
	rows, err := scoped.ListNotificationDeliveries(status, limit, offset)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, rows, nil)
}

func (s *Server) handleNotificationDeliveryInfo(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, "notification:read", app.PermissionNotificationRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.NotificationDeliveryInfo(chi.URLParam(r, "deliveryID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, view, nil)
}

func (s *Server) handleNotificationDeliveryReplay(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, "notification:write", app.PermissionNotificationWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.ReplayNotificationDelivery(chi.URLParam(r, "deliveryID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, view, nil)
}

func notificationHeaderTemplatesToApp(rows []httpHeaderTemplateRequest) []app.HTTPHeaderTemplateInput {
	out := make([]app.HTTPHeaderTemplateInput, 0, len(rows))
	for _, row := range rows {
		out = append(out, app.HTTPHeaderTemplateInput{Name: row.Name, Value: row.Value})
	}
	return out
}

func notificationHeaderTemplatesPtrToApp(rows *[]httpHeaderTemplateRequest) *[]app.HTTPHeaderTemplateInput {
	if rows == nil {
		return nil
	}
	out := notificationHeaderTemplatesToApp(*rows)
	return &out
}

func notificationSecretRefsToApp(rows []httpTemplateSecretRefRequest) []app.HTTPTemplateSecretRefInput {
	out := make([]app.HTTPTemplateSecretRefInput, 0, len(rows))
	for _, row := range rows {
		out = append(out, app.HTTPTemplateSecretRefInput{Alias: row.Alias, ConfigKey: row.ConfigKey})
	}
	return out
}

func notificationSecretRefsPtrToApp(rows *[]httpTemplateSecretRefRequest) *[]app.HTTPTemplateSecretRefInput {
	if rows == nil {
		return nil
	}
	out := notificationSecretRefsToApp(*rows)
	return &out
}
