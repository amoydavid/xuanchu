// MyTasksFilter 描述「我的任务」跨项目聚合视图的过滤条件。
// 与项目内 TaskFilter 不同：这里没有 project 上下文，assignee 固定为当前用户。
export type MyTasksFilter = {
  assignee: string
  status?: string
  project?: string
  priority?: string
  due_after?: string
  due_before?: string
  due_empty?: string
  q?: string
  query?: string
  sort?: string
}

// myTasksPath 把 filter 编码为 GET /api/v1/tasks 的完整路径（含前导 /）。
// 复用现有 RESTful 参数：assignee/status/project/priority/due_after/due_before/q/sort，
// 以及 query 表达式：due.isnull、assignee.isnull。
export function myTasksPath(workspaceSlug: string, filter: MyTasksFilter): string {
  const params = new URLSearchParams()
  params.set("workspace", workspaceSlug)

  if (filter.assignee) {
    params.set("assignee", filter.assignee)
  }

  const directKeys: Array<keyof MyTasksFilter> = [
    "status",
    "project",
    "priority",
    "due_after",
    "due_before",
    "q",
    "sort",
  ]
  for (const key of directKeys) {
    const value = filter[key]
    if (value) {
      params.set(key, value)
    }
  }

  // query 表达式（assignee.isnull / due.isnull）放在 direct 参数之后，
  // 保持稳定的参数顺序，便于测试和缓存。
  const expressions: string[] = []
  if (!filter.assignee) {
    expressions.push("assignee.isnull")
  }
  if (filter.due_empty === "true" || filter.due_empty === "1") {
    expressions.push("due.isnull")
  }
  for (const expr of expressions) {
    params.append("query", expr)
  }

  params.set("limit", "200")

  return `/api/v1/tasks?${params.toString()}`
}
