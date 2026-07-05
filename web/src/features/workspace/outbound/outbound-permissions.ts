// 出站集成控制台前端权限 helper。
// 仅用于在 UI 上"减少误点"，不是真实的安全裁决——服务端 authz + scope 仍是最终裁决。
// 规则与 internal/authz/policy.go + scope 字符串对齐。

import {
  SCOPE_HOOK_WRITE,
  SCOPE_NOTIFICATION_WRITE,
  SCOPE_REMINDER_WRITE,
} from "@/features/workspace/tokens/scopes"

export type OutboundPermissionInput = {
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
 * 判断当前身份是否能写 Hook。
 *
 * 后端规则（policy.go）：
 *   - owner / admin 角色允许 PermissionHookWrite。
 *   - tenant_access_token 只要带 hook:write scope 即可（绕过 role 检查）。
 *   - OIDC browser session（无 token scopes）按 effective_role 判断。
 */
export function canWriteHooks(input: OutboundPermissionInput): boolean {
  const { role, actorType, scopes } = input
  if (actorType === "tenant_access_token") {
    return hasScope(scopes, SCOPE_HOOK_WRITE)
  }
  // 浏览器 session：有 token scopes 时按 scope 兜底，否则按 role。
  if (scopes && scopes.length > 0) {
    return hasScope(scopes, SCOPE_HOOK_WRITE) && !!role && ADMIN_ROLES.has(role)
  }
  return !!role && ADMIN_ROLES.has(role)
}

/** 判断当前身份是否能写 sink（notification:write）。 */
export function canWriteSinks(input: OutboundPermissionInput): boolean {
  const { role, actorType, scopes } = input
  if (actorType === "tenant_access_token") {
    return hasScope(scopes, SCOPE_NOTIFICATION_WRITE)
  }
  if (scopes && scopes.length > 0) {
    return (
      hasScope(scopes, SCOPE_NOTIFICATION_WRITE) &&
      !!role &&
      ADMIN_ROLES.has(role)
    )
  }
  return !!role && ADMIN_ROLES.has(role)
}

/** 判断当前身份是否能写 reminder rule（reminder:write）。 */
export function canWriteReminderRules(input: OutboundPermissionInput): boolean {
  const { role, actorType, scopes } = input
  if (actorType === "tenant_access_token") {
    return hasScope(scopes, SCOPE_REMINDER_WRITE)
  }
  if (scopes && scopes.length > 0) {
    return (
      hasScope(scopes, SCOPE_REMINDER_WRITE) &&
      !!role &&
      ADMIN_ROLES.has(role)
    )
  }
  return !!role && ADMIN_ROLES.has(role)
}

/** 是否整体只读——任一写权限都没有时显示只读提示条。 */
export function isOutboundReadonly(input: OutboundPermissionInput): boolean {
  return (
    !canWriteHooks(input) &&
    !canWriteSinks(input) &&
    !canWriteReminderRules(input)
  )
}
