import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import {
  deleteWorkspaceCustomField,
  listWorkspaceCustomFields,
  setWorkspaceCustomField,
} from "./custom-field-api"
import { WorkspaceCustomFieldsPage } from "./workspace-custom-fields-page"

vi.mock("./custom-field-api", async () => {
  const actual =
    await vi.importActual<typeof import("./custom-field-api")>(
      "./custom-field-api"
    )
  return {
    ...actual,
    deleteWorkspaceCustomField: vi.fn(),
    listWorkspaceCustomFields: vi.fn(),
    setWorkspaceCustomField: vi.fn(),
  }
})

function renderPage(canManage = true) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <WorkspaceCustomFieldsPage canManage={canManage} workspaceSlug="acme" />
    </QueryClientProvider>
  )
}

describe("WorkspaceCustomFieldsPage", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18n.changeLanguage("zh-CN")
    vi.mocked(listWorkspaceCustomFields).mockResolvedValue([
      {
        name: "estimate",
        type: "numeric",
        label: "工作量",
        values: ["1", "2"],
        default: "2",
        source: "database",
        task_value_count: 18,
        active_series_value_count: 2,
      },
      {
        name: "source",
        type: "string",
        label: "来源",
        values: [],
        default: "",
        source: "runtime",
        task_value_count: 0,
        active_series_value_count: 0,
      },
    ])
    vi.mocked(setWorkspaceCustomField).mockResolvedValue({} as never)
    vi.mocked(deleteWorkspaceCustomField).mockResolvedValue(undefined)
  })

  it("shows typed usage and runtime override actions", async () => {
    renderPage()
    expect(await screen.findByText("工作量")).toBeTruthy()
    expect(screen.getByText("18 个任务 · 2 个活动系列")).toBeTruthy()
    expect(screen.getByText("运行时提供")).toBeTruthy()
    expect(screen.getByRole("button", { name: "创建覆盖" })).toBeTruthy()
    expect(screen.getAllByRole("button", { name: "删除" })).toHaveLength(1)

    await userEvent.click(screen.getByRole("button", { name: "创建覆盖" }))
    expect(screen.getByRole("dialog")).toBeTruthy()
    expect(screen.getByDisplayValue("source")).toBeTruthy()
  })

  it("is read-only for members and viewers", async () => {
    renderPage(false)
    expect(await screen.findByText("工作量")).toBeTruthy()
    expect(screen.queryByRole("button", { name: "新建字段" })).toBeNull()
    expect(screen.queryByRole("button", { name: "编辑" })).toBeNull()
    expect(screen.queryByRole("button", { name: "删除" })).toBeNull()
  })
})
