// TaskFilter 描述项目详情页任务表格的过滤条件，与后端 restful query 参数对齐。
// 这些字段会同步到 URL search params，刷新/分享链接均保留。
export type TaskFilter = {
  status?: string
  priority?: string
  assignee?: string
  due_after?: string
  due_before?: string
  due_empty?: string
  assignee_empty?: string
  wait_before?: string
  scheduled_before?: string
  until_before?: string
  tags?: string
  q?: string
  query?: string
  sort?: string
}

export const FILTER_KEYS: Array<keyof TaskFilter> = [
  "status",
  "priority",
  "assignee",
  "due_after",
  "due_before",
  "due_empty",
  "assignee_empty",
  "wait_before",
  "scheduled_before",
  "until_before",
  "tags",
  "q",
  "query",
  "sort",
]

const DIRECT_QUERY_KEYS: Array<keyof TaskFilter> = [
  "status",
  "priority",
  "assignee",
  "due_after",
  "due_before",
  "tags",
  "q",
  "sort",
]

// emptyFilter 判断是否没有任何激活的过滤条件。
export function emptyFilter(filter: TaskFilter): boolean {
  return FILTER_KEYS.every((key) => !filter[key])
}

// activeFilterEntries 返回有值的过滤项，用于渲染活跃 chips。
export function activeFilterEntries(
  filter: TaskFilter
): Array<[keyof TaskFilter, string]> {
  return FILTER_KEYS.filter((key) => !!filter[key]).map((key) => [
    key,
    filter[key] as string,
  ])
}

// filterToTaskQuery 把 filter 编码为追加到 GET /tasks path 的 query string（不含前导 ?）。
// 与后端 restful 参数（status/priority/assignee/due_after/due_before/tags/q）一一对应。
export function filterToTaskQuery(filter: TaskFilter): string {
  const params = new URLSearchParams()
  for (const key of DIRECT_QUERY_KEYS) {
    const value = filter[key]
    if (value) {
      if (key === "assignee" && assigneeValues(value).length > 1) {
        continue
      }
      params.set(key, value)
    }
  }
  for (const expr of advancedFilterExpressions(filter)) {
    params.append("query", expr)
  }
  return params.toString()
}

function advancedFilterExpressions(filter: TaskFilter): string[] {
  const expressions: string[] = []
  if (filter.query) {
    expressions.push(filter.query)
  }
  const assignees = assigneeValues(filter.assignee)
  if (assignees.length > 1) {
    expressions.push(
      `(${assignees.map((assignee) => `assignee:"${escapeQueryValue(assignee)}"`).join(" or ")})`
    )
  }
  if (isTruthy(filter.assignee_empty)) {
    expressions.push("assignee.isnull")
  }
  if (isTruthy(filter.due_empty)) {
    expressions.push("due.isnull")
  }
  if (filter.wait_before) {
    expressions.push(`wait.before:${filter.wait_before}`)
  }
  if (filter.scheduled_before) {
    expressions.push(`scheduled.before:${filter.scheduled_before}`)
  }
  if (filter.until_before) {
    expressions.push(`until.before:${filter.until_before}`)
  }
  return expressions
}

function isTruthy(value: string | undefined): boolean {
  return value === "true" || value === "1" || value === "yes"
}

function assigneeValues(value: string | undefined): string[] {
  return (value ?? "")
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean)
}

function escapeQueryValue(value: string): string {
  return value.replaceAll("\\", "\\\\").replaceAll('"', '\\"')
}
