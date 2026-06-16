// Token 相关类型与 API 契约，对齐后端 tokenResponse / createdTokenResponse。

/** 列表/详情返回的 token 行，对齐后端 tokenResponse。 */
export type TokenRow = {
  id: string
  prefix: string
  name: string
  type: string
  user: { id: string; name: string; email?: string | null }
  workspace_ids: string[]
  project_ids: string[]
  scopes: string[]
  created_at: number
  expires_at?: number | null
  revoked_at?: number | null
  last_used_at?: number | null
}

/** 创建 token 成功响应（含一次性明文）。 */
export type CreatedTokenRow = {
  token: string
} & TokenRow

/** 表单值，创建/编辑共用。 */
export type TokenFormValues = {
  name: string
  type: string // 'pat' | 'agent'
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

/** token 状态派生。 */
export type TokenStatus = "active" | "expired" | "revoked"

export function deriveTokenStatus(row: TokenRow, nowUnix?: number): TokenStatus {
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
