import {
  workspaceApiDelete,
  workspaceApiGet,
  workspaceApiPatch,
  workspaceApiPost,
  workspaceApiPut,
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
  source_type?: string
  source_id?: string
  source_label?: string
  entry?: number
  content?: string
  action?: string
  event_type?: string
  summary?: string
  created_at?: number
  created_by?: UserInfo & {
    user?: UserInfo
    token?: {
      id?: string
      name?: string
      prefix?: string
    }
  }
  actor?: UserInfo
}

export type ProjectSummaryTaskRef = {
  uuid: string
  task_slug?: string
  title: string
  label: string
}

export type ProjectSummaryWorkloadRow = {
  user?: UserInfo | null
  label: string
  open_count: number
  overdue_count: number
  high_priority_count: number
}

export type ProjectTaskSummary = {
  overdue_count: number
  overdue_refs: ProjectSummaryTaskRef[]
  high_priority_open_count: number
  high_priority_open_refs: ProjectSummaryTaskRef[]
  wait_ready_count: number
  wait_ready_refs: ProjectSummaryTaskRef[]
  unassigned_open_count: number
  unassigned_open_refs: ProjectSummaryTaskRef[]
  workload: ProjectSummaryWorkloadRow[]
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

export function projectTaskSummaryPath(
  workspaceSlug: string,
  projectRef: string
): string {
  return `/api/v1/projects/${encodeSegment(projectRef)}/task-summary?${workspaceQuery(workspaceSlug)}`
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

export function getProjectTaskSummary(
  workspaceSlug: string,
  projectRef: string
): Promise<ProjectTaskSummary> {
  return workspaceApiGet<ProjectTaskSummary>(
    projectTaskSummaryPath(workspaceSlug, projectRef)
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

// Project config（项目级配置项）。后端契约：GET/PUT/DELETE /projects/{ref}/config/{key}。
export function projectConfigPath(
  workspaceSlug: string,
  projectRef: string
): string {
  return `/api/v1/projects/${encodeSegment(projectRef)}/config?${workspaceQuery(workspaceSlug)}`
}

export function projectConfigKeyPath(
  workspaceSlug: string,
  projectRef: string,
  key: string
): string {
  return `/api/v1/projects/${encodeSegment(projectRef)}/config/${encodeSegment(key)}?${workspaceQuery(workspaceSlug)}`
}

export type ProjectConfigEntry = {
  key: string
  value: string
}

// 后端 ProjectConfigList 返回 map[string]string，HTTP 形如 {data: {key: value}}。
// 这里把对象形态归一化成 ProjectConfigEntry[]，按 key 排序保持稳定。
export function listProjectConfig(
  workspaceSlug: string,
  projectRef: string
): Promise<ProjectConfigEntry[]> {
  return workspaceApiGet<Record<string, string>>(
    projectConfigPath(workspaceSlug, projectRef)
  ).then(normalizeProjectConfigEntries)
}

export function normalizeProjectConfigEntries(
  raw: Record<string, string> | ProjectConfigEntry[] | unknown
): ProjectConfigEntry[] {
  if (Array.isArray(raw)) {
    return raw as ProjectConfigEntry[]
  }
  if (raw && typeof raw === "object") {
    const obj = raw as Record<string, string>
    return Object.keys(obj)
      .sort()
      .map((key) => ({ key, value: String(obj[key] ?? "") }))
  }
  return []
}

export function setProjectConfig(
  workspaceSlug: string,
  projectRef: string,
  key: string,
  value: string
): Promise<void> {
  return workspaceApiPut<void>(
    projectConfigKeyPath(workspaceSlug, projectRef, key),
    { value }
  )
}

export function deleteProjectConfig(
  workspaceSlug: string,
  projectRef: string,
  key: string
): Promise<void> {
  return workspaceApiDelete<void>(
    projectConfigKeyPath(workspaceSlug, projectRef, key)
  )
}

// Project annotations（项目备注）。
// 后端契约：POST/GET /projects/{ref}/annotations，DELETE /projects/{ref}/annotations/{id}。
// 后端暂无 PATCH，因此只做新增和删除（spec §3.1：不做删除+重建伪编辑）。
export function projectAnnotationsPath(
  workspaceSlug: string,
  projectRef: string
): string {
  return `/api/v1/projects/${encodeSegment(projectRef)}/annotations?${workspaceQuery(workspaceSlug)}`
}

export function projectAnnotationPath(
  workspaceSlug: string,
  projectRef: string,
  annotationId: string
): string {
  return `/api/v1/projects/${encodeSegment(projectRef)}/annotations/${encodeSegment(annotationId)}?${workspaceQuery(workspaceSlug)}`
}

export function listProjectAnnotations(
  workspaceSlug: string,
  projectRef: string
): Promise<ProjectAnnotationInfo[]> {
  return workspaceApiGet<ProjectAnnotationInfo[]>(
    projectAnnotationsPath(workspaceSlug, projectRef)
  )
}

export function addProjectAnnotation(
  workspaceSlug: string,
  projectRef: string,
  content: string
): Promise<ProjectAnnotationInfo> {
  return workspaceApiPost<ProjectAnnotationInfo>(
    projectAnnotationsPath(workspaceSlug, projectRef),
    { content }
  )
}

export function deleteProjectAnnotation(
  workspaceSlug: string,
  projectRef: string,
  annotationId: string
): Promise<void> {
  return workspaceApiDelete<void>(
    projectAnnotationPath(workspaceSlug, projectRef, annotationId)
  )
}

// project effective config 已迁移到 config-definition-api，这里保留 re-export 方便现有引用。
export {
  listProjectEffectiveConfig,
  projectConfigEffectivePath,
} from "@/features/workspace/config/config-definition-api"
