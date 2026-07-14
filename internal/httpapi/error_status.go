package httpapi

import (
	"net/http"

	"git.dajee.net/dajee/xuanchu/internal/authz"
)

// statusForAppErrorCode 集中维护 app 错误码到 HTTP status 的映射。
//
// 授权边界错误（auth/token scope/workspace scope/project scope/membership/
// permission）走 authz 常量；其他领域错误（task/project/hook/notification/
// token 管理等）继续在此处维护，不搬进 internal/authz。
func statusForAppErrorCode(code string) int {
	switch code {
	// 认证与授权边界
	case authz.CodeAuthMissingToken, authz.CodeAuthInvalidToken, authz.CodeAuthTokenExpired, authz.CodeAuthTokenRevoked:
		return http.StatusUnauthorized
	case "admin_acting_session_expired":
		// acting token 过期属于认证失败：spec §10 规定 401。
		return http.StatusUnauthorized
	case "admin_acting_not_allowed":
		return http.StatusUnauthorized
	case "tenant_token_management_denied", "token_management_denied":
		return http.StatusForbidden
	case "token_web_login_disabled":
		// SSO browser session 创建的 PAT/Agent token 不能用于 Console 登录。
		return http.StatusForbidden
	case authz.CodeTokenScopeDenied, authz.CodeWorkspaceScopeDenied, authz.CodeProjectScopeDenied, authz.CodeMembershipNotFound, authz.CodePermissionDenied:
		return http.StatusForbidden
	case authz.CodeWorkspaceRequired:
		return http.StatusBadRequest
	// 资源不存在
	case "workspace_not_found", "project_not_found", "task_not_found", "token_not_found", "tenant_token_not_found", "context_not_found", "hook_not_found", "hook_delivery_not_found", "annotation_not_found", "notification_sink_not_found", "reminder_rule_not_found", "notification_rule_not_found", "notification_delivery_not_found", "admin_acting_not_found", "task_series_not_found", "task_series_occurrence_not_found", "task_occurrence_not_found":
		return http.StatusNotFound
	case "workspace_archived":
		return http.StatusBadRequest
	// 冲突
	case "admin_workspace_exists":
		return http.StatusConflict
	// 循环系列冲突/状态错误（spec §21）
	case "task_series_inactive", "task_recurrence_backlog":
		return http.StatusConflict
	case "task_series_project_closed":
		return http.StatusConflict
	// 循环系列输入校验
	case "task_series_due_required", "task_series_invalid_until", "task_series_invalid_rule", "task_series_invalid_effective_from", "task_series_unsupported_field", "task_series_endpoint_required", "task_occurrence_project_immutable", "task_occurrence_range_required", "task_occurrence_range_too_large", "uda_not_defined", "uda_orphan_readonly", "uda_value_invalid":
		return http.StatusBadRequest
	// 输入校验类
	case "hook_delivery_not_replayable", "hook_endpoint_invalid", "hook_event_types_invalid", "hook_name_invalid", "hook_timeout_invalid", "hook_max_attempts_invalid", "hook_project_required", "hook_scope_invalid", "hook_secret_invalid", "notification_sink_invalid", "reminder_rule_invalid", "notification_rule_invalid", "endpoint_unresolved", "endpoint_host_denied", "endpoint_mode_invalid", "endpoint_template_invalid", "audience_unsupported", "audience_unsupported_for_event", "template_unresolved", "token_ambiguous_ref", "tenant_token_name_required", "tenant_token_scope_invalid", "tenant_token_project_scope_invalid", "tenant_actor_not_user":
		return http.StatusBadRequest
	case "tenant_token_revoked", "tenant_token_expired":
		return http.StatusConflict
	case "token_secret_unavailable":
		// 令牌没有可恢复密文或解密失败：冲突语义，提示重新签发。
		return http.StatusConflict
	case "config_secret_key_missing":
		// 服务端未配置解密密钥：客户端无法通过重试解决，归类为错误请求。
		return http.StatusBadRequest
	case "token_list_failed":
		return http.StatusInternalServerError
	case "route_not_found":
		return http.StatusNotFound
	case "method_not_allowed":
		return http.StatusMethodNotAllowed
	default:
		return http.StatusBadRequest
	}
}
