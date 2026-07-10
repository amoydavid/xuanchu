import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { setWorkspaceToken } from "@/features/workspace/session/workspace-token"
import { i18n } from "@/i18n"

import { TokensPage } from "./tokens-page"
import type { TenantAccessTokenRow, TokenRow } from "./token-api"

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <TooltipProvider>
          <TokensPage />
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

function makeToken(overrides: Partial<TokenRow> = {}): TokenRow {
  return {
    id: "tok-1",
    prefix: "xuanchu_pat_abc",
    name: "ci-deploy",
    type: "agent",
    user: { id: "u1", name: "local" },
    workspace_ids: ["w1"],
    project_ids: [],
    scopes: ["task:read", "task:write"],
    created_at: 100,
    expires_at: null,
    revoked_at: null,
    last_used_at: 200,
    ...overrides,
  }
}

function makeTenantToken(
  overrides: Partial<TenantAccessTokenRow> = {}
): TenantAccessTokenRow {
  return {
    id: "tenant-1",
    prefix: "xuanchu_tenant_abc",
    name: "tenant-ci",
    type: "tenant_access_token",
    workspace_id: "w1",
    project_ids: [],
    scopes: ["task:read"],
    created_at: 100,
    expires_at: null,
    revoked_at: null,
    last_used_at: null,
    ...overrides,
  }
}

function credentialCurrentResponse(
  overrides: Record<string, unknown> = {}
) {
  return {
    actor_type: "user",
    actor: { id: "u-admin", name: "local" },
    token: { type: "pat", scopes: ["token:read", "token:write"] },
    effective_workspace: { slug: "local" },
    effective_role: "owner",
    ...overrides,
  }
}

function membersResponse() {
  return [
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
}

describe("TokensPage", () => {
  beforeEach(async () => {
    sessionStorage.clear()
    setWorkspaceToken("xuanchu_pat_test")
    await i18n.changeLanguage("zh-CN")
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("renders token rows with name, type, scopes count and status", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((input) => {
        const url = String(input)
        if (url.includes("/api/v1/credentials/current")) {
          return okResponse(credentialCurrentResponse())
        }
        if (url.includes("/api/v1/tokens")) {
          return okResponse([makeToken()])
        }
        return okResponse([])
      })

    renderPage()

    await waitFor(() => {
      expect(screen.getByText("ci-deploy")).toBeTruthy()
    })

    // type 显示为 Badge
    expect(screen.getByText("agent")).toBeTruthy()
    // scopes 数量（格式「2 权限范围 (Scope)」）
    expect(screen.getByText(/2 .*权限范围/)).toBeTruthy()
    // 状态：有效
    expect(screen.getByText("有效")).toBeTruthy()
    // 创建按钮
    expect(screen.getByText("创建 Token")).toBeTruthy()

    fetchMock.mockRestore()
  })

  it("renders tenant access token rows from tenant tab", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((input) => {
        const url = String(input)
        if (url.includes("/api/v1/credentials/current")) {
          return okResponse(credentialCurrentResponse())
        }
        if (url.includes("/api/v1/tenant-access-tokens")) {
          return okResponse([
            makeTenantToken({
              id: "tenant-1",
              name: "tenant-ci",
              prefix: "xuanchu_tenant_abc",
              type: "tenant_access_token",
              workspace_id: "ws-local",
            }),
          ])
        }
        if (url.includes("/api/v1/tokens")) {
          return okResponse([makeToken()])
        }
        return okResponse([])
      })

    renderPage()

    await userEvent.click(
      await screen.findByRole("tab", { name: "租户访问令牌" })
    )

    await waitFor(() => {
      expect(screen.getByText("tenant-ci")).toBeTruthy()
    })
    expect(
      fetchMock.mock.calls.some((call) =>
        String(call[0]).includes("/api/v1/tenant-access-tokens")
      )
    ).toBe(true)
    expect(screen.queryByText("xuanchu_tenant_raw_secret")).toBeNull()

    fetchMock.mockRestore()
  })

  it("opens tenant token tab directly for tenant system identity", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((input) => {
        const url = String(input)
        if (url.includes("/api/v1/credentials/current")) {
          return okResponse(
            credentialCurrentResponse({
              actor_type: "tenant_access_token",
              actor: {
                name: "tenant-runtime",
                display_name: "系统身份 / tenant-runtime",
              },
              token: {
                type: "tenant_access_token",
                scopes: ["token:read", "token:write"],
              },
            })
          )
        }
        if (url.includes("/api/v1/tenant-access-tokens")) {
          return okResponse([makeTenantToken({ name: "tenant-runtime" })])
        }
        if (url.includes("/api/v1/tokens")) {
          return Promise.resolve(
            new Response(JSON.stringify({ error: { code: "tenant_actor_not_user" } }), {
              status: 400,
            })
          )
        }
        return okResponse([])
      })

    renderPage()

    await waitFor(() => {
      expect(screen.getByText("tenant-runtime")).toBeTruthy()
    })
    expect(
      (screen.getByRole("tab", { name: "普通 API Tokens" }) as HTMLButtonElement)
        .disabled
    ).toBe(true)
    expect(
      fetchMock.mock.calls.some((call) =>
        String(call[0]).includes("/api/v1/tokens")
      )
    ).toBe(false)

    fetchMock.mockRestore()
  })

  it("shows revoked row with reduced opacity (revoked status badge)", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((input) => {
        const url = String(input)
        if (url.includes("/api/v1/credentials/current")) {
          return okResponse(
            credentialCurrentResponse({
              token: { type: "pat", scopes: ["token:read"] },
            })
          )
        }
        if (url.includes("/api/v1/tokens")) {
          return okResponse([
            makeToken({ name: "legacy", revoked_at: 999, id: "tok-2" }),
          ])
        }
        return okResponse([])
      })

    renderPage()

    await waitFor(() => {
      expect(screen.getByText("已吊销")).toBeTruthy()
    })
    expect(screen.getByText("legacy")).toBeTruthy()

    fetchMock.mockRestore()
  })

  it("renders empty state when no tokens", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation(() => okResponse([]))

    renderPage()

    await waitFor(() => {
      expect(screen.getByText("暂无数据")).toBeTruthy()
    })

    fetchMock.mockRestore()
  })

  it("opens MCP config dialog for API token rows", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((input) => {
        const url = String(input)
        if (url.includes("/api/v1/credentials/current")) {
          return okResponse(credentialCurrentResponse())
        }
        if (url.includes("/mcp-config")) {
          return okResponse({
            token: "xuanchu_agent_revealed",
            token_id: "tok-1",
            token_name: "ci-deploy",
            token_type: "agent",
            prefix: "xuanchu_pat_abc",
            endpoint_path: "/mcp",
            scopes: ["task:read"],
            workspace_ids: ["w1"],
            project_ids: [],
            expires_at: null,
            revoked_at: null,
          })
        }
        if (url.includes("/api/v1/tokens")) {
          return okResponse([makeToken()])
        }
        return okResponse([])
      })

    renderPage()

    const mcpButton = await screen.findByRole("button", {
      name: "MCP 配置：ci-deploy",
    })
    await userEvent.click(mcpButton)

    await waitFor(() => {
      expect(screen.getByText("xuanchu_agent_revealed")).toBeTruthy()
    })
    expect(
      fetchMock.mock.calls.some((call) =>
        String(call[0]).includes("/api/v1/tokens/tok-1/mcp-config")
      )
    ).toBe(true)

    fetchMock.mockRestore()
  })

  it("opens MCP config dialog for tenant token rows", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((input) => {
        const url = String(input)
        if (url.includes("/api/v1/credentials/current")) {
          return okResponse(credentialCurrentResponse())
        }
        if (url.includes("/tenant-access-tokens/") && url.includes("/mcp-config")) {
          return okResponse({
            token: "xuanchu_tenant_revealed",
            token_id: "tenant-1",
            token_name: "tenant-ci",
            token_type: "tenant_access_token",
            prefix: "xuanchu_tenant_abc",
            endpoint_path: "/mcp",
            scopes: ["task:read"],
            workspace_ids: ["ws-local"],
            project_ids: [],
            expires_at: null,
            revoked_at: null,
          })
        }
        if (url.includes("/api/v1/tenant-access-tokens")) {
          return okResponse([makeTenantToken()])
        }
        if (url.includes("/api/v1/tokens")) {
          return okResponse([])
        }
        return okResponse([])
      })

    renderPage()

    await userEvent.click(
      await screen.findByRole("tab", { name: "租户访问令牌" })
    )

    const mcpButton = await screen.findByRole("button", {
      name: "MCP 配置：tenant-ci",
    })
    await userEvent.click(mcpButton)

    await waitFor(() => {
      expect(screen.getByText("xuanchu_tenant_revealed")).toBeTruthy()
    })

    fetchMock.mockRestore()
  })

  it("shows secret unavailable message from MCP config dialog", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((input) => {
        const url = String(input)
        if (url.includes("/api/v1/credentials/current")) {
          return okResponse(credentialCurrentResponse())
        }
        if (url.includes("/mcp-config")) {
          return Promise.resolve(
            new Response(
              JSON.stringify({ error: { code: "token_secret_unavailable" } }),
              { status: 409 }
            )
          )
        }
        if (url.includes("/api/v1/tokens")) {
          return okResponse([makeToken()])
        }
        return okResponse([])
      })

    renderPage()

    const mcpButton = await screen.findByRole("button", {
      name: "MCP 配置：ci-deploy",
    })
    await userEvent.click(mcpButton)

    await waitFor(() => {
      expect(screen.getByText(/没有保存可恢复密文/)).toBeTruthy()
    })

    fetchMock.mockRestore()
  })

  it("shows error message on API failure", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((input) => {
        const url = String(input)
        if (url.includes("/api/v1/credentials/current")) {
          return okResponse(
            credentialCurrentResponse({
              token: { type: "pat", scopes: [] },
            })
          )
        }
        return Promise.resolve(
          new Response(
            JSON.stringify({ error: { code: "token_not_found" } }),
            { status: 500 }
          )
        )
      })

    renderPage()

    await waitFor(() => {
      expect(screen.getByText(/发生错误/)).toBeTruthy()
    })

    fetchMock.mockRestore()
  })

  it("shows owner user field in create dialog for admin", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((input) => {
        const url = String(input)
        if (url.includes("/api/v1/credentials/current")) {
          return okResponse(credentialCurrentResponse())
        }
        if (url.includes("/api/v1/workspaces/") && url.includes("/members")) {
          return okResponse(membersResponse())
        }
        if (url.includes("/api/v1/tokens")) {
          return okResponse([makeToken()])
        }
        return okResponse([])
      })

    renderPage()

    await waitFor(() => {
      expect(screen.getByText("ci-deploy")).toBeTruthy()
    })
    await userEvent.click(screen.getByText("创建 Token"))

    await waitFor(() => {
      expect(screen.getByText("归属用户")).toBeTruthy()
    })
    // 默认「我自己」：列表筛选器和创建弹窗各有一个 UserPicker，均默认显示「我自己」
    expect(screen.getAllByText(/我自己/).length).toBeGreaterThanOrEqual(1)

    fetchMock.mockRestore()
  })

  it("hides owner user field for member role", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((input) => {
        const url = String(input)
        if (url.includes("/api/v1/credentials/current")) {
          return okResponse(
            credentialCurrentResponse({
              effective_role: "member",
              actor: { id: "u-member", name: "member" },
            })
          )
        }
        if (url.includes("/api/v1/tokens")) {
          return okResponse([makeToken()])
        }
        return okResponse([])
      })

    renderPage()

    await waitFor(() => {
      expect(screen.getByText("ci-deploy")).toBeTruthy()
    })
    await userEvent.click(screen.getByText("创建 Token"))

    await waitFor(() => {
      expect(screen.queryByText("归属用户")).toBeNull()
    })
    // member 不显示列表筛选器
    expect(screen.queryByText("查看用户")).toBeNull()

    fetchMock.mockRestore()
  })

  it("shows view user filter for admin", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((input) => {
        const url = String(input)
        if (url.includes("/api/v1/credentials/current")) {
          return okResponse(credentialCurrentResponse())
        }
        if (url.includes("/api/v1/workspaces/") && url.includes("/members")) {
          return okResponse(membersResponse())
        }
        if (url.includes("/api/v1/tokens")) {
          return okResponse([makeToken()])
        }
        return okResponse([])
      })

    renderPage()

    await waitFor(() => {
      expect(screen.getByText("查看用户")).toBeTruthy()
    })

    fetchMock.mockRestore()
  })

  it("requests tokens filtered by user when filter changes", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((input) => {
        const url = String(input)
        if (url.includes("/api/v1/credentials/current")) {
          return okResponse(credentialCurrentResponse())
        }
        if (url.includes("/api/v1/workspaces/") && url.includes("/members")) {
          return okResponse(membersResponse())
        }
        if (url.includes("/api/v1/tokens")) {
          return okResponse([
            makeToken({ name: "zhang-token", id: "tok-zhang" }),
          ])
        }
        return okResponse([])
      })

    renderPage()

    await waitFor(() => {
      expect(screen.getByText("查看用户")).toBeTruthy()
    })
    // 打开筛选器（列表顶部的 combobox）
    const filterCombobox = screen.getAllByRole("combobox")[0]
    await userEvent.click(filterCombobox)
    await waitFor(() => {
      expect(screen.getByText("张三")).toBeTruthy()
    })
    await userEvent.click(screen.getByText("张三"))

    await waitFor(() => {
      expect(
        fetchMock.mock.calls.some((call) =>
          String(call[0]).includes("/api/v1/tokens?user=u-zhang")
        )
      ).toBe(true)
    })

    fetchMock.mockRestore()
  })

  it("reverts to self when filter switched back from another member", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((input) => {
        const url = String(input)
        if (url.includes("/api/v1/credentials/current")) {
          return okResponse(credentialCurrentResponse())
        }
        if (url.includes("/api/v1/workspaces/") && url.includes("/members")) {
          return okResponse(membersResponse())
        }
        if (url.includes("/api/v1/tokens")) {
          return okResponse([makeToken()])
        }
        return okResponse([])
      })

    renderPage()
    await waitFor(() => {
      expect(screen.getByText("查看用户")).toBeTruthy()
    })

    const filterCombobox = () => screen.getAllByRole("combobox")[0]

    // 切到张三
    await userEvent.click(filterCombobox())
    await waitFor(() => expect(screen.getByText("张三")).toBeTruthy())
    await userEvent.click(screen.getByText("张三"))
    await waitFor(() => {
      expect(
        fetchMock.mock.calls.some((call) =>
          String(call[0]).includes("/api/v1/tokens?user=u-zhang")
        )
      ).toBe(true)
    })

    // 记录切回前的请求数
    const callsBefore = fetchMock.mock.calls.length

    // 切回「我自己」
    await userEvent.click(filterCombobox())
    await waitFor(() => expect(screen.getByText(/我自己/)).toBeTruthy())
    await userEvent.click(screen.getAllByText(/我自己/)[0])

    // 切回后应重新请求不带 ?user= 的 /api/v1/tokens
    await waitFor(() => {
      expect(
        fetchMock.mock.calls
          .slice(callsBefore)
          .some(
            (call) =>
              String(call[0]).includes("/api/v1/tokens") &&
              !String(call[0]).includes("user=")
          )
      ).toBe(true)
    })

    fetchMock.mockRestore()
  })
})
