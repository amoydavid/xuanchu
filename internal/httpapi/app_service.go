package httpapi

import (
	"net/http"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/app"
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
		Store:                 s.store,
		Clock:                 s.effectiveClock(),
		Runtime:               &app.RuntimeContext{},
		DisableScopeBootstrap: true,
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
		SubjectUserRef:     strings.TrimSpace(r.Header.Get("X-Xuanchu-As")),
	})
	if err != nil {
		if state, ok := r.Context().Value(logStateContextKey).(*requestLogState); ok && strings.TrimSpace(r.Header.Get("X-Xuanchu-As")) != "" {
			state.impersonateAttempt = strings.TrimSpace(r.Header.Get("X-Xuanchu-As"))
		}
		return nil, requestAuth{}, err
	}
	scoped, err := app.NewService(app.ServiceOptions{
		Store:        s.store,
		Clock:        s.effectiveClock(),
		Runtime:      &authorized.Runtime,
		RequestScope: &authorized.Decision.RequestScope,
	})
	if err != nil {
		return nil, requestAuth{}, err
	}
	authn.EffectiveWorkspace = authorized.Workspace
	// 用授权决策覆盖 access log state。注意：只有走 scopedServiceFor 的 handler
	// 才会到这里，因此 access log 中的 actor/workspace 对于这些请求是「实际解析到的
	// 业务身份」（impersonation 时为 subject），比 authMiddleware 写入的 token 默认值
	// 更准确。未走 scopedServiceFor 的 handler 仍保留 authMiddleware 写入的默认值。
	if state, ok := r.Context().Value(logStateContextKey).(*requestLogState); ok {
		state.actorID = authorized.Decision.Principal.UserID
		state.workspaceID = authorized.Decision.Tenant.WorkspaceID
		state.workspaceRef = authorized.Decision.Tenant.WorkspaceSlug
		if authorized.Decision.Delegator != nil {
			state.delegatorUserID = authorized.Decision.Delegator.UserID
			state.delegatorTokenID = authorized.Decision.Delegator.TokenID
		}
	}
	return scoped, authn, nil
}

func requestWorkspaceRef(r *http.Request) string {
	if value := strings.TrimSpace(r.URL.Query().Get("workspace")); value != "" {
		return value
	}
	return strings.TrimSpace(r.Header.Get("X-Xuanchu-Workspace"))
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
