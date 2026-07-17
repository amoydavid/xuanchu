import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import type { MeResponse } from "@/features/workspace/session/useMe"
import { renderWithRouter } from "@/test/router-wrapper"
import { listWorkspaceEffectiveConfig } from "@/features/workspace/config/config-definition-api"
import { getProjects } from "@/features/workspace/project-workbench/api/project-api"
import {
  doneTask,
  startTask,
  stopTask,
} from "@/features/workspace/project-workbench/api/task-api"
import { getHome, type HomeView } from "./home-api"
import { HomePage } from "./home-page"

vi.mock("./home-api", async () => {
  const actual =
    await vi.importActual<typeof import("./home-api")>("./home-api")
  return { ...actual, getHome: vi.fn() }
})

vi.mock("@/features/workspace/config/config-definition-api", async () => {
  const actual = await vi.importActual<
    typeof import("@/features/workspace/config/config-definition-api")
  >("@/features/workspace/config/config-definition-api")
  return { ...actual, listWorkspaceEffectiveConfig: vi.fn() }
})

vi.mock("@/features/workspace/project-workbench/api/project-api", async () => {
  const actual = await vi.importActual<
    typeof import("@/features/workspace/project-workbench/api/project-api")
  >("@/features/workspace/project-workbench/api/project-api")
  return { ...actual, getProjects: vi.fn() }
})

vi.mock("@/features/workspace/project-workbench/api/task-api", async () => {
  const actual = await vi.importActual<
    typeof import("@/features/workspace/project-workbench/api/task-api")
  >("@/features/workspace/project-workbench/api/task-api")
  return {
    ...actual,
    doneTask: vi.fn(),
    startTask: vi.fn(),
    stopTask: vi.fn(),
  }
})

vi.mock(
  "@/features/workspace/project-workbench/tasks/task-create-dialog",
  () => ({
    TaskCreateDialog: ({
      open,
      projectSlug,
    }: {
      open: boolean
      projectSlug: string
    }) =>
      open ? (
        <div aria-label="任务创建" role="dialog">
          {projectSlug}
        </div>
      ) : null,
  })
)

const userMe: MeResponse = {
  actor_type: "user",
  actor: { id: "user-1", name: "alice", display_name: "张三" },
  effective_workspace: { slug: "acme", name: "示例工作区" },
  effective_role: "owner",
  token: { scopes: ["*"], type: "pat" },
}

function homeView(overrides: Partial<HomeView> = {}): HomeView {
  return {
    generated_at: 1_784_246_400,
    today: "2026-07-17",
    actor_type: "user",
    my_work: {
      open_count: 3,
      started_count: 1,
      overdue_count: 1,
      due_today_count: 1,
      high_priority_open_count: 2,
      items: [
        {
          task: {
            id: "task-1",
            uuid: "task-1",
            task_slug: "OPS-7",
            title: "确认上线清单",
            status: "pending",
            project: "ops",
            priority: "H",
            due: 1_784_303_999,
            start: 1_784_240_000,
          },
          reasons: ["started", "due_today", "high_priority"],
        },
      ],
    },
    project_attention: [
      {
        project: {
          id: "project-1",
          workspace_id: "workspace-1",
          slug: "ops",
          name: "上线准备",
          status: "active",
          task_count: 4,
          pending_count: 2,
          completed_count: 2,
          created_at: 1,
          modified_at: 2,
        },
        overdue_count: 1,
        high_priority_open_count: 1,
        wait_ready_count: 0,
        unassigned_open_count: 0,
        series_metrics: {
          recurring_series_count: 1,
          active_recurring_series_count: 1,
          open_recurring_occurrence_count: 1,
          overdue_recurring_occurrence_count: 1,
        },
        latest_update: {
          id: "annotation-1",
          project_id: "project-1",
          entry: 2,
          content: "已确认发布窗口",
          created_by: {
            type: "user",
            user: { id: "user-2", name: "bob", display_name: "李四" },
          },
          created_at: 2,
        },
      },
    ],
    ...overrides,
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

function renderPage(me: MeResponse = userMe, queryClient = makeQueryClient()) {
  render(
    renderWithRouter(
      <QueryClientProvider client={queryClient}>
        <HomePage me={me} />
      </QueryClientProvider>
    )
  )
  return queryClient
}

describe("HomePage", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    sessionStorage.clear()
    await i18n.changeLanguage("zh-CN")
    vi.mocked(getHome).mockResolvedValue(homeView())
    vi.mocked(listWorkspaceEffectiveConfig).mockResolvedValue([])
    vi.mocked(getProjects).mockResolvedValue([
      homeView().project_attention[0].project,
    ])
    vi.mocked(startTask).mockResolvedValue(homeView().my_work!.items[0].task)
    vi.mocked(stopTask).mockResolvedValue({
      ...homeView().my_work!.items[0].task,
      start: null,
    })
    vi.mocked(doneTask).mockResolvedValue({
      ...homeView().my_work!.items[0].task,
      status: "completed",
    })
  })

  it("renders personal work first and links every count to a stable preset", async () => {
    renderPage()

    expect(
      await screen.findByRole("heading", { name: "你好，张三" })
    ).toBeTruthy()
    expect(
      await screen.findByRole("heading", { name: "我的今日" })
    ).toBeTruthy()
    expect(screen.getByText("确认上线清单")).toBeTruthy()
    expect(screen.getByRole("heading", { name: "项目关注" })).toBeTruthy()
    expect(screen.getByText("已确认发布窗口")).toBeTruthy()
    expect(screen.queryByText("当前操作者")).toBeNull()
    expect(screen.queryByText("最近失败投递")).toBeNull()
    expect(screen.queryByText("最近审计")).toBeNull()
    expect(screen.queryByRole("heading", { name: "工作区信息" })).toBeNull()

    expect(
      document.querySelector('a[href="/my-tasks?tab=started"]')
    ).toBeTruthy()
    expect(
      document.querySelector('a[href="/my-tasks?tab=overdue"]')
    ).toBeTruthy()
    expect(document.querySelector('a[href="/my-tasks?tab=today"]')).toBeTruthy()
    expect(
      screen.getByRole("link", { name: "高优未完成 2" }).getAttribute("href")
    ).toBe("/my-tasks?priority=H&tab=incomplete")

    expect(
      screen.getByRole("link", { name: /确认上线清单/ }).getAttribute("href")
    ).toBe("/workspaces/acme/projects/ops/tasks/OPS-7?from=home")
  })

  it("runs stop and complete actions from a started task", async () => {
    renderPage()
    await screen.findByText("确认上线清单")

    await userEvent.click(screen.getByRole("button", { name: "停止 OPS-7" }))
    expect(stopTask).toHaveBeenCalledWith("acme", "OPS-7")

    await userEvent.click(screen.getByRole("button", { name: "完成 OPS-7" }))
    expect(doneTask).toHaveBeenCalledWith("acme", "OPS-7")
  })

  it("keeps a task visible and shows the backend error when an action fails", async () => {
    vi.mocked(doneTask).mockRejectedValue(new Error("permission denied"))
    renderPage()
    await screen.findByText("确认上线清单")

    await userEvent.click(screen.getByRole("button", { name: "完成 OPS-7" }))

    expect((await screen.findByRole("alert")).textContent).toContain(
      "permission denied"
    )
    expect(screen.getByText("确认上线清单")).toBeTruthy()
  })

  it("uses the stable occurrence reference for projected task links and actions", async () => {
    const projectedTask = {
      id: "occ:series-1:1784303999",
      uuid: undefined,
      title: "每日上线巡检",
      status: "pending",
      project: "ops",
      recurrence_info: {
        role: "occurrence",
        series_id: "series-1",
        series_status: "active",
        rule: "daily",
        recurrence_at: 1_784_303_999,
        materialization: "projected" as const,
      },
    }
    vi.mocked(getHome).mockResolvedValue(
      homeView({
        my_work: {
          open_count: 1,
          started_count: 0,
          overdue_count: 0,
          due_today_count: 1,
          high_priority_open_count: 0,
          items: [{ task: projectedTask, reasons: ["due_today"] }],
        },
      })
    )
    vi.mocked(startTask).mockResolvedValue(projectedTask)

    renderPage()

    const taskLink = await screen.findByRole("link", { name: /每日上线巡检/ })
    expect(taskLink.getAttribute("href")).toBe(
      "/workspaces/acme/projects/ops/tasks/occ%3Aseries-1%3A1784303999?from=home"
    )
    await userEvent.click(screen.getByRole("button", { name: /开始 ↻/ }))
    expect(startTask).toHaveBeenCalledWith("acme", "occ:series-1:1784303999")
  })

  it("shows workspace information only when effective values exist", async () => {
    vi.mocked(listWorkspaceEffectiveConfig).mockResolvedValue([
      {
        key: "notifications.default_sink",
        value: "飞书项目群",
        source: "workspace",
        definition: {
          key: "notifications.default_sink",
          value_type: "string",
          allowed_scopes: ["workspace"],
          label: "默认通知渠道",
          description: "",
          enum_values: [],
          default_value: null,
          required: false,
          secret: false,
          show_on_console_home: true,
          created_at: 0,
          modified_at: 0,
        },
        show_on_console_home: true,
        missing_required: false,
      },
    ])

    renderPage()

    expect(
      await screen.findByRole("heading", { name: "工作区信息" })
    ).toBeTruthy()
    expect(screen.getByText("默认通知渠道")).toBeTruthy()
    expect(screen.getByText("notifications.default_sink")).toBeTruthy()
    expect(screen.getByText("飞书项目群")).toBeTruthy()
  })

  it("shows the system identity home without a fake personal empty state", async () => {
    vi.mocked(getHome).mockResolvedValue(
      homeView({ actor_type: "tenant_access_token", my_work: null })
    )
    renderPage({ ...userMe, actor_type: "tenant_access_token" })

    expect(
      await screen.findByRole("heading", { name: "当前为系统身份" })
    ).toBeTruthy()
    expect(screen.getByText("系统身份没有个人任务。")).toBeTruthy()
    expect(screen.queryByRole("heading", { name: "我的今日" })).toBeNull()
    expect(screen.queryByText("今天没有需要优先处理的任务")).toBeNull()
    expect(
      screen.getAllByRole("link", { name: "查看项目" }).length
    ).toBeGreaterThan(0)
  })

  it("lets a writable user choose a project before opening task creation", async () => {
    renderPage()

    await userEvent.click(
      await screen.findByRole("button", { name: "新建任务" })
    )
    expect(screen.getByRole("dialog", { name: "选择项目" })).toBeTruthy()
    await userEvent.click(screen.getByRole("button", { name: "上线准备" }))

    expect(screen.getByRole("dialog", { name: "任务创建" })).toBeTruthy()
    expect(
      screen.getByRole("dialog", { name: "任务创建" }).textContent
    ).toContain("ops")
  })

  it("keeps a read-only home free of task write actions", async () => {
    renderPage({
      ...userMe,
      effective_role: "viewer",
      token: { scopes: ["task:read", "project:read"], type: "pat" },
    })

    await screen.findByText("确认上线清单")
    expect(screen.queryByRole("button", { name: "新建任务" })).toBeNull()
    expect(screen.queryByRole("button", { name: "停止 OPS-7" })).toBeNull()
    expect(screen.queryByRole("button", { name: "完成 OPS-7" })).toBeNull()
    expect(listWorkspaceEffectiveConfig).not.toHaveBeenCalled()
  })

  it("shows a retryable page error without falling back to the old overview", async () => {
    vi.mocked(getHome).mockRejectedValue(new Error("offline"))
    renderPage()

    expect(await screen.findByText("首页暂时无法加载")).toBeTruthy()
    expect(screen.getByRole("button", { name: "重试" })).toBeTruthy()
    expect(screen.queryByText("最近失败投递")).toBeNull()
  })
})
