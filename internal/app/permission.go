package app

import (
	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/authz"
)

// Permission 复用 authz.Permission，保持 app 层现有 API 稳定。
type Permission = authz.Permission

const (
	PermissionTaskWrite          = authz.PermissionTaskWrite
	PermissionTaskRead           = authz.PermissionTaskRead
	PermissionProjectRead        = authz.PermissionProjectRead
	PermissionProjectManage      = authz.PermissionProjectManage
	PermissionProjectConfigRead  = authz.PermissionProjectConfigRead
	PermissionProjectConfigWrite = authz.PermissionProjectConfigWrite
	PermissionConfigSchemaRead   = authz.PermissionConfigSchemaRead
	PermissionConfigSchemaWrite  = authz.PermissionConfigSchemaWrite
	PermissionContextUse         = authz.PermissionContextUse
	PermissionContextManage      = authz.PermissionContextManage
	PermissionUDARead            = authz.PermissionUDARead
	PermissionUDAManage          = authz.PermissionUDAManage
	PermissionWorkspaceRead      = authz.PermissionWorkspaceRead
	PermissionWorkspaceModify    = authz.PermissionWorkspaceModify
	PermissionWorkspaceArchive   = authz.PermissionWorkspaceArchive
	PermissionMemberManage       = authz.PermissionMemberManage
	PermissionMemberManageOwner  = authz.PermissionMemberManageOwner
	PermissionAuditRead          = authz.PermissionAuditRead
	PermissionTokenRead          = authz.PermissionTokenRead
	PermissionTokenWrite         = authz.PermissionTokenWrite
	PermissionHookRead           = authz.PermissionHookRead
	PermissionHookWrite          = authz.PermissionHookWrite
	PermissionNotificationRead   = authz.PermissionNotificationRead
	PermissionNotificationWrite  = authz.PermissionNotificationWrite
	PermissionReminderRead       = authz.PermissionReminderRead
	PermissionReminderWrite      = authz.PermissionReminderWrite
	PermissionSsoConfigRead      = authz.PermissionSsoConfigRead
	PermissionSsoConfigWrite     = authz.PermissionSsoConfigWrite
)

// PermissionError 复用 authz.PermissionError，保持 app 层现有 API 稳定。
type PermissionError = authz.PermissionError

func (s *Service) Require(p Permission) error {
	if s.runtime.IsTenantActor() {
		capability, ok := tenantCapabilityForPermission(p)
		if !ok || s.requestScope == nil || !s.requestScope.HasCapability(capability) {
			return PermissionError{Code: authz.CodePermissionDenied, Message: "permission denied"}
		}
		return nil
	}
	return requireRolePermission(s.runtime.Role, p)
}

func tenantCapabilityForPermission(p Permission) (string, bool) {
	switch p {
	case PermissionTaskRead:
		return auth.ScopeTaskRead, true
	case PermissionTaskWrite:
		return auth.ScopeTaskWrite, true
	case PermissionProjectRead:
		return auth.ScopeProjectRead, true
	case PermissionProjectManage:
		return auth.ScopeProjectWrite, true
	case PermissionProjectConfigRead, PermissionConfigSchemaRead:
		return auth.ScopeConfigRead, true
	case PermissionUDARead:
		return auth.ScopeConfigRead, true
	case PermissionProjectConfigWrite, PermissionConfigSchemaWrite, PermissionUDAManage:
		return auth.ScopeConfigWrite, true
	case PermissionContextUse:
		return auth.ScopeContextRead, true
	case PermissionContextManage:
		return auth.ScopeContextWrite, true
	case PermissionWorkspaceRead:
		return auth.ScopeWorkspaceRead, true
	case PermissionWorkspaceModify:
		return auth.ScopeWorkspaceWrite, true
	case PermissionSsoConfigRead:
		return auth.ScopeWorkspaceRead, true
	case PermissionSsoConfigWrite:
		return auth.ScopeWorkspaceWrite, true
	case PermissionAuditRead:
		return auth.ScopeAuditRead, true
	case PermissionHookRead:
		return auth.ScopeHookRead, true
	case PermissionHookWrite:
		return auth.ScopeHookWrite, true
	case PermissionNotificationRead:
		return auth.ScopeNotificationRead, true
	case PermissionNotificationWrite:
		return auth.ScopeNotificationWrite, true
	case PermissionReminderRead:
		return auth.ScopeReminderRead, true
	case PermissionReminderWrite:
		return auth.ScopeReminderWrite, true
	default:
		return "", false
	}
}
