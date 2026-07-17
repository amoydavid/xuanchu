import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import type { ReactNode } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import { useTaskAuditQuery } from "../hooks/use-task-detail-data"
import { ActivitySection } from "./activity-section"

vi.mock("../hooks/use-task-detail-data", () => ({
  useTaskAuditQuery: vi.fn(),
}))

function makeWrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
  }
}

describe("TaskChangeHistory", () => {
  beforeEach(async () => {
    await i18n.changeLanguage("zh-CN")
    vi.mocked(useTaskAuditQuery).mockReturnValue({
      data: [],
      isError: false,
      isPending: false,
    } as unknown as ReturnType<typeof useTaskAuditQuery>)
  })

  it("uses the activity title and keeps an empty history cardless", () => {
    render(
      <ActivitySection
        annotations={[]}
        canWrite={false}
        projectSlug="adsops"
        taskRef="ads-1"
        workspaceSlug="acme"
      />,
      {
        wrapper: makeWrapper(
          new QueryClient({
            defaultOptions: { queries: { retry: false } },
          })
        ),
      }
    )

    expect(screen.getByRole("heading", { name: "活动" })).toBeTruthy()
    expect(screen.queryByRole("heading", { name: "注解" })).toBeNull()
    expect(screen.queryByRole("heading", { name: "变更历史" })).toBeNull()

    const empty = screen.getByText("暂无字段级变更记录")
    expect(empty).toBeTruthy()
    expect(empty.closest("section")?.className).not.toMatch(/border|bg-card/)
  })
})
