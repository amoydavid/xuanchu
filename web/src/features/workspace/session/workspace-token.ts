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

// SSO / OIDC browser session 模式
// -----
// 与 sessionStorage token 模式不同，SSO 登录态存在两个 cookie：
// - xuanchu_session（HttpOnly，JS 不可读、不可清，必须由后端清）
// - xuanchu_csrf（JS 可读，作为 double-submit 证据）
// 因此登出时必须调后端 POST /auth/logout，仅清 sessionStorage 无效。

const csrfCookieName = "xuanchu_csrf"

// hasSsoBrowserSession 检测是否存在 xuanchu_csrf cookie。
// 存在即说明当前是 OIDC browser session 模式（SSO 登录后由后端写入）。
export function hasSsoBrowserSession(): boolean {
  if (typeof document === "undefined") return false
  return document.cookie.split("; ").some((row) => row.startsWith(`${csrfCookieName}=`))
}

function readCsrfCookie(): string | undefined {
  const match = document.cookie
    .split("; ")
    .find((row) => row.startsWith(`${csrfCookieName}=`))
  return match?.split("=")[1]
}

// endSsoBrowserSession 调用后端 POST /auth/logout 清除 session/csrf cookie。
// 后端要求 X-Xuanchu-CSRF header 与 cookie 值相等（double-submit CSRF）。
// 用 redirect:"manual" 避免浏览器跟随 302 去抓 HTML（fetch 会按 JSON 解析报错）。
export async function endSsoBrowserSession(): Promise<void> {
  const csrf = readCsrfCookie()
  await fetch("/auth/logout", {
    method: "POST",
    headers: csrf ? { "X-Xuanchu-CSRF": csrf } : {},
    credentials: "same-origin",
    redirect: "manual",
  })
}
