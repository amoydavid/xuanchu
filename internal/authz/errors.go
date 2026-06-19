package authz

// 授权边界错误码集中定义。
//
// 这些常量覆盖认证、token scope、workspace/project allowlist、membership、
// role permission 相关错误。其他领域错误（hook、notification、task、config 等）
// 继续留在各自调用层，不放进本包。
const (
	CodeAuthMissingToken     = "auth_missing_token"
	CodeAuthInvalidToken     = "auth_invalid_token"
	CodeAuthTokenRevoked     = "auth_token_revoked"
	CodeAuthTokenExpired     = "auth_token_expired"
	CodeTokenScopeDenied     = "token_scope_denied"
	CodeWorkspaceScopeDenied = "workspace_scope_denied"
	CodeProjectScopeDenied   = "project_scope_denied"
	CodeMembershipNotFound   = "membership_not_found"
	CodePermissionDenied     = "permission_denied"
	CodeWorkspaceRequired    = "workspace_required"
	CodeWorkspaceArchived    = "workspace_archived"
)

// PermissionError 表示角色权限不足导致的授权拒绝。
type PermissionError struct {
	Code    string
	Message string
}

func (e PermissionError) Error() string { return e.Message }
