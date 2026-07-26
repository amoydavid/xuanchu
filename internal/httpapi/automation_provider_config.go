package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/auth"
)

// automationProviderConfigRequest 是安全 Provider config 写入 DTO。
// APIKey 省略/为空表示保留原值；ClearAPIKey=true 显式清除。
type automationProviderConfigRequest struct {
	BaseURL      string   `json:"base_url"`
	Model        string   `json:"model"`
	AllowedHosts []string `json:"allowed_hosts"`
	APIKey       string   `json:"api_key,omitempty"`
	ClearAPIKey  bool     `json:"clear_api_key,omitempty"`
}

// scopedAutomationProviderConfigService 校验 Provider config 双 scope。
// Workspace：config:read/write + workspace.read/modify；
// Project：复用现有 Project config token/role + project read/manage。
func (s *Server) scopedAutomationProviderConfigService(r *http.Request, projectRef string, write bool) (*app.Service, error) {
	if projectRef == "" {
		// Workspace scope。
		wsScope := auth.ScopeWorkspaceRead
		wsPermission := app.PermissionWorkspaceRead
		configPermission := app.PermissionConfigSchemaRead
		if write {
			wsScope = auth.ScopeWorkspaceWrite
			wsPermission = app.PermissionWorkspaceModify
			configPermission = app.PermissionConfigSchemaWrite
		}
		scoped, _, err := s.scopedService(r, wsScope, wsPermission, "")
		if err != nil {
			return nil, err
		}
		if _, _, err := s.scopedService(r, auth.ScopeConfigRead, configPermission, ""); err != nil {
			return nil, err
		}
		return scoped, nil
	}
	// Project scope：复用现有 Project config 权限链。
	scoped, err := s.scopedProjectAutomationService(r, projectRef, write)
	if err != nil {
		return nil, err
	}
	return scoped, nil
}

func (s *Server) handleWorkspaceAutomationProviderConfigGet(w http.ResponseWriter, r *http.Request) {
	scoped, err := s.scopedAutomationProviderConfigService(r, "", false)
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.WorkspaceAutomationProviderConfig().View()
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, view, nil)
}

func (s *Server) handleWorkspaceAutomationProviderConfigPut(w http.ResponseWriter, r *http.Request) {
	var req automationProviderConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	scoped, err := s.scopedAutomationProviderConfigService(r, "", true)
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.WorkspaceAutomationProviderConfig().Update(app.AutomationProviderConfigInput{
		BaseURL:      strings.TrimSpace(req.BaseURL),
		Model:        strings.TrimSpace(req.Model),
		AllowedHosts: req.AllowedHosts,
		APIKey:       req.APIKey,
		ClearAPIKey:  req.ClearAPIKey,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, view, nil)
}

// handleProjectAutomationProviderConfigGet/Put 暴露 Project scope 的安全 Provider facade，
// 浏览器不再通过通用 Project config list 读取 secret 原值。
func (s *Server) handleProjectAutomationProviderConfigGet(w http.ResponseWriter, r *http.Request) {
	projectRef := chi.URLParam(r, "projectRef")
	scoped, err := s.scopedAutomationProviderConfigService(r, projectRef, false)
	if err != nil {
		writeAppError(w, err)
		return
	}
	project, err := scoped.ResolveProject(projectRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.ProjectAutomationProviderConfig(project.ID).View()
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, view, nil)
}

func (s *Server) handleProjectAutomationProviderConfigPut(w http.ResponseWriter, r *http.Request) {
	projectRef := chi.URLParam(r, "projectRef")
	var req automationProviderConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	scoped, err := s.scopedAutomationProviderConfigService(r, projectRef, true)
	if err != nil {
		writeAppError(w, err)
		return
	}
	project, err := scoped.ResolveProject(projectRef)
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.ProjectAutomationProviderConfig(project.ID).Update(app.AutomationProviderConfigInput{
		BaseURL:      strings.TrimSpace(req.BaseURL),
		Model:        strings.TrimSpace(req.Model),
		AllowedHosts: req.AllowedHosts,
		APIKey:       req.APIKey,
		ClearAPIKey:  req.ClearAPIKey,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, view, nil)
}
