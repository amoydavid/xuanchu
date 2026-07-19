import {
  workspaceApiPost,
} from "@/features/workspace/session/workspace-api"

// ContentReferenceKey 描述一个保留 URI 引用。
export type ContentReferenceKey = {
  type: "user" | "task" | "attachment"
  id: string
}

// ContentReferenceResolution 是 resolve 的单项结果。
export type ContentReferenceResolution =
  | { type: ContentReferenceKey["type"]; id: string; status: "unavailable" }
  | { type: "user"; id: string; status: "resolved"; user: ReferenceUserInfo }
  | { type: "task"; id: string; status: "resolved"; task: ReferenceTaskInfo }
  | { type: "attachment"; id: string; status: "resolved"; attachment: ReferenceAttachmentInfo }

export type ReferenceExternalID = {
  provider: string
  user_type?: string
  external_id: string
}

export type ReferenceUserInfo = {
  id: string
  name: string
  display_name?: string
  email?: string
  external_ids?: ReferenceExternalID[]
}

export type ReferenceTaskInfo = {
  id: string
  title: string
  task_slug?: string
  status?: string
  project?: { id?: string; slug?: string; name?: string }
  url?: string
}

export type ReferenceAttachmentInfo = {
  id: string
  display_name: string
  media_type: string
  size_bytes: number
  inline_capable: boolean
  content_url: string
}

// resolveContentReferences 批量解析引用，保持输入顺序。
//
// 最多 200 个引用；不可读/不存在统一返回 unavailable。
export async function resolveContentReferences(
  references: ContentReferenceKey[]
): Promise<ContentReferenceResolution[]> {
  if (references.length === 0) return []
  const result = await workspaceApiPost<{ results: ContentReferenceResolution[] }>(
    "/api/v1/content-references/resolve",
    { references }
  )
  return result.results
}

// suggestContentReferences 查询用户或任务引用建议。
export async function suggestContentReferences(input: {
  type: "user" | "task"
  query: string
  project?: string
  limit?: number
}): Promise<ContentReferenceResolution[]> {
  const params = new URLSearchParams()
  params.set("type", input.type)
  params.set("q", input.query)
  if (input.project) params.set("project", input.project)
  if (input.limit) params.set("limit", String(input.limit))
  const result = await workspaceApiPost<{ results: ContentReferenceResolution[] }>(
    `/api/v1/content-references/suggestions?${params.toString()}`,
    {}
  )
  return result.results
}
