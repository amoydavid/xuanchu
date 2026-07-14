import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import type { ReactNode } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import {
  deleteTask,
  doneTask,
  getTask,
  getTaskAudit,
  modifyTask,
  startTask,
  stopTask,
} from "../api/task-api"
import { TaskDetailPage } from "./task-detail-page"

const navigateMock = vi.fn()

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, ...props }: { children: ReactNode }) => (
    <a {...props}>{children}</a>
  ),
  useNavigate: () => navigateMock,
}))

vi.mock("../api/task-series-api", () => ({
  skipTaskSeriesOccurrence: vi.fn(),
}))

vi.mock("@/features/workspace/session/useMe", () => ({
  useMe: () => ({
    data: {
      effective_role: "member",
      token: { scopes: ["task:write"], type: "pat" },
    },
  }),
}))

vi.mock("../api/task-api", async () => {
  const actual =
    await vi.importActual<typeof import("../api/task-api")>("../api/task-api")
  return {
    ...actual,
    deleteTask: vi.fn(),
    doneTask: vi.fn(),
    getTask: vi.fn(),
    getTaskAudit: vi.fn(),
    modifyTask: vi.fn(),
    startTask: vi.fn(),
    stopTask: vi.fn(),
  }
})

function makeWrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
  }
}

function makeQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
}

function task(overrides: Record<string, unknown> = {}) {
  return {
    uuid: "task-1",
    task_slug: "ag-23",
    title: "写投放日报",
    description: "整理素材表现",
    status: "pending",
    project: "agentapi",
    priority: "H",
    tags: ["ads"],
    annotations: [{ id: "note-1", description: "需要素材截图" }],
    links: [{ id: "link-1", type: "spec", url: "https://example.com" }],
    ...overrides,
  }
}

function renderPage(projectClosed = false) {
  render(
    <TaskDetailPage
      projectClosed={projectClosed}
      projectSlug="agentapi"
      taskRef="ag-23"
      workspaceSlug="acme"
    />,
    { wrapper: makeWrapper(makeQueryClient()) }
  )
}

describe("TaskDetailPage", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18n.changeLanguage("zh-CN")
    vi.mocked(getTask).mockResolvedValue(task())
    vi.mocked(getTaskAudit).mockResolvedValue([])
    vi.mocked(modifyTask).mockResolvedValue(task())
    vi.mocked(startTask).mockResolvedValue(task({ start: 1_900_000_000 }))
    vi.mocked(stopTask).mockResolvedValue(task({ start: null }))
    vi.mocked(doneTask).mockResolvedValue(task({ status: "completed" }))
    vi.mocked(deleteTask).mockResolvedValue(task({ status: "deleted" }))
  })

  it("saves the title inline", async () => {
    renderPage()

    await screen.findByText("写投放日报")
    await userEvent.click(screen.getByRole("button", { name: "任务标题" }))
    await userEvent.clear(screen.getByLabelText("任务标题"))
    await userEvent.type(screen.getByLabelText("任务标题"), "写周报{Enter}")

    expect(modifyTask).toHaveBeenCalledWith("acme", "ag-23", {
      title: "写周报",
    })
  })

  it("shows the complete description and edits it from a dialog", async () => {
    renderPage()

    await screen.findByText("整理素材表现")
    expect(screen.getByText("整理素材表现")).toBeTruthy()

    await userEvent.click(screen.getByRole("button", { name: "编辑描述" }))
    expect(screen.getByRole("dialog", { name: "编辑任务描述" })).toBeTruthy()
    await userEvent.clear(screen.getByLabelText("任务描述"))
    await userEvent.keyboard("{Control>}{Enter}{/Control}")

    expect(modifyTask).toHaveBeenCalledWith("acme", "ag-23", {
      clear_description: true,
    })
  })

  it("renders markdown description and linked breadcrumbs", async () => {
    vi.mocked(getTask).mockResolvedValue(
      task({
        description: "# 复盘\n\n- 素材\n- 预算",
      })
    )

    renderPage()

    expect(await screen.findByRole("heading", { name: "复盘" })).toBeTruthy()
    expect(
      screen.getByRole("heading", { level: 1, name: "写投放日报" })
    ).toBeTruthy()
    expect(screen.getByText("素材")).toBeTruthy()
    expect(screen.getByText("预算")).toBeTruthy()
    expect(
      screen.getByRole("link", { name: "acme" }).getAttribute("href")
    ).toBe("/projects")
    expect(
      screen.getByRole("link", { name: "agentapi" }).getAttribute("href")
    ).toBe("/workspaces/acme/projects/agentapi")
    expect(document.title).toBe("ag-23 · 写投放日报")
    expect(
      document.querySelectorAll('[class*="md:grid-cols-[minmax(0,1fr)_280px]"]')
    ).toHaveLength(1)
  })

  it("renders localized status and places description above annotations", async () => {
    renderPage()

    await screen.findByText("写投放日报")
    expect(screen.getAllByText("待处理").length).toBeGreaterThan(0)
    expect(screen.queryByText("pending")).toBeNull()

    const descriptionHeading = screen.getByRole("heading", { name: "描述" })
    const annotationsHeading = screen.getByRole("heading", { name: /注解/ })
    expect(
      descriptionHeading.compareDocumentPosition(annotationsHeading) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
    expect(screen.getByRole("button", { name: "编辑描述" })).toBeTruthy()
  })

  it("runs start and done actions and renders the returned completed state", async () => {
    renderPage()

    await screen.findByText("写投放日报")
    await userEvent.click(screen.getByRole("button", { name: "开始" }))
    expect(startTask).toHaveBeenCalledWith("acme", "ag-23")

    await userEvent.click(screen.getByRole("button", { name: "完成任务" }))
    expect(doneTask).toHaveBeenCalledWith("acme", "ag-23")
    expect(
      await screen.findByRole("button", { name: "重新打开任务" })
    ).toBeTruthy()
  })

  it("deletes a pending ordinary task after confirmation", async () => {
    renderPage()

    await screen.findByText("写投放日报")

    expect(screen.queryByRole("button", { name: "删除" })).toBeNull()
    await userEvent.click(
      screen.getByRole("button", { name: "更多操作 ag-23" })
    )
    await userEvent.click(screen.getByRole("menuitem", { name: "删除任务" }))
    expect(screen.getByText("确认删除任务")).toBeTruthy()
    await userEvent.click(screen.getByRole("button", { name: "删除任务" }))
    expect(deleteTask).toHaveBeenCalledWith("acme", "ag-23")
  })

  it("localizes the occurrence banner and occurrence actions in English", async () => {
    await i18n.changeLanguage("en-US")
    vi.mocked(getTask).mockResolvedValue(
      task({
        recurrence_info: {
          role: "occurrence",
          series_id: "series-1",
          series_title: "每日投放巡检",
          series_status: "active",
          rule: "daily",
          recurrence_at: 1_783_036_800,
          materialization: "materialized",
        },
      })
    )
    vi.mocked(deleteTask).mockResolvedValue(
      task({
        status: "deleted",
        recurrence_info: {
          role: "occurrence",
          series_id: "series-1",
          series_status: "active",
          rule: "daily",
          recurrence_at: 1_783_036_800,
          materialization: "materialized",
        },
      }) as never
    )

    renderPage()

    expect(await screen.findByText("Recurring task · Daily")).toBeTruthy()
    expect(screen.getByText("Recurring task: 每日投放巡检")).toBeTruthy()
    expect(screen.getByText(/This occurrence:/)).toBeTruthy()
    expect(
      screen.getByRole("button", { name: "Complete this occurrence" })
    ).toBeTruthy()
    expect(
      screen.queryByRole("button", { name: "Skip this occurrence" })
    ).toBeNull()
    await userEvent.click(
      screen.getByRole("button", { name: "More actions ag-23" })
    )
    await userEvent.click(
      screen.getByRole("menuitem", { name: "Skip this occurrence" })
    )
    expect(screen.getByText(/Skip the .* occurrence\?/)).toBeTruthy()
    expect(
      screen.getByText(
        "This occurrence will be marked as skipped without affecting future occurrences."
      )
    ).toBeTruthy()
    await userEvent.click(
      within(screen.getByRole("alertdialog")).getByRole("button", {
        name: "Skip this occurrence",
      })
    )
    expect(deleteTask).toHaveBeenCalledWith("acme", "ag-23")
    expect(screen.getByRole("status").textContent).toContain(
      "Skipped the July 3, 2026 occurrence"
    )
    expect(screen.queryByText(/循环任务|完成本次|跳过本次/)).toBeNull()
  })

  it("keeps a completed occurrence on its slug and announces the occurrence date", async () => {
    const occurrence = {
      role: "occurrence" as const,
      series_id: "series-1",
      series_status: "active" as const,
      rule: "daily",
      recurrence_at: 1_783_036_800,
      materialization: "materialized" as const,
    }
    vi.mocked(getTask).mockResolvedValue(task({ recurrence_info: occurrence }))
    vi.mocked(doneTask).mockResolvedValue(
      task({ status: "completed", recurrence_info: occurrence }) as never
    )

    renderPage()

    await userEvent.click(
      await screen.findByRole("button", { name: "完成本次" })
    )
    expect(
      await screen.findByRole("button", { name: "重新打开本次" })
    ).toBeTruthy()
    expect(screen.getByRole("status").textContent).toContain(
      "已完成 2026年7月3日这一次"
    )
    expect(navigateMock).not.toHaveBeenCalled()
  })

  it("renders a deleted occurrence as a read-only skipped occurrence", async () => {
    vi.mocked(getTask).mockResolvedValue(
      task({
        status: "deleted",
        recurrence_info: {
          role: "occurrence",
          series_id: "series-1",
          series_status: "stopped",
          rule: "daily",
          recurrence_at: 1_783_036_800,
          materialization: "materialized",
        },
      })
    )

    renderPage()

    expect(await screen.findByText("已跳过")).toBeTruthy()
    expect(screen.queryByRole("button", { name: "开始本次" })).toBeNull()
    expect(screen.queryByRole("button", { name: "完成本次" })).toBeNull()
    expect(screen.queryByRole("button", { name: "跳过本次" })).toBeNull()

    await userEvent.click(
      screen.getByRole("button", { name: "更多操作 ag-23" })
    )
    expect(screen.getByRole("menuitem", { name: "查看循环任务" })).toBeTruthy()
    expect(screen.getByRole("menuitem", { name: "复制本次链接" })).toBeTruthy()
    expect(screen.queryByRole("menuitem", { name: "跳过本次" })).toBeNull()
  })

  it("allows adding a child to a projected occurrence", async () => {
    vi.mocked(getTask).mockResolvedValue(
      task({
        id: "occ:series-1:1783036800",
        uuid: undefined,
        task_slug: undefined,
        recurrence_info: {
          role: "occurrence",
          series_id: "series-1",
          series_status: "active",
          rule: "daily",
          recurrence_at: 1_783_036_800,
          materialization: "projected",
        },
      })
    )

    render(
      <TaskDetailPage
        projectSlug="agentapi"
        taskRef="occ:series-1:1783036800"
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    expect(
      (await screen.findAllByRole("button", { name: "添加子任务" })).length
    ).toBeGreaterThan(0)
  })

  it("presents a projected occurrence as a planned instance without internal ids", async () => {
    vi.mocked(getTask).mockResolvedValue(
      task({
        id: "occ:series-1:1784476799",
        uuid: undefined,
        task_slug: undefined,
        recurrence_info: {
          role: "occurrence",
          series_id: "series-1",
          series_status: "stopped",
          rule: "daily",
          recurrence_at: 1_784_476_799,
          materialization: "projected",
        },
      })
    )

    render(
      <TaskDetailPage
        projectSlug="agentapi"
        taskRef="occ:series-1:1784476799"
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    expect(await screen.findByText("↻07-19")).toBeTruthy()
    expect(screen.getAllByText("计划实例").length).toBeGreaterThan(0)
    expect(screen.getByText(/计划于.*2026/)).toBeTruthy()
    expect(screen.getByText(/首次编辑或执行操作后会创建本次任务/)).toBeTruthy()
    expect(screen.getByText(/修改或完成只影响这一次/)).toBeTruthy()
    expect(screen.getAllByText("继承自循环任务").length).toBeGreaterThanOrEqual(
      4
    )
    expect(screen.getByText("所属循环任务已停止")).toBeTruthy()
    expect(screen.getByRole("button", { name: "开始本次" })).toBeTruthy()
    expect(screen.queryByText("occ:series-1:1784476799")).toBeNull()
  })

  it("replaces an occurrence alias with its materialized slug permalink", async () => {
    vi.mocked(getTask).mockResolvedValue(
      task({
        id: "occ:series-1:1783036800",
        uuid: "occurrence-uuid",
        task_slug: "agentapi-7",
        recurrence_info: {
          role: "occurrence",
          series_id: "series-1",
          series_status: "active",
          rule: "daily",
          recurrence_at: 1_783_036_800,
          materialization: "materialized",
        },
      })
    )

    render(
      <TaskDetailPage
        projectSlug="agentapi"
        taskRef="occ:series-1:1783036800"
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    await screen.findByText("写投放日报")
    await waitFor(() => {
      expect(navigateMock).toHaveBeenCalledWith({
        to: "/workspaces/$workspaceSlug/projects/$projectSlug/tasks/$taskRef",
        params: {
          workspaceSlug: "acme",
          projectSlug: "agentapi",
          taskRef: "agentapi-7",
        },
        replace: true,
      })
    })
  })

  it("preserves the My Tasks return state when canonicalizing an occurrence permalink", async () => {
    vi.mocked(getTask).mockResolvedValue(
      task({
        id: "occ:series-1:1783036800",
        uuid: "occurrence-uuid",
        task_slug: "agentapi-7",
        recurrence_info: {
          role: "occurrence",
          series_id: "series-1",
          series_status: "active",
          rule: "daily",
          recurrence_at: 1_783_036_800,
          materialization: "materialized",
        },
      })
    )

    render(
      <TaskDetailPage
        myTasksReturnSearch="tab=completed&priority=H&q=review&sort=due"
        projectSlug="agentapi"
        taskRef="occ:series-1:1783036800"
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    await screen.findByText("写投放日报")
    await waitFor(() => {
      expect(navigateMock).toHaveBeenCalledWith({
        to: "/workspaces/$workspaceSlug/projects/$projectSlug/tasks/$taskRef",
        params: {
          workspaceSlug: "acme",
          projectSlug: "agentapi",
          taskRef: "agentapi-7",
        },
        search: {
          from: "my-tasks",
          my_tasks_search: "tab=completed&priority=H&q=review&sort=due",
        },
        replace: true,
      })
    })
  })

  it("returns a My Tasks occurrence to the original preset and filters", async () => {
    render(
      <TaskDetailPage
        myTasksReturnSearch="tab=overdue&project=ops&task_type=occurrence&priority=H&q=review&sort=priority"
        projectSlug="agentapi"
        taskRef="ag-23"
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    const backLink = await screen.findByRole("link", { name: "返回我的任务" })
    expect(backLink.getAttribute("href")).toBe(
      "/my-tasks?priority=H&project=ops&q=review&sort=priority&tab=overdue&task_type=occurrence"
    )
  })

  it("returns to the original My Tasks filters after skipping an occurrence", async () => {
    vi.mocked(getTask).mockResolvedValue(
      task({
        recurrence_info: {
          role: "occurrence",
          series_id: "series-1",
          series_status: "active",
          rule: "daily",
          recurrence_at: 1_783_036_800,
          materialization: "materialized",
        },
      })
    )

    render(
      <TaskDetailPage
        myTasksReturnSearch="tab=overdue&priority=H&q=review&sort=priority"
        projectSlug="agentapi"
        taskRef="ag-23"
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    await userEvent.click(
      await screen.findByRole("button", { name: "更多操作 ag-23" })
    )
    await userEvent.click(screen.getByRole("menuitem", { name: "跳过本次" }))
    await userEvent.click(
      within(screen.getByRole("alertdialog")).getByRole("button", {
        name: "跳过本次",
      })
    )

    expect(navigateMock).toHaveBeenCalledWith({
      to: "/my-tasks",
      search: {
        tab: "overdue",
        priority: "H",
        q: "review",
        sort: "priority",
      },
    })
  })

  it("does not show lifecycle actions for completed tasks", async () => {
    vi.mocked(getTask).mockResolvedValue(task({ status: "completed" }))

    renderPage()

    await screen.findByText("写投放日报")
    await waitFor(() => {
      expect(screen.queryByRole("button", { name: "开始" })).toBeNull()
      expect(screen.queryByRole("button", { name: "停止" })).toBeNull()
      expect(screen.queryByRole("button", { name: "完成任务" })).toBeNull()
    })
  })

  it("renders project tasks as read-only when the project is closed", async () => {
    renderPage(true)

    await screen.findByText("写投放日报")
    expect(
      (screen.getByRole("button", { name: "任务标题" }) as HTMLButtonElement)
        .disabled
    ).toBe(true)
    expect(
      (screen.getByRole("button", { name: "编辑描述" }) as HTMLButtonElement)
        .disabled
    ).toBe(true)
    expect(screen.queryByRole("button", { name: "开始" })).toBeNull()
    expect(screen.queryByRole("button", { name: "完成任务" })).toBeNull()
    expect(screen.queryByRole("button", { name: "更多操作 ag-23" })).toBeNull()
  })

  it("disables all write controls for completed tasks", async () => {
    vi.mocked(getTask).mockResolvedValue(task({ status: "completed" }))

    renderPage()

    await screen.findByText("写投放日报")
    expect(
      (screen.getByRole("button", { name: "任务标题" }) as HTMLButtonElement)
        .disabled
    ).toBe(true)
    expect(
      (screen.getByRole("button", { name: "编辑描述" }) as HTMLButtonElement)
        .disabled
    ).toBe(true)
    expect(
      (screen.getByRole("combobox", { name: "优先级" }) as HTMLButtonElement)
        .disabled
    ).toBe(true)
    // 已完成 + 无计划字段：Schedule 整组隐身（spec §9.5），截止日期入口不再渲染。
    expect(screen.queryByRole("button", { name: "截止日期" })).toBeNull()
    expect(
      (screen.getByRole("button", { name: "编辑负责人" }) as HTMLButtonElement)
        .disabled
    ).toBe(true)
    expect(
      (screen.getByRole("button", { name: "编辑标签" }) as HTMLButtonElement)
        .disabled
    ).toBe(true)
    expect(
      (
        screen.getByRole("button", {
          name: "编辑依赖任务",
        }) as HTMLButtonElement
      ).disabled
    ).toBe(true)
    expect(screen.queryByRole("button", { name: "添加注解" })).toBeNull()
    expect(screen.queryByRole("button", { name: "添加链接" })).toBeNull()
    expect(screen.queryByRole("button", { name: "删除" })).toBeNull()
  })

  it("renders mobile detail tabs and switches active tab", async () => {
    renderPage()

    await screen.findByText("写投放日报")
    const descriptionTab = screen.getByRole("tab", { name: "正文" })
    const subtasksTab = screen.getByRole("tab", { name: "子任务" })
    const tabList = screen.getByRole("tablist", { name: "任务详情视图" })

    expect(tabList.getAttribute("data-slot")).toBe("tabs-list")
    // 默认打开正文 tab。
    expect(descriptionTab.getAttribute("aria-selected")).toBe("true")
    await userEvent.click(subtasksTab)
    expect(subtasksTab.getAttribute("aria-selected")).toBe("true")
    expect(descriptionTab.getAttribute("aria-selected")).toBe("false")
  })

  it("renders scalar task change history as natural language", async () => {
    vi.mocked(getTaskAudit).mockResolvedValue([
      {
        id: 1,
        actor: { id: "u1", name: "alice", display_name: "Alice" },
        action: "task.modify",
        target_type: "task",
        target_id: "task-1",
        created_at: 1_783_036_800,
        changes: [
          {
            field: "title",
            kind: "scalar",
            label_key: "projectWorkbench.taskHistory.field.title",
            previous: { raw: "旧标题", text: "旧标题" },
            current: { raw: "新标题", text: "新标题" },
          },
        ],
      },
    ])
    renderPage()

    // 等 audit query resolve 后的 change 内容出现。
    await screen.findByText(/新标题/)
    expect(screen.getByText(/Alice/)).toBeTruthy()
    expect(screen.getByText(/旧标题/)).toBeTruthy()
  })

  it("renders set task change history with added and removed", async () => {
    vi.mocked(getTaskAudit).mockResolvedValue([
      {
        id: 2,
        actor: { id: "u1", name: "alice", display_name: "Alice" },
        action: "task.modify",
        target_type: "task",
        target_id: "task-1",
        created_at: 1_783_036_800,
        changes: [
          {
            field: "assignees",
            kind: "set",
            label_key: "projectWorkbench.taskHistory.field.assignees",
            added: [
              {
                raw: { id: "u2", name: "lisi", display_name: "李四" },
                text: "李四",
              },
            ],
            removed: [
              {
                raw: { id: "u1", name: "zhangsan", display_name: "张三" },
                text: "张三",
              },
            ],
          },
        ],
      },
    ])
    renderPage()

    // 集合变化展示 display_name，不展示 UUID。
    await screen.findByText(/李四/)
    expect(screen.getByText(/张三/)).toBeTruthy()
    expect(screen.queryByText(/u2|u1/)).toBeNull()
    // setChange 模板已含「新增/移除」动词，formatSet 不应再重复拼动词。
    expect(screen.queryByText(/新增 新增/)).toBeNull()
    expect(screen.queryByText(/移除 移除/)).toBeNull()
  })

  it("renders unset placeholder when scalar current is null", async () => {
    vi.mocked(getTaskAudit).mockResolvedValue([
      {
        id: 3,
        actor: { id: "u1", name: "alice", display_name: "Alice" },
        action: "task.modify",
        target_type: "task",
        target_id: "task-1",
        created_at: 1_783_036_800,
        changes: [
          {
            field: "due",
            kind: "scalar",
            label_key: "projectWorkbench.taskHistory.field.due",
            previous: { raw: 1_783_036_800, text: "2026-07-04" },
            current: { raw: null, text: "" },
          },
        ],
      },
    ])
    renderPage()

    // 清空 due 时显示「未设置」，不直接展示 null。
    await screen.findByText(/未设置/)
  })

  it("shows empty placeholder when audit history is empty", async () => {
    vi.mocked(getTaskAudit).mockResolvedValue([])
    renderPage()

    await screen.findByText("暂无字段级变更记录")
  })

  it("renders wait field change with localized field name", async () => {
    vi.mocked(getTaskAudit).mockResolvedValue([
      {
        id: 5,
        actor: { id: "u1", name: "alice", display_name: "Alice" },
        action: "task.modify",
        target_type: "task",
        target_id: "task-1",
        created_at: 1_783_036_800,
        changes: [
          {
            field: "wait",
            kind: "scalar",
            label_key: "projectWorkbench.taskHistory.field.wait",
            previous: { raw: null, text: "" },
            current: { raw: 1_783_036_800, text: "2026-07-04" },
          },
        ],
      },
    ])
    renderPage()

    // 低频字段 wait 也能渲染，字段名走 i18n（暂缓到）。
    await screen.findByText(/暂缓到/)
  })

  it("renders UDA field change entries", async () => {
    vi.mocked(getTaskAudit).mockResolvedValue([
      {
        id: 6,
        actor: { id: "u1", name: "alice", display_name: "Alice" },
        action: "task.modify",
        target_type: "task",
        target_id: "task-1",
        created_at: 1_783_036_800,
        changes: [
          {
            field: "udas",
            kind: "uda",
            label_key: "projectWorkbench.taskHistory.field.udas",
            entries: [
              {
                name: "effort",
                before: { raw: null, text: "" },
                after: { raw: "2h", text: "2h" },
              },
              {
                name: "budget",
                before: { raw: "100", text: "100" },
                after: { raw: null, text: "" },
              },
            ],
          },
        ],
      },
    ])
    renderPage()

    // UDA change：展示每个 UDA 的 name + before/after。
    await screen.findByText(/effort/)
    expect(screen.getByText(/budget/)).toBeTruthy()
    expect(screen.getByText(/2h/)).toBeTruthy()
    // effort before 和 budget after 都是未设置，至少出现一处。
    expect(screen.getAllByText(/未设置/).length).toBeGreaterThan(0)
  })

  it("renders description change with truncated values and expand dialog", async () => {
    vi.mocked(getTaskAudit).mockResolvedValue([
      {
        id: 4,
        actor: { id: "u1", name: "alice", display_name: "Alice" },
        action: "task.modify",
        target_type: "task",
        target_id: "task-1",
        created_at: 1_783_036_800,
        changes: [
          {
            field: "description",
            kind: "scalar",
            label_key: "projectWorkbench.taskHistory.field.description",
            previous: { raw: "# 旧描述", text: "# 旧描述" },
            current: { raw: "# 新描述正文", text: "# 新描述正文" },
          },
        ],
      },
    ])
    renderPage()

    // 列表行展示截断纯文本摘要（去掉 markdown 标记），含旧值和新值。
    await screen.findByText(/新描述正文/)
    expect(screen.getByText(/旧描述/)).toBeTruthy()
    // 不直接展示 markdown 标记 #。
    expect(screen.queryByText(/# 新描述正文/)).toBeNull()

    // 点击展开 Dialog，查看完整 before/after。
    await userEvent.click(screen.getByRole("button", { name: "查看完整内容" }))
    expect(screen.getByText("当前值")).toBeTruthy()
    expect(screen.getByText("原值")).toBeTruthy()
  })
})
