import {
	workspaceApiGet,
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

type ContentReferenceSuggestionResponse =
  | { type: "user"; user?: ReferenceUserInfo }
  | { type: "task"; task?: ReferenceTaskInfo }

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
}, options?: { signal?: AbortSignal }): Promise<ContentReferenceResolution[]> {
  const params = new URLSearchParams()
  params.set("type", input.type)
  params.set("q", input.query)
  if (input.project) params.set("project", input.project)
  if (input.limit) params.set("limit", String(input.limit))
  const result = await workspaceApiGet<{ results: ContentReferenceSuggestionResponse[] }>(
    `/api/v1/content-references/suggestions?${params.toString()}`,
    options
  )
  // suggestion 与 resolve 的 HTTP 响应形状不同：前者已经保证目标可读，
  // 不带 status/id 顶层字段。统一成编辑器消费的 resolved 形状，避免菜单把
  // 所有候选误判为 unavailable。
  return result.results.flatMap((suggestion): ContentReferenceResolution[] => {
    if (suggestion.type === "user" && suggestion.user) {
      return [{
        type: "user",
        id: suggestion.user.id,
        status: "resolved",
        user: suggestion.user,
      }]
    }
    if (suggestion.type === "task" && suggestion.task) {
      return [{
        type: "task",
        id: suggestion.task.id,
        status: "resolved",
        task: suggestion.task,
      }]
    }
    return []
  })
}
