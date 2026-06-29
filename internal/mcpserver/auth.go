package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/authz"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

// RequestScopeInput 是构造 scoped service 的输入参数。
type RequestScopeInput struct {
	Workspace string
	Project   string
	ProjectID string
}

// RuntimeFactory 为 MCP server 提供 scoped app.Service 构造能力。
type RuntimeFactory struct {
	Store *storage.Store
	Clock app.Clock
}

// ServiceForStdio 为 stdio 模式构造 scoped service。
// 从 SQLite 读取 active user/workspace（与 CLI 相同的 local 模式）。
func (f RuntimeFactory) ServiceForStdio(ctx context.Context, input RequestScopeInput, capability string, permission app.Permission) (*app.Service, error) {
	if f.Store == nil {
		return nil, fmt.Errorf("mcp store not configured")
	}
	svc, err := app.NewService(app.ServiceOptions{
		Store:        f.Store,
		Clock:        f.Clock,
		WorkspaceRef: strings.TrimSpace(input.Workspace),
	})
	if err != nil {
		return nil, err
	}
	if err := svc.Require(permission); err != nil {
		return nil, err
	}
	if err := ensureProjectRefsMatch(svc, input.Project, input.ProjectID); err != nil {
		return nil, err
	}
	return svc, nil
}

// ServiceForHTTP 为 HTTP 模式构造 scoped service。
// 如果请求中有 Bearer token 但 context 中没有 auth 信息，
// 会先进行 token 鉴权再构造 scoped service。
func (f RuntimeFactory) ServiceForHTTP(r *http.Request, input RequestScopeInput, capability string, permission app.Permission) (*app.Service, error) {
	if f.Store == nil {
		return nil, fmt.Errorf("mcp store not configured")
	}
	authn, ok := authFromHTTPRequest(r)
	if !ok {
		// 尝试从 Authorization header 鉴权
		if raw, headerOK := bearerTokenFromHeader(r.Header.Get("Authorization")); headerOK {
			svc, err := app.NewService(app.ServiceOptions{
				Store:                 f.Store,
				Clock:                 f.Clock,
				Runtime:               &app.RuntimeContext{},
				DisableScopeBootstrap: true,
			})
			if err != nil {
				return nil, err
			}
			authn, err = svc.AuthenticateBearerToken(raw)
			if err != nil {
				return nil, err
			}
			ok = true
		}
	}
	if !ok {
		return nil, app.RuntimeError{Code: authz.CodeAuthMissingToken, Message: "missing bearer token"}
	}
	workspaceRef := strings.TrimSpace(input.Workspace)
	if workspaceRef == "" {
		workspaceRef = workspaceRefFromHTTPRequest(r)
	}
	projectRef := projectRef(input)
	if workspaceRef == "" && strings.TrimSpace(input.Project) != "" {
		if visible, err := visibleWorkspacesForToken(f.Store, authn); err != nil {
			return nil, err
		} else if len(visible) > 1 {
			return nil, app.RuntimeError{Code: authz.CodeWorkspaceRequired, Message: "workspace is required to resolve project slug"}
		}
	}

	baseSvc, err := app.NewService(app.ServiceOptions{
		Store:                 f.Store,
		Clock:                 f.Clock,
		Runtime:               &app.RuntimeContext{},
		DisableScopeBootstrap: true,
	})
	if err != nil {
		return nil, err
	}
	authorized, err := baseSvc.AuthorizeTokenRequest(app.RequestAuthorizationInput{
		Token:              authn,
		RequiredCapability: capability,
		RequiredPermission: permission,
		WorkspaceRef:       workspaceRef,
		ProjectRef:         projectRef,
		ProjectRefIsID:     strings.TrimSpace(input.ProjectID) != "",
		SubjectUserRef:     strings.TrimSpace(r.Header.Get("X-Xuanchu-As")),
	})
	if err != nil {
		return nil, err
	}
	scoped, err := app.NewService(app.ServiceOptions{
		Store:        f.Store,
		Clock:        f.Clock,
		Runtime:      &authorized.Runtime,
		RequestScope: &authorized.Decision.RequestScope,
	})
	if err != nil {
		return nil, err
	}
	if err := ensureProjectRefsMatch(scoped, input.Project, input.ProjectID); err != nil {
		return nil, err
	}
	return scoped, nil
}

func projectRef(input RequestScopeInput) string {
	if value := strings.TrimSpace(input.ProjectID); value != "" {
		return value
	}
	return strings.TrimSpace(input.Project)
}

func ensureProjectRefsMatch(svc *app.Service, projectSlug, projectID string) error {
	projectSlug = strings.TrimSpace(projectSlug)
	projectID = strings.TrimSpace(projectID)
	if projectSlug == "" || projectID == "" {
		return nil
	}
	byID, err := svc.ProjectInfo(projectID)
	if err != nil {
		return err
	}
	bySlug, err := svc.ProjectInfo(projectSlug)
	if err != nil {
		return err
	}
	if bySlug.ID != byID.ID {
		return app.RuntimeError{Code: "project_mismatch", Message: "project and project_id do not match"}
	}
	return nil
}

func visibleWorkspacesForToken(store *storage.Store, authn app.AuthenticatedToken) ([]storage.WorkspaceWithRole, error) {
	if authn.TenantActor || authn.Token.Type == "tenant_access_token" {
		if len(authn.Token.WorkspaceIDs) != 1 {
			return nil, app.RuntimeError{Code: authz.CodeWorkspaceScopeDenied, Message: "workspace scope denied"}
		}
		workspace, err := storage.NewWorkspaceRepository(store.DB()).GetByID(authn.Token.WorkspaceIDs[0])
		if err != nil {
			return nil, err
		}
		return []storage.WorkspaceWithRole{{Workspace: workspace}}, nil
	}
	rows, err := storage.NewWorkspaceRepository(store.DB()).ListVisibleForUser(authn.User.ID, false)
	if err != nil {
		return nil, err
	}
	if len(authn.Token.WorkspaceIDs) == 0 {
		return rows, nil
	}
	out := make([]storage.WorkspaceWithRole, 0, len(rows))
	for _, row := range rows {
		if slices.Contains(authn.Token.WorkspaceIDs, row.Workspace.ID) {
			out = append(out, row)
		}
	}
	return out, nil
}

// httpAuthContextKey 用于在 http.Request context 中存储 auth 信息。
type httpAuthContextKey string

const mcpAuthKey httpAuthContextKey = "mcp.auth"

// HTTPAuthContext 是存入 http.Request context 的 auth 信息。
type HTTPAuthContext struct {
	Authn app.AuthenticatedToken
}

// SetHTTPAuthContext 将 auth 信息存入 http.Request context。
func SetHTTPAuthContext(r *http.Request, authn app.AuthenticatedToken) *http.Request {
	ctx := context.WithValue(r.Context(), mcpAuthKey, HTTPAuthContext{Authn: authn})
	return r.WithContext(ctx)
}

// authFromHTTPRequest 从 http.Request context 读取 auth 信息。
func authFromHTTPRequest(r *http.Request) (app.AuthenticatedToken, bool) {
	val := r.Context().Value(mcpAuthKey)
	if val == nil {
		return app.AuthenticatedToken{}, false
	}
	hac, ok := val.(HTTPAuthContext)
	if !ok {
		return app.AuthenticatedToken{}, false
	}
	return hac.Authn, true
}

// workspaceRefFromHTTPRequest 从请求头或 query 参数读取 workspace ref。
func workspaceRefFromHTTPRequest(r *http.Request) string {
	if value := strings.TrimSpace(r.URL.Query().Get("workspace")); value != "" {
		return value
	}
	return strings.TrimSpace(r.Header.Get("X-Xuanchu-Workspace"))
}

// AuthenticateHTTPRequest 对 HTTP 请求进行 Bearer token 鉴权。
// 返回更新了 auth context 的 request 和 error。
func (f RuntimeFactory) AuthenticateHTTPRequest(r *http.Request) (*http.Request, error) {
	if _, ok := authFromHTTPRequest(r); ok {
		return r, nil
	}
	raw, ok := bearerTokenFromHeader(r.Header.Get("Authorization"))
	if !ok {
		return nil, app.RuntimeError{Code: authz.CodeAuthMissingToken, Message: "missing bearer token"}
	}
	svc, err := app.NewService(app.ServiceOptions{
		Store:                 f.Store,
		Clock:                 f.Clock,
		Runtime:               &app.RuntimeContext{},
		DisableScopeBootstrap: true,
	})
	if err != nil {
		return nil, err
	}
	authn, err := svc.AuthenticateBearerToken(raw)
	if err != nil {
		return nil, err
	}
	return SetHTTPAuthContext(r, authn), nil
}

func bearerTokenFromHeader(header string) (string, bool) {
	header = strings.TrimSpace(header)
	if header == "" {
		return "", false
	}
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	if parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

// IsRuntimeError 检查 error 是否为 app.RuntimeError。
func IsRuntimeError(err error) bool {
	var runtimeErr app.RuntimeError
	return errors.As(err, &runtimeErr)
}
