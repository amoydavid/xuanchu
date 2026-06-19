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
	}
	for _, tt := range tests {
		if got := statusForAppErrorCode(tt.code); got != tt.want {
			t.Fatalf("statusForAppErrorCode(%q) = %d, want %d", tt.code, got, tt.want)
		}
	}
}
