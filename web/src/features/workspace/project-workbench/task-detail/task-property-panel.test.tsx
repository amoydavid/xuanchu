import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import type { ReactNode } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { getProjectTasks } from "../api/project-api"
import { modifyTask } from "../api/task-api"
import { getWorkspaceMembers } from "../api/users-api"
import { TaskPropertyPanel } from "./task-property-panel"

vi.mock("../api/task-api", async () => {
  const actual =
    await vi.importActual<typeof import("../api/task-api")>("../api/task-api")
  return {
    ...actual,
    modifyTask: vi.fn(),
  }
})

vi.mock("../api/project-api", async () => {
  const actual =
    await vi.importActual<typeof import("../api/project-api")>(
      "../api/project-api"
    )
  return {
    ...actual,
    getProjectTasks: vi.fn(),
  }
})

vi.mock("../api/users-api", () => ({
  getWorkspaceMembers: vi.fn(),
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

function task(overrides: Record<string, unknown> = {}) {
  return {
    uuid: "task-1",
    task_slug: "ads-1",
    title: "写投放日报",
    status: "pending",
    project: "adsops",
    priority: "M",
    due: 1_783_036_800,
    tags: ["ads", "daily"],
    assignees: [{ user_id: "u1", name: "张三" }],
    wait: null,
    scheduled: null,
    until: null,
    recur: "weekly",
    parent: "parent-uuid",
    parent_info: { uuid: "parent-uuid", task_slug: "root-1", title: "父任务" },
    depends: ["dep-1"],
    depends_info: [{ uuid: "dep-1", task_slug: "ads-0", title: "素材审核" }],
    effort: "2h",
    ...overrides,
  }
}

describe("TaskPropertyPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(modifyTask).mockResolvedValue(task())
    vi.mocked(getWorkspaceMembers).mockResolvedValue([
      {
        user_id: "u1",
        name: "张三",
        email: "zhang@example.com",
        role: "member",
        joined_at: 1,
        modified_at: 1,
      },
      {
        user_id: "u2",
        name: "李四",
        email: "li@example.com",
        role: "member",
        joined_at: 1,
        modified_at: 1,
      },
    ])
    vi.mocked(getProjectTasks).mockResolvedValue([
      task(),
      {
        ...task({
          uuid: "dep-2",
          task_slug: "ads-2",
          title: "预算确认",
          status: "active",
          tags: ["dashboard", "daily"],
        }),
      },
    ])
  })

  it("saves priority and due date edits", async () => {
    render(
      <TaskPropertyPanel
        canWrite={true}
        projectSlug="adsops"
        task={task()}
        taskRef="ads-1"
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    await userEvent.click(screen.getByRole("combobox", { name: "优先级" }))
    await userEvent.click(screen.getByRole("option", { name: "H" }))
    expect(modifyTask).toHaveBeenCalledWith("acme", "ads-1", { priority: "H" })

    await userEvent.clear(screen.getByLabelText("截止日期"))
    await userEvent.tab()
    expect(modifyTask).toHaveBeenCalledWith("acme", "ads-1", {
      clear_due: true,
    })
  })

  it("selects, creates, and clears tags from the tag picker", async () => {
    render(
      <TaskPropertyPanel
        canWrite={true}
        projectSlug="adsops"
        task={task()}
        taskRef="ads-1"
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    expect(screen.getByText("ads")).toBeTruthy()
    expect(screen.getByText("daily")).toBeTruthy()
    await userEvent.click(screen.getByRole("button", { name: "编辑标签" }))
    expect(await screen.findByText("dashboard")).toBeTruthy()
    expect(
      screen
        .getByRole("button", { name: "移除标签 ads" })
        .getAttribute("data-slot")
    ).toBe("button")
    await userEvent.click(screen.getByRole("checkbox", { name: "dashboard" }))
    await userEvent.type(
      screen.getByRole("textbox", { name: "搜索标签" }),
      "console"
    )
    await userEvent.click(
      screen.getByRole("button", { name: "新建标签 console" })
    )
    await userEvent.click(screen.getByRole("button", { name: "完成" }))
    expect(modifyTask).toHaveBeenCalledWith("acme", "ads-1", {
      tags: ["ads", "daily", "dashboard", "console"],
    })

    await userEvent.click(screen.getByRole("button", { name: "编辑标签" }))
    await userEvent.click(screen.getByRole("button", { name: "清空标签" }))
    await userEvent.click(screen.getByRole("button", { name: "完成" }))
    expect(modifyTask).toHaveBeenCalledWith("acme", "ads-1", { tags: [] })
  })

  it("selects and clears assignees from workspace members", async () => {
    render(
      <TaskPropertyPanel
        canWrite={true}
        projectSlug="adsops"
        task={task()}
        taskRef="ads-1"
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    expect(screen.getByText("张三")).toBeTruthy()
    await userEvent.click(screen.getByRole("button", { name: "编辑负责人" }))
    expect(await screen.findByText("李四")).toBeTruthy()
    expect(
      screen
        .getByRole("button", { name: "移除负责人 张三" })
        .getAttribute("data-slot")
    ).toBe("button")
    await userEvent.click(screen.getByRole("checkbox", { name: /李四/ }))
    await userEvent.click(screen.getByRole("button", { name: "完成" }))
    expect(modifyTask).toHaveBeenCalledWith("acme", "ads-1", {
      assignees: ["u1", "u2"],
    })

    await userEvent.click(screen.getByRole("button", { name: "编辑负责人" }))
    await userEvent.click(screen.getByRole("button", { name: "清空负责人" }))
    await userEvent.click(screen.getByRole("button", { name: "完成" }))
    expect(modifyTask).toHaveBeenCalledWith("acme", "ads-1", {
      clear_assignees: true,
    })
  })

  it("saves existing UDA values and dependency picker changes", async () => {
    render(
      <TaskPropertyPanel
        canWrite={true}
        projectSlug="adsops"
        task={task()}
        taskRef="ads-1"
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    await userEvent.click(screen.getByRole("button", { name: "UDA effort" }))
    await userEvent.clear(screen.getByLabelText("UDA effort"))
    await userEvent.type(screen.getByLabelText("UDA effort"), "3h{Enter}")
    expect(modifyTask).toHaveBeenCalledWith("acme", "ads-1", {
      udas: { effort: "3h" },
    })

    expect(screen.getByText(/素材审核/)).toBeTruthy()
    await userEvent.click(screen.getByRole("button", { name: "编辑依赖任务" }))
    expect(await screen.findByText("预算确认")).toBeTruthy()
    expect(
      screen
        .getByRole("button", { name: "移除依赖 ads-0" })
        .getAttribute("data-slot")
    ).toBe("button")
    await userEvent.click(screen.getByRole("checkbox", { name: /ads-2/ }))
    await userEvent.click(screen.getByRole("button", { name: "完成" }))
    expect(modifyTask).toHaveBeenCalledWith("acme", "ads-1", {
      depends: ["dep-1", "dep-2"],
    })

    await userEvent.click(screen.getByRole("button", { name: "编辑依赖任务" }))
    await userEvent.click(screen.getByRole("button", { name: "清空依赖" }))
    await userEvent.click(screen.getByRole("button", { name: "完成" }))
    expect(modifyTask).toHaveBeenCalledWith("acme", "ads-1", {
      clear_depends: true,
    })
  })

  it("uses typed editors for boolean, numeric, and date UDA values", async () => {
    render(
      <TaskPropertyPanel
        canWrite={true}
        projectSlug="adsops"
        task={task({
          budget: 1200,
          launch_date: "2026-07-03",
          reviewed: true,
        })}
        taskRef="ads-1"
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    await userEvent.clear(screen.getByLabelText("UDA budget"))
    await userEvent.type(screen.getByLabelText("UDA budget"), "1300{Enter}")
    expect(modifyTask).toHaveBeenCalledWith("acme", "ads-1", {
      udas: { budget: "1300" },
    })

    await userEvent.clear(screen.getByLabelText("UDA launch_date"))
    await userEvent.type(screen.getByLabelText("UDA launch_date"), "2026-07-04")
    await userEvent.tab()
    expect(modifyTask).toHaveBeenCalledWith("acme", "ads-1", {
      udas: { launch_date: "2026-07-04" },
    })

    const reviewed = screen.getByRole("switch", { name: "UDA reviewed" })
    expect(reviewed.getAttribute("data-slot")).toBe("switch")
    await userEvent.click(reviewed)
    expect(modifyTask).toHaveBeenCalledWith("acme", "ads-1", {
      udas: { reviewed: "false" },
    })
  })

  it("renders parent task as readonly", () => {
    render(
      <TaskPropertyPanel
        canWrite={true}
        projectSlug="adsops"
        task={task()}
        taskRef="ads-1"
        workspaceSlug="acme"
      />,
      { wrapper: makeWrapper(makeQueryClient()) }
    )

    expect(screen.getAllByText("父任务").length).toBeGreaterThan(0)
    expect(screen.getByRole("link", { name: /父任务/ })).toBeTruthy()
    expect(screen.queryByLabelText("父任务")).toBeNull()
  })
})
