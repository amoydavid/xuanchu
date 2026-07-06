// Token 相关类型与 API 契约，对齐后端 tokenResponse / createdTokenResponse。

import { ApiError } from "@/lib/api"
import { workspaceApiGet } from "@/features/workspace/session/workspace-api"

/** 列表/详情返回的 token 行，对齐后端 tokenResponse。
 * 注意：workspace_ids/project_ids/scopes 后端可能返回 null（无绑定时），故标为可空。 */
export type TokenRow = {
  id: string
  prefix: string
  name: string
  type: string
  user: { id: string; name: string; email?: string | null }
  workspace_ids: string[] | null
  project_ids: string[] | null
  scopes: string[] | null
  created_at: number
  expires_at?: number | null
  revoked_at?: number | null
  last_used_at?: number | null
}

/** 创建 token 成功响应（含一次性明文）。 */
export type CreatedTokenRow = {
  token: string
} & TokenRow

export type TenantAccessTokenRow = {
  id: string
  prefix: string
  name: string
  type: "tenant_access_token"
  workspace_id: string
  project_ids: string[] | null
  scopes: string[] | null
  created_at: number
  expires_at?: number | null
  revoked_at?: number | null
  last_used_at?: number | null
}

export type CreatedTenantAccessTokenRow = {
  token: string
} & TenantAccessTokenRow

/** 表单值，创建/编辑共用。 */
export type TokenFormValues = {
  name: string
  type: string // 'pat' | 'agent'
  user?: string // 归属用户 user_id，空/undefined = 我自己（仅 admin/owner 代为创建时使用）
  workspaces: string[] // workspace slug/id refs
  scopes: string[]
  projects: string[] // project slug/id refs
  expiresPreset: ExpiresPreset
  expiresAt: string // ISO datetime，仅 preset=custom 时使用
}

export type ExpiresPreset = "never" | "7d" | "30d" | "90d" | "custom"

/** 创建请求体。 */
export type TokenCreateInput = {
  name: string
  type?: string
  user?: string // 归属用户 user_id，后端 resolveTokenTargetUser 校验 admin/owner
  scopes?: string[]
  workspaces?: string[]
  projects?: string[]
  expires_in_seconds?: number | null
}

/**
 * 修改请求体。语义与后端对齐：
 * - undefined（缺省）= 不修改
 * - []（空数组）= 清空
 */
export type TokenModifyInput = {
  name?: string
  scopes?: string[]
  workspaces?: string[]
  projects?: string[]
  expires_in_seconds?: number | null
}

export type TenantAccessTokenFormValues = {
  name: string
  scopes: string[]
  projects: string[]
  expiresPreset: ExpiresPreset
  expiresAt: string
}

export type TenantAccessTokenCreateInput = {
  name: string
  scopes?: string[]
  projects?: string[]
  expires_in_seconds?: number | null
}

export type TenantAccessTokenModifyInput = {
  name?: string
  scopes?: string[]
  projects?: string[]
  expires_in_seconds?: number | null
}

/** token 状态派生。 */
export type TokenStatus = "active" | "expired" | "revoked"

export function deriveTokenStatus(
  row: Pick<TokenRow, "expires_at" | "revoked_at">,
  nowUnix?: number
): TokenStatus {
  if (row.revoked_at) {
    return "revoked"
  }
  const now = nowUnix ?? Math.floor(Date.now() / 1000)
  if (row.expires_at && row.expires_at < now) {
    return "expired"
  }
  return "active"
}

/** expiresPreset + expiresAt → expires_in_seconds（供 API）。null 表示永不过期。 */
export function presetToExpiresSeconds(
  preset: ExpiresPreset,
  customIso: string
): number | null {
  switch (preset) {
    case "never":
      return null
    case "7d":
      return 7 * 24 * 3600
    case "30d":
      return 30 * 24 * 3600
    case "90d":
      return 90 * 24 * 3600
    case "custom": {
      const target = new Date(customIso).getTime()
      if (Number.isNaN(target)) {
        return null
      }
      const diff = Math.floor((target - Date.now()) / 1000)
      return diff > 0 ? diff : null
    }
    default:
      return null
  }
}

/** 由现有 expires_at 反推 preset（编辑时预填用）。 */
export function expiresSecondsToPreset(
  expiresAt?: number | null
): { preset: ExpiresPreset; customIso: string } {
  if (!expiresAt) {
    return { preset: "never", customIso: "" }
  }
  const now = Math.floor(Date.now() / 1000)
  const diffDays = Math.round((expiresAt - now) / 86400)
  if (diffDays === 7) return { preset: "7d", customIso: "" }
  if (diffDays === 30) return { preset: "30d", customIso: "" }
  if (diffDays === 90) return { preset: "90d", customIso: "" }
  return {
    preset: "custom",
    customIso: new Date(expiresAt * 1000).toISOString().slice(0, 16),
  }
}

export function sameExpiresSelection(
  expiresAt: number | null | undefined,
  preset: ExpiresPreset,
  customIso: string
): boolean {
  const current = expiresSecondsToPreset(expiresAt)
  if (current.preset !== preset) {
    return false
  }
  return preset !== "custom" || current.customIso === customIso
}

/** 比较两个字符串数组是否含相同元素集合（顺序无关）。供普通/admin token 表单复用。 */
export function sameSet(a: string[], b: string[]): boolean {
  if (a.length !== b.length) return false
  const set = new Set(a)
  for (const item of b) {
    if (!set.has(item)) return false
  }
  return true
}

/** 从 ApiError 提取 token i18n 错误文案，未知 code 回退通用文案。供普通/admin 复用。 */
export function tokenErrorMessage(
  err: unknown,
  t: (key: string) => string
): string {
  if (err instanceof ApiError) {
    const key = `token.errors.${err.code}`
    const translated = t(key)
    return translated === key ? t("token.errors.unknown") : translated
  }
  return t("token.errors.unknown")
}

/** /mcp-config endpoint 返回的视图，含 raw token 供前端复制。 */
export type TokenMcpConfig = {
  token: string
  token_id: string
  token_name: string
  token_type: string
  prefix: string
  endpoint_path: string
  scopes: string[] | null
  workspace_ids?: string[] | null
  project_ids?: string[] | null
  expires_at?: number | null
  revoked_at?: number | null
}

/** 普通凭证 reveal MCP 配置。 */
export function getTokenMcpConfig(
  ref: string
): Promise<TokenMcpConfig> {
  return workspaceApiGet<TokenMcpConfig>(
    `/api/v1/tokens/${encodeURIComponent(ref)}/mcp-config`
  )
}

/** 租户访问令牌 reveal MCP 配置。 */
export function getTenantTokenMcpConfig(
  ref: string
): Promise<TokenMcpConfig> {
  return workspaceApiGet<TokenMcpConfig>(
    `/api/v1/tenant-access-tokens/${encodeURIComponent(ref)}/mcp-config`
  )
}

/** reveal 查询的 query key，区分 token 类型与 id，便于弹窗按需 invalidate。 */
export function tokenMcpConfigQueryKey(type: string, id: string) {
  return ["token", "mcp-config", type, id] as const
}
