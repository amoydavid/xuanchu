type TaskReferenceFields = {
  id?: string | null
  uuid?: string | null
  task_slug?: string | null
  recurrence_info?: {
    materialization: "projected" | "materialized"
    recurrence_at: number
  } | null
}

/** 用户看到的任务引用与路由引用必须分离：计划实例不暴露 occurrence_ref。 */
export function taskDisplayRef(
  task: TaskReferenceFields,
  locale: string
): string {
  if (task.task_slug) return task.task_slug
  if (task.recurrence_info?.materialization === "projected") {
    const date = new Intl.DateTimeFormat(locale, {
      month: "2-digit",
      day: "2-digit",
    })
      .format(new Date(task.recurrence_info.recurrence_at * 1000))
      .replaceAll("/", "-")
    return `↻${date}`
  }
  return (task.uuid || task.id || "").slice(0, 8)
}

/** API 与 Router 使用的实际引用。计划实例只能使用稳定 occurrence_ref。 */
export function taskRouteRef(task: TaskReferenceFields): string {
  if (task.task_slug) return task.task_slug
  if (task.recurrence_info?.materialization === "projected") {
    return task.id || ""
  }
  return task.uuid || task.id || ""
}

/** 当前资源可用的用户首选 permalink；计划实例尚无 canonical slug。 */
export function canonicalTaskRouteRef(
  task: TaskReferenceFields
): string | null {
  if (task.recurrence_info?.materialization === "projected") return null
  return task.task_slug || task.uuid || task.id || null
}

export function isCanonicalTaskSlug(value: string): boolean {
  return /^[a-z][a-z0-9]{2,9}-[1-9][0-9]*$/.test(value)
}

export function taskStableCacheRef(task: TaskReferenceFields): string {
  return task.id || task.uuid || task.task_slug || ""
}
