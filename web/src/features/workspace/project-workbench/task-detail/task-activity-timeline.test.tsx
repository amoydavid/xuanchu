import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import type { ReactNode } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import {
  addTaskAnnotation,
  deleteTaskAnnotation,
  updateTaskAnnotation,
} from "../api/task-api"
import { useTaskActivityQuery } from "../hooks/use-task-detail-data"
import { EditFeedbackProvider } from "../shared/edit-feedback"
import { ActivitySection } from "./activity-section"

vi.mock("../hooks/use-task-detail-data", async () => {
  const actual = await vi.importActual<
    typeof import("../hooks/use-task-detail-data")
  >("../hooks/use-task-detail-data")
  return { ...actual, useTaskActivityQuery: vi.fn() }
})

vi.mock("../api/task-api", async () => {
  const actual =
    await vi.importActual<typeof import("../api/task-api")>("../api/task-api")
  return {
    ...actual,
    addTaskAnnotation: vi.fn(),
    deleteTaskAnnotation: vi.fn(),
    updateTaskAnnotation: vi.fn(),
  }
})

function wrapper({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider client={new QueryClient()}>
      <EditFeedbackProvider>{children}</EditFeedbackProvider>
    </QueryClientProvider>
  )
}

function activityQuery(overrides: Record<string, unknown> = {}) {
  return {
    data: {
      pages: [
        {
          entries: [
            {
              id: "audit:3",
              kind: "lifecycle",
              action: "completed",
              actor: {
                type: "user",
                user: { id: "u1", name: "lisi", display_name: "李四" },
              },
              occurred_at: "2026-07-24T03:02:00Z",
            },
            {
              id: "annotation:a1",
              kind: "annotation",
              action: "commented",
              actor: { type: "unknown" },
              occurred_at: "2026-07-24T02:30:00Z",
              annotation: { id: "a1", description: "需要补素材截图。" },
            },
            {
              id: "audit:1",
              kind: "lifecycle",
              action: "created",
              actor: { type: "system" },
              occurred_at: "2026-07-23T10:20:00Z",
            },
          ],
          next_cursor: "next-page",
        },
      ],
      pageParams: [""],
    },
    isPending: false,
    isError: false,
    hasNextPage: true,
    isFetchingNextPage: false,
    fetchNextPage: vi.fn(),
    refetch: vi.fn(),
    ...overrides,
  } as unknown as ReturnType<typeof useTaskActivityQuery>
}

describe("TaskActivityTimeline", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18n.changeLanguage("zh-CN")
    vi.mocked(useTaskActivityQuery).mockReturnValue(activityQuery())
    const updated = { uuid: "task-1", title: "任务", status: "pending" }
    vi.mocked(addTaskAnnotation).mockResolvedValue(updated)
    vi.mocked(updateTaskAnnotation).mockResolvedValue(updated)
    vi.mocked(deleteTaskAnnotation).mockResolvedValue(updated)
  })

  it("renders one semantic vertical ordered timeline", () => {
    const { container } = render(
      <ActivitySection
        canWrite={false}
        projectSlug="adsops"
        taskRef="ads-1"
        workspaceSlug="acme"
      />,
      { wrapper }
    )

    const list = screen.getByRole("list", { name: "活动" })
    expect(list.tagName).toBe("OL")
    expect(screen.getAllByRole("listitem")).toHaveLength(3)
    expect(screen.getByText("李四")).toBeTruthy()
    expect(screen.getByText("完成了任务")).toBeTruthy()
    expect(screen.getByText("需要补素材截图。")).toBeTruthy()
    expect(screen.getByText("系统")).toBeTruthy()
    expect(screen.getByText("未知主体")).toBeTruthy()
    expect(container.textContent).not.toContain("task.done")

    const nodes = container.querySelectorAll("[data-activity-node]")
    expect(nodes).toHaveLength(3)
    nodes.forEach((node) =>
      expect(node.getAttribute("aria-hidden")).toBe("true")
    )
    expect(
      screen
        .getAllByRole("listitem")[0]
        .querySelector('[data-activity-line-before="true"]')
    ).toBeNull()
    expect(container.querySelector('[data-activity-tail="more"]')).toBeTruthy()
  })

  it("loads the next page without replacing the current list", () => {
    const query = activityQuery()
    vi.mocked(useTaskActivityQuery).mockReturnValue(query)
    render(
      <ActivitySection
        canWrite={false}
        projectSlug="adsops"
        taskRef="ads-1"
        workspaceSlug="acme"
      />,
      { wrapper }
    )
    fireEvent.click(screen.getByRole("button", { name: "加载更多" }))
    expect(query.fetchNextPage).toHaveBeenCalledOnce()
    expect(screen.getAllByRole("listitem")).toHaveLength(3)
  })

  it("shows a node-free empty state", () => {
    vi.mocked(useTaskActivityQuery).mockReturnValue(
      activityQuery({
        data: { pages: [{ entries: [], next_cursor: null }], pageParams: [""] },
        hasNextPage: false,
      })
    )
    const { container } = render(
      <ActivitySection
        canWrite={false}
        projectSlug="adsops"
        taskRef="ads-1"
        workspaceSlug="acme"
      />,
      { wrapper }
    )
    expect(screen.getByText("暂无活动")).toBeTruthy()
    expect(container.querySelector("[data-activity-node]")).toBeNull()
  })

  it("adds an annotation from the composer above the timeline", async () => {
    render(
      <ActivitySection
        canWrite={true}
        projectSlug="adsops"
        taskRef="ads-1"
        workspaceSlug="acme"
      />,
      { wrapper }
    )

    await userEvent.click(screen.getByRole("button", { name: "写更新" }))
    await userEvent.type(screen.getByLabelText("新增注解"), "素材已经补齐")
    await userEvent.click(screen.getByRole("button", { name: "添加注解" }))

    expect(addTaskAnnotation).toHaveBeenCalledWith("acme", "ads-1", {
      description: "素材已经补齐",
    })
    await waitFor(() => expect(screen.queryByLabelText("新增注解")).toBeNull())
    expect(screen.getByRole("button", { name: "写更新" })).toBeTruthy()
  })

  it("edits and deletes an annotation from its timeline entry", async () => {
    render(
      <ActivitySection
        canWrite={true}
        projectSlug="adsops"
        taskRef="ads-1"
        workspaceSlug="acme"
      />,
      { wrapper }
    )

    await userEvent.click(screen.getByRole("button", { name: "编辑注解" }))
    const editor = screen.getByLabelText("编辑注解内容")
    await userEvent.click(editor)
    await userEvent.keyboard("{Control>}a{/Control}{Backspace}")
    await userEvent.type(editor, "更新后的注解")
    await userEvent.click(screen.getByRole("button", { name: "保存注解" }))
    expect(updateTaskAnnotation).toHaveBeenCalledWith("acme", "ads-1", "a1", {
      description: "更新后的注解",
    })
    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: "编辑注解" })).toBeNull()
    )

    await userEvent.click(screen.getByRole("button", { name: "删除注解" }))
    const confirm = screen.getByRole("alertdialog")
    await userEvent.click(
      within(confirm).getByRole("button", { name: "删除注解" })
    )
    expect(deleteTaskAnnotation).toHaveBeenCalledWith("acme", "ads-1", "a1")
  })

  it("keeps long description changes expandable inside one activity entry", async () => {
    vi.mocked(useTaskActivityQuery).mockReturnValue(
      activityQuery({
        data: {
          pages: [
            {
              entries: [
                {
                  id: "audit:9",
                  kind: "change",
                  action: "fields_changed",
                  actor: { type: "system" },
                  occurred_at: "2026-07-24T03:02:00Z",
                  changes: [
                    {
                      field: "description",
                      kind: "scalar",
                      label_key: "task.field.description",
                      previous: { raw: "旧正文", text: "旧正文" },
                      current: { raw: "新正文", text: "新正文" },
                    },
                  ],
                },
              ],
              next_cursor: null,
            },
          ],
          pageParams: [""],
        },
        hasNextPage: false,
      })
    )
    render(
      <ActivitySection
        canWrite={false}
        projectSlug="adsops"
        taskRef="ads-1"
        workspaceSlug="acme"
      />,
      { wrapper }
    )

    expect(screen.queryByText("旧正文")).toBeNull()
    await userEvent.click(screen.getByRole("button", { name: "查看变更" }))
    expect(screen.getByText("旧正文")).toBeTruthy()
    expect(screen.getByText("新正文")).toBeTruthy()
  })

  it("renders loading, local error, and terminal rail states", () => {
    vi.mocked(useTaskActivityQuery).mockReturnValue(
      activityQuery({ data: undefined, isPending: true })
    )
    const pending = render(
      <ActivitySection
        canWrite={false}
        projectSlug="adsops"
        taskRef="ads-1"
        workspaceSlug="acme"
      />,
      { wrapper }
    )
    expect(
      pending.container.querySelectorAll("[data-activity-skeleton]")
    ).toHaveLength(3)
    pending.unmount()

    vi.mocked(useTaskActivityQuery).mockReturnValue(
      activityQuery({ data: undefined, isPending: false, isError: true })
    )
    const failed = render(
      <ActivitySection
        canWrite={false}
        projectSlug="adsops"
        taskRef="ads-1"
        workspaceSlug="acme"
      />,
      { wrapper }
    )
    expect(screen.getByText("活动暂不可用")).toBeTruthy()
    expect(failed.container.querySelector("[data-activity-node]")).toBeNull()
    failed.unmount()

    vi.mocked(useTaskActivityQuery).mockReturnValue(
      activityQuery({ hasNextPage: false })
    )
    const complete = render(
      <ActivitySection
        canWrite={false}
        projectSlug="adsops"
        taskRef="ads-1"
        workspaceSlug="acme"
      />,
      { wrapper }
    )
    expect(
      complete.container.querySelector('[data-activity-tail="more"]')
    ).toBeNull()
    expect(
      screen
        .getAllByRole("listitem")
        .at(-1)
        ?.querySelector("[data-activity-tail]")
    ).toBeNull()
  })

  it("uses actor display fallbacks and hides write controls in readonly mode", () => {
    vi.mocked(useTaskActivityQuery).mockReturnValue(
      activityQuery({
        data: {
          pages: [
            {
              entries: [
                {
                  id: "audit:token",
                  kind: "lifecycle",
                  action: "started",
                  actor: {
                    type: "tenant_access_token",
                    token: { id: "t1", name: "自动化", prefix: "xct" },
                  },
                  occurred_at: "2026-07-24T03:02:00Z",
                },
                {
                  id: "audit:user-name",
                  kind: "lifecycle",
                  action: "stopped",
                  actor: {
                    type: "user",
                    user: { id: "u2", name: "wangwu", display_name: "" },
                  },
                  occurred_at: "2026-07-24T03:01:00Z",
                },
                {
                  id: "annotation:readonly",
                  kind: "annotation",
                  action: "commented",
                  actor: { type: "user", user: { id: "u3", name: "" } },
                  occurred_at: "2026-07-24T03:00:00Z",
                  annotation: { id: "readonly", description: "只读内容" },
                },
              ],
              next_cursor: null,
            },
          ],
          pageParams: [""],
        },
        hasNextPage: false,
      })
    )
    render(
      <ActivitySection
        canWrite={false}
        projectSlug="adsops"
        taskRef="ads-1"
        workspaceSlug="acme"
      />,
      { wrapper }
    )

    expect(screen.getByText("自动化")).toBeTruthy()
    expect(screen.getByText("wangwu")).toBeTruthy()
    expect(screen.getByText("u3")).toBeTruthy()
    expect(screen.queryByRole("button", { name: "写更新" })).toBeNull()
    expect(screen.queryByRole("button", { name: "编辑注解" })).toBeNull()
    expect(screen.queryByRole("button", { name: "删除注解" })).toBeNull()
  })
})
