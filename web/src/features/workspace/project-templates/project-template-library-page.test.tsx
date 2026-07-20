import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import type { ReactNode } from "react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { setWorkspaceToken } from "@/features/workspace/session/workspace-token"
import { i18n } from "@/i18n"
import { enUS } from "@/locales/en-US"
import { zhCN } from "@/locales/zh-CN"

import { ProjectTemplateLibraryPage } from "./project-template-library-page"
import { WorkspaceSettingsNav } from "./workspace-settings-nav"

vi.mock("@tanstack/react-router", () => ({
  Link: ({
    children,
    className,
    to,
    ...props
  }: {
    children: ReactNode
    className?: string
    to: string
  }) => (
    <a className={className} href={to} {...props}>
      {children}
    </a>
  ),
}))

const current = {
  id: "snap-3",
  version: 3,
  hash: "hash-3",
  source_project_id: "project-ops",
  counts: { tasks: 12, series: 3, configs: 4, automations: 2 },
  required_secret_keys: ["provider.api_key"],
  created_by: {
    type: "user",
    user: {
      id: "user-1",
      name: "alice",
      display_name: "Alice",
      email: null,
      external_ids: [],
    },
  },
  created_at: 1_721_462_400,
}

const template = {
  id: "template-1",
  key: "launch",
  name: "标准上线流程",
  description: "上线前后的标准执行清单",
  status: "active",
  current_snapshot: current,
  created_by: current.created_by,
  created_at: 1_721_462_400,
  modified_at: 1_721_462_400,
}

const historical = {
  ...current,
  id: "snap-1",
  version: 1,
  hash: "hash-1",
  counts: { tasks: 8, series: 2, configs: 3, automations: 1 },
  created_at: 1_720_598_400,
}

function response(data: unknown, status = 200) {
  return Promise.resolve(
    new Response(
      JSON.stringify(status >= 400 ? { error: { code: "boom" } } : { data }),
      {
        headers: { "Content-Type": "application/json" },
        status,
      }
    )
  )
}

function listData(items = [template]) {
  return { items, total: items.length, limit: 20, offset: 0 }
}

function detailData(selected = current) {
  return {
    template,
    snapshot: {
      project: { description: "" },
      configs: [],
      tasks: [],
      series: [],
      automations: [],
    },
    selected_snapshot: selected,
    versions: [current, historical],
  }
}

function renderLibrary({
  canManage = true,
  initialSearch = "",
}: { canManage?: boolean; initialSearch?: string } = {}) {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <TooltipProvider>
          <ProjectTemplateLibraryPage
            canManage={canManage}
            initialSearch={initialSearch}
            workspaceSlug="acme"
          />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

describe("ProjectTemplateLibraryPage", () => {
  beforeEach(async () => {
    sessionStorage.clear()
    setWorkspaceToken("xuanchu_pat_test")
    await i18n.changeLanguage("zh-CN")
  })

  afterEach(() => {
    vi.restoreAllMocks()
    sessionStorage.clear()
  })

  it("renders an explicit loading skeleton", () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(
      () => new Promise(() => {})
    )
    renderLibrary()
    expect(
      screen.getByRole("status", { name: "正在加载项目模板" })
    ).toBeTruthy()
  })

  it("distinguishes API error from an empty template library and retries", async () => {
    let calls = 0
    vi.spyOn(globalThis, "fetch").mockImplementation(() => {
      calls += 1
      return calls === 1 ? response({}, 500) : response(listData([]))
    })
    renderLibrary()

    expect(await screen.findByText("加载项目模板失败")).toBeTruthy()
    expect(screen.queryByText("还没有项目模板")).toBeNull()
    await userEvent.click(screen.getByRole("button", { name: "重试" }))
    expect(await screen.findByText("还没有项目模板")).toBeTruthy()
  })

  it("shows empty CTA separately from no-search-result", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      response(listData([]))
    )
    const { unmount } = renderLibraryAndReturn()
    expect(await screen.findByText("还没有项目模板")).toBeTruthy()
    expect(screen.getByRole("button", { name: "保存新模板" })).toBeTruthy()

    unmount()
    renderLibrary({ initialSearch: "不存在" })
    expect(await screen.findByText("没有匹配的项目模板")).toBeTruthy()
    expect(screen.queryByText("还没有项目模板")).toBeNull()
  })

  it("keeps a responsive list-detail hierarchy and renders snapshot summaries", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation((input) =>
      String(input).includes("/launch?")
        ? response(detailData())
        : response(listData())
    )
    renderLibrary()

    expect(await screen.findByText("标准上线流程")).toBeTruthy()
    const layout = screen.getByTestId("project-template-library-layout")
    expect(layout.className).toContain("grid-cols-1")
    expect(layout.className).toContain("lg:grid-cols")
    expect(await screen.findByText("任务 12")).toBeTruthy()
    expect(await screen.findByText("循环任务 3")).toBeTruthy()
    expect(await screen.findByText("自动化（创建后停用） 2")).toBeTruthy()
    expect(await screen.findByText("project-ops")).toBeTruthy()
  })

  it("switches to a historical version and keeps it read-only but instantiable", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((input) => {
        const url = String(input)
        if (url.includes("snapshot_id=snap-1"))
          return response(detailData(historical))
        if (url.includes("/launch?")) return response(detailData())
        return response(listData())
      })
    renderLibrary()

    await userEvent.click(await screen.findByRole("button", { name: /v1/ }))
    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/v1/project-templates/launch?workspace=acme&snapshot_id=snap-1",
        expect.anything()
      )
    )
    expect(await screen.findByText("历史版本 · 只读")).toBeTruthy()
    expect(
      screen.getByRole("button", { name: "从此版本创建项目" })
    ).toBeTruthy()
    expect(screen.queryByRole("button", { name: "从项目更新快照" })).toBeNull()
  })

  it("hides governance actions without project management permission", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation((input) =>
      String(input).includes("/launch?")
        ? response(detailData())
        : response(listData())
    )
    renderLibrary({ canManage: false })

    expect(await screen.findByText("标准上线流程")).toBeTruthy()
    expect(screen.queryByRole("button", { name: "保存新模板" })).toBeNull()
    expect(screen.queryByRole("button", { name: "归档" })).toBeNull()
    expect(screen.queryByRole("button", { name: "从项目更新快照" })).toBeNull()
  })

  it("archives an active template through the typed endpoint", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((input, init) => {
        const url = String(input)
        if (
          url.endsWith("/archive?workspace=acme") &&
          init?.method === "POST"
        ) {
          return response({
            ...detailData(),
            template: { ...template, status: "archived" },
          })
        }
        if (url.includes("/launch?")) return response(detailData())
        return response(listData())
      })
    renderLibrary()

    await userEvent.click(await screen.findByRole("button", { name: "归档" }))
    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/v1/project-templates/launch/archive?workspace=acme",
        expect.objectContaining({ method: "POST" })
      )
    )
  })

  it("shows lifecycle failure without turning it into archived state", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation((input, init) => {
      const url = String(input)
      if (url.endsWith("/archive?workspace=acme") && init?.method === "POST") {
        return response({}, 500)
      }
      if (url.includes("/launch?")) return response(detailData())
      return response(listData())
    })
    renderLibrary()

    await userEvent.click(await screen.findByRole("button", { name: "归档" }))
    expect(await screen.findByText("更新项目模板生命周期失败")).toBeTruthy()
    expect(screen.queryByText("此模板已归档，只能查看历史版本。")).toBeNull()
  })

  it("shows archived lifecycle feedback, hides create actions, and reactivates", async () => {
    const archivedTemplate = {
      ...template,
      status: "archived",
      archived_at: 1_721_548_800,
    }
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((input, init) => {
        const url = String(input)
        if (
          url.endsWith("/reactivate?workspace=acme") &&
          init?.method === "POST"
        ) {
          return response({ ...detailData(), template })
        }
        if (url.includes("/launch?")) {
          return response({ ...detailData(), template: archivedTemplate })
        }
        return response(listData([archivedTemplate]))
      })
    renderLibrary()

    expect(
      await screen.findByText("此模板已归档，只能查看历史版本。")
    ).toBeTruthy()
    expect(screen.queryByRole("button", { name: "从模板创建" })).toBeNull()
    expect(screen.queryByRole("button", { name: "从项目更新快照" })).toBeNull()
    await userEvent.click(screen.getByRole("button", { name: "重新激活" }))
    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/v1/project-templates/launch/reactivate?workspace=acme",
        expect.objectContaining({ method: "POST" })
      )
    )
  })

  it("provides aligned settings navigation and complete multilingual keys", () => {
    render(<WorkspaceSettingsNav active="projectTemplates" />)
    expect(
      screen.getByRole("link", { name: "配置定义" }).getAttribute("href")
    ).toBe("/settings")
    const templates = screen.getByRole("link", { name: "项目模板" })
    expect(templates.getAttribute("href")).toBe("/settings/project-templates")
    expect(templates.getAttribute("aria-current")).toBe("page")

    expect(Object.keys(zhCN.projectTemplates).sort()).toEqual(
      Object.keys(enUS.projectTemplates).sort()
    )
  })

  it("renders the settings navigation in English", async () => {
    await i18n.changeLanguage("en-US")
    render(<WorkspaceSettingsNav active="configDefinitions" />)

    expect(
      screen.getByRole("link", { name: "Config Definitions" })
    ).toBeTruthy()
    expect(screen.getByRole("link", { name: "Project Templates" })).toBeTruthy()
  })
})

function renderLibraryAndReturn() {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <TooltipProvider>
          <ProjectTemplateLibraryPage canManage workspaceSlug="acme" />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}
