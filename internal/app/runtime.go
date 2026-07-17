package app

import (
	"fmt"
	"net/http"

	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/authz"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

// Role 复用 authz.Role，保持 app 层现有 API 稳定。
type Role = authz.Role

const (
	RoleOwner  = authz.RoleOwner
	RoleAdmin  = authz.RoleAdmin
	RoleMember = authz.RoleMember
	RoleViewer = authz.RoleViewer
)

type RuntimeContext struct {
	ActorType         string
	ActorUserID       string
	ActorName         string
	ActorTokenID      string
	ActorTokenName    string
	ActorTokenPrefix  string
	ActorTokenPurpose string
	WorkspaceID       string
	WorkspaceSlug     string
	Role              Role
	DelegatorTokenID  string
	DelegatorUserID   string
	// 以下三个字段由 server admin acting session 委托链路填充，
	// 与普通 user-agent impersonation 的 DelegatorTokenID/DelegatorUserID 独立。
	AdminActingSessionID    string
	DelegatorAdminTokenID   string
	DelegatorAdminTokenName string
	// WebLoginDisabled 表示当前请求来自 SSO browser session。
	// 由此创建的 PAT/Agent token 会带上 WebLoginDisabled 标记，禁止后续用于 Web Console 登录页登录，
	// 守住「SSO workspace 人工 token 走 SSO 登录」边界。token 在 API/MCP/CLI 仍可用。
	WebLoginDisabled bool
	// ActorTokenType 记录调用者凭证的原始 token type（pat/agent/tenant_access_token/browser_session）。
	// 用于区分「capability 是真实授权边界（PAT/Agent/tenant token）」与
	// 「capability 是交互层人为收紧（browser session）」，影响 token 管理的子集校验语义。
	ActorTokenType string
}

// BrowserSessionTokenType 是 Web Console SSO 登录会话的凭证类型。
// 由 httpapi 层在 AuthenticateBearerToken 之外的 cookie session 路径构造，
// app 层只识别该字符串值，不依赖 httpapi 包。
const BrowserSessionTokenType = "browser_session"

// CredentialIsBrowserSession 判断当前调用者是否为 Web Console 的 SSO browser session。
// browser session 的 capability 集合是交互层人为收紧（见 httpapi.browserSessionScopes），
// 真实授权由 membership role 决定，因此在 token 管理的子集校验里不应作为权限边界。
func (rt RuntimeContext) CredentialIsBrowserSession() bool {
	return rt.ActorTokenType == BrowserSessionTokenType
}

type ServiceOptions struct {
	Store                 *storage.Store
	Clock                 Clock
	NoContext             bool
	DisableScopeBootstrap bool
	RuntimeConfig         map[string]string
	RuntimeOverrides      map[string]string
	ActorRef              string
	WorkspaceRef          string
	Runtime               *RuntimeContext
	RequestScope          *RequestScope
	// SinkTestClient 注入到 notification sink 测试投递；nil 时使用 SSRF-safe 默认 client。
	SinkTestClient *http.Client
	// SinkTestResolver 覆盖 sink 测试投递的 DNS 解析器，便于测试。
	SinkTestResolver HookHostResolver
	// TokenSecretKey 用于加密可恢复的 API token 明文，供 Web Console 按需 reveal。
	TokenSecretKey []byte
	// RequireTokenSecret 为 true 时，缺少 TokenSecretKey 的 token 创建请求会失败。
	RequireTokenSecret bool
}

type RuntimeError struct {
	Code    string
	Message string
}

func (e RuntimeError) Error() string {
	return e.Message
}

func (rt RuntimeContext) IsTenantActor() bool {
	return rt.ActorType == auth.TokenTypeTenantAccess
}

func tenantActorNotUserError() RuntimeError {
	return RuntimeError{Code: "tenant_actor_not_user", Message: "tenant token has no user actor"}
}

func activeWorkspaceMetaKey(userID string) string {
	return "active_workspace." + userID
}

func activeContextMetaKey(userID, workspaceID string) string {
	return "active_context." + userID + "." + workspaceID
}

func ResolveRuntimeContext(store *storage.Store, userRepo *storage.UserRepository, workspaceRepo *storage.WorkspaceRepository, memberRepo *storage.MemberRepository, actorRef, workspaceRef string) (RuntimeContext, error) {
	user, err := resolveActor(store, userRepo, actorRef)
	if err != nil {
		return RuntimeContext{}, err
	}

	workspace, err := resolveWorkspace(store, workspaceRepo, user, workspaceRef)
	if err != nil {
		return RuntimeContext{}, err
	}
	if workspace.ArchivedAt != nil {
		return RuntimeContext{}, RuntimeError{Code: authz.CodeWorkspaceArchived, Message: fmt.Sprintf("workspace %q is archived", workspace.Slug)}
	}

	member, err := memberRepo.Get(user.ID, workspace.ID)
	if err == storage.ErrNotFound {
		return RuntimeContext{}, RuntimeError{Code: authz.CodeMembershipNotFound, Message: fmt.Sprintf("user %q is not a member of workspace %q", user.Name, workspace.Slug)}
	}
	if err != nil {
		return RuntimeContext{}, err
	}

	return RuntimeContext{
		ActorType:     string(authz.ActorUser),
		ActorUserID:   user.ID,
		ActorName:     user.Name,
		WorkspaceID:   workspace.ID,
		WorkspaceSlug: workspace.Slug,
		Role:          Role(member.Role),
	}, nil
}

func resolveActor(store *storage.Store, userRepo *storage.UserRepository, actorRef string) (storage.User, error) {
	if actorRef != "" {
		if user, err := userRepo.GetByID(actorRef); err == nil {
			return user, nil
		}
		return userRepo.GetByName(actorRef)
	}
	if activeID, ok, err := store.GetMeta("active_user_id"); err != nil {
		return storage.User{}, err
	} else if ok && activeID != "" {
		return userRepo.GetByID(activeID)
	}
	return userRepo.GetByName("local")
}

func resolveWorkspace(store *storage.Store, workspaceRepo *storage.WorkspaceRepository, user storage.User, workspaceRef string) (storage.Workspace, error) {
	if workspaceRef != "" {
		return lookupWorkspace(workspaceRepo, workspaceRef)
	}
	if activeRef, ok, err := store.GetMeta(activeWorkspaceMetaKey(user.ID)); err != nil {
		return storage.Workspace{}, err
	} else if ok && activeRef != "" {
		return lookupWorkspace(workspaceRepo, activeRef)
	}
	if user.DefaultWorkspaceID != nil && *user.DefaultWorkspaceID != "" {
		return workspaceRepo.GetByID(*user.DefaultWorkspaceID)
	}
	return workspaceRepo.GetBySlug("local")
}

func lookupWorkspace(workspaceRepo *storage.WorkspaceRepository, ref string) (storage.Workspace, error) {
	if ws, err := workspaceRepo.GetByID(ref); err == nil {
		return ws, nil
	}
	ws, err := workspaceRepo.GetBySlug(ref)
	if err == storage.ErrNotFound {
		return storage.Workspace{}, RuntimeError{Code: "workspace_not_found", Message: fmt.Sprintf("workspace %q not found", ref)}
	}
	return ws, err
}
