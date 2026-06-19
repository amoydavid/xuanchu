package authz

func AllowedForRole(role Role, p Permission) bool {
	switch role {
	case RoleOwner:
		return true
	case RoleAdmin:
		switch p {
		case PermissionTaskRead, PermissionTaskWrite,
			PermissionProjectRead, PermissionProjectManage, PermissionProjectConfigRead, PermissionProjectConfigWrite,
			PermissionConfigSchemaRead, PermissionConfigSchemaWrite,
			PermissionContextUse, PermissionContextManage, PermissionUDAManage, PermissionWorkspaceRead, PermissionWorkspaceModify, PermissionMemberManage, PermissionAuditRead,
			PermissionTokenRead, PermissionTokenWrite,
			PermissionHookRead, PermissionHookWrite,
			PermissionNotificationRead, PermissionNotificationWrite, PermissionReminderRead, PermissionReminderWrite:
			return true
		}
	case RoleMember:
		switch p {
		case PermissionTaskRead, PermissionTaskWrite,
			PermissionProjectRead, PermissionProjectConfigRead, PermissionConfigSchemaRead,
			PermissionContextUse, PermissionContextManage, PermissionWorkspaceRead,
			PermissionReminderRead:
			return true
		}
	case RoleViewer:
		switch p {
		case PermissionTaskRead, PermissionProjectRead, PermissionProjectConfigRead, PermissionConfigSchemaRead, PermissionContextUse, PermissionWorkspaceRead:
			return true
		}
	}
	return false
}
