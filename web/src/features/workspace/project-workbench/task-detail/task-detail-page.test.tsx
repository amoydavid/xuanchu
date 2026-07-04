import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
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

vi.mock("@/features/workspace/session/useMe", () => ({
  useMe: () => ({
    data: {
      effective_role: "member",
      token: { scopes: ["task:write"], type: "pat" },
    },
  }),
}))

vi.mock("../api/task-api", async () => {
  const actual = await vi.importActual<typeof import("../api/task-api")>(
    "../api/task-api"
  )
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

function renderPage() {
  render(
    <TaskDetailPage
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
    expect(screen.getByText("素材")).toBeTruthy()
    expect(screen.getByText("预算")).toBeTruthy()
    expect(screen.getByRole("link", { name: "acme" }).getAttribute("href")).toBe(
      "/projects"
    )
    expect(
      screen.getByRole("link", { name: "agentapi" }).getAttribute("href")
    ).toBe("/workspaces/acme/projects/agentapi")
    expect(
      document.querySelector('[class*="md:grid-cols-[minmax(0,1fr)_280px]"]')
    ).toBeTruthy()
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

  it("runs start, done, and delete actions", async () => {
    renderPage()

    await screen.findByText("写投放日报")
    await userEvent.click(screen.getByRole("button", { name: "开始" }))
    expect(startTask).toHaveBeenCalledWith("acme", "ag-23")

    await userEvent.click(screen.getByRole("button", { name: "完成" }))
    expect(doneTask).toHaveBeenCalledWith("acme", "ag-23")

    await userEvent.click(screen.getByRole("button", { name: "删除" }))
    expect(screen.getByText("确认删除任务")).toBeTruthy()
    await userEvent.click(screen.getByRole("button", { name: "删除任务" }))
    expect(deleteTask).toHaveBeenCalledWith("acme", "ag-23")
  })

  it("does not show lifecycle actions for completed tasks", async () => {
    vi.mocked(getTask).mockResolvedValue(task({ status: "completed" }))

    renderPage()

    await screen.findByText("写投放日报")
    await waitFor(() => {
      expect(screen.queryByRole("button", { name: "开始" })).toBeNull()
      expect(screen.queryByRole("button", { name: "停止" })).toBeNull()
      expect(screen.queryByRole("button", { name: "完成" })).toBeNull()
    })
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
    expect(
      (screen.getByRole("button", { name: "截止日期" }) as HTMLButtonElement)
        .disabled
    ).toBe(true)
    expect(
      (screen.getByRole("button", { name: "编辑负责人" }) as HTMLButtonElement)
        .disabled
    ).toBe(true)
    expect(
      (screen.getByRole("button", { name: "编辑标签" }) as HTMLButtonElement)
        .disabled
    ).toBe(true)
    expect(
      (screen.getByRole("button", { name: "编辑依赖任务" }) as HTMLButtonElement)
        .disabled
    ).toBe(true)
    expect(screen.queryByRole("button", { name: "添加注解" })).toBeNull()
    expect(screen.queryByRole("button", { name: "添加链接" })).toBeNull()
    expect(screen.queryByRole("button", { name: "删除" })).toBeNull()
  })

  it("renders mobile detail tabs and switches active tab", async () => {
    renderPage()

    await screen.findByText("写投放日报")
    const propertyTab = screen.getByRole("tab", { name: "属性" })
    const linkTab = screen.getByRole("tab", { name: "链接" })
    const tabList = screen.getByRole("tablist", { name: "任务详情视图" })

    expect(tabList.getAttribute("data-slot")).toBe("tabs-list")
    expect(propertyTab.getAttribute("aria-selected")).toBe("true")
    await userEvent.click(linkTab)
    expect(linkTab.getAttribute("aria-selected")).toBe("true")
    expect(propertyTab.getAttribute("aria-selected")).toBe("false")
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
            added: [{ raw: { id: "u2", name: "lisi", display_name: "李四" }, text: "李四" }],
            removed: [{ raw: { id: "u1", name: "zhangsan", display_name: "张三" }, text: "张三" }],
          },
        ],
      },
    ])
    renderPage()

    // 集合变化展示 display_name，不展示 UUID。
    await screen.findByText(/李四/)
    expect(screen.getByText(/张三/)).toBeTruthy()
    expect(screen.queryByText(/u2|u1/)).toBeNull()
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
