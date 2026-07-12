// MyTaskTab 描述「我的任务」顶部预设视图（spec §15.8）。
//
// 修订为互斥、可解释的一级预设：
// - incomplete（未完成）：assignee=me 且 status 为 pending 或 waiting
// - today（今天）：open（pending 或 waiting）且 due 在本地今天 [00:00,次日00:00)
// - overdue（逾期）：open 且 due < 今天 00:00
// - noDue（无截止日期）：open 且 due is null
// - completed（已完成）：assignee=me 且 status=completed
//
// 「今天」不混入逾期；所有 open 预设同时包含 pending 和 waiting（不存在 active status）。
import { formatLocalDate } from "@/features/workspace/project-workbench/shared/date-boundary"

import type { MyTasksFilter } from "./my-tasks-api"

export type MyTaskTabKey = "incomplete" | "today" | "overdue" | "noDue" | "completed"

export type MyTaskTab = {
  key: MyTaskTabKey
}

export const MY_TASK_TABS: MyTaskTab[] = [
  { key: "incomplete" },
  { key: "today" },
  { key: "overdue" },
  { key: "noDue" },
  { key: "completed" },
]

// tabFilter 返回某个 tab 对应的 filter 覆盖项（不含 assignee，assignee 由调用方填）。
// 接收 now 参数避免测试依赖当前时间。
export function tabFilter(
  tab: MyTaskTabKey,
  now: Date
): Omit<MyTasksFilter, "assignee"> {
  switch (tab) {
    case "today": {
      // 今天：open（pending 或 waiting）且 due 在本地今天 [00:00, 次日00:00)。
      // 后端 due_before=当天 含当天整天；due_after=当天 从当天 00:00 起。
      // 用 expand 模式展开今天窗口，确保不混入逾期。
      return {
        query: "(status:pending or status:waiting)",
        due_after: formatLocalDate(now),
        due_before: formatLocalDate(now),
      }
    }
    case "overdue": {
      // 逾期：open 且 due < 今天 00:00。
      // 用 materialized 模式（无界历史展开被禁止），due_before=昨天。
      const yesterday = new Date(
        now.getFullYear(),
        now.getMonth(),
        now.getDate() - 1
      )
      return {
        query: "(status:pending or status:waiting)",
        due_before: formatLocalDate(yesterday),
      }
    }
    case "noDue": {
      // 无截止日期：open 且 due is null。
      // projected occurrence 不会出现（它总有 due=recurrence_at）；
      // 允许清除 due 的 materialized occurrence 会出现。
      return {
        query: "(status:pending or status:waiting)",
        due_empty: "true",
      }
    }
    case "completed": {
      // 已完成：assignee=me 且 status=completed。
      // 每次 completed occurrence 独立显示；series ended/stopped 不算任务完成。
      return {
        status: "completed",
      }
    }
    case "incomplete":
    default: {
      // 未完成：assignee=me 且 status 为 pending 或 waiting。
      // 普通任务 + 已进入执行期或已物化 occurrence；不展开无限未来。
      return {
        query: "(status:pending or status:waiting)",
      }
    }
  }
}
