package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"github.com/go-chi/chi/v5"
)

// validateExternalBaseURL 校验 redirect_uri 基地址的 scheme 与 host，
// 防 OIDC redirect_uri 被指向恶意 origin 窃取授权码。
// 允许 https（生产）与 http+localhost/127.0.0.1（本地 dev）。
func validateExternalBaseURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "外部可达地址格式无效"
	}
	if u.Scheme == "https" {
		return ""
	}
	if u.Scheme == "http" {
		host := u.Hostname()
		if host == "localhost" || host == "127.0.0.1" || strings.HasPrefix(host, "127.") {
			return ""
		}
		return "http 外部地址仅允许 localhost"
	}
	return "外部可达地址必须是 http(s) 绝对 URL"
}

// handleWorkspaceSsoConfigGet 返回脱敏的 SSO 配置。
func (s *Server) handleWorkspaceSsoConfigGet(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "workspace")
	_, authn, err := s.scopedServiceWithWorkspace(r, auth.ScopeWorkspaceRead, app.PermissionSsoConfigRead, ref, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	cfgSvc := app.NewOIDCConfigService(storage.NewConfigRepository(s.store.DB()))
	cfg, enabled := cfgSvc.Get(authn.EffectiveWorkspace.ID)
	if !enabled {
		writeSuccess(w, http.StatusOK, map[string]any{"enabled": false}, nil)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]any{"enabled": true, "config": ssoConfigJSON(cfg)}, nil)
}

// handleWorkspaceSsoConfigSet 写入 SSO 配置（secret 加密落库）。
func (s *Server) handleWorkspaceSsoConfigSet(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "workspace")
	_, authn, err := s.scopedServiceWithWorkspace(r, auth.ScopeWorkspaceWrite, app.PermissionSsoConfigWrite, ref, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	var in struct {
		Provider             string `json:"provider"`
		IssuerBaseURL        string `json:"issuer_base_url"`
		OrgID                string `json:"org_id"`
		ClientID             string `json:"client_id"`
		ClientSecret         string `json:"client_secret"`
		DirectoryAccessToken string `json:"directory_access_token"`
		Scopes               string `json:"scopes"`
		RedirectPath         string `json:"redirect_path"`
		ExternalBaseURL      string `json:"external_base_url"`
		SessionTTL           string `json:"session_ttl"`
		SyncInterval         string `json:"sync_interval"`
		InsecureCookie       bool   `json:"insecure_cookie"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error(), nil)
		return
	}
	if in.IssuerBaseURL == "" || in.OrgID == "" || in.ClientID == "" {
		writeError(w, http.StatusBadRequest, "missing_required", "issuer_base_url, org_id, client_id 必填", nil)
		return
	}
	// external_base_url 非空时校验 scheme，防 redirect_uri 被指向恶意 origin 窃取授权码
	if in.ExternalBaseURL != "" {
		if msg := validateExternalBaseURL(in.ExternalBaseURL); msg != "" {
			writeError(w, http.StatusBadRequest, "invalid_external_base_url", msg, nil)
			return
		}
	}
	cfgSvc := app.NewOIDCConfigService(storage.NewConfigRepository(s.store.DB()))
	if err := cfgSvc.Set(authn.EffectiveWorkspace.ID, app.OIDCConfigInput{
		Provider:             in.Provider,
		IssuerBaseURL:        in.IssuerBaseURL,
		OrgID:                in.OrgID,
		ClientID:             in.ClientID,
		ClientSecret:         in.ClientSecret,
		DirectoryAccessToken: in.DirectoryAccessToken,
		Scopes:               in.Scopes,
		RedirectPath:         in.RedirectPath,
		ExternalBaseURL:      in.ExternalBaseURL,
		SessionTTL:           in.SessionTTL,
		SyncInterval:         in.SyncInterval,
		InsecureCookie:       in.InsecureCookie,
	}); err != nil {
		if errors.Is(err, app.ErrConfigSecretKeyMissing) {
			writeError(w, http.StatusBadRequest, "config_secret_key_missing", "服务端未配置 XUANCHU_CONFIG_SECRET_KEY，无法保存 secret", nil)
			return
		}
		if errors.Is(err, app.ErrConfigSecretKeyInvalid) {
			writeError(w, http.StatusInternalServerError, "config_secret_key_invalid", "服务端 secret key 格式错误", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "config_set_failed", err.Error(), nil)
		return
	}
	cfg, _ := cfgSvc.Get(authn.EffectiveWorkspace.ID)
	writeSuccess(w, http.StatusOK, map[string]any{"enabled": true, "config": ssoConfigJSON(cfg)}, nil)
}

// handleWorkspaceSsoSync 触发通讯录同步（插入 pending job）。
func (s *Server) handleWorkspaceSsoSync(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "workspace")
	_, authn, err := s.scopedServiceWithWorkspace(r, auth.ScopeWorkspaceWrite, app.PermissionSsoConfigWrite, ref, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	jobRepo := storage.NewDirectorySyncJobRepository(s.store.DB())
	job, err := jobRepo.Create(authn.EffectiveWorkspace.ID, time.Now().Unix())
	if err != nil {
		if errors.Is(err, storage.ErrSyncInProgress) {
			writeError(w, http.StatusConflict, "sync_in_progress", "已有同步任务进行中", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "sync_create_failed", err.Error(), nil)
		return
	}
	writeSuccess(w, http.StatusAccepted, map[string]any{"job_id": job.ID, "status": job.Status}, nil)
}

// handleWorkspaceSsoSyncJob 查询同步任务状态。
func (s *Server) handleWorkspaceSsoSyncJob(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "workspace")
	_, authn, err := s.scopedServiceWithWorkspace(r, auth.ScopeWorkspaceRead, app.PermissionSsoConfigRead, ref, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	jobID := chi.URLParam(r, "job_id")
	jobRepo := storage.NewDirectorySyncJobRepository(s.store.DB())
	job, err := jobRepo.Get(jobID)
	if err != nil || job.WorkspaceID != authn.EffectiveWorkspace.ID {
		writeError(w, http.StatusNotFound, "job_not_found", "同步任务不存在", nil)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]any{"job": job}, nil)
}

// ssoConfigJSON 把 OIDCConfig 转成 snake_case JSON 视图（前端消费）。
func ssoConfigJSON(cfg app.OIDCConfig) map[string]any {
	return map[string]any{
		"provider":                      cfg.Provider,
		"issuer_base_url":               cfg.IssuerBaseURL,
		"org_id":                        cfg.OrgID,
		"client_id":                     cfg.ClientID,
		"client_secret_masked":          cfg.ClientSecretMasked,
		"directory_access_token_masked": cfg.DirectoryAccessTokenMasked,
		"scopes":                        cfg.Scopes,
		"redirect_path":                 cfg.RedirectPath,
		"external_base_url":             cfg.ExternalBaseURL,
		"session_ttl":                   cfg.SessionTTL,
		"sync_interval":                 cfg.SyncInterval,
		"insecure_cookie":               cfg.InsecureCookie,
	}
}
