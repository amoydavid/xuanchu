package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

type hookCreateRequest struct {
	Name           string   `json:"name"`
	ScopeType      string   `json:"scope_type"`
	ProjectRef     string   `json:"project_ref,omitempty"`
	EventTypes     []string `json:"event_types"`
	EndpointURL    string   `json:"endpoint_url"`
	Secret         string   `json:"secret,omitempty"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty"`
	MaxAttempts    int      `json:"max_attempts,omitempty"`
}

type hookModifyRequest struct {
	Name           *string   `json:"name,omitempty"`
	EventTypes     *[]string `json:"event_types,omitempty"`
	EndpointURL    *string   `json:"endpoint_url,omitempty"`
	Secret         *string   `json:"secret,omitempty"`
	TimeoutSeconds *int      `json:"timeout_seconds,omitempty"`
	MaxAttempts    *int      `json:"max_attempts,omitempty"`
}

type hookResponse struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	ScopeType      string   `json:"scope_type"`
	WorkspaceID    string   `json:"workspace_id"`
	ProjectID      *string  `json:"project_id,omitempty"`
	EventTypes     []string `json:"event_types"`
	EndpointURL    string   `json:"endpoint_url"`
	Enabled        bool     `json:"enabled"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	MaxAttempts    int      `json:"max_attempts"`
	CreatedAt      int64    `json:"created_at"`
	ModifiedAt     int64    `json:"modified_at"`
}

type hookDeliveryResponse struct {
	ID             string            `json:"id"`
	HookID         string            `json:"hook_id"`
	EventID        string            `json:"event_id"`
	EventType      string            `json:"event_type"`
	WorkspaceID    string            `json:"workspace_id"`
	ProjectID      *string           `json:"project_id,omitempty"`
	Actor          task.JSONUserInfo `json:"actor"`
	Payload        map[string]any    `json:"payload"`
	Headers        map[string]string `json:"headers"`
	Status         string            `json:"status"`
	AttemptCount   int               `json:"attempt_count"`
	NextAttemptAt  *int64            `json:"next_attempt_at,omitempty"`
	ClaimExpiresAt *int64            `json:"claim_expires_at,omitempty"`
	LastAttemptAt  *int64            `json:"last_attempt_at,omitempty"`
	LastStatusCode *int              `json:"last_status_code,omitempty"`
	LastError      string            `json:"last_error,omitempty"`
	CreatedAt      int64             `json:"created_at"`
	ModifiedAt     int64             `json:"modified_at"`
}

func (s *Server) handleHookList(w http.ResponseWriter, r *http.Request) {
	projectRef := requestProjectRef(r)
	scoped, _, err := s.scopedService(r, "hook:read", app.PermissionHookRead, projectRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	rows, err := scoped.ListHooks(projectRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	out := make([]hookResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, hookResponseFromView(row))
	}
	writeSuccess(w, http.StatusOK, out, nil)
}

func (s *Server) handleHookCreate(w http.ResponseWriter, r *http.Request) {
	var req hookCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	projectRef := req.ProjectRef
	scoped, _, err := s.scopedService(r, "hook:write", app.PermissionHookWrite, projectRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	created, err := scoped.AddHook(app.HookAddInput{
		Name:           req.Name,
		ScopeType:      app.HookScopeType(req.ScopeType),
		ProjectRef:     req.ProjectRef,
		EventTypes:     req.EventTypes,
		EndpointURL:    req.EndpointURL,
		Secret:         req.Secret,
		TimeoutSeconds: req.TimeoutSeconds,
		MaxAttempts:    req.MaxAttempts,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, hookResponseFromView(created), nil)
}

func (s *Server) handleHookInfo(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, "hook:read", app.PermissionHookRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	hookID := chi.URLParam(r, "hookID")
	hook, err := scoped.HookInfo(hookID)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, hookResponseFromView(hook), nil)
}

func (s *Server) handleHookModify(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, "hook:write", app.PermissionHookWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	var req hookModifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	hookID := chi.URLParam(r, "hookID")
	updated, err := scoped.ModifyHook(hookID, app.HookModifyInput{
		Name:           req.Name,
		EventTypes:     req.EventTypes,
		EndpointURL:    req.EndpointURL,
		Secret:         req.Secret,
		TimeoutSeconds: req.TimeoutSeconds,
		MaxAttempts:    req.MaxAttempts,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, hookResponseFromView(updated), nil)
}

func (s *Server) handleHookDelete(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, "hook:write", app.PermissionHookWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	hookID := chi.URLParam(r, "hookID")
	if err := scoped.DeleteHook(hookID); err != nil {
		writeAppError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleHookEnable(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, "hook:write", app.PermissionHookWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	hookID := chi.URLParam(r, "hookID")
	updated, err := scoped.EnableHook(hookID)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, hookResponseFromView(updated), nil)
}

func (s *Server) handleHookDisable(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, "hook:write", app.PermissionHookWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	hookID := chi.URLParam(r, "hookID")
	updated, err := scoped.DisableHook(hookID)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, hookResponseFromView(updated), nil)
}

func (s *Server) handleHookDeliveryList(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, "hook:read", app.PermissionHookRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	hookID := chi.URLParam(r, "hookID")
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
	rows, err := scoped.ListHookDeliveries(hookID, status, limit, 0)
	if err != nil {
		writeAppError(w, err)
		return
	}
	out := make([]hookDeliveryResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, hookDeliveryResponseFromView(row))
	}
	writeSuccess(w, http.StatusOK, out, nil)
}

func (s *Server) handleHookDeliveryInfo(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, "hook:read", app.PermissionHookRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	deliveryID := chi.URLParam(r, "deliveryID")
	delivery, err := scoped.HookDeliveryInfo(deliveryID)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, hookDeliveryResponseFromView(delivery), nil)
}

func (s *Server) handleHookDeliveryReplay(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, "hook:write", app.PermissionHookWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	deliveryID := chi.URLParam(r, "deliveryID")
	replayed, err := scoped.ReplayHookDelivery(deliveryID)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, hookDeliveryResponseFromView(replayed), nil)
}

func hookResponseFromView(view app.HookView) hookResponse {
	return hookResponse{
		ID:             view.ID,
		Name:           view.Name,
		ScopeType:      view.ScopeType,
		WorkspaceID:    view.WorkspaceID,
		ProjectID:      view.ProjectID,
		EventTypes:     view.EventTypes,
		EndpointURL:    view.EndpointURL,
		Enabled:        view.Enabled,
		TimeoutSeconds: view.TimeoutSeconds,
		MaxAttempts:    view.MaxAttempts,
		CreatedAt:      view.CreatedAt,
		ModifiedAt:     view.ModifiedAt,
	}
}

func hookDeliveryResponseFromView(view app.HookDeliveryView) hookDeliveryResponse {
	return hookDeliveryResponse{
		ID:             view.ID,
		HookID:         view.HookID,
		EventID:        view.EventID,
		EventType:      view.EventType,
		WorkspaceID:    view.WorkspaceID,
		ProjectID:      view.ProjectID,
		Actor:          task.UserInfoToJSON(view.Actor),
		Payload:        view.Payload,
		Headers:        view.Headers,
		Status:         view.Status,
		AttemptCount:   view.AttemptCount,
		NextAttemptAt:  view.NextAttemptAt,
		ClaimExpiresAt: view.ClaimExpiresAt,
		LastAttemptAt:  view.LastAttemptAt,
		LastStatusCode: view.LastStatusCode,
		LastError:      view.LastError,
		CreatedAt:      view.CreatedAt,
		ModifiedAt:     view.ModifiedAt,
	}
}
