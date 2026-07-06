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
	EffectiveRole      string           `json:"effective_role"`
}

type userInfoResponse struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	DisplayName string                `json:"display_name,omitempty"`
	Email       *string               `json:"email"`
	ExternalIDs []task.JSONExternalID `json:"external_ids"`
}

type tokenView struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Type   string   `json:"type"`
	Prefix string   `json:"prefix,omitempty"`
	Scopes []string `json:"scopes"`
}

type workspaceView struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name,omitempty"`
}

type credentialsCurrentResponse struct {
	ActorType          string              `json:"actor_type"`
	Actor              credentialActorView `json:"actor"`
	Token              tokenView           `json:"token"`
	VisibleWorkspaces  []workspaceView     `json:"visible_workspaces,omitempty"`
	EffectiveWorkspace workspaceView       `json:"effective_workspace"`
	EffectiveRole      string              `json:"effective_role"`
	Capabilities       []string            `json:"capabilities"`
}

type credentialActorView struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	DisplayName string                `json:"display_name,omitempty"`
	Email       *string               `json:"email,omitempty"`
	ExternalIDs []task.JSONExternalID `json:"external_ids,omitempty"`
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
	if authn.Authn.TenantActor {
		writeAppError(w, app.RuntimeError{Code: "tenant_actor_not_user", Message: "tenant token has no user actor"})
		return
	}
	actor := task.UserInfo{
		ID:          authn.Authn.User.ID,
		Name:        authn.Authn.User.Name,
		DisplayName: authn.Authn.User.DisplayName,
		Email:       authn.Authn.User.Email,
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
		DisplayName: userView.DisplayName,
		Email:       userView.Email,
		ExternalIDs: userView.ExternalIDs,
	}

	visible := make([]workspaceView, 0, len(authn.VisibleWorkspaces))
	for _, row := range authn.VisibleWorkspaces {
		visible = append(visible, workspaceView{ID: row.Workspace.ID, Slug: row.Workspace.Slug})
	}

	// 从可见 workspace 列表中提取 effective workspace 的角色，供前端判断管理能力。
	effectiveRole := ""
	for _, row := range authn.VisibleWorkspaces {
		if row.Workspace.ID == authn.EffectiveWorkspace.ID {
			effectiveRole = row.Role
			break
		}
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
		EffectiveRole:      effectiveRole,
	}, nil)
}

func (s *Server) handleCredentialsCurrent(w http.ResponseWriter, r *http.Request) {
	authn, ok := authFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusInternalServerError, "api_internal", "internal server error", nil)
		return
	}
	if authn.Authn.TenantActor {
		writeSuccess(w, http.StatusOK, credentialsCurrentResponse{
			ActorType: "tenant_access_token",
			Actor: credentialActorView{
				ID:          authn.Authn.Token.ID,
				Name:        authn.Authn.Token.Name,
				DisplayName: "系统身份 / " + authn.Authn.Token.Name,
			},
			Token: tokenView{
				ID:     authn.Authn.Token.ID,
				Name:   authn.Authn.Token.Name,
				Type:   authn.Authn.Token.Type,
				Prefix: authn.Authn.Token.Prefix,
				Scopes: authn.Authn.Token.Scopes,
			},
			VisibleWorkspaces: []workspaceView{{
				ID:   authn.EffectiveWorkspace.ID,
				Slug: authn.EffectiveWorkspace.Slug,
				Name: authn.EffectiveWorkspace.Name,
			}},
			EffectiveWorkspace: workspaceView{
				ID:   authn.EffectiveWorkspace.ID,
				Slug: authn.EffectiveWorkspace.Slug,
				Name: authn.EffectiveWorkspace.Name,
			},
			EffectiveRole: "owner",
			Capabilities:  append([]string(nil), authn.Authn.Token.Scopes...),
		}, nil)
		return
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
	// Web Console 登录入口：SSO browser session 创建的 PAT/Agent token 标记了 WebLoginDisabled，
	// 禁止用于 Console 登录（守住「SSO workspace 人工 token 走 SSO」边界）。
	// token 在 HTTP API / MCP / CLI 仍可用，只是不能进 Console 登录页。
	if authn.Authn.Token.WebLoginDisabled {
		writeError(w, http.StatusForbidden, "token_web_login_disabled", "该令牌不能用于 Web Console 登录，请使用 SSO 登录", nil)
		return
	}
	userView, err := svc.UserInfo(authn.Authn.User.ID)
	if err != nil {
		writeAppError(w, err)
		return
	}
	actor := task.UserInfo{
		ID:          userView.ID,
		Name:        userView.Name,
		DisplayName: userView.DisplayName,
		Email:       userView.Email,
		ExternalIDs: userView.ExternalIDs,
	}
	visible := make([]workspaceView, 0, len(authn.VisibleWorkspaces))
	effectiveRole := ""
	for _, row := range authn.VisibleWorkspaces {
		visible = append(visible, workspaceView{ID: row.Workspace.ID, Slug: row.Workspace.Slug, Name: row.Workspace.Name})
		if row.Workspace.ID == authn.EffectiveWorkspace.ID {
			effectiveRole = row.Role
		}
	}
	jsonActor := task.UserInfoToJSON(actor)
	writeSuccess(w, http.StatusOK, credentialsCurrentResponse{
		ActorType: "user",
		Actor: credentialActorView{
			ID:          jsonActor.ID,
			Name:        jsonActor.Name,
			DisplayName: jsonActor.DisplayName,
			Email:       jsonActor.Email,
			ExternalIDs: jsonActor.ExternalIDs,
		},
		Token: tokenView{
			ID:     authn.Authn.Token.ID,
			Name:   authn.Authn.Token.Name,
			Type:   authn.Authn.Token.Type,
			Prefix: authn.Authn.Token.Prefix,
			Scopes: authn.Authn.Token.Scopes,
		},
		VisibleWorkspaces:  visible,
		EffectiveWorkspace: workspaceView{ID: authn.EffectiveWorkspace.ID, Slug: authn.EffectiveWorkspace.Slug, Name: authn.EffectiveWorkspace.Name},
		EffectiveRole:      effectiveRole,
		Capabilities:       append([]string(nil), authn.Authn.Token.Scopes...),
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
		DisplayName: jsonInfo.DisplayName,
		Email:       jsonInfo.Email,
		ExternalIDs: externalIDs,
	}
}
