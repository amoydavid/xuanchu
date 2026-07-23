package authz

// AllowedForRole 判断某个角色是否具备某个权限。
//
// 角色含义：
//   - owner: workspace 内最高业务权限
//   - admin: workspace 管理员，但不拥有 owner 级成员管理能力
//   - member: 普通协作成员，能读写任务，不能管理 workspace/project 核心结构
//   - viewer: 只读
func AllowedForRole(role Role, p Permission) bool {
	switch role {
	case RoleOwner:
		return true
	case RoleAdmin:
		switch p {
		case PermissionTaskRead, PermissionTaskWrite,
			PermissionProjectRead, PermissionProjectManage, PermissionProjectConfigRead, PermissionProjectConfigWrite,
			PermissionConfigSchemaRead, PermissionConfigSchemaWrite,
			PermissionContextUse, PermissionContextManage, PermissionUDARead, PermissionUDAManage, PermissionWorkspaceRead, PermissionWorkspaceModify, PermissionMemberManage, PermissionAuditRead,
			PermissionTokenRead, PermissionTokenWrite,
			PermissionHookRead, PermissionHookWrite,
			PermissionNotificationRead, PermissionNotificationWrite, PermissionReminderRead, PermissionReminderWrite:
			return true
		}
	case RoleMember:
		switch p {
		case PermissionTaskRead, PermissionTaskWrite,
			PermissionProjectRead, PermissionProjectConfigRead, PermissionConfigSchemaRead,
			PermissionContextUse, PermissionContextManage, PermissionUDARead, PermissionWorkspaceRead,
			PermissionReminderRead, PermissionAuditRead:
			return true
		}
	case RoleViewer:
		switch p {
		case PermissionTaskRead, PermissionProjectRead, PermissionProjectConfigRead, PermissionConfigSchemaRead, PermissionContextUse, PermissionUDARead, PermissionWorkspaceRead, PermissionAuditRead:
			return true
		}
	}
	return false
}
