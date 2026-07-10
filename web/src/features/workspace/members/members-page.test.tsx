import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { setWorkspaceToken } from "@/features/workspace/session/workspace-token"
import { i18n } from "@/i18n"

import { MembersDetailPage, MembersPage } from "./members-page"

function renderPage({
  role = "owner",
  capabilities = ["member:read", "member:write"],
  workspaceSlug = "dajee",
}: {
  role?: string
  capabilities?: string[]
  workspaceSlug?: string
} = {}) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <TooltipProvider>
          <MembersPage
            credential={{
              actor: { id: "u1", name: "alice", display_name: "Alice Chen" },
              actor_type: "user",
              capabilities,
              effective_role: role,
              effective_workspace: { slug: workspaceSlug, name: workspaceSlug },
              token: { scopes: capabilities, type: "pat" },
            }}
            workspaceSlug={workspaceSlug}
          />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

function renderDetailPage({
  role = "owner",
  capabilities = ["member:read", "member:write", "audit:read", "token:read"],
  userRef = "u2",
  workspaceSlug = "dajee",
}: {
  role?: string
  capabilities?: string[]
  userRef?: string
  workspaceSlug?: string
} = {}) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <TooltipProvider>
          <MembersDetailPage
            credential={{
              actor: { id: "u1", name: "alice", display_name: "Alice Chen" },
              actor_type: "user",
              capabilities,
              effective_role: role,
              effective_workspace: { slug: workspaceSlug, name: workspaceSlug },
              token: { scopes: capabilities, type: "pat" },
            }}
            userRef={userRef}
            workspaceSlug={workspaceSlug}
          />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

function okResponse(data: unknown, status = 200) {
  return Promise.resolve(new Response(JSON.stringify({ data }), { status }))
}

describe("MembersPage", () => {
  beforeEach(async () => {
    window.history.pushState({}, "", "/members")
    sessionStorage.clear()
    setWorkspaceToken("xuanchu_pat_test")
    await i18n.changeLanguage("zh-CN")
  })

  afterEach(() => {
    vi.restoreAllMocks()
    sessionStorage.clear()
  })

  it("updates a member display name through the workspace member endpoint", async () => {
    const user = userEvent.setup()
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((input, init) => {
        const url = String(input)
        const method = (init?.method ?? "GET").toUpperCase()
        if (
          url.includes("/api/v1/workspaces/dajee/members/u2") &&
          method === "PATCH"
        ) {
          return okResponse({
            id: "u2",
            name: "bob",
            display_name: "李四",
            email: "bob@example.com",
            role: "admin",
            joined_at: 110,
            modified_at: 2,
          })
        }
        return okResponse(memberRows())
      })

    renderPage()

    await screen.findByText("Bob Li")
    await user.click(screen.getByRole("button", { name: "打开 Bob Li 的成员操作" }))
    await user.click(screen.getByRole("menuitem", { name: "编辑显示姓名" }))
    await user.clear(screen.getByLabelText("显示姓名"))
    await user.type(screen.getByLabelText("显示姓名"), "李四")
    await user.click(screen.getByRole("button", { name: "保存" }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        expect.stringContaining("/api/v1/workspaces/dajee/members/u2"),
        expect.objectContaining({
          method: "PATCH",
          body: JSON.stringify({ display_name: "李四" }),
        })
      )
    })
  })

  it("adds an existing member from a shadcn dialog", async () => {
    const user = userEvent.setup()
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((_input, init) => {
        const method = (init?.method ?? "GET").toUpperCase()
        if (method === "POST") {
          return okResponse({ ok: true }, 201)
        }
        return okResponse(memberRows())
      })

    renderPage()

    await screen.findByText("Alice Chen")
    await user.click(screen.getByRole("button", { name: "添加成员" }))
    await user.type(screen.getByLabelText("用户"), "carol")
    await user.click(screen.getByRole("button", { name: "添加" }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        expect.stringContaining("/api/v1/workspaces/dajee/members"),
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ user: "carol", role: "member" }),
        })
      )
    })
  })

  it("exposes member detail navigation from the row menu", async () => {
    const user = userEvent.setup()
    vi.spyOn(globalThis, "fetch").mockImplementation(() => okResponse(memberRows()))

    renderPage()

    await screen.findByText("Bob Li")
    await user.click(screen.getByRole("button", { name: "打开 Bob Li 的成员操作" }))
    expect(screen.getByRole("menuitem", { name: "详情" }).getAttribute("href")).toBe(
      "/members/u2"
    )
  })

  it("opens the member detail page when clicking a member row", async () => {
    const user = userEvent.setup()
    vi.spyOn(globalThis, "fetch").mockImplementation(() => okResponse(memberRows()))

    renderPage()

    await screen.findByText("Bob Li")
    const row = screen.getByText("Bob Li").closest("tr")
    expect(row).not.toBeNull()
    await user.click(row as HTMLTableRowElement)

    expect(window.location.pathname).toBe("/members/u2")
  })

  it("creates a new user and member from the add member dialog", async () => {
    const user = userEvent.setup()
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((_input, init) => {
        const method = (init?.method ?? "GET").toUpperCase()
        if (method === "POST") {
          return okResponse({ ok: true }, 201)
        }
        return okResponse(memberRows())
      })

    renderPage()

    await screen.findByText("Alice Chen")
    await user.click(screen.getByRole("button", { name: "添加成员" }))
    await user.click(screen.getByRole("combobox", { name: "成员来源" }))
    await user.click(screen.getByRole("option", { name: "创建新用户" }))
    await user.type(screen.getByLabelText("稳定用户名"), "carol")
    await user.type(screen.getByLabelText("显示姓名"), "Carol Wang")
    await user.type(screen.getByLabelText("邮箱"), "carol@example.com")
    await user.click(screen.getByRole("button", { name: "添加" }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        expect.stringContaining("/api/v1/workspaces/dajee/members"),
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({
            new_user: {
              display_name: "Carol Wang",
              email: "carol@example.com",
              name: "carol",
            },
            role: "member",
          }),
        })
      )
    })
  })

  it("lets admins manage non-owner rows but not owner rows", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(() => okResponse(memberRows()))

    renderPage({ role: "admin" })

    await screen.findByText("Alice Chen")
    const ownerRow = screen.getByText("Alice Chen").closest("tr")
    const adminRow = screen.getByText("Bob Li").closest("tr")
    expect(ownerRow).not.toBeNull()
    expect(adminRow).not.toBeNull()
    expect(
      within(ownerRow as HTMLTableRowElement).queryByRole("button", {
        name: "打开 Alice Chen 的成员操作",
      })
    ).toBeNull()
    expect(
      within(adminRow as HTMLTableRowElement).getByRole("button", {
        name: "打开 Bob Li 的成员操作",
      })
    ).not.toBeNull()
  })

  it("renders member role as readonly", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(() => okResponse(memberRows()))

    renderPage({ capabilities: ["member:read"], role: "member" })

    await screen.findByText("Alice Chen")
    expect(screen.queryByRole("button", { name: "添加成员" })).toBeNull()
    expect(
      screen.queryByRole("button", { name: "打开 Bob Li 的成员操作" })
    ).toBeNull()
    expect(screen.getByText("只读")).not.toBeNull()
  })

  it("localizes role labels and detail field labels in English", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation((input) => {
      const url = String(input)
      if (url.includes("/api/v1/users/u2")) {
        return okResponse({
          active: true,
          created_at: 100,
          display_name: "Bob Li",
          email: "bob@example.com",
          external_ids: [],
          id: "u2",
          modified_at: 120,
          name: "bob",
        })
      }
      if (url.includes("/api/v1/audit") || url.includes("/api/v1/tokens")) {
        return okResponse([])
      }
      return okResponse(memberRows())
    })

    await i18n.changeLanguage("en-US")
    renderPage()

    await screen.findByText("Alice Chen")
    expect(screen.getByText("Owner 1")).not.toBeNull()
    expect(screen.getByText("Admin 1")).not.toBeNull()
    expect(screen.getAllByText("Owner").length).toBeGreaterThan(0)
    expect(screen.getAllByText("Admin").length).toBeGreaterThan(0)
    expect(screen.queryByText("owner 1")).toBeNull()
    expect(screen.queryByText("admin 1")).toBeNull()

    renderDetailPage()

    await screen.findByRole("heading", { name: "Bob Li" })
    expect(screen.getByText("User ID")).not.toBeNull()
    expect(screen.getAllByText("Role").length).toBeGreaterThan(0)
    expect(screen.getAllByText("Admin").length).toBeGreaterThan(0)
    expect(screen.queryByText("admin")).toBeNull()
  })

  it("localizes member detail identity labels in Chinese", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation((input) => {
      const url = String(input)
      if (url.includes("/api/v1/users/u2")) {
        return okResponse({
          active: true,
          created_at: 100,
          display_name: "Bob Li",
          email: "bob@example.com",
          external_ids: [],
          id: "u2",
          modified_at: 120,
          name: "bob",
        })
      }
      if (url.includes("/api/v1/audit") || url.includes("/api/v1/tokens")) {
        return okResponse([])
      }
      return okResponse(memberRows())
    })

    renderDetailPage()

    await screen.findByRole("heading", { name: "Bob Li" })
    expect(screen.getByText("用户 ID")).not.toBeNull()
    expect(screen.getAllByText("管理员").length).toBeGreaterThan(0)
    expect(screen.queryByText("User ID")).toBeNull()
    expect(screen.queryByText("admin")).toBeNull()
  })

  it("removes a member through an AlertDialog confirmation", async () => {
    const user = userEvent.setup()
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((_input, init) => {
        const method = (init?.method ?? "GET").toUpperCase()
        if (method === "DELETE") {
          return okResponse({ ok: true })
        }
        return okResponse(memberRows())
      })

    renderPage()

    await screen.findByText("Bob Li")
    await user.click(screen.getByRole("button", { name: "打开 Bob Li 的成员操作" }))
    await user.click(screen.getByRole("menuitem", { name: "移出 workspace" }))
    await user.click(screen.getByRole("button", { name: "移出成员" }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        expect.stringContaining("/api/v1/workspaces/dajee/members/u2"),
        expect.objectContaining({ method: "DELETE" })
      )
    })
  })

  it("confirms owner promotion before changing role", async () => {
    const user = userEvent.setup()
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((_input, init) => {
        const method = (init?.method ?? "GET").toUpperCase()
        if (method === "PATCH") {
          return okResponse({
            id: "u2",
            name: "bob",
            display_name: "Bob Li",
            email: "bob@example.com",
            role: "owner",
            joined_at: 110,
            modified_at: 2,
          })
        }
        return okResponse(memberRows())
      })

    renderPage()

    await screen.findByText("Bob Li")
    await user.click(screen.getByRole("button", { name: "打开 Bob Li 的成员操作" }))
    await user.click(screen.getByRole("menuitem", { name: "调整角色" }))
    await user.click(screen.getByRole("combobox", { name: "角色" }))
    await user.click(screen.getByRole("option", { name: "所有者" }))
    await user.click(screen.getByRole("button", { name: "继续" }))
    await user.click(screen.getByRole("button", { name: "继续" }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        expect.stringContaining("/api/v1/workspaces/dajee/members/u2"),
        expect.objectContaining({
          method: "PATCH",
          body: JSON.stringify({ role: "owner" }),
        })
      )
    })
  })

  it("renders the secondary member detail page with identity, token, and audit sections", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation((input) => {
      const url = String(input)
      if (url.includes("/api/v1/users/u2")) {
        return okResponse({
          active: true,
          created_at: 100,
          display_name: "Bob Li",
          email: "bob@example.com",
          external_ids: [{ provider: "feishu", external_id: "ou_bob" }],
          id: "u2",
          modified_at: 120,
          name: "bob",
        })
      }
      if (url.includes("/api/v1/audit")) {
        return okResponse([
          {
            action: "member.role",
            created_at: 130,
            id: 1,
            target_id: "u2",
            target_type: "member",
          },
        ])
      }
      if (url.includes("/api/v1/tokens")) {
        return okResponse([
          {
            id: "tok1",
            name: "bob-token",
            prefix: "xuanchu_pat_abc",
            revoked_at: null,
            type: "pat",
            user: { id: "u2", name: "bob" },
          },
        ])
      }
      return okResponse(memberRows())
    })

    renderDetailPage()

    await screen.findByRole("heading", { name: "Bob Li" })
    expect(screen.getByText("成员信息")).not.toBeNull()
    expect(screen.getByText("外部身份")).not.toBeNull()
    expect(screen.getByText("feishu: ou_bob")).not.toBeNull()
    expect(screen.getByText("关联令牌摘要")).not.toBeNull()
    expect(screen.getByText("有效 1")).not.toBeNull()
    expect(screen.getByText("最近成员审计")).not.toBeNull()
    expect(screen.getByText("member.role")).not.toBeNull()
  })
})

function memberRows() {
  return [
          {
            id: "u1",
            name: "alice",
            display_name: "Alice Chen",
            email: "alice@example.com",
            role: "owner",
            joined_at: 100,
            modified_at: 100,
          },
          {
            id: "u2",
            name: "bob",
            display_name: "Bob Li",
            email: "bob@example.com",
            role: "admin",
            joined_at: 110,
            modified_at: 110,
          },
        ]
}
