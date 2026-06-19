package app

import "git.dajee.net/dajee/xuanchu/internal/authz"

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
)

type PermissionError struct {
	Code    string
	Message string
}

func (e PermissionError) Error() string {
	return e.Message
}

func (s *Service) Require(p Permission) error {
	return requireRolePermission(s.runtime.Role, p)
}
