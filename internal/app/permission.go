package app

type Permission string

const (
	PermissionTaskWrite        Permission = "task.write"
	PermissionTaskRead         Permission = "task.read"
	PermissionContextUse       Permission = "context.use"
	PermissionContextManage    Permission = "context.manage"
	PermissionUDAManage        Permission = "uda.manage"
	PermissionWorkspaceModify  Permission = "workspace.modify"
	PermissionWorkspaceArchive Permission = "workspace.archive"
	PermissionMemberManage     Permission = "member.manage"
	PermissionMemberManageOwner Permission = "member.manage.owner"
	PermissionAuditRead        Permission = "audit.read"
)

type PermissionError struct {
	Code    string
	Message string
}

func (e PermissionError) Error() string {
	return e.Message
}

func (s *Service) Require(p Permission) error {
	if s.allowed(p) {
		return nil
	}
	return PermissionError{
		Code:    "permission_denied",
		Message: "permission denied",
	}
}

func (s *Service) allowed(p Permission) bool {
	switch s.runtime.Role {
	case RoleOwner:
		return true
	case RoleAdmin:
		switch p {
		case PermissionTaskRead, PermissionTaskWrite, PermissionContextUse, PermissionContextManage, PermissionUDAManage, PermissionWorkspaceModify, PermissionMemberManage, PermissionAuditRead:
			return true
		}
	case RoleMember:
		switch p {
		case PermissionTaskRead, PermissionTaskWrite, PermissionContextUse, PermissionContextManage:
			return true
		}
	case RoleViewer:
		switch p {
		case PermissionTaskRead, PermissionContextUse:
			return true
		}
	}
	return false
}
