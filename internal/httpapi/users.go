package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/dajee/taskg/internal/app"
)

type userRequest struct {
	Name  string  `json:"name"`
	Email *string `json:"email,omitempty"`
}

type userResponse struct {
	ID                 string  `json:"id"`
	Name               string  `json:"name"`
	Email              *string `json:"email,omitempty"`
	DefaultWorkspaceID *string `json:"default_workspace_id,omitempty"`
	Active             bool    `json:"active"`
	CreatedAt          int64   `json:"created_at"`
	ModifiedAt         int64   `json:"modified_at"`
}

func (s *Server) handleUserList(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, "workspace:read", app.PermissionWorkspaceRead, "")
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
	scoped, _, err := s.scopedService(r, "workspace:write", app.PermissionWorkspaceModify, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	input := app.AddUserInput{Name: req.Name}
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

func (s *Server) handleUserInfo(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "user")
	scoped, _, err := s.scopedService(r, "workspace:read", app.PermissionWorkspaceRead, "")
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
	return userResponse{
		ID:                 user.ID,
		Name:               user.Name,
		Email:              user.Email,
		DefaultWorkspaceID: user.DefaultWorkspaceID,
		Active:             user.Active,
		CreatedAt:          user.CreatedAt,
		ModifiedAt:         user.ModifiedAt,
	}
}
