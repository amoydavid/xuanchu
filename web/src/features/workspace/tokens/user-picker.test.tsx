import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { setWorkspaceToken } from "@/features/workspace/session/workspace-token"
import { i18n } from "@/i18n"

import { UserPicker } from "./user-picker"

function renderPicker(props: React.ComponentProps<typeof UserPicker>) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <TooltipProvider>
          <UserPicker {...props} />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

function okResponse(data: unknown) {
  return Promise.resolve(
    new Response(JSON.stringify({ data }), { status: 200 })
  )
}

const meResponse = {
  actor_type: "user",
  actor: { id: "u-admin", name: "admin", display_name: "管理员" },
  token: { type: "pat", scopes: ["token:read", "token:write"] },
  effective_workspace: { slug: "local" },
  effective_role: "owner",
}

const members = [
  {
    id: "u-admin",
    name: "admin",
    display_name: "管理员",
    email: "admin@example.com",
    role: "owner",
    joined_at: 1,
    modified_at: 1,
  },
  {
    id: "u-zhang",
    name: "zhangsan",
    display_name: "张三",
    email: "zhangsan@example.com",
    role: "member",
    joined_at: 1,
    modified_at: 1,
  },
]

function mockFetch() {
  return vi.spyOn(globalThis, "fetch").mockImplementation((input) => {
    const url = String(input)
    if (url.includes("/api/v1/credentials/current")) return okResponse(meResponse)
    if (url.includes("/api/v1/workspaces/") && url.includes("/members")) {
      return okResponse(members)
    }
    return okResponse([])
  })
}

describe("UserPicker", () => {
  beforeEach(async () => {
    sessionStorage.clear()
    setWorkspaceToken("xuanchu_pat_test")
    await i18n.changeLanguage("zh-CN")
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("shows 'myself' as default when value is empty", async () => {
    mockFetch()
    renderPicker({ value: "", onChange: () => {} })

    await waitFor(() => {
      expect(screen.getByText(/我自己/)).toBeTruthy()
    })
  })

  it("opens dropdown and lists members", async () => {
    mockFetch()
    const user = userEvent.setup()
    renderPicker({ value: "", onChange: () => {} })

    await waitFor(() => {
      expect(screen.getByText(/我自己/)).toBeTruthy()
    })

    await user.click(screen.getByRole("combobox"))
    await waitFor(() => {
      expect(screen.getByText("张三")).toBeTruthy()
    })
  })

  it("filters members by search input", async () => {
    mockFetch()
    const user = userEvent.setup()
    renderPicker({ value: "", onChange: () => {} })

    await waitFor(() => {
      expect(screen.getByText(/我自己/)).toBeTruthy()
    })
    await user.click(screen.getByRole("combobox"))

    await user.type(screen.getByPlaceholderText(/搜索成员/), "zhangsan")
    await waitFor(() => {
      expect(screen.getByText("张三")).toBeTruthy()
    })
    // 「我自己（管理员）」不匹配 zhangsan，应被过滤掉
    expect(screen.queryByText("管理员")).toBeNull()
  })

  it("calls onChange with member user_id when selected", async () => {
    const fetchMock = mockFetch()
    const user = userEvent.setup()
    const onChange = vi.fn()
    renderPicker({ value: "", onChange })

    await waitFor(() => {
      expect(screen.getByText(/我自己/)).toBeTruthy()
    })
    await user.click(screen.getByRole("combobox"))

    await waitFor(() => {
      expect(screen.getByText("张三")).toBeTruthy()
    })
    await user.click(screen.getByText("张三"))

    expect(onChange).toHaveBeenCalledWith("u-zhang")
    fetchMock.mockRestore()
  })
})
