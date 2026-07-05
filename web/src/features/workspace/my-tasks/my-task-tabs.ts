// MyTaskTab 描述「我的任务」顶部预设视图，等价于一组预设 filter。
import type { MyTasksFilter } from "./my-tasks-api"

export type MyTaskTabKey = "all" | "today" | "overdue" | "noDue"

export type MyTaskTab = {
  key: MyTaskTabKey
  // label 由 i18n key 提供：`myTasks.tab.${key}`
}

export const MY_TASK_TABS: MyTaskTab[] = [
  { key: "all" },
  { key: "today" },
  { key: "overdue" },
  { key: "noDue" },
]

// tabFilter 返回某个 tab 对应的 filter 覆盖项（不含 assignee，assignee 由调用方填）。
// 接收 now 参数避免测试依赖当前时间。
export function tabFilter(
  tab: MyTaskTabKey,
  now: Date
): Omit<MyTasksFilter, "assignee"> {
  switch (tab) {
    case "today": {
      // 当地时区当天 23:59:59
      const end = new Date(
        now.getFullYear(),
        now.getMonth(),
        now.getDate(),
        23,
        59,
        59,
        999
      )
      return {
        status: "pending",
        due_before: String(Math.floor(end.getTime() / 1000)),
      }
    }
    case "overdue": {
      return {
        status: "pending",
        due_before: String(Math.floor(now.getTime() / 1000)),
      }
    }
    case "noDue": {
      return {
        status: "pending",
        due_empty: "true",
      }
    }
    case "all":
    default: {
      return {
        status: "pending",
      }
    }
  }
}
