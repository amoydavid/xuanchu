// MyTaskTab 描述「我的任务」顶部预设视图，等价于一组预设 filter。
import { formatLocalDate } from "@/features/workspace/project-workbench/shared/date-boundary"

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
// 日期参数统一使用 YYYY-MM-DD（后端 GET /api/v1/tasks 的 due_before/due_after 只接受该格式，
// 并按本地日界解释：due_before=当天 表示「含当天整天」）。
export function tabFilter(
  tab: MyTaskTabKey,
  now: Date
): Omit<MyTasksFilter, "assignee"> {
  switch (tab) {
    case "today": {
      // 今日到期：due 在今天结束前（含今天整天）。
      return {
        status: "pending",
        due_before: formatLocalDate(now),
      }
    }
    case "overdue": {
      // 逾期：due 在今天之前（昨天及更早）。今天到期的任务不算逾期，
      // 这样「今日到期」和「逾期」不重叠。
      const yesterday = new Date(
        now.getFullYear(),
        now.getMonth(),
        now.getDate() - 1
      )
      return {
        status: "pending",
        due_before: formatLocalDate(yesterday),
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
