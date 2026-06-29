// Admin token 管理 API 类型，对齐后端 tokenResponse。
// 状态派生与过期换算复用普通 console 的 helper，避免重复。

export type AdminTokenRow = {
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

export type AdminTenantAccessTokenRow = {
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

/**
 * 修改请求体。语义与后端对齐：
 * - undefined（缺省）= 不修改
 * - []（空数组）= 清空（仅 scopes）
 */
export type AdminTokenModifyInput = {
  name?: string
  scopes?: string[]
  expires_in_seconds?: number | null
}

export type AdminTenantAccessTokenModifyInput = AdminTokenModifyInput & {
  projects?: string[]
}
