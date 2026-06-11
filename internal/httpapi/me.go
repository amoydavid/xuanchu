package httpapi

import (
	"net/http"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

type meResponse struct {
	Actor              userInfoResponse `json:"actor"`
	Token              tokenView        `json:"token"`
	VisibleWorkspaces  []workspaceView  `json:"visible_workspaces"`
	EffectiveWorkspace workspaceView    `json:"effective_workspace"`
}

type userInfoResponse struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Email       *string               `json:"email"`
	ExternalIDs []task.JSONExternalID `json:"external_ids"`
}

type tokenView struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Type   string   `json:"type"`
	Scopes []string `json:"scopes"`
}

type workspaceView struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	authn, ok := authFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusInternalServerError, "api_internal", "internal server error", nil)
		return
	}
	actor := task.UserInfo{
		ID:    authn.Authn.User.ID,
		Name:  authn.Authn.User.Name,
		Email: authn.Authn.User.Email,
	}
	svc, err := app.NewService(app.ServiceOptions{
		Store:                 s.store,
		Clock:                 s.effectiveClock(),
		DisableScopeBootstrap: true,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	userView, err := svc.UserInfo(authn.Authn.User.ID)
	if err != nil {
		writeAppError(w, err)
		return
	}
	actor = task.UserInfo{
		ID:          userView.ID,
		Name:        userView.Name,
		Email:       userView.Email,
		ExternalIDs: userView.ExternalIDs,
	}

	visible := make([]workspaceView, 0, len(authn.VisibleWorkspaces))
	for _, row := range authn.VisibleWorkspaces {
		visible = append(visible, workspaceView{ID: row.Workspace.ID, Slug: row.Workspace.Slug})
	}

	writeSuccess(w, http.StatusOK, meResponse{
		Actor: userInfoResponseFromInfo(actor),
		Token: tokenView{
			ID:     authn.Authn.Token.ID,
			Name:   authn.Authn.Token.Name,
			Type:   authn.Authn.Token.Type,
			Scopes: authn.Authn.Token.Scopes,
		},
		VisibleWorkspaces:  visible,
		EffectiveWorkspace: workspaceView{ID: authn.EffectiveWorkspace.ID, Slug: authn.EffectiveWorkspace.Slug},
	}, nil)
}

func userInfoResponseFromInfo(info task.UserInfo) userInfoResponse {
	jsonInfo := task.UserInfoToJSON(info)
	externalIDs := jsonInfo.ExternalIDs
	if externalIDs == nil {
		externalIDs = []task.JSONExternalID{}
	}
	return userInfoResponse{
		ID:          jsonInfo.ID,
		Name:        jsonInfo.Name,
		Email:       jsonInfo.Email,
		ExternalIDs: externalIDs,
	}
}
