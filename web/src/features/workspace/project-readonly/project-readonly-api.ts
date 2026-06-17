import { workspaceApiGet } from "@/features/workspace/session/workspace-api"

export type ProjectReadonlyProject = {
  id: string
  workspace_id: string
  slug: string
  name: string
  description?: string
  status: string
  task_count?: number
  created_at?: number
  modified_at?: number
  archived_at?: number | null
}

export type ProjectReadonlyAssignee = {
  user_id?: string
  id?: string
  name?: string
  email?: string
}

export type ProjectReadonlyTask = {
  uuid: string
  task_slug?: string
  description: string
  status: string
  project?: string
  project_id?: string
  priority?: string | null
  due?: number | null
  entry?: string
  modified?: string
  recur?: string
  start?: number | null
  wait?: number | null
  scheduled?: number | null
  until?: number | null
  parent?: string
  annotations?: Array<{ id?: string; entry?: string; description: string }>
  depends?: string[]
  assignees?: ProjectReadonlyAssignee[]
  tags?: string[]
  links?: ProjectReadonlyTaskLink[]
  // UDAs 在后端平铺为顶层字段，此处用索引签名容纳任意自定义字段。
  [key: string]: unknown
}

export type ProjectReadonlyTaskLink = {
  id: string
  type: string
  url: string
  title?: string
  created_at?: string
  created_by?: { id?: string; name?: string; email?: string }
}

// AnnotationPage 是 GET /tasks/{ref}/annotations 的分页响应。
export type AnnotationPage = {
  annotations: Array<{ id?: string; entry?: string; description: string }>
  total: number
  offset: number
  limit: number
}

export type ProjectReadonlyTimelineEntry = {
  id?: string
  action?: string
  event_type?: string
  summary?: string
  created_at?: number
  created_by?: { name?: string }
  actor?: { name?: string }
}

export function projectReadonlyProjectPath(
  workspaceSlug: string,
  projectSlug: string
) {
  return `/api/v1/projects/${encodeURIComponent(projectSlug)}?workspace=${encodeURIComponent(workspaceSlug)}`
}

export function projectReadonlyTasksPath(
  workspaceSlug: string,
  projectSlug: string,
  filterQuery = ""
) {
  const base = `/api/v1/tasks?workspace=${encodeURIComponent(workspaceSlug)}&project=${encodeURIComponent(projectSlug)}&limit=200`
  return filterQuery ? `${base}&${filterQuery}` : base
}

export function projectReadonlyTaskPath(workspaceSlug: string, taskRef: string) {
  return `/api/v1/tasks/${encodeURIComponent(taskRef)}?workspace=${encodeURIComponent(workspaceSlug)}`
}

export function projectReadonlyTimelinePath(
  workspaceSlug: string,
  projectSlug: string
) {
  return `/api/v1/projects/${encodeURIComponent(projectSlug)}/timeline?workspace=${encodeURIComponent(workspaceSlug)}&limit=20`
}

export function projectReadonlyAnnotationsPath(
  workspaceSlug: string,
  taskRef: string,
  offset: number,
  limit: number
) {
  return `/api/v1/tasks/${encodeURIComponent(taskRef)}/annotations?workspace=${encodeURIComponent(workspaceSlug)}&offset=${offset}&limit=${limit}`
}

export function getProjectReadonlyProject(
  workspaceSlug: string,
  projectSlug: string
) {
  return workspaceApiGet<ProjectReadonlyProject>(
    projectReadonlyProjectPath(workspaceSlug, projectSlug)
  )
}

export function getProjectReadonlyTasks(
  workspaceSlug: string,
  projectSlug: string,
  filterQuery = ""
) {
  return workspaceApiGet<ProjectReadonlyTask[]>(
    projectReadonlyTasksPath(workspaceSlug, projectSlug, filterQuery)
  )
}

export function getProjectReadonlyTask(workspaceSlug: string, taskRef: string) {
  return workspaceApiGet<ProjectReadonlyTask>(
    projectReadonlyTaskPath(workspaceSlug, taskRef)
  )
}

export function getProjectReadonlyTimeline(
  workspaceSlug: string,
  projectSlug: string
) {
  return workspaceApiGet<ProjectReadonlyTimelineEntry[]>(
    projectReadonlyTimelinePath(workspaceSlug, projectSlug)
  )
}

export function getProjectReadonlyAnnotations(
  workspaceSlug: string,
  taskRef: string,
  offset: number,
  limit: number
) {
  return workspaceApiGet<AnnotationPage>(
    projectReadonlyAnnotationsPath(workspaceSlug, taskRef, offset, limit)
  )
}
