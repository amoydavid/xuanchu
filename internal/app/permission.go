package app

type Permission string

const (
	PermissionTaskWrite         Permission = "task.write"
	PermissionTaskRead          Permission = "task.read"
	PermissionContextUse        Permission = "context.use"
	PermissionContextManage     Permission = "context.manage"
	PermissionUDAManage         Permission = "uda.manage"
	PermissionWorkspaceModify   Permission = "workspace.modify"
	PermissionWorkspaceArchive  Permission = "workspace.archive"
	PermissionMemberManage      Permission = "member.manage"
	PermissionMemberManageOwner Permission = "member.manage.owner"
	PermissionAuditRead         Permission = "audit.read"
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
