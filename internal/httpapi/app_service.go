package httpapi

import (
	"net/http"
	"strings"

	"github.com/dajee/taskg/internal/app"
)

func (s *Server) scopedService(r *http.Request, capability string, permission app.Permission, projectRef string) (*app.Service, requestAuth, error) {
	return s.scopedServiceFor(r, scopedServiceInput{
		Capability:     capability,
		Permission:     permission,
		WorkspaceRef:   requestWorkspaceRef(r),
		ProjectRef:     projectRef,
		ProjectRefIsID: projectRefIsID(r, projectRef),
	})
}

func (s *Server) scopedServiceWithWorkspace(r *http.Request, capability string, permission app.Permission, workspaceRef, projectRef string) (*app.Service, requestAuth, error) {
	return s.scopedServiceFor(r, scopedServiceInput{
		Capability:   capability,
		Permission:   permission,
		WorkspaceRef: workspaceRef,
		ProjectRef:   projectRef,
	})
}

type scopedServiceInput struct {
	Capability     string
	Permission     app.Permission
	WorkspaceRef   string
	ProjectRef     string
	ProjectRefIsID bool
}

func (s *Server) scopedServiceFor(r *http.Request, input scopedServiceInput) (*app.Service, requestAuth, error) {
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
		RequiredCapability: input.Capability,
		RequiredPermission: input.Permission,
		WorkspaceRef:       input.WorkspaceRef,
		ProjectRef:         strings.TrimSpace(input.ProjectRef),
		ProjectRefIsID:     input.ProjectRefIsID,
		SubjectUserRef:     strings.TrimSpace(r.Header.Get("X-Taskg-As")),
	})
	if err != nil {
		if state, ok := r.Context().Value(logStateContextKey).(*requestLogState); ok && strings.TrimSpace(r.Header.Get("X-Taskg-As")) != "" {
			state.impersonateAttempt = strings.TrimSpace(r.Header.Get("X-Taskg-As"))
		}
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
	if state, ok := r.Context().Value(logStateContextKey).(*requestLogState); ok && authorized.Runtime.DelegatorTokenID != "" {
		state.delegatorUserID = authorized.Runtime.DelegatorUserID
		state.delegatorTokenID = authorized.Runtime.DelegatorTokenID
	}
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

func projectRefIsID(r *http.Request, ref string) bool {
	return strings.TrimSpace(ref) != "" && strings.TrimSpace(r.URL.Query().Get("project_id")) == strings.TrimSpace(ref)
}
