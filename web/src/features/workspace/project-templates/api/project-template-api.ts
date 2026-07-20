import type { QueryClient } from "@tanstack/react-query"

import {
  workspaceApiGet,
  workspaceApiPatch,
  workspaceApiPost,
} from "@/features/workspace/session/workspace-api"

export type ExternalIDInfo = {
  provider: string
  user_type?: string
  external_id: string
}

export type UserInfo = {
  id: string
  name: string
  display_name?: string
  email?: string | null
  external_ids?: ExternalIDInfo[]
}

export type TokenActorInfo = {
  id: string
  name: string
  prefix?: string
}

export type ActorInfo = {
  type: string
  user?: UserInfo
  token?: TokenActorInfo
}

export type Page<T> = {
  items: T[]
  total: number
  limit: number
  offset: number
}

export type ComponentCounts = {
  configs: number
  tasks: number
  series: number
  automations: number
}

export type ProjectTemplateSnapshotSummary = {
  id: string
  version: number
  hash: string
  source_project_id: string
  counts: ComponentCounts
  required_secret_keys: string[]
  created_by: ActorInfo
  created_at: number
}

export type ProjectTemplateSummary = {
  id: string
  key: string
  name: string
  description: string
  status: "active" | "archived"
  current_snapshot?: ProjectTemplateSnapshotSummary
  created_by: ActorInfo
  created_at: number
  modified_at: number
  archived_at?: number
}

export type RelativeLocalTime = {
  day_offset: number
  local_time: string
}

export type UDABlueprint = { raw: string; type?: string }

export type TaskDates = {
  due?: RelativeLocalTime
  wait?: RelativeLocalTime
  scheduled?: RelativeLocalTime
  until?: RelativeLocalTime
}

export type ProjectTemplateConfig = {
  key: string
  mode: string
  value?: string
}

export type ProjectTemplateTask = {
  ref: string
  title: string
  description?: string
  priority?: string
  tags: string[]
  assignees: UserInfo[]
  udas: Record<string, UDABlueprint>
  dates: TaskDates
  parent_ref?: string
  depends_refs: string[]
  links: Array<{ type: string; url: string; title?: string }>
}

export type ProjectTemplateSeries = {
  ref: string
  title: string
  recurrence_rule: string
  description?: string
  priority?: string
  tags: string[]
  assignees: UserInfo[]
  udas: Record<string, UDABlueprint>
  first_due: RelativeLocalTime
  until?: RelativeLocalTime
}

export type ProjectTemplateAutomation = {
  ref: string
  name: string
  description?: string
  trigger_type: string
  trigger_config: {
    schedule_type?: string
    schedule_value?: string
    timezone?: string
    event_type?: string
  }
  condition: {
    task_filter?: string
    max_tasks?: number
    only_added_assignees?: boolean
  }
  action: {
    protocol: string
    base_url_config_key: string
    api_key_config_key: string
    model_config_key: string
    allowed_hosts_config_key?: string
    model_override?: string
    temperature: number
    max_attempts?: number
    attach_metadata?: boolean
  }
  context: { include: string[] }
  instruction_template: string
  system_prompt?: string
}

export type ProjectTemplateSnapshot = {
  project: { description: string }
  configs: ProjectTemplateConfig[]
  tasks: ProjectTemplateTask[]
  series: ProjectTemplateSeries[]
  automations: ProjectTemplateAutomation[]
}

export type ProjectTemplateDetail = {
  template: ProjectTemplateSummary
  snapshot?: ProjectTemplateSnapshot
  versions: ProjectTemplateSnapshotSummary[]
}

export type TaskCandidate = {
  ref: string
  project_id: string
  project_seq?: number
  series_id?: string
  title: string
  status: string
  priority?: string
  due?: number
  assignees: UserInfo[]
  warning_count: number
}

export type SeriesCandidate = {
  ref: string
  project_id: string
  project_seq?: number
  title: string
  status: string
  recurrence_rule: string
  first_due: number
  assignees: UserInfo[]
  created_by: UserInfo
  warning_count: number
}

export type ConfigCandidate = {
  ref: string
  key: string
  label: string
  mode: string
  value_type: string
  warning_count: number
}

export type AutomationCandidate = {
  ref: string
  id: string
  project_id: string
  name: string
  description: string
  enabled: boolean
  trigger_type: string
  created_by: UserInfo
  created_at: number
  warning_count: number
}

export type TaskCandidateFilter = {
  q?: string
  status?: string
  priority?: string
  assignees?: string[]
  tags?: string[]
  due_after?: string
  due_before?: string
  query?: string
  sort?: string
}

export type SeriesCandidateFilter = {
  q?: string
  status?: string
  assignee?: string
  sort?: string
}

export type ConfigCandidateFilter = { q?: string; mode?: string }
export type AutomationCandidateFilter = {
  q?: string
  status?: string
  trigger_type?: string
}

export type CandidatePageOptions<TFilter> = TFilter & {
  limit?: number
  offset?: number
  refs?: string[]
}

export type CandidateKind = "task" | "series" | "config" | "automation"

export type ResolveCandidateSelectionInput = {
  kind: CandidateKind
  task?: TaskCandidateFilter
  series?: SeriesCandidateFilter
  config?: ConfigCandidateFilter
  automation?: AutomationCandidateFilter
}

export type ResolvedCandidateSelection = {
  refs: string[]
  total: number
  source_hash: string
}

export type CaptureSelection = {
  config_keys: string[]
  task_refs: string[]
  series_refs: string[]
  automation_rule_ids: string[]
}

export type TaskRelationResolution = {
  source_task_ref: string
  relation: string
  target_task_ref: string
}

export type ContentRefResolution = {
  source_kind: string
  source_ref: string
  target_task_ref: string
}

export type CaptureResolution = {
  drop_parent_task_refs?: string[]
  drop_depends?: TaskRelationResolution[]
  drop_content_task_refs?: ContentRefResolution[]
  task_date_overrides?: Array<{
    source_task_ref: string
    field: string
    value: RelativeLocalTime | null
  }>
  series_schedule_overrides?: Array<{
    source_series_ref: string
    first_due: RelativeLocalTime
    until: RelativeLocalTime | null
    clear_until: boolean
  }>
}

export type CaptureInput = {
  source_project: string
  anchor_date: string
  selection: CaptureSelection
  resolution?: CaptureResolution
  expected_source_hash?: string
}

export type CreateProjectTemplateInput = {
  key: string
  name: string
  description?: string
  capture: CaptureInput
}

export type ModifyProjectTemplateInput = {
  name?: string
  description?: string
}

export type CaptureIssue = {
  code: string
  source_kind?: string
  source_ref?: string
  target_ref?: string
  user?: UserInfo
  relation?: string
  field?: string
  message: string
}

export type CapturePreview = {
  selection: CaptureSelection
  source_hash: string
  counts: ComponentCounts
  blocking_issues: CaptureIssue[]
  warnings: CaptureIssue[]
  snapshot: ProjectTemplateSnapshot | null
}

export type ProjectTemplateIssue = {
  code: string
  severity: string
  component?: string
  source_ref?: string
  target_ref?: string
  relation?: string
  field?: string
  message: string
}

export type InstantiateInput = {
  snapshot_id?: string
  expected_snapshot_hash: string
  project_slug: string
  project_name: string
  description?: string
  start_date: string
  secret_inputs?: Record<string, string>
  assignee_replacements?: Record<string, string | null>
}

export type SecretResolution = {
  key: string
  resolved_from: string
}

export type AssigneeIssue = {
  user: UserInfo
  affected_refs: string[]
  resolution: string
}

export type InstantiatePreview = {
  template: ProjectTemplateSummary
  snapshot: ProjectTemplateSnapshotSummary
  project: {
    slug: string
    name: string
    description: string
    start_date: string
  }
  counts: ComponentCounts
  secret_resolutions: SecretResolution[]
  assignee_issues: AssigneeIssue[]
  issues: ProjectTemplateIssue[]
  warnings: ProjectTemplateIssue[]
}

export type InstantiateResult = {
  project: {
    id: string
    slug: string
    name: string
    description?: string
    status: string
    [key: string]: unknown
  }
  counts: ComponentCounts
}

export type ProjectTemplateListOptions = {
  status: string
  q: string
  limit: number
  offset: number
}

export const projectTemplateListQueryKey = (
  workspaceSlug: string,
  status: string,
  q: string,
  limit: number,
  offset: number
) => ["project-templates", workspaceSlug, status, q, limit, offset] as const

export const projectTemplateDetailQueryKey = (
  workspaceSlug: string,
  ref: string,
  snapshotID?: string
) => ["project-template", workspaceSlug, ref, snapshotID] as const

export function listProjectTemplates(
  workspaceSlug: string,
  options: ProjectTemplateListOptions
): Promise<Page<ProjectTemplateSummary>> {
  return workspaceApiGet(projectTemplateListPath(workspaceSlug, options))
}

export function getProjectTemplate(
  workspaceSlug: string,
  ref: string,
  snapshotID?: string
): Promise<ProjectTemplateDetail> {
  return workspaceApiGet(
    projectTemplateDetailPath(workspaceSlug, ref, snapshotID)
  )
}

export function listProjectTemplateTaskCandidates(
  workspaceSlug: string,
  projectRef: string,
  options: CandidatePageOptions<TaskCandidateFilter>
): Promise<Page<TaskCandidate>> {
  return workspaceApiGet(
    candidatePath(workspaceSlug, projectRef, "tasks", options)
  )
}

export function listProjectTemplateSeriesCandidates(
  workspaceSlug: string,
  projectRef: string,
  options: CandidatePageOptions<SeriesCandidateFilter>
): Promise<Page<SeriesCandidate>> {
  return workspaceApiGet(
    candidatePath(workspaceSlug, projectRef, "series", options)
  )
}

export function listProjectTemplateConfigCandidates(
  workspaceSlug: string,
  projectRef: string,
  options: CandidatePageOptions<ConfigCandidateFilter>
): Promise<Page<ConfigCandidate>> {
  return workspaceApiGet(
    candidatePath(workspaceSlug, projectRef, "configs", options)
  )
}

export function listProjectTemplateAutomationCandidates(
  workspaceSlug: string,
  projectRef: string,
  options: CandidatePageOptions<AutomationCandidateFilter>
): Promise<Page<AutomationCandidate>> {
  return workspaceApiGet(
    candidatePath(workspaceSlug, projectRef, "automations", options)
  )
}

export function resolveProjectTemplateCandidateSelection(
  workspaceSlug: string,
  projectRef: string,
  input: ResolveCandidateSelectionInput
): Promise<ResolvedCandidateSelection> {
  return workspaceApiPost(
    withWorkspace(
      `${projectCandidateBase(projectRef)}/resolve-selection`,
      workspaceSlug
    ),
    input
  )
}

export function previewProjectTemplateCapture(
  workspaceSlug: string,
  input: CaptureInput
): Promise<CapturePreview> {
  return workspaceApiPost(
    withWorkspace("/api/v1/project-templates/capture-preview", workspaceSlug),
    input
  )
}

export function createProjectTemplate(
  workspaceSlug: string,
  input: CreateProjectTemplateInput
): Promise<ProjectTemplateDetail> {
  return workspaceApiPost(
    withWorkspace("/api/v1/project-templates", workspaceSlug),
    input
  )
}

export function previewProjectTemplateSnapshotCapture(
  workspaceSlug: string,
  ref: string,
  input: CaptureInput
): Promise<CapturePreview> {
  return workspaceApiPost(
    withWorkspace(
      `/api/v1/project-templates/${encodeURIComponent(ref)}/snapshots/capture-preview`,
      workspaceSlug
    ),
    input
  )
}

export function appendProjectTemplateSnapshot(
  workspaceSlug: string,
  ref: string,
  input: CaptureInput
): Promise<ProjectTemplateDetail> {
  return workspaceApiPost(
    withWorkspace(
      `/api/v1/project-templates/${encodeURIComponent(ref)}/snapshots`,
      workspaceSlug
    ),
    input
  )
}

export function modifyProjectTemplate(
  workspaceSlug: string,
  ref: string,
  input: ModifyProjectTemplateInput
): Promise<ProjectTemplateDetail> {
  return workspaceApiPatch(
    withWorkspace(
      `/api/v1/project-templates/${encodeURIComponent(ref)}`,
      workspaceSlug
    ),
    input
  )
}

export function archiveProjectTemplate(
  workspaceSlug: string,
  ref: string
): Promise<ProjectTemplateDetail> {
  return workspaceApiPost(
    withWorkspace(
      `/api/v1/project-templates/${encodeURIComponent(ref)}/archive`,
      workspaceSlug
    )
  )
}

export function reactivateProjectTemplate(
  workspaceSlug: string,
  ref: string
): Promise<ProjectTemplateDetail> {
  return workspaceApiPost(
    withWorkspace(
      `/api/v1/project-templates/${encodeURIComponent(ref)}/reactivate`,
      workspaceSlug
    )
  )
}

export function previewProjectTemplateInstantiation(
  workspaceSlug: string,
  ref: string,
  input: InstantiateInput
): Promise<InstantiatePreview> {
  return workspaceApiPost(
    withWorkspace(
      `/api/v1/project-templates/${encodeURIComponent(ref)}/instantiate-preview`,
      workspaceSlug
    ),
    input
  )
}

export function instantiateProjectTemplate(
  workspaceSlug: string,
  ref: string,
  input: InstantiateInput
): Promise<InstantiateResult> {
  return workspaceApiPost(
    withWorkspace(
      `/api/v1/project-templates/${encodeURIComponent(ref)}/instantiate`,
      workspaceSlug
    ),
    input
  )
}

export type ProjectTemplateMutation = {
  kind:
    | "create"
    | "append"
    | "modify"
    | "archive"
    | "reactivate"
    | "instantiate"
  ref?: string
}

export async function invalidateProjectTemplateMutation(
  queryClient: QueryClient,
  workspaceSlug: string,
  mutation: ProjectTemplateMutation
) {
  await queryClient.invalidateQueries({
    queryKey: ["project-templates", workspaceSlug],
  })
  if (mutation.ref) {
    await queryClient.invalidateQueries({
      queryKey: ["project-template", workspaceSlug, mutation.ref],
    })
  }
  if (mutation.kind === "instantiate") {
    await queryClient.invalidateQueries({
      queryKey: ["projects", workspaceSlug],
    })
  }
}

function projectTemplateListPath(
  workspaceSlug: string,
  options: ProjectTemplateListOptions
) {
  const params = workspaceParams(workspaceSlug)
  params.set("status", options.status)
  params.set("q", options.q)
  params.set("limit", String(options.limit))
  params.set("offset", String(options.offset))
  return `/api/v1/project-templates?${params.toString()}`
}

function projectTemplateDetailPath(
  workspaceSlug: string,
  ref: string,
  snapshotID?: string
) {
  const params = workspaceParams(workspaceSlug)
  if (snapshotID) params.set("snapshot_id", snapshotID)
  return `/api/v1/project-templates/${encodeURIComponent(ref)}?${params.toString()}`
}

function projectCandidateBase(projectRef: string) {
  return `/api/v1/projects/${encodeURIComponent(projectRef)}/template-candidates`
}

function candidatePath(
  workspaceSlug: string,
  projectRef: string,
  kind: string,
  options: object
) {
  const params = workspaceParams(workspaceSlug)
  for (const [key, raw] of Object.entries(options as Record<string, unknown>)) {
    if (raw === undefined || raw === "") continue
    if (Array.isArray(raw)) {
      for (const value of raw)
        params.append(
          key === "assignees" ? "assignee" : key === "refs" ? "ref" : key,
          String(value)
        )
      continue
    }
    params.set(key, String(raw))
  }
  return `/api/v1/projects/${encodeURIComponent(projectRef)}/template-candidates/${kind}?${params.toString()}`
}

function workspaceParams(workspaceSlug: string) {
  return new URLSearchParams({ workspace: workspaceSlug })
}

function withWorkspace(path: string, workspaceSlug: string) {
  return `${path}?${workspaceParams(workspaceSlug).toString()}`
}
