// TaskFilter 描述项目详情页任务表格的过滤条件，与后端 restful query 参数对齐。
// 这些字段会同步到 URL search params，刷新/分享链接均保留。
export type TaskFilter = {
  status?: string
  priority?: string
  assignee?: string
  due_after?: string
  due_before?: string
  tags?: string
  q?: string
}

export const FILTER_KEYS: Array<keyof TaskFilter> = [
  "status",
  "priority",
  "assignee",
  "due_after",
  "due_before",
  "tags",
  "q",
]

// emptyFilter 判断是否没有任何激活的过滤条件。
export function emptyFilter(filter: TaskFilter): boolean {
  return FILTER_KEYS.every((key) => !filter[key])
}

// activeFilterEntries 返回有值的过滤项，用于渲染活跃 chips。
export function activeFilterEntries(filter: TaskFilter): Array<[keyof TaskFilter, string]> {
  return FILTER_KEYS.filter((key) => !!filter[key]).map((key) => [key, filter[key] as string])
}

// filterToTaskQuery 把 filter 编码为追加到 GET /tasks path 的 query string（不含前导 ?）。
// 与后端 restful 参数（status/priority/assignee/due_after/due_before/tags/q）一一对应。
export function filterToTaskQuery(filter: TaskFilter): string {
  const params = new URLSearchParams()
  for (const key of FILTER_KEYS) {
    const value = filter[key]
    if (value) {
      params.set(key, value)
    }
  }
  return params.toString()
}
