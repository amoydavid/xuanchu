import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import type { ReactNode } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { modifyTask } from "../api/task-api"
import { TaskPropertyPanel } from "./task-property-panel"

vi.mock("../api/task-api", async () => {
  const actual = await vi.importActual<typeof import("../api/task-api")>(
    "../api/task-api"
  )
  return {
    ...actual,
    modifyTask: vi.fn(),
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

function task() {
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
    effort: "2h",
  }
}

describe("TaskPropertyPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(modifyTask).mockResolvedValue(task())
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

  it("saves tags and assignees from comma separated input", async () => {
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

    await userEvent.clear(screen.getByLabelText("标签"))
    await userEvent.type(screen.getByLabelText("标签"), "web, console{Enter}")
    expect(modifyTask).toHaveBeenCalledWith("acme", "ads-1", {
      tags: ["web", "console"],
    })

    await userEvent.clear(screen.getByLabelText("负责人"))
    await userEvent.type(screen.getByLabelText("负责人"), "u2,u3{Enter}")
    expect(modifyTask).toHaveBeenCalledWith("acme", "ads-1", {
      assignees: ["u2", "u3"],
    })
  })

  it("saves existing UDA values and dependency list", async () => {
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

    await userEvent.type(screen.getByLabelText("依赖任务"), "dep-1, dep-2{Enter}")
    expect(modifyTask).toHaveBeenCalledWith("acme", "ads-1", {
      depends: ["dep-1", "dep-2"],
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
