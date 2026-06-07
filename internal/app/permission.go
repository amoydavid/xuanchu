package app

type Permission string

const (
	PermissionTaskWrite          Permission = "task.write"
	PermissionTaskRead           Permission = "task.read"
	PermissionProjectRead        Permission = "project.read"
	PermissionProjectManage      Permission = "project.manage"
	PermissionProjectConfigRead  Permission = "project.config.read"
	PermissionProjectConfigWrite Permission = "project.config.write"
	PermissionConfigSchemaRead   Permission = "config.schema.read"
	PermissionConfigSchemaWrite  Permission = "config.schema.write"
	PermissionContextUse         Permission = "context.use"
	PermissionContextManage      Permission = "context.manage"
	PermissionUDAManage          Permission = "uda.manage"
	PermissionWorkspaceRead      Permission = "workspace.read"
	PermissionWorkspaceModify    Permission = "workspace.modify"
	PermissionWorkspaceArchive   Permission = "workspace.archive"
	PermissionMemberManage       Permission = "member.manage"
	PermissionMemberManageOwner  Permission = "member.manage.owner"
	PermissionAuditRead          Permission = "audit.read"
	PermissionTokenRead          Permission = "token.read"
	PermissionTokenWrite         Permission = "token.write"
	PermissionHookRead           Permission = "hook.read"
	PermissionHookWrite          Permission = "hook.write"
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
