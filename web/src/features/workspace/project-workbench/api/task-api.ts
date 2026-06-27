import {
  workspaceApiDelete,
  workspaceApiGet,
  workspaceApiPatch,
  workspaceApiPost,
} from "@/features/workspace/session/workspace-api"

import type {
  ProjectWorkbenchTask,
  ProjectWorkbenchTaskLink,
} from "./project-api"

export type ProjectTask = ProjectWorkbenchTask

export type TaskCreateInput = {
  title: string
  description?: string | null
  project?: string
  project_id?: string
  priority?: string
  due?: number | null
  assignees?: string[]
  depends?: string[]
  wait?: number | null
  scheduled?: number | null
  until?: number | null
  recur?: string | null
  tags?: string[]
  udas?: Record<string, string>
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
  clear_due?: boolean
  wait?: number | null
  clear_wait?: boolean
  scheduled?: number | null
  clear_scheduled?: boolean
  until?: number | null
  clear_until?: boolean
  assignees?: string[]
  remove_assignees?: string[]
  clear_assignees?: boolean
  depends?: string[]
  clear_depends?: boolean
  recur?: string | null
  clear_recur?: boolean
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

export function taskLinkItemPath(
  workspaceSlug: string,
  taskRef: string,
  linkID: string
): string {
  return `/api/v1/tasks/${encodeSegment(taskRef)}/links/${encodeSegment(linkID)}?${workspaceQuery(workspaceSlug)}`
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
