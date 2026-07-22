import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { setWorkspaceToken } from "@/features/workspace/session/workspace-token"
import { i18n } from "@/i18n"

import { ProjectsListPage } from "./projects-list-page"

const navigateMock = vi.hoisted(() => vi.fn())

vi.mock("@tanstack/react-router", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-router")>()
  return {
    ...actual,
    useNavigate: () => navigateMock,
  }
})

function renderPage(
  workspaceSlug = "acme",
  options: {
    view?: "current" | "closed"
    closedStatus?: "archived" | "cancelled"
  } = {}
) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })

  render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <TooltipProvider>
          <ProjectsListPage
            closedStatus={options.closedStatus}
            view={options.view}
            workspaceSlug={workspaceSlug}
          />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

function ok(data: unknown) {
  return Promise.resolve(
    new Response(JSON.stringify({ data }), { status: 200 })
  )
}

const fullWriteScopes = [
  "project:write",
  "task:write",
  "config:write",
  "hook:write",
]

function activeTemplatePage() {
  return {
    items: [
      {
        id: "template-1",
        key: "launch",
        name: "上线流程",
        description: "",
        status: "active",
        current_snapshot: {
          id: "snap-1",
          version: 1,
          hash: "hash-1",
          source_project_id: "project-1",
          counts: { tasks: 1, series: 0, configs: 0, automations: 0 },
          required_secret_keys: [],
          created_by: { type: "user", user: { id: "u1", name: "alice" } },
          created_at: 1,
        },
        created_by: { type: "user", user: { id: "u1", name: "alice" } },
        created_at: 1,
        modified_at: 1,
      },
    ],
    total: 1,
    limit: 20,
    offset: 0,
  }
}

function workspaceCredentials(scopes = fullWriteScopes) {
  return {
    actor_type: "user",
    actor: { id: "u1", name: "alice" },
    token: { type: "pat", scopes },
    effective_workspace: { slug: "acme" },
    effective_role: "owner",
  }
}

function projectPageResponse(
  url: string,
  rows: unknown,
  scopes = fullWriteScopes
) {
  if (url.includes("/credentials/current"))
    return ok(workspaceCredentials(scopes))
  if (url.startsWith("/api/v1/project-templates"))
    return ok(activeTemplatePage())
  return ok(rows)
}

// projectsListFetchMock 同时返回 credentials/current 和 projects 列表。
function projectsListFetchMock(rows: unknown) {
  return vi.spyOn(globalThis, "fetch").mockImplementation((input) => {
    const url = typeof input === "string" ? input : (input as Request).url
    return projectPageResponse(url, rows)
  })
}

function projectsListFetchMockWithoutTemplateScopes() {
  return vi.spyOn(globalThis, "fetch").mockImplementation((input) => {
    const url = typeof input === "string" ? input : (input as Request).url
    if (url.includes("/credentials/current"))
      return ok(workspaceCredentials(["project:write"]))
    if (url.startsWith("/api/v1/project-templates")) {
      return ok({
        items: [
          {
            id: "template-1",
            key: "launch",
            name: "上线流程",
            description: "",
            status: "active",
            current_snapshot: {
              id: "snap-1",
              version: 1,
              hash: "hash-1",
              source_project_id: "project-1",
              counts: { tasks: 1, series: 0, configs: 0, automations: 0 },
              required_secret_keys: [],
              created_by: { type: "user", user: { id: "u1", name: "alice" } },
              created_at: 1,
            },
            created_by: { type: "user", user: { id: "u1", name: "alice" } },
            created_at: 1,
            modified_at: 1,
          },
        ],
        total: 1,
        limit: 20,
        offset: 0,
      })
    }
    return ok([])
  })
}

function fail(code = "project_slug_exists") {
  return Promise.resolve(
    new Response(JSON.stringify({ error: { code } }), { status: 409 })
  )
}

describe("ProjectWorkbench ProjectsListPage", () => {
  beforeEach(async () => {
    sessionStorage.clear()
    setWorkspaceToken("xuanchu_pat_test")
    await i18n.changeLanguage("zh-CN")
  })

  afterEach(() => {
    vi.restoreAllMocks()
    navigateMock.mockReset()
    sessionStorage.clear()
  })

  it("renders project rows with status, progress, task counts, actions, and row navigation", async () => {
    const fetchMock = projectsListFetchMock([
      {
        id: "p1",
        workspace_id: "w1",
        slug: "api",
        name: "API Platform",
        description: "核心 API",
        status: "active",
        task_count: 4,
        pending_count: 2,
        completed_count: 2,
        created_at: 1,
        modified_at: 1,
      },
      {
        id: "p2",
        workspace_id: "w1",
        slug: "web",
        name: "Web Console",
        status: "planning",
        task_count: 0,
        pending_count: 0,
        completed_count: 0,
        created_at: 1,
        modified_at: 1,
      },
    ])

    renderPage()

    const apiRow = await screen.findByRole("row", { name: /API Platform/ })
    expect(within(apiRow).getByText("进行中")).toBeTruthy()
    expect(within(apiRow).queryByText("active")).toBeNull()
    expect(within(apiRow).getByText("50%")).toBeTruthy()
    expect(within(apiRow).getByText("2 / 4")).toBeTruthy()
    expect(
      within(apiRow).getByRole("button", { name: "项目操作" })
    ).toBeTruthy()
    expect(screen.getByText("Web Console")).toBeTruthy()
    expect(screen.getByText("规划中")).toBeTruthy()
    expect(screen.queryByText("planning")).toBeNull()

    await userEvent.click(apiRow)

    expect(navigateMock).toHaveBeenCalledWith({
      to: "/workspaces/$workspaceSlug/projects/$projectSlug",
      params: { workspaceSlug: "acme", projectSlug: "api" },
    })
    expect(
      fetchMock.mock.calls.some(
        ([input]) => input === "/api/v1/projects?workspace=acme&status=open"
      )
    ).toBe(true)
    expect(screen.getByRole("button", { name: "已关闭项目" })).toBeTruthy()
  })

  it("renders archived projects in the separate closed-project view", async () => {
    const fetchMock = projectsListFetchMock([
      {
        id: "p3",
        workspace_id: "w1",
        slug: "legacy",
        name: "旧系统迁移",
        status: "archived",
        task_count: 12,
        pending_count: 0,
        completed_count: 12,
        created_at: 1,
        modified_at: 1,
      },
    ])

    renderPage("acme", { view: "closed", closedStatus: "archived" })

    expect(await screen.findByText("已关闭项目")).toBeTruthy()
    expect(screen.getByText("旧系统迁移")).toBeTruthy()
    expect(screen.queryByRole("button", { name: "新建项目" })).toBeNull()
    expect(
      fetchMock.mock.calls.some(
        ([input]) => input === "/api/v1/projects?workspace=acme&status=archived"
      )
    ).toBe(true)

    await userEvent.click(screen.getByRole("tab", { name: "已取消" }))
    expect(navigateMock).toHaveBeenCalledWith({
      to: "/projects/closed",
      search: { status: "cancelled" },
    })

    const legacyRow = screen.getByRole("row", { name: /旧系统迁移/ })
    await userEvent.click(
      within(legacyRow).getByRole("button", { name: "项目操作" })
    )
    expect(
      screen.getByRole("menuitem", { name: "恢复到进行中" })
    ).toBeTruthy()
    expect(screen.queryByRole("menuitem", { name: "归档" })).toBeNull()
    expect(screen.queryByRole("menuitem", { name: "取消" })).toBeNull()
  })

  it("opens projects from the row actions menu with open + settings entries", async () => {
    projectsListFetchMock([
      {
        id: "p1",
        workspace_id: "w1",
        slug: "api",
        name: "API Platform",
        status: "active",
        task_count: 4,
        pending_count: 2,
        completed_count: 2,
        created_at: 1,
        modified_at: 1,
      },
    ])

    renderPage()

    const apiRow = await screen.findByRole("row", { name: /API Platform/ })
    await userEvent.click(
      within(apiRow).getByRole("button", { name: "项目操作" })
    )

    expect(screen.queryByRole("menuitem", { name: "详情" })).toBeNull()
    expect(screen.getByRole("menuitem", { name: "打开" })).toBeTruthy()
    expect(screen.getByRole("menuitem", { name: "设置" })).toBeTruthy()

    await userEvent.click(screen.getByRole("menuitem", { name: "打开" }))
    expect(navigateMock).toHaveBeenCalledWith({
      to: "/workspaces/$workspaceSlug/projects/$projectSlug",
      params: { workspaceSlug: "acme", projectSlug: "api" },
    })
  })

  it("uses a shadcn confirmation dialog before archiving from row actions", async () => {
    const fetchMock = projectsListFetchMock([
      {
        id: "p1",
        workspace_id: "w1",
        slug: "api",
        name: "API Platform",
        status: "active",
        task_count: 4,
        pending_count: 2,
        completed_count: 2,
        created_at: 1,
        modified_at: 1,
      },
    ])
    const nativeConfirm = vi.spyOn(window, "confirm")

    renderPage()

    const apiRow = await screen.findByRole("row", { name: /API Platform/ })
    await userEvent.click(
      within(apiRow).getByRole("button", { name: "项目操作" })
    )
    await userEvent.click(screen.getByRole("menuitem", { name: "归档" }))

    expect(screen.getByRole("alertdialog")).toBeTruthy()
    expect(screen.getByText("确认关闭项目")).toBeTruthy()
    expect(nativeConfirm).not.toHaveBeenCalled()
    expect(
      fetchMock.mock.calls.some(
        ([input, init]) =>
          input === "/api/v1/projects/api/transition?workspace=acme" &&
          init?.method === "POST"
      )
    ).toBe(false)

    await userEvent.click(screen.getByRole("button", { name: "关闭项目" }))

    await waitFor(() => {
      expect(
        fetchMock.mock.calls.some(
          ([input, init]) =>
            input === "/api/v1/projects/api/transition?workspace=acme" &&
            init?.method === "POST" &&
            init.body === JSON.stringify({ status: "archived" })
        )
      ).toBe(true)
    })
  })

  it("opens create dialog, validates fields, posts the project, and navigates to the new project", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((input, init) => {
        if (init?.method === "POST") {
          return ok({
            id: "p3",
            workspace_id: "w1",
            slug: "ops",
            name: "运维项目",
            status: "planning",
            task_count: 0,
            pending_count: 0,
            completed_count: 0,
            created_at: 1,
            modified_at: 1,
          })
        }
        const url = typeof input === "string" ? input : (input as Request).url
        return projectPageResponse(url, [])
      })

    renderPage()

    await userEvent.click(
      await screen.findByRole("button", { name: "新建项目" })
    )
    await userEvent.click(screen.getByRole("button", { name: "创建项目" }))
    expect(await screen.findByText("名称不能为空")).toBeTruthy()

    await userEvent.type(screen.getByLabelText("Slug"), "A1")
    await userEvent.type(screen.getByLabelText("名称"), "运维项目")
    await userEvent.click(screen.getByRole("button", { name: "创建项目" }))
    expect(
      await screen.findByText("Slug 必须为 3-10 位小写字母或数字，并以字母开头")
    ).toBeTruthy()

    await userEvent.clear(screen.getByLabelText("Slug"))
    await userEvent.type(screen.getByLabelText("Slug"), "ops")
    await userEvent.type(screen.getByLabelText("描述"), "值班协作")
    await userEvent.click(screen.getByRole("button", { name: "创建项目" }))

    await waitFor(() => {
      expect(
        fetchMock.mock.calls.some(([input, init]) => {
          return (
            input === "/api/v1/projects?workspace=acme" &&
            init?.method === "POST" &&
            init.body ===
              JSON.stringify({
                slug: "ops",
                name: "运维项目",
                description: "值班协作",
              })
          )
        })
      ).toBe(true)
    })
    await waitFor(() => {
      expect(navigateMock).toHaveBeenCalledWith({
        to: "/workspaces/$workspaceSlug/projects/$projectSlug",
        params: { workspaceSlug: "acme", projectSlug: "ops" },
      })
    })
  })

  it("places create-from-template beside new project", async () => {
    projectsListFetchMock([])
    renderPage()

    const templateButton = await screen.findByRole("button", {
      name: "从模板创建",
    })
    const createButton = screen.getByRole("button", { name: "新建项目" })
    expect(templateButton.parentElement).toBe(createButton.parentElement)

    await userEvent.click(templateButton)
    expect(
      await screen.findByRole("dialog", { name: "从模板创建项目" })
    ).toBeTruthy()
  })

  it("keeps create-from-template reachable when the first active snapshot needs a missing write scope", async () => {
    projectsListFetchMockWithoutTemplateScopes()
    renderPage()

    await screen.findByText("暂无项目")
    await userEvent.click(
      await screen.findByRole("button", { name: "从模板创建" })
    )
    expect(
      (await screen.findByRole("button", { name: /上线流程/ })).disabled
    ).toBe(true)
  })

  it("keeps dialog input and shows an error after create failure", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation((input, init) => {
      if (init?.method === "POST") {
        return fail()
      }
      const url = typeof input === "string" ? input : (input as Request).url
      return projectPageResponse(url, [])
    })

    renderPage()

    await userEvent.click(
      await screen.findByRole("button", { name: "新建项目" })
    )
    await userEvent.type(screen.getByLabelText("Slug"), "ops")
    await userEvent.type(screen.getByLabelText("名称"), "运维项目")
    await userEvent.click(screen.getByRole("button", { name: "创建项目" }))

    expect(await screen.findByText("项目 slug 已存在")).toBeTruthy()
    expect((screen.getByLabelText("Slug") as HTMLInputElement).value).toBe(
      "ops"
    )
    expect((screen.getByLabelText("名称") as HTMLInputElement).value).toBe(
      "运维项目"
    )
  })
})
