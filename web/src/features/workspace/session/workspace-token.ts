// 普通 workspace token（PAT / Agent）。普通 Console 鉴权用。
const tokenKey = "xuanchu.console.token"

// server admin acting session token。由 /admin/workspaces 详情页签发，
// 只保存在当前浏览器 tab。acting mode 下 Workspace Console 优先使用它。
const actingTokenKey = "xuanchu.console.admin_acting_token"

// acting session 元数据（workspace / actor / role / admin token name）。
// 用于 acting banner 展示和「返回超管界面」的跳转目标。
const actingContextKey = "xuanchu.console.admin_acting_context"
const tenantContextKey = "xuanchu.console.tenant_context"

export type ActingContext = {
  workspaceSlug: string
  workspaceName: string
  actorName: string
  role: string
  adminTokenName: string
}

export type TenantSwitchContext = {
  mode: "tenant"
  workspaceSlug: string
  workspaceName: string
  actorName: string
  tokenName: string
  adminTokenName: string
  returnTo: string
}

export function getWorkspaceToken(): string | null {
  return sessionStorage.getItem(tokenKey)
}

export function setWorkspaceToken(token: string): void {
  sessionStorage.setItem(tokenKey, token)
}

export function clearWorkspaceToken(): void {
  sessionStorage.removeItem(tokenKey)
}

export function getAdminActingToken(): string | null {
  return sessionStorage.getItem(actingTokenKey)
}

export function setAdminActingToken(token: string): void {
  sessionStorage.setItem(actingTokenKey, token)
}

export function clearAdminActingToken(): void {
  sessionStorage.removeItem(actingTokenKey)
}

export function getAdminActingContext(): ActingContext | null {
  const raw = sessionStorage.getItem(actingContextKey)
  if (!raw) {
    return null
  }
  try {
    return JSON.parse(raw) as ActingContext
  } catch {
    return null
  }
}

export function setAdminActingContext(context: ActingContext): void {
  sessionStorage.setItem(actingContextKey, JSON.stringify(context))
}

export function clearAdminActingContext(): void {
  sessionStorage.removeItem(actingContextKey)
}

// clearAdminActingSession 一次性清理 acting token + context，
// 用于「返回超管界面」和 acting token 过期场景。不影响普通 workspace token 和 admin token。
export function clearAdminActingSession(): void {
  clearAdminActingToken()
  clearAdminActingContext()
}

export function getTenantSwitchContext(): TenantSwitchContext | null {
  const raw = sessionStorage.getItem(tenantContextKey)
  if (!raw) {
    return null
  }
  try {
    const parsed = JSON.parse(raw) as TenantSwitchContext
    return parsed.mode === "tenant" ? parsed : null
  } catch {
    return null
  }
}

export function setTenantSwitchContext(context: TenantSwitchContext): void {
  sessionStorage.setItem(tenantContextKey, JSON.stringify(context))
}

export function clearTenantSwitchContext(): void {
  sessionStorage.removeItem(tenantContextKey)
}

export function clearTenantSwitchSession(): void {
  clearWorkspaceToken()
  clearTenantSwitchContext()
}
