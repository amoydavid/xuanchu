import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor, within } from "@testing-library/react"
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

function deferred<T>() {
  let resolve: (value: T) => void = () => undefined
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise
  })
  return { promise, resolve }
}

function listData(items = [template]) {
  return { items, total: items.length, limit: 20, offset: 0 }
}

function detailData(templateValue = template) {
  return {
    template: templateValue,
    snapshot: {
      project: { description: "" },
      configs: [],
      tasks: [],
      series: [],
      automations: [],
    },
    versions: [current, historical],
  }
}

function renderLibrary({
  canManage = true,
  initialSearch = "",
  writeScopes = ["project:write", "task:write", "config:write", "hook:write"],
}: {
  canManage?: boolean
  initialSearch?: string
  writeScopes?: string[]
} = {}) {
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
            writeScopes={writeScopes}
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

  it("keeps the detail fixture aligned with the Task 8 response contract", () => {
    expect(detailData()).not.toHaveProperty("selected_snapshot")
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

  it("treats a status-only empty result as no match and clears all filters", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation(() => response(listData([])))
    renderLibrary()

    expect(await screen.findByText("还没有项目模板")).toBeTruthy()
    const statusSelect = screen.getByRole("combobox", {
      name: "模板状态",
    }) as HTMLSelectElement
    await userEvent.selectOptions(statusSelect, "archived")
    expect(await screen.findByText("没有匹配的项目模板")).toBeTruthy()
    expect(screen.queryByRole("button", { name: "保存新模板" })).toBeNull()

    const searchInput = screen.getByRole("textbox", {
      name: "搜索模板",
    }) as HTMLInputElement
    await userEvent.type(searchInput, "不存在")
    await userEvent.click(screen.getByRole("button", { name: "搜索" }))
    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/v1/project-templates?workspace=acme&status=archived&q=%E4%B8%8D%E5%AD%98%E5%9C%A8&limit=20&offset=0",
        expect.anything()
      )
    )
    await userEvent.click(screen.getByRole("button", { name: "清除筛选条件" }))
    expect(await screen.findByText("还没有项目模板")).toBeTruthy()
    expect(
      (
        screen.getByRole("combobox", {
          name: "模板状态",
        }) as HTMLSelectElement
      ).value
    ).toBe("all")
    expect(
      (screen.getByRole("textbox", { name: "搜索模板" }) as HTMLInputElement)
        .value
    ).toBe("")
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
        if (url.includes("snapshot_id=snap-1")) return response(detailData())
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
    expect(screen.queryByRole("button", { name: "编辑模板信息" })).toBeNull()
    expect(screen.queryByRole("button", { name: "从模板创建" })).toBeNull()
    expect(screen.queryByRole("button", { name: "从项目更新快照" })).toBeNull()
  })

  it.each([
    ["task:write", ["project:write", "config:write", "hook:write"]],
    ["config:write", ["project:write", "task:write", "hook:write"]],
    ["hook:write", ["project:write", "task:write", "config:write"]],
  ])(
    "hides instantiate when a non-empty snapshot lacks %s but keeps lifecycle governance",
    async (_missing, writeScopes) => {
      vi.spyOn(globalThis, "fetch").mockImplementation((input) =>
        String(input).includes("/launch?")
          ? response(detailData())
          : response(listData())
      )
      renderLibrary({ canManage: true, writeScopes })

      expect(await screen.findByRole("button", { name: "归档" })).toBeTruthy()
      expect(
        screen.getByRole("button", { name: "从项目更新快照" })
      ).toBeTruthy()
      expect(screen.queryByRole("button", { name: "从模板创建" })).toBeNull()
    }
  )

  it("allows instantiate without component scopes when selected counts are zero", async () => {
    const zeroSnapshot = {
      ...current,
      id: "snap-zero",
      counts: { tasks: 0, series: 0, configs: 0, automations: 0 },
    }
    const zeroTemplate = { ...template, current_snapshot: zeroSnapshot }
    vi.spyOn(globalThis, "fetch").mockImplementation((input) =>
      String(input).includes("/launch?")
        ? response({
            ...detailData(zeroTemplate),
            versions: [zeroSnapshot],
          })
        : response(listData([zeroTemplate]))
    )
    renderLibrary({ canManage: true, writeScopes: ["project:write"] })

    expect(
      await screen.findByRole("button", { name: "从模板创建" })
    ).toBeTruthy()
  })

  it("selects a source project before opening the shared snapshot wizard", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation((input) => {
      const url = String(input)
      if (url === "/api/v1/projects?workspace=acme&status=all") {
        return response([
          {
            id: "project-1",
            workspace_id: "workspace-1",
            slug: "launch-source",
            name: "上线来源项目",
            status: "active",
            task_count: 20,
            pending_count: 8,
            completed_count: 12,
            created_at: 1,
            modified_at: 1,
          },
        ])
      }
      return url.includes("/launch?")
        ? response(detailData())
        : response(listData())
    })
    renderLibrary()

    await userEvent.click(
      await screen.findByRole("button", { name: "从项目更新快照" })
    )
    expect(screen.getByRole("dialog", { name: "选择来源项目" })).toBeTruthy()
    await userEvent.click(screen.getByRole("button", { name: /上线来源项目/ }))

    const wizard = screen.getByRole("dialog", { name: "从项目更新快照" })
    expect(wizard).toBeTruthy()
    expect(within(wizard).getAllByText(/launch-source/).length).toBeGreaterThan(
      0
    )
  })

  it("archives an active template through the typed endpoint", async () => {
    let archived = false
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
          url.endsWith("/archive?workspace=acme") &&
          init?.method === "POST"
        ) {
          archived = true
          return response(detailData(archivedTemplate))
        }
        if (url.includes("/launch?")) {
          return response(detailData(archived ? archivedTemplate : template))
        }
        return response(listData([archived ? archivedTemplate : template]))
      })
    renderLibrary()

    await userEvent.click(await screen.findByRole("button", { name: "归档" }))
    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/v1/project-templates/launch/archive?workspace=acme",
        expect.objectContaining({ method: "POST" })
      )
    )
    expect(
      await screen.findByText("此模板已归档，只能查看历史版本。")
    ).toBeTruthy()
    expect(screen.queryByRole("button", { name: "从模板创建" })).toBeNull()
    expect(screen.queryByRole("button", { name: "从项目更新快照" })).toBeNull()
    expect(screen.getByRole("button", { name: "编辑模板信息" })).toBeTruthy()
    expect(screen.getByRole("button", { name: "重新激活" })).toBeTruthy()
  })

  it("allows managers to modify archived template metadata and refetches", async () => {
    let modified = false
    const archivedTemplate = {
      ...template,
      status: "archived" as const,
      archived_at: 1_721_548_800,
    }
    const updatedTemplate = {
      ...archivedTemplate,
      name: "已归档新版上线流程",
      description: "已归档后的模板说明",
      modified_at: template.modified_at + 1,
    }
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((input, init) => {
        const url = String(input)
        if (
          url === "/api/v1/project-templates/launch?workspace=acme" &&
          init?.method === "PATCH"
        ) {
          modified = true
          return response(detailData(updatedTemplate))
        }
        if (url.includes("/launch?")) {
          return response(detailData(modified ? updatedTemplate : archivedTemplate))
        }
        return response(listData([modified ? updatedTemplate : archivedTemplate]))
      })
    renderLibrary()

    await userEvent.click(
      await screen.findByRole("button", { name: "编辑模板信息" })
    )
    const dialog = screen.getByRole("dialog", { name: "编辑模板信息" })
    await userEvent.clear(within(dialog).getByLabelText("模板名称"))
    await userEvent.type(
      within(dialog).getByLabelText("模板名称"),
      "已归档新版上线流程"
    )
    await userEvent.clear(within(dialog).getByLabelText("模板说明"))
    await userEvent.type(
      within(dialog).getByLabelText("模板说明"),
      "已归档后的模板说明"
    )
    await userEvent.click(
      within(dialog).getByRole("button", { name: "保存修改" })
    )

    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/v1/project-templates/launch?workspace=acme",
        expect.objectContaining({
          body: JSON.stringify({
            name: "已归档新版上线流程",
            description: "已归档后的模板说明",
          }),
          method: "PATCH",
        })
      )
    )
    expect(await screen.findAllByText("已归档新版上线流程")).not.toHaveLength(0)
    expect(await screen.findByText("已归档后的模板说明")).toBeTruthy()
    expect(
      fetchMock.mock.calls.filter(([url]) => String(url).includes("/launch?"))
        .length
    ).toBeGreaterThan(1)
  })

  it("modifies reachable template metadata and refetches list and detail", async () => {
    let modified = false
    const updatedTemplate = {
      ...template,
      name: "新版上线流程",
      description: "更新后的模板说明",
      modified_at: template.modified_at + 1,
    }
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((input, init) => {
        const url = String(input)
        if (
          url === "/api/v1/project-templates/launch?workspace=acme" &&
          init?.method === "PATCH"
        ) {
          modified = true
          return response(detailData(updatedTemplate))
        }
        if (url.includes("/launch?")) {
          return response(detailData(modified ? updatedTemplate : template))
        }
        return response(listData([modified ? updatedTemplate : template]))
      })
    renderLibrary()

    await userEvent.click(
      await screen.findByRole("button", { name: "编辑模板信息" })
    )
    const dialog = screen.getByRole("dialog", { name: "编辑模板信息" })
    await userEvent.clear(within(dialog).getByLabelText("模板名称"))
    await userEvent.type(
      within(dialog).getByLabelText("模板名称"),
      "新版上线流程"
    )
    await userEvent.clear(within(dialog).getByLabelText("模板说明"))
    await userEvent.type(
      within(dialog).getByLabelText("模板说明"),
      "更新后的模板说明"
    )
    await userEvent.click(
      within(dialog).getByRole("button", { name: "保存修改" })
    )

    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/v1/project-templates/launch?workspace=acme",
        expect.objectContaining({
          body: JSON.stringify({
            name: "新版上线流程",
            description: "更新后的模板说明",
          }),
          method: "PATCH",
        })
      )
    )
    expect(await screen.findAllByText("新版上线流程")).not.toHaveLength(0)
    expect(await screen.findByText("更新后的模板说明")).toBeTruthy()
    expect(
      fetchMock.mock.calls.filter(([url]) => String(url).includes("/launch?"))
        .length
    ).toBeGreaterThan(1)
  })

  it("keeps metadata edit open, disabled, and retryable after failure", async () => {
    const pending = deferred<Response>()
    let attempts = 0
    vi.spyOn(globalThis, "fetch").mockImplementation((input, init) => {
      const url = String(input)
      if (url.endsWith("/launch?workspace=acme") && init?.method === "PATCH") {
        attempts += 1
        return attempts === 1
          ? pending.promise
          : response(detailData({ ...template, name: "重试成功" }))
      }
      return url.includes("/launch?")
        ? response(detailData())
        : response(listData())
    })
    renderLibrary()

    await userEvent.click(
      await screen.findByRole("button", { name: "编辑模板信息" })
    )
    const dialog = screen.getByRole("dialog", { name: "编辑模板信息" })
    await userEvent.clear(within(dialog).getByLabelText("模板名称"))
    await userEvent.type(within(dialog).getByLabelText("模板名称"), "重试成功")
    await userEvent.click(
      within(dialog).getByRole("button", { name: "保存修改" })
    )
    expect(
      (
        within(dialog).getByRole("button", {
          name: "正在保存",
        }) as HTMLButtonElement
      ).disabled
    ).toBe(true)

    pending.resolve(await response({}, 500))
    expect(await within(dialog).findByText("修改模板信息失败")).toBeTruthy()
    await userEvent.click(
      within(dialog).getByRole("button", { name: "保存修改" })
    )
    await waitFor(() => expect(attempts).toBe(2))
    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: "编辑模板信息" })).toBeNull()
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
    let archived = true
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
          archived = false
          return response(detailData(template))
        }
        if (url.includes("/launch?")) {
          return response(detailData(archived ? archivedTemplate : template))
        }
        return response(listData([archived ? archivedTemplate : template]))
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
    await waitFor(() =>
      expect(screen.queryByText("此模板已归档，只能查看历史版本。")).toBeNull()
    )
    expect(screen.getByRole("button", { name: "从模板创建" })).toBeTruthy()
    expect(screen.getByRole("button", { name: "从项目更新快照" })).toBeTruthy()
    expect(screen.getByRole("button", { name: "归档" })).toBeTruthy()
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
          <ProjectTemplateLibraryPage
            canManage
            writeScopes={[
              "project:write",
              "task:write",
              "config:write",
              "hook:write",
            ]}
            workspaceSlug="acme"
          />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}
