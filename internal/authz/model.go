package authz

type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
	RoleViewer Role = "viewer"
)

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
	PermissionNotificationRead   Permission = "notification.read"
	PermissionNotificationWrite  Permission = "notification.write"
	PermissionReminderRead       Permission = "reminder.read"
	PermissionReminderWrite      Permission = "reminder.write"
)

type CredentialKind string

const (
	CredentialPAT            CredentialKind = "pat"
	CredentialAgent          CredentialKind = "agent"
	CredentialServerAdmin    CredentialKind = "server_admin"
	CredentialBrowserSession CredentialKind = "browser_session"
)

type Credential struct {
	Kind         CredentialKind
	TokenID      string
	TokenUserID  string
	Capabilities []string
	WorkspaceIDs []string
	ProjectIDs   []string
}

type Principal struct {
	UserID   string
	UserName string
}

type Delegator struct {
	UserID  string
	TokenID string
}

type TenantScope struct {
	WorkspaceID   string
	WorkspaceSlug string
	ProjectID     *string
}

type RequestScope struct {
	TokenID      string
	TokenType    string
	WorkspaceIDs []string
	ProjectIDs   []string
	Capabilities []string
}

type Requirement struct {
	Capability string
	Permission Permission
}

type Decision struct {
	Principal    Principal
	Delegator    *Delegator
	Credential   Credential
	Tenant       TenantScope
	Role         Role
	RequestScope RequestScope
}
