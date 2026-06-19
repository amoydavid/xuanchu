package httpapi

import (
	"context"
	"net/http"

	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

type adminAuthInfo struct {
	// TokenID 是 DB 持久化 server admin token 的 ID；配置文件 hash token 无 ID 时为 nil。
	TokenID *string
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
		setupRequired, err := s.adminSetupRequired()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "api_internal", "internal server error", nil)
			return
		}
		if setupRequired {
			writeError(w, http.StatusUnauthorized, "admin_setup_required", "admin setup is required", nil)
			return
		}
		raw, ok := bearerToken(r.Header.Get("Authorization"))
		if !ok {
			writeError(w, http.StatusUnauthorized, "admin_auth_required", "admin token is required", nil)
			return
		}
		prefix := auth.AdminTokenDisplayPrefix(raw)
		repo := storage.NewServerAdminTokenRepository(s.store.DB())
		if row, err := repo.GetByPrefix(prefix); err == nil {
			if row.Enabled && row.RevokedAt == nil && auth.VerifyAdminToken(raw, row.TokenHash) {
				_ = repo.TouchLastUsed(row.ID, s.effectiveClock().Unix())
				tokenID := row.ID
				ctx := context.WithValue(r.Context(), adminAuthContextKey{}, adminAuthInfo{TokenID: &tokenID, TokenName: row.Name})
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		} else if err != storage.ErrNotFound {
			writeError(w, http.StatusInternalServerError, "api_internal", "internal server error", nil)
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
