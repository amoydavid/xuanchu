import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { cleanup, render, screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import { MyTasksPage } from "./my-tasks-page"

const navigateMock = vi.fn()

vi.mock("@tanstack/react-router", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-router")>()
  return {
    ...actual,
    useNavigate: () => navigateMock,
    useSearch: () => ({
      priority: "H",
      q: "复盘",
      sort: "priority",
      tab: "completed",
    }),
  }
})

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <MyTasksPage
        actor={undefined}
        actorType="tenant_access_token"
        workspaceSlug="local"
      />
    </QueryClientProvider>
  )
}

afterEach(() => {
  cleanup()
})

describe("MyTasksPage", () => {
  it.each([
    ["zh-CN", ["未完成", "今日到期", "逾期", "无截止", "已完成"]],
    ["en-US", ["Incomplete", "Due today", "Overdue", "No due", "Completed"]],
  ])("renders localized preset tabs in %s", async (language, labels) => {
    await i18n.changeLanguage(language)
    renderPage()

    const tablist = screen.getByRole("tablist", {
      name: i18n.t("myTasks.tabsLabel"),
    })
    expect(tablist).toBeTruthy()
    expect(screen.getAllByRole("tab").map((tab) => tab.textContent)).toEqual(
      labels
    )
    expect(document.body.textContent).not.toContain("myTasks.tab.")
  })

  it("does not render a status selector that conflicts with the preset", async () => {
    await i18n.changeLanguage("zh-CN")
    renderPage()

    expect(screen.queryByLabelText(i18n.t("common.status"))).toBeNull()
  })

  it("restores filters from route search", async () => {
    await i18n.changeLanguage("zh-CN")
    renderPage()

    expect(
      screen.getByRole("tab", { name: "已完成" }).getAttribute("data-state")
    ).toBe("active")
    expect(
      (screen.getByRole("textbox", { name: "搜索" }) as HTMLInputElement)
        .value
    ).toBe("复盘")
    expect(
      screen.getByRole("combobox", { name: "优先级" }).textContent
    ).toContain("H")
    expect(screen.getByRole("combobox", { name: "排序" }).textContent).toContain(
      "优先级"
    )
  })
})
