import type { ProjectWorkbenchTask } from "../api/project-api"
import type { TaskImportTask } from "../api/task-api"
import type { WorkspaceMemberCandidate } from "../api/users-api"

export type TaskImportRow = Record<string, unknown>

export type TaskImportIssueCode =
  | "assignee_not_member"
  | "dependency_not_found"
  | "external_assignee_unverified"
  | "invalid_import_id"
  | "invalid_assignee_reference"
  | "invalid_title"
  | "invalid_priority"
  | "invalid_status"
  | "project_rebound"

export type TaskImportIssue = {
  code: TaskImportIssueCode
  message: string
  ref?: string
  row?: number
  taskTitle?: string
}

export type TaskImportPayload = {
  tasks: TaskImportTask[]
  warnings: TaskImportIssue[]
}

export type TaskImportBuildOptions = {
  nowISO?: string
  projectSlug: string
}

export type TaskImportPreflightOptions = {
  currentProjectSlug: string
  existingTasks: ProjectWorkbenchTask[]
  members: WorkspaceMemberCandidate[]
}

export type TaskImportPreflightResult = {
  blockers: TaskImportIssue[]
  warnings: TaskImportIssue[]
}

const CORE_FIELDS = new Set([
  "id",
  "import_id",
  "uuid",
  "title",
  "description",
  "status",
  "entry",
  "modified",
  "end",
  "due",
  "project",
  "task_slug",
  "priority",
  "tags",
  "start",
  "wait",
  "scheduled",
  "until",
  "annotations",
  "depends",
  "blocked_by",
  "recur",
  "parent",
  "mask",
  "imask",
  "assignees",
  "assignee_display_names",
  "assignee_names",
  "assignee_emails",
  "links",
])

const RESERVED_FIELDS = new Set([
  "project_id",
  "project_seq",
  "depends_info",
  "parent_info",
  "blocked_by_info",
])

const VALID_STATUSES = new Set([
  "pending",
  "completed",
  "deleted",
  "waiting",
  "recurring",
])

const VALID_PRIORITIES = new Set(["H", "M", "L"])
const IMPORT_ID_META = "__xuanchu_import_id"
const IMPORT_ID_INVALID_META = "__xuanchu_import_id_invalid"

export function parseTaskImportJSON(
  text: string,
  options: TaskImportBuildOptions
): TaskImportPayload {
  const parsed = JSON.parse(text) as unknown
  const rows = Array.isArray(parsed)
    ? parsed
    : isRecord(parsed) && Array.isArray(parsed.tasks)
      ? parsed.tasks
      : null
  if (!rows) {
    throw new Error("tasks array is required")
  }
  return normalizeRawTasks(rows, options)
}

export function rowsToTaskImportPayload(
  rows: TaskImportRow[],
  options: TaskImportBuildOptions
): TaskImportPayload {
  const tasks = rows
    .map((row) => normalizeSpreadsheetRow(row))
    .filter((row) => Object.keys(row).length > 0)
  return normalizeRawTasks(tasks, options)
}

export function preflightTaskImport(
  tasks: TaskImportTask[],
  options: TaskImportPreflightOptions
): TaskImportPreflightResult {
  const blockers: TaskImportIssue[] = []
  const warnings: TaskImportIssue[] = []
  const knownTaskRefs = new Set<string>()
  for (const task of tasks) {
    if (task.uuid) {
      knownTaskRefs.add(task.uuid)
    }
  }
  for (const task of options.existingTasks) {
    if (task.uuid) {
      knownTaskRefs.add(task.uuid)
    }
  }
  const importIDs = new Map<string, number>()
  tasks.forEach((task, index) => {
    const row = index + 1
    const invalidRef = taskMetaString(task, IMPORT_ID_INVALID_META)
    if (invalidRef) {
      blockers.push(
        issue("invalid_import_id", `导入 ID ${invalidRef} 必须是字符串`, {
          ref: invalidRef,
          row,
          taskTitle: task.title,
        })
      )
      return
    }
    const importID = taskMetaString(task, IMPORT_ID_META)
    if (!importID) {
      return
    }
    const firstRow = importIDs.get(importID)
    if (firstRow !== undefined) {
      blockers.push(
        issue("invalid_import_id", `导入 ID ${importID} 重复`, {
          ref: importID,
          row,
          taskTitle: task.title,
        })
      )
      blockers.push(
        issue("invalid_import_id", `导入 ID ${importID} 重复`, {
          ref: importID,
          row: firstRow,
        })
      )
      return
    }
    importIDs.set(importID, row)
  })

  const memberRefs = memberReferenceSet(options.members)
  tasks.forEach((task, index) => {
    const row = index + 1
    if (!task.title.trim()) {
      blockers.push(issue("invalid_title", "任务标题不能为空", { row }))
    }
    if (!VALID_STATUSES.has(task.status)) {
      blockers.push(
        issue("invalid_status", `任务状态 ${task.status} 不受支持`, {
          ref: task.status,
          row,
          taskTitle: task.title,
        })
      )
    }
    if (task.priority && !VALID_PRIORITIES.has(task.priority)) {
      blockers.push(
        issue("invalid_priority", `任务优先级 ${task.priority} 不受支持`, {
          ref: task.priority,
          row,
          taskTitle: task.title,
        })
      )
    }
    for (const dep of task.depends ?? []) {
      const ref = String(dep).trim()
      if (ref && !knownTaskRefs.has(ref)) {
        blockers.push(
          issue("dependency_not_found", `依赖任务 ${ref} 不在当前项目或导入文件中`, {
            ref,
            row,
            taskTitle: task.title,
          })
        )
      }
    }
    for (const assignee of assigneePreflightRefs(task.assignees ?? [])) {
      if (!assignee.ref) {
        blockers.push(
          issue(
            "invalid_assignee_reference",
            assignee.display_name
              ? `指派人 ${assignee.display_name} 只有 display_name，缺少 name、email 或 id`
              : "指派人缺少 name、email 或 id",
            {
              ref: assignee.display_name,
              row,
              taskTitle: task.title,
            }
          )
        )
        continue
      }
      const ref = assignee.ref
      if (ref.includes(":")) {
        warnings.push(
          issue("external_assignee_unverified", `外部身份 ${ref} 将由服务端验证`, {
            ref,
            row,
            taskTitle: task.title,
          })
        )
        continue
      }
      if (!memberRefs.has(ref.toLowerCase())) {
        blockers.push(
          issue("assignee_not_member", `指派人 ${ref} 不是当前 workspace 成员`, {
            ref,
            row,
            taskTitle: task.title,
          })
        )
      }
    }
    if (task.project !== options.currentProjectSlug) {
      warnings.push(
        issue("project_rebound", `任务将导入到当前项目 ${options.currentProjectSlug}`, {
          ref: String(task.project ?? ""),
          row,
          taskTitle: task.title,
        })
      )
    }
  })
  return { blockers: uniqueIssues(blockers), warnings: uniqueIssues(warnings) }
}

export function buildTaskImportTemplateRows(): TaskImportRow[] {
  return [
    {
      id: "task-1",
      title: "示例任务",
      description: "可使用 Markdown 记录详细说明",
      status: "pending",
      priority: "M",
      tags: "docs,import",
      assignees: "alice, bob@example.com",
      assignee_display_names: "张三, 李四",
      assignee_emails: "alice@example.com, bob@example.com",
      blocked_by: "",
      due: "2026-07-01",
      wait: "",
      scheduled: "",
      until: "",
      start: "",
      end: "",
      recur: "",
      parent: "",
      task_slug: "",
      annotations: "",
      links: "",
      "uda.estimate": "3",
    },
  ]
}

function normalizeRawTasks(
  rows: unknown[],
  options: TaskImportBuildOptions
): TaskImportPayload {
  const nowISO = options.nowISO ?? new Date().toISOString()
  const warnings: TaskImportIssue[] = []
  const normalizedRows = rows.map((row) => (isRecord(row) ? row : {}))
  const tasks = normalizedRows.map((row, index) => {
    const originalProject = stringOrNull(row.project)
    if (originalProject && originalProject !== options.projectSlug) {
      warnings.push(
        issue("project_rebound", `任务将导入到当前项目 ${options.projectSlug}`, {
          ref: originalProject,
          row: index + 1,
          taskTitle: stringValue(row.title),
        })
      )
    }
    return baseTask(row, options.projectSlug, nowISO)
  })
  applyBlockedByReferences(tasks, normalizedRows)
  return { tasks, warnings: uniqueIssues(warnings) }
}

function baseTask(
  row: Record<string, unknown>,
  projectSlug: string,
  nowISO: string
): TaskImportTask {
  const task: TaskImportTask = {
    uuid: nonEmptyString(row.uuid) ?? newUUID(),
    title: stringValue(row.title).trim(),
    status: nonEmptyString(row.status) ?? "pending",
    entry: normalizeDateTime(row.entry) ?? nowISO,
    modified: normalizeDateTime(row.modified) ?? nowISO,
    project: projectSlug,
  }
  const importID = importRef(row)
  if (importID) {
    defineTaskMeta(task, IMPORT_ID_META, importID)
  }
  const invalidImportID = invalidImportRef(row)
  if (invalidImportID) {
    defineTaskMeta(task, IMPORT_ID_INVALID_META, invalidImportID)
  }
  assignOptionalString(row, task, "description")
  assignOptionalString(row, task, "task_slug")
  assignOptionalString(row, task, "recur")
  assignOptionalString(row, task, "parent")
  assignOptionalString(row, task, "mask")
  assignOptionalInt(row, task, "imask")
  assignOptionalDate(row, task, "end")
  assignOptionalDate(row, task, "due")
  assignOptionalDate(row, task, "start")
  assignOptionalDate(row, task, "wait")
  assignOptionalDate(row, task, "scheduled")
  assignOptionalDate(row, task, "until")
  const priority = nonEmptyString(row.priority)
  if (priority) {
    task.priority = priority
  }
  const tags = stringList(row.tags)
  if (tags) {
    task.tags = tags
  }
  const depends = stringList(row.blocked_by) ?? stringList(row.depends)
  if (depends) {
    task.depends = depends
  }
  const assignees = assigneeListFromRow(row)
  if (assignees) {
    task.assignees = assignees
  }
  if (Array.isArray(row.annotations)) {
    task.annotations = row.annotations
      .filter(isRecord)
      .map((annotation) => ({
        ...(nonEmptyString(annotation.id) ? { id: nonEmptyString(annotation.id) } : {}),
        description: stringValue(annotation.description),
        entry: normalizeDateTime(annotation.entry) ?? task.entry,
      }))
  }
  if (Array.isArray(row.links)) {
    task.links = row.links
  }
  for (const [key, value] of Object.entries(row)) {
    if (CORE_FIELDS.has(key) || RESERVED_FIELDS.has(key)) {
      continue
    }
    const udaName = key.startsWith("uda.") ? key.slice("uda.".length) : key
    if (!udaName) {
      continue
    }
    const raw = valueToUDAString(value)
    if (raw !== null) {
      task[udaName] = raw
    }
  }
  return task
}

function normalizeSpreadsheetRow(row: TaskImportRow): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const [rawKey, rawValue] of Object.entries(row)) {
    const key = rawKey.trim()
    if (!key || rawValue === null || rawValue === undefined || rawValue === "") {
      continue
    }
    out[key] = rawValue
  }
  return out
}

function applyBlockedByReferences(
  tasks: TaskImportTask[],
  rows: Array<Record<string, unknown>>
) {
  const importIDs = new Map<string, string>()
  rows.forEach((row, index) => {
    const ref = importRef(row)
    if (ref && tasks[index]?.uuid) {
      importIDs.set(ref, tasks[index].uuid)
    }
  })

  tasks.forEach((task) => {
    if (!task.depends) {
      return
    }
    task.depends = task.depends.map((ref) => {
      const mapped = importIDs.get(String(ref).trim())
      return mapped ?? ref
    })
  })
}

function importRef(row: Record<string, unknown>): string | undefined {
  return stringImportRef(row.import_id) ?? stringImportRef(row.id)
}

function invalidImportRef(row: Record<string, unknown>): string | undefined {
  const value = "import_id" in row ? row.import_id : row.id
  if (value === null || value === undefined || value === "") {
    return undefined
  }
  return typeof value === "string" ? undefined : String(value)
}

function stringImportRef(value: unknown): string | undefined {
  if (typeof value !== "string") {
    return undefined
  }
  return nonEmptyString(value)
}

function defineTaskMeta(task: TaskImportTask, key: string, value: string) {
  Object.defineProperty(task, key, {
    configurable: true,
    enumerable: false,
    value,
  })
}

function taskMetaString(task: TaskImportTask, key: string): string | undefined {
  const value = (task as Record<string, unknown>)[key]
  return typeof value === "string" ? value : undefined
}

function memberReferenceSet(members: WorkspaceMemberCandidate[]): Set<string> {
  const refs = new Set<string>()
  for (const member of members) {
    refs.add(member.id.toLowerCase())
    refs.add(member.name.toLowerCase())
    if (member.email) {
      refs.add(member.email.toLowerCase())
    }
  }
  return refs
}

type AssigneePreflightRef = {
  ref?: string
  display_name?: string
}

function assigneePreflightRefs(
  assignees: NonNullable<TaskImportTask["assignees"]>
): AssigneePreflightRef[] {
  const refs: AssigneePreflightRef[] = []
  for (const assignee of assignees) {
    if (typeof assignee === "string") {
      refs.push({ ref: assignee.trim() })
      continue
    }
    const ref =
      assignee.id ??
      assignee.email ??
      assignee.name ??
      ""
    refs.push({
      ref: ref.trim() || undefined,
      display_name: assignee.display_name?.trim() || undefined,
    })
  }
  return refs.filter((item) => item.ref || item.display_name)
}

function assigneeListFromRow(
  row: Record<string, unknown>
): TaskImportTask["assignees"] | undefined {
  const assignees = assigneeList(row.assignees)
  const displayNames =
    splitList(row.assignee_display_names) ?? splitList(row.assignee_names)
  const emails = splitList(row.assignee_emails)
  if (!displayNames && !emails) {
    return assignees
  }
  const maxLength = Math.max(
    assignees?.length ?? 0,
    displayNames?.length ?? 0,
    emails?.length ?? 0
  )
  if (maxLength === 0) {
    return undefined
  }
  const out: NonNullable<TaskImportTask["assignees"]> = []
  for (let index = 0; index < maxLength; index += 1) {
    const base = assignees?.[index]
    const displayName = displayNames?.[index]
    const email = emails?.[index]
    if (typeof base === "object" && base !== null) {
      out.push({
        ...base,
        ...(displayName ? { display_name: displayName } : {}),
        ...(email ? { email } : {}),
      })
      continue
    }
    const baseRef = typeof base === "string" ? base.trim() : ""
    if (!baseRef && !displayName && !email) {
      continue
    }
    const item: {
      name?: string
      display_name?: string
      email?: string
    } = {}
    if (baseRef) {
      if (baseRef.includes("@") && !email) {
        item.email = baseRef
      } else {
        item.name = baseRef
      }
    }
    if (displayName) {
      item.display_name = displayName
    }
    if (email) {
      item.email = email
    }
    out.push(item)
  }
  return out.length > 0 ? out : undefined
}

function assigneeList(value: unknown): TaskImportTask["assignees"] | undefined {
  if (Array.isArray(value)) {
    const values = value
      .map((item) => {
        if (typeof item === "string") {
          return item.trim()
        }
        if (isRecord(item)) {
          return item
        }
        return ""
      })
      .filter(Boolean) as NonNullable<TaskImportTask["assignees"]>
    return values.length > 0 ? values : []
  }
  const values = splitList(value)
  return values ? values : undefined
}

function stringList(value: unknown): string[] | undefined {
  if (Array.isArray(value)) {
    return value.map(String).map((item) => item.trim()).filter(Boolean)
  }
  return splitList(value)
}

function splitList(value: unknown): string[] | undefined {
  const raw = nonEmptyString(value)
  if (!raw) {
    return undefined
  }
  return raw
    .split(/[,\n]/)
    .map((item) => item.trim())
    .filter(Boolean)
}

function assignOptionalString(
  row: Record<string, unknown>,
  task: TaskImportTask,
  key: keyof TaskImportTask
) {
  if (!(key in row)) {
    return
  }
  const value = stringOrNull(row[key as string])
  task[key] = value as never
}

function assignOptionalDate(
  row: Record<string, unknown>,
  task: TaskImportTask,
  key: keyof TaskImportTask
) {
  if (!(key in row)) {
    return
  }
  const value = normalizeDateTime(row[key as string])
  task[key] = (value ?? null) as never
}

function assignOptionalInt(
  row: Record<string, unknown>,
  task: TaskImportTask,
  key: keyof TaskImportTask
) {
  if (!(key in row)) {
    return
  }
  const value = Number(row[key as string])
  task[key] = (Number.isFinite(value) ? Math.trunc(value) : null) as never
}

function normalizeDateTime(value: unknown): string | null {
  if (value === null || value === undefined || value === "") {
    return null
  }
  if (value instanceof Date) {
    return value.toISOString()
  }
  if (typeof value === "number" && Number.isFinite(value)) {
    return new Date(value).toISOString()
  }
  const raw = String(value).trim()
  if (!raw) {
    return null
  }
  if (/^\d{4}-\d{2}-\d{2}$/.test(raw)) {
    return `${raw}T00:00:00.000Z`
  }
  const parsed = new Date(raw)
  if (Number.isNaN(parsed.getTime())) {
    return raw
  }
  return parsed.toISOString()
}

function valueToUDAString(value: unknown): string | null {
  if (value === null || value === undefined || value === "") {
    return null
  }
  if (typeof value === "string") {
    return value
  }
  if (typeof value === "number" || typeof value === "boolean") {
    return String(value)
  }
  return JSON.stringify(value)
}

function stringOrNull(value: unknown): string | null {
  if (value === null) {
    return null
  }
  const text = nonEmptyString(value)
  return text ?? null
}

function stringValue(value: unknown): string {
  if (value === null || value === undefined) {
    return ""
  }
  return String(value)
}

function nonEmptyString(value: unknown): string | undefined {
  if (value === null || value === undefined) {
    return undefined
  }
  const text = String(value).trim()
  return text ? text : undefined
}

function newUUID(): string {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID()
  }
  return `task-${Date.now()}-${Math.random().toString(16).slice(2)}`
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

function issue(
  code: TaskImportIssueCode,
  message: string,
  extra: Omit<TaskImportIssue, "code" | "message"> = {}
): TaskImportIssue {
  return { code, message, ...extra }
}

function uniqueIssues(issues: TaskImportIssue[]): TaskImportIssue[] {
  const seen = new Set<string>()
  const out: TaskImportIssue[] = []
  for (const item of issues) {
    const key = [item.code, item.ref, item.row, item.taskTitle].join("\u0000")
    if (seen.has(key)) {
      continue
    }
    seen.add(key)
    out.push(item)
  }
  return out
}
