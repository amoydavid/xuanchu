import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import type { ReactNode } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import { workspaceApiGet } from "@/features/workspace/session/workspace-api"
import { createTask } from "../api/task-api"
import { createTaskSeries } from "../api/task-series-api"
import { getWorkspaceMembers } from "../api/users-api"
import { dateToUnix } from "../shared/date-boundary"
import { TaskCreateDialog } from "./task-create-dialog"

vi.mock("../api/task-api", () => ({
  createTask: vi.fn(),
}))

vi.mock("../api/users-api", () => ({
  getWorkspaceMembers: vi.fn(),
}))

vi.mock("../api/task-series-api", () => ({
  createTaskSeries: vi.fn(),
  modifyTaskSeries: vi.fn(),
}))

vi.mock("@/features/workspace/session/workspace-api", () => ({
  workspaceApiGet: vi.fn(),
}))

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

describe("TaskCreateDialog", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18n.changeLanguage("zh-CN")
    vi.mocked(createTask).mockResolvedValue({
      uuid: "task-1",
      task_slug: "ads-1",
      title: "写日报",
      status: "pending",
      project: "adsops",
    })
    vi.mocked(getWorkspaceMembers).mockResolvedValue([
      {
        id: "u1",
        name: "liuwei",
        display_name: "刘玮",
        email: "liuwei@example.com",
        role: "member",
        joined_at: 1,
        modified_at: 1,
      },
    ])
    vi.mocked(createTaskSeries).mockResolvedValue({
      series: { id: "series-1", title: "每日巡检" },
      occurrence: { id: "occ-1", title: "每日巡检" },
    } as never)
    vi.mocked(workspaceApiGet).mockResolvedValue({})
  })

  it("creates a task with markdown description, assignee, priority, due date, and tags", async () => {
    render(
      <TaskCreateDialog
        filters="status=pending"
        onOpenChange={vi.fn()}
        open={true}
        projectSlug="adsops"
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    await userEvent.type(screen.getByLabelText("任务标题"), "复盘素材")
    expect(screen.getByRole("button", { name: "加粗" })).toBeTruthy()
    await userEvent.type(
      screen.getByLabelText("任务内容"),
      "# 整理异常原因{Enter}{Enter}- 素材"
    )
    await userEvent.click(screen.getByRole("combobox", { name: "优先级" }))
    await userEvent.click(screen.getByRole("option", { name: "H" }))
    await userEvent.click(screen.getByRole("button", { name: "截止日期" }))
    await screen.findByRole("grid")
    await userEvent.click(
      screen.getByRole("button", { name: /Friday, July 3rd, 2026/i })
    )
    await userEvent.click(screen.getByRole("button", { name: "选择负责人" }))
    expect(await screen.findByText("刘玮")).toBeTruthy()
    await userEvent.click(screen.getByRole("checkbox", { name: /刘玮/ }))
    await userEvent.click(screen.getByRole("button", { name: "完成" }))
    await userEvent.type(
      screen.getByLabelText("标签", { exact: true }),
      "ops,review"
    )
    await userEvent.click(screen.getByRole("button", { name: "创建任务" }))

    await waitFor(() => {
      expect(createTask).toHaveBeenCalled()
    })
    const payload = vi.mocked(createTask).mock.calls[0]?.[1]
    expect(payload).toMatchObject({
      assignees: ["u1"],
      due: dateToUnix(new Date(2026, 6, 3), "end"),
      priority: "H",
      project: "adsops",
      tags: ["ops", "review"],
      title: "复盘素材",
    })
    expect(payload.description).toContain("# 整理异常原因")
    expect(payload.description).toContain("- 素材")
  }, 10_000)

  it("returns the created task to the caller", async () => {
    const onCreated = vi.fn()
    render(
      <TaskCreateDialog
        onCreated={onCreated}
        onOpenChange={vi.fn()}
        open={true}
        projectSlug="adsops"
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    await userEvent.type(screen.getByLabelText("任务标题"), "写日报")
    await userEvent.click(screen.getByRole("button", { name: "创建任务" }))

    await waitFor(() => {
      expect(onCreated).toHaveBeenCalledWith(
        expect.objectContaining({ task_slug: "ads-1", title: "写日报" })
      )
    })
  })

  it("keeps the dialog open and shows validation when title is empty", async () => {
    render(
      <TaskCreateDialog
        onOpenChange={vi.fn()}
        open={true}
        projectSlug="adsops"
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    await userEvent.click(screen.getByRole("button", { name: "创建任务" }))

    expect(createTask).not.toHaveBeenCalled()
    expect(screen.getByText("任务标题不能为空")).toBeTruthy()
  })

  it("uses shadcn tabs and keeps exactly one dialog in recurring mode", async () => {
    render(
      <TaskCreateDialog
        onOpenChange={vi.fn()}
        open={true}
        projectSlug="adsops"
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    expect(screen.getByRole("tablist", { name: "任务类型" })).toBeTruthy()
    expect(screen.getByRole("dialog").className).toContain("max-h-[90vh]")
    await userEvent.click(screen.getByRole("tab", { name: "循环任务" }))

    expect(screen.getAllByRole("dialog")).toHaveLength(1)
    expect(screen.getByRole("combobox", { name: "循环规则" })).toBeTruthy()
    expect(screen.getByRole("combobox", { name: "优先级" })).toBeTruthy()
    expect(screen.getByRole("button", { name: "首次截止日期" })).toBeTruthy()
    expect(screen.queryByTestId("task-series-dialog")).toBeNull()
  })

  it("localizes the unified create dialog in English", async () => {
    await i18n.changeLanguage("en-US")
    render(
      <TaskCreateDialog
        onOpenChange={vi.fn()}
        open={true}
        projectSlug="adsops"
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    expect(screen.getByRole("tablist", { name: "Task type" })).toBeTruthy()
    expect(screen.getByLabelText("Task title")).toBeTruthy()
    expect(screen.getByLabelText("Task details")).toBeTruthy()
    expect(screen.getByRole("combobox", { name: "Priority" })).toBeTruthy()
    expect(
      screen.getByRole("button", { name: "Select assignees" })
    ).toBeTruthy()
    expect(screen.getByRole("button", { name: "Due date" })).toBeTruthy()
    expect(screen.getByLabelText("Tags")).toBeTruthy()
    expect(screen.getByRole("button", { name: "Create task" })).toBeTruthy()
  })

  it("returns the created series so the caller can open its detail", async () => {
    const onRecurringCreated = vi.fn()
    render(
      <TaskCreateDialog
        initialMode="recurring"
        onOpenChange={vi.fn()}
        onRecurringCreated={onRecurringCreated}
        open={true}
        projectSlug="adsops"
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    const form = within(screen.getByTestId("task-series-form"))
    await userEvent.type(form.getByLabelText("任务标题"), "每日巡检")
    await userEvent.click(form.getByRole("button", { name: "首次截止日期" }))
    await userEvent.click(
      screen.getByRole("button", { name: /Friday, July 3rd, 2026/i })
    )
    await userEvent.click(form.getByRole("button", { name: "创建循环任务" }))

    await waitFor(() => {
      expect(createTaskSeries).toHaveBeenCalledOnce()
      expect(onRecurringCreated).toHaveBeenCalledWith(
        expect.objectContaining({ id: "series-1" })
      )
    })
  })

  it("submits all shared task fields when creating a recurring task", async () => {
    vi.mocked(workspaceApiGet).mockResolvedValue({
      "uda.channel.type": "string",
      "uda.channel.label": "渠道",
    })
    render(
      <TaskCreateDialog
        initialMode="recurring"
        onOpenChange={vi.fn()}
        open={true}
        projectSlug="adsops"
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    const form = within(screen.getByTestId("task-series-form"))
    await userEvent.type(form.getByLabelText("任务标题"), "每日巡检")
    await userEvent.type(form.getByLabelText("任务内容"), "检查账户异常")
    await userEvent.click(form.getByRole("combobox", { name: "优先级" }))
    await userEvent.click(screen.getByRole("option", { name: "H" }))
    await userEvent.click(form.getByRole("button", { name: "选择负责人" }))
    await userEvent.click(await screen.findByRole("checkbox", { name: /刘玮/ }))
    await userEvent.click(screen.getByRole("button", { name: "完成" }))
    await userEvent.type(form.getByLabelText("标签"), "ops, daily")
    await userEvent.type(await form.findByLabelText("渠道"), "search")
    await userEvent.click(form.getByRole("button", { name: "首次截止日期" }))
    await userEvent.click(
      screen.getByRole("button", { name: /Friday, July 3rd, 2026/i })
    )
    await userEvent.click(form.getByRole("button", { name: "创建循环任务" }))

    await waitFor(() => expect(createTaskSeries).toHaveBeenCalledOnce())
    expect(vi.mocked(createTaskSeries).mock.calls[0]?.[1]).toMatchObject({
      assignees: ["u1"],
      description: "检查账户异常",
      first_due: dateToUnix(new Date(2026, 6, 3), "end"),
      priority: "H",
      project: "adsops",
      recurrence_rule: "daily",
      tags: ["ops", "daily"],
      title: "每日巡检",
      udas: { channel: "search" },
    })
  }, 10_000)

  it("keeps shared fields and copies due once when switching task type", async () => {
    render(
      <TaskCreateDialog
        onOpenChange={vi.fn()}
        open={true}
        projectSlug="adsops"
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    await userEvent.type(screen.getByLabelText("任务标题"), "每日巡检")
    await userEvent.type(screen.getByLabelText("任务内容"), "保留这段说明")
    await userEvent.click(screen.getByRole("button", { name: "截止日期" }))
    await userEvent.click(
      screen.getByRole("button", { name: /Friday, July 3rd, 2026/i })
    )
    await userEvent.click(screen.getByRole("tab", { name: "循环任务" }))

    expect((screen.getByLabelText("任务标题") as HTMLInputElement).value).toBe(
      "每日巡检"
    )
    expect(screen.getByLabelText("任务内容").textContent).toContain(
      "保留这段说明"
    )
    expect(
      screen.getByRole("button", { name: "首次截止日期" }).textContent
    ).toContain("2026-07-03")

    await userEvent.click(screen.getByRole("button", { name: "首次截止日期" }))
    await userEvent.click(
      screen.getByRole("button", { name: /Saturday, July 4th, 2026/i })
    )
    await userEvent.click(screen.getByRole("tab", { name: "普通任务" }))
    await userEvent.click(screen.getByRole("tab", { name: "循环任务" }))

    expect(
      screen.getByRole("button", { name: "首次截止日期" }).textContent
    ).toContain("2026-07-04")
  }, 10_000)
})
