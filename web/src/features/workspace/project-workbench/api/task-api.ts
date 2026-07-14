import {
  workspaceApiDelete,
  workspaceApiGet,
  workspaceApiPatch,
  workspaceApiPost,
} from "@/features/workspace/session/workspace-api"

import type {
  ProjectWorkbenchTask,
  ProjectWorkbenchTaskLink,
  UserInfo,
} from "./project-api"

export type ProjectTask = ProjectWorkbenchTask

export type TaskCreateInput = {
  title: string
  description?: string | null
  project?: string
  project_id?: string
  priority?: string
  due?: number | null
  due_date?: string
  assignees?: string[]
  depends?: string[]
  wait?: number | null
  wait_date?: string
  scheduled?: number | null
  scheduled_date?: string
  until?: number | null
  until_date?: string
  tags?: string[]
  udas?: Record<string, string>
  parent?: string
}

export type TaskModifyInput = {
  title?: string
  description?: string | null
  clear_description?: boolean
  project?: string
  project_id?: string
  priority?: string | null
  clear_project?: boolean
  clear_priority?: boolean
  due?: number | null
  due_date?: string
  clear_due?: boolean
  wait?: number | null
  wait_date?: string
  clear_wait?: boolean
  scheduled?: number | null
  scheduled_date?: string
  clear_scheduled?: boolean
  until?: number | null
  until_date?: string
  clear_until?: boolean
  assignees?: string[]
  remove_assignees?: string[]
  clear_assignees?: boolean
  depends?: string[]
  clear_depends?: boolean
  tags?: string[]
  remove_tags?: string[]
  udas?: Record<string, string>
  clear_udas?: string[]
}

export type TaskAnnotation = {
  id?: string
  entry?: string
  description: string
}

export type TaskAnnotationPage = {
  annotations: TaskAnnotation[]
  total: number
  offset: number
  limit: number
}

export type TaskAnnotationInput = {
  description: string
}

export type TaskLinkInput = {
  type: string
  url: string
  title?: string
}

export type TaskImportTask = {
  uuid: string
  title: string
  description?: string | null
  status: string
  entry: string
  modified: string
  end?: string | null
  due?: string | null
  project?: string | null
  task_slug?: string | null
  priority?: string | null
  tags?: string[] | null
  start?: string | null
  wait?: string | null
  scheduled?: string | null
  until?: string | null
  annotations?: Array<{ id?: string; entry: string; description: string }> | null
  depends?: string[] | null
  parent?: string | null
  assignees?: Array<
    | string
    | {
        id?: string
        name?: string
        display_name?: string
        email?: string | null
      }
  > | null
  links?: unknown[] | null
  [key: string]: unknown
}

export type TaskImportResult = {
  imported: number
}

function encodeSegment(value: string): string {
  return encodeURIComponent(value)
}

function workspaceQuery(workspaceSlug: string): string {
  return `workspace=${encodeURIComponent(workspaceSlug)}`
}

export function tasksPath(workspaceSlug: string): string {
  return `/api/v1/tasks?${workspaceQuery(workspaceSlug)}`
}

export function taskPath(workspaceSlug: string, taskRef: string): string {
  return `/api/v1/tasks/${encodeSegment(taskRef)}?${workspaceQuery(workspaceSlug)}`
}

export function taskStartPath(workspaceSlug: string, taskRef: string): string {
  return `/api/v1/tasks/${encodeSegment(taskRef)}/start?${workspaceQuery(workspaceSlug)}`
}

export function taskStopPath(workspaceSlug: string, taskRef: string): string {
  return `/api/v1/tasks/${encodeSegment(taskRef)}/stop?${workspaceQuery(workspaceSlug)}`
}

export function taskDonePath(workspaceSlug: string, taskRef: string): string {
  return `/api/v1/tasks/${encodeSegment(taskRef)}/done?${workspaceQuery(workspaceSlug)}`
}

export function taskReopenPath(workspaceSlug: string, taskRef: string): string {
  return `/api/v1/tasks/${encodeSegment(taskRef)}/reopen?${workspaceQuery(workspaceSlug)}`
}

export function taskAnnotationPath(
  workspaceSlug: string,
  taskRef: string
): string {
  return `/api/v1/tasks/${encodeSegment(taskRef)}/annotations?${workspaceQuery(workspaceSlug)}`
}

export function taskAnnotationItemPath(
  workspaceSlug: string,
  taskRef: string,
  annotationID: string
): string {
  return `/api/v1/tasks/${encodeSegment(taskRef)}/annotations/${encodeSegment(annotationID)}?${workspaceQuery(workspaceSlug)}`
}

export function taskLinkPath(workspaceSlug: string, taskRef: string): string {
  return `/api/v1/tasks/${encodeSegment(taskRef)}/links?${workspaceQuery(workspaceSlug)}`
}

export function importTasksPath(
  workspaceSlug: string,
  projectSlug: string
): string {
  return `/api/v1/task-imports?${workspaceQuery(workspaceSlug)}&project=${encodeURIComponent(projectSlug)}`
}

export function taskLinkItemPath(
  workspaceSlug: string,
  taskRef: string,
  linkID: string
): string {
  return `/api/v1/tasks/${encodeSegment(taskRef)}/links/${encodeSegment(linkID)}?${workspaceQuery(workspaceSlug)}`
}

// --- 任务变更历史 ---

export type TaskChangeDisplayValue = {
  raw: unknown
  text: string
}

export type TaskChangeField =
  | "assignees"
  | "tags"
  | "due"
  | "priority"
  | "project"
  | "title"
  | "description"
  | "wait"
  | "scheduled"
  | "until"
  | "depends"
  | "udas"

export type TaskScalarFieldChange = {
  field: Exclude<TaskChangeField, "assignees" | "tags" | "udas">
  kind: "scalar"
  label_key: string
  previous: TaskChangeDisplayValue
  current: TaskChangeDisplayValue
}

export type TaskSetFieldChange = {
  field: "assignees" | "tags" | "depends"
  kind: "set"
  label_key: string
  added: TaskChangeDisplayValue[]
  removed: TaskChangeDisplayValue[]
}

export type TaskUDAEntryChange = {
  name: string
  before?: TaskChangeDisplayValue | null
  after?: TaskChangeDisplayValue | null
}

export type TaskUDAFieldChange = {
  field: "udas"
  kind: "uda"
  label_key: string
  entries: TaskUDAEntryChange[]
}

export type TaskFieldChange =
  | TaskScalarFieldChange
  | TaskSetFieldChange
  | TaskUDAFieldChange

export type TaskAuditEntry = {
  id: number
  actor_type?: string
  actor?: UserInfo | null
  actor_token?: { id: string; name: string; prefix: string } | null
  action: string
  target_type: string
  target_id: string
  payload?: unknown
  changes?: TaskFieldChange[]
  created_at: number
}

export function taskAuditPath(
  workspaceSlug: string,
  taskRef: string
): string {
  return `/api/v1/tasks/${encodeSegment(taskRef)}/audit?${workspaceQuery(workspaceSlug)}`
}

export function createTask(
  workspaceSlug: string,
  input: TaskCreateInput
): Promise<ProjectTask> {
  return workspaceApiPost<ProjectTask>(tasksPath(workspaceSlug), input)
}

export function getTask(
  workspaceSlug: string,
  taskRef: string
): Promise<ProjectTask> {
  return workspaceApiGet<ProjectTask>(taskPath(workspaceSlug, taskRef))
}

export function listTaskChildren(
  workspaceSlug: string,
  taskRef: string,
  includeClosed = false
): Promise<ProjectTask[]> {
  const query = includeClosed
    ? `${workspaceQuery(workspaceSlug)}&include_closed=true`
    : workspaceQuery(workspaceSlug)
  return workspaceApiGet<ProjectTask[]>(
    `/api/v1/tasks/${encodeSegment(taskRef)}/children?${query}`
  )
}

export function modifyTask(
  workspaceSlug: string,
  taskRef: string,
  input: TaskModifyInput
): Promise<ProjectTask> {
  return workspaceApiPatch<ProjectTask>(taskPath(workspaceSlug, taskRef), input)
}

export function startTask(
  workspaceSlug: string,
  taskRef: string
): Promise<ProjectTask> {
  return workspaceApiPost<ProjectTask>(taskStartPath(workspaceSlug, taskRef))
}

export function stopTask(
  workspaceSlug: string,
  taskRef: string
): Promise<ProjectTask> {
  return workspaceApiPost<ProjectTask>(taskStopPath(workspaceSlug, taskRef))
}

export function doneTask(
  workspaceSlug: string,
  taskRef: string
): Promise<ProjectTask> {
  return workspaceApiPost<ProjectTask>(taskDonePath(workspaceSlug, taskRef))
}

export function reopenTask(
  workspaceSlug: string,
  taskRef: string
): Promise<ProjectTask> {
  return workspaceApiPost<ProjectTask>(taskReopenPath(workspaceSlug, taskRef))
}

export function deleteTask(
  workspaceSlug: string,
  taskRef: string
): Promise<ProjectTask> {
  return workspaceApiDelete<ProjectTask>(taskPath(workspaceSlug, taskRef))
}

export function getTaskAnnotations(
  workspaceSlug: string,
  taskRef: string
): Promise<TaskAnnotationPage> {
  return workspaceApiGet<TaskAnnotationPage>(
    taskAnnotationPath(workspaceSlug, taskRef)
  )
}

export function addTaskAnnotation(
  workspaceSlug: string,
  taskRef: string,
  input: TaskAnnotationInput
): Promise<ProjectTask> {
  return workspaceApiPost<ProjectTask>(
    taskAnnotationPath(workspaceSlug, taskRef),
    input
  )
}

export function deleteTaskAnnotation(
  workspaceSlug: string,
  taskRef: string,
  annotationID: string
): Promise<ProjectTask> {
  return workspaceApiDelete<ProjectTask>(
    taskAnnotationItemPath(workspaceSlug, taskRef, annotationID)
  )
}

export function updateTaskAnnotation(
  workspaceSlug: string,
  taskRef: string,
  annotationID: string,
  input: TaskAnnotationInput
): Promise<ProjectTask> {
  return workspaceApiPatch<ProjectTask>(
    taskAnnotationItemPath(workspaceSlug, taskRef, annotationID),
    input
  )
}

export function getTaskLinks(
  workspaceSlug: string,
  taskRef: string
): Promise<ProjectWorkbenchTaskLink[]> {
  return workspaceApiGet<ProjectWorkbenchTaskLink[]>(
    taskLinkPath(workspaceSlug, taskRef)
  )
}

export function addTaskLink(
  workspaceSlug: string,
  taskRef: string,
  input: TaskLinkInput
): Promise<ProjectWorkbenchTaskLink> {
  return workspaceApiPost<ProjectWorkbenchTaskLink>(
    taskLinkPath(workspaceSlug, taskRef),
    input
  )
}

export function deleteTaskLink(
  workspaceSlug: string,
  taskRef: string,
  linkID: string
): Promise<ProjectTask> {
  return workspaceApiDelete<ProjectTask>(
    taskLinkItemPath(workspaceSlug, taskRef, linkID)
  )
}

export function updateTaskLink(
  workspaceSlug: string,
  taskRef: string,
  linkID: string,
  input: TaskLinkInput
): Promise<ProjectWorkbenchTaskLink> {
  return workspaceApiPatch<ProjectWorkbenchTaskLink>(
    taskLinkItemPath(workspaceSlug, taskRef, linkID),
    input
  )
}

export function importTasks(
  workspaceSlug: string,
  projectSlug: string,
  tasks: TaskImportTask[]
): Promise<TaskImportResult> {
  return workspaceApiPost<TaskImportResult>(
    importTasksPath(workspaceSlug, projectSlug),
    { schema: "xuanchu.task-import/v1", tasks }
  )
}

export function getTaskAudit(
  workspaceSlug: string,
  taskRef: string
): Promise<TaskAuditEntry[]> {
  return workspaceApiGet<TaskAuditEntry[]>(
    taskAuditPath(workspaceSlug, taskRef)
  )
}

// Task urgency（紧迫度解释）。
// 后端 GET /api/v1/tasks/{ref}/urgency 返回 { uuid, total, items[] }。
export type TaskUrgencyItem = {
  name: string
  coefficient: number
  contribution: number
  reason: string
}

export type TaskUrgency = {
  uuid: string
  total: number
  items: TaskUrgencyItem[]
}

export function taskUrgencyPath(
  workspaceSlug: string,
  taskRef: string
): string {
  return `/api/v1/tasks/${encodeSegment(taskRef)}/urgency?${workspaceQuery(workspaceSlug)}`
}

export function getTaskUrgency(
  workspaceSlug: string,
  taskRef: string
): Promise<TaskUrgency> {
  return workspaceApiGet<TaskUrgency>(taskUrgencyPath(workspaceSlug, taskRef))
}
