import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import { ProjectTaskToolbar } from "./project-task-toolbar"

const navigateMock = vi.fn()

vi.mock("@tanstack/react-router", () => ({
  useNavigate: () => navigateMock,
}))

describe("ProjectTaskToolbar", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18n.changeLanguage("zh-CN")
  })

  it("renders rich filters, sort control, active chips, and create action", () => {
    const onCreateTask = vi.fn()

    render(
      <ProjectTaskToolbar
        canCreateTask={true}
        filter={{
          priority: "H",
          q: "日报",
          sort: "due",
          status: "pending",
        }}
        onCreateTask={onCreateTask}
        toParams={{ projectSlug: "adsops", workspaceSlug: "acme" }}
      />
    )

    expect(screen.getByLabelText("搜索任务")).toBeTruthy()
    expect(screen.getByRole("combobox", { name: "状态" })).toBeTruthy()
    expect(screen.getByRole("combobox", { name: "优先级" })).toBeTruthy()
    expect(screen.getByLabelText("负责人")).toBeTruthy()
    expect(screen.getByLabelText("标签")).toBeTruthy()
    expect(screen.getByRole("combobox", { name: "排序" })).toBeTruthy()
    expect(screen.getByRole("button", { name: "更多筛选" })).toBeTruthy()
    expect(screen.getByRole("button", { name: "新建任务" })).toBeTruthy()
    expect(screen.getByText("优先级=H")).toBeTruthy()
    expect(screen.getByText("搜索=日报")).toBeTruthy()
  })

  it("writes sort changes to route search", async () => {
    render(
      <ProjectTaskToolbar
        canCreateTask={true}
        filter={{}}
        onCreateTask={vi.fn()}
        toParams={{ projectSlug: "adsops", workspaceSlug: "acme" }}
      />
    )

    await userEvent.click(screen.getByRole("combobox", { name: "排序" }))
    await userEvent.click(screen.getByRole("option", { name: "截止日期" }))

    expect(navigateMock).toHaveBeenCalledWith({
      params: { projectSlug: "adsops", workspaceSlug: "acme" },
      search: expect.any(Function),
      to: "/workspaces/$workspaceSlug/projects/$projectSlug",
    })
    const searchUpdater = navigateMock.mock.calls[0]?.[0].search
    expect(searchUpdater({})).toEqual({ sort: "due" })
  })

  it("commits advanced filters and clears active filters", async () => {
    render(
      <ProjectTaskToolbar
        canCreateTask={true}
        filter={{ assignee_empty: "true" }}
        onCreateTask={vi.fn()}
        toParams={{ projectSlug: "adsops", workspaceSlug: "acme" }}
      />
    )

    await userEvent.click(screen.getByRole("button", { name: "更多筛选" }))
    await userEvent.type(
      screen.getByLabelText("原始查询"),
      "annotations:blocked"
    )
    await userEvent.click(screen.getByLabelText("无截止日期"))
    await userEvent.click(screen.getByRole("button", { name: "应用筛选" }))

    await waitFor(() => {
      expect(navigateMock).toHaveBeenCalled()
    })
    const searchUpdater = navigateMock.mock.calls.at(-1)?.[0].search
    expect(searchUpdater({ assignee_empty: "true" })).toEqual({
      assignee_empty: "true",
      due_empty: "true",
      query: "annotations:blocked",
    })

    await userEvent.click(screen.getByRole("button", { name: "负责人=未分配" }))
    const clearUpdater = navigateMock.mock.calls.at(-1)?.[0].search
    expect(clearUpdater({ assignee_empty: "true", query: "x" })).toEqual({
      query: "x",
    })
  })
})
