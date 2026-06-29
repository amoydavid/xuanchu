package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/auth"
)

type userRequest struct {
	Name        string  `json:"name"`
	DisplayName *string `json:"display_name,omitempty"`
	Email       *string `json:"email,omitempty"`
}

type userResponse struct {
	ID                 string               `json:"id"`
	Name               string               `json:"name"`
	DisplayName        string               `json:"display_name"`
	Email              *string              `json:"email,omitempty"`
	DefaultWorkspaceID *string              `json:"default_workspace_id,omitempty"`
	ExternalIDs        []externalIDResponse `json:"external_ids,omitempty"`
	Active             bool                 `json:"active"`
	CreatedAt          int64                `json:"created_at"`
	ModifiedAt         int64                `json:"modified_at"`
}

type externalIDResponse struct {
	Provider   string `json:"provider"`
	ExternalID string `json:"external_id"`
}

type bindExternalIDRequest struct {
	Provider   string `json:"provider"`
	ExternalID string `json:"external_id"`
}

func (s *Server) handleUserList(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, auth.ScopeWorkspaceRead, app.PermissionWorkspaceRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	users, err := scoped.ListUsers()
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, userResponsesFromViews(users), nil)
}

func (s *Server) handleUserCreate(w http.ResponseWriter, r *http.Request) {
	var req userRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	scoped, _, err := s.scopedService(r, auth.ScopeWorkspaceWrite, app.PermissionWorkspaceModify, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	input := app.AddUserInput{Name: req.Name}
	if req.DisplayName != nil {
		input.DisplayName = *req.DisplayName
	}
	if req.Email != nil {
		input.Email = *req.Email
	}
	user, err := scoped.AddUser(input)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, userResponseFromView(user), nil)
}

func (s *Server) handleUserModify(w http.ResponseWriter, r *http.Request) {
	var req userRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	ref := chi.URLParam(r, "user")
	scoped, _, err := s.scopedService(r, auth.ScopeWorkspaceWrite, app.PermissionWorkspaceModify, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	user, err := scoped.ModifyUser(ref, app.ModifyUserInput{
		DisplayName: req.DisplayName,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, userResponseFromView(user), nil)
}

func (s *Server) handleUserInfo(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "user")
	scoped, _, err := s.scopedService(r, auth.ScopeWorkspaceRead, app.PermissionWorkspaceRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	user, err := scoped.UserInfo(ref)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, userResponseFromView(user), nil)
}

func userResponsesFromViews(users []app.UserView) []userResponse {
	out := make([]userResponse, 0, len(users))
	for _, user := range users {
		out = append(out, userResponseFromView(user))
	}
	return out
}

func userResponseFromView(user app.UserView) userResponse {
	extIDs := make([]externalIDResponse, 0, len(user.ExternalIDs))
	for _, eid := range user.ExternalIDs {
		extIDs = append(extIDs, externalIDResponse{Provider: eid.Provider, ExternalID: eid.ExternalID})
	}
	return userResponse{
		ID:                 user.ID,
		Name:               user.Name,
		DisplayName:        user.DisplayName,
		Email:              user.Email,
		DefaultWorkspaceID: user.DefaultWorkspaceID,
		ExternalIDs:        extIDs,
		Active:             user.Active,
		CreatedAt:          user.CreatedAt,
		ModifiedAt:         user.ModifiedAt,
	}
}

func (s *Server) handleExternalIDBind(w http.ResponseWriter, r *http.Request) {
	userRef := chi.URLParam(r, "user")
	var req bindExternalIDRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	if req.Provider == "" || req.ExternalID == "" {
		writeError(w, http.StatusBadRequest, "invalid_input", "provider and external_id are required", nil)
		return
	}
	scoped, _, err := s.scopedService(r, auth.ScopeWorkspaceWrite, app.PermissionWorkspaceModify, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	user, err := scoped.UserInfo(userRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := scoped.BindExternalID(user.ID, req.Provider, req.ExternalID); err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, externalIDResponse{Provider: req.Provider, ExternalID: req.ExternalID}, nil)
}

func (s *Server) handleExternalIDUnbind(w http.ResponseWriter, r *http.Request) {
	userRef := chi.URLParam(r, "user")
	provider := chi.URLParam(r, "provider")
	externalID := chi.URLParam(r, "externalID")
	scoped, _, err := s.scopedService(r, auth.ScopeWorkspaceWrite, app.PermissionWorkspaceModify, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	user, err := scoped.UserInfo(userRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := scoped.UnbindExternalID(user.ID, provider, externalID); err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusNoContent, nil, nil)
}

func (s *Server) handleExternalIDList(w http.ResponseWriter, r *http.Request) {
	userRef := chi.URLParam(r, "user")
	scoped, _, err := s.scopedService(r, auth.ScopeWorkspaceRead, app.PermissionWorkspaceRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	user, err := scoped.UserInfo(userRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	extIDs, err := scoped.ListExternalIDs(user.ID)
	if err != nil {
		writeAppError(w, err)
		return
	}
	out := make([]externalIDResponse, len(extIDs))
	for i, eid := range extIDs {
		out[i] = externalIDResponse{Provider: eid.Provider, ExternalID: eid.ExternalID}
	}
	writeSuccess(w, http.StatusOK, out, nil)
}
