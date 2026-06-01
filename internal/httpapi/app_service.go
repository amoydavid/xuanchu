package httpapi

import (
	"net/http"
	"strings"

	"github.com/dajee/taskg/internal/app"
)

func (s *Server) scopedService(r *http.Request, capability string, permission app.Permission, projectRef string) (*app.Service, requestAuth, error) {
	return s.scopedServiceWithWorkspace(r, capability, permission, requestWorkspaceRef(r), projectRef)
}

func (s *Server) scopedServiceWithWorkspace(r *http.Request, capability string, permission app.Permission, workspaceRef, projectRef string) (*app.Service, requestAuth, error) {
	authn, ok := authFromContext(r.Context())
	if !ok {
		return nil, requestAuth{}, app.RuntimeError{Code: "api_internal", Message: "internal server error"}
	}
	baseSvc, err := app.NewService(app.ServiceOptions{
		Store:   s.store,
		Clock:   s.effectiveClock(),
		Runtime: &app.RuntimeContext{},
	})
	if err != nil {
		return nil, requestAuth{}, err
	}
	authorized, err := baseSvc.AuthorizeTokenRequest(app.RequestAuthorizationInput{
		Token:              authn.Authn,
		RequiredCapability: capability,
		RequiredPermission: permission,
		WorkspaceRef:       workspaceRef,
		ProjectRef:         strings.TrimSpace(projectRef),
	})
	if err != nil {
		return nil, requestAuth{}, err
	}
	scoped, err := app.NewService(app.ServiceOptions{
		Store:        s.store,
		Clock:        s.effectiveClock(),
		Runtime:      &authorized.Runtime,
		RequestScope: &authorized.Scope,
	})
	if err != nil {
		return nil, requestAuth{}, err
	}
	authn.EffectiveWorkspace = authorized.Workspace
	return scoped, authn, nil
}

func requestWorkspaceRef(r *http.Request) string {
	if value := strings.TrimSpace(r.URL.Query().Get("workspace")); value != "" {
		return value
	}
	return strings.TrimSpace(r.Header.Get("X-Taskg-Workspace"))
}

func requestProjectRef(r *http.Request) string {
	if value := strings.TrimSpace(r.URL.Query().Get("project_id")); value != "" {
		return value
	}
	return strings.TrimSpace(r.URL.Query().Get("project"))
}
