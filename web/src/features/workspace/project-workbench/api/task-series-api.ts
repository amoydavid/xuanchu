import {
  workspaceApiDelete,
  workspaceApiGet,
  workspaceApiPatch,
  workspaceApiPost,
} from "@/features/workspace/session/workspace-api"

// --- 原生类型（spec §7.7、§7.8、§13.5）---

/** RecurrenceInfo：occurrence 的循环归属。普通任务为 null。 */
export type RecurrenceInfo = {
  role: string
  series_id: string
  series_title?: string
  series_status: string
  rule: string
  recurrence_at: number
  materialization: "projected" | "materialized"
  overrides?: string[]
  until?: number | null
}

/** TaskOccurrenceView：普通任务和循环 occurrence 的统一视图。 */
export type TaskOccurrenceView = {
  id: string
  url: string
  uuid?: string | null
  task_slug?: string | null
  project_seq?: number | null
  workspace_id?: string
  project_id?: string | null
  project?: string | null
  title: string
  description?: string | null
  status: string
  entry?: number | null
  modified?: number | null
  start?: number | null
  end?: number | null
  due?: number | null
  wait?: number | null
  scheduled?: number | null
  until?: number | null
  parent?: string | null
  priority?: string | null
  tags?: string[]
  udas?: Record<string, string>
  assignees?: Array<{
    id: string
    name: string
    display_name: string
    email?: string | null
    external_ids?: Array<{
      provider: string
      user_type?: string
      external_id: string
    }>
  }>
  depends?: string[]
  annotations?: Array<{ id: string; entry: number; description: string }>
  links?: Array<{
    id: string
    type: string
    url: string
    title?: string
    created_at: string
  }>
  recurrence_info?: RecurrenceInfo | null
}

/** TaskViewPage：TaskOccurrenceView 的分页结果。 */
export type TaskViewPage = {
  items: TaskOccurrenceView[]
  total: number
  limit: number
  offset: number
  occurrence_mode?: string
  range?: { start: number; end: number }
}

/** TaskSeriesView：循环任务系列聚合。 */
export type TaskSeriesView = {
  id: string
  url: string
  workspace_id: string
  project_id: string
  series_slug?: string | null
  project_slug?: string
  title: string
  description?: string | null
  status: "active" | "ended" | "stopped"
  recurrence_rule: string
  first_due: number
  until?: number | null
  priority?: string | null
  tags?: string[]
  udas?: Record<string, string>
  assignees?: Array<{
    id: string
    name: string
    display_name: string
    email?: string | null
    external_ids?: Array<{
      provider: string
      user_type?: string
      external_id: string
    }>
  }>
  open_occurrence_count: number
  completed_count: number
  skipped_count: number
  overdue_count: number
  next_recurrence_at?: number | null
  suggested_rule_effective_from?: number | null
  created_by: {
    id: string
    name: string
    display_name: string
    email?: string | null
    external_ids?: Array<{
      provider: string
      user_type?: string
      external_id: string
    }>
  }
  created_at: number
  modified_at: number
  open_occurrences?: TaskOccurrenceView[]
  recent_completed?: TaskOccurrenceView[]
  recent_skipped?: TaskOccurrenceView[]
}

export type TaskSeriesListPage = {
  items: TaskSeriesView[]
  total: number
  limit: number
  offset: number
}

export type TaskSeriesCreateResult = {
  series: TaskSeriesView
  first_occurrence?: TaskOccurrenceView | null
}

// --- 输入类型 ---

export type TaskSeriesCreateInput = {
  title: string
  description?: string | null
  project?: string
  project_id?: string
  recurrence_rule: string
  first_due?: number
  first_due_date?: string
  until?: number
  until_date?: string
  priority?: string
  assignees?: string[]
  tags?: string[]
  udas?: Record<string, string>
}

export type TaskSeriesModifyInput = {
  title?: string
  description?: string | null
  recurrence_rule?: string
  effective_from?: number
  until?: number
  priority?: string
  assignees?: string[]
  tags?: string[]
  udas?: Record<string, string>
  clear?: string[]
}

export type TaskSeriesListInput = {
  workspace?: string
  project?: string
  project_id?: string
  status?: string
  q?: string
  assignee?: string
  sort?: string
  limit?: number
  offset?: number
}

export type TaskSeriesOccurrenceListInput = {
  workspace?: string
  status?: string
  due_after?: string
  due_before?: string
  limit?: number
  offset?: number
}

// --- 路径构造 ---

export function taskSeriesPath(workspace: string, input: TaskSeriesListInput = {}): string {
  const params = new URLSearchParams()
  params.set("workspace", workspace)
  if (input.project) params.set("project", input.project)
  if (input.project_id) params.set("project_id", input.project_id)
  if (input.status) params.set("status", input.status)
  if (input.q) params.set("q", input.q)
  if (input.assignee) params.set("assignee", input.assignee)
  if (input.sort) params.set("sort", input.sort)
  if (input.limit && input.limit > 0) params.set("limit", String(input.limit))
  if (input.offset && input.offset > 0) params.set("offset", String(input.offset))
  return `/api/v1/task-series?${params.toString()}`
}

export function taskSeriesItemPath(workspace: string, seriesRef: string): string {
  return `/api/v1/task-series/${encodeURIComponent(seriesRef)}?workspace=${encodeURIComponent(workspace)}`
}

export function taskSeriesOccurrencesPath(
  workspace: string,
  seriesRef: string,
  input: TaskSeriesOccurrenceListInput = {},
): string {
  const params = new URLSearchParams()
  params.set("workspace", workspace)
  if (input.status) params.set("status", input.status)
  if (input.due_after) params.set("due_after", input.due_after)
  if (input.due_before) params.set("due_before", input.due_before)
  if (input.limit && input.limit > 0) params.set("limit", String(input.limit))
  if (input.offset && input.offset > 0) params.set("offset", String(input.offset))
  return `/api/v1/task-series/${encodeURIComponent(seriesRef)}/occurrences?${params.toString()}`
}

export function taskSeriesSkipPath(workspace: string, seriesRef: string, occurrenceRef: string): string {
  return `/api/v1/task-series/${encodeURIComponent(seriesRef)}/occurrences/${encodeURIComponent(occurrenceRef)}/skip?workspace=${encodeURIComponent(workspace)}`
}

export function tasksRangePath(
  workspace: string,
  input: { due_after?: string; due_before?: string; occurrence_mode?: string; project?: string },
): string {
  const params = new URLSearchParams()
  params.set("workspace", workspace)
  if (input.project) params.set("project", input.project)
  if (input.due_after) params.set("due_after", input.due_after)
  if (input.due_before) params.set("due_before", input.due_before)
  if (input.occurrence_mode) params.set("occurrence_mode", input.occurrence_mode)
  return `/api/v1/tasks?${params.toString()}`
}

// --- API client ---

export function listTaskSeries(workspace: string, input: TaskSeriesListInput = {}): Promise<TaskSeriesListPage> {
  return workspaceApiGet<TaskSeriesListPage>(taskSeriesPath(workspace, input))
}

export function getTaskSeries(workspace: string, seriesRef: string): Promise<TaskSeriesView> {
  return workspaceApiGet<TaskSeriesView>(taskSeriesItemPath(workspace, seriesRef))
}

export function createTaskSeries(workspace: string, input: TaskSeriesCreateInput): Promise<TaskSeriesCreateResult> {
  return workspaceApiPost<TaskSeriesCreateResult>(`/api/v1/task-series?workspace=${encodeURIComponent(workspace)}`, input)
}

export function stopTaskSeries(workspace: string, seriesRef: string, deleteOpen = false): Promise<TaskSeriesView> {
  return workspaceApiDelete<TaskSeriesView>(`${taskSeriesItemPath(workspace, seriesRef)}&delete_open_occurrences=${deleteOpen}`)
}

export function listTaskSeriesOccurrences(
  workspace: string,
  seriesRef: string,
  input: TaskSeriesOccurrenceListInput = {},
): Promise<TaskViewPage> {
  return workspaceApiGet<TaskViewPage>(taskSeriesOccurrencesPath(workspace, seriesRef, input))
}

export function skipTaskSeriesOccurrence(
  workspace: string,
  seriesRef: string,
  occurrenceRef: string,
): Promise<TaskOccurrenceView> {
  return workspaceApiPost<TaskOccurrenceView>(taskSeriesSkipPath(workspace, seriesRef, occurrenceRef))
}

export function queryTaskViews(
  workspace: string,
  input: { due_after?: string; due_before?: string; occurrence_mode?: string; project?: string },
): Promise<TaskViewPage> {
  return workspaceApiGet<TaskViewPage>(tasksRangePath(workspace, input))
}

export function modifyTaskSeries(workspace: string, seriesRef: string, input: TaskSeriesModifyInput): Promise<TaskSeriesView> {
  return workspaceApiPatch<TaskSeriesView>(taskSeriesItemPath(workspace, seriesRef), input)
}
