// Audit API helper。
// 后端 GET /api/v1/audit 当前只支持 limit 与 project/project_id 服务端筛选；
// actor/action/time range 是前端在当前结果上的二次筛选（spec §2.2）。
export type AuditRow = {
  id: number
  actor_type?: string
  actor?: { id: string; name: string; display_name?: string; email?: string | null } | null
  action: string
  target_type: string
  target_id: string
  project_id?: string | null
  payload?: unknown
  created_at: number
}

export type AuditListParams = {
  project?: string
  limit?: number
}

export function auditPath(params: AuditListParams): string {
  const query = new URLSearchParams()
  query.set("limit", String(params.limit ?? 50))
  if (params.project) {
    query.set("project", params.project)
  }
  return `/api/v1/audit?${query.toString()}`
}
