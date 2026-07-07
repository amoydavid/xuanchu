package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/auth"
)

type contextRequest struct {
	Name   string `json:"name"`
	Filter string `json:"filter"`
}

type contextResponse struct {
	Name       string `json:"name"`
	Filter     string `json:"filter"`
	Active     bool   `json:"active"`
	CreatedAt  int64  `json:"created_at"`
	ModifiedAt int64  `json:"modified_at"`
}

type configSchemaRequest struct {
	ValueType         string   `json:"value_type"`
	AllowedScopes     []string `json:"allowed_scopes"`
	Label             string   `json:"label"`
	Description       string   `json:"description"`
	EnumValues        []string `json:"enum_values"`
	DefaultValue      *string  `json:"default_value"`
	Required          bool     `json:"required"`
	Secret            bool     `json:"secret"`
	ShowOnConsoleHome bool     `json:"show_on_console_home"`
}

func (s *Server) handleContextList(w http.ResponseWriter, r *http.Request) {
	scoped, authn, err := s.scopedService(r, auth.ScopeContextRead, app.PermissionContextUse, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	rows, err := scoped.ContextList()
	if err != nil {
		writeAppError(w, err)
		return
	}
	activeName := ""
	if !authn.Authn.TenantActor {
		var err error
		activeName, _, err = scoped.ActiveContextName()
		if err != nil {
			writeAppError(w, err)
			return
		}
	}
	out := make([]contextResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, contextResponse{
			Name:       row.Name,
			Filter:     row.FilterSource,
			Active:     row.Name == activeName,
			CreatedAt:  row.CreatedAt,
			ModifiedAt: row.ModifiedAt,
		})
	}
	writeSuccess(w, http.StatusOK, out, nil)
}

func (s *Server) handleContextDefine(w http.ResponseWriter, r *http.Request) {
	var req contextRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	scoped, _, err := s.scopedService(r, auth.ScopeContextWrite, app.PermissionContextManage, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := scoped.DefineContext(req.Name, req.Filter); err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, map[string]string{"name": req.Name, "filter": req.Filter}, nil)
}

func (s *Server) handleContextInfo(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	scoped, authn, err := s.scopedService(r, auth.ScopeContextRead, app.PermissionContextUse, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	rows, err := scoped.ContextList()
	if err != nil {
		writeAppError(w, err)
		return
	}
	for _, row := range rows {
		if row.Name == name {
			activeName := ""
			if !authn.Authn.TenantActor {
				var err error
				activeName, _, err = scoped.ActiveContextName()
				if err != nil {
					writeAppError(w, err)
					return
				}
			}
			writeSuccess(w, http.StatusOK, contextResponse{
				Name:       row.Name,
				Filter:     row.FilterSource,
				Active:     row.Name == activeName,
				CreatedAt:  row.CreatedAt,
				ModifiedAt: row.ModifiedAt,
			}, nil)
			return
		}
	}
	writeError(w, http.StatusNotFound, "context_not_found", "context not found", nil)
}

func (s *Server) handleContextDelete(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, auth.ScopeContextWrite, app.PermissionContextManage, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := scoped.ContextDelete(chi.URLParam(r, "name")); err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]bool{"ok": true}, nil)
}

func (s *Server) handleContextUse(w http.ResponseWriter, r *http.Request) {
	scoped, authn, err := s.scopedService(r, auth.ScopeContextWrite, app.PermissionContextUse, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := rejectTenantActor(authn); err != nil {
		writeAppError(w, err)
		return
	}
	if err := scoped.UseContext(chi.URLParam(r, "name")); err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]bool{"ok": true}, nil)
}

func (s *Server) handleContextNone(w http.ResponseWriter, r *http.Request) {
	scoped, authn, err := s.scopedService(r, auth.ScopeContextWrite, app.PermissionContextUse, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := rejectTenantActor(authn); err != nil {
		writeAppError(w, err)
		return
	}
	if err := scoped.ContextNone(); err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]bool{"ok": true}, nil)
}

func (s *Server) handleConfigList(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, auth.ScopeConfigRead, app.PermissionWorkspaceRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	values, err := scoped.ConfigValues()
	if err != nil {
		writeAppError(w, err)
		return
	}
	filtered := make(map[string]string, len(values))
	for key, value := range values {
		if isHTTPBusinessConfigKey(key) {
			filtered[key] = value
		}
	}
	writeSuccess(w, http.StatusOK, filtered, nil)
}

func (s *Server) handleConfigGet(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if !isHTTPBusinessConfigKey(key) {
		writeError(w, http.StatusBadRequest, "config_scope_invalid", "local config is not available over HTTP", nil)
		return
	}
	scoped, _, err := s.scopedService(r, auth.ScopeConfigRead, app.PermissionWorkspaceRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	value, ok, err := scoped.GetConfig(key)
	if err != nil {
		writeAppError(w, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "config_not_found", "config not found", nil)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]string{"value": value}, nil)
}

func (s *Server) handleConfigSet(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if !isHTTPBusinessConfigKey(key) {
		writeError(w, http.StatusBadRequest, "config_scope_invalid", "local config is not writable over HTTP", nil)
		return
	}
	var req configValueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	scoped, _, err := s.scopedService(r, auth.ScopeConfigWrite, app.PermissionWorkspaceModify, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := scoped.SetConfig(key, req.Value); err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]string{"value": req.Value}, nil)
}

func (s *Server) handleConfigUnset(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if !isHTTPBusinessConfigKey(key) {
		writeError(w, http.StatusBadRequest, "config_scope_invalid", "local config is not writable over HTTP", nil)
		return
	}
	scoped, _, err := s.scopedService(r, auth.ScopeConfigWrite, app.PermissionWorkspaceModify, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := scoped.UnsetConfig(key); err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]bool{"ok": true}, nil)
}

func (s *Server) handleConfigSchemaList(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, auth.ScopeConfigRead, app.PermissionConfigSchemaRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	rows, err := scoped.ConfigSchemaList()
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, rows, nil)
}

func (s *Server) handleConfigSchemaGet(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, auth.ScopeConfigRead, app.PermissionConfigSchemaRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	row, ok, err := scoped.ConfigSchemaGet(chi.URLParam(r, "key"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "config_definition_not_found", "config definition not found", nil)
		return
	}
	writeSuccess(w, http.StatusOK, row, nil)
}

func (s *Server) handleConfigSchemaSet(w http.ResponseWriter, r *http.Request) {
	var req configSchemaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	scoped, _, err := s.scopedService(r, auth.ScopeConfigWrite, app.PermissionConfigSchemaWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	input := app.ConfigSchemaInput{
		Key:               chi.URLParam(r, "key"),
		ValueType:         req.ValueType,
		AllowedScopes:     req.AllowedScopes,
		Label:             req.Label,
		Description:       req.Description,
		EnumValues:        req.EnumValues,
		DefaultValue:      req.DefaultValue,
		Required:          req.Required,
		Secret:            req.Secret,
		ShowOnConsoleHome: req.ShowOnConsoleHome,
	}
	if err := scoped.ConfigSchemaSet(input); err != nil {
		writeAppError(w, err)
		return
	}
	row, ok, err := scoped.ConfigSchemaGet(input.Key)
	if err != nil {
		writeAppError(w, err)
		return
	}
	if !ok {
		writeError(w, http.StatusInternalServerError, "api_internal", "config definition missing after write", nil)
		return
	}
	writeSuccess(w, http.StatusOK, row, nil)
}

func (s *Server) handleConfigSchemaDelete(w http.ResponseWriter, r *http.Request) {
	purge := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("purge")), "true")
	scoped, _, err := s.scopedService(r, auth.ScopeConfigWrite, app.PermissionConfigSchemaWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := scoped.ConfigSchemaDelete(chi.URLParam(r, "key"), purge); err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]any{"ok": true, "purge": purge}, nil)
}

func isHTTPBusinessConfigKey(key string) bool {
	return app.IsBusinessConfigKey(key)
}

const consoleHomeSecretMask = "••••••"

func (s *Server) handleConfigSchemaUsage(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, auth.ScopeConfigRead, app.PermissionConfigSchemaRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	usage, err := scoped.ConfigSchemaUsage(chi.URLParam(r, "key"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, usage, nil)
}

func (s *Server) handleProjectConfigEffective(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, auth.ScopeConfigRead, app.PermissionProjectConfigRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	rows, err := scoped.ProjectConfigEffectiveValues(chi.URLParam(r, "projectRef"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, rows, nil)
}

func (s *Server) handleConfigEffective(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, auth.ScopeConfigRead, app.PermissionConfigSchemaRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	consoleHome := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("console_home")), "true")
	rows, err := scoped.WorkspaceConfigEffectiveValues(app.ConfigEffectiveFilter{ConsoleHomeOnly: consoleHome})
	if err != nil {
		writeAppError(w, err)
		return
	}
	// 首页 effective 视图必须遮掩 secret 值，不提供 reveal。
	if consoleHome {
		for i := range rows {
			if !rows[i].Definition.Secret {
				continue
			}
			masked := consoleHomeSecretMask
			rows[i].Value = &masked
			rows[i].ProjectValue = nil
			rows[i].WorkspaceValue = nil
			rows[i].DefaultValue = nil
		}
	}
	writeSuccess(w, http.StatusOK, rows, nil)
}
