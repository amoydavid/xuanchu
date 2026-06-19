package authz

import "testing"

func TestAuthzPermissionErrorType(t *testing.T) {
	permErr := PermissionError{Code: CodePermissionDenied, Message: "permission denied"}
	if permErr.Error() != "permission denied" {
		t.Fatalf("PermissionError() = %q", permErr.Error())
	}
	if permErr.Code != CodePermissionDenied {
		t.Fatalf("code = %q, want %q", permErr.Code, CodePermissionDenied)
	}
}

func TestAuthzErrorCodeConstants(t *testing.T) {
	tests := map[string]string{
		CodeAuthMissingToken:     "auth_missing_token",
		CodeAuthInvalidToken:     "auth_invalid_token",
		CodeAuthTokenRevoked:     "auth_token_revoked",
		CodeAuthTokenExpired:     "auth_token_expired",
		CodeTokenScopeDenied:     "token_scope_denied",
		CodeWorkspaceScopeDenied: "workspace_scope_denied",
		CodeProjectScopeDenied:   "project_scope_denied",
		CodeMembershipNotFound:   "membership_not_found",
		CodePermissionDenied:     "permission_denied",
		CodeWorkspaceRequired:    "workspace_required",
		CodeWorkspaceArchived:    "workspace_archived",
	}
	for got, want := range tests {
		if got != want {
			t.Fatalf("code constant = %q, want %q", got, want)
		}
	}
}
