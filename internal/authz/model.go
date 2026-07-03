// Package authz 表达 xuanchu 的授权决策概念与规则。
//
// 本包只负责纯模型和纯规则，不直接持有 HTTP、MCP、Cobra、GORM 细节。
// 数据读取仍由 internal/app 通过现有 repository 完成，再把输入交给本包做判断。
package authz

// Role 表示 workspace 内的成员角色。
type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
	RoleViewer Role = "viewer"
)

// Permission 是 app 内部权限，格式 resource.action，例如 task.read。
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
	PermissionSsoConfigRead      Permission = "sso.config.read"
	PermissionSsoConfigWrite     Permission = "sso.config.write"
)

// CredentialKind 表示请求带来的凭证类型。
type CredentialKind string

const (
	ActorUser              ActorType = "user"
	ActorTenantAccessToken ActorType = "tenant_access_token"
)

type ActorType string

type Actor struct {
	Type        ActorType
	UserID      string
	UserName    string
	TokenID     string
	TokenName   string
	TokenPrefix string
}

const (
	CredentialPAT            CredentialKind = "pat"
	CredentialAgent          CredentialKind = "agent"
	CredentialTenantAccess   CredentialKind = "tenant_access_token"
	CredentialServerAdmin    CredentialKind = "server_admin"
	CredentialBrowserSession CredentialKind = "browser_session" // 预留，本次不实现
)

// Credential 表示“请求带来的凭证”，而不是业务用户。
type Credential struct {
	Kind         CredentialKind
	TokenID      string
	TokenUserID  string
	Capabilities []string
	WorkspaceIDs []string
	ProjectIDs   []string
}

// Principal 表示“本次业务操作的名义 actor”。
type Principal struct {
	UserID   string
	UserName string
}

// Delegator 表示“实际持有凭证并发起 impersonation 的一方”。
// 只有 impersonation 请求才有 Delegator，且仅用于审计和日志，
// 不参与业务权限放大。
type Delegator struct {
	UserID  string
	TokenID string
}

// TenantScope 表示请求最终落在哪个 workspace / project 范围内。
type TenantScope struct {
	WorkspaceID   string
	WorkspaceSlug string
	ProjectID     *string
}

// RequestScope 表达 token 带来的请求范围：
// capability（token scope）、workspace allowlist、project allowlist。
type RequestScope struct {
	TokenID      string
	TokenType    string
	WorkspaceIDs []string
	ProjectIDs   []string
	Capabilities []string
}

// Requirement 声明单个操作所需的 capability 和 permission。
type Requirement struct {
	Capability string
	Permission Permission
}

// Decision 是授权决策层输出的统一结果。
type Decision struct {
	Actor        Actor
	Principal    Principal
	Delegator    *Delegator
	Credential   Credential
	Tenant       TenantScope
	Role         Role
	RequestScope RequestScope
}
