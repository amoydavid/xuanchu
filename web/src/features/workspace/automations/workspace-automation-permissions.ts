// Workspace Automation 前端权限 helper。
// 仅用于在 UI 上"减少误点"，不是真实的安全裁决——服务端 authz + scope 仍是最终裁决。
// 规则与 internal/authz/policy.go + scope 字符串对齐。

import {
  SCOPE_HOOK_READ,
  SCOPE_HOOK_WRITE,
  SCOPE_WORKSPACE_READ,
  SCOPE_WORKSPACE_WRITE,
} from "@/features/workspace/tokens/scopes"

export type AutomationPermissionInput = {
  /** effective_role: owner / admin / member / viewer */
  role?: string
  /** actor_type: user / tenant_access_token */
  actorType?: string
  /** token scopes，null/undefined 表示无 token 上下文（OIDC session） */
  scopes?: string[] | null
}

const ADMIN_ROLES = new Set(["owner", "admin"])

/** 判断 scope 列表是否包含目标 scope，"*" 表示通配。 */
export function hasScope(
  scopes: string[] | null | undefined,
  scope: string
): boolean {
  if (!scopes) return false
  if (scopes.includes("*")) return true
  return scopes.includes(scope)
}

/**
 * 判断当前身份是否能读 Workspace Automation（workspace:read + hook:read）。
 *
 * 后端规则（permission.go）：
 *   - owner / admin 角色允许 PermissionWorkspaceRead + PermissionHookRead。
 *   - tenant_access_token 必须同时具备 workspace:read 和 hook:read。
 *   - OIDC browser session（无 token scopes）按 effective_role 判断。
 *   - 任意带 scopes 的 token 必须同时满足两个 scope 才显示入口。
 */
export function canReadAutomation(input: AutomationPermissionInput): boolean {
  const { role, actorType, scopes } = input
  if (actorType === "tenant_access_token") {
    return (
      hasScope(scopes, SCOPE_WORKSPACE_READ) && hasScope(scopes, SCOPE_HOOK_READ)
    )
  }
  if (scopes && scopes.length > 0) {
    return (
      hasScope(scopes, SCOPE_WORKSPACE_READ) &&
      hasScope(scopes, SCOPE_HOOK_READ) &&
      !!role &&
      ADMIN_ROLES.has(role)
    )
  }
  return !!role && ADMIN_ROLES.has(role)
}

/**
 * 判断当前身份是否能写 Workspace Automation（workspace:write + hook:write）。
 */
export function canWriteAutomation(input: AutomationPermissionInput): boolean {
  const { role, actorType, scopes } = input
  if (actorType === "tenant_access_token") {
    return (
      hasScope(scopes, SCOPE_WORKSPACE_WRITE) &&
      hasScope(scopes, SCOPE_HOOK_WRITE)
    )
  }
  if (scopes && scopes.length > 0) {
    return (
      hasScope(scopes, SCOPE_WORKSPACE_WRITE) &&
      hasScope(scopes, SCOPE_HOOK_WRITE) &&
      !!role &&
      ADMIN_ROLES.has(role)
    )
  }
  return !!role && ADMIN_ROLES.has(role)
}
