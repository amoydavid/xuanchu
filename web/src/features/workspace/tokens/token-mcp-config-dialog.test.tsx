import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { i18n } from "@/i18n"
import { ApiError } from "@/lib/api"

import { TokenMcpConfigDialog } from "./token-mcp-config-dialog"
import type { TokenRow } from "./token-api"

function renderDialog(ui: React.ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <TooltipProvider>{ui}</TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

function okResponse(data: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify({ data }), { status })
  )
}

function errorResponse(code: string, status: number) {
  return Promise.resolve(
    new Response(JSON.stringify({ error: { code } }), { status })
  )
}

function baseToken(overrides: Partial<TokenRow> = {}): TokenRow {
  return {
    id: "tok-1",
    prefix: "xuanchu_agent_abcd",
    name: "claude-agent",
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

function mockFetchForConfig(
  impl: (input: RequestInfo | URL) => unknown
) {
  return vi.spyOn(globalThis, "fetch").mockImplementation((input) => {
    const result = impl(input as RequestInfo | URL)
    if (result instanceof Response) return Promise.resolve(result)
    return okResponse(result as unknown)
  })
}

describe("TokenMcpConfigDialog", () => {
  beforeEach(async () => {
    await i18n.changeLanguage("zh-CN")
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("renders endpoint, bearer token and config JSON", async () => {
    const token = baseToken()
    const raw = "xuanchu_agent_fullsecret"
    mockFetchForConfig(() => ({
      token: raw,
      token_id: token.id,
      token_name: token.name,
      token_type: "agent",
      prefix: token.prefix,
      endpoint_path: "/mcp",
      scopes: ["task:read"],
      workspace_ids: ["w1"],
      project_ids: [],
      expires_at: null,
      revoked_at: null,
    }))

    renderDialog(
      <TokenMcpConfigDialog
        mode="api"
        onOpenChange={() => {}}
        open
        token={token}
      />
    )

    expect(await screen.findByText("MCP Endpoint")).toBeTruthy()
    expect(screen.getByText("http://127.0.0.1:9090/mcp")).toBeTruthy()
    expect(screen.getByText(raw)).toBeTruthy()
    // Bearer 出现在 label 与 config JSON 中，断言至少一处即可。
    expect(screen.getAllByText(/Bearer/).length).toBeGreaterThan(0)
    expect(
      screen.getByText((content) =>
        content.includes('"url": "http://127.0.0.1:9090/mcp"') &&
        content.includes(`"Authorization": "Bearer ${raw}"`)
      )
    ).toBeTruthy()
  })

  it("shows optional X-Xuanchu-As header when agent token has impersonate scope", async () => {
    const token = baseToken()
    mockFetchForConfig(() => ({
      token: "xuanchu_agent_impersonate",
      token_id: token.id,
      token_name: token.name,
      token_type: "agent",
      prefix: token.prefix,
      endpoint_path: "/mcp",
      scopes: ["task:read", "impersonate"],
      workspace_ids: ["w1"],
      project_ids: [],
      expires_at: null,
      revoked_at: null,
    }))

    renderDialog(
      <TokenMcpConfigDialog
        mode="api"
        onOpenChange={() => {}}
        open
        token={token}
      />
    )

    await waitFor(() => {
      expect(screen.getByText("可选 Header")).toBeTruthy()
    })
    // optional header 代码块中含 X-Xuanchu-As；多处出现，用 getAllByText 断言至少一处。
    expect(screen.getAllByText(/X-Xuanchu-As/).length).toBeGreaterThan(0)
  })

  it("shows tenant hint for tenant access token", async () => {
    const token = baseToken({ type: "tenant_access_token" })
    mockFetchForConfig(() => ({
      token: "xuanchu_tenant_full",
      token_id: token.id,
      token_name: token.name,
      token_type: "tenant_access_token",
      prefix: token.prefix,
      endpoint_path: "/mcp",
      scopes: ["task:read"],
      workspace_ids: ["w1"],
      project_ids: [],
      expires_at: null,
      revoked_at: null,
    }))

    renderDialog(
      <TokenMcpConfigDialog
        mode="tenant"
        onOpenChange={() => {}}
        open
        token={token}
      />
    )

    await waitFor(() => {
      expect(
        screen.getByText(/系统身份调用 HTTP MCP/)
      ).toBeTruthy()
    })
  })

  it("shows reissue message when token secret is unavailable", async () => {
    const token = baseToken()
    vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      errorResponse("token_secret_unavailable", 409)
    )

    renderDialog(
      <TokenMcpConfigDialog
        mode="api"
        onOpenChange={() => {}}
        open
        token={token}
      />
    )

    await waitFor(() => {
      expect(screen.getByText(/没有保存可恢复密文/)).toBeTruthy()
    })
  })

  it("copy button calls navigator.clipboard.writeText", async () => {
    const token = baseToken()
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, {
      clipboard: { writeText },
    })

    mockFetchForConfig(() => ({
      token: "xuanchu_agent_clipboard",
      token_id: token.id,
      token_name: token.name,
      token_type: "agent",
      prefix: token.prefix,
      endpoint_path: "/mcp",
      scopes: ["task:read"],
      workspace_ids: ["w1"],
      project_ids: [],
      expires_at: null,
      revoked_at: null,
    }))

    renderDialog(
      <TokenMcpConfigDialog
        mode="api"
        onOpenChange={() => {}}
        open
        token={token}
      />
    )

    // 等数据加载完成
    await waitFor(() => {
      expect(screen.getByText("xuanchu_agent_clipboard")).toBeTruthy()
    })
    // 点击「复制配置」按钮（文案唯一）
    const copyConfigBtn = await screen.findByRole("button", {
      name: /复制配置/,
    })
    await userEvent.click(copyConfigBtn)
    await waitFor(() => {
      expect(writeText).toHaveBeenCalled()
    })
  })

  it("shows secret-key-missing message when server lacks config secret key", async () => {
    const token = baseToken()
    vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      errorResponse("config_secret_key_missing", 400)
    )

    renderDialog(
      <TokenMcpConfigDialog
        mode="api"
        onOpenChange={() => {}}
        open
        token={token}
      />
    )

    await waitFor(() => {
      expect(screen.getByText(/未配置 config secret key/)).toBeTruthy()
    })
  })

  it("ApiError type guard aligns with backend codes", () => {
    // 静态校验：ApiError 携带 code，供 dialog 分支判断。
    const err = new ApiError(409, "token_secret_unavailable", "x")
    expect(err.code).toBe("token_secret_unavailable")
    expect(err instanceof ApiError).toBe(true)
    // _ = token 占位，避免未使用变量告警
    void baseToken
  })
})
