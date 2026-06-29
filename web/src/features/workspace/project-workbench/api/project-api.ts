import {
  workspaceApiGet,
  workspaceApiPatch,
  workspaceApiPost,
} from "@/features/workspace/session/workspace-api"

export type ProjectStatus = "planning" | "active" | "archived" | "cancelled"

export type UserInfo = {
  id: string
  name: string
  display_name?: string
  email?: string | null
  external_ids?: Array<{
    provider: string
    external_id: string
  }>
}

export type ProjectAnnotationInfo = {
  id: string
  project_id: string
  entry: number
  content: string
  created_by: UserInfo
  created_at: number
}

export type ProjectWorkbenchProject = {
  id: string
  workspace_id: string
  slug: string
  name: string
  description?: string
  status: ProjectStatus | string
  task_count: number
  pending_count: number
  completed_count: number
  created_at: number
  modified_at: number
  archived_at?: number | null
  recent_annotations?: ProjectAnnotationInfo[]
}

export type ProjectWorkbenchTaskRef = {
  uuid: string
  title: string
  task_slug?: string
}

export type ProjectWorkbenchAssignee = {
  user_id?: string
  id?: string
  name?: string
  display_name?: string
  email?: string | null
}

export type ProjectWorkbenchTaskLink = {
  id: string
  type: string
  url: string
  title?: string
  created_at?: string
  created_by?: UserInfo
}

export type ProjectWorkbenchTask = {
  uuid: string
  task_slug?: string
  title: string
  description?: string | null
  status: string
  project?: string | null
  project_id?: string
  priority?: string | null
  due?: string | number | null
  entry?: string
  modified?: string
  end?: string | null
  recur?: string | null
  start?: string | number | null
  wait?: string | number | null
  scheduled?: string | number | null
  until?: string | number | null
  parent?: string | null
  parent_info?: ProjectWorkbenchTaskRef
  blocked_by_info?: ProjectWorkbenchTaskRef[]
  annotations?: Array<{ id?: string; entry?: string; description: string }>
  depends?: string[]
  depends_info?: ProjectWorkbenchTaskRef[]
  assignees?: ProjectWorkbenchAssignee[]
  tags?: string[]
  links?: ProjectWorkbenchTaskLink[]
  [key: string]: unknown
}

export type ProjectTimelineEntry = {
  id?: string
  action?: string
  event_type?: string
  summary?: string
  created_at?: number
  created_by?: UserInfo
  actor?: UserInfo
}

export type ProjectCreateInput = {
  slug: string
  name: string
  description?: string
}

export type ProjectModifyInput = {
  slug?: string
  name?: string
  description?: string
}

export type ProjectTaskFilterParams =
  | URLSearchParams
  | Record<string, string | number | null | undefined>

function encodeSegment(value: string): string {
  return encodeURIComponent(value)
}

function workspaceQuery(workspaceSlug: string): string {
  return `workspace=${encodeURIComponent(workspaceSlug)}`
}

function appendQuery(path: string, query?: URLSearchParams): string {
  if (!query) {
    return path
  }
  const rawQuery = query.toString()
  return rawQuery ? `${path}&${rawQuery}` : path
}

function filterParamsToQuery(filters?: ProjectTaskFilterParams): URLSearchParams {
  if (!filters) {
    return new URLSearchParams()
  }
  if (filters instanceof URLSearchParams) {
    return filters
  }
  const query = new URLSearchParams()
  for (const [key, value] of Object.entries(filters)) {
    if (value !== undefined && value !== null && value !== "") {
      query.set(key, String(value))
    }
  }
  return query
}

export function projectsPath(
  workspaceSlug: string,
  status?: ProjectStatus | "all" | string
): string {
  const base = `/api/v1/projects?${workspaceQuery(workspaceSlug)}`
  if (!status) {
    return base
  }
  const query = new URLSearchParams({ status })
  return appendQuery(base, query)
}

export function projectPath(workspaceSlug: string, projectRef: string): string {
  return `/api/v1/projects/${encodeSegment(projectRef)}?${workspaceQuery(workspaceSlug)}`
}

export function projectTasksPath(
  workspaceSlug: string,
  projectRef: string,
  filters?: ProjectTaskFilterParams
): string {
  const base = `/api/v1/tasks?${workspaceQuery(workspaceSlug)}&project=${encodeURIComponent(projectRef)}&limit=200`
  return appendQuery(base, filterParamsToQuery(filters))
}

export function projectTimelinePath(
  workspaceSlug: string,
  projectRef: string
): string {
  return `/api/v1/projects/${encodeSegment(projectRef)}/timeline?${workspaceQuery(workspaceSlug)}&limit=20`
}

export function projectTransitionPath(
  workspaceSlug: string,
  projectRef: string
): string {
  return `/api/v1/projects/${encodeSegment(projectRef)}/transition?${workspaceQuery(workspaceSlug)}`
}

export function getProjects(
  workspaceSlug: string,
  status?: ProjectStatus | "all" | string
): Promise<ProjectWorkbenchProject[]> {
  return workspaceApiGet<ProjectWorkbenchProject[]>(
    projectsPath(workspaceSlug, status)
  )
}

export function getProject(
  workspaceSlug: string,
  projectRef: string
): Promise<ProjectWorkbenchProject> {
  return workspaceApiGet<ProjectWorkbenchProject>(
    projectPath(workspaceSlug, projectRef)
  )
}

export function getProjectTasks(
  workspaceSlug: string,
  projectRef: string,
  filters?: ProjectTaskFilterParams
): Promise<ProjectWorkbenchTask[]> {
  return workspaceApiGet<ProjectWorkbenchTask[]>(
    projectTasksPath(workspaceSlug, projectRef, filters)
  )
}

export function getProjectTimeline(
  workspaceSlug: string,
  projectRef: string
): Promise<ProjectTimelineEntry[]> {
  return workspaceApiGet<ProjectTimelineEntry[]>(
    projectTimelinePath(workspaceSlug, projectRef)
  )
}

export function createProject(
  workspaceSlug: string,
  input: ProjectCreateInput
): Promise<ProjectWorkbenchProject> {
  return workspaceApiPost<ProjectWorkbenchProject>(
    projectsPath(workspaceSlug),
    input
  )
}

export function modifyProject(
  workspaceSlug: string,
  projectRef: string,
  input: ProjectModifyInput
): Promise<ProjectWorkbenchProject> {
  return workspaceApiPatch<ProjectWorkbenchProject>(
    projectPath(workspaceSlug, projectRef),
    input
  )
}

export function transitionProject(
  workspaceSlug: string,
  projectRef: string,
  status: ProjectStatus | string
): Promise<ProjectWorkbenchProject> {
  return workspaceApiPost<ProjectWorkbenchProject>(
    projectTransitionPath(workspaceSlug, projectRef),
    { status }
  )
}
