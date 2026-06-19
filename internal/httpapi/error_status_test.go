package httpapi

import (
	"net/http"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/authz"
)

func TestStatusForAppErrorCodeAuthorizationBoundary(t *testing.T) {
	tests := []struct {
		code string
		want int
	}{
		{authz.CodeAuthMissingToken, http.StatusUnauthorized},
		{authz.CodeAuthInvalidToken, http.StatusUnauthorized},
		{authz.CodeAuthTokenExpired, http.StatusUnauthorized},
		{authz.CodeAuthTokenRevoked, http.StatusUnauthorized},
		{authz.CodeTokenScopeDenied, http.StatusForbidden},
		{authz.CodeWorkspaceScopeDenied, http.StatusForbidden},
		{authz.CodeProjectScopeDenied, http.StatusForbidden},
		{authz.CodeMembershipNotFound, http.StatusForbidden},
		{authz.CodePermissionDenied, http.StatusForbidden},
		{authz.CodeWorkspaceRequired, http.StatusBadRequest},
		// acting token 过期按 spec §10 映射为 401
		{"admin_acting_session_expired", http.StatusUnauthorized},
		// 吊销不存在的 acting session 映射为 404
		{"admin_acting_not_found", http.StatusNotFound},
	}
	for _, tt := range tests {
		if got := statusForAppErrorCode(tt.code); got != tt.want {
			t.Fatalf("statusForAppErrorCode(%q) = %d, want %d", tt.code, got, tt.want)
		}
	}
}
