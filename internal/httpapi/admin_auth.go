package httpapi

import (
	"context"
	"net/http"

	"git.dajee.net/dajee/xuanchu/internal/auth"
)

type adminAuthInfo struct {
	TokenName string
}

type adminAuthContextKey struct{}

func adminAuthFromContext(ctx context.Context) (adminAuthInfo, bool) {
	v, ok := ctx.Value(adminAuthContextKey{}).(adminAuthInfo)
	return v, ok
}

func (s *Server) adminAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.admin.Enabled {
			writeError(w, http.StatusNotFound, "route_not_found", "route not found", nil)
			return
		}
		raw, ok := bearerToken(r.Header.Get("Authorization"))
		if !ok {
			writeError(w, http.StatusUnauthorized, "admin_auth_required", "admin token is required", nil)
			return
		}
		for _, token := range s.admin.Tokens {
			if !token.Enabled {
				continue
			}
			if auth.VerifyAdminToken(raw, token.Hash) {
				ctx := context.WithValue(r.Context(), adminAuthContextKey{}, adminAuthInfo{TokenName: token.Name})
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}
		writeError(w, http.StatusUnauthorized, "admin_auth_invalid", "admin token is invalid", nil)
	})
}
