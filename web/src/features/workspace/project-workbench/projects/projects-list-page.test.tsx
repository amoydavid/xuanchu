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

function renderPage(workspaceSlug = "acme") {
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
          <ProjectsListPage workspaceSlug={workspaceSlug} />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

function ok(data: unknown) {
  return Promise.resolve(new Response(JSON.stringify({ data }), { status: 200 }))
}

// projectsListFetchMock 同时返回 credentials/current 和 projects 列表。
function projectsListFetchMock(rows: unknown) {
  return vi.spyOn(globalThis, "fetch").mockImplementation((input) => {
    const url = typeof input === "string" ? input : (input as Request).url
    if (url.includes("/credentials/current")) {
      return ok({
        actor_type: "user",
        actor: { id: "u1", name: "alice" },
        token: { type: "pat", scopes: [] },
        effective_workspace: { slug: "acme" },
        effective_role: "owner",
      })
    }
    return ok(rows)
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
    expect(within(apiRow).getByRole("button", { name: "项目操作" })).toBeTruthy()
    expect(screen.getByText("Web Console")).toBeTruthy()
    expect(screen.getByText("规划中")).toBeTruthy()
    expect(screen.queryByText("planning")).toBeNull()

    await userEvent.click(apiRow)

    expect(navigateMock).toHaveBeenCalledWith({
      to: "/workspaces/$workspaceSlug/projects/$projectSlug",
      params: { workspaceSlug: "acme", projectSlug: "api" },
    })
    expect(
      fetchMock.mock.calls.some(([input]) => input === "/api/v1/projects?workspace=acme&status=all")
    ).toBe(true)
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
    await userEvent.click(within(apiRow).getByRole("button", { name: "项目操作" }))

    expect(screen.queryByRole("menuitem", { name: "详情" })).toBeNull()
    expect(screen.getByRole("menuitem", { name: "打开" })).toBeTruthy()
    expect(screen.getByRole("menuitem", { name: "设置" })).toBeTruthy()

    await userEvent.click(screen.getByRole("menuitem", { name: "打开" }))
    expect(navigateMock).toHaveBeenCalledWith({
      to: "/workspaces/$workspaceSlug/projects/$projectSlug",
      params: { workspaceSlug: "acme", projectSlug: "api" },
    })
  })

  it("opens create dialog, validates fields, posts the project, and navigates to the new project", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((_input, init) => {
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
        return ok([])
      })

    renderPage()

    await userEvent.click(await screen.findByRole("button", { name: "新建项目" }))
    await userEvent.click(screen.getByRole("button", { name: "创建项目" }))
    expect(await screen.findByText("名称不能为空")).toBeTruthy()

    await userEvent.type(screen.getByLabelText("Slug"), "A1")
    await userEvent.type(screen.getByLabelText("名称"), "运维项目")
    await userEvent.click(screen.getByRole("button", { name: "创建项目" }))
    expect(await screen.findByText("Slug 必须为 3-10 位小写字母或数字，并以字母开头")).toBeTruthy()

    await userEvent.clear(screen.getByLabelText("Slug"))
    await userEvent.type(screen.getByLabelText("Slug"), "ops")
    await userEvent.type(screen.getByLabelText("描述"), "值班协作")
    await userEvent.click(screen.getByRole("button", { name: "创建项目" }))

    await waitFor(() => {
      expect(fetchMock.mock.calls.some(([input, init]) => {
        return (
          input === "/api/v1/projects?workspace=acme" &&
          init?.method === "POST" &&
          init.body === JSON.stringify({
            slug: "ops",
            name: "运维项目",
            description: "值班协作",
          })
        )
      })).toBe(true)
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

  it("keeps dialog input and shows an error after create failure", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation((_input, init) => {
      if (init?.method === "POST") {
        return fail()
      }
      return ok([])
    })

    renderPage()

    await userEvent.click(await screen.findByRole("button", { name: "新建项目" }))
    await userEvent.type(screen.getByLabelText("Slug"), "ops")
    await userEvent.type(screen.getByLabelText("名称"), "运维项目")
    await userEvent.click(screen.getByRole("button", { name: "创建项目" }))

    expect(await screen.findByText("项目 slug 已存在")).toBeTruthy()
    expect((screen.getByLabelText("Slug") as HTMLInputElement).value).toBe("ops")
    expect((screen.getByLabelText("名称") as HTMLInputElement).value).toBe(
      "运维项目"
    )
  })
})
