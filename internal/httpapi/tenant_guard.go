package httpapi

import "git.dajee.net/dajee/xuanchu/internal/app"

func rejectTenantActor(authn requestAuth) error {
	if authn.Authn.TenantActor {
		return app.RuntimeError{Code: "tenant_actor_not_user", Message: "tenant token has no user actor"}
	}
	return nil
}
